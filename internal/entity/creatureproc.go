package entity

import (
	"fmt"
	"log"
	"math"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// creatureproc.go — the melee procs a creature always has beside its template's own (proc_chance,
// creatureshield.go) and the one an ability's landing gives it for a fight (abilitytimes.go), and its
// procs on a creature it fights (the owner, 2026-11-10, of Innoruuk: "lets make innoruuk have same
// abilities as red swarming death with Largesse of Plague", then "give innoruuk similar to veil of death,
// but scale it to innoruuks hp").
//
//   - A proc (database.AbilityProc) rolls at each of its melee hits that lands: its pinned spell's proc,
//     as a buff's proc on a pet is (proc.go: the figure of its proc line, a range rolled, a bearer's
//     scale taken from three a level as a pet's every stat is; the chance its spell names, else a pet's
//     of its level under it: 3% and 1 more for each 20 of a pet's INT, three a level, at most 25%: 13%
//     at Innoruuk's 70), cut by its spell's resist; or one of its own figures (proc_chance, proc_damage,
//     proc_name, resist). Largesse of Plague (700794, the necromancer's pet buff) is a proc of 650
//     disease, DR.
//   - A veil (database.AbilityVeil) is at every melee hit that lands, not a share of them: a tap of
//     TapPct percent of its own most hit points (1% of Innoruuk's 500,002: 5,000), so a change of its
//     hit points changes it, from what it hit into itself, cut by its resist (AR, as Veil of Death's own
//     proc is: spells 901430 and 1431, a self buff whose offensive proc takes 40 and heals its caster as
//     much on every hit): what the resist leaves lands, and heals it the same; a resist that takes all of
//     it (a creature's strong resist cannot: at least 1 lands) leaves nothing to heal.
//
// On a player either is told as the template's proc is (the creature's 0x00B1 with Sandstorm's creature
// special's visual, or its own fx, a 0x00DB, a line, a death; the player's resist rolled on the
// template's dice, resist.go); on a pet its workbook resists take their share (petstats.go), any other
// takes it whole; on a creature it fights in a scripted fight (scriptedFoes, classtemplate.go), the
// creature's template resists of the kind take theirs (creatureResists), and the hit is the fight's
// (castFoeHitLocked: its hate, its heal at a hit, its death). Since petition 1437 a scripted fight's
// melee hits roll the template's own proc and the fight's on the creature too, where they rolled on
// players and pets alone. An add of a share rolls them at its share (summon.go). Ours: the lines.

// templateProc is a proc a template always has (AbilityProc): its chance (0: a pet's of its level), its
// figure's range and a bearer's scale (ScaleBearer's stat share: a pet's stand-in), its cap, the resist
// that cuts it, whether it fires on undead alone, its name and visual.
type templateProc struct {
	chance       int
	least, most  int
	scalePct     int
	cap          int
	resist       Stat
	undeadOnly   bool
	name         string
	fx           uint32
	spellID      int
	fromItsSpell bool
}

// templateVeil is a veil a template has (AbilityVeil): its share of the creature's most hit points, the
// resist that cuts it, its name and visual.
type templateVeil struct {
	pct    float64
	resist Stat
	name   string
	fx     uint32
}

// veilOf is a veil ability as the world rolls it.
func veilOf(a database.TemplateAbility) templateVeil {
	name := a.Name
	if name == "" {
		name = database.DefaultVeilName
	}
	return templateVeil{pct: a.TapPct, resist: shieldResistStat(a.Resist), name: name, fx: a.FX}
}

// procOf is a proc ability as the world rolls it: its own figures, or its pinned spell's proc (a
// problem's words for a spell that is none or has no proc).
func procOf(db *database.SQLiteDB, a database.TemplateAbility) (templateProc, string, error) {
	if a.Spell == "" && a.SpellID == 0 {
		name := a.ProcName
		if name == "" {
			name = database.DefaultFightProcName
		}
		return templateProc{chance: a.ProcChance, least: a.ProcDamage, most: a.ProcDamage, resist: shieldResistStat(a.Resist),
			name: name, fx: a.FX}, "", nil
	}
	ts, ok, err := pinnedTemplateSpell(db, a)
	if err != nil || !ok {
		return templateProc{}, pinWords(a), err
	}
	s := templateSpellOf(ts)
	for _, e := range s.Def.Effects {
		if e.Kind != EffectProc {
			continue
		}
		p := templateProc{chance: s.Def.ProcChance, least: e.Amount, most: max(e.AmountMax, e.Amount), cap: e.Cap,
			resist: e.Resist, undeadOnly: e.Undead, name: s.Name, spellID: s.SpellID, fromItsSpell: true}
		if p.resist == StatNone {
			p.resist = rulesOf(&s.Def).Resist
		}
		if e.ScaleBearer && e.Scale != StatNone {
			p.scalePct = e.ScalePct
		}
		return p, "", nil
	}
	return templateProc{}, fmt.Sprintf("%q is no proc buff", s.Name), nil
}

// chanceFor is the proc's chance a hit for npc: its own, else a pet's of npc's level under it (proc.go).
func (p *templateProc) chanceFor(npc *NPC) int {
	if p.chance > 0 {
		return p.chance
	}
	return procChancePct(petIntPerLvl * npc.figureLevel())
}

// rollFor is one of the proc's figures for npc: its range rolled on the template's dice, a bearer's scale
// of three a level, within its cap, at an add's share.
func (p *templateProc) rollFor(npc *NPC) int {
	n := p.least
	if p.most > p.least {
		n += templateIntn(p.most - p.least + 1)
	}
	if p.scalePct != 0 {
		n += petIntPerLvl * npc.figureLevel() * p.scalePct / 100
	}
	if p.cap > 0 {
		n = min(n, p.cap)
	}
	return npc.shared(n)
}

// visual is a proc's 0x00B1 visual: its own, else Sandstorm's creature special's.
func procVisual(fx uint32) uint32 {
	if fx != 0 {
		return fx
	}
	return templateSpecialFX
}

// tapOf is a veil's take for npc: its share of npc's most hit points, at least 1.
func (v *templateVeil) tapOf(npc *NPC) int {
	return max(1, int(math.Round(float64(npc.MaxHP)*v.pct/100)))
}

// abilityProcsOnPlayerLocked rolls npc's own procs and veils after one of its melee hits landed on p,
// standing (the file's doc). It reports whether p died of one. Must be called with the lock held.
func (w *World) abilityProcsOnPlayerLocked(sess *session.Session, p *Player, npc *NPC, now time.Time) bool {
	t := npc.tmpl
	for i := range t.procs {
		pr := &t.procs[i]
		if p.IsDead() || pr.undeadOnly || templateIntn(100) >= pr.chanceFor(npc) {
			continue
		}
		if w.creatureProcStatHitLocked(sess, p, npc, pr.name, pr.rollFor(npc), pr.resist, procVisual(pr.fx), now) {
			return true
		}
	}
	for i := range t.veils {
		if p.IsDead() || npc.Dead {
			break
		}
		v := &t.veils[i]
		take := v.tapOf(npc)
		share, how := 0.0, ResistFailed
		if v.resist != StatNone {
			share, how = t.rollResist(p.StatNow(v.resist))
		}
		taken := int(math.Round(float64(take) * (1 - share)))
		if taken <= 0 {
			continue
		}
		w.broadcastNPCSwingLocked(npc, p.ServerID, 0, procVisual(v.fx))
		p.HP = max(p.HP-taken, 0)
		p.StatsDirty = true
		p.LastCombatTime = now
		if sess != nil {
			if nid := w.objectIDForEntityLocked(sess, npc.ServerID); nid != 0 {
				w.sendDamageIndication(sess, nid, -taken, sess.PlayerObjectID(), 0) // the creature's on the player: untagged
			}
			words := ""
			if v.resist != StatNone {
				words = " (" + resistWords(share, how) + ")"
			}
			w.sendChatToSession(sess, fmt.Sprintf("%s's %s takes %d from you%s.", npc.Name, v.name, taken, words))
		}
		w.veilHealLocked(npc, v, taken, sess)
		log.Printf("Template: %s's %s takes %d of %d from %s (resist %.0f%%; HP %d/%d)", npc.Name, v.name, taken, take, p.Name,
			share*100, p.HP, p.MaxHPNow())
		if p.HP <= 0 {
			w.playerDiedLocked(sess, p, now)
			if p.IsDead() {
				return true
			}
		}
	}
	return false
}

// abilityProcsOnPetLocked is abilityProcsOnPlayerLocked on a pet: a workbook pet's resists take their
// share, any other takes it whole. It reports whether the pet died of one. Must be called with the lock
// held.
func (w *World) abilityProcsOnPetLocked(npc, pet *NPC, now time.Time) bool {
	t := npc.tmpl
	for i := range t.procs {
		pr := &t.procs[i]
		if pet.Dead || pr.undeadOnly || templateIntn(100) >= pr.chanceFor(npc) {
			continue
		}
		if w.creatureProcPetHitStatLocked(npc, pet, pr.name, pr.rollFor(npc), pr.resist, procVisual(pr.fx), now) {
			return true
		}
	}
	for i := range t.veils {
		if pet.Dead || npc.Dead {
			break
		}
		v := &t.veils[i]
		take := v.tapOf(npc)
		share := 0.0
		if v.resist != StatNone {
			share = pet.petResistShare(v.resist)
		}
		taken := int(math.Round(float64(take) * (1 - share)))
		if taken <= 0 {
			continue
		}
		w.broadcastNPCSwingLocked(npc, pet.ServerID, 0, procVisual(v.fx))
		pet.HP = max(pet.HP-taken, 0)
		pet.LastCombatTime = now
		w.veilHealLocked(npc, v, taken, nil)
		log.Printf("Template: %s's %s takes %d from the pet %q (HP %d/%d)", npc.Name, v.name, taken, pet.Name, pet.HP, pet.MaxHP)
		if pet.HP == 0 {
			w.killNPC(pet)
			w.dropAggroLocked(pet.ServerID)
			return true
		}
	}
	return false
}

// creatureProcOnNPCLocked rolls npc's melee procs after one of its melee hits of n landed on foe, the
// creature of a scripted fight, standing (the file's doc): its template's own, the one its fight has,
// its own procs, and its veils, each cut by foe's template resists of its kind and landing as the fight's
// hit (castFoeHitLocked). It reports whether foe died of one. Must be called with the lock held.
func (w *World) creatureProcOnNPCLocked(npc, foe *NPC, n int, now time.Time) bool {
	t := npc.tmpl
	if t == nil || foe.Dead || npc.Dead {
		return foe.Dead
	}
	land := func(name string, dmg int, resist Stat, fx uint32) bool {
		taken := -creatureResists(resist, foe, -dmg, now)
		w.broadcastNPCSwingLocked(npc, foe.ServerID, 0, fx)
		w.castFoeHitLocked(npc, foe, taken, name, now)
		return foe.Dead
	}
	if t.ProcChance > 0 && t.ProcDamage > 0 && templateIntn(100) < t.ProcChance &&
		land(procName(t), npc.shared(t.ProcDamage), shieldResistStat(t.ProcResist), templateSpecialFX) {
		return true
	}
	if fp := npc.fightProcNow(); fp != nil && templateIntn(100) < fp.chance && land(fp.name, fp.damage, StatNone, templateSpecialFX) {
		return true
	}
	for i := range t.procs {
		pr := &t.procs[i]
		if pr.undeadOnly && !w.undeadLocked(foe) || templateIntn(100) >= pr.chanceFor(npc) {
			continue
		}
		if land(pr.name, pr.rollFor(npc), pr.resist, procVisual(pr.fx)) {
			return true
		}
	}
	for i := range t.veils {
		if npc.Dead {
			break
		}
		v := &t.veils[i]
		take := v.tapOf(npc)
		taken := -creatureResists(v.resist, foe, -take, now)
		w.broadcastNPCSwingLocked(npc, foe.ServerID, 0, procVisual(v.fx))
		w.castFoeHitLocked(npc, foe, taken, v.name, now)
		w.veilHealLocked(npc, v, taken, nil)
		if foe.Dead {
			return true
		}
	}
	return false
}

// veilHealLocked heals npc what its veil took, up to its most, told to sess (whom it hit's client, or nil).
// Must be called with the lock held.
func (w *World) veilHealLocked(npc *NPC, v *templateVeil, taken int, sess *session.Session) {
	if npc.Dead || taken <= 0 || npc.HP >= npc.MaxHP {
		return
	}
	heal := min(taken, npc.MaxHP-npc.HP)
	npc.HP += heal
	w.tellNPCHealLocked(sess, npc, heal)
}

// creatureProcStatHitLocked is creatureProcHitLocked with the resist a Stat and a visual of its own: one
// of a creature's procs on a player, the figure cut by that resist (StatNone none), its 0x00B1, a line and
// any death. It reports whether the player died of it. Must be called with the lock held.
func (w *World) creatureProcStatHitLocked(sess *session.Session, p *Player, npc *NPC, name string, damage int, resist Stat, fx uint32,
	now time.Time) bool {
	taken, words := damage, ""
	if resist != StatNone {
		share, how := npc.tmpl.rollResist(p.StatNow(resist))
		taken = max(int(math.Round(float64(taken)*(1-share))), 1)
		words = " (" + resistWords(share, how) + ")"
	}
	w.broadcastNPCSwingLocked(npc, p.ServerID, 0, fx)
	p.HP = max(p.HP-taken, 0)
	p.StatsDirty = true
	p.LastCombatTime = now
	if sess != nil {
		if nid := w.objectIDForEntityLocked(sess, npc.ServerID); nid != 0 {
			w.sendDamageIndication(sess, nid, -taken, sess.PlayerObjectID(), 0) // the creature's on the player: untagged
		}
		w.sendChatToSession(sess, fmt.Sprintf("%s's %s hits you for %d%s.", npc.Name, name, taken, words))
	}
	log.Printf("Template: %s's %s hits %s for %d of %d (HP %d/%d)", npc.Name, name, p.Name, taken, damage, p.HP, p.MaxHPNow())
	if p.HP > 0 {
		return false
	}
	w.playerDiedLocked(sess, p, now)
	return p.IsDead() // a GM's #testmode stands at 1 HP: the hit goes on
}

// creatureProcPetHitStatLocked is creatureProcPetHitLocked with the resist a Stat and a visual of its own.
// It reports whether the pet died of it. Must be called with the lock held.
func (w *World) creatureProcPetHitStatLocked(npc, pet *NPC, name string, damage int, resist Stat, fx uint32, now time.Time) bool {
	taken := damage
	if resist != StatNone {
		if share := pet.petResistShare(resist); share > 0 {
			taken = max(int(math.Round(float64(taken)*(1-share))), 1)
		}
	}
	w.broadcastNPCSwingLocked(npc, pet.ServerID, 0, fx)
	pet.HP = max(pet.HP-taken, 0)
	pet.LastCombatTime = now
	log.Printf("Template: %s's %s hits the pet %q for %d (HP %d/%d)", npc.Name, name, pet.Name, taken, pet.HP, pet.MaxHP)
	if pet.HP > 0 {
		return false
	}
	w.killNPC(pet)
	w.dropAggroLocked(pet.ServerID)
	return true
}

// procsWords is #stats' words for a template's own procs and veils, "" for none.
func procsWords(npc *NPC) []string {
	t := npc.tmpl
	var out []string
	for i := range t.procs {
		pr := &t.procs[i]
		fig := fmt.Sprintf("%d", pr.least)
		if pr.most > pr.least {
			fig = fmt.Sprintf("%d-%d", pr.least, pr.most)
		}
		out = append(out, fmt.Sprintf("proc %s: %d%% of its melee hits %s more (%s)", pr.name, pr.chanceFor(npc), fig, statName(pr.resist)))
	}
	for i := range t.veils {
		v := &t.veils[i]
		out = append(out, fmt.Sprintf("veil %s: every melee hit takes %g%% of its hit points (%d) into itself (%s)", v.name, v.pct, v.tapOf(npc),
			statName(v.resist)))
	}
	return out
}

// statName is a resist's short name in #stats, "unresisted" for none.
func statName(s Stat) string {
	switch s {
	case StatFR:
		return "FR"
	case StatCR:
		return "CR"
	case StatLR:
		return "LR"
	case StatAR:
		return "AR"
	case StatPR:
		return "PR"
	case StatDR:
		return "DR"
	}
	return "unresisted"
}
