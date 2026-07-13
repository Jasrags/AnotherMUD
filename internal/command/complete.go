package command

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/keyword"
)

// Tab-completion Phase 0 — the enumeration substrate (spec
// docs/specs/tab-completion.md). The completion query is a pure,
// transport-agnostic function over the command registry and the §5
// typed-argument scopes: given a partial input line and an actor's
// resolve context, it returns the ordered set of candidates for the
// token under completion, each carrying a completion token that
// round-trips through ordinary resolution (the §1 invariant).
//
// There is no surface here — no TAB key, no GMCP, no line editing. The
// only observable consumer is the role-gated `complete` debug verb at
// the bottom of this file; client surfaces are Phase 1/2 (the proposal).

// DefaultCompletionCap bounds the number of candidates one query returns
// before truncation (spec §7/§10). A small list — completion is an
// assist, not a directory dump.
const DefaultCompletionCap = 20

// CompletionKind reports what slot the query was completing (spec §2).
type CompletionKind int

const (
	// CompleteNone means the cursor token has no completable slot — an
	// argument position past the verb's declared arguments, or an
	// argument of an unknown/ungated verb. The candidate list is empty.
	CompleteNone CompletionKind = iota
	// CompleteVerb means the first token is under completion.
	CompleteVerb
	// CompleteArgument means a later token is under completion; Verb and
	// ArgIndex on the result identify which.
	CompleteArgument
)

// CandidateKind tags what a candidate names so a surface can group or
// style it. It is not used for matching.
type CandidateKind string

const (
	CandVerb   CandidateKind = "verb"
	CandItem   CandidateKind = "item"
	CandEntity CandidateKind = "entity"
	CandDoor   CandidateKind = "door"
	CandBulk   CandidateKind = "bulk"
	CandQuest  CandidateKind = "quest"
)

// Candidate is one completion option. Completion is the token to
// substitute for the partial — it MUST resolve, through ordinary
// dispatch + argument resolution, to exactly the thing Display names
// (spec §1). Display is the human label (free to be a full multi-word
// name even when Completion is a single round-tripping token).
type Candidate struct {
	Completion string
	Display    string
	Kind       CandidateKind
}

// CompletionResult is the output of a completion query (spec §2).
type CompletionResult struct {
	Target     CompletionKind
	Verb       string // resolved verb when Target == CompleteArgument
	ArgIndex   int    // declared-argument index the cursor mapped to; -1 if none
	Candidates []Candidate
	Truncated  bool // the candidate set was capped (spec §7)
}

// CompletionOptions tunes a query.
type CompletionOptions struct {
	// IsAdmin includes admin-gated verbs in verb completion and permits
	// argument completion of admin verbs (spec §3/§9). Mirror the
	// dispatch gate: pass true only when the actor holds the admin role.
	IsAdmin bool
	// Cap overrides DefaultCompletionCap when > 0.
	Cap int
}

