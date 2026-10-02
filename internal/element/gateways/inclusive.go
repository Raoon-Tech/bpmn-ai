package gateways

import (
	"context"

	"github.com/Knetic/govaluate"
	"github.com/Raoon-Tech/bpmn-ai/internal/element"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

type InclusiveGateway struct {
	id               string
	name             string
	gatewayType      bpmn.GatewayType
	gatewayDirection bpmn.GatewayDirection
	defaultFlowID    string
	conditions       map[string]string
}

func NewInclusiveGateway(elem bpmn.Element) (element.Element, error) {
	conds := make(map[string]string)
	for k, v := range elem.Conditions {
		conds[k] = v
	}
	return &InclusiveGateway{
		id:               elem.ID,
		name:             elem.Name,
		gatewayType:      elem.GatewayType,
		gatewayDirection: elem.GatewayDirection,
		defaultFlowID:    elem.DefaultFlowID,
		conditions:       conds,
	}, nil
}

func (g *InclusiveGateway) ID() string {
	return g.id
}

func (g *InclusiveGateway) Type() bpmn.ElementType {
	return bpmn.ElementTypeInclusiveGateway
}

func (g *InclusiveGateway) Execute(_ context.Context, execCtx element.ExecutionContext) element.ExecutionResult {
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

	outgoing := elem.OutgoingFlows

	// Converging: just pass through (inclusive gateway convergence is implicit)
	if g.gatewayDirection == bpmn.GatewayDirectionConverging {
		return element.ExecutionResult{
			Action:   element.ActionRoute,
			FlowData: flow,
		}
	}

	// Diverging or unset: evaluate conditions on each outgoing
	if len(outgoing) <= 1 {
		return element.ExecutionResult{
			Action:   element.ActionRoute,
			FlowData: flow,
		}
	}

	var matchedFlows []string
	for _, flowID := range outgoing {
		cond, hasCond := g.conditions[flowID]
		if !hasCond || cond == "" {
			matchedFlows = append(matchedFlows, flowID)
			continue
		}
		if g.evaluateCondition(cond, execCtx) {
			matchedFlows = append(matchedFlows, flowID)
		}
	}

	if len(matchedFlows) == 0 && g.defaultFlowID != "" {
		matchedFlows = append(matchedFlows, g.defaultFlowID)
	}

	if len(matchedFlows) == 0 && len(outgoing) > 0 {
		matchedFlows = append(matchedFlows, outgoing[0])
	}

	return element.ExecutionResult{
		Action:      element.ActionRoute,
		FlowData:    flow,
		FlowFilters: matchedFlows,
	}
}

func (g *InclusiveGateway) GatewayType() bpmn.GatewayType {
	return g.gatewayType
}

func (g *InclusiveGateway) DefaultFlowID() string {
	return g.defaultFlowID
}

func (g *InclusiveGateway) Conditions() map[string]string {
	return g.conditions
}

func (g *InclusiveGateway) evaluateCondition(condition string, execCtx element.ExecutionContext) bool {
	if condition == "" {
		return false
	}

	expression, err := govaluate.NewEvaluableExpression(condition)
	if err != nil {
		return false
	}

	params := make(map[string]interface{})
	for _, varName := range expression.Vars() {
		if val, ok := execCtx.GetVariable(varName); ok {
			params[varName] = val
		}
	}

	result, err := expression.Evaluate(params)
	if err != nil {
		return false
	}

	boolResult, ok := result.(bool)
	return ok && boolResult
}
