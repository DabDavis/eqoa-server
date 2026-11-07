package entity

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
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
// line's "xN", else three of a potion or of an item that stacks to three and
// one of anything else). Using one does what its row of table itemeffect says
// (database/itemeffect.go): a line in spelleffect's words, its first word
// whom it lands on.
//
//   - Drunk ("Self, ..."): a draught turns health into power (its health
//     taken at once, refused when that would kill), a speed potion is Spirit
//     of the Wolf and an invisibility potion invisibility, each on the effects
//     list by its own name and icon, a tonic a resist, a corrective or a
//     restorative takes harmful effects off ("Dispel Effect xN"), a returning
//     potion sends the drinker home, and a line that names a place sends it
//     there ("Self, Teleport to Surefall Glade", EQOAEmu's Surefall Potion,
//     2026-11-01): the spellgates row of that spell, else the first by its
//     spell whose place it is (gates.go), its drinker landed as a self
//     gate's caster; refused before anything is drunk when no row is that
//     place, or its world is one the drinker's game cannot load. A line
//     may cost an item and give one (handlers/potion.go: "Reagent X xN",
//     held to use it and taken with it, refused in red without; "Summon X",
//     given after it is used): Envar's gems (2026-11-01, EQOA-Data's
//     Envaric Ascension guide), each sending its user up one chamber of the
//     Prophet's tower and each needing the cracked gem the last one left
//     ("If you do not have that cracked version of the gem, you will not be
//     allowed to use the gem in the next chamber").
//   - Thrown ("Target, ..."): its hit and what lasts of it land on the
//     creature targeted, as a spell's would (landOnNPCLocked), within the
//     potion's throwing range, and on every creature within its splash
//     (itemeffect.area: the cocktails'); what its target alone takes beside
//     it is its row's target_line (a target-based area's first figure, the
//     workbook's HP1, the line its second, HP2: its legend's "AoE hits target
//     and also the radius from the target, including target a second time",
//     2026-11-01), so the target takes both and the rest of the splash the
//     line, what lasts of it on each once. A thrown potion is a ranged weapon
//     (F3 20261026-014336, jib: "equips in the range attack slot, like a bow,
//     and fires as a range auto attack"): wear 16, a bow's, attack type 10,
//     worn in the ranged slot by Use, and each throw of the ranged
//     auto-attack is one of them, gone from the stack
//     (thrownPotionHitLocked). Use throws one only when it is worn nowhere
//     (a GM's potion; handlers/potion.go usesPotion).
//
// A use may show a body (illusion.go; 2026-11-01, the user's "fix all these"
// of the potions that changed no body): its effect's name read as a spell's
// ("Illusion: Ogre", the Transformative Pill's) and its row's body (a CHAR.ESF
// CSprite's DictID as npcs.modelid holds it: the workbook's Alter Appearance
// sprite, the other way round). A drink shows its drinker in it as a spell's
// illusion does (a race's body of the drinker's own sex), a thrown potion the
// creature it hits, for what lasts of it (NPC.Body).
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
	// Body is the body it shows (its row's, SetBody), 0 none but its name's
	Body uint32
	// TargetShare is what a thrown one's target alone takes beside its line
	// (its row's target_line, SetTargetShare)
	TargetShare []SpellEffect
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
	pe.readBody()
	return pe
}

// SetBody gives the use its row's body (itemeffect.body: a CHAR.ESF CSprite's
// DictID, 0 none).
func (pe *PotionEffect) SetBody(body uint32) {
	pe.Body = body
	pe.readBody()
}

// SetTargetShare gives a thrown use what its target alone takes beside its
// line (itemeffect.target_line, in the line's words; "" none): its hit, read
// as the line is, before the line's own.
func (pe *PotionEffect) SetTargetShare(line string) {
	pe.TargetShare = nil
	if strings.TrimSpace(line) != "" {
		pe.TargetShare = ParseSpellText(line).Effects
	}
}

