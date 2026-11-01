package handlers

import (
	"log"
	"slices"
	"sync"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// handswitch.go — the hands an art wants, put in them by the server (the
// user, 2026-11-01: "fix rangers, they can dual wield and switch to a bow").
//
// CLIENT's own equip table (equipDisplaces, Msg003F) never lets a bow in the
// ranged slot be worn beside a weapon: equipping one takes the other off,
// and were both worn, GetSwing would swing one Attack1. So a ranger holding
// its blades had every arrow art refused for its required equipment (the
// workbook's Range, entity/workbookmore.go), and one holding its bow every
// blade art. A cast its required equipment alone would refuse (entity's
// CastWantsHands: its target, range, power, lockout and the arts' cooldown
// all fine, its reagents carried) now has its hands switched first, from the
// pack (never the bank), the way the item menu's equip switches them
// (planEquipInto, applyPlan: the database changed and 0x003F to the client,
// which takes off what that displaces itself), and goes ahead: never "You do
// not have the right weapon for that." with a switch made. Ours, the art's
// requirement deciding, any class's:
//
//   - Range: the ranged weapon it last held, else its best by DMG (a bow, a
//     crossbow, a thrown weapon; never a thrown potion), the hands taken off
//     as a bow's equip takes them (a thrown weapon's equip takes nothing off,
//     so the server does: 0x0040 each).
//   - A weapon's code (1HS 2HS 1HB 2HB 1HP 2HP): the hands a switch last took
//     off when they fit and are still carried, else the best fitting weapon
//     by DMG in the main hand and, beside a one-hander, the best other
//     fitting one-hander in the off hand for a class that may hold a second
//     weapon (not a knight: offHandWeaponBarred).
//   - None (a monk's punches): the hands taken off.
//   - Shield: the best shield by AC.
//
// An item goes on only where its wearer may wear it (cannotWear: the class,
// the race, the level). What a switch takes off the hands is remembered by
// character, in memory, for the way back, and so is the ranged weapon last
// held (any change of hands the server makes). With nothing carried that
// meets the requirement there is no switch, and the cast is refused as
// before. The switch's hands are checked before they are made: they must
// meet the requirement as the cast will check it (wornLook, entity's
// HandsFit). A fight in progress swings with them from its next round
// (handsChanged).

// keptHands is what the switches remember of a character's hands: the item
// ids a switch took off the main hand and the off hand (0 none), and the
// ranged weapon last held.
type keptHands struct {
	main, off int
	ranged    int
}

// handMemory is keptHands by character (CharServerID), in memory: a restart
// forgets them.
type handMemory struct {
	mu sync.Mutex
	by map[uint32]keptHands
}

func (m *handMemory) get(sid uint32) keptHands {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.by[sid]
}

func (m *handMemory) update(sid uint32, fn func(*keptHands)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.by == nil {
		m.by = map[uint32]keptHands{}
	}
	k := m.by[sid]
	fn(&k)
	m.by[sid] = k
}

// The wear slots of the hands' items (itempattern.equipslot; equip.go).
const (
	wearPrimary = 12
	wearShield  = 13
	wearTwoHand = 15
	wearBow     = 16
	wearThrown  = 17
	equipRanged = 18
)

// handSwitch is one switch: the equip slots the server empties first (what
// the client's own equip would not), then the items put on, in order.
type handSwitch struct {
	what string // the requirement it meets, for the log
	off  []int
	on   []switchOn
}

type switchOn struct {
	itemID, slot int
}

// SwitchHandsFor puts in the player's hands what a cast's required
// equipment names (the file's doc). It reports whether it switched them.
func (h *EquipHandler) SwitchHandsFor(sess *session.Session, player *entity.Player, req string) bool {
	if h.inventoryStore == nil || player == nil {
		return false
	}
	sid := player.CharServerID
	items, err := h.inventoryStore.GetCharacterItems(sid)
	if err != nil {
		log.Printf("Hands: %s's items: %v", player.Name, err)
		return false
	}
	sw, ok := h.planSwitch(items, player, req)
	if !ok {
		log.Printf("Hands: %s carries nothing it may wear that meets %q", player.Name, req)
		return false
	}
	// the hands it takes off, for the way back (a ranged weapon it puts on is
	// applyPlan's to keep, after these)
	before := handsWorn(items)
	h.hands.update(sid, func(k *keptHands) {
		if before.main != 0 || before.off != 0 {
			k.main, k.off = before.main, before.off
		}
		if before.ranged != 0 {
			k.ranged = before.ranged
		}
	})
	done := h.applySwitch(sess, sid, items, sw)
	// what it did is the game's, all or part (a database refusal stops it)
	h.wornChanged(sess, player, sid)
	if done {
		log.Printf("Hands: %s's hands switched for %q (%s): took off %d/%d/%d", player.Name, req, sw.what, before.main, before.off, before.ranged)
	}
	return done
}

// applySwitch makes the switch: the slots it empties (0x0040 each), then each
// item put on as the item menu's equip puts one on (planEquipInto, 0x003F),
// the hands read again between (what one displaced is off). False when the
// database refused a change or an item left the list.
func (h *EquipHandler) applySwitch(sess *session.Session, sid uint32, items []database.CharInventoryEntry, sw handSwitch) bool {
	for _, slot := range sw.off {
		for _, e := range inventoryRows(items) {
			if e.EquipLoc != slot {
				continue
			}
			plan := equipPlan{name: e.Pattern.ItemName, unequip: slot, changes: []equipChange{{e.ItemID, -1}}}
			if !h.applyPlan(sess, sid, plan, "Hands") {
				return false
			}
		}
	}
	for _, o := range sw.on {
		items, err := h.inventoryStore.GetCharacterItems(sid)
		if err != nil {
			log.Printf("Hands: serverID=%d's items: %v", sid, err)
			return false
		}
		var e *database.CharInventoryEntry
		for _, r := range inventoryRows(items) {
			if r.ItemID == o.itemID {
				e = r
			}
		}
		idx := listIndexOf(items, sessionInvList(sess), o.itemID)
		if e == nil || idx < 0 || !fitsEquip(e.Pattern.EquipSlot, o.slot) {
			log.Printf("Hands: serverID=%d's item %d is gone from its list", sid, o.itemID)
			return false
		}
		if !h.applyPlan(sess, sid, planEquipInto(inventoryRows(items), idx, e, o.slot), "Hands") {
			return false
		}
	}
	return true
}

// wornChanged puts a change of the hands into the game: the look and the
// gear (applyEquipmentFromDB) and a fight's swing (handsChanged). (The
// ranged weapon put on is kept for the switches by applyPlan.)
func (h *EquipHandler) wornChanged(sess *session.Session, player *entity.Player, sid uint32) {
	if h.charHandler != nil {
		h.charHandler.applyEquipmentFromDB(player, sid, false)
	}
	handsChanged(h.gameWorld, h.inventoryStore, sess, player)
}

// handsWorn is the hands as worn: the main hand's item (16, 14, 19), the off
// hand's (15, 17, 20) and the ranged weapon (18), by item id, 0 none.
func handsWorn(items []database.CharInventoryEntry) keptHands {
	inv := inventoryRows(items)
	var k keptHands
	if e := wornIn(inv, []int{16, 14, 19}); e != nil {
		k.main = e.ItemID
	}
	if e := wornIn(inv, offHandSlots); e != nil {
		k.off = e.ItemID
	}
	if e := wornIn(inv, []int{equipRanged}); e != nil {
		k.ranged = e.ItemID
	}
	return k
}

// planSwitch is the switch that meets the requirement (the file's doc): the
// first of its kinds, in the order it names them, that something carried
// meets, checked as the cast will check it.
func (h *EquipHandler) planSwitch(items []database.CharInventoryEntry, player *entity.Player, req string) (handSwitch, bool) {
	kept := h.hands.get(player.CharServerID)
	var types []byte
	var kinds []string
	for _, code := range entity.ReqEquipCodes(req) {
		kind := code
		switch code {
		case "RANGE", "RANGED":
			kind = "RANGE"
		case "SHIELD", "NONE":
		default:
			t := entity.EquipCodeTypes(code)
			if len(t) == 0 {
				continue // ANY, or a code no weapon holds
			}
			types = append(types, t...)
			kind = "MELEE"
		}
		if !slices.Contains(kinds, kind) {
			kinds = append(kinds, kind)
		}
	}
	for _, kind := range kinds {
		var plans []handSwitch
		switch kind {
		case "RANGE":
			plans = h.rangedSwitch(items, player, kept)
		case "MELEE":
			plans = meleeSwitch(items, player, kept, types)
		case "SHIELD":
			plans = shieldSwitch(items, player)
		case "NONE":
			plans = bareSwitch(items)
		}
		for _, sw := range plans {
			if switchFits(items, sw, req) {
				return sw, true
			}
		}
	}
	return handSwitch{}, false
}

// carried is the pack's items (not the bank) the player may wear, of the
// wear slots given, that pass keep.
func carried(items []database.CharInventoryEntry, player *entity.Player, wears []int, keep func(*database.CharInventoryEntry) bool) []*database.CharInventoryEntry {
	var out []*database.CharInventoryEntry
	for _, e := range inventoryRows(items) {
		if slices.Contains(wears, e.Pattern.EquipSlot) && cannotWear(player, &e.Pattern) == "" && (keep == nil || keep(e)) {
			out = append(out, e)
		}
	}
	return out
}

// best is the item of the most worth, the first of them on a tie (the
// client's list order: the database's).
func best(items []*database.CharInventoryEntry, worth func(*database.CharInventoryEntry) int) *database.CharInventoryEntry {
	var b *database.CharInventoryEntry
	for _, e := range items {
		if b == nil || worth(e) > worth(b) {
			b = e
		}
	}
	return b
}

func byDMG(e *database.CharInventoryEntry) int { return e.Pattern.WeaponDamage }

// rangedSwitch is the switch to a ranged weapon: the one last held when it
// is carried, else the best by DMG, the hands taken off.
func (h *EquipHandler) rangedSwitch(items []database.CharInventoryEntry, player *entity.Player, kept keptHands) []handSwitch {
	ranged := carried(items, player, []int{wearBow, wearThrown}, func(e *database.CharInventoryEntry) bool {
		at := e.Pattern.AttackType
		return (at == 7 || at == 9 || at == 10) && !thrownPotion(h.gameWorld, h.inventoryStore, &e.Pattern)
	})
	pick := best(ranged, byDMG)
	for _, e := range ranged {
		if e.ItemID == kept.ranged {
			pick = e
		}
	}
	if pick == nil {
		return nil
	}
	// what the equip does not take off itself (a thrown weapon's: nothing)
	var off []int
	displaced := equipDisplaces(pick.Pattern.EquipSlot, equipRanged)
	for _, s := range []int{14, 15, 16, 17, 19, 20} {
		if !slices.Contains(displaced, s) {
			off = append(off, s)
		}
	}
	return []handSwitch{{what: "ranged " + pick.Pattern.ItemName, off: off, on: []switchOn{{pick.ItemID, equipRanged}}}}
}

// meleeSwitch is the switches to a weapon of the attack types: the hands a
// switch took off, then the best by DMG (and a second one-hander).
func meleeSwitch(items []database.CharInventoryEntry, player *entity.Player, kept keptHands, types []byte) []handSwitch {
	var out []handSwitch
	inv := inventoryRows(items)
	find := func(id int) *database.CharInventoryEntry {
		for _, e := range inv {
			if id != 0 && e.ItemID == id && cannotWear(player, &e.Pattern) == "" {
				return e
			}
		}
		return nil
	}
	if main := find(kept.main); main != nil {
		sw := handSwitch{what: "the hands it held", on: []switchOn{{main.ItemID, mainSlotFor(main)}}}
		if off := find(kept.off); off != nil && off.ItemID != main.ItemID && mainSlotFor(main) != 16 &&
			!(off.Pattern.EquipSlot == wearEitherHand && offHandWeaponBarred(player.Class)) {
			sw.on = append(sw.on, switchOn{off.ItemID, offSlotFor(off)})
		}
		out = append(out, sw)
	}
	fits := func(e *database.CharInventoryEntry) bool { return slices.Contains(types, byte(e.Pattern.AttackType)) }
	main := best(carried(items, player, []int{wearPrimary, wearEitherHand, wearTwoHand}, fits), byDMG)
	if main == nil {
		return out
	}
	sw := handSwitch{what: main.Pattern.ItemName, on: []switchOn{{main.ItemID, mainSlotFor(main)}}}
	if main.Pattern.EquipSlot != wearTwoHand && !offHandWeaponBarred(player.Class) {
		second := best(carried(items, player, []int{wearEitherHand}, func(e *database.CharInventoryEntry) bool {
			return e.ItemID != main.ItemID && fits(e)
		}), byDMG)
		if second != nil {
			sw.on = append(sw.on, switchOn{second.ItemID, equipSecondary})
			sw.what += " and " + second.Pattern.ItemName
		}
	}
	return append(out, sw)
}

// shieldSwitch is the switch to the best shield by AC.
func shieldSwitch(items []database.CharInventoryEntry, player *entity.Player) []handSwitch {
	ac := func(e *database.CharInventoryEntry) int {
		if e.Pattern.AC == nil {
			return 0
		}
		return *e.Pattern.AC
	}
	s := best(carried(items, player, []int{wearShield}, nil), ac)
	if s == nil {
		return nil
	}
	return []handSwitch{{what: s.Pattern.ItemName, on: []switchOn{{s.ItemID, 17}}}}
}

// bareSwitch is the switch to bare hands: the weapons taken off, then the
// items held too where they are what keeps the hands from being bare.
func bareSwitch(items []database.CharInventoryEntry) []handSwitch {
	return []handSwitch{{what: "bare hands", off: []int{14, 15, 16, 18}}, {what: "bare hands", off: []int{14, 15, 16, 18, 19, 20}}}
}

// switchFits is whether the switch's hands would meet the requirement as the
// cast checks it: its plan played on a copy of the rows (the client's own
// equip table included, planEquipInto), the look the hands would have
// (wornLook), and HandsFit; and whether it changes anything.
func switchFits(items []database.CharInventoryEntry, sw handSwitch, req string) bool {
	rows := append([]database.CharInventoryEntry(nil), items...)
	changed := false
	for _, slot := range sw.off {
		for i := range rows {
			if rows[i].Location != 1 && rows[i].EquipLoc == slot {
				rows[i].EquipLoc, changed = -1, true
			}
		}
	}
	for _, o := range sw.on {
		inv := inventoryRows(rows)
		var e *database.CharInventoryEntry
		for _, r := range inv {
			if r.ItemID == o.itemID {
				e = r
			}
		}
		if e == nil || !fitsEquip(e.Pattern.EquipSlot, o.slot) {
			return false
		}
		for _, c := range planEquipInto(inv, 0, e, o.slot).changes {
			for i := range rows {
				if rows[i].ItemID == c.itemID {
					rows[i].EquipLoc, changed = c.equipLoc, true
				}
			}
		}
	}
	var p entity.Player
	wornLook(&p, rows)
	return changed && entity.HandsFit(&p, req)
}
