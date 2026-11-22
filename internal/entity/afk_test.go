package entity

import (
	"bytes"
	"slices"
	"testing"
	"time"
)

// afkLines are the away lines queued for a session since the last drain.
func afkLines(d [][]byte) []string { return duelLines(d) }

// The owner, 2026-11-22: "yes add the /afk tag". An away player's name is drawn
// "[AFK] Name", nearest the name after every other tag, and [AFK] is the first
// tag dropped where the 23 bytes will not hold them: with [GM][OG][HC] and a
// 12-letter name the drawn name is the one it would be without it. The name is
// never cut; every drawn name gives the plain name back.
func TestTheAFKTagFits(t *testing.T) {
	hardcoreOn(t)
	for _, c := range []struct {
		name             string
		gm, og, hc, away bool
		want             string
	}{
		{"Bob", false, false, false, true, "[AFK] Bob"},
		{"Bob", false, false, false, false, "Bob"}, // the control
		{"Bob", true, true, true, true, "[GM][OG][HC][AFK] Bob"},
		{"Abcdefghijkl", false, false, true, true, "[HC][AFK] Abcdefghijkl"},    // 22 bytes
		{"Abcdefghijkl", false, true, true, true, "[OG][HC] Abcdefghijkl"},      // [AFK] goes first
		{"Abcdefghijkl", true, true, true, true, "[GM][HC] Abcdefghijkl"},       // then [OG]
		{"Abcdefghijkl", true, true, true, false, "[GM][HC] Abcdefghijkl"},      // the same without it
		{"Abcdefghijklmno", false, false, false, true, "[AFK] Abcdefghijklmno"}, // 15 letters
		{"Abcdefghijklmno", false, false, true, true, "[HC] Abcdefghijklmno"},   // [AFK] goes, [HC] stays
	} {
		p := &Player{Name: c.name, GM: c.gm, OG: c.og, Hardcore: c.hc}
		p.afk.on = c.away
		got := p.ShownName()
		if got != c.want || len(got) > ShownNameMost {
			t.Errorf("%+v: %q, want %q", c, got, c.want)
		}
		if PlainName(got) != c.name {
			t.Errorf("PlainName(%q) = %q", got, PlainName(got))
		}
	}
	if !IsNameTag("[AFK]") || !IsNameTag("[HC][afk]") || IsNameTag("[AFK]Bob") || PlainName("[afk] Bob") != "Bob" || PlainName("[GM][AFK]Bob") != "Bob" {
		t.Error("IsNameTag/PlainName with [AFK]")
	}
	if PlainName("[AFKX] Bob") == "Bob" {
		t.Error("the control: a word that is no tag was taken off")
	}
	if got := ShownNames("Bob", false, false, false); !slices.Equal(got, []string{"Bob", "[AFK] Bob"}) {
		t.Errorf("a player's names %q", got)
	}
	if got := ShownNames("Bob", false, false, true); !slices.Equal(got, []string{"Bob", "[AFK] Bob", "[HC] Bob", "[HC][AFK] Bob"}) {
		t.Errorf("a Hardcore player's names %q", got)
	}
	// on the wire: the channel's 24 bytes at 116, on and off
	p := &Player{Name: "Neo", Level: 60}
	p.afk.on = true
	if name := string(bytes.TrimRight(SerializePlayerUpdate(1, p)[116:140], "\x00")); name != "[AFK] Neo" {
		t.Errorf("the channel's name %q", name)
	}
	p.afk.on = false
	if name := string(bytes.TrimRight(SerializePlayerUpdate(1, p)[116:140], "\x00")); name != "Neo" {
		t.Errorf("the channel's name after %q", name)
	}
}

// /afk turns the tag over, on and off one way, a message with it; the player is
// told each time.
func TestAFKCommand(t *testing.T) {
	d := newDuelists(t)
	duelDrain(d.sa)
	if !d.w.AFKAsk(d.sa, AFKToggle, "") || !d.a.AFK() || d.a.ShownName() != "[AFK] Alpha" {
		t.Fatalf("not away: %q", d.a.ShownName())
	}
	if got := afkLines(duelDrain(d.sa)); len(got) != 2 || got[0] != afkLineOn {
		t.Fatalf("told %q", got)
	}
	if d.w.AFKAsk(d.sa, AFKWantOn, "") || !d.a.AFK() {
		t.Fatal("/afk on while away changed it")
	}
	d.w.AFKAsk(d.sa, AFKToggle, "back in 10") // a message while away: the message changes, still away
	if !d.a.AFK() || d.a.afk.msg != "back in 10" {
		t.Fatalf("message while away: %v %q", d.a.AFK(), d.a.afk.msg)
	}
	duelDrain(d.sa)
	if !d.w.AFKAsk(d.sa, AFKToggle, "") || d.a.AFK() || d.a.ShownName() != "Alpha" {
		t.Fatal("not back")
	}
	if got := afkLines(duelDrain(d.sa)); len(got) != 1 || got[0] != afkLineOff {
		t.Fatalf("told %q", got)
	}
	if d.w.AFKAsk(d.sa, AFKWantOff, "") || d.a.AFK() {
		t.Fatal("/afk off while back changed it")
	}
	if got := afkLines(duelDrain(d.sa)); len(got) != 1 || got[0] != afkLineNotOn {
		t.Fatalf("told %q", got)
	}
	if d.b.AFK() {
		t.Fatal("another player away")
	}
}

