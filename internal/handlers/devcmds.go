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
// subGMs is an accessLevels that also knows the sub GMs (database's SubGM: a
// tester's row marked subgm).
type subGMs interface {
	SubGM(accountID int) (bool, error)
}

// subGMCommands are the GM commands a sub GM may use (the user, 2026-11-08:
// "make his account a sub gm new category above tester than can fly"): a sub
// GM is a tester in the panel and in the game a player but for these.
// The owner, 2026-11-12: "players with the /fly /speed can use also" /delete,
// /clone and /place (gmnpcrow.go).
// The owner, 2026-11-14: "lets give the account for kobe, skywalker access to use
// ./size in addition to the rest he has": a sub GM's /size is on a creature alone
// (chat.go), and kept in its row as any GM's (gmnpcrow.go's keepNPCSize).
var subGMCommands = map[string]bool{"#fly": true, "#speed": true, "#delete": true, "#clone": true, "#place": true, "#move": true, "#roamer": true,
	"#size": true}

// isSubGM reports whether sess's account is a sub GM.
func (h *ChatHandler) isSubGM(sess *session.Session) bool {
	g, ok := h.access.(subGMs)
	if !ok || sess == nil || sess.AccountID() == 0 {
		return false
	}
	yes, err := g.SubGM(sess.AccountID())
	return err == nil && yes
}

// granted reports whether sess's account may use the GM command cmd (spelled
// "#fly") though it is no GM's: a sub GM's commands (subGMCommands), none of a
// GM's others.
func (h *ChatHandler) granted(sess *session.Session, cmd string) bool {
	return subGMCommands[gmAlias(cmd)] && h.isSubGM(sess)
}

// gmAliases are GM commands' other names (spelled "#copy"): the same command,
// whoever may use it. The owner, 2026-11-14: "./copy should work for jib also
// on live server": /copy is /clone.
var gmAliases = map[string]string{"#copy": "#clone"}

// gmAlias is the command cmd (spelled "#copy", lower case) names.
func gmAlias(cmd string) string {
	if a, ok := gmAliases[cmd]; ok {
		return a
	}
	return cmd
}

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
	"#testmode": true, "#tankprofile1": true, "#tankprofile2": true, "#tankprofile3": true,
	"#hp": true, "#tunar": true, "#kill": true, "#pos": true, "#who": true,
	"#guildhall": true, "#effect": true, "#faction": true, "#template": true, "#help": true,
	"#grantcm": true, "#npcinfo": true, "#fx": true, "#spellfx": true,
	"#aggro": true, "#fly": true, "#movecheck": true, "#ticks": true, "#net": true, "#packetlog": true, "#stats": true, "#cm": true, "#cmreset": true, "#debt": true,
	"#trigger": true, "#craftskill": true, "#craftlearn": true,
	"#clearbuffs": true, "#nukebuffs": true, "#reward": true, "#dye": true, "#vampire": true,
	"#rst_mastr": true, "#rst_both": true, "#rem_abil": true, "#banktunar": true, "#servdown": true,
	"#giveitem": true, "#size": true, "#mark": true, "#marks": true, "#camp": true, "#path": true,
	"#markportal": true, "#delete": true, "#clone": true, "#place": true, "#move": true, "#roamer": true,
	"#noclip": true, "#raidboss": true, "#aura": true, "#hazard": true,
	"#copy": true, // #clone (gmAliases)
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
// harbormaster serves ("Freeport", "Neriak", "Highpass Hold"), the NPCs a word of
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
		h.gmReply(sess, fmt.Sprintf("Teleported to %s - (%.0f, %.0f, %.0f) world=%s", via, x, y, z, placeName(world, x, y, z)))
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
		h.gmReply(sess, fmt.Sprintf("Teleported to %s (%s) - (%.0f, %.0f, %.0f) world=%s", p.Name, p.NPC, p.X, p.Y, p.Z, placeName(p.World, p.X, p.Y, p.Z)))
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
				p.NPC, n, name, p.X, p.Y, p.Z, placeName(p.World, p.X, p.Y, p.Z)))
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
	h.gmReply(sess, fmt.Sprintf("Teleported to %s - (%.0f, %.0f, %.0f) world=%s", npcName, x, y, z, placeName(world, x, y, z)))
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
	h.gmReply(sess, fmt.Sprintf("Teleported to %s - (%.0f, %.0f, %.0f) world=%s", t.Name, t.X, t.Y, t.Z, placeName(t.World, t.X, t.Y, t.Z)))
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
		out := ""
		if npc.SpawnOnly {
			out = ", spawn only: out of the world" // entity/spawnonly.go
		}
		h.gmReply(sess, fmt.Sprintf("  %s Lv%d HP=%d (%.0fm%s)", npc.Name, npc.Level, npc.HP, dist, out))
	}
}

