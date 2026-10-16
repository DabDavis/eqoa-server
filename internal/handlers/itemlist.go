package handlers

import (
	"sync"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// invListCap is the records a client item list holds (VIItemList's
// ItemInternal[40]); AddItem is called with 40 at every site.
const invListCap = 40

// invList is the server's copy of the client's inventory list
// (VIClient+0xC428, a VIItemList): which record sits at each list index, the
// key the client files it under, and the counter VIItemList::AddItem
// (0x00BBD130) numbers the next new record from.
//
// The client asks for an item by KEY (VIItemList::GetSlotId, the record's
// +0x2F8: UseItem's 0x003E, SwapItems' 0x003D, Send0062 and the rest) and the
// server answers by LIST INDEX (0x003C, 0x003D, 0x003E, 0x003F). The two agree
// only until a record leaves: VIItemList::Took (0x00BBD430) packs the list and
// never renumbers, and AddItem gives a new record the counter, not the key it
// arrived with. SendMemoryDump fills the list the way it writes it (key =
// position, counter = the count); every record the server adds or takes after
// that must go through add and took so the two copies stay in step.
type invList struct {
	mu   sync.Mutex
	recs []invRec // list order: recs[i] is the record at list index i
	next uint32   // VIItemList's counter (+4)
}

// invRec is one record of an invList.
type invRec struct {
	key       uint32 // what the client sends to name it
	itemID    int    // charinventory.itemid
	patternID int    // the item it is (AddItem stacks onto the same item id)
	qty       int    // the stack
}

// newInvList is the list SendMemoryDump describes: the inventory rows in the
// order it writes them, each keyed by its position, the counter the count.
func newInvList(rows []database.CharInventoryEntry) *invList {
	l := &invList{recs: make([]invRec, 0, len(rows)), next: uint32(len(rows))}
	for i := range rows {
		l.recs = append(l.recs, invRec{
			key:       uint32(i),
			itemID:    rows[i].ItemID,
			patternID: rows[i].Pattern.PatternID,
			qty:       rows[i].StackLeft,
		})
	}
	return l
}

// sessionInvList is the session's copy, or nil before its MemoryDump.
func sessionInvList(sess *session.Session) *invList {
	if sess == nil {
		return nil
	}
	l, _ := sess.InventoryList().(*invList)
	return l
}

// index is VIItemList::Find (0x00BDFFA8): the list index of the record with
// that key, or -1.
func (l *invList) index(key uint32) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := range l.recs {
		if l.recs[i].key == key {
			return i
		}
	}
	return -1
}

// at is the record at list index i.
func (l *invList) at(i int) (invRec, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if i < 0 || i >= len(l.recs) {
		return invRec{}, false
	}
	return l.recs[i], true
}

// add is VIItemList::AddItem (0x00BBD130) for a record the server is about to
// send: a stackable item (maxStack 2 or more) goes onto the FIRST record of the
// same item, clamped at maxStack (what does not fit is dropped); anything else,
// or a stackable item with no record yet, takes a new record at the end under
// the counter, its quantity clamped (stackable) or 1. A full list takes
// nothing. It returns the key the client filed it under and how many it took.
func (l *invList) add(itemID, patternID, qty, maxStack int) (key uint32, took int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if maxStack >= 2 {
		for i := range l.recs {
			if l.recs[i].patternID != patternID {
				continue
			}
			took = qty
			if sum := l.recs[i].qty + qty; maxStack < sum {
				took = maxStack - l.recs[i].qty
				l.recs[i].qty = maxStack
			} else {
				l.recs[i].qty = sum
			}
			return l.recs[i].key, took
		}
	}
	if len(l.recs) >= invListCap {
		return 0, 0
	}
	took = 1
	if maxStack >= 2 {
		took = qty
		if maxStack < took {
			took = maxStack
		}
	}
	key = l.next
	l.next++
	l.recs = append(l.recs, invRec{key: key, itemID: itemID, patternID: patternID, qty: took})
	return key, took
}

// took is VIItemList::Took (0x00BBD430): n off the record at list index i, and
// the record leaves the list, the rest moving down with their keys, when none
// is left. The client asserts a slot in range holding at least n (and then
// never recovers), so a request that breaks that is refused here.
func (l *invList) took(i, n int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if i < 0 || i >= len(l.recs) || l.recs[i].qty < n {
		return false
	}
	l.recs[i].qty -= n
	if l.recs[i].qty == 0 {
		l.recs = append(l.recs[:i], l.recs[i+1:]...)
	}
	return true
}

// sessionBankList is the session's copy of the client's bank list
// (VIBankControl+8), or nil before its MemoryDump.
func sessionBankList(sess *session.Session) *invList {
	if sess == nil {
		return nil
	}
	l, _ := sess.BankList().(*invList)
	return l
}

// wouldAdd is add without the change: how many of qty the client would keep.
func (l *invList) wouldAdd(patternID, qty, maxStack int) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	if maxStack >= 2 {
		for i := range l.recs {
			if l.recs[i].patternID != patternID {
				continue
			}
			if sum := l.recs[i].qty + qty; maxStack < sum {
				return maxStack - l.recs[i].qty
			}
			return qty
		}
	}
	if len(l.recs) >= invListCap {
		return 0
	}
	if maxStack >= 2 {
		return min(qty, maxStack)
	}
	return 1
}

// stackOnto is the list index of the record a stackable item would join
// (AddItem's first record of the same item), or -1 for a new record.
func (l *invList) stackOnto(patternID, maxStack int) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	if maxStack < 2 {
		return -1
	}
	for i := range l.recs {
		if l.recs[i].patternID == patternID {
			return i
		}
	}
	return -1
}

// nextKey is the key AddItem would give a new record (VIItemList +4).
func (l *invList) nextKey() uint32 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.next
}
