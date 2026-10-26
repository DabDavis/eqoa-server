package handlers

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"strings"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The Class Mastery window (0x1401 in, 0x1402 out), CLIENT's
// VIClientPart4EC58 (decomp viclientpart4ec58.cpp) and its windows
// (viwndclassmastery*.cpp). Every request is answered with a reply that
// ends CLIENT's wait (m_pending: sub-ops 1, 2, 5, 6, 8 and 9): the window has
// no timeout, and a request left unanswered leaves it waiting with no way
// out, CLIENT sending nothing more meanwhile.
//
// The list and the ranks are this character's, from cmcatalog
// (entity/cmbuy.go). Until F3 pass 17 the server replayed one captured list
// for everyone (pcaps/ps2_main_capture.pcap #1711, reassembled wrongly: 43 of
// its 96 skills read, the rest another message's bytes) and sold nothing.

// 0x1401's first byte: CLIENT's requests, each followed by the server's token
// (raw: what the last reply or the MemoryDump gave) and, but for Open, an
// argument.
const (
	cmShare  = 0x01 // Select(percent): the mastery points window's share of experience, 0-100
	cmBuy    = 0x02 // Send02(skill): buy a mastery (the inspect window)
	cmRemove = 0x03 // Send03(skill): remove one
	cmOpen   = 0x04 // Open(): the list
	cmBrowse = 0x05 // Browse(skill): one skill's page
)

// 0x1402's first byte: the replies CLIENT's Recv1402 dispatches.
const (
	cmRespShare   = 0x01 // Recv01: token, share
	cmRespDone    = 0x02 // Recv02: token (ends the wait, changes nothing)
	cmRespPoints  = 0x03 // Recv03: unspent, spent (entity.SendMasteryPoints)
	cmRespBought  = 0x05 // Recv05: token, id, unspent, spent, speed, 29 stats, 7 caps, the known flags
	cmRespRemoved = 0x06 // Recv06: token, id, speed, 29 stats, 7 caps, the known flags
	cmRespList    = 0x08 // Recv08: token, the list (VICMSkill a rank)
	cmRespNode    = 0x09 // Recv09: token, one rank (VICMNode)
)

// The longest strings CLIENT keeps (ReadString skips a longer one whole):
// VICMSkill/VICMNode's names and title 64 wide characters, the text 256, an
// ability's and a mastery's line 64.
const (
	cmNameMax = 63
	cmTextMax = 255
	cmLineMax = 63
)

// masteryStore keeps a character's mastery points and the masteries it
// bought (database.SQLiteCharacterStore).
type masteryStore interface {
	SaveMastery(serverID uint32, m database.MasteryRow) error
	BuyCM(serverID uint32, o database.OwnedCM, m database.MasteryRow) error
	RemoveCMs(serverID uint32, ids []int, m database.MasteryRow) error
	RemoveSpellFromBook(serverID uint32, spellID int) (bool, error)
}

// cmBook is a character's spell book (database.SpellbookStore): what a
// mastery needs, and where the abilities it adds go.
type cmBook interface {
	GetCharacterSpells(serverID uint32) ([]database.CharSpellEntry, error)
	AddSpell(serverID uint32, spellID, order int) error
}

// ClassMasteryHandler answers 0x1401.
type ClassMasteryHandler struct {
	sessionManager *session.Manager
	gameWorld      *entity.World
	store          masteryStore
	book           cmBook
}

// NewClassMasteryHandler creates a new class mastery handler.
func NewClassMasteryHandler(sessionMgr *session.Manager) *ClassMasteryHandler {
	return &ClassMasteryHandler{sessionManager: sessionMgr}
}

// SetGameWorld gives the handler the players' mastery points and ranks.
func (h *ClassMasteryHandler) SetGameWorld(gw *entity.World) {
	h.gameWorld = gw
}

// SetStore gives it where to keep them. Without one a change lasts until the
// logout's save (the points; the ranks bought are lost).
func (h *ClassMasteryHandler) SetStore(s masteryStore) {
	h.store = s
}

