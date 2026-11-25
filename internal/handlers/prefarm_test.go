package handlers

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// Petition 2975 and the owner, 2026-11-25: "/prefarm off option for players that removes
// the quest loot drops from mobs" (prefarm.go). Quest 70's belts (prefarmQuest) drop ahead
// on a kill of an orc pawn by a character on step 1; with /prefarm off they do not, and
// the tracker no longer says the pawn "may drop" one; on again, they drop as before. On the
// step itself the character's own belt drops either way.
func TestPrefarmOffKeepsAheadDropsOffItsKills(t *testing.T) {
	f := newQuestFixture(t)
	belt := f.pattern("slashed belt", 20)
	drops := prefarmQuest(f, belt, 0, 0, 0, 0)
	if _, err := f.qe.SetCharacterQuest(f.sid, 70, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	if err := f.db.SetPrefarmOff(f.sid, true); err != nil {
		t.Fatal(err)
	}
	f.msgs()
	f.killFor("Orc Pawn", 901) // ahead, off: none
	if len(*drops) != 0 {
		t.Fatalf("/prefarm off: drops %v, want none", *drops)
	}
	for _, m := range f.msgs() {
		if strings.Contains(m, "may drop") {
			t.Errorf("/prefarm off, still told: %q", m)
		}
	}
	f.killFor("Orc Pawn", 902) // not on the quest's step 3 either, the same
	if len(*drops) != 0 {
		t.Fatalf("/prefarm off: drops %v, want none", *drops)
	}
	if err := f.db.SetPrefarmOff(f.sid, false); err != nil {
		t.Fatal(err)
	}
	f.killFor("Orc Pawn", 903) // on: a belt ahead
	if len(*drops) != 1 || (*drops)[0] != (lootDrop{903, true, belt}) {
		t.Fatalf("/prefarm on: drops %v, want a belt ahead on 903", *drops)
	}
}

// On the step, /prefarm off keeps the player's own drop: only items for no one of the kill
// on their step are kept off.
func TestPrefarmOffKeepsTheStepsOwnDrop(t *testing.T) {
	f := newQuestFixture(t)
	belt := f.pattern("slashed belt", 20)
	drops := lootQuest(f, belt, 0.2, 0.2)
	if err := f.db.SetPrefarmOff(f.sid, true); err != nil {
		t.Fatal(err)
	}
	f.killFor("Orc Pawn", 911) // not on quest 60: off, none
	if len(*drops) != 0 {
		t.Fatalf("off the quest, /prefarm off: drops %v", *drops)
	}
	if _, err := f.qe.SetCharacterQuest(f.sid, 60, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.killFor("Orc Pawn", 912) // on its step: its own belt
	if len(*drops) != 1 || (*drops)[0].npc != 912 || (*drops)[0].ahead {
		t.Fatalf("on the step, /prefarm off: drops %v, want its own belt on 912", *drops)
	}
}

// A group's kill follows its leader's /prefarm (KillCredit.Leader), whoever of it killed: the
// leader off keeps the ahead belt off a member's kill; a member off with the leader on does
// not; a group whose leader is not known drops as ever.
func TestPrefarmIsTheGroupLeadersForItsKills(t *testing.T) {
	f := newQuestFixture(t)
	belt := f.pattern("slashed belt", 20)
	drops := prefarmQuest(f, belt, 0, 0, 0, 0, 0)
	rec := &database.CharacterRecord{CharName: "Leader", AccountID: 2}
	if err := database.NewSQLiteCharacterStore(f.db).Create(rec); err != nil {
		t.Fatal(err)
	}
	s2 := f.tw.mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 11), Port: 7011}, 11, 2, 3, 4, true)
	s2.SetInventoryList(newInvList(nil))
	kill := func(npc entity.EntityID, leader uint32) {
		f.qe.killed(entity.KillCredit{NPCName: "Orc Pawn", NPCSpawn: npc, NPCID: npc, Died: time.Now(), Group: 7, Leader: leader,
			Players: []entity.KillPlayer{{Session: f.tw.s, ServerID: f.sid, Name: "Talker"}, {Session: s2, ServerID: rec.ServerID, Name: "Leader"}}})
	}
	if err := f.db.SetPrefarmOff(rec.ServerID, true); err != nil {
		t.Fatal(err)
	}
	kill(921, rec.ServerID) // the member killed; the leader off: none
	if len(*drops) != 0 {
		t.Fatalf("the leader off: drops %v, want none", *drops)
	}
	if err := f.db.SetPrefarmOff(rec.ServerID, false); err != nil {
		t.Fatal(err)
	}
	if err := f.db.SetPrefarmOff(f.sid, true); err != nil {
		t.Fatal(err)
	}
	kill(922, rec.ServerID) // the member (the killer) off, the leader on: a belt
	if len(*drops) != 1 || (*drops)[0] != (lootDrop{922, true, belt}) {
		t.Fatalf("a member off, the leader on: drops %v, want a belt on 922", *drops)
	}
	kill(923, 0) // no leader known: as ever
	if len(*drops) != 2 {
		t.Fatalf("no leader: drops %v, want a belt on 923", *drops)
	}
	// alone, the killer's own: off
	f.killFor("Orc Pawn", 924)
	if len(*drops) != 2 {
		t.Fatalf("alone and off: drops %v, want none more", *drops)
	}
}

