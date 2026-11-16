package entity

import (
	"log"
	"slices"
	"time"
)

// mercbrew.go — what an alchemist merc makes and takes (the owner,
// 2026-11-15: "the alc mercs should constantly make pots and they are saved
// for future mercs also", then "alchemists mercs should use draughts to
// restore mana, hurts hp, they eat to restore hp, throw the pots and cast
// spells").
//
// Its owner's stock (the owner, 2026-11-15, again: "the alchemist mercs
// should keep making pots until full stack if they are sitting idle, but if
// player moves they follow until idle again, then they keep making more pots
// until full (they carry over to future sessions and are not lost on
// dismiss/death) so they can throw single/aoe pots without running out"):
//
//   - It brews only at ease (mercBrewIdleLocked): alive, beside its owner in
//     its world, no target, no throw or cast on its way, nothing fighting it
//     or its owner, its owner neither swinging nor dead, and for
//     world.merc_brew_settle_seconds (MercBrewSettle) none of that, nor a
//     step of its owner's (followslot.go's frame), nor a step of its own (a
//     stroll at ease, mercidle.go, is no step: it is how it idles), nor a
//     fight of its or its owner's. Its owner's move pauses it: it follows as
//     ever, and its clock starts again once both stand.
//   - At ease it brews a batch (world.merc_brew_batch, MercBrewBatch) every
//     world.merc_brew_seconds (MercBrewEvery), its first batch that long
//     after it settled.
//   - What it brews (mercBrewWantsLocked): a full stack (the potion's own
//     itempattern.maxstack, or world.merc_brew_stack when that is less) of
//     each potion it uses: its best single-target potion and its best area
//     cocktail, as mercThrowablesLocked ranks its path's (so /merc aoe and
//     single both find their kind in the stock), and its book's best power
//     draught (it drinks them: below). Each batch goes to the one least full
//     (its throws' own kind first of two as full). With every one full it
//     brews nothing. Meals are not brewed (no alchemist's book makes one; a
//     merc with none eats a ration's figures). world.merc_brew_cap, when set
//     above 0, is a ceiling on the whole stock besides (0: none brewed).
//   - Its Master Class's keepsakes (mercKeepsLocked): the Transmuter's
//     Transformative Pill and Transmutive Potion, the Metabolist's
//     Miraculous Draught, one at a time for its owner, brewed only when none
//     is held.
//
// The stock is its owner's, not the merc's: the handlers keep it in table
// charmercstock and hand the World its counts at each login, keep restart and
// hire (SetMercStock), so it outlives a dismissal, a death, a logout and a
// restart, and an alchemist hired later throws from it. Its owner takes
// potions out of it through the merc's menu (handlers).
//
// Its draughts (the owner, 2026-11-15: "the alchemist mercs are not
// draughting to restore power even though cleric merc can heal them"): in a
// fight or out of one, out of a cast, when its power and what a draught
// working on it has still to give come under mercDraughtBelow of its most
// (so a draught spent while its spells drain it faster than a draught gives
// is followed by the next, where until then it waited out each draught's
// whole time near empty), it drinks the best power draught it has (a Self
// potion whose line gives power over time and costs health: the Watery to
// the Infused Draught), from what it carries, else its owner's stock, else
// its path's own (what its book's refinements summon, conjured as its
// throws'), its figures a player's (cast.go's spellAmount on its body): the
// health taken at once (a player's draught's poison resist may spare it:
// draughtResisted), the power over the draught's time. The health it costs
// may not leave it under world.merc_draught_hp_floor_pct of its most
// (MercDraughtFloorPct by default) unless a healer stands in its team (a
// cleric, druid or shaman, player or merc, alive and in its world:
// mercHealerNearLocked), and never under mercDraughtHardFloor of its most.
//
// Its meals: out of a fight (nothing fighting it, its owner or its owner's
// mercs, its owner swinging at nothing), below mercEatBelow of its health,
// with no meal on it, it eats: a meal it carries (its health a tick, the
// item's HoT), else a ration (RationRestore: a tenth of its most health a
// tick, a player's ration's), for a meal's minute, its first tick at once, as
// a player's meal (consumable.go); a fight ends it, as a player's.
//
// Each drink and meal shows as a player's does to those round it: the
// eating-and-drinking visual (fxConsumed) on the merc.

