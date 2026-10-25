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
		if strings.HasPrefix(l, "Your faction standing") {
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
// starting standing (1 for both in the capture), and says so in EverQuest's
// words; the XP line is still there.
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
		"Your faction standing with Citizens of Freeport got worse.",
		"Your faction standing with The Freeport Militia got worse.",
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

// A standing stops at -2000: the step that reaches it got worse, the next
// could not possibly get any worse and changes nothing; and at 2000 the same
// the other way.
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
		"Your faction standing with Citizens of Freeport got worse.",
		"Your faction standing with Citizens of Freeport could not possibly get any worse.",
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
		"Your faction standing with Citizens of Freeport got better.",
		"Your faction standing with Citizens of Freeport could not possibly get any better.",
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
