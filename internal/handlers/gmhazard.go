package handlers

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// gmhazard.go — a GM's /hazard (the GM Jibwiz's petition 1960, 2026-11-13: "We need a system to
// apply effects to the world itself without placing a monster ... Build out a system for us to
// replicate this in the world ... Build this in a way we can use the system for other similar
// effects elsewhere in the world"): table hazards' rows (database/hazards.go, entity/hazards.go)
// placed, listed and taken away from the game, as /hazard, #hazard (a GM's, or a granted sub GM's):
//
//   - /hazard add <damage> <every seconds> [radius] [still] [name]: a round area where the GM
//     stands (its feet the floor), radius across (default 1.5, at most 100), hitting a player in
//     it for damage (fixed, through armour) at once and again after every seconds (0.5 to 3600) -
//     only once it has moved since its last hit (the thorns' rule, on_move), or, with "still",
//     standing still too. Its name (default "Hazard") names it in the log and its words ("<name>
//     hurts you for <n>.").
//   - /hazard list: the hazards of the GM's world nearest it (ten, within 200), with their ids.
//   - /hazard remove [id]: that row taken away; with no id, the nearest one holding the GM.
//
// Every change has its gm_audit line (via "game") and is in the world at once (LoadHazards). The GM
// panel's Hazards table edits the rest (a capsule's two ends, below and above, the words).

const hazardUsage = "Usage: /hazard add <damage> <every seconds> [radius] [still] [name] | /hazard list | /hazard remove [id]"

// hazardListReach is how far /hazard list looks, hazardListMax how many it names.
const (
	hazardListReach = 200
	hazardListMax   = 10
)

// hazardAdd is /hazard add's words read: the row it places (its place the caller's), or why not.
func hazardAdd(args []string) (database.Hazard, string) {
	h := database.Hazard{OnMove: true, Name: "Hazard"}
	if len(args) < 2 {
		return h, hazardUsage
	}
	dmg, err := strconv.Atoi(args[0])
	if err != nil || dmg < 1 || dmg > database.HazardDamageMax {
		return h, fmt.Sprintf("/hazard add: damage %q: a whole number, 1 to %d", args[0], database.HazardDamageMax)
	}
	every, err := strconv.ParseFloat(args[1], 64)
	if err != nil || math.IsNaN(every) || every < 0.5 || every > 3600 {
		return h, fmt.Sprintf("/hazard add: every %q: seconds, 0.5 to 3600", args[1])
	}
	h.Damage, h.EveryMS = dmg, int(math.Round(every*1000))
	rest := args[2:]
	if len(rest) > 0 {
		if r, err := strconv.ParseFloat(rest[0], 64); err == nil {
			if math.IsNaN(r) || r <= 0 || r > database.HazardRadiusMax {
				return h, fmt.Sprintf("/hazard add: radius %q: above 0, at most %d", rest[0], database.HazardRadiusMax)
			}
			h.Radius, rest = float32(r), rest[1:]
		}
	}
	if len(rest) > 0 && strings.EqualFold(rest[0], "still") {
		h.OnMove, rest = false, rest[1:]
	}
	if name := strings.TrimSpace(strings.Join(rest, " ")); name != "" {
		if len(name) > 60 {
			name = name[:60]
		}
		h.Name = strings.Map(func(r rune) rune {
			if r < 0x20 || r == '%' {
				return -1
			}
			return r
		}, name)
	}
	return h, ""
}

