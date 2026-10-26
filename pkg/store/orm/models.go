package orm

import (
	"time"

	"gorm.io/datatypes"
)

// ProcessModel is the persisted BPMN process definition.
type ProcessModel struct {
	ID         string         `gorm:"column:id;type:varchar(64);primaryKey"`
	Name       string         `gorm:"column:name;type:varchar(255);not null;default:''"`
	Version    int            `gorm:"column:version;not null;default:1"`
	Definition datatypes.JSON `gorm:"column:definition;not null"`
	CreatedAt  time.Time      `gorm:"column:created_at"`
}

// TableName returns the physical table name.
func (ProcessModel) TableName() string { return "processes" }

// InstanceModel is a process instance (case).
type InstanceModel struct {
	ID          string         `gorm:"column:id;type:varchar(64);primaryKey"`
	ProcessID   string         `gorm:"column:process_id;type:varchar(64);not null;index"`
	Title       string         `gorm:"column:title;type:varchar(255);not null;default:''"`
	Status      string         `gorm:"column:status;type:varchar(32);not null;index"`
	CurrentUser string         `gorm:"column:current_user;type:varchar(255);not null;default:''"`
	Variables   datatypes.JSON `gorm:"column:variables"`
	PIN         string         `gorm:"column:pin;type:varchar(255);not null;default:''"`
	StartedAt   time.Time      `gorm:"column:started_at;not null"`
	UpdatedAt   time.Time      `gorm:"column:updated_at;not null"`
	FinishedAt  *time.Time     `gorm:"column:finished_at"`
}

// TableName returns the physical table name.
func (InstanceModel) TableName() string { return "instances" }

// FlowModel is a single executed flow record.
type FlowModel struct {
	ID          string     `gorm:"column:id;type:varchar(64);primaryKey"`
	InstanceID  string     `gorm:"column:instance_id;type:varchar(64);not null;index;index:idx_flows_thread,priority:1"`
	ElementID   string     `gorm:"column:element_id;type:varchar(191);not null"`
	ElementType string     `gorm:"column:element_type;type:varchar(64);not null"`
	ThreadID    int        `gorm:"column:thread_id;not null;default:1;index:idx_flows_thread,priority:2"`
	PreviousID  string     `gorm:"column:previous_id;type:varchar(191);not null;default:''"`
	Status      string     `gorm:"column:status;type:varchar(32);not null"`
	StartedAt   *time.Time `gorm:"column:started_at"`
	FinishedAt  *time.Time `gorm:"column:finished_at"`
	DurationMs  *int       `gorm:"column:duration_ms"`
}

// TableName returns the physical table name.
func (FlowModel) TableName() string { return "flows" }

// ThreadModel is an execution thread.
type ThreadModel struct {
	ID          int       `gorm:"column:id;primaryKey;autoIncrement"`
	InstanceID  string    `gorm:"column:instance_id;type:varchar(64);not null;index;uniqueIndex:ux_threads_instance_index,priority:1"`
	ThreadIndex int       `gorm:"column:thread_index;not null;uniqueIndex:ux_threads_instance_index,priority:2"`
	ParentIndex *int      `gorm:"column:parent_index"`
	FlowID      string    `gorm:"column:flow_id;type:varchar(64);not null;default:''"`
	Status      string    `gorm:"column:status;type:varchar(32);not null;default:'ACTIVE'"`
	CreatedAt   time.Time `gorm:"column:created_at"`
}

// TableName returns the physical table name.
func (ThreadModel) TableName() string { return "threads" }

// JobModel is a queued job.
type JobModel struct {
	ID           string         `gorm:"column:id;type:varchar(64);primaryKey"`
	InstanceID   string         `gorm:"column:instance_id;type:varchar(64);not null;index"`
	FlowID       string         `gorm:"column:flow_id;type:varchar(64);not null;default:''"`
	Type         string         `gorm:"column:type;type:varchar(64);not null;default:''"`
	Payload      datatypes.JSON `gorm:"column:payload"`
	Status       string         `gorm:"column:status;type:varchar(32);not null;index"`
	RetryCount   int            `gorm:"column:retry_count;not null;default:0"`
	MaxRetries   int            `gorm:"column:max_retries;not null;default:3"`
	ScheduledAt  time.Time      `gorm:"column:scheduled_at;not null;index"`
	ExecutedAt   *time.Time     `gorm:"column:executed_at"`
	ErrorMessage string         `gorm:"column:error_message;type:text"`
	CreatedAt    time.Time      `gorm:"column:created_at"`
}

// TableName returns the physical table name.
func (JobModel) TableName() string { return "jobs" }

// DeadLetterModel is a job that exhausted its retries.
type DeadLetterModel struct {
	ID           string         `gorm:"column:id;type:varchar(64);primaryKey"`
	JobID        string         `gorm:"column:job_id;type:varchar(64);not null;default:''"`
	InstanceID   string         `gorm:"column:instance_id;type:varchar(64);not null;index"`
	Type         string         `gorm:"column:type;type:varchar(64);not null;default:''"`
	Payload      datatypes.JSON `gorm:"column:payload"`
	ErrorMessage string         `gorm:"column:error_message;type:text"`
	RetryCount   int            `gorm:"column:retry_count;not null;default:0"`
	CreatedAt    time.Time      `gorm:"column:created_at"`
}

// TableName returns the physical table name.
func (DeadLetterModel) TableName() string { return "dead_letters" }

// ExecutionLogModel is a single execution log entry.
type ExecutionLogModel struct {
	ID          string    `gorm:"column:id;type:varchar(64);primaryKey"`
	InstanceID  string    `gorm:"column:instance_id;type:varchar(64);not null;index"`
	ElementID   string    `gorm:"column:element_id;type:varchar(191);not null;default:''"`
	ElementType string    `gorm:"column:element_type;type:varchar(64);not null;default:''"`
	Action      string    `gorm:"column:action;type:varchar(64);not null;default:''"`
	DurationMs  int       `gorm:"column:duration_ms;not null;default:0"`
	CreatedAt   time.Time `gorm:"column:created_at"`
}

// TableName returns the physical table name.
func (ExecutionLogModel) TableName() string { return "execution_log" }

// AIAuditLogModel is a single AI call audit entry.
type AIAuditLogModel struct {
	ID           string    `gorm:"column:id;type:varchar(64);primaryKey"`
	InstanceID   string    `gorm:"column:instance_id;type:varchar(64);not null;index"`
	ElementID    string    `gorm:"column:element_id;type:varchar(191);not null;default:''"`
	Model        string    `gorm:"column:model;type:varchar(128);not null;default:''"`
	InputText    string    `gorm:"column:input_text;type:text"`
	OutputText   string    `gorm:"column:output_text;type:text"`
	TokensIn     int       `gorm:"column:tokens_in;not null;default:0"`
	TokensOut    int       `gorm:"column:tokens_out;not null;default:0"`
	DurationMs   int       `gorm:"column:duration_ms;not null;default:0"`
	Success      bool      `gorm:"column:success;not null;default:false"`
	ErrorMessage string    `gorm:"column:error_message;type:text"`
	CreatedAt    time.Time `gorm:"column:created_at"`
}

// TableName returns the physical table name.
func (AIAuditLogModel) TableName() string { return "ai_audit_log" }

// Models returns every model managed by the store, in dependency order.
func Models() []any {
	return []any{
		&ProcessModel{},
		&InstanceModel{},
		&FlowModel{},
		&ThreadModel{},
		&JobModel{},
		&DeadLetterModel{},
		&ExecutionLogModel{},
		&AIAuditLogModel{},
	}
}
