package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/Raoon-Tech/bpmn-ai/internal/element/activities"
	"github.com/Raoon-Tech/bpmn-ai/internal/element/events"
	"github.com/Raoon-Tech/bpmn-ai/internal/element/gateways"
	"github.com/Raoon-Tech/bpmn-ai/internal/engine"
	observability "github.com/Raoon-Tech/bpmn-ai/internal/observability"
	"github.com/Raoon-Tech/bpmn-ai/internal/process"
	"github.com/Raoon-Tech/bpmn-ai/internal/queue"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store/memory"
)

var (
	storeInstance  store.Store
	engineInstance engine.Engine
	auditWriter    *observability.FileAuditWriter
	dispatcher     *observability.Dispatcher
	logger         *observability.Logger
	workerPool     *queue.WorkerPool
)

func main() {
	logger = createLogger()
	storeInstance = memory.NewStore()
	dispatcher = observability.NewDispatcher()

	// Audit log — escribe un archivo .log por instancia dentro del directorio
	var err error
	auditWriter, err = observability.NewFileAuditWriter("./data/audit_logs", true, logger)
	if err != nil {
		logger.Error("error creando audit writer", "error", err)
		os.Exit(1)
	}
	defer auditWriter.Close()
	_ = observability.NewAuditor(dispatcher, auditWriter)

	// Worker pool for ServiceTask
	retry := queue.DefaultRetryPolicy()
	dlq := queue.NewDeadLetterQueue(storeInstance, storeInstance).WithDispatcher(dispatcher)
	workerPool = queue.NewWorkerPool(storeInstance, nil, retry, dlq, queue.WorkerPoolConfig{
		Concurrency:  2,
		PollInterval: 2 * time.Second,
	}).WithDispatcher(dispatcher)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	workerPool.Start(ctx)

	// Engine
	registry := engine.NewElementRegistry()
	registry.Register(bpmn.ElementTypeStartEvent, events.NewStartEvent)
	registry.Register(bpmn.ElementTypeEndEvent, events.NewEndEvent)
	registry.Register(bpmn.ElementTypeUserTask, activities.NewUserTask)
	registry.Register(bpmn.ElementTypeServiceTask, activities.NewServiceTask)
	registry.Register(bpmn.ElementTypeScriptTask, activities.NewScriptTask)
	registry.Register(bpmn.ElementTypeExclusiveGateway, gateways.NewExclusiveGateway)
	registry.Register(bpmn.ElementTypeParallelGateway, gateways.NewParallelGateway)
	registry.Register(bpmn.ElementTypeInclusiveGateway, gateways.NewInclusiveGateway)

	engineInstance = engine.New(engine.Config{
		WorkerCount:      4,
		MaxLoops:         100,
		ExecutionTimeout: 30 * time.Second,
	}, registry, storeInstance, logger, workerPool).WithDispatcher(dispatcher)

	// HTTP server with chi
	r := chi.NewRouter()
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/processes", createProcess)
		r.Get("/processes", listProcesses)
		r.Get("/processes/{id}", getProcess)
		r.Post("/processes/{id}/start", startCase)

		r.Get("/cases", listCases)
		r.Get("/cases/{id}", getCase)
		r.Get("/cases/{id}/tasks", getCaseTasks)
		r.Get("/cases/{id}/history", getCaseHistory)

		r.Post("/tasks/{id}/complete", completeTask)

		r.Get("/audit", getAuditLog)
	})

	port := "8080"
	if p := os.Getenv("PORT"); p != "" {
		port = p
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	go func() {
		logger.Info("API server escuchando", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
		}
	}()

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	logger.Info("shutting down...")
	workerPool.Stop()
	auditWriter.Close()
	logger.Info("server stopped")
}

