package activities

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Raoon-Tech/bpmn-ai/internal/element"
	"github.com/Raoon-Tech/bpmn-ai/pkg/ai"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

type AITask struct {
	id           string
	name         string
	taskType     bpmn.TaskType
	model        string
	profile      string
	systemPrompt string
	outputSchema string
	prompt       string
	promptRef    string
	stream       bool
	toolNames    []string
	ragCollection string
	agentDefs    string
	aiGateway    ai.Gateway
	toolRegistry *ai.ToolRegistry
	modelRouter  *ai.ModelRouter
	ragSystem    *ai.RAGSystem
	promptManager *ai.PromptManager
}

func NewAITaskConstructor(gateway ai.Gateway, toolRegistry *ai.ToolRegistry, modelRouter *ai.ModelRouter, ragSystem *ai.RAGSystem, promptManager *ai.PromptManager) func(elem bpmn.Element) (element.Element, error) {
	return func(elem bpmn.Element) (element.Element, error) {
		return newAITask(elem, gateway, toolRegistry, modelRouter, ragSystem, promptManager)
	}
}

func newAITask(elem bpmn.Element, gateway ai.Gateway, toolRegistry *ai.ToolRegistry, modelRouter *ai.ModelRouter, ragSystem *ai.RAGSystem, promptManager *ai.PromptManager) (*AITask, error) {
	t := &AITask{
		id:           elem.ID,
		name:         elem.Name,
		taskType:     elem.TaskType,
		aiGateway:    gateway,
		toolRegistry: toolRegistry,
		modelRouter:  modelRouter,
		ragSystem:    ragSystem,
		promptManager: promptManager,
	}

	if elem.ExtensionData != nil {
		t.prompt = elem.ExtensionData["scriptBody"]
		t.model = elem.ExtensionData["model"]
		t.profile = elem.ExtensionData["profile"]
		t.systemPrompt = elem.ExtensionData["systemPrompt"]
		t.outputSchema = elem.ExtensionData["outputSchema"]
		t.ragCollection = elem.ExtensionData["rag"]
		t.agentDefs = elem.ExtensionData["agents"]
		t.promptRef = elem.ExtensionData["promptRef"]
		t.stream = elem.ExtensionData["stream"] == "true"
		if tools := elem.ExtensionData["tools"]; tools != "" {
			for _, name := range strings.Split(tools, ",") {
				name = strings.TrimSpace(name)
				if name != "" {
					t.toolNames = append(t.toolNames, name)
				}
			}
		}
	}

	if t.prompt == "" && t.promptRef == "" {
		t.prompt = t.name
	}

	return t, nil
}

func (t *AITask) ID() string {
	return t.id
}

func (t *AITask) Type() bpmn.ElementType {
	return bpmn.ElementTypeAITask
}

func (t *AITask) Execute(ctx context.Context, execCtx element.ExecutionContext) element.ExecutionResult {
	flow := execCtx.Flow()

	// Multi-agent mode: execute sub-tasks in sequence
	if t.agentDefs != "" {
		return t.executeMultiAgent(ctx, execCtx, flow)
	}

	// Standard single-agent mode
	prompt := t.buildPrompt(execCtx)
	systemPrompt := t.resolveSystemPrompt(execCtx)
	tools := t.resolveTools()

	// RAG context enrichment
	if t.ragSystem != nil && t.ragCollection != "" {
		enriched, err := t.ragSystem.EnrichPrompt(ctx, t.ragCollection, prompt, 5)
		if err == nil {
			prompt = enriched
		}
	}

	model := t.resolveModel(execCtx)
	profile := t.resolveProfile(execCtx)

	var gw ai.Gateway = t.aiGateway
	var modelName = model

	if t.modelRouter != nil && profile != "" {
		useTools := len(tools) > 0
		useSchema := t.outputSchema != ""
		resolved, resolvedModel, err := t.modelRouter.Resolve(ctx, profile, model, useTools, useSchema)
		if err == nil {
			gw = resolved
			modelName = resolvedModel
		}
	}

	req := ai.Request{
		Model:        modelName,
		System:       systemPrompt,
		Messages:     []ai.Message{{Role: ai.RoleUser, Content: prompt}},
		MaxTokens:    4096,
		Temperature:  0.7,
		Tools:        tools,
		OutputSchema: t.outputSchemaJSON(),
		Stream:       t.stream,
	}
	if t.stream {
		var partial strings.Builder
		req.OnChunk = func(chunk string) {
			partial.WriteString(chunk)
			execCtx.SetVariable(t.id+"_partial", partial.String())
		}
	}
	resp, err := gw.Generate(ctx, req)
	if err != nil {
		flow.Status = store.FlowStatusError
		return element.ExecutionResult{
			Action:   element.ActionError,
			FlowData: flow,
			Error:    fmt.Errorf("ai_task %s: %w", t.id, err),
		}
	}

	t.setOutputVariables(execCtx, resp)

	flow.Status = store.FlowStatusCompleted
	return element.ExecutionResult{
		Action:   element.ActionRoute,
		FlowData: flow,
	}
}

