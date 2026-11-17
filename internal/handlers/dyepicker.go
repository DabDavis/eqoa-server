package handlers

import (
	"encoding/binary"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// dyepicker.go — the dyers' colour picker (the owner, 2026-11-17: "lets
// upgrade the dyer NPCs to show a RGB color picker, add to client if needed.
// then we have any rgb combo"). Once the player is in the world the server
// offers the picker (protocol.OpcodeDyePicker kind 0); a client with a picker
// window (vanilla) answers its version (OpcodeDyePickerAnswer kind 0), and
// from then on the Equipment Dyer's (dyer.go), the Pet Groomer's
// (petgroomer.go) and the Weapon Enchanter's (enchanter.go) colour questions
// carry one more answer after the palette's, "Pick any colour...". Chosen,
// the server opens the client's picker (kind 1): the slots it dyes, the
// colour now, the dyer's palette, and for a pet the dyes as OpcodePetDyes has
// them, so the client previews the colour on the character or the pet while
// it is picked. The client answers the colour (kind 1) or that it was
// cancelled (kind 2). A colour is applied as the palette's choice would be
// (dyed, written with its gm_audit line, put on the character), but only for
// the picker asked (its token, at most dyePickFor after it opened), with the
// player still in the world it was asked in and near whoever asked it
// (dyePickReach of the NPC, or of where the player stood for its genie),
// and the alpha FF where the slots take none (a pet's dye's alpha is how
// much of it shows; an armour piece of alpha below FF would be see-through).
// The price is the palette's: the dyers and groomers are free, an aura costs
// the enchanter's tokens. A PS2 never answers the offer, so it is never
// offered the answer and its boxes are the palette's, as they were.

// The picker's message kinds (protocol.OpcodeDyePicker, OpcodeDyePickerAnswer).
const (
	DyePickerOffer  = 0 // server: the offer; client: its answer
	DyePickerOpen   = 1 // server: a picker opened
	DyePickerClose  = 2 // server: a picker closed
	DyePickerPicked = 1 // client: the colour chosen
	DyePickerCancel = 2 // client: the picker cancelled
)

// What a picker dyes (the open's target byte).
const (
	DyePickChar = 0 // the character's dye slots (chardyes)
	DyePickPet  = 1 // its pet's areas (petdyes)
	DyePickAura = 2 // a held weapon's aura (instanceaura)
)

// DyePickLabel is the answer that opens the picker.
const DyePickLabel = "Pick any colour..."

// dyePickReach is how far from whoever asked a colour may still be chosen:
// three times the reach to talk, so a player may step back to see itself.
const dyePickReach = entity.InteractRange * 3

// dyePickFor is how long a picker's colour is taken after it opened.
const dyePickFor = 10 * time.Minute

// The auras' places in the live channel's auras (vanilla's
// net::entity::MAIN_HAND_AURA ... OFF_HAND_AURA2): what an aura's picker
// previews.
const (
	auraPlaceMainHand  = 9
	auraPlaceOffHand   = 10
	auraPlaceMainHand2 = 11
	auraPlaceOffHand2  = 12
)

// pickerOffered is whether sess's client is offered "Pick any colour..." in
// a box of room answers holding n others: it answered the offer, and the box
// has room for one more.
func pickerOffered(sess *session.Session, n int) bool {
	return sess.DyePicker() >= 1 && n < dialogRoom(sess)
}

// dyePalette is the dyers' colours, every family's, in order: the picker's
// quick swatches.
func dyePalette() []uint32 {
	var out []uint32
	for _, f := range database.DyeFamilies {
		for _, c := range f.Colours {
			out = append(out, c.RGBA)
		}
	}
	return out
}

// dyePetPreview is a pet as OpcodePetDyes carries it: what the client needs to
// preview its dyes.
type dyePetPreview struct {
	id, look uint32
	dye      [2]uint32
	mask     uint64
	rule     bool
}

// dyeAuraPreview is the aura an aura's picker previews the colour on.
type dyeAuraPreview struct {
	kind   uint8
	source uint32
	nodes  uint8
}

// dyePickerOpen is what a picker is opened with.
type dyePickerOpen struct {
	target  uint8
	alpha   bool
	current uint32
	title   string
	slots   []uint8
	pet     dyePetPreview  // target DyePickPet
	aura    dyeAuraPreview // target DyePickAura
	palette []uint32
}

// encodeDyePickerOpen is OpcodeDyePicker kind 1's message (opcode first).
func encodeDyePickerOpen(token uint32, o dyePickerOpen) []byte {
	le := binary.LittleEndian
	out := le.AppendUint16(nil, uint16(protocol.OpcodeDyePicker))
	out = append(out, DyePickerOpen)
	out = le.AppendUint32(out, token)
	var flags byte
	if o.alpha {
		flags |= 1
	}
	out = append(out, o.target, flags)
	out = le.AppendUint32(out, o.current)
	title := clipRunes(o.title, 60)
	for len(title) > 255 {
		title = clipRunes(title, len([]rune(title))-1)
	}
	out = append(out, byte(len(title)))
	out = append(out, title...)
	slots := o.slots
	if len(slots) > 255 {
		slots = slots[:255]
	}
	out = append(out, byte(len(slots)))
	out = append(out, slots...)
	switch o.target {
	case DyePickPet:
		out = le.AppendUint32(out, o.pet.id)
		out = le.AppendUint32(out, o.pet.look)
		out = le.AppendUint32(out, o.pet.dye[0])
		out = le.AppendUint32(out, o.pet.dye[1])
		out = le.AppendUint64(out, o.pet.mask)
		out = append(out, map[bool]byte{false: entity.PetDyesByMask, true: entity.PetDyesByRule}[o.pet.rule])
	case DyePickAura:
		out = append(out, o.aura.kind)
		out = le.AppendUint32(out, o.aura.source)
		out = append(out, o.aura.nodes)
	}
	pal := o.palette
	if len(pal) > 255 {
		pal = pal[:255]
	}
	out = append(out, byte(len(pal)))
	for _, c := range pal {
		out = le.AppendUint32(out, c)
	}
	return out
}

// SendDyePickerOffer offers the session's client the picker.
func SendDyePickerOffer(sess *session.Session) {
	out := binary.LittleEndian.AppendUint16(nil, uint16(protocol.OpcodeDyePicker))
	queueReliable(sess, protocol.OpcodeDyePicker, append(out, DyePickerOffer, session.DyePickerVersion))
}

// dyePick is a picker open: its token, whoever asked (an NPC, or none: the
// player's own genie, then where the player stood), the world it was asked
// in and when, whether its alpha is the player's, and the dye it applies.
type dyePick struct {
	token   uint32
	npc     *entity.NPC
	x, y, z float32
	world   uint8
	at      time.Time
	alpha   bool
	speaker string
	what    string
	apply   func(sess *session.Session, p *entity.Player, rgba uint32)
}

// dyePicks is the pickers open, one a character (a new one closes the last).
type dyePicks struct {
	mu   sync.Mutex
	seq  uint32
	open map[uint32]*dyePick
	now  func() time.Time
}

var picks = &dyePicks{open: map[uint32]*dyePick{}, now: time.Now}

// openDyePicker opens sess's client's picker for the player: o, applied by
// apply when a colour comes, asked by npc (nil: the genie, where the player
// stands) under speaker's name; what names what it dyes in the log.
func openDyePicker(sess *session.Session, p *entity.Player, npc *entity.NPC, speaker, what string, o dyePickerOpen,
	apply func(sess *session.Session, p *entity.Player, rgba uint32)) {
	if p == nil || sess == nil {
		return
	}
	if o.palette == nil {
		o.palette = dyePalette()
	}
	picks.mu.Lock()
	picks.seq++
	token := 0x7E950000 | (picks.seq & 0xFFFF)
	old := picks.open[p.CharServerID]
	picks.open[p.CharServerID] = &dyePick{token: token, npc: npc, x: p.X, y: p.Y, z: p.Z, world: p.World, at: picks.now(),
		alpha: o.alpha, speaker: speaker, what: what, apply: apply}
	picks.mu.Unlock()
	if old != nil {
		sendDyePickerClose(sess, old.token)
	}
	queueReliable(sess, protocol.OpcodeDyePicker, encodeDyePickerOpen(token, o))
	log.Printf("DyePicker: %s's picker %08X for %s (target %d, now %s)", p.Name, token, what, o.target, database.FormatDye(o.current))
}

func sendDyePickerClose(sess *session.Session, token uint32) {
	out := binary.LittleEndian.AppendUint16(nil, uint16(protocol.OpcodeDyePicker))
	out = append(out, DyePickerClose)
	out = binary.LittleEndian.AppendUint32(out, token)
	queueReliable(sess, protocol.OpcodeDyePicker, out)
}

// closeDyePicker closes the player's picker, if one is open (a new question
// asked of it).
func closeDyePicker(sess *session.Session, p *entity.Player) {
	if p == nil {
		return
	}
	picks.mu.Lock()
	old := picks.open[p.CharServerID]
	delete(picks.open, p.CharServerID)
	picks.mu.Unlock()
	if old != nil && sess != nil {
		sendDyePickerClose(sess, old.token)
	}
}

// take is the player's picker of token, taken out (answered once); nil when
// none of the token is open.
func (ps *dyePicks) take(charID, token uint32) *dyePick {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	k := ps.open[charID]
	if k == nil || k.token != token {
		return nil
	}
	delete(ps.open, charID)
	return k
}

// near is whether the player is still where a colour may be chosen: in the
// world it was asked in and within dyePickReach of whoever asked.
func (k *dyePick) near(p *entity.Player) bool {
	if p.World != k.world {
		return false
	}
	if k.npc != nil {
		return !k.npc.Dead && entity.InRange(p, k.npc, dyePickReach)
	}
	dx, dy, dz := p.X-k.x, p.Y-k.y, p.Z-k.z
	return dx*dx+dy*dy+dz*dz <= dyePickReach*dyePickReach
}

// pickColour is a colour as a picker takes it: alpha FF where its slots take
// none, else at least 1 (0 would hide the slot).
func pickColour(rgba uint32, alpha bool) uint32 {
	if !alpha || rgba&0xFF == 0 {
		return rgba | 0xFF
	}
	return rgba
}

// DyePickerHandler takes a client's OpcodeDyePickerAnswer.
type DyePickerHandler struct {
	gw *entity.World
}

// NewDyePickerHandler makes the picker's answers' handler over the world.
func NewDyePickerHandler(gw *entity.World) *DyePickerHandler { return &DyePickerHandler{gw: gw} }

// RegisterHandlers registers the picker's answers.
func (h *DyePickerHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeDyePickerAnswer, h.HandleDyePicker)
}

