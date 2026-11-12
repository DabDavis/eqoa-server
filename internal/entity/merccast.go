package entity

import (
	"log"
	"math"
	"sort"
	"time"
)

// merccast.go — a merc's spells and arts (merc.go): its class's book up to
// its level (the kit's: SimBookFromDB, every spell its levels give a player of
// the class, no mastery's, epic's or master class's), each cast as a player
// casts it: its own cast time, landing castLand after it, its lockout (cast
// time, recast and castLockoutExtra from its start) before it is cast again,
// the arts' global cooldown (balance.go) between two arts, its power paid as
// it begins, its 0x00B1 from the merc to everyone who sees it, its figures a
// player's of the merc's stats and level (cast.go's spellAmount on its body).
//
// Each turn the merc not already casting looks, in order, for:
//   - a heal (mercHealTurnLocked): its team is itself, its owner and its
//     owner's group, the players and their mercs, alive in its world (a raid
//     group's whole roll); a healer heals one below mercHurtPct of its hit
//     points (a heal over time it has not put on it, else a heal; a group heal
//     when two or more are), one below mercEmergencyPct first with its biggest
//     heal; any other class with a heal in its book (a paladin's, a ranger's)
//     only one below mercEmergencyPct. Never a player in a duel, in Mr.
//     Moogie's challenge or flagged for PvP (ownfight.go: no help in a fight
//     that is a player's own);
//   - with no pet of its own out, its summoning (mercpet.go);
//   - its pet's heal when it is hurt, then its pet's buffs, the best of
//     each line kept on it (mercpetbuff.go);
//   - out of a fight, a buff (mercBuffTurnLocked): the best rank of each line
//     its book holds of what lasts mercBuffLasts or more, on itself (a self
//     buff, or one of a target's its owner already has), its owner (one of a
//     target's) or its group (one of the group's that it or its owner lacks),
//     while half its power or more is left; each line's best alone, the kinds
//     of line across (mercBestBuffs), none far beneath its level
//     (mercBuffFloor), and none another merc's cast (or its own) already
//     brings (mercBuffComingLocked: three mercs of one owner buff him once);
//   - a tank's taunt (mercTauntTurnLocked): its target's hate on itself by
//     its book's taunts (a hate line: Taunt, Provoke, the Pillars), the best
//     that is ready, while its target fights another than itself (one of its
//     team, or none yet); and when its target already fights it, the nearest
//     creature fighting another of its team, which it takes on and taunts
//     (the owner: "if tank they taunt"). A target that fights it already is
//     held: no taunt, its power for its attacks (the owner, 2026-11-11: "lets
//     make melee/tanks use their damage abilities also"; until then a tank
//     taunted on every cooldown, and its taunts, Bait to Rage of Zek, ran its
//     power out: a level 60 warrior's 5,650 in a minute, its strikes waiting);
//   - its attacks at its target (mercAttackTurnLocked): its damage over time
//     kept up (cast again only once the one it put on is about to run out),
//     then its biggest hit that is ready. A healer casts them only with no
//     team mate hurt and mercHealerSpare of its power left; a caster keeps its
//     power for its best: a lesser spell only while what is left after it
//     pays for the best one (the owner: "casters keep their power for their
//     best spells (don't spam to empty)"). A caster or a healer below
//     mercThriftyBelow of its power casts the hit that does the most for its
//     power instead (mercThrifty: the owner, 2026-11-11, "magician mercs need
//     to cast spells for damage like the necro").
//
// What a merc's book holds that it never casts (mercSkips): a summoning, a
// charm, an illusion, a gate or a trip, a resurrection, invisibility, a run
// speed, a size, a cure, a stance, a power ceiling, a pickpocket, a lull, a
// call of a player, a reagent's or an item's, a backstab, a spell for animals
// or summoned creatures alone, a harmful area spell (it pulls what its owner
// did not ask for), a pet's and a corpse's spell; and a harmful spell that
// hits nothing (a debuff, a root, a slow alone). A spell for undead alone goes
// only at undead.

// The merc's heal thresholds and spare power (ours: EverQuest's mercenaries
// heal a little below three quarters).
const (
	// mercHurtPct is the share of its hit points below which a healer heals
	// a team mate
	mercHurtPct = 70
	// mercEmergencyPct is the share below which any merc with a heal heals,
	// a healer first with its biggest
	mercEmergencyPct = 40
	// mercHealerSpare is the share of its power a healer keeps for its heals:
	// it casts at its owner's foe only above it (70 until 2026-11-11, when a
	// cleric that had buffed and healed never had it in a fight again: none
	// of its hits in a minute's fight at 30 or 60)
	mercHealerSpare = 40
	// mercBuffSpare: a merc buffs out of a fight only with this share of its
	// power or more
	mercBuffSpare = 50
)

// mercBuffLasts is the least a buff lasts for a merc to keep it up out of a
// fight: a long one (a 30 minute AC buff, a 10 minute skin), not a fight's
// two minutes.
const mercBuffLasts = 5 * time.Minute

// mercUse is what a merc casts a spell for.
type mercUse uint8

