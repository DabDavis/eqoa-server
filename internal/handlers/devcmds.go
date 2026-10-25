package handlers

import (
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// The GM commands as a dev console (F3 20261021-003556 and -005232): typed
// in the chat line as #name, or /name from a client that forwards the
// /commands it does not know itself (eqoa-rs does, as Sandstorm's S2 does; a
// retail PS2 keeps an unknown /command to itself, so there it is #name).
//
// A GM is an account whose accountinfo.acctlevel is 1 or more, read on each
// command, so a change in the database counts without a new login. To
// anyone else a GM command is answered as an unknown /command is, and never
// said aloud. #hddsetup is everyone's: it is the hard disk's setup a PS2
// player runs (also through the Freeport setup NPC), not a GM's.
//
// Until 2026-10-21 every player was a GM.

// accessLevels reads an account's acctlevel (database.SQLiteAccountStore).
type accessLevels interface {
	AccessLevel(accountID int) (int, error)
}

// SetAccessLevels gives the GM commands the accounts' levels. Without it
// nobody is a GM.
func (h *ChatHandler) SetAccessLevels(a accessLevels) {
	h.access = a
}

// progressStore keeps a level a GM sets (database.SQLiteCharacterStore);
// purseStore a purse (database.InventoryStore).
type progressStore interface {
	SetProgress(serverID uint32, level, totalXP int) error
}

type purseStore interface {
	SetTunar(serverID uint32, tunar, bankTunar int) error
}

// SetStores gives #setlevel, #tunar and their like where to keep what they
// set. Without them a change lasts until the next login.
func (h *ChatHandler) SetStores(p progressStore, t purseStore) {
	h.progress, h.purse = p, t
}

// SetTemplate gives #template its work (CharacterHandler.ApplyTemplate).
func (h *ChatHandler) SetTemplate(fn func(*session.Session) ([]string, error)) {
	h.template = fn
}

// isGM is whether sess's account has acctlevel 1 or more.
func (h *ChatHandler) isGM(sess *session.Session) bool {
	if h.access == nil || sess == nil || sess.AccountID() == 0 {
		return false
	}
	level, err := h.access.AccessLevel(sess.AccountID())
	return err == nil && level >= 1
}

// gmCommands names every GM command, for the / spelling.
var gmCommands = map[string]bool{
	"#goto": true, "#teleport": true, "#tele": true, "#gotoplayer": true,
	"#summon": true, "#spawn": true, "#despawn": true, "#monstersearch": true,
	"#nearby": true, "#world": true, "#speed": true, "#setlevel": true,
	"#hp": true, "#tunar": true, "#kill": true, "#pos": true, "#who": true,
	"#guildhall": true, "#effect": true, "#template": true, "#help": true,
}

// slashGM is the # spelling of a /command line that names a GM command
// ("/goto freeport" is "#goto freeport"), or "" for any other line.
func slashGM(line string) string {
	if !strings.HasPrefix(line, "/") {
		return ""
	}
	word, _, _ := strings.Cut(line, " ")
	if !gmCommands["#"+strings.ToLower(word[1:])] {
		return ""
	}
	return "#" + line[1:]
}

// teleportTo moves sess's player to (x, y, z) in world, as #goto does: the
// world the visibility filters on, the position, the client's 0x07F6 and the
// world load's hold on the state channels. A world sess's client cannot load
// (LoadableWorlds) is refused, said to gm, and false returned: the client
// would stay where it was while the server filed the player under that world.
func (h *ChatHandler) teleportTo(gm, sess *session.Session, p *entity.Player, x, y, z, facing float32, world byte) bool {
	if !CanLoadWorld(sess, world) {
		h.gmReply(gm, CannotLoad(p.Name, world))
		return false
	}
	worldChanged := p.World != world
	p.World = world
	h.gameWorld.UpdatePosition(p.ServerID, x, y, z, facing)
	sendTeleportGM(sess, x, y, z, facing, world)
	beginWorldLoad(sess, worldChanged)
	return true
}

// searchNames is the names (sorted) that q (lowered) begins, then those with
// a later word q begins: "guard" finds "Guard Dahvel" before "Captain of
// the Guard", "nagaf" finds "Nagafen" and "Lord Nagafen", and "guard a" only
// the guards whose name goes on with an A.
func searchNames(names []string, q string) []string {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return nil
	}
	var first, later []string
	for _, n := range names {
		l := strings.ToLower(n)
		switch {
		case strings.HasPrefix(l, q):
			first = append(first, n)
		case strings.Contains(l, " "+q):
			later = append(later, n)
		}
	}
	return append(first, later...)
}

