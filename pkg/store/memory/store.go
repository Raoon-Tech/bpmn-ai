package memory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

// Store is an in-memory implementation of store.Store for testing.
type Store struct {
	mu          sync.RWMutex
	processes   map[string]*bpmn.Process
	instances   map[string]*store.InstanceRecord
	flows       map[string]*store.FlowRecord
	threads     map[string]map[int]*store.ThreadRecord
	jobs        map[string]*store.JobRecord
	deadLetters map[string]*store.DeadLetterRecord
	logs        []*store.ExecutionLogEntry
	aiLogs      []*store.AIAuditLogEntry

	threadSeq int
}

// NewStore creates a new in-memory store.
func NewStore() *Store {
	return &Store{
		processes:   make(map[string]*bpmn.Process),
		instances:   make(map[string]*store.InstanceRecord),
		flows:       make(map[string]*store.FlowRecord),
		threads:     make(map[string]map[int]*store.ThreadRecord),
		jobs:        make(map[string]*store.JobRecord),
		deadLetters: make(map[string]*store.DeadLetterRecord),
		logs:        make([]*store.ExecutionLogEntry, 0),
	}
}

func (s *Store) SaveProcess(_ context.Context, proc *bpmn.Process) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.processes[proc.ID] = proc
	return nil
}

func (s *Store) GetProcess(_ context.Context, id string) (*bpmn.Process, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.processes[id]
	if !ok {
		return nil, fmt.Errorf("process %s not found", id)
	}
	return p, nil
}

func (s *Store) ListProcesses(_ context.Context) ([]*bpmn.Process, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*bpmn.Process, 0, len(s.processes))
	for _, p := range s.processes {
		result = append(result, p)
	}
	return result, nil
}

func (s *Store) DeleteProcess(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.processes[id]; !ok {
		return fmt.Errorf("process %s not found", id)
	}
	delete(s.processes, id)
	return nil
}

func (s *Store) CreateInstance(ctx context.Context, inst *store.InstanceRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if inst.ID == "" {
		inst.ID = uuid.New().String()
	}
	if inst.TenantID == "" {
		inst.TenantID = store.TenantFromContext(ctx)
	}
	inst.StartedAt = time.Now()
	inst.UpdatedAt = inst.StartedAt
	s.instances[inst.ID] = inst
	return nil
}

func (s *Store) GetInstance(ctx context.Context, id string) (*store.InstanceRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	i, ok := s.instances[id]
	if !ok {
		return nil, fmt.Errorf("instance %s not found", id)
	}
	if tenant := store.TenantFromContext(ctx); tenant != "" && i.TenantID != "" && i.TenantID != tenant {
		return nil, fmt.Errorf("instance %s not found", id)
	}
	return i, nil
}

func (s *Store) UpdateInstance(_ context.Context, inst *store.InstanceRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	inst.UpdatedAt = time.Now()
	s.instances[inst.ID] = inst
	return nil
}

func (s *Store) ListInstances(ctx context.Context, status store.InstanceStatus) ([]*store.InstanceRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tenant := store.TenantFromContext(ctx)
	var result []*store.InstanceRecord
	for _, i := range s.instances {
		if tenant != "" && i.TenantID != "" && i.TenantID != tenant {
			continue
		}
		if status == "" || i.Status == status {
			result = append(result, i)
		}
	}
	return result, nil
}

func (s *Store) CreateFlow(_ context.Context, flow *store.FlowRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if flow.ID == "" {
		flow.ID = uuid.New().String()
	}
	now := time.Now()
	flow.StartedAt = &now
	s.flows[flow.ID] = flow
	return nil
}

func (s *Store) UpdateFlow(_ context.Context, flow *store.FlowRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flows[flow.ID] = flow
	return nil
}

func (s *Store) GetFlow(_ context.Context, id string) (*store.FlowRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, ok := s.flows[id]
	if !ok {
		return nil, fmt.Errorf("flow %s not found", id)
	}
	return f, nil
}

func (s *Store) GetFlowsByInstance(_ context.Context, instanceID string) ([]*store.FlowRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*store.FlowRecord
	for _, f := range s.flows {
		if f.InstanceID == instanceID {
			result = append(result, f)
		}
	}
	return result, nil
}

func (s *Store) GetActiveFlowsByThread(_ context.Context, instanceID string, threadID int) ([]*store.FlowRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*store.FlowRecord
	for _, f := range s.flows {
		if f.InstanceID == instanceID && f.ThreadID == threadID && f.Status == store.FlowStatusActive {
			result = append(result, f)
		}
	}
	return result, nil
}

