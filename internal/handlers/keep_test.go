package handlers

import (
	"encoding/binary"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// keepDB is a database with an account and its character.
func keepDB(t *testing.T) *database.SQLiteDB {
	t.Helper()
	db, err := database.NewSQLite(t.TempDir() + "/keep.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, q := range []string{
		`INSERT INTO accountinfo (accountid, username, pwhash, acctstatus, acctlevel) VALUES (3, 'player', 'x', 1, 0)`,
		`INSERT INTO characters (charName, accountid, serverid, modelid, tclass, race, humType, level, haircolor, hairlength, hairstyle,
			faceoption, sex, world, x, y, z) VALUES ('Hero', 3, 7, 1, 3, 5, 1, 20, 0, 0, 0, 0, 0, 0, 25330, 54.1, 15721)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

// keepProcess is one process's character handler and World over db, made the
// one a resume goes through (keepChars), as RegisterDefaultHandlers makes it.
func keepProcess(t *testing.T, db *database.SQLiteDB) (*CharacterHandler, *entity.World, *session.Manager) {
	t.Helper()
	sm := session.NewManager()
	gw := entity.NewWorld()
	gw.SessionManager = sm
	h := NewCharacterHandler(database.NewSQLiteCharacterStore(db), sm, database.NewInventoryStore(db), database.NewSpellbookStore(db))
	h.SetGameWorld(gw)
	gw.OnPlayerRespawn(h.respawnPlayer)
	prev := keepChars.Swap(h)
	t.Cleanup(func() { keepChars.Store(prev) })
	return h, gw, sm
}

// queuedOpcodes are the opcodes of the reliable messages queued on s.
func queuedOpcodes(s *session.Session) map[protocol.GameOpcode]int {
	n := map[protocol.GameOpcode]int{}
	for _, m := range s.Queue().PeekPending() {
		if d := m.Data(); m.Type.IsReliable() && len(d) >= 2 {
			n[protocol.GameOpcode(binary.LittleEndian.Uint16(d))]++
		}
	}
	return n
}

// A character in the world through a restart: the new process builds it from
// its row as a login does, gives back the server's copy of the client's item
// list (the keys the client sends until its dump) and what its client asked
// for; nothing goes to the client until its dump is due, and then a dump goes
// where it stands, with the line that says the server was updated.
func TestAResumedCharacterComesBackWithItsListsAndADump(t *testing.T) {
	db := keepDB(t)
	_, gw, sm := keepProcess(t, db)
	sess := sm.CreateSession(&net.UDPAddr{IP: net.IPv4(203, 0, 113, 7), Port: 5007}, 7, 7, 7, 1, true)
	sess.SetAccountID(3)
	sess.SetPlayerObjectID(500000007)
	sess.SetState(session.StateInGame)
	sess.SetCharacterInWorld(true)
	rec, err := database.NewSQLiteCharacterStore(db).GetByServerID(7)
	if err != nil {
		t.Fatal(err)
	}
	if p := keepChars.Load().enterFromRecord(sess, rec); p == nil {
		t.Fatal("no player")
	}
	// the client's list as its dump and two adds left it: keys 0, 1, then 5
	sess.SetInventoryList(&invList{recs: []invRec{{key: 0, itemID: 11, patternID: 100, qty: 1},
		{key: 1, itemID: 12, patternID: 101, qty: 20}, {key: 5, itemID: 13, patternID: 102, qty: 1}}, next: 6})
	kp, ok := gw.KeepPlayer(sess, time.Now())
	if !ok {
		t.Fatal("no player kept")
	}
	ex := KeepExtras(sess, gw.SessionPlayer(sess))
	b, _ := json.Marshal(struct {
		S session.Kept
		P entity.KeptPlayer
		E KeptExtras
	}{sess.Keep(), kp, ex})

	// the new process
	_, gw2, sm2 := keepProcess(t, db)
	var back struct {
		S session.Kept
		P entity.KeptPlayer
		E KeptExtras
	}
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	rs, err := sm2.Resume(back.S)
	if err != nil {
		t.Fatal(err)
	}
	p, err := ResumeCharacter(rs, back.E, back.P, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "Hero" || p.Level != 20 || gw2.SessionPlayer(rs) != p || p.CharServerID != 7 {
		t.Fatalf("player %+v", p)
	}
	l := sessionInvList(rs)
	if l == nil || len(l.recs) != 3 || l.recs[2].key != 5 || l.recs[2].itemID != 13 || l.next != 6 {
		t.Fatalf("the item list %+v", l)
	}
	if n := queuedOpcodes(rs)[protocol.OpcodeMemoryDump]; n != 0 {
		t.Fatal("a dump went before it was due")
	}
	if !ResumeDump(rs, false) {
		t.Fatal("no dump")
	}
	ops := queuedOpcodes(rs)
	if ops[protocol.OpcodeMemoryDump] != 1 {
		t.Fatalf("dumps queued: %v", ops)
	}
	if lines := gmReplies(t, rs); !hasLine(lines, ResumeLine) {
		t.Fatalf("the lines: %q", lines)
	}
	// the dump's own list replaces the kept one (keys from 0 again)
	if l := sessionInvList(rs); l == nil || l.next != uint32(len(l.recs)) {
		t.Fatalf("the list after the dump %+v", l)
	}
}

// One that was dead when the restart came respawns with its dump, as it
// would have; and the control: a session whose account is not the
// character's gets no character.
func TestAResumedDeadCharacterRespawnsAndAStrangersIsRefused(t *testing.T) {
	db := keepDB(t)
	_, gw, sm := keepProcess(t, db)
	k := session.Kept{Addr: "203.0.113.7:5007", InstanceID: 7, SessionID: 500000007, ClientEndpoint: 7, ServerEndpoint: 1,
		State: session.StateInGame, HasInstance: true, DidServerInitiate: true, CharacterInWorld: true, InGame: true,
		AccountID: 3, CharServerID: 7, PlayerObjectID: 500000007}
	rs, err := sm.Resume(k)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ResumeCharacter(rs, KeptExtras{}, entity.KeptPlayer{CharServerID: 7, Name: "Hero", Dead: true}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if !ResumeDump(rs, true) || queuedOpcodes(rs)[protocol.OpcodeMemoryDump] != 1 {
		t.Fatalf("no respawn dump: %v", queuedOpcodes(rs))
	}
	if p := gw.SessionPlayer(rs); p == nil || p.IsDead() {
		t.Fatal("the respawned character is not alive")
	}

	k.Addr, k.InstanceID, k.AccountID = "203.0.113.8:5008", 8, 4
	rs2, err := sm.Resume(k)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ResumeCharacter(rs2, KeptExtras{}, entity.KeptPlayer{CharServerID: 7, Name: "Hero"}, time.Now()); err == nil {
		t.Fatal("another account's session was given the character")
	}
}
