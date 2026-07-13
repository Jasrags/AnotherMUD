// Package visibility answers one question — "can this observer see that
// target right now?" — and is the single seam every renderer, roster, and
// target resolver routes through instead of reading a room's entity list
// directly (docs/specs/visibility.md §1, §2).
//
// This file is the pure filter primitive (spec §2): the composition rule
// over concealment layers, the self-always-visible and bypass invariants,
// and the per-source pierce dispatch. It is deliberately decoupled from the
// engine — it depends only on the small Observer/Target interfaces below,
// which the session/command layer adapts to. Concealment *sources* (hide,
// sneak, darkness, invisibility) supply Layers and observer capabilities;
// with no active layers the filter degrades to today's permissive "yes"
// (§2.2), so wiring it in is behavior-neutral until a source lights up.
//
// Visibility is a perception model, NOT an authorization boundary (§1.2):
// an unknown layer source fails open (visible) rather than concealing.
// Authorization is roles (roles-and-permissions).
package visibility

// SourceType identifies a kind of concealment layer (spec §2.2, §3).
type SourceType string

const (
	// SourceHide is a stationary roll-gated concealment (§3.1).
	SourceHide SourceType = "hide"
	// SourceSneak is a moving roll-gated concealment (§3.2).
	SourceSneak SourceType = "sneak"
	// SourceMagicalInvis is flag-gated invisibility pierced by see_invisible (§3.4).
	SourceMagicalInvis SourceType = "magical-invis"
	// SourceAdminInvis is rank-gated wizinvis pierced by equal/greater rank (§3.4).
	SourceAdminInvis SourceType = "admin-invis"
	// SourceDarkness is the environmental layer applied to non-luminous
	// occupants of a dark room, pierced by light / see_in_dark (§3.3). The
	// caller assembles this layer onto a target only when the room is dark
	// and the target is non-luminous; the primitive just composes it.
	SourceDarkness SourceType = "darkness"
	// SourceQuestSpawn is the quest-scoped ownership gate (quest-spawns.md
	// Phase 2): a mob/item spawned for one player's quest run does not exist
	// for any other observer. Unlike the perception layers this is an
	// EXISTENCE gate, not a can-I-perceive-it question, so it fails CLOSED
	// (§1.2 exception): the caller attaches the layer ONLY to entities the
	// observer does not own — the owner never carries the layer (so they see
	// their own set). The pierce rule pierces for no one by default; a caller
	// may configure a staff bypass by setting the layer's Score to a minimum
	// admin rank (see pierces), and Bypass() still short-circuits at CanSee.
	SourceQuestSpawn SourceType = "quest-spawn"
)

// RollGated reports whether the source resolves through the §4.2 perception
// contest (hide, sneak) rather than a yes/no counter (§1.1, PD-1). The other
// sources are flag-gated.
func (s SourceType) RollGated() bool { return s == SourceHide || s == SourceSneak }

// Layer is one concealment carried by a target (spec §2.2). A target may
// carry several at once; the filter composes them with AND (§1.1).
type Layer struct {
	// Source is the concealment kind, selecting the pierce rule.
	Source SourceType
	// Score is the perception-contest difficulty for a roll-gated layer
	// (§4.2), or the minimum admin rank for SourceAdminInvis (§3.4). It is
	// ignored for the other flag-gated sources.
	Score int
	// Instance identifies this concealment establishment for sticky
	// detection memory (§4.1): it changes each time the source re-establishes
	// (a re-hide is a new instance), so a remembered pierce keys off the
	// right thing. Zero for flag-gated sources, which need no memory.
	Instance uint64
}

// Target is a thing that may be observed (spec §2). It exposes the
// concealment layers currently active on it; an empty slice means fully
// visible (legacy parity).
type Target interface {
	// VisibilityID uniquely identifies the target so the filter can apply
	// the self-always-visible invariant (§2.1).
	VisibilityID() string
	// ConcealmentLayers returns the active concealment layers on this
	// target, including the room darkness layer when applicable (assembled
	// by the caller, §3.3). Nil/empty ⇒ no concealment.
	ConcealmentLayers() []Layer
}

