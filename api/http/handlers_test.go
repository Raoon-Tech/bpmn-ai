package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Raoon-Tech/bpmn-ai/internal/engine"
	"github.com/Raoon-Tech/bpmn-ai/internal/observability"
	"github.com/Raoon-Tech/bpmn-ai/internal/process"
	"github.com/Raoon-Tech/bpmn-ai/internal/queue"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store/memory"
)

const testUUID = "123e4567-e89b-12d3-a456-426614174000"

type noopEngine struct{}

func (noopEngine) Registry() *engine.ElementRegistry                                  { return nil }
func (noopEngine) Run(_ context.Context, _ *process.Instance) error                    { return nil }
func (noopEngine) Continue(_ context.Context, _, _ string, _ map[string]any) error     { return nil }
func (noopEngine) SendMessage(_ context.Context, _, _ string, _ map[string]any) error  { return nil }
func (noopEngine) SendSignal(_ context.Context, _ string, _ map[string]any) ([]string, error) { return nil, nil }
func (n noopEngine) WithDispatcher(_ *observability.Dispatcher) engine.Engine          { return n }
func (noopEngine) JobHandler() queue.JobHandler                                          { return nil }

func newTestServer(t *testing.T) *Server {
	t.Helper()

	store := memory.NewStore()
	logger, _ := observability.NewFromConfig("error", "text")
	metrics := observability.NewMetrics()
	retry := queue.DefaultRetryPolicy()
	dlq := queue.NewDeadLetterQueue(store, store)
	q := queue.NewWorkerPool(store, nil, retry, dlq, queue.WorkerPoolConfig{
		Concurrency:  1,
		PollInterval: 5 * time.Second,
	})

	return NewServer(ServerConfig{
		Host:         "127.0.0.1",
		Port:         0,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
		DisableCSRF:  true,
	}, store, noopEngine{}, q, logger, metrics)
}

func TestHealthCheck(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp healthResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "ok" {
		t.Errorf("expected status ok, got %s", resp.Status)
	}
}

func TestReadinessCheck(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()

	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestCreateProcess(t *testing.T) {
	srv := newTestServer(t)

	body := createProcessRequest{
		Name: "Test Process",
		BPMNXML: `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" targetNamespace="test">
  <process id="123e4567-e89b-12d3-a456-426614174000" name="Test">
    <startEvent id="start-1"/>
    <endEvent id="end-1"/>
    <sequenceFlow id="flow-1" sourceRef="start-1" targetRef="end-1"/>
  </process>
</definitions>`,
	}

	data, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/processes", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["id"] == "" {
		t.Error("expected process ID")
	}
}