// listNames answers with up to ten of names, and how many more there are.
func (h *ChatHandler) listNames(sess *session.Session, names []string) {
	for i, n := range names {
		if i == 10 {
			h.gmReply(sess, fmt.Sprintf("  ...and %d more: type more of the name", len(names)-10))
			break
		}
		h.gmReply(sess, "  "+n)
	}
}

// npcNamed reports an NPC whose name is q, spelled out (without case).
func (h *ChatHandler) npcNamed(q string) bool {
	for _, n := range h.gameWorld.NPCNames() {
		if strings.EqualFold(n, strings.TrimSpace(q)) {
			return true
		}
	}
	return false
}

// pickNPCName is the NPC name q means: the one spelled so (without case),
// else the only one it begins (searchNames). Several candidates are listed
// and none is picked.
func (h *ChatHandler) pickNPCName(sess *session.Session, q string) (string, bool) {
	names := h.gameWorld.NPCNames()
	for _, n := range names {
		if strings.EqualFold(n, q) {
			return n, true
		}
	}
	found := searchNames(names, q)
	switch len(found) {
	case 0:
		return "", false
	case 1:
		return found[0], true
	}
	h.gmReply(sess, fmt.Sprintf("%d NPCs match %q; which one?", len(found), q))
	h.listNames(sess, found)
	return "", false
}