// setLevelCommand is #setlevel [name] <level> (Jibrish, petition 1265:
// "/setlevel Nuke 60 should set the character nuke to 60 if im targeting them,
// but not add or remove spells or change anything beyond level"): the
// character a name names, in the world or not, else the GM's target player,
// else the GM (gmCharacter, as #banktunar's), the name and the level in either
// order. Only the level changes, as setLevelFor sets it: no spell, item, tunar
// or mastery (#template is the command that gives those).
func (h *ChatHandler) setLevelCommand(sess *session.Session, gm *entity.Player, args []string) {
	name, level := "", ""
	switch len(args) {
	case 0:
	case 1:
		level = args[0]
		if _, err := strconv.Atoi(level); err != nil {
			name, level = args[0], ""
		}
	default:
		name, level = args[0], args[1]
		if _, err := strconv.Atoi(name); err == nil {
			if _, err := strconv.Atoi(level); err != nil {
				name, level = level, name // #setlevel 60 Nuke
			}
		}
	}
	s, ok := h.gmCharacter(sess, gm, name, "Set level")
	if !ok {
		return
	}
	n, err := strconv.Atoi(level)
	if err != nil {
		switch {
		case s.p != nil:
			h.gmReply(sess, fmt.Sprintf("%s is level %d", s.name, s.p.Level))
		case h.resets != nil:
			if rec, err := h.resets.GetByServerID(s.serverID); err == nil && rec != nil {
				h.gmReply(sess, fmt.Sprintf("%s is level %d (not in the world)", s.name, rec.Level))
			}
		}
		h.gmReply(sess, "Usage: #setlevel [name] <level> - the level alone; no name: your target player, else you")
		return
	}
	if s.sess == nil {
		h.setRowLevel(sess, gm, s, n)
		return
	}
	if s.p != gm {
		log.Printf("GM: %s sets %s's level to %d", gm.Name, s.name, n)
	}
	h.setLevelFor(sess, s.sess, s.p, n)
}

// setRowLevel is #setlevel for a character out of the world: its row's level
// with no experience into it, under the cap its account's has (a GM's 100),
// read by its next login, which gives the training points the level is due
// (character.go's enterFromRecord).
func (h *ChatHandler) setRowLevel(sess *session.Session, gm *entity.Player, s gmSubject, level int) {
	if h.progress == nil || h.resets == nil {
		h.gmReply(sess, "Set level: this server keeps no characters")
		return
	}
	rec, err := h.resets.GetByServerID(s.serverID)
	if err != nil || rec == nil {
		h.gmReply(sess, fmt.Sprintf("Set level: no row for %s (%v)", s.name, err))
		return
	}
	isGM := false
	if h.access != nil {
		l, err := h.access.AccessLevel(rec.AccountID)
		isGM = err == nil && l >= 1
	}
	level = max(1, min(entity.LevelCap(isGM), level))
	if err := h.progress.SetProgress(s.serverID, level, entity.TotalXP(level, 0)); err != nil {
		log.Printf("GM: #setlevel %d for '%s' (not in the world) not kept: %v", level, s.name, err)
		h.gmReply(sess, fmt.Sprintf("%s's level not set: %v", s.name, err))
		return
	}
	log.Printf("GM: %s sets %s's level to %d (not in the world)", gm.Name, s.name, level)
	h.gmReply(sess, fmt.Sprintf("%s's level set to %d (not in the world: from its next login)", s.name, level))
}

// setLevelFor is #setlevel on player, whose session is sess, said to gm (the
// GM's own, another player's, or the GM's channel's Buff PC): the level where
// the game holds it, told to the client (0x0020 and the stat cap, as the GM
// panel tells it), and kept with no experience into it. What the level itself
// gives goes with it (its stats, the training points it is due, the
// heirlooms' figures); nothing else changes. The level was the live Player's
// alone once, so a level set was gone at the next login (Jibrish 60 in the
// game and 1 in the database, F3 pass 13). A player whose level a GM set is
// told.
func (h *ChatHandler) setLevelFor(gm, sess *session.Session, player *entity.Player, level int) {
	said := "Level"
	if gm != sess {
		said = player.Name + "'s level"
	}
	level, err := h.applyLevel(sess, player, level)
	if err != nil {
		h.gmReply(gm, fmt.Sprintf("%s set to %d, but not kept: %v", said, level, err))
		return
	}
	h.gmReply(gm, fmt.Sprintf("%s set to %d", said, level))
	if gm != sess {
		SendColoredChat(sess, fmt.Sprintf("A GM set your level to %d.", level), 255, 255, 0)
	}
}

// applyLevel is setLevelFor's work, said to no one (Dabrish's too,
// dabrish.go): player's level, under its account's cap, where the game holds
// it, told to its client and kept. It returns the level it came to and a
// failure to keep it.
func (h *ChatHandler) applyLevel(sess *session.Session, player *entity.Player, level int) (int, error) {
	level = max(1, min(entity.LevelCap(h.isGM(sess)), level)) // the account's, read now
	before := player.UnusedTP
	h.gameWorld.SetPlayerLevel(player.CharServerID, level)
	SendLevel(sess, level, h.gameWorld.CharacterBar(player.CharServerID), h.gameWorld.CharacterTrainerCap(player.CharServerID, level))
	// the training points the level is due (entity/training.go)
	if unused, total, ok := h.gameWorld.LiveTrainingPoints(player.ServerID); ok && unused != before {
		entity.SendTrainingPoints(sess, unused)
		if tp, ok := h.progress.(interface {
			SetTrainingPoints(uint32, int, int) error
		}); ok {
			if err := tp.SetTrainingPoints(player.CharServerID, unused, total); err != nil {
				log.Printf("Level %d for '%s': training points not kept: %v", level, player.Name, err)
			}
		}
	}
	if h.progress != nil {
		if err := h.progress.SetProgress(player.CharServerID, level, entity.TotalXP(level, 0)); err != nil {
			log.Printf("Level %d for '%s' not kept: %v", level, player.Name, err)
			return level, err
		}
	}
	return level, nil
}

