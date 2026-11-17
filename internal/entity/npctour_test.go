package entity

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// santaSpec is tools/santa_tour_20261117.json: the tour the tool writes.
type santaSpec struct {
	Name      string               `json:"name"`
	Host      string               `json:"host"`
	Vehicle   string               `json:"vehicle"`
	Members   []string             `json:"members"`
	World     uint8                `json:"world"`
	Speed     float32              `json:"speed"`
	Countdown float32              `json:"countdown"`
	Cooldown  float32              `json:"cooldown"`
	Board     float32              `json:"board"`
	Season    string               `json:"season"`
	Route     []database.TourPoint `json:"route"`
	Deck      [][3]float32         `json:"deck"`
	Drop      []float32            `json:"drop"`
	Lines     database.TourLines   `json:"lines"`
	WalkOn    []struct {
		Name   string  `json:"name"`
		Mode   string  `json:"mode"`
		Radius float32 `json:"radius"`
		Height float32 `json:"height"`
	} `json:"walkon"`
}

func santaTour(t *testing.T) (database.NPCTour, []database.NPCWalkOn) {
	t.Helper()
	b, err := os.ReadFile("../../tools/santa_tour_20261117.json")
	if err != nil {
		t.Fatal(err)
	}
	var s santaSpec
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	tour := database.NPCTour{Name: s.Name, Host: s.Host, Vehicle: s.Vehicle, Members: s.Members, World: s.World, Route: s.Route,
		Deck: s.Deck, Drop: s.Drop, Speed: s.Speed, Countdown: s.Countdown, Cooldown: s.Cooldown, Board: s.Board, Lines: s.Lines, Season: s.Season}
	if p := tour.Check(); p != "" {
		t.Fatalf("the tool's tour: %s", p)
	}
	var wo []database.NPCWalkOn
	for _, w := range s.WalkOn {
		wo = append(wo, database.NPCWalkOn{Name: database.NPCClassKey(w.Name), Mode: w.Mode, Radius: w.Radius, Height: w.Height})
	}
	return tour, wo
}

// The NPCs as Heirloom's rows stand (christmas4's sleigh tool): the sleigh
// facing a half turn, the team east of it facing east, Santa at the reins.
var santaRows = []struct {
	id      EntityID
	name    string
	x, y, z float32
	facing  byte
	size    float32
}{
	{190715, "Santa Claus", 25287.310546875, 54.8471832275391, 15768.896484375, 64, 1},
	{190716, "Santa's Sleigh", 25285.1, 54.1, 15768.9, 128, 1},
	{190717, "Rudolph", 25301.7, 54.1, 15768.9, 64, 0.65}, // two pairs ahead (christmas_full_team_20261117.py)
	{190718, "Dasher", 25294.2, 54.1, 15768.0, 64, 0.65},
	{190719, "Dancer", 25294.2, 54.1, 15769.8, 64, 0.65},
	{190720, "Prancer", 25291.7, 54.1, 15768.0, 64, 0.65},
	{190721, "Vixen", 25291.7, 54.1, 15769.8, 64, 0.65},
	{190730, "Comet", 25296.7, 54.1, 15768.0, 64, 0.65},
	{190731, "Cupid", 25296.7, 54.1, 15769.8, 64, 0.65},
	{190732, "Donner", 25299.2, 54.1, 15768.0, 64, 0.65},
	{190733, "Blitzen", 25299.2, 54.1, 15769.8, 64, 0.65},
	{190713, "Holiday Elf", 25262.2, 54.094, 15744.1, 64, 1},
}

// tourRig is the tour's world: its NPCs, players with sessions, a clock.
type tourRig struct {
	t     *testing.T
	w     *World
	mgr   *session.Manager
	now   time.Time
	sess  map[EntityID]*session.Session
	tour  database.NPCTour
	nextP EntityID
}

