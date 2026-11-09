package handlers

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A rehabilitator's templates (petition 1322, jibrish: "Add a save rehab
// template option to the rehabilitators. They should allow people to freely
// respec, but also allow them to save 2 different rehab state and CM
// templates. Note they will gain CM's as they go so the template should only
// apply when they had spent if they saved it. For example they rehab, spend
// TP's and 500 of their 700 cm's. They talk to rehab, hit save template, pick
// a slot and the rehaber remembers those purchases. If they come back with 705
// cm's and pick that template, it still only spends the 500 and just
// unassigns the other points for them to spend as they wish").
//
// The menu's third answer (rehab.go), in the same boxes a PS2 reads (0x0046
// lines, 0x0034 questions under rehabBoxMark):
//
//   - what the two slots hold, then save or use;
//   - save: which slot (each answer what it holds), what is kept and what it
//     replaces, "Shall I remember it?": the training points each stat has taken
//     (the row's tp columns) and the ranks held with what each cost, in the
//     order bought, kept in table rehabtemplates (database/rehabtemplates.go);
//   - use: which slot, what it gives back and spends again, what stays unspent,
//     what it cannot buy and why, "Shall I begin?": every training point and
//     mastery given back and spent again as the slot keeps them, never more
//     than it spent (entity.World.ApplyRespec), free, counted as a "template"
//     reset (charresets).
//
// A question is done only on the terms it was asked on: anything changed
// meanwhile (a point earned, a rank bought, the slot saved again) asks it
// again with what is true now. The client is told as the window's own
// messages tell it, the change and no more: a rank no longer held as a removal
// (0x1402 sub-op 6, its abilities out of the book and those it replaced back),
// a rank held anew as a purchase (sub-op 5, its abilities in), then the points
// (sub-op 3); a rank held before and after is not touched, so its abilities
// keep their places in the book and on the bars. The training as a trainer's:
// 0x001D with the unspent points' change, the stat channel at once.

// templateStore is what using a template keeps of a character
// (database.SQLiteCharacterStore).
type templateStore interface {
	OwnedCMsInOrder(serverID uint32) ([]database.OwnedCM, error)
	RespendTraining(serverID uint32, from, to [7]int, unused, total int) error
	SetOwnedCMs(serverID uint32, remove []int, put []database.OwnedCM, m database.MasteryRow) error
}

// What a rehabilitator says of templates (ours). A box holds 217 characters
// with the speaker's name (splitSay cuts a longer line after a sentence), a
// question 127, an answer 63 (quests.go).
const (
	rehabTemplatesAnswer = "Save or use a template"
	rehabTplAbout        = "I can remember how your training points and Combat Masteries are spent, in two templates, and spend you so again, free. What you hold past a template stays yours to spend."
	rehabAskTemplates    = "Which shall it be?"
	rehabTplSaveLabel    = "Save a template"
	rehabTplUseLabel     = "Use a template"
	rehabAskSaveSlot     = "Which slot shall hold it?"
	rehabAskUseSlot      = "Which template shall I spend you by?"
	rehabAskRemember     = "Shall I remember it?"
	rehabRememberYes     = "Yes, remember it."
	rehabNothingToKeep   = "You have spent no training points and hold no Combat Mastery: there is nothing for me to remember."
	rehabNoTemplates     = "I remember no template of yours yet: save one first."
	rehabSlotEmpty       = "Slot %d holds nothing yet: save a template in it first."
	rehabAlreadySpent    = "You are spent as slot %d keeps you already: there is nothing to change."
	rehabTplMoved        = "Something has changed since I asked."
)

// templatesServed is whether this server keeps templates: the rows, the
// characters' store and the masteries.
func (r *Rehab) templatesServed() bool {
	return r != nil && r.tpl != nil && r.cm != nil && r.chars != nil && r.rows != nil && r.gw != nil
}

// --- what is said of a template ---

// countOfThing is "1 mastery" or "12 masteries".
func countOfThing(n int, one, many string) string {
	return fmt.Sprintf("%d %s", n, plural(n, one, many))
}

// masterClassIn is the Master Class among ranks ("" for none): its mastery's
// name, else the rank's.
func (r *Rehab) masterClassIn(ids []int) string {
	for _, id := range ids {
		if rk := r.gw.CMRankByID(id); rk != nil && rk.Category == entity.CMCategoryMaster {
			if rk.Mastery != "" {
				return rk.Mastery
			}
			return rk.Name
		}
	}
	return ""
}

