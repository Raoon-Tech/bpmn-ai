package orm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

// Store implements store.Store on top of GORM.
type Store struct {
	db *gorm.DB
}

// New wraps an open GORM connection in a store.Store implementation.
func New(db *gorm.DB) *Store {
	return &Store{db: db}
}

// DB exposes the underlying GORM handle (used by administrative commands).
func (s *Store) DB() *gorm.DB {
	return s.db
}

// --- Process definitions ---

func (s *Store) SaveProcess(ctx context.Context, proc *bpmn.Process) error {
	def, err := json.Marshal(proc)
	if err != nil {
		return fmt.Errorf("marshal process: %w", err)
	}
	m := &ProcessModel{
		ID:         proc.ID,
		Name:       proc.Name,
		Version:    proc.Version,
		Definition: datatypes.JSON(def),
		CreatedAt:  proc.CreatedAt,
	}
	return s.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"name", "version", "definition"}),
		}).
		Create(m).Error
}

func (s *Store) GetProcess(ctx context.Context, id string) (*bpmn.Process, error) {
	var m ProcessModel
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("process %s not found", id)
	}
	if err != nil {
		return nil, err
	}
	var proc bpmn.Process
	if err := json.Unmarshal(m.Definition, &proc); err != nil {
		return nil, fmt.Errorf("unmarshal process: %w", err)
	}
	return &proc, nil
}

func (s *Store) ListProcesses(ctx context.Context) ([]*bpmn.Process, error) {
	var models []ProcessModel
	if err := s.db.WithContext(ctx).Order("created_at DESC").Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]*bpmn.Process, 0, len(models))
	for i := range models {
		var proc bpmn.Process
		if err := json.Unmarshal(models[i].Definition, &proc); err != nil {
			return nil, fmt.Errorf("unmarshal process %s: %w", models[i].ID, err)
		}
		result = append(result, &proc)
	}
	return result, nil
}

// --- Process instances ---

func (s *Store) CreateInstance(ctx context.Context, inst *store.InstanceRecord) error {
	if inst.ID == "" {
		inst.ID = uuid.New().String()
	}
	if inst.StartedAt.IsZero() {
		inst.StartedAt = time.Now()
	}
	inst.UpdatedAt = inst.StartedAt

	vars, err := encodeMap(inst.Variables)
	if err != nil {
		return fmt.Errorf("marshal variables: %w", err)
	}
	return s.db.WithContext(ctx).Create(&InstanceModel{
		ID:          inst.ID,
		ProcessID:   inst.ProcessID,
		Title:       inst.Title,
		Status:      string(inst.Status),
		CurrentUser: inst.CurrentUser,
		Variables:   vars,
		PIN:         inst.PIN,
		StartedAt:   inst.StartedAt,
		UpdatedAt:   inst.UpdatedAt,
		FinishedAt:  inst.FinishedAt,
	}).Error
}

func (s *Store) GetInstance(ctx context.Context, id string) (*store.InstanceRecord, error) {
	var m InstanceModel
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("instance %s not found", id)
	}
	if err != nil {
		return nil, err
	}
	return fromInstanceModel(&m), nil
}

func (s *Store) UpdateInstance(ctx context.Context, inst *store.InstanceRecord) error {
	vars, err := encodeMap(inst.Variables)
	if err != nil {
		return fmt.Errorf("marshal variables: %w", err)
	}
	inst.UpdatedAt = time.Now()
	return s.db.WithContext(ctx).Model(&InstanceModel{}).
		Where("id = ?", inst.ID).
		Updates(map[string]any{
			"title":        inst.Title,
			"status":       string(inst.Status),
			"current_user": inst.CurrentUser,
			"variables":    vars,
			"pin":          inst.PIN,
			"updated_at":   inst.UpdatedAt,
			"finished_at":  inst.FinishedAt,
		}).Error
}

func (s *Store) ListInstances(ctx context.Context, status store.InstanceStatus) ([]*store.InstanceRecord, error) {
	var models []InstanceModel
	q := s.db.WithContext(ctx).Order("started_at DESC")
	if status != "" {
		q = q.Where("status = ?", string(status))
	}
	if err := q.Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]*store.InstanceRecord, 0, len(models))
	for i := range models {
		result = append(result, fromInstanceModel(&models[i]))
	}
	return result, nil
}

func fromInstanceModel(m *InstanceModel) *store.InstanceRecord {
	inst := &store.InstanceRecord{
		ID:          m.ID,
		ProcessID:   m.ProcessID,
		Title:       m.Title,
		Status:      store.InstanceStatus(m.Status),
		CurrentUser: m.CurrentUser,
		PIN:         m.PIN,
		StartedAt:   m.StartedAt,
		UpdatedAt:   m.UpdatedAt,
		FinishedAt:  m.FinishedAt,
	}
	inst.Variables = decodeMap(m.Variables)
	return inst
}

