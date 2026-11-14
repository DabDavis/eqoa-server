package database

import (
	"fmt"
	"sort"
)

// npctint: a tint on the bodies of the NPCs of a name, for a client that draws
// one (entity/npctint.go, protocol.OpcodeNPCTints; the server owner,
// 2026-11-10: Yhafiran, "the black and green oriental dragon", wears the disc's
// only green oriental dragon, CHAR.ESF 0xB5C8A651, which the Plane of
// Disease's twins wear too, and no black and green body exists, so "vanilla
// draws Yhafiran tinted toward black and green"). A PS2 is never told: it draws
// the body in its own colours.
//
// color: 0xRRGGBBAA, red in the high byte (as chardyes holds a colour): each of
// the body's colours' red, green and blue times the tint's (255ths: 0xFF keeps
// a channel, 0x00 makes it black), as much of it as the alpha says (0xFF the
// whole, 0x80 about half, 0 none: a row kept with nothing drawn). The key is
// the name as npcclass keys it (NPCClassKey: any case, trimmed, no trailing
// '$'), so a GM's #spawn copy and every spawn of the name share it.
//
// glow (2026-11-13, the GM Jibrish's petition 1893: "Use the new glow mechanic
// to make mr. moogies whole body glow a soft red"): 0xRRGGBBAA, a light of the
// body's own, beside the tint: vanilla's held-item glow (its aura.rs: a
// material's emissive colour) laid on every layer of the body, the colour at
// the alpha's share (0xFF whole, 0 none: no glow, the default). Only a client
// that answers the offer with layout 2 is told it (entity/npctint.go); a row of
// a glow alone tints nothing for a layout 1 client. source: gm,
// or seed for tools/seed_npc_tint.py's rows (its --undo takes out exactly what
// it wrote).
//
// aura_kind, aura_source, aura_nodes (2026-11-13, the owner: Mr. Moogie's aura
// "can be a template to make any model have particle effects"; claude-notes/
// auras-20261113.md): which aura the name's bodies wear, in the glow's colour
// (AuraDefault the shell while the glow lights, AuraNone, AuraShell,
// AuraParticles, AuraBoth); the DictID of the item or model whose particle
// sprite the particles are (0 the Sword of Virtue's, 0x88EC092D); on how many
// of the body's nodes (0 the default 8; the client takes 32 at most). Told
// only to a client that answers the offer with layout 3; particles need no
// glow (drawn in the source's own colours).
const npcTintSchema = `
	CREATE TABLE IF NOT EXISTS npctint (
		name   TEXT PRIMARY KEY,
		color  INTEGER NOT NULL DEFAULT 4294967295,
		glow   INTEGER NOT NULL DEFAULT 0,
		aura_kind   INTEGER NOT NULL DEFAULT 0,
		aura_source INTEGER NOT NULL DEFAULT 0,
		aura_nodes  INTEGER NOT NULL DEFAULT 0,
		note   TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'gm'
	);`

