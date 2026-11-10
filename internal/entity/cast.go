package entity

import (
	"fmt"
	"log"
	"math"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// cast.go — casting a spell, as Sandstorm answers it.
//
// The client's request is Send0032 (0x0032): the ability stamp, the bar slot,
// the target (handlers/spellbook.go resolves the slot's spell and its line).
// What Sandstorm does with one, recorded with eqoa harvest on 2026-10-21 (a
// druid of 23; claude-notes/casting-sandstorm-20261021.md):
//
//   - started, in one datagram: 0x00CD (the ability's display index, stamp 0,
//     a lockout of cast time + recast + 300 ms), 0x00B1 (caster, target, no
//     others, the cast time in ms, the spell's visual), and the stat channel
//     with the power cost already taken;
//   - landed 1.3 to 1.6 s after the cast time: a heal or a hit is a 0x00DB
//     (caster, the amount signed, target) — a heal only when the target was
//     short of its maximum, and for the whole amount — and an effect with a
//     duration goes on the effects channel (0x43) as the ability's front icon
//     and name, a recast one moved to the end;
//   - the lockout over: 0x00CD again, with 0;
//   - refused, one line of 0x0A7B in (255, 63, 63): "No target.", "Target is
//     friendly." (a harmful spell on a friend, the caster too), "Target is
//     not friendly." (a heal on a creature), "Out of range.", "Not enough
//     power.";
//   - a cast sent during another's cast time (and 0.4 s on): nothing at all;
//   - a cast sent into its own recovery, its cast time over: the red line
//     "Recovery time for that ability has not been met (N.N s)" and the
//     player's own 0x00B1 at itself, cast 0, the lockout cloud 0x9EC7854A,
//     at most one about every 1.25 s (recoveryAnswerLocked).
//
// What a spell does comes from its line (spelltext.go); a line this cannot
// read lands as nothing, the cast itself (power, lockout, animation) as on
// Sandstorm.

// castLand is how long after its cast time a spell lands: Sandstorm's land
// 1.2 to 1.7 s after, 1.4 typically (14 casts in the captures, 9 recorded).
const castLand = 1400 * time.Millisecond

// castBusy is how long past its cast time a cast keeps another from
// starting: Sandstorm dropped one 0.37 s after a 2 s cast and took one 0.55 s
// after.
const castBusy = 400 * time.Millisecond

// castLockoutExtra is what Sandstorm's lockout adds to the cast time and the
// recast: 300 ms on every one recorded (3300 for a 2 s cast of recast 1).
const castLockoutExtra = 300 * time.Millisecond

// effectTick is a "per tic" effect's tick: six seconds, so the lists' 24 s
// heals tick four times ("63 x4").
const effectTick = 6 * time.Second

// tickDuration is how long a per-tick effect lasts when its line gives no
// duration ("Group, Increase HP per tic by 76 ; Increase 10% based on CHA"):
// the lists' usual, Minor Healing's, Healing's and Light Healing's 24 s. A
// guess for the lines that leave it out.
const tickDuration = 24 * time.Second

// speedBuff is what a movement buff makes of the run speed: Sandstorm's 8.83
// became 11.48 under Spirit of the Wolf.
const speedBuff = 1.3

// RunSpeed is a player's run speed with nothing on it: what Sandstorm's dump
// and its 0x00F8 carry in every capture (0x410D5555, 8.833333), and CLIENT's
// own m_speed before a dump. 1.3 times it is 0x4137BBBB, Spirit of the Wolf's
// 0x00F8 there to the bit. It was 20 until 2026-10-23 (since the first
// import), so a PS2 here ran 2.3 times Sandstorm's speed; the Rust client
// kept its own 8.83 and took no speed from a server, which is why a speed
// buff did nothing in it. It takes them now.
var RunSpeed = math.Float32frombits(0x410D5555)

// The refusals' words, as Sandstorm's.
const (
	castNoTarget    = "No target."
	castFriendly    = "Target is friendly."
	castNotFriendly = "Target is not friendly."
	castOutOfRange  = "Out of range."
	castNoPower     = "Not enough power."
	// a pet's spell with no pet: CLIENT's own word for it (1021), which it
	// shows itself for a pet command with none
	castNoPet = "You do not have a pet."
)

var castRefusalRGB = [3]byte{255, 63, 63}

// A cast into its own recovery, as Sandstorm answered four spells spammed
// (lane-spellfx24, 2026-10-30: Rockskin, Minor Healing, Roar and Focus): its
// red line, the lockout left to a tenth ("(300.2 s)"), and the player's own
// CastingCombat at itself, cast 0, the lockout cloud (SPELLFX.CSF's effect
// 216: a puff of dust at the feet with a sound), sent as a cast is, so those
// around see it too (an alchemist spamming beside the recorder was seen so).
// A send in the cast time itself got nothing; after it, Sandstorm answered at
// most one about every 1.25 s (1.03-1.40 s apart, mostly 1.25, to sends 0.25 s
// apart) and dropped the rest: recoveryAnswerGap.
const (
	castRecoveryWords        = "Recovery time for that ability has not been met (%.1f s)"
	fxLockoutCloud    uint32 = 0x9EC7854A
	recoveryAnswerGap        = 1200 * time.Millisecond
)

// recoveryAnswerLocked answers a cast sent into its own recovery (left of it
// to go), unless one was answered less than recoveryAnswerGap ago.
func (w *World) recoveryAnswerLocked(sess *session.Session, p *Player, spell *CastSpell, left time.Duration, now time.Time) {
	if !p.recoveryAnswered.IsZero() && now.Sub(p.recoveryAnswered) < recoveryAnswerGap {
		log.Printf("Cast: %s casts %q, locked out for %v: not answered (%v after the last answer)", p.Name, spell.Name,
			left.Round(time.Millisecond), now.Sub(p.recoveryAnswered).Round(time.Millisecond))
		return
	}
	p.recoveryAnswered = now
	text := fmt.Sprintf(castRecoveryWords, left.Seconds())
	log.Printf("Cast: %s casts %q, locked out for %v: %q", p.Name, spell.Name, left.Round(time.Millisecond), text)
	w.sendColoredLocked(sess, text, castRefusalRGB)
	w.broadcastCastLocked(p, p.ServerID, 0, fxLockoutCloud)
}

// The visuals Sandstorm's recorded casts named, as the stand-ins for spells
// with none recorded: a heal's (Minor Healing's and Healing's), a strike's
// (Fiery Burst's) and a buff's (Rockskin's).
const (
	fxHeal   uint32 = 0xC2B700FA
	fxDamage uint32 = 0xE78024D9
	fxBuff   uint32 = 0x1DAEB5B6
)

// CastSpell is a spell as the caster has it: its ability record's fields,
// its line read, its visual.
type CastSpell struct {
	SpellID      int
	DisplayIndex int    // its place in the book: 0x00CD names it
	Level        int    // the ability's level (abilitylvl): a summoned pet's (pet.go)
	Scope        int    // VIAbility's: 0 self, 1 target, 2 group, 3 pet, 4 corpse
	Name         string // the effect's name on channel 0x43
	Icon         uint32 // the ability's front icon: the effect's icon
	CastTime     time.Duration
	Recast       time.Duration
	PowerCost    int
	Range        float32
	Def          SpellDef
	FX           uint32    // 0x00B1's visual; 0 for the stand-in of its kind
	Area         SpellArea // what it lands on beside its target (area.go)
	// HandsBack is the handlers' switch of hands for the cast having taken
	// up a ranged weapon in place of the hands it took off
	// (handlers/handswitch.go): a self cast's landing gives them back, the
	// bow borrowed for it (handsBackLocked; a cast at a creature gives back
	// a switch's hands by its reach, marked or not)
	HandsBack bool
	// Travel is where an item's cast sends its caster as it lands (the Rock
	// of Freeport, handlers/rockfreeport.go), nil for a spell: the caster
	// alone, handed to the travel hook as a gate's traveller is (gates.go),
	// and nothing else lands. It is no ability of the book, so it has no
	// lockout and no 0x00CD; while one is on its way, begun and not landed,
	// another is let go unanswered (StartCast).
	Travel *GateSpot
	// Familiar is the familiar an item's cast calls as it lands (familiar.go),
	// nil for a spell: its caster's, and nothing else lands; no ability of the
	// book, as Travel's
	Familiar *FamiliarDef
	// resist is the resist that cuts a templated NPC's spell's hit on a
	// player, by its element (classtemplate.go); a player's spell has none
	resist Stat
	// unresistable is a templated NPC's spell no resist cuts: its line says so
	// (the Earth Primeval's "huge unresistable nuke")
	unresistable bool
	// duel is set when a duel aimed the cast at its caster's opponent or the
	// opponent's pet (duelSpellTargetLocked): its landing is the duel's
	// alone (duelLandLocked)
	duel bool
	// figures is whose its figures are to a client that colours its numbers
	// (numbertags.go): its caster's spell, or a proc's, a pet's, a gift's
	figures castFigures
}

// SpellOf is an ability record as the World casts it: handlers' castSpell
// for a player's book (displayIndex its place there), a templated NPC's
// (classtemplate.go) with none. The record gives cast time and recast in
// seconds (Sandstorm's: Minor Healing's 2 and 1 are a 3300 ms lockout), the
// front icon the effects channel shows, the range as writeAbilityRecord
// sends it; its line is read with its name, which says what an illusion is
// (illusion.go) and gives a line where the database has none, and its area
// as the line reads (area.go: a spellarea row goes over it, With).
func SpellOf(sp *database.SpellPatternRecord, displayIndex int, line string, fx uint32) CastSpell {
	return SpellOfRow(sp, displayIndex, line, "", fx)
}

// SpellOfRow is SpellOf with the spell's workbook effects (spelleffect.effects,
// JSON; "" for none), which when there are any are what it does
// (SpellDefFrom, spelljson.go).
func SpellOfRow(sp *database.SpellPatternRecord, displayIndex int, line, effects string, fx uint32) CastSpell {
	def := SpellDefFrom(sp.SpellName, line, sp.SpellDesc, effects)
	return CastSpell{
		SpellID:      sp.SpellID,
		DisplayIndex: displayIndex,
		Level:        sp.AbilityLvl,
		Scope:        sp.Scope,
		Name:         sp.SpellName,
		Icon:         uint32(sp.Icon),
		CastTime:     time.Duration(sp.CastTime) * time.Second,
		Recast:       time.Duration(sp.Recast) * time.Second,
		PowerCost:    sp.Power,
		Range:        math.Float32frombits(sp.RangeBits()),
		Def:          def,
		FX:           fx,
		Area:         ReadSpellArea(sp.SpellName, line, def),
	}
}

// visual is the spell's 0x00B1 visual.
func (s *CastSpell) visual() uint32 {
	if s.FX != 0 {
		return s.FX
	}
	fx, _ := StandInVisual(s.Def)
	return fx
}

// StandInVisual is the visual a spell with none of its own plays, by what it
// does, and its kind: a heal's ("heal"), a harmful spell's ("damage"),
// anything else a buff's ("buff").
func StandInVisual(d SpellDef) (uint32, string) {
	for _, e := range d.Effects {
		if (e.Kind == EffectHP || e.Kind == EffectHPTick) && e.Amount > 0 && !e.Self {
			return fxHeal, "heal"
		}
	}
	if d.Harmful {
		return fxDamage, "damage"
	}
	return fxBuff, "buff"
}

// StandInVisuals are the stand-ins by kind.
func StandInVisuals() map[string]uint32 {
	return map[string]uint32{"heal": fxHeal, "damage": fxDamage, "buff": fxBuff}
}

// castState is one cast, from its start to its landing.
type castState struct {
	spell    CastSpell
	caster   EntityID
	targets  []EntityID
	others   []EntityID // an area spell's others (area.go)
	ends     time.Time  // the cast time over: the caster may cast again
	lands    time.Time
	castEnds time.Time // the cast time itself over (ends less castBusy)
}

// castLockout is one ability's lockout: 0x00CD again when it is over.
type castLockout struct {
	spellID      int
	displayIndex int
	until        time.Time
}

// ActiveEffect is a spell's effect on a player or creature, from its landing
// for its duration.
type ActiveEffect struct {
	SpellID   int
	Name      string
	Icon      uint32
	Caster    EntityID
	Until     time.Time
	NextTick  time.Time
	HPTick    int // each tick, signed
	PowerTick int
	// TickDrain is whom a damage tick's take goes to (a drain over time:
	// Infernal Bond's "Decrease HP per tic by 240, give to self"), DrainNone
	// for nobody; TickRange the spell's range, for a group's share.
	TickDrain DrainTo
	TickRange float32
	// ticking is set while the effect's own tick lands: its hit is no
	// contact for itself (contactNPCLocked), as Caltrops' tick ended its own
	// snare at its landing
	ticking bool
	Mods    StatMods
	Root    bool
	Snare   bool
	Speed   bool
	Invis   bool
	// Proc is a proc's damage (proc.go): what the bearer's melee hits may do
	// more, 0 for none; ProcMax the top of its range.
	Proc, ProcMax int
	// ProcScale and ProcScalePct: a proc whose scale is its bearer's
	// (SpellEffect.ScaleBearer) adds that share of the bearer's stat at each
	// proc; ProcCap is the most a proc does, 0 for no cap.
	ProcScale    Stat
	ProcScalePct int
	ProcCap      int
	// ProcUndead: the proc fires on undead creatures alone (Smite Undead's)
	ProcUndead bool
	// Form is the body an illusion shows its bearer in (illusion.go), 0 for
	// none; FormSize its scale, 0 for the bearer's own.
	Form     uint32
	FormSize float32
	// Ticks is how many ticks a heal over time on a player has left
	// (hot.go); 0 for an effect that ticks until it ends (a creature's, a
	// pet's, one kept from before ticks were counted).
	Ticks int
	// Shield is a damage shield or a defensive proc (dshield.go), nil for
	// none
	Shield *Shield
	// PowerCap is the most the bearer's maximum power is while it lasts,
	// when PowerCapped (powercap.go: the ranger's Eye line, "Set Maximum
	// Power to 0")
	PowerCap    int
	PowerCapped bool
	// Food is a meal's or a drink's rule (consumable.go), nil for every
	// other effect and for one with no foodeffect row
	Food *FoodRule
	// duelFoe is the duel opponent that put a harmful effect on a player
	// (duelfight.go), 0 for every other effect: it alone holds and slows a
	// player (SpeedNow), comes off at the duel's end and is never kept over
	// a logout (buffsave.go)
	duelFoe EntityID
	// npcFoe is the creature whose damage over time it is on a player (a
	// template's debuff whose spell has a damage tick: classtemplate.go), 0 for
	// every other effect: each tick hurts as the creature's spell does
	// (npcTickLocked), at its element's resist (npcResist); never kept over a
	// logout (buffsave.go)
	npcFoe    EntityID
	npcResist Stat
	// pet is the pet whose damage over time it is on a creature (a pet
	// template's AbilityDoT, pettemplate.go; Caster its owner), 0 for every
	// other effect: each tick is one of the pet's hits (petTickLocked)
	pet EntityID
	// npcCaster is the creature whose damage over time it is on the creature
	// it fights (a template's AbilityDoT, creaturedot.go), 0 for every other
	// effect: each tick lands as that fight's hit (npcDoTTickLocked)
	npcCaster EntityID
	// merc is the merc whose damage over time or debuff it is on a creature
	// (merccast.go; Caster the merc too), 0 for every other effect: each tick
	// is one of the merc's hits (mercTickLocked)
	merc EntityID
	// Line and Rank are a beneficial spell's line and its level on a player
	// (buffline.go): only the best of a line counts; "" for none
	Line string
	Rank int
	// Rules are what the workbook's effects say beside their numbers
	// (workbook.go): kept over a logout
	Rules EffectRules
	// parts are what each of its lasting parts puts on, when a hit, its
	// bearer's cast or its own action may end some and not the others
	// (parts.go): kept over a logout; nil for one that ends whole
	parts []*effectPart
	// ProcSpells are its procs that cast more than a hit (workbookmore.go)
	ProcSpells []ProcSpell
	// figures is whose its ticks' figures are (numbertags.go): its cast's
	figures castFigures
}

// StatMods is what effects add to a player's stats (kept over a logout as
// JSON, the stats it changes only).
type StatMods struct {
	Str      int `json:",omitempty"`
	Sta      int `json:",omitempty"`
	Agi      int `json:",omitempty"`
	Dex      int `json:",omitempty"`
	Wis      int `json:",omitempty"`
	Int      int `json:",omitempty"`
	Cha      int `json:",omitempty"`
	AC       int `json:",omitempty"`
	MaxHP    int `json:",omitempty"`
	MaxPower int `json:",omitempty"`
	FR       int `json:",omitempty"`
	CR       int `json:",omitempty"`
	PR       int `json:",omitempty"`
	DR       int `json:",omitempty"`
	LR       int `json:",omitempty"`
	AR       int `json:",omitempty"`
	// HoT and PoT are added to the six-second regen (regen.go): food and
	// drink's (consumable.go)
	HoT int `json:",omitempty"`
	PoT int `json:",omitempty"`
	// Melee is added to every melee hit the bearer lands (offence.go's
	// playerMeleeHit): Frenzy's damage per hit (vampire.go)
	Melee int `json:",omitempty"`
	// the workbook's melee resists (a creature's debuff of each: negative),
	// hit point factor and off and def mods (workbookmore.go)
	Pierce   int `json:",omitempty"`
	Blunt    int `json:",omitempty"`
	Slash    int `json:",omitempty"`
	HPFactor int `json:",omitempty"`
	OffMod   int `json:",omitempty"`
	DefMod   int `json:",omitempty"`
}

func (m *StatMods) add(st Stat, n int) {
	switch st {
	case StatStr:
		m.Str += n
	case StatSta:
		m.Sta += n
	case StatAgi:
		m.Agi += n
	case StatDex:
		m.Dex += n
	case StatWis:
		m.Wis += n
	case StatInt:
		m.Int += n
	case StatCha:
		m.Cha += n
	case StatAC:
		m.AC += n
	case StatMaxHP:
		m.MaxHP += n
	case StatMaxPower:
		m.MaxPower += n
	case StatFR:
		m.FR += n
	case StatCR:
		m.CR += n
	case StatPR:
		m.PR += n
	case StatDR:
		m.DR += n
	case StatLR:
		m.LR += n
	case StatAR:
		m.AR += n
	case StatMelee:
		m.Melee += n
	case StatHoT:
		m.HoT += n
	case StatPoT:
		m.PoT += n
	case StatPierce, StatRanged: // the workbook's "ranged" was pierce (its known issues)
		m.Pierce += n
	case StatBlunt:
		m.Blunt += n
	case StatSlash:
		m.Slash += n
	case StatHPFactor:
		m.HPFactor += n
	case StatOffMod:
		m.OffMod += n
	case StatDefMod:
		m.DefMod += n
	}
}

// get is m's change of st (StatRanged is Pierce's, as add has it).
func (m StatMods) get(st Stat) int {
	switch st {
	case StatStr:
		return m.Str
	case StatSta:
		return m.Sta
	case StatAgi:
		return m.Agi
	case StatDex:
		return m.Dex
	case StatWis:
		return m.Wis
	case StatInt:
		return m.Int
	case StatCha:
		return m.Cha
	case StatAC:
		return m.AC
	case StatMaxHP:
		return m.MaxHP
	case StatMaxPower:
		return m.MaxPower
	case StatFR:
		return m.FR
	case StatCR:
		return m.CR
	case StatPR:
		return m.PR
	case StatDR:
		return m.DR
	case StatLR:
		return m.LR
	case StatAR:
		return m.AR
	case StatMelee:
		return m.Melee
	case StatHoT:
		return m.HoT
	case StatPoT:
		return m.PoT
	case StatPierce, StatRanged:
		return m.Pierce
	case StatBlunt:
		return m.Blunt
	case StatSlash:
		return m.Slash
	case StatHPFactor:
		return m.HPFactor
	case StatOffMod:
		return m.OffMod
	case StatDefMod:
		return m.DefMod
	}
	return 0
}

// take takes o's changes back off m (an effect's parts that end apart:
// parts.go's takeOff).
func (m *StatMods) take(o StatMods) {
	m.Str -= o.Str
	m.Sta -= o.Sta
	m.Agi -= o.Agi
	m.Dex -= o.Dex
	m.Wis -= o.Wis
	m.Int -= o.Int
	m.Cha -= o.Cha
	m.AC -= o.AC
	m.MaxHP -= o.MaxHP
	m.MaxPower -= o.MaxPower
	m.FR -= o.FR
	m.CR -= o.CR
	m.PR -= o.PR
	m.DR -= o.DR
	m.LR -= o.LR
	m.AR -= o.AR
	m.HoT -= o.HoT
	m.PoT -= o.PoT
	m.Melee -= o.Melee
	m.Pierce -= o.Pierce
	m.Blunt -= o.Blunt
	m.Slash -= o.Slash
	m.HPFactor -= o.HPFactor
	m.OffMod -= o.OffMod
	m.DefMod -= o.DefMod
}

func (m *StatMods) sum(o StatMods) {
	m.Str += o.Str
	m.Sta += o.Sta
	m.Agi += o.Agi
	m.Dex += o.Dex
	m.Wis += o.Wis
	m.Int += o.Int
	m.Cha += o.Cha
	m.AC += o.AC
	m.MaxHP += o.MaxHP
	m.MaxPower += o.MaxPower
	m.FR += o.FR
	m.CR += o.CR
	m.PR += o.PR
	m.DR += o.DR
	m.LR += o.LR
	m.AR += o.AR
	m.HoT += o.HoT
	m.PoT += o.PoT
	m.Melee += o.Melee
	m.Pierce += o.Pierce
	m.Blunt += o.Blunt
	m.Slash += o.Slash
	m.HPFactor += o.HPFactor
	m.OffMod += o.OffMod
	m.DefMod += o.DefMod
}

// StatNow is a stat as the client sees it: the player's own, plus effects,
// STR to CHA within its cap, the level's and what its masteries raise it by
// (mastery.go).
func (p *Player) StatNow(st Stat) int {
	m := &p.Mods
	switch st {
	case StatStr:
		return p.capStat(StatStr, p.Str+m.Str)
	case StatSta:
		return p.capStat(StatSta, p.Sta+m.Sta)
	case StatAgi:
		if agi, _, ok := p.tankProfile(); ok { // a GM's #tankprofile (gmchannel.go)
			return agi
		}
		return p.capStat(StatAgi, p.Agi+m.Agi)
	case StatDex:
		return p.capStat(StatDex, p.Dex+m.Dex)
	case StatWis:
		return p.capStat(StatWis, p.Wis+m.Wis)
	case StatInt:
		return p.capStat(StatInt, p.Int+m.Int)
	case StatCha:
		return p.capStat(StatCha, p.Cha+m.Cha)
	case StatAC:
		if _, ac, ok := p.tankProfile(); ok { // a GM's #tankprofile (gmchannel.go)
			return ac
		}
		return p.AC + m.AC
	case StatMaxHP:
		return p.MaxHPNow()
	case StatMaxPower:
		return p.MaxPowerNow()
	case StatFR:
		return p.FireR + m.FR
	case StatCR:
		return p.ColdR + m.CR
	case StatPR:
		return p.PoisonR + m.PR
	case StatDR:
		return p.DiseaseR + m.DR
	case StatLR:
		return p.LightningR + m.LR
	case StatAR:
		return p.ArcaneR + m.AR
	}
	return 0
}

// MaxHPNow is the maximum hit points with effects: what they add, and what
// their stamina adds through the class's formula.
func (p *Player) MaxHPNow() int {
	n := p.MaxHP + p.Mods.MaxHP + p.Mods.HPFactor*p.Level // a hit point factor's (workbookmore.go)
	if sta := p.StatNow(StatSta); sta != p.Sta {
		n += p.maxHPFor(sta) - p.maxHPFor(p.Sta)
	}
	return max(n, 1)
}

// MaxPowerNow is the maximum power with effects, as MaxHPNow, and at most
// the lowest ceiling an effect puts on it (powercap.go).
func (p *Player) MaxPowerNow() int {
	n := p.MaxPower + p.Mods.MaxPower
	m := p.Mods
	if m.Str|m.Sta|m.Agi|m.Dex|m.Wis|m.Int|m.Cha != 0 {
		n += CalculateMaxPower(p.Level, p.Class, p.StatNow(StatStr), p.StatNow(StatSta), p.StatNow(StatAgi), p.StatNow(StatDex),
			p.StatNow(StatWis), p.StatNow(StatInt), p.StatNow(StatCha)) -
			CalculateMaxPower(p.Level, p.Class, p.Str, p.Sta, p.Agi, p.Dex, p.Wis, p.Int, p.Cha)
	}
	if top, ok := p.powerCap(); ok {
		n = min(n, top)
	}
	return max(n, 0)
}

// Invisible is whether an effect hides the player (entity state byte 154,
// which Sandstorm's Camouflage set on the caster's own channel).
func (p *Player) Invisible() bool {
	if p.GMHidden {
		return true
	}
	for _, e := range p.Effects {
		if e.Invis {
			return true
		}
	}
	return false
}

// The movement rate, as the players' own formula thread gives it
// (eqoa.forumotion.com t279 "Calculations", 2010): "Normal running = movement
// rate of 20, Spirit of the Wolf = movement rate of 100 ... various movement
// cms will increase the basic movement rate of 20, but only SoW gives 100.
// 100 is the maximum the move-rate can be." It names no speed, so 20 is the
// run speed (Sandstorm's 8.833333) and 100 Sandstorm's measured Spirit of the
// Wolf, speedBuff times it, linear between (the user's choice, 2026-10-25;
// read literally, 100 would run five times as fast).
const (
	moveRateBase = 20
	moveRateMax  = 100
)

// MoveRate is the player's movement rate now: 20 and what movement masteries
// add (MoveRateBonus: the masteries bought, cmbuy.go), 100 under a movement buff,
// never more than 100, so speed buffs and masteries never stack past it.
func (p *Player) MoveRate() int {
	r := moveRateBase + p.MoveRateBonus
	for _, e := range p.Effects {
		if e.Speed {
			r = moveRateMax
		}
	}
	return min(r, moveRateMax)
}

// SpeedNow is the run speed with effects (0x00F8): the movement rate as a
// speed, 20 the run speed and 100 Spirit of the Wolf's to the bit.
func (p *Player) SpeedNow() float32 {
	if p.Frozen {
		return 0 // a GM's freeze (gmchannel.go)
	}
	slow := float32(1)
	for _, e := range p.Effects {
		// a hold: a duel opponent's (duelfight.go), or one of the player's
		// own or a friend's cast puts on it (the druid's Spirit Form, the
		// paladin's barriers: the user, 2026-11-01, "yes hold the tree form");
		// no other harm lands on a player (classtemplate.go puts nothing
		// lasting on one). Told as a run speed of 0 (0x00F8), as a duel's
		// hold is, and the run speed again at its end
		if e.Root {
			return 0
		}
		if e.duelFoe != 0 && e.Snare { // a duel opponent's slow
			slow = duelSnare
		}
	}
	slow *= p.slowFactor() // a buff's own slow (workbook.go)
	switch r := p.MoveRate(); {
	case r <= moveRateBase:
		return p.Speed * slow
	case r >= moveRateMax:
		return p.Speed * speedBuff * slow
	default:
		f := 1 + (speedBuff-1)*float64(r-moveRateBase)/float64(moveRateMax-moveRateBase)
		return p.Speed * float32(f) * slow
	}
}

// GateFunc is the hook that sends a player home (Return Home's landing):
// handlers' teleport to where a login starts.
type GateFunc func(sess *session.Session, p *Player)

// OnGate sets the gate hook.
func (w *World) OnGate(fn GateFunc) {
	w.mu.Lock()
	w.onGate = fn
	w.mu.Unlock()
}

// StartCast is the server's half of a 0x0032: the cast checked, then begun
// as Sandstorm begins one, or refused with its line, or let go unanswered
// (another cast on, the ability locked out). It reports whether it began.
func (w *World) StartCast(sess *session.Session, spell CastSpell, targetObjectID uint32, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || p.IsDead() {
		return false
	}
	switch wait, until := p.castWaitFor(&spell, now); wait {
	case castDuring:
		log.Printf("Cast: %s casts %q during %q: not answered", p.Name, spell.Name, p.Casting.spell.Name)
		return false
	case castLocked:
		w.recoveryAnswerLocked(sess, p, &spell, until.Sub(now), now)
		return false
	case castCooling:
		log.Printf("Cast: %s uses %q %v before the arts' cooldown ends: not answered", p.Name, spell.Name, p.artReady.Sub(now).Round(time.Millisecond))
		return false
	}
	if spell.Travel != nil && w.travelOnItsWayLocked(p) {
		log.Printf("Cast: %s uses %q while a travel of its own is on its way: not answered", p.Name, spell.Name)
		return false
	}
	targets, refusal, fizzle := w.castRefusalLocked(sess, p, &spell, targetObjectID, now, true)
	if refusal != "" {
		log.Printf("Cast: %s casts %q on %#x: %s", p.Name, spell.Name, targetObjectID, refusal)
		if fizzle {
			target := p.ServerID
			if len(targets) > 0 {
				target = targets[0]
			}
			w.broadcastCastLocked(p, target, charmFizzle, spell.visual())
		}
		w.sendColoredLocked(sess, refusal, castRefusalRGB)
		return false
	}

	p.Power -= spell.PowerCost
	p.StatsDirty = true
	if isArt(&spell) {
		p.artReady = now.Add(w.balance.ArtCooldown(p.Level))
	}
	w.pvpCastBegunLocked(p, &spell, targets, now) // pvp.go: an attack given
	w.breakInvisLocked(p, "a spell cast")         // invis.go
	if spell.Def.Harmful {
		w.breakBoardWolfLocked(p, "a harmful spell") // boardwolf.go
	}
	w.dispelOnCastLocked(p, spell.Def.Harmful) // workbook.go: what ends at a cast (a run speed only at a harmful one)
	lockout := spell.CastTime + spell.Recast + castLockoutExtra
	if spell.Travel == nil && spell.Familiar == nil { // an item's travel or familiar is no ability: nothing to lock out
		p.Lockouts = append(p.Lockouts, castLockout{spellID: spell.SpellID, displayIndex: spell.DisplayIndex, until: now.Add(lockout)})
	}
	cs := &castState{spell: spell, caster: p.ServerID, targets: targets, ends: now.Add(spell.CastTime + castBusy), lands: now.Add(spell.CastTime + castLand),
		castEnds: now.Add(spell.CastTime)}
	w.areaStartLocked(p, cs, now) // area.go: an area spell's others
	p.Casting = cs
	w.endFoodOnCastLocked(p) // consumable.go: a meal or drink a cast ends
	w.casts = append(w.casts, cs)

	if spell.Travel == nil && spell.Familiar == nil {
		sendCooldown(sess, spell.DisplayIndex, lockout)
	}
	target := p.ServerID
	if len(targets) > 0 {
		target = targets[0]
	}
	w.broadcastCastLocked(p, target, spell.CastTime, spell.visual(), cs.others...)
	log.Printf("Cast: %s begins %q (spell %d) on %#x: %v cast, %d power (%d left), lands in %v",
		p.Name, spell.Name, spell.SpellID, targetObjectID, spell.CastTime, spell.PowerCost, p.Power, spell.CastTime+castLand)
	return true
}

// TravelBusy reports whether a travel cast (CastSpell.Travel) the session's
// player began now would be let go before its checks: dead or gone, within
// another cast's time, or a travel of its own on its way. Its handler asks
// first, so a quick slot pressed over and over looks for no place
// (handlers/rockfreeport.go: Dabrish is found by a walk over every NPC).
func (w *World) TravelBusy(sess *session.Session, now time.Time) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || p.IsDead() {
		return true
	}
	if p.Casting != nil && now.Before(p.Casting.ends) {
		return true
	}
	return w.travelOnItsWayLocked(p)
}

