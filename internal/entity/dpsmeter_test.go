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

// fightOf is the fight p's meter is shown (its group's, alone its own) when it
// is under way, else nil. It takes no lock: a test calls it from its one
// goroutine, the lock held or not.
func fightOf(w *World, p *Player) *dpsFight {
	if f := *w.dpsScopeLocked(p); f != nil && f.live {
		return f
	}
	return nil
}

// line is the line of maker in the fight p's meter is shown: its damage and its
// pet's share; ok false when it has none (or p has no meter).
func line(w *World, p, maker *Player) (damage, pet int64, ok bool) {
	f := fightOf(w, p)
	if p.dps == nil || f == nil {
		return 0, 0, false
	}
	l := f.lines[maker.ServerID]
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
				got, gotPet, _ := line(r.w, v, r.p)
				if got != want || gotPet != wantPet {
					t.Errorf("%s's meter: %d (pet %d) of the enchanter's, want %d (pet %d) of %v", v.Name, got, gotPet, want, wantPet, told)
				}
			}
			if !c.counts && (fightOf(r.w, r.p) != nil || fightOf(r.w, r.friend) != nil) {
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
	if fightOf(r.w, r.friend) != nil {
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
	got, gotPet, ok := line(r.w, r.friend, r.p)
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
		got, pet, _ := line(r.w, r.p, r.p)
		if taken <= 0 || got != taken || pet != taken {
			t.Errorf("%s: the meter has %d (pet %d), the creature took %d", kind, got, pet, taken)
		}
		if (kind == petKindOurs) != (len(told) == 1) {
			t.Errorf("%s: told %v", kind, told)
		}
	}
}

// sameFight is whether two members' messages are the same fight as sent: the
// same number, state, length, whole, count and lines (names, classes, damage,
// pets, flag 2) but for each receiver's own flag 1 and place.
func sameFight(a, b meterMsg) bool {
	strip := func(m meterMsg) string {
		ls := append([]meterMsgLine(nil), m.lines...)
		for i := range ls {
			ls[i].flags &^= 1
		}
		m.lines, m.place = ls, 0
		return fmt.Sprintf("%+v", m)
	}
	return strip(a) == strip(b)
}

// ownFlags is the lines of m whose flag 1 (the receiver's own) is set, by name.
func ownFlags(m meterMsg) (out []string) {
	for _, l := range m.lines {
		if l.flags&1 != 0 {
			out = append(out, l.name)
		}
	}
	return out
}

