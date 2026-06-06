package light

// Fuel burn-down (spec §3.2). A lit fuel-burning source loses fuel on a
// recurring tick and gutters out (becomes unlit) at zero. The drain
// shape mirrors sustenance (economy-survival §4.4): a configured cadence
// and amount, applied by a composition-root tick handler. A permanent
// source (no fuel property) never burns.

// FuelConfig is the burn cadence/amount (spec §3.2 / §11).
type FuelConfig struct {
	// BurnAmount is the fuel removed per burn tick.
	BurnAmount int
	// BurnCadence is the burn interval in engine ticks. The world-tick
	// fuel handler registers at this cadence.
	BurnCadence uint64
}

// DefaultFuelConfig returns a documented starting point: one unit of
// fuel per burn tick, on the same 30-second cadence sustenance drains
// at (cadence 300 at the default 100ms tick). Content sizes a torch's
// `fuel` value against this to set its lifetime.
func DefaultFuelConfig() FuelConfig {
	return FuelConfig{BurnAmount: 1, BurnCadence: 300}
}

// FuelSource is the slice of an item the burn loop mutates: read its
// lit/fuel state and atomically decrement fuel. *entities.ItemInstance
// satisfies it.
type FuelSource interface {
	Property(key string) (any, bool)
	SetProperty(key string, value any)
	DecrementInt(key string, amount int) (remaining int, hitZero bool)
}

// Burn applies one burn step to src and reports the outcome:
//
//   - An unlit source, or a permanent source (no fuel property), is left
//     untouched: returns (0, false).
//   - A lit fuel source is decremented by amount. When fuel reaches
//     zero the source gutters: its lit flag is cleared in the same step
//     and guttered is true. The caller then notifies the holder,
//     publishes light.source.extinguished, and (Phase 6) transitions
//     the room if this was its light.
//
// burnedFuel reports whether src was an eligible (lit, fuel-bearing)
// source that actually burned this step — the caller uses it to skip
// non-sources without inspecting fuel itself.
func Burn(src FuelSource, amount int) (remaining int, guttered, burnedFuel bool) {
	if src == nil || !IsLit(src) {
		return 0, false, false
	}
	if _, ok := src.Property(PropItemFuel); !ok {
		// Permanent source: always-on while lit, never gutters (§3.2).
		return 0, false, false
	}
	rem, hitZero := src.DecrementInt(PropItemFuel, amount)
	if hitZero {
		src.SetProperty(PropItemLit, false)
		return 0, true, true
	}
	return rem, false, true
}
