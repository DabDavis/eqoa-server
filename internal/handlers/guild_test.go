package handlers

import (
	"encoding/binary"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// The guild messages, read back the way CLIENT reads them (Recv0468
// 0x00BF0D38, Recv0579 0x00BEF788, Recv057A 0x00BEF6D8), and the 0x0578
// commands driven end to end.

func TestGuildMemberRecordIsRecv0468Sub3(t *testing.T) {
	m := entity.GuildMember{ID: 25175, Name: "Mcgregor", Class: 5, Race: 0, Level: 51, Rank: 3}
	r := open(t, entity.BuildGuildMember(m, true), protocol.OpcodeGuildInfo)
	if sub := r.u8(); sub != 3 {
		t.Fatalf("sub %d", sub)
	}
	if id, name := r.signed(), r.str8(24); id != 25175 || name != "Mcgregor" {
		t.Errorf("id %d name %q", id, name)
	}
	if c, ra, lv, rk := r.signed(), r.signed(), r.signed(), r.signed(); c != 5 || ra != 0 || lv != 51 || rk != 3 {
		t.Errorf("class %d race %d level %d rank %d", c, ra, lv, rk)
	}
	if on := r.u8(); on != 1 {
		t.Errorf("online %d", on)
	}
	r.done(t)
}

func TestGuildInfoAndSmallSubs(t *testing.T) {
	r := open(t, entity.BuildGuildInfo(&entity.Guild{Name: "Prophets of Zion", Charter: "Be kind"}), protocol.OpcodeGuildInfo)
	if sub, name, motd := r.u8(), r.wide(128), r.wide(512); sub != 1 || name != "Prophets of Zion" || motd != "Be kind" {
		t.Errorf("sub %d name %q charter %q", sub, name, motd)
	}
	r.done(t)
	r = open(t, entity.BuildGuildRemoveMember(70000), protocol.OpcodeGuildInfo)
	if sub, id := r.u8(), r.signed(); sub != 4 || id != 70000 {
		t.Errorf("remove: %d %d", sub, id)
	}
	r.done(t)
	r = open(t, entity.BuildGuildAdded("Ann", "Bob"), protocol.OpcodeGuildInfo)
	if sub, a, b := r.u8(), r.str8(24), r.str8(24); sub != 15 || a != "Ann" || b != "Bob" {
		t.Errorf("added: %d %q %q", sub, a, b)
	}
	r.done(t)
	r = open(t, buildGuildInvitation(500000003, "Ann", "Zion"), protocol.OpcodeGuildInviteStuff)
	if sub, _, id, name, guild := r.u8(), r.signed(), r.u32(), r.str8(24), r.wide(128); sub != 1 || id != 500000003 || name != "Ann" || guild != "Zion" {
		t.Errorf("invitation: %d %d %q %q", sub, id, name, guild)
	}
	r.done(t)
	r = open(t, buildGuildFailed(guildCmdCreate, guildFailName), protocol.OpcodeGuildCommandFailed)
	if cmd, why := r.u8(), r.u8(); cmd != 1 || why != 2 {
		t.Errorf("failed: %d %d", cmd, why)
	}
	r.done(t)
}

// guildCmd builds a 0x0578 request.
func guildCmd(sub byte, body ...[]byte) *protocol.Message {
	return groupMsg(opGuildCommand, append([][]byte{{sub}}, body...)...)
}

func wideb(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := binary.LittleEndian.AppendUint32(nil, uint32(len(u)))
	for _, c := range u {
		b = binary.LittleEndian.AppendUint16(b, c)
	}
	return b
}

func zzb(v int32) []byte {
	u := uint32(v) << 1
	if v < 0 {
		u = ^u
	}
	var b []byte
	for {
		c := byte(u & 0x7F)
		u >>= 7
		if u == 0 {
			return append(b, c)
		}
		b = append(b, c|0x80)
	}
}

// subsOf returns the 0x0468 sub-types in msgs, in order.
func subsOf(msgs [][]byte) []byte {
	var subs []byte
	for _, m := range msgs {
		if len(m) > 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(m)) == protocol.OpcodeGuildInfo {
			subs = append(subs, m[2])
		}
	}
	return subs
}

func hasSub(msgs [][]byte, sub byte) bool {
	for _, s := range subsOf(msgs) {
		if s == sub {
			return true
		}
	}
	return false
}

