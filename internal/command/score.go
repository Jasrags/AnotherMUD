package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// scoreSubject is the identity/resource read surface the score sheet
// needs beyond the base Actor (vitals come from combat.Combatant, level
// from ProgressionHolder). The production connActor satisfies it; tests
// use a fake. The handler type-asserts and renders whatever is present,
// so a minimal actor degrades to just its name rather than erroring.
type scoreSubject interface {
	RaceID() string
	ClassID() string
	Alignment() int
	AlignmentTag() string
	Gold() int
	Sustenance() int
	Mana() int
	Movement() int
	StatValue(progression.StatType) int
}

// ScoreHandler implements `score` (aliased `sc`) — the player's character
// sheet: identity, level/track, vitals, the six attributes, AC/hit,
// alignment, gold, and sustenance. Self-only; `consider <target>` sizes
// up others. Renders plain aligned lines (works on every client).
func ScoreHandler(ctx context.Context, c *Context) error {
	d := scoreData{Name: c.Actor.Name()}

	if ss, ok := c.Actor.(scoreSubject); ok {
		d.Race = titleCase(ss.RaceID())
		d.Class = titleCase(ss.ClassID())
		d.HasResources = true
		d.Mana, d.MV = ss.Mana(), ss.Movement()
		d.HasStats = true
		d.STR = ss.StatValue(progression.StatSTR)
		d.INT = ss.StatValue(progression.StatINT)
		d.WIS = ss.StatValue(progression.StatWIS)
		d.DEX = ss.StatValue(progression.StatDEX)
		d.CON = ss.StatValue(progression.StatCON)
		d.LUCK = ss.StatValue(progression.StatLUCK)
		d.HasAlign = true
		// AlignmentTag returns the raw tag id ("alignment_neutral"); show
		// the bare word ("neutral") on the sheet.
		d.Align = ss.Alignment()
		d.AlignTag = strings.TrimPrefix(ss.AlignmentTag(), "alignment_")
		d.HasGold = true
		d.Gold = ss.Gold()
		d.HasSust = true
		d.Sust = ss.Sustenance()
		// Tier thresholds aren't externally configurable (only the drain
		// rate is — Phase 2), so the default config's tiers match the live
		// service. If thresholds ever become configurable, read them off a
		// threaded SustenanceService instead.
		d.SustTier = titleCase(string(economy.DefaultSustenanceConfig().TierOf(d.Sust)))
	}

	if cb, ok := c.Actor.(combat.Combatant); ok {
		d.HasVitals = true
		d.HP, d.MaxHP = cb.Vitals().Snapshot()
		st := cb.Stats()
		d.AC, d.Hit = st.AC, st.HitMod
	}

	if ph, ok := c.Actor.(ProgressionHolder); ok && c.Progression != nil {
		// Primary track = the first registered track the actor has info
		// for (adventure today; the score sheet shows one headline level).
		for _, td := range c.Progression.Tracks().All() {
			info, ok := ph.TrackInfo(c.Progression, td.Name)
			if !ok {
				continue
			}
			d.HasLevel = true
			d.Track = td.DisplayName
			if d.Track == "" {
				d.Track = td.Name
			}
			d.Level, d.XP, d.XpToNext = info.Level, info.XP, info.XpToNext
			d.AtMax = info.MaxLevel > 0 && info.Level >= info.MaxLevel
			break
		}
	}

	return c.Actor.Write(ctx, renderScore(d))
}

// scoreData is the rendered character sheet's data, gathered from the
// actor's interfaces. The Has* flags mark which sections the actor could
// supply so renderScore omits the rest (a minimal/test actor shows only
// its name).
type scoreData struct {
	Name        string
	Race, Class string

	HasVitals    bool
	HP, MaxHP    int
	HasResources bool
	Mana, MV     int

	HasStats                      bool
	STR, INT, WIS, DEX, CON, LUCK int
	AC, Hit                       int

	HasAlign bool
	AlignTag string
	Align    int

	HasGold bool
	Gold    int

	HasSust  bool
	Sust     int
	SustTier string

	HasLevel bool
	Track    string
	Level    int
	XP       int64
	XpToNext int64
	AtMax    bool
}

// renderScore formats the sheet as plain aligned lines (spec
// ui-rendering-help — score is a player status surface). Pure: every
// section is gated on its Has* flag, so it is unit-testable without an
// actor.
func renderScore(d scoreData) string {
	var b strings.Builder
	b.WriteString(d.Name)

	// Identity + headline level.
	identity := strings.TrimSpace(d.Race + " " + d.Class)
	if identity != "" || d.HasLevel {
		b.WriteByte('\n')
		b.WriteString(identity)
		if d.HasLevel {
			if identity != "" {
				b.WriteString(" — ")
			}
			fmt.Fprintf(&b, "level %d (%s)", d.Level, d.Track)
		}
	}

	// Vitals: HP always when present; MA/MV appended when the actor has
	// resource pools (thin pools today — current == max).
	if d.HasVitals {
		fmt.Fprintf(&b, "\nHP %d/%d", d.HP, d.MaxHP)
		if d.HasResources {
			fmt.Fprintf(&b, "   MA %d/%d   MV %d/%d", d.Mana, d.Mana, d.MV, d.MV)
		}
	}

	// Attributes + combat.
	if d.HasStats {
		fmt.Fprintf(&b, "\nSTR %d  INT %d  WIS %d  DEX %d  CON %d  LUCK %d",
			d.STR, d.INT, d.WIS, d.DEX, d.CON, d.LUCK)
		fmt.Fprintf(&b, "\nAC %d   Hit %+d", d.AC, d.Hit)
	}

	// Alignment + gold on one line.
	if d.HasAlign || d.HasGold {
		b.WriteByte('\n')
		if d.HasAlign {
			fmt.Fprintf(&b, "Alignment %s (%d)", d.AlignTag, d.Align)
		}
		if d.HasGold {
			if d.HasAlign {
				b.WriteString("    ")
			}
			fmt.Fprintf(&b, "Gold %d", d.Gold)
		}
	}

	if d.HasSust {
		fmt.Fprintf(&b, "\nSustenance: %s (%d/%d)", d.SustTier, d.Sust, economy.MaxSustenance)
	}

	if d.HasLevel {
		if d.AtMax {
			fmt.Fprintf(&b, "\nXP %d   (max level)", d.XP)
		} else {
			fmt.Fprintf(&b, "\nXP %d   (%d to next level)", d.XP, d.XpToNext)
		}
	}

	return b.String()
}

// titleCase upper-cases the first rune of s (for race/class ids and the
// sustenance tier word). Leaves the rest untouched — these are single
// lowercase tokens like "human" / "fighter" / "full".
func titleCase(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}
