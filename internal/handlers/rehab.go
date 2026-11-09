package handlers

import (
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The rehabilitators (the user, 2026-10-30): "Rehabilitator Anja in Muniels
// tea garden and another near freeport NPC, talk to them". Talked to, one
// opens a menu of the quest boxes a PS2 reads (a 0x0046 box a line, a 0x0034
// question with its answers; rehabBoxMark marks their ids), answers as
// CLIENT's VIWndResponse shows a page (its own Close answers 255, nothing):
//
//   - a reset, then which: its Combat Masteries, as #rst_mastr
//     (classmastery.go: every rank taken, its points back, 0x1402 sub-op 6 a
//     rank then 3, word 1885); its stat (training) points, as #rst_both's
//     training half (gmresets.go: 0x001D, the stats channel, word 1886); or
//     both;
//   - a path chosen again (rehabpaths.go): which of its picks, then which of
//     that step's other options, changed on the spot: the old option's spells
//     out of the book and its items taken back, the new one's spells taught
//     and its items given, the pick kept, no quest done again (the user,
//     2026-10-30: "choose again on the spot");
//   - a template saved or used (rehabtemplates.go; petition 1322): how the
//     training points and masteries are spent now kept in one of two slots,
//     or every one given back and spent again as a slot keeps them, never
//     more than it spent.
//
// Each is asked again before it is done, and refused when the character has
// nothing of it, or (a path) wears what the path gave or has no room for what
// the new pick gives (the quests' rule, questroom.go). Every reset is free,
// every time, at every rehabilitator (the user, 2026-11-06: "lets make rehab
// resets free from anja etc"). Until then a character had 1 free mastery
// reset, 1 free stat point reset and 2 free path respecs, then paid tunar
// ("1mil for cms, 2mil for both") and a Rose of Rehabilitation a path ("Roses
// for paths"), sold here for 10,000,000 and given by a class's level 60 epic;
// the GMs' prices and free counts (table rehabsettings) are not read any
// more, and a rose a character holds is not asked for. Each reset is still
// counted, for the record (charresets, as used). The words are ours.

// rehabBoxMark marks a rehabilitator's question's id (0x0034's u32, echoed by
// 0x0035), beside the quests' 0x51, the triggers' 0x52 and the duels' 0x53.
const rehabBoxMark = 0x54000000

// IsRehabBox is whether a box id is a rehabilitator's.
func IsRehabBox(id uint32) bool { return id&0xFF000000 == rehabBoxMark }

// IsRehabilitator is whether an NPC is a rehabilitator: Rehabilitator Anja,
// Rehabilitator Frejo, and any NPC a GM names so.
func IsRehabilitator(name string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(name)), "rehabilitator ")
}

// What a rehabilitator says (ours).
const (
	rehabGreeting    = "Be welcome, %s. What a soul has learned it may set down, and I help it do so, that it may learn anew."
	rehabAskMenu     = "What would you have of me?"
	rehabAskResets   = "Which shall I reset?"
	rehabAsYouWish   = "As you wish."
	rehabAskBegin    = "Shall I begin?"
	rehabYes         = "Yes, begin."
	rehabNotNow      = "Not now."
	rehabMasteryIs   = "I will draw every Combat Mastery out of you, and the points you spent on them come back to be spent again."
	rehabTrainingIs  = "Every training point you have spent comes back to you, and your stats return to what your birth and calling gave you."
	rehabBothIs      = "Every mastery and every training point comes back to you, to be spent as you choose."
	rehabNoMastery   = "You hold no Combat Mastery for me to draw out."
	rehabNoTraining  = "You have spent no training points for me to give back."
	rehabNothing     = "You hold no mastery and have spent no training points: there is nothing for me to undo."
	rehabNoPath      = "You have chosen no path that I can change."
	rehabAskPath     = "Which path will you choose again?"
	rehabAskInstead  = "What will you take instead of %s?"
	rehabNothingElse = "There is nothing else to choose there."
	rehabChangeYes   = "Yes, change it."
	rehabBookFull    = "Your spell book has no room for %s."
	rehabKnowsAll    = "You know every ability this path offers already: none leaves your book."
	rehabTrouble     = "Forgive me, my thoughts are elsewhere. Come back in a little while."
	rehabWorn        = "Take off %s first: what a path gave goes back with it."
	rehabDone        = "It is done."
	rehabAskChange   = "Shall I make the change?"
)

// The menus' answers, and what a confirmation does.
const (
	rehabMastery = iota + 1
	rehabTraining
	rehabBoth
	rehabPath
	rehabResets    // the menu's reset: which is asked
	rehabTemplates // the menu's templates: save or use is asked
	rehabTplSave   // a template saved (rehabtemplates.go)
	rehabTplUse    // a template used
)

// The questions a box asks.
const (
	rehabStageMenu    = iota + 1 // the menu, or which reset
	rehabStagePaths              // which path
	rehabStageNewPick            // which other option of its step
	rehabStageConfirm            // yes or no to what ask says
	rehabStageSlot               // which template slot, to save (what rehabTplSave) or use
)