const (
	useNone      mercUse = iota
	useNuke              // a hit at a creature: an art or a spell
	useDoT               // damage over time at a creature
	useTaunt             // hate at a creature (its damage too)
	useHeal              // a heal of one
	useHoT               // a heal over time of one
	useGroupHeal         // a heal of the group, now or over time
	useBuff              // what lasts on a friend: itself, one or the group
)

// MercUseName is what a merc casts a spell for, in words ("" for a spell it
// never casts): the handlers' real-data test's.
func MercUseName(s *CastSpell) string {
	return [...]string{useNone: "", useNuke: "hit", useDoT: "damage over time", useTaunt: "taunt", useHeal: "heal",
		useHoT: "heal over time", useGroupHeal: "group heal", useBuff: "buff"}[mercUseOf(s)]
}

// MercBuffsKept is the buffs a merc of the kit keeps up, as its book holds
// them (mercBook: each line's best, none far beneath its level): the
// handlers' real-data test's.
func MercBuffsKept(kit MercKit) []*CastSpell {
	ms := &mercState{kit: kit, body: newMercBody(1, &kit)}
	var out []*CastSpell
	for _, sp := range ms.mercBook() {
		if sp.use == useBuff {
			out = append(out, sp.s)
		}
	}
	return out
}

// atFoe is whether the use is cast at a creature.
func (u mercUse) atFoe() bool { return u == useNuke || u == useDoT || u == useTaunt }

// mercSkips is whether a merc never casts the spell (the file's doc).
func mercSkips(s *CastSpell) bool {
	d := &s.Def
	switch {
	case s.Travel != nil || s.Familiar != nil || d.Summons() || d.Charms() || d.FormKind != FormNone || d.Resurrects() > 0,
		d.Reagent != "" || d.Backstab || d.TargetKind != "" || d.Stance || len(d.Items) > 0,
		s.Scope == 3 || s.Scope == 4,
		d.Harmful && s.Area.Centre != AreaNone:
		return true
	}
	for _, e := range d.Effects {
		switch e.Kind {
		case EffectGate, EffectSummon, EffectCharm, EffectIllusion, EffectEndIllusion, EffectResurrect, EffectInvis, EffectHide,
			EffectSpeed, EffectSize, EffectSummonPlayer, EffectPickpocket, EffectForget, EffectLull, EffectCure, EffectPowerCap,
			EffectBreath, EffectDisguise, EffectMarkHeal, EffectRaiment, EffectHPCap, EffectStatPin:
			return true
		}
	}
	return false
}

// mercUseOf is what a merc casts the spell for, useNone for a spell it never
// casts.
func mercUseOf(s *CastSpell) mercUse {
	if mercSkips(s) {
		return useNone
	}
	d := &s.Def
	var hate, hit, tick, heal, hot, lasting bool
	for _, e := range d.Effects {
		if e.Self {
			continue
		}
		switch e.Kind {
		case EffectHate:
			hate = hate || e.Amount > 0
		case EffectHP:
			hit = hit || e.Amount < 0
			heal = heal || e.Amount > 0 && e.Drain == DrainNone
		case EffectHPTick:
			tick = tick || e.Amount < 0
			hot = hot || e.Amount > 0
		case EffectStat, EffectShield, EffectDefProc, EffectProc, EffectMitigate, EffectProcHeal:
			lasting = lasting || e.Kind != EffectStat || e.Amount > 0
		case EffectPowerTick:
			lasting = lasting || e.Amount > 0
		}
	}
	if d.Harmful {
		switch {
		case s.Scope != 1:
			return useNone // a self's or a group's line read as harmful (a price, a stance's stat): not at a foe
		case hate && isTaunt(s): // taunt.go: not a hit whose damage outweighs its hate (Hero's Blow)
			return useTaunt
		case hit:
			return useNuke
		case tick:
			return useDoT
		}
		return useNone
	}
	switch {
	case (heal || hot) && s.Scope == 2:
		return useGroupHeal
	case heal && s.Scope == 1:
		return useHeal
	case hot && s.Scope == 1:
		return useHoT
	case (heal || hot || lasting) && d.lasts():
		return useBuff
	}
	return useNone
}

// mercSpell is a spell of the merc's book as it casts it: what for, and what
// it is worth to it (mercWorth).
type mercSpell struct {
	s     *CastSpell
	use   mercUse
	worth float64
	// short: a buff that lasts under mercBuffLasts, kept in a bard's book
	// (its songs) and an enchanter's (its power bursts) alone (mercsongs.go)
	short bool
}

// mercBook is the merc's book as it casts it, its spells' worth on its body
// as it was made: made again with each kit (a new body).
func (ms *mercState) mercBook() []mercSpell {
	if ms.book != nil && ms.bookOf == ms.body {
		return ms.book
	}
	var out []mercSpell
	for i := range ms.kit.Spells {
		s := &ms.kit.Spells[i]
		u := mercUseOf(s)
		if u == useNone {
			continue
		}
		short := false
		if u == useBuff {
			if s.Def.Duration < mercBuffLasts {
				if !mercKeepsShort(ms.kit.Class, s) {
					continue // a fight's buff
				}
				short = true
			} else if s.Level*mercBuffFloor < ms.kit.Level && mercPlainBuff(s) {
				continue // far beneath it: a level 1 +1 INT at 60 (mercBuffFloor)
			}
		}
		out = append(out, mercSpell{s: s, use: u, worth: mercWorth(s, u, ms.body), short: short})
	}
	out = mercBestBuffs(out)
	// the biggest first: each turn takes the first of its kind it may cast
	sort.SliceStable(out, func(i, j int) bool { return out[i].worth > out[j].worth })
	ms.book, ms.bookOf = out, ms.body
	return out
}

