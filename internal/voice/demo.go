package voice

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/coder/websocket"
)

// The demo (voice.demo.enabled; the user's "a demo mode for sandstorm using
// vanilla rust client"): a way in for a client playing on a server that
// offers no voice. It signals over a WebSocket on the account server
// (/api/voice/demo) instead of the game's opcodes and vouches for itself:
// its name and its group's channel (a "d:" id its client derives) cannot be
// checked here, so it is shown as " (demo)" and held to its own limits. The
// media is the same connection, slots and data channel as the game's, and
// the lobby's rooms are shared with the game's players.

// DemoConfig is the demo's.
type DemoConfig struct {
	MaxSessions int           // at once; 0: 100
	PerAddress  int           // at once from one address (an IPv6 /64); 0: 4
	Idle        time.Duration // no message and no connected voice this long closes it; 0: 10 minutes
	TTL         time.Duration // the TURN credentials'; 0: 24 h
}

const (
	demoNameMax   = 24
	demoServerMax = 64
	demoReadLimit = 4096
	demoHelloWait = 10 * time.Second
	demoQueue     = 256
	demoWriteWait = 10 * time.Second
)

// Demo is the demo's WebSocket service.
type Demo struct {
	s   *Server
	cfg DemoConfig

	mu       sync.Mutex
	sessions map[*demoSession]bool
	perAddr  map[string]int
}

// NewDemo makes the demo's service over the forwarding server.
func NewDemo(s *Server, cfg DemoConfig) *Demo {
	if cfg.MaxSessions <= 0 {
		cfg.MaxSessions = 100
	}
	if cfg.PerAddress <= 0 {
		cfg.PerAddress = 4
	}
	if cfg.Idle <= 0 {
		cfg.Idle = 10 * time.Minute
	}
	if cfg.TTL <= 0 {
		cfg.TTL = 24 * time.Hour
	}
	return &Demo{s: s, cfg: cfg, sessions: map[*demoSession]bool{}, perAddr: map[string]int{}}
}

// addrKey is what the per-address limit counts by: an IPv4 address, or an
// IPv6 address's /64.
func addrKey(ip string) string {
	p := net.ParseIP(ip)
	if p == nil {
		return ip
	}
	if v4 := p.To4(); v4 != nil {
		return v4.String()
	}
	return p.Mask(net.CIDRMask(64, 128)).String() + "/64"
}

// demoSession is one demo client.
type demoSession struct {
	d      *Demo
	conn   *websocket.Conn
	addr   string
	key    string
	out    chan []byte
	cancel context.CancelFunc
	failed atomic.Bool // its queue overflowed

	name, server string
	id           uint32
	seq          atomic.Uint32 // the server's signal numbers
	rate         Rate
	asm          Assembler
	lastMsg      atomic.Int64 // unix nanoseconds: the reader and the watch both keep it
	credsUntil   time.Time
}

// The demo's messages (PROTOCOL.md, Demo mode).
type (
	demoIn struct {
		Hello *struct {
			Name     string   `json:"name"`
			Server   string   `json:"server"`
			Channels []string `json:"channels"`
		} `json:"hello"`
		Channels *[]string `json:"channels"`
		Signal   *demoPart `json:"signal"`
		Room     *demoRoom `json:"room"`
	}
	demoPart struct {
		Seq   uint32 `json:"seq"`
		Kind  byte   `json:"kind"`
		Part  int    `json:"part"`
		Parts int    `json:"parts"`
		Text  string `json:"text"`
	}
	demoRoom struct {
		Op       string `json:"op"`
		Name     string `json:"name"`
		Hash     string `json:"hash"`
		Unlisted bool   `json:"unlisted"`
	}
	demoICE struct {
		URL        string `json:"url"`
		Username   string `json:"username"`
		Credential string `json:"credential"`
	}
	demoMember struct {
		Peer     uint32 `json:"peer"`
		Name     string `json:"name"`
		Channels byte   `json:"channels"`
	}
	demoLobbyRoom struct {
		Name    string `json:"name"`
		Members int    `json:"members"`
		Locked  bool   `json:"locked"`
	}
)

