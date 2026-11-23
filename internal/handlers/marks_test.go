package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// marksRig is the portal rig with Heirloom Marks on, its awards written at once and every roll won
// unless rolls says otherwise.
type marksRig struct {
	*portalRig
	m     *HeirloomMarks
	db    *database.SQLiteDB
	rolls int
	roll  float64
}

func newMarksRig(t *testing.T) *marksRig {
	t.Helper()
	r := &marksRig{portalRig: newPortalRig(t)}
	store, inv := gmRows(t, r.chatWorld)
	r.db = store.DB()
	SetHeirloomMarks(true)
	t.Cleanup(func() { SetHeirloomMarks(false) })
	r.m = NewHeirloomMarks(r.db, r.gw, nil, inv, nil)
	r.m.inline = true
	r.m.now = func() time.Time { return r.now }
	r.m.roll = func() float64 { r.rolls++; return r.roll }
	r.ih.marks, r.h.marks = r.m, r.m
	r.pa.Level = 10
	return r
}

func (r *marksRig) marks() database.CharMarks {
	r.t.Helper()
	m, err := r.db.CharMarksOf(r.pa.CharServerID)
	if err != nil {
		r.t.Fatal(err)
	}
	return m
}

// kill is a kill heard of Anna's at weight 1, of a creature spawned at (x, z) in world 0.
func (r *marksRig) kill(group int, x, z float32) {
	r.m.killHeard(entity.KillMarks{Sess: r.anna, ServerID: r.pa.CharServerID, Name: r.pa.Name, Weight: 1, Group: group,
		NPCRow: 900, NPCName: "orc pawn", World: 0, SpawnX: x, SpawnZ: z})
}

// A quest pays its first finish alone, not a repeatable one, not one outlevelled by 6 (by 5 it
// does), not a level 60 epic and not a 60 that does not relive.
func TestQuestMarksFirstFinishInReach(t *testing.T) {
	r := newMarksRig(t)
	q := &database.Quest{ID: 101, Name: "Ten Minutes", MinLevel: 10, Steps: []database.QuestStep{
		{Kind: database.StepTalk}, {Kind: database.StepKill, Count: 4}, {Kind: database.StepTalk}}}
	r.m.QuestDone(r.anna, r.pa, q, true, r.pa.Level)
	if got := r.marks().Marks; got != 17 {
		t.Fatalf("a 10-minute quest: %d marks, want 17", got)
	}
	if !hasMsg(r.msgs(r.anna), "chat:You earn 17 Heirloom Marks.") {
		t.Error("no line")
	}
	r.m.QuestDone(r.anna, r.pa, q, false, r.pa.Level)
	rep := *q
	rep.Repeatable = true
	r.m.QuestDone(r.anna, r.pa, &rep, true, r.pa.Level)
	r.pa.Level = 16
	r.m.QuestDone(r.anna, r.pa, q, true, r.pa.Level)
	if got := r.marks().Marks; got != 17 {
		t.Fatalf("a second finish, a repeatable and one outlevelled by 6 paid: %d", got)
	}
	r.pa.Level = 15 // the control: by 5 it pays
	r.m.QuestDone(r.anna, r.pa, q, true, r.pa.Level)
	if got := r.marks().Marks; got != 34 {
		t.Fatalf("outlevelled by 5: %d, want 34", got)
	}
	if _, err := r.db.Exec(`CREATE TABLE IF NOT EXISTS quest_import_quests (key TEXT PRIMARY KEY, questid INTEGER NOT NULL, file TEXT NOT NULL DEFAULT '', digest TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 1)`); err != nil {
		t.Fatal(err)
	}
	r.db.Exec(`INSERT INTO quest_import_quests (key, questid) VALUES ('60/epic/warrior', 102)`)
	r.m.keys = nil
	epic := *q
	epic.ID, epic.MinLevel = 102, 59
	r.pa.Level = 59
	r.m.QuestDone(r.anna, r.pa, &epic, true, r.pa.Level)
	r.pa.Level = 60
	r.m.QuestDone(r.anna, r.pa, q, true, r.pa.Level)
	if got := r.marks().Marks; got != 34 {
		t.Fatalf("a level 60 epic or a 60 paid: %d", got)
	}
}