// mercBuffFloor: a merc keeps up no buff (not a song or a burst: short) of a
// level under a mercBuffFloor-th of its own (ours, 2026-11-12: a level 60
// cleric merc cast Divine Wisdom, +1 INT, and Minor Blessing, +1 CHA, both of
// level 1 to 3, the owner: "buffs are stacking on Jibrish from mercs that
// normally shouldn't").
const mercBuffFloor = 3

// mercPlainBuff is whether a buff only changes stats or shields its bearer
// (a damage shield): what mercBuffFloor holds to. A proc, power or a heal
// over time is kept at any level.
func mercPlainBuff(s *CastSpell) bool {
	for _, e := range s.Def.Effects {
		switch e.Kind {
		case EffectStat, EffectShield, EffectDefProc:
		default:
			return false
		}
	}
	return len(s.Def.Effects) > 0
}

// mercBestBuffs is the book with each buff line's best alone (the owner,
// 2026-11-12): a buff goes when another of the book is of its line
// (sameLine, the kinds across: buffline.go) and of a higher rank (of one
// rank, the first kept), or of a nested line that covers it (covers, by the
// figures: staticMods), so a lower one, which would only be refused or take
// off the better, is never cast (the cleric's Disease, Poison, Flame and Ice
// Ward under Lightning Ward; Minor Blessing beside Blessing).
func mercBestBuffs(book []mercSpell) []mercSpell {
	out := book[:0:0]
	for i, a := range book {
		if a.use != useBuff {
			out = append(out, a)
			continue
		}
		la, ra, ma := EffectLine(a.s), lineRank(a.s), staticMods(a.s)
		beaten := false
		for j, b := range book {
			if j == i || b.use != useBuff || b.short != a.short || la == "" {
				continue
			}
			lb, rb := EffectLine(b.s), lineRank(b.s)
			if sameLine(la, lb) && (rb > ra || rb == ra && j < i) || nestedLines(la, lb) && covers(staticMods(b.s), ma) {
				beaten = true
				break
			}
		}
		if !beaten {
			out = append(out, a)
		}
	}
	return out
}

// mercWorth is what a spell is worth to the merc: a hit's or a heal's
// figure as its body would roll it on average (a tick's times its ticks), a
// taunt's hate and hit, a buff's level.
func mercWorth(s *CastSpell, u mercUse, b *Player) float64 {
	if u == useBuff {
		return float64(s.Level)
	}
	// a tick's figure counts each time it ticks: its "N ticks", else every
	// effectTick of its time from its landing on (a line with no time lasts
	// tickDuration: cast.go)
	ticks := math.Floor(float64(tickDuration)/float64(effectTick)) + 1
	switch {
	case s.Def.Ticks > 0:
		ticks = float64(s.Def.Ticks)
	case s.Def.Duration > 0:
		ticks = math.Floor(float64(s.Def.Duration)/float64(effectTick)) + 1
	}
	v := 0.0
	for _, e := range s.Def.Effects {
		if e.Self {
			continue
		}
		mean := math.Abs(float64(e.Amount)+float64(max(e.AmountMax, e.Amount))) / 2
		bonus := float64(e.LevelPct * b.Level / 100)
		if e.Scale != StatNone && e.ScalePct != 0 {
			bonus += float64(b.StatNow(e.Scale) * e.ScalePct / 100)
		}
		n := mean + bonus
		if e.Cap > 0 {
			n = min(n, float64(e.Cap))
		}
		switch e.Kind {
		case EffectHP:
			v += n
		case EffectHPTick:
			v += n * ticks
		case EffectHate:
			if u == useTaunt {
				v += n
			}
		}
	}
	return v
}

// mercReady is whether the merc may begin the spell now: its lockout over,
// the arts' cooldown for an art, its power.
func (ms *mercState) mercReady(s *CastSpell, now time.Time) bool {
	if until, ok := ms.ready[s.SpellID]; ok && now.Before(until) {
		return false
	}
	if isArt(s) && now.Before(ms.artReady) {
		return false
	}
	return ms.npc.Power >= s.PowerCost && (s.Def.ReqEquip == "" || equipFits(ms.body, s.Def.ReqEquip))
}

// mercReaches is whether the spell reaches from the merc to a place: its
// range, a creature's allowance for its size, 0 no limit.
func mercReaches(m *NPC, s *CastSpell, x, y, z, extra float32) bool {
	return s.Range <= 0 || dist3(m.X, m.Y, m.Z, x, y, z) <= s.Range+extra
}

