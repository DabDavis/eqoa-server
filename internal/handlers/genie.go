package handlers

import (
	"database/sql"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// genie.go — the Genie's Lamp (the user, 2026-11-06: "lets give players a
// heirloom shop and bank NPC(doesnt replace pet slot) make it a genie. the
// item is a lamp in their inventory, infinite uses. when the player prompts,
// choice for bank and choice to buy/sell at shop. genie disappears after the
// prompt is gone", then "also give third option to genie for blacksmith").
//
// The lamp is an item record of CLIENT's usable type named GenieLampName
// (tools/seed_genie_lamp.py makes it and puts it among every combo's starting
// items; a character with none is given one at its login, giveGenieLamp).
// Its Use (the item menu's or a quick slot's 0x003E, equip.go) is never
// spent: it brings the player's genie (entity/genie.go), which asks at once,
// in a box under mark genieBoxMark, what its master wishes: the bank, its
// wares or the forge, or nothing. Talking to it asks again; anyone else is
// answered one line. Any answer, or the box closed, sends the genie away (a
// box CLIENT declines at once, because a menu was up when it came, does not:
// the genie waits to be talked to, GenieWait). The windows are CLIENT's own:
//   - the bank: 0x124D, empty (the dispatcher's case opens it from the game
//     screen, which an answered box returns to: VIWndGame's 276, mode 0); its
//     moves carry CLIENT's target, whatever it is (VIBankControl), so any id
//     is the genie's while the window is the player's;
//   - its wares: 0x00B7 with GenieWindowID as the merchant, which CLIENT
//     hands back with each buy and sale (m_p1B300): an Heirloom Outfitter's
//     catalog (genieShop: the one selling the player's class's heirlooms with
//     the fewest other classes), the buyer's own view of each heirloom; buying
//     and selling are any merchant's (merchant.go);
//   - the forge: 0x0061 with GenieWindowID as the blacksmith (m_p22B78, handed
//     back with each repair), repairs at a blacksmith's price (blacksmith.go:
//     the item's value by its wear; heirlooms, worth nothing, mend free).
// windowNPC takes a window request as the genie's while the player holds the
// window (World.GenieWindowNPC: GenieReach from where the genie stood, in its
// world, alive, used within GenieWindowIdle). CLIENT sends nothing when the
// bank or the merchant window closes, so the window ends by those limits; the
// forge's close (0x0063) ends it at once.

// GenieLampName is the lamp's record's name.
const GenieLampName = "Genie's Lamp"

// genieBoxMark marks a genie's question's id (beside the quests' 0x51, the
// triggers' 0x52, the duels' 0x53, the rehabilitators' 0x54, the advisors'
// 0x55, the dyers' 0x56 and the coachmen's ledgers' 0x57).
const genieBoxMark = 0x58000000

// genieDeclinedWithin is how soon a closed box counts as one CLIENT declined
// itself (Msg0034: a box that comes while a menu is up is answered 255 at
// once) rather than one the player closed.
const genieDeclinedWithin = 1500 * time.Millisecond

// The genie's answers, in the box's order.
const (
	genieBank = iota
	genieShop
	genieSmith
	genieNothing
)

var genieAnswers = []string{"My bank", "Your wares", "Mend my gear", "Nothing, thank you"}

// The genie's words (ours).
const (
	genieGreeting   = "Your wish is my command, master."
	genieQuestion   = "What do you wish?"
	genieNotYours   = "I answer only to the one who holds my lamp."
	genieFarewell   = "Until you rub the lamp again."
	genieNoWares    = "I have no wares for one of your calling."
	genieShopGreets = "Heirlooms for you, master."
	genieNoGenie    = "The lamp is cold. Try again in a moment."
)

// IsGenieBox reports a box a genie asked.
func IsGenieBox(id uint32) bool { return id&0xFF000000 == genieBoxMark }

// IsGenieLamp is whether an item record is the lamp: worn nowhere and so
// named.
func IsGenieLamp(p *database.ItemPatternRecord) bool {
	return p != nil && p.EquipSlot == -1 && strings.EqualFold(strings.TrimSpace(p.ItemName), GenieLampName)
}

// Genie runs the lamps' genies.
type Genie struct {
	gw      *entity.World
	db      *database.SQLiteDB
	windows *NPCWindowHandler
	now     func() time.Time

	mu    sync.Mutex
	seq   uint32
	asked map[uint32]genieAsk // by the character's serverid
	shops map[int]genieShopAt // the shop's catalog by class
}

type genieAsk struct {
	id uint32
	at time.Time
}

type genieShopAt struct {
	npcid int
	at    time.Time
}

// genieShopTTL is how long a class's shop is kept before it is looked for
// again (the outfitters' stock may change: tools/heirloom_epics.py).
const genieShopTTL = 10 * time.Minute

// NewGenie makes the genies over the world, the database (the outfitters'
// stock) and the window handler (the bank, the merchant, the blacksmith).
func NewGenie(gw *entity.World, db *database.SQLiteDB, windows *NPCWindowHandler) *Genie {
	g := &Genie{gw: gw, db: db, windows: windows, now: time.Now, asked: map[uint32]genieAsk{}, shops: map[int]genieShopAt{}}
	if windows != nil {
		windows.genie = g
	}
	return g
}

func (g *Genie) say(sess *session.Session, line string) {
	for _, part := range splitSay(entity.GenieName+": "+line, questBoxChars) {
		SendDialogueBox(sess, 0, part)
	}
}

func (g *Genie) ask(sess *session.Session, p *entity.Player) {
	g.mu.Lock()
	g.seq++
	a := genieAsk{id: genieBoxMark | (g.seq & 0xFFFFFF), at: g.now()}
	g.asked[p.CharServerID] = a
	g.mu.Unlock()
	SendQuestPopupMulti(sess, a.id, genieQuestion, genieAnswers)
}

// Summon is the lamp used: the player's genie comes and asks.
func (g *Genie) Summon(sess *session.Session, p *entity.Player) {
	if g == nil || g.gw == nil || p == nil {
		return
	}
	if _, err := g.gw.SummonGenie(sess, g.shopOf(int(p.Class)), g.now()); err != nil {
		log.Printf("Genie: %s's lamp: %v", p.Name, err)
		if !errors.Is(err, entity.ErrGenieNoPlayer) {
			g.say(sess, genieNoGenie)
		}
		return
	}
	g.say(sess, genieGreeting)
	g.ask(sess, p)
}

// Talk is a genie talked to: its master is asked again, anyone else told it
// is not theirs. It reports whether the NPC was a genie.
func (g *Genie) Talk(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	if g == nil || g.gw == nil || p == nil || npc == nil {
		return false
	}
	genie, mine := g.gw.GenieOf(sess, npc)
	if !genie {
		return false
	}
	forgetDialogs(p)
	if !mine {
		g.say(sess, genieNotYours)
		return true
	}
	g.ask(sess, p)
	return true
}

// Answer is HandleDialogueOption's question for a genie's box. It reports
// whether the box was a genie's (an old one does nothing).
func (g *Genie) Answer(sess *session.Session, p *entity.Player, box uint32, option byte) bool {
	if !IsGenieBox(box) {
		return false
	}
	if g == nil || g.gw == nil || p == nil {
		return true
	}
	g.mu.Lock()
	a, ok := g.asked[p.CharServerID]
	if ok && a.id == box {
		delete(g.asked, p.CharServerID)
	}
	g.mu.Unlock()
	if !ok || a.id != box {
		return true
	}
	now := g.now()
	switch int(option) {
	case genieBank:
		if !g.gw.GenieOpened(sess, entity.GenieBank, 0, now) {
			return true
		}
		queueReliable(sess, protocol.OpcodeBankUI, build(protocol.OpcodeBankUI, 2, nil))
		if g.windows != nil && g.windows.auction != nil { // what the auction house holds, filed (auction.go)
			g.windows.auction.bankOpened(sess, p)
		}
		log.Printf("Genie: %s opened the bank", p.Name)
	case genieShop:
		npc, id, ok := g.window(sess, entity.GenieShop, now)
		if !ok {
			return true
		}
		if npc.NPCID == 0 || g.windows == nil {
			g.gw.DismissGenie(sess)
			g.say(sess, genieNoWares)
			return true
		}
		g.windows.openMerchant(sess, p, npc, id, genieShopGreets)
	case genieSmith:
		if _, id, ok := g.window(sess, entity.GenieSmith, now); ok {
			queueReliable(sess, protocol.OpcodeBlackSmithMenu, blacksmithPayload(id))
			log.Printf("Genie: %s opened the forge", p.Name)
		}
	case 255:
		if now.Sub(a.at) < genieDeclinedWithin {
			// CLIENT's own: the box came while a menu was up; the genie
			// stays to be talked to
			log.Printf("Genie: %s's client declined the box at once: the genie waits", p.Name)
			return true
		}
		g.gw.DismissGenie(sess)
	default: // nothing, thank you
		g.gw.DismissGenie(sess)
		g.say(sess, genieFarewell)
	}
	return true
}

// window opens the genie's window of the kind: the genie leaves and the
// window's id is its GenieWindowID.
func (g *Genie) window(sess *session.Session, kind entity.GenieWindow, now time.Time) (*entity.NPC, uint32, bool) {
	npc, ok := g.gw.GenieNPC(sess)
	if !ok {
		return nil, 0, false
	}
	id := entity.GenieWindowID(npc.ServerID)
	if !g.gw.GenieOpened(sess, kind, id, now) {
		return nil, 0, false
	}
	return npc, id, true
}

// windowNPC is windowNPC's question for the genie: the stand-in NPC when the
// request is the session's genie's window of that flag (GenieWindowNPC).
func (g *Genie) windowNPC(sess *session.Session, id uint32, flag int) (*entity.NPC, bool) {
	if g == nil || g.gw == nil {
		return nil, false
	}
	kind := entity.GenieNoWindow
	switch flag {
	case npcFlagBank:
		kind = entity.GenieBank
	case npcFlagMerchant:
		kind = entity.GenieShop
	case npcFlagBlacksmith:
		kind = entity.GenieSmith
	}
	if kind == entity.GenieNoWindow {
		return nil, false
	}
	return g.gw.GenieWindowNPC(sess, kind, id, g.now())
}

// shopOf is the npcitem list a genie sells a class: the Heirloom Outfitter's
// selling that class's heirlooms whose stock holds the fewest other classes'
// (the lowest id of those), 0 when none does; kept genieShopTTL.
func (g *Genie) shopOf(class int) int {
	if g.db == nil {
		return 0
	}
	g.mu.Lock()
	s, ok := g.shops[class]
	g.mu.Unlock()
	if ok && g.now().Sub(s.at) <= genieShopTTL {
		return s.npcid
	}
	var npcid int
	err := g.db.QueryRow(`SELECT i.npcid FROM npcitem i
		JOIN npcs n ON n.npc_id = i.npcid
		JOIN heirlooms h ON h.patternid = i.patternid
		WHERE n.npc_name = 'Heirloom Outfitter'
		GROUP BY i.npcid
		HAVING SUM(h.class = ?) > 0
		ORDER BY COUNT(DISTINCT h.class), i.npcid
		LIMIT 1`, class).Scan(&npcid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		log.Printf("Genie: the shop of class %d: %v", class, err)
	}
	g.mu.Lock()
	g.shops[class] = genieShopAt{npcid: npcid, at: g.now()}
	g.mu.Unlock()
	return npcid
}

// genieLampID is the lamp's record, 0 when there is none.
func genieLampID(db *database.SQLiteDB) int { return keptItemID(db, GenieLampName, "Genie") }

// keptItemID is the record of an item every character keeps (the lamp, the
// fang: wolffang.go), worn nowhere and so named, 0 when there is none.
func keptItemID(db *database.SQLiteDB, name, tag string) int {
	var id int
	if err := db.QueryRow(`SELECT patternid FROM itempattern WHERE itemname = ? AND equipslot = -1 ORDER BY patternid LIMIT 1`,
		name).Scan(&id); err != nil && !errors.Is(err, sql.ErrNoRows) {
		log.Printf("%s: the record of %s: %v", tag, name, err)
	}
	return id
}

// giveGenieLamp gives a character with no lamp one at its login, before its
// items are read: in the pack while it holds fewer than invListCap records
// (worn ones among them), else the bank, else none (the next login tries
// again). Nothing when there is no lamp record.
func (h *CharacterHandler) giveGenieLamp(serverID uint32) {
	if h.store == nil || h.inventoryStore == nil {
		return
	}
	h.giveKeptItem(serverID, genieLampID(h.store.DB()), "Genie", "lamp")
}

// giveKeptItem gives a character holding no item of record id one at its
// login, as giveGenieLamp says (tag the log's word, noun the item's). Nothing
// for record 0.
func (h *CharacterHandler) giveKeptItem(serverID uint32, id int, tag, noun string) {
	if id == 0 || h.store == nil || h.inventoryStore == nil {
		return
	}
	db := h.store.DB()
	var held, pack, bank int
	if err := db.QueryRow(`SELECT COALESCE(SUM(patternid = ?), 0), COALESCE(SUM(location = 0), 0), COALESCE(SUM(location = 1), 0)
		FROM charinventory WHERE serverid = ?`, id, serverID).Scan(&held, &pack, &bank); err != nil {
		log.Printf("%s: character %d's items: %v", tag, serverID, err)
		return
	}
	if held > 0 {
		return
	}
	location := 0
	switch {
	case pack < invListCap:
	case bank < invListCap:
		location = 1
	default:
		log.Printf("%s: character %d's pack and bank are full: no %s this login", tag, serverID, noun)
		return
	}
	var hp int
	_ = db.QueryRow(`SELECT maxhp FROM itempattern WHERE patternid = ?`, id).Scan(&hp)
	if _, err := h.inventoryStore.AddCharItem(serverID, id, 1, hp, 0, location); err != nil {
		log.Printf("%s: character %d's %s: %v", tag, serverID, noun, err)
		return
	}
	log.Printf("%s: character %d given its %s (location %d)", tag, serverID, noun, location)
}
