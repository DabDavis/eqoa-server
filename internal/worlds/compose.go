package worlds

// A world composed of the disc's own zones (cmd/worldbuild): each zone copied
// whole out of the world file it is on, where it stands there, into a world of
// its own. The recipe names the zones (a file of the disc and a zone index);
// nothing of the disc is in it, so it is kept in this repository
// (tools/worlds/), and every server builds its world from its own disc, as
// cmd/groundbake builds its ground. A client that can (eqoa-rust-client-
// vanilla) is sent the recipe (handlers/serverworlds.go) and builds the same
// file from its player's disc: the built file's CRC, and each zone's, are how
// both ends know they hold the same world. A PS2 has no such builder; a
// patched one could be served the file over the hard-disk stager's /worlds/.
//
// The file is the shape every retail world has (TUNARIA.ESF's records and
// versions, measured):
//
//	OBJF header    "FJBO", 1, 43855, 0, 32 (the tree), -1
//	Root 0x8000    v0, 2 records
//	  World 0x8100 v0, the zones: each 0x3000 record verbatim, its absolute
//	               offsets moved with it (ResourceTable 0x9000, ZoneStaticTable
//	               0x32C0: claude-notes/authoring-a-zone.md)
//	  WorldBase    0x8200 v2, 4 records:
//	    0x8220 v1  the header: zone count, box, rows, columns, 4, 2000, origin
//	    0x8210 v2  the zones' proxies, each the source's with its offsets moved
//	    0x8230 v0  the world's BSP over the zones' cells
//	    0x8240 v0  a terrain profile a sub-cell: each zone's from its own world
//
// Zones keep their coordinates (every vertex, actor and pre-translation in a
// zone is in world space: moving one would rewrite them all), so no two of a
// recipe's zones may share a cell of the 2000-unit grid. The +8 word of the
// header is 43855 in every retail file, worlds and the rest: kept.

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Record types this file writes or reads.
const (
	typeRoot          = 0x8000
	typeWorld         = 0x8100
	typeWorldBase     = 0x8200
	typeZoneProxies   = 0x8210
	typeBaseHeader    = 0x8220
	typeWorldTree     = 0x8230
	typeWorldRegions  = 0x8240
	typeZone          = 0x3000
	typeZoneBase      = 0x3200
	typeResourceTable = 0x9000
	typeStaticTable   = 0x32C0

	proxySize = 56
	leafBit   = 0x40000000

	// every retail world's grid
	cellSize = 2000
	subdiv   = 4
	originXZ = 1000

	// the OBJF header's +8 word, the same in every file of the disc
	retailHeaderWord = 43855
)

// Recipe is a composed world as its maker writes it (tools/worlds/*.json).
type Recipe struct {
	// World is the slot it is (worldN.ESF; handlers' worldPaths 8 up).
	World int `json:"world"`
	// Name is what players are told it is.
	Name  string       `json:"name"`
	Zones []RecipeZone `json:"zones"`
	// ExtraFiles are files of extras (ExtrasFile; paths beside the recipe)
	// whose zones' extras ReadRecipe adds after each zone's own: a long
	// list a tool writes kept out of the recipe a person edits.
	ExtraFiles []string `json:"extraFiles,omitempty"`
}

// RecipeZone is one zone of a recipe: zone Zone of the disc's file From.
type RecipeZone struct {
	// Name is the zone's name in the world (a client that can shows it where
	// the game names its zones).
	Name string `json:"name"`
	From string `json:"from"`
	Zone int    `json:"zone"`
	// Race is the racedefs race whose home it is (0: none); Note is the
	// maker's word for it. Neither changes the file.
	Race int    `json:"race,omitempty"`
	Note string `json:"note,omitempty"`
	// Extras are the disc's art placed in the zone as actors of its own
	// (extras.go): each a sprite by DictID, where it stands, its turn and
	// size. A zone with none is copied as it is on the disc.
	Extras []Extra `json:"extras,omitempty"`
}

// Built is a recipe as built: what cmd/worldbuild writes beside the world
// (worldN.json), and what the server tells a client to build.
type Built struct {
	Recipe
	// Size and CRC are the built file's (CRC-32, IEEE).
	Size int64  `json:"size"`
	CRC  uint32 `json:"crc"`
	// Rows and Cols are its grid's.
	Rows int `json:"rows"`
	Cols int `json:"cols"`
	// Parts are its zones as built, in the recipe's order.
	Parts []BuiltZone `json:"parts"`
}

