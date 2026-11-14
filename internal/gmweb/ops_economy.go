package gmweb

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/economy"
)

// The Economy page (the owner, 2026-11-14: "lets add an economy tracker for
// the gm panel shows tunar accumulated from loot, selling, etc. and trend make
// it informative"): the ledger internal/economy writes (database/economy.go),
// read as totals by source for today, 7 and 30 days, the hourly and daily
// trend, the money supply and its spread, who made and spent the most, and the
// lines that say it in words. gm.economy_other_db names another shard's
// database, read-only, for its ledger beside this one's.

func (s *Server) registerEconomyOps() {
	s.addOp(&Op{
		Name: "economy", Title: "Economy (tunar made, spent, held)", Scope: scopeRead,
		Description: "The tunar economy from the server's ledger: every movement of tunar counted at its source, an " +
			"hour a row. Faucets make tunar (corpse coins, group shares, pickpocketing, items sold to merchants, quest " +
			"rewards, new characters' purses, GM grants), sinks end it (merchant purchases, buy-backs, repairs, quest " +
			"payments, waysender fares, fever cures, auction listing fees, the auction house's own lots, GM removals), " +
			"transfers move it between characters (trades, auction sales, duel wagers). Returns: windows today (UTC), 7d " +
			"and 30d with each source's amount, count and share; hourly (48 h) and daily (days, default 30) series of " +
			"faucets, sinks, transfers, net and the money supply; the latest wealth snapshot (purses, banks, auction " +
			"escrow, GMs' characters apart, median, p90, p99, how many hold over 100k/1M/10M, the richest twenty) with " +
			"its change over a day and a week; today's and the week's top earners, spenders and traders; and callouts, " +
			"plain-language lines saying what matters. shard \"other\" reads the other shard's ledger when this panel " +
			"names its database (gm.economy_other_db).",
		Input: obj(map[string]any{
			"days":  integer("days of the daily series, 1 to 120 (default 30)"),
			"shard": str("\"here\" (default) or \"other\": the other shard's ledger, read-only, when configured"),
		}),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Days  int    `json:"days"`
				Shard string `json:"shard"`
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			return c.s.economyReport(req.Shard, req.Days, time.Now())
		},
	})
}

// economySum is a window's or a bucket's totals.
type economySum struct {
	Faucet   int64 `json:"faucet"`
	Sink     int64 `json:"sink"`
	Transfer int64 `json:"transfer"`
	Net      int64 `json:"net"` // faucet - sink: tunar the economy gained
}

func (e *economySum) add(kind string, n int64) {
	switch economy.Kind(kind) {
	case economy.Faucet:
		e.Faucet += n
	case economy.Sink:
		e.Sink += n
	case economy.Transfer:
		e.Transfer += n
	}
	e.Net = e.Faucet - e.Sink
}

type economySource struct {
	Source string  `json:"source"`
	Kind   string  `json:"kind"`
	Label  string  `json:"label"`
	Count  int64   `json:"count"`
	Amount int64   `json:"amount"`
	Share  float64 `json:"share"` // of its kind's total, percent
}

type economyWindow struct {
	Since int64   `json:"since"`
	Days  float64 `json:"days"` // what the ledger covers of it
	economySum
	Sources []economySource `json:"sources"`
}

type economyPoint struct {
	T int64 `json:"t"`
	economySum
	Supply       *int64 `json:"supply,omitempty"`       // all tunar held at the bucket's last snapshot
	PlayerSupply *int64 `json:"playerSupply,omitempty"` // of it the players'
}

type economyChar struct {
	ServerID uint32           `json:"serverid"`
	Name     string           `json:"name"`
	Staff    bool             `json:"staff,omitempty"`
	Made     int64            `json:"made"`  // faucets' tunar to it
	Spent    int64            `json:"spent"` // its tunar to sinks
	In       int64            `json:"in"`    // transfers to it
	Out      int64            `json:"out"`   // its transfers to others
	Net      int64            `json:"net"`   // made + in - spent - out
	Sources  map[string]int64 `json:"sources"`
}

