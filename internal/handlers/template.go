package handlers

import (
	"cmp"
	"errors"
	"fmt"
	"log"
	"slices"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// #template (also /template): a GM's own character made a level-60 one of its
// class and race at once, for testing (jib's ask, F3 pass 13, 2026-10-25):
// level 60 with no experience into it, every spell of its class in the book
// (classspells, as world.start_all_spells gives a new character), 1,000,000
// tunar in the purse, and in each slot the best item of this server's own
// table (itempattern) its class and race may wear at 60, worn.
//
// The gear (templateGear): an item counts only where CLIENT's
// VIPlayer::CanUse lets the character use it (the class's bit in classuse,
// the race's in raceuse, levelreq at most 60) and its wear slot goes where it
// is put (itemSlotToEquipSlot, fitsEquip), so a row with no class or race,
// or no wear slot the client takes, is never picked.
//
//   - Armour and jewellery by templateScore, each slot its best; two of a kind
//     for the ears and the rings (the same item twice unless it is LORE); the
//     forearms either one pair of bracers or two bracelets, whichever adds
//     more; the robe too.
//   - The hands by the damage the player's melee rolls on (offence.go rolls on
//     the main hand's or the two-hander's DMG alone): the best two-hander
//     against the best main-hand weapon with the best thing for the other
//     hand beside it (a shield, a second weapon or an item held there, by
//     templateScore), the higher DMG first, then the more score; a tie goes to
//     the two-hander.
//   - A bow or a thrown weapon, the best by its DMG, in the bag: the client
//     takes the ranged slot's item off for any weapon in the hands
//     (equipDisplaces), so it is carried, not worn.
//
// Ties go to the higher levelreq, then the lower patternid. The table is used
// as it stands: an item whose row says it adds STR 387 scores as one that
// does.
//
// What the character had stays in its bag; only what was worn in a slot the
// template fills, or one a picked item takes off, comes off, and a picked item
// already in the bag is worn rather than given again. The client's list holds
// 40 records (invListCap): what does not fit is not given, and is named.
//
// It all reaches the client as a MemoryDump at the character's place, as a
// respawn's does (respawn.go): the level, the experience, the purse, the
// inventory with what is worn, and the ability list in one message. CLIENT's
// Msg000D plays the respawn cue and fades the screen in, enters the world
// again where it stands, and answers 0x0014, as after a death.

const (
	templateLevel = 60
	templateTunar = 1_000_000
)

// templateWear is every wear slot the template looks in.
var templateWear = []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19}

// templateSlotNames names the equip slots (charinventory.equiploc) in the
// template's answer.
var templateSlotNames = map[int]string{
	0: "Head", 1: "Hands", 2: "Ear", 3: "Ear", 4: "Neck", 5: "Chest",
	6: "Forearm", 7: "Forearm", 8: "Forearms", 9: "Ring", 10: "Ring",
	11: "Waist", 12: "Legs", 13: "Feet", 14: "Main hand", 15: "Off hand",
	16: "Both hands", 17: "Shield", 18: "Ranged", 19: "Held", 20: "Off hand",
	21: "Robe",
}

// templateScore is what an item is worth to the template: its AC, each point
// of the seven stats twice that, as is each point of hit points and power a
// tick, half its hit point and power maxima, and its resists at a quarter.
func templateScore(p *database.ItemPatternRecord) int {
	v := func(x *int) int {
		if x == nil {
			return 0
		}
		return *x
	}
	stats := v(p.Str) + v(p.Sta) + v(p.Agi) + v(p.Dex) + v(p.Wis) + v(p.Intelligence) + v(p.Cha)
	resists := v(p.PR) + v(p.DR) + v(p.FR) + v(p.CR) + v(p.LR) + v(p.AR)
	return v(p.AC) + 2*(stats+v(p.PoT)+v(p.HoT)) + (v(p.HPMAX)+v(p.POWMAX))/2 + resists/4
}

// templatePick is an item the template wears in an equip slot, or carries
// (slot -1).
type templatePick struct {
	slot    int
	pattern database.ItemPatternRecord
}

// byScore orders items best first for an armour slot: score, then the higher
// level, then the lower patternid.
func byScore(a, b database.ItemPatternRecord) int {
	return cmp.Or(
		cmp.Compare(templateScore(&b), templateScore(&a)),
		cmp.Compare(b.LevelReq, a.LevelReq),
		cmp.Compare(a.PatternID, b.PatternID))
}

// byDamage orders weapons best first: the DMG the melee rolls on, then as
// byScore.
func byDamage(a, b database.ItemPatternRecord) int {
	return cmp.Or(cmp.Compare(b.WeaponDamage, a.WeaponDamage), byScore(a, b))
}

