package feat

// LevelsPerFeat is the character-level interval that grants a feat credit
// (feats §Acquiring — docs/proposals/wot-feats.md §2.2): a credit at every 3rd
// level (3, 6, 9, …), on top of the single credit granted at character
// creation. A magnitude per spec convention; the engine wiring reads it.
const LevelsPerFeat = 3

// CreditsForLevelChange returns how many feat credits a character earns moving
// from oldLevel to newLevel (EPIC S4 Phase 2). It counts the LevelsPerFeat
// multiples crossed, so it is correct for a single +1 level-up step and robust
// to a multi-level jump. A non-advancing or backward change earns nothing; a
// negative oldLevel is floored at 0.
func CreditsForLevelChange(oldLevel, newLevel int) int {
	if newLevel <= oldLevel {
		return 0
	}
	if oldLevel < 0 {
		oldLevel = 0
	}
	return newLevel/LevelsPerFeat - oldLevel/LevelsPerFeat
}
