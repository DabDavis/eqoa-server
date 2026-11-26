package handlers

import (
	"encoding/binary"
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// mobDetailsSwitch turns world.mob_details on for one test, off again after it.
func mobDetailsSwitch(t *testing.T) {
	t.Helper()
	SetMobDetails(true)
	t.Cleanup(func() { SetMobDetails(false) })
}

// readMobDetails reads an OpcodeMobDetails back (from its opcode on): the id,
// the status and, shown, the details.
func readMobDetails(t *testing.T, b []byte) (uint32, byte, *mobDetails) {
	t.Helper()
	if op := protocol.GameOpcode(binary.LittleEndian.Uint16(b)); op != protocol.OpcodeMobDetails || b[2] != mobDetailsLayout {
		t.Fatalf("not a layout 1 OpcodeMobDetails: %x", b)
	}
	id, status := binary.LittleEndian.Uint32(b[3:]), b[7]
	if status != mobDetailsShown {
		if len(b) != 8 {
			t.Fatalf("status %d carries %d bytes more", status, len(b)-8)
		}
		return id, status, nil
	}
	p := 8
	text := func() string {
		n := int(b[p])
		s := string(b[p+1 : p+1+n])
		p += 1 + n
		return s
	}
	d := &mobDetails{}
	d.Name = text()
	d.Level = int(b[p])
	p++
	d.Class = text()
	d.Con = b[p]
	p++
	d.HP, d.MaxHP = int(binary.LittleEndian.Uint32(b[p:])), int(binary.LittleEndian.Uint32(b[p+4:]))
	p += 8
	lines := int(b[p])
	p++
	for ; lines > 0; lines-- {
		d.Lines = append(d.Lines, text())
	}
	rows := int(binary.LittleEndian.Uint16(b[p:]))
	d.LeftOut = b[p+2]&1 != 0
	p += 3
	for ; rows > 0; rows-- {
		r := lootRow{Source: b[p], Chance: float64(binary.LittleEndian.Uint16(b[p+1:])) / 10000, Pattern: int(binary.LittleEndian.Uint32(b[p+3:]))}
		p += 7
		r.Text = text()
		d.Rows = append(d.Rows, r)
	}
	if p != len(b) {
		t.Fatalf("read %d of %d bytes", p, len(b))
	}
	return id, status, d
}

// near is whether two chances agree to a hundredth of a percent (the wire's).
func near(a, b float64) bool { return math.Abs(a-b) < 0.00005 }

// The chart is the corpse's model (Heirloom petition 3163): its own item every
// kill, and each table item at the chance rollLoot gives it, the table's
// chance a roll times the item's share of the weights, over its rolls
// (beetle: 40% a roll, two rolls, the sword 3 of 4 weights: 1-(1-0.3)^2, the
// bait 1-(1-0.1)^2). Checked against rollLoot itself over 200,000 corpses. The
// control: a level no table row covers lists the creature's own item alone.
func TestMobDetailsChartIsTheCorpsesModel(t *testing.T) {
	f := newCorpseFixture(t, 1)
	bindLoot(t, f, "beetle", 40, 2)
	npc := &entity.NPC{ServerID: 5001, Name: "beetle", NPCID: corpseNPCID, Level: 15, HP: 90, MaxHP: 90}

	lines, rows := f.h.lootChart(npc)
	if len(lines) != 0 || len(rows) != 3 {
		t.Fatalf("lines %q rows %+v, want three rows", lines, rows)
	}
	want := []lootRow{
		{lootFromOwn, 1, f.sword, "Rusty Sword"},
		{lootFromTable, 1 - 0.7*0.7, f.sword, "Rusty Sword"},
		{lootFromTable, 1 - 0.9*0.9, f.bait, "Fishing Bait"},
	}
	for i, w := range want {
		if r := rows[i]; r.Source != w.Source || r.Pattern != w.Pattern || r.Text != w.Text || !near(r.Chance, w.Chance) {
			t.Errorf("row %d: %+v, want %+v", i, r, w)
		}
	}

	// rollLoot's own dice over many corpses agree with the chart
	rolls, err := f.inv.NPCLootRolls("beetle", 15)
	if err != nil {
		t.Fatal(err)
	}
	const n = 200000
	r := rand.New(rand.NewSource(3163))
	seen := map[int]int{}
	for i := 0; i < n; i++ {
		got := map[int]bool{}
		for _, it := range rollLoot(rolls, r.Intn) {
			got[it.Pattern.PatternID] = true
		}
		for id := range got {
			seen[id]++
		}
	}
	for _, row := range rows[1:] {
		if obs := float64(seen[row.Pattern]) / n; math.Abs(obs-row.Chance) > 0.005 {
			t.Errorf("%s: the chart says %.4f, rollLoot gave it on %.4f of corpses", row.Text, row.Chance, obs)
		}
	}

	npc.Level = 25
	if _, rows := f.h.lootChart(npc); len(rows) != 1 || rows[0].Source != lootFromOwn {
		t.Fatalf("the control, level 25: %+v, want its own item alone", rows)
	}
	// a corpse of nothing (a pet's) lists nothing
	npc.Owner = 1
	if lines, rows := f.h.lootChart(npc); len(rows) != 0 || len(lines) != 1 {
		t.Fatalf("a pet's: lines %q rows %+v", lines, rows)
	}
}

// A category is summarised (its region's pools are large): the rate, or with
// counts every count but none's share; an empty pool is left out. Two tables
// giving one item are one row, either landing. The control: a table of no
// chance gives no row.
func TestMobDetailsCategoriesAndMerges(t *testing.T) {
	a := database.CharInventoryEntry{Pattern: database.ItemPatternRecord{PatternID: 1, ItemName: "gem"}}
	b := database.CharInventoryEntry{Pattern: database.ItemPatternRecord{PatternID: 2, ItemName: "fang"}}
	cats := categoryChart([]database.CategoryRoll{
		{Category: "gems", Rate: 0.3, Choices: []database.LootChoice{{Item: a, Weight: 1}, {Item: b, Weight: 2}}},
		{Category: "trash", Counts: []float64{70, 20, 10}, Choices: []database.LootChoice{{Item: b, Weight: 1}}},
		{Category: "rare", Rate: 0.01},
	})
	if len(cats) != 2 || !near(cats[0].Chance, 0.3) || cats[0].Text != "gems: one of 2 items" || cats[0].Pattern != 0 ||
		!near(cats[1].Chance, 0.3) || cats[1].Text != "trash: one of 1 item" {
		t.Fatalf("categories %+v", cats)
	}
	rows := mergeLootRows(append(
		tableChart([]database.LootRoll{{Chance: 50, Rolls: 1, Choices: []database.LootChoice{{Item: a, Weight: 1}}}}, lootFromTable),
		tableChart([]database.LootRoll{{Chance: 50, Rolls: 1, Choices: []database.LootChoice{{Item: a, Weight: 1}}},
			{Chance: 0, Rolls: 3, Choices: []database.LootChoice{{Item: b, Weight: 1}}}}, lootFromTable)...))
	if len(rows) != 1 || !near(rows[0].Chance, 0.75) {
		t.Fatalf("merged %+v, want one row at 0.75", rows)
	}
}

// The wire: a chance of any size above none is at least 1 (0.01%), at most
// 10000; status 0 reads back whole, past the most rows the flag set.
func TestMobDetailsWire(t *testing.T) {
	if chanceBasis(0) != 0 || chanceBasis(0.000001) != 1 || chanceBasis(0.5) != 5000 || chanceBasis(2) != 10000 {
		t.Fatal("chanceBasis")
	}
	d := &mobDetails{Name: "moss snake", Level: 3, Class: "Warrior", Con: entity.ConGreen, HP: 30, MaxHP: 40,
		Lines: []string{"a line"}}
	for i := 0; i < mobDetailsRowsMost+5; i++ {
		d.Rows = append(d.Rows, lootRow{Source: lootFromTable, Chance: 0.25, Pattern: 7, Text: "fang"})
	}
	id, status, got := readMobDetails(t, buildMobDetails(0x186A1, mobDetailsShown, d))
	if id != 0x186A1 || status != 0 || got.Name != "moss snake" || got.Level != 3 || got.Class != "Warrior" ||
		got.Con != entity.ConGreen || got.HP != 30 || got.MaxHP != 40 || len(got.Lines) != 1 || got.Lines[0] != "a line" {
		t.Fatalf("read back %+v", got)
	}
	if len(got.Rows) != mobDetailsRowsMost || !got.LeftOut || got.Rows[0] != (lootRow{lootFromTable, 0.25, 7, "fang"}) {
		t.Fatalf("rows %d left out %v first %+v", len(got.Rows), got.LeftOut, got.Rows[0])
	}
	if _, status, _ := readMobDetails(t, buildMobDetails(5, mobDetailsSoon, nil)); status != mobDetailsSoon {
		t.Fatal("status 2")
	}
}

// mobAsk is a client's OpcodeMobDetailsAsk about id, as the handler gets it.
func mobAsk(id uint32) *protocol.Message {
	b := make([]byte, 5)
	b[0] = 1
	binary.LittleEndian.PutUint32(b[1:], id)
	return incoming(protocol.OpcodeMobDetailsAsk, b)
}

// The answers: off, status 3 (and no offer); on, a monster's details for the
// asker (its con colour the asker's: green, a level 3 to a 30); an NPC with
// the NPC bit, a corpse or an unknown id status 1; past three asks at once
// status 2, then one again two seconds on. The quality-of-life lock (an [OG]
// character's session) is status 3 and no offer, as world.vanilla_ui's.
func TestMobDetailsAnswers(t *testing.T) {
	f := newCorpseFixture(t, 1)
	bindLoot(t, f, "moss snake", 40, 2)
	f.player.Level = 30
	snake := &entity.NPC{ServerID: 701, Name: "moss snake", NPCID: corpseNPCID, Level: 15, HP: 30, MaxHP: 40, Class: 1}
	f.npcs[0x9001] = snake
	f.npcs[0x9002] = &entity.NPC{ServerID: 702, Name: "Guard", Level: 40, NPCType: 0x80, HP: 9, MaxHP: 9}
	f.npcs[0x9003] = &entity.NPC{ServerID: 703, Name: "dead rat", Level: 2, Dead: true}
	clock := time.Unix(1_800_000_000, 0)
	h := &MobDetailsHandler{win: f.h, find: f.h.find, playerOf: f.h.playerOf, now: func() time.Time { return clock }}
	ask := func(id uint32) (uint32, byte, *mobDetails) {
		t.Helper()
		f.sent()
		if err := h.HandleMobDetailsAsk(f.sess, mobAsk(id)); err != nil {
			t.Fatal(err)
		}
		got := f.sent()
		if len(got) != 1 {
			t.Fatalf("%d messages, want one", len(got))
		}
		return readMobDetails(t, got[0])
	}

	SendMobDetailsOffer(f.sess)
	if _, st, _ := ask(0x9001); st != mobDetailsOff || len(f.sent()) != 0 {
		t.Fatalf("the control, off: status %d", st)
	}
	mobDetailsSwitch(t)
	SendMobDetailsOffer(f.sess)
	if got := f.sent(); len(got) != 1 || protocol.GameOpcode(binary.LittleEndian.Uint16(got[0])) != protocol.OpcodeMobDetailsOffer || got[0][2] != 1 {
		t.Fatalf("on: the offer %x", got)
	}
	id, st, d := ask(0x9001)
	if id != 0x9001 || st != mobDetailsShown || d.Name != "moss snake" || d.Level != 15 || d.Class != "Ranger" ||
		d.Con != entity.ConColor(30, 15) || d.HP != 30 || d.MaxHP != 40 || len(d.Rows) != 3 {
		t.Fatalf("the snake: id %x status %d %+v", id, st, d)
	}
	if _, st, _ := ask(0x9002); st != mobDetailsNone {
		t.Fatalf("an NPC with the NPC bit: status %d", st)
	}
	if _, st, _ := ask(0x9099); st != mobDetailsNone {
		t.Fatalf("an unknown id: status %d", st)
	}
	if _, st, _ := ask(0x9001); st != mobDetailsSoon {
		t.Fatalf("a fourth ask at once: status %d, want 2", st)
	}
	clock = clock.Add(mobDetailsEvery)
	if _, st, _ := ask(0x9003); st != mobDetailsNone {
		t.Fatalf("a corpse, an ask later: status %d, want 1", st)
	}

	f.sess.SetOG(true)
	t.Cleanup(func() { f.sess.SetOG(false) })
	clock = clock.Add(10 * mobDetailsEvery)
	SendMobDetailsOffer(f.sess)
	if _, st, _ := ask(0x9001); st != mobDetailsOff || len(f.sent()) != 0 {
		t.Fatalf("an [OG] character's: status %d, want 3 and no offer", st)
	}
}

// The quests' loot step items on the chart (questpreloot.go): a loot step's
// item drops to anyone, at its step's chance, off its creature alone; on the
// asker's own step it says so. The control: another creature lists none.
func TestMobDetailsQuestRows(t *testing.T) {
	f := newQuestFixture(t)
	belt := f.pattern("slashed belt", 20)
	lootQuest(f, belt)
	k := entity.KillCredit{NPCName: "Orc Pawn", NPCSpawn: 900, NPCID: 900}
	rows := f.qe.LootChartRows(k, f.sid)
	if len(rows) != 1 || rows[0].Source != lootFromQuest || rows[0].Pattern != belt || !near(rows[0].Chance, 0.5) || rows[0].Text != "slashed belt" {
		t.Fatalf("not on the quest: %+v", rows)
	}
	if _, err := f.qe.SetCharacterQuest(f.sid, 60, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	if rows := f.qe.LootChartRows(k, f.sid); len(rows) != 1 || rows[0].Text != "slashed belt (your quest)" {
		t.Fatalf("on its step: %+v", rows)
	}
	if rows := f.qe.LootChartRows(entity.KillCredit{NPCName: "a bat", NPCSpawn: 901, NPCID: 901}, f.sid); len(rows) != 0 {
		t.Fatalf("the control, a bat: %+v", rows)
	}
}
