package entity

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// The Efreeti Lord's and the Ghilan Noble's kits as tools/fix_pets_1110.py
// leaves them (petitions 1478 and 1495): the tool's rows on the mage pets' rig.

const (
	fx1110Tide uint32 = 0x0B149501
	// the kits: petition 1450's with the tool's ability after them
	efreetiKit1478 = `[{"every":30.0,"first":1.0,"kind":"nuke","spell_id":905453},` +
		`{"every":1800.0,"first":5.0,"kind":"buff","spell_id":902164,"target":"self"},` +
		`{"every":1800.0,"first":7.0,"kind":"buff","spell_id":902164,"target":"owner"}]`
	ghilanTide1495 = `{"every":6.0,"first":2.0,"kind":"heal","spell_id":905495,"target":"group","below":100}`
)

// pets1110Rig is the mage pets' rig with the tool's spell and kits.
func pets1110Rig(t *testing.T) *petRig {
	t.Helper()
	r, db := magePetRig(t, true)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast,
		eqprequire, spellname, spelldesc) VALUES (905495, 60, 0, 0, 16880, 0, 0, 0, 0, 1, 6, -1, 'Ghilan''s Healing Tide', '')`)
	exec(`INSERT INTO spelleffect (spellid, line, fxdict, source, effects) VALUES (905495, 'Target, Increase Hitpoints by 150', ?,
		'test', '{"v":1,"scope":"target","target_type":"Owner/Self","harmful":false,"stack":0,"level":60,"min_level":1,`+
		`"req_equipment":"Any","effects":[{"type":"Dmg/Heal","kind":"hp","amount":150}]}')`, int64(fx1110Tide))
	exec(`UPDATE petstats SET abilities = ? WHERE spellid = 701087`, efreetiKit1478)
	exec(`UPDATE petstats SET abilities = ? WHERE spellid = 701088`, ghilanKit1450[:len(ghilanKit1450)-1]+","+ghilanTide1495+"]")
	if _, err := r.w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	for _, p := range r.w.ClassTemplateProblems() {
		if strings.Contains(p, "petstats") {
			t.Fatalf("a pet row's problem: %s", p)
		}
	}
	return r
}

// Petition 1478 (the magician Lifeispain): "Efreeti Lord doesn't cast a buff on
// the caster. He casts one only himself". With the tool's row the Efreeti Lord
// following its owner casts Efreeti's Fire on itself and then on its owner: both
// carry it, and neither is cast again while they do.
func TestTheEfreetiLordsFireIsOnItsOwnerToo(t *testing.T) {
	r := pets1110Rig(t)
	pet, _ := r.summon(t, summonEfreetiLord, 0)
	has := func(effects []*ActiveEffect) int {
		n := 0
		for _, e := range effects {
			if e.SpellID == 902164 {
				n++
			}
		}
		return n
	}
	for ms := 9700; ms < 40000; ms += 100 {
		r.w.mu.Lock()
		r.w.petSystem(r.at(time.Duration(ms) * time.Millisecond))
		r.w.mu.Unlock()
	}
	if has(pet.Effects) != 1 || has(r.p.Effects) != 1 {
		t.Fatalf("Efreeti's Fire: %d on the pet, %d on its owner; want one on each", has(pet.Effects), has(r.p.Effects))
	}
	if r.p.Mods.Dex <= 0 {
		t.Errorf("its owner's DEX from it: %d", r.p.Mods.Dex)
	}
}

