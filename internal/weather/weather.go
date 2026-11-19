// Package weather is the regional weather (board #1549, a GM's rule):
// sandstorms only on sand ground, snow only on snow ground, rain everywhere
// else, nothing on lava, unless a GM calls it.
//
// Where the ground is comes from the game's own terrain profiles: every world
// file's base record (0x8200) carries a grid of VITerrainProfile indices, one
// per 500x500 square (0x8220 header, 0x8240 bytes; CLIENT
// VIWorld::CalcProfileBlend), which cmd/groundbake writes beside the ground
// files as <world>.profiles (profiles.go). The GMs' tables say which climate
// each profile is (weatherprofiles), which zones are something else
// (weatherzones) and how each climate's weather comes (weatherclimates).
//
// Automatic weather is a pure function of the clock (engine.go): nothing is
// saved, a restart changes nothing and both shards agree. A GM's weather
// (#weather, a trigger's weather step, the panel's weather_set) is an
// override held in memory. The package imports neither the database nor the
// entities and never calls out; its Engine has its own lock, taken after the
// World's.
package weather

import (
	"fmt"
	"math"
	"strings"
)

// Kind is a weather's kind, as 0x7E94 numbers it.
type Kind uint8

const (
	Clear Kind = iota
	Rain
	Snow
	Sand
	Whiteout
	kinds // how many
)

var kindNames = [kinds]string{"clear", "rain", "snow", "sand", "whiteout"}

func (k Kind) String() string {
	if k < kinds {
		return kindNames[k]
	}
	return fmt.Sprintf("kind %d", uint8(k))
}

// Title is the kind's name for a line ("Snow").
func (k Kind) Title() string {
	s := k.String()
	return strings.ToUpper(s[:1]) + s[1:]
}

// ParseKind reads a kind as a GM types it ("sandstorm" is sand).
func ParseKind(s string) (Kind, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "clear":
		return Clear, true
	case "rain":
		return Rain, true
	case "snow":
		return Snow, true
	case "sand", "sandstorm":
		return Sand, true
	case "whiteout":
		return Whiteout, true
	}
	return 0, false
}

// The strengths a GM names.
const (
	Light    = 0.15
	Moderate = 0.5
	Heavy    = 1.0
)

// ParseStrength reads light, moderate (medium) or heavy.
func ParseStrength(s string) (float64, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "light":
		return Light, true
	case "moderate", "medium":
		return Moderate, true
	case "heavy":
		return Heavy, true
	}
	return 0, false
}

// StrengthName is the nearest name of a strength.
func StrengthName(s float64) string {
	switch {
	case s >= 0.75:
		return "heavy"
	case s >= 0.325:
		return "moderate"
	}
	return "light"
}

// Climate is what weather a square has of itself.
type Climate string

const (
	Temperate Climate = "temperate" // rain
	Arctic    Climate = "arctic"    // snow, sometimes a whiteout
	Desert    Climate = "desert"    // sand
	None      Climate = "none"      // nothing (lava, homes, dungeons, a world with no profile file)
)

// Climates is every climate, in the order lines list them.
var Climates = []Climate{Temperate, Arctic, Desert, None}

// ParseClimate reads one of the four.
func ParseClimate(s string) (Climate, bool) {
	c := Climate(strings.ToLower(strings.TrimSpace(s)))
	for _, k := range Climates {
		if c == k {
			return c, true
		}
	}
	return "", false
}

// KindOf is the kind a climate's automatic weather is.
func KindOf(c Climate) Kind {
	switch c {
	case Temperate:
		return Rain
	case Arctic:
		return Snow
	case Desert:
		return Sand
	}
	return Clear
}

// ClimateOfKind is the climate whose rule a GM's weather of kind takes its
// fade from (clear: temperate's).
func ClimateOfKind(k Kind) Climate {
	switch k {
	case Snow, Whiteout:
		return Arctic
	case Sand:
		return Desert
	}
	return Temperate
}

// Rule is one weatherclimates row: how a climate's weather comes.
type Rule struct {
	Climate        Climate
	Kind           Kind
	CycleMinutes   int     // one event at most a cycle
	EventChance    float64 // of an event in a cycle
	EventMin       int     // minutes
	EventMax       int     // minutes
	WeightLight    int
	WeightModerate int
	WeightHeavy    int
	WhiteoutChance float64 // of a whiteout in a heavy snow
	WhiteoutMin    int     // seconds it holds
	WhiteoutMax    int
	FadeSeconds    int
}

