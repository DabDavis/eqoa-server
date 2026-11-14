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

// /aura (#aura): a GM gives an aura to the NPCs of its target's name (table
// npctint's glow and aura columns, entity/npctint.go) or to an item's worn
// piece (table itemaura, entity/pieceaura.go), kept, audited and sent at once
// to every client that draws auras (the owner, 2026-11-13: Mr. Moogie's aura
// "can be a template to make any model have particle effects ... even pieces
// of armor like bracers"; claude-notes/auras-20261113.md):
//
//	/aura <kind> <RRGGBBAA> [source] [nodes]    the target NPC's name
//	/aura off                                   its name's glow and aura gone (its tint kept)
//	/aura item <id> <kind> <RRGGBBAA> [source] [nodes]
//	/aura item <id> off
//
// The easier words (the owner, 2026-11-13: "make /aura command easier for gms
// to use like /aura blue particles chest, /aura yellow shell wholebody (the
// whole model like mr moogie)"), in any order, any case:
//
//	/aura blue particles chest      the target player's (or your own, or a merc's) chest piece alone (charaura)
//	/aura yellow shell wholebody    the target NPC's whole body (its name's npctint); on a
//	                                player its body and underclothes alone, under its armour (charaura)
//	/aura black trim legs nodes=4 source=88EC092D
//	/aura off chest, /aura off wholebody
//	/aura epic weapons source=Sickle_of_Avhi_Escron
//	                                the item held in each hand drawn with the epic's own glow, sheen and
//	                                particles (charaura slots 9 and 10; the owner, 2026-11-14: "give zaeires
//	                                equipped weapons the necro's tainted aura from the epic sickle")
//
// colour: a name (auraColors) or RRGGBB / RRGGBBAA hex, black a dark aura;
// kind: shell, particles (sparkles), both, trim, trimshell, off (none); where:
// chest, bracers (arms), gloves (hands), legs, boots (feet), helm (head), robe,
// cape (cloak; 2026-11-14) (that character's piece in the slot alone, whatever item it is: charaura,
// the owner's rule of 2026-11-13; itemaura, an item that always glows, is the
// panel's and /aura item's) or wholebody (body, whole: an NPC's name; a
// player's or merc's own body and underclothes), mainhand (primary), offhand
// (secondary; a shield held there too) or weapons (both hands): the item held
// there, whatever it is, of kind epic (the source epic's own glow, sheen and
// particles; a colour recolours them, none keeps the epic's) or particles
// alone; none: an NPC's whole body. source=
// a DictID in hex or an item's name (its model), nodes= 0-32. The old forms
// below are read first, as they were.
//
// kind: default, none, shell, particles, both, trim, trimshell (or 0-6); the
// colour hex, 0 none, a black or near-black one (database.AuraDark: every
// channel at most 30) a DARK aura, a shadow; source a DictID in hex (0 the
// Sword of Virtue's); nodes 0-32.

const auraUsage = "Usage: /aura <kind> <RRGGBBAA> [source] [nodes] on your target NPC's name, /aura off; " +
	"/aura item <id> <kind> <RRGGBBAA> [source] [nodes], /aura item <id> off. kind: default, none, shell, particles, both, " +
	"trim, trimshell (0-6); a colour of every channel at most 30 (000000FF black) is a dark aura; source: a DictID in hex (0 the Sword of Virtue's); nodes 0-32 (0 the default)"

// auraKinds are the kinds by name (database.AuraDefault ...).
var auraKinds = map[string]uint8{"default": database.AuraDefault, "none": database.AuraNone, "shell": database.AuraShell,
	"particles": database.AuraParticles, "both": database.AuraBoth, "trim": database.AuraTrim, "trimshell": database.AuraTrimShell}

// auraKindNames are the kinds' names by number.
var auraKindNames = [...]string{"default", "none", "shell", "particles", "both", "trim", "trimshell", "epic", "circlet", "energy"}

// parseHex32 reads a 32-bit hex value, with or without 0x.
func parseHex32(s string) (uint32, bool) {
	s = strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(s), "0x"), "#")
	v, err := strconv.ParseUint(s, 16, 32)
	return uint32(v), err == nil
}

// parseAura reads <kind> <RRGGBBAA> [source] [nodes], or "off" (all zero).
func parseAura(args []string) (a database.ItemAura, ok bool) {
	if len(args) == 1 && strings.EqualFold(args[0], "off") {
		return a, true
	}
	if len(args) < 2 || len(args) > 4 {
		return a, false
	}
	k, named := auraKinds[strings.ToLower(args[0])]
	if !named {
		n, err := strconv.Atoi(args[0])
		if err != nil || !database.ValidAuraKind(int64(n)) {
			return a, false
		}
		k = uint8(n)
	}
	a.Kind = k
	if a.Color, ok = parseHex32(args[1]); !ok {
		return a, false
	}
	if len(args) > 2 {
		if a.Source, ok = parseHex32(args[2]); !ok {
			return a, false
		}
	}
	if len(args) > 3 {
		n, err := strconv.Atoi(args[3])
		if err != nil || !database.ValidAuraNodes(int64(n)) {
			return a, false
		}
		a.Nodes = uint8(n)
	}
	return a, true
}

