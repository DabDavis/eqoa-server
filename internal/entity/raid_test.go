package entity

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A group of more than four is a raid group (raid.go; the user, 2026-11-07: "lets
// remove group member cap, but if over 4 it is a raid group, with 80% xp cut"). Each
// rule beside its control, a group of four: a kill's experience, whichever way the
// kill came, and what is not a kill's; what each client is told, read back as
// CLIENT's Msg0772 and Channel40 read it; the lines; the group's spells; a keep
// restart, a replaced login, a takeover and a logout.

// raidRig is n players at one place, each with a session, grouped by the real
// invite and accept.
type raidRig struct {
	t    *testing.T
	w    *World
	sm   *session.Manager
	sess []*session.Session
	p    []*Player
}

var raidNames = []string{"Ann", "Bob", "Cal", "Dee", "Eve", "Fay", "Gus", "Hal", "Ivy", "Jon", "Kip", "Lou"}

func raidID(i int) uint32 { return uint32(2001 + i) }

func newRaidRig(t *testing.T, n, level int) *raidRig {
	t.Helper()
	r := &raidRig{t: t, w: NewWorld(), sm: session.NewManager()}
	r.w.SessionManager = r.sm
	for i := 0; i < n; i++ {
		s := r.open(i)
		p := r.w.CreatePlayer(s, r.record(i, level))
		p.HP, p.MaxHP = 500, 500
		p.TotalXP = 0 // what it earns from here (earned)
		r.sess, r.p = append(r.sess, s), append(r.p, p)
	}
	return r
}

func (r *raidRig) open(i int) *session.Session {
	s := r.sm.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, byte(10+i)), Port: 6400 + i}, uint32(100+i), uint32(100+i), uint16(100+i), 2, true)
	s.SetInGame(true)
	s.SetCharacterInWorld(true)
	s.SetPlayerObjectID(500000000 + uint32(i))
	return s
}

func (r *raidRig) record(i, level int) *database.CharacterRecord {
	return &database.CharacterRecord{ServerID: raidID(i), CharName: raidNames[i], AccountID: 7 + i, Level: level, X: 1000, Y: 10, Z: 1000}
}

// join is member i asked by member by, and accepting.
func (r *raidRig) join(by, i int) {
	r.t.Helper()
	r.w.GroupInvite(r.sess[by], r.w.IDSeenBy(r.sess[by], r.p[i]))
	r.w.GroupAccept(r.sess[i], r.w.IDSeenBy(r.sess[i], r.p[by]))
	if r.p[i].GroupID == 0 || r.p[i].GroupID != r.p[by].GroupID {
		r.t.Fatalf("%s is in group %d after %s's invite (%d)", r.p[i].Name, r.p[i].GroupID, r.p[by].Name, r.p[by].GroupID)
	}
}

// form puts the first n players in one group, player 0 leading.
func (r *raidRig) form(n int) {
	r.t.Helper()
	for i := 1; i < n; i++ {
		r.join(0, i)
	}
}

func (r *raidRig) drainAll() {
	for _, s := range r.sess {
		drain(s)
	}
}

func (r *raidRig) sync() {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.TickCount += 5 - r.w.TickCount%5
	r.w.groupSyncSystem()
}

// heard is the lines session i was queued, drained.
func (r *raidRig) heard(i int) []string { return linesOf(drain(r.sess[i])) }

// readRoster reads a 0x0772 as CLIENT's Msg0772 does, m_group being four long: a
// count past it is the overflow, and fails the test.
func readRoster(t *testing.T, m []byte) (ids []uint32, names []string) {
	t.Helper()
	b := m[2:]
	if len(b) < 8 {
		t.Fatalf("a short roster % x", m)
	}
	count := int(binary.LittleEndian.Uint32(b[4:]))
	if count > 4 {
		t.Fatalf("a roster of %d: the id of the fifth lands on CLIENT's m_p2BE4C", count)
	}
	b = b[8:]
	for i := 0; i < count; i++ {
		ids = append(ids, binary.LittleEndian.Uint32(b))
		b = b[4:]
	}
	for i := 0; i < count; i++ {
		n := int(binary.LittleEndian.Uint32(b))
		if n > 23 {
			t.Fatalf("a name of %d bytes: CLIENT's holds 24", n)
		}
		names = append(names, string(b[4:4+n]))
		b = b[4+n:]
	}
	if len(b) != 0 {
		t.Fatalf("%d bytes after the roster's names", len(b))
	}
	return ids, names
}

// rostersIn is every roster in msgs, read as CLIENT reads them.
func rostersIn(t *testing.T, msgs [][]byte) (out [][]string) {
	t.Helper()
	for _, m := range msgs {
		if opOfMsg(m) == protocol.OpcodeCreateGroup {
			_, names := readRoster(t, m)
			out = append(out, names)
		}
	}
	return out
}

// lastRoster is the last roster session i was queued, drained, read as CLIENT
// reads it: nil for none.
func (r *raidRig) lastRoster(i int) (ids []uint32, names []string) {
	r.t.Helper()
	for _, m := range drain(r.sess[i]) {
		if opOfMsg(m) == protocol.OpcodeCreateGroup {
			ids, names = readRoster(r.t, m)
		}
	}
	return ids, names
}

