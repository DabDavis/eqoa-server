package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// A GM's marks, made into a camp's static spawns or a creature's path, in the
// game (the GM Jibrish setting up XP camps, 2026-11-08: "maybe a command to
// capture multiple points? so i can go HERE, HERE and HERE", "or at least
// static spawns"; petition 1293: four static spawns on Xrag's rock, each
// popping a child or a pariah of Xrag, where no roamer came). Each a GM's
// (acctlevel 1 or more), as #name or /name:
//
//   - /mark [note]: where the GM's character stands now (world, x, y, z,
//     facing) kept as its account's next mark, with the note (gmmarks; a
//     logout and a restart keep them); /marks lists them, /marks undo drops
//     the last, /marks clear all of them;
//   - /camp <creature>[, <creature>...]: a static spawn at each mark, its own
//     row of npcs, each a copy of a row of one of the names at random (the
//     row of the name nearest the first mark, not a camp's: its body, level,
//     gear, and by its name its loot, class, template and faction), facing as
//     the GM faced there; each stands whatever its kind, pops one of the names
//     at random each time it comes back, and keeps the wander out of the camp
//     (entity/campspawn.go). The marks are cleared. /camp undo takes the GM's
//     last camp away and gives its marks back when the GM holds none;
//   - /path [loop|pingpong|once] [walk|run]: the GM's target, an NPC of a row,
//     walks a path through the marks (npcpaths, as the GM panel's map gives
//     one; entity/path.go), the marks cleared; /path clear takes it away.
//
// Every change goes to the database and the running world at once, each row
// with its gm_audit line in the change's transaction (database/gmcamps.go),
// so the GM panel lists and reverts it as its own edits. A GM's petition (a
// /bug, F3, Support's form) carries its marks in its server view
// (petitionview.go), for whoever reads it to place them.

// campTools is what /mark, /camp and /path keep their work in: the server's
// database, and the first kinds' roll.
type campTools struct {
	db  *database.SQLiteDB
	rnd *rand.Rand
}

// SetCampStore gives /mark, /camp and /path where they keep what they make,
// and the players' camp reports where they are filed (campfix.go). Without it
// they answer that nothing is kept.
func (h *ChatHandler) SetCampStore(db *database.SQLiteDB) {
	h.camps.db = db
	h.campfix.mu.Lock()
	h.campfix.db = db
	h.campfix.mu.Unlock()
}

// markNoteMax is the most of a mark's note kept, in characters.
const markNoteMax = 100

// gmEditOf is sess's GM as the audit log names a change from the game.
func gmEditOf(sess *session.Session) database.GMEdit {
	who := database.GMEdit{AccountID: sess.AccountID(), Via: "game"}
	if a := sess.RemoteAddr(); a != nil {
		who.IP = a.IP.String()
	}
	return who
}

// cleanMarkNote is a mark's note as kept: no control character, at most
// markNoteMax characters, trimmed.
func cleanMarkNote(s string) string {
	var b strings.Builder
	n := 0
	for _, r := range strings.ToValidUTF8(s, "?") {
		if unicode.IsControl(r) {
			r = ' '
		}
		if n == markNoteMax {
			break
		}
		b.WriteRune(r)
		n++
	}
	return strings.TrimSpace(b.String())
}

// markPlace is a mark's place as the GM reads it.
func markPlace(m database.GMMark) string {
	return fmt.Sprintf("(%.1f, %.1f, %.1f) in world %d", m.X, m.Y, m.Z, m.World)
}

// markLine is mark n as /mark and /marks say it.
func markLine(n int, m database.GMMark) string {
	l := fmt.Sprintf("Mark %d: %s", n, markPlace(m))
	if m.Note != "" {
		l += " - " + m.Note
	}
	return l
}

// facingByte is a facing in radians as an npcs row keeps it: 0-255 round
// the circle, as the channels send it (statesync.go).
func facingByte(f float64) int {
	f = math.Mod(f, 2*math.Pi)
	if f < 0 {
		f += 2 * math.Pi
	}
	return int(math.Round(f/(2*math.Pi)*256)) % 256
}

