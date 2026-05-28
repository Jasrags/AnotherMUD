package quest

import "testing"

func markerDef(id string) *Definition {
	return &Definition{
		ID: id, Abandonable: true, Giver: "core:giver-mob",
		Stages: []Stage{
			{ID: "s0", Objectives: []Objective{
				{ID: "s0-collect-0", Type: "collect", Target: "core:gem"},
				{ID: "s0-deliver-1", Type: "deliver", Target: "core:letter", NPC: "core:mayor"},
				{ID: "s0-kill-2", Type: "kill", Target: "core:rat"},
			}},
			{ID: "s1", Objectives: []Objective{
				{ID: "s1-collect-0", Type: "collect", Target: "core:later-item"},
			}},
		},
	}
}

func markerService(t *testing.T) *Service {
	t.Helper()
	reg := NewRegistry()
	if err := reg.Register(markerDef("q")); err != nil {
		t.Fatal(err)
	}
	svc := NewService(Config{Registry: reg})
	svc.Accept(&fakePlayer{id: "p1"}, "q", false)
	return svc
}

func TestHasMarker(t *testing.T) {
	svc := markerService(t)
	tests := []struct {
		template string
		want     bool
		why      string
	}{
		{"core:giver-mob", true, "giver always marks"},
		{"core:gem", true, "current-stage collect target"},
		{"core:mayor", true, "current-stage deliver npc"},
		{"core:rat", false, "kill objectives never mark"},
		{"core:letter", false, "deliver item target is not the marked entity (npc is)"},
		{"core:later-item", false, "next-stage collect not yet active"},
		{"core:unrelated", false, "unrelated entity"},
		{"", false, "empty template"},
	}
	for _, tt := range tests {
		if got := svc.HasMarker("p1", tt.template); got != tt.want {
			t.Errorf("HasMarker(%q) = %v, want %v (%s)", tt.template, got, tt.want, tt.why)
		}
	}
}

func TestHasMarkerNoStateOrPlayer(t *testing.T) {
	svc := markerService(t)
	if svc.HasMarker("ghost", "core:gem") {
		t.Error("player with no state should have no markers")
	}
}

func TestMarkerAfterStageAdvance(t *testing.T) {
	svc := markerService(t)
	// complete stage 0 → stage 1; the next-stage collect target now marks,
	// and the stage-0 targets no longer do.
	svc.AdvanceObjective("p1", "q", "s0-collect-0", 1)
	svc.AdvanceObjective("p1", "q", "s0-deliver-1", 1)
	svc.AdvanceObjective("p1", "q", "s0-kill-2", 1) // completes stage 0
	if !svc.HasMarker("p1", "core:later-item") {
		t.Error("stage-1 collect target should mark after advance")
	}
	if svc.HasMarker("p1", "core:gem") {
		t.Error("stage-0 collect target should no longer mark")
	}
	// giver still marks across stages
	if !svc.HasMarker("p1", "core:giver-mob") {
		t.Error("giver should still mark after stage advance")
	}
}

func TestSecretQuestNoMarkers(t *testing.T) {
	reg := NewRegistry()
	d := markerDef("sec")
	d.Secret = true
	_ = reg.Register(d)
	svc := NewService(Config{Registry: reg})
	svc.Accept(&fakePlayer{id: "p1"}, "sec", false)
	for _, id := range []string{"core:giver-mob", "core:gem", "core:mayor"} {
		if svc.HasMarker("p1", id) {
			t.Errorf("secret quest should not mark %q", id)
		}
	}
}

func TestMarkedTemplatesBulk(t *testing.T) {
	svc := markerService(t)
	in := []string{"core:rat", "core:gem", "core:unrelated", "core:giver-mob"}
	got := svc.MarkedTemplates("p1", in)
	if len(got) != 2 || got[0] != "core:gem" || got[1] != "core:giver-mob" {
		t.Errorf("MarkedTemplates = %v, want [core:gem core:giver-mob]", got)
	}
}
