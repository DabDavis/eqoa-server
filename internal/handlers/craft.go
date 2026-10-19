package handlers

import (
	"fmt"
	"log"
	"sort"
	"sync"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Tradeskills, as CLIENT's VIClientPart3834C drives them (decomp
// viclientpart3834c.cpp): requests on 0x1101 {u8 sub-op, u32 token, args},
// replies on 0x1102 {u8 sub-op, ...}. The token is the server's; every reply
// that carries one sets CLIENT's.
//
//	1 {u32 id}        remove a recipe        -> 2 {u32 id, u32 token}
//	2 {u32 id, u32 b} create from a recipe   -> 9 the skill report
//	3 {u32 from, to}  move a recipe          -> 8 {u32 token, from, to}
//	4 {zigzag n, ids} combine (experiment)   -> 12 {u8 2, u32 token}: "nothing"
//	5 {u32 x}         scribe a discovery     -> 1 {u32 token}
//	6 {u32 skill}     the skill's recipes    -> 15 {u32 token, u32 n, recipes}
//	7 {u32 id}        a recipe's detail      -> 16 {u32 token, detail}
//
// THE BOOK IS A STAND-IN: the server has no recipe data, so each session
// starts with three placeholder recipes per skill and component 0, enough to
// drive the windows. Creating raises the skill's level by one (the report
// says so) and makes nothing; a combine never makes anything.

const (
	opcodeCraftRequest protocol.GameOpcode = 0x1101
	opcodeCraftReply   protocol.GameOpcode = 0x1102
)

// craftRecipe is VIRec144_C017C0 on the wire.
type craftRecipe struct {
	x00, order, id                int
	level, skill, comp, difficult byte
	name                          string
}

// craftState is one session's book, skill slots and token.
type craftState struct {
	token  uint32
	book   []craftRecipe
	levels [6]int
}

// CraftHandler serves 0x1101.
type CraftHandler struct {
	mu     sync.Mutex
	states map[*session.Session]*craftState
}

func NewCraftHandler() *CraftHandler {
	return &CraftHandler{states: map[*session.Session]*craftState{}}
}

func (h *CraftHandler) RegisterHandlers(r *Registry) {
	r.Register(opcodeCraftRequest, h.HandleCraft)
}

// standInBook is the placeholder book (see above): skills 0, 1 and 3 (the
// start menu's Weapons, Armor and Jewelry), component 0.
func standInBook() []craftRecipe {
	var b []craftRecipe
	id := 1000
	for _, skill := range []byte{0, 1, 3} {
		for k := 0; k < 3; k++ {
			b = append(b, craftRecipe{order: len(b), id: id, level: byte(1 + 2*k), skill: skill, difficult: byte(k), name: fmt.Sprintf("Stand-in Recipe %d-%d", skill, k+1)})
			id++
		}
	}
	return b
}

func (h *CraftHandler) state(sess *session.Session) *craftState {
	s := h.states[sess]
	if s == nil {
		s = &craftState{token: 1, book: standInBook()}
		h.states[sess] = s
	}
	return s
}

func (h *CraftHandler) reply(sess *session.Session, fill func(w *buffer.Writer)) {
	queueReliable(sess, opcodeCraftReply, build(opcodeCraftReply, 4096, fill))
}

// craftRecipeBytes writes VIRec144_C017C0::Read's form: three zigzags, four
// bytes, the name.
func writeCraftRecipe(w *buffer.Writer, r craftRecipe) {
	w.Write7BitEncodedInt64(int64(r.x00))
	w.Write7BitEncodedInt64(int64(r.order))
	w.Write7BitEncodedInt64(int64(r.id))
	w.WriteByte(r.level)
	w.WriteByte(r.skill)
	w.WriteByte(r.comp)
	w.WriteByte(r.difficult)
	w.WriteStringUnicode(r.name)
}

func (s *craftState) find(id int) int {
	for i, r := range s.book {
		if r.id == id {
			return i
		}
	}
	return -1
}

// HandleCraft is 0x1101.
func (h *CraftHandler) HandleCraft(sess *session.Session, msg *protocol.Message) error {
	r := buffer.NewReader(msg.Data())
	sub, err := r.ReadByte()
	if err != nil {
		return nil
	}
	if _, err := r.ReadUint32(); err != nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.state(sess)
	s.token++
	switch sub {
	case 6: // the skill's recipes, by their order
		skill, _ := r.ReadUint32()
		var list []craftRecipe
		for _, rec := range s.book {
			if uint32(rec.skill) == skill {
				list = append(list, rec)
			}
		}
		sort.SliceStable(list, func(a, b int) bool { return list[a].order < list[b].order })
		h.reply(sess, func(w *buffer.Writer) {
			w.WriteByte(15)
			w.WriteUint32(s.token)
			w.WriteUint32(uint32(len(list)))
			for _, rec := range list {
				writeCraftRecipe(w, rec)
			}
		})
		log.Printf("Craft: %d recipes of skill %d", len(list), skill)
	case 7: // the detail (VICraftDetail::Read)
		id, _ := r.ReadUint32()
		i := s.find(int(id))
		if i < 0 {
			h.reply(sess, func(w *buffer.Writer) { w.WriteByte(7) })
			return nil
		}
		rec := s.book[i]
		h.reply(sess, func(w *buffer.Writer) {
			w.WriteByte(16)
			w.WriteUint32(s.token)
			w.Write7BitEncodedInt64(int64(rec.id))
			w.Write7BitEncodedInt64(-1)
			w.Write7BitEncodedInt64(-1)
			w.WriteByte(0)
			w.WriteByte(rec.skill)
			w.WriteByte(rec.comp)
			w.WriteByte(rec.difficult)
			w.WriteUint16(1)
			w.WriteByte(rec.level)
			w.WriteStringUnicode(rec.name)
			w.WriteStringUnicode("A stand-in recipe: the server has no recipe data.")
			w.Write7BitEncodedInt64(1) // stats
			w.Write7BitEncodedInt64(0)
			w.Write7BitEncodedInt64(int64(rec.level))
			w.Write7BitEncodedInt64(2) // ingredients
			w.WriteStringUnicode("Crude Iron")
			w.WriteUint16(2)
			w.WriteStringUnicode("Water Flask")
			w.WriteUint16(1)
			w.Write7BitEncodedInt64(1) // tools
			w.WriteStringUnicode("Smithing Hammer")
			w.WriteByte(1)
		})
	case 2: // create: the skill report (Recv09), nothing made
		id, _ := r.ReadUint32()
		i := s.find(int(id))
		if i < 0 {
			h.reply(sess, func(w *buffer.Writer) { w.WriteByte(3) })
			return nil
		}
		rec := s.book[i]
		slot := int(rec.skill)
		s.levels[slot]++
		h.reply(sess, func(w *buffer.Writer) {
			w.WriteByte(9)
			w.WriteUint32(s.token)
			w.WriteUint32(0)                          // ok
			w.WriteUint32(uint32(slot))               // the slot
			w.WriteUint32(uint32(s.levels[slot]))     // its level
			w.WriteUint32(uint32(s.levels[slot] / 5)) // its rank
			w.WriteUint32(0)
			w.WriteUint32(10)         // the XP
			w.WriteUint32(0)          // no reagent broke
			w.WriteUint32(0xFFFFFFFF) // no icon
			w.WriteStringUnicode(rec.name)
		})
	case 1: // remove
		id, _ := r.ReadUint32()
		if i := s.find(int(id)); i >= 0 {
			s.book = append(s.book[:i], s.book[i+1:]...)
		}
		h.reply(sess, func(w *buffer.Writer) {
			w.WriteByte(2)
			w.WriteUint32(id)
			w.WriteUint32(s.token)
		})
	case 3: // move `from` to `to`'s place (Recv08's rule)
		from, _ := r.ReadUint32()
		to, _ := r.ReadUint32()
		f, t := s.find(int(from)), s.find(int(to))
		if f >= 0 && t >= 0 {
			at, fo := s.book[t].order, s.book[f].order
			for i := range s.book {
				if s.book[i].order > fo {
					s.book[i].order--
				}
			}
			for i := range s.book {
				if i != f && s.book[i].order >= at {
					s.book[i].order++
				}
			}
			s.book[f].order = at
		}
		h.reply(sess, func(w *buffer.Writer) {
			w.WriteByte(8)
			w.WriteUint32(s.token)
			w.WriteUint32(from)
			w.WriteUint32(to)
		})
	case 4: // combine: "The items you chose do not make anything."
		n, _ := r.Read7BitEncodedInt64()
		log.Printf("Craft: combine of %d items", n)
		h.reply(sess, func(w *buffer.Writer) {
			w.WriteByte(12)
			w.WriteByte(2)
			w.WriteUint32(s.token)
		})
	case 5: // scribe
		h.reply(sess, func(w *buffer.Writer) {
			w.WriteByte(1)
			w.WriteUint32(s.token)
		})
	}
	return nil
}
