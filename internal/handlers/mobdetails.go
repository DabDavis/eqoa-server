package handlers

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// mobdetails.go — a creature's details and loot chart (world.mob_details;
// Heirloom petition 3163: "view a mob's loot chart: right-click a mob to view
// its details like stats and loot table"; the owner approved it).
//
// A client that knows it (vanilla: a right click on a creature) asks with
// OpcodeMobDetailsAsk naming the creature as it knows it, and is answered with
// OpcodeMobDetails: its name, level, class, the asker's consider colour, its
// hit points, and what its corpse can hold, read off the very loot model a
// corpse rolls with (loot.go's rollCorpse), so the chart is what can drop and
// never a promise of what will:
//
//   - its own items (npcitem): every kill;
//   - what it holds (helddrop.go): every kill, its look's rare record 1 in
//     world.held_rare_one_in in place of the plain one;
//   - its loot tables (npcloot + loottables, by lootNames: its name, its row,
//     then the tables every creature rolls), each item's chance a kill from
//     the table's chance, rolls and the item's weight among the table's items
//     for the creature's level;
//   - its region's categories (lootcategory.go), summarised: a row a category,
//     its chance of an item a kill and how many items its pool holds;
//   - the quests' loot step items its kill drops (questpreloot.go): those that
//     drop to anyone, and those the asker is on the step of or would have
//     ahead of it, at the step's chance.
//
// Left out, as the corpse leaves them out: equipment past the creature's
// level reach and scrolls below it (lootFits, judged at the creature's level as
// the corpse is), a table row of no weight, a binding of no chance, a disabled
// quest's step but for a player on it, and an encounter's item that drops to no
// one off the step (aheadExcludedLocked). A creature whose corpse holds
// nothing (a pet's, a charmed one's, a dummy's, an add's: OpenCorpse) shows no
// loot. Coins are not listed.
//
// Only a monster is told of: a living NPC without the NPC bit (mobInfoOf's,
// the creatures CLIENT lets a player attack), no pet or mercenary; anything
// else is answered status 1. The quality-of-life lock holds (vanillaui.go,
// ogaccount.go): on a world.vanilla_ui shard or for an [OG] character no offer
// goes and an ask is answered status 3. Asks are limited per session
// (mobDetailsBurst at once, then one every mobDetailsEvery), past it status 2.

// mobDetailsOn is world.mob_details: off unless set.
var mobDetailsOn atomic.Bool

// SetMobDetails turns the creatures' details on or off (world.mob_details).
func SetMobDetails(on bool) { mobDetailsOn.Store(on) }

// MobDetailsOn reports world.mob_details.
func MobDetailsOn() bool { return mobDetailsOn.Load() }

// mobDetailsLayout is OpcodeMobDetails' layout.
const mobDetailsLayout = 1

// OpcodeMobDetails' statuses.
const (
	mobDetailsShown = 0
	mobDetailsNone  = 1
	mobDetailsSoon  = 2
	mobDetailsOff   = 3
)

// The loot rows' sources (OpcodeMobDetails).
const (
	lootFromOwn      = 0
	lootFromHeld     = 1
	lootFromTable    = 2
	lootFromCategory = 3
	lootFromQuest    = 4
	lootFromEvery    = 5
)

// mobDetailsRowsMost is the most loot rows an answer carries (a flag says
// more were left out); mobDetailsTextMost a row's or a line's most bytes.
const (
	mobDetailsRowsMost = 150
	mobDetailsTextMost = 63
)

// The asks' limit: mobDetailsBurst at once, then one every mobDetailsEvery.
const (
	mobDetailsBurst = 3
	mobDetailsEvery = 2 * time.Second
)

// lootRow is one row of the chart: where it comes from, its chance a kill
// (0 to 1), the item record (0 for a category's row) and its text.
type lootRow struct {
	Source  uint8
	Chance  float64
	Pattern int
	Text    string
}

// mobDetails is what an answer of status 0 says.
type mobDetails struct {
	Name      string
	Level     int
	Class     string
	Con       byte
	HP, MaxHP int
	Lines     []string
	Rows      []lootRow
	LeftOut   bool
}

