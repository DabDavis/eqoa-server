package entity

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// balancesim.go — one fight at a time, fought out by the World's own
// systems, for tuning the game's balance (balance.go) and holding it to its
// targets (balancesim_test.go; cmd/balancesim prints the tables).
//
// A player of a class, level and kit meets an NPC made as the server makes
// one (NPCFromRecord: its hit points, class and swing), and the fight runs
// tick by tick through aiSystem, attackSystem, petSystem, castSystem,
// effectSystem, regenSystem and npcRegenSystem: the rounds (rounds.go),
// Sandstorm's rolls (offence.go, defence.go), the spells as their lines
// read (cast.go), the pets (pet.go), the regeneration (regen.go,
// npcregen.go) and the balance's scaling,
// with the dice seeded. What is the simulator's own is only the player's
// hands (simPlay): the attack button held, which spell it casts and when.
//
// It swaps the package's dice and silences the log while it runs: a tool's
// and a test's, never a running server's.

// SimPlayer is the player a fight is fought with.
type SimPlayer struct {
	Class, Race, Level int
	// Base is its base stats, STR..CHA, before what it wears
	Base [7]int
	Gear Gear
	// Worn is the equip slots worn (14 main hand, 15 off hand, 16 both
	// hands, 17 shield, 18 ranged, 19 and 20 held), for its swing
	// (GetSwing), and AttackType the swinging item's (1-6 melee), 0 for
	// bare hands
	Worn       [21]bool
	AttackType int
	// CMs is what its masteries give, by the mastery window's slots
	CMs [CMSlots]int
	// Spells is its book: what it may cast
	Spells []CastSpell
	Play   SimPlay
}

// SimPlay is how the simulated player fights.
type SimPlay struct {
	// Melee: it holds the attack button from the start
	Melee bool
	// Buffs: before the fight it casts every lasting beneficial spell of its
	// book on itself (each its best of a line is not looked for: all of
	// them), then rests to full
	Buffs bool
	// Spells: in the fight it casts its harmful spells at the NPC as they
	// come up (the most damage a second of lockout first), and heals itself
	// below HealAt of its hit points (0: half)
	Spells bool
	HealAt float64
	// Pet: before the fight it summons the highest pet of its book, which
	// takes the NPC on first; its owner stands off and casts (Melee is then
	// the owner's too)
	Pet bool
	// Root: it fights as a rooting caster does (root.go's reason): from
	// simRootRange off it opens with its highest root, and whenever the NPC
	// is free (resisted, broken, worn off) it roots it again before anything
	// else; once the NPC is held it steps back to simRootRange (the walk's
	// time spent casting nothing) and casts its spells from there, none of
	// more than simRootLongCast
	Root bool
	// Charm: before the fight it charms a creature CharmDiff levels from its
	// own with the best charm of its book that takes that level (a
	// necromancer's undead charm a skeleton; a resisted one is let be and
	// another tried), which then fights for it as a summoned pet does (Pet's
	// play): the creature's own numbers, as charm.go keeps them
	Charm     bool
	CharmDiff int
}

// SimNPC is the NPC: a row as the server spawns it.
type SimNPC struct {
	Name   string
	Level  int
	Class  int // npcclass's: 0 a warrior's swing
	Weapon bool
	Shield bool
}

// SimResult is how a fight went.
type SimResult struct {
	Won, Died       bool // the NPC died; the player did
	Time            time.Duration
	HP, MaxHP       int
	Power, MaxPower int
	NPCHP, NPCMaxHP int
	// HadPet: a pet fought; PetDied: it died; its hit points at the end
	HadPet, PetDied bool
	PetHP, PetMaxHP int
	PetLevel        int
	Casts           int
}

// HPShare is the player's hit points left, a share of its most.
func (r SimResult) HPShare() float64 {
	if r.MaxHP <= 0 {
		return 0
	}
	return float64(r.HP) / float64(r.MaxHP)
}

