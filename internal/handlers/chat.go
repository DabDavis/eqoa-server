package handlers

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Chat, as a PS2 sends it and expects it back.
//
// The client keeps a chat MODE, VIClient+0x382E4: GroupChanged (0x00BD9B48 [V])
// stores it, sends it as 0x000E and prints "Chat Mode: <mode>." (words
// 595-599). Plain typed text goes out in that mode: 0x000B in say, whisper and
// group, 0x0C01 sub 2 in shout, 0x0578 sub 12 in guild. A /command sets the
// mode for one line and restores it (/g: 0x000E{2}, 0x000B, 0x000E{kept}).
// A tell has no mode: /t and /tell send the WHOLE typed line as 0x000B, and /r
// sends "/t <the 0x07A3 name> <text>" the same way (the chat-line parser
// func_00BCFE78 [A]; claude-notes/go-client-egress-pass-20261016/senders.md
// section 3). So the server routes 0x000B by the line first, then by the mode
// the sender's last 0x000E named.
const (
	ChatModeSay     byte = 0
	ChatModeWhisper byte = 1
	ChatModeGroup   byte = 2
	ChatModeShout   byte = 3
	ChatModeGuild   byte = 4
)

// sayRadius is how far a say carries (the C# reference's SayChat.Radius).
const sayRadius = 200.0

// Tell colour, 0x0A7B: no capture holds a tell, so it is a choice [P].
const tellR, tellG, tellB = 0xFF, 0x7F, 0xFF

// ChatHandler handles chat-related messages.
type ChatHandler struct {
	sessionManager *session.Manager
	gameWorld      *entity.World

	mu    sync.Mutex
	modes map[entity.EntityID]byte // each player's last 0x000E

	access accessLevels // the accounts' acctlevel: who is a GM (devcmds.go)

	// where a GM's level and purse are kept, and #template (devcmds.go)
	progress progressStore
	purse    purseStore
	template func(*session.Session) ([]string, error)
	mastery  masteryStore         // #grantcm's (mastery.go)
	cm       *ClassMasteryHandler // #cm's and #cmreset's (classmastery.go)
	resets   resetStore           // #rst_mastr's, #rst_both's, #banktunar's rows (gmresets.go)

	visuals spellVisualStore // #fx's and #spellfx's spells (spellfx.go)

	fishing *FishingHandler // /fish (fishing.go)
	craft   *CraftHandler   // #craftskill's and #craftlearn's (craft.go)

	gm gmChannel // the GM's TEST menu (gmchannel.go)

	dyes     dyeStore     // #dye's (dye.go)
	vampires vampireStore // #vampire's (vampirecmd.go)
}

// NewChatHandler creates a new chat handler.
func NewChatHandler(sessionMgr *session.Manager) *ChatHandler {
	return &ChatHandler{sessionManager: sessionMgr, modes: make(map[entity.EntityID]byte)}
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
	r.Register(protocol.OpcodeProfanityFilter, h.HandleProfanityFilter)
}

// chatMode is the mode p's last 0x000E named, say before any.
func (h *ChatHandler) chatMode(p *entity.Player) byte {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.modes[p.ServerID]
}

// HandleChangeChatMode is 0x000E: GroupChanged's SendOpcodeByte(14, mode).
// Every world entry sends the kept mode again, so it is kept per character.
func (h *ChatHandler) HandleChangeChatMode(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 1 || h.gameWorld == nil {
		return nil
	}
	mode := data[0]
	player := h.gameWorld.SessionPlayer(sess)
	if player == nil {
		return nil
	}
	h.mu.Lock()
	h.modes[player.ServerID] = mode
	h.mu.Unlock()
	log.Printf("ChangeChatMode: %s mode=%d", player.Name, mode)
	return nil
}

// readChatLine reads 0x000B: func_00BE6188(11, text, m_target) [A], a wide
// string (u32 count of UTF-16 units, then the units) and the sender's target,
// a raw u32 (0xFFFFFFFF for none; absent from an old client). CLIENT sends no
// line of 128 units or more; anything past 1023 is refused here.
func readChatLine(data []byte) (text string, target uint32, ok bool) {
	reader := buffer.NewReader(data)
	n, err := reader.ReadInt32()
	if err != nil || n <= 0 || n > 1023 {
		return "", 0, false
	}
	text, err = reader.ReadStringUnicode(int(n))
	if err != nil {
		return "", 0, false
	}
	target = 0xFFFFFFFF
	if t, err := reader.ReadUint32(); err == nil {
		target = t
	}
	return text, target, true
}

