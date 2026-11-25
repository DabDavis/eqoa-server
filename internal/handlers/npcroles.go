package handlers

import (
	"encoding/binary"
	"log"
	"sort"
	"sync"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// npcroles.go — what the server knows of its NPCs that vanilla cannot read off
// their names and flags, for its minimap's colours (Heirloom petition 3092:
// "colour friendly NPC squares on the minimap by role: spell merchants ...").
// A bank, a blacksmith and a merchant are the NPC's own flags (CLIENT's
// m_npcFlags 2, 8, 1), a coachman his name; a spell merchant is a "Merchant"
// like any other, known only by what he sells: his npcitem list's scrolls
// (database.SpellMerchants), and the class whose scrolls they mostly are.
//
// Sent as protocol.OpcodeNpcRoles to a client that answered the bug report
// offer (vanilla; a PS2 never does and is never sent it), at that answer: every
// world's spell merchants by name, in parts of at most npcRolesPart bytes.

// npcRolesPart is the most a part's body holds.
const npcRolesPart = 4000

// The roles OpcodeNpcRoles names.
const npcRoleSpellMerchant = 1

// npcRoles is the parts as built, made at the first send from the store
// SetNpcRoleStore gave (nil: nothing sent).
var npcRoles struct {
	sync.Mutex
	store interface {
		SpellMerchants(icon int) (map[string]int, error)
	}
	parts [][]byte
	built bool
}

// SetNpcRoleStore gives the store the spell merchants are read from (nil:
// none are sent); the parts are built again at the next send.
func SetNpcRoleStore(s *database.SpellbookStore) {
	npcRoles.Lock()
	defer npcRoles.Unlock()
	npcRoles.store, npcRoles.parts, npcRoles.built = nil, nil, false
	if s != nil {
		npcRoles.store = s
	}
}

// buildNpcRoles is OpcodeNpcRoles' parts for the spell merchants by name and
// class, in name order: each part a u8 version, u8 flags, u16 count and its
// NPCs; none for none.
func buildNpcRoles(spell map[string]int) [][]byte {
	names := make([]string, 0, len(spell))
	for n := range spell {
		if n != "" && len(n) <= 255 {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	var parts [][]byte
	var body []byte
	count := 0
	flush := func(last bool) {
		flags := byte(0)
		if len(parts) == 0 {
			flags |= 1
		}
		if last {
			flags |= 2
		}
		b := binary.LittleEndian.AppendUint16(nil, uint16(protocol.OpcodeNpcRoles))
		b = append(b, 1, flags)
		b = binary.LittleEndian.AppendUint16(b, uint16(count))
		parts = append(parts, append(b, body...))
		body, count = nil, 0
	}
	for _, n := range names {
		entry := []byte{npcRoleSpellMerchant, 255, byte(len(n))}
		if c := spell[n]; c >= 0 && c <= 14 {
			entry[1] = byte(c)
		}
		entry = append(entry, n...)
		if count > 0 && len(body)+len(entry) > npcRolesPart {
			flush(false)
		}
		body = append(body, entry...)
		count++
	}
	if count > 0 {
		flush(true)
	}
	return parts
}

// npcRoleParts is the parts, built once from the store.
func npcRoleParts() [][]byte {
	npcRoles.Lock()
	defer npcRoles.Unlock()
	if !npcRoles.built && npcRoles.store != nil {
		spell, err := npcRoles.store.SpellMerchants(scrollIcon)
		if err != nil {
			log.Printf("NpcRoles: reading the spell merchants: %v", err)
			return nil // asked again at the next send
		}
		npcRoles.parts, npcRoles.built = buildNpcRoles(spell), true
		log.Printf("NpcRoles: %d spell merchants in %d parts", len(spell), len(npcRoles.parts))
	}
	return npcRoles.parts
}

// SendNpcRoles sends the session's client the NPCs' roles (a vanilla client:
// one that answered the bug report offer).
func SendNpcRoles(sess *session.Session) {
	if !sess.BugReports() {
		return
	}
	for _, p := range npcRoleParts() {
		queueReliable(sess, protocol.OpcodeNpcRoles, p)
	}
}
