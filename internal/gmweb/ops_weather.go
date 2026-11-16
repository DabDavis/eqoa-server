package gmweb

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/eqoa/server/internal/weather"
)

// ops_weather.go — the regional weather (#1549, internal/weather,
// entity/weather.go): weather_status reads it (the climates, the profile map,
// the GMs' weathers, the zones with weather now); weather_set calls a GM's
// weather and weather_end ends one, audited as live actions. A restart drops
// a GM's weather; the automatic weather is the clock's.

// liveWeather reads the weather's tables again (an edit of weatherprofiles,
// weatherzones or weatherclimates; the change is committed).
func (s *Server) liveWeather() string {
	if s.d.World == nil || s.d.DB == nil {
		return "no world is running: at the next start"
	}
	if s.d.World.Weather == nil {
		return "the regional weather is off on this server (world.weather: false)"
	}
	if _, _, _, err := s.d.World.LoadWeather(s.d.DB); err != nil {
		return "not applied to the world: " + err.Error()
	}
	return "At once: the next weather tick uses it"
}

// checkWeatherRow holds a row of the weather's tables to what the engine reads
// (weather.CheckProfileRow, CheckZoneRow, Rule.Check). A column the row
// leaves out takes the table's default.
func checkWeatherRow(table string, row map[string]any) error {
	whole := func(col string, def int) (int, error) {
		v, ok := row[col]
		if !ok || v == nil {
			return def, nil
		}
		f, fok := asFloat(v)
		if !fok || f != math.Trunc(f) || math.Abs(f) > 1e6 {
			return 0, badRequest("%s is a whole number", col)
		}
		return int(f), nil
	}
	real := func(col string, def float64) (float64, error) {
		v, ok := row[col]
		if !ok || v == nil {
			return def, nil
		}
		f, fok := asFloat(v)
		if !fok || math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, badRequest("%s is a number", col)
		}
		return f, nil
	}
	word := func(col, def string) string {
		if v, ok := row[col]; ok && v != nil {
			return strings.TrimSpace(fmt.Sprint(v))
		}
		return def
	}
	switch table {
	case "weatherprofiles":
		p, err := whole("profile", -1)
		if err != nil {
			return err
		}
		if err := weather.CheckProfileRow(p, word("climate", "temperate")); err != nil {
			return badRequest("%v", err)
		}
	case "weatherzones":
		w, err := whole("world", -1)
		if err != nil {
			return err
		}
		c, err := whole("col", -1)
		if err != nil {
			return err
		}
		r, err := whole("row", -1)
		if err != nil {
			return err
		}
		if err := weather.CheckZoneRow(w, c, r, word("climate", "none")); err != nil {
			return badRequest("%v", err)
		}
	case "weatherclimates":
		var x weather.Rule
		x.Climate = weather.Climate(strings.ToLower(word("climate", "")))
		k, ok := weather.ParseKind(word("kind", ""))
		if !ok || k == weather.Clear || k == weather.Whiteout {
			return badRequest("kind is rain (temperate), snow (arctic) or sand (desert)")
		}
		x.Kind = k
		ints := []struct {
			col string
			def int
			dst *int
		}{{"cycle_minutes", 40, &x.CycleMinutes}, {"event_min", 0, &x.EventMin}, {"event_max", 0, &x.EventMax},
			{"weight_light", 60, &x.WeightLight}, {"weight_moderate", 30, &x.WeightModerate}, {"weight_heavy", 10, &x.WeightHeavy},
			{"whiteout_min", 30, &x.WhiteoutMin}, {"whiteout_max", 60, &x.WhiteoutMax}, {"fade_seconds", 30, &x.FadeSeconds}}
		for _, c := range ints {
			v, err := whole(c.col, c.def)
			if err != nil {
				return err
			}
			*c.dst = v
		}
		var err error
		if x.EventChance, err = real("event_chance", 1); err != nil {
			return err
		}
		if x.WhiteoutChance, err = real("whiteout_chance", 0); err != nil {
			return err
		}
		if err := x.Check(); err != nil {
			return badRequest("%v", err)
		}
	}
	return nil
}

// weatherPlace is where a weather op takes effect.
type weatherPlace struct {
	World *int     `json:"world"`
	X     *float64 `json:"x"`
	Z     *float64 `json:"z"`
	Where string   `json:"where"`
	Near  int      `json:"near"`
}

var weatherPlaceProps = map[string]any{
	"world": integer("the world: 0 Tunaria, 1 the Rathe, 2 Odus, 3 Lavastorm, 4 the Plane of Sky, 5 Secrets, 10+ the server's own"),
	"x":     number("the place's x"),
	"z":     number("the place's z"),
	"where": strEnum("here: the place's zone (2000x2000); near: the zones within near cols and rows of it; world: every zone of its world with weather of its own, and its own", "here", "near", "world"),
	"near":  integer("for where near: 1 to 5 zones round"),
}

