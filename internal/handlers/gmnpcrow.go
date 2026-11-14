package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// gmnpcrow.go — a GM's /delete and /clone (the owner, 2026-11-12: "add
// /delete for the mob you target (GM only), /clone target at the spot the GM
// is standing and facing"; then "/place <mob name>", and "players with the
// /fly /speed can use also"). A GM's (acctlevel 1 or more) and a sub GM's
// (devcmds.go's subGMCommands, as /fly and /speed), as /delete, #delete or a
// PS2's "/t delete" (and the same for clone and place):
//
//   - /delete: the GM's target, an NPC of a row, deleted for good: its npcs
//     row and the rows keyed by it (database/gmnpcrow.go: npcrespawn,
//     npcwander, npctitle, signtext, npcpaths, campspawns), out of the world
//     at once. A row a quest step, a quest's wall or a GM portal names is
//     refused. /delete undo puts the last delete back, any GM's (the owner, 2026-11-12), in the world at
//     once; the GM panel's audit log reverts each line too.
//   - /clone: a copy of the target's row (every column, its npc_id, so a
//     merchant sells the same) where the GM stands, facing as the GM faces,
//     in the GM's world, with its respawn time, wander, title and sign text;
//     in the world at once. /delete on the copy takes it away.
//   - /place <name> (quotes may go round it): as /clone, of a row of that
//     name (database's PlaceSourceRow: one in the GM's world if any, of the
//     most common body and level); no NPC of the name: refused, close names
//     suggested. Any world's name: one only in another world is copied into
//     the GM's; a row's name not standing in the world now too. A spawn-only
//     name's (npcspawnonly: a quest's or trigger's spawn copies its original,
//     as filth squid's): the new row stands all the same, by its row of table
//     npcstands (database/npcstands.go; the owner, 2026-11-13, of GMs refused
//     /place filth squid in the Plane of Disease), which /delete takes with
//     it, /delete undo puts back and /clone copies; every other row and spawn
//     of the name is as before.
//   - /move (the owner, 2026-11-12: "they target a mob and it moves the mob
//     there and sets spawn point there also"): the target's row's spawn point,
//     facing and world set to the GM's, in the world at once; /move undo puts
//     the GM's last move back (refused when the row changed since).
//   - a wander after /move, /clone or /place's name ("/clone roamer to make a
//     roamer mob"): roamer (npcwander radius 300, a far roamer's long legs),
//     roamer <radius>, wander <radius>, or stay (the row's own wander taken
//     away: it moves as its name's or kind's rule says), read in at once.
//
// Refused (entity/gmnpcrow.go): a player, a spawn of no row (a GM's, a
// quest's, a trigger's, an event's), a dungeon instance's copy, a pet, a
// mercenary, a familiar, a genie, a portal, a spawn-only template, anything in
// an instance. Every row's change has its gm_audit line (via "game").

// npcRowWords are /delete's and /clone's words, lower case.
var npcRowWords = map[string]bool{"delete": true, "clone": true, "place": true, "move": true, "roamer": true}

// npcRowTell reads a PS2's "/t delete" or "/t clone" (its keyboards type no
// #, and it keeps an unknown /command to itself): the word and what follows
// ("undo"), or false for a tell.
func npcRowTell(name, body string) (string, string, bool) {
	w := strings.ToLower(name)
	b := strings.TrimSpace(body)
	if !npcRowWords[w] {
		return "", "", false
	}
	if w == "place" {
		return w, b, b != ""
	}
	b = strings.ToLower(b)
	fields := strings.Fields(b)
	switch {
	case b == "":
	case (w == "delete" || w == "move") && b == "undo":
	case w == "roamer":
		// "/t roamer", "/t roamer 500", "/t roamer stay"
		if _, err := strconv.ParseFloat(b, 64); b != "stay" && err != nil {
			return "", "", false
		}
	case w != "delete":
		if _, rest, bad := wanderOption(fields); bad != "" || len(rest) > 0 {
			return "", "", false
		}
	default:
		return "", "", false
	}
	return w, b, true
}

