package handlers

import (
	"log"
	"sync"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The bank window. The client has the bank's contents already — the
// MemoryDump carries the list (VIBankControl +8) and the bank's tunar
// (VIPlayer +0x38) — so the server only opens the window and carries out
// moves. No capture holds a bank session: the formats are CLIENT's [A].
//
//	C>S 0x124D  u32 the banker                         InteractWithTarget [V]
//	S>C 0x124D  (empty): open                          the dispatcher's case: VIWndGame
//	            0x00C55888 (named BankClose, provisionally — it plays sound 15, makes the bank
//	            panel +0x6AC the active window, mode 11) and VIBankControl::Close (pending ← 0)
//	C>S 0x1254  u32 the banker, u8 from the bank, u32 key, zz count     func_00BB9A18
//	C>S 0x1255  u32 the banker, u8 0 deposit / 1 withdraw, zz amount    func_00BB9B78
//	S>C 0x1250  u8 bank index, zz count: the bank's Took               func_00BB9F68
//	S>C 0x1251  a record: the bank's AddItem(maxStack, 40)             func_00BB9EE8
//	S>C 0x1253  zz: the bank's tunar (SetPlayerP38)                    VIBankControl::Recv
//
// 0x1250, 0x1251 and 0x1253 each clear the bank's pending flag (+0x78C8,
// which the two requests set to 2 and 1); an inventory move pairs one with
// 0x003C or 0x003B, a tunar move with 0x0052. The direction of 0x1255 is
// the tunar window's (func_00C196B0): mode 1 deposits, bounded by the purse,
// and sends 0; mode 2 withdraws, bounded by the bank, and sends 1.

// bankMu serializes the bank: the session's copy of its list (invList), its
// tunar as the game holds it (Player.BankTunar) and the rows of both, so a
// move of the bank's (here), the auction house's (auction.go) and a
// MemoryDump's reading of them (character.go) never interleave. Nothing takes
// it with the World's lock held; the auction house takes its own first.
var bankMu sync.Mutex

// bankTookPayload is 0x1250: n off the bank record at index.
func bankTookPayload(index, n int) []byte {
	return build(protocol.OpcodeRemoveBankItem, 16, func(w *buffer.Writer) {
		w.WriteByte(byte(index))
		w.Write7BitEncodedInt64(int64(n))
	})
}

// bankAddPayload is 0x1251: one record for the bank's list.
func bankAddPayload(e *database.CharInventoryEntry, key uint32) []byte {
	return build(protocol.OpcodeAddBankItem, 1024, func(w *buffer.Writer) {
		writeSingleItem(w, e, int(key))
	})
}

// bankTunarPayload is 0x1253: the bank's tunar.
func bankTunarPayload(tunar int) []byte {
	return build(protocol.OpcodeConfirmBankTunar, 16, func(w *buffer.Writer) {
		w.Write7BitEncodedInt64(int64(tunar))
	})
}

// HandleBankOpen is 0x124D, u32 the banker: the same opcode back opens the
// window.
func (h *NPCWindowHandler) HandleBankOpen(sess *session.Session, msg *protocol.Message) error {
	id, err := buffer.NewReader(msg.Data()).ReadUint32()
	if err != nil {
		return nil
	}
	player, npc, ok := h.windowNPC(sess, id, npcFlagBank, "Bank")
	if !ok {
		refuse(sess)
		return nil
	}
	// a quest's step to do at it is done first (questevents.go: CLIENT
	// never talks to an NPC with a window)
	if questsAtWindow(sess, player, npc) {
		return nil
	}
	queueReliable(sess, protocol.OpcodeBankUI, build(protocol.OpcodeBankUI, 2, nil))
	log.Printf("Bank: %s opened %q", player.Name, npc.Name)
	// what the auction house holds for it, filed if there is room now
	if h.auction != nil {
		h.auction.bankOpened(sess, player)
	}
	return nil
}

// HandleBankItem is 0x1254: an item into the bank (from the inventory, the
// byte 0) or out of it (non-zero), by its key in the list it leaves.
func (h *NPCWindowHandler) HandleBankItem(sess *session.Session, msg *protocol.Message) error {
	r := buffer.NewReader(msg.Data())
	id, e1 := r.ReadUint32()
	fromBank, e2 := r.ReadByte()
	key, e3 := r.ReadUint32()
	n, e4 := r.Read7BitEncodedInt64()
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
		return nil
	}
	if !h.bankItem(sess, id, fromBank != 0, key, int(n)) {
		refuse(sess)
	}
	return nil
}