// auraWords says an aura in words.
func auraWords(a database.ItemAura) string {
	if !a.Shows() {
		return "no aura"
	}
	kind := strconv.Itoa(int(a.Kind))
	if int(a.Kind) < len(auraKindNames) {
		kind = auraKindNames[a.Kind]
	}
	src := "the Sword of Virtue's"
	if a.Source != 0 {
		src = fmt.Sprintf("%08X's", a.Source)
	}
	if a.Kind == database.AuraCirclet {
		color := "its own colours"
		if database.NPCGlowShows(a.Color) {
			color = fmt.Sprintf("in %08X", a.Color)
		}
		at := ""
		if a.Nodes != 0 {
			at = fmt.Sprintf(", %d%% up the head", a.Nodes)
		}
		return fmt.Sprintf("aura circlet: %s glowing ring worn round the head (while no helm is drawn), %s%s", src, color, at)
	}
	if a.Kind == database.AuraEpic {
		color := "its own colours"
		if database.NPCGlowShows(a.Color) {
			color = fmt.Sprintf("in %08X", a.Color)
		}
		where := ""
		if a.Nodes != 0 {
			where = fmt.Sprintf(", its particles %d%% along the weapon", a.Nodes)
		}
		return fmt.Sprintf("aura epic: %s look (glow, sheen and particles), %s%s", src, color, where)
	}
	nodes := "the default nodes"
	if a.Nodes != 0 {
		nodes = fmt.Sprintf("%d nodes", a.Nodes)
	}
	dark := ""
	if database.AuraDark(a.Color) && database.AuraNamed(a.Kind) {
		dark = " (dark)"
	}
	return fmt.Sprintf("aura %s in %08X%s (particles %s, %s)", kind, a.Color, dark, src, nodes)
}

// auraUsageShort is /aura with nothing: the easy words, with examples.
const auraUsageShort = "Usage: /aura <colour> <kind> <where>, any order: /aura blue particles chest, /aura yellow shell wholebody, " +
	"/aura epic weapons source=<epic>, /aura looks: named. Colour: red blue green yellow gold orange purple pink cyan white black silver teal " +
	"or RRGGBB(AA) (black: dark); kind: shell particles both trim trimshell epic off; where: chest bracers gloves legs boots " +
	"helm robe cape mainhand offhand weapons (theirs alone) or wholebody (an NPC's whole model; a player's body under its armour); source=<DictID|item name> nodes=N"

// auraColors are the colours by name, alpha FF.
var auraColors = map[string]uint32{
	"red": 0xFF2020FF, "blue": 0x3060FFFF, "green": 0x30FF40FF, "yellow": 0xFFFF30FF, "gold": 0xFFC040FF,
	"orange": 0xFF8020FF, "purple": 0xA040FFFF, "pink": 0xFF60C0FF, "cyan": 0x30FFFFFF, "white": 0xFFFFFFFF,
	"black": 0x000000FF, "silver": 0xC0C8D8FF, "teal": 0x20C0B0FF,
}

// auraWhere are the places by word: a worn piece (entity.AuraChest ...), a
// hand (entity.AuraMainHand, AuraOffHand), auraWholeBody, or auraBothHands
// (both hands' slots at once).
const (
	auraWholeBody = -2
	auraBothHands = -3
)

var auraWhere = map[string]int{
	"chest": entity.AuraChest, "breastplate": entity.AuraChest, "bracers": entity.AuraBracer, "bracer": entity.AuraBracer,
	"arms": entity.AuraBracer, "gloves": entity.AuraGloves, "hands": entity.AuraGloves, "legs": entity.AuraLegs,
	"boots": entity.AuraBoots, "feet": entity.AuraBoots, "helm": entity.AuraHelm, "head": entity.AuraHelm,
	"robe": entity.AuraRobe, "cape": entity.AuraCape, "cloak": entity.AuraCape,
	"mainhand": entity.AuraMainHand, "primary": entity.AuraMainHand, "offhand": entity.AuraOffHand, "secondary": entity.AuraOffHand,
	"weapons": auraBothHands, "weapon": auraBothHands, "bothhands": auraBothHands, "wholebody": auraWholeBody, "body": auraWholeBody, "whole": auraWholeBody,
}

// auraPieceNames name the worn pieces in replies.
var auraPieceNames = [entity.AuraSlots]string{"chest", "bracers", "gloves", "legs", "boots", "helm", "robe", "body", "cape", "main hand", "off hand"}

