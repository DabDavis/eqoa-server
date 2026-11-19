package entity

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// A player's catapult (cmpetlook.go's PetArt, decorlights.go's
// petArtViewLocked; the owner, 2026-11-17: "where [a GM] is is a catapult prop
// ... lets give it to [a player]").

// catapultArt is Tunaria's catapult prop (zone 108), 0xE64B8D78, as
// cmcatalog's effects write it ("PetArt:3863711096").
const catapultArt = 0xE64B8D78

// catapultRanks is petLookRanks with the catapult's rank for character 77.
func catapultRanks() []database.CMRankRow {
	return append(petLookRanks(), database.CMRankRow{ID: 900919, Classes: 32767, Races: 2047, Name: "Construct (Catapult)", Level: 1,
		Requires: "Fire Elemental@1", RequireSpells: "717|718", Effects: "PetArt:3863711096,OnlyFor:77", Enabled: true})
}

// A PetArt rank gives the pets of its spells the art and leaves their body as
// it is (a PetLook rank's body still applies with it: what a PS2 sees); the
// art goes, in byte 28's model, only to a client of layout 3 in a world where
// a pet row of decorart has it, and the pet stays selectable. The controls:
// a PS2 and a layout 2 client get the body; a world with no row gets the
// body; the rank gone, the pet has no art; a pet row's label never makes an
// NPC of that name a decoration.
func TestAPetArtRankDressesThePetForTheClientsThatDrawIt(t *testing.T) {
	r, _ := sheetRig(t, sheetFireRow)
	r.w.SetCMCatalog(catapultRanks())
	if a := r.w.cms.byID[900919]; a.PetArt != catapultArt || a.PetLook != 0 || a.OnlyFor != 77 {
		t.Fatalf("parsed PetArt %#x PetLook %#x OnlyFor %d", a.PetArt, a.PetLook, a.OnlyFor)
	}
	plain, _ := r.summon(t, fireElemental(), 0)
	body := plain.ModelID
	if plain.Art != 0 {
		t.Fatalf("no rank: art %#x", plain.Art)
	}
	r.p.CMs = map[int]int{900919: 0}
	r.p.CharServerID = 77
	r.p.Power = r.p.MaxPower
	pet, _ := r.summon(t, fireElemental(), 60_000_000_000)
	if pet.Art != catapultArt || pet.ModelID != body {
		t.Fatalf("with the rank: art %#x body %#x, want the catapult over the spell's own %#x", pet.Art, uint32(pet.ModelID), uint32(body))
	}
	// a decoration named as the pet row's label: still itself
	other := &NPC{ServerID: 4100, Name: "Catapult", ModelID: 0x15BDF1BD, Size: 1, World: pet.World, X: pet.X + 2, Z: pet.Z}
	r.w.NPCs[other.ServerID] = other
	r.w.SpatialIndex.Insert(other.ServerID, other.X, other.Z)
	art := map[string]database.DecorArt{"catapult": {Name: "catapult", Model: catapultArt, World: int(pet.World), Zone: 108, Kind: database.DecorArtPet,
		Shot: 0xBFEE4CCD, ShotScale: 0.2}}
	r.w.SetDecorLights(nil, art, []database.DecorArt{art["catapult"]})
	view := func(layout uint8, id EntityID) []byte {
		r.sess.SetDecorLights(layout)
		r.w.mu.Lock()
		defer r.w.mu.Unlock()
		b := r.w.serializeEntityForChannel(r.sess, id, 7)
		if len(b) < 32 {
			t.Fatalf("serialize %d: %d bytes", id, len(b))
		}
		return b
	}
	for _, c := range []struct {
		layout uint8
		want   uint32
	}{{0, uint32(body)}, {2, uint32(body)}, {3, catapultArt}} {
		b := view(c.layout, pet.ServerID)
		if m := binary.LittleEndian.Uint32(b[28:]); m != c.want || b[26]&1 == 0 {
			t.Errorf("layout %d: model %08X byte 26 %d, want %08X and selectable", c.layout, m, b[26], c.want)
		}
	}
	if b := view(3, other.ServerID); binary.LittleEndian.Uint32(b[28:]) != 0x15BDF1BD || b[26]&1 == 0 {
		t.Errorf("an NPC named as the pet row's label: model %08X byte 26 %d, want its own and selectable", binary.LittleEndian.Uint32(b[28:]), b[26])
	}
	// the Pet Groomer dyes the art its owner's client draws (the owner,
	// 2026-11-18: "the catapult pet needs two tone color regions for pet
	// groomer"): the catapult to a layout 3 client, the body to a PS2
	for _, c := range []struct {
		layout uint8
		want   uint32
	}{{3, catapultArt}, {0, uint32(body)}} {
		r.sess.SetDecorLights(c.layout)
		if l, ok := r.w.PetLookOf(r.sess); !ok || l.Look != c.want {
			t.Errorf("layout %d: the groomer's look %#x, want %#x", c.layout, l.Look, c.want)
		}
		if _, look, _, _, _, ok := r.w.PetDyePreview(r.sess); !ok || look != c.want {
			t.Errorf("layout %d: the preview's look %#x, want %#x", c.layout, look, c.want)
		}
	}
	// another world's row only
	moved := map[string]database.DecorArt{"catapult": art["catapult"]}
	a := moved["catapult"]
	a.World = int(pet.World) + 1
	moved["catapult"] = a
	r.w.SetDecorLights(nil, moved, nil)
	if b := view(3, pet.ServerID); binary.LittleEndian.Uint32(b[28:]) != uint32(body) {
		t.Errorf("a world with no row, layout 3: model %08X, want the body", binary.LittleEndian.Uint32(b[28:]))
	}
	// at layout 4 the art is everywhere (its zone among the assets): the
	// owner, 2026-11-18, "[a player] construct is turning back into a spider"
	// (the Teleporter to Odus)
	if b := view(4, pet.ServerID); binary.LittleEndian.Uint32(b[28:]) != catapultArt {
		t.Errorf("a world with no row, layout 4: model %08X, want the catapult", binary.LittleEndian.Uint32(b[28:]))
	}
	if b := view(2, pet.ServerID); binary.LittleEndian.Uint32(b[28:]) != uint32(body) {
		t.Errorf("layout 2: model %08X, want the body", binary.LittleEndian.Uint32(b[28:]))
	}
	// with a PetLook rank too: the PS2's body is that rank's, the art stays
	r.w.mu.Lock()
	r.p.CMs = map[int]int{900917: 0, 900919: 0}
	if look, art := r.w.cmPetLookLocked(r.p, 717), r.w.cmPetArtLocked(r.p, 717); look != jarekLook || art != catapultArt {
		t.Errorf("both ranks: look %#x art %#x", look, art)
	}
	if art := r.w.cmPetArtLocked(r.p, 999); art != 0 {
		t.Errorf("a pet of a spell the rank does not require: art %#x", art)
	}
	r.p.CMs = nil
	r.w.mu.Unlock()
	r.p.Power = r.p.MaxPower
	back, _ := r.summon(t, fireElemental(), 120_000_000_000)
	if back.Art != 0 {
		t.Errorf("the rank gone: art %#x", back.Art)
	}
}

