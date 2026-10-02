package http

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/Raoon-Tech/bpmn-ai/api/middleware"
	"github.com/Raoon-Tech/bpmn-ai/internal/process"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

const bpmnParseTimeout = 30 * time.Second

type createProcessRequest struct {
	Name    string `json:"name"`
	BPMNXML string `json:"bpmn_xml"`
}

type startCaseRequest struct {
	Title     string         `json:"title"`
	Variables map[string]any `json:"variables"`
}

type claimTaskRequest struct {
	UserID string `json:"user_id"`
}

type completeTaskRequest struct {
	Variables map[string]any `json:"variables"`
}

func (s *Server) createProcess(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)

	var req createProcessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.BPMNXML == "" {
		writeError(w, http.StatusBadRequest, "bpmn_xml is required")
		return
	}

	parser := bpmn.NewParser()
	parseCtx, parseCancel := context.WithTimeout(r.Context(), bpmnParseTimeout)
	defer parseCancel()
	done := make(chan *bpmn.Process, 1)
	var parseErr error
	go func() {
		p, err := parser.Parse([]byte(req.BPMNXML))
		if err != nil {
			parseErr = err
			done <- nil
			return
		}
		done <- p
	}()
	var proc *bpmn.Process
	select {
	case proc = <-done:
		if proc == nil {
			s.logger.Error("bpmn parse error", "error", parseErr)
			writeError(w, http.StatusBadRequest, "invalid BPMN XML format")
			return
		}
	case <-parseCtx.Done():
		writeError(w, http.StatusRequestTimeout, "BPMN parsing timed out")
		return
	}
	proc.Name = req.Name

	if err := s.store.SaveProcess(r.Context(), proc); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save process")
		return
	}

	s.logger.Info("process created", "process_id", proc.ID, "name", proc.Name)
	writeCreated(w, map[string]any{
		"id":   proc.ID,
		"name": proc.Name,
	})
}

func (s *Server) listProcesses(w http.ResponseWriter, r *http.Request) {
	procs, err := s.store.ListProcesses(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list processes")
		return
	}

	result := make([]map[string]any, 0, len(procs))
	for _, p := range procs {
		result = append(result, map[string]any{
			"id":   p.ID,
			"name": p.Name,
		})
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) getProcess(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" || !isValidUUID(id) {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	proc, err := s.store.GetProcess(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "process not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":   proc.ID,
		"name": proc.Name,
	})
}

func (s *Server) startCase(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)

	id := chi.URLParam(r, "id")
	if id == "" || !isValidUUID(id) {
		writeError(w, http.StatusBadRequest, "process id is required")
		return
	}

	var req startCaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.Variables) > 100 {
		writeError(w, http.StatusBadRequest, "too many variables (max 100)")
		return
	}

	proc, err := s.store.GetProcess(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "process not found")
		return
	}

	instance := process.NewInstance(proc, req.Variables)
	if req.Title != "" {
		instance.Title = req.Title
	}

	if err := s.store.CreateInstance(r.Context(), instance.ToRecord()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create case")
		return
	}

	if s.engine != nil {
		go func() {
			execCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := s.engine.Run(execCtx, instance); err != nil {
				s.logger.Error("engine execution failed", "error", err, "instance_id", instance.ID)
			}
		}()
	}

	s.logger.Info("case started", "case_id", instance.ID, "process_id", id)
	writeCreated(w, map[string]any{
		"id":     instance.ID,
		"status": instance.State,
	})
}

func (s *Server) listCases(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	cases, err := s.store.ListInstances(r.Context(), store.InstanceStatus(status))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list cases")
		return
	}

	result := make([]map[string]any, 0, len(cases))
	for _, c := range cases {
		result = append(result, map[string]any{
			"id":         c.ID,
			"process_id": c.ProcessID,
			"title":      c.Title,
			"status":     string(c.Status),
		})
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) getCase(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" || !isValidUUID(id) {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	c, err := s.store.GetInstance(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "case not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":         c.ID,
		"process_id": c.ProcessID,
		"title":      c.Title,
		"status":     string(c.Status),
		"variables":  c.Variables,
	})
}

func (s *Server) getCaseTasks(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" || !isValidUUID(id) {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	flows, err := s.store.GetFlowsByInstance(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get tasks")
		return
	}

	tasks := make([]map[string]any, 0)
	for _, f := range flows {
		if f.ElementType == bpmn.ElementTypeUserTask && f.Status == store.FlowStatusActive {
			tasks = append(tasks, map[string]any{
				"flow_id":      f.ID,
				"element_id":   f.ElementID,
				"status":       string(f.Status),
				"thread_id":    f.ThreadID,
				"started_at":   f.StartedAt,
				"duration_ms":  f.DurationMs,
			})
		}
	}

	writeJSON(w, http.StatusOK, tasks)
}

func (s *Server) getCaseHistory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" || !isValidUUID(id) {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	_, err := s.store.GetInstance(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "case not found")
		return
	}

	log, err := s.store.GetExecutionLog(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get history")
		return
	}

	writeJSON(w, http.StatusOK, log)
}