func (p weatherPlace) resolve() (int, float32, float32, weather.Where, error) {
	if p.World == nil || p.X == nil || p.Z == nil {
		return 0, 0, 0, weather.Where{}, badRequest("give world, x and z")
	}
	if *p.World < 0 || *p.World > 255 || math.IsNaN(*p.X) || math.IsNaN(*p.Z) || math.Abs(*p.X) > 1e6 || math.Abs(*p.Z) > 1e6 {
		return 0, 0, 0, weather.Where{}, badRequest("world is 0-255, x and z within 1000000")
	}
	w, err := weather.ParseWhere(p.Where, p.Near)
	if err != nil {
		return 0, 0, 0, weather.Where{}, badRequest("%v", err)
	}
	return *p.World, float32(*p.X), float32(*p.Z), w, nil
}

func zoneIDs(zs []weather.Zone) []string {
	out := make([]string, len(zs))
	for i, z := range zs {
		out[i] = z.ID()
	}
	return out
}

func overrideOut(o weather.Override) map[string]any {
	return map[string]any{"id": o.ID, "world": o.World, "zones": len(o.Zones), "zone_ids": zoneIDs(o.Zones), "kind": o.Kind.String(),
		"strength": weather.StrengthName(o.Strength), "from": o.From.UTC().Format(time.RFC3339), "until": o.Until.UTC().Format(time.RFC3339),
		"by": o.By, "run": o.Run, "ended": o.Ended}
}

