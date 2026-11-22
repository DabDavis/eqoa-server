package entity

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// The enchanter's charm (F3 20261025-164922), held to what the reference server sent
// an enchanter of 49 (harvests/sandstorm-spells-20261021/pets/
// dolph-charm-*.json): 0x007E with the creature's own id, 0x1003 with its
// name, its channel's 142 3, 143 0 and npc flags 0x80; and to the live
// game's rules as eqoa.fandom.com has them (2026-11-02, charm.go's doc): the
// caps, undead charmed, the CHA share of the time, the one zone grace.

func entrancing(name string, level int, line string) CastSpell {
	return CastSpell{SpellID: 500 + level, DisplayIndex: 8, Level: level, Scope: 1, Name: name, CastTime: 5 * time.Second,
		Recast: time.Second, PowerCost: 100, Range: 30, Def: ParseSpell(line, "Charms an enemy to aid you.")}
}

func captivation() CastSpell {
	return entrancing("Entrancing Captivation", 39, "Target, Charm ; Increase 25% based on CHA")
}

func enticement() CastSpell {
	return entrancing("Entrancing Enticement", 4, "Target, Charm ; Increase 25% based on CHA , 4min")
}

// charmRig is an enchanter of 49 (CHA 150) beside the pet rig's crab, a
// level 34 creature on its first channel, and a level 1 Rotting Flesh on its
// second.
func charmRig(t *testing.T) (*petRig, *NPC, uint32) {
	r := newPetRig(t)
	r.p.Name, r.p.Class, r.p.Level, r.p.Cha = "Dolph", classEnchanter, 49, 150
	crab := r.enemy
	crab.SpawnX, crab.SpawnY, crab.SpawnZ = crab.X, crab.Y, crab.Z
	flesh := &NPC{ServerID: 3300, Name: "Rotting Flesh", Level: 1, HP: 40, MaxHP: 40, ModelID: 0x1BBB898C, Size: 1,
		X: 8635, Y: 75, Z: 5425, World: 2, SpawnX: 8635, SpawnY: 75, SpawnZ: 5425}
	r.w.NPCs[flesh.ServerID] = flesh
	r.w.SpatialIndex.Insert(flesh.ServerID, flesh.X, flesh.Z)
	vs := GetOrCreateVisibility(r.sess)
	vs.ChannelEntity[1], vs.ChannelObjectID[1] = flesh.ServerID, 0x01312E97
	return r, flesh, 0x01312E97
}

const crabID = 0x0133E8A0

// castCharm casts and lands spell at the object id, and returns what was
// sent from the start on.
func castCharm(t *testing.T, r *petRig, spell CastSpell, target uint32, at time.Duration) (bool, [][]byte) {
	t.Helper()
	r.sent()
	began := r.w.StartCast(r.sess, spell, target, r.at(at))
	msgs := r.sent()
	if began {
		r.w.mu.Lock()
		r.w.castSystem(r.at(at + spell.CastTime + castLand))
		r.w.mu.Unlock()
		for _, m := range r.sent() {
			if opOfMsg(m) != protocol.OpcodeCooldownTimer {
				msgs = append(msgs, m)
			}
		}
	}
	return began, msgs
}

func TestTheCharmLinesAreRead(t *testing.T) {
	for _, c := range []struct {
		line, desc string
		dur        time.Duration
		max, pct   int
	}{
		{"Target, Charm ; Increase 25% based on CHA , 4min", "", 4 * time.Minute, 0, 25},
		{"Target, Charm ; Increase 25% based on CHA", "Charms an enemy to aid you.", 0, 0, 25},
		{"Target, Charm ; Increase 25% based on CHA, 2.5min", "", 150 * time.Second, 0, 25},
		{"Target, Charms up to level 54 mobs, 1 hour", "", time.Hour, 54, 0},
		{"Target, Charm ; Increase 100% based on CHA, 5min", "Charm up to level 54. Duration: 45 min", 5 * time.Minute, 54, 100},
	} {
		d := ParseSpell(c.line, c.desc)
		e := charmEffect(&d)
		if !d.Charms() || !d.Harmful || d.Duration != c.dur || d.CharmMax != c.max || e.ScalePct != c.pct || (c.pct > 0 && e.Scale != StatCha) {
			t.Errorf("%q: charms %v, harmful %v, %v, max %d, %+v", c.line, d.Charms(), d.Harmful, d.Duration, d.CharmMax, e)
		}
	}
}

