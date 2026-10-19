package handlers

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// ChatHandler handles chat-related messages.
type ChatHandler struct {
	sessionManager *session.Manager
	gameWorld      *entity.World
}

// NewChatHandler creates a new chat handler.
func NewChatHandler(sessionMgr *session.Manager) *ChatHandler {
	return &ChatHandler{sessionManager: sessionMgr}
}

// SetGameWorld sets the game world reference (called after world init).
func (h *ChatHandler) SetGameWorld(gw *entity.World) {
	h.gameWorld = gw
}

// RegisterHandlers registers chat handlers with the registry.
func (h *ChatHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeClientSayChat, h.HandleSayChat)
	r.Register(protocol.OpcodeClientShout, h.HandleShoutChat)
	r.Register(protocol.OpcodeChangeChatMode, h.HandleChangeChatMode)
}

// HandleSayChat handles local say messages (broadcast to players within 200 units).
// C# reference: ChatMessage.ProcessClientChat → SayChat.ProcessSay
// Wire format in:  int32 charCount + UTF-16LE chars
// Wire format out: uint16 opcode + WriteStringUnicode(message) via OpcodeClientMessage
func (h *ChatHandler) HandleSayChat(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 4 {
		return nil
	}

	reader := buffer.NewReader(data)

	// C# ProcessClientChat: int32 messageLength at offset 0, then Unicode string
	charCount, _ := reader.ReadInt32()
	if charCount <= 0 {
		return nil
	}
	message, err := reader.ReadStringUnicode(int(charCount))
	if err != nil {
		log.Printf("SayChat: failed to read unicode message: %v", err)
		return nil
	}

	log.Printf("SayChat from %s: %s", sess.RemoteAddr(), message)

	if h.gameWorld == nil {
		return nil
	}

	// Get sender player
	player := h.gameWorld.SessionPlayer(sess)
	if player == nil {
		log.Printf("SayChat: sender has no player entity, playerObjectID=%d", sess.PlayerObjectID())
		return nil
	}

	// /hddsetup, for players who reach for the slash. The PS2 client eats most
	// /-commands locally, so this only ever fires when it forwards one as plain
	// chat — which costs nothing to support and saves explaining why the
	// spelling everyone tries is the one that does not work.
	if strings.HasPrefix(strings.ToLower(message), "/hddsetup") {
		hddSetupCommand(sess, player, strings.TrimSpace(message[len("/hddsetup"):]),
			func(t string) { h.gmReply(sess, t) }, hddChatConfirmHint)
		return nil
	}

	// GM commands start with #
	if strings.HasPrefix(message, "#") {
		h.handleGMCommand(sess, player, message)
		return nil
	}

	// Format: "CharName: message" (matching C# SayChat.ProcessSay)
	chatMessage := fmt.Sprintf("%s: %s", player.Name, message)

	// Build the outgoing message
	outMsg := buildClientMessage(chatMessage)

	// Query entities within 200 units (matching C# SayChat.Radius = 200.0f)
	nearby := h.gameWorld.SpatialIndex.QueryRadius(player.X, player.Z, 200.0)

	// Send to each nearby player session
	sentCount := 0
	for _, nearbyID := range nearby {
		nearSess := h.gameWorld.FindSessionByEntityID(nearbyID)
		if nearSess != nil {
			nearSess.Queue().AddWithSequence(outMsg)
			sentCount++
		}
	}

	// Ensure sender always gets their own message (they may not be in spatial index yet)
	if !h.entityIDInList(player.ServerID, nearby) {
		sess.Queue().AddWithSequence(outMsg)
		sentCount++
	}

	log.Printf("SayChat: sent '%s' to %d players", chatMessage, sentCount)
	return nil
}