// channelRecords is session i's 0x40 channel as CLIENT's Channel40 reads it: each
// record's id, a zero id (the channel's padding) left out.
func (r *raidRig) channelRecords(i int) []uint32 {
	r.t.Helper()
	data := r.sess[i].Connection().GetServerGroupUpdate().GetData()
	if len(data) != 39 {
		r.t.Fatalf("the channel holds %d bytes, want the 39 of three members", len(data))
	}
	var out []uint32
	for k := 0; k+13 <= len(data); k += 13 {
		if id := binary.LittleEndian.Uint32(data[k:]); id != 0 {
			out = append(out, id)
		}
	}
	return out
}

func (r *raidRig) killable(level int) *NPC {
	npc := &NPC{ServerID: 9000, Name: "a gnoll", Level: level, HP: 1, MaxHP: 1000, X: 1000, Y: 10, Z: 1000, Size: 1}
	r.w.NPCs[npc.ServerID] = npc
	return npc
}

func (r *raidRig) kill(by int, npc *NPC) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.awardKillXPLocked(r.sess[by], r.p[by], npc)
}

// Every member's experience from a kill is a fifth of what it would be, from the
// fifth member up; up to four it is as ever. The figures are the model's own shares.
func TestARaidGroupsKillIsWorthAFifth(t *testing.T) {
	for _, tc := range []struct {
		n   int
		cut bool
	}{{1, false}, {2, false}, {3, false}, {4, false}, {5, true}, {6, true}, {8, true}, {12, true}} {
		t.Run(fmt.Sprintf("%d members", tc.n), func(t *testing.T) {
			r := newRaidRig(t, tc.n, 30)
			r.form(tc.n)
			levels := make([]int, tc.n)
			for i := range levels {
				levels[i] = 30
			}
			whole := groupExp(30, 30, levels)
			want := whole
			if tc.cut {
				want = whole * 20 / 100
			}
			if whole <= 0 || (tc.cut && want == whole) {
				t.Fatalf("no test: whole %d, want %d", whole, want)
			}
			r.kill(0, r.killable(30))
			for i, p := range r.p {
				if got := earned(p, 0); got != want {
					t.Errorf("%s earned %d, want %d (of %d)", p.Name, got, want, whole)
				}
				if tc.n > 1 && i > 0 && p.GroupID != r.p[0].GroupID {
					t.Errorf("%s is not in the group", p.Name)
				}
			}
		})
	}
}

// The group's size decides, not who is near: five in a group and two near the kill
// share as two and are cut, and those away get nothing. The control: four in a group,
// two near, is the same share uncut.
func TestARaidGroupsCutGoesByItsSizeNotWhoIsNear(t *testing.T) {
	for _, tc := range []struct {
		n   int
		cut bool
	}{{4, false}, {5, true}} {
		r := newRaidRig(t, tc.n, 30)
		r.form(tc.n)
		for _, p := range r.p[2:] {
			p.X += KillShareRange + 50
		}
		r.kill(0, r.killable(30))
		want := groupExp(30, 30, []int{30, 30})
		if tc.cut {
			want = want * 20 / 100
		}
		if got := earned(r.p[1], 0); got != want {
			t.Errorf("%d in the group, two near: the near member earned %d, want %d", tc.n, got, want)
		}
		if got := earned(r.p[2], 0); got != 0 {
			t.Errorf("%d in the group: one away earned %d", tc.n, got)
		}
	}
}

// Back at four the cut is over, however the fifth went, and a new fifth starts it
// again.
func TestFallingBackToFourEndsTheCut(t *testing.T) {
	share := func(n int) int {
		levels := make([]int, n)
		for i := range levels {
			levels[i] = 30
		}
		return groupExp(30, 30, levels)
	}
	gain := func(r *raidRig, p *Player) int {
		before := earned(p, 0)
		r.kill(0, r.killable(30))
		return earned(p, 0) - before
	}
	for _, tc := range []struct {
		name string
		goes func(r *raidRig)
	}{
		{"leaves", func(r *raidRig) { r.w.GroupLeave(r.sess[4]) }},
		{"booted", func(r *raidRig) { r.w.GroupBoot(r.sess[0], r.w.IDSeenBy(r.sess[0], r.p[4])) }},
		{"logs out", func(r *raidRig) { r.w.RemovePlayerBySession(r.sess[4]) }},
		{"the leader leaves", func(r *raidRig) { r.w.GroupLeave(r.sess[3]) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRaidRig(t, 5, 30)
			r.form(5)
			if got, want := gain(r, r.p[1]), share(5)*20/100; got != want {
				t.Fatalf("five: earned %d, want %d", got, want)
			}
			tc.goes(r)
			if got, want := gain(r, r.p[1]), share(4); got != want {
				t.Errorf("four: earned %d, want %d", got, want)
			}
		})
	}
	r := newRaidRig(t, 6, 30)
	r.form(5)
	r.w.GroupLeave(r.sess[4])
	r.join(0, 5)
	if got, want := gain(r, r.p[1]), share(5)*20/100; got != want {
		t.Errorf("a new fifth: earned %d, want %d", got, want)
	}
}