// Refused at its start, nothing taken: a creature above the charm's level,
// "Your target is too high a level to charm." The control: the crab, level
// 34, takes a charm of 39 and the cast begins.
func TestACharmIsRefusedTooHighALevel(t *testing.T) {
	r, _, _ := charmRig(t)
	if began, msgs := castCharm(t, r, entrancing("Entrancing Coercion", 29, "Target, Charm ; Increase 25% based on CHA"), crabID, 0); began ||
		len(msgs) != 1 || !bytes.Contains(msgs[0], wide16("too high a level")) {
		t.Errorf("a level 34 crab and a charm of 29: began %v, %x", began, msgs)
	}
	r2, _, _ := charmRig(t)
	if began, _ := castCharm(t, r2, captivation(), crabID, 0); !began {
		t.Error("the control: the crab under a charm of 39 was refused")
	}
}

// castsAt is whether msgs hold a cast's 0x00B1.
func castsAt(msgs [][]byte) bool {
	for _, m := range msgs {
		if opOfMsg(m) == protocol.OpcodeCastingCombat {
			return true
		}
	}
	return false
}

// redLine is a cast's refusal as the reference server sends one: 0x0A7B, the words,
// (255, 63, 63).
func redLine(s string) []byte {
	b := binary.LittleEndian.AppendUint16(nil, uint16(protocol.OpcodeColoredChat))
	b = binary.LittleEndian.AppendUint32(b, uint32(len(s)))
	return append(append(b, wide16(s)...), castRefusalRGB[:]...)
}

// F3 20261029-082049 (a GM: "Charm line should not be castable while you
// currently have a pet"): with a charmed creature out a charm is not cast at
// all, whatever is targeted (a creature it could take, an undead, no one): the
// one red line, as the reference server refuses a cast, and no 0x00B1 (the 600 ms fizzle
// it showed, which looked cast), no lockout, no power, no hate. A summoned pet
// out is no refusal since petition 1348: it goes when the charm takes hold
// (charm_summoned_test.go). The controls: with no pet the same charm is cast
// (its 0x00B1 goes out); with the summoned pet out it is cast too, and a spell
// that is no charm; the charmed one let go, the charm is cast again.
func TestACharmIsNotCastWithAPetOut(t *testing.T) {
	defer func(f func(int) int) { charmIntn = f }(charmIntn)
	charmIntn = dice(99)
	refused := func(t *testing.T, r *petRig, spell CastSpell, target uint32, at time.Duration, what string) {
		t.Helper()
		power, lockouts := r.p.Power, len(r.p.Lockouts)
		began, msgs := castCharm(t, r, spell, target, at)
		if began || len(msgs) != 1 || !bytes.Equal(msgs[0], redLine(charmHavePet)) {
			t.Errorf("%s: began %v, sent %x, want only %x", what, began, msgs, redLine(charmHavePet))
		}
		if r.p.Power != power || len(r.p.Lockouts) != lockouts || r.p.Casting != nil && r.p.Casting.spell.SpellID == spell.SpellID {
			t.Errorf("%s: power %d (was %d), lockouts %d (were %d), casting %v", what, r.p.Power, power, len(r.p.Lockouts), lockouts, r.p.Casting != nil)
		}
	}

	// the control: no pet, the charm is cast
	r0, _, _ := charmRig(t)
	if began, msgs := castCharm(t, r0, captivation(), crabID, 0); !began || !castsAt(msgs) {
		t.Fatalf("the control, no pet: began %v, sent %x", began, msgs)
	}

	// the control: a summoned pet out, the charm is cast (petition 1348), and
	// so is a spell that is no charm
	r, _, _ := charmRig(t)
	r.summon(t, fireElemental(), time.Minute)
	if began, msgs := castCharm(t, r, captivation(), crabID, 2*time.Minute); !began || !castsAt(msgs) {
		t.Errorf("the control, a summoned pet out: began %v, sent %x", began, msgs)
	}
	nuke := CastSpell{SpellID: 610, DisplayIndex: 9, Level: 20, Scope: 1, Name: "Fiery Burst", CastTime: 2 * time.Second,
		Recast: time.Second, PowerCost: 10, Range: 30, Def: ParseSpellText("Target, Decrease Hitpoints by 26")}

	// a charmed pet: the crab taken, a second charm at the Rotting Flesh, at
	// the crab itself and at no one
	r2, flesh, fleshID2 := charmRig(t)
	if began, _ := castCharm(t, r2, captivation(), crabID, 0); !began || r2.w.petOfLocked(r2.p) != r2.enemy {
		t.Fatalf("the crab was not charmed: began %v", began)
	}
	refused(t, r2, enticement(), fleshID2, time.Minute, "a charmed pet, at an undead")
	refused(t, r2, captivation(), crabID, 2*time.Minute, "a charmed pet, at itself")
	refused(t, r2, captivation(), 0xFFFFFFFF, 3*time.Minute, "a charmed pet, at no one")
	if flesh.Owner != 0 || len(flesh.Threats) != 0 {
		t.Errorf("the Rotting Flesh was touched: owner %d, threats %v", flesh.Owner, flesh.Threats)
	}
	// the control: the charmed pet out, a spell that is no charm is cast
	r2.p.Power = r2.p.MaxPower
	if began, msgs := castCharm(t, r2, nuke, fleshID2, 4*time.Minute); !began || !castsAt(msgs) {
		t.Errorf("the control, a nuke with the charmed pet out: began %v, sent %x", began, msgs)
	}
	// the control: the charmed one let go, the charm is cast again
	r2.w.PetCommand(r2.sess, petCmdDismiss, 0, r2.at(5*time.Minute))
	if r2.w.petOfLocked(r2.p) != nil {
		t.Fatal("the charmed crab stayed")
	}
	flesh.Threats, flesh.CurrentEnemy = nil, 0
	r2.p.Power = r2.p.MaxPower
	if began, msgs := castCharm(t, r2, enticement(), fleshID2, 6*time.Minute); !began || !castsAt(msgs) {
		t.Errorf("the control, the charmed pet let go: began %v, sent %x", began, msgs)
	}
}

