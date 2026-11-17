package entity

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// decorlights.go — strings of decoration lights (the owner, 2026-11-16: the
// Christmas tree's lights "in different colors", "a template so we can put
// christmas lights on fences buildings etc"; tables lighttemplates and
// lightstrings, database/decorlights.go).
//
// The world holds every string of a season that is on (or of none), its bulbs
// laid, read at the start, when a season turns (handlers/gmtriggers.go's season
// step) and after a GM's edit. A client that draws them answers the offer at its
// world entry (handlers/decorlights.go) and is sent them
// (protocol.OpcodeDecorLights), and again whenever they change: the whole set. A
// string on an NPC's name hangs on every NPC of the name the client draws, so a
// tree's lights go where the tree goes and leave with it (a season's NPCs held out
// of the world while it is off: spawnonly.go). A PS2 never answers and is sent
// none.

// decorLightState is the world's strings and the message that tells them.
type decorLightState struct {
	mu   sync.Mutex
	body []byte // OpcodeDecorLights's body; nil: none at all
	// strings and bulbs, for the log and the GM
	strings, bulbs int
	// art is table decorart by NPC name (keyed): the bodies a client that
	// draws decorations is sent in place of the rows' own
	art atomic.Pointer[map[string]database.DecorArt]
}

// DecorLightsLayout is the newest layout of OpcodeDecorLights: 2 (the walk-ons
// after the papers, and OpcodeRide: npctour.go).
const DecorLightsLayout = 2

// DecorCordBreak is the bit of a bulb's colour byte that breaks the cord
// before it (a ring's first bulb after the ring below, a box's edge apart from
// the last): the colour index is below both.
const DecorCordBreak = 0x40

// DecorCordSag is how far a cord hangs between two bulbs, of their distance.
const DecorCordSag = 0.06

// DecorTopBulb is the bit of a bulb's colour byte that marks a string's top
// (a tree's star, the string's params' top): the colour index is the rest.
const DecorTopBulb = 0x80

// DecorString is a string of lights as the world sends it.
type DecorString struct {
	NPC    string // the key of the NPC name it hangs on ("": in World)
	World  uint8
	Effect uint8
	Period float32
	Size   float32
	Bright float32
	Colors []uint32 // 0xRRGGBB
	Bulbs  []database.Bulb
	// Cord is the wire from bulb to bulb (0xRRGGBB; CordOn false: none, a
	// points string's never), CordWidth its thickness, CordSag how far it
	// hangs between two bulbs (of their distance)
	Cord      uint32
	CordOn    bool
	CordWidth float32
	CordSag   float32
	// the look past the glows (database.LightTemplate's): a model at each
	// bulb (0 none) BulbScale across, the cord's texture (0 a plain wire), a
	// span one repeat
	BulbModel   uint32
	BulbScale   float32
	CordTexture uint32
	CordRepeat  float32
	// Gifts are a presents string's boxes (no bulbs)
	Gifts   []database.Gift
	ID      int64
	Season  string
	Problem string // why it lays nothing ("" none)
}