func newTourRig(t *testing.T) *tourRig {
	t.Helper()
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	for _, r := range santaRows {
		f := float32(r.facing) / 256 * 2 * math.Pi
		n := &NPC{ServerID: r.id, Name: r.name, Level: 50, HP: 1000, MaxHP: 1000, X: r.x, Y: r.y, Z: r.z, Facing: f,
			SpawnX: r.x, SpawnY: r.y, SpawnZ: r.z, SpawnFacing: f, Size: r.size}
		w.NPCs[n.ServerID] = n
		w.SpatialIndex.Insert(n.ServerID, n.X, n.Z)
	}
	tour, wo := santaTour(t)
	w.SetNPCTours([]database.NPCTour{tour}, wo, map[string]bool{"christmas": true})
	return &tourRig{t: t, w: w, mgr: mgr, now: time.Date(2026, 12, 24, 20, 0, 0, 0, time.UTC), sess: map[EntityID]*session.Session{}, tour: tour, nextP: 5000}
}

// player puts a player at (x, z) on the sand; rides: its client answered layout 2.
func (r *tourRig) player(name string, x, z float32, rides bool) *Player {
	r.nextP++
	p := &Player{ServerID: r.nextP, Name: name, Level: 20, HP: 100, MaxHP: 100, Speed: 8.833333, X: x, Y: 54.1, Z: z}
	r.w.Players[p.ServerID] = p
	r.w.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
	s := r.mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, byte(r.nextP%250)), Port: 7000 + int(r.nextP)}, uint32(r.nextP), 1, 1, 2, true)
	s.SetECSEntity(p)
	s.SetCharacterInWorld(true)
	if rides {
		s.SetDecorLights(2)
	}
	r.sess[p.ServerID] = s
	return p
}

// tick runs the tour system at d past the rig's clock, which moves on to it.
func (r *tourRig) tick(d time.Duration) {
	r.now = r.now.Add(d)
	r.w.mu.Lock()
	r.w.tourSystem(r.now)
	r.w.mu.Unlock()
}

// fly ticks every 100 ms for d.
func (r *tourRig) fly(d time.Duration) {
	for k := time.Duration(0); k < d; k += 100 * time.Millisecond {
		r.tick(100 * time.Millisecond)
	}
}

// sent drains p's queue: each message whole, opcode first.
func (r *tourRig) sent(p *Player) [][]byte {
	var out [][]byte
	for _, m := range r.sess[p.ServerID].Queue().GetPending() {
		out = append(out, append([]byte(nil), m.Data()...))
	}
	return out
}

// texts is the chat lines among msgs (0x0A7A plain, 0x0A7B coloured).
func texts(msgs [][]byte) []string {
	var out []string
	for _, m := range msgs {
		op := opOfMsg(m)
		if op != protocol.OpcodeClientMessage && op != protocol.OpcodeColoredChat {
			continue
		}
		n := int(binary.LittleEndian.Uint32(m[2:6]))
		var b strings.Builder
		for i := 0; i < n && 6+2*i+1 < len(m); i++ {
			b.WriteRune(rune(binary.LittleEndian.Uint16(m[6+2*i:])))
		}
		out = append(out, b.String())
	}
	return out
}

func opsOf(msgs [][]byte) map[protocol.GameOpcode]int {
	out := map[protocol.GameOpcode]int{}
	for _, m := range msgs {
		out[opOfMsg(m)]++
	}
	return out
}

func (r *tourRig) npc(name string) *NPC {
	for _, n := range r.w.NPCs {
		if n.Name == name {
			return n
		}
	}
	r.t.Fatalf("no %s", name)
	return nil
}

func anyContains(lines []string, s string) bool {
	for _, l := range lines {
		if strings.Contains(l, s) {
			return true
		}
	}
	return false
}