// The real-time limit: a quest taken 3 minutes ago pays at most 10.
func TestQuestMarksRealTimeLimit(t *testing.T) {
	r := newMarksRig(t)
	q := &database.Quest{ID: 103, MinLevel: 10, Steps: []database.QuestStep{{Kind: database.StepTalk}, {Kind: database.StepTalk}, {Kind: database.StepTalk}}}
	qs := database.NewQuestStore(r.db)
	if err := qs.SaveCharQuest(&database.CharQuest{ServerID: r.pa.CharServerID, QuestID: 103, Step: 1}); err != nil {
		t.Fatal(err)
	}
	qs.MarkQuestTaken(r.pa.CharServerID, 103, r.now.Add(-3*time.Minute))
	r.m.QuestDone(r.anna, r.pa, q, true, r.pa.Level)
	if got := r.marks().Marks; got != 10 {
		t.Fatalf("taken 3 minutes ago: %d, want 10 (20 uncapped)", got)
	}
}

// A quest is judged by the level it was finished at, before its experience: a reward that took its
// finisher past minlevel + 5, or to 60, does not take its marks away. The controls: finished at 16,
// or at 60, it pays nothing.
func TestQuestMarksJudgeTheLevelBeforeTheReward(t *testing.T) {
	r := newMarksRig(t)
	q := &database.Quest{ID: 104, MinLevel: 10, Steps: []database.QuestStep{
		{Kind: database.StepTalk}, {Kind: database.StepKill, Count: 4}, {Kind: database.StepTalk}}}
	r.pa.Level = 19 // where the reward left it
	r.m.QuestDone(r.anna, r.pa, q, true, 16)
	if got := r.marks().Marks; got != 0 {
		t.Fatalf("finished at 16: %d marks, want 0", got)
	}
	r.m.QuestDone(r.anna, r.pa, q, true, 15)
	if got := r.marks().Marks; got != 17 {
		t.Fatalf("finished at 15, its reward taking it to 19: %d marks, want 17", got)
	}
	top := *q
	top.ID, top.MinLevel = 105, 55
	r.pa.Level = 60
	r.m.QuestDone(r.anna, r.pa, &top, true, 60)
	if got := r.marks().Marks; got != 17 {
		t.Fatalf("finished at 60: %d marks, want 17", got)
	}
	r.m.QuestDone(r.anna, r.pa, &top, true, 59)
	if got := r.marks().Marks; got != 34 {
		t.Fatalf("finished at 59, its reward taking it to 60: %d marks, want 34", got)
	}
}

