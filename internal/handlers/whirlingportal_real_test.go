package handlers

import (
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// Heirloom's database of 2026-11-13 (EQOA_REAL_DB names it; a copy is read),
// with the GMs' changes of that evening done as the game does them: Bonefisher
// Chklurgl (row 122018) cloned and the original deleted (the Portal of the
// Twins, 122072 -> 168438, is in the snapshot already), and a filth squid
// placed where a GM stands (/place of a spawn-only name: its row stands). The
// Whirling Portal (101289) walked through the quest engine: the portal, the
// new Bonefisher by name, the reach under the lake, the four hostile filth
// squids (copies of the spawn-only original, while the placed one stands), the
// kraken, the Bonefisher again. And the twins' portal: the Open Bloody
// Portal's trip lands at the new row. Skipped without a database.
func TestRealWhirlingPortalAfterGMRows(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	path := filepath.Join(t.TempDir(), "wp.db")
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	out, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	in.Close()
	out.Close()
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	gm := database.GMEdit{AccountID: 1, Username: "gm", Via: "game"}
	bone := int64(122018)
	if rec, err := database.NewNPCStore(db).GetNPC(int(bone)); err == nil {
		nb, _, err := db.CloneNPCRowAudited(bone, database.NPCClone{World: rec.World, X: float64(rec.XCoord), Y: float64(rec.YCoord), Z: float64(rec.ZCoord),
			Facing: 0}, gm, "Bonefisher Chklurgl (row 122018)")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.DeleteNPCRowAudited(bone, gm, "Bonefisher Chklurgl (row 122018)"); err != nil {
			t.Fatalf("the Bonefisher's delete: %v", err)
		}
		t.Logf("Bonefisher Chklurgl cloned as row %d, row 122018 deleted", nb)
	}
	src2, err := db.PlaceSourceRow("filth squid", 5)
	if err != nil {
		t.Fatal(err)
	}
	placed, copied, err := db.PlaceNPCRowAudited(src2, database.NPCClone{World: 5, X: 5100, Y: -49.9, Z: 3650}, gm,
		"filth squid", "a GM's /place of filth squid")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("filth squid placed as row %d (a copy of %d), %v", placed, src2, copied)

	w := entity.NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	log.SetOutput(io.Discard)
	w.LoadNPCs(database.NewNPCStore(db))
	if _, _, err := w.LoadSpawnOnly(db); err != nil {
		t.Fatal(err)
	}
	if _, err := w.LoadQuestWalls(db); err != nil {
		t.Fatal(err)
	}
	if _, err := w.LoadAggroRules(db); err != nil {
		t.Fatal(err)
	}
	if _, err := w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	log.SetOutput(os.Stderr)
	inv := database.NewInventoryStore(db)
	qe, err := NewQuestEngine(db, w, mgr, inv, nil)
	if err != nil {
		t.Fatal(err)
	}
	rec := &database.CharacterRecord{CharName: "Squidtest", AccountID: 1, Level: 55}
	if err := database.NewSQLiteCharacterStore(db).Create(rec); err != nil {
		t.Fatal(err)
	}
	sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 9), Port: 7032}, 32, 1, 1, 2, true)
	p := &entity.Player{ServerID: 900032, CharServerID: rec.ServerID, Name: "Squidtest", Level: 55, HP: 5000, MaxHP: 5000,
		X: 5045, Y: -49, Z: 3570, World: 5}
	w.Players[p.ServerID] = p
	w.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	sess.SetInventoryList(newInvList(nil))
	named := func(name string) *entity.NPC {
		for _, c := range w.NPCs {
			if c.Name == name && !c.Temporary && !c.SpawnOnly && c.World == 5 {
				return c
			}
		}
		t.Fatalf("no %q standing in world 5", name)
		return nil
	}
	alive := func(name string) (out []*entity.NPC) {
		for _, c := range w.NPCs {
			if c.Temporary && !c.Dead && strings.EqualFold(c.Name, name) {
				out = append(out, c)
			}
		}
		return out
	}
	stepOf := func() int {
		for _, cq := range qe.CharacterQuests(rec.ServerID) {
			if cq.QuestID == 101289 {
				if cq.Done > 0 && cq.Step == 0 {
					return -1
				}
				return cq.Step
			}
		}
		return 0
	}
	kill := func(n *entity.NPC) {
		n.Dead, n.DeathTime = true, time.Now()
		qe.killed(entity.KillCredit{NPCName: n.Name, NPCSpawn: n.ServerID, NPCID: n.ServerID, Died: time.Now(),
			Players: []entity.KillPlayer{{Session: sess, ServerID: rec.ServerID, Name: p.Name}}})
	}

	// the placed filth squid stands; the originals stay out
	if n := w.NPCs[entity.EntityID(placed)]; n == nil || n.SpawnOnly {
		t.Fatalf("the placed filth squid does not stand: %+v", n)
	}
	for _, id := range []entity.EntityID{123084, 123085, 123086} {
		if n := w.NPCs[id]; n == nil || !n.SpawnOnly {
			t.Fatalf("row %d (a quest's spawn-only original) stands", id)
		}
	}

	if _, err := qe.SetCharacterQuest(rec.ServerID, 101289, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	qe.Talk(sess, p, named("Whirling Portal"))
	if s := stepOf(); s != 2 {
		t.Fatalf("after the Whirling Portal: step %d, want 2", s)
	}
	b := named("Bonefisher Chklurgl")
	if b.ServerID == entity.EntityID(bone) {
		t.Fatal("the deleted Bonefisher row is still the one standing")
	}
	w.UpdatePosition(p.ServerID, b.X+2, b.Y, b.Z+2, 0)
	qe.Talk(sess, p, b)
	if s := stepOf(); s != 3 {
		t.Fatalf("after the Bonefisher (row %d): step %d, want 3", b.ServerID, s)
	}
	w.UpdatePosition(p.ServerID, 5060, -50, 3600, 0)
	qe.checkReach()
	if s := stepOf(); s != 4 {
		t.Fatalf("under the lake: step %d, want 4", s)
	}
	squids := alive("filth squid")
	if len(squids) != 4 {
		t.Fatalf("%d filth squids rose, want 4", len(squids))
	}
	for _, s := range squids {
		if s.CopyOf != 123084 || s.SpawnOnly || s.NPCType&0x80 != 0 {
			t.Fatalf("a quest's filth squid: copy of %d, spawn-only %v, type %#x", s.CopyOf, s.SpawnOnly, s.NPCType)
		}
	}
	for _, s := range squids {
		kill(s)
	}
	if s := stepOf(); s != 5 {
		t.Fatalf("four squids dead: step %d, want 5", s)
	}
	k := alive("Sthlumchoxius")
	if len(k) != 1 {
		t.Fatalf("%d Sthlumchoxius, want 1", len(k))
	}
	kill(k[0])
	if s := stepOf(); s != 6 {
		t.Fatalf("the kraken dead: step %d, want 6", s)
	}
	w.UpdatePosition(p.ServerID, b.X+2, b.Y, b.Z+2, 0)
	qe.Talk(sess, p, b)
	if s := stepOf(); s != -1 {
		t.Fatalf("back at the Bonefisher: step %d, want finished", s)
	}

	// the twins' portal: one row of the name, the GM's new one; the trip lands there
	tw := named("Portal of the Twins")
	x, _, z, _, world, ok := w.SpawnOfNPC("Portal of the Twins")
	if !ok || world != 5 || x != tw.SpawnX || z != tw.SpawnZ || tw.ServerID != 168438 {
		t.Fatalf("the Portal of the Twins' landing: (%.0f, %.0f) world %d %v, row %d", x, z, world, ok, tw.ServerID)
	}
	if _, _, _, _, _, ok := w.SpawnOfNPC("Bloody Portal"); !ok {
		t.Fatal("no Bloody Portal for the twins' portal to send back to")
	}
	t.Logf("Portal of the Twins: row %d at (%.1f, %.1f) world %d", tw.ServerID, x, z, world)
}
