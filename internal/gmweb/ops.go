package gmweb

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// An operation is one thing a GM can do: the page calls it at api/op/<name>
// with its arguments as the JSON body, an AI agent as the MCP tool of the same
// name (mcp.go) or the OpenAPI operation (openapi.go). Its description and
// schema are written for whoever reads them, an agent included.

// Op is one operation.
type Op struct {
	Name        string
	Title       string
	Description string
	// Scope is what the credential needs: scopeRead, scopeWrite, scopeAdmin.
	Scope string
	// Destructive marks what removes or overwrites (an MCP hint to ask first).
	Destructive bool
	// Input is the JSON Schema of the arguments (an object).
	Input map[string]any
	// Core marks the operations a client with a small budget for them (a
	// custom GPT's actions) should be given first.
	Core    bool
	Handler func(c *opCtx) (any, error)
}

// opCtx is one call.
type opCtx struct {
	s    *Server
	a    *actor
	raw  json.RawMessage
	r    *http.Request
	args map[string]any
}

// decode reads the arguments into v (numbers kept exact).
func (c *opCtx) decode(v any) error {
	raw := c.raw
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		raw = []byte("{}")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(v); err != nil {
		return badRequest("the arguments do not match the schema: %v", err)
	}
	return nil
}

// opError is a refusal with an HTTP status.
type opError struct {
	status int
	msg    string
}

func (e *opError) Error() string { return e.msg }

func badRequest(format string, a ...any) error {
	return &opError{http.StatusBadRequest, fmt.Sprintf(format, a...)}
}

func forbidden(format string, a ...any) error {
	return &opError{http.StatusForbidden, fmt.Sprintf(format, a...)}
}

func notFound(format string, a ...any) error {
	return &opError{http.StatusNotFound, fmt.Sprintf(format, a...)}
}

// errStatus is the HTTP status an operation's error answers with.
func errStatus(err error) int {
	var oe *opError
	if errors.As(err, &oe) {
		return oe.status
	}
	var ce *errConflict
	if errors.As(err, &ce) {
		return http.StatusConflict
	}
	return http.StatusBadRequest
}

func (s *Server) addOp(op *Op) {
	if s.ops == nil {
		s.ops = map[string]*Op{}
	}
	if _, dup := s.ops[op.Name]; dup {
		panic("gmweb: operation " + op.Name + " twice")
	}
	if op.Input == nil {
		op.Input = obj(nil)
	}
	s.ops[op.Name] = op
	s.opList = append(s.opList, op)
}

// registerOps registers every operation (ops_*.go).
func (s *Server) registerOps() {
	s.registerDiscoveryOps()
	s.registerWorkOps() // ops_work.go: the work board, read before anything changes
	s.registerRowOps()
	s.registerWorldOps()
	s.registerPlayerOps()
	s.registerAccountOps()
	s.registerAdminOps()
	s.registerQuestOps()
	s.registerPathOps()
	s.registerVisualOps()
	s.registerSuggestionOps()
	s.registerMapLayerOps()
	s.registerMapMarkOps()
	s.registerTriggerOps()  // ops_triggers.go
	s.registerPetitionOps() // ops_petitions.go
}

// run calls an operation as an actor, with the scope check, and logs a
// failure. It is the one way in for the page, MCP and OpenAPI alike.
func (s *Server) run(a *actor, name string, raw json.RawMessage, r *http.Request) (any, error) {
	op, ok := s.ops[name]
	if !ok {
		return nil, notFound("no operation %q", name)
	}
	if a.mustEnrol {
		return nil, forbidden("this server asks every GM for an authenticator: enrol one first (the panel's Security page)")
	}
	if !a.may(op.Scope) {
		switch op.Scope {
		case scopeAdmin:
			if a.Level < s.cfg.AdminLevel {
				return nil, forbidden("%s needs an admin (acctlevel %d+)", name, s.cfg.AdminLevel)
			}
			if !a.Admin {
				return nil, forbidden("%s is an admin's tool, and an admin's tools need a sign-in with an authenticator code: enrol one on the panel's Security page and sign in again", name)
			}
			return nil, forbidden("%s needs a credential with the admin scope", name)
		default:
			return nil, forbidden("%s changes things, and this credential is read-only", name)
		}
	}
	c := &opCtx{s: s, a: a, raw: raw, r: r}
	start := time.Now()
	out, err := op.Handler(c)
	if err != nil && errStatus(err) >= 500 {
		log.Printf("GM op %s by %s (%s) failed after %v: %v", name, a.Username, a.Via, time.Since(start), err)
	}
	return out, err
}

// handleOp is api/op/<name>: POST the arguments, get the result.
func (s *Server) handleOp(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/op/")
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST the arguments as JSON")
		return
	}
	a, err := s.authenticate(r)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	var raw json.RawMessage
	if err := decodeBody(r, &raw); err != nil && r.ContentLength != 0 {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	out, err := s.run(a, name, raw, r)
	if err != nil {
		var ce *errConflict
		if errors.As(err, &ce) {
			writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "current": ce.current})
			return
		}
		writeError(w, errStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// handleOpList lists the operations the caller may use, with their schemas.
func (s *Server) handleOpList(w http.ResponseWriter, r *http.Request) {
	a, err := s.authenticate(r)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	var out []map[string]any
	for _, op := range s.opList {
		out = append(out, map[string]any{
			"name": op.Name, "title": op.Title, "description": op.Description,
			"scope": op.Scope, "destructive": op.Destructive, "allowed": a.may(op.Scope),
			"input": op.Input,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ops": out})
}

// JSON Schema helpers, for the operations' inputs.

func obj(props map[string]any, required ...string) map[string]any {
	if props == nil {
		props = map[string]any{}
	}
	o := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		o["required"] = required
	}
	return o
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

func integer(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}

func number(desc string) map[string]any { return map[string]any{"type": "number", "description": desc} }

func boolean(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}

func strEnum(desc string, vals ...string) map[string]any {
	return map[string]any{"type": "string", "description": desc, "enum": vals}
}

func arr(items map[string]any, desc string) map[string]any {
	return map[string]any{"type": "array", "items": items, "description": desc}
}

// anyValue is a column value: a number, text, true/false or null.
func anyValue(desc string) map[string]any {
	return map[string]any{"description": desc, "type": []string{"string", "number", "integer", "boolean", "null"}}
}

// freeObject is an object of column -> value.
func freeObject(desc string) map[string]any {
	return map[string]any{"type": "object", "description": desc, "additionalProperties": true}
}

// idOrName is a thing named by its id or its name.
func idOrName(desc string) map[string]any {
	return map[string]any{"description": desc, "type": []string{"integer", "string"}}
}

var filterSchema = obj(map[string]any{
	"column": str("the column"),
	"op": strEnum("= (default), !=, <, <=, >, >=, like (contains, any case), prefix, in / notin (value is a list), "+
		"null, notnull, bits (every bit of value set)", "=", "!=", "<", "<=", ">", ">=", "like", "prefix", "in", "notin",
		"null", "notnull", "bits"),
	"value": map[string]any{"description": "the value to compare with; a list for in/notin"},
}, "column")

var keySchema = freeObject(`the row's key: its primary key column(s) and value(s), as every row read carries it under "__key" ({"id": 42}); a table without a key of its own: {"rowid": n}`)
