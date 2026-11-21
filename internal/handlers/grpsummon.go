package handlers

// grpsummon.go — /grpsummon: every other member of the caller's group, or of its
// raid, brought to the caller (the owner, 2026-11-21: "/grpsummon new GM command
// summons whole raid or group that they are in. also give [a player] the ability
// to his account").
//
// Who may: a GM (acctlevel 1 or more, as every # command), a sub GM (a tester's
// row marked subgm: devcmds.go's isSubGM; the owner, 2026-11-21: "sub gms can
// /grpsummon"), or an account of world.grpsummon_accounts (SetGrpSummon; usernames in any case, the username
// read on each use as /noclip's). Anyone else is answered as an unknown command,
// and a PS2's "/t grpsummon" stays a tell. Spelled /grpsummon, #grpsummon or, from
// a PS2 (which forwards only tells), "/t grpsummon".
//
// Each member goes as #summon moves one player (teleportTo: the world the
// visibility filters on, the place, the client's 0x07F6 and the world load's
// hold), round the caller as a group gate lands its members (GateRingSpot), so
// they do not stand in one another. A pet or a mercenary comes along after its
// owner as through any teleport (entity/pet.go, merc.go). A member is left where
// it is, and named to the caller with why, when it is:
//   - not in the world (logged out, or between characters);
//   - loading a world;
//   - dead;
//   - in a duel accepted (its countdown or its fight: World.Dueling);
//   - in an arena match (capture the flag, the siege: CTFInMatch);
//   - a Hardcore character, when the caller is no GM (as no portal carries one:
//     hcrules.go; a GM's #summon moves anyone);
//   - of a game that cannot load the caller's world (LoadableWorlds).
//
// The caller is refused while dead or in an arena match. An account that is no
// GM's (a sub GM's too) waits world.grpsummon_cooldown (unset: 60 s) between two that brought
// anyone; a GM never waits. The words are ours.

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// grpSummonDefaultWait is a listed account's wait when world.grpsummon_cooldown
// is unset.
const grpSummonDefaultWait = 60 * time.Second

// What /grpsummon answers.
const (
	grpSummonNoGroup = "You are in no group."
	grpSummonDead    = "You cannot summon while you are dead."
	grpSummonArena   = "You cannot summon into an arena match."
	grpSummonWait    = "Group summon is ready again in %d s."
	grpSummonCame    = "Summoned your %s: %s."
	grpSummonNoneCam = "No one of your %s came."
	grpSummonLeft    = "Not summoned: %s."
	grpSummonYou     = "%s summons the %s to them."
	// grpSummonHelp is /help's line to a sub GM or a listed account
	grpSummonHelp = "  /grpsummon - brings every other member of your group or raid to you (on a PS2: /t grpsummon)"
)

var grpSummon struct {
	mu    sync.Mutex
	names map[string]bool
	wait  time.Duration
	last  map[int]time.Time // by account id: its last summon that brought anyone
}

// SetGrpSummon sets world.grpsummon_accounts (usernames, any case) and
// world.grpsummon_cooldown in seconds (below 0: the default, 60 s; 0: none).
func SetGrpSummon(names []string, waitSeconds int) {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		if n = strings.ToLower(strings.TrimSpace(n)); n != "" {
			m[n] = true
		}
	}
	wait := grpSummonDefaultWait
	if waitSeconds >= 0 {
		wait = time.Duration(waitSeconds) * time.Second
	}
	grpSummon.mu.Lock()
	grpSummon.names, grpSummon.wait, grpSummon.last = m, wait, map[int]time.Time{}
	grpSummon.mu.Unlock()
	if len(m) > 0 {
		log.Printf("GrpSummon: %d accounts may /grpsummon though no GM's (world.grpsummon_accounts), every %s", len(m), wait)
	}
}

// grpSummonAccount reports whether the account of that username is listed.
func grpSummonAccount(username string) bool {
	grpSummon.mu.Lock()
	defer grpSummon.mu.Unlock()
	return grpSummon.names[strings.ToLower(strings.TrimSpace(username))]
}

// grpSummonLine reports a line that is /grpsummon: "/grpsummon" or
// "#grpsummon" (any case), or a PS2's "/t grpsummon" with nothing after it.
func grpSummonLine(message string) bool {
	f := strings.Fields(message)
	switch {
	case len(f) == 1:
		w := strings.ToLower(f[0])
		return w == "/grpsummon" || w == "#grpsummon"
	case len(f) == 2:
		w := strings.ToLower(f[0])
		return (w == "/t" || w == "/tell") && strings.EqualFold(f[1], "grpsummon")
	}
	return false
}