// wanderOption reads a wander option off the end of args: "roamer" (radius
// database.RoamerRadius), "roamer <radius>", "wander <radius>" or "stay"
// (its own wander taken away). It returns the option (nil for none), the
// args before it, and what is wrong with a radius ("" when nothing is).
func wanderOption(args []string) (*database.NPCWanderSet, []string, string) {
	n := len(args)
	if n == 0 {
		return nil, args, ""
	}
	last := strings.ToLower(args[n-1])
	switch last {
	case "roamer":
		return &database.NPCWanderSet{Radius: database.RoamerRadius, Note: "a GM's roamer"}, args[:n-1], ""
	case "stay":
		return &database.NPCWanderSet{Clear: true}, args[:n-1], ""
	}
	if n < 2 {
		return nil, args, ""
	}
	word := strings.ToLower(args[n-2])
	if word != "roamer" && word != "wander" {
		return nil, args, ""
	}
	r, err := strconv.ParseFloat(last, 64)
	if err != nil || r < 1 || r > database.MaxWanderRadius || math.IsNaN(r) {
		return nil, args, fmt.Sprintf("%s's radius is 1 to %d", word, database.MaxWanderRadius)
	}
	note := "a GM's wander"
	if word == "roamer" {
		note = "a GM's roamer"
	}
	return &database.NPCWanderSet{Radius: r, Note: note}, args[:n-2], ""
}

// wanderWords says a wander option to the GM.
func wanderWords(w *database.NPCWanderSet) string {
	switch {
	case w == nil:
		return ""
	case w.Clear:
		return "its own wander taken away (it moves as its name's or its kind's rule says)"
	case w.Stand:
		return "it stands on its spawn until pulled, whatever its name's or its kind's rule (/roamer to roam again)"
	case w.Radius > 70:
		return fmt.Sprintf("a roamer, %g from its spawn, on long legs", w.Radius)
	}
	return fmt.Sprintf("wandering %g from its spawn", w.Radius)
}

// wanderUsage is the wander options, for a usage line.
const wanderUsage = "add roamer (wanders 300), roamer <radius>, wander <radius>, or stay (its own wander taken away)"

// applyWander gives a new row id the wander w (clone and place), audited,
// read into the world; a line to add to the reply.
func (h *ChatHandler) applyWander(sess *session.Session, id int64, w *database.NPCWanderSet, detail string) string {
	if w == nil {
		return ""
	}
	changed, err := h.camps.db.SetNPCWanderAudited(id, *w, gmEditOf(sess), detail)
	if err != nil {
		return " Its wander is not set: " + err.Error() + "."
	}
	if changed {
		h.reloadNPCExtras([]string{"npcwander"})
	}
	return " " + upperFirst(wanderWords(w)) + "." + h.wanderNote(id)
}

// wanderNote is " (it stands: why)" when the world's rules keep row id from
// wandering whatever its own wander says, else "".
func (h *ChatHandler) wanderNote(id int64) string {
	st, ok := h.gameWorld.NPCStateOf(entity.EntityID(id))
	if !ok || strings.HasPrefix(st.Movement, "it wanders") {
		return ""
	}
	return " (but it does not wander: " + st.Movement + ")"
}

func upperFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// npcRowTarget is the GM's target as an NPC of a row a GM may delete or
// clone, or false, said why.
func (h *ChatHandler) npcRowTarget(sess *session.Session, player *entity.Player, cmd string) (entity.EntityID, *database.NPCRecord, bool) {
	if player.TargetObjectID == 0 || player.TargetObjectID == 0xFFFFFFFF {
		h.gmReply(sess, cmd+": target the creature first")
		return 0, nil, false
	}
	ent, ok := h.gameWorld.FindEntityByObjectID(sess, player.TargetObjectID)
	if !ok {
		h.gmReply(sess, cmd+": your target has left the world")
		return 0, nil, false
	}
	if p, isPlayer := ent.(*entity.Player); isPlayer {
		h.gmReply(sess, fmt.Sprintf("%s: %s is a player, not a creature", cmd, p.Name))
		return 0, nil, false
	}
	npc, isNPC := ent.(*entity.NPC)
	if !isNPC {
		h.gmReply(sess, cmd+": your target is no creature")
		return 0, nil, false
	}
	name, why := h.gameWorld.RowNPCRefusal(npc.ServerID)
	if why != "" {
		if name == "" {
			name = "your target"
		}
		h.gmReply(sess, fmt.Sprintf("%s: %s %s: nothing done.", cmd, name, why))
		return 0, nil, false
	}
	rec, err := database.NewNPCStore(h.camps.db).GetNPC(int(npc.ServerID))
	if errors.Is(err, sql.ErrNoRows) {
		h.gmReply(sess, fmt.Sprintf("%s: %s has no NPC row: nothing done.", cmd, name))
		return 0, nil, false
	}
	if err != nil {
		h.gmReply(sess, fmt.Sprintf("%s: %s's row: %v", cmd, name, err))
		return 0, nil, false
	}
	return npc.ServerID, rec, true
}

