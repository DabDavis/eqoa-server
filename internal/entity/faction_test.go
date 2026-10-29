package entity

import (
	"encoding/binary"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// guardHits are a Freeport guard's rows as the town seed writes them.
var guardHits = []database.FactionHitRow{
	{NPCName: "Guard Dahvel", Faction: "Citizens of Freeport", Delta: -25},
	{NPCName: "Guard Dahvel", Faction: "The Freeport Militia", Delta: -50},
}

// lines is every chat line (0x0007) queued for sess so far.
func lines(sess *session.Session) []string {
	var out []string
	for _, m := range sess.Queue().PeekPending() {
		d := m.Data()
		if len(d) < 6 || protocol.GameOpcode(binary.LittleEndian.Uint16(d)) != protocol.OpcodeClientMessage {
			continue
		}
		n := int(binary.LittleEndian.Uint32(d[2:]))
		u := make([]uint16, n)
		for i := range u {
			u[i] = binary.LittleEndian.Uint16(d[6+2*i:])
		}
		out = append(out, string(utf16.Decode(u)))
	}
	return out
}

func factionLines(sess *session.Session) []string {
	var out []string
	for _, l := range lines(sess) {
		if strings.Contains(l, " faction with ") {
			out = append(out, l)
		}
	}
	return out
}

// newGuardFight is newFight's world with a Freeport guard to kill and its
// hits set.
func newGuardFight(t *testing.T) *fight {
	t.Helper()
	f := newFight(t)
	f.npc.Name = "Guard Dahvel"
	f.npc.HP = 1
	if kept, unknown := f.w.SetFactionHits(guardHits); kept != 2 || unknown != 0 {
		t.Fatalf("hits kept %d, unknown %d", kept, unknown)
	}
	return f
}

// A melee kill moves the killer's standings by the NPC's rows, from the
// starting standing (1 for both in the capture), and says so in Sandstorm's
// white lines, the standing after the change and the change; the XP line is
// still there.
func TestAKillChangesTheKillersFactions(t *testing.T) {
	f := newGuardFight(t)
	f.w.mu.Lock()
	killed := f.w.playerHitLocked(f.sess, f.p, f.npc, f.npcID)
	f.w.mu.Unlock()
	if !killed {
		t.Fatal("the guard lived")
	}
	if got := f.p.Factions; got["Citizens of Freeport"] != -24 || got["The Freeport Militia"] != -49 || len(got) != 2 {
		t.Errorf("standings %v, want Citizens of Freeport -24 and The Freeport Militia -49", got)
	}
	want := []string{
		"-24 faction with Citizens of Freeport (-25)",
		"-49 faction with The Freeport Militia (-50)",
	}
	if got := factionLines(f.sess); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("lines %q, want %q", got, want)
	}
	if !strings.Contains(strings.Join(lines(f.sess), "|"), "XP.") {
		t.Error("no XP line")
	}
	// the window lists the new standings in their place
	list := f.w.PlayerFactions(f.sess)
	for i, s := range list {
		if s.Name == "Citizens of Freeport" && s.Standing != -24 {
			t.Errorf("the window says %d", s.Standing)
		}
		if i > 0 && (list[i-1].Standing < s.Standing || list[i-1].Standing == s.Standing && list[i-1].Name > s.Name) {
			t.Fatalf("the window's order breaks at %d: %v then %v", i, list[i-1], s)
		}
	}
	if len(list) != len(database.DefaultFactions) {
		t.Errorf("the window lists %d factions, want %d", len(list), len(database.DefaultFactions))
	}
}

// An NPC with no rows changes nothing and says nothing; a row naming no
// faction is not kept.
func TestAKillWithoutRowsChangesNothing(t *testing.T) {
	f := newFight(t)
	f.npc.HP = 1
	if kept, unknown := f.w.SetFactionHits(append(guardHits, database.FactionHitRow{NPCName: "Crab", Faction: "No Such Faction", Delta: -5})); kept != 2 || unknown != 1 {
		t.Fatalf("kept %d, unknown %d", kept, unknown)
	}
	f.w.mu.Lock()
	f.w.playerHitLocked(f.sess, f.p, f.npc, f.npcID)
	f.w.mu.Unlock()
	if len(f.p.Factions) != 0 || len(factionLines(f.sess)) != 0 {
		t.Errorf("a crab moved %v and said %q", f.p.Factions, factionLines(f.sess))
	}
}

