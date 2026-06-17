package combat

// Range bands (ranged-combat §5.1). A fight between two combatants carries a
// range state: an ordered vocabulary indexed from the MELEE band (0 — the
// colocated state all pre-ranged combat assumes) outward to the farthest band.
// Advancing decrements toward melee; withdrawing increments toward far. The
// vocabulary is an engine baseline for v1 (pack-declarable is a later
// extension, mirroring weapon tiers); the spec keeps the names content-defined.
//
// Indexing from melee=0 means a fight with no band entry reads 0 (melee), so
// every existing melee fight is unchanged — the band machinery is invisible
// until a ranged engagement opens at a farther band.
var rangeBands = []string{"melee", "near", "far"}

// meleeBand is the closest band (index 0) — the default colocated state.
const meleeBand = 0

// farBand is the farthest band a ranged engagement opens at (§5.2).
func farBand() int { return len(rangeBands) - 1 }

// BandName returns the display name for a band index, clamped to the vocabulary
// so an out-of-range index degrades to the nearest valid name rather than
// panicking.
func BandName(i int) string {
	if i < 0 {
		i = 0
	}
	if i >= len(rangeBands) {
		i = len(rangeBands) - 1
	}
	return rangeBands[i]
}

// bandKey is the order-independent key for a pairing's range band: distance is
// mutual, so (A,B) and (B,A) map to the same entry (ranged-combat §5 — the band
// is per attacker↔target pair).
type bandKey struct{ lo, hi CombatantID }

func makeBandKey(a, b CombatantID) bandKey {
	if a > b {
		a, b = b, a
	}
	return bandKey{lo: a, hi: b}
}

// openingBand returns the band a fresh engagement INITIATED by attacker opens
// at (§5.2): a ranged-wielding initiator (bow or thrown weapon in hand) opens
// at the far band; a melee initiator opens at melee. Reads the attacker's
// wielded-weapon class through the locator, so it MUST be called outside m.mu
// (the locator takes session/entities locks; m.mu stays inner — see
// engageWithReason's name-resolution rationale).
func (m *Manager) openingBand(attacker CombatantID) int {
	c, ok := m.locator.LookupCombatant(attacker)
	if !ok {
		return meleeBand
	}
	switch c.Stats().RangedClass {
	case RangedProjectile, RangedThrown:
		return farBand()
	default:
		return meleeBand
	}
}

// BandOf returns the current range band index for the a↔b pairing, or meleeBand
// (0) when untracked — so a melee fight (or any pairing with no ranged opener)
// reads the colocated default. Safe for concurrent use.
func (m *Manager) BandOf(a, b CombatantID) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.bands[makeBandKey(a, b)]
}

// setBandLocked records the band for a pairing. Caller MUST hold m.mu.
func (m *Manager) setBandLocked(a, b CombatantID, band int) {
	if m.bands == nil {
		m.bands = make(map[bandKey]int)
	}
	m.bands[makeBandKey(a, b)] = band
}

// AdjustBand moves the a↔b pairing one band toward melee (delta < 0, advance)
// or toward far (delta > 0, withdraw), clamped to [melee, far]. Returns the new
// band index. A no-op delta or an out-of-range move that clamps still returns
// the (possibly unchanged) band. Used by the round-loop auto-close and the
// advance/withdraw verbs (§5.2, §5.4).
func (m *Manager) AdjustBand(a, b CombatantID, delta int) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := makeBandKey(a, b)
	band := m.bands[key] + delta
	if band < meleeBand {
		band = meleeBand
	}
	if band > farBand() {
		band = farBand()
	}
	if m.bands == nil {
		m.bands = make(map[bandKey]int)
	}
	m.bands[key] = band
	return band
}
