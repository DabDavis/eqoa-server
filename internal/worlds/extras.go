package worlds

// Extras: art of the disc placed in a composed zone as actors of its own (a
// recipe zone's "extras"). Each is a ZoneActor (0x6000) like the zone's own:
// a sprite or model by DictID, where it stands, its turn, its size and its
// static light. It is filed in the actor list (0x3270) of the room whose box
// holds it, after the room's own actors, so every slot the zone's baked
// lighting names stays where it was; so a client draws and collides it as it
// does the zone's own (vanilla's vi::world and scene), and cmd/groundbake
// bakes its collision into the server's walls (what the movement check, line
// of sight and creatures' walls read).
//
// Where its art comes from. When the zone registers the sprite's DictID
// itself (its own trees and rocks), nothing more is needed. Otherwise the
// extra names where the art is on the disc (From, and Zone when From is a
// world file), and the composer COPIES the records it needs into the zone:
// the sprite's record whole, and every record it names by DictID that the
// zone does not register already (a palette, the surfaces its materials
// name, a group's members, an LOD's levels, an HSprite's attachments, an
// effect volume's surface and sprite, a particle's texture), each before the
// records that name it (a PS2's ParseMaterial bails on a texture not yet
// registered: claude-notes/dictid-cross-file-resolution.md), appended to the
// zone's resources (0x3100) with a ResourceTable entry each, in its DictID
// order.
//
// Copying, not a reference to art some other file holds, because a copied
// zone loads the same wherever it is read: a DictID resolves only against
// what is resident, and what is resident differs between the game, the
// page, a viewer and the server, whose ground is baked from the world file
// alone (a sprite another file holds would have no collision there at all);
// a PS2's parse fails outright on a reference it cannot resolve. A copied
// record is the disc's own bytes, under the disc's own DictID.
//
// What cannot be copied, refused: a terrain patch (a SimpleSubSprite, 0x2310,
// at the top of the resources is drawn and baked as the zone's terrain), art
// whose geometry stands on its zone's pre-translations (a PrimBuffer of kind 4,
// a CollBuffer packed with vertex groups: its vertices would be offset by the
// other zone's table), a character body (0x2700, whose skins this does not
// follow), and a DictID the world already registers with other bytes (the
// first registration wins, so one zone would draw another's art).
//
// Nothing is moved: the zone's records keep their order, the extras' records
// come after its own resources and actors, and every absolute offset its
// tables hold (ResourceTable 0x9000, ZoneStaticTable 0x32C0) is mapped to
// where the record it names now is. eqoa-rust-client-vanilla's
// serverworlds::extras is the other copy of this: a change here is one
// there, or no world with extras builds.

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Record types extras read or write.
const (
	typeZoneResources = 0x3100
	typeZoneActors    = 0x3290 // under a zone: the rooms' actor lists
	typeRoomActors    = 0x3270
	typeRoomID        = 0x3280
	typeActorList     = 0x6040
	typeActor         = 0x6000
	typeZoneRooms     = 0x3230
	typeZoneRoom      = 0x3240

	actorVersion = 2 // every retail ZoneActor's
	actorSize    = 36
)

// DictID is a dictionary id, written in a recipe as "0x8BB97B7B" (or a
// number).
type DictID uint32

// MarshalJSON writes it as hex.
func (d DictID) MarshalJSON() ([]byte, error) { return []byte(fmt.Sprintf(`"0x%08X"`, uint32(d))), nil }

// UnmarshalJSON reads "0x...", a decimal string, or a number.
func (d *DictID) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if strings.HasPrefix(s, `"`) {
		var str string
		if err := json.Unmarshal(b, &str); err != nil {
			return err
		}
		s = str
	}
	base := 10
	if h, ok := strings.CutPrefix(strings.ToLower(s), "0x"); ok {
		s, base = h, 16
	}
	v, err := strconv.ParseUint(s, base, 32)
	if err != nil {
		return fmt.Errorf("a DictID: %q", string(b))
	}
	*d = DictID(v)
	return nil
}