// travelOnItsWayLocked reports a travel cast of the player's (CastSpell.Travel)
// begun and not yet landed. Must be called with the lock held.
func (w *World) travelOnItsWayLocked(p *Player) bool {
	for _, cs := range w.casts {
		if cs.caster == p.ServerID && cs.spell.Travel != nil {
			return true
		}
	}
	return false
}

// castWait is what StartCast does with a cast before its checks.
type castWait int

const (
	castGoes    castWait = iota // on to its checks (castRefusalLocked)
	castDuring                  // within another cast: not answered
	castLocked                  // the ability locked out: its recovery's answer
	castCooling                 // an art within the arts' cooldown: not answered
)

// castWaitFor is what StartCast does with the player's cast before its
// checks, and when the ability's lockout ends for one locked out.
func (p *Player) castWaitFor(spell *CastSpell, now time.Time) (castWait, time.Time) {
	if p.Casting != nil && now.Before(p.Casting.ends) {
		// the same spell past its cast time is its recovery's to answer
		// (below); within the cast time, or another spell in the 0.4 s
		// after it: nothing, as on Sandstorm
		if p.Casting.spell.SpellID != spell.SpellID || now.Before(p.Casting.castEnds) {
			return castDuring, time.Time{}
		}
	}
	for _, l := range p.Lockouts {
		if l.spellID == spell.SpellID && now.Before(l.until) {
			return castLocked, l.until
		}
	}
	if isArt(spell) && now.Before(p.artReady) { // the arts' global cooldown (balance.go)
		return castCooling, time.Time{}
	}
	return castGoes, time.Time{}
}

