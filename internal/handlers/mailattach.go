package handlers

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"sync/atomic"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/economy"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// mailattach.go — what a tell-gram carries: tunar and an item (the owner,
// 2026-11-19: "the mail system should let players mail each other armor,
// tunar, etc."). The grams themselves are mail.go's; the rows are
// database/mail.go's.
//
//   - A gram carries tunar and at most mailAttachItems record (one stack, or
//     part of one) from the sender's pack, worn items not among them: the most
//     the compose window has room to show. It goes to one name, not the
//     sender's own; alts of one account mail each other (that is the point).
//   - What may be mailed is what may be traded, and less: no No Trade item,
//     no No Rent one (nor one norentvanish names), no copy with its own aura
//     (database/instanceaura.go), no record bound to one character
//     (itemowner.go), no heirloom. A Lore item goes only to one who holds none
//     and has none coming, and is checked again at the take.
//   - The send takes the item and the tunar (and the postage, world.mail_postage,
//     0 unset) from the sender and files the gram with them in one transaction;
//     the server holds them (mail_attachments) until the recipient takes them,
//     in one transaction too: the item into the pack, the tunar into the purse.
//     A pack with no room (or a Lore item already held) refuses the take and the
//     attachment stays on the gram.
//   - Deleting a gram that still carries something returns it to its sender:
//     a gram back in the sender's box, "Returned: ...", carrying it (its
//     sender gone, it is dropped, logged). A returned gram is not returned
//     again: it cannot be deleted until what it carries is taken.
//   - A Hardcore character uses no mail (mail.go): it sends nothing and nothing
//     is sent it, and it takes nothing from a gram it held from before.
//   - Every send, take, return and drop is logged (mail_attachment_log, the GM
//     panel's "Mail attachments log"; the server's log), the tunar counted by
//     the economy (economy.Mail, a transfer, at the send and at a return;
//     economy.Postage, a sink).
//   - On the wire, protocol.OpcodeMailAttach: offered with the mail forms; a
//     client that answers it (vanilla) is told what each gram carries and
//     takes it; one that never does (a PS2, an older vanilla) is told in the
//     gram's text that something is attached, and that deleting the gram sends
//     it back. A PS2 has no mail window, and the mail has no chat commands, so
//     there is no command to take with.

// mailAttachItems is how many records one gram may carry.
const mailAttachItems = 1

// mailPostage is world.mail_postage: the tunar a gram carrying something
// costs its sender, on top of what it carries.
var mailPostage atomic.Int64

// SetMailPostage sets world.mail_postage (cmd/gameserver), at least 0.
func SetMailPostage(n int) { mailPostage.Store(int64(max(0, n))) }

// mailAttachOn is whether this server's mail carries attachments: a database
// to keep them and the item windows to move them (registry.go). The offer goes
// out only then.
var mailAttachOn atomic.Bool

// The sub-ops of protocol.OpcodeMailAttach.
const (
	mailAttachOffer   byte = 0 // out: the offer; in: the answer
	mailAttachSend    byte = 1 // in: a live send with attachments
	mailAttachShow    byte = 2 // out: what a gram carries
	mailAttachMarks   byte = 3 // out: the inbox's grams that carry something
	mailAttachTake    byte = 4 // in: take; out: its answer
	mailAttachRefused byte = 5 // out: why a send was refused
)

func mailAttachMsg(sub byte, size int, fill func(w *buffer.Writer)) []byte {
	return build(protocol.OpcodeMailAttach, size, func(w *buffer.Writer) {
		w.WriteByte(sub)
		if fill != nil {
			fill(w)
		}
	})
}

// sendMailAttachOffer offers the attachments (a client that shows them
// answers); nothing on a server whose mail carries none.
func sendMailAttachOffer(sess *session.Session) {
	if !mailAttachOn.Load() {
		return
	}
	queueReliable(sess, protocol.OpcodeMailAttach, mailAttachMsg(mailAttachOffer, 16, func(w *buffer.Writer) {
		w.WriteByte(1)
		w.WriteByte(mailAttachItems)
		w.WriteUint32(uint32(mailPostage.Load()))
	}))
}