// spendText says a spending: "74 training points (STR 30, STA 20) and 12
// masteries for 500 points (Master Class: Destroyer)", either part alone, or
// "nothing".
func (r *Rehab) spendText(tp [7]int, cms []database.OwnedCM) string {
	total := 0
	for _, v := range tp {
		total += max(v, 0)
	}
	var parts []string
	if total > 0 {
		parts = append(parts, fmt.Sprintf("%s (%s)", countOfThing(total, "training point", "training points"), spentLine(tp)))
	}
	if len(cms) > 0 {
		cost := 0
		ids := make([]int, len(cms))
		for i, o := range cms {
			cost += o.Cost
			ids[i] = o.ID
		}
		m := fmt.Sprintf("%s for %s", countOfThing(len(cms), "mastery", "masteries"), countOfThing(cost, "point", "points"))
		if mc := r.masterClassIn(ids); mc != "" {
			m += " (Master Class: " + mc + ")"
		}
		parts = append(parts, m)
	}
	if len(parts) == 0 {
		return "nothing"
	}
	return strings.Join(parts, " and ")
}

// slotLabel is a slot as its answer offers it: "Slot 1: 12 masteries (500
// points), 74 training points", "Slot 2: empty".
func slotLabel(slot int, t database.RehabTemplate, ok bool) string {
	if !ok || t.Empty() {
		return fmt.Sprintf("Slot %d: empty", slot)
	}
	var parts []string
	if len(t.CMs) > 0 {
		parts = append(parts, fmt.Sprintf("%s (%s)", countOfThing(len(t.CMs), "mastery", "masteries"),
			countOfThing(t.CMTotal(), "point", "points")))
	}
	if n := t.TPTotal(); n > 0 {
		parts = append(parts, countOfThing(n, "training point", "training points"))
	}
	return fmt.Sprintf("Slot %d: %s", slot, strings.Join(parts, ", "))
}

// slotHolds is what a slot holds, said whole: its spending, the level and
// the day it was kept.
func (r *Rehab) slotHolds(slot int, t database.RehabTemplate, ok bool) string {
	if !ok || t.Empty() {
		return fmt.Sprintf("Slot %d holds nothing yet.", slot)
	}
	when := ""
	if len(t.Saved) >= 10 {
		when = " on " + t.Saved[:10]
	}
	return fmt.Sprintf("Slot %d holds %s, kept at level %d%s.", slot, r.spendText(t.TP, t.CMs), t.Level, when)
}

// --- the menu and the slots ---

// templates is the character's templates by slot; false (said) when they
// cannot be read.
func (r *Rehab) templates(sess *session.Session, p *entity.Player, speaker string) (map[int]database.RehabTemplate, bool) {
	tpls, err := r.rows.RehabTemplates(p.CharServerID)
	if err != nil {
		log.Printf("Rehab: %s's templates: %v", p.Name, err)
		r.say(sess, speaker, rehabTrouble)
		return nil, false
	}
	return tpls, true
}

// templatesMenu says what the slots hold and asks save or use.
func (r *Rehab) templatesMenu(sess *session.Session, p *entity.Player, speaker string) {
	if !r.templatesServed() {
		return
	}
	tpls, ok := r.templates(sess, p, speaker)
	if !ok {
		return
	}
	lines := []string{rehabTplAbout}
	for slot := 1; slot <= database.RehabTemplateSlots; slot++ {
		t, has := tpls[slot]
		lines = append(lines, r.slotHolds(slot, t, has))
	}
	r.say(sess, speaker, lines...)
	r.ask(sess, p, &rehabBox{speaker: speaker, stage: rehabStageMenu, options: []int{rehabTplSave, rehabTplUse}},
		rehabAskTemplates, []string{rehabTplSaveLabel, rehabTplUseLabel})
}

// askSlot asks which slot to save in or use (what), each answer what it holds.
func (r *Rehab) askSlot(sess *session.Session, p *entity.Player, speaker string, what int) {
	if !r.templatesServed() {
		return
	}
	tpls, ok := r.templates(sess, p, speaker)
	if !ok {
		return
	}
	var labels []string
	var slots []int
	some := false
	for slot := 1; slot <= database.RehabTemplateSlots; slot++ {
		t, has := tpls[slot]
		some = some || (has && !t.Empty())
		labels = append(labels, slotLabel(slot, t, has))
		slots = append(slots, slot)
	}
	question := rehabAskSaveSlot
	if what == rehabTplUse {
		if !some {
			r.say(sess, speaker, rehabNoTemplates)
			return
		}
		question = rehabAskUseSlot
	}
	r.ask(sess, p, &rehabBox{speaker: speaker, stage: rehabStageSlot, what: what, options: slots}, question, labels)
}