// templateGear is what the template wears and carries out of items (those the
// character may wear: database.WearablePatterns), as the file's doc says.
func templateGear(items []database.ItemPatternRecord) []templatePick {
	of := func(order func(a, b database.ItemPatternRecord) int, wear ...int) []database.ItemPatternRecord {
		var out []database.ItemPatternRecord
		for _, it := range items {
			if slices.Contains(wear, it.EquipSlot) {
				out = append(out, it)
			}
		}
		slices.SortStableFunc(out, order)
		return out
	}
	// two of one kind: the best twice, unless it is LORE (then the next)
	two := func(c []database.ItemPatternRecord) []database.ItemPatternRecord {
		switch {
		case len(c) == 0:
			return nil
		case c[0].Lore == 0:
			return []database.ItemPatternRecord{c[0], c[0]}
		case len(c) > 1:
			return c[:2]
		}
		return c[:1]
	}
	var picks []templatePick
	wear := func(slot int, p database.ItemPatternRecord) {
		picks = append(picks, templatePick{slot: slot, pattern: p})
	}
	one := func(slot int, c []database.ItemPatternRecord) {
		if len(c) > 0 {
			wear(slot, c[0])
		}
	}

	one(0, of(byScore, 1))  // head
	one(1, of(byScore, 19)) // hands
	for i, p := range two(of(byScore, 3)) {
		wear(2+i, p) // ears
	}
	one(4, of(byScore, 4)) // neck
	one(5, of(byScore, 5)) // chest
	// the forearms: a pair of bracers or two bracelets
	bracers, bracelets := of(byScore, 7), two(of(byScore, 6))
	sum := func(c []database.ItemPatternRecord) int {
		n := 0
		for i := range c {
			n += templateScore(&c[i])
		}
		return n
	}
	if len(bracers) > 0 && templateScore(&bracers[0]) >= sum(bracelets) {
		wear(8, bracers[0])
	} else {
		for i, p := range bracelets {
			wear(6+i, p)
		}
	}
	for i, p := range two(of(byScore, 8)) {
		wear(9+i, p) // rings
	}
	one(11, of(byScore, 9))  // waist
	one(12, of(byScore, 10)) // legs
	one(13, of(byScore, 11)) // feet
	one(21, of(byScore, 2))  // robe

	// the hands
	twoHanders, mains := of(byDamage, 15), of(byDamage, 12, 14)
	var main, off *database.ItemPatternRecord
	offSlot := -1
	if len(mains) > 0 {
		main = &mains[0]
		// the other hand: a shield (17), a second weapon (15) or a held item
		// (20), whichever scores most (nothing rolls the second weapon's DMG)
		var best database.ItemPatternRecord
		found := false
		for _, c := range []struct {
			slot int
			c    []database.ItemPatternRecord
		}{{17, of(byScore, 13)}, {15, of(byScore, 14)}, {20, of(byScore, 18)}} {
			for _, p := range c.c {
				if p.PatternID == main.PatternID && p.Lore != 0 {
					continue // a LORE weapon is held once
				}
				if !found || byScore(p, best) < 0 {
					best, offSlot, found = p, c.slot, true
				}
				break
			}
		}
		if found {
			off = &best
		}
	}
	useTwoHander := len(twoHanders) > 0
	if useTwoHander && main != nil {
		th := &twoHanders[0]
		one := templateScore(main)
		if off != nil {
			one += templateScore(off)
		}
		useTwoHander = cmp.Or(cmp.Compare(th.WeaponDamage, main.WeaponDamage), cmp.Compare(templateScore(th), one)) >= 0
	}
	switch {
	case useTwoHander:
		wear(16, twoHanders[0])
	case main != nil:
		wear(14, *main)
		if off != nil {
			wear(offSlot, *off)
		}
	}

	// a bow or a thrown weapon, carried
	if ranged := of(byDamage, 16, 17); len(ranged) > 0 {
		wear(-1, ranged[0])
	}
	return picks
}

// templateRow is one charinventory change the template makes.
type templateRow struct {
	itemID  int // an inventory row reused, or 0 for a new one
	slot    int // where it is worn, -1 carried
	pattern database.ItemPatternRecord
}

// templatePlan is what the template does to the inventory rows held (items,
// as GetCharacterItems returns them): the picks, each on a row of its own (an
// unworn row of the same item reused, else a new one), the rows that come off,
// and the picks there was no room for.
type templatePlan struct {
	rows    []templateRow
	takeOff []int // itemids
	noRoom  []templatePick
}

func planTemplate(items []database.CharInventoryEntry, picks []templatePick) templatePlan {
	var plan templatePlan
	inv := inventoryRows(items)
	room := invListCap - len(inv)
	used := map[int]bool{}
	// the slots the template fills, and every slot a worn pick takes off
	clears := map[int]bool{}
	for _, p := range picks {
		if p.slot >= 0 {
			clears[p.slot] = true
			for _, s := range equipDisplaces(p.pattern.EquipSlot, p.slot) {
				clears[s] = true
			}
		}
	}
	for _, p := range picks {
		row := templateRow{slot: p.slot, pattern: p.pattern}
		// a row of the item already held: unworn, or worn in a slot the
		// template empties anyway (not one it leaves alone)
		for _, e := range inv {
			if e.Pattern.PatternID == p.pattern.PatternID && !used[e.ItemID] && (e.EquipLoc < 0 || clears[e.EquipLoc]) {
				row.itemID = e.ItemID
				break
			}
		}
		if row.itemID == 0 {
			if room <= 0 {
				plan.noRoom = append(plan.noRoom, p)
				continue
			}
			room--
		} else {
			used[row.itemID] = true
		}
		plan.rows = append(plan.rows, row)
	}
	for _, e := range inv {
		if e.EquipLoc >= 0 && clears[e.EquipLoc] && !used[e.ItemID] {
			plan.takeOff = append(plan.takeOff, e.ItemID)
		}
	}
	return plan
}