// HandleShoutChat handles zone-wide shout messages (broadcast to all in-world players).
// C# reference: ShoutChat.ProcessShout
// Wire format in:  1 byte skip + int32 charCount + UTF-16LE chars
// Wire format out: uint16 opcode + WriteStringUnicode(message) + 4 color bytes via OpcodeColoredChat
func (h *ChatHandler) HandleShoutChat(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 5 {
		return nil
	}

	reader := buffer.NewReader(data)

	// C# ShoutChat.ProcessShout(Session, Message): skip 1 byte, then int32 messageLength at offset 1
	_, _ = reader.ReadByte() // chat mode flag byte
	charCount, _ := reader.ReadInt32()
	if charCount <= 0 {
		return nil
	}
	message, err := reader.ReadStringUnicode(int(charCount))
	if err != nil {
		log.Printf("ShoutChat: failed to read unicode message: %v", err)
		return nil
	}

	log.Printf("ShoutChat from %s: %s", sess.RemoteAddr(), message)

	if h.gameWorld == nil {
		return nil
	}

	// Get sender player
	player := h.gameWorld.SessionPlayer(sess)
	if player == nil {
		log.Printf("ShoutChat: sender has no player entity, playerObjectID=%d", sess.PlayerObjectID())
		return nil
	}

	// Format: "CharName shouts: message" (matching C# ShoutChat.ProcessShout)
	chatMessage := fmt.Sprintf("%s shouts: %s", player.Name, message)

	// Build colored message with green color bytes {0x3F, 0xFF, 0x3F, 0x00}
	outMsg := buildColoredMessage(chatMessage)

	// Broadcast to all in-world player sessions.
	sentCount := 0
	sentToSelf := false
	h.gameWorld.ForEachPlayer(func(p *entity.Player) {
		playerSess := h.gameWorld.FindSessionByEntityID(p.ServerID)
		if playerSess != nil {
			playerSess.Queue().AddWithSequence(outMsg)
			sentCount++
			if p.ServerID == player.ServerID {
				sentToSelf = true
			}
		}
	})

	// Ensure sender always gets the message
	if !sentToSelf {
		sess.Queue().AddWithSequence(outMsg)
		sentCount++
	}

	log.Printf("ShoutChat: sent '%s' to %d players", chatMessage, sentCount)
	return nil
}

// HandleChangeChatMode handles chat mode changes.
func (h *ChatHandler) HandleChangeChatMode(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 1 {
		return nil
	}

	reader := buffer.NewReader(data)
	mode, _ := reader.ReadByte()

	log.Printf("ChangeChatMode from %s: mode=%d", sess.RemoteAddr(), mode)

	return nil
}

// entityIDInList checks if an entity ID is in a list.
func (h *ChatHandler) entityIDInList(id entity.EntityID, list []entity.EntityID) bool {
	for _, e := range list {
		if e == id {
			return true
		}
	}
	return false
}

// buildClientMessage builds a 0x0A7A line (clientMessagePayload): white text,
// at most 1023 characters.
func buildClientMessage(message string) *protocol.Message {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeClientMessage)
	msg.SetData(clientMessagePayload(message))
	return msg
}

// buildColoredMessage builds a 0x0A7B line (coloredChatPayload) in green
// (0x3F, 0xFF, 0x3F). CLIENT reads three colour bytes; the C# reference's
// fourth was never read.
func buildColoredMessage(message string) *protocol.Message {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeColoredChat)
	msg.SetData(coloredChatPayload(message, 0x3F, 0xFF, 0x3F))
	return msg
}

// ChatType represents the type of chat message.
type ChatType byte

const (
	ChatTypeSay     ChatType = 0
	ChatTypeShout   ChatType = 1
	ChatTypeGroup   ChatType = 2
	ChatTypeGuild   ChatType = 3
	ChatTypeTell    ChatType = 4
	ChatTypeSystem  ChatType = 5
	ChatTypeError   ChatType = 6
	ChatTypeAuction ChatType = 7
)

// ChatColor represents chat message colors.
type ChatColor uint32

