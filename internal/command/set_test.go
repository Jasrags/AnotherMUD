package command_test

import (
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/faction"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/property"
	"github.com/Jasrags/AnotherMUD/internal/reputation"
)

// setPropRegistry builds a property registry exercising the three relevant
// states: an admin-settable string, an admin-settable int (for the type-
// coercion path), and a registered-but-NOT-admin-settable property.
func setPropRegistry(t *testing.T) *property.Registry {
	t.Helper()
	reg := property.NewRegistry()
	must := func(e property.Entry) {
		if err := reg.RegisterEngine(e); err != nil {
			t.Fatalf("RegisterEngine %q: %v", e.Name, err)
		}
	}
	must(property.Entry{Name: "key_for", Type: property.TypeString, AdminSettable: true})
	must(property.Entry{Name: "weight", Type: property.TypeInt, AdminSettable: true})
	must(property.Entry{Name: "secret_flag", Type: property.TypeString, AdminSettable: false})
	return reg
}

// An admin sets a mob's HP: the live vital changes, the confirmation
// reports the new fraction, and one admin.action fires with the kind/type/
// value in its args.
func TestSet_VitalHPOnMobLivesAndAudits(t *testing.T) {
	f := newConsiderFixture(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Bus = bus

	dispatchRole(t, env, admin, "set vital hp guard 10")

	if cur, _ := f.guard.Vitals().Snapshot(); cur != 10 {
		t.Errorf("guard HP = %d, want 10", cur)
	}
	if !strings.Contains(admin.lastLine(), "HP set to 10/40") {
		t.Errorf("confirmation = %q, want 'HP set to 10/40'", admin.lastLine())
	}
	if len(*got) != 1 {
		t.Fatalf("admin.action count = %d, want 1", len(*got))
	}
	ev := (*got)[0].(eventbus.AdminAction)
	if ev.Verb != "set" || ev.Target != f.guard.EntityID() || ev.Args != "vital hp=10" {
		t.Errorf("event = %+v, want verb=set target=%s args='vital hp=10'", ev, f.guard.EntityID())
	}
}

// `set gold amount self <n>` funds the admin through the currency service —
// the supported way to seed gold for testing/GMing (admin-verbs §4).
func TestSet_GoldOnSelf(t *testing.T) {
	f := newConsiderFixture(t)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Currency = economy.NewCurrencyService(nil)

	dispatchRole(t, env, admin, "set gold amount self 500")

	if env.Currency.Read(admin) != 500 {
		t.Errorf("gold = %d, want 500", env.Currency.Read(admin))
	}
	if !strings.Contains(admin.lastLine(), "Gold set to 500") {
		t.Errorf("confirmation = %q, want 'Gold set to 500'", admin.lastLine())
	}
}

// A negative gold value is refused with no write (§4).
func TestSet_GoldNegativeRefused(t *testing.T) {
	f := newConsiderFixture(t)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Currency = economy.NewCurrencyService(nil)

	dispatchRole(t, env, admin, "set gold amount self -5")

	if env.Currency.Read(admin) != 0 {
		t.Errorf("gold = %d, want 0 (refused)", env.Currency.Read(admin))
	}
}

// A value above the target's maximum clamps to max (§4).
func TestSet_VitalClampsToMax(t *testing.T) {
	f := newConsiderFixture(t)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()

	dispatchRole(t, env, admin, "set vital hp guard 999")

	if cur, max := f.guard.Vitals().Snapshot(); cur != max {
		t.Errorf("guard HP = %d/%d, want clamped to max", cur, max)
	}
}

// A non-numeric vital value is a usage error that writes nothing and
// audits nothing (§4).
func TestSet_VitalNonNumericRefused(t *testing.T) {
	f := newConsiderFixture(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Bus = bus

	dispatchRole(t, env, admin, "set vital hp guard abc")

	if cur, _ := f.guard.Vitals().Snapshot(); cur != 40 {
		t.Errorf("guard HP = %d, want 40 (unchanged on bad value)", cur)
	}
	if !strings.Contains(admin.lastLine(), "whole number") {
		t.Errorf("message = %q, want a numeric usage error", admin.lastLine())
	}
	if len(*got) != 0 {
		t.Errorf("a refused set must not audit, got %d", len(*got))
	}
}

// An unknown vital type is refused without writing.
func TestSet_UnknownVitalRefused(t *testing.T) {
	f := newConsiderFixture(t)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()

	dispatchRole(t, env, admin, "set vital mana guard 5")

	if !strings.Contains(admin.lastLine(), "Unknown vital") {
		t.Errorf("message = %q, want 'Unknown vital'", admin.lastLine())
	}
}

// An unknown kind reports it and falls through to the usage panel.
func TestSet_UnknownKindShowsUsage(t *testing.T) {
	f := newConsiderFixture(t)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()

	dispatchRole(t, env, admin, "set wibble x guard 5")

	out := allLines(admin.testActor)
	if !strings.Contains(out, "Unknown set kind") {
		t.Errorf("output = %q, want 'Unknown set kind'", out)
	}
	if !strings.Contains(out, "vital") || !strings.Contains(out, "hp") {
		t.Errorf("usage panel = %q, want it to list vital(hp)", out)
	}
}

// A bare set renders the self-documenting usage panel and audits nothing.
func TestSet_BareRendersUsagePanel(t *testing.T) {
	f := newConsiderFixture(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Bus = bus

	dispatchRole(t, env, admin, "set")

	out := allLines(admin.testActor)
	if !strings.Contains(out, "Usage: set") || !strings.Contains(out, "vital") {
		t.Errorf("usage panel = %q, want grammar + vital kind", out)
	}
	if len(*got) != 0 {
		t.Errorf("bare set must not audit, got %d", len(*got))
	}
}

// set is admin-gated (§2): a non-admin gets the unknown-verb "Huh?", with
// no write and no audit — and no disclosure that the verb exists.
func TestSet_RefusedForNonAdmin(t *testing.T) {
	f := newConsiderFixture(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	bob := newRoleActor("Bob", "p-bob") // no admin role
	bob.SetRoom(f.room)
	env := f.env()
	env.Bus = bus

	dispatchRole(t, env, bob, "set vital hp guard 10")

	if bob.lastLine() != "Huh?" {
		t.Errorf("refusal = %q, want 'Huh?'", bob.lastLine())
	}
	if cur, _ := f.guard.Vitals().Snapshot(); cur != 40 {
		t.Errorf("guard HP = %d, want 40 (non-admin must not write)", cur)
	}
	if len(*got) != 0 {
		t.Errorf("a refused set must not audit, got %d", len(*got))
	}
}

// An admin sets an admin-settable string property on a room item: the live
// property bag changes, the confirmation reports it, and one admin.action
// fires with kind/type/value in its args (admin-verbs §4 / M19.4h).
func TestSetProperty_OnItemWritesAndAudits(t *testing.T) {
	f := newConsiderFixture(t)
	item := f.spawnInRoom(t, sword())
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Bus = bus
	env.Properties = setPropRegistry(t)

	dispatchRole(t, env, admin, "set property key_for sword door-7")

	if v, _ := item.Property("key_for"); v != "door-7" {
		t.Errorf("key_for = %v, want \"door-7\"", v)
	}
	if !strings.Contains(admin.lastLine(), "key_for") {
		t.Errorf("confirmation = %q, want it to mention key_for", admin.lastLine())
	}
	if len(*got) != 1 {
		t.Fatalf("admin.action count = %d, want 1", len(*got))
	}
	ev := (*got)[0].(eventbus.AdminAction)
	if ev.Verb != "set" || ev.Target != string(item.ID()) || ev.Args != "property key_for=door-7" {
		t.Errorf("event = %+v, want verb=set target=%s args='property key_for=door-7'", ev, item.ID())
	}
}

// An int-typed property coerces a numeric value; the stored value is an int,
// not the raw string.
func TestSetProperty_IntCoercion(t *testing.T) {
	f := newConsiderFixture(t)
	item := f.spawnInRoom(t, sword())
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Properties = setPropRegistry(t)

	dispatchRole(t, env, admin, "set property weight sword 12")

	v, ok := item.Property("weight")
	if !ok {
		t.Fatal("weight property not written")
	}
	if n, isInt := v.(int); !isInt || n != 12 {
		t.Errorf("weight = %v (%T), want int 12", v, v)
	}
}

// A non-numeric value for an int property is a usage error: nothing is
// written and nothing is audited (§4).
func TestSetProperty_IntTypeMismatchRefused(t *testing.T) {
	f := newConsiderFixture(t)
	item := f.spawnInRoom(t, sword())
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Bus = bus
	env.Properties = setPropRegistry(t)

	dispatchRole(t, env, admin, "set property weight sword heavy")

	if _, ok := item.Property("weight"); ok {
		t.Error("weight written despite type mismatch")
	}
	if !strings.Contains(admin.lastLine(), "whole number") {
		t.Errorf("message = %q, want a numeric usage error", admin.lastLine())
	}
	if len(*got) != 0 {
		t.Errorf("a refused set must not audit, got %d", len(*got))
	}
}

// A registered-but-not-admin-settable property is refused without writing.
func TestSetProperty_NotAdminSettableRefused(t *testing.T) {
	f := newConsiderFixture(t)
	item := f.spawnInRoom(t, sword())
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Properties = setPropRegistry(t)

	dispatchRole(t, env, admin, "set property secret_flag sword x")

	if _, ok := item.Property("secret_flag"); ok {
		t.Error("secret_flag written despite not being admin-settable")
	}
	if !strings.Contains(admin.lastLine(), "not admin-settable") {
		t.Errorf("message = %q, want 'not admin-settable'", admin.lastLine())
	}
}

// An unknown (unregistered) property is refused.
func TestSetProperty_UnknownRefused(t *testing.T) {
	f := newConsiderFixture(t)
	f.spawnInRoom(t, sword())
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Properties = setPropRegistry(t)

	dispatchRole(t, env, admin, "set property nonesuch sword x")

	if !strings.Contains(admin.lastLine(), "Unknown property") {
		t.Errorf("message = %q, want 'Unknown property'", admin.lastLine())
	}
}

// A reserved key (template_id / room_id) is refused before any registry
// lookup, so the instance identity can't be corrupted via set.
func TestSetProperty_ReservedKeyRefused(t *testing.T) {
	f := newConsiderFixture(t)
	item := f.spawnInRoom(t, sword())
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Properties = setPropRegistry(t)
	before, _ := item.Property("template_id")

	dispatchRole(t, env, admin, "set property template_id sword evil")

	if after, _ := item.Property("template_id"); after != before {
		t.Errorf("template_id mutated: %v -> %v", before, after)
	}
	if !strings.Contains(admin.lastLine(), "reserved") {
		t.Errorf("message = %q, want 'reserved'", admin.lastLine())
	}
}

// Setting a property on a player target is not supported yet (no property
// bag) — it is refused gracefully without writing (M19.4h+ deferral).
func TestSetProperty_OnPlayerRefused(t *testing.T) {
	f := newConsiderFixture(t)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Properties = setPropRegistry(t)

	dispatchRole(t, env, admin, "set property key_for self door-1")

	if !strings.Contains(admin.lastLine(), "no settable properties") {
		t.Errorf("message = %q, want 'no settable properties'", admin.lastLine())
	}
}

// The usage panel lists the property kind alongside vital.
func TestSetProperty_UsagePanelListsProperty(t *testing.T) {
	f := newConsiderFixture(t)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()

	dispatchRole(t, env, admin, "set")

	out := allLines(admin.testActor)
	if !strings.Contains(out, "property") {
		t.Errorf("usage panel = %q, want it to list the property kind", out)
	}
}

// `set tag add <tag> <mob>` writes the live tag, re-indexes the store so a
// later GetByTag surfaces it, and audits with the kind/type/value (M19.4i).
func TestSetTag_AddOnMobWritesReindexesAndAudits(t *testing.T) {
	f := newConsiderFixture(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Bus = bus

	dispatchRole(t, env, admin, "set tag add guard cursed")

	if !f.guard.HasTag("cursed") {
		t.Error("guard missing the cursed tag after set tag add")
	}
	// The store index reflects it once the write-side buckets publish.
	f.store.SwapTagIndex()
	if hits := f.store.GetByTag("cursed"); len(hits) != 1 || hits[0].ID() != f.guard.ID() {
		t.Errorf("GetByTag(cursed) = %v, want [%q]", hits, f.guard.ID())
	}
	if len(*got) != 1 {
		t.Fatalf("admin.action count = %d, want 1", len(*got))
	}
	ev := (*got)[0].(eventbus.AdminAction)
	if ev.Verb != "set" || ev.Target != f.guard.EntityID() || ev.Args != "tag add=cursed" {
		t.Errorf("event = %+v, want verb=set target=%s args='tag add=cursed'", ev, f.guard.EntityID())
	}
}

// `set tag remove <tag> <mob>` drops a previously-added tag.
func TestSetTag_RemoveOnMob(t *testing.T) {
	f := newConsiderFixture(t)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	f.guard.AddTag("cursed")

	dispatchRole(t, env, admin, "set tag remove guard cursed")

	if f.guard.HasTag("cursed") {
		t.Error("guard still carries cursed after set tag remove")
	}
	if !strings.Contains(admin.lastLine(), "removed") {
		t.Errorf("confirmation = %q, want it to report removal", admin.lastLine())
	}
}

// `set tag add <tag> self` tags the admin (a player target) and audits.
func TestSetTag_AddOnPlayerPersists(t *testing.T) {
	f := newConsiderFixture(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Bus = bus

	dispatchRole(t, env, admin, "set tag add self vip")

	if !admin.hasTag("vip") {
		t.Errorf("player missing the vip tag after set tag add self; tags=%v", admin.tags)
	}
	if len(*got) != 1 {
		t.Fatalf("admin.action count = %d, want 1", len(*got))
	}
}

// A tag in a manager-owned namespace is refused before any write — an admin
// cannot hand-author an alignment / faction / reputation tag and desync the
// owning manager. The tag strings come from the real managers, so a prefix
// rename in those packages trips this test.
func TestSetTag_ManagerOwnedNamespacesRefused(t *testing.T) {
	cases := map[string]string{
		"alignment":  progression.TagAlignmentEvil,
		"faction":    faction.RankTag("townsfolk", "friendly"),
		"reputation": reputation.TierTag("Known Locally"),
	}
	for label, tag := range cases {
		t.Run(label, func(t *testing.T) {
			f := newConsiderFixture(t)
			bus := eventbus.New()
			got := captureEvents(t, bus, eventbus.EventAdminAction)
			admin := adminInRoom(f, "Maerys", "p-admin")
			env := f.env()
			env.Bus = bus

			dispatchRole(t, env, admin, "set tag add guard "+tag)

			if f.guard.HasTag(tag) {
				t.Errorf("guard carries manager-owned tag %q — guard bypassed", tag)
			}
			if !strings.Contains(admin.lastLine(), "cannot be set directly") {
				t.Errorf("message = %q, want the manager-owned refusal", admin.lastLine())
			}
			if len(*got) != 0 {
				t.Errorf("a refused set must not audit, got %d", len(*got))
			}
		})
	}
}

// An unknown tag op (neither add nor remove) is refused, writing nothing.
func TestSetTag_UnknownOpRefused(t *testing.T) {
	f := newConsiderFixture(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Bus = bus

	dispatchRole(t, env, admin, "set tag frobnicate guard cursed")

	if f.guard.HasTag("cursed") {
		t.Error("guard tagged despite an unknown op")
	}
	if !strings.Contains(admin.lastLine(), "Unknown tag op") {
		t.Errorf("message = %q, want 'Unknown tag op'", admin.lastLine())
	}
	if len(*got) != 0 {
		t.Errorf("a refused set must not audit, got %d", len(*got))
	}
}

// A structural engine tag (entities.TagMob) cannot be removed — doing so
// would drop the mob out of GetByTag(TagMob) and silence its AI for its
// lifetime. Refused before any mutation, and no audit fires.
func TestSetTag_StructuralMobTagRefused(t *testing.T) {
	f := newConsiderFixture(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Bus = bus

	dispatchRole(t, env, admin, "set tag remove guard mob")

	if !f.guard.HasTag("mob") {
		t.Error("structural 'mob' tag removed — the AI dispatcher would lose the mob")
	}
	if !strings.Contains(admin.lastLine(), "cannot be set directly") {
		t.Errorf("message = %q, want the structural-tag refusal", admin.lastLine())
	}
	if len(*got) != 0 {
		t.Errorf("a refused set must not audit, got %d", len(*got))
	}
}

// A no-op set tag (re-adding a tag already present) writes nothing and does
// NOT audit — the audit log stays limited to genuine tag changes.
func TestSetTag_NoOpDoesNotAudit(t *testing.T) {
	f := newConsiderFixture(t)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventAdminAction)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()
	env.Bus = bus
	f.guard.AddTag("cursed")

	dispatchRole(t, env, admin, "set tag add guard cursed")

	if !strings.Contains(admin.lastLine(), "Already tagged") {
		t.Errorf("message = %q, want an 'Already tagged' no-op notice", admin.lastLine())
	}
	if len(*got) != 0 {
		t.Errorf("a no-op set must not audit, got %d", len(*got))
	}
}

// The usage panel lists the tag kind with its add/remove ops.
func TestSetTag_UsagePanelListsTag(t *testing.T) {
	f := newConsiderFixture(t)
	admin := adminInRoom(f, "Maerys", "p-admin")
	env := f.env()

	dispatchRole(t, env, admin, "set")

	out := allLines(admin.testActor)
	if !strings.Contains(out, "tag") || !strings.Contains(out, "add") || !strings.Contains(out, "remove") {
		t.Errorf("usage panel = %q, want it to list tag(add, remove)", out)
	}
}
