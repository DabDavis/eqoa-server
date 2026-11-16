package handlers

import (
	"encoding/binary"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
	"github.com/eqoa/server/internal/weather"
)

// The regional weather (#1549; entity/weather.go, internal/weather).
//
// Once the player is in the world the server offers the weather's timelines
// (OpcodeWeather, a u32 1) when the weather is on; a client that draws them
// (vanilla's sandstorms and whiteouts, #1550) answers 1, and 0 to stop. A PS2
// ignores the offer and has its rain and snow from 0x00C5/0x00C7.

// buildWeatherOffer is OpcodeWeather from the server: a u32, 1.
func buildWeatherOffer() []byte {
	return build(protocol.OpcodeWeather, 6, func(w *buffer.Writer) {
		w.WriteUint32(1)
	})
}

// SendWeatherOffer offers the session's client the weather's timelines.
func SendWeatherOffer(sess *session.Session) {
	queueReliable(sess, protocol.OpcodeWeather, buildWeatherOffer())
}

// WeatherHandler takes a client's OpcodeWeather: whether it draws the
// weather's timelines.
type WeatherHandler struct {
	world *entity.World
}

// NewWeatherHandler creates the weather handler.
func NewWeatherHandler(w *entity.World) *WeatherHandler { return &WeatherHandler{world: w} }

// RegisterHandlers registers the weather handler.
func (h *WeatherHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeWeather, h.HandleWeather)
}