// The alchemist's stock's defaults (world.merc_brew_seconds,
// merc_brew_batch, merc_brew_settle_seconds, merc_brew_cap, merc_brew_stack):
// 20 potions every 5 s at ease, 3 s after it and its owner stood still, so a
// 2,000 stack (most potions' and cocktails') fills in some 8 minutes of rest
// and a 9,999 one in 42; no ceiling on the whole stock but the stacks, and
// each stack its record's.
const (
	MercBrewEvery  = 5 * time.Second
	MercBrewBatch  = 20
	MercBrewSettle = 3 * time.Second
	MercBrewCap    = 0
	// mercBrewStackUnknown is a stack's most for a potion with no record's
	mercBrewStackUnknown = 20
)

// MercDraughtFloorPct is the share of its most health below which a draught
// would leave an alchemist merc, so it does not drink
// (world.merc_draught_hp_floor_pct).
const MercDraughtFloorPct = 50

// When it drinks and eats (ours).
const (
	// mercDraughtBelow: it drinks a draught below this share of its power
	// (with what a draught on it has still to give)
	mercDraughtBelow = 30
	// mercDraughtHardFloor: no draught leaves it under this share of its
	// health, a healer near or not
	mercDraughtHardFloor = 10
	// mercEatBelow: out of a fight it eats below this share of its health
	mercEatBelow = 70
)

// mercSupply is the World's alchemists' supply: each owner's stock by item
// (the handlers' rows), and the rates.
type mercSupply struct {
	stock map[EntityID]map[int]int // by owner's entity id
	// char is whose stock each entity's is (an entity id is used again by
	// another login: its stock is not the new player's)
	char   map[EntityID]uint32
	every  time.Duration
	batch  int
	settle time.Duration
	stack  int // a stack's most at the most (0: its record's)
	cap    int // -1 none, 0 none brewed, else the whole stock's ceiling
	floor  int // percent of its most health a draught keeps it above
	set    bool
}

func (s *mercSupply) brewEvery() time.Duration {
	if s.set && s.every > 0 {
		return s.every
	}
	return MercBrewEvery
}

func (s *mercSupply) brewBatch() int {
	if s.batch > 0 {
		return s.batch
	}
	return MercBrewBatch
}

func (s *mercSupply) brewSettle() time.Duration {
	if s.settle > 0 {
		return s.settle
	}
	return MercBrewSettle
}

// brewCeiling is the whole stock's ceiling (0 none) and whether brewing is
// off (world.merc_brew_cap 0).
func (s *mercSupply) brewCeiling() (most int, off bool) {
	switch {
	case s.set && s.cap == 0:
		return 0, true
	case s.set && s.cap > 0:
		return s.cap, false
	}
	return MercBrewCap, false
}

// stackOf is the most of pe the merc brews: its record's stack, at most
// world.merc_brew_stack.
func (s *mercSupply) stackOf(pe *PotionEffect) int {
	n := mercBrewStackUnknown
	if pe != nil && pe.MaxStack > 0 {
		n = pe.MaxStack
	}
	if s.stack > 0 {
		n = min(n, s.stack)
	}
	return n
}

func (s *mercSupply) draughtFloor() int {
	if s.set && s.floor >= 0 {
		return s.floor
	}
	return MercDraughtFloorPct
}

// stockOf is the owner's stock, nil before the handlers gave it.
func (s *mercSupply) stockOf(p *Player) map[int]int {
	if p == nil || s.stock == nil || s.char[p.ServerID] != p.CharServerID {
		return nil
	}
	return s.stock[p.ServerID]
}

func (s *mercSupply) total(p *Player) int {
	n := 0
	for _, c := range s.stockOf(p) {
		n += c
	}
	return n
}

// take takes up to n of the item from the owner's stock and reports how many.
func (s *mercSupply) take(p *Player, patternID, n int) int {
	st := s.stockOf(p)
	if st == nil {
		return 0
	}
	n = min(n, st[patternID])
	if n <= 0 {
		return 0
	}
	st[patternID] -= n
	if st[patternID] <= 0 {
		delete(st, patternID)
	}
	return n
}

