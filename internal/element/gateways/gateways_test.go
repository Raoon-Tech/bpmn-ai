package gateways

import (
	"context"
	"testing"

	"github.com/Raoon-Tech/bpmn-ai/internal/element"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store/memory"
)

type mockInstance struct {
	id string
}

func (m *mockInstance) GetID() string                     { return m.id }
func (m *mockInstance) GetState() string                   { return "ACTIVE" }
func (m *mockInstance) GetVariable(key string) (any, bool) { return nil, false }
func (m *mockInstance) SetVariable(key string, value any)  {}

type mockExecCtx struct {
	flow      *store.FlowRecord
	elem      bpmn.Element
	instance  element.Instance
	stored    element.ElementStore
	variables map[string]any
}

func (m *mockExecCtx) Instance() element.Instance {
	return m.instance
}

func (m *mockExecCtx) Flow() *store.FlowRecord {
	return m.flow
}

func (m *mockExecCtx) GetVariable(key string) (any, bool) {
	v, ok := m.variables[key]
	return v, ok
}

func (m *mockExecCtx) SetVariable(key string, value any) {
	if m.variables == nil {
		m.variables = make(map[string]any)
	}
	m.variables[key] = value
}

func (m *mockExecCtx) Store() element.ElementStore {
	return m.stored
}

func (m *mockExecCtx) Element() (bpmn.Element, bool) {
	return m.elem, true
}

