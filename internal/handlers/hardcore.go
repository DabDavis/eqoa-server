package handlers

import (
	"encoding/binary"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// hardcore.go — [HC] Hardcore characters on a shard (world.hardcore; the owner,
// 2026-11-18: "lets add [HC] tag for characters. Hardcore mode chars stay dead
// and cannot respawn"). See the file's later parts and entity/hardcore.go.

// hardcoreSettings is world.hardcore as the handlers read it.
type hardcoreSettings struct {
	on                 bool
	shard              string
	logout             time.Duration
	otherDB, otherMark string
}

var hardcoreCfg atomic.Pointer[hardcoreSettings]

func init() { SetHardcore(config.HardcoreConfig{}) }

// hardcoreLogoutDefault is how long after a fall the client is sent to its
// login unless world.hardcore.fall_logout_seconds says: the respawn's wait.
const hardcoreLogoutDefault = entity.PlayerRespawnDelay

// SetHardcore sets world.hardcore.
func SetHardcore(c config.HardcoreConfig) {
	s := &hardcoreSettings{on: c.Enabled, shard: strings.TrimSpace(c.Shard), logout: hardcoreLogoutDefault,
		otherDB: strings.TrimSpace(c.OtherDB), otherMark: strings.TrimSpace(c.OtherMark)}
	if c.FallLogoutSeconds > 0 {
		s.logout = time.Duration(min(c.FallLogoutSeconds, 600)) * time.Second
	}
	hardcoreCfg.Store(s)
	entity.SetHardcoreOn(c.Enabled)
}

// HardcoreOn reports world.hardcore.
func HardcoreOn() bool { return hardcoreCfg.Load().on }

// ---------------------------------------------------------------------------
// The offer and the list (protocol.OpcodeHardcore, OpcodeHardcoreChars).
//
// A PS2's character select shows only what 0x002C carries (CLIENT's
// CharacterList: the name, 23 bytes, the level, class and race, the look), so a
// PS2 is told with the name alone (ps2ListName): "[HC] Name" for a living
// Hardcore character, "Name RIP 11/18 08:45" (the fall's UTC date and time,
// shortened to fit) for a fallen one. Vanilla takes the offer and is sent each
// character's state with its plain name, which it shows in place of the list's.

// buildHardcoreOffer is OpcodeHardcore from the server: a u32, 1.
func buildHardcoreOffer() []byte {
	return build(protocol.OpcodeHardcore, 6, func(w *buffer.Writer) {
		w.WriteUint32(1)
	})
}

// SendHardcoreOffer offers the session's client Hardcore, once a session,
// where world.hardcore is on.
func SendHardcoreOffer(sess *session.Session) {
	if HardcoreOn() && sess.OfferHardcoreOnce() {
		queueReliable(sess, protocol.OpcodeHardcore, buildHardcoreOffer())
	}
}

// hardcoreCharsVersion is OpcodeHardcoreChars' first byte.
const hardcoreCharsVersion = 1

// The flags of an OpcodeHardcoreChars entry.
const (
	hcFlagHardcore   = 1
	hcFlagFallen     = 2
	hcFlagChoiceOpen = 4
)

// encodeHardcoreChars is OpcodeHardcoreChars' body for records (the list's
// order) by their states.
func encodeHardcoreChars(records []*database.CharacterRecord, states map[uint32]database.HardcoreState) []byte {
	if len(records) > 65535 {
		records = records[:65535]
	}
	b := binary.LittleEndian.AppendUint16([]byte{hardcoreCharsVersion}, uint16(len(records)))
	for _, c := range records {
		st := states[uint32(c.ServerID)]
		b = binary.LittleEndian.AppendUint32(b, uint32(c.ServerID))
		b = appendStr8(b, c.CharName)
		var flags byte
		if st.Hardcore {
			flags |= hcFlagHardcore
		}
		if st.Fallen() {
			flags |= hcFlagFallen
		}
		if st.ChoiceOpen() {
			flags |= hcFlagChoiceOpen
		}
		b = append(b, flags)
		if st.Fallen() {
			f := st.Fall
			b = binary.LittleEndian.AppendUint32(b, uint32(max(f.At, 0)))
			b = append(b, byte(max(0, min(255, f.Level))))
			b = binary.LittleEndian.AppendUint16(b, uint16(max(0, min(65535, f.CMs))))
			b = appendStr8(b, f.Killer)
			b = appendStr8(b, f.Place)
		}
	}
	return b
}

// hardcoreStates is the account's characters' states (none on a failed read).
func (h *CharacterHandler) hardcoreStates(accountID int) map[uint32]database.HardcoreState {
	if h.store == nil || h.store.DB() == nil || accountID <= 0 {
		return nil
	}
	st, err := h.store.DB().HardcoreOfAccount(accountID)
	if err != nil {
		log.Printf("Hardcore: account %d's characters: %v", accountID, err)
		return nil
	}
	return st
}

// sendHardcoreChars sends a client that took the offer its list's states.
func (h *CharacterHandler) sendHardcoreChars(sess *session.Session, records []*database.CharacterRecord, states map[uint32]database.HardcoreState) {
	if !HardcoreOn() || !sess.HardcoreClient() {
		return
	}
	body := encodeHardcoreChars(records, states)
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeHardcoreChars)
	msg.SetData(append(binary.LittleEndian.AppendUint16(make([]byte, 0, 2+len(body)), uint16(protocol.OpcodeHardcoreChars)), body...))
	sess.Queue().AddWithSequence(msg)
}

