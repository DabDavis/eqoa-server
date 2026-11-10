package entity

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The damage meter (dpsmeter.go): what each player of a group is told it did,
// counted once on the meter of every member, in fights that end when the group
// goes quiet, sent to the clients that asked and to no other.

// meterMsg is one OpcodeDPSMeterData as a client reads it.
type meterMsg struct {
	fight    uint32
	over     bool
	lengthMS uint32
	total    uint32
	all      int
	place    int
	lines    []meterMsgLine
}

type meterMsgLine struct {
	flags       byte
	class       byte
	damage, pet uint32
	name        string
}

func (l meterMsgLine) String() string {
	return fmt.Sprintf("%s(%d) %d pet %d flags %d", l.name, l.class, l.damage, l.pet, l.flags)
}

// readMeterMsg reads a body past its opcode; it fails the test on one cut
// short or with bytes past its last line.
func readMeterMsg(t *testing.T, b []byte) meterMsg {
	t.Helper()
	at := 0
	need := func(n int) []byte {
		if at+n > len(b) {
			t.Fatalf("0x7E3C % X: short at %d", b, at)
		}
		s := b[at : at+n]
		at += n
		return s
	}
	if v := need(1)[0]; v != 1 {
		t.Fatalf("layout %d", v)
	}
	m := meterMsg{fight: binary.LittleEndian.Uint32(need(4))}
	m.over = need(1)[0] == 1
	m.lengthMS = binary.LittleEndian.Uint32(need(4))
	m.total = binary.LittleEndian.Uint32(need(4))
	m.all = int(binary.LittleEndian.Uint16(need(2)))
	m.place = int(binary.LittleEndian.Uint16(need(2)))
	n := int(need(1)[0])
	for range n {
		var l meterMsgLine
		h := need(2)
		l.flags, l.class = h[0], h[1]
		l.damage = binary.LittleEndian.Uint32(need(4))
		l.pet = binary.LittleEndian.Uint32(need(4))
		l.name = string(need(int(need(1)[0])))
		m.lines = append(m.lines, l)
	}
	if at != len(b) {
		t.Fatalf("0x7E3C % X: %d bytes past its lines", b, len(b)-at)
	}
	return m
}

// meterMsgs drains s's queue and returns its meter messages, in order.
func meterMsgs(t *testing.T, s *session.Session) []meterMsg {
	t.Helper()
	var out []meterMsg
	for _, m := range s.Queue().GetPending() {
		b := m.Data()
		if len(b) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(b)) == protocol.OpcodeDPSMeterData {
			out = append(out, readMeterMsg(t, b[2:]))
		}
	}
	return out
}

// meterClock makes the meter's clock the test's, from t0, and puts it back when
// the test ends; set moves it.
func meterClock(t *testing.T, t0 time.Time) (set func(time.Duration) time.Time) {
	t.Helper()
	at := t0
	was := dpsMeterNow
	dpsMeterNow = func() time.Time { return at }
	t.Cleanup(func() { dpsMeterNow = was })
	return func(d time.Duration) time.Time {
		at = t0.Add(d)
		return at
	}
}

// group makes the players one group, the first its leader.
func group(w *World, id uint32, ps ...*Player) *Group {
	g := &Group{ID: id, lastRoster: map[EntityID]string{}}
	for _, p := range ps {
		g.Members = append(g.Members, p.ServerID)
		p.GroupID = id
	}
	if w.groups == nil {
		w.groups = make(map[uint32]*Group)
	}
	w.groups[id] = g
	return g
}

// line is p's meter's line of maker: its damage and its pet's share; ok false
// when it has none.
func line(p, maker *Player) (damage, pet int64, ok bool) {
	if p.dps == nil || p.dps.lines == nil {
		return 0, 0, false
	}
	l := p.dps.lines[maker.ServerID]
	if l == nil {
		return 0, 0, false
	}
	return l.damage, l.pet, true
}

