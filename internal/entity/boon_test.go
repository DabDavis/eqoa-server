package entity

import (
	"encoding/binary"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// boonsOn is how many boons a player has, and the first one's effect.
func boonsOn(p *Player) (int, *ActiveEffect) {
	n := 0
	var first *ActiveEffect
	for _, e := range p.Effects {
		if e.SpellID == BoonEffectID {
			if n++; first == nil {
				first = e
			}
		}
	}
	return n, first
}

// A town's boon doubles every award while it is on: two of a group's members
// of a level kill a creature, and the one with the boon earns twice the
// other's share (its own share doubled, the group's left alone), its line
// saying so; a quest's award (AwardXP) is doubled the same. The control: the
// member without it earns the kill's share, told in Sandstorm's line.
func TestABoonDoublesEveryAward(t *testing.T) {
	k := newKillWorld(t, 30, 30)
	with, without := k.players[0], k.players[1]
	g, ok := k.w.GiveBoon(k.sess[0], "Freeport", time.Now())
	if !ok || g.Name != "Boon of Freeport" || g.Replaced != "" || g.Rule != DefaultBoon {
		t.Fatalf("the boon: %+v %v", g, ok)
	}
	sentOf(k.sess[0])
	npc := &NPC{ServerID: 5101, Name: "Froglok", Level: 30, X: 1000, Z: 1000}
	k.w.awardKillXPLocked(k.sess[0], with, npc)
	share := groupExp(30, 30, []int{30, 30})
	if share <= 0 || earned(with, 0) != 2*share || earned(without, 0) != share {
		t.Fatalf("with the boon %d, without %d; want %d and %d", earned(with, 0), earned(without, 0), 2*share, share)
	}
	if lines, _ := sentOf(k.sess[0]); len(lines) != 1 || lines[0] != fmt.Sprintf("You receive %d XP (boon x2).", 2*share) {
		t.Errorf("the line with the boon: %q", lines)
	}
	if lines, _ := sentOf(k.sess[1]); len(lines) != 1 || lines[0] != fmt.Sprintf("You receive %d XP.", share) {
		t.Errorf("the control's line: %q", lines)
	}

	before, other := earned(with, 0), earned(without, 0)
	k.w.AwardXP(k.sess[0], with, 150)
	k.w.AwardXP(k.sess[1], without, 150)
	if got, ctl := earned(with, 0)-before, earned(without, 0)-other; got != 300 || ctl != 150 {
		t.Errorf("a quest's 150: %d with the boon, %d without", got, ctl)
	}
}

// At the level cap the boon doubles the mastery experience a kill makes, and
// while a debt is owed it doubles the whole award, the debt's half included.
func TestABoonDoublesMasteriesAndTheDebtsHalf(t *testing.T) {
	npc := &NPC{ServerID: 5102, Name: "Froglok Slayer", Level: 60, X: 1000, Z: 1000}
	worth := groupExp(60, 60, []int{60})
	k := newKillWorld(t, 60)
	p := k.players[0]
	k.w.GiveBoon(k.sess[0], "Halas", time.Now())
	sentOf(k.sess[0])
	k.w.awardKillXPLocked(k.sess[0], p, npc)
	if earned(p, 0) != 2*worth {
		t.Errorf("a level 60's kill with the boon: %d, want %d", earned(p, 0), 2*worth)
	}
	if lines, _ := sentOf(k.sess[0]); len(lines) != 1 || lines[0] != fmt.Sprintf("You received %d class mastery XP (boon x2).", 2*worth) {
		t.Errorf("the mastery line: %q", lines)
	}

	d := newKillWorld(t, 60)
	q := d.players[0]
	q.Debt = 10 * worth
	d.w.GiveBoon(d.sess[0], "Halas", time.Now())
	d.w.awardKillXPLocked(d.sess[0], q, npc)
	if paid := 10*worth - q.Debt; paid != worth || earned(q, 10*worth) != 2*worth {
		t.Errorf("owing: %d paid of the debt, %d earned; want %d and %d", paid, earned(q, 10*worth), worth, 2*worth)
	}
}

// Talking to a board again puts its boon back to its whole time, and another
// town's board's takes its place: one boon on the player, one slot on the
// effects channel, each line its own. The control: the first talk's line is
// the blessing's.
func TestABoonRenewsAndNeverStacks(t *testing.T) {
	k := newKillWorld(t, 20)
	p := k.players[0]
	t0 := time.Now()
	g, _ := k.w.GiveBoon(k.sess[0], "Freeport", t0)
	if g.Line() != "You have been blessed by the Boon of Freeport: double experience for 4 hours of play." {
		t.Errorf("the first line: %q", g.Line())
	}
	t1 := t0.Add(90 * time.Minute)
	g, _ = k.w.GiveBoon(k.sess[0], "Halas", t1)
	if g.Replaced != "Boon of Freeport" || g.Left != 150*time.Minute {
		t.Errorf("renewed: %+v", g)
	}
	if g.Line() != "The Boon of Halas renews your blessing: double experience for 4 hours of play." {
		t.Errorf("the renewal's line: %q", g.Line())
	}
	n, e := boonsOn(p)
	if n != 1 || e.Name != "Boon of Halas" || e.Icon != BoonIcon || !e.Until.Equal(t1.Add(4*time.Hour)) {
		t.Fatalf("%d boons on, the first %+v", n, e)
	}
	if len(p.Buffs) != 1 || p.Buffs[0] != (BuffSlot{Icon: BoonIcon, Name: "Boon of Halas"}) {
		t.Errorf("the effects channel's slots: %+v", p.Buffs)
	}
	b := SerializeBuffUpdate(p)
	if binary.LittleEndian.Uint32(b) != 1 || binary.LittleEndian.Uint32(b[4:]) != BoonIcon {
		t.Errorf("channel 0x43: % x", b[:12])
	}
}

// A new boon is played on its bearer for those round it, as a buff's
// landing: the buff stand-in's 0x00B1 from the player at itself. A renewal
// plays none (a board talked to over and over casts nothing at everyone
// near).
func TestABoonIsSeen(t *testing.T) {
	k := newKillWorld(t, 20)
	k.sess[0].SetPlayerObjectID(4242)
	casts := func() [][3]uint32 { // caster, target, visual
		var out [][3]uint32
		for _, m := range k.sess[0].Queue().GetPending() {
			d := m.Data()
			if protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeCastingCombat {
				out = append(out, [3]uint32{binary.LittleEndian.Uint32(d[2:]), binary.LittleEndian.Uint32(d[6:]),
					binary.LittleEndian.Uint32(d[len(d)-4:])})
			}
		}
		return out
	}
	k.w.GiveBoon(k.sess[0], "Freeport", time.Now())
	if c := casts(); len(c) != 1 || c[0] != [3]uint32{4242, 4242, fxBuff} {
		t.Fatalf("0x00B1s: %x, want the buff's from the player at itself", c)
	}
	k.w.GiveBoon(k.sess[0], "Freeport", time.Now())
	if c := casts(); len(c) != 0 {
		t.Errorf("a renewal played %x", c)
	}
}

// A death takes every effect off but the boon, which goes on giving its share
// after the respawn and stays on the effects channel. The control: a meal on
// beside it goes with the death.
func TestABoonOutlivesADeath(t *testing.T) {
	k := newKillWorld(t, 20)
	p := k.players[0]
	now := time.Now()
	k.w.GiveBoon(k.sess[0], "Neriak", now)
	if err := k.w.Consume(k.sess[0], Consumable{Name: "Spiced Jerky", Icon: 7, Amount: 5}, now); err != nil {
		t.Fatal(err)
	}
	if len(p.Effects) != 2 || len(p.Buffs) != 2 {
		t.Fatalf("the control: %d effects, %d slots before the death", len(p.Effects), len(p.Buffs))
	}
	k.w.mu.Lock()
	k.w.playerDiedLocked(k.sess[0], p, now)
	due := k.w.playerRespawnSystem(now.Add(PlayerRespawnDelay))
	k.w.mu.Unlock()
	if len(due) != 1 {
		t.Fatalf("no respawn")
	}
	if n, _ := boonsOn(p); n != 1 || p.hasEffect(FoodEffectID) || len(p.Effects) != 1 {
		t.Fatalf("after the death: %d boons, the meal on %v, %d effects", n, p.hasEffect(FoodEffectID), len(p.Effects))
	}
	if len(p.Buffs) != 1 || p.Buffs[0].Name != "Boon of Neriak" {
		t.Errorf("the slots after the death: %+v", p.Buffs)
	}
	debt := p.Debt
	before := earned(p, debt)
	k.w.AwardXP(k.sess[0], p, 1000)
	if got := earned(p, debt) - before; got != 2000 {
		t.Errorf("after the respawn: %d earned of 1000, want 2000", got)
	}
}

// The boon is kept over a logout with what is left of it, its time running
// again at the login (buffsave.go), and gives its share there. The control:
// the same character's new player before the restore earns no more than an
// award.
func TestABoonIsKeptOverALogout(t *testing.T) {
	k := newKillWorld(t, 20)
	t0 := time.Now()
	k.w.GiveBoon(k.sess[0], "Oggok", t0)
	rows, ok := k.w.SavedEffects(k.players[0].ServerID, t0.Add(time.Hour))
	if !ok || len(rows) != 1 || rows[0].SpellID != BoonEffectID || rows[0].LeftMS != (3*time.Hour).Milliseconds() ||
		rows[0].Name != "Boon of Oggok" || rows[0].Icon != BoonIcon {
		t.Fatalf("kept: %+v %v", rows, ok)
	}

	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 2, 1), Port: 7300}, 1, 1, 1, 2, true)
	p := &Player{ServerID: 2001, Name: "Sharer", Level: 20, HP: 100, MaxHP: 100}
	w.Players[p.ServerID] = p
	s.SetECSEntity(p)
	s.SetCharacterInWorld(true)
	w.AwardXP(s, p, 100)
	if earned(p, 0) != 100 {
		t.Fatalf("the control: %d", earned(p, 0))
	}
	login := t0.Add(24 * time.Hour)
	if n := w.RestoreEffects(s, rows, login); n != 1 {
		t.Fatalf("restored %d", n)
	}
	if n, e := boonsOn(p); n != 1 || !e.Until.Equal(login.Add(3*time.Hour)) || len(p.Buffs) != 1 {
		t.Fatalf("after the login: %d boons, %+v", n, e)
	}
	w.AwardXP(s, p, 100)
	if earned(p, 0) != 300 {
		t.Errorf("after the login: %d earned, want 100 + 200", earned(p, 0))
	}
}

