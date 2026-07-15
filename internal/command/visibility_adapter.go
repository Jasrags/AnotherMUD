package command

import (
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/light"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/questspawn"
	"github.com/Jasrags/AnotherMUD/internal/visibility"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// This file bridges the engine's concrete types to the decoupled visibility
// filter (internal/visibility) and builds the per-room visibility predicate
// (visibility §2, §4, §5.4). The SAME predicate gates both command target
// resolution (ResolveContext.CanSee) and the room render occupant list, so
// "what you can target" and "what you can see" stay consistent — and the
// observer's sticky detection set converges across both.
//
// Sources wired so far: darkness (S2, via internal/light) and hide (S3).
// Sneak / invisibility (later slices) extend the layer assembly + observer
// capabilities below; the predicate seam does not move.

// hideable is the target-read capability: an occupant that may be hide-
// concealed (visibility §3.1). connActor implements it; non-implementers are
// treated as never hidden. Optional interface (the LightViewer pattern) so
// the broad Actor interface need not grow.
type hideable interface {
	IsHidden() bool
	ConcealmentScore() int
	HiddenInstance() uint64
}

// perceiver is the observer-side capability: perception + sticky detection
// memory for the §4.2 contest (visibility §4.1). connActor implements it; a
// nil perceiver cannot pierce roll-gated concealment (it never wins a
// contest), which is the correct degraded behavior for a viewer with no
// perception wired (tests). ContestOutcome reports the remembered result
// (won, done) so a contest — win OR loss — is rolled at most once per
// room-entry; RecordContest stores it.
type perceiver interface {
	PerceptionBonus() int
	ContestOutcome(instance uint64) (won, done bool)
	RecordContest(instance uint64, won bool)
}

// visObserver adapts a viewer to visibility.Observer. PiercesDarkness comes
// from the light system (plus ultrasound); the roll-gated path (hide/sneak)
// delegates to the optional perceiver + a d20 roller, unless DetectsHidden
// auto-pierces it (the detect-hidden trait or ultrasound). SeesInvisible/
// AdminRank are set by the invisibility/admin terms in visibilityPredicate.
type visObserver struct {
	id            string
	piercesDark   bool
	seesInvis     bool               // the see_invisible counter (§4.3) → pierces magical invis
	detectsHidden bool               // the detect-hidden counter (§4.3) → auto-pierces hide/sneak
	adminRank     int                // 0 = ordinary; ≥1 = staff (pierces admin-invis of ≤ rank)
	per           perceiver          // nil ⇒ cannot pierce roll-gated concealment
	roller        progression.Roller // nil ⇒ ditto (no contest possible)
}

func (o visObserver) VisibilityID() string  { return o.id }
func (o visObserver) Bypass() bool          { return false }
func (o visObserver) PiercesDarkness() bool { return o.piercesDark }
func (o visObserver) SeesInvisible() bool   { return o.seesInvis }
func (o visObserver) AdminRank() int        { return o.adminRank }
func (o visObserver) DetectsHidden() bool   { return o.detectsHidden }

// InvisibleFlag is the effect flag that makes an entity magically invisible
// (visibility §3.4): any active effect carrying it conceals the bearer behind
// the see-invisible counter. SeeInvisibleFlag is the counter that pierces it —
// honored both as a racial/ability tag (HasTag, like darkvision) and as an
// effect flag (HasFlag). Content-contract strings, exported so the effect
// lifecycle bridge (entity.revealed on expiry) can reference the same names.
const (
	InvisibleFlag    = "invisible"
	SeeInvisibleFlag = "see_invisible"
)

// UltrasoundFlag is the capability key for ultrasound (active echolocation) —
// a sense that reveals the PHYSICAL, so it defeats visual concealment
// (visibility §3, §4.3): it pierces darkness (echolocation is not light) and
// auto-detects hidden/sneaking occupants (a body echoes whether or not it is
// seen). Sourced like the light vision modes: a racial tag (HasTag) OR an
// equipped grant (a cybereye `grants: [ultrasound]`, item-modification §6). It
// does NOT pierce magical invisibility (that has its own see-invisible counter)
// nor the admin/quest-spawn gates. Defeat-by-silence is a future counter.
const UltrasoundFlag = "ultrasound"

// AlreadyPierced reports a remembered WINNING contest, so the filter
// short-circuits to visible without re-rolling (§4.1). A remembered LOSS is
// not "pierced" — it falls through to Contest, which returns the sticky loss.
func (o visObserver) AlreadyPierced(instance uint64) bool {
	if o.per == nil {
		return false
	}
	won, done := o.per.ContestOutcome(instance)
	return done && won
}

// Contest resolves a roll-gated layer with one-roll-per-room-entry sticky
// memory (§4.1): a remembered outcome (win OR loss) is returned without
// re-rolling; otherwise it runs the §4.2 contest (d20 + perception vs the
// concealment score, via the skill-check primitive), records the result, and
// returns it. Without a perceiver or roller the observer cannot pierce.
func (o visObserver) Contest(layer visibility.Layer) bool {
	if o.per == nil || o.roller == nil {
		return false
	}
	if won, done := o.per.ContestOutcome(layer.Instance); done {
		return won // sticky — never re-roll the same instance this room
	}
	won := progression.ResolveSkillCheck(o.roller, o.per.PerceptionBonus(), layer.Score).Success
	o.per.RecordContest(layer.Instance, won)
	return won
}

// visTarget adapts a room occupant to visibility.Target, carrying the
// concealment layers the caller assembled for it.
type visTarget struct {
	id     string
	layers []visibility.Layer
}

func (t visTarget) VisibilityID() string                  { return t.id }
func (t visTarget) ConcealmentLayers() []visibility.Layer { return t.layers }

// visibilityPredicate builds the per-room CanSee closure for the actor
// (visibility §4, §5.4), or nil when nothing in the room is concealed from
// this viewer — the legacy permissive path, so a plain lit room with no
// hidden occupants allocates no closure and consumers skip filtering.
//
// Two sources compose (AND, via the filter): darkness (the viewer's
// effective light is Black → non-luminous occupants concealed; §3.3) and
// hide (a room occupant carrying the `hidden` tag → concealed behind a
// perception contest; §3.1, §4.2). A luminous item is visible in the dark to
// anyone; the viewer's own perception + sticky memory pierce hides.
func (c *Context) visibilityPredicate() func(string) bool {
	if c.Actor == nil {
		return nil
	}
	room := c.Actor.Room()
	if room == nil {
		return nil
	}

	// Darkness term: the viewer's per-viewer effective light (darkvision +
	// carried light already folded in). Black ⇒ darkness conceals.
	lvl := light.Lit
	if c.Light != nil {
		lvl = EffectiveLight(c.Light, room, c.Actor, c.Items, c.Placement)
	}
	dark := lvl <= light.Black

	// Hide term: the hidden occupants of the room and their concealment
	// layers, keyed by entity id. Built once so the closure is a cheap map
	// lookup per candidate.
	hidden := hiddenOccupants(c, room.ID)

	// Admin-invis term (§3.4): the wizinvis occupants of the room, keyed by
	// player id. Flag-gated — an observer pierces iff its admin rank ≥ the
	// layer's (here, any admin sees any wizinvis admin; non-admins see none).
	adminInvis := adminInvisibleOccupants(c, room.ID)

	// Magical-invis term (§3.4): the occupants carrying an active `invisible`
	// effect flag, keyed by player id. Flag-gated by the see-invisible counter
	// (§4.3) — NOT by admin rank or a perception contest. Resolve the viewer's
	// counter first and skip building the map entirely when they already pierce
	// it: a see-invisible viewer would pierce every magical-invis layer anyway,
	// so the per-player HasFlag scan would be wasted work.
	seesInvis := c.viewerSeesInvisible()
	var magicalInvis map[string]struct{}
	if !seesInvis {
		magicalInvis = magicalInvisibleOccupants(c, room.ID)
	}

	// Quest-spawn term (quest-spawns.md Phase 2): the room's quest-spawned
	// items/mobs owned by SOMEONE ELSE. Owner-gated existence — a foreign
	// spawn does not exist for this viewer, so it is concealed unconditionally
	// (the SourceQuestSpawn layer never pierces). The viewer's OWN spawns are
	// absent from this set and so carry no layer (self sees own set).
	foreignSpawns := foreignQuestSpawns(c, room.ID)

	if !dark && len(hidden) == 0 && len(adminInvis) == 0 && len(magicalInvis) == 0 && len(foreignSpawns) == 0 {
		return nil // nothing concealed from this viewer
	}

	// Ultrasound (§3, §4.3): an echolocation sense that reveals the physical, so
	// it pierces darkness (not light-dependent) AND auto-detects hidden/sneaking
	// occupants. Sourced racially or from a cybereye grant.
	hasUltrasound := c.viewerHasUltrasound()

	obs := visObserver{
		id: c.Actor.PlayerID(),
		// !dark = the viewer is in an adequately lit room (above Black, with
		// their own light/darkvision already folded into EffectiveLight), so
		// darkness is not a barrier for them. Ultrasound also pierces darkness
		// (echolocation is not sight). When dark and neither holds, a
		// SourceDarkness layer is appended below and this false fails to pierce.
		piercesDark: !dark || hasUltrasound,
		seesInvis:   seesInvis,
		// Detect-hidden auto-pierces roll-gated hide/sneak without a contest
		// (§4.3): the detect_hidden trait/effect (previously wired only for exit
		// discovery) OR ultrasound.
		detectsHidden: hasUltrasound || c.actorDetectsHidden(),
		roller:        c.SkillRoller,
	}
	// Admin rank (flat roles → binary, §3.4): a staff observer pierces the
	// admin-invis layer (Score 1); an ordinary viewer (rank 0) does not.
	isAdmin := actorIsAdmin(c.Actor, c.AdminRole)
	if isAdmin {
		obs.adminRank = adminInvisRank
	}
	// Quest-spawn staff bypass (quest-spawns.md §10): a staffer pierces the
	// gate only when they have NOT silenced the clutter (`showspawns on`, the
	// default). Otherwise the attached layer carries Score 0 → never pierced,
	// so they see only their own spawns like an ordinary player.
	questSpawnBypassScore := 0
	if isAdmin && viewerShowsOtherQuestSpawns(c.Actor) {
		questSpawnBypassScore = questSpawnBypassRank
	}
	if p, ok := c.Actor.(perceiver); ok {
		obs.per = p
	}
	items := c.Items
	return func(id string) bool {
		var layers []visibility.Layer
		if dark && !luminousItemID(items, id) {
			layers = append(layers, visibility.Layer{Source: visibility.SourceDarkness})
		}
		if hl, ok := hidden[id]; ok {
			layers = append(layers, hl)
		}
		if _, ok := adminInvis[id]; ok {
			layers = append(layers, visibility.Layer{Source: visibility.SourceAdminInvis, Score: adminInvisRank})
		}
		if _, ok := magicalInvis[id]; ok {
			layers = append(layers, visibility.Layer{Source: visibility.SourceMagicalInvis})
		}
		if _, ok := foreignSpawns[id]; ok {
			// Score = the staff rank that bypasses the gate: a staff observer
			// (adminRank set below) pierces a foreign spawn for moderation /
			// inspection, mirroring admin-invis; an ordinary viewer (rank 0)
			// does not (quest-spawns.md §10 admin bypass). A staffer who
			// silenced the clutter (`showspawns off`) gets Score 0 here — the
			// bypass is withheld and they see only their own, like a player.
			layers = append(layers, visibility.Layer{Source: visibility.SourceQuestSpawn, Score: questSpawnBypassScore})
		}
		return visibility.CanSee(obs, visTarget{id: id, layers: layers})
	}
}

// foreignQuestSpawns returns the room's quest-spawned entities owned by a
// player OTHER than the observer, keyed by entity id (quest-spawns.md Phase 2).
// Built by reading the owner marker (questspawn.OwnerProperty) off each placed
// entity; the observer's own spawns and unmarked (ordinary) entities are
// excluded. Empty — the common case — when the room holds no quest spawns, so
// the predicate short-circuits to its legacy permissive path.
func foreignQuestSpawns(c *Context, roomID world.RoomID) map[string]struct{} {
	if c.Placement == nil || c.Items == nil {
		return nil
	}
	self := c.Actor.PlayerID()
	var out map[string]struct{}
	for _, id := range c.Placement.InRoom(roomID) {
		e, ok := c.Items.GetByID(id)
		if !ok {
			continue
		}
		owner := questSpawnOwner(e)
		if owner == "" || owner == self {
			continue // ordinary entity, or the viewer's own spawn (self-visible)
		}
		if out == nil {
			out = make(map[string]struct{})
		}
		out[string(id)] = struct{}{}
	}
	return out
}

// questSpawnOwner returns the owning player's id stamped on a quest-spawned
// entity (questspawn.OwnerProperty), or "" for an entity that is not a quest
// spawn. Both ItemInstance and MobInstance expose Property; anything else (or a
// non-string value) reads as unowned.
func questSpawnOwner(e entities.Entity) string {
	p, ok := e.(interface {
		Property(string) (any, bool)
	})
	if !ok {
		return ""
	}
	v, ok := p.Property(questspawn.OwnerProperty)
	if !ok {
		return ""
	}
	owner, _ := v.(string)
	return owner
}

// questSpawnBlockedFrom reports whether a placed entity is a quest spawn the
// actor must not see or interact with (quest-spawns.md Phase 2): a foreign
// spawn — owned by another player — blocks unless the viewer is a bypassing
// staffer (admin with `showspawns` on). It is the defensive guard the
// feature-specific room scans (loot, harvest, shop, mount, auction, campfire)
// apply so the owner gate holds beyond the generic ArgEntity / render seam —
// e.g. a future quest-spawned mob that also carries a shop or node role would
// otherwise be interactable by non-owners. A non-quest entity and the viewer's
// own spawn never block.
func (c *Context) questSpawnBlockedFrom(e entities.Entity) bool {
	owner := questSpawnOwner(e)
	if owner == "" || c.Actor == nil || owner == c.Actor.PlayerID() {
		return false
	}
	// Staff bypass mirrors the gate (owner + the `showspawns` clutter toggle).
	if actorIsAdmin(c.Actor, c.AdminRole) && viewerShowsOtherQuestSpawns(c.Actor) {
		return false
	}
	return true
}

// QuestSpawnVisible builds the render-side quest-spawn filter for a viewer
// (quest-spawns.md Phase 2): it reports whether a placed entity should appear
// in the viewer's room render. A quest-spawned entity owned by another player
// is hidden; ordinary entities and the viewer's own spawns show. Staff bypass
// the gate entirely (moderation/inspection), mirroring the resolve-side admin
// pierce (§10 admin bypass) — UNLESS the staffer has silenced the clutter with
// `showspawns off`, in which case they fall back to owner-only visibility.
// Returns nil (show everything) for a bypassing staff viewer, or when the
// viewer has no player identity — tests, headless, and pre-login renders — so
// those paths keep the legacy behavior. This mirrors the resolve-side
// SourceQuestSpawn gate so "what you see" and "what you can target" stay
// consistent (visibility §5.4).
func QuestSpawnVisible(viewer Actor, adminRole string) func(entities.Entity) bool {
	if viewer == nil {
		return nil
	}
	if actorIsAdmin(viewer, adminRole) && viewerShowsOtherQuestSpawns(viewer) {
		return nil // staff bypass in effect
	}
	viewerID := viewer.PlayerID()
	if viewerID == "" {
		return nil
	}
	return func(e entities.Entity) bool {
		owner := questSpawnOwner(e)
		return owner == "" || owner == viewerID
	}
}

// viewerSeesInvisible reports whether the actor carries the see-invisible
// counter (§4.3): honored as a racial/ability tag (HasTag, like darkvision)
// OR as an active effect flag (HasFlag). Either grants the binary pierce.
func (c *Context) viewerSeesInvisible() bool {
	if t, ok := c.Actor.(taggable); ok && t.HasTag(SeeInvisibleFlag) {
		return true
	}
	return c.Effects != nil && c.Effects.HasFlag(c.Actor.PlayerID(), SeeInvisibleFlag)
}

// viewerHasUltrasound reports whether the actor has ultrasound (§3, §4.3):
// sourced from a racial tag (HasTag, like darkvision) OR an equipped capability
// (a cybereye enhancement's `grants: [ultrasound]`, item-modification §6). Feeds
// both the darkness pierce and the hidden-detect terms in visibilityPredicate.
func (c *Context) viewerHasUltrasound() bool {
	if t, ok := c.Actor.(taggable); ok && t.HasTag(UltrasoundFlag) {
		return true
	}
	if v, ok := c.Actor.(visionCapable); ok && v.HasEquippedCapability(UltrasoundFlag) {
		return true
	}
	return false
}

// magicalInvisibleOccupants returns the set of room players carrying an active
// `invisible` effect flag (§3.4), keyed by player id. Empty when the locator
// or effect manager is unwired or no one is invisible. The observer is
// self-excluded (self is always visible to self, §2.1). Players only in v1 —
// magically invisible mobs are a future extension (§9), matching hide/sneak.
func magicalInvisibleOccupants(c *Context, roomID world.RoomID) map[string]struct{} {
	if c.Locator == nil || c.Effects == nil {
		return nil
	}
	self := c.Actor.PlayerID()
	var out map[string]struct{}
	for _, p := range c.Locator.PlayersInRoom(roomID) {
		pid := p.PlayerID()
		if pid == "" || pid == self {
			continue
		}
		if !c.Effects.HasFlag(pid, InvisibleFlag) {
			continue
		}
		if out == nil {
			out = make(map[string]struct{})
		}
		out[pid] = struct{}{}
	}
	return out
}

// adminInvisRank is the binary admin rank used for wizinvis (§3.4): with the
// engine's flat role model there is one staff tier, so an admin-invis layer
// carries rank 1 and any admin observer (also rank 1) pierces it while an
// ordinary observer (rank 0) does not.
const adminInvisRank = 1

// questSpawnBypassRank is the minimum admin rank that bypasses the quest-spawn
// existence gate (quest-spawns.md §10 admin bypass) so staff can see/target
// another player's foreign quest spawns for moderation. Same flat staff tier
// as admin-invis; an ordinary viewer (rank 0) never pierces.
const questSpawnBypassRank = adminInvisRank

// adminInvisibleOccupants returns the set of room players currently walking
// invisibly via `wizinvis` (§3.4), keyed by player id. Empty when the locator
// is unwired or no one is wizinvis. The observer is self-excluded (self is
// always visible to self, §2.1).
func adminInvisibleOccupants(c *Context, roomID world.RoomID) map[string]struct{} {
	if c.Locator == nil {
		return nil
	}
	self := c.Actor.PlayerID()
	var out map[string]struct{}
	for _, p := range c.Locator.PlayersInRoom(roomID) {
		if self != "" && p.PlayerID() == self {
			continue
		}
		ai, ok := p.(adminInvisible)
		if !ok || !ai.IsAdminInvisible() {
			continue
		}
		if out == nil {
			out = make(map[string]struct{})
		}
		out[p.PlayerID()] = struct{}{}
	}
	return out
}

// movingConcealment is the mover-read capability for the §3.2 movement
// filter: a mover that may be sneaking. connActor implements it; a
// non-implementer (test actor) is treated as not sneaking, so its movement
// broadcasts unfiltered.
type movingConcealment interface {
	IsSneaking() bool
	SneakConcealmentScore() int
}

// sneakUnseenBy returns the player ids of the occupants of roomID who FAIL to
// perceive the sneaking mover, so the movement broadcaster can add them to
// its exclusion list (visibility §3.2: an occupant who pierces the sneak sees
// the enter/leave line, one who does not sees nothing). Returns nil when the
// mover is not sneaking, the locator is unwired, or no roller is available —
// the legacy path (everyone sees the line).
//
// The contest is a FRESH per-call perception check, deliberately NOT routed
// through the §4.1 sticky detection memory: movement is event-driven, so
// there is no per-look re-roll to dedupe, and recording into another actor's
// detection set from the mover's goroutine would add a needless cross-actor
// writer. (The sticky memory remains a render/resolver — i.e. CanSee —
// concern; §4.1.) The mover itself is never in the returned set.
func sneakUnseenBy(c *Context, roomID world.RoomID, mover Actor) []string {
	mc, ok := mover.(movingConcealment)
	if !ok || !mc.IsSneaking() || c.Locator == nil || c.SkillRoller == nil {
		return nil // legacy permissive path: everyone sees the line
	}
	score := mc.SneakConcealmentScore()
	moverID := mover.PlayerID()
	var unseen []string
	for _, o := range c.Locator.PlayersInRoom(roomID) {
		oid := o.PlayerID()
		if oid == "" || oid == moverID {
			continue // self always perceives its own movement (§2.1)
		}
		// An occupant pierces iff it has a perception capability and wins the
		// §4.2 contest against the sneak score. No perceiver ⇒ cannot pierce ⇒
		// does not see the line. (A nil roller short-circuits to the permissive
		// path above, so it never reaches here.)
		per, hasPer := o.(perceiver)
		if !hasPer || !progression.ResolveSkillCheck(c.SkillRoller, per.PerceptionBonus(), score).Success {
			unseen = append(unseen, oid)
		}
	}
	return unseen
}

// hiddenOccupants returns the hide concealment layer of every currently-
// hidden player in the room, keyed by player id (visibility §3.1). Empty
// when the locator is unwired or no one is hidden. Mobs are not hideable in
// v1, so only players contribute (§9).
func hiddenOccupants(c *Context, roomID world.RoomID) map[string]visibility.Layer {
	if c.Locator == nil {
		return nil
	}
	self := c.Actor.PlayerID()
	var out map[string]visibility.Layer
	for _, p := range c.Locator.PlayersInRoom(roomID) {
		// Self is always visible to self (§2.1); exclude the observer from the
		// hidden map so the function is correct independent of the CanSee
		// self-guard and the upstream self-exclusions.
		if self != "" && p.PlayerID() == self {
			continue
		}
		h, ok := p.(hideable)
		if !ok || !h.IsHidden() {
			continue
		}
		if out == nil {
			out = make(map[string]visibility.Layer)
		}
		out[p.PlayerID()] = visibility.Layer{
			Source:   visibility.SourceHide,
			Score:    h.ConcealmentScore(),
			Instance: h.HiddenInstance(),
		}
	}
	return out
}

// luminousItemID reports whether the id names a room item that emits light
// (a lit torch on the ground) — such a target is visible in the dark to
// anyone (visibility §3.3). Mobs and players are not luminous in v1, so a
// non-item id is never luminous.
//
// Precondition: callers pass only IDs that came from the room's candidate
// lists (RoomItems/RoomEntities), which BuildResolveContext already filters
// to room-placed entities. The luminosity check itself reads c.Items by id
// and does NOT re-validate room placement, so passing an arbitrary in-world
// item id (e.g. one in an inventory) would report its raw lit state — fine
// for the current callers (the predicate, fed only room candidates), but
// revisit this if the helper gains other callers.
func luminousItemID(items *entities.Store, id string) bool {
	if items == nil {
		return false
	}
	it, ok := itemInstanceByID(items, entities.EntityID(id))
	if !ok {
		return false
	}
	return light.Contribution(it) > light.Black
}
