package handlers

import (
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// petgroomer.go — the Pet Groomers (the owner, 2026-11-16: Zerkziee's
// drachnid pet should "have 2 different color areas that can be changed like
// armor dye(will be a template for pet color dye). then add Pet Groomer near
// Equipment Dyer also"). Talked to with a pet out, a groomer asks what to dye
// of it (its clothing, its body, both, or every dye off, as the client tells
// clothing from body: a body that wears none is dyed whole; a petdyeareas
// row names a body's areas in place of the rule),
// then a family of the dyers' colours and a colour of it (database.DyeFamilies,
// the Equipment Dyer's own, dyer.go), and dyes it at once, for nothing, as the
// dyer does: table petdyes (database.SetPetDye, a row edit with its gm_audit
// line, so the GM panel lists and reverts it), kept by the pet's body, so each
// look of a pet keeps its own dyes. The pet is drawn dyed by a client that asks
// for pet dyes (vanilla; entity/petdyes.go); a PS2 never sees them, and the
// groomer says so. Any NPC named "Pet Groomer" is one
// (tools/pet_groomers_20261116.py stands one beside each Equipment Dyer). Its
// questions go under box mark 0x6D000000.

// groomerBoxMark marks a groomer's question's id.
const groomerBoxMark = 0x6D000000

// IsGroomerBox reports a box a groomer asked.
func IsGroomerBox(id uint32) bool { return id&0xFF000000 == groomerBoxMark }

// PetGroomerName is the groomers' name.
const PetGroomerName = "Pet Groomer"

// IsPetGroomer is whether an NPC of the name is a groomer.
func IsPetGroomer(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), PetGroomerName)
}

// The groomer's questions, in the order they come.
const (
	groomWhat   = iota // groomChoices
	groomFamily        // database.DyeFamilies
	groomShade         // dyerShades(family)
)

// groomChoice is an answer to what to dye: the areas it dyes (0 area 1, 1
// area 2), none for every dye off.
type groomChoice struct {
	label string
	areas []int
}

// groomChoices is what a groomer asks of a pet: each area, both and every dye
// off for a body of two areas; its whole body and its dye off for one of one.
func groomChoices(pet entity.PetLook) []groomChoice {
	if pet.Rule {
		// the client's rule (the owner, 2026-11-16: "the pets need their
		// clothes as one dye and the rest another like we two tone
		// equipment"): area 1 its clothing, area 2 the rest; a body that
		// wears none is all area 2
		if !pet.Clothed {
			return []groomChoice{{label: "Its whole body", areas: []int{1}}, {label: "Remove its dye"}}
		}
		return []groomChoice{{label: "Clothing", areas: []int{0}}, {label: "Body", areas: []int{1}},
			{label: "Both", areas: []int{0, 1}}, {label: "Remove all dyes"}}
	}
	if !pet.Area.TwoAreas() {
		return []groomChoice{{label: "Its whole body", areas: []int{0}}, {label: "Remove its dye"}}
	}
	a1, a2 := strings.TrimSpace(pet.Area.Area1), strings.TrimSpace(pet.Area.Area2)
	if a1 == "" {
		a1 = "Body"
	}
	return []groomChoice{{label: a1, areas: []int{0}}, {label: a2, areas: []int{1}},
		{label: "Both areas", areas: []int{0, 1}}, {label: "Remove all dyes"}}
}

// petDyeStore is what a groomer writes through (database.SQLiteDB).
type petDyeStore interface {
	PetDyesOf(serverID uint32) (map[uint32][database.PetDyeAreas]uint32, error)
	SetPetDye(serverID, look uint32, area int, color uint32, who database.GMEdit, detail string) (uint32, int64, error)
}

// Groomer keeps each character's question in hand.
type Groomer struct {
	gw    *entity.World
	dyes  petDyeStore
	mu    sync.Mutex
	seq   uint32
	asked map[uint32]groomAsk
}

// groomAsk is a question asked: its box, which question (groomWhat...), the
// pet's body it is for (a pet summoned anew in another meanwhile keeps
// these), the choices it asked, the areas chosen and their words, the
// family, and who asked it.
type groomAsk struct {
	id      uint32
	stage   int
	look    uint32
	pet     string
	choices []groomChoice
	areas   []int
	what    string
	family  int
	speaker string
}

