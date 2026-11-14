// Package formulamode is which damage a shard lands, switched by its GMs (the
// owner, 2026-11-14: "give GMs /formula jibs heirloom/live, /formula default
// heirloom/live so we can switch between the two"): its operator's formula
// file applied (live), the built-in figures (off), or the built-in with the
// formula's figure recorded beside it (shadow; entity/dmgshadow.go).
//
// The mode is a row of the shard's own database (database/formulamode.go):
// the config's combat.formula_mode seeds it, a GM's switch overwrites it, and
// a restart keeps it. A switch takes effect at once on the shard it is made
// on. Both shards' servers run on one machine, so a GM may switch another
// shard too (combat.formula_peers names its database): the row is written
// there, with a line of its inbox, and its server (Watch) applies it within
// FormulaPollEvery and tells its GMs. Each server writes what it has loaded
// beside its row, so a switch to live or shadow on a shard with no formula
// loaded is refused, on either side.
package formulamode

import (
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// FormulaPollEvery is how often a server reads its row and inbox.
const FormulaPollEvery = 3 * time.Second

// Control is a shard's switch.
type Control struct {
	db    *database.SQLiteDB
	w     *entity.World
	shard string            // this shard's name, lower case ("heirloom")
	peers map[string]string // another shard's name -> its database file

	mu        sync.Mutex
	tell      func(line string) // to this shard's GMs in the world
	published struct {
		loaded int
		shadow bool
		ok     bool
	}
}

// New makes the switch of shard (its name in commands: "heirloom") on db and
// w, seeding the row with the config's mode, and applies the row's mode. db
// may be nil (no persistence: the config's mode alone).
func New(db *database.SQLiteDB, w *entity.World, shard string, peers map[string]string, seed entity.FormulaMode) (*Control, error) {
	c := &Control{db: db, w: w, shard: strings.ToLower(strings.TrimSpace(shard)), peers: map[string]string{}}
	for n, p := range peers {
		if n = strings.ToLower(strings.TrimSpace(n)); n != "" && n != c.shard {
			c.peers[n] = strings.TrimSpace(p)
		}
	}
	if db == nil {
		w.SetDamageFormulaMode(seed)
		return c, nil
	}
	row, err := db.SeedFormulaMode(seed.String())
	if err != nil {
		w.SetDamageFormulaMode(seed)
		return c, err
	}
	m, err := entity.ParseFormulaMode(row.Mode)
	if err != nil {
		m = entity.FormulaOff
	}
	w.SetDamageFormulaMode(m)
	log.Printf("Damage formulas: mode %s on %s (set by %s at %s)", m, c.label(c.shard), row.By, row.At)
	c.publish()
	return c, nil
}

// SetTell gives the switch its line to this shard's GMs (handlers.TellGMs).
func (c *Control) SetTell(fn func(line string)) {
	c.mu.Lock()
	c.tell = fn
	c.mu.Unlock()
}

func (c *Control) say(line string) {
	c.mu.Lock()
	fn := c.tell
	c.mu.Unlock()
	if fn != nil {
		fn(line)
	}
}

// Shard is this shard's name.
func (c *Control) Shard() string { return c.shard }

// Shards is every shard the switch reaches: this one first, then the others
// by name.
func (c *Control) Shards() []string {
	out := []string{c.shard}
	var ps []string
	for n := range c.peers {
		ps = append(ps, n)
	}
	sort.Strings(ps)
	return append(out, ps...)
}

func (c *Control) label(shard string) string {
	if shard == "" {
		return "this shard"
	}
	return strings.ToUpper(shard[:1]) + shard[1:]
}

// Status is a shard's mode as the switch sees it.
type Status struct {
	Shard     string `json:"shard"`
	Here      bool   `json:"here"`
	Mode      string `json:"mode"`
	By        string `json:"by,omitempty"`
	At        string `json:"at,omitempty"`
	Loaded    int    `json:"formulas_loaded"`
	ShadowLog bool   `json:"shadow_log"`
	SeenAt    string `json:"seen_at,omitempty"` // when its server last wrote what it has loaded
	Err       string `json:"error,omitempty"`
}

// Status is every shard's mode.
func (c *Control) Status() []Status {
	var out []Status
	for _, sh := range c.Shards() {
		out = append(out, c.status(sh))
	}
	return out
}

func (c *Control) status(shard string) Status {
	st := Status{Shard: shard, Here: shard == c.shard}
	if st.Here {
		_, names := c.w.DamageFormulaStatus()
		path, _, _ := c.w.DamageFormulaShadowLog()
		st.Mode, st.Loaded, st.ShadowLog = c.w.DamageFormulaMode().String(), len(names), path != ""
		if c.db != nil {
			if row, err := c.db.FormulaSetting(); err == nil && row.Exists {
				st.By, st.At = row.By, row.At
			}
		}
		return st
	}
	peer, err := c.openPeer(shard)
	if err != nil {
		st.Err = err.Error()
		return st
	}
	defer peer.Close()
	row, err := peer.FormulaSetting()
	switch {
	case err != nil:
		st.Err = err.Error()
	case !row.Exists:
		st.Mode, st.Err = "off", "its server has not reported (not running this build yet?)"
	default:
		st.Mode, st.By, st.At, st.Loaded, st.ShadowLog, st.SeenAt = row.Mode, row.By, row.At, row.Loaded, row.ShadowLog, row.SeenAt
	}
	return st
}

// ErrUnknownShard is a switch of a shard the switch does not reach.
var ErrUnknownShard = errors.New("unknown shard")

func (c *Control) openPeer(shard string) (*database.SQLiteDB, error) {
	path, ok := c.peers[shard]
	if !ok {
		return nil, fmt.Errorf("%w %q (this server reaches %s)", ErrUnknownShard, shard, strings.Join(c.Shards(), ", "))
	}
	if path == "" {
		return nil, fmt.Errorf("%s's database is not configured here (combat.formula_peers)", c.label(shard))
	}
	if c.db != nil && c.db.SameFile(path) {
		return nil, fmt.Errorf("combat.formula_peers names this server's own database for %s", c.label(shard))
	}
	return database.OpenFormulaPeer(path)
}

// ModeWord is a mode as the commands say it: "jibs" for live, "default" for
// off, "shadow".
func ModeWord(m string) string {
	switch m {
	case "live":
		return "jibs (the formula file applied)"
	case "shadow":
		return "shadow (built-in applied, the formula recorded beside it)"
	}
	return "default (built-in damage)"
}

// Set switches shard ("" for this one) to mode, as who (via says where:
// "game", "panel"). It returns the line to tell the GM, or why not.
func (c *Control) Set(shard string, m entity.FormulaMode, who database.GMEdit) (string, error) {
	shard = strings.ToLower(strings.TrimSpace(shard))
	if shard == "" {
		shard = c.shard
	}
	if c.db == nil {
		return "", errors.New("this server has no database to keep the mode in")
	}
	if who.Username == "" {
		var u string
		if err := c.db.QueryRow(`SELECT username FROM accountinfo WHERE accountid = ?`, who.AccountID).Scan(&u); err == nil {
			who.Username = u
		}
	}
	name := c.label(shard)
	detail := fmt.Sprintf("damage formula mode of %s set to %s (%s, via %s)", name, m, ModeWord(m.String()), who.Via)
	if shard == c.shard {
		if m != entity.FormulaOff {
			if _, names := c.w.DamageFormulaStatus(); len(names) == 0 {
				return "", fmt.Errorf("no formula loaded on %s (combat.formula_file)", name)
			}
		}
		if m == entity.FormulaShadow {
			if path, _, _ := c.w.DamageFormulaShadowLog(); path == "" {
				return "", fmt.Errorf("no shadow log configured on %s (combat.formula_shadow_log)", name)
			}
		}
		if err := c.db.SetFormulaModeAudited(m.String(), who, detail, ""); err != nil {
			return "", err
		}
		c.w.SetDamageFormulaMode(m)
		line := fmt.Sprintf("Damage on %s is now %s (by %s).", name, ModeWord(m.String()), who.Username)
		c.say("[Formula] " + line)
		return line, nil
	}
	peer, err := c.openPeer(shard)
	if err != nil {
		return "", err
	}
	defer peer.Close()
	row, err := peer.FormulaSetting()
	if err != nil {
		return "", err
	}
	if m != entity.FormulaOff {
		if !row.Exists {
			return "", fmt.Errorf("%s's server has not reported its formulas (not running this build yet?)", name)
		}
		if row.Loaded == 0 {
			return "", fmt.Errorf("no formula loaded on %s (its combat.formula_file)", name)
		}
		if m == entity.FormulaShadow && !row.ShadowLog {
			return "", fmt.Errorf("no shadow log configured on %s (its combat.formula_shadow_log)", name)
		}
	}
	if err := peer.SetFormulaModeAudited(m.String(), who, detail+" from "+c.label(c.shard), c.shard); err != nil {
		return "", err
	}
	// this shard's audit has the act too
	if _, err := c.db.AuditGMLive(who, detail); err != nil {
		log.Printf("Damage formulas: the audit of %s's switch here: %v", name, err)
	}
	line := fmt.Sprintf("Damage on %s is now %s (by %s); its server takes it up within %d seconds.", name, ModeWord(m.String()),
		who.Username, int(FormulaPollEvery/time.Second))
	c.say("[Formula] " + line)
	return line, nil
}

// Poll applies what another shard's GM set here (the inbox, then the row),
// telling this shard's GMs, and writes what this server has loaded when it
// changed.
func (c *Control) Poll() {
	if c.db == nil {
		return
	}
	notes, err := c.db.TakeFormulaModeInbox()
	if err != nil {
		log.Printf("Damage formulas: the mode inbox: %v", err)
	}
	row, err := c.db.FormulaSetting()
	if err != nil {
		log.Printf("Damage formulas: the mode row: %v", err)
	} else if row.Exists {
		if m, err := entity.ParseFormulaMode(row.Mode); err == nil && m != c.w.DamageFormulaMode() {
			c.w.SetDamageFormulaMode(m)
		}
	}
	for _, n := range notes {
		c.say(fmt.Sprintf("[Formula] Damage on %s is now %s (by %s on %s).", c.label(c.shard), ModeWord(n.Mode), n.By, c.label(n.Shard)))
	}
	c.publish()
}

// publish writes what this server has loaded, when it changed.
func (c *Control) publish() {
	_, names := c.w.DamageFormulaStatus()
	path, _, _ := c.w.DamageFormulaShadowLog()
	c.mu.Lock()
	same := c.published.ok && c.published.loaded == len(names) && c.published.shadow == (path != "")
	c.mu.Unlock()
	if same {
		return
	}
	if err := c.db.SetFormulaLoaded(len(names), path != ""); err != nil {
		log.Printf("Damage formulas: what is loaded not written: %v", err)
		return
	}
	c.mu.Lock()
	c.published.loaded, c.published.shadow, c.published.ok = len(names), path != "", true
	c.mu.Unlock()
}

// Watch polls every FormulaPollEvery until stop closes (nil: for ever).
func (c *Control) Watch(stop <-chan struct{}) {
	t := time.NewTicker(FormulaPollEvery)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			c.Poll()
		}
	}
}
