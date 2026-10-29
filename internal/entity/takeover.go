package entity

import (
	"fmt"
	"strings"
	"time"

	"github.com/eqoa/server/internal/session"
)

// takeover.go — a character still in the world taken over by a new login of
// its own account (jib's F3 20261029-212855-51f2bfaa: "if they get booted to
// server select, pick the character it waits. They should be able to just
// take over that existing character ... retaining any and all debuffs / buffs
// / agro from session to session to avoid abuse").
//
// A client that drops leaves its character in the world: nothing tells the
// server (a page's relay socket simply goes quiet), and the session lives on
// until its pings go unanswered, 50 of world.session_timeout_game. Until
// 2026-10-29 character select refused the character to any other window for
// session.LiveWithin (60 s) after the dropped client's last packet (CLIENT's
// 0x07F0, "That character is in an area of the world that is currently
// unavailable. Please try again later."), then REPLACED it: a Player built
// anew from the row, its hit points and power filled by the login, its
// lockouts, breath, a death on its way and a GM's freeze gone, its pet left
// without an owner, the rest saved and read back.
//
// Now the new session takes the live Player itself. The same struct stays in
// w.Players under the same id, so everything the world keeps on it, or by its
// id, goes on as it was: hit points, power, every effect with its own end and
// next tick, the lockouts, a cast on its way, the NPCs' threat on it and the
// fights they are in with it, its group, its pet (whose owner is this very
// Player), factions, masteries, a death and its respawn's time, a GM's
// freeze. Nothing is saved or read back, so nothing can be dropped or doubled.
// The sessions that had it are told as a replaced login is (0x07A4) and let
// go of it first, so their end saves and removes nothing. What the Player held
// of its OLD client goes (handedOverLocked), and the new one is told what its
// MemoryDump does not carry (SendTakenOver).

// Takeover is what a character carried into the session that took it over,
// for the login's log line and the tests.
type Takeover struct {
	From      int // the sessions it was taken from (0: none had it: a character its session lost)
	Told      int // of those, told they were logged in elsewhere (0x07A4)
	HP, MaxHP int
	Power     int
	MaxPower  int
	Effects   []string // each effect and what is left of it
	Lockouts  int
	Casting   bool
	Fighting  int // NPCs with a threat on it
	Group     uint32
	Pet       string
	Dead      bool
	X, Y, Z   float32
	World     uint8
}

// String is the takeover in a log line.
func (t Takeover) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d/%d HP, %d/%d power at (%.1f, %.1f, %.1f) world %d", t.HP, t.MaxHP, t.Power, t.MaxPower, t.X, t.Y, t.Z, t.World)
	if t.Dead {
		b.WriteString(", dead")
	}
	fmt.Fprintf(&b, "; %d effects", len(t.Effects))
	if len(t.Effects) > 0 {
		fmt.Fprintf(&b, " (%s)", strings.Join(t.Effects, ", "))
	}
	fmt.Fprintf(&b, ", %d lockouts", t.Lockouts)
	if t.Casting {
		b.WriteString(", a cast on its way")
	}
	fmt.Fprintf(&b, ", %d NPCs fighting it", t.Fighting)
	if t.Group != 0 {
		fmt.Fprintf(&b, ", group %d", t.Group)
	}
	if t.Pet != "" {
		fmt.Fprintf(&b, ", pet %q", t.Pet)
	}
	fmt.Fprintf(&b, "; from %d sessions (%d told)", t.From, t.Told)
	return b.String()
}

