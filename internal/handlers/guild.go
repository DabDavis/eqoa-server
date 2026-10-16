package handlers

import (
	"log"
	"sync"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// guild.go — guild commands, the way a PS2 client sends them: one opcode,
// 0x0578, and a sub-command byte (the VIClientPart2EF24 senders, 0x00BEEC08 ..
// 0x00BEF600 [A]; claude-notes/go-client-egress-pass-20261016/senders.md §4):
//
//	1 create {wide name}       2 disband        3 leave
//	4 promote / 5 demote / 6 remove {zigzag member id}
//	8 invite {str8 name, wide guild name}       9 join {u32 inviter}
//	10 refuse {u32 inviter, u8 reason: 1 declined, 2 busy, 4 in a guild}
//	11 charter {wide text}     12 chat {wide text}
//
// A failure is 0x0579 {u8 command, u8 reason} (Recv0579 0x00BEF788 [A]):
// create 1 "group members in guilds" (565), 2 "name belongs to another
// guild" (566); otherwise 5 "not qualified" (546), 8 "not in a guild" (570),
// 11 "the guild is full" (584), 12 "Member is the lowest rank" (745). An
// invitation is 0x057A (Recv057A 0x00BEF6D8 [A]): 1 to the invitee
// {zigzag, u32 inviter id, str8 name, wide guild}, 2 to the inviter {zigzag,
// str8 name} "%s has accepted your invitation." (557), 3 to the inviter {u8
// code, str8 name}: 1 declined (558), 2 busy (559), 3 cannot accept (560),
// 4 already in a guild (576). The guild itself is 0x0468 (entity/guild.go).
// The invented 0x0469-0x046F, which no PS2 sends, are gone.

const opGuildCommand = protocol.GameOpcode(0x0578)

// The 0x0578 sub-commands.
const (
	guildCmdCreate  byte = 1
	guildCmdDisband byte = 2
	guildCmdLeave   byte = 3
	guildCmdPromote byte = 4
	guildCmdDemote  byte = 5
	guildCmdRemove  byte = 6
	guildCmdInvite  byte = 8
	guildCmdJoin    byte = 9
	guildCmdRefuse  byte = 10
	guildCmdCharter byte = 11
	guildCmdChat    byte = 12
)

// 0x0579 reasons.
const (
	guildFailGrouped     byte = 1  // create: 565
	guildFailName        byte = 2  // create: 566
	guildFailNotAllowed  byte = 5  // 546
	guildFailNotInGuild  byte = 8  // 570
	guildFailLowestRank  byte = 12 // 745
	guildInviteDeclined  byte = 1  // 0x057A/3: 558
	guildInviteBusy      byte = 2  // 559
	guildInviteCannot    byte = 3  // 560
	guildInviteInAGuild  byte = 4  // 576
	guildOfficerRank          = 1  // the lowest rank that may invite [P]
	guildCharterMaxUnits      = 255
	guildNameMaxUnits         = 63
)

// GuildHandler handles the guild commands.
type GuildHandler struct {
	sessionManager *session.Manager
	gameWorld      *entity.World

	mu      sync.Mutex
	invites map[entity.EntityID]guildInvite // invitee → its waiting invitation
}

type guildInvite struct {
	inviter entity.EntityID
	guild   uint32
}

// NewGuildHandler creates a guild handler.
func NewGuildHandler(sessionMgr *session.Manager) *GuildHandler {
	return &GuildHandler{sessionManager: sessionMgr, invites: make(map[entity.EntityID]guildInvite)}
}

// SetGameWorld sets the world and hooks departures: the guild sees a member
// go offline, and invitations to and from it lapse.
func (h *GuildHandler) SetGameWorld(gw *entity.World) {
	h.gameWorld = gw
	if gw != nil {
		gw.OnPlayerLeaving(h.playerLeaving)
	}
}

// RegisterHandlers registers 0x0578.
func (h *GuildHandler) RegisterHandlers(r *Registry) {
	r.Register(opGuildCommand, h.HandleGuildCommand)
}

// HandleGuildCommand is 0x0578.
func (h *GuildHandler) HandleGuildCommand(sess *session.Session, msg *protocol.Message) error {
	if h.gameWorld == nil {
		return nil
	}
	p := h.gameWorld.SessionPlayer(sess)
	if p == nil {
		return nil
	}
	r := newClientReader(msg)
	sub := r.u8()
	if r.err {
		return nil
	}
	switch sub {
	case guildCmdCreate:
		h.create(sess, p, r.str16(guildNameMaxUnits))
	case guildCmdDisband:
		h.disband(sess, p)
	case guildCmdLeave:
		h.leave(sess, p)
	case guildCmdPromote, guildCmdDemote:
		delta := 1
		if sub == guildCmdDemote {
			delta = -1
		}
		h.rank(sess, p, sub, uint32(r.signed()), delta)
	case guildCmdRemove:
		h.remove(sess, p, uint32(r.signed()))
	case guildCmdInvite:
		name := r.str8(24)
		r.str16(64) // the guild's name, as the inviter's client has it
		h.invite(sess, p, name)
	case guildCmdJoin:
		h.join(sess, p, r.u32())
	case guildCmdRefuse:
		inviter := r.u32()
		h.refuse(sess, p, inviter, r.u8())
	case guildCmdCharter:
		h.charter(sess, p, r.str16(guildCharterMaxUnits))
	case guildCmdChat:
		text := r.str16(1023)
		if r.err || text == "" {
			return nil
		}
		// Recv0468/5 prints the line alone, so the speaker goes into it;
		// every online member hears it, the speaker too.
		g := h.gameWorld.GuildOf(p)
		if g == nil {
			queueReliable(sess, protocol.OpcodeGuildCommandFailed, buildGuildFailed(guildCmdChat, guildFailNotInGuild))
			return nil
		}
		h.broadcast(g, nil, entity.BuildGuildChat(0, p.Name+": "+text))
	default:
		log.Printf("Guild: sub-command %d from %s ignored", sub, p.Name)
	}
	return nil
}

func (h *GuildHandler) create(sess *session.Session, p *entity.Player, name string) {
	g, err := h.gameWorld.CreateGuild(name, p)
	switch err {
	case nil:
	case entity.ErrInGuild:
		queueReliable(sess, protocol.OpcodeGuildCommandFailed, buildGuildFailed(guildCmdCreate, guildFailGrouped))
		return
	default:
		queueReliable(sess, protocol.OpcodeGuildCommandFailed, buildGuildFailed(guildCmdCreate, guildFailName))
		return
	}
	log.Printf("Guild: %s created %q (%d)", p.Name, g.Name, g.ID)
	h.sendFullGuild(sess, g)
	queueReliable(sess, protocol.OpcodeGuildInfo, entity.BuildGuildWord(entity.GuildWordJoined))
	h.gameWorld.MarkGuildAnnounced(p)
}

func (h *GuildHandler) disband(sess *session.Session, p *entity.Player) {
	g, online, err := h.gameWorld.DisbandGuild(p)
	if h.failed(sess, guildCmdDisband, err) {
		return
	}
	log.Printf("Guild: %s disbanded %q", p.Name, g.Name)
	for _, m := range online {
		h.sendTo(m, entity.BuildGuildWord(entity.GuildWordDisbanded), entity.BuildGuildReset())
	}
}

func (h *GuildHandler) leave(sess *session.Session, p *entity.Player) {
	id := entity.GuildMemberID(p)
	g, ended, lead, err := h.gameWorld.LeaveGuild(p)
	if h.failed(sess, guildCmdLeave, err) {
		return
	}
	log.Printf("Guild: %s left %q", p.Name, g.Name)
	queueReliable(sess, protocol.OpcodeGuildInfo, entity.BuildGuildReset())
	if ended {
		return
	}
	h.broadcast(g, nil, entity.BuildGuildRemoveMember(id))
	h.announceLead(g, lead)
}

func (h *GuildHandler) rank(sess *session.Session, p *entity.Player, sub byte, id uint32, delta int) {
	g, m, err := h.gameWorld.SetGuildRank(p, id, delta)
	if err == entity.ErrLowestRank {
		queueReliable(sess, protocol.OpcodeGuildCommandFailed, buildGuildFailed(sub, guildFailLowestRank))
		return
	}
	if h.failed(sess, sub, err) {
		return
	}
	log.Printf("Guild: %s set %s to rank %d", p.Name, m.Name, m.Rank)
	h.broadcast(g, nil, entity.BuildGuildMember(*m, h.gameWorld.GuildMemberPlayer(m.ID) != nil))
	if t := h.gameWorld.GuildMemberPlayer(m.ID); t != nil {
		h.sendTo(t, entity.BuildGuildWord(entity.GuildWordRank))
	}
}

func (h *GuildHandler) remove(sess *session.Session, p *entity.Player, id uint32) {
	removed := h.gameWorld.GuildMemberPlayer(id)
	g, m, err := h.gameWorld.RemoveGuildMember(p, id)
	if h.failed(sess, guildCmdRemove, err) {
		return
	}
	log.Printf("Guild: %s removed %s from %q", p.Name, m.Name, g.Name)
	if removed != nil {
		h.sendTo(removed, entity.BuildGuildWord(entity.GuildWordRemoved), entity.BuildGuildReset())
	}
	h.broadcast(g, removed, entity.BuildGuildRemoveMember(id))
}

func (h *GuildHandler) invite(sess *session.Session, p *entity.Player, name string) {
	g := h.gameWorld.GuildOf(p)
	if g == nil {
		queueReliable(sess, protocol.OpcodeGuildCommandFailed, buildGuildFailed(guildCmdInvite, guildFailNotInGuild))
		return
	}
	if me, ok := h.gameWorld.GuildMemberRecord(g, entity.GuildMemberID(p)); !ok || me.Rank < guildOfficerRank {
		queueReliable(sess, protocol.OpcodeGuildCommandFailed, buildGuildFailed(guildCmdInvite, guildFailNotAllowed))
		return
	}
	invitee := h.gameWorld.PlayerByName(name)
	switch {
	case invitee == nil || invitee == p:
		queueReliable(sess, protocol.OpcodeGuildInviteStuff, buildGuildInviteResult(guildInviteCannot, name))
		return
	case invitee.GuildID != 0:
		queueReliable(sess, protocol.OpcodeGuildInviteStuff, buildGuildInviteResult(guildInviteInAGuild, invitee.Name))
		return
	}
	h.mu.Lock()
	h.invites[invitee.ServerID] = guildInvite{inviter: p.ServerID, guild: g.ID}
	h.mu.Unlock()
	log.Printf("Guild: %s invites %s to %q", p.Name, invitee.Name, g.Name)
	// the inviter's id as the invitee's client knows it (ids are per session);
	// what 9 or 10 sends back resolves through that same table
	if is := h.gameWorld.FindSessionByEntityID(invitee.ServerID); is != nil {
		queueReliable(is, protocol.OpcodeGuildInviteStuff, buildGuildInvitation(h.gameWorld.IDSeenBy(is, p), p.Name, g.Name))
	}
}

func (h *GuildHandler) join(sess *session.Session, p *entity.Player, inviterID uint32) {
	inv, ok := h.takeInvite(p.ServerID)
	inviter := h.gameWorld.PlayerSeenBy(sess, inviterID)
	if !ok || inviter == nil || inviter.ServerID != inv.inviter {
		return
	}
	g := h.gameWorld.GuildByID(inv.guild)
	if g == nil || h.gameWorld.JoinGuild(g, p) != nil {
		return
	}
	log.Printf("Guild: %s joined %q", p.Name, g.Name)
	h.sendFullGuild(sess, g)
	queueReliable(sess, protocol.OpcodeGuildInfo, entity.BuildGuildWord(entity.GuildWordJoined))
	h.gameWorld.MarkGuildAnnounced(p)
	if rec, ok := h.gameWorld.GuildMemberRecord(g, entity.GuildMemberID(p)); ok {
		h.broadcast(g, p, entity.BuildGuildMember(rec, true), entity.BuildGuildAdded(inviter.Name, p.Name))
	}
	h.sendRaw(inviter, protocol.OpcodeGuildInviteStuff, buildGuildInviteAccepted(p.Name))
}

func (h *GuildHandler) refuse(sess *session.Session, p *entity.Player, inviterID uint32, reason byte) {
	inv, ok := h.takeInvite(p.ServerID)
	inviter := h.gameWorld.PlayerSeenBy(sess, inviterID)
	if !ok || inviter == nil || inviter.ServerID != inv.inviter {
		return
	}
	code := guildInviteDeclined
	switch reason {
	case 2:
		code = guildInviteBusy
	case 4:
		code = guildInviteInAGuild
	}
	h.sendRaw(inviter, protocol.OpcodeGuildInviteStuff, buildGuildInviteResult(code, p.Name))
}

func (h *GuildHandler) charter(sess *session.Session, p *entity.Player, text string) {
	g, err := h.gameWorld.SetCharter(p, text)
	if h.failed(sess, guildCmdCharter, err) {
		return
	}
	log.Printf("Guild: %s set %q's charter", p.Name, g.Name)
	h.broadcast(g, nil, entity.BuildGuildInfo(g), entity.BuildGuildWord(entity.GuildWordCharter))
}

// failed answers a command's error with 0x0579 and reports whether there
// was one.
func (h *GuildHandler) failed(sess *session.Session, cmd byte, err error) bool {
	switch err {
	case nil:
		return false
	case entity.ErrNotInGuild:
		queueReliable(sess, protocol.OpcodeGuildCommandFailed, buildGuildFailed(cmd, guildFailNotInGuild))
	default:
		queueReliable(sess, protocol.OpcodeGuildCommandFailed, buildGuildFailed(cmd, guildFailNotAllowed))
	}
	return true
}

// announceLead tells the guild who leads it now, when that changed.
func (h *GuildHandler) announceLead(g *entity.Guild, lead *entity.GuildMember) {
	if lead == nil {
		return
	}
	t := h.gameWorld.GuildMemberPlayer(lead.ID)
	h.broadcast(g, nil, entity.BuildGuildMember(*lead, t != nil))
	if t != nil {
		h.sendTo(t, entity.BuildGuildWord(entity.GuildWordRank))
	}
}

// playerLeaving: the guild sees the member go offline; its invitations lapse.
func (h *GuildHandler) playerLeaving(p *entity.Player) {
	h.mu.Lock()
	delete(h.invites, p.ServerID)
	for invitee, inv := range h.invites {
		if inv.inviter == p.ServerID {
			delete(h.invites, invitee)
		}
	}
	h.mu.Unlock()
	g := h.gameWorld.GuildOf(p)
	if g == nil {
		return
	}
	if rec, ok := h.gameWorld.GuildMemberRecord(g, entity.GuildMemberID(p)); ok {
		h.broadcast(g, p, entity.BuildGuildMember(rec, false))
	}
}

func (h *GuildHandler) takeInvite(invitee entity.EntityID) (guildInvite, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	inv, ok := h.invites[invitee]
	delete(h.invites, invitee)
	return inv, ok
}

// sendFullGuild sends 0x0468/1 and a 0x0468/3 per member, as Sandstorm's
// world entry does.
func (h *GuildHandler) sendFullGuild(sess *session.Session, g *entity.Guild) {
	queueReliable(sess, protocol.OpcodeGuildInfo, entity.BuildGuildInfo(g))
	for _, r := range h.gameWorld.GuildRoster(g) {
		queueReliable(sess, protocol.OpcodeGuildInfo, entity.BuildGuildMember(r.GuildMember, r.Online))
	}
}

// broadcast sends 0x0468 messages to g's online members but skip.
func (h *GuildHandler) broadcast(g *entity.Guild, skip *entity.Player, msgs ...[]byte) {
	for _, m := range h.gameWorld.GuildMembers(g.ID) {
		if m != skip {
			h.sendTo(m, msgs...)
		}
	}
}

// sendTo queues 0x0468 messages to p.
func (h *GuildHandler) sendTo(p *entity.Player, msgs ...[]byte) {
	for _, m := range msgs {
		h.sendRaw(p, protocol.OpcodeGuildInfo, m)
	}
}

func (h *GuildHandler) sendRaw(p *entity.Player, op protocol.GameOpcode, payload []byte) {
	if sess := h.gameWorld.FindSessionByEntityID(p.ServerID); sess != nil {
		queueReliable(sess, op, payload)
	}
}

// buildGuildFailed is 0x0579: the command and why it failed.
func buildGuildFailed(cmd, reason byte) []byte {
	return build(protocol.OpcodeGuildCommandFailed, 4, func(w *buffer.Writer) {
		w.WriteByte(cmd)
		w.WriteByte(reason)
	})
}

// buildGuildInvitation is 0x057A/1, to the invitee: a zigzag Recv057A reads
// past (0), the inviter's id (what 9 and 10 send back), its name and the
// guild's.
func buildGuildInvitation(inviterID uint32, inviter, guild string) []byte {
	return build(protocol.OpcodeGuildInviteStuff, 256, func(w *buffer.Writer) {
		w.WriteByte(1)
		w.Write7BitEncodedInt64(0)
		w.WriteUint32(inviterID)
		writeString8(w, inviter, 24)
		writeWideString(w, guild, 128)
	})
}

// buildGuildInviteAccepted is 0x057A/2, to the inviter: "%s has accepted
// your invitation." (557).
func buildGuildInviteAccepted(name string) []byte {
	return build(protocol.OpcodeGuildInviteStuff, 64, func(w *buffer.Writer) {
		w.WriteByte(2)
		w.Write7BitEncodedInt64(0)
		writeString8(w, name, 24)
	})
}

// buildGuildInviteResult is 0x057A/3, to the inviter: why an invitation
// went nowhere.
func buildGuildInviteResult(code byte, name string) []byte {
	return build(protocol.OpcodeGuildInviteStuff, 64, func(w *buffer.Writer) {
		w.WriteByte(3)
		w.WriteByte(code)
		writeString8(w, name, 24)
	})
}
