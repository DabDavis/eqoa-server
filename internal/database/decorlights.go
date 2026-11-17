package database

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// lighttemplates and lightstrings: strings of decoration lights (the owner,
// 2026-11-16: "needs lights on trees in different colors. the lights can be a
// template so we can put christmas lights on fences buildings etc. only tree for
// now"). Not retail: a client that draws them asks (handlers/decorlights.go) and
// is sent every string of a season that is on; a PS2 is sent none.
//
// A template is how a string's bulbs look: their colours (laid along the string
// in turn, or at random), how they light (steady, twinkling, alternating, a
// chase), how fast, how far apart, how big and how bright. A string is where the
// bulbs hang: on every NPC of a name (npc: in the NPC's own frame, x right, y up,
// z ahead, at its size 1, turned and scaled with it, so a tree's lights follow the
// tree wherever a GM moves or sizes it, and go when it goes) or in a world (npc
// blank: world coordinates), in a shape: a spiral round a cone (a tree's
// garland), rings round one, a line (a fence's rail, a roof's edge), the edges of
// a box (a building), or points. The server lays the bulbs (DecorBulbs); the
// client draws each as a glow at its place in its colour, lit as the template
// says, and casts their light with Dynamic Lighting on.
const decorLightsSchema = `
	CREATE TABLE IF NOT EXISTS lighttemplates (
		name    TEXT PRIMARY KEY COLLATE NOCASE,
		colors  TEXT NOT NULL DEFAULT 'FFFFFF',
		pattern TEXT NOT NULL DEFAULT 'cycle',
		effect  TEXT NOT NULL DEFAULT 'twinkle',
		period  REAL NOT NULL DEFAULT 1.5,
		spacing REAL NOT NULL DEFAULT 1,
		size    REAL NOT NULL DEFAULT 0.3,
		bright  REAL NOT NULL DEFAULT 1,
		cord    TEXT NOT NULL DEFAULT '1E3F1E',
		cord_width REAL NOT NULL DEFAULT 0.06,
		note    TEXT NOT NULL DEFAULT '',
		source  TEXT NOT NULL DEFAULT 'gm'
	);
	CREATE TABLE IF NOT EXISTS decorart (
		name   TEXT PRIMARY KEY,
		model  INTEGER NOT NULL DEFAULT 0,
		world  INTEGER NOT NULL DEFAULT 0,
		zone   INTEGER NOT NULL DEFAULT 0,
		sink   REAL NOT NULL DEFAULT 0,
		note   TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'gm'
	);
	CREATE TABLE IF NOT EXISTS lightstrings (
		id       INTEGER PRIMARY KEY,
		template TEXT NOT NULL DEFAULT '',
		npc      TEXT NOT NULL DEFAULT '',
		world    INTEGER NOT NULL DEFAULT 0,
		shape    TEXT NOT NULL DEFAULT 'line',
		params   TEXT NOT NULL DEFAULT '{}',
		season   TEXT NOT NULL DEFAULT '',
		note     TEXT NOT NULL DEFAULT '',
		source   TEXT NOT NULL DEFAULT 'gm'
	);`

// decorart: an NPC (by name) that a client drawing decorations sees in another
// body than its npcs row's: model, a sprite one of world's zones holds (zone, its
// index in the world's file), which that client reads off its disc, drawn sink
// of the art's own units lower than the NPC stands (the owner: "make the stock
// cut off so the firs touch the ground": a creature stands on the floor, so the
// art is lowered, not the NPC) (the owner's
// Christmas tree is a fir of Tunaria's northern zones, a LOD sprite no character
// file holds). Every other client (a PS2) is sent the row's own model, a body
// CHAR.ESF holds: a fir would be a sprite it has not loaded in Freeport, drawn as
// the debug "!Character".

// ensureDecorLights makes tables lighttemplates, lightstrings and decorart (initSchema).
func (db *SQLiteDB) ensureDecorLights() error {
	if _, err := db.Exec(decorLightsSchema); err != nil {
		return fmt.Errorf("lighttemplates: %w", err)
	}
	if err := db.ensureDecorArtSink(); err != nil {
		return err
	}
	if _, err := db.Exec(decorAssetsSchema); err != nil {
		return fmt.Errorf("decorassets: %w", err)
	}
	return db.ensureLightTemplateCord()
}