// HandleDyePicker takes the client's answer to the offer (its version), a
// colour chosen (applied, if the picker is open and the player near) or a
// picker cancelled.
func (h *DyePickerHandler) HandleDyePicker(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 2 {
		log.Printf("DyePicker: short (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	switch data[0] {
	case DyePickerOffer:
		sess.SetDyePicker(data[1])
		log.Printf("DyePicker: %s speaks version %d (asked %d)", sess.RemoteAddr(), sess.DyePicker(), data[1])
		return nil
	case DyePickerPicked, DyePickerCancel:
	default:
		return nil
	}
	if len(data) < 5 || (data[0] == DyePickerPicked && len(data) < 9) {
		log.Printf("DyePicker: short answer (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	if h.gw == nil {
		return nil
	}
	p := h.gw.SessionPlayer(sess)
	if p == nil {
		return nil
	}
	token := binary.LittleEndian.Uint32(data[1:5])
	k := picks.take(p.CharServerID, token)
	if k == nil {
		log.Printf("DyePicker: %s answered picker %08X, not open", p.Name, token)
		return nil
	}
	if data[0] == DyePickerCancel {
		log.Printf("DyePicker: %s cancelled the picker for %s", p.Name, k.what)
		return nil
	}
	rgba := pickColour(binary.LittleEndian.Uint32(data[5:9]), k.alpha)
	say := func(line string) {
		for _, part := range splitSay(k.speaker+": "+line, questBoxChars) {
			SendDialogueBox(sess, 0, part)
		}
	}
	switch {
	case picks.now().Sub(k.at) > dyePickFor:
		say("My dyes have dried. Ask me again.")
		log.Printf("DyePicker: %s's picker for %s came too late", p.Name, k.what)
		return nil
	case !k.near(p):
		say("Come back to me to be dyed.")
		log.Printf("DyePicker: %s's picker for %s came from too far", p.Name, k.what)
		return nil
	}
	log.Printf("DyePicker: %s picked %s for %s", p.Name, database.FormatDye(rgba), k.what)
	k.apply(sess, p, rgba)
	return nil
}

// colourWords is a picked colour as a dyer says it: its palette name if it
// is one, else its hex ("#3FA7D6").
func colourWords(rgba uint32) string {
	for _, f := range database.DyeFamilies {
		for _, c := range f.Colours {
			if c.RGBA == rgba {
				return strings.ToLower(c.Label)
			}
		}
	}
	if rgba&0xFF == 0xFF {
		return fmt.Sprintf("#%06X", rgba>>8)
	}
	return fmt.Sprintf("#%08X", rgba)
}