const (
	ColorWhite  ChatColor = 0xFFFFFFFF
	ColorRed    ChatColor = 0xFFFF0000
	ColorGreen  ChatColor = 0xFF00FF00
	ColorBlue   ChatColor = 0xFF0000FF
	ColorYellow ChatColor = 0xFFFFFF00
	ColorCyan   ChatColor = 0xFF00FFFF
	ColorPurple ChatColor = 0xFFFF00FF
	ColorOrange ChatColor = 0xFFFF8000
)

// SendChatMessage sends a chat message to a session using Unicode encoding.
func (h *ChatHandler) SendChatMessage(sess *session.Session, message string) {
	outMsg := buildClientMessage(message)
	sess.Queue().AddWithSequence(outMsg)
}

// SendColoredMessage sends a colored chat message using Unicode encoding.
func (h *ChatHandler) SendColoredMessage(sess *session.Session, message string) {
	outMsg := buildColoredMessage(message)
	sess.Queue().AddWithSequence(outMsg)
}

// SendErrorMessage sends a red error message.
func (h *ChatHandler) SendErrorMessage(sess *session.Session, message string) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeErrorMessage)
	writer := buffer.NewWriter(msg.Span())

	writer.WriteUint16(uint16(protocol.OpcodeErrorMessage))
	writer.WriteStringUnicode(message)

	msg.SetSize(writer.Position())
	sess.Queue().AddWithSequence(msg)
}

// SendSystemMessage sends a system message.
func (h *ChatHandler) SendSystemMessage(sess *session.Session, message string) {
	h.SendChatMessage(sess, message)
}

// SendGroupChat sends a message to all group members.
func (h *ChatHandler) SendGroupChat(sess *session.Session, message string, senderName string) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeGroupChat)
	writer := buffer.NewWriter(msg.Span())

	writer.WriteUint16(uint16(protocol.OpcodeGroupChat))
	writer.WriteStringUnicode(fmt.Sprintf("%s: %s", senderName, message))

	msg.SetSize(writer.Position())
	sess.Queue().AddWithSequence(msg)
}

// worldNames labels each world index for GM output. Index 9 is the one world
// that is not on the disc: it is streamed off the PS2 hard disk as
// pfs0:/T127.ESF, through the "hdd:" prefix our SD teaches VIFile::Open.
//
// INDEX 8 IS NO LONGER GUK. It was, while worlds came off the memory card; the
// SD we serve now hooks only "hdd:" and its entry 8 is a byte-identical clone
// of entry 2 (Odus), the placeholder that keeps an unclaimed index reading a
// real world. It is named for what it loads, because a label that says "Guk"
// while the client loads Odus is how the server and the client's table drift
// apart without anything failing — the same drift that put a world 9 NPC in
// world 0. Rename it the day something is put there.
//
// This list is also the RANGE CHECK for #goto and #tele, so a world
// the client's table can reach but this array cannot name is silently
// unreachable from the server side. It must stay the same length as
// handlers.worldPaths; TestEveryReachableWorldHasAPath pins that.
var worldNames = [WorldCount]string{
	"Tunaria", "Rathe", "Odus", "Lavastm", "Planesky",
	"Secrets", "world6", "Secrets2",
	// 8..29 are filled by init() with the SD's own naming convention.
}

// WorldCount is the length of the client's world table, which our SD builds and
// whose two `sltiu` bounds it raises to this number. Slots 0-7 come from
// Sandstorm's S2; 8 upward are ours, named worldN and loading hdd:worldN.ESF,
// so a new world is an ESF dropped into worlds/ rather than an SD rebuild.
//
// CHANGING THIS ALONE IS NOT ENOUGH — it must match
// `make_hdd_sd.py --world-count`. A server that names more worlds than the
// client's table holds sends players to an index the client indexes past the
// end of.
const WorldCount = 30