// Through the quest engine: a quest whose own experience levels its finisher past the quest's limit
// pays its marks. The fixture's quest 10010 (minlevel 1: it pays to level 6) given to a level 6 with
// the experience of levels 6 and 7 leaves it at 8.
func TestQuestMarksAQuestsRewardDoesNotOutlevelIt(t *testing.T) {
	f := newQuestFixture(t)
	f.exec(`UPDATE queststeps SET xp = ? WHERE questid = 10010 AND step = 2`, entity.CharXPDict[6]+entity.CharXPDict[7])
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	m := NewHeirloomMarks(f.db, f.tw.w, nil, f.inv, nil)
	m.inline = true
	later := time.Now().Add(time.Hour) // the real-time limit out of the way
	m.now = func() time.Time { return later }
	SetHeirloomMarks(true)
	SetHeirloomMarksService(m)
	t.Cleanup(func() { SetHeirloomMarks(false); SetHeirloomMarksService(nil) })
	f.tw.p.Level = 6
	malsis := f.tw.add(1, &entity.NPC{ServerID: 701, Name: "Malsis", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	kellina := f.tw.add(2, &entity.NPC{ServerID: 702, Name: "Kellina", SpawnX: 25404, SpawnY: 54.1, SpawnZ: 15726})
	f.tw.talk(malsis)
	f.tw.pick(boxID(f.msgs()), 0)
	f.tw.talk(kellina)
	f.msgs()
	f.own(f.pincer, 2)
	f.tw.talk(kellina)
	f.tw.pick(boxID(f.msgs()), 0)
	msgs := f.msgs()
	if s, d := f.progress(10010); s != 0 || d != 1 {
		t.Fatalf("not finished: step %d done %d", s, d)
	}
	if f.tw.p.Level != 8 {
		t.Fatalf("the reward left it at level %d, want 8", f.tw.p.Level)
	}
	got, err := f.db.CharMarksOf(f.sid)
	if err != nil {
		t.Fatal(err)
	}
	if got.Marks != 13 {
		t.Fatalf("finished at 6, its reward taking it to 8: %d marks, want 13", got.Marks)
	}
	f.want(msgs, "chat:You earn 13 Heirloom Marks.")
}

// Kill marks: each won roll a mark, no roll past 38 in the hour; a group of four earners x1.3 and a
// Hot zone's +35% on a creature spawned inside it, the fraction carried; #hotzone off stops the
// bonus at once; a lost roll gives nothing.
func TestKillMarksCapGroupAndHotZone(t *testing.T) {
	r := newMarksRig(t)
	for i := 0; i < database.KillMarksAnHour; i++ {
		r.kill(0, 0, 0)
	}
	if got := r.marks().Marks; got != database.KillMarksAnHour {
		t.Fatalf("%d kills: %d marks", database.KillMarksAnHour, got)
	}
	rolled := r.rolls
	r.kill(0, 0, 0)
	if r.rolls != rolled || r.marks().Marks != database.KillMarksAnHour {
		t.Fatalf("past the cap: rolled %v, %d marks", r.rolls != rolled, r.marks().Marks)
	}
	r.now = r.now.Add(time.Hour)
	r.roll = 0.5 // the control: a lost roll
	r.kill(0, 0, 0)
	if r.marks().Marks != database.KillMarksAnHour {
		t.Fatal("a lost roll paid")
	}
	r.roll = 0
	// a Hot zone round (5000, 5000), made by a GM's #hotzone where Anna stands
	r.pa.X, r.pa.Z = 5000, 5000
	r.m.hotZoneCommand(r.anna, r.pa, "here 100 Orc Camp")
	zones, _ := r.db.HotZones()
	if len(zones) != 1 || !zones[0].Enabled || zones[0].BonusPct != 35 {
		t.Fatalf("zones %+v", zones)
	}
	before := r.marks().Marks
	for i := 0; i < 10; i++ {
		r.kill(3, 5010, 5010) // four earners: x1.3, in the zone x1.35: 1.755 a kill
	}
	if got := r.marks().Marks - before; got != 17 {
		t.Fatalf("ten kills at 1.755: %d, want 17", got)
	}
	var bonus float64
	r.db.QueryRow(`SELECT bonus FROM charmarks_log ORDER BY id DESC LIMIT 1`).Scan(&bonus)
	if bonus < 1.754 || bonus > 1.756 {
		t.Fatalf("the last kill's bonus %v", bonus)
	}
	r.m.hotZoneCommand(r.anna, r.pa, "off 1")
	r.kill(0, 5010, 5010)
	r.db.QueryRow(`SELECT bonus FROM charmarks_log ORDER BY id DESC LIMIT 1`).Scan(&bonus)
	if bonus != 1 {
		t.Fatalf("after #hotzone off: bonus %v", bonus)
	}
	var audit int
	r.db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE tbl = 'hotzones'`).Scan(&audit)
	if audit != 2 {
		t.Errorf("%d audit lines, want 2", audit)
	}
	// a kill outside the zone: no zone bonus (the control)
	r.m.hotZoneCommand(r.anna, r.pa, "on 1")
	r.kill(0, 9000, 9000)
	r.db.QueryRow(`SELECT bonus FROM charmarks_log ORDER BY id DESC LIMIT 1`).Scan(&bonus)
	if bonus != 1 {
		t.Fatalf("a kill outside the zone: bonus %v", bonus)
	}
}

// kill60 is a kill heard of Anna's as a level 60 not reliving (At60) at weight 1, its group's earners
// group and with its 60s group60.
func (r *marksRig) kill60(group, group60 int, x, z float32) {
	r.m.killHeard(entity.KillMarks{Sess: r.anna, ServerID: r.pa.CharServerID, Name: r.pa.Name, Weight: 1, Group: group,
		At60: true, Group60: group60, NPCRow: 901, NPCName: "drake", World: 0, SpawnX: x, SpawnZ: z})
}

// A level 60 earns no kill mark anywhere until a GM opens a camp zone to 60s (#hotzone level60 <id>
// on): then its creatures roll for it as for an earner, the group's 60s counting for the group
// bonus there (for those below 60 too). The controls: the same zone before it is opened, a kill
// outside it, the zone turned off; a quest zone cannot be opened.
func TestKillMarksAtSixtyOnlyWhereAZoneIsOpenToThem(t *testing.T) {
	r := newMarksRig(t)
	r.pa.Level = 60
	r.kill60(0, 0, 5010, 5010)
	if r.rolls != 0 || r.marks().Marks != 0 {
		t.Fatalf("no zone: %d rolls, %d marks", r.rolls, r.marks().Marks)
	}
	r.pa.X, r.pa.Z = 5000, 5000
	r.m.hotZoneCommand(r.anna, r.pa, "here 100 Drake Camp")
	r.kill60(0, 0, 5010, 5010)
	if r.rolls != 0 || r.marks().Marks != 0 {
		t.Fatalf("a Hot zone not open to 60s: %d rolls, %d marks", r.rolls, r.marks().Marks)
	}
	r.drain()
	r.m.hotZoneCommand(r.anna, r.pa, "level60 1 on")
	if got := r.msgs(r.anna); !hasMsg(got, "chat:Level 60s earn here: 1 on: Drake Camp, +35%, level 60 too, world 0 (5000, *") {
		t.Fatalf("#hotzone level60 on: %q", got)
	}
	r.kill60(0, 2, 5010, 5010) // two 60s of its group near: x1.2, in the zone x1.35 (a new zone's default)
	if r.rolls != 1 || r.marks().Marks != 1 {
		t.Fatalf("an open zone: %d rolls, %d marks", r.rolls, r.marks().Marks)
	}
	var bonus float64
	r.db.QueryRow(`SELECT bonus FROM charmarks_log ORDER BY id DESC LIMIT 1`).Scan(&bonus)
	if bonus < 1.619 || bonus > 1.621 {
		t.Fatalf("a 60's kill in an open zone: bonus %v, want 1.62", bonus)
	}
	r.kill60(0, 0, 9000, 9000)
	if r.rolls != 1 {
		t.Fatal("a 60's kill outside the zone rolled")
	}
	// one below 60 in the open zone counts its 60s too
	r.pa.Level = 30
	r.kill(0, 5010, 5010)
	r.db.QueryRow(`SELECT bonus FROM charmarks_log ORDER BY id DESC LIMIT 1`).Scan(&bonus)
	if bonus < 1.349 || bonus > 1.351 {
		t.Fatalf("a level 30 alone in the open zone: bonus %v", bonus)
	}
	r.m.killHeard(entity.KillMarks{Sess: r.anna, ServerID: r.pa.CharServerID, Name: r.pa.Name, Weight: 1, Group: 0, Group60: 3,
		NPCRow: 900, NPCName: "orc pawn", World: 0, SpawnX: 5010, SpawnZ: 5010})
	r.db.QueryRow(`SELECT bonus FROM charmarks_log ORDER BY id DESC LIMIT 1`).Scan(&bonus)
	if bonus < 1.754 || bonus > 1.756 {
		t.Fatalf("a level 30 with three 60s in the open zone: bonus %v, want 1.755", bonus)
	}
	r.pa.Level = 60
	rolled := r.rolls
	r.m.hotZoneCommand(r.anna, r.pa, "off 1")
	r.kill60(0, 0, 5010, 5010)
	if r.rolls != rolled {
		t.Fatal("a 60's kill rolled in a zone turned off")
	}
	r.m.hotZoneCommand(r.anna, r.pa, "quests homelands/erudite Erudite quests")
	r.drain()
	r.m.hotZoneCommand(r.anna, r.pa, "level60 2 on")
	if got := r.msgs(r.anna); !hasMsg(got, "chat:"+hotZone60Camp) {
		t.Fatalf("a quest zone opened to 60s: %q", got)
	}
	zones, _ := r.db.HotZones()
	if len(zones) != 2 || !zones[0].Level60 || zones[1].Level60 {
		t.Fatalf("zones %+v", zones)
	}
}

// /marks says the marks, the hour, the ring, the rank and the zones; at 60 not reliving, the line
// that says how to earn.
func TestMarksCommand(t *testing.T) {
	r := newMarksRig(t)
	r.db.AwardMarks(r.pa.CharServerID, database.MarksAward{Source: "quest", Base: 1960, Bonus: 1}, r.now)
	r.db.SpendMarks(r.pa.CharServerID, 1548, "test", func(c *database.CharMarks) error { c.HeirloomRank = 2; return nil }, r.now)
	r.drain()
	r.m.marksCommand(r.anna, r.pa)
	got := r.msgs(r.anna)
	want := []string{"chat:Heirloom Marks: 412 (1,960 earned in all).", "chat:Kill marks this hour: 0 of 38.",
		"chat:Ring of Guises: none yet. Heirloom rank: 51.", "chat:Hot zones: none."}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("/marks: %q", got)
	}
	r.pa.Level = 60
	r.m.marksCommand(r.anna, r.pa)
	if !hasMsg(r.msgs(r.anna), "chat:"+marksAt60Line) {
		t.Error("a 60 is not told how to earn")
	}
}

// The Keeper's ring: rank 1 for 100, the next for 125; a guise not unlocked cannot be picked.
func TestRingOfGuisesRanks(t *testing.T) {
	r := newMarksRig(t)
	if ringPrice(1) != 100 || ringPrice(14) != 425 {
		t.Fatalf("prices %d, %d", ringPrice(1), ringPrice(14))
	}
	sum := 0
	for i := 1; i <= len(ringGuises); i++ {
		sum += ringPrice(i)
	}
	if sum != 3675 || len(ringGuises) != 14 {
		t.Fatalf("all 14: %d", sum)
	}
	r.db.AwardMarks(r.pa.CharServerID, database.MarksAward{Source: "gm", Base: 300, Bonus: 1}, r.now)
	r.m.buy(r.anna, r.pa, keeperRank) // the control: a rank is bought without an item
	if m := r.marks(); m.HeirloomRank != 1 || m.Marks != 150 {
		t.Fatalf("rank 1: %+v", m)
	}
	r.db.SpendMarks(r.pa.CharServerID, 0, "", func(c *database.CharMarks) error { c.RingRank, c.RingGuise = 2, ringGuises[0]; return nil }, r.now)
	r.m.setGuise(r.anna, r.pa, ringGuises[5]) // rank 6's: locked
	if r.marks().RingGuise != ringGuises[0] {
		t.Fatal("a locked guise was set")
	}
	r.m.setGuise(r.anna, r.pa, ringGuises[1])
	if r.marks().RingGuise != ringGuises[1] {
		t.Fatal("an unlocked guise was not set")
	}
	total := 0
	for _, p := range heirloomRankPrices {
		total += p
	}
	if total != 3650 {
		t.Errorf("the ranks: %d in all", total)
	}
}

// A tome: refused below 60, while reliving, at the mastery cap; read at 60.
func TestMasteryTomeRefusals(t *testing.T) {
	p := &entity.Player{Level: 59}
	if tomeRefusal(p, entity.Mastery{}) != tomeNot60 {
		t.Fatal("below 60")
	}
	p.Level = 60
	if tomeRefusal(p, entity.Mastery{}) != "" {
		t.Fatal("the control: a 60")
	}
	if tomeRefusal(p, entity.Mastery{Points: entity.MasteryCap}) != tomeCapped {
		t.Fatal("at the cap")
	}
	p.Relive, p.Level = &entity.ReliveState{Level: 60}, 20
	if tomeRefusal(p, entity.Mastery{}) != tomeReliving {
		t.Fatal("reliving")
	}
	if tomeXP != 150*327448 {
		t.Errorf("the tome's experience %d", tomeXP)
	}
}

// Relive: refused while something worn (an heirloom aside) asks more than the level picked, naming it;
// at that level or higher it starts, the row active, the player at its level and its save the 60's;
// /relive end puts the 60 back with the row kept for Resume, which the menu then offers.
func TestReliveAtTheRehabilitators(t *testing.T) {
	r := newMarksRig(t)
	r.gw.OnReliveSave(r.m.saveRelive)
	r.pa.Level = 60
	n := func(v int) *int { return &v }
	items := database.NewItemPatternStore(r.db)
	if err := items.UpsertItemPattern(&database.ItemPatternRecord{PatternID: 900100, ItemName: "Plate of 55", EquipSlot: 5, LevelReq: 55,
		AC: n(300), ClassUse: 1, RaceUse: 32767, Rent: 1, MaxStack: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.db.Exec(`INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber)
		VALUES (?, 1, 100, 0, 900100, 5, 0, 0)`, r.pa.CharServerID); err != nil {
		t.Fatal(err)
	}
	if l, _ := r.m.reliveMenu(r.pa); len(l) != 1 || l[0] != reliveAnswer {
		t.Fatalf("a 60's menu: %q", l)
	}
	r.drain()
	r.m.start(r.anna, r.pa, "Rehabilitator Anja", 30, 0, 0)
	if r.pa.Reliving() || !hasMsg(r.msgs(r.anna), "box:Rehabilitator Anja: Take off what asks more than level 30 first: Plate of 55.") {
		t.Fatal("not refused for the plate")
	}
	r.m.start(r.anna, r.pa, "Rehabilitator Anja", 55, 0, 0)
	row, ok, _ := r.db.CharReliveOf(r.pa.CharServerID)
	if !r.pa.Reliving() || r.pa.Level != 55 || !ok || !row.Active || row.Level != 55 {
		t.Fatalf("level %d, row %+v %v", r.pa.Level, row, ok)
	}
	if lvl, _, _ := r.pa.SavedProgress(); lvl != 60 {
		t.Fatalf("its save writes %d", lvl)
	}
	if l, _ := r.m.reliveMenu(r.pa); len(l) != 1 || l[0] != reliveEndAnswer {
		t.Fatalf("a reliving menu: %q", l)
	}
	r.m.reliveCommand(r.anna, r.pa, "end")
	row, _, _ = r.db.CharReliveOf(r.pa.CharServerID)
	if r.pa.Reliving() || r.pa.Level != 60 || row.Active || row.Level != 55 {
		t.Fatalf("ended: level %d, row %+v", r.pa.Level, row)
	}
	if l, _ := r.m.reliveMenu(r.pa); len(l) != 2 || l[1] != "Resume at 55" {
		t.Fatalf("after: %q", l)
	}
}
