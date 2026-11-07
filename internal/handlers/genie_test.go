package handlers

import (
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

// The genie against a temporary database and a real world: a warrior with a
// sword, two Heirloom Outfitters (one selling the warriors' and the monks'
// heirlooms, one the warriors' alone), the window handler and the genie.
type genieFixture struct {
	t        *testing.T
	db       *database.SQLiteDB
	inv      *database.InventoryStore
	w        *entity.World
	sess     *session.Session
	p        *entity.Player
	windows  *NPCWindowHandler
	g        *Genie
	now      time.Time
	sword    int
	warHeirl int
	mnkHeirl int
}

func newGenieFixture(t *testing.T) *genieFixture {
	t.Helper()
	db, err := database.NewSQLite(t.TempDir() + "/genie.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	rec := &database.CharacterRecord{CharName: "Master", AccountID: 1, Level: 30}
	if err := database.NewSQLiteCharacterStore(db).Create(rec); err != nil {
		t.Fatal(err)
	}
	f := &genieFixture{t: t, db: db, inv: database.NewInventoryStore(db), now: time.Unix(1_800_000_000, 0)}
	pattern := func(name string, value, slot int) int {
		t.Helper()
		res, err := db.Exec(`INSERT INTO itempattern
			(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
			 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
			 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
			VALUES (?, 0, 0, 0, ?, 0, 0, 1, 0, 1, 50, 0, 1, 1, 100, 50, 32767, 32767, 0, 0, 0, 0, ?, '', 0, 0)`,
			value, slot, name)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return int(id)
	}
	f.sword = pattern("Rusty Sword", 100, 12)
	f.warHeirl = pattern("Heirloom Blade", 0, 14)
	f.mnkHeirl = pattern("Heirloom Claws", 0, 14)
	for _, q := range []string{
		`INSERT INTO heirlooms (patternid, class, role) VALUES (?, 0, 'main')`,
		`INSERT INTO heirlooms (patternid, class, role) VALUES (?, 4, 'main')`,
	} {
		id := f.warHeirl
		if strings.Contains(q, ", 4,") {
			id = f.mnkHeirl
		}
		if _, err := db.Exec(q, id); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range []struct {
		npcid int
		items []int
	}{{5001, []int{f.warHeirl, f.mnkHeirl}}, {5002, []int{f.warHeirl}}} {
		if _, err := db.Exec(`INSERT INTO npcs (npc_name, npc_id) VALUES ('Heirloom Outfitter', ?)`, s.npcid); err != nil {
			t.Fatal(err)
		}
		for i, pid := range s.items {
			if _, err := db.Exec(`INSERT INTO npcitem (npcid, stackleft, remainHP, patternid, listnumber) VALUES (?, 1, 100, ?, ?)`,
				s.npcid, pid, i); err != nil {
				t.Fatal(err)
			}
		}
	}
	f.w = entity.NewWorld()
	mgr := session.NewManager()
	f.w.SessionManager = mgr
	f.sess = mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 9), Port: 7019}, 9, 1, 1, 2, true)
	f.p = &entity.Player{ServerID: 1019, CharServerID: rec.ServerID, Name: "Master", X: 100, Y: 54, Z: 100, Level: 30,
		HP: 100, MaxHP: 100, Tunar: 1000}
	f.w.Players[f.p.ServerID] = f.p
	f.sess.SetECSEntity(f.p)
	f.sess.SetCharacterInWorld(true)
	f.windows = NewNPCWindowHandler(mgr, f.inv)
	f.windows.SetGameWorld(f.w)
	f.g = NewGenie(f.w, db, f.windows)
	f.g.now = func() time.Time { return f.now }
	if _, err := f.inv.AddCharItem(rec.ServerID, f.sword, 1, 40, 0, 0); err != nil {
		t.Fatal(err)
	}
	f.loadLists()
	return f
}

func (f *genieFixture) loadLists() {
	f.t.Helper()
	rows, err := f.inv.GetCharacterItems(f.p.CharServerID)
	if err != nil {
		f.t.Fatal(err)
	}
	var inv, bank []database.CharInventoryEntry
	for _, r := range rows {
		if r.Location == 1 {
			bank = append(bank, r)
		} else {
			inv = append(inv, r)
		}
	}
	f.sess.SetInventoryList(newInvList(inv))
	f.sess.SetBankList(newInvList(bank))
}