// Extra is a piece of the disc's art placed in a composed zone: an actor of
// its own (a ZoneActor).
type Extra struct {
	// Sprite is the DictID of what is placed: a sprite or a model (a
	// SimpleSprite, an LOD, a group, an HSprite, ...), as a ZoneActor names
	// one.
	Sprite DictID `json:"sprite"`
	// From is the disc file whose records the art is, when the zone does not
	// register Sprite itself (empty: the zone's own); Zone is the zone of it
	// whose records they are when From is a world file (unused otherwise).
	From string `json:"from,omitempty"`
	Zone int    `json:"zone,omitempty"`
	// At is where it stands, in the world (every actor of a zone is in world
	// space, as the zone is).
	At [3]float32 `json:"at"`
	// HPR is its turn in radians: heading (about the up axis), pitch and
	// roll, a ZoneActor's three.
	HPR [3]float32 `json:"hpr,omitempty"`
	// Scale is its size (0: 1).
	Scale float32 `json:"scale,omitempty"`
	// Color is its static light, RGBA: what is added to the sun on it, and
	// how much of the sun it takes (none given: 0, 0, 0, 255, nothing added
	// and all of the sun, as most of the disc's actors in the open).
	Color *[4]uint8 `json:"color,omitempty"`
	// Note is the maker's word for it. It does not change the file.
	Note string `json:"note,omitempty"`
}

// ActorScale is its scale as its ZoneActor has it: 1 when none is given.
func (e Extra) ActorScale() float32 {
	if e.Scale == 0 {
		return 1
	}
	return e.Scale
}

// ActorLight is its static light as its ZoneActor has it: none given is 0,
// 0, 0, 255.
func (e Extra) ActorLight() [4]uint8 {
	if e.Color == nil {
		return [4]uint8{0, 0, 0, 255}
	}
	return *e.Color
}

// ArtSource is where an extra's art is: a disc file, and the zone of it when
// it is a world file (-1 otherwise; From empty is the zone's own).
type ArtSource struct {
	From string
	Zone int
}

func (e Extra) source() ArtSource {
	if e.From == "" {
		return ArtSource{}
	}
	return ArtSource{From: strings.ToUpper(e.From), Zone: e.Zone}
}

func (e Extra) check() error {
	nums := []float32{e.At[0], e.At[1], e.At[2], e.HPR[0], e.HPR[1], e.HPR[2], e.Scale}
	for _, v := range nums {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return fmt.Errorf("sprite %#08x: a number that is not one", uint32(e.Sprite))
		}
	}
	switch {
	case e.Sprite == 0:
		return errors.New("no sprite")
	case e.Scale < 0:
		return fmt.Errorf("sprite %#08x: scale %v", uint32(e.Sprite), e.Scale)
	case e.From != "" && (e.Zone < 0 || e.Zone > 0xFFFE):
		return fmt.Errorf("sprite %#08x: zone %d of %s", uint32(e.Sprite), e.Zone, e.From)
	case len(e.From) > 255:
		return fmt.Errorf("sprite %#08x: a file name of %d bytes", uint32(e.Sprite), len(e.From))
	}
	return nil
}

// ExtrasFile is a file of extras a recipe takes in (Recipe.ExtraFiles): each
// zone's, named by the zone of the disc it is (a world file and an index).
type ExtrasFile struct {
	Note  string        `json:"note,omitempty"`
	Zones []ExtrasOfOne `json:"zones"`
}

// ExtrasOfOne is one zone's extras in an ExtrasFile.
type ExtrasOfOne struct {
	From   string  `json:"from"`
	Zone   int     `json:"zone"`
	Name   string  `json:"name,omitempty"` // the maker's, for reading
	Extras []Extra `json:"extras"`
}

// node is a record of a zone in the making: its type, version, children,
// its own bytes (a leaf's body; after a parent's children, what its body has
// past them, none in retail), and where it was in the source record (-1: one
// added) and is in the record written.
type node struct {
	ty      uint16
	version int16
	kids    []*node
	tail    []byte
	old     int64
	at      int64
	size    int64 // its body's, once laid out
}

// parseNodes reads the records of b from at to end into nodes (a forest),
// keeping each one's offset in b.
func parseNodes(b []byte, at, end int64) ([]*node, error) {
	var out []*node
	for at < end {
		n, next, err := parseNode(b, at, end)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
		at = next
	}
	return out, nil
}