// HandleWeather makes the client's answer its player's: a u32, nonzero to be
// sent the timelines, 0 to stop. Nothing is answered.
func (h *WeatherHandler) HandleWeather(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 4 || h.world == nil {
		log.Printf("Weather: short (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	on := binary.LittleEndian.Uint32(data[:4]) != 0
	if !h.world.SetWeatherDraws(sess, on) {
		log.Printf("Weather: %s has no player in the world", sess.RemoteAddr())
		return nil
	}
	log.Printf("Weather: %s %s", sess.RemoteAddr(), map[bool]string{true: "draws it", false: "stopped"}[on])
	return nil
}

// weatherUsage is #weather's line in #help (gmHelpLines) and its usage.
const weatherUsage = "#weather [status|list|auto|end <id>|reload|<kind> [strength] [minutes] [here|near N|world] [say words]]"

// parseWhereArgs reads [here|near N|world] at args[i]: the place's reach and
// the next argument's index.
func parseWhereArgs(args []string, i int) (weather.Where, int, error) {
	if i >= len(args) {
		return weather.Where{Kind: "here"}, i, nil
	}
	switch strings.ToLower(args[i]) {
	case "here", "world":
		w, err := weather.ParseWhere(args[i], 0)
		return w, i + 1, err
	case "near":
		n := 1
		if i+1 < len(args) {
			if v, err := strconv.Atoi(args[i+1]); err == nil {
				n = v
				i++
			}
		}
		w, err := weather.ParseWhere("near", n)
		return w, i + 1, err
	}
	return weather.Where{Kind: "here"}, i, nil
}

// zoneList is "N zones (w0-c2-r2 ...)": the first few ids.
func zoneList(zs []weather.Zone) string {
	ids := make([]string, 0, 4)
	for i, z := range zs {
		if i == 3 {
			ids = append(ids, "...")
			break
		}
		ids = append(ids, z.ID())
	}
	n := fmt.Sprintf("%d zones", len(zs))
	if len(zs) == 1 {
		n = "1 zone"
	}
	return n + " (" + strings.Join(ids, " ") + ")"
}

// weatherCommand is #weather (see weatherUsage). The place is the GM's own.
func (h *ChatHandler) weatherCommand(sess *session.Session, p *entity.Player, args []string) {
	gw := h.gameWorld
	if gw == nil || p == nil {
		return
	}
	if gw.Weather == nil {
		h.gmReply(sess, "#weather: the regional weather is off on this server (world.weather: false)")
		return
	}
	world, x, z := int(p.World), p.X, p.Z
	sub := "status"
	if len(args) > 0 {
		sub = strings.ToLower(args[0])
	}
	now := time.Now()
	switch sub {
	case "status":
		h.weatherStatus(sess, p, now)
	case "list":
		os := gw.Weather.Overrides(now)
		if len(os) == 0 {
			h.gmReply(sess, "#weather: no GM weather in force")
			return
		}
		for _, o := range os {
			h.gmReply(sess, fmt.Sprintf("#%d w%d: %s (%s) on %d zones until %s, by %s", o.ID, o.World, o.Kind.Title(),
				weather.StrengthName(o.Strength), len(o.Zones), o.Until.Local().Format("15:04"), o.By))
		}
	case "auto":
		where, _, err := parseWhereArgs(args, 1)
		if err != nil {
			h.gmReply(sess, "#weather auto: "+err.Error())
			return
		}
		n := gw.EndWeather(world, x, z, where)
		log.Printf("GM: %s weather auto %s at w%d (%.0f, %.0f): %d zones", p.Name, where, world, x, z, n)
		h.gmReply(sess, fmt.Sprintf("GM weather ended on %d zones (%s): each clears over its fade, then the automatic weather comes back", n, where))
	case "end":
		if len(args) < 2 {
			h.gmReply(sess, "Usage: #weather end <id> (#weather list names them)")
			return
		}
		id, err := strconv.Atoi(strings.TrimPrefix(args[1], "#"))
		if err != nil || !gw.EndWeatherOverride(id) {
			h.gmReply(sess, fmt.Sprintf("#weather end: no GM weather #%s in force", args[1]))
			return
		}
		log.Printf("GM: %s weather end #%d", p.Name, id)
		h.gmReply(sess, fmt.Sprintf("GM weather #%d ended: it clears over its fade", id))
	case "reload":
		pr, zn, cl, err := gw.LoadWeather(h.camps.db)
		if err != nil {
			h.gmReply(sess, "#weather reload: "+err.Error())
			return
		}
		log.Printf("GM: %s weather reload: %d profiles, %d zones, %d climates", p.Name, pr, zn, cl)
		h.gmReply(sess, fmt.Sprintf("Weather tables read: %d profiles, %d zones, %d climates", pr, zn, cl))
	default:
		h.weatherCall(sess, p, args, now)
	}
}

// weatherCall is #weather <kind> [strength] [minutes] [here|near N|world] [say words].
func (h *ChatHandler) weatherCall(sess *session.Session, p *entity.Player, args []string, now time.Time) {
	kind, ok := weather.ParseKind(args[0])
	if !ok {
		h.gmReply(sess, "Usage: "+weatherUsage)
		return
	}
	strength, minutes, i := weather.Heavy, 10, 1
	if i < len(args) {
		if s, ok := weather.ParseStrength(args[i]); ok {
			strength = s
			i++
		}
	}
	if i < len(args) {
		if m, err := strconv.Atoi(args[i]); err == nil {
			if m < 2 || m > 120 {
				h.gmReply(sess, "#weather: minutes are 2 to 120")
				return
			}
			minutes = m
			i++
		}
	}
	where, i, err := parseWhereArgs(args, i)
	if err != nil {
		h.gmReply(sess, "#weather: "+err.Error())
		return
	}
	text := ""
	if i < len(args) {
		if strings.ToLower(args[i]) != "say" {
			h.gmReply(sess, "Usage: "+weatherUsage)
			return
		}
		text = strings.Join(args[i+1:], " ")
	}
	zones, until, id, err := h.gameWorld.CallWeather(int(p.World), p.X, p.Z, where, kind, strength, minutes, text, p.Name, 0)
	if err != nil {
		h.gmReply(sess, "#weather: "+err.Error())
		return
	}
	log.Printf("GM: %s weather %s %s %d min %s at w%d (%.0f, %.0f): #%d on %d zones", p.Name, kind, weather.StrengthName(strength),
		minutes, where, p.World, p.X, p.Z, id, len(zones))
	st := " (" + weather.StrengthName(strength) + ")"
	if kind == weather.Clear {
		st = ""
	}
	h.gmReply(sess, fmt.Sprintf("%s%s for %d min on %s, until %s (#%d).", kind.Title(), st, minutes, zoneList(zones),
		until.Local().Format("15:04"), id))
}

// weatherStatus is #weather's own: the GM's zone and square, what shows now
// and until when, and the next automatic event there.
func (h *ChatHandler) weatherStatus(sess *session.Session, p *entity.Player, now time.Time) {
	e := h.gameWorld.Weather
	world := int(p.World)
	z := weather.ZoneAt(world, p.X, p.Z)
	sx, sz := weather.SquareAt(p.X, p.Z)
	prof, has, climate := e.Square(world, sx, sz)
	ps := "no profile file"
	if has {
		ps = fmt.Sprintf("profile %d", prof)
	}
	if p.Instance != 0 {
		h.gmReply(sess, fmt.Sprintf("%s, square (%d, %d): an instance (a home, a dungeon): no weather", z.ID(), sx, sz))
		return
	}
	h.gmReply(sess, fmt.Sprintf("%s, square (%d, %d): %s, %s", z.ID(), sx, sz, ps, climate))
	cl := e.ZoneClimates(z)
	if len(cl) > 0 {
		var parts []string
		for c, m := range cl {
			parts = append(parts, fmt.Sprintf("%s %d", c, popcount(m)))
		}
		sort.Strings(parts)
		h.gmReply(sess, "Zone's squares: "+strings.Join(parts, ", "))
	}
	if tl, ok := e.At(world, p.X, p.Z, now); ok {
		k, s := tl.Showing(now)
		mode := "automatic"
		if tl.Override != 0 {
			mode = fmt.Sprintf("GM weather #%d by %s", tl.Override, tl.By)
		}
		if s > 0 {
			h.gmReply(sess, fmt.Sprintf("Now: %s %.2f (%s), until %s; %s", k.Title(), s, weather.StrengthName(s),
				tl.End().Local().Format("15:04:05"), mode))
		} else if now.Before(tl.Start()) {
			h.gmReply(sess, fmt.Sprintf("Now: clear; %s from %s", mode, tl.Start().Local().Format("15:04:05")))
		} else {
			h.gmReply(sess, "Now: clear; "+mode)
		}
	} else {
		h.gmReply(sess, "Now: clear (no weather of its own)")
	}
	if nx, ok := e.Next(world, sx, sz, now); ok {
		k, s := weather.Clear, 0.0
		for _, ph := range nx.Phases {
			if ph.Strength > s {
				k, s = ph.Kind, ph.Strength
			}
		}
		h.gmReply(sess, fmt.Sprintf("Next automatic: %s (%s) from %s to %s", k.Title(), weather.StrengthName(s),
			nx.Start().Local().Format("15:04:05"), nx.End().Local().Format("15:04:05")))
	}
}

func popcount(m uint16) int {
	n := 0
	for ; m != 0; m &= m - 1 {
		n++
	}
	return n
}
