package entity

import (
	"fmt"

	"github.com/eqoa/server/internal/database"
)

// npcKey is an NPC's class key (database.NPCClassKey) for the name it was
// made from. The per-viewer channel work (a decoration's art, a name's dyes
// and robe) looks rows up by it for each session with the NPC in view each
// tick (a profile of the Live shard, 2026-11-20: thousands of channels a
// tick), and the key is a pure function of the name, so it is kept until the
// name changes.
type npcKey struct {
	name, key string
	ok        bool
}

// classKey is database.NPCClassKey(n.Name). It writes the NPC: only for
// callers holding the world's lock to write (the tick's channel passes), never
// under a read lock, where two readers would write it at once.
func (n *NPC) classKey() string {
	if !n.key.ok || n.key.name != n.Name {
		n.key = npcKey{name: n.Name, key: database.NPCClassKey(n.Name), ok: true}
	} else if npcKeyCheck {
		if k := database.NPCClassKey(n.Name); k != n.key.key {
			panic(fmt.Sprintf("npckey: %q kept the key %q, NPCClassKey says %q", n.Name, n.key.key, k))
		}
	}
	return n.key.key
}

// npcKeyCheck has classKey check a kept key against NPCClassKey (this
// package's tests set it, main_test.go).
var npcKeyCheck bool
