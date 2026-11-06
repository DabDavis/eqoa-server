package entity

import (
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The resurrection spells (resurrect.go; jib's review, F3 pass 19): a rez
// gives back its printed share of the debt its target's latest death added,
// once per death, to a target dead or released in range.

// rezRig is a level 50 cleric (Gnolie), a level 49 warrior (Bran) six steps
// off and a second cleric (Liss), each in a session and each in the others'
// view; beside them Liss's pet and a crab, in Gnolie's view.
type rezRig struct {
	w          *World
	sa, sb, sc *session.Session
	a, b, c    *Player
	t0         time.Time
}

// the ids the players see each other, the pet and the crab by
const (
	rezA    = 0x00AA0001
	rezB    = 0x00BB0002
	rezC    = 0x00CC0003
	rezPet  = 0x00DD0004
	rezCrab = 0x00EE0005
)

func newRezRig(t *testing.T) *rezRig {
	t.Helper()
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	enter := func(n byte, p *Player, own uint32) *session.Session {
		s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, n), Port: 7000 + int(n)}, uint32(n), 1, 1, 2, true)
		w.Players[p.ServerID] = p
		w.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
		s.SetECSEntity(p)
		s.SetCharacterInWorld(true)
		s.SetPlayerObjectID(own)
		return s
	}
	a := &Player{ServerID: 1001, CharServerID: 71, Name: "Gnolie", Class: 9, Level: 50, HP: 900, MaxHP: 900, Power: 800, MaxPower: 800,
		X: 100, Y: 10, Z: 100}
	b := &Player{ServerID: 1002, CharServerID: 72, Name: "Bran", Class: 0, Level: 49, HP: 1200, MaxHP: 1200, Power: 300, MaxPower: 300,
		X: 106, Y: 10, Z: 100}
	c := &Player{ServerID: 1003, CharServerID: 73, Name: "Liss", Class: 9, Level: 50, HP: 900, MaxHP: 900, Power: 800, MaxPower: 800,
		X: 100, Y: 10, Z: 106}
	r := &rezRig{w: w, a: a, b: b, c: c, t0: time.Unix(1_800_000_000, 0)}
	r.sa, r.sb, r.sc = enter(21, a, rezA), enter(22, b, rezB), enter(23, c, rezC)
	pet := &NPC{ServerID: 3001, Name: "Fire Elemental ", Level: 39, HP: 2340, MaxHP: 2340, Owner: c.ServerID, X: 102, Y: 10, Z: 104}
	crab := &NPC{ServerID: 3002, Name: "Desert Crab", Level: 34, HP: 2040, MaxHP: 2040, NPCType: 0x0100, X: 98, Y: 10, Z: 98}
	w.NPCs[pet.ServerID], w.NPCs[crab.ServerID] = pet, crab
	see := func(s *session.Session, ids map[EntityID]uint32) {
		vs := GetOrCreateVisibility(s)
		i := 0
		for e, o := range ids {
			vs.ChannelEntity[i], vs.ChannelObjectID[i] = e, o
			i++
		}
	}
	see(r.sa, map[EntityID]uint32{b.ServerID: rezB, c.ServerID: rezC, pet.ServerID: rezPet, crab.ServerID: rezCrab})
	see(r.sb, map[EntityID]uint32{a.ServerID: rezA, c.ServerID: rezC})
	see(r.sc, map[EntityID]uint32{a.ServerID: rezA, b.ServerID: rezB})
	return r
}

func (r *rezRig) at(d time.Duration) time.Time { return r.t0.Add(d) }

// die is p's death at d, by the death path.
func (r *rezRig) die(p *Player, s *session.Session, d time.Duration) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.playerDiedLocked(s, p, r.at(d))
}

// respawn runs the respawn at d: the dead whose 15 s are over stand again.
func (r *rezRig) respawn(d time.Duration) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.playerRespawnSystem(r.at(d))
}

// land runs the casts whose landing has come by d.
func (r *rezRig) land(d time.Duration) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.castSystem(r.at(d))
}

