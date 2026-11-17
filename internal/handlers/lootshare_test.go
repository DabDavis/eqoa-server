package handlers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// No Trade stacks on a corpse are shared one to a character (lootshare.go; the owner,
// 2026-11-17: "the keys dropping from Plane of sky are stacking so only 1 player gets
// them since they are notrade").

// skyKey adds a No Trade key stackable to 999 and puts a stack of n of it on the
// creature (as tools/sky_keys_20261111.py does: one npcitem row of stackleft 20).
func (r *raidLootRig) skyKey(name string, n int) int {
	r.t.Helper()
	res, err := r.f.db.Exec(`INSERT INTO itempattern
		(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
		 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (0, 0, 0, 0, 0, 0, 1, 1, 0, 0, 0, 0, 1, 999, 0, 0, 32767, 0, 0, 0, 0, 0, ?, 'a key', 0, 0)`, name)
	if err != nil {
		r.t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := r.f.db.Exec(`INSERT INTO npcitem (npcid, stackleft, remainHP, patternid, listnumber)
		VALUES (?, ?, 0, ?, ?)`, corpseNPCID, n, id, len(r.pat)); err != nil {
		r.t.Fatal(err)
	}
	r.pat[name] = int(id)
	return int(id)
}

// lootCall runs a loot handler for session i with a payload.
func (r *raidLootRig) lootCall(i int, op protocol.GameOpcode, fill func(w *buffer.Writer)) {
	r.t.Helper()
	msg := protocol.NewMessage(protocol.MessageTypeReliable, op)
	msg.SetData(build(op, 256, fill)[2:])
	reg := NewRegistry()
	r.f.h.RegisterHandlers(reg)
	h, ok := reg.Get(op)
	if !ok {
		r.t.Fatalf("no handler for 0x%04X", uint16(op))
	}
	if err := h(r.sess[i], msg); err != nil {
		r.t.Fatal(err)
	}
}

// view is what player i's window on the corpse lists now: names and counts.
func (r *raidLootRig) view(i int) map[string]int {
	ls := r.f.h.loots()
	ls.mu.Lock()
	defer ls.mu.Unlock()
	out := map[string]int{}
	c := ls.corpses[r.f.npc.ServerID]
	if c == nil {
		return out
	}
	for _, it := range ls.viewLocked(r.f.npc.ServerID, c, r.p[i].CharServerID) {
		out[it.Pattern.ItemName] += it.StackLeft
	}
	return out
}

// keyAt is the position of the item named in player i's view, -1 none.
func (r *raidLootRig) keyAt(i int, name string) int {
	ls := r.f.h.loots()
	ls.mu.Lock()
	defer ls.mu.Unlock()
	c := ls.corpses[r.f.npc.ServerID]
	if c == nil {
		return -1
	}
	for k, it := range ls.viewLocked(r.f.npc.ServerID, c, r.p[i].CharServerID) {
		if it.Pattern.ItemName == name {
			return k
		}
	}
	return -1
}

// A six-player raid kills a key boss: each member opens the corpse in turn and takes
// everything (Take all), and each comes away with ONE key, the corpse lying until the
// last has its own; the one before it could not take the rest. The raid's rules go on
// over the corpse's other items as before (the ring is rolled on). After every member
// has had theirs, the spares are one stack, anyone's.
func TestARaidKillGivesEveryMemberItsKey(t *testing.T) {
	r := newRaidLootRig(t, 6)
	r.skyKey("Rain Key 3", 20)
	r.item("Ring of Granavik", 1, 7, 1, 0, 0, 1) // no trade, not stackable: rolled
	r.kill(entity.LootNeedGreed)
	if r.rollOf("Ring of Granavik") == nil {
		t.Fatal("the ring is not rolled on: the raid's rules broke")
	}
	c := r.corpse()
	if len(c.items) != 0 || len(c.shares) != 1 {
		t.Fatalf("corpse: %d free items, %d shares; want the key shared alone", len(c.items), len(c.shares))
	}
	for i := 0; i < 6; i++ {
		if got := r.view(i)["Rain Key 3"]; got != 1 {
			t.Fatalf("%s sees %d keys before anyone took one, want 1 of its own", r.p[i].Name, got)
		}
	}
	for i := 0; i < 6; i++ {
		r.f.h.OpenCorpse(r.sess[i], r.f.npc)
		r.lootCall(i, protocol.OpcodeLootTakeAll, nil)
		r.lootCall(i, protocol.OpcodeClientCloseLoot, nil)
		want := 1
		if i == 5 {
			want = 15 // its own, then the 14 spares, every member served
		}
		if got := r.owns(i, "Rain Key 3"); got != want {
			t.Fatalf("%s took %d keys, want %d", r.p[i].Name, got, want)
		}
		if i < 5 && !r.corpse().owedLocked() {
			t.Fatalf("the corpse owes no key with %d members still to take theirs", 5-i)
		}
		r.drain()
	}
	if r.corpse() != nil && r.corpse().owedLocked() {
		t.Fatal("a key is still owed after every member took one")
	}
}

// A single take (0x0019) of the key's record takes one, whatever count is asked; the
// taker sees no second; a raid member who holds the key already is not given another,
// and the copy goes to the next one; a kill bigger than the stack still gives each its
// own (never fewer copies than characters).
func TestNoTradeKeysNeverStackOnACorpse(t *testing.T) {
	r := newRaidLootRig(t, 6)
	key := r.skyKey("Day Key 2", 4) // fewer than the raid
	// member 2 holds one from an earlier kill
	if _, err := r.f.db.Exec(`INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber)
		VALUES (?, 1, 0, 0, ?, -1, 0, 0)`, r.p[2].CharServerID, key); err != nil {
		t.Fatal(err)
	}
	rows, _ := r.f.inv.GetCharacterItems(r.p[2].CharServerID)
	held := newInvList(rows)
	held.bank = newInvList(nil)
	r.sess[2].SetInventoryList(held)
	r.kill(entity.LootNeedGreed)
	if c := r.corpse(); len(c.shares) != 1 || c.shares[0].left != 6 {
		t.Fatalf("a stack of 4 for 6 seats: %+v", c.shares)
	}
	r.f.h.OpenCorpse(r.sess[0], r.f.npc)
	k := r.keyAt(0, "Day Key 2")
	r.lootCall(0, protocol.OpcodeClientLoot, func(w *buffer.Writer) { w.WriteUint32(uint32(k)); w.WriteUint32(20) })
	if got := r.owns(0, "Day Key 2"); got != 1 {
		t.Fatalf("a take of 20 gave %d keys, want 1", got)
	}
	if got := r.view(0)["Day Key 2"]; got != 0 {
		t.Fatalf("the taker sees %d more keys while others wait", got)
	}
	r.lootCall(0, protocol.OpcodeClientCloseLoot, nil)

	r.f.h.OpenCorpse(r.sess[2], r.f.npc) // holds one: offered none
	if got := r.view(2)["Day Key 2"]; got != 0 {
		t.Fatalf("a member holding the key is offered %d more", got)
	}
	r.lootCall(2, protocol.OpcodeClientCloseLoot, nil)
	for _, i := range []int{1, 3, 4, 5} {
		r.f.h.OpenCorpse(r.sess[i], r.f.npc)
		r.lootCall(i, protocol.OpcodeLootTakeAll, nil)
		r.lootCall(i, protocol.OpcodeClientCloseLoot, nil)
		if got := r.owns(i, "Day Key 2"); got < 1 {
			t.Fatalf("%s got no key", r.p[i].Name)
		}
	}
	if got := r.owns(2, "Day Key 2"); got != 1 {
		t.Fatalf("the member who held one has %d", got)
	}
	total := 0
	for i := range r.p {
		total += r.owns(i, "Day Key 2")
	}
	if total != 7 { // the one held before + five taken + the sixth copy, a spare, the last one owed's
		t.Fatalf("%d keys in the raid's hands, want 7", total)
	}
}

// The controls: one in no group, alone at its kill, takes the whole No Trade stack in
// one Take all (the share's spares are its once it has its own); a stack that trades
// is one record as ever.
func TestALoneKillerTakesTheWholeNoTradeStack(t *testing.T) {
	r := newRaidLootRig(t, 1)
	r.skyKey("frog hair", 3)
	r.item("Bone Chips", 20, 0, 0, 0, 0, 1) // trades: a stack of 10, one record
	r.f.h.heardKillRight(entity.KillCredit{NPCID: r.f.npc.ServerID, Died: r.f.npc.DeathTime,
		Players: []entity.KillPlayer{{ServerID: r.p[0].CharServerID, Name: r.p[0].Name}}})
	r.f.h.OpenCorpse(r.sess[0], r.f.npc)
	if c := r.corpse(); len(c.items) != 1 || c.items[0].StackLeft != 10 || len(c.shares) != 1 {
		t.Fatalf("corpse: %+v / %d shares", c.items, len(c.shares))
	}
	r.lootCall(0, protocol.OpcodeLootTakeAll, nil)
	if a, b := r.owns(0, "frog hair"), r.owns(0, "Bone Chips"); a != 3 || b != 10 {
		t.Fatalf("a lone killer took %d frog hair (want 3) and %d bone chips (want 10)", a, b)
	}
}

// A corpse's shares come through a restart: who had theirs, who waits, what is left.
func TestASharedKeyComesThroughARestart(t *testing.T) {
	r := newRaidLootRig(t, 3)
	r.skyKey("Night Key 4", 20)
	r.kill(entity.LootNeedGreed)
	r.f.h.OpenCorpse(r.sess[0], r.f.npc)
	r.lootCall(0, protocol.OpcodeLootTakeAll, nil)
	r.lootCall(0, protocol.OpcodeClientCloseLoot, nil)
	prev := keepWindows.Swap(r.f.h)
	t.Cleanup(func() { keepWindows.Store(prev) })
	b, err := json.Marshal(KeepCorpses())
	if err != nil {
		t.Fatal(err)
	}
	var kept []KeptCorpse
	if err := json.Unmarshal(b, &kept); err != nil {
		t.Fatal(err)
	}
	ls := r.f.h.loots()
	ls.mu.Lock()
	delete(ls.corpses, r.f.npc.ServerID)
	ls.mu.Unlock()
	if n := RestoreCorpses(kept); n != 1 {
		t.Fatalf("restored %d corpses", n)
	}
	if got := r.view(0)["Night Key 4"]; got != 0 {
		t.Fatalf("the one that had its key sees %d after the restart", got)
	}
	if got := r.view(1)["Night Key 4"]; got != 1 {
		t.Fatalf("one still owed sees %d after the restart, want 1", got)
	}
	if c := r.corpse(); c.shares[0].left != 19 || !c.owedLocked() {
		t.Fatalf("share after the restart: %+v", c.shares[0])
	}
}

// TestRealSkyKeysAreShared rolls every Plane of Sky key boss's corpse on a server
// database (EQOA_REAL_DB, copied): each key is a share of 20, none of it a free record,
// and no other No Trade stack lies on any creature's own items.
func TestRealSkyKeysAreShared(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	path := filepath.Join(t.TempDir(), "keys.db")
	in, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, in, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT n.npc_id, n.npc_name, n.npc_level, p.itemname, i.stackleft FROM npcitem i
		JOIN itempattern p ON p.patternid = i.patternid JOIN npcs n ON n.npc_id = i.npcid
		WHERE p.trade > 0 AND i.stackleft > 1 ORDER BY n.npc_id`)
	if err != nil {
		t.Fatal(err)
	}
	type boss struct {
		id, level, n int
		name, key    string
	}
	var bosses []boss
	for rows.Next() {
		var b boss
		if err := rows.Scan(&b.id, &b.name, &b.level, &b.key, &b.n); err != nil {
			t.Fatal(err)
		}
		bosses = append(bosses, b)
	}
	rows.Close()
	h := NewNPCWindowHandler(nil, database.NewInventoryStore(db))
	h.lootFloat = func() float64 { return 0.999999 } // no category lands: the boss's own items alone
	h.lootIntn = func(n int) int { return n - 1 }
	keys := 0
	for _, b := range bosses {
		npc := &entity.NPC{ServerID: entity.EntityID(b.id), NPCID: b.id, Name: b.name, Level: b.level, World: 4}
		c := h.rollCorpse(npc, time.Now(), 60)
		for _, it := range c.items {
			if it.Pattern.Trade > 0 && it.StackLeft > 1 {
				t.Errorf("%s: %d %s lie as one record", b.name, it.StackLeft, it.Pattern.ItemName)
			}
		}
		found := false
		for _, s := range c.shares {
			if s.rec.Pattern.ItemName == b.key && s.left == b.n && s.rec.StackLeft == 1 {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: %d %s not a share", b.name, b.n, b.key)
		}
		if strings.Contains(strings.ToLower(b.key), "key") {
			keys++
		}
		t.Logf("%s (%d): %d %s shared", b.name, b.id, b.n, b.key)
	}
	if keys != 21 || len(bosses) != 21 {
		t.Errorf("%d No Trade stacks (%d keys), want the 21 Sky keys", len(bosses), keys)
	}
}
