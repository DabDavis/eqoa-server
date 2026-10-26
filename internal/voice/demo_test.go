package voice

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// The demo's WebSocket: clients that vouch for themselves, through the same
// forwarding server.

const (
	chanA = "d:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	chanB = "d:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// newDemo serves the demo, the client's address taken from X-Test-IP.
func newDemo(t *testing.T, s *Server, cfg DemoConfig) (*Demo, string) {
	t.Helper()
	d := NewDemo(s, cfg)
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d.ServeDemo(w, r, r.Header.Get("X-Test-IP"))
	}))
	t.Cleanup(hs.Close)
	return d, "ws" + strings.TrimPrefix(hs.URL, "http")
}

// demoClient is one demo client's socket, its messages by their key.
type demoClient struct {
	t      *testing.T
	c      *websocket.Conn
	in     chan map[string]json.RawMessage
	held   []map[string]json.RawMessage // read by refused, not yet looked at
	closed chan struct{}
	seq    uint32
}

// take is the next message, a held one first.
func (dc *demoClient) take(end <-chan time.Time) (map[string]json.RawMessage, bool) {
	if len(dc.held) > 0 {
		m := dc.held[0]
		dc.held = dc.held[1:]
		return m, true
	}
	select {
	case m := <-dc.in:
		return m, true
	case <-end:
		return nil, false
	}
}

func dialDemo(t *testing.T, url, ip string, hello any) *demoClient {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: http.Header{"X-Test-IP": {ip}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.CloseNow() })
	dc := &demoClient{t: t, c: c, in: make(chan map[string]json.RawMessage, 1024), closed: make(chan struct{})}
	go func() {
		defer close(dc.closed)
		for {
			_, b, err := c.Read(context.Background())
			if err != nil {
				return
			}
			var m map[string]json.RawMessage
			if json.Unmarshal(b, &m) == nil {
				dc.in <- m
			}
		}
	}()
	dc.send(hello)
	return dc
}

func (dc *demoClient) send(v any) {
	b, _ := json.Marshal(v)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	dc.c.Write(ctx, websocket.MessageText, b)
}

// next is the next message with key; nil when none comes in time.
func (dc *demoClient) next(key string, wait time.Duration) json.RawMessage {
	end := time.After(wait)
	for {
		m, ok := dc.take(end)
		if !ok {
			return nil
		}
		if v, ok := m[key]; ok {
			return v
		}
	}
}

// refused is the error a refused client is told, "" when it is not.
func (dc *demoClient) refused() string {
	for {
		select {
		case m := <-dc.in:
			if v, ok := m["error"]; ok {
				var s string
				json.Unmarshal(v, &s)
				<-dc.closed
				return s
			}
			if _, ok := m["config"]; ok {
				return ""
			}
			dc.held = append(dc.held, m)
		case <-time.After(5 * time.Second):
			dc.t.Fatal("neither refused nor configured")
		}
	}
}

func hello(name string, chs ...string) map[string]any {
	if chs == nil {
		chs = []string{}
	}
	return map[string]any{"hello": map[string]any{"name": name, "server": "Sandstorm", "channels": chs}}
}

// roster is the next roster a demo client gets.
func (dc *demoClient) roster() []demoMember {
	dc.t.Helper()
	raw := dc.next("roster", 5*time.Second)
	var ms []demoMember
	if raw == nil || json.Unmarshal(raw, &ms) != nil {
		dc.t.Fatal("no roster")
	}
	return ms
}

// lastRoster waits a moment and gives the last roster that came.
func (dc *demoClient) lastRoster(wait time.Duration) []demoMember {
	var last []demoMember
	end := time.After(wait)
	for {
		m, ok := dc.take(end)
		if !ok {
			return last
		}
		if raw, ok := m["roster"]; ok {
			last = nil
			json.Unmarshal(raw, &last)
		}
	}
}

// media gives the demo client a connection, its offer and the answer in
// parts over the socket.
func (dc *demoClient) media(name string, s *Server) *testClient {
	c := &testClient{t: dc.t, s: s, got: make(chan heard, 4096), msgs: make(chan string, 1024)}
	c.connect(name, func(sdp string) {
		dc.seq++
		parts := Split(sdp)
		for i, p := range parts {
			dc.send(map[string]any{"signal": map[string]any{"seq": dc.seq, "kind": KindOffer, "part": i, "parts": len(parts), "text": p}})
		}
	}, func() string {
		var a Assembler
		for {
			raw := dc.next("signal", 10*time.Second)
			if raw == nil {
				dc.t.Fatal("no answer")
			}
			var p demoPart
			json.Unmarshal(raw, &p)
			if p.Kind != KindAnswer {
				continue
			}
			if text, done := a.Add(p.Seq, p.Kind, p.Part, p.Parts, p.Text, time.Now()); done {
				return text
			}
		}
	})
	return c
}