// cast begins spell from s's player at target at d and lands it; it reports
// whether it began.
func (r *rezRig) cast(s *session.Session, spell CastSpell, target uint32, d time.Duration) bool {
	if !r.w.StartCast(s, spell, target, r.at(d)) {
		return false
	}
	r.land(d + spell.CastTime + castLand)
	return true
}

// linesOf is every line in msgs, plain and coloured.
func linesOf(msgs [][]byte) []string {
	var out []string
	for _, m := range msgs {
		if op := opOfMsg(m); op == protocol.OpcodeClientMessage || op == protocol.OpcodeColoredChat {
			out = append(out, chatText(m))
		}
	}
	return out
}

// debtsOf is every debt 0x00D9 told in msgs.
func debtsOf(msgs [][]byte) []uint32 {
	var out []uint32
	for _, m := range msgs {
		if len(m) >= 6 && opOfMsg(m) == protocol.OpcodeExpDebt {
			out = append(out, binary.LittleEndian.Uint32(m[2:]))
		}
	}
	return out
}

func hasLine(lines []string, want string) bool {
	for _, l := range lines {
		if l == want {
			return true
		}
	}
	return false
}

// The rez spells as eqoaclassic's records have them (cast 5, recast 1, range
// 30, the 16880 of their record; Renew's range 0, no limit), with
// EQOA-Data's lines.
func rezSpell(id int, name, line string, power int, rng float32) CastSpell {
	return CastSpell{SpellID: id, DisplayIndex: id % 50, Level: 20, Scope: 1, Name: name, CastTime: 5 * time.Second,
		Recast: time.Second, PowerCost: power, Range: rng, Def: ParseSpellNamed(name, line, line)}
}

func revive() CastSpell {
	return rezSpell(700209, "Revive", "Target, Ressurect Player: restore 20% experience debt", 100, 30)
}

func renew() CastSpell {
	return rezSpell(453, "Renew", "Target, Ressurect Player: restore 10% experience debt", 100, 0)
}

func faithsRevival() CastSpell {
	return rezSpell(700887, "Faith's Revival", "Self, Ressurect Player: restore 20% experience debt", 100, 30)
}

// refusedNoRez checks a rez refused as it began: the fizzle's brief cast,
// the red line, nothing taken.
func refusedNoRez(t *testing.T, began bool, msgs [][]byte, want string, p *Player, power, lockouts int) {
	t.Helper()
	if began {
		t.Fatalf("begun; want %q", want)
	}
	if len(msgs) != 2 || opOfMsg(msgs[0]) != protocol.OpcodeCastingCombat || opOfMsg(msgs[1]) != protocol.OpcodeColoredChat {
		t.Fatalf("sent %x, want the fizzle and a line", msgs)
	}
	if ms := binary.LittleEndian.Uint32(msgs[0][11:]); ms != uint32(charmFizzle/time.Millisecond) {
		t.Errorf("the fizzle lasts %d ms", ms)
	}
	if got := chatText(msgs[1]); got != want || !strings.HasSuffix(string(msgs[1]), string(castRefusalRGB[:])) {
		t.Errorf("the line %q (%x), want %q in red", got, msgs[1], want)
	}
	if p.Power != power || len(p.Lockouts) != lockouts {
		t.Errorf("power %d (was %d), lockouts %d (were %d)", p.Power, power, len(p.Lockouts), lockouts)
	}
}