// HandleSayChat is 0x000B: a typed line. Server commands first (/hddsetup and
// the GM's #), then a tell, then any other /command, then the sender's mode.
func (h *ChatHandler) HandleSayChat(sess *session.Session, msg *protocol.Message) error {
	message, target, ok := readChatLine(msg.Data())
	if !ok || h.gameWorld == nil {
		return nil
	}

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

	// /filter (or /profanity): the profanity filter turned over, everyone's
	// (profanity.go). The game's own toggle, action 0x0B052000, is in no
	// menu of the retail client, so a player had no way to it.
	if word, _, _ := strings.Cut(strings.ToLower(message), " "); word == "/filter" || word == "/profanity" {
		return h.HandleProfanityFilter(sess, nil)
	}

	// /fish (or #fish): a fishing session with the pole in the hand, everyone's
	// (fishing.go). A PS2 keeps an unknown /command to itself, so this comes
	// only from a client that forwards it; a PS2 uses the pole from a quick
	// slot instead.
	if word, _, _ := strings.Cut(strings.ToLower(message), " "); (word == "/fish" || word == "#fish") && h.fishing != nil {
		h.fishing.StartFishing(sess)
		return nil
	}

	// /duel (duel.go): everyone's, from a client that forwards it (a PS2
	// keeps an unknown /command to itself and challenges by "/tell <name>
	// duel" instead, below)
	if word, rest, _ := strings.Cut(message, " "); strings.EqualFold(word, "/duel") {
		duelCommand(h.gameWorld, sess, rest, target)
		return nil
	}

	// /lfg (or #lfg) on|off: the live group search's registration (lfg.go),
	// everyone's. The live client sends 0x0801 for its own /lfg; vanilla
	// forwards the line; a PS2 keeps an unknown /command to itself, so its
	// player types "#lfg on", which goes out as chat
	if word, rest, _ := strings.Cut(message, " "); strings.EqualFold(word, "/lfg") || strings.EqualFold(word, "#lfg") {
		lfgCommand(h.gameWorld, sess, strings.ToLower(word), rest)
		return nil
	}

	// /roll (or /random; #roll, #random from a PS2, which keeps an unknown
	// /command to itself): a random number for everyone in say range,
	// everyone's (roll.go)
	if word, rest, _ := strings.Cut(message, " "); isRollWord(word) {
		h.roll(sess, player, strings.Fields(rest))
		return nil
	}

	// GM commands start with #
	if strings.HasPrefix(message, "#") {
		h.handleGMCommand(sess, player, message)
		return nil
	}

	if name, body, isTell := parseTell(message); isTell {
		if wager, ok := duelTell(body); ok {
			// Sandstorm's challenge, "whisper "duel" to another player"
			// (duel.go): the challenge, not a tell
			h.gameWorld.DuelChallenge(sess, 0, name, wager, time.Now())
			return nil
		}
		if h.gmChatGate(sess, player, "tell "+name, body) {
			h.tell(sess, player, name, body)
		}
		return nil
	}

	// A GM's /command spelling of a # command (devcmds.go), from a client
	// that forwards the /commands it does not know.
	if gm := slashGM(message); gm != "" && h.isGM(sess) {
		h.handleGMCommand(sess, player, gm)
		return nil
	}

	// Any other /command reaches the server only from a client that forwards
	// what it does not know (Sandstorm's S2 does): answer the sender alone,
	// with retail's word 826, instead of saying it to the room.
	if strings.HasPrefix(message, "/") {
		word := message
		if i := strings.IndexByte(word, ' '); i >= 0 {
			word = word[:i]
		}
		SendErrorMessage(sess, fmt.Sprintf("Unrecognized command %q.", word))
		return nil
	}

	if !h.gmChatGate(sess, player, chatModeName(h.chatMode(player)), message) {
		return nil // a GM's Silence (gmchannel.go)
	}
	switch h.chatMode(player) {
	case ChatModeGroup:
		h.groupChat(sess, player, message)
	case ChatModeGuild:
		RouteGuildChat(h.gameWorld, sess, player, message)
	case ChatModeShout:
		h.shout(sess, player, message)
	case ChatModeWhisper:
		h.whisper(sess, player, target, message)
	default:
		h.say(sess, player, message)
	}
	return nil
}

