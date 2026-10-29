package entity

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// The enchanter's charm (F3 20261025-164922), held to what Sandstorm sent
// an enchanter of 49 (harvests/sandstorm-spells-20261021/pets/
// dolph-charm-*.json): 0x007E with the creature's own id, 0x1003 with its
// name, its channel's 142 3, 143 0 and npc flags 0x80; "Attempt resisted!"
// on undead.

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
	flesh := &NPC{ServerID: 3300, Name: "Rotting Flesh", Level: 1, HP: 40, MaxHP: 40, ModelID: 0x1BBB898C, Size: 1,
		X: 8635, Y: 75, Z: 5425, World: 2}
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

// redLine is a cast's refusal as Sandstorm sends one: 0x0A7B, the words,
// (255, 63, 63).
func redLine(s string) []byte {
	b := binary.LittleEndian.AppendUint16(nil, uint16(protocol.OpcodeColoredChat))
	b = binary.LittleEndian.AppendUint32(b, uint32(len(s)))
	return append(append(b, wide16(s)...), castRefusalRGB[:]...)
}

// F3 20261029-082049 (jib: "Charm line should not be castable while you
// currently have a pet"): with a pet out, summoned or charmed, a charm is not
// cast at all, whatever is targeted (a creature it could take, an undead, no
// one): the one red line, as Sandstorm refuses a cast, and no 0x00B1 (the
// 600 ms fizzle it showed, which looked cast), no lockout, no power, no hate.
// The controls: with no pet the same charm is cast (its 0x00B1 goes out); with
// the pet out a spell that is no charm is cast; the pet dismissed, the charm
// is cast again.
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

	// a summoned pet
	r, _, fleshID := charmRig(t)
	pet, _ := r.summon(t, fireElemental(), time.Minute)
	refused(t, r, captivation(), crabID, 2*time.Minute, "a summoned pet, at the crab")
	refused(t, r, enticement(), fleshID, 3*time.Minute, "a summoned pet, at an undead")
	refused(t, r, captivation(), 0xFFFFFFFF, 4*time.Minute, "a summoned pet, at no one")
	if len(r.enemy.Threats) != 0 || r.enemy.Owner != 0 {
		t.Errorf("the crab was touched: threats %v, owner %d", r.enemy.Threats, r.enemy.Owner)
	}
	// the control: the pet out, a spell that is no charm is cast
	nuke := CastSpell{SpellID: 610, DisplayIndex: 9, Level: 20, Scope: 1, Name: "Fiery Burst", CastTime: 2 * time.Second,
		Recast: time.Second, PowerCost: 10, Range: 30, Def: ParseSpellText("Target, Decrease Hitpoints by 26")}
	if began, msgs := castCharm(t, r, nuke, crabID, 5*time.Minute); !began || !castsAt(msgs) {
		t.Errorf("the control, a nuke with the pet out: began %v, sent %x", began, msgs)
	}
	// the control: the pet dismissed, the charm is cast
	r.w.PetCommand(r.sess, petCmdDismiss, 0, r.at(6*time.Minute))
	if r.w.petOfLocked(r.p) != nil {
		t.Fatalf("the pet %q stayed", pet.Name)
	}
	r.enemy.Threats, r.enemy.CurrentEnemy = nil, 0
	if began, msgs := castCharm(t, r, captivation(), crabID, 7*time.Minute); !began || !castsAt(msgs) {
		t.Errorf("the control, the pet dismissed: began %v, sent %x", began, msgs)
	}

	// a charmed pet: the crab taken, a second charm at the Rotting Flesh
	r2, flesh, fleshID2 := charmRig(t)
	if began, _ := castCharm(t, r2, captivation(), crabID, 0); !began || r2.w.petOfLocked(r2.p) != r2.enemy {
		t.Fatalf("the crab was not charmed: began %v", began)
	}
	refused(t, r2, enticement(), fleshID2, time.Minute, "a charmed pet, at an undead")
	if flesh.Owner != 0 || len(flesh.Threats) != 0 {
		t.Errorf("the Rotting Flesh was touched: owner %d, threats %v", flesh.Owner, flesh.Threats)
	}
}