// The route as laid out: about two and a half minutes, starting and ending on
// the sleigh's home heading the way its team stands (east), its waypoints
// reached in order, and the whole party rigid round the sleigh all the way.
func TestTourRouteAndFormation(t *testing.T) {
	r := newTourRig(t)
	d := r.w.tours.defs["santa's sleigh tour"]
	if d == nil {
		t.Fatal("no tour")
	}
	if d.total < 120 || d.total > 200 {
		t.Errorf("the flight takes %.0f s, want 2 to 3 minutes", d.total)
	}
	if east := float32(math.Pi / 2); math.Abs(float64(angleBetween(d.home, east))) > 0.2 {
		t.Errorf("it leaves heading %.2f, want east (%.2f): the team stands east of the sleigh", d.home, east)
	}
	for i := 1; i < len(d.arrive); i++ {
		if d.arrive[i] < d.leave[i-1] || d.leave[i] < d.arrive[i] {
			t.Fatalf("point %d: arrive %.1f, leave %.1f after %.1f", i, d.arrive[i], d.leave[i], d.leave[i-1])
		}
		if math.Abs(d.sAt(d.arrive[i])-float64(d.knotS[i])) > 0.05 {
			t.Errorf("point %d: at its arrival the sleigh is at %.2f along, the point at %.2f", i, d.sAt(d.arrive[i]), d.knotS[i])
		}
	}
	// the party from the sleigh, across the ground: as at home, all the way
	veh := r.npc("Santa's Sleigh")
	var members []tourMember
	for _, n := range []string{"Santa Claus", "Rudolph", "Dasher", "Dancer", "Prancer", "Vixen", "Comet", "Cupid", "Donner", "Blitzen"} {
		m := r.npc(n)
		members = append(members, tourMember{id: m.ServerID, rel: [3]float32{m.SpawnX - veh.SpawnX, m.SpawnY - veh.SpawnY, m.SpawnZ - veh.SpawnZ},
			home: [4]float32{m.SpawnX, m.SpawnY, m.SpawnZ, m.SpawnFacing}})
	}
	vm := tourMember{vehicle: true, home: [4]float32{veh.SpawnX, veh.SpawnY, veh.SpawnZ, veh.SpawnFacing}}
	for tt := 0.0; tt <= d.total; tt += 0.5 {
		vx, _, vz, _ := d.memberAt(tt, vm)
		for _, m := range members {
			x, _, z, _ := d.memberAt(tt, m)
			got := math.Hypot(float64(x-vx), float64(z-vz))
			want := math.Hypot(float64(m.rel[0]), float64(m.rel[2]))
			if math.Abs(got-want) > 1e-2 {
				t.Fatalf("at %.1f s a member stands %.3f from the sleigh, at home %.3f", tt, got, want)
			}
		}
	}
	// home at both ends, exactly
	for _, tt := range []float64{0, d.total} {
		for _, m := range append(members, vm) {
			x, y, z, f := d.memberAt(tt, m)
			if math.Abs(float64(x-m.home[0])) > 1e-3 || math.Abs(float64(y-m.home[1])) > 1e-3 || math.Abs(float64(z-m.home[2])) > 1e-3 ||
				math.Abs(float64(angleBetween(f, m.home[3]))) > 1e-3 {
				t.Errorf("at %.0f s a member is at (%.2f, %.2f, %.2f) facing %.3f, home (%.2f, %.2f, %.2f) facing %.3f",
					tt, x, y, z, f, m.home[0], m.home[1], m.home[2], m.home[3])
			}
		}
	}
	// smooth: no step of the sleigh in a tenth of a second past the speed
	for tt := 0.0; tt+0.1 <= d.total; tt += 0.1 {
		ax, ay, az, _ := d.memberAt(tt, vm)
		bx, by, bz, _ := d.memberAt(tt+0.1, vm)
		if v := math.Sqrt(sq(float64(bx-ax))+sq(float64(by-ay))+sq(float64(bz-az))) / 0.1; v > float64(d.Speed)*1.25 {
			t.Fatalf("at %.1f s the sleigh moves at %.1f a second (speed %.0f)", tt, v, d.Speed)
		}
	}
}