// Every way a creature dies by a player gives a raid group's cut: a melee hit, a
// spell, a pet's hit. The control: the same kill in a group of four.
func TestEveryKillIsCut(t *testing.T) {
	paths := []struct {
		name string
		kill func(r *raidRig, npc *NPC)
	}{
		{"melee", func(r *raidRig, npc *NPC) {
			r.w.mu.Lock()
			defer r.w.mu.Unlock()
			if !r.w.playerHitLocked(r.sess[0], r.p[0], npc, 900001) {
				r.t.Fatal("the swing did not kill")
			}
		}},
		{"spell", func(r *raidRig, npc *NPC) {
			r.w.mu.Lock()
			defer r.w.mu.Unlock()
			if !r.w.spellHitLocked(r.p[0], r.sess[0], npc, 500) {
				r.t.Fatal("the spell did not kill")
			}
		}},
		{"pet", func(r *raidRig, npc *NPC) {
			pet := &NPC{ServerID: 9500, Name: "a golem", Level: 30, HP: 100, MaxHP: 100, Owner: r.p[0].ServerID, X: 1000, Y: 10, Z: 1000}
			r.w.NPCs[pet.ServerID] = pet
			ps := &petState{owner: r.p[0], npc: pet}
			r.w.mu.Lock()
			defer r.w.mu.Unlock()
			if !r.w.petLandHitLocked(ps, npc, 500, time.Now(), false) {
				r.t.Fatal("the pet did not kill")
			}
		}},
	}
	for _, path := range paths {
		for _, tc := range []struct {
			n   int
			cut bool
		}{{4, false}, {5, true}, {6, true}} {
			t.Run(fmt.Sprintf("%s, %d members", path.name, tc.n), func(t *testing.T) {
				r := newRaidRig(t, tc.n, 30)
				r.form(tc.n)
				path.kill(r, r.killable(30))
				levels := make([]int, tc.n)
				for i := range levels {
					levels[i] = 30
				}
				want := groupExp(30, 30, levels)
				if tc.cut {
					want = want * 20 / 100
				}
				for _, p := range r.p {
					if got := earned(p, 0); got != want {
						t.Errorf("%s earned %d, want %d", p.Name, got, want)
					}
				}
			})
		}
	}
}

// A quest's experience is no kill's: it is not cut, in a raid group or out of one.
func TestAQuestsExperienceIsNotCut(t *testing.T) {
	r := newRaidRig(t, 6, 30)
	r.form(6)
	r.w.AwardXP(r.sess[1], r.p[1], 150000)
	if got := earned(r.p[1], 0); got != 150000 {
		t.Errorf("a raid group's member's quest gave %d, want 150000", got)
	}
}

// The cut is on the whole award, before a boon's, a debt's and the masteries' shares:
// each is its share of the fifth. Controls: the same award in a group of four.
func TestTheCutComesBeforeTheAwardsShares(t *testing.T) {
	const level60kill = 60
	for _, tc := range []struct {
		name  string
		level int
		setup func(r *raidRig, p *Player)
		check func(t *testing.T, p *Player, award int)
	}{
		{"a boon doubles what is left", 30, func(r *raidRig, p *Player) {
			r.w.boon.Percent = 200
			p.Effects = append(p.Effects, &ActiveEffect{SpellID: BoonEffectID, Until: time.Now().Add(time.Hour)})
		}, func(t *testing.T, p *Player, award int) {
			if got := earned(p, 0); got != award*2 {
				t.Errorf("earned %d, want %d (twice %d)", got, award*2, award)
			}
		}},
		{"half of what is left pays a debt", 30, func(r *raidRig, p *Player) {
			p.Debt = 1 << 30
		}, func(t *testing.T, p *Player, award int) {
			if paid := 1<<30 - p.Debt; paid != award-award/2 && paid != award/2 {
				t.Errorf("paid %d of the debt, want half of %d", paid, award)
			}
			if got := earned(p, 1<<30); got != award {
				t.Errorf("earned %d in all, want %d", got, award)
			}
		}},
		{"a level 60's goes to its masteries", 60, func(r *raidRig, p *Player) {}, func(t *testing.T, p *Player, award int) {
			if p.Mastery.XP+CMPointXP(p.Mastery.Points) == 0 || p.TotalXP != 0 {
				t.Errorf("masteries %+v, levels %d", p.Mastery, p.TotalXP)
			}
			if got := earned(p, 0); got != award {
				t.Errorf("earned %d, want %d", got, award)
			}
		}},
	} {
		for _, group := range []struct {
			n   int
			cut bool
		}{{4, false}, {5, true}} {
			t.Run(fmt.Sprintf("%s, %d members", tc.name, group.n), func(t *testing.T) {
				r := newRaidRig(t, group.n, tc.level)
				r.form(group.n)
				tc.setup(r, r.p[1])
				mob := 30
				if tc.level == 60 {
					mob = level60kill
				}
				levels := make([]int, group.n)
				for i := range levels {
					levels[i] = tc.level
				}
				award := groupExp(mob, tc.level, levels)
				if group.cut {
					award = award * 20 / 100
				}
				r.kill(0, r.killable(mob))
				tc.check(t, r.p[1], award)
			})
		}
	}
}

