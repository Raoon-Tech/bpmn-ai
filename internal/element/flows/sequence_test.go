package flows

import (
	"context"
	"testing"

	"github.com/Raoon-Tech/bpmn-ai/internal/element"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

type mockExecCtx struct {
	flow *store.FlowRecord
}

func (m *mockExecCtx) Instance() element.Instance {
	return nil
}

func (m *mockExecCtx) Flow() *store.FlowRecord {
	return m.flow
}

func (m *mockExecCtx) GetVariable(key string) (any, bool) {
	return nil, false
}

func (m *mockExecCtx) SetVariable(key string, value any) {}

func (m *mockExecCtx) Store() element.ElementStore {
	return nil
}

func (m *mockExecCtx) Element() (bpmn.Element, bool) {
	return bpmn.Element{}, true
}

func TestSequenceFlow_Execute(t *testing.T) {
	elem := bpmn.Element{
		ID:   "flow-1",
		Name: "Default Flow",
		ExtensionData: map[string]string{
			"sourceRef": "start-1",
			"targetRef": "end-1",
		},
	}
	f, err := NewSequenceFlow(elem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if f.ID() != "flow-1" {
		t.Errorf("expected ID flow-1, got %s", f.ID())
	}
	if f.Type() != bpmn.ElementTypeSequenceFlow {
		t.Errorf("expected type sequenceFlow, got %s", f.Type())
	}

	result := f.Execute(context.Background(), &mockExecCtx{
		flow: &store.FlowRecord{ElementID: "flow-1"},
	})

	if result.Action != element.ActionRoute {
		t.Errorf("expected ActionRoute, got %s", result.Action)
	}
}

func TestSequenceFlow_Fields(t *testing.T) {
	elem := bpmn.Element{
		ID:   "flow-1",
		Name: "Cond Flow",
		ExtensionData: map[string]string{
			"sourceRef":           "gw-1",
			"targetRef":           "task-1",
			"conditionExpression": "${amount > 100}",
			"isDefault":           "true",
		},
	}
	raw, _ := NewSequenceFlow(elem)
	f := raw.(*SequenceFlow)

	if f.SourceRef() != "gw-1" {
		t.Errorf("expected sourceRef 'gw-1', got %s", f.SourceRef())
	}
	if f.TargetRef() != "task-1" {
		t.Errorf("expected targetRef 'task-1', got %s", f.TargetRef())
	}
	if f.Condition() != "${amount > 100}" {
		t.Errorf("expected condition '${amount > 100}', got %s", f.Condition())
	}
	if !f.IsDefault() {
		t.Error("expected IsDefault to be true")
	}
}
