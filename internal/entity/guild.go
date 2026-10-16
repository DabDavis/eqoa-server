package entity

import (
	"errors"
	"sort"
	"strings"
	"unicode/utf16"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// guild.go — guilds, held in memory while the server runs. A member is a
// character, by its database id (Player.CharServerID): 0x0468/3's member id
// is that id on Sandstorm too (Mcgregor's 25175 in pcaps/ps2_sandstorm.pcap,
// the MemoryDump's serverID). The members' Players carry GuildID and
// GuildName so that other code (chat routing) can tell who is in which.

// Guild ranks, as 0x0468/3 carries them (VIGuildMember's zigzag rank): 0 is a
// member, the lowest ("\"Member\" is the lowest rank and cannot be demoted",
// word 745), up to 3, the leader. Sandstorm's 250-member roster in the
// captures has 221 at 0, 21 at 1, 5 at 2 and 3 at 3.
const (
	GuildRankMember = 0
	GuildRankLeader = 3
)

// Guild is one guild.
type Guild struct {
	ID      uint32
	Name    string
	Charter string // the message 0x0468/1 carries after the name
	Members map[uint32]*GuildMember
}

// GuildMember is a member's record, kept while it is offline too.
type GuildMember struct {
	ID                 uint32 // the character's database id
	Name               string
	Class, Race, Level int
	Rank               int
}

var (
	ErrInGuild      = errors.New("already in a guild")
	ErrNotInGuild   = errors.New("not in a guild")
	ErrGuildName    = errors.New("guild name taken or empty")
	ErrNotQualified = errors.New("not qualified")
	ErrLowestRank   = errors.New("lowest rank")
	ErrNoSuchMember = errors.New("no such member")
)

// GuildMemberID is the id a character is a guild member by.
func GuildMemberID(p *Player) uint32 {
	if p.CharServerID != 0 {
		return p.CharServerID
	}
	return uint32(p.ServerID)
}

func memberRecord(p *Player, rank int) *GuildMember {
	return &GuildMember{ID: GuildMemberID(p), Name: p.Name, Class: p.Class, Race: p.Race, Level: p.Level, Rank: rank}
}

// GuildOf is p's guild, or nil.
func (w *World) GuildOf(p *Player) *Guild {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.guildOfLocked(p)
}

func (w *World) guildOfLocked(p *Player) *Guild {
	if p == nil || p.GuildID == 0 {
		return nil
	}
	return w.guilds[p.GuildID]
}

// CreateGuild makes a guild named name with leader as its leader.
func (w *World) CreateGuild(name string, leader *Player) (*Guild, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	name = strings.TrimSpace(name)
	if leader.GuildID != 0 {
		return nil, ErrInGuild
	}
	if name == "" {
		return nil, ErrGuildName
	}
	for _, g := range w.guilds {
		if strings.EqualFold(g.Name, name) {
			return nil, ErrGuildName
		}
	}
	if w.guilds == nil {
		w.guilds = make(map[uint32]*Guild)
	}
	w.nextGuildID++
	g := &Guild{ID: w.nextGuildID, Name: name, Members: map[uint32]*GuildMember{}}
	w.guilds[g.ID] = g
	w.joinLocked(g, leader, GuildRankLeader)
	return g, nil
}

// JoinGuild adds p to g as a member.
func (w *World) JoinGuild(g *Guild, p *Player) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if p.GuildID != 0 {
		return ErrInGuild
	}
	if w.guilds[g.ID] != g {
		return ErrNotInGuild
	}
	w.joinLocked(g, p, GuildRankMember)
	return nil
}

func (w *World) joinLocked(g *Guild, p *Player, rank int) {
	g.Members[GuildMemberID(p)] = memberRecord(p, rank)
	p.GuildID = g.ID
	p.GuildName = g.Name
	p.GuildRank = rank
	p.GuildMOTD = g.Charter
}

// LeaveGuild takes p out of its guild. A departing leader hands the guild to
// the highest-ranked member left (the earliest id among equals); the last one
// out ends it. It returns the guild, whether it ended and who leads it now
// (nil when unchanged or ended).
func (w *World) LeaveGuild(p *Player) (*Guild, bool, *GuildMember, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	g := w.guildOfLocked(p)
	if g == nil {
		return nil, false, nil, ErrNotInGuild
	}
	ended, lead := w.removeLocked(g, GuildMemberID(p))
	return g, ended, lead, nil
}

