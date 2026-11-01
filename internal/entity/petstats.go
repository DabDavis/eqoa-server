package entity

import (
	"fmt"
	"log"
	"math"
	"sort"
	"time"

	"github.com/eqoa/server/internal/database"
)

// petstats.go — a summoned pet of the workbook's numbers (table petstats,
// database/petstats.go; the user's choice of 2026-11-01 for EQOAEmu's spell
// workbook: "Workbook wins, re-fit", then "workbook wins on all" and "the
// correct pet models are in the worksheet": its body and weapons over
// whatever this server's NPCs, aliases and stand-ins gave the pet, GMs'
// included). A summoning spell with a petstats row makes its pet out of the
// row:
//
//   - its body the row's (the template NPC its size and colours come from
//     one of this server's NPCs wearing that body, else the one pet.go would
//     have picked), its weapons the row's models with their kinds' attack
//     types on its hands, its name and level the row's (the level never
//     above its caster's);
//   - its hit points the row's in place of its level's, the balance's pet
//     columns on top as on any pet (balance.go: a re-fit's to tune), its
//     power, and its HoT (scaled as its hit points are) and PoT a tick;
//   - its armour, AGI and archetype take off a creature's hits on it as a
//     player's do (defence.go), its resists a creature's spells; its melee a
//     player's of its type with the row's weapon (offence.go: STR the top,
//     DEX the bottom), its off hand every other hit of a round;
//   - what it casts, the row's abilities (its own spells pinned by id,
//     through pettemplate.go's machinery), in place of its pet template's
//     (keep_template: those after them); the template row's class and swing
//     it keeps (the user's of 2026-10-30: the water pet a cleric, the earth
//     a tank that quads, a necromancer's pet a warrior's round), and with no
//     template row it swings as pet.go has it (an enchanter's once a round,
//     Grand Animation twice);
//   - the stances it takes are the row's ("Passive" alone: it fights only
//     when told to), and a non-combat pet (the workbook's contraptions and
//     swamp friends) fights nothing at all;
//   - none of petrotation.go's recorded necromancer rounds, openers and
//     instants, nor pettap.go's tap: its spells are the workbook's.
//
// A pet with no row is what it always was.

// petSheet is a petstats row as the world summons by it: its row, and its
// abilities with the spells they pin.
type petSheet struct {
	database.PetStats
	abilities []templateAbility
	arch      archetype
	problems  []string
}

// petSheetArch is the archetype a pet's type fights and defends as: a
// player's of it (no type, and a non-combat pet, a tank's: a pet's as
// pet.go had it).
func petSheetArch(typ string) archetype {
	switch typ {
	case database.PetTypeMelee:
		return archMelee
	case database.PetTypePriest:
		return archPriest
	case database.PetTypeCaster:
		return archCaster
	}
	return archTank
}

// pinnedTemplateSpell is the spell an ability pins, by id or by name.
func pinnedTemplateSpell(db *database.SQLiteDB, a database.TemplateAbility) (database.TemplateSpell, bool, error) {
	if a.SpellID != 0 {
		return db.TemplateSpellByID(a.SpellID)
	}
	return db.TemplateSpellNamed(a.Spell)
}

// pinWords is what an ability pins, for a problem's line.
func pinWords(a database.TemplateAbility) string {
	if a.SpellID != 0 {
		return fmt.Sprintf("no spell %d", a.SpellID)
	}
	return fmt.Sprintf("no spell named %q", a.Spell)
}

