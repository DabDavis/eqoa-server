// Package formula is a small, safe expression language for formulas an
// operator keeps outside the repository (entity/dmgformula.go: the damage
// formulas of combat.formula_file; docs/damage-formulas.md is the user's
// guide).
//
// A value is a number (float64), a string, a record (named fields, from a
// table of the file) or a list. An expression has + - * / % ^ (power),
// comparisons (== != on numbers and strings, < <= > >= on numbers; 1 or 0),
// && || ! (a nonzero number is true), c ? a : b, a record's field (r.name),
// and a fixed set of functions: pure arithmetic (min, max, clamp, abs, floor,
// ceil, round, trunc, sqrt, pow, log, log10, exp, sign, if), the file's
// tables (lookup, field, has, len, at), and random draws from a source the
// caller gives (uniform, normal, beta, pick, pick_index), which a test may
// force to a fixed sequence (Forced). A program is a list of named steps, each
// reading the inputs and the steps before it, then its result and optional
// min/max clamps.
//
// There are no assignments, loops, recursion or I/O: a compiled program is a
// tree of closures whose run is bounded by its size (bounded at compile time)
// and the tables' (bounded at load); the one sampler with a rejection loop
// (beta's Gamma draws) gives up after a fixed number of tries.
//
// Errors carry a line and column and a description of the mistake, never the
// formula's text, a name it did not know, or a table's values: the formulas
// and tables may be private, and an error is logged.
//
// It is hand-written rather than a general expression library (expr-lang/expr
// was the candidate) because that library's errors quote the source with a
// caret under it, which would put a private formula in the log, and because a
// closure tree over a slice of values runs a formula in well under a
// microsecond with no allocation, per swing of thousands of NPCs.
package formula

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// The bounds on a formula.
const (
	MaxSource = 8192 // bytes of one expression
	MaxDepth  = 64   // nesting of an expression
	MaxNodes  = 2000 // operators, numbers and names in an expression
	MaxSteps  = 64   // named steps of a program
	maxArgs   = 16   // a function's arguments
)

// Kind is a value's type.
type Kind uint8

const (
	Num Kind = iota
	Str
	Rec  // a record: named fields
	List // a list of values
)

// Value is a formula's value.
type Value struct {
	N float64
	S string
	P *Record // a record's or a list's contents
	K Kind
}

// Number is a number value.
func Number(x float64) Value { return Value{N: x} }

// String is a string value.
func String(s string) Value { return Value{S: s, K: Str} }

// Record is a table's record (its fields), or a list (its items). It is read
// only once loaded.
type Record struct {
	fields map[string]Value
	items  []Value
}

// Field is a record's field.
func (r *Record) Field(name string) (Value, bool) {
	if r == nil {
		return Value{}, false
	}
	v, ok := r.fields[name]
	return v, ok
}

var emptyRecord = &Record{fields: map[string]Value{}}

// Rand is a program's random draws. Each draw function a formula calls takes
// exactly one call here, so a Forced source replays a fixed vector.
type Rand interface {
	Uniform() float64                 // in [0, 1)
	Normal(mu, sigma float64) float64 // N(mu, sigma)
	Beta(a, b float64) float64        // Beta(a, b), in (0, 1); NaN for a or b not > 0
}

// Env is one run: the slots (the inputs, then the steps) and the draws.
type Env struct {
	V    []Value
	Rand Rand
	bad  string // the first runtime mistake, "" for none
}

func (e *Env) fail(why string) {
	if e.bad == "" {
		e.bad = why
	}
}

// Error is a compile error: where, and what kind of mistake, without the
// formula's text.
type Error struct {
	Line, Col int
	Msg       string
}

func (e *Error) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("line %d, column %d: %s", e.Line, e.Col, e.Msg)
	}
	return fmt.Sprintf("column %d: %s", e.Col, e.Msg)
}

// RunError is a runtime mistake: which part of the program (a step's number,
// "result", "min", "max") and what kind, never a name or a value.
type RunError struct {
	Part, Msg string
}

func (e *RunError) Error() string { return e.Part + ": " + e.Msg }

type fn func(e *Env) Value

// Program is a compiled program.
type Program struct {
	steps            []fn
	result, min, max fn
	first            int // the slot of the first step
	slots            int // the Env's V must be at least this long
	uses             []int
	draws            bool // it calls a random draw
}

// Slots is the length an Env's V must have at least.
func (p *Program) Slots() int { return p.slots }

// Uses is the indices of the inputs the program reads, each once.
func (p *Program) Uses() []int { return p.uses }

