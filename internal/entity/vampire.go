package entity

import (
	"fmt"
	"log"
	"time"

	"github.com/eqoa/server/internal/session"
)

// vampire.go — the sun burns a vampire by day. The user, 2026-10-30: on
// Sandstorm "a vampire character (Afkqq) loses 50 HP about every 6 seconds
// while it is daylight", and "yes please" to eqoaclassic's vampires doing the
// same; then "shade protecting vampire from dot would be good".
//
// What Sandstorm sends (spellfx24's afkqq-1 session, 2026-10-30 03:03 UTC, a
// level 43 dwarf cleric standing in the open at Tunaria (15290, 54, 9249) at
// the client's 10:18): 0x00DB {u32 0xFFFFFFFF, zigzag -50, u32 the player},
// from no one, from the login on, with no line, on the regeneration's tick
// after its heals: the stat channel went 1693 to 1680 (+37 regeneration, -50)
// in one update, and at full health stood at 1799 of 1849 from tick to tick,
// a heal over time's figure (0x00DB from the player) just before the -50.
// EQOA-Data's vampire guide says the live game's the same: "During daylight
// hours, a -50 DoT effect applies"; "If infected, you may notice a DoT (-50)
// during daylight".
//
// Here: every VampireSun.Interval (the regeneration's tick when it is 6 s,
// after the regeneration, as Sandstorm's), each vampire in the world (table
// charvampire, database/vampire.go: the vampire quest's bats, #vampire and the
// GM panel make one) that is alive, not loading a world, under the open sky
// and in daylight as its own client shows it takes VampireSun.Damage, told as
// Sandstorm tells it. It can kill: a death as any other (respawn.go: its debt,
// the respawn at its bind).
//
// Daylight is the client's own: the hour its clock shows (session/gameclock.go:
// the 0x0013 the server sent it, carried forward at the client's 983.04 ticks
// a second and blended as VISyncTime blends) with the sun over the horizon,
// Sunrise to Sunset. Clouds are no shade: where the terrain profile keeps a
// stormy sky (VIWorld::SetProfiles' sky type 2: the Zaoran Plateau's, the
// Lavastorm's) or the Plane of Sky's (profile 20) the client draws no sun by
// day, but it is day there all the same.
//
// Shade is anything over the vampire in the collision the game keeps
// (internal/ground's Roof): a house's ceiling or roof, a floor above, a
// bridge, the rock over a cave, a cavern or a city under the ground (Neriak's
// streets have the cavern's roof 27 over them; Moradhim's halls are roofed 35
// to 50 up and the rock is 55 up), and a big tree's crown, whose collision is
// a trunk and a crown (Tunaria's big trees: a trunk a few units across to
// about 8 up, a crown 20 to 40 across from about 13 up to 26-37), so a
// vampire standing under one is in its shade; a plant has no collision and is
// none. Measured over eqoaclassic's 23,922 spawns: of Tunaria's 19,711, 33.7%
// have something over them (the dungeons, the cities' houses, the caves); the
// open country's creatures almost never (grass snakes 5 of 119, rats 2 of
// 135, boars 0 of 50, each of them under a big tree's crown). The look up
// goes shadeReach far: a cave's roof can be 170 over its floor (the gnolls'
// cave round Tunaria (12900, 56, 12450)).

// VampireSun is how the sun treats a vampire (world.vampire_sun_*, the GM
// panel's settings): what it takes a tick (0: nothing), the tick, whether a
// roof shelters a vampire, and whether a GM's vampire burns too.
type VampireSun struct {
	Damage   int
	Interval time.Duration
	Shade    bool
	GMs      bool
}

// DefaultVampireSun is Sandstorm's 50 every 6 s, shade sheltering (the user's
// word) and a GM's vampire burning as anyone's (so a GM who makes one sees
// what a player sees).
var DefaultVampireSun = VampireSun{Damage: 50, Interval: RegenInterval, Shade: true, GMs: true}

// ticks is the interval in the world's 100 ms ticks, at least one.
func (r VampireSun) ticks() uint64 {
	return max(1, uint64(r.Interval/(100*time.Millisecond)))
}

