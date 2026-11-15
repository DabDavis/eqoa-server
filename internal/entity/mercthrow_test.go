package entity

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// An alchemist merc throws its potions (mercthrow.go; the owner, 2026-11-15:
// "the alchemist mercs should be throwing their pots but arent currently"):
// its round a player alchemist's Range1 throw, shown as 0x00B1 with Range1's
// effect and its 3.5 s delay, no 2 s cast; its book's potion spell thrown,
// never cast; the potion its mixture summons thrown when it is the better;
// a cocktail's splash on the creatures round its target; its other spells
// still cast between its throws.

const (
	testVolatile = 50003
	testCocktail = 50018
)

func alcPotions(w *World) {
	vol := NewPotionEffect(testVolatile, "Volatile Potion", "Target, Decrease Hitpoints by 300", "", 0, "", 0, 0)
	ck := NewPotionEffect(testCocktail, "Klik'Anon Cocktail", "Target, Decrease Hitpoints by 214", "", 0, "", 0, 10)
	w.SetPotionEffects([]PotionEffect{vol, ck})
}

// castsSeen is the 0x00B1s the session was sent: caster, target, delay, effect.
type castSeen struct {
	caster, target, ms, fx uint32
}

func (r *mercRig) castsSeen() []castSeen {
	var out []castSeen
	for _, m := range r.sess.Queue().GetPending() {
		b := m.Data()
		if opOfMsg(b) != protocol.OpcodeCastingCombat {
			continue
		}
		n := int(b[10])
		at := 11 + 4*n
		out = append(out, castSeen{binary.LittleEndian.Uint32(b[2:]), binary.LittleEndian.Uint32(b[6:]),
			binary.LittleEndian.Uint32(b[at:]), binary.LittleEndian.Uint32(b[at+4:])})
	}
	return out
}

// seeRig puts ids on the owner's entity slots, as its visibility would.
func (r *mercRig) seeRig(ids ...EntityID) {
	vs := GetOrCreateVisibility(r.sess)
	for i, id := range ids {
		vs.ChannelEntity[i+1], vs.ChannelObjectID[i+1] = id, 0x01010000+uint32(id)
	}
}

func TestMercAlchemistThrowsItsPotions(t *testing.T) {
	r := newMercRig(t)
	alcPotions(r.w)
	fire := bookSpell(700606, "Freeport Fire Potion", 1, 2*time.Second, 3*time.Second, 231, "TARGET Debuff HP by 120")
	mix := bookSpell(700085, "Volatile Mixture", 0, 2*time.Second, 3*time.Second, 20, "Self, Summon Volatile Potion x3")
	spray := bookSpell(700033, "Fire Spray", 1, 2*time.Second, 6*time.Second, 30, "Target, Decrease Hitpoints by 66")
	k := r.kit(14, 0, 0, fire, mix, spray)
	k.Look.Primary, k.Look.PrimaryType, k.Look.Secondary, k.Look.SecondaryType = 0, 0, 0x45700E37, 10
	ms := r.hire(k, MercAssist)
	m := ms.npc
	crab := r.creature("Desert Crab", 28, 10)
	crab.HP, crab.MaxHP = 900000, 900000
	r.seeRig(m.ServerID, crab.ServerID)
	r.run(0, time.Second)
	r.sess.Queue().GetPending()
	ms.ordered = crab.ServerID
	r.run(1100*time.Millisecond, 11*time.Second)
	if _, cast := ms.ready[fire.SpellID]; cast {
		t.Error("its potion spell was cast as a spell")
	}
	throws, sprays := 0, 0
	for _, c := range r.castsSeen() {
		if c.caster != 0x01010000+uint32(m.ServerID) || c.target != 0x01010000+uint32(crab.ServerID) {
			continue
		}
		switch c.fx {
		case HashResourceID("Range1"):
			throws++
			if c.ms != 3500 {
				t.Errorf("a throw shown with %d ms, want Range1's 3500", c.ms)
			}
		default:
			sprays++
		}
	}
	// ten seconds: a throw every 3.5 s, Fire Spray (its cast time and recast) between
	if throws < 2 || throws > 4 {
		t.Errorf("%d throws in ten seconds, want a Range1 round's (3)", throws)
	}
	if sprays == 0 {
		t.Error("it cast no spell between its throws")
	}
	if crab.HP >= crab.MaxHP {
		t.Fatal("no potion landed")
	}
	if crab.CurrentEnemy != m.ServerID {
		t.Errorf("the crab fights %d, not the merc", crab.CurrentEnemy)
	}
}

// The better of a potion spell and a mixture's potion is thrown: the mixture's
// Volatile Potion (300) over the potion spell's 120.
func TestMercAlchemistThrowsTheBest(t *testing.T) {
	r := newMercRig(t)
	alcPotions(r.w)
	fire := bookSpell(700606, "Freeport Fire Potion", 1, 2*time.Second, 3*time.Second, 231, "TARGET Debuff HP by 120")
	mix := bookSpell(700085, "Volatile Mixture", 0, 2*time.Second, 3*time.Second, 20, "Self, Summon Volatile Potion x3")
	ms := r.hire(r.kit(14, 0, 0, fire, mix), MercAssist)
	crab := r.creature("Crab", 28, 10)
	r.w.mu.Lock()
	got, ok := r.w.mercThrowPickLocked(ms, crab)
	r.w.mu.Unlock()
	if !ok || got.pe.PatternID != testVolatile {
		t.Fatalf("throws %+v, want the Volatile Potion", got.pe.ItemName)
	}
	// with the mixture turned off in its menu, the potion spell
	ms.kit.SpellsOff = []int{mix.SpellID}
	r.w.mu.Lock()
	got, ok = r.w.mercThrowPickLocked(ms, crab)
	r.w.mu.Unlock()
	if !ok || got.spell != fire.SpellID {
		t.Fatalf("throws %q, want Freeport Fire Potion", got.pe.ItemName)
	}
	// another class throws nothing
	r2 := newMercRig(t)
	alcPotions(r2.w)
	wiz := r2.hire(r2.kit(13, 0, 0, fire, mix), MercAssist)
	r2.w.mu.Lock()
	_, ok = r2.w.mercThrowPickLocked(wiz, crab)
	r2.w.mu.Unlock()
	if ok {
		t.Error("a wizard merc throws potions")
	}
}

// A cocktail's splash lands on the creatures round its target, as
// throwLocked's, never on another's pet.
func TestMercAlchemistCocktailSplash(t *testing.T) {
	r := newMercRig(t)
	alcPotions(r.w)
	ck := bookSpell(700195, "Klik'Anon Cocktail", 0, 2*time.Second, 3*time.Second, 20, "Self, Summon Klik'Anon Cocktail x3")
	ms := r.hire(r.kit(14, 0, 0, ck), MercAssist)
	a := r.creature("Crab A", 28, 10)
	b := r.creature("Crab B", 28, 13)
	pet := r.creature("Someone's Pet", 28, 12)
	pet.Owner = 999999
	r.run(0, time.Second)
	ms.ordered = a.ServerID
	r.run(1100*time.Millisecond, 3*time.Second)
	if a.HP >= a.MaxHP || b.HP >= b.MaxHP {
		t.Fatalf("the cocktail's target %d/%d, its splash %d/%d", a.HP, a.MaxHP, b.HP, b.MaxHP)
	}
	if pet.HP < pet.MaxHP {
		t.Error("the splash hit another's pet")
	}
}
