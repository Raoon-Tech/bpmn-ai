package activities

import (
	"context"
	"fmt"
	"strings"

	"github.com/Knetic/govaluate"

	"github.com/Raoon-Tech/bpmn-ai/internal/element"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

type ScriptTask struct {
	id         string
	name       string
	taskType   bpmn.TaskType
	scriptBody string
	scriptType string
}

func NewScriptTask(elem bpmn.Element) (element.Element, error) {
	st := &ScriptTask{
		id:       elem.ID,
		name:     elem.Name,
		taskType: elem.TaskType,
	}
	if elem.ExtensionData != nil {
		if v, ok := elem.ExtensionData["scriptBody"]; ok {
			st.scriptBody = v
		}
		if v, ok := elem.ExtensionData["scriptType"]; ok {
			st.scriptType = v
		}
	}
	return st, nil
}

func (t *ScriptTask) ID() string {
	return t.id
}

func (t *ScriptTask) Type() bpmn.ElementType {
	return bpmn.ElementTypeScriptTask
}

func (t *ScriptTask) Execute(_ context.Context, execCtx element.ExecutionContext) element.ExecutionResult {
	flow := execCtx.Flow()
	flow.Status = store.FlowStatusCompleted

	if t.scriptBody == "" {
		return element.ExecutionResult{
			Action:   element.ActionRoute,
			FlowData: flow,
		}
	}

	switch bpmn.ScriptType(t.scriptType) {
	case bpmn.ScriptTypeBusinessRule:
		t.evalBusinessRule(execCtx)
	case bpmn.ScriptTypeChangeField:
		t.evalChangeField(execCtx)
	case bpmn.ScriptTypeAssignTeam:
		execCtx.SetVariable("assigned_team", t.scriptBody)
	case bpmn.ScriptTypeAssignUser:
		execCtx.SetVariable("assigned_user", t.scriptBody)
	case bpmn.ScriptTypeAddRelated:
		execCtx.SetVariable("related_entity", t.scriptBody)
	default:
		// Plain script: evaluate as expression
		t.evalExpression(execCtx)
	}

	return element.ExecutionResult{
		Action:   element.ActionRoute,
		FlowData: flow,
	}
}

func (t *ScriptTask) evalBusinessRule(execCtx element.ExecutionContext) {
	vars := collectVars(t.scriptBody, execCtx)
	expression, err := govaluate.NewEvaluableExpression(t.scriptBody)
	if err != nil {
		execCtx.SetVariable("script_result", fmt.Sprintf("error: %v", err))
		return
	}
	result, err := expression.Evaluate(vars)
	if err != nil {
		execCtx.SetVariable("script_result", fmt.Sprintf("error: %v", err))
		return
	}
	execCtx.SetVariable("script_result", result)
}

func (t *ScriptTask) evalChangeField(execCtx element.ExecutionContext) {
	parts := strings.SplitN(t.scriptBody, "=", 2)
	if len(parts) != 2 {
		execCtx.SetVariable("script_result", fmt.Sprintf("error: invalid change_field format, expected field=value"))
		return
	}
	field := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])
	execCtx.SetVariable(field, value)
	execCtx.SetVariable("script_result", "ok")
}

func (t *ScriptTask) evalExpression(execCtx element.ExecutionContext) {
	vars := collectVars(t.scriptBody, execCtx)
	expression, err := govaluate.NewEvaluableExpression(t.scriptBody)
	if err != nil {
		execCtx.SetVariable("script_result", fmt.Sprintf("error: %v", err))
		return
	}
	result, err := expression.Evaluate(vars)
	if err != nil {
		execCtx.SetVariable("script_result", fmt.Sprintf("error: %v", err))
		return
	}
	execCtx.SetVariable("script_result", result)
}

func collectVars(expression string, execCtx element.ExecutionContext) map[string]any {
	expr, err := govaluate.NewEvaluableExpression(expression)
	if err != nil {
		return nil
	}
	vars := make(map[string]any)
	for _, name := range expr.Vars() {
		if val, ok := execCtx.GetVariable(name); ok {
			vars[name] = val
		}
	}
	return vars
}

func (t *ScriptTask) TaskType() bpmn.TaskType {
	return t.taskType
}

func (t *ScriptTask) Assignee() string {
	return ""
}

func (t *ScriptTask) CandidateUsers() []string {
	return nil
}

func (t *ScriptTask) CandidateGroups() []string {
	return nil
}
