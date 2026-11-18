package handlers

import (
	"fmt"
	"math"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// The owner, 2026-11-18: a /place of Tunare (row 190868, a copy of 162665) stood
// without her pets, Wrath and Grace of the Mother, spawn-only originals beside
// the original, paired with her by trigger 141's resummons alone. /place now
// makes a row of each beside the new one, as the originals stand off hers,
// standing (npcstands) as hers does, and the rows pair with her (a pet's
// master); /clone of the placed row brings its pets again; #spawn brings copies.
// The control: /place of a creature with none makes one row.
func TestPlaceBringsItsCompanions(t *testing.T) {
	c, db, _, gs, gp, _ := campWorld(t)
	for _, r := range []struct {
		id   int
		name string
		x, z float64
	}{{150, "Tunare", 25344.88, 15749.21}, {151, "Wrath of the Mother", 25340.17, 15743.23}, {152, "Grace of the Mother", 25352.49, 15754.42}} {
		if _, err := db.Exec(`INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race, zone)
			VALUES (?, ?, 0, ?, 54, ?, 0, ?, 1.0, 60, 0, '0', 7)`, r.id, r.name, r.x, r.z, campCreature); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO npcspawnonly (name, note, source) VALUES (?, 'trigger 141', 'gm')`, r.name); err != nil {
			t.Fatal(err)
		}
	}
	c.gw.LoadNPCs(database.NewNPCStore(db))
	if _, _, err := c.gw.LoadSpawnOnly(db); err != nil {
		t.Fatal(err)
	}
	steps := `[{"do":"spawn","npc":"Tunare"},{"do":"resummon","npc":"Wrath of the Mother","by":"Tunare","seconds":20},` +
		`{"do":"resummon","npc":"Grace of the Mother","by":"Tunare","seconds":20},{"do":"despawn"}]`
	pets := triggerCompanions([]database.GMTrigger{{Slot: 141, Steps: steps}})
	if len(pets) != 2 || pets[database.NPCClassKey("Wrath of the Mother")] != "Tunare" {
		t.Fatalf("trigger 141's pairs: %v", pets)
	}
	c.gw.SetTriggerCompanions(pets)

	stand(c, gp, 25600, 52, 15600, 0)
	before := count(t, db, `SELECT COUNT(*) FROM npcs`)
	c.say(gs, "/place Tunare")
	var nid int64
	db.QueryRow(`SELECT MIN(id) FROM npcs WHERE id > 152`).Scan(&nid)
	l := gmReplies(t, gs)
	want := fmt.Sprintf("Its companions beside it: Wrath of the Mother as row %d, Grace of the Mother as row %d", nid+1, nid+2)
	if !hasLine(l, fmt.Sprintf("Placed Tunare as row %d (a copy of row 150)", nid)) || !hasLine(l, want) {
		t.Fatalf("/place Tunare: %q", l)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM npcs`); n != before+3 {
		t.Fatalf("%d rows made, want 3", n-before)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM npcstands WHERE npc_row IN (?, ?, ?)`, nid, nid+1, nid+2); n != 3 {
		t.Fatalf("%d of the three rows stand", n)
	}
	wrath := npcRowMap(t, db, nid+1)
	if wrath["npc_name"] != "Wrath of the Mother" || math.Abs(wrath["x_coord"].(float64)-(25600-4.71)) > 0.01 ||
		math.Abs(wrath["z_coord"].(float64)-(15600-5.98)) > 0.01 {
		t.Fatalf("Wrath's row: %v", wrath)
	}
	for _, p := range []int64{nid + 1, nid + 2} {
		if m, ok := c.gw.CompanionMaster(entity.EntityID(p)); !ok || int64(m) != nid {
			t.Fatalf("row %d's master %d %v, want %d", p, m, ok, nid)
		}
	}

	// /clone of the placed Tunare brings her pets too
	targetAs(gp, entity.EntityID(nid), 100300, gs)
	stand(c, gp, 25700, 52, 15700, 0)
	c.say(gs, "/clone")
	var cl int64
	db.QueryRow(`SELECT MIN(id) FROM npcs WHERE id > ?`, nid+2).Scan(&cl)
	if l := gmReplies(t, gs); !hasLine(l, fmt.Sprintf("Cloned Tunare as row %d", cl)) || !hasLine(l, "Its companions beside it: Wrath of the Mother") {
		t.Fatalf("/clone: %q", l)
	}
	if m, ok := c.gw.CompanionMaster(entity.EntityID(cl + 1)); !ok || int64(m) != cl {
		t.Fatalf("the clone's Wrath's master %d %v", m, ok)
	}

	// #spawn: copies, told so
	c.say(gs, "#spawn Tunare")
	if l := gmReplies(t, gs); !hasLine(l, "Spawned Tunare with its 2 companion(s) beside it.") {
		t.Fatalf("#spawn: %q", l)
	}

	// the control: a creature with none
	before = count(t, db, `SELECT COUNT(*) FROM npcs`)
	c.say(gs, "/place wolf")
	if l := gmReplies(t, gs); hasLine(l, "companions") {
		t.Fatalf("/place wolf: %q", l)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM npcs`); n != before+1 {
		t.Fatalf("/place wolf made %d rows", n-before)
	}
}