// ServeDemo is the demo's WebSocket, the client's address as the account
// server reads it.
func (d *Demo) ServeDemo(w http.ResponseWriter, r *http.Request, clientIP string) {
	// no cookie or account rides on it, so any page may open it
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true, CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		return
	}
	defer c.CloseNow()
	c.SetReadLimit(demoReadLimit)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ds := &demoSession{d: d, conn: c, addr: clientIP, key: addrKey(clientIP), out: make(chan []byte, demoQueue), cancel: cancel}

	d.mu.Lock()
	refusal := ""
	switch {
	case len(d.sessions) >= d.cfg.MaxSessions:
		refusal = "The voice demo is full; try again later."
	case d.perAddr[ds.key] >= d.cfg.PerAddress:
		refusal = "Too many voice demo sessions from your address."
	default:
		d.sessions[ds] = true
		d.perAddr[ds.key]++
	}
	d.mu.Unlock()
	if refusal != "" {
		refuse(ctx, c, refusal)
		return
	}
	defer d.release(ds)

	go ds.write(ctx)
	hctx, hcancel := context.WithTimeout(ctx, demoHelloWait)
	_, first, err := c.Read(hctx)
	hcancel()
	if err != nil {
		return
	}
	if why := ds.hello(first); why != "" {
		refuse(ctx, c, why)
		return
	}
	start := time.Now()
	log.Printf("Voice demo: %q on %q from %s started (%08x)", ds.name, ds.server, ds.addr, ds.id)
	defer func() {
		d.s.Leave(ds.id, false)
		log.Printf("Voice demo: %q on %q from %s ended after %v", ds.name, ds.server, ds.addr, time.Since(start).Round(time.Second))
	}()
	go ds.watch(ctx)
	for {
		_, msg, err := c.Read(ctx)
		if err != nil || ds.failed.Load() {
			return
		}
		ds.handle(msg, time.Now())
	}
}

func (d *Demo) release(ds *demoSession) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.sessions, ds)
	if d.perAddr[ds.key]--; d.perAddr[ds.key] <= 0 {
		delete(d.perAddr, ds.key)
	}
}

// refuse tells the client why and closes.
func refuse(ctx context.Context, c *websocket.Conn, why string) {
	b, _ := json.Marshal(map[string]string{"error": why})
	wctx, cancel := context.WithTimeout(ctx, demoWriteWait)
	defer cancel()
	c.Write(wctx, websocket.MessageText, b)
	c.Close(websocket.StatusPolicyViolation, "refused")
}

// demoText is a demo's name or server as given: trimmed, at most max
// printable characters, false otherwise (and for an empty name).
func demoText(s string, max int) (string, bool) {
	s = strings.TrimSpace(s)
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) > max {
		return s, false
	}
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return s, false
		}
	}
	return s, true
}

// demoGroup is a demo's group channel out of its channels: none, or one
// "d:" id.
func demoGroup(chs []string) (string, bool) {
	switch len(chs) {
	case 0:
		return "", true
	case 1:
		return ChannelID("d:", chs[0])
	}
	return "", false
}

// hello takes the client's first message: its name, its server, its group's
// channel. It says why it is refused, "" when it is not.
func (ds *demoSession) hello(msg []byte) string {
	var in demoIn
	if json.Unmarshal(msg, &in) != nil || in.Hello == nil {
		return "Say hello first."
	}
	name, ok := demoText(in.Hello.Name, demoNameMax)
	if !ok || name == "" {
		return "A name is 1 to 24 printable characters."
	}
	server, ok := demoText(in.Hello.Server, demoServerMax)
	if !ok {
		return "A server's name is at most 64 printable characters."
	}
	group, ok := demoGroup(in.Hello.Channels)
	if !ok {
		return `A demo's channel is one "d:" and 32 hex digits.`
	}
	ds.name, ds.server = name, server
	ds.lastMsg.Store(time.Now().UnixNano())
	if ds.id = ds.d.s.Join(name+" (demo)", group, ds); ds.id == 0 {
		return "The voice server is going down."
	}
	ds.sendConfig(time.Now())
	return ""
}

// sendConfig sends the ICE servers (this server's TURN, when it has one,
// with credentials minted now) and how long they last.
func (ds *demoSession) sendConfig(now time.Time) {
	ice := []demoICE{}
	var ttl time.Duration
	if urls := ds.d.s.TURNURLs(); len(urls) > 0 {
		ttl = ds.d.cfg.TTL
		ds.credsUntil = now.Add(ttl)
		user := Username(ds.credsUntil, ds.id)
		cred := Credential(ds.d.s.TURNSecret(), user)
		for _, u := range urls {
			ice = append(ice, demoICE{u, user, cred})
		}
	}
	ds.queue(map[string]any{"config": map[string]any{"ice": ice, "ttl": int(ttl / time.Second)}})
}

