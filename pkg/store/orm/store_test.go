package orm

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

func openTestDB(t *testing.T, path string) *gorm.DB {
	t.Helper()
	db, err := Open(Config{Driver: "sqlite", File: path, LogLevel: "silent"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	return New(openTestDB(t, filepath.Join(t.TempDir(), "store.db")))
}

func TestStore_Processes(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	proc := &bpmn.Process{ID: "p1", Name: "First", Version: 1, CreatedAt: time.Now()}
	if err := st.SaveProcess(ctx, proc); err != nil {
		t.Fatalf("save process: %v", err)
	}

	proc.Name = "Updated"
	proc.Version = 2
	if err := st.SaveProcess(ctx, proc); err != nil {
		t.Fatalf("upsert process: %v", err)
	}

	got, err := st.GetProcess(ctx, "p1")
	if err != nil {
		t.Fatalf("get process: %v", err)
	}
	if got.Name != "Updated" || got.Version != 2 {
		t.Fatalf("unexpected process: name=%q version=%d", got.Name, got.Version)
	}

	list, err := st.ListProcesses(ctx)
	if err != nil {
		t.Fatalf("list processes: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 process, got %d", len(list))
	}

	if _, err := st.GetProcess(ctx, "missing"); err == nil {
		t.Fatal("expected not found error")
	}
}

func TestStore_Instances(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	inst := &store.InstanceRecord{
		ID:          "i1",
		ProcessID:   "p1",
		Title:       "Case",
		Status:      store.InstanceStatusCreated,
		Variables:   map[string]any{"amount": float64(10)},
		CurrentUser: "ana",
	}
	if err := st.CreateInstance(ctx, inst); err != nil {
		t.Fatalf("create instance: %v", err)
	}
	if inst.StartedAt.IsZero() || inst.UpdatedAt.IsZero() {
		t.Fatal("expected timestamps to be set")
	}

	got, err := st.GetInstance(ctx, "i1")
	if err != nil {
		t.Fatalf("get instance: %v", err)
	}
	if got.Variables["amount"] != float64(10) {
		t.Fatalf("unexpected variables: %#v", got.Variables)
	}

	got.Status = store.InstanceStatusWaiting
	got.Variables["approved"] = true
	if err := st.UpdateInstance(ctx, got); err != nil {
		t.Fatalf("update instance: %v", err)
	}

	byStatus, err := st.ListInstances(ctx, store.InstanceStatusWaiting)
	if err != nil {
		t.Fatalf("list instances: %v", err)
	}
	if len(byStatus) != 1 || byStatus[0].Variables["approved"] != true {
		t.Fatalf("unexpected list result: %#v", byStatus)
	}

	all, err := st.ListInstances(ctx, "")
	if err != nil {
		t.Fatalf("list all instances: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(all))
	}
}

func TestStore_Flows(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	flow := &store.FlowRecord{
		InstanceID:  "i1",
		ElementID:   "task-1",
		ElementType: bpmn.ElementTypeUserTask,
		ThreadID:    1,
		Status:      store.FlowStatusActive,
	}
	if err := st.CreateFlow(ctx, flow); err != nil {
		t.Fatalf("create flow: %v", err)
	}
	if flow.ID == "" || flow.StartedAt == nil {
		t.Fatal("expected generated id and start time")
	}

	active, err := st.GetActiveFlowsByThread(ctx, "i1", 1)
	if err != nil {
		t.Fatalf("get active flows: %v", err)
	}
	if len(active) != 1 || active[0].ElementType != bpmn.ElementTypeUserTask {
		t.Fatalf("unexpected active flows: %#v", active)
	}

	flow.Status = store.FlowStatusCompleted
	finished := time.Now()
	flow.FinishedAt = &finished
	if err := st.UpdateFlow(ctx, flow); err != nil {
		t.Fatalf("update flow: %v", err)
	}

	active, err = st.GetActiveFlowsByThread(ctx, "i1", 1)
	if err != nil {
		t.Fatalf("get active flows after update: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("expected no active flows, got %d", len(active))
	}

	all, err := st.GetFlowsByInstance(ctx, "i1")
	if err != nil {
		t.Fatalf("get flows by instance: %v", err)
	}
	if len(all) != 1 || all[0].Status != store.FlowStatusCompleted {
		t.Fatalf("unexpected flows: %#v", all)
	}
}

func TestStore_Threads(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	thread := &store.ThreadRecord{InstanceID: "i1", ThreadIndex: 1, FlowID: "f1", Status: "ACTIVE"}
	if err := st.CreateThread(ctx, thread); err != nil {
		t.Fatalf("create thread: %v", err)
	}
	if thread.ID == 0 {
		t.Fatal("expected thread id to be populated")
	}

	thread.FlowID = "f2"
	if err := st.CreateThread(ctx, thread); err != nil {
		t.Fatalf("upsert thread: %v", err)
	}

	threads, err := st.GetThreadsByInstance(ctx, "i1")
	if err != nil {
		t.Fatalf("get threads: %v", err)
	}
	if len(threads) != 1 || threads[0].FlowID != "f2" {
		t.Fatalf("unexpected threads: %#v", threads)
	}

	if err := st.CloseThread(ctx, "i1", 1); err != nil {
		t.Fatalf("close thread: %v", err)
	}
	threads, err = st.GetThreadsByInstance(ctx, "i1")
	if err != nil {
		t.Fatalf("get threads after close: %v", err)
	}
	if threads[0].Status != "CLOSED" {
		t.Fatalf("expected CLOSED, got %q", threads[0].Status)
	}
}

func TestStore_Jobs(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	job := &store.JobRecord{
		InstanceID: "i1",
		FlowID:     "f1",
		Type:       "timer",
		Payload:    map[string]any{"delay": "1s"},
		Status:     store.JobStatusPending,
		MaxRetries: 3,
	}
	if err := st.CreateJob(ctx, job); err != nil {
		t.Fatalf("create job: %v", err)
	}
	if job.ID == "" {
		t.Fatal("expected generated job id")
	}

	pending, err := st.GetPendingJobs(ctx, 10)
	if err != nil {
		t.Fatalf("get pending jobs: %v", err)
	}
	if len(pending) != 1 || pending[0].Payload["delay"] != "1s" {
		t.Fatalf("unexpected pending jobs: %#v", pending)
	}

	job.Status = store.JobStatusCompleted
	executed := time.Now()
	job.ExecutedAt = &executed
	if err := st.UpdateJob(ctx, job); err != nil {
		t.Fatalf("update job: %v", err)
	}

	pending, err = st.GetPendingJobs(ctx, 10)
	if err != nil {
		t.Fatalf("get pending jobs after update: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected no pending jobs, got %d", len(pending))
	}

	got, err := st.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if got.Status != store.JobStatusCompleted {
		t.Fatalf("unexpected status %q", got.Status)
	}

	farFuture := &store.JobRecord{
		InstanceID:  "i1",
		Type:        "timer",
		Status:      store.JobStatusPending,
		ScheduledAt: time.Now().Add(time.Hour),
	}
	if err := st.CreateJob(ctx, farFuture); err != nil {
		t.Fatalf("create scheduled job: %v", err)
	}
	pending, err = st.GetPendingJobs(ctx, 10)
	if err != nil {
		t.Fatalf("get pending jobs with future job: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("future job must not be pending yet, got %d", len(pending))
	}
}

func TestStore_DeadLettersAndLogs(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	dl := &store.DeadLetterRecord{
		JobID:        "j1",
		InstanceID:   "i1",
		Type:         "timer",
		Payload:      map[string]any{"attempts": float64(3)},
		ErrorMessage: "boom",
		RetryCount:   3,
	}
	if err := st.CreateDeadLetter(ctx, dl); err != nil {
		t.Fatalf("create dead letter: %v", err)
	}
	if dl.ID == "" {
		t.Fatal("expected generated dead letter id")
	}

	list, err := st.GetDeadLetters(ctx, "i1")
	if err != nil {
		t.Fatalf("get dead letters: %v", err)
	}
	if len(list) != 1 || list[0].ErrorMessage != "boom" {
		t.Fatalf("unexpected dead letters: %#v", list)
	}

	recent, err := st.ListDeadLetters(ctx, 10)
	if err != nil {
		t.Fatalf("list dead letters: %v", err)
	}
	if len(recent) != 1 {
		t.Fatalf("expected 1 dead letter, got %d", len(recent))
	}

	if _, err := st.GetDeadLetter(ctx, dl.ID); err != nil {
		t.Fatalf("get dead letter: %v", err)
	}

	if err := st.LogExecution(ctx, &store.ExecutionLogEntry{
		InstanceID:  "i1",
		ElementID:   "task-1",
		ElementType: bpmn.ElementTypeUserTask,
		Action:      "complete",
		DurationMs:  12,
	}); err != nil {
		t.Fatalf("log execution: %v", err)
	}
	logs, err := st.GetExecutionLog(ctx, "i1")
	if err != nil {
		t.Fatalf("get execution log: %v", err)
	}
	if len(logs) != 1 || logs[0].Action != "complete" {
		t.Fatalf("unexpected execution log: %#v", logs)
	}

	if err := st.LogAICall(ctx, &store.AIAuditLogEntry{
		InstanceID: "i1",
		ElementID:  "ai-1",
		Model:      "gpt-4o",
		InputText:  "hi",
		OutputText: "hello",
		TokensIn:   1,
		TokensOut:  2,
		Success:    true,
	}); err != nil {
		t.Fatalf("log ai call: %v", err)
	}
	var count int64
	if err := st.db.Model(&AIAuditLogModel{}).Count(&count).Error; err != nil {
		t.Fatalf("count ai audit: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 ai audit row, got %d", count)
	}
}

func TestStore_PersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "persist.db")
	ctx := context.Background()

	db1 := openTestDB(t, path)
	st1 := New(db1)
	inst := &store.InstanceRecord{
		ID:        "persisted",
		ProcessID: "p1",
		Status:    store.InstanceStatusInProgress,
		Variables: map[string]any{"step": float64(2)},
	}
	if err := st1.CreateInstance(ctx, inst); err != nil {
		t.Fatalf("create instance: %v", err)
	}
	sqlDB, _ := db1.DB()
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close first connection: %v", err)
	}

	db2 := openTestDB(t, path)
	st2 := New(db2)
	got, err := st2.GetInstance(ctx, "persisted")
	if err != nil {
		t.Fatalf("get instance after reopen: %v", err)
	}
	if got.Variables["step"] != float64(2) {
		t.Fatalf("unexpected variables after reopen: %#v", got.Variables)
	}
}
