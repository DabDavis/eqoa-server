package handlers

import (
	"fmt"
	"math/rand"
	"net"
	"os"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

func holidayTestFixture(t *testing.T) (*questFixture, *TriggerEngine) {
	t.Helper()
	file := os.Getenv("EQOA_EVENT_SQL")
	if file == "" {
		t.Skip("requires the selected Heirloom content fixture")
	}
	sql, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	f := newQuestFixture(t)
	if _, err = f.db.Exec(string(sql)); err != nil {
		t.Fatal(err)
	}
	f.exec("UPDATE gmseasons SET active=1 WHERE name='Thanksgiving'")
	if err = f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	w := f.tw.w
	w.LoadNPCs(database.NewNPCStore(f.db))
	if _, err = w.LoadClassTemplates(f.db); err != nil {
		t.Fatal(err)
	}
	if _, _, err = w.LoadSpawnOnly(f.db); err != nil {
		t.Fatal(err)
	}
	e := NewTriggerEngine(f.db, w, f.tw.mgr, f.inv)
	triggerEngine.Store(e)
	t.Cleanup(func() { triggerEngine.CompareAndSwap(e, nil) })
	if err = e.Reload(); err != nil {
		t.Fatal(err)
	}
	f.tw.h.SetDialogLines(f.db.NPCDialog)
	f.tw.p.HP, f.tw.p.MaxHP, f.tw.p.Level = 20000, 20000, 60
	return f, e
}

func TestThanksgivingJourney(t *testing.T) {
	f, e := holidayTestFixture(t)
	w := f.tw.w
	player := f.tw.p
	interact := func(id entity.EntityID) []string {
		t.Helper()
		n := w.NPCs[id]
		if n == nil || n.Dead || n.SpawnOnly || n.NPCType&128 == 0 {
			t.Fatalf("not a talkable actor %d: %+v", id, n)
		}
		player.World, player.X, player.Y, player.Z = n.World, n.X+1, n.Y, n.Z
		obj := f.tw.add(2, n)
		f.tw.talk(obj)
		m := f.msgs()
		if len(m) == 0 {
			t.Fatalf("interact %s did not answer", n.Name)
		}
		return m
	}
	answer := func(id entity.EntityID) []string {
		t.Helper()
		m := interact(id)
		if box := boxID(m); box != 0 {
			f.tw.pick(box, 0)
			m = append(m, f.msgs()...)
		}
		return m
	}
	check := func(step, done int) {
		t.Helper()
		s, d := f.progress(turkeyQuest)
		if s != step || d != done {
			t.Fatalf("quest %d/%d, expected %d/%d", s, d, step, done)
		}
	}
	pay := func() int {
		m, err := f.db.CharMarksOf(f.sid)
		if err != nil {
			t.Fatal(err)
		}
		return m.Marks
	}
	for _, id := range []entity.EntityID{197602, 197603} {
		m := interact(id)
		if len(m) == 0 {
			t.Fatal("silent courtier")
		}
	}
	answer(197601)
	check(1, 0)
	// Both field commanders can offer work to a level 60. Finish one full six-kill circuit through the quest engine.
	for _, v := range []struct {
		id     entity.EntityID
		quest  int
		target string
		x, z   float32
	}{{197604, 109911, "hill giant", 13712, 24281}, {197605, 109912, "deckhand", 25410, 6519}} {
		answer(v.id)
		if s, _ := f.progress(v.quest); s != 1 {
			t.Fatalf("no patrol %d", v.quest)
		}
		credit := entity.KillCredit{NPCID: 701, NPCSpawn: 701, NPCName: v.target, World: 0, X: v.x, Z: v.z, Players: []entity.KillPlayer{{ServerID: f.sid, Session: f.tw.s}}}
		outside := credit
		outside.World = 2
		f.qe.killed(outside)
		for j := 0; j < 5; j++ {
			f.qe.killed(credit)
		}
		if s, _ := f.progress(v.quest); s != 1 {
			t.Fatal("out-of-camp kill counted")
		}
		f.qe.killed(credit)
		if s, _ := f.progress(v.quest); s != 2 {
			t.Fatal("missing 6th patrol kill")
		}
		answer(v.id)
		if s, d := f.progress(v.quest); s != 0 || d != 1 {
			t.Fatal("unpaid patrol")
		}
	}
	if pay() != 14 {
		t.Fatalf("two patrols paid %d", pay())
	}
	for i := 0; i < 3; i++ {
		answer(197604)
		for j := 0; j < 6; j++ {
			f.qe.killed(entity.KillCredit{NPCID: 701, NPCSpawn: 701, NPCName: "hill giant", World: 0, X: 13712, Z: 24281, Players: []entity.KillPlayer{{ServerID: f.sid, Session: f.tw.s}}})
		}
		answer(197604)
	}
	if pay() != 35 {
		t.Fatalf("five patrols from zero should fund the tribute, got %d", pay())
	}
	f.exec("UPDATE charmarks SET marks=34 WHERE serverid=?", f.sid)
	if f.qe.turkeyPatrolOpen(f.sid) {
		t.Fatal("spending other marks reopened a sixth patrol payment")
	}
	answer(197601)
	check(1, 0)
	if pay() != 34 {
		t.Fatal("short collection debited")
	}
	f.exec("UPDATE charmarks SET marks=35 WHERE serverid=?", f.sid)
	// Force a failing progress write. Both the ledger and journal must roll back.
	f.exec("CREATE TRIGGER fail_pay BEFORE UPDATE ON charquests WHEN NEW.questid=109910 AND NEW.step=2 BEGIN SELECT RAISE(ABORT,'test'); END")
	answer(197601)
	check(1, 0)
	if pay() != 35 {
		t.Fatal("failed payment lost marks")
	}
	f.exec("DROP TRIGGER fail_pay")
	answer(197601)
	check(2, 0)
	if pay() != 0 {
		t.Fatalf("35 mark debit left %d", pay())
	}
	answer(197606)
	check(3, 0)
	f.own(envarArmor, 9)
	answer(197601)
	check(3, 0)
	if f.held(envarArmor) != 9 {
		t.Fatal("took nine armor")
	}
	// Simulate the tenth unit joining its existing client stack.
	if _, err := GiveItemNow(f.tw.s, w, f.inv, mustHolidayPattern(t, f, envarArmor), 1); err != nil {
		t.Fatal(err)
	}
	answer(197601)
	check(4, 0)
	if f.held(envarArmor) != 0 {
		t.Fatal("armor not consumed")
	}
	cross := func() {
		player.World, player.Y, player.Z = 0, 54.1, 15760
		player.X = 25225
		f.qe.checkReach()
		player.X = 25217
		f.qe.checkReach()
	}
	player.World, player.X, player.Y, player.Z = 0, 25217, 54.1, 15760
	f.qe.checkReach()
	check(4, 0)
	now := time.Now()
	e.now = func() time.Time { return now }
	cross()
	check(5, 0)
	if len(e.runs) != 1 {
		t.Fatalf("expected one encounter, have %d", len(e.runs))
	}
	e.StopRuns(340)
	f.qe.checkStranded(now)
	f.qe.checkStranded(now.Add(strandGrace + time.Second))
	check(4, 0)
	cross()
	check(5, 0)
	// A second player crosses after the curtain went up and joins the same raid.
	guest := &database.CharacterRecord{CharName: "Carver", AccountID: 2}
	if err := database.NewSQLiteCharacterStore(f.db).Create(guest); err != nil {
		t.Fatal(err)
	}
	f.exec("INSERT INTO charquests(serverid,questid,step) VALUES(?,?,4)", guest.ServerID, turkeyQuest)
	mate := &entity.Player{ServerID: 2009, CharServerID: guest.ServerID, Name: "Carver", Level: 60, HP: 25000, MaxHP: 25000, X: 25225, Y: 54.1, Z: 15760}
	ms := f.tw.mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 10), Port: 7010}, 10, 1, 1, 2, true)
	w.Players[mate.ServerID] = mate
	ms.SetECSEntity(mate)
	ms.SetCharacterInWorld(true)
	f.qe.Journal(guest.ServerID)
	f.qe.checkReach()
	mate.X = 25217
	f.qe.checkReach()
	var guestStep int
	if err := f.db.QueryRow("SELECT step FROM charquests WHERE serverid=? AND questid=?", guest.ServerID, turkeyQuest).Scan(&guestStep); err != nil || guestStep != 5 || len(e.runs) != 1 {
		t.Fatalf("second raid member step %d, runs %d, %v", guestStep, len(e.runs), err)
	}
	var king, santa, grinch *entity.NPC
	now = now.Add(3 * time.Second)
	e.Tick()
	for _, s := range e.runs[0].spawns {
		switch s.name {
		case "Turkey King":
			king = w.NPCs[s.id]
		case "Santa Claus of Freeport":
			santa = w.NPCs[s.id]
		case "The Grinch":
			grinch = w.NPCs[s.id]
		}
	}
	if king == nil || santa == nil || grinch == nil {
		t.Fatalf("missing run actors: %+v", e.runs[0].spawns)
	}
	for i := 0; i < 7; i++ {
		now = now.Add(2 * time.Second)
		e.Tick()
	}
	if santa.HP != 1 || grinch.HP != 1 || santa.NPCType&128 == 0 || grinch.NPCType&128 == 0 {
		t.Fatalf("knockdowns %d/%d", santa.HP, grinch.HP)
	}
	for i := 0; i < 12 && !king.EventAggro; i++ {
		now = now.Add(5 * time.Second)
		e.Tick()
	}
	if !grinch.Dead || !grinch.NoLoot || santa.Dead || !king.EventAggro || king.HP != 2000000 || king.NPCType&128 != 0 {
		t.Fatalf("bad raid handoff: Grinch dead %v, Santa dead %v, King hp %d hostile %v", grinch.Dead, santa.Dead, king.HP, king.EventAggro)
	}
	check(5, 0)
	kill := entity.KillCredit{NPCName: "Turkey King", NPCID: king.ServerID, OriginalRow: 197601, Died: now, World: 0, Players: []entity.KillPlayer{{ServerID: f.sid, Session: f.tw.s}, {ServerID: guest.ServerID, Session: ms}}}
	wrong := kill
	wrong.OriginalRow = 190954
	f.qe.killed(wrong)
	check(5, 0)
	king.HP, king.Dead, king.DeathTime = 0, true, now
	f.qe.killed(kill)
	check(6, 0)
	e.Tick()
	if err := f.db.QueryRow("SELECT step FROM charquests WHERE serverid=? AND questid=?", guest.ServerID, turkeyQuest).Scan(&guestStep); err != nil || guestStep != 6 {
		t.Fatalf("second raider missed victory credit %d %v", guestStep, err)
	}
	var returning bool
	for _, sp := range e.runs[0].spawns {
		if sp.name == "Santa Claus of Freeport" && w.NPCs[sp.id] != nil && w.NPCs[sp.id] != santa {
			returning = true
		}
	}
	if !returning {
		t.Fatal("Santa did not reappear after victory")
	}
	items, err := f.inv.GetNPCItems(king.NPCID)
	if err != nil || len(items) != 20 {
		t.Fatalf("corpse %d rows, %v", len(items), err)
	}
	for _, i := range items {
		if i.Pattern.PatternID != stuffing || i.StackLeft != 1 {
			t.Fatal("wrong corpse row")
		}
	}
	answer(190715)
	check(6, 0)
	loot := NewNPCWindowHandler(f.tw.mgr, f.inv)
	loot.SetGameWorld(w)
	player.World, player.X, player.Y, player.Z = king.World, king.X+1, king.Y, king.Z
	if !loot.OpenCorpse(f.tw.s, king) {
		t.Fatal("the raid corpse did not open")
	}
	take := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeClientLoot)
	take.SetData([]byte{0, 0, 0, 0, 1, 0, 0, 0})
	if err := loot.HandleLootItem(f.tw.s, take); err != nil || f.held(stuffing) != 1 {
		t.Fatalf("loot one parcel: %v", err)
	}
	ls := loot.loots()
	if c := ls.corpses[king.ServerID]; c == nil || len(ls.viewLocked(king.ServerID, c, guest.ServerID)) != 19 {
		t.Fatal("the raid must have 19 parcels remaining")
	}
	loot.closeLoot(f.tw.s)
	answer(190715)
	check(0, 1)
	if f.held(stuffing) != 0 || f.held(turkeyClub) != 1 {
		t.Fatal("Santa exchange")
	}
	f.tw.talk(f.tw.add(2, w.NPCs[190715]))
	f.msgs()
	check(0, 1)
	if f.held(turkeyClub) != 1 {
		t.Fatal("duplicate reward")
	}
	club := mustHolidayPattern(t, f, turkeyClub)
	if club.ItemDesc != "This procs.. something, but you're not sure what" || club.WeaponDamage != 790 || club.AttackType != 3 || club.EquipSlot != 14 || club.Trade != 1 || club.Lore != 1 || club.ClassUse != 32767 || club.RaceUse != 1023 {
		t.Fatalf("wrong club %+v", club)
	}
	var spent int
	f.db.QueryRow("SELECT count(*) FROM charmarks_log WHERE source='spend' AND ref='Thanksgiving tribute'").Scan(&spent)
	if spent != 1 {
		t.Fatal("duplicate payment")
	}
}