// NewGroomer makes the groomers over a pet dye store.
func NewGroomer(gw *entity.World, dyes petDyeStore) *Groomer {
	return &Groomer{gw: gw, dyes: dyes, asked: map[uint32]groomAsk{}}
}

func (g *Groomer) say(sess *session.Session, speaker, line string) {
	for _, part := range splitSay(speaker+": "+line, questBoxChars) {
		SendDialogueBox(sess, 0, part)
	}
}

func (g *Groomer) ask(sess *session.Session, p *entity.Player, a groomAsk, question string, answers []string) {
	g.mu.Lock()
	g.seq++
	a.id = groomerBoxMark | (g.seq & 0xFFFFFF)
	g.asked[p.CharServerID] = a
	g.mu.Unlock()
	SendQuestPopupMulti(sess, a.id, clipRunes(question, questAskChars), answers)
}

// Talk is a groomer talked to: what to dye of the player's pet. It reports
// whether the NPC was one.
func (g *Groomer) Talk(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	if g == nil || p == nil || npc == nil || npc.Dead || !IsPetGroomer(npc.Name) {
		return false
	}
	g.Offer(sess, p, strings.TrimSpace(npc.Name))
	return true
}

// Offer is a groomer's questions under speaker's name.
func (g *Groomer) Offer(sess *session.Session, p *entity.Player, speaker string) {
	if g == nil || p == nil {
		return
	}
	forgetDialogs(p)
	if g.dyes == nil || g.gw == nil {
		g.say(sess, speaker, "My brushes are packed away today.")
		return
	}
	pet, ok := g.gw.PetLookOf(sess)
	if !ok {
		g.say(sess, speaker, "I groom and dye pets, free of charge. Call yours to your side and I will color its coat. Only those who see the live game's colors see a dye.")
		return
	}
	line := fmt.Sprintf("What a fine %s! I dye pets any color you like, free of charge, and it keeps its colors for this form. Only those who see the live game's colors see a dye.", pet.Name)
	if pet.Rule {
		line = fmt.Sprintf("What a fine %s! I dye a pet's clothing one color and its body another, as the dyers two-tone armor, free of charge, and it keeps its colors for this form. Only those who see the live game's colors see a dye.", pet.Name)
		if !pet.Clothed {
			line = fmt.Sprintf("What a fine %s! It wears no clothing, so I dye its whole body, any color you like, free of charge, and it keeps its colors for this form. Only those who see the live game's colors see a dye.", pet.Name)
		}
	}
	g.say(sess, speaker, line)
	choices := groomChoices(pet)
	labels := make([]string, len(choices))
	for i, c := range choices {
		labels[i] = c.label
	}
	g.ask(sess, p, groomAsk{speaker: speaker, stage: groomWhat, look: pet.Look, pet: pet.Name, choices: choices},
		fmt.Sprintf("What shall I dye on your %s?", strings.ToLower(pet.Name)), labels)
}