// HandleMailAttach is protocol.OpcodeMailAttach from a client.
func (h *MailHandler) HandleMailAttach(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 1 {
		return nil
	}
	r := buffer.NewReader(data[1:])
	switch data[0] {
	case mailAttachOffer:
		v, err := r.ReadByte()
		if err != nil {
			return nil
		}
		sess.SetMailAttach(v != 0)
		log.Printf("MailAttach: %s %s", sess.RemoteAddr(), map[bool]string{true: "shows and takes what a gram carries", false: "does not"}[v != 0])
		return nil
	}
	if h.gameWorld == nil {
		return nil
	}
	me := h.gameWorld.SessionPlayer(sess)
	if me == nil {
		return nil
	}
	switch data[0] {
	case mailAttachSend:
		req, ok := readLiveSend(r)
		if !ok {
			return nil
		}
		tunar, err := r.ReadUint32()
		if err != nil {
			return nil
		}
		n, err := r.Read7BitEncodedInt64()
		if err != nil || n < 0 || n > 16 {
			return nil
		}
		var items []mailItemReq
		for i := 0; i < int(n); i++ {
			key, e1 := r.ReadUint32()
			count, e2 := r.Read7BitEncodedInt64()
			if e1 != nil || e2 != nil {
				return nil
			}
			items = append(items, mailItemReq{key: key, n: int(count)})
		}
		req.subject, req.text = consoleLine(req.subject, gramSubjectMost), consoleBlock(req.text, gramTextMost)
		h.sendAttached(sess, me, req, int(int32(tunar)), items)
	case mailAttachTake:
		id, err := r.ReadUint32()
		if err != nil {
			return nil
		}
		ok := h.take(sess, me, id)
		queueReliable(sess, protocol.OpcodeMailAttach, mailAttachMsg(mailAttachTake, 16, func(w *buffer.Writer) {
			w.WriteUint32(id)
			w.WriteByte(b2u8(ok))
		}))
	}
	return nil
}

// mailItemReq is a record a send names: its key in the pack and how many.
type mailItemReq struct {
	key uint32
	n   int
}

// refuseSend tells the client why a send with attachments went nowhere, and
// answers the send (12: the compose stays, to fix it).
func refuseSend(sess *session.Session, why string) {
	queueReliable(sess, protocol.OpcodeMailAttach, mailAttachMsg(mailAttachRefused, 1100, func(w *buffer.Writer) {
		writeWideString(w, why, 1024)
	}))
	gramReply(sess, 1, func(w *buffer.Writer) { w.WriteByte(12) })
}

// unmailable is why a record cannot go on a gram, "" when it can.
func (h *MailHandler) unmailable(row *database.CharInventoryEntry) string {
	p := &row.Pattern
	switch {
	case row.EquipLoc != -1:
		return fmt.Sprintf("You are wearing %s: only what is in your pack can be mailed.", p.ItemName)
	case p.Trade != 0:
		return fmt.Sprintf("%s is No Trade and cannot be mailed.", p.ItemName)
	case p.Rent == 0:
		return fmt.Sprintf("%s is No Rent and cannot be mailed.", p.ItemName)
	case row.AuraTag != "":
		return fmt.Sprintf("%s has an aura of its own and cannot be mailed: trade it in person.", p.ItemName)
	}
	if _, bound := itemOwnerOf(p.PatternID); bound {
		return fmt.Sprintf("%s is made for one character alone and cannot be mailed.", p.ItemName)
	}
	if h.db != nil {
		if gone, err := h.db.VanishesAtLogout(p.PatternID); err != nil || gone {
			return fmt.Sprintf("%s is No Rent and cannot be mailed.", p.ItemName)
		}
		hl, err := h.db.Heirlooms()
		if err != nil {
			return "The post cannot take that just now."
		}
		if _, ok := hl[p.PatternID]; ok {
			return fmt.Sprintf("%s is an heirloom and cannot be mailed.", p.ItemName)
		}
	}
	return ""
}