// Draws is whether the program makes a random draw.
func (p *Program) Draws() bool { return p.draws }

// Run runs the program on e (its inputs already in e.V). The result is a
// finite number of 0 or more within the clamps, or an error: a runtime
// mistake (a field a record lacks, a value of the wrong kind, a draw that
// failed), or a result that is not a number, is NaN, infinite or negative. A
// clamp that is not a finite number is left out.
func (p *Program) Run(e *Env) (float64, error) {
	if p == nil || len(e.V) < p.slots {
		return 0, &RunError{Part: "program", Msg: "too few slots"}
	}
	e.bad = ""
	for i, s := range p.steps {
		e.V[p.first+i] = s(e)
		if e.bad != "" {
			return 0, &RunError{Part: "step " + strconv.Itoa(i+1), Msg: e.bad}
		}
	}
	r := p.result(e)
	switch {
	case e.bad != "":
		return 0, &RunError{Part: "result", Msg: e.bad}
	case r.K != Num:
		return 0, &RunError{Part: "result", Msg: "not a number"}
	case math.IsNaN(r.N):
		return 0, &RunError{Part: "result", Msg: "not a number (NaN)"}
	case math.IsInf(r.N, 0):
		return 0, &RunError{Part: "result", Msg: "infinite"}
	case r.N < 0:
		return 0, &RunError{Part: "result", Msg: "negative"}
	}
	x := r.N
	if p.min != nil {
		if lo := p.min(e); e.bad == "" && lo.K == Num && !math.IsNaN(lo.N) && !math.IsInf(lo.N, 0) {
			x = math.Max(x, lo.N)
		}
		e.bad = ""
	}
	if p.max != nil {
		if hi := p.max(e); e.bad == "" && hi.K == Num && !math.IsNaN(hi.N) && !math.IsInf(hi.N, 0) {
			x = math.Min(x, hi.N)
		}
		e.bad = ""
	}
	return x, nil
}

// Compiler compiles programs against a set of inputs and tables.
type Compiler struct {
	Inputs map[string]int     // an input's slot
	Tables map[string]*Record // a table: a record of records, by key
}

// Source is a program's text: its steps in order, its result and clamps
// ("" for none). Each Pos is where that text begins in the file, for errors.
type Source struct {
	Steps    []NamedExpr
	Result   Expr
	Min, Max Expr
}

// Expr is one expression's text and where it begins.
type Expr struct {
	Text      string
	Line, Col int // where the text begins (1-based); 0 when unknown
	Block     bool
}

// NamedExpr is a step.
type NamedExpr struct {
	Name string
	Expr
}

// SourceError is a compile error placed in the file: which part, and where.
type SourceError struct {
	Part string // "step 2", "result", "min", "max"
	Err  *Error
	Line int // the file's line, 0 when unknown
	Col  int
}

func (e *SourceError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("line %d, column %d (%s): %s", e.Line, e.Col, e.Part, e.Err.Msg)
	}
	return fmt.Sprintf("%s: %v", e.Part, e.Err)
}

func place(part string, x Expr, err error) error {
	fe, ok := err.(*Error)
	if !ok {
		return fmt.Errorf("%s: %v", part, err)
	}
	se := &SourceError{Part: part, Err: fe}
	if x.Line > 0 {
		switch {
		case x.Block:
			se.Line, se.Col = x.Line+fe.Line, fe.Col
		case fe.Line > 1:
			se.Line, se.Col = x.Line+fe.Line-1, fe.Col
		default:
			se.Line, se.Col = x.Line, x.Col+fe.Col-1
		}
	}
	return se
}