func parseNode(b []byte, at, end int64) (*node, int64, error) {
	if at+12 > end {
		return nil, 0, fmt.Errorf("a record header at %d runs past %d", at, end)
	}
	ty := binary.LittleEndian.Uint16(b[at:])
	ver := int16(binary.LittleEndian.Uint16(b[at+2:]))
	size := int64(int32(binary.LittleEndian.Uint32(b[at+4:])))
	count := int64(int32(binary.LittleEndian.Uint32(b[at+8:])))
	body := at + 12
	if size < 0 || body+size > end || count < 0 {
		return nil, 0, fmt.Errorf("record %#x at %d: size %d, %d records", ty, at, size, count)
	}
	n := &node{ty: ty, version: ver, old: at}
	p := body
	for i := int64(0); i < count; i++ {
		k, next, err := parseNode(b, p, body+size)
		if err != nil {
			return nil, 0, err
		}
		n.kids = append(n.kids, k)
		p = next
	}
	n.tail = b[p : body+size]
	return n, body + size, nil
}

// layout sets each record's place (from at) and size; it returns the end.
func layout(n *node, at int64) int64 {
	n.at = at
	p := at + 12
	for _, k := range n.kids {
		p = layout(k, p)
	}
	p += int64(len(n.tail))
	n.size = p - at - 12
	return p
}

// write appends the records as laid out.
func write(out []byte, n *node) []byte {
	out = binary.LittleEndian.AppendUint16(out, n.ty)
	out = binary.LittleEndian.AppendUint16(out, uint16(n.version))
	out = binary.LittleEndian.AppendUint32(out, uint32(n.size))
	out = binary.LittleEndian.AppendUint32(out, uint32(len(n.kids)))
	for _, k := range n.kids {
		out = write(out, k)
	}
	return append(out, n.tail...)
}

// body is a record's bytes past its header as they stand (a leaf's).
func (n *node) child(ty uint16) *node {
	for _, k := range n.kids {
		if k.ty == ty {
			return k
		}
	}
	return nil
}

// Which records register a DictID, as the game reads them (the decomp's
// m_dict->Add sites; vanilla's esf crate): the id is the first word of the
// record's own body, or of its first child (a header), which registers the
// parent. The first registration wins.
var (
	idInBody   = map[uint16]bool{0x1000: true, 0x2D00: true, 0x2B00: true, 0x2E00: true, 0x2600: true, 0x5000: true, 0x1230: true}
	idInBodyV2 = map[uint16]bool{0x1200: true, 0x1210: true}
	idInHeader = map[uint16]bool{0x2000: true, 0x2200: true, 0x2700: true, 0x2A00: true, 0x2A40: true, 0x2C00: true, 0x2F00: true,
		0x2310: true, 0x2320: true, 0xB100: true, 0xC100: true, 0xC300: true, 0x1110: true, 0x7000: true, 0xB000: true, 0xB030: true,
		0xC200: true, 0xC000: true}
)

func word(b []byte, o int) uint32 {
	if o < 0 || o+4 > len(b) {
		return 0
	}
	return binary.LittleEndian.Uint32(b[o:])
}

// registers walks n and its records in file order, calling reg with each
// DictID registered and the record it names.
func registers(n *node, reg func(id uint32, r *node)) {
	if idInBody[n.ty] || idInBodyV2[n.ty] && n.version >= 2 {
		if n.leafBody() >= 4 {
			if id := word(n.firstBytes(), 0); id != 0 {
				reg(id, n)
			}
		}
	}
	for i, k := range n.kids {
		if i == 0 && idInHeader[n.ty] && !(idInBody[k.ty] || idInBodyV2[k.ty] && k.version >= 2) && k.leafBody() >= 4 {
			if id := word(k.firstBytes(), 0); id != 0 {
				reg(id, n)
			}
		}
		registers(k, reg)
	}
}

// leafBody is a record's body size as parsed (children and tail).
func (n *node) leafBody() int64 {
	s := int64(len(n.tail))
	for _, k := range n.kids {
		s += 12 + k.leafBody()
	}
	return s
}

// firstBytes is the start of a record's body as it stands: its first
// child's header when it has children (a body's first word, as the game
// reads it), else its own bytes.
func (n *node) firstBytes() []byte {
	if len(n.kids) == 0 {
		return n.tail
	}
	var h []byte
	k := n.kids[0]
	h = binary.LittleEndian.AppendUint16(h, k.ty)
	h = binary.LittleEndian.AppendUint16(h, uint16(k.version))
	return h
}

