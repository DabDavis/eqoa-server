package handlers

import (
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// raidcmds.go — the raid's commands as clients send them (the GM Jibrish's petitions
// 1448, 1451, 1452 and 1453, 2026-11-10): the typed lines, everyone's, in their /, # and a
// PS2's "/t <word>" spellings (chat.go hands them here), and vanilla's raid offer, rolls
// and menu (protocol.OpcodeRaidOffer..OpcodeRaidCommand). The rules are the World's
// (entity/raidcmds.go, entity/raidrules.go) and the loot's (raidloot.go).
//
// A PS2's CLIENT keeps an unknown /command to itself unless it has an S2 (word 826), and
// neither of its keyboards types a #, but every CLIENT sends "/t" on as typed, so each
// command is also "/t <word> ...": the raid's words always, and /t invite, /t target, /t
// tar and /t help unless a player of that name is in the world (then it is a tell to
// them), /t need, /t greed and /t pass while the sender has a roll to answer, /t assign
// and /t abandon while it holds a corpse as master looter (else they are tells), as /t
// record and /t campfix go.

// RaidHandler takes vanilla's raid messages (0x7E43, 0x7E46, 0x7E48).
type RaidHandler struct {
	gameWorld *entity.World
	loot      *NPCWindowHandler
}

// NewRaidHandler is the raid's handler over the World and the loot's.
func NewRaidHandler(gw *entity.World, loot *NPCWindowHandler) *RaidHandler {
	return &RaidHandler{gameWorld: gw, loot: loot}
}

// RegisterHandlers registers the raid's messages.
func (h *RaidHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeRaidAnswer, h.handleAnswer)
	r.Register(protocol.OpcodeRaidRollChoice, h.handleRollChoice)
	r.Register(protocol.OpcodeRaidCommand, h.handleCommand)
	r.Register(protocol.OpcodeReadyCheckAnswer, h.handleReadyAnswer) // readycheck.go
	r.Register(protocol.OpcodeReadyCheckReply, h.handleReadyReply)
}

// buildRaidOffer is protocol.OpcodeRaidOffer: a u8, 1.
func buildRaidOffer() []byte {
	return build(protocol.OpcodeRaidOffer, 3, func(w *buffer.Writer) { w.WriteByte(1) })
}

// SendRaidOffer offers the session's client the raid's windows: one that shows them
// answers 0x7E43, a PS2 ignores it and is told everything in chat lines.
func SendRaidOffer(sess *session.Session) {
	queueReliable(sess, protocol.OpcodeRaidOffer, buildRaidOffer())
}

// handleAnswer is 0x7E43: a u8 (a longer number's first byte), nonzero to have the
// windows. Nothing is answered.
func (h *RaidHandler) handleAnswer(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 1 || h.gameWorld == nil {
		log.Printf("Raid: short answer from %s", sess.RemoteAddr())
		return nil
	}
	if !h.gameWorld.SetRaidWindows(sess, data[0] != 0) {
		log.Printf("Raid: %s has no player in the world", sess.RemoteAddr())
	}
	return nil
}

// handleRollChoice is 0x7E46: u32 roll id, u8 choice.
func (h *RaidHandler) handleRollChoice(sess *session.Session, msg *protocol.Message) error {
	r := buffer.NewReader(msg.Data())
	id, e1 := r.ReadUint32()
	c, e2 := r.ReadByte()
	if e1 != nil || e2 != nil || h.loot == nil || id == 0 {
		return nil
	}
	h.loot.RaidRollChoice(sess, int(id), rollChoice(c))
	return nil
}

// Vanilla's menu's raid commands (protocol.OpcodeRaidCommand's u8 op).
const (
	raidOpLoot     = 1
	raidOpLeave    = 2
	raidOpAssign   = 3
	raidOpAbandon  = 4
	raidOpInvite   = 5
	raidOpFlag     = 6
	raidOpAssist   = 7
	raidOpLeader   = 8
	raidOpHelper   = 9
	raidOpMainAsst = 10
	raidOpKick     = 11
	// petition 1687 (entity/raidsubgroups.go): 12 a member to a group (u32 the
	// member as the client knows it, u8 the group), 13 two members' groups traded
	// (u32, u32)
	raidOpGroup = 12
	raidOpSwap  = 13
	// petition 1879 (entity/instancemembers.go): 14 a player added to the party's
	// instance, 15 one taken out (u32 the player as the client knows it); the
	// group's menu's as much as the raid's
	raidOpInstAdd    = 14
	raidOpInstRemove = 15
)

