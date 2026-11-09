package handlers

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// archivist.go — the Spell Archivist's questions (the GM Jibrish's petition
// 1279, 2026-11-08: "Add a dialogue option to the Spell Archivist. This should
// be "Grant basic spells". This should grant all the spells of the player's
// class that are vendor bought, from level 1 to their current level (cap 60),
// not master class or mastery spells, only those missing", and an option to
// clear the spell book, keeping Return Home). He is a merchant (his shop sells
// the buyer's class's scrolls past the class vendors' 49: buyback.go's
// classShop), so CLIENT never talks to him: InteractWithTarget asks for his
// window (0x004C) and sends no 0x0004. That request is answered with his
// question, in a box under archivistBoxMark, and 0x00D7, which clears CLIENT's
// pending request, as a shop offers its quest (questsAtWindow). His answers:
//
//   - Browse your scrolls: his shop, as it always opened, under the id the
//     client named him by (each buy and sale hands it back), while he is in
//     reach (windowNPC), as a genie's wares open (genie.go);
//   - Grant basic spells: every spell of the player's class a merchant sells
//     as a scroll (spellArchive.basic) and, since petition 1361, every one its
//     quests teach beside the paths (sideSpells, archivistside.go: the class
//     quests' level 3 to 15 spells, the ability quests' 23 to 47, ...), that
//     its book lacks, by its record or its exact name as the scribe tells a
//     spell known (scribe.go), at the next display indices below 252, off the
//     bar, as a scroll scribed goes in; nothing is taken out. The client is
//     sent 0x00B3 for each with the flag 0, so word 721 ("You have gained a
//     new ability.") is not said dozens of times, and one box says how many;
//   - Grant path spells: one path of the class's at a time, the spells its
//     path quests give at 20, 30, 40 and 49 (archivistpaths.go: petitions
//     1325 and 1361), another path's taken out for it;
//   - Clear my spellbook: every spell and ability out of the book but Return
//     Home (any record so named; its bar slot stays), asked Yes or No first.
//     The client's book loses each at once by 0x00B8, which erases the FIRST
//     ability of a display index (Msg00B8), so one sharing an index with
//     Return Home is not sent (it could take Return Home instead), nor one at
//     252 or more (it has no index of its own): those leave the client's book
//     at the next login, and the player is told. CLIENT draws the memorized
//     bar from the ability records, so a memorized spell leaves the bar with
//     it; the L2 menu's quick chat is text, not abilities, and stays;
//   - Nothing, thank you.
//
// Any NPC named SpellArchivistName is one; one stands in Freeport on the
// Heirloom Test Server alone, and a server with none never comes here. The
// words are ours.

// archivistBoxMark marks his questions' ids (beside the quests' 0x51 and the
// others to Dabrish's 0x5C).
const archivistBoxMark = 0x5D000000

// IsArchivistBox reports a box a Spell Archivist asked.
func IsArchivistBox(id uint32) bool { return id&0xFF000000 == archivistBoxMark }

// isSpellArchivist is whether an NPC is a Spell Archivist (classShop's test).
func isSpellArchivist(npc *entity.NPC) bool { return npc != nil && npc.Name == SpellArchivistName }

// archivistLevelCap is the highest level whose spells he grants (the
// petition's "cap 60").
const archivistLevelCap = 60

// His question's answers, in the box's order.
const (
	archivistBrowse = iota
	archivistGrant
	archivistPath
	archivistClear
	archivistNothing
)

var archivistAnswers = []string{"Browse your scrolls", "Grant basic spells", "Grant path spells", "Clear my spellbook", "Nothing, thank you"}

// The questions he asks.
const (
	archivistAskMenu  = iota // archivistAnswers
	archivistAskClear        // Yes or No to clearing the book
	archivistAskPath         // which path (archivistpaths.go)
	archivistAskSwap         // Yes or No to a path's change
)

// What he says (ours). A question shows 127 characters (questAskChars), an
// answer 63 and a box 217 (questBoxChars), his name included.
const (
	archivistQuestion       = SpellArchivistName + ": 'Your calling's scrolls and paths are in my keeping. I can write their spells into your book, or clear it.'"
	archivistClearIs        = "I take every spell and ability out of your book but Return Home, those your quests and masteries gave too. What the merchants sell and your calling's quests teach I can grant again, and one path."
	archivistClearAsk       = SpellArchivistName + ": 'Shall I clear your book?'"
	archivistClearYes       = "Yes, clear it"
	archivistClearNo        = "No"
	archivistGranted        = "There: %s of your calling written into your book, every one the merchants sell or its quests teach up to level %d that you lacked."
	archivistGrantedSome    = "There: %s written into your book, which has no room for %d more. Make room in it and ask again."
	archivistHasAll         = "Your book holds every spell of your calling the merchants sell or its quests teach up to level %d already."
	archivistBookFull       = "Your book has no room for another spell."
	archivistCleared        = "Your book is clear but for Return Home: %s gone."
	archivistNothingToClear = "Your book holds nothing but Return Home: there is nothing to clear."
	archivistKept           = "As you wish: your book stays as it is."
	archivistTrouble        = "Something went wrong with my books. Try again."
	archivistAway           = "You are too far from the " + SpellArchivistName + "."
	archivistLater          = "%s will be gone from your book at your next login."
)

