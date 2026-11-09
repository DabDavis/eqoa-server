package handlers

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The Rock of Freeport (rockfreeport.go) driven through the handlers a client
// reaches: 0x002A and 0x0014 for the gift, 0x003E for the stone's Use, the
// World's tick for its landing. Wanderer stands in Tunaria far from Dabrish;
// the people, places and the visual are made up.

// rockOnFor turns the switch on for one test.
func rockOnFor(t *testing.T) {
	t.Helper()
	SetRockOfFreeport(true)
	t.Cleanup(func() { SetRockOfFreeport(false) })
}

type rockRig struct {
	t    *testing.T
	db   *database.SQLiteDB
	inv  *database.InventoryStore
	gw   *entity.World
	sess *session.Session
	p    *entity.Player
	sid  uint32
	h    *EquipHandler
	rock int
}

const rockMe = uint32(0x1424A1D0) // the id Wanderer's client knows itself by

func newRockRig(t *testing.T) *rockRig {
	t.Helper()
	db, err := database.NewSQLite(t.TempDir() + "/rock.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := database.NewSQLiteCharacterStore(db)
	rec := &database.CharacterRecord{CharName: "Wanderer", AccountID: 1}
	if err := store.Create(rec); err != nil {
		t.Fatal(err)
	}
	r := &rockRig{t: t, db: db, inv: database.NewInventoryStore(db), gw: entity.NewWorld(), sid: rec.ServerID}
	sm := session.NewManager()
	r.gw.SessionManager = sm
	r.sess = sm.CreateSession(&net.UDPAddr{IP: net.IPv4(203, 0, 113, 77), Port: 5077}, 77, 77, 77, 1, true)
	r.p = r.gw.CreatePlayer(r.sess, &database.CharacterRecord{ServerID: rec.ServerID, CharName: "Wanderer", X: 9000, Y: 50, Z: 9000})
	r.p.HP, r.p.MaxHP = 100, 100
	r.sess.SetCharacterInWorld(true)
	r.sess.SetPlayerObjectID(rockMe)
	spells := database.NewSpellbookStore(db)
	ch := NewCharacterHandler(store, sm, r.inv, spells)
	ch.gameWorld = r.gw
	r.gw.OnTravel(ch.travelPlayer) // as RegisterDefaultHandlers sets it
	r.gw.SetLoadableWorlds(LoadableWorlds)
	r.h = NewEquipHandler(sm, r.inv, spells, ch)
	r.h.SetGameWorld(r.gw)
	if r.rock, _, err = EnsureRockOfFreeport(db); err != nil {
		t.Fatal(err)
	}
	return r
}

// dabrish stands Dabrish at (5100, 50, 5200) facing a quarter turn (+x).
func (r *rockRig) dabrish() {
	r.gw.NPCs[entity.EntityID(77001)] = &entity.NPC{ServerID: 77001, Name: "Dabrish", Level: 60, World: 0, X: 5100, Y: 50,
		Z: 5200, Facing: math.Pi / 2, HP: 100, MaxHP: 100}
}

// key is the position of a record in the client's list once the list is
// loaded (as SendMemoryDump leaves it).
func (r *rockRig) key(pattern int) uint32 {
	r.t.Helper()
	rows, err := r.inv.GetCharacterItems(r.sid)
	if err != nil {
		r.t.Fatal(err)
	}
	r.sess.SetInventoryList(newInvList(rows))
	for i := range rows {
		if rows[i].Pattern.PatternID == pattern {
			return uint32(i)
		}
	}
	r.t.Fatalf("no item %d", pattern)
	return 0
}

// use is the item menu's Use (interaction 0) or a quick slot's (1), and what
// the session was sent.
func (r *rockRig) use(key uint32, interaction byte) [][]byte {
	r.t.Helper()
	payload := build(protocol.OpcodeInteractItem, 16, func(w *buffer.Writer) {
		w.WriteUint32(key)
		w.WriteUint32(0xFFFFFFFF)
		w.WriteByte(interaction)
	})[2:]
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeInteractItem)
	msg.SetData(payload)
	if err := r.h.HandleInteractItem(r.sess, msg); err != nil {
		r.t.Fatal(err)
	}
	return r.sent()
}

func (r *rockRig) sent() [][]byte {
	var out [][]byte
	for _, m := range r.sess.Queue().GetPending() {
		out = append(out, append([]byte(nil), m.Data()...))
	}
	return out
}

// castOf is the 0x00B1 among msgs, nil without one.
func castOf(msgs [][]byte) []byte {
	for _, m := range msgs {
		if len(m) >= 2 && opOf(m) == protocol.OpcodeCastingCombat {
			return m
		}
	}
	return nil
}