// --- Flow records ---

func (s *Store) CreateFlow(ctx context.Context, flow *store.FlowRecord) error {
	if flow.ID == "" {
		flow.ID = uuid.New().String()
	}
	now := time.Now()
	flow.StartedAt = &now
	return s.db.WithContext(ctx).Create(toFlowModel(flow)).Error
}

func (s *Store) UpdateFlow(ctx context.Context, flow *store.FlowRecord) error {
	return s.db.WithContext(ctx).Model(&FlowModel{}).
		Where("id = ?", flow.ID).
		Updates(map[string]any{
			"element_id":   flow.ElementID,
			"element_type": string(flow.ElementType),
			"thread_id":    flow.ThreadID,
			"previous_id":  flow.PreviousID,
			"status":       string(flow.Status),
			"started_at":   flow.StartedAt,
			"finished_at":  flow.FinishedAt,
			"duration_ms":  flow.DurationMs,
		}).Error
}

func (s *Store) GetFlow(ctx context.Context, id string) (*store.FlowRecord, error) {
	var m FlowModel
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("flow %s not found", id)
	}
	if err != nil {
		return nil, err
	}
	return fromFlowModel(&m), nil
}

func (s *Store) GetFlowsByInstance(ctx context.Context, instanceID string) ([]*store.FlowRecord, error) {
	var models []FlowModel
	if err := s.db.WithContext(ctx).
		Where("instance_id = ?", instanceID).
		Order("started_at").
		Find(&models).Error; err != nil {
		return nil, err
	}
	return fromFlowModels(models), nil
}

func (s *Store) GetActiveFlowsByThread(ctx context.Context, instanceID string, threadID int) ([]*store.FlowRecord, error) {
	var models []FlowModel
	if err := s.db.WithContext(ctx).
		Where("instance_id = ? AND thread_id = ? AND status = ?", instanceID, threadID, string(store.FlowStatusActive)).
		Order("started_at").
		Find(&models).Error; err != nil {
		return nil, err
	}
	return fromFlowModels(models), nil
}

func toFlowModel(f *store.FlowRecord) *FlowModel {
	return &FlowModel{
		ID:          f.ID,
		InstanceID:  f.InstanceID,
		ElementID:   f.ElementID,
		ElementType: string(f.ElementType),
		ThreadID:    f.ThreadID,
		PreviousID:  f.PreviousID,
		Status:      string(f.Status),
		StartedAt:   f.StartedAt,
		FinishedAt:  f.FinishedAt,
		DurationMs:  f.DurationMs,
	}
}

func fromFlowModel(m *FlowModel) *store.FlowRecord {
	return &store.FlowRecord{
		ID:          m.ID,
		InstanceID:  m.InstanceID,
		ElementID:   m.ElementID,
		ElementType: bpmn.ElementType(m.ElementType),
		ThreadID:    m.ThreadID,
		PreviousID:  m.PreviousID,
		Status:      store.FlowStatus(m.Status),
		StartedAt:   m.StartedAt,
		FinishedAt:  m.FinishedAt,
		DurationMs:  m.DurationMs,
	}
}

func fromFlowModels(models []FlowModel) []*store.FlowRecord {
	result := make([]*store.FlowRecord, 0, len(models))
	for i := range models {
		result = append(result, fromFlowModel(&models[i]))
	}
	return result
}

// --- Threads ---

func (s *Store) CreateThread(ctx context.Context, thread *store.ThreadRecord) error {
	if thread.CreatedAt.IsZero() {
		thread.CreatedAt = time.Now()
	}
	m := &ThreadModel{
		InstanceID:  thread.InstanceID,
		ThreadIndex: thread.ThreadIndex,
		ParentIndex: thread.ParentIndex,
		FlowID:      thread.FlowID,
		Status:      thread.Status,
		CreatedAt:   thread.CreatedAt,
	}
	err := s.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "instance_id"}, {Name: "thread_index"}},
			DoUpdates: clause.AssignmentColumns([]string{"flow_id", "status"}),
		}).
		Create(m).Error
	if err != nil {
		return err
	}
	if m.ID != 0 {
		thread.ID = m.ID
		return nil
	}
	// MySQL does not report the id on ON DUPLICATE KEY UPDATE.
	var id int
	if err := s.db.WithContext(ctx).Model(&ThreadModel{}).
		Select("id").
		Where("instance_id = ? AND thread_index = ?", thread.InstanceID, thread.ThreadIndex).
		Scan(&id).Error; err == nil {
		thread.ID = id
	}
	return nil
}