func (t *AITask) executeMultiAgent(ctx context.Context, execCtx element.ExecutionContext, flow *store.FlowRecord) element.ExecutionResult {
	agents, err := ai.ParseAgents(t.agentDefs)
	if err != nil {
		flow.Status = store.FlowStatusError
		return element.ExecutionResult{
			Action:   element.ActionError,
			FlowData: flow,
			Error:    fmt.Errorf("ai_task %s: parse agents: %w", t.id, err),
		}
	}

	executor := ai.NewMultiAgentExecutor(t.aiGateway)

	var globalVars []struct{ k, v string }
	for _, name := range extractVarNames(t.prompt) {
		if val, ok := execCtx.GetVariable(name); ok {
			globalVars = append(globalVars, struct{ k, v string }{name, fmt.Sprintf("%v", val)})
		}
	}
	vars := make(map[string]string)
	for _, kv := range globalVars {
		vars[kv.k] = kv.v
	}

	results, err := executor.Execute(ctx, agents, vars)
	if err != nil {
		flow.Status = store.FlowStatusError
		return element.ExecutionResult{
			Action:   element.ActionError,
			FlowData: flow,
			Error:    fmt.Errorf("ai_task %s: multi-agent: %w", t.id, err),
		}
	}

	for key, val := range results {
		execCtx.SetVariable(t.id+"_agent_"+key, val)
	}

	flow.Status = store.FlowStatusCompleted
	return element.ExecutionResult{
		Action:   element.ActionRoute,
		FlowData: flow,
	}
}

func (t *AITask) setOutputVariables(execCtx element.ExecutionContext, resp ai.Response) {
	resultText := resp.Text

	if t.outputSchema != "" {
		validator := ai.NewSchemaValidator()
		parsed, err := validator.Validate([]byte(t.outputSchema), resultText)
		if err != nil {
			execCtx.SetVariable(t.id+"_validation_error", err.Error())
		} else {
			execCtx.SetVariable(t.id+"_parsed", parsed)
		}
	}

	execCtx.SetVariable(t.id+"_result", resultText)
	execCtx.SetVariable(t.id+"_model", resp.Model)
	execCtx.SetVariable(t.id+"_tokens_in", resp.TokensIn)
	execCtx.SetVariable(t.id+"_tokens_out", resp.TokensOut)

	// Cumulative cost tracking across all AI calls in this instance
	cost := t.estimateCost(resp)
	prevCost := float64(0)
	if val, ok := execCtx.GetVariable("ai_total_cost"); ok {
		switch v := val.(type) {
		case float64:
			prevCost = v
		case int:
			prevCost = float64(v)
		}
	}
	execCtx.SetVariable("ai_total_cost", prevCost+cost)
	execCtx.SetVariable(t.id+"_cost", cost)

	if len(resp.ToolCalls) > 0 {
		toolCallsJSON, _ := json.Marshal(resp.ToolCalls)
		execCtx.SetVariable(t.id+"_tool_calls", string(toolCallsJSON))
	}
}

