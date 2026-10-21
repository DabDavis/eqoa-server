package entity

// Gear is what the worn items add, by the stat ids an item's stat list
// carries (handlers' itemPatternStats): the client's GetStatTotal sums them
// the same way, over every item whose equip slot is not -1, onto the stat
// channel's base half. On Sandstorm the stat channel's totals (bytes 0-27,
// 56, 88-111) are the base stats plus exactly these plus the spells', the
// maximum hit points the base maximum plus the worn items' hit points
// (stat 8), and the maximum power the power of all the stats plus the
// worn items' (stat 10), measured 2026-10-21.
type Gear struct {
	Str, Sta, Agi, Dex, Wis, Int, Cha int
	MaxHP, MaxPower                   int // stats 8 and 10
	PoT, HoT                          int // 12 and 13
	AC                                int // 14
	PR, DR, FR, CR, LR, AR            int // 22-27
	// WeaponDamage is the primary weapon's itempattern.weapondamage (the
	// tooltip's DMG), 0 for none. Not yet used: see RecalcStats.
	WeaponDamage int
}

// Add puts one worn item's stats, by stat id, into the sums.
func (g *Gear) Add(id, v int) {
	switch id {
	case 0:
		g.Str += v
	case 1:
		g.Sta += v
	case 2:
		g.Agi += v
	case 3:
		g.Dex += v
	case 4:
		g.Wis += v
	case 5:
		g.Int += v
	case 6:
		g.Cha += v
	case 8:
		g.MaxHP += v
	case 10:
		g.MaxPower += v
	case 12:
		g.PoT += v
	case 13:
		g.HoT += v
	case 14:
		g.AC += v
	case 22:
		g.PR += v
	case 23:
		g.DR += v
	case 24:
		g.FR += v
	case 25:
		g.CR += v
	case 26:
		g.LR += v
	case 27:
		g.AR += v
	}
}

// RecalcStats puts the player's stats back together from the base stats,
// the worn items and the level: the stats it has (base and gear; what
// spells add stays in Mods), the maxima from them (CalculateMaxHP of all the
// stamina, CalculateMaxPower of all the stats, each with the gear's own),
// the AC and the resists. Hit points and power are kept, within the new
// maxima; fill brings them to the maxima (a login).
func (p *Player) RecalcStats(fill bool) {
	g := &p.Gear
	p.Str = p.BaseStr + g.Str
	p.Sta = p.BaseSta + g.Sta
	p.Agi = p.BaseAgi + g.Agi
	p.Dex = p.BaseDex + g.Dex
	p.Wis = p.BaseWis + g.Wis
	p.Int = p.BaseInt + g.Int
	p.Cha = p.BaseCha + g.Cha
	p.MaxHP = CalculateMaxHP(p.Level, p.Class, p.Sta) + g.MaxHP
	p.MaxPower = CalculateMaxPower(p.Level, p.Class, p.Str, p.Sta, p.Agi, p.Dex, p.Wis, p.Int, p.Cha) + g.MaxPower
	p.AC = g.AC
	p.PoisonR, p.DiseaseR, p.FireR, p.ColdR, p.LightningR, p.ArcaneR = g.PR, g.DR, g.FR, g.CR, g.LR, g.AR
	// Combat keeps its old placeholders (stats.go) until Sandstorm's melee
	// and defense are measured: the AC of 50 and a weapon damage of level/3
	// that CalculateMeleeDamage was tuned on. The worn weapon's rating
	// (g.WeaponDamage, 560 on a level 50 sword) is some 35 times that, and
	// the worn AC up to 30 times, so neither may go in before the formulas.
	p.OffMod = CalculateOffMod(p.BaseStr, p.BaseDex, p.Level)
	p.DefMod = CalculateDefMod(50, p.BaseAgi, p.Level)
	p.WeaponDamage = p.Level / 3
	if fill {
		p.HP, p.Power = p.MaxHPNow(), p.MaxPowerNow()
	} else {
		p.HP, p.Power = min(p.HP, p.MaxHPNow()), min(p.Power, p.MaxPowerNow())
	}
}

// BaseMaxHP is the stat channel's base maximum hit points (byte 148) as
// Sandstorm sends it: what all the stamina gives, the worn and the spells'
// included, without anything's own hit points.
func (p *Player) BaseMaxHP() int {
	return CalculateMaxHP(p.Level, p.Class, p.StatNow(StatSta))
}

// BaseMaxPower is the stat channel's base maximum power (byte 156) as
// Sandstorm sends it: what the base stats alone give.
func (p *Player) BaseMaxPower() int {
	return CalculateMaxPower(p.Level, p.Class, p.BaseStr, p.BaseSta, p.BaseAgi, p.BaseDex, p.BaseWis, p.BaseInt, p.BaseCha)
}

// SetGear gives the player what its worn items add and recalculates its
// stats (fill: hit points and power to the maxima, as at a login).
func (w *World) SetGear(id EntityID, g Gear, fill bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if p, ok := w.Players[id]; ok {
		p.Gear = g
		p.RecalcStats(fill)
	}
}
