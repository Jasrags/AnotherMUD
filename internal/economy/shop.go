package economy

import (
	"math"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/item"
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
// §3.7). The query is normalized (article stripped, lowercased,
// hyphens→spaces) and matched as a prefix against each resolvable
// entry's normalized name AND its short id (the segment after the
// last ':'). Returns the single matching template, or nil when the
// query matches zero or more than one entry — ambiguity is treated as
// no match (the caller reports ItemNotForSale either way, §3.7).
func resolveStock(tpls *item.Templates, cfg ShopConfig, query string) *item.Template {
	if tpls == nil {
		return nil
	}
	q := normalizeQuery(query)
	if q == "" {
		return nil
	}
	var match *item.Template
	count := 0
	for _, id := range cfg.Sells {
		tpl, err := tpls.Get(item.TemplateID(id))
		if err != nil {
			continue
		}
		nameKey := normalizeQuery(tpl.Name)
		idKey := normalizeQuery(shortID(string(tpl.ID)))
		if strings.HasPrefix(nameKey, q) || strings.HasPrefix(idKey, q) {
			match = tpl
			count++
		}
	}
	if count != 1 {
		// 0 → no match; >1 → ambiguous (§3.7).
		return nil
	}
	return match
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

// normalizeQuery strips a leading article, lowercases, and converts
// hyphens to spaces, then collapses surrounding whitespace (spec
// §3.7 / §3.8 normalization).
func normalizeQuery(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = stripArticle(s)
	s = strings.ReplaceAll(s, "-", " ")
	return strings.TrimSpace(s)
}

// stripArticle removes a leading "a ", "an ", or "the " (input already
// lowercased + trimmed by the caller).
func stripArticle(s string) string {
	for _, art := range []string{"a ", "an ", "the "} {
		if strings.HasPrefix(s, art) {
			return strings.TrimSpace(s[len(art):])
		}
	}
	return s
}