func (s *Store) CreateThread(_ context.Context, thread *store.ThreadRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.threadSeq++
	thread.ID = s.threadSeq
	thread.CreatedAt = time.Now()

	if _, ok := s.threads[thread.InstanceID]; !ok {
		s.threads[thread.InstanceID] = make(map[int]*store.ThreadRecord)
	}
	s.threads[thread.InstanceID][thread.ThreadIndex] = thread
	return nil
}

func (s *Store) UpdateThread(_ context.Context, thread *store.ThreadRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if instanceThreads, ok := s.threads[thread.InstanceID]; ok {
		instanceThreads[thread.ThreadIndex] = thread
	}
	return nil
}

func (s *Store) GetThreadsByInstance(_ context.Context, instanceID string) ([]*store.ThreadRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	instanceThreads, ok := s.threads[instanceID]
	if !ok {
		return nil, nil
	}
	result := make([]*store.ThreadRecord, 0, len(instanceThreads))
	for _, t := range instanceThreads {
		result = append(result, t)
	}
	return result, nil
}

func (s *Store) CloseThread(_ context.Context, instanceID string, threadIndex int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if instanceThreads, ok := s.threads[instanceID]; ok {
		if t, ok := instanceThreads[threadIndex]; ok {
			t.Status = "CLOSED"
		}
	}
	return nil
}

func (s *Store) CreateJob(_ context.Context, job *store.JobRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job.ID == "" {
		job.ID = uuid.New().String()
	}
	job.CreatedAt = time.Now()
	if job.ScheduledAt.IsZero() {
		job.ScheduledAt = job.CreatedAt
	}
	s.jobs[job.ID] = job
	return nil
}

func (s *Store) UpdateJob(_ context.Context, job *store.JobRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.ID] = job
	return nil
}

func (s *Store) GetPendingJobs(_ context.Context, limit int) ([]*store.JobRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now()
	var result []*store.JobRecord
	for _, j := range s.jobs {
		if j.Status == store.JobStatusPending && !j.ScheduledAt.After(now) {
			result = append(result, j)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (s *Store) GetJob(_ context.Context, id string) (*store.JobRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	j, ok := s.jobs[id]
	if !ok {
		return nil, fmt.Errorf("job %s not found", id)
	}
	return j, nil
}

func (s *Store) CreateDeadLetter(_ context.Context, dl *store.DeadLetterRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if dl.ID == "" {
		dl.ID = uuid.New().String()
	}
	dl.CreatedAt = time.Now()
	s.deadLetters[dl.ID] = dl
	return nil
}

func (s *Store) GetDeadLetters(_ context.Context, instanceID string) ([]*store.DeadLetterRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*store.DeadLetterRecord
	for _, dl := range s.deadLetters {
		if dl.InstanceID == instanceID {
			result = append(result, dl)
		}
	}
	return result, nil
}

func (s *Store) GetDeadLetter(_ context.Context, id string) (*store.DeadLetterRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	dl, ok := s.deadLetters[id]
	if !ok {
		return nil, fmt.Errorf("dead letter %s not found", id)
	}
	return dl, nil
}

func (s *Store) ListDeadLetters(_ context.Context, limit int) ([]*store.DeadLetterRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*store.DeadLetterRecord
	for _, dl := range s.deadLetters {
		result = append(result, dl)
		if len(result) >= limit {
			break
		}
	}
	return result, nil
}

func (s *Store) LogExecution(_ context.Context, entry *store.ExecutionLogEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	entry.CreatedAt = time.Now()
	s.logs = append(s.logs, entry)
	return nil
}

func (s *Store) LogAICall(_ context.Context, entry *store.AIAuditLogEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	entry.CreatedAt = time.Now()
	s.aiLogs = append(s.aiLogs, entry)
	return nil
}

func (s *Store) GetExecutionLog(_ context.Context, instanceID string) ([]*store.ExecutionLogEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*store.ExecutionLogEntry
	for _, e := range s.logs {
		if e.InstanceID == instanceID {
			result = append(result, e)
		}
	}
	return result, nil
}

// Reset clears all data in the store.
func (s *Store) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.processes = make(map[string]*bpmn.Process)
	s.instances = make(map[string]*store.InstanceRecord)
	s.flows = make(map[string]*store.FlowRecord)
	s.threads = make(map[string]map[int]*store.ThreadRecord)
	s.jobs = make(map[string]*store.JobRecord)
	s.deadLetters = make(map[string]*store.DeadLetterRecord)
	s.logs = make([]*store.ExecutionLogEntry, 0)
	s.aiLogs = make([]*store.AIAuditLogEntry, 0)
	s.threadSeq = 0
}
