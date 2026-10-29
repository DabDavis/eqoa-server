package handlers

import (
	"fmt"
	"log"
	"math/rand"
	"sort"
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

// Tradeskills, as CLIENT's VIClientPart3834C drives them (decomp
// viclientpart3834c.cpp; claude-notes/tradeskill-protocol-20261029.md):
// requests on 0x1101 {u8 sub-op, u32 token, args}, replies on 0x1102 {u8
// sub-op, ...}. The token is the server's and the client only echoes it;
// Sandstorm's is 0 in the dump and in its replies, and so is this server's.
//
//	1 {u32 id}          remove a recipe     -> 2 {u32 id, u32 token}
//	2 {u32 id, u32 0}   create from a book  -> 9 made, 10 failed, 11 the tool broke,
//	                                           5 not yet recovered, 3 / 7 refused
//	3 {u32 from, to}    arrange the book    -> 8 {u32 token, from, to}
//	4 {zz n, n zz ids}  experiment          -> 12 {u8 op, u32 token, ...}
//	5 {u32 x}           scribe a discovery  -> 1 {u32 token}
//	6 {u32 skill}       the skill's book    -> 15 {u32 token, u32 n, n recipes}
//	7 {u32 id}          inspect a recipe    -> 16 {u32 token, detail}
//
// Every request but 5 sets the client's pending flag and refuses to go out
// again until a reply clears it (§1): each is answered, a refusal too (3 or
// 7, silent, beside a red line of ours saying why).
//
// The recipes, the characters' skills and books are tables
// (database/craft.go); the rules and their numbers, ours, are craftrules.go;
// how items move, craftitems.go.

const (
	opcodeCraftRequest protocol.GameOpcode = 0x1101
	opcodeCraftReply   protocol.GameOpcode = 0x1102
)

// The requests' sub-ops.
const (
	craftReqRemove  byte = 1
	craftReqCreate  byte = 2
	craftReqArrange byte = 3
	craftReqCombine byte = 4
	craftReqScribe  byte = 5
	craftReqBook    byte = 6
	craftReqInspect byte = 7
)

// The replies' sub-ops (Recv1102 0x00BFE118).
const (
	craftRepScribed    byte = 1  // {token}: "You have scribed a new recipe." (word 1544)
	craftRepRemoved    byte = 2  // {id, token}: the recipe leaves the client's book
	craftRepRefused    byte = 3  // nothing: silent
	craftRepRecovering byte = 5  // {token}: "You have not yet recovered." in red (1567)
	craftRepUnknown    byte = 7  // nothing: silent
	craftRepMoved      byte = 8  // {token, from, to}: Recv08's reorder
	craftRepMade       byte = 9  // the skill report in the Recipe Book's box
	craftRepFailed     byte = 10 // {token, reagentBroke}: "You fail to create anything."
	craftRepToolBroke  byte = 11 // {token, reagentBroke}: "You broke your tool."
	craftRepExperiment byte = 12 // {op, token, ...}
	craftRepSlots      byte = 14 // the six slots, no token
	craftRepBook       byte = 15 // {token, n, recipes}
	craftRepDetail     byte = 16 // {token, detail}
)

// 12's ops (Recv12 0x00BFEC78); the Experiment window shows each.
const (
	craftExpNothing    byte = 2 // "The items you chose do not make anything." (1599)
	craftExpTooLow     byte = 3 // "Your tradeskill level is not high enough to create that item." (1598)
	craftExpFailed     byte = 4 // {reagentBroke, took}: "You fail to create anything."
	craftExpToolBroke  byte = 5 // {reagentBroke, took}: "You broke your tool."
	craftExpMade       byte = 6 // the skill report, took, the item
	craftExpDiscovered byte = 7 // the same, x2C the recipe: "Would you like to add this to your recipe book?"
)

// craftToken is the token every reply carries (Sandstorm's 0).
const craftToken uint32 = 0

// craftCombineMax is what an Experiment's combine holds at most
// (AddToCombine's four).
const craftCombineMax = 4

// craftBookMax is the most recipes one book reply lists. A guaranteed DRDP
// message may be 5,324,800 bytes (connection_get_max_message_size,
// claude-notes/decomp-phase48-the-connection-20260828.md) and a recipe is 164
// at most on the wire, so the whole of any book fits one reply, which the
// session's queue cuts into its 1,156-byte segments (session/queue.go). The
// cap is for the console's heap: CLIENT keeps the book in a VIPool of
// 152-byte nodes, doubling it as it grows (1,000 recipes are 1,024 nodes,
// 155 KB), and a failed Add empties the book. Past it the book's first
// craftBookMax recipes in its order are sent.
const craftBookMax = 1000

// The lines the server says beside a refusal, in Sandstorm's red. The level
// one is CLIENT's own word 1598; the rest are ours.
const (
	craftSayTooLow     = "Your tradeskill level is not high enough to create that item."
	craftSayNoTool     = "You need your %s tool equipped."
	craftSayToolWeak   = "Your %s is not good enough for that."
	craftSayToolBroken = "Your %s is broken."
	craftSayLacking    = "You do not have what this recipe needs."
	craftSayNoRoom     = "You have no room for it."
	craftSayRecovering = "You have not yet recovered."
	craftSayCannotMake = "That recipe cannot be made here yet."
	craftSayKnown      = "You already know that recipe."
)

// craftSession is what one session's crafting keeps between requests.
type craftSession struct {
	sess      *session.Session
	attempted time.Time // its last attempt (craftCooldown)
	discovery int       // the recipe its last 12/7 offered to scribe, 0 none
	// found is the recipes a discovery's XP was given for this session: a
	// recipe not scribed stays out of the book, so making it again is a
	// 12/7 again, at a make's XP (ours: else declining the question would
	// give ten times the XP every time)
	found map[int]bool
}

// CraftHandler serves 0x1101.
type CraftHandler struct {
	db    *database.SQLiteDB
	inv   *database.InventoryStore
	items *database.ItemPatternStore

	// view is the World's CraftView, roll the dice (0 to 1), now the clock
	// (tests stand in for each).
	view func(*session.Session) (entity.CraftView, bool)
	roll func() float64
	now  func() time.Time

	mu     sync.Mutex
	states map[uint32]*craftSession // by character
}

// NewCraftHandler makes the handler over the database (nil: every request
// is refused, answered all the same).
func NewCraftHandler(db *database.SQLiteDB, inv *database.InventoryStore) *CraftHandler {
	h := &CraftHandler{db: db, inv: inv, roll: rand.Float64, now: time.Now, states: map[uint32]*craftSession{}}
	if db != nil {
		h.items = database.NewItemPatternStore(db)
	}
	return h
}

// SetGameWorld gives the handler the World's view of a player, and forgets a
// player's crafting as it leaves the world.
func (h *CraftHandler) SetGameWorld(gw *entity.World) {
	if gw == nil {
		return
	}
	h.view = gw.CraftView
	gw.OnPlayerLeaving(func(p *entity.Player) {
		h.mu.Lock()
		delete(h.states, p.CharServerID)
		h.mu.Unlock()
	})
}

func (h *CraftHandler) RegisterHandlers(r *Registry) {
	r.Register(opcodeCraftRequest, h.HandleCraft)
}

// --- replies ---

// craftWriteMax is the most bytes any one write of a reply's makes (a 64-bit
// zigzag; a string is written a unit at a time).
const craftWriteMax = 10

// buildFitted is build with a buffer as large as the message needs: a
// buffer.Writer refuses a write past its end without a word and goes on, so
// a message written into too small a buffer comes out short (the stand-in's
// fixed 4,096 bytes cut a book past about 27 recipes, and CLIENT's ReadList
// fills the rest with garbage). It is written again into a buffer twice the
// size until more room is left over than any one write takes: a refused
// write leaves less than that, and room never comes back.
func buildFitted(op protocol.GameOpcode, fill func(w *buffer.Writer)) []byte {
	for size := 256; ; size *= 2 {
		buf := make([]byte, size)
		w := buffer.NewWriter(buf)
		w.WriteUint16(uint16(op))
		fill(w)
		if w.Remaining() > craftWriteMax {
			return buf[:w.Position()]
		}
	}
}

// craftReply is 0x1102 with its sub-op and what fill writes.
func craftReply(sub byte, fill func(w *buffer.Writer)) []byte {
	return buildFitted(opcodeCraftReply, func(w *buffer.Writer) {
		w.WriteByte(sub)
		if fill != nil {
			fill(w)
		}
	})
}

// craftSilent is a reply of the sub-op alone (3, 7).
func craftSilent(sub byte) []byte { return craftReply(sub, nil) }

// craftTokenReply is a reply of the sub-op and the token (1, 5).
func craftTokenReply(sub byte) []byte {
	return craftReply(sub, func(w *buffer.Writer) { w.WriteUint32(craftToken) })
}

func craftRemovedPayload(id uint32) []byte {
	return craftReply(craftRepRemoved, func(w *buffer.Writer) {
		w.WriteUint32(id)
		w.WriteUint32(craftToken)
	})
}

func craftMovedPayload(from, to uint32) []byte {
	return craftReply(craftRepMoved, func(w *buffer.Writer) {
		w.WriteUint32(craftToken)
		w.WriteUint32(from)
		w.WriteUint32(to)
	})
}

// craftReport is the skill report 9, 12/6 and 12/7 carry: the slot's new
// level and rank (absolute: CLIENT replaces its slot with them) and the XP
// the craft gave.
type craftReport struct {
	slot  int
	skill craftSkill
	xp    int
}

func (r craftReport) write(w *buffer.Writer) {
	s := r.skill.slot()
	w.WriteUint32(0) // ok: 0 or more, the slot is updated
	w.WriteUint32(uint32(r.slot))
	w.WriteUint32(s[1])
	w.WriteUint32(s[2])
	w.WriteUint32(s[3])
	w.WriteUint32(uint32(r.xp))
}

// craftMadePayload is 9 (Recv09): the report, no reagent broken, the
// product's icon for the box and its name for "You have created "%S.""
func craftMadePayload(r craftReport, icon uint32, name string) []byte {
	return craftReply(craftRepMade, func(w *buffer.Writer) {
		w.WriteUint32(craftToken)
		r.write(w)
		w.WriteUint32(0) // reagentBroke
		w.WriteUint32(icon)
		writeWideString(w, name, 128)
	})
}

// craftFailedPayload is 10 (failed) or 11 (the tool broke): reagentBroke adds
// "You broke a recipe reagent."
func craftFailedPayload(sub byte, reagentBroke bool) []byte {
	return craftReply(sub, func(w *buffer.Writer) {
		w.WriteUint32(craftToken)
		w.WriteUint32(craftBool(reagentBroke))
	})
}

func craftBool(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}

// craftExpPayload is 12 with an op that carries nothing more (2, 3).
func craftExpPayload(op byte) []byte {
	return craftReply(craftRepExperiment, func(w *buffer.Writer) {
		w.WriteByte(op)
		w.WriteUint32(craftToken)
	})
}

func writeCraftTook(w *buffer.Writer, took [][2]uint32) {
	w.WriteUint32(uint32(len(took)))
	for _, t := range took {
		w.WriteUint32(t[0])
		w.WriteUint32(t[1])
	}
}

// craftExpFailedPayload is 12/4 or 12/5 (TookItems, TookItems2): what the
// attempt took off the client's copy of the list.
func craftExpFailedPayload(op byte, took [][2]uint32) []byte {
	return craftReply(craftRepExperiment, func(w *buffer.Writer) {
		w.WriteByte(op)
		w.WriteUint32(craftToken)
		w.WriteUint32(craftBool(len(took) > 0))
		writeCraftTook(w, took)
	})
}

// craftExpMadePayload is 12/6 or 12/7 (ItemResult): the report, no reagent
// broken, x2C (the recipe, which a 12/7's Yes sends back in sub-op 5), what
// it took, and the item made, which the client files in its copy of the list
// (AddItem: its key is the client's own).
func craftExpMadePayload(op byte, r craftReport, recipe int, took [][2]uint32, item *database.CharInventoryEntry, key uint32) []byte {
	return craftReply(craftRepExperiment, func(w *buffer.Writer) {
		w.WriteByte(op)
		w.WriteUint32(craftToken)
		r.write(w)
		w.WriteUint32(0) // reagentBroke
		w.WriteUint32(uint32(recipe))
		writeCraftTook(w, took)
		writeSingleItem(w, item, int(key))
	})
}

// craftSlotsPayload is 14: the six slots, no token (ReadSlots 0x00BFD178).
func craftSlotsPayload(slots [database.CraftSkills][4]uint32) []byte {
	return craftReply(craftRepSlots, func(w *buffer.Writer) {
		for _, s := range slots {
			for _, v := range s {
				w.WriteUint32(v)
			}
		}
	})
}

// craftBookEntry is a recipe in the book's reply, its place in the order.
type craftBookEntry struct {
	recipe *database.CraftRecipe
	ord    int
}

// craftBookPayload is 15 (Recv15): the raw u32 count, then each recipe as
// VIRec144_C017C0::Read reads one: zigzag x00 (never read: 0), the order,
// the id; u8 level, skill, component, rarity; the name, 63 characters at
// most.
func craftBookPayload(list []craftBookEntry) []byte {
	return craftReply(craftRepBook, func(w *buffer.Writer) {
		w.WriteUint32(craftToken)
		w.WriteUint32(uint32(len(list)))
		for _, e := range list {
			r := e.recipe
			w.Write7BitEncodedInt64(0)
			w.Write7BitEncodedInt64(int64(e.ord))
			w.Write7BitEncodedInt64(int64(r.ID))
			w.WriteByte(byte(r.Level))
			w.WriteByte(byte(r.Skill))
			w.WriteByte(byte(r.Comp))
			w.WriteByte(byte(r.Rarity))
			writeWideString(w, r.Name, 128)
		}
	})
}

// craftDetail is a recipe as its Inspect shows it.
type craftDetail struct {
	recipe      *database.CraftRecipe
	product     *database.ItemPatternRecord // nil: no such item here
	ingredients []craftDetailLine
	tool        string
	equipped    bool
}

type craftDetailLine struct {
	name  string
	count int
}

// craftDetailPayload is 16 (VICraftDetail::Read 0x00BFFE68): three zigzags
// nothing shows (the recipe, -1, -1), the recipe's level, skill and component
// (not shown), its rarity, the yield (u16), the product's level, name and
// description (the item's, else the recipe's), its stats as item stat ids,
// the ingredients (name, u16 count) and the reagents: the skill's tool, with
// "(equipped)" when one good enough for the recipe is in the hand.
func craftDetailPayload(d craftDetail) []byte {
	r := d.recipe
	name, desc, level := r.Name, r.Description, 0
	var stats map[int]int
	if p := d.product; p != nil {
		name, level, stats = p.ItemName, p.LevelReq, itemPatternStats(p)
		if strings.TrimSpace(p.ItemDesc) != "" {
			desc = p.ItemDesc
		}
	}
	ids := make([]int, 0, len(stats))
	for id := range stats {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return craftReply(craftRepDetail, func(w *buffer.Writer) {
		w.WriteUint32(craftToken)
		w.Write7BitEncodedInt64(int64(r.ID))
		w.Write7BitEncodedInt64(-1)
		w.Write7BitEncodedInt64(-1)
		w.WriteByte(byte(r.Level))
		w.WriteByte(byte(r.Skill))
		w.WriteByte(byte(r.Comp))
		w.WriteByte(byte(r.Rarity))
		w.WriteUint16(uint16(r.Yield))
		w.WriteByte(byte(min(max(level, 0), 255)))
		writeWideString(w, name, 128)
		writeWideString(w, desc, 512)
		w.Write7BitEncodedInt64(int64(len(ids)))
		for _, id := range ids {
			w.Write7BitEncodedInt64(int64(id))
			w.Write7BitEncodedInt64(int64(stats[id]))
		}
		w.Write7BitEncodedInt64(int64(len(d.ingredients)))
		for _, in := range d.ingredients {
			writeWideString(w, in.name, 128)
			w.WriteUint16(uint16(min(in.count, 65535)))
		}
		w.Write7BitEncodedInt64(1)
		writeWideString(w, d.tool, 128)
		w.WriteByte(byte(craftBool(d.equipped)))
	})
}

// --- requests ---

// craftAnswer is what a request is answered with: the reply (every request
// that sets the client's pending flag gets one), then the inventory's
// messages, then lines in red.
type craftAnswer struct {
	reply []byte
	batch craftBatch
	say   []string
}

func craftAnswerOf(reply []byte) craftAnswer { return craftAnswer{reply: reply} }

// refusal is a reply and a red line saying why.
func craftRefusal(reply []byte, line string) craftAnswer {
	return craftAnswer{reply: reply, say: []string{line}}
}

func (a craftAnswer) send(sess *session.Session) {
	if a.reply != nil {
		queueReliable(sess, opcodeCraftReply, a.reply)
	}
	a.batch.send(sess)
	for _, l := range a.say {
		SendColoredChat(sess, l, scribeRefusalRGB[0], scribeRefusalRGB[1], scribeRefusalRGB[2])
	}
}

// craftUnanswered is the silent reply that clears a request's pending flag
// when nothing else can be said (a short request, no character, no
// database): 12/2 for an Experiment (its window shows that nothing was
// made), 7 for the rest, nothing for 5 (it sets none).
func craftUnanswered(sub byte) craftAnswer {
	switch sub {
	case craftReqScribe:
		return craftAnswer{}
	case craftReqCombine:
		return craftAnswerOf(craftExpPayload(craftExpNothing))
	case craftReqRemove, craftReqCreate, craftReqArrange, craftReqBook, craftReqInspect:
		return craftAnswerOf(craftSilent(craftRepUnknown))
	}
	return craftAnswer{} // no such request: the client sets no flag for it
}

// craftCtx is one request's: the player, its crafting state, the recipes,
// the session's copy of the client's list.
type craftCtx struct {
	sess    *session.Session
	view    entity.CraftView
	st      *craftSession
	recipes *database.CraftRecipes
	l       *invList
}

func (c *craftCtx) char() uint32 { return c.view.CharServerID }

// begin is the request's context, false when it cannot be served. Must be
// called with h.mu held.
func (h *CraftHandler) begin(sess *session.Session) (*craftCtx, bool) {
	if h.view == nil || h.db == nil || h.inv == nil {
		return nil, false
	}
	v, ok := h.view(sess)
	if !ok {
		return nil, false
	}
	recipes, err := h.db.CraftRecipes()
	if err != nil {
		log.Printf("Craft: %s: the recipes: %v", v.Name, err)
		return nil, false
	}
	st := h.states[v.CharServerID]
	if st == nil || st.sess != sess {
		st = &craftSession{sess: sess, found: map[int]bool{}}
		h.states[v.CharServerID] = st
	}
	return &craftCtx{sess: sess, view: v, st: st, recipes: recipes, l: sessionInvList(sess)}, true
}

// HandleCraft is 0x1101.
func (h *CraftHandler) HandleCraft(sess *session.Session, msg *protocol.Message) error {
	r := buffer.NewReader(msg.Data())
	sub, err := r.ReadByte()
	if err != nil {
		return nil
	}
	_, terr := r.ReadUint32() // the token, never compared
	h.mu.Lock()
	defer h.mu.Unlock()
	c, ok := h.begin(sess)
	var a craftAnswer
	switch {
	case terr != nil || !ok:
		a = craftUnanswered(sub)
	default:
		a = h.dispatch(c, sub, r)
	}
	a.send(sess)
	return nil
}

// dispatch answers one request. Must be called with h.mu held.
func (h *CraftHandler) dispatch(c *craftCtx, sub byte, r *buffer.Reader) craftAnswer {
	switch sub {
	case craftReqRemove:
		return h.remove(c, r)
	case craftReqCreate:
		return h.create(c, r)
	case craftReqArrange:
		return h.arrange(c, r)
	case craftReqCombine:
		return h.combine(c, r)
	case craftReqScribe:
		return h.scribe(c, r)
	case craftReqBook:
		return h.book(c, r)
	case craftReqInspect:
		return h.inspect(c, r)
	}
	log.Printf("Craft: %s: sub-op %d unknown", c.view.Name, sub)
	return craftAnswer{}
}

// bookOf is the character's book: each recipe in it the server has, in its
// order.
func (h *CraftHandler) bookOf(c *craftCtx) ([]craftBookEntry, error) {
	rows, err := h.db.CharRecipeBook(c.char())
	if err != nil {
		return nil, err
	}
	var out []craftBookEntry
	for _, row := range rows {
		if rec, ok := c.recipes.Recipe(row.Recipe); ok {
			out = append(out, craftBookEntry{recipe: rec, ord: row.Ord})
		}
	}
	return out, nil
}

// inBook is whether the recipe is in the character's book.
func inBook(book []craftBookEntry, id int) bool {
	for _, e := range book {
		if e.recipe.ID == id {
			return true
		}
	}
	return false
}

// book is 6: the skill's recipes in the book's order (15).
func (h *CraftHandler) book(c *craftCtx, r *buffer.Reader) craftAnswer {
	skill, err := r.ReadUint32()
	if err != nil {
		return craftUnanswered(craftReqBook)
	}
	all, err := h.bookOf(c)
	if err != nil {
		log.Printf("Craft: %s's book: %v", c.view.Name, err)
		return craftUnanswered(craftReqBook)
	}
	var list []craftBookEntry
	for _, e := range all {
		if uint32(e.recipe.Skill) == skill {
			list = append(list, e)
		}
	}
	if len(list) > craftBookMax {
		log.Printf("Craft: %s's %s book holds %d recipes: the first %d sent", c.view.Name, craftSkillName(int(skill)), len(list), craftBookMax)
		list = list[:craftBookMax]
	}
	return craftAnswerOf(craftBookPayload(list))
}

// remove is 1: the recipe out of the book (2, which the client's book
// follows); 7 when the book could not be changed (the client keeps it, as
// the server does).
func (h *CraftHandler) remove(c *craftCtx, r *buffer.Reader) craftAnswer {
	id, err := r.ReadUint32()
	if err != nil {
		return craftUnanswered(craftReqRemove)
	}
	removed, err := h.db.RemoveCharRecipe(c.char(), int(id))
	if err != nil {
		log.Printf("Craft: %s removing recipe %d: %v", c.view.Name, id, err)
		return craftUnanswered(craftReqRemove)
	}
	log.Printf("Craft: %s removed recipe %d (in the book: %v)", c.view.Name, id, removed)
	return craftAnswerOf(craftRemovedPayload(id))
}

// arrangeBook is Recv08's rule over the whole book (claude-notes §5): with at
// the to-recipe's order read first, every order above the from-recipe's
// goes down one, every one at or above at up one, and the from-recipe takes
// at: moved down it lands after to, moved up before it. The client applies
// it to its one skill's recipes, which keep exactly the orders it gives
// them. It returns the orders that changed, nil when from or to is not in
// the book.
func arrangeBook(book []database.CharRecipe, from, to int) map[int]int {
	f, t := -1, -1
	for i, e := range book {
		if e.Recipe == from {
			f = i
		}
		if e.Recipe == to {
			t = i
		}
	}
	if f < 0 || t < 0 {
		return nil
	}
	at, fo := book[t].Ord, book[f].Ord
	changed := map[int]int{}
	for i, e := range book {
		o := e.Ord
		if i == f {
			o = at
		} else {
			if o > fo {
				o--
			}
			if o >= at {
				o++
			}
		}
		if o != e.Ord {
			changed[e.Recipe] = o
		}
	}
	return changed
}

// arrange is 3: a recipe moved to another's place (8, which the client's
// book follows); 7 when either is not in the book or the book could not be
// changed.
func (h *CraftHandler) arrange(c *craftCtx, r *buffer.Reader) craftAnswer {
	from, e1 := r.ReadUint32()
	to, e2 := r.ReadUint32()
	if e1 != nil || e2 != nil {
		return craftUnanswered(craftReqArrange)
	}
	book, err := h.db.CharRecipeBook(c.char())
	if err != nil {
		log.Printf("Craft: %s's book: %v", c.view.Name, err)
		return craftUnanswered(craftReqArrange)
	}
	changed := arrangeBook(book, int(from), int(to))
	if changed == nil {
		log.Printf("Craft: %s moved recipe %d to %d, one of which the book lacks", c.view.Name, from, to)
		return craftUnanswered(craftReqArrange)
	}
	if err := h.db.SetCharRecipeOrders(c.char(), changed); err != nil {
		log.Printf("Craft: %s's book order: %v", c.view.Name, err)
		return craftUnanswered(craftReqArrange)
	}
	return craftAnswerOf(craftMovedPayload(from, to))
}

// inspect is 7: a recipe in the book as its Inspect shows it (16); 7 for one
// the book does not hold.
func (h *CraftHandler) inspect(c *craftCtx, r *buffer.Reader) craftAnswer {
	id, err := r.ReadUint32()
	if err != nil {
		return craftUnanswered(craftReqInspect)
	}
	book, err := h.bookOf(c)
	rec, ok := c.recipes.Recipe(int(id))
	if err != nil || !ok || !inBook(book, rec.ID) {
		return craftUnanswered(craftReqInspect)
	}
	d := craftDetail{recipe: rec}
	if p, err := h.items.GetItemPattern(rec.Product); err == nil {
		d.product = p
	}
	at := map[int]int{} // an item's line, its entries together
	for _, in := range rec.Ingredients {
		if i, ok := at[in.Item]; ok {
			d.ingredients[i].count += in.Count
			continue
		}
		name := fmt.Sprintf("item %d", in.Item)
		if p, err := h.items.GetItemPattern(in.Item); err == nil {
			name = p.ItemName
		}
		at[in.Item] = len(d.ingredients)
		d.ingredients = append(d.ingredients, craftDetailLine{name: name, count: in.Count})
	}
	d.tool = fmt.Sprintf("%s tool, tier %d", craftSkillName(rec.Skill), rec.Level)
	if rows, err := h.inv.GetCharacterItems(c.char()); err == nil {
		if t, ok := craftToolOf(rows, c.l, rec.Skill); ok {
			d.equipped = craftToolFits(t.row.Pattern.LevelReq, rec.Level)
		}
	}
	return craftAnswerOf(craftDetailPayload(d))
}

// craftAttempt is what an attempt at a recipe needs, checked before anything
// is taken.
type craftAttempt struct {
	recipe  *database.CraftRecipe
	skills  [database.CraftSkills]database.CraftSkill
	skill   craftSkill
	tool    craftTool
	product database.ItemPatternRecord
	rows    []database.CharInventoryEntry
	held    []craftHeldRow
	all     []craftTake // every ingredient
}

// craftCheck is why an attempt cannot be made, "" when it can, with whether
// the refusal is the level's (the Experiment's 12/3).
type craftCheck struct {
	line   string
	tooLow bool
}

// prepare checks an attempt at the recipe before anything is taken, as both
// the Recipe Book's Create and an Experiment make one: the skill live, the
// tool (worn in Held, good enough for the recipe's level, not broken), the
// skill's level, the product an item here, every ingredient held, and room
// for the product once they are taken. The cooldown is the callers'.
func (h *CraftHandler) prepare(c *craftCtx, rec *database.CraftRecipe) (craftAttempt, craftCheck) {
	a := craftAttempt{recipe: rec}
	var err error
	if a.skills, err = h.db.CharCraftSkills(c.char()); err != nil {
		log.Printf("Craft: %s's skills: %v", c.view.Name, err)
		return a, craftCheck{line: craftSayCannotMake}
	}
	a.skill = craftSkillOf(a.skills, rec.Skill)
	if !a.skill.live {
		return a, craftCheck{line: craftSayTooLow, tooLow: true}
	}
	if a.rows, err = h.inv.GetCharacterItems(c.char()); err != nil {
		log.Printf("Craft: %s's items: %v", c.view.Name, err)
		return a, craftCheck{line: craftSayCannotMake}
	}
	tool, ok := craftToolOf(a.rows, c.l, rec.Skill)
	switch {
	case !ok:
		return a, craftCheck{line: fmt.Sprintf(craftSayNoTool, craftSkillName(rec.Skill))}
	case !craftToolFits(tool.row.Pattern.LevelReq, rec.Level):
		return a, craftCheck{line: fmt.Sprintf(craftSayToolWeak, tool.row.Pattern.ItemName)}
	case tool.broken():
		return a, craftCheck{line: fmt.Sprintf(craftSayToolBroken, tool.row.Pattern.ItemName)}
	}
	a.tool = tool
	if rec.Level > a.skill.level {
		return a, craftCheck{line: craftSayTooLow, tooLow: true}
	}
	p, err := h.items.GetItemPattern(rec.Product)
	if err != nil {
		log.Printf("Craft: recipe %d %q makes item %d: %v", rec.ID, rec.Name, rec.Product, err)
		return a, craftCheck{line: craftSayCannotMake}
	}
	a.product = *p
	if p.MaxStack >= 2 && rec.Yield > p.MaxStack {
		log.Printf("Craft: recipe %d %q makes %d of %q, which stacks to %d", rec.ID, rec.Name, rec.Yield, p.ItemName, p.MaxStack)
		return a, craftCheck{line: craftSayCannotMake}
	}
	a.held = craftHeld(a.rows, c.l)
	if a.all, ok = planCraftTakes(a.held, craftNeeds(rec, false)); !ok {
		return a, craftCheck{line: craftSayLacking}
	}
	if !craftRoom(c.l, a.all, p, rec.Yield) {
		return a, craftCheck{line: craftSayNoRoom}
	}
	return a, craftCheck{}
}

// craftOutcome is how an attempt went.
type craftOutcome int

const (
	craftMade craftOutcome = iota
	craftFailed
	craftToolBroke
)

// attempt makes an attempt prepared: the cooldown starts, the tool wears
// (broken, the attempt fails), the dice roll; a failure takes what a failure
// loses, a success every ingredient, gives the product and the skill its XP,
// kept (a.skill is the skill after). It returns how it went, what it took,
// the records given and the XP the skill took. The messages go into the
// batch.
func (h *CraftHandler) attempt(c *craftCtx, a *craftAttempt, discovery bool, out *craftBatch) (craftOutcome, []craftTake, []craftGiven, int) {
	rec := a.recipe
	c.st.attempted = h.now()
	broke := h.wearTool(c.char(), a.tool, out)
	outcome := craftToolBroke
	chance := 0.0
	if !broke {
		chance = craftChance(a.skill.points(), rec.Difficulty, craftStatSum(rec.Skill, c.view.Stats))
		outcome = craftFailed
		if h.roll() < chance {
			outcome = craftMade
		}
	}
	if outcome != craftMade {
		lost, _ := planCraftTakes(a.held, craftNeeds(rec, true))
		taken := h.takeCraft(c.char(), c.l, a.held, lost, out)
		log.Printf("Craft: %s failed %q (recipe %d, chance %.2f, tool broke %v): %d lost", c.view.Name, rec.Name, rec.ID, chance, broke, len(taken))
		return outcome, taken, nil, 0
	}
	taken := h.takeCraft(c.char(), c.l, a.held, a.all, out)
	given := h.giveCraft(c.char(), c.l, a.product, rec.Yield, out)
	gain := a.skill.addXP(craftGain(rec.XP, rec.Level, a.skill.level, discovery))
	if err := h.db.SetCharCraftSkill(c.char(), rec.Skill, a.skill.row()); err != nil {
		log.Printf("Craft: %s's %s: %v", c.view.Name, craftSkillName(rec.Skill), err)
	}
	log.Printf("Craft: %s made %d %q (recipe %d, chance %.2f): %s level %d rank %d, +%d XP", c.view.Name, rec.Yield,
		a.product.ItemName, rec.ID, chance, craftSkillName(rec.Skill), a.skill.level, a.skill.rank, gain)
	return craftMade, taken, given, gain
}

// recovering is whether the session's last attempt was under craftCooldown
// ago.
func (h *CraftHandler) recovering(c *craftCtx) bool {
	return !c.st.attempted.IsZero() && h.now().Sub(c.st.attempted) < craftCooldown
}

// create is 2, the Recipe Book's Create Item: a recipe in the book made.
// Refused: 7 for one not in the book, 5 within the cooldown, else a silent
// 3 and a red line. Made: 9 with the product's name and icon; failed, 10;
// the tool broken, 11.
func (h *CraftHandler) create(c *craftCtx, r *buffer.Reader) craftAnswer {
	id, err := r.ReadUint32()
	if err != nil {
		return craftUnanswered(craftReqCreate)
	}
	book, err := h.bookOf(c)
	rec, ok := c.recipes.Recipe(int(id))
	if err != nil || !ok || !inBook(book, rec.ID) {
		log.Printf("Craft: %s asked to make recipe %d, which the book does not hold", c.view.Name, id)
		return craftAnswerOf(craftSilent(craftRepUnknown))
	}
	if h.recovering(c) {
		return craftAnswerOf(craftTokenReply(craftRepRecovering))
	}
	a, check := h.prepare(c, rec)
	if check.line != "" {
		return craftRefusal(craftSilent(craftRepRefused), check.line)
	}
	var out craftAnswer
	outcome, taken, given, gain := h.attempt(c, &a, false, &out.batch)
	switch outcome {
	case craftToolBroke:
		out.reply = craftFailedPayload(craftRepToolBroke, len(taken) > 0)
	case craftFailed:
		out.reply = craftFailedPayload(craftRepFailed, len(taken) > 0)
	default:
		if len(given) == 0 {
			log.Printf("Craft: %s made %q and nothing was given", c.view.Name, rec.Name)
		}
		out.reply = craftMadePayload(craftReport{rec.Skill, a.skill, gain}, craftItemIcon(&a.product), a.product.ItemName)
	}
	return out
}

// combine is 4, the Experiment: the combine's items (pattern ids, one a
// unit, at most four) made into the recipe whose ingredients they are
// exactly. The reply (12) goes before the inventory's messages, so a copy of
// the list the client takes between them is the one the reply's takes apply
// to (claude-notes §8).
func (h *CraftHandler) combine(c *craftCtx, r *buffer.Reader) craftAnswer {
	n, err := r.Read7BitEncodedInt64()
	if err != nil || n < 1 || n > craftCombineMax {
		return craftUnanswered(craftReqCombine)
	}
	units := map[int]int{}
	order := []int{} // the combine's items, first seen first
	for i := int64(0); i < n; i++ {
		id, err := r.Read7BitEncodedInt64()
		if err != nil {
			return craftUnanswered(craftReqCombine)
		}
		if units[int(id)] == 0 {
			order = append(order, int(id))
		}
		units[int(id)]++
	}
	rows, err := h.inv.GetCharacterItems(c.char())
	if err != nil {
		log.Printf("Craft: %s's items: %v", c.view.Name, err)
		return craftUnanswered(craftReqCombine)
	}
	held := craftHeld(rows, c.l)
	for id, k := range units {
		if craftHeldCount(held, id) < k {
			log.Printf("Craft: %s combined %d of item %d, holding fewer", c.view.Name, k, id)
			return craftAnswerOf(craftExpPayload(craftExpNothing))
		}
	}
	book, err := h.bookOf(c)
	if err != nil {
		log.Printf("Craft: %s's book: %v", c.view.Name, err)
		return craftUnanswered(craftReqCombine)
	}
	var rec *database.CraftRecipe
	for _, cand := range c.recipes.Combine(units) {
		if cand.Discover || inBook(book, cand.ID) {
			rec = cand
			break
		}
	}
	if rec == nil {
		return h.combineNothing(c, held, units, order)
	}
	discovery := !inBook(book, rec.ID)
	bonus := discovery && !c.st.found[rec.ID]
	a, check := h.prepare(c, rec)
	switch {
	case check.tooLow:
		return craftAnswerOf(craftExpPayload(craftExpTooLow))
	case check.line != "":
		return craftRefusal(craftExpPayload(craftExpNothing), check.line)
	case h.recovering(c):
		return craftRefusal(craftExpPayload(craftExpNothing), craftSayRecovering)
	}
	var out craftAnswer
	outcome, taken, given, gain := h.attempt(c, &a, bonus, &out.batch)
	took := craftTookList(taken)
	switch {
	case outcome == craftToolBroke:
		out.reply = craftExpFailedPayload(craftExpToolBroke, took)
	case outcome == craftFailed:
		out.reply = craftExpFailedPayload(craftExpFailed, took)
	case len(given) == 0:
		// the room was looked at first; a database that refused the
		// product leaves the client's copy as the takes left it
		log.Printf("Craft: %s made %q and nothing was given", c.view.Name, rec.Name)
		out.reply = craftExpFailedPayload(craftExpFailed, took)
	default:
		op := craftExpMade
		if discovery {
			op = craftExpDiscovered
			c.st.discovery = rec.ID
			c.st.found[rec.ID] = true
		}
		out.reply = craftExpMadePayload(op, craftReport{rec.Skill, a.skill, gain}, rec.ID, took, &given[0].rec, given[0].key)
	}
	return out
}

// combineNothing is an Experiment whose items make nothing: those some recipe
// loses on a failure are lost all the same (12/4, "You broke a recipe
// reagent."), else nothing is taken (12/2).
func (h *CraftHandler) combineNothing(c *craftCtx, held []craftHeldRow, units map[int]int, order []int) craftAnswer {
	var needs []craftNeed
	for _, id := range order {
		if c.recipes.ConsumedOnFail(id) {
			needs = append(needs, craftNeed{id, units[id]})
		}
	}
	if len(needs) == 0 {
		return craftAnswerOf(craftExpPayload(craftExpNothing))
	}
	takes, ok := planCraftTakes(held, needs)
	if !ok {
		return craftAnswerOf(craftExpPayload(craftExpNothing))
	}
	var out craftAnswer
	taken := h.takeCraft(c.char(), c.l, held, takes, &out.batch)
	log.Printf("Craft: %s's combine made nothing: %d of its items lost", c.view.Name, len(taken))
	out.reply = craftExpFailedPayload(craftExpFailed, craftTookList(taken))
	return out
}

// scribe is 5, a 12/7's Yes: the recipe it offered into the book, at its end
// (1); anything else is ignored (5 sets no pending flag).
func (h *CraftHandler) scribe(c *craftCtx, r *buffer.Reader) craftAnswer {
	x, err := r.ReadUint32()
	if err != nil || c.st.discovery == 0 || int(x) != c.st.discovery {
		return craftAnswer{}
	}
	if _, ok := c.recipes.Recipe(int(x)); !ok {
		return craftAnswer{}
	}
	added, err := h.db.AddCharRecipes(c.char(), int(x))
	if err != nil || len(added) == 0 {
		if err != nil {
			log.Printf("Craft: %s scribing recipe %d: %v", c.view.Name, x, err)
		}
		return craftAnswer{}
	}
	c.st.discovery = 0
	log.Printf("Craft: %s scribed recipe %d", c.view.Name, x)
	return craftAnswerOf(craftTokenReply(craftRepScribed))
}

// --- outside the windows ---

// craftSlotsOf is a character's six slots.
func craftSlotsOf(rows [database.CraftSkills]database.CraftSkill) [database.CraftSkills][4]uint32 {
	var out [database.CraftSkills][4]uint32
	for i := range out {
		out[i] = craftSkillOf(rows, i).slot()
	}
	return out
}

// craftSlotsDefault is the slots of a character with no rows: Sandstorm's,
// in every dump it sent ({99, 1, 0, 98} four times, {99, 99, 99, 98} twice).
func craftSlotsDefault() [database.CraftSkills][4]uint32 {
	return craftSlotsOf([database.CraftSkills]database.CraftSkill{})
}

// DumpSlots is the six slots the MemoryDump carries for a character
// (writeMemoryDumpTail); with no database, or one that fails, the defaults.
func (h *CraftHandler) DumpSlots(charID uint32) [database.CraftSkills][4]uint32 {
	if h == nil || h.db == nil {
		return craftSlotsDefault()
	}
	rows, err := h.db.CharCraftSkills(charID)
	if err != nil {
		log.Printf("Craft: character %d's skills for the dump: %v", charID, err)
		return craftSlotsDefault()
	}
	return craftSlotsOf(rows)
}

// SendCraftSlots sends a character's six slots to its session (14): its
// skills changed outside a craft (a GM's).
func SendCraftSlots(sess *session.Session, db *database.SQLiteDB, charID uint32) error {
	rows, err := db.CharCraftSkills(charID)
	if err != nil {
		return err
	}
	queueReliable(sess, opcodeCraftReply, craftSlotsPayload(craftSlotsOf(rows)))
	return nil
}

// UseScroll is a recipe scroll used from the inventory (the item menu's Use
// or a quick slot's, equip.go): the recipes it teaches that the book lacks
// put at its end, one scroll taken (0x003C) and CLIENT's own "You have
// scribed a new recipe." (1); a book that holds them all already is told so
// and keeps the scroll. It reports whether the item is a recipe scroll.
func (h *CraftHandler) UseScroll(sess *session.Session, charID uint32, idx int, row *database.CharInventoryEntry) bool {
	if h == nil || h.db == nil || h.inv == nil {
		return false
	}
	recipes, err := h.db.CraftRecipes()
	if err != nil {
		log.Printf("Craft: the recipes: %v", err)
		return false
	}
	teaches := recipes.Scroll(row.Pattern.PatternID)
	if len(teaches) == 0 {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	ids := make([]int, len(teaches))
	for i, r := range teaches {
		ids[i] = r.ID
	}
	added, err := h.db.AddCharRecipes(charID, ids...)
	if err != nil {
		log.Printf("Craft: character %d scribing %q: %v", charID, row.Pattern.ItemName, err)
		return true
	}
	if len(added) == 0 {
		craftRefusal(nil, craftSayKnown).send(sess)
		return true
	}
	// the recipes first: should taking the scroll then fail, the player
	// keeps a scroll of recipes known rather than losing one for nothing
	var out craftAnswer
	out.reply = craftTokenReply(craftRepScribed)
	l := sessionInvList(sess)
	var err2 error
	if row.StackLeft <= 1 {
		err2 = h.inv.DeleteCharItem(charID, row.ItemID)
	} else {
		err2 = h.inv.SetItemStack(charID, row.ItemID, row.StackLeft-1)
	}
	switch {
	case err2 != nil:
		log.Printf("Craft: character %d taking the scroll %q: %v (its recipes stay learned)", charID, row.Pattern.ItemName, err2)
	case l != nil && l.took(idx, 1):
		out.batch.add(protocol.OpcodeRemoveInvItem, removeInvItemPayload(idx, 0, 1))
	}
	out.send(sess)
	log.Printf("Craft: character %d scribed recipes %v from %q", charID, added, row.Pattern.ItemName)
	return true
}

// --- the GM's ---

// SetSkill is #craftskill: the GM's own skill set to a level and rank (its
// XP into the rank 0), kept, and the six slots sent (14).
func (h *CraftHandler) SetSkill(sess *session.Session, skill, level, rank int) (string, error) {
	if skill < 0 || skill >= database.CraftSkills {
		return "", fmt.Errorf("the skill is 0-5: %s", craftSkillList())
	}
	if level < database.CraftLevelMin || level > database.CraftLevelMax || rank < 0 || rank > craftRankMax {
		return "", fmt.Errorf("the level is 1-10 and the rank 0-9")
	}
	v, ok := h.gmView(sess)
	if !ok {
		return "", fmt.Errorf("tradeskills are not served here")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	s := craftSkill{level: level, rank: rank, live: true}
	if err := h.db.SetCharCraftSkill(v.CharServerID, skill, s.row()); err != nil {
		return "", err
	}
	if err := SendCraftSlots(sess, h.db, v.CharServerID); err != nil {
		return "", err
	}
	log.Printf("Craft: GM %s set %s to level %d rank %d", v.Name, craftSkillName(skill), level, rank)
	return fmt.Sprintf("%s: level %d, rank %d", craftSkillName(skill), level, rank), nil
}

// Skills is the GM's own six skills, for #craftskill with nothing after it.
func (h *CraftHandler) Skills(sess *session.Session) ([]string, error) {
	v, ok := h.gmView(sess)
	if !ok {
		return nil, fmt.Errorf("tradeskills are not served here")
	}
	rows, err := h.db.CharCraftSkills(v.CharServerID)
	if err != nil {
		return nil, err
	}
	var out []string
	for i := range rows {
		s := craftSkillOf(rows, i)
		if !s.live {
			out = append(out, fmt.Sprintf("%d %s: not live", i, craftSkillName(i)))
			continue
		}
		out = append(out, fmt.Sprintf("%d %s: level %d, rank %d, %d of %d XP", i, craftSkillName(i), s.level, s.rank,
			s.xp, craftXPPerRank*s.level))
	}
	return out, nil
}

// Learn is #craftlearn: a recipe (by id), or every recipe (of a skill), put
// in the GM's own book.
func (h *CraftHandler) Learn(sess *session.Session, args []string) (string, error) {
	const usage = "#craftlearn <recipe id | all [skill 0-5]>"
	if len(args) == 0 {
		return "", fmt.Errorf("usage: %s", usage)
	}
	v, ok := h.gmView(sess)
	if !ok {
		return "", fmt.Errorf("tradeskills are not served here")
	}
	recipes, err := h.db.CraftRecipes()
	if err != nil {
		return "", err
	}
	var ids []int
	switch {
	case strings.EqualFold(args[0], "all"):
		skill := -1
		if len(args) > 1 {
			if skill, err = strconv.Atoi(args[1]); err != nil || skill < 0 || skill >= database.CraftSkills {
				return "", fmt.Errorf("usage: %s", usage)
			}
		}
		for _, r := range recipes.All() {
			if skill < 0 || r.Skill == skill {
				ids = append(ids, r.ID)
			}
		}
	default:
		id, err := strconv.Atoi(args[0])
		if err != nil {
			return "", fmt.Errorf("usage: %s", usage)
		}
		if _, ok := recipes.Recipe(id); !ok {
			return "", fmt.Errorf("no recipe %d", id)
		}
		ids = []int{id}
	}
	if len(ids) == 0 {
		return "no recipes to learn", nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	added, err := h.db.AddCharRecipes(v.CharServerID, ids...)
	if err != nil {
		return "", err
	}
	log.Printf("Craft: GM %s learned %d recipes", v.Name, len(added))
	return fmt.Sprintf("%d recipe(s) put in your book, %d known already (open the Recipe Book again to see them)",
		len(added), len(ids)-len(added)), nil
}

// gmView is the GM's own view, false with no world or database.
func (h *CraftHandler) gmView(sess *session.Session) (entity.CraftView, bool) {
	if h == nil || h.view == nil || h.db == nil {
		return entity.CraftView{}, false
	}
	return h.view(sess)
}

func craftSkillList() string {
	parts := make([]string, len(craftSkillNames))
	for i, n := range craftSkillNames {
		parts[i] = fmt.Sprintf("%d %s", i, n)
	}
	return strings.Join(parts, ", ")
}
