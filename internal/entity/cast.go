package entity

import (
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
//   - a cast sent during another, or during its own lockout: nothing at all.
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
	// resist is the resist that cuts a templated NPC's spell's hit on a
	// player, by its element (classtemplate.go); a player's spell has none
	resist Stat
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
	def := ParseSpellNamed(sp.SpellName, line, sp.SpellDesc)
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
	spell   CastSpell
	caster  EntityID
	targets []EntityID
	others  []EntityID // an area spell's others (area.go)
	ends    time.Time  // the cast time over: the caster may cast again
	lands   time.Time
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
	Mods      StatMods
	Root      bool
	Snare     bool
	Speed     bool
	Invis     bool
	// Proc is a proc's damage (proc.go): what the bearer's melee hits may do
	// more, 0 for none; ProcMax the top of its range.
	Proc, ProcMax int
	// ProcScale and ProcScalePct: a proc whose scale is its bearer's
	// (SpellEffect.ScaleBearer) adds that share of the bearer's stat at each
	// proc; ProcCap is the most a proc does, 0 for no cap.
	ProcScale    Stat
	ProcScalePct int
	ProcCap      int
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
	// duelFoe is the duel opponent that put a harmful effect on a player
	// (duelfight.go), 0 for every other effect: it alone holds and slows a
	// player (SpeedNow), comes off at the duel's end and is never kept over
	// a logout (buffsave.go)
	duelFoe EntityID
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
	}
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
	n := p.MaxHP + p.Mods.MaxHP
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
		// a duel opponent's hold or slow (duelfight.go); no other harm lands
		// on a player
		if e.duelFoe != 0 && e.Root {
			return 0
		}
		if e.duelFoe != 0 && e.Snare {
			slow = duelSnare
		}
	}
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
	if p.Casting != nil && now.Before(p.Casting.ends) {
		log.Printf("Cast: %s casts %q during %q: not answered", p.Name, spell.Name, p.Casting.spell.Name)
		return false
	}
	for _, l := range p.Lockouts {
		if l.spellID == spell.SpellID && now.Before(l.until) {
			log.Printf("Cast: %s casts %q, locked out for %v: not answered", p.Name, spell.Name, l.until.Sub(now).Round(time.Millisecond))
			return false
		}
	}
	// a charm while a pet is out is refused before anything is looked at,
	// whatever is targeted (charm.go)
	refusal := w.charmPetRefusalLocked(p, &spell)
	var targets []EntityID
	if refusal == "" {
		targets, refusal = w.castTargetsLocked(sess, p, &spell, targetObjectID)
	}
	fizzle := false
	if refusal == "" && spell.Def.Harmful {
		refusal = w.waterRefusalLocked(p, targets) // water.go
	}
	if refusal == "" && spell.Def.undeadOnly() {
		refusal, fizzle = w.undeadStartLocked(targets) // undead.go
	}
	if refusal == "" && spell.Def.Charms() {
		refusal = w.charmStartLocked(&spell, targets) // charm.go
	}
	if refusal == "" && spell.Def.Resurrects() > 0 {
		refusal, fizzle = w.rezStartLocked(targets, &spell.Def, now) // resurrect.go
	}
	if refusal == "" {
		refusal = w.gateStartLocked(sess, &spell) // gates.go
	}
	if refusal == "" && p.Power < spell.PowerCost {
		refusal = castNoPower
	}
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
	w.breakInvisLocked(p, "a spell cast") // invis.go
	lockout := spell.CastTime + spell.Recast + castLockoutExtra
	p.Lockouts = append(p.Lockouts, castLockout{spellID: spell.SpellID, displayIndex: spell.DisplayIndex, until: now.Add(lockout)})
	cs := &castState{spell: spell, caster: p.ServerID, targets: targets, ends: now.Add(spell.CastTime + castBusy), lands: now.Add(spell.CastTime + castLand)}
	w.areaStartLocked(p, cs, now) // area.go: an area spell's others
	p.Casting = cs
	w.casts = append(w.casts, cs)

	sendCooldown(sess, spell.DisplayIndex, lockout)
	target := p.ServerID
	if len(targets) > 0 {
		target = targets[0]
	}
	w.broadcastCastLocked(p, target, spell.CastTime, spell.visual(), cs.others...)
	log.Printf("Cast: %s begins %q (spell %d) on %#x: %v cast, %d power (%d left), lands in %v",
		p.Name, spell.Name, spell.SpellID, targetObjectID, spell.CastTime, spell.PowerCost, p.Power, spell.CastTime+castLand)
	return true
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
			return nil, castOutOfRange
		}
		return []EntityID{pet.ServerID}, ""
	case 2:
		ids := []EntityID{p.ServerID}
		for _, m := range w.groupMembersLocked(p) {
			if m != p && !m.IsDead() && m.World == p.World && !outOfRange(p, m.X, m.Y, m.Z, spell.Range) {
				ids = append(ids, m.ServerID)
			}
		}
		if d.hasProc() {
			ids = w.groupProcPetsLocked(p, ids, spell.Range) // enchanter.go
		}
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
	switch t := ent.(type) {
	case *Player:
		if d.Harmful {
			return nil, castFriendly
		}
		if t != p && outOfRange(p, t.X, t.Y, t.Z, spell.Range) {
			return nil, castOutOfRange
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
				return nil, castOutOfRange
			}
			return []EntityID{t.ServerID}, ""
		}
		if !d.Harmful {
			return nil, castNotFriendly
		}
		if t.NPCType&0x80 != 0 {
			return nil, castFriendly // not attackable (CLIENT's own test, +0x25A & 0x80)
		}
		if outOfRange(p, t.X, t.Y, t.Z, spell.Range+npcReachExtra(t)) {
			return nil, castOutOfRange
		}
		return []EntityID{t.ServerID}, ""
	}
	return nil, castNoTarget
}

