package entity

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/session"
)

// Food and drink against a world with one player: a 50 of its maxima is its
// own HoT and PoT (20 and 10 here), the meal's and the drink's on top.

func newEater(t *testing.T) (*World, *session.Session, *Player, time.Time) {
	t.Helper()
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 9), Port: 7000}, 1, 1, 1, 2, true)
	p := &Player{ServerID: 1001, Name: "Eater", Class: 0, Level: 20, HP: 500, MaxHP: 1000, Power: 100, MaxPower: 500}
	w.Players[p.ServerID] = p
	sess.SetECSEntity(p)
	return w, sess, p, time.Unix(1_800_000_000, 0)
}

const (
	breadIcon = 0x5A0D1B33
	waterIcon = 0xB10ADA3A
)

// A meal adds its HoT and a drink its PoT to the six-second regen and to the
// stat channel's HoT and PoT, each shown on the effects channel by its
// item's name and icon; a new meal replaces the last, and a meal and a drink
// stay on together.
func TestConsumeAddsToTheRegen(t *testing.T) {
	w, sess, p, now := newEater(t)
	if p.HoTNow() != 20 || p.PoTNow() != 10 {
		t.Fatalf("the control: its own HoT %d, PoT %d; want 20, 10", p.HoTNow(), p.PoTNow())
	}
	if !w.Consume(sess, Consumable{Name: "hearty bread", Icon: breadIcon, Amount: 10}, now) ||
		!w.Consume(sess, Consumable{Drink: true, Name: "well water", Icon: waterIcon, Amount: 2}, now) {
		t.Fatal("Consume refused a living player")
	}
	if p.HoTNow() != 30 || p.PoTNow() != 12 {
		t.Fatalf("with bread and water: HoT %d, PoT %d; want 30, 12", p.HoTNow(), p.PoTNow())
	}
	if len(p.Buffs) != 2 || p.Buffs[0] != (BuffSlot{Icon: breadIcon, Name: "hearty bread"}) ||
		p.Buffs[1] != (BuffSlot{Icon: waterIcon, Name: "well water"}) {
		t.Fatalf("effects channel %+v", p.Buffs)
	}
	p.regenTick()
	if p.HP != 530 || p.Power != 112 {
		t.Errorf("a tick: %d HP, %d power; want 530, 112", p.HP, p.Power)
	}
	b := SerializeStatUpdate(p)
	if pot, hot := int32(binary.LittleEndian.Uint32(b[48:])), int32(binary.LittleEndian.Uint32(b[52:])); pot != 12 || hot != 30 {
		t.Errorf("stat channel PoT %d HoT %d; want 12, 30", pot, hot)
	}

	// a better meal replaces the bread: its HoT alone, the drink kept
	w.Consume(sess, Consumable{Name: "pie", Icon: 0x1234, Amount: 49}, now.Add(10*time.Second))
	if p.HoTNow() != 20+49 || p.PoTNow() != 12 || len(p.Effects) != 2 {
		t.Errorf("after pie: HoT %d PoT %d, %d effects; want 69, 12, 2", p.HoTNow(), p.PoTNow(), len(p.Effects))
	}
	if p.Buffs[len(p.Buffs)-1].Name != "pie" {
		t.Errorf("the new meal goes last on the channel: %+v", p.Buffs)
	}

	// no meal for the dead
	p.HP = 0
	if w.Consume(sess, Consumable{Name: "boiled egg", Amount: 14}, now) {
		t.Error("a dead player ate")
	}
}

// A meal lasts a minute, then its HoT is gone.
func TestConsumableLastsAMinute(t *testing.T) {
	w, sess, p, now := newEater(t)
	w.Consume(sess, Consumable{Name: "hearty bread", Icon: breadIcon, Amount: 10}, now)
	w.mu.Lock()
	defer w.mu.Unlock()
	w.effectSystem(now.Add(ConsumableDuration - time.Second))
	if p.HoTNow() != 30 {
		t.Fatalf("a second short of the minute: HoT %d, want 30", p.HoTNow())
	}
	w.effectSystem(now.Add(ConsumableDuration))
	if p.HoTNow() != 20 || len(p.Buffs) != 0 {
		t.Errorf("after the minute: HoT %d, %d on the channel; want 20, 0", p.HoTNow(), len(p.Buffs))
	}
}

