package gmweb

// One GM panel for both shards (the owner, 2026-11-21: "we could have both
// servers in 1 gm panel instead of 2 different panels"). Each shard's game
// server still runs its own panel on its own database; the panel GMs use (the
// owner's, Heirloom's) hands a call meant for another shard to that shard's
// panel on the same machine, as the GM who made it.
//
// WHICH SHARD: every operation takes an optional "shard" (MCP's tools, the
// page's and an agent's api/op/<name>, the OpenAPI description alike), or the
// query's ?shard= (the page's switch). Nothing, or this panel's own name
// (gm.shard_id, "heirloom" unset), runs it here; another shard's name
// (gm.other_shards[].name) hands it over; anything else is refused, saying
// which names there are. An operation that has a "shard" argument of its own
// (economy's "here"/"other", set_formula_mode's shard) keeps it for any value
// that is no shard's name.
//
// TRUST: the receiving panel takes a handed call at api/shard/op/<name> only
// when it names a key (gm.shard_key_file) and the call carries it, from a
// loopback address with no proxy's forwarding headers (so never through the
// public host). The call says who the GM is (account id and name), how they
// came (the via, which gets " via <shard> panel" added) and what their
// credential allows (its scope, and whether it was signed in with an
// authenticator). The receiving panel then judges by ITS account: the same
// id, the same name, a panel role there (a GM below its min_level is refused),
// an admin's tools by its own admin_level, its own role grants; and audits the
// change in its own log under that GM. The authenticator was asked once, on
// the panel signed in to: it is not asked again.

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"
)

// hereOnlyOps are the operations that are one for every shard, kept on this
// panel (the owner's): the work board, which every GM and agent reads before
// changing anything on any shard, and the personal boards. They run here
// whatever shard is named, the page's switch included.
var hereOnlyOps = map[string]bool{
	"work_board": true, "post_work": true, "update_work_overview": true,
	"my_boards": true, "board_messages": true, "post_board_message": true,
}

// shardKeyHeader carries the shared key on a handed call.
const shardKeyHeader = "X-EQOA-Shard-Key"

// shardKeyMin is the shortest key taken, in bytes.
const shardKeyMin = 32

// shardCallTimeout bounds a handed call (the panel's own WriteTimeout is 60 s).
const shardCallTimeout = 55 * time.Second

// shardPeer is another shard's panel this one hands calls to.
type shardPeer struct {
	id, label, url string
	key            string
	keyErr         string // why the key is missing: every call to it says so
}

// shardCall is a handed call's body.
type shardCall struct {
	From      string          `json:"from"`
	AccountID int             `json:"accountid"`
	Username  string          `json:"username"`
	Via       string          `json:"via"`
	IP        string          `json:"ip"`
	Scope     string          `json:"scope"`
	MFA       bool            `json:"mfa"`
	Args      json.RawMessage `json:"args"`
}

