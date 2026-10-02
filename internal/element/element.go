package element

import (
	"context"
	"time"

	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

// Action represents the action the engine should take after element execution.
type Action string

const (
	ActionRoute     Action = "ROUTE"
	ActionWait      Action = "WAIT"
	ActionForm      Action = "FORM"
	ActionError     Action = "ERROR"
	ActionComplete  Action = "COMPLETE"
	ActionSkip      Action = "SKIP"
	ActionQueue     Action = "QUEUE"
	ActionTerminate  Action = "TERMINATE"
	ActionThrowError   Action = "THROW_ERROR"
	ActionCallActivity Action = "CALL_ACTIVITY"
)

// ExecutionResult is the result of executing a BPMN element.
type ExecutionResult struct {
	Action      Action
	FlowData    *store.FlowRecord
	FlowID      string
	FlowFilters []string
	Error       error
	DurationMs  int
	ContinueAt   *time.Time // scheduled continuation time (e.g., for TimerEvent)
	CalledElement string   // process ID to invoke (for CallActivity)
}

// Element is the base interface for all BPMN elements.
type Element interface {
	ID() string
	Type() bpmn.ElementType
	Execute(ctx context.Context, execCtx ExecutionContext) ExecutionResult
}

// ExecutionContext provides the context for element execution.
type ExecutionContext interface {
	Instance() Instance
	Flow() *store.FlowRecord
	GetVariable(key string) (any, bool)
	SetVariable(key string, value any)
	Store() ElementStore
	Element() (bpmn.Element, bool)
}

// Instance is a minimal interface for process instances.
type Instance interface {
	GetID() string
	GetState() string
	GetVariable(key string) (any, bool)
	SetVariable(key string, value any)
}
