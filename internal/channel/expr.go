// Package channel is the derived-channel layer: a fixed engine-consumed
// vocabulary (attack, defense, damage_bonus, …) that content fills from
// its own arbitrary attribute set via per-pack formulas. The kernel reads
// channels, never an attribute name — so a generic-RPG, Wheel-of-Time, and
// Shadowrun pack can feed the same resolution kernel from different stats
// (design: docs/themes/channel-vocabulary.md §3/§7).
//
// expr.go is the formula evaluator: a small, dependency-free arithmetic
// language over a stat-lookup function. It is deliberately hand-rolled and
// NOT a general expression library — content authors write these formulas,
// so the surface must have zero code-execution capability: only numeric
// literals, attribute names (resolved through the injected lookup),
// arithmetic (+ - * /), parentheses, and a fixed whitelist of pure
// functions (mod, floor, ceil, abs, min, max). No loops, no assignment, no
// arbitrary calls. Evaluation is bounded by the parsed AST depth, which is
// bounded by the (length-capped) source.
package channel

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// MaxFormulaLen caps a formula's source length. A channel formula is a
// short arithmetic expression; the cap is a defensive bound on parser
// recursion depth from pathological (deeply-nested) content input.
const MaxFormulaLen = 512

// ErrEmptyFormula is returned by Parse for blank source.
var ErrEmptyFormula = errors.New("channel: empty formula")

// Expr is a parsed, reusable channel formula. Parse once at pack load;
// Eval many times per entity. Safe for concurrent Eval (the AST is
// immutable; the lookup is the caller's to make safe).
type Expr struct {
	root node
	src  string
}

// String returns the original formula source (for diagnostics/round-trip).
func (e Expr) String() string { return e.src }

// Eval computes the formula's value against lookup, which resolves an
// attribute name to its effective value (0 for unknown names — an unset
// attribute reads as zero, matching StatBlock.Effective). The arithmetic
// runs in float64 (so `body/2` and ceil/floor behave); the result is
// rounded half-away-from-zero to the int a channel consumer expects.
func (e Expr) Eval(lookup func(name string) int) int {
	if e.root == nil {
		return 0
	}
	v := e.root.eval(lookup)
	return int(math.Round(v))
}

// Parse compiles a formula. Stat names are lowercased on parse so a
// formula's `Reaction` matches a StatBlock keyed by `reaction`.
func Parse(src string) (Expr, error) {
	if len(src) > MaxFormulaLen {
		return Expr{}, fmt.Errorf("channel: formula exceeds %d chars", MaxFormulaLen)
	}
	if strings.TrimSpace(src) == "" {
		return Expr{}, ErrEmptyFormula
	}
	p := &parser{lex: newLexer(src)}
	if err := p.advance(); err != nil {
		return Expr{}, err
	}
	root, err := p.parseExpr()
	if err != nil {
		return Expr{}, err
	}
	if p.cur.kind != tokEOF {
		return Expr{}, fmt.Errorf("channel: unexpected %q at end of formula", p.cur.text)
	}
	return Expr{root: root, src: strings.TrimSpace(src)}, nil
}

// MustParse is Parse that panics on error — for engine-defined formulas
// (tests, baked-in defaults) where a malformed literal is a programmer
// bug, never content input.
func MustParse(src string) Expr {
	e, err := Parse(src)
	if err != nil {
		panic("channel.MustParse(" + src + "): " + err.Error())
	}
	return e
}

// --- AST ---------------------------------------------------------------

type node interface {
	eval(lookup func(string) int) float64
}

type numNode float64

func (n numNode) eval(func(string) int) float64 { return float64(n) }

type varNode string

func (n varNode) eval(lookup func(string) int) float64 {
	if lookup == nil {
		return 0
	}
	return float64(lookup(string(n)))
}

type binNode struct {
	op   byte
	l, r node
}