// refs are the DictIDs the records under n name (in file order, each once):
// the places a record names another by DictID rather than holding it.
func refs(n *node) []uint32 {
	var out []uint32
	seen := map[uint32]bool{}
	add := func(id uint32) {
		if id != 0 && id != 0xFFFFFFFF && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	var walk func(n *node)
	walk = func(n *node) {
		b := n.tail
		switch n.ty {
		case 0x1100: // a material: each layer's texture
			cnt := int(int32(word(b, 0)))
			p := 4
			if n.version > 1 {
				p += 4
			}
			if n.version > 2 {
				p += 4
			}
			for i := 0; i >= 0 && i < cnt && i < 64 && p+68 <= len(b); i++ {
				add(word(b, p+4))
				p += 68
			}
		case 0x2311, 0x2321: // a sub-sprite's header: its palette
			add(word(b, 4))
		case 0x2C30: // a group's members
			cnt := int(int32(word(b, 0)))
			for i := 0; i < cnt && 4+32*i+4 <= len(b); i++ {
				add(word(b, 4+32*i))
			}
		case 0x2A30, 0x2A70: // an LOD's levels
			cnt := int(int32(word(b, 0)))
			for i := 0; i < cnt && 4+8*i+4 <= len(b); i++ {
				add(word(b, 4+8*i))
			}
		case 0x2500: // an HSprite's attachments
			cnt := int(int32(word(b, 0)))
			for i := 0; i < cnt && 8+12*i+4 <= len(b); i++ {
				add(word(b, 8+12*i))
			}
		case 0xC330: // an effect volume's surface and sprite
			if len(b) >= 32 {
				add(word(b, 24))
				add(word(b, 28))
			}
		case 0xC020: // a particle definition's texture
			add(word(b, 0))
		}
		for _, k := range n.kids {
			walk(k)
		}
	}
	walk(n)
	return out
}

// art is the records an extra's art is taken from: a zone of a world file,
// or a whole file; its registrations.
type art struct {
	roots []*node
	bytes []byte
	dict  map[uint32]*node
}

func newArt(roots []*node, b []byte) *art {
	a := &art{roots: roots, bytes: b, dict: map[uint32]*node{}}
	for _, r := range roots {
		registers(r, func(id uint32, n *node) {
			if _, ok := a.dict[id]; !ok {
				a.dict[id] = n
			}
		})
	}
	return a
}

// openArt reads the records of an art source off the disc: zone src.Zone of
// a world file, or the whole of any other (a CSF inflated).
func openArt(src Source, s ArtSource) (*art, error) {
	rd, size, err := src.Open(s.From)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", s.From, err)
	}
	if w, err := openWorld(rd, size); err == nil {
		if s.Zone < 0 || s.Zone >= len(w.zones) {
			return nil, fmt.Errorf("%s has %d zones, not %d", s.From, len(w.zones), s.Zone+1)
		}
		rec, err := readAt(w.r, w.zones[s.Zone][0], int(w.zones[s.Zone][1]))
		if err != nil {
			return nil, err
		}
		roots, err := parseNodes(rec, 0, int64(len(rec)))
		if err != nil {
			return nil, fmt.Errorf("%s zone %d: %w", s.From, s.Zone, err)
		}
		return newArt(roots, rec), nil
	}
	b, err := readAt(rd, 0, int(size))
	if err != nil {
		return nil, err
	}
	if len(b) >= 4 && string(b[:4]) != "FJBO" {
		if b, err = inflateCSF(b); err != nil {
			return nil, fmt.Errorf("%s: %w", s.From, err)
		}
	}
	if len(b) < 24 || string(b[:4]) != "FJBO" {
		return nil, fmt.Errorf("%s: not an OBJF file", s.From)
	}
	start := int64(binary.LittleEndian.Uint64(b[16:]))
	if start < 0 || start > int64(len(b)) {
		return nil, fmt.Errorf("%s: its records at %d", s.From, start)
	}
	roots, err := parseForest(b, start)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", s.From, err)
	}
	return newArt(roots, b), nil
}

// parseForest reads a file's records from start to its end, stopping at a
// zero type at the top (as the game's VIObjFile does).
func parseForest(b []byte, start int64) ([]*node, error) {
	var out []*node
	at, end := start, int64(len(b))
	for at+12 <= end && binary.LittleEndian.Uint16(b[at:]) != 0 {
		n, next, err := parseNode(b, at, end)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
		at = next
	}
	return out, nil
}