func mustHolidayPattern(t *testing.T, f *questFixture, id int) database.ItemPatternRecord {
	t.Helper()
	p, err := database.NewItemPatternStore(f.db).GetItemPattern(id)
	if err != nil {
		t.Fatal(err)
	}
	return *p
}

func TestThanksgivingArmorRollAndClose(t *testing.T) {
	f, _ := holidayTestFixture(t)
	r := NewSeasonRewards(f.db, f.tw.w, f.tw.mgr, f.inv)
	rng := rand.New(rand.NewSource(11262026))
	r.roll = rng.Float64
	drops := 0
	r.drop = func(_ entity.EntityID, _ time.Time, item database.CharInventoryEntry) {
		if item.Pattern.PatternID == envarArmor {
			drops += item.StackLeft
		}
	}
	var spawn int
	var name string
	f.db.QueryRow("SELECT spawn,name FROM thanksgiving_envar_trash ORDER BY spawn LIMIT 1").Scan(&spawn, &name)
	k := entity.KillCredit{NPCName: name, NPCSpawn: entity.EntityID(spawn), NPCID: 781, World: 1, Players: []entity.KillPlayer{{ServerID: f.sid}, {ServerID: 92}, {ServerID: 93}}}
	for i := 0; i < 100000; i++ {
		r.Killed(k)
	}
	if drops < 19500 || drops > 20500 {
		t.Fatalf("armor rolls %d/100000", drops)
	}
	t.Logf("%d shared drops / 100000 eligible kills (three players)", drops)
	r.roll = func() float64 { return 0 }
	original := drops
	for _, v := range []struct {
		world uint8
		id    entity.EntityID
		name  string
	}{{0, k.NPCSpawn, name}, {1, 0, name}, {1, 119599, "quintessence of Veeshan"}, {1, k.NPCSpawn, "High Priest Vorin"}} {
		bad := k
		bad.World, bad.NPCSpawn, bad.NPCName = v.world, v.id, v.name
		r.Killed(bad)
	}
	if drops != original {
		t.Fatal("armor dropped from boss/copy/outside Envar")
	}
	f.own(envarArmor, 9)
	f.own(stuffing, 1)
	f.exec("UPDATE gmseasons SET active=0 WHERE name='Thanksgiving'")
	r.Refresh()
	r.Killed(k)
	if drops != original || f.held(envarArmor) != 0 || f.held(stuffing) != 1 {
		t.Fatal("season cleanup or drop gating")
	}
	err := func() error { _, e := f.inv.AddCharItem(f.sid, envarArmor, 1, 0, 0, 0); return e }()
	if err == nil {
		t.Fatal("expired corpse armor could enter a pack")
	}
}

