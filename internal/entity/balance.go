package entity

import (
	"log"
	"math"
	"math/rand"
	"sort"
	"time"

	"github.com/eqoa/server/internal/database"
)

// balance.go — the game's balance, the GMs' to tune: tables balancelevel
// and balancecon (database/balance.go; the GM panel's "Balance by level"
// and "Balance by con", live). Sandstorm's measured formulas stay as they
// are (offence.go, defence.go, the players' stats); this scales what goes
// into a fight and what comes out of it, in one place:
//
//   - an NPC's hit points, by its level (applyBalanceLocked, at its spawn
//     and whenever the table changes);
//   - an NPC's melee hits, by its level and by its con on the one it hits
//     (a player or a pet): npcHitLocked; on a player, then the share its
//     class takes (balanceclass): npcHitPlayerLocked;
//   - what a player or a pet does to an NPC (melee, spells and their ticks,
//     procs, damage shields, a pet's hits, taunts and taps), by the NPC's
//     con on the attacker: takenLocked;
//   - a summoned pet's hit points and all its damage, by its owner's level,
//     and its fights con as its owner's would (balanceLevelOf): the spells'
//     pets lag their owners by up to thirteen levels at 60 (a magician's
//     last pet of its levels is Ghilan, 47), and a pet is its owner's
//     weapon. A charmed creature is still a creature: its own level's.
//   - a root on an NPC, by its con on the root's caster: resisted at its
//     landing, freed by a hit (root.go);
//   - the combat arts' global cooldown, by the player's level (ArtCooldown):
//     an art (an ability with no cast time cast at a creature) started keeps
//     every other art from starting for that long; one sent in it is not
//     answered, as a cast sent while another is on (StartCast).
//
// The con is the NPC's level less its foe's (calcConColor's: 1-2 yellow, 3
// and more red). Training dummies are left as measured. Why the seed rows
// are what they are: claude-notes/balance-20261029.md, and the simulator
// that holds them to their targets (balancesim.go, balancesim_test.go).

// meleeIntn is the melee rolls' dice: a player's hit on an NPC, an NPC's
// on a player or a pet, a pet's on an NPC (a test's and the fight
// simulator's own, balancesim.go).
var meleeIntn = rand.Intn

// spellIntn is the dice of a spell's amount in its range (spellAmount).
var spellIntn = rand.Intn

// Balance is the tables, read.
type Balance struct {
	levels  []database.BalanceLevel // by level
	cons    []database.BalanceCon   // by diff
	classes map[int]database.BalanceClass
}

// NewBalance is a balance of these rows (any order); none changes nothing.
func NewBalance(levels []database.BalanceLevel, cons []database.BalanceCon) *Balance {
	b := &Balance{levels: append([]database.BalanceLevel(nil), levels...), cons: append([]database.BalanceCon(nil), cons...)}
	sort.Slice(b.levels, func(i, j int) bool { return b.levels[i].Level < b.levels[j].Level })
	sort.Slice(b.cons, func(i, j int) bool { return b.cons[i].Diff < b.cons[j].Diff })
	return b
}

// WithClasses is the balance with these class rows (balanceclass) in place
// of its own.
func (b *Balance) WithClasses(rows []database.BalanceClass) *Balance {
	ls, cs := b.Rows()
	out := NewBalance(ls, cs)
	out.classes = map[int]database.BalanceClass{}
	for _, r := range rows {
		out.classes[r.Class] = r
	}
	return out
}

// Rows is the balance's rows, by level and by diff (none for a nil balance).
func (b *Balance) Rows() ([]database.BalanceLevel, []database.BalanceCon) {
	if b == nil {
		return nil, nil
	}
	return append([]database.BalanceLevel(nil), b.levels...), append([]database.BalanceCon(nil), b.cons...)
}

// ClassRows is the balance's class rows, by class.
func (b *Balance) ClassRows() []database.BalanceClass {
	if b == nil {
		return nil
	}
	var out []database.BalanceClass
	for _, r := range b.classes {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Class < out[j].Class })
	return out
}