// How a template's bulbs light (lighttemplates.effect), as the wire's u8.
const (
	LightSteady    = "steady"    // always lit
	LightTwinkle   = "twinkle"   // each bulb brightens and dims on its own, out of step with its neighbours
	LightAlternate = "alternate" // the odd bulbs and the even ones lit in turn
	LightChase     = "chase"     // a run of light travels along the string
)

// LightEffects is every effect, in the wire's order (its index is the u8).
var LightEffects = []string{LightSteady, LightTwinkle, LightAlternate, LightChase}

// How a template lays its colours along a string (lighttemplates.pattern).
const (
	LightCycle  = "cycle"  // the colours in turn
	LightRandom = "random" // a colour at random a bulb (the same every time the string is laid)
)

// The shapes of a string (lightstrings.shape) and their params (a JSON object).
const (
	// a garland wound round a cone from its foot up: base [x,y,z] (its axis's foot, default
	// 0,0,0), r0 the radius at y0 (over the base), r1 at y1, turns
	ShapeSpiral = "spiral"
	// level rings round a cone: base, r0, y0, r1, y1, rings (how many, foot to top)
	ShapeRings = "rings"
	// from [x,y,z] to [x,y,z], sagging by sag between its posts every posts (0: none)
	ShapeLine = "line"
	// the edges of a box from min [x,y,z] to max [x,y,z]: edges "top" (the four round its
	// top: a roof line), "bottom", or "all" (the twelve)
	ShapeBox = "box"
	// points [[x,y,z], ...], a bulb at each
	ShapePoints = "points"
	// points [[x,y,z], ...] in order, a bulb at each and the cord through them: a garland laid
	// by hand, or fitted to a model's surface (the owner, 2026-11-16: the lights "not quite lined
	// on the tree perfectly": tools fit each bulb to the fir's boughs)
	ShapePath = "path"
	// presents under a tree: boxes [{at: [x,y,z] (its bottom's middle), size: [w,h,d], yaw
	// (degrees), color: RRGGBB, ribbon: RRGGBB (blank none), bow: true}], drawn as wrapped gift
	// boxes (the owner: "we need some different color presents under the tree"); no bulbs, no
	// template needed
	ShapePresents = "presents"
)

// LightShapes is every shape, in the order the panel offers them.
var LightShapes = []string{ShapeSpiral, ShapeRings, ShapeLine, ShapeBox, ShapePoints, ShapePath, ShapePresents}

// The bounds of the lights: bulbs on one string, in a message, colours a template.
const (
	DecorMaxBulbs      = 600
	DecorMaxSent       = 1500
	DecorMaxColors     = 16
	DecorMinSpacing    = 0.05
	DecorMaxPeriod     = 60
	DecorMaxSize       = 10
	DecorDefaultPeriod = 1.5
)

// LightTemplate is a row of lighttemplates.
type LightTemplate struct {
	Name    string
	Colors  []uint32 // 0xRRGGBB
	Pattern string
	Effect  string
	Period  float64
	Spacing float64
	Size    float64
	Bright  float64
	// Cord is the wire the bulbs hang on, 0xRRGGBB (CordOn false: none: the
	// column blank or "none"), CordWidth how thick, in the string's units
	Cord      uint32
	CordOn    bool
	CordWidth float64
	// The look past the glows (christmas5; the owner: "the magic globe can be
	// used for bulbs and made different colors", "the rope hanging also can be
	// used to decorate the tree"): Bulb a model drawn at each bulb (a DictID,
	// one of decorassets'; 0 none), BulbScale its size across; CordTexture the
	// cord drawn with a texture (a decorassets DictID; 0 a plain wire), a span
	// between bulbs one repeat of it; CordSag how far the cord hangs between
	// two bulbs, of their distance (0: the default)
	Bulb        uint32
	BulbScale   float64
	CordTexture uint32
	CordRepeat  float64
	CordSag     float64
	Note        string
}

