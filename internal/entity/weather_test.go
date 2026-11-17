package entity

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
	"github.com/eqoa/server/internal/weather"
)

// The regional weather's sends (weather.go) on a 2x2-zone world: zone (0,0)
// temperate (profile 0), (1,0) half snow half sand, (0,1) lava (19), (1,1)
// temperate; tools/seed_weather.py's rules.

func weatherTestEngine() *weather.Engine {
	g := &weather.Profiles{Rows: 2, Cols: 2, Subdiv: 4, Cell: 2000, OriginX: 1000, OriginZ: 1000, Data: make([]byte, 64)}
	for j := 0; j < 8; j++ {
		for i := 0; i < 8; i++ {
			switch {
			case i >= 4 && j < 4 && i < 6:
				g.Data[j*8+i] = 2
			case i >= 6 && j < 4:
				g.Data[j*8+i] = 3
			case i < 4 && j >= 4:
				g.Data[j*8+i] = 19
			}
		}
	}
	r := weather.Rules{Profiles: map[int]weather.Climate{}, Zones: map[weather.Zone]weather.Climate{}, Climates: map[weather.Climate]weather.Rule{}}
	for p := 0; p < 22; p++ {
		r.Profiles[p] = weather.Temperate
	}
	r.Profiles[2], r.Profiles[10], r.Profiles[3], r.Profiles[11], r.Profiles[19] = weather.Arctic, weather.Arctic, weather.Desert, weather.Desert, weather.None
	base := weather.Rule{CycleMinutes: 40, EventChance: 1, WeightLight: 60, WeightModerate: 30, WeightHeavy: 10, WhiteoutMin: 30, WhiteoutMax: 60, FadeSeconds: 30}
	t, a, d := base, base, base
	t.Climate, t.Kind, t.EventMin, t.EventMax = weather.Temperate, weather.Rain, 6, 12
	a.Climate, a.Kind, a.EventMin, a.EventMax, a.WhiteoutChance = weather.Arctic, weather.Snow, 6, 12, 0.5
	d.Climate, d.Kind, d.EventMin, d.EventMax = weather.Desert, weather.Sand, 3, 6
	r.Climates[weather.Temperate], r.Climates[weather.Arctic], r.Climates[weather.Desert] = t, a, d
	e := weather.NewEngine()
	e.SetGrids(map[int]*weather.Profiles{0: g})
	e.SetRules(r)
	return e
}

type weatherWorld struct {
	w   *World
	p   *Player
	ps2 *session.Session
}

func newWeatherWorld(t *testing.T, x, z float32) *weatherWorld {
	t.Helper()
	w := NewWorld()
	w.Weather = weatherTestEngine()
	mgr := session.NewManager()
	w.SessionManager = mgr
	p := &Player{ServerID: 1001, Name: "Rainy", Level: 20, X: x, Z: z}
	s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 8), Port: 7008}, 8, 1, 1, 2, true)
	w.Players[p.ServerID] = p
	s.SetECSEntity(p)
	s.SetCharacterInWorld(true)
	return &weatherWorld{w: w, p: p, ps2: s}
}

// pass is one weather pass at now (every tenth tick).
func (ww *weatherWorld) pass(now time.Time) {
	ww.w.TickCount += 10
	ww.w.mu.Lock()
	ww.w.weatherSystem(now)
	ww.w.mu.Unlock()
}

// sent is the weather messages queued: each its opcode and body.
type sentMsg struct {
	op   protocol.GameOpcode
	body []byte
}

func weatherSent(s *session.Session) []sentMsg {
	var out []sentMsg
	for _, m := range s.Queue().GetPending() {
		d := m.Data()
		if len(d) < 2 {
			continue
		}
		switch op := protocol.GameOpcode(binary.LittleEndian.Uint16(d)); op {
		case protocol.OpcodeWeatherControl, protocol.OpcodeSkyboxWeather, protocol.OpcodeWeatherData:
			out = append(out, sentMsg{op, append([]byte(nil), d[2:]...)})
		}
	}
	return out
}

var weatherT0 = time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)

// heavyEvent finds an automatic heavy event over the place.
func heavyEvent(t *testing.T, e *weather.Engine, x, z float32) weather.Timeline {
	t.Helper()
	for i := 0; i < 2000; i++ {
		now := weatherT0.Add(time.Duration(i) * 20 * time.Minute)
		if tl, ok := e.At(0, x, z, now); ok && tl.Phases[2].Strength == 1 {
			return tl
		}
	}
	t.Fatal("no heavy event in 2000 tries")
	return weather.Timeline{}
}

