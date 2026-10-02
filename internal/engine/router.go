package engine

import (
	"github.com/Raoon-Tech/bpmn-ai/internal/element"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

// NextFlow represents a flow that should be executed next.
type NextFlow struct {
	FlowID     string
	ElementID  string
	ElementType bpmn.ElementType
	ThreadID   int
}

// FlowRouter determines which elements to execute next based on execution results.
type FlowRouter struct{}

// NewFlowRouter creates a new flow router.
func NewFlowRouter() *FlowRouter {
	return &FlowRouter{}
}

// Route determines the next flows to execute based on the execution result.
func (r *FlowRouter) Route(result element.ExecutionResult, proc *bpmn.Process, threadID int) []NextFlow {
	if result.Action != element.ActionRoute && result.Action != element.ActionQueue {
		return nil
	}

	currentFlow := result.FlowData
	elem, ok := proc.Elements[currentFlow.ElementID]
	if !ok {
		return nil
	}

	var nextFlows []NextFlow

	for _, flowID := range elem.OutgoingFlows {
		flow, ok := proc.Flows[flowID]
		if !ok {
			continue
		}

		// Apply flow filters if specified
		if len(result.FlowFilters) > 0 {
			if !contains(result.FlowFilters, flowID) {
				continue
			}
		}

		if flowElem, hasFlowElem := proc.Elements[flowID]; hasFlowElem && flowElem.Type == bpmn.ElementTypeSequenceFlow {
			nextFlows = append(nextFlows, NextFlow{
				FlowID:      flowID,
				ElementID:   flowID,
				ElementType: bpmn.ElementTypeSequenceFlow,
				ThreadID:    threadID,
			})
		} else {
			nextFlows = append(nextFlows, NextFlow{
				FlowID:      flowID,
				ElementID:   flow.TargetRef,
				ElementType: proc.Elements[flow.TargetRef].Type,
				ThreadID:    threadID,
			})
		}
	}

	return nextFlows
}

// ResolveElementID resolves the element ID for a flow.
func (r *FlowRouter) ResolveElementID(flowID string, proc *bpmn.Process) string {
	flow, ok := proc.Flows[flowID]
	if !ok {
		return ""
	}
	return flow.TargetRef
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// CreateFlowRecord creates a new flow record for execution.
func CreateFlowRecord(instanceID, elementID string, elemType bpmn.ElementType, threadID int, previousID string) *store.FlowRecord {
	return &store.FlowRecord{
		InstanceID:  instanceID,
		ElementID:   elementID,
		ElementType: elemType,
		ThreadID:    threadID,
		PreviousID:  previousID,
		Status:      store.FlowStatusActive,
	}
}