func (r VampireSun) String() string {
	if r.Damage <= 0 {
		return "the sun burns no vampire (vampire_sun_damage 0)"
	}
	s := fmt.Sprintf("the sun takes %d off a vampire every %v from %02.0f:00 to %02.0f:00", r.Damage, r.Interval, Sunrise, Sunset)
	if r.Shade {
		s += ", but not under a roof"
	}
	if !r.GMs {
		s += "; a GM's vampire is spared"
	}
	return s
}

// Sunrise and Sunset are the hours the sun stands on the horizon. The sky
// turns it a fifteenth of a half turn an hour (VIAtmosphere::Update: the
// sun's sprite at (sin w, -cos w) x 35, w = hour x pi/12, the moon opposite)
// and its light comes the same way (VIScene::InitTimeOfDay: m_tod[h].m_b =
// (-sin, cos, 0), straight down at noon), so it is over the horizon after 6:00
// and before 18:00, and the stars are out only outside those hours
// (VIAtmosphere::ProcessTransitions). The player's lantern keeps its own
// hours (on before 6:00 and after 16:00: SetupTimeOfDayLights).
const (
	Sunrise float32 = 6
	Sunset  float32 = 18
)

// Daylight reports the sun over the horizon at the hour.
func Daylight(hour float32) bool { return hour > Sunrise && hour < Sunset }

// ClientHour is the time of day the session's client shows at now: the clock
// its last 0x0013 set (session.GameHour), or, with no session or none sent,
// the hour a 0x0013 sent now would show.
func ClientHour(sess *session.Session, now time.Time) float32 {
	if sess != nil {
		if h, ok := sess.GameHour(now); ok {
			return h
		}
	}
	return session.ServerHour(now)
}

// sunSource is who the sun's damage is from: no one, 0xFFFFFFFF, as
// Sandstorm sends it. CLIENT shows a 0x00DB whose attacker is not the player
// as damage taken, over the player (VIClient::DamageFromOther), as a
// creature's hit is.
const sunSource uint32 = 0xFFFFFFFF

// The look up for a roof: from shadeFrom over the vampire's feet (past the
// floor it stands on, under the lowest ceiling it could stand beneath; a
// player's place is its feet, Afkqq's 0.03 over the floor) to shadeReach.
// Over eqoaclassic's spawns nothing covered was covered only further up than
// 273.
const (
	shadeFrom  float32 = 1
	shadeReach float32 = 1000
)

// S2's world 7 ("secrets2", the vampires' Sanctuary on Sandstorm) is the
// Frontiers disc's world 5 file again (data3\Secrets.esf), whose ground the
// bake writes as world 5's.
const (
	secretsTwin  uint8 = 7
	secretsWorld uint8 = 5
)

// Roofs is what is over a place (internal/ground's Roof): whether anything
// stands over (x, z) between two heights. A Ground that is also Roofs gives
// a vampire its shade; one that is not shelters no one, and a vampire burns
// wherever it is day.
type Roofs interface {
	Roof(world uint8, x, y0, y1, z float32) bool
}

// shadedLocked reports something over the player (Roofs). Must be called
// with the lock held.
func (w *World) shadedLocked(p *Player) bool {
	roofs, ok := w.ground.(Roofs)
	if !ok {
		return false
	}
	world := p.World
	if world == secretsTwin {
		world = secretsWorld
	}
	return roofs.Roof(world, p.X, p.Y+shadeFrom, p.Y+shadeReach, p.Z)
}

// vampireSunSystem burns every vampire the sun reaches, every
// VampireSun.ticks (the file's doc). Must be called with the lock held,
// after regenSystem.
func (w *World) vampireSunSystem(now time.Time) {
	r := w.vampireSun
	if r.Damage <= 0 || w.TickCount%r.ticks() != 0 {
		return
	}
	for _, p := range w.Players {
		if !p.Vampire || p.IsDead() || p.respawnPending() || (p.GM && !r.GMs) {
			continue
		}
		var sess *session.Session
		if w.SessionManager != nil {
			// in the world (its client entered it) and not loading one
			if sess = w.findSessionByEntityIDLocked(p.ServerID); sess == nil || sess.StateChannelsHeld() {
				continue
			}
		}
		if !Daylight(ClientHour(sess, now)) || (r.Shade && w.shadedLocked(p)) {
			continue
		}
		w.sunBurnLocked(sess, p, r.Damage, now)
	}
}

