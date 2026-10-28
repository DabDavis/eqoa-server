package handlers

import (
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Fishing: the server's half of CLIENT's VIClientPart3FC10 (0x00BEB7E0) and
// VIWndFishing (claude-notes/vanilla-decomp-ports-20261028.md).
//
// CLIENT never asks to fish: only 0x0F02 1 opens its window, and the client
// casts, takes and leaves what comes up through 0x0F01. No record of the live
// server's side exists, so what starts a session and what a cast brings up
// are this server's:
//
//   - A session starts when the player uses a fishing pole (wear slot 20)
//     from the HUD (0x003E from a quick slot: equipped first when it is not;
//     CLIENT's disc layout lets the pole on its fourth quick slot), uses a
//     gear set whose main hand is one (0x0022), or says /fish. The item
//     menu's Equip only equips: the window opens only over the bare HUD
//     (VIWndGame::OpenFishing cancels otherwise).
//   - The client casts only with a pole and bait (wear 21) worn, the pole
//     not broken, and the cast's splash on water or slime (FindWater); the
//     server checks the same of its own copy and, with the ground files, the
//     water ten ahead.
//   - Two to five seconds after the cast: nothing bites (8, the bait kept),
//     a fish gets away with the bait (9), the line snaps on the pole (10: a
//     tenth of the pole's condition gone, the bait too), or the catch comes
//     up (7, the bait gone), picked by weight from table fishcatch. A cast's
//     quality (CLIENT's grade of the power meter: 0, 5 or 15) and the
//     angler's skill (characters.fishing, 0 to 100, plus the worn items'
//     fish stat) make a bite likelier and a loss less likely; a catch may
//     teach a point of skill.
//   - The catch list's Take All (Reel), an item (Keep) and Leave All (Pull)
//     are answered as CLIENT's Caught and CloseBait read them: Caught(slot)
//     files the catch the way the client's own AddItem does (onto its stack
//     or a new record), which the server mirrors in its list and database.
//
// A session ends with the client's Cancel (the window's back key, a
// respawn, a window the HUD could not open), or the server's Stop when the
// angler dies, fights, or is gone.

// The fishing opcodes: 0x0F02 to the client (Recv0F02's byte is the
// sub-op), 0x0F01 from it.
const (
	OpcodeFishing      protocol.GameOpcode = 0x0F02
	OpcodeFishingReply protocol.GameOpcode = 0x0F01
)

// Recv0F02's sub-ops, and the client's state after each (VIClientPart3FC10
// +0x7770): 0 no window, 1 may cast, 2 cast, 3 the catch list, 4 waiting for
// a Keep's answer.
const (
	fishStart   byte = 1  // the window opens, the greeting (word 1431); 1
	fishStop    byte = 2  // the window closes, the catch forgotten; 0
	fishReady   byte = 3  // 1, nothing said
	fishCaught  byte = 4  // u32 slot, u32 (unread): that catch into the inventory; 3
	fishWaiting byte = 5  // 3
	fishClose   byte = 6  // the catch list closes (VIWndFishing::CloseBait); 1
	fishLoot    byte = 7  // a VIItemList: the catch list opens (word 1434); 3
	fishMissed  byte = 8  // word 1435; 1
	fishLost    byte = 9  // word 1436; 1
	fishDamaged byte = 10 // word 1437; 1
)

// 0x0F01's sub-ops.
const (
	fishCast   byte = 1 // u32 quality: 0 the meter's middle, 5 near it, 15 the rest
	fishCancel byte = 2
	fishReel   byte = 3 // the catch list's Take All
	fishPull   byte = 4 // its Leave All
	fishKeep   byte = 5 // u32 the catch's key, u32 its count
)

// The wear slots (itempattern.equipslot) of a fishing pole and of bait, and
// the equip slots they go in (equip.go's itemSlotToEquipSlot).
const (
	wearFishingPole = 20
	wearBait        = 21
	equipPole       = 14
)

// The server's numbers, ours: no record of the live game gives them.
const (
	fishBiteMin      = 2 * time.Second // a cast's answer comes this long after it at the earliest
	fishBiteSpread   = 3 * time.Second // and up to this much later
	fishSkillMax     = 100
	fishPoleWearDiv  = 10 // a snapped line takes a tenth of the pole's condition
	fishWatchEvery   = time.Second
	fishLostPercent  = 15 // of the bites, before skill
	fishSnapPercent  = 5  // of the bites, when the pole has a condition
	fishGainDivisor  = 2  // a catch teaches a point with (100 - skill) / this percent
	fishSecondChance = 10 // percent: a second item comes up with the first
)

// The lines the server says, ours.
const (
	fishNoPole     = "You need a fishing pole in your hand to fish."
	fishFighting   = "You can't fish while you are fighting."
	fishDead       = "You can't fish now."
	fishNoBait     = "You need bait to fish."
	fishNoWater    = "Your line found no water there."
	fishFull       = "Your inventory is full."
	fishSkillUp    = "You have become better at fishing! (%d)"
	fishNothingYet = "Nothing bites in these waters."
	fishHowTo      = "To fish, use the pole from a quick slot or a weapon set, or say /fish."
)

// fishState is one angler's session.
type fishState struct {
	sess   *session.Session
	char   uint32 // the character's serverid
	skill  int    // characters.fishing, read at the start
	cast   int    // the cast in the water's number, 0 when none is
	catch  []database.CharInventoryEntry
	keys   []uint32 // the catch's keys, as the list sent them
	casted time.Time
}

// FishingHandler runs the fishing sessions.
type FishingHandler struct {
	inv   *database.InventoryStore
	items *database.ItemPatternStore
	db    *database.SQLiteDB
	chars *database.SQLiteCharacterStore

	// view is the World's FishingView and playerOf its SessionPlayer, rnd
	// the dice, now the clock, after a timer (tests stand in for each)
	view     func(*session.Session, time.Time) (entity.FishingView, bool)
	playerOf func(*session.Session) *entity.Player
	rnd      func(int) int
	now      func() time.Time
	after    func(time.Duration, func()) *time.Timer

	mu     sync.Mutex
	active map[*session.Session]*fishState
	casts  int
	stop   chan struct{}
}

// NewFishingHandler makes the handler. Any store may be nil (tests).
func NewFishingHandler(db *database.SQLiteDB, chars *database.SQLiteCharacterStore, inv *database.InventoryStore) *FishingHandler {
	h := &FishingHandler{
		inv: inv, db: db, chars: chars,
		rnd:    rand.Intn,
		now:    time.Now,
		after:  time.AfterFunc,
		active: map[*session.Session]*fishState{},
	}
	if db != nil {
		h.items = database.NewItemPatternStore(db)
	}
	return h
}

// SetGameWorld gives the handler the world's view of a player, and drops a
// player's session as it leaves the world.
func (h *FishingHandler) SetGameWorld(gw *entity.World) {
	if gw == nil {
		return
	}
	h.view = gw.FishingView
	h.playerOf = gw.SessionPlayer
	gw.OnPlayerLeaving(func(p *entity.Player) {
		h.mu.Lock()
		defer h.mu.Unlock()
		for s, st := range h.active {
			if st.char == p.CharServerID {
				delete(h.active, s)
			}
		}
	})
}

// RegisterHandlers registers 0x0F01.
func (h *FishingHandler) RegisterHandlers(r *Registry) {
	r.Register(OpcodeFishingReply, h.HandleFishing)
}

// Start begins the watch over the sessions: an angler who dies, fights or
// leaves is stopped.
func (h *FishingHandler) Start() {
	h.mu.Lock()
	if h.stop != nil {
		h.mu.Unlock()
		return
	}
	h.stop = make(chan struct{})
	stop := h.stop
	h.mu.Unlock()
	go func() {
		t := time.NewTicker(fishWatchEvery)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				h.watch()
			}
		}
	}()
}

