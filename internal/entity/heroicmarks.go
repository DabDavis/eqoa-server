package entity

import (
	"fmt"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// LoadHeroicMarks enables the feature after checking the reference taunts and
// installing the catalog. A load failure leaves the reward paths closed.
func (w *World) LoadHeroicMarks(db *database.SQLiteDB) error {
	book := database.NewSpellbookStore(db)
	taunts := make(map[int][]SpellEffect)
	for id := range database.HeroicTauntSpellIDs() {
		row, err := book.SpellPattern(id)
		if err != nil {
			return fmt.Errorf("read taunt %d: %w", id, err)
		}
		fx, _, err := book.SpellEffectRow(id)
		if err != nil {
			return fmt.Errorf("read taunt %d effects: %w", id, err)
		}
		spell := SpellOfRow(row, 0, fx.Line, fx.Effects, fx.FX)
		for _, e := range spell.Def.Effects {
			if e.Kind == EffectHate && e.Amount > 0 && !e.Self {
				taunts[id] = append(taunts[id], e)
			}
		}
		if len(taunts[id]) == 0 || !isTaunt(&spell) {
			return fmt.Errorf("reference spell %d is not a taunt", id)
		}
	}
	if err := db.InstallHeroicMarks(); err != nil {
		return err
	}
	w.mu.Lock()
	w.heroicMarks, w.heroicTaunts = true, taunts
	w.mu.Unlock()
	return nil
}

func (w *World) HeroicMarksEnabled() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.heroicMarks
}

func (w *World) heroicMarkBossLocked(npc *NPC) string {
	if w.heroicMarks && npc.Instance != 0 && npc.mythic != nil && npc.ServerID == npc.mythic.boss {
		return EncounterMythicNagafen
	}
	return ""
}

func heroicElement(element string) Stat {
	switch element {
	case "fire":
		return StatFR
	case "cold":
		return StatCR
	case "lightning":
		return StatLR
	case "arcane":
		return StatAR
	case "poison":
		return StatPR
	case "disease":
		return StatDR
	}
	return StatNone
}

func (w *World) heroicDamageLocked(sess *session.Session, p *Player, npc *NPC, amount int, element string, now time.Time) bool {
	damage := -creatureResists(heroicElement(element), npc, -amount, now)
	w.broadcastCastLocked(p, npc.ServerID, 0, fxShieldHit)
	return w.spellHitLocked(p, sess, npc, damage, numberProc)
}

// Each landed auto attack rolls the worn relic alongside the usual weapon
// and effect procs. Threat is additive, including when another player leads.
func (w *World) heroicAttackProcsLocked(sess *session.Session, p *Player, npc *NPC, now time.Time) bool {
	if archetypeOf(p.Class) == archTank {
		for _, id := range p.Gear.HeroicRelics {
			role, _, ok := database.HeroicRelicOf(id)
			spell := role.Taunts[p.Class]
			if !ok || spell == 0 || procIntn(100) >= role.AttackChance {
				continue
			}
			var hate int
			for _, effect := range w.heroicTaunts[spell] {
				hate += spellAmount(effect, p)
			}
			w.updateAggro(npc, p.ServerID, int(pctOf(int64(playerTauntHate(hate)), playerTauntHatePct(p.Class))))
		}
	}
	return p.heroicDamageProcs(false, func(amount int, element string) bool {
		return w.heroicDamageLocked(sess, p, npc, amount, element, now)
	})
}

func (p *Player) heroicDamageProcs(spell bool, hit func(int, string) bool) bool {
	for _, id := range p.Gear.HeroicRelics {
		role, element, ok := database.HeroicRelicOf(id)
		if !ok || !role.Matches(p.Class) {
			continue
		}
		chance, damage := role.AttackChance, role.AttackDamage
		if spell {
			chance, damage = role.SpellChance, role.SpellDamage
		}
		if damage > 0 && procIntn(100) < chance && hit(damage, element) {
			return true
		}
	}
	return false
}

// One roll per spell with landed damage effects, including DoTs and area
// casts. Copies of the cast for splash and resists share its roll state.
func (cs *castState) takeHeroicSpellRoll() bool {
	if cs.heroicProcUsed == nil || *cs.heroicProcUsed {
		return false
	}
	for _, e := range cs.spell.Def.Effects {
		if (e.Kind == EffectHP || e.Kind == EffectHPTick) && e.Amount < 0 && !e.Self {
			*cs.heroicProcUsed = true
			return true
		}
	}
	return false
}

func (w *World) heroicDuelProcsLocked(d *Duel, p *Player, m duelMark, spell bool, now time.Time) bool {
	return p.heroicDamageProcs(spell, func(amount int, element string) bool {
		resist := heroicElement(element)
		w.broadcastCastLocked(p, m.id(), 0, fxShieldHit)
		if m.t != nil {
			damage := d.hit(amount)
			if !legacyResists() && elementStat(resist) {
				damage = resolvePlayerElemental(playerElem{d0: amount, resist: m.t.StatNow(resist), fight: d.share()}).Taken
			}
			return w.duelHurtAtLocked(d, p, p.ServerID, m.t, damage, amount, now, numberProc)
		}
		damage := m.pet.duelElementalHit(amount, resist, now)
		return damage > 0 && w.duelHurtMarkLocked(d, p, m, damage, now, numberProc)
	})
}

func heroicHealBonus(p *Player) int {
	var pct int
	for _, id := range p.Gear.HeroicRelics {
		if role, _, ok := database.HeroicRelicOf(id); ok && role.Matches(p.Class) {
			pct += role.HealCharismaPct
		}
	}
	if pct == 0 {
		return 0
	}
	return p.StatNow(StatCha) * pct / 100
}