// Starting: the talk offers it; the start shouts it to the town, with the
// countdown; one who climbs aboard by the talk and one standing by the sleigh
// ride, one far off does not; a talk out or resting says how long.
func TestTourStartCountdownBoardFlyLand(t *testing.T) {
	r := newTourRig(t)
	santa, elf := r.npc("Santa Claus"), r.npc("Holiday Elf")
	starter := r.player("Starter", 25283, 15763, true) // vanilla, by the tree
	byS := r.player("Bystander", 25287, 15771, false)  // a PS2 standing by the sleigh
	late := r.player("Latecomer", 25250, 15720, true)  // climbs aboard by the talk, 60 off
	far := r.player("Faraway", 25290, 15300, false)    // the north gate: hears the shout, does not ride
	off := r.player("Offworld", 25285, 15769, false)
	off.World = 3

	talk, ok := r.w.TalkTour(starter, santa, r.now)
	if !ok || len(talk.Answers) != 2 || !strings.Contains(talk.Answers[0], "tour of Freeport") {
		t.Fatalf("Santa's talk idle: %+v", talk)
	}
	if line := r.w.AnswerTour(r.sess[starter.ServerID], starter, talk.Tour, true, r.now); line == "" {
		t.Fatal("no word on the start")
	}
	for _, p := range []*Player{starter, byS, late, far} {
		if l := texts(r.sent(p)); !anyContains(l, "Santa Claus shouts: Ho ho ho! All aboard!") || !anyContains(l, "in 30 seconds") {
			t.Errorf("%s heard %q", p.Name, l)
		}
	}
	if l := texts(r.sent(off)); len(l) != 0 {
		t.Errorf("another world heard %q", l)
	}
	r.tick(5 * time.Second)
	talk, _ = r.w.TalkTour(late, santa, r.now)
	if len(talk.Answers) != 2 || !strings.Contains(talk.Ask, "25 seconds") {
		t.Fatalf("Santa's talk counting down: %+v", talk)
	}
	r.w.AnswerTour(r.sess[late.ServerID], late, talk.Tour, true, r.now)
	r.tick(15 * time.Second)
	if l := texts(r.sent(far)); !anyContains(l, "Last call") {
		t.Errorf("no last call: %q", l)
	}
	r.tick(10*time.Second + time.Millisecond)
	run := r.w.tours.runs["santa's sleigh tour"]
	if run.phase != tourFlying {
		t.Fatalf("not away: phase %d", run.phase)
	}
	for _, p := range []*Player{starter, byS, late} {
		if run.riders[p.ServerID] == nil || p.tourRide == nil {
			t.Errorf("%s is not aboard", p.Name)
		}
	}
	if run.riders[far.ServerID] != nil || run.riders[off.ServerID] != nil {
		t.Error("one far off, or in another world, is aboard")
	}
	// the vanilla riders told to ride, every rider held (speed 0) and told the start
	for _, p := range []*Player{starter, byS, late} {
		msgs := r.sent(p)
		ops := opsOf(msgs)
		if ops[protocol.OpcodePlayerSpeed] != 1 {
			t.Errorf("%s: no speed 0 (%v)", p.Name, ops)
		}
		rides := p != byS
		if (ops[protocol.OpcodeRide] == 1) != rides {
			t.Errorf("%s: ride messages %d, rides %v", p.Name, ops[protocol.OpcodeRide], rides)
		}
		if !anyContains(texts(msgs), "Santa Claus: Hold on to your hats!") {
			t.Errorf("%s: no word as it leaves: %q", p.Name, texts(msgs))
		}
		if p.SpeedNow() != 0 {
			t.Errorf("%s walks at %.2f aboard", p.Name, p.SpeedNow())
		}
	}
	// a rider's own reports are not its place
	before := [3]float32{starter.X, starter.Y, starter.Z}
	r.w.ApplyClientUpdate(starter.ServerID, ClientUpdate{X: 1, Y: 2, Z: 3, At: r.now})
	if [3]float32{starter.X, starter.Y, starter.Z} != before {
		t.Error("a rider's report moved it")
	}
	// out: Santa says how long, the Elf where he is
	talk, _ = r.w.TalkTour(far, santa, r.now)
	if talk.Say == "" || len(talk.Answers) != 0 {
		t.Errorf("Santa's talk out: %+v", talk)
	}
	if line, ok := r.w.TourStandLine(elf, r.now); !ok || !strings.Contains(line, "Santa's out on his sleigh") {
		t.Errorf("the Elf: %q %v", line, ok)
	}

	// 10 s in, on its way: a velocity the clients carry it on by
	r.fly(10 * time.Second)
	veh := r.npc("Santa's Sleigh")
	if veh.VelX == 0 && veh.VelZ == 0 {
		t.Error("the sleigh has no velocity: the clients would not carry it on")
	}
	// 20 s in: aloft (the harbor), the party together, the riders on the deck
	r.fly(10 * time.Second)
	if veh.Y < 70 {
		t.Errorf("20 s in the sleigh is at y %.1f", veh.Y)
	}
	for _, p := range []*Player{starter, byS, late} {
		if dist := math.Hypot(float64(p.X-veh.X), float64(p.Z-veh.Z)); dist > 3.5 || p.Y < veh.Y || p.Y > veh.Y+1.5 {
			t.Errorf("%s is %.1f from the sleigh, at y %.1f to its %.1f", p.Name, dist, p.Y, veh.Y)
		}
	}
	rud := r.npc("Rudolph")
	if dist := math.Hypot(float64(rud.X-veh.X), float64(rud.Z-veh.Z)); math.Abs(dist-16.6) > 0.05 {
		t.Errorf("Rudolph is %.2f from the sleigh, 16.6 at home", dist)
	}
	// the harbor's line, and the PS2 rider set down to watch it
	d := run.def
	r.fly(time.Duration((d.arrive[4]-d.leave[0]-20+0.5)*float64(time.Second)) + time.Second)
	sm := r.sent(byS)
	if !anyContains(texts(sm), "Freeport's harbor") {
		t.Errorf("no harbor line: %q", texts(sm))
	}
	if opsOf(sm)[protocol.OpcodeTeleport] != 1 {
		t.Errorf("the PS2 rider was not set down at the harbor's view: %v", opsOf(sm))
	}
	if opsOf(r.sent(starter))[protocol.OpcodeTeleport] != 0 {
		t.Error("a vanilla rider was teleported")
	}
	if !anyContains(texts(r.sent(late)), "Freeport's harbor") {
		t.Error("the latecomer heard no harbor line")
	}

	// home: everyone where they stood, the riders set down beside the tree
	r.fly(time.Duration((d.total-(r.now.Sub(run.at).Seconds()))*float64(time.Second)) + 200*time.Millisecond)
	if run.phase != tourResting {
		t.Fatalf("not home: phase %d", run.phase)
	}
	for _, row := range santaRows {
		n := r.w.NPCs[row.id]
		if n.X != row.x || n.Y != row.y || n.Z != row.z || n.moving() || math.Abs(float64(angleBetween(n.Facing, n.SpawnFacing))) > 1e-6 {
			t.Errorf("%s home at (%.2f, %.2f, %.2f) facing %.3f moving %v", n.Name, n.X, n.Y, n.Z, n.Facing, n.moving())
		}
	}
	for _, p := range []*Player{starter, byS, late} {
		if p.tourRide != nil {
			t.Errorf("%s still aboard", p.Name)
		}
		if dist := math.Hypot(float64(p.X-25281), float64(p.Z-15761)); dist > 3 || p.Y != 54.1 {
			t.Errorf("%s set down at (%.1f, %.1f, %.1f)", p.Name, p.X, p.Y, p.Z)
		}
		msgs := r.sent(p)
		ops := opsOf(msgs)
		if p == byS {
			if ops[protocol.OpcodeTeleport] < 1 {
				t.Errorf("the PS2 rider was not teleported home: %v", ops)
			}
		} else if ops[protocol.OpcodeRide] != 1 {
			t.Errorf("%s: no ride off: %v", p.Name, ops)
		}
		if ops[protocol.OpcodePlayerSpeed] != 1 || p.SpeedNow() == 0 {
			t.Errorf("%s: its speed not given back (%v, %.2f)", p.Name, ops, p.SpeedNow())
		}
		if !anyContains(texts(msgs), "Back by the tree") {
			t.Errorf("%s: no landing line: %q", p.Name, texts(msgs))
		}
	}
	// resting: Santa says when the next leaves; then he offers it again
	talk, _ = r.w.TalkTour(starter, santa, r.now)
	if !strings.Contains(talk.Say, "next tour leaves in about 120 seconds") {
		t.Errorf("resting: %+v", talk)
	}
	if _, ok := r.w.TourStandLine(elf, r.now); ok {
		t.Error("the Elf still says he is out")
	}
	r.tick(121 * time.Second)
	if talk, _ = r.w.TalkTour(starter, santa, r.now); len(talk.Answers) != 2 {
		t.Errorf("after the rest: %+v", talk)
	}
}

