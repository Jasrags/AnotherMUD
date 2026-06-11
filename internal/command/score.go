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
	BackgroundID() string
	Alignment() int
	AlignmentTag() string
	Gold() int
	Sustenance() int
	Mana() int
	Movement() int
	StatValue(progression.StatType) int
	Saves() progression.Saves
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
		d.Background = titleCase(ss.BackgroundID())
		d.HasResources = true
		d.Mana, d.MV = ss.Mana(), ss.Movement()
		d.HasStats = true
		d.STR = ss.StatValue(progression.StatSTR)
		d.INT = ss.StatValue(progression.StatINT)
		d.WIS = ss.StatValue(progression.StatWIS)
		d.DEX = ss.StatValue(progression.StatDEX)
		d.CON = ss.StatValue(progression.StatCON)
		d.LUCK = ss.StatValue(progression.StatLUCK)
		// Saving throws (saves §2/§4): derived from class + ability mods.
		d.HasSaves = true
		sv := ss.Saves()
		d.Fort, d.Reflex, d.Will = sv.Fortitude, sv.Reflex, sv.Will
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
// per sub-slot: the compact slot name ("wield", "light") and the item name
// wrapped in its rarity tag (or a subtle "(empty)"). Both the score sheet
// and `eq` render these rows, so the two views list the same slots in the
// same order with the same labels. Returns nil when slots/items are
// unavailable.
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
			row := equipRow{Slot: def.Name, Name: "<subtle>(empty)</subtle>"}
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

// equipRow is one worn-equipment line: the compact slot name ("wield") and
// the item's display name, already wrapped in its rarity markup by the
// gatherer. Shared by the score sheet's Equipment column and `eq`.
type equipRow struct {
	Slot string
	Name string
}

