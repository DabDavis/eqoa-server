package handlers

import (
	"errors"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The GM's channel: CLIENT's VIClientPart382E8 (decomp viadmin.cpp), behind
// the start menu's TEST entry, and the commands it sends.
//
//	S>C 0x0700 {35, zigzag n}   RecvEnabled: n != 0 shows TEST; n > 0 also
//	                            takes the GM's /create, /mastery and /tradelvl
//	                            and gives an auction minutes for hours
//	S>C 0x0700 {50, wide name}  RecvPrompt: a Yes/No box, the client's own
//	                            words for giving an item and the name (31
//	                            units at most); Yes sends 0x0701 48, No 49
//	C>S 0x0701 {u8 sub-op, ...} one command, its fields as gmFields says
//	C>S 0x0702 {100, u8 x}      the TRIGGER entry's number
//
// Only a GM (accountinfo.acctlevel 1 or more: devcmds.go's isGM) is sent the
// channel, at its world entry, and each 0x0701 and 0x0702 is checked again
// as it comes: anyone else's is logged and dropped. No capture of SOE's
// server answering one exists, so what each sub-op does here is this
// server's: the GM command it already had where one fits, a line saying so
// where none does.

// The 0x0700 sub-ops.
const (
	gmSubEnabled byte = 35
	gmSubPrompt  byte = 50
)

// The 0x0701 sub-ops, by what the TEST menu calls them.
const (
	gmSubInvis         byte = 1
	gmSubBuffPC        byte = 2
	gmSubSummonItem    byte = 3
	gmSubSprite        byte = 4
	gmSubRevertSprite  byte = 5
	gmSubGotoLocation  byte = 6
	gmSubGotoSpawner   byte = 7
	gmSubForceSpawn    byte = 8
	gmSubKick          byte = 9
	gmSubSilence       byte = 10
	gmSubGotoPlayer    byte = 11
	gmSubWhereIs       byte = 12
	gmSubBroadcast     byte = 13
	gmSub14            byte = 14 // sent by a menu id no menu holds
	gmSubSafeMode      byte = 15
	gmSubKill          byte = 16
	gmSubHeal          byte = 17
	gmSubNetSim        byte = 18
	gmSubServerLoad    byte = 19
	gmSubSummonPlayer  byte = 20
	gmSubWhoIs         byte = 21
	gmSubHistoryToken  byte = 22
	gmSubKillSpawner   byte = 23
	gmSubUnkillSpawner byte = 24
	gmSubGiveItem      byte = 25
	gmSubToggleNPC     byte = 32
	gmSubRename        byte = 33
	gmSubSnoop         byte = 34
	gmSubEndSnoop      byte = 37
	gmSubDebtOne       byte = 38
	gmSubDebtAll       byte = 39
	gmSubDebugNPC      byte = 40
	gmSubDisbandGuild  byte = 41
	gmSubYes           byte = 48
	gmSubNo            byte = 49
	gmSubFreeze        byte = 51
	gmSubNetStats      byte = 52
	gmSubCreate        byte = 53
	gmSubMastery       byte = 54
	gmSubTradeLevel    byte = 55
)

// gmTrigger is the first byte of a 0x0702 (Do2's constant).
const gmTrigger byte = 100

// A sub-op's fields, as the client's senders write them.
type gmFieldKind int

const (
	gmNone      gmFieldKind = iota
	gmInt                   // u32 (WriteU32)
	gmInt2                  // u32, u32
	gmString                // u32 byte count, the bytes (WriteString(const char *))
	gmIntString             // u32, then a string (Change Name)
	gmWide                  // u32 unit count, UTF-16LE (BROADCAST's wide text)
	gmNetSim                // u8 loss, u16 latency, u16 jitter (SendSimulation)
)

// gmFields is every sub-op the client sends and its fields (viadmin.cpp's
// SEND_* senders, Send16/17, Send33, SendText13, SendSimulation, Toggle19).
var gmFields = map[byte]gmFieldKind{
	gmSubInvis: gmNone, gmSubRevertSprite: gmNone, gmSub14: gmNone, gmSubSafeMode: gmNone,
	gmSubServerLoad: gmNone, gmSubToggleNPC: gmNone, gmSubEndSnoop: gmNone, gmSubYes: gmNone, gmSubNo: gmNone,

	gmSubSummonItem: gmInt, gmSubGotoSpawner: gmInt, gmSubForceSpawn: gmInt, gmSubKill: gmInt,
	gmSubHeal: gmInt, gmSubHistoryToken: gmInt, gmSubKillSpawner: gmInt, gmSubUnkillSpawner: gmInt,
	gmSubSnoop: gmInt, gmSubDebtOne: gmInt, gmSubDebtAll: gmInt, gmSubDebugNPC: gmInt, gmSubNetStats: gmInt,

	gmSubBuffPC: gmInt2, gmSubGiveItem: gmInt2,

	gmSubSprite: gmString, gmSubGotoLocation: gmString, gmSubKick: gmString, gmSubSilence: gmString,
	gmSubGotoPlayer: gmString, gmSubWhereIs: gmString, gmSubSummonPlayer: gmString, gmSubWhoIs: gmString,
	gmSubDisbandGuild: gmString, gmSubFreeze: gmString, gmSubCreate: gmString, gmSubMastery: gmString,
	gmSubTradeLevel: gmString,

	gmSubRename:    gmIntString,
	gmSubBroadcast: gmWide,
	gmSubNetSim:    gmNetSim,
}

// gmTextMax bounds a string field: the client's chat line is never 128
// units, and its edit boxes are shorter still.
const gmTextMax = 1024

// gmRequest is one 0x0701, read.
type gmRequest struct {
	Sub     byte
	A, B    uint32 // the ints, as the client sends them (-1: no target)
	Text    string
	Loss    byte
	Latency uint16
	Jitter  uint16
}

func (r gmRequest) String() string {
	switch gmFields[r.Sub] {
	case gmInt:
		return fmt.Sprintf("sub-op %d (%d)", r.Sub, int32(r.A))
	case gmInt2:
		return fmt.Sprintf("sub-op %d (%d, %d)", r.Sub, int32(r.A), int32(r.B))
	case gmString, gmWide:
		return fmt.Sprintf("sub-op %d %q", r.Sub, r.Text)
	case gmIntString:
		return fmt.Sprintf("sub-op %d (%d, %q)", r.Sub, int32(r.A), r.Text)
	case gmNetSim:
		return fmt.Sprintf("sub-op %d (loss %d, latency %d, jitter %d)", r.Sub, r.Loss, r.Latency, r.Jitter)
	}
	return fmt.Sprintf("sub-op %d", r.Sub)
}

var (
	errGMEmpty   = errors.New("an empty 0x0701")
	errGMUnknown = errors.New("a sub-op the client never sends")
	errGMShort   = errors.New("cut short")
)

// parseGMRequest reads a 0x0701's payload (after the opcode).
func parseGMRequest(data []byte) (gmRequest, error) {
	if len(data) < 1 {
		return gmRequest{}, errGMEmpty
	}
	req := gmRequest{Sub: data[0]}
	kind, ok := gmFields[req.Sub]
	if !ok {
		return req, errGMUnknown
	}
	r := buffer.NewReader(data[1:])
	var err error
	switch kind {
	case gmInt:
		req.A, err = r.ReadUint32()
	case gmInt2:
		if req.A, err = r.ReadUint32(); err == nil {
			req.B, err = r.ReadUint32()
		}
	case gmString:
		req.Text, err = readGMString(r)
	case gmIntString:
		if req.A, err = r.ReadUint32(); err == nil {
			req.Text, err = readGMString(r)
		}
	case gmWide:
		var n uint32
		if n, err = r.ReadUint32(); err == nil {
			if n > gmTextMax {
				err = errGMShort
			} else {
				req.Text, err = r.ReadStringUnicode(int(n))
			}
		}
	case gmNetSim:
		if req.Loss, err = r.ReadByte(); err == nil {
			if req.Latency, err = r.ReadUint16(); err == nil {
				req.Jitter, err = r.ReadUint16()
			}
		}
	}
	if err != nil {
		return req, errGMShort
	}
	return req, nil
}

// readGMString reads VIMemStream::WriteString(const char *): a u32 byte
// count, then the bytes, no terminator. The client made them out of its
// wide text with char16to8 (a unit past 255 is '?'), so each byte is a
// Latin-1 character.
func readGMString(r *buffer.Reader) (string, error) {
	n, err := r.ReadUint32()
	if err != nil || n > gmTextMax {
		return "", errGMShort
	}
	b, err := r.ReadBytes(int(n))
	if err != nil {
		return "", errGMShort
	}
	runes := make([]rune, len(b))
	for i, c := range b {
		runes[i] = rune(c)
	}
	return string(runes), nil
}

// gmChannel is the TEST menu's state for the server's run: the Give Item a
// GM's Yes is awaited for, who is silenced, who snoops whom, who waits for a
// player's network counters, and where items and names are kept.
type gmChannel struct {
	mu       sync.Mutex
	gifts    map[entity.EntityID]gmGift                   // by the GM's player
	silenced map[uint32]bool                              // characters (serverid)
	snoops   map[entity.EntityID]map[entity.EntityID]bool // snooped player: its GMs
	netWait  map[*session.Session][]entity.EntityID       // a player's session: the GMs asking

	inv   *database.InventoryStore
	names nameStore
}

// gmGift is a Give Item a GM has yet to say Yes or No to.
type gmGift struct {
	to      uint32 // the character (serverid)
	toName  string
	pattern database.ItemPatternRecord
	at      time.Time
}

// gmGiftLife is how long a Give Item waits for its Yes.
const gmGiftLife = 5 * time.Minute

// nameStore checks and keeps a GM's rename (database.SQLiteCharacterStore).
type nameStore interface {
	Rename(serverID uint32, name string) error
}

// SetGMStores gives the GM's channel where items are looked up and given,
// and where a rename is kept. Without them Summon Item, Give Item, /create
// and Change Name say so and do nothing.
func (h *ChatHandler) SetGMStores(inv *database.InventoryStore, names nameStore) {
	h.gm.mu.Lock()
	defer h.gm.mu.Unlock()
	h.gm.inv, h.gm.names = inv, names
}

// RegisterGMChannel registers 0x0701 and 0x0702.
func (h *ChatHandler) RegisterGMChannel(r *Registry) {
	r.Register(protocol.OpcodeGMCommand, h.HandleGMCommand)
	r.Register(protocol.OpcodeGMTrigger, h.HandleGMTrigger)
}

// buildGMEnabled is 0x0700 {35, zigzag n}.
func buildGMEnabled(n int64) []byte {
	return build(protocol.OpcodeAdminCode, 16, func(w *buffer.Writer) {
		w.WriteByte(gmSubEnabled)
		w.Write7BitEncodedInt64(n)
	})
}

// buildGMPrompt is 0x0700 {50, name}: RecvPrompt's ReadString(name, 64).
func buildGMPrompt(name string) []byte {
	return build(protocol.OpcodeAdminCode, 8+64, func(w *buffer.Writer) {
		w.WriteByte(gmSubPrompt)
		writeWideString(w, name, 64)
	})
}

// SendGMChannel gives a GM's client the channel (0x0700 35, on) at its world
// entry; anyone else's is sent nothing. A client keeps it until its next
// login (vanilla) or the console's next boot (a PS2), whichever comes.
func (h *ChatHandler) SendGMChannel(sess *session.Session) {
	if !h.isGM(sess) {
		return
	}
	queueReliable(sess, protocol.OpcodeAdminCode, buildGMEnabled(1))
	log.Printf("GM channel: sent to account %d", sess.AccountID())
}

// HandleGMCommand is 0x0701: a GM's command, run; anyone else's dropped.
func (h *ChatHandler) HandleGMCommand(sess *session.Session, msg *protocol.Message) error {
	req, err := parseGMRequest(msg.Data())
	if err != nil {
		log.Printf("GM channel: %s from %s: % X", err, sess.RemoteAddr(), msg.Data())
		return nil
	}
	if !h.isGM(sess) {
		log.Printf("GM channel: %s from account %d, not a GM: dropped", req, sess.AccountID())
		return nil
	}
	if h.gameWorld == nil {
		return nil
	}
	gm := h.gameWorld.SessionPlayer(sess)
	if gm == nil {
		return nil
	}
	log.Printf("GM channel: %s: %s", gm.Name, req)
	h.gmCommand(sess, gm, req)
	return nil
}

// HandleGMTrigger is 0x0702 {100, x}: the TEST menu's TRIGGER, logged and
// told back; nothing on this server listens for one.
func (h *ChatHandler) HandleGMTrigger(sess *session.Session, msg *protocol.Message) error {
	d := msg.Data()
	if len(d) < 2 || d[0] != gmTrigger {
		log.Printf("GM channel: 0x0702 %X from %s: not a trigger", d, sess.RemoteAddr())
		return nil
	}
	if !h.isGM(sess) {
		log.Printf("GM channel: trigger %d from account %d, not a GM: dropped", d[1], sess.AccountID())
		return nil
	}
	log.Printf("GM channel: trigger %d from account %d", d[1], sess.AccountID())
	h.gmReply(sess, fmt.Sprintf("Trigger %d: nothing on this server answers a trigger", d[1]))
	return nil
}

// gmCommand runs one command for the GM gm (whose session is sess).
func (h *ChatHandler) gmCommand(sess *session.Session, gm *entity.Player, req gmRequest) {
	switch req.Sub {
	case gmSubInvis:
		if on, ok := h.gameWorld.ToggleGMHidden(gm.ServerID); ok {
			if on {
				h.gmReply(sess, "You are invisible (Toggle Invis again to be seen)")
			} else {
				h.gmReply(sess, "You are visible again")
			}
		}
	case gmSubBuffPC:
		t, ts, ok := h.gmPlayerTarget(sess, gm, req.A, "Buff PC")
		if ok {
			h.setLevelFor(sess, ts, t, int(int32(req.B)))
		}
	case gmSubSummonItem:
		h.gmGiveNow(sess, gm, sess, int(int32(req.A)), 1)
	case gmSubSprite:
		h.gmSprite(sess, gm, req.Text)
	case gmSubRevertSprite:
		h.gameWorld.SetGMForm(gm.ServerID, 0, 0)
		h.gmReply(sess, "Your own body again")
	case gmSubGotoLocation:
		h.gmGotoLocation(sess, gm, req.Text)
	case gmSubGotoSpawner, gmSubForceSpawn, gmSubKillSpawner, gmSubUnkillSpawner:
		h.gmSpawner(sess, gm, req.Sub, int(int32(req.A)))
	case gmSubKick:
		if t, ts, ok := h.gmNamed(sess, req.Text, "Kick"); ok {
			if t == gm {
				h.gmReply(sess, "That is you.")
				return
			}
			Eject(ts)
			h.gmReply(sess, fmt.Sprintf("Kicked %s.", t.Name))
			log.Printf("GM: %s kicked %s", gm.Name, t.Name)
		}
	case gmSubSilence:
		h.gmSilence(sess, gm, req.Text)
	case gmSubGotoPlayer:
		h.gotoPlayerCommand(sess, gm, strings.Fields(req.Text))
	case gmSubWhereIs:
		if t, _, ok := h.gmNamed(sess, req.Text, "Where Is"); ok {
			h.gmReply(sess, h.whereIs(t))
		}
	case gmSubBroadcast:
		text := strings.TrimSpace(req.Text)
		if text == "" {
			return
		}
		n := Broadcast(h.sessionManager, text, gmBroadcastRGB)
		h.gmReply(sess, fmt.Sprintf("Broadcast to %d players.", n))
	case gmSub14:
		h.gmReply(sess, "Sub-op 14: no TEST entry sends it, and this server has nothing for it")
	case gmSubSafeMode:
		ignored, _, ok := h.gameWorld.AggroIgnored(sess)
		if ok && h.gameWorld.SetAggroIgnored(sess, !ignored) {
			if !ignored {
				h.gmReply(sess, "Safe Mode on: creatures leave you alone (as #aggro off); a fight already on goes on")
			} else {
				h.gmReply(sess, "Safe Mode off: creatures attack you on sight again")
			}
		}
	case gmSubKill:
		h.gmKill(sess, gm, req.A)
	case gmSubHeal:
		h.gmHeal(sess, gm, req.A)
	case gmSubNetSim:
		h.gmReply(sess, fmt.Sprintf("Network simulation asked for: %d%% loss, %d ms latency, %d ms jitter. Your own client simulates it; this server simulates nothing.",
			req.Loss, req.Latency, req.Jitter))
	case gmSubServerLoad:
		h.gmReply(sess, "Server Load: this server sends no load readouts")
	case gmSubSummonPlayer:
		h.summonCommand(sess, gm, strings.Fields(req.Text))
	case gmSubWhoIs:
		if t, ts, ok := h.gmNamed(sess, req.Text, "Who Is"); ok {
			for _, l := range h.whoIs(t, ts) {
				h.gmReply(sess, l)
			}
		}
	case gmSubHistoryToken:
		h.gmReply(sess, "Summon by history token: this server keeps no item history")
	case gmSubGiveItem:
		h.gmGiveAsk(sess, gm, req.A, int(int32(req.B)))
	case gmSubYes, gmSubNo:
		h.gmGiveAnswer(sess, gm, req.Sub == gmSubYes)
	case gmSubToggleNPC:
		h.gmReply(sess, "Toggle NPC: this server has nothing for it")
	case gmSubRename:
		h.gmRename(sess, gm, req.A, req.Text)
	case gmSubSnoop:
		h.gmSnoop(sess, gm, req.A)
	case gmSubEndSnoop:
		if h.gm.endSnoops(gm.ServerID) {
			h.gmReply(sess, "Snoop ended")
		} else {
			h.gmReply(sess, "You were snooping no one")
		}
	case gmSubDebtOne, gmSubDebtAll:
		h.gmDebt(sess, gm, req.A, req.Sub == gmSubDebtAll)
	case gmSubDebugNPC:
		id := req.A
		if id == 0xFFFFFFFF || id == 0 {
			h.gmReply(sess, "No target selected")
			return
		}
		target, found := h.gameWorld.FindEntityByObjectID(sess, id)
		if !found {
			h.gmReply(sess, "Target not found")
			return
		}
		SendDialogueBox(sess, 0, EntityInfo(target, id))
	case gmSubDisbandGuild:
		h.gmDisband(sess, gm, req.Text)
	case gmSubFreeze:
		h.gmFreeze(sess, gm, req.Text)
	case gmSubNetStats:
		t, ts, ok := h.gmPlayerTarget(sess, gm, req.A, "Network Stats")
		if ok {
			h.gm.waitNetStats(ts, gm.ServerID)
			SendServerReady(ts)
			h.gmReply(sess, fmt.Sprintf("Asked %s's client for its network counters", t.Name))
		}
	case gmSubCreate:
		h.gmCreate(sess, gm, req.Text)
	case gmSubMastery:
		h.grantMasteryCommand(sess, strings.Fields(req.Text))
	case gmSubTradeLevel:
		h.gmReply(sess, "Trade levels: this server has none")
	}
}

// gmBroadcastRGB is a BROADCAST's colour: the GM panel's line to everyone
// (gmweb's default yellow).
var gmBroadcastRGB = [3]byte{0xFF, 0xE0, 0x40}

// gmPlayerTarget is the player an id from the GM's client names (its target)
// and its session; no target (-1, or 0) is the GM itself. Anything but a
// player is refused, said to the GM.
func (h *ChatHandler) gmPlayerTarget(sess *session.Session, gm *entity.Player, id uint32, what string) (*entity.Player, *session.Session, bool) {
	if id == 0xFFFFFFFF || id == 0 {
		return gm, sess, true
	}
	e, found := h.gameWorld.FindEntityByObjectID(sess, id)
	t, isPlayer := e.(*entity.Player)
	if !found || !isPlayer {
		h.gmReply(sess, what+" takes a player: target one (or none, for yourself)")
		return nil, nil, false
	}
	if t == gm {
		return gm, sess, true
	}
	ts := h.gameWorld.FindSessionByEntityID(t.ServerID)
	if ts == nil {
		h.gmReply(sess, t.Name+" has no session.")
		return nil, nil, false
	}
	return t, ts, true
}

// gmNamed is the online player of that name and its session, said to the GM
// when there is none.
func (h *ChatHandler) gmNamed(sess *session.Session, name, what string) (*entity.Player, *session.Session, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		h.gmReply(sess, what+": no name given")
		return nil, nil, false
	}
	t := h.gameWorld.PlayerByName(name)
	if t == nil {
		h.gmReply(sess, fmt.Sprintf("No player named %q is online.", name))
		return nil, nil, false
	}
	ts := h.gameWorld.FindSessionByEntityID(t.ServerID)
	if ts == nil {
		h.gmReply(sess, fmt.Sprintf("%s has no session.", t.Name))
		return nil, nil, false
	}
	return t, ts, true
}

