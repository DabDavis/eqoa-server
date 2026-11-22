package entity

import (
	"fmt"
	"log"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// placetexts.go — the live game's place texts (work board 773 step 41 and 792
// step 47: Highbourne's library hall, its cartographers' room and its dock
// quarter): a box with one answer, Done (CLIENT's question box, 0x0034, as a
// quest event's popup), opened once each time a player walks into an area
// (table placetexts, database/places.go), never again while it stays inside.
// placeTextSystem looks at every living player twice a second (as the hazards
// do); not while it is dead or loading a world. The first look at a player
// after it joins the world (a login, a keep restart's restore) only notes
// where it stands, so one that logs in inside an area is not told: it did not
// walk in. What a player is inside is memory only, which a keep restart drops
// and that first look takes again.

// placeTextEvery is how often, in ticks, placeTextSystem looks (0.5 s).
const placeTextEvery = 5

// placeTextBoxMark marks a place text's box id (0x0035 echoes it; its answer
// changes nothing).
const placeTextBoxMark = 0x7B000000

// IsPlaceTextBox reports a place text's box id.
func IsPlaceTextBox(id uint32) bool { return id&0xFF000000 == placeTextBoxMark }

// PlaceTextBoxMark is the mark itself, for the handlers' test that keeps every feature's mark its own.
func PlaceTextBoxMark() uint32 { return placeTextBoxMark }

// placeTextState is the areas by world.
type placeTextState struct {
	byWorld map[uint8][]database.PlaceText
	n       int
}

// SetPlaceTexts sets the areas (table placetexts).
func (w *World) SetPlaceTexts(ps []database.PlaceText) {
	by := make(map[uint8][]database.PlaceText)
	for _, p := range ps {
		by[p.World] = append(by[p.World], p)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.placeTexts = placeTextState{byWorld: by, n: len(ps)}
}

// LoadPlaceTexts reads table placetexts into the world: at the start and after
// a GM's edit. Call it without the database held. A row the game cannot show is
// logged and left out. It reports how many it read.
func (w *World) LoadPlaceTexts(db *database.SQLiteDB) (int, error) {
	ps, bad, err := db.PlaceTexts()
	if err != nil {
		return 0, fmt.Errorf("placetexts: %w", err)
	}
	for _, b := range bad {
		log.Printf("WARNING: placetexts %s: left out", b)
	}
	w.SetPlaceTexts(ps)
	return len(ps), nil
}

// placeTextSystem opens an area's text for each player that walked into it
// (the file's doc). Must be called with the lock held.
func (w *World) placeTextSystem() {
	if w.TickCount%placeTextEvery != 0 || w.placeTexts.n == 0 {
		return
	}
	for _, p := range w.Players {
		areas := w.placeTexts.byWorld[p.World]
		if p.HP <= 0 || p.respawnPending() {
			continue
		}
		sess := w.findSessionByEntityIDLocked(p.ServerID)
		if sess == nil || sess.StateChannelsHeld() {
			continue // loading a world: where it stands is not yet where it is
		}
		first := !p.placeSeen
		p.placeSeen = true
		for _, a := range areas {
			in := a.Holds(p.X, p.Y, p.Z)
			was := p.placeIn[a.ID]
			switch {
			case in && !was:
				if p.placeIn == nil {
					p.placeIn = make(map[int]bool)
				}
				p.placeIn[a.ID] = true
				if !first {
					w.sendPlaceTextLocked(sess, a)
					log.Printf("PlaceText: %s walked into %d", p.Name, a.ID)
				}
			case !in && was:
				delete(p.placeIn, a.ID)
			}
		}
		// an area of another world (a trip) is left
		for id := range p.placeIn {
			if !placeTextIn(areas, id) {
				delete(p.placeIn, id)
			}
		}
	}
}

func placeTextIn(areas []database.PlaceText, id int) bool {
	for _, a := range areas {
		if a.ID == id {
			return true
		}
	}
	return false
}

// sendPlaceTextLocked opens a's box for sess.
func (w *World) sendPlaceTextLocked(sess *session.Session, a database.PlaceText) {
	w.sendDuelBoxLocked(sess, placeTextBoxMark|uint32(a.ID)&0xFFFFFF, a.Text, []string{a.Answer})
}