// Petition 1495 (the magician Waldorf): the Ghilan Noble's third heal, "an AOE
// heal for 150 that heals players and pets in range". Its tide lands 150 (never
// more than is missing) on each hurt one of its side within 30 of it: itself,
// its owner, a group mate and the mate's pet; not on a group mate 40 off, a
// player of no group of its owner's, nor a group mate in a duel. It is wanted
// only while one of them is hurt.
func TestTheGhilanNoblesTideHealsItsSideInReach(t *testing.T) {
	r := pets1110Rig(t)
	pet, _ := r.summon(t, summonGhilanNoble, 0)
	ps := r.w.pets[r.p.ServerID]
	ab := ps.petAbilities()
	if len(ab) != 7 || ab[6].TargetOr() != database.AbilityOnGroup || ab[6].pinned == nil || ab[6].pinned.SpellID != 905495 {
		t.Fatalf("its abilities: %d, the last %+v; want the tide seventh", len(ab), ab[len(ab)-1].TemplateAbility)
	}
	tide := &ab[6]
	player := func(id EntityID, name string, dx float32) *Player {
		p := &Player{ServerID: id, Name: name, Class: 0, Level: 60, HP: 3000, MaxHP: 4000, X: r.p.X + dx, Y: r.p.Y, Z: r.p.Z,
			World: r.p.World}
		r.w.Players[id] = p
		r.w.SpatialIndex.Insert(id, p.X, p.Z)
		return p
	}
	mate, far, stranger, duelist, rival := player(2001, "Mate", 5), player(2002, "Far", 40), player(2003, "Stranger", 5),
		player(2004, "Duelist", 5), player(2005, "Rival", 6)
	g := &Group{ID: 9, Members: []EntityID{r.p.ServerID, mate.ServerID, far.ServerID, duelist.ServerID}}
	if r.w.groups == nil {
		r.w.groups = map[uint32]*Group{}
	}
	r.w.groups[g.ID] = g
	for _, p := range []*Player{r.p, mate, far, duelist} {
		p.GroupID = g.ID
	}
	r.w.duels.add(&Duel{ID: 3, A: duelist.ServerID, B: rival.ServerID, who: [2]*Player{duelist, rival}, Phase: duelOn})
	matePet := &NPC{ServerID: 3900, Name: "Fire Elemental", Level: 39, HP: 1000, MaxHP: 2340, Owner: mate.ServerID,
		X: mate.X, Y: mate.Y, Z: mate.Z, World: mate.World}
	r.w.NPCs[matePet.ServerID] = matePet
	r.w.pets[mate.ServerID] = &petState{owner: mate, npc: matePet, stance: petDefensive}
	now := r.at(30 * time.Second)
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	// no one of its side hurt: not wanted (the far mate, the stranger and the duelist are)
	r.p.MaxHP, r.p.HP = 4000, 4000
	mate.HP, matePet.HP = mate.MaxHP, matePet.MaxHP
	if r.w.petWantsLocked(ps, tide, now) {
		t.Fatal("the tide is wanted with no one of its side in reach hurt")
	}
	r.p.HP, mate.HP, matePet.HP, pet.HP = 3000, 3950, 1000, pet.MaxHP-500
	if !r.w.petWantsLocked(ps, tide, now) {
		t.Fatal("the tide is not wanted with its owner hurt")
	}
	if !r.w.petCastLocked(ps, 6, nil, now) {
		t.Fatal("the tide was not cast")
	}
	r.w.petCastSystemLocked(now.Add(castLand))
	for _, c := range []struct {
		name      string
		got, want int
	}{
		{"its owner", r.p.HP, 3150}, {"a group mate missing 50", mate.HP, 4000}, {"the mate's pet", matePet.HP, 1150},
		{"itself", pet.HP, pet.MaxHP - 350}, {"a group mate 40 off", far.HP, 3000}, {"a stranger", stranger.HP, 3000},
		{"a group mate in a duel", duelist.HP, 3000},
	} {
		if c.got != c.want {
			t.Errorf("%s: %d hit points after the tide, want %d", c.name, c.got, c.want)
		}
	}
}

// Its tide is cast every 6 s while its owner is hurt, in a fight and out of
// one, its other heals and its nukes as they were.
func TestTheGhilanNoblesTideComesEverySixSeconds(t *testing.T) {
	r := pets1110Rig(t)
	r.p.Effects = append(r.p.Effects, &ActiveEffect{SpellID: 902167, Name: "Ghilan's Blessing", Until: r.at(time.Hour)})
	hurt := func(pet *NPC) { r.p.MaxHP, r.p.HP = 100000, 90000 } // over 60%: neither of its other heals
	f := swarmFightFor(t, r, summonGhilanNoble, 4650, 60*time.Second, hurt)
	at, ms := f.of(fx1110Tide)
	if len(at) < 8 || len(at) > 10 {
		t.Fatalf("the tide cast %d times in 60 s (%v): want every 6 s", len(at), at)
	}
	for i := 1; i < len(at); i++ {
		if d := at[i] - at[i-1]; d < 6000 || d > 9000 {
			t.Errorf("tides %d ms apart (%v): want 6 s, a cast of its own in the way at most", d, at)
			break
		}
	}
	for _, m := range ms {
		if m != 0 {
			t.Errorf("the tide's cast time %d ms, want instant", m)
			break
		}
	}
	if got := r.p.HP - 90000; got != 150*landedBy(at, 0, 9700+60000) {
		t.Errorf("its owner healed %d by %d tides landed", got, landedBy(at, 0, 9700+60000))
	}
	if n := f.logged(`lands "Ghilan's Chill" on Desert Crab for 1000 before`); n == 0 {
		t.Error("its chill never landed with the tide in its kit")
	}
}