// mercCastTurnLocked is the merc's cast this turn, if any (the file's doc):
// a heal, else its pet's summoning, heal or buff, else out of a fight a
// buff, else a tank's taunt, else an attack. It reports a cast begun. Must
// be called with the lock held.
func (w *World) mercCastTurnLocked(ms *mercState, t *NPC, now time.Time) bool {
	if w.mercHealTurnLocked(ms, now) {
		return true
	}
	if w.mercSummonTurnLocked(ms, now) { // its pet, when it has none out (mercpet.go)
		return true
	}
	if w.mercPetCareTurnLocked(ms, now) { // its pet's heal, then its pet's buffs (mercpetbuff.go)
		return true
	}
	// a bard's songs kept up, an enchanter's procs and power on its team, in
	// a fight or out of one (mercsongs.go)
	if w.mercSongTurnLocked(ms, now) || w.mercTeamBuffTurnLocked(ms, now) {
		return true
	}
	if t == nil {
		if w.mercInFightLocked(ms) {
			return false
		}
		return w.mercBuffTurnLocked(ms, now)
	}
	if ms.role == roleTank && w.mercTauntTurnLocked(ms, t, now) {
		return true
	}
	return w.mercAttackTurnLocked(ms, w.NPCs[ms.target], now)
}

// mercInFightLocked reports the merc, or its owner, in a fight: a creature
// fighting either, or its owner swinging. Must be called with the lock held.
func (w *World) mercInFightLocked(ms *mercState) bool {
	if ms.owner.Attacking() {
		return true
	}
	o, m := ms.owner, ms.npc
	for _, id := range w.SpatialIndex.QueryRadius(o.X, o.Z, petAssist) {
		npc := w.NPCs[id]
		if npc != nil && !npc.Dead && (npc.CurrentEnemy == o.ServerID || npc.CurrentEnemy == m.ServerID) {
			return true
		}
	}
	return false
}

// mercMate is one of a merc's team: a player, or an NPC (itself, another's
// merc).
type mercMate struct {
	p *Player
	n *NPC
}

func (mm mercMate) id() EntityID {
	if mm.p != nil {
		return mm.p.ServerID
	}
	return mm.n.ServerID
}

func (mm mercMate) hp() (hp, top int) {
	if mm.p != nil {
		return mm.p.HP, mm.p.MaxHPNow()
	}
	return mm.n.HP, mm.n.MaxHP
}

func (mm mercMate) place() (x, y, z float32) {
	if mm.p != nil {
		return mm.p.X, mm.p.Y, mm.p.Z
	}
	return mm.n.X, mm.n.Y, mm.n.Z
}

func (mm mercMate) effects() []*ActiveEffect {
	if mm.p != nil {
		return mm.p.Effects
	}
	return mm.n.Effects
}

// mercTeamLocked is the merc's team: itself, its owner, and its owner's
// group's other players and mercs, alive and in its world; a player in a duel,
// in Mr. Moogie's challenge or flagged for PvP left out (ownfight.go). Must be
// called with the lock held.
func (w *World) mercTeamLocked(ms *mercState) []mercMate {
	m := ms.npc
	out := []mercMate{{n: m}}
	add := func(p *Player) {
		if p != nil && !p.IsDead() && !p.respawnPending() && p.World == m.World && !w.ownFightLocked(p) {
			out = append(out, mercMate{p: p})
		}
	}
	add(ms.owner)
	g := w.groups[ms.owner.GroupID]
	if ms.owner.GroupID == 0 || g == nil {
		return out
	}
	for _, id := range g.Members {
		if id == ms.owner.ServerID || id == m.ServerID {
			continue
		}
		if p := w.Players[id]; p != nil {
			add(p)
			continue
		}
		if o := w.NPCs[id]; o.isMerc() && !o.Dead && o.World == m.World {
			out = append(out, mercMate{n: o})
		}
	}
	return out
}

// mercHealTurnLocked is a heal the merc casts now, if any (the file's doc).
// It reports a cast begun. Must be called with the lock held.
func (w *World) mercHealTurnLocked(ms *mercState, now time.Time) bool {
	book := ms.mercBook()
	has := false
	for _, sp := range book {
		if sp.use == useHeal || sp.use == useHoT || sp.use == useGroupHeal {
			has = true
			break
		}
	}
	if !has {
		return false
	}
	hurtPct := mercEmergencyPct
	if ms.role == roleHealer {
		hurtPct = mercHurtPct
	}
	type hurt struct {
		mm    mercMate
		share float64
	}
	var hurts []hurt
	for _, mm := range w.mercTeamLocked(ms) {
		hp, top := mm.hp()
		if top <= 0 || hp <= 0 || hp*100 >= top*hurtPct {
			continue
		}
		hurts = append(hurts, hurt{mm, float64(hp) / float64(top)})
	}
	if len(hurts) == 0 {
		return false
	}
	sort.SliceStable(hurts, func(i, j int) bool { return hurts[i].share < hurts[j].share })
	m := ms.npc
	// two or more hurt: a group heal reaching them
	if len(hurts) >= 2 && ms.role == roleHealer {
		for _, sp := range book {
			if sp.use != useGroupHeal || !ms.mercReady(sp.s, now) {
				continue
			}
			x, y, z := hurts[0].mm.place()
			if !mercReaches(m, sp.s, x, y, z, 0) {
				continue
			}
			w.mercCastLocked(ms, sp.s, m.ServerID, true, now)
			return true
		}
	}
	low := hurts[0]
	x, y, z := low.mm.place()
	emergency := low.share*100 < mercEmergencyPct
	pick := func(use mercUse) bool {
		for _, sp := range book {
			if sp.use != use || !ms.mercReady(sp.s, now) || !mercReaches(m, sp.s, x, y, z, 0) {
				continue
			}
			if use == useHoT && buffOn(low.mm.effects(), sp.s) {
				continue // its heal over time on it already
			}
			if use == useGroupHeal {
				w.mercCastLocked(ms, sp.s, m.ServerID, true, now)
				return true
			}
			w.mercCastLocked(ms, sp.s, low.mm.id(), false, now)
			return true
		}
		return false
	}
	if emergency {
		return pick(useHeal) || pick(useGroupHeal) || pick(useHoT)
	}
	return pick(useHoT) || pick(useHeal) || pick(useGroupHeal)
}