// A tell to an away player is answered with its message, once a minute to one
// teller; another teller has its own minute; back, nothing.
func TestAFKReplyOnceAMinute(t *testing.T) {
	d := newDuelists(t)
	t0 := d.t0
	if _, ok := d.w.AFKReply(d.a.ServerID, d.b.ServerID, t0); ok {
		t.Fatal("answered while not away (the control)")
	}
	d.w.AFKAsk(d.sa, AFKWantOn, "back in 10")
	if l, ok := d.w.AFKReply(d.a.ServerID, d.b.ServerID, t0); !ok || l != "Alpha is away: back in 10" {
		t.Fatalf("first: %q %v", l, ok)
	}
	if _, ok := d.w.AFKReply(d.a.ServerID, d.b.ServerID, t0.Add(59*time.Second)); ok {
		t.Fatal("answered twice in a minute")
	}
	if l, ok := d.w.AFKReply(d.a.ServerID, d.c.ServerID, t0.Add(59*time.Second)); !ok || l != "Alpha is away: back in 10" {
		t.Fatal("another teller not answered")
	}
	if _, ok := d.w.AFKReply(d.a.ServerID, d.b.ServerID, t0.Add(61*time.Second)); !ok {
		t.Fatal("not answered after the minute")
	}
	if _, ok := d.w.AFKReply(d.a.ServerID, d.a.ServerID, t0); ok {
		t.Fatal("answered itself")
	}
	d.w.AFKAsk(d.sa, AFKWantOff, "")
	d.w.AFKAsk(d.sa, AFKWantOn, "")
	if l, ok := d.w.AFKReply(d.a.ServerID, d.b.ServerID, t0.Add(62*time.Second)); !ok || l != "Alpha is away." {
		t.Fatalf("no message: %q %v (a new away has a new minute)", l, ok)
	}
}

// A 0x40 report ends it past 2 units from where it began, not within; a
// server's move (a summon) takes the anchor with it.
func TestAFKEndsOnAMove(t *testing.T) {
	d := newDuelists(t)
	d.a.Speed = 100 // the movement check is off; no limits in the way
	d.w.AFKAsk(d.sa, AFKWantOn, "")
	duelDrain(d.sa)
	report := func(x, z float32) {
		d.w.ApplyClientUpdate(d.a.ServerID, ClientUpdate{X: x, Y: d.a.Y, Z: z, At: d.t0})
	}
	report(101.5, 100) // 1.5: a turn in place
	report(100, 101.5)
	if !d.a.AFK() {
		t.Fatal("ended by a shuffle")
	}
	d.w.UpdatePosition(d.a.ServerID, 500, 10, 500, 0) // a GM's summon
	report(500.5, 500)
	if !d.a.AFK() {
		t.Fatal("ended by a summon")
	}
	report(503, 500) // 3 from where it was set down
	if d.a.AFK() || d.a.ShownName() != "Alpha" {
		t.Fatal("not ended by a walk")
	}
	if got := afkLines(duelDrain(d.sa)); len(got) != 1 || got[0] != afkLineOff {
		t.Fatalf("told %q", got)
	}
}

// An attack press and a cast end it.
func TestAFKEndsOnAnAttackAndACast(t *testing.T) {
	f := newFight(t)
	f.w.AFKAsk(f.sess, AFKWantOn, "")
	f.w.StopAttackOnRetarget(f.sess, f.npcID)
	f.w.SetTarget(f.p.ServerID, f.npcID)
	if !f.w.EngageAttack(f.sess, f.npcID, swing6s2, f.t0) {
		t.Fatal("not engaged")
	}
	if f.p.AFK() {
		t.Fatal("an attack left it on")
	}

	c := newCaster(t)
	c.w.AFKAsk(c.sess, AFKWantOn, "")
	if !c.p.AFK() {
		t.Fatal("the control: ended before the cast")
	}
	if !c.w.StartCast(c.sess, minorHeal(), c.me, c.at(0)) {
		t.Fatal("no cast")
	}
	if c.p.AFK() {
		t.Fatal("a cast left it on")
	}
}

// A keep restart keeps it, its message and its anchor; a player not away is
// kept not away; a fresh Player (a login) is not away.
func TestAFKKeptOverARestart(t *testing.T) {
	d := newDuelists(t)
	d.w.AFKAsk(d.sa, AFKWantOn, "brb")
	now := d.t0
	k, ok := d.w.KeepPlayer(d.sa, now)
	if !ok || !k.AFK || k.AFKMsg != "brb" || k.AFKPlace == nil || *k.AFKPlace != [3]float32{100, 10, 100} {
		t.Fatalf("kept %+v", k)
	}
	if k2, _ := d.w.KeepPlayer(d.sb, now); k2.AFK || k2.AFKPlace != nil {
		t.Fatal("kept for a player not away")
	}
	d.a.afk = afkState{} // the new image's Player, built from its row
	if d.a.AFK() {
		t.Fatal("the control: a fresh Player away")
	}
	d.w.RestoreKeptPlayer(d.sa, k, now)
	if !d.a.AFK() || d.a.afk.msg != "brb" || d.a.ShownName() != "[AFK] Alpha" || d.a.afk.at != [3]float32{100, 10, 100} {
		t.Fatalf("restored %+v %q", d.a.afk, d.a.ShownName())
	}
}