// The boon wears off at its time, and the next award is the plain one.
func TestABoonWearsOff(t *testing.T) {
	k := newKillWorld(t, 20)
	p := k.players[0]
	t0 := time.Now()
	k.w.GiveBoon(k.sess[0], "Qeynos", t0)
	k.w.mu.Lock()
	k.w.effectSystem(t0.Add(4*time.Hour - time.Second))
	k.w.mu.Unlock()
	if n, _ := boonsOn(p); n != 1 {
		t.Fatal("the control: gone before its time")
	}
	k.w.mu.Lock()
	k.w.effectSystem(t0.Add(4 * time.Hour))
	k.w.mu.Unlock()
	if n, _ := boonsOn(p); n != 0 || len(p.Buffs) != 0 {
		t.Fatalf("at its time: %d boons, slots %+v", n, p.Buffs)
	}
	sentOf(k.sess[0])
	k.w.AwardXP(k.sess[0], p, 100)
	if lines, _ := sentOf(k.sess[0]); earned(p, 0) != 100 || len(lines) != 1 || lines[0] != "You receive 100 XP." {
		t.Errorf("after it: %d earned, %q", earned(p, 0), lines)
	}
}

// A GM's change of the percent is every boon's at once, and 100 gives none:
// the boards give no boon then, and one on gives no more than an award. The
// time is a new boon's. Under 100 is 100: no boon takes experience away.
func TestABoonsSettingsCountAtOnce(t *testing.T) {
	k := newKillWorld(t, 20)
	p := k.players[0]
	k.w.GiveBoon(k.sess[0], "Freeport", time.Now())
	sentOf(k.sess[0])
	k.w.SetBoon(Boon{Percent: 300, Duration: time.Hour})
	k.w.AwardXP(k.sess[0], p, 100)
	if lines, _ := sentOf(k.sess[0]); earned(p, 0) != 300 || len(lines) != 1 || lines[0] != "You receive 300 XP (boon x3)." {
		t.Errorf("at 300%%: %d earned, %q", earned(p, 0), lines)
	}
	now := time.Now()
	g, ok := k.w.GiveBoon(k.sess[0], "Freeport", now)
	if _, e := boonsOn(p); !ok || !e.Until.Equal(now.Add(time.Hour)) || g.Line() != "The Boon of Freeport renews your blessing: triple experience for 1 hour of play." {
		t.Errorf("a boon after the change: %q, %+v", g.Line(), e)
	}

	k.w.SetBoon(Boon{Percent: 100, Duration: time.Hour})
	if _, ok := k.w.GiveBoon(k.sess[0], "Freeport", time.Now()); ok {
		t.Error("at 100% a board gave a boon")
	}
	sentOf(k.sess[0])
	before := earned(p, 0)
	k.w.AwardXP(k.sess[0], p, 100)
	if lines, _ := sentOf(k.sess[0]); earned(p, 0)-before != 100 || len(lines) != 1 || lines[0] != "You receive 100 XP." {
		t.Errorf("at 100%%, a boon on: %d earned, %q", earned(p, 0)-before, lines)
	}
	k.w.SetBoon(Boon{Percent: 40})
	if b := k.w.Boon(); b.Percent != 100 || b.Duration != DefaultBoon.Duration {
		t.Errorf("40%% and no time: %+v", b)
	}
}

