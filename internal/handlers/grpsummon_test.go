package handlers

import (
	"encoding/binary"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// grpSummonRig is a chat world of n players (account i+1 each, all alive, near
// one another in world 0), account 1 a GM and the rest not, each named.
func grpSummonRig(t *testing.T, n int) (*chatWorld, []*session.Session, []*entity.Player) {
	t.Helper()
	c := newChatWorld(t)
	lv, names := levels{1: 1}, map[int]string{}
	var ss []*session.Session
	var ps []*entity.Player
	for i := 0; i < n; i++ {
		name := "Member" + string(rune('A'+i))
		s, p := c.join(i+1, name, 25000+float32(i), 15000+float32(i), 0)
		s.SetAccountID(i + 1)
		p.HP, p.MaxHP = 100, 100
		names[i+1] = strings.ToLower(name)
		ss, ps = append(ss, s), append(ps, p)
	}
	c.h.SetAccessLevels(namedLevels{levels: lv, names: names})
	SetGrpSummon(nil, -1)
	t.Cleanup(func() { SetGrpSummon(nil, -1) })
	return c, ss, ps
}

// raidUp makes ps[0] lead a group of everyone in ps: an invite each up to four,
// then the leader's raid flag, every other one within its reach accepting it.
func raidUp(t *testing.T, c *chatWorld, ss []*session.Session, ps []*entity.Player) {
	t.Helper()
	for i := 1; i < len(ss) && i < entity.GroupWindowSize; i++ {
		c.gw.GroupInvite(ss[0], c.gw.IDSeenBy(ss[0], ps[i]))
		c.gw.GroupAccept(ss[i], c.gw.IDSeenBy(ss[i], ps[0]))
	}
	if len(ss) > entity.GroupWindowSize {
		c.gw.RaidCall(ss[0])
		for i := entity.GroupWindowSize; i < len(ss); i++ {
			c.gw.GroupAccept(ss[i], c.gw.IDSeenBy(ss[i], ps[0]))
		}
	}
	if got := len(c.gw.GroupMembers(ps[0])); got != len(ss) {
		t.Fatalf("the group has %d members, want %d", got, len(ss))
	}
	for _, s := range ss {
		s.Queue().GetPending()
	}
}

// The owner, 2026-11-21: "/grpsummon new GM command summons whole raid or group
// that they are in". A GM's group of three: both others come to it, into its world,
// each told who summoned it, the GM told who came; the control: a member of no
// group is left where it is.
func TestGrpSummonBringsAGMsGroup(t *testing.T) {
	c, ss, ps := grpSummonRig(t, 4)
	raidUp(t, c, ss[:3], ps[:3])
	ps[0].X, ps[0].Z, ps[0].World = 30000, 9000, 1
	c.say(ss[0], "/grpsummon")
	for i := 1; i <= 2; i++ {
		x, z, w, ok := teleported(ss[i])
		if !ok || w != 1 || ps[i].World != 1 {
			t.Fatalf("%s: teleported %v to world %d (its world %d)", ps[i].Name, ok, w, ps[i].World)
		}
		if dx, dz := x-30000, z-9000; dx*dx+dz*dz > 100 {
			t.Fatalf("%s landed at (%v, %v), far from the GM", ps[i].Name, x, z)
		}
		if got := texts(heard(t, ss[i])); !slices.Contains(got, "MemberA summons the group to them.") {
			t.Fatalf("%s heard %q", ps[i].Name, got)
		}
	}
	if got := texts(heard(t, ss[0])); !slices.Equal(got, []string{"[GM] Summoned your group: MemberB, MemberC."}) {
		t.Fatalf("the GM heard %q", got)
	}
	if _, _, _, ok := teleported(ss[3]); ok || ps[3].World != 0 {
		t.Fatal("a player of no group was summoned")
	}
	// #grpsummon and a PS2's "/t grpsummon" are the same
	for _, line := range []string{"#grpsummon", "/t grpsummon"} {
		ss[1].ReleaseStateChannels() // its world loaded
		ss[1].Queue().GetPending()
		ss[0].Queue().GetPending()
		c.say(ss[0], line)
		if _, _, _, ok := teleported(ss[1]); !ok {
			t.Fatalf("%q summoned no one", line)
		}
	}
}

// A raid of eight: all seven others come, told it is the raid.
func TestGrpSummonBringsARaid(t *testing.T) {
	c, ss, ps := grpSummonRig(t, 8)
	raidUp(t, c, ss, ps)
	if !c.gw.GroupIsRaid(ps[0]) {
		t.Fatal("no raid")
	}
	ps[0].X, ps[0].Z = 26000, 14000
	c.say(ss[0], "/grpsummon")
	for i := 1; i < 8; i++ {
		x, z, _, ok := teleported(ss[i])
		if !ok {
			t.Fatalf("%s did not come", ps[i].Name)
		}
		if dx, dz := x-26000, z-14000; dx*dx+dz*dz > 400 {
			t.Fatalf("%s landed at (%v, %v)", ps[i].Name, x, z)
		}
		if got := texts(heard(t, ss[i])); !slices.Contains(got, "MemberA summons the raid to them.") {
			t.Fatalf("%s heard %q", ps[i].Name, got)
		}
	}
	if got := texts(heard(t, ss[0])); len(got) != 1 || !strings.HasPrefix(got[0], "[GM] Summoned your raid: MemberB, ") || strings.Count(got[0], ",") != 6 {
		t.Fatalf("the GM heard %q", got)
	}
}

// A player of world.grpsummon_accounts (any case) may, told without [GM], and
// waits the cooldown after; one not listed is answered as an unknown command.
// With the cooldown 0 there is no wait; /help names it to a listed account alone.
func TestGrpSummonForAListedAccountWithItsWait(t *testing.T) {
	c, ss, ps := grpSummonRig(t, 4)
	raidUp(t, c, ss[1:], ps[1:]) // MemberB leads B, C, D; MemberA (the GM) is no member
	SetGrpSummon([]string{" MEMBERB "}, -1)

	// MemberC, not listed
	c.say(ss[2], "/grpsummon")
	if got := heard(t, ss[2]); len(got) != 1 || got[0].text != `Unrecognized command "/grpsummon".` {
		t.Fatalf("an unlisted player's /grpsummon: %+v", got)
	}
	ss[2].Queue().GetPending()
	c.say(ss[2], "#grpsummon")
	if got := heard(t, ss[2]); len(got) != 1 || got[0].text != `Unrecognized command "#grpsummon".` {
		t.Fatalf("an unlisted player's #grpsummon: %+v", got)
	}
	if _, _, _, ok := teleported(ss[1]); ok {
		t.Fatal("an unlisted player summoned")
	}

	ps[1].X, ps[1].Z = 27000, 13000
	t0 := time.Now()
	c.say(ss[1], "/grpsummon")
	if _, _, _, ok := teleported(ss[2]); !ok {
		t.Fatal("the listed player summoned no one")
	}
	if got := texts(heard(t, ss[1])); !slices.Equal(got, []string{"Summoned your group: MemberC, MemberD."}) {
		t.Fatalf("the listed player heard %q", got)
	}
	ss[1].Queue().GetPending()
	ss[2].Queue().GetPending()
	c.say(ss[1], "/grpsummon")
	got := texts(heard(t, ss[1]))
	if len(got) != 1 || !strings.HasPrefix(got[0], "Group summon is ready again in ") {
		t.Fatalf("again at once: %q", got)
	}
	if _, _, _, ok := teleported(ss[2]); ok {
		t.Fatal("the cooldown summoned")
	}
	// its time over
	ss[1].Queue().GetPending()
	ss[2].ReleaseStateChannels() // its world loaded
	c.h.grpSummonCommand(ss[1], ps[1], false, t0.Add(61*time.Second))
	if _, _, _, ok := teleported(ss[2]); !ok {
		t.Fatal("no summon after the cooldown")
	}
	// cooldown 0: none
	SetGrpSummon([]string{"memberb"}, 0)
	ss[2].Queue().GetPending()
	ss[2].ReleaseStateChannels()
	c.say(ss[1], "/grpsummon")
	ss[2].ReleaseStateChannels()
	c.say(ss[1], "/grpsummon")
	if n := teleportCount(ss[2]); n != 2 {
		t.Fatalf("cooldown 0: %d summons, want 2", n)
	}
	// unlisted again: unknown
	SetGrpSummon(nil, -1)
	ss[1].Queue().GetPending()
	c.say(ss[1], "/grpsummon")
	if got := heard(t, ss[1]); len(got) != 1 || got[0].text != `Unrecognized command "/grpsummon".` {
		t.Fatalf("after the list was emptied: %+v", got)
	}
	// /help names it to a listed account alone
	SetGrpSummon([]string{"memberb"}, -1)
	ss[1].Queue().GetPending()
	ss[2].Queue().GetPending()
	c.say(ss[1], "/help")
	c.say(ss[2], "/help")
	if !slices.Contains(texts(heard(t, ss[1])), grpSummonHelp) || slices.Contains(texts(heard(t, ss[2])), grpSummonHelp) {
		t.Fatal("/help's line is not the listed account's alone")
	}
}

// teleportCount is how many 0x07F6s are queued for s.
func teleportCount(s *session.Session) int {
	n := 0
	for _, m := range s.Queue().PeekPending() {
		if d := m.Data(); len(d) >= 15 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeTeleport {
			n++
		}
	}
	return n
}

// The members left behind are named, with why: not in the world, dead, in a duel;
// the one free to come comes.
func TestGrpSummonNamesWhoStays(t *testing.T) {
	c, ss, ps := grpSummonRig(t, 6)
	raidUp(t, c, ss[:4], ps[:4])
	// MemberB in a duel with MemberE (accepted: its countdown)
	now := time.Now()
	c.gw.DuelChallenge(ss[1], 0, "MemberE", 0, now)
	id, _ := c.gw.DuelOf(ps[4])
	c.gw.DuelAnswer(ss[4], id, true, now)
	if !c.gw.Dueling(ps[1]) {
		t.Fatal("no duel")
	}
	ps[2].HP = 0 // MemberC dead
	ss[3].SetCharacterInWorld(false)
	// MemberD gone from the world
	c.say(ss[0], "/grpsummon")
	got := texts(heard(t, ss[0]))
	want := []string{"[GM] No one of your group came.",
		"[GM] Not summoned: MemberB (in a duel), MemberC (dead), MemberD (not in the world)."}
	if !slices.Equal(got, want) {
		t.Fatalf("the GM heard %q", got)
	}
	for i := 1; i <= 3; i++ {
		if _, _, _, ok := teleported(ss[i]); ok {
			t.Fatalf("%s came", ps[i].Name)
		}
	}
	// the control: MemberC alive, MemberD back
	ps[2].HP = 100
	ss[3].SetCharacterInWorld(true)
	ss[0].Queue().GetPending()
	c.say(ss[0], "/grpsummon")
	if got := texts(heard(t, ss[0])); !slices.Equal(got, []string{"[GM] Summoned your group: MemberC, MemberD.", "[GM] Not summoned: MemberB (in a duel)."}) {
		t.Fatalf("the GM heard %q", got)
	}
	// at once again: MemberC and MemberD still loading their place
	ss[2].HoldStateChannels(time.Minute)
	ss[3].HoldStateChannels(time.Minute)
	ss[0].Queue().GetPending()
	c.say(ss[0], "/grpsummon")
	if got := texts(heard(t, ss[0])); !slices.Equal(got, []string{"[GM] No one of your group came.",
		"[GM] Not summoned: MemberB (in a duel), MemberC (loading), MemberD (loading)."}) {
		t.Fatalf("the GM heard %q", got)
	}
	// a caller in no group
	ss[5].Queue().GetPending()
	c.h.SetAccessLevels(namedLevels{levels: levels{1: 1, 6: 1}, names: map[int]string{1: "membera", 6: "memberf"}})
	c.say(ss[5], "/grpsummon")
	if got := texts(heard(t, ss[5])); !slices.Equal(got, []string{"[GM] " + grpSummonNoGroup}) {
		t.Fatalf("a GM of no group heard %q", got)
	}
}

// The owner, 2026-11-21: "sub gms can /grpsummon". A sub GM (a tester's row marked
// subgm) may, told without [GM], and waits the cooldown as a listed account does;
// the control: a plain player of the same group is answered as an unknown command.
func TestGrpSummonForASubGM(t *testing.T) {
	c, ss, ps := grpSummonRig(t, 4)
	raidUp(t, c, ss[1:], ps[1:]) // MemberB leads B, C, D
	c.h.SetAccessLevels(subs{levels: levels{1: 1}, sub: map[int]bool{2: true}})

	c.say(ss[2], "/grpsummon")
	if got := heard(t, ss[2]); len(got) != 1 || got[0].text != `Unrecognized command "/grpsummon".` {
		t.Fatalf("a plain player's /grpsummon: %+v", got)
	}
	if _, _, _, ok := teleported(ss[3]); ok {
		t.Fatal("a plain player summoned")
	}
	ss[2].Queue().GetPending()

	ps[1].X, ps[1].Z = 27000, 13000
	c.say(ss[1], "/t grpsummon") // a PS2's
	if _, _, _, ok := teleported(ss[2]); !ok {
		t.Fatal("the sub GM summoned no one")
	}
	if got := texts(heard(t, ss[1])); !slices.Equal(got, []string{"Summoned your group: MemberC, MemberD."}) {
		t.Fatalf("the sub GM heard %q", got)
	}
	ss[1].Queue().GetPending()
	ss[2].Queue().GetPending()
	ss[2].ReleaseStateChannels()
	c.say(ss[1], "#grpsummon")
	if got := texts(heard(t, ss[1])); len(got) != 1 || !strings.HasPrefix(got[0], "Group summon is ready again in ") {
		t.Fatalf("a sub GM again at once: %q", got)
	}
	if _, _, _, ok := teleported(ss[2]); ok {
		t.Fatal("the sub GM's cooldown summoned")
	}
}
