package property

import (
	"strings"
	"testing"
)

func entry(name string, t ValueType) Entry {
	return Entry{Name: name, Type: t, Description: name}
}

func TestRegistry_RegisterEngineAndGet(t *testing.T) {
	r := NewRegistry()
	if err := r.RegisterEngine(entry("weight", TypeInt)); err != nil {
		t.Fatalf("RegisterEngine: %v", err)
	}
	got, ok := r.Get("weight", "")
	if !ok || got.Name != "weight" || got.Type != TypeInt {
		t.Errorf("Get(weight) = %+v ok=%v", got, ok)
	}
	if r.Len() != 1 {
		t.Errorf("Len = %d, want 1", r.Len())
	}
}

func TestRegistry_RejectsDuplicateEngine(t *testing.T) {
	r := NewRegistry()
	_ = r.RegisterEngine(entry("weight", TypeInt))
	err := r.RegisterEngine(entry("weight", TypeString))
	if err == nil || !strings.Contains(err.Error(), "duplicate key") {
		t.Errorf("dup engine: %v", err)
	}
}

func TestRegistry_RejectsHyphenName(t *testing.T) {
	r := NewRegistry()
	err := r.RegisterEngine(entry("quest-grant", TypeString))
	if err == nil || !strings.Contains(err.Error(), "snake_case") {
		t.Errorf("hyphen: %v", err)
	}
}

func TestRegistry_RejectsLeadingDigit(t *testing.T) {
	r := NewRegistry()
	err := r.RegisterEngine(entry("1st_thing", TypeInt))
	if err == nil || !strings.Contains(err.Error(), "snake_case") {
		t.Errorf("leading digit: %v", err)
	}
}

func TestRegistry_AcceptsValidSnakeCase(t *testing.T) {
	r := NewRegistry()
	for _, name := range []string{"a", "a_b", "level", "hp_max", "stat_growth_1", "x_y_z"} {
		if err := r.RegisterEngine(entry(name, TypeInt)); err != nil {
			t.Errorf("valid %q: %v", name, err)
		}
	}
}

func TestRegistry_RejectsInvalidValueType(t *testing.T) {
	r := NewRegistry()
	err := r.RegisterEngine(Entry{Name: "x", Type: ValueType(99)})
	if err == nil || !strings.Contains(err.Error(), "invalid Type") {
		t.Errorf("bad type: %v", err)
	}
}

func TestRegistry_PackPropertyKeysWithPrefix(t *testing.T) {
	r := NewRegistry()
	if err := r.RegisterPack("mypack", entry("trade", TypeString)); err != nil {
		t.Fatalf("RegisterPack: %v", err)
	}
	if got, ok := r.Get("mypack:trade", ""); !ok || got.Pack != "mypack" {
		t.Errorf("Get(mypack:trade) = %+v ok=%v", got, ok)
	}
}

func TestRegistry_PackShadowsEngineIsError(t *testing.T) {
	r := NewRegistry()
	_ = r.RegisterEngine(entry("weight", TypeInt))
	err := r.RegisterPack("mypack", entry("weight", TypeInt))
	if err == nil || !strings.Contains(err.Error(), "shadows engine") {
		t.Errorf("pack-shadows-engine: %v", err)
	}
}

func TestRegistry_PackPackCollisionIsError(t *testing.T) {
	r := NewRegistry()
	_ = r.RegisterPack("mypack", entry("trade", TypeString))
	err := r.RegisterPack("mypack", entry("trade", TypeString))
	if err == nil || !strings.Contains(err.Error(), "duplicate key") {
		t.Errorf("dup pack key: %v", err)
	}
}

func TestRegistry_TwoPacksSameNameOK(t *testing.T) {
	r := NewRegistry()
	if err := r.RegisterPack("p1", entry("trade", TypeString)); err != nil {
		t.Fatalf("p1: %v", err)
	}
	if err := r.RegisterPack("p2", entry("trade", TypeInt)); err != nil {
		t.Fatalf("p2: %v", err)
	}
	if _, ok := r.Get("p1:trade", ""); !ok {
		t.Error("p1:trade missing")
	}
	if _, ok := r.Get("p2:trade", ""); !ok {
		t.Error("p2:trade missing")
	}
}

func TestRegistry_GetShorthandResolvesAgainstCurrentPack(t *testing.T) {
	r := NewRegistry()
	_ = r.RegisterPack("mypack", entry("trade", TypeString))

	if got, ok := r.Get("trade", "mypack"); !ok || got.Pack != "mypack" {
		t.Errorf("shorthand resolve: %+v ok=%v", got, ok)
	}
}

