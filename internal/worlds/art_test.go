package worlds

import (
	"bytes"
	"encoding/binary"
	"os"
	"strings"
	"testing"
)

// objf is an OBJF file of the records, its header as VIObjFile writes one
// (a file type of 0xAB4F, Sandstorm's ss-art.csf's).
func objf(records ...[]byte) []byte {
	b := []byte("FJBO")
	b = binary.LittleEndian.AppendUint32(b, uint32(len(records)))
	b = binary.LittleEndian.AppendUint32(b, 0xAB4F)
	b = binary.LittleEndian.AppendUint32(b, 0)
	b = binary.LittleEndian.AppendUint64(b, 32)
	b = binary.LittleEndian.AppendUint64(b, ^uint64(0))
	for _, r := range records {
		b = append(b, r...)
	}
	return b
}

func TestAddArt(t *testing.T) {
	src := extrasFakes()
	baseRec := surfaceRec(0x7C000001, 0x11)
	base, err := CompressCSF(objf(baseRec))
	if err != nil {
		t.Fatal(err)
	}
	r := ArtRecipe{Pieces: []ArtPiece{
		{Sprite: idRock, From: "B.ESF", Zone: 1},
		{Sprite: idGroup, From: "b.esf", Zone: 1},
		{Sprite: 0x7C000001, From: "B.ESF", Zone: 1}, // the base's own: nothing brought
	}}
	out, built, err := AddArt(src, base, r, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(out[:4]) != "CESF" || binary.LittleEndian.Uint64(out[24:]) != 40 {
		t.Fatalf("not a CSF: % x", out[:40])
	}
	in, err := inflateCSF(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(in) != built.Inflated || binary.LittleEndian.Uint64(out[16:]) != uint64(len(in)) {
		t.Errorf("inflated %d, built says %d, the header %d", len(in), built.Inflated, binary.LittleEndian.Uint64(out[16:]))
	}
	if got := binary.LittleEndian.Uint32(in[4:]); got != 4 || built.Objects != 4 || built.BaseObjects != 1 {
		t.Errorf("%d records (built %d of base %d), want 4 of 1", got, built.Objects, built.BaseObjects)
	}
	// the base's records first, as they were
	if !bytes.Equal(in[32:32+len(baseRec)], baseRec) {
		t.Error("the base's record changed")
	}
	// each record after those it names: the rock's surface, the rock, then the group
	want := [][]DictID{{idRockSrf, idRock}, {idGroup}}
	if len(built.Brought) != 3 || !equalIDs(built.Brought[0], want[0]) || !equalIDs(built.Brought[1], want[1]) || len(built.Brought[2]) != 0 {
		t.Errorf("brought %x, want %x", built.Brought, want)
	}
	ids, err := ArtIDs(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []DictID{0x7C000001, idRockSrf, idRock, idRockPal, idGroup} {
		found := false
		for _, g := range ids {
			found = found || g == id
		}
		if !found {
			t.Errorf("%#08x is not registered by the file (%x)", uint32(id), ids)
		}
	}
	a, _ := openArt(src, ArtSource{From: "B.ESF", Zone: 1})
	roots, err := parseForest(in, 32)
	if err != nil || len(roots) != 4 {
		t.Fatalf("%d records: %v", len(roots), err)
	}
	if !bytes.Equal(recordBytes(roots[2]), recordBytes(a.dict[idRock])) || !bytes.Equal(recordBytes(roots[1]), recordBytes(a.dict[idRockSrf])) {
		t.Error("a copied record is not the disc's bytes")
	}

	// an OBJF base is taken as well, and the same pieces again bring nothing new
	again, b2, err := AddArt(src, in, r, nil)
	if err != nil {
		t.Fatal(err)
	}
	if b2.Objects != 4 || len(b2.Brought[0]) != 0 || len(b2.Brought[1]) != 0 || len(b2.Brought[2]) != 0 {
		t.Errorf("the built file again: %d records, brought %x", b2.Objects, b2.Brought)
	}
	if in2, _ := inflateCSF(again); !bytes.Equal(in2, in) {
		t.Error("adding what is there changed the file")
	}
}

func equalIDs(a, b []DictID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestAddArtRefuses(t *testing.T) {
	src := extrasFakes()
	base := objf(surfaceRec(0x7C000001, 0x11))
	for _, c := range []struct {
		name string
		p    ArtPiece
		want string
	}{
		{"pre-translated", ArtPiece{Sprite: idTerrain, From: "B.ESF", Zone: 1}, "pre-translations"},
		{"not there", ArtPiece{Sprite: 0x7DDDDDDD, From: "B.ESF", Zone: 1}, "does not register"},
		{"no file", ArtPiece{Sprite: idRock}, "from"},
		{"no zone", ArtPiece{Sprite: idRock, From: "B.ESF", Zone: 7}, "zones"},
	} {
		if _, _, err := AddArt(src, base, ArtRecipe{Pieces: []ArtPiece{c.p}}, nil); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", c.name, err, c.want)
		}
	}
	// a base whose header miscounts its records
	bad := objf(surfaceRec(0x7C000001, 0x11))
	binary.LittleEndian.PutUint32(bad[4:], 3)
	if _, _, err := AddArt(src, bad, ArtRecipe{}, nil); err == nil {
		t.Error("a miscounted base was taken")
	}

	// a file the game registers first: the same bytes are fine, other bytes refused
	same := memSource{"R.ESF": objf(surfaceRec(idRockSrf, 0xBB))}
	other := memSource{"R.ESF": objf(surfaceRec(idRockSrf, 0xCC))}
	p := ArtRecipe{Pieces: []ArtPiece{{Sprite: idRock, From: "B.ESF", Zone: 1}}}
	for _, c := range []struct {
		src memSource
		ok  bool
	}{{same, true}, {other, false}} {
		res, err := OpenResident(c.src, "R.ESF")
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = AddArt(src, base, p, []Resident{res})
		if (err == nil) != c.ok {
			t.Errorf("resident (ok %v): %v", c.ok, err)
		}
	}
}

// CompressCSF splits at 256 KB as VICSFFile::Compress does, and its header
// holds the sizes and the largest segment.
func TestCompressCSFSegments(t *testing.T) {
	data := bytes.Repeat([]byte("eqoa"), (1<<18)/4*2+100)
	c, err := CompressCSF(data)
	if err != nil {
		t.Fatal(err)
	}
	if n := binary.LittleEndian.Uint32(c[4:]); n != 3 {
		t.Errorf("%d segments, want 3", n)
	}
	back, err := inflateCSF(c)
	if err != nil || !bytes.Equal(back, data) {
		t.Fatalf("round trip: %v", err)
	}
	total, most, pos := 0, 0, 40
	for i := 0; i < 3; i++ {
		n := int(binary.LittleEndian.Uint32(c[pos:]))
		total, most = total+n, max(most, n)
		pos += 8 + n
	}
	if pos != len(c) || binary.LittleEndian.Uint64(c[8:]) != uint64(total) || binary.LittleEndian.Uint32(c[32:]) != uint32(most) ||
		binary.LittleEndian.Uint64(c[16:]) != uint64(len(data)) {
		t.Errorf("header % x: total %d most %d end %d of %d", c[:40], total, most, pos, len(c))
	}
}

// The papered board off the real disc (EQOA_ISO): Highpass's zone holds it
// whole, its surfaces inside its own palette, so it alone is brought, and
// none of the files the game reads first holds any of its DictIDs as other
// bytes.
func TestAddArtBoardOffTheDisc(t *testing.T) {
	iso := os.Getenv("EQOA_ISO")
	if iso == "" {
		t.Skip("EQOA_ISO not set")
	}
	d, err := OpenDisc(iso)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var res []Resident
	for _, n := range []string{"CHAR.ESF", "ITEM.ESF", "ARIAL.CSF", "UI.CSF", "ARIALUI.CSF", "ITEMICON.ESF", "SPELLFX.CSF", "FX.CSF", "ADDART.CSF"} {
		r, err := OpenResident(d, n)
		if err != nil {
			t.Fatal(err)
		}
		res = append(res, r)
	}
	base := objf(surfaceRec(0x7C000001, 0x11))
	out, built, err := AddArt(d, base, ArtRecipe{Pieces: []ArtPiece{{Sprite: 0x260EB42F, From: "TUNARIA.ESF", Zone: 80}}}, res)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d bytes, %d inflated, brought %x", len(out), built.Inflated, built.Brought)
	if len(built.Brought[0]) != 1 || built.Brought[0][0] != 0x260EB42F || built.Inflated != len(base)+12+56135 {
		t.Errorf("brought %x, %d inflated", built.Brought, built.Inflated)
	}
}
