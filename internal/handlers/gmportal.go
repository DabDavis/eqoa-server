package handlers

import (
	"errors"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// A GM's portals (petition 1624, the GM Jibrish: "We need a GM command that
// allows me to mark the entry and exit points of teleporters. It should be like
// /markportal and if im targeting something that NPC should be the start of the
// teleport. Then I /markportal exit and it either uses where im standing, or
// what im targeting but not both as the destination. Add a brief dialogue to
// the entry portal with a yes or no confirmation if they do not already have
// one. Once I mark the exit, have it automatically create a ticket with all the
// relevant information captured for you to add it."). Each a GM's (acctlevel 1
// or more), as /name or #name:
//
//   - /markportal: the GM's target, an NPC of a row, kept as the GM's portal
//     entry (in memory, by account, until its exit is marked, another entry
//     replaces it, /markportal cancel or a restart);
//   - /markportal exit [place]: the landing, ONE of: the GM's target when it
//     has one (an NPC: in front of it, as a waysender's trip lands; a player:
//     its spot), else where the GM stands, facing as it faces; the reply says
//     which. The pair is kept as a row of gmportals (database/gmportals.go),
//     OFF, and a ticket is filed from the GM's character (Internal / Portal,
//     with the server's view) holding everything a builder needs: the entry
//     (row, name, world, spot, facing, body, level, and what it already says),
//     the landing (world, spot, facing, the target's name and row), the NPCs
//     near each end, the question it will ask and a ready wayRoutes line. The
//     place is what the question names ("Do you wish to travel to <place>?"):
//     the GM's words, else the target's name, else the nearest named NPC, else
//     the world's;
//   - /markportal on [id] | off [id]: a portal switched on or off (no id: the
//     GM's last), at once: an entry with a portal on asks "Do you wish to travel
//     to <place>?" Yes or No, and Yes is a waysender's trip there (fee and level
//     0 unless a GM sets the row's), in the same world or another its client
//     loads (CanLoadWorld); the group is not moved, as a waysender's trip moves
//     no one else. An entry that is a waysender already lists the portal's
//     place among its own trips; a quest's step at the NPC, a sign's words and
//     the named services (the Teleporter, a dyer, ...) come first;
//   - /markportal list: the last portals marked; /markportal cancel: the entry
//     forgotten.
//
// The ticket is for whoever builds the portal ("for you to add it"): switching
// its row on is the building, or a wayRoutes line for a portal that should
// ship in the code. Every row change has its gm_audit line.

// GMPortals is /markportal's and the portals' talk's: the database, each GM's
// entry, and the portals switched on by entry NPC, read again every
// gmPortalsReread (a row switched on by hand on the box is live within it).
type GMPortals struct {
	db *database.SQLiteDB

	mu      sync.Mutex
	entries map[int]portalEntry // by the GM's account
	last    map[int]int64       // the GM's last portal marked, by account
	on      map[uint32][]database.GMPortal
	read    time.Time
	now     func() time.Time
}

// gmPortalsReread is how old the portals switched on may be before a talk
// reads them again.
const gmPortalsReread = 30 * time.Second

// NewGMPortals is the portals kept in db.
func NewGMPortals(db *database.SQLiteDB) *GMPortals {
	return &GMPortals{db: db, entries: map[int]portalEntry{}, last: map[int]int64{}, now: time.Now}
}

// portalEntry is a GM's marked entry: the NPC's row and where its row stands.
type portalEntry struct {
	NPCID           int
	Name            string
	World           int
	X, Y, Z, Facing float64
	FacingByte      int
	Model, Level    int
	Says            []string
}

// enabled is the portals switched on whose entry is the NPC of that row.
func (g *GMPortals) enabled(npcID uint32) []database.GMPortal {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.on == nil || g.now().Sub(g.read) >= gmPortalsReread {
		g.reloadLocked()
	}
	return g.on[npcID]
}

// reloadLocked reads the portals switched on again.
func (g *GMPortals) reloadLocked() {
	g.read = g.now()
	rows, err := g.db.EnabledGMPortals()
	if err != nil {
		log.Printf("Portals: gmportals: %v", err)
		if g.on == nil {
			g.on = map[uint32][]database.GMPortal{}
		}
		return
	}
	on := map[uint32][]database.GMPortal{}
	for _, r := range rows {
		on[uint32(r.NPCID)] = append(on[uint32(r.NPCID)], r)
	}
	g.on = on
}

// forget makes the next talk read the portals again (a portal switched).
func (g *GMPortals) forget() {
	g.mu.Lock()
	g.on = nil
	g.mu.Unlock()
}

// facingOfByte is an npcs row's facing (0-255 round the circle) in radians.
func facingOfByte(b int) float64 {
	return float64(b&0xFF) / 256 * 2 * math.Pi
}

// portalLandmarkReach is how far from a point the NPCs named beside it stand.
const portalLandmarkReach = 200

// portalLandmarks are up to n names of NPCs of rows (none of skip's row)
// within portalLandmarkReach of (x, z) in world, nearest first, each name
// once, with its row and distance.
func portalLandmarks(gw *entity.World, world int, x, z float64, skip, n int) []string {
	if gw == nil || world < 0 {
		return nil
	}
	type near struct {
		s entity.NPCState
		d float64
	}
	var all []near
	for _, s := range gw.NPCStates(world) {
		if s.Temporary || int(s.ID) == skip {
			continue
		}
		d := math.Hypot(float64(s.SpawnX)-x, float64(s.SpawnZ)-z)
		if d <= portalLandmarkReach {
			all = append(all, near{s, d})
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].d < all[j].d })
	seen := map[string]bool{}
	var out []string
	for _, a := range all {
		if seen[strings.ToLower(a.s.Name)] {
			continue
		}
		seen[strings.ToLower(a.s.Name)] = true
		out = append(out, fmt.Sprintf("%s (npc %d, %.0f away)", a.s.Name, a.s.ID, a.d))
		if len(out) == n {
			break
		}
	}
	return out
}