// spellCount is n spells in words.
func spellCount(n int) string {
	if n == 1 {
		return "1 spell"
	}
	return fmt.Sprintf("%d spells", n)
}

// Archivist runs the Spell Archivists over the spell book store and the window
// handler: his shop, and windowNPC, which tells he is still in reach.
type Archivist struct {
	spells  *database.SpellbookStore
	windows *NPCWindowHandler
	now     func() time.Time
	// quests is the running quests (Quests): the paths' reward quests and the
	// quests a character did (archivistpaths.go)
	quests func() *QuestEngine

	mu    sync.Mutex
	seq   uint32
	asked map[uint32]archivistAsk // by the character's serverid

	// book is held through a grant or a clear, so two cannot read a book
	// before either writes it and write the same spells twice; and the archive
	// with it, kept archiveTTL from when it was read (archiveAt)
	book      sync.Mutex
	archive   *spellArchive
	archiveAt time.Time
}

// archiveTTL is how long the archive is kept before it is read again, as a
// genie keeps its shop (genieShopTTL): finding the merchants reads every npcs
// row (0.26 s on the Heirloom server's 61,245, on the one connection the
// server's database has), and a GM's change to a merchant's stock, classspells
// or the spells reaches a grant within it.
const archiveTTL = 10 * time.Minute

// archivistAsk is a question asked: its box, which question, and the id the
// client named the Archivist by (its 0x004C), which his shop opens under; a
// path's: the paths offered, by answer, with their names (archivistAskPath),
// and the path and the change as they were asked (archivistAskSwap: its name,
// the spells out and in). A path is done only while its index still names it.
type archivistAsk struct {
	id       uint32
	stage    int
	npc      uint32
	paths    []int
	names    []string
	path     int
	pathName string
	out, in  []int
}

// NewArchivist makes the Spell Archivists over the spell book store and the
// window handler, whose merchant requests to one he answers first
// (HandleMerchantOpen).
func NewArchivist(spells *database.SpellbookStore, windows *NPCWindowHandler) *Archivist {
	a := &Archivist{spells: spells, windows: windows, now: time.Now, quests: Quests, asked: map[uint32]archivistAsk{}}
	if windows != nil {
		windows.archivist = a
	}
	return a
}

func (a *Archivist) say(sess *session.Session, line string) {
	for _, part := range splitSay(SpellArchivistName+": "+line, questBoxChars) {
		SendDialogueBox(sess, 0, part)
	}
}

func (a *Archivist) ask(sess *session.Session, p *entity.Player, stage int, npc uint32, question string, answers []string) {
	a.askWith(sess, p, archivistAsk{stage: stage, npc: npc}, question, answers)
}

// askWith asks a question kept as ask (its box id made here).
func (a *Archivist) askWith(sess *session.Session, p *entity.Player, ask archivistAsk, question string, answers []string) {
	a.mu.Lock()
	a.seq++
	ask.id = archivistBoxMark | (a.seq & 0xFFFFFF)
	a.asked[p.CharServerID] = ask
	a.mu.Unlock()
	SendQuestPopupMulti(sess, ask.id, clipRunes(question, questAskChars), answers)
}

// Ask is a merchant request (HandleMerchantOpen; id the object the client
// named): his question when npc is a Spell Archivist, the window waiting for
// "Browse your scrolls". It reports whether it asked; the request is then
// answered 0x00D7, so CLIENT takes window requests again.
func (a *Archivist) Ask(sess *session.Session, p *entity.Player, npc *entity.NPC, id uint32) bool {
	if a == nil || a.spells == nil || p == nil || !isSpellArchivist(npc) || npc.Dead {
		return false
	}
	forgetDialogs(p)
	a.ask(sess, p, archivistAskMenu, id, archivistQuestion, archivistAnswers)
	log.Printf("Archivist: %s asked what %s wishes", npc.Name, p.Name)
	return true
}