// readShardKey reads a key file: its text, trimmed, of shardKeyMin bytes or
// more. A file others may read is used, and warned of.
func readShardKey(path string) (string, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if fi.Mode().Perm()&0o077 != 0 {
		log.Printf("GM panel: the shard key file %s can be read by others (%v): chmod 600 it", path, fi.Mode().Perm())
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	key := strings.TrimSpace(string(b))
	if len(key) < shardKeyMin {
		return "", fmt.Errorf("%s holds %d bytes; a key is %d or more (openssl rand -hex 32)", path, len(key), shardKeyMin)
	}
	return key, nil
}

// initShards reads gm.other_shards and gm.shard_key_file. A key that cannot be
// read is logged and leaves that way shut (its calls say why), never the panel.
func (s *Server) initShards() error {
	own := s.ownShard()
	for _, o := range s.cfg.OtherShards {
		id := strings.ToLower(strings.TrimSpace(o.Name))
		if id == "" || id == own {
			return fmt.Errorf("gm.other_shards: a shard's name is empty or this panel's own (%q)", own)
		}
		if s.peer(id) != nil {
			return fmt.Errorf("gm.other_shards: %q twice", id)
		}
		u, err := url.Parse(strings.TrimRight(o.URL, "/"))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("gm.other_shards: %s's url %q is not an http(s) URL", id, o.URL)
		}
		p := &shardPeer{id: id, label: strings.TrimSpace(o.Label), url: u.String()}
		if p.label == "" {
			p.label = strings.ToUpper(id[:1]) + id[1:]
		}
		if key, err := readShardKey(o.KeyFile); err != nil {
			p.keyErr = fmt.Sprintf("this panel has no key for the %s shard (gm.other_shards key_file): %v", p.label, err)
			log.Printf("GM panel: %s", p.keyErr)
		} else {
			p.key = key
		}
		s.peers = append(s.peers, p)
	}
	if path := strings.TrimSpace(s.cfg.ShardKeyFile); path != "" {
		key, err := readShardKey(path)
		if err != nil {
			log.Printf("GM panel: gm.shard_key_file: %v: no other shard's panel can hand calls to this one", err)
		} else {
			s.shardKey = key
		}
	}
	// the operations with a "shard" of their own keep it, peers or none
	s.ownShardArg = map[string]bool{}
	for _, op := range s.opList {
		if p, ok := op.Input["properties"].(map[string]any); ok && p["shard"] != nil {
			s.ownShardArg[op.Name] = true
		}
	}
	if len(s.peers) > 0 {
		s.addShardArgument()
	}
	return nil
}

// ownShard is this panel's name in an operation's shard argument.
func (s *Server) ownShard() string {
	if id := strings.ToLower(strings.TrimSpace(s.cfg.ShardID)); id != "" {
		return id
	}
	return "heirloom"
}

// ownLabel is what the page's switch calls this panel's shard.
func (s *Server) ownLabel() string {
	if l := strings.TrimSpace(s.cfg.ShardLabel); l != "" {
		return l
	}
	id := s.ownShard()
	return strings.ToUpper(id[:1]) + id[1:]
}

// peer is the other shard of that name or label, or nil.
func (s *Server) peer(name string) *shardPeer {
	for _, p := range s.peers {
		if strings.EqualFold(name, p.id) || strings.EqualFold(name, p.label) {
			return p
		}
	}
	return nil
}

// isOwnShard is whether a name is this panel's shard: its id, its label or its
// line of the server list.
func (s *Server) isOwnShard(name string) bool {
	return strings.EqualFold(name, s.ownShard()) || strings.EqualFold(name, s.ownLabel()) ||
		(s.shardName() != "" && strings.EqualFold(name, s.shardName()))
}

// shardNames are the names the argument takes, this panel's first.
func (s *Server) shardNames() []string {
	out := []string{s.ownShard()}
	for _, p := range s.peers {
		out = append(out, p.id)
	}
	return out
}

// shardList is the shards for the page's switch.
func (s *Server) shardList() []map[string]any {
	out := []map[string]any{{"id": s.ownShard(), "label": s.ownLabel(), "here": true}}
	for _, p := range s.peers {
		out = append(out, map[string]any{"id": p.id, "label": p.label, "here": false, "ready": p.keyErr == ""})
	}
	return out
}

// addShardArgument gives every operation's schema the optional "shard". An
// operation with its own keeps it, told of the shards' names too. Each schema
// is copied first: some share their properties' map.
func (s *Server) addShardArgument() {
	names := s.shardNames()
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = fmt.Sprintf("%q", n)
	}
	desc := fmt.Sprintf("which shard's server runs this: %s (default, this panel's own) or %s; another shard's runs "+
		"it on its own database and world, as you, judged by your account there and audited there",
		quoted[0], strings.Join(quoted[1:], ", "))
	for _, op := range s.opList {
		in := make(map[string]any, len(op.Input))
		for k, v := range op.Input {
			in[k] = v
		}
		props := map[string]any{}
		if p, ok := op.Input["properties"].(map[string]any); ok {
			for k, v := range p {
				props[k] = v
			}
		}
		if hereOnlyOps[op.Name] {
			props["shard"] = str(fmt.Sprintf("ignored: this board is one for every shard, kept on %s's panel", s.ownLabel()))
		} else if own, ok := props["shard"].(map[string]any); ok {
			cp := make(map[string]any, len(own))
			for k, v := range own {
				cp[k] = v
			}
			d, _ := cp["description"].(string)
			cp["description"] = d + fmt.Sprintf("; or a shard's name (%s): the whole call then runs on that shard's server", strings.Join(quoted, ", "))
			props["shard"] = cp
		} else {
			props["shard"] = str(desc)
		}
		in["properties"] = props
		op.Input = in
	}
}