// summon rubs the lamp and returns the genie's box.
func (f *genieFixture) summon() uint32 {
	f.t.Helper()
	f.sess.Queue().GetPending()
	f.g.Summon(f.sess, f.p)
	msgs := questMsgs(f.sess)
	id := boxID(msgs)
	if !IsGenieBox(id) || !hasMsg(msgs, "box:Genie: Your wish is my command, master.") ||
		!strings.Contains(strings.Join(msgs, "\n"), ":What do you wish?|My bank|Your wares|Mend my gear|Rehabilitation|Nothing, thank you") {
		f.t.Fatalf("the genie's coming: %q", msgs)
	}
	return id
}

func (f *genieFixture) genieNPC() *entity.NPC {
	for _, n := range f.w.NPCs {
		if n.GenieOf == f.p.ServerID {
			return n
		}
	}
	return nil
}

// send feeds a window request as the client sends it.
func (f *genieFixture) send(op protocol.GameOpcode, fill func(w *buffer.Writer)) [][]byte {
	f.t.Helper()
	f.sess.Queue().GetPending()
	payload := build(op, 256, fill)[2:]
	msg := protocol.NewMessage(protocol.MessageTypeReliable, op)
	msg.SetData(payload)
	r := NewRegistry()
	f.windows.RegisterHandlers(r)
	h, ok := r.Get(op)
	if !ok {
		f.t.Fatalf("no handler for 0x%04X", uint16(op))
	}
	if err := h(f.sess, msg); err != nil {
		f.t.Fatal(err)
	}
	var out [][]byte
	for _, m := range f.sess.Queue().GetPending() {
		out = append(out, append([]byte(nil), m.Data()...))
	}
	return out
}

func (f *genieFixture) answer(box uint32, option byte) [][]byte {
	f.t.Helper()
	f.sess.Queue().GetPending()
	if !f.g.Answer(f.sess, f.p, box, option) {
		f.t.Fatalf("box %08X not the genie's", box)
	}
	var out [][]byte
	for _, m := range f.sess.Queue().GetPending() {
		out = append(out, append([]byte(nil), m.Data()...))
	}
	return out
}

func (f *genieFixture) where(pattern int) (location, hp int) {
	f.t.Helper()
	for _, r := range func() []database.CharInventoryEntry {
		rows, _ := f.inv.GetCharacterItems(f.p.CharServerID)
		return rows
	}() {
		if r.Pattern.PatternID == pattern {
			return r.Location, r.RemainHP
		}
	}
	return -1, 0
}

