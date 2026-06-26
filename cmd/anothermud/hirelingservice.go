package main

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// hirelingService implements command.HirelingService (hireable-mobs.md §2/§3)
// over the boot spawn pipeline: it resolves hireable templates, materializes an
// owned hireling into the world, and removes it again. It owns none of the
// durable state (that lives on the player save) — the composition-root glue
// between the hire verbs and the mob spawner, mirroring mountService.
type hirelingService struct {
	spawner *bootSpawner
	store   *entities.Store
}

// HirelingName returns a hireling template's display name and whether the id
// resolves to a hireable mob (a template carrying a `hireling:` block). A
// non-hireable or unknown id returns ("", false).
func (h *hirelingService) HirelingName(templateID string) (string, bool) {
	tpl, err := h.spawner.mobTemplates.Get(mob.TemplateID(templateID))
	if err != nil || tpl.Hireling == nil {
		return "", false
	}
	return tpl.Name, true
}

// RecruiterOffers returns the hirelings offered by the recruiter mob templates
// among recruiterTemplateIDs (hireable-mobs.md §3.1) — the catalog `hire` resolves
// against. A non-recruiter id contributes nothing; each recruiter's offer entries
// resolve to hireable templates (carrying a `hireling:` block). Deduped by
// hireling template id, stably ordered by name then id.
func (h *hirelingService) RecruiterOffers(recruiterTemplateIDs []string) []command.HireableOffer {
	seen := make(map[string]bool)
	var out []command.HireableOffer
	for _, rid := range recruiterTemplateIDs {
		rt, err := h.spawner.mobTemplates.Get(mob.TemplateID(rid))
		if err != nil || rt.Recruiter == nil {
			continue
		}
		for _, offer := range rt.Recruiter.Offers {
			tpl, ok := h.resolveHireable(offer)
			if !ok || seen[string(tpl.ID)] {
				continue
			}
			seen[string(tpl.ID)] = true
			out = append(out, command.HireableOffer{
				TemplateID: string(tpl.ID),
				Name:       tpl.Name,
				HireCost:   tpl.Hireling.HireCost,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].TemplateID < out[j].TemplateID
	})
	return out
}

// resolveHireable resolves a recruiter's offer entry to a hireable mob template:
// an exact template id first, else a name/keyword match against hireable
// templates. Returns false when nothing hireable matches (a content typo just
// drops that offer rather than failing the whole recruiter).
func (h *hirelingService) resolveHireable(offer string) (*mob.Template, bool) {
	if t, err := h.spawner.mobTemplates.Get(mob.TemplateID(offer)); err == nil && t.Hireling != nil {
		return t, true
	}
	q := strings.ToLower(strings.TrimSpace(offer))
	tpls := h.spawner.mobTemplates.All()
	sort.Slice(tpls, func(i, j int) bool { return tpls[i].ID < tpls[j].ID })
	for _, tpl := range tpls {
		if tpl.Hireling != nil && hireableMatches(tpl.Name, string(tpl.ID), q) {
			return tpl, true
		}
	}
	return nil, false
}

// hireableMatches reports whether a query matches a hireling template by name or
// id leaf (case-insensitive substring), mirroring the command layer's
// templateMatches. An empty query matches the first hireable template.
func hireableMatches(name, id, q string) bool {
	if q == "" {
		return true
	}
	if strings.Contains(strings.ToLower(name), q) {
		return true
	}
	leaf := id
	if i := strings.LastIndex(id, ":"); i >= 0 {
		leaf = id[i+1:]
	}
	return strings.Contains(strings.ToLower(leaf), q)
}

// Materialize spawns the owned hireling into roomID through the full mob pipeline
// (the same path area spawning uses) and stamps ownerID as its owner + marks it a
// hireling. Returns the live hireling's entity id.
func (h *hirelingService) Materialize(ctx context.Context, ownerID, templateID string, roomID world.RoomID) (entities.EntityID, error) {
	id, err := h.spawner.spawnMob(ctx, templateID, roomID)
	if err != nil {
		return "", err
	}
	// The owner + hireling stamp is load-bearing: slice-2 follow/guard/combat-assist
	// gates on IsHireling()/OwnerID(). If the just-spawned mob can't be stamped
	// (a concurrent untrack, or — never in practice — a non-mob entity), surface an
	// error so HireHandler refunds rather than recording a contract on an unmarked
	// creature that all later behavior would silently skip.
	e, ok := h.store.GetByID(id)
	if !ok {
		logging.From(ctx).Warn("hireling materialize: mob missing after spawn",
			slog.String("template", templateID), slog.String("entity", string(id)))
		return "", fmt.Errorf("hireling materialize: mob %s not found after spawn", id)
	}
	inst, ok := e.(*entities.MobInstance)
	if !ok {
		logging.From(ctx).Warn("hireling materialize: unexpected entity type",
			slog.String("template", templateID), slog.String("entity", string(id)))
		return "", fmt.Errorf("hireling materialize: entity %s is not a mob", id)
	}
	inst.SetOwner(ownerID)
	inst.SetHireling(true)
	return id, nil
}

// Dematerialize removes a live hireling from the world (dismiss / §9 logout): out
// of its room and out of the entity store. Ownership (the save record) is
// untouched. Reports whether the hireling was present and removed.
func (h *hirelingService) Dematerialize(ctx context.Context, id entities.EntityID) bool {
	if _, ok := h.store.GetByID(id); !ok {
		return false
	}
	h.spawner.placement.Remove(id)
	if err := h.store.Untrack(id); err != nil {
		logging.From(ctx).Warn("hireling dematerialize: untrack failed",
			slog.String("hireling", string(id)), slog.Any("err", err))
	}
	return true
}

// responsiblePlayer maps a MobKilled killer id to the player who bears
// responsibility for the kill: a direct player killer (viaHireling=false), or the
// OWNER of a hireling that landed the blow (viaHireling=true — hireable-mobs.md
// §6). Returns "" for a wild/scripted killer that credits no one. The on-kill
// hooks (faction standing, kill-XP) share this resolution but differ on what they
// do with it — kill-XP gates a hireling kill on owner participation (§6.4), while
// faction standing attributes unconditionally (a consequence, not a reward).
func responsiblePlayer(store *entities.Store, killerID string) (pid string, viaHireling bool) {
	if p, ok := strings.CutPrefix(killerID, combat.PlayerPrefix); ok {
		return p, false
	}
	if owner := hirelingOwnerOf(store, killerID); owner != "" {
		return owner, true
	}
	return "", false
}

// hirelingOwnerOf maps a combatant id to the owner player id of the hireling it
// names, or "" when the id is not an owned hireling (hireable-mobs.md §6.3/§6.4).
// Used by the corpse owner-set + kill-XP hooks to credit a hireling's kill to its
// owner.
func hirelingOwnerOf(store *entities.Store, combatantID string) string {
	eid := combat.EntityIDOf(combat.CombatantID(combatantID))
	e, ok := store.GetByID(entities.EntityID(eid))
	if !ok {
		return ""
	}
	m, ok := e.(*entities.MobInstance)
	if !ok || !m.IsHireling() {
		return ""
	}
	return m.OwnerID()
}