// itemByID is the item of that number, said to the GM when there is none.
func (h *ChatHandler) itemByID(sess *session.Session, id int) (database.ItemPatternRecord, bool) {
	h.gm.mu.Lock()
	inv := h.gm.inv
	h.gm.mu.Unlock()
	if inv == nil {
		h.gmReply(sess, "Items: this server keeps none")
		return database.ItemPatternRecord{}, false
	}
	p, ok, err := inv.ItemPatternByID(id)
	if err != nil {
		h.gmReply(sess, fmt.Sprintf("Item %d: %v", id, err))
		return database.ItemPatternRecord{}, false
	}
	if !ok {
		h.gmReply(sess, fmt.Sprintf("No item %d.", id))
		return database.ItemPatternRecord{}, false
	}
	return p, true
}

// gmGiveNow gives n of item id to the player of session to, now (Summon
// Item, a Yes to Give Item, /create), and says so to the GM.
func (h *ChatHandler) gmGiveNow(sess *session.Session, gm *entity.Player, to *session.Session, id, n int) {
	p, ok := h.itemByID(sess, id)
	if !ok {
		return
	}
	h.gmGivePattern(sess, gm, to, p, n)
}

func (h *ChatHandler) gmGivePattern(sess *session.Session, gm *entity.Player, to *session.Session, p database.ItemPatternRecord, n int) {
	h.gm.mu.Lock()
	inv := h.gm.inv
	h.gm.mu.Unlock()
	t := h.gameWorld.SessionPlayer(to)
	if inv == nil || t == nil {
		h.gmReply(sess, "Items: this server keeps none")
		return
	}
	given, err := GiveItemNow(to, h.gameWorld, inv, p, n)
	switch {
	case err != nil:
		h.gmReply(sess, fmt.Sprintf("%q not given: %v", p.ItemName, err))
	case given == 0:
		h.gmReply(sess, fmt.Sprintf("%q not given: %s's inventory is full", p.ItemName, t.Name))
	case to == sess:
		h.gmReply(sess, fmt.Sprintf("Summoned %d %q (item %d)", given, p.ItemName, p.PatternID))
	default:
		h.gmReply(sess, fmt.Sprintf("Gave %s %d %q (item %d)", t.Name, given, p.ItemName, p.PatternID))
		GMLine(to, fmt.Sprintf("A GM gave you %s.", p.ItemName), gmBroadcastRGB)
	}
	if err == nil && given > 0 {
		log.Printf("GM: %s gave %s %d of item %d %q", gm.Name, t.Name, given, p.PatternID, p.ItemName)
	}
}