// readBody gives the use the body its effect's name says as a spell's does
// ("Illusion: Ogre"; a glamour's too) or its row gives (Body; with both the
// row's, a race's body still its bearer's sex's: illusion.go): an illusion on
// a drinker, as a self illusion spell's, or on the creature a thrown one
// hits, for what lasts of it (ten minutes when its line gives no time, as an
// illusion spell's).
func (pe *PotionEffect) readBody() {
	var named SpellDef
	named.readIllusion(pe.Name, "")
	form := ""
	if named.FormKind == FormSelf || named.FormKind == FormGroup {
		form = named.Form
	}
	if form == "" && pe.Body == 0 {
		return
	}
	d := &pe.Def
	if !d.hasIllusion() {
		d.Effects = append(d.Effects, SpellEffect{Kind: EffectIllusion})
	}
	d.Form, d.FormModel = form, pe.Body
	if !pe.Thrown() {
		d.FormKind = FormSelf // a thrown one keeps its target (castTargetsLocked)
	}
	if d.Duration <= 0 {
		d.Duration = illusionDuration
	}
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
	// a potion that takes its drinker to a named place: that gate's landing
	// (gates.go), or refused before anything is drunk
	var travel *GateTravel
	if place, ok := potionPlace(&pe.Def); ok {
		g, found := w.namedGateLocked(place)
		switch {
		case !found:
			return potionNoPlace, nil, nil
		case g.Kind == database.GateEvac:
			gate = true // an evacuation's place is its traveller's bind
		case !w.loadsLocked(sess, g.World):
			return fmt.Sprintf(gateCannotLoad, gatePlace(g)), nil, nil
		default:
			x, y, z := w.gateLandingLocked(g)
			travel = &GateTravel{Sess: sess, P: p, Spell: pe.Name,
				To: GateSpot{World: g.World, X: x, Y: y, Z: z, Place: gatePlace(g)}}
		}
	}
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
	// a potion drunk shows its drinker (the user, 2026-11-02: "potions should
	// still break it"), before its own effects land: an invisibility potion's
	// stays (invis.go)
	w.breakInvisLocked(p, "a potion")
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
	if travel != nil {
		// the tick hands it to the travel hook (runTravels), as a gate's
		w.gates.due = append(w.gates.due, *travel)
		log.Printf("Potion: %q sends %s to %s (%.1f, %.1f, %.1f) world %d", pe.ItemName, p.Name, travel.To.Place,
			travel.To.X, travel.To.Y, travel.To.Z, travel.To.World)
	}
	if gate {
		return "", &respawnDue{sess: sess, p: p}, nil
	}
	return "", nil, nil
}