// fishPayload is 0x0F02 with a sub-op, then what more writes.
func fishPayload(sub byte, more func(w *buffer.Writer)) []byte {
	return build(OpcodeFishing, 8+4096, func(w *buffer.Writer) {
		w.WriteByte(sub)
		if more != nil {
			more(w)
		}
	})
}

func fishSend(sess *session.Session, sub byte, more func(w *buffer.Writer)) {
	queueReliable(sess, OpcodeFishing, fishPayload(sub, more))
}

// catchPayload is 0x0F02 7: the catch as VIItemList::Read reads it (the
// zigzag count, the list's counter, then each record), each record keyed by
// its place in the list.
func catchPayload(catch []database.CharInventoryEntry) []byte {
	return fishPayload(fishLoot, func(w *buffer.Writer) {
		w.Write7BitEncodedInt64(int64(len(catch)))
		w.WriteUint32(uint32(len(catch)))
		for i := range catch {
			writeSingleItem(w, &catch[i], i)
		}
	})
}

// caughtPayload is 0x0F02 4: the catch at slot into the inventory (Caught
// reads the second word and keeps nothing of it: the count taken here).
func caughtPayload(slot, n int) []byte {
	return fishPayload(fishCaught, func(w *buffer.Writer) {
		w.WriteUint32(uint32(slot))
		w.WriteUint32(uint32(n))
	})
}

