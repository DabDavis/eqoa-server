package handlers

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// The hard-disk setup commands, as the SD payload sees them.
//
// A message whose FIRST character is U+E000 is not text: the payload's hook on
// the engine's message-display routine reads the second character as a verb,
// acts on it, and never draws the line. U+E000 is in the Unicode private use
// area, so it cannot occur in anything a player or a translator types — which
// matters, because that hook runs on every line of text the game displays.
//
// The verbs must match hdd_sd.S's CMD_* constants. TestHDDVerbsMatchPayload
// reads them out of the payload source rather than trusting this comment.
// Spelled as an escape, never as the character itself: it is invisible, and
// an editor or a copy-paste that drops it would leave every control string
// looking like ordinary text the player then sees.
const hddMark = '\uE000'

const (
	HDDCheck  rune = 1 // bring the driver stack up and mount; writes nothing
	HDDCreate rune = 2 // create EQOAWORLDS on an already-formatted disk
	HDDFormat rune = 3 // FORMAT THE WHOLE DISK, then create. Erases everything.
	// HDDRepair re-makes an EXISTING partition that will not mount. EQPREP has
	// taken this verb since it was written and nothing could reach it: the
	// setup SCREEN's menu was its only caller. The screen is gone, so the
	// channel carries it -- a capability with no route to it is one that does
	// not exist. It is not destructive in the way format is: it rebuilds the
	// one partition rather than erasing the disk, so it needs no timed arm.
	HDDRepair rune = 4
)

// ConsoleMidSDRefresh is set by main.go to the patch server's answer to "is this
// console between the two SD passes right now". nil means nobody wired it, and
// nil answers no — the technician then behaves exactly as it did before this
// existed, which is the right default for a server whose patcher is not the one
// that provisioned the console.
//
// WHY THE TECHNICIAN NEEDS TO ASK. Every command it sends is a control string
// the payload's display hook is supposed to swallow. With no payload nothing
// swallows it, and the client DRAWS it: the runes are in the Unicode private use
// area and have no glyphs, so the player gets a line of question marks and then
// silence. Observed on hardware 2026-08-12, mid-dance.
//
// That also refutes what this file used to claim — that a client without the
// payload "draws two invisible characters and nothing else happens", called
// there "the right failure". It is not invisible and it is not right.
var ConsoleMidSDRefresh func(host string) bool

// hddConsoleUpdating reports whether this player's console is known to be
// mid-update, and is the one condition under which the technician refuses to
// speak to the payload at all.
func hddConsoleUpdating(sess *session.Session) bool {
	if ConsoleMidSDRefresh == nil || sess == nil {
		return false
	}
	host := sessionIP(sess)
	return host != "" && ConsoleMidSDRefresh(host)
}

// HDDUpdatingReply is what the technician says instead of sending a command to a
// payload that is not there. It names the remedy, because the state is real and
// self-correcting: the console has deleted its old hard-disk support and will
// fetch the new one the next time the patcher runs.
const HDDUpdatingReply = "Your console is still collecting its hard-disk " +
	"support - it dropped the old copy and has not picked up the new one yet. " +
	"Quit to the title screen and let it update once more, then come back and " +
	"I can read your disk."

// sendHDDCommand hands the payload one verb, and records who asked.
//
// It rides the ordinary chat-message opcode, which is what makes this work at
// all: no new opcode, no CLIENT hook of our own on the receive path, and a
// server that is talking to a client WITHOUT the payload simply has its control
// string ignored — the client draws two invisible characters and nothing else
// happens. That is the right failure: nothing breaks, and the NPC's own text
// explains that the disk support is not installed.
func sendHDDCommand(sess *session.Session, player *entity.Player, verb rune) {
	token := newHDDToken()
	noteHDDRequest(sess, player, verb, token)
	sess.Queue().AddWithSequence(buildClientMessage(hddControlString(verb, token)))
}