// ApplyTemplate is #template on the session's character (the file's doc). It
// returns the lines the GM is answered with.
func (h *CharacterHandler) ApplyTemplate(sess *session.Session) ([]string, error) {
	gw := h.gameWorld
	if gw == nil || h.store == nil || h.inventoryStore == nil {
		return nil, errors.New("the server has no character store")
	}
	player := gw.SessionPlayer(sess)
	if player == nil {
		return nil, errors.New("no character in the world")
	}
	snap, ok := gw.PlayerSnapshot(player.ServerID)
	if !ok {
		return nil, errors.New("no character in the world")
	}
	sid := snap.CharServerID
	patterns, err := h.inventoryStore.WearablePatterns(snap.Class, snap.Race, templateLevel, templateWear)
	if err != nil {
		return nil, fmt.Errorf("the items: %w", err)
	}
	items, err := h.inventoryStore.GetCharacterItems(sid)
	if err != nil {
		return nil, fmt.Errorf("the inventory: %w", err)
	}
	plan := planTemplate(items, templateGear(patterns))

	for _, id := range plan.takeOff {
		if err := h.inventoryStore.SetEquipLoc(sid, id, -1); err != nil {
			return nil, fmt.Errorf("taking off item %d: %w", id, err)
		}
	}
	var lines []string
	worn := 0
	for _, r := range plan.rows {
		id := r.itemID
		if id == 0 {
			n := 1
			if r.slot < 0 && r.pattern.MaxStack > 1 {
				n = r.pattern.MaxStack // a stack of thrown weapons
			}
			if id, err = h.inventoryStore.AddCharItem(sid, r.pattern.PatternID, n, r.pattern.MaxHP, 0, 0); err != nil {
				return nil, fmt.Errorf("giving %q: %w", r.pattern.ItemName, err)
			}
		}
		if err := h.inventoryStore.SetEquipLoc(sid, id, r.slot); err != nil {
			return nil, fmt.Errorf("wearing %q: %w", r.pattern.ItemName, err)
		}
		if r.slot >= 0 {
			worn++
			lines = append(lines, fmt.Sprintf("  %s: %s", templateSlotNames[r.slot], r.pattern.ItemName))
		} else {
			lines = append(lines, fmt.Sprintf("  In the bag: %s", r.pattern.ItemName))
		}
	}
	for _, p := range plan.noRoom {
		lines = append(lines, fmt.Sprintf("  No room for %s", p.pattern.ItemName))
	}

	if err := h.store.SetProgress(sid, templateLevel, entity.TotalXP(templateLevel, 0)); err != nil {
		return nil, fmt.Errorf("the level: %w", err)
	}
	// the purse alone: the bank's tunar is written only under bankMu
	// (bank.go), and the snapshot's may be older than an auction's payment
	if err := h.inventoryStore.SetPurse(sid, templateTunar); err != nil {
		return nil, fmt.Errorf("the tunar: %w", err)
	}
	added, err := h.store.GrantClassSpells(sid, snap.Class)
	if err != nil {
		return nil, fmt.Errorf("the spells: %w", err)
	}

	// The game's copy, then the client's: the level, the purse, what the worn
	// items add (full hit points and power at the new level), and the dump.
	gw.SetPlayerLevel(sid, templateLevel)
	if unused, total, ok := gw.LiveTrainingPoints(player.ServerID); ok {
		if err := h.store.SetTrainingPoints(sid, unused, total); err != nil {
			log.Printf("Template: '%s' training points not kept: %v", player.Name, err)
		}
	}
	bankMu.Lock()
	gw.SetPlayerTunar(sid, templateTunar, player.BankTunar)
	bankMu.Unlock()
	h.applyEquipmentFromDB(player, sid, true)
	gw.SyncSelfChannels(sess)
	if now, ok := gw.PlayerSnapshot(player.ServerID); ok {
		h.queueMemoryDump(sess, &now)
	}

	head := fmt.Sprintf("Template: level %d, %d tunar, %d spells scribed, %d items worn",
		templateLevel, templateTunar, added, worn)
	log.Printf("Template: '%s' (class %d, race %d) %s; %d taken off, %d without room",
		snap.Name, snap.Class, snap.Race, head[len("Template: "):], len(plan.takeOff), len(plan.noRoom))
	return append([]string{head}, lines...), nil
}
