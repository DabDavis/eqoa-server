package handlers

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A trade with one's own merc through the trade window (merctrade.go,
// Heirloom petition 3306): the items offered go onto the merc at the
// player's accept; cancel moves nothing; another's merc, a pet, any other NPC
// and a dead merc are refused with the trade's line; what the merc's give
// refuses (a No Rent item that vanishes at logout) is refused as an offer,
// while a NO TRADE LORE item goes as the merc's give lets it; tunar is
// refused; a merc with no room refuses the trade before anything moves.

type mercTradeRig struct {
	*portalRig
	m   *Mercs
	db  *database.SQLiteDB
	h   *TradeHandler
	npc *entity.NPC // Anna's merc
}

const mercTradeSeen = uint32(100001)

func newMercTradeRig(t *testing.T) *mercTradeRig {
	t.Helper()
	r, m, db := newMercFixture(t)
	invItems(t, db)
	m.hireWith(r.anna, r.pa, 13, 0, 0, -1)
	h := NewTradeHandler(r.sm, &NPCWindowHandler{inv: database.NewInventoryStore(db)})
	h.SetGameWorld(r.gw)
	h.mercs = m
	rig := &mercTradeRig{portalRig: r, m: m, db: db, h: h}
	rig.npc = rig.mercOf(r.pa)
	rig.sees(rig.npc)
	r.drain()
	return rig
}

// mercOf is p's merc in the world.
func (r *mercTradeRig) mercOf(p *entity.Player) *entity.NPC {
	r.t.Helper()
	for _, n := range r.gw.NPCs {
		if n.Owner == p.ServerID {
			if _, ok := r.gw.MercOwnSlot(r.sessOf(p), n.ServerID); ok {
				return n
			}
		}
	}
	r.t.Fatalf("%s has no merc out", p.Name)
	return nil
}

func (r *mercTradeRig) sessOf(p *entity.Player) *session.Session {
	for _, s := range []*session.Session{r.anna, r.bert, r.cora, r.dora} {
		if r.gw.SessionPlayer(s) == p {
			return s
		}
	}
	return nil
}

// sees puts n on Anna's first slot, as mercTradeSeen.
func (r *mercTradeRig) sees(n *entity.NPC) {
	vs := entity.GetOrCreateVisibility(r.anna)
	vs.ChannelEntity[0], vs.ChannelObjectID[0] = n.ServerID, mercTradeSeen
}

func (r *mercTradeRig) ask() {
	r.t.Helper()
	if err := r.h.HandleTradeRequest(r.anna, incoming(protocol.OpcodeTradeRequest, u32b(mercTradeSeen))); err != nil {
		r.t.Fatal(err)
	}
}

func (r *mercTradeRig) ui(sub byte, fill func(w *buffer.Writer)) {
	r.t.Helper()
	b := make([]byte, 64)
	w := buffer.NewWriter(b)
	w.WriteByte(sub)
	w.WriteUint32(mercTradeSeen)
	if fill != nil {
		fill(w)
	}
	if err := r.h.HandleTradeUI(r.anna, incoming(protocol.OpcodeTradeUI, b[:w.Position()])); err != nil {
		r.t.Fatal(err)
	}
}

// offer is sub 3: n of the pack's record of that pattern.
func (r *mercTradeRig) offer(pattern, n int) {
	r.t.Helper()
	l := sessionInvList(r.anna)
	for i := 0; ; i++ {
		rec, ok := l.at(i)
		if !ok {
			r.t.Fatalf("no %d in the pack", pattern)
		}
		if rec.patternID == pattern {
			r.ui(3, func(w *buffer.Writer) { w.WriteUint32(rec.key); w.Write7BitEncodedInt64(int64(n)) })
			return
		}
	}
}

// sent is what Anna was sent: each 0x0056 as "sub:N" (in order), each line
// as portalRig.msgs reads it.
func (r *mercTradeRig) sent() (subs []string, lines []string) {
	for _, m := range r.anna.Queue().PeekPending() {
		if d := m.Data(); len(d) >= 3 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeTradeUI {
			subs = append(subs, fmt.Sprintf("sub:%d", d[2]))
		}
	}
	for _, l := range r.msgs(r.anna) {
		if l != "op:0056" {
			lines = append(lines, l)
		}
	}
	return subs, lines
}

