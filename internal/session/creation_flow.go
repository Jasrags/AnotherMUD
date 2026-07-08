package session

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/ansi"
	"github.com/Jasrags/AnotherMUD/internal/conn"
	"github.com/Jasrags/AnotherMUD/internal/feat"
	"github.com/Jasrags/AnotherMUD/internal/help"
	"github.com/Jasrags/AnotherMUD/internal/login"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/render"
	"github.com/Jasrags/AnotherMUD/internal/wizard"
)

// M12.3 — the interactive character-creation flow (spec §3-§7). The
// wizard primitive (internal/wizard) runs as a post-login, pre-actor
// phase: it assembles the player's chosen race/class onto the baseline
// save BEFORE the runtime connActor (and its race/class application +
// alignment seed) is built, so the existing M12.2 commit path persists
// the chosen values unchanged. Because the actor isn't built or in the
// Manager during the wizard, a mid-creation disconnect persists nothing
// (§8) — runCreation just returns on the read error.

// Telnet echo control for secret steps (the default creation flow has
// none, but the IO honors them for any pack flow that does). Mirrors
// login's sequences; duplicated here to keep the session layer free of a
// login-internal dependency.
var (
	creationIacWillEcho = []byte{0xFF, 0xFB, 0x01}
	creationIacWontEcho = []byte{0xFF, 0xFC, 0x01}
)

// maxCreationRestarts caps the §7 restart loop so a malformed pack flow
// whose completion handler always fails can't spin forever consuming a
// client's input. The engine-default flow can only fail via a confirm
// "no" (a deliberate user action), so this never bites it; it's a guard
// for future pack-supplied flows.
const maxCreationRestarts = 8

// ErrCreationAbandoned is returned by runCreation when the restart cap is
// hit. run() treats it like a disconnect: nothing is persisted, the
// connection closes.
var ErrCreationAbandoned = errors.New("session: character creation abandoned after too many restarts")

// creationEntity is the pending character the flow assembles (spec §1
// "pending entity"). Choice handlers populate it; runCreation copies the
// chosen ids onto the baseline save after validation. rejected is set by
// a confirm "no" so OnComplete can trigger a restart (§7).
type creationEntity struct {
	raceID       string
	classID      string
	backgroundID string
	gender       string
	rejected     bool

	// channelingGift records the WoT channeling origin chosen at the
	// channeling step ("spark"/"learn"/"none"). It is consequential: the
	// WoT flow's gift-gated class steps offer channeler vs non-channeler
	// classes based on it (the decoupled capability gate — see
	// giftedClassStep / progression.Class.AllowsGift), and runCreation
	// stamps it onto loaded.Player.ChannelingGift (persisted, save v28).
	channelingGift string

	// backgroundFeat is the feat chosen from the background's FeatOptions
	// (the pick-one chooser); empty when the background offers <2 options
	// (the granter then auto-grants the single option). backgroundEquipment is
	// the chosen EquipmentPackages index (0 default). runCreation stamps both
	// onto the save (v29); the character.created granter applies them.
	backgroundFeat      string
	backgroundEquipment int
}

// genderOptions is the v1 binary gender set offered at creation. Gender is a
// general character attribute (it fills the engine's pre-existing
// AllowedGenders eligibility contract); in the WoT pack it also derives a
// channeler's saidin/saidar affinity (WoT S2 Phase 3). Static (no registry) —
// a pack wanting a different set is a future content-driven concern.
var genderOptions = []wizard.Option{
	{Label: "Male", Value: "male"},
	{Label: "Female", Value: "female"},
}

// CreationFlowFor selects a creation flow by the server's primary active
// world (character-identity §2). Customized worlds branch here ("Option A":
// the flow assembly is per-pack Go, while the wizard engine stays generic);
// every other world — "", "starter-world", or an unknown namespace — takes
// the engine-default NewCreationFlow, preserved byte-for-byte. The world
// string is the namespace of the single kind:world pack (registries.Worlds[0]
// today; co-host is deferred). A nil return propagates (the §2 "no flow →
// immediate commit" path) regardless of branch.
func CreationFlowFor(world string, races *progression.RaceRegistry, classes *progression.ClassRegistry, backgrounds *progression.BackgroundRegistry, feats *feat.Registry) *wizard.Flow {
	switch strings.ToLower(strings.TrimSpace(world)) {
	case "wot":
		return newWoTCreationFlow(world, races, classes, backgrounds, feats)
	default:
		return newDefaultCreationFlow(world, races, classes, backgrounds, feats)
	}
}

