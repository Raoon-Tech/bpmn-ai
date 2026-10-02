//go:build integration

package sql

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"

	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

var testStore *Store
var testPool *dockertest.Pool
var testResource *dockertest.Resource

func TestMain(m *testing.M) {
	if os.Getenv("PG_URL") != "" {
		// Use external PostgreSQL
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s, err := NewStore(ctx, os.Getenv("PG_URL"))
		if err != nil {
			log.Fatalf("connect: %v", err)
		}
		testStore = s
		os.Exit(m.Run())
	}

	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Fatalf("dockertest: %v", err)
	}
	testPool = pool

	resource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "postgres",
		Tag:        "16-alpine",
		Env: []string{
			"POSTGRES_USER=bpmn",
			"POSTGRES_PASSWORD=bpmn",
			"POSTGRES_DB=bpmn",
		},
	}, func(config *docker.HostConfig) {
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	if err != nil {
		log.Fatalf("postgres container: %v", err)
	}
	testResource = resource

	hostPort := resource.GetPort("5432/tcp")
	dsn := fmt.Sprintf("postgres://bpmn:bpmn@localhost:%s/bpmn?sslmode=disable", hostPort)

	pool.Retry(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		s, err := NewStore(ctx, dsn)
		if err != nil {
			return err
		}
		testStore = s
		return nil
	})

	if testStore == nil {
		log.Fatalf("failed to connect to test postgres")
	}

	runMigrations(testStore)

	code := m.Run()

	if err := pool.Purge(resource); err != nil {
		log.Printf("purge: %v", err)
	}
	os.Exit(code)
}

func runMigrations(s *Store) {
	migration := `
	CREATE TABLE IF NOT EXISTS processes (
		id TEXT PRIMARY KEY, name TEXT NOT NULL, version INT NOT NULL DEFAULT 1,
		definition JSONB NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE TABLE IF NOT EXISTS instances (
		id TEXT PRIMARY KEY, process_id TEXT NOT NULL REFERENCES processes(id),
		title TEXT, status TEXT NOT NULL DEFAULT 'CREATED', current_user TEXT,
		variables JSONB NOT NULL DEFAULT '{}', pin TEXT, started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), finished_at TIMESTAMPTZ
	);
	CREATE TABLE IF NOT EXISTS flows (
		id TEXT PRIMARY KEY, instance_id TEXT NOT NULL REFERENCES instances(id),
		element_id TEXT NOT NULL, element_type TEXT NOT NULL, thread_id INT NOT NULL DEFAULT 1,
		previous_id TEXT, status TEXT NOT NULL DEFAULT 'PENDING',
		started_at TIMESTAMPTZ, finished_at TIMESTAMPTZ, duration_ms INT
	);
	CREATE TABLE IF NOT EXISTS threads (
		id SERIAL PRIMARY KEY, instance_id TEXT NOT NULL REFERENCES instances(id),
		thread_index INT NOT NULL, parent_index INT, flow_id TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'ACTIVE', created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		UNIQUE(instance_id, thread_index)
	);
	CREATE TABLE IF NOT EXISTS jobs (
		id TEXT PRIMARY KEY, instance_id TEXT NOT NULL REFERENCES instances(id),
		flow_id TEXT, type TEXT NOT NULL, payload JSONB NOT NULL,
		status TEXT NOT NULL DEFAULT 'PENDING', retry_count INT NOT NULL DEFAULT 0,
		max_retries INT NOT NULL DEFAULT 3, scheduled_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		executed_at TIMESTAMPTZ, error_message TEXT, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE TABLE IF NOT EXISTS dead_letters (
		id TEXT PRIMARY KEY, job_id TEXT, instance_id TEXT NOT NULL REFERENCES instances(id),
		type TEXT, payload JSONB NOT NULL, error_message TEXT NOT NULL,
		retry_count INT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE TABLE IF NOT EXISTS execution_log (
		id TEXT PRIMARY KEY, instance_id TEXT NOT NULL REFERENCES instances(id),
		element_id TEXT NOT NULL, element_type TEXT NOT NULL, action TEXT NOT NULL,
		duration_ms INT, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);`
	if _, err := testStore.pool.Exec(context.Background(), migration); err != nil {
		log.Fatalf("migration: %v", err)
	}
}

func TestPgSaveAndGetProcess(t *testing.T) {
	proc := &bpmn.Process{
		ID:   "test-proc-1",
		Name: "Test Process",
		Elements: map[string]bpmn.Element{
			"s1": {ID: "s1", Type: bpmn.ElementTypeStartEvent},
			"e1": {ID: "e1", Type: bpmn.ElementTypeEndEvent},
		},
		Flows: map[string]bpmn.Flow{
			"f1": {ID: "f1", SourceRef: "s1", TargetRef: "e1"},
		},
	}
	if err := testStore.SaveProcess(context.Background(), proc); err != nil {
		t.Fatalf("SaveProcess: %v", err)
	}
	got, err := testStore.GetProcess(context.Background(), "test-proc-1")
	if err != nil {
		t.Fatalf("GetProcess: %v", err)
	}
	if got.Name != "Test Process" {
		t.Errorf("expected Test Process, got %s", got.Name)
	}
	if len(got.Elements) != 2 {
		t.Errorf("expected 2 elements, got %d", len(got.Elements))
	}
}

