package handlers

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// clientList is the client's own item list as CLIENT keeps it (vanilla's
// inventory.rs, VIItemList): records in list order, each under the key its
// AddItem gave it from the counter, a Took by list index. It applies the
// server's messages in the order they were queued, as the client does.
type clientList struct {
	recs []clientRec
	next uint32
}

type clientRec struct {
	key            uint32
	pattern, qty   int
	maxStack, item int
}

// fromServer is the client's list as a dump leaves it: the server's copy's
// records, keyed by position, the counter the count.
func clientFromDump(l *invList) *clientList {
	c := &clientList{next: uint32(len(l.recs))}
	for i, r := range l.recs {
		c.recs = append(c.recs, clientRec{key: uint32(i), pattern: r.patternID, qty: r.qty})
	}
	return c
}

// apply is one queued message: 0x003B AddInvItem (AddItem: onto the first
// record of the same item up to its stack, else a new record under the
// counter), 0x003C Msg003C (Took: n off list index i, the record gone at none).
func (c *clientList) apply(t *testing.T, data []byte, maxStack map[int]int) {
	t.Helper()
	op := protocol.GameOpcode(binary.LittleEndian.Uint16(data))
	r := buffer.NewReader(data[2:])
	switch op {
	case protocol.OpcodeAddInvItem:
		qty, _ := r.Read7BitEncodedInt64()
		for k := 0; k < 4; k++ { // condition, charges, equip, quick slot
			_, _ = r.Read7BitEncodedInt64()
		}
		_, _ = r.ReadUint32() // the key the server filed it under: CLIENT keeps its own
		pat, _ := r.Read7BitEncodedInt64()
		ms := maxStack[int(pat)]
		if ms >= 2 {
			for i := range c.recs {
				if c.recs[i].pattern == int(pat) {
					c.recs[i].qty = min(ms, c.recs[i].qty+int(qty))
					return
				}
			}
		}
		n := 1
		if ms >= 2 {
			n = min(int(qty), ms)
		}
		c.recs = append(c.recs, clientRec{key: c.next, pattern: int(pat), qty: n})
		c.next++
	case protocol.OpcodeRemoveInvItem:
		i, _ := r.ReadByte()
		_, _ = r.ReadByte()
		n, _ := r.Read7BitEncodedInt64()
		if int(i) >= len(c.recs) || c.recs[i].qty < int(n) {
			t.Errorf("the client's Took(%d, %d) on a list of %d: CLIENT asserts", i, n, len(c.recs))
			return
		}
		c.recs[i].qty -= int(n)
		if c.recs[i].qty == 0 {
			c.recs = append(c.recs[:i], c.recs[i+1:]...)
		}
	}
}

// apart is how the client's list and the server's copy differ, or "".
func (c *clientList) apart(l *invList) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(c.recs) != len(l.recs) || c.next != l.next {
		return fmt.Sprintf("the client holds %d records (next key %d), the server's copy %d (next key %d)",
			len(c.recs), c.next, len(l.recs), l.next)
	}
	for i := range c.recs {
		a, b := c.recs[i], l.recs[i]
		if a.key != b.key || a.pattern != b.patternID || a.qty != b.qty {
			return fmt.Sprintf("list index %d: the client's key %d (item %d x%d), the server's key %d (item %d x%d)",
				i, a.key, a.pattern, a.qty, b.key, b.patternID, b.qty)
		}
	}
	return ""
}