// castRefusalLocked is a cast's checks after its waits, in StartCast's
// order: whom it lands on, or the line refusing it and whether it fizzles.
// With hands false its required equipment is left out (CastWantsHands).
// Must be called with the lock held.
func (w *World) castRefusalLocked(sess *session.Session, p *Player, spell *CastSpell, targetObjectID uint32, now time.Time,
	hands bool) (targets []EntityID, refusal string, fizzle bool) {
	// a charm while a pet is out is refused before anything is looked at,
	// whatever is targeted (charm.go)
	refusal = w.charmPetRefusalLocked(p, spell)
	if refusal == "" {
		targets, refusal = w.castTargetsLocked(sess, p, spell, targetObjectID)
	}
	if refusal == "" && spell.Def.Harmful {
		refusal = w.waterRefusalLocked(p, targets) // water.go
	}
	if refusal == "" && spell.Def.undeadOnly() {
		refusal, fizzle = w.undeadStartLocked(targets) // undead.go
	}
	if refusal == "" && spell.Def.TargetKind != "" {
		refusal, fizzle = w.targetKindStartLocked(&spell.Def, targets) // workbookmore.go
	}
	if refusal == "" && hands && spell.Def.ReqEquip != "" {
		refusal = equipRefusal(p, spell.Def.ReqEquip) // workbookmore.go
	}
	if refusal == "" && spell.Def.Backstab {
		refusal = w.backstabRefusalLocked(p, targets) // backstab.go
	}
	if refusal == "" && spell.Def.Charms() {
		refusal = w.charmStartLocked(spell, targets) // charm.go
	}
	if refusal == "" && spell.Def.Resurrects() > 0 {
		refusal, fizzle = w.rezStartLocked(p, spell, targets, now) // resurrect.go
	}
	if refusal == "" {
		refusal = w.gateStartLocked(sess, spell) // gates.go
	}
	if refusal == "" && p.Power < spell.PowerCost {
		refusal = castNoPower
	}
	return targets, refusal, fizzle
}