// wantCast is the stone's 0x00B1: Wanderer at itself, 2000 ms, the visual.
func wantCast(fx uint32) []byte {
	b := []byte{0xB1, 0x00}
	b = binary.LittleEndian.AppendUint32(b, rockMe)
	b = binary.LittleEndian.AppendUint32(b, rockMe)
	b = append(b, 0)
	b = binary.LittleEndian.AppendUint32(b, 2000)
	return binary.LittleEndian.AppendUint32(b, fx)
}

// textOf is the text of the first message of op among msgs, "" without one.
func textOf(msgs [][]byte, op protocol.GameOpcode) string {
	for _, m := range msgs {
		if len(m) < 2 || opOf(m) != op {
			continue
		}
		rd := buffer.NewReader(m[2:])
		n, _ := rd.ReadUint32()
		var sb strings.Builder
		for i := uint32(0); i < n; i++ {
			u, _ := rd.ReadUint16()
			sb.WriteRune(rune(u))
		}
		return sb.String()
	}
	return ""
}

// The whole of it: the stone's Use from the item menu is a 2 s cast in Return
// Home's visual (its spelleffect row's) with its line, no lockout; nothing moves
// until it lands (its cast time and castLand, as every cast), then Wanderer
// stands where the Summoning Portal's way home lands, 3 in front of Dabrish by
// his facing (freeportSpot), and still holds the stone. A quick slot's press
// casts it the same. The controls: another gem casts nothing; with no Dabrish
// the stone says so in red and casts nothing; without Return Home's row the
// visual is ReturnHomeVisual.
func TestTheRockOfFreeportCastsYouToDabrish(t *testing.T) {
	rockOnFor(t)
	r := newRockRig(t)
	r.dabrish()
	if _, err := r.db.Exec(`INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES (?, 'Self, Return to bind point', ?, 'test')`,
		database.ReturnHomeSpell, int64(0x0BADF00D)); err != nil {
		t.Fatal(err)
	}
	gem, err := r.db.Exec(`INSERT INTO itempattern (itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (0, 0, 0, 0, -1, 0, 1, 1, 0, 0, 0, 0, 1, 1, 0, 80, 32767, 32767, 7, 0, 0, 0, 'Spitestone', '', 0, 0)`)
	if err != nil {
		t.Fatal(err)
	}
	spite, _ := gem.LastInsertId()
	for _, id := range []int{int(spite), r.rock} {
		if _, err := r.inv.AddCharItem(r.sid, id, 1, 0, 0, 0); err != nil {
			t.Fatal(err)
		}
	}

	// the control: another gem
	if sent := r.use(r.key(int(spite)), 0); castOf(sent) != nil {
		t.Fatalf("Spitestone cast %X", castOf(sent))
	}

	t0 := time.Now()
	sent := r.use(r.key(r.rock), 0)
	t1 := time.Now()
	if c := castOf(sent); !bytes.Equal(c, wantCast(0x0BADF00D)) {
		t.Fatalf("the cast % X, want % X (itself, 2000 ms, Return Home's row's visual); sent %s", c, wantCast(0x0BADF00D), ops(sent))
	}
	if got := textOf(sent, protocol.OpcodeClientMessage); got != rockCasting {
		t.Errorf("the line %q, want %q", got, rockCasting)
	}
	for _, m := range sent {
		if len(m) >= 2 && opOf(m) == protocol.OpcodeCooldownTimer {
			t.Errorf("a 0x00CD % X: the stone is no ability", m)
		}
	}
	// the quick slot pressed again during the cast: nothing, as a cast's
	if again := r.use(r.key(r.rock), 1); len(again) != 0 {
		t.Errorf("a press during the cast sent %s", ops(again))
	}
	want, err := freeportSpot(r.gw)
	if err != nil {
		t.Fatal(err)
	}
	if want.X != 5100+3 || math.Abs(float64(want.Z-5200)) > 1e-3 || want.Y != 50 || want.World != 0 {
		t.Fatalf("freeportSpot %+v, want 3 in front of Dabrish facing +x", want)
	}
	r.gw.Tick(t0.Add(RockCastTime + 1350*time.Millisecond))
	if snap, _ := r.gw.PlayerSnapshot(r.p.ServerID); snap.X != 9000 || snap.Z != 9000 {
		t.Fatalf("moved before the landing, to (%.1f, %.1f)", snap.X, snap.Z)
	}
	r.sent()
	r.gw.Tick(t1.Add(RockCastTime + 1400*time.Millisecond))
	snap, _ := r.gw.PlayerSnapshot(r.p.ServerID)
	if snap.World != want.World || snap.X != want.X || snap.Y != want.Y || snap.Z != want.Z {
		t.Errorf("Wanderer stands at world %d (%.2f, %.2f, %.2f), want %+v", snap.World, snap.X, snap.Y, snap.Z, want)
	}
	if got := r.sent(); !strings.Contains(ops(got), fmt.Sprintf("%04X", uint16(protocol.OpcodeTeleport))) {
		t.Errorf("no teleport (0x00B2) sent at the landing: %s", ops(got))
	}
	rows, _ := r.inv.GetCharacterItems(r.sid)
	held := 0
	for _, row := range rows {
		if row.Pattern.PatternID == r.rock {
			held += row.StackLeft
		}
	}
	if held != 1 {
		t.Errorf("%d stones left, want the one: it is never spent", held)
	}

	// a quick slot's press casts it the same, in ReturnHomeVisual without the row
	if _, err := r.db.Exec(`DELETE FROM spelleffect WHERE spellid = ?`, database.ReturnHomeSpell); err != nil {
		t.Fatal(err)
	}
	if c := castOf(r.use(r.key(r.rock), 1)); !bytes.Equal(c, wantCast(ReturnHomeVisual)) {
		t.Errorf("the quick slot's cast % X, want % X", c, wantCast(ReturnHomeVisual))
	}
	r.gw.Tick(time.Now().Add(RockCastTime + 1400*time.Millisecond))
	r.sent()

	// the control: no Dabrish, no cast
	delete(r.gw.NPCs, entity.EntityID(77001))
	sent = r.use(r.key(r.rock), 0)
	if castOf(sent) != nil || textOf(sent, protocol.OpcodeErrorMessage) != rockNoPlace {
		t.Errorf("with no Dabrish: sent %s (%q)", ops(sent), textOf(sent, protocol.OpcodeErrorMessage))
	}
}