const (
	simTick      = 100 * time.Millisecond
	simMe        = 0x00AA0001 // the player's object id
	simFoeID     = 0x00BB0001 // the NPC's on the player's channel 0
	simCharmID   = 0x00CC0001 // a creature to charm, on the player's channel 1
	simPlayer    = EntityID(1001)
	simFoe       = EntityID(5001)
	simApart     = 2.0  // how far apart a melee fight starts
	simOwnerAt   = 7.0  // how far off its NPC a pet's owner stands
	simRootRange = 18.0 // how far off its NPC a rooting caster stands
	// simRootLongCast is the longest cast a rooting caster starts: one longer
	// (a wizard's Phoenix Glory, 15 s) would hold its hands through a break
	simRootLongCast = 5 * time.Second
)

// simLogTo is where a simulated fight's log goes (a test's, to read one
// fight by); nil: nowhere.
var simLogTo io.Writer

// simMu keeps one simulation at a time: the dice and the log are the
// package's.
var simMu sync.Mutex

// SimulateFight fights one fight under the balance (nil: none), the dice
// seeded with seed, for at most limit (a fight still on then is neither won
// nor lost).
func SimulateFight(pl SimPlayer, foe SimNPC, b *Balance, seed int64, limit time.Duration) SimResult {
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

	f := newSimFight(pl, foe, b)
	return f.run(limit)
}

// simDice puts the simulator's seeded dice in every roll a fight makes, and
// gives back what puts the package's own back.
func simDice(rng *rand.Rand) func() {
	melee, spell, proc, tap, charm, resist, root := meleeIntn, spellIntn, procIntn, petTapIntn, charmIntn, resistRand, rootIntn
	meleeIntn, spellIntn, procIntn, petTapIntn, charmIntn, resistRand, rootIntn = rng.Intn, rng.Intn, rng.Intn, rng.Intn, rng.Intn, rng.Float64, rng.Intn
	return func() {
		meleeIntn, spellIntn, procIntn, petTapIntn, charmIntn, resistRand, rootIntn = melee, spell, proc, tap, charm, resist, root
	}
}

// simFight is one fight's world.
type simFight struct {
	w    *World
	sess *session.Session
	p    *Player
	npc  *NPC
	pl   SimPlayer
	now  time.Time
	t0   time.Time

	nukes, heals, petHeals []CastSpell
	roots                  []CastSpell // the book's roots, the highest first (SimPlay.Root)
	reserve                int         // power kept for a heal
	casts                  int
	moveUntil              time.Time // a rooting caster stepping back: no cast till then
	rootLands              time.Time // a rooting caster's root in flight lands then
}

func newSimFight(pl SimPlayer, foe SimNPC, b *Balance) *simFight {
	w := NewWorld()
	w.balance = b
	mgr := session.NewManager()
	w.SessionManager = mgr
	sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 9, 9, 9), Port: 7000}, 1, 1, 1, 2, true)
	p := &Player{ServerID: simPlayer, Name: "Simmer", Class: pl.Class, Race: pl.Race, Level: pl.Level,
		Speed: 8.833333, X: 10000, Y: 0, Z: 10000, World: 0}
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

	name := foe.Name
	if name == "" {
		name = "a sim creature"
	}
	apart := float32(simApart)
	if pl.Play.Root {
		apart = simRootRange
	}
	rec := &database.NPCRecord{ID: int(simFoe), NPCName: name, NPCLevel: foe.Level, NPCType: 0x0100,
		XCoord: p.X + apart, YCoord: p.Y, ZCoord: p.Z, Size: 1}
	if foe.Weapon {
		rec.PrimaryWeapon = 1
	}
	if foe.Shield {
		rec.Shield = 1
	}
	npc := NPCFromRecord(rec, foe.Class, nil)
	w.applyNPCSettingsLocked(npc) // as LoadNPCs makes it: the balance's hit points (balance.go)
	w.NPCs[npc.ServerID] = npc
	w.SpatialIndex.Insert(npc.ServerID, npc.X, npc.Z)
	vs := GetOrCreateVisibility(sess)
	vs.ChannelEntity[0] = npc.ServerID
	vs.ChannelObjectID[0] = simFoeID

	f := &simFight{w: w, sess: sess, p: p, npc: npc, pl: pl, t0: time.Unix(1_900_000_000, 0)}
	f.sortBook()
	return f
}