// Complete runs the completion query (spec §2). It is read-only and
// never errors: every path yields a defined, possibly-empty result
// (spec §8). rc is the actor's resolve context (the same one the
// argument driver consults); pass the zero value for verb-only contexts.
func (r *Registry) Complete(partial string, rc ResolveContext, opts CompletionOptions) CompletionResult {
	limit := opts.Cap
	if limit <= 0 {
		limit = DefaultCompletionCap
	}

	// Trailing whitespace means the player finished the previous token
	// and is starting a new, empty one at the next position (spec §2).
	trailing := len(partial) != len(strings.TrimRight(partial, " \t"))
	fields := strings.Fields(partial)

	// Verb slot: empty line, or a single still-being-typed first token.
	if len(fields) == 0 || (len(fields) == 1 && !trailing) {
		partialVerb := ""
		if len(fields) == 1 {
			partialVerb = fields[0]
		}
		cands, trunc := r.completeVerb(partialVerb, opts.IsAdmin, limit)
		return CompletionResult{Target: CompleteVerb, ArgIndex: -1, Candidates: cands, Truncated: trunc}
	}

	// Argument slot.
	verb := fields[0]
	argTokens := fields[1:]
	var partialTok string
	var committed []string
	if trailing {
		partialTok = ""
		committed = argTokens
	} else {
		partialTok = argTokens[len(argTokens)-1]
		committed = argTokens[:len(argTokens)-1]
	}

	reg, ok := r.resolveRegistration(verb)
	if !ok {
		// Unknown verb: no argument scope to enumerate (spec §8).
		return CompletionResult{Target: CompleteNone, ArgIndex: -1}
	}
	// Admin gate (spec §3/§9): a non-admin must not learn an admin verb's
	// argument shape any more than its existence. Treat as no slot.
	if reg.admin && !opts.IsAdmin {
		return CompletionResult{Target: CompleteNone, ArgIndex: -1}
	}

	def, defIdx, ok := argDefForCursor(reg.args, committed)
	if !ok {
		// A real verb, but the cursor token has no declared argument
		// (past the end, or the verb declares none / is hand-parsed).
		return CompletionResult{Target: CompleteArgument, Verb: reg.keyword, ArgIndex: -1}
	}

	cands, trunc := completeArgument(def, partialTok, rc, limit)
	return CompletionResult{
		Target:     CompleteArgument,
		Verb:       reg.keyword,
		ArgIndex:   defIdx,
		Candidates: cands,
		Truncated:  trunc,
	}
}

// CompleteLine runs the completion query for actor on a partial input
// line, building the actor's resolve context from env the way Dispatch
// does. It is the entry point for surfaces that don't already hold a
// Context — the GMCP completion handler (and char-mode later). The
// dispatcher and the `complete`/`suggest` verbs call Complete directly.
// isAdmin (admin-verb visibility in the actor's own completion) is derived
// from the actor's role against env.AdminRole.
func (r *Registry) CompleteLine(env Env, actor Actor, partial string) CompletionResult {
	c := &Context{
		Actor:     actor,
		World:     env.World,
		Items:     env.Items,
		Placement: env.Placement,
		Locator:   env.Locator,
		Quests:    env.Quests, // ArgQuest completion needs the offer set
		registry:  r,
	}
	isAdmin := false
	if h, ok := actor.(RoleHolder); ok {
		role := env.AdminRole
		if role == "" {
			role = defaultAdminRole
		}
		isAdmin = h.HasRole(role)
	}
	return r.Complete(partial, c.BuildResolveContext(), CompletionOptions{IsAdmin: isAdmin})
}

// completeVerb returns matching verb candidates in the SAME priority
// dispatch routes with (spec §3): exact match first, then ascending
// registration order. Admin verbs appear only for admins. Both primary
// keywords and aliases are eligible (both are routable input).
func (r *Registry) completeVerb(partial string, isAdmin bool, limit int) ([]Candidate, bool) {
	lower := strings.ToLower(partial)

	r.mu.RLock()
	var matched []registration
	for _, k := range r.ordered {
		if lower != "" && !strings.HasPrefix(k, lower) {
			continue
		}
		reg := r.byKey[k]
		if reg.admin && !isAdmin {
			continue
		}
		matched = append(matched, reg)
	}
	r.mu.RUnlock()

	sort.SliceStable(matched, func(i, j int) bool {
		// Exact match sorts ahead of longer prefix matches (parity with
		// §2.3 resolution: `n` ahead of `north` for partial `n`).
		ei := lower != "" && matched[i].keyword == lower
		ej := lower != "" && matched[j].keyword == lower
		if ei != ej {
			return ei
		}
		return matched[i].order < matched[j].order
	})

	trunc := false
	if len(matched) > limit {
		matched = matched[:limit]
		trunc = true
	}
	cands := make([]Candidate, 0, len(matched))
	for _, reg := range matched {
		display := reg.keyword
		if reg.meta != nil && reg.meta.brief != "" {
			display = reg.keyword + " — " + reg.meta.brief
		}
		cands = append(cands, Candidate{Completion: reg.keyword, Display: display, Kind: CandVerb})
	}
	return cands, trunc
}

