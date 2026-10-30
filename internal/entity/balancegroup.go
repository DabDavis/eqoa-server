package entity

import (
	"io"
	"log"
	"math/rand"
	"net"
	"time"
)

// balancegroup.go — a group's fight in the fight simulator (balancesim.go):
// two or three players of a level against one NPC, for the user's red target
// of 2026-10-30 ("Red (+3 and up): needs a group. A solo player mostly
// loses; a fitting group (2-3 of the level) wins").
//
// Each member is a player as SimulateFight makes one, with its own session,
// in one group (its group spells land on the others), fighting as its
// SimPlay says: hand to hand from simApart off the NPC, or, not melee, from
// simOwnerAt off; a heal goes to the member lowest on hit points below the
// healer's HealAt, itself included; a pet class's pet fights beside it. The
// first member pulls (its pet, if it has one), and the NPC fights whoever
// its hate makes its enemy, as on the server (addThreatLocked). Before the
// fight each member casts its buffs in turn (a group buff lands on all) and
// all rest to full. Root play and charm are a lone player's.

// SimGroupResult is how a group's fight went.
type SimGroupResult struct {
	Won, Wiped bool // the NPC died; every member did
	Deaths     int
	Time       time.Duration
	// HPShare is the members' mean share of their hit points at the end, a
	// dead member's 0
	HPShare float64
}

// simMateID is the object id member j has on the others' channels 2+j.
const simMateID = 0x00AC0000

// SimulateGroupFight fights one fight of the group (pls, the first the
// puller) against the NPC under the balance, the dice seeded, for at most
// limit.
func SimulateGroupFight(pls []SimPlayer, foe SimNPC, b *Balance, seed int64, limit time.Duration) SimGroupResult {
	simMu.Lock()
	defer simMu.Unlock()
	rng := rand.New(rand.NewSource(seed))
	restore := simDice(rng)
	defer restore()
	out := log.Writer()
	if simLogTo != nil {
		log.SetOutput(simLogTo)
	} else {
		log.SetOutput(io.Discard)
	}
	defer log.SetOutput(out)

	ms := newSimGroup(pls, foe, b)
	return runSimGroup(ms, limit)
}

// newSimGroup is the group's world: the first member made as a lone player
// is, the NPC beside it, then the others joined round it and into one group.
func newSimGroup(pls []SimPlayer, foe SimNPC, b *Balance) []*simFight {
	lead := newSimFight(pls[0], foe, b)
	w, npc := lead.w, lead.npc
	ms := []*simFight{lead}
	for i, pl := range pls[1:] {
		k := i + 1
		sess := w.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 9, 9, byte(10+k)), Port: 7000}, 1, 1, 1, 2, true)
		p := &Player{ServerID: simPlayer + EntityID(k), Name: "Simmer" + string(rune('A'+k)), Class: pl.Class, Race: pl.Race,
			Level: pl.Level, Speed: 8.833333, X: lead.p.X, Y: lead.p.Y, Z: lead.p.Z + float32(k), World: 0}
		p.BaseStr, p.BaseSta, p.BaseAgi, p.BaseDex = pl.Base[0], pl.Base[1], pl.Base[2], pl.Base[3]
		p.BaseWis, p.BaseInt, p.BaseCha = pl.Base[4], pl.Base[5], pl.Base[6]
		p.Gear = pl.Gear
		p.CMEffects = pl.CMs
		p.RecalcStats(true)
		w.Players[p.ServerID] = p
		w.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
		sess.SetECSEntity(p)
		sess.SetCharacterInWorld(true)
		sess.SetPlayerObjectID(simMe)
		vs := GetOrCreateVisibility(sess)
		vs.ChannelEntity[0] = npc.ServerID
		vs.ChannelObjectID[0] = simFoeID
		m := &simFight{w: w, sess: sess, p: p, npc: npc, pl: pl, t0: lead.t0}
		m.sortBook()
		ms = append(ms, m)
	}
	g := &Group{ID: 1, lastRoster: map[EntityID]string{}}
	for i, m := range ms {
		m.mates, m.index = ms, i
		m.p.GroupID = g.ID
		g.Members = append(g.Members, m.p.ServerID)
		vs := GetOrCreateVisibility(m.sess)
		for j, o := range ms {
			if j != i {
				vs.ChannelEntity[2+j] = o.p.ServerID
				vs.ChannelObjectID[2+j] = simMateID + uint32(j)
			}
		}
		// in reach of the NPC, or standing off it
		at := float32(simApart)
		if !m.pl.Play.Melee {
			at = simOwnerAt
		}
		ox, oz := m.p.X, m.p.Z
		m.p.X, m.p.Z = npc.X-at, npc.Z+float32(i)*0.6
		w.SpatialIndex.Update(m.p.ServerID, ox, oz, m.p.X, m.p.Z)
	}
	w.groups = map[uint32]*Group{g.ID: g}
	w.nextGroupID = g.ID
	return ms
}

// lowestMate is the member lowest on hit points by share, as this member
// names it (its own object id or another's channel id), and whether it is
// below this member's HealAt. Must be called with the lock held.
func (f *simFight) lowestMate() (uint32, bool) {
	best, share := -1, 2.0
	for j, o := range f.mates {
		if o.p.IsDead() {
			continue
		}
		if s := float64(o.p.HP) / float64(max(o.p.MaxHPNow(), 1)); s < share {
			best, share = j, s
		}
	}
	if best < 0 {
		return simMe, false
	}
	id := uint32(simMe)
	if best != f.index {
		id = simMateID + uint32(best)
	}
	return id, share <= f.healAt()
}