// rehabTerms is what a reset does to a character as it stands: the parts it
// resets. Two asks agree only on equal terms.
type rehabTerms struct {
	mastery, training bool
}

// rehabBox is the question a rehabilitator asked a character last.
type rehabBox struct {
	id      uint32
	speaker string
	stage   int
	options []int      // the menu: what each answer is; a pick's other options
	picks   []pathPick // the picks offered, in order
	pick    *pathPick  // the pick whose other options are offered
	what    int        // a confirmation: what it does
	terms   rehabTerms // and on what terms it was asked
	swap    *pathSwap  // a path's change, as it was asked
	slot    int        // a template's slot (rehabtemplates.go)
	tpl     string     // and what saving or using it does, as it was asked
}

// rehabRows is what a rehabilitator keeps: each character's counts and
// templates (database.SQLiteDB), and which spells merchants sell as scrolls.
type rehabRows interface {
	CountReset(serverID uint32, kind string, free bool, at time.Time) error
	MerchantScrollNames(icon int) ([]string, error)
	RehabTemplates(serverID uint32) (map[int]database.RehabTemplate, error)
	SaveRehabTemplate(serverID uint32, t database.RehabTemplate, at time.Time) error
}

// Rehab runs the rehabilitators.
type Rehab struct {
	rows   rehabRows
	gw     *entity.World
	inv    *database.InventoryStore
	spells *database.SpellbookStore
	cm     *ClassMasteryHandler
	chars  resetStore          // the characters' rows: the training reset
	tpl    templateStore       // and a template's use (rehabtemplates.go); nil: no templates
	quests func() *QuestEngine // the running quests (Quests)
	now    func() time.Time

	mu    sync.Mutex
	boxes map[uint32]*rehabBox // by character
	seq   uint32
}

// NewRehab is the rehabilitators over the database, the world and the
// stores; cm resets the masteries and takes abilities out of the book.
func NewRehab(db *database.SQLiteDB, gw *entity.World, inv *database.InventoryStore, spells *database.SpellbookStore,
	cm *ClassMasteryHandler, chars resetStore) *Rehab {
	r := &Rehab{rows: db, gw: gw, inv: inv, spells: spells, cm: cm, chars: chars, quests: Quests, now: time.Now,
		boxes: map[uint32]*rehabBox{}}
	if ts, ok := chars.(templateStore); ok {
		r.tpl = ts
	}
	if gw != nil {
		gw.OnPlayerLeaving(func(p *entity.Player) {
			r.mu.Lock()
			delete(r.boxes, p.CharServerID)
			r.mu.Unlock()
		})
	}
	return r
}

// --- talking and answering ---

// Talk is a rehabilitator talked to: the greeting and the menu. It reports
// whether the NPC was one.
func (r *Rehab) Talk(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	if r == nil || p == nil || npc == nil || npc.Dead || !IsRehabilitator(npc.Name) {
		return false
	}
	forgetDialogs(p)
	speaker := strings.TrimSpace(npc.Name)
	r.say(sess, speaker, fmt.Sprintf(rehabGreeting, p.Name))
	r.menu(sess, p, speaker)
	log.Printf("Rehab: %s talked to %s", p.Name, speaker)
	return true
}

// Offer is the greeting and the menu in another's name, with no rehabilitator by:
// a Genie's Lamp's genie's (genie.go).
func (r *Rehab) Offer(sess *session.Session, p *entity.Player, speaker string) {
	if r == nil || p == nil {
		return
	}
	forgetDialogs(p)
	r.say(sess, speaker, fmt.Sprintf(rehabGreeting, p.Name))
	r.menu(sess, p, speaker)
	log.Printf("Rehab: %s asked %s", p.Name, speaker)
}

// Answer is HandleDialogueOption's question for a rehabilitator's box: the
// answer carried out. It reports whether the box was a rehabilitator's (an
// old one does nothing).
func (r *Rehab) Answer(sess *session.Session, p *entity.Player, box uint32, option byte) bool {
	if !IsRehabBox(box) {
		return false
	}
	if r == nil || p == nil || option == 255 {
		// a close is not the last word: the PS2 sends it just before the
		// real pick when two boxes were open (interact.go)
		return true
	}
	r.mu.Lock()
	b := r.boxes[p.CharServerID]
	if b == nil || b.id != box {
		r.mu.Unlock()
		return true
	}
	delete(r.boxes, p.CharServerID)
	r.mu.Unlock()
	i := int(option)
	switch b.stage {
	case rehabStageMenu:
		if i < len(b.options) {
			r.picked(sess, p, b.speaker, b.options[i])
		}
	case rehabStagePaths:
		if i < len(b.picks) {
			r.askNewPick(sess, p, b.speaker, b.picks[i])
		}
	case rehabStageNewPick:
		if i < len(b.options) && b.pick != nil {
			r.askSwap(sess, p, b.speaker, *b.pick, b.options[i])
		}
	case rehabStageSlot:
		if i < len(b.options) {
			r.slotPicked(sess, p, b.speaker, b.what, b.options[i])
		}
	case rehabStageConfirm:
		if i == 0 {
			r.confirmed(sess, p, b)
		} else {
			r.say(sess, b.speaker, rehabAsYouWish)
		}
	}
	return true
}

