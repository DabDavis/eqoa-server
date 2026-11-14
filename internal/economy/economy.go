// Package economy counts every movement of tunar where it happens (the owner,
// 2026-11-14: "lets add an economy tracker for the gm panel shows tunar
// accumulated from loot, selling, etc. and trend make it informative").
//
// A movement is a faucet (tunar made: a corpse's coins, a merchant paying for
// an item, a quest's reward, a GM's grant), a sink (tunar gone: a merchant's
// price, a repair, a waysender's fare, the auction house's fee and its own
// standing lots) or a transfer (tunar from one character to another: a trade,
// an auction between players, a duel's wager), which neither makes nor ends
// any. Each is a counter added under one mutex, by the hour and source and by
// the day and character, nothing else on the game's path; Run writes them to
// the database once a minute (database/economy.go) and once an hour measures
// what everyone holds. The GM panel's Economy page reads the tables
// (gmweb/ops_economy.go).
package economy

import (
	"log"
	"sync"
	"time"

	"github.com/eqoa/server/internal/database"
)

// Kind is what a movement does to the tunar there is.
type Kind string

const (
	Faucet   Kind = "faucet"
	Sink     Kind = "sink"
	Transfer Kind = "transfer"
)

// Source is where a movement comes from.
type Source string

// The sources, each at its one place in the code.
const (
	Loot          Source = "loot"           // a corpse's coins, the opener's part (handlers/corpsecoins.go)
	LootSplit     Source = "loot_split"     // a group member's share of them
	Pickpocket    Source = "pickpocket"     // a rogue's pick of a creature's pockets (entity/workbookmore.go)
	MerchantSell  Source = "merchant_sell"  // an item sold to a merchant (handlers/merchant.go)
	QuestReward   Source = "quest_reward"   // a quest step's tunar (handlers/quests.go)
	QuestRefund   Source = "quest_refund"   // a step's tunar handed back when its encounter is put back
	StartingPurse Source = "starting_purse" // a new character's 5,000 (entity/tunar.go)
	GMGive        Source = "gm_give"        // #tunar, #banktunar, #template, the panel's edits, upward
	MerchantBuy   Source = "merchant_buy"   // an item bought from a merchant
	BuyBack       Source = "buyback"        // an item sold or destroyed bought back (handlers/buyback.go)
	Repair        Source = "repair"         // a blacksmith's mending (handlers/blacksmith.go)
	QuestPayment  Source = "quest_payment"  // a give or show step's tunar (a portal guardian's fare)
	Travel        Source = "travel"         // a waysender's fare (handlers/interact.go)
	Cure          Source = "cure"           // a were fever's cure at the campfire (handlers/wereinfect.go)
	AuctionFee    Source = "auction_fee"    // the auction house's listing fee (handlers/auction.go)
	AuctionHouse  Source = "auction_house"  // a buy from the house's own standing lots (no seller)
	GMTake        Source = "gm_take"        // the GMs' edits, downward
	Trade         Source = "trade"          // tunar in a trade window
	AuctionSale   Source = "auction_sale"   // an auction won: the price, the winner's to the seller's bank
	DuelWager     Source = "duel_wager"     // a duel's wager: the loser's stake to the winner
)

// Info says what a source is.
type Info struct {
	Source Source `json:"source"`
	Kind   Kind   `json:"kind"`
	Label  string `json:"label"`
	Help   string `json:"help"`
	// Phrase is the source in a sentence ("80% of it from corpse coins").
	Phrase string `json:"phrase"`
}

// Sources are every source, faucets, then sinks, then transfers.
var Sources = []Info{
	{Loot, Faucet, "Corpse coins", "a creature's corpse's tunar, the share of the one who opened it", "corpse coins"},
	{LootSplit, Faucet, "Corpse coins, group shares", "the other group members' equal shares of a corpse's tunar", "corpse coins' group shares"},
	{Pickpocket, Faucet, "Pickpocketing", "a rogue's pick of a creature's pockets", "pickpocketing"},
	{MerchantSell, Faucet, "Sold to merchants", "items sold to an NPC merchant", "items sold to merchants"},
	{QuestReward, Faucet, "Quest rewards", "a quest step's tunar reward", "quest rewards"},
	{QuestRefund, Faucet, "Quest hand-backs", "a step's tunar handed back when its encounter is put back", "quest hand-backs"},
	{StartingPurse, Faucet, "New characters' purses", "the 5,000 tunar a character's first login starts with", "new characters' purses"},
	{GMGive, Faucet, "GM grants", "#tunar, #banktunar and #template raising a purse or a bank, and the panel's edits upward", "GM grants"},
	{MerchantBuy, Sink, "Bought from merchants", "items bought from an NPC merchant", "merchant purchases"},
	{BuyBack, Sink, "Bought back", "items sold or destroyed, bought back at a merchant", "buy-backs"},
	{Repair, Sink, "Repairs", "a blacksmith mending an item", "repairs"},
	{QuestPayment, Sink, "Quest payments", "a quest step's tunar handed in (a portal guardian's fare)", "quest payments"},
	{Travel, Sink, "Waysender fares", "a waysender's fee for a trip", "waysender fares"},
	{Cure, Sink, "Fever cures", "a were fever drawn out at the campfire", "fever cures"},
	{AuctionFee, Sink, "Auction listing fees", "the auction house's fee for putting a lot up", "auction listing fees"},
	{AuctionHouse, Sink, "Auction house's own lots", "buys from the house's standing lots, which have no seller", "the auction house's own lots"},
	{GMTake, Sink, "GM removals", "#tunar, #banktunar and the panel's edits lowering a purse or a bank", "GM removals"},
	{Trade, Transfer, "Trades", "tunar given in a trade window", "trades"},
	{AuctionSale, Transfer, "Auction sales", "an auction won between players: its price, from the winner to the seller", "auction sales"},
	{DuelWager, Transfer, "Duel wagers", "a duel's lost stake, paid to its winner", "duel wagers"},
}