// argDefForCursor maps the cursor token to a declared argument by
// replaying the §5.4 driver's preposition-skipping walk over the
// committed tokens (everything typed before the cursor). It returns the
// matched ArgDefinition and its declared index, or ok=false when the
// cursor falls past the declared arguments (or the command declares
// none). Parity with the driver is the point: the scope this selects is
// the scope the resolver would use for the same token (spec §5).
func argDefForCursor(defs []ArgDefinition, committed []string) (ArgDefinition, int, bool) {
	i := 0 // index into committed
	for defIdx := range defs {
		def := defs[defIdx]
		// Preposition skip: a committed token matching this arg's
		// preposition is consumed as the preposition, not the arg.
		if i < len(committed) && hasPrep(def.Prepositions, committed[i]) {
			i++
		}
		// Text slurps the remainder — the cursor token is part of it.
		if def.Type == ArgText {
			return def, defIdx, true
		}
		if i < len(committed) {
			// This arg is already satisfied by a committed token.
			i++
			continue
		}
		// Committed tokens exhausted: the cursor token IS this arg.
		return def, defIdx, true
	}
	return ArgDefinition{}, -1, false
}

// completeArgument enumerates and disambiguates candidates for one
// argument slot (spec §4/§6). Free-token types (keyword/text/number)
// have nothing to enumerate.
func completeArgument(def ArgDefinition, partial string, rc ResolveContext, limit int) ([]Candidate, bool) {
	switch def.Type {
	case ArgKeyword, ArgText, ArgNumber:
		return nil, false
	case ArgDoor:
		return completeDoor(partial, rc, limit)
	case ArgQuest:
		if rc.Quests == nil {
			return nil, false
		}
		return completeQuestRefs(rc.Quests.EnumerateAcceptable(), partial, limit)
	case ArgActiveQuest:
		if rc.Quests == nil {
			return nil, false
		}
		return completeQuestRefs(rc.Quests.EnumerateActive(), partial, limit)
	default:
		named := scopeFor(def.Type, rc)
		cands := disambiguate(named, partial)

		// `visible` also matches the actor's own name (spec §4; the
		// resolver self-checks ActorName). Offer self when it prefixes.
		if def.Type == ArgVisible && rc.ActorName != "" && hasFoldPrefix(rc.ActorName, partial) {
			cands = append([]Candidate{{
				Completion: rc.ActorName, Display: rc.ActorName, Kind: CandEntity,
			}}, cands...)
		}

		// Bulk slots may additionally offer the `all` / `all.<kw>`
		// grammar (spec §4).
		if def.Bulk {
			cands = append(cands, bulkCandidates(named, partial)...)
		}
		return capCandidates(cands, limit)
	}
}

// scopeFor selects the candidate scope for an argument type, reusing the
// exact same per-scope candidate sets the resolvers consult (spec §4).
func scopeFor(t ArgType, rc ResolveContext) []keyword.Named {
	switch t {
	case ArgInventory:
		return itemsAsNamed(rc.Inventory)
	case ArgEquipped:
		return itemsAsNamed(rc.Equipped)
	case ArgShopItem:
		if rc.Shop != nil {
			return rc.Shop.EnumerateStock()
		}
		return nil
	case ArgRoomItem:
		return itemsAsNamed(rc.RoomItems)
	case ArgContainer:
		return append(containersAsNamed(rc.Inventory), containersAsNamed(rc.RoomItems)...)
	case ArgEntity:
		return entitiesAsNamed(rc.RoomEntities)
	case ArgPlayer:
		return entitiesAsNamed(filterEntityType(rc.RoomEntities, entityTypePlayer))
	case ArgNPC:
		return entitiesAsNamed(filterEntityType(rc.RoomEntities, entityTypeMob))
	case ArgGiveTarget:
		return entitiesAsNamed(rc.RoomEntities) // players + mobs are valid recipients
	case ArgFindable:
		return append(itemsAsNamed(rc.Inventory), itemsAsNamed(rc.RoomItems)...)
	case ArgVisible:
		// Self is added by completeArgument; here, items then entities,
		// in the resolver's scan order (inventory → room → entities).
		out := append(itemsAsNamed(rc.Inventory), itemsAsNamed(rc.RoomItems)...)
		return append(out, entitiesAsNamed(rc.RoomEntities)...)
	}
	return nil
}

