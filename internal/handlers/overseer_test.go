package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// Petition 1699 (Skywalker): "Overseer Yurgulch can be used to check whether
// Bertoxxulous, Uzrathixius, or Poxathixius spawns are up and available to kill".
// The Plane of Disease as it stands since pod_quests_20261112: Bertoxxulous only by
// his throne's trigger (a lock, an hour, then 3 days 16 hours of waits), the twins
// by their rows' 2-day respawns.

// overseerRows are Yurgulch's rows as tools/overseer_yurgulch_20261112.py writes
// them, in short.
func overseerRows() []database.NPCOverseerRow {
	return []database.NPCOverseerRow{
		{ID: 1, Overseer: "Overseer Yurgulch", Boss: "Bertoxxulous", World: -1, Trigger: "Bertoxxulous Rises", Ord: 1,
			Greeting: "Yurgulch watches!", Up: "%b up on throne!", Ready: "%b sleeps. Throne ready.", Resting: "Throne rests. Ready %t."},
		{ID: 2, Overseer: "Overseer Yurgulch", Boss: "Uzrathixius", World: -1, Ord: 2, Up: "%b up!", Down: "%b dead. Back %t."},
		{ID: 3, Overseer: "Overseer Yurgulch", Boss: "Poxathixius", World: -1, Ord: 3, Up: "%b up!", Down: "%b dead. Back %t."},
	}
}

// overseerWorld is the trigger world with the plane's bosses in world 0: the throne's
// trigger at (100, 50, 100), Bertoxxulous's spawn-only original, the twins' rows (2
// days back), and Yurgulch.
func overseerWorld(t *testing.T) (*trigWorld, *entity.NPC, *entity.NPC, *entity.NPC) {
	t.Helper()
	w := newTrigWorld(t)
	waits := strings.Repeat(`,{"do":"wait","seconds":14400}`, 22)
	w.setAt(30, "Bertoxxulous Rises", "near", 50, 0, 100, 50, 100, `[{"do":"lock","lock":"throne of bertoxxulous"},`+
		`{"do":"spawn","npc":"Bertoxxulous","hostile":true,"spread":8,"minutes":60},{"do":"awaitdead"}`+waits+`]`, true)
	w.gw.NPCs[800] = &entity.NPC{ServerID: 800, Name: "Bertoxxulous", HP: 100, MaxHP: 100, Level: 66, X: 102, Y: 50, Z: 120,
		SpawnX: 102, SpawnY: 50, SpawnZ: 120, SpawnOnly: true, RespawnTime: 72 * time.Hour}
	uz := &entity.NPC{ServerID: 801, Name: "Uzrathixius", HP: 100, MaxHP: 100, Level: 65, X: 80, Y: 50, Z: 300,
		SpawnX: 80, SpawnY: 50, SpawnZ: 300, RespawnTime: 48 * time.Hour}
	px := &entity.NPC{ServerID: 802, Name: "Poxathixius", HP: 100, MaxHP: 100, Level: 65, X: 120, Y: 50, Z: 300,
		SpawnX: 120, SpawnY: 50, SpawnZ: 300, RespawnTime: 48 * time.Hour}
	yu := &entity.NPC{ServerID: 803, Name: "Overseer Yurgulch", HP: 100, MaxHP: 100, Level: 55, NPCType: 0x80, X: 0, Y: 50, Z: 0}
	for _, n := range []*entity.NPC{uz, px, yu} {
		w.gw.NPCs[n.ServerID] = n
	}
	return w, uz, px, yu
}

func (w *trigWorld) yurgulch(yu *entity.NPC) []string {
	return OverseerLines(w.gw, w.e, yu, "Skywalker", overseerRows(), w.now)
}