// gmGiveAsk is Give Item: the item looked up and the GM asked with the
// channel's Yes/No (0x0700 50) before it goes to the target (or, with no
// target, to the GM).
func (h *ChatHandler) gmGiveAsk(sess *session.Session, gm *entity.Player, target uint32, id int) {
	t, _, ok := h.gmPlayerTarget(sess, gm, target, "Give Item")
	if !ok {
		return
	}
	p, ok := h.itemByID(sess, id)
	if !ok {
		return
	}
	h.gm.mu.Lock()
	if h.gm.gifts == nil {
		h.gm.gifts = make(map[entity.EntityID]gmGift)
	}
	h.gm.gifts[gm.ServerID] = gmGift{to: t.CharServerID, toName: t.Name, pattern: p, at: time.Now()}
	h.gm.mu.Unlock()
	h.gmReply(sess, fmt.Sprintf("Give %s %q (item %d)? Yes gives it.", t.Name, p.ItemName, p.PatternID))
	queueReliable(sess, protocol.OpcodeAdminCode, buildGMPrompt(p.ItemName))
}

// gmGiveAnswer is the prompt's Yes (48) or No (49): the Give Item waiting
// given or dropped. With none waiting (or one past gmGiftLife) nothing.
func (h *ChatHandler) gmGiveAnswer(sess *session.Session, gm *entity.Player, yes bool) {
	h.gm.mu.Lock()
	g, ok := h.gm.gifts[gm.ServerID]
	delete(h.gm.gifts, gm.ServerID)
	h.gm.mu.Unlock()
	if !ok || time.Since(g.at) > gmGiftLife {
		log.Printf("GM channel: %s answered a prompt with nothing waiting", gm.Name)
		return
	}
	if !yes {
		h.gmReply(sess, fmt.Sprintf("Not given: %q to %s", g.pattern.ItemName, g.toName))
		return
	}
	to := SessionOfCharacter(h.gameWorld, h.sessionManager, g.to)
	if to == nil {
		h.gmReply(sess, fmt.Sprintf("%s is no longer in the world: %q not given", g.toName, g.pattern.ItemName))
		return
	}
	h.gmGivePattern(sess, gm, to, g.pattern, 1)
}