// counted is what of the figures told the meter counts: damage (a negative
// amount) of a tag (its maker's, numbertags.go), not on the player itself nor
// on its own pet; and the pet's share of it.
func counted(fs []figTag, me, petO uint32) (damage, pet int64) {
	for _, f := range fs {
		if f.amount >= 0 || f.tag < 0 || f.dst == me || f.dst == petO {
			continue
		}
		damage += int64(-f.amount)
		if f.tag&0x80 != 0 {
			pet += int64(-f.amount)
		}
	}
	return damage, pet
}

// Every path that tells a player a figure of its making (TestNumberTagsByPath's
// list): the meter counts exactly the damage told, the pet's apart, on the
// player's own meter and on its group's friend's, once each. The controls are
// figures told that are not damage a meter counts: heals, a figure another
// made on the player, a hit on its own pet (Destroy Contraption); each is told,
// and counts nothing.
func TestDPSMeterCountsTheFiguresTold(t *testing.T) {
	defer func(f func(int) int) { procIntn = f }(procIntn)
	hit := func(n int) SpellEffect { return SpellEffect{Kind: EffectHP, Amount: -n, AmountMax: -n} }
	heal := func(n int) SpellEffect { return SpellEffect{Kind: EffectHP, Amount: n, AmountMax: n} }
	tick := func(n int) SpellEffect { return SpellEffect{Kind: EffectHPTick, Amount: n, AmountMax: n} }
	for _, c := range []struct {
		name   string
		run    func(r *tagRig)
		counts bool // some damage is counted; false: the control, told and none counted
	}{
		{"a swing", func(r *tagRig) { r.w.playerHitLocked(r.sess, r.p, r.npc, r.npcO) }, true},
		{"a swing on a training dummy", func(r *tagRig) {
			r.npc.Dummy, r.npc.Name = TrainingDummy, "Training Dummy Lv20"
			r.w.playerHitLocked(r.sess, r.p, r.npc, r.npcO)
		}, true},
		{"a nuke", func(r *tagRig) { r.w.landOnNPCLocked(r.cast(hit(50)), r.p, r.sess, r.npc, r.t0) }, true},
		{"a damage over time's tick", func(r *tagRig) {
			r.w.landOnNPCLocked(r.cast(tick(-20)), r.p, r.sess, r.npc, r.t0)
		}, true},
		{"a life tap: the hit, not the take", func(r *tagRig) {
			e := hit(40)
			e.Drain = DrainCaster
			r.w.landOnNPCLocked(r.cast(e), r.p, r.sess, r.npc, r.t0)
		}, true},
		{"a damage shield's answer", func(r *tagRig) {
			r.p.Effects = []*ActiveEffect{{SpellID: 9200, Name: "Test Shield", Until: r.later, Shield: &Shield{Min: 12, Max: 12}}}
			r.w.playerShieldsLocked(r.sess, r.p, r.npc, r.t0)
		}, true},
		{"a defensive proc", func(r *tagRig) {
			procIntn = dice(0, 99)
			r.p.Effects = []*ActiveEffect{{SpellID: 9201, Name: "Test Ward", Until: r.later, Shield: &Shield{ProcMin: 30, ProcMax: 30}}}
			r.w.playerShieldsLocked(r.sess, r.p, r.npc, r.t0)
		}, true},
		{"an effect's proc, not its life steal", func(r *tagRig) {
			procIntn = dice(0, 99)
			r.p.Effects = []*ActiveEffect{{SpellID: 9202, Name: "Test Flux", Until: r.later, Proc: 40, ProcMax: 40, Rules: EffectRules{ProcHeal: 25}}}
			r.w.playerProcsLocked(r.sess, r.p, r.npc, r.t0)
		}, true},
		{"a weapon's proc", func(r *tagRig) {
			procIntn = dice(0, 99)
			r.p.Gear.MainProc = &WeaponProc{Item: "Test Blade", Kind: ProcHit, Damage: 33, Chance: 100}
			r.w.weaponProcLocked(r.sess, r.p, r.npc, r.t0)
		}, true},
		{"a weapon's tap", func(r *tagRig) {
			procIntn = dice(0, 99)
			r.p.Gear.MainProc = &WeaponProc{Item: "Test Saw", Kind: ProcTap, Damage: 25, Chance: 100}
			r.w.weaponProcLocked(r.sess, r.p, r.npc, r.t0)
		}, true},
		{"a worn item's answer", func(r *tagRig) {
			r.p.Gear.Shields = []WornShield{{Item: "Test Crown", Damage: 30, Heal: 15}}
			r.w.wornShieldsLocked(r.sess, r.p, r.npc)
		}, true},
		{"a spell proc's hit and tick, not its heal", func(r *tagRig) {
			procIntn = dice(0, 99)
			own := heal(22)
			own.Self = true
			r.p.Effects = []*ActiveEffect{{SpellID: 9205, Name: "Test Fervor", Until: r.later,
				ProcSpells: []ProcSpell{{Chance: 100, Duration: 12 * time.Second, Effects: []SpellEffect{own, hit(44), tick(-6)}}}}}
			r.w.procSpellsLocked(r.p, r.sess, r.npc, false, r.t0)
		}, true},
		{"its pet's swing", func(r *tagRig) { r.w.petHitLocked(r.ps, r.npc, r.t0) }, true},
		{"its pet's taunt", func(r *tagRig) { r.w.petTauntLocked(r.ps, r.npc, r.t0) }, true},
		{"its pet's damage over time's tick", func(r *tagRig) {
			ae := &ActiveEffect{SpellID: 9208, Name: "Test Swarm", Caster: r.p.ServerID, pet: r.pet.ServerID, Until: r.later, NextTick: r.t0, HPTick: -15}
			r.npc.Effects = []*ActiveEffect{ae}
			r.w.tickNPCEffectLocked(r.npc, ae, r.t0)
		}, true},
		{"its pet's damage shield", func(r *tagRig) {
			r.pet.Effects = []*ActiveEffect{{SpellID: 9209, Name: "Test Shield", Until: r.later, Shield: &Shield{Min: 10, Max: 10}}}
			r.w.petShieldsLocked(r.pet, r.npc, r.t0)
		}, true},
		{"its pet's effect's proc", func(r *tagRig) {
			procIntn = dice(0, 99)
			r.pet.Effects = []*ActiveEffect{{SpellID: 9211, Name: "Test Flux", Until: r.later, Proc: 20, ProcMax: 20}}
			r.w.petProcsLocked(r.ps, r.npc, r.t0)
		}, true},
		// the controls: told, none counted
		{"a heal on oneself", func(r *tagRig) { r.w.landOnPlayerLocked(r.cast(heal(100)), r.p, r.sess, r.p, r.t0) }, false},
		{"a heal on a friend", func(r *tagRig) { r.w.landOnPlayerLocked(r.cast(heal(100)), r.p, r.sess, r.friend, r.t0) }, false},
		{"a heal over time's tick", func(r *tagRig) { r.w.landOnPlayerLocked(r.cast(tick(30)), r.p, r.sess, r.p, r.t0) }, false},
		{"its pet's heal on itself", func(r *tagRig) { r.w.tellPetHealLocked(r.ps, 40) }, false},
		{"a hit on its own pet (Destroy Contraption)", func(r *tagRig) {
			r.w.landOnPetLocked(r.cast(hit(300)), r.p, r.sess, r.pet, r.t0)
		}, false},
		{"a training dummy's hit and heal on it", func(r *tagRig) {
			r.npc.Dummy, r.npc.Name, r.npc.Level = TrainingDummy, "Training Dummy Lv20", 20
			r.p.AC = 600
			r.w.dummyHitLocked(r.sess, r.npc, r.p, r.t0)
		}, false},
		{"the sun on a vampire", func(r *tagRig) { r.w.sunBurnLocked(r.sess, r.p, 50, r.t0) }, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newTagRig(t)
			meterClock(t, r.t0)
			group(r.w, 77, r.p, r.friend)
			r.p.dps, r.friend.dps = &dpsMeter{}, &dpsMeter{}
			r.w.mu.Lock()
			c.run(r)
			r.w.mu.Unlock()
			told := figTags(t, r.sess)
			r.fsess.Queue().GetPending()
			if len(told) == 0 {
				t.Fatal("nothing told: the case makes no figure")
			}
			want, wantPet := counted(told, r.me, r.petO)
			if c.counts != (want > 0) {
				t.Fatalf("told %v: %d counts, want some %v", told, want, c.counts)
			}
			for _, v := range []*Player{r.p, r.friend} {
				got, gotPet, _ := line(v, r.p)
				if got != want || gotPet != wantPet {
					t.Errorf("%s's meter: %d (pet %d) of the enchanter's, want %d (pet %d) of %v", v.Name, got, gotPet, want, wantPet, told)
				}
			}
			if !c.counts && (r.p.dps.live || r.friend.dps.live) {
				t.Errorf("a control started a fight")
			}
		})
	}
}