// ---------------------------------------------------------------------------
// THE NONCE.
//
// The pending table below narrows a report to the console that asked, which
// leaves one case it cannot decide: two players behind one address who ask for
// the same thing at the same time. Nothing the server knows separates them,
// because everything it knows about the report came from the network.
//
// So the server issues a value with the command and the payload hands it back.
// It is the only field in the datagram that could not have been produced by
// whoever sent it, which makes it both the tiebreak and the reason a forged
// report is no longer worth writing.
//
// THE ENCODING IS FORCED BY THE PATH IT TRAVELS. The command reaches the
// console as a UTF-16 chat string, so a code unit in the surrogate range would
// be rewritten to U+FFFD by the encoder before it ever hit the wire, and a zero
// unit could be taken for a terminator. Both are avoided by keeping every unit
// inside the private use area: hddTokenBase | 12 bits, four units, 48 bits.
// hdd_sd.S checks that range on the way in and discards anything else, which is
// what lets an OLDER server — whose messages have nothing after the verb —
// reach a NEWER payload without inventing a token out of adjacent memory.
// ---------------------------------------------------------------------------

const (
	hddTokenUnits = 4                  // must match CMD_TOKN in hdd_sd.S
	hddTokenBase  = 0xE000             // must match CMD_TOKV
	hddTokenMask  = 0xF000             // must match CMD_TOKM
	hddTokenBits  = hddTokenUnits * 12 // 48; must match udp.TokenBits
	hddTokenMax   = 1<<hddTokenBits - 1
)

// newHDDToken returns a fresh nonce, or 0 if the system has no randomness to
// give. Zero is the value that means "no token", so a failure here degrades to
// the pending-table match rather than to a guessable token — the one outcome
// that would be worse than not having a nonce at all.
func newHDDToken() uint64 {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		log.Printf("HDD: no randomness for a command nonce (%v); this report "+
			"will be matched by account alone", err)
		return 0
	}
	return binary.BigEndian.Uint64(b[:]) & hddTokenMax
}

// hddControlString builds what the payload's hook reads: the marker, the verb,
// then the token most significant chunk first. A zero token appends nothing, so
// the string stays exactly the two units an older payload expects.
func hddControlString(verb rune, token uint64) string {
	out := []rune{hddMark, verb}
	if token == 0 {
		return string(out)
	}
	for i := hddTokenUnits - 1; i >= 0; i-- {
		chunk := (token >> (12 * uint(i))) & 0x0FFF
		out = append(out, rune(hddTokenBase|chunk))
	}
	return string(out)
}

// ---------------------------------------------------------------------------
// WHO ASKED.
//
// The payload's answer arrives on the world-delivery socket, not on the game
// session, so the datagram itself identifies nothing but the console's address.
// Matching on that address alone — which is all this used to do — is wrong in
// three separate ways, and only the first is cosmetic:
//
//   - two players behind one address are both told, including the one who
//     asked for nothing;
//   - a player who asked, then reconnected, is told on a session that no
//     longer exists, and hears nothing at all;
//   - a datagram from anywhere can claim a reply, because nothing in it has to
//     be known in advance.
//
// So the server remembers the ASK. The address still has to match — a report is
// only believed from the console it was sent to — but it selects a recorded
// request rather than a set of sessions, and the request carries the ACCOUNT,
// which is the identity that survives a reconnect where a session pointer does
// not.
//
// On its own this does not close the hole; it narrows it to two players behind
// one address who ask for the SAME verb inside the same window. Closing that
// needs the payload to echo something only the server could have issued, which
// is what newHDDToken above and CMD_TOKN in hdd_sd.S are for. Both mechanisms
// live here at once, because a console running an older payload cannot echo
// anything and still has to be answered.
// ---------------------------------------------------------------------------

// hddRequestWindow is how long an unanswered ask stays live.
//
// Generous on purpose. The payload runs EQPREP synchronously inside the
// message-display hook and only then reports, so this has to cover a whole
// disk format on real hardware — and the cost of being too generous is a stale
// entry matching a later report from the same console, while the cost of being
// too mean is a player who waited through a format and is told nothing.
const hddRequestWindow = 15 * time.Minute

// hddMaxRequests bounds the table. Reports are best-effort and some asks are
// never answered (no payload, no disk, a console switched off mid-format), so
// entries that nothing will ever consume are the normal case and must not
// accumulate.
const hddMaxRequests = 64

// hddRequest is one ask, waiting for its answer.
type hddRequest struct {
	account int             // survives a reconnect; the session pointer does not
	player  entity.EntityID // to prefer the same character, if it is still on
	name    string          // for the log, which is read by a human
	verb    rune
	ip      string
	token   uint64 // the nonce issued with the command; 0 if none was
	at      time.Time
}