// NewCreationFlow builds the engine-default creation flow from the race
// and class registries (spec §2/§3): an intro, a gender choice, a race
// choice (when any races are registered), a class choice (when any classes
// are), an optional background choice, and a confirm. The completion
// handler (§6.3) fails only when the player declined at the confirm step,
// which restarts the flow (§7); a missing race/class is acceptable because
// the downstream applyRace/applyClass fall back to the configured defaults.
// Returns nil when there is nothing to choose (no races AND no classes AND
// no backgrounds) so the caller takes the §2 "no flow → immediate commit"
// path.
func NewCreationFlow(races *progression.RaceRegistry, classes *progression.ClassRegistry, backgrounds *progression.BackgroundRegistry, feats *feat.Registry) *wizard.Flow {
	return newDefaultCreationFlow("", races, classes, backgrounds, feats)
}

// newDefaultCreationFlow is NewCreationFlow with the active world threaded in for
// menu scoping (a world's own classes/backgrounds hide the tapestry-core
// baseline; see worldClassFilter). world == "" disables scoping — the shape the
// unit-test NewCreationFlow wrapper uses.
func newDefaultCreationFlow(world string, races *progression.RaceRegistry, classes *progression.ClassRegistry, backgrounds *progression.BackgroundRegistry, feats *feat.Registry) *wizard.Flow {
	if !hasCreationContent(races, classes, backgrounds) {
		return nil
	}
	steps := []wizard.Step{introStep(), genderStep()}
	steps = appendCreationContent(steps, world, races, classes, backgrounds, feats, false)
	steps = append(steps, confirmStep())
	return creationFlow(steps)
}

// newWoTCreationFlow is the Wheel-of-Time pack's creation flow. It reuses the
// engine-default intro/gender/race/background/confirm steps, but (1) inserts a
// channeling step immediately after gender — gender must precede it, since
// saidin/saidar affinity derives from the chosen gender downstream — and (2)
// replaces the single class step with a decoupled capability gate: two
// gift-gated class steps, of which exactly one renders per character. A
// channeling-capable character (spark/learn) is offered the channeler classes;
// a "cannot channel" character is offered the non-channeler classes. The gate
// lives in the static-options wizard via per-entity Skip predicates (the
// engine evaluates Skip against the channeling gift chosen two steps earlier);
// no wizard-engine change is needed. Returns nil on empty content for the same
// reason NewCreationFlow does.
func newWoTCreationFlow(world string, races *progression.RaceRegistry, classes *progression.ClassRegistry, backgrounds *progression.BackgroundRegistry, feats *feat.Registry) *wizard.Flow {
	if !hasCreationContent(races, classes, backgrounds) {
		return nil
	}
	steps := []wizard.Step{introStep(), genderStep(), channelingStep()}
	steps = appendCreationContent(steps, world, races, classes, backgrounds, feats, true)
	steps = append(steps, confirmStep())
	return creationFlow(steps)
}

// hasCreationContent reports whether any pickable creation content exists — the
// §2 "no flow → immediate commit" guard.
func hasCreationContent(races *progression.RaceRegistry, classes *progression.ClassRegistry, backgrounds *progression.BackgroundRegistry) bool {
	return (races != nil && len(races.All()) > 0) ||
		(classes != nil && len(classes.All()) > 0) ||
		(backgrounds != nil && len(backgrounds.All()) > 0)
}

// appendCreationContent adds the race → class → background → background-feat →
// background-equipment steps. Class and background options are eligibility-
// filtered against the entity's chosen race category + gender (dynamic
// OptionsFn); when giftGated, the class options are ALSO filtered by the chosen
// channeling gift (the WoT decoupled capability gate, now one dynamic step
// instead of two Skip-gated ones). The two background-choice steps Skip unless
// the chosen background offers ≥2 options. Shared by every flow builder.
func appendCreationContent(steps []wizard.Step, world string, races *progression.RaceRegistry, classes *progression.ClassRegistry, backgrounds *progression.BackgroundRegistry, feats *feat.Registry, giftGated bool) []wizard.Step {
	if races != nil && len(races.All()) > 0 {
		steps = append(steps, raceStep(raceOptions(races)))
	}
	if classes != nil && len(classes.All()) > 0 {
		steps = append(steps, dynamicClassStep(world, classes, races, giftGated))
	}
	if backgrounds != nil && len(backgrounds.All()) > 0 {
		steps = append(steps,
			dynamicBackgroundStep(world, backgrounds, races),
			backgroundFeatStep(backgrounds, feats),
			backgroundEquipmentStep(backgrounds),
		)
	}
	return steps
}

