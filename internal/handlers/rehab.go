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
// question with its answers; rehabBoxMark marks their ids) that says what the
// character has left and offers, three answers as CLIENT's VIWndResponse
// shows a page (its own Close answers 255, nothing):
//
//   - a reset, then which: its Combat Masteries, as #rst_mastr
//     (classmastery.go: every rank taken, its points back, 0x1402 sub-op 6 a
//     rank then 3, word 1885); its stat (training) points, as #rst_both's
//     training half (gmresets.go: 0x001D, the stats channel, word 1886); or
//     both;
//   - a path undone (rehabpaths.go: a quest's pick of abilities, its spells
//     out of the book and its items taken back), which of the paths it took;
//   - a Rose of Rehabilitation bought.
//
// Each is asked again before it is done, and refused when the character has
// nothing of it, cannot pay, or (a path) wears what the path gave. A
// character's first resets are free (database.RehabSettings: 1 mastery
// reset, 1 stat point reset, 2 path respecs); then a reset costs tunar
// ("1mil for cms, 2mil for both") and a path a Rose of Rehabilitation ("Roses
// for paths"), sold here ("for 10 million tunar") and given by a class's
// level 60 epic ("1 free path respec w/ 60 epic. Or rose as reward"). The
// prices and free counts are the GMs' (table rehabsettings), the counts each
// character's (charresets). The words are ours.

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
	rehabGreeting   = "Be welcome, %s. What a soul has learned it may set down, and I help it do so, that it may learn anew."
	rehabAskMenu    = "What would you have me undo?"
	rehabAskResets  = "Which shall I reset?"
	rehabAsYouWish  = "As you wish."
	rehabAskBegin   = "Shall I begin?"
	rehabYes        = "Yes, begin."
	rehabNotNow     = "Not now."
	rehabMasteryIs  = "I will draw every Combat Mastery out of you, and the points you spent on them come back to be spent again."
	rehabTrainingIs = "Every training point you have spent comes back to you, and your stats return to what your birth and calling gave you."
	rehabBothIs     = "Every mastery and every training point comes back to you, to be spent as you choose."
	rehabNoMastery  = "You hold no Combat Mastery for me to draw out."
	rehabNoTraining = "You have spent no training points for me to give back."
	rehabNothing    = "You hold no mastery and have spent no training points: there is nothing for me to undo."
	rehabNoPath     = "You have chosen no path that I can undo."
	rehabAskPath    = "Which path shall I undo?"
	rehabUndoYes    = "Yes, undo it."
	rehabRoseIs     = "A Rose of Rehabilitation lets me undo one more of your paths once your free path respecs are spent. It is yours alone: it cannot be traded."
	rehabRoseYes    = "Yes, buy a rose."
	rehabRoseKeep   = "Keep it close. When you would undo a path, bring it to me."
	rehabNoRoses    = "I have no roses to give just now."
	rehabTrouble    = "Forgive me, my thoughts are elsewhere. Come back in a little while."
	rehabPackFull   = "Your pack is full. Make room for the rose, then come back."
	rehabWorn       = "Take off %s first: what a path gave goes back with it."
	rehabDone       = "It is done."
)

// The menus' answers, and what a confirmation does.
const (
	rehabMastery = iota + 1
	rehabTraining
	rehabBoth
	rehabPath
	rehabRose
	rehabResets // the menu's reset: which is asked
)

// The questions a box asks.
const (
	rehabStageMenu    = iota + 1 // the menu, or which reset
	rehabStagePaths              // which path
	rehabStageConfirm            // yes or no to what ask says
)

// rehabTerms is what a reset costs a character now: the free resets it uses,
// the tunar, the rose. Two asks agree only on equal terms.
type rehabTerms struct {
	mastery, training         bool // the parts done (a path: neither)
	freeMastery, freeTraining bool // a free reset of each part used
	freePath, rose            bool // a path: a free respec, else a rose
	tunar                     int
}

// rehabBox is the question a rehabilitator asked a character last.
type rehabBox struct {
	id      uint32
	speaker string
	stage   int
	options []int       // the menu: what each answer is
	paths   []pathQuest // the paths offered, in order
	what    int         // a confirmation: what it does
	terms   rehabTerms  // and on what terms it was asked
	path    *pathUndo   // a path's undoing, as it was asked
	then    int         // a rose bought for a path: the paths asked next
}