// disambiguate turns the scope-order matched set for a partial into
// individually-addressable candidates (spec §6): a distinguishing
// keyword where one uniquely resolves to the entity, else an ordinal
// token (`kw`, `2.kw`, `3.kw`) for true duplicates. Every returned
// completion token round-trips to its distinct entity.
func disambiguate(scope []keyword.Named, partial string) []Candidate {
	matched := filterScope(scope, partial)
	out := make([]Candidate, 0, len(matched))
	for _, e := range matched {
		token := distinguishingToken(scope, e, partial)
		out = append(out, Candidate{
			Completion: token,
			Display:    e.Name(),
			Kind:       candKindOf(e),
		})
	}
	return out
}

// distinguishingToken picks the completion token for e within scope: the
// first of e's own keywords (name-first-word when it has none) that
// resolves uniquely to e, preferring one that extends the partial; or,
// for a true duplicate, the ordinal selector that lands on e in scope
// order (spec §6).
func distinguishingToken(scope []keyword.Named, e keyword.Named, partial string) string {
	tokens := candidateTokens(e)
	lower := strings.ToLower(partial)

	// Prefer a unique token that extends what the player typed, then any
	// unique token, so the suggestion visibly grows the partial.
	for _, preferExtending := range []bool{true, false} {
		for _, t := range tokens {
			if preferExtending && lower != "" && !strings.HasPrefix(t, lower) {
				continue
			}
			if uniqueResolves(scope, t, e) {
				return t
			}
		}
	}

	// True duplicate: ordinal selector on the first usable base keyword.
	base := ""
	if len(tokens) > 0 {
		base = tokens[0]
	}
	if base == "" {
		return "" // degenerate (no keyword, empty name) — unaddressable
	}
	n := scopeOrdinal(scope, base, e)
	if n <= 1 {
		return base
	}
	return fmt.Sprintf("%d.%s", n, base)
}

// candidateTokens is the ordered set of single tokens that could address
// e: its keywords (lowercased) or, when it has none (e.g. a player),
// the first word of its name.
func candidateTokens(e keyword.Named) []string {
	kws := e.Keywords()
	if len(kws) > 0 {
		out := make([]string, 0, len(kws))
		for _, k := range kws {
			out = append(out, strings.ToLower(k))
		}
		return out
	}
	if f := firstWord(e.Name()); f != "" {
		return []string{strings.ToLower(f)}
	}
	return nil
}

// uniqueResolves reports whether token t resolves to e and ONLY e within
// scope — checked through the real resolver (ResolveAll) so the verdict
// matches what submitting t would do (spec §1/§6).
func uniqueResolves(scope []keyword.Named, t string, e keyword.Named) bool {
	matches := keyword.ResolveAll(scope, t)
	return len(matches) == 1 && sameCandidate(matches[0], e)
}

// scopeOrdinal returns e's 1-based position among the scope entities
// that match base, in scope iteration order — the index the ordinal
// selector `n.base` will land on (spec §6).
func scopeOrdinal(scope []keyword.Named, base string, e keyword.Named) int {
	n := 0
	for _, c := range scope {
		if keyword.Matches(c, base) {
			n++
			if sameCandidate(c, e) {
				return n
			}
		}
	}
	// e was not found among base-matchers. Return 0 (not the running
	// count) so the caller emits the bare base rather than an ordinal
	// that would alias a DIFFERENT entity — a §1 round-trip breach.
	// Unreachable for real candidates (every item/entity has a stable
	// id sameCandidate matches on); the guard is defensive.
	return 0
}

