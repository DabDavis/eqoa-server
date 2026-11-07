package entity

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// databaseDuelEnd is a duel's end of two made-up characters.
var databaseDuelEnd = database.DuelEnd{A: database.RecordFighter{ServerID: 1, Name: "x"}, B: database.RecordFighter{ServerID: 2, Name: "y"}}

// recorded is what reached the combat record's hook, the world's runner run as a tick
// runs it after its lock.
type recorded struct {
	d   *duelists
	ops []RecordOp
}

func newRecorded(d *duelists) *recorded {
	r := &recorded{d: d}
	d.a.CharServerID, d.b.CharServerID, d.c.CharServerID = 71, 72, 73
	d.w.OnCombatRecord(func(op RecordOp) { r.ops = append(r.ops, op) })
	return r
}

func (r *recorded) take() []RecordOp {
	r.d.w.runCombatRecords()
	ops := r.ops
	r.ops = nil
	return ops
}

// pvpHit lands n of from's PvP damage on to at x, from's own (no halving asked here).
func (d *duelists) pvpHit(t *testing.T, from, to *Player, n int, x time.Duration) bool {
	t.Helper()
	d.w.mu.Lock()
	defer d.w.mu.Unlock()
	f := d.w.pvpFightLocked(from, to)
	if f == nil {
		t.Fatalf("%s and %s are no foes", from.Name, to.Name)
	}
	return d.w.duelHurtLocked(f, from, from.ServerID, to, n, d.at(x))
}

func fighterIDs(op RecordOp) (killer, victim uint32, assists []uint32) {
	for _, a := range op.Kill.Assists {
		assists = append(assists, a.ServerID)
	}
	return op.Kill.Killer.ServerID, op.Kill.Victim.ServerID, assists
}

// A PvP death counts once: the killer's kill, the victim's death, and an assist for each
// other player whose attack landed on the victim in the 30 s before (never the killer);
// one who hurt it earlier than that is no helper. The victim's list of who hurt it goes
// with its death. The controls: a second killing blow on the dead, and a death of no
// player's doing, count nothing.
func TestRecordPvPKillAndAssists(t *testing.T) {
	d := newPvP(t)
	r := newRecorded(d)
	d.flag(t, d.sa, 0)
	d.flag(t, d.sb, 0)
	d.flag(t, d.sc, 0)
	d.b.HP = 900
	d.pvpHit(t, d.c, d.b, 20, 0)
	d.pvpHit(t, d.a, d.b, 20, 20*time.Second)
	if over := d.pvpHit(t, d.a, d.b, 100_000, 25*time.Second); !over || !d.b.respawnPending() {
		t.Fatalf("no kill: HP %d", d.b.HP)
	}
	ops := r.take()
	if len(ops) != 1 || ops[0].Kill == nil || ops[0].Duel != nil {
		t.Fatalf("counted %+v", ops)
	}
	k, v, assists := fighterIDs(ops[0])
	if k != 71 || v != 72 || len(assists) != 1 || assists[0] != 73 {
		t.Fatalf("killer %d, victim %d, assists %v", k, v, assists)
	}
	if kill := ops[0].Kill; kill.Victim.Name != "Beta" || kill.Victim.Level != 32 || kill.Killer.Level != 30 || !kill.At.Equal(d.at(25*time.Second)) {
		t.Fatalf("the kill %+v", kill)
	}
	if len(d.b.pvp.hurt) != 0 {
		t.Fatalf("the dead keeps who hurt it: %+v", d.b.pvp.hurt)
	}
	d.w.mu.Lock()
	d.w.pvpDeathLocked(d.c, d.b, d.at(26*time.Second)) // a blow on the dead
	d.w.mu.Unlock()
	if ops := r.take(); len(ops) != 0 {
		t.Fatalf("a second death counted: %+v", ops)
	}

	// one who hurt it 31 s before is no helper; the killer's own hits never are
	d = newPvP(t)
	r = newRecorded(d)
	d.flag(t, d.sa, 0)
	d.flag(t, d.sb, 0)
	d.flag(t, d.sc, 0)
	d.b.HP = 900
	d.pvpHit(t, d.c, d.b, 20, 0)
	d.pvpHit(t, d.a, d.b, 20, 10*time.Second)
	d.pvpHit(t, d.a, d.b, 100_000, 31*time.Second)
	ops = r.take()
	if len(ops) != 1 {
		t.Fatalf("counted %+v", ops)
	}
	if _, _, assists := fighterIDs(ops[0]); len(assists) != 0 {
		t.Fatalf("assists %v: the third hit 31 s before, the killer its own", assists)
	}

	// a death of no player's doing (a creature's, a fall's)
	d = newPvP(t)
	r = newRecorded(d)
	d.flag(t, d.sa, 0)
	d.flag(t, d.sb, 0)
	d.pvpHit(t, d.a, d.b, 20, 0)
	d.w.mu.Lock()
	d.w.playerDownLocked(d.sb, d.b, d.at(time.Second), 0)
	d.w.mu.Unlock()
	if ops := r.take(); len(ops) != 0 {
		t.Fatalf("a death to no player counted: %+v", ops)
	}
}