// The tags only colour the figures: a session that never asked for them is
// counted the same (its player's figures are its own whatever its client
// draws), and so is a PS2's player on its group's meter.
func TestDPSMeterCountsWithoutTheTags(t *testing.T) {
	r := newTagRig(t)
	meterClock(t, r.t0)
	r.sess.SetNumberTags(false)
	group(r.w, 77, r.p, r.friend)
	r.friend.dps = &dpsMeter{} // the enchanter's client draws no meter: a PS2's
	r.w.mu.Lock()
	// the control: a figure of no kind is none of the player's making, on a
	// creature or not
	r.w.sendDamageIndication(r.sess, r.me, -500, r.npcO, 0)
	if r.friend.dps.live {
		r.w.mu.Unlock()
		t.Fatal("a figure of no kind counted")
	}
	r.w.landOnNPCLocked(r.cast(SpellEffect{Kind: EffectHP, Amount: -50, AmountMax: -50}), r.p, r.sess, r.npc, r.t0)
	r.w.petHitLocked(r.ps, r.npc, r.t0)
	r.w.mu.Unlock()
	var sum, pet int64
	for _, f := range figTags(t, r.sess)[1:] {
		if f.tag != -1 {
			t.Fatalf("%v tagged to a session that never asked", f)
		}
		sum -= int64(f.amount)
		if f.src == r.me && f.dst == r.npcO && f.amount != -50 {
			pet -= int64(f.amount)
		}
	}
	got, gotPet, ok := line(r.friend, r.p)
	if !ok || got != sum || gotPet != pet || sum <= 50 {
		t.Fatalf("the friend's meter has %d (pet %d), want the %d told (pet %d)", got, gotPet, sum, pet)
	}
	if r.p.dps != nil {
		t.Fatal("a meter for a client that never asked")
	}
}

