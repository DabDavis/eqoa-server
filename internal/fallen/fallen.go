// Package fallen is the public page of the Hardcore heroes (the owner,
// 2026-11-18: a fallen Hardcore character is "shown on a fallen-heroes list";
// "also class cm count etc"; "make the leaderboard and memorial show on the
// website too"): /hardcore (eqoaclassic.com/eqoa/hardcore; /fallen sends its
// reader there) and /api/hardcore on the account pages' server, read-only:
// each shard's living Hardcore characters ranked and its falls as their rows
// keep them (database hardcore.go), nothing of any account. This shard's, and with
// world.hardcore.other_db another's, opened read-only at most once a minute.
package fallen

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// fresh is how long a read is shown before the databases are read again.
const fresh = time.Minute

// most is how many falls a shard lists.
const most = 500

// Hero is one fall as the page and its API show it.
type Hero struct {
	Shard     string `json:"shard,omitempty"`
	Name      string `json:"name"`
	Level     int    `json:"level"`
	Class     string `json:"class"`
	Race      string `json:"race"`
	CMs       int    `json:"cms"`
	Fell      string `json:"fell"` // RFC 3339, UTC
	FellUnix  int64  `json:"fell_unix"`
	Killer    string `json:"killer"`
	Cause     string `json:"cause"`
	Place     string `json:"place"`
	Played    int64  `json:"played_seconds"`
	LivedDays int64  `json:"lived_seconds,omitempty"` // creation to fall; 0 unknown (made before the columns)
}

// Page serves the page and its API.
type Page struct {
	db                        *database.SQLiteDB
	shard, otherDB, otherMark string
	open                      func(string) (*sql.DB, error)
	now                       func() time.Time

	mu     sync.Mutex
	at     time.Time
	heroes []Hero
	living []Living
}

// Living is a living Hardcore character as the leaderboard ranks it.
type Living struct {
	Rank   int    `json:"rank"`
	Shard  string `json:"shard,omitempty"`
	Name   string `json:"name"`
	Level  int    `json:"level"`
	Class  string `json:"class"`
	Race   string `json:"race"`
	CMs    int    `json:"cms"`
	Alive  int64  `json:"alive_seconds,omitempty"` // since its creation; 0 unknown
	Played int64  `json:"played_seconds"`
	xp     int64
	born   int64
}

// livingTop is how many living a shard lists.
const livingTop = 100

// New is the page over db with world.hardcore's names; nil with no db.
func New(db *database.SQLiteDB, c config.HardcoreConfig) *Page {
	if db == nil {
		return nil
	}
	return &Page{db: db, shard: strings.TrimSpace(c.Shard), otherDB: strings.TrimSpace(c.OtherDB),
		otherMark: strings.TrimSpace(c.OtherMark), open: database.OpenReadOnly, now: time.Now}
}

// HeroOf is a fall as the page shows it.
func HeroOf(f database.FallRecord, shard string) Hero {
	h := Hero{Shard: shard, Name: f.Name, Level: f.Level, Class: entity.ClassName(f.Class), Race: entity.RaceName(f.Race),
		CMs: f.CMs, FellUnix: f.At, Fell: time.Unix(f.At, 0).UTC().Format(time.RFC3339), Killer: f.Killer, Cause: f.Cause,
		Place: f.Place, Played: f.Played}
	if f.Born > 0 && f.At > f.Born {
		h.LivedDays = f.At - f.Born
	}
	return h
}

// Heroes is every shard's falls, the newest first.
func (p *Page) Heroes() ([]Hero, error) {
	h, _, err := p.read()
	return h, err
}

// LivingRanked is every shard's living Hardcore characters, ranked.
func (p *Page) LivingRanked() ([]Living, error) {
	_, l, err := p.read()
	return l, err
}

func livingOf(l database.LivingHero, shard string, now int64) Living {
	v := Living{Shard: shard, Name: l.Name, Level: l.Level, Class: entity.ClassName(l.Class), Race: entity.RaceName(l.Race),
		CMs: l.CMs, Played: l.Played, xp: l.TotalXP, born: l.Born}
	if l.Born > 0 && now > l.Born {
		v.Alive = now - l.Born
	}
	return v
}

