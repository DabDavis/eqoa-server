package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// Heirloom petition 3049, the second isle of rain: "Sterie Glitterlight will be
// roaming in a short circle around this location, he will not spawn unless
// someone logs the quest." Quest 101302's step 1 (talk to Sterie) spawns her at
// her place from an original that stands out of the world; a player who lost
// that spawn (its life up, away and back, a keep restart) gets it again on
// coming within questRespawnReach of where the original stands, once, and
// talking to the copy does the step. The controls: from 40 off nothing comes,
// and a talk step whose target stands in the world is never made again.
func TestALostTalkStepSpawnOfASpawnOnlyNPCComesBack(t *testing.T) {
	f := newQuestFixture(t)
	f.tw.p.HP, f.tw.p.MaxHP = 100, 100 // alive: in the world
	const cx, cz = 25440, 15725
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (130, 'Sterie Glitterlight', 1, 'Trixie Longwings')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal, says, events)
		VALUES (130, 1, 'talk', 'Sterie Glitterlight', 'Find Sterie.', 'You found me!',
			'[{"on":"start","do":"spawn","npc":"Sterie Glitterlight","minutes":60,"world":0,"x":25440,"y":54,"z":15725}]')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (130, 2, 'kill', 'Kaglanor', 'Kill Kaglanor.')`)
	f.qe.Reload()
	sterie := &entity.NPC{ServerID: 120091, Name: "Sterie Glitterlight", NPCType: 0x80, Level: 60, HP: 100, MaxHP: 100,
		SpawnX: cx, SpawnY: 54, SpawnZ: cz, X: cx, Y: 54, Z: cz}
	f.tw.w.NPCs[sterie.ServerID] = sterie
	f.tw.w.SpatialIndex.Insert(sterie.ServerID, sterie.X, sterie.Z)
	f.tw.w.SetSpawnOnly(map[string]bool{database.NPCClassKey("Sterie Glitterlight"): true})
	if _, err := f.qe.SetCharacterQuest(f.sid, 130, QuestSetStep, 1); err != nil {
		t.Fatal(err)
	}

	// the step's start makes her at her place
	if cs := f.questCopies(); len(cs) != 1 || cs[0].X != cx || cs[0].Z != cz {
		t.Fatalf("the step started: %d Steries, want one at her place", len(cs))
	}
	away := func() {
		t.Helper()
		back := f.qe.sessionOf
		f.qe.sessionOf = func(uint32) *session.Session { return nil }
		f.qe.expireSpawns()
		f.qe.sessionOf = back
		if n := f.liveCopies(); n != 0 {
			t.Fatalf("the player away, %d Steries stay", n)
		}
	}
	away()

	f.tw.p.X, f.tw.p.Z = cx+40, cz
	f.qe.respawnLost()
	if n := f.liveCopies(); n != 0 {
		t.Fatalf("40 off the original, %d Steries came", n)
	}
	f.tw.p.X = cx + 20
	f.qe.respawnLost()
	var cs []*entity.NPC
	for _, c := range f.questCopies() {
		if f.tw.w.NPCs[c.ServerID] != nil && !c.Dead && c.QuestSpawn {
			cs = append(cs, c)
		}
	}
	if len(cs) != 1 || cs[0].X != cx || cs[0].Z != cz || cs[0].SpawnOnly || cs[0].CurrentEnemy != 0 {
		t.Fatalf("20 off the original: %d Steries (want one, at her place, attacking no one)", len(cs))
	}
	f.qe.respawnLost()
	if n := f.liveCopies(); n != 1 {
		t.Fatalf("a second Sterie while the first stands: %d", n)
	}

	// the copy answers the talk step
	f.tw.p.X = cx + 3
	vs := entity.GetOrCreateVisibility(f.tw.s)
	vs.ChannelEntity[5], vs.ChannelObjectID[5] = cs[0].ServerID, 100005
	f.tw.talk(100005)
	if s, _ := f.progress(130); s != 2 {
		t.Fatalf("talked to Sterie's copy, the quest is at step %d, want 2", s)
	}

	// the control: a talk step's NPC that stands in the world is never made again
	if _, err := f.qe.SetCharacterQuest(f.sid, 130, QuestSetStep, 1); err != nil {
		t.Fatal(err)
	}
	away()
	f.tw.w.SetSpawnOnly(nil)
	f.qe.respawnLost()
	if n := f.liveCopies(); n != 0 {
		t.Errorf("a talk step's NPC that stands in the world was made again: %d", n)
	}
}