func (s *Server) economyReport(shard string, days int, now time.Time) (map[string]any, error) {
	if days <= 0 {
		days = 30
	}
	days = min(days, 120)
	name := s.shardName()
	if name == "" {
		name = "this server"
	}
	var q database.Queryer = s.d.DB
	other := strings.TrimSpace(s.cfg.EconomyOtherDB)
	otherName := strings.TrimSpace(s.cfg.EconomyOtherName)
	if otherName == "" && other != "" {
		otherName = "the other shard"
	}
	switch shard {
	case "", "here":
		// the counts not yet written first, so the page is to the second
		_ = economy.Default.Flush(s.d.DB)
	case "other":
		if other == "" {
			return nil, badRequest("this panel names no other shard's database (gm.economy_other_db)")
		}
		db, err := database.OpenReadOnly(other)
		if err != nil {
			return nil, fmt.Errorf("the other shard's database: %w", err)
		}
		defer db.Close()
		q, name = db, otherName
	default:
		return nil, badRequest("shard is \"here\" or \"other\"")
	}
	out := map[string]any{"shard": name, "generated": now.UTC().Format(time.RFC3339), "now": now.Unix(), "days": days}
	if other != "" {
		out["other"] = map[string]any{"name": otherName, "showing": shard == "other"}
	}
	out["sources"] = economy.Sources

	day := now.Unix() / 86400 * 86400
	hourNow := now.Unix() / 3600 * 3600
	since := time.Unix(day-int64(max(days, 30)-1)*86400, 0)
	hours, err := database.TunarHours(q, since, now.Add(time.Hour))
	if err != nil {
		return nil, fmt.Errorf("the ledger: %w (no ledger there yet?)", err)
	}
	first := database.TunarLedgerFirst(q)
	out["first"] = first

	labels := map[string]economy.Info{}
	for _, i := range economy.Sources {
		labels[string(i.Source)] = i
	}
	window := func(from int64) economyWindow {
		w := economyWindow{Since: from}
		by := map[string]*economySource{}
		for _, r := range hours {
			if r.Hour < from {
				continue
			}
			w.add(r.Kind, r.Amount)
			e := by[r.Source]
			if e == nil {
				e = &economySource{Source: r.Source, Kind: r.Kind, Label: labels[r.Source].Label}
				if e.Label == "" {
					e.Label = r.Source
				}
				by[r.Source] = e
			}
			e.Count += r.Count
			e.Amount += r.Amount
		}
		for _, e := range by {
			total := map[string]int64{string(economy.Faucet): w.Faucet, string(economy.Sink): w.Sink, string(economy.Transfer): w.Transfer}[e.Kind]
			if total > 0 {
				e.Share = math.Round(float64(e.Amount)*1000/float64(total)) / 10
			}
			w.Sources = append(w.Sources, *e)
		}
		sort.Slice(w.Sources, func(i, j int) bool {
			a, b := w.Sources[i], w.Sources[j]
			if a.Kind != b.Kind {
				return kindOrder(a.Kind) < kindOrder(b.Kind)
			}
			return a.Amount > b.Amount
		})
		start := max(from, first)
		if first == 0 {
			start = now.Unix()
		}
		w.Days = math.Round(float64(now.Unix()-start)/86400*10) / 10
		return w
	}
	today, week, month := window(day), window(day-6*86400), window(day-29*86400)
	out["windows"] = map[string]any{"today": today, "7d": week, "30d": month}

	// the wealth snapshots: the series' supply, the latest, its change
	snaps, err := database.TunarWealthSince(q, since)
	if err != nil {
		return nil, err
	}
	supplyAt := func(from, until int64) (*int64, *int64) {
		var last *database.TunarWealth
		for i := range snaps {
			if snaps[i].Hour >= from && snaps[i].Hour < until {
				last = &snaps[i]
			}
		}
		if last == nil {
			return nil, nil
		}
		a, p := last.Supply(), last.PlayerSupply()
		return &a, &p
	}
	var hourly []economyPoint
	for t := hourNow - 47*3600; t <= hourNow; t += 3600 {
		pt := economyPoint{T: t}
		for _, r := range hours {
			if r.Hour == t {
				pt.add(r.Kind, r.Amount)
			}
		}
		pt.Supply, pt.PlayerSupply = supplyAt(t, t+3600)
		hourly = append(hourly, pt)
	}
	var daily []economyPoint
	for t := day - int64(days-1)*86400; t <= day; t += 86400 {
		pt := economyPoint{T: t}
		for _, r := range hours {
			if r.Hour >= t && r.Hour < t+86400 {
				pt.add(r.Kind, r.Amount)
			}
		}
		pt.Supply, pt.PlayerSupply = supplyAt(t, t+86400)
		daily = append(daily, pt)
	}
	out["hourly"], out["daily"] = hourly, daily

	var latest *database.TunarWealth
	if len(snaps) > 0 {
		latest = &snaps[len(snaps)-1]
		ago := func(secs int64) *database.TunarWealth {
			var best *database.TunarWealth
			for i := range snaps {
				if snaps[i].Hour <= latest.Hour-secs {
					best = &snaps[i]
				}
			}
			if best == nil && snaps[0].Hour < latest.Hour {
				best = &snaps[0] // the oldest there is, when younger than asked
			}
			return best
		}
		wealth := map[string]any{"latest": latest, "supply": latest.Supply(), "playerSupply": latest.PlayerSupply()}
		for key, secs := range map[string]int64{"day": 86400, "week": 7 * 86400} {
			if b := ago(secs); b != nil {
				wealth[key] = map[string]any{"hour": b.Hour, "supply": b.Supply(), "playerSupply": b.PlayerSupply(),
					"change": latest.Supply() - b.Supply(), "pct": pct(latest.Supply()-b.Supply(), b.Supply())}
			}
		}
		out["wealth"] = wealth
	}

	// who made, spent and moved the most
	chars, err := database.TunarChars(q, time.Unix(day-6*86400, 0), now.Add(24*time.Hour))
	if err != nil {
		return nil, err
	}
	staff := economyStaff(q, s.cfg.MinLevel)
	tops := func(from int64) map[string]any {
		by := map[uint32]*economyChar{}
		for _, r := range chars {
			if r.Day < from {
				continue
			}
			e := by[r.ServerID]
			if e == nil {
				e = &economyChar{ServerID: r.ServerID, Name: r.Name, Staff: staff[r.ServerID], Sources: map[string]int64{}}
				by[r.ServerID] = e
			}
			if r.Name != "" {
				e.Name = r.Name
			}
			switch economy.KindOf(economy.Source(r.Source)) {
			case economy.Faucet:
				e.Made += r.Got
				e.Sources[r.Source] += r.Got
			case economy.Sink:
				e.Spent += r.Paid
				e.Sources[r.Source] -= r.Paid
			case economy.Transfer:
				e.In += r.Got
				e.Out += r.Paid
				e.Sources[r.Source] += r.Got - r.Paid
			}
			e.Net = e.Made + e.In - e.Spent - e.Out
		}
		list := make([]economyChar, 0, len(by))
		for _, e := range by {
			list = append(list, *e)
		}
		top := func(key func(economyChar) int64) []economyChar {
			l := append([]economyChar(nil), list...)
			sort.SliceStable(l, func(i, j int) bool {
				if key(l[i]) != key(l[j]) {
					return key(l[i]) > key(l[j])
				}
				return l[i].Name < l[j].Name
			})
			var o []economyChar
			for _, e := range l {
				if key(e) <= 0 || len(o) == 10 {
					break
				}
				o = append(o, e)
			}
			return o
		}
		return map[string]any{
			"earners":  top(func(e economyChar) int64 { return e.Made }),
			"spenders": top(func(e economyChar) int64 { return e.Spent }),
			"traders":  top(func(e economyChar) int64 { return e.In + e.Out }),
			"active":   len(list),
		}
	}
	todayTop, weekTop := tops(day), tops(day-6*86400)
	out["top"] = map[string]any{"today": todayTop, "7d": weekTop}
	out["callouts"] = economyCallouts(today, week, first, now, latest, out["wealth"], todayTop)
	return out, nil
}

