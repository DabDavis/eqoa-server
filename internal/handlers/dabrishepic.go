package handlers

import (
	"errors"
	"fmt"
	"log"
	"regexp"
	"slices"
	"sort"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// dabrishepic.go — Dabrish exchanges a path epic for the other path's (the
// owner, 2026-11-13: "allow dabrish to exchange epic weapons for the other
// path for players"). A class's path epics are the two items its level 49
// path reward quest offers at its weapon pick (30,40,49/<group>/L49-<class>,
// archivistpaths.go's reRewardKey: the warrior's Steelwind and Foereaver, the
// alchemist's Metamorphic Robes and Odan's Bonded Gloves), read off the
// running quests as their bases (an option's Ascended record counts for its
// base, database/ascended.go). The level 60 epics' rewards are no path's:
// their picks are of up to six weapons, robes and tunics, none of them paired
// with a path, so one of them is refused as having no counterpart.
//
// "Exchange my epic weapon", his first box's sixth answer, reads the
// character's rows:
//
//   - a path epic of its class in the pack, unworn, is offered: which he gives
//     for it is the other path's in the same version, the regular record for a
//     regular one, the Ascended record for an Ascended one (refused when the
//     other has none), an imported copy for an imported one (the transfer's
//     damper, database/imported.go: the other's copy, made when it has none).
//     With several, he asks which first;
//   - refused, each with its own line: one worn (taken off first, as a
//     rehabilitator's path change refuses, rehab.go), one banked, the other
//     path's epic owned already in any version and anywhere (no duplicates), a
//     level 60 epic (no counterpart), an Heirloom of a path epic (the Heirloom
//     Outfitter sells either for nothing), and none at all.
//
// He says what is held and what he gives, then asks Yes or No. On Yes it is
// planned again from the rows (refused if anything moved) and done in one
// transaction (InventoryStore.ExchangeCharItem): the row deleted, the new one
// at the end of the pack with the old one's share of its durability and its
// charges; the client told 0x003C for the old record and 0x003B for the new,
// the session's copy of its list kept in step. A level 49 reward quest's kept
// weapon pick is moved with it, so a rehabilitator's path change later takes
// back what the character holds. Free: the rehabilitators' resets are free
// since 2026-11-06 (rehab.go; before, 2 free path respecs then a Rose of
// Rehabilitation, sold for 10,000,000, each), and this is no more than half of
// one. Logged; no gm_audit line, as no quest reward, sale or rehabilitator's
// change has one. The words are ours.

// The exchange's answer in his first box, and its boxes' marks in dabrishAsk.rng.
const (
	dabrishEpicLabel   = "Exchange my epic weapon"
	dabrishEpicWhich   = 100 // which of several epics
	dabrishEpicConfirm = 101 // Yes or No
)

// His words.
const (
	dabrishEpicNoPaths  = "I know of no path epics for your class."
	dabrishEpicNone     = "You hold no level 49 path epic of your class for me to exchange."
	dabrishEpicNoPair   = "The %s belongs to no path: I have nothing of another path to give for it."
	dabrishEpicHeirloom = "Your %s is an heirloom: the Heirloom Outfitter gives either path's for nothing."
	dabrishEpicWorn     = "Take the %s off first, then ask me again."
	dabrishEpicBanked   = "Bring the %s from your bank first, then ask me again."
	dabrishEpicOwned    = "You own the %s already. I will not give you a second."
	dabrishEpicNoMatch  = "There is no %s version of the %s for me to give for your %s."
	dabrishEpicShow     = "You hold the %s. For it I give you the %s, the other path's epic, free of charge."
	dabrishEpicAsk      = "Dabrish: 'Exchange the %s for the %s?'"
	dabrishEpicAskWhich = "Dabrish: 'Which epic shall I exchange?'"
	dabrishEpicYes      = "Yes, exchange it."
	dabrishEpicNo       = "No."
	dabrishEpicNotNow   = "As you like. Your %s stays yours."
	dabrishEpicDone     = "There: the %s is yours, and the %s mine."
	dabrishEpicMoved    = "You no longer hold the %s as you did. Ask me again."
)

// reEpic60Key is a level 60 epic reward quest's import key: its class's slug.
var reEpic60Key = regexp.MustCompile(`^60/[^/]+/reward-([a-z]+)$`)

// epicSwap is one exchange planned: the pack's row, what it is and what is
// given for it.
type epicSwap struct {
	itemID     int
	from       int    // the row's record
	fromName   string // its name
	fromBase   int    // the path epic it is a version of
	toBase     int    // the other path's
	to         int    // the record given; 0 with importedOf
	importedOf int    // toBase, whose imported copy is given (made when none)
	toName     string
}

// dabrishEpics is what the exchange reads and writes. Nil: no exchange.
type dabrishEpics struct {
	inv    *database.InventoryStore
	quests func() *QuestEngine
	now    func() time.Time
}

// newDabrishEpics is the exchange over the inventory and the running quests.
func newDabrishEpics(inv *database.InventoryStore) *dabrishEpics {
	if inv == nil {
		return nil
	}
	return &dabrishEpics{inv: inv, quests: Quests, now: time.Now}
}

// classEpicItems is what the class's enabled epic quests offer: the level 49
// path reward quests' options' items (pair), and the level 60 epic rewards'
// options' and steps' items (sixty), each record once, in order.
func (q *QuestEngine) classEpicItems(class int) (pair, sixty []int) {
	if q == nil || class < 0 || class >= len(database.TransferClassSlugs) {
		return nil, nil
	}
	slug := database.TransferClassSlugs[class]
	q.mu.Lock()
	defer q.mu.Unlock()
	qs := slices.Clone(q.cat.all)
	sort.Slice(qs, func(i, j int) bool { return qs[i].ID < qs[j].ID })
	add := func(to []int, items []database.ItemCount) []int {
		for _, it := range items {
			if !slices.Contains(to, it.PatternID) {
				to = append(to, it.PatternID)
			}
		}
		return to
	}
	for _, qu := range qs {
		if !qu.Enabled {
			continue
		}
		key := importKey(qu.Source)
		if m := reRewardKey.FindStringSubmatch(key); m != nil && m[1] == "49" && m[2] == slug {
			for i := range qu.Steps {
				for _, o := range qu.Steps[i].Choices.Options {
					if !o.Stay {
						pair = add(pair, o.Reward.Items)
					}
				}
			}
		} else if m := reEpic60Key.FindStringSubmatch(key); m != nil && m[1] == slug {
			for i := range qu.Steps {
				sixty = add(sixty, qu.Steps[i].RewardFor(class).Items)
				for _, o := range qu.Steps[i].Choices.Options {
					sixty = add(sixty, o.Reward.Items)
				}
			}
		}
	}
	return pair, sixty
}

// epicVersions is the records' versions: an Ascended record's and an
// imported copy's base.
type epicVersions struct {
	asc, imp map[int]int
}

// base is a record's base and its version: "Ascended", "imported" or "".
func (v epicVersions) base(id int) (int, string) {
	if b, ok := v.asc[id]; ok {
		return b, "Ascended"
	}
	if b, ok := v.imp[id]; ok {
		return b, "imported"
	}
	return id, ""
}

// of is every record that is base in some version: itself, its Ascended
// records, its imported copies.
func (v epicVersions) of(base int) []int {
	out := []int{base}
	for _, m := range []map[int]int{v.asc, v.imp} {
		for c, b := range m {
			if b == base {
				out = append(out, c)
			}
		}
	}
	sort.Ints(out[1:])
	return out
}

// copyOf is base's record in the version, and whether it has one.
func (v epicVersions) copyOf(base int, version string) (int, bool) {
	var m map[int]int
	switch version {
	case "":
		return base, true
	case "Ascended":
		m = v.asc
	default:
		m = v.imp
	}
	best := 0
	for c, b := range m {
		if b == base && (best == 0 || c < best) {
			best = c
		}
	}
	return best, best != 0
}

// name is a record's name, or its number.
func (e *dabrishEpics) name(id int) string {
	if pat, ok, err := e.inv.ItemPatternByID(id); err == nil && ok {
		return pat.ItemName
	}
	return fmt.Sprintf("item %d", id)
}

// plan is what the character may exchange: each path epic of its class in its
// pack, unworn, with what it would get; or, with none, why not.
func (e *dabrishEpics) plan(p *entity.Player) ([]epicSwap, string, error) {
	pairIDs, sixtyIDs := e.quests().classEpicItems(p.Class)
	asc, imp, err := e.inv.ItemVersions()
	if err != nil {
		return nil, "", err
	}
	v := epicVersions{asc: asc, imp: imp}
	var pair []int
	for _, id := range pairIDs {
		if b, _ := v.base(id); !slices.Contains(pair, b) {
			pair = append(pair, b)
		}
	}
	if len(pair) != 2 {
		return nil, dabrishEpicNoPaths, nil
	}
	sixty := map[int]bool{}
	for _, id := range sixtyIDs {
		b, _ := v.base(id)
		sixty[b] = !slices.Contains(pair, b)
	}
	heirlooms, err := e.inv.Heirlooms()
	if err != nil {
		return nil, "", err
	}
	rows, err := e.inv.GetCharacterItems(p.CharServerID)
	if err != nil {
		return nil, "", err
	}
	var swaps []epicSwap
	var refusals, worn, banked, noPair, heirloom []string
	for _, row := range rows {
		gear := row.Pattern.EquipSlot >= 1 && row.Pattern.MaxStack < 2
		b, version := v.base(row.Pattern.PatternID)
		i := slices.Index(pair, b)
		switch {
		case i < 0 && gear && sixty[b]:
			noPair = append(noPair, row.Pattern.ItemName)
			continue
		case i < 0:
			if def, ok := heirlooms[row.Pattern.PatternID]; ok && def.Epic != 0 {
				if eb, _ := v.base(def.Epic); slices.Contains(pair, eb) {
					heirloom = append(heirloom, row.Pattern.ItemName)
				}
			}
			continue
		case !gear:
			continue
		case row.Location == database.LocBank:
			banked = append(banked, row.Pattern.ItemName)
			continue
		case row.EquipLoc >= 0:
			worn = append(worn, row.Pattern.ItemName)
			continue
		}
		other := pair[1-i]
		otherName := e.name(other)
		if owned, err := e.inv.OwnsAnyOf(p.CharServerID, v.of(other)); err != nil {
			return nil, "", err
		} else if owned {
			refusals = append(refusals, fmt.Sprintf(dabrishEpicOwned, otherName))
			continue
		}
		s := epicSwap{itemID: row.ItemID, from: row.Pattern.PatternID, fromName: row.Pattern.ItemName, fromBase: b, toBase: other}
		to, ok := v.copyOf(other, version)
		switch {
		case ok:
			s.to, s.toName = to, e.name(to)
		case version == "imported":
			s.importedOf, s.toName = other, otherName // the damper's copy, made at the exchange
		default:
			refusals = append(refusals, fmt.Sprintf(dabrishEpicNoMatch, version, otherName, row.Pattern.ItemName))
			continue
		}
		swaps = append(swaps, s)
	}
	if len(swaps) > 0 {
		return swaps, "", nil
	}
	switch {
	case len(refusals) > 0:
		return nil, refusals[0], nil
	case len(worn) > 0:
		return nil, fmt.Sprintf(dabrishEpicWorn, worn[0]), nil
	case len(banked) > 0:
		return nil, fmt.Sprintf(dabrishEpicBanked, banked[0]), nil
	case len(noPair) > 0:
		return nil, fmt.Sprintf(dabrishEpicNoPair, noPair[0]), nil
	case len(heirloom) > 0:
		return nil, fmt.Sprintf(dabrishEpicHeirloom, heirloom[0]), nil
	}
	return nil, dabrishEpicNone, nil
}

// dabrishEpicStart is his first box's exchange: what may be exchanged, asked
// which when several, else shown and asked Yes or No.
func (h *InteractHandler) dabrishEpicStart(sess *session.Session, p *entity.Player, npc entity.EntityID) {
	swaps, no, err := h.dabrishEpic.plan(p)
	switch {
	case err != nil:
		log.Printf("Dabrish: %s's epics: %v", p.Name, err)
		dabrishSay(sess, dabrishFailed)
		return
	case no != "":
		dabrishSay(sess, no)
		return
	case len(swaps) == 1:
		h.dabrishEpicAsk(sess, p, npc, swaps[0])
		return
	}
	swaps = swaps[:min(len(swaps), maxDialogOptions)]
	answers := make([]string, len(swaps))
	for i, s := range swaps {
		answers[i] = clipRunes(s.fromName, questAnswerChars)
	}
	h.boardMu.Lock()
	id := h.dabrishAskLocked(p, dabrishEpicWhich, npc)
	a := h.dabrish.asked[p.CharServerID]
	a.swaps = swaps
	h.dabrish.asked[p.CharServerID] = a
	h.boardMu.Unlock()
	SendQuestPopupMulti(sess, id, dabrishEpicAskWhich, answers)
}

// dabrishEpicAsk shows an exchange and asks Yes or No.
func (h *InteractHandler) dabrishEpicAsk(sess *session.Session, p *entity.Player, npc entity.EntityID, s epicSwap) {
	dabrishSay(sess, fmt.Sprintf(dabrishEpicShow, s.fromName, s.toName))
	h.boardMu.Lock()
	id := h.dabrishAskLocked(p, dabrishEpicConfirm, npc)
	a := h.dabrish.asked[p.CharServerID]
	a.swaps = []epicSwap{s}
	h.dabrish.asked[p.CharServerID] = a
	h.boardMu.Unlock()
	SendQuestPopupMulti(sess, id, clipRunes(fmt.Sprintf(dabrishEpicAsk, s.fromName, s.toName), questAskChars),
		[]string{dabrishEpicYes, dabrishEpicNo})
}

// answerDabrishEpic is an answer to one of the exchange's boxes.
func (h *InteractHandler) answerDabrishEpic(sess *session.Session, p *entity.Player, asked dabrishAsk, n int) {
	if n >= len(asked.swaps) && asked.rng == dabrishEpicWhich {
		return
	}
	switch asked.rng {
	case dabrishEpicWhich:
		h.dabrishEpicAsk(sess, p, asked.npc, asked.swaps[n])
	case dabrishEpicConfirm:
		if len(asked.swaps) != 1 {
			return
		}
		if n != 0 {
			dabrishSay(sess, fmt.Sprintf(dabrishEpicNotNow, asked.swaps[0].fromName))
			return
		}
		h.dabrishEpicDo(sess, p, asked.swaps[0])
	}
}

// dabrishEpicDo carries out an exchange agreed to: planned again (refused
// when the row or what it gets moved), done in one transaction, the client
// told, the level 49 quest's kept pick moved with it.
func (h *InteractHandler) dabrishEpicDo(sess *session.Session, p *entity.Player, want epicSwap) {
	e := h.dabrishEpic
	swaps, no, err := e.plan(p)
	if err != nil {
		log.Printf("Dabrish: %s's epics: %v", p.Name, err)
		dabrishSay(sess, dabrishFailed)
		return
	}
	i := slices.IndexFunc(swaps, func(s epicSwap) bool {
		return s.itemID == want.itemID && s.from == want.from && s.toBase == want.toBase && s.to == want.to
	})
	if i < 0 {
		if no == "" {
			no = fmt.Sprintf(dabrishEpicMoved, want.fromName)
		}
		dabrishSay(sess, no)
		return
	}
	s := swaps[i]
	done, err := e.inv.ExchangeCharItem(p.CharServerID, database.ItemExchange{ItemID: s.itemID, From: s.from, To: s.to,
		ImportedOf: s.importedOf, Source: "Dabrish's epic exchange"}, e.now())
	if errors.Is(err, database.ErrExchangeGone) {
		dabrishSay(sess, fmt.Sprintf(dabrishEpicMoved, s.fromName))
		return
	}
	if err != nil {
		log.Printf("Dabrish: exchanging %s's %q (row %d): %v", p.Name, s.fromName, s.itemID, err)
		dabrishSay(sess, dabrishFailed)
		return
	}
	pat, ok, err := e.inv.ItemPatternByID(done.To)
	if err != nil || !ok {
		log.Printf("Dabrish: %s's new record %d: %v", p.Name, done.To, err)
	}
	if l := sessionInvList(sess); l != nil {
		if at := l.indexOfItem(s.itemID); at >= 0 && l.took(at, 1) {
			queueReliable(sess, protocol.OpcodeRemoveInvItem, removeInvItemPayload(at, 0, 1))
		}
		if ok {
			rec := wireRecord(database.CharInventoryEntry{ItemID: done.ItemID, StackLeft: 1, RemainHP: done.RemainHP,
				Charges: done.Charges, Pattern: pat})
			if key, took := l.add(done.ItemID, done.To, 1, pat.MaxStack); took > 0 {
				queueReliable(sess, protocol.OpcodeAddInvItem, addInvItemPayload(&rec, key, readsCapes(sess)))
			}
		}
	}
	moved := e.quests().moveEpicPick(p.CharServerID, s.fromBase, s.toBase, e.versionsBase())
	log.Printf("Dabrish: %s exchanged %q (record %d, row %d) for %q (record %d, row %d), condition %d, charges %d, picks moved %d, free",
		p.Name, s.fromName, s.from, s.itemID, s.toName, done.To, done.ItemID, done.RemainHP, done.Charges, moved)
	dabrishSay(sess, fmt.Sprintf(dabrishEpicDone, s.toName, s.fromName))
}

// versionsBase is a record's base, read now (moveEpicPick's).
func (e *dabrishEpics) versionsBase() func(int) int {
	asc, imp, err := e.inv.ItemVersions()
	if err != nil {
		log.Printf("Dabrish: the records' versions: %v", err)
	}
	v := epicVersions{asc: asc, imp: imp}
	return func(id int) int { b, _ := v.base(id); return b }
}

// moveEpicPick moves the character's kept level 49 weapon picks from the
// option giving from to the one giving to with the same spells (the cleric's
// "Elemental Bulwark and the Ascended Mace of Repentence"), as a
// rehabilitator's change keeps one (setPick). It returns how many it moved.
func (q *QuestEngine) moveEpicPick(serverID uint32, from, to int, base func(int) int) int {
	if q == nil {
		return 0
	}
	gives := func(o database.StepOption, b int) bool {
		for _, it := range o.Reward.Items {
			if base(it.PatternID) == b {
				return true
			}
		}
		return false
	}
	moved := 0
	for _, pq := range q.pathQuests(serverID) {
		if pq.level != 49 {
			continue
		}
		picks := database.ParsePicks(pq.row.Picks)
		for i := range pq.quest.Steps {
			st := &pq.quest.Steps[i]
			opts := st.Choices.Options
			k, ok := picks[st.Step]
			if !ok || k >= len(opts) || !gives(opts[k], from) {
				continue
			}
			for j, o := range opts {
				if j != k && gives(o, to) && slices.Equal(o.Reward.Spells, opts[k].Reward.Spells) {
					if q.setPick(serverID, pq.quest.ID, st.Step, j) {
						moved++
					}
					break
				}
			}
		}
	}
	return moved
}
