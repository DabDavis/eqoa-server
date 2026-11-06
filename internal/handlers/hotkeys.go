package handlers

import (
	"log"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// hotkeys.go — the L2 menu's quick chat kept for each character (petition 1003,
// Waldorf: "L2 changes to hotkeys don't save after logging out"; the user,
// 2026-11-06: "build the hotkey saving"). CLIENT and vanilla both tell the server
// each change (VIWndMenu4's editor, quickchat.rs), which nothing here read, so the
// MemoryDump sent the default sets at every login:
//
//	0x006C {zz set, zz slot, wide text, wide title}: one choice (Send006C)
//	0x006A {zz set, 4 x {zz slot, wide text, wide title}}: a set made (Send006A)
//	0x006B {zz set}: a set taken away (Send006B), a choice no more a branch
//
// Each goes into table hotkeys at once (database/hotkeys.go), so a crash or a
// restart loses none. A set or slot CLIENT's own table cannot hold is dropped, and
// the words are cut to what its record keeps.

// hotkeyText reads a choice's two strings, cut to what CLIENT's record keeps.
func hotkeyText(r *buffer.Reader) (database.HotkeyPair, bool) {
	text, ok := readWide(r)
	if !ok {
		return database.HotkeyPair{}, false
	}
	title, ok := readWide(r)
	if !ok {
		return database.HotkeyPair{}, false
	}
	return database.HotkeyPair{Text: cutWide(text, database.HotkeyTextMax), Title: cutWide(title, database.HotkeyTitleMax)}, true
}

// hotkeyOwner is the character a change is for, 0 for none.
func (h *CharacterHandler) hotkeyOwner(sess *session.Session) uint32 {
	if h.store == nil || h.gameWorld == nil {
		return 0
	}
	if p := h.gameWorld.SessionPlayer(sess); p != nil {
		return p.CharServerID
	}
	return 0
}

// HandleHotkeyChoice is 0x006C.
func (h *CharacterHandler) HandleHotkeyChoice(sess *session.Session, msg *protocol.Message) error {
	serverID := h.hotkeyOwner(sess)
	if serverID == 0 {
		return nil
	}
	r := buffer.NewReader(msg.Data())
	id, err1 := r.Read7BitEncodedInt64()
	slot, err2 := r.Read7BitEncodedInt64()
	p, ok := hotkeyText(r)
	if err1 != nil || err2 != nil || !ok {
		log.Printf("Hotkeys: a short 0x006C from character %d: %x", serverID, msg.Data())
		return nil
	}
	if err := h.store.DB().SetHotkeyChoice(serverID, defaultHotkeySets(), int(id), int(slot), p); err != nil {
		log.Printf("Hotkeys: character %d set %d choice %d: %v", serverID, id, slot, err)
	}
	return nil
}

// HandleHotkeySet is 0x006A.
func (h *CharacterHandler) HandleHotkeySet(sess *session.Session, msg *protocol.Message) error {
	serverID := h.hotkeyOwner(sess)
	if serverID == 0 {
		return nil
	}
	r := buffer.NewReader(msg.Data())
	id, err := r.Read7BitEncodedInt64()
	if err != nil {
		log.Printf("Hotkeys: a short 0x006A from character %d: %x", serverID, msg.Data())
		return nil
	}
	s := database.HotkeySet{ID: int(id)}
	for range s.Pairs {
		slot, err := r.Read7BitEncodedInt64()
		p, ok := hotkeyText(r)
		if err != nil || !ok {
			log.Printf("Hotkeys: a short 0x006A from character %d: %x", serverID, msg.Data())
			return nil
		}
		if slot >= 0 && slot < int64(len(s.Pairs)) { // CLIENT's own loader skips any other
			s.Pairs[slot] = p
		}
	}
	if err := h.store.DB().SetHotkeySet(serverID, defaultHotkeySets(), s); err != nil {
		log.Printf("Hotkeys: character %d set %d: %v", serverID, id, err)
	}
	return nil
}

// HandleHotkeyDrop is 0x006B.
func (h *CharacterHandler) HandleHotkeyDrop(sess *session.Session, msg *protocol.Message) error {
	serverID := h.hotkeyOwner(sess)
	if serverID == 0 {
		return nil
	}
	id, err := buffer.NewReader(msg.Data()).Read7BitEncodedInt64()
	if err != nil {
		log.Printf("Hotkeys: a short 0x006B from character %d: %x", serverID, msg.Data())
		return nil
	}
	if err := h.store.DB().DropHotkeySet(serverID, defaultHotkeySets(), int(id)); err != nil {
		log.Printf("Hotkeys: character %d dropping set %d: %v", serverID, id, err)
	}
	return nil
}

// hotkeysOf is what the character's MemoryDump sends: its own sets, or the
// defaults while it has changed none (or its rows cannot be read).
func (h *CharacterHandler) hotkeysOf(serverID uint32) []database.HotkeySet {
	if h.store != nil {
		sets, err := h.store.DB().Hotkeys(serverID)
		if err != nil {
			log.Printf("Hotkeys: character %d: %v", serverID, err)
		} else if len(sets) > 0 {
			return sets
		}
	}
	return defaultHotkeySets()
}

// defaultHotkeySets are defaultHotkeys as sets.
func defaultHotkeySets() []database.HotkeySet {
	out := make([]database.HotkeySet, len(defaultHotkeys))
	for i, hk := range defaultHotkeys {
		out[i] = database.HotkeySet{ID: int(hk.dirCode), Pairs: [4]database.HotkeyPair{
			{Text: hk.nText, Title: hk.nTitle}, {Text: hk.wText, Title: hk.wTitle},
			{Text: hk.eText, Title: hk.eTitle}, {Text: hk.sText, Title: hk.sTitle},
		}}
	}
	return out
}

// writeHotkeys writes the MemoryDump's sets, as CLIENT's VIRec6A reads them:
// count(zigzag), then each set's id(zigzag) and its four choices, each its
// slot(zigzag: the bytes 0, 2, 4, 6), its text and its title.
func writeHotkeys(writer *buffer.Writer, sets []database.HotkeySet) {
	writer.Write7BitEncodedInt64(int64(len(sets)))
	for _, s := range sets {
		writer.Write7BitEncodedInt64(int64(s.ID))
		for slot, p := range s.Pairs {
			writer.Write7BitEncodedInt64(int64(slot))
			writer.WriteStringUnicode(p.Text)
			writer.WriteStringUnicode(p.Title)
		}
	}
}