// MeleeTakenPct is the share of an NPC's melee hit a player of the class
// takes (balanceclass), 100 with no row.
func (b *Balance) MeleeTakenPct(class int) float64 {
	if b == nil {
		return 100
	}
	if r, ok := b.classes[class]; ok {
		return r.MeleeTakenPct
	}
	return 100
}

// curve is the line through points (x, y) at x: between two points the
// straight line, past the ends the end's (as the tables read).
func curve(points [][2]float64, x float64) float64 {
	if len(points) == 0 {
		return 0
	}
	sort.Slice(points, func(i, j int) bool { return points[i][0] < points[j][0] })
	if x <= points[0][0] {
		return points[0][1]
	}
	for i := 1; i < len(points); i++ {
		if a, c := points[i-1], points[i]; x <= c[0] {
			if c[0] == a[0] {
				return c[1]
			}
			return a[1] + (c[1]-a[1])*(x-a[0])/(c[0]-a[0])
		}
	}
	return points[len(points)-1][1]
}

// WithLevelColumn is the balance with one column of its level table made the
// line through points (level, value): rows are added at the points' levels
// (their other columns the table's there), and every row's column is set
// (cmd/balancesim's -try flags: rows tried before they are written).
func (b *Balance) WithLevelColumn(points [][2]float64, set func(*database.BalanceLevel, float64)) *Balance {
	ls, cs := b.Rows()
	have := map[int]bool{}
	for _, r := range ls {
		have[r.Level] = true
	}
	for _, p := range points {
		if l := int(p[0]); !have[l] {
			have[l] = true
			ls = append(ls, b.Level(l))
		}
	}
	for i := range ls {
		set(&ls[i], curve(points, float64(ls[i].Level)))
	}
	return NewBalance(ls, cs).WithClasses(b.ClassRows())
}

// WithConColumn is WithLevelColumn for the con table: one column of it made
// the line through points (diff, value).
func (b *Balance) WithConColumn(points [][2]float64, set func(*database.BalanceCon, float64)) *Balance {
	ls, cs := b.Rows()
	have := map[int]bool{}
	for _, r := range cs {
		have[r.Diff] = true
	}
	for _, p := range points {
		if d := int(p[0]); !have[d] {
			have[d] = true
			cs = append(cs, b.Con(d))
		}
	}
	for i := range cs {
		set(&cs[i], curve(points, float64(cs[i].Diff)))
	}
	return NewBalance(ls, cs).WithClasses(b.ClassRows())
}

// SeedBalance is the seed rows' balance (database.BalanceSeedLevels and
// BalanceSeedCons): what a database made today starts with.
func SeedBalance() *Balance {
	return NewBalance(database.BalanceSeedLevels, database.BalanceSeedCons).WithClasses(database.BalanceSeedClasses)
}

// between is the line through (x0, y0) and (x1, y1) at x.
func between(x, x0, x1 int, y0, y1 float64) float64 {
	if x1 == x0 {
		return y0
	}
	return y0 + (y1-y0)*float64(x-x0)/float64(x1-x0)
}

// Level is the level table at an NPC's level: its row's, or the line
// between the rows on either side, or the nearest end's; every percent 100
// with no rows.
func (b *Balance) Level(level int) database.BalanceLevel {
	out := database.BalanceLevel{Level: level, HPPct: 100, DamagePct: 100, ConPct: 100, PetHPPct: 100, PetDamagePct: 100}
	if b == nil || len(b.levels) == 0 {
		return out
	}
	rows := b.levels
	i := sort.Search(len(rows), func(i int) bool { return rows[i].Level >= level })
	switch {
	case i == len(rows):
		out = rows[len(rows)-1]
	case rows[i].Level == level || i == 0:
		out = rows[i]
	default:
		a, c := rows[i-1], rows[i]
		out.HPPct = between(level, a.Level, c.Level, a.HPPct, c.HPPct)
		out.DamagePct = between(level, a.Level, c.Level, a.DamagePct, c.DamagePct)
		out.ConPct = between(level, a.Level, c.Level, a.ConPct, c.ConPct)
		out.PetHPPct = between(level, a.Level, c.Level, a.PetHPPct, c.PetHPPct)
		out.PetDamagePct = between(level, a.Level, c.Level, a.PetDamagePct, c.PetDamagePct)
		out.ArtCooldownMS = between(level, a.Level, c.Level, a.ArtCooldownMS, c.ArtCooldownMS)
	}
	out.Level = level
	return out
}