// shardArg is the arguments' "shard" when it is a string: present tells
// whether the key is there at all.
func shardArg(raw json.RawMessage) (name string, present bool, isString bool) {
	t := bytes.TrimSpace(raw)
	if len(t) == 0 || t[0] != '{' {
		return "", false, false
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(t, &m) != nil {
		return "", false, false
	}
	v, ok := m["shard"]
	if !ok {
		return "", false, false
	}
	if err := json.Unmarshal(v, &name); err != nil {
		return "", true, false
	}
	return strings.TrimSpace(name), true, true
}

// withoutShard is the arguments with "shard" taken out (its numbers kept as
// they were written).
func withoutShard(raw json.RawMessage) json.RawMessage {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return raw
	}
	delete(m, "shard")
	out, err := json.Marshal(m)
	if err != nil {
		return raw
	}
	return out
}

// routesAway is whether a call goes to another shard's panel (for MCP's check
// that the tool is one here).
func (s *Server) routesAway(raw json.RawMessage, query string) bool {
	name := strings.TrimSpace(query)
	if name == "" {
		name, _, _ = shardArg(raw)
	}
	return name != "" && !s.isOwnShard(name) && s.peer(name) != nil
}

// dispatch is the way in for the page, MCP and OpenAPI: it picks the shard
// (query names it first, else the arguments) and runs the operation here or
// hands it to that shard's panel. mayForward is false for a call another
// panel handed to this one, which goes no further.
func (s *Server) dispatch(a *actor, name string, raw json.RawMessage, r *http.Request, query string, mayForward bool) (any, error) {
	route, fromArgs := strings.TrimSpace(query), false
	argName, present, isString := shardArg(raw)
	if route == "" && present {
		route, fromArgs = argName, true
		if !isString && !s.ownShardArg[name] {
			return nil, badRequest("shard is a shard's name: %s", strings.Join(s.shardNames(), ", "))
		}
	}
	if route == "" {
		if present && !s.ownShardArg[name] {
			raw = withoutShard(raw) // "shard": "" is here; some operations refuse an argument they do not know
		}
		return s.run(a, name, raw, r)
	}
	if hereOnlyOps[name] && len(s.peers) > 0 {
		if present {
			raw = withoutShard(raw)
		}
		return s.run(a, name, raw, r)
	}
	if s.isOwnShard(route) {
		if fromArgs {
			raw = withoutShard(raw) // the operation's own: "" is here
		}
		return s.run(a, name, raw, r)
	}
	if p := s.peer(route); p != nil {
		if !mayForward {
			return nil, badRequest("this call was handed here from another shard's panel, and goes no further (shard %q)", route)
		}
		if a.mustEnrol {
			return nil, forbidden("this server asks every GM, tester and storyteller for an authenticator: enrol one first (the panel's Security page)")
		}
		if fromArgs {
			raw = withoutShard(raw)
		}
		return s.forward(p, a, name, raw)
	}
	if fromArgs && s.ownShardArg[name] {
		return s.run(a, name, raw, r) // the operation's own meaning ("other", a formula peer)
	}
	if len(s.peers) == 0 {
		return nil, badRequest("unknown shard %q: this panel is %q's and hands calls to no other shard", route, s.ownShard())
	}
	return nil, badRequest("unknown shard %q: the shards are %s", route, strings.Join(s.shardNames(), ", "))
}

