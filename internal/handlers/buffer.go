package handlers

import (
	"log"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// buffer.go — Buffer's talk (the GM Jibrish's petition 1390, Heirloom: "Add a vendor close but north of the
// announcement board in FP named "Buffer". When spoken to, buffer gives you avatar, order, pure heart, the cleric +
// all resists buffs and a stackable GM AC that is +500ac buff. They last for 5 hours. It casts it after 1 second.").
// Any NPC named Buffer is him (talked to before quests, as Dabrish and the Teleporter); a data step stands him on the
// Heirloom Test Server 5 north and 1 west of the Freeport Announcements board and writes the GM Armor's record. A talk
// is no question: he begins his cast at the speaker at once, and it lands a second later (entity/buffer.go: five
// hours of play each, the speaker alone, a second talk renewing them). His spells are their records, read at each
// talk (a GM's edit of one counts at once), each by its id and checked by its name (a record renamed is not
// given, and is logged).

// BufferSpell is one of his spells: its record and its name.
type BufferSpell struct {
	ID   int
	Name string
}

// BufferGMArmorID is the GM Armor's record, his own spell (the data step writes it; no class has it): "Target,
// Increase AC by 500" on a workbook stack type of its own (its id), so no other buff is on its line and it stacks
// with every one (entity/buffline.go): no AC buff takes it off, it takes none off.
const BufferGMArmorID = 905120

// BufferSpells are what he casts, in order (each the strongest of its name on 2026-11-09's data).
var BufferSpells = []BufferSpell{
	{700675, "Avatar"},             // the shaman's 51 (the only record of the name): +1500 HP, its max and +150 STR STA AGI DEX
	{700805, "Order"},              // the cleric's 60 single-target +650 AC and max HP (Group Order its group form)
	{901847, "Pure Heart"},         // the 60 mastery's +1200 max HP and power (its PB AE on him alone here)
	{701184, "Mega's Clutch Ward"}, // the top of the cleric's all-resist line (Elemental Ward ... Bulwark): +85 to all six
	{BufferGMArmorID, "GM Armor"},  // his own: +500 AC, stacking with everything
}

// Buffer is his talk: the World, what reads his spells (the records as the World lands them, and the names of
// those it could not read) and the clock.
type Buffer struct {
	gw     *entity.World
	spells func() ([]entity.CastSpell, []string)
	now    func() time.Time
}

// NewBuffer is his talk over the World and the spell records; nil without either (an NPC named Buffer then has
// nothing to say).
func NewBuffer(gw *entity.World, store *database.SpellbookStore) *Buffer {
	if gw == nil || store == nil {
		return nil
	}
	return &Buffer{gw: gw, spells: func() ([]entity.CastSpell, []string) { return bufferSpells(store) }, now: time.Now}
}

// bufferSpells reads BufferSpells' records, each as the World lands it, and the names of those missing or renamed.
func bufferSpells(store *database.SpellbookStore) ([]entity.CastSpell, []string) {
	var out []entity.CastSpell
	var missing []string
	for _, b := range BufferSpells {
		sp, err := store.SpellPattern(b.ID)
		if err != nil || sp == nil || !strings.EqualFold(strings.TrimSpace(sp.SpellName), b.Name) {
			got := ""
			if sp != nil {
				got = sp.SpellName
			}
			log.Printf("Buffer: spell %d is %q, not %q (%v): not given", b.ID, got, b.Name, err)
			missing = append(missing, b.Name)
			continue
		}
		row, found, err := store.SpellEffectRow(b.ID)
		if err != nil {
			log.Printf("Buffer: %s's effects (%d): %v: not given", b.Name, b.ID, err)
			missing = append(missing, b.Name)
			continue
		}
		if !found {
			row = database.SpellEffectRow{} // read by its name, as a cast reads a spell with no row
		}
		out = append(out, entity.SpellOfRow(sp, 0, row.Line, row.Effects, row.FX))
	}
	return out, missing
}

// Talk is a talk to npc: his cast when npc is Buffer (said in a line; a refusal in red), else false.
func (b *Buffer) Talk(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	if b == nil || npc == nil || !entity.IsBuffer(npc.Name) {
		return false
	}
	spells, missing := b.spells()
	if len(spells) == 0 {
		log.Printf("Buffer: %s talks to him, and he has none of his spells (%s)", p.Name, strings.Join(missing, ", "))
		SendClientMessage(sess, entity.BufferLineNone)
		return true
	}
	switch line, begun := b.gw.StartBuffer(sess, npc.ServerID, spells, b.now()); {
	case begun:
		SendClientMessage(sess, line)
	case line != "":
		SendColoredChat(sess, line, 255, 63, 63)
	}
	return true
}