// HandleShoutChat is 0x0C01, VIClientPart2EFCC's message: sub 2 is a shout
// (func_00CBBE10 [A]: u8 2, a wide string); sub 1 is func_00CBBEE0's u8 (a
// switch the client's menu sends, probably whether it hears shouts — its sense
// is not pinned, so it is only logged).
func (h *ChatHandler) HandleShoutChat(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 1 || h.gameWorld == nil {
		return nil
	}
	if data[0] != 2 {
		log.Printf("Shout: sub %d from %s: % X", data[0], sess.RemoteAddr(), data)
		return nil
	}
	message, _, ok := readChatLine(data[1:])
	if !ok {
		return nil
	}
	player := h.gameWorld.SessionPlayer(sess)
	if player == nil {
		log.Printf("ShoutChat: sender has no player entity, playerObjectID=%d", sess.PlayerObjectID())
		return nil
	}
	if !h.gmChatGate(sess, player, "shout", message) {
		return nil
	}
	h.shout(sess, player, message)
	return nil
}

// playerSessions maps every in-world player to its session, in one pass:
// World.FindSessionByEntityID walks every session for every lookup, and
// calling it inside ForEachPlayer takes the world's read lock twice.
func playerSessions(gw *entity.World) map[entity.EntityID]*session.Session {
	out := make(map[entity.EntityID]*session.Session)
	if gw == nil || gw.SessionManager == nil {
		return out
	}
	gw.SessionManager.ForEach(func(s *session.Session) bool {
		if !s.CharacterInWorld() {
			return true
		}
		if p := gw.SessionPlayer(s); p != nil {
			out[p.ServerID] = s
		}
		return true
	})
	return out
}

// hearer is a player who hears a line, and its session.
type hearer struct {
	s *session.Session
	p *entity.Player
}

// inSayRange is who hears player's say: every other player within sayRadius
// in its world, then the speaker itself.
func (h *ChatHandler) inSayRange(sess *session.Session, player *entity.Player) []hearer {
	sessions := playerSessions(h.gameWorld)
	var hear []hearer
	h.gameWorld.ForEachPlayer(func(p *entity.Player) {
		if p.World != player.World || p.ServerID == player.ServerID {
			return
		}
		dx, dz := float64(p.X-player.X), float64(p.Z-player.Z)
		if math.Sqrt(dx*dx+dz*dz) <= sayRadius {
			if s := sessions[p.ServerID]; s != nil {
				hear = append(hear, hearer{s, p})
			}
		}
	})
	return append(hear, hearer{sess, player})
}

// say is mode 0: "Name: text", white (0x0A7A, as Sandstorm says NPC lines
// [C]), to every player within sayRadius in the sender's world.
func (h *ChatHandler) say(sess *session.Session, player *entity.Player, message string) {
	line := fmt.Sprintf("%s: %s", player.Name, message)
	hear := h.inSayRange(sess, player)
	for _, l := range hear {
		l.s.Queue().AddWithSequence(buildClientMessage(lineFor(l.p, line)))
	}
	log.Printf("SayChat: %q to %d players", line, len(hear))
}

// shout is mode 3: "Name shouts: text" in (63, 255, 63), 0x0A7B, as Sandstorm
// sends it [C], to every player in the world.
func (h *ChatHandler) shout(sess *session.Session, player *entity.Player, message string) {
	line := fmt.Sprintf("%s shouts: %s", player.Name, message)
	n := 0
	self := false
	sessions := playerSessions(h.gameWorld)
	h.gameWorld.ForEachPlayer(func(p *entity.Player) {
		s := sessions[p.ServerID]
		if s == nil {
			return
		}
		s.Queue().AddWithSequence(buildColoredMessage(lineFor(p, line)))
		n++
		if p.ServerID == player.ServerID {
			self = true
		}
	})
	if !self {
		sess.Queue().AddWithSequence(buildColoredMessage(lineFor(player, line)))
		n++
	}
	log.Printf("ShoutChat: %q to %d players", line, n)
}

// groupChat is mode 2: 0x069E (the dispatcher [A]: a u32 CLIENT does not use,
// a wide line in (.5, .5, 1)) with the C# reference's "Name tells the group:"
// to every member, the sender included, but a member who chose Hide Chat on
// the Group menu (lfg.go). Not in a group: word 629.
func (h *ChatHandler) groupChat(sess *session.Session, player *entity.Player, message string) {
	members := h.gameWorld.GroupChatHearers(player)
	if len(members) == 0 {
		SendErrorMessage(sess, "You are not in a group.")
		return
	}
	line := fmt.Sprintf("%s tells the group: %s", player.Name, message)
	sessions := playerSessions(h.gameWorld)
	for _, m := range members {
		if s := sessions[m.ServerID]; s != nil {
			SendGroupChat(s, uint32(player.ServerID), lineFor(m, line))
		}
	}
	log.Printf("GroupChat: %q to %d members", line, len(members))
}