// reloadNPCExtras reads into the world again the tables of a row's extras
// that changed (database/gmnpcrow.go's npcRowExtras), but npcpaths, which the
// callers set or clear by the one NPC.
func (h *ChatHandler) reloadNPCExtras(tables []string) {
	db := h.camps.db
	for _, t := range tables {
		var err error
		switch t {
		case "npcrespawn":
			_, err = h.gameWorld.LoadRespawnRules(db)
		case "npcwander":
			_, err = h.gameWorld.LoadRowWander(db)
		case "npctitle":
			_, err = h.gameWorld.LoadNPCTitles(db)
		case "signtext":
			database.InvalidateSignText()
		case "campspawns":
			_, err = h.gameWorld.LoadCampSpawns(db)
		case "npcstands":
			_, _, err = h.gameWorld.LoadSpawnOnly(db)
		}
		if err != nil {
			log.Printf("GM: %s not read again: %v", t, err)
		}
	}
}

// standsWords tells the GM a new row (/place, /clone) of a spawn-only name
// stands (its npcstands row among the tables copied or made), or "".
func standsWords(tables []string) string {
	for _, t := range tables {
		if t == "npcstands" {
			return " It is a standing copy of a quest's or trigger's spawn (npcspawnonly): this row stands in the world " +
				"and respawns as its own; the quests' and triggers' spawns of the name come as before."
		}
	}
	return ""
}

// upsertRowNPC puts npcs row id into the world as its row makes it now.
func (h *ChatHandler) upsertRowNPC(id int64) (*database.NPCRecord, error) {
	npcs := database.NewNPCStore(h.camps.db)
	rec, err := npcs.GetNPC(int(id))
	if err != nil {
		return nil, err
	}
	class, _ := npcs.NPCClassOf(rec.NPCName)
	_, err = h.gameWorld.UpsertNPC(rec, class)
	return rec, err
}

// deleteCommand is /delete or /delete undo (the file's doc).
func (h *ChatHandler) deleteCommand(sess *session.Session, player *entity.Player, args []string) {
	if h.camps.db == nil {
		h.gmReply(sess, "/delete: no database here")
		return
	}
	if len(args) == 1 && strings.EqualFold(args[0], "undo") {
		h.deleteUndo(sess, player)
		return
	}
	if len(args) > 0 {
		h.gmReply(sess, "Usage: /delete - your target creature's row deleted for good; /delete undo puts the last delete back, any GM's, the one before at each press")
		return
	}
	id, rec, ok := h.npcRowTarget(sess, player, "/delete")
	if !ok {
		return
	}
	what := fmt.Sprintf("%s (row %d)", rec.NPCName, id)
	d, err := h.camps.db.DeleteNPCRowAudited(int64(id), gmEditOf(sess), what)
	var inUse *database.ErrNPCRowInUse
	switch {
	case errors.As(err, &inUse):
		h.gmReply(sess, fmt.Sprintf("/delete: %s is not deleted: %v.", what, err))
		return
	case errors.Is(err, sql.ErrNoRows):
		h.gmReply(sess, fmt.Sprintf("/delete: %s has no row any more.", what))
		return
	case err != nil:
		h.gmReply(sess, "/delete: nothing deleted: "+err.Error())
		return
	}
	h.gameWorld.RemoveNPC(id)
	for _, t := range d.Extras {
		if t == "npcpaths" {
			h.gameWorld.ClearNPCPath(id)
		}
	}
	h.reloadNPCExtras(d.Extras)
	line := "Deleted " + what
	if len(d.Extras) > 0 {
		line += ", with its " + strings.Join(d.Extras, ", ")
	}
	h.gmReply(sess, line+". /delete undo puts it back.")
	log.Printf("GM: %s deleted NPC %s at (%.1f, %.1f, %.1f) world %d, extras %v", player.Name, what,
		rec.XCoord, rec.YCoord, rec.ZCoord, rec.World, d.Extras)
}