// A PS2 (never answered the offer) is sent 0x00C7 first, then exactly three
// 0x00C5 for one heavy rain (light, heavy, clear) and nothing between; never 0x7E94.
func TestWeatherPS2OneEvent(t *testing.T) {
	ww := newWeatherWorld(t, 600, 600)
	tl := heavyEvent(t, ww.w.Weather, 600, 600)
	from := tl.Start().Add(-90 * time.Second)
	ww.pass(from)
	got := weatherSent(ww.ps2)
	if len(got) != 1 || got[0].op != protocol.OpcodeSkyboxWeather || !bytes.Equal(got[0].body, []byte{0}) {
		t.Fatalf("first pass: %+v, want one 0x00C7 0", got)
	}
	var bytesSent []byte
	for now := from.Add(time.Second); !now.After(tl.End().Add(90 * time.Second)); now = now.Add(time.Second) {
		ww.pass(now)
		for _, m := range weatherSent(ww.ps2) {
			if m.op != protocol.OpcodeWeatherControl {
				t.Fatalf("at %v: %v sent", now, m.op)
			}
			bytesSent = append(bytesSent, m.body...)
		}
	}
	if !bytes.Equal(bytesSent, []byte{1, 3, 0}) {
		t.Fatalf("0x00C5s %v, want 1 3 0", bytesSent)
	}
}

// Heavy snow on the snow squares: 4, 6, 0.
func TestWeatherPS2Snow(t *testing.T) {
	ww := newWeatherWorld(t, 2600, 600)
	tl := heavyEvent(t, ww.w.Weather, 2600, 600)
	if tl.Climate != weather.Arctic {
		t.Fatalf("climate %s", tl.Climate)
	}
	from := tl.Start().Add(-10 * time.Second)
	ww.pass(from)
	weatherSent(ww.ps2)
	var bs []byte
	for now := from.Add(time.Second); !now.After(tl.End().Add(10 * time.Second)); now = now.Add(time.Second) {
		ww.pass(now)
		for _, m := range weatherSent(ww.ps2) {
			bs = append(bs, m.body...)
		}
	}
	if !bytes.Equal(bs, []byte{4, 6, 0}) {
		t.Fatalf("snow bytes %v, want 4 6 0 (a whiteout is heavy snow's byte)", bs)
	}
}

// Lava is clear unless a GM calls weather there; WeatherResend sends one
// 0x00C7; an instance is clear.
func TestWeatherLavaGMResendInstance(t *testing.T) {
	ww := newWeatherWorld(t, 600, 2600)
	for i := 0; i < 300; i++ {
		ww.pass(weatherT0.Add(time.Duration(i) * 30 * time.Second))
	}
	got := weatherSent(ww.ps2)
	if len(got) != 1 || got[0].op != protocol.OpcodeSkyboxWeather || got[0].body[0] != 0 {
		t.Fatalf("lava over 2.5 h: %+v, want the one 0x00C7 0", got)
	}
	zones, until, id, err := ww.w.CallWeather(0, 600, 2600, weather.Where{Kind: "here"}, weather.Snow, weather.Heavy, 10, "", "Dab", 0)
	if err != nil || len(zones) != 1 || id == 0 || until.Before(time.Now()) {
		t.Fatalf("CallWeather: %v %v %v", zones, until, err)
	}
	ww.pass(time.Now())
	got = weatherSent(ww.ps2)
	if len(got) != 1 || got[0].op != protocol.OpcodeWeatherControl || got[0].body[0] != 6 {
		t.Fatalf("a GM's heavy snow on lava: %+v, want 0x00C5 6", got)
	}
	ww.w.WeatherResend(ww.ps2)
	ww.pass(time.Now())
	got = weatherSent(ww.ps2)
	if len(got) != 1 || got[0].op != protocol.OpcodeSkyboxWeather || got[0].body[0] != 6 {
		t.Fatalf("after WeatherResend: %+v, want one 0x00C7 6", got)
	}
	ww.p.Instance = 77
	ww.pass(time.Now())
	if got = weatherSent(ww.ps2); len(got) != 1 || got[0].body[0] != 0 {
		t.Fatalf("in an instance: %+v, want 0x00C5 0", got)
	}
	if n := ww.w.EndWeatherOverride(id); !n {
		t.Fatal("EndWeatherOverride")
	}
}

