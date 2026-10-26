package entity

import (
	"errors"
	"log"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/eqoa/server/internal/session"
)

// potion.go — what using an item does: the alchemists' potions (F3
// 20261025-223151, -223841 and -223959, jib's account of the live game; pass
// 5's 61b2f4e2: their summoning spells did nothing).
//
// An alchemist's spell summons the potions ("SELF Summon Watery Draught",
// "Summons Unstable Potion X3", EQOA-Data's words kept as spelleffect has
// them): at its landing the spell's words name an item, and the handlers give
// the caster that many of it (summonItemsLocked: the item by name, count the
// line's "xN", else three of an item that stacks to three and one of
// anything else). Using one does what its row of table itemeffect says
// (database/itemeffect.go): a line in spelleffect's words, its first word
// whom it lands on.
//
//   - Drunk ("Self, ..."): a draught turns health into power (its health
//     taken at once, refused when that would kill), a speed potion is Spirit
//     of the Wolf and an invisibility potion invisibility, each on the effects
//     list by its own name and icon, a tonic a resist, a corrective or a
//     restorative takes harmful effects off ("Dispel Effect xN"), a returning
//     potion sends the drinker home.
//   - Thrown ("Target, ..."): its hit and what lasts of it land on the
//     creature targeted, as a spell's would (landOnNPCLocked), within the
//     potion's throwing range, and on every creature within its splash
//     (itemeffect.area: the cocktails'). A thrown potion is also worn as a
//     thrown weapon (wear 17, attack type 10), and then each throw of the
//     auto-attack is one of them, gone from the stack (thrownPotionHitLocked).
//
// What ends one early is its row's: "attack" (jib: a quickening "break[s] on
// any aggressive action done by the character": its auto-attack, a harmful
// cast or a throw), "damage" (a sublimation "drops on aggressive action ... or
// upon taking any damage": LastCombatTime, a hit taken or dealt, past its
// start), and wear, a percent chance each six-second tick (a sublimation's
// 1%). An effect's id is potionEffectBase less the item's id, so a second
// potion of a kind replaces the first and the rules come back after a logout.
//
// The numbers are EQOA-Data's where its lists give them (the "Mix Potion"
// lines are the thrown potions' hits) and ours elsewhere: the draughts mirror
// the alchemist's infusions of their levels, the tonics past the second rank
// and the throwing range are chosen (tools/seed_potions.py writes them, and a
// GM can change any of them).

// potionEffectBase is under every potion's effect id: the item's id is taken
// from it. No spell has a negative id, and food and drink's are -1001/-1002.
const potionEffectBase = -1_000_000

// PotionRange is how far a potion is thrown: ours, no record gives one.
const PotionRange = 20

// PotionEffectID is the id of the effect an item's potion puts on.
func PotionEffectID(patternID int) int { return potionEffectBase - patternID }

// potionOfEffect is the item an effect id is a potion's, if it is one.
func potionOfEffect(id int) (int, bool) {
	if id > potionEffectBase {
		return 0, false
	}
	return potionEffectBase - id, true
}

// PotionEffect is what using an item does (an itemeffect row, read).
type PotionEffect struct {
	PatternID      int
	ItemName       string
	Name           string // on the effects list
	Icon           uint32 // its icon there
	Def            SpellDef
	Area           float32 // a thrown one's splash round its target
	BreaksOnAttack bool
	BreaksOnDamage bool
	WearPct        int
	Cures          int // harmful effects a drink takes off
}

// reDispel is a cure's words in a line ("Dispel Effect x3"): spelltext.go
// leaves them unread.
var reDispel = regexp.MustCompile(`^(?:dispel|cure|remove)\s+(?:harmful\s+)?effects?(?:\s+x\s*(\d+))?$`)

// NewPotionEffect reads an itemeffect row.
func NewPotionEffect(patternID int, itemName, line, name string, icon uint32, breaks string, wear int, area float64) PotionEffect {
	pe := PotionEffect{PatternID: patternID, ItemName: itemName, Name: name, Icon: icon,
		Def: ParseSpellText(line), Area: float32(area), WearPct: wear}
	if strings.TrimSpace(pe.Name) == "" {
		pe.Name = itemName
	}
	for _, b := range strings.Split(strings.ToLower(breaks), ",") {
		switch strings.TrimSpace(b) {
		case "attack":
			pe.BreaksOnAttack = true
		case "damage":
			pe.BreaksOnDamage = true
		}
	}
	for _, u := range pe.Def.Unread {
		if m := reDispel.FindStringSubmatch(strings.TrimSpace(u)); m != nil {
			pe.Cures = 1
			if m[1] != "" {
				pe.Cures, _ = strconv.Atoi(m[1])
			}
		}
	}
	return pe
}