// Every member near a raid group's kill is credited for its quests, as in any group.
// The control: one away is not.
func TestAQuestKillCreditsARaidGroup(t *testing.T) {
	r := newRaidRig(t, 6, 30)
	r.form(6)
	r.p[5].X += KillShareRange + 50
	var got []string
	r.w.onKill = append(r.w.onKill, func(c KillCredit) {
		for _, k := range c.Players {
			got = append(got, k.Name)
		}
	})
	npc := r.killable(30)
	r.w.mu.Lock()
	r.w.spellHitLocked(r.p[0], r.sess[0], npc, 500)
	r.w.mu.Unlock()
	if strings.Join(got, " ") != "Ann Bob Cal Dee Eve" {
		t.Errorf("credited %v", got)
	}
}

// With any number in a group, each client's roster is four at most, read as CLIENT
// reads it: the leader first, the client's own character among them under its own
// id. Up to four everyone is told of everyone in the order they joined, as ever; past
// that the leader, the client's own and the earliest of the others, in that order of
// joining. The channel names the same others.
func TestEachClientIsToldOfFourAtMost(t *testing.T) {
	for _, tc := range []struct {
		n    int
		want map[int]string // by viewer, whose own place is past the first four's
	}{
		{1, nil},
		{2, nil},
		{3, nil},
		{4, nil},
		{5, map[int]string{4: "Ann Bob Cal Eve"}},
		{6, map[int]string{4: "Ann Bob Cal Eve", 5: "Ann Bob Cal Fay"}},
		{12, map[int]string{4: "Ann Bob Cal Eve", 7: "Ann Bob Cal Hal", 11: "Ann Bob Cal Lou"}},
	} {
		t.Run(fmt.Sprintf("%d members", tc.n), func(t *testing.T) {
			r := newRaidRig(t, tc.n, 30)
			r.form(tc.n)
			if tc.n == 1 {
				return
			}
			for i := range r.p {
				ids, names := r.lastRoster(i)
				if len(ids) > GroupWindowSize || len(names) != len(ids) {
					t.Fatalf("%s's roster %v %v", r.p[i].Name, ids, names)
				}
				want := strings.Join(raidNames[:min(tc.n, 4)], " ")
				if i >= 4 {
					want = raidNames[0] + " " + raidNames[1] + " " + raidNames[2] + " " + raidNames[i]
				}
				if w, ok := tc.want[i]; ok && w != want {
					t.Fatalf("the test: %q against %q", w, want)
				}
				if got := strings.Join(names, " "); got != want {
					t.Errorf("%s is told of %q, want %q", r.p[i].Name, got, want)
				}
				own := 0
				for _, id := range ids {
					if id == r.sess[i].PlayerObjectID() {
						own++
					}
				}
				if own != 1 {
					t.Errorf("%s's own id comes %d times in %v", r.p[i].Name, own, ids)
				}
				if i == 0 && ids[0] != r.sess[0].PlayerObjectID() {
					t.Errorf("the leader is not member 0: %v", ids)
				}
			}
			r.sync()
			for i := range r.p {
				var others []uint32
				ids := []uint32{}
				for _, o := range func() []*Player {
					r.w.mu.RLock()
					defer r.w.mu.RUnlock()
					return r.w.shownMembersLocked(r.w.groups[r.p[i].GroupID], r.p[i])
				}() {
					id := r.w.IDSeenBy(r.sess[i], o)
					ids = append(ids, id)
					if o != r.p[i] {
						others = append(others, id)
					}
				}
				got := r.channelRecords(i)
				if fmt.Sprint(got) != fmt.Sprint(others) {
					t.Errorf("%s's channel names %v, want %v of its roster %v", r.p[i].Name, got, others, ids)
				}
				for _, id := range got {
					if id == r.sess[i].PlayerObjectID() {
						t.Errorf("%s's channel names itself", r.p[i].Name)
					}
				}
			}
		})
	}
}