// WORLD 6 IS NOW THE HDD SETUP SCREEN, claimed 2026-08-11.
//
// It was dead and always had been: named "Underfoot", loading
// data3\Underfoot.esf, which is on no retail disc — an ISO 9660 walk finds
// TUNARIA, RATHE, ODUS, LAVASTM, PLANESKY and SECRETS, no UNDERFOOT — and in no
// patch set. Sandstorm's S2 ships the entry regardless. The note here used to
// say to claim it deliberately with `make_hdd_sd.py --world 6=` and rename it
// in the same change; this is that change.
//
// THE SCREEN IS GONE, AND SO IS THE SLOT'S SPECIAL MEANING. Its path was
// "hdd:HDDSETUP.ESF" and the SD payload's VIFile::Open hook watched for exactly
// that name: instead of opening anything it sceRead EQSCENE.BIN off the card and
// ExecPS2'd it. Everything that screen did to the disk was EQPREP's, and EQPREP
// is on the command channel -- check, create, format and repair, the last of
// which only the screen used to offer. So the hook, the token and the constant
// went together, and slot 6 is an ordinary unused world again.

func init() {
	for i := 8; i < WorldCount; i++ {
		worldNames[i] = fmt.Sprintf("world%d", i)
		worldPaths[uint8(i)] = fmt.Sprintf("hdd:world%d.ESF", i)
	}
}

// teleDest is one named shortcut for #tele.
//
// Each names the NPC that stands there wherever one does, and the NPC's row is
// then the single source of truth for where "there" is: `#tele freeport` and
// walking up to Coachman Ronks cannot disagree, and moving the NPC moves the
// shortcut with it. The literal coordinates are the fallback for when that NPC
// is not loaded — an unrecognised name in this table would otherwise silently
// become "teleport to the origin", which in world 9 is 12,000 units outside the
// only zone that has terrain.
type teleDest struct {
	NPC     string  // NPC whose spawn point defines this destination, if any
	X, Y, Z float32 // fallback, used only when that NPC is not loaded
	World   byte
	Label   string
}

// teleDests are the named GM teleport shortcuts, keyed lower-case.
var teleDests = map[string]teleDest{
	"freeport": {
		NPC: "Coachman Ronks",
		X:   25273.7, Y: 54.1, Z: 15719.1, World: 0,
		Label: "Freeport (Coachman Ronks)",
	},
	// Where every login lands. See loginSpawn in character.go: a character's
	// saved position is overridden with this on world entry, so the anchor NPC
	// is the single thing that decides where players start.
	"vaal": {
		NPC: "Archivist Vaal",
		X:   25385.7, Y: 54.1, Z: 15741.5, World: 0,
		Label: "Freeport (Archivist Vaal)",
	},
	// World 9 is Tunaria zone 127 (Widows Peak) lifted by cmd/zonelift and
	// streamed off the PS2 hard disk. The fallback Y is 400 rather than the
	// ground's 66.5 on purpose: it is the height every verified arrival there
	// has used, and dropping the last few units is how it was checked. Machado
	// stands on the measured ground at the same X/Z, so resolving through him
	// simply lands you the same place without the fall.
	"world9": {
		NPC: "Harbormaster Machado",
		X:   13000, Y: 400, Z: 23000, World: 9,
		Label: "Widows Peak (world 9, HDD)",
	},
}