// guildChatLinePayload is 0x0468 sub 5, Recv0468 (0x00BF0D38 [A]): a u32 the
// client does not use, then a wide line of at most 255 characters, drawn in
// (.25, 1, .8).
func guildChatLinePayload(senderID uint32, text string) []byte {
	return build(protocol.OpcodeGuildInfo, 2+1+4+4+510, func(w *buffer.Writer) {
		w.WriteByte(5)
		w.WriteUint32(senderID)
		writeWideString(w, text, 512)
	})
}

// RouteGuildChat is guild chat from 0x000B in mode 4 (0x0578 sub 12 is the
// guild handler's): "Name: text" to every online member, the sender included,
// as the guild handler sends it.
// Not in a guild: an error to the sender alone.
func RouteGuildChat(gw *entity.World, sess *session.Session, player *entity.Player, message string) {
	if gw == nil || player == nil {
		return
	}
	if player.GuildID == 0 {
		SendErrorMessage(sess, "You are not in a guild.")
		return
	}
	// the guild handler's line (0x0578 sub 12): CLIENT prints it as it is
	line := player.Name + ": " + message
	sessions := playerSessions(gw)
	type member struct {
		s *session.Session
		p *entity.Player
	}
	var to []member
	gw.ForEachPlayer(func(p *entity.Player) {
		if p.GuildID == player.GuildID {
			if s := sessions[p.ServerID]; s != nil {
				to = append(to, member{s, p})
			}
		}
	})
	for _, m := range to {
		queueReliable(m.s, protocol.OpcodeGuildInfo, entity.BuildGuildChat(0, lineFor(m.p, line)))
	}
	log.Printf("GuildChat: %q to %d members", line, len(to))
}

// parseTell splits a /t or /tell line (the command word in any case, as the
// client's parser lower-cases it) into the name and the text.
func parseTell(line string) (name, text string, ok bool) {
	word, rest, _ := strings.Cut(line, " ")
	switch strings.ToLower(word) {
	case "/t", "/tell":
	default:
		return "", "", false
	}
	rest = strings.TrimLeft(rest, " ")
	name, text, _ = strings.Cut(rest, " ")
	return name, strings.TrimLeft(text, " "), true
}

// findPlayerByName is the in-world player named name (case-insensitive).
func findPlayerByName(gw *entity.World, name string) *entity.Player {
	var found *entity.Player
	gw.ForEachPlayer(func(p *entity.Player) {
		if found == nil && strings.EqualFold(p.Name, name) {
			found = p
		}
	})
	return found
}

// tell delivers a tell to the named player only: "Sender tells you, 'text'",
// with 0x07A3 naming the sender so the recipient's /r answers them
// (Msg07A3 [A]: ReadString(name, 24) into +0x4F0B8, the only writer of the
// reply name). The sender sees "You told Name, 'text'"; CLIENT echoes
// nothing itself. An unknown or offline name is an error to the sender alone.
// The texts are a choice [P]: no capture holds a tell.
func (h *ChatHandler) tell(sess *session.Session, from *entity.Player, name, text string) {
	if name == "" || text == "" {
		SendErrorMessage(sess, "Tell Failed")
		return
	}
	to := findPlayerByName(h.gameWorld, name)
	var toSess *session.Session
	if to != nil {
		toSess = playerSessions(h.gameWorld)[to.ServerID]
	}
	if toSess == nil {
		SendErrorMessage(sess, fmt.Sprintf("%s is not connected.", name))
		return
	}
	SendReplyName(toSess, from.Name)
	SendColoredChat(toSess, lineFor(to, fmt.Sprintf("%s tells you, '%s'", from.Name, text)), tellR, tellG, tellB)
	SendColoredChat(sess, lineFor(from, fmt.Sprintf("You told %s, '%s'", to.Name, text)), tellR, tellG, tellB)
	log.Printf("Tell: %s -> %s", from.Name, to.Name)
}