// gotoCommand is #goto, also #teleport: x y z [world], or a place by name: a
// #tele shortcut or a travel NPC (ResolveTeleDest), a town a coachman or a
// harbormaster serves ("Freeport", "Neriak", "Highpass"), the NPCs a word of
// whose names it is when they stand together ("guk": NamesakePlace), or any
// NPC. Only of the worlds the GM's game can load.
func (h *ChatHandler) gotoCommand(sess *session.Session, player *entity.Player, cmd string, args []string) {
	if len(args) == 0 {
		h.gmReply(sess, "Usage: "+cmd+" <x> <y> <z> [world] | "+cmd+" <town or NPC name>")
		h.gmReply(sess, "  world: 0=Tunaria 1=Rathe 2=Odus 3=Lavastm 4=Planesky")
		h.gmReply(sess, "         5=Secrets 6=world6 7=Secrets2")
		return
	}
	if len(args) >= 3 {
		x, ex := strconv.ParseFloat(args[0], 32)
		y, ey := strconv.ParseFloat(args[1], 32)
		z, ez := strconv.ParseFloat(args[2], 32)
		if ex == nil && ey == nil && ez == nil {
			world := byte(0)
			if len(args) >= 4 {
				w, _ := strconv.Atoi(args[3])
				if w >= 0 && w < len(worldNames) {
					world = byte(w)
				}
			}
			if !h.teleportTo(sess, sess, player, float32(x), float32(y), float32(z), player.Facing, world) {
				return
			}
			h.gmReply(sess, fmt.Sprintf("Teleported to (%.0f, %.0f, %.0f) world=%s", x, y, z, worldNames[world]))
			log.Printf("GM: %s teleported to (%.0f, %.0f, %.0f) world=%d", player.Name, x, y, z, world)
			return
		}
	}
	name := strings.Join(args, " ")
	loadable := LoadableWorlds(sess)
	if x, y, z, world, via, ok := ResolveTeleDest(h.gameWorld, name); ok {
		if !h.teleportTo(sess, sess, player, x, y, z, player.Facing, world) {
			return
		}
		h.gmReply(sess, fmt.Sprintf("Teleported to %s - (%.0f, %.0f, %.0f) world=%s", via, x, y, z, worldName(world)))
		log.Printf("GM: %s teleported to %s", player.Name, via)
		return
	}
	places := h.gameWorld.Places(loadable)
	var matched []entity.Place
	for _, p := range places {
		if strings.EqualFold(p.Name, name) {
			matched = []entity.Place{p}
			break
		}
		if strings.HasPrefix(strings.ToLower(p.Name), strings.ToLower(name)) {
			matched = append(matched, p)
		}
	}
	if len(matched) == 1 {
		// on the NPC's own spot, as #tele lands: a step aside can be a wall
		p := matched[0]
		if !h.teleportTo(sess, sess, player, p.X, p.Y, p.Z, player.Facing, p.World) {
			return
		}
		h.gmReply(sess, fmt.Sprintf("Teleported to %s (%s) - (%.0f, %.0f, %.0f) world=%s", p.Name, p.NPC, p.X, p.Y, p.Z, worldName(p.World)))
		log.Printf("GM: %s teleported to %s", player.Name, p.Name)
		return
	}
	if len(matched) > 1 {
		h.gmReply(sess, fmt.Sprintf("%d places match %q; which one?", len(matched), name))
		for _, p := range matched {
			h.gmReply(sess, "  "+p.Name)
		}
		return
	}
	// several NPCs that a word of the name finds, standing together: their
	// place ("guk", the "froglok guk ..." of Guk), where pickNPCName would list
	// them; one name spelled out in full is still that NPC
	if !h.npcNamed(name) {
		if p, n, ok := NamesakePlace(h.gameWorld, name, loadable); ok && n > 1 {
			if !h.teleportTo(sess, sess, player, p.X, p.Y, p.Z, player.Facing, p.World) {
				return
			}
			h.gmReply(sess, fmt.Sprintf("Teleported to %s, where the %d NPCs %q finds stand - (%.0f, %.0f, %.0f) world=%s",
				p.NPC, n, name, p.X, p.Y, p.Z, worldName(p.World)))
			log.Printf("GM: %s teleported to %s (%q, %d NPCs)", player.Name, p.NPC, name, n)
			return
		}
	}
	npcName, ok := h.pickNPCName(sess, name)
	if !ok {
		if len(searchNames(h.gameWorld.NPCNames(), name)) == 0 {
			var towns []string
			for _, p := range places {
				towns = append(towns, p.Name)
			}
			h.gmReply(sess, fmt.Sprintf("No place or NPC called %q. Towns: %s", name, strings.Join(towns, ", ")))
		}
		return
	}
	x, y, z, _, world, _ := h.gameWorld.NPCByName(npcName)
	if !h.teleportTo(sess, sess, player, x, y, z, player.Facing, world) {
		return
	}
	h.gmReply(sess, fmt.Sprintf("Teleported to %s - (%.0f, %.0f, %.0f) world=%s", npcName, x, y, z, worldName(world)))
	log.Printf("GM: %s teleported to %s", player.Name, npcName)
}

// gotoPlayerCommand is #gotoplayer <name>: to an online player, in its world.
func (h *ChatHandler) gotoPlayerCommand(sess *session.Session, player *entity.Player, args []string) {
	if len(args) == 0 {
		h.gmReply(sess, "Usage: #gotoplayer <name>")
		return
	}
	name := strings.Join(args, " ")
	t := h.gameWorld.PlayerByName(name)
	if t == nil {
		h.gmReply(sess, fmt.Sprintf("No player named %q is online.", name))
		return
	}
	if t == player {
		h.gmReply(sess, "That is you.")
		return
	}
	if !h.teleportTo(sess, sess, player, t.X, t.Y, t.Z, player.Facing, t.World) {
		return
	}
	h.gmReply(sess, fmt.Sprintf("Teleported to %s - (%.0f, %.0f, %.0f) world=%s", t.Name, t.X, t.Y, t.Z, worldName(t.World)))
	log.Printf("GM: %s went to %s", player.Name, t.Name)
}