func TestCreateProcess_Invalid(t *testing.T) {
	srv := newTestServer(t)

	tests := []struct {
		name string
		body any
		want int
	}{
		{"empty body", nil, http.StatusBadRequest},
		{"no name", createProcessRequest{BPMNXML: "not xml"}, http.StatusBadRequest},
		{"no xml", createProcessRequest{Name: "test"}, http.StatusBadRequest},
		{"invalid xml", createProcessRequest{Name: "test", BPMNXML: "not xml"}, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var data []byte
			if tt.body != nil {
				data, _ = json.Marshal(tt.body)
			}

			req := httptest.NewRequest(http.MethodPost, "/api/v1/processes", bytes.NewReader(data))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			srv.Router().ServeHTTP(rec, req)

			if rec.Code != tt.want {
				t.Errorf("expected %d, got %d: %s", tt.want, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestListProcesses(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/processes", nil)
	rec := httptest.NewRecorder()

	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var procs []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&procs); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(procs) != 0 {
		t.Errorf("expected 0 processes, got %d", len(procs))
	}
}

func TestStartCase(t *testing.T) {
	srv := newTestServer(t)

	body := createProcessRequest{
		Name: "Test Process",
		BPMNXML: `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" targetNamespace="test">
  <process id="123e4567-e89b-12d3-a456-426614174000" name="Test">
    <startEvent id="start-1"/>
    <endEvent id="end-1"/>
    <sequenceFlow id="flow-1" sourceRef="start-1" targetRef="end-1"/>
  </process>
</definitions>`,
	}

	data, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/processes", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	var procResp map[string]any
	json.NewDecoder(rec.Body).Decode(&procResp)
	procID := procResp["id"].(string)

	startBody := startCaseRequest{
		Title:     "Test Case",
		Variables: map[string]any{"amount": 5000},
	}

	data, _ = json.Marshal(startBody)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/processes/"+procID+"/start", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()

	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var caseResp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&caseResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if caseResp["id"] == "" {
		t.Error("expected case ID")
	}
}

func TestListCases(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cases", nil)
	rec := httptest.NewRecorder()

	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestGetCase_NotFound(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cases/"+testUUID, nil)
	rec := httptest.NewRecorder()

	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestClaimTask(t *testing.T) {
	srv := newTestServer(t)
	s := srv.store.(*memory.Store)

	flow := &store.FlowRecord{
		ID:          testUUID,
		InstanceID:  "test-instance",
		ElementID:   "task-1",
		ElementType: bpmn.ElementTypeUserTask,
		ThreadID:    1,
		Status:      store.FlowStatusActive,
	}
	if err := s.CreateFlow(nil, flow); err != nil {
		t.Fatalf("create flow: %v", err)
	}

	body := claimTaskRequest{UserID: "user-1"}
	data, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/"+testUUID+"/claim", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCompleteTask(t *testing.T) {
	srv := newTestServer(t)
	s := srv.store.(*memory.Store)

	now := time.Now()
	flow := &store.FlowRecord{
		ID:          testUUID,
		InstanceID:  "test-instance",
		ElementID:   "task-1",
		ElementType: bpmn.ElementTypeUserTask,
		ThreadID:    1,
		Status:      store.FlowStatusActive,
		StartedAt:   &now,
	}
	if err := s.CreateFlow(nil, flow); err != nil {
		t.Fatalf("create flow: %v", err)
	}

	body := completeTaskRequest{Variables: map[string]any{"approved": true}}
	data, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/"+testUUID+"/complete", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetCaseHistory(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cases/"+testUUID+"/history", nil)
	rec := httptest.NewRecorder()

	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for nonexistent case, got %d", rec.Code)
	}
}

func TestGetCaseDiagram(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cases/"+testUUID+"/diagram", nil)
	rec := httptest.NewRecorder()

	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for nonexistent case, got %d", rec.Code)
	}
}

func TestGetCaseTasks(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cases/"+testUUID+"/tasks", nil)
	rec := httptest.NewRecorder()

	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestGetProcess_NotFound(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/processes/"+testUUID, nil)
	rec := httptest.NewRecorder()

	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestStartCase_ProcessNotFound(t *testing.T) {
	srv := newTestServer(t)

	body := startCaseRequest{Title: "Test"}
	data, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/processes/"+testUUID+"/start", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestCreateProcess_MissingFields(t *testing.T) {
	srv := newTestServer(t)

	tests := []struct {
		name string
		body createProcessRequest
	}{
		{"missing name", createProcessRequest{BPMNXML: "<xml/>"}},
		{"missing xml", createProcessRequest{Name: "test"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/processes", bytes.NewReader(data))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			srv.Router().ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", rec.Code)
			}
		})
	}
}

func TestClaimTask_MissingUserID(t *testing.T) {
	srv := newTestServer(t)

	body := claimTaskRequest{}
	data, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/"+testUUID+"/claim", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestCSRFTokenEndpoint(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/csrf-token", nil)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	token, ok := resp["csrf_token"].(string)
	if !ok || token == "" {
		t.Fatal("expected non-empty csrf_token in response")
	}

	cookies := rec.Result().Cookies()
	var csrfCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "csrf_token" {
			csrfCookie = c
			break
		}
	}
	if csrfCookie == nil {
		t.Fatal("expected csrf_token cookie")
	}
	if csrfCookie.Value != token {
		t.Errorf("cookie value %q != response token %q", csrfCookie.Value, token)
	}
}

func TestCSRF_RejectsPostWithoutToken(t *testing.T) {
	store := memory.NewStore()
	logger, _ := observability.NewFromConfig("error", "text")
	metrics := observability.NewMetrics()
	retry := queue.DefaultRetryPolicy()
	dlq := queue.NewDeadLetterQueue(store, store)
	q := queue.NewWorkerPool(store, nil, retry, dlq, queue.WorkerPoolConfig{
		Concurrency:  1,
		PollInterval: 5 * time.Second,
	})
	srv := NewServer(ServerConfig{
		Host:         "127.0.0.1",
		Port:         0,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}, store, noopEngine{}, q, logger, metrics)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/processes", nil)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCSRF_AcceptsPostWithValidToken(t *testing.T) {
	store := memory.NewStore()
	logger, _ := observability.NewFromConfig("error", "text")
	metrics := observability.NewMetrics()
	retry := queue.DefaultRetryPolicy()
	dlq := queue.NewDeadLetterQueue(store, store)
	q := queue.NewWorkerPool(store, nil, retry, dlq, queue.WorkerPoolConfig{
		Concurrency:  1,
		PollInterval: 5 * time.Second,
	})
	srv := NewServer(ServerConfig{
		Host:         "127.0.0.1",
		Port:         0,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}, store, noopEngine{}, q, logger, metrics)

	// First get a CSRF token
	tokenReq := httptest.NewRequest(http.MethodGet, "/api/v1/csrf-token", nil)
	tokenRec := httptest.NewRecorder()
	srv.Router().ServeHTTP(tokenRec, tokenReq)

	if tokenRec.Code != http.StatusOK {
		t.Fatalf("csrf-token endpoint: expected 200, got %d", tokenRec.Code)
	}

	var tokenResp map[string]any
	json.NewDecoder(tokenRec.Body).Decode(&tokenResp)
	csrfToken := tokenResp["csrf_token"].(string)

	cookies := tokenRec.Result().Cookies()
	var csrfCookie string
	for _, c := range cookies {
		if c.Name == "csrf_token" {
			csrfCookie = c.Value
		}
	}

	// Now make a POST with the token
	body := createProcessRequest{
		Name: "CSRF Test",
		BPMNXML: `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" targetNamespace="test">
  <process id="123e4567-e89b-12d3-a456-426614174000" name="Test">
    <startEvent id="start-1"/>
    <endEvent id="end-1"/>
    <sequenceFlow id="flow-1" sourceRef="start-1" targetRef="end-1"/>
  </process>
</definitions>`,
	}
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/processes", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfToken)
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: csrfCookie})
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRequestID_Header(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-Request-ID", "custom-id-123")
	rec := httptest.NewRecorder()

	srv.Router().ServeHTTP(rec, req)

	if rec.Header().Get("X-Request-ID") != "custom-id-123" {
		t.Errorf("expected X-Request-ID header to be custom-id-123, got %s", rec.Header().Get("X-Request-ID"))
	}
}

