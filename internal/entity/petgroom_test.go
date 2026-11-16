package entity

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Zerkziee's construct as Jarek's Creation (cmpetlook.go) and the pets' dyes
// (petdyes.go; the owner, 2026-11-16).

// jarekLook is Jarek's Creation's body, 0x15764DA8, as cmcatalog's effects
// write it ("PetLook:360074664").
const jarekLook = 0x15764DA8

// petLookRanks is a catalog of the fire elemental's look for character 77
// alone (as Zerkziee's construct: requires the summoning spell, 717 in the
// pet rig, or 718 standing in for it), and a rank of another body requiring
// no spell for anyone.
func petLookRanks() []database.CMRankRow {
	return []database.CMRankRow{
		{ID: 900917, Classes: 32767, Races: 2047, Name: "Construct (Jarek's Creation)", Level: 1, Requires: "Fire Elemental@1",
			RequireSpells: "717|718", Effects: "PetLook:360074664,OnlyFor:77", Enabled: true},
		{ID: 900918, Classes: 32767, Races: 2047, Name: "Any Pet's Look", Level: 1, Effects: "PetLook:305419896", Enabled: true},
	}
}

// A rank of OnlyFor is offered to its one character alone, and listed by no
// class and race (OfferedTo: the archivist's and the mercs' lists); a rank
// without it is offered by class and race as before (the control).
func TestARankForOneCharacterAlone(t *testing.T) {
	w := &World{}
	w.SetCMCatalog(petLookRanks())
	r, any := w.cms.byID[900917], w.cms.byID[900918]
	if r.PetLook != jarekLook || r.OnlyFor != 77 || r.Effects != ([CMSlots]int{}) {
		t.Fatalf("parsed PetLook %#x OnlyFor %d effects %v: want 0x15764DA8, 77 and no slot's", r.PetLook, r.OnlyFor, r.Effects)
	}
	him, other := &Player{CharServerID: 77, Class: 1, Race: 3}, &Player{CharServerID: 78, Class: 1, Race: 3}
	if !r.offered(him) || r.offered(other) || r.OfferedTo(1, 3) {
		t.Errorf("offered him %v, another %v, by class and race %v: want true, false, false", r.offered(him), r.offered(other), r.OfferedTo(1, 3))
	}
	if !any.offered(him) || !any.offered(other) || !any.OfferedTo(1, 3) {
		t.Error("the control: a rank for everyone is offered to everyone")
	}
	r.Enabled = false
	if r.offered(him) {
		t.Error("a disabled rank offered to its character")
	}
}

// The rank changes the body of a pet of the spells it requires, at its
// wearers' size, and nothing else of it; a pet of another spell keeps its
// own body, and so does the pet once the rank is gone (the controls). The
// rank requiring no spell changes every pet; of two that apply, the
// higher id's.
func TestAPetLookRankChangesTheBodyAlone(t *testing.T) {
	r, _ := sheetRig(t, sheetFireRow)
	r.w.SetCMCatalog(petLookRanks())
	r.w.NPCs[4000] = &NPC{ServerID: 4000, Name: "Jarek's Creation", ModelID: jarekLook, Size: 1.25}
	plain, _ := r.summon(t, fireElemental(), 0)
	want := *plain
	r.p.CMs = map[int]int{900917: 0}
	r.p.CharServerID = 77
	r.p.Power = r.p.MaxPower
	pet, _ := r.summon(t, fireElemental(), 60_000_000_000)
	if uint32(pet.ModelID) != jarekLook || pet.Size != 1.25 {
		t.Fatalf("with the rank: body %#x size %.2f, want Jarek's Creation's at its wearers' 1.25", uint32(pet.ModelID), pet.Size)
	}
	if pet.Name != want.Name || pet.Level != want.Level || pet.MaxHP != want.MaxHP || pet.WeaponDamage != want.WeaponDamage ||
		pet.sheet != want.sheet || pet.Class != want.Class {
		t.Errorf("with the rank the pet changed more than its look: %q L%d HP %d wd %d class %d, want %q L%d HP %d wd %d class %d",
			pet.Name, pet.Level, pet.MaxHP, pet.WeaponDamage, pet.Class, want.Name, want.Level, want.MaxHP, want.WeaponDamage, want.Class)
	}
	r.w.mu.Lock()
	if got := r.w.cmPetLookLocked(r.p, 999); got != 0 {
		t.Errorf("a pet of a spell the rank does not require: look %#x, want none", got)
	}
	if got := r.w.cmPetLookLocked(r.p, 718); got != jarekLook {
		t.Errorf("a pet of the required spell's alternative: look %#x, want Jarek's Creation", got)
	}
	r.p.CMs = map[int]int{900917: 0, 900918: 0}
	if got := r.w.cmPetLookLocked(r.p, 717); got != 0x12345678 {
		t.Errorf("of two ranks: %#x, want the higher id's", got)
	}
	r.p.CMs = map[int]int{900918: 0}
	if got := r.w.cmPetLookLocked(r.p, 999); got != 0x12345678 {
		t.Errorf("a rank requiring no spell: %#x, want its look on any pet", got)
	}
	if got := r.w.petLookSizeLocked(0x12345678); got != 1 {
		t.Errorf("a body nobody wears: size %.2f, want 1", got)
	}
	r.p.CMs = nil
	r.w.mu.Unlock()
	r.p.Power = r.p.MaxPower
	back, _ := r.summon(t, fireElemental(), 120_000_000_000)
	if back.ModelID != want.ModelID || back.Size != want.Size {
		t.Errorf("the rank gone: body %#x size %.2f, want the spell's own %#x %.2f", uint32(back.ModelID), back.Size, uint32(want.ModelID), want.Size)
	}
}

