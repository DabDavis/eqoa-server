package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// Talking to a corpse out of its reach says so (Live petition 5348: "When trying to
// loot a corpse too far away, it should say something like 'This is too far away'.
// Right now it says nothing") and opens nothing; one within reach opens. The control
// for the reach: a corpse is reached as the attack button reaches it, InteractRange
// and nothing more for a creature of no size.
func TestACorpseOutOfReachSaysSo(t *testing.T) {
	tw := newTalkWorld(t, 1000, 1000)
	var opened []string
	tw.h.corpse = func(_ *session.Session, n *entity.NPC) bool {
		opened = append(opened, n.Name)
		return true
	}
	far := tw.add(1, &entity.NPC{ServerID: 701, Name: "far beetle", Dead: true, SpawnX: 1000, SpawnY: 54, SpawnZ: 1000 + entity.InteractRange + 2})
	tw.talk(far)
	if len(opened) != 0 {
		t.Fatalf("a corpse out of reach opened: %v", opened)
	}
	if got := opcodes(tw.s); len(got) != 1 {
		t.Fatalf("a corpse out of reach answered %v, want one line", got)
	}
	near := tw.add(2, &entity.NPC{ServerID: 702, Name: "near beetle", Dead: true, SpawnX: 1000, SpawnY: 54, SpawnZ: 1000 + entity.InteractRange - 1})
	tw.talk(near)
	if len(opened) != 1 || opened[0] != "near beetle" {
		t.Fatalf("a corpse within reach: opened %v", opened)
	}
	if got := opcodes(tw.s); len(got) != 0 {
		t.Fatalf("a corpse within reach said %v besides opening", got)
	}
}