func (t *AITask) estimateCost(resp ai.Response) float64 {
	pricing := map[string]struct{ in, out float64 }{
		"gpt-4o":             {0.01, 0.03},
		"gpt-4o-mini":        {0.0015, 0.006},
		"gpt-5.5":            {0.03, 0.18},
		"gpt-5.4":            {0.022, 0.14},
		"gpt-5.4-mini":       {0.0068, 0.04},
		"gpt-5.4-nano":       {0.0018, 0.011},
		"claude-sonnet-4-6":  {0.03, 0.15},
		"claude-haiku-4-5":   {0.008, 0.04},
		"claude-opus-4-8":    {0.0429, 0.2146},
	}
	for name, p := range pricing {
		if strings.Contains(resp.Model, name) {
			return float64(resp.TokensIn)/1000*p.in + float64(resp.TokensOut)/1000*p.out
		}
	}
	return 0
}

func (t *AITask) outputSchemaJSON() []byte {
	if t.outputSchema == "" {
		return nil
	}
	return []byte(t.outputSchema)
}

func (t *AITask) resolveTools() []ai.ToolDefinition {
	if t.toolRegistry == nil || len(t.toolNames) == 0 {
		return nil
	}
	defs, err := t.toolRegistry.GetFunctions(t.toolNames...)
	if err != nil {
		return nil
	}
	return defs
}

func (t *AITask) TaskType() bpmn.TaskType {
	return t.taskType
}

func (t *AITask) Assignee() string {
	return ""
}

func (t *AITask) CandidateUsers() []string {
	return nil
}

func (t *AITask) CandidateGroups() []string {
	return nil
}

func (t *AITask) buildPrompt(execCtx element.ExecutionContext) string {
	prompt := t.prompt

	// Resolve prompt-ref from PromptManager if defined
	if t.promptRef != "" && t.promptManager != nil {
		if body, hash, ok := t.promptManager.Resolve(t.promptRef); ok {
			prompt = body
			execCtx.SetVariable(t.id+"_prompt_hash", hash)
			execCtx.SetVariable(t.id+"_prompt_ref", t.promptRef)
		}
	}
	if t.promptRef != "" && prompt == "" {
		prompt = t.name
	}

	for _, name := range extractVarNames(prompt) {
		if val, ok := execCtx.GetVariable(name); ok {
			prompt = strings.ReplaceAll(prompt, "{{"+name+"}}", fmt.Sprintf("%v", val))
		}
	}

	return prompt
}

func (t *AITask) resolveModel(execCtx element.ExecutionContext) string {
	if t.model != "" {
		return t.model
	}
	if val, ok := execCtx.GetVariable("ai_model"); ok {
		if s, ok := val.(string); ok && s != "" {
			return s
		}
	}
	return "gpt-4o"
}

func (t *AITask) resolveProfile(execCtx element.ExecutionContext) string {
	if t.profile != "" {
		return t.profile
	}
	if val, ok := execCtx.GetVariable("ai_profile"); ok {
		if s, ok := val.(string); ok && s != "" {
			return s
		}
	}
	return "auto"
}

func (t *AITask) resolveSystemPrompt(execCtx element.ExecutionContext) string {
	if t.systemPrompt != "" {
		return t.systemPrompt
	}
	if val, ok := execCtx.GetVariable("ai_system_prompt"); ok {
		if s, ok := val.(string); ok {
			return s
		}
	}
	return ""
}

func extractVarNames(s string) []string {
	var names []string
	start := 0
	for {
		open := strings.Index(s[start:], "{{")
		if open < 0 {
			break
		}
		close := strings.Index(s[start+open:], "}}")
		if close < 0 {
			break
		}
		name := strings.TrimSpace(s[start+open+2 : start+open+close])
		if name != "" {
			names = append(names, name)
		}
		start = start + open + close + 2
	}
	return names
}
