package command

import "strings"

// DefaultChainCap bounds how many commands one input line can expand to via
// chaining (§4.1) and repeat (§4.2). A line of two hundred semicolons, or a
// `999n`, cannot produce more than this many commands. Callers pass their
// own cap; ParseInput falls back to this when given a non-positive one.
const DefaultChainCap = 10

// maxRepeatCount is the count assigned when the leading-digit string of a
// repeat token overflows int parsing — large enough that the chain cap is
// what actually bounds the expansion, never the parsed value.
const maxRepeatCount = 1 << 30

// ParseInput splits a raw input line into an ordered list of expanded
// command segments (commands-and-dispatch §4). Each returned string is a
// ready-to-dispatch command line; under §4.3's no-quoting rule a segment is
// a lossless serialization of its (verb, args) pair, so the caller can feed
// each straight to Dispatch.
//
//   - §4.1: the line is split on `;`; each segment is trimmed and parsed
//     independently; empty segments are dropped; the total is bounded by
//     cap (trailing segments past the cap are dropped silently).
//   - §4.2: within a segment, a first token of leading digits + a non-digit
//     suffix (`3n`, `12east`, `2pick`) expands into count copies of the
//     suffix-verb, bounded by the remaining cap. A pure-digit token (`3`)
//     is NOT expanded. Count zero runs once.
//   - §4.3: each segment is space-split with empty tokens dropped; no
//     quoting or escaping is interpreted.
func ParseInput(raw string, cap int) []string {
	if cap <= 0 {
		cap = DefaultChainCap
	}
	out := make([]string, 0, cap)
	for _, segment := range strings.Split(raw, ";") {
		if len(out) >= cap {
			break // §4.1 cap reached — drop the rest silently.
		}
		fields := strings.Fields(segment) // §4.3 space-split, empties dropped.
		if len(fields) == 0 {
			continue
		}

		count, verb, isRepeat := splitRepeat(fields[0])
		if !isRepeat {
			out = append(out, strings.Join(fields, " "))
			continue
		}

		// §4.2 repeat: rebuild the segment with the suffix-verb in place of
		// the count-prefixed token, then emit count copies (count 0 → 1),
		// bounded by what's left under the cap.
		line := verb
		if len(fields) > 1 {
			line = verb + " " + strings.Join(fields[1:], " ")
		}
		n := count
		if n < 1 {
			n = 1
		}
		if remaining := cap - len(out); n > remaining {
			n = remaining
		}
		for i := 0; i < n; i++ {
			out = append(out, line)
		}
	}
	return out
}

// splitRepeat splits a token of the form <digits><non-digit-suffix> into its
// numeric count and suffix verb. isRepeat is false for a token with no
// leading digit, or one that is all digits (no suffix) — the latter passes
// through as an ordinary verb (§4.2: `3` is not expanded). An overflowing
// digit run yields maxRepeatCount so the chain cap, not the parsed number,
// bounds the expansion.
func splitRepeat(token string) (count int, verb string, isRepeat bool) {
	i := 0
	for i < len(token) && token[i] >= '0' && token[i] <= '9' {
		i++
	}
	if i == 0 || i == len(token) {
		return 0, "", false
	}
	n := 0
	for _, d := range token[:i] {
		// Clamp BEFORE the multiply so the accumulator can never overflow
		// int (even on a 32-bit target) — the parsed value only needs to
		// exceed any sane chain cap, which maxRepeatCount does by far.
		if n >= maxRepeatCount/10 {
			n = maxRepeatCount
			break
		}
		n = n*10 + int(d-'0')
	}
	return n, token[i:], true
}
