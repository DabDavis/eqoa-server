package worlds

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"strings"
)

//go:embed neriak-room.cut.gz
var neriakRoom []byte

// IsNeriakArena identifies the reserved disc-derived arena recipe. Older
// clients build a different CRC and cannot acknowledge the cropped world.
func IsNeriakArena(r Recipe) bool {
	return r.World == 16 && r.Name == "Neriak Duel Arena" && len(r.Zones) == 1 &&
		strings.EqualFold(r.Zones[0].From, "TUNARIA.ESF") && r.Zones[0].Zone == 45
}

// The trusted, embedded plan contains only zero, disc-relative copy, and
// authored field writes. tools/neriak_duel_room.py defines the mesh cut.
func cutNeriakRoom(rec []byte) error {
	if crc32.ChecksumIEEE(rec) != 0xfec7de9c {
		return fmt.Errorf("Neriak arena requires the Frontiers Tunaria zone 45")
	}
	source := bytes.Clone(rec)
	gz, err := gzip.NewReader(bytes.NewReader(neriakRoom))
	if err != nil {
		return err
	}
	defer gz.Close()
	code, err := io.ReadAll(gz)
	if err != nil {
		return err
	}
	for len(code) != 0 {
		if len(code) < 9 {
			return io.ErrUnexpectedEOF
		}
		op, dest, n := code[0], binary.LittleEndian.Uint32(code[1:]), binary.LittleEndian.Uint32(code[5:])
		code = code[9:]
		if uint64(dest)+uint64(n) > uint64(len(rec)) {
			return fmt.Errorf("invalid arena output range")
		}
		out := rec[dest : dest+n]
		switch op {
		case 0:
			clear(out)
		case 1:
			if len(code) < 4 {
				return io.ErrUnexpectedEOF
			}
			src := binary.LittleEndian.Uint32(code)
			code = code[4:]
			if uint64(src)+uint64(n) > uint64(len(source)) {
				return fmt.Errorf("invalid arena source range")
			}
			copy(out, source[src:src+n])
		case 2:
			if uint64(n) > uint64(len(code)) {
				return io.ErrUnexpectedEOF
			}
			copy(out, code[:n])
			code = code[n:]
		default:
			return fmt.Errorf("unknown arena cut instruction %d", op)
		}
	}
	return nil
}
