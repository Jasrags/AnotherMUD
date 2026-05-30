package progression

import (
	"github.com/Jasrags/AnotherMUD/internal/srckey"
	"github.com/Jasrags/AnotherMUD/internal/stats"
)

// ApplyMobClassGrowth applies class-bound stat growth to a mob's
// StatBlock at spawn (mobs-ai-spawning §3.2). For each entry in
// cls.StatGrowth, computes `growth.Average() × level` and writes
// the deltas under srckey.ClassGrowth(cls.ID) so removal /
// reapplication is a single source-keyed operation.
//
// Returns true when at least one non-zero modifier landed. The
// guard returns false in three legitimate cases:
//
//   - level <= 0 — the spawn declared no level; no growth applies.
//   - len(cls.StatGrowth) == 0 — the class has no growth table.
//   - every dice expression averages to zero — degenerate but valid.
//
// And in two "defensive — should never happen from current callers"
// cases:
//
//   - sb == nil  — caller passed a nil StatBlock; the bootSpawner
//                  never does because Store.SpawnMob always builds
//                  one. Treated as no-op rather than panic so a
//                  future caller can't accidentally crash the boot.
//   - cls == nil — caller passed a nil Class; the bootSpawner only
//                  calls this after a `(*Class, true)` registry hit.
//                  No-op for the same reason.
//
// Spec posture: integer averaging via DiceExpr.Average — 1d6 → 3,
// 2d6 → 7. The level multiplier is applied AFTER averaging so the
// result is deterministic per (class, level) pair.
//
// Used by the M14.3 mob spawn path. Player-side stat growth is a
// different path (rolled dice on level-up via ApplyStatGrowth, not
// averaged) and stays in level_up.go.
func ApplyMobClassGrowth(sb *StatBlock, cls *Class, level int) bool {
	if sb == nil || cls == nil || level <= 0 || len(cls.StatGrowth) == 0 {
		return false
	}
	var mods []stats.Modifier
	for stat, growth := range cls.StatGrowth {
		delta := growth.Average() * level
		if delta == 0 {
			continue
		}
		mods = append(mods, stats.Modifier{Stat: string(stat), Value: delta})
	}
	if len(mods) == 0 {
		return false
	}
	sb.AddModifiers(srckey.ClassGrowth(cls.ID), mods)
	return true
}
