package handlers

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// /markportal (gmportal.go, petition 1624): a GM's target is a portal's entry,
// /markportal exit its landing (the target, else where the GM stands, never
// both), kept as a gmportals row OFF with a ticket filed for building; a row
// switched on makes its entry NPC ask Yes or No and Yes travels there.

// The NPC rows of a portal world, world 0, near the GM Bob at (110, 110).
const (
	portalEntryRow = 201 // the entry
	portalCoachRow = 202 // a coachman, a landing by target
	portalOtherRow = 203 // an NPC with no portal
)

// portalWorld is bugWorld (Ann a player, account 2; Bob a GM, account 1) on a
// database holding the rows above, loaded, with /markportal on it.
func portalWorld(t *testing.T) (*chatWorld, *database.SQLiteDB, *session.Session, *session.Session, *entity.Player) {
	t.Helper()
	c, h, db, as, bs := bugWorld(t)
	h.limit.off = true
	for _, r := range []struct {
		id      int
		name    string
		x, y, z float64
		facing  int
	}{
		{portalEntryRow, "Mysterious Portal", 120, 50, 120, 64},
		{portalCoachRow, "Coachman Test", 500, 52, 500, 0},
		{portalOtherRow, "a moss snake", 130, 50, 115, 0},
	} {
		if _, err := db.Exec(`INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race, zone)
			VALUES (?, ?, 0, ?, ?, ?, ?, 4660, 1.0, 1, 0, '0', 7)`, r.id, r.name, r.x, r.y, r.z, r.facing); err != nil {
			t.Fatal(err)
		}
	}
	c.gw.LoadNPCs(database.NewNPCStore(db))
	c.gw.SetAggro(false)
	c.h.SetCampStore(db)
	c.h.portals = NewGMPortals(db)
	return c, db, as, bs, c.gw.SessionPlayer(bs)
}

// targetNPC makes the NPC of row the GM's target, on its channel slot.
func targetNPC(c *chatWorld, s *session.Session, p *entity.Player, row entity.EntityID) {
	vs := entity.GetOrCreateVisibility(s)
	slot := int(row - 200)
	vs.ChannelEntity[slot], vs.ChannelObjectID[slot] = row, uint32(100000+row)
	p.TargetObjectID = uint32(100000 + row)
}

