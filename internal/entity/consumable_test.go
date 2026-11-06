package entity

import (
	"encoding/binary"
	"encoding/hex"
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
	if err := w.Consume(sess, Consumable{Name: "hearty bread", Icon: breadIcon, Amount: 10}, now); err != nil {
		t.Fatal(err)
	}
	if err := w.Consume(sess, Consumable{Drink: true, Name: "well water", Icon: waterIcon, Amount: 2}, now); err != nil {
		t.Fatal(err)
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

	// no meal in a fight (it would end at once), nothing for the dead;
	// nothing changes
	p.AttackTarget = 2002
	if err := w.Consume(sess, Consumable{Name: "boiled egg", Amount: 14}, now); err != ErrConsumeFighting {
		t.Errorf("in a fight: %v, want ErrConsumeFighting", err)
	}
	p.AttackTarget, p.HP = 0, 0
	if err := w.Consume(sess, Consumable{Name: "boiled egg", Amount: 14}, now); err != ErrConsumeNoPlayer {
		t.Errorf("dead: %v, want ErrConsumeNoPlayer", err)
	}
	if p.HoTNow() != 20+49 || len(p.Effects) != 2 {
		t.Errorf("after the refusals: HoT %d, %d effects", p.HoTNow(), len(p.Effects))
	}
	// but a drink in a fight, as Sandstorm took Elven Wine mid-fight
	p.AttackTarget, p.HP = 2002, 500
	if err := w.Consume(sess, Consumable{Drink: true, Name: "Elven Wine", Amount: 5}, now); err != nil {
		t.Errorf("a drink in a fight: %v, want it drunk", err)
	}
	if p.PoTNow() != 10+5 {
		t.Errorf("the drink in a fight: PoT %d, want 15", p.PoTNow())
	}
}

// Eating or drinking plays Sandstorm's visual on the player: its own 0x00B1
// at itself, cast 0, 0xCF84F260 (the cheer, its sound and particles), in
// Sandstorm's bytes with this player's id. The control: a dead player's
// plays nothing.
func TestConsumedPlaysTheCheer(t *testing.T) {
	w, sess, p, _ := newEater(t)
	sess.SetCharacterInWorld(true)
	sess.SetPlayerObjectID(0x284AEFD9)
	w.Consumed(sess)
	msgs := sess.Queue().GetPending()
	if len(msgs) != 1 || hex.EncodeToString(msgs[0].Data()) != "b100d9ef4a28d9ef4a280000000000"+"60f284cf" {
		var got []string
		for _, m := range msgs {
			got = append(got, hex.EncodeToString(m.Data()))
		}
		t.Fatalf("sent %v, want Sandstorm's b100d9ef4a28d9ef4a28000000000060f284cf", got)
	}
	p.HP = 0
	w.Consumed(sess)
	if n := len(sess.Queue().GetPending()); n != 0 {
		t.Errorf("a dead player's: %d messages", n)
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

// A meal ends once its eater engages, lands a blow or a harmful spell, or is
// hit, after eating (LastCombatTime moves past the meal); a drink runs on
// through it (Sandstorm's Elven Wine gave its power through its owner's
// fight). The control: a fight that ended before the meal leaves it on.
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
				if len(p.Effects) != 1 || p.Effects[0].SpellID != DrinkEffectID || len(p.Buffs) != 1 || p.HoTNow() != 20 || p.PoTNow() != 12 {
					t.Errorf("the meal still on, or the drink gone: %d effects, %d on the channel, HoT %d PoT %d", len(p.Effects), len(p.Buffs), p.HoTNow(), p.PoTNow())
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

// A meal or a drink with a foodeffect row (EQOAEmu's workbook, the import of
// 2026-11-01): its own time, ended by any spell its eater casts when its row
// says so, and by a fight when its row says so, a drink too, which is then
// refused in a fight; kept over a logout with its rule. The control: a drink
// whose row says no contact runs through a fight, as one with no row does.
func TestConsumableRuleFromItsRow(t *testing.T) {
	w, sess, p, now := newEater(t)
	ale := Consumable{Drink: true, Name: "pale ale", Icon: waterIcon, Amount: 12, Duration: 2 * time.Minute,
		Rule: &FoodRule{EndsOnCast: true, EndsOnContact: true}}
	cider := Consumable{Drink: true, Name: "plain cider", Icon: waterIcon, Amount: 5, Duration: 2 * time.Minute,
		Rule: &FoodRule{}}
	if err := w.Consume(sess, ale, now); err != nil {
		t.Fatal(err)
	}
	w.mu.Lock()
	w.effectSystem(now.Add(ConsumableDuration + time.Second))
	w.mu.Unlock()
	if p.PoTNow() != 22 {
		t.Fatalf("a second past the minute: PoT %d, want 22 (its row's two minutes)", p.PoTNow())
	}
	// a cast ends it
	w.mu.Lock()
	w.endFoodOnCastLocked(p)
	w.mu.Unlock()
	if p.PoTNow() != 10 || len(p.Effects) != 0 {
		t.Fatalf("after a cast: PoT %d, %d effects", p.PoTNow(), len(p.Effects))
	}
	// a fight ends a drink whose row says contact, and refuses one in a fight
	w.Consume(sess, ale, now)
	p.LastCombatTime = now.Add(time.Second)
	w.mu.Lock()
	w.consumableSystem(now.Add(2 * time.Second))
	w.mu.Unlock()
	if len(p.Effects) != 0 {
		t.Fatalf("a contact drink through a fight: %d effects", len(p.Effects))
	}
	p.AttackTarget = 2002
	if err := w.Consume(sess, ale, now.Add(3*time.Second)); err != ErrConsumeFighting {
		t.Fatalf("a contact drink in a fight: %v, want ErrConsumeFighting", err)
	}
	if err := w.Consume(sess, cider, now.Add(3*time.Second)); err != nil {
		t.Fatalf("a drink no fight ends, in a fight: %v", err)
	}
	p.AttackTarget = 0
	w.mu.Lock()
	w.consumableSystem(now.Add(4 * time.Second))
	w.endFoodOnCastLocked(p) // its row says no cast either
	w.mu.Unlock()
	if p.PoTNow() != 15 {
		t.Fatalf("the cider through a fight and a cast: PoT %d, want 15", p.PoTNow())
	}
	// over a logout the rule comes back with it (the ale replaces the cider:
	// one drink at a time)
	w.Consume(sess, ale, now.Add(5*time.Second))
	rows, _ := w.SavedEffects(p.ServerID, now.Add(10*time.Second))
	w2, sess2, p2, later := newEater(t)
	w2.RestoreEffects(sess2, rows, later)
	if p2.PoTNow() != 22 {
		t.Fatalf("restored: PoT %d, want 22", p2.PoTNow())
	}
	w2.mu.Lock()
	w2.endFoodOnCastLocked(p2)
	w2.mu.Unlock()
	if p2.PoTNow() != 10 || len(p2.Effects) != 0 {
		t.Errorf("a restored ale through a cast: PoT %d, %d effects; want 10, 0", p2.PoTNow(), len(p2.Effects))
	}
}

// An item with both a HoT and a PoT (the workbook's Blessed Fish) gives both
// from the meal's slot: a drink beside it adds its own, another meal
// replaces it whole.
func TestConsumableGivesBoth(t *testing.T) {
	w, sess, p, now := newEater(t)
	w.Consume(sess, Consumable{Name: "blessed fish", Icon: breadIcon, Amount: 33, Also: 33}, now)
	if p.HoTNow() != 53 || p.PoTNow() != 43 {
		t.Fatalf("the fish: HoT %d PoT %d, want 53, 43", p.HoTNow(), p.PoTNow())
	}
	w.Consume(sess, Consumable{Drink: true, Name: "well water", Icon: waterIcon, Amount: 2}, now)
	if p.PoTNow() != 45 {
		t.Fatalf("the fish and a drink: PoT %d, want 45", p.PoTNow())
	}
	w.Consume(sess, Consumable{Name: "hearty bread", Icon: breadIcon, Amount: 10}, now)
	if p.HoTNow() != 30 || p.PoTNow() != 12 {
		t.Errorf("bread after the fish: HoT %d PoT %d, want 30, 12", p.HoTNow(), p.PoTNow())
	}
}

// A ration's HoT and PoT are its eater's (the user, 2026-11-06: "scale with
// level, and doesnt take too long to restore"): a tenth of its most health
// and power a tick, whatever the ration's own figures, so with the regen's
// own fiftieth an empty bar is full within the minute; at least its level
// each, and no power for one with none. The control: a meal of the same
// figures without the flag gives its own.
func TestARationIsItsEatersTenth(t *testing.T) {
	w, sess, p, now := newEater(t) // level 20, 1000 HP, 500 power
	if err := w.Consume(sess, Consumable{Name: "Food", Amount: 1, Ration: true}, now); err != nil {
		t.Fatal(err)
	}
	if p.HoTNow() != 20+100 || p.PoTNow() != 10+50 {
		t.Fatalf("a ration at 1000 HP and 500 power: HoT %d PoT %d; want 120, 60", p.HoTNow(), p.PoTNow())
	}
	p.HP, p.Power = 0+1, 0
	for i := 0; i < 9; i++ {
		p.regenTick()
	}
	if p.HP != 1000 || p.Power != 500 {
		t.Errorf("nine ticks from empty (54 s): %d HP, %d power; want both full", p.HP, p.Power)
	}
	// a level 5 warrior of 40 HP and no power: its level, no PoT
	w2, sess2, low, _ := newEater(t)
	low.Level, low.MaxHP, low.HP, low.MaxPower, low.Power = 5, 40, 10, 0, 0
	if err := w2.Consume(sess2, Consumable{Name: "Food", Amount: 1, Ration: true}, now); err != nil {
		t.Fatal(err)
	}
	// (its own PoT is the regen's least, 1: nothing of the ration's)
	if hot, pot := RationRestore(low); hot != 5 || pot != 0 || low.HoTNow() != 1+5 || low.PoTNow() != 1 {
		t.Errorf("a level 5 of 40 HP and no power: ration %d/%d, HoT %d PoT %d; want 5/0, 6, 1", hot, pot, low.HoTNow(), low.PoTNow())
	}
	// the control: the same figures as a meal
	w3, sess3, q, _ := newEater(t)
	if err := w3.Consume(sess3, Consumable{Name: "bread", Amount: 1}, now); err != nil {
		t.Fatal(err)
	}
	if q.HoTNow() != 20+1 || q.PoTNow() != 10 {
		t.Errorf("a meal of 1: HoT %d PoT %d; want 21, 10", q.HoTNow(), q.PoTNow())
	}
}