// handleCommand is 0x7E48: u8 op and its arguments, as the typed commands do.
func (h *RaidHandler) handleCommand(sess *session.Session, msg *protocol.Message) error {
	gw := h.gameWorld
	r := buffer.NewReader(msg.Data())
	op, err := r.ReadByte()
	if err != nil || gw == nil {
		return nil
	}
	u32 := func() (uint32, bool) {
		v, err := r.ReadUint32()
		return v, err == nil
	}
	switch op {
	case raidOpLoot:
		if rule, err := r.ReadByte(); err == nil {
			gw.RaidLootRuleTo(sess, entity.LootRule(rule))
		}
	case raidOpLeave:
		gw.RaidLeave(sess)
	case raidOpAssign:
		key, ok1 := u32()
		id, ok2 := u32()
		if ok1 && ok2 && h.loot != nil {
			h.loot.MasterAssign(sess, int(key), id, "")
		}
	case raidOpAbandon:
		if h.loot != nil {
			h.loot.MasterAbandon(sess)
		}
	case raidOpInvite:
		if id, ok := u32(); ok {
			gw.RaidInvite(sess, "", id)
		}
	case raidOpFlag:
		gw.RaidCall(sess)
	case raidOpAssist:
		if id, ok := gw.RaidAssist(sess); ok {
			SendAssistTarget(sess, id)
		}
	case raidOpLeader, raidOpHelper, raidOpMainAsst, raidOpKick:
		id, ok := u32()
		if !ok {
			return nil
		}
		none := ""
		if id == 0 {
			none = "none"
		}
		switch op {
		case raidOpLeader:
			if id != 0 {
				gw.RaidLeader(sess, "", id)
			}
		case raidOpHelper:
			gw.RaidHelper(sess, none, id)
		case raidOpMainAsst:
			gw.RaidAssistSet(sess, none, id)
		case raidOpKick:
			if id != 0 {
				gw.RaidKick(sess, "", id)
			}
		}
	case raidOpGroup:
		id, ok := u32()
		n, err := r.ReadByte()
		if ok && err == nil {
			gw.RaidGroupMove(sess, "", id, int(n))
		}
	case raidOpSwap:
		a, ok1 := u32()
		b, ok2 := u32()
		if ok1 && ok2 {
			gw.RaidGroupSwap(sess, "", "", a, b)
		}
	case raidOpInstAdd, raidOpInstRemove:
		id, ok := u32()
		if !ok {
			return nil
		}
		if op == raidOpInstAdd {
			dungeonsTell(sess, gw.DungeonAddMember(sess, "", id))
		} else {
			dungeonsTell(sess, gw.DungeonRemoveMember(sess, "", id, time.Now()))
		}
	default:
		log.Printf("Raid: unknown command %d from %s", op, sess.RemoteAddr())
	}
	return nil
}

// raidLine routes a typed line that is a raid command, or /help, /invite, /target, /tar
// (in its / or # spelling): true when it was one. A GM's #help stays its own.
func (h *ChatHandler) raidLine(sess *session.Session, player *entity.Player, message string, target uint32) bool {
	word, rest, _ := strings.Cut(strings.TrimSpace(message), " ")
	w := strings.ToLower(word)
	if len(w) < 2 || w[0] != '/' && w[0] != '#' {
		return false
	}
	if w == "#help" && h.isGM(sess) {
		return false
	}
	return h.raidCommand(sess, player, w[1:], strings.TrimSpace(rest), target, false)
}

// raidTell routes a PS2's "/t <word> ..." spelling of a command (the file's doc): true
// when it was one, false for a tell.
func (h *ChatHandler) raidTell(sess *session.Session, player *entity.Player, name, body string) bool {
	cmd, rest := strings.ToLower(strings.TrimSpace(name)), strings.TrimSpace(body)
	switch cmd {
	case "raid":
		if rest != "" {
			return false // a tell to someone called Raid
		}
	case "raidaoe", "raidinvite", "raidleave", "raidkick", "raidleader", "raidhelper", "raidassist", "raidloot", "rsay", "raidgroup", "raidswap", "readycheck",
		"grouploot", "groupnbg":
	case "ready", "notready": // only while the sender has a ready check to answer
		if rest != "" || !h.gameWorld.HasReadyCheck(sess) {
			return false
		}
	case "invite", "target", "tar", "help":
		if findPlayerByName(h.gameWorld, cmd) != nil {
			return false // a tell to them
		}
	case "need", "greed", "pass":
		if h.raid == nil || h.raid.loot == nil || !h.raid.loot.HasRoll(sess, rollNumber(rest)) {
			return false
		}
	case "assign", "abandon":
		if h.raid == nil || h.raid.loot == nil || !h.raid.loot.HoldsMasterCorpse(sess) {
			return false
		}
	default:
		return false
	}
	return h.raidCommand(sess, player, cmd, rest, player.TargetObjectID, true)
}

