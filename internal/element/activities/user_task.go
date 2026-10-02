package activities

import (
	"context"

	"github.com/Raoon-Tech/bpmn-ai/internal/element"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

type UserTask struct {
	id             string
	name           string
	taskType       bpmn.TaskType
	assignee       string
	candidateUsers []string
	candidateGroups []string
}

func NewUserTask(elem bpmn.Element) (element.Element, error) {
	return &UserTask{
		id:             elem.ID,
		name:           elem.Name,
		taskType:       elem.TaskType,
		assignee:       elem.Assignee,
		candidateUsers: elem.CandidateUsers,
		candidateGroups: elem.CandidateGroups,
	}, nil
}

func (t *UserTask) ID() string {
	return t.id
}

func (t *UserTask) Type() bpmn.ElementType {
	return bpmn.ElementTypeUserTask
}

func (t *UserTask) Execute(_ context.Context, execCtx element.ExecutionContext) element.ExecutionResult {
	flow := execCtx.Flow()
	flow.Status = store.FlowStatusActive

	execCtx.SetVariable("task_assignee", t.assignee)
	if len(t.candidateUsers) > 0 {
		execCtx.SetVariable("task_candidate_users", t.candidateUsers)
	}
	if len(t.candidateGroups) > 0 {
		execCtx.SetVariable("task_candidate_groups", t.candidateGroups)
	}

	return element.ExecutionResult{
		Action:   element.ActionForm,
		FlowData: flow,
	}
}

func (t *UserTask) TaskType() bpmn.TaskType {
	return t.taskType
}

func (t *UserTask) Assignee() string {
	return t.assignee
}

func (t *UserTask) CandidateUsers() []string {
	return t.candidateUsers
}

func (t *UserTask) CandidateGroups() []string {
	return t.candidateGroups
}