// smokesPack runs Live petition 5401's mix against one character's pack at
// once, as his packets' goroutines and the World's summon hook ran it at the
// kappa camps: Feed's Unit of Blood one at a time (GiveItemNow, the summon
// hook's), seven Units spent at a time (takeStacks, Vampiric Strike's
// reagent), Cloth Pants coming in (a loot's new records) and Destroys of them
// (the item menu's 0x003C), each step done as dispatch does it. The client's
// list, fed the queued messages in order, must end as the server's copy.
func smokesPack(t *testing.T, dispatch func(sess *session.Session, fn func())) string {
	f := newScribeFixture(t)
	f.potionWorld()
	blood := f.potion("Unit of Blood", -1, 30, 0, "", "", "")
	pants := f.potion("Cloth Pants", 10, 1, 0, "", "", "")
	gem := f.potion("Hematite", -1, 50, 0, "", "", "")
	maxStack := map[int]int{blood: 30, pants: 1, gem: 50}
	for i := 0; i < 6; i++ {
		if _, err := f.inv.AddCharItem(f.sid, gem, 3, 0, 0, 0); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := f.inv.GetCharacterItems(f.sid)
	if err != nil {
		t.Fatal(err)
	}
	f.sess.SetInventoryList(newInvListCap(rows, 100))
	client := clientFromDump(sessionInvList(f.sess))
	pBlood, _, err := f.inv.ItemPatternByName("Unit of Blood")
	if err != nil {
		t.Fatal(err)
	}
	pPants, _, err := f.inv.ItemPatternByName("Cloth Pants")
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	run := func(n int, step func(i int)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < n; i++ {
				dispatch(f.sess, func() { step(i) })
				time.Sleep(time.Duration(200+97*(i%7)) * time.Microsecond) // the steps spread over the same span
			}
		}()
	}
	run(70, func(int) { // Feed
		_, _ = GiveItemNow(f.sess, f.h.gameWorld, f.inv, pBlood, 1) // an error: the comparison below says how
	})
	run(70, func(int) { // Vampiric Strike's seven Units, when there are seven
		items, err := f.inv.GetCharacterItems(f.sid)
		if err != nil {
			t.Error(err)
			return
		}
		var held []database.CharInventoryEntry
		n := 0
		for _, it := range items {
			if it.Pattern.PatternID == blood && it.Location == 0 {
				held = append(held, it)
				n += it.StackLeft
			}
		}
		if n >= 7 {
			takeStacks(f.inv, f.sess, f.player, "Cast", held, 7)
		}
	})
	run(70, func(int) { // a kappa's Cloth Pants
		_, _ = GiveItemNow(f.sess, f.h.gameWorld, f.inv, pPants, 1)
	})
	// Destroys, two packets at a time: each the oldest or the second oldest
	// pants the list holds (the client names them by its own keys; the
	// server's copy's are the same while the two are in step)
	for _, nth := range []int{0, 1} {
		run(70, func(int) {
			l := sessionInvList(f.sess)
			l.mu.Lock()
			key, seen, found := uint32(0), 0, false
			for i := range l.recs {
				if l.recs[i].patternID == pants {
					if seen == nth {
						key, found = l.recs[i].key, true
						break
					}
					seen++
				}
			}
			l.mu.Unlock()
			if found {
				f.h.destroy(f.sess, key, 1)
			}
		})
	}
	wg.Wait()

	for _, m := range f.sess.Queue().GetPending() {
		if d := m.Data(); len(d) >= 2 {
			client.apply(t, d, maxStack)
		}
	}
	if why := client.apart(sessionInvList(f.sess)); why != "" {
		return why
	}
	// and the server's copy as the database holds the pack: each record a
	// row of its item, its stack, and no row the copy lacks
	rows, err = f.inv.GetCharacterItems(f.sid)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[int]database.CharInventoryEntry{}
	for _, r := range rows {
		if r.Location == 0 {
			byID[r.ItemID] = r
		}
	}
	l := sessionInvList(f.sess)
	for i, r := range l.recs {
		row, ok := byID[r.itemID]
		switch {
		case !ok:
			return fmt.Sprintf("list index %d (key %d) is row %d, which the database no longer holds", i, r.key, r.itemID)
		case row.Pattern.PatternID != r.patternID || max(row.StackLeft, 1) != r.qty:
			return fmt.Sprintf("list index %d (key %d): row %d is item %d x%d, the list says item %d x%d",
				i, r.key, r.itemID, row.Pattern.PatternID, row.StackLeft, r.patternID, r.qty)
		}
		delete(byID, r.itemID)
	}
	if len(byID) > 0 {
		return fmt.Sprintf("%d rows in the database the list does not hold", len(byID))
	}
	return ""
}

