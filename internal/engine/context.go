package engine

import (
	"context"

	"github.com/Raoon-Tech/bpmn-ai/internal/element"
	"github.com/Raoon-Tech/bpmn-ai/internal/observability"
	"github.com/Raoon-Tech/bpmn-ai/internal/process"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

// ExecutionContext holds the immutable state for a single element execution.
type ExecutionContext struct {
	ctx      context.Context
	instance *process.Instance
	flow     *store.FlowRecord
	store    element.ElementStore
	logger   *observability.Logger
}

// NewExecutionContext creates a new execution context.
func NewExecutionContext(
	ctx context.Context,
	instance *process.Instance,
	flow *store.FlowRecord,
	store element.ElementStore,
	logger *observability.Logger,
) *ExecutionContext {
	return &ExecutionContext{
		ctx:      ctx,
		instance: instance,
		flow:     flow,
		store:    store,
		logger:   logger,
	}
}

// Context returns the underlying context for cancellation.
func (e *ExecutionContext) Context() context.Context {
	return e.ctx
}

// Instance returns the process instance.
func (e *ExecutionContext) Instance() element.Instance {
	return e.instance
}

// Flow returns the current flow record.
func (e *ExecutionContext) Flow() *store.FlowRecord {
	return e.flow
}

// GetVariable returns a process variable.
func (e *ExecutionContext) GetVariable(key string) (any, bool) {
	return e.instance.GetVariable(key)
}

// SetVariable sets a process variable.
func (e *ExecutionContext) SetVariable(key string, value any) {
	e.instance.SetVariable(key, value)
}

// Store returns the element-accessible persistence interface.
func (e *ExecutionContext) Store() element.ElementStore {
	return e.store
}

// Element returns the BPMN element for the current flow.
func (e *ExecutionContext) Element() (bpmn.Element, bool) {
	elem, ok := e.instance.Process.Elements[e.flow.ElementID]
	return elem, ok
}

// Ensure ExecutionContext implements element.ExecutionContext.
var _ element.ExecutionContext = (*ExecutionContext)(nil)
