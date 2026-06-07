package command

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/render"
	"github.com/Jasrags/AnotherMUD/internal/slot"
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
// alignment, gold, sustenance, trains, AND the full worn-equipment list.
// Self-only; `consider <target>` sizes up others. Rendered as a framed,
// color-tagged bento panel (render.Panel, ui-rendering-help §8); color
// degrades cleanly and the frame is ASCII so no glyph fallback is needed.
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

	// Trains available to spend (training §; same surface the `train` verb
	// reads). Probed via an anonymous interface so the sheet does not pull
	// in the training adapter type — a minimal actor simply omits the row.
	if th, ok := c.Actor.(interface{ TrainsAvailable() int }); ok {
		d.HasTrains = true
		d.Trains = th.TrainsAvailable()
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

	// Equipment: every slot in registration order (folded in from the old
	// standalone view — `eq` still renders the focused list). Occupied
	// slots carry the item name colored by rarity; empties read subtle.
	d.Equip = gatherScoreEquip(c)
	d.HasEquip = len(d.Equip) > 0

	return c.Actor.Write(ctx, renderScore(d))
}

// gatherScoreEquip walks the slot registry in order and returns one row
// per sub-slot, the item name wrapped in its rarity tag (or a subtle
// "(empty)"). Mirrors EquipmentHandler's iteration so the score sheet and
// `eq` show the same slots; returns nil when slots/items are unavailable.
func gatherScoreEquip(c *Context) []equipRow {
	if c.Items == nil || c.Slots == nil {
		return nil
	}
	equipped := c.Actor.Equipment()
	var rows []equipRow
	for _, def := range c.Slots.All() {
		for i := 0; i < def.Max; i++ {
			key, err := slot.BuildKey(def.Name, i, def.Max)
			if err != nil {
				continue
			}
			row := equipRow{Label: def.Label, Name: "<subtle>(empty)</subtle>"}
			if id, ok := equipped[key]; ok {
				if e, ok := c.Items.GetByID(id); ok {
					if inst, ok := e.(*entities.ItemInstance); ok {
						tag := itemRarityTag(inst)
						row.Name = "<" + tag + ">" + inst.Name() + "</" + tag + ">"
					}
				}
			}
			rows = append(rows, row)
		}
	}
	return rows
}

// equipRow is one worn-equipment line: a slot label and the item's
// display name, already wrapped in its color markup by the gatherer.
type equipRow struct {
	Label string
	Name  string
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

	HasTrains bool
	Trains    int

	HasLevel bool
	Track    string
	Level    int
	XP       int64
	XpToNext int64
	AtMax    bool

	HasEquip bool
	Equip    []equipRow
}

// scorePanelWidth is the fixed visible width of the score sheet. 80 is
// the classic terminal width and matches render.DefaultPanelWidth.
const scorePanelWidth = 80

// renderScore formats the sheet as a framed, color-tagged bento panel
// (render.Panel — ASCII frame, tag-aware width math). Pure: every section
// is gated on its Has* flag, so it is unit-testable without an actor. A
// minimal actor (name only) renders as the bare name, no frame.
func renderScore(d scoreData) string {
	if !(d.HasStats || d.HasVitals || d.HasLevel || d.HasEquip) {
		return d.Name
	}

	// Top-left: identity. Top-right: combat + vitals.
	var charCol, combatCol []string
	charCol = append(charCol, scHi(d.Name))
	if identity := strings.TrimSpace(d.Race + " " + d.Class); identity != "" {
		charCol = append(charCol, scHi(identity))
	}
	if d.HasLevel {
		charCol = append(charCol, scHi(fmt.Sprintf("Level %d", d.Level))+" "+scSub("· "+d.Track))
	}
	if d.HasAlign {
		charCol = append(charCol, scKV("Alignment", scHi(fmt.Sprintf("%s (%d)", d.AlignTag, d.Align)), 11))
	}
	if d.HasVitals {
		combatCol = append(combatCol,
			scKV("Armor Class", scHi(strconv.Itoa(d.AC)), 12),
			scKV("Hit Bonus", scHi(fmt.Sprintf("%+d", d.Hit)), 12),
			scKV("HP", scTier(d.HP, d.MaxHP, "hp", fmt.Sprintf("%d / %d", d.HP, d.MaxHP)), 12),
		)
	}
	if d.HasResources {
		// MA/MV are thin pools today (current == max); shown as plain
		// current/max with no bar until live pools land (BACKLOG §2).
		combatCol = append(combatCol, scSub("MA")+" <mana>"+fmt.Sprintf("%d/%d", d.Mana, d.Mana)+
			"</mana>    "+scSub("MV")+" <mv>"+fmt.Sprintf("%d/%d", d.MV, d.MV)+"</mv>")
	}

	// Mid-left: attributes (two per row). Mid-right: purse + training.
	var attrCol, purseCol []string
	if d.HasStats {
		attrCol = append(attrCol,
			scAttr("STR", d.STR, "DEX", d.DEX),
			scAttr("INT", d.INT, "CON", d.CON),
			scAttr("WIS", d.WIS, "LUCK", d.LUCK),
		)
	}
	if d.HasGold {
		purseCol = append(purseCol, scKV("Gold", "<gold>"+commafy(int64(d.Gold))+"</gold>", 12))
	}
	if d.HasSust {
		purseCol = append(purseCol, scKV("Sustenance",
			scTier(d.Sust, economy.MaxSustenance, "good", fmt.Sprintf("%s (%d/%d)", d.SustTier, d.Sust, economy.MaxSustenance)), 12))
	}
	if d.HasTrains {
		tag := "subtle"
		if d.Trains > 0 {
			tag = "good"
		}
		purseCol = append(purseCol, scKV("Trains", "<"+tag+">"+fmt.Sprintf("%d unspent", d.Trains)+"</"+tag+">", 12))
	}

	sections := []render.Section{
		{Rows: append([]render.Row{scTitlePair("Character", "Combat")}, scColRows(charCol, combatCol)...)},
	}
	if len(attrCol) > 0 || len(purseCol) > 0 {
		rows := append([]render.Row{scTitlePair("Attributes", "Purse & Training")}, scColRows(attrCol, purseCol)...)
		sections = append(sections, render.Section{SeparatorAbove: render.RuleMinor, Rows: rows})
	}
	if d.HasEquip {
		rows := []render.Row{render.TitleRow("Equipment", "")}
		for i := 0; i < len(d.Equip); i += 2 {
			left := scEquipCell(d.Equip[i])
			right := ""
			if i+1 < len(d.Equip) {
				right = scEquipCell(d.Equip[i+1])
			}
			rows = append(rows, render.CellRow([]render.Cell{{Content: left, Fill: true}, {Content: right, Fill: true}}, false))
		}
		sections = append(sections, render.Section{SeparatorAbove: render.RuleMajor, Rows: rows})
	}
	if d.HasLevel {
		var xp string
		if d.AtMax {
			xp = fmt.Sprintf("XP %s  (max level)", commafy(d.XP))
		} else {
			xp = fmt.Sprintf("XP %s  (%s to next level)", commafy(d.XP), commafy(d.XpToNext))
		}
		last := &sections[len(sections)-1]
		last.Rows = append(last.Rows, render.FooterRow(xp))
	}

	out, err := render.Panel{Width: scorePanelWidth, Sections: sections}.Render()
	if err != nil {
		// Title overflow is the only error path and can't occur with the
		// short fixed titles here; degrade to the bare name rather than
		// surfacing a render error to the player.
		return d.Name
	}
	return out
}