// slotPicked is a slot answered: saved in or used.
func (r *Rehab) slotPicked(sess *session.Session, p *entity.Player, speaker string, what, slot int) {
	switch what {
	case rehabTplSave:
		r.askSave(sess, p, speaker, slot)
	case rehabTplUse:
		r.askUse(sess, p, speaker, slot)
	}
}

// --- saving ---

// currentSpend is how the character is spent now, as a template keeps it:
// the training points each stat has taken (its row's tp columns, as a reset
// reads them) and the ranks it holds in the world with what each cost, in
// the order its rows bought them (one they lack after, by id).
func (r *Rehab) currentSpend(sess *session.Session, p *entity.Player) (database.RehabTemplate, error) {
	t := database.RehabTemplate{Level: p.Level}
	rec, err := r.chars.GetByServerID(p.CharServerID)
	if err != nil {
		return t, err
	}
	t.TP = database.TrainingSpent([7]int{rec.TPStrength, rec.TPStamina, rec.TPAgility, rec.TPDexterity,
		rec.TPWisdom, rec.TPIntel, rec.TPCharisma})
	owned, ok := r.gw.OwnedCMs(sess)
	if !ok {
		return t, fmt.Errorf("not in the world")
	}
	order, err := r.tpl.OwnedCMsInOrder(p.CharServerID)
	if err != nil {
		return t, err
	}
	for _, o := range order {
		if paid, held := owned[o.ID]; held {
			t.CMs = append(t.CMs, database.OwnedCM{ID: o.ID, Cost: paid})
			delete(owned, o.ID)
		}
	}
	var rest []int
	for id := range owned {
		rest = append(rest, id)
	}
	sort.Ints(rest)
	for _, id := range rest {
		t.CMs = append(t.CMs, database.OwnedCM{ID: id, Cost: owned[id]})
	}
	return t, nil
}

// saveTerms is what saving does, for asking again when it changed: what is
// kept, and what the slot held when asked.
func saveTerms(cur database.RehabTemplate, old database.RehabTemplate, had bool) string {
	return fmt.Sprintf("%v|%s|%v|%s", cur.TP, database.FormatTemplateCMs(cur.CMs), had, old.Saved)
}

// askSave says what saving in slot keeps and what it replaces, and asks.
func (r *Rehab) askSave(sess *session.Session, p *entity.Player, speaker string, slot int) {
	cur, err := r.currentSpend(sess, p)
	if err != nil {
		log.Printf("Rehab: %s's spending: %v", p.Name, err)
		r.say(sess, speaker, rehabTrouble)
		return
	}
	if cur.Empty() {
		r.say(sess, speaker, rehabNothingToKeep)
		return
	}
	tpls, ok := r.templates(sess, p, speaker)
	if !ok {
		return
	}
	old, had := tpls[slot]
	lines := []string{"You have spent " + r.spendText(cur.TP, cur.CMs) + "."}
	if had && !old.Empty() {
		lines = append(lines, fmt.Sprintf("Slot %d holds %s now: it will hold this instead.", slot, r.spendText(old.TP, old.CMs)))
	}
	r.say(sess, speaker, lines...)
	r.ask(sess, p, &rehabBox{speaker: speaker, stage: rehabStageConfirm, what: rehabTplSave, slot: slot,
		tpl: saveTerms(cur, old, had)}, rehabAskRemember, []string{rehabRememberYes, rehabNotNow})
}