// Thrown is whether the potion is thrown at a target rather than drunk.
func (pe *PotionEffect) Thrown() bool { return pe.Def.Target != TargetSelf }

// duration is how long what lasts of it lasts, as lasting() takes it.
func (pe *PotionEffect) duration() time.Duration {
	if pe.Def.Duration > 0 {
		return pe.Def.Duration
	}
	for _, e := range pe.Def.Effects {
		if e.Kind == EffectHPTick || e.Kind == EffectPowerTick {
			return tickDuration
		}
	}
	return 0
}

// castSpell is the potion as a spell for the cast code's landing: its effect
// id, name and icon, its line, scope 0 (self) or 1 (target).
func (pe *PotionEffect) castSpell() CastSpell {
	s := CastSpell{SpellID: PotionEffectID(pe.PatternID), Name: pe.Name, Icon: pe.Icon, Def: pe.Def}
	if pe.Thrown() {
		s.Scope, s.Range = 1, PotionRange
	}
	return s
}

// SummonItemFunc gives a character count of the item named (0: the item's own
// count), summoned by the spell; it runs on its own goroutine, never under the
// World's lock (handlers/potion.go).
type SummonItemFunc func(sess *session.Session, charServerID uint32, item string, count int, spell string)

// ThrownPotionFunc takes one of the item off the character's thrown stack:
// an auto-attack threw it. Its own goroutine, as SummonItemFunc.
type ThrownPotionFunc func(sess *session.Session, charServerID uint32, patternID int)

// potionTable is the World's potions: every item's use, by its id, and the
// handlers' hooks.
type potionTable struct {
	byPattern map[int]*PotionEffect
	onSummon  SummonItemFunc
	onThrown  ThrownPotionFunc
}

func (t *potionTable) note(pe PotionEffect) {
	if t.byPattern == nil {
		t.byPattern = make(map[int]*PotionEffect)
	}
	t.byPattern[pe.PatternID] = &pe
}

// SetPotionEffects is every item's use, loaded at the start.
func (w *World) SetPotionEffects(list []PotionEffect) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.potions.byPattern = make(map[int]*PotionEffect, len(list))
	for _, pe := range list {
		w.potions.note(pe)
	}
}

// NotePotionEffect keeps an item's use as read now (a GM may have changed it).
func (w *World) NotePotionEffect(pe PotionEffect) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.potions.note(pe)
}

// ThrownPotion is whether the item is a potion thrown at a target.
func (w *World) ThrownPotion(patternID int) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	pe := w.potions.byPattern[patternID]
	return pe != nil && pe.Thrown()
}

// OnSummonItem sets the hook that gives a spell's summoned items.
func (w *World) OnSummonItem(fn SummonItemFunc) {
	w.mu.Lock()
	w.potions.onSummon = fn
	w.mu.Unlock()
}

// OnPotionThrown sets the hook that takes a thrown potion off its stack.
func (w *World) OnPotionThrown(fn ThrownPotionFunc) {
	w.mu.Lock()
	w.potions.onThrown = fn
	w.mu.Unlock()
}

// The refusal Use answers a draught whose health would kill its drinker:
// ours, the game has no word for it.
const PotionTooWeak = "You are too weak to drink that."

// ErrPotionNoPlayer is UsePotion with no living player.
var ErrPotionNoPlayer = errors.New("no living player")

// UsePotion is a potion used: drunk, or thrown at the creature the client
// names. It answers the refusal (red, the cast's words where it has them:
// "No target.", "Target is friendly.", "Out of range."), "" when it was used
// and one is to be taken off the stack.
func (w *World) UsePotion(sess *session.Session, pe PotionEffect, targetObjectID uint32, now time.Time) (string, error) {
	refusal, gate, err := w.usePotion(sess, pe, targetObjectID, now)
	if gate != nil {
		w.runGates([]respawnDue{*gate})
	}
	return refusal, err
}