// Compile compiles a program.
func (c *Compiler) Compile(src Source) (*Program, error) {
	if len(src.Steps) > MaxSteps {
		return nil, fmt.Errorf("more than %d steps", MaxSteps)
	}
	first := 0
	for _, i := range c.Inputs {
		first = max(first, i+1)
	}
	p := &Program{first: first, slots: first + len(src.Steps)}
	scope := map[string]int{}
	for k, v := range c.Inputs {
		scope[k] = v
	}
	used := map[int]bool{}
	comp := func(part string, x Expr) (fn, error) {
		ps := &parser{c: c, scope: scope, used: used}
		n, err := ps.compile(x.Text)
		if err != nil {
			return nil, place(part, x, err)
		}
		p.draws = p.draws || ps.draws
		return n.fn, nil
	}
	for i, s := range src.Steps {
		part := "step " + strconv.Itoa(i+1)
		if !validName(s.Name) {
			return nil, fmt.Errorf("%s: a step's name is letters, digits and _", part)
		}
		if _, taken := scope[s.Name]; taken {
			return nil, fmt.Errorf("%s: a step's name is an input's or an earlier step's", part)
		}
		if _, taken := funcs[s.Name]; taken {
			return nil, fmt.Errorf("%s: a step's name is a function's", part)
		}
		f, err := comp(part, s.Expr)
		if err != nil {
			return nil, err
		}
		p.steps = append(p.steps, f)
		scope[s.Name] = first + i
	}
	if strings.TrimSpace(src.Result.Text) == "" {
		return nil, fmt.Errorf("result: none given")
	}
	var err error
	if p.result, err = comp("result", src.Result); err != nil {
		return nil, err
	}
	if strings.TrimSpace(src.Min.Text) != "" {
		if p.min, err = comp("min", src.Min); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(src.Max.Text) != "" {
		if p.max, err = comp("max", src.Max); err != nil {
			return nil, err
		}
	}
	for i := range used {
		if i < first {
			p.uses = append(p.uses, i)
		}
	}
	return p, nil
}

// Compile compiles one expression with inputs alone (no tables, no steps).
func Compile(src string, inputs map[string]int) (*Program, error) {
	return (&Compiler{Inputs: inputs}).Compile(Source{Result: Expr{Text: src}})
}

// --- lexer ---

type tokKind int

const (
	tkEOF tokKind = iota
	tkNum
	tkStr
	tkName
	tkOp
)

type token struct {
	kind      tokKind
	op        string
	num       float64
	text      string // a name's or a string's
	line, col int
}

// what describes a token in an error, never quoting a name, number or string.
func (t token) what() string {
	switch t.kind {
	case tkEOF:
		return "end of expression"
	case tkNum:
		return "number"
	case tkStr:
		return "string"
	case tkName:
		return "name"
	}
	return "operator " + strconv.Quote(t.op) // operators are the language's, not the formula's
}

var twoCharOps = map[string]bool{"==": true, "!=": true, "<=": true, ">=": true, "&&": true, "||": true}

const oneCharOps = "+-*/%^()<>!?:,."

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
func isAlpha(c byte) bool { return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

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
		case c == '#': // a comment to the line's end
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		case isDigit(c) || c == '.' && i+1 < len(src) && isDigit(src[i+1]):
			j := i
			for j < len(src) && (isDigit(src[j]) || src[j] == '.') {
				j++
			}
			if j < len(src) && (src[j] == 'e' || src[j] == 'E') {
				k := j + 1
				if k < len(src) && (src[k] == '+' || src[k] == '-') {
					k++
				}
				if k < len(src) && isDigit(src[k]) {
					for k < len(src) && isDigit(src[k]) {
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
		case c == '"':
			var b strings.Builder
			j := i + 1
			for ; j < len(src) && src[j] != '"'; j++ {
				if src[j] == '\n' {
					return nil, &Error{Line: line, Col: start, Msg: "string not closed on its line"}
				}
				if src[j] == '\\' && j+1 < len(src) && (src[j+1] == '"' || src[j+1] == '\\') {
					j++
				}
				b.WriteByte(src[j])
			}
			if j >= len(src) {
				return nil, &Error{Line: line, Col: start, Msg: "string not closed"}
			}
			toks = append(toks, token{kind: tkStr, text: b.String(), line: line, col: start})
			col += j + 1 - i
			i = j + 1
		case isAlpha(c):
			j := i
			for j < len(src) && (isAlpha(src[j]) || isDigit(src[j])) {
				j++
			}
			toks = append(toks, token{kind: tkName, text: src[i:j], line: line, col: start})
			col += j - i
			i = j
		default:
			if i+1 < len(src) && twoCharOps[src[i:i+2]] {
				toks = append(toks, token{kind: tkOp, op: src[i : i+2], line: line, col: start})
				i += 2
				col += 2
				continue
			}
			if strings.IndexByte(oneCharOps, c) < 0 {
				return nil, &Error{Line: line, Col: start, Msg: "character not allowed in an expression"}
			}
			toks = append(toks, token{kind: tkOp, op: string(c), line: line, col: start})
			i++
			col++
		}
		if len(toks) > MaxNodes*2 {
			return nil, &Error{Line: line, Col: start, Msg: fmt.Sprintf("expression has more than %d tokens", MaxNodes*2)}
		}
	}
	toks = append(toks, token{kind: tkEOF, line: line, col: col})
	return toks, nil
}

// --- parser: precedence climbing into closures ---

type node struct {
	fn    fn
	konst bool  // its value is known at compile time
	cv    Value // that value
}

func constNode(v Value) node {
	return node{fn: func(*Env) Value { return v }, konst: true, cv: v}
}

type parser struct {
	c     *Compiler
	scope map[string]int
	used  map[int]bool
	toks  []token
	pos   int
	depth int
	nodes int
	draws bool
}

func (p *parser) compile(src string) (node, error) {
	if len(src) > MaxSource {
		return node{}, &Error{Line: 1, Col: 1, Msg: fmt.Sprintf("expression longer than %d bytes", MaxSource)}
	}
	toks, err := lex(src)
	if err != nil {
		return node{}, err
	}
	p.toks = toks
	n, err := p.ternary()
	if err != nil {
		return node{}, err
	}
	if t := p.peek(); t.kind != tkEOF {
		return node{}, p.errAt(t, "unexpected "+t.what())
	}
	return n, nil
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
		return p.errAt(p.peek(), fmt.Sprintf("expression nested deeper than %d", MaxDepth))
	}
	if p.nodes > MaxNodes {
		return p.errAt(p.peek(), fmt.Sprintf("expression has more than %d parts", MaxNodes))
	}
	return nil
}

func (p *parser) leave() { p.depth-- }

// fold makes n a constant when every input of it was, running it once now: a
// mistake then (a string added to a number) is a compile error at t.
func (p *parser) fold(n node, inputsConst bool, t token) (node, error) {
	if !inputsConst {
		return n, nil
	}
	e := &Env{}
	v := n.fn(e)
	if e.bad != "" {
		return node{}, p.errAt(t, e.bad)
	}
	return constNode(v), nil
}

func truthOf(e *Env, v Value) bool {
	if v.K != Num {
		e.fail("a condition is not a number")
		return false
	}
	return v.N != 0 && !math.IsNaN(v.N)
}

func numOf(e *Env, v Value) float64 {
	if v.K != Num {
		e.fail("a value used as a number is not one")
		return math.NaN()
	}
	return v.N
}

func b2v(b bool) Value {
	if b {
		return Value{N: 1}
	}
	return Value{}
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
	t := p.next()
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
	return p.fold(node{fn: func(e *Env) Value {
		if truthOf(e, cf(e)) {
			return af(e)
		}
		return bf(e)
	}}, c.konst && a.konst && b.konst, t)
}

func (p *parser) or() (node, error) {
	l, err := p.and()
	for err == nil && p.isOp("||") {
		t := p.next()
		var r node
		if r, err = p.and(); err != nil {
			break
		}
		lf, rf := l.fn, r.fn
		l, err = p.fold(node{fn: func(e *Env) Value { return b2v(truthOf(e, lf(e)) || truthOf(e, rf(e))) }}, l.konst && r.konst, t)
	}
	return l, err
}

func (p *parser) and() (node, error) {
	l, err := p.cmp()
	for err == nil && p.isOp("&&") {
		t := p.next()
		var r node
		if r, err = p.cmp(); err != nil {
			break
		}
		lf, rf := l.fn, r.fn
		l, err = p.fold(node{fn: func(e *Env) Value { return b2v(truthOf(e, lf(e)) && truthOf(e, rf(e))) }}, l.konst && r.konst, t)
	}
	return l, err
}

func equal(a, b Value) bool {
	if a.K != b.K {
		return false
	}
	switch a.K {
	case Num:
		return a.N == b.N
	case Str:
		return a.S == b.S
	}
	return a.P == b.P
}

func (p *parser) cmp() (node, error) {
	l, err := p.add()
	for err == nil {
		t := p.peek()
		if t.kind != tkOp {
			break
		}
		lf := l.fn
		var mk func(rf fn) fn
		switch t.op {
		case "==":
			mk = func(rf fn) fn { return func(e *Env) Value { return b2v(equal(lf(e), rf(e))) } }
		case "!=":
			mk = func(rf fn) fn { return func(e *Env) Value { return b2v(!equal(lf(e), rf(e))) } }
		case "<", "<=", ">", ">=":
			op := t.op
			mk = func(rf fn) fn {
				return func(e *Env) Value {
					a, b := numOf(e, lf(e)), numOf(e, rf(e))
					switch op {
					case "<":
						return b2v(a < b)
					case "<=":
						return b2v(a <= b)
					case ">":
						return b2v(a > b)
					}
					return b2v(a >= b)
				}
			}
		}
		if mk == nil {
			break
		}
		p.next()
		var r node
		if r, err = p.add(); err != nil {
			break
		}
		l, err = p.fold(node{fn: mk(r.fn)}, l.konst && r.konst, t)
	}
	return l, err
}

func arith(op string, lf, rf fn) fn {
	switch op {
	case "+":
		return func(e *Env) Value { return Value{N: numOf(e, lf(e)) + numOf(e, rf(e))} }
	case "-":
		return func(e *Env) Value { return Value{N: numOf(e, lf(e)) - numOf(e, rf(e))} }
	case "*":
		return func(e *Env) Value { return Value{N: numOf(e, lf(e)) * numOf(e, rf(e))} }
	case "/":
		return func(e *Env) Value { return Value{N: numOf(e, lf(e)) / numOf(e, rf(e))} }
	case "%":
		return func(e *Env) Value { return Value{N: math.Mod(numOf(e, lf(e)), numOf(e, rf(e)))} }
	}
	return func(e *Env) Value { return Value{N: math.Pow(numOf(e, lf(e)), numOf(e, rf(e)))} }
}

func (p *parser) add() (node, error) {
	l, err := p.mul()
	for err == nil && (p.isOp("+") || p.isOp("-")) {
		t := p.next()
		var r node
		if r, err = p.mul(); err != nil {
			break
		}
		l, err = p.fold(node{fn: arith(t.op, l.fn, r.fn)}, l.konst && r.konst, t)
	}
	return l, err
}

func (p *parser) mul() (node, error) {
	l, err := p.unary()
	for err == nil && (p.isOp("*") || p.isOp("/") || p.isOp("%")) {
		t := p.next()
		var r node
		if r, err = p.unary(); err != nil {
			break
		}
		l, err = p.fold(node{fn: arith(t.op, l.fn, r.fn)}, l.konst && r.konst, t)
	}
	return l, err
}

func (p *parser) unary() (node, error) {
	if p.isOp("-") || p.isOp("+") || p.isOp("!") {
		if err := p.enter(); err != nil {
			return node{}, err
		}
		defer p.leave()
		t := p.next()
		x, err := p.unary()
		if err != nil {
			return node{}, err
		}
		xf := x.fn
		switch t.op {
		case "-":
			return p.fold(node{fn: func(e *Env) Value { return Value{N: -numOf(e, xf(e))} }}, x.konst, t)
		case "!":
			return p.fold(node{fn: func(e *Env) Value { return b2v(!truthOf(e, xf(e))) }}, x.konst, t)
		}
		return p.fold(node{fn: func(e *Env) Value { return Value{N: numOf(e, xf(e))} }}, x.konst, t)
	}
	return p.pow()
}

func (p *parser) pow() (node, error) {
	b, err := p.postfix()
	if err != nil || !p.isOp("^") {
		return b, err
	}
	t := p.next()
	x, err := p.unary() // right-associative, tighter than a unary minus on its left: -2^2 is -4
	if err != nil {
		return node{}, err
	}
	return p.fold(node{fn: arith("^", b.fn, x.fn)}, b.konst && x.konst, t)
}

// postfix is a primary and its fields: r.name.name...
func (p *parser) postfix() (node, error) {
	n, err := p.primary()
	for err == nil && p.isOp(".") {
		p.next()
		t := p.next()
		if t.kind != tkName {
			return node{}, p.errAt(t, "expected a field's name after \".\"")
		}
		if err = p.enter(); err != nil {
			return node{}, err
		}
		p.leave()
		rf, name := n.fn, t.text
		n = node{fn: func(e *Env) Value {
			r := rf(e)
			if r.K != Rec {
				e.fail("a field read of a value that is not a record")
				return Value{}
			}
			v, ok := r.P.fields[name]
			if !ok {
				e.fail("a field the record lacks (field() gives a default)")
			}
			return v
		}}
	}
	return n, err
}

func (p *parser) primary() (node, error) {
	if err := p.enter(); err != nil {
		return node{}, err
	}
	defer p.leave()
	t := p.next()
	switch t.kind {
	case tkNum:
		return constNode(Number(t.num)), nil
	case tkStr:
		return constNode(String(t.text)), nil
	case tkName:
		if p.isOp("(") {
			return p.call(t)
		}
		i, ok := p.scope[t.text]
		if !ok {
			return node{}, p.errAt(t, "unknown name")
		}
		p.used[i] = true
		return node{fn: func(e *Env) Value { return e.V[i] }}, nil
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