func TestThanksgivingOperatorSwitches(t *testing.T) {
	f, e := holidayTestFixture(t)
	f.exec("UPDATE gmseasons SET active=0 WHERE name='Christmas'")
	f.own(envarArmor, 3)
	if _, err := f.inv.AddCharItem(f.sid, envarArmor, 8, 0, 0, database.LocBank); err != nil {
		t.Fatal(err)
	}
	f.own(stuffing, 1)
	fire := func(slot int) {
		t.Helper()
		if lines, err := e.Fire(slot, TriggerFirer{Name: "test GM", Sess: f.tw.s, Player: f.tw.p, Quiet: true}); err != nil {
			t.Fatalf("GM trigger %d: %v %v", slot, lines, err)
		}
	}
	active := func(on bool) {
		t.Helper()
		if got, err := f.db.SeasonActive("Thanksgiving"); err != nil || got != on {
			t.Fatalf("season %v %v", got, err)
		}
		var enabled int
		f.db.QueryRow("SELECT count(*) FROM quests WHERE source='thanksgiving-2026' AND enabled=1").Scan(&enabled)
		if on && enabled != 3 || !on && enabled != 0 {
			t.Fatalf("enabled quests %d", enabled)
		}
		for _, id := range []entity.EntityID{197601, 197602, 197603, 197604, 197605, 197606, 190715, 190712} {
			n := f.tw.w.NPCs[id]
			if n == nil || n.SpawnOnly == on {
				t.Fatalf("actor %d visible in wrong season (%v)", id, on)
			}
		}
	}
	fire(342)
	active(false)
	if f.held(envarArmor) != 0 || f.held(stuffing) != 1 {
		t.Fatal("season close did not purge only the armor across pack and bank")
	}
	if _, err := e.Fire(340, TriggerFirer{Quiet: true}); err != ErrTriggerOff {
		t.Fatalf("closed encounter fired: %v", err)
	}
	fire(341)
	active(true)
	fire(340)
	if !e.RunningNamed("The King's Last Thanksgiving") {
		t.Fatal("encounter failed to reopen")
	}
	fire(342)
	active(false)
	if e.RunningNamed("The King's Last Thanksgiving") {
		t.Fatal("live raid remained after season end")
	}
}

