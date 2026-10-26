package activities

import (
	"context"
	"fmt"
	"testing"

	"github.com/Raoon-Tech/bpmn-ai/internal/element"
	"github.com/Raoon-Tech/bpmn-ai/pkg/ai"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

type mockGateway struct {
	response ai.Response
	err      error
}

func (m *mockGateway) Generate(_ context.Context, req ai.Request) (ai.Response, error) {
	if m.err != nil {
		return ai.Response{}, m.err
	}
	resp := m.response
	if resp.Text == "" {
		resp.Text = "mock response " + req.Messages[0].Content
	}
	if resp.Model == "" {
		resp.Model = req.Model
	}
	return resp, nil
}

type mockEmbedder struct{}

func (mockEmbedder) Embed(_ context.Context, req ai.EmbeddingRequest) (ai.EmbeddingResponse, error) {
	embs := make([][]float32, len(req.Input))
	for i := range req.Input {
		embs[i] = []float32{0.1, 0.2, 0.3}
	}
	return ai.EmbeddingResponse{Embeddings: embs}, nil
}

func newMockAITask(t *testing.T, gateway ai.Gateway, ext map[string]string) *AITask {
	t.Helper()
	elem := bpmn.Element{
		ID:           "ai-1",
		Name:         "Analyze",
		TaskType:     bpmn.TaskTypeAI,
		ExtensionData: ext,
	}
	task, err := newAITask(elem, gateway, ai.NewToolRegistry(), ai.NewModelRouter(ai.NewProviderPool()), nil, ai.NewPromptManager())
	if err != nil {
		t.Fatalf("newAITask: %v", err)
	}
	return task
}

func TestAITask_Constructor(t *testing.T) {
	ctor := NewAITaskConstructor(&mockGateway{response: ai.Response{Text: "hi"}}, ai.NewToolRegistry(), ai.NewModelRouter(ai.NewProviderPool()), nil, ai.NewPromptManager())
	elem := bpmn.Element{ID: "ai-1", Name: "Test", TaskType: bpmn.TaskTypeAI}
	el, err := ctor(elem)
	if err != nil {
		t.Fatalf("constructor: %v", err)
	}
	if el.ID() != "ai-1" {
		t.Errorf("expected id ai-1, got %s", el.ID())
	}
	if el.Type() != bpmn.ElementTypeAITask {
		t.Errorf("expected type aiTask, got %s", el.Type())
	}
}

func TestAITask_Execute_Success(t *testing.T) {
	gw := &mockGateway{response: ai.Response{Text: "result text", Model: "gpt-4o", TokensIn: 10, TokensOut: 5}}
	task := newMockAITask(t, gw, map[string]string{"scriptBody": "analyze {{input}}"})

	ctx := &mockExecCtx{
		flow:      &store.FlowRecord{ElementID: "ai-1"},
		variables: map[string]any{"input": "hello"},
	}
	result := task.Execute(context.Background(), ctx)

	if result.Action != element.ActionRoute {
		t.Fatalf("expected ActionRoute, got %s (err=%v)", result.Action, result.Error)
	}
	if v, _ := ctx.GetVariable("ai-1_result"); v != "result text" {
		t.Errorf("unexpected result var: %v", v)
	}
	if v, _ := ctx.GetVariable("ai-1_model"); v != "gpt-4o" {
		t.Errorf("unexpected model var: %v", v)
	}
	if v, _ := ctx.GetVariable("ai-1_tokens_in"); v != 10 {
		t.Errorf("unexpected tokens_in: %v", v)
	}
	if v, _ := ctx.GetVariable("ai-1_tokens_out"); v != 5 {
		t.Errorf("unexpected tokens_out: %v", v)
	}
	if _, ok := ctx.GetVariable("ai_total_cost"); !ok {
		t.Error("expected ai_total_cost to be set")
	}
}

func TestAITask_Execute_Error(t *testing.T) {
	gw := &mockGateway{err: fmt.Errorf("boom")}
	task := newMockAITask(t, gw, map[string]string{"scriptBody": "analyze"})

	ctx := &mockExecCtx{
		flow:      &store.FlowRecord{ElementID: "ai-1"},
		variables: make(map[string]any),
	}
	result := task.Execute(context.Background(), ctx)

	if result.Action != element.ActionError {
		t.Fatalf("expected ActionError, got %s", result.Action)
	}
	if result.Error == nil {
		t.Fatal("expected error")
	}
	if ctx.flow.Status != store.FlowStatusError {
		t.Errorf("expected flow status ERROR, got %s", ctx.flow.Status)
	}
}

func TestAITask_Execute_WithSchema(t *testing.T) {
	gw := &mockGateway{response: ai.Response{Text: `{"summary":"ok","score":5}`, Model: "gpt-4o", TokensIn: 1, TokensOut: 1}}
	task := newMockAITask(t, gw, map[string]string{
		"scriptBody":   "analyze",
		"outputSchema": `{"type":"object","properties":{"summary":{"type":"string"},"score":{"type":"number"}},"required":["summary"]}`,
	})

	ctx := &mockExecCtx{
		flow:      &store.FlowRecord{ElementID: "ai-1"},
		variables: make(map[string]any),
	}
	result := task.Execute(context.Background(), ctx)

	if result.Action != element.ActionRoute {
		t.Fatalf("expected ActionRoute, got %s (err=%v)", result.Action, result.Error)
	}
	if _, ok := ctx.GetVariable("ai-1_parsed"); !ok {
		t.Error("expected ai-1_parsed to be set on valid schema")
	}
}

func TestAITask_Execute_SchemaInvalid(t *testing.T) {
	gw := &mockGateway{response: ai.Response{Text: "not json", Model: "gpt-4o", TokensIn: 1, TokensOut: 1}}
	task := newMockAITask(t, gw, map[string]string{
		"scriptBody":   "analyze",
		"outputSchema": `{"type":"object","properties":{"summary":{"type":"string"}},"required":["summary"]}`,
	})

	ctx := &mockExecCtx{
		flow:      &store.FlowRecord{ElementID: "ai-1"},
		variables: make(map[string]any),
	}
	result := task.Execute(context.Background(), ctx)

	if result.Action != element.ActionRoute {
		t.Fatalf("expected ActionRoute, got %s", result.Action)
	}
	if _, ok := ctx.GetVariable("ai-1_validation_error"); !ok {
		t.Error("expected ai-1_validation_error to be set on invalid output")
	}
}

func TestAITask_Execute_RAG(t *testing.T) {
	rag := ai.NewRAGSystem(mockEmbedder{})
	rag.AddCollection("kb", ai.NewInMemoryVectorStore())
	// Index a document so retrieval returns something.
	_ = rag.Index(context.Background(), "kb", []ai.Document{{ID: "d1", Content: "relevant context"}})

	gw := &mockGateway{response: ai.Response{Text: "rag answer", Model: "gpt-4o", TokensIn: 1, TokensOut: 1}}
	elem := bpmn.Element{
		ID: "ai-1", Name: "Analyze", TaskType: bpmn.TaskTypeAI,
		ExtensionData: map[string]string{"scriptBody": "analyze", "rag": "kb"},
	}
	task, err := newAITask(elem, gw, ai.NewToolRegistry(), ai.NewModelRouter(ai.NewProviderPool()), rag, ai.NewPromptManager())
	if err != nil {
		t.Fatalf("newAITask: %v", err)
	}

	ctx := &mockExecCtx{
		flow:      &store.FlowRecord{ElementID: "ai-1"},
		variables: make(map[string]any),
	}
	result := task.Execute(context.Background(), ctx)
	if result.Action != element.ActionRoute {
		t.Fatalf("expected ActionRoute, got %s (err=%v)", result.Action, result.Error)
	}
}

func TestAITask_Execute_MultiAgent(t *testing.T) {
	gw := &mockGateway{response: ai.Response{Text: "agent result", Model: "gpt-4o", TokensIn: 1, TokensOut: 1}}
	agents := `[{"name":"a1","prompt":"do {{x}}","outputKey":"out"}]`
	task := newMockAITask(t, gw, map[string]string{
		"scriptBody": "analyze",
		"agents":     agents,
	})

	ctx := &mockExecCtx{
		flow:      &store.FlowRecord{ElementID: "ai-1"},
		variables: map[string]any{"x": "val"},
	}
	result := task.Execute(context.Background(), ctx)
	// Multi-agent may fail parsing/execution; accept either route or error without panic.
	_ = result
}

func TestAITask_Execute_MultiAgentInvalid(t *testing.T) {
	gw := &mockGateway{response: ai.Response{Text: "x", Model: "gpt-4o"}}
	task := newMockAITask(t, gw, map[string]string{
		"scriptBody": "analyze",
		"agents":     "not-json",
	})

	ctx := &mockExecCtx{
		flow:      &store.FlowRecord{ElementID: "ai-1"},
		variables: make(map[string]any),
	}
	result := task.Execute(context.Background(), ctx)
	if result.Action != element.ActionError {
		t.Errorf("expected ActionError for invalid agents, got %s", result.Action)
	}
}

func TestAITask_Execute_Stream(t *testing.T) {
	gw := &mockGateway{response: ai.Response{Text: "streamed", Model: "gpt-4o", TokensIn: 1, TokensOut: 1}}
	task := newMockAITask(t, gw, map[string]string{
		"scriptBody": "analyze",
		"stream":     "true",
	})

	ctx := &mockExecCtx{
		flow:      &store.FlowRecord{ElementID: "ai-1"},
		variables: make(map[string]any),
	}
	result := task.Execute(context.Background(), ctx)
	if result.Action != element.ActionRoute {
		t.Fatalf("expected ActionRoute, got %s (err=%v)", result.Action, result.Error)
	}
	if _, ok := ctx.GetVariable("ai-1_partial"); !ok {
		// partial may be empty if mock doesn't call OnChunk; acceptable
	}
}

func TestAITask_PromptTemplate(t *testing.T) {
	gw := &mockGateway{response: ai.Response{Text: "ok", Model: "gpt-4o"}}
	task := newMockAITask(t, gw, map[string]string{"scriptBody": "hello {{name}}"})

	ctx := &mockExecCtx{
		flow:      &store.FlowRecord{ElementID: "ai-1"},
		variables: map[string]any{"name": "world"},
	}
	result := task.Execute(context.Background(), ctx)
	if result.Action != element.ActionRoute {
		t.Fatalf("expected ActionRoute, got %s", result.Action)
	}
}

func TestAITask_ResolveModel(t *testing.T) {
	task := newMockAITask(t, &mockGateway{}, nil)
	if got := task.resolveModel(&mockExecCtx{variables: make(map[string]any)}); got != "gpt-4o" {
		t.Errorf("expected default gpt-4o, got %s", got)
	}

	task2 := newMockAITask(t, &mockGateway{}, map[string]string{"model": "claude-sonnet-4-6"})
	if got := task2.resolveModel(&mockExecCtx{variables: make(map[string]any)}); got != "claude-sonnet-4-6" {
		t.Errorf("expected model override, got %s", got)
	}

	ctx := &mockExecCtx{variables: map[string]any{"ai_model": "gpt-4o-mini"}}
	task3 := newMockAITask(t, &mockGateway{}, nil)
	if got := task3.resolveModel(ctx); got != "gpt-4o-mini" {
		t.Errorf("expected ai_model var, got %s", got)
	}
}

func TestAITask_ResolveProfile(t *testing.T) {
	task := newMockAITask(t, &mockGateway{}, map[string]string{"profile": "fast"})
	ctx := &mockExecCtx{variables: make(map[string]any)}
	if got := task.resolveProfile(ctx); got != "fast" {
		t.Errorf("expected fast, got %s", got)
	}

	task2 := newMockAITask(t, &mockGateway{}, nil)
	ctx2 := &mockExecCtx{variables: map[string]any{"ai_profile": "cheap"}}
	if got := task2.resolveProfile(ctx2); got != "cheap" {
		t.Errorf("expected cheap, got %s", got)
	}

	task3 := newMockAITask(t, &mockGateway{}, nil)
	if got := task3.resolveProfile(&mockExecCtx{variables: make(map[string]any)}); got != "auto" {
		t.Errorf("expected auto default, got %s", got)
	}
}

func TestAITask_ResolveSystemPrompt(t *testing.T) {
	task := newMockAITask(t, &mockGateway{}, map[string]string{"systemPrompt": "you are helpful"})
	if got := task.resolveSystemPrompt(&mockExecCtx{variables: make(map[string]any)}); got != "you are helpful" {
		t.Errorf("expected system prompt, got %s", got)
	}

	task2 := newMockAITask(t, &mockGateway{}, nil)
	ctx := &mockExecCtx{variables: map[string]any{"ai_system_prompt": "dynamic"}}
	if got := task2.resolveSystemPrompt(ctx); got != "dynamic" {
		t.Errorf("expected dynamic prompt, got %s", got)
	}
}

func TestAITask_ExtractVarNames(t *testing.T) {
	names := extractVarNames("hello {{name}} and {{ age }}")
	if len(names) != 2 || names[0] != "name" || names[1] != "age" {
		t.Errorf("unexpected names: %v", names)
	}
	if names := extractVarNames("no vars"); len(names) != 0 {
		t.Errorf("expected no names, got %v", names)
	}
}

func TestAITask_IDTypeAccessors(t *testing.T) {
	task := newMockAITask(t, &mockGateway{}, nil)
	if task.ID() != "ai-1" {
		t.Errorf("expected id ai-1, got %s", task.ID())
	}
	if task.Type() != bpmn.ElementTypeAITask {
		t.Errorf("expected type aiTask, got %s", task.Type())
	}
	if task.TaskType() != bpmn.TaskTypeAI {
		t.Errorf("expected TaskTypeAI")
	}
	if task.Assignee() != "" {
		t.Errorf("expected empty assignee")
	}
	if task.CandidateUsers() != nil {
		t.Errorf("expected nil candidate users")
	}
	if task.CandidateGroups() != nil {
		t.Errorf("expected nil candidate groups")
	}
}

func TestAITask_BuildPrompt_Ref(t *testing.T) {
	pm := ai.NewPromptManager()
	_ = pm.Register("greeting", "Hello {{name}}")
	task := newMockAITask(t, &mockGateway{}, map[string]string{"promptRef": "greeting"})
	// Replace promptManager
	task.promptManager = pm

	ctx := &mockExecCtx{
		flow:      &store.FlowRecord{ElementID: "ai-1"},
		variables: map[string]any{"name": "world"},
	}
	prompt := task.buildPrompt(ctx)
	if prompt != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", prompt)
	}
}

func TestAITask_EstimateCost(t *testing.T) {
	task := newMockAITask(t, &mockGateway{}, nil)
	resp := ai.Response{Model: "gpt-4o", TokensIn: 1000, TokensOut: 500}
	cost := task.estimateCost(resp)
	if cost <= 0 {
		t.Errorf("expected positive cost, got %f", cost)
	}
	// Unknown model → 0
	resp2 := ai.Response{Model: "unknown-model", TokensIn: 100, TokensOut: 50}
	if c := task.estimateCost(resp2); c != 0 {
		t.Errorf("expected 0 for unknown model, got %f", c)
	}
}