// A rider who logs out on the way is set down (and saved) beside the tree; one
// a gate takes away stays where the gate put it; the flight goes on.
func TestTourLogoutAndAGateMidRide(t *testing.T) {
	r := newTourRig(t)
	santa := r.npc("Santa Claus")
	a := r.player("Leaver", 25285, 15770, true)
	b := r.player("Gater", 25286, 15768, false)
	c := r.player("Stayer", 25284, 15768, true)
	talk, _ := r.w.TalkTour(a, santa, r.now)
	r.w.AnswerTour(r.sess[a.ServerID], a, talk.Tour, true, r.now)
	r.tick(30*time.Second + time.Millisecond)
	r.fly(30 * time.Second)
	if a.Y < 70 {
		t.Fatalf("not aloft: %.1f", a.Y)
	}
	if x, y, z, back := a.SavePlace(); !back || x != 25281 || y != 54.1 || z != 15761 {
		t.Errorf("a rider's logout saves (%.1f, %.1f, %.1f) %v", x, y, z, back)
	}
	r.w.RemovePlayer(a.ServerID)
	run := r.w.tours.runs["santa's sleigh tour"]
	if run.riders[a.ServerID] != nil || a.tourRide != nil {
		t.Error("the one logged out still rides")
	}
	if a.Y != 54.1 || math.Hypot(float64(a.X-25281), float64(a.Z-15761)) > 0.01 {
		t.Errorf("logged out at (%.1f, %.1f, %.1f)", a.X, a.Y, a.Z)
	}
	// a gate: the server's own move elsewhere
	r.w.UpdatePosition(b.ServerID, 20000, 60, 9000, 0)
	r.fly(time.Second)
	if run.riders[b.ServerID] != nil || b.tourRide != nil {
		t.Error("the one gated away still rides")
	}
	if b.X != 20000 || b.Z != 9000 {
		t.Errorf("the gated one was moved to (%.1f, %.1f)", b.X, b.Z)
	}
	if !anyContains(texts(r.sent(b)), "You've left Santa's sleigh") {
		t.Error("the gated one was not told")
	}
	if run.phase != tourFlying || run.riders[c.ServerID] == nil {
		t.Fatal("the flight did not go on with the one who stayed")
	}
}