// TakeOverPlayer gives sess the character's Player still in the world
// (charServerID: its characters row), taken from every other session that
// had it, and says what it carried; nil when the character has no Player in
// the world, and the login builds one from its row. sess is the session
// entering the world with it; whose account the character is, and whether a
// live session of another account holds it, is the caller's to have checked.
func (w *World) TakeOverPlayer(sess *session.Session, charServerID uint32) (*Player, Takeover) {
	if sess == nil || charServerID == 0 {
		return nil, Takeover{}
	}
	now := time.Now()
	w.mu.Lock()
	p := w.playerByCharacterLocked(charServerID)
	if p == nil {
		w.mu.Unlock()
		return nil, Takeover{}
	}
	// the sessions that had it, each once (a session can be filed under two
	// keys: GetOrCreateSession's routing to a master session)
	var from []*session.Session
	if w.SessionManager != nil {
		seen := map[*session.Session]bool{sess: true}
		w.SessionManager.ForEach(func(s *session.Session) bool {
			if !seen[s] && (w.sessionPlayerLocked(s) == p || s.CharacterServerID() == charServerID) {
				seen[s] = true
				from = append(from, s)
			}
			return true
		})
	}
	// let go of it before they are ended: their cleanup then finds no player
	// to save or take out of the world (RemovePlayerBySession)
	for _, s := range from {
		if w.sessionPlayerLocked(s) == p {
			s.SetECSEntity(nil)
		}
	}
	sess.SetCharacterServerID(charServerID)
	sess.SetECSEntity(p)
	w.handedOverLocked(p)
	t := w.takeoverOfLocked(p, now)
	t.From = len(from)
	w.mu.Unlock()

	// told as a replaced login is: 0x07A4, kept ReplacedGrace to hear it
	if w.SessionManager != nil {
		for _, s := range from {
			if w.SessionManager.ReplaceSession(s) {
				t.Told++
			}
		}
	}
	return p, t
}

// handedOverLocked forgets what p held of the client it had: the ids it named
// things by (each session numbers the world its own way: its target), the
// attack button it held and the swing it drove (its NPCs' fights with it go
// on), its velocity (the dump stands it still), the menus and questions open
// on that client, and what that client asked for (power bars, voice, the
// profanity filter off: on at every login), so the new client asks again. Its
// guild, its group's roster and its pet are told to the new session as at a
// login. Must be called with the lock held.
func (w *World) handedOverLocked(p *Player) {
	p.TargetObjectID = 0
	p.AutoAttack = false
	p.disengage()
	p.VelX, p.VelY, p.VelZ = 0, 0, 0
	p.PendingCoachDests, p.CoachPage, p.PendingYesNo = nil, 0, false
	p.PendingHDDMenu = 0
	p.PendingBind = Bind{}
	p.PendingSpar = SparPick{}
	p.ProfanityFilterOff = false
	p.PowerBars, p.powerBarsSent, p.powerBarsDue = false, "", false
	w.voiceGoneLocked(p)
	p.StatsDirty = true
	delete(w.announced, p) // guildAnnounceSystem tells the new client its guild
	if g := w.groups[p.GroupID]; g != nil && p.GroupID != 0 {
		delete(g.lastRoster, p.ServerID) // and its group's roster
	}
	if ps := w.petStateLocked(p); ps != nil {
		ps.told = 0 // and its pet (0x007E), as after any dump
	}
}

// takeoverOfLocked is what p carries now. Must be called with the lock held.
func (w *World) takeoverOfLocked(p *Player, now time.Time) Takeover {
	t := Takeover{HP: p.HP, MaxHP: p.MaxHPNow(), Power: p.Power, MaxPower: p.MaxPowerNow(), Casting: p.Casting != nil,
		Group: p.GroupID, Dead: p.respawnPending() || p.IsDead(), X: p.X, Y: p.Y, Z: p.Z, World: p.World}
	for _, e := range p.Effects {
		if left := e.Until.Sub(now); left > 0 {
			t.Effects = append(t.Effects, fmt.Sprintf("%s %s", effectLabel(e), left.Round(time.Second)))
		}
	}
	for _, l := range p.Lockouts {
		if now.Before(l.until) {
			t.Lockouts++
		}
	}
	for _, npc := range w.NPCs {
		if _, ok := npc.Threats[p.ServerID]; ok || npc.CurrentEnemy == p.ServerID {
			t.Fighting++
		}
	}
	if pet := w.petOfLocked(p); pet != nil {
		t.Pet = pet.Name
	}
	return t
}

func effectLabel(e *ActiveEffect) string {
	if e.Name != "" {
		return e.Name
	}
	return fmt.Sprintf("spell %d", e.SpellID)
}

// SendTakenOver tells sess's client, after the MemoryDump of a character it
// took over, what the dump does not carry and the old client was shown: each
// ability's lockout still running (0x00CD with what is left of it), so its
// badge is on the new client's ability bar as it was on the old one's (the
// server holds the lockout either way). It returns how many it sent.
func (w *World) SendTakenOver(sess *session.Session, now time.Time) int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return 0
	}
	n := 0
	for _, l := range p.Lockouts {
		if left := l.until.Sub(now); left > 0 {
			sendCooldown(sess, l.displayIndex, left)
			n++
		}
	}
	return n
}
