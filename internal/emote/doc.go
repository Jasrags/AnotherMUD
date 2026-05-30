// Package emote is the M13.7 player-emitted social-action substrate:
// a registry of named emotes (smile, nod, wave, …) and a freeform
// pose verb. Output is room-scoped — emotes never publish through
// the notifications queue.
//
// Spec: docs/specs/emotes.md.
//
// v1 scope (M13.7):
//   - In-memory Registry; baseline emotes registered programmatically
//     at the composition root.
//   - Pronoun substitution with a single default pronoun set
//     (they/them) for every entity — per the locked v1 decision,
//     character-creation pronouns are deferred.
//   - Target resolution lives in internal/command/emote.go and
//     leans on the existing player Locator + mob keyword resolver
//     (consistent with the consider / kill verbs).
//
// M13.7b adds pack-loaded emote YAML, per-character pronouns, and
// item/mob pronoun overrides.
package emote
