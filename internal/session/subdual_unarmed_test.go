package session

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/size"
)

// connActor.Stats threads the unarmed-subdual default (subdual-damage §6): an
// UNARMED player's strikes are nonlethal when the host enables it. A wielded
// weapon ignores the unarmed flag — its own `subdual` governs.
func TestConnActor_UnarmedSubdual(t *testing.T) {
	base := map[progression.StatType]int{progression.StatSTR: 10}

	t.Run("unarmed + flag on → subdual", func(t *testing.T) {
		a := &connActor{statBlock: progression.NewWithBase(base), unarmedSubdual: true}
		if !a.Stats().Subdual {
			t.Error("an unarmed player with unarmedSubdual=true should strike subdual")
		}
	})

	t.Run("unarmed + flag off → lethal", func(t *testing.T) {
		a := &connActor{statBlock: progression.NewWithBase(base), unarmedSubdual: false}
		if a.Stats().Subdual {
			t.Error("an unarmed player with unarmedSubdual=false should strike lethally")
		}
	})

	t.Run("wielding a lethal weapon ignores the unarmed flag", func(t *testing.T) {
		a := &connActor{statBlock: progression.NewWithBase(base), unarmedSubdual: true}
		a.weapon.Store(&weaponInfo{dice: combat.DiceExpr{Count: 1, Sides: 8}, name: "a sword", wieldMode: size.OneHanded, subdual: false})
		if a.Stats().Subdual {
			t.Error("a wielded lethal weapon must stay lethal even with unarmedSubdual=true")
		}
	})

	t.Run("wielding a subdual weapon strikes subdual", func(t *testing.T) {
		a := &connActor{statBlock: progression.NewWithBase(base), unarmedSubdual: false}
		a.weapon.Store(&weaponInfo{dice: combat.DiceExpr{Count: 1, Sides: 6}, name: "a sap", wieldMode: size.OneHanded, subdual: true})
		if !a.Stats().Subdual {
			t.Error("a wielded subdual weapon should strike subdual")
		}
	})
}