// worldClassFilter returns a keep-predicate that scopes the creation class menu
// to the active world's OWN classes when it registered any — hiding the
// tapestry-core baseline (`fighter`) that every world inherits via the core
// dependency. A world that ships no classes of its own inherits all registered
// classes (the core baseline is then the only source). world == "" (the
// unit-test NewCreationFlow path) disables scoping. Composed with the
// eligibility + gift filters in dynamicClassStep.
func worldClassFilter(classes *progression.ClassRegistry, world string) func(*progression.Class) bool {
	pass := func(*progression.Class) bool { return true }
	if world == "" || classes == nil {
		return pass
	}
	owns := false
	for _, c := range classes.All() {
		if strings.EqualFold(c.Pack, world) {
			owns = true
			break
		}
	}
	if !owns {
		return pass
	}
	return func(c *progression.Class) bool { return strings.EqualFold(c.Pack, world) }
}

// worldBackgroundFilter is worldClassFilter for backgrounds — a world that ships
// its own backgrounds hides tapestry-core's `commoner`.
func worldBackgroundFilter(backgrounds *progression.BackgroundRegistry, world string) func(*progression.Background) bool {
	pass := func(*progression.Background) bool { return true }
	if world == "" || backgrounds == nil {
		return pass
	}
	owns := false
	for _, b := range backgrounds.All() {
		if strings.EqualFold(b.Pack, world) {
			owns = true
			break
		}
	}
	if !owns {
		return pass
	}
	return func(b *progression.Background) bool { return strings.EqualFold(b.Pack, world) }
}

// eligibilityOf returns the (race category, gender) of an in-creation entity,
// for the GetEligible/EligibleFor option filter. Empty category when raceless.
func eligibilityOf(ce *creationEntity, races *progression.RaceRegistry) (category, gender string) {
	gender = ce.gender
	if races != nil && ce.raceID != "" {
		if r, ok := races.Get(ce.raceID); ok {
			category = r.Category
		}
	}
	return category, gender
}

// dynamicClassStep is the single class step (default + WoT). Its options are the
// classes the entity is eligible for (race category + gender); when giftGated
// they are further filtered by the chosen channeling gift (Class.AllowsGift) —
// the WoT capability gate. One dynamic step replaces the prior two Skip-gated
// gift steps.
func dynamicClassStep(world string, classes *progression.ClassRegistry, races *progression.RaceRegistry, giftGated bool) *wizard.ChoiceStep {
	worldKeep := worldClassFilter(classes, world)
	options := func(e wizard.Entity) []wizard.Option {
		ce := e.(*creationEntity)
		cat, gen := eligibilityOf(ce, races)
		return classOptionsFiltered(classes, func(c *progression.Class) bool {
			if !worldKeep(c) {
				return false
			}
			if !c.EligibleFor(cat, gen) {
				return false
			}
			if giftGated {
				return c.AllowsGift(ce.channelingGift)
			}
			return true
		})
	}
	return &wizard.ChoiceStep{
		ID:        "class",
		Prompt:    "Choose your class:",
		OptionsFn: options,
		OnSelect:  func(e wizard.Entity, v any) { e.(*creationEntity).classID = v.(string) },
		// Skip an empty menu (no class the entity is eligible for, given its
		// gift) rather than render an unsatisfiable prompt — matches the old
		// "omit the step" behavior and avoids a creation soft-lock. applyClass
		// falls back to the default class downstream.
		Skip: func(e wizard.Entity) bool { return len(options(e)) == 0 },
	}
}

