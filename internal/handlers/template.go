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

// #template (also /template): a character made a level-60 one of its class at
// once, for testing (jib's ask, F3 pass 13, 2026-10-25): level 60 with no experience
// into it (one past it stays where it is), every spell of its class in the book
// (classspells, a mastery's, an epic's and a master class's too: all but a were's),
// at least 1,000,000 tunar in the purse (never fewer than it had), and of equipment
// only the class's heirlooms it lacks (the user, 2026-11-07: "lets make template
// only add all the class spells not all the extra equipment either, only heirloom if
// they dont have").
//
// The heirlooms (templateHeirlooms): the lowest id of each of the class's roles
// (templateRoles), unless the character already holds one of that role, worn, in
// its pack or in its bank. A new one is worn when a slot it fits is free and wearing
// it takes nothing off (fitsEquip, equipDisplaces), else it stays in the pack: what
// the character wears is never touched. So one with hands and a shield free takes
// the main-hand heirloom and the shield and carries the two-hander, and a class with
// no main-hand heirloom takes its two-hander and carries the shield. The client's
// list holds 40 records (invListCap): what does not fit is not given, and is named.
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

// templateRoles is the order the template goes through a class's heirlooms in:
// the armour and jewellery, then the hands, the main hand and its shield before a
// two-hander, so they are the ones worn (a class with no main-hand heirloom has its
// two-hander worn first: templateHeirlooms).
var templateRoles = []string{"head", "hands", "ear", "neck", "chest", "bracers", "bracelet", "ring", "waist", "legs", "feet", "robe",
	"main", "shield", "twohand", "ranged"}

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

// templateHeirlooms gives the character the class's heirlooms it lacks (the file's
// doc): the answer's lines, how many were given and how many of them are worn.
func (h *CharacterHandler) templateHeirlooms(sid uint32, class int) (lines []string, given, worn int, err error) {
	defs, err := h.inventoryStore.Heirlooms()
	if err != nil {
		return nil, 0, 0, fmt.Errorf("the heirlooms: %w", err)
	}
	items, err := h.inventoryStore.GetCharacterItems(sid)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("the inventory: %w", err)
	}
	held := map[string]bool{} // its pack and its bank
	for i := range items {
		if d, ok := defs[items[i].Pattern.PatternID]; ok && d.Class == class {
			held[d.Role] = true
		}
	}
	inv := inventoryRows(items)
	room := invListCap - len(inv)
	taken := map[int]bool{}
	for _, e := range inv {
		if e.EquipLoc >= 0 {
			taken[e.EquipLoc] = true
		}
	}
	byRole := map[string]int{}
	for id, d := range defs {
		if d.Class == class && (byRole[d.Role] == 0 || id < byRole[d.Role]) {
			byRole[d.Role] = id
		}
	}
	roles := templateRoles
	if byRole["main"] == 0 {
		roles = slices.DeleteFunc(slices.Clone(templateRoles), func(r string) bool { return r == "twohand" })
		at := slices.Index(roles, "shield")
		roles = slices.Insert(roles, at, "twohand")
	}
	for _, role := range roles {
		id := byRole[role]
		if id == 0 || held[role] {
			continue
		}
		p, ok, err := h.inventoryStore.ItemPatternByID(id)
		if err != nil || !ok {
			return lines, given, worn, fmt.Errorf("heirloom %d: %v", id, err)
		}
		if room <= 0 {
			lines = append(lines, fmt.Sprintf("  No room for %s", p.ItemName))
			continue
		}
		rowID, err := h.inventoryStore.AddCharItem(sid, id, 1, p.MaxHP, 0, 0)
		if err != nil {
			return lines, given, worn, fmt.Errorf("giving %q: %w", p.ItemName, err)
		}
		room--
		given++
		slot := templateFreeSlot(p.EquipSlot, taken)
		if slot < 0 {
			lines = append(lines, fmt.Sprintf("  In the bag: %s", p.ItemName))
			continue
		}
		if err := h.inventoryStore.SetEquipLoc(sid, rowID, slot); err != nil {
			return lines, given, worn, fmt.Errorf("wearing %q: %w", p.ItemName, err)
		}
		taken[slot] = true
		worn++
		lines = append(lines, fmt.Sprintf("  %s: %s", templateSlotNames[slot], p.ItemName))
	}
	return lines, given, worn, nil
}

// templateFreeSlot is an equip slot an item of that wear slot fits and may go in with
// nothing taken off, -1 for none.
func templateFreeSlot(wear int, taken map[int]bool) int {
	for e := 0; e <= 21; e++ {
		if !fitsEquip(wear, e) || taken[e] {
			continue
		}
		if !slices.ContainsFunc(equipDisplaces(wear, e), func(s int) bool { return taken[s] }) {
			return e
		}
	}
	return -1
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
	lines, given, worn, err := h.templateHeirlooms(sid, snap.Class)
	if err != nil {
		return nil, err
	}

	level := max(snap.Level, templateLevel)
	if snap.Level < templateLevel {
		if err := h.store.SetProgress(sid, templateLevel, entity.TotalXP(templateLevel, 0)); err != nil {
			return nil, fmt.Errorf("the level: %w", err)
		}
	}
	// the purse alone: the bank's tunar is written only under bankMu
	// (bank.go), and the snapshot's may be older than an auction's payment
	purse := max(snap.Tunar, templateTunar)
	if err := h.inventoryStore.SetPurse(sid, purse); err != nil {
		return nil, fmt.Errorf("the tunar: %w", err)
	}
	added, err := h.store.GrantEveryClassSpell(sid, snap.Class)
	if err != nil {
		return nil, fmt.Errorf("the spells: %w", err)
	}

	// The game's copy, then the client's: the level, the purse, what the worn
	// items add (full hit points and power at the new level), and the dump.
	if snap.Level < templateLevel {
		gw.SetPlayerLevel(sid, templateLevel)
		if unused, total, ok := gw.LiveTrainingPoints(player.ServerID); ok {
			if err := h.store.SetTrainingPoints(sid, unused, total); err != nil {
				log.Printf("Template: '%s' training points not kept: %v", player.Name, err)
			}
		}
	}
	bankMu.Lock()
	gw.SetPlayerTunar(sid, purse, player.BankTunar)
	bankMu.Unlock()
	h.applyEquipmentFromDB(player, sid, true)
	handsChanged(gw, h.inventoryStore, sess, player) // a fight swings with the new hands (combat.go)
	gw.SyncSelfChannels(sess)
	if now, ok := gw.PlayerSnapshot(player.ServerID); ok {
		h.queueMemoryDump(sess, &now)
	}

	head := fmt.Sprintf("Template: level %d, %d tunar, %d spells scribed, %d heirlooms given, %d of them worn",
		level, purse, added, given, worn)
	log.Printf("Template: '%s' (class %d, race %d) %s", snap.Name, snap.Class, snap.Race, head[len("Template: "):])
	return append([]string{head}, lines...), nil
}
