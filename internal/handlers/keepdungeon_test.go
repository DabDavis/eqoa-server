package handlers

import (
	"encoding/json"
	"net"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
	"github.com/eqoa/server/internal/worlds"
)

// keepDungeonProcess is one process over db with the dungeons on (world 14, "bb"), its corpses' handler the one a
// restart keeps and restores through (keepWindows), and every player its gates send collected.
func keepDungeonProcess(t *testing.T, db *database.SQLiteDB) (*entity.World, *session.Manager, *NPCWindowHandler, *[]entity.GateTravel) {
	t.Helper()
	_, gw, sm := keepProcess(t, db)
	t.Cleanup(func() { gw.SetDungeons(false, 14, nil, entity.DungeonSettings{}) })
	gw.LoadNPCs(database.NewNPCStore(db))
	if !SetupDungeons(gw, config.DungeonsConfig{Enabled: true}, nil) {
		t.Fatal("the dungeons are not on")
	}
	wh := NewNPCWindowHandler(sm, database.NewInventoryStore(db))
	wh.SetGameWorld(gw)
	prev := keepWindows.Swap(wh)
	t.Cleanup(func() { keepWindows.Store(prev) })
	sent := &[]entity.GateTravel{}
	gw.OnTravel(func(tr entity.GateTravel) { *sent = append(*sent, tr) })
	return gw, sm, wh, sent
}

// keepDungeonSession is a session of account acct in the world holding world 14, its character char.
func keepDungeonSession(sm *session.Manager, acct int, char uint32, port int) *session.Session {
	s := sm.CreateSession(&net.UDPAddr{IP: net.IPv4(203, 0, 113, byte(port%250)), Port: port}, char, char, uint16(char), 1, true)
	s.SetAccountID(acct)
	s.SetPlayerObjectID(500000000 + char)
	s.SetState(session.StateInGame)
	s.SetCharacterInWorld(true)
	s.SetServerWorlds([]uint8{14})
	s.SetCharacterServerID(char)
	return s
}

