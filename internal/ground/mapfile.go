package ground

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"unsafe"
)

// The collision mapped rather than decoded (the user, 2026-11-07: "a shard if we
// do it right shouldnt use much more resources"). A zone decoded is on its
// process's heap: 2.4 MB on average, all six worlds' 652 MB, in every process
// that plays there, and the same bytes in each. cmd/groundmap writes what a
// zone's load makes (its vertices, its triangles, its grids; a walls buffer's and
// a walls zone's the same) into a file beside the ground file, laid out as
// memory, and the load then takes slices over a read-only mapping of it: nothing
// on the heap, the same bytes for every query, one copy in the page cache however
// many processes map it, and the kernel may drop it under pressure and read it
// again, as it may not a heap. A load costs a page fault for what is touched,
// not 6 ms of decoding.
//
// The map is the decode by construction: it is written by calling load and
// buffer and storing what they return, and it is used only while it is the map of
// the source beside it (the source's size and CRC-32C in its header), so a new
// bake is decoded as before until cmd/groundmap runs again. Without a map, or on a
// machine that cannot map (no mmap, or another byte order), every load decodes as
// it always did. A map is written to a new file and renamed, so a process that has
// the old one mapped goes on with it.
//
// A file: "EQGM", the byte-order mark, the version, the kind ('G' a ground file's,
// 'W' a walls file's), the source's size and CRC-32C, the counts, then a table of
// (offset, count) pairs, then the arrays, each at an 8-byte boundary.

const (
	mapMagic     = "EQGM"
	mapVersion   = 1
	mapOrderMark = uint32(0x01020304)
	mapKindZones = uint32('G')
	mapKindWalls = uint32('W')

	// mapZoneArrays is a ground zone's arrays: verts, tris, walls and each
	// grid's offsets and cells; mapBufArrays a walls buffer's: verts and
	// strips; mapWallArrays a walls zone's: places, offsets and cells.
	mapZoneArrays = 7
	mapBufArrays  = 2
	mapWallArrays = 3

	mapHeadSize = 4 + 4 + 4 + 4 + 8 + 4 + 4 + 4 + 4 // ..., source size, CRC, two counts, padding
)

var castagnoli = crc32.MakeTable(crc32.Castagnoli)

// nativeLittleEndian is whether this machine's memory reads as the files do.
func nativeLittleEndian() bool {
	b := [4]byte{4, 3, 2, 1}
	return *(*uint32)(unsafe.Pointer(&b[0])) == mapOrderMark
}

// mapPath is the map of a source file.
func mapPath(source string) string { return source + ".map" }

// viewOf is a slice's bytes.
func viewOf[T any](s []T) []byte {
	if len(s) == 0 {
		return nil
	}
	var z T
	return unsafe.Slice((*byte)(unsafe.Pointer(&s[0])), len(s)*int(unsafe.Sizeof(z)))
}

// sliceOf is n Ts over b.
func sliceOf[T any](b []byte, n int) []T {
	if n == 0 {
		return nil
	}
	return unsafe.Slice((*T)(unsafe.Pointer(&b[0])), n)
}

// fileCRC is a file's size and CRC-32C.
func fileCRC(f *os.File) (int64, uint32, error) {
	h := crc32.New(castagnoli)
	n, err := io.Copy(h, io.NewSectionReader(f, 0, 1<<62))
	return n, h.Sum32(), err
}

// mapWriter writes a map: the table is filled as the arrays go and written last.
type mapWriter struct {
	f     *os.File
	at    int64
	table []uint64
	next  int
}

// array writes one array and notes it in the table.
func array[T any](w *mapWriter, s []T) error {
	if pad := -w.at & 7; pad != 0 {
		if _, err := w.f.Write(make([]byte, pad)); err != nil {
			return err
		}
		w.at += pad
	}
	w.table[w.next], w.table[w.next+1] = uint64(w.at), uint64(len(s))
	w.next += 2
	if len(s) == 0 {
		return nil
	}
	b := viewOf(s)
	if _, err := w.f.Write(b); err != nil {
		return err
	}
	w.at += int64(len(b))
	return nil
}

