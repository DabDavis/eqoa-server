package handlers

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// /aura weapon: an aura on ONE copy of a held weapon (table instanceaura,
// database/instanceaura.go; the owner, 2026-11-16: "for weapons with aura they
// can be marked for an aura though they are the same weapon as other players,
// it will have a tag 'Aura: Virtuous' for example. so it doesnt just stick to
// all his weps"). The copy held in the hand named, by the target player (or
// yourself, or a mercenary): it shows on whoever holds that copy, and goes
// with it through a trade, the bank and the auction house.
//
//	/aura weapon mainhand Virtuous sword of virtue blue
//	/aura weapon offhand tag=Tainted epic source=5FD4AE7C
//	/aura weapon mainhand Shadowed kind 3 colour 000000FF
//	/aura weapon mainhand second particles source=1D875ECD along=100
//	/aura weapon weapons tag Freezing        the tag alone, both hands' copies (their auras kept)
//	/aura weapon mainhand notag               the tag gone, the auras kept
//	/aura weapon mainhand second off          the second aura gone
//	/aura weapon mainhand off                 the copy's row gone (tag and auras)
//	/aura weapon mainhand                     what the copy has
//
// The hand: mainhand (primary), offhand (secondary; a shield held there too)
// or weapons (both). The tag: tag=<Tag> or tag <Tag> anywhere (underscores
// are spaces), or the first word after the hand that is no aura word; at most
// database.InstanceTagMost letters, digits, spaces, ' or -. The aura: the
// easy words a hand takes (parseAuraEasy: particles, epic, energy, a named
// look, a colour, source=, along=), and "kind N", "source X", "colour X",
// "along N" as pairs. A hand of the character's own (charaura 9/10) still
// comes first (entity/pieceaura.go): the reply says so.

// auraWeaponHands are the hands /aura weapon takes.
var auraWeaponHands = map[string][]int{
	"mainhand": {entity.AuraMainHand}, "primary": {entity.AuraMainHand}, "main": {entity.AuraMainHand},
	"offhand": {entity.AuraOffHand}, "secondary": {entity.AuraOffHand}, "off-hand": {entity.AuraOffHand},
	"weapons": {entity.AuraMainHand, entity.AuraOffHand}, "both": {entity.AuraMainHand, entity.AuraOffHand},
	"bothhands": {entity.AuraMainHand, entity.AuraOffHand},
}

// auraWeaponSpec is /aura weapon's words read.
type auraWeaponSpec struct {
	Hands  []int
	Tag    string
	HasTag bool // a tag named (or notag: "")
	Off    bool // the whole row gone
	Show   bool // nothing but the hand: what the copy has
	Aura   *auraSpec
}

// isAuraWord reports whether a word is one of /aura's easy words (so not a tag).
func isAuraWord(w string) bool {
	w = strings.ToLower(w)
	if _, ok := auraEasyKinds[w]; ok {
		return true
	}
	if _, ok := auraColors[w]; ok {
		return true
	}
	if _, ok := auraWhere[w]; ok {
		return true
	}
	switch w {
	case "off", "none", "second", "kind", "source", "src", "colour", "color", "along", "nodes", "tag", "notag":
		return true
	}
	if strings.Contains(w, "=") {
		return true
	}
	h := strings.TrimPrefix(strings.TrimPrefix(w, "0x"), "#")
	if isHex(h, 6) || isHex(h, 8) {
		return true
	}
	for _, l := range auraLooks {
		if w == l.Words[0] || strings.ReplaceAll(w, "_", "") == strings.Join(l.Words, "") {
			return true
		}
	}
	return false
}