// RemoveGuildMember takes the member id out of g: by actor, who must outrank
// it. The removed member's Player, if online, is out of the guild too.
func (w *World) RemoveGuildMember(actor *Player, id uint32) (*Guild, *GuildMember, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	g := w.guildOfLocked(actor)
	if g == nil {
		return nil, nil, ErrNotInGuild
	}
	m := g.Members[id]
	if m == nil {
		return g, nil, ErrNoSuchMember
	}
	if a := g.Members[GuildMemberID(actor)]; a == nil || a.Rank <= m.Rank || m.ID == a.ID {
		return g, m, ErrNotQualified
	}
	w.removeLocked(g, id)
	return g, m, nil
}

// removeLocked drops member id from g, clears its Player, and passes the lead
// on or ends the guild.
func (w *World) removeLocked(g *Guild, id uint32) (ended bool, lead *GuildMember) {
	m := g.Members[id]
	delete(g.Members, id)
	if p := w.memberPlayerLocked(id); p != nil && p.GuildID == g.ID {
		clearGuild(p)
	}
	if len(g.Members) == 0 {
		delete(w.guilds, g.ID)
		return true, nil
	}
	if m != nil && m.Rank == GuildRankLeader && w.leaderLocked(g) == nil {
		lead = highest(g)
		lead.Rank = GuildRankLeader
		if p := w.memberPlayerLocked(lead.ID); p != nil {
			p.GuildRank = GuildRankLeader
		}
	}
	return false, lead
}

func clearGuild(p *Player) {
	p.GuildID = 0
	p.GuildName = ""
	p.GuildRank = 0
	p.GuildMOTD = ""
}

func (w *World) leaderLocked(g *Guild) *GuildMember {
	for _, m := range g.Members {
		if m.Rank == GuildRankLeader {
			return m
		}
	}
	return nil
}

// highest is g's highest-ranked member, the lowest id among equals.
func highest(g *Guild) *GuildMember {
	var best *GuildMember
	for _, m := range g.Members {
		if best == nil || m.Rank > best.Rank || (m.Rank == best.Rank && m.ID < best.ID) {
			best = m
		}
	}
	return best
}

// SetGuildRank moves member id one rank up (delta 1) or down (-1): by actor,
// who must outrank both where the member is and where it would go. The
// leader's rank is not given away here.
func (w *World) SetGuildRank(actor *Player, id uint32, delta int) (*Guild, *GuildMember, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	g := w.guildOfLocked(actor)
	if g == nil {
		return nil, nil, ErrNotInGuild
	}
	m := g.Members[id]
	if m == nil {
		return g, nil, ErrNoSuchMember
	}
	a := g.Members[GuildMemberID(actor)]
	to := m.Rank + delta
	switch {
	case a == nil || a.ID == m.ID || a.Rank <= m.Rank:
		return g, m, ErrNotQualified
	case to < GuildRankMember:
		return g, m, ErrLowestRank
	case to >= a.Rank || to >= GuildRankLeader:
		return g, m, ErrNotQualified
	}
	m.Rank = to
	if p := w.memberPlayerLocked(id); p != nil {
		p.GuildRank = to
	}
	return g, m, nil
}

// SetCharter sets g's charter, the message 0x0468/1 carries: the leader's
// to set (CLIENT's "Enter Guild Charter", word 1028, is the leader's menu).
func (w *World) SetCharter(actor *Player, text string) (*Guild, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	g := w.guildOfLocked(actor)
	if g == nil {
		return nil, ErrNotInGuild
	}
	if a := g.Members[GuildMemberID(actor)]; a == nil || a.Rank != GuildRankLeader {
		return g, ErrNotQualified
	}
	g.Charter = text
	for _, p := range w.Players {
		if p.GuildID == g.ID {
			p.GuildMOTD = text
		}
	}
	return g, nil
}

// DisbandGuild ends actor's guild, the leader's to do. It returns the
// members' Players that were online, all now out of it.
func (w *World) DisbandGuild(actor *Player) (*Guild, []*Player, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	g := w.guildOfLocked(actor)
	if g == nil {
		return nil, nil, ErrNotInGuild
	}
	if a := g.Members[GuildMemberID(actor)]; a == nil || a.Rank != GuildRankLeader {
		return g, nil, ErrNotQualified
	}
	var online []*Player
	for _, p := range w.Players {
		if p.GuildID == g.ID {
			online = append(online, p)
			clearGuild(p)
		}
	}
	delete(w.guilds, g.ID)
	return g, online, nil
}

// RosterEntry is a member with whether it is online now.
type RosterEntry struct {
	GuildMember
	Online bool
}

// GuildRoster is g's members, highest rank first, then by name.
func (w *World) GuildRoster(g *Guild) []RosterEntry {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.rosterLocked(g)
}