// Taken: the crab is the enchanter's pet as the reference server's ant was, by its own
// id, its channel turned player-side; it keeps its level and hit points,
// swings as itself, follows and fights for its master.
func TestACharmMakesTheCreatureAPet(t *testing.T) {
	r, _, _ := charmRig(t)
	defer func(f func(int) int) { charmIntn = f }(charmIntn)
	charmIntn = dice(99)
	crab := r.enemy
	crab.HP = 1500
	began, msgs := castCharm(t, r, captivation(), crabID, 0)
	if !began || r.w.petOfLocked(r.p) != crab || crab.Owner != r.p.ServerID || crab.NPCType != charmNPCType || crab.HP != 1500 || crab.Level != 34 {
		t.Fatalf("began %v; the crab %+v", began, *crab)
	}
	var tail [][]byte
	for _, m := range msgs {
		if op := opOfMsg(m); op == protocol.OpcodeSetPet || op == protocol.OpcodeNoCollideAdd {
			tail = append(tail, m)
		}
	}
	if len(tail) != 2 || binary.LittleEndian.Uint32(tail[0][2:]) != crabID ||
		!bytes.Equal(tail[1], unhex(t, "03100b0000004465736572742043726162000000"+"60")) {
		t.Errorf("0x007E/0x1003 % X", tail)
	}
	buf := SerializeNPCUpdate(crabID, crab)
	for _, c := range []struct {
		at   int
		want byte
	}{{142, 3}, {143, 0}, {145, 0x00}, {146, 0x80}} {
		if buf[c.at] != c.want {
			t.Errorf("channel byte %d = %#x, want Sandstorm's charmed ant's %#x", c.at, buf[c.at], c.want)
		}
	}
	if _, _, _, hits := npcRound(crab); crab.SwingFixed || len(hits) != 2 {
		t.Errorf("the crab swings %d a round (fixed %v): its own Attack2 at 34", len(hits), crab.SwingFixed)
	}
	r.p.X += 30
	for d := time.Second; d <= 10*time.Second; d += 100 * time.Millisecond {
		r.tick(10*time.Second + d)
	}
	if dd := distXZ(crab.X, crab.Z, r.p.X, r.p.Z); dd > petFollowStart {
		t.Errorf("the charmed crab is %.1f off its master", dd)
	}
}

