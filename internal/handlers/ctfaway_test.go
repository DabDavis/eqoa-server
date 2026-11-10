package handlers

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// A merc waits outside capture the flag's arena and comes back from its hire's row, as at a login, through the
// World's whole tick and its event (entity/ctfaway.go's MercNoteBack, merc.go's note): its owner in the arena's world,
// it is out of the world and /merc says it waits; out again, it is back at its owner's side, told so. The control:
// Cora, with no hire, is brought nothing.
func TestCTFMercComesBackFromItsRow(t *testing.T) {
	r, m, db := newMercFixture(t)
	now := time.Unix(1_800_000_000, 0)
	m.now = func() time.Time { return now }
	r.gw.OnMercNote(m.note) // as Start hears the World
	set := entity.DefaultCTFSettings()
	r.gw.SetCTF(true, set, entity.CTFLayout{Name: "the Test Canyon"})
	if err := db.SetMerc(database.MercRow{ServerID: r.pa.CharServerID, Name: "Ralthas", Class: 0, Race: 0, Stance: 2}); err != nil {
		t.Fatal(err)
	}
	m.comeBack(r.anna, r.pa)
	if _, ok := r.gw.MercInfoOf(r.anna); !ok {
		t.Fatal("no merc before the arena")
	}
	r.drain()

	r.pa.GM = true // stands in the arena's world with no match: the stray rule leaves a GM
	r.pa.World = set.World
	r.gw.Tick(now)
	if _, ok := r.gw.MercInfoOf(r.anna); ok {
		t.Fatal("out in the arena's world")
	}
	if got := r.msgs(r.anna); !hasMsg(got, "chat:Your mercenary Ralthas waits outside the arena: it rejoins you when you leave it.") {
		t.Errorf("going in, Anna was sent %q", got)
	}
	r.say(r.anna, "/merc")
	if got := r.msgs(r.anna); !hasMsg(got, "red:Your mercenary Ralthas waits outside the arena: it rejoins you when you leave it.") {
		t.Errorf("/merc in the arena: %q", got)
	}
	if _, ok := r.gw.MercInfoOf(r.anna); ok {
		t.Fatal("/merc called it into the arena")
	}

	r.pa.World = 0
	r.gw.Tick(now.Add(100 * time.Millisecond))
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, ok := r.gw.MercInfoOf(r.anna); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("not back once its owner left the arena's world")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := r.msgs(r.anna); !hasMsg(got, "chat:Your mercenary Ralthas is back at your side.") {
		t.Errorf("coming out, Anna was sent %q", got)
	}
	if _, ok := r.gw.MercInfoOf(r.cora); ok {
		t.Error("Cora, with no hire, has a merc")
	}
}
