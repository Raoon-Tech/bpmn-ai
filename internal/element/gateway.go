package element

import "github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"

// Gateway represents a BPMN gateway.
type Gateway interface {
	Element

	// GatewayType returns the type of gateway.
	GatewayType() bpmn.GatewayType

	// DefaultFlowID returns the default flow ID for diverging gateways.
	DefaultFlowID() string

	// Conditions returns the condition expressions for each outgoing flow.
	Conditions() map[string]string
}