// CastWantsHands reports a cast its required equipment alone would refuse:
// the spell names weapons the player's hands do not hold (the workbook's
// required equipment, workbookmore.go), and nothing else (its target, its
// range, power, the ability's lockout, the arts' cooldown, another cast on)
// would refuse it or let it go. The handlers then put what it wants in the
// hands from the pack (handlers/handswitch.go) before StartCast; any other
// cast is StartCast's to answer as it always was, the hands left alone.
func (w *World) CastWantsHands(sess *session.Session, spell CastSpell, targetObjectID uint32, now time.Time) bool {
	if spell.Def.ReqEquip == "" {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || p.IsDead() || equipFits(p, spell.Def.ReqEquip) {
		return false
	}
	if wait, _ := p.castWaitFor(&spell, now); wait != castGoes {
		return false
	}
	_, refusal, _ := w.castRefusalLocked(sess, p, &spell, targetObjectID, now, false)
	return refusal == ""
}

// HandsBackFunc is the hook that puts back the hands a switch took off for
// a cast (handlers' HandsBack, handswitch.go). It runs with the World's lock
// let go, after the tick that landed the cast (runHandsBack).
type HandsBackFunc func(sess *session.Session, p *Player)

// OnHandsBack sets the hands-back hook.
func (w *World) OnHandsBack(fn HandsBackFunc) {
	w.mu.Lock()
	w.onHandsBack = fn
	w.mu.Unlock()
}

// handsBackLocked is a cast's hands at its landing. Any cast whose required
// equipment is a ranged weapon (wantsRanged: an arrow art) hands its caster
// to the hands-back hook (runHandsBack), which gives back the hands a switch
// took off to take up the bow it holds (handlers/handswitch.go: whenever that
// was, a pull at range's arrow art too), when the caster stands within its
// melee reach of the cast's target: CLIENT's TargetInRange for bare hands or
// a melee weapon (meleeReach, a creature's allowance for its size: npcReach,
// reach.go). So a ranger's arrow art used in melee leaves it on its blades
// and one shot from further keeps the bow, its auto-attack shooting on (asked
// whether the server should "switch back to the blades by itself right after
// an arrow art used in melee range", of a ranger left swinging its bow until
// a blade art, the user, 2026-11-01: "lets fix all these"; a pull at range
// included, the coordinator's ruling of the same day). A self cast's target
// is its caster: the bow its own switch took up (CastSpell.HandsBack) is
// borrowed for it and goes back, and with a switch's bow already in hand its
// reach is the one to the creature it fights (none, the bow kept), so a buff
// at range keeps a shooting ranger's bow. Never while another cast is on
// (begun after this one's busy time: Casting), which has its own hands. Must
// be called with the lock held.
func (w *World) handsBackLocked(cs *castState, caster *Player) {
	if !wantsRanged(cs.spell.Def.ReqEquip) || caster.Casting != nil || caster.IsDead() || len(cs.targets) == 0 {
		return
	}
	target := cs.targets[0]
	if target == caster.ServerID {
		if cs.spell.HandsBack {
			w.queueHandsBackLocked(caster) // the bow its own switch took up: borrowed
			return
		}
		target = caster.AttackTarget // the creature it fights, if any
	}
	var x, y, z float32
	reach := float32(meleeReach)
	switch {
	case target == 0:
		return // no fight: the bow kept
	case w.Players[target] != nil:
		x, y, z = w.Players[target].GetPosition()
	case w.NPCs[target] != nil:
		npc := w.NPCs[target]
		x, y, z = npc.GetPosition()
		reach = npcReach(npc)
	default:
		return // gone: no reach to stand in
	}
	if outOfRange(caster, x, y, z, reach) {
		if cs.spell.HandsBack { // its own switch's bow (said once a switch, not at every shot)
			log.Printf("Hands: %s's %q landed %.1f from its target (melee reach %.1f): the ranged weapon kept", caster.Name, cs.spell.Name,
				dist3(caster.X, caster.Y, caster.Z, x, y, z), reach)
		}
		return
	}
	w.queueHandsBackLocked(caster)
}

// queueHandsBackLocked queues the caster for the hands-back hook
// (runHandsBack). Must be called with the lock held.
func (w *World) queueHandsBackLocked(caster *Player) {
	if sess := w.findSessionByEntityIDLocked(caster.ServerID); sess != nil {
		w.handsBack = append(w.handsBack, respawnDue{sess: sess, p: caster})
	}
}

// runHandsBack hands each player whose hands go back (handsBackLocked) to
// the hands-back hook, one that began a cast or died since left as it is.
// Must be called WITHOUT the lock.
func (w *World) runHandsBack() {
	w.mu.Lock()
	due, fn := w.handsBack, w.onHandsBack
	w.handsBack = nil
	back := due[:0]
	for _, d := range due {
		if d.p.Casting == nil && !d.p.IsDead() {
			back = append(back, d)
		}
	}
	w.mu.Unlock()
	if fn == nil {
		return
	}
	for _, d := range back {
		fn(d.sess, d.p)
	}
}

// castTargetsLocked is whom a cast will land on, or the line refusing it.
// The ability's scope decides, as CLIENT words it (VITargetWord): a self
// spell lands on the caster whatever is targeted (Sandstorm answered one
// sent with no target, and one sent at a creature, on the caster), a group
// one on the caster's group within its range, anything else on the target.
func (w *World) castTargetsLocked(sess *session.Session, p *Player, spell *CastSpell, targetObjectID uint32) ([]EntityID, string) {
	d := &spell.Def
	// an illusion goes by its name, whatever its record's scope (illusion.go)
	switch d.FormKind {
	case FormSelf, FormEnd:
		return []EntityID{p.ServerID}, ""
	case FormGroup:
		return w.glamourTargetsLocked(sess, p, spell, targetObjectID)
	}
	// a resurrection whose line says Self goes on its caster, whatever its
	// record's scope (resurrect.go: the paladin's self-rez)
	if d.Target == TargetSelf && d.Resurrects() > 0 {
		return []EntityID{p.ServerID}, ""
	}
	// Call of the Hero goes on its caster, whatever its record's scope: it
	// asks its far group as it lands (callhero.go)
	if d.callsHero() {
		return []EntityID{p.ServerID}, ""
	}
	switch spell.Scope {
	case 0:
		return []EntityID{p.ServerID}, ""
	case 3:
		// the caster's pet, whatever is targeted: CLIENT sends its target as
		// it is and never puts the pet in its place (pet.go)
		pet := w.petOfLocked(p)
		if pet == nil {
			return nil, castNoPet
		}
		if outOfRange(p, pet.X, pet.Y, pet.Z, spell.Range) {
			return nil, refuseOutOfRange(p, spell, pet.Name, pet.X, pet.Y, pet.Z, spell.Range)
		}
		if !w.helpSeenLocked(p, d, pet.World, pet.X, pet.Y, pet.Z, false) { // sight.go
			return nil, castNotSeen
		}
		return []EntityID{pet.ServerID}, ""
	case 2:
		ids := []EntityID{p.ServerID}
		for _, m := range w.groupMembersLocked(p) {
			// the members its caster can see (sight.go)
			if m != p && !m.IsDead() && m.World == p.World && !outOfRange(p, m.X, m.Y, m.Z, spell.Range) &&
				w.helpSeenLocked(p, d, m.World, m.X, m.Y, m.Z, false) {
				ids = append(ids, m.ServerID)
			}
		}
		if d.hasProc() {
			ids = w.groupProcPetsLocked(p, ids, spell.Range) // enchanter.go
		}
		ids = w.groupMercsLocked(p, ids, spell.Range) // a merc is a member (mercgroup.go)
		return ids, ""
	}
	if targetObjectID == 0 || targetObjectID == 0xFFFFFFFF {
		return nil, castNoTarget
	}
	var ent Entity
	if targetObjectID == sess.PlayerObjectID() {
		ent = p
	} else if e, ok := w.findEntityByObjectIDLocked(sess, targetObjectID); ok {
		ent = e
	} else {
		return nil, castNoTarget
	}
	if ids, why, ok := w.duelSpellTargetLocked(p, spell, ent); ok { // a duel's (duelfight.go)
		return ids, why
	}
	if ids, why, ok := w.pvpSpellTargetLocked(p, spell, ent); ok { // two flagged players' (pvp.go)
		return ids, why
	}
	switch t := ent.(type) {
	case *Player:
		if d.Harmful {
			return nil, castFriendly
		}
		if t != p && outOfRange(p, t.X, t.Y, t.Z, spell.Range) {
			return nil, refuseOutOfRange(p, spell, t.Name, t.X, t.Y, t.Z, spell.Range)
		}
		if !w.helpSeenLocked(p, d, t.World, t.X, t.Y, t.Z, t == p) { // sight.go
			return nil, castNotSeen
		}
		return []EntityID{t.ServerID}, ""
	case *NPC:
		if t.Dead {
			return nil, castNoTarget
		}
		if t.Owner != 0 {
			// a pet is on the players' side: helped, never harmed (pet.go)
			if d.Harmful {
				return nil, castFriendly
			}
			if outOfRange(p, t.X, t.Y, t.Z, spell.Range) {
				return nil, refuseOutOfRange(p, spell, t.Name, t.X, t.Y, t.Z, spell.Range)
			}
			if !w.helpSeenLocked(p, d, t.World, t.X, t.Y, t.Z, false) { // sight.go
				return nil, castNotSeen
			}
			return []EntityID{t.ServerID}, ""
		}
		if !d.Harmful {
			return nil, castNotFriendly
		}
		if t.NPCType&0x80 != 0 {
			return nil, castFriendly // not attackable (CLIENT's own test, +0x25A & 0x80)
		}
		if line := challengeRefusal(t, p.ServerID); line != "" {
			return nil, line // another's duel (metalmoogie.go)
		}
		if t.challenge != nil && d.Charms() {
			return nil, duelLine(MetalLineCharm, t)
		}
		if reach := spell.Range + npcReachExtra(t); outOfRange(p, t.X, t.Y, t.Z, reach) {
			return nil, refuseOutOfRange(p, spell, t.Name, t.X, t.Y, t.Z, reach)
		}
		return []EntityID{t.ServerID}, ""
	}
	return nil, castNoTarget
}

// outOfRange is whether a place is beyond a spell's range from the caster:
// centre to centre in three dimensions, as CLIENT's TargetInRange measures a
// melee reach (0x00BDC4B8), so a target of the caster's height 3 ahead is in
// a reach of 4.5 and one 3.4 above or below it at that distance is not. A
// range of 0 is no limit: Renew's record has one, and it is cast at a corpse.
func outOfRange(p *Player, x, y, z, r float32) bool {
	if r <= 0 {
		return false
	}
	dx, dy, dz := p.X-x, p.Y-y, p.Z-z
	return float32(math.Sqrt(float64(dx*dx+dy*dy+dz*dz))) > r
}

// refuseOutOfRange is castOutOfRange, the distance logged as it was measured
// (across and up or down), so a refusal at a target that looked near is told
// from the log (jib's quest walk of 2026-10-30: Rapid Strike refused at a
// dummy #spawn had put 3 ahead).
func refuseOutOfRange(p *Player, spell *CastSpell, target string, x, y, z, r float32) string {
	dx, dy, dz := float64(p.X-x), float64(p.Y-y), float64(p.Z-z)
	log.Printf("Cast: %s's %q at %s: %.2f off (%.2f across, %.2f up or down), its reach %.2f",
		p.Name, spell.Name, target, math.Sqrt(dx*dx+dy*dy+dz*dz), math.Hypot(dx, dz), math.Abs(dy), r)
	return castOutOfRange
}

// groupMembersLocked is GroupMembers without taking the lock.
func (w *World) groupMembersLocked(p *Player) []*Player {
	g := w.groups[p.GroupID]
	if p.GroupID == 0 || g == nil {
		return nil
	}
	var out []*Player
	for _, id := range g.Members {
		if m := w.Players[id]; m != nil {
			out = append(out, m)
		}
	}
	return out
}

// broadcastCastLocked sends 0x00B1 to the caster and to everyone who sees
// it, each with the ids it knows the caster and the target by, and an area
// spell's others it sees (area.go).
func (w *World) broadcastCastLocked(caster *Player, target EntityID, castTime time.Duration, fx uint32, others ...EntityID) {
	if w.SessionManager == nil {
		return
	}
	ms := uint32(castTime / time.Millisecond)
	w.SessionManager.ForEach(func(s *session.Session) bool {
		if !s.CharacterInWorld() {
			return true
		}
		c := w.objectIDSeenByLocked(s, caster.ServerID)
		if c == 0 {
			return true
		}
		t := w.objectIDSeenByLocked(s, target)
		if t == 0 {
			t = c
		}
		w.sendCastingCombat(s, c, t, ms, fx, w.areaSeenLocked(s, others)...)
		return true
	})
}

// objectIDSeenByLocked is the id session s knows an entity by: its own
// player's, else the visibility system's (0 when it does not see it).
func (w *World) objectIDSeenByLocked(s *session.Session, id EntityID) uint32 {
	if p := w.sessionPlayerLocked(s); p != nil && p.ServerID == id {
		return s.PlayerObjectID()
	}
	return w.objectIDForEntityLocked(s, id)
}

// castSystem lands every cast whose time has come, and ends the lockouts
// that are over. Every tick; must be called with the lock held. It returns
// the players a gate sends home, for the gate hook.
func (w *World) castSystem(now time.Time) []respawnDue {
	var gates []respawnDue
	keep := w.casts[:0]
	for _, cs := range w.casts {
		if now.Before(cs.lands) {
			keep = append(keep, cs)
			continue
		}
		caster := w.Players[cs.caster]
		if caster == nil {
			continue
		}
		if caster.Casting == cs {
			caster.Casting = nil
		}
		if caster.IsDead() {
			continue
		}
		if g := w.landLocked(cs, caster, now); g {
			gates = append(gates, respawnDue{sess: w.findSessionByEntityIDLocked(caster.ServerID), p: caster})
		}
		w.handsBackLocked(cs, caster) // the hands its switch took off, back in melee reach
	}
	for i := len(keep); i < len(w.casts); i++ {
		w.casts[i] = nil
	}
	w.casts = keep

	for _, p := range w.Players {
		left := p.Lockouts[:0]
		for _, l := range p.Lockouts {
			if now.Before(l.until) {
				left = append(left, l)
				continue
			}
			if sess := w.findSessionByEntityIDLocked(p.ServerID); sess != nil {
				sendCooldown(sess, l.displayIndex, 0)
			}
		}
		p.Lockouts = left
	}
	return gates
}

// landLocked is one cast landing on each of its targets still there. It
// reports a gate for the caster.
func (w *World) landLocked(cs *castState, caster *Player, now time.Time) (gate bool) {
	d := &cs.spell.Def
	casterSess := w.findSessionByEntityIDLocked(caster.ServerID)
	if to := cs.spell.Travel; to != nil { // an item's travel: its caster sent, nothing else lands
		if casterSess != nil {
			w.gates.due = append(w.gates.due, GateTravel{Sess: casterSess, P: caster, To: *to, Spell: cs.spell.Name})
			log.Printf("Cast: %s's %q lands: to %s (%.1f, %.1f, %.1f) world %d", caster.Name, cs.spell.Name, to.Place, to.X, to.Y, to.Z, to.World)
		}
		return false
	}
	if f := cs.spell.Familiar; f != nil { // an item's familiar: its caster's, nothing else lands (familiar.go)
		w.summonFamiliarLocked(caster, casterSess, f, now)
		return false
	}
	for _, e := range d.Effects {
		if e.Kind == EffectGate {
			gate = true
		}
	}
	if w.summonItemsLocked(cs, caster, casterSess) && len(d.Effects) == 0 { // potion.go
		return gate
	}
	// a gate's line may read as its price alone ("Set Maximum Power to 0"),
	// which is its caster's (gates.go), never its travellers'
	if w.gateLandLocked(cs, caster, casterSess) && (len(d.Effects) == 0 || d.onlyPowerCaps()) { // gates.go
		return gate
	}
	if len(d.Effects) == 0 {
		log.Printf("Cast: %s's %q lands: nothing read in its line %v", caster.Name, cs.spell.Name, d.Unread)
	}
	// what it does to its caster, once (lifetap.go: a pact's price); the
	// targets get the rest
	cs = w.landOnCasterLocked(cs, caster, casterSess, now)
	d = &cs.spell.Def
	if d.Summons() {
		w.summonPetLocked(cs, caster, casterSess, now)
		return gate
	}
	for _, id := range cs.targets {
		if t := w.Players[id]; t != nil {
			if w.duelLandLocked(cs, caster, casterSess, t, now) { // harm on a player: a duel's (duelfight.go)
				continue
			}
			w.rezLandLocked(cs, caster, casterSess, t, now) // resurrect.go: on the dead too
			// a heal or a buff spares a body out of its caster's sight (sight.go)
			if !t.IsDead() && !w.helpHiddenLocked(cs, caster, casterSess, t.Name, t.World, t.X, t.Y, t.Z, t == caster) {
				w.landOnPlayerLocked(cs, caster, casterSess, t, now)
			}
			continue
		}
		if npc := w.NPCs[id]; npc != nil && !npc.Dead {
			if d.Charms() {
				w.charmLandLocked(cs, caster, casterSess, npc, now) // charm.go
				continue
			}
			if npc.Owner != 0 {
				if w.duelLandPetLocked(cs, caster, npc, now) { // harm on a pet: a duel's (duelfight.go)
					continue
				}
				if !w.helpHiddenLocked(cs, caster, casterSess, npc.Name, npc.World, npc.X, npc.Y, npc.Z, false) { // sight.go
					w.landOnPetLocked(cs, caster, casterSess, npc, now)
				}
				continue
			}
			if w.areaOwnSparedLocked(caster, cs, npc) { // area.go: behind a rock from its caster
				log.Printf("LoS: %s's %q spares its target %s at (%.1f, %.1f, %.1f), from (%.1f, %.1f, %.1f)", caster.Name, cs.spell.Name, npc.Name, npc.X, npc.Y, npc.Z, caster.X, caster.Y, caster.Z)
				continue
			}
			w.landOnNPCLocked(cs, caster, casterSess, npc, now)
		}
	}
	if d.callsHero() {
		return gate // its call is its caster's alone: no area's others (callhero.go)
	}
	w.landOthersLocked(cs, caster, casterSess, now) // area.go: an area spell's others
	return gate
}

// spellAmount is one effect's amount from this caster: a roll in its range,
// plus its percent of the caster's stat, signed as the effect is.
func spellAmount(e SpellEffect, caster *Player) int {
	n := rollAmount(e)
	bonus := 0
	if e.Scale != StatNone && e.ScalePct != 0 {
		bonus = caster.StatNow(e.Scale) * e.ScalePct / 100
	}
	// the workbook's level scaling: the caster's level times its share
	bonus += caster.Level * e.LevelPct / 100
	if n < 0 {
		n -= bonus
	} else {
		n += bonus
	}
	return capAmount(n, e.Cap)
}

// rollAmount is one effect's figure before any caster's stat: a roll in its
// range, signed as the effect is.
func rollAmount(e SpellEffect) int {
	n := e.Amount
	if e.AmountMax > e.Amount {
		n += spellIntn(e.AmountMax - e.Amount + 1)
	}
	return n
}

// capAmount is n held to cap either way (an effect's "max N"); cap 0 holds
// nothing.
func capAmount(n, cap int) int {
	if cap <= 0 {
		return n
	}
	return max(min(n, cap), -cap)
}

// lasting is the part of a spell that stays on its target for the
// duration, from this caster, or nil when nothing does; with what each of
// its lasting parts puts on when some may end apart (partsOf, parts.go).
func lasting(cs *castState, caster *Player, now time.Time) *ActiveEffect {
	ae := lastingParts(cs, caster, now)
	if ae != nil {
		ae.parts = partsOf(cs, caster, now, ae.Until.Sub(now))
	}
	return ae
}

// lastingParts is lasting's effect of the spell's parts as they are.
func lastingParts(cs *castState, caster *Player, now time.Time) *ActiveEffect {
	d := &cs.spell.Def
	dur := d.Duration
	if dur <= 0 {
		for _, e := range d.Effects {
			if e.Kind == EffectHPTick || e.Kind == EffectPowerTick {
				dur = tickDuration
			}
		}
	}
	if dur <= 0 {
		return nil
	}
	ae := &ActiveEffect{SpellID: cs.spell.SpellID, Name: cs.spell.Name, Icon: cs.spell.Icon, Caster: caster.ServerID,
		Until: now.Add(dur), NextTick: now, Ticks: d.Ticks}
	ae.Rules = rulesOf(d) // workbook.go
	ae.ProcSpells = d.ProcSpells
	ae.figures = cs.spell.figures // its ticks told as its cast's figures (numbertags.go)
	any := false
	for _, e := range d.Effects {
		if ae.workbookLasting(e, d) { // workbook.go: its own kinds
			any = true
			continue
		}
		switch e.Kind {
		case EffectHPTick:
			ae.HPTick += spellAmount(e, caster)
			if e.Drain != DrainNone && e.Amount < 0 {
				ae.TickDrain, ae.TickRange = e.Drain, cs.spell.Range
			}
		case EffectPowerTick:
			ae.PowerTick += spellAmount(e, caster)
		case EffectStat:
			if !e.Set { // a stat set is its bearer's to measure (setStatsLocked)
				ae.Mods.add(e.Stat, spellAmount(e, caster))
			}
		case EffectRoot:
			ae.Root = true
		case EffectSnare:
			ae.Snare = true
		case EffectSpeed:
			ae.Speed = true
		case EffectInvis:
			ae.Invis = true
		case EffectProc:
			// the line's damage as it is (jib: "Damage has no mod"), and a
			// line's own scale: its caster's (enchanter.go: Stormlord's Rune),
			// or its bearer's at each proc (proc.go)
			if e.ScaleBearer {
				ae.Proc, ae.ProcMax = e.Amount, max(e.AmountMax, e.Amount)
				ae.ProcScale, ae.ProcScalePct = e.Scale, e.ScalePct
			} else {
				bonus := procBonus(e, caster)
				ae.Proc, ae.ProcMax = e.Amount+bonus, max(e.AmountMax, e.Amount)+bonus
			}
			ae.ProcCap = e.Cap
			ae.ProcUndead = e.Undead
			if e.Cap > 0 && !e.ScaleBearer {
				ae.Proc, ae.ProcMax = min(ae.Proc, e.Cap), min(ae.ProcMax, e.Cap)
			}
		case EffectIllusion:
			// its body is its bearer's to choose (landIllusionLocked)
		case EffectShield, EffectDefProc:
			ae.Shield = ae.Shield.with(e, cs, caster) // dshield.go
		case EffectPowerCap:
			if !ae.PowerCapped || e.Amount < ae.PowerCap {
				ae.PowerCap, ae.PowerCapped = e.Amount, true
			}
		case EffectSize:
			ae.Rules.Grow += e.Amount // the body's scale (Player.Body)
		default:
			continue
		}
		any = true
	}
	if !any {
		// a duration over nothing that lasts (a hit with "30sec"): shown,
		// as Sandstorm shows Minor Healing's, and gone at its end
		ae.NextTick = time.Time{}
	}
	return ae
}

// landOnPlayerLocked is a beneficial spell on a player: its instant part,
// then what lasts, on the effects channel.
func (w *World) landOnPlayerLocked(cs *castState, caster *Player, casterSess *session.Session, t *Player, now time.Time) {
	if w.tooLowLocked(caster, casterSess, t, &cs.spell) { // workbook.go: below its lowest level
		return
	}
	illusion := false
	// a spell that raises the maximum heals after the raise (spellnotes.go),
	// or a target at full health would get none of it
	raises := cs.spell.Def.RaisesMaxHP()
	// a self spell's lost hit points are its caster's price, paid in full
	// (lifetap.go selfCostLocked): a price that kills lands nothing more
	ownPrice := cs.spell.ownPrice(caster, t)
	var heals []int
	for _, e := range cs.spell.Def.Effects {
		switch e.Kind {
		case EffectHP:
			n := spellAmount(e, caster)
			if n < 0 && ownPrice {
				if w.selfCostLocked(cs.spell.Name, caster, casterSess, -n, true, false, now) {
					return
				}
				continue
			}
			if raises {
				heals = append(heals, n)
				continue
			}
			w.healLocked(caster, casterSess, t, n, cs.spell.healKind())
		case EffectPower:
			if e.Set { // the workbook's power set to a figure (spelljson.go)
				t.Power = min(max(e.Amount, 0), t.MaxPowerNow())
				t.StatsDirty = true
			} else if n := spellAmount(e, caster); n > 0 {
				t.Power = min(t.Power+n, t.MaxPowerNow())
				t.StatsDirty = true
			}
		case EffectPowerCap:
			// lasting no time, the ceiling is a price paid now (powercap.go);
			// one that lasts holds the power down as it goes on
			if !cs.spell.Def.lasts() && t.Power > e.Amount {
				t.Power = max(e.Amount, 0)
				t.StatsDirty = true
			}
		case EffectEndIllusion:
			w.endIllusionsLocked(t, 0) // illusion.go
		case EffectIllusion:
			illusion = true
		case EffectCure:
			w.cureLocked(cs, t) // workbook.go
		case EffectSummonPlayer:
			if t == caster {
				w.heroCallLandLocked(cs, caster, casterSess) // callhero.go: the far group asked
			}
		}
	}
	if ae := lasting(cs, caster, now); ae != nil && w.lineTakesLocked(caster, casterSess, t, &cs.spell, ae) && // buffline.go
		(ae.HPTick <= 0 || w.placeHoTLocked(t, ae, now)) && // hot.go
		w.placeClarityLocked(caster, casterSess, t, ae, now) { // enchanter.go
		w.dropLineLocked(t, ae) // its line's lower ranks off (buffline.go)
		if illusion {
			w.landIllusionLocked(cs, t, ae) // illusion.go
		}
		setStatsLocked(&cs.spell.Def, t, ae) // vampire.go: "Set STR to 500"
		w.addEffectLocked(t, ae, now)
	}
	for _, n := range heals {
		w.healLocked(caster, casterSess, t, n, cs.spell.healKind())
	}
	log.Printf("Cast: %s's %q lands on %s (%d/%d HP, %d effects)", caster.Name, cs.spell.Name, t.Name, t.HP, t.MaxHPNow(), len(t.Effects))
}

// healLocked restores n hit points to t; the amount is told (0x00DB, whole,
// as Sandstorm tells it, kind to the caster: tellAmountLocked) only when t was
// short of its maximum.
func (w *World) healLocked(caster *Player, casterSess *session.Session, t *Player, n int, kind numberKind) {
	if n <= 0 {
		return
	}
	top := t.MaxHPNow()
	if t.HP >= top {
		return
	}
	before := t.HP
	t.HP = min(t.HP+n, top)
	t.StatsDirty = true
	w.tellAmountLocked(caster, casterSess, t.ServerID, n, kind)
	w.healHateLocked(caster, t, t.HP-before) // aggro.go
	if t.ServerID == caster.ServerID {
		w.healAggroLocked(caster) // a creature it is green to comes now (aggro.go)
	}
}

// tellAmountLocked sends 0x00DB (caster, amount, target) to the caster, told
// as kind (what made it, numbertags.go: 0 for a figure none of the caster's
// made), and, when the target is another player, to it as a figure another
// made on it (untagged).
func (w *World) tellAmountLocked(caster *Player, casterSess *session.Session, target EntityID, amount int, kind numberKind) {
	if casterSess != nil {
		if tid := w.objectIDSeenByLocked(casterSess, target); tid != 0 {
			w.sendDamageIndication(casterSess, casterSess.PlayerObjectID(), amount, tid, kind)
		}
	}
	if target == caster.ServerID {
		return
	}
	if ts := w.findSessionByEntityIDLocked(target); ts != nil {
		if cid := w.objectIDSeenByLocked(ts, caster.ServerID); cid != 0 {
			w.sendDamageIndication(ts, cid, amount, ts.PlayerObjectID(), 0)
		}
	}
}

// addEffectLocked puts an effect on a player: a recast replaces its old one
// and goes to the end of the list, as on Sandstorm. The run speed compared is
// the one after the old one comes off, as that told the client it: taken before,
// a recast of a run speed still on told the client the slower speed and never
// the faster again (petition 1111: "SoW when cast on self does not seem to
// work? my dual box character runs so far ahead").
func (w *World) addEffectLocked(t *Player, ae *ActiveEffect, now time.Time) {
	w.removeEffectLocked(t, ae.SpellID)
	speed := t.SpeedNow()
	t.Effects = append(t.Effects, ae)
	if ae.Name != "" && len(t.Buffs) < MaxEffects {
		t.Buffs = append(t.Buffs, BuffSlot{Icon: ae.Icon, Name: ae.Name})
	}
	w.effectsChangedLocked(t, speed)
	w.tickEffectLocked(t, ae, now)
}

// removeEffectLocked takes spell's effect off a player, and its slot off
// the effects channel.
func (w *World) removeEffectLocked(t *Player, spellID int) bool {
	for i, e := range t.Effects {
		if e.SpellID != spellID {
			continue
		}
		speed := t.SpeedNow()
		t.Effects = append(t.Effects[:i], t.Effects[i+1:]...)
		for j, b := range t.Buffs {
			if b.Name == e.Name && b.Icon == e.Icon {
				t.Buffs = append(t.Buffs[:j], t.Buffs[j+1:]...)
				break
			}
		}
		w.effectsChangedLocked(t, speed)
		return true
	}
	return false
}

// effectsChangedLocked sums the effects' stat changes again, keeps the hit
// points and power within the maximums they make, and tells the client of a
// new run speed. speed is the run speed before the change.
func (w *World) effectsChangedLocked(t *Player, speed float32) {
	t.Mods = sumEffectMods(t.Effects) // buffline.go: a stat nested lines share counts once

	t.HP = min(t.HP, t.MaxHPNow())
	t.Power = min(t.Power, t.MaxPowerNow())
	t.StatsDirty = true
	if s := t.SpeedNow(); s != speed {
		if sess := w.findSessionByEntityIDLocked(t.ServerID); sess != nil {
			sendPlayerSpeed(sess, s)
		}
	}
}

// tickEffectLocked runs an effect's tick on a player when one is due.
func (w *World) tickEffectLocked(t *Player, ae *ActiveEffect, now time.Time) {
	if ae.NextTick.IsZero() || now.Before(ae.NextTick) || (ae.HPTick == 0 && ae.PowerTick == 0) {
		return
	}
	w.advanceTickLocked(ae, now) // hot.go
	times := 1
	if ae.Rules.Twice { // the workbook's first tick, twice (workbook.go)
		ae.Rules.Twice, times = false, 2
	}
	for ; times > 0; times-- {
		w.runTickLocked(t, ae, now)
	}
}

// runTickLocked is one tick of an effect on a player.
func (w *World) runTickLocked(t *Player, ae *ActiveEffect, now time.Time) {
	if ae.HPTick < 0 {
		w.duelTickLocked(t, ae, now) // a duel's damage over time (duelfight.go)
		w.npcTickLocked(t, ae, now)  // a creature's (classtemplate.go)
		// the bearer's own spell losing hit points each tick (Cursed Flesh's
		// "Decrease HP per tic by 800"): its price, until it wears off or a
		// Remove Illusion or True Form ends its look, never below 1
		// (lifetap.go selfCostLocked)
		if ae.Caster == t.ServerID && ae.duelFoe == 0 && ae.npcFoe == 0 {
			w.selfCostLocked(ae.Name, t, w.findSessionByEntityIDLocked(t.ServerID), -ae.HPTick, false, true, now)
		}
	}
	if ae.HPTick > 0 {
		// the caster may be gone (logged out, or the effect kept over a
		// logout): the figure is then the target's own, as Sandstorm's first
		// tick after a login was, and none of its making (numbertags.go: a
		// player's own effect has the player for its caster, there while it
		// ticks)
		caster, kind := w.Players[ae.Caster], ae.tickKind(true)
		if caster == nil {
			caster, kind = t, 0
		}
		w.healLocked(caster, w.findSessionByEntityIDLocked(caster.ServerID), t, ae.HPTick, kind)
	}
	if ae.PowerTick > 0 && t.Power < t.MaxPowerNow() {
		t.Power = min(t.Power+ae.PowerTick, t.MaxPowerNow())
		t.StatsDirty = true
	}
}

// landOnNPCLocked is a harmful spell on a creature: its hit, its hate, and
// what lasts (a damage tick; a hold, a slow or a stat change is kept, though
// no creature moves or uses its stats yet).
func (w *World) landOnNPCLocked(cs *castState, caster *Player, casterSess *session.Session, npc *NPC, now time.Time) {
	if challengeBars(npc, caster.ServerID) {
		return // another's duel: nothing of the spell lands (metalmoogie.go)
	}
	caster.LastCombatTime = now
	cs, took := w.resistLocked(cs, caster, casterSess, npc, now) // workbook.go: its resists
	if !took {
		w.updateAggro(npc, caster.ServerID, 1) // resisted, it is angered all the same
		return
	}
	for _, e := range cs.spell.Def.Effects {
		switch e.Kind {
		case EffectHP:
			if n := creatureResists(e.Resist, npc, arcaneDebuffed(e.Resist, npc, debuffScaled(e, npc, spellAmount(e, caster), now), now), now); n < 0 {
				killed := w.spellHitHateLocked(caster, casterSess, npc, -n, e.HateOff, cs.spell.hitKind())
				// a life tap's heal (lifetap.go): what landed, after the
				// con's cut (balance.go)
				w.drainLocked(cs, caster, casterSess, e, w.dealtLocked(npc, caster, -n), now)
				if killed {
					return
				}
			}
		case EffectHate:
			w.updateAggro(npc, caster.ServerID, spellAmount(e, caster))
		case EffectForget:
			w.forgetLocked(npc, caster) // vampire.go: Vampiric Gaze
		case EffectPickpocket:
			w.pickpocketLocked(caster, casterSess, npc, now) // workbookmore.go
		default:
			// a drained stat or power is the caster's (lifetap.go)
			w.drainLocked(cs, caster, casterSess, e, -spellAmount(e, caster), now)
		}
	}
	if cs.spell.Def.forgets() {
		return // the gaze angers nothing: what lasts of it, and its hate of 1, are not given
	}
	// a spell that only holds or slows still angers it, a resisted root too
	// (root.go): hate of 1, as a resisted charm's, since a hate of none makes
	// no enemy (addThreatLocked takes the highest above 0); a lull that took
	// angers nothing (workbook.go). It comes before what lasts of the spell
	// goes on: the anger is a contact (contactNPCLocked), which ends what was
	// on the creature that ends at a hit, and until 2026-11-01 it came after,
	// so every root and snare that ends at contact ended at its own landing
	// (Root, Arcane Bindings, Caltrops, Enshrouding darkness: the user's
	// "until hit")
	if !cs.spell.Def.lulls() && !cs.spell.Def.picks() { // a pickpocket angers only when caught (workbookmore.go)
		w.updateAggro(npc, caster.ServerID, 1)
	}
	ae := lasting(cs, caster, now)
	if ae != nil && cs.spell.Def.hasIllusion() {
		w.landNPCIllusionLocked(cs, npc, ae) // illusion.go: a thrown potion's body on the creature
	}
	if ae != nil && w.rootResistLocked(caster, casterSess, npc, ae, now) { // root.go
		for i, old := range npc.Effects {
			if old.SpellID == ae.SpellID {
				npc.Effects = append(npc.Effects[:i], npc.Effects[i+1:]...)
				break
			}
		}
		ae.Line, ae.Rank = BuffLine(&cs.spell), lineRank(&cs.spell) // only the best of a line counts (pet.go's creatureMods)
		npc.Effects = append(npc.Effects, ae)
		npc.syncEffectMaxHP() // a maximum's debuff (pet.go's creatureMods)
		if w.affected == nil {
			w.affected = make(map[EntityID]*NPC)
		}
		w.affected[npc.ServerID] = npc
		w.tickNPCEffectLocked(npc, ae, now)
	}
}

// spellHitLocked is a spell's damage on a creature: the figure to the
// caster (told as kind: numbertags.go), the hate, and on a kill the XP and
// the faction hits. It reports the kill.
func (w *World) spellHitLocked(caster *Player, casterSess *session.Session, npc *NPC, n int, kind numberKind) bool {
	return w.spellHitHateLocked(caster, casterSess, npc, n, 0, kind)
}

// spellHitHateLocked is spellHitLocked with hateOff percent of the damage
// dealt making no hate (SpellEffect.HateOff): the rest is the hate, and the
// hit angers the creature all the same (updateAggro's contact, its kind
// called, its companion), so a hit of no hate still makes a fight.
func (w *World) spellHitHateLocked(caster *Player, casterSess *session.Session, npc *NPC, n, hateOff int, kind numberKind) bool {
	if npc.Dummy != NotDummy {
		// told, taken off nothing (dummy.go)
		w.tellAmountLocked(caster, casterSess, npc.ServerID, -n, kind)
		w.dummyEngagedLocked(npc, caster, time.Now())
		return false
	}
	n = w.dealtLocked(npc, caster, n) // balance.go
	npc.HP = max(npc.HP-n, 0)
	w.tellAmountLocked(caster, casterSess, npc.ServerID, -n, kind)
	w.updateAggro(npc, caster.ServerID, n-n*hateOff/100)
	w.rootBreakLocked(npc, caster.Level) // root.go
	log.Printf("Cast: %s's spell hit %s for %d (HP %d/%d)", caster.Name, npc.Name, n, npc.HP, npc.MaxHP)
	if npc.HP > 0 {
		w.hitHealLocked(npc, n, casterSess) // its heal at every hit that lands (hitheal.go)
		return false
	}
	npc.Effects = nil
	w.killNPC(npc)
	w.awardKillXPLocked(casterSess, caster, npc) // killexp.go
	w.factionKillLocked(casterSess, caster, npc)
	if caster.AttackTarget == npc.ServerID {
		caster.disengage()
	}
	return true
}

// tickNPCEffectLocked runs a damage tick on a creature when one is due.
func (w *World) tickNPCEffectLocked(npc *NPC, ae *ActiveEffect, now time.Time) {
	if ae.NextTick.IsZero() || now.Before(ae.NextTick) || ae.HPTick == 0 {
		return
	}
	if ae.pet != 0 {
		w.petTickLocked(npc, ae, now) // a pet's damage over time (pettemplate.go)
		return
	}
	if ae.npcCaster != 0 {
		w.npcDoTTickLocked(npc, ae, now) // a creature's on the creature it fights (creaturedot.go)
		return
	}
	if ae.merc != 0 {
		w.mercTickLocked(npc, ae, now) // a merc's (mercfight.go)
		return
	}
	if ae.HPTick > 0 {
		// a heal over time cast on it as a charmed pet, kept once it went
		// free (charm.go); a pet's own tick in petEffectsLocked
		if npc.Owner == 0 && !npc.Dead {
			ae.NextTick = ae.NextTick.Add(effectTick)
			npc.HP = min(npc.HP+ae.HPTick, npc.MaxHP)
		}
		return
	}
	ae.NextTick = ae.NextTick.Add(effectTick)
	times := 1
	if ae.Rules.Twice { // the workbook's first tick, twice (workbook.go)
		ae.Rules.Twice, times = false, 2
	}
	ae.ticking = true
	defer func() { ae.ticking = false }()
	for ; times > 0; times-- {
		if caster := w.Players[ae.Caster]; caster != nil && !npc.Dead {
			sess := w.findSessionByEntityIDLocked(caster.ServerID)
			hit := -creatureResists(ae.Rules.Resist, npc, arcaneDebuffed(ae.Rules.Resist, npc, ae.HPTick, now), now)
			died := w.spellHitLocked(caster, sess, npc, hit, ae.tickKind(false))
			// a drain over time: each tick's figure to the caster (or its
			// group), whole, as a tap's is though it killed (lifetap.go)
			if ae.TickDrain != DrainNone && !caster.IsDead() {
				for _, p := range w.tickDrainersLocked(caster, ae) {
					w.healLocked(caster, sess, p, hit, ae.tickKind(true))
				}
			}
			if died {
				return
			}
		}
	}
}

// effectSystem runs the effects' ticks and takes off the ones that are over.
// Every tick; must be called with the lock held.
func (w *World) effectSystem(now time.Time) {
	for _, p := range w.Players {
		if len(p.Effects) == 0 {
			continue
		}
		for _, ae := range append([]*ActiveEffect(nil), p.Effects...) {
			if !now.Before(ae.Until) {
				if ae.Ticks > 0 {
					w.tickEffectLocked(p, ae, now) // a heal's last tick due as it ends (hot.go)
				}
				w.removeEffectLocked(p, ae.SpellID)
				log.Printf("Cast: %q wears off %s", ae.Name, p.Name)
				continue
			}
			w.tickEffectLocked(p, ae, now)
		}
	}
	// only the creatures a spell is on: there are tens of thousands
	for id, npc := range w.affected {
		if len(npc.Effects) == 0 || npc.Dead || w.NPCs[id] != npc {
			npc.Effects = nil
			if !npc.Dead && npc.effMaxHP != 0 {
				was := npc.MaxHP
				npc.syncEffectMaxHP() // its last maximum's effect ended (pet.go)
				if npc.MaxHP > was {
					w.wakeNPCLocked(npc) // short of its maximum: the regeneration's (npcsets.go)
				}
			}
			delete(w.affected, id)
			continue
		}
		left := npc.Effects[:0]
		for _, ae := range npc.Effects {
			if now.Before(ae.Until) {
				left = append(left, ae)
			}
		}
		npc.Effects = left
		was := npc.MaxHP
		npc.syncEffectMaxHP() // a maximum's effect that ended taken back (pet.go)
		if npc.MaxHP > was {
			w.wakeNPCLocked(npc) // short of its maximum: the regeneration's (npcsets.go)
		}
		for _, ae := range append([]*ActiveEffect(nil), npc.Effects...) {
			w.tickNPCEffectLocked(npc, ae, now)
			if npc.Dead {
				break
			}
		}
	}
}

// clearSpellsLocked ends a dead player's cast and takes its effects off, all
// but a town's boon, which outlives a death (boon.go).
func (w *World) clearSpellsLocked(p *Player) {
	p.Casting = nil
	for _, ae := range append([]*ActiveEffect(nil), p.Effects...) {
		if ae.SpellID != BoonEffectID {
			w.removeEffectLocked(p, ae.SpellID)
		}
	}
}

// runGates hands each player a gate sent home to the gate hook. Must be
// called WITHOUT the lock.
func (w *World) runGates(due []respawnDue) {
	if len(due) == 0 {
		return
	}
	w.mu.RLock()
	fn := w.onGate
	w.mu.RUnlock()
	for _, d := range due {
		if fn != nil && d.sess != nil {
			fn(d.sess, d.p)
		}
	}
}

// sendCooldown is 0x00CD: the ability's display index, stamp 0, the lockout
// in ms (zigzag), 0 when it is over. A lockout of no index (-1: its spell
// left the book, bookchange.go) is shown on nothing, and nothing is sent.
func sendCooldown(sess *session.Session, displayIndex int, lockout time.Duration) {
	if displayIndex < 0 {
		return
	}
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCooldownTimer)
	w := buffer.NewWriter(msg.Span())
	w.WriteUint16(uint16(protocol.OpcodeCooldownTimer))
	w.WriteByte(byte(displayIndex))
	w.WriteUint32(0)
	w.Write7BitEncodedInt64(int64(lockout / time.Millisecond))
	msg.SetSize(w.Position())
	sess.Queue().AddWithSequence(msg)
}

// sendPlayerSpeed is 0x00F8: the run speed, a float.
func sendPlayerSpeed(sess *session.Session, speed float32) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodePlayerSpeed)
	w := buffer.NewWriter(msg.Span())
	w.WriteUint16(uint16(protocol.OpcodePlayerSpeed))
	w.WriteUint32(math.Float32bits(speed))
	msg.SetSize(w.Position())
	sess.Queue().AddWithSequence(msg)
}

// sendColoredLocked is 0x0A7B: a line in a colour.
func (w *World) sendColoredLocked(sess *session.Session, text string, rgb [3]byte) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeColoredChat)
	wr := buffer.NewWriter(msg.Span())
	wr.WriteUint16(uint16(protocol.OpcodeColoredChat))
	wr.WriteStringUnicode(text)
	wr.WriteByte(rgb[0])
	wr.WriteByte(rgb[1])
	wr.WriteByte(rgb[2])
	msg.SetSize(wr.Position())
	sess.Queue().AddWithSequence(msg)
}
