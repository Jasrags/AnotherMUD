// Package effect holds the engine-wide registry of effect templates
// addressable by stable id. Both abilities (which compose effect
// templates inline) and consumables (which reference effect_id on
// item.consumed events) target the same EffectManager surface; this
// package owns the id→template map that the consumable path needs.
//
// Spec: economy-survival §6.3 ("effects subscriber"); progression
// §5 (the EffectTemplate type itself stays in internal/progression).
//
// v1 scope (M14.2):
//   - In-memory Registry indexed by case-insensitive id.
//   - YAML loader for <pack>/effects/*.yaml content files (loaded
//     by internal/pack at boot, namespaced like every other content
//     id).
//   - Templates re-use progression.EffectTemplate verbatim — this
//     package adds a lookup, not a parallel type.
//
// The registry is "neutral" per PD-5: it does not import
// internal/economy or internal/ability code, and abilities continue
// to inline EffectTemplate values where appropriate. The registry
// exists for the case where a non-ability source (a potion, a future
// quest grant, a future room aura) needs to apply an effect without
// hardcoding its modifiers at the source.
package effect