// nearestName is the name of the nearest NPC of a row within reach of (x, z)
// in world (none of skip's row), or "".
func nearestName(gw *entity.World, world int, x, z, reach float64, skip int) string {
	best, name := reach, ""
	if gw == nil {
		return ""
	}
	for _, s := range gw.NPCStates(world) {
		if s.Temporary || int(s.ID) == skip {
			continue
		}
		if d := math.Hypot(float64(s.SpawnX)-x, float64(s.SpawnZ)-z); d <= best {
			best, name = d, s.Name
		}
	}
	return name
}

// portalSays is what the NPC of that name and row already answers a talk with,
// as far as the server knows it: its trips, its bind, its quests, its words,
// its portals on.
func (h *ChatHandler) portalSays(npc *entity.NPC, row int) []string {
	var says []string
	name := npc.Name
	if rs := wayRoutes[name]; len(rs) > 0 {
		var to []string
		for _, r := range rs {
			to = append(to, r.To)
		}
		says = append(says, "a waysender's trips to "+strings.Join(to, ", ")+" (the portal's place is listed with them)")
	} else if strings.HasPrefix(name, "Waysender ") {
		says = append(says, "named a waysender, with no trips")
	}
	if IsSpiritmaster(name) {
		says = append(says, "a spiritmaster's bind (asked first)")
	}
	if entity.IsCoachman(name) {
		says = append(says, "a coachman's menu")
	}
	if entity.IsWarpNPC(name) {
		says = append(says, "a gatekeeper's or harbormaster's worlds")
	}
	if _, ok := npcTextFor(name); ok {
		says = append(says, "a greeter's words (world.npc_text)")
	}
	db := h.camps.db
	if db != nil {
		if rows, err := db.SignTexts(); err == nil && IsSign(npc, rows) {
			says = append(says, "a sign's words: a sign says only them, so the portal would never ask")
		}
		if qs, err := db.Query(`SELECT questid, name FROM quests WHERE enabled <> 0 AND giver = ? COLLATE NOCASE ORDER BY questid LIMIT 5`, name); err == nil {
			for qs.Next() {
				var id int
				var q string
				if qs.Scan(&id, &q) == nil {
					says = append(says, fmt.Sprintf("gives quest %d %q (its steps come first)", id, q))
				}
			}
			qs.Close()
		}
		if on, err := db.EnabledGMPortals(); err == nil {
			for _, p := range on {
				if p.NPCID == row {
					says = append(says, fmt.Sprintf("portal %d to %s, on", p.ID, p.Place))
				}
			}
		}
	}
	return says
}