// writeMap writes source's map, to a new file renamed over the old.
func writeMap(source string, kind uint32, counts [2]uint32, pairs int, fill func(*mapWriter) error) (int64, error) {
	src, err := os.Open(source)
	if err != nil {
		return 0, err
	}
	defer src.Close()
	size, sum, err := fileCRC(src)
	if err != nil {
		return 0, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(source), filepath.Base(source)+".map-*")
	if err != nil {
		return 0, err
	}
	defer os.Remove(tmp.Name()) // gone already when it was renamed
	defer tmp.Close()
	head := make([]byte, mapHeadSize)
	copy(head, mapMagic)
	binary.LittleEndian.PutUint32(head[4:], mapOrderMark)
	binary.LittleEndian.PutUint32(head[8:], mapVersion)
	binary.LittleEndian.PutUint32(head[12:], kind)
	binary.LittleEndian.PutUint64(head[16:], uint64(size))
	binary.LittleEndian.PutUint32(head[24:], sum)
	binary.LittleEndian.PutUint32(head[28:], counts[0])
	binary.LittleEndian.PutUint32(head[32:], counts[1])
	if _, err := tmp.Write(head); err != nil {
		return 0, err
	}
	w := &mapWriter{f: tmp, at: mapHeadSize + int64(pairs)*16, table: make([]uint64, pairs*2)}
	if _, err := tmp.Write(make([]byte, pairs*16)); err != nil {
		return 0, err
	}
	if err := fill(w); err != nil {
		return 0, err
	}
	if w.next != len(w.table) {
		return 0, fmt.Errorf("%d array entries of %d written", w.next/2, pairs)
	}
	if _, err := tmp.WriteAt(viewOf(w.table), mapHeadSize); err != nil {
		return 0, err
	}
	if err := tmp.Chmod(0o644); err != nil {
		return 0, err
	}
	if err := tmp.Sync(); err != nil {
		return 0, err
	}
	if err := tmp.Close(); err != nil {
		return 0, err
	}
	return w.at, os.Rename(tmp.Name(), mapPath(source))
}

// mapFile is a map open: the file's bytes, and its table of (offset, count).
type mapFile struct {
	data  []byte
	table []uint64
}

// openMap maps source's map, when there is one of this source. None is no error.
func openMap(source string, src *os.File, kind uint32, counts [2]uint32, pairs int) (*mapFile, error) {
	f, err := os.Open(mapPath(source))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	data, err := mmapFile(f)
	if err != nil || data == nil {
		return nil, err
	}
	bad := func(why string) (*mapFile, error) {
		munmapFile(data)
		return nil, fmt.Errorf("%s: %s", mapPath(source), why)
	}
	if len(data) < mapHeadSize+pairs*16 || string(data[:4]) != mapMagic {
		return bad("not a map")
	}
	if !nativeLittleEndian() {
		munmapFile(data)
		return nil, nil // another byte order: decoded as ever
	}
	if binary.LittleEndian.Uint32(data[4:]) != mapOrderMark {
		return bad("not a map")
	}
	if binary.LittleEndian.Uint32(data[8:]) != mapVersion {
		return bad("another version")
	}
	if binary.LittleEndian.Uint32(data[12:]) != kind ||
		binary.LittleEndian.Uint32(data[28:]) != counts[0] || binary.LittleEndian.Uint32(data[32:]) != counts[1] {
		return bad("another file's")
	}
	size, sum, err := fileCRC(src)
	if err != nil {
		return bad(err.Error())
	}
	if uint64(size) != binary.LittleEndian.Uint64(data[16:]) || sum != binary.LittleEndian.Uint32(data[24:]) {
		return bad("made from another bake: cmd/groundmap writes it again")
	}
	m := &mapFile{data: data, table: sliceOf[uint64](data[mapHeadSize:], pairs*2)}
	for i := 0; i < pairs; i++ {
		off, n := m.table[2*i], m.table[2*i+1]
		if off > uint64(len(data)) || off&7 != 0 || n > uint64(len(data)) {
			return bad("its table is wrong")
		}
	}
	return m, nil
}

// close unmaps it.
func (m *mapFile) close() {
	if m != nil {
		munmapFile(m.data)
	}
}

// mapped is entry i of the table as Ts, nil when it does not fit the file.
func mapped[T any](m *mapFile, i int) []T {
	off, n := m.table[2*i], m.table[2*i+1]
	var z T
	if n == 0 || off+n*uint64(unsafe.Sizeof(z)) > uint64(len(m.data)) {
		return nil
	}
	return sliceOf[T](m.data[off:], int(n))
}