var (
	hddReqMu sync.Mutex
	hddReqs  []hddRequest
)

// noteHDDRequest records an ask. Called for every verb the server sends,
// including the ones AutoHDD sends, so a self-test is matched the same way a
// player is rather than through a second path that could rot unnoticed.
func noteHDDRequest(sess *session.Session, player *entity.Player, verb rune,
	token uint64) {
	if sess == nil || player == nil {
		return
	}
	ip := sessionIP(sess)
	if ip == "" {
		return
	}
	hddReqMu.Lock()
	defer hddReqMu.Unlock()
	pruneHDDRequestsLocked()
	hddReqs = append(hddReqs, hddRequest{
		account: sess.AccountID(),
		player:  player.ServerID,
		name:    player.Name,
		verb:    verb,
		ip:      ip,
		token:   token,
		at:      time.Now(),
	})
	// Oldest first, so dropping the overflow drops the asks least likely to
	// still be waiting for an answer.
	if len(hddReqs) > hddMaxRequests {
		hddReqs = hddReqs[len(hddReqs)-hddMaxRequests:]
	}
}

// pruneHDDRequestsLocked drops expired entries. Caller holds hddReqMu.
func pruneHDDRequestsLocked() {
	live := hddReqs[:0]
	for _, r := range hddReqs {
		if time.Since(r.at) <= hddRequestWindow {
			live = append(live, r)
		}
	}
	hddReqs = live
}

// takeHDDRequest consumes the ask a report answers, reporting whether one was
// found. Entries are consumed whether or not the verb agreed: one ask, one
// answer, so a duplicate datagram cannot be delivered twice.
//
// A token decides it outright when the report carries one — that is the whole
// point of issuing it, and it is the only case where two players behind one
// address asking for the same thing can be told apart. The address still has to
// agree: a token is a secret the console was told, so a report bearing it from
// somewhere else is a replay rather than an answer.
//
// Without a token — an older payload, or a command that carried none — matching
// falls back to oldest-first among the entries from that address, preferring an
// exact verb match. The verb is a hint rather than a key there, because the
// payload reports the verb it ACTED on, and a verb it does not implement is
// counted and dropped; the ask is still this console's and still deserves an
// answer.
func takeHDDRequest(ip string, verb rune, token uint64) (hddRequest, bool) {
	hddReqMu.Lock()
	defer hddReqMu.Unlock()
	pruneHDDRequestsLocked()

	best := -1
	for i, r := range hddReqs {
		if r.ip != ip {
			continue
		}
		if token != 0 {
			if r.token == token {
				best = i
				break
			}
			continue // a report WITH a token answers only its own ask
		}
		if r.verb == verb {
			best = i
			break // oldest exact match wins outright
		}
		if best < 0 {
			best = i // oldest from this console, as a fallback
		}
	}
	if best < 0 {
		return hddRequest{}, false
	}
	req := hddReqs[best]
	hddReqs = append(hddReqs[:best], hddReqs[best+1:]...)
	return req, true
}

// sessionIP is the console's address with the port dropped. The port is a
// different one on the reporting socket, so only the host can ever match.
func sessionIP(sess *session.Session) string {
	addr := sess.RemoteAddr()
	if addr == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return ""
	}
	return host
}

// THE DESTRUCTIVE STEP IS ARMED, NOT ASKED.
//
// The setup SCENE puts "format the disk" behind two presses with an arm that
// clears on any movement. A dialogue has no such affordance — one mis-picked
// line would format a drive — so the arm is rebuilt here: FORMAT is refused
// unless the same player asked for it within formatArmWindow, and asking for
// anything else clears it.
//
// The payload cannot help with this. It sees a verb and cannot tell a confirmed
// request from a mis-clicked one, so it does not try; the whole guard lives on
// this side, where the player's intent actually is.
const formatArmWindow = 30 * time.Second

var (
	formatArmMu sync.Mutex
	formatArmed = map[entity.EntityID]time.Time{}
)

func armFormat(id entity.EntityID) {
	formatArmMu.Lock()
	defer formatArmMu.Unlock()
	formatArmed[id] = time.Now()
}