// Undead are charmed as anything else (the wiki's Charmable Mobs page:
// spectres, spirits, lost souls, ghouls and dead riders): an enchanter's
// charm takes the Rotting Flesh; resisted (a roll under the charm's 10%), with
// the reference server's line, and the creature comes at the caster. The controls: the
// same roll does not resist at the crab, and the necromancer's undead charm
// is refused at the crab (undead.go).
func TestACharmTakesUndead(t *testing.T) {
	r, flesh, fleshID := charmRig(t)
	defer func(f func(int) int) { charmIntn = f }(charmIntn)
	charmIntn = dice(99)
	if began, _ := castCharm(t, r, captivation(), fleshID, 0); !began || flesh.Owner != r.p.ServerID || r.w.petOfLocked(r.p) != flesh {
		t.Fatalf("an undead not charmed: began %v, owner %d", began, flesh.Owner)
	}

	charmIntn = dice(9) // under the 10%: resisted
	r2, flesh2, fleshID2 := charmRig(t)
	_, msgs2 := castCharm(t, r2, captivation(), fleshID2, 0)
	want := unhex(t, "7b0a1100000041007400740065006d00700074002000720065007300690073007400650064002100ff3f3f")
	found := false
	for _, m := range msgs2 {
		found = found || bytes.Equal(m, want)
	}
	if !found || flesh2.Owner != 0 || flesh2.CurrentEnemy != r2.p.ServerID {
		t.Errorf("resisted: sent %x; Rotting Flesh owner %d, enemy %d", msgs2, flesh2.Owner, flesh2.CurrentEnemy)
	}
	charmIntn = dice(10) // the control: at the 10% it holds
	r3, _, _ := charmRig(t)
	if _, _ = castCharm(t, r3, captivation(), crabID, 0); r3.enemy.Owner != r3.p.ServerID {
		t.Errorf("a roll of 10 did not take the crab: %+v", *r3.enemy)
	}

	r4, _, _ := charmRig(t)
	r4.p.Class = classNecromancer
	coerce := CastSpell{SpellID: 974, DisplayIndex: 8, Level: 50, Scope: 1, Name: "Coerce Undead", CastTime: 5 * time.Second,
		Recast: 30 * time.Second, PowerCost: 260, Range: 30,
		Def: ParseSpell("Target undead, Charm ; Increase 25% based on CHA, 2min", "CM: Charm an Undead")}
	began, msgs := castCharm(t, r4, coerce, crabID, 0)
	said := false
	for _, m := range msgs {
		said = said || bytes.Contains(m, wide16(castNotUndead))
	}
	if began || r4.enemy.Owner != 0 || !said {
		t.Errorf("the necromancer's undead charm at the crab: began %v, owner %d, %x", began, r4.enemy.Owner, msgs)
	}
}

