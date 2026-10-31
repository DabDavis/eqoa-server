package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"

	"github.com/eqoa/server/internal/ground"
)

// What a land sample is.
const (
	sNone uint8 = iota // no ground
	sBad               // water, slime, lava or mud
	sLand              // open land: a creature may be placed here
	sTown              // a town's ground: walked through, nothing placed
	sZone              // a boundary or sea zone of Tunaria's: walked through, nothing placed
)

// landGrid is a world's land every step units: the highest floor, what it
// is, and the reachable piece it belongs to.
type landGrid struct {
	world      int
	minX, minZ float64
	nx, nz     int
	h          []float32
	kind       []uint8
	edges      []uint8 // bit 0: joined to the sample east, bit 1: south
	piece      []int32 // the reachable piece, -1 for none
	pieceSize  []int32
}

func (l *landGrid) at(i, j int) int { return j*l.nx + i }

func (l *landGrid) xz(k int) (float64, float64) {
	return l.minX + float64(k%l.nx)*step, l.minZ + float64(k/l.nx)*step
}

// cell is the sample nearest (x, z), -1 off the grid.
func (l *landGrid) cell(x, z float64) int {
	i := int(math.Round((x - l.minX) / step))
	j := int(math.Round((z - l.minZ) / step))
	if i < 0 || j < 0 || i >= l.nx || j >= l.nz {
		return -1
	}
	return l.at(i, j)
}

// placeable reports a sample a creature may be placed on.
func (l *landGrid) placeable(k int) bool {
	return l.kind[k] == sLand && l.piece[k] >= 0 && l.pieceSize[l.piece[k]] >= minPiece
}

// extents is the span of a world's ground file's zones (its table: "EQGR",
// version, count, then each zone's min x, min z, max x, max z, base x, y, z,
// offset, strips and vertices).
func extents(dir string, w int) (minX, minZ, maxX, maxZ float64, err error) {
	f, err := os.Open(filepath.Join(dir, fmt.Sprintf("%d.ground", w)))
	if err != nil {
		return 0, 0, 0, 0, err
	}
	defer f.Close()
	head := make([]byte, 12)
	if _, err = f.Read(head); err != nil || string(head[:4]) != "EQGR" {
		return 0, 0, 0, 0, fmt.Errorf("%d.ground: not a ground file", w)
	}
	count := binary.LittleEndian.Uint32(head[8:])
	minX, minZ, maxX, maxZ = math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	e := make([]byte, 44)
	for n := uint32(0); n < count; n++ {
		if _, err = f.Read(e); err != nil {
			return
		}
		if binary.LittleEndian.Uint32(e[40:]) == 0 {
			continue
		}
		fl := func(k int) float64 { return float64(math.Float32frombits(binary.LittleEndian.Uint32(e[4*k:]))) }
		minX, minZ = math.Min(minX, fl(0)), math.Min(minZ, fl(1))
		maxX, maxZ = math.Max(maxX, fl(2)), math.Max(maxZ, fl(3))
	}
	return
}

// notLand is a Tunaria zone no creature belongs in (tools/seed_signs.py's
// NOT_LAND): the world's boundary mountains and the sea.
var notLand = regexp.MustCompile(`(?i)^(Open Sea|Empty)$|Boundary|Ocean|Dread Sea`)

// tunariaZone is the WORDS.TXT word naming Tunaria's 2,000-unit cell at (x,
// z) (CLIENT's ZoneName grid), or 641 (none).
func tunariaZone(x, z float64) int {
	row, col := int(math.Floor(z/2000))-1, int(math.Floor(x/2000))-1
	if row < 0 || row >= len(zones0) || col < 0 || col >= len(zones0[row]) {
		return 641
	}
	return zones0[row][col]
}

// zones0 is CLIENT's zone grid of Tunaria (ZoneName; tools/seed_signs.py's
// ZONES_TUNARIA): row z / 2000 - 1, column x / 2000 - 1.
var zones0 = [][]int{
	{641, 641, 641, 641, 641, 641, 641, 641, 641, 1152, 1153, 1176, 1051},
	{1046, 1057, 1058, 1074, 1075, 364, 1097, 1123, 1124, 1151, 1154, 1175, 1042},
	{1047, 1056, 1059, 1073, 1076, 1096, 1098, 1122, 1125, 1150, 361, 1174, 1045},
	{1047, 1055, 1060, 1072, 1077, 1095, 608, 1121, 1126, 1149, 1155, 1173, 1047},
	{1047, 1054, 1061, 1071, 1078, 1094, 1099, 1120, 1127, 1148, 1156, 1172, 1047},
	{1047, 1053, 1062, 602, 1079, 1093, 1100, 1119, 1128, 1147, 1157, 1171, 1047},
	{1047, 1052, 1063, 1070, 1080, 1092, 1101, 1118, 1129, 1146, 1158, 358, 1047},
	{1047, 357, 1064, 1069, 1081, 1091, 1102, 1117, 1130, 1145, 1159, 1170, 1047},
	{1047, 1050, 1065, 1068, 1082, 1090, 1103, 1116, 1131, 1144, 1160, 1047, 1047},
	{1048, 366, 1066, 1067, 1083, 1089, 1104, 1115, 1132, 1143, 1161, 1047, 1047},
	{1049, 1049, 641, 641, 641, 1088, 1105, 1114, 1133, 1142, 1162, 1047, 1047},
	{641, 641, 641, 641, 641, 1087, 1106, 1113, 1134, 1141, 1163, 1169, 1047},
	{641, 641, 641, 641, 641, 1086, 1107, 1112, 1135, 1140, 1164, 1168, 1047},
	{641, 641, 641, 641, 641, 1085, 1108, 1111, 1136, 1139, 1165, 1167, 1047},
	{641, 641, 641, 641, 641, 1084, 1109, 1110, 1137, 1138, 1166, 363, 1047},
	{641, 641, 641, 641, 641, 1047, 1047, 1047, 1047, 1047, 1047, 1047, 1047},
}

