package handlers

import (
	"slices"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A talk to one of a people listed in factionrefusetalk (petition 3111) by a
// player at or below -350 with them is one refusal box and none of its
// business: here a greeter's words (world.npc_text) are not said. The line is
// the faction's row's, else the default; the controls: -349, a GM at -2000,
// and the faction no longer listed all hear its words.
func TestTalkRefusedByAListedFaction(t *testing.T) {
	tw := newTalkWorld(t, 100, 100)
	tw.h.isGM = func(*session.Session) bool { return false }
	was := NPCText()
	SetNPCText(map[string]string{"Graumord citizen": "Welcome to Graumord."})
	t.Cleanup(func() { SetNPCText(was) })
	id := tw.add(1, &entity.NPC{ServerID: 5001, Name: "Graumord citizen", NPCType: 0x80, SpawnX: 101, SpawnY: 54, SpawnZ: 100})
	tw.w.SetNPCFactions([]database.NPCFactionRow{{Name: "graumord citizen", World: -1, Faction: "Citizens of Graumord"}})
	tw.w.SetFactionTalkRefusals([]database.FactionTalkRefusalRow{{Faction: "Citizens of Graumord"}})
	words, refused := "box:Welcome to Graumord.", "box:Graumord citizen regards you coldly and will not speak with you."
	say := func() []string { tw.s.Queue().GetPending(); tw.talk(id); return archivistMsgs(tw.s) }

	tw.p.Factions = map[string]int{"Citizens of Graumord": -350}
	if got := say(); !slices.Equal(got, []string{refused}) {
		t.Fatalf("at -350: sent %q, want only %q", got, refused)
	}
	tw.w.SetFactionTalkRefusals([]database.FactionTalkRefusalRow{{Faction: "Citizens of Graumord", Line: "%n turns away from you, %p."}})
	if got := say(); !slices.Equal(got, []string{"box:Graumord citizen turns away from you, Talker."}) {
		t.Fatalf("the row's own line: sent %q", got)
	}
	tw.p.Factions["Citizens of Graumord"] = -349
	if got := say(); !slices.Contains(got, words) {
		t.Fatalf("at -349: sent %q, want its words", got)
	}
	tw.p.Factions["Citizens of Graumord"] = -2000
	tw.h.isGM = func(*session.Session) bool { return true }
	if got := say(); !slices.Contains(got, words) {
		t.Fatalf("a GM at -2000: sent %q, want its words", got)
	}
	tw.h.isGM = func(*session.Session) bool { return false }
	tw.w.SetFactionTalkRefusals(nil)
	if got := say(); !slices.Contains(got, words) {
		t.Fatalf("the faction unlisted: sent %q, want its words", got)
	}
}

// A banker of a listed people opens no bank for the red bands: the refusal
// box and 0x00D7, no 0x124D; at -349 the bank opens.
func TestBankRefusedByAListedFaction(t *testing.T) {
	f := newArchivistFixture(t, nil)
	f.windows.isGM = func(*session.Session) bool { return false }
	f.stand("Graumord banker", 0, 0x82, 200003)
	f.w.SetNPCFactions([]database.NPCFactionRow{{Name: "graumord banker", World: -1, Faction: "Citizens of Graumord"}})
	f.w.SetFactionTalkRefusals([]database.FactionTalkRefusalRow{{Faction: "Citizens of Graumord"}})
	open := func() []string {
		f.sess.Queue().GetPending()
		if err := f.windows.HandleBankOpen(f.sess, incoming(protocol.OpcodeBankUI, u32b(200003))); err != nil {
			t.Fatal(err)
		}
		return archivistMsgs(f.sess)
	}
	f.p.Factions = map[string]int{"Citizens of Graumord": -1000}
	f.wantMsgs("at -1000", open(), "box:Graumord banker regards you coldly and will not speak with you.", "op:00D7")
	f.p.Factions["Citizens of Graumord"] = -349
	f.wantMsgs("at -349", open(), "op:124D")
}
