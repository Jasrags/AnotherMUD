package combat

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/pool"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// Combat publishes through an EventSink rather than directly through
// eventbus.Bus, because eventbus imports entities and entities imports
// combat (MobInstance carries Vitals / Stats fields). A combat →
// eventbus edge would close that cycle. EventSink is the indirection
// that keeps combat free of the eventbus dependency; cmd/anothermud
// wires a real sink that publishes to the bus when a subscriber
// actually needs the events (M7.5 for mob.killed → spawn untrack,
// M7.6 for combat.ended → flee-cooldown clear).
//
// In M7.2 there is no engine subscriber yet — the production sink can
// log-only, and tests use a recording sink to assert the right
// payloads emit.

// EventSink consumes combat events. Implementations live outside the
// combat package. All methods MUST tolerate concurrent calls from the
// Manager's mutation path. Implementations MUST NOT call back into
// the Manager from a handler — combat publishes after releasing its
// mutex, so re-entrant Engage / Disengage would not deadlock, but the
// resulting causal chain (engage → handler → engage → handler) is
// undefined and easy to make recursive.
type EventSink interface {
	OnEngagement(ctx context.Context, e Engagement)
	OnCombatEnded(ctx context.Context, e CombatEnded)
	// M7.4 auto-attack additions. Sinks that don't care about per-
	// swing detail (e.g. the M7.2 log-only sink before M7.4) can
	// embed nopSink to satisfy these.
	OnHit(ctx context.Context, e Hit)
	OnMiss(ctx context.Context, e Miss)
	OnEvade(ctx context.Context, e Evade)
	OnVitalDepleted(ctx context.Context, e VitalDepleted)
	// OnSaveResolved reports a resolved saving throw (saves §3). Like
	// Evade, this is reserved ahead of broad use: the massive-damage
	// consumer (saves §4) emits it today; S2/S5 weaves and conditions
	// will emit it as they land. Sinks that don't care embed nopSink.
	OnSaveResolved(ctx context.Context, e SaveResolved)
	// OnRangedDry reports a projectile swing skipped for want of ammo
	// (ranged-combat §3). Sinks that don't care embed nopSink.
	OnRangedDry(ctx context.Context, e RangedDry)
	// OnBandChange reports a range-band move (ranged-combat §5.2/§5.4): an
	// auto-close, advance, or withdraw. Sinks that don't care embed nopSink.
	OnBandChange(ctx context.Context, e BandChange)
}

// Engagement is dispatched after both sides of an Engage have been
// inserted into each other's combat lists (spec combat §2.1 step 3).
// Symmetric — one Engagement event per Engage call, not one per side.
//
// AttackerID / TargetID carry the pre-engagement roles. RoomID is the
// shared room at engagement time; combat refuses cross-room engages
// in M7.6 once tag checks are in, so for now this is always the
// caller-supplied room.
type Engagement struct {
	AttackerID   CombatantID
	TargetID     CombatantID
	AttackerName string
	TargetName   string
	RoomID       world.RoomID
}

// CombatEnded is dispatched when an entity's combat list becomes
// empty (spec §2.2 and §2.3). One event per side that empties —
// a pairwise Disengage between two combatants whose lists become
// empty dispatches two CombatEnded events; a DisengageAll
// dispatches one per opponent that emptied PLUS one for the entity
// itself.
//
// CombatantID is the entity that left combat. CombatantName is
// included for log convenience; subscribers that need richer state
// resolve through a Locator.
type CombatEnded struct {
	CombatantID   CombatantID
	CombatantName string
	RoomID        world.RoomID
}

// Hit is dispatched when an auto-attack swing connects (combat §4.3
// step 4). Damage is the amount actually subtracted from Vitals, after
// the §4.5 "at least 1 on a hit" clamp. DamageType is the per-damage-
// type label used by AC tables; M7.4 ships a single type
// (DamageTypePhysical) until M8 widens it.
type Hit struct {
	AttackerID   CombatantID
	TargetID     CombatantID
	AttackerName string
	TargetName   string
	WeaponName   string
	Damage       int
	DamageType   string
	IsCritical   bool
	// Soak is the amount of damage the defender's armour/soak absorbed on this
	// swing (pre-soak damage minus the applied Damage). 0 when nothing was
	// soaked. Renderers use it to explain a hit that armour floored to 1 — e.g. a
	// crit whose damage was fully absorbed reads "glances off the armour" rather
	// than a misleading "critical hit!" beside a 1-damage number.
	Soak int
	// Subdual is true when the wielded weapon is nonlethal (subdual-damage §2).
	// Carried for renderers (a knock-out blow reads differently from a wound);
	// the mechanical consequence rides VitalDepleted.Subdual on a finishing blow.
	Subdual bool
	// Ineffective is true when the swing LANDED but dealt no damage because the
	// weapon could not penetrate the defender's armor (subdual-damage §6 — a whip
	// vs. an armored foe). Damage is 0 and no vital is depleted; renderers phrase
	// it as a harmless lash rather than a wound.
	Ineffective bool
	RoomID      world.RoomID
}