// ps2ListName is the name a character is listed by on a client that did not
// take the offer (a PS2): a living Hardcore character's "[HC] Name", a fallen
// one's "Name RIP" and when it fell (UTC), as much of "11/18/26 08:45" as fits
// CLIENT's 23 bytes with the name whole: the time goes first, then the year,
// then the spaces ("NamelyFifteennnRIP11/18"). Any other's its own.
func ps2ListName(name string, st database.HardcoreState) string {
	if st.Fallen() {
		t := time.Unix(st.Fall.At, 0).UTC()
		for _, s := range []string{
			" RIP " + t.Format("01/02/06 15:04"),
			" RIP " + t.Format("01/02 15:04"),
			" RIP " + t.Format("01/02/06"),
			" RIP " + t.Format("01/02"),
			" RIP" + t.Format("01/02"),
			"RIP" + t.Format("01/02"),
			"RIP",
		} {
			if len(name)+len(s) <= entity.ShownNameMost {
				return name + s
			}
		}
		return name
	}
	if st.Hardcore && HardcoreOn() && len(entity.HCTag)+len(name) <= entity.ShownNameMost {
		return entity.HCTag + name
	}
	return name
}

// listNameFor is the name the session's list carries for c.
func listNameFor(sess *session.Session, c *database.CharacterRecord, states map[uint32]database.HardcoreState) string {
	st, ok := states[uint32(c.ServerID)]
	if !ok || sess.HardcoreClient() {
		return c.CharName
	}
	return ps2ListName(c.CharName, st)
}

// HardcoreHandler takes a client's answer to the offer.
type HardcoreHandler struct {
	chars *CharacterHandler
}

// NewHardcoreHandler creates the offer's handler.
func NewHardcoreHandler(chars *CharacterHandler) *HardcoreHandler {
	return &HardcoreHandler{chars: chars}
}

// RegisterHandlers registers it.
func (h *HardcoreHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeHardcore, h.HandleHardcore)
}

// HandleHardcore takes the answer: a u32, nonzero to show Hardcore. A 1 is
// taken only on a session it was offered on; a client still choosing a
// character is sent its characters' states at once.
func (h *HardcoreHandler) HandleHardcore(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 4 {
		return nil
	}
	on := binary.LittleEndian.Uint32(data[:4]) != 0
	if on && !sess.HardcoreOffered() {
		log.Printf("Hardcore: %s asked for it, which was not offered", sess.RemoteAddr())
		return nil
	}
	sess.SetHardcoreClient(on)
	if !on || h.chars == nil || h.chars.store == nil || sess.AccountID() <= 0 {
		return nil
	}
	if sess.State() != session.StateCharacterSelect || sess.CharacterInWorld() {
		return nil
	}
	records, err := h.chars.store.GetByAccountID(sess.AccountID())
	if err != nil {
		return nil
	}
	records = h.chars.sessionChars(sess, records)
	h.chars.sendHardcoreChars(sess, records, h.chars.hardcoreStates(sess.AccountID()))
	return nil
}

// ---------------------------------------------------------------------------
// The choice at creation: a client that took the offer writes 'H', a u8
// version (1) and a u8 choice (1 Hardcore) after 0x002B's own fields, before
// the race options' block.

const (
	hardcoreMark    = 'H'
	hardcoreVersion = 1
)

// hardcoreBlock reads the choice off the front of rest (the bytes after
// 0x002B's fields): present is whether it is there; rest the bytes after it.
// ok false: a block cut short or of another version.
func hardcoreBlock(rest []byte) (hardcore, present, ok bool, after []byte) {
	if len(rest) == 0 || rest[0] != hardcoreMark {
		return false, false, true, rest
	}
	if len(rest) < 3 || rest[1] != hardcoreVersion || rest[2] > 1 {
		return false, true, false, rest
	}
	return rest[2] == 1, true, true, rest[3:]
}

