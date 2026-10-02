package memory

import (
	"context"
	"testing"
	"time"

	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

func TestStore_SaveAndGetProcess(t *testing.T) {
	s := NewStore()
	ctx := context.Background()

	proc := &bpmn.Process{
		ID:           "proc-1",
		Name:         "Test Process",
		Elements:     make(map[string]bpmn.Element),
		Flows:        make(map[string]bpmn.Flow),
	}

	if err := s.SaveProcess(ctx, proc); err != nil {
		t.Fatalf("save process: %v", err)
	}

	got, err := s.GetProcess(ctx, "proc-1")
	if err != nil {
		t.Fatalf("get process: %v", err)
	}
	if got.ID != "proc-1" {
		t.Errorf("expected ID proc-1, got %s", got.ID)
	}
	if got.Name != "Test Process" {
		t.Errorf("expected name Test Process, got %s", got.Name)
	}
}

func TestStore_GetProcess_NotFound(t *testing.T) {
	s := NewStore()
	_, err := s.GetProcess(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent process")
	}
}

func TestStore_CreateGetInstance(t *testing.T) {
	s := NewStore()
	ctx := context.Background()

	inst := &store.InstanceRecord{
		ID:        "case-1",
		ProcessID: "proc-1",
		Status:    store.InstanceStatusCreated,
		Variables: map[string]any{"key": "value"},
	}

	if err := s.CreateInstance(ctx, inst); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	got, err := s.GetInstance(ctx, "case-1")
	if err != nil {
		t.Fatalf("get instance: %v", err)
	}
	if got.ID != "case-1" {
		t.Errorf("expected ID case-1, got %s", got.ID)
	}
	if got.Variables["key"] != "value" {
		t.Errorf("expected variable value, got %v", got.Variables["key"])
	}
}

func TestStore_UpdateInstance(t *testing.T) {
	s := NewStore()
	ctx := context.Background()

	inst := &store.InstanceRecord{
		ID:     "case-1",
		Status: store.InstanceStatusCreated,
	}
	s.CreateInstance(ctx, inst)

	inst.Status = store.InstanceStatusInProgress
	s.UpdateInstance(ctx, inst)

	got, _ := s.GetInstance(ctx, "case-1")
	if got.Status != store.InstanceStatusInProgress {
		t.Errorf("expected status IN_PROGRESS, got %s", got.Status)
	}
}

func TestStore_CreateGetFlow(t *testing.T) {
	s := NewStore()
	ctx := context.Background()

	flow := &store.FlowRecord{
		ID:          "flow-1",
		InstanceID:  "case-1",
		ElementID:   "start-1",
		ElementType: bpmn.ElementTypeStartEvent,
		Status:      store.FlowStatusActive,
	}

	if err := s.CreateFlow(ctx, flow); err != nil {
		t.Fatalf("create flow: %v", err)
	}

	got, err := s.GetFlow(ctx, "flow-1")
	if err != nil {
		t.Fatalf("get flow: %v", err)
	}
	if got.ElementID != "start-1" {
		t.Errorf("expected elementID start-1, got %s", got.ElementID)
	}
}

func TestStore_UpdateFlow(t *testing.T) {
	s := NewStore()
	ctx := context.Background()

	flow := &store.FlowRecord{
		ID:          "flow-1",
		InstanceID:  "case-1",
		ElementID:   "task-1",
		ElementType: bpmn.ElementTypeUserTask,
		Status:      store.FlowStatusActive,
	}
	if err := s.CreateFlow(ctx, flow); err != nil {
		t.Fatalf("create flow: %v", err)
	}

	flow.Status = store.FlowStatusCompleted
	now := time.Now()
	flow.FinishedAt = &now
	d := 100
	flow.DurationMs = &d

	if err := s.UpdateFlow(ctx, flow); err != nil {
		t.Fatalf("update flow: %v", err)
	}

	got, err := s.GetFlow(ctx, "flow-1")
	if err != nil {
		t.Fatalf("get flow: %v", err)
	}
	if got.Status != store.FlowStatusCompleted {
		t.Errorf("expected status COMPLETED, got %s", got.Status)
	}
	if got.FinishedAt == nil {
		t.Errorf("expected FinishedAt to be set")
	}
	if got.DurationMs == nil || *got.DurationMs != 100 {
		t.Errorf("expected DurationMs 100, got %v", got.DurationMs)
	}
}

func TestStore_GetFlow_NotFound(t *testing.T) {
	s := NewStore()
	_, err := s.GetFlow(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent flow")
	}
}

func TestStore_GetFlowsByInstance(t *testing.T) {
	s := NewStore()
	ctx := context.Background()

	s.CreateFlow(ctx, &store.FlowRecord{ID: "f1", InstanceID: "case-1", ElementID: "e1"})
	s.CreateFlow(ctx, &store.FlowRecord{ID: "f2", InstanceID: "case-1", ElementID: "e2"})
	s.CreateFlow(ctx, &store.FlowRecord{ID: "f3", InstanceID: "case-2", ElementID: "e3"})

	flows, err := s.GetFlowsByInstance(ctx, "case-1")
	if err != nil {
		t.Fatalf("get flows: %v", err)
	}
	if len(flows) != 2 {
		t.Errorf("expected 2 flows, got %d", len(flows))
	}
}

func TestStore_Jobs(t *testing.T) {
	s := NewStore()
	ctx := context.Background()

	job := &store.JobRecord{
		ID:         "job-1",
		InstanceID: "case-1",
		Type:       "timer",
		Status:     store.JobStatusPending,
		MaxRetries: 3,
	}

	if err := s.CreateJob(ctx, job); err != nil {
		t.Fatalf("create job: %v", err)
	}

	pending, err := s.GetPendingJobs(ctx, 10)
	if err != nil {
		t.Fatalf("get pending jobs: %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("expected 1 pending job, got %d", len(pending))
	}

	got, err := s.GetJob(ctx, "job-1")
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if got.Type != "timer" {
		t.Errorf("expected type timer, got %s", got.Type)
	}
}

func TestStore_ExecutionLog(t *testing.T) {
	s := NewStore()
	ctx := context.Background()

	entry := &store.ExecutionLogEntry{
		InstanceID:  "case-1",
		ElementID:   "start-1",
		ElementType: bpmn.ElementTypeStartEvent,
		Action:      "ROUTE",
		DurationMs:  5,
	}

	if err := s.LogExecution(ctx, entry); err != nil {
		t.Fatalf("log execution: %v", err)
	}

	logs, err := s.GetExecutionLog(ctx, "case-1")
	if err != nil {
		t.Fatalf("get execution log: %v", err)
	}
	if len(logs) != 1 {
		t.Errorf("expected 1 log entry, got %d", len(logs))
	}
	if logs[0].Action != "ROUTE" {
		t.Errorf("expected action ROUTE, got %s", logs[0].Action)
	}
}

func TestStore_Reset(t *testing.T) {
	s := NewStore()
	ctx := context.Background()

	s.SaveProcess(ctx, &bpmn.Process{ID: "p1", Elements: make(map[string]bpmn.Element), Flows: make(map[string]bpmn.Flow)})
	s.CreateInstance(ctx, &store.InstanceRecord{ID: "c1"})
	s.CreateFlow(ctx, &store.FlowRecord{ID: "f1"})
	s.CreateJob(ctx, &store.JobRecord{ID: "j1"})

	s.Reset()

	procs, _ := s.ListProcesses(ctx)
	if len(procs) != 0 {
		t.Errorf("expected 0 processes after reset, got %d", len(procs))
	}

	insts, _ := s.ListInstances(ctx, "")
	if len(insts) != 0 {
		t.Errorf("expected 0 instances after reset, got %d", len(insts))
	}
}

func TestStore_UpdateJob(t *testing.T) {
	s := NewStore()
	ctx := context.Background()

	job := &store.JobRecord{ID: "uj-1", InstanceID: "c-1", Type: "test", Status: store.JobStatusPending}
	if err := s.CreateJob(ctx, job); err != nil {
		t.Fatalf("create job: %v", err)
	}
	job.Status = store.JobStatusCompleted
	now := time.Now()
	job.ExecutedAt = &now
	if err := s.UpdateJob(ctx, job); err != nil {
		t.Fatalf("update job: %v", err)
	}
	got, err := s.GetJob(ctx, "uj-1")
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if got.Status != store.JobStatusCompleted {
		t.Errorf("expected COMPLETED, got %s", got.Status)
	}
	if got.ExecutedAt == nil || got.ExecutedAt.IsZero() {
		t.Error("expected non-zero ExecutedAt")
	}
}

func TestStore_Threads(t *testing.T) {
	s := NewStore()
	ctx := context.Background()

	// Create flow first (needed for thread)
	flow := &store.FlowRecord{ID: "tf-1", InstanceID: "c-1", ElementID: "s1", ElementType: bpmn.ElementTypeStartEvent, ThreadID: 1}
	if err := s.CreateFlow(ctx, flow); err != nil {
		t.Fatalf("create flow: %v", err)
	}

	thread := &store.ThreadRecord{
		InstanceID:  "c-1",
		ThreadIndex: 1,
		ParentIndex: nil,
		FlowID:      "tf-1",
	}
	if err := s.CreateThread(ctx, thread); err != nil {
		t.Fatalf("create thread: %v", err)
	}

	threads, err := s.GetThreadsByInstance(ctx, "c-1")
	if err != nil {
		t.Fatalf("get threads: %v", err)
	}
	if len(threads) != 1 {
		t.Fatalf("expected 1 thread, got %d", len(threads))
	}
	if threads[0].ThreadIndex != 1 {
		t.Errorf("expected ThreadIndex=1, got %d", threads[0].ThreadIndex)
	}

	// UpdateThread
	parentIdx := 0
	threads[0].ParentIndex = &parentIdx
	if err := s.UpdateThread(ctx, threads[0]); err != nil {
		t.Fatalf("update thread: %v", err)
	}

	// CloseThread
	if err := s.CloseThread(ctx, "c-1", 1); err != nil {
		t.Fatalf("close thread: %v", err)
	}
}

func TestStore_DeadLetters(t *testing.T) {
	s := NewStore()
	ctx := context.Background()

	dl := &store.DeadLetterRecord{
		ID:           "dl-1",
		InstanceID:   "c-1",
		JobID:        "j-1",
		Type:         "test",
		Payload:      map[string]any{"err": "fail"},
		ErrorMessage: "something went wrong",
		RetryCount:   3,
	}
	if err := s.CreateDeadLetter(ctx, dl); err != nil {
		t.Fatalf("create dead letter: %v", err)
	}

	got, err := s.GetDeadLetter(ctx, "dl-1")
	if err != nil {
		t.Fatalf("get dead letter: %v", err)
	}
	if got.ErrorMessage != "something went wrong" {
		t.Errorf("expected 'something went wrong', got %s", got.ErrorMessage)
	}
	if got.Payload["err"] != "fail" {
		t.Errorf("expected payload err=fail, got %v", got.Payload["err"])
	}

	byInstance, err := s.GetDeadLetters(ctx, "c-1")
	if err != nil {
		t.Fatalf("get dead letters by instance: %v", err)
	}
	if len(byInstance) != 1 {
		t.Errorf("expected 1 dead letter, got %d", len(byInstance))
	}

	list, err := s.ListDeadLetters(ctx, 10)
	if err != nil {
		t.Fatalf("list dead letters: %v", err)
	}
	if len(list) < 1 {
		t.Errorf("expected at least 1 dead letter")
	}
}

func TestStore_ListProcesses(t *testing.T) {
	s := NewStore()
	ctx := context.Background()

	s.SaveProcess(ctx, &bpmn.Process{ID: "lp-1", Name: "LP 1", Elements: make(map[string]bpmn.Element), Flows: make(map[string]bpmn.Flow)})
	s.SaveProcess(ctx, &bpmn.Process{ID: "lp-2", Name: "LP 2", Elements: make(map[string]bpmn.Element), Flows: make(map[string]bpmn.Flow)})

	procs, err := s.ListProcesses(ctx)
	if err != nil {
		t.Fatalf("list processes: %v", err)
	}
	if len(procs) < 2 {
		t.Errorf("expected at least 2 processes, got %d", len(procs))
	}
}

func TestStore_ListInstancesWithFilter(t *testing.T) {
	s := NewStore()
	ctx := context.Background()

	s.CreateInstance(ctx, &store.InstanceRecord{ID: "li-1", Status: store.InstanceStatusCreated})
	s.CreateInstance(ctx, &store.InstanceRecord{ID: "li-2", Status: store.InstanceStatusInProgress})

	all, err := s.ListInstances(ctx, "")
	if err != nil {
		t.Fatalf("list all instances: %v", err)
	}
	if len(all) < 2 {
		t.Errorf("expected at least 2 instances, got %d", len(all))
	}

	created, err := s.ListInstances(ctx, store.InstanceStatusCreated)
	if err != nil {
		t.Fatalf("list created instances: %v", err)
	}
	for _, inst := range created {
		if inst.Status != store.InstanceStatusCreated {
			t.Errorf("expected CREATED, got %s", inst.Status)
		}
	}
}
