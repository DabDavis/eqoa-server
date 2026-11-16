package weather

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Profiles is one world's terrain-profile grid, as cmd/groundbake writes it
// (<world>.profiles beside <world>.ground): the world base's 0x8220 header and
// its 0x8240 bytes, one VITerrainProfile index per sub-cell, row-major over
// the world (wide = Cols*Subdiv), rows along z and columns along x; sub-cell
// (i, j) spans x from OriginX - Cell/2 + i*Cell/Subdiv (vanilla's
// Grid::profile_blend). On the disc every grid is 2000-unit cells, subdiv 4,
// origin (1000, 0, 1000): a sub-cell is a 500x500 square, i = sx.
//
// The file, little-endian: "WXPR", u8 version 1, i32 rows, i32 cols, i32
// subdiv, f32 cell, f32 origin_x, f32 origin_z, then rows*cols*subdiv*subdiv
// bytes.
type Profiles struct {
	Rows, Cols, Subdiv int
	Cell               float32
	OriginX, OriginZ   float32
	Data               []byte
}

const (
	profilesMagic   = "WXPR"
	profilesVersion = 1
	// ProfilesExt is the file's suffix: <world>.profiles.
	ProfilesExt = ".profiles"
)

// Check holds a grid to what the file may say.
func (p *Profiles) Check() error {
	if p.Rows <= 0 || p.Cols <= 0 || p.Subdiv <= 0 || p.Rows > 4096 || p.Cols > 4096 || p.Subdiv > 64 {
		return fmt.Errorf("a %dx%d grid of subdiv %d", p.Rows, p.Cols, p.Subdiv)
	}
	if !(p.Cell > 0) || math.IsInf(float64(p.Cell), 0) || math.IsNaN(float64(p.OriginX)) || math.IsNaN(float64(p.OriginZ)) {
		return fmt.Errorf("cell %g, origin (%g, %g)", p.Cell, p.OriginX, p.OriginZ)
	}
	if n := p.Rows * p.Cols * p.Subdiv * p.Subdiv; len(p.Data) != n {
		return fmt.Errorf("%d profile bytes, want %d", len(p.Data), n)
	}
	return nil
}

// WriteProfiles writes a grid as the file.
func WriteProfiles(w io.Writer, p *Profiles) error {
	if err := p.Check(); err != nil {
		return err
	}
	b := make([]byte, 0, 29+len(p.Data))
	b = append(b, profilesMagic...)
	b = append(b, profilesVersion)
	le := binary.LittleEndian
	for _, v := range []int{p.Rows, p.Cols, p.Subdiv} {
		b = le.AppendUint32(b, uint32(int32(v)))
	}
	for _, v := range []float32{p.Cell, p.OriginX, p.OriginZ} {
		b = le.AppendUint32(b, math.Float32bits(v))
	}
	b = append(b, p.Data...)
	_, err := w.Write(b)
	return err
}

// ReadProfiles reads the file.
func ReadProfiles(r io.Reader) (*Profiles, error) {
	br := bufio.NewReader(r)
	head := make([]byte, 29)
	if _, err := io.ReadFull(br, head); err != nil {
		return nil, fmt.Errorf("profiles: %w", err)
	}
	if string(head[:4]) != profilesMagic {
		return nil, errors.New("profiles: not a profiles file")
	}
	if head[4] != profilesVersion {
		return nil, fmt.Errorf("profiles: version %d", head[4])
	}
	le := binary.LittleEndian
	i := func(o int) int { return int(int32(le.Uint32(head[o:]))) }
	f := func(o int) float32 { return math.Float32frombits(le.Uint32(head[o:])) }
	p := &Profiles{Rows: i(5), Cols: i(9), Subdiv: i(13), Cell: f(17), OriginX: f(21), OriginZ: f(25)}
	if p.Rows <= 0 || p.Cols <= 0 || p.Subdiv <= 0 || p.Rows > 4096 || p.Cols > 4096 || p.Subdiv > 64 {
		return nil, fmt.Errorf("profiles: a %dx%d grid of subdiv %d", p.Rows, p.Cols, p.Subdiv)
	}
	p.Data = make([]byte, p.Rows*p.Cols*p.Subdiv*p.Subdiv)
	if _, err := io.ReadFull(br, p.Data); err != nil {
		return nil, fmt.Errorf("profiles: %w", err)
	}
	if err := p.Check(); err != nil {
		return nil, fmt.Errorf("profiles: %w", err)
	}
	return p, nil
}

// At is the profile under a 500x500 square (its centre's sub-cell); false off
// the grid.
func (p *Profiles) At(sx, sz int) (int, bool) {
	if p == nil {
		return 0, false
	}
	step := float64(p.Cell) / float64(p.Subdiv)
	x0 := float64(p.OriginX) - float64(p.Cell)/2
	z0 := float64(p.OriginZ) - float64(p.Cell)/2
	i := int(math.Floor((float64(sx)*SquareSize + SquareSize/2 - x0) / step))
	j := int(math.Floor((float64(sz)*SquareSize + SquareSize/2 - z0) / step))
	wide, high := p.Cols*p.Subdiv, p.Rows*p.Subdiv
	if i < 0 || j < 0 || i >= wide || j >= high {
		return 0, false
	}
	return int(p.Data[j*wide+i]), true
}

// Squares calls fn for each 500x500 square the grid covers (its sub-cells'
// centres), with its profile.
func (p *Profiles) Squares(fn func(sx, sz, profile int)) {
	if p == nil {
		return
	}
	step := float64(p.Cell) / float64(p.Subdiv)
	x0 := float64(p.OriginX) - float64(p.Cell)/2
	z0 := float64(p.OriginZ) - float64(p.Cell)/2
	wide, high := p.Cols*p.Subdiv, p.Rows*p.Subdiv
	seen := map[[2]int]bool{}
	for j := 0; j < high; j++ {
		for i := 0; i < wide; i++ {
			sx := int(math.Floor((x0 + (float64(i)+0.5)*step) / SquareSize))
			sz := int(math.Floor((z0 + (float64(j)+0.5)*step) / SquareSize))
			if seen[[2]int{sx, sz}] {
				continue
			}
			seen[[2]int{sx, sz}] = true
			if prof, ok := p.At(sx, sz); ok {
				fn(sx, sz, prof)
			}
		}
	}
}

// ReadProfilesDir reads every <world>.profiles in dir: world to grid. A
// missing dir is no grids; a bad file is an error naming it.
func ReadProfilesDir(dir string) (map[int]*Profiles, error) {
	ents, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return map[int]*Profiles{}, nil
	} else if err != nil {
		return nil, err
	}
	out := map[int]*Profiles{}
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ProfilesExt) {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSuffix(name, ProfilesExt))
		if err != nil || n < 0 || n > 255 {
			continue
		}
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		p, err := ReadProfiles(f)
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		out[n] = p
	}
	return out, nil
}
