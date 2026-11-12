package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
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
//     refused. /delete undo puts the GM's last delete back, in the world at
//     once; the GM panel's audit log reverts each line too.
//   - /clone: a copy of the target's row (every column, its npc_id, so a
//     merchant sells the same) where the GM stands, facing as the GM faces,
//     in the GM's world, with its respawn time, wander, title and sign text;
//     in the world at once. /delete on the copy takes it away.
//   - /place <name> (quotes may go round it): as /clone, of a row of that
//     name (database's PlaceSourceRow: one in the GM's world if any, of the
//     most common body and level); no NPC of the name: refused, close names
//     suggested.
//
// Refused (entity/gmnpcrow.go): a player, a spawn of no row (a GM's, a
// quest's, a trigger's, an event's), a dungeon instance's copy, a pet, a
// mercenary, a familiar, a genie, a portal, a spawn-only template, anything in
// an instance. Every row's change has its gm_audit line (via "game").

// npcRowWords are /delete's and /clone's words, lower case.
var npcRowWords = map[string]bool{"delete": true, "clone": true, "place": true}

// npcRowTell reads a PS2's "/t delete" or "/t clone" (its keyboards type no
// #, and it keeps an unknown /command to itself): the word and what follows
// ("undo"), or false for a tell.
func npcRowTell(name, body string) (string, string, bool) {
	w := strings.ToLower(name)
	b := strings.TrimSpace(body)
	if w != "place" {
		b = strings.ToLower(b)
	}
	if !npcRowWords[w] || w != "place" && b != "" && !(w == "delete" && b == "undo") || w == "place" && b == "" {
		return "", "", false
	}
	return w, b, true
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
		}
		if err != nil {
			log.Printf("GM: %s not read again: %v", t, err)
		}
	}
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
		h.gmReply(sess, "Usage: /delete - your target creature's row deleted for good; /delete undo puts your last back")
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
		h.gmReply(sess, "You have deleted nothing that is not back already.")
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
	h.gmReply(sess, fmt.Sprintf("Put back %s (row %d) with %s.%s", r.Name, r.ID, strings.Join(r.Tables, ", "), live))
	log.Printf("GM: %s put back NPC %d (%s): %v", player.Name, r.ID, r.Name, r.Tables)
}

// cloneCommand is /clone (the file's doc).
func (h *ChatHandler) cloneCommand(sess *session.Session, player *entity.Player, args []string) {
	if h.camps.db == nil {
		h.gmReply(sess, "/clone: no database here")
		return
	}
	if len(args) > 0 {
		h.gmReply(sess, "Usage: /clone - a copy of your target creature where you stand, facing as you face")
		return
	}
	id, rec, ok := h.npcRowTarget(sess, player, "/clone")
	if !ok {
		return
	}
	at := database.NPCClone{World: int(player.World), X: float64(player.X), Y: float64(player.Y), Z: float64(player.Z),
		Facing: facingByte(float64(player.Facing))}
	nid, copied, err := h.camps.db.CloneNPCRowAudited(int64(id), at, gmEditOf(sess), fmt.Sprintf("%s (row %d)", rec.NPCName, id))
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
	q := strings.Trim(strings.TrimSpace(strings.Join(args, " ")), `"'`)
	q = strings.TrimSpace(q)
	if q == "" {
		h.gmReply(sess, `Usage: /place "<creature name>" - a new one where you stand, facing as you face, a copy of a row of that name`)
		return
	}
	name, ok := h.pickNPCName(sess, q)
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
	if why := h.gameWorld.PlaceNameRefused(name); why != "" {
		h.gmReply(sess, fmt.Sprintf("/place: %s %s: nothing made.", name, why))
		return
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
	nid, copied, err := h.camps.db.PlaceNPCRowAudited(src, at, gmEditOf(sess), fmt.Sprintf("%s (row %d)", name, src))
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
	if player.Flying {
		line += " (you fly: it stands in the air)"
	}
	h.gmReply(sess, line+" /delete on it takes it away.")
	log.Printf("GM: %s placed %s as %d (a copy of %d) at (%.1f, %.1f, %.1f) world %d facing %d", player.Name, name, nid, src,
		at.X, at.Y, at.Z, at.World, at.Facing)
}