func TestExclusiveGateway_Execute(t *testing.T) {
	elem := bpmn.Element{
		ID:            "gw-1",
		Name:          "Exclusive",
		GatewayType:   bpmn.GatewayTypeExclusive,
		DefaultFlowID: "flow-default",
		OutgoingFlows: []string{"flow-a", "flow-b", "flow-default"},
	}
	rawG, err := NewExclusiveGateway(elem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	g := rawG.(*ExclusiveGateway)

	if g.ID() != "gw-1" {
		t.Errorf("expected ID gw-1, got %s", g.ID())
	}
	if g.Type() != bpmn.ElementTypeExclusiveGateway {
		t.Errorf("expected type exclusiveGateway, got %s", g.Type())
	}
	if g.DefaultFlowID() != "flow-default" {
		t.Errorf("expected default flow flow-default, got %s", g.DefaultFlowID())
	}

	result := g.Execute(context.Background(), &mockExecCtx{
		flow: &store.FlowRecord{ElementID: "gw-1"},
		elem: bpmn.Element{OutgoingFlows: []string{"flow-a", "flow-b", "flow-default"}},
	})

	if result.Action != element.ActionRoute {
		t.Errorf("expected ActionRoute, got %s", result.Action)
	}
}

func TestExclusiveGateway_ConditionMatch(t *testing.T) {
	g, _ := NewExclusiveGateway(bpmn.Element{
		ID:            "gw-1",
		DefaultFlowID: "flow-default",
		Conditions:    map[string]string{"flow-a": "amount > 500"},
		OutgoingFlows: []string{"flow-a", "flow-b"},
	})

	result := g.Execute(context.Background(), &mockExecCtx{
		flow: &store.FlowRecord{ElementID: "gw-1"},
		elem: bpmn.Element{OutgoingFlows: []string{"flow-a", "flow-b"}},
		variables: map[string]any{"amount": 1000},
	})

	if result.Action != element.ActionRoute {
		t.Errorf("expected ActionRoute, got %s", result.Action)
	}
	if len(result.FlowFilters) != 1 || result.FlowFilters[0] != "flow-a" {
		t.Errorf("expected flow-a to match, got %v", result.FlowFilters)
	}
}

func TestExclusiveGateway_ConditionNoMatchUsesDefault(t *testing.T) {
	g, _ := NewExclusiveGateway(bpmn.Element{
		ID:            "gw-1",
		DefaultFlowID: "flow-b",
		Conditions:    map[string]string{"flow-a": "amount > 500"},
		OutgoingFlows: []string{"flow-a", "flow-b"},
	})

	result := g.Execute(context.Background(), &mockExecCtx{
		flow: &store.FlowRecord{ElementID: "gw-1"},
		elem: bpmn.Element{OutgoingFlows: []string{"flow-a", "flow-b"}},
		variables: map[string]any{"amount": 100},
	})

	if result.Action != element.ActionRoute {
		t.Errorf("expected ActionRoute, got %s", result.Action)
	}
	if len(result.FlowFilters) != 1 || result.FlowFilters[0] != "flow-b" {
		t.Errorf("expected flow-b (default) to be selected, got %v", result.FlowFilters)
	}
}

func TestExclusiveGateway_ConditionNoMatchAndNoDefaultUsesFirst(t *testing.T) {
	g, _ := NewExclusiveGateway(bpmn.Element{
		ID:            "gw-1",
		Conditions:    map[string]string{"flow-a": "amount > 500"},
		OutgoingFlows: []string{"flow-a", "flow-b"},
	})

	result := g.Execute(context.Background(), &mockExecCtx{
		flow: &store.FlowRecord{ElementID: "gw-1"},
		elem: bpmn.Element{OutgoingFlows: []string{"flow-a", "flow-b"}},
		variables: map[string]any{"amount": 100},
	})

	if result.Action != element.ActionRoute {
		t.Errorf("expected ActionRoute, got %s", result.Action)
	}
	if len(result.FlowFilters) != 1 || result.FlowFilters[0] != "flow-a" {
		t.Errorf("expected first flow flow-a to be selected, got %v", result.FlowFilters)
	}
}

func TestExclusiveGateway_ConditionBooleanVar(t *testing.T) {
	g, _ := NewExclusiveGateway(bpmn.Element{
		ID:            "gw-1",
		DefaultFlowID: "flow-b",
		Conditions:    map[string]string{"flow-a": "approved == true"},
		OutgoingFlows: []string{"flow-a", "flow-b"},
	})

	result := g.Execute(context.Background(), &mockExecCtx{
		flow: &store.FlowRecord{ElementID: "gw-1"},
		elem: bpmn.Element{OutgoingFlows: []string{"flow-a", "flow-b"}},
		variables: map[string]any{"approved": true},
	})

	if len(result.FlowFilters) != 1 || result.FlowFilters[0] != "flow-a" {
		t.Errorf("expected flow-a when approved=true, got %v", result.FlowFilters)
	}
}

func TestExclusiveGateway_ConditionInvalidExpr(t *testing.T) {
	g, _ := NewExclusiveGateway(bpmn.Element{
		ID:            "gw-1",
		DefaultFlowID: "flow-b",
		Conditions:    map[string]string{"flow-a": "invalid {{{ expr"},
		OutgoingFlows: []string{"flow-a", "flow-b"},
	})

	result := g.Execute(context.Background(), &mockExecCtx{
		flow: &store.FlowRecord{ElementID: "gw-1"},
		elem: bpmn.Element{OutgoingFlows: []string{"flow-a", "flow-b"}},
		variables: map[string]any{"amount": 100},
	})

	if len(result.FlowFilters) != 1 || result.FlowFilters[0] != "flow-b" {
		t.Errorf("expected default flow-b on invalid expression, got %v", result.FlowFilters)
	}
}

func TestExclusiveGateway_ConditionMissingVariable(t *testing.T) {
	g, _ := NewExclusiveGateway(bpmn.Element{
		ID:            "gw-1",
		DefaultFlowID: "flow-b",
		Conditions:    map[string]string{"flow-a": "nonexistent > 0"},
		OutgoingFlows: []string{"flow-a", "flow-b"},
	})

	result := g.Execute(context.Background(), &mockExecCtx{
		flow: &store.FlowRecord{ElementID: "gw-1"},
		elem: bpmn.Element{OutgoingFlows: []string{"flow-a", "flow-b"}},
		variables: map[string]any{"amount": 100},
	})

	if len(result.FlowFilters) != 1 || result.FlowFilters[0] != "flow-b" {
		t.Errorf("expected default flow-b when variable missing, got %v", result.FlowFilters)
	}
}

func TestExclusiveGateway_CopyConditionsFromElement(t *testing.T) {
	rawG, _ := NewExclusiveGateway(bpmn.Element{
		ID:            "gw-1",
		DefaultFlowID: "flow-b",
		Conditions:    map[string]string{"flow-a": "amount > 0"},
	})
	eg := rawG.(*ExclusiveGateway)

	if len(eg.Conditions()) != 1 || eg.Conditions()["flow-a"] != "amount > 0" {
		t.Errorf("expected conditions to be copied from element, got %v", eg.Conditions())
	}
}

func TestParallelGateway_Execute(t *testing.T) {
	elem := bpmn.Element{
		ID:          "gw-p-1",
		Name:        "Parallel",
		GatewayType: bpmn.GatewayTypeParallel,
	}
	rawG, err := NewParallelGateway(elem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	g := rawG.(*ParallelGateway)

	if g.ID() != "gw-p-1" {
		t.Errorf("expected ID gw-p-1, got %s", g.ID())
	}
	if g.Type() != bpmn.ElementTypeParallelGateway {
		t.Errorf("expected type parallelGateway, got %s", g.Type())
	}
	if g.GatewayType() != bpmn.GatewayTypeParallel {
		t.Errorf("expected GatewayTypeParallel, got %s", g.GatewayType())
	}
}

func TestParallelGateway_Diverging(t *testing.T) {
	g, _ := NewParallelGateway(bpmn.Element{
		ID: "gw-p-1", GatewayType: bpmn.GatewayTypeParallel,
	})

	result := g.Execute(context.Background(), &mockExecCtx{
		flow: &store.FlowRecord{ElementID: "gw-p-1"},
		elem: bpmn.Element{
			OutgoingFlows: []string{"flow-a", "flow-b"},
			IncomingFlows: []string{"flow-in"},
		},
	})

	if result.Action != element.ActionRoute {
		t.Errorf("diverging parallel: expected ActionRoute, got %s", result.Action)
	}
	if len(result.FlowFilters) != 0 {
		t.Errorf("diverging parallel: expected no filters, got %v", result.FlowFilters)
	}
}

func TestInclusiveGateway_Execute(t *testing.T) {
	tests := []struct {
		name        string
		outgoing    []string
		conditions  map[string]string
		defaultFlow string
		wantMin     int
	}{
		{
			name:     "multiple outgoing with no conditions",
			outgoing: []string{"flow-a", "flow-b"},
			wantMin:  2,
		},
		{
			name:        "uses default when no match",
			outgoing:    []string{"flow-a", "flow-b"},
			defaultFlow: "flow-b",
			wantMin:     1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			elem := bpmn.Element{
				ID:            "gw-inc-1",
				GatewayType:   bpmn.GatewayTypeInclusive,
				DefaultFlowID: tt.defaultFlow,
				Conditions:    tt.conditions,
				OutgoingFlows: tt.outgoing,
			}
			g, err := NewInclusiveGateway(elem)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if g.Type() != bpmn.ElementTypeInclusiveGateway {
				t.Errorf("expected type inclusiveGateway, got %s", g.Type())
			}

			result := g.Execute(context.Background(), &mockExecCtx{
				flow: &store.FlowRecord{ElementID: "gw-inc-1"},
				elem: bpmn.Element{
					OutgoingFlows: tt.outgoing,
				},
				variables: make(map[string]any),
			})

			if result.Action != element.ActionRoute {
				t.Errorf("expected ActionRoute, got %s", result.Action)
			}
			if len(result.FlowFilters) < tt.wantMin {
				t.Errorf("expected at least %d flow filters, got %d", tt.wantMin, len(result.FlowFilters))
			}
		})
	}
}

func TestInclusiveGateway_ConditionEvaluation(t *testing.T) {
	g, _ := NewInclusiveGateway(bpmn.Element{
		ID:            "gw-inc-1",
		DefaultFlowID: "flow-c",
		Conditions:    map[string]string{"flow-a": "amount > 500", "flow-b": "priority == \"high\""},
		OutgoingFlows: []string{"flow-a", "flow-b", "flow-c"},
	})

	result := g.Execute(context.Background(), &mockExecCtx{
		flow: &store.FlowRecord{ElementID: "gw-inc-1"},
		elem: bpmn.Element{OutgoingFlows: []string{"flow-a", "flow-b", "flow-c"}},
		variables: map[string]any{"amount": 1000, "priority": "high"},
	})

	if result.Action != element.ActionRoute {
		t.Errorf("expected ActionRoute, got %s", result.Action)
	}
	// flow-c has no condition so it's always included; flow-a and flow-b match
	if len(result.FlowFilters) != 3 {
		t.Errorf("expected 3 matching flows (2 conditions + 1 unconditional), got %v", result.FlowFilters)
	}
}

func TestInclusiveGateway_ConditionPartialMatch(t *testing.T) {
	g, _ := NewInclusiveGateway(bpmn.Element{
		ID:            "gw-inc-1",
		DefaultFlowID: "flow-c",
		Conditions:    map[string]string{"flow-a": "amount > 500", "flow-b": "priority == \"high\""},
		OutgoingFlows: []string{"flow-a", "flow-b", "flow-c"},
	})

	result := g.Execute(context.Background(), &mockExecCtx{
		flow: &store.FlowRecord{ElementID: "gw-inc-1"},
		elem: bpmn.Element{OutgoingFlows: []string{"flow-a", "flow-b", "flow-c"}},
		variables: map[string]any{"amount": 100, "priority": "high"},
	})

	if result.Action != element.ActionRoute {
		t.Errorf("expected ActionRoute, got %s", result.Action)
	}
	// flow-a doesn't match (amount=100 not > 500), flow-b matches, flow-c has no condition
	if len(result.FlowFilters) != 2 {
		t.Errorf("expected 2 matching flows (flow-b + unconditional flow-c), got %v", result.FlowFilters)
	}
}

func TestEventBasedGateway_Execute(t *testing.T) {
	tests := []struct {
		name     string
		outgoing []string
		wantAct  element.Action
	}{
		{
			name:     "single outgoing routes directly",
			outgoing: []string{"flow-a"},
			wantAct:  element.ActionRoute,
		},
		{
			name:     "multiple outgoing waits for event",
			outgoing: []string{"flow-a", "flow-b"},
			wantAct:  element.ActionWait,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			elem := bpmn.Element{
				ID:          "gw-eb-1",
				GatewayType: bpmn.GatewayTypeEventBased,
			}
			g, err := NewEventBasedGateway(elem)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if g.Type() != bpmn.ElementTypeEventBasedGateway {
				t.Errorf("expected type eventBasedGateway, got %s", g.Type())
			}

			result := g.Execute(context.Background(), &mockExecCtx{
				flow: &store.FlowRecord{ElementID: "gw-eb-1"},
				elem: bpmn.Element{OutgoingFlows: tt.outgoing},
			})

			if result.Action != tt.wantAct {
				t.Errorf("expected %s, got %s", tt.wantAct, result.Action)
			}
		})
	}
}