// doSave keeps how the character is spent in the slot asked, when that is
// still what was asked (asked again when not).
func (r *Rehab) doSave(sess *session.Session, p *entity.Player, b *rehabBox) {
	cur, err := r.currentSpend(sess, p)
	if err != nil {
		log.Printf("Rehab: %s's spending: %v", p.Name, err)
		r.say(sess, b.speaker, rehabTrouble)
		return
	}
	if cur.Empty() {
		r.say(sess, b.speaker, rehabNothingToKeep)
		return
	}
	tpls, ok := r.templates(sess, p, b.speaker)
	if !ok {
		return
	}
	old, had := tpls[b.slot]
	if saveTerms(cur, old, had) != b.tpl {
		r.say(sess, b.speaker, rehabTplMoved)
		r.askSave(sess, p, b.speaker, b.slot)
		return
	}
	cur.Slot = b.slot
	if err := r.rows.SaveRehabTemplate(p.CharServerID, cur, r.now()); err != nil {
		log.Printf("Rehab: %s's template %d not kept: %v", p.Name, b.slot, err)
		r.say(sess, b.speaker, rehabTrouble)
		return
	}
	r.say(sess, b.speaker, fmt.Sprintf("%s Slot %d holds %s now.", rehabDone, b.slot, r.spendText(cur.TP, cur.CMs)))
	log.Printf("Rehab: %s saved template %d: training %v, masteries %s (level %d)", p.Name, b.slot, cur.TP,
		database.FormatTemplateCMs(cur.CMs), cur.Level)
}

// --- using ---

// templateRanks is a template's ranks as the World plans them.
func templateRanks(t database.RehabTemplate) []entity.CMTemplateRank {
	out := make([]entity.CMTemplateRank, len(t.CMs))
	for i, o := range t.CMs {
		out[i] = entity.CMTemplateRank{ID: o.ID, Cost: o.Cost}
	}
	return out
}

// useInputs is what planning a template's use reads besides the World: what
// each stat has taken (the row's tp columns) and the book and quests a rank
// needs (cmKnowsOf).
func (r *Rehab) useInputs(p *entity.Player) ([7]int, entity.CMKnows, error) {
	rec, err := r.chars.GetByServerID(p.CharServerID)
	if err != nil {
		return [7]int{}, entity.CMKnows{}, err
	}
	from := database.TrainingSpent([7]int{rec.TPStrength, rec.TPStamina, rec.TPAgility, rec.TPDexterity,
		rec.TPWisdom, rec.TPIntel, rec.TPCharisma})
	return from, r.cm.cmKnowsOf(p), nil
}

// useTerms is what using a template does, for asking again when it changed.
func useTerms(pl entity.RespecPlan, t database.RehabTemplate) string {
	var skipped []int
	for _, s := range pl.Skipped {
		skipped = append(skipped, s.ID)
	}
	return fmt.Sprintf("%d|%s|%v|%v|%d|%v|%v|%v|%v|%v", t.Slot, t.Saved, pl.TPFrom, pl.TPTo, pl.TPBefore,
		pl.Bought, skipped, pl.Removed, pl.Before, pl.After)
}

// skippedText says the ranks not bought and why, the first few: "Brawn 2
// (you must have Brawn 1 first) and Godborn (too few mastery points are left
// for it)".
func skippedText(sk []entity.CMSkip) string {
	const most = 6
	var parts []string
	for i, s := range sk {
		if i == most {
			parts = append(parts, fmt.Sprintf("%d more", len(sk)-most))
			break
		}
		why := strings.TrimSuffix(strings.TrimSpace(s.Why), ".")
		if why == "" {
			parts = append(parts, s.Name)
			continue
		}
		parts = append(parts, fmt.Sprintf("%s (%s)", s.Name, strings.ToLower(why[:1])+why[1:]))
	}
	return joinAnd(parts)
}

// planSpent is what a plan spends, as a template's spending.
func planSpent(pl entity.RespecPlan) database.RehabTemplate {
	t := database.RehabTemplate{TP: pl.TPTo}
	for _, b := range pl.Bought {
		t.CMs = append(t.CMs, database.OwnedCM{ID: b.ID, Cost: b.Cost})
	}
	return t
}

// leftText says what stays unspent: "205 of your 705 mastery points and 408
// of your 482 training points stay yours to spend."
func leftText(pl entity.RespecPlan, tail string) string {
	tpAll := pl.TPAfter
	for _, v := range pl.TPTo {
		tpAll += v
	}
	return fmt.Sprintf("%d of your %d mastery points and %d of your %d training points %s.", pl.After.Points,
		pl.After.Points+pl.After.Spent, pl.TPAfter, tpAll, tail)
}

