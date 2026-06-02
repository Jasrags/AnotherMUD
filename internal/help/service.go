package help

import (
	"sort"
	"strings"
	"sync"
)

// Status is the outcome of a Query (§9.6).
type Status int

const (
	StatusOK       Status = iota // exactly one topic resolved (Topic set)
	StatusMultiple               // several fuzzy matches (Matches set)
	StatusNoMatch                // nothing matched (Term echoed)
)

// Result is the structured outcome of Query. Exactly one of Topic /
// Matches is meaningful depending on Status; Term is always the original
// query term.
type Result struct {
	Status  Status
	Topic   *Topic
	Matches []Summary
	Term    string
}

type entry struct {
	topic *Topic
	order int
}

// Service holds the registered topics and answers query/list/category
// lookups with role gating. Safe for concurrent reads; registration is
// expected at boot.
type Service struct {
	mu sync.RWMutex
	// canonical set keyed by namespaced id — the dedup source of truth
	// for enumeration (fuzzy match, categories).
	byNS map[string]entry
	// lookups: bare id AND namespaced id both point here; title is
	// lower-cased; category groups namespaced ids.
	byID    map[string]entry
	byTitle map[string]entry

	// roleResolver maps a requester's entity id to their visibility tier
	// (§9.5). Injected at composition time (SetRoleResolver) so the help
	// package stays free of any session / role dependency. nil ⇒ the flat
	// default (any logged-in id is a player). Set once at boot, before
	// queries are served.
	roleResolver RoleResolver
}

// RoleResolver maps a requester's entity id to their help visibility tier
// (ui-rendering-help §9.5). The composition root supplies one backed by the
// session manager + the configured admin role, so an admin sees admin-tier
// topics and a player does not.
type RoleResolver func(entityID string) Role

// NewService returns an empty help service.
func NewService() *Service {
	return &Service{
		byNS:    make(map[string]entry),
		byID:    make(map[string]entry),
		byTitle: make(map[string]entry),
	}
}

// SetRoleResolver installs the requester-tier resolver (§9.5). Called once
// at boot; not safe to call concurrently with queries.
func (s *Service) SetRoleResolver(fn RoleResolver) { s.roleResolver = fn }

// AddTopic registers t at the given load order. A later registration of
// the same id/title wins when its order is >= the incumbent's (higher
// wins; equal keeps the newest — §9.4). Topics missing id or title are
// rejected (returns false); the loader validates separately so it can
// distinguish that from a precedence-lost false.
//
// NamespacedID is computed from PackName + ID (or the bare id) and
// written into t under the write lock. The service takes ownership of t:
// callers must not mutate or re-register the same pointer afterward.
func (s *Service) AddTopic(t *Topic, order int) bool {
	if t == nil || strings.TrimSpace(t.ID) == "" || strings.TrimSpace(t.Title) == "" {
		return false
	}
	ns := t.ID
	if t.PackName != "" {
		ns = t.PackName + ":" + t.ID
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if cur, ok := s.byNS[ns]; ok && order < cur.order {
		return false // a higher-order registration already won
	}
	t.NamespacedID = ns
	e := entry{topic: t, order: order}
	s.byNS[ns] = e
	putIfHigher(s.byID, strings.ToLower(t.ID), e)
	putIfHigher(s.byID, strings.ToLower(ns), e)
	putIfHigher(s.byTitle, strings.ToLower(t.Title), e)
	return true
}

// putIfHigher writes e at key when absent or when e.order >= the
// incumbent's order (higher wins, equal keeps newest).
func putIfHigher(m map[string]entry, key string, e entry) {
	if cur, ok := m[key]; ok && e.order < cur.order {
		return
	}
	m[key] = e
}

// requesterTier resolves the visibility tier for an entity id. Empty id
// (pre-login) sees only role-less topics. Otherwise the injected
// roleResolver (§9.5) decides — an admin sees admin-tier topics; without a
// resolver, every logged-in id is a player (the flat default).
func (s *Service) requesterTier(entityID string) Role {
	if entityID == "" {
		return RoleNone
	}
	if s.roleResolver != nil {
		return s.roleResolver(entityID)
	}
	return RolePlayer
}

func visible(t *Topic, tier Role) bool {
	return tier >= t.Role
}

// Query resolves term for the requester (§9.6): exact id → exact title →
// fuzzy keyword/title. Fuzzy with one match returns OK; several returns
// Multiple; none returns NoMatch. All paths apply the role gate.
func (s *Service) Query(entityID, term string) Result {
	tier := s.requesterTier(entityID)
	key := strings.ToLower(strings.TrimSpace(term))

	s.mu.RLock()
	defer s.mu.RUnlock()

	if e, ok := s.byID[key]; ok && visible(e.topic, tier) {
		return Result{Status: StatusOK, Topic: e.topic, Term: term}
	}
	if e, ok := s.byTitle[key]; ok && visible(e.topic, tier) {
		return Result{Status: StatusOK, Topic: e.topic, Term: term}
	}

	var matches []*Topic
	for _, e := range s.byNS {
		if !visible(e.topic, tier) {
			continue
		}
		if topicMatches(e.topic, key) {
			matches = append(matches, e.topic)
		}
	}
	switch len(matches) {
	case 0:
		return Result{Status: StatusNoMatch, Term: term}
	case 1:
		return Result{Status: StatusOK, Topic: matches[0], Term: term}
	default:
		sort.Slice(matches, func(i, j int) bool { return matches[i].NamespacedID < matches[j].NamespacedID })
		summaries := make([]Summary, len(matches))
		for i, t := range matches {
			summaries[i] = t.summary()
		}
		return Result{Status: StatusMultiple, Matches: summaries, Term: term}
	}
}

// topicMatches reports whether term (already lower-cased) appears in the
// title or any keyword, case-insensitively.
func topicMatches(t *Topic, term string) bool {
	if term == "" {
		return false
	}
	if strings.Contains(strings.ToLower(t.Title), term) {
		return true
	}
	for _, k := range t.Keywords {
		if strings.Contains(strings.ToLower(k), term) {
			return true
		}
	}
	return false
}

// HasTopic reports whether a topic with the given bare or namespaced id is
// registered (any visibility tier). Used by command help generation to skip
// verbs that already have an authored topic, so pack content always wins.
func (s *Service) HasTopic(id string) bool {
	key := strings.ToLower(strings.TrimSpace(id))
	if key == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.byID[key]
	return ok
}

// List returns visible topic summaries in a category (§9.7).
func (s *Service) List(entityID, category string) []Summary {
	tier := s.requesterTier(entityID)
	cat := strings.ToLower(strings.TrimSpace(category))

	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []Summary
	for _, e := range s.byNS {
		if !visible(e.topic, tier) {
			continue
		}
		if strings.ToLower(e.topic.Category) == cat {
			out = append(out, e.topic.summary())
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Categories returns every category name with at least one visible
// topic, sorted alphabetically (§9.7).
func (s *Service) Categories(entityID string) []string {
	tier := s.requesterTier(entityID)

	s.mu.RLock()
	defer s.mu.RUnlock()

	seen := make(map[string]struct{})
	for _, e := range s.byNS {
		if !visible(e.topic, tier) {
			continue
		}
		if e.topic.Category != "" {
			seen[e.topic.Category] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for c := range seen {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}
