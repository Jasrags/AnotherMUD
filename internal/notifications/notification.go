package notifications

import "time"

// Notification is the immutable addressed-message envelope the queue
// holds. Once published it is never rewritten — text, priority,
// recipients, and timestamps are fixed at publish time.
//
// Spec: docs/specs/notifications.md §2.
type Notification struct {
	// ID uniquely identifies this notification within the process
	// lifetime. The Manager (M13.1c) assigns it at publish if the
	// caller did not supply one.
	ID string

	// Recipients lists every entity id the notification is addressed
	// to. The Manager fans out to each independently; partial
	// success is acceptable per spec §11.
	Recipients []string

	// Priority controls drain order. See Priority.
	Priority Priority

	// Kind is a short stable string identifying the category
	// (`tell`, `channel`, `system`, …). Used by GMCP routing
	// (Theme B) and by structured logging.
	Kind string

	// Text is the rendered line. The substrate does not own
	// rendering — the publisher delivers the final string.
	Text string

	// PublishedAt is stamped from the engine Clock at publish
	// time (never wall time). See docs/specs/time-and-clock.md.
	PublishedAt time.Time

	// Sender optionally identifies the publishing entity (e.g.,
	// the speaking player for tells). May be empty for system
	// notifications.
	Sender string

	// Channel optionally identifies the chat channel id for
	// Kind=="channel" notifications (e.g. `ooc`, `tapestry-core:
	// trade`). Empty for other kinds. The M16.4g Comm.Channel GMCP
	// flusher reads it to build a structured channel-text payload
	// in parallel with the plain-text Deliver path.
	Channel string
}
