// Package command implements the keyword registry and player-input
// dispatcher described in docs/specs/commands-and-dispatch.md.
//
// M1 scope (intentionally small):
//   - Registration: keyword + handler. No aliases, no priority, no
//     roles, no arg types, no GMCP, no help generation.
//   - Resolution: exact match first, then prefix match against all
//     registered keywords; ties broken by registration order.
//   - Dispatch: player route only. Empty input → no-op. Unknown verb
//     → "Huh?". No chain (";"), no repeat ("3n"), no flood control.
//
// The narrow surface is deliberate: M1 only needs look / movement /
// quit, and any additional machinery now would be guesswork before a
// real consumer (pack-registered commands, mob route) shows up.
package command

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// ErrQuit is returned by Dispatch when the actor's quit verb fires.
// The session loop unwinds cleanly on this — it is a signal, not a
// failure.
var ErrQuit = errors.New("command: quit")

// actorRoomID returns the actor's current room id, or "" when roomless
// (mid-transition / test actors). Used by the §6 unknown-verb log.
func actorRoomID(a Actor) world.RoomID {
	if r := a.Room(); r != nil {
		return r.ID
	}
	return ""
}

// truncateForLog caps s at maxRunes runes (appending an ellipsis when cut) so
// an oversized line can't bloat a log entry. Rune-aware so it never splits a
// multibyte rune.
func truncateForLog(s string, maxRunes int) string {
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "…"
}

// Handler is the function invoked for a matched command.
type Handler func(ctx context.Context, c *Context) error

// defaultCommandCategory is the bucket a command's generated help topic
// lands in when its registration leaves Category unset (spec
// commands-and-dispatch §8).
const defaultCommandCategory = "commands"

// Command is a full command registration (spec commands-and-dispatch
// §2.1). Beyond the keyword + handler that Register takes, it carries the
// optional metadata listing and help-generation UIs need: aliases that
// route to the same handler, a category, a one-line brief, and synthesized
// syntax lines. A registration that supplies any of this metadata becomes
// discoverable via Commands() (and thus help generation); a bare
// keyword+handler does not.
type Command struct {
	Keyword  string
	Aliases  []string
	Category string // defaults to "commands" when any metadata is present
	Brief    string
	Syntax   []string
	Keywords []string
	Handler  Handler

	// Admin marks the command as administrative (admin-verbs §2): the
	// dispatcher refuses it — with the SAME "Huh?" an unknown verb produces,
	// so the verb is not enumerable — unless the actor holds the configured
	// admin role (Env.AdminRole). Admin commands are also hidden from help
	// for non-admins. The check runs once, at dispatch, before the handler.
	Admin bool

	// Args declares the command's typed arguments (commands-and-
	// dispatch §5). When non-empty, Dispatch resolves them against the
	// actor's scope BEFORE calling the handler (Option A): on success
	// the resolved values land in Context.Resolved keyed by each
	// ArgDefinition.Name; on a resolution failure the dispatcher writes
	// the error to the actor and the handler never runs. When empty,
	// Dispatch skips resolution entirely and the handler reads the raw
	// Context.Args tokens as before — this is what lets handlers
	// migrate onto the pipeline one at a time.
	Args []ArgDefinition

	// HandParsed marks a command that declares Args for completion and
	// help synthesis (commands-and-dispatch §5/§8, tab-completion §4) but
	// parses them ITSELF in the handler — the dispatcher must NOT
	// auto-resolve them. Used by verbs whose argument scope can't be
	// expressed by the single-scope auto-resolve pipeline: `get` (item
	// scope flips on the `from` preposition) and `kill` (self-check must
	// run before resolving, and the entity arg excludes self). The
	// handler keeps reading raw Args; completion gets the type info for
	// free. When false (the default), declared Args are auto-resolved as
	// before.
	HandParsed bool

	// BreaksConcealment marks a "loud" command that reveals a hidden/sneaking
	// actor (visibility §4.5): attacking, casting an offensive weave, speaking
	// aloud, or loud manipulation. Before such a command's handler runs, the
	// dispatcher drops the actor's roll-based concealment and emits
	// entity.revealed(acted), so the action is observed. Quiet commands (look,
	// inventory, score) leave it false. Magical/admin invisibility (a flag-
	// gated source) is exempt — it does not break on action (§3.4).
	BreaksConcealment bool

	// IsAction marks a command that takes a physical action and is therefore
	// refused while the actor is occupied by an in-flight timed action
	// (action-economy.md §4): the dispatcher returns "You are busy <label>."
	// before the handler runs, unless this dispatch is itself the deferred
	// completion replaying the action (Env.ReplayAction). Passive/parser verbs
	// (look, score, say, quit, stop) leave it false — a busy player can still
	// observe, talk, and disconnect.
	IsAction bool
}