// groupTick is one world tick, every member's clock with it.
func groupTick(ms []*simFight) {
	ms[0].tick()
	for _, m := range ms[1:] {
		m.now = ms[0].now
		m.sess.Queue().GetPending()
	}
}

func runSimGroup(ms []*simFight, limit time.Duration) SimGroupResult {
	lead := ms[0]
	w, npc := lead.w, lead.npc
	var prep time.Duration
	for _, m := range ms {
		prep += m.prepTime()
	}
	lead.now = lead.t0.Add(-prep - time.Second)
	pets := make([]*NPC, len(ms))
	for i, m := range ms {
		for _, o := range ms {
			o.now = lead.now
		}
		if m.pl.Play.Buffs {
			m.castBuffs()
		}
		if m.pl.Play.Pet {
			pets[i] = m.summon()
		}
		lead.now = m.now
	}
	for _, m := range ms {
		m.now = lead.now
	}
	for lead.now.Before(lead.t0) {
		groupTick(ms)
	}
	w.mu.Lock()
	for i, m := range ms {
		m.p.HP, m.p.Power = m.p.MaxHPNow(), m.p.MaxPowerNow()
		if pet := pets[i]; pet != nil {
			pet.HP = pet.MaxHP
			ox, oz := pet.X, pet.Z
			pet.X, pet.Y, pet.Z = npc.X+3, npc.Y, npc.Z+float32(i)*0.6
			w.SpatialIndex.Update(pet.ServerID, ox, oz, pet.X, pet.Z)
		}
	}
	w.mu.Unlock()

	// the pull: the first member, or its pet
	opener := lead.p.ServerID
	if pets[0] != nil {
		opener = pets[0].ServerID
	}
	for i, m := range ms {
		if pets[i] != nil {
			w.PetCommand(m.sess, petCmdAttack, simFoeID, m.now)
		}
	}
	w.mu.Lock()
	w.addThreatLocked(npc, opener, 1)
	w.mu.Unlock()
	for _, m := range ms {
		if m.pl.Play.Melee {
			m.engage()
		}
	}
	for lead.now.Sub(lead.t0) < limit {
		groupTick(ms)
		w.mu.RLock()
		done, alive := npc.Dead, 0
		for _, m := range ms {
			if !m.p.IsDead() {
				alive++
			}
		}
		w.mu.RUnlock()
		if done || alive == 0 {
			break
		}
		for i, m := range ms {
			w.mu.RLock()
			dead := m.p.IsDead()
			w.mu.RUnlock()
			if !dead {
				m.play(pets[i])
			}
		}
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	r := SimGroupResult{Won: npc.Dead, Time: lead.now.Sub(lead.t0)}
	for _, m := range ms {
		if m.p.IsDead() {
			r.Deaths++
			continue
		}
		r.HPShare += float64(m.p.HP) / float64(max(m.p.MaxHPNow(), 1))
	}
	r.HPShare /= float64(len(ms))
	r.Wiped = r.Deaths == len(ms)
	return r
}

// SimGroupCell is a group's fights against NPCs Diff levels above its first
// member: won, wiped, deaths a fight, and in the fights won the members'
// mean share of hit points left and the length.
type SimGroupCell struct {
	Diff                int
	Fights, Won, Wiped  int
	Deaths, HPLeft, Sec float64
}

// WinShare is the share of the fights won.
func (c SimGroupCell) WinShare() float64 {
	if c.Fights == 0 {
		return 0
	}
	return float64(c.Won) / float64(c.Fights)
}

// SimGroupCellOf fights the group against an NPC of its first member's level
// plus diff, fights times, the seeds counting up from seed.
func SimGroupCellOf(pls []SimPlayer, foe SimNPC, diff int, b *Balance, fights int, seed int64, limit time.Duration) SimGroupCell {
	c := SimGroupCell{Diff: diff, Fights: fights}
	foe.Level = max(pls[0].Level+diff, 1)
	for i := range fights {
		r := SimulateGroupFight(pls, foe, b, seed+int64(i), limit)
		c.Deaths += float64(r.Deaths)
		if r.Wiped {
			c.Wiped++
		}
		if r.Won {
			c.Won++
			c.HPLeft += r.HPShare
			c.Sec += r.Time.Seconds()
		}
	}
	c.Deaths /= float64(max(fights, 1))
	if c.Won > 0 {
		c.HPLeft /= float64(c.Won)
		c.Sec /= float64(c.Won)
	}
	return c
}

// SimGroupFor is a fitting group for a class at a level (the user's "a
// fitting group (2-3 of the level)"), its members' kits made by kit: of two,
// the class and a cleric (a warrior with a cleric); of three, a warrior, the
// class and a cleric (a wizard for the class's own place when it is one of
// them). The warrior pulls; a pet class fights with its pet, a caster off
// the NPC, a cleric heals the group at 60%.
func SimGroupFor(class, level, size int, kit func(class int) (SimPlayer, error)) ([]SimPlayer, error) {
	const warrior, cleric, wizard = 0, 9, 13
	var classes []int
	switch {
	case size <= 2 && class == cleric:
		classes = []int{warrior, cleric}
	case size <= 2:
		classes = []int{class, cleric}
	case class == warrior || class == cleric:
		classes = []int{warrior, wizard, cleric}
	default:
		classes = []int{warrior, class, cleric}
	}
	var out []SimPlayer
	for _, c := range classes {
		pl, err := kit(c)
		if err != nil {
			return nil, err
		}
		switch {
		case c == cleric:
			pl.Play.HealAt = 0.6
		case SimHasPet(pl) && archetypeOf(c) == archCaster:
			pl.Play.Pet, pl.Play.Melee = true, false
		case archetypeOf(c) == archCaster:
			pl.Play.Melee = false
		}
		out = append(out, pl)
	}
	return out, nil
}
