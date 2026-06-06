package light

import "testing"

func TestEffectFloorFor(t *testing.T) {
	c := DefaultConfig()
	// No EffectFloors configured → always Black.
	if got := c.EffectFloorFor([]string{"infravision"}); got != Black {
		t.Fatalf("EffectFloorFor with empty map = %v, want Black", got)
	}

	c.EffectFloors = map[string]Level{"infravision": Gloom, "cast_light": Dim}
	cases := []struct {
		name  string
		flags []string
		want  Level
	}{
		{"no flags", nil, Black},
		{"unmatched flag", []string{"haste"}, Black},
		{"single match", []string{"infravision"}, Gloom},
		{"brightest of several", []string{"infravision", "cast_light"}, Dim},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := c.EffectFloorFor(tc.flags); got != tc.want {
				t.Fatalf("EffectFloorFor(%v) = %v, want %v", tc.flags, got, tc.want)
			}
		})
	}
}

func TestViewerFloor_DarkvisionOnly(t *testing.T) {
	c := DefaultConfig()
	if got := c.ViewerFloor(false, nil); got != Black {
		t.Fatalf("no darkvision, no effect = %v, want Black", got)
	}
	if got := c.ViewerFloor(true, nil); got != Gloom {
		t.Fatalf("darkvision floor = %v, want Gloom", got)
	}
}

func TestViewerFloor_EffectBeatsDarkvision(t *testing.T) {
	c := DefaultConfig()
	c.EffectFloors = map[string]Level{"cast_light": Dim}
	// A cast-light effect lifts a darkvision viewer above the gloom cap.
	if got := c.ViewerFloor(true, []string{"cast_light"}); got != Dim {
		t.Fatalf("darkvision + cast_light = %v, want Dim", got)
	}
	// And lifts a non-darkvision viewer too.
	if got := c.ViewerFloor(false, []string{"cast_light"}); got != Dim {
		t.Fatalf("no darkvision + cast_light = %v, want Dim", got)
	}
}

func TestViewerFloor_DarkvisionBeatsWeakerEffect(t *testing.T) {
	c := DefaultConfig()
	c.EffectFloors = map[string]Level{"dim_sense": Black} // weaker than darkvision
	if got := c.ViewerFloor(true, []string{"dim_sense"}); got != Gloom {
		t.Fatalf("darkvision vs weaker effect = %v, want Gloom", got)
	}
}

func TestDarkvisionFlagConstant(t *testing.T) {
	if DarkvisionFlag != "darkvision" {
		t.Fatalf("DarkvisionFlag = %q, want darkvision", DarkvisionFlag)
	}
}