// The necromancer's zombie fights untold (petrotation.go), and counts as its
// pet's share where the figure would be told; the control is ours, told and
// counted the same.
func TestDPSMeterCountsTheUntoldPets(t *testing.T) {
	for _, kind := range []petKind{petKindZombie, petKindOurs} {
		r := newTagRig(t)
		meterClock(t, r.t0)
		r.p.dps = &dpsMeter{}
		r.ps.kind = kind
		hp := r.npc.HP
		r.w.mu.Lock()
		r.w.petDamageLocked(r.ps, r.npc, 77, r.t0, numberMelee)
		r.w.mu.Unlock()
		told := figTags(t, r.sess)
		taken := int64(hp - r.npc.HP)
		got, pet, _ := line(r.p, r.p)
		if taken <= 0 || got != taken || pet != taken {
			t.Errorf("%s: the meter has %d (pet %d), the creature took %d", kind, got, pet, taken)
		}
		if (kind == petKindOurs) != (len(told) == 1) {
			t.Errorf("%s: told %v", kind, told)
		}
	}
}

// The scope is the group at each figure: a solo player's meter has itself, its
// group's every member's meter has every member's damage; one who joins
// mid-fight is counted from then on (its own meter's fight starts there), one
// who leaves is counted no more and its line is kept, flagged, until the fight
// ends. Figures of a player in no group reach no one else's meter (the
// control).
func TestDPSMeterScopeFollowsTheGroup(t *testing.T) {
	r := newTagRig(t)
	at := meterClock(t, r.t0)
	third := &Player{ServerID: 1003, Name: "Healer", Class: 9, Level: 20, HP: 700, MaxHP: 700, X: 101, Y: 10, Z: 101}
	tsess := r.w.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 10), Port: 7002}, 3, 3, 3, 4, true)
	r.w.Players[third.ServerID] = third
	tsess.SetECSEntity(third)
	tsess.SetCharacterInWorld(true)
	tsess.SetPlayerObjectID(0x0140CCC0)
	r.p.dps, r.friend.dps, third.dps = &dpsMeter{}, &dpsMeter{}, &dpsMeter{}
	friendHit := func(n int) {
		r.w.mu.Lock()
		r.w.sendDamageIndication(r.fsess, r.fsess.PlayerObjectID(), -n, r.npcF, numberMelee)
		r.w.mu.Unlock()
	}
	myHit := func(n int) {
		r.w.mu.Lock()
		r.w.sendDamageIndication(r.sess, r.me, -n, r.npcO, numberSpell)
		r.w.mu.Unlock()
	}
	// alone: each its own
	myHit(100)
	friendHit(40)
	if d, _, _ := line(r.friend, r.p); d != 0 {
		t.Fatalf("the friend's meter has %d of the enchanter's, in no group", d)
	}
	if d, _, _ := line(r.p, r.p); d != 100 {
		t.Fatalf("the enchanter's own: %d", d)
	}
	r.p.dps, r.friend.dps = &dpsMeter{}, &dpsMeter{}
	// a group: each counts on both
	g := group(r.w, 77, r.p, r.friend)
	at(time.Second)
	myHit(100)
	friendHit(40)
	for _, v := range []*Player{r.p, r.friend} {
		mine, _, _ := line(v, r.p)
		theirs, _, _ := line(v, r.friend)
		if mine != 100 || theirs != 40 {
			t.Errorf("%s's meter: %d and %d, want 100 and 40", v.Name, mine, theirs)
		}
	}
	if third.dps.live {
		t.Fatal("the third's meter counted a group it is not in")
	}
	// the third joins: counted from its first figure, its own fight from the
	// group's next figure
	g.Members, third.GroupID = append(g.Members, third.ServerID), g.ID
	at(2 * time.Second)
	myHit(50)
	if d, _, _ := line(third, r.p); d != 50 || !third.dps.start.Equal(r.t0.Add(2*time.Second)) {
		t.Errorf("the third's meter: %d of the enchanter's from %v", d, third.dps.start)
	}
	if d, _, _ := line(r.p, r.p); d != 150 {
		t.Errorf("the enchanter's fight goes on: %d", d)
	}
	// the friend leaves: no more on the others', theirs no more on its
	r.w.mu.Lock()
	r.w.leaveGroupLocked(r.friend)
	r.w.mu.Unlock()
	at(3 * time.Second)
	friendHit(60)
	myHit(10)
	if d, _, _ := line(r.p, r.friend); d != 40 {
		t.Errorf("the enchanter's meter kept %d of the friend's, want the 40 before it left", d)
	}
	if d, _, _ := line(r.friend, r.p); d != 150 {
		t.Errorf("the friend's meter has %d of the enchanter's, want the 150 before it left", d)
	}
	if d, _, _ := line(r.friend, r.friend); d != 100 {
		t.Errorf("the friend's own: %d", d)
	}
	// on the wire: the one gone flagged 2, the receiver's own 1, a member of
	// none counted listed with 0
	r.w.mu.Lock()
	r.w.dpsMeterSystem(r.t0.Add(3 * time.Second))
	r.w.mu.Unlock()
	msgs := meterMsgs(t, r.sess)
	if len(msgs) != 1 {
		t.Fatalf("the enchanter was sent %d meters", len(msgs))
	}
	got := fmt.Sprint(msgs[0].lines)
	want := fmt.Sprint([]meterMsgLine{{flags: 1, class: classEnchanter, damage: 160, name: "Druid"}, {flags: 2, damage: 40, name: "Tank"}, {class: 9, name: "Healer"}})
	if got != want || msgs[0].all != 3 || msgs[0].total != 200 || msgs[0].place != 1 {
		t.Errorf("the enchanter's lines %s (%d in all, %d), want %s", got, msgs[0].all, msgs[0].total, want)
	}
}

