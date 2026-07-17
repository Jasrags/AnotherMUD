package command

import (
	"context"
	"fmt"

	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// Checkpoint room-property keys (sin-and-legality.md §7.1). A destination room
// opts into being an access-controlled threshold by carrying these: a positive
// checkpoint_scanner is the scan DC, and the optional checkpoint_permit names the
// access category a carried credential must clear. The command layer owns the
// parse (mirrors the shop block), so the world package stays free of the concept.
const (
	propCheckpointScanner = "checkpoint_scanner"
	propCheckpointPermit  = "checkpoint_permit"
)

// checkpointBlocks reports whether a SIN checkpoint on the destination room
// refuses the mover's crossing (sin-and-legality.md §7.1), and — when it does —
// returns the write error from the refusal message so the caller can return it
// directly. A room with no positive checkpoint_scanner is not a checkpoint
// (blocked=false). The gate fails open when the credential surface isn't wired
// (no shop service, or an actor that isn't a Shopper — e.g. a test double).
func checkpointBlocks(ctx context.Context, c *Context, dst *world.Room) (bool, error) {
	scanner, _ := dst.PropertyInt(propCheckpointScanner)
	if scanner <= 0 {
		return false, nil
	}
	if c.Shop == nil {
		return false, nil
	}
	shopper, ok := c.Actor.(economy.Shopper)
	if !ok {
		return false, nil
	}
	permit, _ := dst.PropertyString(propCheckpointPermit)
	outcome, burned := c.Shop.CheckpointScan(shopper, permit, scanner, shopScanner(c))
	switch outcome {
	case economy.CheckpointOK:
		return false, nil
	case economy.CheckpointNoSIN:
		return true, c.Actor.Write(ctx, "A scanner sweeps you at the checkpoint and finds no valid credentials. The gate stays shut.")
	case economy.CheckpointNoPermit:
		return true, c.Actor.Write(ctx, "The checkpoint scanner reads your papers, but none carries the access this gate demands. It stays shut.")
	case economy.CheckpointBurned:
		// The scan burned the presented fake (an instance property write in the
		// gate); persist it by re-syncing the inventory tree + flipping the dirty
		// bit, so the burn survives a relog (sin-and-legality.md §7).
		c.Actor.MarkContentsDirty()
		reportBurnCrime(ctx, c) // getting caught at a checkpoint is a crime (heat, §7 v2)
		return true, c.Actor.Write(ctx, fmt.Sprintf("The checkpoint scanner flags %s as forged — it voids the credential and turns you back. %s is burned.", burned, capitalize(burned)))
	default:
		return true, c.Actor.Write(ctx, "The checkpoint refuses you.")
	}
}
