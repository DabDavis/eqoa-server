// Package formula is a small, safe arithmetic language for formulas an
// operator keeps outside the repository (entity/dmgformula.go: the damage
// formulas of combat.formula_file).
//
// A formula is one expression over float64 numbers and named variables:
// + - * / % ^ (power), comparisons (== != < <= > >=, 1 or 0), && || ! (any
// nonzero is true), c ? a : b, parentheses, and a fixed set of pure functions
// (min, max, clamp, abs, floor, ceil, round, trunc, sqrt, pow, log, log10,
// exp, sign, if). There are no assignments, loops, strings, calls out of the
// package, or any I/O: a compiled formula is a tree of closures that runs in
// time bounded by its size, which compiling bounds (MaxSource, MaxDepth,
// MaxNodes).
//
// Compile errors carry a line and column and a description of what was wrong,
// never the formula's own text (a name it did not know included): the
// formulas may be private, and an error is logged.
//
// It is hand-written rather than a general expression library (expr-lang/expr
// was the candidate) because: the library's errors quote the source with a
// caret under it, which would put a private formula in the log; damage is all
// float64 arithmetic, so a typed, dynamic language buys nothing; and a closure
// tree over a []float64 evaluates a typical formula in tens of nanoseconds
// with no allocation, per swing of thousands of NPCs.
package formula

import (
	"fmt"
	"math"
	"strconv"
)

// The bounds on a formula.
const (
	MaxSource = 8192 // bytes of one formula
	MaxDepth  = 64   // nesting of its expression
	MaxNodes  = 2000 // operators, numbers and names in it
	maxArgs   = 16   // a function's arguments
)

// Error is a compile error: where, and what kind of mistake, without the
// formula's text.
type Error struct {
	Line, Col int // 1-based; Line 0 when the formula is a single line
	Msg       string
}

func (e *Error) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("line %d, column %d: %s", e.Line, e.Col, e.Msg)
	}
	return fmt.Sprintf("column %d: %s", e.Col, e.Msg)
}

// Program is a compiled formula.
type Program struct {
	eval  func(v []float64) float64
	uses  []int // the variables it reads, each once
	nvars int   // the length Eval's slice must have at least
	konst bool  // it reads no variable
}

// Eval runs the formula on the variables (indexed as Compile's vars map
// says; v must be at least as long as that map's highest index + 1, which
// Eval checks, returning NaN rather than panicking). It may return NaN or an
// infinity (0/0, sqrt(-1), log(0), ...): the caller decides what such a
// result means.
func (p *Program) Eval(v []float64) float64 {
	if p == nil || len(v) < p.nvars {
		return math.NaN()
	}
	return p.eval(v)
}

// Uses is the indices of the variables the formula reads, each once.
func (p *Program) Uses() []int { return p.uses }

// Constant is whether the formula reads no variable.
func (p *Program) Constant() bool { return p.konst }

// Compile compiles one formula. vars names the variables it may read and
// their index in the slice Eval is given. A name not in vars, a function not
// known, a syntax error or a formula past the bounds is an *Error.
func Compile(src string, vars map[string]int) (*Program, error) {
	if len(src) > MaxSource {
		return nil, &Error{Col: 1, Msg: fmt.Sprintf("formula longer than %d bytes", MaxSource)}
	}
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks, vars: vars, used: map[int]bool{}}
	n, err := p.ternary()
	if err != nil {
		return nil, err
	}
	if t := p.peek(); t.kind != tkEOF {
		return nil, p.errAt(t, "unexpected "+t.what())
	}
	prog := &Program{eval: n.fn, konst: n.konst}
	for i := range p.used {
		prog.uses = append(prog.uses, i)
		prog.nvars = max(prog.nvars, i+1)
	}
	return prog, nil
}

// --- lexer ---

type tokKind int

const (
	tkEOF tokKind = iota
	tkNum
	tkName
	tkOp
)

type token struct {
	kind      tokKind
	op        string
	num       float64
	name      string
	line, col int
}

// what describes a token in an error, never quoting a name or a number.
func (t token) what() string {
	switch t.kind {
	case tkEOF:
		return "end of formula"
	case tkNum:
		return "number"
	case tkName:
		return "name"
	}
	return "operator " + strconv.Quote(t.op) // operators are the language's, not the formula's
}

var twoCharOps = map[string]bool{"==": true, "!=": true, "<=": true, ">=": true, "&&": true, "||": true}