// takeFormatArm consumes the arm, reporting whether it was live. One arm, one
// format: it is removed whether or not it had expired, so a stale entry can
// never authorise a later request.
func takeFormatArm(id entity.EntityID) bool {
	formatArmMu.Lock()
	defer formatArmMu.Unlock()
	at, ok := formatArmed[id]
	delete(formatArmed, id)
	return ok && time.Since(at) <= formatArmWindow
}

func clearFormatArm(id entity.EntityID) {
	formatArmMu.Lock()
	defer formatArmMu.Unlock()
	delete(formatArmed, id)
}

// HDDSetupNPC is the Freeport technician. Matched by NAME rather than by id,
// the same way the travel NPCs are, so the row can be re-inserted or moved
// without the code having to learn a new number. sql/freeport_hdd_setup.sql
// puts him there; TestHDDSetupNPCNameMatchesSQL keeps the two spellings equal.
const HDDSetupNPC = "Archivist Vaal"

// The two menus. Which one is open decides what a numbered pick MEANS, which is
// why the player carries it: option 2 is "Odus" on a travel NPC and "format the
// disk" here, and a menu that answered the wrong question would answer it
// destructively.
const (
	HDDMenuNone = iota
	HDDMenuMain
	HDDMenuConfirmFormat
)

// hddMenuMain is ordered so the destructive line is not first and not last —
// not under the cursor when the box opens, and not where a held button lands.
var hddMenuMain = []string{
	"Check the hard disk",
	"Create the world partition",
	"FORMAT THE HARD DISK - erases everything",
	"Repair the world partition",
	"Never mind",
}

// hddMenuConfirmFormat puts the safe answer FIRST, so the pick that requires no
// thought is the one that does nothing. This is the dialogue half of the guard
// the setup scene spells with two presses and a clearing arm; the server half
// is the timed arm in hddSetupCommand, and both have to agree before anything
// is erased.
var hddMenuConfirmFormat = []string{
	"No - leave the disk alone",
	"Yes - ERASE THE ENTIRE HARD DISK",
}

// HDDMenuFor returns the menu a pick should be read against, or nil.
func HDDMenuFor(menu int) []string {
	switch menu {
	case HDDMenuMain:
		return hddMenuMain
	case HDDMenuConfirmFormat:
		return hddMenuConfirmFormat
	}
	return nil
}

// HDDGreeting is what the technician says when the box opens.
const HDDGreeting = "I keep the archives. If your console has a hard disk I can " +
	"prepare it to hold worlds. What shall I do?"

// HandleHDDMenuPick applies a numbered pick and returns the next menu to show
// (HDDMenuNone closes the dialogue). `reply` carries the technician's words.
//
// Everything destructive goes through hddSetupCommand, so the NPC cannot format
// a disk by a route the chat command's guard does not also cover.
func HandleHDDMenuPick(sess *session.Session, player *entity.Player, menu, idx int,
	reply func(string)) int {

	items := HDDMenuFor(menu)
	if idx < 0 || idx >= len(items) {
		clearFormatArm(player.ServerID)
		return HDDMenuNone
	}

	switch menu {
	case HDDMenuMain:
		switch idx {
		case 0:
			hddSetupCommand(sess, player, "check", reply, "")
		case 1:
			hddSetupCommand(sess, player, "create", reply, "")
		case 2:
			// Arms the timer and says what formatting costs; the confirm menu
			// is the second, independent step.
			hddSetupCommand(sess, player, "format", reply,
				"Answer the next question only if you are certain.")
			return HDDMenuConfirmFormat
		case 3:
			hddSetupCommand(sess, player, "repair", reply, "")
		default:
			clearFormatArm(player.ServerID)
			reply("As you wish.")
		}
		return HDDMenuNone

	case HDDMenuConfirmFormat:
		if idx == 1 {
			hddSetupCommand(sess, player, "format confirm", reply, "")
		} else {
			clearFormatArm(player.ServerID)
			reply("Wise. The disk is untouched.")
		}
		return HDDMenuNone
	}
	return HDDMenuNone
}

