package entity

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
	"github.com/eqoa/server/internal/weather"
)

// The regional weather (#1549, internal/weather): each player is sent the
// weather of the 500x500 square it stands on, once a second when it changes.
//
// Every client is sent the retail byte (0x00C5, 0x00C7): CLIENT's case 0x00C5
// calls VIClient::SetWeather(w, 0), a change FADED in (VIAtmosphere::SetWeather
// sets its blend to 1e-5), and 0x00C7 SetWeather(w, 1), at ONCE (the blend at
// 1); 1-3 are rain light/moderate/heavy and 4-6 snow (the atmosphere's lv
// table {0, .15, .5, 1, .15, .5, 1} and its st split, whatever the decomp's
// WXTYPE names say), 0 and 7 up clear. The byte is the target of the phase
// being faded to or held at now (weather.Timeline.Byte): an event is three
// 0x00C5s (light at its start, its strength a fade later, 0 a fade before its
// end); a sandstorm is 0 (no retail weather draws sand). After a MemoryDump
// (HandleCharacterInWorld's answer) or a world loaded (0x0049) the byte goes
// again as 0x00C7 (WeatherResend), as on a first entry.
//
// A client that draws the weather itself (vanilla, #1550) answers the offer
// (OpcodeWeather 0x7E90, a u32 1) with 1, and is sent the timelines of its
// zone and of any other within 100 units of it (OpcodeWeatherData 0x7E91,
// weather.EncodeData) on entry, a change of zone and a change of revision.
//
// A player in an instance (a home, a dungeon: Instance != 0) has no weather.
// world.weather false in the config: no engine, nothing sent.

// LoadWeather reads the three tables into the engine (the start, #weather
// reload, the GM panel's edits): the rows read and each bad row's problem
// logged. An error with no engine (world.weather false).
func (w *World) LoadWeather(db *database.SQLiteDB) (profiles, zones, climates int, err error) {
	if w.Weather == nil {
		return 0, 0, 0, fmt.Errorf("the regional weather is off (world.weather)")
	}
	if db == nil {
		return 0, 0, 0, fmt.Errorf("no database")
	}
	r, bad, err := db.WeatherRules()
	if err != nil {
		return 0, 0, 0, err
	}
	for _, b := range bad {
		log.Printf("Weather: %v (left out)", b)
	}
	w.Weather.SetRules(r)
	return len(r.Profiles), len(r.Zones), len(r.Climates), nil
}

// SetWeatherDraws makes sess's player's client draw the weather's timelines
// (on: 0x7E91 from the next pass) or stop. False when sess has no player.
func (w *World) SetWeatherDraws(sess *session.Session, on bool) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return false
	}
	p.WeatherDraws = on
	p.weatherKey = ""
	return true
}

// WeatherResend makes the next pass send sess's player its weather again in
// full: the retail byte as 0x00C7 (at once) and the timelines.
func (w *World) WeatherResend(sess *session.Session) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if p := w.sessionPlayerLocked(sess); p != nil {
		p.weatherSent, p.weatherKey = false, ""
	}
}

// weatherOfLocked is a player's retail byte and its 0x7E91 zones at now.
func (w *World) weatherOfLocked(p *Player, now time.Time, zones bool) (byte, []weather.ZoneData) {
	if p.Instance != 0 {
		return 0, nil
	}
	var b byte
	if tl, ok := w.Weather.At(int(p.World), p.X, p.Z, now); ok {
		b = tl.Byte(now)
	}
	if !zones {
		return b, nil
	}
	var out []weather.ZoneData
	for _, z := range weather.ZonesNear(int(p.World), p.X, p.Z) {
		out = append(out, weather.ZoneData{Zone: z, Timelines: w.Weather.Zone(z, now)})
	}
	return b, out
}

