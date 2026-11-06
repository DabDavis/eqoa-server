package handlers

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// weaponFight is a level 60 bard (class 5, INT 120) wielding the named record
// of a real database in each hand, as a login puts it on (wornGear, SetGear),
// swinging at a Training Dummy beside it (which never dies) for rounds rounds
// through the world's own fight (EngageAttack, then World.Tick every 100 ms
// with the attack button held): the log's "Proc:" lines about the weapon,
// and how many of the bard's hits landed (its 0x00DB figures on the dummy, a
// proc's among them).
func weaponFight(t *testing.T, db *database.SQLiteDB, name string, rounds int) (procs []string, hits int) {
	t.Helper()
	var id int
	if err := db.QueryRow(`SELECT patternid FROM itempattern WHERE lower(itemname) = lower(?) ORDER BY patternid LIMIT 1`, name).Scan(&id); err != nil {
		t.Fatalf("no record %q: %v", name, err)
	}
	rec, err := database.NewItemPatternStore(db).GetItemPattern(id)
	if err != nil {
		t.Fatal(err)
	}
	gear := wornGear([]database.CharInventoryEntry{
		{EquipLoc: 14, RemainHP: rec.MaxHP, StackLeft: 1, Pattern: *rec},
		{EquipLoc: 15, RemainHP: rec.MaxHP, StackLeft: 1, Pattern: *rec},
	})

	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	w := entity.NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 8), Port: 7000}, 1, 1, 1, 2, true)
	sess.SetPlayerObjectID(0x1424A1D0)
	p := w.CreatePlayer(sess, &database.CharacterRecord{CharName: "Rhymer", ServerID: 9001, TClass: 5, Race: 0, Level: 60,
		TPIntel: 60, X: 100, Y: 10, Z: 100})
	if p == nil {
		t.Fatal("no player")
	}
	sess.SetCharacterInWorld(true)
	w.SetGear(p.ServerID, gear, true)
	if _, err := w.UpsertNPC(&database.NPCRecord{ID: 77001, NPCName: "Training Dummy", NPCLevel: 50, XCoord: 101, YCoord: 10,
		ZCoord: 100, HP: 50000}, 0); err != nil {
		t.Fatal(err)
	}
	time.Sleep(entity.NPCRefreshHold + 100*time.Millisecond) // a new row's NPC is out of view that long, on the wall clock

	t0 := time.Unix(1_800_000_000, 0)
	now := t0
	step := func(d time.Duration) {
		for end := now.Add(d); now.Before(end); now = now.Add(100 * time.Millisecond) {
			w.Tick(now)
		}
	}
	step(time.Second) // the visibility system puts the dummy on a slot
	vs := entity.GetOrCreateVisibility(sess)
	var dummy uint32
	for i, e := range vs.ChannelEntity {
		if e == 77001 {
			dummy = vs.ChannelObjectID[i]
		}
	}
	if dummy == 0 {
		log.SetOutput(os.Stderr)
		t.Fatalf("the dummy is not in view: slots %v; log:\n%s", vs.ChannelEntity[:4], tail(buf.String(), 3000))
	}
	swing := entity.SwingFor(5, 60, [21]bool{14: true, 15: true}, true, rec.AttackType, 0)
	if !w.EngageAttack(sess, dummy, swing, now) {
		t.Fatal("not engaged")
	}
	w.ApplyClientUpdate(p.ServerID, entity.ClientUpdate{X: 100, Y: 10, Z: 100, AutoAttack: true, HasAutoAttack: true})
	for r := 0; r < rounds; r++ {
		step(swing.Delay)
		for _, m := range sess.Queue().GetPending() {
			b := m.Data()
			if len(b) >= 7 && binary.LittleEndian.Uint16(b) == uint16(protocol.OpcodeDamageIndication) &&
				binary.LittleEndian.Uint32(b[2:6]) == sess.PlayerObjectID() {
				hits++
			}
		}
	}

	fightLog = tail(buf.String(), 4000)
	for _, l := range strings.Split(buf.String(), "\n") {
		switch {
		case strings.Contains(l, "Proc:") && strings.Contains(strings.ToLower(l), strings.ToLower(name)):
			procs = append(procs, l)
		}
	}
	return procs, hits
}

var fightLog string

func tail(s string, n int) string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}

// realDB opens EQOA_REAL_DB, a copy of a server's database, or skips.
func realDB(t *testing.T) *database.SQLiteDB {
	t.Helper()
	path := os.Getenv("EQOA_REAL_DB")
	if path == "" {
		t.Skip("EQOA_REAL_DB names a copy of a server's database")
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// A Selo's Jeweled Rapier in each hand, as a login wears it, over a long
// fight with a training dummy (TestRealSelosRapierFight measured 600 hits and
// no proc before entity/weaponproc.go): now some of its hits proc, each for
// its 140, about as often as proc.go's rule says for the bard's INT. The
// control is the bard's other level 49 pick, Ballad of Mithril, a weapon of
// the same hand and no proc: none.
func TestRealSelosRapierFight(t *testing.T) {
	db := realDB(t)
	procs, hits := weaponFight(t, db, "Selo's Jeweled Rapier", 150)
	t.Logf("150 rounds of Selo's Jeweled Rapier: %d figures on the dummy, %d of them procs", hits, len(procs))
	for i, l := range procs {
		if i < 3 {
			t.Log(l)
		}
		if !strings.Contains(l, "for 140") {
			t.Errorf("a proc not of 140: %s", l)
		}
	}
	melee := hits - len(procs)
	if melee < 500 || len(procs) < melee/30 || len(procs) > melee/4 {
		t.Errorf("%d procs on %d hits", len(procs), melee)
	}
	if procs, hits := weaponFight(t, db, "Ballad of Mithril", 50); len(procs) != 0 || hits < 150 {
		t.Errorf("the control, Ballad of Mithril: %d procs on %d hits", len(procs), hits)
	}
}

// Every weapon record whose description names a proc reads as one, and none
// of the rest does but the named ones (entity/weaponproc.go's table): the
// list, by what each does.
func TestRealWeaponProcsRead(t *testing.T) {
	db := realDB(t)
	rows, err := db.Query(`SELECT patternid, itemname, itemdesc FROM itempattern WHERE equipslot IN (12, 14, 15, 16, 18)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var read, missed []string
	for rows.Next() {
		var id int
		var name, desc string
		if err := rows.Scan(&id, &name, &desc); err != nil {
			t.Fatal(err)
		}
		wp, ok := entity.WeaponProcOf(id, name, desc)
		names := strings.Contains(strings.ToLower(desc), "proc")
		switch {
		case ok:
			read = append(read, fmt.Sprintf("%d %s: %s", id, name, wp))
		case names:
			missed = append(missed, fmt.Sprintf("%d %s: %q", id, name, desc))
		}
	}
	sort.Strings(read)
	t.Logf("%d weapons proc:\n%s", len(read), strings.Join(read, "\n"))
	t.Logf("named a proc and not read (no figure or effect known):\n%s", strings.Join(missed, "\n"))
	if len(read) < 15 {
		t.Errorf("only %d weapons read a proc", len(read))
	}
	for _, m := range missed {
		if !strings.Contains(m, "Infectious Dread") {
			t.Errorf("not read: %s", m)
		}
	}
}
