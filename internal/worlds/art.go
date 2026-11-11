package worlds

// Patch art: the disc's art added to the art file the patch server sends
// (patches/serve/ss-art.csf), so a sprite only some zones hold is had
// everywhere. CLIENT reads its added art off the memory card at world entry
// (retail addart.csf through a 16 KB buffer; Sandstorm's S2 reads ss-art.csf
// through a bigger one, our served S2's 256 KB, Sandstorm's own live S2's 80
// KB: claude-notes/esf-asset-injection.md) and parses it into the live scene,
// re-parsing it at every scene's SetSky; vanilla adds the server folder's copy
// to its library (crates/eqoa/src/main.rs, s2::Rules::art_path). So a
// creature whose body is such a sprite (an NPC's modelid) draws in every
// world, where before it drew only where a zone holding it was loaded (a PS2
// shows "!Character" elsewhere, vanilla nothing).
//
// AddArt copies a piece's records out of the disc as extras do (extras.go's
// need: the sprite's record whole and, before it, every record it names by
// DictID that the file does not register already), appended to the file's
// top-level records after its own, so the base file's records, Sandstorm's
// robes and set 9 among them, keep their bytes and their order. Nothing of
// the disc is in the repository: tools/art/*.json names the pieces, and
// cmd/artbuild reads them off the operator's disc.

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
)

// ArtRecipe is what goes into a patch art file (tools/art/*.json).
type ArtRecipe struct {
	Note   string     `json:"note,omitempty"`
	Pieces []ArtPiece `json:"pieces"`
}

// ArtPiece is a sprite of the disc added to it: its DictID and where its
// records are (a disc file, and the zone of it when it is a world file).
type ArtPiece struct {
	Sprite DictID `json:"sprite"`
	From   string `json:"from"`
	Zone   int    `json:"zone,omitempty"`
	Note   string `json:"note,omitempty"`
}

// ArtBuilt is an art file as built.
type ArtBuilt struct {
	// Size and CRC are the written CSF's; Inflated its records' (OBJF)
	// size.
	Size     int    `json:"size"`
	CRC      uint32 `json:"crc"`
	Inflated int    `json:"inflated"`
	// BaseObjects is how many top-level records the base had, Objects how
	// many the file has.
	BaseObjects int `json:"baseObjects"`
	Objects     int `json:"objects"`
	// Brought are the records each piece brought (its DictIDs, the records
	// named first), in the recipe's order.
	Brought [][]DictID `json:"brought"`
}

// Resident is a file the game has registered before it reads the patch
// art, for AddArt's clash check: a DictID one registers as other bytes
// would be drawn as that file's (the first registration wins).
type Resident struct {
	Name string
	Art  *art
}

// OpenResident reads a whole disc file's registrations (a CSF inflated).
func OpenResident(src Source, name string) (Resident, error) {
	a, err := openArt(src, ArtSource{From: name, Zone: -1})
	if err != nil {
		return Resident{}, err
	}
	return Resident{Name: name, Art: a}, nil
}

// MaxPatchArt is the most a PS2 reads of ss-art.csf with our served S2: its
// staging buffer, malloc(0x40000) (a file as big is cut short).
const MaxPatchArt = 0x40000

