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

// A cast's reagents (entity's SpellDef.Reagent: a line's "Reagent Unit of
// Blood x2"): the vampires' abilities cost Units of Blood (EQOA-Data's
// vampire guide: "all vampire abilities require them as reagents";
// entity/vampire.go). The cast is refused, in the red of a cast's other
// refusals, while the caster carries fewer (its bank's do not count), and
// they are taken (the database, the session's list, 0x003C) only when the
// World begins the cast, so a cast refused for any other reason takes none.
// The refusal's words are ours: Sandstorm has one, never recorded. A
// potion's line says the same of what using it costs (potion.go: Envar's
// gems, each needing the cracked gem the last one left), refused and taken
// the same way, with the potion.

// reagentRows is the caster's carried stacks of the cast's reagent, oldest
// first, or false with the cast refused (said to the caster).
func (h *SpellbookHandler) reagentRows(sess *session.Session, p *entity.Player, spell *entity.CastSpell) ([]database.CharInventoryEntry, bool) {
	return reagentHeld(h.inv, sess, p, "Cast", spell.Name, &spell.Def)
}

// reagentHeld is the player's carried stacks of the reagent d names, oldest
// first, or false with what needs it (what: a spell's or an item's name)
// refused, said to the player in red; tag begins the log's line ("Cast",
// "Potion").
func reagentHeld(inv *database.InventoryStore, sess *session.Session, p *entity.Player, tag, what string, d *entity.SpellDef) ([]database.CharInventoryEntry, bool) {
	verb := "uses"
	if tag == "Cast" {
		verb = "casts"
	}
	refuse := func(text string) {
		log.Printf("%s: %s %s %q: %s", tag, p.Name, verb, what, text)
		SendColoredChat(sess, text, scribeRefusalRGB[0], scribeRefusalRGB[1], scribeRefusalRGB[2])
	}
	if inv == nil {
		refuse(fmt.Sprintf("%s needs %s, and none can be carried here.", what, reagentWords(d.Reagent, d.ReagentCount)))
		return nil, false
	}
	pat, ok, err := inv.ItemPatternByName(d.Reagent)
	if err != nil {
		log.Printf("%s: %q's reagent %q: %v", tag, what, d.Reagent, err)
		return nil, false
	}
	if !ok {
		refuse(fmt.Sprintf("%s needs %s, which this server does not have.", what, reagentWords(d.Reagent, d.ReagentCount)))
		return nil, false
	}
	items, err := inv.GetCharacterItems(p.CharServerID)
	if err != nil {
		log.Printf("%s: %s's inventory: %v", tag, p.Name, err)
		return nil, false
	}
	var held []database.CharInventoryEntry
	n := 0
	for _, it := range items {
		if it.Pattern.PatternID == pat.PatternID && it.Location == 0 && it.StackLeft > 0 {
			held = append(held, it)
			n += it.StackLeft
		}
	}
	if n < d.ReagentCount {
		refuse(fmt.Sprintf("%s needs %s; you have %d.", what, reagentWords(pat.ItemName, d.ReagentCount), n))
		return nil, false
	}
	return held, true
}

// takeReagent takes n of the stacks held, in the database, the session's
// list and the client's (0x003C), the first stacks first.
func (h *SpellbookHandler) takeReagent(sess *session.Session, p *entity.Player, held []database.CharInventoryEntry, n int) {
	takeStacks(h.inv, sess, p, "Cast", held, n)
}

// takeStacks takes n of the stacks held, in the database, the session's list
// and the client's (0x003C), the first stacks first; tag begins the log's
// line.
func takeStacks(inv *database.InventoryStore, sess *session.Session, p *entity.Player, tag string, held []database.CharInventoryEntry, n int) {
	l := sessionInvList(sess)
	for _, r := range held {
		if n <= 0 {
			return
		}
		k := min(n, r.StackLeft)
		var err error
		if k >= r.StackLeft {
			err = inv.DeleteCharItem(p.CharServerID, r.ItemID)
		} else {
			err = inv.SetItemStack(p.CharServerID, r.ItemID, r.StackLeft-k)
		}
		if err != nil {
			log.Printf("%s: taking %d %q from %s: %v", tag, k, r.Pattern.ItemName, p.Name, err)
			return
		}
		n -= k
		if l != nil {
			if idx := l.indexOfItem(r.ItemID); idx >= 0 && l.took(idx, k) && sess != nil {
				queueReliable(sess, protocol.OpcodeRemoveInvItem, removeInvItemPayload(idx, 0, k))
			}
		}
		log.Printf("%s: %s spends %d %q", tag, p.Name, k, r.Pattern.ItemName)
	}
}

// reagentWords is n of an item in words: "1 Unit of Blood", "2 Units of
// Blood" (the first word made plural).
func reagentWords(name string, n int) string {
	if n == 1 {
		return "1 " + name
	}
	first, rest, _ := strings.Cut(name, " ")
	if !strings.HasSuffix(strings.ToLower(first), "s") {
		first += "s"
	}
	if rest != "" {
		return fmt.Sprintf("%d %s %s", n, first, rest)
	}
	return fmt.Sprintf("%d %s", n, first)
}