// Bran dies at level 49 (the chart's 6,137,969) and Gnolie's Revive lands on
// the body 8.4 s later: 20% of that death's debt off what Bran owes, the
// chance spent, 0x00D9 with the new debt and a line to Bran, a line to
// Gnolie, its power spent. The body stays down and the respawn at 15 s is
// the bind point's as ever, the lighter debt kept (ours: a rez revives no
// one). The control: before the cast, the death's debt is all there.
func TestARezGivesBackItsShareOnce(t *testing.T) {
	r := newRezRig(t)
	r.die(r.b, r.sb, 0)
	if r.b.Debt != 6137969 || r.b.Rez.Debt != 6137969 {
		t.Fatalf("after the death: debt %d, chance %+v", r.b.Debt, r.b.Rez)
	}
	drain(r.sa)
	drain(r.sb)
	if !r.cast(r.sa, revive(), rezB, 2*time.Second) {
		t.Fatal("the rez at the body did not begin")
	}
	want := 6137969 - 1227594 // 20% of the death's debt, rounded
	if r.b.Debt != want {
		t.Fatalf("Bran owes %d, want %d", r.b.Debt, want)
	}
	if r.b.Rez != (RezChance{}) {
		t.Errorf("the chance is not spent: %+v", r.b.Rez)
	}
	if r.a.Power != 700 {
		t.Errorf("Gnolie's power %d, want 700", r.a.Power)
	}
	toB, toA := drain(r.sb), drain(r.sa)
	if d := debtsOf(toB); len(d) != 1 || d[0] != uint32(want) {
		t.Errorf("0x00D9 to Bran %v, want [%d]", d, want)
	}
	if !hasLine(linesOf(toB), "Gnolie's Revive took 1227594 off your experience debt (4910375 left).") {
		t.Errorf("Bran told %q", linesOf(toB))
	}
	if !hasLine(linesOf(toA), "Your Revive took 1227594 off Bran's experience debt.") {
		t.Errorf("Gnolie told %q", linesOf(toA))
	}
	if !r.b.IsDead() {
		t.Error("the rez revived the body")
	}
	r.respawn(15 * time.Second)
	if r.b.IsDead() || r.b.Debt != want {
		t.Errorf("after the respawn: dead %v, debt %d", r.b.IsDead(), r.b.Debt)
	}
}

// Once per death: a second rez is refused as it begins (the fizzle, the red
// line, no power, no lockout), and of two begun before either lands the
// second finds nothing, its power spent, its caster told.
func TestASecondRezDoesNothing(t *testing.T) {
	r := newRezRig(t)
	r.die(r.b, r.sb, 0)
	if !r.cast(r.sa, revive(), rezB, 2*time.Second) {
		t.Fatal("the first rez did not begin")
	}
	debt := r.b.Debt
	drain(r.sa)
	power, lockouts := r.a.Power, len(r.a.Lockouts)
	began := r.w.StartCast(r.sa, revive(), rezB, r.at(10*time.Second))
	refusedNoRez(t, began, drain(r.sa), "Bran has no experience to recover.", r.a, power, lockouts)
	if r.b.Debt != debt {
		t.Errorf("Bran owes %d after the second rez, want %d", r.b.Debt, debt)
	}

	r2 := newRezRig(t)
	r2.die(r2.b, r2.sb, 0)
	if !r2.w.StartCast(r2.sa, revive(), rezB, r2.at(time.Second)) || !r2.w.StartCast(r2.sc, revive(), rezB, r2.at(2*time.Second)) {
		t.Fatal("the two rezzes did not both begin")
	}
	r2.land(7400 * time.Millisecond) // Gnolie's
	first := r2.b.Debt
	drain(r2.sc)
	r2.land(8400 * time.Millisecond) // Liss's
	if first != 6137969-1227594 || r2.b.Debt != first {
		t.Errorf("Bran owes %d after the first, %d after the second; want %d both", first, r2.b.Debt, 6137969-1227594)
	}
	if r2.c.Power != 700 {
		t.Errorf("Liss's power %d, want 700: a rez that lands spends it", r2.c.Power)
	}
	if !hasLine(linesOf(drain(r2.sc)), "Bran has no experience to recover.") {
		t.Error("Liss was not told hers found nothing")
	}
}