// Miss is dispatched when a swing fails to land (combat §4.3 step 5).
// IsFumble is true when the swing was decided by a natural-1 roll
// (§4.4) rather than by AC math.
type Miss struct {
	AttackerID   CombatantID
	TargetID     CombatantID
	AttackerName string
	TargetName   string
	WeaponName   string
	IsFumble     bool
	RoomID       world.RoomID
}

// Evade is dispatched when a defensive passive ability pre-empts a
// swing (combat §4.3 step 2). M7.4 has no passive-abilities surface
// so this event is never emitted today — the type is reserved for
// M9 abilities and lives here so the EventSink contract stabilizes
// before that wiring lands.
type Evade struct {
	AttackerID   CombatantID
	TargetID     CombatantID
	AttackerName string
	TargetName   string
	AbilityName  string // empty until M9 abilities exist
	RoomID       world.RoomID
}

// VitalDepleted is dispatched when a damage application drops a
// combatant's named vital to zero (combat §4.3 step 4 "vital-
// depleted", §6 "When an entity's HP reaches zero"). Today only
// Vital="hp" is emitted; future pools (Shadowrun's Stun/Physical
// monitors, an overflow death track) reuse the type by passing their
// own pool.Kind. AttackerID is the attribution surface M7.5 consumes
// to credit a kill.
//
// Vital carries a pool.Kind's string value (see VitalHP). It is a plain
// string, not a pool.Kind, deliberately — the same cross-package
// decoupling that keeps the save axis a string (combat must not force a
// shared typed vocabulary on every event consumer).
type VitalDepleted struct {
	VictimID   CombatantID
	VictimName string
	AttackerID CombatantID
	Vital      string
	// Subdual is true when the FINISHING blow came from a nonlethal weapon
	// (subdual-damage §4). The death pipeline reads it to KNOCK OUT (cancel the
	// death, restore the victim, apply the unconscious condition) instead of
	// killing. False (the default) on every lethal finish, an ability/DoT death,
	// or a non-combat depletion — all of which kill exactly as before.
	Subdual bool
	RoomID  world.RoomID
}

// RangedDry is dispatched when a projectile auto-attack swing cannot fire
// for want of matching ammunition (ranged-combat §3). The swing is skipped
// and the attacker stays engaged — it resumes firing the moment ammo is
// available again. WeaponName is the launcher (e.g. "a short bow") and
// AmmoKind is what it needed (e.g. "arrow"), so a renderer can phrase a
// clear "click — out of arrows" line.
type RangedDry struct {
	AttackerID   CombatantID
	TargetID     CombatantID
	AttackerName string
	TargetName   string
	WeaponName   string
	AmmoKind     string
	// Style is the attacker's wielded-weapon flavor voice (rangedflavor), so the
	// sink can phrase a style-appropriate dry-fire / unloaded line. Empty → the
	// default style / engine floor.
	Style string
	// Unloaded distinguishes a reload-gated weapon that is simply not chambered
	// (a crossbow awaiting `load`) from one out of ammunition (action-economy.md
	// §7.1). The sink renders a "reload it" prompt for the former, an
	// out-of-ammo line for the latter.
	Unloaded bool
	RoomID   world.RoomID
}

// BandChange is dispatched when a pairing's range band moves (ranged-combat
// §5.2/§5.4): a melee combatant auto-closing toward melee, or a manual
// advance/withdraw. Subject is the combatant who moved, Opponent the other side
// of the pairing. Closing is true when the band moved toward melee (advance /
// auto-close), false when it opened toward far (withdraw). NewBand/NewBandName
// are the resulting band so a renderer can phrase "X closes to near" / "X opens
// to far".
type BandChange struct {
	SubjectID    CombatantID
	SubjectName  string
	OpponentID   CombatantID
	OpponentName string
	NewBand      int
	NewBandName  string
	Closing      bool
	RoomID       world.RoomID
}

// DamageTypePhysical is the single damage-type label M7.4 emits on
// every hit event. M8 widens the type space (slashing, piercing,
// elemental, etc.) when AC tables exist to discriminate them.
const DamageTypePhysical = "physical"

// VitalHP is the hit-point vital name in VitalDepleted events. Derived
// from pool.KindHP so the event string and the pool kind are provably the
// same value; subscribers compare against this rather than re-spelling
// the literal.
const VitalHP = string(pool.KindHP)

// nopSink is the EventSink used when Manager is constructed with a
// nil sink. Centralized so the mutation path always has a non-nil
// dispatch target and doesn't have to nil-guard at every emission
// site.
type nopSink struct{}

func (nopSink) OnEngagement(context.Context, Engagement)       {}
func (nopSink) OnCombatEnded(context.Context, CombatEnded)     {}
func (nopSink) OnHit(context.Context, Hit)                     {}
func (nopSink) OnMiss(context.Context, Miss)                   {}
func (nopSink) OnEvade(context.Context, Evade)                 {}
func (nopSink) OnVitalDepleted(context.Context, VitalDepleted) {}
func (nopSink) OnSaveResolved(context.Context, SaveResolved)   {}
func (nopSink) OnRangedDry(context.Context, RangedDry)         {}
func (nopSink) OnBandChange(context.Context, BandChange)       {}
