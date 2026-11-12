package entity

import (
	"fmt"
	"strings"

	"github.com/eqoa/server/internal/database"
)

// npctitle.go — a line over an NPC's name (the GM Jibrish's petitions 1765
// and 1766, 2026-11-12: "Give the testing heroes vendor a title like
// mercenary's have. Make it say "XP Grinding Rewards"", "Give dabrish a
// title "XP/CM's"").
//
// The Mercenary Recruitment boards' words float over them on a plate
// (boardplate.go): a second entity in each view, after its board, shown by its
// name alone. A titled NPC brings one the same way, on its own body (as it is
// drawn now) with its skins hidden, nothing worn or held, not selectable, at
// titleScale of its size, so the plate's name stands over the NPC's own by a
// quarter of that height (a human's 2.096: half a unit over it), under the
// title. CLIENT's name plate is at the body's name node, wherever that is, so
// the same body is the one whose node is known to stand over the NPC's.
//
// The titles are table npctitle's, by the npcs row (database/npctitle.go),
// read at the start and by the GM panel's edit of the table; an NPC made
// again from its row takes its row's (applyNPCSettingsLocked). A copy of it
// (a spawn's) has none.

// titleScale is a title plate's size over its NPC's.
const titleScale float32 = 1.25

// maxTitle is the longest title shown: a name's 24 letters (the entity
// channel's name field).
const maxTitle = 24

// LoadNPCTitles reads table npctitle into the world and gives each NPC of a
// row its title. It reports how many rows have one.
func (w *World) LoadNPCTitles(db *database.SQLiteDB) (int, error) {
	rows, err := db.NPCTitleRows()
	if err != nil {
		return 0, fmt.Errorf("npctitle: %w", err)
	}
	m := make(map[EntityID]string, len(rows))
	for id, t := range rows {
		if t = cleanTitle(t); t != "" {
			m[EntityID(id)] = t
		}
	}
	w.SetNPCTitles(m)
	return len(m), nil
}

// SetNPCTitles sets the titles by NPC row and gives every NPC its own.
func (w *World) SetNPCTitles(m map[EntityID]string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.npcTitles = m
	for id, n := range w.NPCs {
		if n.Temporary {
			continue // a copy, a pet, a merc: no row of its own
		}
		n.Title = m[id]
	}
}

// cleanTitle is a title as shown: trimmed, cut to maxTitle bytes.
func cleanTitle(t string) string {
	t = strings.TrimSpace(t)
	if len(t) > maxTitle {
		t = strings.TrimSpace(t[:maxTitle])
	}
	return t
}

// serializeTitlePlate is the channel of n's title plate under objectID
// (serializePlate's for a titled NPC): n's place, facing, world, NPC bits and
// level, its body at titleScale of its size, under its title, nothing worn or
// held, its skins hidden and not selectable.
func serializeTitlePlate(objectID uint32, n *NPC) []byte {
	model, size := n.Body()
	if size <= 0 {
		size = 1
	}
	p := &NPC{
		ServerID: plateID(n),
		Name:     n.Title,
		X:        n.X, Y: n.Y, Z: n.Z,
		Facing:  n.Facing,
		World:   n.World,
		ModelID: int64(model),
		Size:    size * titleScale,
		NPCType: n.NPCType,
		Level:   n.Level,
		Race:    "0",
	}
	return plateBytes(objectID, p)
}