// The latest death alone: Bran dies at 30 (738,036), stands again, and dies
// at 31 (862,615). A rez gives back 20% of the second death's debt, not of
// both and not of the first. So does one begun on the first death's chance
// that lands after the second death.
func TestASecondDeathRetiresTheFirst(t *testing.T) {
	want := 738036 + 862615 - 172523
	r := newRezRig(t)
	r.b.Level = 30
	r.die(r.b, r.sb, 0)
	r.respawn(15 * time.Second)
	r.b.Level = 31
	r.die(r.b, r.sb, time.Minute)
	if r.b.Debt != 738036+862615 || r.b.Rez.Debt != 862615 {
		t.Fatalf("after two deaths: debt %d, chance %+v", r.b.Debt, r.b.Rez)
	}
	if !r.cast(r.sa, revive(), rezB, time.Minute+2*time.Second) {
		t.Fatal("not begun")
	}
	if r.b.Debt != want {
		t.Errorf("Bran owes %d, want %d: 20%% of the latest death's 862615 alone", r.b.Debt, want)
	}

	r2 := newRezRig(t)
	r2.b.Level = 30
	r2.die(r2.b, r2.sb, 0)
	r2.respawn(15 * time.Second)
	if !r2.w.StartCast(r2.sa, revive(), rezB, r2.at(20*time.Second)) {
		t.Fatal("the rez on the first death's chance did not begin")
	}
	r2.b.Level = 31
	r2.die(r2.b, r2.sb, 22*time.Second)
	r2.land(26400 * time.Millisecond)
	if r2.b.Debt != want {
		t.Errorf("a rez landing after the second death: Bran owes %d, want %d", r2.b.Debt, want)
	}
}

// Below level 6 a death adds no debt, and a rez still offers its target the
// way back to where it died (petition 1051, Jibheals: "Even if the character
// has no debt to clear, they should still get the popup to get back to where
// they last died IF eligible to rez"): the cast lands, CLIENT's prompt goes
// (the hook), a Yes in its time leads back to the body (the target back to
// life when it still lies dead), a No or a late answer nowhere. The control: a
// player who never died has nothing to offer, and is refused as ever.
func TestARezOffersTheWayBackDebtOrNone(t *testing.T) {
	r := newRezRig(t)
	var offered []string
	r.w.OnRezOffer(func(s *session.Session, caster string) { offered = append(offered, caster) })
	r.b.Level = 5
	r.die(r.b, r.sb, 0)
	if r.b.Debt != 0 || !r.b.Rez.returns(r.at(time.Second)) || r.b.Rez.X != 106 || r.b.Rez.Z != 100 {
		t.Fatalf("a level 5 death: debt %d, chance %+v", r.b.Debt, r.b.Rez)
	}
	drain(r.sa)
	if !r.cast(r.sa, revive(), rezB, 2*time.Second) {
		t.Fatalf("the rez was refused: %q", linesOf(drain(r.sa)))
	}
	if len(offered) != 1 || offered[0] != "Gnolie" || r.b.RezReturn.Until.IsZero() || r.b.Rez != (RezChance{}) {
		t.Fatalf("the offer: %q, return %+v, chance %+v", offered, r.b.RezReturn, r.b.Rez)
	}
	if !r.b.respawnPending() {
		t.Fatal("not dead before the answer")
	}
	world, x, _, z, ok := r.w.TakeRezReturn(r.sb, true, r.at(10*time.Second))
	if !ok || world != 0 || x != 106 || z != 100 || r.b.respawnPending() || r.b.HP != r.b.MaxHP {
		t.Fatalf("the Yes: %v (%v, %v) world %d; dead %v, HP %d", ok, x, z, world, r.b.respawnPending(), r.b.HP)
	}
	if _, _, _, _, ok := r.w.TakeRezReturn(r.sb, true, r.at(11*time.Second)); ok {
		t.Fatal("a second Yes led back again")
	}

	r2 := newRezRig(t)
	r2.w.OnRezOffer(func(*session.Session, string) {})
	r2.die(r2.b, r2.sb, 0)
	r2.cast(r2.sa, revive(), rezB, 2*time.Second)
	if _, _, _, _, ok := r2.w.TakeRezReturn(r2.sb, false, r2.at(5*time.Second)); ok {
		t.Fatal("a No led back")
	}
	r2.die(r2.b, r2.sb, time.Minute)
	r2.cast(r2.sa, revive(), rezB, time.Minute+2*time.Second)
	if _, _, _, _, ok := r2.w.TakeRezReturn(r2.sb, true, r2.at(time.Minute+RezOfferFor+5*time.Second)); ok {
		t.Fatal("a late Yes led back")
	}

	r3 := newRezRig(t)
	drain(r3.sa)
	began := r3.w.StartCast(r3.sa, revive(), rezB, r3.at(2*time.Second))
	refusedNoRez(t, began, drain(r3.sa), "Bran has no experience to recover.", r3.a, 800, 0)
}