// summonCommand is #summon <name>: an online player brought beside the GM,
// into the GM's world, and told who did it.
func (h *ChatHandler) summonCommand(sess *session.Session, player *entity.Player, args []string) {
	if len(args) == 0 {
		h.gmReply(sess, "Usage: #summon <name>")
		return
	}
	name := strings.Join(args, " ")
	t := h.gameWorld.PlayerByName(name)
	if t == nil {
		h.gmReply(sess, fmt.Sprintf("No player named %q is online.", name))
		return
	}
	if t == player {
		h.gmReply(sess, "That is you.")
		return
	}
	ts := h.gameWorld.FindSessionByEntityID(t.ServerID)
	if ts == nil {
		h.gmReply(sess, fmt.Sprintf("%s has no session.", t.Name))
		return
	}
	if !h.teleportTo(sess, ts, t, player.X, player.Y, player.Z, t.Facing, player.World) {
		return
	}
	h.gmReply(ts, fmt.Sprintf("%s summoned you.", player.Name))
	h.gmReply(sess, fmt.Sprintf("Summoned %s.", t.Name))
	log.Printf("GM: %s summoned %s", player.Name, t.Name)
}

// spawnCommand is #spawn <name>: a copy of that NPC a few units in front of
// the GM (entity.World.SpawnNPCCopy), gone after its corpse time once killed
// or at #despawn. Without a name, the NPCs nearby (#nearby).
func (h *ChatHandler) spawnCommand(sess *session.Session, player *entity.Player, args []string) {
	if len(args) == 0 {
		h.nearbyCommand(sess, player)
		return
	}
	name, ok := h.pickNPCName(sess, strings.Join(args, " "))
	if !ok {
		if len(searchNames(h.gameWorld.NPCNames(), strings.Join(args, " "))) == 0 {
			h.gmReply(sess, fmt.Sprintf("No NPC called %q (try #monstersearch).", strings.Join(args, " ")))
		}
		return
	}
	x, z := player.X+3*float32(math.Sin(float64(player.Facing))), player.Z+3*float32(math.Cos(float64(player.Facing)))
	facing := player.Facing + math.Pi
	id, spelled, ok := h.gameWorld.SpawnNPCCopy(name, x, player.Y, z, facing, player.World)
	if !ok {
		h.gmReply(sess, fmt.Sprintf("No NPC called %q.", name))
		return
	}
	h.gmReply(sess, fmt.Sprintf("Spawned %s. Target it and #despawn to take it away.", spelled))
	log.Printf("GM: %s spawned %s (entity ID=%d) at (%.1f, %.1f, %.1f) world=%d",
		player.Name, spelled, id, x, player.Y, z, player.World)
}

// despawnCommand is #despawn: the targeted NPC taken away, when a #spawn
// made it.
func (h *ChatHandler) despawnCommand(sess *session.Session, player *entity.Player) {
	if player.TargetObjectID == 0 || player.TargetObjectID == 0xFFFFFFFF {
		h.gmReply(sess, "No target selected")
		return
	}
	target, found := h.gameWorld.FindEntityByObjectID(sess, player.TargetObjectID)
	npc, isNPC := target.(*entity.NPC)
	if !found || !isNPC {
		h.gmReply(sess, "Target an NPC that #spawn made.")
		return
	}
	name, ok := h.gameWorld.DespawnNPC(npc.ServerID)
	if !ok {
		h.gmReply(sess, fmt.Sprintf("%s was not made by #spawn.", npc.Name))
		return
	}
	h.gmReply(sess, fmt.Sprintf("Despawned %s.", name))
	log.Printf("GM: %s despawned %s", player.Name, name)
}