// CommandInfo is the read-only view of a registered command's metadata,
// returned by Commands() for listing and help-generation UIs. Slice fields
// are fresh copies — safe to mutate.
type CommandInfo struct {
	Keyword  string
	Aliases  []string
	Category string
	Brief    string
	Syntax   []string
	Keywords []string
	// Admin is true for an administrative command (admin-verbs §2). Help
	// listings hide these from actors who don't hold the admin role.
	Admin bool
	// Args is the command's typed-argument declaration (§5), surfaced so
	// the help generator can synthesize a syntax line from it (§8). Empty
	// for untyped commands.
	Args []ArgDefinition
}

// cmdMeta is the stored metadata for a primary command registration. It is
// non-nil only on the primary entry of a RegisterCommand call that carried
// metadata; bare Register and alias entries leave it nil so they're
// excluded from listings.
type cmdMeta struct {
	category string
	brief    string
	syntax   []string
	keywords []string
	aliases  []string
	admin    bool
	// args is the command's typed-argument declaration, retained so the
	// help generator can synthesize a syntax line from it (§8). Stored on
	// the primary registration only; aliases inherit via the primary.
	args []ArgDefinition
}

type registration struct {
	keyword string
	order   int
	handler Handler
	// alias marks an entry that routes to a primary's handler under an
	// alternate keyword; aliases never appear in Commands().
	alias bool
	// meta is non-nil only on a primary registration that carried
	// metadata. It is the source for Commands() / help generation.
	meta *cmdMeta
	// args is the command's declared typed-argument list (§5). Empty
	// for handlers not yet migrated onto the arg-typing pipeline;
	// Dispatch resolves it before the handler runs when non-empty (and
	// handParsed is false). Carried on aliases too so an alias resolves
	// (and completes) identically to its primary.
	args []ArgDefinition
	// handParsed suppresses auto-resolution of args at dispatch — the
	// handler parses them itself (see Command.HandParsed). Completion
	// still reads args. Carried on aliases alongside args.
	handParsed bool
	// admin gates the command on the admin role at dispatch (admin-verbs
	// §2). Carried on every registration (primary AND alias) so an alias
	// of an admin command is gated too.
	admin bool
	// breaksConcealment reveals a hidden/sneaking actor before the handler
	// runs (visibility §4.5; see Command.BreaksConcealment). Carried on
	// aliases too so `take` reveals exactly like `get`.
	breaksConcealment bool
	// isAction gates the command on the actor's busy state at dispatch
	// (action-economy.md §4; see Command.IsAction). Carried on aliases too.
	isAction bool
}

// Registry holds the command keyword → handler bindings.
//
// All public methods are safe for concurrent use, but in M1 the
// expectation is "register at boot, read during play".
type Registry struct {
	mu      sync.RWMutex
	byKey   map[string]registration
	order   int
	ordered []string // keywords in registration order, for prefix scans

	// argResolvers is the §5 arg-typing resolver registry the
	// dispatcher consults for commands that declare Args. Seeded with
	// the engine-baseline resolvers; packs extend it via ArgResolvers.
	argResolvers *ArgResolverRegistry
}

// New returns an empty Registry seeded with the engine-baseline arg
// resolvers (keyword/text/number/inventory/…/door).
func New() *Registry {
	return &Registry{
		byKey:        make(map[string]registration),
		argResolvers: NewArgResolverRegistry(),
	}
}

// ArgResolvers exposes the dispatcher's arg-type resolver registry so
// the composition root can register pack-authored arg types (§5.3)
// before play begins. Never nil for a Registry built via New().
func (r *Registry) ArgResolvers() *ArgResolverRegistry {
	return r.argResolvers
}

// Register binds keyword to h with no listing metadata. Keywords are
// stored lowercased. Duplicate keywords return an error. Commands
// registered this way are routable but invisible to Commands() / help
// generation — use RegisterCommand to make a command discoverable.
func (r *Registry) Register(keyword string, h Handler) error {
	return r.RegisterCommand(Command{Keyword: keyword, Handler: h})
}