func TestMarkPortalFilesATicketAndKeepsItOff(t *testing.T) {
	c, db, as, bs, bob := portalWorld(t)

	// a player's: an unknown command, nothing kept (the control)
	c.say(as, "/markportal")
	if l := gmReplies(t, as); !hasLine(l, `Unrecognized command "/markportal"`) {
		t.Fatalf("a player's /markportal: %q", l)
	}
	// no target: the usage
	bob.TargetObjectID = 0xFFFFFFFF
	c.say(bs, "/markportal")
	if l := gmReplies(t, bs); !hasLine(l, "/markportal: target the NPC that is the portal's entry first") {
		t.Fatalf("no target: %q", l)
	}
	// exit before an entry
	c.say(bs, "/markportal exit")
	if l := gmReplies(t, bs); !hasLine(l, "/markportal exit: mark the entry first") {
		t.Fatalf("exit with no entry: %q", l)
	}

	targetNPC(c, bs, bob, portalEntryRow)
	c.say(bs, "#markportal")
	if l := gmReplies(t, bs); !hasLine(l, "Portal entry: Mysterious Portal (npc 201) at (120.0, 50.0, 120.0) in Tunaria") {
		t.Fatalf("entry: %q", l)
	}
	// the entry itself is no landing
	c.say(bs, "/markportal exit")
	if l := gmReplies(t, bs); !hasLine(l, "your target is the entry, Mysterious Portal") {
		t.Fatalf("exit at the entry: %q", l)
	}

	// where the GM stands: no target
	bob.TargetObjectID = 0xFFFFFFFF
	stand(c, bob, 300, 60.5, 400, 1.5)
	c.say(bs, "/markportal exit the Old Ruins")
	l := gmReplies(t, bs)
	if !hasLine(l, "Portal 1: Mysterious Portal to the Old Ruins, (300.0, 60.5, 400.0) in Tunaria, by where you stand.") ||
		!hasLine(l, "Filed as Ticket #1001 for building. It is off until then: /markportal on 1 opens it now.") {
		t.Fatalf("exit: %q", l)
	}
	p, err := db.GMPortalByID(1)
	if err != nil || p.Enabled || p.NPCID != portalEntryRow || p.Place != "the Old Ruins" || p.World != 0 || p.X != 300 ||
		p.Y != 60.5 || p.Z != 400 || p.Facing != 1.5 || p.ExitBy != "stand" || p.ExitName != "" || p.Petition != 1001 ||
		p.CharName != "Bob" || p.AccountID != 1 {
		t.Fatalf("the row: %+v %v", p, err)
	}
	if n := auditLines(t, db, "gmportals", "insert"); n != 1 {
		t.Fatalf("%d inserts audited", n)
	}
	pet, ok, err := db.PetitionByID(1001)
	if err != nil || !ok || pet.Category != portalCategory || pet.Subcategory != portalSubcategory || pet.CharName != "Bob" {
		t.Fatalf("ticket: %+v %v %v", pet, ok, err)
	}
	for _, want := range []string{
		"Internal / Portal",
		"ENTRY: Mysterious Portal (npc 201), Tunaria (world 0) at (120.00, 50.00, 120.00), facing 1.5708 rad (byte 64), model 4660",
		"It says nothing else yet",
		"Near it: a moss snake (npc 203,",
		"EXIT (where the GM stood, facing as it faced):",
		"Tunaria (world 0) at (300.00, 60.50, 400.00), facing 1.5000 rad",
		`ASKS: "Do you wish to travel to the Old Ruins?" Yes / No`,
		"BUILD: /markportal on 1",
		`"Mysterious Portal": {{To: "the Old Ruins", At: &wayPlace{World: 0, X: 300, Y: 60.5, Z: 400, Facing: 1.5}}},`,
	} {
		if !strings.Contains(pet.Text, want) {
			t.Fatalf("the ticket lacks %q:\n%s", want, pet.Text)
		}
	}
	v := views(t, db)[1001]
	if v.Via != "markportal" || v.Portal == nil || v.Portal.ID != 1 || v.Portal.Entry.NPCID != portalEntryRow ||
		v.Portal.Exit.By != "stand" || v.Portal.Exit.X != 300 || v.Character.Name != "Bob" {
		t.Fatalf("the view: %+v", v.Portal)
	}
	// the entry was used: another exit wants another entry
	c.say(bs, "/markportal exit")
	if l := gmReplies(t, bs); !hasLine(l, "mark the entry first") {
		t.Fatalf("a second exit: %q", l)
	}

	// by the target, an NPC: in front of it, its name the place
	targetNPC(c, bs, bob, portalEntryRow)
	c.say(bs, "/markportal")
	gmReplies(t, bs)
	targetNPC(c, bs, bob, portalCoachRow)
	c.say(bs, "/markportal exit")
	if l := gmReplies(t, bs); !hasLine(l, "Portal 2: Mysterious Portal to Coachman Test, (500.0, 52.0, 503.0) in Tunaria, by your target, Coachman Test.") {
		t.Fatalf("exit by target: %q", l)
	}
	p, _ = db.GMPortalByID(2)
	if p.ExitBy != "target" || p.ExitName != "Coachman Test" || p.ExitNPCID != portalCoachRow || p.Enabled || p.Petition != 1002 {
		t.Fatalf("row 2: %+v", p)
	}
	pet, _, _ = db.PetitionByID(1002)
	if !strings.Contains(pet.Text, "EXIT (by the GM's target, the NPC Coachman Test, npc 202, standing at (500.00, 52.00, 500.00)") {
		t.Fatalf("ticket 2:\n%s", pet.Text)
	}

	// switched: no id is the GM's last; an id; a missing one
	c.say(bs, "/markportal on")
	if l := gmReplies(t, bs); !hasLine(l, `Portal 2 is now on: Mysterious Portal (npc 201) asks "Do you wish to travel to Coachman Test?"`) {
		t.Fatalf("on: %q", l)
	}
	c.say(bs, "/markportal on 2")
	if l := gmReplies(t, bs); !hasLine(l, "Portal 2 is already on") {
		t.Fatalf("on again: %q", l)
	}
	c.say(bs, "/markportal off 2")
	if l := gmReplies(t, bs); !hasLine(l, "Portal 2 is now off") {
		t.Fatalf("off: %q", l)
	}
	c.say(bs, "/markportal on 9")
	if l := gmReplies(t, bs); !hasLine(l, "there is no portal 9") {
		t.Fatalf("on 9: %q", l)
	}
	if n := auditLines(t, db, "gmportals", "update"); n != 4 { // two tickets kept, on, off
		t.Fatalf("%d updates audited", n)
	}
	c.say(bs, "/markportal list")
	if l := gmReplies(t, bs); !hasLine(l, "Portal 2 (off): Mysterious Portal (npc 201) to Coachman Test") ||
		!hasLine(l, "Portal 1 (off): Mysterious Portal (npc 201) to the Old Ruins") || !hasLine(l, "in Tunaria, ticket #1001") {
		t.Fatalf("list: %q", l)
	}
	// the entry already having a portal on is said at the next mark
	c.say(bs, "/markportal on 1")
	gmReplies(t, bs)
	targetNPC(c, bs, bob, portalEntryRow)
	c.say(bs, "/markportal")
	if l := gmReplies(t, bs); !hasLine(l, "It already has: portal 1 to the Old Ruins, on") {
		t.Fatalf("an entry with a portal: %q", l)
	}
	c.say(bs, "/markportal cancel")
	if l := gmReplies(t, bs); !hasLine(l, "the entry Mysterious Portal (npc 201) is forgotten") {
		t.Fatalf("cancel: %q", l)
	}
	if pets := views(t, db); len(pets) != 2 {
		t.Fatalf("%d tickets", len(pets))
	}
	var raw string
	db.QueryRow(`SELECT server_view FROM petitions WHERE id = 1002`).Scan(&raw)
	var keys map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &keys); err != nil || keys["portal"] == nil {
		t.Fatalf("no portal key: %v", err)
	}
}

