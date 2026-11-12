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
// A title's colour (the GM Jibrish's petitions 1790 and 1791, 2026-11-12:
// "Color the XP/CM title on Dabrish green, semi bright green so it shows but
// not too green as to be unreadable", and the Testing Heroes Vendor's) is one
// of CLIENT's three name colours, the same for every viewer: a name plate's
// is VICSprite::NameColors[c] (gold 212,192,111; blue 100,175,220; green
// 90,255,80), c picked by VICreatureEntity::ApplyLook alone from the actor's
// own channel: 0 when its m_p256 (byte 142) is under 2, else 2 when its
// m_p299 (byte 157) has bit 0, else 1. No viewer's level, con or faction
// enters it (con colours are the target ring's), and vanilla picks it the
// same way (play.rs's NAME_COLORS). So a plate coloured blue or green carries
// byte 142 = 3, a pet's (pet.go), which CLIENT's target cycle and
// TargetCentre skip besides its byte 26 (and IsSelectable takes only 2), and
// green byte 157 = 1 too. The NPC's own name keeps its colour.
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
	c := make(map[EntityID]uint8)
	for id, r := range rows {
		if t := cleanTitle(r.Title); t != "" {
			m[EntityID(id)] = t
			if k := TitleColor(r.Color); k != 0 {
				c[EntityID(id)] = k
			}
		}
	}
	w.SetNPCTitleColors(c)
	w.SetNPCTitles(m)
	return len(m), nil
}

// titleColors is npctitle's color by name: CLIENT's NameColors index.
var titleColors = map[string]uint8{"": 0, "gold": 0, "yellow": 0, "blue": 1, "green": 2}

// TitleColor is the NameColors index of npctitle's color, any case; 0 (an
// NPC's own gold) for one it does not know.
func TitleColor(name string) uint8 {
	return titleColors[strings.ToLower(strings.TrimSpace(name))]
}

// SetNPCTitleColors sets the titles' colours by NPC row (NameColors indexes;
// an NPC missing is 0, as its own name). SetNPCTitles after it gives them.
func (w *World) SetNPCTitleColors(m map[EntityID]uint8) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.npcTitleColors = m
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
		n.TitleColor = w.npcTitleColors[id]
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
// held, its skins hidden and not selectable, in its title's colour.
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
	buf := plateBytes(objectID, p)
	if n.TitleColor != 0 {
		buf[142] = 3 // m_p256 >= 2: a pet's name colour, not selectable
		buf[157] = 0
		if n.TitleColor == 2 {
			buf[157] = 1 // m_p299 bit 0: NameColors[2], green
		}
	}
	return buf
}
