package events

import (
	"context"
	"strings"

	"github.com/Raoon-Tech/bpmn-ai/internal/element"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

// StartEvent implements the BPMN start event.
type StartEvent struct {
	id   string
	name string
}

// NewStartEvent creates a new start event element.
func NewStartEvent(elem bpmn.Element) (element.Element, error) {
	return &StartEvent{
		id:   elem.ID,
		name: elem.Name,
	}, nil
}

// ID returns the element ID.
func (s *StartEvent) ID() string {
	return s.id
}

// Type returns the element type.
func (s *StartEvent) Type() bpmn.ElementType {
	return bpmn.ElementTypeStartEvent
}

// Execute runs the start event logic.
func (s *StartEvent) Execute(_ context.Context, execCtx element.ExecutionContext) element.ExecutionResult {
	flow := execCtx.Flow()

	return element.ExecutionResult{
		Action:   element.ActionRoute,
		FlowData: flow,
	}
}

// EventDefinition returns the event definition.
func (s *StartEvent) EventDefinition() bpmn.EventDefinition {
	return bpmn.EventDefinition{}
}

// EndEvent implements the BPMN end event.
type EndEvent struct {
	id   string
	name string
}

// NewEndEvent creates a new end event element.
func NewEndEvent(elem bpmn.Element) (element.Element, error) {
	return &EndEvent{
		id:   elem.ID,
		name: elem.Name,
	}, nil
}

// ID returns the element ID.
func (e *EndEvent) ID() string {
	return e.id
}

// Type returns the element type.
func (e *EndEvent) Type() bpmn.ElementType {
	return bpmn.ElementTypeEndEvent
}

// Execute runs the end event logic.
func (e *EndEvent) Execute(_ context.Context, execCtx element.ExecutionContext) element.ExecutionResult {
	flow := execCtx.Flow()

	// Mark flow as completed
	now := execCtx.Flow().FinishedAt
	if now == nil {
		t := execCtx.Flow().StartedAt
		if t != nil {
			now = t
		}
	}
	flow.Status = store.FlowStatusCompleted

	// Check if this end event is a sub-process exit
	elemDef, ok := execCtx.Element()
	if ok && elemDef.ExtensionData != nil {
		if exitStr, exists := elemDef.ExtensionData["subprocess_exit_flows"]; exists && exitStr != "" {
			return element.ExecutionResult{
				Action:      element.ActionRoute,
				FlowData:    flow,
				FlowFilters: strings.Split(exitStr, ","),
			}
		}
	}

	return element.ExecutionResult{
		Action:   element.ActionComplete,
		FlowData: flow,
	}
}

// EventDefinition returns the event definition.
func (e *EndEvent) EventDefinition() bpmn.EventDefinition {
	return bpmn.EventDefinition{}
}