// hazardCommand is /hazard (the file's doc).
func (h *ChatHandler) hazardCommand(sess *session.Session, player *entity.Player, args []string) {
	db := h.camps.db
	if db == nil {
		h.gmReply(sess, "/hazard: no database here")
		return
	}
	if len(args) == 0 {
		h.gmReply(sess, hazardUsage)
		return
	}
	switch strings.ToLower(args[0]) {
	case "add":
		hz, bad := hazardAdd(args[1:])
		if bad != "" {
			h.gmReply(sess, bad)
			return
		}
		hz.X1, hz.Y1, hz.Z1 = player.X, player.Y, player.Z
		hz.X2, hz.Y2, hz.Z2 = player.X, player.Y, player.Z
		hz.Note = fmt.Sprintf("/hazard add by %s", player.Name)
		hz.Source = "gm"
		detail := fmt.Sprintf("/hazard add: %q, %d every %.1f s (radius %.1f, %s) at (%.1f, %.1f, %.1f) world %d",
			hz.Name, hz.Damage, float64(hz.EveryMS)/1000, hz.RadiusOr(), hazardRule(hz), hz.X1, hz.Y1, hz.Z1, player.World)
		id, err := db.AddHazardAudited(hz, int(player.World), gmEditOf(sess), detail)
		if err != nil {
			h.gmReply(sess, "/hazard add: nothing placed: "+err.Error())
			return
		}
		h.reloadHazards(sess)
		h.gmReply(sess, fmt.Sprintf("Hazard %d: %q here, %d damage every %.1f s, radius %.1f, %s. /hazard remove %d takes it away.",
			id, hz.Name, hz.Damage, float64(hz.EveryMS)/1000, hz.RadiusOr(), hazardRule(hz), id))
		log.Printf("GM: %s %s (row %d)", player.Name, detail, id)
	case "list":
		all, _, err := db.Hazards()
		if err != nil {
			h.gmReply(sess, "/hazard list: "+err.Error())
			return
		}
		near := hazardsNear(all, player)
		inWorld := 0
		for _, hz := range all {
			if hz.World == player.World {
				inWorld++
			}
		}
		h.gmReply(sess, fmt.Sprintf("%d hazards in this world, %d within %d:", inWorld, len(near), hazardListReach))
		for i, n := range near {
			if i == hazardListMax {
				h.gmReply(sess, fmt.Sprintf("  ... %d more", len(near)-hazardListMax))
				break
			}
			hz := n.h
			h.gmReply(sess, fmt.Sprintf("  %d %q: %d every %.1f s, radius %.1f, %s, %.0f away (%s)", hz.ID, hz.Name, hz.Damage,
				float64(hz.EveryOr())/1000, hz.RadiusOr(), hazardRule(hz), n.d, hz.Source))
		}
	case "remove", "delete":
		var id int64
		if len(args) > 1 {
			v, err := strconv.ParseInt(args[1], 10, 64)
			if err != nil || v <= 0 {
				h.gmReply(sess, hazardUsage)
				return
			}
			id = v
		} else {
			all, _, err := db.Hazards()
			if err != nil {
				h.gmReply(sess, "/hazard remove: "+err.Error())
				return
			}
			for _, n := range hazardsNear(all, player) {
				if database.HazardHolds(n.h, player.X, player.Y, player.Z) {
					id = n.h.ID
					break
				}
			}
			if id == 0 {
				h.gmReply(sess, "/hazard remove: you stand in none; /hazard list names them, /hazard remove <id>")
				return
			}
		}
		ok, err := db.DeleteHazardAudited(id, gmEditOf(sess), fmt.Sprintf("/hazard remove %d by %s", id, player.Name))
		switch {
		case err != nil:
			h.gmReply(sess, "/hazard remove: nothing removed: "+err.Error())
			return
		case !ok:
			h.gmReply(sess, fmt.Sprintf("/hazard remove: no hazard %d", id))
			return
		}
		h.reloadHazards(sess)
		h.gmReply(sess, fmt.Sprintf("Hazard %d removed (the GM panel's audit log can put it back).", id))
		log.Printf("GM: %s removed hazard %d", player.Name, id)
	default:
		h.gmReply(sess, hazardUsage)
	}
}

// hazardRule says when an area hits again.
func hazardRule(hz database.Hazard) string {
	if hz.OnMove {
		return "again only once moved"
	}
	return "standing still too"
}

type hazardNear struct {
	h database.Hazard
	d float64
}

// hazardsNear is the hazards of p's world within hazardListReach of it (the nearest point of each
// segment), nearest first.
func hazardsNear(all []database.Hazard, p *entity.Player) []hazardNear {
	var out []hazardNear
	for _, hz := range all {
		if hz.World != p.World {
			continue
		}
		d := segDist(hz, p.X, p.Z)
		if d <= hazardListReach {
			out = append(out, hazardNear{hz, d})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].d < out[j].d })
	return out
}

// segDist is how far (x, z) is across from hz's segment.
func segDist(hz database.Hazard, x, z float32) float64 {
	sx, sz := float64(hz.X2-hz.X1), float64(hz.Z2-hz.Z1)
	px, pz := float64(x-hz.X1), float64(z-hz.Z1)
	t := 0.0
	if l := sx*sx + sz*sz; l > 0 {
		t = math.Min(math.Max((px*sx+pz*sz)/l, 0), 1)
	}
	return math.Hypot(px-t*sx, pz-t*sz)
}

// reloadHazards reads table hazards into the world again after a /hazard change.
func (h *ChatHandler) reloadHazards(sess *session.Session) {
	if _, err := h.gameWorld.LoadHazards(h.camps.db); err != nil {
		h.gmReply(sess, "/hazard: kept, but not in the world until a restart: "+err.Error())
	}
}