// A fight on the wire: it starts at the first figure; a message at the next
// pass, then at most one a second while something changes and none while
// nothing does; its length from the first figure to the latest; DPSMeterIdle
// after the last a final one (over), and the next figure starts another under
// another number. The pet's share rides its owner's line, counted once.
func TestDPSMeterAFightsCourse(t *testing.T) {
	r := newTagRig(t)
	at := meterClock(t, r.t0)
	r.p.dps = &dpsMeter{}
	pass := func(d time.Duration) []meterMsg {
		r.w.mu.Lock()
		r.w.TickCount = 0
		r.w.dpsMeterSystem(r.t0.Add(d))
		r.w.mu.Unlock()
		return meterMsgs(t, r.sess)
	}
	hit := func(d time.Duration, n int, kind numberKind) {
		at(d)
		r.w.mu.Lock()
		r.w.sendDamageIndication(r.sess, r.me, -n, r.npcO, kind)
		r.w.mu.Unlock()
	}
	if got := pass(0); len(got) != 0 {
		t.Fatalf("no fight, sent %v", got)
	}
	hit(0, 100, numberMelee)
	m := pass(500 * time.Millisecond)
	if len(m) != 1 || m[0].over || m[0].lengthMS != 0 || len(m[0].lines) != 1 || m[0].lines[0].damage != 100 {
		t.Fatalf("the first pass sent %+v", m)
	}
	first := m[0].fight
	if got := pass(time.Second); len(got) != 0 {
		t.Fatalf("nothing changed, sent %+v", got)
	}
	hit(1200*time.Millisecond, 30, numberSpell.byPet())
	if got := pass(1300 * time.Millisecond); len(got) != 0 {
		t.Fatalf("changed under a second after the last, sent %+v", got)
	}
	m = pass(1500 * time.Millisecond)
	if len(m) != 1 || m[0].fight != first || m[0].lengthMS != 1200 || m[0].lines[0].damage != 130 || m[0].lines[0].pet != 30 {
		t.Fatalf("a second on: %+v", m)
	}
	// quiet: nothing until DPSMeterIdle after the last figure, then its end
	if got := pass(1200*time.Millisecond + DPSMeterIdle - time.Millisecond); len(got) != 0 {
		t.Fatalf("ended early: %+v", got)
	}
	m = pass(1200*time.Millisecond + DPSMeterIdle)
	if len(m) != 1 || !m[0].over || m[0].fight != first || m[0].lengthMS != 1200 || m[0].lines[0].damage != 130 {
		t.Fatalf("the end: %+v", m)
	}
	if got := pass(20 * time.Second); len(got) != 0 {
		t.Fatalf("after its end: %+v", got)
	}
	hit(30*time.Second, 7, numberMelee)
	m = pass(30 * time.Second)
	if len(m) != 1 || m[0].over || m[0].fight == first || m[0].fight == 0 || m[0].lines[0].damage != 7 {
		t.Fatalf("the next fight: %+v (the first was %d)", m, first)
	}
}