// ArtCooldown is the combat arts' global cooldown for a player of the level
// (balancelevel's art_cooldown_ms, by the player's level): how long after
// one art starts before another may (isArt). None with no rows.
func (b *Balance) ArtCooldown(level int) time.Duration {
	if b == nil || len(b.levels) == 0 {
		return 0
	}
	return time.Duration(math.Round(b.Level(level).ArtCooldownMS)) * time.Millisecond
}

// isArt is whether a spell is a combat art for the global cooldown: an
// ability with no cast time cast at a creature (a Kick, a Bash, a strike).
func isArt(s *CastSpell) bool {
	return s.CastTime <= 0 && s.Def.Harmful
}

// Con is the con table at a level difference (the NPC's less its foe's), as
// Level reads the level table.
func (b *Balance) Con(diff int) database.BalanceCon {
	out := database.BalanceCon{Diff: diff, DamagePct: 100, TakenPct: 100}
	if b == nil || len(b.cons) == 0 {
		return out
	}
	rows := b.cons
	i := sort.Search(len(rows), func(i int) bool { return rows[i].Diff >= diff })
	switch {
	case i == len(rows):
		out = rows[len(rows)-1]
	case rows[i].Diff == diff || i == 0:
		out = rows[i]
	default:
		a, c := rows[i-1], rows[i]
		out.DamagePct = between(diff, a.Diff, c.Diff, a.DamagePct, c.DamagePct)
		out.TakenPct = between(diff, a.Diff, c.Diff, a.TakenPct, c.TakenPct)
		out.RootResistPct = between(diff, a.Diff, c.Diff, a.RootResistPct, c.RootResistPct)
		out.RootBreakPct = between(diff, a.Diff, c.Diff, a.RootBreakPct, c.RootBreakPct)
	}
	out.Diff = diff
	return out
}

// pct is n times p percent, rounded, at least 1 when n is.
func pct(n int, p float64) int {
	if n <= 0 || p == 100 {
		return n
	}
	return max(int(math.Round(float64(n)*p/100)), 1)
}

// NPCMaxHP is an NPC's hit points at its level under the balance.
func (b *Balance) NPCMaxHP(level int) int {
	return pct(NPCMaxHP(level), b.Level(level).HPPct)
}

// PetMaxHP is a summoned pet's of its level.
func (b *Balance) PetMaxHP(level int) int {
	return pct(NPCMaxHP(level), b.Level(level).PetHPPct)
}

// SetBalance puts a balance in place (nil: none) and gives every NPC the hit
// points it makes for its level, each keeping its share of them.
func (w *World) SetBalance(b *Balance) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.balance = b
	for _, npc := range w.NPCs {
		w.rebalanceLocked(npc)
	}
}

// LoadBalance reads the tables into the world (SetBalance): how many rows
// the level and con tables have.
func (w *World) LoadBalance(db *database.SQLiteDB) (levels, cons int, err error) {
	ls, err := db.BalanceLevels()
	if err != nil {
		return 0, 0, err
	}
	cs, err := db.BalanceCons()
	if err != nil {
		return 0, 0, err
	}
	cls, err := db.BalanceClasses()
	if err != nil {
		return 0, 0, err
	}
	w.SetBalance(NewBalance(ls, cs).WithClasses(cls))
	log.Printf("Balance: %d level rows, %d con rows, %d class rows", len(ls), len(cs), len(cls))
	return len(ls), len(cs), nil
}

// balanced is whether the balance acts on the NPC: not a training dummy
// (dummy.go), whose fights are Sandstorm's as measured.
func balanced(npc *NPC) bool { return npc != nil && npc.Dummy == NotDummy }

// summoned is whether the NPC is a summoned pet (a charmed creature is not).
func summoned(npc *NPC) bool { return npc.Owner != 0 && npc.Charm == nil }

