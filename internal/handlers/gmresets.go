package handlers

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The live CLIENT's GM commands (SOE's eqoa-live-1500, the Corsten state):
// /rst_mastr, /rst_both, /rem_abil, /banktunar and /servdown in its ChatLine
// (0x012C88D0), each doing something only when the GM's channel is on
// (VIClientPart382E8::IsEnabled), sent as 0x0701 sub-op 57, 58 and 59 (the rest
// of the line after one space, an 8-bit string), 56 (a zigzag atoi of it,
// nothing for 0) and 101 (nothing). Every client has them as # commands (a
// Frontiers disc's PS2 has no such slash commands; vanilla forwards /name),
// all run here:
//
//	#rst_mastr [name]           every mastery of the named character (else
//	                            the target player, else the GM) taken and what
//	                            it cost given back, as #cmreset (0x1402 sub-op
//	                            6 a rank, then 3; the live word 1885 "Your
//	                            Mastery points have been reset" to the player)
//	#rst_both [name]            that and every training point back: the base
//	                            stats their race's and class's, 0x001D with
//	                            old - new (CLIENT's Msg001D: m_p40 -= z,
//	                            silently), the stats sent again, the live word
//	                            1886 "Your Training points have been reset"
//	#rem_abil <name> <ability>  an ability (its spell id or name) out of the
//	                            named character's book, and out of its client's
//	                            at once (0x00B8)
//	#banktunar [name] <n>       n tunar added to the bank (negative: taken
//	                            away) of the named character, else the target
//	                            player, else the GM; the live client sends
//	                            nothing for 0, so its number is what is added
//	#servdown                   the server down and up again, as the GM
//	                            panel's restart (everyone told, saved, sent to
//	                            the login): an admin's (gm.admin_level), and the
//	                            second within serverDownWindow does it
//
// A named character in the world is changed where it stands and told; one
// out of it on its rows, there at its next login. What SOE's server did with
// each is not recorded: these are this server's (the text a name, which the
// live client's own words leave open, is inferred).

// TrainingResetText is what a character whose training points were reset is
// told: the live CLIENT's word 1886, which no code of that client shows (its
// server sent it as a line).
const TrainingResetText = "Your Training points have been reset"

// resetStore is what the resets and #banktunar read and keep of a character
// (database.SQLiteCharacterStore).
type resetStore interface {
	GetByName(name string) (*database.CharacterRecord, error)
	GetByServerID(serverID uint32) (*database.CharacterRecord, error)
	ResetTraining(serverID uint32) (database.TrainingReset, error)
	TakeTraining(serverID uint32, r database.TrainingReset) error
	AddBankTunar(serverID uint32, n int) (int, error)
}

// SetResetStore gives the resets and #banktunar the characters' rows. Without
// it they act on characters in the world alone, and keep nothing of the
// training or the bank.
func (h *ChatHandler) SetResetStore(s resetStore) {
	h.resets = s
}

// gmSubject is a character a GM's command names: in the world (its session
// and player) or its row alone.
type gmSubject struct {
	name     string
	serverID uint32
	sess     *session.Session // nil: not in the world
	p        *entity.Player
}

// gmCharacter is the character name names, in the world or not; no name is
// the GM's target player, else the GM. Said to the GM when there is none.
func (h *ChatHandler) gmCharacter(sess *session.Session, gm *entity.Player, name, what string) (gmSubject, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		t, ts, ok := h.gmPlayerTarget(sess, gm, gm.TargetObjectID, what)
		if !ok {
			return gmSubject{}, false
		}
		return gmSubject{name: t.Name, serverID: t.CharServerID, sess: ts, p: t}, true
	}
	if t := h.gameWorld.PlayerByName(name); t != nil && h.gameWorld.IsLivePlayer(t) {
		if ts := h.gameWorld.FindSessionByEntityID(t.ServerID); ts != nil {
			return gmSubject{name: t.Name, serverID: t.CharServerID, sess: ts, p: t}, true
		}
	}
	if h.resets == nil {
		h.gmReply(sess, fmt.Sprintf("%s: no player named %q is online, and this server keeps no characters", what, name))
		return gmSubject{}, false
	}
	rec, err := h.resets.GetByName(name)
	if err != nil || rec == nil {
		h.gmReply(sess, fmt.Sprintf("%s: no character named %q", what, name))
		return gmSubject{}, false
	}
	return gmSubject{name: rec.CharName, serverID: rec.ServerID}, true
}

