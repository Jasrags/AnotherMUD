package economy

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// CurrencyLabel is a pack's money-DISPLAY vocabulary — the fantasy default
// ("725 gold") vs the Shadowrun reskin ("725¥"). It only changes how a balance
// is printed; the internal ledger is unchanged (a single int balance,
// field-named gold on the holder). Resolved boot-wide from the primary world's
// manifest (character-select §4.1 splash pattern) — co-host / per-world keying
// can layer on later, exactly like the splash and creation-flow seams.
//
// The zero value is usable: its methods fall back to the gold default, so a
// consumer handed an unset label still prints sensibly.
type CurrencyLabel struct {
	// Noun is the currency's name for prose — "gold" → "You have no gold.",
	// "nuyen" → "You have no nuyen." Lowercase; Title upper-cases for headings.
	Noun string
	// Suffix is appended to an amount for compact display, INCLUDING any leading
	// space the format wants: " gold" → "725 gold"; "¥" → "725¥".
	Suffix string
}

// DefaultCurrency is the fantasy baseline every pack inherits unless it declares
// its own `currency:` manifest block.
var DefaultCurrency = CurrencyLabel{Noun: "gold", Suffix: " gold"}

// Name returns the currency noun for prose, defaulting to gold.
func (c CurrencyLabel) Name() string {
	if c.Noun == "" {
		return DefaultCurrency.Noun
	}
	return c.Noun
}

// suffix returns the amount suffix, defaulting to the gold form.
func (c CurrencyLabel) suffix() string {
	if c.Suffix == "" {
		return DefaultCurrency.Suffix
	}
	return c.Suffix
}

// Amount formats n for display: "725 gold" or "725¥". The amount string is
// passed in already-formatted so callers that want thousands separators can
// supply their own; use Format for the plain integer form.
func (c CurrencyLabel) Amount(amount string) string {
	return amount + c.suffix()
}

// Format is the convenience integer form: Format(725) → "725¥" / "725 gold".
func (c CurrencyLabel) Format(n int) string {
	return c.Format64(int64(n))
}

// Format64 is Format for int64 amounts (shop prices are int64) — it avoids
// narrowing an int64 balance through int on 32-bit targets.
func (c CurrencyLabel) Format64(n int64) string {
	return c.Amount(fmt.Sprintf("%d", n))
}

// Symbol returns the compact unit to hug an amount WHERE A NOUN LABEL IS ALREADY
// PRESENT (the score purse row, headed "Nuyen"/"Gold"). A spelled-out unit — one
// written with a leading space, " gold" — would be redundant beside such a label,
// so it collapses to "" (the row shows just the number). A symbol unit — "¥",
// written hugging the number with no leading space — is kept ("1,250¥"). This is
// the one heuristic the seam leans on: a leading space marks a word, its absence
// marks a symbol.
func (c CurrencyLabel) Symbol() string {
	s := c.suffix()
	if strings.HasPrefix(s, " ") {
		return ""
	}
	return s
}

// Title returns the currency noun with its first letter upper-cased, for a
// heading like the score sheet's "Nuyen" / "Gold" purse label.
func (c CurrencyLabel) Title() string {
	name := c.Name()
	r, size := utf8.DecodeRuneInString(name)
	if r == utf8.RuneError {
		return name
	}
	return string(unicode.ToUpper(r)) + name[size:]
}