// BuiltZone is one zone as built: its record in the source file (its size,
// header and body, and their CRC), the cell it stands in, and its bounds.
type BuiltZone struct {
	From  string     `json:"from"`
	Zone  int        `json:"zone"`
	Size  int        `json:"size"`
	CRC   uint32     `json:"crc"`
	Col   int        `json:"col"`
	Row   int        `json:"row"`
	Box   [6]float32 `json:"box"`
	Start [3]float32 `json:"centre"`
	// Extras is how many extras it was given, Brought how many records of
	// the disc their art brought into it (extras.go).
	Extras  int `json:"extras,omitempty"`
	Brought int `json:"brought,omitempty"`
}

// Source reads the disc's files: a file's bytes at an offset.
type Source interface {
	// Open is the file name's reader and its size.
	Open(name string) (io.ReaderAt, int64, error)
}

// ReadRecipe reads a recipe file.
func ReadRecipe(path string) (Recipe, error) {
	var r Recipe
	b, err := os.ReadFile(path)
	if err != nil {
		return r, err
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return r, fmt.Errorf("%s: %w", path, err)
	}
	if err := r.takeExtraFiles(filepath.Dir(path)); err != nil {
		return r, err
	}
	return r, r.check()
}

// takeExtraFiles adds each extras file's zones' extras (ExtraFiles, read
// from dir) after the zone's own, naming the zone by its world file and
// index; then the recipe names no files (what is built holds them all).
func (r *Recipe) takeExtraFiles(dir string) error {
	for _, name := range r.ExtraFiles {
		path := name
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, name)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var f ExtrasFile
		if err := json.Unmarshal(b, &f); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		for _, z := range f.Zones {
			found := false
			for i := range r.Zones {
				if strings.EqualFold(r.Zones[i].From, z.From) && r.Zones[i].Zone == z.Zone {
					r.Zones[i].Extras = append(r.Zones[i].Extras, z.Extras...)
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("%s: %s zone %d is not one of the recipe's", path, z.From, z.Zone)
			}
		}
	}
	r.ExtraFiles = nil
	return nil
}

func (r Recipe) check() error {
	if r.World < 6 || r.World > 255 {
		return fmt.Errorf("world %d: a composed world is a slot past the disc's six", r.World)
	}
	if len(r.Zones) == 0 {
		return errors.New("no zones")
	}
	if len(r.Zones) > 255 {
		return fmt.Errorf("%d zones: at most 255", len(r.Zones))
	}
	for i, z := range r.Zones {
		if z.From == "" || z.Zone < 0 || z.Zone > 0xFFFF {
			return fmt.Errorf("zone %d: %q zone %d", i, z.From, z.Zone)
		}
		if len(z.Extras) > 0xFFFF {
			return fmt.Errorf("zone %d: %d extras, at most 65535", i, len(z.Extras))
		}
		sources := map[ArtSource]bool{}
		for k, e := range z.Extras {
			if err := e.check(); err != nil {
				return fmt.Errorf("zone %d (%s %d) extra %d: %w", i, z.From, z.Zone, k, err)
			}
			if s := e.source(); s.From != "" {
				sources[s] = true
			}
		}
		if len(sources) > 254 {
			return fmt.Errorf("zone %d: its extras' art from %d places, at most 254", i, len(sources))
		}
	}
	return nil
}

// ReadBuilt reads a built recipe (worldN.json).
func ReadBuilt(path string) (Built, error) {
	var b Built
	raw, err := os.ReadFile(path)
	if err != nil {
		return b, err
	}
	if err := json.Unmarshal(raw, &b); err != nil {
		return b, fmt.Errorf("%s: %w", path, err)
	}
	if len(b.Parts) != len(b.Zones) {
		return b, fmt.Errorf("%s: %d parts for %d zones", path, len(b.Parts), len(b.Zones))
	}
	return b, b.check()
}

// srcWorld is a source world file's index: where each zone's record is, its
// proxies, its grid and its regions.
type srcWorld struct {
	r       io.ReaderAt
	size    int64
	zones   [][2]int64 // record offset, record length (header and body)
	proxies [][]byte   // 56 bytes each, as stored
	rows    int
	cols    int
	regions []byte
}

func readAt(r io.ReaderAt, off int64, n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := r.ReadAt(b, off); err != nil {
		return nil, err
	}
	return b, nil
}