func (s *Server) registerWeatherOps() {
	engine := func(c *opCtx) (*weather.Engine, error) {
		if c.s.d.World == nil {
			return nil, badRequest("no world is running")
		}
		if c.s.d.World.Weather == nil {
			return nil, badRequest("the regional weather is off on this server (world.weather: false)")
		}
		return c.s.d.World.Weather, nil
	}
	s.addOp(&Op{
		Name: "weather_status", Title: "The regional weather", Scope: scopeRead,
		Description: "The regional weather (#1549): the climates' rules (weatherclimates), the terrain profiles' climates (weatherprofiles), " +
			"the GMs' zones (weatherzones), every GM weather in force, and per world the zones with weather now (id, its squares' " +
			"climates, the kind and strength showing, automatic or a GM's, until when). With world, x and z: that square's profile, " +
			"climate, what shows there and its next automatic event. Sandstorms show only on clients that draw them (vanilla); a PS2 " +
			"shows rain and snow.",
		Input: obj(map[string]any{"world": integer("one world only (and the place's, with x and z)"), "x": number("a place's x"), "z": number("a place's z")}),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				World *int     `json:"world"`
				X     *float64 `json:"x"`
				Z     *float64 `json:"z"`
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			e, err := engine(c)
			if err != nil {
				return nil, err
			}
			now := time.Now()
			r := e.Rules()
			climates := map[string]any{}
			for cl, x := range r.Climates {
				climates[string(cl)] = map[string]any{"kind": x.Kind.String(), "cycle_minutes": x.CycleMinutes, "event_chance": x.EventChance,
					"event_min": x.EventMin, "event_max": x.EventMax, "weights": []int{x.WeightLight, x.WeightModerate, x.WeightHeavy},
					"whiteout_chance": x.WhiteoutChance, "whiteout_seconds": []int{x.WhiteoutMin, x.WhiteoutMax}, "fade_seconds": x.FadeSeconds}
			}
			profiles := map[string]string{}
			for p, cl := range r.Profiles {
				profiles[fmt.Sprint(p)] = string(cl)
			}
			zrows := map[string]string{}
			for z, cl := range r.Zones {
				zrows[z.ID()] = string(cl)
			}
			var ovs []map[string]any
			for _, o := range e.Overrides(now) {
				ovs = append(ovs, overrideOut(o))
			}
			worlds := e.Worlds()
			if req.World != nil {
				worlds = []int{*req.World}
			}
			byWorld := map[string]any{}
			for _, w := range worlds {
				var zs []map[string]any
				for _, zn := range e.WeatherNow(w, now) {
					cls := map[string]int{}
					for cl, m := range zn.Climates {
						n := 0
						for ; m != 0; m &= m - 1 {
							n++
						}
						cls[string(cl)] = n
					}
					mode := "auto"
					if zn.Override != 0 {
						mode = fmt.Sprintf("override #%d by %s", zn.Override, zn.By)
					}
					zs = append(zs, map[string]any{"zone": zn.Zone.ID(), "climates": cls, "kind": zn.Kind.String(),
						"strength": math.Round(zn.Strength*100) / 100, "mode": mode, "until": zn.Until.UTC().Format(time.RFC3339)})
				}
				byWorld[fmt.Sprint(w)] = zs
			}
			out := map[string]any{"now": now.UTC().Format(time.RFC3339), "climates": climates, "profiles": profiles,
				"zones": zrows, "overrides": ovs, "weather_now": byWorld, "worlds_with_ground": e.Worlds()}
			if req.World != nil && req.X != nil && req.Z != nil {
				x, z := float32(*req.X), float32(*req.Z)
				sx, sz := weather.SquareAt(x, z)
				prof, has, cl := e.Square(*req.World, sx, sz)
				sq := map[string]any{"zone": weather.ZoneAt(*req.World, x, z).ID(), "square": []int{sx, sz}, "climate": string(cl)}
				if has {
					sq["profile"] = prof
				}
				if tl, ok := e.At(*req.World, x, z, now); ok {
					k, st := tl.Showing(now)
					sq["showing"] = map[string]any{"kind": k.String(), "strength": math.Round(st*100) / 100, "override": tl.Override,
						"from": tl.Start().UTC().Format(time.RFC3339), "until": tl.End().UTC().Format(time.RFC3339), "retail_byte": tl.Byte(now)}
				}
				if nx, ok := e.Next(*req.World, sx, sz, now); ok {
					k, st := weather.Clear, 0.0
					for _, ph := range nx.Phases {
						if ph.Strength > st {
							k, st = ph.Kind, ph.Strength
						}
					}
					sq["next"] = map[string]any{"start": nx.Start().UTC().Format(time.RFC3339), "end": nx.End().UTC().Format(time.RFC3339),
						"kind": k.String(), "strength": weather.StrengthName(st)}
				}
				out["square"] = sq
			}
			return out, nil
		},
	})
	setProps := map[string]any{
		"kind":     strEnum("the weather: clear holds the sky clear", "rain", "snow", "sand", "whiteout", "clear"),
		"strength": strEnum("light, moderate or heavy (heavy)", "light", "moderate", "heavy"),
		"minutes":  integer("how long, 2 to 120 (10)"),
		"text":     str("said to the players standing in those zones, in the GMs' yellow (%p each one's name); empty: nothing"),
	}
	for k, v := range weatherPlaceProps {
		setProps[k] = v
	}
	s.addOp(&Op{
		Name: "weather_set", Title: "Call a GM's weather", Scope: scopeWrite,
		Description: "A GM's weather on the zones round a place (here, near N or its world), over every square whatever its ground " +
			"(lava too): it eases in from what shows over its fade, holds, and clears by its end; then the automatic weather comes back " +
			"with its next event. Sandstorms and whiteouts show on clients that draw them (vanilla); a PS2 shows rain and snow. Held in " +
			"memory: a restart drops it. Audited.",
		Input: obj(setProps, "kind", "world", "x", "z"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				weatherPlace
				Kind     string `json:"kind"`
				Strength string `json:"strength"`
				Minutes  int    `json:"minutes"`
				Text     string `json:"text"`
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			if _, err := engine(c); err != nil {
				return nil, err
			}
			kind, ok := weather.ParseKind(req.Kind)
			if !ok {
				return nil, badRequest("kind is rain, snow, sand, whiteout or clear")
			}
			strength := weather.Heavy
			if req.Strength != "" {
				if strength, ok = weather.ParseStrength(req.Strength); !ok {
					return nil, badRequest("strength is light, moderate or heavy")
				}
			}
			if req.Minutes == 0 {
				req.Minutes = 10
			}
			if len([]rune(req.Text)) > 200 {
				return nil, badRequest("text is 200 letters at most")
			}
			world, x, z, where, err := req.weatherPlace.resolve()
			if err != nil {
				return nil, err
			}
			zones, until, id, err := c.s.d.World.CallWeather(world, x, z, where, kind, strength, req.Minutes, req.Text, c.a.Username, 0)
			if err != nil {
				return nil, badRequest("%v", err)
			}
			msg := fmt.Sprintf("weather %s %s %d min on %d zones (%s at w%d %.0f,%.0f)", kind, weather.StrengthName(strength), req.Minutes,
				len(zones), where, world, x, z)
			c.s.audit(c.a, "live", "", nil, nil, nil, msg)
			return map[string]any{"id": id, "zones": zoneIDs(zones), "until": until.UTC().Format(time.RFC3339), "message": msg}, nil
		},
	})
	endProps := map[string]any{"id": integer("a GM weather's id (weather_status lists them); or give the place")}
	for k, v := range weatherPlaceProps {
		endProps[k] = v
	}
	s.addOp(&Op{
		Name: "weather_end", Title: "End a GM's weather", Scope: scopeWrite,
		Description: "Ends a GM's weather, by its id or on the zones round a place: it clears over its fade, then the automatic " +
			"weather comes back with its first event that starts after that. Audited.",
		Input: obj(endProps),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				weatherPlace
				ID int `json:"id"`
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			if _, err := engine(c); err != nil {
				return nil, err
			}
			var msg string
			n := 0
			if req.ID != 0 {
				if !c.s.d.World.EndWeatherOverride(req.ID) {
					return nil, notFound("no GM weather #%d in force", req.ID)
				}
				msg = fmt.Sprintf("weather #%d ended", req.ID)
				n = 1
			} else {
				world, x, z, where, err := req.weatherPlace.resolve()
				if err != nil {
					return nil, err
				}
				n = c.s.d.World.EndWeather(world, x, z, where)
				msg = fmt.Sprintf("weather ended on %d zones (%s at w%d %.0f,%.0f)", n, where, world, x, z)
			}
			c.s.audit(c.a, "live", "", nil, nil, nil, msg)
			return map[string]any{"ended": n, "message": msg}, nil
		},
	})
}