func TestExclusiveGateway_NoElement(t *testing.T) {
	g, _ := NewExclusiveGateway(bpmn.Element{ID: "gw-1"})

	result := g.Execute(context.Background(), &mockExecCtx{
		flow: &store.FlowRecord{ElementID: "gw-1"},
		elem: bpmn.Element{OutgoingFlows: []string{"flow-a"}},
	})

	if result.Action != element.ActionRoute {
		t.Errorf("expected ActionRoute, got %s", result.Action)
	}
}

func TestInclusiveGateway_NoElement(t *testing.T) {
	g, _ := NewInclusiveGateway(bpmn.Element{ID: "gw-inc-1"})

	result := g.Execute(context.Background(), &mockExecCtx{
		flow: &store.FlowRecord{ElementID: "gw-inc-1"},
		elem: bpmn.Element{OutgoingFlows: []string{"flow-a"}},
	})

	if result.Action != element.ActionRoute {
		t.Errorf("expected ActionRoute, got %s", result.Action)
	}
}

func TestEventBasedGateway_NoElement(t *testing.T) {
	g, _ := NewEventBasedGateway(bpmn.Element{ID: "gw-eb-1"})

	result := g.Execute(context.Background(), &mockExecCtx{
		flow: &store.FlowRecord{ElementID: "gw-eb-1"},
		elem: bpmn.Element{OutgoingFlows: []string{"flow-a"}},
	})

	if result.Action != element.ActionRoute {
		t.Errorf("expected ActionRoute, got %s", result.Action)
	}
}

