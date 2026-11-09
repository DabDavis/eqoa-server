package entity

import (
	"fmt"
	"sort"

	"github.com/eqoa/server/internal/session"
)

// A rehabilitator's template used (handlers/rehabtemplates.go; petition 1322,
// jibrish: "save 2 different rehab state and CM templates ... If they come
// back with 705 cm's and pick that template, it still only spends the 500 and
// just unassigns the other points for them to spend as they wish"): every
// training point and Combat Mastery a character holds given back, then spent
// again as the template keeps them, never more than it spent. What the
// character holds past that stays unspent.
//
//   - The training first, as the masteries' stat requirements read the base it
//     makes: each stat takes what the template asks, held where its base would
//     pass the level's cap (the trainer's rule, StatCapBase); with fewer points
//     than it asks, each stat takes its share (fitTraining).
//   - Then the masteries, at what each costs now (the catalog's, as the window
//     would charge), in the order the template keeps them (the order bought), in
//     passes until a pass buys nothing, so a rank whose parent, ranks or points
//     spent come later in the list is bought once they are: each one bought as
//     the window buys it (cmBlockLocked's rules, the abilities in the book as the
//     ranks before it leave them). One the character cannot buy any more is left
//     out with the purchase's own words. The template's total is a ceiling: a
//     rank that would take what is spent past what the template spent is left
//     out (a GM raised its cost), as is one the points left cannot pay for.

// CMTemplateRank is a rank a template keeps or a plan buys: its id and what it
// cost (when the template was saved; now, in a plan).
type CMTemplateRank struct{ ID, Cost int }

// CMSkip is a template's rank not bought, and why, in the purchase's words.
type CMSkip struct {
	ID   int
	Name string
	Why  string
}

// Why a template's rank was not bought, past cmBlockLocked's words.
const (
	CMSkipGone      = "That mastery is no longer offered."
	CMSkipNotYours  = "That mastery is not offered to you."
	CMSkipCostlier  = "It costs more now than when the template was saved."
	CMSkipNoPoints  = "Too few mastery points are left for it."
	CMSkipNoCatalog = "No mastery can be bought here."
)

// RespecPlan is what a template comes to on a character as it stands.
type RespecPlan struct {
	// TPFrom is what each stat had taken (the row's tp columns), TPWant what
	// the template asks of each held under the level's cap, TPTo what each
	// takes; TPBefore and TPAfter the unspent points before and after, TPTotal
	// every point given (never below the unspent ones)
	TPFrom, TPWant, TPTo       [7]int
	TPBefore, TPAfter, TPTotal int
	// TPShort: fewer points than it asks, each stat its share; TPCapped: a
	// stat held at the level's cap
	TPShort, TPCapped bool
	// Held is the ranks the character holds now, Returned what they cost
	// (given back); Bought the template's ranks bought, in the order bought,
	// at what each costs now, Paid their sum; Skipped the rest
	Held, Returned int
	Bought         []CMTemplateRank
	Paid           int
	Skipped        []CMSkip
	// Removed are held now and not after, Added after and not now, Kept both
	// (by id; Added in the order bought)
	Removed, Added, Kept []int
	// Before and After are the mastery points
	Before, After Mastery
}

// Changes is whether using the template changes anything: the stats' points,
// a rank held or what one cost.
func (pl RespecPlan) Changes() bool {
	if pl.TPFrom != pl.TPTo || pl.TPBefore != pl.TPAfter || len(pl.Removed) > 0 || len(pl.Added) > 0 {
		return true
	}
	return pl.Before.Points != pl.After.Points || pl.Before.Spent != pl.After.Spent
}

// fitTraining is what each stat takes of avail points when want asks for
// more in all: its share (want × avail / the sum, rounded down), the points
// the rounding leaves to the stats it cut the most (a tie to the first, STR
// before CHA); never more than it asks. short is whether it asked for more.
func fitTraining(want [7]int, avail int) (got [7]int, short bool) {
	sum := 0
	for _, v := range want {
		sum += max(v, 0)
	}
	if sum <= avail {
		for i, v := range want {
			got[i] = max(v, 0)
		}
		return got, false
	}
	if avail <= 0 {
		return got, true
	}
	var rest [7]int
	left := avail
	for i, v := range want {
		v = max(v, 0)
		got[i] = v * avail / sum
		rest[i] = v * avail % sum
		left -= got[i]
	}
	order := []int{0, 1, 2, 3, 4, 5, 6}
	sort.SliceStable(order, func(a, b int) bool { return rest[order[a]] > rest[order[b]] })
	for _, i := range order {
		if left == 0 {
			break
		}
		if rest[i] > 0 {
			got[i]++
			left--
		}
	}
	return got, true
}