// mercBuffTurnLocked is a buff the merc puts up out of a fight, if any (the
// file's doc). It reports a cast begun. Must be called with the lock held.
func (w *World) mercBuffTurnLocked(ms *mercState, now time.Time) bool {
	m, o := ms.npc, ms.owner
	if m.MaxPower <= 0 || m.Power*100 < m.MaxPower*mercBuffSpare || o.IsDead() || w.ownFightLocked(o) {
		return false
	}
	ownerNear := o.World == m.World
	for _, sp := range ms.mercBook() {
		if sp.use != useBuff || sp.short || !ms.mercReady(sp.s, now) {
			continue
		}
		s := sp.s
		mine := !buffOn(m.Effects, s) && !w.mercBuffComingLocked(ms, s, m.ServerID)
		theirs := ownerNear && !buffOn(o.Effects, s) && (s.Def.MinLevel <= 0 || o.Level >= s.Def.MinLevel) &&
			mercReaches(m, s, o.X, o.Y, o.Z, 0) && !w.mercBuffComingLocked(ms, s, o.ServerID)
		switch s.Scope {
		case 0:
			if mine {
				w.mercCastLocked(ms, s, m.ServerID, false, now)
				return true
			}
		case 1:
			if theirs {
				w.mercCastLocked(ms, s, o.ServerID, false, now)
				return true
			}
			if mine && !mercGivesProc(s) { // its own swings run no procs (mercsongs.go)
				w.mercCastLocked(ms, s, m.ServerID, false, now)
				return true
			}
		case 2:
			if mine || theirs {
				w.mercCastLocked(ms, s, m.ServerID, true, now)
				return true
			}
		}
	}
	return false
}

// mercBuffComingLocked is whether a merc's buff of s's line (or one that would
// keep s off) is on its way to id: cast at it, or a group's cast by a merc of
// the same owner (its team); its own cast too (a 5 s cast's landing comes
// after its busy time ends). Three mercs of one owner each began
// the same buff on him within a second, none landed yet to be seen
// (2026-11-12, the GM Jibrish's three clerics: Group Resist Affliction landed
// on him three times over); since then the first cast stands for all.
// Must be called with the lock held.
func (w *World) mercBuffComingLocked(ms *mercState, s *CastSpell, id EntityID) bool {
	if len(w.mercCasts) == 0 {
		return false
	}
	line := EffectLine(s)
	for _, c := range w.mercCasts {
		if mercUseOf(&c.spell) != useBuff {
			continue
		}
		if c.target != id && !(c.group && c.ms.owner == ms.owner) {
			continue
		}
		cl := EffectLine(&c.spell)
		if c.spell.SpellID == s.SpellID || sameLine(cl, line) && lineRank(&c.spell) >= lineRank(s) ||
			nestedLines(cl, line) && covers(staticMods(&c.spell), staticMods(s)) {
			return true
		}
	}
	return false
}

// mercLooseFoeLocked is the nearest creature within petAssist of the merc's
// owner fighting another of its team (a player, its pet, its merc), for a tank
// already holding its own target. Must be called with the lock held.
func (w *World) mercLooseFoeLocked(ms *mercState) *NPC {
	m, o := ms.npc, ms.owner
	team := map[EntityID]bool{}
	for _, mm := range w.mercTeamLocked(ms) {
		if mm.n != m {
			team[mm.id()] = true
		}
	}
	if pet := w.petOfLocked(o); pet != nil {
		team[pet.ServerID] = true
	}
	var best *NPC
	var bestD float32
	for _, id := range w.SpatialIndex.QueryRadius(o.X, o.Z, petAssist) {
		npc := w.NPCs[id]
		if npc == nil || !mercMayFight(m, npc) || !team[npc.CurrentEnemy] {
			continue
		}
		if d := distXZ(m.X, m.Z, npc.X, npc.Z); best == nil || d < bestD {
			best, bestD = npc, d
		}
	}
	return best
}

