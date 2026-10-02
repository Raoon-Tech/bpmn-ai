package events

import (
	"context"

	"github.com/Raoon-Tech/bpmn-ai/internal/element"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

type SignalThrowEvent struct {
	id        string
	name      string
	signalRef string
}

func NewSignalThrowEvent(elem bpmn.Element) (element.Element, error) {
	return &SignalThrowEvent{
		id:        elem.ID,
		name:      elem.Name,
		signalRef: elem.EventDefinition.SignalRef,
	}, nil
}

func (e *SignalThrowEvent) ID() string { return e.id }
func (e *SignalThrowEvent) Type() bpmn.ElementType { return bpmn.ElementTypeSignalThrow }
func (e *SignalThrowEvent) EventDefinition() bpmn.EventDefinition {
	return bpmn.EventDefinition{Type: bpmn.EventTypeSignal, SignalRef: e.signalRef}
}

func (e *SignalThrowEvent) Execute(_ context.Context, execCtx element.ExecutionContext) element.ExecutionResult {
	flow := execCtx.Flow()
	flow.Status = store.FlowStatusCompleted

	if e.signalRef != "" {
		execCtx.SetVariable("signal_ref", e.signalRef)
	}

	return element.ExecutionResult{
		Action:   element.ActionRoute,
		FlowData: flow,
	}
}
