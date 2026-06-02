package help

import (
	"strings"
	"testing"
)

func topic(pack, id, title, cat string, role Role, keywords ...string) *Topic {
	return &Topic{PackName: pack, ID: id, Title: title, Category: cat, Role: role, Keywords: keywords}
}

func newSvc(t *testing.T) *Service {
	t.Helper()
	s := NewService()
	if !s.AddTopic(topic("core", "look", "Look", "commands", RoleNone, "examine", "l"), 0) {
		t.Fatal("add look")
	}
	if !s.AddTopic(topic("core", "score", "Score Sheet", "commands", RolePlayer, "stats"), 0) {
		t.Fatal("add score")
	}
	if !s.AddTopic(topic("core", "wizinfo", "Wizard Info", "admin", RoleAdmin, "wiz"), 0) {
		t.Fatal("add wiz")
	}
	return s
}

func TestAddTopicRejectsMissingFields(t *testing.T) {
	s := NewService()
	if s.AddTopic(&Topic{Title: "No ID"}, 0) {
		t.Error("topic without id should be rejected")
	}
	if s.AddTopic(&Topic{ID: "x"}, 0) {
		t.Error("topic without title should be rejected")
	}
}

func TestQueryExactIDAndNamespaced(t *testing.T) {
	s := newSvc(t)
	for _, term := range []string{"look", "core:look", "LOOK"} {
		r := s.Query("player1", term)
		if r.Status != StatusOK || r.Topic.ID != "look" {
			t.Errorf("Query(%q) = %+v", term, r)
		}
	}
}

func TestQueryExactTitle(t *testing.T) {
	s := newSvc(t)
	r := s.Query("player1", "score sheet")
	if r.Status != StatusOK || r.Topic.ID != "score" {
		t.Errorf("title query = %+v", r)
	}
}

func TestQueryFuzzyKeyword(t *testing.T) {
	s := newSvc(t)
	r := s.Query("player1", "examine") // keyword of look
	if r.Status != StatusOK || r.Topic.ID != "look" {
		t.Errorf("keyword query = %+v", r)
	}
}

func TestQueryMultiple(t *testing.T) {
	s := NewService()
	s.AddTopic(topic("core", "cast", "Cast Spell", "magic", RoleNone, "magic"), 0)
	s.AddTopic(topic("core", "spells", "Spell List", "magic", RoleNone, "magic"), 0)
	r := s.Query("p1", "magic")
	if r.Status != StatusMultiple || len(r.Matches) != 2 {
		t.Fatalf("expected 2 matches, got %+v", r)
	}
	// sorted by namespaced id: core:cast before core:spells
	if r.Matches[0].ID != "cast" || r.Matches[1].ID != "spells" {
		t.Errorf("match order = %+v", r.Matches)
	}
}

func TestQueryNoMatch(t *testing.T) {
	s := newSvc(t)
	r := s.Query("p1", "nonsense")
	if r.Status != StatusNoMatch || r.Term != "nonsense" {
		t.Errorf("no-match = %+v", r)
	}
}

func TestRoleGate(t *testing.T) {
	s := newSvc(t)
	// admin topic invisible to a player (placeholder gate)
	if r := s.Query("player1", "wizinfo"); r.Status != StatusNoMatch {
		t.Errorf("admin topic should be hidden from player: %+v", r)
	}
	// pre-login (empty id) sees only role-less: score (player) hidden
	if r := s.Query("", "score"); r.Status != StatusNoMatch {
		t.Errorf("player topic should be hidden pre-login: %+v", r)
	}
	// role-less look visible pre-login
	if r := s.Query("", "look"); r.Status != StatusOK {
		t.Errorf("roleless topic should be visible pre-login: %+v", r)
	}
}

func TestLoadOrderPrecedence(t *testing.T) {
	s := NewService()
	s.AddTopic(&Topic{PackName: "a", ID: "look", Title: "Old Look"}, 0)
	// higher order overrides the bare-id + title lookups
	s.AddTopic(&Topic{PackName: "b", ID: "look", Title: "New Look"}, 5)
	r := s.Query("p1", "look")
	if r.Status != StatusOK || r.Topic.Title != "New Look" {
		t.Errorf("higher load-order should win: %+v", r)
	}
	// a lower-order late registration must not override
	s.AddTopic(&Topic{PackName: "c", ID: "look", Title: "Lower"}, 1)
	r = s.Query("p1", "look")
	if r.Topic.Title != "New Look" {
		t.Errorf("lower load-order must not override: %+v", r)
	}
}

func TestListAndCategories(t *testing.T) {
	s := newSvc(t)
	cmds := s.List("player1", "commands")
	if len(cmds) != 2 {
		t.Errorf("commands category = %d topics, want 2", len(cmds))
	}
	// wizinfo is admin-role and hidden from a player, so its "admin"
	// category must not appear — only "commands" is visible.
	cats := s.Categories("player1")
	if strings.Join(cats, ",") != "commands" {
		t.Errorf("categories = %v, want [commands] (admin role-gated out)", cats)
	}
}

// adminResolver maps a single id to admin tier, everyone else to player —
// standing in for the composition root's manager-backed resolver.
func adminResolver(adminID string) RoleResolver {
	return func(entityID string) Role {
		if entityID == adminID {
			return RoleAdmin
		}
		return RolePlayer
	}
}

// With a resolver elevating "admin1", the admin-tier topic resolves for the
// admin but stays hidden from an ordinary player (§9.5).
func TestRoleResolver_AdminSeesAdminTopic(t *testing.T) {
	s := newSvc(t)
	s.SetRoleResolver(adminResolver("admin1"))

	if r := s.Query("admin1", "wizinfo"); r.Status != StatusOK || r.Topic.ID != "wizinfo" {
		t.Errorf("admin Query(wizinfo) = %+v, want OK", r)
	}
	if r := s.Query("player1", "wizinfo"); r.Status != StatusNoMatch {
		t.Errorf("player Query(wizinfo) = %+v, want NoMatch (still gated)", r)
	}
}

// The admin category + its topics list for an admin and not for a player.
func TestRoleResolver_AdminCategoryAndList(t *testing.T) {
	s := newSvc(t)
	s.SetRoleResolver(adminResolver("admin1"))

	if got := s.List("admin1", "admin"); len(got) != 1 || got[0].ID != "wizinfo" {
		t.Errorf("admin List(admin) = %+v, want [wizinfo]", got)
	}
	if got := s.List("player1", "admin"); len(got) != 0 {
		t.Errorf("player List(admin) = %+v, want empty", got)
	}

	if !containsStr(s.Categories("admin1"), "admin") {
		t.Errorf("admin Categories missing 'admin': %v", s.Categories("admin1"))
	}
	if containsStr(s.Categories("player1"), "admin") {
		t.Errorf("player Categories should not include 'admin': %v", s.Categories("player1"))
	}
}

// A resolver never elevates the pre-login (empty) id — it short-circuits to
// RoleNone before the resolver runs.
func TestRoleResolver_EmptyEntityStaysNone(t *testing.T) {
	s := newSvc(t)
	s.SetRoleResolver(adminResolver("")) // would elevate "" if consulted

	if r := s.Query("", "wizinfo"); r.Status != StatusNoMatch {
		t.Errorf("pre-login Query(wizinfo) = %+v, want NoMatch", r)
	}
	if r := s.Query("", "score"); r.Status != StatusNoMatch {
		t.Errorf("pre-login Query(score) = %+v, want NoMatch (player-tier gated)", r)
	}
}

func containsStr(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