// Kept over a save and load, its time counted in the world only: Bran logs
// out ten minutes after its death (twenty left), and logs in three days
// later. A rez 19 minutes into that session lands; in another session from
// the same row, one 20 minutes and a second in is refused (ours: 30 minutes
// in the world). A chance spent is kept as no row at all.
func TestTheChanceIsKeptOverASaveAndLoad(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/rez.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := database.NewSQLiteCharacterStore(db)

	r := newRezRig(t)
	r.die(r.b, r.sb, 0)
	row, ok := r.w.SavedRez(r.b.ServerID, r.at(10*time.Minute))
	if !ok || row != (database.RezRow{Debt: 6137969, LeftMS: 20 * 60 * 1000, Placed: true, X: 106, Y: 10, Z: 100}) {
		t.Fatalf("kept %+v (%v), want the death's debt, its place and twenty minutes", row, ok)
	}
	if err := store.SaveRez(r.b.CharServerID, row); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadRez(r.b.CharServerID)
	if err != nil || loaded != row {
		t.Fatalf("loaded %+v (%v), want %+v", loaded, err, row)
	}

	login := func() *rezRig {
		n := newRezRig(t)
		n.t0 = r.t0.Add(72 * time.Hour)
		n.b.Debt = 6137969 // characters.debt, as the logout kept it
		if !n.w.RestoreRez(n.sb, loaded, n.t0) {
			t.Fatal("the kept chance does not count at the login")
		}
		return n
	}
	r2 := login()
	if !r2.cast(r2.sa, revive(), rezB, 19*time.Minute) || r2.b.Debt != 6137969-1227594 {
		t.Fatalf("a rez 19 minutes after the login: Bran owes %d", r2.b.Debt)
	}
	if b := r2.b.RezReturn; b.X != 106 || b.Y != 10 || b.Z != 100 || b.Until.IsZero() { // petition 1130
		t.Fatalf("the way back after the login: %+v", b)
	}
	r3 := login()
	drain(r3.sa)
	began := r3.w.StartCast(r3.sa, revive(), rezB, r3.at(20*time.Minute+time.Second))
	refusedNoRez(t, began, drain(r3.sa), "Bran has no experience to recover.", r3.a, 800, 0)

	spent, ok := r2.w.SavedRez(r2.b.ServerID, r2.at(20*time.Minute))
	if !ok || spent != (database.RezRow{}) {
		t.Fatalf("a spent chance kept as %+v", spent)
	}
	if err := store.SaveRez(r2.b.CharServerID, spent); err != nil {
		t.Fatal(err)
	}
	if got, err := store.LoadRez(r2.b.CharServerID); err != nil || got != (database.RezRow{}) {
		t.Errorf("after a spent chance's save the row is %+v (%v)", got, err)
	}
	r4 := newRezRig(t)
	if r4.w.RestoreRez(r4.sb, database.RezRow{}, r4.t0) || r4.b.Rez != (RezChance{}) {
		t.Errorf("no row restores %+v", r4.b.Rez)
	}
}

