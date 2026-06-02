package economy

import (
	"math"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/keyword"
)

// Tag / property names the shop feature recognizes (spec
// economy-survival §3.1). The shop tag marks a vendor NPC; no_sell on
// an item refuses a sale (§3.6 step 2).
const (
	TagShop   = "shop"
	TagNoSell = "no_sell"
	// PropValue is the item value the buy/sell prices are computed
	// from (§3.3). Shared with the currency auto-convert path.
	PropValue = "value"
)

// EconomyConfig holds the global shop defaults a per-shop ShopConfig
// falls back to (spec §3.1). The documented defaults are a 1.2 buy
// markup and a 0.5 sell discount; both are configurable.
type EconomyConfig struct {
	BuyMarkup    float64
	SellDiscount float64
}

// DefaultEconomyConfig returns the spec §3.1 defaults.
func DefaultEconomyConfig() EconomyConfig {
	return EconomyConfig{BuyMarkup: 1.2, SellDiscount: 0.5}
}

// ShopConfig is a per-shop record (spec §3.1). Sells lists the item
// template ids the shop offers. BuyMarkup / SellDiscount override the
// global defaults only when positive — zero / unset falls through
// (§3.1, §3.3).
type ShopConfig struct {
	Sells        []string
	BuyMarkup    float64
	SellDiscount float64
}

// markup returns the effective buy multiplier: the per-shop override
// when positive, else the global default (§3.3).
func (c ShopConfig) markup(global EconomyConfig) float64 {
	if c.BuyMarkup > 0 {
		return c.BuyMarkup
	}
	return global.BuyMarkup
}

// discount returns the effective sell multiplier (§3.3).
func (c ShopConfig) discount(global EconomyConfig) float64 {
	if c.SellDiscount > 0 {
		return c.SellDiscount
	}
	return global.SellDiscount
}

// buyPrice computes max(1, round(value × markup)) as int64 so very
// expensive items don't overflow and a shop never sells for free
// (spec §3.3).
func buyPrice(value int, cfg ShopConfig, global EconomyConfig) int64 {
	return floorAtOne(float64(value) * cfg.markup(global))
}

// sellPrice computes max(1, round(value × discount)) (spec §3.3).
func sellPrice(value int, cfg ShopConfig, global EconomyConfig) int64 {
	return floorAtOne(float64(value) * cfg.discount(global))
}

func floorAtOne(v float64) int64 {
	p := int64(math.Round(v))
	if p < 1 {
		return 1
	}
	return p
}

// ShopOutcome enumerates the result of a shop operation (spec §3.5 /
// §3.6 / §3.9 return reasons).
type ShopOutcome int

const (
	// ShopOK — operation succeeded.
	ShopOK ShopOutcome = iota
	// ShopItemNotForSale — stock query missed / ambiguous, item
	// creation failed, or a cancellable event vetoed (§3.5 / §3.7).
	ShopItemNotForSale
	// ShopInsufficientGold — buyer can't afford the item; the price
	// rides along so the caller can report it (§3.5 step 4).
	ShopInsufficientGold
	// ShopItemNotInInventory — sell query matched nothing the player
	// holds (§3.6 step 1).
	ShopItemNotInInventory
	// ShopItemIsNoSell — the item carries the no_sell tag (§3.6 step 2).
	ShopItemIsNoSell
	// ShopItemValueZero — the item's value is zero / missing (§3.6
	// step 3).
	ShopItemValueZero
)

// Listing is one row of a shop's offered stock (spec §3.4).
type Listing struct {
	TemplateID string
	Name       string
	BuyPrice   int64
}

// listings resolves the shop's sells list into displayable rows,
// dropping entries whose template id is unknown or whose value is not
// positive (spec §3.4). Order follows the sells list.
func listings(tpls *item.Templates, cfg ShopConfig, global EconomyConfig) []Listing {
	if tpls == nil {
		return nil
	}
	out := make([]Listing, 0, len(cfg.Sells))
	for _, id := range cfg.Sells {
		tpl, err := tpls.Get(item.TemplateID(id))
		if err != nil {
			continue
		}
		value := templateValue(tpl)
		if value <= 0 {
			continue
		}
		out = append(out, Listing{
			TemplateID: string(tpl.ID),
			Name:       tpl.Name,
			BuyPrice:   buyPrice(value, cfg, global),
		})
	}
	return out
}

// resolveStock matches query against the shop's sells list (spec
// §3.7) using the shared keyword rules (exact keyword → prefix keyword
// → name substring, inventory-equipment-items §6.1), so stock answers
// to its content keywords the same way look/get/wear do — `cap` finds
// "a leather cap". Each entry's short id (the segment after the last
// ':'), in both hyphenated and spaced form, is offered as a synthetic
// keyword so `<template-id>` lookups keep working. Returns the single
// matching template, or nil when the query matches zero or more than
// one entry — ambiguity is treated as no match (the caller reports
// ItemNotForSale either way, §3.7).
func resolveStock(tpls *item.Templates, cfg ShopConfig, query string) *item.Template {
	if tpls == nil || strings.TrimSpace(query) == "" {
		return nil
	}
	cands := make([]keyword.Named, 0, len(cfg.Sells))
	for _, id := range cfg.Sells {
		tpl, err := tpls.Get(item.TemplateID(id))
		if err != nil {
			continue
		}
		cands = append(cands, namedTemplate{tpl})
	}
	// ResolveAll returns every match by the §6.1 exact/prefix/substring
	// rules; §3.7 wants the unambiguous single match, so len != 1 → nil
	// (0 = no match, >1 = ambiguous).
	matches := keyword.ResolveAll(cands, query)
	if len(matches) != 1 {
		return nil
	}
	return matches[0].(namedTemplate).tpl
}

// namedTemplate adapts an item.Template to keyword.Named so shop stock
// resolves by the same rules as live item instances. The template's
// short id is appended (hyphenated and spaced) as a synthetic keyword
// so `<template-id>` queries keep resolving even when the id differs
// from the display name (§3.7 short-id match).
type namedTemplate struct{ tpl *item.Template }

func (n namedTemplate) Name() string { return n.tpl.Name }

func (n namedTemplate) Keywords() []string {
	sid := shortID(string(n.tpl.ID))
	out := make([]string, 0, len(n.tpl.Keywords)+2)
	out = append(out, n.tpl.Keywords...)
	out = append(out, sid)
	if spaced := strings.ReplaceAll(sid, "-", " "); spaced != sid {
		out = append(out, spaced)
	}
	return out
}

// templateValue reads the integer `value` property off a template,
// tolerating the int / int64 / float64 shapes yaml.v3 produces. Zero
// when absent or non-numeric.
func templateValue(tpl *item.Template) int {
	if tpl == nil || tpl.Properties == nil {
		return 0
	}
	switch n := tpl.Properties[PropValue].(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

// shortID returns the segment after the last ':' in a namespace-
// qualified id, or the whole string when unqualified (spec §3.7).
func shortID(id string) string {
	if i := strings.LastIndex(id, ":"); i >= 0 {
		return id[i+1:]
	}
	return id
}