func (n binNode) eval(lookup func(string) int) float64 {
	l, r := n.l.eval(lookup), n.r.eval(lookup)
	switch n.op {
	case '+':
		return l + r
	case '-':
		return l - r
	case '*':
		return l * r
	case '/':
		if r == 0 {
			return 0 // division by zero yields 0 rather than NaN/Inf
		}
		return l / r
	}
	return 0
}

type negNode struct{ n node }

func (n negNode) eval(lookup func(string) int) float64 { return -n.n.eval(lookup) }

type callNode struct {
	fn   string
	args []node
}

func (n callNode) eval(lookup func(string) int) float64 {
	switch n.fn {
	case "mod": // d20 ability modifier: floor((x-10)/2)
		return math.Floor((n.args[0].eval(lookup) - 10) / 2)
	case "floor":
		return math.Floor(n.args[0].eval(lookup))
	case "ceil":
		return math.Ceil(n.args[0].eval(lookup))
	case "trunc": // round toward zero — matches Go integer division
		return math.Trunc(n.args[0].eval(lookup))
	case "abs":
		return math.Abs(n.args[0].eval(lookup))
	case "min":
		out := n.args[0].eval(lookup)
		for _, a := range n.args[1:] {
			out = math.Min(out, a.eval(lookup))
		}
		return out
	case "max":
		out := n.args[0].eval(lookup)
		for _, a := range n.args[1:] {
			out = math.Max(out, a.eval(lookup))
		}
		return out
	}
	return 0
}

// funcArity is the fixed whitelist of callable functions and their
// required argument counts (-1 = variadic, min 1). A name absent here is
// a parse error — there is no way for content to reach an arbitrary call.
var funcArity = map[string]int{
	"mod": 1, "floor": 1, "ceil": 1, "trunc": 1, "abs": 1,
	"min": -1, "max": -1,
}

// --- lexer -------------------------------------------------------------

type tokKind int

const (
	tokEOF tokKind = iota
	tokNum
	tokIdent
	tokOp     // + - * /
	tokLParen // (
	tokRParen // )
	tokComma  // ,
)

type token struct {
	kind tokKind
	text string
	num  float64
}

type lexer struct {
	src []rune
	pos int
}

func newLexer(src string) *lexer { return &lexer{src: []rune(src)} }

func (l *lexer) next() (token, error) {
	for l.pos < len(l.src) && unicode.IsSpace(l.src[l.pos]) {
		l.pos++
	}
	if l.pos >= len(l.src) {
		return token{kind: tokEOF}, nil
	}
	c := l.src[l.pos]
	switch {
	case c == '(':
		l.pos++
		return token{kind: tokLParen, text: "("}, nil
	case c == ')':
		l.pos++
		return token{kind: tokRParen, text: ")"}, nil
	case c == ',':
		l.pos++
		return token{kind: tokComma, text: ","}, nil
	case c == '+' || c == '-' || c == '*' || c == '/':
		l.pos++
		return token{kind: tokOp, text: string(c)}, nil
	case unicode.IsDigit(c) || c == '.':
		return l.lexNumber()
	case isIdentStart(c):
		return l.lexIdent()
	}
	return token{}, fmt.Errorf("channel: unexpected character %q in formula", string(c))
}

func (l *lexer) lexNumber() (token, error) {
	start := l.pos
	for l.pos < len(l.src) && (unicode.IsDigit(l.src[l.pos]) || l.src[l.pos] == '.') {
		l.pos++
	}
	text := string(l.src[start:l.pos])
	v, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return token{}, fmt.Errorf("channel: bad number %q", text)
	}
	return token{kind: tokNum, text: text, num: v}, nil
}

func (l *lexer) lexIdent() (token, error) {
	start := l.pos
	for l.pos < len(l.src) && isIdentPart(l.src[l.pos]) {
		l.pos++
	}
	// Stat/function names are case-insensitive; lowercased so a formula's
	// `Reaction` matches a StatBlock keyed by `reaction`.
	return token{kind: tokIdent, text: strings.ToLower(string(l.src[start:l.pos]))}, nil
}