// A newcomer past the fourth changes no one's window, and a window moves only when
// one of its own leaves: the next to have joined takes the place, the rest in order.
func TestARaidGroupsWindowsHoldStill(t *testing.T) {
	r := newRaidRig(t, 8, 30)
	r.form(6)
	before := make([][]string, 6)
	for i := range before {
		_, before[i] = r.lastRoster(i)
	}
	r.join(0, 6)
	r.join(1, 7) // any member's invite, as vanilla sends one
	for i := range before {
		if _, names := r.lastRoster(i); fmt.Sprint(names) != fmt.Sprint(before[i]) {
			t.Errorf("%s's window went from %v to %v with newcomers", r.p[i].Name, before[i], names)
		}
	}
	r.drainAll()

	r.w.GroupLeave(r.sess[2]) // Cal, in nearly every window
	want := map[int]string{
		0: "Ann Bob Dee Eve", 1: "Ann Bob Dee Eve", 3: "Ann Bob Dee Eve", 4: "Ann Bob Dee Eve",
		5: "Ann Bob Dee Fay", 6: "Ann Bob Dee Gus", 7: "Ann Bob Dee Hal",
	}
	for i, w := range want {
		if _, names := r.lastRoster(i); strings.Join(names, " ") != w {
			t.Errorf("after Cal left %s sees %v, want %s", r.p[i].Name, names, w)
		}
	}

	r.w.GroupLeave(r.sess[0]) // the leader: the next to have joined leads, member 0 in every window
	want = map[int]string{1: "Bob Dee Eve Fay", 3: "Bob Dee Eve Fay", 4: "Bob Dee Eve Fay", 5: "Bob Dee Eve Fay",
		6: "Bob Dee Eve Gus", 7: "Bob Dee Eve Hal"}
	for i, w := range want {
		ids, names := r.lastRoster(i)
		if strings.Join(names, " ") != w {
			t.Errorf("after the leader left %s sees %v, want %s", r.p[i].Name, names, w)
		}
		if i == 1 && ids[0] != r.sess[1].PlayerObjectID() {
			t.Errorf("the new leader is not its own member 0: %v", ids)
		}
	}
}

// A roster is sent again when an id its client knows changes, four at most still, and
// names only those its window holds: seeing a member past the window changes nothing.
func TestARaidGroupsRostersFollowWhatTheClientSees(t *testing.T) {
	r := newRaidRig(t, 6, 30)
	r.form(6)
	r.drainAll()
	see := func(viewer, other int, id uint32) {
		vs := GetOrCreateVisibility(r.sess[viewer])
		vs.ChannelEntity[0], vs.ChannelObjectID[0] = r.p[other].ServerID, id
	}
	see(1, 2, 100009) // Bob sees Cal, in his window
	r.sync()
	ids, names := r.lastRoster(1)
	if len(ids) != 4 || ids[2] != 100009 || names[2] != "Cal" {
		t.Errorf("Bob's roster after seeing Cal: %v %v", ids, names)
	}
	if got := r.channelRecords(1); len(got) != 3 || got[1] != 100009 {
		t.Errorf("Bob's channel after seeing Cal: %v", got)
	}
	see(0, 5, 100010) // Ann sees Fay, past her window
	r.sync()
	if ids, names := r.lastRoster(0); ids != nil {
		t.Errorf("Ann was sent a roster for seeing one past her window: %v %v", ids, names)
	}
	for _, id := range r.channelRecords(0) {
		if id == 100010 {
			t.Errorf("Ann's channel names Fay, past her window")
		}
	}
}

// Nothing refuses a joiner: never 0x069A 4 ("the group is full"), by the leader's
// invite or a member's, however many there are.
func TestNoGroupIsFull(t *testing.T) {
	r := newRaidRig(t, 12, 30)
	r.form(4)
	r.drainAll()
	for i := 4; i < 12; i++ {
		r.join(i%3, i)
		if got := len(r.w.GroupMembers(r.p[0])); got != i+1 {
			t.Fatalf("after %s: %d members", r.p[i].Name, got)
		}
		for _, m := range drain(r.sess[i]) {
			if opOfMsg(m) == protocol.OpcodeRemoveGroupMember {
				t.Errorf("%s was refused: % x", r.p[i].Name, m)
			}
		}
	}
	if got := r.p[11].GroupID; got != r.p[0].GroupID {
		t.Errorf("the twelfth is in group %d, the leader's is %d", got, r.p[0].GroupID)
	}
}

// Everyone is told when it becomes a raid group and when it stops being one; a
// newcomer past that is told what it joined and the rest who came; one who goes is
// named. Up to four nothing is said: the control.
func TestTheGroupIsToldWhenItRaids(t *testing.T) {
	r := newRaidRig(t, 7, 30)
	r.form(4)
	for i := 0; i < 4; i++ {
		if got := r.heard(i); len(got) != 0 {
			t.Errorf("%s was told %q forming a group of four", r.p[i].Name, got)
		}
	}
	r.join(0, 4) // a raid group now
	for i := 0; i < 5; i++ {
		if got := r.heard(i); !hasLine(got, raidNowLine) || (i < 4 && !hasLine(got, "Eve joined the raid group (5 members).")) {
			t.Errorf("%s heard %q at the fifth", r.p[i].Name, got)
		}
	}
	r.join(0, 5)
	if got := r.heard(5); !hasLine(got, raidJoinedLine) || hasLine(got, raidNowLine) {
		t.Errorf("the sixth heard %q", got)
	}
	for i := 0; i < 5; i++ {
		if got := r.heard(i); len(got) != 1 || got[0] != "Fay joined the raid group (6 members)." {
			t.Errorf("%s heard %q at the sixth", r.p[i].Name, got)
		}
	}
	r.w.GroupLeave(r.sess[5])
	for i := 0; i < 5; i++ {
		if got := r.heard(i); len(got) != 1 || got[0] != "Fay left the raid group (5 members)." {
			t.Errorf("%s heard %q at five again", r.p[i].Name, got)
		}
	}
	if got := r.heard(5); len(got) != 0 {
		t.Errorf("the one who left heard %q", got)
	}
	r.w.GroupBoot(r.sess[0], r.w.IDSeenBy(r.sess[0], r.p[4]))
	for i := 0; i < 4; i++ {
		got := r.heard(i)
		if len(got) != 2 || got[0] != "Eve left the raid group (4 members)." || got[1] != raidEndLine {
			t.Errorf("%s heard %q at four", r.p[i].Name, got)
		}
	}
	r.sync()
	r.w.GroupLeave(r.sess[3])
	for i := 0; i < 3; i++ {
		if got := r.heard(i); len(got) != 0 {
			t.Errorf("%s was told %q in a group of three", r.p[i].Name, got)
		}
	}
}