func createLogger() *observability.Logger {
	l, err := observability.NewFromConfig("debug", "text")
	if err != nil {
		log.Fatalf("error creando logger: %v", err)
	}
	return l
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func createProcess(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		BPMNXML string `json:"bpmn_xml"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "cuerpo invalido")
		return
	}
	if req.Name == "" || req.BPMNXML == "" {
		writeError(w, http.StatusBadRequest, "name y bpmn_xml son requeridos")
		return
	}

	parser := bpmn.NewParser()
	proc, err := parser.Parse([]byte(req.BPMNXML))
	if err != nil {
		writeError(w, http.StatusBadRequest, "XML BPMN invalido: "+err.Error())
		return
	}
	proc.Name = req.Name

	if err := storeInstance.SaveProcess(r.Context(), proc); err != nil {
		writeError(w, http.StatusInternalServerError, "error guardando proceso")
		return
	}

	logger.Info("proceso creado", "id", proc.ID, "name", proc.Name)
	writeJSON(w, http.StatusCreated, map[string]any{"id": proc.ID, "name": proc.Name})
}

func listProcesses(w http.ResponseWriter, r *http.Request) {
	procs, err := storeInstance.ListProcesses(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error listando procesos")
		return
	}
	result := make([]map[string]any, 0, len(procs))
	for _, p := range procs {
		result = append(result, map[string]any{"id": p.ID, "name": p.Name})
	}
	writeJSON(w, http.StatusOK, result)
}

func getProcess(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	proc, err := storeInstance.GetProcess(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "proceso no encontrado")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": proc.ID, "name": proc.Name})
}

func startCase(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req struct {
		Title     string         `json:"title"`
		Variables map[string]any `json:"variables"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	proc, err := storeInstance.GetProcess(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "proceso no encontrado")
		return
	}

	instance := process.NewInstance(proc, req.Variables)
	if req.Title != "" {
		instance.Title = req.Title
	}

	if err := storeInstance.CreateInstance(r.Context(), instance.ToRecord()); err != nil {
		writeError(w, http.StatusInternalServerError, "error creando caso")
		return
	}

	// Ejecutar el engine en background
	go func() {
		execCtx, execCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer execCancel()
		if err := engineInstance.Run(execCtx, instance); err != nil {
			if instance.State != process.StateWaiting {
				logger.Error("error ejecutando proceso", "error", err, "case_id", instance.ID)
			}
		}
		storeInstance.UpdateInstance(context.Background(), instance.ToRecord())
	}()

	logger.Info("caso iniciado", "case_id", instance.ID, "status", instance.State)
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":     instance.ID,
		"status": instance.State,
	})
}

func listCases(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	cases, err := storeInstance.ListInstances(r.Context(), store.InstanceStatus(status))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error listando casos")
		return
	}
	result := make([]map[string]any, 0, len(cases))
	for _, c := range cases {
		result = append(result, map[string]any{
			"id":     c.ID,
			"process_id": c.ProcessID,
			"title":  c.Title,
			"status": string(c.Status),
		})
	}
	writeJSON(w, http.StatusOK, result)
}

func getCase(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	c, err := storeInstance.GetInstance(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "caso no encontrado")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":        c.ID,
		"process_id": c.ProcessID,
		"title":     c.Title,
		"status":    string(c.Status),
		"variables": c.Variables,
	})
}

func getCaseTasks(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	flows, err := storeInstance.GetFlowsByInstance(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error obteniendo tareas")
		return
	}
	tasks := make([]map[string]any, 0)
	for _, f := range flows {
		if f.ElementType == bpmn.ElementTypeUserTask && f.Status == store.FlowStatusActive {
			tasks = append(tasks, map[string]any{
				"flow_id":    f.ID,
				"element_id": f.ElementID,
				"status":     string(f.Status),
				"thread_id":  f.ThreadID,
				"started_at": f.StartedAt,
			})
		}
	}
	writeJSON(w, http.StatusOK, tasks)
}

func getCaseHistory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	logs, err := storeInstance.GetExecutionLog(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error obteniendo history")
		return
	}
	writeJSON(w, http.StatusOK, logs)
}

func completeTask(w http.ResponseWriter, r *http.Request) {
	flowID := chi.URLParam(r, "id")

	var req struct {
		Variables map[string]any `json:"variables"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	flow, err := storeInstance.GetFlow(r.Context(), flowID)
	if err != nil {
		writeError(w, http.StatusNotFound, "tarea no encontrada")
		return
	}
	if flow.Status != store.FlowStatusActive {
		writeError(w, http.StatusConflict, "la tarea no esta activa")
		return
	}

	// Marcar flow como completado
	now := time.Now()
	flow.Status = store.FlowStatusCompleted
	flow.FinishedAt = &now
	d := int(time.Since(*flow.StartedAt).Milliseconds())
	flow.DurationMs = &d
	storeInstance.UpdateFlow(r.Context(), flow)

	// Continuar ejecucion en background usando el engine
	go func() {
		execCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := engineInstance.Continue(execCtx, flow.InstanceID, flowID, req.Variables); err != nil {
			logger.Error("engine continuation failed", "error", err, "instance_id", flow.InstanceID)
		}
	}()

	writeJSON(w, http.StatusOK, map[string]any{
		"task_id": flowID,
		"status":  "COMPLETED",
	})
}

func getAuditLog(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir("./data/audit_logs")
	if err != nil {
		writeJSON(w, http.StatusOK, []string{})
		return
	}
	var logs []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "audit_") {
			continue
		}
		data, err := os.ReadFile(filepath.Join("./data/audit_logs", entry.Name()))
		if err != nil {
			continue
		}
		logs = append(logs, fmt.Sprintf("=== %s ===\n%s", entry.Name(), string(data)))
	}
	writeJSON(w, http.StatusOK, logs)
}
