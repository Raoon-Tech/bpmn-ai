package flows

import (
	"context"

	"github.com/Raoon-Tech/bpmn-ai/internal/element"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

// SequenceFlow implements the BPMN sequence flow.
type SequenceFlow struct {
	id        string
	name      string
	sourceRef string
	targetRef string
	condition string
	isDefault bool
}

// NewSequenceFlow creates a new sequence flow element.
func NewSequenceFlow(elem bpmn.Element) (element.Element, error) {
	sf := &SequenceFlow{
		id:   elem.ID,
		name: elem.Name,
	}
	if elem.ExtensionData != nil {
		sf.sourceRef = elem.ExtensionData["sourceRef"]
		sf.targetRef = elem.ExtensionData["targetRef"]
		sf.condition = elem.ExtensionData["conditionExpression"]
		sf.isDefault = elem.ExtensionData["isDefault"] == "true"
	}
	return sf, nil
}

// ID returns the element ID.
func (s *SequenceFlow) ID() string {
	return s.id
}

// Type returns the element type.
func (s *SequenceFlow) Type() bpmn.ElementType {
	return bpmn.ElementTypeSequenceFlow
}

// Execute runs the sequence flow logic (pass-through).
func (s *SequenceFlow) Execute(_ context.Context, execCtx element.ExecutionContext) element.ExecutionResult {
	flow := execCtx.Flow()
	flow.Status = store.FlowStatusCompleted

	return element.ExecutionResult{
		Action:   element.ActionRoute,
		FlowData: flow,
	}
}

// SourceRef returns the source element ID.
func (s *SequenceFlow) SourceRef() string {
	return s.sourceRef
}

// TargetRef returns the target element ID.
func (s *SequenceFlow) TargetRef() string {
	return s.targetRef
}

// Condition returns the condition expression.
func (s *SequenceFlow) Condition() string {
	return s.condition
}

// IsDefault returns true if this is the default flow.
func (s *SequenceFlow) IsDefault() bool {
	return s.isDefault
}
