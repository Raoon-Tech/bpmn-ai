package activities

import (
	"context"

	"github.com/Raoon-Tech/bpmn-ai/internal/element"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

type CallActivity struct {
	id            string
	name          string
	calledElement string
}

func NewCallActivity(elem bpmn.Element) (element.Element, error) {
	return &CallActivity{
		id:            elem.ID,
		name:          elem.Name,
		calledElement: elem.CalledElement,
	}, nil
}

func (ca *CallActivity) ID() string {
	return ca.id
}

func (ca *CallActivity) Type() bpmn.ElementType {
	return bpmn.ElementTypeCallActivity
}

func (ca *CallActivity) Execute(_ context.Context, execCtx element.ExecutionContext) element.ExecutionResult {
	flow := execCtx.Flow()
	flow.Status = store.FlowStatusCompleted

	if ca.calledElement == "" {
		return element.ExecutionResult{
			Action:   element.ActionRoute,
			FlowData: flow,
		}
	}

	return element.ExecutionResult{
		Action:        element.ActionCallActivity,
		FlowData:      flow,
		CalledElement: ca.calledElement,
	}
}