// Observer brings the perception and counters that pierce concealment
// (spec §3, §4). The session/command layer adapts a player/mob to this.
type Observer interface {
	// VisibilityID uniquely identifies the observer (for the self check).
	VisibilityID() string
	// Bypass skips the whole filter — admin verbs reaching concealed
	// targets pass this (§2.1, admin-verbs §3). Bypass is a caller
	// decision; the filter never consults roles itself (§1.2).
	Bypass() bool
	// PiercesDarkness reports that the observer can see in a dark room —
	// it carries a lit light source or has the see_in_dark trait (§3.3).
	PiercesDarkness() bool
	// SeesInvisible holds the see_invisible counter that pierces magical
	// invisibility (§3.4, §4.3).
	SeesInvisible() bool
	// AdminRank is the observer's admin rank; it pierces admin
	// invisibility of equal or lower required rank (§3.4).
	AdminRank() int
	// DetectsHidden auto-pierces roll-gated concealment (hide/sneak)
	// without a contest (§4.3).
	DetectsHidden() bool
	// AlreadyPierced reports sticky detection memory: has this observer
	// already pierced this roll-gated concealment instance (§4.1)? Avoids
	// per-render flicker and re-rolling.
	AlreadyPierced(instance uint64) bool
	// Contest runs the §4.2 perception contest against a roll-gated layer
	// the observer has not yet pierced, recording a success in the
	// observer's detection set. The caller owns the RNG and formula; the
	// primitive only decides WHEN a contest is needed (after DetectsHidden
	// and AlreadyPierced have both declined).
	//
	// Synchronization contract: AlreadyPierced and Contest are the only
	// side-effecting members (Contest writes the detection set). The
	// primitive does NOT lock across the AlreadyPierced→Contest pair, so an
	// ADAPTER MUST make each call individually safe (its own lock). The
	// unlocked gap means two concurrent CanSee calls for the same observer
	// and instance may both contest before either records — a benign
	// double-roll that converges (sticky memory makes the next call cheap);
	// it is never a correctness bug. Per-observer concurrent CanSee is rare
	// in practice (dispatch is largely serial), so the gap is acceptable.
	Contest(layer Layer) bool
}

// CanSee reports whether observer can see target right now (spec §2). An
// observer always sees itself (§2.1); a bypassing caller sees everything
// (§2.1); otherwise the observer must pierce every active layer (§2.2 AND
// composition). With no layers the result is the legacy permissive yes.
func CanSee(o Observer, t Target) bool {
	if o.VisibilityID() == t.VisibilityID() {
		return true // self is always visible (§2.1)
	}
	if o.Bypass() {
		return true // explicit caller bypass (§2.1)
	}
	for _, layer := range t.ConcealmentLayers() {
		if !pierces(o, layer) {
			return false // unpierced layer hides the target (§2.2)
		}
	}
	return true
}

// pierces reports whether the observer defeats a single concealment layer
// (spec §2.2 per-layer rule). An unknown source fails open — visibility is a
// perception model, not security (§1.2).
func pierces(o Observer, layer Layer) bool {
	switch layer.Source {
	case SourceDarkness:
		return o.PiercesDarkness()
	case SourceMagicalInvis:
		return o.SeesInvisible()
	case SourceAdminInvis:
		return o.AdminRank() >= layer.Score
	case SourceQuestSpawn:
		// Existence gate (quest-spawns.md Phase 2): the layer is attached only
		// to entities the observer does not own, so its mere presence means
		// "not yours." Fails CLOSED by design (§1.2 exception) — an owning
		// observer never carries the layer, and Bypass() already short-circuits
		// above. A caller MAY grant a staff bypass (moderation/inspection) by
		// setting layer.Score to the minimum admin rank, exactly like
		// SourceAdminInvis; Score 0 (the default) pierces for NO ONE, so the
		// gate stays closed unless a bypass is explicitly configured.
		return layer.Score > 0 && o.AdminRank() >= layer.Score
	case SourceHide, SourceSneak:
		// Roll-gated (§4): a detect trait or a remembered pierce skips the
		// contest; otherwise run it (and record the result) via the observer.
		if o.DetectsHidden() {
			return true
		}
		if o.AlreadyPierced(layer.Instance) {
			return true
		}
		return o.Contest(layer)
	default:
		return true // unknown layer ⇒ do not conceal (fail open, §1.2)
	}
}

// Visible returns the subset of targets the observer can see (spec §2's
// VisibleEntities, generic so callers keep their concrete element type). The
// observer itself, if present, is always retained (§2.1, via CanSee). The
// input slice is never mutated. The result is nil whenever nothing is visible
// — empty input, all-concealed, or nil input all return nil, so a caller may
// treat nil as "nothing to show". The backing array is allocated lazily on
// the first visible target, so an all-concealed room (common on a render
// tick) allocates nothing.
func Visible[T Target](o Observer, targets []T) []T {
	var out []T
	for _, t := range targets {
		if CanSee(o, t) {
			if out == nil {
				out = make([]T, 0, len(targets))
			}
			out = append(out, t)
		}
	}
	return out
}
