package emote

import "testing"

func TestSubstitute_NamesAndPronouns(t *testing.T) {
	alice := Subject{Name: "Alice", Pronouns: DefaultPronouns}
	bob := Subject{Name: "Bob", Pronouns: DefaultPronouns}

	got := Substitute("$n smiles at $N and waves $s hand at $S friend.", alice, bob)
	want := "Alice smiles at Bob and waves their hand at their friend."
	if got != want {
		t.Errorf("Substitute = %q, want %q", got, want)
	}
}

func TestSubstitute_ReflexiveForms(t *testing.T) {
	alice := Subject{Name: "Alice", Pronouns: DefaultPronouns}
	bob := Subject{Name: "Bob", Pronouns: DefaultPronouns}
	got := Substitute("$n hugs $M.", alice, bob)
	if got != "Alice hugs themselves." {
		t.Errorf("reflexive = %q", got)
	}
}

func TestSubstitute_UnknownTokenPassesThrough(t *testing.T) {
	alice := Subject{Name: "Alice", Pronouns: DefaultPronouns}
	got := Substitute("$n waves $x.", alice, Subject{})
	if got != "Alice waves $x." {
		t.Errorf("unknown token = %q, want pass-through", got)
	}
}

func TestSubstitute_TrailingDollarIsLiteral(t *testing.T) {
	got := Substitute("price is $", Subject{}, Subject{})
	if got != "price is $" {
		t.Errorf("trailing $ = %q", got)
	}
}

func TestSubstitute_Empty(t *testing.T) {
	if got := Substitute("", Subject{}, Subject{}); got != "" {
		t.Errorf("empty = %q", got)
	}
}

func TestSubstitute_ItPronouns(t *testing.T) {
	alice := Subject{Name: "Alice", Pronouns: DefaultPronouns}
	rock := Subject{Name: "a rock", Pronouns: ItPronouns}
	got := Substitute("$n nudges $N with $s foot.", alice, rock)
	if got != "Alice nudges a rock with their foot." {
		t.Errorf("it target = %q", got)
	}
}