// A reset drops the fight under way, nothing more sent of it, and the next
// figure starts another under another number; off forgets the meter; a client
// that never asked (a PS2's) is sent nothing, whatever its group does.
func TestDPSMeterResetOffAndNeverAsked(t *testing.T) {
	r := newTagRig(t)
	at := meterClock(t, r.t0)
	group(r.w, 77, r.p, r.friend)
	if !r.w.SetDPSMeter(r.sess, true) || !r.w.DPSMeter(r.sess) || r.w.DPSMeter(r.fsess) {
		t.Fatal("the answer not taken")
	}
	pass := func(d time.Duration) {
		r.w.mu.Lock()
		r.w.dpsMeterSystem(r.t0.Add(d))
		r.w.mu.Unlock()
	}
	hit := func(s *session.Session, me, foe uint32, n int) {
		r.w.mu.Lock()
		r.w.sendDamageIndication(s, me, -n, foe, numberMelee)
		r.w.mu.Unlock()
	}
	hit(r.sess, r.me, r.npcO, 100)
	hit(r.fsess, r.fsess.PlayerObjectID(), r.npcF, 80)
	pass(0)
	m := meterMsgs(t, r.sess)
	if len(m) != 1 || len(m[0].lines) != 2 || m[0].total != 180 {
		t.Fatalf("sent %+v", m)
	}
	if got := meterMsgs(t, r.fsess); len(got) != 0 {
		t.Fatalf("a client that never asked was sent %+v", got)
	}
	old := m[0].fight
	if !r.w.ResetDPSMeter(r.sess) {
		t.Fatal("the reset not taken")
	}
	at(2 * time.Second)
	pass(2 * time.Second)
	pass(30 * time.Second)
	if got := meterMsgs(t, r.sess); len(got) != 0 {
		t.Fatalf("the dropped fight was sent %+v", got)
	}
	hit(r.fsess, r.fsess.PlayerObjectID(), r.npcF, 5)
	pass(31 * time.Second)
	m = meterMsgs(t, r.sess)
	if len(m) != 1 || m[0].fight == old || m[0].total != 5 {
		t.Fatalf("after the reset: %+v", m)
	}
	if r.w.ResetDPSMeter(r.fsess) {
		t.Fatal("a reset taken from a client with no meter")
	}
	if !r.w.SetDPSMeter(r.sess, false) || r.w.DPSMeter(r.sess) || r.p.dps != nil {
		t.Fatal("off not taken")
	}
	hit(r.sess, r.me, r.npcO, 100)
	pass(32 * time.Second)
	if got := meterMsgs(t, r.sess); len(got) != 0 {
		t.Fatalf("off, sent %+v", got)
	}
}

