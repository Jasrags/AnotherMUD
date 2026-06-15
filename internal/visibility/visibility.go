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
	// primitive only decides when a contest is needed.
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
// observer itself, if present, is always retained (§2.1, via CanSee). A nil
// input yields a nil result; the input slice is never mutated.
func Visible[T Target](o Observer, targets []T) []T {
	if len(targets) == 0 {
		return nil
	}
	out := make([]T, 0, len(targets))
	for _, t := range targets {
		if CanSee(o, t) {
			out = append(out, t)
		}
	}
	return out
}