// rollNumber is a typed roll's number, 0 for none (the oldest unanswered).
func rollNumber(s string) int {
	f := strings.Fields(s)
	if len(f) == 0 {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimPrefix(f[0], "#"))
	if err != nil || n < 1 {
		return -1
	}
	return n
}

// raidCommand runs one command (the file's doc), true when cmd is one. target is the
// sender's target as its client knows it.
func (h *ChatHandler) raidCommand(sess *session.Session, player *entity.Player, cmd, rest string, target uint32, tell bool) bool {
	gw := h.gameWorld
	if gw == nil {
		return false
	}
	if cmd == "raid" { // "/raid group <name> <n>", "/raid swap <a> <b>", "/raid groups"
		if sub, more, _ := strings.Cut(rest, " "); strings.EqualFold(sub, "group") || strings.EqualFold(sub, "groups") || strings.EqualFold(sub, "swap") {
			cmd, rest = "raid"+strings.TrimSuffix(strings.ToLower(sub), "s"), strings.TrimSpace(more)
		}
	}
	switch cmd {
	case "raid", "raidaoe":
		gw.RaidCall(sess)
	case "raidgroup":
		raidGroupLine(gw, sess, rest)
	case "raidswap":
		if f := strings.Fields(rest); len(f) == 2 {
			gw.RaidGroupSwap(sess, f[0], f[1], 0, 0)
		} else {
			SendErrorMessage(sess, "Usage: /raidswap <name> <name> - trades two members' groups.")
		}
	case "raidinvite":
		gw.RaidInvite(sess, rest, target)
	case "raidleave":
		gw.RaidLeave(sess)
	case "raidkick":
		gw.RaidKick(sess, rest, target)
	case "raidleader":
		gw.RaidLeader(sess, rest, 0)
	case "raidhelper":
		gw.RaidHelper(sess, rest, 0)
	case "raidassist":
		if rest != "" {
			gw.RaidAssistSet(sess, rest, 0)
		} else if id, ok := gw.RaidAssist(sess); ok {
			SendAssistTarget(sess, id)
		}
	case "raidloot":
		gw.RaidLootRule(sess, rest)
	case "grouploot", "groupnbg": // petition 1878 (entity/grouploot.go)
		gw.GroupLootRule(sess, rest)
	case "rsay":
		h.raidSay(sess, player, rest)
	case "readycheck": // petition 1711 (entity/readycheck.go)
		gw.ReadyCheck(sess)
	case "ready", "notready":
		gw.ReadyAnswer(sess, 0, cmd == "ready", true)
	case "invite":
		gw.InviteByName(sess, rest)
	case "target", "tar":
		targetByName(gw, sess, rest)
	case "help":
		h.playerHelp(sess)
	case "need", "greed", "pass":
		n := rollNumber(rest)
		if n < 0 || h.raid == nil || h.raid.loot == nil {
			SendErrorMessage(sess, "Usage: /need, /greed or /pass, and the roll's number (none: your oldest).")
			return true
		}
		choice := map[string]rollChoice{"need": rollNeed, "greed": rollGreed, "pass": rollPass}[cmd]
		h.raid.loot.RaidRollChoice(sess, n, choice)
	case "assign":
		f := strings.Fields(rest)
		n := 0
		if len(f) == 2 {
			n, _ = strconv.Atoi(strings.TrimPrefix(f[0], "#"))
		}
		if n < 1 || h.raid == nil || h.raid.loot == nil {
			SendErrorMessage(sess, "Usage: /assign <item number> <name> (a PS2: /t assign 1 Bob).")
			return true
		}
		h.raid.loot.MasterAssign(sess, n-1, 0, f[1])
	case "abandon":
		if h.raid != nil && h.raid.loot != nil {
			h.raid.loot.MasterAbandon(sess)
		}
	default:
		return false
	}
	log.Printf("Raid: %s's %q %q (tell %v)", player.Name, cmd, rest, tell)
	return true
}

