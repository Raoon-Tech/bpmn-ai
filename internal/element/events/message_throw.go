package events

import (
	"context"

	"github.com/Raoon-Tech/bpmn-ai/internal/element"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
)

type MessageThrowEvent struct {
	id       string
	name     string
	eventDef bpmn.EventDefinition
}

func NewMessageThrowEvent(elem bpmn.Element) (element.Element, error) {
	return &MessageThrowEvent{
		id:       elem.ID,
		name:     elem.Name,
		eventDef: elem.EventDefinition,
	}, nil
}

func (e *MessageThrowEvent) ID() string {
	return e.id
}

func (e *MessageThrowEvent) Type() bpmn.ElementType {
	return bpmn.ElementTypeMessageThrow
}

func (e *MessageThrowEvent) Execute(_ context.Context, execCtx element.ExecutionContext) element.ExecutionResult {
	flow := execCtx.Flow()
	flow.Status = "COMPLETED"

	if e.eventDef.MessageRef != "" {
		execCtx.SetVariable("message_ref", e.eventDef.MessageRef)
	}

	return element.ExecutionResult{
		Action:   element.ActionRoute,
		FlowData: flow,
	}
}

func (e *MessageThrowEvent) EventDefinition() bpmn.EventDefinition {
	return e.eventDef
}
