package process

import (
	"strings"
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

func TestInstance_GetID_GetState(t *testing.T) {
	proc := &bpmn.Process{ID: "proc-1", Elements: make(map[string]bpmn.Element), Flows: make(map[string]bpmn.Flow)}
	inst := NewInstance(proc, nil)

	if inst.GetID() != inst.ID {
		t.Errorf("GetID mismatch: %s vs %s", inst.GetID(), inst.ID)
	}
	if inst.GetState() != string(StateCreated) {
		t.Errorf("GetState: got %s, want CREATED", inst.GetState())
	}
	_ = inst.Transition(StateInProgress)
	if inst.GetState() != string(StateInProgress) {
		t.Errorf("GetState after transition: got %s", inst.GetState())
	}
}

func TestInstance_TryComplete(t *testing.T) {
	proc := &bpmn.Process{ID: "proc-1", Elements: make(map[string]bpmn.Element), Flows: make(map[string]bpmn.Flow)}
	inst := NewInstance(proc, nil)
	_ = inst.Transition(StateInProgress)

	if !inst.TryComplete() {
		t.Fatal("first TryComplete should succeed")
	}
	if inst.State != StateCompleted {
		t.Errorf("expected COMPLETED, got %s", inst.State)
	}
	// Idempotent: subsequent calls report the stored result.
	if !inst.TryComplete() {
		t.Error("subsequent TryComplete should report completed")
	}
}

func TestInstance_TryComplete_Invalid(t *testing.T) {
	proc := &bpmn.Process{ID: "proc-1", Elements: make(map[string]bpmn.Element), Flows: make(map[string]bpmn.Flow)}
	inst := NewInstance(proc, nil) // CREATED -> cannot complete directly
	if inst.TryComplete() {
		t.Error("TryComplete from CREATED should fail")
	}
}

func TestInstance_NextThreadID(t *testing.T) {
	proc := &bpmn.Process{ID: "proc-1", Elements: make(map[string]bpmn.Element), Flows: make(map[string]bpmn.Flow)}
	inst := NewInstance(proc, nil)
	first := inst.NextThreadID()
	second := inst.NextThreadID()
	if second <= first {
		t.Errorf("expected increasing thread ids, got %d then %d", first, second)
	}
}

func TestIsValidTransition_UnknownState(t *testing.T) {
	if IsValidTransition(State("BOGUS"), StateInProgress) {
		t.Error("unknown current state should not allow transition")
	}
}

func TestSanitizeVariables_DepthLimit(t *testing.T) {
	deep := map[string]any{
		"l1": map[string]any{
			"l2": map[string]any{
				"l3": map[string]any{
					"l4": map[string]any{"x": "y"},
				},
			},
		},
	}
	result := sanitizeVariables(deep, 0)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestSanitizeSlice(t *testing.T) {
	input := []any{"a", 1, true, nil, []any{"nested"}, map[string]any{"k": "v"}}
	result := sanitizeSlice(input, 0)
	if len(result) != len(input) {
		t.Errorf("expected %d items, got %d", len(input), len(result))
	}
	// depth limit
	deep := sanitizeSlice([]any{[]any{[]any{[]any{[]any{"x"}}}}}, 0)
	if deep == nil {
		t.Fatal("expected non-nil")
	}
}

func TestSanitizeVariables_SizeLimit(t *testing.T) {
	big := make(map[string]any)
	big["x"] = strings.Repeat("a", 1<<20+1)
	result := sanitizeVariables(big, 0)
	// Should not panic; result may be empty due to size limit
	_ = result
}

func TestGeneratePIN(t *testing.T) {
	pin := generatePIN()
	if len(pin) != 4 {
		t.Errorf("expected 4-digit PIN, got %q", pin)
	}
}

func TestNewInstance_NilVariables(t *testing.T) {
	proc := &bpmn.Process{ID: "proc-1", Elements: make(map[string]bpmn.Element), Flows: make(map[string]bpmn.Flow)}
	inst := NewInstance(proc, nil)
	if inst.Variables == nil {
		t.Error("expected non-nil variables")
	}
	if inst.PIN == "" {
		t.Error("expected non-empty PIN")
	}
	if inst.ID == "" {
		t.Error("expected non-empty ID")
	}
}

func TestNewInstance_WithVariables(t *testing.T) {
	proc := &bpmn.Process{ID: "proc-1", Elements: make(map[string]bpmn.Element), Flows: make(map[string]bpmn.Flow)}
	inst := NewInstance(proc, map[string]any{"key": "val", "num": 42})
	if v, ok := inst.GetVariable("key"); !ok || v != "val" {
		t.Errorf("expected key=val, got %v", v)
	}
	if v, ok := inst.GetVariable("num"); !ok || v != 42 {
		t.Errorf("expected num=42, got %v", v)
	}
}

func TestInstance_ToRecord_TenantAndFields(t *testing.T) {
	proc := &bpmn.Process{ID: "proc-1", Elements: make(map[string]bpmn.Element), Flows: make(map[string]bpmn.Flow)}
	inst := NewInstance(proc, map[string]any{"k": "v"})
	inst.Title = "T"
	rec := inst.ToRecord()
	if rec.ProcessID != "proc-1" {
		t.Errorf("expected process_id proc-1, got %s", rec.ProcessID)
	}
	if rec.Variables["k"] != "v" {
		t.Errorf("unexpected variables: %v", rec.Variables)
	}
}