// hddSetupCommand implements #hddsetup, and is what the Freeport setup NPC
// calls once the player has picked a line.
//
// `reply` is how the caller talks back — gmReply for the chat command, NPC
// dialogue for the NPC — so the two share the guard and the wording rather
// than growing two copies that drift.
// `confirmHint` is how THIS caller's player confirms — a typed command in chat,
// a second menu from the NPC. The guard is the same either way; only the
// sentence that describes it differs, so only that sentence is passed in.
func hddSetupCommand(sess *session.Session, player *entity.Player, arg string,
	reply func(string), confirmHint string) {

	cmd := strings.ToLower(strings.TrimSpace(arg))

	// Refused before anything is armed or sent. `help` is exempt because it
	// talks to the PLAYER and never to the payload, so it is still useful — and
	// still true — while the console is updating.
	if cmd != "" && cmd != "help" && hddConsoleUpdating(sess) {
		clearFormatArm(player.ServerID)
		reply(HDDUpdatingReply)
		log.Printf("HDD: %s asked for %q but their console is mid-SD-refresh; "+
			"refused rather than talking to a payload that is not loaded",
			player.Name, cmd)
		return
	}

	switch cmd {
	case "", "help":
		clearFormatArm(player.ServerID)
		reply("Hard disk setup:")
		reply("  check  - report what is on the disk. Writes nothing.")
		reply("  create - make the EQOAWORLDS partition (disk already formatted)")
		reply("  format - ERASE THE WHOLE DISK, then make the partition")
		reply("  repair - re-make the partition when it will not mount")
		reply("The result comes back here when the console has finished.")

	case "repair":
		clearFormatArm(player.ServerID)
		sendHDDCommand(sess, player, HDDRepair)
		reply("Repairing the EQOAWORLDS partition.")
		log.Printf("HDD: %s requested REPAIR", player.Name)

	case "check":
		clearFormatArm(player.ServerID)
		sendHDDCommand(sess, player, HDDCheck)
		reply("Checking the hard disk.")
		log.Printf("HDD: %s requested CHECK", player.Name)

	case "create":
		clearFormatArm(player.ServerID)
		sendHDDCommand(sess, player, HDDCreate)
		reply("Creating the EQOAWORLDS partition.")
		log.Printf("HDD: %s requested CREATE", player.Name)

	case "format":
		armFormat(player.ServerID)
		reply("FORMATTING ERASES EVERYTHING ON THE HARD DISK - every save, every")
		reply("other game's data, permanently.")
		if confirmHint != "" {
			reply(confirmHint)
		}
		log.Printf("HDD: %s armed FORMAT", player.Name)

	case "format confirm":
		if !takeFormatArm(player.ServerID) {
			reply("Nothing armed, or the confirmation timed out. Ask for " +
				"'format' again if you meant it.")
			log.Printf("HDD: %s tried to confirm FORMAT with no live arm",
				player.Name)
			return
		}
		sendHDDCommand(sess, player, HDDFormat)
		// ASKING, not doing. The console decides: the payload refuses to format
		// a partition the game is streaming from, and says so in its report a
		// moment later. Announcing "formatting the disk" here meant the player
		// was told their disk was being erased and then, in the next line, that
		// nothing had happened — measured on console 2026-08-11.
		reply("Asking the console to format the hard disk. I will tell you " +
			"what it says.")
		log.Printf("HDD: %s CONFIRMED FORMAT — asking the console to erase the "+
			"disk", player.Name)

	default:
		clearFormatArm(player.ServerID)
		reply(fmt.Sprintf("Unknown option %q. Try: check, create, format, repair.", arg))
	}
}

// hddChatConfirmHint is the typed half of the format guard. The NPC passes its
// own (a second menu) instead, so neither path describes the other's mechanism.
var hddChatConfirmHint = fmt.Sprintf(
	"Type '#hddsetup format confirm' within %d seconds to go ahead.",
	int(formatArmWindow.Seconds()))

// HDDReport is one answer from a console, as the handlers see it. The wire
// decoding is udp.ParseStatus's job; this is the part that has to name a
// player, and it is a struct rather than four parameters so that the nonce can
// arrive later without every caller changing shape again.
type HDDReport struct {
	IP       string // the console the datagram came from
	Verb     rune   // the command it acted on
	Token    uint64 // the nonce that command carried; 0 = it carried none
	Describe string // the sentence the technician will say
}

