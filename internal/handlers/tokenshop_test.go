package handlers

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// The token shops (tokenshop.go): the Toy Shoppe as tools/toy_shoppe_20261116.py
// writes it, on the portal rig (Anna at (5000, 50, 5000) in world 0) and a
// database holding Toymaker Tinsel 3 off her, the Holly Token and three toys
// at the tool's prices. The records are made up but for their names.

type toyRig struct {
	*portalRig
	db    *database.SQLiteDB
	inv   *database.InventoryStore
	shop  *entity.NPC
	token database.ItemPatternRecord
	ids   []int // the toys, as listed
}

var toyWares = []struct {
	name  string
	price int
	sort  int
	says  string
}{
	{"Santa's Elf", 18, 1, "Become one of Santa's elves."},
	{"Barrel of Monkeys", 36, 2, ""},
	{"Automatic Snowball Launcher", 54, 3, ""},
}

const toyShop = "Toymaker Tinsel"

func newToyRig(t *testing.T) *toyRig {
	t.Helper()
	r := &toyRig{portalRig: newPortalRig(t)}
	db, err := database.NewSQLite(filepath.Join(t.TempDir(), "toys.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	r.db, r.inv = db, database.NewInventoryStore(db)
	chars := database.NewSQLiteCharacterStore(db)
	rec := &database.CharacterRecord{CharName: "Anna", AccountID: 1, Level: 60}
	if err := chars.Create(rec); err != nil {
		t.Fatal(err)
	}
	r.pa.CharServerID, r.pa.Level = rec.ServerID, 60
	r.anna.SetInventoryList(newInvList(nil))
	if _, err := db.Exec(`INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race, zone)
		VALUES (990101, ?, 0, 5003, 50, 5000, 0, 2845600533, 1.0, 30, 128, '3', 0)`, toyShop); err != nil {
		t.Fatal(err)
	}
	add := func(name string, stack, lore int) int {
		res, err := db.Exec(`INSERT INTO itempattern
			(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
			 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
			 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
			VALUES (0, 0, 209763529, 0, -1, 0, 1, 1, 0, 0, 0, 0, 1, ?, 0, 0, 32767, 32767, 2, ?, 0, 0, ?, ?, 0, 0)`,
			stack, lore, name, "The record's own words for "+name+".")
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return int(id)
	}
	tok := add(DefaultEnchanterToken, 999, 0)
	pat, err := database.NewItemPatternStore(db).GetItemPattern(tok)
	if err != nil || pat == nil {
		t.Fatalf("the token: %v", err)
	}
	r.token = *pat
	// written in another order than listed: the list goes by sort
	for i := len(toyWares) - 1; i >= 0; i-- {
		w := toyWares[i]
		id := add(w.name, 1, 1)
		r.ids = append([]int{id}, r.ids...)
		if _, err := db.Exec(`INSERT INTO tokenshop (shop, patternid, price, token, says, sort, source) VALUES (?, ?, ?, '', ?, ?, 'toy-shoppe')`,
			toyShop, id, w.price, w.says, w.sort); err != nil {
			t.Fatal(err)
		}
	}
	r.gw.LoadNPCs(database.NewNPCStore(db))
	for _, n := range r.gw.NPCs {
		if n.Name == toyShop {
			r.shop = n
		}
	}
	if r.shop == nil {
		t.Fatal("no shop")
	}
	r.ih.tokenShop = NewTokenShop(r.gw, db, r.inv)
	if r.ih.tokenShop == nil {
		t.Fatal("no shop's talk")
	}
	return r
}

// carry gives Anna n Holly Tokens.
func (r *toyRig) carry(n int) {
	r.t.Helper()
	if got, err := GiveItemNow(r.anna, r.gw, r.inv, r.token, n); err != nil || got != n {
		r.t.Fatalf("giving %d tokens: %d, %v", n, got, err)
	}
	r.drain()
}

func (r *toyRig) count(pattern int) int {
	r.t.Helper()
	var n int
	if err := r.db.QueryRow(`SELECT COALESCE(sum(stackleft), 0) FROM charinventory WHERE serverid = ? AND patternid = ?`, r.pa.CharServerID, pattern).Scan(&n); err != nil {
		r.t.Fatal(err)
	}
	return n
}

func (r *toyRig) visit() ([]string, uint32) {
	r.t.Helper()
	r.drain()
	r.talk(r.anna, r.shop)
	got := r.msgs(r.anna)
	return got, heroesBoxOf(got)
}

// choose answers a box: what Anna was sent but the line showing a ware, and the next box.
func (r *toyRig) choose(box uint32, option byte) ([]string, uint32) {
	r.t.Helper()
	r.pick(r.anna, box, option)
	var got []string
	for _, m := range r.msgs(r.anna) {
		if strings.HasPrefix(m, "chat:"+toyShop+": ") && strings.Contains(m, "Holly Token") {
			continue // the ware shown (show)
		}
		got = append(got, m)
	}
	return got, heroesBoxOf(got)
}

const toyList = "Santa's Elf: 18|Barrel of Monkeys: 36|Automatic Snowball Launcher: 54"

func TestTheToyShoppeSellsEachToyForHollyTokens(t *testing.T) {
	for i, w := range toyWares {
		t.Run(w.name, func(t *testing.T) {
			r := newToyRig(t)
			r.carry(w.price + 5)
			got, box := r.visit()
			r.want("Anna", got,
				fmt.Sprintf("box:%s: Welcome, Anna! Everything here is paid in Holly Tokens, and you carry %d.", toyShop, w.price+5),
				fmt.Sprintf("ask#%d:%s: 'You carry %d Holly Tokens. What will you have?'|%s", box, toyShop, w.price+5, toyList))
			if box&0xFF000000 != 0x66000000 || !IsTokenShopBox(box) {
				t.Fatalf("the box %#x is not under the mark 0x66000000", box)
			}
			got, buy := r.choose(box, byte(i))
			about := w.says
			if about == "" {
				about = "The record's own words for " + w.name + "."
			}
			r.want("Anna", got, fmt.Sprintf("box:%s: %s", toyShop, about),
				fmt.Sprintf("ask#%d:%s: '%s, for %d Holly Tokens?'|Yes, I will take it|No, not now", buy, toyShop, theItem(w.name), w.price))
			if r.count(r.ids[i]) != 0 || r.count(r.token.PatternID) != w.price+5 {
				t.Fatal("asking gave or took something")
			}
			got, _ = r.choose(buy, 0)
			if !hasMsg(got, fmt.Sprintf("box:%s: %s is yours! You have 5 Holly Tokens left.", toyShop, theItem(w.name))) {
				t.Errorf("the purchase said %q", got)
			}
			if r.count(r.ids[i]) != 1 || r.count(r.token.PatternID) != 5 {
				t.Fatalf("after: %d held, %d tokens", r.count(r.ids[i]), r.count(r.token.PatternID))
			}
			if l := sessionInvList(r.anna); l.held(r.ids[i]) != 1 || l.held(r.token.PatternID) != 5 {
				t.Errorf("the session's list: %d held, %d tokens", l.held(r.ids[i]), l.held(r.token.PatternID))
			}
			var p database.TokenShopPurchase
			if err := r.db.QueryRow(`SELECT serverid, charname, shop, patternid, item, price, token FROM tokenshop_purchases`).Scan(
				&p.ServerID, &p.CharName, &p.Shop, &p.PatternID, &p.Item, &p.Price, &p.Token); err != nil {
				t.Fatal(err)
			}
			if p.ServerID != r.pa.CharServerID || p.CharName != "Anna" || p.Shop != toyShop || p.PatternID != r.ids[i] ||
				p.Item != w.name || p.Price != w.price || p.Token != DefaultEnchanterToken {
				t.Errorf("the log: %+v", p)
			}
			// the old box again: nothing
			if got, _ := r.choose(buy, 0); len(got) != 0 || r.count(r.token.PatternID) != 5 {
				t.Errorf("the old box answered again: %q", got)
			}
			// LORE: a second is refused, nothing taken
			r.carry(100)
			_, box = r.visit()
			got, next := r.choose(box, byte(i))
			r.want("Anna", got, fmt.Sprintf("box:%s: You can hold only one %s.", toyShop, w.name))
			if next != 0 || r.count(r.token.PatternID) != 105 {
				t.Errorf("a second: box %#x, %d tokens", next, r.count(r.token.PatternID))
			}
		})
	}
}

// Short of the price by one: told how far short, no box asked, nothing taken;
// the tokens spent between the question and the Yes are counted again.
func TestTheToyShoppeRefusesTooFewTokens(t *testing.T) {
	r := newToyRig(t)
	r.carry(35)
	_, box := r.visit()
	got, next := r.choose(box, 1)
	r.want("Anna", got, fmt.Sprintf("box:%s: The Barrel of Monkeys costs 36 Holly Tokens, and you carry 35: 1 more to earn.", toyShop))
	if next != 0 || r.count(r.ids[1]) != 0 || r.count(r.token.PatternID) != 35 {
		t.Errorf("box %#x, %d held, %d tokens", next, r.count(r.ids[1]), r.count(r.token.PatternID))
	}
	_, box = r.visit()
	_, buy := r.choose(box, 0) // Santa's Elf, 18
	if buy == 0 {
		t.Fatal("no Yes asked for the elf")
	}
	held, _, _ := packStacksOf(r.inv, r.pa, r.token.PatternID)
	takeStacks(r.inv, r.anna, r.pa, "test", held, 20) // 15 left
	r.drain()
	got, _ = r.choose(buy, 0)
	r.want("Anna", got, fmt.Sprintf("box:%s: Santa's Elf costs 18 Holly Tokens, and you carry 15: 3 more to earn.", toyShop))
	if r.count(r.ids[0]) != 0 || r.count(r.token.PatternID) != 15 {
		t.Errorf("%d held, %d tokens", r.count(r.ids[0]), r.count(r.token.PatternID))
	}
	var n int
	if err := r.db.QueryRow(`SELECT count(*) FROM tokenshop_purchases`).Scan(&n); err != nil || n != 0 {
		t.Errorf("%d logged (%v)", n, err)
	}
}

// A price a GM changed between the list and the Yes asks again; a row priced 0
// is off the list; an NPC of another name is no shop, and a GM's new shop is
// one within the names' TTL.
func TestTheToyShoppeReadsItsStockAgain(t *testing.T) {
	r := newToyRig(t)
	r.carry(100)
	_, box := r.visit()
	_, buy := r.choose(box, 0)
	if _, err := r.db.Exec(`UPDATE tokenshop SET price = 20 WHERE patternid = ?`, r.ids[0]); err != nil {
		t.Fatal(err)
	}
	got, _ := r.choose(buy, 0)
	r.want("Anna", got, fmt.Sprintf("box:%s: My prices have changed since I showed you. Ask me again.", toyShop))
	if r.count(r.token.PatternID) != 100 {
		t.Errorf("%d tokens", r.count(r.token.PatternID))
	}
	if _, err := r.db.Exec(`UPDATE tokenshop SET price = 0 WHERE patternid = ?`, r.ids[2]); err != nil {
		t.Fatal(err)
	}
	got, box = r.visit()
	if !hasMsg(got, fmt.Sprintf("ask#%d:%s: 'You carry 100 Holly Tokens. What will you have?'|Santa's Elf: 20|Barrel of Monkeys: 36", box, toyShop)) {
		t.Errorf("the list after: %q", got)
	}
	ts := r.ih.tokenShop
	if ts.IsShop("Weapon Enchanter") || !ts.IsShop("  toymaker tinsel ") {
		t.Error("the shops by name")
	}
}

func TestTheItem(t *testing.T) {
	for name, want := range map[string]string{"Barrel of Monkeys": "The Barrel of Monkeys", "Santa's Elf": "Santa's Elf",
		"The Big Book": "The Big Book"} {
		if got := theItem(name); got != want {
			t.Errorf("%q: %q", name, got)
		}
	}
}

// The Weapon Enchanter's price in words: "18 Holly Tokens", not "18 Hollys
// Token"; a reagent's "of" keeps its word.
func TestReagentWordsPlural(t *testing.T) {
	for name, want := range map[string]string{"Holly Token": "18 Holly Tokens", "Unit of Blood": "18 Units of Blood",
		"Cracked Ruby Gem": "18 Cracked Ruby Gems", "Glass": "18 Glass", "Bone Chips": "18 Bone Chips"} {
		if got := reagentWords(name, 18); got != want {
			t.Errorf("%q: %q, want %q", name, got, want)
		}
	}
	if got := reagentWords("Holly Token", 1); got != "1 Holly Token" {
		t.Errorf("one: %q", got)
	}
}