// buildLand samples a world's land (the file's types): the highest floor
// every step, what it is, and which samples join. towns are the towns'
// people (townClear round each is town ground); words, when given, names
// Tunaria's zones; seeds are the places on the open ground creatures and
// towns stand, from which the reachable pieces are found.
func buildLand(g *ground.Ground, env *ground.Env, dir string, w int, towns *index, words []string,
	seeds [][2]float64) (*landGrid, error) {
	minX, minZ, maxX, maxZ, err := extents(dir, w)
	if err != nil {
		return nil, err
	}
	l := &landGrid{world: w, minX: math.Floor(minX/step) * step, minZ: math.Floor(minZ/step) * step}
	l.nx = int((maxX-l.minX)/step) + 1
	l.nz = int((maxZ-l.minZ)/step) + 1
	n := l.nx * l.nz
	l.h = make([]float32, n)
	l.kind = make([]uint8, n)
	l.edges = make([]uint8, n)
	l.piece = make([]int32, n)
	zoneOut := map[int]bool{}
	if w == 0 && words != nil {
		for _, row := range zones0 {
			for _, word := range row {
				if word < len(words) && notLand.MatchString(words[word]) {
					zoneOut[word] = true
				}
			}
		}
	}
	for k := 0; k < n; k++ {
		x, z := l.xz(k)
		h, ok := g.Floor(uint8(w), float32(x), 1e6, float32(z))
		if !ok {
			continue
		}
		l.h[k] = h
		switch {
		case badAt(env, w, x, float64(h), z):
			l.kind[k] = sBad
		case towns.within(x, z, townClear):
			l.kind[k] = sTown
		case w == 0 && zoneOut[tunariaZone(x, z)]:
			l.kind[k] = sZone
		default:
			l.kind[k] = sLand
		}
	}
	walk := func(k int) bool { return l.kind[k] >= sLand }
	for j := 0; j < l.nz; j++ {
		for i := 0; i < l.nx; i++ {
			k := l.at(i, j)
			if !walk(k) {
				continue
			}
			x, z := l.xz(k)
			if i+1 < l.nx && walk(k+1) && joins(g, w, x, l.h[k], z, x+step, l.h[k+1], z) {
				l.edges[k] |= 1
			}
			if j+1 < l.nz && walk(k+l.nx) && joins(g, w, x, l.h[k], z, x, l.h[k+l.nx], z+step) {
				l.edges[k] |= 2
			}
		}
	}
	for k := range l.piece {
		l.piece[k] = -1
	}
	for _, s := range seeds {
		k := l.cell(s[0], s[1])
		if k < 0 || !walk(k) || l.piece[k] >= 0 {
			continue
		}
		id := int32(len(l.pieceSize))
		size := l.flood(k, id)
		l.pieceSize = append(l.pieceSize, size)
	}
	return l, nil
}

// joins reports two samples a creature walks between: a rise under maxRise
// and no wall between them.
func joins(g *ground.Ground, w int, x0 float64, h0 float32, z0, x1 float64, h1 float32, z1 float64) bool {
	if math.Abs(float64(h1-h0)) > maxRise {
		return false
	}
	return !g.Wall(uint8(w), float32(x0), h0+1, float32(z0), float32(x1), h1+1, float32(z1))
}

// neighbours calls f with each sample joined to k.
func (l *landGrid) neighbours(k int, f func(int)) {
	if l.edges[k]&1 != 0 {
		f(k + 1)
	}
	if l.edges[k]&2 != 0 {
		f(k + l.nx)
	}
	if k%l.nx > 0 && l.edges[k-1]&1 != 0 {
		f(k - 1)
	}
	if k >= l.nx && l.edges[k-l.nx]&2 != 0 {
		f(k - l.nx)
	}
}

// flood marks the piece reachable from k as id; its size in samples.
func (l *landGrid) flood(k int, id int32) int32 {
	stack := []int{k}
	l.piece[k] = id
	var size int32
	for len(stack) > 0 {
		c := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		size++
		l.neighbours(c, func(nb int) {
			if l.piece[nb] < 0 {
				l.piece[nb] = id
				stack = append(stack, nb)
			}
		})
	}
	return size
}

// badAt reports water, slime, lava or mud at a floor.
func badAt(env *ground.Env, w int, x, h, z float64) bool {
	fl, ok := env.Flags(uint8(w), float32(x), float32(h)+1, float32(z))
	return ok && fl&envBad != 0
}