// markPortalCommand is /markportal [exit [place] | on [id] | off [id] | list | cancel].
func (h *ChatHandler) markPortalCommand(sess *session.Session, player *entity.Player, args []string) {
	const usage = "/markportal (target the entry NPC) | /markportal exit [place] (your target, else where you stand) | /markportal on [id] | off [id] | list | cancel"
	g := h.portals
	if g == nil || h.camps.db == nil {
		h.gmReply(sess, "/markportal: portals are not kept here")
		return
	}
	if len(args) == 0 {
		h.markPortalEntry(sess, player)
		return
	}
	switch strings.ToLower(args[0]) {
	case "exit":
		h.markPortalExit(sess, player, strings.TrimSpace(strings.Join(args[1:], " ")))
	case "on", "off", "confirm":
		h.markPortalSwitch(sess, player, strings.ToLower(args[0]) != "off", args[1:])
	case "list":
		h.markPortalList(sess)
	case "cancel":
		g.mu.Lock()
		e, had := g.entries[sess.AccountID()]
		delete(g.entries, sess.AccountID())
		g.mu.Unlock()
		if had {
			h.gmReply(sess, fmt.Sprintf("/markportal: the entry %s (npc %d) is forgotten", e.Name, e.NPCID))
		} else {
			h.gmReply(sess, "/markportal: you had no entry marked")
		}
	default:
		h.gmReply(sess, "Usage: "+usage)
	}
}

// markPortalEntry is /markportal: the GM's target, an NPC of a row, kept as
// its entry.
func (h *ChatHandler) markPortalEntry(sess *session.Session, player *entity.Player) {
	if player.TargetObjectID == 0 || player.TargetObjectID == 0xFFFFFFFF {
		h.gmReply(sess, "/markportal: target the NPC that is the portal's entry first (then /markportal exit)")
		return
	}
	ent, ok := h.gameWorld.FindEntityByObjectID(sess, player.TargetObjectID)
	npc, isNPC := ent.(*entity.NPC)
	if !ok || !isNPC {
		h.gmReply(sess, "/markportal: your target is no NPC: target the NPC that is the portal's entry")
		return
	}
	st, ok := h.gameWorld.NPCStateOf(npc.ServerID)
	if !ok {
		h.gmReply(sess, "/markportal: your target has left the world")
		return
	}
	if st.Temporary {
		h.gmReply(sess, fmt.Sprintf("/markportal: %s is a spawn with no row (a GM's, a quest's, a trigger's, a pet): a portal's entry must be an NPC of a row", st.Name))
		return
	}
	rec, err := database.NewNPCStore(h.camps.db).GetNPC(int(st.ID))
	if err != nil {
		h.gmReply(sess, fmt.Sprintf("/markportal: %s's row: %v", st.Name, err))
		return
	}
	e := portalEntry{NPCID: rec.ID, Name: rec.NPCName, World: rec.World, X: float64(rec.XCoord), Y: float64(rec.YCoord),
		Z: float64(rec.ZCoord), FacingByte: rec.Facing & 0xFF, Facing: facingOfByte(rec.Facing), Model: rec.ModelID, Level: st.Level,
		Says: h.portalSays(npc, rec.ID)}
	h.portals.mu.Lock()
	h.portals.entries[sess.AccountID()] = e
	h.portals.mu.Unlock()
	h.gmReply(sess, fmt.Sprintf("Portal entry: %s (npc %d) at (%.1f, %.1f, %.1f) in %s. Now /markportal exit [place]: your target, else where you stand.",
		e.Name, e.NPCID, e.X, e.Y, e.Z, placeName(byte(e.World), float32(e.X), float32(e.Y), float32(e.Z))))
	if len(e.Says) > 0 {
		h.gmReply(sess, "It already has: "+strings.Join(e.Says, "; "))
	}
	log.Printf("GM: %s marked portal entry %s (npc %d)", player.Name, e.Name, e.NPCID)
}

// portalExit is a portal's landing as /markportal exit took it.
type portalExit struct {
	By          string // "target" or "stand"
	Name        string // the target's name ("" standing)
	NPCID       int    // the target's npcs row (0: a player, a spawn of no row, standing)
	Kind        string // "npc", "player", ""
	World       int
	X, Y, Z     float64 // the landing
	Facing      float64
	AtX, AtY    float64 // the target's own spot (by target)
	AtZ, AtFace float64
}