// The lines are in what a PS2 types and draws, nothing that takes a |.
func TestTheRaidLinesAreAPS2s(t *testing.T) {
	for _, l := range []string{raidNowLine, raidJoinedLine, raidEndLine, raidStillLine} {
		if strings.ContainsAny(l, "|#$&<>[]\\^`{}\t\n") {
			t.Errorf("%q", l)
		}
		for _, c := range l {
			if c > 126 {
				t.Errorf("%q has %q", l, c)
			}
		}
	}
	if !strings.Contains(raidNowLine, "80% less XP") || !strings.Contains(raidJoinedLine, "80% less XP") {
		t.Errorf("%q, %q", raidNowLine, raidJoinedLine)
	}
}

// A kept group of six comes back through a restart: every client's roster and its
// channel as they were, no one told again, the cut on, and back at four it ends and
// is said. A group brought back at four when it was kept at six is told it ended. The
// control: a kept group of four is told nothing.
func TestAKeptRaidGroupFormsAgain(t *testing.T) {
	through := func(old *raidRig, back int) *raidRig {
		b, err := json.Marshal(old.w.KeepGroups())
		if err != nil {
			t.Fatal(err)
		}
		var kept []KeptGroup
		if err := json.Unmarshal(b, &kept); err != nil {
			t.Fatal(err)
		}
		r := newRaidRig(t, back, 30)
		for i := range r.p {
			r.p[i].CharServerID = raidID(i)
		}
		if n, err := r.w.RestoreGroups(kept); err != nil || n != 1 {
			t.Fatalf("%d formed, %v", n, err)
		}
		return r
	}
	old := newRaidRig(t, 6, 30)
	for i := range old.p {
		old.p[i].CharServerID = raidID(i)
	}
	old.form(6)
	if len(old.w.KeepGroups()[0].Members) != 6 {
		t.Fatalf("kept %+v", old.w.KeepGroups())
	}
	before := make([][]string, 6)
	for i := range before {
		_, before[i] = old.lastRoster(i)
	}
	r := through(old, 6)
	if got := len(r.w.GroupMembers(r.p[0])); got != 6 {
		t.Fatalf("back with %d", got)
	}
	r.sync()
	for i := range r.p {
		msgs := drain(r.sess[i])
		rosters := rostersIn(t, msgs)
		if len(rosters) != 1 || fmt.Sprint(rosters[0]) != fmt.Sprint(before[i]) {
			t.Errorf("%s's rosters %v, kept %v", r.p[i].Name, rosters, before[i])
		}
		if lines := linesOf(msgs); len(lines) != 0 {
			t.Errorf("%s was told %q", r.p[i].Name, lines)
		}
		if got := r.channelRecords(i); len(got) != 3 {
			t.Errorf("%s's channel names %v", r.p[i].Name, got)
		}
	}
	r.kill(0, r.killable(30))
	if got, want := earned(r.p[1], 0), groupExp(30, 30, []int{30, 30, 30, 30, 30, 30})*20/100; got != want {
		t.Errorf("after the restart a kill gave %d, want %d", got, want)
	}
	r.w.GroupLeave(r.sess[5])
	r.w.GroupLeave(r.sess[4])
	if got := r.heard(0); !hasLine(got, raidEndLine) {
		t.Errorf("back at four after the restart: %q", got)
	}

	short := through(old, 4) // two characters did not come back
	short.sync()
	for i := range short.p {
		if got := short.heard(i); !hasLine(got, raidEndLine) {
			t.Errorf("%s, of a raid group back at four, heard %q", short.p[i].Name, got)
		}
	}

	four := newRaidRig(t, 4, 30)
	for i := range four.p {
		four.p[i].CharServerID = raidID(i)
	}
	four.form(4)
	r = through(four, 4)
	r.sync()
	for i := range r.p {
		if got := r.heard(i); len(got) != 0 {
			t.Errorf("%s, of a group of four, heard %q", r.p[i].Name, got)
		}
	}
}

