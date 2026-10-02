package events

import (
	"context"

	"github.com/Raoon-Tech/bpmn-ai/internal/element"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
)

type SignalCatchEvent struct {
	id        string
	name      string
	signalRef string
}

func NewSignalCatchEvent(elem bpmn.Element) (element.Element, error) {
	return &SignalCatchEvent{
		id:        elem.ID,
		name:      elem.Name,
		signalRef: elem.EventDefinition.SignalRef,
	}, nil
}

func (e *SignalCatchEvent) ID() string { return e.id }
func (e *SignalCatchEvent) Type() bpmn.ElementType { return bpmn.ElementTypeSignalCatch }
func (e *SignalCatchEvent) EventDefinition() bpmn.EventDefinition {
	return bpmn.EventDefinition{Type: bpmn.EventTypeSignal, SignalRef: e.signalRef}
}

func (e *SignalCatchEvent) Execute(_ context.Context, execCtx element.ExecutionContext) element.ExecutionResult {
	flow := execCtx.Flow()

	if e.signalRef != "" {
		execCtx.SetVariable("expected_signal", e.signalRef)
	}

	return element.ExecutionResult{
		Action:   element.ActionWait,
		FlowData: flow,
	}
}
