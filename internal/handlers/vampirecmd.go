package handlers

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// #vampire: who the sun burns by day (entity/vampire.go, table charvampire).
//
//	#vampire       what the sun does now to the GM's target (a player) or the
//	               GM: a vampire or not, the hour its client shows, a roof
//	               over it, and the settings
//	#vampire on    makes it a vampire; off takes that back
//
// A change is written to charvampire with its gm_audit line in one
// transaction (database.SetVampire), as the GM panel writes a row, so the
// panel lists and reverts it, and is in force where the character stands.

// vampireStore is what #vampire writes through (database.SQLiteDB).
type vampireStore interface {
	SetVampire(serverID uint32, on bool, who database.GMEdit, detail string) (int64, error)
}

// SetVampireStore gives #vampire its table. Without it nobody is made one.
func (h *ChatHandler) SetVampireStore(s vampireStore) { h.vampires = s }

func (h *ChatHandler) vampireCommand(sess *session.Session, args []string) {
	if h.gameWorld == nil {
		return
	}
	st, ok := h.gameWorld.SunOn(sess, time.Now())
	if !ok || st.ServerID == 0 {
		h.gmReply(sess, "#vampire: no character in the world")
		return
	}
	if len(args) == 0 {
		h.gmReply(sess, sunLine(st, h.gameWorld.VampireSun()))
		return
	}
	var on bool
	switch strings.ToLower(args[0]) {
	case "on":
		on = true
	case "off":
	default:
		h.gmReply(sess, "Usage: #vampire [on|off] (your target, a player, else you)")
		return
	}
	if h.vampires == nil {
		h.gmReply(sess, "#vampire: vampires are not kept here")
		return
	}
	who := database.GMEdit{AccountID: sess.AccountID(), Via: "game"}
	if a := sess.RemoteAddr(); a != nil {
		who.IP = a.IP.String()
	}
	if _, err := h.vampires.SetVampire(st.ServerID, on, who, "#vampire in the game"); err != nil {
		h.gmReply(sess, "#vampire: "+err.Error())
		return
	}
	h.gameWorld.SetVampire(st.ServerID, on)
	log.Printf("GM: account %d set %s a vampire: %v", sess.AccountID(), st.Name, on)
	if now, ok := h.gameWorld.SunOn(sess, time.Now()); ok {
		st = now
	}
	h.gmReply(sess, sunLine(st, h.gameWorld.VampireSun()))
}

// sunLine is #vampire's answer: what the sun does to the character now.
func sunLine(st entity.SunState, r entity.VampireSun) string {
	hh := int(st.Hour)
	mm := int((st.Hour - float32(hh)) * 60)
	when := fmt.Sprintf("%02d:%02d, night", hh, mm)
	if st.Day {
		when = fmt.Sprintf("%02d:%02d, day", hh, mm)
	}
	var what string
	switch {
	case !st.Vampire:
		what = "is no vampire"
	case r.Damage <= 0:
		what = "is a vampire, and the sun burns none (vampire_sun_damage 0)"
	case st.Spared:
		what = "is a vampire, spared as a GM's (vampire_sun_gms false)"
	case !st.Day:
		what = "is a vampire, safe until sunrise (06:00)"
	case st.Shaded:
		what = "is a vampire, in the shade: something is over it"
	case st.Burns:
		what = fmt.Sprintf("is a vampire under the open sky: the sun takes %d every %v", r.Damage, r.Interval)
	default:
		what = "is a vampire, dead or on its way back"
	}
	return fmt.Sprintf("%s %s (its client shows %s)", st.Name, what, when)
}
