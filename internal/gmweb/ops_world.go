package gmweb

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/handlers"
)

// The running world: who is online, the NPCs as they stand, and what a GM
// does to them there: respawn, kill, spawn a copy, reload, message, kick,
// teleport.

func entityID(id int64) entity.EntityID { return entity.EntityID(id) }

// colours a GM's lines may be sent in.
var lineColours = map[string][3]byte{
	"yellow": {0xFF, 0xE0, 0x40}, "white": {0xFF, 0xFF, 0xFF}, "green": {0x3F, 0xFF, 0x3F},
	"red": {0xFF, 0x50, 0x50}, "cyan": {0x40, 0xE0, 0xFF}, "orange": {0xFF, 0xA0, 0x30}, "pink": {0xFF, 0x7F, 0xFF},
}

func colourOf(name string) ([3]byte, error) {
	if name == "" {
		return lineColours["yellow"], nil
	}
	if c, ok := lineColours[strings.ToLower(name)]; ok {
		return c, nil
	}
	h := strings.TrimPrefix(name, "#")
	if len(h) == 6 {
		if v, err := strconv.ParseUint(h, 16, 32); err == nil {
			return [3]byte{byte(v >> 16), byte(v >> 8), byte(v)}, nil
		}
	}
	return [3]byte{}, badRequest("colour is one of yellow, white, green, red, cyan, orange, pink, or #rrggbb")
}

var colourSchema = str("the line's colour: yellow (default), white, green, red, cyan, orange, pink, or #rrggbb")

// characterRef names a character by its serverid or its name.
func (s *Server) characterRef(v any) (int64, string, error) {
	switch x := v.(type) {
	case json.Number:
		v = string(x)
	case float64:
		v = strconv.FormatInt(int64(x), 10)
	}
	text := strings.TrimSpace(fmt.Sprint(v))
	if text == "" || v == nil {
		return 0, "", badRequest("name a character (its name or serverid)")
	}
	var id int64
	var name string
	var err error
	if n, convErr := strconv.ParseInt(text, 10, 64); convErr == nil {
		err = s.d.DB.QueryRow(`SELECT serverid, charName FROM characters WHERE serverid = ?`, n).Scan(&id, &name)
	} else {
		err = s.d.DB.QueryRow(`SELECT serverid, charName FROM characters WHERE LOWER(charName) = LOWER(?)`, text).Scan(&id, &name)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", notFound("no character %q", text)
	}
	return id, name, err
}

// accountRef names an account by its id or its name.
func (s *Server) accountRef(v any) (accountState, error) {
	switch x := v.(type) {
	case json.Number:
		v = string(x)
	case float64:
		v = strconv.FormatInt(int64(x), 10)
	}
	text := strings.TrimSpace(fmt.Sprint(v))
	if v == nil || text == "" {
		return accountState{}, badRequest("name an account (its name or accountid)")
	}
	if n, err := strconv.Atoi(text); err == nil {
		if acc, err := s.accountByID(n); err == nil {
			return acc, nil
		}
	}
	acc, err := s.accountByName(text)
	if errors.Is(err, errNoAccount) {
		return acc, notFound("no account %q", text)
	}
	return acc, err
}

