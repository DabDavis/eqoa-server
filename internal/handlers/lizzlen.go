package handlers

import (
	"fmt"
	"log"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// lizzlen.go — Master Lizzlen, the Plane of Sky's recharger (the player Hoofbite's petition 1985,
// 2026-11-13: "Master Lizzlen in plane of sky is supposed to be the npc that will allow you to
// recharge clicky items such as the Ring of Speed and other clicky style POS drops").
//
// A talk to him restores, free, the charges of every item with a use of charges (itemeffect.charges,
// a record that stacks no more than one: InventoryStore.ItemCharges) that the player carries in its
// pack, not worn, short of its full count: its remaincharge set to the full count, and the client
// told by the item taken off its list and given again with them (0x003C, 0x003B: CLIENT has no
// message that sets an item's charges; the item comes back at the end of the pack, as an exchange
// at Dabrish's does). A worn one is named and left: taken off, it is recharged at the next talk.
// Nothing short: he says so.

// LizzlenName is the recharger's name.
const LizzlenName = "Master Lizzlen"

// lizzlenSay is a line of his in the dialog box.
func lizzlenSay(sess *session.Session, line string) {
	for _, part := range splitSay(LizzlenName+": "+line, questBoxChars) {
		SendDialogueBox(sess, 0, part)
	}
}

// rechargeStore is what the recharge reads and writes (database.InventoryStore).
type rechargeStore interface {
	GetCharacterItems(serverID uint32) ([]database.CharInventoryEntry, error)
	ItemCharges(patternID int) int
	SetItemCharges(serverID uint32, itemID, charges int) error
}

// lizzlenTalk is a talk to npc: the recharge when npc is Master Lizzlen, else false.
func (h *InteractHandler) lizzlenTalk(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	if npc == nil || npc.Dead || !strings.EqualFold(strings.TrimSpace(npc.Name), LizzlenName) {
		return false
	}
	if h.recharge == nil {
		lizzlenSay(sess, "My tools are packed away today.")
		return true
	}
	done, worn, err := rechargeItems(sess, p, h.recharge)
	if err != nil {
		log.Printf("Lizzlen: %s's items: %v", p.Name, err)
		lizzlenSay(sess, "Something went wrong at my bench. Try me again.")
		return true
	}
	switch {
	case len(done) > 0:
		lizzlenSay(sess, "There, as good as the day they were made: "+strings.Join(done, ", ")+".")
	case len(worn) == 0:
		lizzlenSay(sess, "Bring me an item whose charges you have used and I will restore them. Carry it in your pack, not worn.")
	}
	if len(worn) > 0 {
		lizzlenSay(sess, "Take off "+strings.Join(worn, ", ")+" and talk to me again: I cannot work on what you wear.")
	}
	return true
}

// rechargeItems restores the charges of p's pack items short of them (the file's doc): the names
// recharged (with their counts) and the worn ones left.
func rechargeItems(sess *session.Session, p *entity.Player, inv rechargeStore) (done, worn []string, err error) {
	rows, err := inv.GetCharacterItems(p.CharServerID)
	if err != nil {
		return nil, nil, err
	}
	l := sessionInvList(sess)
	for _, r := range rows {
		if r.Location != database.LocPack || r.Pattern.MaxStack >= 2 {
			continue
		}
		full := inv.ItemCharges(r.Pattern.PatternID)
		if full <= 0 || r.Charges >= full {
			continue
		}
		name := r.Pattern.ItemName
		if r.EquipLoc >= 0 {
			worn = append(worn, name)
			continue
		}
		at := -1
		if l != nil {
			at = l.indexOfItem(r.ItemID)
		}
		if at < 0 {
			continue // not in the client's list: nothing to tell it by
		}
		if err := inv.SetItemCharges(p.CharServerID, r.ItemID, full); err != nil {
			return done, worn, err
		}
		if l.took(at, 1) {
			queueReliable(sess, protocol.OpcodeRemoveInvItem, removeInvItemPayload(at, 0, 1))
		}
		rec := wireRecord(r)
		rec.StackLeft, rec.Charges = 1, full
		if key, took := l.add(r.ItemID, r.Pattern.PatternID, 1, r.Pattern.MaxStack); took > 0 {
			queueReliable(sess, protocol.OpcodeAddInvItem, addInvItemPayload(&rec, key, readsCapes(sess)))
		}
		done = append(done, fmt.Sprintf("%s (%d charges)", name, full))
		log.Printf("Lizzlen: %s's %q (row %d) recharged %d -> %d", p.Name, name, r.ItemID, r.Charges, full)
	}
	return done, worn, nil
}
