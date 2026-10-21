package handlers

import (
	"encoding/binary"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// levels is accountinfo.acctlevel by account id.
type levels map[int]int

func (l levels) AccessLevel(id int) (int, error) {
	return l[id], nil
}

// gmWorld is a chat world where account 1 is a GM and account 2 is not.
func gmWorld(t *testing.T) (*chatWorld, *session.Session, *entity.Player, *session.Session, *entity.Player) {
	c := newChatWorld(t)
	c.h.SetAccessLevels(levels{1: 1, 2: 0})
	gs, gp := c.join(1, "Gm", 25000, 15000, 0)
	gs.SetAccountID(1)
	ps, pp := c.join(2, "Player", 25010, 15010, 0)
	ps.SetAccountID(2)
	return c, gs, gp, ps, pp
}

// teleported is where the 0x07F6 queued for s sends it, if one is.
func teleported(s *session.Session) (x, z float32, world byte, ok bool) {
	for _, m := range s.Queue().PeekPending() {
		d := m.Data()
		if len(d) >= 15 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeTeleport {
			world = d[2]
			x = f32(d[3:])
			z = f32(d[11:])
			ok = true
		}
	}
	return
}

func f32(b []byte) float32 {
	return math.Float32frombits(binary.LittleEndian.Uint32(b))
}

// A GM command is a GM's: a player's #pos and /goto are answered as an
// unknown command and never said; the GM's are run, in either spelling. The
// control: #hddsetup is everyone's.
func TestGMCommandsAreAGMsAlone(t *testing.T) {
	c, gs, _, ps, _ := gmWorld(t)
	c.say(ps, "#pos")
	if got := heard(t, ps); len(got) != 1 || got[0].op != protocol.OpcodeErrorMessage || got[0].text != `Unrecognized command "#pos".` {
		t.Fatalf("a player's #pos: %+v", got)
	}
	ps.Queue().GetPending()
	c.say(ps, "/goto 1 2 3")
	if got := heard(t, ps); len(got) != 1 || got[0].text != `Unrecognized command "/goto".` {
		t.Fatalf("a player's /goto: %+v", got)
	}
	if _, _, _, moved := teleported(ps); moved {
		t.Fatal("a player's /goto moved it")
	}
	c.say(gs, "#pos")
	if got := heard(t, gs); len(got) != 1 || !strings.HasPrefix(got[0].text, "[GM] Position: (25000.0") {
		t.Fatalf("the GM's #pos: %+v", got)
	}
	gs.Queue().GetPending()
	c.say(gs, "/POS")
	if got := heard(t, gs); len(got) != 1 || !strings.HasPrefix(got[0].text, "[GM] Position:") {
		t.Fatalf("the GM's /POS: %+v", got)
	}
	ps.Queue().GetPending()
	c.say(ps, "#hddsetup")
	for _, l := range heard(t, ps) {
		if strings.HasPrefix(l.text, "Unrecognized") {
			t.Fatalf("#hddsetup refused a player: %+v", l)
		}
	}
	// nobody is a GM without the accounts' levels
	c.h.SetAccessLevels(nil)
	gs.Queue().GetPending()
	c.say(gs, "#pos")
	if got := heard(t, gs); len(got) != 1 || got[0].op != protocol.OpcodeErrorMessage {
		t.Fatalf("#pos with no levels known: %+v", got)
	}
}

// "guard" finds the names it begins first, then a later word's; "guard a"
// only the guards going on with an A; nothing for no query.
func TestSearchNames(t *testing.T) {
	names := []string{"Captain of the Guard", "Guard Alben", "Guard Dahvel", "guard alys", "Lord Nagafen", "Nagafen"}
	if got := searchNames(names, "guard"); !slices.Equal(got, []string{"Guard Alben", "Guard Dahvel", "guard alys", "Captain of the Guard"}) {
		t.Fatalf("guard: %v", got)
	}
	if got := searchNames(names, "GUARD A"); !slices.Equal(got, []string{"Guard Alben", "guard alys"}) {
		t.Fatalf("guard a: %v", got)
	}
	if got := searchNames(names, "nagaf"); !slices.Equal(got, []string{"Nagafen", "Lord Nagafen"}) {
		t.Fatalf("nagaf: %v", got)
	}
	if got := searchNames(names, " "); got != nil {
		t.Fatalf("blank: %v", got)
	}
}

// /goto by a coachman's town, /gotoplayer to another player, and /summon
// bringing one beside the GM, into the GM's world.
func TestGotoSummonAndGotoPlayer(t *testing.T) {
	c, gs, gp, ps, pp := gmWorld(t)
	c.gw.NPCs[500] = &entity.NPC{ServerID: 500, Name: "Coachman Lothi", X: 25100, Y: 1, Z: 8800, SpawnX: 25100, SpawnY: 1, SpawnZ: 8800}
	c.say(gs, "/goto neriak")
	if x, z, w, ok := teleported(gs); !ok || x != 25100 || z != 8800 || w != 0 {
		t.Fatalf("/goto neriak: (%v, %v) world %d, %v", x, z, w, ok)
	}
	if gp.X != 25100 || gp.Z != 8800 {
		t.Fatalf("the GM is at (%v, %v)", gp.X, gp.Z)
	}
	gs.Queue().GetPending()
	pp.World = 3
	c.say(gs, "#gotoplayer player")
	if x, z, w, ok := teleported(gs); !ok || x != 25010 || z != 15010 || w != 3 || gp.World != 3 {
		t.Fatalf("#gotoplayer: (%v, %v) world %d, %v; GM's world %d", x, z, w, ok, gp.World)
	}
	pp.World = 0
	gp.World = 1
	gs.Queue().GetPending()
	c.say(gs, "/summon PLAYER")
	if x, z, w, ok := teleported(ps); !ok || x != gp.X || z != gp.Z || w != 1 || pp.World != 1 {
		t.Fatalf("/summon: (%v, %v) world %d, %v", x, z, w, ok)
	}
	if got := texts(heard(t, ps)); !slices.Contains(got, "[GM] Gm summoned you.") {
		t.Fatalf("the summoned heard %v", got)
	}
	// the control: a name nobody has moves nobody
	gs.Queue().GetPending()
	c.say(gs, "/summon Nobody")
	if _, _, _, ok := teleported(gs); ok {
		t.Fatal("summoning nobody moved the GM")
	}
	if got := texts(heard(t, gs)); len(got) != 1 || got[0] != `[GM] No player named "Nobody" is online.` {
		t.Fatalf("summoning nobody: %v", got)
	}
}

// /spawn puts a temporary copy of the NPC its name begins near the GM;
// several candidates are listed instead; /monstersearch lists them.
func TestSpawnAndMonsterSearch(t *testing.T) {
	c, gs, gp, _, _ := gmWorld(t)
	c.gw.NPCs[600] = &entity.NPC{ServerID: 600, Name: "Lord Nagafen", Level: 60, HP: 1, MaxHP: 9000, Dead: true}
	c.gw.NPCs[601] = &entity.NPC{ServerID: 601, Name: "Guard Alben"}
	c.gw.NPCs[602] = &entity.NPC{ServerID: 602, Name: "Guard Dahvel"}
	before := len(c.gw.NPCs)
	c.say(gs, "/spawn nagaf")
	if len(c.gw.NPCs) != before+1 {
		t.Fatalf("NPCs %d after /spawn, want %d", len(c.gw.NPCs), before+1)
	}
	var spawned *entity.NPC
	for _, n := range c.gw.NPCs {
		if n.Temporary {
			spawned = n
		}
	}
	if spawned == nil || spawned.Name != "Lord Nagafen" || spawned.Dead || spawned.HP != 9000 || spawned.World != gp.World {
		t.Fatalf("spawned %+v", spawned)
	}
	if dx, dz := spawned.X-gp.X, spawned.Z-gp.Z; dx*dx+dz*dz > 9.01 || dx*dx+dz*dz < 8.99 {
		t.Fatalf("spawned %v away, want 3", dx*dx+dz*dz)
	}
	gs.Queue().GetPending()
	c.say(gs, "#spawn guard")
	if len(c.gw.NPCs) != before+1 {
		t.Fatal("an ambiguous #spawn spawned")
	}
	if got := texts(heard(t, gs)); !slices.Equal(got, []string{`[GM] 2 NPCs match "guard"; which one?`, "[GM]   Guard Alben", "[GM]   Guard Dahvel"}) {
		t.Fatalf("#spawn guard: %v", got)
	}
	gs.Queue().GetPending()
	c.say(gs, "/monstersearch guard d")
	if got := texts(heard(t, gs)); !slices.Equal(got, []string{`[GM] 1 NPC matches "guard d":`, "[GM]   Guard Dahvel"}) {
		t.Fatalf("/monstersearch: %v", got)
	}
	if _, ok := c.gw.DespawnNPC(601); ok {
		t.Fatal("despawned an NPC with a database row")
	}
	if name, ok := c.gw.DespawnNPC(spawned.ServerID); !ok || name != "Lord Nagafen" || len(c.gw.NPCs) != before {
		t.Fatalf("despawn: %q %v, NPCs %d", name, ok, len(c.gw.NPCs))
	}
}