// Two demo clients naming one channel see each other in their rosters (as
// " (demo)"), signal through the socket (an offer in parts, the answer in
// parts) and hear each other; a third naming another channel is in neither
// roster and hears nothing, until it names theirs (the control).
func TestDemoClientsInAChannelHearEachOther(t *testing.T) {
	s := newTestServer(t)
	_, url := newDemo(t, s, DemoConfig{})
	a := dialDemo(t, url, "203.0.113.1", hello("Asker", chanA))
	b := dialDemo(t, url, "203.0.113.2", hello("Bard", chanA))
	c := dialDemo(t, url, "203.0.113.3", hello("Crier", chanB))
	for _, x := range []*demoClient{a, b, c} {
		if x.next("config", 5*time.Second) == nil {
			t.Fatal("no config")
		}
	}
	if ms := a.lastRoster(300 * time.Millisecond); len(ms) != 1 || ms[0].Name != "Bard (demo)" || ms[0].Channels != 1 {
		t.Fatalf("the asker's roster %+v", ms)
	}
	am, bm, cm := a.media("Asker", s), b.media("Bard", s), c.media("Crier", s)
	am.send(1)
	time.Sleep(100 * time.Millisecond)
	talkFor(600*time.Millisecond, []*testClient{am}, []uint8{20}, []string{"A"})
	gb := bm.listen(200 * time.Millisecond)
	if len(gb) != 1 || gb[firstKey(gb)] < 10 {
		t.Fatalf("the channel's other member heard %v", gb)
	}
	if g := cm.listen(50 * time.Millisecond); len(g) != 0 {
		t.Fatalf("another channel's member heard %v", g)
	}
	c.send(map[string]any{"channels": []string{chanA}})
	if ms := c.lastRoster(300 * time.Millisecond); len(ms) != 2 {
		t.Fatalf("in the channel, its roster %+v", ms)
	}
	talkFor(600*time.Millisecond, []*testClient{am}, []uint8{20}, []string{"A"})
	if g := cm.listen(200 * time.Millisecond); len(g) != 1 {
		t.Fatalf("in the channel, it heard %v", g)
	}
}

func firstKey(m map[uint32]int) uint32 {
	for k := range m {
		return k
	}
	return 0
}

// At most 100 at once (the 100th in, the 101st refused, one more in once
// one has gone), and 4 from one address (an IPv6 /64 counts as one; another
// /64 is another).
func TestDemoCaps(t *testing.T) {
	s := newTestServer(t)
	_, url := newDemo(t, s, DemoConfig{})
	var all []*demoClient
	for i := 0; i < 100; i++ {
		c := dialDemo(t, url, fmt.Sprintf("10.0.%d.%d", i/200, i%200+1), hello(fmt.Sprintf("P%d", i)))
		if why := c.refused(); why != "" {
			t.Fatalf("session %d refused: %s", i+1, why)
		}
		all = append(all, c)
	}
	if why := dialDemo(t, url, "10.9.9.9", hello("Extra")).refused(); why == "" {
		t.Fatal("the 101st was let in")
	}
	all[0].c.Close(websocket.StatusNormalClosure, "")
	<-all[0].closed
	time.Sleep(100 * time.Millisecond)
	if why := dialDemo(t, url, "10.9.9.9", hello("Extra")).refused(); why != "" {
		t.Fatalf("one gone, another refused: %s", why)
	}

	s2 := newTestServer(t)
	_, url2 := newDemo(t, s2, DemoConfig{})
	for i := 1; i <= 4; i++ {
		if why := dialDemo(t, url2, "203.0.113.9", hello("Same")).refused(); why != "" {
			t.Fatalf("session %d from one address refused: %s", i, why)
		}
		if why := dialDemo(t, url2, fmt.Sprintf("2001:db8::%d", i), hello("Six")).refused(); why != "" {
			t.Fatalf("session %d from one /64 refused: %s", i, why)
		}
	}
	if why := dialDemo(t, url2, "203.0.113.9", hello("Same")).refused(); why == "" {
		t.Fatal("a 5th from one address was let in")
	}
	if why := dialDemo(t, url2, "2001:db8::ff", hello("Six")).refused(); why == "" {
		t.Fatal("a 5th from one /64 was let in")
	}
	if why := dialDemo(t, url2, "2001:db8:0:1::1", hello("Six")).refused(); why != "" {
		t.Fatalf("another /64 refused: %s", why)
	}
}