func TestGuildLifecycle(t *testing.T) {
	f := newGroupFixture(t, "Ann", "Bob", "Cam")
	gh := NewGuildHandler(f.sm)
	gh.SetGameWorld(f.world)
	run := func(who string, m *protocol.Message) { gh.HandleGuildCommand(f.sess[who], m) }

	run("Ann", guildCmd(guildCmdCreate, wideb("Zion")))
	if subs := subsOf(f.take("Ann")); len(subs) < 3 || subs[0] != 1 || subs[1] != 3 || subs[len(subs)-1] != entity.GuildWordJoined {
		t.Fatalf("create sent subs %v", subs)
	}
	if g := f.world.GuildOf(f.p["Ann"]); g == nil || g.Name != "Zion" {
		t.Fatal("no guild")
	}
	// A second guild of the same name is refused (566).
	run("Bob", guildCmd(guildCmdCreate, wideb("zion")))
	r := open(t, lastOf(f.take("Bob"), protocol.OpcodeGuildCommandFailed), protocol.OpcodeGuildCommandFailed)
	if cmd, why := r.u8(), r.u8(); cmd != 1 || why != guildFailName {
		t.Errorf("duplicate name: %d %d", cmd, why)
	}

	// Ann invites Bob; Bob joins.
	run("Ann", guildCmd(guildCmdInvite, str8b("Bob"), wideb("Zion")))
	inv := lastOf(f.take("Bob"), protocol.OpcodeGuildInviteStuff)
	if inv == nil {
		t.Fatal("Bob got no invitation")
	}
	run("Bob", guildCmd(guildCmdJoin, u32b(invitedBy(t, inv))))
	bob := f.take("Bob")
	if subs := subsOf(bob); subs[0] != 1 || !hasSub(bob, 3) || subs[len(subs)-1] != entity.GuildWordJoined {
		t.Errorf("joiner got %v", subs)
	}
	ann := f.take("Ann")
	if !hasSub(ann, 3) || !hasSub(ann, 15) || lastOf(ann, protocol.OpcodeGuildInviteStuff) == nil {
		t.Errorf("inviter got %v", opsOf(ann))
	}

	// Bob is a member (rank 0): demoting him further fails with 745.
	bobID := int32(entity.GuildMemberID(f.p["Bob"]))
	run("Ann", guildCmd(guildCmdDemote, zzb(bobID)))
	r = open(t, lastOf(f.take("Ann"), protocol.OpcodeGuildCommandFailed), protocol.OpcodeGuildCommandFailed)
	if cmd, why := r.u8(), r.u8(); cmd != guildCmdDemote || why != guildFailLowestRank {
		t.Errorf("demote a member: %d %d", cmd, why)
	}
	// Promote him: everyone sees his record, and he hears his rank changed.
	run("Ann", guildCmd(guildCmdPromote, zzb(bobID)))
	if f.p["Bob"].GuildRank != 1 || !hasSub(f.take("Bob"), entity.GuildWordRank) {
		t.Errorf("promotion: rank %d", f.p["Bob"].GuildRank)
	}
	f.take("Ann")
	// Bob, an officer, may invite; he invites Cam, who refuses (busy).
	run("Bob", guildCmd(guildCmdInvite, str8b("Cam"), wideb("Zion")))
	f.take("Cam")
	run("Cam", guildCmd(guildCmdRefuse, u32b(f.seen("Cam", "Bob")), []byte{2}))
	r = open(t, lastOf(f.take("Bob"), protocol.OpcodeGuildInviteStuff), protocol.OpcodeGuildInviteStuff)
	if sub, code, name := r.u8(), r.u8(), r.str8(24); sub != 3 || code != guildInviteBusy || name != "Cam" {
		t.Errorf("refusal: %d %d %q", sub, code, name)
	}

	// The charter: the leader's to set, and everyone is told.
	run("Ann", guildCmd(guildCmdCharter, wideb("Be kind")))
	if b := f.take("Bob"); !hasSub(b, 1) || !hasSub(b, entity.GuildWordCharter) {
		t.Errorf("charter: Bob got %v", subsOf(b))
	}
	f.take("Ann")
	run("Bob", guildCmd(guildCmdCharter, wideb("mine now")))
	r = open(t, lastOf(f.take("Bob"), protocol.OpcodeGuildCommandFailed), protocol.OpcodeGuildCommandFailed)
	if cmd, why := r.u8(), r.u8(); cmd != guildCmdCharter || why != guildFailNotAllowed {
		t.Errorf("an officer's charter: %d %d", cmd, why)
	}

	// The leader leaves: Bob leads.
	run("Ann", guildCmd(guildCmdLeave))
	if subs := subsOf(f.take("Ann")); len(subs) != 1 || subs[0] != 2 {
		t.Errorf("leaver got %v", subs)
	}
	if f.p["Bob"].GuildRank != entity.GuildRankLeader {
		t.Errorf("Bob's rank %d after the leader left", f.p["Bob"].GuildRank)
	}
	b := f.take("Bob")
	if !hasSub(b, 4) || !hasSub(b, entity.GuildWordRank) {
		t.Errorf("Bob got %v", subsOf(b))
	}

	// Bob disbands it.
	run("Bob", guildCmd(guildCmdDisband))
	if subs := subsOf(f.take("Bob")); len(subs) != 2 || subs[0] != entity.GuildWordDisbanded || subs[1] != 2 {
		t.Errorf("disband: %v", subs)
	}
	if f.world.GuildOf(f.p["Bob"]) != nil || f.p["Bob"].GuildID != 0 {
		t.Error("the guild outlived its disbanding")
	}
}

