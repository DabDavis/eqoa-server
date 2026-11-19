package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// The Spell Archivist copies no scrolls for a Hardcore character (petition
// 2399): his question is never asked; a plain character is asked it.
func TestSpellArchivistRefusesHardcore(t *testing.T) {
	was := entity.HardcoreOn()
	entity.SetHardcoreOn(true)
	defer entity.SetHardcoreOn(was)
	a := NewArchivist(&database.SpellbookStore{}, nil)
	npc := &entity.NPC{Name: SpellArchivistName}
	bran := &entity.Player{ServerID: 1072, CharServerID: 72, Name: "Bran", Hardcore: true}
	s := testSession()
	if !a.Ask(s, bran, npc, 7) || !hcSaid(s, "I copy no scrolls") {
		t.Error("the Spell Archivist served Bran")
	}
	a.mu.Lock()
	_, asked := a.asked[bran.CharServerID]
	a.mu.Unlock()
	if asked {
		t.Error("Bran was asked the Archivist's question")
	}
	liss := &entity.Player{ServerID: 1073, CharServerID: 73, Name: "Liss"}
	s = testSession()
	if !a.Ask(s, liss, npc, 7) || hcSaid(s, "I copy no scrolls") {
		t.Error("the control: Liss was refused")
	}
}