var kinds = func() map[Source]Kind {
	m := map[Source]Kind{}
	for _, i := range Sources {
		m[i.Source] = i.Kind
	}
	return m
}()

// KindOf is a source's kind ("" for one not listed).
func KindOf(s Source) Kind { return kinds[s] }

type hourKey struct {
	hour int64
	src  Source
}

type charKey struct {
	day int64
	id  uint32
	src Source
}

type tally struct{ count, amount int64 }

type charTally struct {
	name             string
	count, got, paid int64
}

// Ledger is the counts not yet written.
type Ledger struct {
	mu    sync.Mutex
	hours map[hourKey]tally
	chars map[charKey]charTally
	// started: characters whose starting purse was counted by this process
	// (a character whose row has never stored any tunar is given it at each
	// login until a change of its purse stores one)
	started map[uint32]bool
	now     func() time.Time
	held    bool
}

// New is an empty ledger.
func New() *Ledger {
	return &Ledger{hours: map[hourKey]tally{}, chars: map[charKey]charTally{}, started: map[uint32]bool{}, now: time.Now}
}

// Default is the server's ledger: the game's sources count into it, Run writes it.
var Default = New()

func hourOf(t time.Time) int64 { return t.Unix() / 3600 * 3600 }
func dayOf(t time.Time) int64  { return t.Unix() / 86400 * 86400 }

// add counts one movement of n (n > 0) and the characters' sides (id 0: none).
func (l *Ledger) add(src Source, n int, fromID uint32, fromName string, toID uint32, toName string) {
	if n <= 0 || l == nil {
		return
	}
	l.mu.Lock()
	now := l.now()
	hk := hourKey{hourOf(now), src}
	t := l.hours[hk]
	t.count++
	t.amount += int64(n)
	l.hours[hk] = t
	day := dayOf(now)
	if toID != 0 {
		ck := charKey{day, toID, src}
		c := l.chars[ck]
		c.count++
		c.got += int64(n)
		if toName != "" {
			c.name = toName
		}
		l.chars[ck] = c
	}
	if fromID != 0 {
		ck := charKey{day, fromID, src}
		c := l.chars[ck]
		c.count++
		c.paid += int64(n)
		if fromName != "" {
			c.name = fromName
		}
		l.chars[ck] = c
	}
	l.mu.Unlock()
}

// Made counts n tunar made for a character (a faucet).
func (l *Ledger) Made(src Source, id uint32, name string, n int) { l.add(src, n, 0, "", id, name) }

// Spent counts n tunar a character spent to nobody (a sink).
func (l *Ledger) Spent(src Source, id uint32, name string, n int) { l.add(src, n, id, name, 0, "") }

// Moved counts n tunar from one character to another (a transfer).
func (l *Ledger) Moved(src Source, fromID uint32, fromName string, toID uint32, toName string, n int) {
	l.add(src, n, fromID, fromName, toID, toName)
}

// Made, Spent, Moved and GMChange count into Default.
func Made(src Source, id uint32, name string, n int)  { Default.Made(src, id, name, n) }
func Spent(src Source, id uint32, name string, n int) { Default.Spent(src, id, name, n) }
func Moved(src Source, fromID uint32, fromName string, toID uint32, toName string, n int) {
	Default.Moved(src, fromID, fromName, toID, toName, n)
}

// GMChange counts a GM's change of a character's tunar by delta: a grant
// upward, a removal downward.
func GMChange(id uint32, name string, delta int) { Default.GMChange(id, name, delta) }