// Answer is HandleDialogueOption's question for his box: the answer carried
// out. It reports whether the box was his (an old one does nothing).
func (a *Archivist) Answer(sess *session.Session, p *entity.Player, box uint32, option byte) bool {
	if !IsArchivistBox(box) {
		return false
	}
	if a == nil || p == nil || option == 255 {
		// a close is not the last word: the PS2 sends it just before the
		// real pick when two boxes were open (interact.go)
		return true
	}
	a.mu.Lock()
	ask, ok := a.asked[p.CharServerID]
	if ok && ask.id == box {
		delete(a.asked, p.CharServerID)
	}
	a.mu.Unlock()
	if !ok || ask.id != box {
		return true
	}
	n := int(option)
	switch ask.stage {
	case archivistAskMenu:
		switch n {
		case archivistBrowse:
			a.browse(sess, ask.npc)
		case archivistGrant:
			a.grant(sess, ask.npc)
		case archivistPath:
			a.askPaths(sess, ask.npc)
		case archivistClear:
			a.askClear(sess, ask.npc)
		}
	case archivistAskClear:
		if n == 0 {
			a.clear(sess, ask.npc)
		} else {
			a.say(sess, archivistKept)
		}
	case archivistAskPath:
		if n < len(ask.paths) && n < len(ask.names) {
			a.pickPath(sess, ask.npc, ask.paths[n], ask.names[n])
		}
	case archivistAskSwap:
		if n == 0 {
			a.swapPath(sess, ask)
		} else {
			a.say(sess, archivistKept)
		}
	}
	return true
}

// here is the player and the Archivist the client named id while he is still
// one, alive, in reach (windowNPC); the player is told when he is not.
func (a *Archivist) here(sess *session.Session, id uint32) (*entity.Player, *entity.NPC, bool) {
	if a.windows != nil {
		if p, npc, ok := a.windows.windowNPC(sess, id, npcFlagMerchant, "Archivist"); ok && isSpellArchivist(npc) && !npc.Dead {
			return p, npc, true
		}
	}
	SendColoredChat(sess, archivistAway, scribeRefusalRGB[0], scribeRefusalRGB[1], scribeRefusalRGB[2])
	return nil, nil, false
}

// browse opens his shop under the id the client named him by.
func (a *Archivist) browse(sess *session.Session, id uint32) {
	if p, npc, ok := a.here(sess, id); ok {
		a.windows.openMerchant(sess, p, npc, id, merchantGreeting)
	}
}

// grant writes the basic spells the player's book lacks into it (the notes at
// the top).
func (a *Archivist) grant(sess *session.Session, id uint32) {
	p, _, ok := a.here(sess, id)
	if !ok {
		return
	}
	a.book.Lock()
	defer a.book.Unlock()
	trouble := func(what string, err error) {
		log.Printf("Archivist: %s's grant, %s: %v", p.Name, what, err)
		a.say(sess, archivistTrouble)
	}
	arch, err := a.archived()
	if err != nil {
		trouble("the archive", err)
		return
	}
	book, err := a.spells.GetCharacterSpells(p.CharServerID)
	if err != nil {
		trouble("the book", err)
		return
	}
	level := min(p.Level, archivistLevelCap)
	want := lacking(book, basicSpells(a.quests(), arch, p.Class, p.Race, p.Level))
	if owned, err := a.spells.OwnedRanks(p.CharServerID); err != nil {
		trouble("the masteries", err)
		return
	} else if taken := takenByOwned(arch, owned); len(taken) > 0 {
		kept := make([]database.SpellName, 0, len(want))
		for _, s := range want {
			if !taken[s.SpellID] {
				kept = append(kept, s)
			}
		}
		want = kept
	}
	if len(want) == 0 {
		a.say(sess, fmt.Sprintf(archivistHasAll, level))
		return
	}
	ids := make([]int, len(want))
	for i, s := range want {
		ids[i] = s.SpellID
	}
	order := nextDisplayIndex(book)
	n, err := a.spells.AddSpells(p.CharServerID, ids, order)
	if err != nil {
		trouble("writing", err)
		return
	}
	if n == 0 {
		log.Printf("Archivist: %s's book is full: %d basic spells lacked", p.Name, len(ids))
		a.say(sess, archivistBookFull)
		return
	}
	// what went in, as the book now holds it: 0x00B3 each, the flag 0
	if book, err = a.spells.GetCharacterSpells(p.CharServerID); err != nil {
		log.Printf("Archivist: %s's book after the grant: %v (the spells are written, shown at the next login)", p.Name, err)
	}
	added := make(map[int]bool, n)
	for _, id := range ids[:n] {
		added[id] = true
	}
	for i := range book {
		if e := &book[i]; added[e.Pattern.SpellID] && e.AddedOrder >= order && e.AddedOrder < order+n {
			SendAbilityAdd(sess, 0, false, e)
		}
	}
	log.Printf("Archivist: %s (class %d, level %d) granted %d basic spells at display indices %d-%d, %d with no room",
		p.Name, p.Class, p.Level, n, order, order+n-1, len(ids)-n)
	if n < len(ids) {
		a.say(sess, fmt.Sprintf(archivistGrantedSome, spellCount(n), len(ids)-n))
		return
	}
	a.say(sess, fmt.Sprintf(archivistGranted, spellCount(n), level))
}