// BuildDecorStrings lays the bulbs of every string of a season in active (by
// database.SeasonKey; a string of none always), each by its template. A string
// whose template is missing or whose shape lays nothing is returned with its
// Problem and no bulbs.
func BuildDecorStrings(templates map[string]database.LightTemplate, rows []database.LightString, active map[string]bool) []DecorString {
	var out []DecorString
	for _, r := range rows {
		if s := database.SeasonKey(r.Season); s != "" && !active[s] {
			continue
		}
		d := DecorString{ID: r.ID, Season: r.Season, World: uint8(max(0, min(r.World, 255)))}
		if n := strings.TrimSpace(r.NPC); n != "" {
			d.NPC = TintWireName(database.NPCClassKey(n))
		}
		if strings.EqualFold(strings.TrimSpace(r.Shape), database.ShapePresents) {
			gifts, err := database.DecorGifts(r)
			if err != nil {
				d.Problem = err.Error()
			}
			d.Gifts = gifts
			out = append(out, d)
			continue
		}
		t, ok := templates[database.SeasonKey(r.Template)]
		if !ok {
			d.Problem = fmt.Sprintf("no light template %q", r.Template)
			out = append(out, d)
			continue
		}
		bulbs, err := database.DecorBulbs(t, r)
		if err != nil {
			d.Problem = err.Error()
			out = append(out, d)
			continue
		}
		d.Effect = uint8(max(0, database.LightEffectByte(t.Effect)))
		d.Period = float32(t.Period)
		if d.Period <= 0 {
			d.Period = database.DecorDefaultPeriod
		}
		d.Size, d.Bright = float32(t.Size), float32(t.Bright)
		if t.CordOn && t.CordWidth > 0 && !strings.EqualFold(strings.TrimSpace(r.Shape), database.ShapePoints) {
			d.Cord, d.CordOn, d.CordWidth, d.CordSag = t.Cord, true, float32(t.CordWidth), DecorCordSag
			if t.CordSag > 0 {
				d.CordSag = float32(min(t.CordSag, 1))
			}
			d.CordTexture, d.CordRepeat = t.CordTexture, float32(max(t.CordRepeat, 0))
		}
		d.BulbModel, d.BulbScale = t.Bulb, float32(t.BulbScale)
		d.Colors = append([]uint32(nil), t.Colors...)
		d.Bulbs = bulbs
		// a tree's star: a colour of its own, its colour byte's top bit set
		// (DecorTopBulb), which a client draws as a sparkle
		if c, at, ok := database.DecorTopColor(r); ok && len(d.Colors) < DecorTopBulb {
			d.Colors = append(d.Colors, c)
			d.Bulbs = append(d.Bulbs, database.Bulb{X: at[0], Y: at[1], Z: at[2], Color: uint8(len(d.Colors)-1) | DecorTopBulb})
		}
		out = append(out, d)
	}
	return out
}

