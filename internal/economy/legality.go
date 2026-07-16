package economy

import (
	"sort"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/entities"
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
	// PropCredentialRating is the fake's quality — the bonus the §7 scan roll
	// adds. Absent ⇒ 0.
	PropCredentialRating = "credential_rating"
	// PropBurned is the persisted per-instance "spent fake" flag (§7): a
	// credential whose store scan failed. A burned credential clears no gate.
	PropBurned = "burned"
)

// Legality bands (sin-and-legality.md §2).
const (
	LegalityLegal      = "legal"
	LegalityRestricted = "restricted"
	LegalityForbidden  = "forbidden"
)

// LicenseScanner rolls a legit store's §7 SIN scan: given the presented fake's
// rating and the store's scanner rating, it reports whether the fake passes
// (true) or is caught (false ⇒ the credential burns). Supplied per-call by the
// command layer (which owns the d20 roller), the same way SkillChecker /
// StandingFunc are. nil ⇒ no scan (fail-open, nothing burns).
type LicenseScanner func(credentialRating, scannerRating int) bool

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

// credentialBurned reports whether an item instance is a burned (spent) fake
// (§7). Truthy for a bool true or any non-zero numeric flag, tolerant of how the
// flag was decoded from the save.
func credentialBurned(inst *entities.ItemInstance) bool {
	v, ok := inst.Property(PropBurned)
	if !ok {
		return false
	}
	switch b := v.(type) {
	case bool:
		return b
	case int:
		return b != 0
	case int64:
		return b != 0
	case float64:
		return b != 0
	default:
		return false
	}
}

// carriedCredential is one credential the buyer is holding, resolved for the
// legality gate (§4). Inst backs the burn write (§7); Rating feeds the scan.
type carriedCredential struct {
	Inst    *entities.ItemInstance
	Name    string
	Rating  int
	Burned  bool
	Permits map[string]bool
}

// buyerCredentials collects the credential items the buyer is carrying (§3),
// including burned ones (the caller filters — the gate excludes burned, the
// licenses verb shows them marked). The gate reads only carried inventory this
// slice — an equipped credential does not count (§4, documented limitation).
func (s *ShopService) buyerCredentials(sh Shopper) []carriedCredential {
	var out []carriedCredential
	for _, id := range sh.Inventory() {
		inst := s.itemInstance(id)
		if inst == nil || !instanceHasTag(inst, TagCredential) {
			continue
		}
		out = append(out, carriedCredential{
			Inst:    inst,
			Name:    inst.Name(),
			Rating:  propInt(inst.Properties()[PropCredentialRating]),
			Burned:  credentialBurned(inst),
			Permits: credentialPermits(inst.Properties()),
		})
	}
	return out
}

// CredentialInfo is a carried credential surfaced to the licenses verb
// (sin-and-legality.md §5): a display name, the permit categories it clears, and
// whether it has been burned (spent).
type CredentialInfo struct {
	Name    string
	Permits []string // permit categories, sorted for stable display
	Burned  bool
}

// CarriedCredentials lists the credentials the shopper is carrying for the
// licenses verb (sin-and-legality.md §5), independent of any shop — burned ones
// included so the player sees a spent fake. Permits are sorted so the display is
// deterministic.
func (s *ShopService) CarriedCredentials(sh Shopper) []CredentialInfo {
	creds := s.buyerCredentials(sh)
	out := make([]CredentialInfo, 0, len(creds))
	for _, c := range creds {
		perms := make([]string, 0, len(c.Permits))
		for p := range c.Permits {
			perms = append(perms, p)
		}
		sort.Strings(perms)
		out = append(out, CredentialInfo{Name: c.Name, Permits: perms, Burned: c.Burned})
	}
	return out
}

// licenseGate is the resolved outcome of the §4/§7 licensing gate: the shop
// outcome, plus the required permit (for ShopLicenseRequired) and the burned
// credential's name (for ShopSINBurned) so the caller can name them.
type licenseGate struct {
	Outcome ShopOutcome
	Permit  string
	Burned  string
}

// refusesLicense runs the sin-and-legality.md §4 purchase gate for a
// requires_license shop, and — on a restricted good whose permit a credential
// clears — the §7 scan. A shadow vendor (RequiresLicense false, the default)
// never gates here. scan may be nil (no roll / fail-open).
func (s *ShopService) refusesLicense(sh Shopper, shop ShopConfig, tpl *item.Template, scan LicenseScanner) licenseGate {
	if !shop.RequiresLicense || tpl == nil {
		return licenseGate{Outcome: ShopOK}
	}
	band, permit := itemLegality(tpl.Properties)
	// A legitimate storefront never sells contraband, papers or not (§4.3).
	if band == LegalityForbidden {
		return licenseGate{Outcome: ShopForbiddenGoods}
	}
	// Only unburned credentials count toward any check (§4, §7).
	var valid []carriedCredential
	for _, c := range s.buyerCredentials(sh) {
		if !c.Burned {
			valid = append(valid, c)
		}
	}
	// Identity presence: any valid credential clears a legal good (§4.1).
	if len(valid) == 0 {
		return licenseGate{Outcome: ShopSINRequired}
	}
	if band != LegalityRestricted {
		return licenseGate{Outcome: ShopOK}
	}
	// Restricted → a credential must list the permit (§4.2). A restricted good
	// that names no permit is cleared by any valid credential (no scan — the scan
	// scrutinizes the license, and there is none to check).
	if permit == "" {
		return licenseGate{Outcome: ShopOK}
	}
	// Pick the highest-rated matching credential — the runner flashes their best
	// fake, and that is the one at risk in the scan (§7).
	var best *carriedCredential
	for i := range valid {
		if valid[i].Permits[permit] && (best == nil || valid[i].Rating > best.Rating) {
			best = &valid[i]
		}
	}
	if best == nil {
		return licenseGate{Outcome: ShopLicenseRequired, Permit: permit}
	}
	// §7 scan: the store rolls only when it has scrutiny (ScannerRating > 0) and a
	// scanner is wired. A failed roll burns the presented fake and refuses the sale.
	if shop.ScannerRating > 0 && scan != nil && !scan(best.Rating, shop.ScannerRating) {
		best.Inst.SetProperty(PropBurned, true)
		return licenseGate{Outcome: ShopSINBurned, Burned: best.Name}
	}
	return licenseGate{Outcome: ShopOK}
}