// auraEasyKinds are the easy words' kinds (off is apart).
var auraEasyKinds = map[string]uint8{"shell": database.AuraShell, "particles": database.AuraParticles,
	"sparkles": database.AuraParticles, "both": database.AuraBoth, "trim": database.AuraTrim, "trimshell": database.AuraTrimShell,
	"default": database.AuraDefault, "epic": database.AuraEpic, "circlet": database.AuraCirclet, "energy": database.AuraEnergy}

// auraSpec is the easy words read.
type auraSpec struct {
	A          database.ItemAura // colour, kind, source, nodes
	Off        bool
	Where      int // a piece, auraWholeBody, or -1 none named
	SourceName string
	Look       string // a named look's name (auraLooks), "" none
}

// auraLook is a named aura a GM gives by its name (the owner, 2026-11-14:
// "give zerkziee hachet and ranger level 60 epics the level 49 druid staff
// glow (also make a template like other auras)"): its words, in order, in
// /aura (any case; also run together or with underscores), and the aura they
// stand for. An epic look goes on a held weapon (both hands when no place is
// named); a colour named with it recolours it.
type auraLook struct {
	Words []string
	A     database.ItemAura
	What  string
	Hands bool // a held weapon's whatever its kind: both hands unless one is named
}

// CircletHeightMonk is the monk circlet's height on the head, percent of it
// from the chin (database.ValidCharAuraNodes).
const CircletHeightMonk = 72

// auraLooks are the named looks, /aura looks lists them.
var auraLooks = []auraLook{
	// Nodes on a hand is where along the weapon (database.AuraAlongMost): the
	// staff's sparkles at 45% of the weapon's length, mid-blade on a
	// scimitar (the owner, 2026-11-14: "lets move the druid epic aura down
	// zerkziee scimitar down the blade more"; until then at the staff's own
	// share, 78%, near the tip)
	{[]string{"druid", "staff", "glow"}, database.ItemAura{Kind: database.AuraEpic, Source: 0xCBF4135A, Nodes: 45},
		"the druid's level 49 epic staff, Oak Staff of Eilerawdl (CBF4135A): an amber glow, a gold sheen and its sparkles mid-blade (along=N moves them)", false},
	// the monk's level 49 epic, the Ascendant staff: the glowing ring with its
	// criss-cross at its yin-yang end worn as a circlet (the owner,
	// 2026-11-14: "the effect on the end of the monks epic yin yang staff the
	// circle of light with the criss cross lets make that effect into a
	// circlet on the head of dust")
	// Nodes on a circlet is how high on the head (database.AuraAlongMost): the
	// upper forehead and temples, above the brows (the owner, 2026-11-14: "the
	// circlet is sitting too low on dust"; the client's own 62% crossed the eyes)
	{[]string{"monk", "circlet"}, database.ItemAura{Kind: database.AuraCirclet, Source: 0x1D875ECD, Nodes: CircletHeightMonk},
		"the monk's level 49 epic staff, Ascendant staff (1D875ECD): its glowing ring with the criss-cross worn round the head at the temples (while no helm is drawn; height=N moves it)", false},
	{[]string{"sickle", "taint"}, database.ItemAura{Kind: database.AuraEpic, Source: 0x5FD4AE7C},
		"the necromancer's epic, Sickle of Avhi Escron (5FD4AE7C): an olive glow and its green mist (Zaeire's)", false},
	// a spell visual's hand emitters on a held weapon (the owner, 2026-11-14:
	// "0x6c151a60 the spell effect of the player's hands the ice dripping make
	// a weapon aura with /aura blue freezing offhand"): the particles kind with
	// the spell visual as its source (vanilla's aura::spell_emitters: the ice
	// drip 0x5D471C8E its caster's list starts on the hands), at the hand, or
	// along= up the weapon; a colour turns it that hue
	{[]string{"freezing"}, database.ItemAura{Kind: database.AuraParticles, Source: 0x6C151A60},
		"the ice dripping off a caster's hands (spell visual 6C151A60) on a held weapon (/aura blue freezing offhand)", true},
}

// takeAuraLook finds a named look in args (its words in a row, or run
// together, or joined by underscores) and returns the rest without it.
func takeAuraLook(args []string) ([]string, *auraLook) {
	low := make([]string, len(args))
	for i, a := range args {
		low[i] = strings.ToLower(strings.TrimSpace(a))
	}
	for k := range auraLooks {
		l := &auraLooks[k]
		joined := strings.Join(l.Words, "")
		for i := range low {
			if strings.ReplaceAll(low[i], "_", "") == joined {
				return append(append([]string(nil), args[:i]...), args[i+1:]...), l
			}
			if i+len(l.Words) <= len(low) && strings.Join(low[i:i+len(l.Words)], " ") == strings.Join(l.Words, " ") {
				return append(append([]string(nil), args[:i]...), args[i+len(l.Words):]...), l
			}
		}
	}
	return args, nil
}

