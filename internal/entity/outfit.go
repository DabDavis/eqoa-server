package entity

import (
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// outfit.go — what an item's illusion dresses its bearer in (table itemoutfit,
// database/tokenshop.go; the owner, 2026-11-16: "Santa's Elf illusion the
// gnome with red/green clothing"). An item whose use shows a body (its
// itemeffect row: entity/potion.go) and has outfit rows is a toy illusion:
// while the body it gave is the one shown (Player.ownBody's: no GM's sprite,
// no later form over it), the bearer's own armour, robe and their colours are
// not sent but the outfit's are, to every client, the bearer's own too
// (CLIENT dresses its own sprite off its channel):
//
//   - the entity channel's armour sets (bytes 93-98) and robe (109), and in
//     the tints (99-104, 110) the disc tint nearest each colour (nearestTint):
//     all a PS2 draws, one of fifteen tints a slot, no trim;
//   - a client of the live look (vanilla) the colours themselves, the trims'
//     too (LookDyes: the outfit's in place of the bearer's dyes and pieces'
//     colours; its held items keep theirs).
//
// The held items, the cape, hair and face stay the bearer's, and so does the
// character list (an illusion is never on it). Used again while its body is
// on, the item takes the illusion off (EndItemForm): a PS2 has no other way to
// click one off, and none of its cooldown is spent.

// Outfit is an item's outfit: the armour sets (chest .. helm), the robe's
// style (0xFF none) and the colours by dye slot (0 none).
type Outfit struct {
	Sets [6]byte
	Robe byte
	Dyes Dyes
}

var itemOutfits atomic.Pointer[map[int]*Outfit]

// LoadItemOutfits reads table itemoutfit into the world: at the start and
// after a GM's edit. A client sees a change when its bearer's channel is next
// sent.
func (w *World) LoadItemOutfits(db *database.SQLiteDB) (int, error) {
	rows, err := db.ItemOutfits()
	if err != nil {
		return 0, fmt.Errorf("itemoutfit: %w", err)
	}
	SetItemOutfits(rows)
	return len(rows), nil
}

// SetItemOutfits keeps the outfits, by item.
func SetItemOutfits(rows map[int]database.ItemOutfitRow) {
	m := make(map[int]*Outfit, len(rows))
	for pid, r := range rows {
		m[pid] = &Outfit{Sets: r.Sets, Robe: r.Robe, Dyes: Dyes(r.Colors)}
	}
	itemOutfits.Store(&m)
}

// ItemOutfitOf is the outfit of an item, or nil.
func ItemOutfitOf(patternID int) *Outfit {
	m := itemOutfits.Load()
	if m == nil {
		return nil
	}
	return (*m)[patternID]
}

// shownOutfit is the outfit of the item whose illusion is the body shown, or
// nil (ownBody's order: a GM's sprite first, then the newest form).
func (p *Player) shownOutfit() *Outfit {
	if p.GMForm != 0 {
		return nil
	}
	for i := len(p.Effects) - 1; i >= 0; i-- {
		if e := p.Effects[i]; p.shownForm(e) {
			if pid, ok := potionOfEffect(e.SpellID); ok {
				return ItemOutfitOf(pid)
			}
			return nil
		}
	}
	return nil
}

// putOutfit dresses a player's channel buffer (the disc's 162 bytes) in o.
func putOutfit(o *Outfit, buf []byte) {
	if o == nil || len(buf) < 111 {
		return
	}
	for i := 0; i < 6; i++ {
		buf[ChannelArmor+i] = o.Sets[i]
		buf[99+i] = ArmorTint(o.Dyes[database.DyeChest+i])
	}
	buf[109], buf[110] = o.Robe, 0
	if o.Robe != 0xFF && o.Dyes[database.DyeRobe] != 0 {
		buf[110] = nearestTint(o.Dyes[database.DyeRobe])
	}
}

// outfitDyes is what the live look draws a bearer of o in: o's colours, with
// the bearer's held items' (d, LookDyes' own) kept.
func outfitDyes(o *Outfit, d *Dyes) *Dyes {
	out := o.Dyes
	for _, s := range []int{database.DyePrimary, database.DyeSecondary, database.DyeShield, database.DyeCape} {
		out[s] = 0
		if d != nil {
			out[s] = d[s]
		}
	}
	return &out
}

// Words of a toy illusion taken off by its item (ours).
const itemFormOff = "You put the %s away and are yourself again."

// EndItemForm takes off sess's player's illusion of the item (a toy
// illusion's: one with an outfit) when it bears it, told to it, and reports
// whether it did; an item with no outfit, or one whose illusion is not on,
// does nothing.
func (w *World) EndItemForm(sess *session.Session, patternID int, itemName string, now time.Time) bool {
	if ItemOutfitOf(patternID) == nil {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return false
	}
	id := PotionEffectID(patternID)
	for _, e := range p.Effects {
		if e.SpellID == id && e.Form != 0 && now.Before(e.Until) {
			if !w.removeEffectLocked(p, id) {
				return false
			}
			log.Printf("Outfit: %s put %q away: its illusion is off", p.Name, itemName)
			w.sendColoredLocked(sess, fmt.Sprintf(itemFormOff, itemName), outfitOffRGB)
			return true
		}
	}
	return false
}

var outfitOffRGB = [3]byte{255, 255, 255}