// inflateCSF is a CSF's OBJF bytes: a 40-byte header (the block count at
// +4, the blocks' offset at +24), then each block an 8-byte length and a
// zlib stream.
func inflateCSF(b []byte) ([]byte, error) {
	if len(b) < 40 {
		return nil, errors.New("not a CSF")
	}
	blocks := binary.LittleEndian.Uint32(b[4:])
	pos := int64(binary.LittleEndian.Uint64(b[24:]))
	if pos < 40 || pos > int64(len(b)) || blocks == 0 || blocks > 1<<20 {
		return nil, errors.New("not a CSF")
	}
	var out bytes.Buffer
	for i := uint32(0); i < blocks; i++ {
		if pos+8 > int64(len(b)) {
			return nil, fmt.Errorf("CSF block %d: cut short", i)
		}
		n := int64(binary.LittleEndian.Uint64(b[pos:]) & 0xFFFFFFFF)
		pos += 8
		if pos+n > int64(len(b)) {
			return nil, fmt.Errorf("CSF block %d: past the end", i)
		}
		z, err := zlib.NewReader(bytes.NewReader(b[pos : pos+n]))
		if err != nil {
			return nil, fmt.Errorf("CSF block %d: %w", i, err)
		}
		if _, err := io.Copy(&out, z); err != nil {
			return nil, fmt.Errorf("CSF block %d: %w", i, err)
		}
		pos += n
	}
	return out.Bytes(), nil
}

// cloneNode copies a record and its children as new ones (old -1).
func cloneNode(n *node) *node {
	c := &node{ty: n.ty, version: n.version, tail: append([]byte(nil), n.tail...), old: -1}
	for _, k := range n.kids {
		c.kids = append(c.kids, cloneNode(k))
	}
	return c
}

// recordBytes is a record whole (header and body) as it stands.
func recordBytes(n *node) []byte {
	m := cloneNode(n)
	layout(m, 0)
	return write(nil, m)
}

// copied is a record an extra's art brought into the zone: the record, the
// DictID it registers (its ResourceTable entry's).
type copied struct {
	n  *node
	id uint32
}

// room is a zone room's box (none before version 2).
type room struct {
	box [6]float32
	has bool
}

// pickRoom is the room an extra at p is filed under: the first whose box
// holds it; else the first whose box holds it seen from above; else the one
// nearest from above (the first of those as near). Retail's files are the
// same: every actor of the homelands' zones is inside its room's box.
func pickRoom(rooms []room, p [3]float32) int {
	in := func(r room, y bool) bool {
		return r.has && r.box[0] <= p[0] && p[0] <= r.box[3] && r.box[2] <= p[2] && p[2] <= r.box[5] &&
			(!y || r.box[1] <= p[1] && p[1] <= r.box[4])
	}
	for i, r := range rooms {
		if in(r, true) {
			return i
		}
	}
	for i, r := range rooms {
		if in(r, false) {
			return i
		}
	}
	best, bestD := -1, math.Inf(1)
	for i, r := range rooms {
		if !r.has {
			continue
		}
		dx := float64(max(r.box[0]-p[0], 0, p[0]-r.box[3]))
		dz := float64(max(r.box[2]-p[2], 0, p[2]-r.box[5]))
		if d := float64(dx*dx) + float64(dz*dz); d < bestD {
			best, bestD = i, d
		}
	}
	return best
}

// ActorBits is an extra's number as the world file and the server-worlds
// list both carry it: -0 as 0. A client builds the file off the list, so the
// two must agree bit for bit; a -0 in the file and 0 on the wire made every
// client's Dungeons (world 14) fail its CRC (2026-11-14).
func ActorBits(v float32) uint32 {
	if v == 0 {
		v = 0
	}
	return math.Float32bits(v)
}

// actorNode is an extra's ZoneActor record.
func actorNode(e Extra) *node {
	b := binary.LittleEndian.AppendUint32(nil, uint32(e.Sprite))
	for _, v := range e.At {
		b = binary.LittleEndian.AppendUint32(b, ActorBits(v))
	}
	for _, v := range e.HPR {
		b = binary.LittleEndian.AppendUint32(b, ActorBits(v))
	}
	b = binary.LittleEndian.AppendUint32(b, ActorBits(e.ActorScale()))
	l := e.ActorLight()
	b = append(b, l[:]...)
	return &node{ty: typeActor, version: actorVersion, tail: b, old: -1}
}