// auraLooksReply is /aura looks: every named look and how to give it, a
// line each (one chat message each stays one fragment).
func auraLooksReply() []string {
	out := []string{"Named auras (on a targeted player or yourself; an epic look goes on the held weapons, or name mainhand/offhand; a circlet on the head; a colour recolours it):"}
	for _, l := range auraLooks {
		out = append(out, fmt.Sprintf("/aura %s: %s.", strings.Join(l.Words, " "), l.What))
	}
	return out
}

// isHex reports whether s is n hex digits.
func isHex(s string, n int) bool {
	if len(s) != n {
		return false
	}
	_, err := strconv.ParseUint(s, 16, 32)
	return err == nil
}

// parseAuraEasy reads the easy words; an error names the word it could not.
func parseAuraEasy(args []string) (sp auraSpec, err error) {
	sp.Where = -1
	args, look := takeAuraLook(args)
	color, kind, naming, along, nodes, height := false, false, false, false, false, false
	for _, raw := range args {
		w := strings.ToLower(strings.TrimSpace(raw))
		if w == "" {
			continue
		}
		if k, v, eq := strings.Cut(w, "="); eq {
			switch k {
			case "source", "src":
				orig := raw[strings.Index(raw, "=")+1:]
				if h, ok := parseHex32(v); ok && v != "" {
					sp.A.Source = h
				} else if strings.TrimSpace(orig) != "" {
					// an item's name: its words after it too, up to a
					// word of the command's (underscores are spaces)
					sp.SourceName, naming = strings.ReplaceAll(strings.TrimSpace(orig), "_", " "), true
				} else {
					return sp, fmt.Errorf("%s: source=<DictID in hex or an item's name>", raw)
				}
			case "nodes":
				n, e := strconv.Atoi(v)
				if e != nil || !database.ValidAuraNodes(int64(n)) {
					return sp, fmt.Errorf("%s: nodes is 0-%d", raw, database.AuraNodesMost)
				}
				sp.A.Nodes, nodes = uint8(n), true
			case "along":
				// where along a held weapon: a share of its length in percent
				n, e := strconv.Atoi(strings.TrimSuffix(v, "%"))
				if e != nil || n < 1 || n > database.AuraAlongMost {
					return sp, fmt.Errorf("%s: along is 1-%d, percent of the weapon's length from the hand", raw, database.AuraAlongMost)
				}
				sp.A.Nodes, along = uint8(n), true
			case "height":
				// how high on the head a circlet sits: a share of the head in percent
				n, e := strconv.Atoi(strings.TrimSuffix(v, "%"))
				if e != nil || n < 1 || n > database.AuraAlongMost {
					return sp, fmt.Errorf("%s: height is 1-%d, percent of the head from the chin", raw, database.AuraAlongMost)
				}
				sp.A.Nodes, height = uint8(n), true
			default:
				return sp, fmt.Errorf("%s: only source=, nodes=, along= and height= take a value", raw)
			}
			continue
		}
		_, isKind := auraEasyKinds[w]
		_, isWhere := auraWhere[w]
		_, isColor := auraColors[w]
		if naming && !isKind && !isWhere && !isColor && w != "off" && w != "none" {
			sp.SourceName += " " + strings.TrimSpace(raw)
			continue
		}
		naming = false
		if w == "off" || w == "none" {
			sp.Off = true
			continue
		}
		if k, ok := auraEasyKinds[w]; ok {
			if kind {
				return sp, fmt.Errorf("%s: one kind at a time", raw)
			}
			sp.A.Kind, kind = k, true
			continue
		}
		if at, ok := auraWhere[w]; ok {
			if sp.Where != -1 {
				return sp, fmt.Errorf("%s: one place at a time", raw)
			}
			sp.Where = at
			continue
		}
		c, named := auraColors[w]
		h := strings.TrimPrefix(strings.TrimPrefix(w, "0x"), "#")
		switch {
		case named:
		case isHex(h, 6):
			v, _ := strconv.ParseUint(h, 16, 32)
			c = uint32(v)<<8 | 0xFF
		case isHex(h, 8):
			v, _ := strconv.ParseUint(h, 16, 32)
			c = uint32(v)
		default:
			return sp, fmt.Errorf("%s: not a colour, kind or place", raw)
		}
		if color {
			return sp, fmt.Errorf("%s: one colour at a time", raw)
		}
		sp.A.Color, color = c, true
	}
	if look != nil {
		switch {
		case sp.Off:
			return sp, fmt.Errorf("off takes only a place (/aura off weapons)")
		case kind && sp.A.Kind != look.A.Kind:
			return sp, fmt.Errorf("%s names its own kind", strings.Join(look.Words, " "))
		case sp.A.Source != 0 || sp.SourceName != "":
			return sp, fmt.Errorf("%s names its own source", strings.Join(look.Words, " "))
		}
		sp.A.Kind, sp.A.Source, sp.Look = look.A.Kind, look.A.Source, strings.Join(look.Words, " ")
		if !along && !nodes && !height {
			sp.A.Nodes = look.A.Nodes // the look's own place along the weapon, or height on the head
		}
		if sp.Where == -1 && (look.A.Kind == database.AuraEpic || look.Hands) {
			sp.Where = auraBothHands
		}
		if sp.Where == -1 && look.A.Kind == database.AuraCirclet {
			sp.Where = entity.AuraHelm // a circlet goes round the head
		}
	}
	if height {
		switch {
		case along || nodes:
			return sp, fmt.Errorf("height= is a circlet's, one byte with along= and nodes=")
		case sp.A.Kind != database.AuraCirclet:
			return sp, fmt.Errorf("height= is a circlet's: /aura monk circlet height=N")
		}
	}
	if along {
		hand := sp.Where == entity.AuraMainHand || sp.Where == entity.AuraOffHand || sp.Where == auraBothHands
		switch {
		case nodes:
			return sp, fmt.Errorf("along= and nodes= are one byte: along= on a weapon, nodes= on the body")
		case !hand:
			return sp, fmt.Errorf("along= is a held weapon's: mainhand, offhand or weapons")
		case sp.Off:
			return sp, fmt.Errorf("off takes only a place (/aura off weapons)")
		}
	}
	if look != nil {
		return sp, nil
	}
	if sp.Off {
		if color || kind {
			return sp, fmt.Errorf("off takes only a place (/aura off chest)")
		}
		sp.A = database.ItemAura{}
		return sp, nil
	}
	if !kind {
		if !color {
			return sp, fmt.Errorf("name a colour or a kind")
		}
		sp.A.Kind = database.AuraShell
	}
	if !color && sp.A.Kind != database.AuraParticles && sp.A.Kind != database.AuraEpic && sp.A.Kind != database.AuraCirclet && sp.A.Kind != database.AuraEnergy {
		return sp, fmt.Errorf("name a colour (only particles and epic need none: they keep the source's own)")
	}
	return sp, nil
}