// gmSprite is Change Sprite, "model [scale]": the GM shown in that body (a
// DictID as the channel carries it, hex with 0x or decimal, as #npcinfo's
// Model= gives it) at that scale (the GM's own without one).
func (h *ChatHandler) gmSprite(sess *session.Session, gm *entity.Player, text string) {
	f := strings.Fields(text)
	if len(f) == 0 {
		h.gmReply(sess, "Change Sprite: a model (0x70D898C6, or #npcinfo's Model= number) and a scale")
		return
	}
	model, ok := parseModel(f[0])
	if !ok || model == 0 {
		h.gmReply(sess, fmt.Sprintf("Change Sprite: %q is no model number", f[0]))
		return
	}
	var size float32
	if len(f) > 1 {
		s, err := strconv.ParseFloat(f[1], 32)
		if err != nil || s <= 0 || s > 20 {
			h.gmReply(sess, fmt.Sprintf("Change Sprite: %q is no scale (more than 0, at most 20)", f[1]))
			return
		}
		size = float32(s)
	}
	h.gameWorld.SetGMForm(gm.ServerID, model, size)
	if size > 0 {
		h.gmReply(sess, fmt.Sprintf("Your body is model 0x%08X at %.2f; Revert Sprite gives yours back", model, size))
	} else {
		h.gmReply(sess, fmt.Sprintf("Your body is model 0x%08X; Revert Sprite gives yours back", model))
	}
}

