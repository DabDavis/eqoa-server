package handlers

import (
	"slices"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// hostileRow is an npcdialog row of an NPC's own name in mood hostile, as
// tools/jib742_20261121.py writes them.
func hostileRow(id int, name, line string) database.NPCDialogLine {
	return database.NPCDialogLine{ID: id, Name: name, Mood: entity.MoodHostile, Line: line, Weight: 1,
		Race: -1, SpeakerRace: -1, Class: -1, NotRace: -1}
}

func hasPrefix(msgs []string, prefix string) bool {
	return slices.ContainsFunc(msgs, func(m string) bool { return strings.HasPrefix(m, prefix) })
}

// A coachman of a faction that holds the player at or below -350 (board #742)
// signs no ledger and opens no menu: he says his own hostile line, else "I
// will not help you, <name>.", his name before it. The controls: at -349 he
// signs and offers his menu; a GM at -2000 is served; with his faction row
// gone nothing is refused.
func TestACoachmanRefusesTheRedBands(t *testing.T) {
	tw, ronks, book := coachWorld(t)
	book[44] = map[string]bool{"Highpass Hold": true}
	tw.w.SetNPCFactions([]database.NPCFactionRow{{Name: "coachman ronks", World: -1, Faction: "Citizens of Freeport"}})
	refused := "box:Coachman Ronks: I will not help you, Talker."

	tw.p.Factions = map[string]int{"Citizens of Freeport": -350}
	tw.s.Queue().GetPending()
	tw.talk(ronks)
	got := archivistMsgs(tw.s)
	if !slices.Contains(got, refused) || hasPrefix(got, "ask#") || book[44]["Freeport"] {
		t.Fatalf("at -350: sent %q, Freeport signed %v; want his refusal, no menu, no ledger signed", got, book[44]["Freeport"])
	}

	// his own hostile line, the player's name in it
	tw.h.SetDialogLines(func() ([]database.NPCDialogLine, error) {
		return []database.NPCDialogLine{hostileRow(1, "Coachman Ronks", "Gah! Go way, %p!")}, nil
	})
	tw.talk(ronks)
	if got := archivistMsgs(tw.s); !slices.Contains(got, "box:Coachman Ronks: Gah! Go way, Talker!") || hasPrefix(got, "ask#") {
		t.Fatalf("with his own hostile line: sent %q", got)
	}

	// the control: -349 is served, signed and offered his menu
	tw.p.Factions["Citizens of Freeport"] = -349
	tw.talk(ronks)
	got = archivistMsgs(tw.s)
	if hasPrefix(got, "box:Coachman Ronks: Gah") || !hasPrefix(got, "ask#") || !book[44]["Freeport"] {
		t.Fatalf("at -349: sent %q, Freeport signed %v; want his menu and his ledger signed", got, book[44]["Freeport"])
	}

	// a GM is always served
	tw.p.Factions["Citizens of Freeport"] = -2000
	tw.h.isGM = func(*session.Session) bool { return true }
	tw.talk(ronks)
	if got := archivistMsgs(tw.s); hasPrefix(got, "box:Coachman Ronks: Gah") || !hasPrefix(got, "ask#") {
		t.Fatalf("a GM at -2000: sent %q, want his menu", got)
	}
	tw.h.isGM = func(*session.Session) bool { return false }

	// no npcfaction row: nothing refused, whatever the standing
	tw.w.SetNPCFactions(nil)
	tw.talk(ronks)
	if got := archivistMsgs(tw.s); hasPrefix(got, "box:Coachman Ronks: Gah") || !hasPrefix(got, "ask#") {
		t.Fatalf("no faction row: sent %q, want his menu", got)
	}
}

// A merchant of a faction that holds the player at or below -350 opens no
// window (no 0x00B7, 0x00D7 to clear the request) and prints the filmed chat
// line in red; one with a hostile line of its own says that in a box; a buy
// with the window open is refused the same way. The controls: -349 and an NPC
// of no faction open the shop, and a buy at -349 is made.
func TestAMerchantRefusesTheRedBands(t *testing.T) {
	f := newArchivistFixture(t, nil)
	sword := f.item("Rusty Sword", false, 9, 32767, 1023, 1)
	f.shop("baga shopkeeper", 0x81, 777, sword)
	f.stand("baga shopkeeper", 777, 0x81, 200001)
	f.shop("Jextalan shopkeeper", 0x81, 778, sword)
	f.stand("Jextalan shopkeeper", 778, 0x81, 200002)
	f.w.SetNPCFactions([]database.NPCFactionRow{
		{Name: "baga shopkeeper", World: -1, Faction: "Citizens of Baga"},
		{Name: "jextalan shopkeeper", World: -1, Faction: "People of Jextalan"},
	})
	f.windows.SetDialogLines(func() ([]database.NPCDialogLine, error) {
		return []database.NPCDialogLine{hostileRow(1, "Jextalan shopkeeper", "You have nothing I want and I have nothing you need. Good day.")}, nil
	})
	f.p.Factions = map[string]int{"Citizens of Baga": -350, "People of Jextalan": -450}

	f.wantMsgs("baga at -350", f.open(200001), "red:"+merchantRefusal, "op:00D7")
	f.wantMsgs("Jextalan at -450", f.open(200002),
		"box:Jextalan shopkeeper: You have nothing I want and I have nothing you need. Good day.", "op:00D7")

	// a GM is served at -2000 (the owner, 2026-11-21: "exempt GMs at merchants too")
	f.windows.isGM = func(*session.Session) bool { return true }
	f.p.Factions["Citizens of Baga"] = -2000
	f.wantMsgs("a GM at -2000", f.open(200001), "shop:200001 1")
	f.windows.isGM = func(*session.Session) bool { return false }
	f.p.Factions["Citizens of Baga"] = -350

	// a buy refused: the chat line, nothing given, the request cleared
	f.sess.Queue().GetPending()
	if err := f.windows.HandleMerchantBuy(f.sess, incoming(protocol.OpcodeMerchantBuy, append([]byte{0, 2}, u32b(200001)...))); err != nil {
		t.Fatal(err)
	}
	f.wantMsgs("a buy at -350", archivistMsgs(f.sess), "red:"+merchantRefusal, "op:00D7")

	// the controls
	f.p.Factions["Citizens of Baga"] = -349
	f.wantMsgs("baga at -349", f.open(200001), "shop:200001 1")
	f.sess.Queue().GetPending()
	if err := f.windows.HandleMerchantBuy(f.sess, incoming(protocol.OpcodeMerchantBuy, append([]byte{0, 2}, u32b(200001)...))); err != nil {
		t.Fatal(err)
	}
	if got := archivistMsgs(f.sess); !slices.Contains(got, "item") || hasPrefix(got, "red:") {
		t.Errorf("a buy at -349: sent %q, want the item", got)
	}
	f.w.SetNPCFactions(nil)
	f.wantMsgs("Jextalan with no faction row", f.open(200002), "shop:200002 1")
}
