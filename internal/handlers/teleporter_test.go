package handlers

import (
	"math"
	"math/rand"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// teleRig is the portal rig (Anna at (5000, 50, 5000) in world 0) on a
// database holding the Teleporter beside her, one row of each raid's first
// boss 100 off its landing (and its way in's NPC), each #tele camp's anchor on
// its written spot, and two GM camps: one of frost bats far from every camp,
// one beside the xrag camp. The Teleporter is on, its clock the rig's.
func teleRig(t *testing.T) (*portalRig, *database.SQLiteDB, *entity.NPC) {
	t.Helper()
	r := newPortalRig(t)
	SetTeleporter(true)
	t.Cleanup(func() { SetTeleporter(false) })
	db, err := database.NewSQLite(filepath.Join(t.TempDir(), "tele.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	id := 1000
	row := func(name string, world uint8, x, y, z float32, facing, level, typ int) {
		t.Helper()
		id++
		if _, err := db.Exec(`INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race, zone)
			VALUES (?, ?, ?, ?, ?, ?, ?, 1893243078, 1.0, ?, ?, '0', 0)`, id, name, world, x, y, z, facing, level, typ); err != nil {
			t.Fatal(err)
		}
	}
	row(TeleporterName, 0, 5003, 50, 5000, 61, 60, 128)
	arrive := map[string][3]float32{"Nagafen Teleport Arrive": {4867.64, -29, 4423.88}, "Portal of the Twins": {4872, -19.9, 2987},
		"Portal to the Lake": {5640, -20.6, 3760}, "ZP1": {4898.8, 147.66, 7048.79}}
	arriveWorld := map[string]uint8{"Nagafen Teleport Arrive": 3, "Portal of the Twins": 5, "Portal to the Lake": 5, "ZP1": 5}
	for _, rd := range teleRaids {
		var w uint8
		var x, y, z float32
		if rd.at != nil {
			w, x, y, z = rd.at.World, rd.at.X, rd.at.Y, rd.at.Z
		} else {
			a := arrive[rd.arrive]
			w, x, y, z = arriveWorld[rd.arrive], a[0], a[1], a[2]
			row(rd.arrive, w, x, y, z, 0, 2, 384)
		}
		for i, b := range rd.bosses {
			row(b, w, x+100, y, z+float32(i), 0, 60+i, 0)
		}
		if rd.face != "" {
			row(rd.face, w, x-20, y, z, 0, 60, 384)
		}
	}
	for _, d := range campDests {
		low, _ := parseLevels(d.Levels)
		row(d.NPC, d.World, d.X, d.Y, d.Z, 0, max(low, 1), 0)
	}
	// the GMs' camps: frost bats at (9000, 9000), and a camp on Xrag's rock
	gm := func(camp int, kinds string, x, z float32, level int) {
		row(strings.Split(kinds, ",")[0], 0, x, 50, z, 0, level, 0)
		if _, err := db.Exec(`INSERT INTO campspawns (npcid, camp, kinds) VALUES (?, ?, ?)`, id, camp, kinds); err != nil {
			t.Fatal(err)
		}
	}
	gm(7, "frost bat, ice bat", 9000, 9000, 20)
	gm(7, "frost bat, ice bat", 9010, 9000, 22)
	row("ice bat", 0, 9100, 50, 9100, 0, 21, 0)
	x := campDests["xrag"]
	gm(8, "child of Xrag, pariah of Xrag", x.X+40, x.Z, 62)
	r.gw.LoadNPCs(database.NewNPCStore(db))
	if _, err := r.gw.LoadCampSpawns(db); err != nil {
		t.Fatal(err)
	}
	r.ih.tele.now = func() time.Time { return r.now }
	var tp *entity.NPC
	for _, n := range r.gw.NPCs {
		if n.Name == TeleporterName {
			tp = n
		}
	}
	return r, db, tp
}

// walk visits every box below n: each holds at most maxDialogOptions answers,
// each answer and question as long as CLIENT shows; it returns the trips'
// answers in order.
func walkTele(t *testing.T, n *teleNode, path string) []string {
	t.Helper()
	if n.spot != nil {
		return []string{n.label}
	}
	if len(n.kids) == 0 || len(n.kids) > maxDialogOptions {
		t.Errorf("%s: %d answers", path, len(n.kids))
	}
	if l := len([]rune(n.ask)); l == 0 || l > questAskChars {
		t.Errorf("%s: its question is %d characters: %q", path, l, n.ask)
	}
	var out []string
	for _, k := range n.kids {
		if l := len([]rune(k.label)); l == 0 || l > questAnswerChars {
			t.Errorf("%s: an answer of %d characters: %q", path, l, k.label)
		}
		out = append(out, walkTele(t, k, path+" > "+k.label)...)
	}
	return out
}

// Its boxes: Raids and the camps by levels at first, every box at most six
// answers (CLIENT shows three a page), each raid once under where it is, named
// with its bosses' levels, each camp once in level order, a GM's camp of its
// own named by its kinds and their levels, a GM's camp beside a #tele camp
// folded into it.
func TestTeleporterBoxes(t *testing.T) {
	r, _, _ := teleRig(t)
	root := r.ih.tele.tree(r.gw, r.anna)
	trips := walkTele(t, root, "first")
	labels := func(n *teleNode) []string {
		var l []string
		for _, k := range n.kids {
			l = append(l, k.label)
		}
		return l
	}
	if got := strings.Join(labels(root), "|"); got != "Raids|Isle of Dread|Plane of Sky|Camps, levels 1-25|Camps, levels 28-61" {
		t.Errorf("the first box: %q", got)
	}
	if got := strings.Join(labels(root.kids[0]), "|"); got != strings.Join(teleRaidGroups, "|") {
		t.Errorf("the raids' box: %q", got)
	}
	if got := strings.Join(labels(root.kids[0].kids[0]), "|"); got != "Lady Vox, Permafrost (60)|Siliskor (60)|Skahyir's cave (60)|"+
		"Cazic-Thule's Estate, Isle of Dread (60)|Megaladon, Isle of Dread (60)" {
		t.Errorf("Tunaria's raids: %q", got)
	}
	if got := labels(root.kids[0].kids[2]); len(got) != 3 || got[1] != "Bertoxxulous and the twins (60-62)" {
		t.Errorf("the Plane of Disease's raids: %q", got)
	}
	n := 0
	for _, s := range trips {
		if strings.HasPrefix(s, "Frost bat, ice bat") {
			n++
			if s != "Frost bat, ice bat (20-22)" {
				t.Errorf("the GM's camp: %q", s)
			}
		}
		if strings.HasPrefix(s, "Child of Xrag, pariah") {
			t.Errorf("the camp on Xrag's rock is listed apart from the xrag camp: %q", s)
		}
	}
	if n != 1 || len(trips) != len(teleRaids)+len(telePlaces)+len(campDests)+1 {
		t.Errorf("%d trips, %d of the GM's frost bats: %q", len(trips), n, trips)
	}
	// the camps in their levels' order
	lv := func(s string) int {
		i := strings.LastIndex(s, "(")
		lo, _ := parseLevels(strings.TrimSuffix(s[i+1:], ")"))
		return lo
	}
	camps := trips[len(teleRaids)+len(telePlaces):]
	if !sort.SliceIsSorted(camps, func(i, j int) bool { return lv(camps[i]) < lv(camps[j]) }) {
		t.Errorf("the camps out of their levels' order: %q", camps)
	}
}

// pickPath answers each box of the path from a talk, by its answer's text.
func pickPath(t *testing.T, r *portalRig, tp *entity.NPC, path ...string) []string {
	t.Helper()
	r.drain()
	r.talk(r.anna, tp)
	got := r.msgs(r.anna)
	for _, want := range path {
		box := boxID(got)
		if box&0xFF000000 != teleporterBoxMark {
			t.Fatalf("before %q: %q", want, got)
		}
		i := strings.Index(got[len(got)-1], ":")
		opts := strings.Split(got[len(got)-1][i+1:], "|")[1:]
		pick := -1
		for j, o := range opts {
			if o == want {
				pick = j
			}
		}
		if pick < 0 {
			t.Fatalf("no %q in %q", want, opts)
		}
		r.pick(r.anna, box, byte(pick))
		got = r.msgs(r.anna)
	}
	return got
}

// A raid's trip lands at its way in facing its boss, a waysender's way: "Very
// well." and the warp; one by an NPC lands in front of it, in its world; a
// camp's lands beside its creatures (with no ground files, on its anchor).
func TestTeleporterTrips(t *testing.T) {
	r, _, tp := teleRig(t)
	got := pickPath(t, r, tp, "Raids", "Tunaria and the Isle of Dread", "Lady Vox, Permafrost (60)")
	if !hasMsg(got, "chat:Teleporter: Very well.") || !hasMsg(got, "teleport") {
		t.Fatalf("Lady Vox's trip: %q", got)
	}
	if r.pa.World != 0 || r.pa.X != 4860 || r.pa.Y != 61.62 || r.pa.Z != 4765 {
		t.Errorf("at Lady Vox's: world %d (%.2f, %.2f, %.2f)", r.pa.World, r.pa.X, r.pa.Y, r.pa.Z)
	}
	// facing her row, 100 east
	if math.Abs(float64(r.pa.Facing)-math.Pi/2) > 0.01 {
		t.Errorf("facing %.3f, not her (pi/2)", r.pa.Facing)
	}

	back := func() { r.gw.UpdatePosition(r.pa.ServerID, 5000, 50, 5000, 0); r.pa.World = 0 }
	back()
	pickPath(t, r, tp, "Raids", "Odus, Lavastorm and the Plane of Sky", "Lord Nagafen, Lavastorm (60)")
	if r.pa.World != 3 || math.Abs(float64(r.pa.Z-4426.88)) > 0.01 || math.Abs(float64(r.pa.X-4867.64)) > 0.01 {
		t.Errorf("by the Nagafen Teleport Arrive: world %d (%.2f, %.2f, %.2f)", r.pa.World, r.pa.X, r.pa.Y, r.pa.Z)
	}

	// the Isle of Dread (the owner, 2026-11-11): on Norach's raft, facing the
	// pier, for nothing
	back()
	tunar := r.pa.Tunar
	got = pickPath(t, r, tp, "Isle of Dread")
	if !hasMsg(got, "teleport") || r.pa.World != 0 || r.pa.X != 24578 || r.pa.Y != 52.6 || r.pa.Z != 3626 ||
		math.Abs(float64(r.pa.Facing)-3.9) > 0.01 || r.pa.Tunar != tunar {
		t.Errorf("the Isle of Dread: %q, world %d (%.2f, %.2f, %.2f) facing %.2f, tunar %d -> %d", got, r.pa.World,
			r.pa.X, r.pa.Y, r.pa.Z, r.pa.Facing, tunar, r.pa.Tunar)
	}

	// the Plane of Sky (petition 1605, Jibrish): where he stood, for nothing
	back()
	got = pickPath(t, r, tp, "Plane of Sky")
	if !hasMsg(got, "teleport") || r.pa.World != 4 || r.pa.X != 5000.09 || r.pa.Z != 4998.23 ||
		math.Abs(float64(r.pa.Facing)-3.068) > 0.01 || r.pa.Tunar != tunar {
		t.Errorf("the Plane of Sky: %q, world %d (%.2f, %.2f, %.2f) facing %.2f", got, r.pa.World,
			r.pa.X, r.pa.Y, r.pa.Z, r.pa.Facing)
	}

	back()
	got = pickPath(t, r, tp, "Camps, levels 1-25", "Levels 1-9", "Hatebone orcs, Halas (1-5)")
	d := campDests["hatebone"]
	if !hasMsg(got, "teleport") || r.pa.World != 0 || r.pa.X != d.X || r.pa.Z != d.Z {
		t.Errorf("the hatebone camp: %q, world %d (%.1f, %.1f)", got, r.pa.World, r.pa.X, r.pa.Z)
	}

	back()
	got = pickPath(t, r, tp, "Camps, levels 1-25", "Levels 20-25", "Frost bat, ice bat (20-22)")
	if !hasMsg(got, "teleport") || r.pa.World != 0 || math.Hypot(float64(r.pa.X-9000), float64(r.pa.Z-9000)) > 20 {
		t.Errorf("the GM's camp: %q, at (%.1f, %.1f)", got, r.pa.X, r.pa.Z)
	}
}

// The controls: off, it only says so; a raid whose boss is gone and a camp
// whose creatures are gone are not offered (after the list is read again); a
// pick from afar, an old box and a close go nowhere; another name is not it.
func TestTeleporterRefuses(t *testing.T) {
	r, _, tp := teleRig(t)
	SetTeleporter(false)
	r.drain()
	r.talk(r.anna, tp)
	if got := r.msgs(r.anna); len(got) != 1 || got[0] != "box:Teleporter: My portals are closed on this server." {
		t.Errorf("off: %q", got)
	}
	SetTeleporter(true)

	// from afar
	r.drain()
	r.talk(r.anna, tp)
	box := boxID(r.msgs(r.anna))
	r.gw.UpdatePosition(r.pa.ServerID, 5100, 50, 5000, 0)
	r.pick(r.anna, box, 0)
	if got := r.msgs(r.anna); len(got) != 1 || got[0] != "box:Teleporter: Come back to me to go: my portals open beside me." {
		t.Errorf("from afar: %q", got)
	}
	r.gw.UpdatePosition(r.pa.ServerID, 5000, 50, 5000, 0)
	// an old box, then a close
	r.pick(r.anna, box, 0)
	if got := r.msgs(r.anna); len(got) != 0 {
		t.Errorf("an old box: %q", got)
	}
	r.talk(r.anna, tp)
	box = boxID(r.msgs(r.anna))
	r.pick(r.anna, box, 255)
	if got := r.msgs(r.anna); len(got) != 0 {
		t.Errorf("a close: %q", got)
	}
	r.pick(r.anna, box, 0)
	if got := r.msgs(r.anna); boxID(got)&0xFF000000 != teleporterBoxMark || !strings.Contains(got[0], "|Tunaria and the Isle of Dread|") {
		t.Errorf("the pick after a close: %q", got)
	}

	// Lady Vox and the hatebone guards gone: read again after a minute
	var vox, guard entity.EntityID
	for id, n := range r.gw.NPCs {
		switch n.Name {
		case "Lady Vox":
			vox = id
		case campDests["hatebone"].NPC:
			guard = id
		}
	}
	r.gw.RemoveNPC(vox)
	r.gw.RemoveNPC(guard)
	has := func(want string) bool {
		for _, s := range walkTele(t, r.ih.tele.tree(r.gw, r.anna), "first") {
			if strings.HasPrefix(s, want) {
				return true
			}
		}
		return false
	}
	if !has("Lady Vox") || !has("Hatebone orcs, Halas") {
		t.Errorf("within the minute the list is kept as read")
	}
	r.now = r.now.Add(teleCatalogFor)
	if has("Lady Vox") || has("Hatebone orcs, Halas") {
		t.Errorf("a raid with no boss, or a camp with no creatures, is offered")
	}
	if !has("Siliskor") || !has("Hatebone orcs 2, Halas") {
		t.Errorf("the others went with them")
	}

	other := &entity.NPC{ServerID: 9999, Name: "Teleporters", X: 5001, Y: 50, Z: 5000, Level: 50, HP: 100, MaxHP: 100, NPCType: 0x80}
	r.gw.NPCs[other.ServerID] = other
	r.drain()
	r.talk(r.anna, other)
	if got := r.msgs(r.anna); boxID(got)&0xFF000000 == teleporterBoxMark {
		t.Errorf("another name asked its question: %q", got)
	}
}

// levelFold: every box at most six answers, the camps in order, all of them,
// for any count; a cut where the levels change where one does as well.
func TestLevelFoldShapes(t *testing.T) {
	rnd := rand.New(rand.NewSource(1))
	for n := 1; n <= 180; n++ {
		nodes := make([]*teleNode, n)
		for i := range nodes {
			lv := 1 + rnd.Intn(66)
			nodes[i] = &teleNode{label: "c", low: lv, high: lv, spot: &teleSpot{}}
		}
		sort.Slice(nodes, func(i, j int) bool { return nodes[i].low < nodes[j].low })
		parts := levelFold(append([]*teleNode(nil), nodes...), 5)
		if len(parts) > 5 {
			t.Fatalf("%d camps: %d answers at first", n, len(parts))
		}
		var leaves []*teleNode
		var visit func(ns []*teleNode)
		visit = func(ns []*teleNode) {
			for _, p := range ns {
				if p.spot != nil {
					leaves = append(leaves, p)
					continue
				}
				if len(p.kids) == 0 || len(p.kids) > maxDialogOptions {
					t.Fatalf("%d camps: a box of %d", n, len(p.kids))
				}
				visit(p.kids)
			}
		}
		visit(parts)
		if len(leaves) != n {
			t.Fatalf("%d camps: %d reached", n, len(leaves))
		}
		for i := range leaves {
			if leaves[i] != nodes[i] {
				t.Fatalf("%d camps: out of order at %d", n, i)
			}
		}
	}
	lv := []int{1, 1, 1, 2, 2, 2, 3, 3, 3, 3}
	var nodes []*teleNode
	for _, l := range lv {
		nodes = append(nodes, &teleNode{low: l, high: l, spot: &teleSpot{}})
	}
	parts := levelParts(nodes, maxDialogOptions)
	if len(parts) != 2 || len(parts[0].kids) != 6 || parts[0].label != "Levels 1-2" || parts[1].label != "Level 3" {
		t.Errorf("the cut: %d parts, %+v", len(parts), parts)
	}
}

// Its words fit their boxes.
func TestTeleporterWordsFit(t *testing.T) {
	for _, q := range []string{teleAskRoot, teleAskRaids, teleAskRaid, teleAskLevels, teleAskCamp} {
		if n := len([]rune(q)); n > questAskChars {
			t.Errorf("%q is %d characters, a question shows %d", q, n, questAskChars)
		}
	}
	for g, name := range teleRaidGroups {
		n := 0
		for _, rd := range teleRaids {
			if rd.group == g {
				n++
			}
		}
		if n == 0 || n > maxDialogOptions {
			t.Errorf("%s holds %d raids", name, n)
		}
	}
	for k, name := range teleCampNames {
		if _, ok := campDests[k]; !ok {
			t.Errorf("%s names no #tele camp", k)
		}
		if l := len([]rune(withLevels(name, 10, 20))); l > 40 {
			t.Errorf("%q is %d characters", name, l)
		}
	}
	for k := range campDests {
		if teleCampNames[k] == "" {
			t.Errorf("the camp %s has no name of the Teleporter's", k)
		}
	}
}

// A grouped player's trip takes the group (petition 1408: "should take the
// group at same time to same spot"): a member by the Teleporter goes at once
// to the picker's very landing, facing as the picker does, told who took them;
// a member away from it, and a dead one, stay where they are. A camp's landing
// is worked out from its creatures, and still one for both.
func TestTeleporterTakesTheGroup(t *testing.T) {
	r, _, tp := teleRig(t)
	r.gw.GroupInvite(r.anna, r.gw.IDSeenBy(r.anna, r.pc))
	r.gw.GroupAccept(r.cora, r.gw.IDSeenBy(r.cora, r.pa))
	r.gw.GroupInvite(r.anna, r.gw.IDSeenBy(r.anna, r.pb))
	r.gw.GroupAccept(r.bert, r.gw.IDSeenBy(r.bert, r.pa))
	if n := len(r.gw.GroupMembers(r.pa)); n != 3 {
		t.Fatalf("the group: %d members", n)
	}
	got := pickPath(t, r, tp, "Raids", "Tunaria and the Isle of Dread", "Lady Vox, Permafrost (60)")
	if !hasMsg(got, "teleport") || r.pa.X != 4860 || r.pa.Z != 4765 {
		t.Fatalf("Anna's trip: %q at (%.1f, %.1f)", got, r.pa.X, r.pa.Z)
	}
	cora := r.msgs(r.cora)
	if !hasMsg(cora, "chat:Teleporter: Anna takes the group to Lady Vox, Permafrost (60).") || !hasMsg(cora, "teleport") {
		t.Errorf("Cora was not taken along: %q", cora)
	}
	if r.pc.World != r.pa.World || r.pc.X != r.pa.X || r.pc.Y != r.pa.Y || r.pc.Z != r.pa.Z || r.pc.Facing != r.pa.Facing {
		t.Errorf("Cora at world %d (%.2f, %.2f, %.2f) facing %.3f, Anna at world %d (%.2f, %.2f, %.2f) facing %.3f",
			r.pc.World, r.pc.X, r.pc.Y, r.pc.Z, r.pc.Facing, r.pa.World, r.pa.X, r.pa.Y, r.pa.Z, r.pa.Facing)
	}
	if bert := r.msgs(r.bert); hasMsg(bert, "teleport") || r.pb.X != 20000 || r.pb.Z != 20000 {
		t.Errorf("Bert, far from the Teleporter, was moved: %q at (%.1f, %.1f)", bert, r.pb.X, r.pb.Z)
	}

	// both back by the Teleporter: a camp, the same spot for both
	for _, p := range []*entity.Player{r.pa, r.pc} {
		r.gw.UpdatePosition(p.ServerID, 5000, 50, 5000, 0)
		p.World = 0
	}
	r.drain()
	pickPath(t, r, tp, "Camps, levels 1-25", "Levels 20-25", "Frost bat, ice bat (20-22)")
	if math.Hypot(float64(r.pa.X-9000), float64(r.pa.Z-9000)) > 20 || r.pc.X != r.pa.X || r.pc.Z != r.pa.Z {
		t.Errorf("the GM's camp: Anna at (%.1f, %.1f), Cora at (%.1f, %.1f)", r.pa.X, r.pa.Z, r.pc.X, r.pc.Z)
	}

	// a dead member stays
	for _, p := range []*entity.Player{r.pa, r.pc} {
		r.gw.UpdatePosition(p.ServerID, 5000, 50, 5000, 0)
		p.World = 0
	}
	r.pc.HP = 0
	r.drain()
	pickPath(t, r, tp, "Raids", "Tunaria and the Isle of Dread", "Lady Vox, Permafrost (60)")
	if cora := r.msgs(r.cora); hasMsg(cora, "teleport") || r.pc.X != 5000 || r.pc.Z != 5000 {
		t.Errorf("dead Cora was moved: %q at (%.1f, %.1f)", cora, r.pc.X, r.pc.Z)
	}
}