// picked is an answer of the menu.
func (r *Rehab) picked(sess *session.Session, p *entity.Player, speaker string, what int) {
	switch what {
	case rehabResets:
		r.resetMenu(sess, p, speaker)
	case rehabMastery, rehabTraining, rehabBoth:
		r.askReset(sess, p, speaker, what)
	case rehabPath:
		r.askPaths(sess, p, speaker)
	case rehabTemplates:
		r.templatesMenu(sess, p, speaker)
	case rehabTplSave, rehabTplUse:
		r.askSlot(sess, p, speaker, what)
	}
}

// confirmed is a yes: what was asked, done when its terms still hold (asked
// again on the new terms when not).
func (r *Rehab) confirmed(sess *session.Session, p *entity.Player, b *rehabBox) {
	switch b.what {
	case rehabMastery, rehabTraining, rehabBoth:
		r.doReset(sess, p, b)
	case rehabPath:
		r.doSwap(sess, p, b)
	case rehabTplSave:
		r.doSave(sess, p, b)
	case rehabTplUse:
		r.doUse(sess, p, b)
	}
}

// --- the menu ---

// menu asks what to do: a reset, a path or, where templates are kept, a
// template, three answers of a page of CLIENT's.
func (r *Rehab) menu(sess *session.Session, p *entity.Player, speaker string) {
	labels := []string{"Reset my masteries or stat points", "Choose a path again"}
	options := []int{rehabResets, rehabPath}
	if r.templatesServed() {
		labels, options = append(labels, rehabTemplatesAnswer), append(options, rehabTemplates)
	}
	r.ask(sess, p, &rehabBox{speaker: speaker, stage: rehabStageMenu, options: options}, rehabAskMenu, labels)
}

// resetMenu asks which reset: the masteries, the stat points or both.
func (r *Rehab) resetMenu(sess *session.Session, p *entity.Player, speaker string) {
	labels := []string{"My Combat Masteries", "My stat points", "Both"}
	r.ask(sess, p, &rehabBox{speaker: speaker, stage: rehabStageMenu,
		options: []int{rehabMastery, rehabTraining, rehabBoth}}, rehabAskResets, labels)
}

// --- masteries and stat points ---

// resetParts is what the character has to reset: whether it holds a
// mastery, whether it spent a training point.
func (r *Rehab) resetParts(sess *session.Session, p *entity.Player) (mastery, training bool) {
	mastery = r.gw.CMCount(sess) > 0
	if r.chars != nil {
		if rec, err := r.chars.GetByServerID(p.CharServerID); err == nil {
			spent := database.TrainingSpent([7]int{rec.TPStrength, rec.TPStamina, rec.TPAgility, rec.TPDexterity,
				rec.TPWisdom, rec.TPIntel, rec.TPCharisma})
			training = database.TrainingReset{Spent: spent}.Refund() > 0
		}
	}
	return mastery, training
}

// resetPlan is a reset asked for (rehabMastery, rehabTraining, rehabBoth)
// as the character stands: the parts done, or why nothing.
func (r *Rehab) resetPlan(sess *session.Session, p *entity.Player, what int) (rehabTerms, string) {
	hasM, hasT := r.resetParts(sess, p)
	switch what {
	case rehabMastery:
		if !hasM {
			return rehabTerms{}, rehabNoMastery
		}
		return rehabTerms{mastery: true}, ""
	case rehabTraining:
		if !hasT {
			return rehabTerms{}, rehabNoTraining
		}
		return rehabTerms{training: true}, ""
	}
	if !hasM && !hasT {
		return rehabTerms{}, rehabNothing
	}
	return rehabTerms{mastery: hasM, training: hasT}, ""
}

// askReset asks to reset the masteries, the stat points or both: what it
// does, or why it cannot be done.
func (r *Rehab) askReset(sess *session.Session, p *entity.Player, speaker string, what int) {
	t, no := r.resetPlan(sess, p, what)
	if no != "" {
		r.say(sess, speaker, no)
		return
	}
	var is string
	switch {
	case t.mastery && t.training:
		is = rehabBothIs
	case t.mastery:
		is = rehabMasteryIs
		if what == rehabBoth {
			is = "You have spent no training points, so I will draw out your masteries alone. " + is
		}
	default:
		is = rehabTrainingIs
		if what == rehabBoth {
			is = "You hold no Combat Mastery, so I will give back your training points alone. " + is
		}
	}
	r.say(sess, speaker, is)
	r.ask(sess, p, &rehabBox{speaker: speaker, stage: rehabStageConfirm, what: what, terms: t},
		rehabAskBegin, []string{rehabYes, rehabNotNow})
}