func fishSay(sess *session.Session, text string) {
	SendColoredChat(sess, text, scribeRefusalRGB[0], scribeRefusalRGB[1], scribeRefusalRGB[2])
}

// gear is what the angler wears for fishing, out of its inventory rows: the
// pole in its hand, the bait, and the fish stat of everything worn.
type fishGear struct {
	pole, bait *database.CharInventoryEntry
	fish       int
}

func wornFishingGear(items []database.CharInventoryEntry) fishGear {
	var g fishGear
	for i := range items {
		e := &items[i]
		if e.EquipLoc < 0 || e.Location != 0 {
			continue
		}
		switch {
		case e.Pattern.EquipSlot == wearFishingPole && e.EquipLoc == equipPole:
			g.pole = e
		case e.Pattern.EquipSlot == wearBait && g.bait == nil:
			g.bait = e // CLIENT's HasEquipped(21): any slot
		}
		if e.Pattern.Fish != nil {
			g.fish += *e.Pattern.Fish
		}
	}
	return g
}

func (h *FishingHandler) gearOf(char uint32) (fishGear, error) {
	if h.inv == nil {
		return fishGear{}, fmt.Errorf("no inventory")
	}
	items, err := h.inv.GetCharacterItems(char)
	if err != nil {
		return fishGear{}, err
	}
	return wornFishingGear(items), nil
}

// StartFishing opens a fishing session for the session's player, or says why
// not. It reports whether the window was asked for.
func (h *FishingHandler) StartFishing(sess *session.Session) bool {
	if h == nil || h.view == nil {
		return false
	}
	v, ok := h.view(sess, h.now())
	if !ok {
		return false
	}
	switch {
	case v.Dead:
		fishSay(sess, fishDead)
		return false
	case v.Fighting:
		fishSay(sess, fishFighting)
		return false
	}
	g, err := h.gearOf(v.CharServerID)
	if err != nil {
		log.Printf("Fishing: %s: %v", v.Name, err)
		return false
	}
	if g.pole == nil {
		fishSay(sess, fishNoPole)
		return false
	}
	skill := 0
	if h.chars != nil {
		if s, err := h.chars.FishingSkill(v.CharServerID); err == nil {
			skill = s
		}
	}
	h.mu.Lock()
	h.active[sess] = &fishState{sess: sess, char: v.CharServerID, skill: skill}
	h.mu.Unlock()
	fishSend(sess, fishStart, nil)
	log.Printf("Fishing: %s starts (skill %d) at (%.0f, %.0f) world %d", v.Name, skill, v.X, v.Z, v.World)
	return true
}

