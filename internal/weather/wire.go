package weather

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"time"
)

// 0x7E94 (protocol.OpcodeWeatherData), the timelines a client that draws the
// weather itself is sent (vanilla, #1550), little-endian:
//
//	u8 zones (<= 4: the player's, then any other within 100 units of it), each:
//	  u8 world, u16 col, u16 row, u8 timelines (0-3), each:
//	    u16 squares (bit (sz%4)*4 + (sx%4); a GM's 0xFFFF), u32 revision, u32 seed,
//	    f32 wind_x, f32 wind_z, u8 n (2-12), n phases of
//	      {i32 at_ms relative to this message (negative = past), u8 kind, u8 strength (round(s*255))}
//
// A square in no timeline is clear.

const (
	MaxDataZones     = 4
	MaxDataTimelines = 3
	MaxPhases        = 12
	NearZoneUnits    = 100 // another zone is sent when the player is this near it
)

// ZoneData is one zone of a 0x7E94.
type ZoneData struct {
	Zone      Zone
	Timelines []Timeline
}

// ZonesNear is the zone of a place, then each other within NearZoneUnits of
// it (at most three more).
func ZonesNear(world int, x, z float32) []Zone {
	own := ZoneAt(world, x, z)
	out := []Zone{own}
	for dr := -1; dr <= 1; dr++ {
		for dc := -1; dc <= 1; dc++ {
			if dc == 0 && dr == 0 {
				continue
			}
			c, r := own.Col+dc, own.Row+dr
			if c < 0 || r < 0 {
				continue
			}
			x0, z0 := float64(c*ZoneSize), float64(r*ZoneSize)
			ddx := math.Max(0, math.Max(x0-float64(x), float64(x)-(x0+ZoneSize)))
			ddz := math.Max(0, math.Max(z0-float64(z), float64(z)-(z0+ZoneSize)))
			if math.Hypot(ddx, ddz) <= NearZoneUnits && len(out) < MaxDataZones {
				out = append(out, Zone{world, c, r})
			}
		}
	}
	return out
}

// DataKey is what a 0x7E94 of these zones says, for sending it only when it
// changes: the zones' ids and their timelines' revisions.
func DataKey(zones []ZoneData) string {
	var b strings.Builder
	for _, z := range zones {
		b.WriteString(z.Zone.ID())
		for _, t := range z.Timelines {
			fmt.Fprintf(&b, ":%08x/%04x", t.Revision, t.Mask)
		}
		b.WriteByte(';')
	}
	return b.String()
}

// EncodeData is 0x7E94's body for zones at now (each phase's time relative to
// it). Zones past the fourth, timelines past the third, a zone off the u16
// grid and a timeline of fewer than 2 or more than 12 phases are left out.
func EncodeData(zones []ZoneData, now time.Time) []byte {
	le := binary.LittleEndian
	out := []byte{0}
	n := 0
	for _, z := range zones {
		if n == MaxDataZones {
			break
		}
		if z.Zone.World < 0 || z.Zone.World > 255 || z.Zone.Col < 0 || z.Zone.Col > 0xFFFF || z.Zone.Row < 0 || z.Zone.Row > 0xFFFF {
			continue
		}
		n++
		out = append(out, byte(z.Zone.World))
		out = le.AppendUint16(out, uint16(z.Zone.Col))
		out = le.AppendUint16(out, uint16(z.Zone.Row))
		at := len(out)
		out = append(out, 0)
		m := 0
		for _, t := range z.Timelines {
			if m == MaxDataTimelines || len(t.Phases) < 2 || len(t.Phases) > MaxPhases {
				continue
			}
			m++
			out = le.AppendUint16(out, t.Mask)
			out = le.AppendUint32(out, t.Revision)
			out = le.AppendUint32(out, t.Seed)
			out = le.AppendUint32(out, math.Float32bits(t.WindX))
			out = le.AppendUint32(out, math.Float32bits(t.WindZ))
			out = append(out, byte(len(t.Phases)))
			for _, p := range t.Phases {
				ms := p.At.Sub(now).Milliseconds()
				ms = max(math.MinInt32, min(math.MaxInt32, ms))
				out = le.AppendUint32(out, uint32(int32(ms)))
				out = append(out, byte(p.Kind))
				out = append(out, byte(math.Round(max(0, min(1, p.Strength))*255)))
			}
		}
		out[at] = byte(m)
	}
	out[0] = byte(n)
	return out
}