// SetMercBrew sets the stock's rate and cap and a draught's floor
// (world.merc_brew_seconds, merc_brew_cap, merc_draught_hp_floor_pct: below 0
// each its default; a cap of 0 brews none).
func (w *World) SetMercBrew(every time.Duration, cap, floorPct int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.mercSupply.every, w.mercSupply.cap, w.mercSupply.floor, w.mercSupply.set = every, cap, min(floorPct, 100), true
	if every < 0 {
		w.mercSupply.every = 0
	}
}

// SetMercBrewBatch sets how many potions a brew makes, how long it and its
// owner stand before it brews, and a stack's most at the most
// (world.merc_brew_batch, merc_brew_settle_seconds, merc_brew_stack: 0 or
// below each its default, the stack its record's).
func (w *World) SetMercBrewBatch(batch int, settle time.Duration, stack int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.mercSupply.batch, w.mercSupply.settle, w.mercSupply.stack = max(batch, 0), max(settle, 0), max(stack, 0)
}

// MercBrewCapNow is the whole stock's ceiling as set, 0 for none.
func (w *World) MercBrewCapNow() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	most, _ := w.mercSupply.brewCeiling()
	return most
}

// MercStockKnown is whether the World holds the character's stock (given at
// its login, a keep restart's merc or a hire).
func (w *World) MercStockKnown(charID uint32) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.mercSupply.stockOf(w.playerByCharacterLocked(charID)) != nil
}

// SetMercStock gives the World the character's stock as kept (the handlers'
// read at a login, a keep restart, or after its owner took from it).
func (w *World) SetMercStock(charID uint32, stock map[int]int) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.playerByCharacterLocked(charID)
	if p == nil {
		return false
	}
	if w.mercSupply.stock == nil {
		w.mercSupply.stock = map[EntityID]map[int]int{}
		w.mercSupply.char = map[EntityID]uint32{}
	}
	st := make(map[int]int, len(stock))
	for id, n := range stock {
		if n > 0 {
			st[id] = n
		}
	}
	w.mercSupply.stock[p.ServerID] = st
	w.mercSupply.char[p.ServerID] = p.CharServerID
	return true
}

// MercStockOf is the character's stock as the World has it (nil none).
func (w *World) MercStockOf(charID uint32) map[int]int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	st := w.mercSupply.stockOf(w.playerByCharacterLocked(charID))
	if st == nil {
		return nil
	}
	out := make(map[int]int, len(st))
	for id, n := range st {
		out[id] = n
	}
	return out
}

// TakeMercStock takes up to n of the item out of the character's stock for
// its pack (the handlers put them there and keep the row); it reports how
// many it took.
func (w *World) TakeMercStock(charID uint32, patternID, n int) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.mercSupply.take(w.playerByCharacterLocked(charID), patternID, n)
}

// GiveBackMercStock puts n of the item back in the character's stock (a
// withdrawal the database refused).
func (w *World) GiveBackMercStock(charID uint32, patternID, n int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if st := w.mercSupply.stockOf(w.playerByCharacterLocked(charID)); st != nil && n > 0 {
		st[patternID] += n
	}
}

// mercBrewWant is a potion the merc keeps a stack of: its use, the stack's
// most and how many the stock holds.
type mercBrewWant struct {
	pe         *PotionEffect
	most, have int
}

// mercBrewWantsLocked is what the merc keeps a full stack of (the file's
// doc), its throws' own kind (/merc aoe or single) first, then the other
// kind, then its draught, then its keepsakes (one each). Must be called with
// the lock held.
func (w *World) mercBrewWantsLocked(ms *mercState) []mercBrewWant {
	st := w.mercSupply.stockOf(ms.owner)
	var out []mercBrewWant
	add := func(id int) {
		pe := w.potions.byPattern[id]
		if pe == nil {
			return
		}
		for _, o := range out {
			if o.pe.PatternID == id {
				return
			}
		}
		out = append(out, mercBrewWant{pe: pe, most: w.mercSupply.stackOf(pe), have: st[id]})
	}
	aoe := ms.mercAoE()
	book := w.mercThrowablesLocked(ms) // best first
	for _, kind := range []bool{aoe, !aoe} {
		for _, c := range book {
			if c.spell == 0 && (c.pe.Area > 0) == kind {
				add(c.pe.PatternID)
				break
			}
		}
	}
	if d := w.mercBookDraughtsLocked(ms); len(d) > 0 {
		add(d[0].pe.PatternID)
	}
	// its Master Class's keepsakes: one each, for its owner (mercKeepsLocked)
	for _, pe := range w.mercKeepsLocked(ms) {
		if slices.ContainsFunc(out, func(o mercBrewWant) bool { return o.pe.PatternID == pe.PatternID }) {
			continue
		}
		have := st[pe.PatternID]
		for _, c := range ms.kit.Carried {
			if c.PatternID == pe.PatternID && c.Count > 0 {
				have += c.Count
			}
		}
		out = append(out, mercBrewWant{pe: pe, most: mercKeepOne, have: have})
	}
	return out
}

