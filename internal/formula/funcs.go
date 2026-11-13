package formula

import (
	"math"
	"strconv"
)

// funcDef is a function a formula may call: its argument counts (maxN -1 is
// up to maxArgs), whether it is pure (folded when its arguments are
// constant), and how its call is built.
type funcDef struct {
	minN, maxN int
	pure       bool
	build      func(p *parser, name token, args []node) (node, error)
}

func math1(f func(float64) float64) funcDef {
	return funcDef{1, 1, true, func(_ *parser, _ token, a []node) (node, error) {
		x := a[0].fn
		return node{fn: func(e *Env) Value { return Value{N: f(numOf(e, x(e)))} }}, nil
	}}
}

func fold2(f func(a, b float64) float64) funcDef {
	return funcDef{1, -1, true, func(_ *parser, _ token, a []node) (node, error) {
		fns := make([]fn, len(a))
		for i, n := range a {
			fns[i] = n.fn
		}
		return node{fn: func(e *Env) Value {
			r := numOf(e, fns[0](e))
			for _, g := range fns[1:] {
				r = f(r, numOf(e, g(e)))
			}
			return Value{N: r}
		}}, nil
	}}
}

func sign(x float64) float64 {
	switch {
	case x > 0:
		return 1
	case x < 0:
		return -1
	}
	return 0
}

// keyOf is a lookup key's text: a string as it is, a number as the shortest
// decimal that reads back to it (5, 2.5).
func keyOf(v Value) (string, bool) {
	switch v.K {
	case Str:
		return v.S, true
	case Num:
		return strconv.FormatFloat(v.N, 'g', -1, 64), true
	}
	return "", false
}

// weightOf is a weighted choice's weight: a record's field (or a number
// item itself), finite and not negative.
func weightOf(e *Env, v Value, field string) float64 {
	if v.K == Num {
		return v.N
	}
	if v.K != Rec {
		e.fail("pick: an item is neither a record nor a number")
		return 0
	}
	w, ok := v.P.fields[field]
	if !ok || w.K != Num {
		e.fail("pick: an item without a numeric weight")
		return 0
	}
	return w.N
}

// choose is the index a uniform draw u picks among weights (each finite and
// not negative, their total above 0), or -1 with e failed.
func choose(e *Env, u float64, n int, weight func(i int) float64) int {
	var cache [64]float64 // the weights of a short list, read once
	if n <= len(cache) {
		for i := range n {
			cache[i] = weight(i)
		}
		weight = func(i int) float64 { return cache[i] }
	}
	total := 0.0
	for i := range n {
		w := weight(i)
		if math.IsNaN(w) || math.IsInf(w, 0) || w < 0 {
			e.fail("pick: a weight that is negative or not finite")
			return -1
		}
		total += w
	}
	if total <= 0 || math.IsNaN(u) || u < 0 || u >= 1 {
		e.fail("pick: no weight above 0, or a failed draw")
		return -1
	}
	target, last := u*total, -1
	for i := range n {
		w := weight(i)
		if w <= 0 {
			continue
		}
		last = i
		if target < w {
			return i
		}
		target -= w
	}
	return last // a rounding past the end: the last weighted item
}

func drawOK(e *Env, x float64) Value {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		e.fail("a random draw failed")
	}
	return Value{N: x}
}

func randOf(e *Env) Rand {
	if e.Rand == nil {
		e.fail("no random source")
		return nil
	}
	return e.Rand
}

var funcs map[string]funcDef