const oneCharOps = "+-*/%^()<>!?:,"

func lex(src string) ([]token, error) {
	var toks []token
	line, col := 1, 1
	for i := 0; i < len(src); {
		c := src[i]
		start := col
		switch {
		case c == '\n':
			line, col = line+1, 1
			i++
			continue
		case c == ' ' || c == '\t' || c == '\r':
			i++
			col++
			continue
		case c >= '0' && c <= '9' || c == '.':
			j := i
			for j < len(src) && (src[j] >= '0' && src[j] <= '9' || src[j] == '.') {
				j++
			}
			if j < len(src) && (src[j] == 'e' || src[j] == 'E') {
				k := j + 1
				if k < len(src) && (src[k] == '+' || src[k] == '-') {
					k++
				}
				if k < len(src) && src[k] >= '0' && src[k] <= '9' {
					for k < len(src) && src[k] >= '0' && src[k] <= '9' {
						k++
					}
					j = k
				}
			}
			f, err := strconv.ParseFloat(src[i:j], 64)
			if err != nil || math.IsInf(f, 0) {
				return nil, &Error{Line: line, Col: start, Msg: "malformed number"}
			}
			toks = append(toks, token{kind: tkNum, num: f, line: line, col: start})
			col += j - i
			i = j
		case c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
			j := i
			for j < len(src) && (src[j] == '_' || src[j] >= 'a' && src[j] <= 'z' || src[j] >= 'A' && src[j] <= 'Z' || src[j] >= '0' && src[j] <= '9') {
				j++
			}
			toks = append(toks, token{kind: tkName, name: src[i:j], line: line, col: start})
			col += j - i
			i = j
		default:
			if i+1 < len(src) && twoCharOps[src[i:i+2]] {
				toks = append(toks, token{kind: tkOp, op: src[i : i+2], line: line, col: start})
				i += 2
				col += 2
				continue
			}
			found := false
			for k := 0; k < len(oneCharOps); k++ {
				if oneCharOps[k] == c {
					found = true
					break
				}
			}
			if !found {
				return nil, &Error{Line: line, Col: start, Msg: "character not allowed in a formula"}
			}
			toks = append(toks, token{kind: tkOp, op: string(c), line: line, col: start})
			i++
			col++
		}
		if len(toks) > MaxNodes*2 {
			return nil, &Error{Line: line, Col: start, Msg: fmt.Sprintf("formula has more than %d tokens", MaxNodes*2)}
		}
	}
	toks = append(toks, token{kind: tkEOF, line: line, col: col})
	return toks, nil
}

// --- parser: precedence climbing into closures ---

type node struct {
	fn    func(v []float64) float64
	konst bool
}

func constNode(x float64) node {
	return node{fn: func([]float64) float64 { return x }, konst: true}
}

// fold makes n a constant when every input of it was.
func fold(n node, inputsConst bool) node {
	if inputsConst {
		return constNode(n.fn(nil))
	}
	return n
}

type parser struct {
	toks  []token
	pos   int
	depth int
	nodes int
	vars  map[string]int
	used  map[int]bool
}

func (p *parser) peek() token { return p.toks[p.pos] }
func (p *parser) next() token {
	t := p.toks[p.pos]
	if t.kind != tkEOF {
		p.pos++
	}
	return t
}

func (p *parser) isOp(op string) bool {
	t := p.peek()
	return t.kind == tkOp && t.op == op
}

func (p *parser) errAt(t token, msg string) error {
	return &Error{Line: t.line, Col: t.col, Msg: msg}
}

func (p *parser) enter() error {
	p.depth++
	p.nodes++
	if p.depth > MaxDepth {
		return p.errAt(p.peek(), fmt.Sprintf("formula nested deeper than %d", MaxDepth))
	}
	if p.nodes > MaxNodes {
		return p.errAt(p.peek(), fmt.Sprintf("formula has more than %d parts", MaxNodes))
	}
	return nil
}

func (p *parser) leave() { p.depth-- }