// doReset carries out a reset asked and agreed: the masteries and the stat
// points reset, then each part done counted.
func (r *Rehab) doReset(sess *session.Session, p *entity.Player, b *rehabBox) {
	t, no := r.resetPlan(sess, p, b.what)
	if no != "" {
		r.say(sess, b.speaker, no)
		return
	}
	if t != b.terms {
		r.askReset(sess, p, b.speaker, b.what) // the parts moved: asked again
		return
	}
	var lines []string
	didM, didT, trouble := false, false, false
	if t.mastery {
		res := r.cm.resetMasteries(sess)
		switch {
		case res.refusal != "" || res.ranks == 0:
		case res.notKept != nil:
			didM, trouble = true, true
		default:
			didM = true
			lines = append(lines, fmt.Sprintf("%d %s unlearned, and %d points are yours to spend again.",
				res.ranks, plural(res.ranks, "mastery is", "masteries are"), res.points))
		}
	}
	if t.training && r.chars != nil {
		tr, ok, err := trainingBack(r.gw, r.chars, sess, p.CharServerID)
		switch {
		case !ok:
			if err != nil {
				log.Printf("Rehab: %s's training reset: %v", p.Name, err)
			}
		case err != nil:
			log.Printf("Rehab: the training reset of '%s' not kept: %v", p.Name, err)
			didT, trouble = true, true
		default:
			didT = true
			lines = append(lines, fmt.Sprintf("%d training points are yours to spend again.", tr.Refund()))
		}
	}
	if !didM && !didT {
		r.say(sess, b.speaker, rehabNothing)
		return
	}
	if didM {
		r.count(p, database.ResetMastery)
	}
	if didT {
		r.count(p, database.ResetTraining)
	}
	if trouble {
		lines = append(lines, "Something went wrong keeping it: tell a GM.")
	}
	r.say(sess, b.speaker, rehabDone+" "+strings.Join(lines, " "))
	log.Printf("Rehab: %s reset (masteries %v, stat points %v), free", p.Name, didM, didT)
}

// --- paths ---

// pathPick is a character's pick at a path's step: the quest and its row, the
// step, and the option it has, its kept pick or, from before picks were
// kept, the one the book and the pack name (inferred).
type pathPick struct {
	pq       pathQuest
	step     int
	option   int
	inferred bool
}

// options is the pick's step's options.
func (pk pathPick) options() []database.StepOption {
	if st := pk.pq.quest.Step(pk.step); st != nil {
		return st.Choices.Options
	}
	return nil
}

// label is the pick as the paths' question offers it: its level and answer.
func (pk pathPick) label() string {
	return clipRunes(fmt.Sprintf("Level %d: %s", pk.pq.level, pk.options()[pk.option].Answer), questAnswerChars)
}

// pathSwap is a pick changed for another option of its step on the spot: what
// leaves the book and what enters it, the items taken back and given (an item
// both options give stays), and what stops it (worn, no room) or went missing.
type pathSwap struct {
	pick    pathPick
	to      int
	out, in []entity.CMAbility
	back    []database.ItemCount // held now, not worn
	give    []database.ItemCount
	missing []string // the old option's, held nowhere now (sold, destroyed)
	worn    []string // the old option's, worn: taken off first
	short   string   // the new items the pack has no room for
	granted bool     // the book holds two options' spells nothing else explains: none leaves it
}

// optionSpells is the spells an option teaches, by id and by name (spellKey):
// its spells, and its scrolls' (a level 20 path's), which it taught when
// scribed.
func (r *Rehab) optionSpells(o database.StepOption, names []database.SpellName) (map[int]bool, map[string]bool) {
	ids, keys := map[int]bool{}, map[string]bool{}
	for _, sp := range o.Reward.Spells {
		ids[sp] = true
	}
	for _, it := range o.Reward.Items {
		pat, ok, err := r.inv.ItemPatternByID(it.PatternID)
		if err != nil || !ok || !isScroll(&pat) {
			continue
		}
		if sp, ok := scrollSpell(names, pat.ItemName, pat.LevelReq); ok {
			ids[sp.SpellID] = true
			keys[spellKey(sp.Name)] = true
		}
	}
	return ids, keys
}

// spellNames is every spell's name, for the scrolls (nil without a spell
// book).
func (r *Rehab) spellNames() []database.SpellName {
	if r.spells == nil {
		return nil
	}
	names, err := r.spells.SpellNames()
	if err != nil {
		log.Printf("Rehab: the spells' names: %v", err)
	}
	return names
}

// pickedPaths is the character's picks at its paths' steps, by level, then
// step: each kept pick, and each from before picks were kept that the book
// and the pack name (inferPick); a step they name none or several options of
// is left out.
func (r *Rehab) pickedPaths(p *entity.Player) []pathPick {
	qe := r.quests()
	var out []pathPick
	var book map[int]string
	var owned map[int]int
	var names []database.SpellName
	for _, pq := range qe.pathQuests(p.CharServerID) {
		picks := database.ParsePicks(pq.row.Picks)
		for i := range pq.quest.Steps {
			st := &pq.quest.Steps[i]
			opts := st.Choices.Options
			if len(opts) == 0 || !passed(pq.row, st.Step) {
				continue
			}
			if k, ok := picks[st.Step]; ok && k < len(opts) {
				out = append(out, pathPick{pq: pq, step: st.Step, option: k})
				continue
			}
			if book == nil {
				book, owned = r.bookAndOwned(p)
				names = r.spellNames()
			}
			kept := qe.spellsKeptBut(p.CharServerID, p.Class, pq.quest.ID, st.Step)
			if k := r.inferPick(opts, book, owned, kept, names); k >= 0 {
				out = append(out, pathPick{pq: pq, step: st.Step, option: k, inferred: true})
			}
		}
	}
	return out
}