// With Faction Messages off (0x00C9 1) the standings still move, and no line
// is sent.
func TestFactionLinesHonourTheOption(t *testing.T) {
	f := newGuardFight(t)
	f.sess.SetFactionMessagesOff(true)
	f.w.mu.Lock()
	f.w.playerHitLocked(f.sess, f.p, f.npc, f.npcID)
	f.w.mu.Unlock()
	if f.p.Factions["Citizens of Freeport"] != -24 {
		t.Errorf("standing %d, want -24", f.p.Factions["Citizens of Freeport"])
	}
	if got := factionLines(f.sess); len(got) != 0 {
		t.Errorf("lines %q with the option off", got)
	}
}

// A standing stops at -2000: the step that reaches it says -2000 and the
// change asked; the next changes nothing and says the same, as Sandstorm's
// gnoll kills at the floor did (F3 20261024-200725: "-2000 faction with
// Blackburrow Gnolls (-10)" nine times running); and at 2000 the same the
// other way.
func TestStandingsStayInRange(t *testing.T) {
	f := newGuardFight(t)
	f.w.SetFactionHits([]database.FactionHitRow{{NPCName: "Guard Dahvel", Faction: "Citizens of Freeport", Delta: -25}})
	f.p.Factions = map[string]int{"Citizens of Freeport": -1990}
	kill := func() {
		f.npc.Dead, f.npc.HP = false, 1
		f.w.mu.Lock()
		f.w.playerHitLocked(f.sess, f.p, f.npc, f.npcID)
		f.w.mu.Unlock()
	}
	kill()
	kill()
	if got := f.p.Factions["Citizens of Freeport"]; got != FactionMin {
		t.Errorf("standing %d, want %d", got, FactionMin)
	}
	want := []string{
		"-2000 faction with Citizens of Freeport (-25)",
		"-2000 faction with Citizens of Freeport (-25)",
	}
	if got := factionLines(f.sess); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("lines %q, want %q", got, want)
	}
	f.sess.Queue().GetPending()
	f.w.SetFactionHits([]database.FactionHitRow{{NPCName: "Guard Dahvel", Faction: "Citizens of Freeport", Delta: 4000}})
	kill()
	kill()
	if got := f.p.Factions["Citizens of Freeport"]; got != FactionMax {
		t.Errorf("standing %d, want %d", got, FactionMax)
	}
	want = []string{
		"2000 faction with Citizens of Freeport (+4000)",
		"2000 faction with Citizens of Freeport (+4000)",
	}
	if got := factionLines(f.sess); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("lines %q, want %q", got, want)
	}
}

// A spell's kill counts as a swing's does, and so does a GM's #kill (no XP),
// and a GM's #spawn copy counts as its original.
func TestEveryKillChangesFactions(t *testing.T) {
	c := newCaster(t)
	c.npc.Name, c.npc.HP = "Guard Dahvel", 1
	c.w.SetFactionHits(guardHits)
	c.w.mu.Lock()
	c.w.spellHitLocked(c.p, c.sess, c.npc, 5)
	c.w.mu.Unlock()
	if c.p.Factions["Citizens of Freeport"] != -24 {
		t.Errorf("a spell's kill: %v", c.p.Factions)
	}

	f := newGuardFight(t)
	xp := f.p.TotalXP
	if name, ok := f.w.KillNPCByObjectID(f.sess, f.npcID); !ok || name != "Guard Dahvel" {
		t.Fatalf("#kill: %q %v", name, ok)
	}
	if f.p.Factions["The Freeport Militia"] != -49 || f.p.TotalXP != xp {
		t.Errorf("#kill: standings %v, XP %d -> %d", f.p.Factions, xp, f.p.TotalXP)
	}

	g := newGuardFight(t)
	id, _, ok := g.w.SpawnNPCCopy("Guard Dahvel", 103, 10, 100, 0, 0)
	if !ok {
		t.Fatal("no copy")
	}
	cp := g.w.NPCs[id]
	cp.HP = 1
	g.w.mu.Lock()
	g.w.playerHitLocked(g.sess, g.p, cp, g.npcID)
	g.w.mu.Unlock()
	if g.p.Factions["Citizens of Freeport"] != -24 {
		t.Errorf("a copy's kill: %v", g.p.Factions)
	}
}

// A GM's #faction names a faction by its name or by a part only it holds,
// clamps, and refuses one that names several.
func TestSetFactionByName(t *testing.T) {
	f := newFight(t)
	if name, v, err := f.w.SetFaction(f.sess, "citizens of freeport", 5000); err != nil || name != "Citizens of Freeport" || v != FactionMax {
		t.Errorf("exact: %q %d %v", name, v, err)
	}
	if name, v, err := f.w.SetFaction(f.sess, "freeport militia", -3); err != nil || name != "The Freeport Militia" || v != -3 {
		t.Errorf("a part: %q %d %v", name, v, err)
	}
	if _, _, err := f.w.SetFaction(f.sess, "citizens", 1); err == nil || !strings.Contains(err.Error(), "names") {
		t.Errorf("ambiguous: %v", err)
	}
	if _, _, err := f.w.SetFaction(f.sess, "zzz", 1); err == nil {
		t.Error("no faction: no error")
	}
	if got := f.w.FactionsMatching(f.sess, "FREEPORT"); len(got) != 2 || got[0].Name != "Citizens of Freeport" || got[0].Standing != FactionMax {
		t.Errorf("matching: %v", got)
	}
}