// sendAttached is a live send carrying tunar and items: checked whole, then
// taken and filed in one transaction (database.SendGramAttached), the
// sender's pack and purse told, the recipient told it came. Any refusal sends
// nothing and takes nothing.
func (h *MailHandler) sendAttached(sess *session.Session, me *entity.Player, req gramSend, tunar int, items []mailItemReq) {
	if me.HardcoreBound() {
		SendErrorMessage(sess, hcNoMailWords)
		gramReply(sess, 1, func(w *buffer.Writer) { w.WriteByte(12) })
		return
	}
	if len(items) == 0 && tunar == 0 {
		// nothing attached: a plain send
		out := h.deliverFrom(me.Name, me.CharServerID, req.to, req.subject, req.text, sess)
		gramReply(sess, 1, out.writeLive)
		return
	}
	if !mailAttachOn.Load() || h.db == nil || h.items == nil || h.items.inv == nil {
		refuseSend(sess, "This server's mail carries no items or tunar.")
		return
	}
	names := gramRecipients(req.to)
	switch {
	case len(names) == 0:
		gramReply(sess, 1, func(w *buffer.Writer) { w.WriteByte(gramUnknown); w.Write7BitEncodedInt64(0) })
		return
	case len(names) > 1:
		refuseSend(sess, "A mail carrying items or tunar goes to one name only.")
		return
	case tunar < 0:
		refuseSend(sess, "That is not an amount of tunar.")
		return
	case len(items) > mailAttachItems:
		refuseSend(sess, fmt.Sprintf("A mail carries at most %d item.", mailAttachItems))
		return
	}
	to, ok, err := h.db.GramRecipientNamed(names[0])
	if err != nil {
		log.Printf("MailAttach: %s's recipient %q: %v", me.Name, names[0], err)
		refuseSend(sess, "The post cannot take that just now.")
		return
	}
	if !ok {
		gramReply(sess, 1, func(w *buffer.Writer) {
			w.WriteByte(gramUnknown)
			w.Write7BitEncodedInt64(1)
			writeString8(w, names[0], 24)
		})
		return
	}
	switch {
	case to.ServerID == me.CharServerID:
		refuseSend(sess, "You cannot mail items or tunar to yourself.")
		return
	case h.hardcoreNamed(to.Name):
		refuseSend(sess, fmt.Sprintf(hcNoMailThemWords, to.Name))
		return
	}
	postage := int(mailPostage.Load())
	if tunar+postage > me.Tunar {
		if postage > 0 {
			refuseSend(sess, fmt.Sprintf("You do not carry %d tunar (the postage is %d).", tunar+postage, postage))
		} else {
			refuseSend(sess, fmt.Sprintf("You do not carry %d tunar.", tunar))
		}
		return
	}
	var take *database.GramItemTake
	var at int // the record's list index
	var whole bool
	inv := sessionInvList(sess)
	for _, it := range items {
		if inv == nil {
			refuseSend(sess, "The post cannot take that just now.")
			return
		}
		at = inv.index(it.key)
		rec, ok := inv.at(at)
		if !ok || it.n < 1 || it.n > rec.qty {
			refuseSend(sess, "You do not have that many of that item.")
			return
		}
		row, err := h.items.inv.GetCharItem(me.CharServerID, rec.itemID)
		if err != nil {
			refuseSend(sess, "You do not have that item.")
			return
		}
		if why := h.unmailable(&row); why != "" {
			refuseSend(sess, why)
			return
		}
		if row.Location != database.LocPack {
			refuseSend(sess, "Only what is in your pack can be mailed.")
			return
		}
		if row.Pattern.Lore != 0 {
			held, err := h.db.LoreHeldOrComing(to.ServerID, row.Pattern.PatternID)
			if err != nil || held {
				refuseSend(sess, fmt.Sprintf("%s already has %s, which is Lore: one at a time.", to.Name, row.Pattern.ItemName))
				return
			}
		}
		take = &database.GramItemTake{ItemID: rec.itemID, N: it.n, PatternID: row.Pattern.PatternID, ItemName: row.Pattern.ItemName}
		whole = it.n == rec.qty
	}

	h.mu.Lock()
	key := strings.ToLower(to.Name)
	box := h.boxes[key]
	unread := 0
	for _, g := range box {
		if !g.read && strings.EqualFold(g.from, me.Name) {
			unread++
		}
	}
	reason := byte(0)
	switch {
	case len(box) >= mailboxMax:
		reason = gramBoxFull
	case unread >= unreadPerSender:
		reason = gramTooMany
	}
	if reason != 0 {
		h.mu.Unlock()
		gramReply(sess, 1, gramOutcome{failed: []string{to.Name}, reasons: []byte{reason}}.writeLive)
		return
	}
	g := &gram{from: me.Name, fromID: me.CharServerID, ownerID: to.ServerID, to: []string{to.Name},
		subject: req.subject, text: req.text, date: mailNow()}
	x := &database.GramSendAttached{Gram: g.row(key), ToName: to.Name, Purse: me.Tunar - tunar - postage,
		Tunar: tunar, Postage: postage, Item: take}
	if err := h.items.inv.SendGramAttached(x); err != nil {
		h.mu.Unlock()
		log.Printf("MailAttach: %s to %s: %v", me.Name, to.Name, err)
		if errors.Is(err, database.ErrGramItemGone) {
			refuseSend(sess, "You do not have that many of that item.")
		} else {
			refuseSend(sess, "The post cannot take that just now.")
		}
		return
	}
	g.id = x.Gram.ID
	if atts, err := h.db.GramAttachments(g.id); err == nil {
		g.atts = atts
	} else {
		log.Printf("MailAttach: gram %d's attachments: %v", g.id, err)
	}
	h.boxes[key] = append(h.boxes[key], g)
	if take != nil && inv.took(at, take.N) {
		queueReliable(sess, protocol.OpcodeRemoveInvItem, removeInvItemPayload(at, 0, take.N))
	}
	if tunar > 0 || postage > 0 {
		me.Tunar = x.Purse
		queueReliable(sess, protocol.OpcodePlayerTunar, tunarPayload(me.Tunar))
	}
	h.mu.Unlock()

	if tunar > 0 {
		economy.Moved(economy.Mail, me.CharServerID, me.Name, to.ServerID, to.Name, tunar)
	}
	if postage > 0 {
		economy.Spent(economy.Postage, me.CharServerID, me.Name, postage)
	}
	what := attachWords(g.atts)
	log.Printf("MailAttach: %s -> %s gram %d carrying %s (postage %d, whole record %v)", me.Name, to.Name, g.id, what, postage, whole)
	gramReply(sess, 1, func(w *buffer.Writer) { w.WriteByte(gramSent) })
	if postage > 0 {
		SendColoredChat(sess, fmt.Sprintf("The postage was %d tunar.", postage), 255, 255, 0)
	}
	h.tellArrived(to.Name)
}