// handle takes one message after the hello, within the allowance.
func (ds *demoSession) handle(msg []byte, now time.Time) {
	if !ds.rate.Take(len(msg), now) {
		return
	}
	ds.lastMsg.Store(now.UnixNano())
	var in demoIn
	if json.Unmarshal(msg, &in) != nil {
		return
	}
	switch {
	case in.Channels != nil:
		group, ok := demoGroup(*in.Channels)
		if !ok {
			ds.queue(map[string]string{"error": `A demo's channel is one "d:" and 32 hex digits.`})
			return
		}
		ds.d.s.SetGroup(ds.id, group)
	case in.Signal != nil:
		p := in.Signal
		if CheckPart(p.Kind, p.Part, p.Parts, p.Text) != nil {
			return
		}
		if text, done := ds.asm.Add(p.Seq, p.Kind, p.Part, p.Parts, p.Text, now); done {
			ds.d.s.Signal(ds.id, p.Kind, text)
		}
	case in.Room != nil:
		ds.room(in.Room)
	}
}

// room is a lobby op: join (answered with the room's state, then the
// lobby), leave (the same) or list (the lobby).
func (ds *demoSession) room(r *demoRoom) {
	switch r.Op {
	case "join":
		res, name := ds.d.s.JoinRoom(ds.id, r.Name, r.Hash, r.Unlisted)
		ds.queue(map[string]any{"roomState": map[string]any{"result": res, "name": name}})
	case "leave":
		name, _ := ds.d.s.LeaveRoom(ds.id)
		ds.queue(map[string]any{"roomState": map[string]any{"result": RoomLeft, "name": name}})
	case "list":
	default:
		return
	}
	rooms := []demoLobbyRoom{}
	for _, l := range ds.d.s.Lobby() {
		rooms = append(rooms, demoLobbyRoom{l.Name, l.Members, l.Locked})
	}
	ds.queue(map[string]any{"lobby": rooms})
}

// Signal is the forwarding server's message for this client, in parts.
func (ds *demoSession) Signal(kind byte, text string) {
	seq := ds.seq.Add(1)
	parts := Split(text)
	for i, c := range parts {
		ds.queue(map[string]any{"signal": demoPart{Seq: seq, Kind: kind, Part: i, Parts: len(parts), Text: c}})
	}
}

// Roster is who this client may hear.
func (ds *demoSession) Roster(ms []Member) {
	out := make([]demoMember, 0, len(ms))
	for _, m := range ms {
		out = append(out, demoMember{m.ID, m.Name, m.Channels})
	}
	ds.queue(map[string]any{"roster": out})
}

// queue hands a message to the writer; a client too slow to take its
// messages is closed (the forwarding server never waits on one).
func (ds *demoSession) queue(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	select {
	case ds.out <- b:
	default:
		if !ds.failed.Swap(true) {
			ds.cancel()
		}
	}
}

func (ds *demoSession) write(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case b := <-ds.out:
			wctx, cancel := context.WithTimeout(ctx, demoWriteWait)
			err := ds.conn.Write(wctx, websocket.MessageText, b)
			cancel()
			if err != nil {
				ds.cancel()
				return
			}
		}
	}
}

// watch closes an idle session (no message and no connected voice for the
// idle time) and renews the TURN credentials before they run out.
func (ds *demoSession) watch(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			if ds.d.s.Connected(ds.id) {
				ds.lastMsg.Store(now.UnixNano())
			}
			if now.Sub(time.Unix(0, ds.lastMsg.Load())) > ds.d.cfg.Idle {
				ds.queue(map[string]string{"error": fmt.Sprintf("Closed after %v idle.", ds.d.cfg.Idle)})
				time.Sleep(100 * time.Millisecond)
				ds.cancel()
				return
			}
			if !ds.credsUntil.IsZero() && now.After(ds.credsUntil.Add(-refreshBefore(ds.d.cfg.TTL))) {
				ds.sendConfig(now)
			}
		}
	}
}

// refreshBefore is how long before they run out credentials are sent again:
// a quarter of their life, at most an hour.
func refreshBefore(ttl time.Duration) time.Duration {
	return min(time.Hour, ttl/4)
}