func TestOverseerSaysWhichBossesAreUp(t *testing.T) {
	w, uz, px, yu := overseerWorld(t)
	want := func(step string, lines ...string) {
		t.Helper()
		got := w.yurgulch(yu)
		if strings.Join(got, "|") != strings.Join(append([]string{"Yurgulch watches!"}, lines...), "|") {
			t.Fatalf("%s: he says %q, want %q", step, got, lines)
		}
	}
	// the twins up; Bertoxxulous never stood (spawn-only), his throne free
	want("all quiet", "Bertoxxulous sleeps. Throne ready.", "Uzrathixius up!", "Poxathixius up!")

	// Poxathixius killed: back 2 days after his corpse goes (3 minutes)
	px.Dead, px.HP, px.DeathTime = true, 0, w.now
	want("Poxathixius dead", "Bertoxxulous sleeps. Throne ready.", "Uzrathixius up!", "Poxathixius dead. Back in about 2 days.")
	// his corpse looted and gone 47 hours 50 minutes ago: soon
	px.CorpseGone = w.now.Add(-47*time.Hour - 50*time.Minute)
	want("Poxathixius soon", "Bertoxxulous sleeps. Throne ready.", "Uzrathixius up!", "Poxathixius dead. Back soon.")
	// a GM's Kill Spawner holds him: no time known
	px.GMHeld = true
	if got := w.yurgulch(yu)[3]; got != "Poxathixius is not up, and no one knows when." {
		t.Fatalf("held: %q", got)
	}
	px.GMHeld, px.Dead, px.HP = false, false, 100

	// the bonescepter at the throne: Bertoxxulous up
	if _, err := w.e.FireNamed("Bertoxxulous Rises", TriggerFirer{Name: "quest", Quiet: true}); err != nil {
		t.Fatal(err)
	}
	want("Bertoxxulous up", "Bertoxxulous up on throne!", "Uzrathixius up!", "Poxathixius up!")

	// killed: the throne rests 22 waits of 4 hours, 3 days 16 hours
	for _, n := range w.spawned() {
		if n.Name == "Bertoxxulous" {
			w.kill(n)
		}
	}
	w.at(time.Second)
	uz.Dead, uz.HP, uz.DeathTime = true, 0, w.now.Add(-30*time.Hour)
	want("throne resting", "Throne rests. Ready in about 3 days and 16 hours.", "Uzrathixius dead. Back in about 18 hours.", "Poxathixius up!")
	if w.e.RestOf("Bertoxxulous Rises", w.now).Free.Sub(w.now) != 88*time.Hour {
		t.Fatalf("the rest is %v, want 88h", w.e.RestOf("Bertoxxulous Rises", w.now).Free.Sub(w.now))
	}
	// 82 hours on (each wait ticked through): 6 hours left
	for d := 4 * time.Hour; d <= 80*time.Hour; d += 4 * time.Hour {
		w.at(time.Second + d)
	}
	w.at(time.Second + 82*time.Hour)
	if got := w.yurgulch(yu)[1]; got != "Throne rests. Ready in about 6 hours." {
		t.Fatalf("82 hours on: %q", got)
	}
	w.at(time.Second + 84*time.Hour)
	if got := w.yurgulch(yu)[1]; got != "Throne rests. Ready in about 4 hours." {
		t.Fatalf("84 hours on, the last wait's: %q", got)
	}
	// the rest over: the throne answers again (the last wait held it to its end)
	w.at(time.Second + 88*time.Hour)
	if got := w.yurgulch(yu)[1]; got != "Bertoxxulous sleeps. Throne ready." {
		t.Fatalf("88 hours on: %q", got)
	}
}