// oldAuraForm reports whether args are the old <kind> <RRGGBBAA> [source]
// [nodes] (a kind word or 0-6 first, a colour of other than six digits: six
// are the easy words' RRGGBB) and reads them.
func oldAuraForm(args []string) (database.ItemAura, bool) {
	if len(args) < 2 {
		return database.ItemAura{}, false
	}
	h := strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(args[1]), "0x"), "#")
	if isHex(h, 6) {
		return database.ItemAura{}, false
	}
	return parseAura(args)
}

// auraCommand is /aura's: see the file's doc.
func (h *ChatHandler) auraCommand(sess *session.Session, player *entity.Player, args []string) {
	db := h.camps.db
	if db == nil {
		h.gmReply(sess, "/aura: no database: nothing is kept")
		return
	}
	if len(args) == 0 {
		h.gmReply(sess, auraUsageShort)
		return
	}
	if strings.EqualFold(args[0], "item") {
		if len(args) < 3 {
			h.gmReply(sess, auraUsage)
			return
		}
		id, err := strconv.Atoi(args[1])
		a, ok := parseAura(args[2:])
		if err != nil || id < 0 || !ok {
			h.gmReply(sess, auraUsage)
			return
		}
		a.PatternID = id
		h.setItemAura(sess, player, a, fmt.Sprintf("Item %d", id))
		return
	}
	if a, ok := oldAuraForm(args); ok {
		h.setNPCAura(sess, player, a)
		return
	}
	if len(args) == 1 && strings.EqualFold(args[0], "looks") {
		for _, l := range auraLooksReply() {
			h.gmReply(sess, l)
		}
		return
	}
	sp, err := parseAuraEasy(args)
	if err != nil {
		h.gmReply(sess, "/aura: "+err.Error()+". "+auraUsageShort)
		return
	}
	if sp.SourceName != "" {
		var model int64
		if e := db.QueryRow(`SELECT model FROM itempattern WHERE itemname = ? COLLATE NOCASE AND model != 0 ORDER BY patternid LIMIT 1`,
			sp.SourceName).Scan(&model); e != nil {
			h.gmReply(sess, fmt.Sprintf("/aura: no item named %q with a model for source=", sp.SourceName))
			return
		}
		sp.A.Source = uint32(model)
	}
	// whom: the target, else yourself
	var target entity.Entity
	if player.TargetObjectID != 0 && player.TargetObjectID != entity.NoAssist {
		ent, found := h.gameWorld.FindEntityByObjectID(sess, player.TargetObjectID)
		if !found {
			h.gmReply(sess, "/aura: your target is gone")
			return
		}
		target = ent
	}
	npc, isNPC := target.(*entity.NPC)
	mercKey, isMerc := database.CharAuraKey{}, false
	if isNPC {
		mercKey, isMerc = npc.MercAuraKey()
	}
	// an NPC's whole body (not a mercenary's): its name's, npctint
	if isNPC && !isMerc && sp.Where != -1 && sp.Where != auraWholeBody {
		h.gmReply(sess, fmt.Sprintf("/aura: %s wears no items of its own (its armour is its sets): /aura <colour> <kind> wholebody", npc.Name))
		return
	}
	if isNPC && !isMerc {
		if sp.A.Kind == database.AuraEpic && !sp.Off {
			h.gmReply(sess, "/aura: epic is a held weapon's look: on a player (or a mercenary): /aura epic weapons source=<epic's name>")
			return
		}
		if sp.A.Kind == database.AuraCirclet && !sp.Off {
			h.gmReply(sess, "/aura: a circlet is a player's (or a mercenary's) head's: /aura monk circlet")
			return
		}
		if database.NPCClassKey(npc.Name) == "" {
			h.gmReply(sess, "/aura: your target has no name to keep an aura by")
			return
		}
		h.setNPCAura(sess, player, sp.A)
		return
	}
	if sp.Where == -1 {
		h.gmReply(sess, "/aura: name where on a player: wholebody (the body and underclothes, under the armour) or a slot "+
			"(chest, bracers, gloves, legs, boots, helm, robe, cape, mainhand, offhand, weapons): /aura blue shell wholebody, /aura blue shell chest")
		return
	}
	// a character's own (charaura): the target player, yourself, or a mercenary
	var key database.CharAuraKey
	var mask uint16
	who, whose := "You", "your"
	switch t := target.(type) {
	case nil:
		key, mask = database.CharAuraKey{CharID: player.CharServerID}, player.WornMask
	case *entity.Player:
		key, mask, who, whose = database.CharAuraKey{CharID: t.CharServerID}, t.WornMask, t.Name, t.Name+"'s"
	case *entity.NPC:
		key, mask, who, whose = mercKey, t.WornMask, t.Name, t.Name+"'s"
	default:
		h.gmReply(sess, "/aura: target a player, a mercenary or nothing (yourself)")
		return
	}
	if key.CharID == 0 {
		h.gmReply(sess, "/aura: "+who+" has no character record to keep an aura by")
		return
	}
	slots, place := []int{database.CharAuraBody}, "body (under the armour)"
	switch sp.Where {
	case auraWholeBody:
	case auraBothHands:
		slots, place = []int{database.CharAuraMainHand, database.CharAuraOffHand}, "weapons (both hands)"
	default:
		slots, place = []int{sp.Where}, auraPieceNames[sp.Where]
	}
	// a hand's kinds: epic or particles; epic a hand's alone
	if sp.A.Shows() {
		for _, slot := range slots {
			if database.ValidCharAuraKind(int64(slot), int64(sp.A.Kind)) {
				continue
			}
			if slot == database.CharAuraMainHand || slot == database.CharAuraOffHand {
				h.gmReply(sess, "/aura: a held weapon takes epic, energy or particles: /aura epic weapons source=Sickle_of_Avhi_Escron, /aura energy offhand source=<item>")
			} else if sp.A.Kind == database.AuraCirclet {
				h.gmReply(sess, "/aura: a circlet goes round the head: /aura monk circlet (or helm/head)")
			} else {
				h.gmReply(sess, "/aura: epic is a held weapon's look: mainhand, offhand or weapons")
			}
			return
		}
	}
	detail := fmt.Sprintf("/aura in the game: %s %s: %s", whose, place, auraWords(sp.A))
	if sp.Look != "" {
		detail += " (the named look " + sp.Look + ")"
	}
	for _, slot := range slots {
		if err := db.SetCharAuraAudited(key, slot, sp.A, gmEditOf(sess), detail); err != nil {
			h.gmReply(sess, "/aura: not kept: "+err.Error())
			return
		}
	}
	slot := slots[0]
	if _, err := h.gameWorld.LoadItemAuras(db); err != nil {
		h.gmReply(sess, "/aura: kept, not yet in the world: "+err.Error())
		return
	}
	note := ""
	if sp.Look != "" && sp.A.Shows() {
		note = " (" + sp.Look + ")"
	}
	switch {
	case slot == database.CharAuraMainHand || slot == database.CharAuraOffHand:
		note += " (on whatever that hand holds, from a client that draws held items' auras)"
	case sp.A.Kind == database.AuraCirclet:
		note += " (round the head while no helm is drawn, from a client that draws circlets)"
	case slot != database.CharAuraBody && mask&(1<<slot) == 0 && sp.A.Shows():
		note += " (nothing worn there now: it shows when a piece is)"
	}
	h.gmReply(sess, fmt.Sprintf("%s %s: now %s (%s), %s alone%s.", strings.ToUpper(whose[:1])+whose[1:], place, auraNow(sp.A), auraWords(sp.A),
		map[bool]string{true: "this mercenary", false: "this character"}[isMerc], note))
	log.Printf("GM: %s aura %s %s: %s", player.Name, whose, place, auraWords(sp.A))
}

