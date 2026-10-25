package handlers

import (
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// bindsKept is a bindStore that remembers what it was given.
type bindsKept map[uint32]database.BindRow

func (b bindsKept) SaveBind(id uint32, r database.BindRow) error {
	b[id] = r
	return nil
}

// said is the chat lines queued for the session (taken from a copy).
func said(tw *talkWorld) []string {
	var out []string
	for _, l := range heard(tw.t, tw.s) {
		out = append(out, l.text)
	}
	return out
}

// A spiritmaster asks "Do you wish to set this as your bind point?", Yes
// binds the player right in front of it, kept, and Return Home takes it
// there (F3 20261025-172313). The controls: No binds nothing, and before any
// bind Return Home goes where every login starts.
func TestASpiritmasterBinds(t *testing.T) {
	tw := newTalkWorld(t, 23594, 6304)
	tw.p.Y = 32.6
	tw.p.CharServerID = 44
	kept := bindsKept{}
	tw.h.binds = kept
	savi := &entity.NPC{ServerID: 101714, Name: "Spiritmaster Savi", SpawnX: 23594.3, SpawnY: 32.6, SpawnZ: 6300.9,
		SpawnFacing: float32(252) / 256 * 2 * math.Pi}
	id := tw.add(1, savi)
	ch := &CharacterHandler{gameWorld: tw.w}

	// before: home is the login spawn, beside Vaal
	ch.gatePlayer(tw.s, tw.p)
	vx, _, _, _ := teleported(tw.s)
	tw.s.Queue().GetPending()
	if want := teleDests[loginSpawnDest].X + loginSpawnOffset; math.Abs(float64(vx-want)) > 0.1 {
		t.Fatalf("the control: Return Home before a bind went to x %.1f, want the login spawn's %.1f", vx, want)
	}
	tw.w.UpdatePosition(tw.p.ServerID, 23594, 32.6, 6304, 0) // back at Savi's

	tw.talk(id)
	if got := optionBox(t, tw.s); !slices.Equal(got, []string{"Yes", "No"}) {
		t.Fatalf("Spiritmaster Savi offered %q, want Yes and No", got)
	}
	tw.pick(id, 1)
	if _, bound := tw.w.BindOf(tw.p.ServerID); bound || len(kept) != 0 {
		t.Fatal("the control: No bound the player")
	}
	tw.talk(id)
	tw.s.Queue().GetPending()
	tw.pick(id, 0)
	b, bound := tw.w.BindOf(tw.p.ServerID)
	if !bound {
		t.Fatal("Yes did not bind the player")
	}
	if d := math.Hypot(float64(b.X-savi.SpawnX), float64(b.Z-savi.SpawnZ)); math.Abs(d-bindReach) > 0.01 {
		t.Errorf("bound %.2f from Savi, want %.1f in front of her", d, bindReach)
	}
	h := float64(savi.SpawnFacing)
	if dx, dz := float64(b.X-savi.SpawnX), float64(b.Z-savi.SpawnZ); dx*math.Sin(h)+dz*math.Cos(h) < 2.9 {
		t.Errorf("bound at (%.1f, %.1f), not in front of Savi (heading %.2f)", b.X, b.Z, h)
	}
	if r, ok := kept[44]; !ok || r.Spiritmaster != "Spiritmaster Savi" || r.X != b.X || r.Z != b.Z {
		t.Errorf("kept %+v (%v), want the bind under character 44", r, ok)
	}
	if got := said(tw); !slices.Contains(got, "Spiritmaster Savi: "+boundLine) {
		t.Errorf("Savi said %q", got)
	}
	tw.s.Queue().GetPending()
	ch.gatePlayer(tw.s, tw.p)
	if x, z, w, ok := teleported(tw.s); !ok || x != b.X || z != b.Z || w != 0 {
		t.Errorf("Return Home went to (%.1f, %.1f) world %d (%v), want the bind point (%.1f, %.1f)", x, z, w, ok, b.X, b.Z)
	}
}

// A bind point the session's game cannot load, or no place to stand, is
// passed over for the login spawn (homeOf).
func TestAnUnusableBindIsPassedOver(t *testing.T) {
	tw := newTalkWorld(t, 23594, 6304)
	tw.w.SetBind(tw.p.ServerID, entity.Bind{Set: true, World: 8, X: 1125, Y: -62.2, Z: 1143.7, By: "Spiritmaster Far"})
	x, _, _, world, _, ok := homeOf(tw.w, tw.s, tw.p.ServerID)
	if want := teleDests[loginSpawnDest].X + loginSpawnOffset; !ok || world != 0 || x != want {
		t.Errorf("a bind in world 8 for the disc's six: home (%.1f) world %d, want the login spawn", x, world)
	}
	tw.w.SetBind(tw.p.ServerID, entity.Bind{Set: true, World: 0, X: 23597, Y: 32.6, Z: 6300, By: "Spiritmaster Savi"})
	if x, _, _, world, _, ok := homeOf(tw.w, tw.s, tw.p.ServerID); !ok || world != 0 || x != 23597 {
		t.Errorf("the control: a good bind gave home x %.1f world %d", x, world)
	}
}

// Waysender Brindel sends a player of level 55 or more to Lavastorm's
// outpost (world 3), asked as Yes or No, and tells anyone below that they
// are too weak (F3 20261025-173746). The control: No sends nobody.
func TestBrindelSendsTheStrongToLavastorm(t *testing.T) {
	tw := newTalkWorld(t, 24300, 6563)
	brindel := tw.add(1, &entity.NPC{ServerID: 106277, Name: "Waysender Brindel", SpawnX: 24300.9, SpawnY: 54.1, SpawnZ: 6566.5})
	tw.w.NPCs[900] = &entity.NPC{ServerID: 900, Name: "Explorer Springa", World: 3, SpawnX: 7908, SpawnY: 75, SpawnZ: 5925}

	tw.p.Level = 30
	tw.talk(brindel)
	var box string
	for _, m := range tw.s.Queue().GetPending() {
		if d := m.Data(); len(d) > 6 && protocol.GameOpcode(d[0])|protocol.GameOpcode(d[1])<<8 == protocol.OpcodeDialogueBox {
			box = wideText(d[6:])
		}
	}
	if !strings.Contains(box, "too weak") || !strings.Contains(box, "Lavastorm") {
		t.Fatalf("a level 30 was told %q, want too weak for Lavastorm", box)
	}
	tw.pick(brindel, 0)
	if _, _, _, moved := teleported(tw.s); moved {
		t.Fatal("a level 30 was sent to Lavastorm")
	}

	tw.p.Level = 55
	tw.talk(brindel)
	if got := optionBox(t, tw.s); !slices.Equal(got, []string{"Yes", "No"}) {
		t.Fatalf("a level 55 was offered %q, want Yes and No", got)
	}
	tw.pick(brindel, 1)
	if _, _, _, moved := teleported(tw.s); moved {
		t.Fatal("the control: No sent the player")
	}
	tw.talk(brindel)
	tw.s.Queue().GetPending()
	tw.pick(brindel, 0)
	x, z, world, moved := teleported(tw.s)
	if !moved || world != 3 || x != 7908 || z != 5925 {
		t.Errorf("Yes sent the player to (%.0f, %.0f) world %d (%v), want Lavastorm's outpost (7908, 5925) world 3", x, z, world, moved)
	}
	if got := said(tw); !slices.Contains(got, "Waysender Brindel: Very well.") {
		t.Errorf("Brindel said %q", got)
	}
	if tw.p.World != 3 {
		t.Errorf("the server filed the player in world %d, want 3", tw.p.World)
	}
}

// Waysender Ti`Thox lists Freeport and Grobb with their fees and takes the
// fee from a player who has it; one who has not is told the price and stays.
func TestATripCostsItsFee(t *testing.T) {
	tw := newTalkWorld(t, 25027, 8910)
	tw.p.Y = 0.6
	thox := tw.add(1, &entity.NPC{ServerID: 2616, Name: "Waysender Ti`Thox", SpawnX: 25027.5, SpawnY: 0.6, SpawnZ: 8913.3})
	tw.w.NPCs[695] = &entity.NPC{ServerID: 695, Name: "Waysender Abodai", SpawnX: 25405.4, SpawnY: 54.1, SpawnZ: 15781}
	tw.w.NPCs[101655] = &entity.NPC{ServerID: 101655, Name: "Waysender Zapgit", SpawnX: 25436.3, SpawnY: 63.8, SpawnZ: 31347.3}
	tw.h.spend = func(_ *session.Session, p *entity.Player, n int) bool {
		if n > p.Tunar {
			return false
		}
		p.Tunar -= n
		return true
	}

	tw.p.Tunar = 50
	tw.talk(thox)
	if got := optionBox(t, tw.s); !slices.Equal(got, []string{"Freeport (100 tunar)", "Grobb (225 tunar)"}) {
		t.Fatalf("Ti`Thox offered %q", got)
	}
	tw.pick(thox, 0)
	if _, _, _, moved := teleported(tw.s); moved || tw.p.Tunar != 50 {
		t.Fatalf("a player with 50 tunar was sent (tunar now %d)", tw.p.Tunar)
	}

	tw.p.Tunar = 500
	tw.talk(thox)
	tw.s.Queue().GetPending()
	tw.pick(thox, 0)
	if x, z, _, moved := teleported(tw.s); !moved || x != 25405.4 || z != 15781 {
		t.Errorf("Freeport: sent to (%.1f, %.1f) (%v), want Abodai's spot", x, z, moved)
	}
	if tw.p.Tunar != 400 {
		t.Errorf("tunar %d after the trip, want 400", tw.p.Tunar)
	}
}

// A new box replaces the old one: a spiritmaster's question left open, then
// a coachman's list, and the pick goes to the coachman's list.
func TestANewBoxReplacesTheOld(t *testing.T) {
	tw := newTalkWorld(t, 25274, 15716)
	savi := tw.add(1, &entity.NPC{ServerID: 1, Name: "Spiritmaster Imaryn", SpawnX: 25275, SpawnY: 57.8, SpawnZ: 15717})
	ronks := tw.add(2, &entity.NPC{ServerID: 2, Name: "Coachman Ronks", SpawnX: 25273.7, SpawnY: 54.1, SpawnZ: 15719.1})
	tw.w.NPCs[3] = &entity.NPC{ServerID: 3, Name: "Coachman Huff", SpawnX: 18334.8, SpawnY: 54, SpawnZ: 11357.8}
	tw.talk(savi)
	tw.talk(ronks)
	tw.s.Queue().GetPending()
	tw.pick(ronks, 0)
	if x, _, _, moved := teleported(tw.s); !moved || x != 18334.8 {
		t.Errorf("the pick went to the spiritmaster's question, not to Rivervale (moved %v to x %.1f)", moved, x)
	}
	if _, bound := tw.w.BindOf(tw.p.ServerID); bound {
		t.Error("the coach's pick bound the player")
	}
}
