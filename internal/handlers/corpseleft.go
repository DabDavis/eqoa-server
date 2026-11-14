package handlers

import (
	"log"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// corpseleft.go — a corpse left where its creature fell, as a copy, so it lies 20 minutes
// while its creature comes back as before (entity/corpselinger.go: the GM Jibrish's Live
// petition 5035). Everything the loot knows of the corpse goes to the copy's id: its rolled
// items and coins, its kill group and open time, a loot step's drops, a raid's kill not yet
// rolled, its rolls, winners and master looter (and the corpse a master looter was last
// listed); the windows open on it are closed (the client lets go of the creature, and opens
// the copy afresh). Then the World is told the copy is the loot's (CorpseMoved).

// moveCorpse is the World's hook for corpses left as copies (OnCorpseLeft).
func (h *NPCWindowHandler) moveCorpse(moves []entity.CorpseLeft) {
	ls := h.loots()
	ls.mu.Lock()
	for _, m := range moves {
		h.moveCorpseLocked(ls, m)
	}
	ls.mu.Unlock()
	if h.corpseMoved != nil {
		for _, m := range moves {
			h.corpseMoved(m.To)
		}
	}
}

// moveCorpseLocked moves one corpse's loot (the file's doc). Must be called with ls.mu held.
func (h *NPCWindowHandler) moveCorpseLocked(ls *lootState, m entity.CorpseLeft) {
	from, to := m.From, m.To
	for s, open := range ls.open {
		if open == from {
			delete(ls.open, s)
			delete(ls.since, s)
			queueReliable(s, protocol.OpcodeClientCloseLoot, closeLootPayload())
		}
	}
	if c := ls.corpses[from]; c != nil {
		delete(ls.corpses, from)
		if c.died.Equal(m.Died) {
			ls.corpses[to] = c
		}
	}
	if d := ls.drops[from]; d != nil {
		delete(ls.drops, from)
		if d.died.Equal(m.Died) {
			ls.drops[to] = d
		}
	}
	for _, r := range ls.rolls {
		if r.corpse == from && r.died.Equal(m.Died) {
			r.corpse = to
		}
	}
	for char, id := range ls.listed {
		if id == from {
			ls.listed[char] = to
		}
	}
	if v, ok := h.killRights.LoadAndDelete(from); ok {
		if r := v.(lootRight); r.died.Equal(m.Died) {
			h.killRights.Store(to, r)
		}
	}
	if v, ok := h.raids.kills.LoadAndDelete(from); ok {
		if k := v.(entity.KillCredit); k.Died.Equal(m.Died) {
			k.NPCID = to
			h.raids.kills.Store(to, k)
		}
	}
	log.Printf("Loot: the corpse of creature %d is %d's now (left to lie)", from, to)
}