// The genie's three windows, end to end through the window handlers: the
// bank (0x124D; a move carrying CLIENT's target, any id), its wares (0x00B7
// under its window id: the outfitter selling the warriors' alone, bought from
// at its price, 0; another id refused), the forge (0x0061 under its id: a
// repair at a blacksmith's price; the forge's close ends it); each answer
// sends the genie out of the world. The controls: talked to, it asks its
// master again and tells anyone else it is not theirs; a box CLIENT declined
// at once leaves it standing, one the player closed sends it away, as does
// "Nothing, thank you"; a class no outfitter sells to gets no wares.
func TestTheGenie(t *testing.T) {
	f := newGenieFixture(t)

	// the bank
	box := f.summon()
	g := f.genieNPC()
	if g == nil || g.NPCID != 5002 {
		t.Fatalf("the genie: %+v (want the warriors' own outfitter, 5002)", g)
	}
	msgs := f.answer(box, genieBank)
	if len(msgs) == 0 || opOf(msgs[0]) != protocol.OpcodeBankUI || f.genieNPC() != nil {
		t.Fatalf("Bank: sent %s, genie %v", ops(msgs), f.genieNPC())
	}
	sent := f.send(protocol.OpcodeBankItem, func(w *buffer.Writer) {
		w.WriteUint32(0xFFFFFFFF) // CLIENT's target: nothing
		w.WriteByte(0)
		w.WriteUint32(0) // the sword's key
		w.Write7BitEncodedInt64(1)
	})
	if loc, _ := f.where(f.sword); loc != 1 {
		t.Fatalf("the sword into the genie's bank: sent %s, location %d", ops(sent), loc)
	}
	f.loadLists()
	sent = f.send(protocol.OpcodeBankItem, func(w *buffer.Writer) { // and back out
		w.WriteUint32(4242)
		w.WriteByte(1)
		w.WriteUint32(0)
		w.Write7BitEncodedInt64(1)
	})
	if loc, _ := f.where(f.sword); loc != 0 {
		t.Fatalf("the sword out of the genie's bank: sent %s, location %d", ops(sent), loc)
	}
	f.loadLists()

	// its wares
	box = f.summon()
	gid := entity.GenieWindowID(f.genieNPC().ServerID)
	msgs = f.answer(box, genieShop)
	if len(msgs) != 1 || opOf(msgs[0]) != protocol.OpcodeMerchantBox || f.genieNPC() != nil {
		t.Fatalf("Shop: sent %s", ops(msgs))
	}
	r := buffer.NewReader(msgs[0][2:])
	if id, _ := r.ReadUint32(); id != gid {
		t.Fatalf("the shop's merchant id %08X, want %08X", id, gid)
	}
	if !strings.Contains(string(msgs[0]), "H\x00e\x00i\x00r\x00l\x00o\x00o\x00m\x00s\x00 \x00f\x00o\x00r") {
		t.Errorf("the shop's greeting is not the genie's")
	}
	buy := func(id uint32) [][]byte {
		return f.send(protocol.OpcodeMerchantBuy, func(w *buffer.Writer) {
			w.Write7BitEncodedInt64(0)
			w.Write7BitEncodedInt64(1)
			w.WriteUint32(id)
		})
	}
	if sent := buy(gid + 1); len(sent) != 1 || opOf(sent[0]) != protocol.OpcodeCancelPending {
		t.Fatalf("a buy under another id: sent %s", ops(sent))
	}
	if sent := buy(gid); len(sent) == 0 || opOf(sent[0]) != protocol.OpcodeAddInvItem {
		t.Fatalf("a buy from the genie: sent %s", ops(sent))
	}
	if loc, _ := f.where(f.warHeirl); loc != 0 {
		t.Fatalf("the warriors' heirloom not bought: location %d", loc)
	}
	if loc, _ := f.where(f.mnkHeirl); loc != -1 {
		t.Fatal("the monks' heirloom sold to a warrior")
	}
	f.loadLists()

	// the forge
	box = f.summon()
	gid = entity.GenieWindowID(f.genieNPC().ServerID)
	msgs = f.answer(box, genieSmith)
	if len(msgs) != 1 || opOf(msgs[0]) != protocol.OpcodeBlackSmithMenu {
		t.Fatalf("Smith: sent %s", ops(msgs))
	}
	if id, _ := buffer.NewReader(msgs[0][2:]).ReadUint32(); id != gid {
		t.Fatalf("the forge's id %08X, want %08X", id, gid)
	}
	repair := func() [][]byte {
		return f.send(protocol.OpcodeRequestRepair, func(w *buffer.Writer) {
			w.WriteUint32(0) // the sword's key
			w.WriteUint32(gid)
		})
	}
	sent = repair()
	if _, hp := f.where(f.sword); hp != 100 || f.p.Tunar != 1000-repairCost(100, 40, 100) {
		t.Fatalf("the sword mended: sent %s, hp %d, tunar %d", ops(sent), hp, f.p.Tunar)
	}
	f.send(protocol.OpcodeCloseBlacksmithMenu, func(w *buffer.Writer) {})
	if _, ok := f.g.windowNPC(f.sess, gid, npcFlagBlacksmith); ok {
		t.Error("the forge held past its close")
	}

	// talked to: its master asked again, another told
	f.summon()
	f.sess.Queue().GetPending()
	if !f.g.Talk(f.sess, f.p, f.genieNPC()) || boxID(questMsgs(f.sess)) == 0 {
		t.Fatal("its master talking: no question")
	}
	other := f.w.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 10), Port: 7020}, 10, 1, 1, 2, true)
	q := &entity.Player{ServerID: 1020, Name: "Other", HP: 1, MaxHP: 1}
	f.w.Players[q.ServerID] = q
	other.SetECSEntity(q)
	if !f.g.Talk(other, q, f.genieNPC()) || !hasMsg(questMsgs(other), "box:Genie: I answer only to the one who holds my lamp.") {
		t.Fatal("another talking: not told")
	}
	if f.g.Talk(f.sess, f.p, &entity.NPC{ServerID: 3, Name: "Genie"}) {
		t.Error("an NPC named Genie taken for one")
	}

	// a box CLIENT declined at once: the genie stays; one closed later: gone
	box = f.summon()
	f.answer(box, 255)
	if f.genieNPC() == nil {
		t.Fatal("the genie left on a box declined at once")
	}
	f.sess.Queue().GetPending()
	f.g.Talk(f.sess, f.p, f.genieNPC())
	box = boxID(questMsgs(f.sess))
	f.now = f.now.Add(3 * time.Second)
	f.answer(box, 255)
	if f.genieNPC() != nil {
		t.Fatal("the genie stayed on a box closed")
	}
	box = f.summon()
	if msgs := f.answer(box, genieNothing); f.genieNPC() != nil || len(msgs) == 0 {
		t.Fatal("Nothing, thank you: the genie stayed")
	}

	// no outfitter sells a cleric's heirlooms here
	f.p.Class = 9
	f.g.shops = map[int]genieShopAt{}
	box = f.summon()
	f.sess.Queue().GetPending()
	f.g.Answer(f.sess, f.p, box, genieShop)
	if !hasMsg(questMsgs(f.sess), "box:Genie: I have no wares for one of your calling.") || f.genieNPC() != nil {
		t.Fatal("a class with no outfitter: wares offered or the genie kept")
	}
}