// Let go: at the line's end (Entrancing Enticement's 4 minutes) and when
// dismissed it turns on its master, off the pet bar (0x007F, 0x1004), itself
// again; a master's logout lets it go in peace.
func TestACharmEnds(t *testing.T) {
	defer func(f func(int) int) { charmIntn = f }(charmIntn)
	charmIntn = dice(99)
	for _, c := range []struct {
		name    string
		end     func(r *petRig)
		hostile bool
	}{
		// landed at 6.4 s, then 4 minutes and 150 CHA's 37.5 s
		{"worn off", func(r *petRig) { r.tick(4*time.Minute + 40*time.Second); r.tick(4*time.Minute + 50*time.Second) }, true},
		{"dismissed", func(r *petRig) { r.w.PetCommand(r.sess, petCmdDismiss, 0, r.at(time.Minute)) }, true},
		{"its master logged out", func(r *petRig) { delete(r.w.Players, r.p.ServerID); r.tick(time.Minute) }, false},
	} {
		r, _, _ := charmRig(t)
		crab := r.enemy
		crab.Level = 2 // Entrancing Enticement's cap: its level 4 less 2
		if began, _ := castCharm(t, r, enticement(), crabID, 0); !began || crab.Owner == 0 {
			t.Fatalf("%s: not charmed", c.name)
		}
		r.tick(time.Minute - time.Second)
		if crab.Owner == 0 {
			t.Fatalf("%s: let go before its time", c.name)
		}
		if c.name == "worn off" {
			r.tick(4*time.Minute + 20*time.Second) // past the line's 4 minutes, inside its CHA share
			if crab.Owner == 0 {
				t.Fatalf("%s: let go at the line's time, its CHA share not counted", c.name)
			}
		}
		r.sent()
		c.end(r)
		msgs := r.sent()
		if crab.Owner != 0 || crab.Charm != nil || crab.NPCType != 0x0100 || r.w.pets[r.p.ServerID] != nil {
			t.Errorf("%s: the crab %+v", c.name, *crab)
		}
		if hostile := crab.CurrentEnemy == r.p.ServerID; hostile != c.hostile {
			t.Errorf("%s: it turned on its master %v, want %v", c.name, hostile, c.hostile)
		}
		if c.hostile && (len(msgs) < 2 || opOfMsg(msgs[0]) != protocol.OpcodeClearPet || opOfMsg(msgs[1]) != protocol.OpcodeNoCollideRemove) {
			t.Errorf("%s: sent %x", c.name, msgs)
		}
	}
}

// Dead as a pet: its corpse is still its master's (no one loots it); it
// comes back itself, at its spawn. Its master taken into another world (a
// coach to Tunaria) takes it past its grace: the charm is gone, it is home at
// once and fights no one, its master told; and one let go far from home in
// its own world (three zones off) is sent home too, where one let go a zone
// off (its grace) turns on its master there.
func TestACharmedCreatureDiesAndComesBackItself(t *testing.T) {
	defer func(f func(int) int) { charmIntn = f }(charmIntn)
	charmIntn = dice(99)
	r, _, _ := charmRig(t)
	crab := r.enemy
	castCharm(t, r, captivation(), crabID, 0)
	r.sent()
	r.w.mu.Lock()
	crab.World = 0 // led to Tunaria, killed there (a GM's TEST menu KILL: no hit)
	crab.HP = 0
	r.w.killNPC(crab)
	r.w.mu.Unlock()
	r.tick(time.Minute + time.Second)
	// its death ends the charm (the owner, 2026-11-14: "a GM has charmed 2 dead
	// crocodiles and they stayed charmed"): off its master's bar, its master
	// told, the corpse a creature's again (no owner, no charmed flags), of
	// nothing (no one's to loot), lying where it fell
	cleared := false
	for _, m := range r.sent() {
		cleared = cleared || opOfMsg(m) == protocol.OpcodeClearPet
	}
	if crab.Owner != 0 || crab.Charm != nil || crab.NPCType != 0x0100 || r.w.pets[r.p.ServerID] != nil || !cleared {
		t.Errorf("its corpse: owner %d, charm %v, npc type %#x, pet kept %v, master told %v", crab.Owner, crab.Charm != nil,
			crab.NPCType, r.w.pets[r.p.ServerID] != nil, cleared)
	}
	if !crab.Dead || !crab.CorpseOfNothing() || crab.World != 0 {
		t.Errorf("its corpse: dead %v, of nothing %v, world %d (where it fell, 0)", crab.Dead, crab.CorpseOfNothing(), crab.World)
	}
	r.w.mu.Lock()
	r.w.respawnNPCLocked(crab)
	r.w.mu.Unlock()
	if crab.Owner != 0 || crab.Charm != nil || crab.NPCType != 0x0100 || crab.World != 2 || crab.X != crab.SpawnX || crab.CorpseOfNothing() {
		t.Errorf("back: %+v", *crab)
	}

	// a dead creature is not charmed: the charm lands on nothing
	rd, _, _ := charmRig(t)
	rd.w.mu.Lock()
	rd.enemy.HP = 0
	rd.w.killNPC(rd.enemy)
	rd.w.mu.Unlock()
	castCharm(t, rd, captivation(), crabID, 0)
	if rd.enemy.Owner != 0 || rd.enemy.Charm != nil || rd.w.pets[rd.p.ServerID] != nil {
		t.Errorf("a corpse charmed: owner %d, charm %v", rd.enemy.Owner, rd.enemy.Charm != nil)
	}

	r2, _, _ := charmRig(t)
	crab2 := r2.enemy
	castCharm(t, r2, captivation(), crabID, 0)
	r2.sent()
	r2.p.World, r2.p.X, r2.p.Z = 0, 20000, 20000 // a coach to Tunaria
	r2.tick(time.Minute)
	msgs := r2.sent()
	if crab2.World != 2 || crab2.X != crab2.SpawnX || crab2.Owner != 0 || crab2.CurrentEnemy != 0 || r2.w.pets[r2.p.ServerID] != nil {
		t.Errorf("its master in Tunaria: %+v", *crab2)
	}
	told := false
	for _, m := range msgs {
		told = told || opOfMsg(m) == protocol.OpcodeClearPet
	}
	if !told {
		t.Errorf("its master was not told (0x007F): %x", msgs)
	}

	for _, c := range []struct {
		name    string
		dx      float32
		home    bool
		hostile bool
	}{
		{"a zone off", charmZone, false, true},
		{"three zones off", 3 * charmZone, true, false},
	} {
		r3, _, _ := charmRig(t)
		crab3 := r3.enemy
		castCharm(t, r3, captivation(), crabID, 0)
		r3.w.mu.Lock()
		crab3.X += c.dx
		r3.p.X = crab3.X + 2
		r3.w.mu.Unlock()
		r3.w.PetCommand(r3.sess, petCmdDismiss, 0, r3.at(time.Minute))
		if home := crab3.X == crab3.SpawnX; home != c.home || (crab3.CurrentEnemy == r3.p.ServerID) != c.hostile || crab3.Owner != 0 {
			t.Errorf("%s: home %v (want %v), enemy %d (hostile want %v)", c.name, home, c.home, crab3.CurrentEnemy, c.hostile)
		}
	}
}

