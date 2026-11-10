package handlers

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// The Testing Heroes Vendor (heroes.go; petition 1471), on the portal rig
// (Anna at (5000, 50, 5000) in world 0, a row of characters of her own) and a
// database holding him 3 off her, the faction as the tool writes it and five
// wares at the petition's costs. The records are made up.

type heroesRig struct {
	*portalRig
	db     *database.SQLiteDB
	inv    *database.InventoryStore
	chars  *database.SQLiteCharacterStore
	vendor *entity.NPC
	ids    []int // the five wares, cheapest first
}

var heroesWares = []struct {
	name       string
	cost       int
	slot, lore int
	says       string
}{
	{"Elixir of the Giant Gnome", 25, -1, 0, "Five draughts, each a giant gnome for thirty minutes."},
	{"Tome of Heroic Training", 100, -1, 0, ""},
	{"Weakened Breastplate of the Anointed", 250, 5, 1, ""},
	{"Breastplate of the Anointed", 500, 5, 1, ""},
	{"Ascended Breastplate of the Anointed", 1000, 5, 1, ""},
}

func newHeroesRig(t *testing.T) *heroesRig {
	t.Helper()
	r := &heroesRig{portalRig: newPortalRig(t)}
	SetTestingHeroes(true)
	t.Cleanup(func() { SetTestingHeroes(false) })
	db, err := database.NewSQLite(filepath.Join(t.TempDir(), "heroes.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	r.db, r.inv, r.chars = db, database.NewInventoryStore(db), database.NewSQLiteCharacterStore(db)
	// Anna's row, under her session's character id
	rec := &database.CharacterRecord{CharName: "Anna", AccountID: 1, Level: 60}
	if err := r.chars.Create(rec); err != nil {
		t.Fatal(err)
	}
	r.pa.CharServerID = rec.ServerID
	r.pa.Level = 60
	r.anna.SetInventoryList(newInvList(nil))
	if _, err := db.Exec(`INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race, zone)
		VALUES (990001, ?, 0, 5003, 50, 5000, 0, 1893243078, 1.0, 60, 128, '0', 0)`, HeroesVendorName); err != nil {
		t.Fatal(err)
	}
	for _, w := range heroesWares {
		res, err := db.Exec(`INSERT INTO itempattern
			(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
			 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
			 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
			VALUES (0, 0, 209763529, 0, ?, 0, 0, 1, 0, 0, 0, 0, 1, 1, 0, 0, 0, 0, 2, ?, 0, 0, ?, ?, 0, 0)`,
			w.slot, w.lore, w.name, "The record's own words for "+w.name+".")
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		r.ids = append(r.ids, int(id))
		if _, err := db.Exec(`INSERT INTO heroes_stock (patternid, cost, says, source) VALUES (?, ?, ?, 'testing-heroes')`, id, w.cost, w.says); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO itemeffect (patternid, line, charges) VALUES (?, 'Self, Body Size 4, 30min', 5)`, r.ids[0]); err != nil {
		t.Fatal(err)
	}
	r.gw.LoadNPCs(database.NewNPCStore(db))
	for _, n := range r.gw.NPCs {
		if n.Name == HeroesVendorName {
			r.vendor = n
		}
	}
	if r.vendor == nil {
		t.Fatal("no vendor")
	}
	r.gw.SetFactionDefaults([]database.FactionDefault{{Name: "Citizens of Freeport", Standing: 1}, {Name: entity.HeroesFaction, Standing: entity.FactionMin}})
	r.gw.SetTestingHeroes(true)
	r.ih.heroes = NewHeroes(r.gw, db, r.inv, r.chars)
	if r.ih.heroes == nil {
		t.Fatal("no vendor's talk")
	}
	return r
}

// stand sets Anna's standing with the Testing Heroes to n above the floor.
func (r *heroesRig) stand(n int) {
	r.pa.Factions = map[string]int{entity.HeroesFaction: entity.FactionMin + n}
}

func (r *heroesRig) standing() int {
	_, s, _ := r.gw.HeroesBalance(r.anna)
	return s
}

// visit is Anna's talk to the vendor: what she was sent, and the box of the
// question among it (0 none).
func (r *heroesRig) visit() ([]string, uint32) {
	r.t.Helper()
	r.drain()
	r.talk(r.anna, r.vendor)
	got := r.msgs(r.anna)
	return got, heroesBoxOf(got)
}

// choose is Anna's answer to a box: what she was sent, and the next box.
func (r *heroesRig) choose(box uint32, option byte) ([]string, uint32) {
	r.t.Helper()
	r.pick(r.anna, box, option)
	got := r.msgs(r.anna)
	return got, heroesBoxOf(got)
}

// heroesBoxOf is the id of the last question among msgs ("ask#id:..."), 0 none.
func heroesBoxOf(msgs []string) uint32 {
	var box uint32
	for _, m := range msgs {
		if rest, ok := strings.CutPrefix(m, "ask#"); ok {
			id, _, _ := strings.Cut(rest, ":")
			n, _ := strconv.ParseUint(id, 10, 32)
			box = uint32(n)
		}
	}
	return box
}

// held is how many of the record Anna's row of charinventory holds.
func (r *heroesRig) held(pattern int) int {
	r.t.Helper()
	var n int
	if err := r.db.QueryRow(`SELECT count(*) FROM charinventory WHERE serverid = ? AND patternid = ?`, r.pa.CharServerID, pattern).Scan(&n); err != nil {
		r.t.Fatal(err)
	}
	return n
}

func (r *heroesRig) purchases() int {
	r.t.Helper()
	var n int
	if err := r.db.QueryRow(`SELECT count(*) FROM heroes_purchases`).Scan(&n); err != nil {
		r.t.Fatal(err)
	}
	return n
}

const heroesList = "Elixir of the Giant Gnome: 25|Tome of Heroic Training: 100|Weakened Breastplate of the Anointed: 250|" +
	"Breastplate of the Anointed: 500|Ascended Breastplate of the Anointed: 1000"

// A talk greets with the standing and what it leaves to spend, then lists the
// five wares with their costs, cheapest first, under his own box mark; a
// newcomer has nothing to spend and every pick says how far short it is.
func TestTheHeroesVendorListsHisStock(t *testing.T) {
	r := newHeroesRig(t)
	got, box := r.visit()
	r.want("Anna", got,
		"box:Testing Heroes Vendor: Well met, Anna. Every kill that gives you experience earns 1 faction with the Testing Heroes, a raid boss 5. You stand at -2000: 0 above the bottom, yours to spend.",
		fmt.Sprintf("ask#%d:Testing Heroes Vendor: 'You have 0 to spend. What will you have?'|%s", box, heroesList))
	if !IsHeroesBox(box) || box&0xFF000000 != 0x64000000 || IsHeroesBox(0x63000001) {
		t.Errorf("the box %#x is not under his mark 0x64000000", box)
	}
	got, next := r.choose(box, 0)
	r.want("Anna", got, "box:Testing Heroes Vendor: The Elixir of the Giant Gnome costs 25 faction and you have 0 to spend: 25 more to earn.")
	if next != 0 || r.held(r.ids[0]) != 0 || r.standing() != entity.FactionMin {
		t.Errorf("a newcomer: a box %#x, %d held, standing %d", next, r.held(r.ids[0]), r.standing())
	}
	// 24 of 25: still short, by one
	r.stand(24)
	_, box = r.visit()
	got, _ = r.choose(box, 0)
	r.want("Anna", got, "box:Testing Heroes Vendor: The Elixir of the Giant Gnome costs 25 faction and you have 24 to spend: 1 more to earn.")
}

// Each of the five is bought with standing enough: the pick says what it is
// (the stock row's words, else the record's own) and asks; Yes takes the cost
// off the standing (told as a faction's change is), gives the item (the
// elixir with its five charges), keeps the standing at once and logs the
// purchase. A standing of exactly the cost buys, to the floor.
func TestTheHeroesVendorSellsEachWare(t *testing.T) {
	for i, w := range heroesWares {
		t.Run(w.name, func(t *testing.T) {
			r := newHeroesRig(t)
			r.stand(w.cost)
			got, box := r.visit()
			if !hasMsg(got, fmt.Sprintf("ask#%d:Testing Heroes Vendor: 'You have %d to spend. What will you have?'|%s", box, w.cost, heroesList)) {
				t.Fatalf("the list: %q", got)
			}
			got, buy := r.choose(box, byte(i))
			about := w.says
			if about == "" {
				about = "The record's own words for " + w.name + "."
			}
			r.want("Anna", got, "box:Testing Heroes Vendor: "+about,
				fmt.Sprintf("ask#%d:Testing Heroes Vendor: 'The %s, for %d of your %d?'|Yes, I will take it|No, not now", buy, w.name, w.cost, w.cost))
			if r.held(r.ids[i]) != 0 || r.standing() != entity.FactionMin+w.cost {
				t.Fatal("asking gave or took something")
			}
			got, _ = r.choose(buy, 0)
			if !hasMsg(got, fmt.Sprintf("chat:-2000 faction with Testing Heroes (-%d)", w.cost)) ||
				!hasMsg(got, fmt.Sprintf("box:Testing Heroes Vendor: The %s is yours. You have 0 left to spend.", w.name)) ||
				!hasMsg(got, "op:003B") {
				t.Errorf("the purchase said %q", got)
			}
			if r.held(r.ids[i]) != 1 || r.standing() != entity.FactionMin {
				t.Fatalf("after the purchase: %d held, standing %d", r.held(r.ids[i]), r.standing())
			}
			if l := sessionInvList(r.anna); l.held(r.ids[i]) != 1 {
				t.Errorf("the session's list holds %d", l.held(r.ids[i]))
			}
			var kept int
			if err := r.db.QueryRow(`SELECT standing FROM charfactions WHERE serverid = ? AND faction = ?`, r.pa.CharServerID, entity.HeroesFaction).Scan(&kept); err != nil || kept != entity.FactionMin {
				t.Errorf("the standing kept at once: %d (%v), want %d", kept, err, entity.FactionMin)
			}
			var p database.HeroesPurchase
			if err := r.db.QueryRow(`SELECT serverid, charname, patternid, item, cost, standing_before, standing_after, vendor FROM heroes_purchases`).Scan(
				&p.ServerID, &p.CharName, &p.PatternID, &p.Item, &p.Cost, &p.StandingBefore, &p.StandingAfter, &p.Vendor); err != nil {
				t.Fatal(err)
			}
			if p.ServerID != r.pa.CharServerID || p.CharName != "Anna" || p.PatternID != r.ids[i] || p.Item != w.name || p.Cost != w.cost ||
				p.StandingBefore != entity.FactionMin+w.cost || p.StandingAfter != entity.FactionMin || p.Vendor != HeroesVendorName {
				t.Errorf("the log: %+v", p)
			}
			if i == 0 { // the elixir comes with its five charges
				var charges int
				if err := r.db.QueryRow(`SELECT remaincharge FROM charinventory WHERE serverid = ? AND patternid = ?`, r.pa.CharServerID, r.ids[0]).Scan(&charges); err != nil || charges != 5 {
					t.Errorf("the elixir's charges: %d (%v), want 5", charges, err)
				}
			}
			// the old box again: nothing
			if got, _ := r.choose(buy, 0); len(got) != 0 || r.held(r.ids[i]) != 1 || r.purchases() != 1 {
				t.Errorf("the old box answered again: %q, %d held, %d logged", got, r.held(r.ids[i]), r.purchases())
			}
		})
	}
}

// One short of each cost buys nothing: the pick says how far short, no box
// asks, and nothing is taken, given or logged.
func TestTheHeroesVendorRefusesAStandingTooLow(t *testing.T) {
	r := newHeroesRig(t)
	for i, w := range heroesWares {
		r.stand(w.cost - 1)
		_, box := r.visit()
		got, next := r.choose(box, byte(i))
		r.want("Anna", got, fmt.Sprintf("box:Testing Heroes Vendor: The %s costs %d faction and you have %d to spend: 1 more to earn.", w.name, w.cost, w.cost-1))
		if next != 0 || r.held(r.ids[i]) != 0 || r.standing() != entity.FactionMin+w.cost-1 {
			t.Errorf("%s: a box %#x, %d held, standing %d", w.name, next, r.held(r.ids[i]), r.standing())
		}
	}
	if r.purchases() != 0 {
		t.Errorf("%d purchases logged", r.purchases())
	}
}

// A standing spent between the question and the Yes (the same character's
// other box could not, but a GM's #faction can) is checked again at the Yes.
func TestTheHeroesVendorChecksAgainAtTheYes(t *testing.T) {
	r := newHeroesRig(t)
	r.stand(300)
	_, box := r.visit()
	_, buy := r.choose(box, 2)
	r.stand(249)
	got, _ := r.choose(buy, 0)
	r.want("Anna", got, "box:Testing Heroes Vendor: The Weakened Breastplate of the Anointed costs 250 faction and you have 249 to spend: 1 more to earn.")
	if r.held(r.ids[2]) != 0 || r.standing() != entity.FactionMin+249 || r.purchases() != 0 {
		t.Errorf("%d held, standing %d, %d logged", r.held(r.ids[2]), r.standing(), r.purchases())
	}

	// a price a GM changed since the list: asked again, nothing taken
	r.stand(300)
	_, box = r.visit()
	_, buy = r.choose(box, 2)
	if _, err := r.db.Exec(`UPDATE heroes_stock SET cost = 200 WHERE patternid = ?`, r.ids[2]); err != nil {
		t.Fatal(err)
	}
	got, _ = r.choose(buy, 0)
	r.want("Anna", got, "box:Testing Heroes Vendor: My prices have changed since I showed you. Ask me again.")
	if r.held(r.ids[2]) != 0 || r.standing() != entity.FactionMin+300 {
		t.Errorf("%d held, standing %d", r.held(r.ids[2]), r.standing())
	}
	// and the new price sells
	_, box = r.visit()
	_, buy = r.choose(box, 2) // 200 still sorts third
	got, _ = r.choose(buy, 0)
	if !hasMsg(got, "box:Testing Heroes Vendor: The Weakened Breastplate of the Anointed is yours. You have 100 left to spend.") {
		t.Errorf("at the new price: %q", got)
	}
}

// No, a close, a walk away and a second purchase of a Lore item.
func TestTheHeroesVendorsOtherAnswers(t *testing.T) {
	r := newHeroesRig(t)
	r.stand(2000)
	_, box := r.visit()
	_, buy := r.choose(box, 3)
	if got, next := r.choose(buy, 1); len(got) != 0 || next != 0 || r.held(r.ids[3]) != 0 || r.standing() != 0 {
		t.Errorf("No: %q, %d held, standing %d", got, r.held(r.ids[3]), r.standing())
	}
	// a close (255) is not the last word: the box still answers
	_, box = r.visit()
	_, buy = r.choose(box, 3)
	if got, _ := r.choose(buy, 255); len(got) != 0 {
		t.Errorf("a close said %q", got)
	}
	if got, _ := r.choose(buy, 0); !hasMsg(got, "box:Testing Heroes Vendor: The Breastplate of the Anointed is yours. You have 1500 left to spend.") {
		t.Errorf("Yes after a close: %q", got)
	}
	// a Lore item held already: refused at the pick, nothing taken
	_, box = r.visit()
	got, next := r.choose(box, 3)
	r.want("Anna", got, "box:Testing Heroes Vendor: You can hold only one Breastplate of the Anointed.")
	if next != 0 || r.held(r.ids[3]) != 1 || r.standing() != -500 {
		t.Errorf("a second Lore item: box %#x, %d held, standing %d", next, r.held(r.ids[3]), r.standing())
	}
	// the Weakened one is a record of its own: sold beside it
	_, box = r.visit()
	_, buy = r.choose(box, 2)
	if got, _ := r.choose(buy, 0); !hasMsg(got, "box:Testing Heroes Vendor: The Weakened Breastplate of the Anointed is yours. You have 1250 left to spend.") {
		t.Errorf("the Weakened beside the Breastplate: %q", got)
	}
	// walked away before the Yes
	_, box = r.visit()
	_, buy = r.choose(box, 0)
	r.pa.X += 200
	got, _ = r.choose(buy, 0)
	r.want("Anna", got, "box:Testing Heroes Vendor: Come back to me to trade.")
	if r.held(r.ids[0]) != 0 || r.standing() != -750 {
		t.Errorf("from afar: %d held, standing %d", r.held(r.ids[0]), r.standing())
	}
	if r.purchases() != 2 {
		t.Errorf("%d purchases logged, want 2", r.purchases())
	}
}

// A full pack is told at the pick and nothing is taken; an item that could
// not be handed over after the payment gives the payment back.
func TestTheHeroesVendorTakesNothingForNothing(t *testing.T) {
	r := newHeroesRig(t)
	r.stand(100)
	r.anna.SetInventoryList(newInvListCap([]database.CharInventoryEntry{{ItemID: 1, StackLeft: 1, Pattern: database.ItemPatternRecord{PatternID: 7, MaxStack: 1}}}, 1))
	_, box := r.visit()
	got, next := r.choose(box, 0)
	r.want("Anna", got, "box:Testing Heroes Vendor: Your pack is too full for the Elixir of the Giant Gnome. Make room and come back to me.")
	if next != 0 || r.standing() != entity.FactionMin+100 {
		t.Errorf("a full pack: box %#x, standing %d", next, r.standing())
	}

	r.anna.SetInventoryList(newInvList(nil))
	r.ih.heroes.give = func(*session.Session, database.ItemPatternRecord) (int, error) { return 0, errors.New("the database is away") }
	_, box = r.visit()
	_, buy := r.choose(box, 0)
	got, _ = r.choose(buy, 0)
	if !hasMsg(got, "chat:-2000 faction with Testing Heroes (-25)*") && !hasMsg(got, "chat:-1925 faction with Testing Heroes (-25)") {
		t.Errorf("the payment was not told: %q", got)
	}
	if !hasMsg(got, "chat:-1900 faction with Testing Heroes (+25)") || !hasMsg(got, "box:Testing Heroes Vendor: I could not hand it over: you keep your faction. Try again.") {
		t.Errorf("the refund: %q", got)
	}
	if r.standing() != entity.FactionMin+100 || r.purchases() != 0 {
		t.Errorf("after a failed give: standing %d, %d logged", r.standing(), r.purchases())
	}
	var kept int
	if err := r.db.QueryRow(`SELECT standing FROM charfactions WHERE serverid = ? AND faction = ?`, r.pa.CharServerID, entity.HeroesFaction).Scan(&kept); err != nil || kept != entity.FactionMin+100 {
		t.Errorf("the standing kept after the refund: %d (%v)", kept, err)
	}
}

// Off (world.testing_heroes unset), an NPC of his name is no vendor: the talk
// is not his, and an old box of his does nothing. On without the faction's
// row, or without stock, he says so and sells nothing.
func TestTheHeroesVendorIsOffUnlessSet(t *testing.T) {
	r := newHeroesRig(t)
	r.stand(100)
	_, box := r.visit()
	SetTestingHeroes(false)
	if r.ih.heroes.Talk(r.anna, r.pa, r.vendor) {
		t.Error("off: the talk was his")
	}
	if got, _ := r.choose(box, 0); len(got) != 0 {
		t.Errorf("off: his old box said %q", got)
	}
	var none *Heroes
	if none.Talk(r.anna, r.pa, r.vendor) || !none.Answer(r.anna, r.pa, heroesBoxMark|1, 0) || none.Answer(r.anna, r.pa, 0x63000001, 0) {
		t.Error("no vendor's talk: a talk was his, or a box of his was not, or another's was")
	}
	SetTestingHeroes(true)

	// another NPC is not him
	other := *r.vendor
	other.Name = "Merchant Kari"
	if r.ih.heroes.Talk(r.anna, r.pa, &other) {
		t.Error("another NPC's talk was his")
	}

	// no stock
	if _, err := r.db.Exec(`UPDATE heroes_stock SET cost = 0`); err != nil {
		t.Fatal(err)
	}
	got, box := r.visit()
	if box != 0 || !hasMsg(got, "box:Testing Heroes Vendor: I have nothing to sell today.") {
		t.Errorf("no stock: %q", got)
	}
	// the faction's row gone (the binary in before the tool): closed
	r.gw.SetFactionDefaults([]database.FactionDefault{{Name: "Citizens of Freeport", Standing: 1}})
	r.gw.SetTestingHeroes(true)
	got, box = r.visit()
	r.want("Anna", got, "box:Testing Heroes Vendor: My ledger is closed today.")
	if box != 0 {
		t.Error("closed, and a box")
	}
}

// More wares than a box holds go five a page with More..., the last page's
// More going round; every question and answer is as long as CLIENT shows, for
// the petition's five and for names as long as a record's may be.
func TestTheHeroesVendorsBoxesFit(t *testing.T) {
	for n := 0; n <= 14; n++ {
		seen := map[int]bool{}
		pages := 0
		for page := 0; ; page++ {
			shown, more := heroesPageOf(n, page)
			count := shown[1] - shown[0]
			if more {
				count++
			}
			if count > maxDialogOptions || (n > 0 && shown[1] <= shown[0]) {
				t.Fatalf("%d wares, page %d: %v more %v", n, page, shown, more)
			}
			for i := shown[0]; i < shown[1]; i++ {
				seen[i] = true
			}
			pages++
			if !more || len(seen) >= n {
				break
			}
		}
		if len(seen) != n || (n <= maxDialogOptions && pages != 1) {
			t.Errorf("%d wares: %d seen in %d pages", n, len(seen), pages)
		}
		if n > maxDialogOptions { // round to the first
			last, _ := heroesPageOf(n, pages-1)
			again, _ := heroesPageOf(n, pages)
			if again[0] != 0 || last[1] != n {
				t.Errorf("%d wares: the last page %v, the one after %v", n, last, again)
			}
		}
	}
	long := strings.Repeat("W", 80)
	for _, name := range append([]string{long}, func() []string {
		var s []string
		for _, w := range heroesWares {
			s = append(s, w.name)
		}
		return s
	}()...) {
		w := heroesWare{row: database.HeroesStockRow{Cost: 1000}, pat: database.ItemPatternRecord{ItemName: name}}
		if l := len([]rune(w.label())); l > questAnswerChars || !strings.HasSuffix(w.label(), ": 1000") {
			t.Errorf("the answer %q is %d characters", w.label(), l)
		}
		if name != long {
			if l := len([]rune(fmt.Sprintf(heroesAskBuy, name, 1000, 4000))); l > questAskChars {
				t.Errorf("the question for %q is %d characters", name, l)
			}
		}
	}
	for _, s := range []string{heroesYes, heroesNo, heroesMore} {
		if len(s) > questAnswerChars {
			t.Errorf("%q is too long an answer", s)
		}
	}
	if l := len(fmt.Sprintf(heroesAskList, 4000)); l > questAskChars {
		t.Errorf("the list's question is %d characters", l)
	}
	if l := len(HeroesVendorName + ": " + fmt.Sprintf(heroesGreeting, "Abcdefghijklmnopqrst", -2000, 4000)); l > questBoxChars {
		t.Errorf("the greeting is %d characters for a long name, a box %d", l, questBoxChars)
	}

	// a seventh ware: the box pages, and More shows it
	r := newHeroesRig(t)
	for i := 0; i < 2; i++ {
		res, err := r.db.Exec(`INSERT INTO itempattern
			(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
			 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
			 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
			VALUES (0, 0, 209763529, 0, -1, 0, 0, 1, 0, 0, 0, 0, 1, 1, 0, 0, 0, 0, 2, 0, 0, 0, ?, '', 0, 0)`, fmt.Sprintf("A GM's Ware %d", i+1))
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		if _, err := r.db.Exec(`INSERT INTO heroes_stock (patternid, cost) VALUES (?, ?)`, id, 2000+i); err != nil {
			t.Fatal(err)
		}
	}
	r.stand(3000)
	got, box := r.visit()
	if !hasMsg(got, fmt.Sprintf("ask#%d:Testing Heroes Vendor: 'You have 3000 to spend. What will you have?'|%s|More...", box, heroesList)) {
		t.Fatalf("the first page: %q", got)
	}
	got, box = r.choose(box, 5)
	if !hasMsg(got, fmt.Sprintf("ask#%d:Testing Heroes Vendor: 'You have 3000 to spend. What will you have?'|A GM's Ware 1: 2000|A GM's Ware 2: 2001|More...", box)) {
		t.Fatalf("the second page: %q", got)
	}
	_, buy := r.choose(box, 1)
	if got, _ := r.choose(buy, 0); !hasMsg(got, "box:Testing Heroes Vendor: The A GM's Ware 2 is yours. You have 999 left to spend.") {
		t.Errorf("a GM's ware off the second page: %q", got)
	}
}

// The interact handler asks him before the quests and the town's dialog, and
// his boxes' answers reach him through 0x0035 (the rig's talk and pick are
// the handler's own); a record of his stock that is no item is left out.
func TestTheHeroesVendorLeavesOutARowOfNoItem(t *testing.T) {
	r := newHeroesRig(t)
	if _, err := r.db.Exec(`INSERT INTO heroes_stock (patternid, cost) VALUES (424242, 1)`); err != nil {
		t.Fatal(err)
	}
	got, box := r.visit()
	if !hasMsg(got, fmt.Sprintf("ask#%d:Testing Heroes Vendor: 'You have 0 to spend. What will you have?'|%s", box, heroesList)) {
		t.Errorf("the list with a row of no item: %q", got)
	}
}