// parseModel reads a model number: hex with 0x, else decimal (an int32's
// negative as #npcinfo can print one).
func parseModel(s string) (uint32, bool) {
	if h, ok := strings.CutPrefix(strings.ToLower(s), "0x"); ok {
		v, err := strconv.ParseUint(h, 16, 32)
		return uint32(v), err == nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil || v < math.MinInt32 || v > math.MaxUint32 {
		return 0, false
	}
	return uint32(v), true
}

// gmGotoLocation is Goto Location, "world x y z" (or "x y z" in the GM's
// world), as #goto x y z world.
func (h *ChatHandler) gmGotoLocation(sess *session.Session, gm *entity.Player, text string) {
	f := strings.Fields(text)
	world := int(gm.World)
	switch len(f) {
	case 4:
		w, err := strconv.Atoi(f[0])
		if err != nil || w < 0 || w >= len(worldNames) {
			h.gmReply(sess, fmt.Sprintf("Goto Location: %q is no world (0..%d)", f[0], len(worldNames)-1))
			return
		}
		world, f = w, f[1:]
	case 3:
	default:
		h.gmReply(sess, "Goto Location: world x y z")
		return
	}
	var v [3]float64
	for i := range v {
		n, err := strconv.ParseFloat(f[i], 32)
		if err != nil {
			h.gmReply(sess, fmt.Sprintf("Goto Location: %q is no number", f[i]))
			return
		}
		v[i] = n
	}
	if !h.teleportTo(sess, sess, gm, float32(v[0]), float32(v[1]), float32(v[2]), gm.Facing, byte(world)) {
		return
	}
	h.gmReply(sess, fmt.Sprintf("Teleported to (%.0f, %.0f, %.0f) world=%s", v[0], v[1], v[2], worldName(byte(world))))
	log.Printf("GM: %s went to (%.1f, %.1f, %.1f) world=%d", gm.Name, v[0], v[1], v[2], world)
}

// gmSpawner is Goto Spawner, Force Spawn, Kill Spawner and Unkill Spawner on
// the NPC a spawner number names (its npcs row, else an entity id:
// #npcinfo's NPCID= and ServerID=).
func (h *ChatHandler) gmSpawner(sess *session.Session, gm *entity.Player, sub byte, n int) {
	id, ok := h.gameWorld.NPCBySpawn(n)
	if !ok {
		h.gmReply(sess, fmt.Sprintf("No spawner %d (an NPCID or a ServerID, as #npcinfo shows them)", n))
		return
	}
	st, _ := h.gameWorld.NPCStateOf(id)
	switch sub {
	case gmSubGotoSpawner:
		if !h.teleportTo(sess, sess, gm, st.SpawnX, st.SpawnY, st.SpawnZ, gm.Facing, st.World) {
			return
		}
		h.gmReply(sess, fmt.Sprintf("Teleported to %s's spawn - (%.0f, %.0f, %.0f) world=%s", st.Name, st.SpawnX, st.SpawnY, st.SpawnZ, worldName(st.World)))
	case gmSubForceSpawn:
		if h.gameWorld.RespawnNPCNow(id) {
			h.gmReply(sess, fmt.Sprintf("%s spawned on its spawn point", st.Name))
		}
	case gmSubKillSpawner:
		if name, ok := h.gameWorld.HoldNPC(id, true); ok {
			h.gmReply(sess, fmt.Sprintf("%s is dead and stays so until Unkill Spawner (or a restart)", name))
			log.Printf("GM: %s held spawner %d (%s)", gm.Name, n, name)
		} else {
			h.gmReply(sess, fmt.Sprintf("%s cannot be held (a #spawn copy or a training dummy)", st.Name))
		}
	case gmSubUnkillSpawner:
		if name, ok := h.gameWorld.HoldNPC(id, false); ok {
			h.gmReply(sess, fmt.Sprintf("%s comes back again; spawned now if it was dead", name))
			log.Printf("GM: %s released spawner %d (%s)", gm.Name, n, name)
		}
	}
}

// gmKill is KILL on the GM's target or on the GM itself: an NPC as #kill
// does, a player as a death (its experience debt, its respawn).
func (h *ChatHandler) gmKill(sess *session.Session, gm *entity.Player, id uint32) {
	if id == 0xFFFFFFFF || id == 0 {
		h.gmReply(sess, "No target selected")
		return
	}
	target, found := h.gameWorld.FindEntityByObjectID(sess, id)
	if !found {
		h.gmReply(sess, "Target not found")
		return
	}
	switch t := target.(type) {
	case *entity.NPC:
		if h.gameWorld.KillNPCNow(t.ServerID) {
			h.gmReply(sess, fmt.Sprintf("Killed %s", t.Name))
		} else {
			h.gmReply(sess, fmt.Sprintf("%s is dead already, or cannot die", t.Name))
		}
	case *entity.Player:
		ts := sess
		if t != gm {
			ts = h.gameWorld.FindSessionByEntityID(t.ServerID)
		}
		if h.gameWorld.KillPlayerNow(ts, t.ServerID) {
			h.gmReply(sess, fmt.Sprintf("Killed %s", t.Name))
			log.Printf("GM: %s killed %s", gm.Name, t.Name)
		} else {
			h.gmReply(sess, fmt.Sprintf("%s is dead already", t.Name))
		}
	}
}

// gmHeal is HEAL on the GM's target or on the GM itself: an NPC or a player
// to full health (a player's power too).
func (h *ChatHandler) gmHeal(sess *session.Session, gm *entity.Player, id uint32) {
	if id == 0xFFFFFFFF || id == 0 {
		h.gmReply(sess, "No target selected")
		return
	}
	target, found := h.gameWorld.FindEntityByObjectID(sess, id)
	if !found {
		h.gmReply(sess, "Target not found")
		return
	}
	switch t := target.(type) {
	case *entity.NPC:
		if name, ok := h.gameWorld.HealNPC(t.ServerID); ok {
			h.gmReply(sess, fmt.Sprintf("Healed %s", name))
		} else {
			h.gmReply(sess, fmt.Sprintf("%s is dead", t.Name))
		}
	case *entity.Player:
		if h.gameWorld.HealPlayer(t.CharServerID) {
			h.gmReply(sess, fmt.Sprintf("Healed %s", t.Name))
		} else {
			h.gmReply(sess, fmt.Sprintf("%s is dead", t.Name))
		}
	}
}

// gmDebt is Remove 1 Debt (one death's at the player's level) and Remove
// All Debt, on the GM's target or on the GM.
func (h *ChatHandler) gmDebt(sess *session.Session, gm *entity.Player, id uint32, all bool) {
	t, _, ok := h.gmPlayerTarget(sess, gm, id, "Remove Debt")
	if !ok {
		return
	}
	debt, _ := h.gameWorld.DebtOf(t.ServerID)
	left := 0
	if !all {
		left = max(0, debt-entity.DeathDebt(t.Level))
	}
	h.gameWorld.SetDebt(t.ServerID, left)
	h.gmReply(sess, fmt.Sprintf("%s's experience debt: %d, was %d", t.Name, left, debt))
}

// whereIs is Where Is's line: the player's world, place and, when a town's
// coachman or a travel NPC stands within gmNearPlace, that town.
func (h *ChatHandler) whereIs(t *entity.Player) string {
	line := fmt.Sprintf("%s is in %s at (%.0f, %.0f, %.0f)", t.Name, worldName(t.World), t.X, t.Y, t.Z)
	best, bestD := "", float64(gmNearPlace)
	for _, p := range h.gameWorld.Places(nil) {
		if p.World != t.World {
			continue
		}
		if d := math.Hypot(float64(p.X-t.X), float64(p.Z-t.Z)); d < bestD {
			best, bestD = p.Name, d
		}
	}
	if best != "" {
		line += fmt.Sprintf(", %.0f from %s", bestD, best)
	}
	return line
}

// gmNearPlace is how near a town Where Is names it.
const gmNearPlace = 1000

// whoIs is Who Is's lines.
func (h *ChatHandler) whoIs(t *entity.Player, ts *session.Session) []string {
	lines := []string{fmt.Sprintf("%s: level %d %s %s", t.Name, t.Level, entity.RaceName(t.Race), entity.ClassName(t.Class))}
	if t.GuildName != "" {
		lines = append(lines, "  guild: "+t.GuildName)
	}
	level := 0
	if h.access != nil {
		level, _ = h.access.AccessLevel(ts.AccountID())
	}
	lines = append(lines, fmt.Sprintf("  account %d (level %d), character %d", ts.AccountID(), level, t.CharServerID))
	lines = append(lines, "  "+h.whereIs(t))
	return lines
}

// gmSilence is Silence, a toggle: a silenced player's say, shout, tell,
// group and guild lines go nowhere (gmChatGate), until the GM silences it
// again or the server restarts.
func (h *ChatHandler) gmSilence(sess *session.Session, gm *entity.Player, name string) {
	t, ts, ok := h.gmNamed(sess, name, "Silence")
	if !ok {
		return
	}
	h.gm.mu.Lock()
	if h.gm.silenced == nil {
		h.gm.silenced = make(map[uint32]bool)
	}
	on := !h.gm.silenced[t.CharServerID]
	if on {
		h.gm.silenced[t.CharServerID] = true
	} else {
		delete(h.gm.silenced, t.CharServerID)
	}
	h.gm.mu.Unlock()
	if on {
		h.gmReply(sess, fmt.Sprintf("%s is silenced (Silence again lets it speak)", t.Name))
		SendErrorMessage(ts, "You have been silenced by a GM.")
	} else {
		h.gmReply(sess, fmt.Sprintf("%s may speak again", t.Name))
		SendErrorMessage(ts, "You may speak again.")
	}
	log.Printf("GM: %s silenced %s: %v", gm.Name, t.Name, on)
}

// gmChatGate is every chat line's check (say, shout, tells, group, guild):
// false for a silenced speaker (told so), and the line copied to the GMs who
// snoop the speaker. kind names the channel for the copy.
func (h *ChatHandler) gmChatGate(sess *session.Session, p *entity.Player, kind, text string) bool {
	h.gm.mu.Lock()
	silenced := h.gm.silenced[p.CharServerID]
	var snoopers []entity.EntityID
	for id := range h.gm.snoops[p.ServerID] {
		snoopers = append(snoopers, id)
	}
	h.gm.mu.Unlock()
	for _, id := range snoopers {
		if s := h.gameWorld.FindSessionByEntityID(id); s != nil {
			h.gmReply(s, fmt.Sprintf("Snoop: %s (%s): %s", p.Name, kind, text))
		}
	}
	if silenced {
		SendErrorMessage(sess, "You have been silenced by a GM.")
		return false
	}
	return true
}

// gmSnoop is Snoop on the GM's target (a player): its chat lines copied to
// the GM until End Snoop or either leaves the world.
func (h *ChatHandler) gmSnoop(sess *session.Session, gm *entity.Player, id uint32) {
	if id == 0xFFFFFFFF || id == 0 {
		h.gmReply(sess, "Snoop takes a player: target one")
		return
	}
	t, _, ok := h.gmPlayerTarget(sess, gm, id, "Snoop")
	if !ok {
		return
	}
	if t == gm {
		h.gmReply(sess, "That is you.")
		return
	}
	h.gm.mu.Lock()
	if h.gm.snoops == nil {
		h.gm.snoops = make(map[entity.EntityID]map[entity.EntityID]bool)
	}
	if h.gm.snoops[t.ServerID] == nil {
		h.gm.snoops[t.ServerID] = make(map[entity.EntityID]bool)
	}
	h.gm.snoops[t.ServerID][gm.ServerID] = true
	h.gm.mu.Unlock()
	h.gmReply(sess, fmt.Sprintf("Snooping %s: what it says comes to you until End Snoop", t.Name))
	log.Printf("GM: %s snoops %s", gm.Name, t.Name)
}

// endSnoops ends every snoop of the GM's player, reporting whether it had
// one.
func (g *gmChannel) endSnoops(gm entity.EntityID) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	had := false
	for id, gms := range g.snoops {
		if gms[gm] {
			had = true
			delete(gms, gm)
		}
		if len(gms) == 0 {
			delete(g.snoops, id)
		}
	}
	return had
}