// outOfRange is whether a place is beyond a spell's range from the caster.
// A range of 0 is no limit: Renew's record has one, and it is cast at a
// corpse.
func outOfRange(p *Player, x, y, z, r float32) bool {
	if r <= 0 {
		return false
	}
	dx, dy, dz := p.X-x, p.Y-y, p.Z-z
	return float32(math.Sqrt(float64(dx*dx+dy*dy+dz*dz))) > r
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
			if !t.IsDead() {
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
				w.landOnPetLocked(cs, caster, casterSess, npc, now)
				continue
			}
			w.landOnNPCLocked(cs, caster, casterSess, npc, now)
		}
	}
	w.landOthersLocked(cs, caster, casterSess, now) // area.go: an area spell's others
	return gate
}

// spellAmount is one effect's amount from this caster: a roll in its range,
// plus its percent of the caster's stat, signed as the effect is.
func spellAmount(e SpellEffect, caster *Player) int {
	n := rollAmount(e)
	if e.Scale != StatNone && e.ScalePct != 0 {
		bonus := caster.StatNow(e.Scale) * e.ScalePct / 100
		if n < 0 {
			n -= bonus
		} else {
			n += bonus
		}
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
// duration, from this caster, or nil when nothing does.
func lasting(cs *castState, caster *Player, now time.Time) *ActiveEffect {
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
	any := false
	for _, e := range d.Effects {
		switch e.Kind {
		case EffectHPTick:
			ae.HPTick += spellAmount(e, caster)
		case EffectPowerTick:
			ae.PowerTick += spellAmount(e, caster)
		case EffectStat:
			ae.Mods.add(e.Stat, spellAmount(e, caster))
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
	illusion := false
	// a spell that raises the maximum heals after the raise (spellnotes.go),
	// or a target at full health would get none of it
	raises := cs.spell.Def.RaisesMaxHP()
	var heals []int
	for _, e := range cs.spell.Def.Effects {
		switch e.Kind {
		case EffectHP:
			if raises {
				heals = append(heals, spellAmount(e, caster))
				continue
			}
			w.healLocked(caster, casterSess, t, spellAmount(e, caster))
		case EffectPower:
			if n := spellAmount(e, caster); n > 0 {
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
		}
	}
	if ae := lasting(cs, caster, now); ae != nil && (ae.HPTick <= 0 || w.placeHoTLocked(t, ae, now)) && // hot.go
		w.placeClarityLocked(caster, casterSess, t, ae, now) { // enchanter.go
		if illusion {
			w.landIllusionLocked(cs, t, ae) // illusion.go
		}
		w.addEffectLocked(t, ae, now)
	}
	for _, n := range heals {
		w.healLocked(caster, casterSess, t, n)
	}
	log.Printf("Cast: %s's %q lands on %s (%d/%d HP, %d effects)", caster.Name, cs.spell.Name, t.Name, t.HP, t.MaxHPNow(), len(t.Effects))
}

// healLocked restores n hit points to t; the amount is told (0x00DB, whole,
// as Sandstorm tells it) only when t was short of its maximum.
func (w *World) healLocked(caster *Player, casterSess *session.Session, t *Player, n int) {
	if n <= 0 {
		return
	}
	top := t.MaxHPNow()
	if t.HP >= top {
		return
	}
	t.HP = min(t.HP+n, top)
	t.StatsDirty = true
	w.tellAmountLocked(caster, casterSess, t.ServerID, n)
}

// tellAmountLocked sends 0x00DB (caster, amount, target) to the caster and,
// when the target is another player, to it.
func (w *World) tellAmountLocked(caster *Player, casterSess *session.Session, target EntityID, amount int) {
	if casterSess != nil {
		if tid := w.objectIDSeenByLocked(casterSess, target); tid != 0 {
			w.sendDamageIndication(casterSess, casterSess.PlayerObjectID(), amount, tid)
		}
	}
	if target == caster.ServerID {
		return
	}
	if ts := w.findSessionByEntityIDLocked(target); ts != nil {
		if cid := w.objectIDSeenByLocked(ts, caster.ServerID); cid != 0 {
			w.sendDamageIndication(ts, cid, amount, ts.PlayerObjectID())
		}
	}
}

// addEffectLocked puts an effect on a player: a recast replaces its old one
// and goes to the end of the list, as on Sandstorm.
func (w *World) addEffectLocked(t *Player, ae *ActiveEffect, now time.Time) {
	speed := t.SpeedNow()
	w.removeEffectLocked(t, ae.SpellID)
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
	var m StatMods
	for _, e := range t.Effects {
		m.sum(e.Mods)
	}
	t.Mods = m
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
	if ae.HPTick < 0 {
		w.duelTickLocked(t, ae, now) // a duel's damage over time (duelfight.go)
	}
	if ae.HPTick > 0 {
		// the caster may be gone (logged out, or the effect kept over a
		// logout): the figure is then the target's own, as Sandstorm's first
		// tick after a login was
		caster := w.Players[ae.Caster]
		if caster == nil {
			caster = t
		}
		w.healLocked(caster, w.findSessionByEntityIDLocked(caster.ServerID), t, ae.HPTick)
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
	caster.LastCombatTime = now
	for _, e := range cs.spell.Def.Effects {
		switch e.Kind {
		case EffectHP:
			if n := spellAmount(e, caster); n < 0 {
				killed := w.spellHitLocked(caster, casterSess, npc, -n)
				// a life tap's heal (lifetap.go): what landed, after the
				// con's cut (balance.go)
				w.drainLocked(cs, caster, casterSess, e, w.takenLocked(npc, caster.Level, -n), now)
				if killed {
					return
				}
			}
		case EffectHate:
			w.updateAggro(npc, caster.ServerID, spellAmount(e, caster))
		default:
			// a drained stat or power is the caster's (lifetap.go)
			w.drainLocked(cs, caster, casterSess, e, -spellAmount(e, caster), now)
		}
	}
	if ae := lasting(cs, caster, now); ae != nil {
		for i, old := range npc.Effects {
			if old.SpellID == ae.SpellID {
				npc.Effects = append(npc.Effects[:i], npc.Effects[i+1:]...)
				break
			}
		}
		npc.Effects = append(npc.Effects, ae)
		if w.affected == nil {
			w.affected = make(map[EntityID]*NPC)
		}
		w.affected[npc.ServerID] = npc
		w.tickNPCEffectLocked(npc, ae, now)
	}
	// a spell that only holds or slows still angers it
	w.updateAggro(npc, caster.ServerID, 0)
}

// spellHitLocked is a spell's damage on a creature: the figure to the
// caster, the hate, and on a kill the XP and the faction hits. It reports
// the kill.
func (w *World) spellHitLocked(caster *Player, casterSess *session.Session, npc *NPC, n int) bool {
	if npc.Dummy != NotDummy {
		// told, taken off nothing (dummy.go)
		w.tellAmountLocked(caster, casterSess, npc.ServerID, -n)
		w.dummyEngagedLocked(npc, caster, time.Now())
		return false
	}
	n = w.takenLocked(npc, caster.Level, n) // balance.go
	npc.HP = max(npc.HP-n, 0)
	w.tellAmountLocked(caster, casterSess, npc.ServerID, -n)
	w.updateAggro(npc, caster.ServerID, n)
	log.Printf("Cast: %s's spell hit %s for %d (HP %d/%d)", caster.Name, npc.Name, n, npc.HP, npc.MaxHP)
	if npc.HP > 0 {
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
	if ae.NextTick.IsZero() || now.Before(ae.NextTick) || ae.HPTick >= 0 {
		return
	}
	ae.NextTick = ae.NextTick.Add(effectTick)
	if caster := w.Players[ae.Caster]; caster != nil && !npc.Dead {
		w.spellHitLocked(caster, w.findSessionByEntityIDLocked(caster.ServerID), npc, -ae.HPTick)
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
		for _, ae := range append([]*ActiveEffect(nil), npc.Effects...) {
			w.tickNPCEffectLocked(npc, ae, now)
			if npc.Dead {
				break
			}
		}
	}
}

// clearSpellsLocked ends a dead player's cast and takes its effects off.
func (w *World) clearSpellsLocked(p *Player) {
	p.Casting = nil
	for len(p.Effects) > 0 {
		w.removeEffectLocked(p, p.Effects[0].SpellID)
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
// in ms (zigzag), 0 when it is over.
func sendCooldown(sess *session.Session, displayIndex int, lockout time.Duration) {
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
