package entity

import (
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// Mr. Moogie's challenge (the GM Jibrish's petitions 1379 and 1380, by Jibcro): a rig of a database
// with Mr. Moogie, his metal twin's spawn-only row on its class template (the figures the data step
// gives him: 347% of a level 60's hit points, melee 750%, a 750 kick every 6 s, a 750 unresistable
// lifetap every 30 s, a 25% 500 lightning proc), Heirloom's balance rows for a level 60 (hit points
// 400%, melee 160%, an even con, a warrior's 110% at 60), and players beside him.

const (
	metalTapSpell  = 7101
	metalBody      = 0x01020304 // a stand-in body: the disc's is the data's
	metalTestSize  = 7.2
	metalLifetap   = "Gear Grinder"
	metalTestMelee = 750
)

type metalRig struct {
	t      *testing.T
	db     *database.SQLiteDB
	w      *World
	mgr    *session.Manager
	t0     time.Time
	moogie *NPC
	next   int
}

func newMetalRig(t *testing.T) *metalRig {
	t.Helper()
	db, err := database.NewSQLite(t.TempDir() + "/metal.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	// Mr. Moogie in town and his metal twin's original 30 off him, out of the world
	exec(`INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race)
		VALUES (700, 'Mr. Moogie', 0, 1000, 10, 1000, 0, 1213913426, 2.0, 80, 128, '0')`)
	exec(`INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race, npc_id)
		VALUES (701, 'Metal Moogie', 0, 1000, 10, 1030, 128, ?, ?, 60, 0, '0', 701)`, metalBody, metalTestSize)
	exec(`INSERT INTO npcspawnonly (name, note, source) VALUES ('metal moogie', 'test', 'test')`)
	exec(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast,
		eqprequire, spellname, spelldesc) VALUES (?, 60, 0, 0, 16880, 0, 0, 0, 0, 1, 30, 0, ?, '')`, metalTapSpell, metalLifetap)
	exec(`INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES (?, 'Target, Drain HP by 750 give self, Unresistable', 0, 'test')`, metalTapSpell)
	exec(`INSERT INTO classtemplates (name, class, hp_pct, taken_pct, melee_pct, proc_chance, proc_damage, proc_name, proc_resist,
		kick_damage, kick_every, kick_first, abilities, note, source)
		VALUES ('metal moogie', 0, 347, 100, ?, 25, 500, 'Static Discharge', 'LR', 750, 6, 6, ?, 'test', 'test')`,
		metalTestMelee, fmt.Sprintf(`[{"kind":"lifetap","spell_id":%d,"every":30,"first":30}]`, metalTapSpell))
	exec(`INSERT INTO npctemplate (name, template, note, source) VALUES ('metal moogie', 'metal moogie', 'test', 'test')`)
	r := &metalRig{t: t, db: db, t0: time.Unix(1_800_000_000, 0)}
	w := NewWorld()
	r.mgr = session.NewManager()
	w.SessionManager = r.mgr
	w.LoadNPCs(database.NewNPCStore(db))
	if _, _, err := w.LoadSpawnOnly(db); err != nil {
		t.Fatal(err)
	}
	if _, err := w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	for _, p := range w.ClassTemplateProblems() {
		if strings.Contains(p, "metal moogie") {
			t.Fatalf("his template's problem: %q", p) // the seeded templates' own, of spells this rig lacks, are not his
		}
	}
	w.SetBalance(NewBalance(
		[]database.BalanceLevel{{Level: 60, HPPct: 400, DamagePct: 160, ConPct: 100, PetHPPct: 100, PetDamagePct: 100, PlayerMeleePct: 100}},
		[]database.BalanceCon{{Diff: 0, DamagePct: 100, TakenPct: 100}}).
		WithClassLevels([]database.BalanceClassLevel{{Class: 0, Level: 60, MeleeTakenPct: 110, PetHPPct: 100, DealtPct: 100, MeleeDealtPct: 100}}))
	r.w = w
	r.moogie = w.NPCs[700]
	return r
}

func (r *metalRig) at(d time.Duration) time.Time { return r.t0.Add(d) }

// player puts a level 60 warrior in the world at (x, z) beside Mr. Moogie.
func (r *metalRig) player(name string, x, z float32) (*Player, *session.Session) {
	r.t.Helper()
	r.next++
	id := EntityID(1000 + r.next)
	sess := r.mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 9, byte(r.next)), Port: 7000 + r.next}, uint32(id), 1, 1, 2, true)
	p := &Player{ServerID: id, CharServerID: uint32(5000 + r.next), Name: name, Class: 0, Level: 60, HP: 5000, MaxHP: 5000,
		Speed: 8.833333, X: x, Y: 10, Z: z, Size: PlayerSize}
	r.w.Players[id] = p
	r.w.SpatialIndex.Insert(id, x, z)
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	sess.SetPlayerObjectID(0x14000000 + uint32(id))
	return p, sess
}

// see puts an entity on the session's next free slot under the rig's object id for it.
func (r *metalRig) see(sess *session.Session, id EntityID) uint32 {
	vs := GetOrCreateVisibility(sess)
	for i := range vs.ChannelEntity {
		if vs.ChannelEntity[i] == id {
			return vs.ChannelObjectID[i]
		}
		if vs.ChannelEntity[i] == 0 {
			vs.ChannelEntity[i], vs.ChannelObjectID[i] = id, 0x00200000+uint32(id)
			return vs.ChannelObjectID[i]
		}
	}
	r.t.Fatal("no slot")
	return 0
}

// challenge is the session's player asking Mr. Moogie, and the Metal Moogie standing after, his
// countdown run out (petition 1623): the rig's clock moved on by it, so d+0 after is the FIGHT, and the
// challenger's countdown lines drained. rise is the challenge alone.
func (r *metalRig) challenge(sess *session.Session, d time.Duration) (string, bool, *NPC) {
	r.t.Helper()
	line, ok := r.w.MetalMoogieChallenge(sess, r.moogie.ServerID, r.at(d))
	if ok {
		for s := 0; s <= MetalCountdown; s++ {
			r.w.mu.Lock()
			r.w.metalMoogieSystem(r.at(d + time.Duration(s)*time.Second))
			r.w.mu.Unlock()
		}
		r.t0 = r.t0.Add(MetalCountdown * time.Second)
		drain(sess)
	}
	return r.standing(line, ok)
}

// rise is the session's player asking Mr. Moogie, and the Metal Moogie standing after, held in his
// countdown.
func (r *metalRig) rise(sess *session.Session, d time.Duration) (string, bool, *NPC) {
	r.t.Helper()
	line, ok := r.w.MetalMoogieChallenge(sess, r.moogie.ServerID, r.at(d))
	return r.standing(line, ok)
}

// standing is the Metal Moogie standing, with the challenge's answer.
func (r *metalRig) standing(line string, ok bool) (string, bool, *NPC) {
	r.w.mu.RLock()
	defer r.w.mu.RUnlock()
	if r.w.metal == nil {
		return line, ok, nil
	}
	return line, ok, r.w.NPCs[r.w.metal.npc]
}

// tick runs the world's fights, casts landing, effects and Mr. Moogie's duel at d.
func (r *metalRig) tick(d time.Duration) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	now := r.at(d)
	r.w.aiSystem(now)
	r.w.templateSystem(now)
	r.w.effectSystem(now)
	r.w.metalMoogieSystem(now)
}

// A challenge stands one Metal Moogie up, his challenger's to fight: a copy of the spawn-only
// original at its spot (30 off Mr. Moogie), on his template (347% of a level 60's 14,400: 49,968 hit
// points, about Jibcro's 50k), attackable, going for the challenger at once. While he stands, another
// player's ask is refused with a line naming whom he duels, the challenger's own second ask too, and
// the challenger may not have Mr. Moogie's help (a heal mid-duel); the control: another player may.
func TestMetalMoogieRisesForOneChallengerAtATime(t *testing.T) {
	r := newMetalRig(t)
	a, sa := r.player("Anna", 1000, 1008)
	_, sb := r.player("Bert", 1004, 1008)
	if !r.w.MetalMoogieHere() {
		t.Fatal("no Metal Moogie to send: the spawn-only original is not seen")
	}
	line, ok, c := r.challenge(sa, 0)
	if !ok || c == nil || line != fmt.Sprintf(MetalLineRises, "Anna") {
		t.Fatalf("Anna's challenge: %q %v %v", line, ok, c)
	}
	if c.Name != MetalMoogieName || !c.Temporary || c.SpawnOnly || c.challenge == nil || c.challenge.player != a.ServerID ||
		c.NPCType&0x80 != 0 || c.CurrentEnemy != a.ServerID || c.Level != 60 || uint32(c.ModelID) != metalBody || c.Size != metalTestSize {
		t.Fatalf("the Metal Moogie: %+v", c)
	}
	if c.MaxHP != 49968 || c.HP != c.MaxHP || c.TemplateName() != "metal moogie" {
		t.Errorf("his hit points %d/%d on template %q, want 49,968 whole on metal moogie", c.HP, c.MaxHP, c.TemplateName())
	}
	if c.X != 1000 || c.Z != 1030 {
		t.Errorf("he rose at (%.1f, %.1f), not his original's spot (1000, 1030)", c.X, c.Z)
	}
	orig := r.w.NPCs[701]
	if orig == nil || !orig.SpawnOnly || orig.challenge != nil {
		t.Errorf("the original after: %+v", orig)
	}
	line, ok, again := r.challenge(sb, time.Second)
	if ok || again != c || line != fmt.Sprintf(MetalLineBusy, "Anna") {
		t.Errorf("Bert's ask while Anna duels: %q %v", line, ok)
	}
	if line, ok, _ := r.challenge(sa, 2*time.Second); ok || line != MetalLineMine {
		t.Errorf("Anna's second ask: %q %v", line, ok)
	}
	n := 0
	for _, x := range r.w.NPCs {
		if x.Name == MetalMoogieName && !x.SpawnOnly {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d Metal Moogies stand, want one", n)
	}
	if why := r.w.MoogieBusy(sa, r.moogie.ServerID); why != MetalLineDueling {
		t.Errorf("Anna asks Mr. Moogie mid-duel: %q", why)
	}
	if why := r.w.MoogieBusy(sb, r.moogie.ServerID); why != "" {
		t.Errorf("Bert asks Mr. Moogie (the control): %q", why)
	}
	if !r.w.MetalDuelOf(sa) || r.w.MetalDuelOf(sb) {
		t.Errorf("duels: Anna %v, Bert %v", r.w.MetalDuelOf(sa), r.w.MetalDuelOf(sb))
	}
}

// Without the spawn-only original (a server whose data has no Metal Moogie) Mr. Moogie sends no one.
func TestNoMetalMoogieWithoutHisOriginal(t *testing.T) {
	r := newMetalRig(t)
	_, sa := r.player("Anna", 1000, 1008)
	r.w.SetSpawnOnly(map[string]bool{})
	if r.w.MetalMoogieHere() {
		t.Error("a Metal Moogie to send with none spawn-only")
	}
	if line, ok, c := r.challenge(sa, 0); ok || c != nil || line != MetalLineAbsent {
		t.Errorf("a challenge with no original: %q %v %v", line, ok, c)
	}
}

// Only the challenger, and the challenger's pet, may hurt him, and he fights the challenger alone, as
// a duel's two are each other's: Bert's melee is not engaged (told in a line, once in MetalTellEvery),
// his harmful spell is refused (Anna's goes: the control), his area passes Metal Moogie by (Anna's
// takes him), his pet will not attack (Anna's would), hate from him is none (a hit's, a heal's on
// Anna), and every client but Anna's is shown the NPC bit (CLIENT swings at no such creature); Anna's
// own melee hit lands. He attacks no one on sight but Anna, no guard takes him on, and a GM trigger's
// sic passes him by.
func TestOnlyTheChallengerHurtsMetalMoogie(t *testing.T) {
	r := newMetalRig(t)
	a, sa := r.player("Anna", 1000, 1026)
	b, sb := r.player("Bert", 1004, 1026)
	_, _, c := r.challenge(sa, 0)
	if c == nil {
		t.Fatal("no Metal Moogie")
	}
	oa, ob := r.see(sa, c.ServerID), r.see(sb, c.ServerID)
	drain(sa)
	drain(sb)

	// melee
	if r.w.EngageAttack(sb, ob, Swing{Kind: 4, Delay: 6 * time.Second, Reach: 30}, r.at(time.Second)) {
		t.Error("Bert engaged Anna's Metal Moogie")
	}
	if b.AttackTarget != 0 {
		t.Errorf("Bert's attack target %d", b.AttackTarget)
	}
	if got := linesOf(drain(sb)); len(got) != 1 || got[0] != fmt.Sprintf(MetalLineNotYour, "Anna") {
		t.Errorf("Bert told: %q", got)
	}
	r.w.EngageAttack(sb, ob, Swing{Kind: 4, Delay: 6 * time.Second, Reach: 30}, r.at(2*time.Second))
	if got := linesOf(drain(sb)); len(got) != 0 {
		t.Errorf("Bert told again within %v: %q", MetalTellEvery, got)
	}
	if !r.w.EngageAttack(sa, oa, Swing{Kind: 4, Delay: 6 * time.Second, Reach: 30}, r.at(time.Second)) || a.AttackTarget != c.ServerID {
		t.Error("Anna (the control) did not engage her Metal Moogie")
	}
	r.w.mu.Lock()
	before := c.HP
	r.w.playerHitLocked(sa, a, c, oa)
	landed := before - c.HP
	r.w.mu.Unlock()
	if landed <= 0 {
		t.Errorf("Anna's hit took nothing off him (%d -> %d)", before, c.HP)
	}

	// spells
	nuke := CastSpell{SpellID: 9301, Name: "Test Bolt", Scope: 1, Range: 100, Def: ParseSpellText("Target, Decrease Hitpoints by 300")}
	charm := CastSpell{SpellID: 9302, Name: "Test Charm", Scope: 1, Range: 100, Def: ParseSpellText("Target, Charm, 5min")}
	r.w.mu.Lock()
	if ids, why := r.w.castTargetsLocked(sb, b, &nuke, ob); len(ids) != 0 || why != fmt.Sprintf(MetalLineNotYour, "Anna") {
		t.Errorf("Bert's nuke at him: %v %q", ids, why)
	}
	if ids, why := r.w.castTargetsLocked(sa, a, &nuke, oa); len(ids) != 1 || ids[0] != c.ServerID || why != "" {
		t.Errorf("Anna's nuke at him (the control): %v %q", ids, why)
	}
	if ids, why := r.w.castTargetsLocked(sa, a, &charm, oa); len(ids) != 0 || why != MetalLineCharm {
		t.Errorf("Anna's charm at him: %v %q", ids, why)
	}
	hp := c.HP
	r.w.landOnNPCLocked(&castState{spell: nuke, caster: b.ServerID, targets: []EntityID{c.ServerID}}, b, sb, c, r.at(3*time.Second))
	if c.HP != hp {
		t.Errorf("Bert's spell landed on him anyway: %d -> %d", hp, c.HP)
	}
	storm := CastSpell{SpellID: 9303, Name: "Test Storm", Scope: 1, Range: 100, Def: ParseSpellText("Target, Decrease Hitpoints by 100")}
	storm.Area.Centre = AreaOnTarget
	csB := &castState{spell: storm, caster: b.ServerID}
	if near := r.w.areaCreaturesLocked(b, csB, c.X, c.Y, c.Z, 50, r.at(3*time.Second)); len(near) != 0 {
		t.Errorf("Bert's area takes %v", near)
	}
	csA := &castState{spell: storm, caster: a.ServerID}
	if near := r.w.areaCreaturesLocked(a, csA, c.X, c.Y, c.Z, 50, r.at(3*time.Second)); len(near) != 1 {
		t.Errorf("Anna's area (the control) takes %v", near)
	}

	// pets
	petB := &NPC{ServerID: 9001, Name: "Bert's pet", Owner: b.ServerID, World: 0}
	petA := &NPC{ServerID: 9002, Name: "Anna's pet", Owner: a.ServerID, World: 0}
	if petMayFight(petB, c) || !petMayFight(petA, c) {
		t.Errorf("pets: Bert's may fight him %v, Anna's (the control) %v", petMayFight(petB, c), petMayFight(petA, c))
	}

	// hate
	r.w.addThreatLocked(c, b.ServerID, 100000)
	r.w.addThreatLocked(c, petA.ServerID, 100000)
	if _, ok := c.Threats[b.ServerID]; ok || c.CurrentEnemy != a.ServerID {
		t.Errorf("hate after Bert's: %v, enemy %d", c.Threats, c.CurrentEnemy)
	}
	a.HP = 100
	r.w.healHateLocked(b, a, 1000, 900)
	if _, ok := c.Threats[b.ServerID]; ok {
		t.Errorf("Bert's heal on Anna made him hate Bert: %v", c.Threats)
	}

	// on sight, guards, triggers
	if r.w.threatensLocked(c, b) || !r.w.threatensLocked(c, a) {
		t.Errorf("on sight: Bert %v, Anna %v", r.w.threatensLocked(c, b), r.w.threatensLocked(c, a))
	}
	guard := &NPC{ServerID: 9003, Name: "Guard", World: 0, X: 1000, Z: 1026, ModelID: 5}
	if r.w.guardsPreyLocked(guard, c, r.at(3*time.Second)) {
		t.Error("a guard would take him on")
	}

	// the view
	ba := r.w.serializeEntityForChannel(sa, c.ServerID, oa)
	bb := r.w.serializeEntityForChannel(sb, c.ServerID, ob)
	r.w.mu.Unlock()
	if ba[146]&0x80 != 0 || ba[143] != 1 {
		t.Errorf("Anna sees him with npc byte %#x, targetable %d: want a creature to fight", ba[146], ba[143])
	}
	if bb[146]&0x80 == 0 || bb[143] != 0 {
		t.Errorf("Bert sees him with npc byte %#x, targetable %d: want the NPC bit", bb[146], bb[143])
	}
	if name, ok := r.w.SetNPCOn(r.moogie.ServerID, MetalMoogieName, 100, false); ok {
		t.Errorf("a trigger's sic set Mr. Moogie on %q", name)
	}
	if line, ok := r.w.MetalMoogieTalk(c); !ok || line != fmt.Sprintf(MetalLineTalk, "Anna") {
		t.Errorf("a talk to him: %q %v", line, ok)
	}
}

// His figures on what Jibcro measured them by, a 2,000 AC warrior of the most AGI (#tankprofile2: AC
// 2000, AGI 400), level 60: he quads (a warrior's Attack4 from level 35), each hit near 300 (the
// template's 750% of the soft floor's, then the balance's 160% and the warrior's 110%).
func TestMetalMoogieHitsAWarriorNearThreeHundred(t *testing.T) {
	r := newMetalRig(t)
	a, sa := r.player("Anna", 1000, 1026)
	a.GMTankProfile = 2
	_, _, c := r.challenge(sa, 0)
	if c == nil {
		t.Fatal("no Metal Moogie")
	}
	if _, _, _, hits := npcRound(c); len(hits) != 4 {
		t.Errorf("he hits %d times a round, want a quad", len(hits))
	}
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	if a.StatNow(StatAC) != 2000 || a.StatNow(StatAgi) != 400 {
		t.Fatalf("the profile: AC %d, AGI %d", a.StatNow(StatAC), a.StatNow(StatAgi))
	}
	sum, lo, hi := 0, math.MaxInt, 0
	const n = 1000
	for i := 0; i < n; i++ {
		k := i
		hit := r.w.npcHitPlayerLocked(c, a, npcMeleeOn(c, a, func(m int) int { return k * m / n }))
		sum += hit
		lo, hi = min(lo, hit), max(hi, hit)
	}
	mean := float64(sum) / n
	if mean < 285 || mean > 315 || lo < 250 || hi > 350 {
		t.Errorf("his hits on the warrior: mean %.1f, %d-%d; want about 300", mean, lo, hi)
	}
	t.Logf("his hits on a 2,000 AC 400 AGI level 60 warrior: mean %.1f, %d-%d", mean, lo, hi)
}

// His kick comes every 6 s at his enemy for 750, fixed and through any armour; his lifetap every 30 s
// for 750, unresisted however high the player's resists, the same back on him; his proc is 500 on
// about a quarter of his melee hits, cut by the player's lightning resist. Over 62.5 s in reach of a
// player with hit points enough: ten kicks (at 6 to 60 s) and two taps (cast at 30 and 60 s, each
// landing castLand later).
func TestMetalMoogieKicksEverySixAndTapsEveryThirty(t *testing.T) {
	r := newMetalRig(t)
	a, sa := r.player("Anna", 1000, 1029)
	a.HP, a.MaxHP = 2_000_000, 2_000_000
	a.ArcaneR, a.LightningR = 5000, 0
	a.GMTankProfile = 3 // his melee hits far from 750 (about 260-300), so a 750 is a kick's or a tap's
	_, _, c := r.challenge(sa, 0)
	if c == nil {
		t.Fatal("no Metal Moogie")
	}
	oa := r.see(sa, c.ServerID)
	c.HP = c.MaxHP - 5000 // a tap has something to heal
	drain(sa)
	var dmg [][3]int64
	var lines []string
	for d := time.Duration(0); d <= 62500*time.Millisecond; d += 100 * time.Millisecond {
		r.tick(d)
		msgs := drain(sa)
		dmg = append(dmg, damageIndications(msgs)...)
		lines = append(lines, linesOf(msgs)...)
	}
	kicks, taps, heals := 0, 0, 0
	for _, l := range lines {
		if strings.HasPrefix(l, "Metal Moogie's "+metalLifetap+" hits you for 750") {
			taps++
		}
	}
	for _, x := range dmg {
		switch {
		case x[0] == int64(oa) && x[2] == int64(oa) && x[1] == 750:
			heals++
		case x[0] == int64(oa) && x[1] == -750:
			kicks++ // a tap is -750 too: counted off below
		}
	}
	kicks -= taps
	if kicks != 10 || taps != 2 || heals != 2 {
		t.Errorf("in 62.5 s: %d kicks, %d taps, %d heals of 750; want 10, 2, 2 (lines %q)", kicks, taps, heals, lines)
	}
	procs := 0
	for _, l := range lines {
		if strings.Contains(l, "Static Discharge") {
			procs++
		}
	}
	if procs < 2 || procs > 25 { // about 40 hits at 25%: 10, sd 2.7
		t.Errorf("%d Static Discharges in 62.5 s of melee, want about a quarter of some 40 hits (lines %q)", procs, lines)
	}
}

// Petition 1380: the hit the challenger cannot take is a duel's loss, as a duel's loser is left: 1 HP,
// no death, no debt. Metal Moogie leaves the world at once, and Mr. Moogie calls the challenger
// pathetic in /say ("Mr. Moogie: ..."), heard by those within MetalSayHeard of him and no one farther;
// a blow still on its way within a second kills no one. The control: his own death leaves his corpse
// lying, the duel won, nothing said in /say.
func TestMetalMoogieDefeatsHisChallenger(t *testing.T) {
	r := newMetalRig(t)
	a, sa := r.player("Anna", 1000, 1029)
	_, sc := r.player("Cora", 1100, 1000) // 100 off Mr. Moogie
	_, sd := r.player("Dora", 1400, 1000) // 400 off
	_, _, c := r.challenge(sa, 0)
	if c == nil {
		t.Fatal("no Metal Moogie")
	}
	debt := a.Debt
	drain(sa)
	drain(sc)
	drain(sd)
	r.w.mu.Lock()
	a.HP = 10
	r.w.templateStrikeLocked(c, a.ServerID, r.at(time.Second), 750, "Kick")
	r.w.mu.Unlock()
	if a.HP != 1 || a.IsDead() || a.respawnPending() || a.Debt != debt {
		t.Errorf("Anna after the blow she could not take: HP %d, dead %v, debt %d -> %d", a.HP, a.IsDead(), debt, a.Debt)
	}
	if _, ok := r.w.NPCs[c.ServerID]; ok || r.w.metal != nil {
		t.Errorf("Metal Moogie still stands after his win")
	}
	say := "Mr. Moogie: " + fmt.Sprintf(MetalSayPathetic, "Anna")
	la, lc, ld := linesOf(drain(sa)), linesOf(drain(sc)), linesOf(drain(sd))
	if !hasLine(la, say) || !hasLine(la, MetalLineLost) {
		t.Errorf("Anna heard %q", la)
	}
	if !hasLine(lc, say) {
		t.Errorf("Cora, 100 off him, heard %q", lc)
	}
	if len(ld) != 0 {
		t.Errorf("Dora, 400 off him, heard %q", ld)
	}
	r.w.mu.Lock()
	r.w.npcAttackPlayer(c, a.ServerID, r.at(1500*time.Millisecond)) // a blow on its way
	a.HP = 0
	r.w.playerDiedLocked(sa, a, r.at(1500*time.Millisecond))
	r.w.mu.Unlock()
	if a.IsDead() || a.HP != 1 {
		t.Errorf("a blow within a second of the loss: HP %d, dead %v", a.HP, a.IsDead())
	}
	r.w.mu.Lock()
	a.HP = 0
	r.w.playerDiedLocked(sa, a, r.at(5*time.Second))
	r.w.mu.Unlock()
	if !a.IsDead() {
		t.Error("a death after the duel (the control) did not kill")
	}

	// the control: his own death
	r2 := newMetalRig(t)
	a2, sa2 := r2.player("Anna", 1000, 1029)
	_, sc2 := r2.player("Cora", 1100, 1000)
	_, _, c2 := r2.challenge(sa2, 0)
	o2 := r2.see(sa2, c2.ServerID)
	drain(sa2)
	drain(sc2)
	r2.w.mu.Lock()
	c2.HP = 1
	r2.w.playerHitLocked(sa2, a2, c2, o2)
	r2.w.mu.Unlock()
	r2.tick(time.Second)
	if !c2.Dead || r2.w.NPCs[c2.ServerID] != c2 || r2.w.metal != nil {
		t.Errorf("after his death: dead %v, lies %v, duel %v", c2.Dead, r2.w.NPCs[c2.ServerID] == c2, r2.w.metal)
	}
	if got := linesOf(drain(sa2)); !hasLine(got, MetalLineWon) {
		t.Errorf("Anna told at her win: %q", got)
	}
	if got := linesOf(drain(sc2)); len(got) != 0 {
		t.Errorf("Cora heard at Anna's win: %q", got)
	}
	if a2.TotalXP != 0 || a2.XPInLevel != 0 {
		t.Errorf("Anna's win gave experience: %d", a2.TotalXP)
	}
	if duel, mine, _ := r2.w.ChallengeLooter(sa2, c2); !duel || !mine {
		t.Errorf("Anna's own corpse to loot: duel %v, hers %v", duel, mine)
	}
	if duel, mine, line := r2.w.ChallengeLooter(sc2, c2); !duel || mine || line != fmt.Sprintf(MetalLineLoot, "Anna") {
		t.Errorf("Cora at Anna's corpse: duel %v, hers %v, %q", duel, mine, line)
	}
	if !c2.DuelSpawn() {
		t.Error("his corpse is no duel's")
	}
}

// The duel ends, Metal Moogie gone and the challenger told why, when the challenger flees past
// MetalArenaRadius of the arena's centre, goes to another world, stays past MetalMaxTime, or leaves
// the world; and he takes his challenger up again when he let go (a feigned death).
func TestMetalMoogieDuelEnds(t *testing.T) {
	cases := []struct {
		name string
		do   func(r *metalRig, a *Player) time.Duration
		line string
	}{
		{"fled", func(r *metalRig, a *Player) time.Duration { a.Z += MetalArenaRadius + 20; return time.Second }, MetalLineFled},
		{"another world", func(r *metalRig, a *Player) time.Duration { a.World = 2; return time.Second }, MetalLineFled},
		{"time", func(r *metalRig, a *Player) time.Duration { return MetalMaxTime + time.Second }, MetalLineTime},
		{"left", func(r *metalRig, a *Player) time.Duration {
			r.w.mu.Lock()
			r.w.leavingLocked(a)
			delete(r.w.Players, a.ServerID)
			r.w.mu.Unlock()
			return time.Second
		}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newMetalRig(t)
			a, sa := r.player("Anna", 1000, 1020)
			_, _, c := r.challenge(sa, 0)
			if c == nil {
				t.Fatal("no Metal Moogie")
			}
			drain(sa)
			r.tick(tc.do(r, a))
			if _, ok := r.w.NPCs[c.ServerID]; ok || r.w.metal != nil {
				t.Errorf("he still stands")
			}
			got := linesOf(drain(sa))
			if tc.line != "" && !hasLine(got, tc.line) {
				t.Errorf("told %q, want %q", got, tc.line)
			}
			if line, ok, again := r.rise(sa, 2*time.Second); tc.line != "" && (!ok || again == nil) {
				t.Errorf("a new challenge after the end: %q %v", line, ok)
			}
		})
	}
	r := newMetalRig(t)
	a, sa := r.player("Anna", 1000, 1020)
	_, _, c := r.challenge(sa, 0)
	r.w.mu.Lock()
	r.w.dropAggroLocked(a.ServerID) // a feigned death
	r.w.mu.Unlock()
	if c.CurrentEnemy == a.ServerID {
		t.Fatal("he kept Anna through the drop")
	}
	r.tick(time.Second)
	if c.CurrentEnemy != a.ServerID {
		t.Errorf("he let Anna go: enemy %d", c.CurrentEnemy)
	}
}

// The Duelist's Favor: "Self, Increase Size by 50%, 60min" shows its bearer half again as big, on the
// state channel every client draws (bytes 36-39), for an hour; kept (Mr. Moogie's own "Clear my buffs"
// and a strip leave it on, as the Boon), over a logout too; and gone at its time or a death.
func TestDuelistsFavorGrowsHalfAgainForAnHour(t *testing.T) {
	d := ParseSpellNamed(DuelistsFavorName, "Self, Increase Size by 50%, 60min", "")
	if len(d.Effects) != 1 || d.Effects[0].Kind != EffectSize || d.Effects[0].Amount != 50 || d.Duration != time.Hour || d.Harmful {
		t.Fatalf("the line read: %+v", d)
	}
	if e := ParseSpellText("Target, Decrease Size by 20%, 5min").Effects; len(e) != 1 || e[0].Kind != EffectSize || e[0].Amount != -20 {
		t.Errorf("a shrink read: %+v", e)
	}
	favor := CastSpell{SpellID: 9401, Name: DuelistsFavorName, Scope: 0, Icon: 0x11223344, Def: d}
	r := newMetalRig(t)
	a, sa := r.player("Anna", 1000, 1008)
	size := func() float32 {
		r.w.mu.Lock()
		defer r.w.mu.Unlock()
		b := SerializePlayerUpdate(1, a)
		return math.Float32frombits(binary.LittleEndian.Uint32(b[36:40]))
	}
	if s := size(); s != PlayerSize {
		t.Fatalf("Anna's size before: %v", s)
	}
	if !r.w.GiveDuelistsFavor(sa, favor, r.at(0)) {
		t.Fatal("the favor did not land")
	}
	if s := size(); math.Abs(float64(s-1.5*PlayerSize)) > 1e-6 {
		t.Errorf("Anna's size with the favor: %v, want %v", s, 1.5*PlayerSize)
	}
	ae := a.Effects[len(a.Effects)-1]
	if ae.Name != DuelistsFavorName || !ae.Rules.Kept || ae.Rules.Grow != 50 || !ae.Until.Equal(r.at(time.Hour)) {
		t.Errorf("the effect: %+v", ae)
	}
	if _, n, _ := r.w.MoogieClear(sa); n != 0 || !a.hasEffect(9401) {
		t.Errorf("Mr. Moogie's clear took %d, the favor on %v", n, a.hasEffect(9401))
	}
	r.w.mu.Lock()
	r.w.stripPlayerLocked(a, r.at(time.Minute))
	r.w.mu.Unlock()
	if !a.hasEffect(9401) {
		t.Error("a strip took the favor")
	}
	// over a logout
	rows, _ := r.w.SavedEffects(a.ServerID, r.at(10*time.Minute))
	a.Effects, a.Buffs = nil, nil
	if s := size(); s != PlayerSize {
		t.Fatalf("Anna's size with the effects off: %v", s)
	}
	if n := r.w.RestoreEffects(sa, rows, r.at(10*time.Minute)); n != 1 {
		t.Fatalf("%d effects back", n)
	}
	if s := size(); math.Abs(float64(s-1.5*PlayerSize)) > 1e-6 || !a.Effects[0].Rules.Kept {
		t.Errorf("after a logout: size %v, kept %v", s, a.Effects[0].Rules.Kept)
	}
	// its hour (50 minutes left at the logout's 10)
	r.tick(10*time.Minute + 50*time.Minute + time.Second)
	if a.hasEffect(9401) || size() != PlayerSize {
		t.Errorf("after its hour: on %v, size %v", a.hasEffect(9401), size())
	}
	// a death
	r.w.GiveDuelistsFavor(sa, favor, r.at(2*time.Hour))
	r.w.mu.Lock()
	a.HP = 0
	r.w.playerDiedLocked(sa, a, r.at(2*time.Hour+time.Minute))
	r.w.mu.Unlock()
	if a.hasEffect(9401) || size() != PlayerSize {
		t.Errorf("after a death: on %v, size %v", a.hasEffect(9401), size())
	}
}
