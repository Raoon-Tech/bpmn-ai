package activities

import (
	"context"

	"github.com/Raoon-Tech/bpmn-ai/internal/element"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

type SubProcess struct {
	id          string
	name        string
	entryFlowID string
}

func NewSubProcess(elem bpmn.Element) (element.Element, error) {
	entryFlowID := elem.ID + "_sp_entry"
	return &SubProcess{
		id:          elem.ID,
		name:        elem.Name,
		entryFlowID: entryFlowID,
	}, nil
}

func (sp *SubProcess) ID() string {
	return sp.id
}

func (sp *SubProcess) Type() bpmn.ElementType {
	return bpmn.ElementTypeSubProcess
}

func (sp *SubProcess) Execute(_ context.Context, execCtx element.ExecutionContext) element.ExecutionResult {
	flow := execCtx.Flow()
	flow.Status = store.FlowStatusCompleted

	return element.ExecutionResult{
		Action:      element.ActionRoute,
		FlowData:    flow,
		FlowFilters: []string{sp.entryFlowID},
	}
}