// dynamicBackgroundStep offers the backgrounds the entity is eligible for (race
// category + gender) — closing the standing "wizard never calls GetEligible"
// gap on the same dynamic-options seam.
func dynamicBackgroundStep(world string, backgrounds *progression.BackgroundRegistry, races *progression.RaceRegistry) *wizard.ChoiceStep {
	worldKeep := worldBackgroundFilter(backgrounds, world)
	options := func(e wizard.Entity) []wizard.Option {
		ce := e.(*creationEntity)
		cat, gen := eligibilityOf(ce, races)
		return backgroundOptionsFiltered(backgrounds, func(b *progression.Background) bool {
			return worldKeep(b) && b.EligibleFor(cat, gen)
		})
	}
	return &wizard.ChoiceStep{
		ID:        "background",
		Prompt:    "Choose your background:",
		OptionsFn: options,
		OnSelect:  func(e wizard.Entity, v any) { e.(*creationEntity).backgroundID = v.(string) },
		// Skip when no eligible background (avoids an unsatisfiable prompt).
		Skip: func(e wizard.Entity) bool { return len(options(e)) == 0 },
	}
}

// backgroundFeatStep lets the player pick ONE feat from the chosen background's
// FeatOptions (backgrounds §2). Skipped unless the chosen background offers ≥2
// options — 0 means no feat choice, 1 is auto-granted by the granter.
func backgroundFeatStep(backgrounds *progression.BackgroundRegistry, feats *feat.Registry) *wizard.ChoiceStep {
	return &wizard.ChoiceStep{
		ID:     "background-feat",
		Prompt: "Choose your background feat:",
		OptionsFn: func(e wizard.Entity) []wizard.Option {
			bg := chosenBackground(e, backgrounds)
			if bg == nil {
				return nil
			}
			opts := make([]wizard.Option, 0, len(bg.FeatOptions))
			for _, fid := range bg.FeatOptions {
				label := fid
				tag, desc := "", ""
				if feats != nil {
					if f, ok := feats.Get(fid); ok {
						label = displayOr(f.DisplayName, fid)
						desc = f.Description
					}
				}
				opts = append(opts, wizard.Option{Label: label, Tag: tag, Description: desc, Value: fid})
			}
			return opts
		},
		OnSelect: func(e wizard.Entity, v any) { e.(*creationEntity).backgroundFeat = v.(string) },
		// Skip unless the chosen background offers ≥2 feat options — 0 = no
		// choice, 1 = auto-granted by the granter. Same chosenBackground path as
		// OptionsFn so the two never disagree.
		Skip: func(e wizard.Entity) bool {
			bg := chosenBackground(e, backgrounds)
			return bg == nil || len(bg.FeatOptions) < 2
		},
	}
}

// backgroundEquipmentStep lets the player pick ONE equipment package from the
// chosen background's EquipmentPackages (backgrounds §2). Skipped unless the
// chosen background offers ≥2 packages. The package label joins the bare item
// ids (namespace stripped) — readable without the item-template registry.
func backgroundEquipmentStep(backgrounds *progression.BackgroundRegistry) *wizard.ChoiceStep {
	return &wizard.ChoiceStep{
		ID:     "background-equipment",
		Prompt: "Choose your starting equipment:",
		OptionsFn: func(e wizard.Entity) []wizard.Option {
			bg := chosenBackground(e, backgrounds)
			if bg == nil {
				return nil
			}
			opts := make([]wizard.Option, 0, len(bg.EquipmentPackages))
			for i, pkg := range bg.EquipmentPackages {
				opts = append(opts, wizard.Option{Label: packageLabel(pkg), Value: i})
			}
			return opts
		},
		OnSelect: func(e wizard.Entity, v any) { e.(*creationEntity).backgroundEquipment = v.(int) },
		Skip: func(e wizard.Entity) bool {
			bg := chosenBackground(e, backgrounds)
			return bg == nil || len(bg.EquipmentPackages) < 2
		},
	}
}

// chosenBackground resolves the entity's selected background (nil when none).
func chosenBackground(e wizard.Entity, backgrounds *progression.BackgroundRegistry) *progression.Background {
	if backgrounds == nil {
		return nil
	}
	id := e.(*creationEntity).backgroundID
	if id == "" {
		return nil
	}
	b, _ := backgrounds.Get(id)
	return b
}

// packageLabel renders an equipment package as a comma-joined list of its bare
// item ids (namespace stripped), e.g. "wot:shortbow" + "wot:buckler" →
// "shortbow, buckler".
func packageLabel(pkg []string) string {
	if len(pkg) == 0 {
		return "(nothing)"
	}
	parts := make([]string, 0, len(pkg))
	for _, it := range pkg {
		if i := strings.LastIndex(it, ":"); i >= 0 {
			it = it[i+1:]
		}
		parts = append(parts, it)
	}
	return strings.Join(parts, ", ")
}