// sendWeatherByte sends the retail byte: 0x00C7 at once, else 0x00C5 faded.
func sendWeatherByte(s *session.Session, b byte, instant bool) {
	op := protocol.OpcodeWeatherControl
	if instant {
		op = protocol.OpcodeSkyboxWeather
	}
	sendGroupMsg(s, op, func(wr *buffer.Writer) { wr.WriteByte(b) })
}

// weatherSystem sends each player in the world its weather when it changed:
// once a second. Must be called with the lock held.
func (w *World) weatherSystem(now time.Time) {
	if w.Weather == nil || w.SessionManager == nil || w.TickCount%10 != 0 {
		return
	}
	w.SessionManager.ForEach(func(s *session.Session) bool {
		p := w.sessionPlayerLocked(s)
		if p == nil || !s.CharacterInWorld() || s.StateChannelsHeld() {
			return true
		}
		b, zones := w.weatherOfLocked(p, now, p.WeatherDraws)
		switch {
		case !p.weatherSent:
			sendWeatherByte(s, b, true)
		case b != p.weatherByte:
			sendWeatherByte(s, b, false)
		}
		p.weatherSent, p.weatherByte = true, b
		if p.WeatherDraws {
			if key := "z" + weather.DataKey(zones); key != p.weatherKey {
				p.weatherKey = key
				body := weather.EncodeData(zones, now)
				sendGroupMsg(s, protocol.OpcodeWeatherData, func(wr *buffer.Writer) { wr.WriteBytes(body) })
			}
		}
		return true
	})
}

// gmWeatherRGB is the GMs' yellow a GM's weather's words come in.
var gmWeatherRGB = [3]byte{0xFF, 0xE0, 0x40}

// tellZonesLocked says text (%p the player's name, as a trigger's) to every
// player in the world standing in zones. Must be called with the lock held.
func (w *World) tellZonesLocked(world int, zones []weather.Zone, text string) {
	if strings.TrimSpace(text) == "" || w.SessionManager == nil {
		return
	}
	in := map[weather.Zone]bool{}
	for _, z := range zones {
		in[z] = true
	}
	w.SessionManager.ForEach(func(s *session.Session) bool {
		p := w.sessionPlayerLocked(s)
		if p == nil || !s.CharacterInWorld() || p.Instance != 0 || int(p.World) != world || !in[weather.ZoneAt(world, p.X, p.Z)] {
			return true
		}
		w.sendColoredLocked(s, strings.ReplaceAll(text, "%p", p.Name), gmWeatherRGB)
		return true
	})
}

// CallWeather is a GM's weather (weather.Engine.Call) on the zones where
// takes round a place, with text said to the players standing in them: the
// zones and its end.
func (w *World) CallWeather(world int, x, z float32, where weather.Where, kind weather.Kind, strength float64, minutes int,
	text, by string, run int) ([]weather.Zone, time.Time, int, error) {
	if w.Weather == nil {
		return nil, time.Time{}, 0, fmt.Errorf("the regional weather is off (world.weather)")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	o, err := w.Weather.Call(world, x, z, where, kind, strength, minutes, by, run, time.Now())
	if err != nil {
		return nil, time.Time{}, 0, err
	}
	w.tellZonesLocked(world, o.Zones, text)
	return o.Zones, o.Until, o.ID, nil
}

// EndWeather ends the GMs' weathers on the zones where takes round a place:
// how many zones.
func (w *World) EndWeather(world int, x, z float32, where weather.Where) int {
	if w.Weather == nil {
		return 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.Weather.End(world, x, z, where, time.Now())
}

// EndWeatherOverride ends one GM weather by its id.
func (w *World) EndWeatherOverride(id int) bool {
	if w.Weather == nil {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.Weather.EndID(id, time.Now())
}

// EndWeatherRun ends the GM weathers a trigger run called: how many zones.
func (w *World) EndWeatherRun(run int) int {
	if w.Weather == nil {
		return 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.Weather.EndRun(run, time.Now())
}
