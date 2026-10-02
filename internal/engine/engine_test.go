package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Raoon-Tech/bpmn-ai/internal/element/activities"
	"github.com/Raoon-Tech/bpmn-ai/internal/element/events"
	"github.com/Raoon-Tech/bpmn-ai/internal/element/gateways"
	"github.com/Raoon-Tech/bpmn-ai/internal/observability"
	"github.com/Raoon-Tech/bpmn-ai/internal/process"
	"github.com/Raoon-Tech/bpmn-ai/pkg/ai"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store/memory"
)

func TestEngine_Run_SimpleSequence(t *testing.T) {
	// Build a simple process: start -> end
	proc := &bpmn.Process{
		ID:           "proc-1",
		Name:         "Simple Sequence",
		StartEventID: "start-1",
		Elements: map[string]bpmn.Element{
			"start-1": {ID: "start-1", Type: bpmn.ElementTypeStartEvent, OutgoingFlows: []string{"flow-1"}},
			"end-1":   {ID: "end-1", Type: bpmn.ElementTypeEndEvent, IncomingFlows: []string{"flow-1"}},
		},
		Flows: map[string]bpmn.Flow{
			"flow-1": {ID: "flow-1", SourceRef: "start-1", TargetRef: "end-1"},
		},
	}

	// Create registry
	registry := NewElementRegistry()
	registry.Register(bpmn.ElementTypeStartEvent, events.NewStartEvent)
	registry.Register(bpmn.ElementTypeEndEvent, events.NewEndEvent)

	// Create store and engine
	store := memory.NewStore()
	logger, _ := observability.NewFromConfig("debug", "text")
	if logger == nil {
		t.Fatal("failed to create logger")
	}

	cfg := Config{
		WorkerCount:      1,
		MaxLoops:         100,
		ExecutionTimeout: 10 * time.Second,
	}

	eng := New(cfg, registry, store, logger, nil)

	// Create instance and run
	instance := process.NewInstance(proc, nil)
	if err := store.CreateInstance(context.Background(), instance.ToRecord()); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := eng.Run(ctx, instance)
	if err != nil {
		t.Fatalf("engine run failed: %v", err)
	}

	if instance.State != process.StateCompleted {
		t.Errorf("expected state COMPLETED, got %s", instance.State)
	}

	// Verify flows were created
	flowRecs, err := store.GetFlowsByInstance(context.Background(), instance.ID)
	if err != nil {
		t.Fatalf("get flows: %v", err)
	}
	if len(flowRecs) < 2 {
		t.Errorf("expected at least 2 flow records, got %d", len(flowRecs))
	}
}

func TestEngine_Run_ParallelGateway(t *testing.T) {
	// Build process: start -> parallel(div) -> end-a, end-b -> parallel(conv) -> end
	proc := &bpmn.Process{
		ID:           "proc-parallel",
		Name:         "Parallel Process",
		StartEventID: "start-1",
		Elements: map[string]bpmn.Element{
			"start-1": {ID: "start-1", Type: bpmn.ElementTypeStartEvent, OutgoingFlows: []string{"flow-1"}},
			"gw-div":  {ID: "gw-div", Type: bpmn.ElementTypeParallelGateway, IncomingFlows: []string{"flow-1"}, OutgoingFlows: []string{"flow-2a", "flow-2b"}},
			"end-a":   {ID: "end-a", Type: bpmn.ElementTypeEndEvent, IncomingFlows: []string{"flow-2a"}},
			"end-b":   {ID: "end-b", Type: bpmn.ElementTypeEndEvent, IncomingFlows: []string{"flow-2b"}},
		},
		Flows: map[string]bpmn.Flow{
			"flow-1":  {ID: "flow-1", SourceRef: "start-1", TargetRef: "gw-div"},
			"flow-2a": {ID: "flow-2a", SourceRef: "gw-div", TargetRef: "end-a"},
			"flow-2b": {ID: "flow-2b", SourceRef: "gw-div", TargetRef: "end-b"},
		},
	}

	registry := NewElementRegistry()
	registry.Register(bpmn.ElementTypeStartEvent, events.NewStartEvent)
	registry.Register(bpmn.ElementTypeEndEvent, events.NewEndEvent)
	registry.Register(bpmn.ElementTypeParallelGateway, gateways.NewParallelGateway)

	store := memory.NewStore()
	logger, _ := observability.NewFromConfig("info", "text")

	cfg := Config{
		WorkerCount:      2,
		MaxLoops:         100,
		ExecutionTimeout: 10 * time.Second,
	}

	eng := New(cfg, registry, store, logger, nil)

	instance := process.NewInstance(proc, nil)
	if err := store.CreateInstance(context.Background(), instance.ToRecord()); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := eng.Run(ctx, instance)
	if err != nil {
		t.Fatalf("engine run failed: %v", err)
	}

	// Verify parallel branches created threads
	threads, err := store.GetThreadsByInstance(context.Background(), instance.ID)
	if err != nil {
		t.Fatalf("get threads: %v", err)
	}
	if len(threads) < 2 {
		t.Errorf("expected at least 2 threads for parallel branches, got %d", len(threads))
	}
}

func TestEngine_Run_ExclusiveGateway(t *testing.T) {
	// Build process: start -> exclusive -> end-approved, end-rejected
	proc := &bpmn.Process{
		ID:           "proc-exclusive",
		Name:         "Exclusive Process",
		StartEventID: "start-1",
		Elements: map[string]bpmn.Element{
			"start-1": {ID: "start-1", Type: bpmn.ElementTypeStartEvent, OutgoingFlows: []string{"flow-1"}},
			"gw-1":    {ID: "gw-1", Type: bpmn.ElementTypeExclusiveGateway, IncomingFlows: []string{"flow-1"}, OutgoingFlows: []string{"flow-2", "flow-3"}, DefaultFlowID: "flow-3"},
			"end-app": {ID: "end-app", Type: bpmn.ElementTypeEndEvent, IncomingFlows: []string{"flow-2"}},
			"end-rej": {ID: "end-rej", Type: bpmn.ElementTypeEndEvent, IncomingFlows: []string{"flow-3"}},
		},
		Flows: map[string]bpmn.Flow{
			"flow-1": {ID: "flow-1", SourceRef: "start-1", TargetRef: "gw-1"},
			"flow-2": {ID: "flow-2", SourceRef: "gw-1", TargetRef: "end-app"},
			"flow-3": {ID: "flow-3", SourceRef: "gw-1", TargetRef: "end-rej"},
		},
	}

	registry := NewElementRegistry()
	registry.Register(bpmn.ElementTypeStartEvent, events.NewStartEvent)
	registry.Register(bpmn.ElementTypeEndEvent, events.NewEndEvent)
	registry.Register(bpmn.ElementTypeExclusiveGateway, gateways.NewExclusiveGateway)

	store := memory.NewStore()
	logger, _ := observability.NewFromConfig("info", "text")

	cfg := Config{
		WorkerCount:      1,
		MaxLoops:         100,
		ExecutionTimeout: 10 * time.Second,
	}

	eng := New(cfg, registry, store, logger, nil)

	instance := process.NewInstance(proc, nil)
	if err := store.CreateInstance(context.Background(), instance.ToRecord()); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := eng.Run(ctx, instance)
	if err != nil {
		t.Fatalf("engine run failed: %v", err)
	}

	if instance.State != process.StateCompleted {
		t.Errorf("expected state COMPLETED, got %s", instance.State)
	}
}

func TestEngine_Run_NoStartEvent(t *testing.T) {
	proc := &bpmn.Process{
		ID:           "proc-nostart",
		Name:         "No Start",
		StartEventID: "",
		Elements:     map[string]bpmn.Element{},
		Flows:        map[string]bpmn.Flow{},
	}

	registry := NewElementRegistry()
	store := memory.NewStore()
	logger, _ := observability.NewFromConfig("info", "text")
	eng := New(Config{WorkerCount: 1}, registry, store, logger, nil)

	instance := process.NewInstance(proc, nil)
	err := eng.Run(context.Background(), instance)
	if err == nil {
		t.Fatal("expected error for process with no start event")
	}
}

func TestEngine_Run_ContextCancellation(t *testing.T) {
	proc := &bpmn.Process{
		ID:           "proc-cancel",
		Name:         "Cancel Test",
		StartEventID: "start-1",
		Elements: map[string]bpmn.Element{
			"start-1": {ID: "start-1", Type: bpmn.ElementTypeStartEvent, OutgoingFlows: []string{"flow-1"}},
			"end-1":   {ID: "end-1", Type: bpmn.ElementTypeEndEvent, IncomingFlows: []string{"flow-1"}},
		},
		Flows: map[string]bpmn.Flow{
			"flow-1": {ID: "flow-1", SourceRef: "start-1", TargetRef: "end-1"},
		},
	}

	registry := NewElementRegistry()
	registry.Register(bpmn.ElementTypeStartEvent, events.NewStartEvent)
	registry.Register(bpmn.ElementTypeEndEvent, events.NewEndEvent)

	store := memory.NewStore()
	logger, _ := observability.NewFromConfig("info", "text")
	eng := New(Config{WorkerCount: 1}, registry, store, logger, nil)

	instance := process.NewInstance(proc, nil)
	if err := store.CreateInstance(context.Background(), instance.ToRecord()); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := eng.Run(ctx, instance)
	if err != context.Canceled {
		t.Errorf("expected context.Canceled error, got %v", err)
	}
}

func TestFailSafeManager_Timeout(t *testing.T) {
	fs := NewFailSafeManager(1*time.Millisecond, 100)
	time.Sleep(5 * time.Millisecond)

	err := fs.Check("elem-1")
	if err == nil {
		t.Fatal("expected timeout error")
	}

	timeoutErr, ok := err.(*ExecutionTimeoutError)
	if !ok {
		t.Fatalf("expected ExecutionTimeoutError, got %T", err)
	}
	if timeoutErr.Elapsed < timeoutErr.Limit {
		t.Errorf("expected elapsed >= limit")
	}
}

func TestFailSafeManager_LoopCount(t *testing.T) {
	fs := NewFailSafeManager(10*time.Second, 3)

	// Execute same element 3 times (should pass)
	for i := 0; i < 3; i++ {
		if err := fs.Check("elem-1"); err != nil {
			t.Fatalf("unexpected error on iteration %d: %v", i+1, err)
		}
	}

	// 4th execution should fail
	err := fs.Check("elem-1")
	if err == nil {
		t.Fatal("expected loop count error")
	}

	loopErr, ok := err.(*NestedLoopError)
	if !ok {
		t.Fatalf("expected NestedLoopError, got %T", err)
	}
	if loopErr.Count != 4 {
		t.Errorf("expected count 4, got %d", loopErr.Count)
	}
	if loopErr.Limit != 3 {
		t.Errorf("expected limit 3, got %d", loopErr.Limit)
	}
}