// auraNow says an aura in the easy words: "a blue particles aura", "no aura".
func auraNow(a database.ItemAura) string {
	if !a.Shows() {
		return "no aura"
	}
	kind := strconv.Itoa(int(a.Kind))
	if int(a.Kind) < len(auraKindNames) {
		kind = auraKindNames[a.Kind]
	}
	color := fmt.Sprintf("%08X", a.Color)
	for n, c := range auraColors {
		if c == a.Color {
			color = n
		}
	}
	if a.Kind == database.AuraCirclet && !database.NPCGlowShows(a.Color) {
		return fmt.Sprintf("a circlet of %08X", a.Source)
	}
	if a.Kind == database.AuraEpic && !database.NPCGlowShows(a.Color) {
		return fmt.Sprintf("the epic look of %08X", a.Source)
	}
	if database.AuraDark(a.Color) && database.AuraNamed(a.Kind) {
		color += " (dark)"
	}
	return fmt.Sprintf("a %s %s aura", color, kind)
}

// setItemAura keeps item a.PatternID's aura (audited) and puts it in the
// world; what names the item in the reply.
func (h *ChatHandler) setItemAura(sess *session.Session, player *entity.Player, a database.ItemAura, what string) {
	db := h.camps.db
	found, err := db.SetItemAuraAudited(a, gmEditOf(sess), fmt.Sprintf("/aura in the game: %s %s", what, auraWords(a)))
	switch {
	case err != nil:
		h.gmReply(sess, "/aura: not kept: "+err.Error())
		return
	case !found:
		h.gmReply(sess, fmt.Sprintf("/aura: no item %d", a.PatternID))
		return
	}
	n, err := h.gameWorld.LoadItemAuras(db)
	if err != nil {
		h.gmReply(sess, "/aura: kept, not yet in the world: "+err.Error())
		return
	}
	h.gmReply(sess, fmt.Sprintf("%s now has %s (%s); everyone wearing it shows it, at once (%d items with an aura).", what, auraNow(a), auraWords(a), n))
	log.Printf("GM: %s aura %s: %s", player.Name, what, auraWords(a))
}

