package economy

import (
	"sort"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/item"
)

// Legality / licensing property + tag names (sin-and-legality.md §6). The engine
// is setting-agnostic: a content pack skins a "credential" as a fake SIN and a
// "permit" as a license. An item with none of these behaves exactly as before,
// so an untagged world never sees the gate.
const (
	// PropLegality bands a good: "legal" (default), "restricted", "forbidden".
	// An absent/unrecognized value falls open to legal (§2, never trap content
	// behind a typo).
	PropLegality = "legality"
	// PropPermit names the permit category that clears a restricted good (§2).
	// Meaningful only when legality == restricted.
	PropPermit = "permit"
	// TagCredential marks a carried item as an identity credential (§3).
	TagCredential = "credential"
	// PropPermits is the permit categories a credential clears (§3).
	PropPermits = "permits"
)

// Legality bands (sin-and-legality.md §2).
const (
	LegalityLegal      = "legal"
	LegalityRestricted = "restricted"
	LegalityForbidden  = "forbidden"
)

// itemLegality reads the (band, permit) legality pair off a template property
// bag. An absent or unrecognized band falls open to legal; permit is only
// meaningful for a restricted good (§2).
func itemLegality(props map[string]any) (band, permit string) {
	band = LegalityLegal
	if raw, ok := props[PropLegality].(string); ok {
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case LegalityRestricted:
			band = LegalityRestricted
		case LegalityForbidden:
			band = LegalityForbidden
		}
	}
	if band == LegalityRestricted {
		permit, _ = props[PropPermit].(string)
		permit = strings.ToLower(strings.TrimSpace(permit))
	}
	return band, permit
}

// credentialPermits reads a credential's permit categories off a property bag
// (§3). Values are lower-cased and trimmed. A non-list / absent value yields an
// empty set — a valid identity that clears no specific permit.
func credentialPermits(props map[string]any) map[string]bool {
	set := map[string]bool{}
	raw, ok := props[PropPermits]
	if !ok {
		return set
	}
	// YAML decodes a list to []any (or []string when homogeneous); accept both.
	switch list := raw.(type) {
	case []any:
		for _, v := range list {
			if s, ok := v.(string); ok {
				if s = strings.ToLower(strings.TrimSpace(s)); s != "" {
					set[s] = true
				}
			}
		}
	case []string:
		for _, s := range list {
			if s = strings.ToLower(strings.TrimSpace(s)); s != "" {
				set[s] = true
			}
		}
	}
	return set
}

// carriedCredential is one credential the buyer is holding, resolved for the
// legality gate (§4). Name is for the refusal message; Permits is what it clears.
type carriedCredential struct {
	Name    string
	Permits map[string]bool
}

// buyerCredentials collects the credential items the buyer is carrying (§3).
// The gate reads only carried inventory this slice — an equipped credential does
// not count (§4, documented limitation).
func (s *ShopService) buyerCredentials(sh Shopper) []carriedCredential {
	var out []carriedCredential
	for _, id := range sh.Inventory() {
		inst := s.itemInstance(id)
		if inst == nil || !instanceHasTag(inst, TagCredential) {
			continue
		}
		out = append(out, carriedCredential{
			Name:    inst.Name(),
			Permits: credentialPermits(inst.Properties()),
		})
	}
	return out
}

// CredentialInfo is a carried credential surfaced to the licenses verb
// (sin-and-legality.md §5): a display name and the permit categories it clears.
type CredentialInfo struct {
	Name    string
	Permits []string // permit categories, sorted for stable display
}

// CarriedCredentials lists the credentials the shopper is carrying for the
// licenses verb (sin-and-legality.md §5), independent of any shop. Permits are
// sorted so the display is deterministic.
func (s *ShopService) CarriedCredentials(sh Shopper) []CredentialInfo {
	creds := s.buyerCredentials(sh)
	out := make([]CredentialInfo, 0, len(creds))
	for _, c := range creds {
		perms := make([]string, 0, len(c.Permits))
		for p := range c.Permits {
			perms = append(perms, p)
		}
		sort.Strings(perms)
		out = append(out, CredentialInfo{Name: c.Name, Permits: perms})
	}
	return out
}

// refusesLicense runs the sin-and-legality.md §4 purchase gate for a
// requires_license shop against the resolved stock item. It returns the refusal
// outcome (ShopOK when the sale is allowed) plus the required permit for a
// ShopLicenseRequired result so the caller can name it. A shop that does not
// require a license (the default, i.e. a shadow vendor) never refuses here.
func (s *ShopService) refusesLicense(sh Shopper, shop ShopConfig, tpl *item.Template) (ShopOutcome, string) {
	if !shop.RequiresLicense || tpl == nil {
		return ShopOK, ""
	}
	band, permit := itemLegality(tpl.Properties)
	// A legitimate storefront never sells contraband, papers or not (§4.3).
	if band == LegalityForbidden {
		return ShopForbiddenGoods, ""
	}
	creds := s.buyerCredentials(sh)
	// Identity presence: any credential clears a legal good (§4.1).
	if len(creds) == 0 {
		return ShopSINRequired, ""
	}
	if band != LegalityRestricted {
		return ShopOK, ""
	}
	// Restricted → a single carried credential must list the permit (§4.2). A
	// restricted good that names no permit is cleared by any valid credential.
	if permit == "" {
		return ShopOK, ""
	}
	for _, c := range creds {
		if c.Permits[permit] {
			return ShopOK, ""
		}
	}
	return ShopLicenseRequired, permit
}