func TestFailSafeManager_DifferentElements(t *testing.T) {
	fs := NewFailSafeManager(10*time.Second, 2)

	// Execute different elements - should not trigger loop limit
	for i := 0; i < 10; i++ {
		elemID := "elem-" + string(rune('a'+i))
		if err := fs.Check(elemID); err != nil {
			t.Fatalf("unexpected error for %s: %v", elemID, err)
		}
	}

	if fs.TotalExecutions() != 10 {
		t.Errorf("expected 10 total executions, got %d", fs.TotalExecutions())
	}
}

func TestFlowRouter_Route(t *testing.T) {
	router := NewFlowRouter()

	proc := &bpmn.Process{
		Elements: map[string]bpmn.Element{
			"gw-1": {ID: "gw-1", OutgoingFlows: []string{"flow-a", "flow-b"}},
			"end-a": {ID: "end-a", Type: bpmn.ElementTypeEndEvent},
			"end-b": {ID: "end-b", Type: bpmn.ElementTypeEndEvent},
		},
		Flows: map[string]bpmn.Flow{
			"flow-a": {ID: "flow-a", SourceRef: "gw-1", TargetRef: "end-a"},
			"flow-b": {ID: "flow-b", SourceRef: "gw-1", TargetRef: "end-b"},
		},
	}

	flow := &store.FlowRecord{
		ElementID: "gw-1",
		ThreadID:  1,
	}

	result := ExecutionResult{
		Action:   ActionRoute,
		FlowData: flow,
	}

	next := router.Route(result, proc, 1)
	if len(next) != 2 {
		t.Errorf("expected 2 next flows, got %d", len(next))
	}
}

func TestFlowRouter_RouteWithFilters(t *testing.T) {
	router := NewFlowRouter()

	proc := &bpmn.Process{
		Elements: map[string]bpmn.Element{
			"gw-1": {ID: "gw-1", OutgoingFlows: []string{"flow-a", "flow-b", "flow-c"}},
			"end-a": {ID: "end-a", Type: bpmn.ElementTypeEndEvent},
			"end-b": {ID: "end-b", Type: bpmn.ElementTypeEndEvent},
			"end-c": {ID: "end-c", Type: bpmn.ElementTypeEndEvent},
		},
		Flows: map[string]bpmn.Flow{
			"flow-a": {ID: "flow-a", SourceRef: "gw-1", TargetRef: "end-a"},
			"flow-b": {ID: "flow-b", SourceRef: "gw-1", TargetRef: "end-b"},
			"flow-c": {ID: "flow-c", SourceRef: "gw-1", TargetRef: "end-c"},
		},
	}

	flow := &store.FlowRecord{
		ElementID: "gw-1",
		ThreadID:  1,
	}

	result := ExecutionResult{
		Action:      ActionRoute,
		FlowData:    flow,
		FlowFilters: []string{"flow-b"},
	}

	next := router.Route(result, proc, 1)
	if len(next) != 1 {
		t.Errorf("expected 1 next flow with filter, got %d", len(next))
	}
	if len(next) > 0 && next[0].FlowID != "flow-b" {
		t.Errorf("expected flow-b, got %s", next[0].FlowID)
	}
}

func TestElementRegistry_RegisterAndGet(t *testing.T) {
	reg := NewElementRegistry()

	reg.Register(bpmn.ElementTypeStartEvent, events.NewStartEvent)

	if !reg.Has(bpmn.ElementTypeStartEvent) {
		t.Error("expected registry to have startEvent")
	}

	elemDef := bpmn.Element{ID: "start-1", Type: bpmn.ElementTypeStartEvent, Name: "Start"}
	elem, err := reg.Get(elemDef)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if elem.ID() != "start-1" {
		t.Errorf("expected ID start-1, got %s", elem.ID())
	}
	if elem.Type() != bpmn.ElementTypeStartEvent {
		t.Errorf("expected type startEvent, got %s", elem.Type())
	}
}

func TestElementRegistry_GetUnknown(t *testing.T) {
	reg := NewElementRegistry()

	elemDef := bpmn.Element{ID: "unknown-1", Type: "unknown"}
	_, err := reg.Get(elemDef)
	if err == nil {
		t.Fatal("expected error for unknown element type")
	}
}

func TestExecutionContext(t *testing.T) {
	proc := &bpmn.Process{
		ID:           "proc-1",
		StartEventID: "start-1",
		Elements: map[string]bpmn.Element{
			"start-1": {ID: "start-1", Type: bpmn.ElementTypeStartEvent},
		},
	}

	instance := process.NewInstance(proc, map[string]any{"key": "value"})
	flow := &store.FlowRecord{ElementID: "start-1"}
	store := memory.NewStore()
	logger, _ := observability.NewFromConfig("info", "text")

	ctx := NewExecutionContext(context.Background(), instance, flow, store, logger)

	// Test variable access
	val, ok := ctx.GetVariable("key")
	if !ok || val != "value" {
		t.Errorf("expected variable value, got %v", val)
	}

	// Test set variable
	ctx.SetVariable("new", 42)
	val, ok = ctx.GetVariable("new")
	if !ok || val != 42 {
		t.Errorf("expected new variable to be 42, got %v", val)
	}

	// Test element access
	elem, ok := ctx.Element()
	if !ok {
		t.Fatal("expected element")
	}
	if elem.ID != "start-1" {
		t.Errorf("expected element ID start-1, got %s", elem.ID)
	}
}

func TestEngine_Run_TerminateEvent(t *testing.T) {
	proc := &bpmn.Process{
		ID:           "proc-term",
		Name:         "Terminate Process",
		StartEventID: "start-1",
		Elements: map[string]bpmn.Element{
			"start-1": {ID: "start-1", Type: bpmn.ElementTypeStartEvent, OutgoingFlows: []string{"flow-1"}},
			"term-1":  {ID: "term-1", Type: bpmn.ElementTypeTerminateEvent, IncomingFlows: []string{"flow-1"}},
		},
		Flows: map[string]bpmn.Flow{
			"flow-1": {ID: "flow-1", SourceRef: "start-1", TargetRef: "term-1"},
		},
	}

	registry := NewElementRegistry()
	registry.Register(bpmn.ElementTypeStartEvent, events.NewStartEvent)
	registry.Register(bpmn.ElementTypeTerminateEvent, events.NewTerminateEvent)

	s := memory.NewStore()
	logger, _ := observability.NewFromConfig("info", "text")
	eng := New(Config{WorkerCount: 1, MaxLoops: 100, ExecutionTimeout: 5 * time.Second}, registry, s, logger, nil)

	instance := process.NewInstance(proc, nil)
	if err := s.CreateInstance(context.Background(), instance.ToRecord()); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := eng.Run(ctx, instance)
	if err == nil {
		t.Fatal("expected error from terminate event")
	}
	if !strings.Contains(err.Error(), "process terminated") {
		t.Errorf("expected process terminated error, got %v", err)
	}
	if instance.State != process.StateTerminated {
		t.Errorf("expected state TERMINATED, got %s", instance.State)
	}
}

func TestEngine_Run_UserTask(t *testing.T) {
	proc := &bpmn.Process{
		ID:           "proc-task",
		Name:         "User Task Process",
		StartEventID: "start-1",
		Elements: map[string]bpmn.Element{
			"start-1": {ID: "start-1", Type: bpmn.ElementTypeStartEvent, OutgoingFlows: []string{"flow-1"}},
			"task-1":  {ID: "task-1", Type: bpmn.ElementTypeUserTask, IncomingFlows: []string{"flow-1"}, OutgoingFlows: []string{"flow-2"}, Assignee: "user-1"},
			"end-1":   {ID: "end-1", Type: bpmn.ElementTypeEndEvent, IncomingFlows: []string{"flow-2"}},
		},
		Flows: map[string]bpmn.Flow{
			"flow-1": {ID: "flow-1", SourceRef: "start-1", TargetRef: "task-1"},
			"flow-2": {ID: "flow-2", SourceRef: "task-1", TargetRef: "end-1"},
		},
	}

	registry := NewElementRegistry()
	registry.Register(bpmn.ElementTypeStartEvent, events.NewStartEvent)
	registry.Register(bpmn.ElementTypeEndEvent, events.NewEndEvent)
	registry.Register(bpmn.ElementTypeUserTask, activities.NewUserTask)

	s := memory.NewStore()
	logger, _ := observability.NewFromConfig("info", "text")
	eng := New(Config{WorkerCount: 1, MaxLoops: 100, ExecutionTimeout: 5 * time.Second}, registry, s, logger, nil)

	instance := process.NewInstance(proc, nil)
	if err := s.CreateInstance(context.Background(), instance.ToRecord()); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := eng.Run(ctx, instance)
	if err != nil {
		t.Fatalf("engine run failed: %v", err)
	}
	if instance.State != process.StateWaiting {
		t.Errorf("expected state WAITING, got %s", instance.State)
	}
}

func TestEngine_Run_ScriptTask(t *testing.T) {
	proc := &bpmn.Process{
		ID:           "proc-script",
		Name:         "Script Task Process",
		StartEventID: "start-1",
		Elements: map[string]bpmn.Element{
			"start-1":  {ID: "start-1", Type: bpmn.ElementTypeStartEvent, OutgoingFlows: []string{"flow-1"}},
			"script-1": {ID: "script-1", Type: bpmn.ElementTypeScriptTask, IncomingFlows: []string{"flow-1"}, OutgoingFlows: []string{"flow-2"}},
			"end-1":    {ID: "end-1", Type: bpmn.ElementTypeEndEvent, IncomingFlows: []string{"flow-2"}},
		},
		Flows: map[string]bpmn.Flow{
			"flow-1": {ID: "flow-1", SourceRef: "start-1", TargetRef: "script-1"},
			"flow-2": {ID: "flow-2", SourceRef: "script-1", TargetRef: "end-1"},
		},
	}

	registry := NewElementRegistry()
	registry.Register(bpmn.ElementTypeStartEvent, events.NewStartEvent)
	registry.Register(bpmn.ElementTypeEndEvent, events.NewEndEvent)
	registry.Register(bpmn.ElementTypeScriptTask, activities.NewScriptTask)

	s := memory.NewStore()
	logger, _ := observability.NewFromConfig("info", "text")
	eng := New(Config{WorkerCount: 1, MaxLoops: 100, ExecutionTimeout: 5 * time.Second}, registry, s, logger, nil)

	instance := process.NewInstance(proc, nil)
	if err := s.CreateInstance(context.Background(), instance.ToRecord()); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := eng.Run(ctx, instance)
	if err != nil {
		t.Fatalf("engine run failed: %v", err)
	}
	if instance.State != process.StateCompleted {
		t.Errorf("expected state COMPLETED, got %s", instance.State)
	}
}