// ensureNPCTint makes table npctint (initSchema).
func (db *SQLiteDB) ensureNPCTint() error {
	if _, err := db.Exec(npcTintSchema); err != nil {
		return fmt.Errorf("npctint: %w", err)
	}
	// a table made before the glow (2026-11-13) takes its column
	has, err := hasColumn(db.DB, "npctint", "glow")
	if err != nil {
		return fmt.Errorf("npctint: %w", err)
	}
	if !has {
		if _, err := db.Exec(`ALTER TABLE npctint ADD COLUMN glow INTEGER NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("npctint: glow: %w", err)
		}
	}
	// and one made before the auras (2026-11-13) theirs
	for _, col := range []string{"aura_kind", "aura_source", "aura_nodes"} {
		has, err := hasColumn(db.DB, "npctint", col)
		if err != nil {
			return fmt.Errorf("npctint: %w", err)
		}
		if !has {
			if _, err := db.Exec(`ALTER TABLE npctint ADD COLUMN ` + col + ` INTEGER NOT NULL DEFAULT 0`); err != nil {
				return fmt.Errorf("npctint: %s: %w", col, err)
			}
		}
	}
	return nil
}

// The aura kinds (npctint.aura_kind, itemaura.kind): vanilla's
// net::npctint::Aura.
const (
	AuraDefault   = 0 // the shell while the glow (the piece's colour) lights; nothing without
	AuraNone      = 1 // no aura, whatever the glow
	AuraShell     = 2 // a form-fitting glow round the body (or piece), in the glow's colour
	AuraParticles = 3 // a held item's emitter on the body's nodes; no glow: the source's colours
	AuraBoth      = 4 // the shell and the particles
	AuraTrim      = 5 // the armour's gold trim lit in the glow's colour (sets 4 and 9's plate), a soft halo off it
	AuraTrimShell = 6 // the trim and the shell
	AuraKindMost  = AuraTrimShell
	// AuraEpic is a held weapon drawn with an epic weapon's own look (the
	// owner, 2026-11-14: "give zaeires equipped weapons the necro's tainted
	// aura from the epic sickle"): the source's model (an ITEM.ESF hand
	// model, the epic's) lends the held item its materials' emissive glow,
	// its additive sheen and its particle sprites; a colour that lights
	// recolours them (as a held item's own colour does its glow), none keeps
	// the source's own. On the hands' charaura slots alone (CharAuraMainHand,
	// CharAuraOffHand: ValidCharAuraKind), which only a client that answered
	// the live look's bit 6 is sent: never in npctint or itemaura, so
	// AuraKindMost stays 6 and every older client reads what it did.
	AuraEpic = 7
	// AuraCirclet is an epic's effect worn as a circlet round the head (the
	// owner, 2026-11-14: "the effect on the end of the monks epic yin yang
	// staff the circle of light with the criss cross lets make that effect
	// into a circlet on the head of dust"): the source's glowing additive
	// part (the Ascendant staff's ring at its yin-yang end, 0x1D875ECD) laid
	// flat round the head at the brow and sized to it, following the head.
	// On the helm's charaura slot alone (CharAuraHelm), shown whether a helm
	// is worn or not (the client draws it while no helm is drawn); never in
	// npctint or itemaura. A client before it reads the default (a shell on a
	// worn helm while the colour lights: nothing with no colour).
	AuraCirclet = 8
	// AuraEnergy is an energy skin on a held item (the owner, 2026-11-14, of
	// Dust's Heirloom Shield of Twisted Virtue: "should look like the shield he
	// had before but shaped like the current shield now with the same diamond
	// and cross pattern but with the energy shield effect"): the held item keeps
	// its own mesh and layers, and the source's additive layers (texture,
	// colour, scroll: the old energy shield 0x2EC56029's streaks) are laid over
	// it, in the colour's hue when it lights (vanilla's aura::energy_skin). A
	// hand's alone (charaura 9/10, or the held item's own itemaura); a client
	// before it draws nothing of it.
	AuraEnergy = 9
)

// CharAuraHelm is the helm's charaura slot (entity.AuraHelm): a circlet's.
const CharAuraHelm = 5

// AuraDarkMost is the brightest an aura's colour (its RGB, before its alpha)
// is and still a DARK aura: a colour whose every channel is at most this
// (black 0x000000FF, a near-black 0x100018FF) draws as a darkening shadow, not
// an additive glow (vanilla's net::npctint::dark; the owner, 2026-11-13:
// "make dusts armor trim glow a black aura"). No wire of its own: the colour
// says it.
const AuraDarkMost = 0x30

// AuraDark reports whether an aura colour (0xRRGGBBAA) is a dark aura: an
// alpha, and RGB no channel of which passes AuraDarkMost (black included).
func AuraDark(glow uint32) bool {
	r, g, b := glow>>24, (glow>>16)&0xFF, (glow>>8)&0xFF
	return glow&0xFF != 0 && r <= AuraDarkMost && g <= AuraDarkMost && b <= AuraDarkMost
}

// AuraNamed reports whether a kind names a shell or a trim outright (not the
// default), which a black colour, lighting nothing, draws as a dark aura.
func AuraNamed(kind uint8) bool {
	return kind == AuraShell || kind == AuraBoth || kind == AuraTrim || kind == AuraTrimShell
}

// AuraNodesMost is the most nodes the client puts particles on.
const AuraNodesMost = 32

// AuraParticlesShow reports whether an aura kind draws particles, which need
// no glow.
func AuraParticlesShow(kind uint8) bool { return kind == AuraParticles || kind == AuraBoth }

// AuraShows reports whether an aura of a glow (0xRRGGBBAA) and kind draws
// anything: particles, or the shell (or trim) while the glow lights, or a
// shell or trim named outright in a dark colour (AuraDark: black too).
func AuraShows(glow uint32, kind uint8) bool {
	if kind == AuraEpic || kind == AuraCirclet || kind == AuraEnergy {
		return true // the source's own look, coloured or not
	}
	if kind > AuraKindMost {
		kind = AuraDefault // the client reads any other as the default
	}
	return AuraParticlesShow(kind) || (kind != AuraNone && NPCGlowShows(glow)) || (AuraNamed(kind) && AuraDark(glow))
}

// ValidAuraKind and ValidAuraNodes hold a GM's aura to what the client reads.
func ValidAuraKind(k int64) bool  { return k >= 0 && k <= AuraKindMost }
func ValidAuraNodes(n int64) bool { return n >= 0 && n <= AuraNodesMost }

// AuraAlongMost is the most a held item's aura's nodes byte says: on a hand
// (CharAuraMainHand, CharAuraOffHand) it is no node count but where along
// the held item the borrowed particles sit, 1-100 the share in percent of
// its reach from the hand (vanilla's aura::along), 0 the source's own share
// (the owner, 2026-11-14: "lets move the druid epic aura down zerkziee
// scimitar down the blade more"). A client before it ignores it.
const AuraAlongMost = 100

// ValidCharAuraNodes holds a charaura row's nodes to its slot and kind: a
// hand's 0-AuraAlongMost (where along the held item), a circlet's (kind
// AuraCirclet) 0-AuraAlongMost too (how high on the head it sits: 1-100 the
// share in percent of the head's height from the chin, 0 the client's own
// 62%, which crossed the eyes: the owner, 2026-11-14, "the circlet is sitting
// too low on dust"), every other's ValidAuraNodes.
func ValidCharAuraNodes(slot, kind, n int64) bool {
	if slot == CharAuraMainHand || slot == CharAuraOffHand || kind == AuraCirclet {
		return n >= 0 && n <= AuraAlongMost
	}
	return ValidAuraNodes(n)
}

// ValidItemAura holds an itemaura row's kind and nodes: the kinds a body or
// piece has (ValidAuraKind, nodes ValidAuraNodes), or a circlet (AuraCirclet,
// nodes its height 0-AuraAlongMost): an item that IS a circlet (the owner,
// 2026-11-14: "for dusts circlet make it an actual item with that model we just
// made, named crown of twist virtue"). A circlet is shown on a worn helm-slot
// item alone (entity.pieceAuras): on any other piece it is none.
func ValidItemAura(kind, nodes int64) bool {
	if kind == AuraCirclet || kind == AuraEpic || kind == AuraEnergy {
		return nodes >= 0 && nodes <= AuraAlongMost // a circlet's height, a held item's along
	}
	return ValidAuraKind(kind) && ValidAuraNodes(nodes)
}

// HandAuraKind reports whether a kind draws on a held item: the particles,
// an epic's look, an energy skin. An itemaura row of one on a held item's
// record shows on whatever hand holds it (entity.pieceAuras); of epic or
// energy on a worn piece, none.
func HandAuraKind(k int64) bool {
	return k == AuraParticles || k == AuraEpic || k == AuraEnergy
}

// ValidCharAuraKind holds a charaura row's kind to what its slot draws: a
// hand (CharAuraMainHand, CharAuraOffHand) none, the particles (the source's
// particle sprites on the held item) or the epic's look (AuraEpic); every
// other slot the kinds an aura on a body or piece has (ValidAuraKind).
func ValidCharAuraKind(slot, k int64) bool {
	if slot == CharAuraMainHand || slot == CharAuraOffHand {
		return HandAuraKind(k) || k == AuraNone
	}
	if slot == CharAuraHelm && k == AuraCirclet {
		return true // the helm's slot: a circlet too
	}
	return ValidAuraKind(k)
}

// NPCTint is a row of npctint, read: the name's key, its colour, its glow and
// its aura.
type NPCTint struct {
	Name  string // NPCClassKey of the row's name
	Color uint32 // 0xRRGGBBAA
	Glow  uint32 // 0xRRGGBBAA, 0 none
	// the aura (AuraDefault ...; source a DictID, 0 the Sword of Virtue's;
	// nodes 0 the default 8): told at layout 3 alone
	AuraKind   uint8
	AuraSource uint32
	AuraNodes  uint8
}

// NPCGlowShows reports whether a glow lights anything: an alpha of 0 or a
// black glow is none.
func NPCGlowShows(glow uint32) bool {
	return glow&0xFF != 0 && glow>>8 != 0
}

// Shows reports whether the row draws anything: its tint, its glow, or its
// aura's particles (which need no glow) or a dark aura (a black glow lights
// nothing): drawn at layout 3 alone.
func (t NPCTint) Shows() bool {
	return t.ShowsGlow() || AuraParticlesShow(t.AuraKind) || (AuraNamed(t.AuraKind) && AuraDark(t.Glow))
}

// ShowsGlow reports whether the row draws anything by its tint or its glow:
// what a layout 2 client is told.
func (t NPCTint) ShowsGlow() bool { return NPCTintChanges(t.Color) || NPCGlowShows(t.Glow) }

// NPCTintChanges reports whether a tint changes anything drawn: an alpha of 0
// or a white tint leaves the body as it is.
func NPCTintChanges(color uint32) bool {
	return color&0xFF != 0 && color>>8 != 0xFFFFFF
}

// tint32 is an npctint colour as 32 bits, whether it was stored unsigned (the
// GM panel's) or as a signed 32-bit value; anything wider is none.
func tint32(c int64) (uint32, bool) {
	if c < -(1<<31) || c > 0xFFFFFFFF {
		return 0, false
	}
	return uint32(c), true
}

// ValidNPCTint reports whether an npctint colour is a 32-bit 0xRRGGBBAA (the
// GM panel's check of a row).
func ValidNPCTint(c int64) bool {
	_, ok := tint32(c)
	return ok
}

// NPCTints is every row of npctint that tints or lights something, by
// NPCClassKey of the name (a blank one, and one whose colour and glow change
// nothing, left out), in the keys' order. Two rows of one key: the later name
// in the table's order wins.
func (db *SQLiteDB) NPCTints() ([]NPCTint, error) {
	rows, err := db.Query(`SELECT name, color, glow, aura_kind, aura_source, aura_nodes FROM npctint ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	by := make(map[string]NPCTint)
	for rows.Next() {
		var name string
		var color, glow, kind, source, nodes int64
		if err := rows.Scan(&name, &color, &glow, &kind, &source, &nodes); err != nil {
			return nil, err
		}
		k := NPCClassKey(name)
		c, ok := tint32(color)
		g, gok := tint32(glow)
		if k == "" || !ok {
			continue
		}
		if !gok {
			g = 0
		}
		t := NPCTint{Name: k, Color: c, Glow: g}
		if ValidAuraKind(kind) && ValidAuraNodes(nodes) {
			if src, ok := tint32(source); ok {
				t.AuraKind, t.AuraSource, t.AuraNodes = uint8(kind), src, uint8(nodes)
			}
		}
		if t.Shows() {
			by[k] = t
		} else {
			delete(by, k)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]NPCTint, 0, len(by))
	for _, t := range by {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