// EncodeDecorLights is OpcodeDecorLights's body for the strings: those that lay
// bulbs, in order, until database.DecorMaxSent bulbs are in it; then the art
// the decorations' NPCs wear (a u16 count, each a u32 model DictID, a u8 world,
// a u16 zone index: the zone of the world's file that holds the sprite, which
// the client reads to have it, and an f32 sink: drawn that much lower, in the
// art's own units), by model.
func EncodeDecorLights(strs []DecorString, art []database.DecorArt, assets ...database.DecorAsset) (body []byte, sent, bulbs int) {
	b := make([]byte, 2, 64)
	var looks []int // each string sent with a look: its index among those sent
	for _, d := range strs {
		if len(d.Bulbs) == 0 {
			continue
		}
		if bulbs+len(d.Bulbs) > database.DecorMaxSent {
			break
		}
		if d.NPC != "" {
			b = append(b, 1, byte(len(d.NPC)))
			b = append(b, d.NPC...)
		} else {
			b = append(b, 0, d.World)
		}
		b = append(b, d.Effect)
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(d.Period))
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(d.Size))
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(d.Bright))
		cord := uint32(0)
		if d.CordOn {
			cord = d.Cord<<8 | 0xFF
		}
		b = binary.LittleEndian.AppendUint32(b, cord)
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(d.CordWidth))
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(d.CordSag))
		b = append(b, byte(len(d.Colors)))
		for _, c := range d.Colors {
			b = binary.LittleEndian.AppendUint32(b, c<<8|0xFF)
		}
		b = binary.LittleEndian.AppendUint16(b, uint16(len(d.Bulbs)))
		for _, u := range d.Bulbs {
			b = binary.LittleEndian.AppendUint32(b, math.Float32bits(u.X))
			b = binary.LittleEndian.AppendUint32(b, math.Float32bits(u.Y))
			b = binary.LittleEndian.AppendUint32(b, math.Float32bits(u.Z))
			c := u.Color
			if u.Break {
				c |= DecorCordBreak
			}
			b = append(b, c, u.Phase)
		}
		if d.BulbModel != 0 || d.CordTexture != 0 {
			looks = append(looks, sent)
		}
		sent++
		bulbs += len(d.Bulbs)
	}
	binary.LittleEndian.PutUint16(b, uint16(sent))
	b = binary.LittleEndian.AppendUint16(b, uint16(len(art)))
	for _, a := range art {
		b = binary.LittleEndian.AppendUint32(b, a.Model)
		b = append(b, byte(max(0, min(a.World, 255))))
		b = binary.LittleEndian.AppendUint16(b, uint16(max(0, min(a.Zone, 0xFFFF))))
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(a.Sink))
	}
	// then the presents (a u16 count of sets, each its anchor as a string's, a
	// u16 count of boxes and each: f32 x, y, z (its bottom's middle), w, h, d,
	// yaw (radians), u32 colour 0xRRGGBBFF, u32 ribbon (0 none), u8 bow); a
	// client before them reads none
	var sets []DecorString
	for _, d := range strs {
		if len(d.Gifts) > 0 {
			sets = append(sets, d)
		}
	}
	b = binary.LittleEndian.AppendUint16(b, uint16(len(sets)))
	for _, d := range sets {
		if d.NPC != "" {
			b = append(b, 1, byte(len(d.NPC)))
			b = append(b, d.NPC...)
		} else {
			b = append(b, 0, d.World)
		}
		b = binary.LittleEndian.AppendUint16(b, uint16(len(d.Gifts)))
		for _, g := range d.Gifts {
			for _, v := range []float32{g.At[0], g.At[1], g.At[2], g.Size[0], g.Size[1], g.Size[2], g.Yaw} {
				b = binary.LittleEndian.AppendUint32(b, math.Float32bits(v))
			}
			b = binary.LittleEndian.AppendUint32(b, g.Color<<8|0xFF)
			rib := uint32(0)
			if g.RibbonOn {
				rib = g.Ribbon<<8 | 0xFF
			}
			b = binary.LittleEndian.AppendUint32(b, rib)
			bow := byte(0)
			if g.Bow {
				bow = 1
			}
			b = append(b, bow)
		}
	}
	// then the looks (a client before them reads none): the assets a look
	// names (a u16 count, each u8 world, u16 zone, u8 count of u32 DictIDs),
	// the strings' looks (a u16 count, each the u16 index of the string among
	// those sent, u32 bulb model, f32 its scale, u32 cord texture, f32 its
	// repeat) and the presents' papers (a u16 count of sets, as above, each a
	// u16 count of boxes, each u32 paper, f32 its gain)
	used := map[uint32]bool{}
	k := 0
	for _, d := range strs {
		if len(d.Bulbs) == 0 || k >= sent {
			continue
		}
		k++
		used[d.BulbModel], used[d.CordTexture] = true, true
	}
	for _, d := range sets {
		for _, g := range d.Gifts {
			used[g.Paper] = true
		}
	}
	var named []database.DecorAsset
	for _, a := range assets {
		for _, id := range a.IDs {
			if id != 0 && used[id] {
				named = append(named, a)
				break
			}
		}
	}
	b = binary.LittleEndian.AppendUint16(b, uint16(len(named)))
	for _, a := range named {
		b = append(b, byte(a.World))
		b = binary.LittleEndian.AppendUint16(b, uint16(a.Zone))
		ids := a.IDs[:min(len(a.IDs), 255)]
		b = append(b, byte(len(ids)))
		for _, id := range ids {
			b = binary.LittleEndian.AppendUint32(b, id)
		}
	}
	b = binary.LittleEndian.AppendUint16(b, uint16(len(looks)))
	k = 0
	for _, d := range strs {
		if len(d.Bulbs) == 0 || k >= sent {
			continue
		}
		if d.BulbModel != 0 || d.CordTexture != 0 {
			b = binary.LittleEndian.AppendUint16(b, uint16(k))
			b = binary.LittleEndian.AppendUint32(b, d.BulbModel)
			b = binary.LittleEndian.AppendUint32(b, math.Float32bits(d.BulbScale))
			b = binary.LittleEndian.AppendUint32(b, d.CordTexture)
			b = binary.LittleEndian.AppendUint32(b, math.Float32bits(d.CordRepeat))
		}
		k++
	}
	b = binary.LittleEndian.AppendUint16(b, uint16(len(sets)))
	for _, d := range sets {
		b = binary.LittleEndian.AppendUint16(b, uint16(len(d.Gifts)))
		for _, g := range d.Gifts {
			b = binary.LittleEndian.AppendUint32(b, g.Paper)
			gain := g.Gain
			if gain <= 0 {
				gain = 1
			}
			b = binary.LittleEndian.AppendUint32(b, math.Float32bits(gain))
		}
	}
	return b, sent, bulbs
}