// The body byte for byte, the most damage first; past DPSMeterLinesMost the
// receiver's own takes the last place, the count and the whole stay all of
// them; a name is cut to 23 bytes on a character's edge.
func TestDPSMeterOnTheWire(t *testing.T) {
	rows := []dpsRow{
		{id: 1, line: dpsLine{name: "Dab", class: 3, damage: 1000, pet: 0}, flags: 1},
		{id: 2, line: dpsLine{name: "Jib", class: 10, damage: 300, pet: 120}, flags: 2},
	}
	got := encodeDPSMeter(0x01020304, true, 1500*time.Millisecond, rows)
	want := []byte{1, 4, 3, 2, 1, 1, 0xDC, 5, 0, 0, 0x14, 5, 0, 0, 2, 0, 1, 0, 2,
		1, 3, 0xE8, 3, 0, 0, 0, 0, 0, 0, 3, 'D', 'a', 'b',
		2, 10, 0x2C, 1, 0, 0, 0x78, 0, 0, 0, 3, 'J', 'i', 'b'}
	if !bytes.Equal(got, want) {
		t.Fatalf("body % X\nwant % X", got, want)
	}
	var many []dpsRow
	for i := range 30 {
		many = append(many, dpsRow{id: EntityID(i + 1), line: dpsLine{name: fmt.Sprintf("M%02d", i), damage: int64(1000 - i)}})
	}
	many[27].flags = 1
	m := readMeterMsg(t, encodeDPSMeter(9, false, time.Second, many))
	if len(m.lines) != DPSMeterLinesMost || m.all != 30 || m.place != 28 || m.lines[DPSMeterLinesMost-1].name != "M27" || m.lines[DPSMeterLinesMost-2].name != "M22" {
		t.Fatalf("%d lines of %d, last %s", len(m.lines), m.all, m.lines[len(m.lines)-1].name)
	}
	if m.total != 30*1000-29*30/2 {
		t.Fatalf("the whole %d", m.total)
	}
	many[27].flags, many[3].flags = 0, 1
	m = readMeterMsg(t, encodeDPSMeter(9, false, time.Second, many))
	if m.lines[DPSMeterLinesMost-1].name != "M23" || m.place != 4 {
		t.Fatalf("its own among the top: the last is %s, want the 24th", m.lines[DPSMeterLinesMost-1].name)
	}
	if got := dpsName(strings.Repeat("é", 12)); got != strings.Repeat("é", 11) {
		t.Fatalf("cut %q", got)
	}
	if got := dpsName("Dabvantestwithaverylongname"); len(got) != 23 {
		t.Fatalf("cut %q", got)
	}
}