func (h *NPCWindowHandler) bankItem(sess *session.Session, id uint32, fromBank bool, key uint32, n int) bool {
	player, _, ok := h.windowNPC(sess, id, npcFlagBank, "BankItem")
	inv, bank := sessionInvList(sess), sessionBankList(sess)
	if !ok || h.inv == nil || inv == nil || bank == nil || n < 1 {
		return false
	}
	bankMu.Lock()
	defer bankMu.Unlock()
	src, dst, dstLoc := inv, bank, 1
	if fromBank {
		src, dst, dstLoc = bank, inv, 0
	}
	i := src.index(key)
	rec, found := src.at(i)
	if !found || rec.qty < n {
		log.Printf("BankItem: %s moves %d of key %d, which the list does not hold", player.Name, n, key)
		return false
	}
	row, err := h.inv.GetCharItem(player.CharServerID, rec.itemID)
	if err != nil {
		log.Printf("BankItem: %s: %v", player.Name, err)
		return false
	}
	if row.EquipLoc != -1 {
		log.Printf("BankItem: %s tried to bank %q while wearing it", player.Name, row.Pattern.ItemName)
		return false
	}
	p := &row.Pattern
	took := dst.wouldAdd(p.PatternID, n, p.MaxStack)
	if took == 0 {
		log.Printf("BankItem: %s: no room for %q", player.Name, p.ItemName)
		return false
	}
	var out *database.CharInventoryEntry
	var dstKey uint32
	if took == rec.qty && dst.stackOnto(p.PatternID, p.MaxStack) < 0 {
		// The whole record crosses as it is: the row changes location.
		if err := h.inv.MoveCharItem(player.CharServerID, rec.itemID, dstLoc); err != nil {
			log.Printf("BankItem: %s: %v", player.Name, err)
			return false
		}
		src.took(i, took)
		dstKey, _ = dst.add(rec.itemID, p.PatternID, took, p.MaxStack)
		w := wireRecord(row)
		w.StackLeft = took
		out = &w
	} else {
		// Part of a stack, or onto a stack already there.
		if err := h.takeItem(player, src, i, rec, took); err != nil {
			log.Printf("BankItem: %s: %v", player.Name, err)
			return false
		}
		if out, dstKey, err = h.giveItem(player, dst, row, took, dstLoc); err != nil {
			log.Printf("BankItem: %s: %v (%d %q left the list and arrived nowhere)", player.Name, err, took, p.ItemName)
			return false
		}
	}
	if fromBank {
		queueReliable(sess, protocol.OpcodeRemoveBankItem, bankTookPayload(i, took))
		queueReliable(sess, protocol.OpcodeAddInvItem, addInvItemPayload(out, dstKey))
	} else {
		queueReliable(sess, protocol.OpcodeRemoveInvItem, removeInvItemPayload(i, 0, took))
		queueReliable(sess, protocol.OpcodeAddBankItem, bankAddPayload(out, dstKey))
	}
	log.Printf("BankItem: %s moved %d %q %s", player.Name, took, p.ItemName,
		map[bool]string{true: "out of the bank", false: "into the bank"}[fromBank])
	return true
}

// HandleBankTunar is 0x1255: tunar into the bank (0) or out of it (1).
func (h *NPCWindowHandler) HandleBankTunar(sess *session.Session, msg *protocol.Message) error {
	r := buffer.NewReader(msg.Data())
	id, e1 := r.ReadUint32()
	dir, e2 := r.ReadByte()
	amount, e3 := r.Read7BitEncodedInt64()
	if e1 != nil || e2 != nil || e3 != nil {
		return nil
	}
	player, _, ok := h.windowNPC(sess, id, npcFlagBank, "BankTunar")
	if !ok || amount < 1 || dir > 1 || (h.gameWorld != nil && !h.gameWorld.IsLivePlayer(player)) {
		refuse(sess)
		return nil
	}
	bankMu.Lock()
	defer bankMu.Unlock()
	a := int(amount)
	if dir == 0 && a <= player.Tunar {
		player.Tunar -= a
		player.BankTunar += a
	} else if dir == 1 && a <= player.BankTunar {
		player.BankTunar -= a
		player.Tunar += a
	} else {
		log.Printf("BankTunar: %s moves %d (dir %d) with %d / %d", player.Name, a, dir, player.Tunar, player.BankTunar)
		refuse(sess)
		return nil
	}
	// the purse and the bank both moved: both kept (saveTunar keeps the
	// purse alone; the bank's tunar is written only under bankMu)
	if h.inv != nil {
		if err := h.inv.SetTunar(player.CharServerID, player.Tunar, player.BankTunar); err != nil {
			log.Printf("BankTunar: saving %s's %d / %d: %v", player.Name, player.Tunar, player.BankTunar, err)
		}
	}
	queueReliable(sess, protocol.OpcodePlayerTunar, tunarPayload(player.Tunar))
	queueReliable(sess, protocol.OpcodeConfirmBankTunar, bankTunarPayload(player.BankTunar))
	log.Printf("BankTunar: %s now %d carried, %d banked", player.Name, player.Tunar, player.BankTunar)
	return nil
}