// mercTauntTurnLocked is a tank's taunt (the file's doc). It reports a cast
// begun. Must be called with the lock held.
func (w *World) mercTauntTurnLocked(ms *mercState, t *NPC, now time.Time) bool {
	m := ms.npc
	if t.CurrentEnemy == m.ServerID && ms.ordered == 0 {
		if o := w.mercLooseFoeLocked(ms); o != nil {
			ms.target, t = o.ServerID, o
			m.endRound()
		}
	}
	if t.CurrentEnemy == m.ServerID {
		return false // it holds it: its power for its attacks (the owner, 2026-11-11)
	}
	for _, sp := range ms.mercBook() {
		if sp.use != useTaunt || !ms.mercReady(sp.s, now) || !mercReaches(m, sp.s, t.X, t.Y, t.Z, npcReachExtra(t)) ||
			!w.petUndeadMayLocked(sp.s, t) {
			continue
		}
		w.mercCastLocked(ms, sp.s, t.ServerID, false, now)
		return true
	}
	return false
}

// mercAttackTurnLocked is the merc's attack at t (the file's doc). It reports
// a cast begun. Must be called with the lock held.
func (w *World) mercAttackTurnLocked(ms *mercState, t *NPC, now time.Time) bool {
	if t == nil || t.Dead {
		return false
	}
	m := ms.npc
	if ms.role == roleHealer && m.Power*100 < m.MaxPower*mercHealerSpare {
		return false
	}
	if ms.kit.Class == classBard && m.Power*100 < m.MaxPower*mercBardSpare {
		return false // its power for its songs (mercsongs.go)
	}
	book := ms.mercBook()
	// a caster or a healer short of power casts thriftily (mercThrifty)
	thrifty := (ms.role == roleCaster || ms.role == roleHealer) && m.Power*100 < m.MaxPower*mercThriftyBelow
	// what a caster keeps its power for: its best hit's cost
	reserve := 0
	if ms.role == roleCaster && !thrifty {
		for _, sp := range book {
			if sp.use == useNuke && w.petUndeadMayLocked(sp.s, t) {
				reserve = sp.s.PowerCost
				break
			}
		}
	}
	can := func(sp mercSpell, best bool) bool {
		s := sp.s
		if !ms.mercReady(s, now) || !mercReaches(m, s, t.X, t.Y, t.Z, npcReachExtra(t)) || !w.petUndeadMayLocked(s, t) {
			return false
		}
		return best || m.Power-s.PowerCost >= reserve
	}
	// its damage over time kept up
	for _, sp := range book {
		if sp.use == useDoT && can(sp, false) && !w.mercDoTUpLocked(ms, sp.s, t, now) {
			w.mercCastLocked(ms, sp.s, t.ServerID, false, now)
			return true
		}
	}
	if thrifty {
		if sp := mercThrifty(book, func(sp mercSpell) bool { return sp.use == useNuke && can(sp, true) }); sp != nil {
			w.mercCastLocked(ms, sp.s, t.ServerID, false, now)
			return true
		}
		return false
	}
	first := true
	for _, sp := range book {
		if sp.use != useNuke {
			continue
		}
		if can(sp, first && sp.s.PowerCost == reserve) {
			w.mercCastLocked(ms, sp.s, t.ServerID, false, now)
			return true
		}
		first = false
	}
	// a melee fighter's taunt that hits too (a Hero's Blow), on cooldown
	if ms.role == roleMelee {
		for _, sp := range book {
			if sp.use == useTaunt && can(sp, false) {
				w.mercCastLocked(ms, sp.s, t.ServerID, false, now)
				return true
			}
		}
	}
	return false
}

// mercThriftyBelow is the share of its power below which a caster or a
// healer casts its hits thriftily (mercThrifty).
const mercThriftyBelow = 25

// mercThrifty is the hit of the book a merc short of power casts (the owner,
// 2026-11-11: "magician mercs need to cast spells for damage like the
// necro"): of those ok allows, the one that does the most for its power (its
// worth over its cost; a free one first), the bigger of two as good. A
// magician's book is hits alone, each to be cast again 3 s on: cast biggest
// first it emptied its power in half a minute and then cast one Blaze in
// every 25 s of its power's return, where a necromancer's damage over time,
// its pacts and its life taps kept it casting. Its power's return is what it
// casts with then, and the hit that does most for it does the most damage.
func mercThrifty(book []mercSpell, ok func(mercSpell) bool) *mercSpell {
	var best *mercSpell
	var bestEff float64
	for i := range book {
		sp := &book[i]
		if !ok(*sp) {
			continue
		}
		eff := sp.worth / float64(max(sp.s.PowerCost, 1))
		if sp.s.PowerCost <= 0 {
			eff = math.Inf(1)
		}
		if best == nil || eff > bestEff || eff == bestEff && sp.worth > best.worth {
			best, bestEff = sp, eff
		}
	}
	return best
}

// mercDoTUpLocked is whether the merc's damage over time s is up on t: on its
// way, or on it with more than its cast's time and landing to run. Must be
// called with the lock held.
func (w *World) mercDoTUpLocked(ms *mercState, s *CastSpell, t *NPC, now time.Time) bool {
	for _, c := range w.mercCasts {
		if c.ms == ms && c.target == t.ServerID && c.spell.SpellID == s.SpellID {
			return true
		}
	}
	lead := s.CastTime + castLand
	for _, e := range t.Effects {
		if e.SpellID == s.SpellID && e.merc == ms.npc.ServerID && e.Until.Sub(now) > lead {
			return true
		}
	}
	return false
}