func TestEngine_Run_ServiceTask(t *testing.T) {
	proc := &bpmn.Process{
		ID:           "proc-svc",
		Name:         "Service Task Process",
		StartEventID: "start-1",
		Elements: map[string]bpmn.Element{
			"start-1": {ID: "start-1", Type: bpmn.ElementTypeStartEvent, OutgoingFlows: []string{"flow-1"}},
			"svc-1":   {ID: "svc-1", Type: bpmn.ElementTypeServiceTask, IncomingFlows: []string{"flow-1"}, OutgoingFlows: []string{"flow-2"}},
			"end-1":   {ID: "end-1", Type: bpmn.ElementTypeEndEvent, IncomingFlows: []string{"flow-2"}},
		},
		Flows: map[string]bpmn.Flow{
			"flow-1": {ID: "flow-1", SourceRef: "start-1", TargetRef: "svc-1"},
			"flow-2": {ID: "flow-2", SourceRef: "svc-1", TargetRef: "end-1"},
		},
	}

	registry := NewElementRegistry()
	registry.Register(bpmn.ElementTypeStartEvent, events.NewStartEvent)
	registry.Register(bpmn.ElementTypeEndEvent, events.NewEndEvent)
	registry.Register(bpmn.ElementTypeServiceTask, activities.NewServiceTask)

	s := memory.NewStore()
	logger, _ := observability.NewFromConfig("info", "text")
	eng := New(Config{WorkerCount: 1, MaxLoops: 100, ExecutionTimeout: 5 * time.Second}, registry, s, logger, nil)

	instance := process.NewInstance(proc, nil)
	if err := s.CreateInstance(context.Background(), instance.ToRecord()); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := eng.Run(ctx, instance)
	if err != nil {
		t.Fatalf("engine run failed: %v", err)
	}
	if instance.State != process.StateCompleted {
		t.Errorf("expected state COMPLETED, got %s", instance.State)
	}
}

func TestEngine_Run_ExclusiveGatewayWithConditions(t *testing.T) {
	proc := &bpmn.Process{
		ID:           "proc-cond",
		Name:         "Conditional",
		StartEventID: "start-1",
		Elements: map[string]bpmn.Element{
			"start-1": {ID: "start-1", Type: bpmn.ElementTypeStartEvent, OutgoingFlows: []string{"flow-1"}},
			"gw-1": {ID: "gw-1", Type: bpmn.ElementTypeExclusiveGateway,
				IncomingFlows: []string{"flow-1"}, OutgoingFlows: []string{"flow-2", "flow-3"},
				DefaultFlowID: "flow-3"},
			"end-app": {ID: "end-app", Type: bpmn.ElementTypeEndEvent, IncomingFlows: []string{"flow-2"}},
			"end-rej": {ID: "end-rej", Type: bpmn.ElementTypeEndEvent, IncomingFlows: []string{"flow-3"}},
		},
		Flows: map[string]bpmn.Flow{
			"flow-1": {ID: "flow-1", SourceRef: "start-1", TargetRef: "gw-1"},
			"flow-2": {ID: "flow-2", SourceRef: "gw-1", TargetRef: "end-app"},
			"flow-3": {ID: "flow-3", SourceRef: "gw-1", TargetRef: "end-rej"},
		},
	}

	registry := NewElementRegistry()
	registry.Register(bpmn.ElementTypeStartEvent, events.NewStartEvent)
	registry.Register(bpmn.ElementTypeEndEvent, events.NewEndEvent)
	registry.Register(bpmn.ElementTypeExclusiveGateway, gateways.NewExclusiveGateway)

	s := memory.NewStore()
	logger, _ := observability.NewFromConfig("info", "text")
	eng := New(Config{WorkerCount: 1, MaxLoops: 100, ExecutionTimeout: 5 * time.Second}, registry, s, logger, nil)

	instance := process.NewInstance(proc, nil)
	instance.SetVariable("approved", true)
	if err := s.CreateInstance(context.Background(), instance.ToRecord()); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := eng.Run(ctx, instance)
	if err != nil {
		t.Fatalf("engine run failed: %v", err)
	}
	if instance.State != process.StateCompleted {
		t.Errorf("expected state COMPLETED, got %s", instance.State)
	}
}

func TestEngine_Run_InclusiveGateway(t *testing.T) {
	proc := &bpmn.Process{
		ID:           "proc-inc",
		Name:         "Inclusive",
		StartEventID: "start-1",
		Elements: map[string]bpmn.Element{
			"start-1": {ID: "start-1", Type: bpmn.ElementTypeStartEvent, OutgoingFlows: []string{"flow-1"}},
			"gw-inc":  {ID: "gw-inc", Type: bpmn.ElementTypeInclusiveGateway, IncomingFlows: []string{"flow-1"}, OutgoingFlows: []string{"flow-2", "flow-3"}},
			"end-a":   {ID: "end-a", Type: bpmn.ElementTypeEndEvent, IncomingFlows: []string{"flow-2"}},
			"end-b":   {ID: "end-b", Type: bpmn.ElementTypeEndEvent, IncomingFlows: []string{"flow-3"}},
		},
		Flows: map[string]bpmn.Flow{
			"flow-1": {ID: "flow-1", SourceRef: "start-1", TargetRef: "gw-inc"},
			"flow-2": {ID: "flow-2", SourceRef: "gw-inc", TargetRef: "end-a"},
			"flow-3": {ID: "flow-3", SourceRef: "gw-inc", TargetRef: "end-b"},
		},
	}

	registry := NewElementRegistry()
	registry.Register(bpmn.ElementTypeStartEvent, events.NewStartEvent)
	registry.Register(bpmn.ElementTypeEndEvent, events.NewEndEvent)
	registry.Register(bpmn.ElementTypeInclusiveGateway, gateways.NewInclusiveGateway)

	s := memory.NewStore()
	logger, _ := observability.NewFromConfig("info", "text")
	eng := New(Config{WorkerCount: 1, MaxLoops: 100, ExecutionTimeout: 5 * time.Second}, registry, s, logger, nil)

	instance := process.NewInstance(proc, nil)
	if err := s.CreateInstance(context.Background(), instance.ToRecord()); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := eng.Run(ctx, instance)
	if err != nil {
		t.Fatalf("engine run failed: %v", err)
	}
	if instance.State != process.StateCompleted {
		t.Errorf("expected state COMPLETED, got %s", instance.State)
	}
}

func TestEngine_Run_MessageThrowEvent(t *testing.T) {
	proc := &bpmn.Process{
		ID:           "proc-msg",
		Name:         "Message",
		StartEventID: "start-1",
		Elements: map[string]bpmn.Element{
			"start-1": {ID: "start-1", Type: bpmn.ElementTypeStartEvent, OutgoingFlows: []string{"flow-1"}},
			"msg-1":   {ID: "msg-1", Type: bpmn.ElementTypeMessageThrow, IncomingFlows: []string{"flow-1"}, OutgoingFlows: []string{"flow-2"}},
			"end-1":   {ID: "end-1", Type: bpmn.ElementTypeEndEvent, IncomingFlows: []string{"flow-2"}},
		},
		Flows: map[string]bpmn.Flow{
			"flow-1": {ID: "flow-1", SourceRef: "start-1", TargetRef: "msg-1"},
			"flow-2": {ID: "flow-2", SourceRef: "msg-1", TargetRef: "end-1"},
		},
	}

	registry := NewElementRegistry()
	registry.Register(bpmn.ElementTypeStartEvent, events.NewStartEvent)
	registry.Register(bpmn.ElementTypeEndEvent, events.NewEndEvent)
	registry.Register(bpmn.ElementTypeMessageThrow, events.NewMessageThrowEvent)

	s := memory.NewStore()
	logger, _ := observability.NewFromConfig("info", "text")
	eng := New(Config{WorkerCount: 1, MaxLoops: 100, ExecutionTimeout: 5 * time.Second}, registry, s, logger, nil)

	instance := process.NewInstance(proc, nil)
	if err := s.CreateInstance(context.Background(), instance.ToRecord()); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := eng.Run(ctx, instance)
	if err != nil {
		t.Fatalf("engine run failed: %v", err)
	}
	if instance.State != process.StateCompleted {
		t.Errorf("expected state COMPLETED, got %s", instance.State)
	}
}

func TestEngine_Run_EventBasedGateway(t *testing.T) {
	proc := &bpmn.Process{
		ID:           "proc-eb",
		Name:         "Event Based",
		StartEventID: "start-1",
		Elements: map[string]bpmn.Element{
			"start-1": {ID: "start-1", Type: bpmn.ElementTypeStartEvent, OutgoingFlows: []string{"flow-1"}},
			"gw-eb":   {ID: "gw-eb", Type: bpmn.ElementTypeEventBasedGateway, IncomingFlows: []string{"flow-1"}, OutgoingFlows: []string{"flow-2", "flow-3"}},
			"end-1":   {ID: "end-1", Type: bpmn.ElementTypeEndEvent, IncomingFlows: []string{"flow-2"}},
			"end-2":   {ID: "end-2", Type: bpmn.ElementTypeEndEvent, IncomingFlows: []string{"flow-3"}},
		},
		Flows: map[string]bpmn.Flow{
			"flow-1": {ID: "flow-1", SourceRef: "start-1", TargetRef: "gw-eb"},
			"flow-2": {ID: "flow-2", SourceRef: "gw-eb", TargetRef: "end-1"},
			"flow-3": {ID: "flow-3", SourceRef: "gw-eb", TargetRef: "end-2"},
		},
	}

	registry := NewElementRegistry()
	registry.Register(bpmn.ElementTypeStartEvent, events.NewStartEvent)
	registry.Register(bpmn.ElementTypeEndEvent, events.NewEndEvent)
	registry.Register(bpmn.ElementTypeEventBasedGateway, gateways.NewEventBasedGateway)

	s := memory.NewStore()
	logger, _ := observability.NewFromConfig("info", "text")
	eng := New(Config{WorkerCount: 1, MaxLoops: 100, ExecutionTimeout: 5 * time.Second}, registry, s, logger, nil)

	instance := process.NewInstance(proc, nil)
	if err := s.CreateInstance(context.Background(), instance.ToRecord()); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := eng.Run(ctx, instance)
	if err != nil {
		t.Fatalf("engine run failed: %v", err)
	}
	if instance.State != process.StateWaiting {
		t.Errorf("expected state WAITING for event-based gateway, got %s", instance.State)
	}
}