// credScope is what the actor's credential could do on a shard whose level
// allowed it: a session's the widest (admin once signed in with an
// authenticator), a token's its own.
func credScope(a *actor) string {
	if a.sess != nil {
		if a.MFA {
			return scopeAdmin
		}
		return scopeWrite
	}
	return a.Scope
}

var shardClient = &http.Client{
	Timeout: shardCallTimeout,
	// a panel on this machine: no proxy, no redirect followed
	Transport:     &http.Transport{Proxy: nil, MaxIdleConnsPerHost: 4, IdleConnTimeout: 90 * time.Second},
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// forward hands an operation to another shard's panel and returns its answer
// as that panel gave it: the result, or its refusal with its status.
func (s *Server) forward(p *shardPeer, a *actor, name string, raw json.RawMessage) (any, error) {
	if p.key == "" {
		return nil, &opError{http.StatusServiceUnavailable, p.keyErr}
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = json.RawMessage("{}")
	}
	body, err := json.Marshal(shardCall{From: s.ownShard(), AccountID: a.AccountID, Username: a.Username,
		Via: a.Via, IP: a.IP, Scope: credScope(a), MFA: a.MFA, Args: raw})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), shardCallTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url+"/api/shard/op/"+url.PathEscape(name), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(shardKeyHeader, p.key)
	start := time.Now()
	resp, err := shardClient.Do(req)
	if err != nil {
		log.Printf("GM op %s by %s (%s) for the %s shard: %v", name, a.Username, a.Via, p.label, err)
		return nil, &opError{http.StatusBadGateway, fmt.Sprintf("the %s shard's panel did not answer (%v): is its server running?", p.label, err)}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 256<<20))
	if err != nil {
		return nil, &opError{http.StatusBadGateway, fmt.Sprintf("the %s shard's answer was cut off: %v", p.label, err)}
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var out any
	if err := dec.Decode(&out); err != nil {
		return nil, &opError{http.StatusBadGateway, fmt.Sprintf("the %s shard answered %d with no JSON: %s", p.label, resp.StatusCode, clip(string(data), 200))}
	}
	if resp.StatusCode == http.StatusOK {
		if d := time.Since(start); d > 10*time.Second {
			log.Printf("GM op %s by %s (%s) on the %s shard took %v", name, a.Username, a.Via, p.label, d)
		}
		return out, nil
	}
	m, _ := out.(map[string]any)
	msg, _ := m["error"].(string)
	if msg == "" {
		msg = fmt.Sprintf("the %s shard answered %d", p.label, resp.StatusCode)
	}
	if noOp, _ := m["noOp"].(bool); noOp {
		also := ""
		if _, here := s.ops[name]; here {
			also = fmt.Sprintf(": it is %s's alone (call it without shard, or with shard %q)", s.ownLabel(), s.ownShard())
		}
		return nil, notFound("%q is not an operation on the %s shard%s", name, p.label, also)
	}
	if cur, ok := m["current"].(map[string]any); ok && resp.StatusCode == http.StatusConflict {
		return nil, &shardConflict{msg: msg, current: cur}
	}
	status := resp.StatusCode
	if status == http.StatusUnauthorized {
		// the page takes a 401 for its own session's end: the other shard's
		// refusal is not that
		status = http.StatusForbidden
	}
	if status < 400 {
		status = http.StatusBadGateway
	}
	return nil, &opError{status, fmt.Sprintf("%s (the %s shard)", msg, p.label)}
}

// shardConflict is another shard's conflict, its current row as it gave it.
type shardConflict struct {
	msg     string
	current map[string]any
}

func (e *shardConflict) Error() string { return e.msg }

// conflictRow is an operation error's current row, when it is a conflict.
func conflictRow(err error) (map[string]any, bool) {
	var ce *errConflict
	if errors.As(err, &ce) {
		return ce.current, true
	}
	var sc *shardConflict
	if errors.As(err, &sc) {
		return sc.current, true
	}
	return nil, false
}