// filterScope returns scope members matching partial under the resolver
// rules; an empty partial matches the whole scope (spec §4).
func filterScope(scope []keyword.Named, partial string) []keyword.Named {
	if partial == "" {
		return scope
	}
	var out []keyword.Named
	for _, c := range scope {
		if keyword.Matches(c, partial) {
			out = append(out, c)
		}
	}
	return out
}

// bulkCandidates offers the bulk grammar for a bulk-capable slot (spec
// §4): `all`, and `all.<kw>` for any keyword shared by ≥2 matched
// entities, each gated by the partial.
func bulkCandidates(scope []keyword.Named, partial string) []Candidate {
	lower := strings.ToLower(partial)
	var out []Candidate
	if partial == "" || strings.HasPrefix("all", lower) {
		out = append(out, Candidate{Completion: "all", Display: "all (everything)", Kind: CandBulk})
	}
	// Keywords shared by ≥2 matched entities qualify for all.<kw>.
	matched := filterScope(scope, partial)
	counts := map[string]int{}
	for _, e := range matched {
		seen := map[string]bool{}
		for _, k := range e.Keywords() {
			lk := strings.ToLower(k)
			if seen[lk] {
				continue
			}
			seen[lk] = true
			counts[lk]++
		}
	}
	shared := make([]string, 0)
	for k, n := range counts {
		if n >= 2 {
			shared = append(shared, k)
		}
	}
	sort.Strings(shared)
	for _, k := range shared {
		tok := "all." + k
		if partial == "" || strings.HasPrefix(tok, lower) || strings.HasPrefix(k, lower) {
			out = append(out, Candidate{Completion: tok, Display: tok, Kind: CandBulk})
		}
	}
	return out
}

// completeDoor enumerates the room's doors (spec §4) when the door scope
// supports enumeration; otherwise no candidates. The completion token is
// the door's direction short string, which always round-trips through
// the door resolver; matching also accepts the door's name words.
func completeDoor(partial string, rc ResolveContext, limit int) ([]Candidate, bool) {
	if rc.Doors == nil {
		return nil, false
	}
	en, ok := rc.Doors.(doorEnumerator)
	if !ok {
		return nil, false
	}
	lower := strings.ToLower(partial)
	var cands []Candidate
	for _, d := range en.EnumerateDoors() {
		if partial == "" || doorMatches(d, lower) {
			display := d.Direction
			if d.Door.Name != "" {
				display = fmt.Sprintf("%s (%s)", d.Door.Name, d.Direction)
			}
			cands = append(cands, Candidate{Completion: d.Direction, Display: display, Kind: CandDoor})
		}
	}
	return capCandidates(cands, limit)
}

// completeQuestRefs filters a quest-ref set for one quest slot (spec
// tab-completion §4) — shared by ArgQuest (accept: room offers) and
// ArgActiveQuest (abandon: active quests). The completion token is the
// bare quest id, which round-trips through quest.Service.ResolveID (the
// §1 invariant); the partial matches the bare id OR the display name, so
// "ga" finds both "gate-patrol" and "Gate Patrol". An empty ref set
// yields no candidates.
func completeQuestRefs(refs []QuestRef, partial string, limit int) ([]Candidate, bool) {
	lower := strings.ToLower(partial)
	var cands []Candidate
	seen := map[string]bool{}
	for _, q := range refs {
		if q.BareID == "" || seen[q.BareID] {
			continue
		}
		if partial != "" && !strings.HasPrefix(q.BareID, lower) && !hasFoldPrefix(q.Name, partial) {
			continue
		}
		seen[q.BareID] = true
		cands = append(cands, Candidate{Completion: q.BareID, Display: q.Name, Kind: CandQuest})
	}
	return capCandidates(cands, limit)
}