// take gives the player what a gram in its box carries: refused (told why,
// nothing moved) when it is Hardcore, the gram carries nothing or was not sent
// to this character, the pack has no room, or a Lore item is already held.
func (h *MailHandler) take(sess *session.Session, me *entity.Player, id uint32) bool {
	if me.HardcoreBound() {
		SendErrorMessage(sess, hcNoMailWords)
		return false
	}
	if h.db == nil || h.items == nil || h.items.inv == nil {
		return false
	}
	mine := strings.ToLower(me.Name)
	h.mu.Lock()
	defer h.mu.Unlock()
	var g *gram
	for _, x := range h.boxes[mine] {
		if x.id == id {
			g = x
		}
	}
	if g == nil || len(g.atts) == 0 {
		SendErrorMessage(sess, "Nothing is attached to that mail.")
		return false
	}
	if g.ownerID != 0 && g.ownerID != me.CharServerID {
		SendErrorMessage(sess, "What that mail carries was sent to another character of your name.")
		return false
	}
	inv := sessionInvList(sess)
	if inv == nil {
		return false
	}
	t := database.GramTake{GramID: g.id, ToID: me.CharServerID, ToName: me.Name, Purse: me.Tunar, At: time.Now()}
	var pat *database.ItemPatternRecord
	var item database.GramAttachment
	for _, a := range g.atts {
		t.IDs = append(t.IDs, a.ID)
		if !a.Item() {
			continue
		}
		p, err := database.NewItemPatternStore(h.db).GetItemPattern(a.PatternID)
		if err != nil || p == nil {
			log.Printf("MailAttach: gram %d's record %d: %v", g.id, a.PatternID, err)
			SendErrorMessage(sess, "The post cannot give that just now.")
			return false
		}
		h.db.ViewPattern(me.CharServerID, p)
		if p.Lore != 0 && inv.loreHeld(p.PatternID) {
			SendErrorMessage(sess, fmt.Sprintf(loreLine, p.ItemName))
			return false
		}
		if inv.loreRoom(p, inv.wouldAdd(p.PatternID, a.Stack, p.MaxStack)) < a.Stack ||
			len(inv.roomFor(nil, []invGive{{patternID: p.PatternID, n: a.Stack, maxStack: p.MaxStack}})) > 0 {
			SendErrorMessage(sess, fmt.Sprintf("Your pack has no room for %s: it stays on the mail.", a.ItemName))
			return false
		}
		if i := inv.stackOnto(p.PatternID, p.MaxStack); i >= 0 {
			rec, _ := inv.at(i)
			t.Onto = rec.itemID
		}
		pat, item = p, a
	}
	got, err := h.items.inv.TakeGramAttachments(t)
	if err != nil {
		log.Printf("MailAttach: %s taking gram %d: %v", me.Name, g.id, err)
		if errors.Is(err, database.ErrGramTaken) {
			g.atts = nil
			SendErrorMessage(sess, "Nothing is attached to that mail any more.")
		} else {
			SendErrorMessage(sess, "The post cannot give that just now.")
		}
		return false
	}
	what := attachWords(g.atts)
	g.atts = nil
	if got.Tunar > 0 {
		me.Tunar = got.Purse
		queueReliable(sess, protocol.OpcodePlayerTunar, tunarPayload(me.Tunar))
	}
	if got.Item != nil && pat != nil {
		key, _ := inv.add(got.ItemID, pat.PatternID, item.Stack, pat.MaxStack)
		e := database.CharInventoryEntry{ItemID: got.ItemID, StackLeft: item.Stack, RemainHP: item.RemainHP, Charges: item.Charges,
			EquipLoc: -1, Location: -1, Pattern: *pat}
		queueReliable(sess, protocol.OpcodeAddInvItem, addInvItemPayload(&e, key, readsCapes(sess)))
	}
	h.sendAttachmentsLocked(sess, g)
	SendColoredChat(sess, fmt.Sprintf("You take %s from the mail.", what), 255, 255, 0)
	log.Printf("MailAttach: %s took %s from gram %d (from %s)", me.Name, what, g.id, g.from)
	return true
}