// parseAuraWeapon reads /aura weapon's words after "weapon"; false: args[0]
// names no hand (the older /aura words go on as they were).
func parseAuraWeapon(args []string) (sp auraWeaponSpec, ok bool, err error) {
	if len(args) == 0 {
		return sp, false, nil
	}
	hands, isHand := auraWeaponHands[strings.ToLower(args[0])]
	if !isHand {
		return sp, false, nil
	}
	sp.Hands = hands
	rest := args[1:]
	if len(rest) == 0 {
		sp.Show = true
		return sp, true, nil
	}
	var words []string
	sawAura := false
	for i := 0; i < len(rest); i++ {
		raw := strings.TrimSpace(rest[i])
		w := strings.ToLower(raw)
		next := func() (string, error) {
			if i+1 >= len(rest) {
				return "", fmt.Errorf("%s takes a value", raw)
			}
			i++
			return strings.TrimSpace(rest[i]), nil
		}
		switch {
		case w == "":
			continue
		case strings.HasPrefix(w, "tag="):
			sp.Tag, sp.HasTag = strings.ReplaceAll(raw[4:], "_", " "), true
		case w == "tag":
			v, e := next()
			if e != nil {
				return sp, true, e
			}
			sp.Tag, sp.HasTag = strings.ReplaceAll(v, "_", " "), true
		case w == "notag":
			sp.Tag, sp.HasTag = "", true
		case w == "kind":
			v, e := next()
			if e != nil {
				return sp, true, e
			}
			n, ce := strconv.Atoi(v)
			if ce != nil || n < 0 || n >= len(auraKindNames) {
				words = append(words, v) // a kind's name
			} else {
				words = append(words, auraKindNames[n])
			}
			sawAura = true
		case w == "source" || w == "src" || w == "along" || w == "nodes":
			v, e := next()
			if e != nil {
				return sp, true, e
			}
			if w == "nodes" {
				w = "along" // on a hand the byte is where along the weapon
			}
			if w == "src" {
				w = "source"
			}
			words = append(words, w+"="+v)
			sawAura = true
		case strings.HasPrefix(w, "nodes="):
			words = append(words, "along="+raw[6:])
			sawAura = true
		case w == "colour" || w == "color":
			v, e := next()
			if e != nil {
				return sp, true, e
			}
			words = append(words, v)
			sawAura = true
		case !sawAura && !sp.HasTag && len(words) == 0 && !isAuraWord(w):
			sp.Tag, sp.HasTag = strings.ReplaceAll(raw, "_", " "), true // the first word that is no aura word
		default:
			words = append(words, raw)
			sawAura = true
		}
	}
	if len(words) == 1 && strings.EqualFold(words[0], "off") {
		if sp.HasTag {
			return sp, true, fmt.Errorf("off takes the whole row (tag and auras): /aura weapon %s off", args[0])
		}
		sp.Off = true
		return sp, true, nil
	}
	if sp.HasTag && !database.ValidInstanceTag(sp.Tag) {
		return sp, true, fmt.Errorf("a tag is at most %d letters, digits, spaces, ' or - (%q)", database.InstanceTagMost, sp.Tag)
	}
	if len(words) == 0 {
		return sp, true, nil // the tag alone
	}
	place := "mainhand"
	if len(sp.Hands) == 2 {
		place = "weapons"
	} else if sp.Hands[0] == entity.AuraOffHand {
		place = "offhand"
	}
	a, e := parseAuraEasy(append(words, place))
	if e != nil {
		return sp, true, e
	}
	if !a.Off && !database.HandAuraKind(int64(a.A.Kind)) {
		return sp, true, fmt.Errorf("a weapon's aura is particles, epic or energy (or a named look): /aura weapon mainhand Virtuous sword of virtue")
	}
	sp.Aura = &a
	return sp, true, nil
}

