package handlers

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// Tallon Zek's raid as eqoa.forumotion.com t218 tells it (tools/tallon_zek_20261118.py):
// the spawn's engage window, the portal its key's bearer closes, the sentry's flag,
// and the tool's own triggers walked end to end on an applied copy of a shard.

// A spawn with an engage window (Tallon's 20 minutes): not fought, it leaves at the
// window's end, its stay of two hours notwithstanding; fought, it stays past even its
// stay while the fight lasts, and a fight's end alive starts the window again.
func TestTriggerSpawnEngageWindow(t *testing.T) {
	w := newTrigWorld(t)
	w.set(20, "Window", "near", 50, `[{"do":"spawn","npc":"Orc Pawn","minutes":120,"engage":20}]`)
	if _, err := w.fire(20); err != nil {
		t.Fatal(err)
	}
	w.at(19 * time.Minute)
	if len(w.spawned()) != 1 {
		t.Fatal("gone inside its window")
	}
	w.at(21 * time.Minute)
	if len(w.spawned()) != 0 {
		t.Fatal("not engaged in 20 minutes, it stayed")
	}

	// fought at 10 minutes, the fight over at 30: the window again from there
	if _, err := w.fire(20); err != nil {
		t.Fatal(err)
	}
	w.at(22 * time.Minute)
	n := w.spawned()[0]
	n.Threats = map[entity.EntityID]int64{1: 5}
	w.at(30 * time.Minute)
	n.Threats = nil
	w.at(31 * time.Minute)
	w.at(50 * time.Minute)
	if len(w.spawned()) != 1 {
		t.Fatal("gone 19 minutes after its fight ended")
	}
	w.at(52 * time.Minute)
	if len(w.spawned()) != 0 {
		t.Fatal("its window again after the fight: it stayed past it")
	}

	// in a fight past its whole stay: kept until the fight ends
	if _, err := w.fire(20); err != nil {
		t.Fatal(err)
	}
	w.at(53 * time.Minute)
	n = w.spawned()[0]
	n.Threats = map[entity.EntityID]int64{1: 5}
	w.at(53*time.Minute + 121*time.Minute)
	if len(w.spawned()) != 1 {
		t.Fatal("taken away mid-fight at its stay's end")
	}
	n.Threats = nil
	w.at(53*time.Minute + 122*time.Minute)
	if len(w.spawned()) != 0 {
		t.Fatal("its fight over past its stay: it stayed")
	}
	// a spawn with no window is not taken mid-fight at its stay either (Live
	// ticket 5308, 2026-11-19: it was, fight or none); its fight over, it goes
	// spawnFightGrace later
	w.set(21, "Plain", "near", 50, `[{"do":"spawn","npc":"Orc Pawn","minutes":10}]`)
	if _, err := w.fire(21); err != nil {
		t.Fatal(err)
	}
	w.at(176 * time.Minute)
	w.spawned()[0].Threats = map[entity.EntityID]int64{1: 5}
	w.at(187 * time.Minute)
	if len(w.spawned()) != 1 {
		t.Fatal("a spawn with no window was taken mid-fight at its ten minutes")
	}
	w.spawned()[0].Threats = nil
	w.at(187*time.Minute + spawnFightGrace/2)
	if len(w.spawned()) != 1 {
		t.Fatal("a spawn with no window went before the grace after its fight")
	}
	w.at(187*time.Minute + spawnFightGrace + time.Second)
	if len(w.spawned()) != 0 {
		t.Fatal("a spawn with no window stayed past its fight and the grace")
	}
	// the control: one past its stay that never fought goes at once
	w.set(22, "Plain2", "near", 50, `[{"do":"spawn","npc":"Orc Pawn","minutes":10}]`)
	if _, err := w.fire(22); err != nil {
		t.Fatal(err)
	}
	w.at(200 * time.Minute)
	w.at(211 * time.Minute)
	if len(w.spawned()) != 0 {
		t.Fatal("a spawn with no window outstayed its ten minutes out of any fight")
	}
}

// tzRig is the portal rig's Anna (the key's bearer), Cora and Bert at level 60 by the
// Zek camp's portal, a quest engine holding quest 101327 as the tool writes it.
type tzRig struct {
	*portalRig
	db         *database.SQLiteDB
	inv        *database.InventoryStore
	key, bent  int
	portal     *entity.NPC
	qid, steps int
}

