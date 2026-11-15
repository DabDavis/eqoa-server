package entity

import (
	"log"
	"time"
)

// mercbrew.go — what an alchemist merc makes and takes (the owner,
// 2026-11-15: "the alc mercs should constantly make pots and they are saved
// for future mercs also", then "alchemists mercs should use draughts to
// restore mana, hurts hp, they eat to restore hp, throw the pots and cast
// spells").
//
// Its owner's stock: while out and alive an alchemist merc makes one of its
// path's potions (the one it would throw of its own kind, /merc aoe or
// single: mercBrewPickLocked) every brew interval (world.merc_brew_seconds,
// MercBrewEvery by default) into its owner's stock, the whole stock held to
// world.merc_brew_cap potions (MercBrewCap). The stock is its owner's, not
// the merc's: the handlers keep it in table charmercstock and hand the World
// its counts at each login (SetMercStock), so it outlives a dismissal, a
// death, a logout and a restart, and an alchemist hired later throws from it.
// Its owner takes potions out of it through the merc's menu (handlers).
//
// Its draughts: below mercDraughtBelow of its power, out of a cast, with no
// draught working on it, it drinks the best power draught it has (a Self
// potion whose line gives power over time and costs health: the Watery to the
// Infused Draught), from what it carries, else its owner's stock, else its
// path's own (what its book's refinements summon, conjured as its throws'),
// its figures a player's (cast.go's spellAmount on its body): the health
// taken at once (a player's draught's poison resist may spare it:
// draughtResisted), the power over the draught's time. Never when the health
// it costs would leave it under world.merc_draught_hp_floor_pct of its most
// (MercDraughtFloorPct by default), nor one whose cost is more than it has.
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
// world.merc_brew_cap): one potion every 30 s while out, at most 200 in the
// stock (ours: twice a pack's stack of a common potion's 100, so a stock
// lasts some fights and a player's withdrawal leaves some).
const (
	MercBrewEvery = 30 * time.Second
	MercBrewCap   = 200
)

// MercDraughtFloorPct is the share of its most health below which a draught
// would leave an alchemist merc, so it does not drink
// (world.merc_draught_hp_floor_pct).
const MercDraughtFloorPct = 50

// When it drinks and eats (ours).
const (
	// mercDraughtBelow: it drinks a draught below this share of its power
	mercDraughtBelow = 30
	// mercEatBelow: out of a fight it eats below this share of its health
	mercEatBelow = 70
)

// mercSupply is the World's alchemists' supply: each owner's stock by item
// (the handlers' rows), and the rates.
type mercSupply struct {
	stock map[EntityID]map[int]int // by owner's entity id
	every time.Duration
	cap   int
	floor int // percent of its most health a draught keeps it above
	set   bool
}

func (s *mercSupply) brewEvery() time.Duration {
	if s.set && s.every > 0 {
		return s.every
	}
	return MercBrewEvery
}

func (s *mercSupply) brewCap() int {
	if s.set && s.cap >= 0 {
		return s.cap
	}
	return MercBrewCap
}

func (s *mercSupply) draughtFloor() int {
	if s.set && s.floor >= 0 {
		return s.floor
	}
	return MercDraughtFloorPct
}

// stockOf is the owner's stock, nil before the handlers gave it.
func (s *mercSupply) stockOf(p *Player) map[int]int {
	if p == nil || s.stock == nil {
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
// each its default).
func (w *World) SetMercBrew(every time.Duration, cap, floorPct int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.mercSupply.every, w.mercSupply.cap, w.mercSupply.floor, w.mercSupply.set = every, cap, min(floorPct, 100), true
	if every < 0 {
		w.mercSupply.every = 0
	}
}

// MercBrewCapNow is the stock's cap as set (MercBrewCap by default).
func (w *World) MercBrewCapNow() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.mercSupply.brewCap()
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
	}
	st := make(map[int]int, len(stock))
	for id, n := range stock {
		if n > 0 {
			st[id] = n
		}
	}
	w.mercSupply.stock[p.ServerID] = st
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

// mercBrewPickLocked is the potion the merc makes: the best of its path's of
// its own kind (/merc aoe or single), else its path's best. Must be called
// with the lock held.
func (w *World) mercBrewPickLocked(ms *mercState) (int, bool) {
	aoe := ms.mercAoE()
	book := w.mercThrowablesLocked(ms)
	for _, c := range book {
		if c.spell == 0 && (c.pe.Area > 0) == aoe {
			return c.pe.PatternID, true
		}
	}
	for _, c := range book {
		if c.spell == 0 {
			return c.pe.PatternID, true
		}
	}
	return 0, false
}

// mercBrewLocked makes the merc's potion into its owner's stock when its
// time has come (the file's doc). Must be called with the lock held.
func (w *World) mercBrewLocked(ms *mercState, now time.Time) {
	if !ms.mercThrowsPotions() {
		return
	}
	st := w.mercSupply.stockOf(ms.owner)
	if st == nil {
		return // its owner's stock not read yet
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
	if cap := w.mercSupply.brewCap(); cap > 0 && w.mercSupply.total(ms.owner) >= cap {
		return
	}
	id, ok := w.mercBrewPickLocked(ms)
	if !ok {
		return
	}
	st[id]++
	w.noteMercLocked(MercNote{Kind: MercNoteBrewed, Owner: ms.owner, Slot: ms.slot, Name: ms.npc.Name, PatternID: id, Count: 1})
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
	byName := map[string]*PotionEffect{}
	for _, pe := range w.potions.byPattern {
		byName[lowerTrim(pe.ItemName)] = pe
	}
	seen := map[int]bool{}
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
	add(carried)
	add(stock)
	add(book)
	return out
}

// mercDrinkingLocked is whether a draught still works on the merc.
func mercDrinking(n *NPC) bool {
	for _, e := range n.Effects {
		if _, ok := potionOfEffect(e.SpellID); ok && e.PowerTick > 0 {
			return true
		}
	}
	return false
}

// mercDrinkTurnLocked is the merc's draught when its power is low (the file's
// doc); it reports one drunk. Must be called with the lock held.
func (w *World) mercDrinkTurnLocked(ms *mercState, now time.Time) bool {
	m := ms.npc
	if !ms.mercThrowsPotions() || m.MaxPower <= 0 || m.Power*100 >= m.MaxPower*mercDraughtBelow || mercDrinking(m) {
		return false
	}
	floor := m.MaxHP * w.mercSupply.draughtFloor() / 100
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
	name, hot, carried := "Ration", 0, 0
	for _, c := range ms.kit.Carried {
		if c.Food && c.Count > 0 && c.HoT > hot {
			name, hot, carried = c.Name, c.HoT, c.ID
		}
	}
	if carried == 0 {
		hot, _ = RationRestore(ms.body)
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
