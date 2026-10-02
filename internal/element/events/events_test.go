package events

import (
	"context"
	"testing"
	"time"

	"github.com/Raoon-Tech/bpmn-ai/internal/element"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

type mockExecutionContext struct {
	flow       *store.FlowRecord
	instance   element.Instance
	stored     element.ElementStore
	variables  map[string]any
	elem       bpmn.Element
}

func (m *mockExecutionContext) Instance() element.Instance {
	return m.instance
}

func (m *mockExecutionContext) Flow() *store.FlowRecord {
	return m.flow
}

func (m *mockExecutionContext) GetVariable(key string) (any, bool) {
	v, ok := m.variables[key]
	return v, ok
}

func (m *mockExecutionContext) SetVariable(key string, value any) {
	if m.variables == nil {
		m.variables = make(map[string]any)
	}
	m.variables[key] = value
}

func (m *mockExecutionContext) Store() element.ElementStore {
	return m.stored
}

func (m *mockExecutionContext) Element() (bpmn.Element, bool) {
	return m.elem, m.elem.ID != ""
}

func TestStartEvent_Execute(t *testing.T) {
	elem := bpmn.Element{ID: "start-1", Name: "Start"}
	s, err := NewStartEvent(elem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if s.ID() != "start-1" {
		t.Errorf("expected ID start-1, got %s", s.ID())
	}
	if s.Type() != bpmn.ElementTypeStartEvent {
		t.Errorf("expected type startEvent, got %s", s.Type())
	}

	result := s.Execute(context.Background(), &mockExecutionContext{
		flow: &store.FlowRecord{ElementID: "start-1"},
	})

	if result.Action != element.ActionRoute {
		t.Errorf("expected ActionRoute, got %s", result.Action)
	}
}

func TestEndEvent_Execute(t *testing.T) {
	elem := bpmn.Element{ID: "end-1", Name: "End"}
	e, err := NewEndEvent(elem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if e.ID() != "end-1" {
		t.Errorf("expected ID end-1, got %s", e.ID())
	}
	if e.Type() != bpmn.ElementTypeEndEvent {
		t.Errorf("expected type endEvent, got %s", e.Type())
	}

	result := e.Execute(context.Background(), &mockExecutionContext{
		flow: &store.FlowRecord{ElementID: "end-1"},
	})

	if result.Action != element.ActionComplete {
		t.Errorf("expected ActionComplete, got %s", result.Action)
	}
}

func TestTerminateEvent_Execute(t *testing.T) {
	elem := bpmn.Element{
		ID: "term-1", Name: "Terminate",
		EventDefinition: bpmn.EventDefinition{Type: bpmn.EventTypeTerminate},
	}
	e, err := NewTerminateEvent(elem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if e.ID() != "term-1" {
		t.Errorf("expected ID term-1, got %s", e.ID())
	}
	if e.Type() != bpmn.ElementTypeTerminateEvent {
		t.Errorf("expected type terminateEvent, got %s", e.Type())
	}

	result := e.Execute(context.Background(), &mockExecutionContext{
		flow: &store.FlowRecord{ElementID: "term-1"},
	})

	if result.Action != element.ActionTerminate {
		t.Errorf("expected ActionTerminate, got %s", result.Action)
	}
}

func TestTimerEvent_Execute(t *testing.T) {
	tests := []struct {
		name       string
		eventDef   bpmn.EventDefinition
		wantAction element.Action
	}{
		{
			name:       "with timer value",
			eventDef:   bpmn.EventDefinition{Type: bpmn.EventTypeTimer, TimerType: bpmn.TimerTypeDuration, TimerValue: "PT1H"},
			wantAction: element.ActionWait,
		},
		{
			name:       "without timer value",
			eventDef:   bpmn.EventDefinition{Type: bpmn.EventTypeNone},
			wantAction: element.ActionRoute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			elem := bpmn.Element{ID: "timer-1", EventDefinition: tt.eventDef}
			e, err := NewTimerEvent(elem)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if e.Type() != bpmn.ElementTypeTimerEvent {
				t.Errorf("expected type timerEvent, got %s", e.Type())
			}

			ctx := &mockExecutionContext{
				flow:      &store.FlowRecord{ElementID: "timer-1"},
				variables: make(map[string]any),
			}
			result := e.Execute(context.Background(), ctx)
			if result.Action != tt.wantAction {
				t.Errorf("expected %s, got %s", tt.wantAction, result.Action)
			}
		})
	}
}

func TestTimerEvent_ContinueAt(t *testing.T) {
	elem := bpmn.Element{
		ID: "timer-1",
		EventDefinition: bpmn.EventDefinition{
			Type:       bpmn.EventTypeTimer,
			TimerType:  bpmn.TimerTypeDuration,
			TimerValue: "PT1H",
		},
	}
	e, err := NewTimerEvent(elem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := &mockExecutionContext{
		flow:      &store.FlowRecord{ElementID: "timer-1"},
		variables: make(map[string]any),
	}
	result := e.Execute(context.Background(), ctx)

	if result.Action != element.ActionWait {
		t.Errorf("expected ActionWait, got %s", result.Action)
	}
	if result.ContinueAt == nil {
		t.Fatal("expected ContinueAt to be set for duration timer")
	}
	expected := time.Now().Add(1 * time.Hour)
	diff := result.ContinueAt.Sub(expected)
	if diff < -2*time.Second || diff > 2*time.Second {
		t.Errorf("expected ContinueAt ~1h from now, diff=%v", diff)
	}
}

func TestParseISODuration(t *testing.T) {
	tests := []struct {
		input    string
		want     time.Duration
		wantErr  bool
	}{
		{"PT1H", time.Hour, false},
		{"PT30M", 30 * time.Minute, false},
		{"PT30S", 30 * time.Second, false},
		{"P1D", 24 * time.Hour, false},
		{"P1DT1H", 25 * time.Hour, false},
		{"PT1H30M", 90 * time.Minute, false},
		{"", 0, true},
		{"not-duration", 0, true},
	}

	for _, tt := range tests {
		d, err := parseISODuration(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseISODuration(%q) expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseISODuration(%q) unexpected error: %v", tt.input, err)
			continue
		}
		if d != tt.want {
			t.Errorf("parseISODuration(%q) = %v, want %v", tt.input, d, tt.want)
		}
	}
}

func TestCalculateSchedule_Duration(t *testing.T) {
	tm := CalculateSchedule(bpmn.TimerTypeDuration, "PT30M")
	if tm == nil {
		t.Fatal("expected non-nil time")
	}
	expected := time.Now().Add(30 * time.Minute)
	diff := tm.Sub(expected)
	if diff < -2*time.Second || diff > 2*time.Second {
		t.Errorf("expected ~30m, diff=%v", diff)
	}
}

func TestCalculateSchedule_Date(t *testing.T) {
	tm := CalculateSchedule(bpmn.TimerTypeDate, "2027-06-01T12:00:00Z")
	if tm == nil {
		t.Fatal("expected non-nil time")
	}
	if tm.Year() != 2027 || tm.Month() != 6 || tm.Day() != 1 {
		t.Errorf("expected 2027-06-01, got %v", tm)
	}
}

func TestCalculateSchedule_Empty(t *testing.T) {
	tm := CalculateSchedule("unknown", "")
	if tm != nil {
		t.Errorf("expected nil for unknown type, got %v", tm)
	}
}

func TestMessageThrowEvent_Execute(t *testing.T) {
	elem := bpmn.Element{
		ID: "msg-throw-1", Name: "Send Message",
		EventDefinition: bpmn.EventDefinition{Type: bpmn.EventTypeMessage, MessageRef: "msg-1"},
	}
	e, err := NewMessageThrowEvent(elem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if e.Type() != bpmn.ElementTypeMessageThrow {
		t.Errorf("expected type messageThrow, got %s", e.Type())
	}

	ctx := &mockExecutionContext{
		flow:      &store.FlowRecord{ElementID: "msg-throw-1"},
		variables: make(map[string]any),
	}
	result := e.Execute(context.Background(), ctx)
	if result.Action != element.ActionRoute {
		t.Errorf("expected ActionRoute, got %s", result.Action)
	}

	msgRef, ok := ctx.GetVariable("message_ref")
	if !ok || msgRef != "msg-1" {
		t.Errorf("expected message_ref to be msg-1, got %v", msgRef)
	}
}

func TestMessageCatchEvent_Execute(t *testing.T) {
	elem := bpmn.Element{
		ID: "msg-catch-1", Name: "Receive Message",
		EventDefinition: bpmn.EventDefinition{Type: bpmn.EventTypeMessage, MessageRef: "msg-1"},
	}
	e, err := NewMessageCatchEvent(elem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if e.Type() != bpmn.ElementTypeMessageCatch {
		t.Errorf("expected type messageCatch, got %s", e.Type())
	}

	ctx := &mockExecutionContext{
		flow:      &store.FlowRecord{ElementID: "msg-catch-1"},
		variables: make(map[string]any),
	}
	result := e.Execute(context.Background(), ctx)
	if result.Action != element.ActionWait {
		t.Errorf("expected ActionWait, got %s", result.Action)
	}

	msgRef, ok := ctx.GetVariable("expected_message")
	if !ok || msgRef != "msg-1" {
		t.Errorf("expected expected_message to be msg-1, got %v", msgRef)
	}
}

func TestErrorEndEvent_Execute(t *testing.T) {
	elem := bpmn.Element{
		ID: "err-end-1", Name: "Error End",
		EventDefinition: bpmn.EventDefinition{Type: bpmn.EventTypeError, ErrorCode: "ERR-001"},
	}
	e, err := NewErrorEndEvent(elem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e.ID() != "err-end-1" {
		t.Errorf("expected ID err-end-1, got %s", e.ID())
	}
	if e.Type() != bpmn.ElementTypeErrorEnd {
		t.Errorf("expected type errorEnd, got %s", e.Type())
	}
	result := e.Execute(context.Background(), &mockExecutionContext{
		flow: &store.FlowRecord{ElementID: "err-end-1"},
	})
	if result.Action != element.ActionThrowError {
		t.Errorf("expected ActionThrowError, got %s", result.Action)
	}
	if result.Error == nil {
		t.Error("expected non-nil error")
	}
}

func TestErrorCatchEvent_Execute(t *testing.T) {
	elem := bpmn.Element{
		ID: "err-catch-1", Name: "Error Catch",
		EventDefinition: bpmn.EventDefinition{Type: bpmn.EventTypeError, ErrorCode: "ERR-001"},
		AttachedToRef:   "sp-1",
	}
	e, err := NewErrorCatchEvent(elem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e.ID() != "err-catch-1" {
		t.Errorf("expected ID err-catch-1, got %s", e.ID())
	}
	if e.Type() != bpmn.ElementTypeErrorCatch {
		t.Errorf("expected type errorCatch, got %s", e.Type())
	}
	result := e.Execute(context.Background(), &mockExecutionContext{
		flow: &store.FlowRecord{ElementID: "err-catch-1"},
	})
	if result.Action != element.ActionRoute {
		t.Errorf("expected ActionRoute, got %s", result.Action)
	}
}

func TestSignalThrowEvent_Execute(t *testing.T) {
	elem := bpmn.Element{
		ID: "sig-throw-1", Name: "Signal Throw",
		EventDefinition: bpmn.EventDefinition{Type: bpmn.EventTypeSignal, SignalRef: "sig-1"},
	}
	e, err := NewSignalThrowEvent(elem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e.ID() != "sig-throw-1" {
		t.Errorf("expected ID sig-throw-1, got %s", e.ID())
	}
	if e.Type() != bpmn.ElementTypeSignalThrow {
		t.Errorf("expected type signalThrow, got %s", e.Type())
	}
	result := e.Execute(context.Background(), &mockExecutionContext{
		flow: &store.FlowRecord{ElementID: "sig-throw-1"},
	})
	if result.Action != element.ActionRoute {
		t.Errorf("expected ActionRoute, got %s", result.Action)
	}
}

func TestSignalCatchEvent_Execute(t *testing.T) {
	elem := bpmn.Element{
		ID: "sig-catch-1", Name: "Signal Catch",
		EventDefinition: bpmn.EventDefinition{Type: bpmn.EventTypeSignal, SignalRef: "sig-1"},
	}
	e, err := NewSignalCatchEvent(elem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e.ID() != "sig-catch-1" {
		t.Errorf("expected ID sig-catch-1, got %s", e.ID())
	}
	if e.Type() != bpmn.ElementTypeSignalCatch {
		t.Errorf("expected type signalCatch, got %s", e.Type())
	}
	mock := &mockExecutionContext{
		flow:      &store.FlowRecord{ElementID: "sig-catch-1"},
		variables: make(map[string]any),
	}
	result := e.Execute(context.Background(), mock)
	if result.Action != element.ActionWait {
		t.Errorf("expected ActionWait, got %s", result.Action)
	}
	sigRef, ok := mock.GetVariable("expected_signal")
	if !ok || sigRef != "sig-1" {
		t.Errorf("expected expected_signal=sig-1, got %v", sigRef)
	}
}

func TestEventDefinitions(t *testing.T) {
	tests := []struct {
		name     string
		elem     bpmn.Element
		newFn    func(bpmn.Element) (element.Element, error)
		hasDef   bool
		wantRef  string
	}{
		{"StartEvent", bpmn.Element{ID: "s1"}, func(e bpmn.Element) (element.Element, error) { return NewStartEvent(e) }, false, ""},
		{"EndEvent", bpmn.Element{ID: "e1"}, func(e bpmn.Element) (element.Element, error) { return NewEndEvent(e) }, false, ""},
		{"TerminateEvent", bpmn.Element{ID: "t1", EventDefinition: bpmn.EventDefinition{Type: bpmn.EventTypeTerminate}}, func(e bpmn.Element) (element.Element, error) { return NewTerminateEvent(e) }, true, ""},
		{"TimerEvent", bpmn.Element{ID: "tm1", EventDefinition: bpmn.EventDefinition{Type: bpmn.EventTypeTimer, TimerValue: "PT1H"}}, func(e bpmn.Element) (element.Element, error) { return NewTimerEvent(e) }, true, ""},
		{"MessageThrow", bpmn.Element{ID: "mt1", EventDefinition: bpmn.EventDefinition{Type: bpmn.EventTypeMessage, MessageRef: "msg-1"}}, func(e bpmn.Element) (element.Element, error) { return NewMessageThrowEvent(e) }, true, "msg-1"},
		{"MessageCatch", bpmn.Element{ID: "mc1", EventDefinition: bpmn.EventDefinition{Type: bpmn.EventTypeMessage, MessageRef: "msg-1"}}, func(e bpmn.Element) (element.Element, error) { return NewMessageCatchEvent(e) }, true, "msg-1"},
		{"ErrorEnd", bpmn.Element{ID: "ee1", EventDefinition: bpmn.EventDefinition{Type: bpmn.EventTypeError, ErrorCode: "ERR"}}, func(e bpmn.Element) (element.Element, error) { return NewErrorEndEvent(e) }, true, "ERR"},
		{"ErrorCatch", bpmn.Element{ID: "ec1", EventDefinition: bpmn.EventDefinition{Type: bpmn.EventTypeError, ErrorCode: "ERR"}}, func(e bpmn.Element) (element.Element, error) { return NewErrorCatchEvent(e) }, true, "ERR"},
		{"SignalThrow", bpmn.Element{ID: "st1", EventDefinition: bpmn.EventDefinition{Type: bpmn.EventTypeSignal, SignalRef: "sig-1"}}, func(e bpmn.Element) (element.Element, error) { return NewSignalThrowEvent(e) }, true, "sig-1"},
		{"SignalCatch", bpmn.Element{ID: "sc1", EventDefinition: bpmn.EventDefinition{Type: bpmn.EventTypeSignal, SignalRef: "sig-1"}}, func(e bpmn.Element) (element.Element, error) { return NewSignalCatchEvent(e) }, true, "sig-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name+"_EventDefinition", func(t *testing.T) {
			el, err := tt.newFn(tt.elem)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			ed := el.(interface{ EventDefinition() bpmn.EventDefinition }).EventDefinition()
			if !tt.hasDef {
				return // Start/End return empty, skip type/reference check
			}
			if ed.MessageRef != tt.wantRef && ed.ErrorCode != tt.wantRef && ed.SignalRef != tt.wantRef {
				if tt.wantRef != "" {
					t.Errorf("expected ref %s, got msg=%s err=%s sig=%s", tt.wantRef, ed.MessageRef, ed.ErrorCode, ed.SignalRef)
				}
			}
		})
	}
}

func TestIDMethods(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		newFn   func(bpmn.Element) (element.Element, error)
		wantTyp bpmn.ElementType
	}{
		{"StartEvent", "s1", func(e bpmn.Element) (element.Element, error) { return NewStartEvent(e) }, bpmn.ElementTypeStartEvent},
		{"EndEvent", "e1", func(e bpmn.Element) (element.Element, error) { return NewEndEvent(e) }, bpmn.ElementTypeEndEvent},
		{"TerminateEvent", "t1", func(e bpmn.Element) (element.Element, error) { return NewTerminateEvent(e) }, bpmn.ElementTypeTerminateEvent},
		{"TimerEvent", "tm1", func(e bpmn.Element) (element.Element, error) { return NewTimerEvent(e) }, bpmn.ElementTypeTimerEvent},
		{"MessageThrow", "mt1", func(e bpmn.Element) (element.Element, error) { return NewMessageThrowEvent(e) }, bpmn.ElementTypeMessageThrow},
		{"MessageCatch", "mc1", func(e bpmn.Element) (element.Element, error) { return NewMessageCatchEvent(e) }, bpmn.ElementTypeMessageCatch},
		{"ErrorEnd", "ee1", func(e bpmn.Element) (element.Element, error) { return NewErrorEndEvent(e) }, bpmn.ElementTypeErrorEnd},
		{"ErrorCatch", "ec1", func(e bpmn.Element) (element.Element, error) { return NewErrorCatchEvent(e) }, bpmn.ElementTypeErrorCatch},
		{"SignalThrow", "st1", func(e bpmn.Element) (element.Element, error) { return NewSignalThrowEvent(e) }, bpmn.ElementTypeSignalThrow},
		{"SignalCatch", "sc1", func(e bpmn.Element) (element.Element, error) { return NewSignalCatchEvent(e) }, bpmn.ElementTypeSignalCatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			el, err := tt.newFn(bpmn.Element{ID: tt.id})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if el.ID() != tt.id {
				t.Errorf("expected ID %s, got %s", tt.id, el.ID())
			}
			if el.Type() != tt.wantTyp {
				t.Errorf("expected type %s, got %s", tt.wantTyp, el.Type())
			}
		})
	}
}

func TestEndEvent_SubProcessExit(t *testing.T) {
	elem := bpmn.Element{
		ID:   "sp-end-1",
		Name: "Sub End",
		ExtensionData: map[string]string{
			"subprocess_exit_flows": "flow-exit-1,flow-exit-2",
		},
	}
	e, err := NewEndEvent(elem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	result := e.Execute(context.Background(), &mockExecutionContext{
		flow: &store.FlowRecord{ElementID: "sp-end-1"},
		elem: elem,
	})
	if result.Action != element.ActionRoute {
		t.Errorf("expected ActionRoute for subprocess exit, got %s", result.Action)
	}
	if len(result.FlowFilters) != 2 || result.FlowFilters[0] != "flow-exit-1" {
		t.Errorf("expected FlowFilters [flow-exit-1 flow-exit-2], got %v", result.FlowFilters)
	}
}

func TestCalculateSchedule_Cron(t *testing.T) {
	tm := CalculateSchedule(bpmn.TimerTypeCycle, "every 5 s")
	if tm == nil {
		t.Fatal("expected non-nil time for cron")
	}
	diff := time.Until(*tm)
	if diff < 0 || diff > 10*time.Second {
		t.Errorf("expected 5s from now, got %v", diff)
	}

	tm = CalculateSchedule(bpmn.TimerTypeCycle, "*/5 * * * *")
	if tm == nil {
		t.Fatal("expected non-nil time for 5-field cron")
	}
}

func TestCalculateSchedule_InvalidDuration(t *testing.T) {
	tm := CalculateSchedule(bpmn.TimerTypeDuration, "invalid")
	if tm != nil {
		t.Errorf("expected nil for invalid duration, got %v", tm)
	}

	tm = CalculateSchedule(bpmn.TimerTypeDate, "not-a-date")
	if tm != nil {
		t.Errorf("expected nil for invalid date, got %v", tm)
	}

	tm = CalculateSchedule(bpmn.TimerTypeCycle, "invalid cron")
	if tm != nil {
		t.Errorf("expected nil for invalid cron, got %v", tm)
	}
}

func TestTimerEventKey(t *testing.T) {
	key := timerEventKey("elem-1")
	if key != "timer:elem-1" {
		t.Errorf("expected timer:elem-1, got %s", key)
	}
}