// A duel's end counts once, with who lost, how and the stake: a defeat, a surrender (a
// loss too), a logout's forfeit and a draw. A duel called off in its countdown and two
// flagged players' fight count nothing as duels.
func TestRecordDuelEnds(t *testing.T) {
	duelOf := func(t *testing.T, ops []RecordOp) RecordOp {
		t.Helper()
		if len(ops) != 1 || ops[0].Duel == nil || ops[0].Kill != nil {
			t.Fatalf("counted %+v", ops)
		}
		return ops[0]
	}

	// a defeat, with a wager both staked
	d := newDuelists(t)
	r := newRecorded(d)
	d.a.Tunar, d.b.Tunar = 1000, 500
	d.w.DuelChallenge(d.sa, d.aSeesB, "", 200, d.at(0))
	duel := d.w.duels.of[d.a.ServerID]
	d.w.DuelBoxAnswer(d.sb, DuelBoxID(duel.ID), 0, d.at(time.Second))
	d.countDown(t, duel, time.Second)
	d.w.mu.Lock()
	d.w.duelHurtLocked(duel, d.b, d.b.ServerID, d.a, 1_000_000, d.at(2*time.Second))
	d.w.mu.Unlock()
	e := duelOf(t, r.take()).Duel
	if e.A.ServerID != 71 || e.B.ServerID != 72 || e.Loser != 71 || e.Surrendered || e.Wager != 200 {
		t.Fatalf("a defeat: %+v", e)
	}

	// a surrender
	d = newDuelists(t)
	r = newRecorded(d)
	d.start(t)
	d.w.DuelCancel(d.sb, d.at(3*time.Second))
	if e := duelOf(t, r.take()).Duel; e.Loser != 72 || !e.Surrendered || e.Wager != 0 {
		t.Fatalf("a surrender: %+v", e)
	}

	// a logout's forfeit
	d = newDuelists(t)
	r = newRecorded(d)
	d.start(t)
	d.w.RemovePlayer(d.a.ServerID)
	if e := duelOf(t, r.take()).Duel; e.Loser != 71 || e.Surrendered {
		t.Fatalf("a logout: %+v", e)
	}

	// a draw
	d = newDuelists(t)
	r = newRecorded(d)
	d.start(t)
	d.tick(time.Second + DuelMaxTime)
	if e := duelOf(t, r.take()).Duel; e.Loser != 0 || e.Surrendered || e.Wager != 0 {
		t.Fatalf("a draw: %+v", e)
	}

	// called off in its countdown: no fight, no count
	d = newDuelists(t)
	r = newRecorded(d)
	d.w.DuelChallenge(d.sa, d.aSeesB, "", 0, d.at(0))
	duel = d.w.duels.of[d.a.ServerID]
	d.w.DuelBoxAnswer(d.sb, DuelBoxID(duel.ID), 0, d.at(time.Second))
	d.w.DuelCancel(d.sa, d.at(2*time.Second))
	if d.w.duels.of[d.a.ServerID] != nil {
		t.Fatal("not called off")
	}
	if ops := r.take(); len(ops) != 0 {
		t.Fatalf("a duel called off counted: %+v", ops)
	}

	// two flagged players' fight ends in a death, no duel
	d = newPvP(t)
	r = newRecorded(d)
	d.flag(t, d.sa, 0)
	d.flag(t, d.sb, 0)
	d.pvpHit(t, d.a, d.b, 100_000, time.Second)
	if ops := r.take(); len(ops) != 1 || ops[0].Kill == nil || ops[0].Duel != nil {
		t.Fatalf("a PvP fight: %+v", ops)
	}
}

