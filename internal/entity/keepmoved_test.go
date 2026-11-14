package entity

import (
	"encoding/json"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// The owner, 2026-11-14: "there was a croc that aggroed weird just recently it charged very
// fast". Build215's keep restart put Croc Island's moved crocodiles on their rows' new spawns
// (RowMoved) while every client kept them in view where they had stood: the client slid each to
// its new place, up to 117 away, in a moment. Now a creature put on its row's new place is let go
// of in every view (removed whole, its slot empty): the visibility round brings it in where it
// stands. The control: the same creature on its unmoved row stays in its slot.
func TestARowMovedCreatureIsLetGoOfThroughARestart(t *testing.T) {
	for _, moved := range []bool{false, true} {
		old := newKeepRig(t, "crocodile", false)
		c := old.w.NPCs[5001]
		c.SpawnX, c.SpawnZ, c.HP = 25367.8, 17327.7, 50 // hurt: written
		old.see(t, old.sess, 5001)
		b, err := json.Marshal(struct {
			S session.Kept
			V KeptView
			N []KeptNPC
		}{old.sess.Keep(), old.w.KeepView(old.sess), old.w.KeepNPCs()})
		if err != nil {
			t.Fatal(err)
		}
		var back struct {
			S session.Kept
			V KeptView
			N []KeptNPC
		}
		if err := json.Unmarshal(b, &back); err != nil || len(back.N) != 1 {
			t.Fatalf("written %s (%v)", b, err)
		}
		w := NewWorld()
		sm := session.NewManager()
		w.SessionManager = sm
		sess, err := sm.Resume(back.S)
		if err != nil {
			t.Fatal(err)
		}
		w.CreatePlayer(sess, &database.CharacterRecord{ServerID: 61, CharName: "Jibcro", AccountID: 7, Level: 60,
			Race: 3, TClass: 11, X: 25375, Y: 54.1, Z: 15721})
		spawnX := float32(25367.8)
		if moved {
			spawnX = 25382.4 // the data step's new spawn
		}
		w.NPCs[5001] = &NPC{ServerID: 5001, Name: "crocodile", HP: 100, MaxHP: 100, Level: 10, SpawnX: spawnX, SpawnZ: 17327.7,
			X: spawnX, Z: 17327.7}
		if r := w.RestoreNPCs(back.N); (r.RowMoved == 1) != moved {
			t.Fatalf("moved %v: restored %+v", moved, r)
		}
		kept, gone := w.RestoreView(sess, back.V)
		slot := sess.VisibilityState().(*VisibilityState).ChannelEntity[0]
		switch {
		case !moved && (kept != 1 || slot != 5001):
			t.Errorf("the control: %d kept, slot %d, want the crocodile kept", kept, slot)
		case moved && (gone != 1 || slot != 0):
			t.Errorf("on its row's new place: %d taken away, slot %d, want it let go of", gone, slot)
		}
		if moved {
			if data, _, _ := sess.Connection().GetServerObject(1).GenerateXORDelta(); data == nil || data[0] != 0 {
				t.Errorf("no removal: %x", data)
			}
		}
	}
}