// scoreData is the rendered character sheet's data, gathered from the
// actor's interfaces. The Has* flags mark which sections the actor could
// supply so renderScore omits the rest (a minimal/test actor shows only
// its name).
type scoreData struct {
	Name        string
	Race, Class string
	Background  string

	HasVitals    bool
	HP, MaxHP    int
	HasResources bool
	Mana, MV     int

	HasStats                      bool
	STR, INT, WIS, DEX, CON, LUCK int
	AC, Hit                       int

	HasSaves           bool
	Fort, Reflex, Will int

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
	if d.Background != "" {
		charCol = append(charCol, scKV("Background", scHi(d.Background), 11))
	}
	if d.HasLevel {
		// ASCII separator (not "·") — panel width math is byte-based, so a
		// multi-byte glyph would over-count this row and drift the border.
		charCol = append(charCol, scHi(fmt.Sprintf("Level %d", d.Level))+" "+scSub("- "+d.Track))
		charCol = append(charCol, scXPLine(d))
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
	if d.HasSaves {
		// Fortitude / Reflex / Will (saves §2). Compact so the row fits the
		// Combat column width; values are signed (a negative ability mod can
		// push a weak save below zero).
		combatCol = append(combatCol, scKV("Saves",
			scHi(fmt.Sprintf("Fort %+d  Ref %+d  Will %+d", d.Fort, d.Reflex, d.Will)), 12))
	}

	// Lower row: three columns — Attributes, Purse & Training, and the worn
	// Equipment (compact slot names). Attributes is fixed-narrow; the other
	// two fill the remainder so neither the sustenance line nor item names
	// truncate (a third equal column would clip both).
	var attrLines, purseLines, equipLines []string
	if d.HasStats {
		attrLines = append(attrLines,
			scAttr("STR", d.STR, "DEX", d.DEX),
			scAttr("INT", d.INT, "CON", d.CON),
			scAttr("WIS", d.WIS, "LUCK", d.LUCK),
		)
	}
	if d.HasGold {
		purseLines = append(purseLines, scKV("Gold", "<gold>"+commafy(int64(d.Gold))+"</gold>", 12))
	}
	if d.HasSust {
		purseLines = append(purseLines, scKV("Sustenance",
			scTier(d.Sust, economy.MaxSustenance, "good", fmt.Sprintf("%s (%d/%d)", d.SustTier, d.Sust, economy.MaxSustenance)), 12))
	}
	if d.HasTrains {
		tag := "subtle"
		if d.Trains > 0 {
			tag = "good"
		}
		purseLines = append(purseLines, scKV("Trains", "<"+tag+">"+fmt.Sprintf("%d unspent", d.Trains)+"</"+tag+">", 12))
	}
	if d.HasEquip {
		for _, r := range d.Equip {
			equipLines = append(equipLines, scEquipCell(r))
		}
	}

	sections := []render.Section{
		{Rows: scColumns([]scCol{
			{title: "Character", lines: charCol},
			{title: "Combat", lines: combatCol},
		})},
	}
	var lower []scCol
	if len(attrLines) > 0 {
		lower = append(lower, scCol{title: "Attributes", width: scAttrColWidth, lines: attrLines})
	}
	if len(purseLines) > 0 {
		lower = append(lower, scCol{title: "Purse & Training", lines: purseLines})
	}
	if len(equipLines) > 0 {
		lower = append(lower, scCol{title: "Equipment", lines: equipLines})
	}
	if len(lower) > 0 {
		sections = append(sections, render.Section{SeparatorAbove: render.RuleMinor, Rows: scColumns(lower)})
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

// scAttrColWidth is the fixed visible width of the Attributes column in
// the lower section. "WIS 10   LUCK 12" (the widest attribute line) is 16
// columns; the Purse and Equipment columns fill the rest of the panel.
const scAttrColWidth = 16

// scCol is one column of a score section: a <title> header, the
// pre-formatted content lines, and a fixed visible width (0 = fill an
// equal share of the row's leftover width).
type scCol struct {
	title string
	width int
	lines []string
}

// scColumns builds a section's rows from N columns: a title row over the
// columns, then one cell-row per content line (shorter columns padded with
// blanks so the frame stays rectangular). The last column always fills so
// the row spans the full panel width regardless of fixed widths before it.
func scColumns(cols []scCol) []render.Row {
	if len(cols) == 0 {
		return nil
	}
	last := len(cols) - 1
	cell := func(j int, content string) render.Cell {
		fill := cols[j].width == 0 || j == last
		w := cols[j].width
		if fill {
			w = 0
		}
		return render.Cell{Content: content, Width: w, Fill: fill}
	}
	n := 0
	for _, c := range cols {
		if len(c.lines) > n {
			n = len(c.lines)
		}
	}
	titleCells := make([]render.Cell, len(cols))
	for j, c := range cols {
		titleCells[j] = cell(j, "<title>"+c.title+"</title>")
	}
	rows := []render.Row{render.CellRow(titleCells, true)}
	for i := 0; i < n; i++ {
		cells := make([]render.Cell, len(cols))
		for j, c := range cols {
			content := ""
			if i < len(c.lines) {
				content = c.lines[i]
			}
			cells[j] = cell(j, content)
		}
		rows = append(rows, render.CellRow(cells, true))
	}
	return rows
}

// scXPLine formats the experience line shown in the Character column:
// current XP plus either the remaining XP to the next level or a max-level
// marker. Plain text reads "XP 12,500  (2,500 to next level)".
func scXPLine(d scoreData) string {
	tail := "(" + commafy(d.XpToNext) + " to next level)"
	if d.AtMax {
		tail = "(max level)"
	}
	return scSub("XP") + " " + scHi(commafy(d.XP)) + "  " + scSub(tail)
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

// scEquipCell formats one worn-equipment cell for the score sheet: the
// subtle short slot name padded to a fixed column, then the (already
// color-wrapped) item name.
func scEquipCell(r equipRow) string {
	if r.Slot == "" && r.Name == "" {
		return ""
	}
	pad := 9 - len(r.Slot)
	if pad < 1 {
		pad = 1
	}
	return scSub(r.Slot) + strings.Repeat(" ", pad) + r.Name
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
