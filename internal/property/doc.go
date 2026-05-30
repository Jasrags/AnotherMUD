// Package property holds the engine-wide property-name registry and
// the tagged-value envelope codec used when a property's type is
// unknown to the registry at save time.
//
// Spec: docs/specs/persistence.md §2 (registry), §4.4 / §4.5
// (envelope).
//
// v1 scope (M14.4):
//   - In-memory Registry with engine vs. pack scopes, snake_case
//     validation, shadow-protection, closed value-type enum.
//   - §2.4 name resolution: direct → currentPack:name → declared
//     dependency packs in order.
//   - Tagged-value envelope (Wrap / Unwrap) with §4.5 self-healing
//     nested-tag collapse.
//
// Per PD-6 (locked 2026-05-30): properties are CODE-DECLARED.
// Features call Registry.RegisterEngine at boot; the pack manifest
// does NOT have a `properties:` content path. Content authors who
// want a new pack-scoped property still drive code (a pack's
// engine-side init calls RegisterPack), but ad-hoc YAML-only
// property declarations are out of scope.
//
// Integration with player.Save and other persistence consumers is
// orthogonal — those callers reach into Registry.SerializeProperty
// / DeserializeProperty as they need them. v1 ships the substrate;
// existing save shapes continue to use their hardcoded fields.
package property
