package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/weather"
)

// A weather step calls a GM's weather at the run's place (the GM's zone, its
// words to the players standing there in the GMs' yellow), by the trigger,
// for the run; stopping the run ends it. The control: the player 1000 away
// (another zone) hears nothing.
func TestTriggerWeatherStep(t *testing.T) {
	w := newTrigWorld(t)
	w.gw.Weather = weather.NewEngine()
	w.set(1, "Storm", "gm", 0, `[{"do":"weather","weather":"snow","strength":"moderate","minutes":15,"text":"Snow falls on %p"},`+
		`{"do":"wait","seconds":600}]`)
	w.clear()
	lines, err := w.fire(1)
	if err != nil || !hasLine(lines, "weather snow moderate on 1 zones for 15 min") {
		t.Fatalf("fire: %q %v", lines, err)
	}
	ovs := w.gw.Weather.Overrides(time.Now())
	if len(ovs) != 1 || ovs[0].Run == 0 || ovs[0].By != "trigger 1 Storm" || ovs[0].Kind != weather.Snow ||
		ovs[0].Zones[0] != weather.ZoneAt(0, w.gp.X, w.gp.Z) {
		t.Fatalf("overrides %+v", ovs)
	}
	heardYellow := func(name string, ls []line) bool {
		for _, l := range ls {
			if l.op == protocol.OpcodeColoredChat && l.text == "Snow falls on "+name && l.rgb == [3]byte{0xFF, 0xE0, 0x40} {
				return true
			}
		}
		return false
	}
	if !heardYellow("Player", heard(t, w.ps)) {
		t.Error("the player in the zone heard nothing")
	}
	if ls := heard(t, w.fs); len(ls) > 0 {
		for _, l := range ls {
			if strings.HasPrefix(l.text, "Snow falls") {
				t.Errorf("the player 1000 away heard %q", l.text)
			}
		}
	}
	if runs, _ := w.e.StopRuns(1); runs != 1 {
		t.Fatalf("StopRuns: %d", runs)
	}
	ovs = w.gw.Weather.Overrides(time.Now())
	if len(ovs) != 1 || !ovs[0].Ended || ovs[0].Until.After(time.Now().Add(time.Minute)) {
		t.Fatalf("after the stop: %+v, want it ended, clearing over its fade", ovs)
	}
}
