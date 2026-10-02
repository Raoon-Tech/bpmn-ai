package gateways

import (
	"context"

	"github.com/Knetic/govaluate"
	"github.com/Raoon-Tech/bpmn-ai/internal/element"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

type ExclusiveGateway struct {
	id               string
	name             string
	gatewayType      bpmn.GatewayType
	gatewayDirection bpmn.GatewayDirection
	defaultFlowID    string
	conditions       map[string]string
}

func NewExclusiveGateway(elem bpmn.Element) (element.Element, error) {
	conds := make(map[string]string)
	for k, v := range elem.Conditions {
		conds[k] = v
	}
	return &ExclusiveGateway{
		id:               elem.ID,
		name:             elem.Name,
		gatewayType:      elem.GatewayType,
		gatewayDirection: elem.GatewayDirection,
		defaultFlowID:    elem.DefaultFlowID,
		conditions:       conds,
	}, nil
}

func (g *ExclusiveGateway) ID() string {
	return g.id
}

func (g *ExclusiveGateway) Type() bpmn.ElementType {
	return bpmn.ElementTypeExclusiveGateway
}

func (g *ExclusiveGateway) Execute(_ context.Context, execCtx element.ExecutionContext) element.ExecutionResult {
	flow := execCtx.Flow()
	flow.Status = store.FlowStatusCompleted

	// Converging: just pass through, no condition evaluation needed
	if g.gatewayDirection == bpmn.GatewayDirectionConverging {
		return element.ExecutionResult{
			Action:   element.ActionRoute,
			FlowData: flow,
		}
	}

	// Diverging or unset: evaluate conditions
	var matchedFlowID string

	for _, flowID := range g.getOutgoingFlowIDs(execCtx) {
		cond, hasCond := g.conditions[flowID]
		if !hasCond || cond == "" {
			continue
		}

		if g.evaluateCondition(cond, execCtx) {
			matchedFlowID = flowID
			break
		}
	}

	if matchedFlowID == "" && g.defaultFlowID != "" {
		matchedFlowID = g.defaultFlowID
	}

	if matchedFlowID == "" {
		matchedFlowID = g.getFirstOutgoingFlowID(execCtx)
	}

	return element.ExecutionResult{
		Action:      element.ActionRoute,
		FlowData:    flow,
		FlowFilters: []string{matchedFlowID},
	}
}

func (g *ExclusiveGateway) GatewayType() bpmn.GatewayType {
	return g.gatewayType
}

func (g *ExclusiveGateway) DefaultFlowID() string {
	return g.defaultFlowID
}

func (g *ExclusiveGateway) Conditions() map[string]string {
	return g.conditions
}

func (g *ExclusiveGateway) SetCondition(flowID, expression string) {
	g.conditions[flowID] = expression
}

func (g *ExclusiveGateway) getOutgoingFlowIDs(execCtx element.ExecutionContext) []string {
	elem, ok := execCtx.Element()
	if !ok {
		return nil
	}
	return elem.OutgoingFlows
}

func (g *ExclusiveGateway) getFirstOutgoingFlowID(execCtx element.ExecutionContext) string {
	elem, ok := execCtx.Element()
	if !ok {
		return ""
	}
	if len(elem.OutgoingFlows) > 0 {
		return elem.OutgoingFlows[0]
	}
	return ""
}

func (g *ExclusiveGateway) evaluateCondition(condition string, execCtx element.ExecutionContext) bool {
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