func init() {
	funcs = map[string]funcDef{
		"abs":   math1(math.Abs),
		"floor": math1(math.Floor),
		"ceil":  math1(math.Ceil),
		"round": math1(math.Round),
		"trunc": math1(math.Trunc),
		"sqrt":  math1(math.Sqrt),
		"log":   math1(math.Log),
		"log10": math1(math.Log10),
		"exp":   math1(math.Exp),
		"sign":  math1(sign),
		"min":   fold2(math.Min),
		"max":   fold2(math.Max),
		"pow": {2, 2, true, func(_ *parser, _ token, a []node) (node, error) {
			return node{fn: arith("^", a[0].fn, a[1].fn)}, nil
		}},
		"clamp": {3, 3, true, func(_ *parser, _ token, a []node) (node, error) {
			x, lo, hi := a[0].fn, a[1].fn, a[2].fn
			return node{fn: func(e *Env) Value {
				return Value{N: math.Min(math.Max(numOf(e, x(e)), numOf(e, lo(e))), numOf(e, hi(e)))}
			}}, nil
		}},
		"if": {3, 3, true, func(_ *parser, _ token, a []node) (node, error) {
			c, x, y := a[0].fn, a[1].fn, a[2].fn
			return node{fn: func(e *Env) Value {
				if truthOf(e, c(e)) {
					return x(e)
				}
				return y(e)
			}}, nil
		}},

		// lookup("table", key, key, ...): the table's record under the first
		// key it has (exact name, then family, then "default", as the
		// formula lists them); the empty record when it has none, whose
		// field() reads give their defaults. The table is named by a string
		// written in the formula, checked at compile time.
		"lookup": {2, -1, false, func(p *parser, name token, a []node) (node, error) {
			if !a[0].konst || a[0].cv.K != Str {
				return node{}, p.errAt(name, "lookup's first argument is a table's name in quotes")
			}
			tbl, ok := p.c.Tables[a[0].cv.S]
			if !ok {
				return node{}, p.errAt(name, "unknown table")
			}
			keys := make([]fn, len(a)-1)
			for i, n := range a[1:] {
				keys[i] = n.fn
			}
			return node{fn: func(e *Env) Value {
				for _, k := range keys {
					s, ok := keyOf(k(e))
					if !ok {
						e.fail("lookup: a key is neither a string nor a number")
						return Value{}
					}
					if r, ok := tbl.fields[s]; ok {
						return r
					}
				}
				return Value{P: emptyRecord, K: Rec}
			}}, nil
		}},
		// field(record, "name", default): the field, or default when the
		// record lacks it
		"field": {3, 3, false, func(_ *parser, _ token, a []node) (node, error) {
			r, nm, d := a[0].fn, a[1].fn, a[2].fn
			return node{fn: func(e *Env) Value {
				rv, nv := r(e), nm(e)
				if rv.K != Rec || nv.K != Str {
					e.fail("field: not a record and a field's name")
					return Value{}
				}
				if v, ok := rv.P.fields[nv.S]; ok {
					return v
				}
				return d(e)
			}}, nil
		}},
		// has(record, "name"): 1 when the record has the field
		"has": {2, 2, false, func(_ *parser, _ token, a []node) (node, error) {
			r, nm := a[0].fn, a[1].fn
			return node{fn: func(e *Env) Value {
				rv, nv := r(e), nm(e)
				if rv.K != Rec || nv.K != Str {
					e.fail("has: not a record and a field's name")
					return Value{}
				}
				_, ok := rv.P.fields[nv.S]
				return b2v(ok)
			}}, nil
		}},
		// len(list): its items
		"len": {1, 1, false, func(_ *parser, _ token, a []node) (node, error) {
			l := a[0].fn
			return node{fn: func(e *Env) Value {
				v := l(e)
				if v.K != List {
					e.fail("len: not a list")
					return Value{}
				}
				return Value{N: float64(len(v.P.items))}
			}}, nil
		}},
		// at(list, i): its item i (from 0)
		"at": {2, 2, false, func(_ *parser, _ token, a []node) (node, error) {
			l, ix := a[0].fn, a[1].fn
			return node{fn: func(e *Env) Value {
				v, i := l(e), numOf(e, ix(e))
				if v.K != List || math.IsNaN(i) || i < 0 || int(i) >= len(v.P.items) {
					e.fail("at: not a list, or an index out of it")
					return Value{}
				}
				return v.P.items[int(i)]
			}}, nil
		}},

		// uniform() in [0, 1), uniform(lo, hi) in [lo, hi): one draw
		"uniform": {0, 2, false, func(p *parser, name token, a []node) (node, error) {
			p.draws = true
			if len(a) == 1 {
				return node{}, p.errAt(name, "uniform takes no argument or two")
			}
			if len(a) == 0 {
				return node{fn: func(e *Env) Value {
					if r := randOf(e); r != nil {
						return drawOK(e, r.Uniform())
					}
					return Value{}
				}}, nil
			}
			lo, hi := a[0].fn, a[1].fn
			return node{fn: func(e *Env) Value {
				l, h := numOf(e, lo(e)), numOf(e, hi(e))
				if r := randOf(e); r != nil {
					return drawOK(e, l+(h-l)*r.Uniform())
				}
				return Value{}
			}}, nil
		}},
		// normal(mu, sigma): one draw
		"normal": {2, 2, false, func(p *parser, _ token, a []node) (node, error) {
			p.draws = true
			mu, sg := a[0].fn, a[1].fn
			return node{fn: func(e *Env) Value {
				m, s := numOf(e, mu(e)), numOf(e, sg(e))
				if r := randOf(e); r != nil {
					return drawOK(e, r.Normal(m, s))
				}
				return Value{}
			}}, nil
		}},
		// beta(alpha, beta): one draw
		"beta": {2, 2, false, func(p *parser, _ token, a []node) (node, error) {
			p.draws = true
			al, be := a[0].fn, a[1].fn
			return node{fn: func(e *Env) Value {
				x, y := numOf(e, al(e)), numOf(e, be(e))
				if r := randOf(e); r != nil {
					return drawOK(e, r.Beta(x, y))
				}
				return Value{}
			}}, nil
		}},
		// pick(list[, "weightfield"]): one item of the list, chosen by one
		// uniform draw with the chance of its weight (a record's "weight"
		// field, or the named one; a number item is its own weight)
		"pick": {1, 2, false, func(p *parser, name token, a []node) (node, error) {
			p.draws = true
			field := "weight"
			if len(a) == 2 {
				if !a[1].konst || a[1].cv.K != Str {
					return node{}, p.errAt(name, "pick's second argument is a field's name in quotes")
				}
				field = a[1].cv.S
			}
			l := a[0].fn
			return node{fn: func(e *Env) Value {
				v := l(e)
				if v.K != List {
					e.fail("pick: not a list")
					return Value{}
				}
				r := randOf(e)
				if r == nil {
					return Value{}
				}
				items := v.P.items
				i := choose(e, r.Uniform(), len(items), func(i int) float64 { return weightOf(e, items[i], field) })
				if i < 0 || e.bad != "" {
					return Value{}
				}
				return items[i]
			}}, nil
		}},
		// pick_index(w0, w1, ...): an index from 0, chosen by one uniform
		// draw with the chance of its weight
		"pick_index": {1, -1, false, func(p *parser, _ token, a []node) (node, error) {
			p.draws = true
			fns := make([]fn, len(a))
			for i, n := range a {
				fns[i] = n.fn
			}
			return node{fn: func(e *Env) Value {
				var ws [maxArgs]float64
				for i, f := range fns {
					ws[i] = numOf(e, f(e))
				}
				r := randOf(e)
				if r == nil || e.bad != "" {
					return Value{}
				}
				i := choose(e, r.Uniform(), len(fns), func(i int) float64 { return ws[i] })
				return Value{N: float64(i)}
			}}, nil
		}},
	}
}

func (p *parser) call(name token) (node, error) {
	fd, ok := funcs[name.name()]
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
				return node{}, p.errAt(p.peek(), "too many arguments")
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
	maxN := fd.maxN
	if maxN < 0 {
		maxN = maxArgs
	}
	if len(args) < fd.minN || len(args) > maxN {
		return node{}, p.errAt(name, "wrong number of arguments")
	}
	n, err := fd.build(p, name, args)
	if err != nil {
		return node{}, err
	}
	allConst := fd.pure
	for _, a := range args {
		allConst = allConst && a.konst
	}
	return p.fold(n, allConst, name)
}

func (t token) name() string { return t.text }
