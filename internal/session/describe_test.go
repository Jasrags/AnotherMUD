package session

import (
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/progression"
)

func describeTestActor(name string, race *progression.Race, class *progression.Class) *connActor {
	return &connActor{
		save:  &player.Save{ID: "p-1", Name: name},
		race:  race,
		class: class,
	}
}

func TestConnActorDescription_RaceAndClass(t *testing.T) {
	a := describeTestActor("Alice",
		&progression.Race{ID: "human", DisplayName: "Human", Tagline: "The adaptable everyfolk of the realms."},
		&progression.Class{ID: "fighter", DisplayName: "Fighter"},
	)
	got := a.Description()
	// Name, "a Human Fighter" noun phrase, and the race tagline.
	for _, want := range []string{"Alice", "a Human Fighter", "adaptable everyfolk"} {
		if !strings.Contains(got, want) {
			t.Errorf("Description() = %q, missing %q", got, want)
		}
	}
}

func TestConnActorDescription_VowelArticle(t *testing.T) {
	a := describeTestActor("Borin",
		&progression.Race{ID: "elf", DisplayName: "Elf"},
		&progression.Class{ID: "archer", DisplayName: "Archer"},
	)
	if got := a.Description(); !strings.Contains(got, "an Elf Archer") {
		t.Errorf("Description() = %q, want vowel article 'an Elf Archer'", got)
	}
}

func TestConnActorDescription_RaceOnly(t *testing.T) {
	a := describeTestActor("Cara",
		&progression.Race{ID: "dwarf", DisplayName: "Dwarf"},
		nil,
	)
	got := a.Description()
	if !strings.Contains(got, "a Dwarf.") {
		t.Errorf("Description() = %q, want race-only noun 'a Dwarf.'", got)
	}
}

func TestConnActorDescription_NoRaceOrClassIsEmpty(t *testing.T) {
	// Neither set → empty string so the look handler renders its generic
	// fallback rather than a vacuous "You see Alice, a ."
	a := describeTestActor("Dane", nil, nil)
	if got := a.Description(); got != "" {
		t.Errorf("raceless+classless Description() = %q, want empty", got)
	}
}