// portalExitOf is the landing of /markportal exit: the GM's target's (an NPC:
// in front of it, on dry ground where the world knows the ground; a player:
// its spot) when it has one, else where the GM stands; or why not.
func (h *ChatHandler) portalExitOf(sess *session.Session, player *entity.Player, entry portalEntry) (portalExit, string) {
	if player.TargetObjectID == 0 || player.TargetObjectID == 0xFFFFFFFF {
		return portalExit{By: "stand", World: int(player.World), X: float64(player.X), Y: float64(player.Y), Z: float64(player.Z),
			Facing: float64(player.Facing)}, ""
	}
	ent, ok := h.gameWorld.FindEntityByObjectID(sess, player.TargetObjectID)
	if !ok {
		return portalExit{}, "your target has left: target the landing again, or clear your target to use where you stand"
	}
	switch t := ent.(type) {
	case *entity.Player:
		if t == player {
			return portalExit{By: "stand", World: int(player.World), X: float64(player.X), Y: float64(player.Y), Z: float64(player.Z),
				Facing: float64(player.Facing)}, ""
		}
		return portalExit{By: "target", Kind: "player", Name: t.Name, World: int(t.World), X: float64(t.X), Y: float64(t.Y),
			Z: float64(t.Z), Facing: float64(t.Facing), AtX: float64(t.X), AtY: float64(t.Y), AtZ: float64(t.Z), AtFace: float64(t.Facing)}, ""
	case *entity.NPC:
		st, ok := h.gameWorld.NPCStateOf(t.ServerID)
		if !ok {
			return portalExit{}, "your target has left the world"
		}
		if !st.Temporary && int(st.ID) == entry.NPCID {
			return portalExit{}, fmt.Sprintf("your target is the entry, %s: target the landing's NPC, or clear your target to use where you stand", st.Name)
		}
		x, y, z, face := float64(st.SpawnX), float64(st.SpawnY), float64(st.SpawnZ), float64(st.Facing)
		row := 0
		if !st.Temporary {
			if rec, err := database.NewNPCStore(h.camps.db).GetNPC(int(st.ID)); err == nil {
				x, y, z, face, row = float64(rec.XCoord), float64(rec.YCoord), float64(rec.ZCoord), facingOfByte(rec.Facing), rec.ID
			}
		}
		e := portalExit{By: "target", Kind: "npc", Name: st.Name, NPCID: row, World: int(st.World),
			AtX: x, AtY: y, AtZ: z, AtFace: face, Y: y}
		// in front of it, not inside it, as a waysender's trip lands (wayDests)
		fx, fz, ff := inFrontOf(float32(x), float32(z), float32(face))
		e.X, e.Z, e.Facing = float64(fx), float64(fz), float64(ff)
		if lx, ly, lz, lf, ok := h.gameWorld.ArrivalLanding(st.World, float32(x), float32(y), float32(z), float32(face), bindReach); ok {
			e.X, e.Y, e.Z, e.Facing = float64(lx), float64(ly), float64(lz), float64(lf)
		}
		return e, ""
	}
	return portalExit{}, "your target is neither an NPC nor a player"
}

// portalPlaceMost is the most characters of a place's name kept.
const portalPlaceMost = 60