func (s *Server) registerWorldOps() {
	s.addOp(&Op{
		Name: "online_players", Title: "Players online", Scope: scopeRead, Core: true,
		Description: "Every character in the world now: its account, level, class, race, world and place, health, " +
			"purse, and what it is fighting.",
		Handler: func(c *opCtx) (any, error) {
			if c.s.d.World == nil {
				return map[string]any{"players": []any{}}, nil
			}
			ps := c.s.d.World.PlayerStates()
			out := make([]map[string]any, 0, len(ps))
			for _, p := range ps {
				acct := ""
				if acc, err := c.s.accountByID(p.AccountID); err == nil {
					acct = acc.Username
				}
				out = append(out, map[string]any{
					"serverid": p.ServerID, "name": p.Name, "account": acct, "accountid": p.AccountID,
					"level": p.Level, "class": className(p.Class), "race": raceName(p.Race),
					"world": p.World, "worldName": worldName(int(p.World)),
					"x": round1(p.X), "y": round1(p.Y), "z": round1(p.Z), "facing": p.Facing,
					"hp": p.HP, "maxHP": p.MaxHP, "power": p.Power, "maxPower": p.MaxPower,
					"tunar": p.Tunar, "dead": p.Dead, "fighting": p.Fighting, "guild": p.GuildName,
				})
			}
			return map[string]any{"players": out}, nil
		},
	})
	s.addOp(&Op{
		Name: "world_npcs", Title: "NPCs as they stand", Scope: scopeRead,
		Description: "The NPCs in the running world (not their rows): where each stands now, alive or dead, its " +
			"health and whom it fights; in one world, near a point, or by name. Also the GMs' temporary spawns.",
		Input: obj(map[string]any{
			"world":  integer("the world index (0 Tunaria, 1 Rathe, 2 Odus, 3 Lavastorm, 4 Plane of Sky, 5 Secrets...)"),
			"x":      number("with z and radius: only NPCs within radius of (x, z)"),
			"z":      number(""),
			"radius": number("default 200"),
			"name":   str("only names containing this"),
			"limit":  integer("default 200, at most 2000"),
		}, "world"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				World  int
				X, Z   *float64
				Radius float64
				Name   string
				Limit  int
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			if c.s.d.World == nil {
				return map[string]any{"npcs": []any{}}, nil
			}
			if req.Limit <= 0 {
				req.Limit = 200
			}
			req.Limit = min(req.Limit, 2000)
			if req.Radius <= 0 {
				req.Radius = 200
			}
			name := strings.ToLower(strings.TrimSpace(req.Name))
			var out []entity.NPCState
			total := 0
			for _, n := range c.s.d.World.NPCStates(req.World) {
				if name != "" && !strings.Contains(strings.ToLower(n.Name), name) {
					continue
				}
				if req.X != nil && req.Z != nil {
					dx, dz := float64(n.X)-*req.X, float64(n.Z)-*req.Z
					if math.Hypot(dx, dz) > req.Radius {
						continue
					}
				}
				total++
				if len(out) < req.Limit {
					out = append(out, n)
				}
			}
			if req.X != nil && req.Z != nil {
				sort.Slice(out, func(i, j int) bool {
					di := math.Hypot(float64(out[i].X)-*req.X, float64(out[i].Z)-*req.Z)
					dj := math.Hypot(float64(out[j].X)-*req.X, float64(out[j].Z)-*req.Z)
					return di < dj
				})
			}
			if out == nil {
				out = []entity.NPCState{}
			}
			return map[string]any{"npcs": out, "total": total}, nil
		},
	})
	s.addOp(&Op{
		Name: "map_markers", Title: "Map markers", Scope: scopeRead,
		Description: "Every NPC and player of one world as compact markers, for drawing a map: npcs is " +
			"[id, x, z, level, npc_type, flags, name index] (flags: 1 dead, 2 a GM's spawn, 4 hostile), names the " +
			"names the index points at; players is the online players there.",
		Input: obj(map[string]any{"world": integer("the world index")}, "world"),
		Handler: func(c *opCtx) (any, error) {
			var req struct{ World int }
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			if c.s.d.World == nil {
				return map[string]any{"npcs": []any{}, "names": []string{}, "players": []any{}}, nil
			}
			index := map[string]int{}
			var names []string
			npcs := make([][7]any, 0, 4096)
			for _, n := range c.s.d.World.NPCStates(req.World) {
				i, ok := index[n.Name]
				if !ok {
					i = len(names)
					index[n.Name] = i
					names = append(names, n.Name)
				}
				flags := 0
				if n.Dead {
					flags |= 1
				}
				if n.Temporary {
					flags |= 2
				}
				if n.Faction == entity.FactionHostile {
					flags |= 4
				}
				npcs = append(npcs, [7]any{n.ID, round1(n.X), round1(n.Z), n.Level, n.NPCType, flags, i})
			}
			var players []map[string]any
			for _, p := range c.s.d.World.PlayerStates() {
				if int(p.World) == req.World {
					players = append(players, map[string]any{"serverid": p.ServerID, "name": p.Name, "x": round1(p.X),
						"z": round1(p.Z), "facing": p.Facing, "level": p.Level, "dead": p.Dead})
				}
			}
			if players == nil {
				players = []map[string]any{}
			}
			return map[string]any{"world": req.World, "npcs": npcs, "names": names, "players": players}, nil
		},
	})
	s.addOp(&Op{
		Name: "npc_action", Title: "Act on an NPC", Scope: scopeWrite,
		Description: "Do something to one NPC in the running world by its id: respawn (back on its spawn point, " +
			"alive, at full health), kill (a corpse, back after its respawn time), refresh (re-read its row: what an " +
			"edit does), despawn (only a GM's temporary spawn).",
		Input: obj(map[string]any{
			"id":     integer("the NPC's id (its npcs row, or a spawn's id)"),
			"action": strEnum("what to do", "respawn", "kill", "refresh", "despawn"),
		}, "id", "action"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				ID     int64
				Action string
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			w := c.s.d.World
			if w == nil {
				return nil, badRequest("no world is running")
			}
			st, ok := w.NPCStateOf(entityID(req.ID))
			if !ok && req.Action != "refresh" {
				return nil, notFound("no NPC %d in the world", req.ID)
			}
			var msg string
			switch req.Action {
			case "respawn":
				w.RespawnNPCNow(entityID(req.ID))
				msg = fmt.Sprintf("%s respawned on its spawn point", st.Name)
			case "kill":
				if !w.KillNPCNow(entityID(req.ID)) {
					return nil, badRequest("%s is dead already, or a training dummy", st.Name)
				}
				msg = fmt.Sprintf("%s killed; back in %.0f s", st.Name, st.RespawnSec)
			case "refresh":
				msg = c.s.liveNPC("update", nil, map[string]any{"id": req.ID})
			case "despawn":
				name, ok := w.DespawnNPC(entityID(req.ID))
				if !ok {
					return nil, badRequest("%s is not a GM's spawn (delete its row to remove it for good)", st.Name)
				}
				msg = fmt.Sprintf("%s despawned", name)
			default:
				return nil, badRequest("action is respawn, kill, refresh or despawn")
			}
			c.s.audit(c.a, "live", "", nil, nil, nil, fmt.Sprintf("npc %d: %s", req.ID, msg))
			return map[string]any{"ok": true, "message": msg}, nil
		},
	})
	s.addOp(&Op{
		Name: "spawn_npc", Title: "Spawn a temporary NPC", Scope: scopeWrite,
		Description: "Put a temporary copy of an NPC (by name, as #spawn does) somewhere: at coordinates, or beside " +
			"an online player. It has no row: it is gone after its corpse time once killed, or at despawn, or at a " +
			"restart. To add an NPC for good, clone_row its npcs row instead.",
		Input: obj(map[string]any{
			"template": str("the NPC name to copy"),
			"x":        number(""), "y": number(""), "z": number(""),
			"world":  integer("with x, y, z"),
			"beside": str("instead of coordinates: an online character's name, a few steps in front of them"),
		}, "template"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Template string
				X, Y, Z  *float64
				World    int
				Beside   string
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			w := c.s.d.World
			if w == nil {
				return nil, badRequest("no world is running")
			}
			var x, y, z, facing float32
			var world uint8
			switch {
			case req.Beside != "":
				id, _, err := c.s.characterRef(req.Beside)
				if err != nil {
					return nil, err
				}
				p, on := c.s.onlineCharacter(uint32(id))
				if !on {
					return nil, badRequest("%s is not online", req.Beside)
				}
				x = p.X + 3*float32(math.Sin(float64(p.Facing)))
				z = p.Z + 3*float32(math.Cos(float64(p.Facing)))
				y, facing, world = p.Y, p.Facing+math.Pi, p.World
			case req.X != nil && req.Y != nil && req.Z != nil:
				x, y, z, world = float32(*req.X), float32(*req.Y), float32(*req.Z), uint8(req.World)
			default:
				return nil, badRequest("give x, y, z (and world), or beside")
			}
			id, spelled, ok := w.SpawnNPCCopy(req.Template, x, y, z, facing, world)
			if !ok {
				return nil, notFound("no NPC called %q in the world", req.Template)
			}
			msg := fmt.Sprintf("spawned a temporary %s (id %d) at (%.0f, %.0f, %.0f) in %s", spelled, id, x, y, z, worldName(int(world)))
			c.s.audit(c.a, "live", "", nil, nil, nil, msg)
			return map[string]any{"id": id, "name": spelled, "message": msg}, nil
		},
	})
	s.addOp(&Op{
		Name: "reload_npcs", Title: "Reload every NPC", Scope: scopeWrite,
		Description: "Put the whole npcs table into the running world again: NPCs whose rows changed outside the " +
			"panel (an importer, SQL) are remade, new rows spawn, deleted rows leave; unchanged NPCs are left as they " +
			"are. The GMs' temporary spawns stay.",
		Handler: func(c *opCtx) (any, error) {
			if c.s.d.World == nil || c.s.d.NPCs == nil {
				return nil, badRequest("no world is running")
			}
			changed, removed, err := c.s.d.World.ReloadNPCs(c.s.d.NPCs)
			if err != nil {
				return nil, err
			}
			msg := fmt.Sprintf("NPCs reloaded: %d made anew, %d removed", changed, removed)
			c.s.audit(c.a, "live", "npcs", nil, nil, nil, msg)
			return map[string]any{"changed": changed, "removed": removed, "message": msg}, nil
		},
	})
	s.addOp(&Op{
		Name: "broadcast", Title: "Message everyone", Scope: scopeWrite, Core: true,
		Description: "Send one line to every player in the world, in their chat window (as from the server).",
		Input:       obj(map[string]any{"text": str("the line (up to 400 characters)"), "colour": colourSchema}, "text"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Text   string
				Colour string
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			text := strings.TrimSpace(req.Text)
			if text == "" || len([]rune(text)) > 400 {
				return nil, badRequest("the line is 1 to 400 characters")
			}
			rgb, err := colourOf(req.Colour)
			if err != nil {
				return nil, err
			}
			n := handlers.Broadcast(c.s.d.Sessions, text, rgb)
			c.s.audit(c.a, "live", "", nil, nil, nil, fmt.Sprintf("broadcast to %d players: %s", n, text))
			return map[string]any{"sentTo": n}, nil
		},
	})
	s.addOp(&Op{
		Name: "message_player", Title: "Message a player", Scope: scopeWrite, Core: true,
		Description: "Send one line to one online character's chat window, marked as from a GM.",
		Input: obj(map[string]any{
			"character": idOrName("the character, by name or serverid"),
			"text":      str("the line"),
			"colour":    colourSchema,
		}, "character", "text"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Character any
				Text      string
				Colour    string
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			id, name, err := c.s.characterRef(req.Character)
			if err != nil {
				return nil, err
			}
			sess := c.s.sessionFor(uint32(id))
			if sess == nil {
				return nil, badRequest("%s is not online", name)
			}
			text := strings.TrimSpace(req.Text)
			if text == "" || len([]rune(text)) > 400 {
				return nil, badRequest("the line is 1 to 400 characters")
			}
			rgb, err := colourOf(req.Colour)
			if err != nil {
				return nil, err
			}
			handlers.GMLine(sess, "[GM] "+text, rgb)
			c.s.audit(c.a, "live", "", nil, nil, nil, fmt.Sprintf("message to %s: %s", name, text))
			return map[string]any{"sent": true}, nil
		},
	})
	s.addOp(&Op{
		Name: "kick_player", Title: "Kick a player", Scope: scopeWrite, Destructive: true, Core: true,
		Description: "Eject a character from the game (\"You have been ejected by a server administrator.\"), " +
			"saved as at a logout. With account instead, every session of that account. A reason is sent to them " +
			"first. It does not stop them logging in again: ban_account does.",
		Input: obj(map[string]any{
			"character": idOrName("the character, by name or serverid"),
			"account":   idOrName("or the account, by name or accountid: all its sessions"),
			"reason":    str("a line they see before they go"),
		}),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Character any
				Account   any
				Reason    string
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			reason := strings.TrimSpace(req.Reason)
			rgb := lineColours["red"]
			if req.Character != nil {
				id, name, err := c.s.characterRef(req.Character)
				if err != nil {
					return nil, err
				}
				sess := c.s.sessionFor(uint32(id))
				if sess == nil {
					return nil, badRequest("%s is not online", name)
				}
				if acc, err := c.s.accountByID(sess.AccountID()); err == nil && acc.Level >= c.a.Level && acc.ID != c.a.AccountID {
					return nil, forbidden("%s's account is at or above your level", name)
				}
				if reason != "" {
					handlers.GMLine(sess, "[GM] "+reason, rgb)
				}
				handlers.Eject(sess)
				c.s.audit(c.a, "live", "", nil, nil, nil, fmt.Sprintf("kicked %s: %s", name, reason))
				return map[string]any{"kicked": name}, nil
			}
			if req.Account != nil {
				acc, err := c.s.accountRef(req.Account)
				if err != nil {
					return nil, err
				}
				if acc.Level >= c.a.Level && acc.ID != c.a.AccountID {
					return nil, forbidden("%s is at or above your level", acc.Username)
				}
				if reason != "" {
					for _, p := range c.s.d.World.PlayerStates() {
						if p.AccountID == acc.ID {
							if sess := c.s.sessionFor(p.ServerID); sess != nil {
								handlers.GMLine(sess, "[GM] "+reason, rgb)
							}
						}
					}
				}
				n := handlers.EjectAccount(c.s.d.Sessions, acc.ID)
				c.s.audit(c.a, "live", "", nil, nil, nil, fmt.Sprintf("kicked account %s (%d sessions): %s", acc.Username, n, reason))
				return map[string]any{"account": acc.Username, "sessions": n}, nil
			}
			return nil, badRequest("name a character or an account")
		},
	})
	s.addOp(&Op{
		Name: "teleport_character", Title: "Teleport a character", Scope: scopeWrite, Core: true,
		Description: "Move a character: one online is teleported where it stands (as a harbormaster's warp), one " +
			"offline has its saved place changed for its next login. Say where with ONE of: place (a travel " +
			"destination such as \"freeport\", a travel NPC's name, or a word of the names of NPCs standing " +
			"together, such as \"guk\"), npc (an NPC's name or id: on its spot), " +
			"player (an online character: on their spot), or x, y, z with world. The place must be in its world and " +
			"above the floor, and in a world the character's game loads (the disc's six, unless it is a patched " +
			"PS2's; an offline character logs in only in those six).",
		Input: obj(map[string]any{
			"character": idOrName("the character to move, by name or serverid"),
			"place":     str("a travel destination or travel NPC (teleport_destinations lists them)"),
			"npc":       idOrName("an NPC by name or id, or a word of the names of NPCs standing together (\"guk\")"),
			"player":    str("an online character's name"),
			"x":         number(""), "y": number(""), "z": number(""),
			"world": integer("with x, y, z: the world index"),
		}, "character"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Character any
				Place     string
				NPC       any
				Player    string
				X, Y, Z   *float64
				World     *int
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			return c.s.teleportCharacter(c.a, req.Character, req.Place, req.NPC, req.Player, req.X, req.Y, req.Z, req.World)
		},
	})
	s.addOp(&Op{
		Name: "teleport_destinations", Title: "Travel destinations", Scope: scopeRead,
		Description: "The named places teleport_character's place takes: the GM shortcuts and every travel NPC's spot, " +
			"in the six worlds every game has. A place may also be named by the NPCs standing there (\"guk\": the " +
			"froglok guk ... of Guk).",
		Handler: func(c *opCtx) (any, error) {
			var out []map[string]any
			for _, d := range handlers.TeleDestinations() {
				if !handlers.DiscWorld(d.World) {
					continue
				}
				out = append(out, map[string]any{"name": d.Name, "label": d.Label, "world": d.World})
			}
			if c.s.d.World != nil {
				for _, d := range handlers.WarpDestinations(c.s.d.World) {
					out = append(out, map[string]any{"name": d.Name, "label": d.Label, "world": d.World})
				}
				for _, p := range c.s.d.World.Places(handlers.DiscWorld) {
					out = append(out, map[string]any{"name": p.NPC, "label": p.Name, "world": p.World})
				}
			}
			return map[string]any{"destinations": out}, nil
		},
	})
}