// remove deletes gram id from me's box: 13 when it went. One that carries
// something goes back to its sender (database.ReturnGram) in a gram filed in
// the sender's box, said in the line returned, the sender's name returned to
// be told; a returned one is not returned again, but kept (refused) until what
// it carries is taken.
func (h *MailHandler) remove(me *entity.Player, id uint32) (status byte, said, back string) {
	mine := strings.ToLower(me.Name)
	h.mu.Lock()
	defer h.mu.Unlock()
	box := h.boxes[mine]
	at := -1
	for i, g := range box {
		if g.id == id {
			at = i
			break
		}
	}
	if at < 0 {
		return 0, "", ""
	}
	g := box[at]
	drop := func() { h.boxes[mine] = append(box[:at:at], box[at+1:]...) }
	if len(g.atts) == 0 {
		if h.db != nil {
			if err := h.db.DeleteGram(g.id); err != nil {
				log.Printf("Mail: deleting gram %d: %v", g.id, err)
				return 0, "", ""
			}
		}
		drop()
		return 13, "", ""
	}
	if g.returned || h.db == nil {
		return 0, "Take what this mail carries before you delete it.", ""
	}
	var backRow *database.Gram
	var sender database.GramRecipient
	if r, ok, err := h.db.GramRecipientNamed(g.from); err != nil {
		log.Printf("Mail: gram %d's sender %q: %v", g.id, g.from, err)
		return 0, "", ""
	} else if ok && (g.fromID == 0 || r.ServerID == g.fromID) {
		sender = r
		subject := consoleLine("Returned: "+g.subject, gramSubjectMost)
		text := fmt.Sprintf("%s did not take what your mail carried, and it is back with you: take it from this mail.", me.Name)
		backG := &gram{from: me.Name, fromID: me.CharServerID, ownerID: r.ServerID, to: []string{r.Name},
			subject: subject, text: text, date: mailNow(), returned: true}
		row := backG.row(r.Name)
		backRow = &row
	}
	what := attachWords(g.atts)
	if err := h.db.ReturnGram(g.id, mine, backRow); err != nil {
		log.Printf("Mail: returning gram %d: %v", g.id, err)
		return 0, "", ""
	}
	drop()
	tunar := 0
	for _, a := range g.atts {
		tunar += a.Tunar
	}
	if backRow == nil {
		log.Printf("MailAttach: %s deleted gram %d from %s, who is gone: %s dropped", me.Name, g.id, g.from, what)
		return 13, fmt.Sprintf("%s is gone: what the mail carried went with it.", g.from), ""
	}
	bg := gramOf(*backRow)
	bg.atts, _ = h.db.GramAttachments(bg.id)
	skey := strings.ToLower(sender.Name)
	h.boxes[skey] = append(h.boxes[skey], bg)
	if tunar > 0 {
		economy.Moved(economy.Mail, me.CharServerID, me.Name, sender.ServerID, sender.Name, tunar)
	}
	log.Printf("MailAttach: %s deleted gram %d: %s returned to %s (gram %d)", me.Name, g.id, what, sender.Name, bg.id)
	return 13, fmt.Sprintf("What the mail carried went back to %s.", sender.Name), sender.Name
}