type hdr struct {
	ty      uint16
	version int16
	size    int64
	count   int
	at      int64 // the record's offset
}

func readHdr(r io.ReaderAt, at, limit int64) (hdr, error) {
	if at+12 > limit {
		return hdr{}, fmt.Errorf("a record header at %d runs past %d", at, limit)
	}
	b, err := readAt(r, at, 12)
	if err != nil {
		return hdr{}, err
	}
	h := hdr{ty: binary.LittleEndian.Uint16(b), version: int16(binary.LittleEndian.Uint16(b[2:])),
		size: int64(int32(binary.LittleEndian.Uint32(b[4:]))), count: int(int32(binary.LittleEndian.Uint32(b[8:]))), at: at}
	if h.size < 0 || at+12+h.size > limit || h.count < 0 {
		return hdr{}, fmt.Errorf("record %#x at %d: size %d, %d records", h.ty, at, h.size, h.count)
	}
	return h, nil
}

// children are the records directly under the record h (they begin its body).
func children(r io.ReaderAt, h hdr) ([]hdr, error) {
	out := make([]hdr, 0, h.count)
	p, end := h.at+12, h.at+12+h.size
	for i := 0; i < h.count; i++ {
		c, err := readHdr(r, p, end)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
		p = c.at + 12 + c.size
	}
	return out, nil
}

func openWorld(r io.ReaderAt, size int64) (*srcWorld, error) {
	head, err := readAt(r, 0, 32)
	if err != nil {
		return nil, err
	}
	if string(head[:4]) != "FJBO" {
		return nil, errors.New("not an OBJF file")
	}
	start := int64(binary.LittleEndian.Uint64(head[16:]))
	root, err := readHdr(r, start, size)
	if err != nil {
		return nil, err
	}
	if root.ty != typeRoot {
		return nil, fmt.Errorf("its first record is %#x, not a world's root", root.ty)
	}
	top, err := children(r, root)
	if err != nil {
		return nil, err
	}
	w := &srcWorld{r: r, size: size}
	var base *hdr
	for i := range top {
		switch top[i].ty {
		case typeWorld:
			zs, err := children(r, top[i])
			if err != nil {
				return nil, err
			}
			for _, z := range zs {
				if z.ty == typeZone {
					w.zones = append(w.zones, [2]int64{z.at, 12 + z.size})
				}
			}
		case typeWorldBase:
			base = &top[i]
		}
	}
	if base == nil {
		return nil, errors.New("no world base")
	}
	bs, err := children(r, *base)
	if err != nil {
		return nil, err
	}
	for _, c := range bs {
		body, err := readAt(r, c.at+12, int(c.size))
		if err != nil {
			return nil, err
		}
		switch c.ty {
		case typeBaseHeader:
			if c.version == 0 || len(body) < 56 {
				return nil, fmt.Errorf("world header v%d of %d bytes", c.version, len(body))
			}
			w.rows = int(int32(binary.LittleEndian.Uint32(body[28:])))
			w.cols = int(int32(binary.LittleEndian.Uint32(body[32:])))
			if d := int32(binary.LittleEndian.Uint32(body[36:])); d != subdiv {
				return nil, fmt.Errorf("world header: %d sub-cells a side", d)
			}
		case typeZoneProxies:
			n := int(int32(binary.LittleEndian.Uint32(body)))
			if n < 0 || 4+n*proxySize > len(body) {
				return nil, fmt.Errorf("%d proxies in %d bytes", n, len(body))
			}
			for i := 0; i < n; i++ {
				w.proxies = append(w.proxies, body[4+i*proxySize:4+(i+1)*proxySize])
			}
		case typeWorldRegions:
			w.regions = body
		}
	}
	return w, nil
}

// proxyOf is the proxy whose record offset is the zone's: found by matching,
// never by index (authoring-a-zone.md: a patched world's proxies went stale).
func (w *srcWorld) proxyOf(zi int) ([]byte, error) {
	at := w.zones[zi][0]
	for _, p := range w.proxies {
		if int64(binary.LittleEndian.Uint64(p)) == at {
			return p, nil
		}
	}
	return nil, fmt.Errorf("no proxy names zone %d (record at %d): the file's proxies are stale", zi, at)
}

