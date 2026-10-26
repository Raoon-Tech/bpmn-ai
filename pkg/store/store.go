package store

import (
	"context"
	"time"

	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
)

// FlowStatus represents the status of a flow execution.
type FlowStatus string

const (
	FlowStatusPending   FlowStatus = "PENDING"
	FlowStatusActive    FlowStatus = "ACTIVE"
	FlowStatusCompleted FlowStatus = "COMPLETED"
	FlowStatusSkipped   FlowStatus = "SKIPPED"
	FlowStatusError     FlowStatus = "ERROR"
)

// InstanceStatus represents the status of a process instance.
type InstanceStatus string

const (
	InstanceStatusCreated    InstanceStatus = "CREATED"
	InstanceStatusInProgress InstanceStatus = "IN_PROGRESS"
	InstanceStatusWaiting    InstanceStatus = "WAITING"
	InstanceStatusSuspended  InstanceStatus = "SUSPENDED"
	InstanceStatusCompleted  InstanceStatus = "COMPLETED"
	InstanceStatusError      InstanceStatus = "ERROR"
	InstanceStatusTerminated InstanceStatus = "TERMINATED"
)

// FlowRecord represents a flow execution record.
type FlowRecord struct {
	ID          string
	InstanceID  string
	ElementID   string
	ElementType bpmn.ElementType
	ThreadID    int
	PreviousID  string
	Status      FlowStatus
	StartedAt   *time.Time
	FinishedAt  *time.Time
	DurationMs  *int
}

// InstanceRecord represents a process instance (case).
type InstanceRecord struct {
	ID         string
	ProcessID  string
	Title      string
	Status     InstanceStatus
	CurrentUser string
	TenantID   string
	Variables  map[string]any
	PIN        string
	StartedAt  time.Time
	UpdatedAt  time.Time
	FinishedAt *time.Time
}

// ThreadRecord represents an execution thread.
type ThreadRecord struct {
	ID            int
	InstanceID    string
	ThreadIndex   int
	ParentIndex   *int
	FlowID        string
	Status        string
	CreatedAt     time.Time
}

// JobStatus represents the status of a queued job.
type JobStatus string

const (
	JobStatusPending   JobStatus = "PENDING"
	JobStatusRunning   JobStatus = "RUNNING"
	JobStatusCompleted JobStatus = "COMPLETED"
	JobStatusFailed    JobStatus = "FAILED"
	JobStatusDead      JobStatus = "DEAD"
)

// JobRecord represents a queued job.
type JobRecord struct {
	ID           string
	InstanceID   string
	FlowID       string
	Type         string
	Payload      map[string]any
	Status       JobStatus
	RetryCount   int
	MaxRetries   int
	ScheduledAt  time.Time
	ExecutedAt   *time.Time
	ErrorMessage string
	CreatedAt    time.Time
}

// DeadLetterRecord represents a job that exceeded all retries.
type DeadLetterRecord struct {
	ID           string
	JobID        string
	InstanceID   string
	Type         string
	Payload      map[string]any
	ErrorMessage string
	RetryCount   int
	CreatedAt    time.Time
}

// Store defines the persistence interface for the BPMN engine.
type Store interface {
	// Process definitions
	SaveProcess(ctx context.Context, proc *bpmn.Process) error
	GetProcess(ctx context.Context, id string) (*bpmn.Process, error)
	ListProcesses(ctx context.Context) ([]*bpmn.Process, error)
	DeleteProcess(ctx context.Context, id string) error

	// Process instances
	CreateInstance(ctx context.Context, inst *InstanceRecord) error
	GetInstance(ctx context.Context, id string) (*InstanceRecord, error)
	UpdateInstance(ctx context.Context, inst *InstanceRecord) error
	ListInstances(ctx context.Context, status InstanceStatus) ([]*InstanceRecord, error)

	// Flow records
	CreateFlow(ctx context.Context, flow *FlowRecord) error
	UpdateFlow(ctx context.Context, flow *FlowRecord) error
	GetFlow(ctx context.Context, id string) (*FlowRecord, error)
	GetFlowsByInstance(ctx context.Context, instanceID string) ([]*FlowRecord, error)
	GetActiveFlowsByThread(ctx context.Context, instanceID string, threadID int) ([]*FlowRecord, error)

	// Threads
	CreateThread(ctx context.Context, thread *ThreadRecord) error
	UpdateThread(ctx context.Context, thread *ThreadRecord) error
	GetThreadsByInstance(ctx context.Context, instanceID string) ([]*ThreadRecord, error)
	CloseThread(ctx context.Context, instanceID string, threadIndex int) error

	// Jobs
	CreateJob(ctx context.Context, job *JobRecord) error
	UpdateJob(ctx context.Context, job *JobRecord) error
	GetPendingJobs(ctx context.Context, limit int) ([]*JobRecord, error)
	GetJob(ctx context.Context, id string) (*JobRecord, error)

	// Dead letter queue
	CreateDeadLetter(ctx context.Context, dl *DeadLetterRecord) error
	GetDeadLetters(ctx context.Context, instanceID string) ([]*DeadLetterRecord, error)
	GetDeadLetter(ctx context.Context, id string) (*DeadLetterRecord, error)
	ListDeadLetters(ctx context.Context, limit int) ([]*DeadLetterRecord, error)

	// Execution log
	LogExecution(ctx context.Context, entry *ExecutionLogEntry) error
	GetExecutionLog(ctx context.Context, instanceID string) ([]*ExecutionLogEntry, error)

	// AI audit log
	LogAICall(ctx context.Context, entry *AIAuditLogEntry) error
}

// AIAuditLogEntry represents a single AI call audit log entry.
type AIAuditLogEntry struct {
	ID           string
	InstanceID   string
	ElementID    string
	Model        string
	InputText    string
	OutputText   string
	TokensIn     int
	TokensOut    int
	DurationMs   int
	Success      bool
	ErrorMessage string
	CreatedAt    time.Time
}

// ExecutionLogEntry represents a single execution log entry.
type ExecutionLogEntry struct {
	ID          string
	InstanceID  string
	ElementID   string
	ElementType bpmn.ElementType
	Action      string
	DurationMs  int
	CreatedAt   time.Time
}
