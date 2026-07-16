package session

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/gmcp"
	"github.com/Jasrags/AnotherMUD/internal/logging"
)

// flushGmcpShop snapshots the shop the actor is standing at into the rich
// Char.Shop trade form (web-client-plan P3 Slice B+) and emits a frame when it
// differs from the last-sent shadow. Rides the same gmcp-items-flush tick pass
// as the craft form (shop affordability + the sell list track money + carried
// items, this pass's inputs) with the same no-op guards: non-GMCP conn, GMCP
// inactive, or no shop service wired.
//
// The form is CONTEXTUAL: when the actor is not at a shop the payload is closed
// (open=false, empty lists). Because it is rebuilt and byte-diffed every tick, a
// room change into/out of a shop, a spent coin, or a picked-up sellable item all
// re-emit. Marshaled-bytes shadow (like Char.Recipes), guarded by gmcpItemsMu.
func (a *connActor) flushGmcpShop(ctx context.Context, shopSvc *economy.ShopService, money economy.CurrencyLabel) {
	if shopSvc == nil {
		return
	}
	sender, ok := a.conn.(gmcpSender)
	if !ok || !sender.GmcpActive() {
		return
	}

	data, err := json.Marshal(a.buildShopForm(shopSvc, money))
	if err != nil {
		return
	}

	a.gmcpItemsMu.Lock()
	unchanged := a.gmcpShopValid && string(a.gmcpShopLast) == string(data)
	if !unchanged {
		a.gmcpShopLast = data
		a.gmcpShopValid = true
	}
	a.gmcpItemsMu.Unlock()
	if unchanged {
		return
	}

	if err := sender.SendGmcp(ctx, gmcp.PackageCharShop, data); err != nil {
		logging.From(ctx).Debug("gmcp shop send failed",
			slog.String("player", a.PlayerName()),
			slog.Any("err", err))
	}
}

// buildShopForm resolves the shop NPC in the actor's room and projects it into
// the Char.Shop payload via the economy service's read-only ShopForm, supplying
// the buyer's skill + faction predicates (built from a.prof/a.faction) so the
// panel matches what buy/sell would actually do. When no shop is present the
// payload is closed (open=false) so the client hides the panel. Prices are
// formatted here through the world's CurrencyLabel — the wire carries no currency
// vocabulary.
func (a *connActor) buildShopForm(shopSvc *economy.ShopService, money economy.CurrencyLabel) gmcp.CharShop {
	mob := a.shopMobInRoom()
	if mob == nil {
		return gmcp.CharShop{Open: false, Buy: []gmcp.ShopItem{}, Sell: []gmcp.ShopItem{}}
	}
	cfg := command.ShopConfigFromMob(mob)
	form := shopSvc.ShopForm(a, cfg, a.shopSkillChecker(), a.shopStandingFunc())
	return gmcp.CharShop{
		Open:       true,
		Shopkeeper: mob.Name(),
		Money:      money.Format64(form.Balance),
		Refused:    form.Refused,
		Buy:        shopOffers(form.Buy, money, "buy"),
		Sell:       shopOffers(form.Sell, money, "sell"),
	}
}

// shopOffers converts economy ShopOffer rows into wire ShopItem rows: the price
// formatted through the CurrencyLabel and the submit command built as
// `<verb> <token>` (buy/sell). A single-count sell row omits qty (a client reads
// absent as 1).
func shopOffers(offers []economy.ShopOffer, money economy.CurrencyLabel, verb string) []gmcp.ShopItem {
	out := make([]gmcp.ShopItem, 0, len(offers))
	for _, o := range offers {
		qty := o.Qty
		if qty <= 1 {
			qty = 0
		}
		out = append(out, gmcp.ShopItem{
			Name:       o.Name,
			Price:      money.Format64(o.Price),
			Qty:        qty,
			Cmd:        verb + " " + o.Token,
			Affordable: o.Affordable,
		})
	}
	return out
}

// shopMobInRoom returns the first shop-tagged mob in the actor's room the actor
// may interact with, or nil (economy-survival §3.2 — "the first NPC carrying the
// shop tag"). It mirrors command.findShopInRoom EXACTLY, including the
// quest-spawn ownership gate: a foreign quest-scoped shopkeeper (owned by another
// player) is skipped (quest-spawns.md Phase 2), so the panel never dangles buy/
// sell offers the `buy`/`sell` verbs would refuse with "There is no shop here."
func (a *connActor) shopMobInRoom() *entities.MobInstance {
	room := a.Room()
	if room == nil || a.items == nil || a.placement == nil {
		return nil
	}
	// Same entity-visibility predicate the room renderer uses (session.go /
	// linkdead.go): an admin sees foreign quest spawns, a bystander does not. A
	// nil predicate means "show everything" (staff bypass / no player identity) —
	// guarded exactly as roomrender.go does, NOT called directly (it would panic).
	visible := command.QuestSpawnVisible(a, a.adminRole)
	for _, id := range a.placement.InRoom(room.ID) {
		e, ok := a.items.GetByID(id)
		if !ok {
			continue
		}
		if visible != nil && !visible(e) {
			continue // foreign quest spawn — not interactable
		}
		mob, ok := e.(*entities.MobInstance)
		if !ok {
			continue
		}
		for _, t := range mob.Tags() {
			if strings.EqualFold(t, economy.TagShop) {
				return mob
			}
		}
	}
	return nil
}

// shopSkillChecker builds the §7 purchase-skill predicate from the actor's
// proficiency manager (nil when proficiency isn't wired → no gate). Mirrors
// command.shopSkillChecker so the panel gates buys the same way the verb does.
func (a *connActor) shopSkillChecker() economy.SkillChecker {
	if a.prof == nil {
		return nil
	}
	eid := a.PlayerID()
	if eid == "" {
		eid = a.ID()
	}
	return func(discipline string, level int) bool {
		have, _ := a.prof.Proficiency(eid, discipline)
		return have >= level
	}
}

// shopStandingFunc builds the faction §6 standing resolver from the actor's
// faction manager (nil when faction isn't wired → no gate/pricing). connActor is
// a faction.Entity, so its own standing feeds the shop's access + favored-pricing
// logic. Mirrors command.shopStandingFunc; returns ok=false for a faction not in
// content so the shop fails open on a content typo.
func (a *connActor) shopStandingFunc() economy.StandingFunc {
	if a.faction == nil {
		return nil
	}
	return func(factionID string) (int, bool) {
		def, ok := a.faction.Registry().Get(factionID)
		if !ok {
			return 0, false
		}
		return a.faction.Get(a, def), true
	}
}
