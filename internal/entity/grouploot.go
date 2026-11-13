package entity

import (
	"encoding/binary"
	"fmt"
	"log"
	"strings"

	"github.com/eqoa/server/internal/session"
)

// grouploot.go — a plain group's need before greed (the GM Jibrish's petition 1878,
// 2026-11-13: "Create an option under the group menu to allow players to turn on need
// before green loot system that raids has").
//
// A raid's loot rules (raidrules.go, handlers/raidloot.go) were a raid's alone: a group of
// four or fewer, not flagged, looted free for all. Now a plain group's leader may set its
// rule to need before greed and back: /grouploot needgreed|ffa (on|off too; /groupnbg the
// same; a PS2's /t grouploot), /raidloot in a plain group, and vanilla's menu's loot rule
// (protocol.OpcodeRaidCommand 1). Master looter stays a raid's. Not in combat, as a raid's.
// The rule is the group's (Group.rules.loot, as a raid's): kept while the group lasts,
// through a keep restart (KeptGroup), into a raid it becomes; gone with the group. Under
// it a group's kill is a raid's kill to the loot (raidKillLocked: its seats, its rolls,
// its rules' items, its lines), and its members' clients that show the raid's windows are
// told it in OpcodeRaidState with leader 0 (no raid), the rule byte 1 and flag bit 3
// (groupRuleFlag), their leader's bit 1 as a raid's.

// groupRuleFlag is OpcodeRaidState's flag bit for a plain group's own rule.
const groupRuleFlag = 8

// plainGroupNeedGreedLocked is p's group when it is no raid and its rule is need before
// greed, else nil. Must be called with the lock held.
func (w *World) plainGroupNeedGreedLocked(p *Player) *Group {
	if p == nil || p.GroupID == 0 {
		return nil
	}
	g := w.groups[p.GroupID]
	if g == nil || raidingLocked(g) || g.rules.loot != LootNeedGreed || len(g.Members) == 0 {
		return nil
	}
	return g
}

// groupLootStateLocked is OpcodeRaidState's body for p in no raid: its group's need before
// greed (the file's doc), else all zero, appended to b. Must be called with the lock held.
func (w *World) groupLootStateLocked(p *Player, b []byte) []byte {
	g := w.plainGroupNeedGreedLocked(p)
	if g == nil {
		return append(b, make([]byte, 14)...)
	}
	b = binary.LittleEndian.AppendUint32(b, 0)
	b = binary.LittleEndian.AppendUint32(b, 0)
	b = binary.LittleEndian.AppendUint32(b, 0)
	flags := byte(groupRuleFlag)
	if len(w.raidFoesLocked(g)) > 0 {
		flags |= 1
	}
	if g.Members[0] == p.ServerID {
		flags |= 2
	}
	return append(b, byte(g.rules.loot), flags)
}

// GroupLootRule is /grouploot (and /groupnbg): the word a rule (ParseLootRule's, or on
// for need before greed and off for free for all); none, the group's rule as it stands.
func (w *World) GroupLootRule(sess *session.Session, word string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return
	}
	g := w.groups[p.GroupID]
	if p.GroupID == 0 || g == nil {
		w.refuseLocked(sess, "You are not in a group.")
		return
	}
	word = strings.ToLower(strings.TrimSpace(word))
	if word == "" {
		w.raidLineLocked(sess, fmt.Sprintf("Your group's loot rule is %s. Its leader sets it: /grouploot needgreed or /grouploot ffa.", g.rules.loot))
		return
	}
	rule, ok := ParseLootRule(word)
	switch word {
	case "on":
		rule, ok = LootNeedGreed, true
	case "off":
		rule, ok = LootFreeForAll, true
	}
	if !ok {
		w.refuseLocked(sess, "Loot rules: /grouploot needgreed (on) or /grouploot ffa (off).")
		return
	}
	if raidingLocked(g) {
		w.raidLootRuleLocked(sess, p, rule)
		return
	}
	w.groupLootRuleLocked(sess, p, rule)
}

// groupLootRuleLocked sets a plain group's rule (the file's doc). Must be called with the
// lock held.
func (w *World) groupLootRuleLocked(sess *session.Session, p *Player, rule LootRule) {
	g := w.groups[p.GroupID]
	if g == nil || len(g.Members) == 0 {
		w.refuseLocked(sess, "You are not in a group.")
		return
	}
	if g.Members[0] != p.ServerID {
		w.refuseLocked(sess, "Only your group's leader can set its loot rule.")
		return
	}
	if rule == LootMaster {
		w.refuseLocked(sess, "Master looter is a raid's loot rule: a group loots free for all or need before greed.")
		return
	}
	if rule == g.rules.loot {
		w.raidLineLocked(sess, fmt.Sprintf("Your group's loot rule is %s already.", rule))
		return
	}
	if len(w.raidFoesLocked(g)) > 0 {
		w.refuseLocked(sess, "Your group is in combat: its loot rule cannot change until the fight is over.")
		return
	}
	g.rules.loot = rule
	log.Printf("Group: %s sets group %d's loot rule to %s", p.Name, g.ID, rule)
	w.groupSayColoredLocked(g, fmt.Sprintf("%s sets the group's loot rule: %s.", p.Name, rule), nil)
	w.raidStateNowLocked(g)
}