func TestParallelGateway_Converging_Wait(t *testing.T) {
	st := memory.NewStore()
	instID := "inst-1"
	_ = st.CreateFlow(context.Background(), &store.FlowRecord{
		InstanceID:  instID,
		ElementID:   "gw-p-1",
		ElementType: bpmn.ElementTypeParallelGateway,
		Status:      store.FlowStatusActive,
	})

	g, _ := NewParallelGateway(bpmn.Element{
		ID: "gw-p-1", GatewayType: bpmn.GatewayTypeParallel,
	})

	result := g.Execute(context.Background(), &mockExecCtx{
		flow:   &store.FlowRecord{InstanceID: instID, ElementID: "gw-p-1"},
		elem:   bpmn.Element{IncomingFlows: []string{"flow-a", "flow-b"}, OutgoingFlows: []string{"flow-out"}},
		stored: st,
	})

	// Only 1 of 2 incoming flows have reached the gateway, should wait
	if result.Action != element.ActionWait {
		t.Errorf("expected ActionWait, got %s", result.Action)
	}
}

func TestParallelGateway_Converging_Proceed(t *testing.T) {
	st := memory.NewStore()
	instID := "inst-1"
	// Pre-create flow records for BOTH incoming flows reaching the gateway
	for i := 0; i < 2; i++ {
		_ = st.CreateFlow(context.Background(), &store.FlowRecord{
			InstanceID:  instID,
			ElementID:   "gw-p-1",
			ElementType: bpmn.ElementTypeParallelGateway,
			Status:      store.FlowStatusActive,
		})
	}

	g, _ := NewParallelGateway(bpmn.Element{
		ID: "gw-p-1", GatewayType: bpmn.GatewayTypeParallel,
	})

	result := g.Execute(context.Background(), &mockExecCtx{
		flow:   &store.FlowRecord{InstanceID: instID, ElementID: "gw-p-1"},
		elem:   bpmn.Element{IncomingFlows: []string{"flow-a", "flow-b"}, OutgoingFlows: []string{"flow-out"}},
		stored: st,
	})

	if result.Action != element.ActionRoute {
		t.Errorf("expected ActionRoute, got %s", result.Action)
	}
}