// deleteUndo is /delete undo: the GM's last delete put back, in the world at
// once.
func (h *ChatHandler) deleteUndo(sess *session.Session, player *entity.Player) {
	r, ok, err := h.camps.db.UndoLastNPCDelete(sess.AccountID(), gmEditOf(sess))
	switch {
	case err != nil:
		h.gmReply(sess, "/delete undo: "+err.Error())
		return
	case !ok:
		h.gmReply(sess, "No delete is left to put back.")
		return
	}
	h.reloadNPCExtras(r.Tables)
	live := ""
	if _, err := h.upsertRowNPC(r.ID); err != nil {
		live = fmt.Sprintf(" It is not in the world yet (at the next start): %v.", err)
	}
	for _, t := range r.Tables {
		if t != "npcpaths" {
			continue
		}
		recs, _, err := database.NewNPCPathStore(h.camps.db).All()
		for _, pr := range recs {
			if int64(pr.NPCID) == r.ID && err == nil {
				if p, err := entity.PathFromRecord(pr); err == nil {
					_ = h.gameWorld.SetNPCPath(entity.EntityID(r.ID), p)
				}
			}
		}
	}
	more := ""
	if n, err := h.camps.db.NPCDeletesToUndo(sess.AccountID()); err == nil && n > 0 {
		// each press the one before (the owner, 2026-11-12: "keep going if press again")
		more = fmt.Sprintf(" /delete undo again puts back the one before (%d more).", n)
	}
	whose := ""
	if r.By != "" {
		whose = " (deleted by " + r.By + ")"
	}
	h.gmReply(sess, fmt.Sprintf("Put back %s (row %d)%s with %s.%s%s", r.Name, r.ID, whose, strings.Join(r.Tables, ", "), live, more))
	log.Printf("GM: %s put back NPC %d (%s): %v", player.Name, r.ID, r.Name, r.Tables)
}

// cloneCommand is /clone (the file's doc).
func (h *ChatHandler) cloneCommand(sess *session.Session, player *entity.Player, args []string) {
	if h.camps.db == nil {
		h.gmReply(sess, "/clone: no database here")
		return
	}
	wander, rest, bad := wanderOption(args)
	if bad != "" || len(rest) > 0 {
		h.gmReply(sess, "Usage: /clone - a copy of your target creature where you stand, facing as you face; "+wanderUsage)
		return
	}
	id, rec, ok := h.npcRowTarget(sess, player, "/clone")
	if !ok {
		return
	}
	at := database.NPCClone{World: int(player.World), X: float64(player.X), Y: float64(player.Y), Z: float64(player.Z),
		Facing: facingByte(float64(player.Facing))}
	what := fmt.Sprintf("%s (row %d)", rec.NPCName, id)
	nid, copied, err := h.camps.db.CloneNPCRowAudited(int64(id), at, gmEditOf(sess), what)
	if err != nil {
		h.gmReply(sess, "/clone: nothing made: "+err.Error())
		return
	}
	h.reloadNPCExtras(copied)
	line := fmt.Sprintf("Cloned %s as row %d", rec.NPCName, nid)
	if len(copied) > 0 {
		line += ", with its " + strings.Join(copied, ", ")
	}
	line += "."
	if _, err := h.upsertRowNPC(nid); err != nil {
		line += fmt.Sprintf(" It is not in the world yet (at the next start): %v.", err)
	}
	line += h.applyWander(sess, nid, wander, fmt.Sprintf("/clone in the game: %s, a copy of NPC %d", what, id))
	line += standsWords(copied)
	if player.Flying {
		line += " (you fly: it stands in the air)"
	}
	h.gmReply(sess, line+" /delete on it takes it away.")
	log.Printf("GM: %s cloned NPC %d (%s) as %d at (%.1f, %.1f, %.1f) world %d facing %d", player.Name, id, rec.NPCName, nid,
		at.X, at.Y, at.Z, at.World, at.Facing)
}

// closeNames is up to 10 of names sharing a word of 3 letters or more with q
// (any case), for a name no NPC has.
func closeNames(names []string, q string) []string {
	var words []string
	for _, w := range strings.Fields(strings.ToLower(q)) {
		if len(w) >= 3 {
			words = append(words, w)
		}
	}
	var out []string
	for _, n := range names {
		ln := strings.ToLower(n)
		for _, w := range words {
			if strings.Contains(ln, w) {
				out = append(out, n)
				break
			}
		}
		if len(out) == 10 {
			break
		}
	}
	return out
}