// SetSpellbook gives it the spell books (the abilities a mastery needs and
// adds). Without one no ability is needed or given.
func (h *ClassMasteryHandler) SetSpellbook(b cmBook) {
	h.book = b
}

// RegisterHandlers registers class mastery handlers with the registry.
func (h *ClassMasteryHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeClassMastery, h.HandleClassMastery)
}

// HandleClassMastery handles 0x1401: a sub-op byte, the token (u32) and its
// argument (u32). EVERY request is answered.
func (h *ClassMasteryHandler) HandleClassMastery(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 5 {
		log.Printf("ClassMastery: short payload (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	sub := data[0]
	token := binary.LittleEndian.Uint32(data[1:5])
	var arg uint32
	if len(data) >= 9 {
		arg = binary.LittleEndian.Uint32(data[5:9])
	}

	// the replies in order, the first of them the one that ends the wait
	var replies [][]byte
	one := func(b []byte) {
		if b != nil {
			replies = append(replies, b)
		}
	}
	switch sub {
	case cmShare:
		one(cmShareReply(token, h.setShare(sess, int(int32(arg)))))
	case cmOpen:
		one(h.list(sess, token))
	case cmBrowse:
		one(h.browse(sess, token, int(int32(arg))))
	case cmBuy:
		one(h.buy(sess, token, int(int32(arg))))
	case cmRemove:
		replies = h.remove(sess, token, int(int32(arg)))
	default:
		log.Printf("ClassMastery: unknown sub-op %#x (%#x) from %s", sub, arg, sess.RemoteAddr())
	}
	if len(replies) == 0 {
		replies = [][]byte{cmDone(token)}
	}
	for _, r := range replies {
		queueReliable(sess, protocol.OpcodeClassMasteryServer, r)
	}
	return nil
}

// cmShareReply is Recv01: the token and the share now.
func cmShareReply(token uint32, share int) []byte {
	return build(protocol.OpcodeClassMasteryServer, 2+9, func(w *buffer.Writer) {
		w.WriteByte(cmRespShare)
		w.WriteUint32(token)
		w.WriteUint32(uint32(share))
	})
}

// cmDone is Recv02: the token alone.
func cmDone(token uint32) []byte {
	return build(protocol.OpcodeClassMasteryServer, 2+5, func(w *buffer.Writer) {
		w.WriteByte(cmRespDone)
		w.WriteUint32(token)
	})
}

// cmBig builds a reply that may be long (a class's list runs past 10 KB; the
// queue fragments it).
func cmBig(fill func(w *buffer.Writer)) []byte {
	buf := make([]byte, 65536)
	w := buffer.NewWriter(buf)
	w.WriteUint16(uint16(protocol.OpcodeClassMasteryServer))
	fill(w)
	return buf[:w.Position()]
}

// writeWide is VIMemStream::ReadString(unsigned short *, MaxBytes): a u32
// count of wide characters, then them, cut to max.
func writeWide(w *buffer.Writer, s string, max int) {
	r := []rune(s)
	if len(r) > max {
		r = r[:max]
	}
	w.WriteUint32(uint32(len(r)))
	for _, c := range r {
		if c > 0xFFFF {
			c = '?'
		}
		w.WriteUint16(uint16(c))
	}
}

// writeCMSkill is one VICMSkill (VICMSkill::Read): its key (the id), owned,
// known, the id, its parent's key, the category, the cost, the name.
func writeCMSkill(w *buffer.Writer, e entity.CMEntry) {
	w.Write7BitEncodedInt64(int64(e.ID))
	w.WriteByte(b2u8(e.Owned))
	w.WriteByte(b2u8(e.Known))
	w.Write7BitEncodedInt64(int64(e.ID))
	w.Write7BitEncodedInt64(int64(e.Parent))
	w.WriteByte(byte(e.Category))
	w.Write7BitEncodedInt64(int64(e.Cost))
	writeWide(w, e.Name, cmNameMax)
}

func b2u8(b bool) byte {
	if b {
		return 1
	}
	return 0
}

// cmBookOf is the spells in the player's spell book, by id.
func (h *ClassMasteryHandler) cmBookOf(p *entity.Player) map[int]bool {
	out := map[int]bool{}
	if h.book == nil || p == nil {
		return out
	}
	book, err := h.book.GetCharacterSpells(p.CharServerID)
	if err != nil {
		log.Printf("ClassMastery: %s's book: %v", p.Name, err)
		return out
	}
	for _, e := range book {
		out[e.Pattern.SpellID] = true
	}
	return out
}

// list is Recv08: the token and every rank the character is offered.
func (h *ClassMasteryHandler) list(sess *session.Session, token uint32) []byte {
	if h.gameWorld == nil {
		return nil
	}
	p := h.gameWorld.SessionPlayer(sess)
	entries := h.gameWorld.CMList(sess, h.cmBookOf(p))
	log.Printf("ClassMastery: list of %d masteries to %s", len(entries), sess.RemoteAddr())
	return cmBig(func(w *buffer.Writer) {
		w.WriteByte(cmRespList)
		w.WriteUint32(token)
		w.WriteUint32(uint32(len(entries)))
		for _, e := range entries {
			writeCMSkill(w, e)
		}
	})
}

// browse is Recv09: one rank (VICMNode::Read), or Recv02 for a rank the
// character is not offered.
func (h *ClassMasteryHandler) browse(sess *session.Session, token uint32, id int) []byte {
	if h.gameWorld == nil {
		return nil
	}
	p := h.gameWorld.SessionPlayer(sess)
	n, ok := h.gameWorld.CMNodeOf(sess, id, h.cmBookOf(p))
	if !ok {
		SendErrorMessage(sess, "That mastery is not offered to you.")
		return nil
	}
	return cmBig(func(w *buffer.Writer) {
		w.WriteByte(cmRespNode)
		w.WriteUint32(token)
		writeCMNode(w, n)
	})
}

// writeCMNode is one VICMNode (VICMNode::Read): the key, owned, known, the id
// buy and remove send, the parent's key, the category, the level, two ints
// (m_x14: the cost, which no window shows; m_x18: the points spent it needs),
// the base stats it needs (STR..CHA), the name, the rank (m_xB8, unshown),
// the title (the inspect window's "Inspect: %S"), the text (its first page),
// the stats it modifies (slot, value), its abilities (the inspect window
// lists a 1 under "Adds Abilities" and a 0 under "Removes Abilities"; the
// requirements window lists the 0s as needed), and the masteries it needs.
func writeCMNode(w *buffer.Writer, n entity.CMNode) {
	w.Write7BitEncodedInt64(int64(n.ID))
	w.WriteByte(b2u8(n.Owned))
	w.WriteByte(b2u8(n.Known))
	w.Write7BitEncodedInt64(int64(n.ID))
	w.Write7BitEncodedInt64(int64(n.Parent))
	w.WriteByte(byte(n.Category))
	w.WriteByte(byte(min(max(n.Level, 0), 255)))
	w.Write7BitEncodedInt64(int64(n.Cost))
	w.Write7BitEncodedInt64(int64(n.Spent))
	for _, v := range n.MinStats {
		w.Write7BitEncodedInt64(int64(v))
	}
	writeWide(w, n.Name, cmNameMax)
	w.Write7BitEncodedInt64(int64(n.Rank))
	writeWide(w, n.Title, cmNameMax)
	writeWide(w, n.Text, cmTextMax)
	w.Write7BitEncodedInt64(int64(len(n.Mods)))
	for _, m := range n.Mods {
		w.WriteByte(byte(m.Slot))
		w.Write7BitEncodedInt64(int64(m.Value))
	}
	w.Write7BitEncodedInt64(int64(len(n.Abilities)))
	for _, a := range n.Abilities {
		writeWide(w, a.Text, cmLineMax)
		w.WriteByte(b2u8(a.Add))
	}
	w.Write7BitEncodedInt64(int64(len(n.Masteries)))
	for _, m := range n.Masteries {
		writeWide(w, m, cmLineMax)
	}
}

// writeCMState is what Recv05 and Recv06 read after the points: the run
// speed (raw; CLIENT sets it), the 29 stats and the 7 caps the masteries
// give (zigzag), then the known flag of every rank listed (a raw count and
// raw (id, flag) pairs: ReadFlags).
func writeCMState(w *buffer.Writer, st entity.CMState, list []entity.CMEntry) {
	w.WriteUint32(math.Float32bits(st.Speed))
	for _, v := range st.Stats {
		w.Write7BitEncodedInt64(int64(v))
	}
	for _, v := range st.Bonuses {
		w.Write7BitEncodedInt64(int64(v))
	}
	w.WriteUint32(uint32(len(list)))
	for _, e := range list {
		w.WriteUint32(uint32(e.ID))
		w.WriteUint32(uint32(b2u8(e.Known)))
	}
}

// buy is a purchase (sub-op 2): Recv05 when bought, else the refusal in red
// and Recv02.
func (h *ClassMasteryHandler) buy(sess *session.Session, token uint32, id int) []byte {
	if h.gameWorld == nil {
		return nil
	}
	p := h.gameWorld.SessionPlayer(sess)
	if p == nil {
		return nil
	}
	res := h.gameWorld.BuyCM(sess, id, h.cmBookOf(p))
	if res.Refusal != "" {
		log.Printf("ClassMastery: %s buying %d refused: %s", p.Name, id, res.Refusal)
		SendErrorMessage(sess, res.Refusal)
		return nil
	}
	if h.store != nil {
		if err := h.store.BuyCM(p.CharServerID, database.OwnedCM{ID: id, Cost: res.Paid}, MasteryRow(res.Mastery)); err != nil {
			log.Printf("ClassMastery: %s's purchase of %s not kept: %v", p.Name, res.Rank.Name, err)
		}
	}
	SendColoredChat(sess, fmt.Sprintf("You have bought %s for %d mastery points.", res.Rank.Name, res.Paid), 255, 255, 0)
	h.giveAbilities(sess, p, res.Rank.Adds)
	h.takeAbilities(sess, p, res.Rank.Removes)
	list := h.gameWorld.CMList(sess, h.cmBookOf(p))
	return cmBig(func(w *buffer.Writer) {
		w.WriteByte(cmRespBought)
		w.WriteUint32(token)
		w.WriteUint32(uint32(id))
		w.WriteUint32(uint32(res.Mastery.Points))
		w.WriteUint32(uint32(res.Mastery.Spent))
		writeCMState(w, res.State, list)
	})
}

// remove is a removal (sub-op 3): Recv06 and the points (Recv03, which
// Recv06 does not carry: the spent ones fewer, the unspent as they were, as
// a removal gives nothing back, so CLIENT announces no points earned) when
// removed, else the refusal in red and Recv02.
func (h *ClassMasteryHandler) remove(sess *session.Session, token uint32, id int) [][]byte {
	if h.gameWorld == nil {
		return nil
	}
	p := h.gameWorld.SessionPlayer(sess)
	if p == nil {
		return nil
	}
	res := h.gameWorld.RemoveCM(sess, id)
	if res.Refusal != "" {
		log.Printf("ClassMastery: %s removing %d refused: %s", p.Name, id, res.Refusal)
		SendErrorMessage(sess, res.Refusal)
		return nil
	}
	if h.store != nil {
		if err := h.store.RemoveCMs(p.CharServerID, res.Removed, MasteryRow(res.Mastery)); err != nil {
			log.Printf("ClassMastery: %s's removal of %d not kept: %v", p.Name, id, err)
		}
	}
	name := fmt.Sprintf("mastery %d", id)
	if res.Rank != nil {
		name = res.Rank.Name
		h.takeAbilities(sess, p, res.Rank.Adds)
		h.giveAbilities(sess, p, res.Rank.Removes)
	}
	SendColoredChat(sess, fmt.Sprintf("You have removed %s: its %d mastery points are gone, to be earned again.", name, res.Paid), 255, 255, 0)
	list := h.gameWorld.CMList(sess, h.cmBookOf(p))
	removed := cmBig(func(w *buffer.Writer) {
		w.WriteByte(cmRespRemoved)
		w.WriteUint32(token)
		w.WriteUint32(uint32(id))
		writeCMState(w, res.State, list)
	})
	return [][]byte{removed, cmPoints(res.Mastery)}
}

// cmPoints is Recv03: the points unspent and spent (entity.SendMasteryPoints'
// message).
func cmPoints(m entity.Mastery) []byte {
	return build(protocol.OpcodeClassMasteryServer, 2+9, func(w *buffer.Writer) {
		w.WriteByte(cmRespPoints)
		w.WriteUint32(uint32(m.Points))
		w.WriteUint32(uint32(m.Spent))
	})
}

// giveAbilities puts a mastery's abilities in the book, live (0x00B3, "You
// have gained a new ability.", as a scribed scroll's), those it holds already
// or whose spell was not found left.
func (h *ClassMasteryHandler) giveAbilities(sess *session.Session, p *entity.Player, abilities []entity.CMAbility) {
	if h.book == nil {
		return
	}
	for _, a := range abilities {
		if a.SpellID <= 0 {
			continue
		}
		book, err := h.book.GetCharacterSpells(p.CharServerID)
		if err != nil {
			return
		}
		known := false
		for _, e := range book {
			if e.Pattern.SpellID == a.SpellID {
				known = true
				break
			}
		}
		if known {
			continue
		}
		if nextDisplayIndex(book) >= 252 { // the book's display indices end at 252
			SendErrorMessage(sess, fmt.Sprintf("Your spell book is full: %s was not added.", a.Name))
			return
		}
		if err := h.book.AddSpell(p.CharServerID, a.SpellID, nextDisplayIndex(book)); err != nil {
			log.Printf("ClassMastery: %s's %s not added: %v", p.Name, a.Name, err)
			continue
		}
		if book, err = h.book.GetCharacterSpells(p.CharServerID); err == nil {
			for i := range book {
				if book[i].Pattern.SpellID == a.SpellID {
					SendAbilityAdd(sess, 0, true, &book[i])
					break
				}
			}
		}
	}
}

// takeAbilities takes a mastery's replaced abilities out of the book; the
// client shows them gone at the next login (no message takes one out live).
func (h *ClassMasteryHandler) takeAbilities(sess *session.Session, p *entity.Player, abilities []entity.CMAbility) {
	if h.store == nil {
		return
	}
	var gone []string
	for _, a := range abilities {
		if a.SpellID <= 0 {
			continue
		}
		if ok, err := h.store.RemoveSpellFromBook(p.CharServerID, a.SpellID); err != nil {
			log.Printf("ClassMastery: %s's %s not taken out: %v", p.Name, a.Name, err)
		} else if ok {
			gone = append(gone, a.Name)
		}
	}
	if len(gone) > 0 {
		SendColoredChat(sess, strings.Join(gone, ", ")+" will be gone from your book at your next login.", 255, 255, 0)
	}
}

// setShare keeps the player's share of experience going to mastery points
// (CLIENT clamps it to 0-100 before sending, and so does this): the share
// now.
func (h *ClassMasteryHandler) setShare(sess *session.Session, pct int) int {
	pct = max(0, min(100, pct))
	if h.gameWorld == nil {
		return pct
	}
	m, ok := h.gameWorld.SetMasteryPercent(sess, pct)
	if !ok {
		return pct
	}
	if p := h.gameWorld.SessionPlayer(sess); p != nil && h.store != nil {
		if err := h.store.SaveMastery(p.CharServerID, MasteryRow(m)); err != nil {
			log.Printf("ClassMastery: %s's share of %d%% not kept: %v", p.Name, m.Percent, err)
		}
	}
	log.Printf("ClassMastery: %s gives %d%% of experience to masteries", sess.RemoteAddr(), m.Percent)
	return m.Percent
}

// MasteryRow is a player's mastery points as charmastery keeps them.
func MasteryRow(m entity.Mastery) database.MasteryRow {
	return database.MasteryRow{Points: m.Points, Spent: m.Spent, Percent: m.Percent, XP: m.XP}
}

// PlayerMastery is a charmastery row as the player holds it.
func PlayerMastery(r database.MasteryRow) entity.Mastery {
	return entity.Mastery{Points: r.Points, Spent: r.Spent, Percent: r.Percent, XP: r.XP}
}

// GMLines is #cm: the masteries of the player the GM targets, else the GM's
// own, with their points and what they give.
func (h *ClassMasteryHandler) GMLines(sess *session.Session, objectID uint32) []string {
	if h.gameWorld == nil {
		return []string{"#cm: no world"}
	}
	who := h.gameWorld.CMSubject(sess, objectID)
	lines := h.gameWorld.CMLines(who)
	if len(lines) == 0 {
		return []string{"#cm: not in the world"}
	}
	return append(lines, h.gameWorld.CMSummary())
}

// GMReset is #cmreset: every mastery of the player the GM targets (else the
// GM's own) taken away and what they cost given back, the abilities they
// added taken out and those they replaced given back, kept, and told to its
// client (Recv06 a rank, then the points). What is said to the GM.
func (h *ClassMasteryHandler) GMReset(sess *session.Session, objectID uint32) string {
	if h.gameWorld == nil {
		return "#cmreset: no world"
	}
	who := h.gameWorld.CMSubject(sess, objectID)
	p := h.gameWorld.SessionPlayer(who)
	if p == nil {
		return "#cmreset: not in the world"
	}
	res := h.gameWorld.ResetCMs(who)
	if res.Refusal != "" {
		return "#cmreset: " + res.Refusal
	}
	if len(res.Removed) == 0 {
		return fmt.Sprintf("%s has no masteries: %s", p.Name, entity.MasteryLine(res.Mastery))
	}
	if h.store != nil {
		if err := h.store.RemoveCMs(p.CharServerID, res.Removed, MasteryRow(res.Mastery)); err != nil {
			log.Printf("GM: #cmreset of '%s' not kept: %v", p.Name, err)
			return fmt.Sprintf("%s's masteries were reset but not kept: %v", p.Name, err)
		}
	}
	list := h.gameWorld.CMList(who, h.cmBookOf(p))
	for _, id := range res.Removed {
		if r := h.gameWorld.CMRankByID(id); r != nil {
			h.takeAbilities(who, p, r.Adds)
			h.giveAbilities(who, p, r.Removes)
		}
		queueReliable(who, protocol.OpcodeClassMasteryServer, cmBig(func(w *buffer.Writer) {
			w.WriteByte(cmRespRemoved)
			w.WriteUint32(0)
			w.WriteUint32(uint32(id))
			writeCMState(w, res.State, list)
		}))
	}
	queueReliable(who, protocol.OpcodeClassMasteryServer, cmPoints(res.Mastery))
	if who != sess {
		SendColoredChat(who, fmt.Sprintf("A GM has reset your masteries: %d mastery points back.", res.Paid), 255, 255, 0)
	}
	log.Printf("GM: masteries of '%s' reset, %d ranks, %d points back (%s)", p.Name, len(res.Removed), res.Paid, entity.MasteryLine(res.Mastery))
	return fmt.Sprintf("%s: %d masteries taken away, %d points back: %s", p.Name, len(res.Removed), res.Paid, entity.MasteryLine(res.Mastery))
}