func truth(x float64) bool { return x != 0 && !math.IsNaN(x) }

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func (p *parser) ternary() (node, error) {
	if err := p.enter(); err != nil {
		return node{}, err
	}
	defer p.leave()
	c, err := p.or()
	if err != nil || !p.isOp("?") {
		return c, err
	}
	p.next()
	a, err := p.ternary()
	if err != nil {
		return node{}, err
	}
	if !p.isOp(":") {
		return node{}, p.errAt(p.peek(), "expected \":\" of \"?\"")
	}
	p.next()
	b, err := p.ternary()
	if err != nil {
		return node{}, err
	}
	cf, af, bf := c.fn, a.fn, b.fn
	return fold(node{fn: func(v []float64) float64 {
		if truth(cf(v)) {
			return af(v)
		}
		return bf(v)
	}}, c.konst && a.konst && b.konst), nil
}

func (p *parser) or() (node, error) {
	l, err := p.and()
	for err == nil && p.isOp("||") {
		p.next()
		var r node
		if r, err = p.and(); err != nil {
			break
		}
		lf, rf := l.fn, r.fn
		l = fold(node{fn: func(v []float64) float64 { return b2f(truth(lf(v)) || truth(rf(v))) }}, l.konst && r.konst)
	}
	return l, err
}

func (p *parser) and() (node, error) {
	l, err := p.cmp()
	for err == nil && p.isOp("&&") {
		p.next()
		var r node
		if r, err = p.cmp(); err != nil {
			break
		}
		lf, rf := l.fn, r.fn
		l = fold(node{fn: func(v []float64) float64 { return b2f(truth(lf(v)) && truth(rf(v))) }}, l.konst && r.konst)
	}
	return l, err
}

var cmpOps = map[string]func(a, b float64) bool{
	"==": func(a, b float64) bool { return a == b },
	"!=": func(a, b float64) bool { return a != b },
	"<":  func(a, b float64) bool { return a < b },
	"<=": func(a, b float64) bool { return a <= b },
	">":  func(a, b float64) bool { return a > b },
	">=": func(a, b float64) bool { return a >= b },
}

func (p *parser) cmp() (node, error) {
	l, err := p.add()
	for err == nil {
		t := p.peek()
		op, ok := cmpOps[t.op]
		if t.kind != tkOp || !ok {
			break
		}
		p.next()
		var r node
		if r, err = p.add(); err != nil {
			break
		}
		lf, rf := l.fn, r.fn
		l = fold(node{fn: func(v []float64) float64 { return b2f(op(lf(v), rf(v))) }}, l.konst && r.konst)
	}
	return l, err
}

func (p *parser) add() (node, error) {
	l, err := p.mul()
	for err == nil && (p.isOp("+") || p.isOp("-")) {
		op := p.next().op
		var r node
		if r, err = p.mul(); err != nil {
			break
		}
		lf, rf := l.fn, r.fn
		var n node
		if op == "+" {
			n.fn = func(v []float64) float64 { return lf(v) + rf(v) }
		} else {
			n.fn = func(v []float64) float64 { return lf(v) - rf(v) }
		}
		l = fold(n, l.konst && r.konst)
	}
	return l, err
}

func (p *parser) mul() (node, error) {
	l, err := p.unary()
	for err == nil && (p.isOp("*") || p.isOp("/") || p.isOp("%")) {
		op := p.next().op
		var r node
		if r, err = p.unary(); err != nil {
			break
		}
		lf, rf := l.fn, r.fn
		var n node
		switch op {
		case "*":
			n.fn = func(v []float64) float64 { return lf(v) * rf(v) }
		case "/":
			n.fn = func(v []float64) float64 { return lf(v) / rf(v) }
		default:
			n.fn = func(v []float64) float64 { return math.Mod(lf(v), rf(v)) }
		}
		l = fold(n, l.konst && r.konst)
	}
	return l, err
}

func (p *parser) unary() (node, error) {
	if p.isOp("-") || p.isOp("+") || p.isOp("!") {
		if err := p.enter(); err != nil {
			return node{}, err
		}
		defer p.leave()
		op := p.next().op
		x, err := p.unary()
		if err != nil {
			return node{}, err
		}
		xf := x.fn
		switch op {
		case "-":
			return fold(node{fn: func(v []float64) float64 { return -xf(v) }}, x.konst), nil
		case "!":
			return fold(node{fn: func(v []float64) float64 { return b2f(!truth(xf(v))) }}, x.konst), nil
		}
		return x, nil
	}
	return p.pow()
}