func (s *Server) getCaseDiagram(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" || !isValidUUID(id) {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	c, err := s.store.GetInstance(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "case not found")
		return
	}

	proc, err := s.store.GetProcess(r.Context(), c.ProcessID)
	if err != nil {
		writeError(w, http.StatusNotFound, "process not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"case_id":    c.ID,
		"process_id": proc.ID,
		"elements":   len(proc.Elements),
		"flows":      len(proc.Flows),
	})
}

func (s *Server) claimTask(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)

	id := chi.URLParam(r, "id")
	if id == "" || !isValidUUID(id) {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	var req claimTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.UserID == "" {
		writeError(w, http.StatusBadRequest, "user_id is required")
		return
	}

	flow, err := s.store.GetFlow(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	if flow.Status != store.FlowStatusActive {
		writeError(w, http.StatusConflict, "task is not available for claiming")
		return
	}

	flow.Status = store.FlowStatusActive
	if err := s.store.UpdateFlow(r.Context(), flow); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to claim task")
		return
	}

	s.logger.Info("task claimed", "task_id", id, "user_id", req.UserID)
	writeJSON(w, http.StatusOK, map[string]any{
		"task_id":    id,
		"claimed_by": req.UserID,
		"status":     string(flow.Status),
	})
}

func (s *Server) completeTask(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)

	id := chi.URLParam(r, "id")
	if id == "" || !isValidUUID(id) {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	var req completeTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.Variables) > 100 {
		writeError(w, http.StatusBadRequest, "too many variables (max 100)")
		return
	}

	flow, err := s.store.GetFlow(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	if flow.Status != store.FlowStatusActive {
		writeError(w, http.StatusConflict, "task is not active")
		return
	}

	now := time.Now()
	flow.Status = store.FlowStatusCompleted
	flow.FinishedAt = &now
	d := int(time.Since(*flow.StartedAt).Milliseconds())
	flow.DurationMs = &d

	if err := s.store.UpdateFlow(r.Context(), flow); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to complete task")
		return
	}

	if s.engine != nil {
		go func() {
			execCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := s.engine.Continue(execCtx, flow.InstanceID, flow.ID, req.Variables); err != nil {
				s.logger.Error("engine continuation failed", "error", err, "instance_id", flow.InstanceID)
			}
		}()
	}

	s.logger.Info("task completed", "task_id", id)
	writeJSON(w, http.StatusOK, map[string]any{
		"task_id": id,
		"status":  string(flow.Status),
	})
}

type sendMessageRequest struct {
	InstanceID string         `json:"instance_id"`
	MessageRef string         `json:"message_ref"`
	Variables  map[string]any `json:"variables,omitempty"`
}

func (s *Server) sendMessage(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)

	var req sendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.InstanceID == "" || !isValidUUID(req.InstanceID) {
		writeError(w, http.StatusBadRequest, "valid instance_id is required")
		return
	}
	if req.MessageRef == "" {
		writeError(w, http.StatusBadRequest, "message_ref is required")
		return
	}
	if len(req.Variables) > 100 {
		writeError(w, http.StatusBadRequest, "too many variables (max 100)")
		return
	}

	if s.engine != nil {
		go func() {
			execCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := s.engine.SendMessage(execCtx, req.InstanceID, req.MessageRef, req.Variables); err != nil {
				s.logger.Error("message delivery failed", "error", err, "instance_id", req.InstanceID, "message_ref", req.MessageRef)
			}
		}()
	}

	s.logger.Info("message sent", "instance_id", req.InstanceID, "message_ref", req.MessageRef)
	writeJSON(w, http.StatusOK, map[string]any{
		"instance_id": req.InstanceID,
		"message_ref": req.MessageRef,
		"status":      "delivered",
	})
}

type sendSignalRequest struct {
	SignalRef string         `json:"signal_ref"`
	Variables map[string]any `json:"variables,omitempty"`
}

func (s *Server) sendSignal(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)

	var req sendSignalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.SignalRef == "" {
		writeError(w, http.StatusBadRequest, "signal_ref is required")
		return
	}
	if len(req.Variables) > 100 {
		writeError(w, http.StatusBadRequest, "too many variables (max 100)")
		return
	}

	if s.engine != nil {
		go func() {
			execCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if _, err := s.engine.SendSignal(execCtx, req.SignalRef, req.Variables); err != nil {
				s.logger.Error("signal broadcast failed", "error", err, "signal_ref", req.SignalRef)
			}
		}()
	}

	s.logger.Info("signal sent", "signal_ref", req.SignalRef)
	writeJSON(w, http.StatusOK, map[string]any{
		"signal_ref": req.SignalRef,
		"status":     "broadcast",
	})
}

func (s *Server) getCSRFToken(w http.ResponseWriter, r *http.Request) {
	token, err := middleware.GenerateCSRFToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	middleware.SetCSRFCookie(w, token)
	writeJSON(w, http.StatusOK, map[string]any{
		"csrf_token": token,
	})
}

func isValidUUID(id string) bool {
	_, err := uuid.Parse(id)
	return err == nil
}