// mercKeepOne is how many of a keepsake the merc holds for its owner (the
// owner, 2026-11-16: "and they will only make if they dont have one so player
// can take it from their inventory").
const mercKeepOne = 1

// mercKeepsLocked is what its Master Class's mixtures make, which the merc
// brews for its owner one at a time (the owner, 2026-11-16: "alchemist mercs
// should also make the transformative ogre pills if they are the right
// level", "and they will only make if they dont have one so player can take
// it from their inventory", then "make the transmutive potion and miraculous
// draught one at a time too"): each potion a mixture its own Master Class's
// ranks give makes (mercRankPotionsLocked: a spell its ranks add, in its
// book, not turned off; the ranks its level's, as the handlers give them, and
// none above its level here either). So a Transmuter of 60 keeps a
// Transformative Pill (Creative Transmutation, cmcatalog 7402: Create
// Transformative) and a Transmutive Potion (Transmutative Mixture, 7403), a
// Metabolist of 60 a Miraculous Draught (Miraculous Mix, 7412): the gate a
// player alchemist's is, who has those spells only by buying the rank, where
// classspells' level-60 rows hand them to any alchemist. A keepsake's stack is
// one (mercKeepOne): it brews one when its owner's stock and what it carries
// hold none, at ease and at its rate as its other potions, and none while one
// waits there; its owner takes it from the Stock tab, and it brews the next.
// A stock copy is its owner's: the merc never throws or drinks one (no
// damage throw: throwWorth; no power: draughtOf); its own uses of them are
// conjured, never from the stock (mercselfpotion.go). Must be called with the
// lock held.
func (w *World) mercKeepsLocked(ms *mercState) []*PotionEffect {
	if !ms.mercThrowsPotions() {
		return nil
	}
	var ranks []*CMRank
	for _, r := range ms.kit.MasterRanks {
		if r != nil && r.Level <= ms.kit.Level {
			ranks = append(ranks, r)
		}
	}
	return w.mercRankPotionsLocked(ms, ranks)
}

// mercBrewPickLocked is the potion the merc brews next and how many may go
// in: of its wants (mercBrewWantsLocked) the least full, false with every
// stack full. Must be called with the lock held.
func (w *World) mercBrewPickLocked(ms *mercState) (int, int, bool) {
	best, room := -1, 0
	var bestFill float64
	for _, c := range w.mercBrewWantsLocked(ms) {
		if c.have >= c.most {
			continue
		}
		fill := float64(c.have) / float64(c.most)
		if best < 0 || fill < bestFill {
			best, bestFill, room = c.pe.PatternID, fill, c.most-c.have
		}
	}
	if best < 0 {
		return 0, 0, false
	}
	return best, room, true
}

// mercBrewIdleLocked is whether the merc is at ease to brew (the file's doc),
// noting its own steps. Must be called with the lock held.
func (w *World) mercBrewIdleLocked(ms *mercState, now time.Time) bool {
	o, n := ms.owner, ms.npc
	if n.moving() && !ms.idle.walking {
		ms.brewStirred = now // a step of its own (a stroll at ease is none)
	}
	if n.Dead || o.IsDead() || o.respawnPending() || n.World != o.World || ms.target != 0 || ms.throwing != nil ||
		now.Before(ms.busy) || o.Attacking() || distXZ(n.X, n.Z, o.X, o.Z) > petLost || w.mercInFightLocked(ms) {
		return false
	}
	settle := w.mercSupply.brewSettle()
	settled := func(t time.Time) bool { return t.IsZero() || !now.Before(t.Add(settle)) }
	f := w.followFrameLocked(o, now)
	return settled(f.movedAt) && settled(ms.brewStirred) && settled(ms.idle.quiet) && settled(ms.busy) &&
		settled(n.LastCombatTime) && settled(o.LastCombatTime)
}