// Live petition 5401 (2026-11-21, Smoke): Cloth Pants that could be neither
// sold nor destroyed, his client asking for keys the server's copy of his list
// did not hold. His pack changed from several goroutines at once (each packet
// is handled in its own, the summon hook runs in its own), and a step's look-up,
// change and message were not one: a Destroy's list index went stale under a
// take before it, a give stacked onto a record another had just emptied and
// deleted. Done under the session's item lists' hold, as the UDP listener's
// dispatch and withItems now do every one, the client's list ends as the
// server's copy, key for key, and the copy as the database. Without the hold
// (dispatch calling fn bare) this mix went apart in 5 rounds of 5 on the dev
// box (2026-11-21): a list record whose row was gone ("key 9 is item 10, which
// the inventory no longer holds", the Live log's second kind of miss), a give
// refused for "0 rows changed", and a 0x003C for a list index past the
// client's list, which CLIENT asserts on.
func TestAPackChangedFromEverySideAtOnceStaysInStep(t *testing.T) {
	for round := 0; round < 3; round++ {
		if why := smokesPack(t, withItems); why != "" {
			t.Fatalf("round %d: the lists went apart: %s", round, why)
		}
	}
}

// A request for a key the server's copy does not hold (the lists apart) sends
// the client its lists again where it stands, a MemoryDump as a restart's
// resume sends, with a line that says so: the client's list and the copy are
// the database's again (key = position), so the next Destroy reaches the item.
// Not again within itemsResyncEvery; and a corpse's window open is closed first.
func TestAKeyTheServerDoesNotHoldSendsTheListsAgain(t *testing.T) {
	db := keepDB(t)
	_, gw, sm := keepProcess(t, db)
	sess := sm.CreateSession(&net.UDPAddr{IP: net.IPv4(203, 0, 113, 8), Port: 5008}, 8, 8, 8, 1, true)
	sess.SetAccountID(3)
	sess.SetPlayerObjectID(500000007)
	sess.SetState(session.StateInGame)
	sess.SetCharacterInWorld(true)
	rec, err := database.NewSQLiteCharacterStore(db).GetByServerID(7)
	if err != nil {
		t.Fatal(err)
	}
	p := keepChars.Load().enterFromRecord(sess, rec)
	if p == nil {
		t.Fatal("no player")
	}
	inv := database.NewInventoryStore(db)
	res, err := db.Exec(`INSERT INTO itempattern
		(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
		 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (87, 0, 209763529, 0, 10, 0, 0, 1, 0, 0, 0, 0, 1, 1, 0, 0, 32767, 1023, 0, 0, 0, 0, 'Cloth Pants', '', 0, 0)`)
	if err != nil {
		t.Fatal(err)
	}
	pants, _ := res.LastInsertId()
	row, err := inv.AddCharItem(7, int(pants), 1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	// the server's copy as Smoke's was: the pants under key 3; his client
	// files them under key 9
	sess.SetInventoryList(&invList{recs: []invRec{{key: 3, itemID: row, patternID: int(pants), qty: 1}}, next: 4, cap: 100})
	h := NewEquipHandler(nil, inv, nil, keepChars.Load())
	h.playerOf = func(*session.Session) *entity.Player { return gw.SessionPlayer(sess) }

	if h.destroy(sess, 9, 1) {
		t.Fatal("a key the server's copy does not hold was destroyed")
	}
	if n := queuedOpcodes(sess)[protocol.OpcodeMemoryDump]; n != 1 {
		t.Fatalf("dumps queued after the miss: %d", n)
	}
	if lines := gmReplies(t, sess); !hasLine(lines, itemsResyncLine) {
		t.Fatalf("the lines: %q", lines)
	}
	l := sessionInvList(sess)
	if l == nil || len(l.recs) != 1 || l.recs[0].key != 0 || l.recs[0].itemID != row || l.next != 1 {
		t.Fatalf("the list after the dump %+v", l)
	}
	// the next Destroy names the pants by the dump's key, and reaches them
	sess.Queue().GetPending()
	if !h.destroy(sess, 0, 1) {
		t.Fatal("the pants were not destroyed after the lists were sent again")
	}
	// a second miss straight after sends nothing more
	h.destroy(sess, 9, 1)
	if n := queuedOpcodes(sess)[protocol.OpcodeMemoryDump]; n != 0 {
		t.Fatalf("a second dump within %v", itemsResyncEvery)
	}
}