// Taken: the crab is the enchanter's pet as Sandstorm's ant was, by its own
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

// Resisted: undead always, with Sandstorm's line, and the creature comes at
// the caster.
func TestACharmOnUndeadIsResisted(t *testing.T) {
	r, flesh, fleshID := charmRig(t)
	defer func(f func(int) int) { charmIntn = f }(charmIntn)
	charmIntn = dice(99)
	_, msgs := castCharm(t, r, captivation(), fleshID, 0)
	want := unhex(t, "7b0a1100000041007400740065006d00700074002000720065007300690073007400650064002100ff3f3f")
	found := false
	for _, m := range msgs {
		found = found || bytes.Equal(m, want)
	}
	if !found || flesh.Owner != 0 || flesh.CurrentEnemy != r.p.ServerID {
		t.Errorf("sent %x; Rotting Flesh owner %d, enemy %d", msgs, flesh.Owner, flesh.CurrentEnemy)
	}
	charmIntn = dice(0)
	r2, _, _ := charmRig(t)
	if _, _ = castCharm(t, r2, captivation(), crabID, 0); r2.enemy.Owner != 0 || r2.enemy.CurrentEnemy != r2.p.ServerID {
		t.Errorf("a roll under the resist took the crab: %+v", *r2.enemy)
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
		{"worn off", func(r *petRig) { r.tick(4*time.Minute + 10*time.Second) }, true},
		{"dismissed", func(r *petRig) { r.w.PetCommand(r.sess, petCmdDismiss, 0, r.at(time.Minute)) }, true},
		{"its master logged out", func(r *petRig) { delete(r.w.Players, r.p.ServerID); r.tick(time.Minute) }, false},
	} {
		r, _, _ := charmRig(t)
		crab := r.enemy
		crab.Level = 3 // under Entrancing Enticement's 4
		if began, _ := castCharm(t, r, enticement(), crabID, 0); !began || crab.Owner == 0 {
			t.Fatalf("%s: not charmed", c.name)
		}
		r.tick(time.Minute - time.Second)
		if crab.Owner == 0 {
			t.Fatalf("%s: let go before its time", c.name)
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
// comes back itself, in its own world, though it followed its master into
// another; and one let go in another world is sent home.
func TestACharmedCreatureDiesAndComesBackItself(t *testing.T) {
	defer func(f func(int) int) { charmIntn = f }(charmIntn)
	charmIntn = dice(99)
	r, _, _ := charmRig(t)
	crab := r.enemy
	castCharm(t, r, captivation(), crabID, 0)
	r.p.World, r.p.X, r.p.Z = 0, 20000, 20000 // a coach to Tunaria
	r.tick(time.Minute)
	if crab.World != 0 || crab.Owner == 0 {
		t.Fatalf("it did not come along: world %d, owner %d", crab.World, crab.Owner)
	}
	r.w.mu.Lock()
	r.w.killNPC(crab)
	r.w.mu.Unlock()
	r.tick(time.Minute + time.Second)
	if crab.Owner == 0 || r.w.pets[r.p.ServerID] != nil {
		t.Errorf("its corpse: owner %d (no one's to loot needs its master's), pet kept %v", crab.Owner, r.w.pets[r.p.ServerID] != nil)
	}
	r.w.mu.Lock()
	r.w.respawnNPCLocked(crab)
	r.w.mu.Unlock()
	if crab.Owner != 0 || crab.Charm != nil || crab.NPCType != 0x0100 || crab.World != 2 || crab.X != crab.SpawnX {
		t.Errorf("back: %+v", *crab)
	}

	r2, _, _ := charmRig(t)
	crab2 := r2.enemy
	castCharm(t, r2, captivation(), crabID, 0)
	r2.p.World, r2.p.X, r2.p.Z = 0, 20000, 20000
	r2.tick(time.Minute)
	r2.w.PetCommand(r2.sess, petCmdDismiss, 0, r2.at(2*time.Minute))
	if crab2.World != 2 || crab2.X != crab2.SpawnX || crab2.Owner != 0 || crab2.CurrentEnemy != 0 {
		t.Errorf("let go in Tunaria: %+v", *crab2)
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