// The counts reach the hook in order, after the lock (a tick's runner, or SaveAll's
// flush); with no hook (a server with no database) nothing is kept waiting.
func TestRecordQueue(t *testing.T) {
	w := NewWorld()
	w.mu.Lock()
	w.queueRecordLocked(RecordOp{Duel: &databaseDuelEnd})
	w.mu.Unlock()
	if len(w.records.queue) != 0 {
		t.Fatal("kept with no hook")
	}
	var got []string
	w.OnCombatRecord(func(op RecordOp) { got = append(got, op.Fighters()[0].Name) })
	w.mu.Lock()
	for _, n := range []string{"one", "two", "three"} {
		e := databaseDuelEnd
		e.A.Name = n
		w.queueRecordLocked(RecordOp{Duel: &e})
	}
	w.mu.Unlock()
	w.FlushCombatRecords()
	if strings.Join(got, ",") != "one,two,three" {
		t.Fatalf("written %v", got)
	}
	w.FlushCombatRecords()
	if len(got) != 3 {
		t.Fatalf("written twice: %v", got)
	}
}

// A target window's lines: none for an empty record, PvP's kills and deaths and the
// duels' won and lost (and drawn) once there are any, each a line under the guild; the
// guild is the player's own, so a line that would not fit with it is left out. The
// lines reach 0x0010's guild (TargetInfoLines) once the summary is set.
func TestRecordSummaryInTheTargetWindow(t *testing.T) {
	if got := (RecordSummary{}).Lines(); len(got) != 0 {
		t.Fatalf("an empty record: %q", got)
	}
	s := RecordSummary{PvPKills: 12, PvPDeaths: 3, DuelWins: 5, DuelLosses: 2}
	if got := strings.Join(s.Lines(), "|"); got != "PvP 12-3|Duels 5-2" {
		t.Fatalf("lines %q", got)
	}
	if got := strings.Join((RecordSummary{DuelDraws: 1}).Lines(), "|"); got != "Duels 0-0-1" {
		t.Fatalf("a draw alone: %q", got)
	}
	if got := strings.Join((RecordSummary{PvPDeaths: 1}).Lines(), "|"); got != "PvP 0-1" {
		t.Fatalf("a death alone: %q", got)
	}
	if got := withRecord("Blades", s.Lines()); got != "Blades\nPvP 12-3\nDuels 5-2" {
		t.Fatalf("under a guild: %q", got)
	}
	if got := withRecord("", s.Lines()); got != "PvP 12-3\nDuels 5-2" {
		t.Fatalf("no guild: %q", got)
	}
	long := strings.Repeat("g", 50)
	if got := withRecord(long, s.Lines()); got != long+"\nPvP 12-3" {
		t.Fatalf("a long guild: %q", got)
	}
	if got := withRecord(strings.Repeat("g", 63), s.Lines()); got != strings.Repeat("g", 63) {
		t.Fatalf("a guild that fills the line: %q", got)
	}

	d := newDuelists(t)
	d.a.CharServerID = 71
	if _, guild := d.w.TargetInfoLines(d.b, d.a); guild != "Blades" {
		t.Fatalf("no record yet: %q", guild)
	}
	d.w.SetRecordSummary(71, s)
	if _, guild := d.w.TargetInfoLines(d.b, d.a); guild != "Blades\nPvP 12-3\nDuels 5-2" {
		t.Fatalf("with a record: %q", guild)
	}
	d.w.SetRecordSummary(99, RecordSummary{PvPKills: 1})
	if d.w.RecordSummaryOf(d.a) != s {
		t.Fatal("another character's summary landed on Alpha")
	}
}

// What a keep restart carries: whether the client has the window (its summary is read
// again from the database by the handlers).
func TestRecordWindowKept(t *testing.T) {
	d := newDuelists(t)
	if !d.w.SetRecordWindow(d.sa, true) || !d.a.RecordWindow {
		t.Fatal("not set")
	}
	now := time.Now()
	k, ok := d.w.KeepPlayer(d.sa, now)
	if !ok || !k.RecordWindow {
		t.Fatalf("kept %+v", k)
	}
	if k2, _ := d.w.KeepPlayer(d.sb, now); k2.RecordWindow {
		t.Fatal("kept for a client without it")
	}
	d.a.RecordWindow = false
	d.w.RestoreKeptPlayer(d.sa, k, now)
	if !d.a.RecordWindow {
		t.Fatal("not restored")
	}
	d.c.CharServerID, d.c.RecordWindow = 73, true
	d.a.CharServerID = 71
	got := d.w.RecordWindows([]uint32{71, 72, 73})
	if len(got) != 2 || got[71] != d.sa || got[73] != d.sc {
		t.Fatalf("the windows: %v", got)
	}
}