// zoneWithExtras is a zone record written with its extras: the record,
// where its ZoneBase record is in it and that record's length, how many
// records its art brought, and the DictIDs those register with their bytes.
type zoneWithExtras struct {
	rec             []byte
	baseAt, baseLen int64
	copied          int
	brought         map[uint32][]byte
}

// zoneArt is what composing one zone's extras needs off the disc, read
// before the file is written: each art source's records.
type zoneArt map[ArtSource]*art

// withExtras is a zone record (rec, at srcAt in its source file) as written
// at outAt of the new file with its extras: the records the extras' art
// needs copied in, an actor each, and every absolute offset of its tables
// mapped; and where its ZoneBase record is in it.
func withExtras(rec []byte, srcAt, outAt int64, extras []Extra, sources zoneArt) (zoneWithExtras, error) {
	var z zoneWithExtras
	fail := func(err error) (zoneWithExtras, error) { return zoneWithExtras{}, err }
	roots, err := parseNodes(rec, 0, int64(len(rec)))
	if err != nil {
		return fail(err)
	}
	if len(roots) != 1 || roots[0].ty != typeZone {
		return fail(errors.New("not a zone record"))
	}
	zone := roots[0]
	res, acts, base := zone.child(typeZoneResources), zone.child(typeZoneActors), zone.child(typeZoneBase)
	if res == nil || acts == nil || base == nil {
		return fail(errors.New("a zone without resources, actors or a base"))
	}
	rt, st, rooms := base.child(typeResourceTable), base.child(typeStaticTable), base.child(typeZoneRooms)
	if rt == nil || st == nil || rooms == nil {
		return fail(errors.New("a zone base without its resource table, static table or rooms"))
	}
	byOld := map[int64]*node{}
	var index func(n *node)
	index = func(n *node) {
		byOld[n.old] = n
		for _, k := range n.kids {
			index(k)
		}
	}
	index(zone)

	// the zone's own registrations, then the copies': the record each id
	// names in the zone as written (the first registration wins)
	reg := map[uint32]*node{}
	registers(zone, func(id uint32, n *node) {
		if _, ok := reg[id]; !ok {
			reg[id] = n
		}
	})

	// the art copied in, each record after those it names
	var copies []copied
	var fresh []uint32 // the DictIDs the copies bring
	var need func(a *art, id uint32, by string) error
	need = func(a *art, id uint32, by string) error {
		if reg[id] != nil {
			return nil
		}
		r, ok := a.dict[id]
		if !ok {
			return fmt.Errorf("%s names %#08x, which its art does not register", by, id)
		}
		switch r.ty {
		case 0x2310, 0x2320:
			return fmt.Errorf("%#08x is a sub-sprite (%#x): one copied into a zone's resources is taken for its terrain", id, r.ty)
		case 0x2700:
			return fmt.Errorf("%#08x is a character body (0x2700): its skins are not followed", id)
		}
		if err := standsAlone(r); err != nil {
			return fmt.Errorf("%#08x: %w", id, err)
		}
		// what it registers is had from now on, but for what the zone has
		// already, which stays the zone's (the first registration wins)
		registers(r, func(d uint32, n *node) {
			if reg[d] == nil {
				reg[d] = n
				fresh = append(fresh, d)
			}
		})
		for _, ref := range refs(r) {
			if err := need(a, ref, fmt.Sprintf("%#08x", id)); err != nil {
				return err
			}
		}
		copies = append(copies, copied{n: cloneNode(r), id: id})
		return nil
	}
	for i, e := range extras {
		if reg[uint32(e.Sprite)] != nil {
			continue
		}
		s := e.source()
		if s.From == "" {
			return fail(fmt.Errorf("extra %d: the zone does not hold %#08x, and the extra names no art (from)", i, uint32(e.Sprite)))
		}
		a := sources[s]
		if a == nil {
			return fail(fmt.Errorf("extra %d: %s zone %d not read", i, s.From, s.Zone))
		}
		if err := need(a, uint32(e.Sprite), fmt.Sprintf("extra %d", i)); err != nil {
			return fail(fmt.Errorf("extra %d (%s zone %d): %w", i, s.From, s.Zone, err))
		}
	}
	for _, c := range copies {
		res.kids = append(res.kids, c.n)
	}

	// the actors, each in its room's list after the room's own
	var rs []room
	for _, r := range rooms.kids {
		if r.ty != typeZoneRoom {
			continue
		}
		var x room
		if r.version >= 2 && len(r.tail) >= 36 {
			for k := range x.box {
				x.box[k] = math.Float32frombits(word(r.tail, 12+4*k))
			}
			x.has = true
		}
		rs = append(rs, x)
	}
	lists := map[int32]*node{}
	for _, l := range acts.kids {
		if l.ty != typeRoomActors {
			continue
		}
		if id := l.child(typeRoomID); id != nil && len(id.tail) >= 4 {
			if l.child(typeActorList) != nil {
				lists[int32(word(id.tail, 0))] = l
			}
		}
	}
	added := map[int32]*node{} // rooms given a list of their own
	for i, e := range extras {
		ri := pickRoom(rs, e.At)
		if ri < 0 {
			return fail(fmt.Errorf("extra %d: the zone has no room to file it under", i))
		}
		l := lists[int32(ri)]
		if l == nil {
			l = &node{ty: typeRoomActors, old: -1, kids: []*node{
				{ty: typeRoomID, old: -1, tail: binary.LittleEndian.AppendUint32(nil, uint32(ri))},
				{ty: typeActorList, old: -1},
			}}
			lists[int32(ri)] = l
			added[int32(ri)] = l
			// in room order, as every retail zone's lists are
			at := len(acts.kids)
			for k, o := range acts.kids {
				if id := o.child(typeRoomID); o.ty == typeRoomActors && id != nil && int32(word(id.tail, 0)) > int32(ri) {
					at = k
					break
				}
			}
			acts.kids = append(acts.kids[:at], append([]*node{l}, acts.kids[at:]...)...)
		}
		al := l.child(typeActorList)
		al.kids = append(al.kids, actorNode(e))
	}

	// the resource table: its entries and the copies', in DictID order
	oldRT := rt.tail
	n := int(int32(word(oldRT, 0)))
	if n < 0 || 4+16*n > len(oldRT) {
		return fail(fmt.Errorf("resource table: %d entries in %d bytes", n, len(oldRT)))
	}
	type entry struct {
		n  *node
		id uint32
	}
	entries := make([]entry, 0, n+len(copies))
	for i := 0; i < n; i++ {
		v := int64(binary.LittleEndian.Uint64(oldRT[4+16*i:]))
		r := byOld[v-srcAt]
		if r == nil {
			return fail(fmt.Errorf("resource table entry %d names %d, no record of the zone", i, v))
		}
		entries = append(entries, entry{r, word(oldRT, 4+16*i+12)})
	}
	for _, c := range copies {
		entries = append(entries, entry{c.n, c.id})
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].id < entries[j].id })
	rt.tail = make([]byte, 4+16*len(entries))

	// lay out, then fill in the tables with where their records are
	end := layout(zone, 0)
	binary.LittleEndian.PutUint32(rt.tail, uint32(len(entries)))
	for i, e := range entries {
		o := 4 + 16*i
		binary.LittleEndian.PutUint64(rt.tail[o:], uint64(outAt+e.n.at))
		binary.LittleEndian.PutUint32(rt.tail[o+8:], uint32(12+e.n.size))
		binary.LittleEndian.PutUint32(rt.tail[o+12:], e.id)
	}
	st.tail = append([]byte(nil), st.tail...) // its bytes are the source record's
	m := int(int32(word(st.tail, 0)))
	if m < 0 || 4+24*m != len(st.tail) {
		return fail(fmt.Errorf("static table: %d rooms in %d bytes", m, len(st.tail)))
	}
	for i := 0; i < m; i++ {
		for _, o := range [2]int{4 + 24*i, 4 + 24*i + 12} {
			v := int64(binary.LittleEndian.Uint64(st.tail[o:]))
			if v < 0 {
				continue
			}
			r := byOld[v-srcAt]
			if r == nil {
				return fail(fmt.Errorf("static table room %d names %d, no record of the zone", i, v))
			}
			binary.LittleEndian.PutUint64(st.tail[o:], uint64(outAt+r.at))
			binary.LittleEndian.PutUint32(st.tail[o+8:], uint32(12+r.size))
		}
		if l := added[int32(i)]; l != nil {
			o := 4 + 24*i
			binary.LittleEndian.PutUint64(st.tail[o:], uint64(outAt+l.at))
			binary.LittleEndian.PutUint32(st.tail[o+8:], uint32(12+l.size))
		}
	}
	out := write(make([]byte, 0, end), zone)
	if int64(len(out)) != end {
		return fail(fmt.Errorf("laid out %d bytes, wrote %d", end, len(out)))
	}
	z.rec, z.baseAt, z.baseLen = out, base.at, 12+base.size
	z.brought = map[uint32][]byte{}
	for _, id := range fresh {
		z.brought[id] = recordBytes(reg[id])
	}
	z.copied = len(copies)
	return z, nil
}

