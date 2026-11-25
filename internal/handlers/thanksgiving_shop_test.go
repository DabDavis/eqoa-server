package handlers

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

type gibletReward struct {
	PatternID int `json:"patternid"`
	Name      string
	Desc      string `json:"description"`
	Says      string
	Line      string
	Charges   int
}

func gibletRewards(t *testing.T) []gibletReward {
	t.Helper()
	var plan struct{ Rewards []gibletReward }
	data, err := os.ReadFile("../../tools/thanksgiving/mark-rewards.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &plan); err != nil || len(plan.Rewards) != 2 {
		t.Fatalf("reward definitions: %+v (%v)", plan, err)
	}
	return plan.Rewards
}

func addGibletReward(t *testing.T, db *database.SQLiteDB, reward gibletReward) {
	t.Helper()
	if err := database.NewItemPatternStore(db).UpsertItemPattern(&database.ItemPatternRecord{
		PatternID: reward.PatternID, ItemName: reward.Name, ItemDesc: reward.Desc, LevelReq: 1,
		EquipSlot: -1, ClassUse: 32767, RaceUse: 32767, MaxStack: 2, Trade: 1, Rent: 1, Lore: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO itemeffect (patternid, line, name, charges) VALUES (?,?,?,?)`,
		reward.PatternID, reward.Line, reward.Name, reward.Charges); err != nil {
		t.Fatal(err)
	}
}

func TestGibletSellsThanksgivingRewardsForMarks(t *testing.T) {
	r := newToyRig(t)
	was := HeirloomMarksOn()
	t.Cleanup(func() { SetHeirloomMarks(was) })
	SetHeirloomMarks(true)
	const shop = "Quartermaster Giblet"
	r.shop.Name = shop
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := r.db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO gmseasons (name, active) VALUES ('Thanksgiving',1)`)
	exec(`INSERT INTO npcseason (name, season) VALUES (lower(?),'Thanksgiving')`, shop)
	if _, err := r.db.SpendMarks(r.pa.CharServerID, -60, "fixture", nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	rewards := gibletRewards(t)
	for i, reward := range rewards {
		addGibletReward(t, r.db, reward)
		exec(`INSERT INTO tokenshop (shop, patternid, price, token, says, sort) VALUES (?, ?, 30, ?, ?, ?)`,
			shop, reward.PatternID, database.HeirloomMarksShopToken, reward.Says, i)
	}
	choose := func(i int) uint32 {
		t.Helper()
		lines, box := r.visit()
		if box == 0 || !strings.Contains(strings.Join(lines, "\n"), "Drunk uncle protection: 30|Yelling at cousins: 30") {
			t.Fatalf("Giblet's two offers: %q", lines)
		}
		lines, confirm := r.choose(box, byte(i))
		if confirm == 0 || !strings.Contains(strings.Join(lines, "\n"), rewards[i].Says) {
			t.Fatalf("offer %d: %q", i, lines)
		}
		return confirm
	}
	assertBalance := func(want int) {
		t.Helper()
		row, err := r.db.CharMarksOf(r.pa.CharServerID)
		if err != nil || row.Marks != want {
			t.Fatalf("balance: %d (%v), want %d", row.Marks, err, want)
		}
	}
	confirm := choose(0)
	exec(`UPDATE gmseasons SET active = 0 WHERE name = 'Thanksgiving'`)
	r.choose(confirm, 0)
	assertBalance(60)
	if r.count(rewards[0].PatternID) != 0 {
		t.Fatal("season close allowed an outstanding order")
	}
	exec(`UPDATE gmseasons SET active = 1 WHERE name = 'Thanksgiving'`)
	confirm = choose(0)
	exec(`CREATE TRIGGER broken_receipt BEFORE INSERT ON tokenshop_purchases BEGIN SELECT RAISE(ABORT, 'fixture failure'); END`)
	r.choose(confirm, 0)
	assertBalance(60)
	if r.count(rewards[0].PatternID) != 0 {
		t.Fatal("purchase failure left an item in the pack")
	}
	exec(`DROP TRIGGER broken_receipt`)
	for i, reward := range rewards {
		confirm = choose(i)
		r.choose(confirm, 0)
		r.choose(confirm, 0) // a retransmitted confirmation cannot buy again
		assertBalance(30 * (1 - i))
		var charges int
		if err := r.db.QueryRow(`SELECT remaincharge FROM charinventory WHERE serverid = ? AND patternid = ?`,
			r.pa.CharServerID, reward.PatternID).Scan(&charges); err != nil || charges != reward.Charges || r.count(reward.PatternID) != 1 {
			t.Fatalf("received %q with %d charges (%v)", reward.Name, charges, err)
		}
	}
	_, list := r.visit()
	_, confirm = r.choose(list, 0)
	if confirm != 0 {
		t.Fatal("could confirm with no marks")
	}
}

func TestThanksgivingCharmKeepsChargeOnRefusal(t *testing.T) {
	f := newScribeFixture(t)
	w := f.potionWorld()
	f.player.Level = 60
	f.player.Class = 0
	reward := gibletRewards(t)[1]
	addGibletReward(t, f.db, reward)
	if _, err := f.inv.AddCharItem(f.sid, reward.PatternID, 2, 0, 0, database.LocPack); err != nil {
		t.Fatal(err)
	}
	f.loadList()
	const targetID = 0x01312E98
	const petID = 0x01312E97
	target := &entity.NPC{ServerID: 2002, Name: "target", Level: 55, HP: 100, MaxHP: 100, X: f.player.X + 2, Y: f.player.Y, Z: f.player.Z}
	pet := &entity.NPC{ServerID: 2003, Name: "pet", Level: 1, HP: 100, MaxHP: 100, X: f.player.X + 3, Y: f.player.Y, Z: f.player.Z}
	w.NPCs[target.ServerID], w.NPCs[pet.ServerID] = target, pet
	vs := entity.GetOrCreateVisibility(f.sess)
	vs.ChannelEntity[0], vs.ChannelObjectID[0] = target.ServerID, targetID
	vs.ChannelEntity[1], vs.ChannelObjectID[1] = pet.ServerID, petID
	checkKept := func() {
		t.Helper()
		msgs := f.useAs(0, targetID, 0)
		var qty, charges int
		if err := f.db.QueryRow(`SELECT stackleft,remaincharge FROM charinventory WHERE serverid = ? AND patternid = ?`,
			f.sid, reward.PatternID).Scan(&qty, &charges); err != nil || qty != 2 || charges != 1 || target.Owner != 0 || len(msgsOf(msgs, protocol.OpcodeInteractItem)) != 0 {
			t.Fatalf("refusal: stack %d, charges %d (%v), target owner %d", qty, charges, err, target.Owner)
		}
	}
	w.SetCharmRules(map[string]database.CharmRule{target.Name: {Charmable: false}})
	checkKept()
	w.SetCharmRules(nil)
	pe := entity.NewPotionEffect(716914, "Puppet Strings", "Target, Charm up to light blue, 10min", "", 0, "", 0, 0)
	if why, err := w.UsePotion(f.sess, pe, petID, time.Now()); why != "" || err != nil {
		t.Fatalf("fixture pet: %q (%v)", why, err)
	}
	checkKept()
	w.PetCommand(f.sess, 5, 0, time.Now())
	msgs := f.useAs(0, targetID, 0)
	if target.Owner != f.player.ServerID || len(msgsOf(msgs, protocol.OpcodeRemoveInvItem)) != 1 || f.stackOf(reward.Name) != 1 {
		t.Fatalf("first charm: owner %d, pack %d", target.Owner, f.stackOf(reward.Name))
	}
	w.PetCommand(f.sess, 5, 0, time.Now())
	second := &entity.NPC{ServerID: 2004, Name: "another target", Level: 50, HP: 100, MaxHP: 100, X: f.player.X + 2, Y: f.player.Y, Z: f.player.Z}
	w.NPCs[second.ServerID] = second
	vs.ChannelEntity[0], vs.ChannelObjectID[0] = second.ServerID, targetID
	msgs = f.useAs(0, targetID, 0)
	if _, ok := sessionInvList(f.sess).at(0); ok || second.Owner != f.player.ServerID ||
		len(msgsOf(msgs, protocol.OpcodeInteractItem)) != 1 || f.stackOf(reward.Name) != 0 {
		t.Fatalf("second charm: owner %d, pack %d", second.Owner, f.stackOf(reward.Name))
	}
}

func TestGibletTwoCopiesAcrossPackAndBank(t *testing.T) {
	r := newToyRig(t)
	wasMarks := HeirloomMarksOn()
	wasBank, slots := database.AccountBankOn(), database.AccountBankSlots()
	t.Cleanup(func() { SetHeirloomMarks(wasMarks); database.SetAccountBank(wasBank, slots) })
	SetHeirloomMarks(true)
	database.SetAccountBank(true, 2000)
	r.shop.Name = "Quartermaster Giblet"
	sid := r.pa.CharServerID
	alt := &database.CharacterRecord{CharName: "GibletBankAlt", AccountID: 1}
	if err := database.NewSQLiteCharacterStore(r.db).Create(alt); err != nil {
		t.Fatal(err)
	}
	rewards := gibletRewards(t)
	for i, reward := range rewards {
		addGibletReward(t, r.db, reward)
		if _, err := r.db.Exec(`INSERT INTO tokenshop (shop,patternid,price,token,sort) VALUES (?,?,30,?,?)`,
			r.shop.Name, reward.PatternID, database.HeirloomMarksShopToken, i); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.db.SpendMarks(sid, -300, "fixture", nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	pid := rewards[0].PatternID
	place := func(owner uint32, loc, n int) int {
		t.Helper()
		id, err := r.inv.AddCharItem(owner, pid, n, 0, 3, loc)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	offer := func(i int) uint32 {
		t.Helper()
		_, box := r.visit()
		_, yes := r.choose(box, byte(i))
		if yes == 0 {
			t.Fatalf("reward %d was refused below the limit", i)
		}
		return yes
	}
	capped := func() {
		t.Helper()
		_, box := r.visit()
		lines, yes := r.choose(box, 0)
		if yes != 0 || !strings.Contains(strings.Join(lines, " "), "inventory and bank") {
			t.Fatalf("wanted cap refusal, got %d %q", yes, lines)
		}
	}
	checkSales := func(n int) {
		t.Helper()
		marks, err := r.db.CharMarksOf(sid)
		var receipts int
		if err != nil || marks.Marks != 300-30*n {
			t.Fatalf("marks %d after %d sales: %v", marks.Marks, n, err)
		}
		if err := r.db.QueryRow("SELECT count(*) FROM tokenshop_purchases WHERE serverid=?", sid).Scan(&receipts); err != nil || receipts != n {
			t.Fatalf("receipts %d, want %d: %v", receipts, n, err)
		}
	}
	place(alt.ServerID, database.LocPack, 2) // an alt's personal pack is outside the buyer's ownership
	r.choose(offer(0), 0)
	// A full pack still has room for the second copy in the same slot.
	l := sessionInvList(r.anna)
	l.cap = 1
	r.choose(offer(0), 0)
	rec, ok := l.at(0)
	if !ok || rec.qty != 2 || len(l.itemIDs()) != 1 {
		t.Fatalf("pack mirror after two purchases: %+v", rec)
	}
	item, err := r.inv.GetCharItem(sid, rec.itemID)
	if err != nil || item.StackLeft != 2 || item.Charges != 3 || item.Pattern.Lore != 1 || item.Pattern.Trade != 1 {
		t.Fatalf("stack after two purchases: %+v (%v)", item, err)
	}
	capped()
	checkSales(2)
	destroy := NewEquipHandler(nil, r.inv, nil, nil)
	destroy.playerOf = r.ih.tokenShop.gw.SessionPlayer
	if !destroy.destroy(r.anna, rec.key, 1) {
		t.Fatal("could not delete a copy")
	}
	yes := offer(0)
	banked := place(alt.ServerID, database.LocBank, 1) // not on this session's bank page
	lines, _ := r.choose(yes, 0)
	if !strings.Contains(strings.Join(lines, " "), "inventory and bank") {
		t.Fatalf("stale confirmation: %q", lines)
	}
	capped()
	checkSales(2)
	if err := r.inv.DeleteCharItem(sid, banked); err != nil {
		t.Fatal(err)
	}
	r.choose(offer(0), 0)
	checkSales(3)
	if !destroy.destroy(r.anna, rec.key, 2) {
		t.Fatal("could not delete the full stack")
	}
	place(alt.ServerID, database.LocBank, 2)
	capped()              // both in the shared bank, zero in the pack
	r.choose(offer(1), 0) // the other reward has its own cap
	checkSales(4)
	// The database enforces the cap even when called without the menu.
	var stock int
	if err := r.db.QueryRow("SELECT id FROM tokenshop WHERE shop=? AND patternid=?", r.shop.Name, pid).Scan(&stock); err != nil {
		t.Fatal(err)
	}
	_, _, err = r.inv.BuyChargedMarkShopItem(stock, database.TokenShopPurchase{ServerID: sid, Shop: r.shop.Name,
		PatternID: pid, Price: 30, Item: rewards[0].Name, Token: database.HeirloomMarksShopToken})
	if !errors.Is(err, database.ErrGibletRewardLimit) {
		t.Fatalf("direct grant at 2/2: %v", err)
	}
	checkSales(4)
	database.SetAccountBank(false, 0)
	place(sid, database.LocBank, 1)
	place(sid, database.LocPackKept, 1)
	capped() // a personal bank and an inventory copy kept off the client's list
	checkSales(4)
	// Two independent confirmations racing for one remaining space can spend once.
	other := &database.CharacterRecord{CharName: "GibletRace", AccountID: 2}
	if err := database.NewSQLiteCharacterStore(r.db).Create(other); err != nil {
		t.Fatal(err)
	}
	place(other.ServerID, database.LocBank, 1)
	if _, err := r.db.SpendMarks(other.ServerID, -60, "fixture", nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	for range 2 {
		go func() {
			_, _, err := r.inv.BuyChargedMarkShopItem(stock, database.TokenShopPurchase{ServerID: other.ServerID,
				Shop: r.shop.Name, PatternID: pid, Price: 30, Item: rewards[0].Name, Token: database.HeirloomMarksShopToken})
			results <- err
		}()
	}
	var sales, refused int
	for range 2 {
		switch err := <-results; {
		case err == nil:
			sales++
		case errors.Is(err, database.ErrGibletRewardLimit):
			refused++
		default:
			t.Fatal(err)
		}
	}
	balance, err := r.db.CharMarksOf(other.ServerID)
	if err != nil || sales != 1 || refused != 1 || balance.Marks != 30 {
		t.Fatalf("concurrent confirmations: %d sales, %d refusals, %d marks (%v)", sales, refused, balance.Marks, err)
	}
}

func TestGibletShieldKeepsChargesPerCopy(t *testing.T) {
	f := newScribeFixture(t)
	f.potionWorld()
	reward := gibletRewards(t)[0]
	addGibletReward(t, f.db, reward)
	f.own(reward.PatternID, 2)
	f.loadList()
	for use := 1; use <= 6; use++ {
		rec, ok := sessionInvList(f.sess).at(0)
		if !ok {
			t.Fatalf("both shields gone before use %d", use)
		}
		f.useAs(rec.key, 0xFFFFFFFF, 0)
		if use == 6 {
			if _, ok := sessionInvList(f.sess).at(0); ok || f.stackOf(reward.Name) != 0 {
				t.Fatal("spent shields still held")
			}
			break
		}
		row, err := f.inv.GetCharItem(f.sid, rec.itemID)
		if err != nil || row.StackLeft != (8-use)/3 || row.FrontCharges() != 3-use%3 {
			t.Fatalf("after %d uses: %+v (%v)", use, row, err)
		}
	}
}

func TestGibletBankJoinsAndSplitsUsedShields(t *testing.T) {
	f := newAcctFixture(t, true, 0)
	reward := gibletRewards(t)[0]
	addGibletReward(t, f.db, reward)
	f.maxStack[reward.PatternID] = 2
	sid := f.sid["Ash"]
	active, err := f.inv.AddCharItem(sid, reward.PatternID, 1, 0, 2, database.LocPack)
	if err != nil {
		t.Fatal(err)
	}
	spare, err := f.inv.AddCharItem(f.sid["Birch"], reward.PatternID, 1, 0, 1, database.LocBank)
	if err != nil {
		t.Fatal(err)
	}
	f.login("Ash", 40)
	f.bankItem("Ash", true, f.keyOf("Ash", true, spare), 1)
	mustOps(t, f.sent("Ash"), protocol.OpcodeRemoveBankItem, protocol.OpcodeAddInvItem)
	check := func(id, qty, charges int) database.CharInventoryEntry {
		t.Helper()
		row, err := f.inv.GetCharItem(sid, id)
		if err != nil || row.StackLeft != qty || row.FrontCharges() != charges {
			t.Fatalf("item %d, want %d copies with %d active charges: %+v (%v)", id, qty, charges, row, err)
		}
		return row
	}
	check(active, 2, 2)
	f.bankItem("Ash", false, f.keyOf("Ash", false, active), 1)
	mustOps(t, f.sent("Ash"), protocol.OpcodeRemoveInvItem, protocol.OpcodeAddBankItem)
	check(active, 1, 2)
	bankIDs := sessionBankList(f.sess["Ash"]).itemIDs()
	if len(bankIDs) != 1 {
		t.Fatalf("wanted one banked shield, got %v", bankIDs)
	}
	check(bankIDs[0], 1, 1)
	f.bankItem("Ash", true, f.keyOf("Ash", true, bankIDs[0]), 1)
	mustOps(t, f.sent("Ash"), protocol.OpcodeRemoveBankItem, protocol.OpcodeAddInvItem)
	row := check(active, 2, 2)
	// Destroying a spare and buying it back keeps its one remaining charge.
	if err := f.inv.SetItemStack(sid, active, 1); err != nil {
		t.Fatal(err)
	}
	receipt, kept, err := f.inv.AddBuyBack(sid, &row, 1, 0, database.BuyBackDestroyed)
	if err != nil || !kept {
		t.Fatalf("buyback receipt: %t (%v)", kept, err)
	}
	if _, err := f.inv.TakeBuyBack(sid, receipt, active, 2, 0); err != nil {
		t.Fatal(err)
	}
	restored := check(active, 2, 2)
	if err := f.inv.SetItemCharges(sid, active, restored.Charges-1); err != nil {
		t.Fatal(err)
	}
	next, err := f.inv.FinishGibletCopy(sid, check(active, 2, 1))
	if err != nil || next != 1 {
		t.Fatalf("spare promoted with %d charges: %v", next, err)
	}
	check(active, 1, 1)
}

func TestGibletCountsOffPageBankAndRespectsHardcore(t *testing.T) {
	f := newAcctFixture(t, true, 0)
	reward := gibletRewards(t)[0]
	addGibletReward(t, f.db, reward)
	f.maxStack[reward.PatternID] = 2
	f.own("Ash", f.sword, 1, 1, database.LocBank)
	banked, err := f.inv.AddCharItem(f.sid["Birch"], reward.PatternID, 1, 0, 2, database.LocBank)
	if err != nil {
		t.Fatal(err)
	}
	carried, err := f.inv.AddCharItem(f.sid["Ash"], reward.PatternID, 1, 0, 1, database.LocPack)
	if err != nil {
		t.Fatal(err)
	}
	f.login("Ash", 1)
	f.bankItem("Ash", false, f.keyOf("Ash", false, carried), 1)
	mustOps(t, f.sent("Ash"), protocol.OpcodeRemoveInvItem, protocol.OpcodeConfirmBankTunar, protocol.OpcodeColoredChat)
	row, err := f.inv.GetCharItem(f.sid["Ash"], banked)
	if err != nil || row.StackLeft != 2 || row.FrontCharges() != 2 {
		t.Fatalf("off-page merge: %+v (%v)", row, err)
	}
	count := func(name string, want int) {
		t.Helper()
		n, err := f.inv.GibletRewardHeld(f.sid[name], reward.PatternID)
		if err != nil || n != want {
			t.Fatalf("%s holds %d, want %d: %v", name, n, want, err)
		}
	}
	count("Ash", 2)
	count("Birch", 2)
	count("Cedar", 0)
	was := entity.HardcoreOn()
	entity.SetHardcoreOn(true)
	t.Cleanup(func() { entity.SetHardcoreOn(was) })
	if _, err := f.db.Exec("UPDATE characters SET hardcore=1 WHERE serverid=?", f.sid["Birch"]); err != nil {
		t.Fatal(err)
	}
	count("Ash", 0)
	count("Birch", 2)
}
