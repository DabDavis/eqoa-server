package handlers

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// familiar.go — familiars, the handlers' half (entity/familiar.go is the
// World's; the GM Jibrish's petition 1445: Healbot, sold by the Genie).
//
// world.familiars (SetFamiliars: off unless set; the Heirloom Test Server sets
// it) turns them on. Off, a familiar's item says it is quiet here, and
// /familiar that familiars are off. A familiar is a row of table familiars by
// the item that calls it (tools/seed_familiars.py makes Healbot's: the item,
// its row, and the item in every Heirloom Outfitter's stock, which is the
// Genie's: genie.go's shopOf); a GM may add others in the panel. The item's
// Use (equip.go) is its cast: entity.FamiliarCast through the World's casting,
// its cast time and visual, no lockout, never spent, used again at will (a new
// familiar takes the place of the old). /familiar (chat.go: "/familiar",
// "#familiar", a PS2's "/t familiar ...") says what is out, "/familiar
// dismiss" sends it away.

var familiarsOn atomic.Bool

// SetFamiliars sets world.familiars.
func SetFamiliars(on bool) { familiarsOn.Store(on) }

// FamiliarsOn reports world.familiars.
func FamiliarsOn() bool { return familiarsOn.Load() }

// familiarsTTL is how long the items that call a familiar are kept before the
// table is read again (a GM's new row counts within it).
const familiarsTTL = time.Minute

// The familiars' lines (ours).
const (
	familiarQuiet   = "%s is quiet here."
	familiarOffLine = "Familiars are off on this server."
	familiarNone    = "You have no familiar out. An item that calls one does, when used."
	familiarCalls   = "You call %s."
)

// Familiars is the handlers' half of the familiars.
type Familiars struct {
	gw  *entity.World
	db  *database.SQLiteDB
	now func() time.Time

	mu    sync.Mutex
	items map[int]bool
	at    time.Time
}

// NewFamiliars is the familiars' handler over the World and the database.
func NewFamiliars(gw *entity.World, db *database.SQLiteDB) *Familiars {
	return &Familiars{gw: gw, db: db, now: time.Now}
}

// familiarsH is the handler RegisterDefaultHandlers made (a keep restart's).
var familiarsH atomic.Pointer[Familiars]

// Start makes the handler the one a keep restart gives familiars back by.
func (f *Familiars) Start() {
	if f != nil {
		familiarsH.Store(f)
	}
}

// IsItem reports an item record that calls a familiar.
func (f *Familiars) IsItem(id int) bool {
	if f == nil || f.db == nil {
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.items == nil || f.now().Sub(f.at) > familiarsTTL {
		items, err := f.db.FamiliarItems()
		if err != nil {
			log.Printf("Familiar: the table: %v", err)
			items = map[int]bool{}
		}
		f.items, f.at = items, f.now()
	}
	return f.items[id]
}

// familiarWords reports a tell's body that is /familiar's: none, or
// "dismiss" (chat.go: anything else to a "familiar" is a tell).
func familiarWords(body string) bool {
	b := strings.ToLower(strings.TrimSpace(body))
	return b == "" || b == "dismiss"
}

// familiarDefOf is a row as the World calls it.
func familiarDefOf(r database.FamiliarRow) entity.FamiliarDef {
	return entity.FamiliarDef{Item: r.PatternID, Name: strings.TrimSpace(r.Name), Model: r.Model, Size: float32(r.Size), HP: r.HP,
		Regen: r.Regen, Cast: time.Duration(r.CastMS) * time.Millisecond, CastFX: r.CastFX, Heal: r.Heal, HealBelow: r.HealBelow,
		HealEvery: time.Duration(r.HealEveryMS) * time.Millisecond, HealFX: r.HealFX}
}

// Use is the Use of an item that calls a familiar: its cast, its familiar
// beside the player as it lands. With familiars off it says so. A use within
// another cast's time is let go, as any cast's is.
func (f *Familiars) Use(sess *session.Session, p *entity.Player, item *database.ItemPatternRecord) {
	if f == nil || f.gw == nil || p == nil || item == nil {
		return
	}
	if !FamiliarsOn() {
		SendErrorMessage(sess, fmt.Sprintf(familiarQuiet, item.ItemName))
		return
	}
	row, ok, err := f.db.Familiar(item.PatternID)
	if err != nil || !ok {
		if err != nil {
			log.Printf("Familiar: %s's %q: %v", p.Name, item.ItemName, err)
		}
		return
	}
	def := familiarDefOf(row)
	if f.gw.StartCast(sess, entity.FamiliarCast(def), 0xFFFFFFFF, f.now()) {
		SendClientMessage(sess, fmt.Sprintf(familiarCalls, def.Name))
	}
}

// Command is /familiar and its words (chat.go's routing): what is out, or
// "dismiss".
func (f *Familiars) Command(sess *session.Session, p *entity.Player, args string) {
	if f == nil || f.gw == nil || p == nil {
		return
	}
	if !FamiliarsOn() {
		SendErrorMessage(sess, familiarOffLine)
		return
	}
	switch strings.ToLower(strings.TrimSpace(args)) {
	case "dismiss":
		name, ok := f.gw.DismissFamiliar(sess)
		if !ok {
			SendErrorMessage(sess, familiarNone)
			return
		}
		SendClientMessage(sess, fmt.Sprintf("You dismiss %s.", name))
	default:
		name, item, ok := f.gw.FamiliarOf(sess)
		if !ok {
			SendClientMessage(sess, familiarNone)
			return
		}
		line := fmt.Sprintf("Your familiar is %s.", name)
		if row, found, err := f.db.Familiar(item); err == nil && found {
			line += " " + entity.FamiliarLine(familiarDefOf(row))
		}
		SendClientMessage(sess, line+" /familiar dismiss sends it away.")
	}
}

// Restore gives a player carried through a keep restart its familiar back
// (entity/keepmerc.go), by its item's row now.
func (f *Familiars) Restore(sess *session.Session, k entity.KeptFamiliar, now time.Time) {
	if f == nil || f.db == nil || !FamiliarsOn() {
		return
	}
	row, ok, err := f.db.Familiar(k.Item)
	if err != nil || !ok {
		return
	}
	f.gw.RestoreFamiliar(sess, familiarDefOf(row), k.HPPct, now)
}