// mercBrewLocked brews the merc's potions into its owner's stock when it is
// at ease and its time has come (the file's doc). Must be called with the
// lock held.
func (w *World) mercBrewLocked(ms *mercState, now time.Time) {
	if !ms.mercThrowsPotions() {
		return
	}
	st := w.mercSupply.stockOf(ms.owner)
	if st == nil {
		return // its owner's stock not read yet
	}
	ceiling, off := w.mercSupply.brewCeiling()
	if off {
		return
	}
	if !w.mercBrewIdleLocked(ms, now) {
		ms.brewAt = time.Time{} // paused: its clock starts again at ease
		return
	}
	every := w.mercSupply.brewEvery()
	if ms.brewAt.IsZero() {
		ms.brewAt = now.Add(every)
		return
	}
	if now.Before(ms.brewAt) {
		return
	}
	ms.brewAt = now.Add(every)
	id, room, ok := w.mercBrewPickLocked(ms)
	if !ok {
		return // every stack full
	}
	n := min(w.mercSupply.brewBatch(), room)
	if ceiling > 0 {
		n = min(n, ceiling-w.mercSupply.total(ms.owner))
	}
	if n <= 0 {
		return
	}
	st[id] += n
	w.noteMercLocked(MercNote{Kind: MercNoteBrewed, Owner: ms.owner, Slot: ms.slot, Name: ms.npc.Name, PatternID: id, Count: n})
}

// mercDraught is a draught the merc may drink: its use, where from, its
// health's cost and its power.
type mercDraught struct {
	pe      PotionEffect
	carried int
	stock   bool
	cost    int
	power   int
}

// draughtOf is pe as a power draught on the merc's body: its health's cost
// now and its power over its time; false for no draught (a drink that gives
// no power over time).
func (ms *mercState) draughtOf(pe *PotionEffect) (cost, power int, ok bool) {
	if pe == nil || pe.Thrown() {
		return 0, 0, false
	}
	ticks := 1
	if d := pe.duration(); d > 0 {
		ticks = int(d/effectTick) + 1
	}
	for _, e := range pe.Def.Effects {
		switch {
		case e.Kind == EffectHP && e.Amount < 0:
			cost -= spellAmount(e, ms.body)
		case e.Kind == EffectPowerTick && e.Amount > 0:
			power += spellAmount(e, ms.body) * ticks
		case e.Kind == EffectPower && e.Amount > 0:
			power += spellAmount(e, ms.body)
		}
	}
	return cost, power, power > 0 && cost > 0
}

// mercDraughtsLocked is the draughts the merc may drink, in its order: what
// it carries, its owner's stock, its path's own (its book's summonings), each
// the most power first. Must be called with the lock held.
func (w *World) mercDraughtsLocked(ms *mercState) []mercDraught {
	var out []mercDraught
	add := func(list []mercDraught) {
		for i := 1; i < len(list); i++ {
			for j := i; j > 0 && list[j].power > list[j-1].power; j-- {
				list[j], list[j-1] = list[j-1], list[j]
			}
		}
		out = append(out, list...)
	}
	var carried, stock, book []mercDraught
	for _, c := range ms.kit.Carried {
		pe := w.potions.byPattern[c.PatternID]
		if cost, power, ok := ms.draughtOf(pe); ok && c.Count > 0 && !c.Food {
			carried = append(carried, mercDraught{pe: *pe, carried: c.ID, cost: cost, power: power})
		}
	}
	for id, n := range w.mercSupply.stockOf(ms.owner) {
		pe := w.potions.byPattern[id]
		if cost, power, ok := ms.draughtOf(pe); ok && n > 0 {
			stock = append(stock, mercDraught{pe: *pe, stock: true, cost: cost, power: power})
		}
	}
	book = w.mercBookDraughtsLocked(ms)
	add(carried)
	add(stock)
	out = append(out, book...)
	return out
}

