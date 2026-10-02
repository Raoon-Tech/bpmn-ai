package engine

import (
	"context"

	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

// EngineStore is the persistence interface required by the BPMN execution engine.
// It aggregates only the storage operations the engine needs, following ISP.
type EngineStore interface {
	// Process
	GetProcess(ctx context.Context, id string) (*bpmn.Process, error)

	// Instance
	GetInstance(ctx context.Context, id string) (*store.InstanceRecord, error)
	UpdateInstance(ctx context.Context, inst *store.InstanceRecord) error
	ListInstances(ctx context.Context, status store.InstanceStatus) ([]*store.InstanceRecord, error)

	// Flow
	CreateFlow(ctx context.Context, flow *store.FlowRecord) error
	UpdateFlow(ctx context.Context, flow *store.FlowRecord) error
	GetFlow(ctx context.Context, id string) (*store.FlowRecord, error)
	GetFlowsByInstance(ctx context.Context, instanceID string) ([]*store.FlowRecord, error)

	// Thread
	CreateThread(ctx context.Context, thread *store.ThreadRecord) error
	GetThreadsByInstance(ctx context.Context, instanceID string) ([]*store.ThreadRecord, error)
	CloseThread(ctx context.Context, instanceID string, threadIndex int) error

	// Job
	CreateJob(ctx context.Context, job *store.JobRecord) error

	// Execution log
	LogExecution(ctx context.Context, entry *store.ExecutionLogEntry) error

	// AI audit log
	LogAICall(ctx context.Context, entry *store.AIAuditLogEntry) error
}