// A login that replaces a session keeps its place in the raid group: its window
// within four, its client reminded once when in the world (it joined nothing, so it
// shows no group), the rest not; a login in a group of four hears nothing. A takeover
// the same. A logout leaves.
func TestAReplacedLoginStaysInTheRaidGroup(t *testing.T) {
	for _, tc := range []struct {
		name string
		n    int
		hear bool
	}{{"a raid group", 6, true}, {"a group of four", 4, false}} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRaidRig(t, tc.n, 30)
			r.form(tc.n)
			r.sync()
			r.drainAll()
			last := tc.n - 1
			ns := r.open(last + 20)
			ns.SetCharacterInWorld(false)
			np := r.w.CreatePlayer(ns, r.record(last, 30))
			np.HP, np.MaxHP = 500, 500
			r.sess[last], r.p[last] = ns, np
			if got := len(r.w.GroupMembers(r.p[0])); got != tc.n || np.GroupID != r.p[0].GroupID {
				t.Fatalf("%d in the group, its id %d", got, np.GroupID)
			}
			r.sync()
			msgs := drain(ns)
			if lines := linesOf(msgs); len(lines) != 0 {
				t.Errorf("told %q before its world", lines)
			}
			rosters := rostersIn(t, msgs)
			if len(rosters) != 1 || len(rosters[0]) > 4 || rosters[0][0] != "Ann" {
				t.Errorf("its roster: %v", rosters)
			}
			ns.SetCharacterInWorld(true)
			r.sync()
			r.sync()
			lines := linesOf(drain(ns))
			if tc.hear && (len(lines) != 1 || lines[0] != raidStillLine) {
				t.Errorf("in the world it heard %q", lines)
			}
			if !tc.hear && len(lines) != 0 {
				t.Errorf("in the world it heard %q", lines)
			}
			for i := 0; i < last; i++ {
				if got := r.heard(i); len(got) != 0 {
					t.Errorf("%s heard %q", r.p[i].Name, got)
				}
			}
		})
	}

	r := newRaidRig(t, 5, 30)
	r.form(5)
	r.sync()
	r.drainAll()
	ns := r.open(30)
	r.p[4].CharServerID = 777
	r.sess[4].SetCharacterServerID(777)
	got, _ := r.w.TakeOverPlayer(ns, 777)
	if got != r.p[4] || got.GroupID != r.p[0].GroupID {
		t.Fatalf("taken over: %v, group %d", got, got.GroupID)
	}
	r.sync()
	msgs := drain(ns)
	if rosters := rostersIn(t, msgs); len(rosters) != 1 || len(rosters[0]) != 4 {
		t.Errorf("a takeover's roster: %v", rosters)
	}
	if lines := linesOf(msgs); len(lines) != 1 || lines[0] != raidStillLine {
		t.Errorf("a takeover heard %q", lines)
	}

	r = newRaidRig(t, 5, 30)
	r.form(5)
	r.w.RemovePlayerBySession(r.sess[2])
	if got := len(r.w.GroupMembers(r.p[0])); got != 4 {
		t.Errorf("after a logout %d are in the group", got)
	}
	if !hasLine(r.heard(0), raidEndLine) {
		t.Error("a logout ended the raid group and no one was told")
	}
}

// A group spell by its record's scope reaches all six in its range; one away and a
// stranger it does not. The control is the group of four's own test
// (TestAGroupHealTakesItsGroupAndTheirPets).
func TestAGroupSpellReachesARaidGroup(t *testing.T) {
	r := newAreaRig(t)
	r.p.Class, r.p.HP = 9, 1000
	members := []EntityID{r.p.ServerID}
	var near []*Player
	for i := 0; i < 5; i++ {
		q := r.player(EntityID(1002+i), raidNames[i], 1005+float32(i), 1000)
		near = append(near, q)
		members = append(members, q.ServerID)
	}
	far := r.player(1010, "Far", 1100, 1000)
	stranger := r.player(1011, "Stranger", 1002, 1000)
	members = append(members, far.ServerID)
	r.w.groups = map[uint32]*Group{7: {ID: 7, Members: members}}
	r.p.GroupID, far.GroupID = 7, 7
	for _, q := range near {
		q.GroupID = 7
	}
	plain := spellOf(9001, 2, "Group Mending", "Group, Increase Hitpoints by 250 ; Increase 25% based on CHA")
	r.cast(t, plain, r.p.ServerID)
	heal := 250 + 25
	for _, q := range near {
		if q.HP != 500+heal {
			t.Errorf("%s at %d, want %d", q.Name, q.HP, 500+heal)
		}
	}
	if far.HP != 500 || stranger.HP != 500 || r.p.HP != 1000+heal {
		t.Errorf("one away %d, a stranger %d, the caster %d", far.HP, stranger.HP, r.p.HP)
	}
}

