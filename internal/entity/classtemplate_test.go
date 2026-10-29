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
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// NPC class templates (classtemplate.go) over a real database: the seeded
// templates, the spells their abilities pick from, npctemplate rows a GM
// writes, and a world with the NPCs loaded from it, fighting players.

const (
	fxLifeTap   = 0x0A0A0A01
	fxSiphon    = 0x0A0A0A02
	fxFireStorm = 0x0A0A0A03
	fxGaze      = 0x0A0A0A04
	fxExplode   = 0x0A0A0A05
	fxPhoenix   = 0x0A0A0A06
	fxShock     = 0x0A0A0A07
	fxHealing   = 0x0A0A0A08
	fxFireBolt  = 0x0A0A0A09
)

type templateRig struct {
	t     *testing.T
	db    *database.SQLiteDB
	w     *World
	mgr   *session.Manager
	t0    time.Time
	npcID map[string]EntityID
	next  uint32
}

func newTemplateRig(t *testing.T) *templateRig {
	t.Helper()
	db, err := database.NewSQLite(t.TempDir() + "/templates.db")
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
	// the game's own spells, as this server keeps them (spellpattern's cast
	// and recast in seconds, range 16880: 30)
	type sp struct {
		id, level, cast, recast int
		name, line              string
		fx                      uint32
		class                   int
	}
	for _, s := range []sp{
		{880, 3, 2, 2, "Life Tap", "Target, Decrease Drain HP by 10", fxLifeTap, 11},
		{903, 16, 2, 3, "Siphon Life", "Target, Drain HP by 30 and gives to self", fxSiphon, 11},
		{1638, 18, 2, 3, "Plague Bolt", "Target, Decrease Hitpoints by 90", 0, 11}, // a nuke: no tap
		{700145, 16, 2, 3, "Fire Storm", "Target, Decrease Hitpoints by 82, aoe Decrease Hitpoints by 36", fxFireStorm, 13},
		{700138, 16, 2, 3, "Burning Gaze", "Target, Decrease Hitpoints by 134", fxGaze, 13},
		{700096, 12, 2, 180, "Explode", "Target, Decrease Hitpoints by 186", fxExplode, 13},
		{700668, 15, 15, 0, "Phoenix Glory", "Target, Decrease Hitpoints by 5000 aoe Decrease Hitpoints by 1998", fxPhoenix, 13},
		{700081, 8, 2, 3, "Static Shock", "Target, Decrease Hitpoints by 104", fxShock, 13},
		{1100, 10, 2, 3, "Healing", "Target, Increase Hitpoints by 125", fxHealing, 9},
		{1632, 4, 2, 3, "Fire Bolt", "Target Debuff Hp by 100", fxFireBolt, 13},
	} {
		exec(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast,
			eqprequire, spellname, spelldesc) VALUES (?, ?, 0, 0, 16880, ?, 10, 0, 0, 1, ?, 0, ?, '')`, s.id, s.level, s.cast, s.recast, s.name)
		exec(`INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES (?, ?, ?, 'test')`, s.id, s.line, int64(s.fx))
		exec(`INSERT INTO classspells (class, spellid, source) VALUES (?, ?, 'dump')`, s.class, s.id)
	}
	// the creatures: a template's, and controls of no template
	r := &templateRig{t: t, db: db, t0: time.Unix(1_800_000_000, 0), npcID: map[string]EntityID{}, next: 0x00100001}
	for i, n := range []struct {
		name  string
		level int
		x     float32
	}{
		{"Gnoll Necro", 20, 100}, {"Gnoll Pup", 20, 103}, {"Gnoll Mage", 20, 1000}, {"Gnoll Brute", 34, 2000},
		{"Gnoll Guard", 30, 3000}, {"Gnoll Priest", 20, 4000}, {"Gnoll Pyro", 20, 5000},
	} {
		id := 500 + i
		exec(`INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race)
			VALUES (?, ?, 0, ?, 10, 100, 0, 1893243078, 1.0, ?, 0, '0')`, id, n.name, n.x, n.level)
		r.npcID[n.name] = EntityID(id)
	}
	w := NewWorld()
	r.mgr = session.NewManager()
	w.SessionManager = r.mgr
	w.LoadNPCs(database.NewNPCStore(db))
	r.w = w
	return r
}

func (r *templateRig) at(d time.Duration) time.Time { return r.t0.Add(d) }

func (r *templateRig) npc(name string) *NPC { return r.w.NPCs[r.npcID[name]] }

// assign writes an npctemplate row and has the world read the tables again,
// as the GM panel's edit does.
func (r *templateRig) assign(name, template string) int {
	r.t.Helper()
	if template == "" {
		if _, err := r.db.Exec(`DELETE FROM npctemplate WHERE name = ?`, name); err != nil {
			r.t.Fatal(err)
		}
	} else if _, err := r.db.Exec(`INSERT OR REPLACE INTO npctemplate (name, template) VALUES (?, ?)`, name, template); err != nil {
		r.t.Fatal(err)
	}
	n, err := r.w.LoadClassTemplates(r.db)
	if err != nil {
		r.t.Fatal(err)
	}
	return n
}

// player puts a player in the world beside (x, z), seeing every NPC.
func (r *templateRig) player(name string, x, z float32) (*Player, *session.Session) {
	r.t.Helper()
	id := EntityID(1001 + len(r.w.Players))
	sess := r.mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 1, byte(id)), Port: 7000}, uint32(id), 1, 1, 2, true)
	p := &Player{ServerID: id, Name: name, Class: 0, Level: 20, HP: 100000, MaxHP: 100000, Speed: 8.833333, X: x, Y: 10, Z: z}
	r.w.Players[id] = p
	r.w.SpatialIndex.Insert(id, x, z)
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	sess.SetPlayerObjectID(0x14000000 + uint32(id))
	vs := GetOrCreateVisibility(sess)
	i := 0
	for _, nid := range r.npcID {
		vs.ChannelEntity[i] = nid
		vs.ChannelObjectID[i] = r.objectID(nid)
		i++
	}
	for _, other := range r.w.Players {
		if other != p {
			vs.ChannelEntity[i] = other.ServerID
			vs.ChannelObjectID[i] = r.objectID(other.ServerID)
			i++
		}
	}
	return p, sess
}

// objectID is the id every session here knows an entity by.
func (r *templateRig) objectID(id EntityID) uint32 { return 0x00200000 + uint32(id) }

// fight sets an NPC on a player (or a pet), as a hit would.
func (r *templateRig) fight(n *NPC, foe EntityID) {
	n.Threats = map[EntityID]int64{foe: 1}
	n.CurrentEnemy = foe
}

// tick runs the fights and the templates' landings at d.
func (r *templateRig) tick(d time.Duration) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.aiSystem(r.at(d))
	r.w.templateSystem(r.at(d))
}

// b1 is a 0x00B1: its caster, target, others, cast time and visual.
type b1 struct {
	caster, target uint32
	others         []uint32
	ms, fx         uint32
}

func castsIn(msgs [][]byte) []b1 {
	var out []b1
	for _, m := range msgs {
		if opOfMsg(m) != protocol.OpcodeCastingCombat {
			continue
		}
		c := b1{caster: binary.LittleEndian.Uint32(m[2:]), target: binary.LittleEndian.Uint32(m[6:])}
		n := int(m[10])
		for i := 0; i < n; i++ {
			c.others = append(c.others, binary.LittleEndian.Uint32(m[11+4*i:]))
		}
		c.ms = binary.LittleEndian.Uint32(m[11+4*n:])
		c.fx = binary.LittleEndian.Uint32(m[15+4*n:])
		out = append(out, c)
	}
	return out
}

// A necromancer's template on a level 20 creature: its hit points are the
// template's 90% of its level's, it is a necromancer, and in its fight it
// taps the player with the highest necromancer life tap at or below its level
// (Siphon Life, 16: not Life Tap, 3, nor Plague Bolt, a nuke) every 5 s from
// 1 s in, its melee held while it casts: the 0x00B1 from it with the tap's
// cast time and visual, the hit (half the line's 30) in a 0x00DB and a line
// to the player, and as much back on the creature. The control fights beside
// it: a creature of no template keeps its level's hit points and casts
// nothing. Taken away again, the template's numbers go with it.
func TestAClassTemplateChangesStatsAndCastsOnItsCadence(t *testing.T) {
	r := newTemplateRig(t)
	necro, pup := r.npc("Gnoll Necro"), r.npc("Gnoll Pup")
	if necro.MaxHP != NPCMaxHP(20) || necro.Class != 0 || necro.TemplateName() != "" {
		t.Fatalf("before any assignment: %d HP, class %d, template %q", necro.MaxHP, necro.Class, necro.TemplateName())
	}
	if n := r.assign("gnoll necro", "necromancer"); n != 1 {
		t.Fatalf("%d NPCs by a template, want the one", n)
	}
	if necro.TemplateName() != "necromancer" || necro.Class != classNecromancer || necro.MaxHP != NPCMaxHP(20)*90/100 || necro.HP != necro.MaxHP {
		t.Errorf("the necromancer: template %q, class %d, %d/%d HP; want class 11 and %d", necro.TemplateName(), necro.Class,
			necro.HP, necro.MaxHP, NPCMaxHP(20)*90/100)
	}
	if pup.TemplateName() != "" || pup.MaxHP != NPCMaxHP(20) || pup.Class != 0 {
		t.Errorf("the control changed: %q %d %d", pup.TemplateName(), pup.MaxHP, pup.Class)
	}

	p, sess := r.player("Hero", 101.5, 100)
	r.fight(necro, p.ServerID)
	r.fight(pup, p.ServerID)
	necro.HP = necro.MaxHP / 2
	hurt := necro.HP
	var taps []time.Duration
	var pupCasts int
	var lines []string
	var heals int
	for d := time.Duration(0); d <= 20*time.Second; d += 100 * time.Millisecond {
		r.tick(d)
		msgs := drain(sess)
		for _, c := range castsIn(msgs) {
			switch c.caster {
			case r.objectID(necro.ServerID):
				if c.fx == fxSiphon {
					if c.ms != 2000 || c.target != sess.PlayerObjectID() {
						t.Errorf("the tap's 0x00B1: %+v", c)
					}
					taps = append(taps, d)
				} else if c.fx == fxLifeTap || c.fx == fxDamage {
					t.Errorf("cast %#x at %v: not its level's tap", c.fx, d)
				}
			case r.objectID(pup.ServerID):
				if c.ms != 4000 { // the control's swings alone (Attack2)
					pupCasts++
				}
			}
		}
		for _, f := range damageIndications(msgs) {
			if f[0] == int64(r.objectID(necro.ServerID)) && f[2] == int64(r.objectID(necro.ServerID)) && f[1] == 15 {
				heals++
			}
		}
		for _, l := range linesOf(msgs) {
			lines = append(lines, l)
		}
	}
	want := []time.Duration{time.Second, 6 * time.Second, 11 * time.Second, 16 * time.Second}
	if fmt.Sprint(taps) != fmt.Sprint(want) {
		t.Errorf("taps at %v, want %v (every 5 s from 1 s in)", taps, want)
	}
	if pupCasts != 0 {
		t.Errorf("the control cast %d times", pupCasts)
	}
	tapLines := 0
	for _, l := range lines {
		switch {
		case l == "Gnoll Necro's Siphon Life hits you for 15 (your resist took 0%).":
			tapLines++
		case strings.Contains(l, "Gnoll Pup"):
			t.Errorf("a line of the control's: %q", l)
		}
	}
	if tapLines != 4 || heals != 4 || necro.HP != hurt+60 {
		t.Errorf("%d tap lines, %d heals told, HP %d (from %d); want 4, 4 and +60: %q", tapLines, heals, necro.HP, hurt, lines)
	}

	// taken away: its own numbers back, and no more casts
	r.assign("gnoll necro", "")
	if necro.TemplateName() != "" || necro.Class != 0 || necro.MaxHP != NPCMaxHP(20) {
		t.Errorf("after the row's delete: %q, class %d, max %d", necro.TemplateName(), necro.Class, necro.MaxHP)
	}
	for d := 21 * time.Second; d <= 30*time.Second; d += 100 * time.Millisecond {
		r.tick(d)
		for _, c := range castsIn(drain(sess)) {
			if c.caster == r.objectID(necro.ServerID) && c.fx == fxSiphon {
				t.Fatalf("a tap at %v after the template went", d)
			}
		}
	}
}

// With npctemplate empty, as the table is made, no NPC fights by a template:
// every one keeps its level's hit points and its class, the seeded templates
// all there to be given.
func TestNoAssignmentChangesNothing(t *testing.T) {
	r := newTemplateRig(t)
	before := map[EntityID][2]int{}
	for id, n := range r.w.NPCs {
		before[id] = [2]int{n.MaxHP, n.Class}
	}
	n, err := r.w.LoadClassTemplates(r.db)
	if err != nil || n != 0 {
		t.Fatalf("%d NPCs by a template (%v), want none", n, err)
	}
	for id, npc := range r.w.NPCs {
		if b := before[id]; b != [2]int{npc.MaxHP, npc.Class} || npc.tmpl != nil {
			t.Errorf("%s changed: %v -> %d %d %v", npc.Name, b, npc.MaxHP, npc.Class, npc.tmpl != nil)
		}
	}
	sum := r.w.ClassTemplateSummary()
	if !strings.HasPrefix(sum[0], "no NPC fights by a class template") || !strings.Contains(sum[0], "melee, necromancer, shadowknight, tank, wizard") {
		t.Errorf("the summary: %q", sum)
	}
	p, sess := r.player("Hero", 1001.5, 100)
	mage := r.npc("Gnoll Mage")
	r.fight(mage, p.ServerID)
	for d := time.Duration(0); d <= 20*time.Second; d += 100 * time.Millisecond {
		r.tick(d)
		for _, c := range castsIn(drain(sess)) {
			if c.ms != 4000 && c.ms != 3000 {
				t.Fatalf("a creature of no template cast %+v", c)
			}
		}
	}
}

// The wizard's template chain-casts: its area spell (the highest wizard one at
// or below its level: Fire Storm, not Phoenix Glory, whose 15 s cast its
// max_cast leaves out) 2 s into the fight, naming the player beside its
// target and not one beyond the area, then at once its single target's
// (Burning Gaze, not Explode, whose recast is 180 s, nor Static Shock, lower);
// both every 15 s. Each lands 1.4 s after its cast time, at 35%: the target
// the storm's 82 and the gaze's 134, the one beside it the storm's 36.
func TestTheWizardChainCastsAnAreaThenASingleTarget(t *testing.T) {
	r := newTemplateRig(t)
	r.assign("gnoll mage", "wizard")
	mage := r.npc("Gnoll Mage")
	if mage.Class != 13 || mage.MaxHP != NPCMaxHP(20)*85/100 {
		t.Fatalf("the wizard: class %d, %d HP", mage.Class, mage.MaxHP)
	}
	target, ts := r.player("Target", 1001.5, 100)
	beside, bs := r.player("Beside", 1001.5, 108)
	far, fs := r.player("Far", 1001.5, 125)
	// each sees the others (player rebuilt the tables in order)
	for _, s := range []*session.Session{ts, bs} {
		vs := GetOrCreateVisibility(s)
		for i, id := range []EntityID{target.ServerID, beside.ServerID, far.ServerID} {
			vs.ChannelEntity[20+i], vs.ChannelObjectID[20+i] = id, r.objectID(id)
		}
	}
	r.fight(mage, target.ServerID)
	type when struct {
		d  time.Duration
		fx uint32
		n  int
	}
	var casts []when
	got := map[string][]string{}
	// to 21 s: the second storm (17 s) lands at 20.4, its cast time and castLand on
	for d := time.Duration(0); d <= 21*time.Second; d += 100 * time.Millisecond {
		r.tick(d)
		msgs := map[string][][]byte{"target": drain(ts), "beside": drain(bs), "far": drain(fs)}
		for _, c := range castsIn(msgs["target"]) {
			if c.caster == r.objectID(mage.ServerID) && c.ms != 3000 {
				casts = append(casts, when{d, c.fx, len(c.others)})
				if c.fx == fxFireStorm && (len(c.others) != 1 || c.others[0] != r.objectID(beside.ServerID)) {
					t.Errorf("the storm's others: %#x, want the one beside", c.others)
				}
			}
		}
		for name, m := range msgs {
			got[name] = append(got[name], linesOf(m)...)
		}
	}
	want := []when{{2 * time.Second, fxFireStorm, 1}, {4400 * time.Millisecond, fxGaze, 0},
		{17 * time.Second, fxFireStorm, 1}, {19400 * time.Millisecond, fxGaze, 0}}
	if fmt.Sprint(casts) != fmt.Sprint(want) {
		t.Errorf("casts %v, want %v", casts, want)
	}
	wantTarget := []string{"Gnoll Mage's Fire Storm hits you for 29 (your resist took 0%).",
		"Gnoll Mage's Burning Gaze hits you for 47 (your resist took 0%).",
		"Gnoll Mage's Fire Storm hits you for 29 (your resist took 0%)."}
	if strings.Join(got["target"], "|") != strings.Join(wantTarget, "|") {
		t.Errorf("the target was told %q, want %q", got["target"], wantTarget)
	}
	if strings.Join(got["beside"], "|") != "Gnoll Mage's Fire Storm hits you for 13 (your resist took 0%).|Gnoll Mage's Fire Storm hits you for 13 (your resist took 0%)." {
		t.Errorf("the one beside was told %q", got["beside"])
	}
	if len(got["far"]) != 0 || far.HP != far.MaxHP {
		t.Errorf("the one beyond the area: %q, HP %d", got["far"], far.HP)
	}
	if beside.HP != beside.MaxHP-26 {
		t.Errorf("the one beside lost %d, want 26", beside.MaxHP-beside.HP)
	}
}

// The melee's template kicks in its share of rounds (the dice here always
// say yes), a second after the round's last hit (Attack2's at 1.5 s): a
// 0x00B1 of cast 0 with Sandstorm's creature special's visual and the level
// times 7.4, whatever the player's armour; its melee hits are 115% of its
// roll. The tank's takes 85% of a player's melee and has 130% of its level's
// hit points.
func TestTheMeleeKicksAndTheTankTakesLess(t *testing.T) {
	r := newTemplateRig(t)
	r.assign("gnoll brute", "melee")
	r.assign("gnoll guard", "tank")
	defer func(f func(int) int) { templateIntn = f }(templateIntn)
	templateIntn = func(int) int { return 0 }
	brute, guard := r.npc("Gnoll Brute"), r.npc("Gnoll Guard")
	if brute.Class != 4 || brute.meleeOut(100) != 115 || brute.MaxHP != NPCMaxHP(34) {
		t.Errorf("the melee: class %d, a 100 hit %d, %d HP", brute.Class, brute.meleeOut(100), brute.MaxHP)
	}
	if guard.MaxHP != NPCMaxHP(30)*130/100 || guard.Class != 0 {
		t.Errorf("the tank: %d HP, class %d", guard.MaxHP, guard.Class)
	}
	p, sess := r.player("Hero", 2001.5, 100)
	p.AC = 1763
	fixed := func(int) int { return 0 }
	plain := &NPC{Name: "plain", Level: 30}
	if got, base := playerMeleeHit(p, guard, fixed), playerMeleeHit(p, plain, fixed); got != int(math.Round(float64(base)*0.85)) {
		t.Errorf("a player's hit on the tank %d, on a plain creature %d", got, base)
	}
	r.fight(brute, p.ServerID)
	var kicks []time.Duration
	for d := time.Duration(0); d <= 4*time.Second; d += 100 * time.Millisecond {
		r.tick(d)
		msgs := drain(sess)
		for _, c := range castsIn(msgs) {
			if c.caster == r.objectID(brute.ServerID) && c.fx == templateSpecialFX {
				if c.ms != 0 {
					t.Errorf("the kick's cast time %d", c.ms)
				}
				kicks = append(kicks, d)
				found := false
				for _, f := range damageIndications(msgs) {
					found = found || f[0] == int64(r.objectID(brute.ServerID)) && f[1] == -252
				}
				if !found {
					t.Errorf("no 0x00DB of 252 with the kick: %v", damageIndications(msgs))
				}
			}
		}
	}
	if fmt.Sprint(kicks) != fmt.Sprint([]time.Duration{2500 * time.Millisecond}) {
		t.Errorf("kicks at %v, want one at 2.5 s", kicks)
	}
}

// A heal is cast on the NPC itself, and only when it is below its share
// (here 50%): at 60% none, at 40% one, the heal's figure added.
func TestAHealIsCastBelowItsShare(t *testing.T) {
	r := newTemplateRig(t)
	if _, err := r.db.Exec(`INSERT INTO classtemplates (name, class, abilities) VALUES ('priest', 9, '[{"kind": "heal", "every": 4}]')`); err != nil {
		t.Fatal(err)
	}
	r.assign("gnoll priest", "priest")
	priest := r.npc("Gnoll Priest")
	p, sess := r.player("Hero", 4001.5, 100)
	r.fight(priest, p.ServerID)
	priest.HP = priest.MaxHP * 60 / 100
	heals := 0
	run := func(from, to time.Duration) {
		for d := from; d <= to; d += 100 * time.Millisecond {
			r.tick(d)
			for _, c := range castsIn(drain(sess)) {
				if c.fx == fxHealing {
					if c.caster != r.objectID(priest.ServerID) || c.target != c.caster {
						t.Errorf("the heal's 0x00B1: %+v, want the priest on itself", c)
					}
					heals++
				}
			}
		}
	}
	run(0, 5*time.Second)
	if heals != 0 {
		t.Fatalf("%d heals at 60%%", heals)
	}
	priest.HP = priest.MaxHP * 40 / 100
	low := priest.HP
	run(5100*time.Millisecond, 9*time.Second)
	if heals != 1 || priest.HP != low+125 {
		t.Errorf("%d heals at 40%%, HP %d from %d; want one, +125", heals, priest.HP, low)
	}
}

// A spell's hit is cut by the player's resist to its element, by its name:
// Fire Bolt by fire resist at the cap, 75% (the dice holding it). An ability
// that pins its spell casts it whatever the class's spells are.
func TestATemplatesSpellIsResisted(t *testing.T) {
	r := newTemplateRig(t)
	if _, err := r.db.Exec(`INSERT INTO classtemplates (name, class, abilities) VALUES ('pyro', -1, '[{"kind": "nuke", "spell": "fire bolt", "every": 10}]')`); err != nil {
		t.Fatal(err)
	}
	r.assign("gnoll pyro", "pyro")
	defer func(f func() float64) { resistRand = f }(resistRand)
	resistRand = func() float64 { return 0.5 }
	pyro := r.npc("Gnoll Pyro")
	if pyro.Class != 0 {
		t.Errorf("class %d: -1 keeps its own", pyro.Class)
	}
	p, sess := r.player("Hero", 5001.5, 100)
	p.FireR = ResistCap
	r.fight(pyro, p.ServerID)
	var lines []string
	for d := time.Duration(0); d <= 4*time.Second; d += 100 * time.Millisecond {
		r.tick(d)
		lines = append(lines, linesOf(drain(sess))...)
	}
	if strings.Join(lines, "|") != "Gnoll Pyro's Fire Bolt hits you for 25 (your resist took 75%)." {
		t.Errorf("lines %q", lines)
	}
	for name, want := range map[string]Stat{"Ice Storm": StatCR, "Fire Storm": StatFR, "Energy Storm": StatLR, "Siphon Life": StatAR,
		"Static Clash": StatLR, "Plague Bolt": StatDR, "Protect": StatAR} {
		if got := templateResist(name, ""); got != want {
			t.Errorf("%s: resist %v, want %v", name, got, want)
		}
	}
	if got := templateResist("Greater Ice Storm", "Target, Decrease Hitpoints by 682 aoe Decrease Hitpoints by 292 - Cold Based AoE"); got != StatCR {
		t.Errorf("by its line: %v", got)
	}
}

// A templated creature fighting a pet casts at the pet: the tap's figure off
// the pet whole (a pet has no resist), as much back on the creature.
func TestATemplateCastsAtAPet(t *testing.T) {
	r := newTemplateRig(t)
	r.assign("gnoll necro", "necromancer")
	necro := r.npc("Gnoll Necro")
	owner, _ := r.player("Owner", 130, 100)
	pet := &NPC{ServerID: 3000, Name: "skeleton", Level: 20, HP: 1000, MaxHP: 1000, X: 101.5, Y: 10, Z: 100, Owner: owner.ServerID}
	r.w.NPCs[pet.ServerID] = pet
	r.w.SpatialIndex.Insert(pet.ServerID, pet.X, pet.Z)
	r.fight(necro, pet.ServerID)
	necro.HP = 100
	for d := time.Duration(0); d <= 4500*time.Millisecond; d += 100 * time.Millisecond {
		r.tick(d)
	}
	if pet.HP > 1000-15 || necro.HP < 115 {
		t.Errorf("the pet %d HP, the necromancer %d: want the tap's 15 off the one and on the other", pet.HP, necro.HP)
	}
}

// A GM's copy of a templated creature fights by its template, its fight its
// own; a pet made on a templated creature's body fights by none.
func TestCopiesAndPetsOfATemplatedCreature(t *testing.T) {
	r := newTemplateRig(t)
	r.assign("gnoll necro", "necromancer")
	necro := r.npc("Gnoll Necro")
	necro.fight().on, necro.fight().busy = true, r.at(time.Hour)
	id, _, ok := r.w.SpawnNPCCopy("Gnoll Necro", 600, 10, 600, 0, 0)
	if !ok {
		t.Fatal("no copy")
	}
	c := r.w.NPCs[id]
	if c.TemplateName() != "necromancer" || c.MaxHP != necro.MaxHP || c.tfight != nil {
		t.Errorf("the copy: %q, %d HP, its fight %+v", c.TemplateName(), c.MaxHP, c.tfight)
	}
	owner := &Player{ServerID: 1500, Name: "Owner", World: 0}
	pet := newPet(necro, 4000, owner, "Summon Gnoll", 20)
	if pet.tmpl != nil || pet.Class != classWarrior || pet.MaxHP != NPCMaxHP(20) {
		t.Errorf("the pet: template %v, class %d, %d HP", pet.tmpl != nil, pet.Class, pet.MaxHP)
	}
}

// The kinds a template's abilities pick: a nuke is a harmful hit of one
// target and no drain, an area spell a hit with an area, a tap a hit that
// heals its caster, a heal a heal that is no pact; a summoning, a charm or a
// spell for undead alone none of them.
func TestTheKindsOfAbility(t *testing.T) {
	spell := func(name, line string) *CastSpell {
		s := SpellOf(&database.SpellPatternRecord{SpellName: name, Scope: 1, CastTime: 2, Recast: 3, Range: 16880}, 0, line, 0)
		return &s
	}
	for _, c := range []struct {
		name, line string
		kinds      string
	}{
		{"Burning Gaze", "Target, Decrease Hitpoints by 134", "nuke"},
		{"Fire Storm", "Target, Decrease Hitpoints by 82, aoe Decrease Hitpoints by 36", "aoe"},
		{"Siphon Life", "Target, Drain HP by 29 and gives to self", "lifetap"},
		{"Darkstrike", "Target, Decrease Hitpoints by 58 ; Increase 20% based on STR, Drain HP by 9 give self; Increase 2% based on INT", "lifetap"},
		{"Healing", "Target, Increase Hitpoints by 125", "heal"},
		{"Gloom Pact", "Target, Increase Hitpoints by 25, Decrease Hitpoints by 50 to Self", ""},
		{"Summon Bones", "Summons a pet", ""},
		{"Smite Undead", "Target undead, Decrease Hitpoints by 100", ""},
		{"Root", "Target, Root, 36sec", ""},
	} {
		s := spell(c.name, c.line)
		var kinds []string
		for _, k := range database.AbilityKinds {
			if templateKindTakes(k, s) {
				kinds = append(kinds, k)
			}
		}
		if strings.Join(kinds, ",") != c.kinds {
			t.Errorf("%s (%q): %v, want %q", c.name, c.line, kinds, c.kinds)
		}
	}
}

// #stats on a templated creature says its template: its hit points against
// its own, its kick, and what each ability casts at its level; the control,
// a creature of no template, has no such line.
func TestStatsSayATemplate(t *testing.T) {
	r := newTemplateRig(t)
	r.assign("gnoll mage", "wizard")
	_, sess := r.player("GM", 1001.5, 100)
	mage, pup := r.npc("Gnoll Mage"), r.npc("Gnoll Pup")
	lines := strings.Join(r.w.StatsReport(sess, r.objectID(mage.ServerID), r.at(0)), "\n")
	for _, want := range []string{
		"Class template wizard: a Wizard, hit points 1020 (85% of its 1200)",
		"aoe Fire Storm (level 16, 2s cast) every 15s at 35%",
		"nuke Burning Gaze (level 16, 2s cast) chained at 35%",
	} {
		if !strings.Contains(lines, want) {
			t.Errorf("#stats on the wizard lacks %q:\n%s", want, lines)
		}
	}
	if lines := strings.Join(r.w.StatsReport(sess, r.objectID(pup.ServerID), r.at(0)), "\n"); strings.Contains(lines, "Class template") {
		t.Errorf("the control has a template line:\n%s", lines)
	}
}