// region is the source's profile of sub-cell (r, c), 0 outside its grid.
func (w *srcWorld) region(r, c int) byte {
	wide, high := w.cols*subdiv, w.rows*subdiv
	if r < 0 || c < 0 || r >= high || c >= wide || r*wide+c >= len(w.regions) {
		return 0
	}
	return w.regions[r*wide+c]
}

func f32(b []byte, o int) float32 { return math.Float32frombits(binary.LittleEndian.Uint32(b[o:])) }

// part is a zone in the making.
type part struct {
	src      *srcWorld
	rec      []byte
	proxy    []byte
	col, row int
	box      [6]float32
}

// Compose builds the world a recipe names out of src, and says what it built.
func Compose(src Source, r Recipe) ([]byte, Built, error) {
	if err := r.check(); err != nil {
		return nil, Built{}, err
	}
	worlds := map[string]*srcWorld{}
	parts := make([]part, len(r.Zones))
	cells := map[[2]int]int{}
	for i, z := range r.Zones {
		name := strings.ToUpper(z.From)
		w := worlds[name]
		if w == nil {
			rd, size, err := src.Open(name)
			if err != nil {
				return nil, Built{}, fmt.Errorf("%s: %w", name, err)
			}
			if w, err = openWorld(rd, size); err != nil {
				return nil, Built{}, fmt.Errorf("%s: %w", name, err)
			}
			worlds[name] = w
		}
		if z.Zone >= len(w.zones) {
			return nil, Built{}, fmt.Errorf("%s has %d zones, not %d", name, len(w.zones), z.Zone+1)
		}
		proxy, err := w.proxyOf(z.Zone)
		if err != nil {
			return nil, Built{}, fmt.Errorf("%s: %w", name, err)
		}
		rec, err := readAt(w.r, w.zones[z.Zone][0], int(w.zones[z.Zone][1]))
		if err != nil {
			return nil, Built{}, err
		}
		p := part{src: w, rec: rec, proxy: append([]byte(nil), proxy...)}
		for k := range p.box {
			p.box[k] = f32(proxy, 32+4*k)
		}
		// its cell: where the middle of its box is
		p.col = int(math.Floor(float64(p.box[0]+p.box[3]) / 2 / cellSize))
		p.row = int(math.Floor(float64(p.box[2]+p.box[5]) / 2 / cellSize))
		if p.col < 0 || p.row < 0 || p.col > 255 || p.row > 255 {
			return nil, Built{}, fmt.Errorf("%s zone %d: cell (%d, %d)", name, z.Zone, p.col, p.row)
		}
		// within its cell, as every retail zone is
		if lo, hi := float64(p.col*cellSize), float64((p.col+1)*cellSize); float64(p.box[0]) < lo-1 || float64(p.box[3]) > hi+1 {
			return nil, Built{}, fmt.Errorf("%s zone %d: x %.0f..%.0f is not inside its cell %d", name, z.Zone, p.box[0], p.box[3], p.col)
		}
		if lo, hi := float64(p.row*cellSize), float64((p.row+1)*cellSize); float64(p.box[2]) < lo-1 || float64(p.box[5]) > hi+1 {
			return nil, Built{}, fmt.Errorf("%s zone %d: z %.0f..%.0f is not inside its cell %d", name, z.Zone, p.box[2], p.box[5], p.row)
		}
		key := [2]int{p.col, p.row}
		if j, taken := cells[key]; taken {
			return nil, Built{}, fmt.Errorf("zones %d (%s %d) and %d (%s %d) both stand in cell (%d, %d): a world holds one zone a cell",
				j, r.Zones[j].From, r.Zones[j].Zone, i, z.From, z.Zone, p.col, p.row)
		}
		cells[key] = i
		parts[i] = p
	}

	cols, rows := 0, 0
	for _, p := range parts {
		cols, rows = max(cols, p.col+1), max(rows, p.row+1)
	}

	// the extras' art, read off the disc before anything is written
	arts := zoneArt{}
	for _, z := range r.Zones {
		for _, e := range z.Extras {
			s := e.source()
			if s.From == "" || arts[s] != nil {
				continue
			}
			a, err := openArt(src, s)
			if err != nil {
				return nil, Built{}, fmt.Errorf("%s zone %d's extras: %w", z.From, z.Zone, err)
			}
			arts[s] = a
		}
	}
	brought := make([]map[uint32][]byte, len(parts))

	built := Built{Recipe: r, Rows: rows, Cols: cols}
	out := make([]byte, 0, 1<<20)
	le32 := func(v uint32) { out = binary.LittleEndian.AppendUint32(out, v) }
	f := func(v float32) { le32(math.Float32bits(v)) }
	begin := func(ty uint16, version int16, count int) int {
		at := len(out)
		out = binary.LittleEndian.AppendUint16(out, ty)
		out = binary.LittleEndian.AppendUint16(out, uint16(version))
		le32(0) // its size, once its body is written
		le32(uint32(count))
		return at
	}
	end := func(at int) {
		binary.LittleEndian.PutUint32(out[at+4:], uint32(len(out)-at-12))
	}

	out = append(out, "FJBO"...)
	le32(1)
	le32(retailHeaderWord)
	le32(0)
	out = binary.LittleEndian.AppendUint64(out, 32)
	out = binary.LittleEndian.AppendUint64(out, ^uint64(0))

	root := begin(typeRoot, 0, 2)
	world := begin(typeWorld, 0, len(parts))
	for i := range parts {
		p := &parts[i]
		at := int64(len(out))
		srcAt := p.src.zones[r.Zones[i].Zone][0]
		copiedIn := 0
		if extras := r.Zones[i].Extras; len(extras) > 0 {
			z, err := withExtras(p.rec, srcAt, at, extras, arts)
			if err != nil {
				return nil, Built{}, fmt.Errorf("%s zone %d: %w", r.Zones[i].From, r.Zones[i].Zone, err)
			}
			out = append(out, z.rec...)
			binary.LittleEndian.PutUint64(p.proxy[0:], uint64(at))
			binary.LittleEndian.PutUint64(p.proxy[8:], uint64(at+z.baseAt))
			binary.LittleEndian.PutUint32(p.proxy[16:], uint32(z.baseLen))
			brought[i], copiedIn = z.brought, z.copied
		} else {
			rec := append([]byte(nil), p.rec...)
			baseAt, err := rebase(rec, at-srcAt)
			if err != nil {
				return nil, Built{}, fmt.Errorf("%s zone %d: %w", r.Zones[i].From, r.Zones[i].Zone, err)
			}
			out = append(out, rec...)
			binary.LittleEndian.PutUint64(p.proxy[0:], uint64(at))
			binary.LittleEndian.PutUint64(p.proxy[8:], uint64(at+baseAt))
		}
		var c [3]float32
		for k := range c {
			c[k] = f32(p.proxy, 20+4*k)
		}
		built.Parts = append(built.Parts, BuiltZone{From: strings.ToUpper(r.Zones[i].From), Zone: r.Zones[i].Zone,
			Size: len(p.rec), CRC: crc32.ChecksumIEEE(p.rec), Col: p.col, Row: p.row, Box: p.box, Start: c,
			Extras: len(r.Zones[i].Extras), Brought: copiedIn})
	}
	end(world)
	if err := clashes(parts, brought); err != nil {
		return nil, Built{}, err
	}

	base := begin(typeWorldBase, 2, 4)
	h := begin(typeBaseHeader, 1, 0)
	le32(uint32(len(parts)))
	f(0)
	f(-1000)
	f(0)
	f(float32(cols * cellSize))
	f(1000)
	f(float32(rows * cellSize))
	le32(uint32(rows))
	le32(uint32(cols))
	le32(subdiv)
	f(cellSize)
	f(originXZ)
	f(0)
	f(originXZ)
	end(h)

	px := begin(typeZoneProxies, 2, 0)
	le32(uint32(len(parts)))
	for _, p := range parts {
		out = append(out, p.proxy...)
	}
	end(px)

	t := begin(typeWorldTree, 0, 0)
	out = append(out, worldTree(parts)...)
	end(t)

	g := begin(typeWorldRegions, 0, 0)
	out = append(out, regions(parts, rows, cols)...)
	end(g)
	end(base)
	end(root)

	built.Size = int64(len(out))
	built.CRC = crc32.ChecksumIEEE(out)
	return out, built, nil
}