// Check holds a rule to what the schedule can draw (the panel's row check and
// the loader's).
func (r Rule) Check() error {
	switch {
	case r.Climate != Temperate && r.Climate != Arctic && r.Climate != Desert:
		return fmt.Errorf("climate %q: temperate, arctic or desert (none has no weather of its own)", r.Climate)
	case r.Kind != KindOf(r.Climate):
		return fmt.Errorf("%s's kind is %s, not %s", r.Climate, KindOf(r.Climate), r.Kind)
	case r.CycleMinutes < 10 || r.CycleMinutes > 240:
		return fmt.Errorf("cycle_minutes %d: 10 to 240", r.CycleMinutes)
	case !(r.EventChance >= 0 && r.EventChance <= 1):
		return fmt.Errorf("event_chance %g: 0 to 1", r.EventChance)
	case r.EventMin < 1 || r.EventMin > 120 || r.EventMax < 1 || r.EventMax > 120:
		return fmt.Errorf("event_min and event_max (%d, %d): 1 to 120 minutes", r.EventMin, r.EventMax)
	case r.EventMax < r.EventMin:
		return fmt.Errorf("event_max %d is below event_min %d", r.EventMax, r.EventMin)
	case r.EventMax*60+720 > r.CycleMinutes*60:
		return fmt.Errorf("event_max %d min leaves no room in a %d-minute cycle (12 minutes clear at least)", r.EventMax, r.CycleMinutes)
	case r.WeightLight < 0 || r.WeightModerate < 0 || r.WeightHeavy < 0 ||
		r.WeightLight > 10000 || r.WeightModerate > 10000 || r.WeightHeavy > 10000:
		return fmt.Errorf("weights (%d, %d, %d): 0 to 10000", r.WeightLight, r.WeightModerate, r.WeightHeavy)
	case r.WeightLight+r.WeightModerate+r.WeightHeavy == 0:
		return fmt.Errorf("the weights are all 0")
	case !(r.WhiteoutChance >= 0 && r.WhiteoutChance <= 1):
		return fmt.Errorf("whiteout_chance %g: 0 to 1", r.WhiteoutChance)
	case r.WhiteoutMin < 1 || r.WhiteoutMin > 600 || r.WhiteoutMax < 1 || r.WhiteoutMax > 600:
		return fmt.Errorf("whiteout_min and whiteout_max (%d, %d): 1 to 600 seconds", r.WhiteoutMin, r.WhiteoutMax)
	case r.WhiteoutMax < r.WhiteoutMin:
		return fmt.Errorf("whiteout_max %d is below whiteout_min %d", r.WhiteoutMax, r.WhiteoutMin)
	case r.FadeSeconds < 1 || r.FadeSeconds > 60:
		return fmt.Errorf("fade_seconds %d: 1 to 60", r.FadeSeconds)
	}
	return nil
}

// CheckProfileRow holds a weatherprofiles row to what the engine reads.
func CheckProfileRow(profile int, climate string) error {
	if profile < 0 || profile > 255 {
		return fmt.Errorf("profile %d: 0 to 255", profile)
	}
	if _, ok := ParseClimate(climate); !ok {
		return fmt.Errorf("climate %q: temperate, arctic, desert or none", climate)
	}
	return nil
}

// CheckZoneRow holds a weatherzones row to what the engine reads.
func CheckZoneRow(world, col, row int, climate string) error {
	switch {
	case world < 0 || world > 29:
		return fmt.Errorf("world %d: 0 to 29", world)
	case col < 0 || col > 1000 || row < 0 || row > 1000:
		return fmt.Errorf("col and row (%d, %d): 0 to 1000 (a zone is 2000 units: col = x/2000, row = z/2000)", col, row)
	}
	if _, ok := ParseClimate(climate); !ok {
		return fmt.Errorf("climate %q: temperate, arctic, desert or none", climate)
	}
	return nil
}

// Rules is what the three tables say.
type Rules struct {
	Profiles map[int]Climate  // weatherprofiles: a terrain profile's climate
	Zones    map[Zone]Climate // weatherzones: a GM's word for a whole zone, above the profiles
	Climates map[Climate]Rule // weatherclimates
}

// The geometry: a zone is a 2000x2000 cell of the world, a square 500x500.
const (
	ZoneSize   = 2000
	SquareSize = 500
	perZone    = ZoneSize / SquareSize // squares a side
)

// Zone is a 2000x2000 cell: col = floor(x/2000), row = floor(z/2000).
type Zone struct {
	World, Col, Row int
}

// ID is its name in lines and keys: "w<world>-c<col>-r<row>".
func (z Zone) ID() string { return fmt.Sprintf("w%d-c%d-r%d", z.World, z.Col, z.Row) }

// ZoneAt is the zone of a place.
func ZoneAt(world int, x, z float32) Zone {
	return Zone{world, floorDiv(x, ZoneSize), floorDiv(z, ZoneSize)}
}

// SquareAt is the square of a place: sx = floor(x/500), sz = floor(z/500).
func SquareAt(x, z float32) (sx, sz int) {
	return floorDiv(x, SquareSize), floorDiv(z, SquareSize)
}

// SquareBit is a square's bit in its zone's 16: (sz%4)*4 + (sx%4).
func SquareBit(sx, sz int) uint16 {
	return 1 << uint(mod(sz, perZone)*perZone+mod(sx, perZone))
}

func floorDiv(v float32, by int) int {
	return int(math.Floor(float64(v) / float64(by)))
}

func mod(a, b int) int {
	m := a % b
	if m < 0 {
		m += b
	}
	return m
}

func idiv(a, b int) int {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

// ZoneOfSquare is the zone a square lies in.
func ZoneOfSquare(world, sx, sz int) Zone {
	return Zone{world, idiv(sx, perZone), idiv(sz, perZone)}
}

// RetailByte is the weather byte CLIENT's 0x00C5/0x00C7 take for a kind at a
// strength: VIAtmosphere's lv table {0, .15, .5, 1, .15, .5, 1} and its st
// split make 1-3 rain light/moderate/heavy and 4-6 snow (whatever the
// decomp's WXTYPE names say), 0 clear. A whiteout is heavy snow's byte; sand
// has none (0): only a client that draws 0x7E94 shows it.
func RetailByte(k Kind, s float64) byte {
	var base byte
	switch k {
	case Rain:
		base = 1
	case Snow, Whiteout:
		base = 4
	default:
		return 0
	}
	switch {
	case s >= 0.75:
		return base + 2
	case s >= 0.325:
		return base + 1
	case s >= 0.01:
		return base
	}
	return 0
}
