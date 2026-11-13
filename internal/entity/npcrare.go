package entity

import (
	"fmt"
	"log"
	"time"

	"github.com/eqoa/server/internal/database"
)

// npcrare.go — a rare spawn on a placeholder (table npcrare, database/npcrare.go;
// the server owner, 2026-11-13: Lord Koreshaa, "is not in our database ...
// wehali myrmidon in kerran isle is a placeholder").
//
// Each time a placeholder's row comes back from a death (respawnNPCLocked) it
// rolls its rule's chance: a hit, while no creature of the rare's name stands
// alive in the world (one at a time, however many placeholders it has), makes it
// the rare, from the rare's row (its body, level, class template by its name,
// loot by its name) on the placeholder's own spot, as a camp's spawn pops a kind
// (campspawn.go's makeKindLocked: its pointer and row kept, out of every view for
// NPCRefreshHold so every client makes it again as itself); a miss makes it its
// own row's kind again (the rare it stood as last life, gone). The rare's own row
// stands out of the world (npcspawnonly by its row, "#id"): it is there to be
// copied, never on a clock of its own; its copy on the placeholder's spot comes
// back, once dead, at the placeholder's respawn time and rolls again. A restart
// that keeps the players keeps a placeholder standing as its rare (keepnpc.go).
// Rules are read at the start and at a GM's edit of the table in the panel.

// rareRule is a placeholder's rule: its own row, the rare's, the chance.
type rareRule struct {
	own, rare *campRec
	chance    float64 // percent, (0, 100]
}

// rareRand is the rare's roll, in percent [0, 100) (a test's own).
var rareRand func(w *World) float64 = func(w *World) float64 { return w.campRand().Float64() * 100 }

// LoadRareSpawns reads table npcrare into the world: at the start and after a
// GM's edit of it. Call it without the database held, the NPCs loaded. A rule
// whose placeholder row is gone, or whose rare no row is called, is logged and
// left out. It reports how many rules it read.
func (w *World) LoadRareSpawns(db *database.SQLiteDB) (int, error) {
	rows, err := db.NPCRares()
	if err != nil {
		return 0, fmt.Errorf("npcrare: %w", err)
	}
	store := database.NewNPCStore(db)
	classes, err := store.NPCClasses()
	if err != nil {
		return 0, fmt.Errorf("npcclass: %w", err)
	}
	rec := func(id int) (*campRec, error) {
		r, err := store.GetNPC(id)
		if err != nil {
			return nil, err
		}
		return &campRec{rec: *r, class: classes[database.NPCClassKey(r.NPCName)]}, nil
	}
	rules := map[EntityID]*rareRule{}
	for _, r := range rows {
		own, err := rec(r.NPCRow)
		if err != nil {
			log.Printf("Rares: placeholder row %d: %v (left out)", r.NPCRow, err)
			continue
		}
		rid, err := db.NPCRowNamed(r.Rare)
		if err != nil {
			return 0, fmt.Errorf("npcrare %d: %w", r.NPCRow, err)
		}
		if rid == 0 {
			log.Printf("Rares: placeholder row %d: no NPC row is called %q (left out)", r.NPCRow, r.Rare)
			continue
		}
		if rid == r.NPCRow {
			log.Printf("Rares: placeholder row %d is its own rare %q (left out)", r.NPCRow, r.Rare)
			continue
		}
		rare, err := rec(rid)
		if err != nil {
			return 0, fmt.Errorf("npcrare %d: the rare's row %d: %w", r.NPCRow, rid, err)
		}
		rules[EntityID(r.NPCRow)] = &rareRule{own: own, rare: rare, chance: r.Chance}
	}
	w.mu.Lock()
	w.rares = rules
	w.mu.Unlock()
	return len(rules), nil
}

// RareSummary says how many placeholders stand for how many rares, for the log.
func (w *World) RareSummary() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	names := map[string]bool{}
	for _, r := range w.rares {
		names[database.NPCClassKey(r.rare.rec.NPCName)] = true
	}
	return fmt.Sprintf("%d placeholders (npcrare) for %d rare spawns", len(w.rares), len(names))
}

// rareAliveLocked reports a creature of the rare's name alive in the world other
// than npc (a GM's #spawn copy counts: one at a time). Must be called with the
// lock held.
func (w *World) rareAliveLocked(key string, npc *NPC) bool {
	for id, n := range w.NPCs {
		if id == npc.ServerID || n.Dead || n.SpawnOnly {
			continue
		}
		if database.NPCClassKey(n.Name) == key {
			return true
		}
	}
	return false
}

// rareRekindLocked is a placeholder coming back from a death: its rare at its
// rule's chance, else its own kind (the file's doc). Must be called with the
// lock held.
func (w *World) rareRekindLocked(npc *NPC, now time.Time) {
	r := w.rares[npc.ServerID]
	if r == nil {
		return
	}
	key := database.NPCClassKey(npc.Name)
	rareKey := database.NPCClassKey(r.rare.rec.NPCName)
	if rareRand(w) < r.chance && !w.rareAliveLocked(rareKey, npc) {
		if key != rareKey {
			log.Printf("Rares: placeholder %d (%q) comes back as its rare %q", npc.ServerID, npc.Name, r.rare.rec.NPCName)
			w.makeKindLocked(npc, r.own.rec, r.rare, now.Add(NPCRefreshHold))
		}
		return
	}
	if key != database.NPCClassKey(r.own.rec.NPCName) {
		log.Printf("Rares: placeholder %d comes back as %q (it was %q)", npc.ServerID, r.own.rec.NPCName, npc.Name)
		w.makeKindLocked(npc, r.own.rec, r.own, now.Add(NPCRefreshHold))
	}
}

// rareKindChangedLocked reports a placeholder standing as its rare (keepnpc.go
// carries it). Must be called with the lock held.
func (w *World) rareKindChangedLocked(npc *NPC) bool {
	r := w.rares[npc.ServerID]
	return r != nil && database.NPCClassKey(npc.Name) == database.NPCClassKey(r.rare.rec.NPCName)
}

// rareKeptKindLocked makes npc, a placeholder just made from its row, its rare
// when a restart's file says it stood as it (name); false otherwise. Must be
// called with the lock held.
func (w *World) rareKeptKindLocked(npc *NPC, name string) bool {
	r := w.rares[npc.ServerID]
	if r == nil || database.NPCClassKey(name) != database.NPCClassKey(r.rare.rec.NPCName) ||
		database.NPCClassKey(npc.Name) == database.NPCClassKey(name) {
		return false
	}
	w.makeKindLocked(npc, r.own.rec, r.rare, time.Time{})
	return true
}
