package events

import (
	"context"

	"github.com/Raoon-Tech/bpmn-ai/internal/element"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

type ErrorCatchEvent struct {
	id        string
	name      string
	errorCode string
}

func NewErrorCatchEvent(elem bpmn.Element) (element.Element, error) {
	return &ErrorCatchEvent{
		id:        elem.ID,
		name:      elem.Name,
		errorCode: elem.EventDefinition.ErrorCode,
	}, nil
}

func (e *ErrorCatchEvent) ID() string {
	return e.id
}

func (e *ErrorCatchEvent) Type() bpmn.ElementType {
	return bpmn.ElementTypeErrorCatch
}

func (e *ErrorCatchEvent) EventDefinition() bpmn.EventDefinition {
	return bpmn.EventDefinition{Type: bpmn.EventTypeError, ErrorCode: e.errorCode}
}
func (e *ErrorCatchEvent) Execute(_ context.Context, execCtx element.ExecutionContext) element.ExecutionResult {
	flow := execCtx.Flow()
	flow.Status = store.FlowStatusCompleted

	return element.ExecutionResult{
		Action:   element.ActionRoute,
		FlowData: flow,
	}
}