// Nobody in the world, or a dead player, is given none.
func TestNoBoonForTheDead(t *testing.T) {
	k := newKillWorld(t, 20)
	p := k.players[0]
	p.HP = 0
	if _, ok := k.w.GiveBoon(k.sess[0], "Freeport", time.Now()); ok || len(p.Effects) != 0 {
		t.Errorf("a dead player was given one: %+v", p.Effects)
	}
	stranger := session.NewManager().CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 3, 1), Port: 7400}, 1, 1, 1, 2, true)
	if _, ok := k.w.GiveBoon(stranger, "Freeport", time.Now()); ok {
		t.Error("a session with no player was given one")
	}
}

// The words: whole hours and minutes, a part of a minute counted whole; the
// tag; what a percent gives.
func TestBoonWords(t *testing.T) {
	for d, want := range map[time.Duration]string{
		4 * time.Hour: "4 hours", time.Hour: "1 hour", 90 * time.Minute: "1 hour 30 minutes", 61 * time.Minute: "1 hour 1 minute",
		45 * time.Minute: "45 minutes", 30 * time.Second: "1 minute", 0: "no time",
	} {
		if got := spanWords(d); got != want {
			t.Errorf("%v: %q, want %q", d, got, want)
		}
	}
	if boonTag(200) != "boon x2" || boonTag(150) != "boon x1.5" {
		t.Errorf("tags: %q %q", boonTag(200), boonTag(150))
	}
	if (Boon{Percent: 150}).What() != "150% experience" || DefaultBoon.What() != "double experience" {
		t.Errorf("what: %q %q", (Boon{Percent: 150}).What(), DefaultBoon.What())
	}
	if BoonName("") != "Boon" || BoonName(" Surefall ") != "Boon of Surefall" {
		t.Errorf("names: %q %q", BoonName(""), BoonName(" Surefall "))
	}
}