// campRand is the first kinds' roll (tests seed it).
func (h *ChatHandler) campRand() *rand.Rand {
	if h.camps.rnd == nil {
		h.camps.rnd = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return h.camps.rnd
}

// markCommand is /mark [note]: where the GM stands kept as its next mark.
func (h *ChatHandler) markCommand(sess *session.Session, player *entity.Player, note string) {
	if h.camps.db == nil {
		h.gmReply(sess, "/mark: marks are not kept here")
		return
	}
	m := database.GMMark{AccountID: sess.AccountID(), World: int(player.World),
		X: float64(player.X), Y: float64(player.Y), Z: float64(player.Z), Facing: float64(player.Facing),
		Note: cleanMarkNote(note), CharName: player.Name}
	m, n, err := h.camps.db.AddGMMark(m, gmEditOf(sess))
	switch {
	case database.ErrTooManyMarks(err):
		h.gmReply(sess, "/mark: "+err.Error())
		return
	case err != nil:
		h.gmReply(sess, "/mark: not kept: "+err.Error())
		return
	}
	line := markLine(n, m)
	if player.Flying {
		line += " (you fly: a spawn here would stand in the air)"
	}
	h.gmReply(sess, line)
	log.Printf("GM: %s marked %s (mark %d)", player.Name, markPlace(m), n)
}

// marksCommand is /marks [undo | clear]: the GM's marks listed, the last
// dropped, or all of them.
func (h *ChatHandler) marksCommand(sess *session.Session, args []string) {
	if h.camps.db == nil {
		h.gmReply(sess, "/marks: marks are not kept here")
		return
	}
	sub := ""
	if len(args) > 0 {
		sub = strings.ToLower(args[0])
	}
	switch sub {
	case "":
		marks, err := h.camps.db.GMMarks(sess.AccountID())
		if err != nil {
			h.gmReply(sess, "/marks: "+err.Error())
			return
		}
		if len(marks) == 0 {
			h.gmReply(sess, "You have no marks: /mark [note] keeps where you stand.")
			return
		}
		h.gmReply(sess, fmt.Sprintf("Your marks (%d), the first made first:", len(marks)))
		for i, m := range marks {
			h.gmReply(sess, fmt.Sprintf("  %s, facing %.2f", markLine(i+1, m), m.Facing))
		}
		h.gmReply(sess, "/camp <creature>[, <creature>...] makes a spawn at each; /path [loop|pingpong|once] [walk|run] walks your target through them; /marks undo, /marks clear")
	case "undo":
		m, n, ok, err := h.camps.db.DropLastGMMark(sess.AccountID(), gmEditOf(sess))
		switch {
		case err != nil:
			h.gmReply(sess, "/marks undo: "+err.Error())
		case !ok:
			h.gmReply(sess, "You have no marks.")
		default:
			h.gmReply(sess, fmt.Sprintf("Mark %d dropped: %s. %d left.", n, markPlace(m), n-1))
		}
	case "clear":
		n, err := h.camps.db.ClearGMMarks(sess.AccountID(), gmEditOf(sess))
		if err != nil {
			h.gmReply(sess, "/marks clear: "+err.Error())
			return
		}
		h.gmReply(sess, fmt.Sprintf("%d %s cleared.", n, plural(n, "mark", "marks")))
	default:
		h.gmReply(sess, "Usage: /marks [undo | clear] - your marks, the last dropped, or all of them")
	}
}

// campCommand is /camp <creature>[, <creature>...] or /camp undo (the file's
// doc).
func (h *ChatHandler) campCommand(sess *session.Session, player *entity.Player, args []string) {
	const usage = "Usage: /camp <creature>[, <creature>...] - a static spawn at each of your marks, popping one of them at random each time; /camp undo"
	if h.camps.db == nil {
		h.gmReply(sess, "/camp: camps are not kept here")
		return
	}
	if len(args) == 1 && strings.EqualFold(args[0], "undo") {
		h.campUndo(sess, player)
		return
	}
	given := database.CampKindNames(strings.Join(args, " "))
	if len(given) == 0 {
		h.gmReply(sess, usage)
		return
	}
	if len(given) > database.MaxCampKinds {
		h.gmReply(sess, fmt.Sprintf("%d kinds: a camp pops at most %d.", len(given), database.MaxCampKinds))
		return
	}
	marks, err := h.camps.db.GMMarks(sess.AccountID())
	if err != nil {
		h.gmReply(sess, "/camp: "+err.Error())
		return
	}
	if len(marks) == 0 {
		h.gmReply(sess, "You have no marks: /mark where each spawn stands first.")
		return
	}
	world := marks[0].World
	for i, m := range marks {
		if m.World != world {
			h.gmReply(sess, fmt.Sprintf("Mark %d is in world %d, mark 1 in world %d: a camp is in one world (/marks undo or /marks clear).", i+1, m.World, world))
			return
		}
	}
	// each name as the NPCs spell it, once, with the row its spawns copy
	type kind struct {
		name string
		from int
		rec  *database.NPCRecord
	}
	var kinds []kind
	seen := map[string]bool{}
	npcs := database.NewNPCStore(h.camps.db)
	for _, q := range given {
		name, ok := h.pickNPCName(sess, q)
		if !ok {
			if len(searchNames(h.gameWorld.NPCNames(), q)) == 0 {
				h.gmReply(sess, fmt.Sprintf("No NPC is called %q: nothing made.", q))
			}
			return
		}
		key := database.NPCClassKey(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		if why := h.gameWorld.CampKindRefused(name); why != "" {
			h.gmReply(sess, fmt.Sprintf("%s %s: nothing made.", name, why))
			return
		}
		id, ok := h.gameWorld.CampTemplate(name, uint8(world), float32(marks[0].X), float32(marks[0].Z))
		if !ok {
			h.gmReply(sess, fmt.Sprintf("No NPC row is called %q (a GM's spawn has none): nothing made.", name))
			return
		}
		rec, err := npcs.GetNPC(int(id))
		if err != nil {
			h.gmReply(sess, fmt.Sprintf("%s's row %d: %v: nothing made.", name, id, err))
			return
		}
		kinds = append(kinds, kind{name: rec.NPCName, from: int(id), rec: rec})
	}
	plan := database.CampPlan{AccountID: sess.AccountID(), CharName: player.Name, World: world, Marks: marks}
	for _, k := range kinds {
		plan.Kinds = append(plan.Kinds, k.name)
	}
	picks := make([]int, len(marks))
	for i, m := range marks {
		picks[i] = h.campRand().Intn(len(kinds))
		plan.Spawns = append(plan.Spawns, database.CampSpawnPlan{From: kinds[picks[i]].from, World: m.World,
			X: m.X, Y: m.Y, Z: m.Z, Facing: facingByte(m.Facing)})
	}
	made, err := h.camps.db.MakeCamp(plan, gmEditOf(sess))
	if err != nil {
		h.gmReply(sess, "/camp: nothing made: "+err.Error())
		return
	}
	// the world: the spawns known as a camp's first, so none takes a step as
	// its kind would, then each in it as its row makes it
	if _, err := h.gameWorld.LoadCampSpawns(h.camps.db); err != nil {
		log.Printf("GM: /camp %d: the camps not read again: %v", made.Camp, err)
	}
	var live []string
	for _, id := range made.NPCIDs {
		rec, err := npcs.GetNPC(id)
		if err != nil {
			live = append(live, fmt.Sprintf("NPC %d: %v", id, err))
			continue
		}
		class, _ := npcs.NPCClassOf(rec.NPCName)
		if _, err := h.gameWorld.UpsertNPC(rec, class); err != nil {
			live = append(live, fmt.Sprintf("NPC %d not in the world yet (at the next start): %v", id, err))
		}
	}
	names := make([]string, len(kinds))
	for i, k := range kinds {
		names[i] = k.name
	}
	spawns := fmt.Sprintf("%d static %s", len(made.NPCIDs), plural(len(made.NPCIDs), "spawn", "spawns"))
	if len(kinds) == 1 {
		h.gmReply(sess, fmt.Sprintf("Camp %d made in world %d: %s of %s.", made.Camp, world, spawns, names[0]))
	} else {
		h.gmReply(sess, fmt.Sprintf("Camp %d made in world %d: %s, %s %s at random every time it comes back.",
			made.Camp, world, spawns, plural(len(made.NPCIDs), "popping", "each popping"), orList(names)))
	}
	for i, id := range made.NPCIDs {
		k := kinds[picks[i]]
		h.gmReply(sess, fmt.Sprintf("  NPC %d: %s (level %d) at mark %d %s", id, k.name, k.rec.NPCLevel, i+1, markPlace(marks[i])))
	}
	for _, l := range live {
		h.gmReply(sess, "  "+l)
	}
	h.gmReply(sess, "Your marks are cleared. /camp undo takes the camp away.")
	log.Printf("GM: %s made camp %d in world %d: %d spawns of %s (NPCs %v)", player.Name, made.Camp, world, len(made.NPCIDs),
		database.CampKindsText(names), made.NPCIDs)
}

// orList is names as "a, b or c".
func orList(names []string) string {
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " or " + names[len(names)-1]
}

// campUndo is /camp undo: the GM's last camp taken away, out of the world at
// once, its marks given back when the GM holds none.
func (h *ChatHandler) campUndo(sess *session.Session, player *entity.Player) {
	u, ok, err := h.camps.db.UndoLastCamp(sess.AccountID(), gmEditOf(sess))
	switch {
	case err != nil:
		h.gmReply(sess, "/camp undo: "+err.Error())
		return
	case !ok:
		h.gmReply(sess, "You have made no camp that still stands.")
		return
	}
	for _, id := range u.NPCIDs {
		h.gameWorld.RemoveNPC(entity.EntityID(id))
	}
	for _, id := range u.Paths {
		h.gameWorld.ClearNPCPath(entity.EntityID(id))
	}
	if _, err := h.gameWorld.LoadCampSpawns(h.camps.db); err != nil {
		log.Printf("GM: /camp undo %d: the camps not read again: %v", u.Camp, err)
	}
	ids := make([]string, len(u.NPCIDs))
	for i, id := range u.NPCIDs {
		ids[i] = strconv.Itoa(id)
	}
	h.gmReply(sess, fmt.Sprintf("Camp %d (%s) taken away: its %d %s gone (NPCs %s).", u.Camp, u.Kinds, len(u.NPCIDs),
		plural(len(u.NPCIDs), "spawn is", "spawns are"), strings.Join(ids, ", ")))
	switch {
	case u.MarksBack > 0:
		h.gmReply(sess, fmt.Sprintf("Its %d %s yours again: /camp makes it again, /marks clear drops them.", u.MarksBack,
			plural(u.MarksBack, "mark is", "marks are")))
	case u.MarksHeld > 0:
		h.gmReply(sess, fmt.Sprintf("You hold %d %s of your own: the camp's are not given back.", u.MarksHeld, plural(u.MarksHeld, "mark", "marks")))
	}
	log.Printf("GM: %s took camp %d away (NPCs %v)", player.Name, u.Camp, u.NPCIDs)
}

// targetRowNPC is the GM's target as an NPC of a row (its npcs id, its row
// and its name), or false, said why.
func (h *ChatHandler) targetRowNPC(sess *session.Session, player *entity.Player, cmd string) (entity.EntityID, *database.NPCRecord, bool) {
	if player.TargetObjectID == 0 || player.TargetObjectID == 0xFFFFFFFF {
		h.gmReply(sess, cmd+": target the NPC first")
		return 0, nil, false
	}
	ent, ok := h.gameWorld.FindEntityByObjectID(sess, player.TargetObjectID)
	npc, isNPC := ent.(*entity.NPC)
	if !ok || !isNPC {
		h.gmReply(sess, cmd+": your target is no NPC")
		return 0, nil, false
	}
	st, ok := h.gameWorld.NPCStateOf(npc.ServerID)
	if !ok {
		h.gmReply(sess, cmd+": your target has left the world")
		return 0, nil, false
	}
	if st.Temporary {
		h.gmReply(sess, fmt.Sprintf("%s: %s is a spawn with no row (a GM's, a quest's, a trigger's, a pet): no path is kept for it", cmd, st.Name))
		return 0, nil, false
	}
	rec, err := database.NewNPCStore(h.camps.db).GetNPC(int(st.ID))
	if errors.Is(err, sql.ErrNoRows) {
		h.gmReply(sess, fmt.Sprintf("%s: %s has no NPC row", cmd, st.Name))
		return 0, nil, false
	}
	if err != nil {
		h.gmReply(sess, fmt.Sprintf("%s: %s's row: %v", cmd, st.Name, err))
		return 0, nil, false
	}
	return st.ID, rec, true
}

// pathPace is a path's pace as the GM reads it.
func pathPace(speed string) string {
	switch speed {
	case database.PathSpeedWalk:
		return fmt.Sprintf("at a walk (%.0f a second)", entity.NPCWalkSpeed)
	case database.PathSpeedRun:
		return fmt.Sprintf("at a run (%.1f a second)", entity.NPCRunSpeed)
	}
	return "at " + speed + " a second"
}

// pathModeText is a path's mode as the GM reads it.
func pathModeText(mode string) string {
	switch mode {
	case database.PathModePingPong:
		return "back and forth"
	case database.PathModeOnce:
		return "once, then it stands at the last point"
	}
	return "in a loop"
}

// pathCommand is /path [loop|pingpong|once] [walk|run], or /path clear (the
// file's doc).
func (h *ChatHandler) pathCommand(sess *session.Session, player *entity.Player, args []string) {
	const usage = "Usage: /path [loop|pingpong|once] [walk|run] - your target walks your marks; /path clear takes its path away"
	if h.camps.db == nil {
		h.gmReply(sess, "/path: paths are not kept here")
		return
	}
	if len(args) == 1 && strings.EqualFold(args[0], "clear") {
		id, rec, ok := h.targetRowNPC(sess, player, "/path clear")
		if !ok {
			return
		}
		gone, err := h.camps.db.ClearNPCPathAudited(int(id), gmEditOf(sess), fmt.Sprintf("/path clear in the game: %s (%d)", rec.NPCName, id))
		switch {
		case err != nil:
			h.gmReply(sess, "/path clear: "+err.Error())
		case !gone:
			h.gmReply(sess, fmt.Sprintf("%s (NPC %d) has no path.", rec.NPCName, id))
		default:
			h.gameWorld.ClearNPCPath(id)
			h.gmReply(sess, fmt.Sprintf("%s (NPC %d) has no path now: it walks back to its spawn point and is as before.", rec.NPCName, id))
			log.Printf("GM: %s took NPC %d's path away", player.Name, id)
		}
		return
	}
	mode, speed := database.PathModeLoop, database.PathSpeedWalk
	modeSet, speedSet := false, false
	for _, a := range args {
		if m, ok := database.PathMode(a); ok && !modeSet {
			mode, modeSet = m, true
			continue
		}
		if s, ok := database.PathSpeed(a); ok && !speedSet {
			speed, speedSet = s, true
			continue
		}
		h.gmReply(sess, usage)
		return
	}
	id, rec, ok := h.targetRowNPC(sess, player, "/path")
	if !ok {
		return
	}
	marks, err := h.camps.db.GMMarks(sess.AccountID())
	if err != nil {
		h.gmReply(sess, "/path: "+err.Error())
		return
	}
	if len(marks) == 0 {
		h.gmReply(sess, "You have no marks: /mark each point of the path first.")
		return
	}
	path := database.NPCPathRecord{NPCID: int(id), World: rec.World, Mode: mode, Speed: speed}
	for i, m := range marks {
		if m.World != rec.World {
			h.gmReply(sess, fmt.Sprintf("Mark %d is in world %d, %s's row in world %d: a path is in its NPC's world.", i+1, m.World, rec.NPCName, rec.World))
			return
		}
		path.Points = append(path.Points, database.PathPoint{X: math.Round(m.X*10) / 10, Y: math.Round(m.Y*10) / 10, Z: math.Round(m.Z*10) / 10})
	}
	if err := path.Check(); err != nil {
		h.gmReply(sess, "/path: "+err.Error())
		return
	}
	detail := fmt.Sprintf("/path in the game: %s (%d), %d points", rec.NPCName, id, len(path.Points))
	if _, err := h.camps.db.SetNPCPathAudited(path, gmEditOf(sess), detail, sess.AccountID(), marks); err != nil {
		h.gmReply(sess, "/path: not given: "+err.Error())
		return
	}
	p, err := entity.PathFromRecord(path)
	if err == nil {
		err = h.gameWorld.SetNPCPath(id, p)
	}
	if err != nil {
		h.gmReply(sess, fmt.Sprintf("%s (NPC %d)'s path is kept but not walked yet (at the next start): %v", rec.NPCName, id, err))
		return
	}
	h.gmReply(sess, fmt.Sprintf("%s (NPC %d) walks a path of %d points now: %s, %s. Your marks are cleared.",
		rec.NPCName, id, len(path.Points), pathModeText(mode), pathPace(speed)))
	h.gmReply(sess, "/path clear takes it away; the GM panel's map shows it (NPC paths).")
	log.Printf("GM: %s gave NPC %d (%s) a path of %d points, %s, %s", player.Name, id, rec.NPCName, len(path.Points), mode, speed)
}
