package formula

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
)

// Entry is one named formula of a file, with its optional clamps.
type Entry struct {
	Name     string
	Expr     *Program
	Min, Max *Program // nil: none
	Line     int      // where its expression began
}

// MaxFileSize is the most a formula file may hold.
const MaxFileSize = 256 << 10

// ParseFile reads a formula file:
//
//	# a comment line; blank lines are ignored
//	name = expression
//	name.min = expression
//	name.max = expression
//
// A line ending in a backslash continues on the next. known says which names
// a file may define (an unknown one is an error, so a misspelt name is
// reported rather than silently unused); vars are the variables every
// expression may read. Every mistake is returned (each an error naming the
// file line and, for an expression, the column inside it, never the text);
// the entries are returned only when there are none.
func ParseFile(data []byte, known func(name string) bool, vars map[string]int) (map[string]*Entry, []error) {
	if len(data) > MaxFileSize {
		return nil, []error{fmt.Errorf("file larger than %d bytes", MaxFileSize)}
	}
	var errs []error
	out := map[string]*Entry{}
	type pending struct {
		line int
		text string
	}
	var logical []pending
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64<<10), MaxFileSize+1)
	lineNo := 0
	var cur *pending
	for sc.Scan() {
		lineNo++
		s := strings.TrimRight(sc.Text(), " \t\r")
		cont := strings.HasSuffix(s, "\\")
		if cont {
			s = strings.TrimSuffix(s, "\\")
		}
		if cur == nil {
			t := strings.TrimSpace(s)
			if t == "" || strings.HasPrefix(t, "#") {
				continue
			}
			cur = &pending{line: lineNo, text: s}
		} else {
			cur.text += "\n" + s
		}
		if !cont {
			logical = append(logical, *cur)
			cur = nil
		}
	}
	if err := sc.Err(); err != nil {
		return nil, []error{fmt.Errorf("reading: %v", err)}
	}
	if cur != nil {
		logical = append(logical, *cur)
	}
	seen := map[string]int{}
	for _, l := range logical {
		eq := strings.IndexByte(l.text, '=')
		if eq < 0 || eq+1 < len(l.text) && l.text[eq+1] == '=' {
			errs = append(errs, fmt.Errorf("line %d: expected \"name = expression\"", l.line))
			continue
		}
		key := strings.TrimSpace(l.text[:eq])
		name, part := key, ""
		if dot := strings.IndexByte(key, '.'); dot >= 0 {
			name, part = key[:dot], key[dot+1:]
		}
		if !validName(name) || (part != "" && part != "min" && part != "max") {
			errs = append(errs, fmt.Errorf("line %d: not a formula name (letters, digits, _; then .min or .max)", l.line))
			continue
		}
		if !known(name) {
			errs = append(errs, fmt.Errorf("line %d: %q is not a formula this server has", l.line, name))
			continue
		}
		if first, dup := seen[key]; dup {
			errs = append(errs, fmt.Errorf("line %d: %s given again (first on line %d)", l.line, key, first))
			continue
		}
		seen[key] = l.line
		prog, err := Compile(l.text[eq+1:], vars)
		if err != nil {
			errs = append(errs, fileErr(l.line, key, err))
			continue
		}
		e := out[name]
		if e == nil {
			e = &Entry{Name: name}
			out[name] = e
		}
		switch part {
		case "":
			e.Expr, e.Line = prog, l.line
		case "min":
			e.Min = prog
		case "max":
			e.Max = prog
		}
	}
	for name, e := range out {
		if e.Expr == nil {
			errs = append(errs, fmt.Errorf("%s: a clamp with no formula", name))
		}
	}
	if len(errs) > 0 {
		return nil, errs
	}
	return out, nil
}

// fileErr places an expression's error in the file: its line there is the
// entry's first line plus the expression's own line less one, and its column
// on the entry's first line counts from the expression's start (after "=").
func fileErr(line int, key string, err error) error {
	if fe, ok := err.(*Error); ok {
		return fmt.Errorf("line %d (%s): column %d of the expression: %s", line+fe.Line-1, key, fe.Col, fe.Msg)
	}
	return fmt.Errorf("line %d (%s): %v", line, key, err)
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