// LightString is a row of lightstrings.
type LightString struct {
	ID       int64
	Template string
	NPC      string
	World    int
	Shape    string
	Params   string
	Season   string
	Note     string
}

// LightParams is a string's params, every shape's fields together.
type LightParams struct {
	Base   *[3]float64  `json:"base,omitempty"`
	R0     float64      `json:"r0,omitempty"`
	Y0     float64      `json:"y0,omitempty"`
	R1     float64      `json:"r1,omitempty"`
	Y1     float64      `json:"y1,omitempty"`
	Turns  float64      `json:"turns,omitempty"`
	Rings  int          `json:"rings,omitempty"`
	From   *[3]float64  `json:"from,omitempty"`
	To     *[3]float64  `json:"to,omitempty"`
	Sag    float64      `json:"sag,omitempty"`
	Posts  float64      `json:"posts,omitempty"`
	Min    *[3]float64  `json:"min,omitempty"`
	Max    *[3]float64  `json:"max,omitempty"`
	Edges  string       `json:"edges,omitempty"`
	Points [][3]float64 `json:"points,omitempty"`
	// Star, when set with Top, is where the star stands (any shape; a spiral's or rings' is
	// over its top without it)
	Star *[3]float64 `json:"star,omitempty"`
	// Boxes are a presents string's gift boxes
	Boxes []GiftBox `json:"boxes,omitempty"`
	// Spacing, when set, is this string's own in place of its template's
	Spacing float64 `json:"spacing,omitempty"`
	// Top, when set, is a bulb of this colour (0xRRGGBB, as a hex string) at the
	// spiral's or rings' top: a tree's star
	Top string `json:"top,omitempty"`
}

// Bulb is one light a string lays: its place (the NPC's frame at size 1, or the
// world's), its colour (an index into the template's), and its phase (0 to 255:
// where in its effect's cycle it starts).
type Bulb struct {
	X, Y, Z float32
	Color   uint8
	Phase   uint8
	// Break: the cord does not run to this bulb from the one before (a ring's
	// first after the one below it, a box's edge apart from the last)
	Break bool
}

// ParseLightColors reads a comma (or space) list of RRGGBB colours, with or
// without '#' or 0x.
func ParseLightColors(s string) ([]uint32, error) {
	var out []uint32
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == ';' }) {
		f = strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(f), "#"), "0x"), "0X")
		if len(f) != 6 {
			return nil, fmt.Errorf("colour %q: RRGGBB", f)
		}
		v, err := strconv.ParseUint(f, 16, 32)
		if err != nil {
			return nil, fmt.Errorf("colour %q: RRGGBB", f)
		}
		out = append(out, uint32(v))
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no colours")
	}
	if len(out) > DecorMaxColors {
		return nil, fmt.Errorf("%d colours: %d at most", len(out), DecorMaxColors)
	}
	return out, nil
}