// planRespecLocked plans a template on p and changes nothing of it: the plan,
// and the player as the plan leaves it (a scratch copy: base stats, unspent
// training points, ranks, mastery points and what the ranks give). from is
// what each stat had taken as the character's row holds it, saved what the
// template asks, ranks its masteries in its order, k the book and quests as
// they are now. Must be called with the lock held.
func (w *World) planRespecLocked(p *Player, from, saved [7]int, ranks []CMTemplateRank, k CMKnows) (RespecPlan, *Player) {
	pl := RespecPlan{TPFrom: from, TPBefore: p.UnusedTP, Held: len(p.CMs), Before: p.Mastery}

	// the training: the base as born (never below it: TakeBackTraining's
	// rule) plus what the template spends again
	bs := LookupBaseStats(p.Race, p.Class)
	floor := [7]int{bs.Str, bs.Sta, bs.Agi, bs.Dex, bs.Wis, bs.Int, bs.Cha}
	base := [7]int{p.BaseStr, p.BaseSta, p.BaseAgi, p.BaseDex, p.BaseWis, p.BaseInt, p.BaseCha}
	avail := p.UnusedTP
	for i := range base {
		avail += max(from[i], 0)
		base[i] = max(base[i]-max(from[i], 0), floor[i])
	}
	limit := StatCapBase(p.Level)
	for i, v := range saved {
		v = max(v, 0)
		if room := max(limit-base[i], 0); v > room {
			v, pl.TPCapped = room, true
		}
		pl.TPWant[i] = v
	}
	pl.TPTo, pl.TPShort = fitTraining(pl.TPWant, avail)
	spent := 0
	for i, v := range pl.TPTo {
		base[i] += v
		spent += v
	}
	pl.TPAfter = avail - spent
	pl.TPTotal = max(p.TotalTP, pl.TPAfter)

	// the masteries, on a scratch player of p's class, race and level with
	// that base and every mastery point it holds unspent
	s := &Player{Class: p.Class, Race: p.Race, Level: p.Level, Mastery: p.Mastery, CMs: map[int]int{}}
	s.BaseStr, s.BaseSta, s.BaseAgi, s.BaseDex, s.BaseWis, s.BaseInt, s.BaseCha = base[0], base[1], base[2], base[3], base[4], base[5], base[6]
	for _, paid := range p.CMs {
		pl.Returned += paid
	}
	s.Mastery.Points += pl.Returned
	s.Mastery.Spent = max(0, s.Mastery.Spent-pl.Returned)
	s.refreshCMLocked(w.cms)
	// the book as the ranks given back leave it (their abilities out, those
	// they replaced back: ResetOnline's), then as each rank bought changes it
	book := make(map[int]bool, len(k.Book))
	for id, v := range k.Book {
		book[id] = v
	}
	for id := range p.CMs {
		if r := w.cmRank(id); r != nil {
			gives, takes := r.BookChange()
			for _, a := range gives {
				if a.SpellID > 0 {
					delete(book, a.SpellID)
				}
			}
			for _, a := range takes {
				if a.SpellID > 0 {
					book[a.SpellID] = true
				}
			}
		}
	}
	kk := CMKnows{Book: book, Quests: k.Quests, QuestNames: k.QuestNames}
	budget := 0
	for _, t := range ranks {
		budget += max(t.Cost, 0)
	}
	why := map[int]string{}
	name := func(id int) string {
		if r := w.cmRank(id); r != nil {
			return r.Name
		}
		return fmt.Sprintf("mastery %d", id)
	}
	skip := func(id int, reason string) {
		pl.Skipped = append(pl.Skipped, CMSkip{ID: id, Name: name(id), Why: reason})
	}
	var pending []int
	seen := map[int]bool{}
	for _, t := range ranks {
		if t.ID > 0 && !seen[t.ID] {
			seen[t.ID] = true
			pending = append(pending, t.ID)
		}
	}
	if w.cms == nil {
		for _, id := range pending {
			skip(id, CMSkipNoCatalog)
		}
		pending = nil
	}
	for len(pending) > 0 {
		var next []int
		bought := false
		for _, id := range pending {
			r := w.cms.byID[id]
			switch {
			case r == nil || !r.Enabled:
				skip(id, CMSkipGone)
				continue
			case !r.offered(s):
				skip(id, CMSkipNotYours)
				continue
			}
			if b := w.cmBlockLocked(s, r, kk); b != "" {
				why[id] = b // a later pass may meet it
				next = append(next, id)
				continue
			}
			if pl.Paid+r.Cost > budget {
				skip(id, CMSkipCostlier)
				continue
			}
			if s.Mastery.Points < r.Cost {
				skip(id, CMSkipNoPoints)
				continue
			}
			s.Mastery.Points -= r.Cost
			s.Mastery.Spent += r.Cost
			s.CMs[id] = r.Cost
			pl.Paid += r.Cost
			pl.Bought = append(pl.Bought, CMTemplateRank{ID: id, Cost: r.Cost})
			s.refreshCMLocked(w.cms)
			gives, takes := r.BookChange()
			for _, a := range gives {
				if a.SpellID > 0 {
					book[a.SpellID] = true
				}
			}
			for _, a := range takes {
				if a.SpellID > 0 {
					delete(book, a.SpellID)
				}
			}
			bought = true
		}
		pending = next
		if !bought {
			break
		}
	}
	for _, id := range pending {
		skip(id, why[id])
	}
	pl.After = s.Mastery

	for id := range p.CMs {
		if _, ok := s.CMs[id]; ok {
			pl.Kept = append(pl.Kept, id)
		} else {
			pl.Removed = append(pl.Removed, id)
		}
	}
	sort.Ints(pl.Kept)
	sort.Ints(pl.Removed)
	for _, b := range pl.Bought {
		if _, ok := p.CMs[b.ID]; !ok {
			pl.Added = append(pl.Added, b.ID)
		}
	}
	return pl, s
}