// loadPetSheets reads petstats: each row and its abilities' spells
// (LoadClassTemplates, at the start and after a GM's edit). A row the world
// cannot use is left out, a problem saying why.
func loadPetSheets(db *database.SQLiteDB) (map[int]*petSheet, []string, error) {
	rows, bad, err := db.PetStatsRows()
	if err != nil {
		return nil, nil, fmt.Errorf("petstats: %w", err)
	}
	var problems []string
	for _, b := range bad {
		problems = append(problems, "petstats "+b+" (its pets as before)")
	}
	ids := make([]int, 0, len(rows))
	for id := range rows {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	out := make(map[int]*petSheet, len(rows))
	for _, id := range ids {
		row := rows[id]
		s := &petSheet{PetStats: row, arch: petSheetArch(row.Type)}
		for i, ra := range row.Abilities {
			a := templateAbility{TemplateAbility: ra,
				every: time.Duration(ra.Every * float64(time.Second)), first: time.Duration(ra.First * float64(time.Second))}
			if ra.SpellID != 0 || ra.Spell != "" {
				ts, ok, err := pinnedTemplateSpell(db, ra)
				if err != nil {
					return nil, nil, fmt.Errorf("petstats %d: %w", id, err)
				}
				if ok {
					a.pinned = templateSpellOf(ts)
				} else {
					s.problems = append(s.problems, fmt.Sprintf("ability %d: %s (it casts nothing)", i+1, pinWords(ra)))
				}
			} else if ra.Kind != database.AbilityTaunt && !(ra.Kind == database.AbilityHeal && ra.HealPct > 0) {
				s.problems = append(s.problems, fmt.Sprintf("ability %d: pins no spell (it casts nothing)", i+1))
			}
			s.abilities = append(s.abilities, a)
		}
		for _, p := range s.problems {
			problems = append(problems, fmt.Sprintf("petstats %d (%s) %s", id, row.Name, p))
		}
		out[id] = s
	}
	return out, problems, nil
}

// petBodyLocked is the NPC a workbook pet is made after: of this server's
// own NPCs wearing its body (not pets, not a GM's copies), the most common
// size's, the lowest id; else the one pet.go would have picked, its body put
// on (a body no NPC wears: the disc has it); nil when there is neither. Must
// be called with the lock held.
func (w *World) petBodyLocked(s *petSheet, spellName, lineName string, class int) *NPC {
	if s.Model != 0 {
		var wearing []*NPC
		for _, npc := range w.NPCs {
			if !npc.Temporary && npc.Owner == 0 && uint32(npc.ModelID) == s.Model {
				wearing = append(wearing, npc)
			}
		}
		if len(wearing) > 0 {
			sizes := map[float32]int{}
			for _, npc := range wearing {
				sizes[npc.Size]++
			}
			sort.Slice(wearing, func(i, j int) bool {
				a, b := wearing[i], wearing[j]
				if sizes[a.Size] != sizes[b.Size] {
					return sizes[a.Size] > sizes[b.Size]
				}
				return a.ServerID < b.ServerID
			})
			return wearing[0]
		}
	}
	t := w.petTemplateLocked(spellName, lineName, class)
	if t == nil || s.Model == 0 {
		return t
	}
	body := *t
	body.ModelID = int64(s.Model)
	body.Size = 1
	body.PrimaryWeapon, body.SecondaryWeapon, body.Shield = 0, 0, 0
	return &body
}

// applyPetSheet makes a new pet the row's: its body, weapons and their
// hands, its numbers for the balance and the fights (the file's doc).
func applyPetSheet(pet *NPC, s *petSheet) {
	pet.sheet = s
	if s.Model != 0 {
		pet.ModelID = int64(s.Model)
	}
	// the row's weapons, none where it names none, and no shield (the
	// workbook gives a pet two hands: a shield would make a creature's
	// Attack3 of its swing from 35, rounds.go)
	pet.PrimaryWeapon, pet.SecondaryWeapon, pet.Shield = int(s.PrimaryModel), int(s.SecondaryModel), 0
	if t := database.PetWeaponTypes[s.Wpn1Type]; t != 0 {
		pet.PrimaryType, pet.SecondaryType = t, attackTypeEmptyHand
		if t2 := database.PetWeaponTypes[s.Wpn2Type]; t2 != 0 && s.Wpn2Dmg > 0 {
			pet.SecondaryType = t2
		}
	}
	pet.OffMod, pet.DefMod = s.OffMod, s.DefMod
	if s.Wpn1Dmg > 0 {
		pet.WeaponDamage = s.Wpn1Dmg
	}
}

// attackTypeEmptyHand is CLIENT's attack type of a hand that holds nothing.
const attackTypeEmptyHand byte = 11

// petBaseHP is a summoned pet's hit points before the balance's pet columns:
// its row's, else its level's.
func (n *NPC) petBaseHP() int {
	if n.sheet != nil && n.sheet.HP > 0 {
		return n.sheet.HP
	}
	return NPCMaxHP(n.Level)
}

// petStat is a workbook pet's stat with what its effects add, within its
// row's maximum for it; 0 for a pet with no row.
func (n *NPC) petStat(st Stat) int {
	s := n.sheet
	if s == nil {
		return 0
	}
	m := n.effectMods()
	base, add, top := 0, 0, 0
	switch st {
	case StatStr:
		base, add, top = s.Str, m.Str, s.StrMax
	case StatSta:
		base, add, top = s.Sta, m.Sta, s.StaMax
	case StatAgi:
		base, add, top = s.Agi, m.Agi, s.AgiMax
	case StatDex:
		base, add, top = s.Dex, m.Dex, s.DexMax
	case StatWis:
		base, add, top = s.Wis, m.Wis, s.WisMax
	case StatInt:
		base, add, top = s.Int, m.Int, s.IntMax
	case StatCha:
		base, add, top = s.Cha, m.Cha, s.ChaMax
	case StatAC:
		base, add = s.AC, m.AC
	case StatFR:
		base, add = s.FR, m.FR
	case StatCR:
		base, add = s.CR, m.CR
	case StatLR:
		base, add = s.LR, m.LR
	case StatPR:
		base, add = s.PR, m.PR
	case StatDR:
		base, add = s.DR, m.DR
	case StatAR:
		base, add = s.AR, m.AR
	}
	v := max(base+add, 0)
	if top > 0 {
		v = min(v, top)
	}
	return v
}

// sheetMeleeHit is a workbook pet's melee hit on a creature: a player's of
// its archetype with the row's weapon (offence.go's lines, the weapon whole:
// a pet's never wears), its STR and DEX what its row and effects give
// (petStat), its off hand's weapon on every other hit of a round when it
// has one. ok false for a pet with no weapon damage (petMeleeHit's
// creature roll).
func sheetMeleeHit(pet, t *NPC, intn func(int) int) (hit int, ok bool) {
	s := pet.sheet
	if s == nil || s.Wpn1Dmg <= 0 {
		return 0, false
	}
	dmg := s.Wpn1Dmg
	if s.Wpn2Dmg > 0 && pet.RoundHits%2 == 0 && pet.RoundHits > 0 {
		dmg = s.Wpn2Dmg // RoundHits counts the hit landing: its 2nd, 4th
	}
	d := float64(dmg)
	a := offenceArchs[s.arch]
	str, dex := float64(pet.petStat(StatStr)), float64(pet.petStat(StatDex))
	def := a.Defence * targetDefence(t)
	high := int(math.Round(offHighD*d + offHighSTR*str + a.High - def))
	low := min(int(math.Round(a.LowShare*(offLowD+offLowDEX*dex)*d+a.Low-def)), high)
	floor := int(math.Floor(offFloorD*d + offFloorSTR*str))
	n := low
	if high > low {
		n += intn(high - low + 1)
	}
	return t.meleeTaken(max(n, floor, 1)), true // as the target's class template takes it (classtemplate.go)
}

// sheetHitOn is a creature's hit on a workbook pet: its roll on a player of
// the pet's archetype, the row's AC and AGI with its effects' (defence.go's
// hitOn).
func sheetHitOn(npc, pet *NPC, intn func(int) int) int {
	r := npcMeleeRoll(npc)
	a := pet.sheet.arch
	top := int(math.Round(r.Top - r.ACSlope*float64(pet.petStat(StatAC)) - agiSlope[a]*float64(pet.petStat(StatAgi)) + archOffset[a]))
	hit := top - r.Width + intn(r.Width+1)
	return max(hit, r.Floor, 1)
}

// sheetFigure is a workbook pet's spell's figure: each hit or heal of its
// effects rolled, plus its scale's share of the pet's stat ("Increase 15%
// based on STR": the workbook's Modifier and Percentage), within its cap, at
// the ability's percent; and the drained share of the hits.
func sheetFigure(pet *NPC, effects []SpellEffect, pct int, heal bool) (n, drained int) {
	for _, e := range effects {
		if e.Kind != EffectHP || e.Self {
			continue
		}
		if heal && (e.Amount <= 0 || e.Drain != DrainNone) || !heal && e.Amount >= 0 {
			continue
		}
		v := rollAmount(e)
		if e.Scale != StatNone && e.ScalePct != 0 {
			share := int(math.Round(float64(pet.petStat(e.Scale)) * float64(e.ScalePct) / 100))
			if v < 0 {
				v -= share
			} else {
				v += share
			}
		}
		v = capAmount(v, e.Cap)
		if v < 0 {
			v = -v
		}
		v = int(math.Round(float64(v) * float64(pct) / 100))
		n += v
		if e.Drain != DrainNone {
			drained += v
		}
	}
	return n, drained
}

// petSheetTemplate is the pet template a workbook pet fights by: its
// summoning's pettemplate row's class, swing and round (base; none: it keeps
// pet.go's warrior class and swing), no share of its hit points, a melee
// hit on it or its own (the row's numbers are its own), and the row's
// abilities, then the base's when the row keeps them, at most
// MaxTemplateAbilities.
func petSheetTemplate(s *petSheet, spell *CastSpell, base *petTemplate) *petTemplate {
	ct := &ClassTemplate{ClassTemplate: database.ClassTemplate{Name: "workbook pet " + s.SheetPetID, Class: -1,
		HPPct: 100, TakenPct: 100, MeleePct: 100}}
	row := database.PetTemplate{Spell: database.TemplateKey(spell.Name), Template: ct.Name, SwingClass: -1, Swing: -1}
	ct.abilities = append(ct.abilities, s.abilities...)
	if base != nil {
		ct.Class = base.tmpl.Class
		ct.Name = base.tmpl.Name + " (workbook pet " + s.SheetPetID + ")"
		row.Weapon, row.SwingClass, row.Swing, row.RoundMS = base.Weapon, base.SwingClass, base.Swing, base.RoundMS
		if s.KeepTemplate {
			ct.abilities = append(ct.abilities, base.tmpl.abilities...)
		}
	}
	if len(ct.abilities) > database.MaxTemplateAbilities {
		ct.abilities = ct.abilities[:database.MaxTemplateAbilities]
	}
	row.Template = ct.Name
	return &petTemplate{PetTemplate: row, tmpl: ct}
}

// petSheetStance is a new workbook pet's stance: defensive when it takes
// that, else the first of its row's (passive alone: it fights only when
// told to).
func petSheetStance(s *petSheet) petStance {
	if s == nil || s.TakesStance(int(petDefensive)) {
		return petDefensive
	}
	for st := petPassive; st <= petAggressive; st++ {
		if s.TakesStance(int(st)) {
			return st
		}
	}
	return petPassive
}

// petFights is whether the pet takes a fight at all: not a workbook
// non-combat pet's.
func (n *NPC) petFights() bool {
	return n.sheet == nil || n.sheet.Type != database.PetTypeNonCombat
}

// petResistShare is the share of a creature's spell of the element a
// workbook pet resists (resist.go's roll of its resist), 0 for any other.
func (n *NPC) petResistShare(element Stat) float64 {
	if n.sheet == nil {
		return 0
	}
	if element == StatNone {
		element = StatAR
	}
	share, _ := RollResist(n.petStat(element))
	return share
}

// petSheetRegen is a workbook pet's tick (npcregen.go's petRegen): its row's
// HoT, scaled as the balance scaled its hit points, and PoT; a 50th of each
// maximum where its row gives none.
func (n *NPC) petSheetRegen() {
	s := n.sheet
	hot, pot := max(1, n.MaxHP/50), max(1, n.MaxPower/50)
	if s.HoT > 0 {
		hot = s.HoT
		if base := n.petBaseHP(); base > 0 && n.MaxHP != base {
			hot = max(1, int(math.Round(float64(s.HoT)*float64(n.MaxHP)/float64(base))))
		}
	}
	if s.PoT > 0 {
		pot = s.PoT
	}
	if n.HP > 0 && n.HP < n.MaxHP {
		n.HP = min(n.HP+hot, n.MaxHP)
	}
	if n.MaxPower > 0 && n.Power < n.MaxPower {
		n.Power = min(n.Power+pot, n.MaxPower)
	}
}

// petSheetSummaryLocked is the workbook's pets for the server's log
// (ClassTemplateSummary).
func (w *World) petSheetSummaryLocked() string {
	n := 0
	for _, s := range w.templates.sheets {
		n += len(s.abilities)
	}
	return fmt.Sprintf("%d summonings make a pet of its petstats row (%d abilities)", len(w.templates.sheets), n)
}

// logPetSheet is a workbook pet's summoning, for the log.
func logPetSheet(pet *NPC, s *petSheet) {
	log.Printf("Pet: %q is workbook pet %s (%s): body %#x, AC %d, STR %d DEX %d AGI %d, weapons %d %s / %d %s, %d abilities",
		pet.Name, s.SheetPetID, s.Type, uint32(pet.ModelID), s.AC, s.Str, s.Dex, s.Agi, s.Wpn1Dmg, s.Wpn1Type, s.Wpn2Dmg,
		s.Wpn2Type, len(s.abilities))
}
