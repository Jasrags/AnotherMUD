// Package chat is the M13.6 chat-channel substrate: a registry of
// named multi-recipient pub-sub topics plus a per-channel ring
// buffer of recent messages.
//
// Spec: docs/specs/chat-channels-and-tells.md §2-§4.
//
// This package owns the *channel* primitives (what a channel is,
// where its scrollback lives). It does NOT own the publish path —
// that lives in internal/command/chat.go and routes through the
// notifications substrate (internal/notifications).
//
// v1 scope (M13.6):
//   - In-memory Registry; channels registered at composition root.
//   - In-memory Scrollback (ring buffer); not yet persisted to
//     saves/channels/<id>.yaml — that's M13.6b.
//   - Subscription state is implicit (every online player is
//     subscribed to every public channel); explicit tune / untune
//     and the chat.subscriptions player-save key are M13.6b.
//
// The data model and Registry shape are intentionally future-
// compatible with the pack-loaded path (a YAML decoder hook lands
// in M13.6b without changing this package's exported surface).
package chat