// raidGroupLine is /raidgroup: alone, the raid's groups; "<name> <n>" a move (petition
// 1687, entity/raidsubgroups.go).
func raidGroupLine(gw *entity.World, sess *session.Session, rest string) {
	f := strings.Fields(rest)
	if len(f) == 0 {
		gw.RaidGroups(sess)
		return
	}
	n, err := strconv.Atoi(strings.TrimPrefix(f[len(f)-1], "#"))
	if len(f) != 2 || err != nil {
		SendErrorMessage(sess, "Usage: /raidgroup <name> <group number>; alone, the raid's groups.")
		return
	}
	gw.RaidGroupMove(sess, f[0], 0, n)
}

// raidSay is /rsay <text>: the raid's chat, the group's when it is no raid (petition 1448:
// "/rsay should also work to talk in raid chat, as an alternate to /group").
func (h *ChatHandler) raidSay(sess *session.Session, player *entity.Player, text string) {
	if strings.TrimSpace(text) == "" {
		SendErrorMessage(sess, "Usage: /rsay <text>")
		return
	}
	if len(h.gameWorld.GroupMembers(player)) == 0 {
		SendErrorMessage(sess, "You are not in a raid.")
		return
	}
	if h.gmChatGate(sess, player, chatModeName(ChatModeGroup), text) {
		h.groupChat(sess, player, plainText(text))
	}
}

// targetByName is /target <name> (/tar): the target the World finds (entity's TargetNear)
// as Msg007B takes one, CLIENT's own and every client's: m_target set and 0x0010 asked.
// None in reach: "No target in range" (petition 1452), and the target as it was.
func targetByName(gw *entity.World, sess *session.Session, name string) {
	if strings.TrimSpace(name) == "" {
		SendErrorMessage(sess, "Usage: /target <name> (or /tar): the nearest within 100 whose name starts so.")
		return
	}
	id, ok := gw.TargetNear(sess, name)
	if !ok {
		SendColoredChat(sess, entity.NoTargetInRange, 255, 63, 63)
		return
	}
	SendAssistTarget(sess, id)
}

// playerHelpLines are /help's lines (petition 1452: "/help should display all the raid
// commands available to players"), each a PS2's line's width or so.
var playerHelpLines = []string{
	"Commands for everyone (on a PS2 type /t and the word: /t raidinvite Bob):",
	"  /raidaoe (or /raid) - asks everyone within 60 in no group to your raid",
	"  /raidinvite [name] - invites a player, or your target, to your raid",
	"  /raidleave - leaves your raid (or group); /rsay <text> - talks to your raid",
	"  /raidkick <name> - takes a member out (the leader or the helper)",
	"  /raidleader [name] - hands the lead to a member; alone, shows the raid",
	"  /raidhelper <name|none> - the leader's one helper, who may invite and kick",
	"  /raidassist <name|none> - the leader names the main assist; alone, assists them",
	"  /raidloot ffa|needgreed|master - the leader's loot rule (not in combat)",
	"  /grouploot needgreed|ffa - a group leader's loot rule, as a raid's",
	"  /raidgroup - the raid's groups; /raidgroup <name> <n> moves one (leader, helper)",
	"  /raidswap <name> <name> - trades two members' groups (the leader or the helper)",
	"  /readycheck - asks your group or raid (its leader or helper) if all are ready",
	"  /ready, /notready - answers a ready check",
	"  /need [n], /greed [n], /pass [n] - a loot roll (no number: your oldest)",
	"  /assign <n> <name>, /abandon - the master looter gives item n, or lets all go",
	"  /invite <name> - invites a player to your group",
	"  /target <name> (or /tar) - targets the nearest within 100 whose name starts so",
	"  /tutorial - the welcome guide again",
	"  /who, /stuck, /roll, /pvp, /lfg on|off, /record [name], /bug <words>, /filter",
	"  /feature <words> - asks the GMs for something new (a feature request, apart from bugs)",
	"  /hide mercs, /show mercs - other players' mercenaries out of your view, or back",
}

// playerHelp is /help: playerHelpLines, and to a GM where its own commands are.
func (h *ChatHandler) playerHelp(sess *session.Session) {
	for _, l := range playerHelpLines {
		SendClientMessage(sess, l)
	}
	if h.isGM(sess) {
		SendClientMessage(sess, "  GM commands: #help")
	}
}