// backgroundOptionsFiltered builds sorted background options, keeping only the
// backgrounds the predicate accepts. Mirrors classOptionsFiltered.
func backgroundOptionsFiltered(backgrounds *progression.BackgroundRegistry, keep func(*progression.Background) bool) []wizard.Option {
	if backgrounds == nil {
		return nil
	}
	all := backgrounds.All()
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	opts := make([]wizard.Option, 0, len(all))
	for _, b := range all {
		if keep != nil && !keep(b) {
			continue
		}
		opts = append(opts, wizard.Option{
			Label:       displayOr(b.DisplayName, b.ID),
			Tag:         b.Tagline,
			Description: b.Description,
			Value:       b.ID,
		})
	}
	return opts
}

// creationFlow wraps an ordered step slice in the standard non-cancellable
// character-creation flow with the §6.3/§7 confirm-restart completion
// handler. Every flow builder funnels through here so the Flow envelope
// (ID, Trigger, Cancellable, OnComplete) is identical across packs.
func creationFlow(steps []wizard.Step) *wizard.Flow {
	return &wizard.Flow{
		ID:          "character-creation",
		Trigger:     "new-player",
		Cancellable: false, // §6.2 — creation cannot be cancelled
		Steps:       steps,
		OnComplete: func(_ context.Context, e wizard.Entity) (bool, string) {
			if e.(*creationEntity).rejected {
				return false, "All right, let's start over."
			}
			return true, ""
		},
	}
}

func introStep() *wizard.InfoStep {
	return &wizard.InfoStep{
		ID:   "intro",
		Text: "Time to create your character.",
	}
}

// genderStep precedes race/class so a future dynamic flow can gate
// class/background eligibility on it (AllowedGenders). Always present once
// any content exists — gender is intrinsic to the character.
func genderStep() *wizard.ChoiceStep {
	return &wizard.ChoiceStep{
		ID:       "gender",
		Prompt:   "Choose your gender:",
		Options:  genderOptions,
		OnSelect: func(e wizard.Entity, v any) { e.(*creationEntity).gender = v.(string) },
	}
}

// channelingStep is the WoT per-pack step (option (a)). It is gender-agnostic
// on purpose: wizard.ChoiceStep options are static, so saidin/saidar-specific
// wording would require two Skip-gated steps (out of scope). The downstream
// affinity already derives from Gender, so this stays flavor for now — the
// selection is recorded on the entity but not persisted.
func channelingStep() *wizard.ChoiceStep {
	return &wizard.ChoiceStep{
		ID:     "channeling",
		Prompt: "Your relationship to the One Power:",
		Options: []wizard.Option{
			{Label: "Born with the spark", Value: "spark",
				Description: "The Power came to you unbidden; you must learn control or die."},
			{Label: "Able to learn", Value: "learn",
				Description: "You could be taught to channel, given a teacher."},
			{Label: "Cannot channel", Value: "none",
				Description: "The True Source is closed to you."},
		},
		OnSelect: func(e wizard.Entity, v any) { e.(*creationEntity).channelingGift = v.(string) },
	}
}

func raceStep(opts []wizard.Option) *wizard.ChoiceStep {
	return &wizard.ChoiceStep{
		ID:       "race",
		Prompt:   "Choose your race:",
		Options:  opts,
		OnSelect: func(e wizard.Entity, v any) { e.(*creationEntity).raceID = v.(string) },
	}
}

func confirmStep() *wizard.ConfirmStep {
	return &wizard.ConfirmStep{
		ID:     "confirm",
		Prompt: "Create this character? (yes/no)",
		OnYes:  func(wizard.Entity) {},
		OnNo:   func(e wizard.Entity) { e.(*creationEntity).rejected = true },
	}
}

// raceOptions builds choice options from the race registry, sorted by id
// for a deterministic menu order.
func raceOptions(races *progression.RaceRegistry) []wizard.Option {
	if races == nil {
		return nil
	}
	all := races.All()
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	opts := make([]wizard.Option, 0, len(all))
	for _, r := range all {
		opts = append(opts, wizard.Option{
			Label:       displayOr(r.DisplayName, r.ID),
			Tag:         r.Tagline,
			Description: r.Description,
			Value:       r.ID,
		})
	}
	return opts
}