// Petition 1130, the GM Jibrish: "Resurrect spells should work on all levels, including
// level 60 res on level 1." A rez the workbook gives a lowest level still lands on a
// level 1, whose death left no debt: the way back is offered. The control: a buff of
// that level and lowest level does not take on it.
func TestALevel60RezReachesLevel1(t *testing.T) {
	r := newRezRig(t)
	offered := 0
	r.w.OnRezOffer(func(*session.Session, string) { offered++ })
	r.a.Level, r.b.Level = 60, 1
	r.die(r.b, r.sb, 0)
	rez := wbSpell(700761, 60, 1, "Reclamation", `{"v":1,"scope":"target","target_type":"Friendly","harmful":false,"min_level":55,
		"effects":[{"id":"021D","type":"Ressurect Player","kind":"resurrect","rez_pct":-60}]}`)
	if rez.Def.Resurrects() != 60 {
		t.Fatalf("the rez's share: %d", rez.Def.Resurrects())
	}
	drain(r.sa)
	if !r.cast(r.sa, rez, rezB, 2*time.Second) {
		t.Fatalf("refused: %q", linesOf(drain(r.sa)))
	}
	if offered != 1 || r.b.RezReturn.Until.IsZero() {
		t.Errorf("the offer: %d, %+v", offered, r.b.RezReturn)
	}
	buff := wbSpell(971, 60, 1, "Test Ward", `{"v":1,"scope":"target","target_type":"Friendly","min_level":55,"duration_s":600,
		"effects":[{"kind":"stat","stat":"ac","amount":10}]}`)
	alive := &Player{ServerID: 1009, Name: "Newbie", Level: 1, HP: 100, MaxHP: 100}
	r.w.mu.Lock()
	r.w.landOnPlayerLocked(&castState{spell: buff, caster: r.a.ServerID}, r.a, r.sa, alive, r.at(time.Minute))
	r.w.mu.Unlock()
	if len(alive.Effects) != 0 {
		t.Errorf("the level 60 buff took on level 1")
	}
}

// Petition 1130: "One rez per death, if they die again they lose the ability to get
// rez'd from the previous death. Only the latest rez death location is saved." A death
// ends the offer of the one before it, unanswered or not, and its share; the next rez
// offers the latest place and gives a share of the latest debt, once.
func TestOnlyTheLatestDeathCounts(t *testing.T) {
	r := newRezRig(t)
	r.w.OnRezOffer(func(*session.Session, string) {})
	r.die(r.b, r.sb, 0)
	if !r.cast(r.sa, revive(), rezB, 2*time.Second) || r.b.RezReturn.X != 106 {
		t.Fatalf("the first rez: %+v", r.b.RezReturn)
	}
	first := r.b.Debt
	r.respawn(20 * time.Second)
	r.b.X, r.b.Z = 500, 600
	r.die(r.b, r.sb, 30*time.Second)
	if r.b.RezReturn != (RezReturn{}) {
		t.Fatalf("the second death left the first's offer: %+v", r.b.RezReturn)
	}
	if _, _, _, _, ok := r.w.TakeRezReturn(r.sb, true, r.at(31*time.Second)); ok {
		t.Fatal("a Yes to the first offer led somewhere after a later death")
	}
	if r.b.Debt <= first {
		t.Fatalf("the second death added no debt: %d, was %d", r.b.Debt, first)
	}
	added, owed := r.b.Debt-first, r.b.Debt
	if !r.cast(r.sa, renew(), rezB, 40*time.Second) { // no recast of its own, unlike Revive
		t.Fatalf("the rez of the latest death was refused: %q", linesOf(drain(r.sa)))
	}
	if want := owed - (added*10+50)/100; r.b.Debt != want {
		t.Errorf("the second rez left %d owed, want %d: 10%% of the latest death's", r.b.Debt, want)
	}
	if b := r.b.RezReturn; b.X != 500 || b.Z != 600 {
		t.Errorf("the way back: %+v, want the latest death's place", b)
	}
	if r.b.Rez != (RezChance{}) {
		t.Errorf("the chance not spent: %+v", r.b.Rez)
	}
	drain(r.sa)
	if r.w.StartCast(r.sa, renew(), rezB, r.at(50*time.Second)) {
		t.Error("a second rez of one death began")
	}
}

