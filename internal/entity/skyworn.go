package entity

import (
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// The period item list gives the Bard's +100 CHA and the Enchanter's five-minute
// form with +470 HP/+240 power. The owner authorized Heirloom estimates for the
// missing details on 2026-11-13. Ten percent per landed incoming melee hit,
// sixty seconds for the Bard buff and the existing pixie body are those estimates.
// Only Ascended copies receive these estimates. Live's unresolved details remain
// in the content research record for player confirmation.
const skyWornBuffChance = 10

func skyRaidWornBuff(base int, imported bool) *CastSpell {
	spell := CastSpell{SpellID: -5_000_000 - base, Icon: 0xDE1BC5EC,
		Def: SpellDef{Target: TargetSelf}}
	switch base {
	case 716023:
		spell.Name = "Seraphic Inspiration"
		spell.Def.Duration = time.Minute
		spell.Def.Effects = []SpellEffect{{Kind: EffectStat, Stat: StatCha, Amount: 100}}
	case 716026:
		spell.Name = "Seraphic Planar Form"
		spell.Def.Duration = 5 * time.Minute
		spell.Def.FormKind = FormSelf
		spell.Def.FormModel = 0x94BA77CD
		spell.Def.Effects = []SpellEffect{
			{Kind: EffectIllusion},
			{Kind: EffectStat, Stat: StatMaxHP, Amount: 470},
			{Kind: EffectStat, Stat: StatMaxPower, Amount: 240},
		}
	default:
		return nil
	}
	for i := range spell.Def.Effects {
		effect := &spell.Def.Effects[i]
		effect.Amount = database.AscendedFigure(effect.Amount)
		if imported {
			effect.Amount = database.ImportedFigure(effect.Amount)
		}
		effect.AmountMax = effect.Amount
	}
	return &spell
}

func (w *World) wornBuffsLocked(sess *session.Session, p *Player, now time.Time) {
	if p.IsDead() {
		return
	}
	for _, shield := range p.Gear.Shields {
		if shield.Buff == nil || procIntn(100) >= skyWornBuffChance {
			continue
		}
		spell := *shield.Buff
		spell.Level = p.Level
		cast := &castState{spell: spell, caster: p.ServerID, targets: []EntityID{p.ServerID}}
		w.landOnPlayerLocked(cast, p, sess, p, now)
	}
}