// inferPick is the option of a step from before picks were kept that the
// character holds something of: its spells in the book (not one another quest
// it keeps gives), its scrolls' spells, its items anywhere, worn too; the one
// that holds the most, -1 when none holds anything or two hold as much.
func (r *Rehab) inferPick(opts []database.StepOption, book map[int]string, owned map[int]int, kept map[int]bool,
	names []database.SpellName) int {
	bookKeys := map[string]bool{}
	for id, n := range book {
		if !kept[id] {
			bookKeys[spellKey(n)] = true
		}
	}
	best, at, tie := 0, -1, false
	for i, o := range opts {
		if o.Stay {
			continue
		}
		score := 0
		ids, keys := r.optionSpells(o, names)
		for id := range ids {
			if _, ok := book[id]; ok && !kept[id] {
				score++
			}
		}
		for k := range keys {
			if bookKeys[k] {
				score++
			}
		}
		for _, it := range o.Reward.Items {
			if owned[it.PatternID] > 0 {
				score++
			}
		}
		switch {
		case score > best:
			best, at, tie = score, i, false
		case score == best && score > 0:
			tie = true
		}
	}
	if tie {
		return -1
	}
	return at
}

// askPaths asks which path to choose again: the character's picks, a step
// each (CLIENT pages them three at a time; its Close is "none").
func (r *Rehab) askPaths(sess *session.Session, p *entity.Player, speaker string) {
	picks := r.pickedPaths(p)
	if len(picks) == 0 {
		r.say(sess, speaker, rehabNoPath)
		return
	}
	if len(picks) > maxDialogOptions {
		picks = picks[len(picks)-maxDialogOptions:] // the latest
	}
	labels := make([]string, len(picks))
	for i, pk := range picks {
		labels[i] = pk.label()
	}
	r.ask(sess, p, &rehabBox{speaker: speaker, stage: rehabStagePaths, picks: picks}, rehabAskPath, labels)
}

// askNewPick asks what to take instead of a pick: its step's other options.
func (r *Rehab) askNewPick(sess *session.Session, p *entity.Player, speaker string, pk pathPick) {
	opts := pk.options()
	var idx []int
	var labels []string
	for i, o := range opts {
		if i == pk.option || o.Stay {
			continue
		}
		idx = append(idx, i)
		labels = append(labels, o.Answer)
	}
	if len(idx) == 0 {
		r.say(sess, speaker, rehabNothingElse)
		return
	}
	r.ask(sess, p, &rehabBox{speaker: speaker, stage: rehabStageNewPick, pick: &pk, options: idx},
		fmt.Sprintf(rehabAskInstead, opts[pk.option].Answer), labels)
}