// LightTemplates is every row of lighttemplates, by name. A row whose colours
// do not read is left out (DecorProblems says why).
func (db *SQLiteDB) LightTemplates() (map[string]LightTemplate, error) {
	if err := db.ensureLightTemplateCord(); err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT name, colors, pattern, effect, period, spacing, size, bright, cord, cord_width, bulb, bulb_scale, cord_texture, cord_repeat, cord_sag, note FROM lighttemplates`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]LightTemplate{}
	for rows.Next() {
		var t LightTemplate
		var colors, cord, bulb, cordTex string
		if err := rows.Scan(&t.Name, &colors, &t.Pattern, &t.Effect, &t.Period, &t.Spacing, &t.Size, &t.Bright, &cord, &t.CordWidth, &bulb, &t.BulbScale, &cordTex, &t.CordRepeat, &t.CordSag, &t.Note); err != nil {
			return nil, err
		}
		t.Bulb, _ = ParseDictID(bulb)
		t.CordTexture, _ = ParseDictID(cordTex)
		if c, err := ParseLightColors(cord); err == nil && len(c) == 1 {
			t.Cord, t.CordOn = c[0], true
		}
		c, err := ParseLightColors(colors)
		if err != nil {
			continue
		}
		t.Colors = c
		out[SeasonKey(t.Name)] = t
	}
	return out, rows.Err()
}

// LightStrings is every row of lightstrings, by id.
func (db *SQLiteDB) LightStrings() ([]LightString, error) {
	rows, err := db.Query(`SELECT id, template, npc, world, shape, params, season, note FROM lightstrings ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LightString
	for rows.Next() {
		var s LightString
		if err := rows.Scan(&s.ID, &s.Template, &s.NPC, &s.World, &s.Shape, &s.Params, &s.Season, &s.Note); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// LightTemplateProblems is what is wrong with a template row's values.
func LightTemplateProblems(colors, pattern, effect string, period, spacing, size, bright float64, cord string, cordWidth float64) []string {
	var out []string
	if c := strings.TrimSpace(cord); c != "" && !strings.EqualFold(c, "none") {
		if v, err := ParseLightColors(c); err != nil || len(v) != 1 {
			out = append(out, fmt.Sprintf("cord %q: one RRGGBB, or blank (none)", cord))
		}
	}
	if cordWidth < 0 || cordWidth > 2 {
		out = append(out, fmt.Sprintf("cord_width %.2f: 0 to 2", cordWidth))
	}
	if _, err := ParseLightColors(colors); err != nil {
		out = append(out, "colors: "+err.Error())
	}
	if pattern != LightCycle && pattern != LightRandom {
		out = append(out, fmt.Sprintf("pattern %q: cycle or random", pattern))
	}
	if LightEffectByte(effect) < 0 {
		out = append(out, fmt.Sprintf("effect %q: one of %s", effect, strings.Join(LightEffects, ", ")))
	}
	if period < 0 || period > DecorMaxPeriod {
		out = append(out, fmt.Sprintf("period %.2f s: 0 (%.1f) to %d", period, DecorDefaultPeriod, DecorMaxPeriod))
	}
	if spacing < DecorMinSpacing {
		out = append(out, fmt.Sprintf("spacing %.2f: at least %.2f", spacing, DecorMinSpacing))
	}
	if size < 0 || size > DecorMaxSize {
		out = append(out, fmt.Sprintf("size %.2f: 0 (no glow: a garland's) to %d", size, DecorMaxSize))
	}
	if bright <= 0 || bright > 4 {
		out = append(out, fmt.Sprintf("bright %.2f: more than 0, at most 4", bright))
	}
	return out
}

// LightEffectByte is an effect's u8 on the wire, -1 for none of them.
func LightEffectByte(effect string) int {
	for i, e := range LightEffects {
		if strings.EqualFold(strings.TrimSpace(effect), e) {
			return i
		}
	}
	return -1
}

// ReadLightParams reads a string's params.
func ReadLightParams(s string) (LightParams, error) {
	var p LightParams
	if strings.TrimSpace(s) == "" {
		return p, nil
	}
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		return p, fmt.Errorf("params: %w", err)
	}
	return p, nil
}

// DecorBulbs lays a string's bulbs as its template says: along its shape, one
// every spacing, the colours in turn (or at random, seeded by the string's id,
// so the same string lays the same colours every time), each its own phase. It
// says what is wrong when it can lay none.
func DecorBulbs(t LightTemplate, s LightString) ([]Bulb, error) {
	p, err := ReadLightParams(s.Params)
	if err != nil {
		return nil, err
	}
	spacing := t.Spacing
	if p.Spacing > 0 {
		spacing = p.Spacing
	}
	if spacing < DecorMinSpacing {
		spacing = DecorMinSpacing
	}
	var pts [][3]float64
	breaks := map[int]bool{} // the points the cord does not run to from the one before
	base := [3]float64{}
	if p.Base != nil {
		base = *p.Base
	}
	switch strings.ToLower(strings.TrimSpace(s.Shape)) {
	case ShapeSpiral:
		if p.Y1 <= p.Y0 || p.Turns <= 0 || (p.R0 <= 0 && p.R1 <= 0) {
			return nil, fmt.Errorf("spiral: needs y1 above y0, turns and a radius (r0, r1)")
		}
		pts = spiral(base, p.R0, p.Y0, p.R1, p.Y1, p.Turns, spacing)
	case ShapeRings:
		if p.Y1 <= p.Y0 || p.Rings <= 0 || (p.R0 <= 0 && p.R1 <= 0) {
			return nil, fmt.Errorf("rings: needs y1 above y0, rings and a radius (r0, r1)")
		}
		for i := 0; i < p.Rings; i++ {
			f := 0.0
			if p.Rings > 1 {
				f = float64(i) / float64(p.Rings-1)
			}
			r := p.R0 + (p.R1-p.R0)*f
			y := p.Y0 + (p.Y1-p.Y0)*f
			n := max(3, int(math.Round(2*math.Pi*r/spacing)))
			if i > 0 {
				breaks[len(pts)] = true
			}
			for k := 0; k < n; k++ {
				a := 2 * math.Pi * (float64(k) + 0.5*float64(i%2)) / float64(n)
				pts = append(pts, [3]float64{base[0] + r*math.Cos(a), base[1] + y, base[2] + r*math.Sin(a)})
			}
		}
	case ShapeLine:
		if p.From == nil || p.To == nil {
			return nil, fmt.Errorf("line: needs from and to")
		}
		pts = line(*p.From, *p.To, spacing, p.Sag, p.Posts)
	case ShapeBox:
		if p.Min == nil || p.Max == nil {
			return nil, fmt.Errorf("box: needs min and max")
		}
		lo, hi := *p.Min, *p.Max
		c := func(x, y, z int) [3]float64 {
			pick := func(i, v int) float64 {
				if v == 0 {
					return lo[i]
				}
				return hi[i]
			}
			return [3]float64{pick(0, x), pick(1, y), pick(2, z)}
		}
		ring := func(y int) [][2][3]float64 {
			return [][2][3]float64{{c(0, y, 0), c(1, y, 0)}, {c(1, y, 0), c(1, y, 1)}, {c(1, y, 1), c(0, y, 1)}, {c(0, y, 1), c(0, y, 0)}}
		}
		var edges [][2][3]float64
		switch strings.ToLower(strings.TrimSpace(p.Edges)) {
		case "", "top":
			edges = ring(1)
		case "bottom":
			edges = ring(0)
		case "all":
			edges = append(append(ring(0), ring(1)...), [][2][3]float64{{c(0, 0, 0), c(0, 1, 0)}, {c(1, 0, 0), c(1, 1, 0)}, {c(1, 0, 1), c(1, 1, 1)}, {c(0, 0, 1), c(0, 1, 1)}}...)
		default:
			return nil, fmt.Errorf("box: edges %q: top, bottom or all", p.Edges)
		}
		for i, e := range edges {
			if i > 0 && e[0] != edges[i-1][1] {
				// an edge apart from the last: its own end too, the cord broken before it
				breaks[len(pts)] = true
				l := line(e[0], e[1], spacing, 0, 0)
				pts = append(pts, l...)
				continue
			}
			l := line(e[0], e[1], spacing, 0, 0)
			pts = append(pts, l[:len(l)-1]...) // each corner once
		}
	case ShapePoints, ShapePath:
		pts = p.Points
	default:
		return nil, fmt.Errorf("shape %q: one of %s", s.Shape, strings.Join(LightShapes, ", "))
	}
	if len(pts) == 0 {
		return nil, fmt.Errorf("%s lays no bulbs", s.Shape)
	}
	if len(pts) > DecorMaxBulbs {
		return nil, fmt.Errorf("%d bulbs: %d at most (a wider spacing)", len(pts), DecorMaxBulbs)
	}
	nc := len(t.Colors)
	if nc == 0 {
		return nil, fmt.Errorf("template %q has no colours", t.Name)
	}
	rnd := uint32(s.ID)*2654435761 + 12345
	next := func() uint32 {
		rnd ^= rnd << 13
		rnd ^= rnd >> 17
		rnd ^= rnd << 5
		return rnd
	}
	out := make([]Bulb, 0, len(pts)+1)
	last := -1
	for i, q := range pts {
		ci := i % nc
		if strings.EqualFold(t.Pattern, LightRandom) && nc > 1 {
			// never the same colour twice in a row
			ci = int(next() % uint32(nc))
			if ci == last {
				ci = (ci + 1 + int(next()%uint32(nc-1))) % nc
			}
		}
		last = ci
		out = append(out, Bulb{X: float32(q[0]), Y: float32(q[1]), Z: float32(q[2]), Color: uint8(ci), Phase: uint8(next() >> 24), Break: breaks[i]})
	}
	// a chase runs along the string: each bulb an eighth of a cycle behind the
	// one before, so a lit run every eight bulbs travels along it
	if strings.EqualFold(t.Effect, LightChase) {
		for i := range out {
			out[i].Phase = uint8(i * 32 % 256)
		}
	}
	// the alternation: odd and even bulbs half a cycle apart
	if strings.EqualFold(t.Effect, LightAlternate) {
		for i := range out {
			out[i].Phase = uint8(128 * (i % 2))
		}
	}
	return out, nil
}

// DecorTopColor is a string's top bulb's colour (Params.Top), if it names one.
func DecorTopColor(s LightString) (uint32, [3]float32, bool) {
	p, err := ReadLightParams(s.Params)
	if err != nil || strings.TrimSpace(p.Top) == "" || (p.Star == nil && p.Y1 <= p.Y0) {
		return 0, [3]float32{}, false
	}
	c, err := ParseLightColors(p.Top)
	if err != nil || len(c) != 1 {
		return 0, [3]float32{}, false
	}
	if p.Star != nil {
		return c[0], [3]float32{float32(p.Star[0]), float32(p.Star[1]), float32(p.Star[2])}, true
	}
	base := [3]float64{}
	if p.Base != nil {
		base = *p.Base
	}
	return c[0], [3]float32{float32(base[0]), float32(base[1] + p.Y1 + 0.3*max(0.3, p.R1)), float32(base[2])}, true
}

// spiral is a garland round a cone: from radius r0 at height y0 to r1 at y1,
// turns times round, a point every spacing along it (measured along the coil).
func spiral(base [3]float64, r0, y0, r1, y1, turns, spacing float64) [][3]float64 {
	at := func(f float64) [3]float64 {
		r := r0 + (r1-r0)*f
		a := 2 * math.Pi * turns * f
		return [3]float64{base[0] + r*math.Cos(a), base[1] + y0 + (y1-y0)*f, base[2] + r*math.Sin(a)}
	}
	const steps = 4000
	var out [][3]float64
	prev := at(0)
	out = append(out, prev)
	run := 0.0
	for i := 1; i <= steps; i++ {
		q := at(float64(i) / steps)
		run += dist3(prev, q)
		prev = q
		if run >= spacing {
			out = append(out, q)
			run = 0
		}
		if len(out) > DecorMaxBulbs {
			break
		}
	}
	return out
}

// line is a point every spacing from a to b (both ends included), each span
// between posts (every posts along it; 0: the whole line one span) sagging by
// sag at its middle.
func line(a, b [3]float64, spacing, sag, posts float64) [][3]float64 {
	l := dist3(a, b)
	n := max(1, int(math.Round(l/spacing)))
	if n > DecorMaxBulbs {
		n = DecorMaxBulbs + 1
	}
	out := make([][3]float64, 0, n+1)
	for i := 0; i <= n; i++ {
		f := float64(i) / float64(n)
		q := [3]float64{a[0] + (b[0]-a[0])*f, a[1] + (b[1]-a[1])*f, a[2] + (b[2]-a[2])*f}
		if sag != 0 {
			span := l
			if posts > 0 {
				span = posts
			}
			d := math.Mod(f*l, span) / span // 0..1 along its span
			q[1] -= sag * 4 * d * (1 - d)
		}
		out = append(out, q)
	}
	return out
}

func dist3(a, b [3]float64) float64 {
	return math.Sqrt((a[0]-b[0])*(a[0]-b[0]) + (a[1]-b[1])*(a[1]-b[1]) + (a[2]-b[2])*(a[2]-b[2]))
}

// DecorArt is a row of decorart.
type DecorArt struct {
	Name  string // the NPC's name, keyed (NPCClassKey)
	Model uint32
	World int
	Zone  int
	Sink  float32 // drawn this far lower, in the art's own units
}

// DecorArts is every row of decorart with a model, by NPCClassKey of its name.
func (db *SQLiteDB) DecorArts() (map[string]DecorArt, error) {
	if err := db.ensureDecorArtSink(); err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT name, model, world, zone, sink FROM decorart`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]DecorArt{}
	for rows.Next() {
		var a DecorArt
		var model int64
		if err := rows.Scan(&a.Name, &model, &a.World, &a.Zone, &a.Sink); err != nil {
			return nil, err
		}
		a.Name = NPCClassKey(a.Name)
		a.Model = uint32(model)
		if a.Name != "" && a.Model != 0 {
			out[a.Name] = a
		}
	}
	return out, rows.Err()
}

// ensureLightTemplateCord gives a lighttemplates table made before its cord
// columns them (the owner: "for the lights let there look like a cord connecting
// them").
func (db *SQLiteDB) ensureLightTemplateCord() error {
	for _, c := range []struct{ name, def string }{{"cord", "TEXT NOT NULL DEFAULT '1E3F1E'"}, {"cord_width", "REAL NOT NULL DEFAULT 0.06"},
		{"bulb", "TEXT NOT NULL DEFAULT ''"}, {"bulb_scale", "REAL NOT NULL DEFAULT 0.35"}, {"cord_texture", "TEXT NOT NULL DEFAULT ''"},
		{"cord_repeat", "REAL NOT NULL DEFAULT 1"}, {"cord_sag", "REAL NOT NULL DEFAULT 0"}} {
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM pragma_table_info('lighttemplates') WHERE name = ?`, c.name).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			if _, err := db.Exec(`ALTER TABLE lighttemplates ADD COLUMN ` + c.name + ` ` + c.def); err != nil {
				return err
			}
		}
	}
	return nil
}