// placeCommand is /place <name> (the file's doc).
func (h *ChatHandler) placeCommand(sess *session.Session, player *entity.Player, args []string) {
	if h.camps.db == nil {
		h.gmReply(sess, "/place: no database here")
		return
	}
	q, wander, bad := h.placeArgs(args)
	if q == "" || bad != "" {
		h.gmReply(sess, `Usage: /place "<creature name>" - a new one where you stand, facing as you face, a copy of a row of that name `+
			`(of any world; the quotes may be left off); `+wanderUsage)
		return
	}
	name, ok := h.pickNPCName(sess, q)
	if !ok {
		if n, found := h.camps.db.NPCRowName(q); found {
			name, ok = n, true // a row's name not standing in the world now
		}
	}
	if !ok {
		if len(searchNames(h.gameWorld.NPCNames(), q)) == 0 {
			line := fmt.Sprintf("/place: no NPC is called %q: nothing made.", q)
			if near := closeNames(h.gameWorld.NPCNames(), q); len(near) > 0 {
				line += " Close: " + strings.Join(near, ", ")
			}
			h.gmReply(sess, line)
		}
		return
	}
	// a spawn-only name (npcspawnonly: a quest's or a trigger's spawn copies
	// its original): the placed row stands all the same (table npcstands),
	// every other row and spawn of the name as before (the owner, 2026-11-13)
	stands := ""
	if h.gameWorld.SpawnOnlyNamed(name) {
		stands = fmt.Sprintf("a GM's /place of %s, a quest's or trigger's spawn (npcspawnonly): this row stands", name)
	}
	src, err := h.camps.db.PlaceSourceRow(name, int(player.World))
	if errors.Is(err, sql.ErrNoRows) {
		h.gmReply(sess, fmt.Sprintf("/place: no NPC row is called %q (a GM's or an event's spawn has none): nothing made.", name))
		return
	}
	if err != nil {
		h.gmReply(sess, "/place: "+err.Error())
		return
	}
	at := database.NPCClone{World: int(player.World), X: float64(player.X), Y: float64(player.Y), Z: float64(player.Z),
		Facing: facingByte(float64(player.Facing))}
	nid, copied, err := h.camps.db.PlaceNPCRowAudited(src, at, gmEditOf(sess), fmt.Sprintf("%s (row %d)", name, src), stands)
	if err != nil {
		h.gmReply(sess, "/place: nothing made: "+err.Error())
		return
	}
	h.reloadNPCExtras(copied)
	line := fmt.Sprintf("Placed %s as row %d (a copy of row %d)", name, nid, src)
	if len(copied) > 0 {
		line += ", with its " + strings.Join(copied, ", ")
	}
	line += "."
	if _, err := h.upsertRowNPC(nid); err != nil {
		line += fmt.Sprintf(" It is not in the world yet (at the next start): %v.", err)
	}
	line += h.applyWander(sess, nid, wander, fmt.Sprintf("/place in the game: %s (row %d), a copy of NPC %d", name, src, src))
	line += standsWords(copied)
	if player.Flying {
		line += " (you fly: it stands in the air)"
	}
	h.gmReply(sess, line+" /delete on it takes it away.")
	log.Printf("GM: %s placed %s as %d (a copy of %d) at (%.1f, %.1f, %.1f) world %d facing %d", player.Name, name, nid, src,
		at.X, at.Y, at.Z, at.World, at.Facing)
}

// placeArgs reads /place's words: the name, in quotes or not, and a wander
// option after it (wanderOption); bad says what is wrong with a radius. A
// name an NPC has whole ("... stay") is a name, no option.
func (h *ChatHandler) placeArgs(args []string) (string, *database.NPCWanderSet, string) {
	line := strings.TrimSpace(strings.Join(args, " "))
	if line == "" {
		return "", nil, ""
	}
	if q := line[0]; q == '"' || q == '\'' {
		end := strings.IndexByte(line[1:], q)
		if end < 0 {
			return strings.TrimSpace(line[1:]), nil, ""
		}
		name := strings.TrimSpace(line[1 : 1+end])
		w, rest, bad := wanderOption(strings.Fields(line[2+end:]))
		if bad == "" && len(rest) > 0 {
			bad = "unknown words after the name: " + strings.Join(rest, " ")
		}
		return name, w, bad
	}
	if h.npcNamed(line) {
		return line, nil, ""
	}
	w, rest, bad := wanderOption(strings.Fields(line))
	if bad != "" {
		return "", nil, bad
	}
	return strings.Trim(strings.Join(rest, " "), `"'`), w, ""
}