// wide16 is s as a wide string's code units, little-endian.
func wide16(s string) []byte {
	var b []byte
	for _, r := range s {
		b = append(b, byte(r), byte(r>>8))
	}
	return b
}

// charmTicks ticks the rig every 100 ms from a to b, as the tick does.
func (r *petRig) charmTicks(a, b time.Duration) {
	for d := a; d <= b; d += 100 * time.Millisecond {
		r.tick(d)
	}
}

// toldLine is whether msgs hold the chat line.
func toldLine(msgs [][]byte, line string) bool {
	for _, m := range msgs {
		if opOfMsg(m) == protocol.OpcodeClientMessage && bytes.Contains(m, wide16(line)) {
			return true
		}
	}
	return false
}

// Charm is one absolute check (#1574, a GM 2026-11-16, petition 2233:
// "basically its an absolute distance check from player location to pet"):
// its master taken 3,000 off in its world (a gate, a coach: alike now), a
// charmed creature is not carried and not let go, it runs to its master at
// Spirit of the Wolf's pace, straight, taking no target though a creature
// beside its path attacks it, until petCatchUp off, then follows; its master
// moved again within charmRunRange it runs on to the new place; past it (4,921
// from the pet) or in another world, it is let go that tick where it stands,
// "Your charm spell has worn off.", fighting no one. Led on foot two zones
// from its spawn, it stays charmed (the old leash is gone).
func TestACharmedCreatureRunsToItsMaster(t *testing.T) {
	defer func(f func(int) int) { charmIntn = f }(charmIntn)
	charmIntn = dice(99)
	r, flesh, _ := charmRig(t)
	crab := r.enemy
	castCharm(t, r, captivation(), crabID, 0)
	if crab.Charm == nil || r.w.pets[r.p.ServerID] == nil {
		t.Fatal("not charmed")
	}
	ps := r.w.pets[r.p.ServerID]
	r.charmTicks(10*time.Second, 11*time.Second) // following beside its master

	// a creature beside its path that fights it
	r.w.mu.Lock()
	flesh.X, flesh.Z = crab.X+2, crab.Z+20
	r.w.SpatialIndex.Update(flesh.ServerID, 8635, 5425, flesh.X, flesh.Z)
	r.w.updateAggro(flesh, crab.ServerID, 50)
	flesh.CurrentEnemy = crab.ServerID
	r.p.Z += 3000 // a gate: 3,000 off in its world
	r.w.mu.Unlock()
	r.tick(12 * time.Second)
	if crab.Charm == nil || r.w.pets[r.p.ServerID] != ps {
		t.Fatal("let go by its master's trip within range")
	}
	d0 := distXZ(crab.X, crab.Z, r.p.X, r.p.Z)
	if d0 < 2900 || !ps.running || crab.Loco != LocoRun {
		t.Fatalf("after the trip: %.0f off its master (placed beside it?), running %v, loco %d", d0, ps.running, crab.Loco)
	}
	r.charmTicks(12*time.Second+100*time.Millisecond, 13*time.Second)
	if got := d0 - distXZ(crab.X, crab.Z, r.p.X, r.p.Z); got < charmRunSpeed-0.5 || got > charmRunSpeed+0.5 {
		t.Errorf("a second's run: %.2f nearer, want %.2f", got, charmRunSpeed)
	}
	if ps.target != 0 || crab.RoundTarget != 0 || !ps.running {
		t.Errorf("attacked on its way: target %d, round %d, running %v", ps.target, crab.RoundTarget, ps.running)
	}

	// its master moved again, still within range of it: on to the new place
	r.w.mu.Lock()
	r.p.X += 1500
	r.w.mu.Unlock()
	d1 := distXZ(crab.X, crab.Z, r.p.X, r.p.Z)
	r.charmTicks(13*time.Second+100*time.Millisecond, 14*time.Second)
	if crab.Charm == nil || !ps.running || d1-distXZ(crab.X, crab.Z, r.p.X, r.p.Z) < charmRunSpeed-0.5 {
		t.Fatalf("its master moved within range: charm %v, running %v, %.0f -> %.0f", crab.Charm != nil, ps.running, d1,
			distXZ(crab.X, crab.Z, r.p.X, r.p.Z))
	}

	// ticked on, it arrives and follows
	end := 14 * time.Second
	for ps.running && end < 10*time.Minute {
		end += 100 * time.Millisecond
		r.tick(end)
	}
	if ps.running || crab.Charm == nil || distXZ(crab.X, crab.Z, r.p.X, r.p.Z) > petCatchUp {
		t.Fatalf("never arrived: running %v, %.0f off", ps.running, distXZ(crab.X, crab.Z, r.p.X, r.p.Z))
	}
	r.charmTicks(end+100*time.Millisecond, end+5*time.Second)
	if crab.Charm == nil || ps.running || distXZ(crab.X, crab.Z, r.p.X, r.p.Z) > petCatchUp {
		t.Fatalf("not following: %.0f off", distXZ(crab.X, crab.Z, r.p.X, r.p.Z))
	}

	// led on foot two zones from its spawn: still charmed
	r.w.mu.Lock()
	crab.X, crab.Z = crab.SpawnX+2*charmZone+10, crab.SpawnZ
	r.p.X, r.p.Z = crab.X+2, crab.Z
	r.w.mu.Unlock()
	end += 6 * time.Second
	r.charmTicks(end, end+time.Second)
	if crab.Charm == nil || r.w.pets[r.p.ServerID] != ps || ps.running {
		t.Fatalf("led two zones on foot: charm %v, running %v", crab.Charm != nil, ps.running)
	}

	for _, c := range []struct {
		name  string
		world uint8
		dx    float32
	}{
		{"4,921 off", 2, charmRunRange + 1},
		{"another world", 0, 2000},
	} {
		r, _, _ := charmRig(t)
		crab := r.enemy
		castCharm(t, r, captivation(), crabID, 0)
		r.charmTicks(10*time.Second, 11*time.Second)
		r.w.mu.Lock()
		r.p.Z += 3000
		r.w.mu.Unlock()
		r.charmTicks(11*time.Second+100*time.Millisecond, 12*time.Second) // running
		if ps := r.w.pets[r.p.ServerID]; ps == nil || !ps.running {
			t.Fatalf("%s: not running", c.name)
		}
		r.sent()
		x, z := crab.X, crab.Z
		r.w.mu.Lock()
		r.p.World, r.p.X, r.p.Z = c.world, crab.X+c.dx, crab.Z
		r.w.mu.Unlock()
		r.tick(12*time.Second + 100*time.Millisecond)
		msgs := r.sent()
		if crab.Charm != nil || crab.Owner != 0 || r.w.pets[r.p.ServerID] != nil {
			t.Fatalf("%s: still charmed", c.name)
		}
		if crab.X != x || crab.Z != z || crab.CurrentEnemy == r.p.ServerID || crab.Threats[r.p.ServerID] != 0 {
			t.Errorf("%s: at (%.1f, %.1f), was (%.1f, %.1f); enemy %d", c.name, crab.X, crab.Z, x, z, crab.CurrentEnemy)
		}
		if !toldLine(msgs, charmWornOff) || opIndex(msgs, protocol.OpcodeClearPet) < 0 {
			t.Errorf("%s: its master not told: %x", c.name, msgs)
		}
	}
}