// A restart keeps the client's answer (its fight ends there: the next figure
// starts another); a takeover drops it, the new client answers for itself.
func TestDPSMeterKeptAndTakenOver(t *testing.T) {
	d := newDuelists(t)
	now := time.Now()
	meterClock(t, now)
	d.w.SetDPSMeter(d.sa, true)
	d.w.mu.Lock()
	d.w.dpsCountLocked(d.a, 50, false, now)
	d.w.mu.Unlock()
	k, ok := d.w.KeepPlayer(d.sa, now)
	if !ok || !k.DPSMeter {
		t.Fatalf("kept %+v", k)
	}
	if k2, _ := d.w.KeepPlayer(d.sb, now); k2.DPSMeter {
		t.Fatal("kept for a client that never asked")
	}
	d.a.dps = nil
	d.w.RestoreKeptPlayer(d.sa, k, now)
	if d.a.dps == nil || d.a.dps.live {
		t.Fatalf("restored %+v: on, no fight", d.a.dps)
	}
	d.a.CharServerID = 77
	s2 := d.w.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 21), Port: 7021}, 21, 21, 1, 2, true)
	s2.SetCharacterInWorld(true)
	s2.SetPlayerObjectID(0x1424A1E1)
	if p, _ := d.w.TakeOverPlayer(s2, 77); p != d.a || p.dps != nil {
		t.Fatalf("taken over with the old client's meter: %v", p.dps)
	}
}

// A duel's hits are the attacker's damage, halved as told, on its meter; its
// opponent's meter has none of them (a figure on it is another's).
func TestDPSMeterInADuel(t *testing.T) {
	d := newDuelists(t)
	meterClock(t, d.t0)
	duel := d.start(t)
	d.a.dps, d.b.dps = &dpsMeter{}, &dpsMeter{}
	d.w.mu.Lock()
	d.w.duelHurtLocked(duel, d.a, d.a.ServerID, d.b, 100, d.at(2*time.Second), numberMelee)
	d.w.mu.Unlock()
	if got, _, _ := line(d.a, d.a); got != int64(duelHalf(100)) {
		t.Errorf("the attacker's meter: %d, want the %d told", got, duelHalf(100))
	}
	if d.b.dps.live {
		t.Error("its opponent's meter counted a hit on it")
	}
}