// scColRows zips two columns of pre-formatted cell content into two-cell
// rows (divider drawn between them), padding the shorter column with
// blank cells so the frame stays rectangular.
func scColRows(left, right []string) []render.Row {
	n := len(left)
	if len(right) > n {
		n = len(right)
	}
	rows := make([]render.Row, 0, n)
	for i := 0; i < n; i++ {
		l, r := "", ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		rows = append(rows, render.CellRow([]render.Cell{{Content: l, Fill: true}, {Content: r, Fill: true}}, true))
	}
	return rows
}

// scTitlePair builds a two-cell row of column headers, each <title>-tagged,
// with a divider between them so the header aligns over the columns below.
func scTitlePair(left, right string) render.Row {
	return render.CellRow([]render.Cell{
		{Content: "<title>" + left + "</title>", Fill: true},
		{Content: "<title>" + right + "</title>", Fill: true},
	}, true)
}

// scKV formats "label<pad>value" with the label subtle and the value
// markup placed at a fixed visible column (labelW), so values line up
// down a column.
func scKV(label, valueMarkup string, labelW int) string {
	pad := labelW - len(label)
	if pad < 1 {
		pad = 1
	}
	return scSub(label) + strings.Repeat(" ", pad) + valueMarkup
}

// scAttr formats two attributes on one line ("STR 16   DEX 14"), the
// first padded to a fixed visible width so the second column aligns.
func scAttr(s1 string, v1 int, s2 string, v2 int) string {
	left := scSub(s1) + " " + scHi(strconv.Itoa(v1))
	vis := len(s1) + 1 + len(strconv.Itoa(v1))
	pad := 9 - vis
	if pad < 1 {
		pad = 1
	}
	return left + strings.Repeat(" ", pad) + scSub(s2) + " " + scHi(strconv.Itoa(v2))
}

// scEquipCell formats one worn-equipment cell: subtle slot label padded
// to a fixed column, then the (already color-wrapped) item name.
func scEquipCell(r equipRow) string {
	if r.Label == "" && r.Name == "" {
		return ""
	}
	pad := 9 - len(r.Label)
	if pad < 1 {
		pad = 1
	}
	return scSub(r.Label) + strings.Repeat(" ", pad) + r.Name
}

// scTier wraps text in a good/warning/danger tag chosen by value/max
// ratio; fullTag names the healthy-tier color (e.g. "hp" or "good").
func scTier(value, max int, fullTag, text string) string {
	tag := fullTag
	if max > 0 {
		switch r := float64(value) / float64(max); {
		case r <= 0.25:
			tag = "danger"
		case r <= 0.5:
			tag = "warning"
		}
	}
	return "<" + tag + ">" + text + "</" + tag + ">"
}

func scHi(s string) string  { return "<highlight>" + s + "</highlight>" }
func scSub(s string) string { return "<subtle>" + s + "</subtle>" }

// commafy formats an integer with thousands separators ("1250" → "1,250").
func commafy(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteByte(s[i])
	}
	if neg {
		return "-" + b.String()
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
