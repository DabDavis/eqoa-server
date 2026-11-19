package entity

import (
	"testing"
	"time"
)

// 2026-11-18, a GM (level 60 enchanter, Live): a level 2 bat he had
// charmed went free and hit him for 13-22 every 3 s; a hatchling of the same
// levels hit him for 15-21 uncharmed. The charm changed nothing that stayed
// (charm.go's unCharmLocked): the hits came from the caster's archetype
// offset (31.7), whole on a creature far below the player (defence.go's
// offsetShare).

// highDice is the roll's top every time.
func highDice(n int) int { return n - 1 }

// npcHitsOn is the NPC's hit on the player at the roll's top, as
// npcMeleeOnPlayerLocked figures it before the mitigation.
func npcHitsOn(w *World, npc *NPC, p *Player) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	src := npc.figures()
	return npc.shared(w.npcHitPlayerLocked(src, p, npcMeleeOn(src, p, highDice)))
}

func TestAFreedCharmHitsAsItDidUncharmed(t *testing.T) {
	defer func(f func(int) int) { charmIntn = f }(charmIntn)
	charmIntn = dice(99)
	for _, c := range []struct {
		name    string
		end     func(r *petRig)
		hostile bool
	}{
		{"worn off", func(r *petRig) { r.tick(4*time.Minute + 40*time.Second); r.tick(4*time.Minute + 50*time.Second) }, true},
		{"dismissed", func(r *petRig) { r.w.PetCommand(r.sess, petCmdDismiss, 0, r.at(time.Minute)) }, true},
		{"its master logged out", func(r *petRig) { delete(r.w.Players, r.p.ServerID); r.tick(time.Minute) }, false},
	} {
		r, _, _ := charmRig(t)
		r.p.Level = 60
		bat := r.enemy
		bat.Level = 2
		before := npcHitsOn(r.w, bat, r.p)
		if before > 2 { // a naked one (no AC, no AGI); the caster's whole offset made it 33
			t.Errorf("%s: a level 2 creature hits a level 60 enchanter for %d uncharmed, want at most 2", c.name, before)
		}
		maxHP := bat.MaxHP
		if began, _ := castCharm(t, r, enticement(), crabID, 0); !began || bat.Owner == 0 {
			t.Fatalf("%s: not charmed", c.name)
		}
		bat.HP = maxHP + 500 // whatever raised it as a pet
		r.tick(time.Minute - time.Second)
		c.end(r)
		if bat.Owner != 0 || bat.Charm != nil {
			t.Fatalf("%s: still charmed", c.name)
		}
		if bat.Level != 2 || bat.MaxHP != maxHP || bat.HP > bat.MaxHP {
			t.Errorf("%s: freed at level %d, %d/%d HP; want level 2, at most %d", c.name, bat.Level, bat.HP, bat.MaxHP, maxHP)
		}
		if after := npcHitsOn(r.w, bat, r.p); after != before {
			t.Errorf("%s: freed it hits for %d, uncharmed %d", c.name, after, before)
		}
	}
}

// The level gap: a creature green to the player adds its share of the
// archetype's offset (its top's growth to the player's level), so a level 2
// one lands 1-2 on even a naked level 60 of any archetype (the caster's offset made it 33); even, blue and red
// fights keep the offset whole.
func TestALowCreatureHitsAHighPlayerForItsFloor(t *testing.T) {
	for _, class := range []int{classEnchanter, 0 /* warrior */, 4 /* monk */, 9 /* cleric */} {
		// naked: no AC and no AGI to take anything off its top of 1.6
		p := &Player{Class: class, Level: 60}
		if hit := creatureRoll(2).hitOn(p, highDice); hit > 2 {
			t.Errorf("class %d: a level 2 creature's best hit on a naked level 60 is %d, want at most 2", class, hit)
		}
	}
	caster := &Player{Class: classEnchanter, Level: 60}
	for _, lvl := range []int{45, 58, 60, 63} { // light blue, dark blue, even, red
		if s := creatureRoll(lvl).offsetShare(caster.Level); s != 1 {
			t.Errorf("a level %d creature on a level 60 adds %.3f of the offset, want all of it", lvl, s)
		}
	}
	if s := creatureRoll(44).offsetShare(60); s >= 1 || s < 0.5 {
		t.Errorf("a level 44 (green) on a level 60 adds %.3f of the offset", s)
	}
	// a dummy's measured roll keeps it whole
	if s := dummyRolls[TrainingDummy][20].offsetShare(60); s != 1 {
		t.Errorf("a dummy's roll adds %.3f of the offset", s)
	}
}