// rehabRows is what a rehabilitator keeps: the settings and each character's
// counts (database.SQLiteDB), and which spells merchants sell as scrolls.
type rehabRows interface {
	RehabSettings() (database.RehabSettings, error)
	CharResets(serverID uint32) (map[string]database.CharReset, error)
	CountReset(serverID uint32, kind string, free bool, at time.Time) error
	MerchantScrollNames(icon int) ([]string, error)
}

// Rehab runs the rehabilitators.
type Rehab struct {
	rows   rehabRows
	gw     *entity.World
	inv    *database.InventoryStore
	spells *database.SpellbookStore
	cm     *ClassMasteryHandler
	chars  resetStore          // the characters' rows: the training reset
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

// Talk is a rehabilitator talked to: what the character has left, and the
// menu. It reports whether the NPC was one.
func (r *Rehab) Talk(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	if r == nil || p == nil || npc == nil || npc.Dead || !IsRehabilitator(npc.Name) {
		return false
	}
	forgetDialogs(p)
	speaker := strings.TrimSpace(npc.Name)
	set, left, ok := r.standing(p)
	if !ok {
		r.say(sess, speaker, rehabTrouble)
		return true
	}
	r.say(sess, speaker, fmt.Sprintf(rehabGreeting, p.Name), r.leftLine(p, set, left))
	r.menu(sess, p, speaker, set, left)
	log.Printf("Rehab: %s talked to %s", p.Name, speaker)
	return true
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
		if i < len(b.paths) {
			r.askPath(sess, p, b.speaker, b.paths[i])
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
	case rehabRose:
		r.askRose(sess, p, speaker)
	}
}

// confirmed is a yes: what was asked, done when its terms still hold (asked
// again on the new terms when not).
func (r *Rehab) confirmed(sess *session.Session, p *entity.Player, b *rehabBox) {
	switch b.what {
	case rehabMastery, rehabTraining, rehabBoth:
		r.doReset(sess, p, b)
	case rehabPath:
		r.doPath(sess, p, b)
	case rehabRose:
		if r.doRose(sess, p, b.speaker, b.terms) && b.then == rehabPath {
			r.askPaths(sess, p, b.speaker)
		}
	}
}

// --- the menu ---

// standing is the settings and the free resets the character has left, by
// kind; false when they cannot be read.
func (r *Rehab) standing(p *entity.Player) (database.RehabSettings, map[string]int, bool) {
	set, err := r.rows.RehabSettings()
	if err != nil {
		log.Printf("Rehab: the settings: %v", err)
		return set, nil, false
	}
	counts, err := r.rows.CharResets(p.CharServerID)
	if err != nil {
		log.Printf("Rehab: %s's resets: %v", p.Name, err)
		return set, nil, false
	}
	left := map[string]int{}
	for _, k := range database.ResetKinds {
		left[k] = counts[k].Left(set.Free(k))
	}
	return set, left, true
}

// leftLine says what the character has left: its free resets and its roses.
func (r *Rehab) leftLine(p *entity.Player, set database.RehabSettings, left map[string]int) string {
	var free []string
	if n := left[database.ResetMastery]; n > 0 {
		free = append(free, countWord(n, "free Combat Mastery reset", "free Combat Mastery resets"))
	}
	if n := left[database.ResetTraining]; n > 0 {
		free = append(free, countWord(n, "free stat point reset", "free stat point resets"))
	}
	if n := left[database.ResetPath]; n > 0 {
		free = append(free, countWord(n, "free path respec", "free path respecs"))
	}
	have := "no free resets"
	if len(free) > 0 {
		have = joinAnd(free)
	}
	roses := "no Rose of Rehabilitation"
	if n := r.rosesHeld(p); n > 0 {
		roses = countWord(n, "Rose of Rehabilitation", "Roses of Rehabilitation")
	}
	return fmt.Sprintf("You have %s left, and hold %s.", have, roses)
}

// menu asks what to undo: a reset, a path (free while one is left, else a
// rose), a rose bought: three answers, a page of CLIENT's.
func (r *Rehab) menu(sess *session.Session, p *entity.Player, speaker string, set database.RehabSettings, left map[string]int) {
	path := "a Rose of Rehabilitation"
	if left[database.ResetPath] > 0 {
		path = "free"
	}
	labels := []string{
		"Reset my masteries or stat points",
		"Undo a path (" + path + ")",
		"Buy a Rose of Rehabilitation (" + tunarText(set.PriceRose) + ")",
	}
	r.ask(sess, p, &rehabBox{speaker: speaker, stage: rehabStageMenu,
		options: []int{rehabResets, rehabPath, rehabRose}}, rehabAskMenu, labels)
}

// resetMenu asks which reset: the masteries, the stat points or both, each
// with its price or "free".
func (r *Rehab) resetMenu(sess *session.Session, p *entity.Player, speaker string) {
	set, left, ok := r.standing(p)
	if !ok {
		r.say(sess, speaker, rehabTrouble)
		return
	}
	labels := []string{
		"My Combat Masteries (" + priceWord(resetTerms(set, left, true, false).tunar) + ")",
		"My stat points (" + priceWord(resetTerms(set, left, false, true).tunar) + ")",
		"Both (" + priceWord(resetTerms(set, left, true, true).tunar) + ")",
	}
	r.ask(sess, p, &rehabBox{speaker: speaker, stage: rehabStageMenu,
		options: []int{rehabMastery, rehabTraining, rehabBoth}}, rehabAskResets, labels)
}

// resetTerms is what a reset of the parts named costs now: each part's free
// reset while it has one left; then its price, or, both parts paid, the price
// of both.
func resetTerms(set database.RehabSettings, left map[string]int, mastery, training bool) rehabTerms {
	t := rehabTerms{mastery: mastery, training: training,
		freeMastery: mastery && left[database.ResetMastery] > 0, freeTraining: training && left[database.ResetTraining] > 0}
	switch {
	case mastery && training && !t.freeMastery && !t.freeTraining:
		t.tunar = set.PriceBoth
	default:
		if mastery && !t.freeMastery {
			t.tunar += set.PriceMastery
		}
		if training && !t.freeTraining {
			t.tunar += set.PriceTraining
		}
	}
	return t
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
// as the character stands: what is done, on what terms, or why nothing.
func (r *Rehab) resetPlan(sess *session.Session, p *entity.Player, what int) (rehabTerms, string) {
	set, left, ok := r.standing(p)
	if !ok {
		return rehabTerms{}, rehabTrouble
	}
	hasM, hasT := r.resetParts(sess, p)
	switch what {
	case rehabMastery:
		if !hasM {
			return rehabTerms{}, rehabNoMastery
		}
		return resetTerms(set, left, true, false), ""
	case rehabTraining:
		if !hasT {
			return rehabTerms{}, rehabNoTraining
		}
		return resetTerms(set, left, false, true), ""
	}
	if !hasM && !hasT {
		return rehabTerms{}, rehabNothing
	}
	return resetTerms(set, left, hasM, hasT), ""
}

// askReset asks to reset the masteries, the stat points or both: what it
// does and what it costs, or why it cannot be done.
func (r *Rehab) askReset(sess *session.Session, p *entity.Player, speaker string, what int) {
	t, no := r.resetPlan(sess, p, what)
	if no != "" {
		r.say(sess, speaker, no)
		return
	}
	if t.tunar > p.Tunar {
		r.say(sess, speaker, fmt.Sprintf("That costs %s, and you carry %s.", tunarText(t.tunar), tunarText(p.Tunar)))
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
		rehabAskBegin+" "+termsLine(t), []string{rehabYes, rehabNotNow})
}

// termsLine says what a reset costs, after the question.
func termsLine(t rehabTerms) string {
	var free []string
	if t.freeMastery {
		free = append(free, "your free mastery reset")
	}
	if t.freeTraining {
		free = append(free, "your free stat point reset")
	}
	switch {
	case t.tunar > 0 && len(free) > 0:
		return fmt.Sprintf("It costs %s and %s.", tunarText(t.tunar), joinAnd(free))
	case t.tunar > 0:
		return fmt.Sprintf("It costs %s.", tunarText(t.tunar))
	}
	return "It uses " + joinAnd(free) + "."
}

// doReset carries out a reset asked and agreed: the tunar taken, the
// masteries and the stat points reset, then each part done counted (free or
// paid) and what a part not done would have cost given back (all of it when
// nothing was there to reset).
func (r *Rehab) doReset(sess *session.Session, p *entity.Player, b *rehabBox) {
	t, no := r.resetPlan(sess, p, b.what)
	if no != "" {
		r.say(sess, b.speaker, no)
		return
	}
	if t != b.terms {
		r.askReset(sess, p, b.speaker, b.what) // the terms moved: asked again
		return
	}
	if !r.pay(sess, p, t.tunar) {
		r.say(sess, b.speaker, fmt.Sprintf("That costs %s, and you carry %s.", tunarText(t.tunar), tunarText(p.Tunar)))
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
	final := rehabTerms{}
	if set, left, ok := r.standing(p); ok && (didM || didT) {
		final = resetTerms(set, left, didM, didT)
	}
	r.refund(sess, p, t.tunar-final.tunar)
	if !didM && !didT {
		r.say(sess, b.speaker, rehabNothing)
		return
	}
	if didM {
		r.count(p, database.ResetMastery, final.freeMastery)
	}
	if didT {
		r.count(p, database.ResetTraining, final.freeTraining)
	}
	if trouble {
		lines = append(lines, "Something went wrong keeping it: tell a GM.")
	}
	r.say(sess, b.speaker, rehabDone+" "+strings.Join(lines, " "))
	if paid := min(t.tunar, final.tunar); paid > 0 {
		r.say(sess, "", fmt.Sprintf("You paid %s.", tunarText(paid)))
	}
	var free []string
	if final.freeMastery {
		free = append(free, "your free mastery reset")
	}
	if final.freeTraining {
		free = append(free, "your free stat point reset")
	}
	if len(free) > 0 {
		r.say(sess, "", "That was "+joinAnd(free)+".")
	}
	log.Printf("Rehab: %s reset (masteries %v, stat points %v), %d tunar, free %v/%v", p.Name, didM, didT,
		min(t.tunar, final.tunar), final.freeMastery, final.freeTraining)
}

// --- paths ---

// pathUndo is a path's undoing: the quests forgotten, the spells that leave
// the book, the items taken back, and what stops it or goes missing.
type pathUndo struct {
	level   int
	chosen  pathQuest // the path named
	quests  []pathQuest
	spells  []entity.CMAbility   // in the book now
	items   []database.ItemCount // held now (not worn)
	names   map[int]string       // the items' names
	scrolls map[int]bool         // the items that are scrolls
	missing []string             // given, held nowhere now (sold, destroyed)
	worn    []string             // given and worn: taken off first
	givers  []string             // who asks the picks again
	terms   rehabTerms
}

// ids is the quests' ids.
func (u *pathUndo) ids() []int {
	out := make([]int, len(u.quests))
	for i, pq := range u.quests {
		out[i] = pq.quest.ID
	}
	return out
}

// pathLabel is a path as the menu offers it: its level and what was picked
// (the kept picks' answers, else what the book and the pack hold of its
// options, else the quest's name).
func (r *Rehab) pathLabel(p *entity.Player, pq pathQuest, book map[int]string, held map[int]int) string {
	what := pickedAnswers(pq.quest, pq.row)
	if len(what) == 0 {
		seen := map[string]bool{}
		for _, o := range pickedOptions(pq.quest, pq.row) {
			for _, sp := range o.Reward.Spells {
				if n, ok := book[sp]; ok && !seen[n] {
					seen[n] = true
					what = append(what, n)
				}
			}
			for _, it := range o.Reward.Items {
				pat, ok, _ := r.inv.ItemPatternByID(it.PatternID)
				if !ok {
					continue
				}
				name := pat.ItemName
				if isScroll(&pat) {
					// a level 20 path's scroll: its spell, once it is used
					name = ""
					for _, n := range book {
						if spellKey(n) == spellKey(pat.ItemName) {
							name = n
						}
					}
					if name == "" && held[it.PatternID] > 0 {
						name = pat.ItemName
					}
				} else if held[it.PatternID] == 0 {
					name = ""
				}
				if name != "" && !seen[name] {
					seen[name] = true
					what = append(what, name)
				}
			}
		}
	}
	if len(what) == 0 {
		what = []string{pq.quest.Name}
	}
	return clipRunes(fmt.Sprintf("Level %d: %s", pq.level, strings.Join(what, "; ")), questAnswerChars)
}

// askPaths asks which path to undo: the paths the character took (CLIENT
// pages them three at a time; its Close is "none"), when it has a free
// respec or a rose; else the rose offered.
func (r *Rehab) askPaths(sess *session.Session, p *entity.Player, speaker string) {
	qe := r.quests()
	paths := qe.pathQuests(p.CharServerID)
	var taken []pathQuest
	for _, pq := range paths {
		if pq.row.Done > 0 || pq.row.Picks != "" {
			taken = append(taken, pq)
		}
	}
	if len(taken) == 0 {
		r.say(sess, speaker, rehabNoPath)
		return
	}
	set, left, ok := r.standing(p)
	if !ok {
		r.say(sess, speaker, rehabTrouble)
		return
	}
	if left[database.ResetPath] == 0 && r.rosesHeld(p) == 0 {
		r.offerRose(sess, p, speaker, set)
		return
	}
	book, held := r.bookAndHeld(p)
	if len(taken) > maxDialogOptions {
		taken = taken[len(taken)-maxDialogOptions:] // the latest
	}
	labels := make([]string, 0, len(taken))
	for _, pq := range taken {
		labels = append(labels, r.pathLabel(p, pq, book, held))
	}
	r.ask(sess, p, &rehabBox{speaker: speaker, stage: rehabStagePaths, paths: taken}, rehabAskPath, labels)
}

// offerRose is a path asked for with no free respec and no rose: a rose
// offered, when the character can pay for one.
func (r *Rehab) offerRose(sess *session.Session, p *entity.Player, speaker string, set database.RehabSettings) {
	if p.Tunar < set.PriceRose {
		r.say(sess, speaker, fmt.Sprintf("Your free path respecs are spent. For each path more I need a Rose of Rehabilitation: "+
			"I sell them for %s, and you carry %s.", tunarText(set.PriceRose), tunarText(p.Tunar)))
		return
	}
	r.say(sess, speaker, fmt.Sprintf("Your free path respecs are spent. For each path more I need a Rose of Rehabilitation, "+
		"and I sell them for %s.", tunarText(set.PriceRose)))
	r.ask(sess, p, &rehabBox{speaker: speaker, stage: rehabStageConfirm, what: rehabRose, terms: rehabTerms{tunar: set.PriceRose},
		then: rehabPath}, "Will you buy one?", []string{rehabRoseYes, rehabNotNow})
}

// planPath is the undoing of the path a quest asked: that quest and, at 30
// and 40, the later levels' of its group; their picks' spells the book holds
// (not one another quest the character keeps gives, nor one a merchant sells
// as a scroll) and items, each where the character holds it; and its terms
// (a free respec, else a rose). Why it cannot be done, when it cannot.
func (r *Rehab) planPath(sess *session.Session, p *entity.Player, chosen pathQuest) (*pathUndo, string) {
	qe := r.quests()
	u := &pathUndo{level: chosen.level, chosen: chosen, names: map[int]string{}, scrolls: map[int]bool{}}
	var found bool
	for _, pq := range qe.pathQuests(p.CharServerID) {
		switch {
		case pq.quest.ID == chosen.quest.ID:
			found = true
		case chosen.level == 20 || pq.level == 20 || pq.group != chosen.group || pq.level < chosen.level:
			continue
		}
		u.quests = append(u.quests, pq)
	}
	if !found {
		return nil, rehabNoPath
	}
	except := map[int]bool{}
	seenGiver := map[string]bool{}
	for _, pq := range u.quests {
		except[pq.quest.ID] = true
		if g := strings.TrimSpace(pq.quest.Giver); g != "" && !seenGiver[g] {
			seenGiver[g] = true
			u.givers = append(u.givers, g)
		}
	}
	keep := qe.spellsKept(p.CharServerID, p.Class, except)
	sold := r.soldScrolls()

	// what the picks gave
	spellIDs := map[int]bool{}
	scrollKeys := map[string]bool{}
	need := map[int]int{}
	sure := map[int]bool{} // given by a kept pick: missed when not held
	var order []int
	var names []database.SpellName
	for _, pq := range u.quests {
		for _, o := range pickedOptions(pq.quest, pq.row) {
			for _, sp := range o.Reward.Spells {
				spellIDs[sp] = true
			}
			for _, it := range o.Reward.Items {
				pat, ok, err := r.inv.ItemPatternByID(it.PatternID)
				if err != nil || !ok {
					continue
				}
				if isScroll(&pat) {
					// a scroll taught its spell when it was used: the spell
					// goes, and the scroll if it is still held
					u.scrolls[it.PatternID] = true
					if names == nil && r.spells != nil {
						names, _ = r.spells.SpellNames()
					}
					if sp, ok := scrollSpell(names, pat.ItemName, pat.LevelReq); ok {
						spellIDs[sp.SpellID] = true
						scrollKeys[spellKey(sp.Name)] = true
					}
				}
				if need[it.PatternID] == 0 {
					order = append(order, it.PatternID)
				}
				need[it.PatternID] += it.Count
				sure[it.PatternID] = sure[it.PatternID] || o.kept
				u.names[it.PatternID] = pat.ItemName
			}
		}
	}

	// the spells the book holds of them
	if r.spells != nil {
		if book, err := r.spells.GetCharacterSpells(p.CharServerID); err == nil {
			for _, e := range book {
				id, key := e.Pattern.SpellID, spellKey(e.Pattern.SpellName)
				if !spellIDs[id] && !scrollKeys[key] {
					continue
				}
				if keep[id] || sold[key] {
					continue
				}
				u.spells = append(u.spells, entity.CMAbility{Name: e.Pattern.SpellName, SpellID: id})
			}
		}
	}

	// the items, where they are held
	rows, err := r.inv.GetCharacterItems(p.CharServerID)
	if err != nil {
		log.Printf("Rehab: %s's items: %v", p.Name, err)
		return nil, rehabTrouble
	}
	for _, id := range order {
		have, worn := 0, 0
		for _, row := range rows {
			if row.Pattern.PatternID != id {
				continue
			}
			if row.Location == 0 && row.EquipLoc >= 0 {
				worn += row.StackLeft
			} else {
				have += row.StackLeft
			}
		}
		n := min(have, need[id])
		if n > 0 {
			u.items = append(u.items, database.ItemCount{PatternID: id, Count: n})
		}
		if n < need[id] {
			switch {
			case worn > 0 && (sure[id] || have == 0):
				u.worn = append(u.worn, "the "+u.names[id])
			case sure[id] && !u.scrolls[id]:
				// a scroll used is its spell learned, which goes above; an
				// option of a pick not kept may never have been given
				u.missing = append(u.missing, u.names[id])
			}
		}
	}

	// the terms: a free respec, else a rose
	set, left, ok := r.standing(p)
	if !ok {
		return nil, rehabTrouble
	}
	switch {
	case left[database.ResetPath] > 0:
		u.terms = rehabTerms{freePath: true}
	case r.rosesHeld(p) > 0:
		u.terms = rehabTerms{rose: true}
	default:
		return nil, fmt.Sprintf("Your free path respecs are spent. For each path more I need a Rose of Rehabilitation, "+
			"and I sell them for %s.", tunarText(set.PriceRose))
	}
	return u, ""
}

// askPath asks to undo a path: what goes (its quests, spells and items) and
// on what terms, or why it cannot.
func (r *Rehab) askPath(sess *session.Session, p *entity.Player, speaker string, chosen pathQuest) {
	u, no := r.planPath(sess, p, chosen)
	if no != "" {
		r.say(sess, speaker, no)
		return
	}
	if len(u.worn) > 0 {
		r.say(sess, speaker, fmt.Sprintf(rehabWorn, joinAnd(u.worn)))
		return
	}
	var lines []string
	if later := laterLevels(u); len(later) > 0 {
		lines = append(lines, fmt.Sprintf("Your level %d path will be undone, and your level %s %s with it.",
			u.level, joinAnd(later), plural(len(later), "path", "paths")))
	} else {
		lines = append(lines, fmt.Sprintf("Your level %d path will be undone.", u.level))
	}
	var goes []string
	if len(u.spells) > 0 {
		goes = append(goes, joinAnd(spellNames(u.spells))+" "+plural(len(u.spells), "leaves", "leave")+" your book")
	}
	if len(u.items) > 0 {
		goes = append(goes, "I take back "+joinAnd(r.itemNames(u.items, "the ")))
	}
	if len(goes) > 0 {
		lines = append(lines, capitalize(strings.Join(goes, ", and "))+".")
	}
	lines = append(lines, "Then choose again with "+joinAnd(u.givers)+".")
	r.say(sess, speaker, lines...)
	question := "Shall I undo it, for a Rose of Rehabilitation?"
	if u.terms.freePath {
		if _, left, ok := r.standing(p); ok {
			question = fmt.Sprintf("Shall I undo it? This is a free path respec (%d left after).", left[database.ResetPath]-1)
		}
	}
	r.ask(sess, p, &rehabBox{speaker: speaker, stage: rehabStageConfirm, what: rehabPath, terms: u.terms, path: u},
		question, []string{rehabUndoYes, rehabNotNow})
}

// laterLevels is the levels a path's undoing takes besides its own.
func laterLevels(u *pathUndo) []string {
	var out []string
	for _, pq := range u.quests {
		if pq.level != u.level {
			out = append(out, strconv.Itoa(pq.level))
		}
	}
	return out
}

// doPath carries out a path's undoing asked and agreed: the plan made again
// (asked again when its quests or terms moved), the rose taken or the free
// respec counted, the quests forgotten, the spells out of the book and the
// items taken back; a line says what was undone.
func (r *Rehab) doPath(sess *session.Session, p *entity.Player, b *rehabBox) {
	if b.path == nil {
		return
	}
	chosen := b.path.chosen
	u, no := r.planPath(sess, p, chosen)
	if no != "" {
		r.say(sess, b.speaker, no)
		return
	}
	if len(u.worn) > 0 {
		r.say(sess, b.speaker, fmt.Sprintf(rehabWorn, joinAnd(u.worn)))
		return
	}
	if u.terms != b.terms || !sameInts(u.ids(), b.path.ids()) {
		r.askPath(sess, p, b.speaker, chosen)
		return
	}
	if u.terms.rose && r.takeItems(sess, p, database.RoseOfRehabilitation, 1) == 0 {
		r.say(sess, b.speaker, "Bring me a Rose of Rehabilitation first.")
		return
	}
	if err := r.quests().forgetQuests(p.CharServerID, u.ids()); err != nil {
		log.Printf("Rehab: %s's path quests not forgotten: %v", p.Name, err)
		if u.terms.rose {
			r.giveRose(sess, p)
		}
		r.say(sess, b.speaker, "Something went wrong: tell a GM.")
		return
	}
	var gone []string
	if r.cm != nil && len(u.spells) > 0 {
		gone = r.cm.takeAbilitiesFrom(sess, p.CharServerID, u.spells)
	}
	var back []database.ItemCount
	for _, it := range u.items {
		if n := r.takeItems(sess, p, it.PatternID, it.Count); n > 0 {
			back = append(back, database.ItemCount{PatternID: it.PatternID, Count: n})
		}
	}
	r.count(p, database.ResetPath, u.terms.freePath)

	what := fmt.Sprintf("Undone: your level %d path", u.level)
	if later := laterLevels(u); len(later) > 0 {
		what += " and your level " + joinAnd(later) + " " + plural(len(later), "path", "paths")
	}
	if len(gone) > 0 {
		what += "; out of your book: " + strings.Join(gone, ", ")
	}
	if len(back) > 0 {
		what += "; taken back: " + strings.Join(r.itemNames(back, ""), ", ")
	}
	if len(u.missing) > 0 {
		what += "; no longer yours to take back: " + strings.Join(u.missing, ", ")
	}
	r.say(sess, b.speaker, rehabDone+" Speak with "+joinAnd(u.givers)+" to choose again.")
	r.say(sess, "", what+".")
	if u.terms.rose {
		r.say(sess, "", "You gave up a Rose of Rehabilitation.")
	} else if _, left, ok := r.standing(p); ok {
		r.say(sess, "", fmt.Sprintf("That was a free path respec: %d left.", left[database.ResetPath]))
	}
	log.Printf("Rehab: %s undid the level %d path (quests %v; spells %v; items %v; missing %v; rose %v)",
		p.Name, u.level, u.ids(), gone, back, u.missing, u.terms.rose)
}

// --- the rose ---

// askRose asks to buy a Rose of Rehabilitation, or says why not.
func (r *Rehab) askRose(sess *session.Session, p *entity.Player, speaker string) {
	set, _, ok := r.standing(p)
	if !ok {
		r.say(sess, speaker, rehabTrouble)
		return
	}
	if no := r.roseRefusal(sess, p, set.PriceRose); no != "" {
		r.say(sess, speaker, no)
		return
	}
	r.say(sess, speaker, rehabRoseIs)
	r.ask(sess, p, &rehabBox{speaker: speaker, stage: rehabStageConfirm, what: rehabRose, terms: rehabTerms{tunar: set.PriceRose}},
		fmt.Sprintf("Will you buy one for %s?", tunarText(set.PriceRose)), []string{rehabRoseYes, rehabNotNow})
}

// roseRefusal is why the character cannot buy a rose for price now: too
// little tunar, a full pack, no rose to give; "" when it can.
func (r *Rehab) roseRefusal(sess *session.Session, p *entity.Player, price int) string {
	pat, ok, err := r.inv.ItemPatternByID(database.RoseOfRehabilitation)
	if err != nil || !ok {
		return rehabNoRoses
	}
	if p.Tunar < price {
		return fmt.Sprintf("A Rose of Rehabilitation costs %s, and you carry %s.", tunarText(price), tunarText(p.Tunar))
	}
	if l := sessionInvList(sess); l == nil || l.wouldAdd(pat.PatternID, 1, pat.MaxStack) == 0 {
		return rehabPackFull
	}
	return ""
}

// doRose sells a rose asked for and agreed: at the price asked (asked again
// when it moved), the tunar taken, the rose given, the tunar given back when
// it could not be. It reports a sale.
func (r *Rehab) doRose(sess *session.Session, p *entity.Player, speaker string, asked rehabTerms) bool {
	set, _, ok := r.standing(p)
	if !ok {
		r.say(sess, speaker, rehabTrouble)
		return false
	}
	if set.PriceRose != asked.tunar {
		r.askRose(sess, p, speaker)
		return false
	}
	if no := r.roseRefusal(sess, p, set.PriceRose); no != "" {
		r.say(sess, speaker, no)
		return false
	}
	if !r.pay(sess, p, set.PriceRose) {
		r.say(sess, speaker, fmt.Sprintf("A Rose of Rehabilitation costs %s, and you carry %s.", tunarText(set.PriceRose), tunarText(p.Tunar)))
		return false
	}
	if !r.giveRose(sess, p) {
		r.refund(sess, p, set.PriceRose)
		r.say(sess, speaker, rehabPackFull)
		return false
	}
	r.say(sess, "", fmt.Sprintf("You paid %s.", tunarText(set.PriceRose)), "You have received "+database.RoseOfRehabilitationName+".")
	r.say(sess, speaker, rehabRoseKeep)
	log.Printf("Rehab: %s bought a Rose of Rehabilitation for %d", p.Name, set.PriceRose)
	return true
}

// giveRose puts a rose in the character's pack, as a quest gives an item;
// false when it could not.
func (r *Rehab) giveRose(sess *session.Session, p *entity.Player) bool {
	pat, ok, err := r.inv.ItemPatternByID(database.RoseOfRehabilitation)
	if err != nil || !ok {
		return false
	}
	n, err := GiveItemNow(sess, r.gw, r.inv, pat, 1)
	if err != nil {
		log.Printf("Rehab: a rose for %s: %v", p.Name, err)
	}
	return n == 1
}

// rosesHeld is how many roses the character holds, its pack's and its
// bank's.
func (r *Rehab) rosesHeld(p *entity.Player) int {
	rows, err := r.inv.GetCharacterItems(p.CharServerID)
	if err != nil {
		return 0
	}
	n := 0
	for _, row := range rows {
		if row.Pattern.PatternID == database.RoseOfRehabilitation && !(row.Location == 0 && row.EquipLoc >= 0) {
			n += row.StackLeft
		}
	}
	return n
}

// --- the purse and the items ---

// pay takes n tunar from the purse (nothing for 0), kept and shown; false
// when it holds less.
func (r *Rehab) pay(sess *session.Session, p *entity.Player, n int) bool {
	if n <= 0 {
		return true
	}
	left, ok := r.gw.TakeTunar(p.CharServerID, n)
	if !ok {
		return false
	}
	if err := r.inv.SetPurse(p.CharServerID, left); err != nil {
		log.Printf("Rehab: %s's purse not kept: %v", p.Name, err)
	}
	queueReliable(sess, protocol.OpcodePlayerTunar, tunarPayload(left))
	return true
}

// refund gives back n tunar pay took.
func (r *Rehab) refund(sess *session.Session, p *entity.Player, n int) {
	if n > 0 {
		r.pay(sess, p, -n)
	}
}

// count counts a reset the character had, free or paid.
func (r *Rehab) count(p *entity.Player, kind string, free bool) {
	if err := r.rows.CountReset(p.CharServerID, kind, free, r.now()); err != nil {
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

// bookAndHeld is the character's book (spell names by id) and the items it
// holds unworn, by patternid.
func (r *Rehab) bookAndHeld(p *entity.Player) (map[int]string, map[int]int) {
	book, held := map[int]string{}, map[int]int{}
	if r.spells != nil {
		if b, err := r.spells.GetCharacterSpells(p.CharServerID); err == nil {
			for _, e := range b {
				book[e.Pattern.SpellID] = e.Pattern.SpellName
			}
		}
	}
	if rows, err := r.inv.GetCharacterItems(p.CharServerID); err == nil {
		for _, row := range rows {
			if !(row.Location == 0 && row.EquipLoc >= 0) {
				held[row.Pattern.PatternID] += row.StackLeft
			}
		}
	}
	return book, held
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

// priceWord is a price in a menu's answer: "free", or its tunar.
func priceWord(n int) string {
	if n <= 0 {
		return "free"
	}
	return tunarText(n)
}

// countWord is "1 thing" or "2 things".
func countWord(n int, one, many string) string {
	return strconv.Itoa(n) + " " + plural(n, one, many)
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

// sameInts is whether two lists hold the same numbers in the same order.
func sameInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