// useLines say what using a template does: what goes back, what is spent
// again, what stays, what is short and what cannot be bought.
func (r *Rehab) useLines(pl entity.RespecPlan, t database.RehabTemplate) []string {
	held := database.RehabTemplate{TP: pl.TPFrom}
	var lines []string
	spend := r.spendText(planSpent(pl).TP, planSpent(pl).CMs)
	if held.TPTotal() > 0 || pl.Held > 0 {
		var back []string
		if n := held.TPTotal(); n > 0 {
			back = append(back, countOfThing(n, "training point", "training points"))
		}
		if pl.Held > 0 {
			back = append(back, fmt.Sprintf("%s (%s)", countOfThing(pl.Held, "mastery", "masteries"),
				countOfThing(pl.Returned, "point", "points")))
		}
		lines = append(lines, fmt.Sprintf("I will give back the %s you have spent, then spend you as slot %d keeps you: %s.",
			strings.Join(back, " and "), t.Slot, spend))
	} else {
		lines = append(lines, fmt.Sprintf("I will spend you as slot %d keeps you: %s.", t.Slot, spend))
	}
	lines = append(lines, leftText(pl, "stay yours to spend"))
	if pl.TPShort {
		want := 0
		for _, v := range pl.TPWant {
			want += v
		}
		lines = append(lines, fmt.Sprintf("You hold %d training points of the %d it spends: each stat takes its share.",
			pl.TPAfter+sum7(pl.TPTo), want))
	}
	if pl.TPCapped {
		lines = append(lines, "A stat it would take past your level's cap stops at the cap.")
	}
	if len(pl.Skipped) > 0 {
		lines = append(lines, fmt.Sprintf("I cannot buy %d of its masteries: %s.", len(pl.Skipped), skippedText(pl.Skipped)))
	}
	return lines
}

// sum7 is the seven stats' points.
func sum7(v [7]int) int {
	n := 0
	for _, x := range v {
		n += x
	}
	return n
}

// template is the slot's template; false (said) when the slot holds none.
func (r *Rehab) template(sess *session.Session, p *entity.Player, speaker string, slot int) (database.RehabTemplate, bool) {
	tpls, ok := r.templates(sess, p, speaker)
	if !ok {
		return database.RehabTemplate{}, false
	}
	t, has := tpls[slot]
	if !has || t.Empty() {
		r.say(sess, speaker, fmt.Sprintf(rehabSlotEmpty, slot))
		return database.RehabTemplate{}, false
	}
	return t, true
}

// planUse is what using the slot's template would do now; false (said) when
// it cannot be planned.
func (r *Rehab) planUse(sess *session.Session, p *entity.Player, speaker string, t database.RehabTemplate) (entity.RespecPlan, bool) {
	from, k, err := r.useInputs(p)
	if err != nil {
		log.Printf("Rehab: %s's row: %v", p.Name, err)
		r.say(sess, speaker, rehabTrouble)
		return entity.RespecPlan{}, false
	}
	pl, ok := r.gw.PlanRespec(sess, from, t.TP, templateRanks(t), k)
	return pl, ok
}

// askUse says what using the slot's template does and asks.
func (r *Rehab) askUse(sess *session.Session, p *entity.Player, speaker string, slot int) {
	t, ok := r.template(sess, p, speaker, slot)
	if !ok {
		return
	}
	pl, ok := r.planUse(sess, p, speaker, t)
	if !ok {
		return
	}
	if !pl.Changes() {
		r.say(sess, speaker, fmt.Sprintf(rehabAlreadySpent, slot))
		return
	}
	r.say(sess, speaker, r.useLines(pl, t)...)
	r.ask(sess, p, &rehabBox{speaker: speaker, stage: rehabStageConfirm, what: rehabTplUse, slot: slot,
		tpl: useTerms(pl, t)}, rehabAskBegin, []string{rehabYes, rehabNotNow})
}