// mercCast is a merc's cast on its way to landing (mercCastSystemLocked).
type mercCast struct {
	ms     *mercState
	life   uint64 // the hire that cast it
	spell  CastSpell
	target EntityID // the creature, the one helped, or the merc for its group
	group  bool     // a group's: its team in the spell's reach
	lands  time.Time
}

// mercCastLocked begins the merc's cast of s at target: its power, its
// lockout, the arts' cooldown, its busy time, and its 0x00B1 with its cast
// time and visual to everyone who sees it. Must be called with the lock held.
func (w *World) mercCastLocked(ms *mercState, s *CastSpell, target EntityID, group bool, now time.Time) {
	m := ms.npc
	m.Power = max(m.Power-s.PowerCost, 0)
	ms.ready[s.SpellID] = now.Add(s.CastTime + s.Recast + castLockoutExtra)
	if isArt(s) {
		ms.artReady = now.Add(w.balance.ArtCooldown(m.Level))
	}
	ms.busy = now.Add(s.CastTime + castBusy)
	w.mercCasts = append(w.mercCasts, &mercCast{ms: ms, life: ms.life, spell: *s, target: target, group: group,
		lands: now.Add(s.CastTime + castLand)})
	if target != m.ServerID && w.NPCs[target] != nil {
		breakNPCInvis(m)
	}
	w.broadcastNPCSwingLocked(m, target, uint32(s.CastTime/time.Millisecond), s.visual())
	log.Printf("Merc: %s's %q begins %q (spell %d) on %s: %v cast, %d power (%d left)", ms.owner.Name, m.Name, s.Name, s.SpellID,
		w.entityNameLocked(target), s.CastTime, s.PowerCost, m.Power)
}

// mercCastSystemLocked lands every merc's cast whose time has come, by a merc
// still out, alive, of the hire that cast it. Every tick (mercSystem); must be
// called with the lock held.
func (w *World) mercCastSystemLocked(now time.Time) {
	if len(w.mercCasts) == 0 {
		return
	}
	var due []*mercCast
	keep := w.mercCasts[:0]
	for _, c := range w.mercCasts {
		if now.Before(c.lands) {
			keep = append(keep, c)
		} else {
			due = append(due, c)
		}
	}
	for i := len(keep); i < len(w.mercCasts); i++ {
		w.mercCasts[i] = nil
	}
	w.mercCasts = keep
	for _, c := range due {
		ms := c.ms
		if ms.npc.Dead || !w.mercLiveLocked(ms) || ms.life != c.life {
			continue
		}
		w.mercLandLocked(c, now)
	}
}

// mercLandLocked lands a merc's cast: at a creature its harm (mercHarmLocked),
// on a friend what a player's cast of it does (cast.go: landOnPlayerLocked on
// a player, landOnPetLocked on a merc), on its team in reach for a group's.
// Must be called with the lock held.
func (w *World) mercLandLocked(c *mercCast, now time.Time) {
	ms, s, m := c.ms, &c.spell, c.ms.npc
	if s.Def.Charms() {
		w.mercCharmLandLocked(ms, s, c.target, now) // /merc charm (mercharm.go)
		return
	}
	if s.Def.Summons() {
		w.mercSummonLandLocked(ms, s, now) // its pet (mercpet.go)
		return
	}
	if mercUseOf(s).atFoe() {
		t := w.NPCs[c.target]
		if t == nil || t.Dead || t.World != m.World || !mercMayFight(m, t) {
			return
		}
		w.mercHarmLocked(ms, s, t, now)
		return
	}
	if !c.group {
		w.mercHelpLocked(ms, s, c.target, now)
		return
	}
	reach := s.Range
	if s.Def.ShareRadius > 0 {
		reach = s.Def.ShareRadius
	}
	for _, mm := range w.mercTeamLocked(ms) {
		x, y, z := mm.place()
		if reach <= 0 || dist3(m.X, m.Y, m.Z, x, y, z) <= reach {
			w.mercHelpLocked(ms, s, mm.id(), now)
		}
	}
}

// mercHelpLocked lands a merc's beneficial spell on one: a player as a
// player's cast lands on it (its heal told to it from the merc, its buff's
// line and its heal over time's rules), a merc as on a pet (its buffs its
// stats: syncMercBody). The merc's body is the caster: its stats scale the
// figures. Must be called with the lock held.
func (w *World) mercHelpLocked(ms *mercState, s *CastSpell, id EntityID, now time.Time) {
	cs := &castState{spell: *s, caster: ms.npc.ServerID, targets: []EntityID{id}}
	cs.spell.figures = figuresMerc // a merc's figures to a client that colours them (numbertags.go)
	// its own pet (mercpetbuff.go)
	if w.mercPetLandLocked(ms, cs, id, now) {
		return
	}
	if p := w.Players[id]; p != nil {
		// not on one whose fight became its own while the cast was on its way
		// (ownfight.go, petition 1491)
		if !p.IsDead() && !p.respawnPending() && !w.ownFightLocked(p) {
			w.landOnPlayerLocked(cs, ms.body, nil, p, now)
		}
		return
	}
	if n := w.NPCs[id]; n.isMerc() && !n.Dead {
		w.landOnPetLocked(cs, ms.body, nil, n, now)
		syncMercBody(n.merc)
	}
}