func TestPgGetProcess_NotFound(t *testing.T) {
	_, err := testStore.GetProcess(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent process")
	}
}

func TestPgCreateAndGetInstance(t *testing.T) {
	saveTestProcess(t, "inst-proc")

	inst := &store.InstanceRecord{
		ID:        uuid.New().String(),
		ProcessID: "inst-proc",
		Status:    store.InstanceStateCreated,
		Variables: map[string]any{"key": "value"},
	}
	if err := testStore.CreateInstance(context.Background(), inst); err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	got, err := testStore.GetInstance(context.Background(), inst.ID)
	if err != nil {
		t.Fatalf("GetInstance: %v", err)
	}
	if got.Status != store.InstanceStateCreated {
		t.Errorf("expected CREATED, got %s", got.Status)
	}
	if got.Variables["key"] != "value" {
		t.Errorf("expected key=value, got %v", got.Variables["key"])
	}
}

func TestPgUpdateInstance(t *testing.T) {
	saveTestProcess(t, "upd-proc")
	inst := createTestInstance(t, "upd-proc")

	inst.Status = store.InstanceStateCompleted
	now := time.Now()
	inst.FinishedAt = &now
	if err := testStore.UpdateInstance(context.Background(), inst); err != nil {
		t.Fatalf("UpdateInstance: %v", err)
	}
	got, err := testStore.GetInstance(context.Background(), inst.ID)
	if err != nil {
		t.Fatalf("GetInstance: %v", err)
	}
	if got.Status != store.InstanceStateCompleted {
		t.Errorf("expected COMPLETED, got %s", got.Status)
	}
}

func TestPgFlowLifecycle(t *testing.T) {
	saveTestProcess(t, "flow-proc")
	inst := createTestInstance(t, "flow-proc")

	flow := &store.FlowRecord{
		InstanceID:  inst.ID,
		ElementID:   "s1",
		ElementType: bpmn.ElementTypeStartEvent,
		ThreadID:    1,
		Status:      store.FlowStatusActive,
	}
	if err := testStore.CreateFlow(context.Background(), flow); err != nil {
		t.Fatalf("CreateFlow: %v", err)
	}
	if flow.ID == "" {
		t.Fatal("expected flow ID to be set")
	}

	got, err := testStore.GetFlow(context.Background(), flow.ID)
	if err != nil {
		t.Fatalf("GetFlow: %v", err)
	}
	if got.ElementID != "s1" {
		t.Errorf("expected s1, got %s", got.ElementID)
	}

	// Update
	now := time.Now()
	got.Status = store.FlowStatusCompleted
	got.FinishedAt = &now
	dur := 100
	got.DurationMs = &dur
	if err := testStore.UpdateFlow(context.Background(), got); err != nil {
		t.Fatalf("UpdateFlow: %v", err)
	}

	flows, err := testStore.GetFlowsByInstance(context.Background(), inst.ID)
	if err != nil {
		t.Fatalf("GetFlowsByInstance: %v", err)
	}
	if len(flows) != 1 {
		t.Fatalf("expected 1 flow, got %d", len(flows))
	}
	if flows[0].Status != store.FlowStatusCompleted {
		t.Errorf("expected COMPLETED, got %s", flows[0].Status)
	}
}

func TestPgThreads(t *testing.T) {
	saveTestProcess(t, "thread-proc")
	inst := createTestInstance(t, "thread-proc")
	flow := createTestFlow(t, inst.ID, "s1")

	thread := &store.ThreadRecord{
		InstanceID:  inst.ID,
		ThreadIndex: 1,
		ParentIndex: nil,
		FlowID:      flow.ID,
		Status:      "ACTIVE",
	}
	if err := testStore.CreateThread(context.Background(), thread); err != nil {
		t.Fatalf("CreateThread: %v", err)
	}

	threads, err := testStore.GetThreadsByInstance(context.Background(), inst.ID)
	if err != nil {
		t.Fatalf("GetThreadsByInstance: %v", err)
	}
	if len(threads) != 1 {
		t.Fatalf("expected 1 thread, got %d", len(threads))
	}

	if err := testStore.CloseThread(context.Background(), inst.ID, 1); err != nil {
		t.Fatalf("CloseThread: %v", err)
	}
}