// mercBookDraughtsLocked is the draughts its book's refinements make (its
// path's own), the most power first. Must be called with the lock held.
func (w *World) mercBookDraughtsLocked(ms *mercState) []mercDraught {
	byName := map[string]*PotionEffect{}
	for _, pe := range w.potions.byPattern {
		byName[lowerTrim(pe.ItemName)] = pe
	}
	seen := map[int]bool{}
	var book []mercDraught
	for i := range ms.kit.Spells {
		s := &ms.kit.Spells[i]
		if containsInt(ms.kit.SpellsOff, s.SpellID) || s.Def.Summons() {
			continue
		}
		for _, it := range append(append([]SpellItem(nil), s.Def.Items...), unreadSummons(&s.Def)...) {
			pe := byName[lowerTrim(it.Name)]
			if cost, power, ok := ms.draughtOf(pe); ok && !seen[pe.PatternID] {
				seen[pe.PatternID] = true
				book = append(book, mercDraught{pe: *pe, cost: cost, power: power})
			}
		}
	}
	for i := 1; i < len(book); i++ {
		for j := i; j > 0 && book[j].power > book[j-1].power; j-- {
			book[j], book[j-1] = book[j-1], book[j]
		}
	}
	return book
}

// mercDraughtComing is the power the draughts on the merc have still to
// give: each tick's power for the ticks left of it.
func mercDraughtComing(n *NPC, now time.Time) int {
	coming := 0
	for _, e := range n.Effects {
		if _, ok := potionOfEffect(e.SpellID); !ok || e.PowerTick <= 0 {
			continue
		}
		if left := e.Until.Sub(now); left > 0 {
			coming += e.PowerTick * (int(left/effectTick) + 1)
		}
	}
	return coming
}

// mercHealerClass is whether the class heals: a cleric, druid or shaman.
func mercHealerClass(class int) bool { return class == 7 || class == 8 || class == 9 }

// mercHealerNearLocked is whether a healer stands in the merc's team (its
// owner, its group's players and mercs, alive in its world). Must be called
// with the lock held.
func (w *World) mercHealerNearLocked(ms *mercState) bool {
	for _, mm := range w.mercTeamLocked(ms) {
		switch {
		case mm.p != nil && mercHealerClass(mm.p.Class):
			return true
		case mm.n != nil && mm.n != ms.npc && mm.n.merc != nil && mercHealerClass(mm.n.merc.kit.Class):
			return true
		}
	}
	return false
}

// mercDrinkTurnLocked is the merc's draught when its power is low (the file's
// doc); it reports one drunk. Must be called with the lock held.
func (w *World) mercDrinkTurnLocked(ms *mercState, now time.Time) bool {
	m := ms.npc
	if !ms.mercThrowsPotions() || m.MaxPower <= 0 || (m.Power+mercDraughtComing(m, now))*100 >= m.MaxPower*mercDraughtBelow {
		return false
	}
	floor := m.MaxHP * mercDraughtHardFloor / 100
	if !w.mercHealerNearLocked(ms) {
		floor = max(floor, m.MaxHP*w.mercSupply.draughtFloor()/100)
	}
	for _, d := range w.mercDraughtsLocked(ms) {
		if m.HP-d.cost < max(floor, 1) {
			continue // it would leave it in danger
		}
		w.mercDrinkLocked(ms, d, now)
		return true
	}
	return false
}

// mercDrinkLocked drinks the draught: its health now, as a player's drink
// pays it (draughtResisted may spare it), the rest landed on the merc as a
// player's self spell on a pet (its power over time), shown. Must be called
// with the lock held.
func (w *World) mercDrinkLocked(ms *mercState, d mercDraught, now time.Time) {
	m, b := ms.npc, ms.body
	cost := d.cost
	if draughtResisted(b.StatNow(StatPR)) {
		cost = 0
	}
	m.HP = max(m.HP-cost, 1)
	spell := d.pe.castSpell()
	drunk := spell
	drunk.Def.Effects = nil
	for _, e := range spell.Def.Effects {
		if e.Kind != EffectHP || e.Amount >= 0 {
			drunk.Def.Effects = append(drunk.Def.Effects, e)
		}
	}
	cs := &castState{spell: drunk, caster: m.ServerID, targets: []EntityID{m.ServerID}}
	w.landOnPetLocked(cs, b, nil, m, now)
	syncMercBody(ms)
	w.mercUsedLocked(ms, d.carried, d.stock, d.pe.PatternID)
	w.broadcastNPCCastLocked(m, m.ServerID, 0, fxConsumed)
	log.Printf("Merc: %s's %q drinks %q: %d health, %d power to come (%d/%d HP, %d/%d power)", ms.owner.Name, m.Name, d.pe.ItemName,
		cost, d.power, m.HP, m.MaxHP, m.Power, m.MaxPower)
}

