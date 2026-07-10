package guard

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Currency must reach players through the currency-label seam
// (economy.CurrencyLabel — c.Money.Format / .Name / .Symbol), never a hardcoded
// literal. A world declares its currency in its pack manifest (nuyen/¥ for
// Shadowrun, gold for the fantasy default), so a baked-in "gold" shows the wrong
// currency the moment a non-fantasy world boots.
//
// This guard scans the in-game, message-emitting source for the two regression
// shapes that are statically decidable:
//
//   - amount+unit — a numeric format verb glued to a currency word ("%d gold",
//     "%d nuyen", "%d¥"). This is the exact bug the seam keeps re-introducing:
//     an amount rendered with a hardcoded unit instead of c.Money.Format(n).
//   - a bare Shadowrun currency token (¥ / "nuyen") hardcoded in prose. Those are
//     always world-specific and must come from the label — a fantasy build should
//     never contain them.
//
// It cannot flag a bare "gold" noun ("your gold"), because in the fantasy-default
// world that IS the correct word and nothing static tells us whether a given line
// is Shadowrun-reachable. Use c.Money.Name() for currency nouns in new code; this
// guard catches the amount case and the reverse (SR token) case, which together
// cover the high-frequency mistakes.
//
// Only STRING LITERALS are inspected (via go/ast), so comments, identifiers,
// struct-tag field names, and the `gold`/`offergold` verb keywords never trip it.
var (
	amountUnitRe = regexp.MustCompile(`(?i)%[-+ #0-9.*]*[dv] ?(gold|nuyen|¥)`)
	bareSRRe     = regexp.MustCompile(`(?i)(¥|\bnuyen\b)`)
)

// scanRoots are the trees walked, relative to this package dir. They cover the
// in-game player-facing emitters (command verbs, the session/notifier layer, the
// trade + auction managers) and the composition root. Deliberately NOT scanned:
// internal/economy (it DEFINES the labels, so "gold"/" gold" literals are correct
// there) and offline tools like cmd/worlddoc.
var scanRoots = []string{
	"../command",
	"../session",
	"../trade",
	"../auction",
	"../../cmd/anothermud",
}

// allowlist maps a "<file-basename>:<line>" anchor to the reason a currency
// literal there is intentional and NOT a player-facing amount display. Empty
// today — the codebase is clean. A genuine exception (a rare admin/debug string)
// adds one documented entry rather than weakening the regexes.
var allowlist = map[string]string{}

// TestCurrencyRegexes pins the guard's own patterns so a future edit can't
// silently defang it (a regex that matches nothing would make the scan above pass
// vacuously). Proves it catches the real bug shapes and spares the legitimate ones.
func TestCurrencyRegexes(t *testing.T) {
	amountHits := []string{
		"You loot %d gold from the corpse", // the reported bug
		"%s adds %d gold to the offer",
		"%d nuyen", "%d¥", "%dgold",
		"%5d gold", "%-3d gold", "Reward: %d GOLD", // flags + case-insensitive
	}
	amountMiss := []string{
		`Keyword: "gold"`, "Show how much gold you carry", // verb keyword + prose noun
		"Gold set to %d.",       // unit precedes the verb — not the amount+unit shape
		"<gold>text</gold>",     // a color tag named gold
		"a hoard of gold coins", // prose, no format verb
	}
	for _, s := range amountHits {
		if !amountUnitRe.MatchString(s) {
			t.Errorf("amountUnitRe should match %q", s)
		}
	}
	for _, s := range amountMiss {
		if amountUnitRe.MatchString(s) {
			t.Errorf("amountUnitRe should NOT match %q", s)
		}
	}

	srHits := []string{"Collect your nuyen at an auctioneer", "725¥", "NUYEN"}
	srMiss := []string{"gold", "your gold", "money", "%d gold"}
	for _, s := range srHits {
		if !bareSRRe.MatchString(s) {
			t.Errorf("bareSRRe should match %q", s)
		}
	}
	for _, s := range srMiss {
		if bareSRRe.MatchString(s) {
			t.Errorf("bareSRRe should NOT match %q", s)
		}
	}
}

func TestNoHardcodedCurrency(t *testing.T) {
	fset := token.NewFileSet()
	for _, root := range scanRoots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			f, perr := parser.ParseFile(fset, path, nil, 0)
			if perr != nil {
				return perr
			}
			ast.Inspect(f, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				val, uerr := strconv.Unquote(lit.Value)
				if uerr != nil {
					val = lit.Value // raw/backtick or odd literal — match the source form
				}
				var why string
				switch {
				case amountUnitRe.MatchString(val):
					why = "an amount is rendered with a hardcoded currency unit"
				case bareSRRe.MatchString(val):
					why = "a Shadowrun currency token (¥/nuyen) is hardcoded"
				default:
					return true
				}
				pos := fset.Position(lit.Pos())
				anchor := filepath.Base(pos.Filename) + ":" + strconv.Itoa(pos.Line)
				if _, ok := allowlist[anchor]; ok {
					return true
				}
				t.Errorf("%s:%d: %s — %q\n  use the currency-label seam (c.Money.Format(n) / c.Money.Name()) instead of a literal; if this is genuinely not a player-facing amount, add %q to the guard allowlist with a reason",
					pos.Filename, pos.Line, why, val, anchor)
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
}