func (w *World) usePotion(sess *session.Session, pe PotionEffect, targetObjectID uint32, now time.Time) (string, *respawnDue, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || p.IsDead() {
		return "", nil, ErrPotionNoPlayer
	}
	w.potions.note(pe)
	spell := pe.castSpell()
	if pe.Thrown() {
		targets, refusal := w.castTargetsLocked(sess, p, &spell, targetObjectID)
		if refusal != "" {
			return refusal, nil, nil
		}
		npc := w.NPCs[targets[0]]
		if npc == nil {
			return castNoTarget, nil, nil
		}
		// played as a spell's is: the thrower's own swing is the client's
		// only in an auto-attack
		w.broadcastCastLocked(p, npc.ServerID, 0, spell.visual())
		w.throwLocked(sess, p, &pe, npc, now)
		log.Printf("Potion: %s throws %q at %s", p.Name, pe.ItemName, npc.Name)
		return "", nil, nil
	}

	cost := 0
	gate := false
	for _, e := range pe.Def.Effects {
		switch {
		case e.Kind == EffectHP && e.Amount < 0:
			cost -= spellAmount(e, p)
		case e.Kind == EffectGate:
			gate = true
		}
	}
	if cost > 0 && p.HP <= cost {
		return PotionTooWeak, nil, nil
	}
	if cost > 0 {
		p.HP -= cost
		p.StatsDirty = true
		w.tellAmountLocked(p, sess, p.ServerID, -cost)
	}
	if pe.Cures > 0 {
		w.curePlayerLocked(p, pe.Cures)
	}
	cs := &castState{spell: spell, caster: p.ServerID, targets: []EntityID{p.ServerID}}
	w.landOnPlayerLocked(cs, p, sess, p, now)
	log.Printf("Potion: %s drinks %q (%d health)", p.Name, pe.ItemName, cost)
	if gate {
		return "", &respawnDue{sess: sess, p: p}, nil
	}
	return "", nil, nil
}

// throwLocked lands a thrown potion on npc, and on every creature that may be
// fought within its splash. Throwing is an attack: what a potion of the
// thrower's own ends on an attack ends, invisibility too.
func (w *World) throwLocked(sess *session.Session, p *Player, pe *PotionEffect, npc *NPC, now time.Time) {
	cs := &castState{spell: pe.castSpell(), caster: p.ServerID, targets: []EntityID{npc.ServerID}}
	w.landOnNPCLocked(cs, p, sess, npc, now)
	if pe.Area > 0 {
		for _, id := range w.SpatialIndex.QueryRadius(npc.X, npc.Z, pe.Area) {
			o := w.NPCs[id]
			if o == nil || o == npc || o.Dead || o.World != npc.World || o.Owner != 0 || o.NPCType&0x80 != 0 {
				continue
			}
			if dist3(o.X, o.Y, o.Z, npc.X, npc.Y, npc.Z) > pe.Area {
				continue
			}
			w.landOnNPCLocked(cs, p, sess, o, now)
		}
	}
	w.breakInvisLocked(p, "a throw") // invis.go
	w.breakPotionsLocked(p, func(pe *PotionEffect) bool { return pe.BreaksOnAttack }, "a throw")
}

// thrownPotionHitLocked is an auto-attack's throw of the potion worn as its
// thrown weapon: the potion lands, one goes off the stack, and with the last
// thrown the fight is let go. It reports whether the fight ended.
func (w *World) thrownPotionHitLocked(sess *session.Session, p *Player, npc *NPC, objectID uint32) bool {
	pe := w.potions.byPattern[p.Swing.Potion]
	if pe == nil || !pe.Thrown() {
		p.disengage()
		return true
	}
	w.throwLocked(sess, p, pe, npc, time.Now())
	p.Swing.PotionLeft--
	if fn := w.potions.onThrown; fn != nil {
		go fn(sess, p.CharServerID, pe.PatternID)
	}
	if p.Swing.PotionLeft <= 0 {
		log.Printf("Potion: %s threw the last %q", p.Name, pe.ItemName)
		p.Swing.Potion = 0
		return true
	}
	return npc.Dead || npc.HP <= 0
}

// reSummonItem is a summoning of an item in a line's unread words: "summon
// watery draught", "summons unstable potion x3" (after any "(sow)" is cut).
var (
	reSummonItem = regexp.MustCompile(`^summ[ou]ns?\s+(?:an?\s+|the\s+)?(.+?)(?:\s+x\s*(\d+))?$`)
	reParens     = regexp.MustCompile(`\s*\([^)]*\)`)
)