func (p *Page) read() ([]Hero, []Living, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.at.IsZero() && p.now().Sub(p.at) < fresh {
		return p.heroes, p.living, nil
	}
	now := p.now().Unix()
	falls, err := p.db.FallenList(most)
	if err != nil {
		return nil, nil, err
	}
	alive, err := p.db.LivingHardcore(livingTop)
	if err != nil {
		return nil, nil, err
	}
	var living []Living
	for _, l := range alive {
		living = append(living, livingOf(l, p.shard, now))
	}
	var out []Hero
	for _, f := range falls {
		out = append(out, HeroOf(f, p.shard))
	}
	if p.otherDB != "" {
		if db, err := p.open(p.otherDB); err != nil {
			log.Printf("Fallen: the other shard's database (%s): %v", p.otherDB, err)
		} else {
			more, err := database.FallenListOf(db, most)
			if err != nil {
				log.Printf("Fallen: the other shard's falls: %v", err)
			}
			for _, f := range more {
				out = append(out, HeroOf(f, p.otherMark))
			}
			others, err := database.LivingHardcoreOf(db, livingTop)
			if err != nil {
				log.Printf("Fallen: the other shard's living: %v", err)
			}
			for _, l := range others {
				living = append(living, livingOf(l, p.otherMark, now))
			}
			db.Close()
		}
	}
	sortNewest(out)
	rankLiving(living)
	p.at, p.heroes, p.living = p.now(), out, living
	return out, living, nil
}

// rankLiving ranks the living of every shard as LivingHardcore does one's.
func rankLiving(l []Living) {
	before := func(a, b Living) bool {
		if a.Level != b.Level {
			return a.Level > b.Level
		}
		if a.CMs != b.CMs {
			return a.CMs > b.CMs
		}
		if a.xp != b.xp {
			return a.xp > b.xp
		}
		return a.Alive > b.Alive
	}
	for i := 1; i < len(l); i++ {
		for j := i; j > 0 && before(l[j], l[j-1]); j-- {
			l[j], l[j-1] = l[j-1], l[j]
		}
	}
	for i := range l {
		l[i].Rank = i + 1
	}
}

func sortNewest(h []Hero) {
	for i := 1; i < len(h); i++ {
		for j := i; j > 0 && h[j].FellUnix > h[j-1].FellUnix; j-- {
			h[j], h[j-1] = h[j-1], h[j]
		}
	}
}

