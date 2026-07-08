package combat

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/world"
)

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

// nearBand is one step out from melee (index 1) — the band a `reach` weapon can
// strike at (special-weapons §3) and the last band a closing melee foe crosses.
const nearBand = 1

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
// at (§5.2): a PROJECTILE-wielding initiator (a bow) opens at the far band; a
// melee or thrown weapon opens at melee. Only a projectile actually shoots from
// range in the round loop (§5.3) — a wielded thrown weapon auto-attacks at
// melee, so opening its fight at far would only waste rounds closing for the
// first swing. The deliberate ranged `throw` verb is one-shot and unaffected by
// bands, so opening at melee costs it nothing. Reads the attacker's wielded
// class through the locator, so it MUST be called outside m.mu (the locator
// takes session/entities locks; m.mu stays inner — see engageWithReason).
func (m *Manager) openingBand(attacker CombatantID) int {
	c, ok := m.locator.LookupCombatant(attacker)
	if !ok {
		return meleeBand
	}
	if c.Stats().RangedClass == RangedProjectile {
		return farBand()
	}
	return meleeBand
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

// MoveBand is the verb-facing band move (ranged-combat §5.4): it steps the
// subject↔opponent pairing one band toward melee (closing — advance) or toward
// far (opening — withdraw), emits the BandChange so both sides see it, and
// returns the new band. moved is false when the two aren't engaged or the band
// is already at the requested extreme (already in melee for an advance, already
// at far for a withdraw) — the caller maps that to a precise message. Distinct
// from flee: this stays in the room (§5.4).
func (m *Manager) MoveBand(ctx context.Context, subject, opponent CombatantID, roomID world.RoomID, closing bool) (int, bool) {
	delta := 1
	if closing {
		delta = -1
	}
	m.mu.Lock()
	if !contains(m.lists[subject], opponent) {
		m.mu.Unlock()
		return meleeBand, false
	}
	key := makeBandKey(subject, opponent)
	cur := m.bands[key]
	next := max(cur+delta, meleeBand)
	if next > farBand() {
		next = farBand()
	}
	if next == cur {
		m.mu.Unlock()
		return cur, false // already at the requested extreme
	}
	if m.bands == nil {
		m.bands = make(map[bandKey]int)
	}
	m.bands[key] = next
	m.mu.Unlock()

	// Closing the distance is a charge: the opponent may answer with a braced
	// set-weapon blow (special-weapons §4). recordCharge takes m.mu, so call it
	// after the unlock above.
	if closing {
		m.recordCharge(subject, opponent)
	}

	// Names + emit outside m.mu (lock-order — see engageWithReason).
	m.sink.OnBandChange(ctx, BandChange{
		SubjectID:    subject,
		SubjectName:  m.lookupName(subject),
		OpponentID:   opponent,
		OpponentName: m.lookupName(opponent),
		NewBand:      next,
		NewBandName:  BandName(next),
		Closing:      closing,
		RoomID:       roomID,
	})
	return next, true
}

// chargeKey is the DIRECTIONAL key for a pending charge — `charger` closed a band
// toward `victim`. Unlike the order-independent bandKey, A→B and B→A are distinct
// entries, so two combatants charging each other in the same round each keep
// their own pending charge instead of one overwriting the other.
type chargeKey struct{ charger, victim CombatantID }

// recordCharge marks that `charger` closed a band toward `victim` (a charge):
// the victim, if it wields a `set` weapon, lands a braced bonus blow on its next
// swing (special-weapons §4). Caller MUST NOT hold m.mu (takes it).
func (m *Manager) recordCharge(charger, victim CombatantID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.charged == nil {
		m.charged = make(map[chargeKey]bool)
	}
	m.charged[chargeKey{charger: charger, victim: victim}] = true
}

// ConsumeCharge reports whether `charger` has a pending charge toward `victim`
// and, if so, clears it (the braced moment is spent on this swing). A set-weapon
// wielder (victim) calls this on its swing to decide whether the set-vs-charge
// bonus applies. Safe for concurrent use.
func (m *Manager) ConsumeCharge(charger, victim CombatantID) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := chargeKey{charger: charger, victim: victim}
	if !m.charged[key] {
		return false
	}
	delete(m.charged, key)
	return true
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
	band := max(m.bands[key]+delta, meleeBand)
	if band > farBand() {
		band = farBand()
	}
	if m.bands == nil {
		m.bands = make(map[bandKey]int)
	}
	m.bands[key] = band
	return band
}