// The owner, 2026-11-13: "on server restart the instanced raids are booting players, lets keep them in the instance
// and progress". A raid of two (Hero leads, Mate is its member) in its instance of a dungeon of four creatures: one
// killed and its corpse gone, one killed and lying with loot on it, one fought to 1,234 hit points and dragged off its
// spawn, one untouched. Through a restart that keeps the sessions (each part written and read back as JSON, the new
// process's World and handlers fresh, restored in udp/keep.go's order: creatures, instances, corpses, players, groups)
// the instance is there under its id, 2 of its 4 killed, the fought one where it stood at its hit points, the corpse
// lying with what it held; both players are in it where they stood (not at the entrance, not moved), and no one is
// sent out however long dungeonsSystem looks.
func TestADungeonInstanceGoesThroughAKeepRestart(t *testing.T) {
	db := keepDB(t)
	for _, q := range []string{
		`INSERT INTO accountinfo (accountid, username, pwhash, acctstatus, acctlevel) VALUES (4, 'mate', 'x', 1, 0)`,
		`INSERT INTO characters (charName, accountid, serverid, modelid, tclass, race, humType, level, haircolor, hairlength, hairstyle,
			faceoption, sex, world, x, y, z) VALUES ('Mate', 4, 8, 1, 3, 5, 1, 60, 0, 0, 0, 0, 0, 0, 5004, 50, 5004)`,
		`UPDATE characters SET world = 0, x = 5002, y = 50, z = 5004, level = 60 WHERE serverid = 7`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range []struct {
		id      int
		name    string
		world   int
		x, z    float32
		npcType int
	}{
		{990201, entity.DungeonWardenName, 0, 5000, 5000, 128},
		{990301, "gnoll scout", 14, 950, 950, 0},
		{990302, "gnoll guard", 14, 1000, 950, 0},
		{990303, "Lord Elgnub", 14, 1050, 1000, 0},
		{990304, "gnoll pup", 14, 950, 1050, 0},
	} {
		if _, err := db.Exec(`INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race, zone)
			VALUES (?, ?, ?, ?, 50, ?, 0, 1843770060, 1, 60, ?, '0', 0)`, n.id, n.name, n.world, n.x, n.z, n.npcType); err != nil {
			t.Fatal(err)
		}
	}
	block := `[{"key":"bb","name":"Test Burrow","boxes":[[900,900,1100,1100]],"entrance":{"x":1000,"y":50,"z":1150,"toward":[1000,1000]}}]`
	SetServerWorlds([]worlds.Built{{Recipe: worlds.Recipe{World: 14, Name: "the Test Dungeons", Dungeons: json.RawMessage(block)}}})
	t.Cleanup(func() { SetServerWorlds(nil) })

	// the old process: both in the world by the warden, grouped, then in Hero's instance
	gw, sm, wh, _ := keepDungeonProcess(t, db)
	hero, mate := keepDungeonSession(sm, 3, 7, 5007), keepDungeonSession(sm, 4, 8, 5008)
	store := database.NewSQLiteCharacterStore(db)
	ps := map[uint32]*entity.Player{}
	for _, s := range []*session.Session{hero, mate} {
		rec, err := store.GetByServerID(s.CharacterServerID())
		if err != nil {
			t.Fatal(err)
		}
		if ps[rec.ServerID] = keepChars.Load().enterFromRecord(s, rec); ps[rec.ServerID] == nil {
			t.Fatal("no player")
		}
	}
	if n, err := gw.RestoreGroups([]entity.KeptGroup{{ID: 41, Members: []uint32{7, 8}}}); n != 1 || err != nil {
		t.Fatalf("the group: %d %v", n, err)
	}
	now := time.Now()
	for _, s := range []*session.Session{hero, mate} {
		if why, ok := gw.DungeonEnter(s, "bb", now); !ok {
			t.Fatal(why)
		}
	}
	// where each stands in it (the gate's landing, then a walk), as the shutdown's save writes it
	where := map[uint32][3]float32{7: {1010, 50, 1010}, 8: {1020, 50, 990}}
	for c, at := range where {
		p := ps[c]
		gw.SpatialIndex.Remove(p.ServerID, p.X, p.Z)
		p.World, p.X, p.Y, p.Z = 14, at[0], at[1], at[2]
		gw.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
		if _, err := db.Exec(`UPDATE characters SET world = 14, x = ?, y = ?, z = ? WHERE serverid = ?`, at[0], at[1], at[2], c); err != nil {
			t.Fatal(err)
		}
	}
	inst, open := gw.DungeonInstanceInfo(ps[7].ServerID)
	if inst == 0 || !open {
		t.Fatalf("Hero in no instance: %d %v", inst, open)
	}
	if mi, _ := gw.DungeonInstanceInfo(ps[8].ServerID); mi != inst {
		t.Fatalf("Mate in instance %d, Hero %d", mi, inst)
	}
	copies := map[uint32]*entity.NPC{} // by the row each is a copy of
	for _, n := range gw.NPCs {
		if n.Temporary && n.Instance == inst {
			copies[uint32(n.CopyOf)] = n
		}
	}
	if len(copies) != 4 {
		t.Fatalf("%d creatures in the instance", len(copies))
	}
	gone, lying, fought := copies[990301], copies[990302], copies[990303]
	gw.KillNPCNow(gone.ServerID)
	gone.CorpseGone = now // looted clean and gone
	gw.KillNPCNow(lying.ServerID)
	loot := []database.CharInventoryEntry{{ItemID: 1, StackLeft: 1, Pattern: database.ItemPatternRecord{PatternID: 4242}}}
	wh.loots().corpses[lying.ServerID] = &corpse{died: lying.DeathTime, items: loot}
	gw.SpatialIndex.Update(fought.ServerID, fought.X, fought.Z, 1040, 1020)
	fought.X, fought.Z, fought.Facing, fought.HP = 1040, 1020, 2.5, 1234
	if !strings.Contains(gw.DungeonStatus(hero), "2 of 4 creatures killed") {
		t.Fatalf("before: %q", gw.DungeonStatus(hero))
	}

	// the restart's file
	type keptSess struct {
		S session.Kept
		P entity.KeptPlayer
	}
	var file struct {
		Sessions []keptSess
		Groups   []entity.KeptGroup
		Dungeons *entity.KeptDungeons
		Corpses  []KeptCorpse
	}
	for _, s := range []*session.Session{hero, mate} {
		kp, ok := gw.KeepPlayer(s, now)
		if !ok {
			t.Fatal("no player kept")
		}
		file.Sessions = append(file.Sessions, keptSess{s.Keep(), kp})
	}
	file.Groups, file.Dungeons, file.Corpses = gw.KeepGroups(), gw.KeepDungeons(now), KeepCorpses()
	b, err := json.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	file.Sessions, file.Groups, file.Dungeons, file.Corpses = nil, nil, nil, nil
	if err := json.Unmarshal(b, &file); err != nil {
		t.Fatal(err)
	}
	if file.Dungeons == nil || len(file.Dungeons.Insts) != 1 || len(file.Dungeons.Insts[0].NPCs) != 3 {
		t.Fatalf("written: %s", b)
	}

	// the new process, in udp/keep.go's ResumeKept's order
	gw2, sm2, wh2, sent := keepDungeonProcess(t, db)
	ids, r := gw2.RestoreDungeons(file.Dungeons)
	if r.Insts != 1 || r.Alive != 2 || r.Dead != 1 || r.NPCsLost != 0 || r.Players != 2 {
		t.Fatalf("restored %+v", r)
	}
	if n := RestoreCorpsesAs(file.Corpses, ids); n != 1 {
		t.Fatalf("%d corpses' contents back", n)
	}
	sess2 := map[uint32]*session.Session{}
	ps2 := map[uint32]*entity.Player{}
	for _, ks := range file.Sessions {
		s, err := sm2.Resume(ks.S)
		if err != nil {
			t.Fatal(err)
		}
		p, err := ResumeCharacter(s, KeptExtras{}, ks.P, now)
		if err != nil {
			t.Fatal(err)
		}
		sess2[p.CharServerID], ps2[p.CharServerID] = s, p
	}
	if n, err := gw2.RestoreGroups(file.Groups); n != 1 || err != nil {
		t.Fatalf("the group again: %d %v", n, err)
	}
	if n := gw2.DungeonsResumed(now); n != 2 {
		t.Fatalf("%d players back in an instance", n)
	}

	for c, at := range where {
		p := ps2[c]
		if got, open := gw2.DungeonInstanceInfo(p.ServerID); got != inst || !open {
			t.Fatalf("%s back in instance %d (open %v), want %d", p.Name, got, open, inst)
		}
		if p.World != 14 || p.X != at[0] || p.Z != at[2] || sess2[c].ResumeMoved() {
			t.Fatalf("%s back at (%.1f, %.1f) world %d (moved %v), want where it stood (%.1f, %.1f)", p.Name, p.X, p.Z,
				p.World, sess2[c].ResumeMoved(), at[0], at[2])
		}
	}
	if p := ps2[8]; p.GroupID != 41 {
		t.Fatalf("Mate's group %d", p.GroupID)
	}
	if st := gw2.DungeonStatus(sess2[7]); !strings.Contains(st, "2 of 4 creatures killed") {
		t.Fatalf("after: %q", st)
	}
	back := map[uint32]*entity.NPC{}
	for _, n := range gw2.NPCs {
		if n.Temporary && n.Instance == inst {
			back[uint32(n.CopyOf)] = n
		}
	}
	if len(back) != 3 || back[990301] != nil {
		t.Fatalf("the instance's creatures: %d, the one whose corpse went made again %v", len(back), back[990301] != nil)
	}
	f := back[990303]
	if f == nil || f.Dead || f.HP != 1234 || f.X != 1040 || f.Z != 1020 || f.Facing != 2.5 || f.ServerID != ids[fought.ServerID] {
		t.Fatalf("the fought one: %+v", f)
	}
	l := back[990302]
	if l == nil || !l.Dead || !l.DeathTime.Equal(lying.DeathTime) {
		t.Fatalf("the lying one: %+v", l)
	}
	if c := wh2.loots().corpses[l.ServerID]; c == nil || len(c.items) != 1 || c.items[0].Pattern.PatternID != 4242 {
		t.Fatalf("its loot: %+v", c)
	}
	if u := back[990304]; u == nil || u.Dead || u.HP != u.MaxHP {
		t.Fatalf("the untouched one: %+v", u)
	}

	// dungeonsSystem looks over them, well past its graces: no one is sent out
	for _, d := range []time.Duration{2, 6, 12, 20, 40} {
		gw2.Tick(now.Add(d * time.Second))
	}
	if len(*sent) != 0 {
		var who []string
		for _, s := range *sent {
			who = append(who, s.P.Name)
		}
		sort.Strings(who)
		t.Fatalf("sent out after the restart: %v", who)
	}
	for c := range where {
		if got, open := gw2.DungeonInstanceInfo(ps2[c].ServerID); got != inst || !open {
			t.Fatalf("%s in instance %d (open %v) after the ticks", ps2[c].Name, got, open)
		}
	}

	// its owner logging in again after the restart (not carried through it) finds the instance too, at its entrance
	gw2.RemovePlayer(ps2[7].ServerID)
	rec := &database.CharacterRecord{ServerID: 7, CharName: "Hero", World: 14, X: 1010, Y: 50, Z: 1010}
	loginSpawn(gw2, rec, func(w uint8) bool { return w == 14 }, nil)
	if rec.World != 14 || rec.X != 1000 || rec.Z != 1150 {
		t.Fatalf("Hero's login after the restart: world %d (%.1f, %.1f), want the entrance", rec.World, rec.X, rec.Z)
	}
}