// rebase moves a zone record's absolute file offsets by delta (it moved that
// far): its ResourceTable's and its ZoneStaticTable's (-1 is none and stays),
// and says where its ZoneBase record is in it.
func rebase(rec []byte, delta int64) (int64, error) {
	r := bytesReader(rec)
	zone, err := readHdr(r, 0, int64(len(rec)))
	if err != nil {
		return 0, err
	}
	if zone.ty != typeZone {
		return 0, fmt.Errorf("record %#x, not a zone", zone.ty)
	}
	kids, err := children(r, zone)
	if err != nil {
		return 0, err
	}
	var zb *hdr
	for i := range kids {
		if kids[i].ty == typeZoneBase {
			zb = &kids[i]
		}
	}
	if zb == nil {
		return 0, errors.New("no zone base")
	}
	bk, err := children(r, *zb)
	if err != nil {
		return 0, err
	}
	moved := 0
	for _, c := range bk {
		body := rec[c.at+12 : c.at+12+c.size]
		switch c.ty {
		case typeResourceTable:
			n := int(int32(binary.LittleEndian.Uint32(body)))
			if n < 0 || 4+16*n > len(body) {
				return 0, fmt.Errorf("resource table: %d entries in %d bytes", n, len(body))
			}
			for i := 0; i < n; i++ {
				o := 4 + 16*i
				v := int64(binary.LittleEndian.Uint64(body[o:]))
				binary.LittleEndian.PutUint64(body[o:], uint64(v+delta))
				moved++
			}
		case typeStaticTable:
			n := int(int32(binary.LittleEndian.Uint32(body)))
			if n < 0 || 4+24*n != len(body) {
				return 0, fmt.Errorf("static table: %d rooms in %d bytes", n, len(body))
			}
			for i := 0; i < n; i++ {
				for _, o := range [2]int{4 + 24*i, 4 + 24*i + 12} {
					if v := int64(binary.LittleEndian.Uint64(body[o:])); v >= 0 {
						binary.LittleEndian.PutUint64(body[o:], uint64(v+delta))
						moved++
					}
				}
			}
		}
	}
	if moved == 0 {
		return 0, errors.New("no resource table to move: a zone that cannot be moved")
	}
	return zb.at, nil
}