// petDyeSets takes the OpcodePetDyes bodies queued for s since the last call
// (GetPending empties the queue).
func petDyeSets(s *session.Session) [][]byte {
	var out [][]byte
	for _, m := range s.Queue().GetPending() {
		d := m.Data()
		if len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodePetDyes {
			out = append(out, append([]byte(nil), d[2:]...))
		}
	}
	return out
}

// petDyesBody is a set's body from its entries.
func petDyesBody(e ...petDyed) []byte { return encodePetDyes(e) }

// A client that draws pet dyes is sent the dyed pets on its entity slots, by
// the slot's id: the owner's dyes on the pet's body, area 2's materials by
// petdyeareas; a body of one area its first colour alone; again only when the
// set changes or it is due (ResendPetDyes). The controls: a client that never
// answered is sent nothing; a pet in a body its owner never dyed, an NPC no
// one owns and a merc are left out.
func TestPetDyesInViewOnlyToClientsThatDrawThem(t *testing.T) {
	aw := newAssistWorld(t)
	w := aw.w
	pet := &NPC{ServerID: 2100, Name: "Construct", ModelID: jarekLook, Owner: aw.b.ServerID}
	w.NPCs[pet.ServerID] = pet
	merc := &NPC{ServerID: 2101, Name: "Merc", ModelID: jarekLook, Owner: aw.b.ServerID, merc: &mercState{}}
	w.NPCs[merc.ServerID] = merc
	vs := GetOrCreateVisibility(aw.asker)
	vs.ChannelEntity[5], vs.ChannelObjectID[5] = pet.ServerID, 100009
	vs.ChannelEntity[6], vs.ChannelObjectID[6] = merc.ServerID, 100010
	aw.b.CharServerID = 55
	aw.b.PetDyes = map[uint32]PetDye{jarekLook: {0xD10000FF, 0x008000FF}}
	aw.asker.SetPetDyes(true)

	w.petDyesSyncSystem()
	one := petDyesBody(petDyed{id: 100009, look: jarekLook, dye: PetDye{0xD10000FF, 0}})
	if got := petDyeSets(aw.asker); len(got) != 1 || !bytes.Equal(got[0], one) {
		t.Fatalf("a body of one area: %x, want %x (its first colour alone)", got, one)
	}
	if got := petDyeSets(aw.leader); len(got) != 0 {
		t.Fatalf("the leader's client never answered and was sent %x", got)
	}
	w.SetPetDyeAreas(map[uint32]database.PetDyeArea{jarekLook: {Look: jarekLook, Area1: "Torso", Area2: "Spider body", Mask: 0b1010}})
	w.petDyesSyncSystem()
	two := petDyesBody(petDyed{id: 100009, look: jarekLook, dye: PetDye{0xD10000FF, 0x008000FF}, mask: 0b1010})
	if got := petDyeSets(aw.asker); len(got) != 1 || !bytes.Equal(got[0], two) {
		t.Fatalf("two areas: %x, want %x", got, two)
	}
	w.petDyesSyncSystem()
	if got := petDyeSets(aw.asker); len(got) != 0 {
		t.Fatalf("unchanged, sent again: %x", got)
	}
	GetOrCreateVisibility(aw.asker).ResendPetDyes()
	w.petDyesSyncSystem()
	if got := petDyeSets(aw.asker); len(got) != 1 || !bytes.Equal(got[0], two) {
		t.Fatalf("due again: %x", got)
	}
	pet.ModelID = 0x9CF72381 // summoned anew in a body never dyed
	w.petDyesSyncSystem()
	if got := petDyeSets(aw.asker); len(got) != 1 || !bytes.Equal(got[0], []byte{0}) {
		t.Fatalf("a body never dyed: %x, want the empty set", got)
	}
	if name, ok := w.SetPetDyes(55, map[uint32][database.PetDyeAreas]uint32{0x9CF72381: {0x0000FFFF, 0}}); !ok || name != "Leader" {
		t.Fatalf("SetPetDyes: %q %v", name, ok)
	}
	w.petDyesSyncSystem()
	blue := petDyesBody(petDyed{id: 100009, look: 0x9CF72381, dye: PetDye{0x0000FFFF, 0}})
	if got := petDyeSets(aw.asker); len(got) != 1 || !bytes.Equal(got[0], blue) {
		t.Fatalf("its new body dyed: %x, want %x", got, blue)
	}
}

// OpcodePetDyes's body: a u8 count, then 24 bytes a pet, little-endian.
func TestEncodePetDyes(t *testing.T) {
	got := encodePetDyes([]petDyed{{id: 0x01020304, look: jarekLook, dye: PetDye{0xAABBCCDD, 0x11223344}, mask: 1<<63 | 5}})
	want := []byte{1, 4, 3, 2, 1, 0xA8, 0x4D, 0x76, 0x15, 0xDD, 0xCC, 0xBB, 0xAA, 0x44, 0x33, 0x22, 0x11, 5, 0, 0, 0, 0, 0, 0, 0x80}
	if !bytes.Equal(got, want) {
		t.Fatalf("%x, want %x", got, want)
	}
	if b := encodePetDyes(nil); !bytes.Equal(b, []byte{0}) {
		t.Fatalf("none: %x", b)
	}
}
