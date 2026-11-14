package gmweb

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// seedLedger puts a day of counts in db: today's loot, sales and purchases,
// yesterday's loot, a trade, and two wealth snapshots a day apart.
func seedLedger(t *testing.T, db *database.SQLiteDB, now time.Time, scale int64) {
	t.Helper()
	hour := now.Unix() / 3600 * 3600
	day := now.Unix() / 86400 * 86400
	if err := db.FlushTunarLedger([]database.TunarHourRow{
		{Hour: hour, Source: "loot", Kind: "faucet", Count: 10, Amount: 800 * scale},
		{Hour: hour, Source: "merchant_sell", Kind: "faucet", Count: 2, Amount: 200 * scale},
		{Hour: hour, Source: "merchant_buy", Kind: "sink", Count: 1, Amount: 300 * scale},
		{Hour: hour, Source: "trade", Kind: "transfer", Count: 1, Amount: 50 * scale},
		{Hour: day - 86400, Source: "loot", Kind: "faucet", Count: 5, Amount: 400 * scale},
	}, []database.TunarCharRow{
		{Day: day, ServerID: 7, Source: "loot", Name: "Hero", Count: 10, Got: 800 * scale},
		{Day: day, ServerID: 7, Source: "merchant_buy", Name: "Hero", Count: 1, Paid: 300 * scale},
		{Day: day, ServerID: 7, Source: "trade", Name: "Hero", Count: 1, Paid: 50 * scale},
		{Day: day, ServerID: 8, Source: "trade", Name: "Pal", Count: 1, Got: 50 * scale},
		{Day: day, ServerID: 8, Source: "merchant_sell", Name: "Pal", Count: 2, Got: 200 * scale},
	}); err != nil {
		t.Fatal(err)
	}
	for i, h := range []int64{hour - 86400, hour} {
		if err := db.SaveTunarWealth(database.TunarWealth{Hour: h, Characters: 1, Holders: 1, Purse: 1000 * scale,
			Bank: int64(i) * 100 * scale, Median: 1000 * scale}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEconomyOp(t *testing.T) {
	f := newFixture(t)
	now := time.Now()
	seedLedger(t, f.db, now, 1)
	gm := f.browser()
	gm.login("gm")
	code, out := gm.op("economy", map[string]any{"days": 7})
	if code != 200 {
		t.Fatalf("%d %v", code, out)
	}
	today := out["windows"].(map[string]any)["today"].(map[string]any)
	if today["faucet"] != float64(1000) || today["sink"] != float64(300) || today["transfer"] != float64(50) || today["net"] != float64(700) {
		t.Errorf("today: %v", today)
	}
	week := out["windows"].(map[string]any)["7d"].(map[string]any)
	if week["faucet"] != float64(1400) {
		t.Errorf("7 days: %v", week)
	}
	srcs := today["sources"].([]any)
	first := srcs[0].(map[string]any)
	if first["source"] != "loot" || first["share"] != float64(80) || first["label"] != "Corpse coins" {
		t.Errorf("the first source: %v", first)
	}
	if n := len(out["daily"].([]any)); n != 7 {
		t.Errorf("%d days", n)
	}
	if n := len(out["hourly"].([]any)); n != 48 {
		t.Errorf("%d hours", n)
	}
	last := out["daily"].([]any)[6].(map[string]any)
	if last["faucet"] != float64(1000) || last["supply"] != float64(1100) {
		t.Errorf("today's point: %v", last)
	}
	w := out["wealth"].(map[string]any)
	if w["supply"] != float64(1100) || w["day"].(map[string]any)["pct"] != float64(10) {
		t.Errorf("wealth: %v", w)
	}
	top := out["top"].(map[string]any)["today"].(map[string]any)
	earn := top["earners"].([]any)
	if len(earn) != 2 || earn[0].(map[string]any)["name"] != "Hero" || earn[0].(map[string]any)["net"] != float64(450) {
		t.Errorf("earners: %v", earn)
	}
	if tr := top["traders"].([]any); len(tr) != 2 {
		t.Errorf("traders: %v", tr)
	}
	calls := strings.Join(toStrings(out["callouts"]), "\n")
	for _, want := range []string{"Net +700 tunar today (1000 made, 300 spent), 80% of it made from corpse coins.",
		"Top earner today: Hero, 800 made (mostly corpse coins).", "Money supply"} {
		if !strings.Contains(calls, want) {
			t.Errorf("no %q in:\n%s", want, calls)
		}
	}
	if out["other"] != nil {
		t.Errorf("an other shard unasked: %v", out["other"])
	}
	if code, _ := gm.op("economy", map[string]any{"shard": "other"}); code != 400 {
		t.Errorf("the other shard unconfigured: %d", code)
	}

	// the other shard's ledger, read-only
	otherPath := filepath.Join(t.TempDir(), "live.db")
	odb, err := database.NewSQLite(otherPath)
	if err != nil {
		t.Fatal(err)
	}
	seedLedger(t, odb, now, 10)
	odb.Close()
	f.s.cfg.EconomyOtherDB, f.s.cfg.EconomyOtherName = otherPath, "Live Server"
	code, out = gm.op("economy", map[string]any{"shard": "other"})
	if code != 200 {
		t.Fatalf("%d %v", code, out)
	}
	if out["shard"] != "Live Server" || out["windows"].(map[string]any)["today"].(map[string]any)["faucet"] != float64(10000) {
		t.Errorf("the other: %v %v", out["shard"], out["windows"])
	}
	_, out = gm.op("economy", nil)
	if o, _ := out["other"].(map[string]any); o["name"] != "Live Server" || o["showing"] != false {
		t.Errorf("the other named: %v", out["other"])
	}

	// a player is no GM
	p := f.browser()
	if code, _ := p.op("economy", nil); code != 401 && code != 403 {
		t.Errorf("signed out: %d", code)
	}
}

// A GM's edit of a character's tunar counts as a grant or a removal, written
// by the op's own flush.
func TestEconomyCountsPanelEdits(t *testing.T) {
	f := newFixture(t)
	boss := f.browser()
	boss.login("boss")
	code, out := boss.op("update_row", map[string]any{"table": "characters", "key": map[string]any{"serverid": 7},
		"values": map[string]any{"tunar": 1100}})
	if code != 200 {
		t.Fatalf("%d %v", code, out)
	}
	code, out = boss.op("update_row", map[string]any{"table": "characters", "key": map[string]any{"serverid": 7},
		"values": map[string]any{"bankTunar": 0, "tunar": 600}})
	if code != 200 {
		t.Fatalf("%d %v", code, out)
	}
	_, out = boss.op("economy", nil)
	today := out["windows"].(map[string]any)["today"].(map[string]any)
	if today["faucet"] != float64(1000) || today["sink"] != float64(500) {
		t.Errorf("today: %v", today)
	}
}

func toStrings(v any) []string {
	var out []string
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}
