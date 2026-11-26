package handlers

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// bestiary.go — each character's bestiary (Heirloom petition 3192, Packmaster: "some
// type of bestiary that shows mobs you encounter and the zones they are from").
// entity/bestiary.go decides what counts (a creature that took the player, or its pet,
// into its hate; a kill credited), database/bestiary.go keeps it (table charbestiary),
// and this file writes the notes, names the zones and shows it:
//
//   - offered with protocol.OpcodeBestiary (0x7EF4) at the world entry, on a server
//     that keeps one (a database), to a session the quality-of-life additions are on
//     for (QoLOnFor: not world.vanilla_ui, not an [OG] character); a PS2 ignores it;
//   - a client that shows one (vanilla's Character > Bestiary) asks for a page with
//     OpcodeBestiaryData (0x7EF5, a u16) as its window opens and at each turn, and is
//     sent that page: the kinds by zone, then by name, each with its kills and when it
//     was first met, a zone's heading over its kinds (bestiaryPages).
//
// The zones are the map's (bestiaryZone): in Tunaria and the disc's other worlds the
// zone EQOA-Data's map names for the camps of the creature's 2000-unit cell (the
// zone grid's cells: weather.ZoneAt), else the nearest camp's within reach, else a
// town or the world as the Hardcore memorial names a place (fallPlace); a heroic
// dungeon's creature is its dungeon's (entity/bestiary.go). A creature is filed under
// the zone it spawns in.

// bestiaryStore is the database's half.
type bestiaryStore interface {
	RecordBestiary([]database.BestiaryNote) error
	BestiaryOf(serverID uint32) ([]database.BestiaryEntry, error)
}

// BestiaryHandler writes the bestiary's notes and answers its pages.
type BestiaryHandler struct {
	world *entity.World
	db    bestiaryStore
	now   func() time.Time

	askMu sync.Mutex
	asked map[uint32]time.Time
}

// bestiaryAskEvery is how often a client's asks are answered (a page turned held).
const bestiaryAskEvery = 250 * time.Millisecond

// bestiaryPageLines is how many rows a page holds, the headings among them: what the
// window shows at once.
const bestiaryPageLines = 14

// bestiaries is the handler a server runs (nil without a database).
var bestiaries atomic.Pointer[BestiaryHandler]

// NewBestiaryHandler creates the bestiary's handler.
func NewBestiaryHandler(w *entity.World, db bestiaryStore) *BestiaryHandler {
	return &BestiaryHandler{world: w, db: db, now: time.Now, asked: map[uint32]time.Time{}}
}

// RegisterHandlers registers the bestiary's opcodes and has the world's notes written here.
func (h *BestiaryHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeBestiary, h.HandleAnswer)
	r.Register(protocol.OpcodeBestiaryData, h.HandleAsk)
	if h.world == nil || h.db == nil {
		return
	}
	bestiaries.Store(h)
	h.world.SetBestiaryZoner(bestiaryZone)
	h.world.OnBestiary(h.write)
}

// write is the world's hook: a tick's notes in one transaction.
func (h *BestiaryHandler) write(notes []database.BestiaryNote) {
	if err := h.db.RecordBestiary(notes); err != nil {
		log.Printf("Bestiary: %d notes not written: %v", len(notes), err)
	}
}

// buildBestiaryOffer is OpcodeBestiary from the server: a u32, 1.
func buildBestiaryOffer() []byte {
	return binary.LittleEndian.AppendUint32(binary.LittleEndian.AppendUint16(nil, uint16(protocol.OpcodeBestiary)), 1)
}

// SendBestiaryOffer offers the session's client the bestiary, on a server that keeps
// one, where the quality-of-life additions are on for it.
func SendBestiaryOffer(sess *session.Session) {
	if bestiaries.Load() == nil || !QoLOnFor(sess) {
		return
	}
	queueReliable(sess, protocol.OpcodeBestiary, buildBestiaryOffer())
}

// HandleAnswer is the client's answer to the offer (a u32): logged, nothing kept, as
// its asks say what it wants.
func (h *BestiaryHandler) HandleAnswer(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) >= 4 {
		log.Printf("Bestiary: %s answers %d", sess.RemoteAddr(), binary.LittleEndian.Uint32(data[:4]))
	}
	return nil
}

// HandleAsk is a client's 0x7EF5: a page of its bestiary, sent now.
func (h *BestiaryHandler) HandleAsk(sess *session.Session, msg *protocol.Message) error {
	if h.world == nil || h.db == nil || !QoLOnFor(sess) {
		return nil
	}
	p := h.world.SessionPlayer(sess)
	if p == nil {
		return nil
	}
	page := 0
	if data := msg.Data(); len(data) >= 2 {
		page = int(binary.LittleEndian.Uint16(data[:2]))
	}
	now := h.now()
	h.askMu.Lock()
	last, seen := h.asked[p.CharServerID]
	if seen && now.Sub(last) >= 0 && now.Sub(last) < bestiaryAskEvery {
		h.askMu.Unlock()
		return nil
	}
	h.asked[p.CharServerID] = now
	h.askMu.Unlock()
	h.world.FlushBestiary() // what this tick met is in it
	entries, err := h.db.BestiaryOf(p.CharServerID)
	if err != nil {
		log.Printf("Bestiary: %s's: %v", p.Name, err)
		return nil
	}
	pages := bestiaryPages(entries, now)
	if page >= len(pages) {
		page = len(pages) - 1
	}
	queueReliable(sess, protocol.OpcodeBestiaryData, buildBestiaryData(page, len(pages), len(entries), pages[page]))
	log.Printf("Bestiary: %s's page %d of %d sent (%d kinds)", p.Name, page+1, len(pages), len(entries))
	return nil
}

