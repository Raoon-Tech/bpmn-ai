package gateways

import (
	"context"
	"sync"

	"github.com/Raoon-Tech/bpmn-ai/internal/element"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

var (
	gatewayMu  sync.Mutex
)

type EventBasedGateway struct {
	id               string
	name             string
	gatewayType      bpmn.GatewayType
	gatewayDirection bpmn.GatewayDirection
}

func NewEventBasedGateway(elem bpmn.Element) (element.Element, error) {
	return &EventBasedGateway{
		id:               elem.ID,
		name:             elem.Name,
		gatewayType:      elem.GatewayType,
		gatewayDirection: elem.GatewayDirection,
	}, nil
}

func (g *EventBasedGateway) ID() string {
	return g.id
}

func (g *EventBasedGateway) Type() bpmn.ElementType {
	return bpmn.ElementTypeEventBasedGateway
}

func (g *EventBasedGateway) Execute(_ context.Context, execCtx element.ExecutionContext) element.ExecutionResult {
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

	// Converging EventBasedGateway is invalid in BPMN; treat as pass-through
	if g.gatewayDirection == bpmn.GatewayDirectionConverging {
		return element.ExecutionResult{
			Action:   element.ActionRoute,
			FlowData: flow,
		}
	}

	outgoing := elem.OutgoingFlows

	// If only one outgoing flow, just route normally
	if len(outgoing) <= 1 {
		return element.ExecutionResult{
			Action:   element.ActionRoute,
			FlowData: flow,
		}
	}

	// Multiple outgoing branches — this gateway must decide which event fires first.
	// Arm the gateway by storing the winner and waiting for the first event to resolve.
	execCtx.SetVariable("eventbased_gateway_armed", g.id)
	execCtx.SetVariable("eventbased_gateway_resolved", false)
	execCtx.SetVariable("eventbased_winning_element", "")

	return element.ExecutionResult{
		Action:   element.ActionWait,
		FlowData: flow,
	}
}

func (g *EventBasedGateway) GatewayType() bpmn.GatewayType {
	return g.gatewayType
}

func (g *EventBasedGateway) DefaultFlowID() string {
	return ""
}

func (g *EventBasedGateway) Conditions() map[string]string {
	return nil
}

// CheckAndResolve checks if the given element ID is the first to resolve
// the event-based gateway. Returns true if the element should proceed.
func CheckAndResolve(execCtx element.ExecutionContext, elementID string) bool {
	gatewayMu.Lock()
	defer gatewayMu.Unlock()

	armed, _ := execCtx.GetVariable("eventbased_gateway_armed")
	if armed == nil || armed.(string) == "" {
		return true // not part of an event-based gateway
	}

	resolved, _ := execCtx.GetVariable("eventbased_gateway_resolved")
	if resolved != nil && resolved.(bool) {
		// Already resolved — only the winning element proceeds
		winner, _ := execCtx.GetVariable("eventbased_winning_element")
		return winner != nil && winner.(string) == elementID
	}

	// First to resolve wins
	execCtx.SetVariable("eventbased_gateway_resolved", true)
	execCtx.SetVariable("eventbased_winning_element", elementID)
	return true
}