// zone is ground zone i as load makes it, its arrays the map's.
func (w *world) mappedZone(i int) *zoneGrid {
	e, at := w.entries[i], i*mapZoneArrays
	z := w.emptyZone(e)
	z.verts = mapped[Vertex](w.m, at)
	z.tris = mapped[int32](w.m, at+1)
	z.walls = mapped[int32](w.m, at+2)
	z.floors.offsets = mapped[int32](w.m, at+3)
	z.floors.cells = mapped[int32](w.m, at+4)
	z.wallGrid.offsets = mapped[int32](w.m, at+5)
	z.wallGrid.cells = mapped[int32](w.m, at+6)
	return z
}

// mappedBuffer is walls buffer i as buffer makes it, its arrays the map's.
func (w *wallWorld) mappedBuffer(i int32) *wallBuffer {
	at := int(i) * mapBufArrays
	return &wallBuffer{verts: mapped[Vertex](w.m, at), strips: mapped[wallStrip](w.m, at+1)}
}

// mappedZone is walls zone i as load makes it, its arrays the map's.
func (w *wallWorld) mappedZone(i int) *wallZone {
	e := w.zones[i]
	at := len(w.bufs)*mapBufArrays + i*mapWallArrays
	z := wallZoneOf(e)
	z.places = mapped[placed](w.m, at)
	z.offsets = mapped[int32](w.m, at+1)
	z.cells = mapped[int32](w.m, at+2)
	return z
}

// WriteMaps writes the map of every ground and walls file in dir, reporting each
// by report (nil: none). One zone is in memory at a time.
func WriteMaps(dir string, report func(file string, bytes int64)) error {
	said := func(file string, n int64) {
		if report != nil {
			report(file, n)
		}
	}
	any := false
	for n := 0; n < 256; n++ {
		path := filepath.Join(dir, fmt.Sprintf("%d.ground", n))
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		w, err := readTable(f)
		if err != nil {
			f.Close()
			return fmt.Errorf("%s: %w", path, err)
		}
		size, err := writeMap(path, mapKindZones, [2]uint32{uint32(len(w.entries)), 0}, len(w.entries)*mapZoneArrays, func(mw *mapWriter) error {
			for i := range w.entries {
				z, err := w.load(i)
				if err != nil {
					return fmt.Errorf("zone %d: %w", i, err)
				}
				if err := array(mw, z.verts); err != nil {
					return err
				}
				if err := array(mw, z.tris); err != nil {
					return err
				}
				if err := array(mw, z.walls); err != nil {
					return err
				}
				if err := array(mw, z.floors.offsets); err != nil {
					return err
				}
				if err := array(mw, z.floors.cells); err != nil {
					return err
				}
				if err := array(mw, z.wallGrid.offsets); err != nil {
					return err
				}
				if err := array(mw, z.wallGrid.cells); err != nil {
					return err
				}
			}
			return nil
		})
		f.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		any = true
		said(mapPath(path), size)
	}
	if !any {
		return fmt.Errorf("no ground files in %s", dir)
	}
	for n := 0; n < 256; n++ {
		path := filepath.Join(dir, fmt.Sprintf("%d.walls", n))
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		w, err := readWalls(f)
		if err != nil {
			f.Close()
			return fmt.Errorf("%s: %w", path, err)
		}
		pairs := len(w.bufs)*mapBufArrays + len(w.zones)*mapWallArrays
		size, err := writeMap(path, mapKindWalls, [2]uint32{uint32(len(w.bufs)), uint32(len(w.zones))}, pairs, func(mw *mapWriter) error {
			for i := range w.bufs {
				b, err := w.buffer(int32(i))
				if err != nil {
					return fmt.Errorf("buffer %d: %w", i, err)
				}
				if err := array(mw, b.verts); err != nil {
					return err
				}
				if err := array(mw, b.strips); err != nil {
					return err
				}
				w.read[i] = nil
			}
			for i := range w.zones {
				z, err := w.load(i)
				if err != nil {
					return fmt.Errorf("zone %d: %w", i, err)
				}
				if err := array(mw, z.places); err != nil {
					return err
				}
				if err := array(mw, z.offsets); err != nil {
					return err
				}
				if err := array(mw, z.cells); err != nil {
					return err
				}
			}
			return nil
		})
		f.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		said(mapPath(path), size)
	}
	return nil
}
