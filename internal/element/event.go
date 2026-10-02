package element

import "github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"

// Event represents a BPMN event.
type Event interface {
	Element

	// EventDefinition returns the event definition.
	EventDefinition() bpmn.EventDefinition
}