// A channel id that is not "d:" and 32 hex digits, two of them, or a name
// of 25 characters, is refused; a well-formed hello (the control) is not.
// A later malformed channel is refused and the group kept.
func TestDemoChecksItsHello(t *testing.T) {
	s := newTestServer(t)
	_, url := newDemo(t, s, DemoConfig{})
	for _, h := range []map[string]any{
		hello("Asker", "d:0123"), hello("Asker", "g:1"), hello("Asker", chanA, chanB),
		hello(strings.Repeat("n", 25), chanA), hello("", chanA), hello("tab\tname", chanA),
		{"channels": []string{chanA}},
	} {
		if why := dialDemo(t, url, "203.0.113.1", h).refused(); why == "" {
			t.Errorf("%v was let in", h)
		}
	}
	a := dialDemo(t, url, "203.0.113.1", hello(strings.Repeat("n", 24), chanA[:2]+strings.ToUpper(chanA[2:])))
	if why := a.refused(); why != "" {
		t.Fatalf("a well-formed hello refused: %s", why)
	}
	b := dialDemo(t, url, "203.0.113.2", hello("Bard", chanA))
	if why := b.refused(); why != "" {
		t.Fatal(why)
	}
	if ms := b.lastRoster(300 * time.Millisecond); len(ms) != 1 {
		t.Fatalf("the channel's case does not matter: %+v", ms)
	}
	if ms := a.lastRoster(300 * time.Millisecond); len(ms) != 1 {
		t.Fatalf("the first's roster %+v", ms)
	}
	b.send(map[string]any{"channels": []string{"d:nothex"}})
	if b.next("error", 2*time.Second) == nil {
		t.Fatal("a malformed channel was taken")
	}
	if ms := a.lastRoster(300 * time.Millisecond); ms != nil {
		t.Fatalf("a refused channel changed the rosters: %+v", ms)
	}
}

// A demo client and a game session share a room of the lobby; the room as
// the demo's lobby lists it; an unlisted room made over the demo is in no
// list.
func TestDemoAndGamePlayersShareRooms(t *testing.T) {
	s := newTestServer(t)
	_, url := newDemo(t, s, DemoConfig{})
	game := newTestSink()
	gid := s.Join("Gamer", "g:7", game)
	if r, _ := s.JoinRoom(gid, "Tavern", "", false); r != RoomIn {
		t.Fatal(r)
	}
	a := dialDemo(t, url, "203.0.113.1", hello("Asker", chanA))
	if why := a.refused(); why != "" {
		t.Fatal(why)
	}
	a.send(map[string]any{"room": map[string]any{"op": "join", "name": "tavern"}})
	var st struct {
		Result int    `json:"result"`
		Name   string `json:"name"`
	}
	json.Unmarshal(a.next("roomState", 2*time.Second), &st)
	if st.Result != RoomIn || st.Name != "Tavern" {
		t.Fatalf("the join: %+v", st)
	}
	var lb []demoLobbyRoom
	json.Unmarshal(a.next("lobby", 2*time.Second), &lb)
	if len(lb) != 1 || lb[0] != (demoLobbyRoom{"Tavern", 2, false}) {
		t.Fatalf("the lobby %+v", lb)
	}
	if ms := game.members(); len(ms) != 1 || ms[0].Name != "Asker (demo)" || ms[0].Channels != 2 {
		t.Fatalf("the game player's roster %+v", ms)
	}
	a.send(map[string]any{"room": map[string]any{"op": "join", "name": "Back Room", "unlisted": true}})
	json.Unmarshal(a.next("roomState", 2*time.Second), &st)
	lb = nil
	json.Unmarshal(a.next("lobby", 2*time.Second), &lb)
	if st.Result != RoomIn || len(lb) != 1 || lb[0].Name != "Tavern" {
		t.Fatalf("an unlisted room: %+v, the lobby %+v", st, lb)
	}
	if r, _ := s.JoinRoom(gid, "BACK ROOM", "", false); r != RoomIn {
		t.Fatalf("the game player joins it by name: %d", r)
	}
	a.send(map[string]any{"room": map[string]any{"op": "leave"}})
	json.Unmarshal(a.next("roomState", 2*time.Second), &st)
	if st.Result != RoomLeft || st.Name != "Back Room" {
		t.Fatalf("leaving: %+v", st)
	}
}

// A session with no message and no connected voice for the idle time is
// closed, told why; one that keeps talking (the control) is not.
func TestDemoClosesAnIdleSession(t *testing.T) {
	s := newTestServer(t)
	_, url := newDemo(t, s, DemoConfig{Idle: 2 * time.Second})
	idle := dialDemo(t, url, "203.0.113.1", hello("Idle"))
	busy := dialDemo(t, url, "203.0.113.2", hello("Busy"))
	for i := 0; i < 12; i++ {
		busy.send(map[string]any{"room": map[string]any{"op": "list"}})
		time.Sleep(300 * time.Millisecond)
	}
	if idle.next("error", time.Second) == nil {
		t.Fatal("an idle session was not told")
	}
	select {
	case <-idle.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("an idle session was not closed")
	}
	select {
	case <-busy.closed:
		t.Fatal("a busy session was closed")
	default:
	}
}