// doUse uses the slot's template, when it still does what was asked (asked
// again when not): the World's (ApplyRespec), then kept (the tp columns, the
// ranks and the points, each in one transaction), the book changed and the
// client told, counted, and said.
func (r *Rehab) doUse(sess *session.Session, p *entity.Player, b *rehabBox) {
	t, ok := r.template(sess, p, b.speaker, b.slot)
	if !ok {
		return
	}
	from, k, err := r.useInputs(p)
	if err != nil {
		log.Printf("Rehab: %s's row: %v", p.Name, err)
		r.say(sess, b.speaker, rehabTrouble)
		return
	}
	pl, ok := r.gw.PlanRespec(sess, from, t.TP, templateRanks(t), k)
	if !ok {
		return
	}
	if useTerms(pl, t) != b.tpl {
		r.say(sess, b.speaker, rehabTplMoved)
		r.askUse(sess, p, b.speaker, b.slot)
		return
	}
	if !pl.Changes() {
		r.say(sess, b.speaker, fmt.Sprintf(rehabAlreadySpent, b.slot))
		return
	}
	pl, st, ok := r.gw.ApplyRespec(sess, from, t.TP, templateRanks(t), k)
	if !ok {
		return
	}
	trouble := false
	if err := r.tpl.RespendTraining(p.CharServerID, pl.TPFrom, pl.TPTo, pl.TPAfter, pl.TPTotal); err != nil {
		log.Printf("Rehab: %s's training by template %d not kept: %v", p.Name, b.slot, err)
		trouble = true
	}
	put := make([]database.OwnedCM, len(pl.Bought))
	for i, x := range pl.Bought {
		put[i] = database.OwnedCM{ID: x.ID, Cost: x.Cost}
	}
	if err := r.tpl.SetOwnedCMs(p.CharServerID, pl.Removed, put, MasteryRow(pl.After)); err != nil {
		log.Printf("Rehab: %s's masteries by template %d not kept: %v", p.Name, b.slot, err)
		trouble = true
	}
	r.cm.templateUsed(sess, p, pl, st)
	if pl.TPBefore != pl.TPAfter {
		SendTrainingPointsSpent(sess, int32(pl.TPBefore-pl.TPAfter))
	}
	sendStatsOf(r.gw, sess)
	r.count(p, database.ResetTemplate)

	lines := []string{fmt.Sprintf("%s You have spent %s.", rehabDone, r.spendText(planSpent(pl).TP, planSpent(pl).CMs)),
		leftText(pl, "are yours to spend")}
	if len(pl.Skipped) > 0 {
		lines = append(lines, "Not bought: "+skippedText(pl.Skipped)+".")
	}
	if trouble {
		lines = append(lines, "Something went wrong keeping it: tell a GM.")
	}
	r.say(sess, b.speaker, lines...)
	log.Printf("Rehab: %s used template %d, free: training %v -> %v (%d -> %d unspent), %d ranks held -> %d bought for %d "+
		"(added %v, removed %v), %d of %d mastery points unspent, skipped %v", p.Name, b.slot, pl.TPFrom, pl.TPTo,
		pl.TPBefore, pl.TPAfter, pl.Held, len(pl.Bought), pl.Paid, pl.Added, pl.Removed, pl.After.Points,
		pl.After.Points+pl.After.Spent, pl.Skipped)
}

// --- the window's messages ---

// templateUsed changes the book as a template used on a character changed
// its ranks (entity.World.ApplyRespec) and tells its client, as a removal and
// a purchase in the window do: a rank no longer held, its abilities out and
// those it replaced back, then Recv06; a rank held anew, its abilities in and
// those it replaces out, then Recv05 (with the points after); then the points
// (Recv03). A rank held before and after is not touched.
func (h *ClassMasteryHandler) templateUsed(who *session.Session, p *entity.Player, pl entity.RespecPlan, st entity.CMState) {
	if h == nil || h.gameWorld == nil {
		return
	}
	for _, id := range pl.Removed {
		if rk := h.gameWorld.CMRankByID(id); rk != nil {
			gives, takes := rk.BookChange()
			h.takeAbilities(who, p, gives)
			h.giveAbilities(who, p, takes)
		}
	}
	for _, id := range pl.Added {
		if rk := h.gameWorld.CMRankByID(id); rk != nil {
			gives, takes := rk.BookChange()
			h.giveAbilities(who, p, gives)
			h.takeAbilities(who, p, takes)
		}
	}
	if len(pl.Removed) == 0 && len(pl.Added) == 0 && pl.Before == pl.After {
		return
	}
	list := h.gameWorld.CMList(who, h.cmKnowsOf(p))
	for _, id := range pl.Removed {
		queueReliable(who, protocol.OpcodeClassMasteryServer, cmBig(func(w *buffer.Writer) {
			w.WriteByte(cmRespRemoved)
			w.WriteUint32(0)
			w.WriteUint32(uint32(id))
			writeCMState(w, st, list)
		}))
	}
	for _, id := range pl.Added {
		queueReliable(who, protocol.OpcodeClassMasteryServer, cmBig(func(w *buffer.Writer) {
			w.WriteByte(cmRespBought)
			w.WriteUint32(0)
			w.WriteUint32(uint32(id))
			w.WriteUint32(uint32(pl.After.Points))
			w.WriteUint32(uint32(pl.After.Spent))
			writeCMState(w, st, list)
		}))
	}
	queueReliable(who, protocol.OpcodeClassMasteryServer, cmPoints(pl.After))
}