// firstWord is s's first word and the rest after it, trimmed.
func firstWord(s string) (string, string) {
	s = strings.TrimSpace(s)
	w, rest, _ := strings.Cut(s, " ")
	return w, strings.TrimSpace(rest)
}

// resetCommand is #rst_mastr (training false) and #rst_both (true) for the
// character text names (its first word; none: the target or the GM).
func (h *ChatHandler) resetCommand(sess *session.Session, gm *entity.Player, text string, training bool) {
	what := "Mastery reset"
	if training {
		what = "Mastery and training reset"
	}
	name, _ := firstWord(text)
	s, ok := h.gmCharacter(sess, gm, name, what)
	if !ok {
		return
	}
	if h.cm == nil {
		h.gmReply(sess, what+": masteries are not served here")
	} else if s.sess != nil {
		h.gmReply(sess, h.cm.ResetOnline(s.sess))
	} else {
		h.gmReply(sess, h.cm.ResetOffline(s.serverID, s.name))
	}
	if training {
		h.gmReply(sess, h.resetTraining(s))
	}
	log.Printf("GM: %s: %s of %s", gm.Name, what, s.name)
}

// resetTraining gives back every training point the character spent (the
// create screen's twenty and the trainer's): in the world, the game's first
// (its base stats and unspent points, the stats channel again, 0x001D with
// old - new, TrainingResetText), then its row; out of it, its row. What is
// said to the GM.
func (h *ChatHandler) resetTraining(s gmSubject) string {
	if h.resets == nil {
		return "Training reset: this server keeps no characters"
	}
	if s.sess != nil {
		rec, err := h.resets.GetByServerID(s.serverID)
		if err != nil {
			return fmt.Sprintf("Training reset of %s: %v", s.name, err)
		}
		spent := database.TrainingSpent([7]int{rec.TPStrength, rec.TPStamina, rec.TPAgility, rec.TPDexterity,
			rec.TPWisdom, rec.TPIntel, rec.TPCharisma})
		if before, after, total, ok := h.gameWorld.TakeBackTraining(s.serverID, spent); ok {
			r := database.TrainingReset{Spent: spent, Unused: after, Total: total}
			if err := h.resets.TakeTraining(s.serverID, r); err != nil {
				log.Printf("GM: the training reset of '%s' not kept: %v", s.name, err)
				return fmt.Sprintf("%s's training points were reset but not kept: %v", s.name, err)
			}
			// the trainer takes z off what it shows: a negative z gives back
			SendTrainingPointsSpent(s.sess, int32(before-after))
			h.sendStatsNow(s.sess)
			SendColoredChat(s.sess, TrainingResetText, 255, 255, 0)
			log.Printf("GM: training of '%s' reset, %d points back (%d unspent of %d)", s.name, r.Refund(), after, total)
			return fmt.Sprintf("%s: %d training points back (%s), %d unspent of %d", s.name, r.Refund(), spentLine(spent), after, total)
		}
		// gone from the world meanwhile: its row is all there is
	}
	r, err := h.resets.ResetTraining(s.serverID)
	if err != nil {
		return fmt.Sprintf("Training reset of %s: %v", s.name, err)
	}
	log.Printf("GM: training of '%s' (offline) reset, %d points back (%d unspent of %d)", s.name, r.Refund(), r.Unused, r.Total)
	return fmt.Sprintf("%s (not in the world: at the next login): %d training points back (%s), %d unspent of %d",
		s.name, r.Refund(), spentLine(r.Spent), r.Unused, r.Total)
}

