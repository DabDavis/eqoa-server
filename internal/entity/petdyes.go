package entity

import (
	"encoding/binary"
	"fmt"
	"log"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// petdyes.go — a pet's dyes: two colours on two areas of its body, which the
// Pet Groomer puts on (handlers/petgroomer.go; the owner, 2026-11-16, of
// Zerkziee's drachnid: "make it have 2 different color areas that can be
// changed like armor dye (will be a template for pet color dye). then add Pet
// Groomer near Equipment Dyer also").
//
// A character's dyes are kept by the pet's body (table petdyes: each look of a
// pet keeps its own), and a body's two areas are table petdyeareas' (area 2 a
// set of the body's palette materials, area 1 the rest; a body with no row is
// one area, the whole of it). A client that draws them answers the offer
// (protocol.OpcodePetDyesOffer) and is told, in the world, which pets on its
// entity slots wear dyes and where (OpcodePetDyes), as the capes in view are
// told (capes.go): the entity channels carry none of it, so a PS2 reads the
// disc's 162 bytes and draws every pet in its body's own colours.

// PetDye is a pet body's two dyes, 0xRRGGBBAA each, 0 none.
type PetDye [database.PetDyeAreas]uint32

// petDyed is one entry of OpcodePetDyes.
type petDyed struct {
	id, look uint32
	dye      PetDye
	mask     uint64
}

// SetPetDyeAreas sets table petdyeareas (as database.PetDyeAreaRows reads it)
// and has every client that draws pet dyes told its view's again.
func (w *World) SetPetDyeAreas(areas map[uint32]database.PetDyeArea) {
	m := make(map[uint32]database.PetDyeArea, len(areas))
	for k, v := range areas {
		m[k] = v
	}
	w.petDyeAreas.Store(&m)
}

// LoadPetDyeAreas reads table petdyeareas into the world: at the start and
// after a GM's edit. Call it without the database held. It reports how many
// bodies it read.
func (w *World) LoadPetDyeAreas(db *database.SQLiteDB) (int, error) {
	areas, bad, err := db.PetDyeAreaRows()
	if err != nil {
		return 0, fmt.Errorf("petdyeareas: %w", err)
	}
	for _, b := range bad {
		log.Printf("PetDyes: %s: left out", b)
	}
	w.SetPetDyeAreas(areas)
	return len(areas), nil
}

// PetDyeArea is a body's areas (petdyeareas), and whether it has a row: a
// body without one is one area, the whole of it.
func (w *World) PetDyeArea(look uint32) (database.PetDyeArea, bool) {
	if m := w.petDyeAreas.Load(); m != nil {
		a, ok := (*m)[look]
		return a, ok
	}
	return database.PetDyeArea{}, false
}

// SetPetDyes puts a character's pet dyes (table petdyes, by body) on its
// player, where it is in the world: the next pass tells the clients that see
// its pet. It reports the player's name and whether it was in the world.
func (w *World) SetPetDyes(charID uint32, d map[uint32][database.PetDyeAreas]uint32) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.playerByCharacterLocked(charID)
	if p == nil {
		return "", false
	}
	p.PetDyes = make(map[uint32]PetDye, len(d))
	for k, v := range d {
		if v != (PetDye{}) {
			p.PetDyes[k] = v
		}
	}
	return p.Name, true
}

// PetLook is a player's pet as the Pet Groomer sees it.
type PetLook struct {
	Name string
	// Look is its body's model DictID, the key its dyes are kept by
	Look uint32
	// Area is its body's areas (petdyeareas); TwoAreas says whether it has a
	// second
	Area database.PetDyeArea
	// Dye is what it wears now
	Dye PetDye
}

// PetLookOf is the player's pet, alive, as the Groomer dyes it; false when
// it has none.
func (w *World) PetLookOf(sess *session.Session) (PetLook, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	pet := w.petOfLocked(p)
	if pet == nil {
		return PetLook{}, false
	}
	look := uint32(pet.ModelID)
	area, ok := w.PetDyeArea(look)
	if !ok {
		area = database.PetDyeArea{Look: look, Area1: "Body"}
	}
	return PetLook{Name: pet.Name, Look: look, Area: area, Dye: p.PetDyes[look]}, true
}

