package tcp

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
)

// The account pages' teleport route.
//
// It moves a character to a named destination or to typed coordinates. Two
// things make it different from the in-game #tele it shares a table with, and
// both are the reason this file exists rather than a few lines bolted onto the
// character list:
//
//   - IT IS REACHABLE BY ANYONE WHO CAN REACH THE PORT, so the character has to
//     be proved to belong to the logged-in account. A serverID is a number a
//     browser can type.
//   - IT CAN MOVE A CHARACTER WHO IS NOT LOGGED IN, which the game cannot, and
//     that is a DIFFERENT write: an online character lives in memory and is
//     saved on disconnect, so a database write would be quietly undone by the
//     player's own logout.

// teleportRequest is what the page posts. `Dest` names a shortcut; when it is
// empty the explicit coordinates are used instead.
type teleportRequest struct {
	ServerID uint32  `json:"serverid"`
	Dest     string  `json:"dest"`
	World    byte    `json:"world"`
	X        float32 `json:"x"`
	Y        float32 `json:"y"`
	Z        float32 `json:"z"`
}

func webError(w http.ResponseWriter, msg string) {
	json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": msg})
}

// handleAPITeleportDests lists the named shortcuts for the page to render.
func (s *AccountServer) handleAPITeleportDests(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.getSession(r) == nil {
		webError(w, "Not logged in")
		return
	}
	var dests []TeleportDest
	if s.Destinations != nil {
		dests = s.Destinations()
	}
	var worlds []string
	if s.WorldNames != nil {
		worlds = s.WorldNames()
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true, "destinations": dests, "worlds": worlds,
	})
}

// handleAPITeleport moves one character on the logged-in account.
func (s *AccountServer) handleAPITeleport(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		webError(w, "Method not allowed")
		return
	}
	sess := s.getSession(r)
	if sess == nil {
		webError(w, "Not logged in")
		return
	}
	if s.ResolveDest == nil || s.ValidatePlace == nil {
		// A server with no game world attached cannot place anything safely.
		// Refusing is the whole answer; writing a row we could not check is how
		// a character gets stranded somewhere nothing will ever load.
		webError(w, "Teleport is not available on this server")
		return
	}

	var req teleportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webError(w, "Bad request")
		return
	}

	// OWNERSHIP FIRST. Everything below moves a character, so nothing below runs
	// until this one belongs to the account that asked. The list is the account's
	// own, so a serverID that is not in it is not this account's to move —
	// whether it exists or not, which is also why the answer does not say.
	chars, err := s.charStore.GetByAccountID(sess.AccountID)
	if err != nil {
		webError(w, "Failed to load characters")
		return
	}
	var owned bool
	for _, c := range chars {
		if c.ServerID == req.ServerID {
			owned = true
		}
	}
	if !owned {
		log.Printf("Web teleport: account %d (%s) asked to move character %d, "+
			"which is not on that account — refused",
			sess.AccountID, sess.Username, req.ServerID)
		webError(w, "No such character on this account")
		return
	}

	x, y, z, world, via := req.X, req.Y, req.Z, req.World, "typed coordinates"
	// NOT lower-cased. A route is named by the NPC that defines it, and
	// ResolveTeleDest is the one that knows which key space a name belongs to.
	if name := strings.TrimSpace(req.Dest); name != "" {
		var ok bool
		x, y, z, world, via, ok = s.ResolveDest(name)
		if !ok {
			webError(w, "Unknown destination "+strconv.Quote(req.Dest))
			return
		}
	}

	// The same two guards the disconnect path applies, because a web form can
	// produce both by typing: a height under the world floor is a fall, and a
	// world that does not contain the position strands the character where no
	// NPC will ever appear.
	if _, err := s.ValidatePlace(x, y, z, world); err != nil {
		webError(w, err.Error())
		return
	}
	if s.CanLoad != nil {
		if err := s.CanLoad(req.ServerID, world); err != nil {
			webError(w, err.Error())
			return
		}
	}

	// ONLINE FIRST, and only write the row when nobody is holding it. The two
	// are alternatives rather than a belt and braces: a logged-in character's
	// position is in memory and written back on disconnect, so a row written
	// underneath one is overwritten by that player's own logout.
	if s.TeleportLive != nil && s.TeleportLive(req.ServerID, x, y, z, world) {
		log.Printf("Web teleport: %s moved character %d to %s (%.0f, %.0f, %.0f) "+
			"world=%d, in game", sess.Username, req.ServerID, via, x, y, z, world)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true, "online": true, "via": via,
			"x": x, "y": y, "z": z, "world": world,
		})
		return
	}

	// OFFLINE, and this world cannot be stored. Refusing is the whole answer:
	// there is no console to hand over, and writing the row would arm a login
	// loop on a character nobody is even playing.
	if s.SendOnly != nil && s.SendOnly(world) {
		log.Printf("Web teleport: %s asked to send offline character %d to "+
			"world %d, which is send-only — refused rather than stored",
			sess.Username, req.ServerID, world)
		webError(w, "That destination only works while the character is logged in")
		return
	}

	record, err := s.charStore.GetByServerID(req.ServerID)
	if err != nil || record == nil {
		webError(w, "Failed to load that character")
		return
	}
	record.X, record.Y, record.Z, record.World = x, y, z, int(world)
	if err := s.charStore.Update(record); err != nil {
		log.Printf("Web teleport: could not save character %d: %v", req.ServerID, err)
		webError(w, "Failed to save the new position")
		return
	}
	log.Printf("Web teleport: %s moved offline character %d (%s) to %s "+
		"(%.0f, %.0f, %.0f) world=%d", sess.Username, req.ServerID, record.CharName,
		via, x, y, z, world)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true, "online": false, "via": via,
		"x": x, "y": y, "z": z, "world": world,
	})
}