func hasTradeSub(subs []string, n int) bool { return hasMsg(subs, fmt.Sprintf("sub:%d", n)) }

func (r *mercTradeRig) open() {
	r.t.Helper()
	r.ask()
	subs, lines := r.sent()
	if len(subs) < 2 || subs[0] != "sub:2" || subs[1] != "sub:6" {
		r.t.Fatalf("the ask was answered %q %q, want the merc's yes (2) and accept (6)", subs, lines)
	}
}

func TestMercTradeGivesItsOwner(t *testing.T) {
	r := newMercTradeRig(t)
	r.pack(r.db, [2]int{itBreastplate, 1}, [2]int{itPotion, 10}, [2]int{itStone, 1})
	r.open()
	r.offer(itBreastplate, 1)
	r.offer(itPotion, 4) // part of a stack
	r.offer(itStone, 1)  // NO TRADE LORE: the merc's give lets it go
	subs, lines := r.sent()
	if n := len(subs); n != 9 || subs[0] != "sub:3" || subs[1] != "sub:7" || subs[2] != "sub:6" {
		t.Fatalf("the offers sent %q %q, want 3/7/6 each", subs, lines)
	}
	// nothing moves before the accept
	if packHas(t, r.db, r.pa.CharServerID, itBreastplate) != 1 {
		t.Fatal("the breastplate left before the accept")
	}
	r.ui(6, nil)
	subs, lines = r.sent()
	if !hasTradeSub(subs, 16) {
		t.Fatalf("no 16 (done) after the accept: %q %q", subs, lines)
	}
	if !hasMsg(lines, "chat:You give Mithril Breastplate to "+r.npc.Name+".") || !hasMsg(lines, "chat:You give 4 Volatile Potion to "+r.npc.Name+".") {
		t.Errorf("lines %q", lines)
	}
	for _, c := range []struct{ pattern, pack, merc int }{{itBreastplate, 0, 1}, {itPotion, 6, 4}, {itStone, 0, 1}} {
		if got := packHas(t, r.db, r.pa.CharServerID, c.pattern); got != c.pack {
			t.Errorf("pattern %d: the pack holds %d, want %d", c.pattern, got, c.pack)
		}
		if got, e := mercHas(t, r.db, r.pa.CharServerID, 1, c.pattern); got != c.merc || e != -1 {
			t.Errorf("pattern %d: the merc holds %d (equip %d), want %d carried", c.pattern, got, e, c.merc)
		}
	}
	if l := sessionInvList(r.anna); l.held(itBreastplate) != 0 || l.held(itPotion) != 6 {
		t.Error("the client's list was not told")
	}
	if r.h.trading(r.anna) {
		t.Error("the trade is still held after 16")
	}
}

// Cancel moves nothing, and forgets the trade.
func TestMercTradeCancelMovesNothing(t *testing.T) {
	r := newMercTradeRig(t)
	r.pack(r.db, [2]int{itBreastplate, 1})
	r.open()
	r.offer(itBreastplate, 1)
	r.ui(1, nil)
	r.ui(6, nil) // an accept after the cancel is no trade's
	if packHas(t, r.db, r.pa.CharServerID, itBreastplate) != 1 {
		t.Fatal("the breastplate left at a cancel")
	}
	if n, _ := mercHas(t, r.db, r.pa.CharServerID, 1, itBreastplate); n != 0 {
		t.Fatal("the merc holds it after a cancel")
	}
	if r.h.trading(r.anna) {
		t.Error("the trade is held after the cancel")
	}
}