// spentLine is what each stat gave back, the stats that gave none left out.
func spentLine(spent [7]int) string {
	names := [7]string{"STR", "STA", "AGI", "DEX", "WIS", "INT", "CHA"}
	var parts []string
	for i, v := range spent {
		if v != 0 {
			parts = append(parts, fmt.Sprintf("%s %d", names[i], v))
		}
	}
	if len(parts) == 0 {
		return "none spent"
	}
	return strings.Join(parts, ", ")
}

// sendStatsNow puts the session's player's stats on its stat channel at
// once, as the trainer's spend does (training.go), besides the tick's.
func (h *ChatHandler) sendStatsNow(sess *session.Session) {
	conn := sess.Connection()
	if conn == nil || h.gameWorld == nil {
		return
	}
	p := h.gameWorld.SessionPlayer(sess)
	if p == nil {
		return
	}
	if ch := conn.GetClientStatUpdate(); ch != nil {
		ch.SetData(entity.SerializeStatUpdate(p))
	}
}

// removeAbilityCommand is #rem_abil <name> <ability>.
func (h *ChatHandler) removeAbilityCommand(sess *session.Session, gm *entity.Player, text string) {
	name, ability := firstWord(text)
	if name == "" || ability == "" {
		h.gmReply(sess, "Usage: #rem_abil <name> <ability: its name or spell id>")
		return
	}
	s, ok := h.gmCharacter(sess, gm, name, "Remove ability")
	if !ok {
		return
	}
	if h.cm == nil {
		h.gmReply(sess, "Remove ability: this server keeps no spell books")
		return
	}
	line, taken := h.cm.RemoveAbility(s.sess, s.serverID, s.name, ability)
	h.gmReply(sess, line)
	if taken {
		log.Printf("GM: %s: removed %q from %s's book", gm.Name, ability, s.name)
	}
}

// bankTunarCommand is #banktunar [name] <n>: the bank's tunar said with no
// number, else n added.
func (h *ChatHandler) bankTunarCommand(sess *session.Session, gm *entity.Player, args []string) {
	name, amount := "", ""
	switch len(args) {
	case 0:
	case 1:
		amount = args[0]
		if _, err := strconv.Atoi(amount); err != nil {
			name, amount = args[0], ""
		}
	default:
		name, amount = args[0], args[1]
	}
	s, ok := h.gmCharacter(sess, gm, name, "Bank tunar")
	if !ok {
		return
	}
	if amount == "" {
		if _, bank, ok := h.gameWorld.PurseOf(s.serverID); ok {
			h.gmReply(sess, fmt.Sprintf("%s has %d tunar in the bank", s.name, bank))
		} else if h.resets != nil {
			if rec, err := h.resets.GetByServerID(s.serverID); err == nil {
				h.gmReply(sess, fmt.Sprintf("%s has %d tunar in the bank (not in the world)", s.name, rec.BankTunar))
			}
		}
		h.gmReply(sess, "Usage: #banktunar [name] <amount> - added to the bank (negative takes away); no name: your target, else you")
		return
	}
	n, err := strconv.Atoi(amount)
	if err != nil {
		h.gmReply(sess, "Usage: #banktunar [name] <amount> - a number, added to the bank (negative takes away)")
		return
	}
	h.addBankTunar(sess, s, n)
}