// stopLocked ends a session and closes the client's window. Must be called
// with h.mu held.
func (h *FishingHandler) stopLocked(sess *session.Session, why string) {
	if _, ok := h.active[sess]; !ok {
		return
	}
	delete(h.active, sess)
	fishSend(sess, fishStop, nil)
	log.Printf("Fishing: %s stopped: %s", sess.RemoteAddr(), why)
}

// watch stops the anglers who died, fight, or are gone.
func (h *FishingHandler) watch() {
	if h.view == nil {
		return
	}
	now := h.now()
	h.mu.Lock()
	defer h.mu.Unlock()
	for sess, st := range h.active {
		v, ok := h.view(sess, now)
		switch {
		case !ok || v.CharServerID != st.char:
			delete(h.active, sess) // gone, or another character on it: nothing to close
		case v.Dead:
			h.stopLocked(sess, "dead")
		case v.Fighting:
			fishSay(sess, fishFighting)
			h.stopLocked(sess, "fighting")
		}
	}
}

// HandleFishing is 0x0F01.
func (h *FishingHandler) HandleFishing(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 1 {
		return nil
	}
	r := buffer.NewReader(data[1:])
	switch data[0] {
	case fishCast:
		q, err := r.ReadUint32()
		if err != nil {
			return nil
		}
		h.cast(sess, q)
	case fishCancel:
		h.mu.Lock()
		if _, ok := h.active[sess]; ok {
			h.stopLocked(sess, "the client's cancel")
		} else {
			// a Start the client could not open, or one from before a restart:
			// its window closes all the same
			fishSend(sess, fishStop, nil)
		}
		h.mu.Unlock()
	case fishReel:
		h.takeAll(sess)
	case fishPull:
		h.mu.Lock()
		if st, ok := h.active[sess]; ok {
			st.catch, st.keys = nil, nil
			fishSend(sess, fishClose, nil)
		}
		h.mu.Unlock()
	case fishKeep:
		key, e1 := r.ReadUint32()
		_, e2 := r.ReadUint32()
		if e1 != nil || e2 != nil {
			return nil
		}
		h.keep(sess, key)
	default:
		log.Printf("Fishing: %s: sub-op %d unknown", sess.RemoteAddr(), data[0])
	}
	return nil
}

// biteChance is the percent a cast of that quality brings a bite, for an
// angler of that skill.
func biteChance(quality uint32, skill int) int {
	base := 40
	switch quality {
	case 0:
		base = 90
	case 5:
		base = 70
	}
	return min(95, base+skill/5)
}

// lostChance is the percent of bites that get away with the bait.
func lostChance(skill int) int {
	return max(5, fishLostPercent-skill/10)
}

// cast is 0x0F01 1: the line goes in, and its answer comes in a few seconds.
func (h *FishingHandler) cast(sess *session.Session, quality uint32) {
	h.mu.Lock()
	st, ok := h.active[sess]
	if !ok {
		h.mu.Unlock()
		fishSend(sess, fishStop, nil) // no session here: close the window
		return
	}
	if st.cast != 0 || len(st.catch) > 0 {
		h.mu.Unlock()
		log.Printf("Fishing: %s cast with a line in or a catch up: not answered", sess.RemoteAddr())
		return
	}
	h.casts++
	st.cast = h.casts
	id := st.cast
	st.casted = h.now()
	h.mu.Unlock()

	wait := fishBiteMin + time.Duration(h.rnd(int(fishBiteSpread/time.Millisecond)+1))*time.Millisecond
	h.after(wait, func() { h.bite(sess, id, quality) })
}

