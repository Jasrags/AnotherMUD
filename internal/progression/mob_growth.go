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
// Returns true when at least one non-zero modifier landed. A nil
// StatBlock, an empty StatGrowth, or a non-positive level all
// return false and leave sb untouched.
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
