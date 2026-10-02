package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Raoon-Tech/bpmn-ai/internal/element/activities"
	"github.com/Raoon-Tech/bpmn-ai/internal/element/events"
	"github.com/Raoon-Tech/bpmn-ai/internal/element/gateways"
	"github.com/Raoon-Tech/bpmn-ai/internal/engine"
	"github.com/Raoon-Tech/bpmn-ai/internal/observability"
	"github.com/Raoon-Tech/bpmn-ai/internal/process"
	"github.com/Raoon-Tech/bpmn-ai/internal/queue"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store/memory"
)

func main() {
	// Logger
	logger, err := observability.NewFromConfig("debug", "text")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creando logger: %v\n", err)
		os.Exit(1)
	}

	// Audit log — ahora usa un directorio y crea un archivo .log por instancia
	auditDir := "data/audit_logs"

	dispatcher := observability.NewDispatcher()
	auditWriter, err := observability.NewFileAuditWriter(auditDir, true, logger)
	if err != nil {
		logger.Error("error creando audit writer", "error", err)
		os.Exit(1)
	}
	_ = observability.NewAuditor(dispatcher, auditWriter)
	logger.Info("audit log habilitado", "dir", auditDir)

	// Parsear BPMN
	xmlData, err := os.ReadFile("examples/order-process.bpmn")
	if err != nil {
		logger.Error("error leyendo BPMN XML", "error", err)
		os.Exit(1)
	}

	parser := bpmn.NewParser()
	proc, err := parser.Parse(xmlData)
	if err != nil {
		logger.Error("error parseando BPMN", "error", err)
		os.Exit(1)
	}
	proc.Name = "Loan Processing Process"
	logger.Info("proceso BPMN parseado",
		"id", proc.ID, "name", proc.Name,
		"elementos", len(proc.Elements),
		"flujos", len(proc.Flows))

	// Store
	store := memory.NewStore()
	if err := store.SaveProcess(context.Background(), proc); err != nil {
		logger.Error("error guardando proceso", "error", err)
		os.Exit(1)
	}

	// Worker pool
	retry := queue.DefaultRetryPolicy()
	dlq := queue.NewDeadLetterQueue(store, store).WithDispatcher(dispatcher)
	q := queue.NewWorkerPool(store, nil, retry, dlq, queue.WorkerPoolConfig{
		Concurrency:  2,
		PollInterval: 2 * time.Second,
	}).WithDispatcher(dispatcher)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.Start(ctx)

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

	eng := engine.New(engine.Config{
		WorkerCount:      4,
		MaxLoops:         100,
		ExecutionTimeout: 30 * time.Second,
	}, registry, store, logger, q).WithDispatcher(dispatcher)

	// Escenario 1: Loan APROBADO (docs ok + credit score ok)
	runScenario(eng, store, proc, "Escenario 1: Loan APROBADO", map[string]any{
		"documentsComplete":   true,
		"creditScoreApproved": true,
	})

	// Escenario 2: Loan RECHAZADO (mal credit score)
	runScenario(eng, store, proc, "Escenario 2: Loan RECHAZADO", map[string]any{
		"documentsComplete":   true,
		"creditScoreApproved": false,
	})

	q.Stop()

	// Cerrar audit writer
	time.Sleep(50 * time.Millisecond)
	auditWriter.Close()

	// Mostrar audit logs
	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("  AUDIT LOGS POR INSTANCIA")
	fmt.Println(strings.Repeat("=", 60))

	entries, err := os.ReadDir(auditDir)
	if err != nil {
		logger.Error("error leyendo directorio audit", "error", err)
		os.Exit(1)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "audit_") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(auditDir, entry.Name()))
		if err != nil {
			continue
		}
		fmt.Printf("\n📄 %s\n", entry.Name())
		fmt.Println(strings.Repeat("-", 60))
		fmt.Println(string(data))
	}

	fmt.Println("\n✅ Ejemplo completado exitosamente.")
}

func runScenario(
	eng engine.Engine,
	store *memory.Store,
	proc *bpmn.Process,
	title string,
	variables map[string]any,
) {
	instance := process.NewInstance(proc, variables)
	instance.Title = title

	if err := store.CreateInstance(context.Background(), instance.ToRecord()); err != nil {
		fmt.Fprintf(os.Stderr, "error creando instancia: %v\n", err)
		return
	}

	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("  %s\n", title)
	fmt.Println(strings.Repeat("=", 60))

	execCtx, execCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer execCancel()

	start := time.Now()
	err := eng.Run(execCtx, instance)
	elapsed := time.Since(start)

	if err != nil {
		fmt.Printf("  Error: %v\n", err)
	}

	fmt.Printf("  Estado: %s (en %v)\n", instance.State, elapsed)

	logs, _ := store.GetExecutionLog(context.Background(), instance.ID)
	fmt.Printf("  %-30s %-15s %s\n", "ELEMENTO", "TIPO", "ACCION")
	for _, entry := range logs {
		fmt.Printf("  %-30s %-15s %s\n", entry.ElementID, entry.ElementType, entry.Action)
	}
}