// ReportHDDStatus is what the payload's answer turns into: the technician
// telling the player what actually happened to their disk. It returns the
// number of players told, which is 0 or 1.
//
// It answers the ASK, not the address — see the comment above hddRequestWindow
// for why the address alone is not an identity. An unsolicited report is
// LOGGED AND DROPPED rather than broadcast: a datagram no one asked for is a
// stale duplicate, a forgery, or a report that outlived the window, and none of
// those is worth telling an arbitrary bystander about their hard disk.
func ReportHDDStatus(sm *session.Manager, gw *entity.World, rep HDDReport) int {
	req, ok := takeHDDRequest(rep.IP, rep.Verb, rep.Token)
	if !ok {
		// With a token this is a replay or a forgery; without one it is a
		// stale duplicate or a report that outlived its window. The log says
		// which, because they call for different answers from a human.
		log.Printf("HDD: UNSOLICITED status from %s (verb %d, token %#x) — no "+
			"live request from that console; dropped: %s",
			rep.IP, rep.Verb, rep.Token, rep.Describe)
		return 0
	}
	if req.verb != rep.Verb {
		// The console acted on something other than what we last sent it, or
		// dropped the verb entirely. The ask is still real, so it is still
		// answered — but the disagreement is the interesting half of a bug
		// report and must not be swallowed.
		log.Printf("HDD: %s asked for verb %d and the console reported verb %d",
			req.name, req.verb, rep.Verb)
	}

	// The account is the identity, so a player who reconnected while the disk
	// was being formatted is still the right person to tell. Their character
	// may have changed in the meantime; prefer the one that asked, accept any.
	var target *session.Session
	sm.ForEach(func(sess *session.Session) bool {
		if sess.AccountID() != req.account {
			return true
		}
		// Only players who are actually in the world can be spoken to. A nil
		// world is the test's case, not the server's, and SessionPlayer has a
		// pointer receiver that would panic on one.
		var player *entity.Player
		if gw != nil {
			if player = gw.SessionPlayer(sess); player == nil {
				return true
			}
		}
		if target == nil {
			target = sess
		}
		if player != nil && player.ServerID == req.player {
			target = sess
			return false // the same character: no better match exists
		}
		return true
	})
	if target == nil {
		log.Printf("HDD: %s (account %d) asked for verb %d but is no longer in "+
			"world; dropped: %s", req.name, req.account, req.verb, rep.Describe)
		return 0
	}
	target.Queue().AddWithSequence(buildClientMessage(HDDSetupNPC + ": " + rep.Describe))
	return 1
}

// AutoHDD fires one hard-disk command after world entry when EQOA_HDD_SELFTEST
// is set, as "verb[,delaySeconds]" (delay defaults to 10).
//
// A TEST AFFORDANCE, off unless the variable is set. The real triggers are a
// chat command and an NPC dialogue, and both need text entry or walking to a
// spot that the tracer's pad commands cannot drive. This exercises everything between the server and the console —
// the control string, the payload's hook, hdd_init, EQPREP and the report — and
// only the picking of a menu line is left untested.
//
// FORMAT IS NOT ACCEPTED HERE. A destructive operation must not be reachable
// from an environment variable, where it would run on every world entry with no
// player having asked for anything and no arm to consume. The guard is
// deliberately not shared with hddSetupCommand: this is a different question
// (may an automated harness do this at all?) with a different answer.
//
// The delay lets the tracer arm its hooks before anything happens: the work it
// triggers runs during the load itself, and a trace armed afterwards reports 0
// hits — an arming-timing artifact that has been misread once already.
func AutoHDD(gw *entity.World, sess *session.Session) {
	spec := os.Getenv("EQOA_HDD_SELFTEST")
	if spec == "" {
		return
	}
	parts := strings.Split(spec, ",")
	verb := strings.ToLower(strings.TrimSpace(parts[0]))
	if verb == "format" || verb == "format confirm" {
		log.Printf("EQOA_HDD_SELFTEST=%q REFUSED: formatting a disk is not "+
			"reachable from an environment variable", spec)
		return
	}
	delay := 10 * time.Second
	if len(parts) > 1 {
		if n, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil && n >= 0 {
			delay = time.Duration(n) * time.Second
		}
	}
	go func() {
		time.Sleep(delay)
		player := gw.SessionPlayer(sess)
		if player == nil {
			log.Printf("EQOA_HDD_SELFTEST: no player on that session any more")
			return
		}
		log.Printf("EQOA_HDD_SELFTEST: sending %q for %s", verb, player.Name)
		hddSetupCommand(sess, player, verb,
			func(t string) { log.Printf("EQOA_HDD_SELFTEST: %s", t) }, "")
	}()
}
