package entity

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/session"
)

// /stuck sends its player home through the gate hook (the user, 2026-11-08: "zaeire is stuck, and we
// can add /stuck also"), then not again for ten minutes, saying how long; a creature fighting it, a
// PvP flag held from a fight and death each refuse it, the hook not called.
func TestStuckSendsHomeOnceInAWhile(t *testing.T) {
	c := newCaster(t)
	var sent []string
	c.w.OnGate(func(_ *session.Session, p *Player) { sent = append(sent, p.Name) })
	if why := c.w.Stuck(c.sess, c.at(0)); why != "" || len(sent) != 1 {
		t.Fatalf("the first /stuck: %q, gates %v", why, sent)
	}
	if why := c.w.Stuck(c.sess, c.at(time.Minute)); !strings.Contains(why, "9 minutes") || len(sent) != 1 {
		t.Errorf("a minute on: %q, gates %v", why, sent)
	}
	later := c.at(StuckEvery + time.Second)
	c.npc.Threats = map[EntityID]int64{c.p.ServerID: 5}
	if why := c.w.Stuck(c.sess, later); why != stuckLineFight || len(sent) != 1 {
		t.Errorf("in a fight: %q, gates %v", why, sent)
	}
	c.npc.Threats = nil
	c.p.pvp.on, c.p.pvp.hold = true, later.Add(time.Minute)
	if why := c.w.Stuck(c.sess, later); why != stuckLineFight || len(sent) != 1 {
		t.Errorf("a PvP flag held: %q, gates %v", why, sent)
	}
	c.p.pvp.on = false
	c.p.HP = 0
	if why := c.w.Stuck(c.sess, later); why != stuckLineDead || len(sent) != 1 {
		t.Errorf("dead: %q, gates %v", why, sent)
	}
	c.p.HP = 100
	if why := c.w.Stuck(c.sess, later); why != "" || len(sent) != 2 {
		t.Errorf("ten minutes on, out of every fight: %q, gates %v", why, sent)
	}
}