// RegisterCommand binds c.Keyword (and each alias) to c.Handler. Keywords
// and aliases are stored lowercased; an exact match on any of them resolves
// to the handler. If c carries any metadata (category, brief, syntax,
// keywords, or aliases) the primary keyword becomes discoverable via
// Commands(). A duplicate primary keyword or alias returns an error, and
// alias collisions are detected before any mutation so a partial command is
// never left registered.
func (r *Registry) RegisterCommand(c Command) error {
	if c.Keyword == "" {
		return errors.New("command.RegisterCommand: empty keyword")
	}
	if c.Handler == nil {
		return errors.New("command.RegisterCommand: nil handler")
	}

	var meta *cmdMeta
	if c.Category != "" || c.Brief != "" || len(c.Syntax) > 0 || len(c.Keywords) > 0 || len(c.Aliases) > 0 {
		cat := c.Category
		if cat == "" {
			cat = defaultCommandCategory
		}
		meta = &cmdMeta{
			category: cat,
			brief:    c.Brief,
			syntax:   append([]string(nil), c.Syntax...),
			keywords: append([]string(nil), c.Keywords...),
			aliases:  append([]string(nil), c.Aliases...),
			admin:    c.Admin,
			args:     append([]ArgDefinition(nil), c.Args...),
		}
	}

	k := strings.ToLower(c.Keyword)
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.byKey[k]; exists {
		return fmt.Errorf("command.RegisterCommand: duplicate keyword %q", k)
	}
	// Pre-validate aliases so a mid-list collision can't leave the
	// primary registered without its aliases.
	lowered := make([]string, 0, len(c.Aliases))
	for _, a := range c.Aliases {
		la := strings.ToLower(a)
		if la == "" {
			return fmt.Errorf("command.RegisterCommand: empty alias for %q", k)
		}
		if la == k {
			return fmt.Errorf("command.RegisterCommand: alias equals keyword %q", k)
		}
		if _, exists := r.byKey[la]; exists {
			return fmt.Errorf("command.RegisterCommand: duplicate alias %q", la)
		}
		lowered = append(lowered, la)
	}

	r.order++
	r.byKey[k] = registration{
		keyword:           k,
		order:             r.order,
		handler:           c.Handler,
		meta:              meta,
		args:              append([]ArgDefinition(nil), c.Args...),
		handParsed:        c.HandParsed,
		admin:             c.Admin,
		breaksConcealment: c.BreaksConcealment,
		isAction:          c.IsAction,
	}
	r.ordered = append(r.ordered, k)
	for _, la := range lowered {
		r.order++
		// Aliases carry the primary's args + handParsed so dispatch
		// resolution and completion behave identically whether the player
		// typed the primary keyword or an alias (e.g. `shut` == `close`,
		// `take` == `get`). Meta stays nil so aliases remain out of help
		// listings.
		r.byKey[la] = registration{
			keyword:           la,
			order:             r.order,
			handler:           c.Handler,
			alias:             true,
			args:              append([]ArgDefinition(nil), c.Args...),
			handParsed:        c.HandParsed,
			admin:             c.Admin,
			breaksConcealment: c.BreaksConcealment,
			isAction:          c.IsAction,
		}
		r.ordered = append(r.ordered, la)
	}
	return nil
}

// Commands returns the metadata for every discoverable primary command
// (those registered via RegisterCommand with metadata), sorted by keyword.
// Aliases and bare Register entries are excluded. Used by listing UIs and
// help generation.
func (r *Registry) Commands() []CommandInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []CommandInfo
	for _, k := range r.ordered {
		reg := r.byKey[k]
		if reg.alias || reg.meta == nil {
			continue
		}
		out = append(out, CommandInfo{
			Keyword:  reg.keyword,
			Aliases:  append([]string(nil), reg.meta.aliases...),
			Category: reg.meta.category,
			Brief:    reg.meta.brief,
			Syntax:   append([]string(nil), reg.meta.syntax...),
			Keywords: append([]string(nil), reg.meta.keywords...),
			Admin:    reg.meta.admin,
			Args:     append([]ArgDefinition(nil), reg.meta.args...),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Keyword < out[j].Keyword })
	return out
}

