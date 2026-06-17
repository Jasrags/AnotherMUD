package main

// WoT S2 Phase 4+ — saidin taint / madness manifestation.
//
// A male channeler accumulates madness as he weaves (the accrual subscriber in
// main.go); saidar (female) is clean and never accrues. Above a threshold the
// taint manifests on the tick — the Power turning on its wielder — as one of the
// Core 5 conditions (S5), escalating in severity as the madness deepens. This
// is the WoT-specific curse mechanic, so it lives in the composition root beside
// the affinity / overchannel wiring rather than a setting-agnostic package.
//
// The mapping is deliberately coarse and content-aligned: the effect ids are the
// bare condition ids the WoT/core packs ship (`fatigued`, `frightened`,
// `stunned`) — the same ids the overchannel cascade applies — so a missing
// effect surfaces a warning rather than silently no-op'ing.

// madnessManifestation picks the condition a manifestation inflicts and the cue
// shown to the channeler, by madness band. The bands mirror the score sheet's
// qualitative labels (madnessBand): a faint whisper merely tires; a shadow on
// the mind brings terror; the clamor of voices whites the world out.
func madnessManifestation(madness int) (effectID, message string) {
	switch {
	case madness >= 75:
		return "stunned", "The voices crescendo into a single shrieking note — the world whites out and you are GONE, lost inside your own skull."
	case madness >= 50:
		return "frightened", "Something vast and wrong turns its attention on you. Nameless terror floods in; you would run from your own skin if you could."
	default:
		return "fatigued", "The taint crawls behind your eyes. A grey wave of exhaustion rolls through you, and for a moment you are not sure whose thoughts these are."
	}
}