func kindOrder(k string) int {
	switch economy.Kind(k) {
	case economy.Faucet:
		return 0
	case economy.Sink:
		return 1
	}
	return 2
}

// economyStaff is the characters of GMs' accounts (acctlevel at or above level).
func economyStaff(q database.Queryer, level int) map[uint32]bool {
	out := map[uint32]bool{}
	if level <= 0 {
		level = 1
	}
	rows, err := q.Query(`SELECT c.serverid FROM characters c JOIN accountinfo a ON a.accountid = c.accountid
		WHERE COALESCE(a.acctlevel, 0) >= ?`, level)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id uint32
		if rows.Scan(&id) == nil {
			out[id] = true
		}
	}
	return out
}

func pct(part, whole int64) float64 {
	if whole == 0 {
		return 0
	}
	return math.Round(float64(part)*1000/float64(whole)) / 10
}

// tunarWords is an amount as players say it: 950, 12.4k, 3.1M.
func tunarWords(n int64) string {
	sign := ""
	if n < 0 {
		sign, n = "-", -n
	}
	switch {
	case n >= 1_000_000_000:
		return fmt.Sprintf("%s%.2fB", sign, float64(n)/1e9)
	case n >= 1_000_000:
		return fmt.Sprintf("%s%.1fM", sign, float64(n)/1e6)
	case n >= 10_000:
		return fmt.Sprintf("%s%.1fk", sign, float64(n)/1e3)
	}
	return fmt.Sprintf("%s%d", sign, n)
}

