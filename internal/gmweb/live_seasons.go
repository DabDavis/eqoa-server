package gmweb

import (
	"fmt"
	"strings"

	"github.com/eqoa/server/internal/database"
)

// The seasons and their decorations (database/seasons.go, decorlights.go): a
// season on or off (gmseasons), the NPCs that stand only in theirs (npcseason),
// and the strings of lights (lighttemplates, lightstrings).

// liveSeasons makes the world hold the seasons as the tables now say: their
// NPCs in or out of every view, their lights lit or put out for every client
// that draws them (the change is committed: the database is free).
func (s *Server) liveSeasons() string {
	if s.d.World == nil || s.d.DB == nil {
		return "no world is running: at the next start"
	}
	held, changed, err := s.d.World.LoadSpawnOnly(s.d.DB)
	if err != nil {
		return "not applied to the world: " + err.Error()
	}
	strs, bulbs, told, problems, err := s.d.World.LoadDecorLights(s.d.DB)
	if err != nil {
		return "NPCs applied; the lights not: " + err.Error()
	}
	line := fmt.Sprintf("at once: %d NPC(s) changed, %d out of the world in all; %d strings of lights lit (%d bulbs), sent to %d client(s) that draw them (a PS2 draws none)",
		changed, held, strs, bulbs, told)
	if len(problems) > 0 {
		line += "; laying nothing: " + strings.Join(problems, "; ")
	}
	return line
}

// checkSeasonRow holds a gmseasons row to a name and an active of 0 or 1.
func checkSeasonRow(row map[string]any) error {
	if v, ok := row["name"]; ok {
		if name, _ := v.(string); strings.TrimSpace(name) == "" {
			return badRequest("name is the season's (Christmas)")
		}
	}
	if v, ok := row["active"]; ok {
		if f, fok := asFloat(v); !fok || (f != 0 && f != 1) {
			return badRequest("active is 1 (on) or 0 (off)")
		}
	}
	return nil
}

// checkNPCSeasonRow holds an npcseason row to an NPC's name and a season.
func checkNPCSeasonRow(row map[string]any) error {
	if v, ok := row["name"]; ok {
		if name, _ := v.(string); database.NPCClassKey(name) == "" {
			return badRequest("name is the NPC's name, as in NPCs (any case)")
		}
	}
	if v, ok := row["season"]; ok {
		if season, _ := v.(string); strings.TrimSpace(season) == "" {
			return badRequest("season is the season's name, as in Seasons (Christmas)")
		}
	}
	return nil
}

// checkLightTemplateRow holds a lighttemplates row to values that light.
func checkLightTemplateRow(row map[string]any) error {
	str := func(k, def string) string {
		if v, ok := row[k].(string); ok {
			return v
		}
		return def
	}
	num := func(k string, def float64) float64 {
		if v, ok := row[k]; ok {
			if f, fok := asFloat(v); fok {
				return f
			}
			return -1
		}
		return def
	}
	if v, ok := row["name"]; ok {
		if name, _ := v.(string); strings.TrimSpace(name) == "" {
			return badRequest("name is the template's (Christmas lights)")
		}
	}
	if p := database.LightTemplateProblems(str("colors", "FFFFFF"), str("pattern", database.LightCycle), str("effect", database.LightTwinkle),
		num("period", 1.5), num("spacing", 1), num("size", 0.3), num("bright", 1), str("cord", ""), num("cord_width", 0.06)); len(p) > 0 {
		return badRequest("%s", strings.Join(p, "; "))
	}
	return nil
}

// checkLightStringRow holds a lightstrings row to a template, a shape and
// params that read.
func checkLightStringRow(row map[string]any) error {
	if v, ok := row["template"]; ok {
		if t, _ := v.(string); strings.TrimSpace(t) == "" {
			return badRequest("template is a light template's name (Christmas lights)")
		}
	}
	if v, ok := row["shape"]; ok {
		shape, _ := v.(string)
		found := false
		for _, s := range database.LightShapes {
			found = found || strings.EqualFold(strings.TrimSpace(shape), s)
		}
		if !found {
			return badRequest("shape is one of %s", strings.Join(database.LightShapes, ", "))
		}
	}
	if v, ok := row["params"]; ok {
		p, _ := v.(string)
		if _, err := database.ReadLightParams(p); err != nil {
			return badRequest("%s: a JSON object, as {\"r0\": 4, \"y0\": 1, \"r1\": 0.5, \"y1\": 12, \"turns\": 6}", err.Error())
		}
	}
	if v, ok := row["world"]; ok {
		if f, fok := asFloat(v); !fok || f < 0 || f > 29 {
			return badRequest("world is 0 to 29 (a string on an NPC hangs wherever the NPC is)")
		}
	}
	return nil
}