// chooseAtCreation keeps a creation's choice (a client that took the offer):
// made, so its first world entry asks nothing.
func (h *CharacterHandler) chooseAtCreation(serverID uint32, name string, on bool) {
	if h.store == nil || h.store.DB() == nil {
		return
	}
	if _, err := h.store.DB().ChooseHardcore(serverID, on); err != nil {
		log.Printf("Hardcore: '%s' made, its choice (%v) not kept: %v", name, on, err)
		return
	}
	log.Printf("Hardcore: '%s' made %s", name, map[bool]string{true: "HARDCORE", false: "a normal character"}[on])
}

// ---------------------------------------------------------------------------
// Character select and the world entry.

// hardcoreSelectRefused reports a fallen character refused at select: told as
// CLIENT tells a character it cannot enter (word 676; vanilla never sends the
// select for one it shows fallen, and says why itself).
func (h *CharacterHandler) hardcoreSelectRefused(sess *session.Session, record *database.CharacterRecord) bool {
	if h.store == nil || h.store.DB() == nil {
		return false
	}
	fallen := false
	if st, err := h.store.DB().HardcoreOf(uint32(record.ServerID)); err == nil && st.Fallen() {
		fallen = true
	}
	if !fallen && h.gameWorld != nil && h.gameWorld.FallenInWorld(uint32(record.ServerID)) {
		fallen = true // its fall not yet written
	}
	if !fallen {
		return false
	}
	log.Printf("SelectedChar: REFUSED '%s' (serverID=%d) from %s: a fallen Hardcore character", record.CharName, record.ServerID, sess.RemoteAddr())
	SendCharSelectNotice(sess, protocol.OpcodeCharSelectAreaUnavailable)
	sess.Queue().SetBodyFlags(protocol.BodyFlagGuaranteedAck)
	return true
}

// hcEntered is when each Hardcore character in the world entered it (its
// played time's count), by its server id.
var hcEntered sync.Map

// applyHardcore reads the session's character's Hardcore state at its entry
// (a login, a takeover, a keep restart's resume) and gives it to its player.
func (h *CharacterHandler) applyHardcore(sess *session.Session, player *entity.Player) {
	if player == nil || h.gameWorld == nil || h.store == nil || h.store.DB() == nil {
		return
	}
	st, err := h.store.DB().HardcoreOf(player.CharServerID)
	if err != nil {
		log.Printf("Hardcore: %s's state: %v", player.Name, err)
		return
	}
	h.gameWorld.SetPlayerHardcore(sess, st.Hardcore, st.Fallen())
	if st.Hardcore && !st.Fallen() {
		hcEntered.LoadOrStore(player.CharServerID, time.Now())
	}
}

// hardcoreLeft adds a Hardcore character's time in the world to its count as
// it leaves (OnPlayerLeaving).
func hardcoreLeft(db *database.SQLiteDB) func(p *entity.Player) {
	return func(p *entity.Player) {
		at, ok := hcEntered.LoadAndDelete(p.CharServerID)
		if !ok || db == nil || p.Fallen {
			return
		}
		if err := db.AddHardcorePlayed(p.CharServerID, int64(time.Since(at.(time.Time))/time.Second)); err != nil {
			log.Printf("Hardcore: %s's time played: %v", p.Name, err)
		}
	}
}

// ---------------------------------------------------------------------------
// The fall (entity.OnHardcoreFall's hook).

// The fall's words (ours).
const (
	hardcoreYouFell = "You have fallen. A Hardcore character's death is final: %s's journey ends here. Their name stands on the Memorial of the Fallen in Freeport."
)

var hardcoreAnnounceRGB = [3]byte{0xC8, 0x3C, 0x3C}

// fallVerb is how a fall's cause reads after "has fallen in <place>, ".
func fallVerb(cause, killer string) string {
	switch cause {
	case entity.CauseCreature, entity.CauseGMEvent, entity.CausePvP:
		return "slain by " + killer
	case entity.CauseDrowning:
		return "drowned"
	case entity.CauseLava:
		return "taken by the lava"
	case entity.CauseSun:
		return "burned by the sun"
	case entity.CauseHazard:
		return "lost when " + killer + " gave way"
	case entity.CauseSelf:
		return "undone by their own spell"
	case entity.CauseGM:
		return "struck down by a GM"
	}
	return "lost to unknown causes"
}

// fallPlace names where a fall happened: a Tunaria town ("Freeport") or the
// wilds near one, the Plane of Sky's isle, a homeland's home, else the world.
func fallPlace(world uint8, x, y, z float32) string {
	switch {
	case world == 0:
		if town, d, ok := entity.NearestTown(world, x, z); ok {
			if d <= 400 {
				return town
			}
			return "the wilds near " + town
		}
		return "Tunaria"
	case int(world) == 4:
		return placeName(world, x, y, z)
	}
	if home, ok := entity.HomeAt(world, x, z); ok {
		return home
	}
	if n, ok := fallWorldNames[world]; ok {
		return n
	}
	return worldName(world)
}