// Answer is HandleDialogueOption's question for a groomer's box. It reports
// whether the box was a groomer's (an old one does nothing).
func (g *Groomer) Answer(sess *session.Session, p *entity.Player, box uint32, option byte) bool {
	if !IsGroomerBox(box) {
		return false
	}
	if g == nil || p == nil || option == 255 {
		return true // a close: the PS2 sends one before the real pick when two boxes were open
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
	n := int(option)
	switch a.stage {
	case groomWhat:
		if n >= len(a.choices) {
			return true
		}
		c := a.choices[n]
		if c.areas == nil {
			g.dye(sess, p, a, []int{0, 1}, 0, "Every dye is off.")
			return true
		}
		what := strings.ToLower(c.label)
		families := make([]string, len(database.DyeFamilies))
		for i, f := range database.DyeFamilies {
			families[i] = f.Label
		}
		g.ask(sess, p, groomAsk{speaker: a.speaker, stage: groomFamily, look: a.look, pet: a.pet, areas: c.areas, what: what},
			fmt.Sprintf("Which colors for %s?", what), families)
	case groomFamily:
		if n < len(database.DyeFamilies) {
			shades := dyerShades(n)
			answers := make([]string, len(shades))
			for i, c := range shades {
				answers[i] = c.Label
			}
			g.ask(sess, p, groomAsk{speaker: a.speaker, stage: groomShade, look: a.look, pet: a.pet, areas: a.areas, what: a.what, family: n},
				fmt.Sprintf("Which color for %s?", a.what), answers)
		}
	case groomShade:
		if shades := dyerShades(a.family); n < len(shades) {
			c := shades[n]
			done := fmt.Sprintf("There: %s.", strings.ToLower(c.Label))
			if c.RGBA == 0 {
				done = "The dye is off."
			}
			g.dye(sess, p, a, a.areas, c.RGBA, done)
		}
	}
	return true
}

// dye writes the areas' colour on the asked body and puts the character's pet
// dyes on it where it stands.
func (g *Groomer) dye(sess *session.Session, p *entity.Player, a groomAsk, areas []int, rgba uint32, done string) {
	who := database.GMEdit{AccountID: sess.AccountID(), Via: "game"}
	if addr := sess.RemoteAddr(); addr != nil {
		who.IP = addr.IP.String()
	}
	for _, area := range areas {
		if _, _, err := g.dyes.SetPetDye(p.CharServerID, a.look, area, rgba, who, "the Pet Groomer"); err != nil {
			log.Printf("Groomer: %s's pet 0x%08X area %d: %v", p.Name, a.look, area+1, err)
			g.say(sess, a.speaker, "Something went wrong with my dyes. Try again.")
			return
		}
	}
	g.reload(p.CharServerID)
	g.say(sess, a.speaker, done)
	log.Printf("Groomer: %s's %s (0x%08X) areas %v: %s", p.Name, a.pet, a.look, areas, database.FormatDye(rgba))
}

// reload puts a character's pet dyes, as table petdyes holds them, on it.
func (g *Groomer) reload(charID uint32) {
	if g.gw == nil || g.dyes == nil {
		return
	}
	got, err := g.dyes.PetDyesOf(charID)
	if err != nil {
		log.Printf("Groomer: character %d's pet dyes: %v", charID, err)
		return
	}
	g.gw.SetPetDyes(charID, got)
}

// loadPetDyes puts a character's pet dyes (table petdyes) on its player at a
// login.
func (h *CharacterHandler) loadPetDyes(serverID uint32) {
	if h.gameWorld == nil || h.store == nil || h.store.DB() == nil {
		return
	}
	got, err := h.store.DB().PetDyesOf(serverID)
	if err != nil {
		log.Printf("SelectedChar: character %d's pet dyes: %v", serverID, err)
		return
	}
	h.gameWorld.SetPetDyes(serverID, got)
}

// petDyesLayout is the layout of OpcodePetDyes the offer names: 2, a rule
// byte per pet (entity.PetDyesLayoutMost).
const petDyesLayout = entity.PetDyesLayoutMost

// buildPetDyesOffer is OpcodePetDyesOffer from the server: a u8, the layout.
func buildPetDyesOffer() []byte {
	return build(protocol.OpcodePetDyesOffer, 3, func(w *buffer.Writer) {
		w.WriteByte(petDyesLayout)
	})
}

// SendPetDyesOffer offers the session's client the pets' dyes.
func SendPetDyesOffer(sess *session.Session) {
	queueReliable(sess, protocol.OpcodePetDyesOffer, buildPetDyesOffer())
}

// PetDyesHandler takes a client's OpcodePetDyesAnswer.
type PetDyesHandler struct{}

// RegisterHandlers registers the pet dyes' answer.
func (h *PetDyesHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodePetDyesAnswer, h.HandlePetDyes)
}

// HandlePetDyes makes the client's answer its session's: a u8, nonzero to be
// told the dyed pets in its view, 0 to stop. On, its view's go at the next
// pass.
func (h *PetDyesHandler) HandlePetDyes(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 1 {
		log.Printf("PetDyes: short (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	layout := min(data[0], petDyesLayout)
	on := layout != 0
	sess.SetPetDyesLayout(layout)
	log.Printf("PetDyes: %s %s (layout %d)", sess.RemoteAddr(), map[bool]string{true: "draws them", false: "stopped"}[on], layout)
	if on {
		entity.GetOrCreateVisibility(sess).ResendPetDyes()
	}
	return nil
}
