package handlers

import (
	"errors"
	"fmt"
	"log"
	"slices"
	"sort"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// mercinv.go — what a merc holds (the owner, 2026-11-15: "lets add inventory
// to mercs same as players, can equip or hold items. that is also a way to
// switch pots on an alc merc. the alc mercs should constantly make pots and
// they are saved for future mercs also"; and "if player chooses cocktail path
// should have a toggle for aoe cocktail thrown pots or the single target pots
// like the knack alc").
//
// A merc holds up to database.MercItemsMax records (table charmercitem), each
// carried or worn. CLIENT has no window for a merc's items, so they are moved
// through the merc's menu (mercmenu.go: Items) and /merc:
//
//   - /merc [name] give [item]: the player's pack's first record of that name
//     (whole; no name, the Give box: the pack's records it may take, five a
//     box). A worn item comes off first by the item menu, as for a trade; a
//     No Rent item that vanishes at logout (norentvanish) is refused (it
//     would outlive its rule there). A NO TRADE item may go (it never leaves
//     its owner: his merc holds it and gives it back); a LORE item too, but
//     it comes back only while its owner holds none (else it stays with the
//     merc, or in the keeping, until he does).
//   - /merc [name] inv: what it holds (the Inventory box, each record its
//     Equip or Unequip and Take back), /merc [name] equip|unequip|take
//     <item>.
//   - /merc [name] use <item>, the Inventory box's Use and vanilla's window's
//     (petition 3285): a meal or a grocer's drink it carries eaten or drunk
//     now, its effect on the merc as its own turn would put it
//     (entity.MercUse), whatever its health or power.
//   - /merc [name] stock: an alchemist's owner's potions made (entity/
//     mercbrew.go), each taken into the pack a stack at a time. A potion of
//     the stock is the player's as any he made: its NO TRADE and LORE flags
//     hold in his pack as they would for one summoned (a LORE potion comes
//     only while he holds none).
//   - /merc [name] aoe | single: a Cocktail alchemist's throws (its area
//     cocktails, or its single-target potions: the default), kept in its row.
//   - /merc claim: what came back from a merc whose hire ended and waited
//     for room in the pack.
//
// Worn items are a player's: the class, race and level gates (cannotWear),
// the slots and what each takes off (equipSlotsFor, heldSlot,
// eitherHandSlot, planEquipInto: a two-hander takes off the main hand, the
// off hand and the shield; a knight's off hand holds no weapon). They stand
// over its heirlooms: its kit (merc.go) is its heirlooms with each slot its
// own item fills (and the slots that item's wear takes off) its item's, so
// what it wears adds to its stats, AC and swing, and shows on its channel,
// through the player's own gear and look (wornGear, wornLook). What it
// carries it uses up: potions and draughts (an alchemist's), meals and
// grocers' drinks (every merc's: entity/mercmeal.go, petition 3061).
//
// Never lost: a record goes from the pack to the merc, and back, in one
// transaction each (database/mercitems.go). A hire that ends (a dismissal,
// a group leader's boot, a hire over its slot) puts what it held in the
// keeping in the transaction that ends it, and the keeping comes into the
// pack as it has room (said, then at each login, /merc inv and /merc claim).
// A merc that dies keeps what it holds: its hire goes on, as a player keeps
// his gear at a death.

// The lines (ours).
const (
	mercInvNone      = "%s holds nothing."
	mercInvHead      = "%s holds %d of %d:"
	mercGiveNone     = "You have no %q in your pack."
	mercGiveWorn     = "Take %s off first."
	mercGiveVanish   = "%s vanishes when you log out: your mercenary will not hold it."
	mercGiveFull     = "%s holds all it can (%d)."
	mercGiveLine     = "You give %s to %s."
	mercTakeLine     = "%s gives you %s."
	mercTakeNone     = "%s holds no %q."
	mercPackFull     = "Your pack has no room for %s."
	mercLoreBack     = "You hold a %s already: %s keeps it until you have none."
	mercEquipNot     = "%s cannot wear %s."
	mercEquipLine    = "%s wears %s."
	mercUnequipLine  = "%s takes off %s."
	mercKeptLine     = "%d of your mercenaries' items wait for room in your pack: /merc claim takes them."
	mercClaimLine    = "%s comes back to your pack from the Mercenary Recruitment board."
	mercStockNone    = "Your alchemists have made no potions yet."
	mercStockHead    = "Your alchemists' potions (%d of %d):"
	mercStockTake    = "You take %d %s from your alchemists' stock."
	mercThrowsNot    = "Only an alchemist of the Cocktail path throws area cocktails."
	mercThrowsAoE    = "%s throws its area cocktails at what fights you, and only at that."
	mercThrowsSingle = "%s throws its single-target potions."
	mercGiveAsk      = "Give %s which item? (%d of %d)"
	mercInvAsk       = "%s holds (%d of %d):"
	mercItemAsk      = "%s: %s"
	mercStockAsk     = "Your alchemists' potions (%d of %d): take a stack."
	mercItemsAsk     = "%s's items:"
	mercUseEat       = "%s eats %s."
	mercUseDrink     = "%s drinks %s."
	mercUseNotOut    = "%s must be out with you to eat or drink."
	mercUseNot       = "%s cannot eat or drink %s."
	mercUseNoPower   = "%s has no power for %s to restore."
)

// mercInvPage is the records an items box lists (five and More).
const mercInvPage = 5

// mercLabel is an answer of a box cut to fit (an answer shows 63 characters).
func mercLabel(s string) string {
	if r := []rune(s); len(r) > 60 {
		return string(r[:57]) + "..."
	}
	return s
}

// mercItemName is a record's name with its stack and where it is worn.
func mercItemName(it database.MercItem) string {
	s := it.Pattern.ItemName
	if it.Stack > 1 {
		s += fmt.Sprintf(" x%d", it.Stack)
	}
	if it.EquipLoc >= 0 {
		s += " (worn)"
	}
	return s
}

// mercWearer is a player of the merc's class, race and level: whom the
// player's gates ask (cannotWear).
func mercWearer(row database.MercRow, level int) *entity.Player {
	return &entity.Player{Class: row.Class, Race: row.Race, Level: max(level, 1)}
}

// itemsOver is the merc's heirlooms with its own worn items over them (the
// file's doc): each own item's slot, and the slots its wear takes off, no
// longer the heirlooms'.
func itemsOver(heir []database.CharInventoryEntry, own []database.CharInventoryEntry) []database.CharInventoryEntry {
	if len(own) == 0 {
		return heir
	}
	taken := map[int]bool{}
	for _, it := range own {
		taken[it.EquipLoc] = true
		for _, e := range equipDisplaces(it.Pattern.EquipSlot, it.EquipLoc) {
			taken[e] = true
		}
	}
	out := make([]database.CharInventoryEntry, 0, len(heir)+len(own))
	for _, it := range heir {
		// an heirloom whose own wear takes off a slot an own item fills goes
		// too (an heirloom staff beside an own shield)
		gone := taken[it.EquipLoc]
		for _, e := range equipDisplaces(it.Pattern.EquipSlot, it.EquipLoc) {
			for _, o := range own {
				gone = gone || o.EquipLoc == e
			}
		}
		if !gone {
			out = append(out, it)
		}
	}
	return append(out, own...)
}

// mercOwnItems is the merc's worn records as worn items (its figures for its
// race and level, an heirloom's: Heirlooms.ViewFor) and what it carries to
// use (entity.MercCarried).
func (m *Mercs) mercOwnItems(row database.MercRow, level int) (worn []database.CharInventoryEntry, carried []entity.MercCarried, err error) {
	if m.db == nil || row.ServerID == 0 {
		return nil, nil, nil
	}
	items, err := m.db.MercItems(row.ServerID, row.Slot)
	if err != nil {
		return nil, nil, err
	}
	for _, it := range items {
		p := it.Pattern
		if it.EquipLoc >= 0 {
			if m.heir != nil {
				m.heir.ViewForOwner(row.ServerID, row.Race, level, &p)
			}
			worn = append(worn, database.CharInventoryEntry{ItemID: -it.ID, Pattern: p, EquipLoc: it.EquipLoc, RemainHP: it.RemainHP, StackLeft: 1})
			continue
		}
		c := entity.MercCarried{ID: it.ID, PatternID: it.PatternID, Count: max(it.Stack, 1), Name: p.ItemName}
		if food, ok := consumableOf(&p); ok && !food.Drink {
			c.Food, c.HoT = true, food.Amount
			if isRation(&p) {
				c.HoT = 0 // a player's ration's (entity.RationRestore)
			}
		} else if ok {
			c.Drink, c.PoT = true, food.Amount // a grocer's drink, its power a tick (entity/mercmeal.go)
		}
		carried = append(carried, c)
	}
	return worn, carried, nil
}

// mercRowOf is the player's hire of the slot, said when there is none.
func (m *Mercs) mercRowOf(sess *session.Session, p *entity.Player, slot int) (database.MercRow, bool) {
	if m.db == nil {
		SendErrorMessage(sess, mercNoneLine)
		return database.MercRow{}, false
	}
	row, ok, err := m.db.Merc(p.CharServerID, slot)
	if err != nil || !ok {
		SendErrorMessage(sess, mercNoneLine)
		return database.MercRow{}, false
	}
	return row, true
}

// rekitSlot gives the merc of the slot out a kit made again from its row (its
// items changed), at its owner's level.
func (m *Mercs) rekitSlot(p *entity.Player, slot int) {
	row, ok, err := m.db.Merc(p.CharServerID, slot)
	if err != nil || !ok {
		return
	}
	kit, err := m.kit(row, p.Level)
	if err != nil {
		log.Printf("Merc: %s's %q's kit: %v", p.Name, row.Name, err)
		return
	}
	m.gw.SetMercKit(p.CharServerID, slot, kit)
}

// packRows is the player's pack's records (not the bank's), as the database
// has them.
func (m *Mercs) packRows(p *entity.Player) ([]database.CharInventoryEntry, error) {
	rows, err := database.NewInventoryStore(m.db).GetCharacterItems(p.CharServerID)
	if err != nil {
		return nil, err
	}
	out := rows[:0]
	for _, r := range rows {
		if r.Location == database.LocPack {
			out = append(out, r)
		}
	}
	return out, nil
}

// vanishes is whether the item vanishes at a logout (norentvanish).
func (m *Mercs) vanishes(patternID int) bool {
	var n int
	if err := m.db.QueryRow(`SELECT COUNT(*) FROM norentvanish WHERE patternid = ?`, patternID).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

// givable is the pack's records the merc may take (the file's doc), in the
// pack's order.
func (m *Mercs) givable(p *entity.Player) []database.CharInventoryEntry {
	rows, err := m.packRows(p)
	if err != nil {
		return nil
	}
	var out []database.CharInventoryEntry
	for _, r := range rows {
		if r.EquipLoc < 0 && !m.vanishes(r.Pattern.PatternID) {
			out = append(out, r)
		}
	}
	return out
}

// giveItem gives the merc of the slot the pack's record itemID, whole: off
// the pack (the client told, 0x003C), onto the merc, its kit made again.
func (m *Mercs) giveItem(sess *session.Session, p *entity.Player, row database.MercRow, itemID int) bool {
	l := sessionInvList(sess)
	if l == nil {
		return false
	}
	idx := -1
	var rec invRec
	for i := 0; ; i++ {
		r, ok := l.at(i)
		if !ok {
			break
		}
		if r.itemID == itemID {
			idx, rec = i, r
			break
		}
	}
	if idx < 0 {
		SendErrorMessage(sess, "That is no longer in your pack.")
		return false
	}
	inv, err := database.NewInventoryStore(m.db).GetCharItem(p.CharServerID, itemID)
	if err != nil {
		SendErrorMessage(sess, "That is no longer in your pack.")
		return false
	}
	n := min(rec.qty, max(inv.StackLeft, 1))
	if line := m.giveN(sess, p, row, l, idx, inv, n); line != "" {
		SendErrorMessage(sess, line)
		return false
	}
	SendClientMessage(sess, fmt.Sprintf(mercGiveLine, inv.Pattern.ItemName, row.Name))
	m.rekitSlot(p, row.Slot)
	m.pushWindow(sess, p, row.Slot)
	return true
}

// giveRefusal is the line the merc's give refuses the pack's record with,
// "" when it may go (the file's doc): a worn item (taken off first), a No
// Rent item that vanishes at logout. A NO TRADE or LORE item may go. The
// one rule for every way in: /merc give, the Give box, vanilla's window and
// the trade window (merctrade.go).
func (m *Mercs) giveRefusal(inv database.CharInventoryEntry) string {
	switch name := inv.Pattern.ItemName; {
	case inv.EquipLoc >= 0:
		return fmt.Sprintf(mercGiveWorn, name)
	case m.vanishes(inv.Pattern.PatternID):
		return fmt.Sprintf(mercGiveVanish, name)
	}
	return ""
}

// giveN moves n of the pack's record at index idx of l (inv its row) onto
// the merc of row's slot, in one transaction (GiveMercItem: onto a stack it
// carries, else a record of its own while it holds fewer than
// MercItemsMax), and tells the client it left the pack (0x003C). It returns
// the line refusing it ("" it went); the caller says what went and makes
// the kit and the window again.
func (m *Mercs) giveN(sess *session.Session, p *entity.Player, row database.MercRow, l *invList, idx int, inv database.CharInventoryEntry, n int) string {
	if line := m.giveRefusal(inv); line != "" {
		return line
	}
	name := inv.Pattern.ItemName
	_, _, err := m.db.GiveMercItem(p.CharServerID, row.Slot, inv.ItemID, n, inv.Pattern.MaxStack)
	switch {
	case errors.Is(err, database.ErrMercFull):
		return fmt.Sprintf(mercGiveFull, row.Name, database.MercItemsMax)
	case errors.Is(err, database.ErrMercItemWorn):
		return fmt.Sprintf(mercGiveWorn, name)
	case err != nil:
		log.Printf("Merc: %s gives %q to %q: %v", p.Name, name, row.Name, err)
		return mercKitFailed
	}
	l.took(idx, n)
	queueReliable(sess, protocol.OpcodeRemoveInvItem, removeInvItemPayload(idx, 0, n))
	log.Printf("Merc: %s gives %d %q (item %d) to %q (slot %d)", p.Name, n, name, inv.ItemID, row.Name, row.Slot)
	return ""
}

// roomFor reports whether the merc of row's slot can take all the gives
// together (pattern, count, its stack limit), as GiveMercItem would file
// them one after another: each onto a stack it carries with room, else a
// record of its own, MercItemsMax records in all.
func (m *Mercs) roomFor(p *entity.Player, row database.MercRow, gives []invGive) bool {
	items, err := m.db.MercItems(p.CharServerID, row.Slot)
	if err != nil {
		return false
	}
	type held struct{ pattern, stack int }
	var carried []held
	for _, it := range items {
		if it.EquipLoc < 0 {
			carried = append(carried, held{it.PatternID, max(it.Stack, 1)})
		}
	}
	count := len(items)
	for _, g := range gives {
		onto := -1
		if g.maxStack > 1 {
			for i, c := range carried {
				if c.pattern == g.patternID && c.stack+g.n <= g.maxStack {
					onto = i
					break
				}
			}
		}
		if onto >= 0 {
			carried[onto].stack += g.n
			continue
		}
		if count >= database.MercItemsMax {
			return false
		}
		count++
		carried = append(carried, held{g.patternID, g.n})
	}
	return true
}

// toPack moves the merc's (or the keeping's) record into the player's pack,
// as much of it as the pack takes: the client told (0x003B). It reports how
// many came, and a line why none did ("" none to say).
func (m *Mercs) toPack(sess *session.Session, p *entity.Player, it database.MercItem, keeper string) (int, string) {
	l := sessionInvList(sess)
	if l == nil {
		return 0, ""
	}
	pat := it.Pattern
	m.db.ViewPattern(p.CharServerID, &pat) // the owner's own view of an heirloom
	want := max(it.Stack, 1)
	came := 0
	for came < want {
		qty := want - came
		if pat.MaxStack < 2 {
			qty = 1
		}
		room := l.wouldAdd(pat.PatternID, qty, pat.MaxStack)
		took := l.loreRoom(&pat, room)
		if took == 0 {
			switch {
			case room > 0:
				return came, fmt.Sprintf(mercLoreBack, pat.ItemName, keeper)
			case came == 0:
				return 0, fmt.Sprintf(mercPackFull, pat.ItemName)
			}
			return came, ""
		}
		onto := 0
		if i := l.stackOnto(pat.PatternID, pat.MaxStack); i >= 0 {
			r, _ := l.at(i)
			onto = r.itemID
		}
		itemID, err := m.db.MercItemToPack(p.CharServerID, it.ID, took, onto)
		if err != nil {
			log.Printf("Merc: %q's %q to %s's pack: %v", keeper, pat.ItemName, p.Name, err)
			return came, ""
		}
		it.Stack -= took
		key, _ := l.add(itemID, pat.PatternID, took, pat.MaxStack)
		out := wireRecord(database.CharInventoryEntry{ItemID: itemID, StackLeft: took, RemainHP: it.RemainHP, Charges: it.Charges, Pattern: pat,
			AuraTag: database.InstanceTag(itemID)}) // the copy's own tag, carried (instanceaura)
		queueReliable(sess, protocol.OpcodeAddInvItem, addInvItemPayload(&out, key, readsCapes(sess)))
		came += took
	}
	return came, ""
}

// mercUsable is whether a carried record is a meal or a grocer's drink its
// owner may have the merc Use (consumableOf's: a player's own).
func mercUsable(p *database.ItemPatternRecord) bool {
	_, ok := consumableOf(p)
	return ok
}

// useItem has the merc eat or drink the carried record now (the file's doc):
// one of it used, the window sent again.
func (m *Mercs) useItem(sess *session.Session, p *entity.Player, row database.MercRow, it database.MercItem) bool {
	name := it.Pattern.ItemName
	if it.EquipLoc >= 0 || !mercUsable(&it.Pattern) {
		SendErrorMessage(sess, fmt.Sprintf(mercUseNot, row.Name, name))
		return false
	}
	if m.gw == nil {
		SendErrorMessage(sess, fmt.Sprintf(mercUseNotOut, row.Name))
		return false
	}
	drink, err := m.gw.MercUse(sess, row.Slot, it.ID, m.now())
	switch {
	case errors.Is(err, entity.ErrMercUseNoPower):
		SendErrorMessage(sess, fmt.Sprintf(mercUseNoPower, row.Name, name))
		return false
	case errors.Is(err, entity.ErrMercUseNone):
		SendErrorMessage(sess, "Your mercenary no longer holds that.")
		m.pushWindow(sess, p, row.Slot)
		return false
	case errors.Is(err, entity.ErrMercUseNotFood):
		SendErrorMessage(sess, fmt.Sprintf(mercUseNot, row.Name, name))
		return false
	case err != nil:
		SendErrorMessage(sess, fmt.Sprintf(mercUseNotOut, row.Name))
		return false
	}
	m.gw.RunMercNotes() // the record's use kept before the window goes (noteSupply)
	log.Printf("Merc: %s has %q (slot %d) use %q (record %d)", p.Name, row.Name, row.Slot, name, it.ID)
	SendClientMessage(sess, fmt.Sprintf(map[bool]string{false: mercUseEat, true: mercUseDrink}[drink], row.Name, name))
	m.pushWindow(sess, p, row.Slot)
	return true
}

// takeItem takes the merc's record back into the pack (its kit made again).
func (m *Mercs) takeItem(sess *session.Session, p *entity.Player, row database.MercRow, it database.MercItem) bool {
	came, why := m.toPack(sess, p, it, row.Name)
	if why != "" {
		SendErrorMessage(sess, why)
	}
	if came == 0 {
		return false
	}
	log.Printf("Merc: %s takes %d %q back from %q (slot %d)", p.Name, came, it.Pattern.ItemName, row.Name, row.Slot)
	SendClientMessage(sess, fmt.Sprintf(mercTakeLine, row.Name, it.Pattern.ItemName))
	m.rekitSlot(p, row.Slot)
	m.pushWindow(sess, p, row.Slot)
	return true
}

// claimKeeping brings the keeping into the pack as it has room, and says what
// still waits.
func (m *Mercs) claimKeeping(sess *session.Session, p *entity.Player, quiet bool) {
	if m.db == nil || sessionInvList(sess) == nil {
		return
	}
	kept, err := m.db.MercItems(p.CharServerID, database.MercKeeping)
	if err != nil || len(kept) == 0 {
		return
	}
	left := 0
	for _, it := range kept {
		came, _ := m.toPack(sess, p, it, "the Mercenary Recruitment board")
		if came > 0 {
			SendClientMessage(sess, fmt.Sprintf(mercClaimLine, it.Pattern.ItemName))
			log.Printf("Merc: %s's %d %q come back from the keeping", p.Name, came, it.Pattern.ItemName)
		}
		if came < max(it.Stack, 1) {
			left++
		}
	}
	if left > 0 && !quiet {
		SendErrorMessage(sess, fmt.Sprintf(mercKeptLine, left))
	} else if left > 0 {
		SendClientMessage(sess, fmt.Sprintf(mercKeptLine, left))
	}
}

// itemNamed is the first of the records whose name holds the words (an exact
// name first), false none.
func itemNamed[T any](list []T, name func(T) string, words string) (T, bool) {
	var zero T
	words = strings.ToLower(strings.TrimSpace(words))
	if words == "" {
		return zero, false
	}
	for _, it := range list {
		if strings.EqualFold(name(it), words) {
			return it, true
		}
	}
	for _, it := range list {
		if strings.Contains(strings.ToLower(name(it)), words) {
			return it, true
		}
	}
	return zero, false
}

// equipPlanFor is the merc's records' slots after it puts it on, as a
// player's toggle (planInteractItem): the first free slot of its wear's, a
// held item and a weapon either hand holds by the player's rules, what that
// slot's wear takes off coming off. A worn one comes off.
func equipPlanFor(items []database.MercItem, it database.MercItem, class int) ([]database.MercEquipChange, int, bool) {
	return equipPlanAt(items, it, class, -1)
}

// equipPlanAt is equipPlanFor into the equip slot want (vanilla's merc
// window, mercwin.go: a slot of the item's wear, whatever fills it coming off,
// carried), -1 the slot equipPlanFor chooses. A worn item asked into another
// slot of its wear moves there; asked into its own, nothing changes.
func equipPlanAt(items []database.MercItem, it database.MercItem, class, want int) ([]database.MercEquipChange, int, bool) {
	if it.EquipLoc >= 0 && (want < 0 || want == it.EquipLoc) {
		if want == it.EquipLoc {
			return nil, want, true
		}
		return []database.MercEquipChange{{ID: it.ID, EquipLoc: -1}}, -1, true
	}
	wear := it.Pattern.EquipSlot
	slots := equipSlotsFor(wear, class)
	if len(slots) == 0 || wear == wearCape || want >= 0 && !slices.Contains(slots, want) {
		return nil, 0, false
	}
	inv := make([]*database.CharInventoryEntry, 0, len(items))
	worn := map[int]bool{}
	var found *database.CharInventoryEntry
	for i := range items {
		e := &database.CharInventoryEntry{ItemID: items[i].ID, EquipLoc: items[i].EquipLoc, Pattern: items[i].Pattern}
		inv = append(inv, e)
		if e.EquipLoc >= 0 {
			worn[e.EquipLoc] = true
		}
		if items[i].ID == it.ID {
			found = e
		}
	}
	if found == nil {
		return nil, 0, false
	}
	chosen := slots[0]
	for _, s := range slots {
		if !worn[s] {
			chosen = s
			break
		}
	}
	switch {
	case want >= 0:
		chosen = want
	case wear == wearHeld:
		chosen = heldSlot(worn)
	case wear == wearEitherHand && !offHandWeaponBarred(class):
		chosen = eitherHandSlot(worn)
	}
	plan := planEquipInto(inv, 0, found, chosen)
	var out []database.MercEquipChange
	if plan.unequip >= 0 && plan.unequip != chosen {
		out = append(out, database.MercEquipChange{ID: it.ID, EquipLoc: -1})
	}
	for _, c := range plan.changes {
		out = append(out, database.MercEquipChange{ID: c.itemID, EquipLoc: c.equipLoc})
	}
	return out, chosen, true
}

// equip puts the merc's record on (or takes it off when worn), by the
// player's gates and slots, its kit made again.
func (m *Mercs) equip(sess *session.Session, p *entity.Player, row database.MercRow, it database.MercItem, items []database.MercItem) bool {
	return m.equipAt(sess, p, row, it, items, -1)
}

// equipAt is equip into the equip slot want (equipPlanAt; -1 equip's).
func (m *Mercs) equipAt(sess *session.Session, p *entity.Player, row database.MercRow, it database.MercItem, items []database.MercItem, want int) bool {
	if it.EquipLoc < 0 || want >= 0 && want != it.EquipLoc {
		if why := cannotWear(mercWearer(row, p.Level), &it.Pattern); why != "" {
			SendErrorMessage(sess, fmt.Sprintf(mercEquipNot, row.Name, it.Pattern.ItemName)+" "+why)
			return false
		}
	}
	changes, slot, ok := equipPlanAt(items, it, row.Class, want)
	if !ok {
		SendErrorMessage(sess, fmt.Sprintf(mercEquipNot, row.Name, it.Pattern.ItemName))
		return false
	}
	if err := m.db.SetMercEquip(p.CharServerID, row.Slot, changes); err != nil {
		log.Printf("Merc: %s's %q's %q: %v", p.Name, row.Name, it.Pattern.ItemName, err)
		SendErrorMessage(sess, mercKitFailed)
		return false
	}
	if slot < 0 {
		SendClientMessage(sess, fmt.Sprintf(mercUnequipLine, row.Name, it.Pattern.ItemName))
	} else {
		SendClientMessage(sess, fmt.Sprintf(mercEquipLine, row.Name, it.Pattern.ItemName))
	}
	log.Printf("Merc: %s's %q (slot %d) %v %q: %+v", p.Name, row.Name, row.Slot, map[bool]string{true: "takes off", false: "wears"}[slot < 0],
		it.Pattern.ItemName, changes)
	m.rekitSlot(p, row.Slot)
	m.pushWindow(sess, p, row.Slot)
	return true
}

// mercItemsOf is the merc's records, said when it has none (none: false).
func (m *Mercs) mercItemsOf(sess *session.Session, row database.MercRow) ([]database.MercItem, bool) {
	items, err := m.db.MercItems(row.ServerID, row.Slot)
	if err != nil {
		SendErrorMessage(sess, mercKitFailed)
		return nil, false
	}
	if len(items) == 0 {
		SendClientMessage(sess, fmt.Sprintf(mercInvNone, row.Name))
		return nil, false
	}
	return items, true
}

// invCommand is /merc's give, inv, equip, unequip, take, use, stock, claim,
// aoe and single (the file's doc), for the merc of the slot.
func (m *Mercs) invCommand(sess *session.Session, p *entity.Player, rows []database.MercRow, slot int, verb, rest string) {
	if verb == "claim" {
		m.claimKeeping(sess, p, false)
		return
	}
	if verb == "stock" {
		m.menuStock(sess, p, 0)
		return
	}
	if slot = m.oneSlot(sess, rows, slot, verb); slot == 0 {
		return
	}
	row, ok := m.mercRowOf(sess, p, slot)
	if !ok {
		return
	}
	switch verb {
	case "aoe", "single":
		m.setThrows(sess, p, row, verb == "aoe")
	case "give":
		if rest == "" {
			m.menuGive(sess, p, slot, 0)
			return
		}
		it, ok := itemNamed(m.givable(p), func(e database.CharInventoryEntry) string { return e.Pattern.ItemName }, rest)
		if !ok {
			SendErrorMessage(sess, fmt.Sprintf(mercGiveNone, rest))
			return
		}
		m.giveItem(sess, p, row, it.ItemID)
	case "inv", "inventory", "items":
		if m.openWindow(sess, p, row) {
			return // vanilla's window (mercwin.go)
		}
		items, ok := m.mercItemsOf(sess, row)
		if !ok {
			return
		}
		SendClientMessage(sess, fmt.Sprintf(mercInvHead, row.Name, len(items), database.MercItemsMax))
		var names []string
		for _, it := range items {
			names = append(names, mercItemName(it))
		}
		SendClientMessage(sess, strings.Join(names, ", ")+".")
		m.menuInv(sess, p, slot, 0)
	case "equip", "unequip", "take", "use":
		items, ok := m.mercItemsOf(sess, row)
		if !ok {
			return
		}
		it, ok := itemNamed(items, func(e database.MercItem) string { return e.Pattern.ItemName }, rest)
		if !ok {
			SendErrorMessage(sess, fmt.Sprintf(mercTakeNone, row.Name, rest))
			return
		}
		switch {
		case verb == "take":
			m.takeItem(sess, p, row, it)
		case verb == "use":
			m.useItem(sess, p, row, it)
		case verb == "equip" && it.EquipLoc >= 0, verb == "unequip" && it.EquipLoc < 0:
			SendClientMessage(sess, fmt.Sprintf(map[bool]string{true: mercEquipLine, false: "%s is not wearing %s."}[it.EquipLoc >= 0], row.Name, it.Pattern.ItemName))
		default:
			m.equip(sess, p, row, it, items)
		}
	}
}

// mercCocktail is whether the hire is an alchemist of the Cocktail path.
func mercCocktail(row database.MercRow) bool {
	return row.Class == mercAlchemist && mercPathOf(row.Class, row.Path) == 1
}

// setThrows is /merc aoe or single: kept in its row, its kit made again.
func (m *Mercs) setThrows(sess *session.Session, p *entity.Player, row database.MercRow, aoe bool) {
	if !mercCocktail(row) {
		SendErrorMessage(sess, mercThrowsNot)
		return
	}
	if err := m.db.SetMercAoE(p.CharServerID, row.Slot, aoe); err != nil {
		log.Printf("Merc: %s's %q's throws not kept: %v", p.Name, row.Name, err)
		SendErrorMessage(sess, mercKitFailed)
		return
	}
	m.rekitSlot(p, row.Slot)
	log.Printf("Merc: %s's %q throws %s", p.Name, row.Name, map[bool]string{true: "area cocktails", false: "single-target potions"}[aoe])
	SendClientMessage(sess, fmt.Sprintf(map[bool]string{true: mercThrowsAoE, false: mercThrowsSingle}[aoe], row.Name))
}

// stockRows is the player's alchemists' stock as the database keeps it, by
// name, each with its record.
type stockRow struct {
	pat database.ItemPatternRecord
	n   int
}

func (m *Mercs) stockRows(p *entity.Player) []stockRow {
	st, err := m.db.MercStock(p.CharServerID)
	if err != nil {
		return nil
	}
	store := database.NewItemPatternStore(m.db)
	var out []stockRow
	for id, n := range st {
		if rec, err := store.GetItemPattern(id); err == nil && rec != nil {
			out = append(out, stockRow{pat: *rec, n: n})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].pat.ItemName < out[j].pat.ItemName })
	return out
}

// takeStock takes a stack of the stock's item (its stack's most, or what is
// left) into the pack, as much as fits.
func (m *Mercs) takeStock(sess *session.Session, p *entity.Player, s stockRow) bool {
	return m.takeStockN(sess, p, s, 0)
}

// takeStockN is takeStock of n (0: a stack), at most a stack and what the
// stock holds (vanilla's merc window's Take: mercwin.go); true when any came.
func (m *Mercs) takeStockN(sess *session.Session, p *entity.Player, s stockRow, n int) bool {
	l := sessionInvList(sess)
	if l == nil {
		return false
	}
	pat := s.pat
	want := min(s.n, max(pat.MaxStack, 1))
	if n > 0 {
		want = min(want, n)
	}
	room := l.loreRoom(&pat, l.wouldAdd(pat.PatternID, want, pat.MaxStack))
	if room == 0 {
		if l.wouldAdd(pat.PatternID, want, pat.MaxStack) > 0 {
			SendErrorMessage(sess, fmt.Sprintf(loreLine, pat.ItemName))
		} else {
			SendErrorMessage(sess, fmt.Sprintf(mercPackFull, pat.ItemName))
		}
		return false
	}
	onto := 0
	if i := l.stackOnto(pat.PatternID, pat.MaxStack); i >= 0 {
		r, _ := l.at(i)
		onto = r.itemID
	}
	itemID, err := m.db.MercStockToPack(p.CharServerID, pat.PatternID, room, pat.MaxHP, onto)
	if err != nil {
		log.Printf("Merc: %s's stock of %q to the pack: %v", p.Name, pat.ItemName, err)
		SendErrorMessage(sess, mercKitFailed)
		return false
	}
	if st, err := m.db.MercStock(p.CharServerID); err == nil {
		m.gw.SetMercStock(p.CharServerID, st) // the World's copy as kept
	}
	key, _ := l.add(itemID, pat.PatternID, room, pat.MaxStack)
	out := wireRecord(database.CharInventoryEntry{ItemID: itemID, StackLeft: room, RemainHP: pat.MaxHP, Pattern: pat})
	queueReliable(sess, protocol.OpcodeAddInvItem, addInvItemPayload(&out, key, readsCapes(sess)))
	log.Printf("Merc: %s takes %d %q from the alchemists' stock", p.Name, room, pat.ItemName)
	SendClientMessage(sess, fmt.Sprintf(mercStockTake, room, pat.ItemName))
	return true
}

// loadStock gives the World the character's stock as kept (a login, a keep
// restart).
func (m *Mercs) loadStock(p *entity.Player) {
	if m.db == nil || p == nil {
		return
	}
	st, err := m.db.MercStock(p.CharServerID)
	if err != nil {
		log.Printf("Merc: %s's stock: %v", p.Name, err)
		return
	}
	m.gw.SetMercStock(p.CharServerID, st)
}

// stockFor gives the World the character's stock as kept when it holds none
// of the character's (a hire after a keep restart that carried no merc of
// its: only a kept merc's Restore and a login read it), so a merc out always
// brews into and throws from its owner's stock. One the World holds is its
// own copy, newer than the rows by the notes on their way.
func (m *Mercs) stockFor(p *entity.Player) {
	if m.db == nil || p == nil || m.gw.MercStockKnown(p.CharServerID) {
		return
	}
	m.loadStock(p)
}

// noteSupply keeps an alchemist's use of what it carried or of the stock, and
// what it made (entity/mercbrew.go's notes).
func (m *Mercs) noteSupply(n entity.MercNote) {
	char := n.Owner.CharServerID
	switch n.Kind {
	case entity.MercNoteUsed:
		if err := m.db.UseMercItem(char, n.ItemID, max(n.Count, 1)); err != nil {
			log.Printf("Merc: %s's %q's use of record %d not kept: %v", n.Owner.Name, n.Name, n.ItemID, err)
		}
	case entity.MercNoteStockUsed:
		if _, err := m.db.UseMercStock(char, n.PatternID, max(n.Count, 1)); err != nil {
			log.Printf("Merc: %s's stock's %d not taken: %v", n.Owner.Name, n.PatternID, err)
		}
		m.stockChanged(n.Owner) // an open window's Stock (mercwin.go)
	case entity.MercNoteBrewed:
		got, err := m.db.AddMercStock(char, n.PatternID, max(n.Count, 1), m.gw.MercBrewCapNow())
		if err != nil {
			log.Printf("Merc: %s's %q's potion not kept: %v", n.Owner.Name, n.Name, err)
		}
		if err != nil || got < max(n.Count, 1) {
			m.loadStock(n.Owner) // the World's copy past what was kept
		}
		m.stockChanged(n.Owner) // an open window's Stock (mercwin.go)
	}
}

// The items' boxes (mercmenu.go's stages).

// menuItems is the Items box: Inventory, Give an item, an alchemist's
// potion stock and the Cocktail toggle's word, and the keeping's claim.
func (m *Mercs) menuItems(sess *session.Session, p *entity.Player, slot int) {
	row, ok := m.mercRowOf(sess, p, slot)
	if !ok {
		return
	}
	items, _ := m.db.MercItems(row.ServerID, row.Slot)
	keys := []string{"inv", "give"}
	answers := []string{fmt.Sprintf("Inventory (%d)", len(items)), "Give an item"}
	if row.Class == mercAlchemist {
		total := 0
		for _, s := range m.stockRows(p) {
			total += s.n
		}
		keys, answers = append(keys, "stock"), append(answers, fmt.Sprintf("Potion stock (%d)", total))
	}
	if kept, _ := m.db.MercItems(p.CharServerID, database.MercKeeping); len(kept) > 0 {
		keys, answers = append(keys, "claim"), append(answers, fmt.Sprintf("Claim returned items (%d)", len(kept)))
	}
	m.mu.Lock()
	m.askLocked(sess, p, mercAsk{stage: mercStageItems, slot: slot, keys: keys}, fmt.Sprintf(mercItemsAsk, row.Name), answers)
	m.mu.Unlock()
}

// menuInv is a page of the merc's records.
func (m *Mercs) menuInv(sess *session.Session, p *entity.Player, slot, page int) {
	row, ok := m.mercRowOf(sess, p, slot)
	if !ok {
		return
	}
	items, ok := m.mercItemsOf(sess, row)
	if !ok {
		return
	}
	pages := (len(items) + mercInvPage - 1) / mercInvPage
	if page < 0 || page >= pages {
		page = 0 // More on the last page: the first again
	}
	var answers []string
	var ids []int
	for _, it := range items[page*mercInvPage : min((page+1)*mercInvPage, len(items))] {
		answers = append(answers, mercLabel(mercItemName(it)))
		ids = append(ids, it.ID)
	}
	if pages > 1 {
		answers = append(answers, "More")
	}
	m.mu.Lock()
	m.askLocked(sess, p, mercAsk{stage: mercStageInv, slot: slot, page: page, ids: ids}, fmt.Sprintf(mercInvAsk, row.Name, page+1, pages), answers)
	m.mu.Unlock()
}

// menuItem is one record's box: Equip or Unequip (an item it may wear), Take
// back.
func (m *Mercs) menuItem(sess *session.Session, p *entity.Player, slot, id int) {
	row, ok := m.mercRowOf(sess, p, slot)
	if !ok {
		return
	}
	items, _ := m.db.MercItems(row.ServerID, row.Slot)
	for _, it := range items {
		if it.ID != id {
			continue
		}
		var keys, answers []string
		switch {
		case it.EquipLoc >= 0:
			keys, answers = append(keys, "unequip"), append(answers, "Unequip")
		case len(equipSlotsFor(it.Pattern.EquipSlot, row.Class)) > 0:
			keys, answers = append(keys, "equip"), append(answers, "Equip")
		case mercUsable(&it.Pattern):
			keys, answers = append(keys, "use"), append(answers, "Use")
		}
		keys, answers = append(keys, "take", "back"), append(answers, "Take back", "Back")
		m.mu.Lock()
		m.askLocked(sess, p, mercAsk{stage: mercStageItem, slot: slot, item: id, keys: keys}, mercLabel(fmt.Sprintf(mercItemAsk, row.Name, mercItemName(it))), answers)
		m.mu.Unlock()
		return
	}
	SendErrorMessage(sess, fmt.Sprintf(mercTakeNone, row.Name, "such item"))
}

// menuGive is a page of the pack's records the merc may take.
func (m *Mercs) menuGive(sess *session.Session, p *entity.Player, slot, page int) {
	row, ok := m.mercRowOf(sess, p, slot)
	if !ok {
		return
	}
	give := m.givable(p)
	if len(give) == 0 {
		SendClientMessage(sess, "Your pack holds nothing your mercenary may take.")
		return
	}
	pages := (len(give) + mercInvPage - 1) / mercInvPage
	if page < 0 || page >= pages {
		page = 0 // More on the last page: the first again
	}
	var answers []string
	var ids []int
	for _, r := range give[page*mercInvPage : min((page+1)*mercInvPage, len(give))] {
		label := r.Pattern.ItemName
		if r.StackLeft > 1 {
			label += fmt.Sprintf(" x%d", r.StackLeft)
		}
		answers = append(answers, mercLabel(label))
		ids = append(ids, r.ItemID)
	}
	if pages > 1 {
		answers = append(answers, "More")
	}
	m.mu.Lock()
	m.askLocked(sess, p, mercAsk{stage: mercStageGive, slot: slot, page: page, ids: ids}, fmt.Sprintf(mercGiveAsk, row.Name, page+1, pages), answers)
	m.mu.Unlock()
}

// menuStock is a page of the alchemists' stock, each taken a stack at a time.
func (m *Mercs) menuStock(sess *session.Session, p *entity.Player, page int) {
	st := m.stockRows(p)
	if len(st) == 0 {
		SendClientMessage(sess, mercStockNone)
		return
	}
	pages := (len(st) + mercInvPage - 1) / mercInvPage
	if page < 0 || page >= pages {
		page = 0 // More on the last page: the first again
	}
	var answers []string
	var ids []int
	for _, s := range st[page*mercInvPage : min((page+1)*mercInvPage, len(st))] {
		answers = append(answers, mercLabel(fmt.Sprintf("%s x%d", s.pat.ItemName, s.n)))
		ids = append(ids, s.pat.PatternID)
	}
	if pages > 1 {
		answers = append(answers, "More")
	}
	m.mu.Lock()
	m.askLocked(sess, p, mercAsk{stage: mercStageStock, page: page, ids: ids}, fmt.Sprintf(mercStockAsk, page+1, pages), answers)
	m.mu.Unlock()
}

// itemsAnswer is a choice in the items' boxes (menuAnswer's).
func (m *Mercs) itemsAnswer(sess *session.Session, p *entity.Player, a mercAsk, n int) {
	pick := func() (int, bool) {
		if n < len(a.ids) {
			return a.ids[n], true
		}
		return 0, false
	}
	switch a.stage {
	case mercStageItems:
		if n >= len(a.keys) {
			return
		}
		switch a.keys[n] {
		case "inv":
			if row, ok := m.mercRowOf(sess, p, a.slot); ok && m.openWindow(sess, p, row) {
				return // vanilla's window (mercwin.go)
			}
			m.menuInv(sess, p, a.slot, 0)
		case "give":
			m.menuGive(sess, p, a.slot, 0)
		case "stock":
			m.menuStock(sess, p, 0)
		case "claim":
			m.claimKeeping(sess, p, false)
		}
	case mercStageInv:
		if id, ok := pick(); ok {
			m.menuItem(sess, p, a.slot, id)
		} else if n == len(a.ids) {
			m.menuInv(sess, p, a.slot, nextPage(a.page))
		}
	case mercStageItem:
		if n >= len(a.keys) {
			return
		}
		row, ok := m.mercRowOf(sess, p, a.slot)
		if !ok {
			return
		}
		items, _ := m.db.MercItems(row.ServerID, row.Slot)
		for _, it := range items {
			if it.ID != a.item {
				continue
			}
			switch a.keys[n] {
			case "equip", "unequip":
				m.equip(sess, p, row, it, items)
			case "take":
				m.takeItem(sess, p, row, it)
			case "use":
				m.useItem(sess, p, row, it)
			case "back":
				m.menuInv(sess, p, a.slot, 0)
			}
			return
		}
	case mercStageGive:
		if id, ok := pick(); ok {
			if row, ok := m.mercRowOf(sess, p, a.slot); ok && m.giveItem(sess, p, row, id) {
				m.menuGive(sess, p, a.slot, a.page)
			}
		} else if n == len(a.ids) {
			m.menuGive(sess, p, a.slot, nextPage(a.page))
		}
	case mercStageStock:
		if id, ok := pick(); ok {
			for _, s := range m.stockRows(p) {
				if s.pat.PatternID == id {
					m.takeStock(sess, p, s)
					break
				}
			}
		} else if n == len(a.ids) {
			m.menuStock(sess, p, nextPage(a.page))
		}
	}
}

// nextPage is the page after page (one past the last asks the first).
func nextPage(page int) int { return page + 1 }