func TestRegistry_GetShorthandFallsThroughDependencies(t *testing.T) {
	r := NewRegistry()
	_ = r.RegisterPack("base", entry("foo", TypeString))
	_ = r.RegisterPack("addon", entry("bar", TypeString))

	// `addon` depends on `base`. A lookup of "foo" in addon's
	// context should walk addon's deps and find base:foo.
	r.SetDependencyResolver(func(pack string) []string {
		if pack == "addon" {
			return []string{"base"}
		}
		return nil
	})

	if got, ok := r.Get("foo", "addon"); !ok || got.Key() != "base:foo" {
		t.Errorf("dep walk: %+v ok=%v", got, ok)
	}
}

func TestRegistry_GetEngineNameWinsOverPackShorthand(t *testing.T) {
	r := NewRegistry()
	// Engine name "weight"; lookup in pack context still hits the
	// engine entry (direct match wins per §2.4 step 1).
	_ = r.RegisterEngine(entry("weight", TypeInt))
	if got, ok := r.Get("weight", "mypack"); !ok || got.Pack != "" {
		t.Errorf("engine direct match: %+v ok=%v", got, ok)
	}
}

func TestRegistry_GetMissReturnsZero(t *testing.T) {
	r := NewRegistry()
	if _, ok := r.Get("nothing", ""); ok {
		t.Error("Get unknown: want miss")
	}
	if _, ok := r.Get("nothing", "mypack"); ok {
		t.Error("Get unknown with pack: want miss")
	}
}

func TestRegistry_IsTransient(t *testing.T) {
	r := NewRegistry()
	_ = r.RegisterEngine(Entry{Name: "alive", Type: TypeBool, Transient: true})
	_ = r.RegisterEngine(Entry{Name: "weight", Type: TypeInt})

	if !r.IsTransient("alive", "") {
		t.Error("alive should be transient")
	}
	if r.IsTransient("weight", "") {
		t.Error("weight should not be transient")
	}
	if r.IsTransient("unknown", "") {
		t.Error("unknown should not be transient (skip via envelope path instead)")
	}
}

func TestRegistry_ValueType(t *testing.T) {
	r := NewRegistry()
	_ = r.RegisterEngine(entry("weight", TypeInt))
	if vt, ok := r.ValueType("weight", ""); !ok || vt != TypeInt {
		t.Errorf("ValueType(weight) = %v ok=%v", vt, ok)
	}
	if _, ok := r.ValueType("nope", ""); ok {
		t.Error("ValueType unknown: want miss")
	}
}

func TestRegistry_RegisterEngineRejectsNonEmptyPack(t *testing.T) {
	r := NewRegistry()
	err := r.RegisterEngine(Entry{Name: "x", Pack: "set", Type: TypeInt})
	if err == nil || !strings.Contains(err.Error(), "Pack must be empty") {
		t.Errorf("engine with Pack: %v", err)
	}
}

func TestRegistry_RegisterPackRejectsEmptyPackName(t *testing.T) {
	r := NewRegistry()
	err := r.RegisterPack("", entry("x", TypeInt))
	if err == nil || !strings.Contains(err.Error(), "empty packName") {
		t.Errorf("empty pack name: %v", err)
	}
}

func TestValueType_StringRoundsTrips(t *testing.T) {
	for vt := TypeString; vt <= TypeListString; vt++ {
		if vt.String() == "unknown" {
			t.Errorf("ValueType(%d) renders as unknown", vt)
		}
	}
	if ValueType(99).String() != "unknown" {
		t.Error("out-of-range type should render as unknown")
	}
}

func TestEntry_KeyShape(t *testing.T) {
	if k := (Entry{Name: "weight"}).Key(); k != "weight" {
		t.Errorf("engine key = %q, want weight", k)
	}
	if k := (Entry{Name: "trade", Pack: "mypack"}).Key(); k != "mypack:trade" {
		t.Errorf("pack key = %q, want mypack:trade", k)
	}
}

func TestRegistry_All(t *testing.T) {
	r := NewRegistry()
	_ = r.RegisterEngine(entry("weight", TypeInt))
	_ = r.RegisterEngine(entry("alive", TypeBool))
	got := r.All()
	if len(got) != 2 {
		t.Errorf("All len = %d, want 2", len(got))
	}
}

func TestRegistry_RejectsEmptyName(t *testing.T) {
	r := NewRegistry()
	if err := r.RegisterEngine(Entry{Type: TypeInt}); err == nil {
		t.Error("empty name: want error")
	}
}

// TestRegistry_GetQualifiedNameInPackContext: a fully-qualified
// lookup (pack:name) doesn't double-prefix when a currentPack is
// also supplied — the contains-colon guard takes the direct path.
func TestRegistry_GetQualifiedNameInPackContext(t *testing.T) {
	r := NewRegistry()
	_ = r.RegisterPack("p1", entry("trade", TypeString))
	if _, ok := r.Get("p1:trade", "addon"); !ok {
		t.Error("qualified lookup with currentPack context should still hit")
	}
}