// The control: Bertoxxulous spawned elsewhere (another world's trigger, Mr. Moogie's)
// is not the plane's: Yurgulch reads his own world.
func TestOverseerReadsHisOwnWorld(t *testing.T) {
	w, _, _, yu := overseerWorld(t)
	w.gw.NPCs[900] = &entity.NPC{ServerID: 900, Name: "Bertoxxulous", HP: 100, MaxHP: 100, World: 2, Temporary: true}
	if got := w.yurgulch(yu)[1]; got != "Bertoxxulous sleeps. Throne ready." {
		t.Fatalf("a Bertoxxulous in another world: %q", got)
	}
	w.gw.NPCs[900].World = 0
	if got := w.yurgulch(yu)[1]; got != "Bertoxxulous up on throne!" {
		t.Fatalf("a Bertoxxulous in his world: %q", got)
	}
	// the trigger off: no one knows
	w.setAt(30, "Bertoxxulous Rises", "near", 50, 0, 100, 50, 100, `[{"do":"lock","lock":"throne"}]`, false)
	delete(w.gw.NPCs, 900)
	if got := w.yurgulch(yu)[1]; got != "Bertoxxulous is not up, and no one knows when." {
		t.Fatalf("the trigger off: %q", got)
	}
}

// Talked to, he answers in one box, his name before it; an NPC with no row is not an
// overseer (the control: nothing said); the table read back in ord's order.
func TestOverseerTalkAndTable(t *testing.T) {
	tw := newTalkWorld(t, 25403, 15725)
	tw.h.overseers = func() (map[string][]database.NPCOverseerRow, error) {
		return map[string][]database.NPCOverseerRow{"overseer yurgulch": overseerRows()[1:]}, nil
	}
	yu := tw.add(1, &entity.NPC{ServerID: 701, Name: "Overseer Yurgulch", Level: 55, NPCType: 0x80, SpawnX: 25404, SpawnY: 54, SpawnZ: 15726})
	tw.add(2, &entity.NPC{ServerID: 702, Name: "Uzrathixius", Level: 65, SpawnX: 26000, SpawnY: 54, SpawnZ: 15726})
	other := tw.add(3, &entity.NPC{ServerID: 703, Name: "Honcho Oozlgurgle", Level: 55, NPCType: 0x80, SpawnX: 25403, SpawnY: 54, SpawnZ: 15724})
	tw.talk(yu)
	got := mobBoxes(tw.taken())
	if len(got) != 1 || got[0] != "Overseer Yurgulch: Uzrathixius up!\nPoxathixius is not up, and no one knows when." {
		t.Fatalf("his talk: %q", got)
	}
	tw.talk(other)
	if got := tw.taken(); len(got) != 0 {
		t.Fatalf("the control: Honcho Oozlgurgle answered %d messages", len(got))
	}

	db, err := database.NewSQLite(t.TempDir() + "/o.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO npcoverseer (overseer, boss, ord) VALUES ('Overseer Yurgulch', 'Poxathixius', 3),
		(' overseer yurgulch ', 'Bertoxxulous', 1), ('Overseer Yurgulch', '', 2), ('Overseer Yurgulch', 'Uzrathixius', 2)`); err != nil {
		t.Fatal(err)
	}
	database.InvalidateNPCOverseer()
	all, err := db.NPCOverseers()
	if err != nil {
		t.Fatal(err)
	}
	var bosses []string
	for _, r := range all["overseer yurgulch"] {
		bosses = append(bosses, r.Boss)
	}
	if strings.Join(bosses, ",") != "Bertoxxulous,Uzrathixius,Poxathixius" || len(all) != 1 {
		t.Fatalf("read back: %v (%d overseers)", bosses, len(all))
	}
}

func TestOverseerWhen(t *testing.T) {
	for _, c := range []struct {
		d    time.Duration
		want string
	}{
		{0, "soon"}, {15 * time.Minute, "soon"}, {16 * time.Minute, "in about 20 minutes"},
		{49 * time.Minute, "in about 50 minutes"}, {70 * time.Minute, "in about an hour"},
		{5*time.Hour + 20*time.Minute, "in about 5 hours"}, {35 * time.Hour, "in about 35 hours"},
		{48 * time.Hour, "in about 2 days"}, {49 * time.Hour, "in about 2 days and 1 hour"},
		{88 * time.Hour, "in about 3 days and 16 hours"},
	} {
		if got := overseerWhen(c.d); got != c.want {
			t.Errorf("%v: %q, want %q", c.d, got, c.want)
		}
	}
}