// addBankTunar adds n to the bank of s, where the game holds it when it is in
// the world (under bankMu, kept and told: 0x1253), else on its row; said to
// the GM.
func (h *ChatHandler) addBankTunar(sess *session.Session, s gmSubject, n int) {
	if n == 0 {
		h.gmReply(sess, "Bank tunar: nothing to add")
		return
	}
	if s.sess != nil {
		bankMu.Lock()
		tunar, before, ok := h.gameWorld.PurseOf(s.serverID)
		after := database.AddTunar(before, n)
		if ok {
			h.gameWorld.SetPlayerTunar(s.serverID, tunar, after)
			if h.purse != nil {
				if err := h.purse.SetTunar(s.serverID, tunar, after); err != nil {
					log.Printf("GM: #banktunar for '%s' not kept: %v", s.name, err)
				}
			}
		}
		bankMu.Unlock()
		if ok {
			queueReliable(s.sess, protocol.OpcodeConfirmBankTunar, bankTunarPayload(after))
			h.gmReply(sess, fmt.Sprintf("%s's bank: %d %+d = %d tunar", s.name, before, after-before, after))
			log.Printf("GM: bank tunar of '%s' %d -> %d", s.name, before, after)
			return
		}
		// gone from the world meanwhile: its row
	}
	if h.resets == nil {
		h.gmReply(sess, "Bank tunar: this server keeps no characters")
		return
	}
	after, err := h.resets.AddBankTunar(s.serverID, n)
	if err != nil {
		h.gmReply(sess, fmt.Sprintf("Bank tunar of %s: %v", s.name, err))
		return
	}
	h.gmReply(sess, fmt.Sprintf("%s's bank (not in the world): %d tunar now", s.name, after))
	log.Printf("GM: bank tunar of '%s' (offline) %+d -> %d", s.name, n, after)
}

// serverDown is #servdown's restart and who may use it (SetServerDown).
var serverDown struct {
	mu    sync.Mutex
	fn    func() error
	level int
}

// serverDownWindow is how long a first #servdown waits for the second.
const serverDownWindow = 30 * time.Second

// SetServerDown gives #servdown (and the live client's /servdown) the
// server's restart, the GM panel's own (cmd/gameserver), and the acctlevel it
// needs (gm.admin_level). Without it #servdown says the server cannot restart
// itself.
func SetServerDown(fn func() error, adminLevel int) {
	serverDown.mu.Lock()
	defer serverDown.mu.Unlock()
	serverDown.fn, serverDown.level = fn, adminLevel
}

// servDownCommand is #servdown: from an admin, the first asks and the second
// within serverDownWindow restarts the server, in a goroutine of its own
// after the GM's line has gone (the restart sends the notice, every client its
// 0x07C0, saves everyone and starts the binary again).
func (h *ChatHandler) servDownCommand(sess *session.Session, gm *entity.Player) {
	serverDown.mu.Lock()
	fn, level := serverDown.fn, serverDown.level
	serverDown.mu.Unlock()
	if fn == nil {
		h.gmReply(sess, "Server down: this server cannot restart itself")
		return
	}
	if have, err := h.access.AccessLevel(sess.AccountID()); err != nil || have < level {
		h.gmReply(sess, fmt.Sprintf("Server down is an admin's (acctlevel %d)", level))
		return
	}
	now := time.Now()
	h.gm.mu.Lock()
	asked, again := h.gm.downAsked[gm.ServerID]
	if h.gm.downAsked == nil {
		h.gm.downAsked = map[entity.EntityID]time.Time{}
	}
	if !again || now.Sub(asked) > serverDownWindow {
		h.gm.downAsked[gm.ServerID] = now
		h.gm.mu.Unlock()
		h.gmReply(sess, fmt.Sprintf("Server down: every player is told, saved and sent to the login, and the server starts again. "+
			"Again within %d seconds to do it.", int(serverDownWindow/time.Second)))
		return
	}
	delete(h.gm.downAsked, gm.ServerID)
	h.gm.mu.Unlock()
	h.gmReply(sess, "Server down: restarting now")
	log.Printf("GM: %s (account %d) took the server down", gm.Name, sess.AccountID())
	go func() {
		time.Sleep(500 * time.Millisecond) // the GM's line goes out first
		if err := fn(); err != nil {
			log.Printf("GM: the restart %s asked for failed: %v", gm.Name, err)
		}
	}()
}