func (w *World) rosterLocked(g *Guild) []RosterEntry {
	var out []RosterEntry
	for _, m := range g.Members {
		p := w.memberPlayerLocked(m.ID)
		out = append(out, RosterEntry{GuildMember: *m, Online: p != nil && p.GuildID == g.ID})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Rank != out[j].Rank {
			return out[i].Rank > out[j].Rank
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// GuildMembers returns the online players of a guild.
func (w *World) GuildMembers(guildID uint32) []*Player {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var members []*Player
	for _, p := range w.Players {
		if p.GuildID == guildID && guildID != 0 {
			members = append(members, p)
		}
	}
	return members
}

// rejoinGuildLocked gives a returning character its guild back: a Player
// made anew at login carries none, while the guild kept its record. Called
// by CreatePlayer; guildAnnounceSystem tells the client.
func (w *World) rejoinGuildLocked(p *Player) {
	id := GuildMemberID(p)
	for _, g := range w.guilds {
		if m := g.Members[id]; m != nil {
			p.GuildID = g.ID
			p.GuildName = g.Name
			p.GuildRank = m.Rank
			p.GuildMOTD = g.Charter
			m.Name, m.Class, m.Race, m.Level = p.Name, p.Class, p.Race, p.Level
			return
		}
	}
}

// MarkGuildAnnounced records that p's client has its guild (a command handler
// that sent it).
func (w *World) MarkGuildAnnounced(p *Player) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.announced == nil {
		w.announced = make(map[*Player]bool)
	}
	w.announced[p] = true
}

// guildAnnounceSystem tells a member that has just entered the world its
// guild: 0x0468/1 and a 0x0468/3 per member, every 10 ticks for any in-world
// session not yet told — after its MemoryDump, which the selection sends at
// once. Must be called with the lock held.
func (w *World) guildAnnounceSystem() {
	if w.SessionManager == nil || w.TickCount%10 != 5 {
		return
	}
	w.SessionManager.ForEach(func(sess *session.Session) bool {
		if !sess.CharacterInWorld() {
			return true
		}
		p := w.sessionPlayerLocked(sess)
		if p == nil || w.announced[p] {
			return true
		}
		if w.announced == nil {
			w.announced = make(map[*Player]bool)
		}
		w.announced[p] = true
		g := w.guildOfLocked(p)
		if g == nil {
			return true
		}
		queueGuild(sess, BuildGuildInfo(g))
		for _, r := range w.rosterLocked(g) {
			queueGuild(sess, BuildGuildMember(r.GuildMember, r.Online))
		}
		// The others see it come online.
		if m := g.Members[GuildMemberID(p)]; m != nil {
			on := BuildGuildMember(*m, true)
			for _, o := range w.Players {
				if o != p && o.GuildID == g.ID {
					if s := w.findSessionByEntityIDLocked(o.ServerID); s != nil {
						queueGuild(s, on)
					}
				}
			}
		}
		return true
	})
}

func queueGuild(sess *session.Session, payload []byte) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeGuildInfo)
	msg.SetData(payload)
	sess.Queue().AddWithSequence(msg)
}

// GuildByID is the guild with that id, or nil.
func (w *World) GuildByID(id uint32) *Guild {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.guilds[id]
}

// GuildMemberPlayer is the online Player that is member id, or nil.
func (w *World) GuildMemberPlayer(id uint32) *Player {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.memberPlayerLocked(id)
}

// GuildMemberRecord is member id's record in g, copied, or false.
func (w *World) GuildMemberRecord(g *Guild, id uint32) (GuildMember, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if m := g.Members[id]; m != nil {
		return *m, true
	}
	return GuildMember{}, false
}

// memberPlayerLocked is the online Player that is member id, or nil.
func (w *World) memberPlayerLocked(id uint32) *Player {
	for _, p := range w.Players {
		if GuildMemberID(p) == id {
			return p
		}
	}
	return nil
}

// --- 0x0468, the guild message ------------------------------------------------
//
// VIClientGuild::Recv0468 (0x00BF0D38 [A]; the member reader
// operator>>(VIGuildMember) and UpdateMember [V]) takes a sub-type byte:
//
//	1  the guild's name and its charter, wide (≤ 63 and ≤ 255 units)
//	2  no guild: name, charter and members cleared
//	3  a member added or replaced: zigzag id, str8 name (≤ 23), zigzag class,
//	   race, level and rank, u8 online
//	4  a zigzag id: that member removed
//	5  a u32 and a wide line of guild chat, in (.25, 1, .8)
//	6-12, 16  a word: 578 created, 579 rank changed (7 and 8), 580 joined,
//	   581 charter changed, 582 disbanded, 583 removed, 1041 disbanded by an
//	   administrator
//	13/14  guild chat shown / hidden (647/648)
//	15  two str8 names: "%s has added %s to the guild." (717)
//
// Sandstorm's world entry (pcaps/ps2_sandstorm.pcap [C]) is one sub 1 and a
// sub 3 per member, 250 of them, the player's own among them.

