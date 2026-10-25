package entity

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// The spells for undead alone (F3 20261025-215937, -222603, -220159): a
// "Target undead" line, or a charm its name or description says is for
// undead, refused at anything that is not with a fizzle and a red line.

func wardDeath() CastSpell {
	return CastSpell{SpellID: 333, DisplayIndex: 9, Level: 13, Scope: 1, Name: "Ward Death", CastTime: 2 * time.Second,
		Recast: time.Second, PowerCost: 40, Range: 30,
		Def: ParseSpellNamed("Ward Death", "Target undead, Decrease Hitpoints by 62 ; Increase 30% based on WIS",
			"Magical energy assaults undead creatures. Modified by WIS.")}
}

func coerceUndead() CastSpell {
	// its line as eqoaclassic keeps it: no target word, and nothing in it but
	// its name and description says undead
	return CastSpell{SpellID: 974, DisplayIndex: 10, Level: 50, Scope: 1, Name: "Coerce Undead", CastTime: 5 * time.Second,
		Recast: time.Second, PowerCost: 100, Range: 30,
		Def: ParseSpellNamed("Coerce Undead", "Charm ; Increase 25% based on CHA", "CM: Charm an Undead")}
}

func TestTheSpellsForUndeadAloneAreRead(t *testing.T) {
	for _, c := range []struct {
		name, line, desc string
		undead           bool
	}{
		{"Ward Death", "Target undead, Decrease Hitpoints by 62 ; Increase 30% based on WIS", "Magical energy assaults undead creatures. Modified by WIS.", true},
		{"Punish Death", "Target undead, Decrease Hitpoints by 118 ; Increase 30% based on INT", "Allows you to punish the undead with a burning darkness. Modified by INT.", true},
		{"Weaken Death", "Target undead, Decrease STR/AGI by 27 ; Increase 15% based on WIS", "", true},
		{"Disable Undead", "Target undead, Decrease HP by 254 ; Increase 30% based on WIS, AOE Decrease HP by 140 ; Increase 15% based on WIS", "", true},
		{"Coerce Undead", "Charm ; Increase 25% based on CHA", "CM: Charm an Undead", true},
		{"Charm Undead", "Target, Charm ; Increase 25% based on CHA", "CM: Charm an Undead.", true},
		// the controls: what mentions undead but is not cast at one
		{"Entrancing Captivation", "Target, Charm ; Increase 25% based on CHA", "Charms an enemy to aid you.", false},
		{"Divine Protection", "Self, Damage Shield Decrease Hitpoints by 2, 5min", "A protective aura injures all undead who come in contact with it.", false},
		{"Reinforce Bone +", "Pet, Increase STR & DEX by 20, 30min", "CM: Reinforces the bones of your undead servant, increasing strength and dexterity for thirty minutes. Modified by STA.", false},
		{"Summon: Undead Mammoth", "", "Summons an undead mammoth companion.", false},
		{"Vexation", "Target, Decrease Hitpoints by 108 ; Increase 15% based on DEX", "", false},
	} {
		d := ParseSpellNamed(c.name, c.line, c.desc)
		if d.undeadOnly() != c.undead {
			t.Errorf("%s: for undead alone %v, want %v (%+v)", c.name, d.undeadOnly(), c.undead, d)
		}
		if c.undead && !d.Harmful {
			t.Errorf("%s: not cast at an enemy", c.name)
		}
	}
	if d := ParseSpellNamed("Ward Death", "Target undead, Decrease Hitpoints by 62 ; Increase 30% based on WIS", ""); len(d.Effects) != 1 ||
		d.Effects[0].Kind != EffectHP || d.Effects[0].Amount != -62 || d.Effects[0].Scale != StatWis {
		t.Errorf("Ward Death's hit: %+v", d.Effects)
	}
}

// A name says undead in whole words: the "bone" of the old list made the
// Hatebone orcs, Bonefish and a Blue-Ribboned Snake undead.
func TestUndeadNames(t *testing.T) {
	for name, want := range map[string]bool{
		"Skeleton": true, "skeleton ": true, "Skeletal Guard": true, "Decaying Zombie": true, "Lesser Ghoul": true,
		"Defiled Mummy": true, "Spectre": true, "Wandering Ghost": true, "Rotting Flesh ": true, "Crumbling Bones": true,
		"Walking Dead": true, "Lost Soul": true, "Wandering Ghast": true, "Undead Farmer": true,
		"Hatebone Guard": false, "Bonefish": false, "Hexbone": false, "Bone Gnawer": false, "Blue-Ribboned Snake": false,
		"Deathfist Runt": false, "Howling Death": false, "Desert Crab": false, "Guard Dahvel": false, "Soulbinder Tara": false,
	} {
		if got := undeadName(name); got != want {
			t.Errorf("undeadName(%q) = %v, want %v", name, got, want)
		}
	}
}

