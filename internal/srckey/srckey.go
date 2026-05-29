// Package srckey holds the modifier-source-key type shared by the stat
// block, the equipment subsystem, and the effect manager. It is a pure
// leaf (no internal imports) so packages that need a source key —
// entities, stats, progression — can all depend on it WITHOUT forming
// an import cycle.
//
// History: SourceKey originally lived in internal/entities, which forced
// internal/stats (and internal/progression) to import entities. That
// blocked entities from ever importing progression (e.g. to give a
// MobInstance a *progression.StatBlock), since entities → progression →
// entities would cycle. Hoisting the key into this leaf removes the only
// real coupling — entities now keeps a type alias for back-compat while
// stats and progression depend on the leaf instead. See
// m8-1-deferred-fixes.
package srckey

// SourceKey is the modifier-source convention (inventory-equipment-items
// §2.3 step 6 / §3.3 step 6): every modifier a subsystem applies carries
// a source that uniquely identifies its origin, so removal reverses
// exactly the right set. It is a string so distinct subsystems segregate
// by prefix ("equipment:", "effect:", …).
type SourceKey string

// Equipment returns the source key the equipment subsystem applies an
// item's modifiers under. id is the item instance's entity id (a bare
// string here so the leaf takes no entities dependency). Centralized so
// the equip and unequip paths cannot drift apart.
func Equipment(id string) SourceKey {
	return SourceKey("equipment:" + id)
}
