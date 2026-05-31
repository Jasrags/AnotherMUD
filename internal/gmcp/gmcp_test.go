package gmcp_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/gmcp"
)

func TestCharVitals_RequiredFieldsAlwaysEmit(t *testing.T) {
	// hp + maxhp emit even at zero — "hp 0" is meaningful (dead)
	// and a client panel that interprets a missing field as "no
	// change" must see the zero. omitempty would hide that.
	out, err := json.Marshal(gmcp.CharVitals{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(out)
	if !strings.Contains(got, `"hp":0`) || !strings.Contains(got, `"maxhp":0`) {
		t.Errorf("zero hp/maxhp must emit explicitly, got %q", got)
	}
}

func TestCharVitals_OptionalFieldsOmitWhenZero(t *testing.T) {
	// mp/maxmp/mv/maxmv/sustenance use omitempty so an engine
	// without those systems emits a minimal payload.
	out, _ := json.Marshal(gmcp.CharVitals{HP: 50, MaxHP: 75})
	got := string(out)
	for _, key := range []string{"mp", "maxmp", "mv", "maxmv", "sustenance"} {
		if strings.Contains(got, `"`+key+`"`) {
			t.Errorf("optional field %q should not emit at zero, got %q", key, got)
		}
	}
	if got != `{"hp":50,"maxhp":75}` {
		t.Errorf("minimal payload = %q", got)
	}
}

func TestCharVitals_AllFieldsEmitWhenSet(t *testing.T) {
	out, _ := json.Marshal(gmcp.CharVitals{
		HP: 50, MaxHP: 100,
		MP: 30, MaxMP: 60,
		MV: 70, MaxMV: 80,
		Sustenance: 90,
	})
	// Order is struct-field order; the keys are lowercase short
	// forms (Tapestry-compatible per PD-2).
	want := `{"hp":50,"maxhp":100,"mp":30,"maxmp":60,"mv":70,"maxmv":80,"sustenance":90}`
	if string(out) != want {
		t.Errorf("full payload = %q, want %q", string(out), want)
	}
}

func TestCharVitals_PackageNameConstant(t *testing.T) {
	if gmcp.PackageCharVitals != "Char.Vitals" {
		t.Errorf("PackageCharVitals = %q, want Char.Vitals", gmcp.PackageCharVitals)
	}
}

func TestRoomInfo_RequiredFieldsAlwaysEmit(t *testing.T) {
	// num / name / exits always emit so a mapper panel can always
	// build a node, even for rooms with no exits.
	out, _ := json.Marshal(gmcp.RoomInfo{
		Num:   "tapestry-core:square",
		Name:  "Town Square",
		Exits: map[string]string{},
	})
	got := string(out)
	for _, key := range []string{`"num"`, `"name"`, `"exits"`} {
		if !strings.Contains(got, key) {
			t.Errorf("required field %s missing in %q", key, got)
		}
	}
}

func TestRoomInfo_OptionalFieldsOmitWhenZero(t *testing.T) {
	out, _ := json.Marshal(gmcp.RoomInfo{
		Num:   "x",
		Name:  "y",
		Exits: map[string]string{},
	})
	got := string(out)
	for _, key := range []string{"area", "keywords", "terrain", "details"} {
		if strings.Contains(got, `"`+key+`"`) {
			t.Errorf("optional %q should omit, got %q", key, got)
		}
	}
}

func TestRoomInfo_PackageConstant(t *testing.T) {
	if gmcp.PackageRoomInfo != "Room.Info" {
		t.Errorf("PackageRoomInfo = %q, want Room.Info", gmcp.PackageRoomInfo)
	}
}
