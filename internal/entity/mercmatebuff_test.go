package entity

import (
	"testing"
	"time"
)

// A shaman merc buffs its owner's other mercs too (Heirloom petition 2904,
// 2026-11-24: "my shaman merc seems to be buffing me and its self, doesnt buff
// other mercs in group"): its target's buff on each of them after its owner
// and itself, and its group's buff again when one of them lacks it while it
// and its owner have it.
func TestMercBuffsOwnersOtherMercs(t *testing.T) {
	r := newMercRig(t)
	brawn := bookSpell(700901, "Tribal Brawn", 1, 3*time.Second, 0, 20, "Target, Increase STR by 20, 30min")
	brawn.Level = mercOwnerLevel
	breeze := bookSpell(700902, "Breeze", 2, 3*time.Second, 0, 20, "Group, Increase AGI by 10, Radius 30, 30min")
	breeze.Level = mercOwnerLevel
	hire := func(slot int, k MercKit) {
		t.Helper()
		if _, why, _ := r.w.HireMercAt(r.sess, slot, k, MercAssist, r.at(0)); why != "" {
			t.Fatalf("slot %d: %s", slot, why)
		}
		r.w.mu.Lock()
		_, up := r.w.invites[r.p.ServerID][r.p.ServerID]
		r.w.mu.Unlock()
		if up {
			r.w.GroupAccept(r.sess, r.sess.PlayerObjectID())
		}
	}
	shaman := r.kit(8, 3, 1, brawn, breeze)
	shaman.Name = "Aliela"
	hire(1, shaman)
	for slot, name := range map[int]string{2: "Garoris", 3: "Joranusis"} {
		k := r.kit(0, 3, 0)
		k.Name = name
		hire(slot, k)
	}
	r.w.mu.Lock()
	mercs := r.w.mercsOfLocked(r.p)
	r.w.mu.Unlock()
	if len(mercs) != 3 {
		t.Fatalf("%d mercs out", len(mercs))
	}
	has := func(effects []*ActiveEffect, id int) bool {
		for _, e := range effects {
			if e.SpellID == id {
				return true
			}
		}
		return false
	}
	d := time.Duration(0)
	run := func(until time.Duration) {
		for ; d <= until; d += 100 * time.Millisecond {
			r.tick(d)
		}
	}
	run(60 * time.Second)
	r.w.mu.Lock()
	if !has(r.p.Effects, brawn.SpellID) || !has(r.p.Effects, breeze.SpellID) {
		t.Errorf("the owner lacks a buff: %v", mateEffectNames(r.p.Effects))
	}
	for _, ms := range mercs {
		if !has(ms.npc.Effects, brawn.SpellID) || !has(ms.npc.Effects, breeze.SpellID) {
			t.Errorf("merc %s lacks a buff: %v", ms.npc.Name, mateEffectNames(ms.npc.Effects))
		}
	}
	// one of the others loses the group's buff: the shaman puts it up again
	var other *NPC
	for _, ms := range mercs {
		if ms.npc.Name == "Garoris" {
			other = ms.npc
		}
	}
	kept := other.Effects[:0]
	for _, e := range other.Effects {
		if e.SpellID != breeze.SpellID {
			kept = append(kept, e)
		}
	}
	other.Effects = kept
	r.w.mu.Unlock()
	run(120 * time.Second)
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	if !has(other.Effects, breeze.SpellID) {
		t.Errorf("Garoris lacks the group's buff again: %v", mateEffectNames(other.Effects))
	}
}

func mateEffectNames(effects []*ActiveEffect) []string {
	var out []string
	for _, e := range effects {
		out = append(out, e.Name)
	}
	return out
}

// Lycanthropy's line as petfix_lycan_20261124 words it (Heirloom petition
// 2903: "isnt adding any hp from the buff"): its "Increase Hitpoints by 5000"
// alone is a heal as it lands (spelltext.go), so its maximum is raised too,
// as every other max-HP buff words it.
func TestLycanthropyLineRaisesMaxHP(t *testing.T) {
	old := ParseSpellNamed("Lycanthropy", "Self, Increase STR/DEX by 300, Increase Hitpoints by 5000, 15min", "")
	if old.RaisesMaxHP() {
		t.Fatalf("the old line raises the maximum: %+v", old.Effects)
	}
	d := ParseSpellNamed("Lycanthropy", "Self, Increase STR/DEX by 300, Increase Maximum HP by 5000, Increase Hitpoints by 5000, 15min", "")
	var maxHP, heal, str, dex int
	for _, e := range d.Effects {
		switch {
		case e.Kind == EffectStat && e.Stat == StatMaxHP:
			maxHP = e.Amount
		case e.Kind == EffectHP:
			heal = e.Amount
		case e.Kind == EffectStat && e.Stat == StatStr:
			str = e.Amount
		case e.Kind == EffectStat && e.Stat == StatDex:
			dex = e.Amount
		}
	}
	if maxHP != 5000 || heal != 5000 || str != 300 || dex != 300 || d.Target != TargetSelf || d.Duration != 15*time.Minute {
		t.Errorf("max HP %d, heal %d, STR %d, DEX %d, target %v, %v: %+v", maxHP, heal, str, dex, d.Target, d.Duration, d.Effects)
	}
}