// bite answers the cast in the water, if it still is.
func (h *FishingHandler) bite(sess *session.Session, id int, quality uint32) {
	h.mu.Lock()
	defer h.mu.Unlock()
	st, ok := h.active[sess]
	if !ok || st.cast != id {
		return // cancelled, or another session now
	}
	st.cast = 0
	missed := func(why string) {
		fishSend(sess, fishMissed, nil)
		log.Printf("Fishing: %s's cast: %s", sess.RemoteAddr(), why)
	}
	if h.view == nil {
		missed("no world")
		return
	}
	v, ok := h.view(sess, h.now())
	switch {
	case !ok || v.CharServerID != st.char:
		delete(h.active, sess)
		return
	case v.Dead:
		h.stopLocked(sess, "dead")
		return
	case v.Fighting:
		fishSay(sess, fishFighting)
		h.stopLocked(sess, "fighting")
		return
	}
	g, err := h.gearOf(st.char)
	if err != nil {
		missed(err.Error())
		return
	}
	switch {
	case g.pole == nil:
		fishSay(sess, fishNoPole)
		h.stopLocked(sess, "no pole")
		return
	case g.bait == nil:
		fishSay(sess, fishNoBait)
		missed("no bait")
		return
	case g.pole.Pattern.MaxHP > 0 && g.pole.RemainHP <= 0:
		missed("a broken pole")
		return
	case v.Water == entity.FishWaterNone:
		fishSay(sess, fishNoWater)
		missed(fmt.Sprintf("no water at (%.1f, %.1f)", v.LandX, v.LandZ))
		return
	}
	skill := min(fishSkillMax, st.skill+g.fish)
	rows := h.catchRows(v, skill)
	if len(rows) == 0 {
		// nothing to come up here: no bait taken
		fishSay(sess, fishNothingYet)
		missed(fmt.Sprintf("no fishcatch row at (%.0f, %.0f) world %d", v.LandX, v.LandZ, v.World))
		return
	}
	if h.rnd(100) >= biteChance(quality, skill) {
		missed(fmt.Sprintf("nothing bit (quality %d, skill %d)", quality, skill))
		return
	}
	// a bite: the bait is gone whatever comes of it
	h.takeBait(sess, st.char, g.bait)
	roll := h.rnd(100)
	switch {
	case roll < lostChance(skill):
		fishSend(sess, fishLost, nil)
		log.Printf("Fishing: %s's fish got away", v.Name)
		return
	case g.pole.Pattern.MaxHP > 0 && roll < lostChance(skill)+fishSnapPercent:
		h.wearPole(sess, st.char, g.pole)
		fishSend(sess, fishDamaged, nil)
		log.Printf("Fishing: %s's line snapped on the pole", v.Name)
		return
	}
	catch := h.rollCatch(rows)
	if len(catch) == 0 {
		missed("the catch's items are not in itempattern")
		return
	}
	st.catch = catch
	st.keys = make([]uint32, len(catch))
	for i := range catch {
		st.keys[i] = uint32(i)
	}
	queueReliable(sess, OpcodeFishing, catchPayload(catch))
	names := make([]string, len(catch))
	for i := range catch {
		names[i] = fmt.Sprintf("%d %q", catch[i].StackLeft, catch[i].Pattern.ItemName)
	}
	log.Printf("Fishing: %s caught %v (quality %d, skill %d)", v.Name, names, quality, skill)
	if st.skill < fishSkillMax && h.rnd(100) < (fishSkillMax-st.skill)/fishGainDivisor {
		st.skill++
		if h.chars != nil {
			if err := h.chars.SetFishingSkill(st.char, st.skill); err != nil {
				log.Printf("Fishing: %s's skill: %v", v.Name, err)
			}
		}
		fishSayGood(sess, fmt.Sprintf(fishSkillUp, st.skill))
	}
}

// fishSayGood is a line in the colour of good news.
func fishSayGood(sess *session.Session, text string) {
	SendColoredChat(sess, text, 255, 255, 63)
}