// Guild word sub-types of 0x0468.
const (
	GuildWordCreated   byte = 6  // 578 "The group leader has created a guild."
	GuildWordRank      byte = 7  // 579 "Your guild rank has changed."
	GuildWordJoined    byte = 9  // 580 "Guild joined."
	GuildWordCharter   byte = 10 // 581 "The guild charter has changed."
	GuildWordDisbanded byte = 11 // 582 "The guild has been disbanded."
	GuildWordRemoved   byte = 12 // 583 "You have been removed from the guild."
)

func guildMsg(fill func(w *buffer.Writer)) []byte {
	buf := make([]byte, 2048)
	w := buffer.NewWriter(buf)
	w.WriteUint16(uint16(protocol.OpcodeGuildInfo))
	fill(w)
	return buf[:w.Position()]
}

// BuildGuildInfo is 0x0468/1: the name and the charter.
func BuildGuildInfo(g *Guild) []byte {
	return guildMsg(func(w *buffer.Writer) {
		w.WriteByte(1)
		writeWide(w, g.Name, 128)
		writeWide(w, g.Charter, 512)
	})
}

// BuildGuildReset is 0x0468/2: no guild.
func BuildGuildReset() []byte {
	return guildMsg(func(w *buffer.Writer) { w.WriteByte(2) })
}

// BuildGuildMember is 0x0468/3: one member's record.
func BuildGuildMember(m GuildMember, online bool) []byte {
	return guildMsg(func(w *buffer.Writer) {
		w.WriteByte(3)
		w.Write7BitEncodedInt64(int64(m.ID))
		writeStr8(w, m.Name, 24)
		w.Write7BitEncodedInt64(int64(m.Class))
		w.Write7BitEncodedInt64(int64(m.Race))
		w.Write7BitEncodedInt64(int64(m.Level))
		w.Write7BitEncodedInt64(int64(m.Rank))
		var on byte
		if online {
			on = 1
		}
		w.WriteByte(on)
	})
}

// BuildGuildRemoveMember is 0x0468/4: member id gone.
func BuildGuildRemoveMember(id uint32) []byte {
	return guildMsg(func(w *buffer.Writer) {
		w.WriteByte(4)
		w.Write7BitEncodedInt64(int64(id))
	})
}

// BuildGuildChat is 0x0468/5: a word CLIENT reads past, then the line.
func BuildGuildChat(speaker uint32, text string) []byte {
	return guildMsg(func(w *buffer.Writer) {
		w.WriteByte(5)
		w.WriteUint32(speaker)
		writeWide(w, text, 512)
	})
}

// BuildGuildWord is 0x0468 with a word sub-type alone (6-14, 16).
func BuildGuildWord(sub byte) []byte {
	return guildMsg(func(w *buffer.Writer) { w.WriteByte(sub) })
}

// BuildGuildAdded is 0x0468/15: who added whom.
func BuildGuildAdded(who, added string) []byte {
	return guildMsg(func(w *buffer.Writer) {
		w.WriteByte(15)
		writeStr8(w, who, 24)
		writeStr8(w, added, 24)
	})
}

// writeStr8 is a string as VIMemStream::ReadString(char *, maxBytes) reads
// one: a u32 count and the bytes, at most maxBytes-1 of them.
func writeStr8(w *buffer.Writer, s string, maxBytes int) {
	b := []byte(s)
	if len(b) > maxBytes-1 {
		b = b[:maxBytes-1]
	}
	w.WriteUint32(uint32(len(b)))
	w.WriteBytes(b)
}

// writeWide is a string as ReadString(unsigned short *, maxBytes) reads one:
// a u32 count of UTF-16 units and the units, at most maxBytes/2-1 of them.
func writeWide(w *buffer.Writer, s string, maxBytes int) {
	u := utf16.Encode([]rune(s))
	if n := maxBytes/2 - 1; len(u) > n {
		u = u[:n]
		if k := len(u); k > 0 && u[k-1] >= 0xD800 && u[k-1] < 0xDC00 {
			u = u[:k-1]
		}
	}
	w.WriteUint32(uint32(len(u)))
	for _, c := range u {
		w.WriteUint16(c)
	}
}