// The lines read: every resurrection of EQOA-Data's lists as eqoaclassic
// keeps them, with the target word each has, nothing left unread and nothing
// harmful; and the other words a line may use. The controls: lines that are
// no rez, the "Renewal" pet heal and "Restorative" among them, read none.
func TestTheRezLinesRead(t *testing.T) {
	for _, c := range []struct {
		name, line string
		pct        int
		target     SpellTarget
	}{
		{"Revive", "Target, Ressurect Player: restore 20% experience debt", 20, TargetSingle},
		{"Resuscitate", "Target, Ressurect Player: restore 30% experience debt", 30, TargetSingle},
		{"Resurrection", "Target, Ressurect Player: restore 40% experience debt", 40, TargetSingle},
		{"Reviviscence", "Target, Ressurect Player: restore 50% experience debt", 50, TargetSingle},
		{"Reclamation", "Ressurect Player: restore 60% experience debt", 60, TargetSingle},
		{"Divine Revival", "Target, 80% REZ", 80, TargetSingle},
		{"Renew", "Target, Ressurect Player: restore 10% experience debt", 10, TargetSingle},
		{"Nature's Renewal", "Target, Ressurect Player: restore 25% experience debt", 25, TargetSingle},
		{"Reanimate", "Target, Ressurect Player: restore 10% experience debt", 10, TargetSingle},
		{"Faith's Revival", "Self, Ressurect Player: restore 20% experience debt", 20, TargetSelf},
		{"Faith's Reward", "Self, Ressurect Player: restore 30% experience debt", 30, TargetSelf},
		{"Faith's Boon", "Self, Ressurect Player: restore 40% experience debt", 40, TargetSelf},
		{"Faith's Succor", "Self, Ressurect Player: restore 50% experience debt", 50, TargetSelf},
		{"Faith's Renewal", "Self, Ressurect Player: restore 60% experience debt", 60, TargetSelf},
		{"a", "Target, Resurrect Player: restore 35% experience debt", 35, TargetSingle},
		{"b", "Target, 35% reduction rez", 35, TargetSingle},
		{"c", "Target, Rez 15%", 15, TargetSingle},
		{"d", "Target, Resurrection: 90% debt", 90, TargetSingle},
	} {
		d := ParseSpellNamed(c.name, c.line, "")
		if d.Resurrects() != c.pct || d.Target != c.target || len(d.Unread) != 0 || d.Harmful || len(d.Effects) != 1 {
			t.Errorf("%s %q: share %d, target %d, unread %q, harmful %v, effects %+v; want %d%%",
				c.name, c.line, d.Resurrects(), d.Target, d.Unread, d.Harmful, d.Effects, c.pct)
		}
	}
	for _, line := range []string{
		"Dispel Effect x3", "Pet, Increase Hitpoints by 539", "Target, Increase HP by 100",
		"Group, Increase HP per tic by 76 ; Increase 10% based on CHA", "Self, Increase FR & CR by 20, 30min",
		"Target, Decrease Hitpoints by 26 ; Increase 8% based on DEX", "Target, Resist Fire 25%", "",
	} {
		if d := ParseSpellNamed("Renewal", line, "Player Resurrection."); d.Resurrects() != 0 {
			t.Errorf("%q reads as a rez of %d%%", line, d.Resurrects())
		}
	}
}

// Every line of EQOA-Data's lists that says resurrect or rez reads as one,
// and no other line does: 14 of them (EQOA_DATA, else a sibling checkout).
func TestEQOADataRezLinesRead(t *testing.T) {
	n := 0
	for _, s := range eqoaDataSpells(t) {
		low := strings.ToLower(s.Line)
		says := strings.Contains(low, "ressurect") || strings.Contains(low, "rez")
		d := ParseSpell(s.Line, "")
		if (d.Resurrects() > 0) != says {
			t.Errorf("%s %q: rez %d%%", s.Name, s.Line, d.Resurrects())
		}
		if says {
			n++
		}
	}
	if n != 14 {
		t.Errorf("%d resurrection lines, want EQOA-Data's 14", n)
	}
}