// askClear says what a clear takes, and asks.
func (a *Archivist) askClear(sess *session.Session, id uint32) {
	p, _, ok := a.here(sess, id)
	if !ok {
		return
	}
	a.say(sess, archivistClearIs)
	a.ask(sess, p, archivistAskClear, id, archivistClearAsk, []string{archivistClearYes, archivistClearNo})
}

// clear takes every spell but Return Home out of the book and out of the
// client's (the notes at the top).
func (a *Archivist) clear(sess *session.Session, id uint32) {
	p, _, ok := a.here(sess, id)
	if !ok {
		return
	}
	a.book.Lock()
	defer a.book.Unlock()
	book, err := a.spells.GetCharacterSpells(p.CharServerID)
	if err != nil {
		log.Printf("Archivist: %s's clear, the book: %v", p.Name, err)
		a.say(sess, archivistTrouble)
		return
	}
	home := map[int]bool{} // the display indices Return Home holds
	var gone []database.CharSpellEntry
	for _, e := range book {
		if database.IsReturnHome(e.Pattern.SpellName) {
			home[e.AddedOrder] = true
		} else {
			gone = append(gone, e)
		}
	}
	if len(gone) == 0 {
		a.say(sess, archivistNothingToClear)
		return
	}
	var ids []int
	seen := map[int]bool{}
	for _, e := range gone {
		if !seen[e.Pattern.SpellID] {
			seen[e.Pattern.SpellID] = true
			ids = append(ids, e.Pattern.SpellID)
		}
	}
	rows, err := a.spells.RemoveSpells(p.CharServerID, ids)
	if err != nil {
		log.Printf("Archivist: %s's clear: %v (nothing taken)", p.Name, err)
		a.say(sess, archivistTrouble)
		return
	}
	// one 0x00B8 a row (takeRows, archivistpaths.go), none at an index
	// Return Home holds
	later := takeRows(sess, gone, home)
	log.Printf("Archivist: %s cleared %d spells (%d rows) from the book, %d of them gone from the client at the next login",
		p.Name, len(ids), rows, len(later))
	a.say(sess, fmt.Sprintf(archivistCleared, spellCount(len(ids))))
	if len(later) > 0 {
		what := strings.Join(later, ", ")
		if len(later) > 4 {
			what = fmt.Sprintf("%d of those spells", len(later))
		}
		SendColoredChat(sess, fmt.Sprintf(archivistLater, what), 255, 255, 0)
	}
}

// lacking is the spells of want a book holds neither by record nor by exact
// name, the first of each name kept, as the scribe tells a spell known
// (scribe.go): a "+" record is another spell, so a bard who has Anthem of
// Hope + lacks Anthem of Hope.
func lacking(book []database.CharSpellEntry, want []database.SpellName) []database.SpellName {
	ids, names := map[int]bool{}, map[string]bool{}
	for _, e := range book {
		ids[e.Pattern.SpellID] = true
		names[exactKey(e.Pattern.SpellName)] = true
	}
	var out []database.SpellName
	for _, s := range want {
		if key := exactKey(s.Name); !ids[s.SpellID] && !names[key] {
			ids[s.SpellID], names[key] = true, true
			out = append(out, s)
		}
	}
	return out
}

// spellArchive is what the grant reads: every scroll a merchant sells, every
// spell pattern by name (a scroll's spell is found among them as the scribe
// finds it), each spell's description, and which classes have each spell and
// how (classspells' sources).
type spellArchive struct {
	scrolls []database.MerchantScroll
	spells  *spellIndex
	names   map[int]database.SpellName // every spell pattern by id (the paths', archivistpaths.go)
	sold    map[string]int             // the scrolls' class masks by their names' spellKey (soldTo)
	// every scroll's record by patternid, sold or not: a quest's reward
	// scroll is read off it (archivistside.go)
	scrollRecs map[int]database.MerchantScroll
	descs      map[int]string
	sources    map[int]map[int]string // spell -> class -> source
	// the enabled mastery ranks: what buying each gives, which he leaves
	// to them (masteryOwn, archivistmastery.go)
	ranks []*entity.CMRank
}