// setTunar is #tunar: the purse where the game holds it, told to the client
// and kept (every other change of the purse is: merchant.go's saveTunar).
func (h *ChatHandler) setTunar(sess *session.Session, player *entity.Player, amount int) {
	amount = max(0, amount)
	// the bank's tunar is written only under bankMu (bank.go): as it is now
	bankMu.Lock()
	defer bankMu.Unlock()
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

// templateCommand is #template (template.go): a character at 60, its class's
// spells, 1,000,000 tunar and the best gear it may wear. The GM's own, or (the
// user, 2026-11-07: "let gms /template a player") the player it names, in the world
// now, who is told. The GM reads what it came to.
func (h *ChatHandler) templateCommand(gm *session.Session, args []string) {
	if h.template == nil {
		h.gmReply(gm, "Template: this server keeps no characters")
		return
	}
	sess := gm
	if len(args) > 0 {
		_, ts, ok := h.gmNamed(gm, args[0], "Template")
		if !ok {
			return
		}
		sess = ts
	}
	lines, err := h.template(sess)
	if err != nil {
		h.gmReply(gm, "Template: "+err.Error())
		return
	}
	if sess != gm {
		SendColoredChat(sess, "A GM gave your character the template: level 60, your class's spells, 1,000,000 tunar, the best gear you may wear, and mastery points.", 255, 255, 0)
	}
	// and the mastery points, to the most a character holds, spent and
	// unspent (the user: "let template also give 1500 CM points to the GM")
	if h.gameWorld != nil {
		switch give, m, ok, err := h.giveMastery(sess, entity.MasteryCap); {
		case !ok:
		case err != nil:
			lines = append(lines, fmt.Sprintf("  Mastery points: %d more, but not kept: %v", give, err))
		default:
			lines = append(lines, fmt.Sprintf("  Mastery points: %d more, %s", give, entity.MasteryLine(m)))
		}
	}
	for _, l := range lines {
		h.gmReply(gm, l)
	}
}

// npcInfoCommand is #npcinfo: the GM's target as EntityInfo has it, in a
// dialogue box (F3 20261025-171528: talking to an NPC with nothing to say
// showed it to everyone; it is a GM's now).
func (h *ChatHandler) npcInfoCommand(sess *session.Session, player *entity.Player) {
	id := player.TargetObjectID
	if id == 0 || id == 0xFFFFFFFF {
		h.gmReply(sess, "No target selected")
		return
	}
	target, found := h.gameWorld.FindEntityByObjectID(sess, id)
	if !found {
		h.gmReply(sess, "Target not found")
		return
	}
	SendDialogueBox(sess, 0, EntityInfo(target, id))
}

// giveItemMax is the most #giveitem gives at once (the GM panel's give_item's).
const giveItemMax = 1000

// giveItemCommand is #giveitem <item id> <player> [count] (petition 1046, the
// GM Jibrish: "Add a gm only command to give a player an item. /giveitem
// ITEMIDHERE PLAYERNAME should be the syntax"): count of the item (1 when not
// given), new and at full condition, into the inventory of the player online
// of that name, as the TEST menu's Give Item gives (gmGivePattern: stacked as
// a purchase is, the player told "A GM gave you ..."); a player not online is
// said so (the GM panel's give_item gives to one offline, at its next login).
func (h *ChatHandler) giveItemCommand(sess *session.Session, gm *entity.Player, args []string) {
	const usage = "Usage: #giveitem <item id> <player> [count]"
	if len(args) < 2 {
		h.gmReply(sess, usage)
		return
	}
	id, err := strconv.Atoi(args[0])
	if err != nil || id <= 0 {
		h.gmReply(sess, fmt.Sprintf("%q is no item id. %s", args[0], usage))
		return
	}
	n := 1
	if len(args) > 2 {
		if n, err = strconv.Atoi(args[2]); err != nil || n < 1 || n > giveItemMax {
			h.gmReply(sess, fmt.Sprintf("The count is 1 to %d. %s", giveItemMax, usage))
			return
		}
	}
	t := h.gameWorld.PlayerByName(args[1])
	if t == nil {
		h.gmReply(sess, fmt.Sprintf("No player named %q is online (the GM panel's give_item gives to one offline).", args[1]))
		return
	}
	to := SessionOfCharacter(h.gameWorld, h.sessionManager, t.CharServerID)
	if to == nil {
		h.gmReply(sess, fmt.Sprintf("%s is not in the world.", t.Name))
		return
	}
	h.gmGiveNow(sess, gm, to, id, n)
}
