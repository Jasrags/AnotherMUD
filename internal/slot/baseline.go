package slot

// RegisterEngineBaseline installs the engine's default body-slot set
// onto r. Called at boot before pack loading so packs can supplement
// (and would error on collision if they try to redefine these).
//
// The baseline is deliberately small — just enough to make the starter
// item templates work (short-sword → wield, leather-cap → head, plus a
// multi-cap "finger" so the keying scheme is exercised) and to give the
// equipment footprint model (inventory-equipment-items §3.3) the second
// hand a two-handed weapon spans. Authoring more slots is a pack concern.
func RegisterEngineBaseline(r *Registry) error {
	baseline := []Def{
		{Name: "wield", Label: "wielded", Max: 1, Scope: EngineScope},
		// The off hand. Cap 1, bare key. A one-handed weapon or shield
		// targets it directly; a two-handed weapon lists it as a companion
		// slot so its footprint (§3.3) ties up both hands.
		{Name: "offhand", Label: "held in the off hand", Max: 1, Scope: EngineScope},
		{Name: "head", Label: "worn on head", Max: 1, Scope: EngineScope},
		{Name: "finger", Label: "worn on finger", Max: 2, Scope: EngineScope},
		// The held-light slot (light-and-darkness §3.3): the one active
		// source the viewer provides. Cap 1, so the slot key is the bare
		// "light". Whether it contends with hands (two-handed weapons /
		// shields) is left to the equipment model (§12) — today it is a
		// free slot.
		{Name: "light", Label: "carried as a light source", Max: 1, Scope: EngineScope},
	}
	for _, d := range baseline {
		if err := r.Register(d); err != nil {
			return err
		}
	}
	return nil
}
