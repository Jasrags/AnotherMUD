package light

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/world"
)

func ptr(l Level) *Level { return &l }

// TestResolve covers the spec §2.2/§2.3 acceptance matrix as a table.
func TestResolve(t *testing.T) {
	cap := Dim // indoor ambient cap used throughout
	cases := []struct {
		name string
		in   Inputs
		want Level
	}{
		{
			name: "outdoors gets full ambient",
			in:   Inputs{Ambient: Lit, Terrain: world.TerrainOutdoors, IndoorCap: cap},
			want: Lit,
		},
		{
			name: "outdoors at darkest period resolves to gloom, not black",
			in:   Inputs{Ambient: Gloom, Terrain: world.TerrainOutdoors, IndoorCap: cap},
			want: Gloom,
		},
		{
			name: "empty terrain behaves as outdoors",
			in:   Inputs{Ambient: Lit, Terrain: "", IndoorCap: cap},
			want: Lit,
		},
		{
			name: "unknown terrain behaves as outdoors (sky-eligible)",
			in:   Inputs{Ambient: Lit, Terrain: "swamp", IndoorCap: cap},
			want: Lit,
		},
		{
			name: "underground is black at noon with no source/override",
			in:   Inputs{Ambient: Lit, Terrain: world.TerrainUnderground, IndoorCap: cap},
			want: Black,
		},
		{
			name: "underground is black at night too",
			in:   Inputs{Ambient: Gloom, Terrain: world.TerrainUnderground, IndoorCap: cap},
			want: Black,
		},
		{
			name: "indoors capped below full daylight",
			in:   Inputs{Ambient: Lit, Terrain: world.TerrainIndoors, IndoorCap: cap},
			want: Dim,
		},
		{
			name: "indoors below the cap passes ambient through",
			in:   Inputs{Ambient: Gloom, Terrain: world.TerrainIndoors, IndoorCap: cap},
			want: Gloom,
		},
		{
			name: "override floors a dark night (lamp-lit street pinned dim)",
			in:   Inputs{Ambient: Gloom, Terrain: world.TerrainOutdoors, IndoorCap: cap, Override: ptr(Dim)},
			want: Dim,
		},
		{
			name: "override ceilings ambient (black-pinned vault defeats daylight)",
			in:   Inputs{Ambient: Lit, Terrain: world.TerrainOutdoors, IndoorCap: cap, Override: ptr(Black)},
			want: Black,
		},
		{
			name: "override is not gated by terrain (pins value underground)",
			in:   Inputs{Ambient: Lit, Terrain: world.TerrainUnderground, IndoorCap: cap, Override: ptr(Lit)},
			want: Lit,
		},
		{
			name: "carried source lights an underground black room",
			in:   Inputs{Ambient: Lit, Terrain: world.TerrainUnderground, IndoorCap: cap, Sources: Dim},
			want: Dim,
		},
		{
			name: "source beats a black-pinned vault (torch in the vault)",
			in:   Inputs{Ambient: Lit, Terrain: world.TerrainOutdoors, IndoorCap: cap, Override: ptr(Black), Sources: Gloom},
			want: Gloom,
		},
		{
			name: "viewer floor lifts a black underground room (darkvision)",
			in:   Inputs{Ambient: Lit, Terrain: world.TerrainUnderground, IndoorCap: cap, ViewerFloor: Gloom},
			want: Gloom,
		},
		{
			name: "viewer floor below ambient does not lower it",
			in:   Inputs{Ambient: Lit, Terrain: world.TerrainOutdoors, IndoorCap: cap, ViewerFloor: Gloom},
			want: Lit,
		},
		{
			name: "max combine: brightest contributor wins",
			in:   Inputs{Ambient: Gloom, Terrain: world.TerrainOutdoors, IndoorCap: cap, Sources: Dim, ViewerFloor: Gloom},
			want: Dim,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Resolve(tc.in); got != tc.want {
				t.Fatalf("Resolve(%+v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestResolve_AlwaysInRange(t *testing.T) {
	// An override above Lit must clamp into range.
	got := Resolve(Inputs{Ambient: Lit, Terrain: world.TerrainOutdoors, Override: ptr(Level(50))})
	if got != Lit {
		t.Fatalf("Resolve with over-bright override = %v, want Lit", got)
	}
}