// throwLocked lands a thrown potion on npc, its own share first (TargetShare,
// two hits as the workbook's two figures are), and its line on every creature
// that may be fought within its splash. Throwing is an attack: what a potion
// of the thrower's own ends on an attack ends, invisibility too.
func (w *World) throwLocked(sess *session.Session, p *Player, pe *PotionEffect, npc *NPC, now time.Time) {
	cs := &castState{spell: pe.castSpell(), caster: p.ServerID, targets: []EntityID{npc.ServerID}}
	target := cs
	if len(pe.TargetShare) > 0 {
		own := *cs
		own.spell.Def.Effects = append(append([]SpellEffect(nil), pe.TargetShare...), cs.spell.Def.Effects...)
		target = &own
	}
	w.landOnNPCLocked(target, p, sess, npc, now)
	if pe.Area > 0 {
		place := areaPlace{world: npc.World, caster: [3]float32{p.X, p.Y, p.Z}, centre: [3]float32{npc.X, npc.Y, npc.Z}, onOther: true}
		for _, id := range w.SpatialIndex.QueryRadius(npc.X, npc.Z, pe.Area) {
			o := w.NPCs[id]
			if o == nil || o == npc || o.Dead || o.World != npc.World || o.Owner != 0 || o.NPCType&0x80 != 0 || o.hiddenAt(now) {
				continue
			}
			if dist3(o.X, o.Y, o.Z, npc.X, npc.Y, npc.Z) > pe.Area || !w.reachesLocked(place, o.X, o.Y, o.Z) {
				continue
			}
			w.landOnNPCLocked(cs, p, sess, o, now)
		}
	}
	w.breakInvisLocked(p, "a throw")     // invis.go
	w.breakBoardWolfLocked(p, "a throw") // boardwolf.go
	w.ownHitLocked(p, "a throw")         // workbook.go: a run speed
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
// watery draught", "summons unstable potion x3" (after any "(sow)" is cut),
// and the game's own record's words as Allakhazam's archived pages give them,
// "Summon Item: foraged berries" (a ranger's Forage Berries, AK 209; jib's
// bug report of 2026-10-27, RNG-1).
var (
	reSummonItem = regexp.MustCompile(`^summ[ou]ns?\s+(?:item\s*:?\s*)?(?:an?\s+|the\s+)?(.+?)(?:\s+x\s*(\d+))?$`)
	reParens     = regexp.MustCompile(`\s*\([^)]*\)`)
	// a creature's summoning the pet words miss ("Summons a zombie to your
	// aid", "Summons a familar creature"): no item
	reSummonCreature = regexp.MustCompile(`\b(?:to your aid|to aid|creature|familiar|familar)\b`)
)

// summonItemsLocked hands a spell's summoned items to the hook, at its
// landing. It reports whether the spell's words summon any.
func (w *World) summonItemsLocked(cs *castState, caster *Player, sess *session.Session) bool {
	d := &cs.spell.Def
	if d.Summons() {
		return false // a pet (pet.go)
	}
	found := false
	// the workbook's items, by name (spelljson.go)
	for _, it := range d.Items {
		found = true
		log.Printf("Cast: %s's %q makes %q (x%d)", caster.Name, cs.spell.Name, it.Name, it.Count)
		if fn := w.potions.onSummon; fn != nil && sess != nil {
			go fn(sess, caster.CharServerID, it.Name, it.Count, cs.spell.Name)
		}
	}
	for _, it := range unreadSummons(d) {
		found = true
		log.Printf("Cast: %s's %q summons %q (x%d)", caster.Name, cs.spell.Name, it.Name, it.Count)
		if fn := w.potions.onSummon; fn != nil && sess != nil {
			go fn(sess, caster.CharServerID, it.Name, it.Count, cs.spell.Name)
		}
	}
	return found
}

// unreadSummons is each summoning of an item in a line's unread words, its
// count the words' xN (0: the item's own, SummonItemFunc's).
func unreadSummons(d *SpellDef) []SpellItem {
	var out []SpellItem
	for _, u := range d.Unread {
		m := reSummonItem.FindStringSubmatch(strings.TrimSpace(reParens.ReplaceAllString(u, "")))
		if m == nil || reSummonCreature.MatchString(u) {
			continue
		}
		count, _ := strconv.Atoi(m[2])
		out = append(out, SpellItem{Name: m[1], Count: count})
	}
	return out
}

// Summons is what using the potion gives its user after it is used, as a
// spell's summoning does (handlers/potion.go): the items its line summons
// ("Summon Cracked Noble's Gem x1": one of Envar's gems leaves its cracked
// version), none for a line that makes a pet.
func (pe *PotionEffect) Summons() []SpellItem {
	if pe.Def.Summons() {
		return nil
	}
	return append(append([]SpellItem(nil), pe.Def.Items...), unreadSummons(&pe.Def)...)
}

// Reagent is the item using the potion costs and how many, "" for none: its
// line's "Reagent X xN", held to use it and taken with it (handlers/
// potion.go: Envar's gems, each needing the cracked gem the last one left).
func (pe *PotionEffect) Reagent() (string, int) {
	if pe.Def.Reagent == "" || pe.Def.ReagentCount <= 0 {
		return "", 0
	}
	return pe.Def.Reagent, pe.Def.ReagentCount
}

// rePotionPlace is the place a drunk potion's line takes its drinker to
// ("Teleport to Surefall Glade"), in the words the line reader leaves unread
// (spelltext.go).
var rePotionPlace = regexp.MustCompile(`^teleports?\s+(?:you\s+)?to\s+(?:the\s+)?(.+?)\.?$`)

// potionNoPlace refuses a potion whose line names a place no gate is: ours,
// as gates.go's refusals are.
const potionNoPlace = "That potion has no destination here."

// potionPlace is the place a potion's line names, if it names one.
func potionPlace(d *SpellDef) (string, bool) {
	for _, u := range d.Unread {
		if m := rePotionPlace.FindStringSubmatch(strings.TrimSpace(u)); m != nil {
			return strings.TrimSpace(m[1]), true
		}
	}
	return "", false
}

// namedGateLocked is the gate a potion's place names: the spellgates row of
// that spell ("Teleport to Jaggedpine Circle"), else the first, by its
// spell, whose place it is ("Teleport to Surefall Glade"). Must be called
// with the lock held.
func (w *World) namedGateLocked(place string) (database.SpellGate, bool) {
	if g, ok := w.gates.rows[database.SpellGateKey(place)]; ok {
		return g, true
	}
	var keys []string
	for k, g := range w.gates.rows {
		if strings.EqualFold(strings.TrimSpace(g.Place), strings.TrimSpace(place)) {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return database.SpellGate{}, false
	}
	sort.Strings(keys)
	return w.gates.rows[keys[0]], true
}

// curePlayerLocked takes up to n harmful effects off a player (a corrective's
// or a restorative's), the oldest first.
func (w *World) curePlayerLocked(p *Player, n int) {
	for _, ae := range append([]*ActiveEffect(nil), p.Effects...) {
		if n <= 0 {
			return
		}
		if !ae.Rules.NoCure && (ae.HPTick < 0 || ae.Root || ae.Snare || ae.Mods.harmful()) {
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
		attacking := p.Attacking() || (p.Casting != nil && p.Casting.spell.Def.Harmful && now.Before(p.Casting.lands))
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
