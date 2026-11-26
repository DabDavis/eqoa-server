package entity

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// RefusesTalk (petition 3111): an NPC of a faction listed in
// factionrefusetalk will not speak to a player at or below -350 with it,
// attackable or with the NPC bit; -349 is spoken to, as is everyone by an NPC
// of an unlisted faction (merchants and coachmen are Refuses' own), a dead
// NPC, and everyone with world.faction_aggro off. A row naming no faction is
// counted and dropped.
func TestRefusesTalkOnlyListedFactions(t *testing.T) {
	f := newAggroFix(t, 20)
	inn := f.npc(2001, "Graumord citizen", 53, humanBody, 1100, 1210)
	inn.NPCType = 0x80
	street := f.npc(2002, "Graumord citizen", 53, humanBody, 1100, 1220)
	baga := f.npc(2003, "baga resident", 30, humanBody, 1100, 1230)
	f.w.SetNPCFactions([]database.NPCFactionRow{
		{Name: "graumord citizen", World: -1, Faction: "Citizens of Graumord"},
		{Name: "baga resident", World: -1, Faction: "Citizens of Baga"},
	})
	f.p.Factions = map[string]int{"Citizens of Graumord": -2000, "Citizens of Baga": -2000}
	if _, _, ok := f.w.RefusesTalk(f.sess, inn); ok {
		t.Fatal("refused with factionrefusetalk empty")
	}
	kept, unknown := f.w.SetFactionTalkRefusals([]database.FactionTalkRefusalRow{
		{Faction: "Citizens of Graumord"}, {Faction: "No Such Faction", Line: "x"}})
	if kept != 1 || unknown != 1 {
		t.Fatalf("kept %d unknown %d, want 1 1", kept, unknown)
	}
	for _, c := range []struct {
		standing int
		refuses  bool
	}{{-2000, true}, {-350, true}, {-349, false}, {0, false}} {
		f.p.Factions["Citizens of Graumord"] = c.standing
		for _, n := range []*NPC{inn, street} {
			fac, line, ok := f.w.RefusesTalk(f.sess, n)
			if ok != c.refuses || ok && (fac != "Citizens of Graumord" || line != "") {
				t.Errorf("standing %d, type %#x: %q %q %v, want refuses %v", c.standing, n.NPCType, fac, line, ok, c.refuses)
			}
		}
	}
	f.p.Factions["Citizens of Graumord"] = -2000
	if _, _, ok := f.w.RefusesTalk(f.sess, baga); ok {
		t.Error("an unlisted faction's NPC refused to talk")
	}
	street.Dead = true
	if _, _, ok := f.w.RefusesTalk(f.sess, street); ok {
		t.Error("a corpse refused (its loot is the talk)")
	}
	f.w.SetFactionAggro(false)
	if _, _, ok := f.w.RefusesTalk(f.sess, inn); ok {
		t.Error("refused with world.faction_aggro off")
	}
}