// setNPCAura keeps the target NPC's name's aura (npctint, audited) and sends it.
func (h *ChatHandler) setNPCAura(sess *session.Session, player *entity.Player, a database.ItemAura) {
	db := h.camps.db
	if player.TargetObjectID == 0 || player.TargetObjectID == entity.NoAssist {
		h.gmReply(sess, "/aura: target a creature (or /aura item <id> ..., or name a worn slot: /aura blue shell chest)")
		return
	}
	ent, found := h.gameWorld.FindEntityByObjectID(sess, player.TargetObjectID)
	npc, isNPC := ent.(*entity.NPC)
	if !found || !isNPC || database.NPCClassKey(npc.Name) == "" {
		h.gmReply(sess, "/aura: your target is no creature: whole-body auras are for NPCs; for a player's armour name the slot (/aura blue shell chest)")
		return
	}
	name := database.NPCClassKey(npc.Name)
	if err := db.SetNPCTintAuraAudited(name, a.Color, a.Kind, a.Source, a.Nodes, gmEditOf(sess),
		fmt.Sprintf("/aura in the game: %s %s", name, auraWords(a))); err != nil {
		h.gmReply(sess, "/aura: not kept: "+err.Error())
		return
	}
	_, told, err := h.gameWorld.LoadNPCTints(db)
	if err != nil {
		h.gmReply(sess, "/aura: kept, not yet in the world: "+err.Error())
		return
	}
	h.gmReply(sess, fmt.Sprintf("Every %s: %s, kept; sent to %d client(s) that draw auras.", name, auraWords(a), told))
	log.Printf("GM: %s aura %s: %s", player.Name, name, auraWords(a))
}

// epicLook is a class's epic weapon's hand model, in the disc's order (ITEM.ESF
// models 44-78, claude-notes/epic-weapons-looks-20261106.md), and what its look
// carries: every one an emissive glow, some an additive sheen, some particles.
type epicLook struct {
	Class, Model, Name string
	Source             uint32
	Sheen, Particles   bool
}