// attachWords says what attachments are: "500 tunar and 3 Bone Chips".
func attachWords(atts []database.GramAttachment) string {
	var parts []string
	for _, a := range atts {
		if a.Item() {
			if a.Stack > 1 {
				parts = append(parts, fmt.Sprintf("%d %s", a.Stack, a.ItemName))
			} else {
				parts = append(parts, a.ItemName)
			}
		} else if a.Tunar > 0 {
			parts = append(parts, fmt.Sprintf("%d tunar", a.Tunar))
		}
	}
	if len(parts) == 0 {
		return "nothing"
	}
	return strings.Join(parts, " and ")
}

// withAttachNote is a gram's text for a client that cannot take what it
// carries: the text, cut to leave room, and a line saying what is attached.
// No '|' (a PS2 dies on a run of them: consoletext.go).
func withAttachNote(text string, atts []database.GramAttachment) string {
	note := consoleLine(fmt.Sprintf("Attached: %s. Take it with a newer client's mail window; deleting this mail sends it back.",
		attachWords(atts)), gramTextMost)
	room := gramTextMost - len([]rune(note)) - 2
	if r := []rune(text); len(r) > room {
		text = string(r[:max(0, room)])
	}
	if text == "" {
		return note
	}
	return text + "\n\n" + note
}

// sendMarks tells a client that takes attachments which grams of its inbox
// carry something (after the inbox).
func (h *MailHandler) sendMarks(sess *session.Session, box []*gram) {
	if !sess.MailAttach() {
		return
	}
	h.mu.Lock()
	type mark struct {
		id           uint32
		tunar, items int
	}
	var marks []mark
	for _, g := range box {
		if len(g.atts) == 0 {
			continue
		}
		m := mark{id: g.id}
		for _, a := range g.atts {
			if a.Item() {
				m.items++
			} else {
				m.tunar += a.Tunar
			}
		}
		marks = append(marks, m)
	}
	h.mu.Unlock()
	queueReliable(sess, protocol.OpcodeMailAttach, mailAttachMsg(mailAttachMarks, 16+9*len(marks), func(w *buffer.Writer) {
		w.Write7BitEncodedInt64(int64(len(marks)))
		for _, m := range marks {
			w.WriteUint32(m.id)
			w.WriteUint32(uint32(m.tunar))
			w.WriteByte(byte(m.items))
		}
	}))
}

// sendAttachments tells a client that takes them what a gram carries (after
// it is opened), nothing included.
func (h *MailHandler) sendAttachments(sess *session.Session, id uint32, atts []database.GramAttachment) {
	if !sess.MailAttach() {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	g := &gram{id: id, atts: atts}
	for _, x := range h.boxes[strings.ToLower(h.nameOf(sess))] {
		if x.id == id {
			g = x
		}
	}
	h.sendAttachmentsLocked(sess, g)
}

// nameOf is the session's player's name ("" none).
func (h *MailHandler) nameOf(sess *session.Session) string {
	if h.gameWorld == nil {
		return ""
	}
	if p := h.gameWorld.SessionPlayer(sess); p != nil {
		return p.Name
	}
	return ""
}

// sendAttachmentsLocked is sub-op 2 for g, with mu held.
func (h *MailHandler) sendAttachmentsLocked(sess *session.Session, g *gram) {
	if !sess.MailAttach() {
		return
	}
	tunar := 0
	var recs []database.CharInventoryEntry
	for _, a := range g.atts {
		if !a.Item() {
			tunar += a.Tunar
			continue
		}
		e := database.CharInventoryEntry{ItemID: int(a.ID), StackLeft: a.Stack, RemainHP: a.RemainHP, Charges: a.Charges,
			EquipLoc: -1, Location: -1}
		if h.db != nil {
			if p, err := database.NewItemPatternStore(h.db).GetItemPattern(a.PatternID); err == nil && p != nil {
				e.Pattern = *p
			}
		}
		if e.Pattern.PatternID == 0 {
			e.Pattern.PatternID, e.Pattern.ItemName, e.Pattern.MaxStack = a.PatternID, a.ItemName, max(1, a.Stack)
		}
		recs = append(recs, e)
	}
	capes := readsCapes(sess)
	queueReliable(sess, protocol.OpcodeMailAttach, mailAttachMsg(mailAttachShow, 4096, func(w *buffer.Writer) {
		w.WriteUint32(g.id)
		w.WriteUint32(uint32(tunar))
		w.Write7BitEncodedInt64(int64(len(recs)))
		for i := range recs {
			writeSingleItem(w, &recs[i], i, capes)
		}
		w.WriteByte(b2u8(g.returned))
	}))
}