func TestEngine_Run_UserTaskFlow(t *testing.T) {
	proc := &bpmn.Process{
		ID:           "proc-flow",
		Name:         "User task flow",
		StartEventID: "start-1",
		Elements: map[string]bpmn.Element{
			"start-1": {ID: "start-1", Type: bpmn.ElementTypeStartEvent, OutgoingFlows: []string{"flow-1"}},
			"task-1":  {ID: "task-1", Type: bpmn.ElementTypeUserTask, IncomingFlows: []string{"flow-1"}, OutgoingFlows: []string{"flow-2"}, Assignee: "user-1"},
			"end-1":   {ID: "end-1", Type: bpmn.ElementTypeEndEvent, IncomingFlows: []string{"flow-2"}},
		},
		Flows: map[string]bpmn.Flow{
			"flow-1": {ID: "flow-1", SourceRef: "start-1", TargetRef: "task-1"},
			"flow-2": {ID: "flow-2", SourceRef: "task-1", TargetRef: "end-1"},
		},
	}

	registry := NewElementRegistry()
	registry.Register(bpmn.ElementTypeStartEvent, events.NewStartEvent)
	registry.Register(bpmn.ElementTypeEndEvent, events.NewEndEvent)
	registry.Register(bpmn.ElementTypeUserTask, activities.NewUserTask)

	s := memory.NewStore()
	logger, _ := observability.NewFromConfig("info", "text")
	eng := New(Config{WorkerCount: 1, MaxLoops: 100, ExecutionTimeout: 5 * time.Second}, registry, s, logger, nil)

	instance := process.NewInstance(proc, nil)
	if err := s.CreateInstance(context.Background(), instance.ToRecord()); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := eng.Run(ctx, instance)
	if err != nil {
		t.Fatalf("engine run failed: %v", err)
	}
	if instance.State != process.StateWaiting {
		t.Errorf("expected WAITING after user task, got %s", instance.State)
	}

	// Verify that the user task assigned variables were set
	assignee, ok := instance.GetVariable("task_assignee")
	if !ok || assignee != "user-1" {
		t.Errorf("expected task_assignee=user-1, got %v", assignee)
	}
}

func TestEngine_UnregisteredElement(t *testing.T) {
	proc := &bpmn.Process{
		ID:           "proc-unk",
		Name:         "Unknown Element",
		StartEventID: "start-1",
		Elements: map[string]bpmn.Element{
			"start-1": {ID: "start-1", Type: bpmn.ElementTypeStartEvent, OutgoingFlows: []string{"flow-1"}},
			"unk-1":   {ID: "unk-1", Type: "unknownType", IncomingFlows: []string{"flow-1"}},
		},
		Flows: map[string]bpmn.Flow{
			"flow-1": {ID: "flow-1", SourceRef: "start-1", TargetRef: "unk-1"},
		},
	}

	registry := NewElementRegistry()
	registry.Register(bpmn.ElementTypeStartEvent, events.NewStartEvent)

	s := memory.NewStore()
	logger, _ := observability.NewFromConfig("info", "text")
	eng := New(Config{WorkerCount: 1, MaxLoops: 100, ExecutionTimeout: 5 * time.Second}, registry, s, logger, nil)

	instance := process.NewInstance(proc, nil)
	if err := s.CreateInstance(context.Background(), instance.ToRecord()); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := eng.Run(ctx, instance)
	if err == nil {
		t.Fatal("expected error for unregistered element")
	}
}

func TestEngine_Run_TimerEvent_NoValue(t *testing.T) {
	proc := &bpmn.Process{
		ID:           "proc-timer",
		Name:         "Timer",
		StartEventID: "start-1",
		Elements: map[string]bpmn.Element{
			"start-1": {ID: "start-1", Type: bpmn.ElementTypeStartEvent, OutgoingFlows: []string{"flow-1"}},
			"timer-1": {ID: "timer-1", Type: bpmn.ElementTypeTimerEvent, IncomingFlows: []string{"flow-1"}, OutgoingFlows: []string{"flow-2"}},
			"end-1":   {ID: "end-1", Type: bpmn.ElementTypeEndEvent, IncomingFlows: []string{"flow-2"}},
		},
		Flows: map[string]bpmn.Flow{
			"flow-1": {ID: "flow-1", SourceRef: "start-1", TargetRef: "timer-1"},
			"flow-2": {ID: "flow-2", SourceRef: "timer-1", TargetRef: "end-1"},
		},
	}

	registry := NewElementRegistry()
	registry.Register(bpmn.ElementTypeStartEvent, events.NewStartEvent)
	registry.Register(bpmn.ElementTypeEndEvent, events.NewEndEvent)
	registry.Register(bpmn.ElementTypeTimerEvent, events.NewTimerEvent)

	s := memory.NewStore()
	logger, _ := observability.NewFromConfig("info", "text")
	eng := New(Config{WorkerCount: 1, MaxLoops: 100, ExecutionTimeout: 5 * time.Second}, registry, s, logger, nil)

	instance := process.NewInstance(proc, nil)
	if err := s.CreateInstance(context.Background(), instance.ToRecord()); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := eng.Run(ctx, instance)
	if err != nil {
		t.Fatalf("engine run failed: %v", err)
	}
	if instance.State != process.StateCompleted {
		t.Errorf("expected state COMPLETED, got %s", instance.State)
	}
}

func TestEngine_ActionConstants(t *testing.T) {
	if ActionRoute != "ROUTE" {
		t.Errorf("expected ROUTE, got %s", ActionRoute)
	}
	if ActionWait != "WAIT" {
		t.Errorf("expected WAIT, got %s", ActionWait)
	}
	if ActionForm != "FORM" {
		t.Errorf("expected FORM, got %s", ActionForm)
	}
	if ActionError != "ERROR" {
		t.Errorf("expected ERROR, got %s", ActionError)
	}
	if ActionComplete != "COMPLETE" {
		t.Errorf("expected COMPLETE, got %s", ActionComplete)
	}
	if ActionSkip != "SKIP" {
		t.Errorf("expected SKIP, got %s", ActionSkip)
	}
	if ActionQueue != "QUEUE" {
		t.Errorf("expected QUEUE, got %s", ActionQueue)
	}
	if ActionTerminate != "TERMINATE" {
		t.Errorf("expected TERMINATE, got %s", ActionTerminate)
	}
}

func TestNewResult(t *testing.T) {
	flow := &store.FlowRecord{ElementID: "test"}
	result := NewResult(ActionRoute, flow)
	if result.Action != ActionRoute {
		t.Errorf("expected ActionRoute, got %s", result.Action)
	}
	if result.FlowData != flow {
		t.Errorf("expected same flow reference")
	}
}

func TestNewResultWithFilters(t *testing.T) {
	flow := &store.FlowRecord{ElementID: "test"}
	filters := []string{"flow-1", "flow-2"}
	result := NewResultWithFilters(ActionRoute, flow, filters)
	if result.Action != ActionRoute {
		t.Errorf("expected ActionRoute, got %s", result.Action)
	}
	if len(result.FlowFilters) != 2 {
		t.Errorf("expected 2 filters, got %d", len(result.FlowFilters))
	}
}

func TestNewErrorResult(t *testing.T) {
	flow := &store.FlowRecord{ElementID: "test"}
	err := assertAnError("test error")
	result := NewErrorResult(flow, err)
	if result.Action != ActionError {
		t.Errorf("expected ActionError, got %s", result.Action)
	}
	if result.Error == nil {
		t.Fatal("expected error")
	}
	if result.Error.Error() != "test error" {
		t.Errorf("expected 'test error', got %s", result.Error.Error())
	}
}

func TestEngine_Run_AuditLogCreated(t *testing.T) {
	proc := &bpmn.Process{
		ID:           "proc-audit",
		Name:         "Audit Test",
		StartEventID: "start-1",
		Elements: map[string]bpmn.Element{
			"start-1": {ID: "start-1", Name: "Start", Type: bpmn.ElementTypeStartEvent, OutgoingFlows: []string{"flow-1"}},
			"end-1":   {ID: "end-1", Name: "End", Type: bpmn.ElementTypeEndEvent, IncomingFlows: []string{"flow-1"}},
		},
		Flows: map[string]bpmn.Flow{
			"flow-1": {ID: "flow-1", SourceRef: "start-1", TargetRef: "end-1"},
		},
	}

	registry := NewElementRegistry()
	registry.Register(bpmn.ElementTypeStartEvent, events.NewStartEvent)
	registry.Register(bpmn.ElementTypeEndEvent, events.NewEndEvent)

	auditDir := t.TempDir()

	store := memory.NewStore()
	logger, _ := observability.NewFromConfig("error", "text")

	dispatcher := observability.NewDispatcher()
	writer, err := observability.NewFileAuditWriter(auditDir, true, logger)
	if err != nil {
		t.Fatalf("failed to create audit writer: %v", err)
	}
	defer writer.Close()
	observability.NewAuditor(dispatcher, writer)

	eng := New(Config{WorkerCount: 1, MaxLoops: 100, ExecutionTimeout: 5 * time.Second}, registry, store, logger, nil)
	eng.WithDispatcher(dispatcher)

	instance := process.NewInstance(proc, nil)
	if err := store.CreateInstance(context.Background(), instance.ToRecord()); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := eng.Run(ctx, instance); err != nil {
		t.Fatalf("engine run failed: %v", err)
	}

	if instance.State != process.StateCompleted {
		t.Errorf("expected COMPLETED, got %s", instance.State)
	}

	auditPath := filepath.Join(auditDir, fmt.Sprintf("audit_%s.log", instance.ID))
	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "BPMN Execution Audit") {
		t.Error("expected audit header")
	}
	if !strings.Contains(content, "Audit Test") {
		t.Error("expected process name in audit log")
	}
	if !strings.Contains(content, "start-1") {
		t.Error("expected start element in audit log")
	}
	if !strings.Contains(content, "end-1") {
		t.Error("expected end element in audit log")
	}
	if !strings.Contains(content, "COMPLETED") {
		t.Error("expected COMPLETED result in audit log")
	}
}