// SendMobDetailsOffer offers the session's client the creatures' details,
// where world.mob_details is on and the session has the quality-of-life
// additions; elsewhere nothing goes.
func SendMobDetailsOffer(sess *session.Session) {
	if !MobDetailsOn() || !QoLOnFor(sess) {
		return
	}
	op := uint16(protocol.OpcodeMobDetailsOffer)
	queueReliable(sess, protocol.OpcodeMobDetailsOffer, []byte{byte(op), byte(op >> 8), mobDetailsLayout})
}

// chanceBasis is a chance (0 to 1) as the wire carries it: hundredths of a
// percent, at least 1 for any chance above none, 10000 at most.
func chanceBasis(p float64) uint16 {
	if !(p > 0) {
		return 0
	}
	v := math.Round(p * 10000)
	if v < 1 {
		return 1
	}
	if v > 10000 {
		return 10000
	}
	return uint16(v)
}

// appendText appends s as a u8-counted ASCII string of at most most bytes
// (what a PS2 can type: consoletext.go).
func appendText(b []byte, s string, most int) []byte {
	s = consoleLine(s, most)
	if len(s) > most {
		s = s[:most]
	}
	b = append(b, byte(len(s)))
	return append(b, s...)
}

// buildMobDetails is OpcodeMobDetails: the id asked about, the status, and for
// status 0 the details (d).
func buildMobDetails(objectID uint32, status byte, d *mobDetails) []byte {
	b := binary.LittleEndian.AppendUint16(nil, uint16(protocol.OpcodeMobDetails))
	b = append(b, mobDetailsLayout)
	b = binary.LittleEndian.AppendUint32(b, objectID)
	b = append(b, status)
	if status != mobDetailsShown || d == nil {
		return b
	}
	b = appendText(b, d.Name, mobDetailsTextMost)
	b = append(b, byte(max(0, min(d.Level, 255))))
	b = appendText(b, d.Class, mobDetailsTextMost)
	b = append(b, d.Con)
	b = binary.LittleEndian.AppendUint32(b, uint32(max(d.HP, 0)))
	b = binary.LittleEndian.AppendUint32(b, uint32(max(d.MaxHP, 0)))
	lines := d.Lines
	if len(lines) > 255 {
		lines = lines[:255]
	}
	b = append(b, byte(len(lines)))
	for _, l := range lines {
		b = appendText(b, l, mobDetailsTextMost)
	}
	rows, left := d.Rows, d.LeftOut
	if len(rows) > mobDetailsRowsMost {
		rows, left = rows[:mobDetailsRowsMost], true
	}
	b = binary.LittleEndian.AppendUint16(b, uint16(len(rows)))
	var flags byte
	if left {
		flags |= 1
	}
	b = append(b, flags)
	for _, r := range rows {
		b = append(b, r.Source)
		b = binary.LittleEndian.AppendUint16(b, chanceBasis(r.Chance))
		b = binary.LittleEndian.AppendUint32(b, uint32(max(r.Pattern, 0)))
		b = appendText(b, r.Text, mobDetailsTextMost)
	}
	return b
}

// mobDetailsAsks is each session's asks' bucket (the limit).
type mobDetailsAsks struct {
	mu   sync.Mutex
	left map[*session.Session]askBucket
}

type askBucket struct {
	tokens float64
	at     time.Time
}

// take reports whether the session may ask now, spending one ask; buckets of
// sessions quiet past a refill are forgotten as it goes.
func (a *mobDetailsAsks) take(sess *session.Session, now time.Time) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.left == nil {
		a.left = map[*session.Session]askBucket{}
	}
	full := time.Duration(mobDetailsBurst) * mobDetailsEvery
	if len(a.left) > 256 {
		for s, b := range a.left {
			if now.Sub(b.at) > full {
				delete(a.left, s)
			}
		}
	}
	b, ok := a.left[sess]
	if !ok {
		b = askBucket{tokens: mobDetailsBurst, at: now}
	}
	b.tokens = math.Min(mobDetailsBurst, b.tokens+now.Sub(b.at).Seconds()/mobDetailsEvery.Seconds())
	b.at = now
	if b.tokens < 1 {
		a.left[sess] = b
		return false
	}
	b.tokens--
	a.left[sess] = b
	return true
}

