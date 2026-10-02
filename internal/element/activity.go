package element

import "github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"

// Activity represents a BPMN activity (task).
type Activity interface {
	Element

	// TaskType returns the type of activity.
	TaskType() bpmn.TaskType

	// Assignee returns the assigned user.
	Assignee() string

	// CandidateUsers returns the list of candidate users.
	CandidateUsers() []string

	// CandidateGroups returns the list of candidate groups.
	CandidateGroups() []string
}