// teleNames lists the shortcut names in a stable order — map iteration is
// randomised, and a help list that reshuffles every call is a small cruelty.
func teleNames() []string {
	names := make([]string, 0, len(teleDests))
	for n := range teleDests {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// worldName labels a world index for GM output, tolerating an out-of-range one
// rather than panicking on a value that only a bug could produce.
func worldName(w byte) string {
	if int(w) < len(worldNames) {
		return worldNames[w]
	}
	return "?"
}

// handleGMCommand processes server-side GM commands (start with #).
// All players are GM on the private server.
func (h *ChatHandler) handleGMCommand(sess *session.Session, player *entity.Player, message string) {
	parts := strings.Fields(message)
	if len(parts) == 0 {
		return
	}
	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "#goto":
		// #goto <x> <y> <z> [world]
		if len(parts) < 4 {
			h.gmReply(sess, "Usage: #goto <x> <y> <z> [world]")
			h.gmReply(sess, "  world: 0=Tunaria 1=Rathe 2=Odus 3=Lavastm 4=Planesky")
			h.gmReply(sess, "         5=Secrets 6=world6 7=Secrets2")
			return
		}
		x, _ := strconv.ParseFloat(parts[1], 32)
		y, _ := strconv.ParseFloat(parts[2], 32)
		z, _ := strconv.ParseFloat(parts[3], 32)
		world := byte(0)
		if len(parts) >= 5 {
			w, _ := strconv.Atoi(parts[4])
			if w >= 0 && w < len(worldNames) {
				world = byte(w)
			}
		}
		// Keep the player's world in step with where we just sent them, or
		// visibility keeps filtering against the old one and the destination
		// looks empty.
		worldChanged := player.World != world
		player.World = world
		h.gameWorld.UpdatePosition(player.ServerID, float32(x), float32(y), float32(z), player.Facing)
		sendTeleportGM(sess, float32(x), float32(y), float32(z), player.Facing, world)
		beginWorldLoad(sess, worldChanged)
		h.gmReply(sess, fmt.Sprintf("Teleported to (%.0f, %.0f, %.0f) world=%s", x, y, z, worldNames[world]))
		log.Printf("GM: %s teleported to (%.0f, %.0f, %.0f) world=%d", player.Name, x, y, z, world)

	case "#tele":
		// #tele <name> — teleport to a named place, so the coordinates of the
		// handful of destinations worth reaching repeatedly do not have to be
		// remembered or retyped. #goto is still the general form.
		if len(parts) < 2 {
			h.gmReply(sess, "Usage: #tele <name>")
			for _, n := range teleNames() {
				d := teleDests[n]
				h.gmReply(sess, fmt.Sprintf("  %s - %s", n, d.Label))
			}
			return
		}
		name := strings.ToLower(parts[1])
		if _, ok := teleDests[name]; !ok {
			h.gmReply(sess, fmt.Sprintf("Unknown place %q. Try: %s",
				parts[1], strings.Join(teleNames(), ", ")))
			return
		}
		// Prefer the NPC's own spawn point; fall back to the literal only when
		// that NPC is not loaded, and SAY SO — a silent fallback is how a
		// destination goes on working after the thing that defines it is gone.
		//
		// Shared with the account pages' teleport, which performs the same act
		// from outside the game: two resolvers would drift, and the drift would
		// be found by a player standing in the wrong place.
		x, y, z, world, via, _ := ResolveTeleDest(h.gameWorld, name)
		worldChanged := player.World != world
		player.World = world
		h.gameWorld.UpdatePosition(player.ServerID, x, y, z, player.Facing)
		sendTeleportGM(sess, x, y, z, player.Facing, world)
		beginWorldLoad(sess, worldChanged)
		h.gmReply(sess, fmt.Sprintf("Teleported to %s - (%.0f, %.0f, %.0f) world=%s",
			via, x, y, z, worldName(world)))
		log.Printf("GM: %s teleported to %s (%.1f, %.1f, %.1f) world=%d",
			player.Name, name, x, y, z, world)

	case "#world":
		// #world [n] — report or set the world the SERVER thinks you are in,
		// without moving you. The recovery hatch for the failure this exists
		// because of: a cross-world teleport whose world fails to LOAD leaves
		// the server filtering visibility to a world the client never reached,
		// so no NPC appears — including the travel NPC you would need to get
		// back. Without this the only fix is editing the database.
		if len(parts) < 2 {
			h.gmReply(sess, fmt.Sprintf("World: %d (%s) at (%.0f, %.0f, %.0f)",
				player.World, worldName(player.World), player.X, player.Y, player.Z))
			h.gmReply(sess, "Usage: #world <n> - set without teleporting")
			return
		}
		w, err := strconv.Atoi(parts[1])
		if err != nil || w < 0 || w >= len(worldNames) {
			h.gmReply(sess, fmt.Sprintf("Bad world; expected 0..%d", len(worldNames)-1))
			return
		}
		player.World = byte(w)
		h.gmReply(sess, fmt.Sprintf("World set to %d (%s). NPCs for that world "+
			"will populate on the next visibility tick.", w, worldName(byte(w))))
		log.Printf("GM: %s set world to %d", player.Name, w)

	case "#speed":
		// #speed <value>
		if len(parts) < 2 {
			h.gmReply(sess, fmt.Sprintf("Speed: %.1f. Usage: #speed <value>", player.Speed))
			return
		}
		speed, _ := strconv.ParseFloat(parts[1], 32)
		if speed < 0 {
			speed = 0
		}
		if speed > 100 {
			speed = 100
		}
		player.Speed = float32(speed)
		SendPlayerSpeed(sess, float32(speed))
		h.gmReply(sess, fmt.Sprintf("Speed set to %.1f", speed))

	case "#setlevel":
		// #setlevel <level>
		if len(parts) < 2 {
			h.gmReply(sess, fmt.Sprintf("Level: %d. Usage: #setlevel <level>", player.Level))
			return
		}
		level, _ := strconv.Atoi(parts[1])
		if level < 1 {
			level = 1
		}
		if level > 60 {
			level = 60
		}
		player.Level = level
		h.gmReply(sess, fmt.Sprintf("Level set to %d", level))

	case "#hp":
		// #hp <current> [max]
		if len(parts) < 2 {
			h.gmReply(sess, fmt.Sprintf("HP: %d/%d. Usage: #hp <current> [max]", player.HP, player.MaxHP))
			return
		}
		hp, _ := strconv.Atoi(parts[1])
		player.HP = hp
		if len(parts) >= 3 {
			maxhp, _ := strconv.Atoi(parts[2])
			player.MaxHP = maxhp
		}
		// HP reaches the console on the stat channel (0x42, m_stat[7] and
		// [8]), which statChannelSyncSystem re-serializes from the Player.
		// 0x0020 is level and exp: sending HP there set the PS2's LEVEL.
		h.gmReply(sess, fmt.Sprintf("HP set to %d/%d", player.HP, player.MaxHP))

	case "#tunar":
		// #tunar <amount>
		if len(parts) < 2 {
			h.gmReply(sess, fmt.Sprintf("Tunar: %d. Usage: #tunar <amount>", player.Tunar))
			return
		}
		amount, _ := strconv.Atoi(parts[1])
		player.Tunar = amount
		SendPlayerTunar(sess, int64(amount))
		h.gmReply(sess, fmt.Sprintf("Tunar set to %d", amount))

	case "#spawn":
		// #spawn — list nearby NPCs
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

	case "#kill":
		// #kill — kill targeted NPC
		if player.TargetObjectID == 0 || player.TargetObjectID == 0xFFFFFFFF {
			h.gmReply(sess, "No target selected")
			return
		}
		target, found := h.gameWorld.FindEntityByObjectID(sess, player.TargetObjectID)
		if !found {
			h.gmReply(sess, "Target not found")
			return
		}
		if _, ok := target.(*entity.NPC); ok {
			// ProcessAttack is one auto-attack swing now (attack.go), which
			// leaves most NPCs standing: kill outright.
			if name, ok := h.gameWorld.KillNPCByObjectID(sess, player.TargetObjectID); ok {
				h.gmReply(sess, fmt.Sprintf("Killed %s", name))
			} else {
				h.gmReply(sess, "Already dead")
			}
		} else {
			h.gmReply(sess, "Can only kill NPCs")
		}

	case "#pos":
		h.gmReply(sess, fmt.Sprintf("Position: (%.1f, %.1f, %.1f) facing=%.2f",
			player.X, player.Y, player.Z, player.Facing))

	case "#who":
		count := 0
		h.gameWorld.ForEachPlayer(func(p *entity.Player) {
			count++
			h.gmReply(sess, fmt.Sprintf("  %s Lv%d at (%.0f, %.0f, %.0f)",
				p.Name, p.Level, p.X, p.Y, p.Z))
		})
		h.gmReply(sess, fmt.Sprintf("%d players online", count))

	case "#hddsetup":
		// Also reachable as the Freeport setup NPC; both go through
		// hddSetupCommand so the format guard cannot exist in one and not the
		// other. `parts[1:]` rejoined, so "format confirm" arrives as one arg.
		hddSetupCommand(sess, player, strings.Join(parts[1:], " "),
			func(t string) { h.gmReply(sess, t) }, hddChatConfirmHint)

	case "#guildhall":
		// Teleport to guild hall (ARENA.ESF zone)
		// ARENA.ESF center: (0, 0, 0), bbox (-19,-4,-32) to (19,6,19)
		// Spawn in main hall area
		sendTeleportGM(sess, 12.0, 0.5, 14.0, 0, 0)
		h.gameWorld.UpdatePosition(player.ServerID, 12.0, 0.5, 14.0, player.Facing)
		beginWorldLoad(sess, player.World != 0)
		player.World = 0
		h.gmReply(sess, "Teleported to Guild Hall (ARENA.ESF)")
		log.Printf("GM: %s entered guild hall", player.Name)

	case "#help":
		h.gmReply(sess, "GM Commands:")
		h.gmReply(sess, "  #goto <x> <y> <z> [world] - teleport")
		h.gmReply(sess, "  #tele <name> - teleport to a named place (no name lists them)")
		h.gmReply(sess, "  #world [n] - show/set world without teleporting")
		h.gmReply(sess, "  #guildhall - enter guild hall")
		h.gmReply(sess, "  #hddsetup [check|create|format] - hard disk setup")
		h.gmReply(sess, "  #speed <val> - set speed")
		h.gmReply(sess, "  #setlevel <lvl> - set level")
		h.gmReply(sess, "  #hp <cur> [max] - set HP")
		h.gmReply(sess, "  #tunar <amt> - set gold")
		h.gmReply(sess, "  #spawn - list nearby NPCs")
		h.gmReply(sess, "  #kill - kill target")
		h.gmReply(sess, "  #pos - show position")
		h.gmReply(sess, "  #who - list players")

	default:
		h.gmReply(sess, fmt.Sprintf("Unknown GM command: %s (try #help)", cmd))
	}
}

// gmReply sends a system message back to the GM.
func (h *ChatHandler) gmReply(sess *session.Session, text string) {
	msg := buildClientMessage("[GM] " + text)
	sess.Queue().AddWithSequence(msg)
}

// sendTeleportGM sends a teleport packet for GM #goto.
// PS2 opcode 0x07F6: world(1) + x,y,z,facing(4×float32) + padding(8) + counter(4)
func sendTeleportGM(sess *session.Session, x, y, z, facing float32, world byte) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeTeleport)
	writer := buffer.NewWriter(msg.Span())
	writer.WriteUint16(uint16(protocol.OpcodeTeleport))
	writer.WriteByte(world)
	writer.WriteFloat32(x)
	writer.WriteFloat32(y)
	writer.WriteFloat32(z)
	writer.WriteFloat32(facing)
	writer.WriteUint32(0) // padding
	writer.WriteUint32(0) // padding
	writer.WriteUint32(0) // counter
	msg.SetSize(writer.Position())
	sess.Queue().AddWithSequence(msg)
}
