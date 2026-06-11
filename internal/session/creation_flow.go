package session

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/ansi"
	"github.com/Jasrags/AnotherMUD/internal/conn"
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
	rejected     bool
}

// NewCreationFlow builds the engine-default creation flow from the race
// and class registries (spec §2/§3): an intro, a race choice (when any
// races are registered), a class choice (when any classes are), and a
// confirm. The completion handler (§6.3) fails only when the player
// declined at the confirm step, which restarts the flow (§7); a missing
// race/class is acceptable because the downstream applyRace/applyClass
// fall back to the configured defaults. Returns nil when there is
// nothing to choose (no races AND no classes) so the caller takes the
// §2 "no flow → immediate commit" path.
func NewCreationFlow(races *progression.RaceRegistry, classes *progression.ClassRegistry, backgrounds *progression.BackgroundRegistry) *wizard.Flow {
	var steps []wizard.Step
	raceOpts := raceOptions(races)
	classOpts := classOptions(classes)
	bgOpts := backgroundOptions(backgrounds)
	if len(raceOpts) == 0 && len(classOpts) == 0 && len(bgOpts) == 0 {
		return nil
	}

	steps = append(steps, &wizard.InfoStep{
		ID:   "intro",
		Text: "Time to create your character.",
	})
	if len(raceOpts) > 0 {
		steps = append(steps, &wizard.ChoiceStep{
			ID:       "race",
			Prompt:   "Choose your race:",
			Options:  raceOpts,
			OnSelect: func(e wizard.Entity, v any) { e.(*creationEntity).raceID = v.(string) },
		})
	}
	if len(classOpts) > 0 {
		steps = append(steps, &wizard.ChoiceStep{
			ID:       "class",
			Prompt:   "Choose your class:",
			Options:  classOpts,
			OnSelect: func(e wizard.Entity, v any) { e.(*creationEntity).classID = v.(string) },
		})
	}
	if len(bgOpts) > 0 {
		steps = append(steps, &wizard.ChoiceStep{
			ID:       "background",
			Prompt:   "Choose your background:",
			Options:  bgOpts,
			OnSelect: func(e wizard.Entity, v any) { e.(*creationEntity).backgroundID = v.(string) },
		})
	}
	steps = append(steps, &wizard.ConfirmStep{
		ID:     "confirm",
		Prompt: "Create this character? (yes/no)",
		OnYes:  func(wizard.Entity) {},
		OnNo:   func(e wizard.Entity) { e.(*creationEntity).rejected = true },
	})

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

func classOptions(classes *progression.ClassRegistry) []wizard.Option {
	if classes == nil {
		return nil
	}
	all := classes.All()
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	opts := make([]wizard.Option, 0, len(all))
	for _, c := range all {
		opts = append(opts, wizard.Option{
			Label:       displayOr(c.DisplayName, c.ID),
			Tag:         c.Tagline,
			Description: c.Description,
			Value:       c.ID,
		})
	}
	return opts
}

// backgroundOptions builds choice options from the background registry,
// sorted by id (backgrounds §3). Eligibility (AllowedCategories/Genders) is
// not enforced in the static wizard — like class options, every background is
// offered; the gates exist for a future dynamic flow.
func backgroundOptions(backgrounds *progression.BackgroundRegistry) []wizard.Option {
	if backgrounds == nil {
		return nil
	}
	all := backgrounds.All()
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	opts := make([]wizard.Option, 0, len(all))
	for _, b := range all {
		opts = append(opts, wizard.Option{
			Label:       displayOr(b.DisplayName, b.ID),
			Tag:         b.Tagline,
			Description: b.Description,
			Value:       b.ID,
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
		}
		return nil
	}
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