// summonItemsLocked hands a spell's summoned items to the hook, at its
// landing. It reports whether the spell summoned any (and so does nothing
// else).
func (w *World) summonItemsLocked(cs *castState, caster *Player, sess *session.Session) bool {
	d := &cs.spell.Def
	if d.Summons() {
		return false // a pet (pet.go)
	}
	found := false
	for _, u := range d.Unread {
		m := reSummonItem.FindStringSubmatch(strings.TrimSpace(reParens.ReplaceAllString(u, "")))
		if m == nil {
			continue
		}
		found = true
		count, _ := strconv.Atoi(m[2])
		log.Printf("Cast: %s's %q summons %q (x%d)", caster.Name, cs.spell.Name, m[1], count)
		if fn := w.potions.onSummon; fn != nil && sess != nil {
			go fn(sess, caster.CharServerID, m[1], count, cs.spell.Name)
		}
	}
	return found
}

// curePlayerLocked takes up to n harmful effects off a player (a corrective's
// or a restorative's), the oldest first.
func (w *World) curePlayerLocked(p *Player, n int) {
	for _, ae := range append([]*ActiveEffect(nil), p.Effects...) {
		if n <= 0 {
			return
		}
		if ae.HPTick < 0 || ae.Root || ae.Snare || ae.Mods.harmful() {
			w.removeEffectLocked(p, ae.SpellID)
			log.Printf("Potion: %q cured off %s", ae.Name, p.Name)
			n--
		}
	}
}

// harmful is whether the changes take anything off.
func (m StatMods) harmful() bool {
	for _, v := range []int{m.Str, m.Sta, m.Agi, m.Dex, m.Wis, m.Int, m.Cha, m.AC, m.MaxHP, m.MaxPower,
		m.FR, m.CR, m.PR, m.DR, m.LR, m.AR, m.HoT, m.PoT} {
		if v < 0 {
			return true
		}
	}
	return false
}

// breakPotionsLocked ends the player's potion effects that end on what
// happened.
func (w *World) breakPotionsLocked(p *Player, ends func(*PotionEffect) bool, why string) {
	for _, ae := range append([]*ActiveEffect(nil), p.Effects...) {
		id, ok := potionOfEffect(ae.SpellID)
		if !ok {
			continue
		}
		if pe := w.potions.byPattern[id]; pe != nil && ends(pe) {
			w.removeEffectLocked(p, ae.SpellID)
			log.Printf("Potion: %q ends on %s: %s", ae.Name, p.Name, why)
		}
	}
}

// potionSystem ends what potions put on when it ends early: an attack (auto-
// attack engaged, a harmful cast begun), a hit since it began, or its chance
// each six-second tick. Every tick; must be called with the lock held.
func (w *World) potionSystem(now time.Time) {
	if len(w.potions.byPattern) == 0 {
		return
	}
	for _, p := range w.Players {
		if len(p.Effects) == 0 {
			continue
		}
		attacking := p.AttackTarget != 0 || (p.Casting != nil && p.Casting.spell.Def.Harmful && now.Before(p.Casting.lands))
		for _, ae := range append([]*ActiveEffect(nil), p.Effects...) {
			id, ok := potionOfEffect(ae.SpellID)
			if !ok {
				continue
			}
			pe := w.potions.byPattern[id]
			if pe == nil {
				continue
			}
			start := ae.Until.Add(-pe.duration())
			why := ""
			switch {
			case pe.BreaksOnAttack && attacking:
				why = "an attack"
			case pe.BreaksOnDamage && p.LastCombatTime.After(start):
				why = "a hit"
			case pe.WearPct > 0:
				if !ae.NextTick.After(start) {
					ae.NextTick = start.Add(effectTick)
				}
				if now.Before(ae.NextTick) {
					continue
				}
				ae.NextTick = ae.NextTick.Add(effectTick)
				if rand.Intn(100) < pe.WearPct {
					why = "it wore off"
				}
			}
			if why != "" {
				w.removeEffectLocked(p, ae.SpellID)
				log.Printf("Potion: %q ends on %s: %s", ae.Name, p.Name, why)
			}
		}
	}
}
