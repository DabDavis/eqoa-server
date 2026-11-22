package handlers

import (
	"strings"
	"testing"
)

// /merc camp (board entry 1657): with no merc, the refusal; hired, the made
// line and its info's Camp; /merc says it holds the camp; the word is /merc's
// for a tell; /t merc camp again breaks it.
func TestMercCampCommand(t *testing.T) {
	r, m, _ := newMercFixture(t)
	r.drain()
	r.say(r.anna, "/merc camp")
	if got := r.msgs(r.anna); !hasMsg(got, "red:*") {
		t.Fatalf("/merc camp with none: %q", got)
	}
	m.hire(r.anna, r.pa, 0, 0, 0)
	r.drain()
	r.say(r.anna, "/merc camp")
	if got := r.msgs(r.anna); !hasMsg(got, "chat:"+mercCampWords(true, false)) {
		t.Fatalf("/merc camp: %q", got)
	}
	if info, _ := r.gw.MercInfoOf(r.anna); !info.Camp {
		t.Fatal("no camp in its info")
	}
	r.say(r.anna, "/merc")
	if got := r.msgs(r.anna); !strings.Contains(strings.Join(got, "\n"), ", holding your camp") {
		t.Errorf("/merc: %q", got)
	}
	if !mercWords("camp") || !mercWords("1 camp") {
		t.Error("camp is not /merc's word")
	}
	r.say(r.anna, "/t merc camp")
	if got := r.msgs(r.anna); !hasMsg(got, "chat:"+mercCampWords(false, false)) {
		t.Errorf("/t merc camp: %q", got)
	}
	if info, _ := r.gw.MercInfoOf(r.anna); info.Camp {
		t.Error("the camp held after it was broken")
	}
}