// isOwnedPet is whether the NPC is a player's pet (its own summoning or a
// charm), not a merc, a merc's pet or a familiar.
func (n *NPC) isOwnedPet() bool {
	return n != nil && n.Owner != 0 && !n.isMerc() && !n.isMercPet() && !n.isFamiliar()
}

// petDyesInViewLocked is the dyed pets on sess's entity slots, by the slot's
// id, in slot order: each a pet whose owner has dyes on its body. Must be
// called with the lock held.
func (w *World) petDyesInViewLocked(sess *session.Session) []petDyed {
	vs, _ := sess.VisibilityState().(*VisibilityState)
	if vs == nil {
		return nil
	}
	var out []petDyed
	for i, id := range vs.ChannelEntity {
		if id == 0 || i >= len(vs.ChannelObjectID) {
			continue
		}
		n := w.NPCs[id]
		if !n.isOwnedPet() || n.Dead {
			continue
		}
		owner := w.Players[n.Owner]
		if owner == nil || len(owner.PetDyes) == 0 {
			continue
		}
		look := uint32(n.ModelID)
		d, ok := owner.PetDyes[look]
		if !ok || d == (PetDye{}) {
			continue
		}
		var mask uint64
		if a, ok := w.PetDyeArea(look); ok && a.TwoAreas() {
			mask = a.Mask
		} else {
			d[1] = 0 // one area: the second's dye, if any, shows nowhere
			if d[0] == 0 {
				continue
			}
		}
		out = append(out, petDyed{id: vs.ChannelObjectID[i], look: look, dye: d, mask: mask})
	}
	return out
}

// PetDyesMost is the most pets one OpcodePetDyes carries.
const PetDyesMost = 255

// encodePetDyes is OpcodePetDyes's body: a u8 count, then each pet's u32 id,
// u32 body, u32 colours of areas 1 and 2 and u64 area 2's materials,
// little-endian (the first PetDyesMost).
func encodePetDyes(pets []petDyed) []byte {
	if len(pets) > PetDyesMost {
		pets = pets[:PetDyesMost]
	}
	out := make([]byte, 1, 1+24*len(pets))
	out[0] = byte(len(pets))
	for _, p := range pets {
		out = binary.LittleEndian.AppendUint32(out, p.id)
		out = binary.LittleEndian.AppendUint32(out, p.look)
		out = binary.LittleEndian.AppendUint32(out, p.dye[0])
		out = binary.LittleEndian.AppendUint32(out, p.dye[1])
		out = binary.LittleEndian.AppendUint64(out, p.mask)
	}
	return out
}

// ResendPetDyes makes the next pass send the session's client its pet dyes in
// view whatever it was sent before (a MemoryDump, a world change, its
// answer). Safe without the world lock.
func (vs *VisibilityState) ResendPetDyes() {
	if vs != nil {
		vs.petDyesDue.Store(true)
	}
}

// petDyesSyncSystem sends each client that draws pet dyes the set its view
// shows (petDyesInViewLocked) when it changed, or when it is due again
// (ResendPetDyes). Every tick, after the visibility system has put this
// tick's entities on their slots, so a pet's channel and its dyes go out
// together. A session that never asked (a PS2's) is skipped. Must be called
// with the lock held.
func (w *World) petDyesSyncSystem() {
	if w.SessionManager == nil {
		return
	}
	w.SessionManager.ForEach(func(s *session.Session) bool {
		if !s.PetDyes() || !s.CharacterInWorld() {
			return true
		}
		vs := GetOrCreateVisibility(s)
		body := encodePetDyes(w.petDyesInViewLocked(s))
		if !vs.petDyesDue.Swap(false) && string(body) == vs.petDyesSent {
			return true
		}
		vs.petDyesSent = string(body)
		s.Queue().AddWithSequence(capesMessage(protocol.OpcodePetDyes, body))
		return true
	})
}