// markPortalExit is /markportal exit [place]: the landing taken, the portal
// kept (off) and its ticket filed.
func (h *ChatHandler) markPortalExit(sess *session.Session, player *entity.Player, place string) {
	g := h.portals
	g.mu.Lock()
	entry, ok := g.entries[sess.AccountID()]
	g.mu.Unlock()
	if !ok {
		h.gmReply(sess, "/markportal exit: mark the entry first: target its NPC and /markportal")
		return
	}
	exit, why := h.portalExitOf(sess, player, entry)
	if why != "" {
		h.gmReply(sess, "/markportal exit: "+why)
		return
	}
	if exit.World < 0 || exit.World >= WorldCount {
		h.gmReply(sess, fmt.Sprintf("/markportal exit: world %d is no world", exit.World))
		return
	}
	place = cleanMarkNote(place)
	if r := []rune(place); len(r) > portalPlaceMost {
		place = strings.TrimSpace(string(r[:portalPlaceMost]))
	}
	switch {
	case place != "":
	case exit.By == "target":
		place = exit.Name
	default:
		if n := nearestName(h.gameWorld, exit.World, exit.X, exit.Z, 100, entry.NPCID); n != "" {
			place = n
		} else {
			place = placeName(byte(exit.World), float32(exit.X), float32(exit.Y), float32(exit.Z))
		}
	}
	p := database.GMPortal{NPCID: entry.NPCID, NPCName: entry.Name, EntryWorld: entry.World, EntryX: entry.X, EntryY: entry.Y,
		EntryZ: entry.Z, Place: place, World: exit.World, X: round2(exit.X), Y: round2(exit.Y), Z: round2(exit.Z),
		Facing: math.Round(exit.Facing*10000) / 10000, ExitBy: exit.By, ExitName: exit.Name, ExitNPCID: exit.NPCID,
		AccountID: sess.AccountID(), CharName: player.Name}
	who := gmEditOf(sess)
	p, err := h.camps.db.AddGMPortal(p, who, fmt.Sprintf("/markportal exit in the game: %s to %s", entry.Name, place))
	if err != nil {
		h.gmReply(sess, "/markportal exit: not kept: "+err.Error())
		return
	}
	g.mu.Lock()
	delete(g.entries, sess.AccountID())
	g.last[sess.AccountID()] = p.ID
	g.mu.Unlock()

	by := "where you stand"
	if exit.By == "target" {
		by = "your target, " + exit.Name
	}
	h.gmReply(sess, fmt.Sprintf("Portal %d: %s to %s, (%.1f, %.1f, %.1f) in %s, by %s.", p.ID, entry.Name, place, p.X, p.Y, p.Z,
		placeName(byte(p.World), float32(p.X), float32(p.Y), float32(p.Z)), by))

	text := portalTicketText(h.gameWorld, p, entry, exit)
	if h.support == nil || h.support.store == nil {
		h.gmReply(sess, fmt.Sprintf("No ticket: this server keeps no petitions. It is off: /markportal on %d opens it.", p.ID))
		return
	}
	pv := portalViewOf(p, entry, exit)
	t, err := h.support.fileTicketWith(sess, player, portalCategory, portalSubcategory, text, "markportal", time.Now(),
		func(v *serverView) { v.Portal = &pv })
	if err != nil {
		log.Printf("GM: %s's portal %d ticket refused: %v", player.Name, p.ID, err)
		h.gmReply(sess, fmt.Sprintf("Its ticket could not be filed (%s). It is off: /markportal on %d opens it now.", refusalText(err), p.ID))
		return
	}
	if _, err := h.camps.db.SetGMPortalPetition(p.ID, t.ID, who, fmt.Sprintf("/markportal exit: portal %d's ticket", p.ID)); err != nil {
		log.Printf("GM: portal %d's ticket %d not kept on its row: %v", p.ID, t.ID, err)
	}
	h.gmReply(sess, fmt.Sprintf("Filed as Ticket #%d for building. It is off until then: /markportal on %d opens it now.", t.ID, p.ID))
	log.Printf("GM: %s marked portal %d: %s (npc %d) to %s (%.1f, %.1f, %.1f) world %d by %s, ticket %d",
		player.Name, p.ID, entry.Name, entry.NPCID, place, p.X, p.Y, p.Z, p.World, exit.By, t.ID)
}

// round2 is v to two places.
func round2(v float64) float64 { return math.Round(v*100) / 100 }

// The ticket's category: Internal / Portal (support.go csCategories).
const (
	portalCategory    = 9
	portalSubcategory = 91
)

// portalView is a portal's ticket's part of the server view, for a tool to
// read: the row, the entry and the landing.
type portalView struct {
	ID    int64        `json:"id"`
	Place string       `json:"place"`
	Entry portalEntry  `json:"entry"`
	Exit  portalExit   `json:"exit"`
	Row   portalRowRef `json:"row"`
}

// portalRowRef is how the row is switched on.
type portalRowRef struct {
	Table   string `json:"table"`
	Enabled bool   `json:"enabled"`
}

func portalViewOf(p database.GMPortal, entry portalEntry, exit portalExit) portalView {
	exit.X, exit.Y, exit.Z, exit.Facing = p.X, p.Y, p.Z, p.Facing
	return portalView{ID: p.ID, Place: p.Place, Entry: entry, Exit: exit, Row: portalRowRef{Table: "gmportals", Enabled: p.Enabled}}
}

// goFloat is v as a Go literal of a wayPlace's.
func goFloat(v float64) string { return strconv.FormatFloat(round2(v), 'f', -1, 64) }

