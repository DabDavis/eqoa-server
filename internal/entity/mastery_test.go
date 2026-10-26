package entity

import (
	"encoding/binary"
	"math"
	"net"
	"testing"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The stat cap before masteries is what SOE's live server sent: 400 at
// level 60 (Corsten's VIPlayer m_p44), 350 at 45 (Lostologist's). A cap
// rises only by the masteries the character BOUGHT (cmbuy.go): its STR MAX
// to CHA MAX, so the five general ranks of a stat and Godborn make 550 at 60
// (jib's cap "including the normal 5 ranks", F3 20261025-163704) and a
// master class's +50 600. The controls: the server's old flat 475 is neither
// base, a character who bought nothing has the base alone (until F3 pass 17
// every character counted the general ranks as held: 550 at 60), and a cap
// raise of one stat leaves the others.
func TestStatCapsAreSOEsAndTheMasteriesBought(t *testing.T) {
	for _, c := range []struct{ level, base int }{{1, 350}, {10, 350}, {45, 350}, {46, 400}, {60, 400}} {
		if got := StatCapBase(c.level); got != c.base {
			t.Errorf("level %d: base cap %d, want %d", c.level, got, c.base)
		}
		p := &Player{Level: c.level}
		for st := StatStr; st <= StatCha; st++ {
			if got := p.StatCapOf(st); got != c.base {
				t.Errorf("level %d, nothing bought: stat %d's cap %d, want %d", c.level, st, got, c.base)
			}
		}
	}
	if old := 475; old == StatCapBase(60) || old == StatCapBase(45) {
		t.Fatal("the control: the old flat cap must differ from SOE's")
	}
	p := &Player{Level: 60}
	p.CMEffects[cmSlotCap] = 15 + 20 + 25 + 30 + 35 + 25 // Brawn 1-5, Godborn
	if got := p.StatCapOf(StatStr); got != 550 {
		t.Errorf("STR with Brawn 1-5 and Godborn: cap %d, want jib's 550", got)
	}
	if got := p.StatCapOf(StatSta); got != 400 {
		t.Errorf("STA beside them: cap %d, want 400 (the control: STR's ranks raise STR's alone)", got)
	}
	if p.CMEffects[cmSlotCap] += 50; p.StatCapOf(StatStr) != 600 {
		t.Errorf("with a master class's +50: %d, want jib's 600", p.StatCapOf(StatStr))
	}
}

// What a stat is past its cap counts for nothing: the stat channel's total,
// the maximum hit points from stamina and the maximum power. Jibrish, a
// level 60 enchanter, had INT 980 and STA 686 on the channel. The control:
// the same stats uncapped give more.
func TestStatsPastTheCapCountForNothing(t *testing.T) {
	p := &Player{Level: 60, Class: 12, BaseStr: 50, BaseSta: 80, BaseAgi: 70, BaseDex: 70, BaseWis: 55, BaseInt: 110, BaseCha: 60}
	p.Gear = Gear{Sta: 606, Agi: 725, Int: 870, Str: 355}
	for i := 0; i < 7; i++ {
		p.CMEffects[cmSlotCap+i] = 150 // every stat's five general ranks and Godborn bought: 550
	}
	p.RecalcStats(true)
	stat := func(off int) int {
		return int(int32(binary.LittleEndian.Uint32(SerializeStatUpdate(p)[off:])))
	}
	for _, c := range []struct {
		name string
		st   Stat
		off  int
		want int
	}{
		{"STR", StatStr, 0, 405},
		{"STA", StatSta, 4, 550},
		{"AGI", StatAgi, 8, 550},
		{"INT", StatInt, 20, 550},
	} {
		if got := p.StatNow(c.st); got != c.want {
			t.Errorf("%s now %d, want %d", c.name, got, c.want)
		}
		if got := stat(c.off); got != c.want {
			t.Errorf("%s on the stat channel %d, want %d", c.name, got, c.want)
		}
	}
	if got, want := p.MaxHPNow(), CalculateMaxHP(60, 12, 550); got != want {
		t.Errorf("max HP %d, want %d (stamina 550)", got, want)
	}
	if got, want := p.MaxPowerNow(), CalculateMaxPower(60, 12, 405, 550, 550, 70, 55, 550, 60); got != want {
		t.Errorf("max power %d, want %d (INT 550)", got, want)
	}
	// a spell's INT past the cap adds nothing, and its STR under it all
	p.Mods.Int, p.Mods.Str = 30, 30
	if got := p.StatNow(StatInt); got != 550 {
		t.Errorf("INT with a spell's 30: %d, want 550", got)
	}
	if got := p.StatNow(StatStr); got != 435 {
		t.Errorf("STR with a spell's 30: %d, want 435", got)
	}
	if got, want := p.MaxPowerNow(), CalculateMaxPower(60, 12, 435, 550, 550, 70, 55, 550, 60); got != want {
		t.Errorf("max power with the spells %d, want %d", got, want)
	}
	if CalculateMaxHP(60, 12, 80+606) == p.MaxHPNow() || CalculateMaxPower(60, 12, 405, 686, 795, 70, 55, 980, 60) == CalculateMaxPower(60, 12, 405, 550, 550, 70, 55, 550, 60) {
		t.Fatal("the control: uncapped stats must give other maxima")
	}
}

// A point costs what the live game's table says (eqoa.fandom.com's "CM
// Point XP Required"), the nth of the character's points: sixteen of its 151
// listed values, all of which round(125000 * 1.0061^(n-1)) gives. The
// control: a curve a hair flatter misses the table.
func TestAMasteryPointCostsTheLiveTables(t *testing.T) {
	table := map[int]int{
		1: 125000, 10: 132032, 100: 228236, 200: 419275, 300: 770218, 400: 1414909,
		500: 2599222, 600: 4774834, 700: 8771486, 770: 13426215, 800: 16113432,
		900: 29600767, 1000: 54377328, 1200: 183504899, 1400: 619266325, 1500: 1137607284,
	}
	missed := 0
	for n, want := range table {
		if got := CMPointXP(n); got != want {
			t.Errorf("point %d: %d, want the table's %d", n, got, want)
		}
		if int(math.Round(125000*math.Pow(1.006, float64(n-1)))) != want {
			missed++
		}
	}
	if missed < len(table)-1 {
		t.Fatalf("the control: 1.006 matched %d of the table's points", len(table)-missed)
	}
}

// Below the level cap with no debt the share divides each award (0-100 by
// one, the mastery points window's step), each point made when the
// masteries' part reaches its cost (CMPointXP of the point it would be);
// 1500 points held, spent and unspent, all of it goes to the level. The
// controls: 0% gives the masteries nothing, and a point is not made early.
func TestTheShareDividesExperience(t *testing.T) {
	for _, c := range []struct {
		m              Mastery
		xp, lvl, cm, n int
	}{
		{Mastery{Percent: 100}, 6000, 0, 6000, 0},
		{Mastery{Percent: 0}, 6000, 6000, 0, 0},
		{Mastery{Percent: 25}, 1000, 750, 250, 0},
		{Mastery{Percent: 33}, 1000, 670, 330, 0},
		{Mastery{Percent: 100, XP: CMPointXP(1) - 1000}, 1200, 0, 1200, 1},
		{Mastery{Percent: 100, XP: CMPointXP(1) - 1}, CMPointXP(2) + 1, 0, CMPointXP(2) + 1, 2},
		{Mastery{Percent: 100, XP: CMPointXP(1) - 1}, CMPointXP(2), 0, CMPointXP(2), 1},
		{Mastery{Percent: 100, XP: 0}, CMPointXP(1) - 1, 0, CMPointXP(1) - 1, 0},
		{Mastery{Percent: 100, Points: 700, Spent: 69, XP: CMPointXP(770) - 5}, 10, 0, 10, 1},
		{Mastery{Percent: 100, Points: 1000, Spent: 500}, 6000, 6000, 0, 0},
		{Mastery{Percent: 100, Points: 1499, XP: CMPointXP(1500) - 100}, 30000, 0, 30000, 1},
	} {
		sh := splitXP(c.m, 30, 0, c.xp)
		lvl, cm := sh.Level, sh.Mastery
		if lvl != c.lvl || cm != c.cm || sh.Debt != 0 {
			t.Errorf("%+v of %d: level %d, masteries %d, debt %d; want %d, %d, 0", c.m, c.xp, lvl, cm, sh.Debt, c.lvl, c.cm)
		}
		m := c.m
		if n := gainMastery(&m, cm); n != c.n {
			t.Errorf("%+v of %d: %d points made, want %d", c.m, c.xp, n, c.n)
		}
		if m.Points+m.Spent > MasteryCap {
			t.Errorf("%+v: %d points held, past %d", c.m, m.Points+m.Spent, MasteryCap)
		}
	}
}

// jib's rules (F3 20261026-042328): at the level cap every bit goes to the
// masteries whatever the window's share; while a debt is owed half of each
// award pays it (never more than is owed), at the cap the other half going to
// the masteries and below it divided by the share; masteries held at 1500 take
// nothing, their part paying what is left of the debt, then the level's. The
// controls: below the cap the share still divides an award (a level 59 at 0%
// gets no mastery experience), and with no debt nothing goes to one.
func TestTheCapAndTheDebtDivideExperience(t *testing.T) {
	full := Mastery{Points: 1000, Spent: 500}
	for _, c := range []struct {
		name            string
		m               Mastery
		level, debt, xp int
		lvl, cm, paid   int
	}{
		{"at the cap the share does not count", Mastery{Percent: 0}, 60, 0, 6000, 0, 6000, 0},
		{"at the cap, 40%", Mastery{Percent: 40}, 60, 0, 6000, 0, 6000, 0},
		{"at the cap with a debt, half each", Mastery{Percent: 0}, 60, 100000, 6000, 0, 3000, 3000},
		{"at the cap, a small debt paid off", Mastery{Percent: 0}, 60, 1000, 6000, 0, 5000, 1000},
		{"an odd award: the debt the smaller half", Mastery{}, 60, 100000, 6001, 0, 3001, 3000},
		{"below the cap, the share as before", Mastery{Percent: 25}, 59, 0, 1000, 750, 250, 0},
		{"below the cap at 0%: nothing to the masteries", Mastery{Percent: 0}, 59, 0, 1000, 1000, 0, 0},
		{"below the cap with a debt: half paid, the rest by the share", Mastery{Percent: 25}, 30, 100000, 1000, 375, 125, 500},
		{"masteries full at the cap: their part pays the debt", full, 60, 100000, 6000, 0, 0, 6000},
		{"masteries full, a small debt: the rest to the level", full, 60, 1000, 6000, 5000, 0, 1000},
		{"masteries full, no debt: all to the level", full, 60, 0, 6000, 6000, 0, 0},
		{"a debt below nothing owes nothing", Mastery{}, 60, -50, 6000, 0, 6000, 0},
		{"nothing awarded", Mastery{Percent: 50}, 60, 100, 0, 0, 0, 0},
	} {
		sh := splitXP(c.m, c.level, c.debt, c.xp)
		if sh.Level != c.lvl || sh.Mastery != c.cm || sh.Debt != c.paid {
			t.Errorf("%s: %+v; want level %d, masteries %d, debt %d", c.name, sh, c.lvl, c.cm, c.paid)
		}
		if sh.Level+sh.Mastery+sh.Debt != max(c.xp, 0) {
			t.Errorf("%s: %+v does not add up to %d", c.name, sh, c.xp)
		}
		if sh.Debt > max(c.debt, 0) {
			t.Errorf("%s: paid %d of a debt of %d", c.name, sh.Debt, c.debt)
		}
	}
	// the controls
	if sh := splitXP(Mastery{Percent: 0}, 59, 0, 1000); sh.Mastery != 0 {
		t.Fatalf("the control: a level 59 at 0%% got %d mastery experience", sh.Mastery)
	}
	if sh := splitXP(Mastery{Percent: 0}, 60, 0, 1000); sh.Debt != 0 || sh.Mastery != 1000 {
		t.Fatalf("the control: no debt, %+v", sh)
	}
}

// #grantcm as jib asked: spent and unspent never past 1500 together, the
// grant unspent. The control: a grant with no cap would give 1500.
func TestGrantCMFollowsJibsExamples(t *testing.T) {
	for _, c := range []struct {
		m           Mastery
		amount      int
		give, after int
	}{
		{Mastery{Points: 100}, 1500, 1400, 1500},
		{Mastery{Points: 50, Spent: 50}, 1500, 1400, 1450},
		{Mastery{Points: 50, Spent: 50}, 100, 100, 150},
		{Mastery{Points: 0, Spent: 1500}, 10, 0, 0},
		{Mastery{}, 2000, 1500, 1500},
	} {
		m := c.m
		if give := grantMastery(&m, c.amount); give != c.give || m.Points != c.after {
			t.Errorf("%+v + %d: gave %d, %d unspent; want %d, %d", c.m, c.amount, give, m.Points, c.give, c.after)
		}
	}
	m := Mastery{Points: 50, Spent: 50}
	if m.Points += 1500; m.Points == 1450 {
		t.Fatal("the control: an uncapped grant must differ")
	}
}

// A kill's experience with a share set: CLIENT is told the points as its
// Recv03 reads them (0x1402, 3, unspent, spent: it announces "You have
// earned N mastery points."), the level the rest (0x0020), and a level
// that moves the stat cap the new cap (0x001C, 400 at 46). The control: no
// share, no 0x1402.
func TestAKillMakesMasteryPointsAndTellsTheCap(t *testing.T) {
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 7), Port: 7101}, 1, 1, 1, 2, true)
	p := &Player{ServerID: 1001, Name: "Mastery", Level: 45, XPInLevel: CharXPDict[45] - 100}
	w.Players[p.ServerID] = p
	s.SetECSEntity(p)
	s.SetCharacterInWorld(true)

	p.Mastery = Mastery{Percent: 50, XP: CMPointXP(1) - 1000}
	w.awardXP(s, p, 4000)
	if p.Mastery.Points != 1 || p.Mastery.XP != 1000 {
		t.Fatalf("mastery %+v, want a point and 1000 toward the next", p.Mastery)
	}
	if p.Level != 46 {
		t.Fatalf("level %d, want 46 (2000 of the 4000 to the level)", p.Level)
	}
	var points, statCap []byte
	for _, m := range s.Queue().PeekPending() {
		d := m.Data()
		switch protocol.GameOpcode(binary.LittleEndian.Uint16(d)) {
		case protocol.OpcodeClassMasteryServer:
			points = d[2:]
		case protocol.OpcodePlayerP44:
			statCap = d[2:]
		}
	}
	if len(points) != 9 || points[0] != 3 || binary.LittleEndian.Uint32(points[1:]) != 1 || binary.LittleEndian.Uint32(points[5:]) != 0 {
		t.Errorf("0x1402: % x, want 03, unspent 1, spent 0", points)
	}
	if len(statCap) != 4 || binary.LittleEndian.Uint32(statCap) != 400 {
		t.Errorf("0x001C: % x, want 400 (level 46)", statCap)
	}

	s.Queue().GetPending()
	p.Mastery.Percent = 0
	w.awardXP(s, p, 4000)
	for _, m := range s.Queue().PeekPending() {
		if protocol.GameOpcode(binary.LittleEndian.Uint16(m.Data())) == protocol.OpcodeClassMasteryServer {
			t.Fatal("the control: with no share, no mastery points")
		}
	}
}