// The scope is the group at each figure and the fight is the group's (the owner,
// 2026-11-11: "drunkenfist came in the raid late and the dps meter was not the
// same as other players"): a solo player's meter has its own fight; a group's
// every member is shown one fight; one who joins mid-fight is shown it under way
// at once, the same fight to the byte as the others but for its own line's flag
// and place; one who leaves counts no more on it, its line kept, flagged 2, its
// own view of the group's fight ends (over) and it goes on to its own fight.
// Figures of a player in no group reach no one else's meter (the control).
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
	pass := func(d time.Duration) {
		r.w.mu.Lock()
		r.w.TickCount = 0
		r.w.dpsMeterSystem(r.t0.Add(d))
		r.w.mu.Unlock()
	}
	// alone: each its own
	myHit(100)
	friendHit(40)
	if d, _, _ := line(r.w, r.friend, r.p); d != 0 {
		t.Fatalf("the friend's meter has %d of the enchanter's, in no group", d)
	}
	if d, _, _ := line(r.w, r.p, r.p); d != 100 {
		t.Fatalf("the enchanter's own: %d", d)
	}
	pass(DPSMeterIdle) // the solo fights end
	for _, s := range []*session.Session{r.sess, r.fsess, tsess} {
		meterMsgs(t, s)
	}
	// a group: one fight, each counts on it
	g := group(r.w, 77, r.p, r.friend)
	at(20 * time.Second)
	myHit(100)
	friendHit(40)
	for _, v := range []*Player{r.p, r.friend} {
		mine, _, _ := line(r.w, v, r.p)
		theirs, _, _ := line(r.w, v, r.friend)
		if mine != 100 || theirs != 40 {
			t.Errorf("%s's meter: %d and %d, want 100 and 40", v.Name, mine, theirs)
		}
	}
	if fightOf(r.w, third) != nil {
		t.Fatal("the third's meter counted a group it is not in")
	}
	pass(20 * time.Second)
	gm, fm := meterMsgs(t, r.sess), meterMsgs(t, r.fsess)
	if len(gm) != 1 || len(fm) != 1 || !sameFight(gm[0], fm[0]) || gm[0].total != 140 {
		t.Fatalf("the group's two were sent %+v and %+v", gm, fm)
	}
	if got := meterMsgs(t, tsess); len(got) != 0 {
		t.Fatalf("the third, in no group, was sent %+v", got)
	}
	// the third joins late: shown the group's fight under way at once, all of
	// it, the same as the others are
	g.Members, third.GroupID = append(g.Members, third.ServerID), g.ID
	at(22 * time.Second)
	myHit(50)
	pass(22 * time.Second)
	em, tm := meterMsgs(t, r.sess), meterMsgs(t, tsess)
	if fm := meterMsgs(t, r.fsess); len(fm) != 1 || !sameFight(fm[0], tm[0]) {
		t.Fatalf("at the late join the friend was sent %+v", fm)
	}
	if len(em) != 1 || len(tm) != 1 {
		t.Fatalf("at the late join: the enchanter %+v, the third %+v", em, tm)
	}
	if !sameFight(em[0], tm[0]) || tm[0].fight != gm[0].fight || tm[0].lengthMS != 2000 || tm[0].total != 190 || tm[0].all != 3 {
		t.Fatalf("the late joiner %+v, the enchanter %+v", tm[0], em[0])
	}
	if fmt.Sprint(ownFlags(em[0])) != "[Druid]" || fmt.Sprint(ownFlags(tm[0])) != "[Healer]" || tm[0].place != 3 || em[0].place != 1 {
		t.Fatalf("own lines %v (place %d) and %v (place %d)", ownFlags(em[0]), em[0].place, ownFlags(tm[0]), tm[0].place)
	}
	if d, _, _ := line(r.w, third, r.p); d != 150 {
		t.Errorf("the third's meter: %d of the enchanter's, want the whole fight's 150", d)
	}
	// the friend leaves: no more on the group's fight, its view of it over, its
	// next figure its own fight
	r.w.mu.Lock()
	r.w.leaveGroupLocked(r.friend)
	r.w.mu.Unlock()
	at(23 * time.Second)
	friendHit(60)
	myHit(10)
	if d, _, _ := line(r.w, r.p, r.friend); d != 40 {
		t.Errorf("the enchanter's meter kept %d of the friend's, want the 40 before it left", d)
	}
	if d, _, _ := line(r.w, r.friend, r.p); d != 0 {
		t.Errorf("the friend's own fight has %d of the enchanter's", d)
	}
	if d, _, _ := line(r.w, r.friend, r.friend); d != 60 {
		t.Errorf("the friend's own: %d", d)
	}
	pass(23 * time.Second)
	msgs, tm := meterMsgs(t, r.sess), meterMsgs(t, tsess)
	if len(msgs) != 1 || len(tm) != 1 || !sameFight(msgs[0], tm[0]) {
		t.Fatalf("the enchanter was sent %+v, the third %+v", msgs, tm)
	}
	got := fmt.Sprint(msgs[0].lines)
	want := fmt.Sprint([]meterMsgLine{{flags: 1, class: classEnchanter, damage: 160, name: "Druid"}, {flags: 2, damage: 40, name: "Tank"}, {class: 9, name: "Healer"}})
	if got != want || msgs[0].all != 3 || msgs[0].total != 200 || msgs[0].place != 1 {
		t.Errorf("the enchanter's lines %s (%d in all, %d), want %s", got, msgs[0].all, msgs[0].total, want)
	}
	fm = meterMsgs(t, r.fsess)
	if len(fm) != 2 || !fm[0].over || fm[0].fight != gm[0].fight || fm[0].total != 200 || fmt.Sprint(ownFlags(fm[0])) != "[Tank]" ||
		fm[1].over || fm[1].fight == gm[0].fight || fm[1].total != 60 || len(fm[1].lines) != 1 {
		t.Fatalf("the leaver was sent %+v", fm)
	}
	if fm[0].lines[1].flags != 3 {
		t.Errorf("the leaver's own line in the group's last view: flags %d, want 3", fm[0].lines[1].flags)
	}
}

