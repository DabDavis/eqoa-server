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
	AC                                int // 14
	// PoTItems and HoTItems are each worn item's PoT and HoT (12, 13), as
	// ItemCredit counts them (regen.go), not summed
	PoTItems, HoTItems     []int
	PR, DR, FR, CR, LR, AR int // 22-27
	// WeaponDamage is the primary weapon's itempattern.weapondamage (the
	// tooltip's DMG), 0 for none; WeaponCond its condition (hit points over
	// their most, 1 when the item holds none) and WeaponTwoHanded whether
	// it takes both hands. The player's melee rolls on them (offence.go).
	WeaponDamage    int
	WeaponCond      float64
	WeaponTwoHanded bool
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
		if v > 0 {
			g.PoTItems = append(g.PoTItems, v)
		}
	case 13:
		if v > 0 {
			g.HoTItems = append(g.HoTItems, v)
		}
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
// the worn items and the level: the stats it has (base and gear, each
// within the level's stat cap, StatCap; what spells add stays in Mods), the
// maxima from them (CalculateMaxHP of all the stamina, CalculateMaxPower of
// all the stats, each with the gear's own), the AC and the resists. Hit
// points and power are kept, within the new maxima; fill brings them to the
// maxima (a login).
func (p *Player) RecalcStats(fill bool) {
	g := &p.Gear
	c := &p.CMEffects // what its masteries give (cmbuy.go)
	p.Str = p.capStat(StatStr, p.BaseStr+g.Str+c[0])
	p.Sta = p.capStat(StatSta, p.BaseSta+g.Sta+c[1])
	p.Agi = p.capStat(StatAgi, p.BaseAgi+g.Agi+c[2])
	p.Dex = p.capStat(StatDex, p.BaseDex+g.Dex+c[3])
	p.Wis = p.capStat(StatWis, p.BaseWis+g.Wis+c[4])
	p.Int = p.capStat(StatInt, p.BaseInt+g.Int+c[5])
	p.Cha = p.capStat(StatCha, p.BaseCha+g.Cha+c[6])
	p.MaxHP = p.maxHPFor(p.Sta) + g.MaxHP + c[cmSlotHP]
	p.MaxPower = CalculateMaxPower(p.Level, p.Class, p.Str, p.Sta, p.Agi, p.Dex, p.Wis, p.Int, p.Cha) + g.MaxPower + c[cmSlotPower]
	p.AC = g.AC + c[cmSlotAC]
	p.PoisonR, p.DiseaseR, p.FireR = g.PR+c[cmSlotPR], g.DR+c[cmSlotDR], g.FR+c[cmSlotFR]
	p.ColdR, p.LightningR, p.ArcaneR = g.CR+c[cmSlotCR], g.LR+c[cmSlotLR], g.AR+c[cmSlotAR]
	// The old OFF/DEF placeholders (stats.go), which nothing rolls on any
	// more: an NPC's melee is defence.go's on the AC and AGI, the player's
	// offence.go's on the worn weapon (Gear). A mastery's OFF/DEF MOD only
	// shows.
	p.OffMod = CalculateOffMod(p.BaseStr, p.BaseDex, p.Level) + c[cmSlotOff]
	p.DefMod = CalculateDefMod(50, p.BaseAgi, p.Level) + c[cmSlotDef]
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
	return p.maxHPFor(p.StatNow(StatSta))
}

// maxHPFor is CalculateMaxHP for the player with a stamina, its masteries'
// HP FACTOR on the class's (Hearty: "Hearty increases your hit points").
func (p *Player) maxHPFor(sta int) int {
	return calculateMaxHPFactor(p.Level, hpFactor(p.Class)+p.CMEffects[cmSlotHPFactor], sta)
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