// gmLeft forgets what a player leaving the world held (World's
// OnPlayerLeaving): its snoops, the snoops on it, its Give Item. A network
// counter asked for of it is dropped with its session's next 0x07E1, which
// does not come.
func (h *ChatHandler) gmLeft(p *entity.Player) {
	h.gm.endSnoops(p.ServerID)
	h.gm.mu.Lock()
	defer h.gm.mu.Unlock()
	delete(h.gm.snoops, p.ServerID)
	delete(h.gm.gifts, p.ServerID)
}

// gmFreeze is Freeze, a toggle: the player's speed 0 until the GM lets it go
// (its own speed then, with whatever its effects give).
func (h *ChatHandler) gmFreeze(sess *session.Session, gm *entity.Player, name string) {
	t, ts, ok := h.gmNamed(sess, name, "Freeze")
	if !ok {
		return
	}
	on, speed, ok := h.gameWorld.ToggleFrozen(t.ServerID)
	if !ok {
		return
	}
	SendPlayerSpeed(ts, speed)
	if on {
		h.gmReply(sess, fmt.Sprintf("%s is frozen (Freeze again lets it go)", t.Name))
		if ts != sess {
			SendErrorMessage(ts, "A GM has frozen you in place.")
		}
	} else {
		h.gmReply(sess, fmt.Sprintf("%s can move again", t.Name))
		if ts != sess {
			SendErrorMessage(ts, "You can move again.")
		}
	}
	log.Printf("GM: %s froze %s: %v", gm.Name, t.Name, on)
}