// tick is one of the World's ticks, the systems a fight lives in, in the
// tick's order (systems.go tickLocked).
func (f *simFight) tick() {
	f.now = f.now.Add(simTick)
	w := f.w
	w.mu.Lock()
	w.TickCount++
	w.aiSystem(f.now)
	w.homeSystem(f.now)
	w.attackSystem(f.now)
	w.petSystem(f.now)
	w.castSystem(f.now)
	w.effectSystem(f.now)
	w.regenSystem(f.now)
	w.npcRegenSystem(f.now) // pets', and a creature's out of its fight (npcregen.go)
	w.mu.Unlock()
	f.sess.Queue().GetPending() // what was sent: no one reads it
}

func (f *simFight) run(limit time.Duration) SimResult {
	w, p, npc := f.w, f.p, f.npc
	// before the fight: its buffs and its pet, then a rest to full, timed
	// to end as the fight begins (a two-minute buff is still on)
	f.now = f.t0.Add(-f.prepTime() - time.Second)
	if f.pl.Play.Buffs {
		f.castBuffs()
	}
	var pet *NPC
	switch {
	case f.pl.Play.Charm:
		pet = f.charm()
	case f.pl.Play.Pet:
		pet = f.summon()
	}
	for f.now.Before(f.t0) {
		f.tick()
	}
	w.mu.Lock()
	p.HP, p.Power = p.MaxHPNow(), p.MaxPowerNow()
	if pet != nil {
		pet.HP = pet.MaxHP
		// the pet beside the NPC; an owner that does not fight hand to hand
		// stands off
		ox, oz := pet.X, pet.Z
		pet.X, pet.Y, pet.Z = npc.X+3, npc.Y, npc.Z
		w.SpatialIndex.Update(pet.ServerID, ox, oz, pet.X, pet.Z)
		if !f.pl.Play.Melee {
			ox, oz = p.X, p.Z
			p.X = npc.X + simOwnerAt
			w.SpatialIndex.Update(p.ServerID, ox, oz, p.X, p.Z)
		}
	}
	w.mu.Unlock()

	// the pull: the NPC fights whoever opens (a rooting caster's root is
	// its pull)
	opener := p.ServerID
	if pet != nil {
		opener = pet.ServerID
		w.PetCommand(f.sess, petCmdAttack, simFoeID, f.now)
	}
	if !f.pl.Play.Root {
		w.mu.Lock()
		w.addThreatLocked(npc, opener, 1)
		w.mu.Unlock()
	}
	if f.pl.Play.Melee {
		f.engage()
	}
	maxHP := npc.MaxHP
	for f.now.Sub(f.t0) < limit {
		f.tick()
		w.mu.RLock()
		done := npc.Dead || p.IsDead()
		w.mu.RUnlock()
		if done {
			break
		}
		f.play(pet)
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	r := SimResult{Won: npc.Dead, Died: p.IsDead(), Time: f.now.Sub(f.t0), HP: p.HP, MaxHP: p.MaxHPNow(),
		Power: p.Power, MaxPower: p.MaxPowerNow(), NPCHP: npc.HP, NPCMaxHP: maxHP, Casts: f.casts}
	if pet != nil {
		r.HadPet, r.PetDied, r.PetHP, r.PetMaxHP, r.PetLevel = true, pet.Dead || pet.HP <= 0, pet.HP, pet.MaxHP, pet.Level
	}
	return r
}

// SimReset is a fight broken off (SimulateReset): the NPC's and the
// player's shares of their hit points then, how long the NPC took to be
// whole again (0: never in the time given), and the player's share then.
type SimReset struct {
	Ended                bool // the fight ended (a death) before the break
	NPCShare, HPShare    float64
	Whole                time.Duration
	HPShareWhenNPCWhole  float64
	PowerShareWhenWhole  float64
	PowerShareAtTheBreak float64
}

// SimulateReset fights as SimulateFight for after, then the player runs
// out of the NPC's reach for good (past npcGiveUp: it gives up, goes home
// and heals out of its fight, npcregen.go) and rests; it reports how long
// the NPC took to be whole again, at most limit.
func SimulateReset(pl SimPlayer, foe SimNPC, b *Balance, seed int64, after, limit time.Duration) SimReset {
	simMu.Lock()
	defer simMu.Unlock()
	rng := rand.New(rand.NewSource(seed))
	restore := simDice(rng)
	defer restore()
	out := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(out)

	f := newSimFight(pl, foe, b)
	f.run(after)
	w, p, npc := f.w, f.p, f.npc
	w.mu.Lock()
	r := SimReset{Ended: npc.Dead || p.IsDead(), NPCShare: float64(npc.HP) / float64(max(npc.MaxHP, 1)),
		HPShare: float64(p.HP) / float64(max(p.MaxHPNow(), 1)), PowerShareAtTheBreak: float64(p.Power) / float64(max(p.MaxPowerNow(), 1))}
	if r.Ended {
		w.mu.Unlock()
		return r
	}
	p.disengage()
	p.AutoAttack = false
	ox, oz := p.X, p.Z
	p.X += 2 * npcGiveUp
	w.SpatialIndex.Update(p.ServerID, ox, oz, p.X, p.Z)
	w.mu.Unlock()
	start := f.now
	for f.now.Sub(start) < limit {
		f.tick()
		w.mu.RLock()
		whole := npc.HP >= npc.MaxHP
		w.mu.RUnlock()
		if whole {
			r.Whole = f.now.Sub(start)
			break
		}
	}
	w.mu.RLock()
	r.HPShareWhenNPCWhole = float64(p.HP) / float64(max(p.MaxHPNow(), 1))
	r.PowerShareWhenWhole = float64(p.Power) / float64(max(p.MaxPowerNow(), 1))
	w.mu.RUnlock()
	return r
}

// prepTime is how long the buffs and the pet take to cast before the fight.
func (f *simFight) prepTime() time.Duration {
	var d time.Duration
	if f.pl.Play.Buffs {
		for _, s := range f.buffs() {
			d += s.CastTime + castLand + castBusy + simTick
		}
	}
	if f.pl.Play.Pet {
		var pet time.Duration
		for i := range f.pl.Spells {
			if s := &f.pl.Spells[i]; simRole(s) == roleSummon {
				pet = max(pet, s.CastTime+castLand+castBusy+simTick)
			}
		}
		d += pet
	}
	if f.pl.Play.Charm {
		if s := f.charmSpell(); s != nil {
			d += s.CastTime + castLand + castBusy + simTick
		}
	}
	return d
}

// charmSpell is the book's best charm for a creature of CharmDiff levels from
// the player: the highest whose cap takes it, nil for none.
func (f *simFight) charmSpell() *CastSpell {
	level := max(f.p.Level+f.pl.Play.CharmDiff, 1)
	var best *CastSpell
	for i := range f.pl.Spells {
		s := &f.pl.Spells[i]
		if s.Def.Charms() && s.Level <= f.p.Level && charmMaxLevel(s) >= level && (best == nil || s.Level > best.Level) {
			best = s
		}
	}
	return best
}

// SimCharmable is whether the kit's book has a charm that takes a creature
// diff levels from its own.
func SimCharmable(pl SimPlayer, diff int) bool {
	f := &simFight{p: &Player{Level: pl.Level}, pl: pl}
	f.pl.Play.CharmDiff = diff
	return f.charmSpell() != nil
}

// charm charms a creature of CharmDiff levels from the player beside it
// (SimPlay.Charm) and gives back the pet it makes: nil when no charm takes
// it or eight in a row are resisted.
func (f *simFight) charm() *NPC {
	best := f.charmSpell()
	if best == nil {
		return nil
	}
	w, p := f.w, f.p
	name := "a sim beast"
	if best.Def.undeadOnly() {
		name = "a sim skeleton" // undead by its name (undead.go)
	}
	level := max(p.Level+f.pl.Play.CharmDiff, 1)
	vs := GetOrCreateVisibility(f.sess)
	for try := range 8 {
		id := EntityID(6001 + try)
		rec := &database.NPCRecord{ID: int(id), NPCName: name, NPCLevel: level, NPCType: 0x0100,
			XCoord: p.X - 3, YCoord: p.Y, ZCoord: p.Z, Size: 1}
		w.mu.Lock()
		c := NPCFromRecord(rec, 0, nil)
		w.applyNPCSettingsLocked(c)
		w.NPCs[c.ServerID] = c
		w.SpatialIndex.Insert(c.ServerID, c.X, c.Z)
		vs.ChannelEntity[1], vs.ChannelObjectID[1] = c.ServerID, simCharmID+uint32(try)
		p.Power = p.MaxPowerNow()
		w.mu.Unlock()
		if !w.StartCast(f.sess, *best, simCharmID+uint32(try), f.now) {
			return nil
		}
		for range int((best.CastTime + castLand + castBusy) / simTick) {
			f.tick()
		}
		w.mu.Lock()
		pet := w.petOfLocked(p)
		if pet == nil {
			// resisted: it comes at its caster; let it be (gone) and try
			// another, the charm's lockout waited out (time not counted)
			w.dropAggroLocked(c.ServerID)
			w.removeNPCLocked(c)
			p.HP = p.MaxHPNow()
			p.Lockouts = nil
		}
		w.mu.Unlock()
		if pet != nil {
			return pet
		}
	}
	return nil
}

// engage presses the attack button at the NPC, and holds it.
func (f *simFight) engage() {
	swing := SwingFor(f.p.Class, f.p.Level, f.pl.Worn, f.pl.AttackType != 0, f.pl.AttackType, 0)
	f.w.EngageAttack(f.sess, simFoeID, swing, f.now)
	f.w.mu.Lock()
	f.p.AutoAttack = true
	f.w.mu.Unlock()
}

// spellRole is what the simulated player uses a spell for.
type spellRole int

const (
	roleNone spellRole = iota
	roleNuke
	roleHeal
	rolePetHeal
	roleBuff
	roleSummon
)

// simRole is what a spell is to the simulator: a harmful spell at a target
// that does damage is a nuke (a damage over time one too, a life tap); a
// beneficial one that gives hit points a heal (scope 3: the pet's); a
// lasting beneficial one with no form, gate, summoning, invisibility or
// price a buff; a summoning a pet's.
func simRole(s *CastSpell) spellRole {
	d := &s.Def
	if d.Summons() {
		return roleSummon
	}
	if d.Charms() || d.FormKind != FormNone || d.Resurrects() > 0 || d.UndeadOnly {
		return roleNone
	}
	var damage, heal, lasting, bad bool
	for _, e := range d.Effects {
		switch e.Kind {
		case EffectHP, EffectHPTick:
			if e.Self && e.Amount < 0 {
				bad = true // a pact's price
			} else if e.Amount < 0 {
				damage = true
			} else if e.Amount > 0 {
				heal = true
			}
		case EffectGate, EffectInvis, EffectPowerCap, EffectIllusion, EffectEndIllusion:
			bad = true
		case EffectStat, EffectShield, EffectDefProc, EffectProc:
			lasting = true
		}
	}
	switch {
	case d.Harmful:
		if damage && s.Scope == 1 {
			return roleNuke
		}
	case bad:
	case heal && s.Scope == 3:
		return rolePetHeal
	case heal && s.Scope != 3 && (d.Duration <= 0 || !lasting):
		return roleHeal
	case lasting && d.Duration >= 30*time.Second && s.Scope != 3:
		return roleBuff
	}
	return roleNone
}

// simDamage is what a cast of the spell does to its target from this
// caster, on average: its hit, and its ticks over its duration.
func simDamage(s *CastSpell, p *Player) float64 {
	var n float64
	for _, e := range s.Def.Effects {
		if e.Self || e.Amount >= 0 || (e.Kind != EffectHP && e.Kind != EffectHPTick) {
			continue
		}
		v := -float64(e.Amount+e.AmountMax) / 2
		if e.Scale != StatNone && e.ScalePct != 0 {
			v += float64(p.StatNow(e.Scale) * e.ScalePct / 100)
		}
		if e.Cap > 0 {
			v = min(v, float64(e.Cap))
		}
		if e.Kind == EffectHPTick {
			v *= float64(simTicks(s))
		}
		n += v
	}
	return n
}

// simHeal is what a cast heals, on average, as simDamage.
func simHeal(s *CastSpell, p *Player) float64 {
	var n float64
	for _, e := range s.Def.Effects {
		if e.Self || e.Amount <= 0 || (e.Kind != EffectHP && e.Kind != EffectHPTick) {
			continue
		}
		v := float64(e.Amount+e.AmountMax) / 2
		if e.Scale != StatNone && e.ScalePct != 0 {
			v += float64(p.StatNow(e.Scale) * e.ScalePct / 100)
		}
		if e.Kind == EffectHPTick {
			v *= float64(simTicks(s))
		}
		n += v
	}
	return n
}

// simTicks is how many ticks a spell's per-tick part lands.
func simTicks(s *CastSpell) int {
	if s.Def.Ticks > 0 {
		return s.Def.Ticks
	}
	dur := s.Def.Duration
	if dur <= 0 {
		dur = tickDuration
	}
	return max(int(dur/effectTick), 1)
}

// lockoutOf is how long a cast keeps the spell from being cast again.
func lockoutOf(s *CastSpell) time.Duration {
	return s.CastTime + s.Recast + castLockoutExtra
}

// sortBook sorts the book into what the player's hands use: the nukes by
// damage a second of their lockout, the heals by what they heal.
func (f *simFight) sortBook() {
	p := f.p
	for _, s := range f.pl.Spells {
		switch simRole(&s) {
		case roleNuke:
			f.nukes = append(f.nukes, s)
		case roleHeal:
			f.heals = append(f.heals, s)
		case rolePetHeal:
			f.petHeals = append(f.petHeals, s)
		}
	}
	for _, s := range f.pl.Spells {
		if simRoots(&s) {
			f.roots = append(f.roots, s)
		}
	}
	sort.SliceStable(f.roots, func(i, j int) bool { return f.roots[i].Level > f.roots[j].Level })
	rate := func(s *CastSpell) float64 { return simDamage(s, p) / lockoutOf(s).Seconds() }
	sort.SliceStable(f.nukes, func(i, j int) bool { return rate(&f.nukes[i]) > rate(&f.nukes[j]) })
	sort.SliceStable(f.heals, func(i, j int) bool { return simHeal(&f.heals[i], p) > simHeal(&f.heals[j], p) })
	sort.SliceStable(f.petHeals, func(i, j int) bool { return simHeal(&f.petHeals[i], p) > simHeal(&f.petHeals[j], p) })
	if len(f.heals) > 0 {
		f.reserve = f.heals[0].PowerCost
	}
}

// simBuffLine is a buff's line as the server takes it (buffline.go: only
// the best of a line counts), else what its effects change.
func simBuffLine(s *CastSpell) string {
	if l := BuffLine(s); l != "" {
		return l
	}
	var keys []string
	for _, e := range s.Def.Effects {
		keys = append(keys, fmt.Sprintf("%d.%d", e.Kind, e.Stat))
	}
	sort.Strings(keys)
	return fmt.Sprint(s.Scope == 3, keys)
}

// SimBuffLines is a book's buff lines (BuffLine) and the spells of each, by
// level: what only the best of counts.
func SimBuffLines(book []CastSpell) map[string][]CastSpell {
	out := map[string][]CastSpell{}
	for _, s := range book {
		if l := BuffLine(&s); l != "" {
			out[l] = append(out[l], s)
		}
	}
	for _, ss := range out {
		sort.SliceStable(ss, func(i, j int) bool { return ss[i].Level < ss[j].Level })
	}
	return out
}

// buffs is the book's buffs, the highest of each line.
func (f *simFight) buffs() []CastSpell {
	best := map[string]int{}
	var out []CastSpell
	for _, s := range f.pl.Spells {
		if simRole(&s) != roleBuff {
			continue
		}
		k := simBuffLine(&s)
		if i, ok := best[k]; ok {
			if s.Level >= out[i].Level {
				out[i] = s
			}
			continue
		}
		best[k] = len(out)
		out = append(out, s)
	}
	return out
}

// castBuffs casts the buffs on the player, one after another.
func (f *simFight) castBuffs() {
	for _, s := range f.buffs() {
		f.w.mu.Lock()
		f.p.Power = f.p.MaxPowerNow()
		f.w.mu.Unlock()
		target := uint32(0xFFFFFFFF)
		if s.Scope == 1 {
			target = simMe
		}
		if f.w.StartCast(f.sess, s, target, f.now) {
			for range int((s.CastTime + castLand + castBusy) / simTick) {
				f.tick()
			}
		}
	}
}

// summon casts the highest summoning of the book and gives back its pet.
func (f *simFight) summon() *NPC {
	var best *CastSpell
	for i := range f.pl.Spells {
		s := &f.pl.Spells[i]
		if simRole(s) == roleSummon && s.Level <= f.p.Level && (best == nil || s.Level > best.Level) {
			best = s
		}
	}
	if best == nil {
		return nil
	}
	// a body for it to wear: the creature it names, its class's stand-in
	f.w.mu.Lock()
	for i, name := range []string{best.Name, petSpellName(best.Name), best.Def.Pet, petClassStandIn[f.p.Class]} {
		if name == "" {
			continue
		}
		id := EntityID(7000 + i)
		f.w.NPCs[id] = &NPC{ServerID: id, Name: name, Level: 1, ModelID: 0x1BBB898C, Size: 1, NPCType: 0x80, X: 40000, Z: 40000}
	}
	f.p.Power = f.p.MaxPowerNow()
	f.w.mu.Unlock()
	if !f.w.StartCast(f.sess, *best, 0xFFFFFFFF, f.now) {
		return nil
	}
	for range int((best.CastTime + castLand + castBusy) / simTick) {
		f.tick()
	}
	f.w.mu.RLock()
	defer f.w.mu.RUnlock()
	return f.w.petOfLocked(f.p)
}

// ready is whether the spell is out of its lockout and paid for, with
// keep power left over.
func (f *simFight) ready(s *CastSpell, keep int) bool {
	p := f.p
	if p.Power < s.PowerCost+keep {
		return false
	}
	for _, l := range p.Lockouts {
		if l.spellID == s.SpellID && f.now.Before(l.until) {
			return false
		}
	}
	return true
}

// onFoe is whether the spell's lasting part is still on the NPC.
func (f *simFight) onFoe(s *CastSpell) bool {
	for _, ae := range f.npc.Effects {
		if ae.SpellID == s.SpellID && f.now.Before(ae.Until) {
			return true
		}
	}
	return false
}

// play is the player's hands for a tick: a heal when low, else its best
// nuke that is up, else nothing (the attack button stays held).
func (f *simFight) play(pet *NPC) {
	if !f.pl.Play.Spells {
		return
	}
	w, p := f.w, f.p
	w.mu.RLock()
	busy := p.Casting != nil && f.now.Before(p.Casting.ends)
	low := float64(p.HP) <= f.healAt()*float64(p.MaxHPNow())
	petLow := pet != nil && !pet.Dead && pet.HP*2 <= pet.MaxHP
	w.mu.RUnlock()
	if busy {
		return
	}
	cast := func(s CastSpell, target uint32) bool {
		if w.StartCast(f.sess, s, target, f.now) {
			f.casts++
			return true
		}
		return false
	}
	if low {
		for _, s := range f.heals {
			w.mu.RLock()
			ok := f.ready(&s, 0)
			w.mu.RUnlock()
			if ok && cast(s, simMe) {
				return
			}
		}
	}
	if petLow {
		for _, s := range f.petHeals {
			w.mu.RLock()
			ok := f.ready(&s, 0)
			w.mu.RUnlock()
			if ok && cast(s, 0xFFFFFFFF) {
				return
			}
		}
	}
	if pet != nil && f.now.Sub(f.t0) < 3*time.Second {
		return // the pet's hate first
	}
	if f.pl.Play.Root && f.rootPlay(cast) {
		return
	}
	for _, s := range f.nukes {
		if f.pl.Play.Root && s.CastTime > simRootLongCast {
			continue // a rooting caster keeps its hands free for the next root
		}
		w.mu.RLock()
		ok := f.ready(&s, f.reserve) && !f.onFoe(&s) && f.keepsHate(pet, &s)
		w.mu.RUnlock()
		if ok && cast(s, simFoeID) {
			return
		}
	}
}

// simRoots is whether a spell roots its target: cast at one, its line's
// root.
func simRoots(s *CastSpell) bool {
	if !s.Def.Harmful || s.Scope != 1 {
		return false
	}
	for _, e := range s.Def.Effects {
		if e.Kind == EffectRoot {
			return true
		}
	}
	return false
}

// SimHasRoot is whether the kit's book has a root.
func SimHasRoot(pl SimPlayer) bool {
	for i := range pl.Spells {
		if simRoots(&pl.Spells[i]) {
			return true
		}
	}
	return false
}

// rootPlay is a rooting caster's hands for a tick (SimPlay.Root): it waits
// out its step back, roots a free NPC, and steps back from a held one that
// is near. It reports whether its hands were busy.
func (f *simFight) rootPlay(cast func(CastSpell, uint32) bool) bool {
	w, p, npc := f.w, f.p, f.npc
	if f.now.Before(f.moveUntil) {
		return true
	}
	w.mu.RLock()
	rooted := npc.rooted(f.now)
	d := distXZ(p.X, p.Z, npc.X, npc.Z)
	w.mu.RUnlock()
	if !rooted {
		if f.now.Before(f.rootLands) {
			return false // one on its way
		}
		for _, s := range f.roots {
			w.mu.RLock()
			ok := f.ready(&s, 0)
			w.mu.RUnlock()
			if ok && cast(s, simFoeID) {
				f.rootLands = f.now.Add(s.CastTime + castLand)
				return true
			}
		}
		return false // no root up: its spells, the NPC on its way
	}
	if d >= simRootRange-1 {
		return false
	}
	// held and near: back to simRootRange off it, at its run
	w.mu.Lock()
	ox, oz := p.X, p.Z
	if d < 0.01 {
		p.X = npc.X + simRootRange
	} else {
		p.X = npc.X + (p.X-npc.X)/d*simRootRange
		p.Z = npc.Z + (p.Z-npc.Z)/d*simRootRange
	}
	w.SpatialIndex.Update(p.ServerID, ox, oz, p.X, p.Z)
	speed := float64(p.SpeedNow())
	w.mu.Unlock()
	f.moveUntil = f.now.Add(time.Duration(float64(simRootRange-d) / max(speed, 1) * float64(time.Second)))
	return true
}

// keepsHate is whether a cast leaves its pet the NPC's foe, as a pet's owner
// plays (its hate under four fifths of the pet's after the spell's damage):
// a pet class lets its pet hold the NPC, and its spells wait for the pet's
// hate to build. With no pet alive, any cast does.
func (f *simFight) keepsHate(pet *NPC, s *CastSpell) bool {
	if pet == nil || pet.Dead {
		return true
	}
	mine := float64(f.npc.Threats[f.p.ServerID]) + simDamage(s, f.p)
	return mine <= 0.8*float64(f.npc.Threats[pet.ServerID])
}

func (f *simFight) healAt() float64 {
	if f.pl.Play.HealAt > 0 {
		return f.pl.Play.HealAt
	}
	return 0.5
}

// SimCell is the fights of one kit against NPCs Diff levels above it.
type SimCell struct {
	Class, Level, Diff int
	Pet                bool
	Fights, Won, Died  int
	// HPLeft is the mean share of hit points left in the fights won, Secs
	// the mean length of those fights
	HPLeft, Secs float64
	PetDied      int
}

// WinShare is the share of the fights won.
func (c SimCell) WinShare() float64 {
	if c.Fights == 0 {
		return 0
	}
	return float64(c.Won) / float64(c.Fights)
}

// SimCellOf fights the kit against an NPC of its level plus diff, fights
// times, the seeds counting up from seed.
func SimCellOf(pl SimPlayer, foe SimNPC, diff int, b *Balance, fights int, seed int64, limit time.Duration) SimCell {
	c := SimCell{Class: pl.Class, Level: pl.Level, Diff: diff, Pet: pl.Play.Pet, Fights: fights}
	foe.Level = max(pl.Level+diff, 1)
	for i := range fights {
		r := SimulateFight(pl, foe, b, seed+int64(i), limit)
		if r.Won {
			c.Won++
			c.HPLeft += r.HPShare()
			c.Secs += r.Time.Seconds()
		}
		if r.Died {
			c.Died++
		}
		if r.PetDied {
			c.PetDied++
		}
	}
	if c.Won > 0 {
		c.HPLeft /= float64(c.Won)
		c.Secs /= float64(c.Won)
	}
	return c
}

// ArchetypeName is a class's archetype as the defence and offence tables
// have it: tank, melee, priest or caster.
func ArchetypeName(class int) string {
	return [...]string{archTank: "tank", archMelee: "melee", archPriest: "priest", archCaster: "caster"}[archetypeOf(class)]
}

// SimHasPet is whether the kit's book has a summoning.
func SimHasPet(pl SimPlayer) bool {
	for i := range pl.Spells {
		if simRole(&pl.Spells[i]) == roleSummon {
			return true
		}
	}
	return false
}
