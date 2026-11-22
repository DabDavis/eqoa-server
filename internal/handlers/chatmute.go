package handlers

import (
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// chatmute.go — a GM's chat mutes by the hour, per account (a GM's ask, board
// entry 1656: "/mute playername 26 should shout chat mute for 1 day [2] hours.
// /mute playername .5 should mute for 30 minutes. /muteall playername 4 should
// mute them from all chat channels, not just /shout. This should apply to all
// characters on the account").
//
//   - #mute <name> <hours>: the account's shouts stop for so long;
//     #muteall <name> <hours>: its every chat line (what gmChatGate passes,
//     the Hardcore channel too). 0 lifts that one; no hours says what holds;
//     #unmute <name> lifts both.
//   - The name is any character's, in the world or not. A GM cannot mute an
//     account at or above its own level.
//   - Kept in table chatmutes (database/chatmutes.go), one row per account and
//     scope, each with its own end: held in memory and read again once
//     chatMuteReread old, so a panel edit counts within that and a chat line
//     never waits on the database. A keep restart or a cold start reads the
//     table again: a mute outlives both.
//   - Players are told when a mute is put on or lifted, at each world entry
//     while it holds, and what is left at each line it stops. The words are
//     ours; each shard keeps its own mutes.

// chatMuteReread is how old the mutes held in memory may be.
const chatMuteReread = 30 * time.Second

// chatMuteMostHours is the longest mute: a year.
const chatMuteMostHours = 8760

// chatMuteUsage is the commands' usage line.
const chatMuteUsage = "Usage: #mute <name> <hours> (shouts) | #muteall <name> <hours> (all chat) | #unmute <name>; .5 is 30 minutes, 0 lifts, no hours: its mutes"

// chatMuteStore keeps the mutes (database.SQLiteDB).
type chatMuteStore interface {
	ChatMutes() ([]database.ChatMute, error)
	SetChatMute(m database.ChatMute) error
	ClearChatMutes(accountID int, scope string) error
}

// chatMuteEnd is one mute: its end and its GM's character.
type chatMuteEnd struct {
	until time.Time
	gm    string
}

// chatMuteAcct is an account's two mutes, apart.
type chatMuteAcct struct{ shout, all chatMuteEnd }

// chatMutes is the mutes held in memory.
type chatMutes struct {
	mu     sync.Mutex
	store  chatMuteStore
	byAcct map[int]chatMuteAcct
	now    func() time.Time // nil: time.Now
	read   time.Time        // when the store was last read
	gen    int              // counts the changes made here
}

func (c *chatMutes) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

// SetChatMuteStore gives the mutes their table. Without it a mute lasts until
// the process ends.
func (h *ChatHandler) SetChatMuteStore(s chatMuteStore) {
	h.mutes.mu.Lock()
	defer h.mutes.mu.Unlock()
	h.mutes.store, h.mutes.read = s, time.Time{}
}

// of is the account's mutes, the store read again when the last read is
// chatMuteReread old (with no lock held; taken only if nothing changed here
// meanwhile, else read again next time).
func (c *chatMutes) of(acct int) chatMuteAcct {
	c.mu.Lock()
	now := c.clock()
	if c.store != nil && (c.read.IsZero() || now.Sub(c.read) >= chatMuteReread) {
		c.read = now
		gen, store := c.gen, c.store
		c.mu.Unlock()
		rows, err := store.ChatMutes()
		c.mu.Lock()
		switch {
		case err != nil:
			log.Printf("ChatMutes: not read (%v); the ones held stand", err)
		case gen != c.gen:
			c.read = time.Time{}
		default:
			m := make(map[int]chatMuteAcct, len(rows))
			for _, r := range rows {
				a := m[r.AccountID]
				e := chatMuteEnd{until: r.Until, gm: r.GM}
				if r.Scope == database.ChatMuteShout {
					a.shout = e
				} else {
					a.all = e
				}
				m[r.AccountID] = a
			}
			c.byAcct = m
		}
	}
	a := c.byAcct[acct]
	c.mu.Unlock()
	return a
}

// set keeps the account's mute of the scope until then (zero: lifted; scope
// "": both lifted), the store first: on its error nothing changes.
func (c *chatMutes) set(acct int, scope string, until time.Time, gm string) error {
	c.mu.Lock()
	store := c.store
	now := c.clock()
	c.mu.Unlock()
	if store != nil {
		var err error
		if until.IsZero() {
			err = store.ClearChatMutes(acct, scope)
		} else {
			err = store.SetChatMute(database.ChatMute{AccountID: acct, Scope: scope, Until: until.UTC().Truncate(time.Second),
				GM: gm, Since: now.UTC().Truncate(time.Second)})
		}
		if err != nil {
			return err
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.byAcct == nil {
		c.byAcct = map[int]chatMuteAcct{}
	}
	a := c.byAcct[acct]
	e := chatMuteEnd{until: until, gm: gm}
	switch scope {
	case database.ChatMuteShout:
		a.shout = e
	case database.ChatMuteAll:
		a.all = e
	default:
		a = chatMuteAcct{}
	}
	c.byAcct[acct] = a
	c.gen++
	return nil
}

// muteSpan is d as the players read it: rounded to the minute, then days,
// hours and minutes, each only when not 0; under 30 s "less than a minute".
func muteSpan(d time.Duration) string {
	if d < 30*time.Second {
		return "less than a minute"
	}
	m := int((d + 30*time.Second) / time.Minute)
	days, hours, mins := m/(24*60), m/60%24, m%60
	var parts []string
	plural := func(n int, one string) {
		if n == 1 {
			parts = append(parts, "1 "+one)
		} else if n > 1 {
			parts = append(parts, strconv.Itoa(n)+" "+one+"s")
		}
	}
	plural(days, "day")
	plural(hours, "hour")
	plural(mins, "minute")
	return strings.Join(parts, " ")
}

// parseMuteHours is a GM's hours: the span (0 lifts) or why it is refused.
func parseMuteHours(raw string) (time.Duration, string) {
	s := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), "h")
	h, err := strconv.ParseFloat(s, 64)
	switch {
	case err != nil || math.IsNaN(h) || math.IsInf(h, 0):
		return 0, fmt.Sprintf("%q is not a number of hours", raw)
	case h < 0:
		return 0, fmt.Sprintf("%q is below 0 hours", raw)
	case h > chatMuteMostHours:
		return 0, fmt.Sprintf("%q is past %d hours (a year)", raw, chatMuteMostHours)
	}
	secs := math.Round(h * 3600)
	if h > 0 && secs == 0 {
		return 0, fmt.Sprintf("%q is under a second", raw)
	}
	return time.Duration(secs) * time.Second, ""
}

// muteWhat is a scope as the lines say it.
func muteWhat(scope string) string {
	if scope == database.ChatMuteShout {
		return "shouting"
	}
	return "all chat"
}

// mutedLine is the line a muted account's chat line of the kind is stopped
// with, and whether it is stopped: a shout by either mute (the shout mute's
// line when it ends later), any other line by the all mute alone.
func (h *ChatHandler) mutedLine(sess *session.Session, kind string) (string, bool) {
	acct := sess.AccountID()
	if acct == 0 {
		return "", false
	}
	a := h.mutes.of(acct)
	now := h.mutes.clock()
	shout, all := now.Before(a.shout.until), now.Before(a.all.until)
	if kind == "shout" && shout && (!all || a.shout.until.After(a.all.until)) {
		return "You are muted from shouting for another " + muteSpan(a.shout.until.Sub(now)) + ".", true
	}
	if all {
		return "You are muted from all chat for another " + muteSpan(a.all.until.Sub(now)) + ".", true
	}
	return "", false
}

// muteHolds is the lines saying each mute of the account that holds.
func (h *ChatHandler) muteHolds(acct int, prefix string) []string {
	if acct == 0 {
		return nil
	}
	a := h.mutes.of(acct)
	now := h.mutes.clock()
	var out []string
	if now.Before(a.shout.until) {
		out = append(out, prefix+"muted from shouting for another "+muteSpan(a.shout.until.Sub(now))+".")
	}
	if now.Before(a.all.until) {
		out = append(out, prefix+"muted from all chat for another "+muteSpan(a.all.until.Sub(now))+".")
	}
	return out
}

// muteEntered is the world entry's hook: a line for each mute that holds.
func (h *ChatHandler) muteEntered(sess *session.Session, _ *entity.Player) {
	for _, l := range h.muteHolds(sess.AccountID(), "You are ") {
		SendErrorMessage(sess, l)
	}
}

// muteTellAccount sends line to every session of the account in the world.
func (h *ChatHandler) muteTellAccount(acct int, line string) {
	if h.sessionManager == nil {
		return
	}
	h.sessionManager.ForEach(func(s *session.Session) bool {
		if s.AccountID() == acct && s.CharacterInWorld() {
			SendErrorMessage(s, line)
		}
		return true
	})
}

// muteSubject is the account a GM's name names: a character in the world, else
// a character's row. ok false: none.
func (h *ChatHandler) muteSubject(name string) (acct int, shown string, ok bool) {
	if h.gameWorld != nil {
		if p := h.gameWorld.PlayerByName(name); p != nil && h.gameWorld.IsLivePlayer(p) {
			if s := h.gameWorld.FindSessionByEntityID(p.ServerID); s != nil && s.AccountID() != 0 {
				return s.AccountID(), p.Name, true
			}
		}
	}
	if h.resets != nil {
		if rec, err := h.resets.GetByName(name); err == nil && rec != nil && rec.AccountID != 0 {
			return rec.AccountID, rec.CharName, true
		}
	}
	return 0, "", false
}

// muteCommand is #mute, #muteall and #unmute (the file's doc).
func (h *ChatHandler) muteCommand(sess *session.Session, gm *entity.Player, cmd string, args []string) {
	if (cmd == "#unmute" && len(args) != 1) || (cmd != "#unmute" && (len(args) < 1 || len(args) > 2)) {
		h.gmReply(sess, chatMuteUsage)
		return
	}
	acct, name, ok := h.muteSubject(args[0])
	if !ok {
		h.gmReply(sess, fmt.Sprintf("Mute: no character named %q", args[0]))
		return
	}
	if cmd != "#unmute" && len(args) == 1 { // what holds
		lines := h.muteHolds(acct, name+"'s account is ")
		if len(lines) == 0 {
			lines = []string{name + "'s account is not muted."}
		}
		for _, l := range append(lines, chatMuteUsage) {
			h.gmReply(sess, l)
		}
		return
	}
	scope := database.ChatMuteShout
	if cmd == "#muteall" {
		scope = database.ChatMuteAll
	}
	var span time.Duration
	if cmd != "#unmute" {
		var why string
		if span, why = parseMuteHours(args[1]); why != "" {
			h.gmReply(sess, "Mute: "+why+". "+chatMuteUsage)
			return
		}
	}
	// only an account below the GM's own level
	if h.access == nil {
		h.gmReply(sess, "Mute: your account's level could not be read just now: try again.")
		return
	}
	mine, err := h.access.AccessLevel(sess.AccountID())
	if err != nil {
		h.gmReply(sess, "Mute: your account's level could not be read just now: try again.")
		return
	}
	theirs, err := h.access.AccessLevel(acct)
	if err != nil {
		h.gmReply(sess, fmt.Sprintf("Mute: %s's account's level could not be read just now: try again.", name))
		return
	}
	if theirs >= mine {
		h.gmReply(sess, fmt.Sprintf("Mute: %s's account is at or above your level: you cannot mute it.", name))
		return
	}
	var until time.Time
	if cmd == "#unmute" {
		scope = ""
	} else if span > 0 {
		until = h.mutes.clock().Add(span)
	}
	if err := h.mutes.set(acct, scope, until, gm.Name); err != nil {
		log.Printf("GM: %s's mute of %s's account %d not kept: %v", gm.Name, name, acct, err)
		h.gmReply(sess, fmt.Sprintf("Mute: %s's account's mute could not be kept just now: try again.", name))
		return
	}
	switch {
	case cmd == "#unmute":
		log.Printf("GM: %s unmuted %s's account %d", gm.Name, name, acct)
		h.muteTellAccount(acct, "A GM has lifted your chat mutes: you may chat again.")
		h.gmReply(sess, name+"'s account may chat again: its mutes are lifted.")
	case until.IsZero():
		log.Printf("GM: %s lifted %s's account %d mute from %s", gm.Name, name, acct, muteWhat(scope))
		h.muteTellAccount(acct, "A GM has lifted your mute from "+muteWhat(scope)+".")
		h.gmReply(sess, name+"'s account is no longer muted from "+muteWhat(scope)+".")
	default:
		log.Printf("GM: %s muted %s's account %d from %s for %s (until %s)", gm.Name, name, acct, muteWhat(scope),
			muteSpan(span), until.UTC().Format(time.RFC3339))
		h.muteTellAccount(acct, "A GM has muted you from "+muteWhat(scope)+" for "+muteSpan(span)+".")
		h.gmReply(sess, fmt.Sprintf("%s's account is muted from %s for %s, until %s UTC: every character of it.", name,
			muteWhat(scope), muteSpan(span), until.UTC().Format("2006-01-02 15:04")))
	}
}
