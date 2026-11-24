package handlers

import (
	"encoding/json"
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
		EquipSlot: -1, ClassUse: 32767, RaceUse: 32767, MaxStack: 1,
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
	if _, err := f.inv.AddCharItem(f.sid, reward.PatternID, 1, 0, 0, database.LocPack); err != nil {
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
		var charges int
		if err := f.db.QueryRow(`SELECT remaincharge FROM charinventory WHERE serverid = ? AND patternid = ?`,
			f.sid, reward.PatternID).Scan(&charges); err != nil || charges != 1 || target.Owner != 0 || len(msgsOf(msgs, protocol.OpcodeInteractItem)) != 0 {
			t.Fatalf("refusal: charges %d (%v), target owner %d", charges, err, target.Owner)
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
	if target.Owner != f.player.ServerID || len(msgsOf(msgs, protocol.OpcodeInteractItem)) != 1 {
		t.Fatalf("successful charm: owner %d, charge messages %d", target.Owner, len(msgsOf(msgs, protocol.OpcodeInteractItem)))
	}
	if _, ok := sessionInvList(f.sess).at(0); ok {
		t.Fatal("spent charm still appears in inventory")
	}
}