func signed(n int64) string {
	if n > 0 {
		return "+" + tunarWords(n)
	}
	return tunarWords(n)
}

// economyCallouts are the lines that say what the numbers mean.
func economyCallouts(today, week economyWindow, first int64, now time.Time, latest *database.TunarWealth,
	wealth any, top map[string]any) []string {
	var out []string
	if first == 0 {
		return []string{"Nothing counted yet: the ledger starts with this build (and the log backfill, when run)."}
	}
	lead := func(w economyWindow, kind economy.Kind) (economySource, bool) {
		for _, e := range w.Sources {
			if e.Kind == string(kind) {
				return e, true // sorted by amount within a kind
			}
		}
		return economySource{}, false
	}
	line := fmt.Sprintf("Net %s tunar today (%s made, %s spent)", signed(today.Net), tunarWords(today.Faucet), tunarWords(today.Sink))
	if f, ok := lead(today, economy.Faucet); ok {
		line += fmt.Sprintf(", %.0f%% of it made from %s", f.Share, strings.ToLower(f.Label))
	}
	out = append(out, line+".")
	if today.Faucet > 0 {
		ratio := float64(today.Sink) / float64(today.Faucet)
		s, _ := lead(today, economy.Sink)
		switch {
		case ratio < 0.25:
			out = append(out, fmt.Sprintf("Sinks remove only %.0f%% of what is made today: tunar is piling up (biggest sink: %s).", ratio*100, strings.ToLower(s.Label)))
		case ratio > 1:
			out = append(out, fmt.Sprintf("More is spent than made today (%.0f%%): the money supply is shrinking.", ratio*100))
		default:
			if s.Label != "" {
				out = append(out, fmt.Sprintf("Sinks remove %.0f%% of what is made today, most of it %s.", ratio*100, strings.ToLower(s.Label)))
			}
		}
	}
	if week.Days >= 2 {
		avg := float64(week.Faucet) / week.Days
		if hrs := float64(now.Unix()%86400) / 86400; hrs > 0.25 && avg > 0 {
			pace := float64(today.Faucet) / hrs // today's faucets at a whole day's pace
			if r := pace / avg; r > 1.5 || r < 0.67 {
				out = append(out, fmt.Sprintf("Today's faucets run at %.1fx the %.0f-day daily average (%s a day).", r, math.Min(week.Days, 7), tunarWords(int64(avg))))
			}
		}
		out = append(out, fmt.Sprintf("This week: net %s (%s made, %s spent, %s changed hands between players).",
			signed(week.Net), tunarWords(week.Faucet), tunarWords(week.Sink), tunarWords(week.Transfer)))
	}
	if gm := sourceAmount(week, economy.GMGive); week.Faucet > 0 && gm*10 >= week.Faucet {
		out = append(out, fmt.Sprintf("GM grants are %.0f%% of this week's faucets (%s): the player economy's own is %s.",
			pct(gm, week.Faucet), tunarWords(gm), tunarWords(week.Faucet-gm)))
	}
	if w, ok := wealth.(map[string]any); ok && latest != nil {
		if wk, ok := w["week"].(map[string]any); ok {
			since := time.Unix(wk["hour"].(int64), 0).UTC()
			span := "this week"
			if now.Sub(since) < 6*24*time.Hour {
				span = "since " + since.Format("Jan 2 15:04") + " UTC"
			}
			p := wk["pct"].(float64)
			dir := "up"
			if p < 0 {
				dir = "down"
			}
			out = append(out, fmt.Sprintf("Money supply %s %.1f%% %s (%s held, players' %s).", dir, math.Abs(p), span,
				tunarWords(latest.Supply()), tunarWords(latest.PlayerSupply())))
		} else {
			out = append(out, fmt.Sprintf("Money supply: %s held (players' %s).", tunarWords(latest.Supply()), tunarWords(latest.PlayerSupply())))
		}
		if ps := latest.Purse + latest.Bank; ps > 0 && latest.Characters >= 20 {
			out = append(out, fmt.Sprintf("The 10 richest players hold %.0f%% of the players' tunar; the median character holds %s, %d hold over 1M.",
				pct(latest.Top10, ps), tunarWords(latest.Median), latest.Over1M))
		}
	}
	if list, ok := top["earners"].([]economyChar); ok && len(list) > 0 {
		e := list[0]
		best, bestN := "", int64(0)
		for src, n := range e.Sources {
			if n > bestN && economy.KindOf(economy.Source(src)) == economy.Faucet {
				best, bestN = src, n
			}
		}
		who := e.Name
		if e.Staff {
			who += " (GM)"
		}
		lbl := best
		for _, i := range economy.Sources {
			if string(i.Source) == best {
				lbl = strings.ToLower(i.Label)
			}
		}
		out = append(out, fmt.Sprintf("Top earner today: %s, %s made (mostly %s).", who, tunarWords(e.Made), lbl))
	}
	if days := float64(now.Unix()-first) / 86400; days < 7 {
		out = append(out, fmt.Sprintf("The ledger counts from %s UTC (%.1f days): the 7- and 30-day figures cover that.",
			time.Unix(first, 0).UTC().Format("Jan 2 15:04"), days))
	}
	return out
}

func sourceAmount(w economyWindow, src economy.Source) int64 {
	for _, e := range w.Sources {
		if e.Source == string(src) {
			return e.Amount
		}
	}
	return 0
}

// countTunarEdit counts a GM's edit of a character's tunar or bank (a row
// change through the panel, a revert among them) as a grant or a removal.
func countTunarEdit(action string, before, after map[string]any) {
	if action != "update" || before == nil || after == nil {
		return
	}
	get := func(m map[string]any, col string) int64 {
		f, _ := asFloat(m[col])
		return int64(f)
	}
	delta := get(after, "tunar") + get(after, "bankTunar") - get(before, "tunar") - get(before, "bankTunar")
	if delta == 0 {
		return
	}
	id, _ := asFloat(after["serverid"])
	name, _ := after["charName"].(string)
	economy.GMChange(uint32(id), name, int(delta))
}