// A player's flesh crawler (petition 2233): charmed on the Isle of Dread at
// its spawn (24848.5, 3306), its master gone home by Return Home to Klik'Anon
// (23619.4, 6289.0), 3,226 off: it is not let go, it runs, and it arrives in
// about 281 s at charmRunSpeed.
func TestZerkzieesCrawlerRunsToKlikAnon(t *testing.T) {
	defer func(f func(int) int) { charmIntn = f }(charmIntn)
	charmIntn = dice(99)
	r, _, _ := charmRig(t)
	crab := r.enemy
	r.w.mu.Lock()
	ox, oz := crab.X, crab.Z
	crab.World, crab.X, crab.Z = 0, 24848.5, 3306
	crab.SpawnX, crab.SpawnZ = crab.X, crab.Z
	r.w.SpatialIndex.Update(crab.ServerID, ox, oz, crab.X, crab.Z)
	px, pz := r.p.X, r.p.Z
	r.p.World, r.p.X, r.p.Z = 0, crab.X-10, crab.Z
	r.w.SpatialIndex.Update(r.p.ServerID, px, pz, r.p.X, r.p.Z)
	r.w.mu.Unlock()
	castCharm(t, r, captivation(), crabID, 0)
	if crab.Charm == nil {
		t.Fatal("not charmed")
	}
	ps := r.w.pets[r.p.ServerID]
	r.w.mu.Lock()
	crab.X, crab.Z = crab.SpawnX, crab.SpawnZ
	r.p.X, r.p.Z = 23619.4, 6289.0 // Return Home
	r.w.mu.Unlock()
	if d := distXZ(crab.X, crab.Z, r.p.X, r.p.Z); d < 3226 || d > 3227 {
		t.Fatalf("%.1f apart", d)
	}
	start := 10 * time.Second
	r.tick(start)
	if crab.Charm == nil || !ps.running {
		t.Fatalf("at the trip: charm %v, running %v", crab.Charm != nil, ps.running)
	}
	end := start
	for ps.running && end < start+10*time.Minute {
		end += 100 * time.Millisecond
		r.tick(end)
	}
	took := (end - start).Seconds()
	// 3,226.3 at 11.483 a second, petCatchUp short of its master: 279.7 s
	if crab.Charm == nil || ps.running || took < 278 || took > 282 {
		t.Errorf("arrived after %.1f s (want about 280): charm %v, running %v", took, crab.Charm != nil, ps.running)
	}
}

// A summoned pet still comes along at once: its master 3,000 off, it is
// beside its master the next tick.
func TestASummonedPetStillComesAlong(t *testing.T) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	if pet == nil {
		t.Fatal("no pet")
	}
	r.tick(20 * time.Second)
	r.w.mu.Lock()
	r.p.Z += 3000
	r.w.mu.Unlock()
	r.tick(21 * time.Second)
	if d := distXZ(pet.X, pet.Z, r.p.X, r.p.Z); d > petFollow+petFollowStart || r.w.pets[r.p.ServerID].running {
		t.Errorf("a summoned pet 3,000 behind: %.0f off its master the next tick", d)
	}
}