// A meal and a drink end once their eater engages, lands a blow or a harmful
// spell, or is hit, after eating (LastCombatTime moves past the meal). The
// control: a fight that ended before the meal leaves it on.
func TestConsumableEndsInAFight(t *testing.T) {
	for _, c := range []struct {
		name   string
		fight  func(p *Player, at time.Time)
		broken bool
	}{
		{"a fight before the meal", func(p *Player, at time.Time) { p.LastCombatTime = at.Add(-10 * time.Second) }, false},
		{"nothing", func(*Player, time.Time) {}, false},
		{"auto-attack engaged", func(p *Player, _ time.Time) { p.AttackTarget = 2002 }, true},
		{"hit, or a blow landed", func(p *Player, at time.Time) { p.LastCombatTime = at.Add(5 * time.Second) }, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			w, sess, p, now := newEater(t)
			w.Consume(sess, Consumable{Name: "hearty bread", Icon: breadIcon, Amount: 10}, now)
			w.Consume(sess, Consumable{Drink: true, Name: "well water", Icon: waterIcon, Amount: 2}, now)
			c.fight(p, now)
			w.mu.Lock()
			w.consumableSystem(now.Add(6 * time.Second))
			w.mu.Unlock()
			if c.broken {
				if len(p.Effects) != 0 || len(p.Buffs) != 0 || p.HoTNow() != 20 || p.PoTNow() != 10 {
					t.Errorf("still on: %d effects, %d on the channel, HoT %d PoT %d", len(p.Effects), len(p.Buffs), p.HoTNow(), p.PoTNow())
				}
				return
			}
			if len(p.Effects) != 2 || p.HoTNow() != 30 || p.PoTNow() != 12 {
				t.Errorf("ended: %d effects, HoT %d PoT %d", len(p.Effects), p.HoTNow(), p.PoTNow())
			}
		})
	}
}

// A fight ends a meal but no spell's effect.
func TestConsumableBreakLeavesSpells(t *testing.T) {
	w, sess, p, now := newEater(t)
	w.Consume(sess, Consumable{Name: "hearty bread", Icon: breadIcon, Amount: 10}, now)
	w.mu.Lock()
	defer w.mu.Unlock()
	w.addEffectLocked(p, &ActiveEffect{SpellID: 458, Name: "Rockskin", Icon: 7, Until: now.Add(30 * time.Minute), Mods: StatMods{AC: 15}}, now)
	p.LastCombatTime = now.Add(time.Second)
	w.consumableSystem(now.Add(2 * time.Second))
	if len(p.Effects) != 1 || p.Effects[0].SpellID != 458 || p.Mods.AC != 15 || p.Mods.HoT != 0 {
		t.Errorf("after the fight: %d effects (%+v), mods %+v", len(p.Effects), p.Effects, p.Mods)
	}
}

// A meal is kept over a logout with what is left of it, as spells' effects
// are (buffsave.go), and its HoT comes back with it.
func TestConsumableKeptOverALogout(t *testing.T) {
	w, sess, p, now := newEater(t)
	w.Consume(sess, Consumable{Name: "hearty bread", Icon: breadIcon, Amount: 10}, now)
	rows, ok := w.SavedEffects(p.ServerID, now.Add(20*time.Second))
	if !ok || len(rows) != 1 || rows[0].SpellID != FoodEffectID || rows[0].LeftMS != 40_000 {
		t.Fatalf("saved %+v", rows)
	}
	w2, sess2, p2, later := newEater(t)
	if n := w2.RestoreEffects(sess2, rows, later); n != 1 {
		t.Fatalf("restored %d", n)
	}
	if p2.HoTNow() != 30 || len(p2.Buffs) != 1 || p2.Buffs[0].Name != "hearty bread" {
		t.Errorf("restored: HoT %d, channel %+v", p2.HoTNow(), p2.Buffs)
	}
	// it still ends in the next fight
	p2.LastCombatTime = later.Add(time.Second)
	w2.mu.Lock()
	w2.consumableSystem(later.Add(2 * time.Second))
	w2.mu.Unlock()
	if p2.HoTNow() != 20 {
		t.Errorf("restored meal through a fight: HoT %d", p2.HoTNow())
	}
}