// auraWeaponCommand is /aura weapon's (the file's doc): true when args named
// a hand and it answered.
func (h *ChatHandler) auraWeaponCommand(sess *session.Session, player *entity.Player, args []string) bool {
	sp, ok, err := parseAuraWeapon(args)
	if !ok {
		return false
	}
	if err != nil {
		h.gmReply(sess, "/aura weapon: "+err.Error()+". Usage: /aura weapon <mainhand|offhand|weapons> [Tag] <aura words> | tag <Tag> | notag | second <aura words> | second off | off")
		return true
	}
	db := h.camps.db
	if sp.Aura != nil && sp.Aura.SourceName != "" {
		var model int64
		if e := db.QueryRow(`SELECT model FROM itempattern WHERE itemname = ? COLLATE NOCASE AND model != 0 ORDER BY patternid LIMIT 1`,
			sp.Aura.SourceName).Scan(&model); e != nil {
			h.gmReply(sess, fmt.Sprintf("/aura weapon: no item named %q with a model for source=", sp.Aura.SourceName))
			return true
		}
		sp.Aura.A.Source = uint32(model)
	}
	// whom: the target player or mercenary, else yourself
	var items *[entity.AuraSlots]int
	var pieces *[entity.AuraSlots]int
	var own *[database.CharAuraSlots]database.ItemAura
	whose := "your"
	ownKey := database.CharAuraKey{CharID: player.CharServerID}
	items, pieces = &player.WornItems, &player.WornPieces
	if player.TargetObjectID != 0 && player.TargetObjectID != entity.NoAssist {
		ent, found := h.gameWorld.FindEntityByObjectID(sess, player.TargetObjectID)
		if !found {
			h.gmReply(sess, "/aura weapon: your target is gone")
			return true
		}
		switch t := ent.(type) {
		case *entity.Player:
			items, pieces, whose, ownKey = &t.WornItems, &t.WornPieces, t.Name+"'s", database.CharAuraKey{CharID: t.CharServerID}
		case *entity.NPC:
			k, isMerc := t.MercAuraKey()
			if !isMerc {
				h.gmReply(sess, "/aura weapon: "+t.Name+" holds no copy of its own: target a player, a mercenary or nothing (yourself)")
				return true
			}
			items, pieces, whose, ownKey = &t.WornItems, &t.WornPieces, t.Name+"'s", k
		default:
			h.gmReply(sess, "/aura weapon: target a player, a mercenary or nothing (yourself)")
			return true
		}
	}
	if m, _ := db.CharAuras(); m != nil {
		if o, has := m[ownKey]; has {
			own = &o
		}
	}
	for _, hand := range sp.Hands {
		name := auraPieceNames[hand]
		id := items[hand]
		if id == 0 {
			h.gmReply(sess, fmt.Sprintf("/aura weapon: %s %s holds no copy of its own (nothing, or a mercenary's lent heirloom)", whose, name))
			continue
		}
		have, _, err := db.InstanceAuraOf(id)
		if err != nil {
			h.gmReply(sess, "/aura weapon: "+err.Error())
			return true
		}
		what := fmt.Sprintf("%s %s's copy (item %d, record %d)", whose, name, id, pieces[hand])
		if sp.Show {
			h.gmReply(sess, what+": "+instanceAuraWords(have))
			continue
		}
		row := have
		row.ItemID = id
		switch {
		case sp.Off:
			row = database.InstanceAura{ItemID: id}
		case sp.Aura != nil && sp.Aura.Second:
			row.Second = sp.Aura.A
			if sp.Aura.Off {
				row.Second = database.ItemAura{}
			}
		case sp.Aura != nil:
			row.First = sp.Aura.A
			if sp.Aura.Off {
				row.First = database.ItemAura{}
			}
		}
		if sp.HasTag {
			row.Tag = sp.Tag
		}
		row.Source = "gm"
		detail := fmt.Sprintf("/aura weapon in the game: %s: %s", what, instanceAuraWords(row))
		if err := db.SetInstanceAuraAudited(row, gmEditOf(sess), detail); err != nil {
			h.gmReply(sess, "/aura weapon: not kept: "+err.Error())
			return true
		}
		note := ""
		if own != nil && own[hand].Shows() {
			note = fmt.Sprintf(" (the character's own %s aura, Character auras slot %d, still comes first: /aura off %s)", name, hand, strings.ReplaceAll(name, " ", ""))
		}
		h.gmReply(sess, fmt.Sprintf("%s: now %s, on whoever holds this copy alone%s. The tag shows on the item's next record (its next login or move).", what, instanceAuraWords(row), note))
		log.Printf("GM: %s /aura weapon %s: %s", player.Name, what, instanceAuraWords(row))
	}
	if !sp.Show {
		if _, err := h.gameWorld.LoadItemAuras(db); err != nil {
			h.gmReply(sess, "/aura weapon: kept, not yet in the world: "+err.Error())
		}
	}
	return true
}

// instanceAuraWords says a copy's row in words.
func instanceAuraWords(a database.InstanceAura) string {
	if a.Empty() {
		return "no tag and no aura of its own"
	}
	tag := "no tag"
	if a.Tag != "" {
		tag = "tag \"Aura: " + a.Tag + "\""
	}
	s := tag + ", first " + auraWords(a.First)
	if a.Second.SecondShows() {
		s += ", second " + auraWords(a.Second)
	}
	if !a.Shows() {
		s += " (a tag alone: the item's record aura shows)"
	}
	return s
}