func TestMetricsEndpoint(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestFullProcessLifecycle(t *testing.T) {
	srv := newTestServer(t)

	body := createProcessRequest{
		Name: "Approval Process",
		BPMNXML: `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" targetNamespace="test">
  <process id="123e4567-e89b-12d3-a456-426614174001" name="Approval">
    <startEvent id="start-1"/>
    <userTask id="task-1" name="Review" assignee="manager"/>
    <endEvent id="end-1"/>
    <sequenceFlow id="flow-1" sourceRef="start-1" targetRef="task-1"/>
    <sequenceFlow id="flow-2" sourceRef="task-1" targetRef="end-1"/>
  </process>
</definitions>`,
	}

	data, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/processes", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("create process: expected 201, got %d", rec.Code)
	}

	var procResp map[string]any
	json.NewDecoder(rec.Body).Decode(&procResp)
	procID := procResp["id"].(string)

	startBody := startCaseRequest{
		Title:     "Approval Request #1",
		Variables: map[string]any{"amount": 10000},
	}
	data, _ = json.Marshal(startBody)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/processes/"+procID+"/start", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("start case: expected 201, got %d", rec.Code)
	}

	var caseResp map[string]any
	json.NewDecoder(rec.Body).Decode(&caseResp)
	caseID := caseResp["id"].(string)

	req = httptest.NewRequest(http.MethodGet, "/api/v1/cases/"+caseID, nil)
	rec = httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("get case: expected 200, got %d", rec.Code)
	}

	var caseDetail map[string]any
	json.NewDecoder(rec.Body).Decode(&caseDetail)

	if caseDetail["title"] != "Approval Request #1" {
		t.Errorf("expected title 'Approval Request #1', got %v", caseDetail["title"])
	}
}