// A client that answered 1 is sent 0x7E94 on entry, a change of zone and a
// change of revision only.
func TestWeatherDataOnlyOnChange(t *testing.T) {
	ww := newWeatherWorld(t, 600, 600)
	if !ww.w.SetWeatherDraws(ww.ps2, true) {
		t.Fatal("no player")
	}
	data := func() int {
		n := 0
		for _, m := range weatherSent(ww.ps2) {
			if m.op == protocol.OpcodeWeatherData {
				n++
				if m.body[0] < 1 || m.body[0] > 4 {
					t.Fatalf("zones %d", m.body[0])
				}
			}
		}
		return n
	}
	now := weatherT0
	ww.pass(now)
	if n := data(); n != 1 {
		t.Fatalf("on entry: %d 0x7E94", n)
	}
	for i := 0; i < 5; i++ {
		now = now.Add(time.Second)
		ww.pass(now)
	}
	if n := data(); n != 0 {
		t.Fatalf("unchanged: %d sent", n)
	}
	ww.p.X = 2600 // another zone
	ww.pass(now)
	if n := data(); n != 1 {
		t.Fatalf("a zone change: %d sent", n)
	}
	// over a few hours each revision change sends one, and no more
	tl, _ := ww.w.Weather.At(0, ww.p.X, ww.p.Z, now)
	ww.pass(tl.End().Add(time.Second))
	if n := data(); n != 1 {
		t.Fatalf("the next event's revision: %d sent", n)
	}
	ww.w.SetWeatherDraws(ww.ps2, false)
	ww.p.X = 600
	ww.pass(now)
	if n := data(); n != 0 {
		t.Fatalf("stopped: %d sent", n)
	}
}

// world.weather false: no engine, nothing sent.
func TestWeatherOff(t *testing.T) {
	ww := newWeatherWorld(t, 600, 600)
	ww.w.Weather = nil
	ww.w.SetWeatherDraws(ww.ps2, true)
	for i := 0; i < 100; i++ {
		ww.pass(weatherT0.Add(time.Duration(i) * time.Minute))
	}
	if got := weatherSent(ww.ps2); len(got) != 0 {
		t.Fatalf("sent with the weather off: %+v", got)
	}
	if _, _, _, err := ww.w.CallWeather(0, 0, 0, weather.Where{Kind: "here"}, weather.Rain, 1, 10, "", "x", 0); err == nil {
		t.Fatal("CallWeather with the weather off")
	}
}

// A keep restart (handlers/keep.go, network/udp/keep.go): the GM's weather put
// back on a new engine, a resumed vanilla's draw flag kept, and once its dump
// asks it again (WeatherResend) it is sent the byte at once (0x00C7) and the
// timelines (0x7E94) with the GM's weather in them.
func TestWeatherKeptOverKeepRestart(t *testing.T) {
	ww := newWeatherWorld(t, 600, 2600) // lava: clear but for a GM's
	if !ww.w.SetWeatherDraws(ww.ps2, true) {
		t.Fatal("no player")
	}
	_, until, id, err := ww.w.CallWeather(0, 600, 2600, weather.Where{Kind: "here"}, weather.Snow, weather.Heavy, 5, "", "Jibrish", 0)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	ww.pass(now)
	weatherSent(ww.ps2)
	kp, ok := ww.w.KeepPlayer(ww.ps2, now)
	if !ok || !kp.WeatherDraws {
		t.Fatalf("kept player: %v, draws %v", ok, kp.WeatherDraws)
	}
	kept := ww.w.Weather.Keep(now)

	// the new process: a new engine and the player built anew from its row
	nw := newWeatherWorld(t, 600, 2600)
	if n, bad := nw.w.Weather.Restore(kept, now); n != 1 || len(bad) != 0 {
		t.Fatalf("restored %d, %v", n, bad)
	}
	if !nw.w.RestoreKeptPlayer(nw.ps2, kp, now) || !nw.p.WeatherDraws {
		t.Fatal("RestoreKeptPlayer: no draw flag")
	}
	nw.pass(now.Add(time.Second))
	weatherSent(nw.ps2)        // what went before its dump
	nw.w.WeatherResend(nw.ps2) // the resume dump's (handlers.ResumeDump)
	nw.pass(now.Add(2 * time.Second))
	got := weatherSent(nw.ps2)
	var instant, data bool
	for _, m := range got {
		switch m.op {
		case protocol.OpcodeSkyboxWeather:
			instant = true
			if m.body[0] != 6 {
				t.Fatalf("0x00C7 %d, want 6 (heavy snow)", m.body[0])
			}
		case protocol.OpcodeWeatherData:
			data = true
		case protocol.OpcodeWeatherControl:
			t.Fatalf("a faded 0x00C5 after the resend: %+v", got)
		}
	}
	if !instant || !data {
		t.Fatalf("after the resume dump: %+v, want 0x00C7 6 and 0x7E94", got)
	}
	os := nw.w.Weather.Overrides(now)
	if len(os) != 1 || os[0].ID != id || !os[0].Until.Equal(until) {
		t.Fatalf("overrides %+v, want #%d until %v", os, id, until)
	}
}