// classOptionsFiltered builds sorted class options, keeping only classes the
// predicate accepts (nil predicate = keep all). The single home for the class
// option mapping + deterministic sort; dynamicClassStep supplies the
// eligibility/gift predicate.
func classOptionsFiltered(classes *progression.ClassRegistry, keep func(*progression.Class) bool) []wizard.Option {
	if classes == nil {
		return nil
	}
	all := classes.All()
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	opts := make([]wizard.Option, 0, len(all))
	for _, c := range all {
		if keep != nil && !keep(c) {
			continue
		}
		opts = append(opts, wizard.Option{
			Label:       displayOr(c.DisplayName, c.ID),
			Tag:         c.Tagline,
			Description: c.Description,
			Value:       c.ID,
		})
	}
	return opts
}

func displayOr(display, id string) string {
	if display != "" {
		return display
	}
	return id
}

// creationIO adapts a raw connection to wizard.IO for the creation phase
// (before the connActor exists). Step text is run through the same
// themed renderer the session uses so markup in help output renders;
// SetEcho drives telnet echo for secret steps.
type creationIO struct {
	c        conn.Connection
	renderer *render.ColorRenderer
	color    bool
}

func (io *creationIO) Write(ctx context.Context, msg string) error {
	_, err := io.c.Write(ctx, []byte(renderCreation(io.renderer, msg, io.color)+"\r\n"))
	return err
}

// SetEcho follows the wizard.IO contract where on==true means "client
// echo visible" (show input) and on==false means "hide input". Telnet
// inverts this at the wire: to HIDE input the server announces it WILL
// echo (so the client stops local-echoing); to SHOW it again the server
// says it WONT. Same convention as the login package.
func (io *creationIO) SetEcho(ctx context.Context, on bool) {
	seq := creationIacWillEcho // on==false → hide input
	if on {
		seq = creationIacWontEcho // on==true → show input
	}
	if cw, ok := io.c.(interface {
		WriteCommand(context.Context, []byte) (int, error)
	}); ok {
		_, _ = cw.WriteCommand(ctx, seq)
		return
	}
	_, _ = io.c.Write(ctx, seq)
}

func renderCreation(r *render.ColorRenderer, msg string, color bool) string {
	if r == nil {
		return ansi.Render(msg, color)
	}
	if color {
		return r.RenderAnsi(msg)
	}
	return r.RenderPlain(msg)
}

