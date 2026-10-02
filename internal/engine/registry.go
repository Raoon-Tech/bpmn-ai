package engine

import (
	"fmt"
	"sync"

	"github.com/Raoon-Tech/bpmn-ai/internal/element"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
)

// ElementConstructor is a function that creates an element from a BPMN definition.
type ElementConstructor func(elem bpmn.Element) (element.Element, error)

// ElementRegistry manages the mapping between BPMN element types and their implementations.
type ElementRegistry struct {
	mu         sync.RWMutex
	constructors map[bpmn.ElementType]ElementConstructor
}

// NewElementRegistry creates a new element registry.
func NewElementRegistry() *ElementRegistry {
	return &ElementRegistry{
		constructors: make(map[bpmn.ElementType]ElementConstructor),
	}
}

// Register registers a constructor for an element type.
func (r *ElementRegistry) Register(elemType bpmn.ElementType, ctor ElementConstructor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.constructors[elemType] = ctor
}

// Get creates an element instance from a BPMN element definition.
func (r *ElementRegistry) Get(elem bpmn.Element) (element.Element, error) {
	r.mu.RLock()
	ctor, ok := r.constructors[elem.Type]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no constructor registered for element type %s", elem.Type)
	}

	return ctor(elem)
}

// Has returns true if a constructor is registered for the element type.
func (r *ElementRegistry) Has(elemType bpmn.ElementType) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.constructors[elemType]
	return ok
}

// List returns all registered element types.
func (r *ElementRegistry) List() []bpmn.ElementType {
	r.mu.RLock()
	defer r.mu.RUnlock()
	types := make([]bpmn.ElementType, 0, len(r.constructors))
	for t := range r.constructors {
		types = append(types, t)
	}
	return types
}