// PlanRespec is what a template would do to the session's player
// (planRespecLocked), changing nothing; false when it is not in the world.
func (w *World) PlanRespec(sess *session.Session, from, saved [7]int, ranks []CMTemplateRank, k CMKnows) (RespecPlan, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return RespecPlan{}, false
	}
	pl, _ := w.planRespecLocked(p, from, saved, ranks, k)
	return pl, true
}

// ApplyRespec uses a template on the session's player: planned as PlanRespec
// plans it, then its base stats, training points, ranks and mastery points
// made what the plan makes them, and its stats, caps and run speed worked out
// again (cmChangedLocked: a new speed told). The caller keeps it, changes the
// book (the ranks' abilities) and tells the client. The plan, the masteries'
// state for the window's messages, and false when the player is not in the
// world (nothing done).
func (w *World) ApplyRespec(sess *session.Session, from, saved [7]int, ranks []CMTemplateRank, k CMKnows) (RespecPlan, CMState, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return RespecPlan{}, CMState{}, false
	}
	pl, s := w.planRespecLocked(p, from, saved, ranks, k)
	p.BaseStr, p.BaseSta, p.BaseAgi, p.BaseDex, p.BaseWis, p.BaseInt, p.BaseCha =
		s.BaseStr, s.BaseSta, s.BaseAgi, s.BaseDex, s.BaseWis, s.BaseInt, s.BaseCha
	p.UnusedTP, p.TotalTP = pl.TPAfter, pl.TPTotal
	p.CMs = s.CMs
	p.Mastery.Points, p.Mastery.Spent = s.Mastery.Points, s.Mastery.Spent
	w.cmChangedLocked(p)
	return pl, p.cmStateLocked(), true
}

// OwnedCMs is the ranks the session's player holds and what each cost; false
// when it is not in the world.
func (w *World) OwnedCMs(sess *session.Session) (map[int]int, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return nil, false
	}
	out := make(map[int]int, len(p.CMs))
	for id, paid := range p.CMs {
		out[id] = paid
	}
	return out, true
}

// UnusedTraining is the session's player's unspent training points; false when
// it is not in the world.
func (w *World) UnusedTraining(sess *session.Session) (int, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return 0, false
	}
	return p.UnusedTP, true
}