// planSwap is a pick changed for option to of its step, as the character
// stands now: the pick read again (a GM's change counts), the old option's
// spells out of the book (not one another quest it keeps gives, nor one
// merchants sell as a scroll, nor one the new option teaches too) and its
// items back, the new option's spells into the book and its items given (an
// item both give stays where it is), and the pack tried for the new items once
// the old ones are taken (the quests' rule: questroom.go). Why it cannot be
// planned, when it cannot.
func (r *Rehab) planSwap(sess *session.Session, p *entity.Player, pk pathPick, to int) (*pathSwap, string) {
	var cur *pathPick
	for _, x := range r.pickedPaths(p) {
		if x.pq.quest.ID == pk.pq.quest.ID && x.step == pk.step {
			x := x
			cur = &x
			break
		}
	}
	if cur == nil {
		return nil, rehabNoPath
	}
	opts := cur.options()
	if to < 0 || to >= len(opts) || to == cur.option || opts[to].Stay {
		return nil, rehabNothingElse
	}
	old, nw := opts[cur.option], opts[to]
	u := &pathSwap{pick: *cur, to: to}
	names := r.spellNames()
	oldIDs, oldKeys := r.optionSpells(old, names)
	newIDs, newKeys := r.optionSpells(nw, names)
	kept := r.quests().spellsKeptBut(p.CharServerID, p.Class, cur.pq.quest.ID, cur.step)
	sold := r.soldScrolls()

	// the book: the old option's spells out, the new one's in. A pick teaches
	// one option's spells, so a book holding two options' that nothing else
	// explains (another quest the character keeps, a merchant's scroll) had
	// them from a class grant (#template, a GM's grant_spell "all",
	// world.start_all_spells while it was on), and keeps them all
	known, keyOf, knownKeys := map[int]bool{}, map[int]string{}, map[string]bool{}
	var book []database.CharSpellEntry
	if r.spells != nil {
		if b, err := r.spells.GetCharacterSpells(p.CharServerID); err == nil {
			book = b
		}
	}
	for _, e := range book {
		known[e.Pattern.SpellID] = true
		keyOf[e.Pattern.SpellID] = spellKey(e.Pattern.SpellName)
		knownKeys[spellKey(e.Pattern.SpellName)] = true
	}
	unexplained := 0
	for _, o := range opts {
		if o.Stay {
			continue
		}
		ids, keys := r.optionSpells(o, names)
		if len(ids)+len(keys) == 0 {
			continue
		}
		all, explained := true, false
		for id := range ids {
			all = all && known[id]
			explained = explained || kept[id] || sold[keyOf[id]]
		}
		for k := range keys {
			all = all && knownKeys[k]
			explained = explained || sold[k]
		}
		if all && !explained {
			unexplained++
		}
	}
	u.granted = unexplained > 1
	var stays []database.CharSpellEntry
	for _, e := range book {
		id, key := e.Pattern.SpellID, spellKey(e.Pattern.SpellName)
		if !u.granted && (oldIDs[id] || oldKeys[key]) && !newIDs[id] && !newKeys[key] && !kept[id] && !sold[key] {
			u.out = append(u.out, entity.CMAbility{Name: e.Pattern.SpellName, SpellID: id})
			continue
		}
		stays = append(stays, e)
	}
	for _, sp := range nw.Reward.Spells {
		if known[sp] {
			continue
		}
		name := fmt.Sprintf("spell %d", sp)
		if pat, err := r.spells.SpellPattern(sp); err == nil && pat != nil {
			name = pat.SpellName
		}
		u.in = append(u.in, entity.CMAbility{Name: name, SpellID: sp})
	}
	if len(u.in) > 0 && nextDisplayIndex(stays)+len(u.in) > 252 { // the book's display indices end at 252
		return nil, fmt.Sprintf(rehabBookFull, joinAnd(spellNames(u.in)))
	}

	// the items: an item both options give stays
	both := map[int]int{}
	for _, it := range nw.Reward.Items {
		both[it.PatternID] += it.Count
	}
	rows, err := r.inv.GetCharacterItems(p.CharServerID)
	if err != nil {
		log.Printf("Rehab: %s's items: %v", p.Name, err)
		return nil, rehabTrouble
	}
	var takes []invTake
	takeFromPack := func(patternID, n int) {
		for _, row := range rows {
			if n == 0 {
				return
			}
			if row.Pattern.PatternID == patternID && row.Location == 0 && row.EquipLoc < 0 && row.StackLeft > 0 {
				k := min(n, row.StackLeft)
				takes = append(takes, invTake{itemID: row.ItemID, n: k})
				n -= k
			}
		}
	}
	for _, it := range old.Reward.Items {
		n := it.Count
		if shared := min(n, both[it.PatternID]); shared > 0 {
			n -= shared
			both[it.PatternID] -= shared
		}
		if n == 0 {
			continue
		}
		have, worn, name := 0, 0, fmt.Sprintf("item %d", it.PatternID)
		scroll := false
		if pat, ok, _ := r.inv.ItemPatternByID(it.PatternID); ok {
			name, scroll = pat.ItemName, isScroll(&pat)
		}
		for _, row := range rows {
			if row.Pattern.PatternID != it.PatternID {
				continue
			}
			if row.Location == 0 && row.EquipLoc >= 0 {
				worn += row.StackLeft
			} else {
				have += row.StackLeft
			}
		}
		if k := min(have, n); k > 0 {
			u.back = append(u.back, database.ItemCount{PatternID: it.PatternID, Count: k})
			takeFromPack(it.PatternID, k)
		}
		if have < n {
			switch {
			case worn > 0:
				u.worn = append(u.worn, "the "+name)
			case !scroll: // a scroll used is its spell learned, which goes above
				u.missing = append(u.missing, name)
			}
		}
	}
	for _, it := range nw.Reward.Items {
		if n := min(it.Count, both[it.PatternID]); n > 0 {
			u.give = append(u.give, database.ItemCount{PatternID: it.PatternID, Count: n})
			both[it.PatternID] -= n
		}
	}

	// the pack, for the new items once the old ones are taken
	if l := sessionInvList(sess); l != nil && len(u.give) > 0 {
		u.short = packShort(l, takes, u.give, func(id int) (database.ItemPatternRecord, bool) {
			pat, ok, err := r.inv.ItemPatternByID(id)
			return pat, ok && err == nil
		})
	}
	return u, ""
}

// swapLines says what a change does: what leaves the book and enters it, what
// is taken back and given.
func (r *Rehab) swapLines(u *pathSwap) []string {
	opts := u.pick.options()
	lines := []string{fmt.Sprintf("Your level %d path will be %s, not %s.", u.pick.pq.level, opts[u.to].Answer, opts[u.pick.option].Answer)}
	var goes []string
	if len(u.out) > 0 {
		goes = append(goes, joinAnd(spellNames(u.out))+" "+plural(len(u.out), "leaves", "leave")+" your book")
	}
	if len(u.in) > 0 {
		goes = append(goes, joinAnd(spellNames(u.in))+" "+plural(len(u.in), "enters", "enter")+" it")
	}
	if len(u.back) > 0 {
		goes = append(goes, "I take back "+joinAnd(r.itemNames(u.back, "the ")))
	}
	if len(u.give) > 0 {
		goes = append(goes, "I give you "+joinAnd(r.itemNames(u.give, "the ")))
	}
	if len(goes) > 0 {
		lines = append(lines, capitalize(joinAnd(goes))+".")
	}
	if u.granted {
		lines = append(lines, rehabKnowsAll)
	}
	return lines
}