func TestEngine_Run_AuditParallelBranches(t *testing.T) {
	proc := &bpmn.Process{
		ID:           "proc-audit-par",
		Name:         "Audit Parallel",
		StartEventID: "start-1",
		Elements: map[string]bpmn.Element{
			"start-1": {ID: "start-1", Name: "Start", Type: bpmn.ElementTypeStartEvent, OutgoingFlows: []string{"flow-1"}},
			"gw-div":  {ID: "gw-div", Name: "Split", Type: bpmn.ElementTypeParallelGateway, IncomingFlows: []string{"flow-1"}, OutgoingFlows: []string{"flow-2a", "flow-2b"}},
			"end-a":   {ID: "end-a", Name: "End A", Type: bpmn.ElementTypeEndEvent, IncomingFlows: []string{"flow-2a"}},
			"end-b":   {ID: "end-b", Name: "End B", Type: bpmn.ElementTypeEndEvent, IncomingFlows: []string{"flow-2b"}},
		},
		Flows: map[string]bpmn.Flow{
			"flow-1":  {ID: "flow-1", SourceRef: "start-1", TargetRef: "gw-div"},
			"flow-2a": {ID: "flow-2a", SourceRef: "gw-div", TargetRef: "end-a"},
			"flow-2b": {ID: "flow-2b", SourceRef: "gw-div", TargetRef: "end-b"},
		},
	}

	registry := NewElementRegistry()
	registry.Register(bpmn.ElementTypeStartEvent, events.NewStartEvent)
	registry.Register(bpmn.ElementTypeEndEvent, events.NewEndEvent)
	registry.Register(bpmn.ElementTypeParallelGateway, gateways.NewParallelGateway)

	auditDir := t.TempDir()

	s := memory.NewStore()
	logger, _ := observability.NewFromConfig("error", "text")

	dispatcher := observability.NewDispatcher()
	writer, err := observability.NewFileAuditWriter(auditDir, true, logger)
	if err != nil {
		t.Fatalf("failed to create audit writer: %v", err)
	}
	defer writer.Close()
	observability.NewAuditor(dispatcher, writer)

	eng := New(Config{WorkerCount: 2, MaxLoops: 100, ExecutionTimeout: 5 * time.Second}, registry, s, logger, nil)
	eng.WithDispatcher(dispatcher)

	instance := process.NewInstance(proc, nil)
	if err := s.CreateInstance(context.Background(), instance.ToRecord()); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := eng.Run(ctx, instance); err != nil {
		t.Fatalf("engine run failed: %v", err)
	}

	auditPath := filepath.Join(auditDir, fmt.Sprintf("audit_%s.log", instance.ID))
	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "parallelGateway") {
		t.Error("expected at least one audit entry for parallelGateway element")
	}

	// Verify expected elements appear in the audit
	for _, elem := range []string{"start-1", "gw-div", "end-a", "end-b"} {
		if !strings.Contains(content, elem) {
			t.Errorf("expected element %s in audit log", elem)
		}
	}
}

func TestEngine_Run_AuditDisabled(t *testing.T) {
	proc := &bpmn.Process{
		ID:           "proc-audit-off",
		Name:         "Audit Off",
		StartEventID: "start-1",
		Elements: map[string]bpmn.Element{
			"start-1": {ID: "start-1", Type: bpmn.ElementTypeStartEvent, OutgoingFlows: []string{"flow-1"}},
			"end-1":   {ID: "end-1", Type: bpmn.ElementTypeEndEvent, IncomingFlows: []string{"flow-1"}},
		},
		Flows: map[string]bpmn.Flow{
			"flow-1": {ID: "flow-1", SourceRef: "start-1", TargetRef: "end-1"},
		},
	}

	registry := NewElementRegistry()
	registry.Register(bpmn.ElementTypeStartEvent, events.NewStartEvent)
	registry.Register(bpmn.ElementTypeEndEvent, events.NewEndEvent)

	auditDir := t.TempDir()

	s := memory.NewStore()
	logger, _ := observability.NewFromConfig("error", "text")

	dispatcher := observability.NewDispatcher()
	writer, err := observability.NewFileAuditWriter(auditDir, false, logger)
	if err != nil {
		t.Fatalf("failed to create audit writer: %v", err)
	}
	defer writer.Close()
	observability.NewAuditor(dispatcher, writer)

	eng := New(Config{WorkerCount: 1, MaxLoops: 100, ExecutionTimeout: 5 * time.Second}, registry, s, logger, nil)
	eng.WithDispatcher(dispatcher)

	instance := process.NewInstance(proc, nil)
	if err := s.CreateInstance(context.Background(), instance.ToRecord()); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := eng.Run(ctx, instance); err != nil {
		t.Fatalf("engine run failed: %v", err)
	}

	auditPath := filepath.Join(auditDir, fmt.Sprintf("audit_%s.log", instance.ID))
	if _, err := os.Stat(auditPath); err == nil {
		data, _ := os.ReadFile(auditPath)
		if len(strings.TrimSpace(string(data))) > 0 {
			t.Error("expected empty audit file when audit is disabled")
		}
	}
}

func TestEngine_Run_SubProcess(t *testing.T) {
	// Build main process with a sub-process that contains a simple start→end flow.
	// This mirrors what the parser produces:
	// start-1 → flow-1 → sp-1 → sp-1_sp_entry → sp-1.sp-start → sp-1.sp-flow-1 → sp-1.sp-end → flow-2 → end-1
	spInternal := &bpmn.Process{
		ID:           "sp-1",
		Name:         "My Sub-Process",
		StartEventID: "sp-start",
		Elements: map[string]bpmn.Element{
			"sp-start": {ID: "sp-start", Type: bpmn.ElementTypeStartEvent},
			"sp-end":   {ID: "sp-end", Type: bpmn.ElementTypeEndEvent},
		},
		Flows: map[string]bpmn.Flow{
			"sp-flow-1": {ID: "sp-flow-1", SourceRef: "sp-start", TargetRef: "sp-end"},
		},
	}

	prefix := "sp-1."
	proc := &bpmn.Process{
		ID:           "proc-sub",
		Name:         "Sub-Process Test",
		StartEventID: "start-1",
		Elements: map[string]bpmn.Element{
			"start-1": {ID: "start-1", Type: bpmn.ElementTypeStartEvent, OutgoingFlows: []string{"flow-1"}},
			"sp-1": {
				ID: "sp-1", Type: bpmn.ElementTypeSubProcess,
				SubProcess:    spInternal,
				SubProcessEnd: prefix + "sp-end",
				IncomingFlows: []string{"flow-1"},
				OutgoingFlows: []string{"flow-2", "sp-1_sp_entry"},
			},
			prefix + "sp-start": {ID: prefix + "sp-start", Type: bpmn.ElementTypeStartEvent, OutgoingFlows: []string{prefix + "sp-flow-1"}},
			prefix + "sp-end": {
				ID: prefix + "sp-end", Type: bpmn.ElementTypeEndEvent,
				IncomingFlows: []string{prefix + "sp-flow-1"},
				OutgoingFlows: []string{prefix + "sp-flow-1", "flow-2"},
				ExtensionData: map[string]string{"subprocess_exit_flows": "flow-2"},
			},
			"end-1": {ID: "end-1", Type: bpmn.ElementTypeEndEvent, IncomingFlows: []string{"flow-2"}},
		},
		Flows: map[string]bpmn.Flow{
			"flow-1":        {ID: "flow-1", SourceRef: "start-1", TargetRef: "sp-1"},
			"flow-2":        {ID: "flow-2", SourceRef: "sp-1", TargetRef: "end-1"},
			"sp-1_sp_entry": {ID: "sp-1_sp_entry", SourceRef: "sp-1", TargetRef: prefix + "sp-start"},
			prefix + "sp-flow-1": {ID: prefix + "sp-flow-1", SourceRef: prefix + "sp-start", TargetRef: prefix + "sp-end"},
		},
	}

	registry := NewElementRegistry()
	registry.Register(bpmn.ElementTypeStartEvent, events.NewStartEvent)
	registry.Register(bpmn.ElementTypeEndEvent, events.NewEndEvent)
	registry.Register(bpmn.ElementTypeSubProcess, activities.NewSubProcess)

	s := memory.NewStore()
	logger, _ := observability.NewFromConfig("info", "text")
	if logger == nil {
		t.Fatal("failed to create logger")
	}

	eng := New(Config{WorkerCount: 1, MaxLoops: 100, ExecutionTimeout: 10 * time.Second}, registry, s, logger, nil)

	instance := process.NewInstance(proc, nil)
	if err := s.CreateInstance(context.Background(), instance.ToRecord()); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := eng.Run(ctx, instance)
	if err != nil {
		t.Fatalf("engine run failed: %v", err)
	}
	if instance.State != process.StateCompleted {
		t.Errorf("expected state COMPLETED, got %s", instance.State)
	}

	// Verify that internal elements were executed
	flows, err := s.GetFlowsByInstance(context.Background(), instance.ID)
	if err != nil {
		t.Fatalf("get flows: %v", err)
	}
	if len(flows) < 4 {
		t.Errorf("expected at least 4 flow records (sp-entry, sub-start, sub-end, exit), got %d", len(flows))
	}

	executed := make(map[string]bool)
	for _, f := range flows {
		executed[f.ElementID] = true
	}
	expectedInternal := []string{prefix + "sp-start", prefix + "sp-end"}
	for _, eid := range expectedInternal {
		if !executed[eid] {
			t.Errorf("expected sub-process element %s to be executed", eid)
		}
	}
}