// sunBurnLocked is the sun's damage on a vampire: its hit points, its figure
// (0x00DB from no one), and a death at 0. sess may be nil (no client to
// tell). Must be called with the lock held.
func (w *World) sunBurnLocked(sess *session.Session, p *Player, n int, now time.Time) {
	p.HP = max(0, p.HP-n)
	p.StatsDirty = true
	if sess != nil {
		w.sendDamageIndication(sess, sunSource, -n, sess.PlayerObjectID())
	}
	log.Printf("Sun: %s, a vampire in daylight, took %d (HP %d/%d)", p.Name, n, p.HP, p.MaxHPNow())
	if p.HP <= 0 {
		w.playerDiedLocked(sess, p, now)
		log.Printf("Sun: %s burned to death", p.Name)
	}
}

// SetVampireSun sets how the sun treats vampires (world.vampire_sun_*, the GM
// panel's settings), read at every tick.
func (w *World) SetVampireSun(r VampireSun) {
	if r.Interval <= 0 {
		r.Interval = DefaultVampireSun.Interval
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.vampireSun = r
}

// VampireSun is how the sun treats vampires now.
func (w *World) VampireSun() VampireSun {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.vampireSun
}

// SetVampire makes a character a vampire (or not) where it stands, every
// Player of it (a replacing login's old one may linger a moment); online is
// false when it is not in the world (its next login reads charvampire).
func (w *World) SetVampire(serverID uint32, on bool) (name string, online bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, p := range w.Players {
		if p.CharServerID == serverID {
			p.Vampire = on
			name, online = p.Name, true
		}
	}
	return name, online
}

// SetPlayerGM records whether the session's character's account is a GM's
// (acctlevel 1 or more, read at its login): VampireSun.GMs spares it or not.
func (w *World) SetPlayerGM(sess *session.Session, gm bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if p := w.sessionPlayerLocked(sess); p != nil {
		p.GM = gm
	}
}

// SunState is what the sun does to a character now (#vampire's answer).
type SunState struct {
	ServerID uint32
	Name     string
	Vampire  bool
	Hour     float32 // the time of day its client shows
	Day      bool
	Shaded   bool // something over it (asked only where it is day and shade shelters)
	Spared   bool // a GM, and the settings spare GMs
	Burns    bool // the sun's next tick takes its damage off it
}

// SunOn is what the sun does now to the GM's target (a player) or else to the
// GM itself; ok is false when the GM is not in the world.
func (w *World) SunOn(sess *session.Session, now time.Time) (s SunState, ok bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	gm := w.sessionPlayerLocked(sess)
	if gm == nil {
		return SunState{}, false
	}
	p, ps := gm, sess
	if gm.TargetObjectID != 0 && gm.TargetObjectID != NoAssist {
		if e, found := w.findEntityByObjectIDLocked(sess, gm.TargetObjectID); found {
			if t, isPlayer := e.(*Player); isPlayer && t != gm {
				p, ps = t, nil
				if w.SessionManager != nil {
					ps = w.findSessionByEntityIDLocked(t.ServerID)
				}
			}
		}
	}
	r := w.vampireSun
	s = SunState{ServerID: p.CharServerID, Name: p.Name, Vampire: p.Vampire, Hour: ClientHour(ps, now)}
	s.Day = Daylight(s.Hour)
	s.Spared = p.GM && !r.GMs
	if s.Day && r.Shade {
		s.Shaded = w.shadedLocked(p)
	}
	s.Burns = s.Vampire && s.Day && !s.Shaded && !s.Spared && r.Damage > 0 && !p.IsDead() && !p.respawnPending()
	return s, true
}

// The vampires' abilities (tools/seed_vampires.py makes their records, their
// lines and the blood they cost; EQOA-Data's vampire guide gives the
// figures, Sandstorm's own records Feed's and Sanctuary's exactly). What
// they need of the World beyond any spell's:
//   - Units of Blood: a line's "Reagent Unit of Blood xN" (spelltext.go) is
//     taken from the caster's inventory as the cast begins, and the cast is
//     refused without them (handlers' cast); Feed's "Summon Unit of Blood"
//     gives one as it lands, as any summoning of an item (potion.go).
//   - a stat set, not raised ("Set STR to 500": Bat Form; "Set STR to 0":
//     Mist Form): the difference from the bearer's own stat as it lands
//     (setStatsLocked), held to the stat's cap as any STR is (400 from
//     level 46, more with the masteries that raise it).
//   - damage per hit ("Increase Damage per Hit by 500": Frenzy): added to
//     every melee hit the bearer lands, a duel's too (offence.go's
//     playerMeleeHit).
//   - the gaze ("Set Aggro to 0": Vampiric Gaze): the creature forgets its
//     hate for the caster (forgetLocked).
//   - Vampire Form shows the fledgling vampires' body, Bat Form the bats'
//     (illusion.go, pet.go's petBodyAliases); Mist Form has no body of its
//     own on this server (the guide's red mist) and lands as its stats.
//   - "Prevents invis/sow pots" (the guide): a vampire may not drink a potion
//     that makes it invisible or quickens it (VampirePotionRefusal).

// setStatsLocked puts each stat the spell sets ("Set STR to 500") on ae as
// the difference from its bearer's own stat as it lands (base, gear and
// masteries: RecalcStats), so what other effects add still counts and the
// stat's cap holds it as any.
func setStatsLocked(d *SpellDef, t *Player, ae *ActiveEffect) {
	for _, e := range d.Effects {
		if e.Kind == EffectStat && e.Set {
			ae.Mods.add(e.Stat, e.Amount-t.ownStat(e.Stat))
		}
	}
}

// ownStat is a stat as the player's own (base, gear and masteries), without
// what effects add.
func (p *Player) ownStat(st Stat) int {
	switch st {
	case StatStr:
		return p.Str
	case StatSta:
		return p.Sta
	case StatAgi:
		return p.Agi
	case StatDex:
		return p.Dex
	case StatWis:
		return p.Wis
	case StatInt:
		return p.Int
	case StatCha:
		return p.Cha
	case StatAC:
		return p.AC
	case StatFR:
		return p.FireR
	case StatCR:
		return p.ColdR
	case StatPR:
		return p.PoisonR
	case StatDR:
		return p.DiseaseR
	case StatLR:
		return p.LightningR
	case StatAR:
		return p.ArcaneR
	}
	return 0
}

// forgets is whether the spell takes its target's hate for the caster away.
func (d *SpellDef) forgets() bool {
	for _, e := range d.Effects {
		if e.Kind == EffectForget {
			return true
		}
	}
	return false
}

// forgetLocked is a creature forgetting its hate for the caster (Vampiric
// Gaze: "Sets aggro with target to 0"): off its threat list, and when it was
// its enemy it turns to the next it hates, or to none (and goes home, as any
// creature left with no enemy does: chase.go). The caster's pet stays hated.
func (w *World) forgetLocked(npc *NPC, caster *Player) {
	delete(npc.Threats, caster.ServerID)
	if npc.CurrentEnemy != caster.ServerID {
		return
	}
	npc.CurrentEnemy = 0
	var top int64
	for id, t := range npc.Threats {
		if t > top {
			top, npc.CurrentEnemy = t, id
		}
	}
	log.Printf("Cast: %s forgets %s (now on %d)", npc.Name, caster.Name, npc.CurrentEnemy)
}

// vampirePotion is a vampire's refusal of a potion that would make it
// invisible or quicken it. The words are ours: no recording has them.
const vampirePotion = "The blood in your veins rejects the potion."

// VampirePotionRefusal is the line refusing the session's character the
// potion, "" when it may drink it: a vampire may not drink one that makes
// it invisible or quickens it (EQOA-Data's vampire guide: "Prevents
// invis/sow pots").
func (w *World) VampirePotionRefusal(sess *session.Session, pe PotionEffect) string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || !p.Vampire {
		return ""
	}
	for _, e := range pe.Def.Effects {
		if e.Kind == EffectInvis || e.Kind == EffectSpeed {
			return vampirePotion
		}
	}
	return ""
}

// VampireOf is whether the session's character is a vampire.
func (w *World) VampireOf(sess *session.Session) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	return p != nil && p.Vampire
}