// The choice is kept per character in charprefarm: on with no row, off and on again kept.
func TestPrefarmIsKept(t *testing.T) {
	db, _ := newSafeDB(t)
	if off, err := db.PrefarmOff(42); err != nil || off {
		t.Fatalf("no row: off %v (%v), want on", off, err)
	}
	for _, off := range []bool{true, false, true} {
		if err := db.SetPrefarmOff(42, off); err != nil {
			t.Fatal(err)
		}
		if got, err := db.PrefarmOff(42); err != nil || got != off {
			t.Fatalf("kept %v (%v), want %v", got, err, off)
		}
	}
	if off, _ := db.PrefarmOff(43); off {
		t.Fatal("another character's choice changed")
	}
}

// Each spelling ("/prefarm off", "#prefarm on", a PS2's "/t prefarm off", any case) sets
// it and says so; "/prefarm" alone says it; "/t prefarm hello" is a tell; "/prefarm maybe"
// is answered with the usage and changes nothing.
func TestPrefarmSpellings(t *testing.T) {
	db, _ := newSafeDB(t)
	c := newChatWorld(t)
	c.h.SetPrefarmStore(db)
	s, p := c.join(5, "Plain", 100, 100, 0)
	for _, tc := range []struct {
		line string
		off  bool
	}{
		{"/prefarm off", true}, {"/prefarm on", false},
		{"#prefarm OFF", true}, {"#Prefarm on", false},
		{"/t prefarm off", true}, {"/tell prefarm on", false},
		{"/PREFARM Off", true}, {"/prefarm", true},
	} {
		c.say(s, tc.line)
		want := prefarmOnLine
		if tc.off {
			want = prefarmOffLine
		}
		if got := texts(heard(t, s)); len(got) != 2 || got[0] != want || got[1] != prefarmSoloLine {
			t.Fatalf("%s: told %q, want %q and the solo line", tc.line, got, want)
		}
		if kept, err := db.PrefarmOff(p.CharServerID); err != nil || kept != tc.off {
			t.Fatalf("%s: kept %v (%v), want %v", tc.line, kept, err, tc.off)
		}
		s.Queue().GetPending()
	}
	c.say(s, "/t prefarm hello there")
	if got := texts(heard(t, s)); len(got) != 1 || got[0] != "prefarm is not connected." {
		t.Fatalf("a tell read as /prefarm: %q", got)
	}
	s.Queue().GetPending()
	c.say(s, "/prefarm maybe")
	if got := texts(heard(t, s)); len(got) != 1 || got[0] != prefarmUsage {
		t.Fatalf("/prefarm maybe: %q", got)
	}
	if kept, _ := db.PrefarmOff(p.CharServerID); !kept {
		t.Fatal("/prefarm maybe changed it")
	}
}

// In a group, /prefarm says whose holds: the leader is told its own holds for the group; a
// member is told the leader's setting by name.
func TestPrefarmSaysWhoseHoldsInAGroup(t *testing.T) {
	db, _ := newSafeDB(t)
	c := newChatWorld(t)
	c.h.SetPrefarmStore(db)
	sa, pa := c.join(5, "Ann", 100, 100, 0)
	sb, pb := c.join(6, "Bob", 101, 100, 0)
	c.group(t, sa, pa, sb, pb)
	sa.Queue().GetPending()
	sb.Queue().GetPending()
	if err := db.SetPrefarmOff(pa.CharServerID, true); err != nil {
		t.Fatal(err)
	}
	c.say(sa, "/prefarm")
	if got := texts(heard(t, sa)); len(got) != 2 || got[0] != prefarmOffLine || got[1] != prefarmLeaderLine {
		t.Fatalf("the leader told %q", got)
	}
	c.say(sb, "/prefarm")
	if got := texts(heard(t, sb)); len(got) != 2 || got[0] != prefarmOnLine || got[1] != fmt.Sprintf(prefarmMemberLine, "Ann", "off") {
		t.Fatalf("the member told %q", got)
	}
}

// /help lists /prefarm.
func TestPrefarmIsInHelp(t *testing.T) {
	if !strings.Contains(strings.Join(playerHelpLines, "\n"), "/prefarm") {
		t.Error("/help does not list /prefarm")
	}
}