// handedFromHere is whether a request came to this listener straight from this
// machine: a loopback peer address, and none of a proxy's forwarding headers
// (a request through nginx comes from loopback too, but carries them).
func handedFromHere(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return false
	}
	for _, h := range []string{"X-Forwarded-For", "X-Real-IP", "Forwarded", "X-Forwarded-Host", "X-Forwarded-Proto"} {
		if r.Header.Get(h) != "" {
			return false
		}
	}
	return true
}

// handleShardOp is api/shard/op/<name>: an operation another shard's panel
// handed to this one, as the GM who called it there.
func (s *Server) handleShardOp(w http.ResponseWriter, r *http.Request) {
	if s.shardKey == "" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	if !handedFromHere(r) {
		log.Printf("GM panel: a shard call refused from %s: not straight from this machine", clientIP(r))
		writeError(w, http.StatusForbidden, "another shard's panel calls from this machine alone")
		return
	}
	got := r.Header.Get(shardKeyHeader)
	if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(s.shardKey)) != 1 {
		log.Printf("GM panel: a shard call refused from %s: wrong or missing key", r.RemoteAddr)
		writeError(w, http.StatusForbidden, "wrong or missing shard key")
		return
	}
	var call shardCall
	if err := decodeBody(r, &call); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/api/shard/op/")
	here := s.ownLabel()
	acc, err := s.accountByID(call.AccountID)
	if err != nil {
		if errors.Is(err, errNoAccount) {
			writeError(w, http.StatusForbidden, fmt.Sprintf("there is no account #%d on %s", call.AccountID, here))
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !strings.EqualFold(acc.Username, call.Username) {
		writeError(w, http.StatusForbidden, fmt.Sprintf("account #%d is %q on %s, not %q", acc.ID, acc.Username, here, call.Username))
		return
	}
	role := s.panelRole(acc)
	if role == "" {
		writeError(w, http.StatusForbidden, fmt.Sprintf("%s has no GM panel on %s (acctlevel %d there; a GM is %d or more)",
			acc.Username, here, acc.Level, s.cfg.MinLevel))
		return
	}
	if !slices.Contains([]string{scopeRead, scopeWrite, scopeAdmin}, call.Scope) {
		writeError(w, http.StatusBadRequest, "scope is read, write or admin")
		return
	}
	if s.cfg.RequireTOTP && !call.MFA {
		writeError(w, http.StatusForbidden, fmt.Sprintf("%s asks every GM for an authenticator: sign in with one", here))
		return
	}
	scope := call.Scope
	if limitedRole(role) && scope == scopeAdmin {
		scope = scopeWrite
	}
	from := cleanName(call.From, 30)
	if from == "" {
		from = "another shard's"
	}
	ip := ""
	if net.ParseIP(strings.TrimSpace(call.IP)) != nil {
		ip = strings.TrimSpace(call.IP)
	}
	a := &actor{
		AccountID: acc.ID, Username: acc.Username, Level: acc.Level, Role: role,
		Admin: acc.Level >= s.cfg.AdminLevel && call.MFA, MFA: call.MFA, Scope: scope,
		Via: clip(cleanName(call.Via, 80), 80) + " via " + from + " panel", IP: ip,
	}
	if _, ok := s.ops[name]; !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": fmt.Sprintf("no operation %q on %s", name, here), "noOp": true})
		return
	}
	out, err := s.dispatch(a, name, call.Args, r, "", false)
	if err != nil {
		if cur, ok := conflictRow(err); ok {
			writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "current": cur})
			return
		}
		writeError(w, errStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// shardInstructions is what MCP's instructions add when this panel reaches
// other shards.
func (s *Server) shardInstructions() string {
	if len(s.peers) == 0 {
		return ""
	}
	var others []string
	for _, p := range s.peers {
		others = append(others, fmt.Sprintf("%q (%s)", p.id, p.label))
	}
	return fmt.Sprintf(" This panel reaches more than one shard (game server): every tool takes an optional shard, "+
		"%q (%s, the default) or %s. Each shard has its own database, world, players and audit log, so say which one you "+
		"mean, and read before you change things on the shard you are going to change.",
		s.ownShard(), s.ownLabel(), strings.Join(others, ", "))
}