// mercEating is the merc's meal on it, nil none.
func mercEating(n *NPC) *ActiveEffect {
	for _, e := range n.Effects {
		if e.SpellID == FoodEffectID {
			return e
		}
	}
	return nil
}

// mercMealIcon is a meal's effect's icon, a player's (handlers/consume.go's
// consumableEffectIcon: Regenerative Aura's).
const mercMealIcon uint32 = 0x828563AF

// mercEatTurnLocked is the merc's meal out of a fight (the file's doc), and a
// fight's end of one; it reports one eaten. Must be called with the lock held.
func (w *World) mercEatTurnLocked(ms *mercState, now time.Time) bool {
	m := ms.npc
	if !ms.mercThrowsPotions() {
		return false
	}
	fight := w.mercInFightLocked(ms) || ms.target != 0
	if ae := mercEating(m); ae != nil {
		if fight || m.LastCombatTime.After(ae.Until.Add(-ConsumableDuration)) {
			w.dropNPCEffect(m, FoodEffectID) // a fight ends a meal, as a player's
			syncMercBody(ms)
		}
		return false
	}
	if fight || m.HP*100 >= m.MaxHP*mercEatBelow {
		return false
	}
	ration, _ := RationRestore(ms.body)
	name, hot, carried := "Ration", 0, 0
	for _, c := range ms.kit.Carried {
		v := c.HoT
		if v <= 0 {
			v = ration // a ration it carries: a player's (consumable.go)
		}
		if c.Food && c.Count > 0 && v > hot {
			name, hot, carried = c.Name, v, c.ID
		}
	}
	if carried == 0 {
		hot = ration
	}
	ae := &ActiveEffect{SpellID: FoodEffectID, Name: name, Icon: mercMealIcon, Caster: m.ServerID, Until: now.Add(ConsumableDuration)}
	ae.Mods.HoT = hot
	m.Effects = append(m.Effects, ae)
	m.HP = min(m.HP+hot, m.MaxHP) // its first tick at once (consumable.go)
	syncMercBody(ms)
	if carried != 0 {
		w.mercUsedLocked(ms, carried, false, 0)
	}
	w.broadcastNPCCastLocked(m, m.ServerID, 0, fxConsumed)
	log.Printf("Merc: %s's %q eats %q: %d health a tick (%d/%d HP)", ms.owner.Name, m.Name, name, hot, m.HP, m.MaxHP)
	return true
}

// dropNPCEffect takes the NPC's effects of the id off.
func (w *World) dropNPCEffect(n *NPC, id int) {
	keep := n.Effects[:0]
	for _, e := range n.Effects {
		if e.SpellID != id {
			keep = append(keep, e)
		}
	}
	for i := len(keep); i < len(n.Effects); i++ {
		n.Effects[i] = nil
	}
	n.Effects = keep
}

func lowerTrim(s string) string {
	b := []byte(s)
	i, j := 0, len(b)
	for i < j && (b[i] == ' ' || b[i] == '\t') {
		i++
	}
	for j > i && (b[j-1] == ' ' || b[j-1] == '\t') {
		j--
	}
	b = b[i:j]
	for k := range b {
		if b[k] >= 'A' && b[k] <= 'Z' {
			b[k] += 'a' - 'A'
		}
	}
	return string(b)
}

// MercBrewsFor is the potions (pattern ids) a merc of the kit keeps a stack
// of in its owner's stock (mercBrewWantsLocked), its body made from the kit:
// the handlers' tests'.
func (w *World) MercBrewsFor(kit MercKit) []int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	ms := &mercState{kit: kit, body: newMercBody(1, &kit)}
	var out []int
	for _, c := range w.mercBrewWantsLocked(ms) {
		out = append(out, c.pe.PatternID)
	}
	return out
}