func TestEngine_Run_ErrorCatch(t *testing.T) {
	// Process: start → sub-process (start → error-end) → error caught by boundary → end
	// The error boundary catches "err-001" and routes to "end-catch"
	spInternal := &bpmn.Process{
		ID:           "sp-1",
		StartEventID: "sp-start",
		Elements: map[string]bpmn.Element{
			"sp-start":     {ID: "sp-start", Type: bpmn.ElementTypeStartEvent},
			"sp-error-end": {ID: "sp-error-end", Type: bpmn.ElementTypeErrorEnd, EventDefinition: bpmn.EventDefinition{Type: bpmn.EventTypeError, ErrorCode: "err-001"}},
		},
		Flows: map[string]bpmn.Flow{
			"sp-flow-1": {ID: "sp-flow-1", SourceRef: "sp-start", TargetRef: "sp-error-end"},
		},
	}

	prefix := "sp-1."
	proc := &bpmn.Process{
		ID:           "proc-error",
		Name:         "Error Catch Test",
		StartEventID: "start-1",
		Elements: map[string]bpmn.Element{
			"start-1": {ID: "start-1", Type: bpmn.ElementTypeStartEvent, OutgoingFlows: []string{"flow-1"}},
			"sp-1": {
				ID: "sp-1", Type: bpmn.ElementTypeSubProcess,
				SubProcess:    spInternal,
				SubProcessEnd: prefix + "sp-error-end",
				IncomingFlows: []string{"flow-1"},
				OutgoingFlows: []string{"flow-sp-out", "sp-1_sp_entry"},
			},
			prefix + "sp-start": {ID: prefix + "sp-start", Type: bpmn.ElementTypeStartEvent, OutgoingFlows: []string{prefix + "sp-flow-1"}},
			prefix + "sp-error-end": {
				ID: prefix + "sp-error-end", Type: bpmn.ElementTypeErrorEnd,
				IncomingFlows: []string{prefix + "sp-flow-1"},
				OutgoingFlows: []string{prefix + "sp-flow-1", "flow-sp-out"},
				EventDefinition: bpmn.EventDefinition{Type: bpmn.EventTypeError, ErrorCode: "err-001"},
				ExtensionData:   map[string]string{"subprocess_exit_flows": "flow-sp-out"},
			},
			"error-catch-1": {
				ID: "error-catch-1", Type: bpmn.ElementTypeErrorCatch,
				AttachedToRef: "sp-1",
				EventDefinition: bpmn.EventDefinition{Type: bpmn.EventTypeError, ErrorCode: "err-001"},
				OutgoingFlows: []string{"flow-catch"},
			},
			"end-catch": {ID: "end-catch", Type: bpmn.ElementTypeEndEvent, IncomingFlows: []string{"flow-catch"}},
		},
		Flows: map[string]bpmn.Flow{
			"flow-1":        {ID: "flow-1", SourceRef: "start-1", TargetRef: "sp-1"},
			"flow-sp-out":   {ID: "flow-sp-out", SourceRef: "sp-1", TargetRef: "error-catch-1"},
			"sp-1_sp_entry": {ID: "sp-1_sp_entry", SourceRef: "sp-1", TargetRef: prefix + "sp-start"},
			prefix + "sp-flow-1": {ID: prefix + "sp-flow-1", SourceRef: prefix + "sp-start", TargetRef: prefix + "sp-error-end"},
			"flow-catch":   {ID: "flow-catch", SourceRef: "error-catch-1", TargetRef: "end-catch"},
		},
	}

	registry := NewElementRegistry()
	registry.Register(bpmn.ElementTypeStartEvent, events.NewStartEvent)
	registry.Register(bpmn.ElementTypeEndEvent, events.NewEndEvent)
	registry.Register(bpmn.ElementTypeSubProcess, activities.NewSubProcess)
	registry.Register(bpmn.ElementTypeErrorCatch, events.NewErrorCatchEvent)
	registry.Register(bpmn.ElementTypeErrorEnd, events.NewErrorEndEvent)

	s := memory.NewStore()
	logger, _ := observability.NewFromConfig("info", "text")
	if logger == nil {
		t.Fatal("failed to create logger")
	}

	eng := New(Config{WorkerCount: 1, MaxLoops: 100, ExecutionTimeout: 10 * time.Second}, registry, s, logger, nil)

	instance := process.NewInstance(proc, nil)
	if err := s.CreateInstance(context.Background(), instance.ToRecord()); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := eng.Run(ctx, instance)
	if err != nil {
		t.Fatalf("engine run failed: %v", err)
	}
	if instance.State != process.StateCompleted {
		t.Errorf("expected state COMPLETED, got %s", instance.State)
	}

	// Verify that the error catch element was executed
	flows, err := s.GetFlowsByInstance(context.Background(), instance.ID)
	if err != nil {
		t.Fatalf("get flows: %v", err)
	}
	executed := make(map[string]bool)
	for _, f := range flows {
		executed[f.ElementID] = true
	}
	if !executed["error-catch-1"] {
		t.Errorf("expected error-catch-1 to be executed")
	}
	if !executed[prefix+"sp-error-end"] {
		t.Errorf("expected sub-process error end event to be executed")
	}
	if !executed["end-catch"] {
		t.Errorf("expected end-catch to be executed")
	}
}