func newTZRig(t *testing.T) *tzRig {
	t.Helper()
	r := &tzRig{portalRig: newPortalRig(t), qid: 101327}
	db, err := database.NewSQLite(t.TempDir() + "/tz.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	r.db, r.inv = db, database.NewInventoryStore(db)
	for _, p := range []*entity.Player{r.pa, r.pc, r.pb} {
		rec := &database.CharacterRecord{CharName: p.Name, AccountID: 1}
		if err := database.NewSQLiteCharacterStore(db).Create(rec); err != nil {
			t.Fatal(err)
		}
		p.CharServerID, p.Level = rec.ServerID, 60
	}
	for _, s := range []*session.Session{r.anna, r.cora, r.bert} {
		s.SetInventoryList(newInvList(nil))
	}
	item := func(name string) int {
		res, err := db.Exec(`INSERT INTO itempattern
			(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype, weapondamage, unk5, levelreq, maxstack,
			 maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color)
			VALUES (0, 0, -441710123, 0, -1, 0, 1, 0, 0, 0, 0, 0, 0, 1, 0, 50, 0, 0, 0, 1, 0, 1, ?, '', 0, 0)`, name)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return int(id)
	}
	r.key, r.bent = item("Tallon's Key"), item("Tallon's Bent Key")
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel, repeatable) VALUES (101327, 'Tallon Zek', 1, 'Portal of Tallon', 60, 60, 1)`)
	exec(fmt.Sprintf(`INSERT INTO queststeps (questid, step, kind, target, items, count, world, x, y, z, radius, journal, says, rewarditems) VALUES
		(101327, 1, 'kill', 'Zek troop', '', 4, 0, 0, 0, 0, 15, 'Kill the Zek troops.', '', ''),
		(101327, 2, 'kill', 'Zek Sergeant', '', 1, 0, 0, 0, 0, 15, 'Kill the Zek Sergeant.', '', ''),
		(101327, 3, 'show', 'Portal of Tallon', '%d', 1, 0, 0, 0, 0, 15, 'Go last with the key.', 'The portal flares.', ''),
		(101327, 4, 'reach', '', '%d', 1, 5, 4715, 185, 7500, 110, 'Up the mountain.', '', '%d'),
		(101327, 5, 'kill', 'Zek sentry', '', 1, 0, 0, 0, 0, 15, 'Kill the Zek Sentry.', '', ''),
		(101327, 6, 'kill', 'Tallon Zek', '', 1, 0, 0, 0, 0, 15, 'Kill Tallon Zek.', '', '')`, r.key, r.key, r.bent))
	qe, err := NewQuestEngine(db, r.gw, r.sm, r.inv, database.NewSpellbookStore(db))
	if err != nil {
		t.Fatal(err)
	}
	qe.Start()
	t.Cleanup(qe.Stop)
	// the portal, a trigger's copy at the camp; the three by it
	r.portal = &entity.NPC{ServerID: 9301, Name: "Portal of Tallon", World: 0, Level: 60, NPCType: 128, Temporary: true,
		SpawnX: 20263.3, SpawnY: 89.5, SpawnZ: 18217.6, X: 20263.3, Y: 89.5, Z: 18217.6, HP: 1, MaxHP: 1}
	r.gw.NPCs[r.portal.ServerID] = r.portal
	r.gw.SpatialIndex.Insert(r.portal.ServerID, r.portal.X, r.portal.Z)
	for i, p := range []*entity.Player{r.pa, r.pc, r.pb} {
		p.World = 0
		r.gw.UpdatePosition(p.ServerID, 20263.3+float32(i)*2, 89.5, 18220, 0)
	}
	r.drain()
	return r
}

func (r *tzRig) own(p *entity.Player, pattern int) {
	r.t.Helper()
	if _, err := r.inv.AddCharItem(p.CharServerID, pattern, 1, 0, 0, 0); err != nil {
		r.t.Fatal(err)
	}
	rows, err := r.inv.GetCharacterItems(p.CharServerID)
	if err != nil {
		r.t.Fatal(err)
	}
	r.gw.FindSessionByEntityID(p.ServerID).SetInventoryList(newInvList(rows))
}

func (r *tzRig) held(p *entity.Player, pattern int) int {
	var n int
	r.db.QueryRow(`SELECT COALESCE(SUM(stackleft), 0) FROM charinventory WHERE serverid = ? AND patternid = ?`, p.CharServerID, pattern).Scan(&n)
	return n
}

func (r *tzRig) on(p *entity.Player, step int) {
	r.t.Helper()
	if _, err := r.db.Exec(`INSERT OR REPLACE INTO charquests (serverid, questid, step, slot) VALUES (?, ?, ?, 1)`, p.CharServerID, r.qid, step); err != nil {
		r.t.Fatal(err)
	}
	if err := Quests().Reload(); err != nil {
		r.t.Fatal(err)
	}
}

func (r *tzRig) step(p *entity.Player) int {
	var step int
	if err := r.db.QueryRow(`SELECT step FROM charquests WHERE serverid = ? AND questid = ?`, p.CharServerID, r.qid).Scan(&step); err != nil {
		return -1
	}
	return step
}

// use talks to the portal and answers its two questions: "use portal", then last
// (option 0, "Yes, I am the last") or not (1). It returns what s was sent after.
func (r *tzRig) use(s *session.Session, last bool) []string {
	r.t.Helper()
	r.drain()
	r.talk(s, r.portal)
	got := r.msgs(s)
	if !hasMsg(got, "ask#"+fmt.Sprint(boxID(got))+":"+raidPortalWaning+"|"+raidPortalUse+"|"+raidPortalLeave) {
		r.t.Fatalf("the portal's first question: %q", got)
	}
	r.pick(s, boxID(got), 0)
	got = r.msgs(s)
	if !hasMsg(got, "ask#"+fmt.Sprint(boxID(got))+":"+raidPortalLastAsk+"|"+raidPortalLast+"|"+raidPortalMore) {
		r.t.Fatalf("the portal's second question: %q", got)
	}
	opt := byte(1)
	if last {
		opt = 0
	}
	r.pick(s, boxID(got), opt)
	return r.msgs(s)
}

// The post's portal: Cora (no key, no quest) and Bert ("Yes", no key) are sent through
// and the portal stays; Anna, the bearer, answering "No" is sent nowhere; her "Yes"
// sends her through, closes it, and does her step 3 although her quest had not
// counted steps 1-2 (Heirloom, 2026-11-18: "Portal of Tallon has nothing to say"); the
// key kept for the ramp.
func TestTallonsPortalTheBearerClosesItLast(t *testing.T) {
	r := newTZRig(t)
	r.own(r.pa, r.key)
	r.on(r.pa, 1)

	got := r.use(r.cora, false)
	if !hasMsg(got, "teleport") || r.pc.World != 5 {
		t.Fatalf("Cora's 'No, there are more': not sent (%q, world %d)", got, r.pc.World)
	}
	got = r.use(r.bert, true)
	if !hasMsg(got, "teleport") || !hasMsg(got, "chat:"+fmt.Sprintf(raidPortalStays, "Tallon's Key")) {
		t.Fatalf("Bert's 'Yes' without the key: %q", got)
	}
	if !r.gw.NPCAlive(r.portal.ServerID) {
		t.Fatal("a 'Yes' without the key closed the portal")
	}
	got = r.use(r.anna, false)
	if hasMsg(got, "teleport") || !hasMsg(got, "box:"+fmt.Sprintf(raidPortalWaitLast, "Tallon's Key")) || r.pa.World != 0 {
		t.Fatalf("the bearer's 'No': %q (world %d)", got, r.pa.World)
	}
	got = r.use(r.anna, true)
	if !hasMsg(got, "teleport") || r.pa.World != 5 || r.pa.X != 4750 || r.pa.Z != 7380 {
		t.Fatalf("the bearer's 'Yes': %q at world %d (%.1f, %.1f)", got, r.pa.World, r.pa.X, r.pa.Z)
	}
	if _, ok := r.gw.NPCs[r.portal.ServerID]; ok {
		t.Fatal("the portal stands after its bearer went last")
	}
	if s := r.step(r.pa); s != 4 {
		t.Fatalf("Anna's step after the portal: %d, want 4", s)
	}
	if n := r.held(r.pa, r.key); n != 1 {
		t.Fatalf("Anna holds %d keys: the portal keeps it for the ramp", n)
	}
}

// A bearer who never took the quest is given it at the portal, on its step 3, and
// goes on as one who had.
func TestTallonsPortalGivesTheBearerTheQuest(t *testing.T) {
	r := newTZRig(t)
	r.own(r.pa, r.key)
	if s := r.step(r.pa); s != -1 {
		t.Fatalf("Anna on step %d before", s)
	}
	r.use(r.anna, true)
	if s := r.step(r.pa); s != 4 {
		t.Fatalf("Anna's step: %d, want 4", s)
	}
}

// The sentry's death flags every player the kill counts for: on its step (5), on an
// earlier one, or without the quest, each is on step 6 after; one past it is left.
func TestTheZekSentrysDeathFlagsTheRaid(t *testing.T) {
	r := newTZRig(t)
	r.on(r.pa, 5) // as a GM, moved there by hand
	r.on(r.pc, 2)
	kp := func(s *session.Session, p *entity.Player) entity.KillPlayer {
		return entity.KillPlayer{Session: s, ServerID: p.CharServerID, Name: p.Name}
	}
	k := entity.KillCredit{NPCName: "Zek sentry", NPCID: 9400, Died: time.Now(),
		Players: []entity.KillPlayer{kp(r.anna, r.pa), kp(r.cora, r.pc), kp(r.bert, r.pb)}}
	q := Quests()
	q.killed(k)
	q.killFlags(k)
	for _, p := range []*entity.Player{r.pa, r.pc, r.pb} {
		if s := r.step(p); s != 6 {
			t.Errorf("%s's step after the sentry: %d, want 6", p.Name, s)
		}
	}
	// the control: another creature's death flags no one
	r.on(r.pb, 2)
	k.NPCName = "Zek troop"
	q.killFlags(k)
	if s := r.step(r.pb); s != 2 {
		t.Errorf("a troop's death moved Bert to %d", s)
	}
}

// The tool's rows walked on the triggers (EQOA_TZ_DB: a shard's copy the tool was
// applied to): four troops' deaths stand the sergeant, his the portal at the camp, the
// portal gone ends the run; the fortress spawns no sentry and waits on the standing
// one's death (the Rages crumbling then), or goes on at once when none stands; Tallon
// spoken to leaves after 20 minutes unfought.
func TestTallonZekToolDataWalk(t *testing.T) {
	src := os.Getenv("EQOA_TZ_DB")
	if src == "" {
		t.Skip("EQOA_TZ_DB names a shard's copy with tools/tallon_zek_20261118.py applied")
	}
	real, err := sql.Open("sqlite", "file:"+src+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer real.Close()
	w := newTrigWorld(t)
	type trow struct {
		slot        int
		name, scope string
		radius      float32
		world       int
		x, y, z     float32
		steps       string
	}
	var trigs []trow
	rows, err := real.Query(`SELECT slot, name, scope, radius, world, x, y, z, steps FROM gmtriggers WHERE name IN ('Zek Sergeant', 'Tallon''s Fortress')`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var r trow
		if err := rows.Scan(&r.slot, &r.name, &r.scope, &r.radius, &r.world, &r.x, &r.y, &r.z, &r.steps); err != nil {
			t.Fatal(err)
		}
		trigs = append(trigs, r)
	}
	rows.Close()
	if len(trigs) != 2 {
		t.Fatalf("%d of the two triggers", len(trigs))
	}
	id := entity.EntityID(9000)
	names := map[string]bool{}
	for _, tr := range trigs {
		w.setAt(tr.slot, tr.name, tr.scope, tr.radius, tr.world, tr.x, tr.y, tr.z, tr.steps, true)
		if probs := database.TriggerProblems(database.GMTrigger{Slot: tr.slot, Name: tr.name, Scope: tr.scope, World: tr.world, Steps: tr.steps}); len(probs) > 0 {
			t.Fatalf("trigger %q's problems: %v", tr.name, probs)
		}
		for _, n := range []string{"Zek Sergeant", "Portal of Tallon", "Tallon's Rage", "Zek captain",
			"Zek hall guard", "fortress golem", "Zek archer"} {
			if strings.Contains(tr.steps, `"npc":"`+n+`"`) && !names[n] {
				names[n] = true
				w.gw.NPCs[id] = &entity.NPC{ServerID: id, Name: n, HP: 1000, MaxHP: 1000, Level: 60, NPCType: 0x80,
					X: 4730, Y: 200, Z: 7600, World: 5, SpawnOnly: true}
				id++
			}
		}
	}
	var deaths int
	real.QueryRow(`SELECT count(*) FROM npcdeath WHERE name = 'zek troop' AND trigger = 'Zek Sergeant' AND chance = 100`).Scan(&deaths)
	if deaths != 1 {
		t.Fatal("no npcdeath row firing the sergeant's trigger")
	}
	if _, err := w.db.Exec(`INSERT INTO npcdeath (name, trigger, chance) VALUES ('zek troop', 'Zek Sergeant', 100)`); err != nil {
		t.Fatal(err)
	}
	tallon := &entity.NPC{ServerID: id, Name: "Tallon Zek", HP: 1000, MaxHP: 1000, Level: 65, NPCType: 0x80,
		X: 4703.5, Y: 162.4, Z: 7641, SpawnX: 4703.5, SpawnY: 162.4, SpawnZ: 7641, World: 5}
	w.gw.NPCs[id] = tallon
	id++
	if err := w.e.Reload(); err != nil {
		t.Fatal(err)
	}
	// the camp: four troops
	var troops []*entity.NPC
	for i := 0; i < 4; i++ {
		n := &entity.NPC{ServerID: id, Name: "Zek troop", HP: 100, MaxHP: 100, Level: 63, X: 20250 + float32(i)*10, Y: 89, Z: 18250, World: 0}
		w.gw.NPCs[id] = n
		troops = append(troops, n)
		id++
	}
	troopDies := func(n *entity.NPC, at time.Duration) {
		w.kill(n)
		w.e.Killed(entity.KillCredit{NPCName: n.Name, NPCID: n.ServerID})
		w.at(at)
	}
	for i, n := range troops[:3] {
		troopDies(n, time.Duration(i+1)*time.Second)
		if w.alive("Zek Sergeant") != 0 {
			t.Fatalf("the sergeant after %d troops", i+1)
		}
	}
	troopDies(troops[3], 4*time.Second)
	if w.alive("Zek Sergeant") != 1 {
		t.Fatal("no sergeant after the fourth troop")
	}
	w.kill(w.first("Zek Sergeant"))
	w.at(5 * time.Second)
	p := w.first("Portal of Tallon")
	if p == nil || p.World != 0 || p.X < 20200 || p.X > 20300 {
		t.Fatalf("the portal after the sergeant: %+v", p)
	}
	w.gw.DespawnNPC(p.ServerID) // its bearer went last (raidportal.go)
	w.at(6 * time.Second)
	if w.e.RunningNamed("Zek Sergeant") {
		t.Fatal("the sergeant's run outlived its portal")
	}

	// the fortress: the standing sentry at the door
	if strings.Contains(trigs[0].steps+trigs[1].steps, `"npc":"Tallon's ward"`) ||
		strings.Contains(trigs[0].steps+trigs[1].steps, `"do":"spawn","npc":"Zek sentry"`) {
		t.Fatal("the fortress's run still spawns a sentry or wards")
	}
	sentry := &entity.NPC{ServerID: id, Name: "Zek sentry", HP: 1000, MaxHP: 1000, Level: 64,
		X: 4737.8, Y: 218.3, Z: 7618.7, World: 5}
	w.gw.NPCs[id] = sentry
	id++
	rs, rp := w.join(10, "Raider", 4730, 7600, 5)
	if _, err := w.e.FireNamed("Tallon's Fortress", TriggerFirer{Name: rp.Name, Sess: rs, Player: rp, Quiet: true}); err != nil {
		t.Fatal(err)
	}
	if w.alive("Zek sentry") != 0 || w.alive("Tallon's Rage") != 5 {
		t.Fatalf("woken: spawned sentries %d, Rages %d", w.alive("Zek sentry"), w.alive("Tallon's Rage"))
	}
	w.at(7 * time.Second)
	if w.alive("Tallon's Rage") != 5 {
		t.Fatal("the Rages crumbled with the sentry alive")
	}
	w.kill(sentry)
	w.e.Killed(entity.KillCredit{NPCName: sentry.Name, NPCID: sentry.ServerID})
	w.at(8 * time.Second)
	if n := w.alive("Tallon's Rage"); n != 0 {
		t.Fatalf("%d Rages outlive the sentry", n)
	}
	if !w.e.Talk(rs, rp, tallon) {
		t.Fatal("Tallon's run did not answer the talk")
	}
	w.at(8*time.Second + 19*time.Minute)
	if w.alive("Tallon Zek") != 1 {
		t.Fatal("Tallon gone inside his window")
	}
	w.at(8*time.Second + 21*time.Minute)
	if w.alive("Tallon Zek") != 0 {
		t.Fatal("Tallon unfought for 20 minutes: still there")
	}
}

// An awaitkill with gone: met at once when no living NPC of its name stands in the
// run's reach (the sentry dead before the fortress woke); the control without it waits.
func TestTriggerAwaitKillGoneWhenNoneStands(t *testing.T) {
	for _, gone := range []bool{true, false} {
		w := newTrigWorld(t)
		w.set(20, "Gone", "near", 50, fmt.Sprintf(`[{"do":"awaitkill","npc":"Zek sentry","any":true,"gone":%v},{"do":"broadcast","text":"open"}]`, gone))
		if _, err := w.fire(20); err != nil {
			t.Fatal(err)
		}
		w.clear()
		w.at(time.Second)
		heard := strings.Join(yellow(t, w.ps), "|")
		if gone != strings.Contains(heard, "open") {
			t.Errorf("gone %v: heard %q", gone, heard)
		}
	}
}