type bytesReader []byte

func (b bytesReader) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off > int64(len(b)) {
		return 0, io.EOF
	}
	n := copy(p, b[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// worldTree is the world's BSP (0x8230's body): split at cell edges, the
// wider spread of columns or rows first, until each side holds one zone. One
// zone is the single-zone shape (guk.ESF's, zonelift's): one node, both
// leaves that zone. A node is (nx, ny, nz, d, front, back), its front where
// n.p + d >= 0 (vi::world::World::find_zone); a leaf reference has 0x40000000.
func worldTree(parts []part) []byte {
	type node struct {
		n           [3]float32
		d           float32
		front, back uint32
	}
	var nodes []node
	var leaves []uint32
	idx := make([]int, len(parts))
	for i := range idx {
		idx[i] = i
	}
	if len(parts) == 1 {
		nodes = append(nodes, node{n: [3]float32{-1, 0, 0}, d: float32((parts[0].col*2 + 1) * cellSize / 2), front: leafBit | 0, back: leafBit | 1})
		leaves = []uint32{0, 0}
	} else {
		var build func(set []int) uint32
		build = func(set []int) uint32 {
			if len(set) == 1 {
				leaves = append(leaves, uint32(set[0]))
				return leafBit | uint32(len(leaves)-1)
			}
			spread := func(cell func(p part) int) (int, int) {
				lo, hi := math.MaxInt, math.MinInt
				for _, i := range set {
					lo, hi = min(lo, cell(parts[i])), max(hi, cell(parts[i]))
				}
				return lo, hi
			}
			cl, ch := spread(func(p part) int { return p.col })
			rl, rh := spread(func(p part) int { return p.row })
			onX := ch-cl >= rh-rl
			cell := func(i int) int {
				if onX {
					return parts[i].col
				}
				return parts[i].row
			}
			s := append([]int(nil), set...)
			sort.SliceStable(s, func(a, b int) bool {
				if cell(s[a]) != cell(s[b]) {
					return cell(s[a]) < cell(s[b])
				}
				return s[a] < s[b]
			})
			// the edge nearest the middle that leaves zones on both sides
			k, best := 0, math.MaxInt
			for j := 1; j < len(s); j++ {
				if cell(s[j]) == cell(s[j-1]) {
					continue
				}
				if d := abs(2*j - len(s)); d < best {
					k, best = j, d
				}
			}
			edge := float32(cell(s[k]) * cellSize)
			me := len(nodes)
			nd := node{d: edge}
			if onX {
				nd.n = [3]float32{-1, 0, 0}
			} else {
				nd.n = [3]float32{0, 0, -1}
			}
			nodes = append(nodes, nd)
			front := build(s[:k]) // below the edge: edge - x >= 0
			back := build(s[k:])
			nodes[me].front, nodes[me].back = front, back
			return uint32(me)
		}
		build(idx)
	}
	b := binary.LittleEndian.AppendUint32(nil, uint32(len(nodes)))
	b = binary.LittleEndian.AppendUint32(b, uint32(len(leaves)))
	for _, n := range nodes {
		for _, v := range n.n {
			b = binary.LittleEndian.AppendUint32(b, math.Float32bits(v))
		}
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(n.d))
		b = binary.LittleEndian.AppendUint32(b, n.front)
		b = binary.LittleEndian.AppendUint32(b, n.back)
	}
	for _, l := range leaves {
		b = binary.LittleEndian.AppendUint32(b, l)
	}
	return b
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// regions is the world's terrain profiles (0x8240's body): one byte a
// sub-cell, rows*4 by cols*4. Each zone takes its own world's bytes over its
// cell and the sub-cell round it (a profile is sampled at sub-cell centres,
// half a sub-cell off the cell's edges: vi::world::Grid::profile_blend), a
// cell's own bytes over any neighbour's margin; 0 elsewhere.
func regions(parts []part, rows, cols int) []byte {
	wide, high := cols*subdiv, rows*subdiv
	out := make([]byte, wide*high)
	put := func(p part, margin int) {
		for r := p.row*subdiv - margin; r < (p.row+1)*subdiv+margin; r++ {
			for c := p.col*subdiv - margin; c < (p.col+1)*subdiv+margin; c++ {
				if r >= 0 && c >= 0 && r < high && c < wide {
					out[r*wide+c] = p.src.region(r, c)
				}
			}
		}
	}
	for _, p := range parts {
		put(p, 1)
	}
	for _, p := range parts {
		put(p, 0)
	}
	return out
}

// DiscSource reads the disc image's files (ISO 9660), by name in any case.
type DiscSource struct {
	f     *os.File
	files map[string][2]int64
}

// OpenDisc opens a disc image.
func OpenDisc(path string) (*DiscSource, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	d := &DiscSource{f: f, files: map[string][2]int64{}}
	if err := d.walk(); err != nil {
		f.Close()
		return nil, err
	}
	return d, nil
}

// Close closes the image.
func (d *DiscSource) Close() error { return d.f.Close() }

// Open is a file of the disc, by name (no directory) in any case.
func (d *DiscSource) Open(name string) (io.ReaderAt, int64, error) {
	e, ok := d.files[strings.ToUpper(name)]
	if !ok {
		return nil, 0, fmt.Errorf("%s is not on the disc", name)
	}
	return io.NewSectionReader(d.f, e[0], e[1]), e[1], nil
}

const sector = 2048

func (d *DiscSource) walk() error {
	pvd, err := readAt(d.f, 16*sector, sector)
	if err != nil {
		return err
	}
	if string(pvd[1:6]) != "CD001" {
		return errors.New("not an ISO 9660 image")
	}
	type dir struct{ lba, size int64 }
	rootRec := pvd[156 : 156+34]
	stack := []dir{{int64(binary.LittleEndian.Uint32(rootRec[2:])), int64(binary.LittleEndian.Uint32(rootRec[10:]))}}
	seen := map[int64]bool{}
	for len(stack) > 0 {
		dr := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[dr.lba] {
			continue
		}
		seen[dr.lba] = true
		body, err := readAt(d.f, dr.lba*sector, int(dr.size))
		if err != nil {
			return err
		}
		for off := 0; off < len(body); {
			n := int(body[off])
			if n == 0 {
				off = (off/sector + 1) * sector
				continue
			}
			if off+n > len(body) || n < 34 {
				break
			}
			rec := body[off : off+n]
			lba := int64(binary.LittleEndian.Uint32(rec[2:]))
			size := int64(binary.LittleEndian.Uint32(rec[10:]))
			nl := int(rec[32])
			name := string(rec[33 : 33+nl])
			off += n
			if name == "\x00" || name == "\x01" {
				continue
			}
			if rec[25]&2 != 0 {
				stack = append(stack, dir{lba, size})
				continue
			}
			if i := strings.IndexByte(name, ';'); i >= 0 {
				name = name[:i]
			}
			d.files[strings.ToUpper(name)] = [2]int64{lba * sector, size}
		}
	}
	return nil
}