// A keep restart lands a flight at once: everyone home, riders beside the
// tree, so nothing kept is in the air.
func TestTourEndsAtAKeepRestart(t *testing.T) {
	r := newTourRig(t)
	santa := r.npc("Santa Claus")
	a := r.player("Rider", 25285, 15770, true)
	talk, _ := r.w.TalkTour(a, santa, r.now)
	r.w.AnswerTour(r.sess[a.ServerID], a, talk.Tour, true, r.now)
	r.tick(30*time.Second + time.Millisecond)
	r.fly(25 * time.Second)
	r.w.StandNPCs()
	for _, row := range santaRows {
		if n := r.w.NPCs[row.id]; n.X != row.x || n.Y != row.y || n.Z != row.z {
			t.Errorf("%s at (%.1f, %.1f, %.1f) after the restart's stand", n.Name, n.X, n.Y, n.Z)
		}
	}
	if a.tourRide != nil || a.Y != 54.1 {
		t.Errorf("the rider: aboard %v at y %.1f", a.tourRide != nil, a.Y)
	}
	if len(r.w.KeepNPCs()) != 0 {
		t.Errorf("the restart keeps %d NPCs off their rows", len(r.w.KeepNPCs()))
	}
}

// No one aboard when it leaves: Santa says so and the tour is offered again
// at once (no rest).
func TestTourWithNoOneAboard(t *testing.T) {
	r := newTourRig(t)
	santa := r.npc("Santa Claus")
	a := r.player("Wanderer", 25285, 15770, true)
	talk, _ := r.w.TalkTour(a, santa, r.now)
	r.w.AnswerTour(r.sess[a.ServerID], a, talk.Tour, true, r.now)
	r.w.UpdatePosition(a.ServerID, 25285, 54.1, 15500, 0) // walked off 270
	r.tick(31 * time.Second)
	run := r.w.tours.runs["santa's sleigh tour"]
	if run.phase != tourIdle {
		t.Fatalf("phase %d with no one aboard", run.phase)
	}
	if !anyContains(texts(r.sent(a)), "No one aboard") {
		t.Errorf("not told: %q", texts(r.sent(a)))
	}
}