func round1(v float32) float64 { return math.Round(float64(v)*10) / 10 }

// teleportCharacter is teleport_character.
func (s *Server) teleportCharacter(a *actor, who any, place string, npc any, player string, x, y, z *float64, world *int) (any, error) {
	id, name, err := s.characterRef(who)
	if err != nil {
		return nil, err
	}
	var tx, ty, tz float32
	var tw byte
	via := ""
	// the worlds the character's game loads: its session's, else (offline)
	// the disc's six, the only ones a login starts in
	sess := s.sessionFor(uint32(id))
	loadable := handlers.LoadableWorlds(sess)
	switch {
	case strings.TrimSpace(place) != "":
		px, py, pz, pw, v, ok := handlers.ResolveTeleDest(s.d.World, place)
		if !ok && s.d.World != nil {
			for _, p := range s.d.World.Places(loadable) {
				if strings.EqualFold(p.Name, place) || strings.EqualFold(p.NPC, place) {
					px, py, pz, pw, v, ok = p.X, p.Y, p.Z, p.World, p.Name, true
					break
				}
			}
		}
		if !ok && s.d.World != nil {
			if p, n, found := handlers.NamesakePlace(s.d.World, place, loadable); found {
				px, py, pz, pw, ok = p.X, p.Y, p.Z, p.World, true
				v = p.NPC
				if n > 1 {
					v = fmt.Sprintf("%s (%d NPCs called %s stand there)", p.NPC, n, place)
				}
			}
		}
		if !ok {
			return nil, notFound("no destination %q (teleport_destinations lists them)", place)
		}
		tx, ty, tz, tw, via = px, py, pz, pw, v
	case npc != nil:
		st, err := s.npcPlace(npc)
		if err != nil {
			// no NPC of that name: the NPCs a word of whose names it is, when
			// they stand together ("guk", Guk's frogloks)
			text := strings.TrimSpace(fmt.Sprint(npc))
			p, n, ok := handlers.NamesakePlace(s.d.World, text, loadable)
			if !ok {
				return nil, err
			}
			st = entity.NPCState{Name: p.NPC, X: p.X, Y: p.Y, Z: p.Z, World: p.World}
			if n > 1 {
				st.Name = fmt.Sprintf("%s (%d NPCs called %s stand there)", p.NPC, n, text)
			}
		}
		tx, ty, tz, tw, via = st.X, st.Y, st.Z, st.World, st.Name
	case strings.TrimSpace(player) != "":
		pid, pname, err := s.characterRef(player)
		if err != nil {
			return nil, err
		}
		p, on := s.onlineCharacter(uint32(pid))
		if !on {
			return nil, badRequest("%s is not online", pname)
		}
		tx, ty, tz, tw, via = p.X, p.Y, p.Z, p.World, pname
	case x != nil && y != nil && z != nil:
		w := 0
		if world != nil {
			w = *world
		}
		if w < 0 || w > 255 {
			return nil, badRequest("world %d does not exist", w)
		}
		tx, ty, tz, tw, via = float32(*x), float32(*y), float32(*z), byte(w), "coordinates"
	default:
		return nil, badRequest("say where: place, npc, player, or x, y, z and world")
	}
	saveWorld, err := handlers.ValidateTeleport(tx, ty, tz, tw)
	if err != nil {
		return nil, badRequest("%v", err)
	}
	// a world the character's game lacks: the client would stay where it was
	// while the server filed it there (online), or the login would start it
	// beside Archivist Vaal instead (offline)
	if !loadable(saveWorld) {
		if sess == nil {
			return nil, badRequest("%s is offline, and a login starts only in the disc's six worlds, not in %s (world %d)",
				name, worldName(int(saveWorld)), saveWorld)
		}
		return nil, badRequest("%s", handlers.CannotLoad(name, saveWorld))
	}
	if p, on := s.onlineCharacter(uint32(id)); on {
		msg, err := s.teleportLive(p, tx, ty, tz, saveWorld)
		if err != nil {
			return nil, err
		}
		s.audit(a, "live", "", nil, nil, nil, fmt.Sprintf("teleported %s to %s: %s", name, via, msg))
		return map[string]any{"online": true, "message": fmt.Sprintf("%s %s (%s)", name, msg, via)}, nil
	}
	t, err := s.table("characters")
	if err != nil {
		return nil, err
	}
	ch, err := s.updateRow(a, t, map[string]any{"serverid": id}, map[string]any{
		"x": float64(tx), "y": float64(ty), "z": float64(tz), "world": int64(saveWorld),
	}, nil, writeOpts{})
	if err != nil {
		return nil, err
	}
	return map[string]any{"online": false, "message": fmt.Sprintf("%s is offline: their next login starts at %s (%.0f, %.0f, %.0f) in %s",
		name, via, tx, ty, tz, worldName(int(saveWorld))), "change": ch}, nil
}

// npcPlace is where an NPC named by id or name stands now (its spawn point
// when it is not in the world).
func (s *Server) npcPlace(v any) (entity.NPCState, error) {
	if n, ok := v.(json.Number); ok {
		v = string(n)
	}
	text := strings.TrimSpace(fmt.Sprint(v))
	if id, err := strconv.ParseInt(text, 10, 64); err == nil {
		if s.d.World != nil {
			if st, ok := s.d.World.NPCStateOf(entityID(id)); ok {
				return st, nil
			}
		}
		rec, err := s.d.NPCs.GetNPC(int(id))
		if err != nil {
			return entity.NPCState{}, notFound("no NPC %d", id)
		}
		return entity.NPCState{ID: entityID(id), Name: rec.NPCName, X: rec.XCoord, Y: rec.YCoord, Z: rec.ZCoord,
			World: uint8(rec.World)}, nil
	}
	if s.d.World != nil {
		if x, y, z, _, w, ok := s.d.World.NPCByName(text); ok {
			return entity.NPCState{Name: text, X: x, Y: y, Z: z, World: w}, nil
		}
	}
	return entity.NPCState{}, notFound("no NPC called %q in the world", text)
}