// MobDetailsHandler answers OpcodeMobDetailsAsk.
type MobDetailsHandler struct {
	gw  *entity.World
	win *NPCWindowHandler
	// questRows is the quests' loot step items of a kill of the creature for the
	// asker (QuestEngine.LootChartRows); nil, none
	questRows func(k entity.KillCredit, asker uint32) []lootRow
	// find and playerOf stand in for the World's (tests)
	find     func(*session.Session, uint32) (entity.Entity, bool)
	playerOf func(*session.Session) *entity.Player
	now      func() time.Time
	asks     mobDetailsAsks
}

// NewMobDetailsHandler creates the creatures' details' handler.
func NewMobDetailsHandler(gw *entity.World, win *NPCWindowHandler) *MobDetailsHandler {
	h := &MobDetailsHandler{gw: gw, win: win, now: time.Now}
	if gw != nil {
		h.find = gw.FindEntityByObjectID
		h.playerOf = gw.SessionPlayer
	}
	return h
}

// RegisterHandlers registers the ask.
func (h *MobDetailsHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeMobDetailsAsk, h.HandleMobDetailsAsk)
}

// HandleMobDetailsAsk answers an ask: a u8 layout and the u32 id the client
// knows a creature by.
func (h *MobDetailsHandler) HandleMobDetailsAsk(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 5 {
		log.Printf("MobDetails: short ask (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	id := binary.LittleEndian.Uint32(data[1:5])
	queueReliable(sess, protocol.OpcodeMobDetails, h.answer(sess, id))
	return nil
}

// answer is the session's answer about the creature it knows by id.
func (h *MobDetailsHandler) answer(sess *session.Session, id uint32) []byte {
	if !MobDetailsOn() || !QoLOnFor(sess) {
		return buildMobDetails(id, mobDetailsOff, nil)
	}
	if !h.asks.take(sess, h.now()) {
		return buildMobDetails(id, mobDetailsSoon, nil)
	}
	var p *entity.Player
	if h.playerOf != nil {
		p = h.playerOf(sess)
	}
	if p == nil || h.find == nil {
		return buildMobDetails(id, mobDetailsNone, nil)
	}
	e, ok := h.find(sess, id)
	npc, isNPC := e.(*entity.NPC)
	if !ok || !isNPC || npc.Owner != 0 || npc.IsMerc() {
		return buildMobDetails(id, mobDetailsNone, nil)
	}
	name, level, monster := mobInfoOf(npc)
	if !monster {
		return buildMobDetails(id, mobDetailsNone, nil)
	}
	d := &mobDetails{Name: name, Level: level, Class: entity.ClassName(npc.Class), Con: entity.ConColor(p.Level, level), HP: npc.HP, MaxHP: npc.MaxHP}
	if h.win != nil {
		d.Lines, d.Rows = h.win.lootChart(npc)
	}
	if h.questRows != nil && h.win != nil && h.win.lootsAnything(npc) {
		spawn := npc.ServerID
		if npc.Temporary {
			spawn = 0
		}
		k := entity.KillCredit{NPCName: npc.Name, NPCSpawn: spawn, NPCID: npc.ServerID, Level: npc.Level, World: npc.World, X: npc.X, Z: npc.Z, OriginalRow: npc.Row()}
		d.Rows = append(d.Rows, h.questRows(k, p.CharServerID)...)
	}
	log.Printf("MobDetails: %s looked at %q (level %d): %d loot rows", p.Name, name, level, len(d.Rows))
	return buildMobDetails(id, mobDetailsShown, d)
}

// lootsAnything is whether npc's corpse holds anything at all (OpenCorpse's
// refusals of a pet's, an add's: none).
func (h *NPCWindowHandler) lootsAnything(npc *entity.NPC) bool {
	return npc.Owner == 0 && !npc.CorpseOfNothing()
}

// lootChart is what npc's corpse can hold, as rollCorpse rolls it at the
// creature's own level: info lines and the rows (the file's doc).
func (h *NPCWindowHandler) lootChart(npc *entity.NPC) (lines []string, rows []lootRow) {
	if !h.lootsAnything(npc) {
		return []string{"Its corpse holds nothing."}, nil
	}
	level := npc.Level
	if h.inv != nil {
		// its own items, every kill
		if items, err := h.inv.GetNPCItems(npc.NPCID); err != nil {
			log.Printf("MobDetails: %q (npc %d): %v", npc.Name, npc.NPCID, err)
		} else {
			items, _ = fitItems(items, level)
			for _, it := range items {
				rows = append(rows, lootRow{Source: lootFromOwn, Chance: 1, Pattern: it.Pattern.PatternID, Text: it.Pattern.ItemName})
			}
		}
	}
	rows = append(rows, heldChart(npc, level)...)
	if h.inv != nil {
		for _, name := range lootNames(npc) {
			rolls, err := h.inv.NPCLootRolls(name, npc.Level)
			if err != nil {
				log.Printf("MobDetails: %q's loot tables (%q): %v", npc.Name, name, err)
				continue
			}
			src := uint8(lootFromTable)
			if name == lootEveryCreature {
				src = lootFromEvery
			}
			for i := range rolls {
				rolls[i].Choices, _ = fitChoices(rolls[i].Choices, level)
			}
			rows = append(rows, tableChart(rolls, src)...)
		}
		if rollsCategories(npc) {
			cats, err := h.inv.CategoryRollsOf(entity.RowKey(npc.Row()), npc.Name, npc.Level)
			if err != nil {
				log.Printf("MobDetails: %q's loot categories: %v", npc.Name, err)
			}
			for i := range cats {
				choiceLevel := level
				if cats[i].Category == "sky twins ascended" { // as rollCorpse: no reach judged
					choiceLevel = 0
				}
				cats[i].Choices, _ = fitChoices(cats[i].Choices, choiceLevel)
			}
			rows = append(rows, categoryChart(cats)...)
		}
	}
	return nil, mergeLootRows(rows)
}

// heldChart is what npc holds as its corpse drops it (heldDropItems): a named
// creature's own record every kill; else its look's plain record, and its
// rare one 1 in RareOneIn in its place.
func heldChart(npc *entity.NPC, level int) []lootRow {
	d := heldDrops.Load()
	if d == nil || !rollsCategories(npc) {
		return nil
	}
	var out []lootRow
	add := func(e database.CharInventoryEntry, p float64) {
		if p > 0 && lootFits(&e.Pattern, level) {
			out = append(out, lootRow{Source: lootFromHeld, Chance: p, Pattern: e.Pattern.PatternID, Text: e.Pattern.ItemName})
		}
	}
	for hand, p := range []struct {
		model  uint32
		shield bool
	}{{uint32(npc.PrimaryWeapon), false}, {uint32(npc.SecondaryWeapon), false}, {uint32(npc.Shield), true}} {
		if p.model == 0 {
			continue
		}
		if e, ok := d.Named(npc.Name, hand); ok {
			add(e, 1)
			continue
		}
		rare := 0.0
		if d.RareOneIn > 0 {
			rare = 1 / float64(d.RareOneIn)
		}
		plain, okPlain := d.Pick(p.model, p.shield, npc.Level, npc.Name, false)
		r, okRare := d.Pick(p.model, p.shield, npc.Level, npc.Name, true)
		switch {
		case okRare && d.Rare(r.Pattern.PatternID) && okPlain:
			add(plain, 1-rare)
			add(r, rare)
		case okRare && d.Rare(r.Pattern.PatternID):
			add(r, rare) // its look has rare records alone
		case okPlain:
			add(plain, 1)
		}
	}
	return out
}

// tableChart is the items of a creature's loot tables (rollLoot's rolls) with
// each one's chance a kill: a roll lands at the table's chance and takes the
// item by its weight among the table's, the table rolled its rolls times.
func tableChart(rolls []database.LootRoll, src uint8) []lootRow {
	var out []lootRow
	for _, r := range rolls {
		total := 0
		for _, c := range r.Choices {
			total += max(c.Weight, 0)
		}
		hit := math.Max(0, math.Min(r.Chance/100, 1))
		if total == 0 || hit == 0 || r.Rolls <= 0 {
			continue
		}
		for _, c := range r.Choices {
			if c.Weight <= 0 {
				continue
			}
			each := hit * float64(c.Weight) / float64(total)
			out = append(out, lootRow{Source: src, Chance: 1 - math.Pow(1-each, float64(r.Rolls)), Pattern: c.Item.Pattern.PatternID, Text: c.Item.Pattern.ItemName})
		}
	}
	return out
}

// categoryChart is a creature's categories (rollCategories), a row each: its
// chance of an item a kill (the rate, or every count but none's share) and the
// items its pool holds for the creature's level; a category whose pool is
// empty gives nothing and is left out.
func categoryChart(cats []database.CategoryRoll) []lootRow {
	var out []lootRow
	for _, c := range cats {
		n := 0
		for _, ch := range c.Choices {
			if ch.Weight > 0 {
				n++
			}
		}
		if n == 0 {
			continue
		}
		p := 0.0
		if len(c.Counts) > 0 {
			sum := 0.0
			for _, v := range c.Counts {
				sum += max(v, 0)
			}
			if sum > 0 {
				p = 1 - max(c.Counts[0], 0)/sum
			}
		} else {
			p = math.Max(0, math.Min(c.Rate, 1))
		}
		if p <= 0 {
			continue
		}
		word := "items"
		if n == 1 {
			word = "item"
		}
		out = append(out, lootRow{Source: lootFromCategory, Chance: p, Text: fmt.Sprintf("%s: one of %d %s", c.Category, n, word)})
	}
	return out
}

// mergeLootRows joins the rows of one item from one kind of source (two tables
// giving it: either), keeping the first one's place.
func mergeLootRows(rows []lootRow) []lootRow {
	out := rows[:0:0]
	at := map[[2]int]int{}
	for _, r := range rows {
		if r.Pattern == 0 {
			out = append(out, r)
			continue
		}
		k := [2]int{int(r.Source), r.Pattern}
		if i, ok := at[k]; ok {
			out[i].Chance = 1 - (1-out[i].Chance)*(1-r.Chance)
			continue
		}
		at[k] = len(out)
		out = append(out, r)
	}
	return out
}

// LootChartRows is the quests' loot step items a kill of the creature k
// describes drops (questpreloot.go's corpseDropsLocked, read and not rolled):
// those it drops to anyone, and for asker (a character) those of a step it is
// on and those it would have ahead of their step, each at the highest chance of
// its steps. Its text names the item, and "your quest" where the asker is on a
// step of it.
func (q *QuestEngine) LootChartRows(k entity.KillCredit, asker uint32) []lootRow {
	q.mu.Lock()
	defer q.mu.Unlock()
	c := noQuests
	if asker != 0 {
		c = q.charLocked(asker)
	}
	type drop struct {
		pat    *database.ItemPatternRecord
		chance float64
		mine   bool
	}
	var order []int
	byItem := map[int]*drop{}
	for _, ref := range q.lootStepsForLocked(k) {
		qu, st := ref.qu, ref.st
		pid, _, ok := st.LootItem()
		if !ok {
			continue
		}
		pat, err := q.items.GetItemPattern(pid)
		if err != nil || pat == nil {
			continue
		}
		mine := onLootStep(c, qu, st)
		drops := mine || q.openDropLocked(qu, st, pat, k.NPCName) ||
			(qu.Enabled && q.aheadRefusalLocked(c, qu, st, k.NPCName) == "" && q.aheadExcludedLocked(qu, st, pat) == "")
		if !drops {
			continue
		}
		d := byItem[pid]
		if d == nil {
			d = &drop{pat: pat}
			byItem[pid] = d
			order = append(order, pid)
		}
		d.chance = max(d.chance, database.LootChance(st.Chance))
		d.mine = d.mine || mine
	}
	out := make([]lootRow, 0, len(order))
	for _, pid := range order {
		d := byItem[pid]
		text := strings.TrimSpace(d.pat.ItemName)
		if d.mine {
			text += " (your quest)"
		}
		out = append(out, lootRow{Source: lootFromQuest, Chance: d.chance, Pattern: pid, Text: text})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return strings.HasSuffix(out[i].Text, "(your quest)") && !strings.HasSuffix(out[j].Text, "(your quest)")
	})
	return out
}