// DecorLightsMessage is OpcodeDecorLights with body.
func DecorLightsMessage(body []byte) *protocol.Message {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeDecorLights)
	msg.SetData(append(binary.LittleEndian.AppendUint16(make([]byte, 0, 2+len(body)), uint16(protocol.OpcodeDecorLights)), body...))
	return msg
}

// SetDecorLights sets the strings the world sends and the art its decorations
// wear (table decorart: every row, art the bodies a client that draws them is
// sent; shown, the rows whose zones it is told to read: those of NPCs in the
// world), and sends them to every client that draws them; it reports how many
// it told.
func (w *World) SetDecorLights(strs []DecorString, art map[string]database.DecorArt, shown []database.DecorArt, assets ...database.DecorAsset) int {
	if art == nil {
		art = map[string]database.DecorArt{}
	}
	w.decorLights.art.Store(&art)
	body, sent, bulbs := EncodeDecorLights(strs, shown, assets...)
	body = AppendWalkOns(body, w.WalkOns())
	w.decorLights.mu.Lock()
	w.decorLights.body, w.decorLights.strings, w.decorLights.bulbs = body, sent, bulbs
	w.decorLights.mu.Unlock()
	return w.RefreshDecorLights()
}

// LoadDecorLights reads tables lighttemplates, lightstrings and gmseasons into
// the world: at the start, when a season turns and after a GM's edit. It
// reports the strings sent, their bulbs, the clients told, and each string that
// lays nothing (with why).
func (w *World) LoadDecorLights(db *database.SQLiteDB) (strs, bulbs, told int, problems []string, err error) {
	templates, err := db.LightTemplates()
	if err != nil {
		return 0, 0, 0, nil, fmt.Errorf("lighttemplates: %w", err)
	}
	rows, err := db.LightStrings()
	if err != nil {
		return 0, 0, 0, nil, fmt.Errorf("lightstrings: %w", err)
	}
	active, err := db.ActiveSeasons()
	if err != nil {
		return 0, 0, 0, nil, fmt.Errorf("gmseasons: %w", err)
	}
	all := BuildDecorStrings(templates, rows, active)
	for _, d := range all {
		if d.Problem != "" {
			problems = append(problems, fmt.Sprintf("string %d: %s", d.ID, d.Problem))
		}
	}
	art, err := db.DecorArts()
	if err != nil {
		return 0, 0, 0, nil, fmt.Errorf("decorart: %w", err)
	}
	// the art of the NPCs in the world (not of a season that is off), one a
	// model: what the clients read off their discs
	held, err := db.NPCSeasonHeld()
	if err != nil {
		return 0, 0, 0, nil, fmt.Errorf("npcseason: %w", err)
	}
	var shown []database.DecorArt
	seen := map[uint32]bool{}
	keys := make([]string, 0, len(art))
	for k := range art {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if a := art[k]; !held[k] && !seen[a.Model] {
			seen[a.Model] = true
			shown = append(shown, a)
		}
	}
	assets, err := db.DecorAssets()
	if err != nil {
		return 0, 0, 0, nil, fmt.Errorf("decorassets: %w", err)
	}
	// the NPCs' tours and walk-ons of the seasons that are on (npctour.go),
	// the walk-ons sent with the decorations
	tours, tourProblems, err := db.NPCTours()
	if err != nil {
		return 0, 0, 0, nil, fmt.Errorf("npctours: %w", err)
	}
	problems = append(problems, tourProblems...)
	walkOns, err := db.NPCWalkOns()
	if err != nil {
		return 0, 0, 0, nil, fmt.Errorf("npcwalkon: %w", err)
	}
	w.SetNPCTours(tours, walkOns, active)
	told = w.SetDecorLights(all, art, shown, assets...)
	w.decorLights.mu.Lock()
	strs, bulbs = w.decorLights.strings, w.decorLights.bulbs
	w.decorLights.mu.Unlock()
	return strs, bulbs, told, problems, nil
}

