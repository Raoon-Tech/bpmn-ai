package process

import (
	"testing"

	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

func TestIsValidTransition(t *testing.T) {
	tests := []struct {
		from    State
		to      State
		want    bool
	}{
		{StateCreated, StateInProgress, true},
		{StateCreated, StateError, true},
		{StateCreated, StateCompleted, false},
		{StateInProgress, StateCompleted, true},
		{StateInProgress, StateWaiting, true},
		{StateInProgress, StateTerminated, true},
		{StateCompleted, StateInProgress, false},
		{StateTerminated, StateInProgress, false},
		{StateError, StateInProgress, true},
		{StateWaiting, StateInProgress, true},
		{StateSuspended, StateInProgress, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.from)+"->"+string(tt.to), func(t *testing.T) {
			got := IsValidTransition(tt.from, tt.to)
			if got != tt.want {
				t.Errorf("IsValidTransition(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestInstance_Transition(t *testing.T) {
	proc := &bpmn.Process{ID: "proc-1", Elements: make(map[string]bpmn.Element), Flows: make(map[string]bpmn.Flow)}
	inst := NewInstance(proc, nil)

	if inst.State != StateCreated {
		t.Fatalf("expected state CREATED, got %s", inst.State)
	}

	if err := inst.Transition(StateInProgress); err != nil {
		t.Fatalf("transition to IN_PROGRESS: %v", err)
	}
	if inst.State != StateInProgress {
		t.Errorf("expected state IN_PROGRESS, got %s", inst.State)
	}

	if err := inst.Transition(StateCompleted); err != nil {
		t.Fatalf("transition to COMPLETED: %v", err)
	}
	if inst.State != StateCompleted {
		t.Errorf("expected state COMPLETED, got %s", inst.State)
	}
	if inst.FinishedAt == nil {
		t.Error("expected FinishedAt to be set")
	}
}

func TestInstance_Transition_Invalid(t *testing.T) {
	proc := &bpmn.Process{ID: "proc-1", Elements: make(map[string]bpmn.Element), Flows: make(map[string]bpmn.Flow)}
	inst := NewInstance(proc, nil)

	err := inst.Transition(StateCompleted)
	if err == nil {
		t.Fatal("expected error for invalid transition CREATED -> COMPLETED")
	}

	invErr, ok := err.(*InvalidTransitionError)
	if !ok {
		t.Fatalf("expected InvalidTransitionError, got %T", err)
	}
	if invErr.From != StateCreated {
		t.Errorf("expected From CREATED, got %s", invErr.From)
	}
	if invErr.To != StateCompleted {
		t.Errorf("expected To COMPLETED, got %s", invErr.To)
	}
}

func TestInstance_Variables(t *testing.T) {
	proc := &bpmn.Process{ID: "proc-1", Elements: make(map[string]bpmn.Element), Flows: make(map[string]bpmn.Flow)}
	inst := NewInstance(proc, map[string]any{"initial": "value"})

	val, ok := inst.GetVariable("initial")
	if !ok {
		t.Fatal("expected variable initial to exist")
	}
	if val != "value" {
		t.Errorf("expected value, got %v", val)
	}

	_, ok = inst.GetVariable("nonexistent")
	if ok {
		t.Error("expected nonexistent variable to return false")
	}

	inst.SetVariable("new", 42)
	val, ok = inst.GetVariable("new")
	if !ok || val != 42 {
		t.Errorf("expected new variable to be 42, got %v", val)
	}
}

func TestInstance_ToRecord(t *testing.T) {
	proc := &bpmn.Process{ID: "proc-1", Elements: make(map[string]bpmn.Element), Flows: make(map[string]bpmn.Flow)}
	inst := NewInstance(proc, map[string]any{"key": "val"})
	inst.Title = "Test Case"

	rec := inst.ToRecord()
	if rec.ID != inst.ID {
		t.Errorf("expected record ID %s, got %s", inst.ID, rec.ID)
	}
	if rec.Title != "Test Case" {
		t.Errorf("expected record title Test Case, got %s", rec.Title)
	}
	if rec.Status != store.InstanceStatusCreated {
		t.Errorf("expected record status CREATED, got %s", rec.Status)
	}
}

func TestNewThread(t *testing.T) {
	parent := 1
	thread := NewThread("case-1", 2, &parent, "flow-1")

	if thread.InstanceID != "case-1" {
		t.Errorf("expected instanceID case-1, got %s", thread.InstanceID)
	}
	if thread.ThreadIndex != 2 {
		t.Errorf("expected threadIndex 2, got %d", thread.ThreadIndex)
	}
	if *thread.ParentIndex != 1 {
		t.Errorf("expected parentIndex 1, got %d", *thread.ParentIndex)
	}
	if thread.CurrentFlowID != "flow-1" {
		t.Errorf("expected currentFlowID flow-1, got %s", thread.CurrentFlowID)
	}
	if thread.State != "ACTIVE" {
		t.Errorf("expected state ACTIVE, got %s", thread.State)
	}
}

func TestInvalidTransitionError_Error(t *testing.T) {
	err := &InvalidTransitionError{From: StateCreated, To: StateCompleted}
	expected := "invalid state transition: CREATED -> COMPLETED"
	if err.Error() != expected {
		t.Errorf("expected error message %q, got %q", expected, err.Error())
	}
}
