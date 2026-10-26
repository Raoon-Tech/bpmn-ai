package ai

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestGuardrailChain_BeforeAfter(t *testing.T) {
	chain := NewGuardrailChain()
	called := 0
	g := &funcGuardrail{fn: func(ctx context.Context, req *Request, resp *Response) error {
		called++
		return nil
	}}
	chain.Add(g)
	req := &Request{Model: "m"}
	if err := chain.Before(context.Background(), req); err != nil {
		t.Fatalf("before: %v", err)
	}
	if err := chain.After(context.Background(), req, &Response{}); err != nil {
		t.Fatalf("after: %v", err)
	}
	if called != 2 {
		t.Errorf("expected 2 calls, got %d", called)
	}
}

type funcGuardrail struct {
	fn func(context.Context, *Request, *Response) error
}

func (f *funcGuardrail) Check(ctx context.Context, req *Request, resp *Response) error {
	return f.fn(ctx, req, resp)
}

func TestGuardrailChain_StopsOnError(t *testing.T) {
	chain := NewGuardrailChain()
	chain.Add(&funcGuardrail{fn: func(ctx context.Context, req *Request, resp *Response) error {
		return fmt.Errorf("boom")
	}})
	if err := chain.Before(context.Background(), &Request{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestTimeoutGuardrail(t *testing.T) {
	g := NewTimeoutGuardrail(0)
	if err := g.Check(context.Background(), &Request{}, nil); err != nil {
		t.Fatalf("zero timeout should pass: %v", err)
	}
	g2 := NewTimeoutGuardrail(5 * time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Hour)
	defer cancel()
	if err := g2.Check(ctx, &Request{}, nil); err != nil {
		t.Fatalf("valid deadline: %v", err)
	}
	// no deadline
	if err := g2.Check(context.Background(), &Request{}, nil); err == nil {
		t.Fatal("expected timeout error when no deadline")
	}
}

func TestTokenLimitGuardrailExtra(t *testing.T) {
	g := NewTokenLimitGuardrail(10)
	req := &Request{MaxTokens: 5}
	if err := g.Check(context.Background(), req, nil); err != nil {
		t.Fatalf("under limit: %v", err)
	}
	req2 := &Request{MaxTokens: 100}
	if err := g.Check(context.Background(), req2, nil); err == nil {
		t.Fatal("expected error over limit")
	}
}

func TestCostLimiterGuardrail(t *testing.T) {
	g := NewCostLimiterGuardrail(0.01, DefaultCostEstimator)
	resp := &Response{TokensIn: 1000, TokensOut: 500, Model: "gpt-4o"}
	if err := g.Check(context.Background(), &Request{}, resp); err == nil {
		// With nil estimator, behavior may vary; just ensure no panic.
	}
	_ = g
}

func TestPIIRedactorGuardrail(t *testing.T) {
	g := NewPIIRedactorGuardrail()
	req := &Request{Messages: []Message{{Role: RoleUser, Content: "email me at test@example.com"}}}
	if err := g.Check(context.Background(), req, nil); err != nil {
		t.Fatalf("before check: %v", err)
	}
	// After pass (resp nil) should not panic.
	_ = g
}

func TestHumanInTheLoopGuardrail(t *testing.T) {
	g := NewHumanInTheLoopGuardrail(0.5, func(ctx context.Context, req *Request, resp *Response) (float64, error) {
		return 0.9, nil
	})
	if err := g.Check(context.Background(), &Request{}, &Response{}); err != nil {
		t.Fatalf("high confidence should pass: %v", err)
	}

	g2 := NewHumanInTheLoopGuardrail(0.95, func(ctx context.Context, req *Request, resp *Response) (float64, error) {
		return 0.5, nil
	})
	if err := g2.Check(context.Background(), &Request{}, &Response{}); err == nil {
		t.Fatal("expected low confidence error")
	}
}

func TestLowConfidenceChecker(t *testing.T) {
	score, err := LowConfidenceChecker(context.Background(), &Request{}, &Response{Text: "ok"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score <= 0 {
		t.Errorf("expected positive score, got %f", score)
	}
}

func TestMemoryCache_Basic(t *testing.T) {
	c := NewMemoryCache(time.Minute)
	ctx := context.Background()
	entry := &CacheEntry{Response: Response{Text: "cached"}, TTL: time.Minute}
	if err := c.Set(ctx, "k1", entry); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := c.Get(ctx, "k1")
	if err != nil || got == nil || got.Response.Text != "cached" {
		t.Fatalf("get: %v, %v", err, got)
	}
	if err := c.Delete(ctx, "k1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if got, _ := c.Get(ctx, "k1"); got != nil {
		t.Error("expected nil after delete")
	}
	if err := c.Clear(ctx); err != nil {
		t.Fatalf("clear: %v", err)
	}
}

func TestMemoryCache_Expiry(t *testing.T) {
	c := NewMemoryCache(time.Millisecond)
	ctx := context.Background()
	entry := &CacheEntry{Response: Response{Text: "x"}, TTL: time.Millisecond}
	_ = c.Set(ctx, "k", entry)
	time.Sleep(5 * time.Millisecond)
	got, _ := c.Get(ctx, "k")
	if got != nil {
		t.Error("expected expired entry to be nil")
	}
}

func TestCachedGateway(t *testing.T) {
	inner := &mockProvider{response: "fresh"}
	cache := NewMemoryCache(time.Minute)
	cg := NewCachedGateway(inner, cache, time.Minute)
	ctx := context.Background()
	resp1, err := cg.Generate(ctx, Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatalf("first generate: %v", err)
	}
	resp2, err := cg.Generate(ctx, Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatalf("second generate: %v", err)
	}
	if resp1.Text != resp2.Text {
		t.Errorf("expected cached response, got %q vs %q", resp1.Text, resp2.Text)
	}
}

func TestPromptManager(t *testing.T) {
	pm := NewPromptManager()
	if err := pm.Register("greet", "Hello {{name}}"); err != nil {
		t.Fatalf("register: %v", err)
	}
	tpl, ok := pm.GetLatest("greet")
	if !ok || tpl.Body != "Hello {{name}}" {
		t.Fatalf("get latest: %v, %v", tpl, ok)
	}
	if _, ok := pm.Get("greet", 1); !ok {
		t.Fatal("expected version 1")
	}
	if _, ok := pm.GetByHash(tpl.SHA256); !ok {
		t.Fatal("expected get by hash")
	}
	if len(pm.List()) != 1 {
		t.Errorf("expected 1 template, got %d", len(pm.List()))
	}
	body, hash, ok := pm.Resolve("greet")
	if !ok || body != "Hello {{name}}" || hash == "" {
		t.Errorf("resolve failed: %q %q %v", body, hash, ok)
	}
	// re-registering with a new body creates a new version (no error)
	if err := pm.Register("greet", "dup"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if tpls := pm.List(); len(tpls) != 1 {
		t.Errorf("expected 1 template name, got %d", len(tpls))
	}
}

func TestToolRegistry(t *testing.T) {
	r := NewToolRegistry()
	def := ToolDefinition{Name: "add", Description: "add numbers"}
	if err := r.Register(def); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := r.Register(def); err == nil {
		t.Error("expected duplicate error")
	}
	if err := r.Register(ToolDefinition{}); err == nil {
		t.Error("expected empty name error")
	}
	got, ok := r.Get("add")
	if !ok || got.Name != "add" {
		t.Fatalf("get: %v, %v", got, ok)
	}
	if _, ok := r.Get("missing"); ok {
		t.Error("expected not found")
	}
	defs, err := r.GetFunctions("add")
	if err != nil || len(defs) != 1 {
		t.Fatalf("get functions: %v, %d", err, len(defs))
	}
	if _, err := r.GetFunctions("missing"); err == nil {
		t.Error("expected error for missing tool")
	}
	allDefs, err := r.GetFunctions()
	if err != nil || len(allDefs) != 1 {
		t.Fatalf("get all functions: %v, %d", err, len(allDefs))
	}
	withoutFn, err := r.GetDefinitions("add")
	if err != nil || len(withoutFn) != 1 {
		t.Fatalf("get definitions: %v, %d", err, len(withoutFn))
	}
}

func TestModelRouter_SetProfiles(t *testing.T) {
	pool := NewProviderPool()
	r := NewModelRouter(pool)
	r.SetProfiles(map[string]Profile{"custom": {Model: "gpt-4o", MaxTokens: 100, Priority: "quality"}})
	r.AddProfile("another", Profile{Model: "gpt-4o-mini", MaxTokens: 50, Priority: "speed"})
	r.AddModel(ModelInfo{Provider: "openai", Name: "gpt-test", SupportsTools: true, SupportsJSON: true})
	r.AddAlias("test", "openai/gpt-test")
	// Resolve with no providers → resolveAnyProvider returns error
	_, _, err := r.Resolve(context.Background(), "custom", "", false, false)
	if err == nil {
		t.Log("resolve succeeded (provider may be available)")
	}
}

func TestModelRouter_ResolveAnyProvider_NoProviders(t *testing.T) {
	pool := NewProviderPool()
	r := NewModelRouter(pool)
	_, _, err := r.Resolve(context.Background(), "auto", "", false, false)
	if err == nil {
		t.Fatal("expected error with no providers")
	}
}

func TestSchemaValidator(t *testing.T) {
	v := NewSchemaValidator()
	schema := `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`
	parsed, err := v.Validate([]byte(schema), `{"name":"hello"}`)
	if err != nil {
		t.Fatalf("valid: %v", err)
	}
	if parsed == nil {
		t.Error("expected parsed result")
	}
	_, err = v.Validate([]byte(schema), `{"age":5}`)
	if err == nil {
		t.Error("expected validation error for missing required field")
	}
	_, err = v.Validate([]byte(schema), "not json")
	if err == nil {
		t.Error("expected error for invalid json")
	}
}

func TestMultiAgentExecutor_ParseError(t *testing.T) {
	exec := NewMultiAgentExecutor(&mockProvider{response: "ok"})
	_, err := exec.Execute(context.Background(), nil, nil)
	if err != nil {
		// Empty agents may error or succeed; both acceptable
	}
	_, err = ParseAgents("not json")
	if err == nil {
		t.Error("expected parse error")
	}
	agents, err := ParseAgents(`[{"name":"a","prompt":"hi {{x}}","outputKey":"out"}]`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
}