// standsAlone refuses art whose geometry is placed by its zone's
// pre-translations: a PrimBuffer of kind 4 (its vertices' groups) or a
// CollBuffer packed with vertex groups (2, 3).
func standsAlone(n *node) error {
	switch n.ty {
	case 0x1200:
		if n.version >= 1 {
			o := 0
			if n.version > 1 {
				o = 4
			}
			if k := int32(word(n.tail, o)); k == 4 {
				return errors.New("its geometry stands on its zone's pre-translations (a PrimBuffer of kind 4)")
			}
		}
	case 0x4200:
		if n.version >= 2 {
			if p := int32(word(n.tail, 0)); p == 2 || p == 3 {
				return errors.New("its collision stands on its zone's pre-translations (a CollBuffer of vertex groups)")
			}
		}
	}
	for _, k := range n.kids {
		if err := standsAlone(k); err != nil {
			return err
		}
	}
	return nil
}

// clashes refuses a world in which the art one zone's extras brought
// registers a DictID another zone registers as other bytes: the first
// registration wins, in a client's dictionary (zones ranked in file order)
// and in the server's bake (the file's first), so one of the two would draw
// and collide as the other's.
func clashes(parts []part, brought []map[uint32][]byte) error {
	some := false
	for _, b := range brought {
		some = some || len(b) > 0
	}
	if !some {
		return nil
	}
	for j, p := range parts {
		roots, err := parseNodes(p.rec, 0, int64(len(p.rec)))
		if err != nil {
			return err
		}
		theirs := map[uint32]*node{}
		for _, r := range roots {
			registers(r, func(id uint32, n *node) {
				if _, ok := theirs[id]; !ok {
					theirs[id] = n
				}
			})
		}
		for i, b := range brought {
			if i == j {
				continue
			}
			for id, rec := range b {
				if n := theirs[id]; n != nil && !bytes.Equal(recordBytes(n), rec) {
					return fmt.Errorf("zone %d's extras bring %#08x, which zone %d holds as other bytes", i, id, j)
				}
			}
		}
	}
	for i := range brought {
		for j := i + 1; j < len(brought); j++ {
			for id, rec := range brought[i] {
				if o, ok := brought[j][id]; ok && !bytes.Equal(o, rec) {
					return fmt.Errorf("zones %d's and %d's extras bring %#08x as other bytes", i, j, id)
				}
			}
		}
	}
	return nil
}

