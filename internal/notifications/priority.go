package notifications

// Priority is the drain-ordering enumeration. Higher numeric values
// drain before lower numeric values. Within a tier, drain order is
// FIFO by PublishedAt.
//
// Spec: docs/specs/notifications.md §3.
type Priority int

const (
	// PriorityChannel is the lowest tier — multi-recipient chat.
	PriorityChannel Priority = iota
	// PriorityTell is the middle tier — one-to-one private chat.
	PriorityTell
	// PrioritySystem is the highest tier — administrative
	// broadcasts, maintenance notices, error replies. Drains first.
	PrioritySystem
)

// String returns the canonical kind-style name for the priority.
// Used in structured-log fields and snapshot serialization.
func (p Priority) String() string {
	switch p {
	case PrioritySystem:
		return "system"
	case PriorityTell:
		return "tell"
	case PriorityChannel:
		return "channel"
	default:
		return "unknown"
	}
}
