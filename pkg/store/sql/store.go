package sql

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

// Store is a PostgreSQL-backed implementation of store.Store.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a new PostgreSQL store using a connection pool.
func NewStore(ctx context.Context, databaseURL string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse db config: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect to db: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Close closes the connection pool.
func (s *Store) Close() {
	s.pool.Close()
}

// Pool returns the underlying pgxpool for migration tools.
func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

// --- Process definitions ---

func (s *Store) SaveProcess(ctx context.Context, proc *bpmn.Process) error {
	def, err := json.Marshal(proc)
	if err != nil {
		return fmt.Errorf("marshal process: %w", err)
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO processes (id, name, version, definition, created_at) 
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (id) DO UPDATE SET name = $2, version = $3, definition = $4`,
		proc.ID, proc.Name, proc.Version, def, proc.CreatedAt,
	)
	return err
}

func (s *Store) GetProcess(ctx context.Context, id string) (*bpmn.Process, error) {
	var def []byte
	err := s.pool.QueryRow(ctx,
		`SELECT definition FROM processes WHERE id = $1`, id,
	).Scan(&def)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("process %s not found", id)
	}
	if err != nil {
		return nil, err
	}
	var proc bpmn.Process
	if err := json.Unmarshal(def, &proc); err != nil {
		return nil, fmt.Errorf("unmarshal process: %w", err)
	}
	return &proc, nil
}

func (s *Store) ListProcesses(ctx context.Context) ([]*bpmn.Process, error) {
	rows, err := s.pool.Query(ctx, `SELECT definition FROM processes ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*bpmn.Process
	for rows.Next() {
		var def []byte
		if err := rows.Scan(&def); err != nil {
			return nil, err
		}
		var proc bpmn.Process
		if err := json.Unmarshal(def, &proc); err != nil {
			return nil, fmt.Errorf("unmarshal process: %w", err)
		}
		result = append(result, &proc)
	}
	return result, rows.Err()
}

// --- Process instances ---

func (s *Store) CreateInstance(ctx context.Context, inst *store.InstanceRecord) error {
	vars, err := json.Marshal(inst.Variables)
	if err != nil {
		return fmt.Errorf("marshal variables: %w", err)
	}
	if inst.ID == "" {
		inst.ID = uuid.New().String()
	}
	if inst.StartedAt.IsZero() {
		inst.StartedAt = time.Now()
	}
	inst.UpdatedAt = inst.StartedAt
	_, err = s.pool.Exec(ctx,
		`INSERT INTO instances (id, process_id, title, status, current_user, variables, pin, started_at, updated_at, finished_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		inst.ID, inst.ProcessID, inst.Title, string(inst.Status), inst.CurrentUser,
		vars, inst.PIN, inst.StartedAt, inst.UpdatedAt, inst.FinishedAt,
	)
	return err
}

func (s *Store) GetInstance(ctx context.Context, id string) (*store.InstanceRecord, error) {
	var (
		statusStr, currentUser, pin string
		vars                        []byte
	)
	inst := &store.InstanceRecord{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, process_id, title, status, current_user, variables, pin, started_at, updated_at, finished_at
		 FROM instances WHERE id = $1`, id,
	).Scan(&inst.ID, &inst.ProcessID, &inst.Title, &statusStr, &currentUser,
		&vars, &pin, &inst.StartedAt, &inst.UpdatedAt, &inst.FinishedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("instance %s not found", id)
	}
	if err != nil {
		return nil, err
	}
	inst.Status = store.InstanceStatus(statusStr)
	inst.CurrentUser = currentUser
	inst.PIN = pin
	if vars != nil {
		if err := json.Unmarshal(vars, &inst.Variables); err != nil {
			return nil, fmt.Errorf("unmarshal variables: %w", err)
		}
	}
	return inst, nil
}

func (s *Store) UpdateInstance(ctx context.Context, inst *store.InstanceRecord) error {
	vars, err := json.Marshal(inst.Variables)
	if err != nil {
		return fmt.Errorf("marshal variables: %w", err)
	}
	inst.UpdatedAt = time.Now()
	_, err = s.pool.Exec(ctx,
		`UPDATE instances SET title=$1, status=$2, current_user=$3, variables=$4, pin=$5, updated_at=$6, finished_at=$7
		 WHERE id=$8`,
		inst.Title, string(inst.Status), inst.CurrentUser, vars, inst.PIN,
		inst.UpdatedAt, inst.FinishedAt, inst.ID,
	)
	return err
}

func (s *Store) ListInstances(ctx context.Context, status store.InstanceStatus) ([]*store.InstanceRecord, error) {
	var rows pgx.Rows
	var err error
	if status == "" {
		rows, err = s.pool.Query(ctx, `SELECT id, process_id, title, status, current_user, variables, pin, started_at, updated_at, finished_at FROM instances ORDER BY started_at DESC`)
	} else {
		rows, err = s.pool.Query(ctx,
			`SELECT id, process_id, title, status, current_user, variables, pin, started_at, updated_at, finished_at FROM instances WHERE status = $1 ORDER BY started_at DESC`,
			string(status),
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*store.InstanceRecord
	for rows.Next() {
		var (
			statusStr, currentUser, pin string
			vars                        []byte
		)
		inst := &store.InstanceRecord{}
		if err := rows.Scan(&inst.ID, &inst.ProcessID, &inst.Title, &statusStr, &currentUser,
			&vars, &pin, &inst.StartedAt, &inst.UpdatedAt, &inst.FinishedAt); err != nil {
			return nil, err
		}
		inst.Status = store.InstanceStatus(statusStr)
		inst.CurrentUser = currentUser
		inst.PIN = pin
		if vars != nil {
			if err := json.Unmarshal(vars, &inst.Variables); err != nil {
				return nil, fmt.Errorf("unmarshal variables: %w", err)
			}
		}
		result = append(result, inst)
	}
	return result, rows.Err()
}

// --- Flow records ---

func (s *Store) CreateFlow(ctx context.Context, flow *store.FlowRecord) error {
	if flow.ID == "" {
		flow.ID = uuid.New().String()
	}
	now := time.Now()
	flow.StartedAt = &now
	_, err := s.pool.Exec(ctx,
		`INSERT INTO flows (id, instance_id, element_id, element_type, thread_id, previous_id, status, started_at, finished_at, duration_ms)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		flow.ID, flow.InstanceID, flow.ElementID, string(flow.ElementType),
		flow.ThreadID, flow.PreviousID, string(flow.Status), flow.StartedAt,
		flow.FinishedAt, flow.DurationMs,
	)
	return err
}

func (s *Store) UpdateFlow(ctx context.Context, flow *store.FlowRecord) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE flows SET element_id=$1, element_type=$2, thread_id=$3, previous_id=$4, status=$5, started_at=$6, finished_at=$7, duration_ms=$8
		 WHERE id=$9`,
		flow.ElementID, string(flow.ElementType), flow.ThreadID, flow.PreviousID,
		string(flow.Status), flow.StartedAt, flow.FinishedAt, flow.DurationMs, flow.ID,
	)
	return err
}

func (s *Store) GetFlow(ctx context.Context, id string) (*store.FlowRecord, error) {
	var statusStr, elemTypeStr string
	flow := &store.FlowRecord{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, instance_id, element_id, element_type, thread_id, previous_id, status, started_at, finished_at, duration_ms
		 FROM flows WHERE id = $1`, id,
	).Scan(&flow.ID, &flow.InstanceID, &flow.ElementID, &elemTypeStr, &flow.ThreadID,
		&flow.PreviousID, &statusStr, &flow.StartedAt, &flow.FinishedAt, &flow.DurationMs)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("flow %s not found", id)
	}
	if err != nil {
		return nil, err
	}
	flow.Status = store.FlowStatus(statusStr)
	flow.ElementType = bpmn.ElementType(elemTypeStr)
	return flow, nil
}

func (s *Store) GetFlowsByInstance(ctx context.Context, instanceID string) ([]*store.FlowRecord, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, instance_id, element_id, element_type, thread_id, previous_id, status, started_at, finished_at, duration_ms
		 FROM flows WHERE instance_id = $1 ORDER BY started_at`, instanceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFlows(rows)
}

func (s *Store) GetActiveFlowsByThread(ctx context.Context, instanceID string, threadID int) ([]*store.FlowRecord, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, instance_id, element_id, element_type, thread_id, previous_id, status, started_at, finished_at, duration_ms
		 FROM flows WHERE instance_id = $1 AND thread_id = $2 AND status = 'ACTIVE' ORDER BY started_at`,
		instanceID, threadID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFlows(rows)
}

func scanFlows(rows pgx.Rows) ([]*store.FlowRecord, error) {
	var result []*store.FlowRecord
	for rows.Next() {
		var statusStr, elemTypeStr string
		flow := &store.FlowRecord{}
		if err := rows.Scan(&flow.ID, &flow.InstanceID, &flow.ElementID, &elemTypeStr,
			&flow.ThreadID, &flow.PreviousID, &statusStr, &flow.StartedAt,
			&flow.FinishedAt, &flow.DurationMs); err != nil {
			return nil, err
		}
		flow.Status = store.FlowStatus(statusStr)
		flow.ElementType = bpmn.ElementType(elemTypeStr)
		result = append(result, flow)
	}
	return result, rows.Err()
}

// --- Threads ---

func (s *Store) CreateThread(ctx context.Context, thread *store.ThreadRecord) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO threads (instance_id, thread_index, parent_index, flow_id, status, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (instance_id, thread_index) DO UPDATE SET flow_id=$4, status=$5`,
		thread.InstanceID, thread.ThreadIndex, thread.ParentIndex,
		thread.FlowID, thread.Status, thread.CreatedAt,
	)
	return err
}

func (s *Store) UpdateThread(ctx context.Context, thread *store.ThreadRecord) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE threads SET flow_id=$1, status=$2 WHERE instance_id=$3 AND thread_index=$4`,
		thread.FlowID, thread.Status, thread.InstanceID, thread.ThreadIndex,
	)
	return err
}

func (s *Store) GetThreadsByInstance(ctx context.Context, instanceID string) ([]*store.ThreadRecord, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT instance_id, thread_index, parent_index, flow_id, status, created_at
		 FROM threads WHERE instance_id = $1 ORDER BY thread_index`, instanceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*store.ThreadRecord
	for rows.Next() {
		t := &store.ThreadRecord{}
		if err := rows.Scan(&t.InstanceID, &t.ThreadIndex, &t.ParentIndex,
			&t.FlowID, &t.Status, &t.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, rows.Err()
}

func (s *Store) CloseThread(ctx context.Context, instanceID string, threadIndex int) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE threads SET status='CLOSED' WHERE instance_id=$1 AND thread_index=$2`,
		instanceID, threadIndex,
	)
	return err
}

// --- Jobs ---

func (s *Store) CreateJob(ctx context.Context, job *store.JobRecord) error {
	if job.ID == "" {
		job.ID = uuid.New().String()
	}
	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now()
	}
	if job.ScheduledAt.IsZero() {
		job.ScheduledAt = job.CreatedAt
	}
	payload, err := json.Marshal(job.Payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO jobs (id, instance_id, flow_id, type, payload, status, retry_count, max_retries, scheduled_at, executed_at, error_message, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		job.ID, job.InstanceID, job.FlowID, job.Type, payload, string(job.Status),
		job.RetryCount, job.MaxRetries, job.ScheduledAt, job.ExecutedAt,
		job.ErrorMessage, job.CreatedAt,
	)
	return err
}

func (s *Store) UpdateJob(ctx context.Context, job *store.JobRecord) error {
	payload, err := json.Marshal(job.Payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	_, err = s.pool.Exec(ctx,
		`UPDATE jobs SET type=$1, payload=$2, status=$3, retry_count=$4, max_retries=$5, scheduled_at=$6, executed_at=$7, error_message=$8
		 WHERE id=$9`,
		job.Type, payload, string(job.Status), job.RetryCount, job.MaxRetries,
		job.ScheduledAt, job.ExecutedAt, job.ErrorMessage, job.ID,
	)
	return err
}

func (s *Store) GetPendingJobs(ctx context.Context, limit int) ([]*store.JobRecord, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, instance_id, flow_id, type, payload, status, retry_count, max_retries, scheduled_at, executed_at, error_message, created_at
		 FROM jobs WHERE status = 'PENDING' AND scheduled_at <= NOW()
		 ORDER BY scheduled_at LIMIT $1`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanJobs(rows)
}

func (s *Store) GetJob(ctx context.Context, id string) (*store.JobRecord, error) {
	job := &store.JobRecord{}
	var statusStr string
	var payload []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, instance_id, flow_id, type, payload, status, retry_count, max_retries, scheduled_at, executed_at, error_message, created_at
		 FROM jobs WHERE id = $1`, id,
	).Scan(&job.ID, &job.InstanceID, &job.FlowID, &job.Type, &payload, &statusStr,
		&job.RetryCount, &job.MaxRetries, &job.ScheduledAt, &job.ExecutedAt,
		&job.ErrorMessage, &job.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("job %s not found", id)
	}
	if err != nil {
		return nil, err
	}
	job.Status = store.JobStatus(statusStr)
	if payload != nil {
		if err := json.Unmarshal(payload, &job.Payload); err != nil {
			return nil, fmt.Errorf("unmarshal payload: %w", err)
		}
	}
	return job, nil
}

func scanJobs(rows pgx.Rows) ([]*store.JobRecord, error) {
	var result []*store.JobRecord
	for rows.Next() {
		var statusStr string
		var payload []byte
		job := &store.JobRecord{}
		if err := rows.Scan(&job.ID, &job.InstanceID, &job.FlowID, &job.Type, &payload,
			&statusStr, &job.RetryCount, &job.MaxRetries, &job.ScheduledAt,
			&job.ExecutedAt, &job.ErrorMessage, &job.CreatedAt); err != nil {
			return nil, err
		}
		job.Status = store.JobStatus(statusStr)
		if payload != nil {
			if err := json.Unmarshal(payload, &job.Payload); err != nil {
				return nil, fmt.Errorf("unmarshal payload: %w", err)
			}
		}
		result = append(result, job)
	}
	return result, rows.Err()
}

// --- Dead letter queue ---

func (s *Store) CreateDeadLetter(ctx context.Context, dl *store.DeadLetterRecord) error {
	if dl.ID == "" {
		dl.ID = uuid.New().String()
	}
	if dl.CreatedAt.IsZero() {
		dl.CreatedAt = time.Now()
	}
	payload, err := json.Marshal(dl.Payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO dead_letters (id, job_id, instance_id, type, payload, error_message, retry_count, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		dl.ID, dl.JobID, dl.InstanceID, dl.Type, payload, dl.ErrorMessage,
		dl.RetryCount, dl.CreatedAt,
	)
	return err
}

func (s *Store) GetDeadLetters(ctx context.Context, instanceID string) ([]*store.DeadLetterRecord, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, job_id, instance_id, type, payload, error_message, retry_count, created_at
		 FROM dead_letters WHERE instance_id = $1 ORDER BY created_at`, instanceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDeadLetters(rows)
}

func (s *Store) GetDeadLetter(ctx context.Context, id string) (*store.DeadLetterRecord, error) {
	var payload []byte
	dl := &store.DeadLetterRecord{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, job_id, instance_id, type, payload, error_message, retry_count, created_at
		 FROM dead_letters WHERE id = $1`, id,
	).Scan(&dl.ID, &dl.JobID, &dl.InstanceID, &dl.Type, &payload,
		&dl.ErrorMessage, &dl.RetryCount, &dl.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("dead letter %s not found", id)
	}
	if err != nil {
		return nil, err
	}
	if payload != nil {
		if err := json.Unmarshal(payload, &dl.Payload); err != nil {
			return nil, fmt.Errorf("unmarshal payload: %w", err)
		}
	}
	return dl, nil
}

func (s *Store) ListDeadLetters(ctx context.Context, limit int) ([]*store.DeadLetterRecord, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, job_id, instance_id, type, payload, error_message, retry_count, created_at
		 FROM dead_letters ORDER BY created_at LIMIT $1`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDeadLetters(rows)
}

func scanDeadLetters(rows pgx.Rows) ([]*store.DeadLetterRecord, error) {
	var result []*store.DeadLetterRecord
	for rows.Next() {
		var payload []byte
		dl := &store.DeadLetterRecord{}
		if err := rows.Scan(&dl.ID, &dl.JobID, &dl.InstanceID, &dl.Type, &payload,
			&dl.ErrorMessage, &dl.RetryCount, &dl.CreatedAt); err != nil {
			return nil, err
		}
		if payload != nil {
			if err := json.Unmarshal(payload, &dl.Payload); err != nil {
				return nil, fmt.Errorf("unmarshal payload: %w", err)
			}
		}
		result = append(result, dl)
	}
	return result, rows.Err()
}

// --- Execution log ---

func (s *Store) LogExecution(ctx context.Context, entry *store.ExecutionLogEntry) error {
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO execution_log (id, instance_id, element_id, element_type, action, duration_ms, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		entry.ID, entry.InstanceID, entry.ElementID, string(entry.ElementType),
		entry.Action, entry.DurationMs, entry.CreatedAt,
	)
	return err
}

func (s *Store) GetExecutionLog(ctx context.Context, instanceID string) ([]*store.ExecutionLogEntry, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, instance_id, element_id, element_type, action, duration_ms, created_at
		 FROM execution_log WHERE instance_id = $1 ORDER BY created_at`, instanceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*store.ExecutionLogEntry
	for rows.Next() {
		var elemTypeStr string
		e := &store.ExecutionLogEntry{}
		if err := rows.Scan(&e.ID, &e.InstanceID, &e.ElementID, &elemTypeStr,
			&e.Action, &e.DurationMs, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.ElementType = bpmn.ElementType(elemTypeStr)
		result = append(result, e)
	}
	return result, rows.Err()
}

func (s *Store) LogAICall(ctx context.Context, entry *store.AIAuditLogEntry) error {
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO ai_audit_log (id, instance_id, element_id, model, input_text, output_text,
		 tokens_in, tokens_out, duration_ms, success, error_message, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		entry.ID, entry.InstanceID, entry.ElementID, entry.Model,
		entry.InputText, entry.OutputText,
		entry.TokensIn, entry.TokensOut, entry.DurationMs,
		entry.Success, entry.ErrorMessage, entry.CreatedAt,
	)
	return err
}