// The pets' looks follow the walk-ons: a u16 count, each model once (two
// worlds' rows of one art one entry), u8 kind 1, u32 shot, f32 its size, f32
// the art's; a
// decoration's art is not among them.
func TestAppendPetArts(t *testing.T) {
	b := AppendPetArts([]byte{9}, []database.DecorArt{
		{Model: 0x0E318233, Zone: 32},
		{Model: catapultArt, World: 0, Zone: 108, Kind: database.DecorArtPet, Shot: 0xBFEE4CCD, ShotScale: 0.25, Scale: 0.35},
		{Model: catapultArt, World: 1, Zone: 13, Kind: database.DecorArtPet, Shot: 0xBFEE4CCD, ShotScale: 0.25},
	})
	if len(b) != 1+2+17 || math.Float32frombits(binary.LittleEndian.Uint32(b[16:])) != 0.35 || b[0] != 9 || binary.LittleEndian.Uint16(b[1:]) != 1 || binary.LittleEndian.Uint32(b[3:]) != catapultArt || b[7] != 1 ||
		binary.LittleEndian.Uint32(b[8:]) != 0xBFEE4CCD || math.Float32frombits(binary.LittleEndian.Uint32(b[12:])) != 0.25 {
		t.Errorf("% X", b)
	}
}

// The pets' looks' assets: each art once, its first row's zone by world then
// zone, its model and its shot; a decoration's art none.
func TestPetArtAssets(t *testing.T) {
	got := PetArtAssets([]database.DecorArt{
		{Name: "tree", Model: 0x0E318233, Zone: 32},
		{Name: "rathe", Model: catapultArt, World: 1, Zone: 13, Kind: database.DecorArtPet, Shot: 0xBFEE4CCD},
		{Name: "tunaria", Model: catapultArt, World: 0, Zone: 108, Kind: database.DecorArtPet, Shot: 0xBFEE4CCD},
		{Name: "plain", Model: 0x1234, World: 2, Zone: 5, Kind: database.DecorArtPet},
	})
	if len(got) != 2 || got[0].World != 0 || got[0].Zone != 108 || len(got[0].IDs) != 2 || got[0].IDs[0] != catapultArt || got[0].IDs[1] != 0xBFEE4CCD ||
		got[1].Zone != 5 || len(got[1].IDs) != 1 {
		t.Errorf("%+v", got)
	}
}

// The pets' looks' assets are sent with the decorations though no string
// draws them (EncodeDecorLights sends only the assets something uses).
func TestPetArtAssetsAreSent(t *testing.T) {
	art := []database.DecorArt{{Name: "tunaria", Model: catapultArt, World: 0, Zone: 108, Kind: database.DecorArtPet, Shot: 0xBFEE4CCD}}
	with, _, _ := EncodeDecorLights(nil, art, PetArtAssets(art)...)
	without, _, _ := EncodeDecorLights(nil, nil, PetArtAssets(art)...)
	if len(with) != len(without)+11+(1+2+1+8) {
		t.Errorf("with the pet art %d bytes, without %d: want the art (11) and its asset (12) more", len(with), len(without))
	}
	if !bytes.Contains(with, []byte{0, 108, 0, 2, 0x78, 0x8D, 0x4B, 0xE6, 0xCD, 0x4C, 0xEE, 0xBF}) {
		t.Errorf("no asset of zone 108 with the catapult and the boulder: % X", with)
	}
}
