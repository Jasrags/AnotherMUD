package combat

import (
	"strings"
	"testing"
)

// fixedRoller returns a programmed sequence of IntN results. Each Call
// consumes one entry; running off the end fails the test loudly because
// the caller didn't bound its rolls.
type fixedRoller struct {
	t      *testing.T
	values []int
	idx    int
}

func (f *fixedRoller) IntN(n int) int {
	if f.idx >= len(f.values) {
		f.t.Fatalf("fixedRoller: out of values after %d rolls", f.idx)
	}
	v := f.values[f.idx]
	f.idx++
	if v < 0 || v >= n {
		f.t.Fatalf("fixedRoller: programmed value %d out of range [0,%d)", v, n)
	}
	return v
}

func TestParseDiceValid(t *testing.T) {
	cases := []struct {
		in   string
		want DiceExpr
	}{
		{"1d6", DiceExpr{1, 6, 0}},
		{"3d8", DiceExpr{3, 8, 0}},
		{"2d4+1", DiceExpr{2, 4, 1}},
		{"1d20-3", DiceExpr{1, 20, -3}},
		{"  1d6  ", DiceExpr{1, 6, 0}}, // surrounding whitespace tolerated
		{"99d1000+99", DiceExpr{99, 1000, 99}},
	}
	for _, c := range cases {
		got, err := ParseDice(c.in)
		if err != nil {
			t.Errorf("ParseDice(%q) returned error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseDice(%q) = %+v, want %+v", c.in, got, c.want)
		}
	}
}

func TestParseDiceInvalid(t *testing.T) {
	cases := []string{
		"",
		"d6",        // missing count
		"1d",        // missing sides
		"1d1",       // sides < 2
		"0d6",       // count < 1
		"100d6",     // count > maxDiceCount
		"1d1001",    // sides > maxDiceSides
		"1 d 6",    // whitespace inside expression
		"1d6 + 1",  // whitespace around modifier
		"1d6+",      // dangling sign
		"abc",
	}
	for _, c := range cases {
		_, err := ParseDice(c)
		if err == nil {
			t.Errorf("ParseDice(%q) = nil error, want failure", c)
		}
	}
}

func TestDiceExprString(t *testing.T) {
	cases := []struct {
		in   DiceExpr
		want string
	}{
		{DiceExpr{1, 6, 0}, "1d6"},
		{DiceExpr{2, 8, 3}, "2d8+3"},
		{DiceExpr{1, 20, -2}, "1d20-2"},
	}
	for _, c := range cases {
		if got := c.in.String(); got != c.want {
			t.Errorf("(%+v).String() = %q, want %q", c.in, got, c.want)
		}
		// Round-trip
		parsed, err := ParseDice(c.want)
		if err != nil {
			t.Errorf("ParseDice(%q) round-trip error: %v", c.want, err)
		}
		if parsed != c.in {
			t.Errorf("ParseDice(%q) round-trip = %+v, want %+v", c.want, parsed, c.in)
		}
	}
}

func TestDiceRoll(t *testing.T) {
	// 3d6+2 with all rolls landing on the max face (5 → +1 = 6 each)
	d, _ := ParseDice("3d6+2")
	r := &fixedRoller{t: t, values: []int{5, 5, 5}}
	got := d.Roll(r)
	if want := 3*6 + 2; got != want {
		t.Errorf("3d6+2 max roll = %d, want %d", got, want)
	}

	// 2d4-1 with all minimums (0 → +1 = 1 each)
	d2, _ := ParseDice("2d4-1")
	r2 := &fixedRoller{t: t, values: []int{0, 0}}
	got2 := d2.Roll(r2)
	if want := 2*1 - 1; got2 != want {
		t.Errorf("2d4-1 min roll = %d, want %d", got2, want)
	}
}

func TestDiceRollZeroExprReturnsZero(t *testing.T) {
	var zero DiceExpr
	if got := zero.Roll(&fixedRoller{t: t}); got != 0 {
		t.Errorf("zero DiceExpr Roll = %d, want 0", got)
	}
}

func TestIsZero(t *testing.T) {
	if !(DiceExpr{}).IsZero() {
		t.Error("zero value should report IsZero true")
	}
	if (DiceExpr{Count: 1, Sides: 6}).IsZero() {
		t.Error("1d6 should not report IsZero")
	}
}

func TestSTRBonus(t *testing.T) {
	cases := []struct {
		str, want int
	}{
		{10, 0},
		{11, 0},
		{12, 1},
		{14, 2},
		{8, -1}, // (8-10)/2 = -1 (Go truncates toward zero: -2/2 = -1)
		{6, -2},
	}
	for _, c := range cases {
		if got := STRBonus(c.str); got != c.want {
			t.Errorf("STRBonus(%d) = %d, want %d", c.str, got, c.want)
		}
	}
}

func TestDefaultUnarmedDamageIsNonZero(t *testing.T) {
	d := DefaultUnarmedDamage()
	if d.IsZero() {
		t.Error("DefaultUnarmedDamage must not return the zero DiceExpr")
	}
	if !strings.HasPrefix(d.String(), "1d") {
		t.Errorf("DefaultUnarmedDamage = %q, expected 1d<sides>", d.String())
	}
}

// TestDiceExprAverage pins the M14.3 mob class-growth math:
// integer averaging for NdM±K is (N*(M+1))/2 + K.
func TestDiceExprAverage(t *testing.T) {
	cases := []struct {
		expr string
		want int
	}{
		{"1d6", 3},      // (1*(6+1))/2 = 3
		{"2d6", 7},      // (2*(6+1))/2 = 7
		{"1d10", 5},     // (1*(10+1))/2 = 5
		{"1d10+2", 7},   // 5 + 2
		{"3d6+1", 11},   // 3*7/2 + 1 = 10 + 1 = 11
		{"1d2", 1},      // (1*3)/2 = 1 (integer truncation)
		{"1d4-1", 1},    // (1*5)/2 - 1 = 2 - 1 = 1
	}
	for _, c := range cases {
		d, err := ParseDice(c.expr)
		if err != nil {
			t.Fatalf("ParseDice %q: %v", c.expr, err)
		}
		if got := d.Average(); got != c.want {
			t.Errorf("Average(%q) = %d, want %d", c.expr, got, c.want)
		}
	}
}

func TestDiceExprAverageZeroExpr(t *testing.T) {
	var d DiceExpr
	if got := d.Average(); got != 0 {
		t.Errorf("Average(zero) = %d, want 0", got)
	}
}