// questionBox is the question of the option box queued for s and its answers.
func questionBox(t *testing.T, s *session.Session) (string, []string) {
	t.Helper()
	for _, m := range s.Queue().GetPending() {
		d := m.Data()
		if len(d) < 6 || protocol.GameOpcode(binary.LittleEndian.Uint16(d)) != protocol.OpcodeQuestPopupMulti {
			continue
		}
		p := 6
		str := func() string {
			n := int(binary.LittleEndian.Uint32(d[p:]))
			u := make([]uint16, n)
			for i := range u {
				u[i] = binary.LittleEndian.Uint16(d[p+4+2*i:])
			}
			p += 4 + 2*n
			return string(utf16.Decode(u))
		}
		q := str()
		n := int(d[p])
		p++
		var out []string
		for range n {
			out = append(out, str())
		}
		return q, out
	}
	return "", nil
}

// A portal on asks its entry's talker Yes or No, and Yes is the trip; No and a
// portal off send no one (the controls); a world the client cannot load is
// not offered; a portal switched on by hand on the box is asked within
// gmPortalsReread; a waysender's own trips are listed with the portal's.
func TestAPortalOnAsksAndSends(t *testing.T) {
	db, err := database.NewSQLite(filepath.Join(t.TempDir(), "portals.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	tw := newTalkWorld(t, 25010, 15010)
	g := NewGMPortals(db)
	clock := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	g.now = func() time.Time { return clock }
	tw.h.portals = g
	who := database.GMEdit{AccountID: 1, Via: "test"}
	add := func(p database.GMPortal) database.GMPortal {
		t.Helper()
		p, err := db.AddGMPortal(p, who, "test")
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	ruins := add(database.GMPortal{NPCID: 201, NPCName: "Mysterious Portal", Place: "the Old Ruins", World: 1, X: 300, Y: 60.5, Z: 400, Facing: 1.5, Enabled: true})
	portal := tw.add(1, &entity.NPC{ServerID: 201, Name: "Mysterious Portal", SpawnX: 25012, SpawnY: 54, SpawnZ: 15012})
	other := tw.add(2, &entity.NPC{ServerID: 203, Name: "a moss snake", SpawnX: 25013, SpawnY: 54, SpawnZ: 15013})

	tw.talk(portal)
	if q, a := questionBox(t, tw.s); q != "Do you wish to travel to the Old Ruins?" || !slices.Equal(a, []string{"Yes", "No"}) {
		t.Fatalf("the portal asked %q %q", q, a)
	}
	tw.pick(portal, 1) // No
	if _, _, _, moved := teleported(tw.s); moved {
		t.Fatal("No sent the player")
	}
	tw.talk(portal)
	tw.s.Queue().GetPending()
	tw.pick(portal, 0)
	if x, z, w, moved := teleported(tw.s); !moved || x != 300 || z != 400 || w != 1 || tw.p.World != 1 {
		t.Fatalf("Yes: (%.1f, %.1f) world %d moved %v", x, z, w, moved)
	}
	// back where it talked, the channels the world change cleared filled again
	tw.p.World, tw.p.X, tw.p.Z = 0, 25010, 15010
	portal = tw.add(1, tw.w.NPCs[201])
	other = tw.add(2, tw.w.NPCs[203])

	// an NPC with no portal: no box (the control)
	tw.talk(other)
	if q, _ := questionBox(t, tw.s); q != "" {
		t.Fatalf("the snake asked %q", q)
	}

	// switched off by hand on the box: still asked until the reread, then not
	if _, _, err := db.SetGMPortalEnabled(ruins.ID, false, who, "test"); err != nil {
		t.Fatal(err)
	}
	tw.talk(portal)
	if q, _ := questionBox(t, tw.s); q == "" {
		t.Fatal("not asked before the reread")
	}
	clock = clock.Add(gmPortalsReread)
	tw.talk(portal)
	if q, _ := questionBox(t, tw.s); q != "" {
		t.Fatalf("a portal off asked %q", q)
	}

	// a world the client cannot load (world 13: a vanilla client holds the
	// disc's six): not offered
	add(database.GMPortal{NPCID: 201, NPCName: "Mysterious Portal", Place: "a home", World: 13, X: 1, Y: 2, Z: 3, Enabled: true})
	g.forget()
	tw.talk(portal)
	if q, _ := questionBox(t, tw.s); q != "" {
		t.Fatalf("a world 13 portal asked %q", q)
	}

	// a waysender with trips of its own lists the portal's place with them
	add(database.GMPortal{NPCID: 695, NPCName: "Waysender Abodai", Place: "the Old Ruins", World: 0, X: 300, Y: 60.5, Z: 400, Enabled: true})
	g.forget()
	abodai := tw.add(3, &entity.NPC{ServerID: 695, Name: "Waysender Abodai", SpawnX: 25011, SpawnY: 54, SpawnZ: 15011})
	tw.w.NPCs[2616] = &entity.NPC{ServerID: 2616, Name: "Waysender Ti`Thox", SpawnX: 25027.5, SpawnY: 0.6, SpawnZ: 8913.3}
	tw.talk(abodai)
	if got := optionBox(t, tw.s); !slices.Equal(got, []string{"Neriak (100 tunar)", "the Old Ruins"}) {
		t.Fatalf("Abodai offered %q", got)
	}
	tw.pick(abodai, 1)
	if x, z, _, moved := teleported(tw.s); !moved || math.Abs(float64(x)-300) > 0.01 || math.Abs(float64(z)-400) > 0.01 {
		t.Fatalf("the portal's place from the list: (%.1f, %.1f) %v", x, z, moved)
	}
}
