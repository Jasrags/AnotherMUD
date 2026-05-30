// Package notifications implements the per-entity priority queue
// substrate that delivers asynchronous addressed messages (tells,
// channel posts, system notices) to players. Online recipients are
// delivered immediately; offline recipients have notifications
// enqueued for delivery on next reconnect.
//
// Spec: docs/specs/notifications.md.
//
// This package owns three layers, landing across M13.1a/b/c:
//
//   - M13.1a (this commit): the Queue type — a single-entity, in-
//     memory, priority-ordered, bounded buffer. Not safe for
//     concurrent use; goroutine safety is the Manager's concern.
//   - M13.1b: per-entity persistence (notifications.yaml) using
//     internal/persistence atomic-write rotation.
//   - M13.1c: Manager keyed by entity id, session-sink wiring for
//     online/offline routing, drain on session active phase.
package notifications
