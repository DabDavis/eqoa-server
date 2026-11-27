package entity

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// Live petitions 5678/5679 (2026-11-27): Neriak's four level 25 guild zombies are Citizens of Neriak by a place row
// (npcfaction 482, within 14.6 of where they spawn), and a GM's name-wide aggressive npcaggro row for "decaying
// zombie" (meant for the wilds' undead) took them out of it: they set on dark elf newbies at 151. The place row's
// faction decides for them again; the controls: a citizen gone KOS is still attacked by them, a decaying zombie away
// from the guild (no place row) still attacks by the GM's row, and a peaceful GM row still spares a KOS player
// inside its place row (large rats in Moradhim).
func TestAPlaceFactionHoldsUnderAGMsAggressiveNameRule(t *testing.T) {
	const neriak = "Citizens of Neriak"
	f := newAggroFix(t, 5)
	f.w.SetFactionDefaults([]database.FactionDefault{{Name: neriak, Standing: 0}, {Name: "Citizens of Moradhim", Standing: 0}})
	f.w.SetFactionStarts([]database.FactionStartRow{{Race: 2, Class: -1, HumType: -1, Faction: neriak, Standing: 151}})
	f.p.Race, f.p.Class = 2, 3 // a dark elf shadow knight, level 5, no rows of his own: 151
	kos := &Player{ServerID: 1002, Name: "Outcast", Level: 5, Race: 2, Class: 3, X: 1000, Y: 10, Z: 1000,
		Factions: map[string]int{neriak: -900}}

	guild := f.npc(3101, "decaying zombie", 25, bearBody, 1003, 1002)
	wilds := f.npc(3102, "decaying zombie", 20, bearBody, 3000, 3000)
	rat := f.npc(3103, "large rat", 10, bearBody, 1001, 1001)
	f.w.SetNPCFactions([]database.NPCFactionRow{
		{Name: "decaying zombie", World: int(guild.World), X: 1000, Z: 1000, Radius: 14.6, Faction: neriak},
		{Name: "large rat", World: int(rat.World), X: 1000, Z: 1000, Radius: 45, Faction: "Citizens of Moradhim"},
	})
	kos.Factions["Citizens of Moradhim"] = -900

	check := func(when string, rules map[string]AggroRule) {
		t.Helper()
		f.w.SetAggroRules(rules)
		f.w.mu.Lock()
		defer f.w.mu.Unlock()
		if got := f.w.standingLocked(f.p, neriak); got != 151 {
			t.Fatalf("%s: the dark elf's standing with %s is %d, want 151", when, neriak, got)
		}
		if f.w.threatensLocked(guild, f.p) {
			t.Errorf("%s: the guild zombie attacks a citizen at 151", when)
		}
		if !f.w.threatensLocked(guild, kos) {
			t.Errorf("%s: the guild zombie spares a KOS dark elf", when)
		}
		if fa, ok := f.w.factionAggroLocked(guild); !ok || fa != neriak {
			t.Errorf("%s: the guild zombie attacks for %q (%v), want %s", when, fa, ok, neriak)
		}
		if !f.w.threatensLocked(wilds, f.p) {
			t.Errorf("%s: the wilds' decaying zombie (no place row) spares a level 5", when)
		}
		if f.w.threatensLocked(rat, kos) {
			t.Errorf("%s: a large rat under a peaceful GM row attacks a KOS player", when)
		}
	}
	check("no GM row for the name", map[string]AggroRule{"large rat": {Aggressive: false}})
	check("the GM's aggressive row for the name",
		map[string]AggroRule{"large rat": {Aggressive: false}, "decaying zombie": {Aggressive: true}})
}
