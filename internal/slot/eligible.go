package slot

import "strings"

// IsEligible reports whether base (a slot's base name) is a member of an
// item's eligible-slot set, case-insensitively (inventory-equipment-items
// §3.3). The set holds lowercased names produced at content load; base is
// lowercased here so a hand-built caller passing a mixed-case slot name
// still matches. An empty set matches nothing — an item declaring no
// eligible slots is not equippable (§3.4 step 3).
func IsEligible(eligible []string, base string) bool {
	base = strings.ToLower(strings.TrimSpace(base))
	if base == "" {
		return false
	}
	for _, s := range eligible {
		if strings.ToLower(strings.TrimSpace(s)) == base {
			return true
		}
	}
	return false
}