// runCreation runs the interactive creation wizard over c for a new
// player, populating loaded.Player.Race/Class on success (spec §3-§7).
// A nil CreationFlow is the §2 "no flow → immediate commit" path (no-op).
// A read/write error (disconnect) is returned and persists nothing — the
// actor is not built yet (§8). A completion-handler failure restarts the
// flow against a fresh pending entity (§7).
func runCreation(ctx context.Context, c conn.Connection, cfg Config, loaded *login.Loaded) error {
	if cfg.CreationFlow == nil {
		return nil
	}
	io := &creationIO{c: c, renderer: cfg.Render, color: cfg.ColorEnabled}

	for attempt := 0; ; attempt++ { // restart loop (§7), capped
		if attempt >= maxCreationRestarts {
			_ = io.Write(ctx, "Character creation could not be completed.")
			return ErrCreationAbandoned
		}
		pending := &creationEntity{}
		// §5 structured flow-step events: a GMCP-active client gets a
		// Char.Wizard frame per rendered step for an in-place creation
		// panel; the sink is nil for plain clients, so the text path
		// (already written by each step's Render) is the universal path.
		sink := newWizardGmcpSink(c)
		inst := wizard.NewInstance(cfg.CreationFlow, pending, io, sink)
		st, err := inst.Start(ctx)
		if err != nil {
			return err
		}
		for st == wizard.StatusAwaitingInput {
			line, err := c.Read(ctx)
			if err != nil {
				return err // disconnect mid-creation → nothing persisted (§8)
			}
			// §3.2 inline inspect: `? <token>` matching a choice option shows
			// that option's detail and re-displays the menu without spending
			// the choice. Tried before help so `? warrior` inspects the class
			// rather than searching help; a non-matching token falls through to
			// the help passthrough below (so `? combat` still reaches help).
			if token, isInspect := creationInspectToken(line); isInspect {
				if handled, ierr := inst.Inspect(ctx, token); ierr != nil {
					return ierr
				} else if handled {
					continue
				}
			}
			// §4 help passthrough: answer help without advancing the step.
			if handled, herr := maybeCreationHelp(ctx, io, cfg, line); herr != nil {
				return herr
			} else if handled {
				continue
			}
			if st, err = inst.Input(ctx, line); err != nil {
				return err
			}
		}

		// §6.3 validation.
		ok, msg := true, ""
		if cfg.CreationFlow.OnComplete != nil {
			ok, msg = cfg.CreationFlow.OnComplete(ctx, pending)
		}
		if !ok {
			if msg != "" {
				if err := io.Write(ctx, msg); err != nil {
					return err
				}
			}
			continue // restart with a fresh baseline pending entity (§7)
		}

		// Success: stamp the chosen ids onto the baseline save. applyRace/
		// applyClass + the alignment seed downstream in run() consume them.
		if pending.raceID != "" {
			loaded.Player.Race = pending.raceID
		}
		if pending.classID != "" {
			// v1 single-class: commit the one chosen class as a 1-element list
			// (the save's class field is a list since v18 — wot-character-model
			// D1). A second class-track later is additive content.
			loaded.Player.Class = []string{pending.classID}
		}
		if pending.backgroundID != "" {
			// The background label (backgrounds §5). Its starting package is
			// granted at character.created (skills/items/gold), not here.
			loaded.Player.Background = pending.backgroundID
			// The pick-one chooser selections (v29): the chosen feat (empty when
			// the background offered <2 options → granter auto-grants the single
			// one) and the chosen equipment-package index. The character.created
			// granter reads these to apply the chosen feat + package.
			loaded.Player.BackgroundFeat = pending.backgroundFeat
			loaded.Player.BackgroundEquipmentChoice = pending.backgroundEquipment
		}
		if pending.gender != "" {
			// Gender (v22). A general attribute; the WoT affinity layer reads it
			// off the actor's save to derive saidin/saidar element strengths.
			loaded.Player.Gender = pending.gender
		}
		if pending.channelingGift != "" {
			// Channeling origin (v28). Only the WoT flow's channeling step sets
			// this; the default flow leaves it empty (no key written). A durable
			// trait distinct from the chosen class — score + future S2 hooks
			// read it off the save.
			loaded.Player.ChannelingGift = pending.channelingGift
		}
		return nil
	}
}

// creationInspectToken extracts the inspect token from a `? <token>` line
// (§3.2). It matches only the question-mark-prefixed form with a non-empty
// token — a bare "?" or the "help" keyword is not an inspect request and is
// left to the help passthrough. The token is whatever follows the "?".
func creationInspectToken(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "?") {
		return "", false
	}
	token := strings.TrimSpace(trimmed[1:])
	if token == "" {
		return "", false
	}
	return token, true
}

// maybeCreationHelp implements §4 help passthrough: input starting with
// "?" or the "help" keyword is answered through the help service without
// advancing the flow. Returns (handled, err).
func maybeCreationHelp(ctx context.Context, io *creationIO, cfg Config, line string) (bool, error) {
	trimmed := strings.TrimSpace(line)
	lower := strings.ToLower(trimmed)
	var term string
	switch {
	case trimmed == "?" || lower == "help":
		term = ""
	case strings.HasPrefix(trimmed, "?"):
		term = strings.TrimSpace(trimmed[1:])
	case strings.HasPrefix(lower, "help "):
		term = strings.TrimSpace(trimmed[len("help "):])
	default:
		return false, nil
	}
	if cfg.Help == nil {
		return true, io.Write(ctx, "Help is not available right now.")
	}
	if term == "" {
		return true, io.Write(ctx, "Type 'help <topic>' for help on a topic.")
	}
	res := cfg.Help.Query("", term)
	switch res.Status {
	case help.StatusOK:
		return true, io.Write(ctx, help.RenderTopic(res.Topic, 0))
	case help.StatusMultiple:
		return true, io.Write(ctx, help.RenderDisambiguation(res.Term, res.Matches, 0))
	default:
		return true, io.Write(ctx, help.RenderNoMatch(res.Term))
	}
}