// askSwap asks to change a pick for option to: what changes, or why it
// cannot.
func (r *Rehab) askSwap(sess *session.Session, p *entity.Player, speaker string, pk pathPick, to int) {
	u, no := r.planSwap(sess, p, pk, to)
	switch {
	case no != "":
		r.say(sess, speaker, no)
		return
	case len(u.worn) > 0:
		r.say(sess, speaker, fmt.Sprintf(rehabWorn, joinAnd(u.worn)))
		return
	case u.short != "":
		r.say(sess, speaker, fmt.Sprintf(questPackFull, u.short))
		return
	}
	r.say(sess, speaker, r.swapLines(u)...)
	r.ask(sess, p, &rehabBox{speaker: speaker, stage: rehabStageConfirm, what: rehabPath, swap: u},
		rehabAskChange, []string{rehabChangeYes, rehabNotNow})
}

// doSwap carries out a change asked and agreed, on the spot: planned again
// (asked again when the pick moved), refused before anything moves when an
// old item is worn or the pack has no room, then the old spells out of the
// book (0x00B8) and its items back, the new spells in (0x00B3) and its items
// given, the pick kept with the quest's row, the change counted. The quest
// rows stay as they are: nothing is done again and no experience is given. A
// line says what changed.
func (r *Rehab) doSwap(sess *session.Session, p *entity.Player, b *rehabBox) {
	if b.swap == nil {
		return
	}
	u, no := r.planSwap(sess, p, b.swap.pick, b.swap.to)
	switch {
	case no != "":
		r.say(sess, b.speaker, no)
		return
	case len(u.worn) > 0:
		r.say(sess, b.speaker, fmt.Sprintf(rehabWorn, joinAnd(u.worn)))
		return
	case u.short != "":
		r.say(sess, b.speaker, fmt.Sprintf(questPackFull, u.short))
		return
	case u.pick.option != b.swap.pick.option:
		r.askSwap(sess, p, b.speaker, u.pick, u.to) // the pick moved: asked again
		return
	}
	var gone []string
	if r.cm != nil && len(u.out) > 0 {
		gone = r.cm.takeAbilitiesFrom(sess, p.CharServerID, u.out)
	}
	var back []database.ItemCount
	for _, it := range u.back {
		if n := r.takeItems(sess, p, it.PatternID, it.Count); n > 0 {
			back = append(back, database.ItemCount{PatternID: it.PatternID, Count: n})
		}
	}
	if r.cm != nil && len(u.in) > 0 {
		r.cm.giveAbilitiesTo(sess, p.CharServerID, p.Name, u.in)
	}
	var given []database.ItemCount
	for _, it := range u.give {
		pat, ok, err := r.inv.ItemPatternByID(it.PatternID)
		if err != nil || !ok {
			continue
		}
		n, err := GiveItemNow(sess, r.gw, r.inv, pat, it.Count)
		if err != nil {
			log.Printf("Rehab: giving %s %q: %v", p.Name, pat.ItemName, err)
		}
		if n > 0 {
			given = append(given, database.ItemCount{PatternID: it.PatternID, Count: n})
		}
	}
	r.quests().setPick(p.CharServerID, u.pick.pq.quest.ID, u.pick.step, u.to)
	r.count(p, database.ResetPath)

	opts := u.pick.options()
	var what []string
	for _, part := range []struct {
		head  string
		names []string
	}{{"Out of your book", gone}, {"Into it", spellNames(u.in)}, {"Taken back", r.itemNames(back, "")},
		{"Given", r.itemNames(given, "")}, {"No longer yours to take back", u.missing}} {
		if len(part.names) > 0 {
			what = append(what, part.head+": "+strings.Join(part.names, ", ")+".")
		}
	}
	r.say(sess, b.speaker, rehabDone+fmt.Sprintf(" Your level %d path is %s now.", u.pick.pq.level, opts[u.to].Answer))
	if len(what) > 0 {
		r.say(sess, "", strings.Join(what, " "))
	}
	log.Printf("Rehab: %s changed %q step %d from %q to %q (out %v, in %v, back %v, given %v, missing %v, inferred %v), free",
		p.Name, u.pick.pq.quest.Name, u.pick.step, opts[u.pick.option].Answer, opts[u.to].Answer, gone, spellNames(u.in),
		back, given, u.missing, u.pick.inferred)
}

// --- the counts and the items ---

// count counts a reset the character had: every one is free (counted as
// used), and the paid column keeps what was paid before they were.
func (r *Rehab) count(p *entity.Player, kind string) {
	if err := r.rows.CountReset(p.CharServerID, kind, true, r.now()); err != nil {
		log.Printf("Rehab: %s's %s reset not counted: %v", p.Name, kind, err)
	}
}