// The lamp at the login: a character holding none gets one, in the pack, or
// the bank when the pack is full; never a second; nothing without the record.
func TestALoginGivesTheLamp(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/lamp.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := database.NewSQLiteCharacterStore(db)
	rec := &database.CharacterRecord{CharName: "Holder", AccountID: 1}
	if err := store.Create(rec); err != nil {
		t.Fatal(err)
	}
	inv := database.NewInventoryStore(db)
	h := NewCharacterHandler(store, nil, inv, database.NewSpellbookStore(db))
	count := func(where string) int {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM charinventory c JOIN itempattern p ON p.patternid = c.patternid
			WHERE c.serverid = ? AND p.itemname = ? AND `+where, rec.ServerID, GenieLampName).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	h.giveGenieLamp(rec.ServerID) // no record yet: nothing
	if count("1") != 0 {
		t.Fatal("a lamp with no record")
	}
	res, err := db.Exec(`INSERT INTO itempattern (itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (0, 0, 0, 0, -1, 0, 1, 1, 0, 0, 0, 0, 1, 1, 0, 80, 32767, 32767, 2, 0, 0, 0, ?, '', 0, 0)`, GenieLampName)
	if err != nil {
		t.Fatal(err)
	}
	lamp, _ := res.LastInsertId()
	h.giveGenieLamp(rec.ServerID)
	h.giveGenieLamp(rec.ServerID)
	if count("c.location = 0") != 1 || count("1") != 1 {
		t.Fatalf("lamps: %d in the pack, %d in all; want one", count("c.location = 0"), count("1"))
	}
	if !IsGenieLamp(&database.ItemPatternRecord{EquipSlot: -1, ItemName: "genie's lamp"}) ||
		IsGenieLamp(&database.ItemPatternRecord{EquipSlot: 18, ItemName: GenieLampName}) {
		t.Error("IsGenieLamp")
	}
	// a full pack: the bank
	if _, err := db.Exec(`DELETE FROM charinventory WHERE patternid = ?`, lamp); err != nil {
		t.Fatal(err)
	}
	res, err = db.Exec(`INSERT INTO itempattern (itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (0, 0, 0, 0, -1, 0, 0, 1, 0, 0, 0, 0, 1, 1, 0, 50, 32767, 32767, 0, 0, 0, 0, 'pebble', '', 0, 0)`)
	if err != nil {
		t.Fatal(err)
	}
	pebble, _ := res.LastInsertId()
	for i := 0; i < invListCap; i++ {
		if _, err := inv.AddCharItem(rec.ServerID, int(pebble), 1, 0, 0, 0); err != nil {
			t.Fatal(err)
		}
	}
	h.giveGenieLamp(rec.ServerID)
	if count("c.location = 1") != 1 {
		t.Fatalf("a full pack's lamp: %d in the bank", count("c.location = 1"))
	}
}

// The lamp used from the inventory (0x003E) brings the genie, and stays.
func TestUsingTheLampBringsTheGenie(t *testing.T) {
	f := newScribeFixture(t)
	w := entity.NewWorld()
	w.SessionManager = session.NewManager()
	f.player.HP, f.player.MaxHP = 100, 100
	w.Players[f.player.ServerID] = f.player
	f.sess.SetECSEntity(f.player)
	f.h.genie = NewGenie(w, f.db, nil)
	res, err := f.db.Exec(`INSERT INTO itempattern (itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (0, 0, 0, 0, -1, 0, 1, 1, 0, 0, 0, 0, 1, 1, 0, 80, 32767, 32767, 2, 0, 0, 0, ?, '', 0, 0)`, GenieLampName)
	if err != nil {
		t.Fatal(err)
	}
	lamp, _ := res.LastInsertId()
	f.own(int(lamp), 1)
	f.loadList()
	f.use(0)
	n := 0
	for _, npc := range w.NPCs {
		if npc.GenieOf == f.player.ServerID {
			n++
		}
	}
	if n != 1 || f.stackOf(GenieLampName) != 1 {
		t.Fatalf("the lamp used: %d genies, %d lamps left", n, f.stackOf(GenieLampName))
	}
}

// The genie offers a rehabilitator's menu (the user, 2026-11-07): the answer sends it away and asks what to undo under
// the rehabilitators' mark, in its name, and the menu's answers go on with no genie by. With no rehabilitators (a server
// with no character store) it only says farewell.
func TestTheGenieOffersRehabilitation(t *testing.T) {
	f := newGenieFixture(t)
	box := f.summon()
	if f.g.rehab != nil {
		t.Fatal("a fixture's genie has rehabilitators of its own")
	}
	msgs := f.answer(box, genieRehab)
	if f.genieNPC() != nil || len(msgs) == 0 || opOf(msgs[0]) == protocol.OpcodeQuestPopupMulti {
		t.Fatalf("no rehabilitators: sent %s, genie %v", ops(msgs), f.genieNPC())
	}

	f.g.rehab = NewRehab(f.db, f.w, f.inv, database.NewSpellbookStore(f.db), nil, database.NewSQLiteCharacterStore(f.db))
	box = f.summon()
	msgs = f.answer(box, genieRehab)
	if f.genieNPC() != nil {
		t.Fatal("the genie stayed")
	}
	var id uint32
	for _, m := range msgs {
		if opOf(m) == protocol.OpcodeQuestPopupMulti {
			r := buffer.NewReader(m[2:])
			id, _ = r.ReadUint32()
		}
	}
	if !IsRehabBox(id) {
		t.Fatalf("sent %s, box %08X: not a rehabilitator's", ops(msgs), id)
	}
	if !strings.Contains(string(joinBytes(msgs)), "G\x00e\x00n\x00i\x00e") {
		t.Error("it does not speak as the genie")
	}
	// the menu's first answer goes on, the genie gone: the resets' menu
	f.sess.Queue().GetPending()
	f.g.rehab.Answer(f.sess, f.p, id, 0)
	var next uint32
	for _, m := range f.sess.Queue().GetPending() {
		if d := m.Data(); opOf(d) == protocol.OpcodeQuestPopupMulti {
			r := buffer.NewReader(d[2:])
			next, _ = r.ReadUint32()
		}
	}
	if !IsRehabBox(next) || next == id {
		t.Errorf("after the menu's first answer: box %08X", next)
	}
}

func joinBytes(msgs [][]byte) []byte {
	var out []byte
	for _, m := range msgs {
		out = append(out, m...)
	}
	return out
}