// monsterSearchCommand is #monstersearch <name>: up to ten NPC names it
// begins (searchNames), each once, sorted.
func (h *ChatHandler) monsterSearchCommand(sess *session.Session, args []string) {
	if len(args) == 0 {
		h.gmReply(sess, "Usage: #monstersearch <the start of a name>")
		return
	}
	q := strings.Join(args, " ")
	found := searchNames(h.gameWorld.NPCNames(), q)
	if len(found) == 0 {
		h.gmReply(sess, fmt.Sprintf("No NPC name begins %q.", q))
		return
	}
	if len(found) == 1 {
		h.gmReply(sess, fmt.Sprintf("1 NPC matches %q:", q))
	} else {
		h.gmReply(sess, fmt.Sprintf("%d NPCs match %q:", len(found), q))
	}
	h.listNames(sess, found)
}

// nearbyCommand lists the NPCs within 200 (#nearby; #spawn alone did, and
// still does).
func (h *ChatHandler) nearbyCommand(sess *session.Session, player *entity.Player) {
	npcs := h.gameWorld.GetNPCsInRadius(player.X, player.Z, 200)
	h.gmReply(sess, fmt.Sprintf("Nearby NPCs (%d):", len(npcs)))
	for i, npc := range npcs {
		if i >= 10 {
			break
		}
		dx := npc.X - player.X
		dz := npc.Z - player.Z
		dist := float32(math.Sqrt(float64(dx*dx + dz*dz)))
		h.gmReply(sess, fmt.Sprintf("  %s Lv%d HP=%d (%.0fm)", npc.Name, npc.Level, npc.HP, dist))
	}
}

// setLevel is #setlevel: the level where the game holds it (its stats and the
// stat channel), told to the client (0x0020, as the GM panel tells it), and
// kept with no experience into it. It was the live Player's alone, so a level
// set was gone at the next login (Jibrish 60 in the game and 1 in the
// database, F3 pass 13).
func (h *ChatHandler) setLevel(sess *session.Session, player *entity.Player, level int) {
	level = max(1, min(60, level))
	h.gameWorld.SetPlayerLevel(player.CharServerID, level)
	SendLevel(sess, level)
	if h.progress != nil {
		if err := h.progress.SetProgress(player.CharServerID, level, entity.TotalXP(level, 0)); err != nil {
			log.Printf("GM: #setlevel %d for '%s' not kept: %v", level, player.Name, err)
			h.gmReply(sess, fmt.Sprintf("Level set to %d, but not kept: %v", level, err))
			return
		}
	}
	h.gmReply(sess, fmt.Sprintf("Level set to %d", level))
}

// setTunar is #tunar: the purse where the game holds it, told to the client
// and kept (every other change of the purse is: merchant.go's saveTunar).
func (h *ChatHandler) setTunar(sess *session.Session, player *entity.Player, amount int) {
	amount = max(0, amount)
	h.gameWorld.SetPlayerTunar(player.CharServerID, amount, player.BankTunar)
	SendPlayerTunar(sess, int64(amount))
	if h.purse != nil {
		if err := h.purse.SetTunar(player.CharServerID, amount, player.BankTunar); err != nil {
			log.Printf("GM: #tunar %d for '%s' not kept: %v", amount, player.Name, err)
			h.gmReply(sess, fmt.Sprintf("Tunar set to %d, but not kept: %v", amount, err))
			return
		}
	}
	h.gmReply(sess, fmt.Sprintf("Tunar set to %d", amount))
}

// templateCommand is #template (template.go): the GM's own character at 60,
// its class's spells, 1,000,000 tunar and the best gear it may wear.
func (h *ChatHandler) templateCommand(sess *session.Session) {
	if h.template == nil {
		h.gmReply(sess, "Template: this server keeps no characters")
		return
	}
	lines, err := h.template(sess)
	if err != nil {
		h.gmReply(sess, "Template: "+err.Error())
		return
	}
	for _, l := range lines {
		h.gmReply(sess, l)
	}
}