// gmRename is Change Name on the GM's target (or the GM): the creation's
// rules (letters only, no reserved or profane word) and the client's own 12
// at most, another character's name refused; kept in the characters table
// and taken by the world at once.
func (h *ChatHandler) gmRename(sess *session.Session, gm *entity.Player, id uint32, name string) {
	t, _, ok := h.gmPlayerTarget(sess, gm, id, "Change Name")
	if !ok {
		return
	}
	name = strings.TrimSpace(name)
	if len(name) > 12 || !validCharName(name) {
		h.gmReply(sess, fmt.Sprintf("Change Name: %q is not a name a character may have (letters, 3 to 12)", name))
		return
	}
	name = strings.ToUpper(name[:1]) + strings.ToLower(name[1:])
	h.gm.mu.Lock()
	names := h.gm.names
	h.gm.mu.Unlock()
	if names == nil {
		h.gmReply(sess, "Change Name: this server keeps no characters")
		return
	}
	if err := names.Rename(t.CharServerID, name); err != nil {
		if errors.Is(err, database.ErrNameTaken) {
			h.gmReply(sess, fmt.Sprintf("Change Name: %s is taken", name))
		} else {
			h.gmReply(sess, fmt.Sprintf("Change Name: %v", err))
		}
		return
	}
	old, _ := h.gameWorld.RenamePlayer(t.ServerID, name)
	h.gmReply(sess, fmt.Sprintf("%s is now %s; its own client shows it from its next login", old, name))
	log.Printf("GM: %s renamed %s (character %d) to %s", gm.Name, old, t.CharServerID, name)
}