// With the switch off the stone does nothing but say so, and the dead are told
// nothing; the control: the switch on, the same stone casts.
func TestTheRockOfFreeportOffAndDead(t *testing.T) {
	r := newRockRig(t)
	r.dabrish()
	if _, err := r.inv.AddCharItem(r.sid, r.rock, 1, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	SetRockOfFreeport(false)
	sent := r.use(r.key(r.rock), 0)
	if castOf(sent) != nil || textOf(sent, protocol.OpcodeErrorMessage) != rockQuiet {
		t.Errorf("switched off: sent %s (%q)", ops(sent), textOf(sent, protocol.OpcodeErrorMessage))
	}
	rockOnFor(t)
	r.p.HP = 0
	if sent := r.use(r.key(r.rock), 0); len(sent) != 0 {
		t.Errorf("the dead's Use sent %s", ops(sent))
	}
	r.p.HP = 100
	if sent := r.use(r.key(r.rock), 0); castOf(sent) == nil {
		t.Errorf("the control, switched on: sent %s", ops(sent))
	}
	if IsRockOfFreeport(&database.ItemPatternRecord{EquipSlot: 18, ItemName: RockOfFreeportName}) ||
		!IsRockOfFreeport(&database.ItemPatternRecord{EquipSlot: -1, ItemName: " rock of freeport"}) {
		t.Error("IsRockOfFreeport")
	}
}

// The record as the server makes it (EnsureRockOfFreeport): the gem type, NO
// TRADE, LORE, worth 0, a stack of one, every class and race, the travel
// stones' icon, at 715050 on a database without one; the kept items' Use and
// quick slot rules read it so (IsRockOfFreeport, the gem quick slot).
func TestTheRockOfFreeportRecord(t *testing.T) {
	r := newRockRig(t)
	if r.rock != rockRecordID {
		t.Errorf("made at %d, want %d", r.rock, rockRecordID)
	}
	p, err := database.NewItemPatternStore(r.db).GetItemPattern(r.rock)
	if err != nil {
		t.Fatal(err)
	}
	if !IsRockOfFreeport(p) || p.ProcAnim != 7 || p.Trade != 1 || p.Lore != 1 || p.ItemCost != 0 || p.MaxStack != 1 ||
		p.ClassUse != 32767 || p.RaceUse != 32767 || p.ItemIcon != 414194432 || p.LevelReq != 1 || p.Rent != 1 {
		t.Errorf("the record %+v", p)
	}
	if _, err := r.inv.AddCharItem(r.sid, r.rock, 1, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	rows, _ := r.inv.GetCharacterItems(r.sid)
	if !quickSlotFits(&rows[r.key(r.rock)], 2) {
		t.Error("the stone is not on the gem quick slot")
	}
}

// The gift through a login (0x002A, then 0x0014): a character holding none is
// given one in its pack, never a second at the next login, and none while one
// is in its bank; a full pack gives none and is told once in the world (a
// yellow line), and the login after room is made gives it. The controls: the
// switch off gives nothing and tells nothing, and so does no record.
func TestALoginGivesTheRockOfFreeport(t *testing.T) {
	a := &database.CharacterRecord{CharName: "Holder", AccountID: 1, Level: 10}
	b := &database.CharacterRecord{CharName: "Packrat", AccountID: 2, Level: 10}
	r := newTakeoverRig(t, a, b)
	inv := database.NewInventoryStore(r.db)
	r.h.inventoryStore = inv
	r.w.OnPlayerEntered(tellRockOwed) // as RegisterDefaultHandlers sets it
	count := func(id uint32, location int) int {
		var n int
		if err := r.db.QueryRow(`SELECT COUNT(*) FROM charinventory c JOIN itempattern p ON p.patternid = c.patternid
			WHERE c.serverid = ? AND p.itemname = ? AND c.location = ?`, id, RockOfFreeportName, location).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	login := func(account int, id uint32) string {
		t.Helper()
		s := r.window(account)
		r.enter(s, id)
		var line string
		for _, m := range s.Queue().GetPending() {
			if d := m.Data(); len(d) >= 2 && opOf(d) == protocol.OpcodeColoredChat && strings.Contains(textOf([][]byte{d}, protocol.OpcodeColoredChat), "Rock of Freeport") {
				line = textOf([][]byte{d}, protocol.OpcodeColoredChat)
			}
		}
		r.w.RemovePlayerBySession(s) // logged out
		return line
	}

	// the controls: no record, then the switch off
	rockOnFor(t)
	login(1, a.ServerID)
	if count(a.ServerID, 0) != 0 {
		t.Fatal("a stone with no record")
	}
	if _, _, err := EnsureRockOfFreeport(r.db); err != nil {
		t.Fatal(err)
	}
	SetRockOfFreeport(false)
	login(1, a.ServerID)
	if count(a.ServerID, 0) != 0 {
		t.Fatal("a stone with the switch off")
	}
	SetRockOfFreeport(true)

	// given once, kept once
	if line := login(1, a.ServerID); line != "" || count(a.ServerID, 0) != 1 {
		t.Fatalf("the first login: %d stones (line %q), want one, said nothing", count(a.ServerID, 0), line)
	}
	login(1, a.ServerID)
	if count(a.ServerID, 0) != 1 {
		t.Fatalf("the second login: %d stones, want one", count(a.ServerID, 0))
	}
	if _, err := r.db.Exec(`UPDATE charinventory SET location = 1 WHERE serverid = ?`, a.ServerID); err != nil {
		t.Fatal(err)
	}
	login(1, a.ServerID)
	if count(a.ServerID, 0) != 0 || count(a.ServerID, 1) != 1 {
		t.Errorf("with one in the bank: %d in the pack, %d banked, want none given", count(a.ServerID, 0), count(a.ServerID, 1))
	}

	// a full pack: none, told once; room made, given
	filler, err := r.db.Exec(`INSERT INTO itempattern (itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (1, 0, 0, 0, -1, 0, 0, 1, 0, 0, 0, 0, 1, 1, 0, 0, 32767, 32767, 0, 0, 0, 0, 'Pebble', '', 0, 0)`)
	if err != nil {
		t.Fatal(err)
	}
	pebble, _ := filler.LastInsertId()
	for i := 0; i < invListCap; i++ {
		if _, err := inv.AddCharItem(b.ServerID, int(pebble), 1, 0, 0, 0); err != nil {
			t.Fatal(err)
		}
	}
	if line := login(2, b.ServerID); line != rockOwedLine || count(b.ServerID, 0) != 0 {
		t.Fatalf("a full pack: %d stones, told %q, want none and %q", count(b.ServerID, 0), line, rockOwedLine)
	}
	SetRockOfFreeport(false) // a login that gives nothing tells nothing either
	if line := login(2, b.ServerID); line != "" {
		t.Errorf("told again at a login with the switch off: %q", line)
	}
	SetRockOfFreeport(true)
	if _, err := r.db.Exec(`DELETE FROM charinventory WHERE itemid = (SELECT MAX(itemid) FROM charinventory WHERE serverid = ?)`, b.ServerID); err != nil {
		t.Fatal(err)
	}
	if line := login(2, b.ServerID); line != "" || count(b.ServerID, 0) != 1 {
		t.Errorf("room made: %d stones, told %q, want one and nothing said", count(b.ServerID, 0), line)
	}
}