func TestThanksgivingGateCrossingConditions(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		a, b, z, y float32
		w          uint8
		gap        time.Duration
		want       bool
	}{{25226, 25217, 15760, 54, 0, time.Second, true}, {25217, 25226, 15760, 54, 0, time.Second, false}, {25226, 25217, 15790, 54, 0, time.Second, false}, {25226, 25217, 15760, 80, 0, time.Second, false}, {25226, 25217, 15760, 54, 1, time.Second, false}, {25800, 25217, 15760, 54, 0, time.Second, false}, {25226, 25217, 15760, 54, 0, time.Minute, false}} {
		var s westGateSample
		if s.crosses(tc.w, tc.a, tc.y, tc.z, now) {
			t.Fatal("initial sight fired")
		}
		if got := s.crosses(tc.w, tc.b, tc.y, tc.z, now.Add(tc.gap)); got != tc.want {
			t.Fatal(fmt.Sprint(tc))
		}
	}
}

// The event's kill rules hold for its own quests alone (thanksgivingQuests,
// read with the catalog): a shard whose quest 109910 or 109911 is another
// quest keeps that quest's own targets. The control: with the event's quests
// loaded, the King's kill step takes only the event's King.
func TestThanksgivingKillRulesOnlyForTheEventsQuests(t *testing.T) {
	old := thanksgivingQuests.Load()
	t.Cleanup(func() { thanksgivingQuests.Store(old) })
	rat := entity.KillCredit{NPCName: "a rat", NPCSpawn: 5, World: 2, X: 100, Z: 100}
	none := map[int]bool{}
	thanksgivingQuests.Store(&none)
	for _, id := range []int{turkeyQuest, 109911} {
		st := &database.QuestStep{Quest: id, Kind: database.StepKill, Target: "a rat"}
		if !stepKill(st, rat) {
			t.Errorf("another shard's quest %d: its rat's kill not taken", id)
		}
	}
	loaded := map[int]bool{turkeyQuest: true, 109911: true, 109912: true}
	thanksgivingQuests.Store(&loaded)
	if stepKill(&database.QuestStep{Quest: turkeyQuest, Kind: database.StepKill, Target: "a rat"}, rat) {
		t.Error("the event's King step took a rat")
	}
}