// fallWorldNames are the disc's worlds by the names players know them by.
var fallWorldNames = map[uint8]string{1: "the Rathe Mountains", 2: "Odus", 3: "Lavastorm", 5: "the Plane of Disease", 7: "the Plane of Disease"}

// HardcoreFallLine is the shard's announcement of a fall: "Meshuggah the level
// 52 Shadow Knight has fallen in Freeport, slain by a Zek sentry."
func HardcoreFallLine(f database.FallRecord) string {
	return fmt.Sprintf("%s the level %d %s has fallen in %s, %s.", f.Name, f.Level, entity.ClassName(f.Class), f.Place, fallVerb(f.Cause, f.Killer))
}

// hardcoreFell is a Hardcore character's fall: written to its row (tried
// again a few times; the select refuses a character fallen in the world
// meanwhile), the shard told, its client told and, after the fall's wait, sent
// back to its login (0x0777, CLIENT's quiet disconnect: a PS2 lies dead with
// its pad shut until a MemoryDump, which never comes), its session ended.
func (h *CharacterHandler) hardcoreFell(sess *session.Session, f entity.HardcoreFall) {
	rec := database.FallRecord{ServerID: f.CharServerID, Name: f.Name, At: f.At.Unix(), Cause: f.Cause, Killer: f.Killer,
		Place: fallPlace(f.World, f.X, f.Y, f.Z), World: int(f.World), X: float64(f.X), Y: float64(f.Y), Z: float64(f.Z),
		Level: f.Level, Class: f.Class, Race: f.Race}
	if at, ok := hcEntered.LoadAndDelete(f.CharServerID); ok {
		rec.Played = int64(f.At.Sub(at.(time.Time)) / time.Second)
	}
	if h.store != nil {
		if m, err := h.store.LoadMastery(f.CharServerID); err == nil {
			rec.CMs = m.Points + m.Spent
		}
	}
	var db *database.SQLiteDB
	if h.store != nil {
		db = h.store.DB()
	}
	written := false
	for try := 0; db != nil && try < 5 && !written; try++ {
		if try > 0 {
			time.Sleep(time.Second)
		}
		fell, err := db.Fall(rec)
		if err != nil {
			log.Printf("Hardcore: %s's fall not written (try %d): %v", f.Name, try+1, err)
			continue
		}
		written = true
		if !fell {
			log.Printf("Hardcore: %s's fall: the row is not a living Hardcore character's (written before?)", f.Name)
		}
	}
	line := HardcoreFallLine(rec)
	log.Printf("Hardcore: FALL %s (serverid %d, level %d %s, %d CMs, %ds played): %s [%s]", f.Name, f.CharServerID, f.Level,
		entity.ClassName(f.Class), rec.CMs, rec.Played, line, f.Cause)
	if h.gameWorld != nil {
		h.gameWorld.AnnounceAll(line, hardcoreAnnounceRGB)
	}
	if sess == nil {
		return
	}
	wait := hardcoreCfg.Load().logout
	SendColoredChat(sess, fmt.Sprintf(hardcoreYouFell, f.Name), hardcoreAnnounceRGB[0], hardcoreAnnounceRGB[1], hardcoreAnnounceRGB[2])
	if sess.HardcoreClient() {
		queueReliable(sess, protocol.OpcodeHardcoreFall, buildHardcoreFall(rec, wait))
	}
	hardcoreAfter(wait, func() {
		if sess.CharacterServerID() != f.CharServerID || sess.PendingTermination() {
			return
		}
		if _, replaced := sess.ReplacedFor(); replaced {
			return
		}
		log.Printf("Hardcore: %s's client sent to its login (0x0777)", f.Name)
		SendDisconnectNoError(sess)
		sess.MarkReplaced() // delivered, then the session saved and let go as an ejected one's
	})
}

// hardcoreAfter runs f after d (time.AfterFunc; a test's own).
var hardcoreAfter = func(d time.Duration, f func()) { time.AfterFunc(d, f) }

// buildHardcoreFall is OpcodeHardcoreFall.
func buildHardcoreFall(f database.FallRecord, wait time.Duration) []byte {
	b := binary.LittleEndian.AppendUint16(nil, uint16(protocol.OpcodeHardcoreFall))
	b = binary.LittleEndian.AppendUint32(b, uint32(max(f.At, 0)))
	b = appendStr8(b, f.Killer)
	b = appendStr8(b, f.Place)
	return binary.LittleEndian.AppendUint16(b, uint16(min(int(wait/time.Second), 65535)))
}