// validCharName is the character creation's name rules
// (CharacterHandler.isValidName, which reads nothing of its handler).
func validCharName(name string) bool {
	var h *CharacterHandler
	return h.isValidName(name)
}

// gmDisband is Disband Guild: the guild of the character of that name ends,
// as its leader's disband does, told to every member online.
func (h *ChatHandler) gmDisband(sess *session.Session, gm *entity.Player, name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		h.gmReply(sess, "Disband Guild: a member's name")
		return
	}
	g, online, err := h.gameWorld.DisbandGuildOfMember(name)
	if err != nil {
		h.gmReply(sess, fmt.Sprintf("Disband Guild: no guild has a member named %q", name))
		return
	}
	for _, m := range online {
		if s := h.gameWorld.FindSessionByEntityID(m.ServerID); s != nil {
			queueReliable(s, protocol.OpcodeGuildInfo, entity.BuildGuildWord(entity.GuildWordDisbanded))
			queueReliable(s, protocol.OpcodeGuildInfo, entity.BuildGuildReset())
		}
	}
	h.gmReply(sess, fmt.Sprintf("Disbanded %q (%d members online told)", g.Name, len(online)))
	log.Printf("GM: %s disbanded %q", gm.Name, g.Name)
}

// waitNetStats notes a GM asking for the counters of the player's session.
func (g *gmChannel) waitNetStats(player *session.Session, gm entity.EntityID) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.netWait == nil {
		g.netWait = make(map[*session.Session][]entity.EntityID)
	}
	for _, id := range g.netWait[player] {
		if id == gm {
			return
		}
	}
	g.netWait[player] = append(g.netWait[player], gm)
}

// gmNetStats is a 0x07E1 (connstats.go): its lines to the GMs that asked for
// that session's counters (Network Stats), once.
func (h *ChatHandler) gmNetStats(sess *session.Session, text string) {
	h.gm.mu.Lock()
	gms := h.gm.netWait[sess]
	delete(h.gm.netWait, sess)
	h.gm.mu.Unlock()
	if len(gms) == 0 {
		return
	}
	who := "the player"
	if p := h.gameWorld.SessionPlayer(sess); p != nil {
		who = p.Name
	}
	for _, id := range gms {
		gm := h.gameWorld.FindSessionByEntityID(id)
		if gm == nil {
			continue
		}
		h.gmReply(gm, who+"'s network counters:")
		for _, l := range strings.Split(text, "\n") {
			if l = strings.TrimSpace(l); l != "" {
				h.gmReply(gm, "  "+l)
			}
		}
	}
}

// gmCreate is the GM's /create, "item [count]": an item by its number or
// its name to the GM (as Summon Item; count 1 to 100, 1 without one).
func (h *ChatHandler) gmCreate(sess *session.Session, gm *entity.Player, text string) {
	f := strings.Fields(text)
	if len(f) == 0 {
		h.gmReply(sess, "/create <item number or name> [count]")
		return
	}
	n := 1
	if len(f) > 1 {
		if c, err := strconv.Atoi(f[len(f)-1]); err == nil {
			n, f = c, f[:len(f)-1]
		}
	}
	if n < 1 || n > 100 {
		h.gmReply(sess, "/create: a count of 1 to 100")
		return
	}
	what := strings.Join(f, " ")
	if id, err := strconv.Atoi(what); err == nil {
		h.gmGiveNow(sess, gm, sess, id, n)
		return
	}
	h.gm.mu.Lock()
	inv := h.gm.inv
	h.gm.mu.Unlock()
	if inv == nil {
		h.gmReply(sess, "Items: this server keeps none")
		return
	}
	p, ok, err := inv.ItemPatternByName(what)
	switch {
	case err != nil:
		h.gmReply(sess, fmt.Sprintf("/create %q: %v", what, err))
	case !ok:
		h.gmReply(sess, fmt.Sprintf("/create: no item named %q", what))
	default:
		h.gmGivePattern(sess, gm, sess, p, n)
	}
}

// chatModeName names a 0x000E mode for a snoop's copy.
func chatModeName(mode byte) string {
	switch mode {
	case ChatModeWhisper:
		return "whisper"
	case ChatModeGroup:
		return "group"
	case ChatModeShout:
		return "shout"
	case ChatModeGuild:
		return "guild"
	}
	return "say"
}