// ServeAPI is /api/fallen: {"heroes": [...]}.
func (p *Page) ServeAPI(w http.ResponseWriter, r *http.Request) {
	heroes, living, err := p.read()
	if err != nil {
		http.Error(w, "the fallen cannot be read just now", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=60")
	json.NewEncoder(w).Encode(map[string]any{"heroes": heroes, "living": living})
}

// Duration is seconds in words: "3d 4h", "5h 12m", "40m".
func Duration(secs int64) string {
	if secs <= 0 {
		return "-"
	}
	d, h, m := secs/86400, secs%86400/3600, secs%3600/60
	switch {
	case d > 0:
		return fmt.Sprintf("%dd %dh", d, h)
	case h > 0:
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dm", max(m, 1))
}

// ServePage is the page (/hardcore; /fallen sends its reader there): the
// site's look (eqoaclassic.com's: dark panels, Cinzel headings, gold
// accents), Hardcore's own accent an ember red; each shard's Living Hardcore
// Leaderboard and Memorial of the Fallen. Read-only, a minute's cache.
func (p *Page) ServePage(w http.ResponseWriter, r *http.Request) {
	heroes, living, err := p.read()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=60")
	var b strings.Builder
	b.WriteString(pageHead)
	shards := p.shards(heroes, living)
	b.WriteString(`<section id="living"><div class="section-head"><h2>Living Hardcore Leaderboard</h2>
<p>Ranked by level, then Combat Mastery, then experience. In Freeport, the Hardcore Leaderboard beside the memorial shows the same.</p><div class="rule"></div></div>`)
	if err != nil {
		b.WriteString(`<p class="none">The heroes cannot be read just now. Try again in a minute.</p>`)
	}
	for _, sh := range shards {
		var rows []Living
		for _, l := range living {
			if l.Shard == sh {
				rows = append(rows, l)
			}
		}
		fmt.Fprintf(&b, `<h3 class="shard">%s</h3>`, html.EscapeString(shardTitle(sh)))
		if len(rows) == 0 {
			b.WriteString(`<p class="none">No Hardcore hero walks this world yet.</p>`)
			continue
		}
		b.WriteString(`<div class="tablewrap"><table><thead><tr><th class="n">#</th><th>Hero</th><th class="n">Level</th><th>Class</th><th class="k">Race</th><th class="n">CMs</th><th class="n">Alive</th><th class="n k">Played</th></tr></thead><tbody>`)
		for i, l := range rows {
			top := ""
			if i < 3 {
				top = ` class="top"`
			}
			fmt.Fprintf(&b, `<tr%s><td class="rk n">%d</td><td class="who"><b>%s</b></td><td class="n">%d</td><td>%s</td><td class="k">%s</td><td class="n">%d</td><td class="n">%s</td><td class="n k">%s</td></tr>`,
				top, i+1, html.EscapeString(l.Name), l.Level, html.EscapeString(l.Class), html.EscapeString(l.Race), l.CMs, Duration(l.Alive), Duration(l.Played))
		}
		b.WriteString(`</tbody></table></div>`)
	}
	b.WriteString(`</section><section id="fallen"><div class="section-head"><h2>Memorial of the Fallen</h2>
<p>Hardcore characters live once. These fell, the newest first; their names stand on the Memorial of the Fallen in Freeport.</p><div class="rule"></div></div>`)
	for _, sh := range shards {
		var rows []Hero
		for _, h := range heroes {
			if h.Shard == sh {
				rows = append(rows, h)
			}
		}
		fmt.Fprintf(&b, `<h3 class="shard">%s</h3>`, html.EscapeString(shardTitle(sh)))
		if len(rows) == 0 {
			b.WriteString(`<p class="none">No Hardcore hero has fallen here yet. May it stay so.</p>`)
			continue
		}
		b.WriteString(`<div class="tablewrap"><table><thead><tr><th>Hero</th><th class="n">Level</th><th>Class</th><th class="k">Race</th><th class="n">CMs</th><th>Fell (UTC)</th><th>Where</th><th>Slain by</th><th class="k">Born (UTC)</th><th class="n">Lived</th><th class="n k">Played</th></tr></thead><tbody>`)
		for _, h := range rows {
			fell := time.Unix(h.FellUnix, 0).UTC().Format("2006-01-02 15:04")
			born := "-"
			if h.LivedDays > 0 {
				born = time.Unix(h.FellUnix-h.LivedDays, 0).UTC().Format("2006-01-02 15:04")
			}
			fmt.Fprintf(&b, `<tr><td class="who"><b class="dead">%s</b></td><td class="n">%d</td><td>%s</td><td class="k">%s</td><td class="n">%d</td><td class="when">%s</td><td>%s</td><td>%s</td><td class="k when">%s</td><td class="n">%s</td><td class="n k">%s</td></tr>`,
				html.EscapeString(h.Name), h.Level, html.EscapeString(h.Class), html.EscapeString(h.Race), h.CMs, fell,
				html.EscapeString(h.Place), html.EscapeString(killerWords(h)), born, Duration(h.LivedDays), Duration(h.Played))
		}
		b.WriteString(`</tbody></table></div>`)
	}
	b.WriteString(`</section>`)
	fmt.Fprintf(&b, `<p class="note">Updated %s UTC, every minute.</p></main>`, p.now().UTC().Format("2006-01-02 15:04"))
	b.WriteString(pageFoot)
	w.Write([]byte(b.String()))
}

// shards is the shards the page has sections for: this one first, then the
// other, then any other a row names.
func (p *Page) shards(heroes []Hero, living []Living) []string {
	out := []string{p.shard}
	seen := map[string]bool{p.shard: true}
	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	if p.otherDB != "" {
		add(p.otherMark)
	}
	for _, h := range heroes {
		add(h.Shard)
	}
	for _, l := range living {
		add(l.Shard)
	}
	return out
}

func shardTitle(s string) string {
	if s == "" {
		return "This server"
	}
	return s + " Server"
}

const pageHead = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Hardcore Heroes - eqoaclassic</title>
  <meta name="description" content="The living Hardcore leaderboard and the Memorial of the Fallen on eqoaclassic's EQOA servers.">
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=Cinzel:wght@500;700;900&family=Inter:wght@400;500;600&display=swap" rel="stylesheet">
  <style>
    :root {
      --ink: #0b0a09; --panel: #15120f; --panel-2: #1c1814; --line: #2e2720;
      --text: #e8e1d6; --muted: #a39887; --gold: #d9a441; --gold-hi: #f3cf7a;
      --ember: #c0532f; --ember-hi: #ef8a62;
    }
    * { box-sizing: border-box; margin: 0; padding: 0; }
    html { scroll-behavior: smooth; }
    body { background: var(--ink); color: var(--text); font-family: Inter, -apple-system, "Segoe UI", Roboto, sans-serif; line-height: 1.6; -webkit-font-smoothing: antialiased; }
    a { color: inherit; }
    .wrap { max-width: 1120px; margin: 0 auto; padding: 0 16px; }
    h1, h2, h3, .brand { font-family: Cinzel, Georgia, serif; letter-spacing: 0.04em; }
    .topbar { position: absolute; top: 0; left: 0; right: 0; z-index: 5; }
    .topbar .wrap { display: flex; align-items: center; justify-content: space-between; padding-top: 18px; padding-bottom: 18px; }
    .brand { font-weight: 700; font-size: 1.05rem; color: var(--text); text-decoration: none; letter-spacing: 0.18em; text-transform: uppercase; }
    .brand span { color: var(--gold); }
    .toplinks { display: flex; gap: 22px; }
    .toplinks a { text-decoration: none; color: var(--muted); font-size: 0.92rem; transition: color .2s; }
    .toplinks a:hover, .toplinks a[aria-current] { color: var(--ember-hi); }
    @media (max-width: 640px) { .toplinks { display: none; } }
    .hero { position: relative; overflow: hidden;
      background: radial-gradient(ellipse at 50% 35%, rgba(11,10,9,0) 0%, rgba(11,10,9,.6) 55%, rgba(11,10,9,1) 100%),
        linear-gradient(180deg, rgba(11,10,9,.45) 0%, rgba(11,10,9,.3) 40%, rgba(11,10,9,1) 100%),
        url("/img/tunaria.webp?v=3") center 30% / cover no-repeat; }
    .hero::after { content: ""; position: absolute; inset: 0; pointer-events: none; background: radial-gradient(circle at 50% 120%, rgba(192,83,47,.22), transparent 55%); }
    .hero .wrap { position: relative; z-index: 2; text-align: center; padding-top: 120px; padding-bottom: 56px; }
    .eyebrow { color: var(--ember-hi); font-size: 0.82rem; letter-spacing: 0.32em; text-transform: uppercase; margin-bottom: 14px; }
    .hero h1 { font-size: clamp(2.2rem, 6vw, 4rem); font-weight: 900; line-height: 1.05; color: #fff; text-shadow: 0 4px 30px rgba(0,0,0,.8), 0 0 60px rgba(192,83,47,.3); }
    .hero h1 em { font-style: normal; color: var(--ember-hi); }
    .hero .tagline { max-width: 640px; margin: 18px auto 0; color: #d6cdbf; font-size: 1.08rem; text-shadow: 0 2px 12px rgba(0,0,0,.9); }
    .jump { display: flex; flex-wrap: wrap; gap: 8px; justify-content: center; margin-top: 26px; }
    .jump a { text-decoration: none; color: #d8d0c4; font-size: .88rem; padding: 7px 13px; border-radius: 99px; background: var(--panel); border: 1px solid var(--line); }
    .jump a:hover { border-color: var(--ember); color: var(--ember-hi); }
    section { padding: 56px 0 8px; }
    .section-head { text-align: center; margin-bottom: 28px; }
    .section-head h2 { font-size: clamp(1.6rem, 4vw, 2.3rem); font-weight: 700; color: #fff; }
    .section-head p { color: var(--muted); max-width: 640px; margin: 10px auto 0; }
    .rule { width: 120px; height: 1px; margin: 16px auto 0; background: linear-gradient(90deg, transparent, var(--ember), transparent); }
    h3.shard { font-size: 1.15rem; color: var(--gold-hi); margin: 26px 0 0; }
    .tablewrap { overflow-x: auto; margin: 12px 0 26px; border: 1px solid var(--line); border-radius: 12px; background: var(--panel); box-shadow: inset 3px 0 0 rgba(192,83,47,.55); }
    table { width: 100%; border-collapse: collapse; font-size: .92rem; }
    th { text-align: left; font-weight: 600; color: var(--muted); font-size: .78rem; text-transform: uppercase; letter-spacing: .05em; padding: 12px 10px; border-bottom: 1px solid var(--line); background: var(--panel-2); white-space: nowrap; }
    td { padding: 10px; border-bottom: 1px solid var(--line); vertical-align: top; }
    tr:last-child td { border-bottom: 0; }
    th.n, td.n { text-align: right; white-space: nowrap; }
    td.rk { color: var(--muted); width: 2.5em; }
    tr.top td.rk { color: var(--ember-hi); font-weight: 700; }
    td.who b { color: #fff; }
    td.who b.dead { color: var(--ember-hi); }
    td.when { white-space: nowrap; font-variant-numeric: tabular-nums; }
    .none { color: var(--muted); text-align: center; padding: 14px 0 22px; }
    .note { color: var(--muted); font-size: .85rem; text-align: center; margin: 20px auto 40px; }
    @media (max-width: 760px) { th.k, td.k { display: none; } td, th { padding: 8px 6px; } table { font-size: .86rem; } }
    footer { border-top: 1px solid var(--line); padding: 34px 0 50px; color: var(--muted); font-size: .88rem; text-align: center; margin-top: 40px; }
    footer a { color: var(--muted); margin: 0 10px; }
    footer a:hover { color: var(--ember-hi); }
  </style>
</head>
<body>
  <header class="topbar">
    <div class="wrap">
      <a class="brand" href="/">eqoa<span>classic</span></a>
      <nav class="toplinks">
        <a href="/#preservation">About</a>
        <a href="/#realms">Servers</a>
        <a href="/#gallery">Gallery</a>
        <a href="/commands.html">Commands</a>
        <a href="/contributors.html">Thanks</a>
        <a href="/eqoa/hardcore" aria-current="page">Hardcore</a>
        <a href="/eqoa/play/">Play now</a>
      </nav>
    </div>
  </header>
  <div class="hero">
    <div class="wrap">
      <div class="eyebrow">You only live once</div>
      <h1><em>Hardcore</em> Heroes</h1>
      <p class="tagline">A Hardcore character is chosen when it is made, wears [HC] before its name, and its first death is its last: no respawn, no resurrection.</p>
      <div class="jump"><a href="#living">Living Leaderboard</a><a href="#fallen">Memorial of the Fallen</a></div>
    </div>
  </div>
  <main class="wrap">
`

const pageFoot = `
  <footer>
    <div class="wrap">
      <p>eqoaclassic.com: a fan-run EQOA preservation project. Free, no monetization, and not affiliated with Daybreak Game Company or Sony. EQOA belongs to its owners; we distribute none of the game.</p>
      <p style="margin-top:10px"><a href="/eqoa/play/">Play</a><a href="/#realms">Servers</a><a href="/commands.html">Commands</a><a href="/contributors.html">Thanks</a><a href="/eqoa/hardcore">Hardcore</a></p>
    </div>
  </footer>
</body>
</html>
`

// killerWords is the "Slain by" cell: the killer, or the cause in words.
func killerWords(h Hero) string {
	switch h.Cause {
	case entity.CauseDrowning:
		return "drowning"
	case entity.CauseLava:
		return "lava"
	case entity.CauseSun:
		return "the sun"
	case entity.CauseHazard:
		return h.Killer + " (gave way)"
	case entity.CauseSelf:
		return "their own spell"
	case entity.CauseGM:
		return "a GM"
	case entity.CauseGMEvent:
		return h.Killer + " (an event)"
	case entity.CausePvP:
		return h.Killer + " (PvP)"
	}
	return h.Killer
}