// AddArt is base (a CSF, or an OBJF) with the recipe's pieces added, as a
// CSF written as VICSFFile::Compress writes one.
func AddArt(src Source, base []byte, r ArtRecipe, resident []Resident) ([]byte, ArtBuilt, error) {
	var built ArtBuilt
	fail := func(err error) ([]byte, ArtBuilt, error) { return nil, ArtBuilt{}, err }
	objf := base
	if len(base) >= 4 && string(base[:4]) != "FJBO" {
		var err error
		if objf, err = inflateCSF(base); err != nil {
			return fail(fmt.Errorf("the base: %w", err))
		}
	}
	if len(objf) < 32 || string(objf[:4]) != "FJBO" {
		return fail(errors.New("the base is not an OBJF file"))
	}
	start := int64(binary.LittleEndian.Uint64(objf[16:]))
	if start < 32 || start > int64(len(objf)) {
		return fail(fmt.Errorf("the base's records at %d", start))
	}
	roots, err := parseForest(objf, start)
	if err != nil {
		return fail(fmt.Errorf("the base: %w", err))
	}
	if n := int(binary.LittleEndian.Uint32(objf[4:])); n != len(roots) {
		return fail(fmt.Errorf("the base says %d records and holds %d", n, len(roots)))
	}
	built.BaseObjects = len(roots)

	reg := map[uint32]*node{}
	for _, n := range roots {
		registers(n, func(id uint32, r *node) {
			if reg[id] == nil {
				reg[id] = r
			}
		})
	}
	sources := map[ArtSource]*art{}
	var added []*node
	for i, p := range r.Pieces {
		if p.Sprite == 0 || p.From == "" {
			return fail(fmt.Errorf("piece %d: a sprite and the file it is in (from)", i))
		}
		s := Extra{Sprite: p.Sprite, From: p.From, Zone: p.Zone}.source()
		a := sources[s]
		if a == nil {
			if a, err = openArt(src, s); err != nil {
				return fail(fmt.Errorf("piece %d: %w", i, err))
			}
			sources[s] = a
		}
		var brought []DictID
		var need func(id uint32, by string) error
		need = func(id uint32, by string) error {
			if reg[id] != nil {
				return nil
			}
			n, ok := a.dict[id]
			if !ok {
				return fmt.Errorf("%s names %#08x, which %s zone %d does not register", by, id, s.From, s.Zone)
			}
			if n.ty == 0x2700 {
				return fmt.Errorf("%#08x is a character body (0x2700): its skins are not followed", id)
			}
			if err := standsAlone(n); err != nil {
				return fmt.Errorf("%#08x: %w", id, err)
			}
			registers(n, func(d uint32, m *node) {
				if reg[d] == nil {
					reg[d] = m
				}
			})
			for _, ref := range refs(n) {
				if err := need(ref, fmt.Sprintf("%#08x", id)); err != nil {
					return err
				}
			}
			added = append(added, cloneNode(n))
			brought = append(brought, DictID(id))
			return nil
		}
		if err := need(uint32(p.Sprite), fmt.Sprintf("piece %d", i)); err != nil {
			return fail(fmt.Errorf("piece %d: %w", i, err))
		}
		built.Brought = append(built.Brought, brought)
	}

	// a DictID a resident file registers as other bytes would be drawn as
	// the resident's
	for _, n := range added {
		var clash error
		registers(n, func(id uint32, m *node) {
			for _, res := range resident {
				if o := res.Art.dict[id]; o != nil && clash == nil && !bytes.Equal(recordBytes(o), recordBytes(m)) {
					clash = fmt.Errorf("%#08x: %s registers it first, as other bytes", id, res.Name)
				}
			}
		})
		if clash != nil {
			return fail(clash)
		}
	}

	// the file: the base's header and records as they are, the added ones
	// after them
	out := append([]byte(nil), objf[:start]...)
	end := start
	for _, n := range roots {
		end = n.old + 12 + int64(binary.LittleEndian.Uint32(objf[n.old+4:]))
	}
	if end != int64(len(objf)) {
		return fail(fmt.Errorf("the base has %d bytes past its records", int64(len(objf))-end))
	}
	out = append(out, objf[start:end]...)
	for _, n := range added {
		layout(n, 0)
		out = write(out, n)
	}
	binary.LittleEndian.PutUint32(out[4:], uint32(len(roots)+len(added)))
	built.Objects = len(roots) + len(added)
	built.Inflated = len(out)

	csf, err := CompressCSF(out)
	if err != nil {
		return fail(err)
	}
	built.Size, built.CRC = len(csf), crc32.ChecksumIEEE(csf)
	return csf, built, nil
}

// CompressCSF writes an OBJF file as a CSF as VICSFFile::Compress does: a
// 40-byte header (the id "CESF", the segment count, the compressed and
// whole sizes, the segments' offset 40, the largest compressed segment),
// then each 256 KB segment an 8-byte record (its compressed size, its size)
// and its zlib stream at level 9.
func CompressCSF(objf []byte) ([]byte, error) {
	const segm = 1 << 18
	var segs [][]byte
	var sizes []int
	total, most := 0, 0
	for p := 0; p < len(objf) || p == 0; p += segm {
		n := min(segm, len(objf)-p)
		var z bytes.Buffer
		w, err := zlib.NewWriterLevel(&z, 9)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(objf[p : p+n]); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
		segs, sizes = append(segs, z.Bytes()), append(sizes, n)
		total += z.Len()
		most = max(most, z.Len())
		if len(objf) == 0 {
			break
		}
	}
	h := make([]byte, 40)
	copy(h, "CESF")
	binary.LittleEndian.PutUint32(h[4:], uint32(len(segs)))
	binary.LittleEndian.PutUint64(h[8:], uint64(total))
	binary.LittleEndian.PutUint64(h[16:], uint64(len(objf)))
	binary.LittleEndian.PutUint64(h[24:], 40)
	binary.LittleEndian.PutUint32(h[32:], uint32(most))
	out := h
	for i, s := range segs {
		out = binary.LittleEndian.AppendUint32(out, uint32(len(s)))
		out = binary.LittleEndian.AppendUint32(out, uint32(sizes[i]))
		out = append(out, s...)
	}
	return out, nil
}

// ArtIDs are the DictIDs a CSF or OBJF file registers, in order (a check's
// and a tool's).
func ArtIDs(b []byte) ([]DictID, error) {
	if len(b) >= 4 && string(b[:4]) != "FJBO" {
		var err error
		if b, err = inflateCSF(b); err != nil {
			return nil, err
		}
	}
	if len(b) < 32 || string(b[:4]) != "FJBO" {
		return nil, errors.New("not an OBJF file")
	}
	roots, err := parseForest(b, int64(binary.LittleEndian.Uint64(b[16:])))
	if err != nil {
		return nil, err
	}
	var ids []DictID
	seen := map[uint32]bool{}
	for _, n := range roots {
		registers(n, func(id uint32, _ *node) {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, DictID(id))
			}
		})
	}
	return ids, nil
}
