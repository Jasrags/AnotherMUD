package notifications

import "time"

// notificationFile is the on-disk YAML shape for a single entity's
// notification queue. The structure mirrors a Queue Snapshot with
// snake_case YAML tags, keeping the pure queue package free of
// serialization concerns. Priority is stored as the canonical
// string ("system" / "tell" / "channel") so the on-disk format is
// stable across future enum value reorderings.
//
// Spec: docs/specs/notifications.md §6.3.
type notificationFile struct {
	Entries []notificationEntry `yaml:"entries,omitempty"`
}

type notificationEntry struct {
	ID          string    `yaml:"id"`
	Recipients  []string  `yaml:"recipients,omitempty"`
	Priority    string    `yaml:"priority"`
	Kind        string    `yaml:"kind"`
	Text        string    `yaml:"text"`
	PublishedAt time.Time `yaml:"published_at"`
	Sender      string    `yaml:"sender,omitempty"`
	// Channel carries the chat channel id for Kind=="channel"
	// entries (M16.4g). Empty for other kinds. Persisted so an
	// offline backlog drained on next login still routes through
	// the GMCP Comm.Channel.Text emitter rather than silently
	// falling back to main-window-text-only.
	Channel string `yaml:"channel,omitempty"`
}

// toFile converts a list of notifications to its on-disk shape.
func toFile(ns []Notification) notificationFile {
	if len(ns) == 0 {
		return notificationFile{}
	}
	out := notificationFile{Entries: make([]notificationEntry, 0, len(ns))}
	for _, n := range ns {
		out.Entries = append(out.Entries, notificationEntry{
			ID:          n.ID,
			Recipients:  append([]string(nil), n.Recipients...),
			Priority:    n.Priority.String(),
			Kind:        n.Kind,
			Text:        n.Text,
			PublishedAt: n.PublishedAt,
			Sender:      n.Sender,
			Channel:     n.Channel,
		})
	}
	return out
}

// parsePriority maps an on-disk priority string back to the enum.
// Unknown strings return (0, false) so the loader can drop the
// entry rather than silently misclassify it.
func parsePriority(s string) (Priority, bool) {
	switch s {
	case "system":
		return PrioritySystem, true
	case "tell":
		return PriorityTell, true
	case "channel":
		return PriorityChannel, true
	default:
		return 0, false
	}
}
