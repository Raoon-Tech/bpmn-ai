package activities

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Raoon-Tech/bpmn-ai/internal/element"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

type mockExecCtx struct {
	flow      *store.FlowRecord
	elem      bpmn.Element
	instance  element.Instance
	stored    element.ElementStore
	variables map[string]any
}

func (m *mockExecCtx) Instance() element.Instance {
	return m.instance
}

func (m *mockExecCtx) Flow() *store.FlowRecord {
	return m.flow
}

func (m *mockExecCtx) GetVariable(key string) (any, bool) {
	v, ok := m.variables[key]
	return v, ok
}

func (m *mockExecCtx) SetVariable(key string, value any) {
	if m.variables == nil {
		m.variables = make(map[string]any)
	}
	m.variables[key] = value
}

func (m *mockExecCtx) Store() element.ElementStore {
	return m.stored
}

func (m *mockExecCtx) Element() (bpmn.Element, bool) {
	return m.elem, true
}

func TestUserTask_Execute(t *testing.T) {
	tests := []struct {
		name           string
		assignee       string
		candidateUsers []string
		candidateGroups []string
	}{
		{
			name:     "with assignee",
			assignee: "user-1",
		},
		{
			name:           "with candidates",
			candidateUsers: []string{"user-a", "user-b"},
			candidateGroups: []string{"group-x"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			elem := bpmn.Element{
				ID:             "task-1",
				Name:           "Review Document",
				TaskType:       bpmn.TaskTypeUser,
				Assignee:       tt.assignee,
				CandidateUsers: tt.candidateUsers,
				CandidateGroups: tt.candidateGroups,
			}
	rawTask, err := NewUserTask(elem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	task := rawTask.(*UserTask)

	if task.ID() != "task-1" {
		t.Errorf("expected ID task-1, got %s", task.ID())
	}
	if task.Type() != bpmn.ElementTypeUserTask {
		t.Errorf("expected type userTask, got %s", task.Type())
	}
	if task.TaskType() != bpmn.TaskTypeUser {
		t.Errorf("expected TaskTypeUser, got %s", task.TaskType())
	}
	if task.Assignee() != tt.assignee {
		t.Errorf("expected assignee %s, got %s", tt.assignee, task.Assignee())
	}

			ctx := &mockExecCtx{
				flow:      &store.FlowRecord{ElementID: "task-1"},
				variables: make(map[string]any),
			}
			result := task.Execute(context.Background(), ctx)

			if result.Action != element.ActionForm {
				t.Errorf("expected ActionForm, got %s", result.Action)
			}
		})
	}
}

func TestScriptTask_Execute(t *testing.T) {
	elem := bpmn.Element{
		ID:       "script-1",
		Name:     "Run Script",
		TaskType: bpmn.TaskTypeScript,
		ExtensionData: map[string]string{
			"scriptBody": "1 + 1",
		},
	}
	rawTask, err := NewScriptTask(elem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	task := rawTask.(*ScriptTask)

	if task.ID() != "script-1" {
		t.Errorf("expected ID script-1, got %s", task.ID())
	}
	if task.Type() != bpmn.ElementTypeScriptTask {
		t.Errorf("expected type scriptTask, got %s", task.Type())
	}
	if task.TaskType() != bpmn.TaskTypeScript {
		t.Errorf("expected TaskTypeScript, got %s", task.TaskType())
	}

	ctx := &mockExecCtx{
		flow:      &store.FlowRecord{ElementID: "script-1"},
		variables: make(map[string]any),
	}
	result := task.Execute(context.Background(), ctx)

	if result.Action != element.ActionRoute {
		t.Errorf("expected ActionRoute, got %s", result.Action)
	}

	val, ok := ctx.GetVariable("script_result")
	if !ok {
		t.Fatalf("expected script_result to be set")
	}
	if val != float64(2) {
		t.Errorf("expected script_result=2, got %v", val)
	}
}

func TestScriptTask_ChangeField(t *testing.T) {
	elem := bpmn.Element{
		ID:       "script-1",
		Name:     "Change Field",
		TaskType: bpmn.TaskTypeScript,
		ExtensionData: map[string]string{
			"scriptBody": "status=approved",
			"scriptType": "change_field",
		},
	}
	rawTask, err := NewScriptTask(elem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	task := rawTask.(*ScriptTask)

	ctx := &mockExecCtx{
		flow:      &store.FlowRecord{ElementID: "script-1"},
		variables: make(map[string]any),
	}
	result := task.Execute(context.Background(), ctx)

	if result.Action != element.ActionRoute {
		t.Errorf("expected ActionRoute, got %s", result.Action)
	}

	val, ok := ctx.GetVariable("status")
	if !ok || val != "approved" {
		t.Errorf("expected status='approved', got %v", val)
	}
}

func TestScriptTask_AssignUser(t *testing.T) {
	elem := bpmn.Element{
		ID:       "script-1",
		Name:     "Assign User",
		TaskType: bpmn.TaskTypeScript,
		ExtensionData: map[string]string{
			"scriptBody": "user-42",
			"scriptType": "assign_user",
		},
	}
	rawTask, err := NewScriptTask(elem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	task := rawTask.(*ScriptTask)

	ctx := &mockExecCtx{
		flow:      &store.FlowRecord{ElementID: "script-1"},
		variables: make(map[string]any),
	}
	result := task.Execute(context.Background(), ctx)

	if result.Action != element.ActionRoute {
		t.Errorf("expected ActionRoute, got %s", result.Action)
	}

	val, ok := ctx.GetVariable("assigned_user")
	if !ok || val != "user-42" {
		t.Errorf("expected assigned_user='user-42', got %v", val)
	}
}

func TestScriptTask_NoScript(t *testing.T) {
	elem := bpmn.Element{
		ID:       "script-1",
		Name:     "Empty Script",
		TaskType: bpmn.TaskTypeScript,
	}
	rawTask, err := NewScriptTask(elem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	task := rawTask.(*ScriptTask)

	ctx := &mockExecCtx{
		flow:      &store.FlowRecord{ElementID: "script-1"},
		variables: make(map[string]any),
	}
	result := task.Execute(context.Background(), ctx)

	if result.Action != element.ActionRoute {
		t.Errorf("expected ActionRoute, got %s", result.Action)
	}
}

func TestServiceTask_Execute(t *testing.T) {
	elem := bpmn.Element{
		ID:       "svc-1",
		Name:     "Call API",
		TaskType: bpmn.TaskTypeService,
	}
	rawTask, err := NewServiceTask(elem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	task := rawTask.(*ServiceTask)

	if task.ID() != "svc-1" {
		t.Errorf("expected ID svc-1, got %s", task.ID())
	}
	if task.Type() != bpmn.ElementTypeServiceTask {
		t.Errorf("expected type serviceTask, got %s", task.Type())
	}
	if task.TaskType() != bpmn.TaskTypeService {
		t.Errorf("expected TaskTypeService, got %s", task.TaskType())
	}

	result := task.Execute(context.Background(), &mockExecCtx{
		flow: &store.FlowRecord{ElementID: "svc-1"},
	})

	if result.Action != element.ActionQueue {
		t.Errorf("expected ActionQueue, got %s", result.Action)
	}
}

func TestSubProcess_Execute(t *testing.T) {
	elem := bpmn.Element{
		ID: "sp-1", Name: "Sub Process",
		SubProcess:    &bpmn.Process{ID: "sp-1"},
		SubProcessEnd: "sp-1.end-1",
	}
	s, err := NewSubProcess(elem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.ID() != "sp-1" {
		t.Errorf("expected ID sp-1, got %s", s.ID())
	}
	if s.Type() != bpmn.ElementTypeSubProcess {
		t.Errorf("expected type subProcess, got %s", s.Type())
	}
	result := s.Execute(context.Background(), &mockExecCtx{
		flow: &store.FlowRecord{ElementID: "sp-1"},
	})
	if result.Action != element.ActionRoute {
		t.Errorf("expected ActionRoute, got %s", result.Action)
	}
	if len(result.FlowFilters) != 1 || result.FlowFilters[0] != "sp-1_sp_entry" {
		t.Errorf("expected FlowFilters [sp-1_sp_entry], got %v", result.FlowFilters)
	}
}

func TestCallActivity_Execute_WithCalledElement(t *testing.T) {
	elem := bpmn.Element{
		ID: "call-1", Name: "Call Activity",
		CalledElement: "called-proc",
	}
	ca, err := NewCallActivity(elem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ca.ID() != "call-1" {
		t.Errorf("expected ID call-1, got %s", ca.ID())
	}
	if ca.Type() != bpmn.ElementTypeCallActivity {
		t.Errorf("expected type callActivity, got %s", ca.Type())
	}
	result := ca.Execute(context.Background(), &mockExecCtx{
		flow: &store.FlowRecord{ElementID: "call-1"},
	})
	if result.Action != element.ActionCallActivity {
		t.Errorf("expected ActionCallActivity, got %s", result.Action)
	}
	if result.CalledElement != "called-proc" {
		t.Errorf("expected CalledElement=called-proc, got %s", result.CalledElement)
	}
}

func TestCallActivity_Execute_NoCalledElement(t *testing.T) {
	elem := bpmn.Element{
		ID: "call-1", Name: "Call Activity",
		CalledElement: "",
	}
	ca, err := NewCallActivity(elem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	result := ca.Execute(context.Background(), &mockExecCtx{
		flow: &store.FlowRecord{ElementID: "call-1"},
	})
	if result.Action != element.ActionRoute {
		t.Errorf("expected ActionRoute for empty calledElement, got %s", result.Action)
	}
}

func TestScriptTask_BusinessRule(t *testing.T) {
	elem := bpmn.Element{
		ID: "script-1", Name: "Business Rule",
		TaskType: bpmn.TaskTypeScript,
		ExtensionData: map[string]string{
			"scriptBody": "a + b",
			"scriptType": "business_rule",
		},
	}
	task, _ := NewScriptTask(elem)
	ctx := &mockExecCtx{
		flow:      &store.FlowRecord{ElementID: "script-1"},
		variables: map[string]any{"a": float64(2), "b": float64(3)},
	}
	result := task.Execute(context.Background(), ctx)
	if result.Action != element.ActionRoute {
		t.Errorf("expected ActionRoute, got %s", result.Action)
	}
	val, _ := ctx.GetVariable("script_result")
	if val != float64(5) {
		t.Errorf("expected 5, got %v", val)
	}
}

func TestScriptTask_BusinessRule_Invalid(t *testing.T) {
	elem := bpmn.Element{
		ID: "script-1", Name: "Business Rule",
		TaskType: bpmn.TaskTypeScript,
		ExtensionData: map[string]string{
			"scriptBody": "a +",
			"scriptType": "business_rule",
		},
	}
	task, _ := NewScriptTask(elem)
	ctx := &mockExecCtx{
		flow:      &store.FlowRecord{ElementID: "script-1"},
		variables: make(map[string]any),
	}
	task.Execute(context.Background(), ctx)
	val, _ := ctx.GetVariable("script_result")
	if !strings.HasPrefix(fmt.Sprintf("%v", val), "error:") {
		t.Errorf("expected error result, got %v", val)
	}
}

func TestScriptTask_BusinessRule_EvalError(t *testing.T) {
	elem := bpmn.Element{
		ID: "script-1", Name: "Business Rule",
		TaskType: bpmn.TaskTypeScript,
		ExtensionData: map[string]string{
			"scriptBody": "a + b",
			"scriptType": "business_rule",
		},
	}
	task, _ := NewScriptTask(elem)
	ctx := &mockExecCtx{
		flow:      &store.FlowRecord{ElementID: "script-1"},
		variables: map[string]any{"a": float64(1)}, // b missing
	}
	task.Execute(context.Background(), ctx)
	val, _ := ctx.GetVariable("script_result")
	if !strings.HasPrefix(fmt.Sprintf("%v", val), "error:") {
		t.Errorf("expected eval error, got %v", val)
	}
}

func TestScriptTask_ChangeField_InvalidFormat(t *testing.T) {
	elem := bpmn.Element{
		ID: "script-1", Name: "Change Field",
		TaskType: bpmn.TaskTypeScript,
		ExtensionData: map[string]string{
			"scriptBody": "noequals",
			"scriptType": "change_field",
		},
	}
	task, _ := NewScriptTask(elem)
	ctx := &mockExecCtx{
		flow:      &store.FlowRecord{ElementID: "script-1"},
		variables: make(map[string]any),
	}
	task.Execute(context.Background(), ctx)
	val, _ := ctx.GetVariable("script_result")
	if !strings.HasPrefix(fmt.Sprintf("%v", val), "error:") {
		t.Errorf("expected error, got %v", val)
	}
}

func TestScriptTask_AssignTeam(t *testing.T) {
	elem := bpmn.Element{
		ID: "script-1", Name: "Assign Team",
		TaskType: bpmn.TaskTypeScript,
		ExtensionData: map[string]string{
			"scriptBody": "team-7",
			"scriptType": "assign_team",
		},
	}
	task, _ := NewScriptTask(elem)
	ctx := &mockExecCtx{
		flow:      &store.FlowRecord{ElementID: "script-1"},
		variables: make(map[string]any),
	}
	task.Execute(context.Background(), ctx)
	val, _ := ctx.GetVariable("assigned_team")
	if val != "team-7" {
		t.Errorf("expected team-7, got %v", val)
	}
}

func TestScriptTask_AddRelated(t *testing.T) {
	elem := bpmn.Element{
		ID: "script-1", Name: "Add Related",
		TaskType: bpmn.TaskTypeScript,
		ExtensionData: map[string]string{
			"scriptBody": "entity-9",
			"scriptType": "add_related",
		},
	}
	task, _ := NewScriptTask(elem)
	ctx := &mockExecCtx{
		flow:      &store.FlowRecord{ElementID: "script-1"},
		variables: make(map[string]any),
	}
	task.Execute(context.Background(), ctx)
	val, _ := ctx.GetVariable("related_entity")
	if val != "entity-9" {
		t.Errorf("expected entity-9, got %v", val)
	}
}

func TestScriptTask_ExpressionError(t *testing.T) {
	elem := bpmn.Element{
		ID: "script-1", Name: "Bad Expr",
		TaskType: bpmn.TaskTypeScript,
		ExtensionData: map[string]string{
			"scriptBody": "1 +",
		},
	}
	task, _ := NewScriptTask(elem)
	ctx := &mockExecCtx{
		flow:      &store.FlowRecord{ElementID: "script-1"},
		variables: make(map[string]any),
	}
	task.Execute(context.Background(), ctx)
	val, _ := ctx.GetVariable("script_result")
	if !strings.HasPrefix(fmt.Sprintf("%v", val), "error:") {
		t.Errorf("expected error, got %v", val)
	}
}

func TestScriptTask_EvalExpressionError(t *testing.T) {
	elem := bpmn.Element{
		ID: "script-1", Name: "Expr Error",
		TaskType: bpmn.TaskTypeScript,
		ExtensionData: map[string]string{
			"scriptBody": "x + 1",
		},
	}
	task, _ := NewScriptTask(elem)
	ctx := &mockExecCtx{
		flow:      &store.FlowRecord{ElementID: "script-1"},
		variables: make(map[string]any), // x missing -> eval error
	}
	task.Execute(context.Background(), ctx)
	val, _ := ctx.GetVariable("script_result")
	if !strings.HasPrefix(fmt.Sprintf("%v", val), "error:") {
		t.Errorf("expected error, got %v", val)
	}
}

func TestUserTask_Accessors(t *testing.T) {
	elem := bpmn.Element{
		ID:             "ut-1",
		Name:           "Review",
		TaskType:       bpmn.TaskTypeUser,
		Assignee:       "user-1",
		CandidateUsers: []string{"u1", "u2"},
		CandidateGroups: []string{"g1"},
	}
	rawUt, _ := NewUserTask(elem)
	task := rawUt.(*UserTask)
	if task.CandidateUsers() == nil || len(task.CandidateUsers()) != 2 {
		t.Errorf("expected 2 candidate users, got %v", task.CandidateUsers())
	}
	if task.CandidateGroups() == nil || len(task.CandidateGroups()) != 1 {
		t.Errorf("expected 1 candidate group, got %v", task.CandidateGroups())
	}
}

func TestServiceTask_Accessors(t *testing.T) {
	elem := bpmn.Element{
		ID:       "svc-1",
		Name:     "Call API",
		TaskType: bpmn.TaskTypeService,
	}
	rawSvc, _ := NewServiceTask(elem)
	task := rawSvc.(*ServiceTask)
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

func TestScriptTask_Accessors(t *testing.T) {
	elem := bpmn.Element{ID: "s-1", Name: "Script", TaskType: bpmn.TaskTypeScript}
	rawSc, _ := NewScriptTask(elem)
	task := rawSc.(*ScriptTask)
	if task.Assignee() != "" {
		t.Errorf("expected empty assignee")
	}
	if task.CandidateUsers() != nil {
		t.Errorf("expected nil candidate users")
	}
	if task.CandidateGroups() != nil {
		t.Errorf("expected nil candidate groups")
	}
	if task.TaskType() != bpmn.TaskTypeScript {
		t.Errorf("expected TaskTypeScript")
	}
}
