package slot

// FreeKey returns the slot key to use for base given current occupancy:
// the lowest-index unoccupied key, or the index-0 key when every index is
// already occupied (the caller then displaces that occupant — auto-swap,
// inventory-equipment-items §3.4 step 6). occupied holds keys already
// claimed (value true) and is not mutated. Returns ErrNotFound when base
// is not a registered slot.
func (r *Registry) FreeKey(base string, occupied map[string]bool) (string, error) {
	def, err := r.Get(base)
	if err != nil {
		return "", err
	}
	for i := 0; i < def.Max; i++ {
		key, kerr := BuildKey(def.Name, i, def.Max)
		if kerr != nil {
			return "", kerr
		}
		if !occupied[key] {
			return key, nil
		}
	}
	// Every index occupied: index 0 is the displacement target.
	return BuildKey(def.Name, 0, def.Max)
}

// Footprint expands a target base slot plus companion base slots into the
// concrete slot keys an item occupies when equipped (inventory-equipment-
// items §3.3). The target key is returned first (the canonical/save key),
// followed by one key per companion. Each base is placed at its lowest
// free index via FreeKey; keys claimed earlier in this call count as
// occupied for later companions, so two companions sharing a base take
// distinct indices. occupied is the holder's current occupancy and is NOT
// mutated. Returns ErrNotFound if the target or any companion base is
// unregistered.
func (r *Registry) Footprint(target string, companions []string, occupied map[string]bool) ([]string, error) {
	claimed := make(map[string]bool, len(occupied)+len(companions)+1)
	for k, v := range occupied {
		if v {
			claimed[k] = true
		}
	}
	keys := make([]string, 0, len(companions)+1)

	targetKey, err := r.FreeKey(target, claimed)
	if err != nil {
		return nil, err
	}
	keys = append(keys, targetKey)
	claimed[targetKey] = true

	for _, comp := range companions {
		k, err := r.FreeKey(comp, claimed)
		if err != nil {
			return nil, err
		}
		keys = append(keys, k)
		claimed[k] = true
	}
	return keys, nil
}