// catchRows is the rows of fishcatch that apply where the line went in, now,
// for this skill.
func (h *FishingHandler) catchRows(v entity.FishingView, skill int) []database.FishCatch {
	if h.db == nil {
		return nil
	}
	rows, err := h.db.FishCatches(v.World)
	if err != nil {
		log.Printf("Fishing: fishcatch: %v", err)
		return nil
	}
	month := int(h.now().Month())
	var apply []database.FishCatch
	for _, c := range rows {
		if c.Applies(v.World, v.LandX, v.LandZ, skill, month) {
			apply = append(apply, c)
		}
	}
	return apply
}

// rollCatch picks what came up: one row by weight, and now and then a
// second, another row. The records are new items at full condition.
func (h *FishingHandler) rollCatch(apply []database.FishCatch) []database.CharInventoryEntry {
	if h.items == nil {
		return nil
	}
	pick := func(skip int) (database.FishCatch, bool) {
		sum := 0
		for _, c := range apply {
			if c.ID != skip {
				sum += c.Weight
			}
		}
		if sum <= 0 {
			return database.FishCatch{}, false
		}
		n := h.rnd(sum)
		for _, c := range apply {
			if c.ID == skip {
				continue
			}
			if n < c.Weight {
				return c, true
			}
			n -= c.Weight
		}
		return database.FishCatch{}, false
	}
	var out []database.CharInventoryEntry
	add := func(c database.FishCatch) {
		p, err := h.items.GetItemPattern(c.ItemID)
		if err != nil {
			log.Printf("Fishing: fishcatch row %d: item %d: %v", c.ID, c.ItemID, err)
			return
		}
		n := c.Count
		if p.MaxStack < 2 {
			n = 1 // a record of an item that does not stack holds one
		} else {
			n = min(n, p.MaxStack)
		}
		out = append(out, database.CharInventoryEntry{
			StackLeft: n, RemainHP: p.MaxHP, EquipLoc: -1, Location: -1, Pattern: *p,
		})
	}
	first, ok := pick(-1)
	if !ok {
		return nil
	}
	add(first)
	if len(out) == 1 && h.rnd(100) < fishSecondChance {
		if second, ok := pick(first.ID); ok {
			add(second)
		}
	}
	return out
}

// takeBait takes one of the worn bait, in the database, the server's copy
// of the client's list, and the client's (0x003C).
func (h *FishingHandler) takeBait(sess *session.Session, char uint32, bait *database.CharInventoryEntry) {
	l := sessionInvList(sess)
	if l == nil || h.inv == nil {
		return
	}
	idx := l.indexOfItem(bait.ItemID)
	if idx < 0 {
		log.Printf("Fishing: the bait (item %d) is not in the client's list", bait.ItemID)
		return
	}
	var err error
	if bait.StackLeft <= 1 {
		err = h.inv.DeleteCharItem(char, bait.ItemID)
	} else {
		err = h.inv.SetItemStack(char, bait.ItemID, bait.StackLeft-1)
	}
	if err != nil {
		log.Printf("Fishing: taking a bait: %v", err)
		return
	}
	l.took(idx, 1)
	queueReliable(sess, protocol.OpcodeRemoveInvItem, removeInvItemPayload(idx, 0, 1))
}

// wearPole takes a tenth of the pole's condition (0x0060 tells the client).
func (h *FishingHandler) wearPole(sess *session.Session, char uint32, pole *database.CharInventoryEntry) {
	l := sessionInvList(sess)
	if l == nil || h.inv == nil {
		return
	}
	rec, ok := l.at(l.indexOfItem(pole.ItemID))
	if !ok {
		return
	}
	hp := max(0, pole.RemainHP-max(1, pole.Pattern.MaxHP/fishPoleWearDiv))
	if err := h.inv.SetItemRemainHP(char, pole.ItemID, hp); err != nil {
		log.Printf("Fishing: the pole's condition: %v", err)
		return
	}
	queueReliable(sess, protocol.OpcodeAdjustItemHP, adjustItemHPPayload(rec.key, hp))
}

