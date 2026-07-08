package session

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// TestConnActorPrimaryTrack locks the primary-track resolution (the track kill-XP
// is granted to and `score` displays): the first bound class's bound_track, else
// the fallback. Regression guard for the c66cea0 fix — see feat.go PrimaryTrack.
func TestConnActorPrimaryTrack(t *testing.T) {
	reg := progression.NewClassRegistry()
	must := func(c *progression.Class) {
		t.Helper()
		if err := reg.Register(c); err != nil {
			t.Fatalf("register %s: %v", c.ID, err)
		}
	}
	must(&progression.Class{ID: "fighter", BoundTrack: "adventurer"})
	must(&progression.Class{ID: "street-samurai", BoundTrack: "street"})
	must(&progression.Class{ID: "trackless", BoundTrack: ""}) // a class with no bound track

	tests := []struct {
		name     string
		classIDs []string
		fallback string
		want     string
	}{
		{"classless → fallback", nil, "adventurer", "adventurer"},
		{"single class → its bound track", []string{"street-samurai"}, "adventurer", "street"},
		{"multiclass → first bound class wins", []string{"street-samurai", "fighter"}, "adventurer", "street"},
		{"first class trackless → next bound class", []string{"trackless", "street-samurai"}, "adventurer", "street"},
		{"all classes trackless → fallback", []string{"trackless"}, "one-power", "one-power"},
		{"unknown class id → fallback", []string{"nonexistent"}, "adventurer", "adventurer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &connActor{classes: reg, classIDs: tt.classIDs}
			if got := a.PrimaryTrack(tt.fallback); got != tt.want {
				t.Errorf("PrimaryTrack(%q) with classes %v = %q, want %q", tt.fallback, tt.classIDs, got, tt.want)
			}
		})
	}
}