func isIdentStart(c rune) bool { return unicode.IsLetter(c) || c == '_' }
func isIdentPart(c rune) bool  { return unicode.IsLetter(c) || unicode.IsDigit(c) || c == '_' }

// --- parser (recursive descent) ----------------------------------------

type parser struct {
	lex *lexer
	cur token
}

func (p *parser) advance() error {
	t, err := p.lex.next()
	if err != nil {
		return err
	}
	p.cur = t
	return nil
}

// expr := term (('+'|'-') term)*
func (p *parser) parseExpr() (node, error) {
	left, err := p.parseTerm()
	if err != nil {
		return nil, err
	}
	for p.cur.kind == tokOp && (p.cur.text == "+" || p.cur.text == "-") {
		op := p.cur.text[0]
		if err := p.advance(); err != nil {
			return nil, err
		}
		right, err := p.parseTerm()
		if err != nil {
			return nil, err
		}
		left = binNode{op: op, l: left, r: right}
	}
	return left, nil
}

// term := factor (('*'|'/') factor)*
func (p *parser) parseTerm() (node, error) {
	left, err := p.parseFactor()
	if err != nil {
		return nil, err
	}
	for p.cur.kind == tokOp && (p.cur.text == "*" || p.cur.text == "/") {
		op := p.cur.text[0]
		if err := p.advance(); err != nil {
			return nil, err
		}
		right, err := p.parseFactor()
		if err != nil {
			return nil, err
		}
		left = binNode{op: op, l: left, r: right}
	}
	return left, nil
}

// factor := number | ident | ident '(' args ')' | '(' expr ')' | '-' factor
func (p *parser) parseFactor() (node, error) {
	switch p.cur.kind {
	case tokNum:
		n := numNode(p.cur.num)
		return n, p.advance()
	case tokOp:
		if p.cur.text == "-" {
			if err := p.advance(); err != nil {
				return nil, err
			}
			inner, err := p.parseFactor()
			if err != nil {
				return nil, err
			}
			return negNode{n: inner}, nil
		}
		return nil, fmt.Errorf("channel: unexpected operator %q", p.cur.text)
	case tokLParen:
		if err := p.advance(); err != nil {
			return nil, err
		}
		inner, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if p.cur.kind != tokRParen {
			return nil, errors.New("channel: missing closing ')'")
		}
		return inner, p.advance()
	case tokIdent:
		name := p.cur.text
		if err := p.advance(); err != nil {
			return nil, err
		}
		if p.cur.kind == tokLParen {
			return p.parseCall(name)
		}
		return varNode(name), nil
	}
	return nil, fmt.Errorf("channel: unexpected token %q", p.cur.text)
}

func (p *parser) parseCall(name string) (node, error) {
	arity, ok := funcArity[name]
	if !ok {
		return nil, fmt.Errorf("channel: unknown function %q", name)
	}
	if err := p.advance(); err != nil { // consume '('
		return nil, err
	}
	var args []node
	if p.cur.kind != tokRParen {
		for {
			arg, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			args = append(args, arg)
			if p.cur.kind != tokComma {
				break
			}
			if err := p.advance(); err != nil {
				return nil, err
			}
		}
	}
	if p.cur.kind != tokRParen {
		return nil, fmt.Errorf("channel: missing ')' in call to %q", name)
	}
	if err := p.advance(); err != nil {
		return nil, err
	}
	if arity == -1 {
		if len(args) < 1 {
			return nil, fmt.Errorf("channel: %q needs at least one argument", name)
		}
	} else if len(args) != arity {
		return nil, fmt.Errorf("channel: %q takes %d argument(s), got %d", name, arity, len(args))
	}
	return callNode{fn: name, args: args}, nil
}