// The deck walked onto: a report standing on the sleigh's deck, 0.7 over the
// sand, is a floor to the movement check, as it is on the client that walks
// onto it; anywhere else a body that high is held up by nothing.
func TestTourDeckIsAFloor(t *testing.T) {
	for _, deck := range []bool{true, false} {
		r := newMCRig(t, MoveCheckEnforce, 40, 0, 50)
		sl := &NPC{ServerID: 9001, Name: "Santa's Sleigh", X: 50, Y: 0, Z: 50, SpawnX: 50, SpawnZ: 50}
		r.w.NPCs[sl.ServerID] = sl
		var wo []database.NPCWalkOn
		if deck {
			wo = []database.NPCWalkOn{{Name: "santa's sleigh", Mode: database.WalkOnDeck, Radius: 2.6, Height: 1}}
		}
		r.w.SetNPCTours(nil, wo, nil)
		r.walk(48, 50, 5, flat)
		missed := 0
		for i := 0; i < 30; i++ { // onto the deck, then three seconds standing on it
			if !r.report(48.5+float32(min(i, 6))*0.25, 0.72, 50, 100) {
				missed++
			}
		}
		if deck && missed != 0 {
			t.Errorf("the deck: %d reports not taken (%s)", missed, r.p.mv.why)
		}
		if !deck && missed == 0 {
			t.Error("no deck: a body 0.72 over the sand was taken for three seconds")
		}
	}
}

// The walk-ons go to a client that walks with the decorations, after the
// papers: the sleigh a deck, the team passed through.
func TestTourWalkOnsSent(t *testing.T) {
	_, wo := santaTour(t)
	b := AppendWalkOns(nil, wo)
	if n := binary.LittleEndian.Uint16(b); int(n) != len(wo) {
		t.Fatalf("count %d", n)
	}
	l := int(b[2])
	if name := string(b[3 : 3+l]); name != "santa's sleigh" || b[3+l] != 1 {
		t.Errorf("first: %q mode %d", name, b[3+l])
	}
	if r := math.Float32frombits(binary.LittleEndian.Uint32(b[4+l:])); r != 2.6 {
		t.Errorf("radius %.2f", r)
	}
}
