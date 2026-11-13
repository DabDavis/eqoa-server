package formula

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"

	"gopkg.in/yaml.v3"
)

// The bounds on a file.
const (
	MaxFileSize   = 4 << 20 // bytes
	maxTableNodes = 500_000 // values in all its tables (aliases counted at each use)
	maxTableDepth = 16      // nesting of a table's values
)

// ParseFile reads a formula file, YAML:
//
//	tables:              # optional: named tables of records
//	  <table>:
//	    <key>:           # a record: any fields, each a number, a string,
//	      <field>: 1.5   #   a record or a list (of numbers, strings or records)
//	      slots:
//	        - {weight: 3, x: 1}
//	formulas:
//	  <name>: <expression>        # a result alone
//	  <name>:
//	    steps:                    # optional, in order; each reads the earlier
//	      - <step>: <expression>
//	    result: <expression>
//	    min: <expression>         # optional clamps
//	    max: <expression>
//
// known says which formula names a file may define (an unknown one is an
// error, so a misspelt name is reported rather than silently unused); inputs
// are the variables every expression may read. Every mistake is returned,
// each naming the file's line (and column in an expression), the formula and
// the part, never an expression's text or a table's value; the programs and
// the count of tables are returned only when there is none.
func ParseFile(data []byte, known func(name string) bool, inputs map[string]int) (map[string]*Program, int, []error) {
	if len(data) > MaxFileSize {
		return nil, 0, []error{fmt.Errorf("file larger than %d bytes", MaxFileSize)}
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, 0, []error{yamlErr(err)}
	}
	if doc.Kind == 0 || len(doc.Content) == 0 {
		return map[string]*Program{}, 0, nil // empty: no formula
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, 0, []error{fmt.Errorf("line %d: the file is a mapping of tables and formulas", root.Line)}
	}
	var errs []error
	var tablesNode, formulasNode *yaml.Node
	for i := 0; i+1 < len(root.Content); i += 2 {
		k, v := root.Content[i], root.Content[i+1]
		switch k.Value {
		case "tables":
			tablesNode = v
		case "formulas":
			formulasNode = v
		default:
			errs = append(errs, fmt.Errorf("line %d: the file holds only tables: and formulas:", k.Line))
		}
	}
	tables := map[string]*Record{}
	if tablesNode != nil {
		tables, errs = readTables(tablesNode, errs)
	}
	progs := map[string]*Program{}
	if formulasNode != nil {
		c := &Compiler{Inputs: inputs, Tables: tables}
		progs, errs = readFormulas(c, formulasNode, known, errs)
	}
	if len(errs) > 0 {
		return nil, 0, errs
	}
	return progs, len(tables), nil
}

var yamlLine = regexp.MustCompile(`line (\d+)`)

// yamlErr is a YAML syntax error as its line alone: the parser's own message
// may quote the text it choked on.
func yamlErr(err error) error {
	if m := yamlLine.FindStringSubmatch(err.Error()); m != nil {
		return fmt.Errorf("line %s: not valid YAML", m[1])
	}
	return fmt.Errorf("not valid YAML")
}

func readTables(n *yaml.Node, errs []error) (map[string]*Record, []error) {
	tables := map[string]*Record{}
	if n.Kind != yaml.MappingNode {
		return tables, append(errs, fmt.Errorf("line %d: tables: is a mapping of named tables", n.Line))
	}
	budget := maxTableNodes
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if !validName(k.Value) {
			errs = append(errs, fmt.Errorf("line %d: a table's name is lower-case letters, digits and _", k.Line))
			continue
		}
		if _, dup := tables[k.Value]; dup {
			errs = append(errs, fmt.Errorf("line %d: a table given twice", k.Line))
			continue
		}
		v = deref(v)
		if v.Kind != yaml.MappingNode {
			errs = append(errs, fmt.Errorf("line %d: a table is a mapping of records by key", v.Line))
			continue
		}
		val, err := toValue(v, 0, &budget)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		tables[k.Value] = val.P
	}
	return tables, errs
}

func deref(n *yaml.Node) *yaml.Node {
	for i := 0; n.Kind == yaml.AliasNode && n.Alias != nil && i < 8; i++ {
		n = n.Alias
	}
	return n
}