func TestParallelGateway_Converging_ErrorFlowIgnored(t *testing.T) {
	st := memory.NewStore()
	instID := "inst-1"
	// Only 1 active flow + 1 errored flow
	_ = st.CreateFlow(context.Background(), &store.FlowRecord{
		InstanceID:  instID,
		ElementID:   "gw-p-1",
		ElementType: bpmn.ElementTypeParallelGateway,
		Status:      store.FlowStatusActive,
	})
	_ = st.CreateFlow(context.Background(), &store.FlowRecord{
		InstanceID:  instID,
		ElementID:   "gw-p-1",
		ElementType: bpmn.ElementTypeParallelGateway,
		Status:      store.FlowStatusError,
	})

	g, _ := NewParallelGateway(bpmn.Element{
		ID: "gw-p-1", GatewayType: bpmn.GatewayTypeParallel,
	})

	result := g.Execute(context.Background(), &mockExecCtx{
		flow:   &store.FlowRecord{InstanceID: instID, ElementID: "gw-p-1"},
		elem:   bpmn.Element{IncomingFlows: []string{"flow-a", "flow-b"}, OutgoingFlows: []string{"flow-out"}},
		stored: st,
	})

	// Errored flow should not count toward completion, only 1 valid branch
	if result.Action != element.ActionWait {
		t.Errorf("expected ActionWait (error flow doesn't count), got %s", result.Action)
	}
}

func TestExclusiveGateway_Conditions(t *testing.T) {
	g, _ := NewExclusiveGateway(bpmn.Element{
		ID:            "gw-1",
		DefaultFlowID: "flow-default",
	})
	eg := g.(*ExclusiveGateway)
	eg.SetCondition("flow-a", "amount > 1000")

	if eg.DefaultFlowID() != "flow-default" {
		t.Errorf("expected default flow-default, got %s", eg.DefaultFlowID())
	}
	if len(eg.Conditions()) != 1 {
		t.Errorf("expected 1 condition, got %d", len(eg.Conditions()))
	}
}

func TestParallelGateway_Conditions(t *testing.T) {
	g, _ := NewParallelGateway(bpmn.Element{ID: "gw-p-1"})
	pg := g.(*ParallelGateway)

	if pg.DefaultFlowID() != "" {
		t.Errorf("expected empty default flow, got %s", pg.DefaultFlowID())
	}
	if pg.Conditions() != nil {
		t.Error("expected nil conditions")
	}
}

func TestCheckAndResolve_FirstCallWins(t *testing.T) {
	vars := map[string]any{
		"eventbased_gateway_armed":    "gw-eb",
		"eventbased_gateway_resolved": false,
		"eventbased_winning_element":  "",
	}
	execCtx := &mockExecCtx{variables: vars}

	// First call should win
	if !CheckAndResolve(execCtx, "timer-1") {
		t.Error("expected first call to return true")
	}
	if vars["eventbased_winning_element"] != "timer-1" {
		t.Errorf("expected winning_element=timer-1, got %v", vars["eventbased_winning_element"])
	}

	// Second call with different element should lose
	if CheckAndResolve(execCtx, "msg-1") {
		t.Error("expected second call to return false")
	}

	// Second call with winning element should pass
	if !CheckAndResolve(execCtx, "timer-1") {
		t.Error("expected winning element to return true")
	}
}

func TestCheckAndResolve_NoArmedGateway(t *testing.T) {
	vars := map[string]any{
		"eventbased_gateway_armed":    "",
		"eventbased_gateway_resolved": false,
		"eventbased_winning_element":  "",
	}
	execCtx := &mockExecCtx{variables: vars}

	if !CheckAndResolve(execCtx, "any-element") {
		t.Error("expected true when no armed gateway")
	}
}