// RefreshDecorLights sends the lights to every session that asked for them
// (with only, to those alone) where they differ from what it was last sent, and
// reports how many it sent them to. A session that never asked (a PS2) is sent
// nothing.
func (w *World) RefreshDecorLights(only ...*session.Session) int {
	if w == nil {
		return 0
	}
	w.decorLights.mu.Lock()
	defer w.decorLights.mu.Unlock()
	body := w.decorLights.body
	if body == nil {
		body = []byte{0, 0}
	}
	empty := w.decorLights.strings == 0
	sent := 0
	send := func(s *session.Session) bool {
		if s == nil || s.DecorLightsLayout() == 0 {
			return true
		}
		if s.TakeDecorLights(string(body), empty) {
			s.Queue().AddWithSequence(DecorLightsMessage(body))
			sent++
		}
		return true
	}
	if len(only) > 0 {
		for _, s := range only {
			send(s)
		}
	} else if w.SessionManager != nil {
		w.SessionManager.ForEach(send)
	}
	if sent > 0 {
		log.Printf("DecorLights: %d strings, %d bulbs to %d clients", w.decorLights.strings, w.decorLights.bulbs, sent)
	}
	return sent
}

// DecorLightSummary says what the lights hold, for the log at the start.
func (w *World) DecorLightSummary() string {
	w.decorLights.mu.Lock()
	defer w.decorLights.mu.Unlock()
	if w.decorLights.strings == 0 {
		return "decoration lights: none lit (lightstrings of a season that is on)"
	}
	return fmt.Sprintf("decoration lights: %d strings, %d bulbs, drawn by the clients that ask", w.decorLights.strings, w.decorLights.bulbs)
}

// decorArtViewLocked shows a decoration (an NPC of table decorart's) as one:
// not selectable to any client (byte 26 bit 0 clear, CLIENT's VIEntity +0x18:
// no Tab, no click, no target box: a tree is not a creature), and, to a client
// that draws decorations, in its art (the model, bytes 28-31); every other
// client is sent the row's own body. Must be called with the lock held.
func (w *World) decorArtViewLocked(sess *session.Session, npc *NPC, buf []byte) {
	if len(buf) < 32 {
		return
	}
	art := w.decorLights.art.Load()
	if art == nil || len(*art) == 0 {
		return
	}
	a, ok := (*art)[database.NPCClassKey(npc.Name)]
	if !ok {
		return
	}
	buf[26] &^= 1
	if sess != nil && sess.DecorLightsLayout() != 0 {
		binary.LittleEndian.PutUint32(buf[28:32], a.Model)
	}
}

// AppendWalkOns is the walk-ons after the papers (layout 2; a client before
// reads none): a u16 count, each the NPC's name (u8 byte count and UTF-8, keyed
// as OpcodeNPCTints keys one), u8 mode (1 a deck: walked into and stood on, 2
// passed through), f32 radius and f32 height (a deck's, across and over its
// feet).
func AppendWalkOns(b []byte, walkOns []database.NPCWalkOn) []byte {
	b = binary.LittleEndian.AppendUint16(b, uint16(len(walkOns)))
	for _, wo := range walkOns {
		name := wo.Name[:min(len(wo.Name), 255)]
		b = append(b, byte(len(name)))
		b = append(b, name...)
		mode := byte(1)
		if wo.Mode == database.WalkOnPass {
			mode = 2
		}
		b = append(b, mode)
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(wo.Radius))
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(wo.Height))
	}
	return b
}
