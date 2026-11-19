package udp

import (
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/session"
	"github.com/eqoa/server/internal/weather"
)

// A GM's weather goes across the exec (weather/keep.go): build321's keep
// restart at 23:46 UTC 2026-11-16 dropped a GM's #weather snow heavy 5 here
// of 23:44:35. It comes back under its own id with the end it had, and a
// weather called after takes an id past it.
func TestAGMWeatherGoesAcrossTheExec(t *testing.T) {
	cfg := config.DefaultConfig()
	server, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	old := keepNPCWorld(t)
	old.Weather = weather.NewEngine()
	l := NewListener(cfg, session.NewManager(), nil, old)
	l.Adopt(server)
	if err := l.Start(); err != nil {
		t.Fatal(err)
	}
	defer l.Stop()
	_, until, id, err := old.CallWeather(0, 2000, 2000, weather.Where{Kind: "here"}, weather.Snow, weather.Heavy, 5, "", "Jinkles", 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _, id2, err := old.CallWeather(0, 9000, 9000, weather.Where{Kind: "here"}, weather.Sand, weather.Moderate, 20, "", "Dax", 0)
	if err != nil {
		t.Fatal(err)
	}

	file := filepath.Join(t.TempDir(), "keep-sessions.json")
	execed := make(chan struct{})
	go l.KeepRestart(KeepOptions{File: file, Exe: "/bin/true", Exec: func(string, []string, []string) error {
		close(execed)
		select {}
	}})
	select {
	case <-execed:
	case <-time.After(5 * time.Second):
		t.Fatal("no exec")
	}
	snap, err := ReadSnapshot(file, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Weather) != 2 {
		t.Fatalf("written %+v, want both GM weathers", snap.Weather)
	}

	now := keepNPCWorld(t)
	now.Weather = weather.NewEngine()
	l2 := NewListener(cfg, session.NewManager(), nil, now)
	l2.ResumeKept(snap, time.Now())
	got := now.Weather.Overrides(time.Now())
	if len(got) != 2 || got[0].ID != id || !got[0].Until.Equal(until) || got[0].By != "Jinkles" || got[0].Kind != weather.Snow || got[1].ID != id2 {
		t.Fatalf("after the exec: %+v, want #%d snow by Jinkles until %v and #%d", got, id, until, id2)
	}
	_, _, next, err := now.CallWeather(0, 2000, 2000, weather.Where{Kind: "here"}, weather.Rain, weather.Light, 3, "", "Dax", 0)
	if err != nil || next <= id2 {
		t.Fatalf("next id %d (%v), want past %d", next, err, id2)
	}

	// and a server with the weather off reads the section and leaves it
	off := keepNPCWorld(t)
	NewListener(cfg, session.NewManager(), nil, off).ResumeKept(snap, time.Now())
}