// A paladin's "Self" rez is its own, whatever is targeted: Bran, a paladin of
// 40, dies, stands again at its bind point, and casts Faith's Revival with
// Gnolie targeted, who died too. 20% of Bran's death's debt comes off Bran's;
// Gnolie's chance is left whole. With nothing targeted it lands the same.
func TestAPaladinsSelfRezIsItsOwn(t *testing.T) {
	for _, target := range []uint32{rezA, 0xFFFFFFFF} {
		r := newRezRig(t)
		r.b.Class, r.b.Level = 2, 40
		r.die(r.b, r.sb, 0) // 2,662,682
		r.die(r.a, r.sa, 0) // 6,658,528
		r.respawn(15 * time.Second)
		drain(r.sb)
		if !r.cast(r.sb, faithsRevival(), target, 20*time.Second) {
			t.Fatalf("target %#x: not begun", target)
		}
		if want := 2662682 - 532536; r.b.Debt != want {
			t.Errorf("target %#x: Bran owes %d, want %d", target, r.b.Debt, want)
		}
		if r.a.Debt != 6658528 || r.a.Rez.Debt != 6658528 {
			t.Errorf("target %#x: Gnolie's debt %d and chance %+v were touched", target, r.a.Debt, r.a.Rez)
		}
		if !hasLine(linesOf(drain(r.sb)), "Your Faith's Revival took 532536 off your experience debt (2130146 left).") {
			t.Errorf("target %#x: Bran not told", target)
		}
	}
}

// Whom a rez reaches is castTargetsLocked's: Revive's 30, not 40 off ("Out
// of range.", nothing taken); Renew's range of 0 none; a creature is not
// friendly; a pet, which owes nothing, has nothing to recover.
func TestARezReachesAsAnySpellDoes(t *testing.T) {
	r := newRezRig(t)
	r.die(r.b, r.sb, 0)
	r.b.X = 140
	drain(r.sa)
	if r.w.StartCast(r.sa, revive(), rezB, r.at(2*time.Second)) || r.a.Power != 800 ||
		!hasLine(linesOf(drain(r.sa)), castOutOfRange) {
		t.Fatalf("Revive at 40 off began, or said otherwise (power %d)", r.a.Power)
	}
	if !r.cast(r.sa, renew(), rezB, 3*time.Second) || r.b.Debt != 6137969-613797 {
		t.Errorf("Renew at 40 off: Bran owes %d, want %d", r.b.Debt, 6137969-613797)
	}
	drain(r.sa)
	if r.w.StartCast(r.sa, revive(), rezCrab, r.at(20*time.Second)) || !hasLine(linesOf(drain(r.sa)), castNotFriendly) {
		t.Error("a rez at a crab is not refused as not friendly")
	}
	lockouts := len(r.a.Lockouts)
	began := r.w.StartCast(r.sa, revive(), rezPet, r.at(21*time.Second))
	refusedNoRez(t, began, drain(r.sa), "Fire Elemental has no experience to recover.", r.a, 700, lockouts)
}

// What it gives back is at most what is still owed: Bran, who has paid all
// but 1,000 of its death's debt, owes nothing after a Revive, and is told it
// is paid off.
func TestARezTakesNoMoreThanIsOwed(t *testing.T) {
	r := newRezRig(t)
	r.die(r.b, r.sb, 0)
	r.b.Debt = 1000
	drain(r.sb)
	if !r.cast(r.sa, revive(), rezB, 2*time.Second) || r.b.Debt != 0 {
		t.Fatalf("Bran owes %d, want 0", r.b.Debt)
	}
	toB := drain(r.sb)
	if d := debtsOf(toB); len(d) != 1 || d[0] != 0 {
		t.Errorf("0x00D9 to Bran %v, want [0]", d)
	}
	if !hasLine(linesOf(toB), "Gnolie's Revive took 1000 off your experience debt, and it is paid off.") {
		t.Errorf("Bran told %q", linesOf(toB))
	}
}