// takeItems takes n of an item from where the character holds it, a record
// at a time: unworn in its pack first (0x003C), then its bank (0x1250, under
// bankMu), kept. It returns how many it took.
func (r *Rehab) takeItems(sess *session.Session, p *entity.Player, patternID, n int) int {
	rows, err := r.inv.GetCharacterItems(p.CharServerID)
	if err != nil {
		log.Printf("Rehab: %s's items: %v", p.Name, err)
		return 0
	}
	took := 0
	take := func(row database.CharInventoryEntry, l *invList, bank bool) {
		k := min(n-took, row.StackLeft)
		var err error
		if k >= row.StackLeft {
			err = r.inv.DeleteCharItem(p.CharServerID, row.ItemID)
		} else {
			err = r.inv.SetItemStack(p.CharServerID, row.ItemID, row.StackLeft-k)
		}
		if err != nil {
			log.Printf("Rehab: taking %q from %s: %v", row.Pattern.ItemName, p.Name, err)
			return
		}
		took += k
		if l == nil {
			return
		}
		if i := l.indexOfItem(row.ItemID); i >= 0 && l.took(i, k) {
			if bank {
				queueReliable(sess, protocol.OpcodeRemoveBankItem, bankTookPayload(i, k))
			} else {
				queueReliable(sess, protocol.OpcodeRemoveInvItem, removeInvItemPayload(i, 0, k))
			}
		}
	}
	for _, row := range rows {
		if took < n && row.Pattern.PatternID == patternID && row.Location == 0 && row.EquipLoc < 0 && row.StackLeft > 0 {
			take(row, sessionInvList(sess), false)
		}
	}
	if took < n {
		bankMu.Lock()
		for _, row := range rows {
			if took < n && row.Pattern.PatternID == patternID && row.Location == 1 && row.StackLeft > 0 {
				take(row, sessionBankList(sess), true)
			}
		}
		bankMu.Unlock()
	}
	return took
}

// bookAndOwned is the character's book (spell names by id) and every item it
// owns, pack, worn and bank, by patternid.
func (r *Rehab) bookAndOwned(p *entity.Player) (map[int]string, map[int]int) {
	book, owned := map[int]string{}, map[int]int{}
	if r.spells != nil {
		if b, err := r.spells.GetCharacterSpells(p.CharServerID); err == nil {
			for _, e := range b {
				book[e.Pattern.SpellID] = e.Pattern.SpellName
			}
		}
	}
	if rows, err := r.inv.GetCharacterItems(p.CharServerID); err == nil {
		for _, row := range rows {
			owned[row.Pattern.PatternID] += row.StackLeft
		}
	}
	return book, owned
}

// soldScrolls is the spells merchants sell as scrolls, by spellKey: one a
// path taught stays in the book, as it may have been bought.
func (r *Rehab) soldScrolls() map[string]bool {
	out := map[string]bool{}
	names, err := r.rows.MerchantScrollNames(scrollIcon)
	if err != nil {
		log.Printf("Rehab: the merchants' scrolls: %v", err)
		return out
	}
	for _, n := range names {
		out[spellKey(n)] = true
	}
	return out
}

// itemNames is the items' names, each with its count past one and the
// article before it.
func (r *Rehab) itemNames(items []database.ItemCount, article string) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		name := fmt.Sprintf("item %d", it.PatternID)
		if pat, ok, _ := r.inv.ItemPatternByID(it.PatternID); ok {
			name = pat.ItemName
		}
		if it.Count > 1 {
			out = append(out, countOf(it.Count, name))
		} else {
			out = append(out, article+name)
		}
	}
	return out
}

// --- the boxes ---

// say says each line in a box of its own (0x0046), the speaker's name
// before it when it has one, cut where CLIENT's box ends.
func (r *Rehab) say(sess *session.Session, speaker string, lines ...string) {
	if sess == nil {
		return
	}
	for _, l := range lines {
		if speaker != "" {
			l = speaker + ": " + l
		}
		for _, part := range splitSay(l, questBoxChars) {
			SendDialogueBox(sess, 0, part)
		}
	}
}

// ask opens a question box (0x0034, under rehabBoxMark; no name, as the
// quests' questions carry none) with its answers, and keeps b as the
// character's question.
func (r *Rehab) ask(sess *session.Session, p *entity.Player, b *rehabBox, question string, labels []string) {
	r.mu.Lock()
	r.seq++
	b.id = rehabBoxMark | (r.seq & 0xFFFFFF)
	r.boxes[p.CharServerID] = b
	r.mu.Unlock()
	for i := range labels {
		labels[i] = clipRunes(labels[i], questAnswerChars)
	}
	if sess != nil {
		SendQuestPopupMulti(sess, b.id, clipRunes(question, questAskChars), labels)
	}
}

// --- words ---

// tunarText is an amount as "1,000,000 tunar".
func tunarText(n int) string {
	s := strconv.Itoa(max(n, 0))
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String() + " tunar"
}

// plural is one for 1, else many.
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// spellNames is the abilities' names, sorted.
func spellNames(v []entity.CMAbility) []string {
	out := make([]string, len(v))
	for i, a := range v {
		out[i] = a.Name
	}
	sort.Strings(out)
	return out
}

// capitalize is s with its first letter upper case.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