// Refused: another's merc, a pet, any other NPC, a dead merc, an id naming
// nothing. The control: Anna's own merc, alive, opens.
func TestMercTradeRefusals(t *testing.T) {
	r := newMercTradeRig(t)
	r.m.hireWith(r.cora, r.pc, 0, 0, 0, -1)
	coras := r.mercOf(r.pc)
	coras.X, coras.Y, coras.Z = r.pa.X+1, r.pa.Y, r.pa.Z
	pet := &entity.NPC{ServerID: 7791, Name: "Fire Elemental", X: r.pa.X + 1, Y: r.pa.Y, Z: r.pa.Z, Level: 20, HP: 100, MaxHP: 100, Owner: r.pa.ServerID}
	r.gw.NPCs[pet.ServerID] = pet
	refused := func(what string, n *entity.NPC) {
		t.Helper()
		if n != nil {
			r.sees(n)
		} else {
			vs := entity.GetOrCreateVisibility(r.anna)
			vs.ChannelEntity[0], vs.ChannelObjectID[0] = 0, 0
		}
		r.drain()
		r.ask()
		subs, lines := r.sent()
		if hasTradeSub(subs, 2) || r.h.trading(r.anna) {
			t.Errorf("%s: a trade opened (%q)", what, subs)
		}
		if !hasTradeSub(subs, 1) || !hasMsg(lines, "colour:"+tradeRefusedLine) {
			t.Errorf("%s: sent %q %q, want the refusal line and the ask called off (1)", what, subs, lines)
		}
	}
	refused("another's merc", coras)
	refused("a pet", pet)
	refused("an NPC", r.liaison())
	refused("nothing", nil)

	r.sees(r.npc)
	r.npc.Dead = true
	refused("a dead merc", r.npc)
	r.npc.Dead = false
	r.drain()
	r.open() // the control
}

// What the merc's own give refuses, the trade refuses as an offer (its line
// said, nothing filed): a No Rent item that vanishes at logout. Tunar is
// refused. The control: a plain item offered beside it goes.
func TestMercTradeRefusesWhatTheMercGiveRefuses(t *testing.T) {
	r := newMercTradeRig(t)
	if _, err := r.db.Exec(`INSERT INTO norentvanish (patternid) VALUES (?)`, itGreatsword); err != nil {
		t.Fatal(err)
	}
	r.pack(r.db, [2]int{itGreatsword, 1}, [2]int{itBuckler, 1})
	r.open()
	r.offer(itGreatsword, 1)
	subs, lines := r.sent()
	if hasTradeSub(subs, 3) || !hasMsg(lines, "red:"+fmt.Sprintf(mercGiveVanish, "Greatsword")) {
		t.Fatalf("a vanishing item offered: %q %q", subs, lines)
	}
	r.ui(5, func(w *buffer.Writer) { w.Write7BitEncodedInt64(100) })
	if _, lines := r.sent(); !hasMsg(lines, "red:"+fmt.Sprintf(mercNoTunarLine, r.npc.Name)) {
		t.Errorf("tunar offered: %q", lines)
	}
	r.offer(itBuckler, 1)
	r.ui(6, nil)
	if packHas(t, r.db, r.pa.CharServerID, itGreatsword) != 1 {
		t.Error("the vanishing greatsword left the pack")
	}
	if n, _ := mercHas(t, r.db, r.pa.CharServerID, 1, itBuckler); n != 1 {
		t.Error("the control: the buckler did not go")
	}

	// a merc holding all it can refuses the trade before anything moves
	for i := 0; i < database.MercItemsMax; i++ {
		if _, err := r.db.Exec(`INSERT INTO charmercitem (serverid, slot, patternid, stack, remainhp, charges, equiploc, added)
			VALUES (?, 1, ?, 1, 100, 0, -1, '')`, r.pa.CharServerID, itStone); err != nil {
			t.Fatal(err)
		}
	}
	r.pack(r.db, [2]int{itBreastplate, 1})
	r.drain()
	r.open()
	r.offer(itBreastplate, 1)
	r.ui(6, nil)
	subs, lines = r.sent()
	if !hasTradeSub(subs, 17) || hasTradeSub(subs, 16) || !hasMsg(lines, "red:"+fmt.Sprintf(mercGiveFull, r.npc.Name, database.MercItemsMax)) {
		t.Errorf("a full merc: %q %q, want 17 and the full line", subs, lines)
	}
	if packHas(t, r.db, r.pa.CharServerID, itBreastplate) != 1 {
		t.Error("the breastplate left for a full merc")
	}
}