// toValue converts a table's YAML into values, within the budget of nodes.
func toValue(n *yaml.Node, depth int, budget *int) (Value, error) {
	*budget--
	if *budget < 0 {
		return Value{}, fmt.Errorf("line %d: the tables hold more than %d values", n.Line, maxTableNodes)
	}
	if depth > maxTableDepth {
		return Value{}, fmt.Errorf("line %d: a table nested deeper than %d", n.Line, maxTableDepth)
	}
	n = deref(n)
	switch n.Kind {
	case yaml.ScalarNode:
		switch n.Tag {
		case "!!int", "!!float":
			x, err := strconv.ParseFloat(n.Value, 64)
			if err != nil {
				if i, err2 := strconv.ParseInt(n.Value, 0, 64); err2 == nil {
					return Number(float64(i)), nil
				}
				return Value{}, fmt.Errorf("line %d: a number this server cannot read", n.Line)
			}
			return Number(x), nil
		case "!!bool":
			var b bool
			if err := n.Decode(&b); err != nil {
				return Value{}, fmt.Errorf("line %d: a value this server cannot read", n.Line)
			}
			return b2v(b), nil
		case "!!str":
			return String(n.Value), nil
		}
		return Value{}, fmt.Errorf("line %d: an empty or unknown kind of value", n.Line)
	case yaml.MappingNode:
		r := &Record{fields: make(map[string]Value, len(n.Content)/2)}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i]
			if k.Kind != yaml.ScalarNode {
				return Value{}, fmt.Errorf("line %d: a key that is not plain text", k.Line)
			}
			if _, dup := r.fields[k.Value]; dup {
				return Value{}, fmt.Errorf("line %d: a key given twice", k.Line)
			}
			v, err := toValue(n.Content[i+1], depth+1, budget)
			if err != nil {
				return Value{}, err
			}
			r.fields[k.Value] = v
		}
		return Value{P: r, K: Rec}, nil
	case yaml.SequenceNode:
		r := &Record{items: make([]Value, 0, len(n.Content))}
		for _, c := range n.Content {
			v, err := toValue(c, depth+1, budget)
			if err != nil {
				return Value{}, err
			}
			r.items = append(r.items, v)
		}
		return Value{P: r, K: List}, nil
	}
	return Value{}, fmt.Errorf("line %d: a value this server cannot read", n.Line)
}

func exprOf(n *yaml.Node) (Expr, bool) {
	if n.Kind != yaml.ScalarNode {
		return Expr{}, false
	}
	x := Expr{Text: n.Value, Line: n.Line, Col: n.Column}
	switch n.Style {
	case yaml.DoubleQuotedStyle, yaml.SingleQuotedStyle:
		x.Col++
	case yaml.LiteralStyle, yaml.FoldedStyle:
		x.Block = true
	}
	return x, true
}

func readFormulas(c *Compiler, n *yaml.Node, known func(string) bool, errs []error) (map[string]*Program, []error) {
	progs := map[string]*Program{}
	if n.Kind != yaml.MappingNode {
		return progs, append(errs, fmt.Errorf("line %d: formulas: is a mapping of formulas by name", n.Line))
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		name := k.Value
		if !known(name) {
			errs = append(errs, fmt.Errorf("line %d: %q is not a formula this server has", k.Line, name))
			continue
		}
		if _, dup := progs[name]; dup {
			errs = append(errs, fmt.Errorf("line %d: %s given twice", k.Line, name))
			continue
		}
		var src Source
		if x, ok := exprOf(v); ok {
			src.Result = x
		} else if v.Kind == yaml.MappingNode {
			bad := false
			for j := 0; j+1 < len(v.Content); j += 2 {
				pk, pv := v.Content[j], v.Content[j+1]
				switch pk.Value {
				case "steps":
					if pv.Kind != yaml.SequenceNode {
						errs = append(errs, fmt.Errorf("line %d (%s): steps: is a list of \"- name: expression\"", pv.Line, name))
						bad = true
						continue
					}
					for si, s := range pv.Content {
						if s.Kind != yaml.MappingNode || len(s.Content) != 2 {
							errs = append(errs, fmt.Errorf("line %d (%s, step %d): a step is one \"name: expression\"", s.Line, name, si+1))
							bad = true
							continue
						}
						x, ok := exprOf(s.Content[1])
						if !ok {
							errs = append(errs, fmt.Errorf("line %d (%s, step %d): a step's expression is text", s.Line, name, si+1))
							bad = true
							continue
						}
						src.Steps = append(src.Steps, NamedExpr{Name: s.Content[0].Value, Expr: x})
					}
				case "result", "min", "max":
					x, ok := exprOf(pv)
					if !ok {
						errs = append(errs, fmt.Errorf("line %d (%s): %s: is an expression", pv.Line, name, pk.Value))
						bad = true
						continue
					}
					switch pk.Value {
					case "result":
						src.Result = x
					case "min":
						src.Min = x
					default:
						src.Max = x
					}
				default:
					errs = append(errs, fmt.Errorf("line %d (%s): a formula has steps:, result:, min: and max: only", pk.Line, name))
					bad = true
				}
			}
			if bad {
				continue
			}
		} else {
			errs = append(errs, fmt.Errorf("line %d (%s): a formula is an expression or a mapping", v.Line, name))
			continue
		}
		p, err := c.Compile(src)
		if err != nil {
			if se, ok := err.(*SourceError); ok && se.Line > 0 {
				errs = append(errs, fmt.Errorf("line %d, column %d (%s, %s): %s", se.Line, se.Col, name, se.Part, se.Err.Msg))
			} else {
				errs = append(errs, fmt.Errorf("line %d (%s): %v", k.Line, name, err))
			}
			continue
		}
		progs[name] = p
	}
	return progs, errs
}

// Names is a set of programs' names, sorted.
func Names(progs map[string]*Program) []string {
	out := make([]string, 0, len(progs))
	for n := range progs {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func validName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '_' && (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}