// doorMatches reports whether lowerPartial prefixes the door's direction
// or any of its match keywords (DoorState.Keywords, carried through
// DoorInfo by EnumerateDoors). Using the door's own keywords — not its
// name words — keeps the completion filter aligned with the resolver,
// which matches on Keywords; content may set Keywords that differ from
// the name words.
func doorMatches(d DoorRef, lowerPartial string) bool {
	if strings.HasPrefix(strings.ToLower(d.Direction), lowerPartial) {
		return true
	}
	for _, kw := range d.Door.Keywords {
		if strings.HasPrefix(strings.ToLower(kw), lowerPartial) {
			return true
		}
	}
	return false
}

// --- small helpers ---

func capCandidates(cands []Candidate, limit int) ([]Candidate, bool) {
	if len(cands) > limit {
		return cands[:limit], true
	}
	return cands, false
}

// sameCandidate compares two scope members by their stable entity id
// (every item/entity candidate exposes EntityID). Falls back to
// interface equality for any candidate without one.
func sameCandidate(a, b keyword.Named) bool {
	ida, idb := candidateID(a), candidateID(b)
	if ida != "" || idb != "" {
		return ida == idb
	}
	return a == b
}

func candidateID(n keyword.Named) string {
	if c, ok := n.(interface{ EntityID() string }); ok {
		return c.EntityID()
	}
	return ""
}

// candKindOf tags an item candidate as an item and everything else as an
// entity (the only two scope shapes besides doors).
func candKindOf(n keyword.Named) CandidateKind {
	if _, ok := n.(ItemCandidate); ok {
		return CandItem
	}
	return CandEntity
}

func firstWord(s string) string {
	if i := strings.IndexByte(strings.TrimSpace(s), ' '); i > 0 {
		return strings.TrimSpace(s)[:i]
	}
	return strings.TrimSpace(s)
}

func hasFoldPrefix(s, prefix string) bool {
	if prefix == "" {
		return true
	}
	return strings.HasPrefix(strings.ToLower(s), strings.ToLower(prefix))
}

// --- the `complete` debug verb (spec §9) ---

// CompleteHandler runs the completion query on the supplied partial line
// and prints the result. It is admin-gated (registered Admin: true) and
// read-only — an introspection tool that backs the substrate, NOT the
// player completion surface (Phase 1/2).
//
// Limitation: dispatch trims the raw line, so a trailing space the
// player typed (`complete get `) is lost and an empty-partial argument
// slot can't be expressed through the verb. Unit tests exercise the
// trailing-space path by calling Complete directly; the verb is for
// live smoke of the filled-token paths.
func CompleteHandler(ctx context.Context, c *Context) error {
	if c.registry == nil {
		return c.Actor.Write(ctx, "Completion is not available.")
	}
	partial := strings.Join(c.Args, " ")
	// Reaching this handler means dispatch already cleared the admin gate
	// (the verb is Admin: true), so the actor holds the admin role under
	// whatever Env.AdminRole the server configured. Pass IsAdmin: true
	// directly rather than re-deriving it against defaultAdminRole, which
	// would be wrong under a custom admin-role string.
	res := c.registry.Complete(partial, c.BuildResolveContext(), CompletionOptions{IsAdmin: true})

	var b strings.Builder
	switch res.Target {
	case CompleteVerb:
		b.WriteString("Completing: verb\n")
	case CompleteArgument:
		if res.ArgIndex >= 0 {
			fmt.Fprintf(&b, "Completing: argument %d of %q\n", res.ArgIndex, res.Verb)
		} else {
			fmt.Fprintf(&b, "Completing: argument of %q (no declared slot)\n", res.Verb)
		}
	default:
		b.WriteString("Completing: (no completable slot)\n")
	}
	if len(res.Candidates) == 0 {
		b.WriteString("  (no candidates)")
	} else {
		for _, cand := range res.Candidates {
			fmt.Fprintf(&b, "  [%s] %s — %s\n", cand.Kind, cand.Completion, cand.Display)
		}
		if res.Truncated {
			b.WriteString("  … (truncated)")
		}
	}
	return c.Actor.Write(ctx, strings.TrimRight(b.String(), "\n"))
}