// Resolve returns the handler that the verb routes to, or nil if no
// match. Exact match wins; on no exact match, the keyword with the
// smallest registration-order index whose name has verb as a prefix
// wins (spec §2.3). Thin wrapper over resolveRegistration so the
// routing rule lives in one place.
func (r *Registry) Resolve(verb string) Handler {
	reg, ok := r.resolveRegistration(verb)
	if !ok {
		return nil
	}
	return reg.handler
}

// resolveRegistration is the §2.3 routing used by Dispatch: it returns
// the full matched registration (handler + declared Args), not just
// the handler, so the dispatcher can pre-resolve typed arguments.
// Resolution order matches Resolve exactly — exact match wins, else
// the lowest registration-order prefix match.
func (r *Registry) resolveRegistration(verb string) (registration, bool) {
	v := strings.ToLower(verb)
	if v == "" {
		return registration{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if reg, ok := r.byKey[v]; ok {
		return reg, true
	}
	var matches []registration
	for _, k := range r.ordered {
		if strings.HasPrefix(k, v) {
			matches = append(matches, r.byKey[k])
		}
	}
	if len(matches) == 0 {
		return registration{}, false
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].order < matches[j].order })
	return matches[0], true
}

// Dispatch parses a raw input line and routes it. Empty / whitespace
// input is a no-op (spec §3.1 step 1). Unknown verbs send "Huh?" to
// the actor and return nil (the bad-input tracker lands later).
//
// env carries the per-server singletons handlers may need (world,
// broadcaster, item store, placement). Any field may be nil; handlers
// MUST guard before dereferencing.
func (r *Registry) Dispatch(ctx context.Context, env Env, actor Actor, raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	fields := strings.Fields(trimmed)
	verb := fields[0]
	args := fields[1:]

	reg, ok := r.resolveRegistration(verb)
	if !ok {
		// Bad-input tracking (§6): record + log the unknown verb. This is the
		// player route only (mobs dispatch elsewhere), so the tracker never
		// sees a mob verb. The admin-gate "Huh?" below is a KNOWN verb being
		// refused and is deliberately not recorded here.
		env.BadInput.Record(verb)
		// Sanitize the untrusted verb + raw input before logging: a control
		// char or newline could otherwise forge a log line under the text
		// handler. raw is also length-capped so a 64KB WS line can't bloat
		// the log.
		logging.From(ctx).Debug("unknown verb",
			slog.String("event", "command.unknown"),
			slog.String("verb", logging.Sanitize(strings.ToLower(verb))),
			slog.String("raw", logging.Sanitize(truncateForLog(raw, 256))),
			slog.String("player", actor.Name()),
			slog.String("room_id", string(actorRoomID(actor))))
		return actor.Write(ctx, "Huh?")
	}

	// Admin gate (admin-verbs §2): an admin-marked command is refused —
	// with the IDENTICAL "Huh?" an unknown verb produces, so a non-admin
	// cannot tell the verb exists — unless the actor holds the admin role.
	// Checked once here, before the Context is built and the handler runs.
	if reg.admin {
		adminRole := env.AdminRole
		if adminRole == "" {
			adminRole = defaultAdminRole
		}
		holder, ok := actor.(RoleHolder)
		if !ok || !holder.HasRole(adminRole) {
			return actor.Write(ctx, "Huh?")
		}
	}

	// Busy gate (action-economy.md §4): an action-marked command is refused
	// while the actor has a timed action in flight — unless this dispatch is
	// the deferred completion replaying the action itself. Checked here, before
	// the Context is built and the handler runs, mirroring the admin gate.
	if reg.isAction && !env.ReplayAction && env.Actions != nil {
		if ider, ok := actor.(interface{ PlayerID() string }); ok {
			if act, busy := env.Actions.Active(ider.PlayerID()); busy {
				label := act.Label
				if label == "" {
					label = "occupied"
				}
				return actor.Write(ctx, fmt.Sprintf("You are busy %s.", label))
			}
		}
	}

	c := &Context{
		Actor:                 actor,
		Actions:               env.Actions,
		ReplayAction:          env.ReplayAction,
		DonTicks:              env.DonTicks,
		Follow:                env.Follow,
		Group:                 env.Group,
		ActorByID:             env.ActorByID,
		World:                 env.World,
		Broadcaster:           env.Broadcaster,
		Items:                 env.Items,
		Placement:             env.Placement,
		Contents:              env.Contents,
		Slots:                 env.Slots,
		Bus:                   env.Bus,
		Properties:            env.Properties,
		Rarity:                env.Rarity,
		Essence:               env.Essence,
		Stacking:              env.Stacking,
		Locator:               env.Locator,
		Roster:                env.Roster,
		BadInput:              env.BadInput,
		Disposition:           env.Disposition,
		Combat:                env.Combat,
		Flee:                  env.Flee,
		ResolveAttack:         env.ResolveAttack,
		ReloadScripts:         env.ReloadScripts,
		Progression:           env.Progression,
		Faction:               env.Faction,
		Effects:               env.Effects,
		EffectTemplates:       env.EffectTemplates,
		SkillRoller:           env.SkillRoller,
		RecognitionDifficulty: env.RecognitionDifficulty,
		Training:              env.Training,
		Abilities:             env.Abilities,
		Proficiency:           env.Proficiency,
		ActionQueue:           env.ActionQueue,
		Recipes:               env.Recipes,
		Known:                 env.Known,
		Craft:                 env.Craft,
		Gathering:             env.Gathering,
		Biomes:                env.Biomes,
		Grades:                env.Grades,
		ForageTables:          env.ForageTables,
		WeatherState:          env.WeatherState,
		Help:                  env.Help,
		Quests:                env.Quests,
		Currency:              env.Currency,
		Money:                 env.Money,
		Mounts:                env.Mounts,
		Hirelings:             env.Hirelings,
		HirelingCap:           env.HirelingCap,
		Guides:                env.Guides,
		Spawn:                 env.Spawn,
		RangedFlavor:          env.RangedFlavor,
		Trades:                env.Trades,
		Auction:               env.Auction,
		Shop:                  env.Shop,
		Rest:                  env.Rest,
		Consumable:            env.Consumable,
		Notifications:         env.Notifications,
		TellResolver:          env.TellResolver,
		RoleTargetResolver:    env.RoleTargetResolver,
		GrantingRole:          env.GrantingRole,
		AdminRole:             env.AdminRole,
		DefaultXPTrack:        env.DefaultXPTrack,
		Announcer:             env.Announcer,
		PlayerRoom:            env.PlayerRoom,
		ChatRegistry:          env.ChatRegistry,
		ChatSubscribers:       env.ChatSubscribers,
		ChatScrollbacks:       env.ChatScrollbacks,
		Clock:                 env.Clock,
		Ambience:              env.Ambience,
		Light:                 env.Light,
		NowTick:               env.NowTick,
		CorpseOwnershipWindow: env.CorpseOwnershipWindow,
		ReloadTicks:           env.ReloadTicks,
		DefaultMoveCost:       env.DefaultMoveCost,
		Raw:                   trimmed,
		Verb:                  strings.ToLower(verb),
		Args:                  args,
		ArgResolver:           r.argResolvers,
		registry:              r,
	}

	// §5 arg-typing (Option A): when the command declares typed args,
	// resolve them against the actor's scope before the handler runs.
	// A resolution failure is terminal for this input — the dispatcher
	// writes the player-facing error and the handler never executes.
	// Commands with no declared Args — or HandParsed commands that
	// declare Args only for completion/help — skip this entirely and read
	// the raw c.Args tokens themselves (the incremental-migration path).
	if len(reg.args) > 0 && !reg.handParsed {
		resolved, warnings, _, err := r.argResolvers.ResolveArgsWithContext(
			reg.args, args, c.BuildResolveContext())
		for _, w := range warnings {
			logging.From(ctx).Debug("argres warning", "verb", c.Verb, "warning", w)
		}
		if err != nil {
			return actor.Write(ctx, err.Error())
		}
		c.Resolved = resolved
	}

	// Reveal-on-action (visibility §4.5): a "loud" command drops the actor's
	// roll-based concealment before the handler runs. Placed after a
	// successful typed-arg resolution, so a typed-arg command that failed to
	// resolve a target ("kill nothing") returns its arg error above and does
	// NOT reveal its caller. Hand-parsed loud verbs (kill, get) resolve their
	// target INSIDE the handler, so reaching here means they reveal on
	// attempt regardless of whether the target exists — an accepted v1
	// over-reveal for those few verbs.
	if reg.breaksConcealment {
		breakConcealmentOnAction(ctx, c)
	}

	return reg.handler(ctx, c)
}