// portalTicketText is a portal's ticket: everything whoever builds it needs.
func portalTicketText(gw *entity.World, p database.GMPortal, entry portalEntry, exit portalExit) string {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }
	w("Portal %d (/markportal, by %s): a portal to build, OFF until then.", p.ID, p.CharName)
	w("")
	w("ENTRY: %s (npc %d), %s (world %d) at (%.2f, %.2f, %.2f), facing %.4f rad (byte %d), model %d (0x%08X), level %d.",
		entry.Name, entry.NPCID, worldName(byte(entry.World)), entry.World, entry.X, entry.Y, entry.Z, entry.Facing, entry.FacingByte,
		entry.Model, uint32(entry.Model), entry.Level)
	if len(entry.Says) == 0 {
		w("It says nothing else yet: the portal's Yes/No is its only dialogue.")
	} else {
		w("It already has: %s.", strings.Join(entry.Says, "; "))
	}
	if near := portalLandmarks(gw, entry.World, entry.X, entry.Z, entry.NPCID, 4); len(near) > 0 {
		w("Near it: %s.", strings.Join(near, ", "))
	}
	w("")
	switch {
	case exit.By == "target" && exit.Kind == "npc":
		row := "a spawn with no row"
		if exit.NPCID != 0 {
			row = fmt.Sprintf("npc %d", exit.NPCID)
		}
		w("EXIT (by the GM's target, the NPC %s, %s, standing at (%.2f, %.2f, %.2f) facing %.4f rad; the landing is in front of it):",
			exit.Name, row, exit.AtX, exit.AtY, exit.AtZ, exit.AtFace)
	case exit.By == "target":
		w("EXIT (by the GM's target, the player %s, where it stood):", exit.Name)
	default:
		w("EXIT (where the GM stood, facing as it faced):")
	}
	w("%s (world %d) at (%.2f, %.2f, %.2f), facing %.4f rad (byte %d).", placeName(byte(p.World), float32(p.X), float32(p.Y), float32(p.Z)), p.World, p.X, p.Y, p.Z, p.Facing,
		facingByte(p.Facing))
	if near := portalLandmarks(gw, p.World, p.X, p.Z, exit.NPCID, 4); len(near) > 0 {
		w("Near it: %s.", strings.Join(near, ", "))
	}
	if p.World != entry.World {
		w("Another world than the entry's: a client that cannot load world %d is not offered the trip.", p.World)
	}
	w("")
	w("ASKS: \"Do you wish to travel to %s?\" Yes / No (no fee, no level; Yes lands at the exit; the group is not moved).", p.Place)
	w("BUILD: /markportal on %d (or UPDATE gmportals SET enabled = 1 WHERE id = %d: live within %s), or in code (waysender.go wayRoutes):",
		p.ID, p.ID, gmPortalsReread)
	w("  %q: {{To: %q, At: &wayPlace{World: %d, X: %s, Y: %s, Z: %s, Facing: %s}}},", entry.Name, p.Place, p.World,
		goFloat(p.X), goFloat(p.Y), goFloat(p.Z), strconv.FormatFloat(math.Round(p.Facing*100)/100, 'f', -1, 64))
	w("(wayRoutes is by NAME: every NPC named %s would ask; the gmportals row is this one NPC's, npc %d.)", entry.Name, entry.NPCID)
	return strings.TrimRight(b.String(), "\n")
}

// markPortalSwitch is /markportal on|off [id]: a portal switched (no id: the
// GM's last marked).
func (h *ChatHandler) markPortalSwitch(sess *session.Session, player *entity.Player, on bool, args []string) {
	word := "on"
	if !on {
		word = "off"
	}
	g := h.portals
	var id int64
	if len(args) > 0 {
		n, err := strconv.ParseInt(strings.TrimPrefix(args[0], "#"), 10, 64)
		if err != nil || n <= 0 {
			h.gmReply(sess, fmt.Sprintf("/markportal %s <portal id> (/markportal list shows them)", word))
			return
		}
		id = n
	} else {
		g.mu.Lock()
		id = g.last[sess.AccountID()]
		g.mu.Unlock()
		if id == 0 {
			h.gmReply(sess, fmt.Sprintf("/markportal %s <portal id>: you marked none since the server started (/markportal list shows them)", word))
			return
		}
	}
	p, changed, err := h.camps.db.SetGMPortalEnabled(id, on, gmEditOf(sess), fmt.Sprintf("/markportal %s in the game", word))
	switch {
	case errors.Is(err, database.ErrNoGMPortal):
		h.gmReply(sess, fmt.Sprintf("/markportal %s: there is no portal %d", word, id))
		return
	case err != nil:
		h.gmReply(sess, fmt.Sprintf("/markportal %s: %v", word, err))
		return
	}
	g.forget()
	state := "now"
	if !changed {
		state = "already"
	}
	if on {
		h.gmReply(sess, fmt.Sprintf("Portal %d is %s on: %s (npc %d) asks \"Do you wish to travel to %s?\"", p.ID, state, p.NPCName, p.NPCID, p.Place))
	} else {
		h.gmReply(sess, fmt.Sprintf("Portal %d is %s off: %s (npc %d) no longer offers %s", p.ID, state, p.NPCName, p.NPCID, p.Place))
	}
	log.Printf("GM: %s switched portal %d %s (changed %v)", player.Name, p.ID, word, changed)
}