// bestiaryValue is what a kind's row says: its kills and when it was first met.
func bestiaryValue(e database.BestiaryEntry, now time.Time) string {
	day := e.FirstSeen.Format("Jan 2")
	if e.FirstSeen.Year() != now.Year() {
		day = e.FirstSeen.Format("Jan 2, 2006")
	}
	if e.Kills > 0 {
		return fmt.Sprintf("%d killed, first %s", e.Kills, day)
	}
	return "met " + day
}

// bestiaryPages is the bestiary as its pages: each at most bestiaryPageLines rows, a
// zone's heading over its kinds, again at a page's top when its zone goes on. No
// kinds: one empty page.
func bestiaryPages(entries []database.BestiaryEntry, now time.Time) [][]recordRow {
	pages := [][]recordRow{nil}
	zone := ""
	for i, e := range entries {
		cur := pages[len(pages)-1]
		newZone := i == 0 || !strings.EqualFold(e.Zone, zone)
		need := 1
		if newZone || len(cur) == 0 {
			need = 2
		}
		if len(cur)+need > bestiaryPageLines {
			pages = append(pages, nil)
			cur = nil
		}
		if newZone || len(cur) == 0 {
			cur = append(cur, recordRow{style: recordHeading, label: e.Zone})
		}
		cur = append(cur, recordRow{style: recordLine, label: e.Kind, value: bestiaryValue(e, now)})
		pages[len(pages)-1] = cur
		zone = e.Zone
	}
	return pages
}

// buildBestiaryData is OpcodeBestiaryData from the server.
func buildBestiaryData(page, pages, kinds int, rows []recordRow) []byte {
	clamp := func(n int) uint16 { return uint16(min(max(n, 0), math.MaxUint16)) }
	b := binary.LittleEndian.AppendUint16(nil, uint16(protocol.OpcodeBestiaryData))
	b = append(b, 1)
	b = binary.LittleEndian.AppendUint16(b, clamp(page))
	b = binary.LittleEndian.AppendUint16(b, clamp(pages))
	b = binary.LittleEndian.AppendUint16(b, clamp(kinds))
	if len(rows) > 255 {
		rows = rows[:255]
	}
	b = append(b, byte(len(rows)))
	for _, r := range rows {
		b = append(b, r.style)
		b = recordString8(b, r.label)
		b = recordString8(b, r.value)
	}
	return b
}

// bestiaryCell keys a 2000-unit cell of a world (the zone grid's).
type bestiaryCell struct {
	world    uint8
	col, row int
}

func bestiaryCellAt(world uint8, x, z float32) bestiaryCell {
	return bestiaryCell{world, int(math.Floor(float64(x) / 2000)), int(math.Floor(float64(z) / 2000))}
}

// bestiaryNearCamp is how far a camp's zone names a place with none in its cell.
const bestiaryNearCamp = 2500

var (
	bestiaryZonesOnce sync.Once
	bestiaryZones     map[bestiaryCell]string
)

// bestiaryCellZones is each cell's zone: the one most of the map's camps in it name
// (ties to the first by name).
func bestiaryCellZones() map[bestiaryCell]string {
	bestiaryZonesOnce.Do(func() {
		votes := map[bestiaryCell]map[string]int{}
		for _, c := range mapCamps {
			if c.Zone == "" {
				continue
			}
			k := bestiaryCellAt(c.World, c.X, c.Z)
			if votes[k] == nil {
				votes[k] = map[string]int{}
			}
			votes[k][c.Zone]++
		}
		bestiaryZones = map[bestiaryCell]string{}
		for k, v := range votes {
			best, n := "", 0
			for z, c := range v {
				if c > n || c == n && z < best {
					best, n = z, c
				}
			}
			bestiaryZones[k] = best
		}
	})
	return bestiaryZones
}

// bestiaryZone names the zone of a place for the bestiary (the world's zone namer):
// its cell's camps' zone, else the nearest camp's within bestiaryNearCamp, else the
// place as fallPlace names it (a town, its wilds, a homeland's home, the world).
func bestiaryZone(world uint8, x, z float32) string {
	if world <= 5 && world != 4 {
		if n, ok := bestiaryCellZones()[bestiaryCellAt(world, x, z)]; ok {
			return n
		}
		best, bestD := "", math.Inf(1)
		for _, c := range mapCamps {
			if c.World != world || c.Zone == "" {
				continue
			}
			if d := math.Hypot(float64(c.X-x), float64(c.Z-z)); d < bestD || d == bestD && c.Zone < best {
				best, bestD = c.Zone, d
			}
		}
		if bestD <= bestiaryNearCamp {
			return best
		}
	}
	return fallPlace(world, x, 0, z)
}