// epicLooks are the epic weapons' looks, by class in the disc's order (the
// level 60 epics wear these models too: the ranger's axes, the monk's claws).
var epicLooks = []epicLook{
	{"shadowknight", "Greatsword", "Beheader of Isthiak", 0x6EB7224D, true, true},
	{"shadowknight", "Polearm", "Downfall", 0x867ED577, true, true},
	{"shadowknight", "Greataxe", "the shadow knight's greataxe", 0x9A1DBA92, true, true},
	{"paladin", "Warhammer", "Judgement", 0xF841266D, true, true},
	{"paladin", "Greatsword", "Virtus Ferrum", 0x8A82B605, true, true},
	{"paladin", "Lance", "the paladin's lance", 0xF2877ACE, true, true},
	{"warrior", "Longsword", "Steelwind", 0x17F9870B, true, false},
	{"warrior", "Greataxe", "Foereaver", 0x942735D3, true, false},
	{"warrior", "Greatsword", "the warrior's greatsword", 0xB44E7A96, true, true},
	{"druid", "Scimitar", "Triad Scimitar", 0xD03942DE, true, false},
	{"druid", "Staff", "Oak Staff of Eilerawdl", 0xCBF4135A, true, true},
	{"cleric", "Hammer", "Hammer of Absolution", 0xB2116A98, true, false},
	{"cleric", "Greathammer", "Mace of Repentence", 0xD8406C85, true, false},
	{"shaman", "Club", "Cudgel of the Spirit Guide", 0xDF034C5A, true, false},
	{"shaman", "Spear", "Spear of Fate", 0x38107CE9, true, true},
	{"bard", "Sabre", "Ballad of Mithril", 0x8E85E4A6, true, false},
	{"bard", "Rapier", "Selo's Jeweled Rapier", 0xF4DD991A, true, false},
	{"bard", "MainGauche", "the bard's main gauche", 0x9EA9384F, true, false},
	{"ranger", "Longsword", "Pure Emerald Longsword", 0xF3A41E80, true, false},
	{"ranger", "Scimitar", "Scimitar of Fury", 0xAB6B7D29, true, false},
	{"ranger", "Axe", "Axe of Nature's Innocence/Purity", 0x11666A69, true, false},
	{"monk", "Knuckles", "Sunsilver Blade", 0xAACB29AF, false, false},
	{"monk", "Staff", "Ascendant staff", 0x1D875ECD, true, true},
	{"monk", "Claws", "Claws of Insurrection/Mayhem", 0x039995A5, true, true},
	{"rogue", "Stiletto", "Heartsbane", 0x0D54F88C, false, true},
	{"rogue", "Dagger", "Blacktongue", 0xDD61A3D4, false, false},
	{"rogue", "Dirk", "the rogue's dirk", 0xBD3C3DC8, false, false},
	{"enchanter", "Rod", "Mithril Eagle Rod", 0x2EDA5089, true, true},
	{"enchanter", "Staff", "Glory of Elddar", 0xDAF955A6, true, true},
	{"wizard", "Rod", "Pulsaric Rod", 0x29B36334, true, true},
	{"wizard", "Staff", "Staff of Nebula", 0x7C7DBEA9, false, true},
	{"magician", "Rod", "Rod of Elemental Union", 0x784CC2A6, true, true},
	{"magician", "Staff", "Crystalmana Crosier", 0x63DCEFAB, true, true},
	{"necromancer", "Totem", "Rod of Consuming Hatred", 0xFD9C3858, true, true},
	{"necromancer", "Sickle", "Sickle of Avhi Escron", 0x5FD4AE7C, false, true},
}

// The epics' named looks (the owner, 2026-11-14: "/aura epic necromancer
// primary etc. to make it easier, if multiple make it druid1, druid2 etc. if
// they have particle effect"): "epic <class>" its first epic's look, "epic
// <class>N" each of them by the disc's order, on both weapons unless
// primary/secondary (mainhand/offhand) is named. Every one glows (its emissive);
// the line says which also carry a sheen and which particles.
func init() {
	n := map[string]int{}
	for _, e := range epicLooks {
		n[e.Class]++
	}
	seen := map[string]int{}
	for _, e := range epicLooks {
		seen[e.Class]++
		what := "a glow"
		if e.Sheen {
			what += ", a sheen"
		}
		if e.Particles {
			what += " and particles"
		}
		a := database.ItemAura{Kind: database.AuraEpic, Source: e.Source}
		line := fmt.Sprintf("the %s epic %s, %s (%08X): %s", e.Class, strings.ToLower(e.Model), e.Name, e.Source, what)
		if seen[e.Class] == 1 {
			// the class's first, by its name alone
			auraLooks = append(auraLooks, auraLook{[]string{"epic", e.Class}, a, line + fmt.Sprintf(" (also epic %s1)", e.Class), true})
		}
		if n[e.Class] > 1 || seen[e.Class] > 1 {
			auraLooks = append(auraLooks, auraLook{[]string{"epic", fmt.Sprintf("%s%d", e.Class, seen[e.Class])}, a, line, true})
		}
	}
}
