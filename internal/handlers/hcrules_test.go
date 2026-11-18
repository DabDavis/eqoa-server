package handlers

import (
	"bytes"
	"testing"
	"time"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// The Hardcore isolation rules on the handlers' side (hcrules.go,
// lootrights.go): a Hardcore character loots only what it earned, trades with
// no one, uses no auction house, is carried by none of the server's own
// conveniences and is handed nothing by Dabrish or a merc liaison.

func hcOn(t *testing.T) {
	t.Helper()
	entity.SetHardcoreOn(true)
	t.Cleanup(func() { entity.SetHardcoreOn(false) })
}

// hcSaid reports a message sent s holding phrase (as UTF-8 or UTF-16LE).
func hcSaid(s *session.Session, phrase string) bool {
	wide := make([]byte, 0, 2*len(phrase))
	for i := 0; i < len(phrase); i++ {
		wide = append(wide, phrase[i], 0)
	}
	for _, m := range s.Queue().PeekPending() {
		if d := m.Data(); bytes.Contains(d, []byte(phrase)) || bytes.Contains(d, wide) {
			return true
		}
	}
	return false
}

// Rule 2: a corpse opens for a Hardcore character only when its kill was its
// own (or its Hardcore group's) alone: never once it is anyone's, never one no
// one was credited with, never a helped kill. The controls: the same corpses
// for a player not Hardcore.
func TestHardcoreLootsOnlyWhatItEarned(t *testing.T) {
	hcOn(t)
	died := time.Unix(1_800_000_000, 0)
	now := died.Add(time.Minute)
	h := &NPCWindowHandler{}
	groups := map[*entity.Player]uint32{}
	h.groupIDOf = func(p *entity.Player) uint32 { return groups[p] }
	h.lootNow = func() time.Time { return now }
	bran := &entity.Player{CharServerID: 72, Name: "Bran", Hardcore: true}
	mate := &entity.Player{CharServerID: 74, Name: "Mate", Hardcore: true}
	other := &entity.Player{CharServerID: 75, Name: "Other", Hardcore: true}
	liss := &entity.Player{CharServerID: 73, Name: "Liss"}
	groups[bran], groups[mate] = 5, 5

	// an earned kill: Bran's, Mate (its group) farther off too; Other never
	h.heardKillRight(entity.KillCredit{NPCID: 1, Died: died, Group: 5, OpenAt: died.Add(6 * time.Minute), Hardcore: true, HCEarned: true,
		Players: []entity.KillPlayer{{ServerID: 72, Name: "Bran"}}})
	if !h.mayLoot(bran, 1, died) || !h.mayLoot(mate, 1, died) || h.mayLoot(other, 1, died) {
		t.Error("the earned kill's rights")
	}
	now = died.Add(10 * time.Minute) // anyone's: not a Hardcore character's
	if h.mayLoot(other, 1, died) || !h.mayLoot(liss, 1, died) {
		t.Error("an unlocked corpse: Other (Hardcore) may loot it, or Liss may not")
	}
	// a helped kill: no one of the Hardcore group
	now = died.Add(time.Minute)
	h.heardKillRight(entity.KillCredit{NPCID: 2, Died: died, Group: 5, OpenAt: died.Add(6 * time.Minute), Hardcore: true})
	if h.mayLoot(bran, 2, died) || h.mayLoot(mate, 2, died) || h.mayLoot(liss, 2, died) {
		t.Error("a helped Hardcore kill opened (or opened for Liss before its time)")
	}
	// a corpse no one was credited with (the guards')
	if h.mayLoot(bran, 3, died) || !h.mayLoot(liss, 3, died) {
		t.Error("an uncredited corpse")
	}
}

// Rules 5 and 6: no trade either way, no auction, none of the conveniences.
func TestHardcoreRefusals(t *testing.T) {
	hcOn(t)
	s := testSession()
	bran := &entity.Player{ServerID: 1072, CharServerID: 72, Name: "Bran", Hardcore: true}
	liss := &entity.Player{ServerID: 1073, CharServerID: 73, Name: "Liss"}
	if !hardcoreNoTrade(s, bran, liss) || !hcSaid(s, hcNoTradeWords) {
		t.Error("Bran's trade was not refused")
	}
	s = testSession()
	if !hardcoreNoTrade(s, liss, bran) || !hcSaid(s, "Bran is a Hardcore adventurer and trades with no one.") {
		t.Error("a trade asked of Bran was not refused")
	}
	if hardcoreNoTrade(testSession(), liss, &entity.Player{Name: "Gnolie"}) {
		t.Error("the control: Liss and Gnolie may not trade")
	}
	s = testSession()
	if !hardcoreNoAuction(s, bran) || !hcSaid(s, hcNoAuctionWords) || hardcoreNoAuction(testSession(), liss) {
		t.Error("the auction house")
	}

	gw := entity.NewWorld()
	ih := &InteractHandler{gameWorld: gw}
	s = testSession()
	if !ih.teleporterTalk(s, bran, &entity.NPC{Name: TeleporterName}) || !hcSaid(s, hcTeleporterWords) {
		t.Error("the Teleporter carried Bran")
	}
	s = testSession()
	if !ih.dabrishTalk(s, bran, &entity.NPC{Name: DabrishName}) || !hcSaid(s, hcDabrishWords) {
		t.Error("Dabrish served Bran")
	}
	s = testSession()
	if !ih.homesTalk(s, bran, &entity.NPC{Name: entity.HomesBoardName}) || !hcSaid(s, hcHomesWords) {
		t.Error("the homes' board served Bran")
	}
	s = testSession()
	(&EquipHandler{gameWorld: gw}).useRockOfFreeport(s, bran)
	if !hcSaid(s, hcRockWords) {
		t.Error("the Rock of Freeport carried Bran")
	}
	p := NewPortal(gw)
	s = testSession()
	if !p.Talk(s, bran, &entity.NPC{Name: "Summoning Portal", PortalOf: 9}) || !hcSaid(s, hcPortalWords) {
		t.Error("a Summoning Portal served Bran")
	}
	gw.Players[bran.ServerID] = bran // nobody calls Bran through a portal
	s = testSession()
	p.call(s, liss, 0, "Bran", time.Now())
	if !hcSaid(s, "Bran is a Hardcore adventurer: no portal carries them.") {
		t.Error("Liss called Bran through her portal")
	}
	s = testSession()
	p.Open(s, bran)
	if !hcSaid(s, hcPortalWords) {
		t.Error("Bran opened a Summoning Portal")
	}
	SetMercs(true)
	t.Cleanup(func() { SetMercs(false) })
	m := &Mercs{gw: gw}
	s = testSession()
	if !m.Talk(s, bran, &entity.NPC{Name: MercLiaisonName}) || !hcSaid(s, hcMercLiaisonWords) {
		t.Error("a merc liaison served Bran")
	}

	// world.hardcore off: Bran is anyone
	entity.SetHardcoreOn(false)
	s = testSession()
	ih.teleporterTalk(s, bran, &entity.NPC{Name: TeleporterName})
	if hcSaid(s, hcTeleporterWords) {
		t.Error("world.hardcore off: the Teleporter refused Bran")
	}
}
