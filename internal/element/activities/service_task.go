package activities

import (
	"context"

	"github.com/Raoon-Tech/bpmn-ai/internal/element"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

type ServiceTask struct {
	id       string
	name     string
	taskType bpmn.TaskType
}

func NewServiceTask(elem bpmn.Element) (element.Element, error) {
	return &ServiceTask{
		id:       elem.ID,
		name:     elem.Name,
		taskType: elem.TaskType,
	}, nil
}

func (t *ServiceTask) ID() string {
	return t.id
}

func (t *ServiceTask) Type() bpmn.ElementType {
	return bpmn.ElementTypeServiceTask
}

func (t *ServiceTask) Execute(_ context.Context, execCtx element.ExecutionContext) element.ExecutionResult {
	flow := execCtx.Flow()
	flow.Status = store.FlowStatusActive

	return element.ExecutionResult{
		Action:   element.ActionQueue,
		FlowData: flow,
	}
}

func (t *ServiceTask) TaskType() bpmn.TaskType {
	return t.taskType
}

func (t *ServiceTask) Assignee() string {
	return ""
}

func (t *ServiceTask) CandidateUsers() []string {
	return nil
}

func (t *ServiceTask) CandidateGroups() []string {
	return nil
}
