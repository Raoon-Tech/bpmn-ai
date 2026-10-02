package engine

import (
	"github.com/Raoon-Tech/bpmn-ai/internal/element"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

// ExecutionResult aliases the element package type for convenience.
type ExecutionResult = element.ExecutionResult

// Action aliases the element package type for convenience.
type Action = element.Action

const (
	ActionRoute     = element.ActionRoute
	ActionWait      = element.ActionWait
	ActionForm      = element.ActionForm
	ActionError     = element.ActionError
	ActionComplete  = element.ActionComplete
	ActionSkip      = element.ActionSkip
	ActionQueue     = element.ActionQueue
	ActionTerminate = element.ActionTerminate
)

// NewResult creates a new execution result.
func NewResult(action Action, flow *store.FlowRecord) ExecutionResult {
	return ExecutionResult{
		Action:   action,
		FlowData: flow,
	}
}

// NewResultWithFilters creates a result with flow filters.
func NewResultWithFilters(action Action, flow *store.FlowRecord, filters []string) ExecutionResult {
	return ExecutionResult{
		Action:      action,
		FlowData:    flow,
		FlowFilters: filters,
	}
}

// NewErrorResult creates an error result.
func NewErrorResult(flow *store.FlowRecord, err error) ExecutionResult {
	return ExecutionResult{
		Action:   ActionError,
		FlowData: flow,
		Error:    err,
	}
}