// markPortalList is /markportal list: the last portals marked.
func (h *ChatHandler) markPortalList(sess *session.Session) {
	ps, err := h.camps.db.RecentGMPortals(10)
	if err != nil {
		h.gmReply(sess, "/markportal list: "+err.Error())
		return
	}
	if len(ps) == 0 {
		h.gmReply(sess, "No portals marked yet.")
		return
	}
	for _, p := range ps {
		state := "off"
		if p.Enabled {
			state = "on"
		}
		ticket := ""
		if p.Petition != 0 {
			ticket = fmt.Sprintf(", ticket #%d", p.Petition)
		}
		h.gmReply(sess, fmt.Sprintf("Portal %d (%s): %s (npc %d) to %s, (%.0f, %.0f, %.0f) in %s%s", p.ID, state, p.NPCName, p.NPCID,
			p.Place, p.X, p.Y, p.Z, placeName(byte(p.World), float32(p.X), float32(p.Y), float32(p.Z)), ticket))
	}
}

// portalDests are the trips the portals switched on at an NPC offer this
// session: in a world its client loads.
func portalDests(sess *session.Session, ps []database.GMPortal, via string) []entity.CoachDest {
	var out []entity.CoachDest
	for _, p := range ps {
		if p.World < 0 || p.World >= WorldCount || !CanLoadWorld(sess, uint8(p.World)) {
			continue
		}
		out = append(out, entity.CoachDest{Name: p.Place, X: float32(p.X), Y: float32(p.Y), Z: float32(p.Z), Facing: float32(p.Facing),
			World: uint8(p.World), Fee: p.Fee, MinLevel: p.MinLevel, Via: via})
	}
	return out
}

// portalTalk asks a player who talks to a portal's entry (a portal switched
// on at the NPC's row) whether to travel: one place as Yes or No, several (a
// waysender's own trips among them) as a waysender's list. It reports whether
// the NPC was one.
func (h *InteractHandler) portalTalk(sess *session.Session, player *entity.Player, npc *entity.NPC) bool {
	if h.portals == nil || npc.Temporary {
		return false
	}
	ps := h.portals.enabled(uint32(npc.ServerID))
	if len(ps) == 0 {
		return false
	}
	dests := portalDests(sess, ps, npc.Name)
	if len(dests) == 0 {
		log.Printf("Portals: %s (npc %d) has portals on, none to a world %s's client loads", npc.Name, npc.ServerID, player.Name)
		return false
	}
	var allowed []entity.CoachDest
	barred := ""
	for _, d := range dests {
		if d.MinLevel > 0 && player.Level < d.MinLevel {
			if barred == "" {
				barred = d.Name
			}
			continue
		}
		allowed = append(allowed, d)
	}
	if IsWaysender(npc.Name) {
		way, _, _ := wayDests(h.gameWorld, sess, npc.Name, player)
		allowed = append(way, allowed...)
	}
	if len(allowed) == 0 {
		forgetDialogs(player)
		h.sendDialogueBox(sess, tooWeak(barred))
		return true
	}
	forgetDialogs(player)
	player.PendingCoachDests = allowed
	if len(allowed) == 1 {
		player.PendingYesNo = true
		h.sendOptionBox(sess, wayAskOne(allowed[0]), []string{"Yes", "No"})
	} else {
		h.sendCoachPage(sess, player)
	}
	log.Printf("Portals: %s (npc %d) offers %s %d trips", npc.Name, npc.ServerID, player.Name, len(allowed))
	return true
}