// invitedBy is the inviter id a 0x057A/1 carries: what the invitee sends back.
func invitedBy(t *testing.T, inv []byte) uint32 {
	r := open(t, inv, protocol.OpcodeGuildInviteStuff)
	r.u8()
	r.signed()
	return r.u32()
}

func TestGuildCommandsOutsideAGuild(t *testing.T) {
	f := newGroupFixture(t, "Ann")
	gh := NewGuildHandler(f.sm)
	gh.SetGameWorld(f.world)
	for _, cmd := range []byte{guildCmdDisband, guildCmdLeave, guildCmdCharter} {
		body := [][]byte{}
		if cmd == guildCmdCharter {
			body = append(body, wideb("x"))
		}
		gh.HandleGuildCommand(f.sess["Ann"], guildCmd(cmd, body...))
		r := open(t, lastOf(f.take("Ann"), protocol.OpcodeGuildCommandFailed), protocol.OpcodeGuildCommandFailed)
		if c, why := r.u8(), r.u8(); c != cmd || why != guildFailNotInGuild {
			t.Errorf("command %d: %d %d", cmd, c, why)
		}
	}
}

// A member who comes back (a new Player at login) is in its guild again and
// is told so by the tick, after its MemoryDump.
func TestGuildAnnouncedAtLogin(t *testing.T) {
	f := newGroupFixture(t, "Ann", "Bob")
	gh := NewGuildHandler(f.sm)
	gh.SetGameWorld(f.world)
	gh.HandleGuildCommand(f.sess["Ann"], guildCmd(guildCmdCreate, wideb("Zion")))
	f.take("Ann")
	f.take("Bob")

	// Ann logs in again: CreatePlayer makes her Player anew from the record.
	old := f.p["Ann"]
	fresh := f.world.CreatePlayer(f.sess["Ann"], &database.CharacterRecord{ServerID: uint32(old.ServerID), CharName: old.Name})
	if fresh == nil || fresh == old || f.world.GuildOf(fresh) == nil {
		t.Fatal("the login did not put the new Player back in its guild")
	}
	for i := 0; i < 10; i++ {
		f.world.Tick(time.Now())
	}
	subs := subsOf(f.take("Ann"))
	if len(subs) < 2 || subs[0] != 1 || subs[1] != 3 {
		t.Errorf("at login Ann got %v, want 1 then 3", subs)
	}
	if fresh.GuildRank != entity.GuildRankLeader {
		t.Errorf("rank %d", fresh.GuildRank)
	}
}

// The join names the inviter by the id the invitation carried, as the
// invitee's session knows it: any other id joins nothing (the control). And
// guild chat reaches every member, the speaker's name in the line.
func TestGuildJoinByTheInviteesIDAndChat(t *testing.T) {
	f := newGroupFixture(t, "Ann", "Bob")
	gh := NewGuildHandler(f.sm)
	gh.SetGameWorld(f.world)
	run := func(who string, m *protocol.Message) { gh.HandleGuildCommand(f.sess[who], m) }
	run("Ann", guildCmd(guildCmdCreate, wideb("Zion")))
	run("Ann", guildCmd(guildCmdInvite, str8b("Bob"), wideb("Zion")))
	id := invitedBy(t, lastOf(f.take("Bob"), protocol.OpcodeGuildInviteStuff))
	if id != f.seen("Bob", "Ann") {
		t.Fatalf("invitation id %#x, Bob knows Ann as %#x", id, f.seen("Bob", "Ann"))
	}
	run("Bob", guildCmd(guildCmdJoin, u32b(id+1)))
	if f.world.GuildOf(f.p["Bob"]) != nil {
		t.Fatal("a join naming the wrong inviter joined")
	}
	run("Ann", guildCmd(guildCmdInvite, str8b("Bob"), wideb("Zion")))
	run("Bob", guildCmd(guildCmdJoin, u32b(id)))
	if f.world.GuildOf(f.p["Bob"]) == nil {
		t.Fatal("the join did not join")
	}
	f.take("Ann")
	f.take("Bob")
	run("Ann", guildCmd(guildCmdChat, wideb("hello")))
	r := open(t, lastOf(f.take("Bob"), protocol.OpcodeGuildInfo), protocol.OpcodeGuildInfo)
	if sub, _, text := r.u8(), r.u32(), r.wide(512); sub != 5 || text != "Ann: hello" {
		t.Errorf("chat: %d %q", sub, text)
	}
	if lastOf(f.take("Ann"), protocol.OpcodeGuildInfo) == nil {
		t.Error("the speaker did not hear its own line")
	}
}