func (s *Store) UpdateThread(ctx context.Context, thread *store.ThreadRecord) error {
	return s.db.WithContext(ctx).Model(&ThreadModel{}).
		Where("instance_id = ? AND thread_index = ?", thread.InstanceID, thread.ThreadIndex).
		Updates(map[string]any{"flow_id": thread.FlowID, "status": thread.Status}).Error
}

func (s *Store) GetThreadsByInstance(ctx context.Context, instanceID string) ([]*store.ThreadRecord, error) {
	var models []ThreadModel
	if err := s.db.WithContext(ctx).
		Where("instance_id = ?", instanceID).
		Order("thread_index").
		Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]*store.ThreadRecord, 0, len(models))
	for i := range models {
		m := &models[i]
		result = append(result, &store.ThreadRecord{
			ID:          m.ID,
			InstanceID:  m.InstanceID,
			ThreadIndex: m.ThreadIndex,
			ParentIndex: m.ParentIndex,
			FlowID:      m.FlowID,
			Status:      m.Status,
			CreatedAt:   m.CreatedAt,
		})
	}
	return result, nil
}

func (s *Store) CloseThread(ctx context.Context, instanceID string, threadIndex int) error {
	return s.db.WithContext(ctx).Model(&ThreadModel{}).
		Where("instance_id = ? AND thread_index = ?", instanceID, threadIndex).
		Update("status", "CLOSED").Error
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
	payload, err := encodeMap(job.Payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	return s.db.WithContext(ctx).Create(&JobModel{
		ID:           job.ID,
		InstanceID:   job.InstanceID,
		FlowID:       job.FlowID,
		Type:         job.Type,
		Payload:      payload,
		Status:       string(job.Status),
		RetryCount:   job.RetryCount,
		MaxRetries:   job.MaxRetries,
		ScheduledAt:  job.ScheduledAt,
		ExecutedAt:   job.ExecutedAt,
		ErrorMessage: job.ErrorMessage,
		CreatedAt:    job.CreatedAt,
	}).Error
}

func (s *Store) UpdateJob(ctx context.Context, job *store.JobRecord) error {
	payload, err := encodeMap(job.Payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	return s.db.WithContext(ctx).Model(&JobModel{}).
		Where("id = ?", job.ID).
		Updates(map[string]any{
			"type":          job.Type,
			"payload":       payload,
			"status":        string(job.Status),
			"retry_count":   job.RetryCount,
			"max_retries":   job.MaxRetries,
			"scheduled_at":  job.ScheduledAt,
			"executed_at":   job.ExecutedAt,
			"error_message": job.ErrorMessage,
		}).Error
}

func (s *Store) GetPendingJobs(ctx context.Context, limit int) ([]*store.JobRecord, error) {
	var models []JobModel
	if err := s.db.WithContext(ctx).
		Where("status = ? AND scheduled_at <= ?", string(store.JobStatusPending), time.Now()).
		Order("scheduled_at").
		Limit(limit).
		Find(&models).Error; err != nil {
		return nil, err
	}
	return fromJobModels(models), nil
}

func (s *Store) GetJob(ctx context.Context, id string) (*store.JobRecord, error) {
	var m JobModel
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("job %s not found", id)
	}
	if err != nil {
		return nil, err
	}
	return fromJobModel(&m), nil
}

func fromJobModel(m *JobModel) *store.JobRecord {
	return &store.JobRecord{
		ID:           m.ID,
		InstanceID:   m.InstanceID,
		FlowID:       m.FlowID,
		Type:         m.Type,
		Payload:      decodeMap(m.Payload),
		Status:       store.JobStatus(m.Status),
		RetryCount:   m.RetryCount,
		MaxRetries:   m.MaxRetries,
		ScheduledAt:  m.ScheduledAt,
		ExecutedAt:   m.ExecutedAt,
		ErrorMessage: m.ErrorMessage,
		CreatedAt:    m.CreatedAt,
	}
}

func fromJobModels(models []JobModel) []*store.JobRecord {
	result := make([]*store.JobRecord, 0, len(models))
	for i := range models {
		result = append(result, fromJobModel(&models[i]))
	}
	return result
}

// --- Dead letter queue ---

func (s *Store) CreateDeadLetter(ctx context.Context, dl *store.DeadLetterRecord) error {
	if dl.ID == "" {
		dl.ID = uuid.New().String()
	}
	if dl.CreatedAt.IsZero() {
		dl.CreatedAt = time.Now()
	}
	payload, err := encodeMap(dl.Payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	return s.db.WithContext(ctx).Create(&DeadLetterModel{
		ID:           dl.ID,
		JobID:        dl.JobID,
		InstanceID:   dl.InstanceID,
		Type:         dl.Type,
		Payload:      payload,
		ErrorMessage: dl.ErrorMessage,
		RetryCount:   dl.RetryCount,
		CreatedAt:    dl.CreatedAt,
	}).Error
}

func (s *Store) GetDeadLetters(ctx context.Context, instanceID string) ([]*store.DeadLetterRecord, error) {
	var models []DeadLetterModel
	if err := s.db.WithContext(ctx).
		Where("instance_id = ?", instanceID).
		Order("created_at").
		Find(&models).Error; err != nil {
		return nil, err
	}
	return fromDeadLetterModels(models), nil
}

func (s *Store) GetDeadLetter(ctx context.Context, id string) (*store.DeadLetterRecord, error) {
	var m DeadLetterModel
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("dead letter %s not found", id)
	}
	if err != nil {
		return nil, err
	}
	return fromDeadLetterModel(&m), nil
}

func (s *Store) ListDeadLetters(ctx context.Context, limit int) ([]*store.DeadLetterRecord, error) {
	var models []DeadLetterModel
	if err := s.db.WithContext(ctx).
		Order("created_at").
		Limit(limit).
		Find(&models).Error; err != nil {
		return nil, err
	}
	return fromDeadLetterModels(models), nil
}

func fromDeadLetterModel(m *DeadLetterModel) *store.DeadLetterRecord {
	return &store.DeadLetterRecord{
		ID:           m.ID,
		JobID:        m.JobID,
		InstanceID:   m.InstanceID,
		Type:         m.Type,
		Payload:      decodeMap(m.Payload),
		ErrorMessage: m.ErrorMessage,
		RetryCount:   m.RetryCount,
		CreatedAt:    m.CreatedAt,
	}
}

func fromDeadLetterModels(models []DeadLetterModel) []*store.DeadLetterRecord {
	result := make([]*store.DeadLetterRecord, 0, len(models))
	for i := range models {
		result = append(result, fromDeadLetterModel(&models[i]))
	}
	return result
}

// --- Execution log ---

func (s *Store) LogExecution(ctx context.Context, entry *store.ExecutionLogEntry) error {
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	return s.db.WithContext(ctx).Create(&ExecutionLogModel{
		ID:          entry.ID,
		InstanceID:  entry.InstanceID,
		ElementID:   entry.ElementID,
		ElementType: string(entry.ElementType),
		Action:      entry.Action,
		DurationMs:  entry.DurationMs,
		CreatedAt:   entry.CreatedAt,
	}).Error
}

func (s *Store) GetExecutionLog(ctx context.Context, instanceID string) ([]*store.ExecutionLogEntry, error) {
	var models []ExecutionLogModel
	if err := s.db.WithContext(ctx).
		Where("instance_id = ?", instanceID).
		Order("created_at").
		Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]*store.ExecutionLogEntry, 0, len(models))
	for i := range models {
		m := &models[i]
		result = append(result, &store.ExecutionLogEntry{
			ID:          m.ID,
			InstanceID:  m.InstanceID,
			ElementID:   m.ElementID,
			ElementType: bpmn.ElementType(m.ElementType),
			Action:      m.Action,
			DurationMs:  m.DurationMs,
			CreatedAt:   m.CreatedAt,
		})
	}
	return result, nil
}

// --- AI audit log ---

func (s *Store) LogAICall(ctx context.Context, entry *store.AIAuditLogEntry) error {
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	return s.db.WithContext(ctx).Create(&AIAuditLogModel{
		ID:           entry.ID,
		InstanceID:   entry.InstanceID,
		ElementID:    entry.ElementID,
		Model:        entry.Model,
		InputText:    entry.InputText,
		OutputText:   entry.OutputText,
		TokensIn:     entry.TokensIn,
		TokensOut:    entry.TokensOut,
		DurationMs:   entry.DurationMs,
		Success:      entry.Success,
		ErrorMessage: entry.ErrorMessage,
		CreatedAt:    entry.CreatedAt,
	}).Error
}

// --- JSON helpers ---

func encodeMap(v map[string]any) (datatypes.JSON, error) {
	if v == nil {
		return datatypes.JSON([]byte("{}")), nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if len(b) == 0 || string(b) == "null" {
		return datatypes.JSON([]byte("{}")), nil
	}
	return datatypes.JSON(b), nil
}

func decodeMap(data datatypes.JSON) map[string]any {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}