func (p *parser) pow() (node, error) {
	b, err := p.primary()
	if err != nil || !p.isOp("^") {
		return b, err
	}
	p.next()
	e, err := p.unary() // right-associative, and binds tighter than a unary minus on its left: -2^2 is -4
	if err != nil {
		return node{}, err
	}
	bf, ef := b.fn, e.fn
	return fold(node{fn: func(v []float64) float64 { return math.Pow(bf(v), ef(v)) }}, b.konst && e.konst), nil
}

// funcs are the functions a formula may call: each pure, on its arguments
// alone. arity -1 is one or more.
var funcs = map[string]struct {
	arity int
	f1    func(float64) float64
	f2    func(a, b float64) float64
}{
	"abs":   {1, math.Abs, nil},
	"floor": {1, math.Floor, nil},
	"ceil":  {1, math.Ceil, nil},
	"round": {1, math.Round, nil},
	"trunc": {1, math.Trunc, nil},
	"sqrt":  {1, math.Sqrt, nil},
	"log":   {1, math.Log, nil},
	"log10": {1, math.Log10, nil},
	"exp":   {1, math.Exp, nil},
	"sign": {1, func(x float64) float64 {
		switch {
		case x > 0:
			return 1
		case x < 0:
			return -1
		}
		return 0
	}, nil},
	"pow":   {2, nil, math.Pow},
	"min":   {-1, nil, math.Min},
	"max":   {-1, nil, math.Max},
	"clamp": {3, nil, nil},
	"if":    {3, nil, nil},
}

func (p *parser) primary() (node, error) {
	if err := p.enter(); err != nil {
		return node{}, err
	}
	defer p.leave()
	t := p.next()
	switch t.kind {
	case tkNum:
		return constNode(t.num), nil
	case tkName:
		if p.isOp("(") {
			return p.call(t)
		}
		i, ok := p.vars[t.name]
		if !ok {
			return node{}, p.errAt(t, "unknown variable")
		}
		p.used[i] = true
		return node{fn: func(v []float64) float64 { return v[i] }}, nil
	case tkOp:
		if t.op == "(" {
			n, err := p.ternary()
			if err != nil {
				return node{}, err
			}
			if !p.isOp(")") {
				return node{}, p.errAt(p.peek(), "expected \")\"")
			}
			p.next()
			return n, nil
		}
	}
	return node{}, p.errAt(t, "unexpected "+t.what())
}

func (p *parser) call(name token) (node, error) {
	fd, ok := funcs[name.name]
	if !ok {
		return node{}, p.errAt(name, "unknown function")
	}
	p.next() // (
	var args []node
	if !p.isOp(")") {
		for {
			a, err := p.ternary()
			if err != nil {
				return node{}, err
			}
			args = append(args, a)
			if len(args) > maxArgs {
				return node{}, p.errAt(p.peek(), fmt.Sprintf("more than %d arguments", maxArgs))
			}
			if !p.isOp(",") {
				break
			}
			p.next()
		}
	}
	if !p.isOp(")") {
		return node{}, p.errAt(p.peek(), "expected \")\" or \",\"")
	}
	p.next()
	if fd.arity >= 0 && len(args) != fd.arity || fd.arity < 0 && len(args) == 0 {
		return node{}, p.errAt(name, "wrong number of arguments")
	}
	allConst := true
	for _, a := range args {
		allConst = allConst && a.konst
	}
	var n node
	switch {
	case name.name == "if":
		c, a, b := args[0].fn, args[1].fn, args[2].fn
		n.fn = func(v []float64) float64 {
			if truth(c(v)) {
				return a(v)
			}
			return b(v)
		}
	case name.name == "clamp":
		x, lo, hi := args[0].fn, args[1].fn, args[2].fn
		n.fn = func(v []float64) float64 { return math.Min(math.Max(x(v), lo(v)), hi(v)) }
	case fd.f1 != nil:
		f, x := fd.f1, args[0].fn
		n.fn = func(v []float64) float64 { return f(x(v)) }
	case len(args) == 1:
		n = args[0]
	case len(args) == 2:
		f, a, b := fd.f2, args[0].fn, args[1].fn
		n.fn = func(v []float64) float64 { return f(a(v), b(v)) }
	default:
		f := fd.f2
		fns := make([]func([]float64) float64, len(args))
		for i, a := range args {
			fns[i] = a.fn
		}
		n.fn = func(v []float64) float64 {
			r := fns[0](v)
			for _, g := range fns[1:] {
				r = f(r, g(v))
			}
			return r
		}
	}
	return fold(n, allConst), nil
}