// mayGrpSummon reports whether sess's account may /grpsummon: a GM's, a sub
// GM's, or one of world.grpsummon_accounts. gm says a GM's (no wait).
func (h *ChatHandler) mayGrpSummon(sess *session.Session) (may, gm bool) {
	if h.isGM(sess) {
		return true, true
	}
	if h.isSubGM(sess) {
		return true, false
	}
	u, ok := h.access.(usernames)
	if !ok || sess == nil || sess.AccountID() == 0 {
		return false, false
	}
	name, err := u.Username(sess.AccountID())
	return err == nil && grpSummonAccount(name), false
}

// grpSummonCommand is /grpsummon from one who may (mayGrpSummon): the file's
// rules.
func (h *ChatHandler) grpSummonCommand(sess *session.Session, player *entity.Player, gm bool, now time.Time) {
	reply := func(s string) { SendClientMessage(sess, s) }
	if gm {
		reply = func(s string) { h.gmReply(sess, s) }
	}
	gw := h.gameWorld
	if snap, ok := gw.PlayerSnapshot(player.ServerID); !ok || snap.IsDead() {
		reply(grpSummonDead)
		return
	}
	if gw.CTFInMatch(sess) {
		reply(grpSummonArena)
		return
	}
	members := gw.GroupMembers(player)
	if len(members) < 2 {
		reply(grpSummonNoGroup)
		return
	}
	if !gm {
		grpSummon.mu.Lock()
		last, wait := grpSummon.last[sess.AccountID()], grpSummon.wait
		grpSummon.mu.Unlock()
		if left := wait - now.Sub(last); !last.IsZero() && left > 0 {
			reply(fmt.Sprintf(grpSummonWait, int((left+time.Second-1)/time.Second)))
			return
		}
	}
	kind := "group"
	if gw.GroupIsRaid(player) {
		kind = "raid"
	}
	at := entity.GateSpot{World: player.World, X: player.X, Y: player.Y, Z: player.Z, Place: player.Name}
	var came, left []string
	ring := 0
	for _, m := range members {
		if m == player {
			continue
		}
		ts := gw.FindSessionByEntityID(m.ServerID)
		why := ""
		switch snap, ok := gw.PlayerSnapshot(m.ServerID); {
		case ts == nil || !ok:
			why = "not in the world"
		case ts.StateChannelsHeld():
			why = "loading"
		case snap.IsDead():
			why = "dead"
		case gw.Dueling(m):
			why = "in a duel"
		case gw.CTFInMatch(ts):
			why = "in an arena match"
		case !gm && m.HardcoreBound():
			why = "Hardcore"
		case !CanLoadWorld(ts, player.World):
			why = "their game cannot load " + worldName(player.World)
		}
		if why != "" {
			left = append(left, fmt.Sprintf("%s (%s)", m.Name, why))
			continue
		}
		ring++
		to := gw.GateRingSpot(at, ring)
		if !h.teleportTo(sess, ts, m, to.X, to.Y, to.Z, m.Facing, to.World) {
			left = append(left, fmt.Sprintf("%s (%s)", m.Name, "their game cannot load "+worldName(player.World)))
			continue
		}
		SendClientMessage(ts, fmt.Sprintf(grpSummonYou, player.Name, kind))
		came = append(came, m.Name)
	}
	if len(came) > 0 {
		reply(fmt.Sprintf(grpSummonCame, kind, strings.Join(came, ", ")))
		if !gm {
			grpSummon.mu.Lock()
			if grpSummon.last == nil {
				grpSummon.last = map[int]time.Time{}
			}
			grpSummon.last[sess.AccountID()] = now
			grpSummon.mu.Unlock()
		}
	} else {
		reply(fmt.Sprintf(grpSummonNoneCam, kind))
	}
	if len(left) > 0 {
		reply(fmt.Sprintf(grpSummonLeft, strings.Join(left, ", ")))
	}
	log.Printf("GrpSummon: %s (gm %v) summons its %s to (%.1f, %.1f, %.1f) world %d: came %v, left %v",
		player.Name, gm, kind, player.X, player.Y, player.Z, player.World, came, left)
}