// moveCommand is /move [roamer [<radius>] | wander <radius> | stay] or /move
// undo (the owner, 2026-11-12: "/move for GMs: they target a mob and it moves
// the mob there and sets spawn point there also"): the target's npcs row's
// spawn point, facing and world set to the GM's, and its wander if asked, in
// the world at once, audited; the same targets and ranks as /clone.
func (h *ChatHandler) moveCommand(sess *session.Session, player *entity.Player, args []string) {
	if h.camps.db == nil {
		h.gmReply(sess, "/move: no database here")
		return
	}
	if len(args) == 1 && strings.EqualFold(args[0], "undo") {
		h.moveUndo(sess, player)
		return
	}
	wander, rest, bad := wanderOption(args)
	if bad != "" || len(rest) > 0 {
		h.gmReply(sess, "Usage: /move - your target creature's spawn point moved where you stand, facing as you face; "+
			wanderUsage+"; /move undo puts your last back")
		return
	}
	id, rec, ok := h.npcRowTarget(sess, player, "/move")
	if !ok {
		return
	}
	at := database.NPCClone{World: int(player.World), X: float64(player.X), Y: float64(player.Y), Z: float64(player.Z),
		Facing: facingByte(float64(player.Facing))}
	what := fmt.Sprintf("%s (row %d)", rec.NPCName, id)
	m, err := h.camps.db.MoveNPCRowAudited(int64(id), at, wander, gmEditOf(sess), what)
	if errors.Is(err, sql.ErrNoRows) {
		h.gmReply(sess, fmt.Sprintf("/move: %s has no row any more.", what))
		return
	}
	if err != nil {
		h.gmReply(sess, "/move: nothing moved: "+err.Error())
		return
	}
	if m.Wander {
		h.reloadNPCExtras([]string{"npcwander"})
	}
	line := fmt.Sprintf("Moved %s here (from %.0f, %.0f, %.0f", what, m.From.X, m.From.Y, m.From.Z)
	if m.From.World != at.World {
		line += fmt.Sprintf(" in world %d", m.From.World)
	}
	line += ")."
	if wander != nil {
		line += " " + upperFirst(wanderWords(wander)) + "."
	}
	if _, err := h.upsertRowNPC(int64(id)); err != nil {
		line += fmt.Sprintf(" It is not moved in the world yet (at the next start): %v.", err)
	} else {
		line += h.wanderNote(int64(id))
	}
	if h.gameWorld.HasNPCPath(id) {
		line += " It walks a path (npcpaths), which is where it was: change the path in the GM panel."
	}
	if player.Flying {
		line += " (you fly: it stands in the air)"
	}
	h.gmReply(sess, line+" /move undo puts it back.")
	log.Printf("GM: %s moved NPC %s from (%.1f, %.1f, %.1f) world %d to (%.1f, %.1f, %.1f) world %d facing %d, wander %+v",
		player.Name, what, m.From.X, m.From.Y, m.From.Z, m.From.World, at.X, at.Y, at.Z, at.World, at.Facing, wander)
}

// moveUndo is /move undo: the GM's last move put back, in the world at once.
func (h *ChatHandler) moveUndo(sess *session.Session, player *entity.Player) {
	r, ok, err := h.camps.db.UndoLastNPCMove(sess.AccountID(), gmEditOf(sess))
	switch {
	case err != nil:
		h.gmReply(sess, "/move undo: "+err.Error())
		return
	case !ok:
		h.gmReply(sess, "You have moved nothing that is not back already.")
		return
	}
	if r.Wander {
		h.reloadNPCExtras([]string{"npcwander"})
	}
	line := fmt.Sprintf("Put %s (row %d) back where it stood", r.Name, r.ID)
	if r.Wander {
		line += ", with its wander as it was"
	}
	line += "."
	if _, err := h.upsertRowNPC(r.ID); err != nil {
		line += fmt.Sprintf(" It is not moved in the world yet (at the next start): %v.", err)
	}
	h.gmReply(sess, line)
	log.Printf("GM: %s put NPC %d (%s) back from a /move", player.Name, r.ID, r.Name)
}