// mercHarmLocked lands a merc's harmful spell on t: its share on itself (a
// taunt's AC: mercSelfShareLocked), each hit a player's figure of its body
// less what the creature resists, through the balance as a player's spell
// (dealtLocked), its life tap's take back on it; its hate; what lasts (a
// damage over time, whose ticks are its hits: mercTickLocked; a nuke's
// debuff) as a player's lands on a creature. Must be called with the lock
// held.
func (w *World) mercHarmLocked(ms *mercState, s *CastSpell, t *NPC, now time.Time) {
	m, b := ms.npc, ms.body
	if !w.petUndeadMayLocked(s, t) {
		return
	}
	w.mercSelfShareLocked(ms, s, now)
	angered := false
	for _, e := range s.Def.Effects {
		if e.Self {
			continue
		}
		switch e.Kind {
		case EffectHP:
			if e.Amount >= 0 {
				continue
			}
			n := creatureResists(e.Resist, t, arcaneDebuffed(e.Resist, t, debuffScaled(e, t, spellAmount(e, b), now), now), now)
			if n >= 0 {
				continue
			}
			dealt := w.dealtLocked(t, b, -n)
			killed := w.mercDamageLocked(ms, t, dealt, numberSpell, now)
			angered = true
			if e.Drain != DrainNone && m.HP < m.MaxHP {
				m.HP = min(m.HP+dealt, m.MaxHP) // a life tap's take, whole though it killed (lifetap.go)
			}
			if killed {
				return
			}
		case EffectHate:
			// a taunt puts the merc at the top of its target's hate, as a pet's
			// taunt does (pet.go's petTopHateLocked), its line's hate on top:
			// a tank holds what it taunts, as a player's taunt does since
			// 2026-11-11 (taunt.go); a hit's hate (Hero's Blow) is added alone
			if isTaunt(s) {
				w.petTopHateLocked(m, t)
				w.updateAggro(t, m.ServerID, max(spellAmount(e, b), 1))
			} else {
				w.updateAggro(t, m.ServerID, spellAmount(e, b))
			}
			angered = true
		}
	}
	if !angered {
		w.updateAggro(t, m.ServerID, 1) // a spell that only lasts angers it all the same
	}
	// what lasts of it on the creature, its own parts alone
	rest := *s
	rest.Def.Effects = nil
	for _, e := range s.Def.Effects {
		if !e.Self {
			rest.Def.Effects = append(rest.Def.Effects, e)
		}
	}
	cs := &castState{spell: rest, caster: m.ServerID, targets: []EntityID{t.ServerID}}
	ae := lasting(cs, b, now)
	if ae == nil || !(ae.HPTick < 0 || ae.Mods != (StatMods{}) || ae.Root || ae.Snare) {
		return
	}
	ae.merc, ae.Caster = m.ServerID, m.ServerID
	ae.HPTick = min(ae.HPTick, 0) // a heal over time on a creature is no merc's
	ae.Line, ae.Rank = BuffLine(&rest), lineRank(&rest)
	keep := t.Effects[:0]
	for _, e := range t.Effects {
		if e.SpellID != s.SpellID || e.merc != m.ServerID {
			keep = append(keep, e)
		}
	}
	for i := len(keep); i < len(t.Effects); i++ {
		t.Effects[i] = nil
	}
	t.Effects = append(keep, ae)
	t.syncEffectMaxHP()
	if w.affected == nil {
		w.affected = make(map[EntityID]*NPC)
	}
	w.affected[t.ServerID] = t
	if ae.Root || ae.Snare {
		t.stopMoving()
	}
	log.Printf("Merc: %s's %q puts %q on %s for %v (%d a tick)", ms.owner.Name, m.Name, s.Name, t.Name, ae.Until.Sub(now), -ae.HPTick)
	w.tickNPCEffectLocked(t, ae, now)
}

// mercSelfShareLocked puts a harmful spell's share on its caster on the merc
// (a taunt's "Increase AC by 500 to Self" for the spell's time: the Pillars),
// as a player's own share is its own (lifetap.go's landOnCasterLocked): its
// stats as an effect of its own, its heal now. A price (hit points taken) is
// left unpaid. Must be called with the lock held.
func (w *World) mercSelfShareLocked(ms *mercState, s *CastSpell, now time.Time) {
	m, b := ms.npc, ms.body
	var mods StatMods
	stats := false
	for _, e := range s.Def.Effects {
		if !e.Self {
			continue
		}
		switch n := spellAmount(e, b); e.Kind {
		case EffectStat:
			mods.add(e.Stat, n)
			stats = true
		case EffectHP:
			if n > 0 {
				m.HP = min(m.HP+n, m.MaxHP)
			}
		}
	}
	if !stats || s.Def.Duration <= 0 {
		return
	}
	ae := &ActiveEffect{SpellID: -s.SpellID, Name: s.Name, Icon: s.Icon, Caster: m.ServerID, Until: now.Add(s.Def.Duration), Mods: mods}
	putOnPetLocked(m, s, ae) // its own effect (its id negative), apart from what the spell puts on its target
	syncMercBody(ms)
}