// GMChange counts a GM's change into l.
func (l *Ledger) GMChange(id uint32, name string, delta int) {
	if delta > 0 {
		l.Made(GMGive, id, name, delta)
	} else if delta < 0 {
		l.Spent(GMTake, id, name, -delta)
	}
}

// StartedPurse counts a character's starting purse once in this process.
func StartedPurse(id uint32, name string, n int) {
	l := Default
	l.mu.Lock()
	seen := l.started[id]
	l.started[id] = true
	l.mu.Unlock()
	if !seen {
		l.Made(StartingPurse, id, name, n)
	}
}

// Pending is the counts not yet written, for a test or the panel's "now".
func (l *Ledger) Pending() ([]database.TunarHourRow, []database.TunarCharRow) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.rowsLocked()
}

func (l *Ledger) rowsLocked() ([]database.TunarHourRow, []database.TunarCharRow) {
	hours := make([]database.TunarHourRow, 0, len(l.hours))
	for k, t := range l.hours {
		hours = append(hours, database.TunarHourRow{Hour: k.hour, Source: string(k.src), Kind: string(KindOf(k.src)),
			Count: t.count, Amount: t.amount})
	}
	chars := make([]database.TunarCharRow, 0, len(l.chars))
	for k, c := range l.chars {
		chars = append(chars, database.TunarCharRow{Day: k.day, ServerID: k.id, Source: string(k.src), Name: c.name,
			Count: c.count, Got: c.got, Paid: c.paid})
	}
	return hours, chars
}

// Flush writes the counts to db and empties them; on a failure they are kept
// for the next (added to what comes meanwhile). A held ledger writes nothing.
func (l *Ledger) Flush(db *database.SQLiteDB) error {
	if l == nil || db == nil {
		return nil
	}
	l.mu.Lock()
	if l.held || (len(l.hours) == 0 && len(l.chars) == 0) {
		l.mu.Unlock()
		return nil
	}
	hours, chars := l.rowsLocked()
	l.hours, l.chars = map[hourKey]tally{}, map[charKey]charTally{}
	l.mu.Unlock()
	if err := db.FlushTunarLedger(hours, chars); err != nil {
		l.mu.Lock()
		for _, r := range hours {
			k := hourKey{r.Hour, Source(r.Source)}
			t := l.hours[k]
			t.count += r.Count
			t.amount += r.Amount
			l.hours[k] = t
		}
		for _, r := range chars {
			k := charKey{r.Day, r.ServerID, Source(r.Source)}
			c := l.chars[k]
			c.count += r.Count
			c.got += r.Got
			c.paid += r.Paid
			if c.name == "" {
				c.name = r.Name
			}
			l.chars[k] = c
		}
		l.mu.Unlock()
		return err
	}
	return nil
}

// Hold stops every write until Hold(false): nothing of this process writes
// the database while a restart that keeps the sessions holds it (a swap may
// put the file back from a copy). The counts go on in memory.
func (l *Ledger) Hold(on bool) {
	l.mu.Lock()
	l.held = on
	l.mu.Unlock()
}

// Measure keeps the wealth snapshot of the hour now, unless one is kept.
func (l *Ledger) Measure(db *database.SQLiteDB, staffLevel int, force bool) error {
	l.mu.Lock()
	held, now := l.held, l.now()
	l.mu.Unlock()
	if held || db == nil {
		return nil
	}
	hour := hourOf(now)
	if !force && db.HasTunarWealth(hour) {
		return nil
	}
	w, err := database.MeasureTunarWealth(db, hour, staffLevel)
	if err != nil {
		return err
	}
	return db.SaveTunarWealth(w)
}

// Run writes Default to db every `every` (a minute when 0) and measures the
// wealth once an hour; stop writes what is left and ends it.
func Run(db *database.SQLiteDB, every time.Duration, staffLevel int) (stop func()) {
	if every <= 0 {
		every = time.Minute
	}
	l := Default
	done := make(chan struct{})
	ended := make(chan struct{})
	var once sync.Once
	go func() {
		defer close(ended)
		t := time.NewTicker(every)
		defer t.Stop()
		failing := false
		for {
			if err := l.Measure(db, staffLevel, false); err != nil {
				log.Printf("Economy: the wealth not measured: %v", err)
			}
			select {
			case <-done:
				return
			case <-t.C:
			}
			err := l.Flush(db)
			switch {
			case err != nil && !failing:
				failing = true
				log.Printf("Economy: the ledger not written (%v): kept for the next", err)
			case err == nil && failing:
				failing = false
				log.Printf("Economy: the ledger written again")
			}
		}
	}()
	return func() {
		once.Do(func() {
			close(done)
			<-ended
			if err := l.Flush(db); err != nil {
				log.Printf("Economy: the ledger's last counts not written: %v", err)
			}
		})
	}
}