// A raid group is one group of more than four (raid.go): its every member, of
// whatever part of it, is shown one fight; a group disbanded mid-fight ends its
// members' views, and each one's next figure starts its own.
func TestDPSMeterRaidOneFightAndDisband(t *testing.T) {
	r := newTagRig(t)
	at := meterClock(t, r.t0)
	ps := []*Player{r.p, r.friend}
	for i := range 5 {
		o := &Player{ServerID: EntityID(2001 + i), Name: fmt.Sprintf("Raider%d", i), Class: 0, Level: 20, HP: 700, MaxHP: 700}
		r.w.Players[o.ServerID] = o
		ps = append(ps, o)
	}
	g := group(r.w, 88, ps...)
	if !g.isRaid() {
		t.Fatal("seven are no raid group")
	}
	r.p.dps, r.friend.dps = &dpsMeter{}, &dpsMeter{}
	pass := func(d time.Duration) {
		r.w.mu.Lock()
		r.w.TickCount = 0
		r.w.dpsMeterSystem(r.t0.Add(d))
		r.w.mu.Unlock()
	}
	r.w.mu.Lock()
	r.w.sendDamageIndication(r.sess, r.me, -100, r.npcO, numberSpell)
	r.w.dpsCountLocked(ps[5], 70, false, r.t0)
	r.w.mu.Unlock()
	at(time.Second)
	r.w.mu.Lock()
	r.w.sendDamageIndication(r.fsess, r.fsess.PlayerObjectID(), -30, r.npcF, numberMelee)
	r.w.mu.Unlock()
	pass(time.Second)
	a, b := meterMsgs(t, r.sess), meterMsgs(t, r.fsess)
	if len(a) != 1 || len(b) != 1 || !sameFight(a[0], b[0]) || a[0].all != 7 || a[0].total != 200 || a[0].lengthMS != 1000 {
		t.Fatalf("the raid's two were sent %+v and %+v", a, b)
	}
	// disbanded: both views end, over; the next figures each its own fight
	r.w.mu.Lock()
	for _, p := range ps {
		p.GroupID = 0
	}
	delete(r.w.groups, g.ID)
	r.w.mu.Unlock()
	at(2 * time.Second)
	r.w.mu.Lock()
	r.w.sendDamageIndication(r.sess, r.me, -5, r.npcO, numberSpell)
	r.w.mu.Unlock()
	pass(2 * time.Second)
	a, b = meterMsgs(t, r.sess), meterMsgs(t, r.fsess)
	if len(a) != 2 || !a[0].over || a[0].fight != b[0].fight || a[1].over || a[1].fight == a[0].fight || a[1].total != 5 {
		t.Fatalf("the enchanter after the disband: %+v", a)
	}
	if len(b) != 1 || !b[0].over || b[0].total != 200 {
		t.Fatalf("the friend after the disband: %+v", b)
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
	// the friend's meter is its own: one member's reset leaves the group's
	// fight going on the others'
	r.w.SetDPSMeter(r.fsess, true)
	at(2 * time.Second)
	hit(r.fsess, r.fsess.PlayerObjectID(), r.npcF, 20)
	pass(2 * time.Second)
	if f := meterMsgs(t, r.fsess); len(f) != 1 || f[0].fight != old || f[0].total != 200 || f[0].over {
		t.Fatalf("the friend after the other's reset: %+v", f)
	}
	pass(30 * time.Second)
	if got := meterMsgs(t, r.sess); len(got) != 0 {
		t.Fatalf("the dropped fight was sent %+v", got)
	}
	if f := meterMsgs(t, r.fsess); len(f) != 1 || f[0].fight != old || !f[0].over {
		t.Fatalf("the friend's end of the fight: %+v", f)
	}
	r.w.SetDPSMeter(r.fsess, false)
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
	if d.a.dps == nil || d.a.dps.view != nil || d.a.dps.shown != 0 {
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
	if got, _, _ := line(d.w, d.a, d.a); got != int64(duelHalf(100)) {
		t.Errorf("the attacker's meter: %d, want the %d told", got, duelHalf(100))
	}
	if fightOf(d.w, d.b) != nil {
		t.Error("its opponent's meter counted a hit on it")
	}
}

// A quiet stretch inside a fight is the same fight (Lord Nagafen, 2026-11-11:
// the caster dead, the tanks only taunting, 11 s of no damage while he fought
// on): a creature with the player, or its pet, on its hate list holds the fight
// open past DPSMeterIdle, up to DPSMeterHeld after the last figure; with the hate
// gone it ends at DPSMeterIdle as before.
func TestDPSMeterHeldWhileEngaged(t *testing.T) {
	for _, c := range []struct {
		name string
		hate func(r *tagRig) EntityID
	}{
		{"the player on its hate list", func(r *tagRig) EntityID { return r.p.ServerID }},
		{"its pet on its hate list", func(r *tagRig) EntityID { return r.pet.ServerID }},
	} {
		t.Run(c.name, func(t *testing.T) {
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
			at(0)
			r.w.mu.Lock()
			r.w.sendDamageIndication(r.sess, r.me, -100, r.npcO, numberMelee)
			r.npc.Threats = map[EntityID]int64{c.hate(r): 10}
			r.w.wakeNPCLocked(r.npc) // as every change of an NPC's hate does
			r.w.mu.Unlock()
			first := pass(500 * time.Millisecond)
			if len(first) != 1 || first[0].over {
				t.Fatalf("the first pass sent %+v", first)
			}
			for _, d := range []time.Duration{DPSMeterIdle, DPSMeterIdle + 5*time.Second, DPSMeterHeld - time.Second} {
				for _, m := range pass(d) {
					if m.over {
						t.Fatalf("ended at %v while engaged: %+v", d, m)
					}
				}
			}
			// a figure in the quiet stretch: the same fight, its length to it
			at(20 * time.Second)
			r.w.mu.Lock()
			r.w.sendDamageIndication(r.sess, r.me, -50, r.npcO, numberMelee)
			r.w.mu.Unlock()
			m := pass(20*time.Second + 500*time.Millisecond)
			if len(m) != 1 || m[0].over || m[0].fight != first[0].fight || m[0].lengthMS != 20000 || m[0].lines[0].damage != 150 {
				t.Fatalf("a figure after the quiet: %+v", m)
			}
			m = pass(20*time.Second + DPSMeterHeld)
			if len(m) != 1 || !m[0].over || m[0].fight != first[0].fight {
				t.Fatalf("held past DPSMeterHeld: %+v", m)
			}
			// hate gone: ends at DPSMeterIdle
			at(200 * time.Second)
			r.w.mu.Lock()
			r.w.sendDamageIndication(r.sess, r.me, -7, r.npcO, numberMelee)
			r.npc.Threats = nil
			r.w.mu.Unlock()
			pass(200*time.Second + 500*time.Millisecond)
			m = pass(200*time.Second + DPSMeterIdle)
			if len(m) != 1 || !m[0].over {
				t.Fatalf("no hate, not ended at DPSMeterIdle: %+v", m)
			}
		})
	}
}
