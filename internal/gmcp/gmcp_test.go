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
	for _, key := range []string{"mp", "maxmp", "mv", "maxmv", "sustenance", "pools"} {
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

func TestCharVitals_GeneralizedPoolsEmit(t *testing.T) {
	// The generalized `pools` map carries any pool by engine kind — including
	// kinds with no fixed slot (the Shadowrun Essence budget); each entry
	// serializes as {cur,max}.
	out, _ := json.Marshal(gmcp.CharVitals{
		HP: 20, MaxHP: 20,
		Pools: map[string]gmcp.PoolVital{
			"essence": {Cur: 55, Max: 60},
		},
	})
	want := `{"hp":20,"maxhp":20,"pools":{"essence":{"cur":55,"max":60}}}`
	if string(out) != want {
		t.Errorf("pools payload = %q, want %q", string(out), want)
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

func TestCharItemsList_EmptyItemsSliceEmitsAsArray(t *testing.T) {
	// Callers MUST initialize Items as an empty slice (not nil)
	// so JSON marshals as `[]`, not `null`. The session flusher
	// upholds this via entityIDsToCharItems which uses make().
	// This test pins the contract from the encoder side.
	out, _ := json.Marshal(gmcp.CharItemsList{
		Location: gmcp.LocationInventory,
		Items:    []gmcp.CharItem{}, // explicit empty, NOT nil
	})
	got := string(out)
	if !strings.Contains(got, `"items":[]`) {
		t.Errorf("empty (non-nil) Items must emit as [], got %q", got)
	}
}

func TestCharItemsList_FullPayload(t *testing.T) {
	out, _ := json.Marshal(gmcp.CharItemsList{
		Location: gmcp.LocationWear,
		Items: []gmcp.CharItem{
			{ID: "item-1", Name: "a leather cap"},
			{ID: "item-2", Name: "a short sword"},
		},
	})
	got := string(out)
	want := `{"location":"wear","items":[{"id":"item-1","name":"a leather cap"},{"id":"item-2","name":"a short sword"}]}`
	if got != want {
		t.Errorf("payload = %q, want %q", got, want)
	}
}

func TestCharItems_LocationConstants(t *testing.T) {
	if gmcp.LocationInventory != "inv" {
		t.Errorf("LocationInventory = %q, want inv", gmcp.LocationInventory)
	}
	if gmcp.LocationWear != "wear" {
		t.Errorf("LocationWear = %q, want wear", gmcp.LocationWear)
	}
	if gmcp.PackageCharItemsList != "Char.Items.List" {
		t.Errorf("PackageCharItemsList = %q", gmcp.PackageCharItemsList)
	}
}

func TestCharInventory_PackageConstant(t *testing.T) {
	if gmcp.PackageCharInventory != "Char.Inventory" {
		t.Errorf("PackageCharInventory = %q, want Char.Inventory", gmcp.PackageCharInventory)
	}
}

func TestCharInventory_PayloadShape(t *testing.T) {
	// A carried stacked item with a drop action, a carried clip with an ammo
	// detail + reload action, an occupied worn slot with a detail, and an empty
	// worn slot (id/name/detail/actions all omitted).
	out, _ := json.Marshal(gmcp.CharInventory{
		Carried: []gmcp.InventoryItem{
			{ID: "item:2", Name: "a crossbow bolt", Qty: 12, Actions: []gmcp.InvAction{{Label: "drop", Cmd: "drop bolt"}}},
			{ID: "item:4", Name: "an Ares Predator V clip", Detail: "15/15 APDS", Actions: []gmcp.InvAction{{Label: "reload", Cmd: "reload clip"}, {Label: "drop", Cmd: "drop clip"}}},
		},
		Worn: []gmcp.WornItem{
			{Slot: "body", ID: "item:3", Name: "an armored vest", Detail: "Armor 4", Actions: []gmcp.InvAction{{Label: "unequip", Cmd: "unequip vest"}}},
			{Slot: "head", Empty: true},
		},
	})
	want := `{"carried":[` +
		`{"id":"item:2","name":"a crossbow bolt","qty":12,"actions":[{"label":"drop","cmd":"drop bolt"}]},` +
		`{"id":"item:4","name":"an Ares Predator V clip","detail":"15/15 APDS","actions":[{"label":"reload","cmd":"reload clip"},{"label":"drop","cmd":"drop clip"}]}` +
		`],"worn":[` +
		`{"slot":"body","id":"item:3","name":"an armored vest","detail":"Armor 4","actions":[{"label":"unequip","cmd":"unequip vest"}]},` +
		`{"slot":"head","empty":true}` +
		`]}`
	if string(out) != want {
		t.Errorf("payload = %q,\nwant       %q", string(out), want)
	}
}

func TestCharInventory_EmptySlicesMarshalAsArrays(t *testing.T) {
	// Non-nil empty slices must serialize as [] (not null) so a client reading
	// "carried is empty" isn't ambiguous with "no change".
	out, _ := json.Marshal(gmcp.CharInventory{
		Carried: []gmcp.InventoryItem{},
		Worn:    []gmcp.WornItem{},
	})
	if string(out) != `{"carried":[],"worn":[]}` {
		t.Errorf("empty payload = %q, want {\"carried\":[],\"worn\":[]}", string(out))
	}
}

func TestCharRecipes_PackageConstant(t *testing.T) {
	if gmcp.PackageCharRecipes != "Char.Recipes" {
		t.Errorf("PackageCharRecipes = %q, want Char.Recipes", gmcp.PackageCharRecipes)
	}
}

func TestCharRecipes_PayloadShape(t *testing.T) {
	// A craftable recipe (all gates met, no block reason, station omitted at 0)
	// and a blocked one (missing ingredient, station tier surfaced, no cmd sent
	// by the client but still carried).
	out, _ := json.Marshal(gmcp.CharRecipes{
		Recipes: []gmcp.CraftRecipe{
			{
				ID: "starter-world:campfire-stew", Name: "campfire stew", Discipline: "cooking",
				Ingredients: []gmcp.RecipeIngredient{{Name: "a hunk of meat", Need: 1, Have: 2}},
				StationMet:  true, SkillMet: true, Craftable: true,
				Cmd: "craft campfire-stew",
			},
			{
				ID: "starter-world:iron-sword", Name: "an iron sword", Discipline: "smithing",
				Ingredients: []gmcp.RecipeIngredient{{Name: "an iron bar", Need: 2, Have: 0}},
				Station:     2, StationMet: false, SkillMet: true, Craftable: false,
				Blocked: "missing ingredients", Cmd: "craft iron-sword",
			},
		},
	})
	want := `{"recipes":[` +
		`{"id":"starter-world:campfire-stew","name":"campfire stew","discipline":"cooking",` +
		`"ingredients":[{"name":"a hunk of meat","need":1,"have":2}],"stationMet":true,"skillMet":true,"craftable":true,"cmd":"craft campfire-stew"},` +
		`{"id":"starter-world:iron-sword","name":"an iron sword","discipline":"smithing",` +
		`"ingredients":[{"name":"an iron bar","need":2,"have":0}],"station":2,"stationMet":false,"skillMet":true,"craftable":false,"blocked":"missing ingredients","cmd":"craft iron-sword"}` +
		`]}`
	if string(out) != want {
		t.Errorf("payload = %q,\nwant       %q", string(out), want)
	}
}

func TestCharRecipes_EmptySliceMarshalsAsArray(t *testing.T) {
	out, _ := json.Marshal(gmcp.CharRecipes{Recipes: []gmcp.CraftRecipe{}})
	if string(out) != `{"recipes":[]}` {
		t.Errorf("empty payload = %q, want {\"recipes\":[]}", string(out))
	}
}

func TestCharCombat_NotInCombatOmitsTargetFields(t *testing.T) {
	// in_combat=false → just the flag; target_* fields all omit
	// so the panel can hide the target tile.
	out, _ := json.Marshal(gmcp.CharCombat{InCombat: false})
	got := string(out)
	if got != `{"in_combat":false}` {
		t.Errorf("not-in-combat payload = %q, want minimal flag-only", got)
	}
}

func TestCharCombat_FullPayloadShape(t *testing.T) {
	out, _ := json.Marshal(gmcp.CharCombat{
		InCombat:        true,
		Target:          "a village guard",
		TargetID:        "mob:e-1",
		TargetHP:        25,
		TargetMaxHP:     50,
		TargetHPPercent: 50,
	})
	want := `{"in_combat":true,"target":"a village guard","target_id":"mob:e-1","target_hp":25,"target_max_hp":50,"target_hp_percent":50}`
	if string(out) != want {
		t.Errorf("full payload = %q, want %q", string(out), want)
	}
}

func TestCharCombat_PackageConstant(t *testing.T) {
	if gmcp.PackageCharCombat != "Char.Combat" {
		t.Errorf("PackageCharCombat = %q, want Char.Combat", gmcp.PackageCharCombat)
	}
}

func TestCharEffectsList_EmptyEffectsEmitsAsArray(t *testing.T) {
	// Empty (non-nil) slice must marshal as `[]`, not `null`, so
	// the panel can distinguish "no active effects" from "no
	// change". The session flusher uses make() to uphold this.
	out, _ := json.Marshal(gmcp.CharEffectsList{Effects: []gmcp.CharEffect{}})
	got := string(out)
	if got != `{"effects":[]}` {
		t.Errorf("empty list = %q, want {\"effects\":[]}", got)
	}
}

func TestCharEffect_OptionalFieldsOmitWhenZero(t *testing.T) {
	// remaining=0 + permanent=false + empty flags + empty source
	// → only `id` emits. Lets a flag-only permanent-via-default
	// effect ship a minimal payload.
	out, _ := json.Marshal(gmcp.CharEffect{ID: "bless"})
	got := string(out)
	if got != `{"id":"bless"}` {
		t.Errorf("minimal effect = %q, want {\"id\":\"bless\"}", got)
	}
}

func TestCharEffect_PermanentEmitsFlagOmitsRemaining(t *testing.T) {
	// Permanent effects suppress remaining (0 is meaningless for
	// them) and set permanent=true so the panel renders the
	// infinity glyph.
	out, _ := json.Marshal(gmcp.CharEffect{ID: "blessed-by-the-light", Permanent: true})
	got := string(out)
	if got != `{"id":"blessed-by-the-light","permanent":true}` {
		t.Errorf("permanent effect = %q", got)
	}
}

func TestCharEffectsList_FullPayloadShape(t *testing.T) {
	out, _ := json.Marshal(gmcp.CharEffectsList{
		Effects: []gmcp.CharEffect{
			{ID: "bless", Remaining: 60, Flags: []string{"buff"}, Source: "ability:bless"},
			{ID: "poisoned", Remaining: 30, Flags: []string{"debuff", "poison"}},
		},
	})
	want := `{"effects":[{"id":"bless","remaining":60,"flags":["buff"],"source":"ability:bless"},{"id":"poisoned","remaining":30,"flags":["debuff","poison"]}]}`
	if string(out) != want {
		t.Errorf("full payload = %q, want %q", string(out), want)
	}
}

func TestCharEffects_PackageConstant(t *testing.T) {
	if gmcp.PackageCharEffects != "Char.Effects" {
		t.Errorf("PackageCharEffects = %q, want Char.Effects", gmcp.PackageCharEffects)
	}
}

func TestCharExperience_EmptyTracksEmitsAsArray(t *testing.T) {
	out, _ := json.Marshal(gmcp.CharExperience{Tracks: []gmcp.CharExperienceTrack{}})
	got := string(out)
	if got != `{"tracks":[]}` {
		t.Errorf("empty tracks = %q, want {\"tracks\":[]}", got)
	}
}

func TestCharExperienceTrack_MinimalShape(t *testing.T) {
	// track/level/xp/maxlevel always emit. name/xpnext/at_max/
	// overflow omit when zero/empty so a level-1, zero-XP, with-
	// cap-but-not-at-max snapshot stays minimal.
	out, _ := json.Marshal(gmcp.CharExperienceTrack{
		Track:    "adventurer",
		Level:    1,
		XP:       0,
		MaxLevel: 50,
	})
	got := string(out)
	want := `{"track":"adventurer","level":1,"xp":0,"maxlevel":50}`
	if got != want {
		t.Errorf("minimal track = %q, want %q", got, want)
	}
}

func TestCharExperienceTrack_MaxLevelEmitsFlag(t *testing.T) {
	// at_max=true + overflow emit; xpnext stays at 0 (omitted by
	// omitempty), so the panel reads "max level reached".
	out, _ := json.Marshal(gmcp.CharExperienceTrack{
		Track:    "adventurer",
		Level:    50,
		XP:       1000000,
		MaxLevel: 50,
		AtMax:    true,
		Overflow: 12345,
	})
	got := string(out)
	if !strings.Contains(got, `"at_max":true`) || !strings.Contains(got, `"overflow":12345`) {
		t.Errorf("at-max payload = %q", got)
	}
	if strings.Contains(got, `"xpnext"`) {
		t.Errorf("at-max payload should omit xpnext, got %q", got)
	}
}

func TestCharExperience_FullPayloadShape(t *testing.T) {
	out, _ := json.Marshal(gmcp.CharExperience{
		Tracks: []gmcp.CharExperienceTrack{
			{Track: "adventurer", Name: "Adventurer", Level: 12, XP: 8500, XPNext: 1500, MaxLevel: 50},
			{Track: "crafting", Level: 3, XP: 250, XPNext: 50, MaxLevel: 20},
		},
	})
	want := `{"tracks":[{"track":"adventurer","name":"Adventurer","level":12,"xp":8500,"xpnext":1500,"maxlevel":50},{"track":"crafting","level":3,"xp":250,"xpnext":50,"maxlevel":20}]}`
	if string(out) != want {
		t.Errorf("full payload = %q, want %q", string(out), want)
	}
}

func TestCharExperience_PackageConstant(t *testing.T) {
	if gmcp.PackageCharExperience != "Char.Experience" {
		t.Errorf("PackageCharExperience = %q, want Char.Experience", gmcp.PackageCharExperience)
	}
}

func TestCommChannelText_FullPayload(t *testing.T) {
	out, _ := json.Marshal(gmcp.CommChannelText{
		Channel: "ooc",
		Talker:  "Alice",
		Text:    "[ooc] Alice: hello",
	})
	want := `{"channel":"ooc","talker":"Alice","text":"[ooc] Alice: hello"}`
	if string(out) != want {
		t.Errorf("payload = %q, want %q", string(out), want)
	}
}

func TestCommChannelText_SystemMessageOmitsTalker(t *testing.T) {
	// System announcements (no speaker) drop the talker field so
	// the panel can render without an attribution prefix.
	out, _ := json.Marshal(gmcp.CommChannelText{
		Channel: "admin",
		Text:    "[admin] Server restart in 5 minutes.",
	})
	got := string(out)
	if strings.Contains(got, `"talker"`) {
		t.Errorf("system message should omit talker, got %q", got)
	}
}

func TestCommChannelText_RequiredFieldsAlwaysEmit(t *testing.T) {
	// channel + text always emit even when empty — an empty
	// channel id is malformed but the encoder must surface it so
	// callers see the bug rather than silently dropping the
	// payload.
	out, _ := json.Marshal(gmcp.CommChannelText{})
	got := string(out)
	for _, key := range []string{`"channel"`, `"text"`} {
		if !strings.Contains(got, key) {
			t.Errorf("required field %s missing in %q", key, got)
		}
	}
}

func TestCommChannel_PackageConstant(t *testing.T) {
	if gmcp.PackageCommChannelText != "Comm.Channel.Text" {
		t.Errorf("PackageCommChannelText = %q, want Comm.Channel.Text", gmcp.PackageCommChannelText)
	}
}

func TestCharLogin_AllFieldsAlwaysEmit(t *testing.T) {
	// name/fullname/account all always emit so a panel can read
	// any of them defensively without inheriting a stale value
	// from a prior login.
	out, _ := json.Marshal(gmcp.CharLogin{})
	got := string(out)
	for _, key := range []string{`"name"`, `"fullname"`, `"account"`} {
		if !strings.Contains(got, key) {
			t.Errorf("required field %s missing in %q", key, got)
		}
	}
}

func TestCharLogin_FullPayload(t *testing.T) {
	out, _ := json.Marshal(gmcp.CharLogin{
		Name:     "Alice",
		FullName: "Alice the Bold",
		Account:  "acc-7",
	})
	want := `{"name":"Alice","fullname":"Alice the Bold","account":"acc-7"}`
	if string(out) != want {
		t.Errorf("payload = %q, want %q", string(out), want)
	}
}

func TestCharStatusVars_EnvelopeShape(t *testing.T) {
	// Vars wrapped in a `vars` envelope (not bare top-level map)
	// so clients can discriminate from other Char.* packages.
	out, _ := json.Marshal(gmcp.CharStatusVars{
		Vars: map[string]string{"class": "Class"},
	})
	want := `{"vars":{"class":"Class"}}`
	if string(out) != want {
		t.Errorf("envelope = %q, want %q", string(out), want)
	}
}

func TestCharStatus_AlignmentZeroEmits(t *testing.T) {
	// alignment=0 is meaningful (neutral); must emit explicitly
	// so the panel can distinguish neutral from "missing".
	out, _ := json.Marshal(gmcp.CharStatus{})
	got := string(out)
	if !strings.Contains(got, `"alignment":0`) {
		t.Errorf("alignment=0 must emit, got %q", got)
	}
}

func TestCharStatus_OptionalFieldsOmitWhenEmpty(t *testing.T) {
	out, _ := json.Marshal(gmcp.CharStatus{Alignment: 100})
	got := string(out)
	for _, key := range []string{"race", "class", "alignment_tag"} {
		if strings.Contains(got, `"`+key+`"`) {
			t.Errorf("optional %q should omit when empty, got %q", key, got)
		}
	}
}

func TestCharStatus_FullPayload(t *testing.T) {
	out, _ := json.Marshal(gmcp.CharStatus{
		Race:         "human",
		Class:        "fighter",
		Alignment:    -50,
		AlignmentTag: "evil",
	})
	want := `{"race":"human","class":"fighter","alignment":-50,"alignment_tag":"evil"}`
	if string(out) != want {
		t.Errorf("payload = %q, want %q", string(out), want)
	}
}

func TestCharLoginStatus_PackageConstants(t *testing.T) {
	cases := []struct {
		got, want string
	}{
		{gmcp.PackageCharLogin, "Char.Login"},
		{gmcp.PackageCharStatusVars, "Char.StatusVars"},
		{gmcp.PackageCharStatus, "Char.Status"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("constant = %q, want %q", c.got, c.want)
		}
	}
}

func TestCharWizardStep_ChoiceCarriesOptions(t *testing.T) {
	out, err := json.Marshal(gmcp.CharWizardStep{
		Flow:   "creation",
		Step:   "race",
		Type:   "choice",
		Prompt: "Choose a race:",
		Options: []gmcp.WizardOption{
			{Label: "Human", Tag: "versatile"},
			{Label: "Dwarf"},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(out)
	for _, want := range []string{
		`"flow":"creation"`, `"step":"race"`, `"type":"choice"`,
		`"prompt":"Choose a race:"`, `"label":"Human"`, `"tag":"versatile"`, `"label":"Dwarf"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("payload missing %s, got %q", want, got)
		}
	}
	// A label-only option omits its tag.
	if strings.Contains(got, `"tag":""`) {
		t.Errorf("empty tag should be omitted, got %q", got)
	}
}

func TestCharWizardStep_NonChoiceOmitsOptionsAndSecret(t *testing.T) {
	// An info/text step with no options and no secret emits the four
	// required keys only — a panel that keys off "type" needs them
	// always, but options/secret stay absent.
	out, _ := json.Marshal(gmcp.CharWizardStep{
		Flow: "creation", Step: "intro", Type: "info", Prompt: "Welcome.",
	})
	got := string(out)
	if got != `{"flow":"creation","step":"intro","type":"info","prompt":"Welcome."}` {
		t.Errorf("minimal payload = %q", got)
	}
}

func TestCharWizardStep_SecretEmitsWhenSet(t *testing.T) {
	out, _ := json.Marshal(gmcp.CharWizardStep{
		Flow: "creation", Step: "password", Type: "text", Prompt: "Password:", Secret: true,
	})
	if got := string(out); !strings.Contains(got, `"secret":true`) {
		t.Errorf("secret text step must carry secret:true, got %q", got)
	}
}

func TestCharCommands_FullPayloadShape(t *testing.T) {
	payload := gmcp.CharCommands{Categories: []gmcp.CharCommandCategory{{
		Key:   "combat",
		Title: "Combat",
		Commands: []gmcp.CharCommand{
			{Keyword: "kill", Brief: "Attack a target.", Syntax: "kill [target]"},
			{Keyword: "flee"},
		},
	}}}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var back gmcp.CharCommands
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if len(back.Categories) != 1 || back.Categories[0].Key != "combat" || len(back.Categories[0].Commands) != 2 {
		t.Errorf("round-trip mismatch: %s", data)
	}
	// Optional brief/syntax omit when empty.
	if got := string(data); !strings.Contains(got, `"keyword":"flee"`) || strings.Contains(got, `"keyword":"flee","brief"`) {
		t.Errorf("flee should omit empty brief/syntax: %s", got)
	}
}

func TestCharCommands_PackageConstant(t *testing.T) {
	if gmcp.PackageCharCommands != "Char.Commands" {
		t.Errorf("PackageCharCommands = %q", gmcp.PackageCharCommands)
	}
}