func TestPgJobLifecycle(t *testing.T) {
	saveTestProcess(t, "job-proc")
	inst := createTestInstance(t, "job-proc")

	job := &store.JobRecord{
		InstanceID:  inst.ID,
		Type:        "test",
		Payload:     map[string]any{"msg": "hello"},
		Status:      store.JobStatusPending,
		ScheduledAt: time.Now(),
	}
	if err := testStore.CreateJob(context.Background(), job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if job.ID == "" {
		t.Fatal("expected job ID")
	}

	got, err := testStore.GetJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if got.Type != "test" {
		t.Errorf("expected test, got %s", got.Type)
	}

	// Get pending
	pending, err := testStore.GetPendingJobs(context.Background(), 10)
	if err != nil {
		t.Fatalf("GetPendingJobs: %v", err)
	}
	if len(pending) < 1 {
		t.Fatal("expected at least 1 pending job")
	}

	// Update
	got.Status = store.JobStatusCompleted
	if err := testStore.UpdateJob(context.Background(), got); err != nil {
		t.Fatalf("UpdateJob: %v", err)
	}
}

func TestPgDeadLetter(t *testing.T) {
	saveTestProcess(t, "dl-proc")
	inst := createTestInstance(t, "dl-proc")

	dl := &store.DeadLetterRecord{
		InstanceID:   inst.ID,
		JobID:        uuid.New().String(),
		Type:         "test",
		Payload:      map[string]any{"err": "fail"},
		ErrorMessage: "something went wrong",
		RetryCount:   3,
	}
	if err := testStore.CreateDeadLetter(context.Background(), dl); err != nil {
		t.Fatalf("CreateDeadLetter: %v", err)
	}
	if dl.ID == "" {
		t.Fatal("expected dead letter ID")
	}

	got, err := testStore.GetDeadLetter(context.Background(), dl.ID)
	if err != nil {
		t.Fatalf("GetDeadLetter: %v", err)
	}
	if got.ErrorMessage != "something went wrong" {
		t.Errorf("expected 'something went wrong', got %s", got.ErrorMessage)
	}

	list, err := testStore.ListDeadLetters(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListDeadLetters: %v", err)
	}
	if len(list) < 1 {
		t.Fatal("expected at least 1 dead letter")
	}

	byInstance, err := testStore.GetDeadLetters(context.Background(), inst.ID)
	if err != nil {
		t.Fatalf("GetDeadLetters: %v", err)
	}
	if len(byInstance) != 1 {
		t.Errorf("expected 1, got %d", len(byInstance))
	}
}

func TestPgExecutionLog(t *testing.T) {
	saveTestProcess(t, "log-proc")
	inst := createTestInstance(t, "log-proc")

	entry := &store.ExecutionLogEntry{
		InstanceID:  inst.ID,
		ElementID:   "s1",
		ElementType: bpmn.ElementTypeStartEvent,
		Action:      "ROUTE",
		DurationMs:  5,
	}
	if err := testStore.LogExecution(context.Background(), entry); err != nil {
		t.Fatalf("LogExecution: %v", err)
	}

	logs, err := testStore.GetExecutionLog(context.Background(), inst.ID)
	if err != nil {
		t.Fatalf("GetExecutionLog: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(logs))
	}
}

func TestPgListProcesses(t *testing.T) {
	saveTestProcess(t, "list-proc-1")
	saveTestProcess(t, "list-proc-2")
	procs, err := testStore.ListProcesses(context.Background())
	if err != nil {
		t.Fatalf("ListProcesses: %v", err)
	}
	if len(procs) < 2 {
		t.Errorf("expected at least 2 processes, got %d", len(procs))
	}
}

func TestPgListInstances(t *testing.T) {
	saveTestProcess(t, "list-inst-proc")
	createTestInstance(t, "list-inst-proc")

	all, err := testStore.ListInstances(context.Background(), "")
	if err != nil {
		t.Fatalf("ListInstances: %v", err)
	}
	if len(all) < 1 {
		t.Errorf("expected at least 1 instance")
	}

	filtered, err := testStore.ListInstances(context.Background(), store.InstanceStateCreated)
	if err != nil {
		t.Fatalf("ListInstances filtered: %v", err)
	}
	if len(filtered) < 1 {
		t.Errorf("expected at least 1 CREATED instance")
	}
}

// --- helpers ---

func saveTestProcess(t *testing.T, id string) {
	t.Helper()
	proc := &bpmn.Process{
		ID:   id,
		Name: "Test " + id,
		Elements: map[string]bpmn.Element{
			"s1": {ID: "s1", Type: bpmn.ElementTypeStartEvent},
			"e1": {ID: "e1", Type: bpmn.ElementTypeEndEvent},
		},
		Flows: map[string]bpmn.Flow{
			"f1": {ID: "f1", SourceRef: "s1", TargetRef: "e1"},
		},
	}
	if err := testStore.SaveProcess(context.Background(), proc); err != nil {
		t.Fatalf("SaveProcess: %v", err)
	}
}

func createTestInstance(t *testing.T, processID string) *store.InstanceRecord {
	t.Helper()
	inst := &store.InstanceRecord{
		ID:        uuid.New().String(),
		ProcessID: processID,
		Status:    store.InstanceStateCreated,
	}
	if err := testStore.CreateInstance(context.Background(), inst); err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	return inst
}

func createTestFlow(t *testing.T, instanceID, elementID string) *store.FlowRecord {
	t.Helper()
	flow := &store.FlowRecord{
		InstanceID:  instanceID,
		ElementID:   elementID,
		ElementType: bpmn.ElementTypeStartEvent,
		ThreadID:    1,
		Status:      store.FlowStatusActive,
	}
	if err := testStore.CreateFlow(context.Background(), flow); err != nil {
		t.Fatalf("CreateFlow: %v", err)
	}
	return flow
}
