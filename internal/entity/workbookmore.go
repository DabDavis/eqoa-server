package entity

import (
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// workbookmore.go — the rest of what EQOAEmu's spell workbook says, which the
// first reading kept as unsupported (the user, 2026-11-01: "lets add these").
// Each rule acts on a spell read from the workbook's effects (spelljson.go);
// where the workbook's legend gives the rule it is the legend's, else it is
// ours and says so.
//
//   - Procs that do more than hit or heal (a hate, a stat taken, a tick,
//     power): the proc casts them as a spell of its own, its bearer the
//     caster, at the proc's chance (an offensive one's, proc.go: by the
//     bearer's INT; a defensive one at every hit, the legend's "Defensive
//     procs have 100% proc rate on hit"), on the creature its bearer hits or
//     that hits its bearer, resisted as any workbook spell (workbook.go); its
//     effects for its bearer ("Self") on the bearer. A pet's do not fire yet.
//   - Melee resists (ours, on the legend's words: Pierce, Blunt, Slash and
//     Ranged Debuff are "Debuff to specific resist of enemy", and its known
//     issues have "Ranged" as pierce): a creature under a debuff of N takes
//     N% more from a player's melee of that kind, the weapon's attack type
//     deciding (1-2 slash, 3-4 and bare hands blunt, 5-6 and the ranged 7, 9
//     and 10 pierce), and resists that basis' spells the less, as an
//     element's debuff lowers its (npcResistKindPct).
//   - OFF Mods and DEF Mods (the legend: "Essentially increases relative
//     level of attacker over monster" and "of defender over monster"): the
//     bearer's melee counts its foe OFF levels lower (the defence offence.go
//     takes off a hit by the foe's level), and a creature's melee on the
//     bearer is a creature's of DEF levels lower (defence.go's roll by the
//     creature's level); a training dummy keeps its measured roll.
//   - HP Factors (the legend: "Increase to MAX hp based on your level (60 hp
//     at lvl 60)"): maximum hit points the factor times the level more.
//   - Pickpocket (ours): a rogue's hand in a living creature's pockets takes
//     tunar, its level to twice it as a corpse's (handlers/corpsecoins.go),
//     with a chance of 50%, 4 a level the rogue is above it (less below),
//     and a fifth of its DEX over 100, from 5% to 95%; caught, the creature
//     fights; once in ten minutes a creature, then it has nothing left. No
//     item yet (the loot is rolled at the death, handlers/loot.go).
//   - Faction change (the workbook's FactionSet holds nothing; the effects
//     are the race illusions' and glamours'): a disguise, ours: while it
//     lasts no NPC of a faction attacks the bearer on sight by its standing
//     (aggro.go's faction rule), the creatures hostile by nature still do.
//   - Summon player (Call of the Hero): its caster's far group is asked, and
//     whoever accepts is brought beside it (callhero.go; until 2026-11-10 the
//     members in its range were brought, unasked, and no far one).
//   - Required equipment (the legend: "Spell must check equipped weapon types
//     before cast"): refused unless a hand holds one of the weapons it names
//     (1HS, 2HS, 1HB, 2HB, 1HP, 2HP, Range, Shield; None bare hands), in red
//     (ours: "You do not have the right weapon for that.").
//   - Target types Animal and Summoned: refused at anything else, as a spell
//     for undead alone is (undead.go): an animal is wildlife (wildlife.go), a
//     summoned creature a pet or one whose name's last word is a summoned
//     kind's (summonedWords, ours); "This spell only works on animals." and
//     "... on summoned creatures." (ours, the undead line's words).

// ProcSpell is one proc of a proc buff that casts more than a hit: what it
// does, how long what lasts of it lasts, its chance (0: an offensive proc's,
// by its bearer's INT), and whether a hit on the bearer sets it off.
type ProcSpell struct {
	Defense bool `json:",omitempty"`
	// Undead: it fires on undead creatures alone (the proc's own target type)
	Undead   bool          `json:",omitempty"`
	Effects  []SpellEffect `json:",omitempty"`
	Duration time.Duration `json:",omitempty"`
	Chance   int           `json:",omitempty"`
}

// picks reports a pickpocket.
func (d *SpellDef) picks() bool {
	for _, e := range d.Effects {
		if e.Kind == EffectPickpocket {
			return true
		}
	}
	return false
}

// procSpellID is the id a proc's own spell lands under on a creature: its
// buff's negated and offset, so it neither takes the buff's place nor is
// taken for the buff.
func procSpellID(buff, i int) int { return -2_000_000 - buff*4 - i }

// procSpellsLocked fires a player's spell procs on a creature: the offensive
// ones after one of its melee hits on it (defense false), the defensive ones
// after the creature's hit on it. Must be called with the lock held.
func (w *World) procSpellsLocked(p *Player, sess *session.Session, npc *NPC, defense bool, now time.Time) {
	w.procSpellsOnLocked(p, sess, defense, now, func() bool { return !npc.Dead }, func() bool { return w.undeadLocked(npc) }, npc.Name, func(cs *castState) {
		cs.targets = []EntityID{npc.ServerID}
		w.landOnNPCLocked(cs, p, sess, npc, now)
	})
}

// procSpellsOnLocked fires p's spell procs of the kind at what standing and undead ask about
// (named name, for the log), land landing each: a creature (procSpellsLocked) or a duel's
// (duelprocs.go). Must be called with the lock held.
func (w *World) procSpellsOnLocked(p *Player, sess *session.Session, defense bool, now time.Time,
	standing, undead func() bool, name string, land func(*castState)) {
	for _, ae := range append([]*ActiveEffect(nil), p.Effects...) {
		if !now.Before(ae.Until) {
			continue
		}
		for i, ps := range ae.ProcSpells {
			if ps.Defense != defense || !standing() || p.IsDead() || (ps.Undead && !undead()) {
				continue
			}
			chance := ps.Chance
			if chance <= 0 {
				chance = procChancePct(p.StatNow(StatInt))
			}
			if procIntn(100) >= chance {
				continue
			}
			var own, on []SpellEffect
			for _, e := range ps.Effects {
				if e.Self {
					own = append(own, e)
				} else {
					on = append(on, e)
				}
			}
			var lasts []SpellEffect
			for _, e := range own { // its share for its bearer
				switch n := spellAmount(e, p); e.Kind {
				case EffectHP:
					w.procHealLocked(p, sess, p, n)
				case EffectPower:
					p.Power = min(max(p.Power+n, 0), p.MaxPowerNow())
					p.StatsDirty = true
				case EffectHPTick, EffectPowerTick:
					e.Self = false // on its bearer as a spell's own target
					lasts = append(lasts, e)
				}
			}
			// a heal over time set off by a hit (Fervor's, Tribal Ward's "waits to
			// work till target is hit"): its own effect, under the proc's id, so
			// another hit renews it and never adds a second
			if len(lasts) > 0 {
				w.landOnPlayerLocked(&castState{spell: CastSpell{SpellID: procSpellID(ae.SpellID, i), Name: ae.Name, Icon: ae.Icon, Level: p.Level,
					Def: SpellDef{Effects: lasts, Duration: ps.Duration, Workbook: true}, figures: figuresProc}, caster: p.ServerID}, p, sess, p, now)
			}
			if len(on) == 0 {
				continue
			}
			log.Printf("Proc: %s's %q casts on %s", p.Name, ae.Name, name)
			land(&castState{spell: CastSpell{SpellID: procSpellID(ae.SpellID, i), Name: ae.Name, Icon: ae.Icon, Level: p.Level,
				Scope: 1, Def: SpellDef{Effects: on, Duration: ps.Duration, Harmful: true, Workbook: true}, figures: figuresProc},
				caster: p.ServerID})
		}
	}
}

// meleeKindOf is the melee resist a weapon's attack type hits (the file's
// doc).
func meleeKindOf(attackType byte) Stat {
	switch attackType {
	case 1, 2:
		return StatSlash
	case 5, 6, 7, 9, 10:
		return StatPierce
	}
	return StatBlunt
}

// npcMeleeDebuffPct is how many percent more a creature takes from melee of
// a kind: its strongest debuff of that melee resist.
func npcMeleeDebuffPct(npc *NPC, kind Stat, now time.Time) int {
	most := 0
	for _, e := range npc.Effects {
		if !now.Before(e.Until) {
			continue
		}
		var v int
		switch kind {
		case StatPierce:
			v = e.Mods.Pierce
		case StatBlunt:
			v = e.Mods.Blunt
		case StatSlash:
			v = e.Mods.Slash
		}
		most = max(most, -v)
	}
	return most
}

// meleeDebuffed is a player's melee hit of n on a creature with its debuff
// of the weapon's kind: the main hand's, or a ranged weapon's when it is the
// ranged one that swings (Gear.WeaponSlot 18: no weapon in hand, whose main
// hand reads as bare).
func meleeDebuffed(p *Player, npc *NPC, n int, now time.Time) int {
	if pct := npcMeleeDebuffPct(npc, meleeKindOf(p.heldType()), now); pct > 0 {
		return n * (100 + pct) / 100
	}
	return n
}

// heldType is the attack type of the weapon it swings: the main hand's, or a ranged
// weapon's when it is the ranged one that swings (Gear.WeaponSlot 18).
func (p *Player) heldType() byte {
	if p.Gear.WeaponSlot == 18 {
		return byte(p.Gear.WeaponType)
	}
	return p.PrimaryType
}

// equipCodes are the workbook's weapon codes by the attack types that hold
// them (handlers' attack types: 1 and 2 slashing one- and two-handed, 3 and
// 4 crushing, 5 and 6 piercing, 7 a bow, 9 a crossbow, 10 thrown).
var equipCodes = map[string][]byte{"1HS": {1}, "2HS": {2}, "1HB": {3}, "2HB": {4}, "1HP": {5}, "2HP": {6},
	"RANGE": {7, 9, 10}, "RANGED": {7, 9, 10}}

// ReqEquipCodes is a spell's required equipment as equipFits reads it: its
// codes, upper case (1HS ... 2HP, RANGE, SHIELD, NONE, ANY), in its order.
func ReqEquipCodes(req string) []string {
	var out []string
	for _, tok := range strings.Split(strings.ToUpper(req), ",") {
		if tok = strings.TrimSpace(tok); tok != "" {
			out = append(out, tok)
		}
	}
	return out
}

// EquipCodeTypes is the attack types a weapon's code names (equipCodes):
// "1HS" 1, "RANGE" 7, 9 and 10; none for any other.
func EquipCodeTypes(code string) []byte { return equipCodes[code] }

// wantsRanged is whether a spell's required equipment names a ranged weapon
// (Range): the casts whose landing in melee reach gives back the hands a
// switch took off to take one up (cast.go handsBackLocked).
func wantsRanged(req string) bool {
	for _, code := range ReqEquipCodes(req) {
		if code == "RANGE" || code == "RANGED" {
			return true
		}
	}
	return false
}

// handEmpty is a hand with no weapon: nothing, or bare hands (11).
func handEmpty(t byte) bool { return t == 0 || t == 11 }

// equipFits reports whether the player's hands hold what a spell's required
// equipment names (any one of its codes).
func equipFits(p *Player, req string) bool {
	for _, tok := range strings.Split(strings.ToUpper(req), ",") {
		switch tok = strings.TrimSpace(tok); tok {
		case "", "ANY":
			return true
		case "SHIELD":
			if p.Shield != 0 {
				return true
			}
		case "NONE":
			if handEmpty(p.PrimaryType) && handEmpty(p.SecondaryType) {
				return true
			}
		default:
			for _, t := range equipCodes[tok] {
				if p.PrimaryType == t || p.SecondaryType == t {
					return true
				}
			}
		}
	}
	return false
}

// HandsFit is equipFits for the handlers: whether the hands a player's worn
// look gives it (PrimaryType, SecondaryType, Shield) hold what a spell's
// required equipment names; the switch for an art checks the hands it would
// make with it before it makes them (handlers/handswitch.go).
func HandsFit(p *Player, req string) bool { return equipFits(p, req) }

// castWrongWeapon refuses a cast whose required equipment no hand holds
// (ours).
const castWrongWeapon = "You do not have the right weapon for that."

// equipRefusal is a cast's refusal for its required equipment, "" when the
// hands hold it.
func equipRefusal(p *Player, req string) string {
	if equipFits(p, req) {
		return ""
	}
	log.Printf("Cast: %s's hands (%d, %d, shield %v) hold none of %q", p.Name, p.PrimaryType, p.SecondaryType, p.Shield != 0, req)
	return castWrongWeapon
}

// The refusals of a spell for animals or for summoned creatures alone (ours,
// the undead line's words).
const (
	castNotAnimal   = "This spell only works on animals."
	castNotSummoned = "This spell only works on summoned creatures."
)

// summonedWords are the last words of a summoned creature's name (ours): the
// elementals and their kin, golems, animations, familiars.
var summonedWords = map[string]bool{"elemental": true, "elementaling": true, "elementalkin": true, "golem": true,
	"animation": true, "familiar": true, "servant": true, "minion": true, "phantasm": true, "construct": true}

// summonedLocked reports a summoned creature (the file's doc).
func (w *World) summonedLocked(npc *NPC) bool {
	if npc.Owner != 0 {
		return true
	}
	f := strings.Fields(strings.ToLower(npc.Name))
	return len(f) > 0 && summonedWords[f[len(f)-1]]
}

// targetKindStartLocked refuses a spell for animals or summoned creatures
// alone at anything else, as the cast begins. Must be called with the lock
// held.
func (w *World) targetKindStartLocked(d *SpellDef, targets []EntityID) (refusal string, fizzle bool) {
	for _, id := range targets {
		npc := w.NPCs[id]
		if npc == nil {
			continue
		}
		switch d.TargetKind {
		case "animal":
			if _, wild := w.wildlifeLocked(npc); !wild {
				return castNotAnimal, true
			}
		case "summoned":
			if !w.summonedLocked(npc) {
				return castNotSummoned, true
			}
		}
	}
	return "", false
}

// Pickpocket's chances and words (ours, the file's doc).
const (
	pickBase     = 50
	pickPerLevel = 4
	pickDexOver  = 100
	pickDexPer   = 5
	pickLeast    = 5
	pickMost     = 95
	pickAgain    = 10 * time.Minute

	pickTook   = "You pick %d tunar from %s's pockets."
	pickCaught = "%s catches you trying to pick its pockets!"
	pickEmpty  = "%s has nothing left to take."
)

// pickIntn is pickpocket's dice (a test's own).
var pickIntn = rand.Intn

// pickChancePct is a pickpocket's chance, in percent.
func pickChancePct(p *Player, npc *NPC) int {
	n := pickBase + pickPerLevel*(p.Level-npc.Level) + (p.StatNow(StatDex)-pickDexOver)/pickDexPer
	return min(max(n, pickLeast), pickMost)
}

// PurseFunc saves a character's purse (handlers'): run on its own goroutine,
// never under the World's lock.
type PurseFunc func(charServerID uint32, tunar int)

// OnPurse sets the hook a change to a player's purse made here goes through.
func (w *World) OnPurse(fn PurseFunc) {
	w.mu.Lock()
	w.onPurse = fn
	w.mu.Unlock()
}

// pickpocketLocked is a pickpocket landing on a creature. Must be called
// with the lock held.
func (w *World) pickpocketLocked(p *Player, sess *session.Session, npc *NPC, now time.Time) {
	say := func(format string, args ...any) {
		if sess != nil {
			w.sendColoredLocked(sess, fmt.Sprintf(format, args...), [3]byte{255, 255, 255})
		}
	}
	if !npc.pickedAt.IsZero() && now.Sub(npc.pickedAt) < pickAgain {
		say(pickEmpty, npc.Name)
		return
	}
	if pickIntn(100) >= pickChancePct(p, npc) {
		log.Printf("Pickpocket: %s is caught by %s", p.Name, npc.Name)
		say(pickCaught, npc.Name)
		w.updateAggro(npc, p.ServerID, 1)
		return
	}
	npc.pickedAt = now
	lvl := max(npc.Level, 1)
	n := lvl + pickIntn(lvl+1)
	p.Tunar += n
	log.Printf("Pickpocket: %s takes %d tunar from %s (%d now)", p.Name, n, npc.Name, p.Tunar)
	say(pickTook, n, npc.Name)
	if sess != nil {
		sendPurse(sess, p.Tunar)
	}
	if fn := w.onPurse; fn != nil {
		go fn(p.CharServerID, p.Tunar)
	}
}

// sendPurse is 0x0052: the purse, its tunar (zigzag), as a merchant's.
func sendPurse(sess *session.Session, tunar int) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodePlayerTunar)
	wr := buffer.NewWriter(msg.Span())
	wr.WriteUint16(uint16(protocol.OpcodePlayerTunar))
	wr.Write7BitEncodedInt64(int64(tunar))
	msg.SetSize(wr.Position())
	sess.Queue().AddWithSequence(msg)
}

// disguised reports a disguise on the player (the file's doc).
func (p *Player) disguised(now time.Time) bool {
	for _, e := range p.Effects {
		if e.Rules.Disguise && now.Before(e.Until) {
			return true
		}
	}
	return false
}

// npcMeleeRollFor is a creature's melee roll on a player: a creature's of
// its level less the player's DEF mods (the file's doc), a dummy's measured.
func npcMeleeRollFor(npc *NPC, p *Player) meleeRoll {
	if p.Mods.DefMod != 0 && npc.Dummy == NotDummy {
		return creatureRoll(max(npc.Level-p.Mods.DefMod, 1)).withMods(npc.creatureMods()) // its effects' STR and DEX (pet.go)
	}
	return npcMeleeRoll(npc)
}