// ArtBox is the box an extra's sprite has in its own space (its header's,
// as the game reads it: a sprite's, an LOD's, a group's or an HSprite's),
// read off the disc where the extra says its art is; its size times the
// extra's scale is what it covers placed. A tool's (cmd/worldedges), for
// laying extras out.
func ArtBox(src Source, e Extra) ([6]float32, error) {
	var box [6]float32
	s := e.source()
	if s.From == "" {
		return box, errors.New("the extra names no art (from)")
	}
	a, err := openArt(src, s)
	if err != nil {
		return box, err
	}
	r := a.dict[uint32(e.Sprite)]
	if r == nil {
		return box, fmt.Errorf("%s zone %d does not register %#08x", s.From, s.Zone, uint32(e.Sprite))
	}
	h := map[uint16]uint16{0x2000: 0x2001, 0x2A00: 0x2A10, 0x2C00: 0x2C10, 0x2200: 0x2210, 0x2F00: 0x2F01}[r.ty]
	hn := r.child(h)
	if h == 0 || hn == nil || len(hn.tail) < 28 {
		return box, fmt.Errorf("%#08x (%#x) has no box", uint32(e.Sprite), r.ty)
	}
	for k := range box {
		box[k] = math.Float32frombits(word(hn.tail, 4+4*k))
	}
	return box, nil
}