// fileCatchLocked puts what of the catch at slot fits into the inventory,
// the way the client's Caught does (VIItemList::AddItem: onto the first
// stack of the item, else a new record, nothing when the list is full), and
// answers with Caught. It returns how many went in, and adds them to the
// quests' takes. Must be called with h.mu held.
func (h *FishingHandler) fileCatchLocked(sess *session.Session, st *fishState, slot int, takes *[]lootTake) (int, bool) {
	l := sessionInvList(sess)
	if l == nil || h.inv == nil || h.playerOf == nil || slot < 0 || slot >= len(st.catch) {
		return 0, false
	}
	p := h.playerOf(sess)
	if p == nil || p.CharServerID != st.char {
		return 0, false
	}
	rec := st.catch[slot]
	took := l.wouldAdd(rec.Pattern.PatternID, rec.StackLeft, rec.Pattern.MaxStack)
	if took > 0 {
		if _, _, err := (&NPCWindowHandler{inv: h.inv}).giveItem(p, l, rec, took, 0); err != nil {
			log.Printf("Fishing: filing %q: %v", rec.Pattern.ItemName, err)
			return 0, false
		}
	}
	queueReliable(sess, OpcodeFishing, caughtPayload(slot, took))
	// VIItemList::Took: the record leaves when none of it is left
	st.catch[slot].StackLeft -= took
	if st.catch[slot].StackLeft <= 0 {
		st.catch = append(st.catch[:slot], st.catch[slot+1:]...)
		st.keys = append(st.keys[:slot], st.keys[slot+1:]...)
	}
	if took > 0 {
		*takes = append(*takes, lootTake{rec.Pattern.PatternID, took})
	}
	return took, true
}

// tellQuests tells the quests what the catch put in the inventory, as a
// corpse's loot does (questloot.go). Called with h.mu not held.
func (h *FishingHandler) tellQuests(sess *session.Session, takes []lootTake) {
	if len(takes) == 0 || h.playerOf == nil {
		return
	}
	tellQuestsLooted(sess, h.playerOf(sess), takes)
}

// takeAll is 0x0F01 3, Take All: every record that fits, then the list
// closes.
func (h *FishingHandler) takeAll(sess *session.Session) {
	var takes []lootTake
	defer func() { h.tellQuests(sess, takes) }() // after the unlock below
	h.mu.Lock()
	defer h.mu.Unlock()
	st, ok := h.active[sess]
	if !ok {
		return
	}
	full := false
	for i := 0; i < len(st.catch); {
		qty := st.catch[i].StackLeft
		took, ok := h.fileCatchLocked(sess, st, i, &takes)
		if !ok {
			break
		}
		if took != qty {
			full = true
			i++ // what did not fit stays on the list at i
		}
	}
	if full {
		fishSay(sess, fishFull)
	}
	st.catch, st.keys = nil, nil
	fishSend(sess, fishClose, nil)
}

// keep is 0x0F01 5: the record of that key into the inventory; the list
// closes once it is empty.
func (h *FishingHandler) keep(sess *session.Session, key uint32) {
	var takes []lootTake
	defer func() { h.tellQuests(sess, takes) }() // after the unlock below
	h.mu.Lock()
	defer h.mu.Unlock()
	st, ok := h.active[sess]
	if !ok {
		return
	}
	slot := -1
	for i, k := range st.keys {
		if k == key {
			slot = i
			break
		}
	}
	if slot < 0 {
		// the client waits (state 4) for an answer: back to its list, or,
		// with no catch up at all, to casting
		if len(st.catch) == 0 {
			fishSend(sess, fishReady, nil)
		} else {
			fishSend(sess, fishWaiting, nil)
		}
		log.Printf("Fishing: %s kept key %d, which the catch does not hold", sess.RemoteAddr(), key)
		return
	}
	qty := st.catch[slot].StackLeft
	took, ok := h.fileCatchLocked(sess, st, slot, &takes)
	if !ok {
		fishSend(sess, fishWaiting, nil)
		return
	}
	if took != qty {
		fishSay(sess, fishFull)
	}
	if len(st.catch) == 0 {
		fishSend(sess, fishClose, nil)
	}
}