// roamerCommand is /roamer [<radius> | off] (the owner, 2026-11-12: "/roamer
// GM command to make existing mobs roam"): the targeted creature's own row
// wanders as a roamer, database.RoamerRadius from its spawn (or <radius>), in
// place, its spawn point kept; "off" (stop, none, no, stay) stands it on its spawn
// whatever its name's rule (a negative npcwander radius, 2026-11-14). The same
// ranks, refusals and audit as /move's wander option.
func (h *ChatHandler) roamerCommand(sess *session.Session, player *entity.Player, args []string) {
	opt := []string{"roamer"}
	switch {
	case len(args) == 1 && roamerOff[strings.ToLower(args[0])]:
		// the owner, 2026-11-14: "/roamer off is not working for jib": it stands on
		// its spawn, its name's rule notwithstanding (npcwander's negative radius)
		id, rec, ok := h.npcRowTarget(sess, player, "roamer")
		if !ok {
			return
		}
		said := h.applyWander(sess, int64(id), &database.NPCWanderSet{Stand: true, Note: "a GM's /roamer off"},
			fmt.Sprintf("/roamer off on %s (row %d)", rec.NPCName, id))
		h.gmReply(sess, fmt.Sprintf("%s (row %d):%s", rec.NPCName, id, said))
		return
	case len(args) == 1:
		opt = []string{"roamer", args[0]}
	case len(args) > 1:
		opt = args
	}
	w, rest, bad := wanderOption(opt)
	if bad != "" || w == nil || len(rest) > 0 {
		if bad == "" {
			bad = "use /roamer, /roamer <radius>, or /roamer off (it stands on its spawn)"
		}
		h.gmReply(sess, "roamer: "+bad)
		return
	}
	id, rec, ok := h.npcRowTarget(sess, player, "roamer")
	if !ok {
		return
	}
	said := h.applyWander(sess, int64(id), w, fmt.Sprintf("/roamer on %s (row %d)", rec.NPCName, id))
	h.gmReply(sess, fmt.Sprintf("%s (row %d):%s", rec.NPCName, id, said))
}

// roamerOff are /roamer's words for "this creature stops roaming".
var roamerOff = map[string]bool{"off": true, "stop": true, "none": true, "no": true, "stay": true}

// targetsCreature is whether the player's target is a creature (an NPC).
func (h *ChatHandler) targetsCreature(sess *session.Session, player *entity.Player) bool {
	if player == nil || player.TargetObjectID == 0 || player.TargetObjectID == 0xFFFFFFFF {
		return false
	}
	ent, ok := h.gameWorld.FindEntityByObjectID(sess, player.TargetObjectID)
	if !ok {
		return false
	}
	_, isNPC := ent.(*entity.NPC)
	return isNPC
}

// keepNPCSize saves the size a GM's /size gave its target creature to the
// creature's npcs row, so it keeps it over respawns and restarts (the owner,
// 2026-11-13). A player's size, a creature with no row (a pet, a summoned
// add), or no database: the running world's alone, as before.
func (h *ChatHandler) keepNPCSize(sess *session.Session, player *entity.Player) {
	if h.camps.db == nil || player.TargetObjectID == 0 || player.TargetObjectID == 0xFFFFFFFF {
		return
	}
	ent, ok := h.gameWorld.FindEntityByObjectID(sess, player.TargetObjectID)
	if !ok {
		return
	}
	npc, isNPC := ent.(*entity.NPC)
	if !isNPC {
		return
	}
	if _, why := h.gameWorld.RowNPCRefusal(npc.ServerID); why != "" {
		h.gmReply(sess, fmt.Sprintf("%s's size is not kept over its respawns: it %s.", npc.Name, why))
		return
	}
	size := float64(h.gameWorld.NPCSize(npc.ServerID))
	what := fmt.Sprintf("%s (row %d)", npc.Name, npc.ServerID)
	if err := h.camps.db.SetNPCRowSizeAudited(int64(npc.ServerID), size, gmEditOf(sess), what); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			h.gmReply(sess, "/size: not kept over a restart: "+err.Error())
		}
		return
	}
	h.gmReply(sess, fmt.Sprintf("%s keeps size %.2f over respawns and restarts.", npc.Name, size))
}