// whisper is mode 1, which no window of the client offers (its four modes
// are say, shout, group and guild): a 0x000B in it goes to the sender's
// target alone, if that is a player, and never to the room [P].
func (h *ChatHandler) whisper(sess *session.Session, from *entity.Player, target uint32, text string) {
	var to *entity.Player
	if target != 0xFFFFFFFF {
		if e, ok := h.gameWorld.FindEntityByObjectID(sess, target); ok {
			to, _ = e.(*entity.Player)
		}
	}
	if to == nil {
		SendErrorMessage(sess, "You must target a player to whisper.")
		return
	}
	toSess := playerSessions(h.gameWorld)[to.ServerID]
	if toSess == nil {
		SendErrorMessage(sess, fmt.Sprintf("%s is not connected.", to.Name))
		return
	}
	SendColoredChat(toSess, lineFor(to, fmt.Sprintf("%s whispers, '%s'", from.Name, text)), tellR, tellG, tellB)
	if toSess != sess {
		SendColoredChat(sess, lineFor(from, fmt.Sprintf("You whisper to %s, '%s'", to.Name, text)), tellR, tellG, tellB)
	}
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
//
// A camp's shortcut (campDests, camps.go) is one creature of many of its name:
// Near takes the NPC of that name in its world nearest the literal spawn
// (within campAnchorReach), so the camp's own creature, not a namesake across
// the world; Levels is the camp's creatures' levels, for the list.
type teleDest struct {
	NPC     string  // NPC whose spawn point defines this destination, if any
	X, Y, Z float32 // fallback, used only when that NPC is not loaded
	World   byte
	Label   string
	Near    bool   // the NPC of that name nearest X, Z in World (a camp's)
	Levels  string // a camp's creatures' levels here
}

// campAnchorReach is how far from its written spawn a camp's anchor may have
// moved and still be the camp's.
const campAnchorReach = 300

// findTeleDest is a #tele name's shortcut: a place's (teleDests), else a
// camp's by its name or an alias (campDests, campAliases), and the name it is
// listed by.
func findTeleDest(name string) (string, teleDest, bool) {
	key := strings.ToLower(strings.TrimSpace(name))
	if d, ok := teleDests[key]; ok {
		return key, d, true
	}
	if a, ok := campAliases[key]; ok {
		key = a
	}
	d, ok := campDests[key]
	return key, d, ok
}

// campNames lists the camps by their creatures' lowest level, then name.
func campNames() []string {
	names := make([]string, 0, len(campDests))
	for n := range campDests {
		names = append(names, n)
	}
	low := func(n string) int {
		v, _ := strconv.Atoi(strings.SplitN(campDests[n].Levels, "-", 2)[0])
		return v
	}
	sort.Slice(names, func(i, j int) bool {
		if a, b := low(names[i]), low(names[j]); a != b {
			return a < b
		}
		return names[i] < names[j]
	})
	return names
}

// campListLines is the camps as #tele lists them: "dfc 33-40" a camp, as
// many to a line as fit in width.
func campListLines(width int) []string {
	var lines []string
	line := ""
	for _, n := range campNames() {
		item := n + " " + campDests[n].Levels
		if line != "" && len(line)+2+len(item) > width {
			lines = append(lines, line)
			line = ""
		}
		if line != "" {
			line += ", "
		}
		line += item
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
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
	// The Plane of Disease's arrival (the Secrets world, 5) and the Dead
	// Hills (NW) of the Rathe (world 1), where Necrologist Zizclank sends a
	// player there (waysender.go); the fallbacks are where tools/seed_pod.py
	// puts them.
	"pod": {
		NPC: "Portal to the Dead Hills",
		X:   5063, Y: -25.3, Z: 3881, World: 5,
		Label: "the Plane of Disease (Portal to the Dead Hills)",
	},
	"deadhills": {
		NPC: "Necrologist Zizclank",
		X:   4280, Y: 97.8, Z: 6412, World: 1,
		Label: "the Dead Hills (Necrologist Zizclank)",
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

// handleGMCommand processes server-side GM commands (start with #): a GM's
// alone (devcmds.go: accountinfo.acctlevel 1 or more), but #hddsetup, which
// is everyone's.
func (h *ChatHandler) handleGMCommand(sess *session.Session, player *entity.Player, message string) {
	parts := strings.Fields(message)
	if len(parts) == 0 {
		return
	}
	cmd := strings.ToLower(parts[0])
	if cmd != "#hddsetup" && !h.isGM(sess) {
		SendErrorMessage(sess, fmt.Sprintf("Unrecognized command %q.", parts[0]))
		return
	}

	switch cmd {
	case "#goto", "#teleport":
		// #goto <x> <y> <z> [world] | <town or NPC name>. Keeping the
		// player's world in step with where it was sent matters: else
		// visibility keeps filtering against the old one and the destination
		// looks empty (teleportTo).
		h.gotoCommand(sess, player, cmd, parts[1:])

	case "#gotoplayer":
		h.gotoPlayerCommand(sess, player, parts[1:])

	case "#summon":
		h.summonCommand(sess, player, parts[1:])

	case "#despawn":
		h.despawnCommand(sess, player)

	case "#monstersearch":
		h.monsterSearchCommand(sess, parts[1:])

	case "#nearby":
		h.nearbyCommand(sess, player)

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
			h.gmReply(sess, "XP camps (their creatures' levels here; #tele camps says where each is):")
			for _, l := range campListLines(70) {
				h.gmReply(sess, "  "+l)
			}
			return
		}
		if strings.EqualFold(parts[1], "camps") {
			h.gmReply(sess, "XP camps, from EQOA-Data's map (#tele <name>):")
			for _, n := range campNames() {
				d := campDests[n]
				h.gmReply(sess, fmt.Sprintf("  %s %s - %s (%s)", n, d.Levels, d.Label, worldName(d.World)))
			}
			return
		}
		name, _, ok := findTeleDest(parts[1])
		if !ok {
			h.gmReply(sess, fmt.Sprintf("Unknown place %q. Try: %s, or a camp (#tele lists them)",
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
		if !CanLoadWorld(sess, world) {
			h.gmReply(sess, CannotLoad(player.Name, world))
			return
		}
		worldChanged := player.World != world
		player.World = world
		facing := player.Facing
		if _, d, _ := findTeleDest(name); d.Near && (d.X != x || d.Z != z) {
			// beside a camp: facing it (CLIENT's heading, VIVect3::HPR)
			facing = float32(math.Atan2(float64(d.X-x), float64(d.Z-z)))
		}
		h.gameWorld.UpdatePosition(player.ServerID, x, y, z, facing)
		sendTeleportGM(sess, x, y, z, facing, world)
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
		h.setLevel(sess, player, level)

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

	case "#debt":
		// #debt [amount]: the GM's experience debt, or set it (0 clears;
		// entity/deathdebt.go)
		if len(parts) < 2 {
			debt, _ := h.gameWorld.DebtOf(player.ServerID)
			h.gmReply(sess, fmt.Sprintf("Experience debt: %d (a death at level %d adds %d). Usage: #debt <amount>, 0 clears",
				debt, player.Level, entity.DeathDebt(player.Level)))
			if rez, left, _ := h.gameWorld.RezOf(player.ServerID, time.Now()); rez > 0 { // entity/resurrect.go
				h.gmReply(sess, fmt.Sprintf("A rez may give back a share of the %d your last death added, for %v more", rez, left.Round(time.Second)))
			}
			return
		}
		amount, err := strconv.Atoi(parts[1])
		if err != nil || amount < 0 {
			h.gmReply(sess, "Usage: #debt <amount>, 0 clears")
			return
		}
		h.gameWorld.SetDebt(player.ServerID, amount)
		h.gmReply(sess, fmt.Sprintf("Experience debt set to %d", amount))

	case "#tunar":
		// #tunar <amount>
		if len(parts) < 2 {
			h.gmReply(sess, fmt.Sprintf("Tunar: %d. Usage: #tunar <amount>", player.Tunar))
			return
		}
		amount, _ := strconv.Atoi(parts[1])
		h.setTunar(sess, player, amount)

	case "#spawn":
		// #spawn <name>: a copy of that NPC beside you; alone, the NPCs near
		h.spawnCommand(sess, player, parts[1:])

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

	case "#npcinfo":
		// #npcinfo: the target's ids, look, level, place and spawn, in a box
		// (what talking to a silent NPC showed before F3 20261025-171528)
		h.npcInfoCommand(sess, player)

	case "#stats":
		// #stats: what the server works out and the game does not show (HoT,
		// PoT, the stats' parts and caps, the rolls), on the target or the GM
		// (entity/statsreport.go; F3 20261026-014422)
		for _, l := range h.gameWorld.StatsReport(sess, player.TargetObjectID, time.Now()) {
			h.gmReply(sess, l)
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

	case "#effect":
		// #effect [<icon> [name] | clear]
		h.effectCommand(sess, parts[1:])

	case "#clearbuffs", "#nukebuffs":
		// #clearbuffs: every effect on the GM ended now, as its time running
		// out ends it (clearbuffscmd.go; F3 20261029-011146)
		h.clearBuffsCommand(sess)

	case "#faction":
		// #faction <faction> [standing]: your standing, or set it
		h.factionCommand(sess, parts[1:])
	case "#template":
		// level 60, every class spell, 1,000,000 tunar, the best gear worn
		// (template.go)
		h.templateCommand(sess)
	case "#grantcm":
		// #grantcm <amount>: mastery points, never past 1500 in all
		// (mastery.go)
		h.grantMasteryCommand(sess, parts[1:])
	case "#cm":
		// #cm: the masteries the target (a player) or the GM bought
		// (classmastery.go)
		if h.cm == nil {
			h.gmReply(sess, "#cm: masteries are not served here")
			break
		}
		for _, l := range h.cm.GMLines(sess, player.TargetObjectID) {
			h.gmReply(sess, l)
		}
	case "#cmreset":
		// #cmreset: every mastery of the target (a player) or the GM taken
		// away, what they cost given back
		if h.cm == nil {
			h.gmReply(sess, "#cmreset: masteries are not served here")
			break
		}
		h.gmReply(sess, h.cm.GMReset(sess, player.TargetObjectID))
	case "#aggro":
		// #aggro [on|off]: whether creatures attack you on sight (aggrocmd.go)
		h.aggroCommand(sess, parts[1:])
	case "#fx":
		// #fx <visual> [seconds] | <spell>: a spell visual played on the
		// target now, nothing saved (spellfx.go)
		h.fxCommand(sess, parts[1:])
	case "#spellfx":
		// #spellfx <spell> [<visual> | 0]: a spell's visual, said or set
		// (spellfx.go)
		h.spellFXCommand(sess, parts[1:])
	case "#trigger":
		// #trigger [n | name | stop [n]]: the GMs' scripted events
		// (gmtriggers.go)
		h.triggerCommand(sess, parts[1:])
	case "#craftskill":
		// #craftskill [<skill 0-5> <level 1-10> [rank 0-9]]: the GM's own
		// tradeskill, said or set (craft.go)
		h.craftSkillCommand(sess, parts[1:])
	case "#craftlearn":
		// #craftlearn <recipe id | all [skill]>: recipes in the GM's own
		// book (craft.go)
		h.craftLearnCommand(sess, parts[1:])
	case "#rst_mastr", "#rst_both":
		// #rst_mastr / #rst_both [name]: the live client's resets, masteries
		// (and training points) given back (gmresets.go)
		h.resetCommand(sess, player, strings.Join(parts[1:], " "), cmd == "#rst_both")
	case "#rem_abil":
		// #rem_abil <name> <ability>: an ability out of a book (gmresets.go)
		h.removeAbilityCommand(sess, player, strings.Join(parts[1:], " "))
	case "#banktunar":
		// #banktunar [name] <n>: n added to a bank (gmresets.go)
		h.bankTunarCommand(sess, player, parts[1:])
	case "#servdown":
		// #servdown: the server restarted, an admin's, asked twice (gmresets.go)
		h.servDownCommand(sess, player)
	case "#dye":
		// #dye [<slot|armor|all> <colour|none>]: the target's (a player) or
		// the GM's dyes, drawn by clients of the live game's look (dye.go)
		h.dyeCommand(sess, parts[1:])
	case "#vampire":
		// #vampire [on|off]: whether the target (a player) or the GM is a
		// vampire, whom the sun burns by day, and what the sun does to it
		// now (vampirecmd.go)
		h.vampireCommand(sess, parts[1:])

	case "#help":
		h.gmReply(sess, "GM Commands (each also as /name):")
		h.gmReply(sess, "  #goto <x> <y> <z> [world] | <town or NPC> - teleport (#teleport too)")
		h.gmReply(sess, "  #gotoplayer <name> - go to an online player")
		h.gmReply(sess, "  #summon <name> - bring an online player to you")
		h.gmReply(sess, "  #spawn <name> - a copy of that NPC beside you; #despawn takes the target away")
		h.gmReply(sess, "  #monstersearch <start of a name> - up to ten NPC names")
		h.gmReply(sess, "  #nearby - NPCs within 200")
		h.gmReply(sess, "  #tele <name> - teleport to a named place (no name lists them)")
		h.gmReply(sess, "  #world [n] - show/set world without teleporting")
		h.gmReply(sess, "  #guildhall - enter guild hall")
		h.gmReply(sess, "  #hddsetup [check|create|format] - hard disk setup")
		h.gmReply(sess, "  #speed <val> - set speed")
		h.gmReply(sess, "  #setlevel <lvl> - set level")
		h.gmReply(sess, "  #hp <cur> [max] - set HP")
		h.gmReply(sess, "  #tunar <amt> - set gold")
		h.gmReply(sess, "  #template - level 60, every spell of your class, 1,000,000 tunar, the best gear you may wear")
		h.gmReply(sess, "  #effect [<icon hex> [name] | clear] - list, add or clear the effects channel's slots (the spells behind them go on)")
		h.gmReply(sess, "  #clearbuffs - every buff on you ended now, as its time running out ends it (#nukebuffs too)")
		h.gmReply(sess, "  #faction <faction> [standing] - your standing with a faction, or set it")
		h.gmReply(sess, "  #grantcm <amount> - mastery points, never past 1500 spent and unspent")
		h.gmReply(sess, "  #debt [amount] - your experience debt, or set it (0 clears)")
		h.gmReply(sess, "  #cm - the masteries your target (a player) or you bought; #cmreset takes them all back, points returned")
		h.gmReply(sess, "  #rst_mastr [name] - a character's masteries all taken back, points returned (no name: your target, else you)")
		h.gmReply(sess, "  #rst_both [name] - that and every training point given back")
		h.gmReply(sess, "  #rem_abil <name> <ability> - an ability (its name or spell id) out of a character's book")
		h.gmReply(sess, "  #banktunar [name] <amount> - added to a bank (negative takes away; no name: your target, else you)")
		h.gmReply(sess, "  #servdown - restart the server, everyone saved (an admin's; say it twice)")
		h.gmReply(sess, "  #aggro [on|off] - whether creatures attack you on sight (off until #aggro on)")
		h.gmReply(sess, "  #fx <visual> [seconds] | <spell> - play a spell visual (0x1DAEB5B6) on your target, nothing saved")
		h.gmReply(sess, "  #spellfx <spell> [<visual> | 0] - a spell's visual, or give it one (0: its stand-in)")
		h.gmReply(sess, "  #trigger [n | name | stop [n]] - the triggers (the GM panel's), fire one, or stop their runs")
		h.gmReply(sess, "  #craftskill [<skill 0-5> <level 1-10> [rank 0-9]] - your tradeskills, or set one")
		h.gmReply(sess, "  #craftlearn <recipe id | all [skill 0-5]> - recipes into your Recipe Book")
		h.gmReply(sess, "  #dye [<slot|armor|all> <RRGGBB|tN|name|none>] - your target's (a player) or your dyes, seen by clients of the live game's look")
		h.gmReply(sess, "  #vampire [on|off] - whether your target (a player) or you is a vampire, burned by day; what the sun does to it now")
		h.gmReply(sess, "  #kill - kill target")
		h.gmReply(sess, "  #npcinfo - the target's ids, model, size, level, spawn and gear")
		h.gmReply(sess, "  #stats - HoT, PoT, stats and caps, the melee rolls, hate: the target's or your own")
		h.gmReply(sess, "  #pos - show position")
		h.gmReply(sess, "  #who - list players")

	default:
		h.gmReply(sess, fmt.Sprintf("Unknown GM command: %s (try #help)", cmd))
	}
}

// effectCommand is #effect: with nothing after it, the player's active
// effects; `clear` takes them all off; `<icon> [name]` puts one on, the icon
// a DictID in hex as VIWndInfluence blits it. They reach the client on state
// channel 0x43 within buffChannelSyncSystem's two seconds.
func (h *ChatHandler) effectCommand(sess *session.Session, args []string) {
	const usage = "Usage: #effect <icon hex> [name] | #effect clear"
	switch {
	case len(args) == 0:
		fx := h.gameWorld.PlayerEffects(sess)
		if len(fx) == 0 {
			h.gmReply(sess, "No effects. "+usage)
		}
		for i, e := range fx {
			h.gmReply(sess, fmt.Sprintf("  %d: %08X %s", i+1, e.Icon, e.Name))
		}
	case strings.EqualFold(args[0], "clear"):
		h.gameWorld.ClearEffects(sess)
		h.gmReply(sess, "Effects cleared")
	default:
		e, err := parseEffect(args)
		if err != nil {
			h.gmReply(sess, usage)
			return
		}
		if !h.gameWorld.AddEffect(sess, e) {
			h.gmReply(sess, fmt.Sprintf("There are %d effects already (the client keeps no more)", entity.MaxEffects))
			return
		}
		h.gmReply(sess, fmt.Sprintf("Effect %08X %q on", e.Icon, e.Name))
	}
}

// parseEffect reads #effect's `<icon> [name]`: the icon a DictID in hex (0x
// optional), the rest of the words the name.
func parseEffect(args []string) (entity.BuffSlot, error) {
	if len(args) == 0 {
		return entity.BuffSlot{}, fmt.Errorf("no icon")
	}
	icon, err := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(args[0]), "0x"), 16, 32)
	if err != nil {
		return entity.BuffSlot{}, err
	}
	return entity.BuffSlot{Icon: uint32(icon), Name: strings.Join(args[1:], " ")}, nil
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
