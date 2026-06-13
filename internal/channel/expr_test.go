package channel

import (
	"errors"
	"strings"
	"testing"
)

// statLookup builds a lookup func from a map; unknown names read 0.
func statLookup(stats map[string]int) func(string) int {
	return func(name string) int { return stats[name] }
}

func TestExpr_Eval(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		stats map[string]int
		want  int
	}{
		// literals & arithmetic
		{"int literal", "7", nil, 7},
		{"addition", "2 + 3", nil, 5},
		{"precedence", "2 + 3 * 4", nil, 14},
		{"parens override precedence", "(2 + 3) * 4", nil, 20},
		{"subtraction", "10 - 4 - 1", nil, 5},
		{"unary minus", "-5 + 2", nil, -3},
		{"unary minus on paren", "-(3 + 2)", nil, -5},

		// variables (attribute lookup); unknown → 0
		{"bare attribute", "str", map[string]int{"str": 16}, 16},
		{"unknown attribute is zero", "ghost", nil, 0},
		{"case-insensitive attribute", "Reaction", map[string]int{"reaction": 6}, 6},

		// functions
		{"mod 10 is 0", "mod(10)", nil, 0},
		{"mod 14 is 2", "mod(14)", nil, 2},
		{"mod 8 is -1", "mod(8)", nil, -1}, // floor((8-10)/2) = floor(-1) = -1
		{"mod 15 is 2", "mod(15)", nil, 2}, // floor(2.5) = 2
		{"floor", "floor(3.9)", nil, 3},
		{"ceil", "ceil(3.1)", nil, 4},
		{"abs", "abs(0 - 7)", nil, 7},
		{"min variadic", "min(5, 2, 9)", nil, 2},
		{"max variadic", "max(5, 2, 9)", nil, 9},

		// division: float internally, rounded at the end
		{"division rounds half away", "5 / 2", nil, 3}, // 2.5 → 3
		{"ceil of division", "ceil(body / 2)", map[string]int{"body": 7}, 4},
		{"division by zero is zero", "5 / 0", nil, 0},

		// --- design §4.1 WoT mapping formulas ---
		{"wot defense", "10 + mod(dex) + armor", map[string]int{"dex": 14, "armor": 2}, 14},
		{"wot damage_bonus", "mod(str)", map[string]int{"str": 16}, 3},

		// --- design §4.2 Shadowrun mapping formulas ---
		{"sr defense", "reaction + intuition", map[string]int{"reaction": 6, "intuition": 5}, 11},
		{"sr hp_max body 6", "8 + ceil(body / 2)", map[string]int{"body": 6}, 11},
		{"sr hp_max body 7", "8 + ceil(body / 2)", map[string]int{"body": 7}, 12},
		{"sr stun_max", "8 + ceil(willpower / 2)", map[string]int{"willpower": 6}, 11},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := Parse(tt.src)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.src, err)
			}
			if got := e.Eval(statLookup(tt.stats)); got != tt.want {
				t.Fatalf("Eval(%q) = %d; want %d", tt.src, got, tt.want)
			}
		})
	}
}

func TestParse_Errors(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"empty", ""},
		{"whitespace only", "   "},
		{"unknown function", "foo(3)"},
		{"missing close paren", "(2 + 3"},
		{"trailing garbage", "2 + 3 4"},
		{"bad operator position", "* 3"},
		{"illegal character", "body & 2"},
		{"wrong arity", "mod(1, 2)"},
		{"variadic needs one arg", "min()"},
		{"dangling operator", "2 +"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Parse(tt.src); err == nil {
				t.Fatalf("Parse(%q) = nil error; want error", tt.src)
			}
		})
	}
}

func TestParse_EmptyIsSentinel(t *testing.T) {
	if _, err := Parse("  "); !errors.Is(err, ErrEmptyFormula) {
		t.Fatalf("Parse(blank) error = %v; want ErrEmptyFormula", err)
	}
}

func TestParse_LengthCap(t *testing.T) {
	long := "1" + strings.Repeat(" + 1", MaxFormulaLen) // well over the cap
	if _, err := Parse(long); err == nil {
		t.Fatal("over-length formula should error")
	}
}

func TestExpr_EvalNilLookup(t *testing.T) {
	// A formula referencing attributes with a nil lookup reads them as 0.
	e := MustParse("10 + mod(dex)")
	if got := e.Eval(nil); got != 5 { // 10 + mod(0)=floor(-5)= -5 → 5
		t.Fatalf("Eval(nil) = %d; want 5", got)
	}
}

func TestMustParse_PanicsOnBad(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("MustParse should panic on a malformed formula")
		}
	}()
	MustParse("foo(")
}