func TestEngine_Run_InterruptingBoundaryTimer(t *testing.T) {
	// Process: start → user-task → end (normal path).
	// Timer boundary on user-task routes to boundary-end.
	// After engine runs (user-task → WAITING), we simulate the timer firing
	// by calling Continue with the boundary timer flow record.
	proc := &bpmn.Process{
		ID:           "proc-boundary",
		Name:         "Boundary Timer Test",
		StartEventID: "start-1",
		Elements: map[string]bpmn.Element{
			"start-1":     {ID: "start-1", Type: bpmn.ElementTypeStartEvent, OutgoingFlows: []string{"flow-1"}},
			"user-task-1": {ID: "user-task-1", Type: bpmn.ElementTypeUserTask, Assignee: "user-1", IncomingFlows: []string{"flow-1"}, OutgoingFlows: []string{"flow-2"}},
			"end-1":       {ID: "end-1", Type: bpmn.ElementTypeEndEvent, IncomingFlows: []string{"flow-2"}},
			"timer-boundary-1": {
				ID: "timer-boundary-1", Type: bpmn.ElementTypeTimerEvent,
				AttachedToRef:  "user-task-1",
				CancelActivity: true,
				EventDefinition: bpmn.EventDefinition{
					Type:       bpmn.EventTypeTimer,
					TimerType:  bpmn.TimerTypeDuration,
					TimerValue: "PT1S",
				},
				OutgoingFlows: []string{"flow-boundary"},
			},
			"boundary-end": {ID: "boundary-end", Type: bpmn.ElementTypeEndEvent, IncomingFlows: []string{"flow-boundary"}},
		},
		Flows: map[string]bpmn.Flow{
			"flow-1":        {ID: "flow-1", SourceRef: "start-1", TargetRef: "user-task-1"},
			"flow-2":        {ID: "flow-2", SourceRef: "user-task-1", TargetRef: "end-1"},
			"flow-boundary": {ID: "flow-boundary", SourceRef: "timer-boundary-1", TargetRef: "boundary-end"},
		},
	}

	registry := NewElementRegistry()
	registry.Register(bpmn.ElementTypeStartEvent, events.NewStartEvent)
	registry.Register(bpmn.ElementTypeEndEvent, events.NewEndEvent)
	registry.Register(bpmn.ElementTypeUserTask, activities.NewUserTask)
	registry.Register(bpmn.ElementTypeTimerEvent, events.NewTimerEvent)

	s := memory.NewStore()
	logger, _ := observability.NewFromConfig("info", "text")
	eng := New(Config{WorkerCount: 1, MaxLoops: 100, ExecutionTimeout: 5 * time.Second}, registry, s, logger, nil)

	instance := process.NewInstance(proc, nil)
	if err := s.CreateInstance(context.Background(), instance.ToRecord()); err != nil {
		t.Fatalf("create instance: %v", err)
	}
	if err := s.SaveProcess(context.Background(), proc); err != nil {
		t.Fatalf("save process: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Run the process — should go to WAITING on UserTask
	if err := eng.Run(ctx, instance); err != nil {
		t.Fatalf("engine run failed: %v", err)
	}
	if instance.State != process.StateWaiting {
		t.Errorf("expected WAITING for user task, got %s", instance.State)
	}

	// The engine should have created a boundary timer flow via scheduleBoundaryTimers
	flows, err := s.GetFlowsByInstance(ctx, instance.ID)
	if err != nil {
		t.Fatalf("get flows: %v", err)
	}

	// Find the boundary timer flow that was created by scheduleBoundaryTimers
	var timerFlowID string
	for _, f := range flows {
		if f.ElementID == "timer-boundary-1" {
			timerFlowID = f.ID
			break
		}
	}
	if timerFlowID == "" {
		t.Fatal("expected boundary timer flow to be created by scheduleBoundaryTimers")
	}

	// Simulate the timer firing by calling Continue with the timer's flow
	if err := eng.Continue(ctx, instance.ID, timerFlowID, nil); err != nil {
		t.Fatalf("continue after timer boundary failed: %v", err)
	}

	// Verify process completed via boundary path
	flows, _ = s.GetFlowsByInstance(ctx, instance.ID)
	boundaryExecuted := false
	userTaskCancelled := false
	for _, f := range flows {
		if f.ElementID == "boundary-end" && f.Status == store.FlowStatusCompleted {
			boundaryExecuted = true
		}
		if f.ElementID == "user-task-1" && f.Status == store.FlowStatusCompleted {
			userTaskCancelled = true
		}
	}
	if !boundaryExecuted {
		t.Errorf("expected boundary-end to be executed")
	}
	if !userTaskCancelled {
		t.Errorf("expected user-task-1 flow to be cancelled by cancelAttachedFlows")
	}
}

func TestEngine_Run_CallActivity(t *testing.T) {
	// Called process: just start → end (no SequenceFlow element)
	calledProc := &bpmn.Process{
		ID:           "called-proc",
		Name:         "Called Process",
		StartEventID: "cal-start",
		Elements: map[string]bpmn.Element{
			"cal-start": {ID: "cal-start", Type: bpmn.ElementTypeStartEvent, OutgoingFlows: []string{"cal-flow-1"}},
			"cal-end":   {ID: "cal-end", Type: bpmn.ElementTypeEndEvent, IncomingFlows: []string{"cal-flow-1"}},
		},
		Flows: map[string]bpmn.Flow{
			"cal-flow-1": {ID: "cal-flow-1", SourceRef: "cal-start", TargetRef: "cal-end"},
		},
	}

	// Main process
	proc := &bpmn.Process{
		ID:           "main-proc",
		Name:         "Main Process",
		StartEventID: "start-1",
		Elements: map[string]bpmn.Element{
			"start-1": {ID: "start-1", Type: bpmn.ElementTypeStartEvent, OutgoingFlows: []string{"flow-1"}},
			"call-1": {
				ID: "call-1", Type: bpmn.ElementTypeCallActivity,
				CalledElement: "called-proc",
				IncomingFlows: []string{"flow-1"},
				OutgoingFlows: []string{"flow-2"},
			},
			"end-1": {ID: "end-1", Type: bpmn.ElementTypeEndEvent, IncomingFlows: []string{"flow-2"}},
		},
		Flows: map[string]bpmn.Flow{
			"flow-1": {ID: "flow-1", SourceRef: "start-1", TargetRef: "call-1"},
			"flow-2": {ID: "flow-2", SourceRef: "call-1", TargetRef: "end-1"},
		},
	}

	registry := NewElementRegistry()
	registry.Register(bpmn.ElementTypeStartEvent, events.NewStartEvent)
	registry.Register(bpmn.ElementTypeEndEvent, events.NewEndEvent)
	registry.Register(bpmn.ElementTypeCallActivity, activities.NewCallActivity)

	s := memory.NewStore()
	logger, _ := observability.NewFromConfig("info", "text")
	eng := New(Config{WorkerCount: 1, MaxLoops: 100, ExecutionTimeout: 10 * time.Second}, registry, s, logger, nil)

	ctx := context.Background()
	if err := s.SaveProcess(ctx, calledProc); err != nil {
		t.Fatalf("SaveProcess called: %v", err)
	}
	if err := s.SaveProcess(ctx, proc); err != nil {
		t.Fatalf("SaveProcess main: %v", err)
	}

	instance := process.NewInstance(proc, nil)
	if err := s.CreateInstance(ctx, instance.ToRecord()); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	runCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := eng.Run(runCtx, instance); err != nil {
		t.Fatalf("engine run failed: %v", err)
	}
	if instance.State != process.StateCompleted {
		t.Errorf("expected COMPLETED, got %s", instance.State)
	}

	flows, err := s.GetFlowsByInstance(ctx, instance.ID)
	if err != nil {
		t.Fatalf("get flows: %v", err)
	}
	executed := make(map[string]bool)
	for _, f := range flows {
		executed[f.ElementID] = true
	}
	if !executed["ca-call-1.cal-start"] {
		t.Errorf("expected called process start event to be executed")
	}
	if !executed["ca-call-1.cal-end"] {
		t.Errorf("expected called process end event to be executed")
	}
}

func TestEngine_Run_SignalCatchSend(t *testing.T) {
	// Process: start → signal-catch → end
	proc := &bpmn.Process{
		ID:           "proc-signal",
		Name:         "Signal Test",
		StartEventID: "start-1",
		Elements: map[string]bpmn.Element{
			"start-1":  {ID: "start-1", Type: bpmn.ElementTypeStartEvent, OutgoingFlows: []string{"flow-1"}},
			"sig-catch-1": {
				ID: "sig-catch-1", Type: bpmn.ElementTypeSignalCatch,
				IncomingFlows: []string{"flow-1"}, OutgoingFlows: []string{"flow-2"},
				EventDefinition: bpmn.EventDefinition{Type: bpmn.EventTypeSignal, SignalRef: "sig-1"},
			},
			"end-1": {ID: "end-1", Type: bpmn.ElementTypeEndEvent, IncomingFlows: []string{"flow-2"}},
		},
		Flows: map[string]bpmn.Flow{
			"flow-1": {ID: "flow-1", SourceRef: "start-1", TargetRef: "sig-catch-1"},
			"flow-2": {ID: "flow-2", SourceRef: "sig-catch-1", TargetRef: "end-1"},
		},
	}

	registry := NewElementRegistry()
	registry.Register(bpmn.ElementTypeStartEvent, events.NewStartEvent)
	registry.Register(bpmn.ElementTypeEndEvent, events.NewEndEvent)
	registry.Register(bpmn.ElementTypeSignalCatch, events.NewSignalCatchEvent)

	s := memory.NewStore()
	logger, _ := observability.NewFromConfig("info", "text")
	eng := New(Config{WorkerCount: 1, MaxLoops: 100, ExecutionTimeout: 5 * time.Second}, registry, s, logger, nil)

	ctx := context.Background()
	if err := s.SaveProcess(ctx, proc); err != nil {
		t.Fatalf("save process: %v", err)
	}

	instance := process.NewInstance(proc, nil)
	if err := s.CreateInstance(ctx, instance.ToRecord()); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	runCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Run → should WAIT at signal-catch
	if err := eng.Run(runCtx, instance); err != nil {
		t.Fatalf("run: %v", err)
	}
	if instance.State != process.StateWaiting {
		t.Fatalf("expected WAITING, got %s", instance.State)
	}

	// Send the signal
	instances, err := eng.SendSignal(ctx, "sig-1", map[string]any{"from": "test"})
	if err != nil {
		t.Fatalf("send signal: %v", err)
	}
	if len(instances) != 1 {
		t.Errorf("expected 1 instance resumed, got %d", len(instances))
	}

	// Verify process completed
	updated, err := s.GetInstance(ctx, instance.ID)
	if err != nil {
		t.Fatalf("get instance: %v", err)
	}
	if updated.Status != store.InstanceStatusCompleted {
		t.Errorf("expected COMPLETED, got %s", updated.Status)
	}
}

func TestEngine_Run_SignalThrow(t *testing.T) {
	// Process: start → signal-throw → end (throw passes through, no wait)
	proc := &bpmn.Process{
		ID:           "proc-sig-throw",
		Name:         "Signal Throw",
		StartEventID: "start-1",
		Elements: map[string]bpmn.Element{
			"start-1":  {ID: "start-1", Type: bpmn.ElementTypeStartEvent, OutgoingFlows: []string{"flow-1"}},
			"sig-throw-1": {
				ID: "sig-throw-1", Type: bpmn.ElementTypeSignalThrow,
				IncomingFlows: []string{"flow-1"}, OutgoingFlows: []string{"flow-2"},
				EventDefinition: bpmn.EventDefinition{Type: bpmn.EventTypeSignal, SignalRef: "sig-1"},
			},
			"end-1": {ID: "end-1", Type: bpmn.ElementTypeEndEvent, IncomingFlows: []string{"flow-2"}},
		},
		Flows: map[string]bpmn.Flow{
			"flow-1": {ID: "flow-1", SourceRef: "start-1", TargetRef: "sig-throw-1"},
			"flow-2": {ID: "flow-2", SourceRef: "sig-throw-1", TargetRef: "end-1"},
		},
	}

	registry := NewElementRegistry()
	registry.Register(bpmn.ElementTypeStartEvent, events.NewStartEvent)
	registry.Register(bpmn.ElementTypeEndEvent, events.NewEndEvent)
	registry.Register(bpmn.ElementTypeSignalThrow, events.NewSignalThrowEvent)

	s := memory.NewStore()
	logger, _ := observability.NewFromConfig("info", "text")
	eng := New(Config{WorkerCount: 1, MaxLoops: 100, ExecutionTimeout: 5 * time.Second}, registry, s, logger, nil)

	ctx := context.Background()
	if err := s.SaveProcess(ctx, proc); err != nil {
		t.Fatalf("save process: %v", err)
	}

	instance := process.NewInstance(proc, nil)
	if err := s.CreateInstance(ctx, instance.ToRecord()); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	runCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := eng.Run(runCtx, instance); err != nil {
		t.Fatalf("run: %v", err)
	}
	if instance.State != process.StateCompleted {
		t.Errorf("expected COMPLETED, got %s", instance.State)
	}
}

func TestEngine_Run_ErrorStartEvent(t *testing.T) {
	// Error thrown in sub-process caught by error start event (ElementTypeErrorCatch at top level)
	spInternal := &bpmn.Process{
		ID:           "sp-1",
		StartEventID: "sp-start",
		Elements: map[string]bpmn.Element{
			"sp-start": {ID: "sp-start", Type: bpmn.ElementTypeStartEvent},
			"sp-err":   {ID: "sp-err", Type: bpmn.ElementTypeErrorEnd, EventDefinition: bpmn.EventDefinition{Type: bpmn.EventTypeError, ErrorCode: "ERR-001"}},
		},
		Flows: map[string]bpmn.Flow{
			"sp-flow-1": {ID: "sp-flow-1", SourceRef: "sp-start", TargetRef: "sp-err"},
		},
	}
	proc := &bpmn.Process{
		ID:           "proc-err-start",
		Name:         "Error Start",
		StartEventID: "start-1",
		Elements: map[string]bpmn.Element{
			"start-1": {ID: "start-1", Type: bpmn.ElementTypeStartEvent, OutgoingFlows: []string{"flow-1"}},
			"sp-1": {
				ID: "sp-1", Type: bpmn.ElementTypeSubProcess,
				SubProcess: spInternal, SubProcessEnd: "sp-1.sp-err",
				IncomingFlows: []string{"flow-1"},
				OutgoingFlows: []string{"flow-2", "sp-1_sp_entry"},
			},
			"sp-1.sp-start": {ID: "sp-1.sp-start", Type: bpmn.ElementTypeStartEvent, OutgoingFlows: []string{"sp-1.sp-flow-1"}},
			"sp-1.sp-err": {
				ID: "sp-1.sp-err", Type: bpmn.ElementTypeErrorEnd,
				IncomingFlows: []string{"sp-1.sp-flow-1"},
				EventDefinition: bpmn.EventDefinition{Type: bpmn.EventTypeError, ErrorCode: "ERR-001"},
				ExtensionData:   map[string]string{"subprocess_exit_flows": "flow-2"},
			},
			"error-start-1": {
				ID: "error-start-1", Type: bpmn.ElementTypeErrorCatch,
				EventDefinition: bpmn.EventDefinition{Type: bpmn.EventTypeError, ErrorCode: "ERR-001"},
				OutgoingFlows:   []string{"flow-catch"},
			},
			"end-catch": {ID: "end-catch", Type: bpmn.ElementTypeEndEvent, IncomingFlows: []string{"flow-catch"}},
		},
		Flows: map[string]bpmn.Flow{
			"flow-1":        {ID: "flow-1", SourceRef: "start-1", TargetRef: "sp-1"},
			"flow-2":        {ID: "flow-2", SourceRef: "sp-1", TargetRef: "error-start-1"},
			"sp-1_sp_entry": {ID: "sp-1_sp_entry", SourceRef: "sp-1", TargetRef: "sp-1.sp-start"},
			"sp-1.sp-flow-1": {ID: "sp-1.sp-flow-1", SourceRef: "sp-1.sp-start", TargetRef: "sp-1.sp-err"},
			"flow-catch":    {ID: "flow-catch", SourceRef: "error-start-1", TargetRef: "end-catch"},
		},
	}
	registry := NewElementRegistry()
	registry.Register(bpmn.ElementTypeStartEvent, events.NewStartEvent)
	registry.Register(bpmn.ElementTypeEndEvent, events.NewEndEvent)
	registry.Register(bpmn.ElementTypeSubProcess, activities.NewSubProcess)
	registry.Register(bpmn.ElementTypeErrorEnd, events.NewErrorEndEvent)
	registry.Register(bpmn.ElementTypeErrorCatch, events.NewErrorCatchEvent)

	s := memory.NewStore()
	logger, _ := observability.NewFromConfig("info", "text")
	eng := New(Config{WorkerCount: 1, MaxLoops: 100, ExecutionTimeout: 5 * time.Second}, registry, s, logger, nil)

	ctx := context.Background()
	s.SaveProcess(ctx, proc)

	instance := process.NewInstance(proc, nil)
	s.CreateInstance(ctx, instance.ToRecord())

	runCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := eng.Run(runCtx, instance); err != nil {
		t.Fatalf("run: %v", err)
	}
	if instance.State != process.StateCompleted {
		t.Errorf("expected COMPLETED, got %s", instance.State)
	}

	flows, _ := s.GetFlowsByInstance(ctx, instance.ID)
	executed := make(map[string]bool)
	for _, f := range flows {
		executed[f.ElementID] = true
	}
	if !executed["error-start-1"] {
		t.Error("expected error-start-1 to be executed")
	}
}

type assertAnError string

func (e assertAnError) Error() string {
	return string(e)
}

// --- AI Task Tests ---

type mockAIGateway struct {
	response string
}

func (m *mockAIGateway) Generate(_ context.Context, req ai.Request) (ai.Response, error) {
	return ai.Response{
		Text:       m.response,
		Model:      "mock-model",
		TokensIn:   len(req.System) + len(req.Messages[0].Content),
		TokensOut:  len(m.response),
		DurationMs: 1,
	}, nil
}

func newMockAIGateway(response string) *mockAIGateway {
	return &mockAIGateway{response: response}
}

func TestEngine_Run_AITask(t *testing.T) {
	proc := &bpmn.Process{
		ID:           "proc-ai",
		Name:         "AI Task Process",
		StartEventID: "start-1",
		Elements: map[string]bpmn.Element{
			"start-1": {
				ID: "start-1", Type: bpmn.ElementTypeStartEvent,
				OutgoingFlows: []string{"flow-1"},
			},
			"ai-1": {
				ID: "ai-1", Type: bpmn.ElementTypeAITask,
				IncomingFlows: []string{"flow-1"},
				OutgoingFlows: []string{"flow-2"},
				ExtensionData: map[string]string{
					"scriptBody":   "Classify this: {{input}}",
					"model":        "gpt-4o",
					"systemPrompt": "You are a classifier.",
				},
			},
			"end-1": {
				ID: "end-1", Type: bpmn.ElementTypeEndEvent,
				IncomingFlows: []string{"flow-2"},
			},
		},
		Flows: map[string]bpmn.Flow{
			"flow-1": {ID: "flow-1", SourceRef: "start-1", TargetRef: "ai-1"},
			"flow-2": {ID: "flow-2", SourceRef: "ai-1", TargetRef: "end-1"},
		},
	}

	registry := NewElementRegistry()
	registry.Register(bpmn.ElementTypeStartEvent, events.NewStartEvent)
	registry.Register(bpmn.ElementTypeEndEvent, events.NewEndEvent)
	registry.Register(bpmn.ElementTypeAITask, activities.NewAITaskConstructor(newMockAIGateway("approved"), ai.NewToolRegistry(), nil, nil, nil))

	s := memory.NewStore()
	logger, _ := observability.NewFromConfig("info", "text")
	eng := New(Config{WorkerCount: 1, MaxLoops: 100, ExecutionTimeout: 5 * time.Second}, registry, s, logger, nil)

	instance := process.NewInstance(proc, map[string]any{"input": "urgent billing issue"})
	if err := s.CreateInstance(context.Background(), instance.ToRecord()); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := eng.Run(ctx, instance)
	if err != nil {
		t.Fatalf("engine run failed: %v", err)
	}

	if instance.State != process.StateCompleted {
		t.Errorf("expected state COMPLETED, got %s", instance.State)
	}

	result, ok := instance.GetVariable("ai-1_result")
	if !ok {
		t.Fatal("expected ai-1_result variable to be set")
	}
	if result != "approved" {
		t.Errorf("expected ai-1_result='approved', got %v", result)
	}

	model, ok := instance.GetVariable("ai-1_model")
	if !ok || model != "mock-model" {
		t.Errorf("expected ai-1_model='mock-model', got %v", model)
	}
}

func TestEngine_Run_AITask_WithTools(t *testing.T) {
	tr := ai.NewToolRegistry()
	err := tr.Register(ai.ToolDefinition{
		Name:        "get_ticket_info",
		Description: "Get ticket information by ID",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}}}`),
		Function: func(_ context.Context, args json.RawMessage) (string, error) {
			return `{"id":"TKT-001","priority":"high","status":"open"}`, nil
		},
	})
	if err != nil {
		t.Fatalf("register tool: %v", err)
	}

	proc := &bpmn.Process{
		ID:           "proc-ai-tools",
		Name:         "AI Task With Tools",
		StartEventID: "start-1",
		Elements: map[string]bpmn.Element{
			"start-1": {
				ID: "start-1", Type: bpmn.ElementTypeStartEvent,
				OutgoingFlows: []string{"flow-1"},
			},
			"ai-1": {
				ID: "ai-1", Type: bpmn.ElementTypeAITask,
				IncomingFlows: []string{"flow-1"},
				OutgoingFlows: []string{"flow-2"},
				ExtensionData: map[string]string{
					"scriptBody": "Check ticket {{ticket_id}}",
					"tools":      "get_ticket_info",
				},
			},
			"end-1": {
				ID: "end-1", Type: bpmn.ElementTypeEndEvent,
				IncomingFlows: []string{"flow-2"},
			},
		},
		Flows: map[string]bpmn.Flow{
			"flow-1": {ID: "flow-1", SourceRef: "start-1", TargetRef: "ai-1"},
			"flow-2": {ID: "flow-2", SourceRef: "ai-1", TargetRef: "end-1"},
		},
	}

	registry := NewElementRegistry()
	registry.Register(bpmn.ElementTypeStartEvent, events.NewStartEvent)
	registry.Register(bpmn.ElementTypeEndEvent, events.NewEndEvent)
	registry.Register(bpmn.ElementTypeAITask, activities.NewAITaskConstructor(newMockAIGateway("ticket found"), tr, nil, nil, nil))

	s := memory.NewStore()
	logger, _ := observability.NewFromConfig("info", "text")
	eng := New(Config{WorkerCount: 1, MaxLoops: 100, ExecutionTimeout: 5 * time.Second}, registry, s, logger, nil)

	instance := process.NewInstance(proc, map[string]any{"ticket_id": "TKT-001"})
	if err := s.CreateInstance(context.Background(), instance.ToRecord()); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = eng.Run(ctx, instance)
	if err != nil {
		t.Fatalf("engine run failed: %v", err)
	}

	if instance.State != process.StateCompleted {
		t.Errorf("expected state COMPLETED, got %s", instance.State)
	}

	result, ok := instance.GetVariable("ai-1_result")
	if !ok {
		t.Fatal("expected ai-1_result variable to be set")
	}
	if result != "ticket found" {
		t.Errorf("expected ai-1_result='ticket found', got %v", result)
	}
}

func TestEngine_Run_AITask_Error(t *testing.T) {
	errorGateway := aiErrorMock{}
	proc := &bpmn.Process{
		ID:           "proc-ai-err",
		Name:         "AI Task Error",
		StartEventID: "start-1",
		Elements: map[string]bpmn.Element{
			"start-1": {
				ID: "start-1", Type: bpmn.ElementTypeStartEvent,
				OutgoingFlows: []string{"flow-1"},
			},
			"ai-1": {
				ID: "ai-1", Type: bpmn.ElementTypeAITask,
				IncomingFlows: []string{"flow-1"},
				OutgoingFlows: []string{"flow-2"},
				ExtensionData: map[string]string{
					"scriptBody": "test",
				},
			},
			"end-1": {
				ID: "end-1", Type: bpmn.ElementTypeEndEvent,
				IncomingFlows: []string{"flow-2"},
			},
		},
		Flows: map[string]bpmn.Flow{
			"flow-1": {ID: "flow-1", SourceRef: "start-1", TargetRef: "ai-1"},
			"flow-2": {ID: "flow-2", SourceRef: "ai-1", TargetRef: "end-1"},
		},
	}

	registry := NewElementRegistry()
	registry.Register(bpmn.ElementTypeStartEvent, events.NewStartEvent)
	registry.Register(bpmn.ElementTypeEndEvent, events.NewEndEvent)
	registry.Register(bpmn.ElementTypeAITask, activities.NewAITaskConstructor(errorGateway, ai.NewToolRegistry(), nil, nil, nil))

	s := memory.NewStore()
	logger, _ := observability.NewFromConfig("info", "text")
	eng := New(Config{WorkerCount: 1, MaxLoops: 100, ExecutionTimeout: 5 * time.Second}, registry, s, logger, nil)

	instance := process.NewInstance(proc, nil)
	if err := s.CreateInstance(context.Background(), instance.ToRecord()); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := eng.Run(ctx, instance)
	if err == nil {
		t.Fatal("expected engine run to fail due to AI error")
	}
	t.Logf("engine returned expected error: %v", err)

	if instance.State != process.StateError {
		t.Logf("instance state: %s (expected ERROR due to AI failure)", instance.State)
	}
}

func TestEngine_Run_AITask_OutputSchema(t *testing.T) {
	proc := &bpmn.Process{
		ID:           "proc-ai-schema",
		Name:         "AI Task With Schema",
		StartEventID: "start-1",
		Elements: map[string]bpmn.Element{
			"start-1": {
				ID: "start-1", Type: bpmn.ElementTypeStartEvent,
				OutgoingFlows: []string{"flow-1"},
			},
			"ai-1": {
				ID: "ai-1", Type: bpmn.ElementTypeAITask,
				IncomingFlows: []string{"flow-1"},
				OutgoingFlows: []string{"flow-2"},
				ExtensionData: map[string]string{
					"scriptBody":   "Classify: {{input}}",
					"outputSchema": `{"type":"object","properties":{"category":{"type":"string"},"score":{"type":"number"}},"required":["category","score"]}`,
				},
			},
			"end-1": {
				ID: "end-1", Type: bpmn.ElementTypeEndEvent,
				IncomingFlows: []string{"flow-2"},
			},
		},
		Flows: map[string]bpmn.Flow{
			"flow-1": {ID: "flow-1", SourceRef: "start-1", TargetRef: "ai-1"},
			"flow-2": {ID: "flow-2", SourceRef: "ai-1", TargetRef: "end-1"},
		},
	}

	registry := NewElementRegistry()
	registry.Register(bpmn.ElementTypeStartEvent, events.NewStartEvent)
	registry.Register(bpmn.ElementTypeEndEvent, events.NewEndEvent)
	registry.Register(bpmn.ElementTypeAITask, activities.NewAITaskConstructor(newMockAIGateway(`{"category":"billing","score":85}`), ai.NewToolRegistry(), nil, nil, nil))

	s := memory.NewStore()
	logger, _ := observability.NewFromConfig("info", "text")
	eng := New(Config{WorkerCount: 1, MaxLoops: 100, ExecutionTimeout: 5 * time.Second}, registry, s, logger, nil)

	instance := process.NewInstance(proc, map[string]any{"input": "urgent billing"})
	if err := s.CreateInstance(context.Background(), instance.ToRecord()); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := eng.Run(ctx, instance)
	if err != nil {
		t.Fatalf("engine run failed: %v", err)
	}

	if instance.State != process.StateCompleted {
		t.Errorf("expected state COMPLETED, got %s", instance.State)
	}

	_, hasParsed := instance.GetVariable("ai-1_parsed")
	if !hasParsed {
		t.Fatal("expected ai-1_parsed variable to be set")
	}

	_, hasValidationErr := instance.GetVariable("ai-1_validation_error")
	if hasValidationErr {
		t.Fatal("expected no validation error")
	}
}

type aiErrorMock struct{}

func (e aiErrorMock) Generate(_ context.Context, _ ai.Request) (ai.Response, error) {
	return ai.Response{}, fmt.Errorf("mock AI failure")
}
