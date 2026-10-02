package events

import (
	"context"
	"fmt"

	"github.com/Raoon-Tech/bpmn-ai/internal/element"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

type ErrorEndEvent struct {
	id        string
	name      string
	errorCode string
}

func NewErrorEndEvent(elem bpmn.Element) (element.Element, error) {
	return &ErrorEndEvent{
		id:        elem.ID,
		name:      elem.Name,
		errorCode: elem.EventDefinition.ErrorCode,
	}, nil
}

func (e *ErrorEndEvent) ID() string {
	return e.id
}

func (e *ErrorEndEvent) Type() bpmn.ElementType {
	return bpmn.ElementTypeErrorEnd
}

func (e *ErrorEndEvent) EventDefinition() bpmn.EventDefinition {
	return bpmn.EventDefinition{Type: bpmn.EventTypeError, ErrorCode: e.errorCode}
}
func (e *ErrorEndEvent) Execute(_ context.Context, execCtx element.ExecutionContext) element.ExecutionResult {
	flow := execCtx.Flow()
	flow.Status = store.FlowStatusCompleted

	return element.ExecutionResult{
		Action:   element.ActionThrowError,
		FlowData: flow,
		Error:    fmt.Errorf("error thrown: code=%s element=%s", e.errorCode, e.id),
	}
}