// ensureDecorArtSink gives a decorart table made before its sink column one.
func (db *SQLiteDB) ensureDecorArtSink() error {
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM pragma_table_info('decorart') WHERE name = 'sink'`).Scan(&n); err != nil || n > 0 {
		return err
	}
	_, err := db.Exec(`ALTER TABLE decorart ADD COLUMN sink REAL NOT NULL DEFAULT 0`)
	return err
}

// GiftBox is one present of a presents string.
type GiftBox struct {
	At     [3]float64 `json:"at"`
	Size   [3]float64 `json:"size"`
	Yaw    float64    `json:"yaw,omitempty"`
	Color  string     `json:"color"`
	Ribbon string     `json:"ribbon,omitempty"`
	Bow    bool       `json:"bow,omitempty"`
	// Paper, when set, is a texture (a decorassets DictID, 0xXXXXXXXX) the box
	// is wrapped in, tinted its colour times Gain (0: 1): a tent's cloth (the
	// owner: "we can use these tents as wrapping paper for presents")
	Paper string  `json:"paper,omitempty"`
	Gain  float64 `json:"gain,omitempty"`
}

// Gift is a present as the world sends it: colours 0xRRGGBB (Ribbon 0 and
// RibbonOn false: none), the yaw in radians.
type Gift struct {
	At, Size [3]float32
	Yaw      float32
	Color    uint32
	Ribbon   uint32
	RibbonOn bool
	Bow      bool
	Paper    uint32
	Gain     float32
}

// DecorMaxGifts is the most presents a string carries.
const DecorMaxGifts = 64

// DecorGifts reads a presents string's boxes, or says what is wrong.
func DecorGifts(s LightString) ([]Gift, error) {
	p, err := ReadLightParams(s.Params)
	if err != nil {
		return nil, err
	}
	if len(p.Boxes) == 0 {
		return nil, fmt.Errorf("presents: no boxes")
	}
	if len(p.Boxes) > DecorMaxGifts {
		return nil, fmt.Errorf("presents: %d boxes, %d at most", len(p.Boxes), DecorMaxGifts)
	}
	out := make([]Gift, 0, len(p.Boxes))
	for i, b := range p.Boxes {
		c, err := ParseLightColors(b.Color)
		if err != nil || len(c) != 1 {
			return nil, fmt.Errorf("presents: box %d's color %q: one RRGGBB", i+1, b.Color)
		}
		g := Gift{Color: c[0], Bow: b.Bow, Yaw: float32(b.Yaw * math.Pi / 180), Gain: 1}
		if p := strings.TrimSpace(b.Paper); p != "" {
			id, err := ParseDictID(p)
			if err != nil || id == 0 {
				return nil, fmt.Errorf("presents: box %d's paper %q: a texture's DictID, 0xXXXXXXXX", i+1, b.Paper)
			}
			g.Paper = id
			if b.Gain < 0 || b.Gain > 8 {
				return nil, fmt.Errorf("presents: box %d's gain %.2f: 0 (1) to 8", i+1, b.Gain)
			}
			if b.Gain > 0 {
				g.Gain = float32(b.Gain)
			}
		}
		if r := strings.TrimSpace(b.Ribbon); r != "" && !strings.EqualFold(r, "none") {
			rc, err := ParseLightColors(r)
			if err != nil || len(rc) != 1 {
				return nil, fmt.Errorf("presents: box %d's ribbon %q: one RRGGBB, or blank", i+1, b.Ribbon)
			}
			g.Ribbon, g.RibbonOn = rc[0], true
		}
		for k := 0; k < 3; k++ {
			if b.Size[k] <= 0 || b.Size[k] > 10 {
				return nil, fmt.Errorf("presents: box %d's size %v: each more than 0, at most 10", i+1, b.Size)
			}
			g.At[k], g.Size[k] = float32(b.At[k]), float32(b.Size[k])
		}
		out = append(out, g)
	}
	return out, nil
}

// ParseDictID reads a DictID written 0xXXXXXXXX (or bare hex, or blank: 0).
func ParseDictID(s string) (uint32, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" {
		return 0, nil
	}
	s = strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X")
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, fmt.Errorf("DictID %q: 0xXXXXXXXX", s)
	}
	return uint32(v), nil
}

// decorassets: art a decoration's look draws that a client need not have
// loaded (christmas5): the textures and models ids (DictIDs, space or comma
// separated) of zone zone (its index in the world's file) of world world,
// which a client drawing decorations reads off its own disc once, whatever
// world it is in (Odus's tents as a present's paper in Freeport). Sent only
// while a string's look or a present names one of its ids.
const decorAssetsSchema = `
	CREATE TABLE IF NOT EXISTS decorassets (
		name   TEXT PRIMARY KEY COLLATE NOCASE,
		world  INTEGER NOT NULL DEFAULT 0,
		zone   INTEGER NOT NULL DEFAULT 0,
		ids    TEXT NOT NULL DEFAULT '',
		note   TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'gm'
	);`

// DecorAsset is a row of decorassets.
type DecorAsset struct {
	Name  string
	World int
	Zone  int
	IDs   []uint32
}

// DecorAssetIDs reads a decorassets row's ids, or says what is wrong.
func DecorAssetIDs(s string) ([]uint32, error) {
	var out []uint32
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == ';' }) {
		id, err := ParseDictID(f)
		if err != nil || id == 0 {
			return nil, fmt.Errorf("ids: %q is not a DictID (0xXXXXXXXX)", f)
		}
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("ids: none (0xXXXXXXXX, space separated)")
	}
	if len(out) > 255 {
		return nil, fmt.Errorf("ids: %d, 255 at most", len(out))
	}
	return out, nil
}

// DecorAssets is every row of decorassets whose ids read, by name.
func (db *SQLiteDB) DecorAssets() ([]DecorAsset, error) {
	if _, err := db.Exec(decorAssetsSchema); err != nil {
		return nil, fmt.Errorf("decorassets: %w", err)
	}
	rows, err := db.Query(`SELECT name, world, zone, ids FROM decorassets ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DecorAsset
	for rows.Next() {
		var a DecorAsset
		var ids string
		if err := rows.Scan(&a.Name, &a.World, &a.Zone, &ids); err != nil {
			return nil, err
		}
		if a.IDs, err = DecorAssetIDs(ids); err != nil || a.World < 0 || a.World > 255 || a.Zone < 0 || a.Zone > 0xFFFF {
			continue
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