// A group's area reaches every member in it and every pet, past the eight it held
// to: ten members and pets land, the 0x00B1 still names the seven nearest, all CLIENT
// holds. A GM's row's cap still holds. A group of four and its pets land as ever.
func TestAGroupsAreaReachesARaidGroup(t *testing.T) {
	layout := func(t *testing.T, others int) (*areaRig, []*Player, []*NPC) {
		r := newAreaRig(t)
		r.p.Class, r.p.HP = 9, 1000
		members := []EntityID{r.p.ServerID}
		var ps []*Player
		var pets []*NPC
		for i := 0; i < others; i++ {
			q := r.player(EntityID(1002+i), raidNames[i], 1002+float32(i), 1000)
			q.GroupID = 7
			members = append(members, q.ServerID)
			ps = append(ps, q)
			pets = append(pets, r.petOf(q, EntityID(2002+i), 1002+float32(i), 1001))
		}
		r.p.GroupID = 7
		r.w.groups = map[uint32]*Group{7: {ID: 7, Members: members}}
		return r, ps, pets
	}
	heal := 250 + 25
	named := func(b1 []byte) int {
		if len(b1) < 11 {
			return -1
		}
		return int(b1[10])
	}
	for _, tc := range []struct {
		others, names int
	}{{3, 6}, {5, 7}, {11, 7}} { // a group of four, of six, of twelve; each with its pets
		t.Run(fmt.Sprintf("%d others", tc.others), func(t *testing.T) {
			r, ps, pets := layout(t, tc.others)
			b1 := r.cast(t, tideOfLife(), r.p.ServerID)
			if got := named(b1); got != tc.names {
				t.Errorf("the 0x00B1 names %d, want %d: % X", got, tc.names, b1)
			}
			for _, q := range ps {
				if q.HP != 500+heal {
					t.Errorf("%s at %d, want %d", q.Name, q.HP, 500+heal)
				}
			}
			for _, pet := range pets {
				if pet.HP != 100+heal {
					t.Errorf("%s at %d, want %d", pet.Name, pet.HP, 100+heal)
				}
			}
		})
	}
	r, ps, pets := layout(t, 5)
	capped := tideOfLife()
	capped.Area.Max = 3
	r.cast(t, capped, r.p.ServerID)
	took := 0
	for i := range ps {
		if ps[i].HP > 500 {
			took++
		}
		if pets[i].HP > 100 {
			took++
		}
	}
	if took != 2 {
		t.Errorf("a GM's cap of 3 (the caster and two): %d others took it", took)
	}
}

// A group's rez reaches all the raid group's dead it reaches.
func TestAGroupsRezReachesARaidGroup(t *testing.T) {
	r := newRezRig(t)
	members := []EntityID{r.a.ServerID, r.b.ServerID, r.c.ServerID}
	var dead []*Player
	for i := 0; i < 9; i++ {
		q := &Player{ServerID: EntityID(1100 + i), CharServerID: uint32(1100 + i), Name: raidNames[i], Class: 0, Level: 49, HP: 1, MaxHP: 900,
			X: 100, Y: 10, Z: 100 + float32(i), Debt: 1000, Rez: RezChance{Debt: 1000, Until: r.at(time.Hour)}}
		r.w.Players[q.ServerID] = q
		r.w.SpatialIndex.Insert(q.ServerID, q.X, q.Z)
		members = append(members, q.ServerID)
		dead = append(dead, q)
		q.GroupID = 9
	}
	r.w.groups = map[uint32]*Group{9: {ID: 9, Members: members}}
	r.a.GroupID, r.b.GroupID, r.c.GroupID = 9, 9, 9
	r.die(r.b, r.sb, 0)
	if !r.cast(r.sa, wbRenew(), rezA, 2*time.Second) {
		t.Fatal("not begun")
	}
	for _, q := range dead {
		if q.Debt >= 1000 {
			t.Errorf("%s owes %d: the rez did not reach it", q.Name, q.Debt)
		}
	}
}

// The search offers a group a joiner would not make a raid group of: three members,
// not four, not five (a client that holds four shows no window for it). The rest of
// the search is lfg_test's.
func TestTheGroupSearchOffersNoRaid(t *testing.T) {
	for _, tc := range []struct {
		n    int
		find bool
	}{{2, true}, {3, true}, {4, false}, {5, false}, {6, false}} {
		r := newRaidRig(t, tc.n+1, 30)
		r.form(tc.n)
		r.p[0].LFG = true
		rows := r.w.LFGSearch(r.sess[tc.n], LFGQuery{Type: 1, Classes: 0xFFFFFFFF, Races: 0xFFFFFFFF, MinLevel: 1, MaxLevel: 60})
		if found := len(rows) == 1; found != tc.find {
			t.Errorf("a group of %d: rows %+v, want found %v", tc.n, rows, tc.find)
		}
	}
}

// Power bars name only those the client's roster holds.
func TestPowerBarsNameARaidGroupsWindowAlone(t *testing.T) {
	r := newRaidRig(t, 6, 30)
	r.form(6)
	for _, p := range r.p {
		p.Power, p.MaxPower = 100, 100
	}
	r.w.mu.RLock()
	bars := r.w.powerBarsLocked(r.sess[0], r.p[0])
	r.w.mu.RUnlock()
	if len(bars) != 3 {
		t.Errorf("the leader's bars: %+v, want its window's three others", bars)
	}
}