// The GMs' tags (npcundead) come before the name, either way; a name with no
// row goes by its words. Keys as npcclass's: any case, trimmed, no '$'.
func TestUndeadTagsComeBeforeTheName(t *testing.T) {
	w := NewWorld()
	w.SetUndeadTags(map[string]bool{"hexbone": true, "rotting flesh": false})
	for name, want := range map[string]bool{
		"Hexbone": true, "Rotting Flesh ": false, "Rotting Flesh$": false, "Skeleton": true, "Desert Crab": false,
	} {
		if got := w.undeadLocked(&NPC{Name: name}); got != want {
			t.Errorf("%q undead %v, want %v", name, got, want)
		}
	}
}

// refusedNotUndead checks a refusal at its start: the fizzle's brief cast
// (0x00B1 of charmFizzle) and the line, nothing taken.
func refusedNotUndead(t *testing.T, r *petRig, began bool, msgs [][]byte, power, lockouts int) {
	t.Helper()
	if began || len(msgs) != 2 || opOfMsg(msgs[0]) != protocol.OpcodeCastingCombat || !bytes.Contains(msgs[1], wide16(castNotUndead)) {
		t.Fatalf("began %v, %x", began, msgs)
	}
	if ms := binary.LittleEndian.Uint32(msgs[0][11:]); ms != uint32(charmFizzle/time.Millisecond) {
		t.Errorf("the fizzle lasts %d ms", ms)
	}
	if !bytes.HasSuffix(msgs[1], castRefusalRGB[:]) {
		t.Errorf("the line is not red: %x", msgs[1])
	}
	if r.p.Power != power || len(r.p.Lockouts) != lockouts {
		t.Errorf("power %d (was %d), lockouts %v (were %d)", r.p.Power, power, r.p.Lockouts, lockouts)
	}
}

// Ward Death at the crab (a living creature): refused, nothing taken; at
// Rotting Flesh it is cast and lands. A GM's tag makes the crab undead.
func TestAnUndeadNukeTakesOnlyUndead(t *testing.T) {
	r, flesh, fleshID := charmRig(t)
	r.p.Name, r.p.Class, r.p.Wis = "Gnolie", 9, 120
	power, lockouts := r.p.Power, len(r.p.Lockouts)
	began, msgs := castCharm(t, r, wardDeath(), crabID, 0)
	refusedNotUndead(t, r, began, msgs, power, lockouts)

	flesh.HP, flesh.MaxHP = 400, 400
	if began, _ := castCharm(t, r, wardDeath(), fleshID, time.Second); !began || flesh.HP >= 400 {
		t.Errorf("on Rotting Flesh: began %v, its HP %d", began, flesh.HP)
	}

	r2, _, _ := charmRig(t)
	r2.w.SetUndeadTags(map[string]bool{"desert crab": true})
	if began, _ := castCharm(t, r2, wardDeath(), crabID, 0); !began || r2.enemy.HP >= 2040 {
		t.Errorf("a crab tagged undead: began %v, its HP %d", began, r2.enemy.HP)
	}
	r3, _, fleshID3 := charmRig(t)
	r3.w.SetUndeadTags(map[string]bool{"rotting flesh": false})
	power, lockouts = r3.p.Power, len(r3.p.Lockouts)
	began, msgs = castCharm(t, r3, wardDeath(), fleshID3, 0)
	refusedNotUndead(t, r3, began, msgs, power, lockouts)
}

// Coerce Undead: refused at the crab with the fizzle; Rotting Flesh, undead,
// is taken on the charm's roll (and resisted on a roll under it), where the
// enchanter's charm always fails on it.
func TestTheUndeadCharmTakesOnlyUndead(t *testing.T) {
	defer func(f func(int) int) { charmIntn = f }(charmIntn)
	charmIntn = dice(99)
	r, flesh, fleshID := charmRig(t)
	r.p.Name, r.p.Class, r.p.Level = "Ulgulu", 11, 60
	power, lockouts := r.p.Power, len(r.p.Lockouts)
	began, msgs := castCharm(t, r, coerceUndead(), crabID, 0)
	refusedNotUndead(t, r, began, msgs, power, lockouts)

	if began, _ := castCharm(t, r, coerceUndead(), fleshID, time.Second); !began || flesh.Owner != r.p.ServerID || r.w.petOfLocked(r.p) != flesh {
		t.Fatalf("on Rotting Flesh: began %v, owner %d", began, flesh.Owner)
	}

	charmIntn = dice(0)
	r2, flesh2, fleshID2 := charmRig(t)
	r2.p.Class, r2.p.Level = 11, 60
	_, msgs = castCharm(t, r2, coerceUndead(), fleshID2, 0)
	resisted := false
	for _, m := range msgs {
		resisted = resisted || bytes.Contains(m, wide16(charmResisted))
	}
	if !resisted || flesh2.Owner != 0 {
		t.Errorf("a roll under the resist: resisted %v, owner %d", resisted, flesh2.Owner)
	}

	charmIntn = dice(99)
	r3, flesh3, fleshID3 := charmRig(t)
	if _, _ = castCharm(t, r3, captivation(), fleshID3, 0); flesh3.Owner != 0 {
		t.Errorf("the enchanter's charm took Rotting Flesh")
	}
}