// archived is the archive, read again when it is older than archiveTTL.
// Called with book held.
func (a *Archivist) archived() (*spellArchive, error) {
	if a.archive != nil && a.now().Sub(a.archiveAt) <= archiveTTL {
		return a.archive, nil
	}
	arch, err := loadSpellArchive(a.spells)
	if err != nil {
		return nil, err
	}
	a.archive, a.archiveAt = arch, a.now()
	return arch, nil
}

// loadSpellArchive reads the archive.
func loadSpellArchive(s *database.SpellbookStore) (*spellArchive, error) {
	var a spellArchive
	var err error
	if a.scrolls, err = s.MerchantScrolls(scrollIcon); err != nil {
		return nil, fmt.Errorf("the merchants' scrolls: %w", err)
	}
	a.sold = make(map[string]int, len(a.scrolls))
	for _, sc := range a.scrolls {
		a.sold[spellKey(sc.Name)] |= sc.ClassUse
	}
	if a.scrollRecs, err = s.ScrollRecords(scrollIcon); err != nil {
		return nil, fmt.Errorf("the scrolls: %w", err)
	}
	names, err := s.SpellNames()
	if err != nil {
		return nil, fmt.Errorf("the spells: %w", err)
	}
	a.spells = newSpellIndex(names)
	a.names = make(map[int]database.SpellName, len(names))
	for _, n := range names {
		a.names[n.SpellID] = n
	}
	if a.descs, err = s.SpellDescriptions(); err != nil {
		return nil, fmt.Errorf("the spells' descriptions: %w", err)
	}
	if a.sources, err = s.ClassSpellSources(); err != nil {
		return nil, fmt.Errorf("classspells: %w", err)
	}
	ranks, err := s.MasteryRanks()
	if err != nil {
		return nil, fmt.Errorf("cmcatalog: %w", err)
	}
	for _, r := range ranks {
		if r.Enabled {
			a.ranks = append(a.ranks, entity.ParseCMRank(r))
		}
	}
	return &a, nil
}

// basic is the spells he grants a character of the class and race at the
// level (the petition's "all the spells of the player's class that are vendor
// bought, from level 1 to their current level (cap 60), not master class or
// mastery spells"), in level, then name order, one a name:
//
//   - of every scroll a merchant sells, those the character could scribe at
//     the level capped at 60 (class, race and level, as the scribe checks
//     them: scrollRefusal);
//   - each scroll's spell the record the scribe would write for the class
//     (scrollSpellFor), of a level no higher;
//   - but a were's (IsWereAbility: a were quest's or a mastery's), a spell
//     the class's classspells row has as a mastery's, an epic's or a master
//     class's (Granted), and, where the class has no row of it, a spell any
//     class has so: merchants sell master classes' and masteries' scrolls
//     too (the magician's Djinni Bolt and Summon Djinn, the rogue's Silent
//     Blade Burglar and Remove Mask, the monk's Freezing Strike).
//
// It is not every classspells row a level gives, which #template grants: the
// class quests give spells no merchant sells (Kick, Taunt, Rapid Strike,
// Spacious Mind), and those stay the quests'.
func (a *spellArchive) basic(class, race, level int) []database.SpellName {
	level = min(level, archivistLevelCap)
	has := func(id int) bool { _, ok := a.sources[id][class]; return ok }
	seen := map[int]bool{}
	var out []database.SpellName
	for _, sc := range a.scrolls {
		if scrollRefusal(sc.ClassUse, sc.RaceUse, sc.LevelReq, class, race, level) != "" {
			continue
		}
		s, ok := a.spells.scrollSpell(sc.Name, sc.LevelReq, has)
		if !ok || s.Level > level || seen[s.SpellID] {
			continue
		}
		seen[s.SpellID] = true
		if database.IsWereAbility(s.Name, a.descs[s.SpellID]) || !grantedTo(a.sources[s.SpellID], class) {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Level != out[j].Level {
			return out[i].Level < out[j].Level
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].SpellID < out[j].SpellID
	})
	return lacking(nil, out)
}

// grantedTo is whether a level gives a class a spell of these classspells rows
// (class -> source): its own row's word when it has one (Granted), else that
// no class has the spell as a mastery's, an epic's or a master class's.
func grantedTo(rows map[int]string, class int) bool {
	if source, ok := rows[class]; ok {
		return database.Granted(source)
	}
	for _, source := range rows {
		if !database.Granted(source) {
			return false
		}
	}
	return true
}