// Kept standings come back at the login, and a GM's edit reaches a character
// in the world (or its deletion, the starting standing again).
func TestStandingsKeptAndEditedOnline(t *testing.T) {
	f := newFight(t)
	f.p.CharServerID = 77
	if n := f.w.RestoreFactions(f.sess, map[string]int{"Citizens of Freeport": -300, "Citizens of Qeynos": 9000}); n != 2 {
		t.Fatalf("restored %d", n)
	}
	saved, ok := f.w.SavedFactions(f.p.ServerID)
	if !ok || saved["Citizens of Freeport"] != -300 || saved["Citizens of Qeynos"] != FactionMax {
		t.Errorf("saved %v %v", saved, ok)
	}
	if name, online := f.w.SetOnlineFaction(77, "Citizens of Freeport", 12, false); !online || name != "Tester" || f.p.Factions["Citizens of Freeport"] != 12 {
		t.Errorf("online edit: %q %v %v", name, online, f.p.Factions)
	}
	f.w.SetOnlineFaction(77, "Citizens of Freeport", 0, true)
	if _, ok := f.p.Factions["Citizens of Freeport"]; ok {
		t.Error("a deleted row kept its standing")
	}
	if _, online := f.w.SetOnlineFaction(78, "Citizens of Freeport", 1, false); online {
		t.Error("an offline character was found")
	}
}

// Sandstorm's faction lines, word for word as its client showed them: the
// standing after the change, the faction, the change asked with its sign;
// at the floor the standing held and the change as asked.
func TestFactionLinesAreSandstorms(t *testing.T) {
	for _, c := range []struct {
		faction         string
		delta, standing int
		want            string
	}{
		// questrec16's first Freeport warrior quest (r04-buy-return)
		{"Merchant House Nothard", 10, 11, "11 faction with Merchant House Nothard (+10)"},
		{"Citizens of Freeport", 5, 6, "6 faction with Citizens of Freeport (+5)"},
		// a kill's, in a report's chat log (F3 20261029-130643)
		{"Centaurs of the Plains", -10, -690, "-690 faction with Centaurs of the Plains (-10)"},
		{"Citizens of Caer Sloth", 1, -561, "-561 faction with Citizens of Caer Sloth (+1)"},
		// at the floor (F3 20261024-200725, nine kills running)
		{"Blackburrow Gnolls", -10, FactionMin, "-2000 faction with Blackburrow Gnolls (-10)"},
		// a name as a table may keep it, and a standing of 0
		{" Citizens of Qeynos ", -1, 0, "0 faction with Citizens of Qeynos (-1)"},
	} {
		if got := factionLine(c.faction, c.delta, c.standing); got != c.want {
			t.Errorf("factionLine(%q, %d, %d) = %q, want %q", c.faction, c.delta, c.standing, got, c.want)
		}
	}
}

// A quest's faction rewards (World.ApplyFactionHits) say Sandstorm's lines
// too, as white chat (0x0A7A, as Sandstorm sent its quest's), one a faction in
// the step's order; with Faction Messages off they still move and say nothing.
func TestQuestFactionRewardsSaySandstormsLines(t *testing.T) {
	f := newFight(t)
	hits := []FactionHit{{"Citizens of Freeport", 10}, {"The Freeport Militia", 5}, {"Citizens of Freeport", 0}}
	f.w.ApplyFactionHits(f.sess, f.p, hits)
	want := []string{"11 faction with Citizens of Freeport (+10)", "6 faction with The Freeport Militia (+5)"}
	if got := factionLines(f.sess); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("lines %q, want %q", got, want)
	}
	for _, m := range f.sess.Queue().PeekPending() {
		d := m.Data()
		if op := protocol.GameOpcode(binary.LittleEndian.Uint16(d)); op != protocol.OpcodeClientMessage {
			t.Errorf("a %#04x among the lines, want only 0x0A7A", uint16(op))
		}
	}
	g := newFight(t)
	g.sess.SetFactionMessagesOff(true)
	g.w.ApplyFactionHits(g.sess, g.p, hits)
	if got := factionLines(g.sess); len(got) != 0 || g.p.Factions["Citizens of Freeport"] != 11 {
		t.Errorf("Faction Messages off: lines %q, standing %d", got, g.p.Factions["Citizens of Freeport"])
	}
}