// balanceLevelOf is the level the balance takes an NPC at: a summoned pet's
// owner's (while the owner is in the world), anything else its own.
func (w *World) balanceLevelOf(npc *NPC) int {
	if summoned(npc) {
		if o := w.Players[npc.Owner]; o != nil {
			return o.Level
		}
	}
	return npc.Level
}

// applyBalanceLocked gives an NPC the hit points the balance gives its level
// (a summoned pet its pet's of its owner's level: the base its own level's),
// whole: at its spawn (applyNPCSettingsLocked) or its summoning.
func (w *World) applyBalanceLocked(npc *NPC) {
	if !balanced(npc) {
		return
	}
	if summoned(npc) {
		npc.MaxHP = pct(NPCMaxHP(npc.Level), w.balance.Level(w.balanceLevelOf(npc)).PetHPPct)
	} else {
		npc.MaxHP = w.balance.NPCMaxHP(npc.Level)
		if t := npc.tmpl; t != nil {
			// its class template's hit points on the balance's
			// (classtemplate.go keeps its base in tbase)
			npc.tbase.maxHP = npc.MaxHP
			npc.MaxHP = max(1, int(math.Round(float64(npc.MaxHP)*float64(t.HPPct)/100)))
		}
	}
	npc.HP = npc.MaxHP
}

// rebalanceLocked is applyBalanceLocked keeping the NPC's share of its hit
// points (a table changed under a fight; a corpse stays at 0).
func (w *World) rebalanceLocked(npc *NPC) {
	if !balanced(npc) || npc.MaxHP <= 0 {
		return
	}
	share := float64(npc.HP) / float64(npc.MaxHP)
	w.applyBalanceLocked(npc)
	if npc.Dead {
		npc.HP = 0
		return
	}
	npc.HP = max(int(math.Round(share*float64(npc.MaxHP))), min(1, npc.HP))
}

// ConAt is the con table at a diff as an NPC of the level takes it: its
// level's con_pct of the way from 100 to the table's percents.
func (b *Balance) ConAt(level, diff int) database.BalanceCon {
	c := b.Con(diff)
	share := b.Level(level).ConPct / 100
	c.DamagePct = 100 + (c.DamagePct-100)*share
	c.TakenPct = 100 + (c.TakenPct-100)*share
	return c
}

// npcHitLocked is an NPC's melee hit of n on a foe of the level: times its
// level's damage and its con's on that foe.
func (w *World) npcHitLocked(npc *NPC, foeLevel, n int) int {
	if !balanced(npc) {
		return n
	}
	b := w.balance
	return pct(pct(n, b.Level(npc.Level).DamagePct), b.ConAt(npc.Level, npc.Level-foeLevel).DamagePct)
}

// npcHitPlayerLocked is an NPC's melee hit of n on a player: npcHitLocked's,
// then the share the player's class takes (balanceclass).
func (w *World) npcHitPlayerLocked(npc *NPC, p *Player, n int) int {
	n = w.npcHitLocked(npc, p.Level, n)
	if !balanced(npc) {
		return n
	}
	return pct(n, w.balance.MeleeTakenPct(p.Class))
}

// takenLocked is n of damage on an NPC from an attacker of the level: times
// what the NPC's con on that attacker lets through.
func (w *World) takenLocked(npc *NPC, attackerLevel, n int) int {
	if !balanced(npc) {
		return n
	}
	return pct(n, w.balance.ConAt(npc.Level, npc.Level-attackerLevel).TakenPct)
}

// petDealtLocked is n of a pet's damage on an NPC: a summoned pet's times its
// owner's level's pet damage (a charmed creature's own, an NPC's: none),
// then as takenLocked at the pet's balance level (balanceLevelOf).
func (w *World) petDealtLocked(pet, npc *NPC, n int) int {
	if !balanced(npc) {
		return n
	}
	lvl := w.balanceLevelOf(pet)
	if summoned(pet) {
		n = pct(n, w.balance.Level(lvl).PetDamagePct)
	}
	return w.takenLocked(npc, lvl, n)
}
