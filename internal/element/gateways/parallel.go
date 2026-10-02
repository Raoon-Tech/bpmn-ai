package gateways

import (
	"context"

	"github.com/Raoon-Tech/bpmn-ai/internal/element"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

type ParallelGateway struct {
	id               string
	name             string
	gatewayType      bpmn.GatewayType
	gatewayDirection bpmn.GatewayDirection
}

func NewParallelGateway(elem bpmn.Element) (element.Element, error) {
	return &ParallelGateway{
		id:               elem.ID,
		name:             elem.Name,
		gatewayType:      elem.GatewayType,
		gatewayDirection: elem.GatewayDirection,
	}, nil
}

func (g *ParallelGateway) ID() string {
	return g.id
}

func (g *ParallelGateway) Type() bpmn.ElementType {
	return bpmn.ElementTypeParallelGateway
}

func (g *ParallelGateway) Execute(ctx context.Context, execCtx element.ExecutionContext) element.ExecutionResult {
	flow := execCtx.Flow()
	flow.Status = store.FlowStatusCompleted

	elem, ok := execCtx.Element()
	if !ok {
		return element.ExecutionResult{
			Action:   element.ActionError,
			FlowData: flow,
			Error:    elementNotFoundError(g.id),
		}
	}

	// Use GatewayDirection if explicitly set
	switch g.gatewayDirection {
	case bpmn.GatewayDirectionDiverging:
		return element.ExecutionResult{
			Action:   element.ActionRoute,
			FlowData: flow,
		}
	case bpmn.GatewayDirectionConverging:
		if g.areAllIncomingComplete(ctx, execCtx) {
			return element.ExecutionResult{
				Action:   element.ActionRoute,
				FlowData: flow,
			}
		}
		return element.ExecutionResult{
			Action:   element.ActionWait,
			FlowData: flow,
		}
	}

	// Fallback: infer from flow structure
	if len(elem.OutgoingFlows) > 1 {
		return element.ExecutionResult{
			Action:   element.ActionRoute,
			FlowData: flow,
		}
	}

	if len(elem.IncomingFlows) > 1 {
		allComplete := g.areAllIncomingComplete(ctx, execCtx)
		if !allComplete {
			return element.ExecutionResult{
				Action:   element.ActionWait,
				FlowData: flow,
			}
		}
	}

	return element.ExecutionResult{
		Action:   element.ActionRoute,
		FlowData: flow,
	}
}

func (g *ParallelGateway) GatewayType() bpmn.GatewayType {
	return g.gatewayType
}

func (g *ParallelGateway) DefaultFlowID() string {
	return ""
}

func (g *ParallelGateway) Conditions() map[string]string {
	return nil
}

func (g *ParallelGateway) areAllIncomingComplete(ctx context.Context, execCtx element.ExecutionContext) bool {
	elem, ok := execCtx.Element()
	if !ok {
		return false
	}

	flows, err := execCtx.Store().GetFlowsByInstance(ctx, execCtx.Flow().InstanceID)
	if err != nil {
		return false
	}

	arrived := 0
	for _, f := range flows {
		if f.ElementID == g.id && f.Status != store.FlowStatusError {
			arrived++
		}
	}

	return arrived >= len(elem.IncomingFlows)
}

func elementNotFoundError(id string) error {
	return &elementNotFound{id: id}
}

type elementNotFound struct {
	id string
}

func (e *elementNotFound) Error() string {
	return "element not found: " + e.id
}
