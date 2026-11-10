package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// A petition's spawn request made a camp report (the owner, 2026-11-10: "lets
// require GM approval"): petitions (bug reports, support.go) ask for spawns
// too, "kappa guard spawns here", "bandit camp not populated", and were
// applied by spawning creatures where the player stood, unreviewed. Now a GM
// (the panel's campfix_from_petition, a button on the petition's page) or a
// session on the box (cmd/campfixfile, no panel token) files the request as a
// waiting campfix_reports row, which waits for a GM's approval on the Camp
// reports page like any other; nothing spawns before that.
//
// The mark is the petition's character's place and facing (its server view,
// petitionview.go: where it stood when it filed); the kind is the filer's
// (add, move, remove, turn), the creature matched as /campfix matches a name
// (campfixMatch over the world's NPC rows); an add of several puts each mark
// on rings round the place (campfixRing), none within campfixApart of
// another. A move's, a turn's and a removal's spawn is the petition's target
// when it is one of the name, else the nearest of the name (as /mark and
// /markremove reach). The report is the petition's player's (its account,
// character, level and class); its note names the petition. It records the
// NPC rows near and flags what the database alone can (a name of few spawns,
// none near, a level off the median, a long move); the world's own checks
// (what attacks on sight, the ground, the water) are the review's, made when a
// GM looks at it. It reads the database alone, so the panel's button and the
// command on the box file the same report.

// CampfixPetition is a petition's spawn request as it is filed: the petition,
// what its mark asks ("add", "move", "remove", "turn"), the creature, how many
// for an add (0: one), and a note.
type CampfixPetition struct {
	Petition int64
	Kind     string
	Name     string
	Count    int
	Note     string
}

// CampfixKinds are the kinds of mark, as a petition's filing takes them.
var CampfixKinds = []string{campfixAdd, campfixMove, campfixRemove, campfixTurn}

// CampfixPetitionPlan is the report a petition's request files: its row (the
// marks, the creatures near and the flags in it as JSON), each read back, and
// the lines saying how the name was taken.
type CampfixPetitionPlan struct {
	Report database.CampfixReport
	Marks  []CampfixMarkRecord
	Near   []CampfixNearRecord
	Flags  []CampfixFlagRecord
	Said   []string
	who    campfixFiler
	camps  map[int]bool
}

// campfixPetitionView is what a filing reads of a petition's server view
// (petitionview.go's serverView): when, the character's place and facing, and
// its target.
type campfixPetitionView struct {
	Filed     string `json:"filed"`
	Character struct {
		Name     string  `json:"name"`
		ServerID uint32  `json:"serverid"`
		Level    int     `json:"level"`
		Class    string  `json:"class"`
		World    int     `json:"world"`
		X        float64 `json:"x"`
		Y        float64 `json:"y"`
		Z        float64 `json:"z"`
		Facing   float64 `json:"facing"`
	} `json:"character"`
	Target *struct {
		Kind  string `json:"kind"`
		Name  string `json:"name"`
		NPCID int    `json:"npcid"`
	} `json:"target"`
}

// campfixRing is where an add's mark i (from 0) stands round a petition's
// place: the place itself, then rings 3 apart holding 6, 12, ... marks, so no
// two marks lie within campfixApart of each other (the nearest are 3 apart).
func campfixRing(i int) (dx, dz float32) {
	if i <= 0 {
		return 0, 0
	}
	ring, k := 1, i-1
	for k >= 6*ring {
		k -= 6 * ring
		ring++
	}
	a := 2 * math.Pi * float64(k) / float64(6*ring)
	r := 3 * float64(ring)
	return float32(r * math.Sin(a)), float32(r * math.Cos(a))
}

// campfixClassID is a class by its name as the server view writes it
// (entity.ClassName), -1 for none.
func campfixClassID(name string) int {
	for c := 0; c < 15; c++ {
		if strings.EqualFold(entity.ClassName(c), strings.TrimSpace(name)) {
			return c
		}
	}
	return -1
}

// PlanCampfixPetition is the report req files, from the petition's server
// view and the database as it is now, or why it cannot be filed.
func PlanCampfixPetition(db *database.SQLiteDB, req CampfixPetition, now time.Time) (CampfixPetitionPlan, error) {
	var plan CampfixPetitionPlan
	kind := strings.ToLower(strings.TrimSpace(req.Kind))
	switch kind {
	case campfixAdd, campfixMove, campfixRemove, campfixTurn:
	default:
		return plan, fmt.Errorf("kind is add, move, remove or turn, not %q", req.Kind)
	}
	count := req.Count
	switch {
	case kind == campfixAdd && count == 0:
		count = 1
	case kind == campfixAdd && (count < 1 || count > CampfixMostMarks):
		return plan, fmt.Errorf("an add's count is 1 to %d", CampfixMostMarks)
	case kind != campfixAdd && count > 1:
		return plan, fmt.Errorf("a count is an add's: a %s names one spawn", kind)
	}
	typed := strings.TrimSpace(req.Name)
	if typed == "" {
		return plan, errors.New("name the creature (as /campfix takes a name: any case, a leading a/an/the left off)")
	}
	p, ok, err := db.PetitionByID(req.Petition)
	if err != nil {
		return plan, fmt.Errorf("petitions: %w", err)
	}
	if !ok {
		return plan, fmt.Errorf("no petition %d", req.Petition)
	}
	raw, err := db.PetitionServerView(p.ID)
	if err != nil {
		return plan, fmt.Errorf("petition %d: %w", p.ID, err)
	}
	var v campfixPetitionView
	if strings.TrimSpace(raw) == "" {
		return plan, fmt.Errorf("petition %d has no server view (filed before the server kept one): there is no place to mark", p.ID)
	}
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return plan, fmt.Errorf("petition %d's server view is not readable: %v", p.ID, err)
	}
	c := v.Character
	if c.Name == "" && c.ServerID == 0 || c.World < 0 || c.World > 255 {
		return plan, fmt.Errorf("petition %d's server view has no character in the world: there is no place to mark", p.ID)
	}
	world := uint8(c.World)
	filed, err := time.Parse(time.RFC3339, v.Filed)
	if err != nil {
		filed, _ = time.Parse(time.RFC3339, p.Filed)
	}
	at := filed.UTC().Format(time.RFC3339)
	if filed.IsZero() {
		at = ""
	}

	spawnOnly, err := db.NPCSpawnOnly()
	if err != nil {
		return plan, fmt.Errorf("npcspawnonly: %w", err)
	}
	rows, err := loadCampfixRows(db, world, spawnOnly)
	if err != nil {
		return plan, err
	}
	x, y, z, facing := float32(c.X), float32(c.Y), float32(c.Z), float32(c.Facing)
	name, _, said, why := campfixMatch(typed, func(names []string) ([]entity.CampfixNPC, []entity.CampfixName) {
		return rows.scan(x, y, z, campfixSpan, names)
	})
	plan.Said = said
	if why != "" {
		return plan, errors.New(why)
	}
	key := campfixKey(name)
	who := c.Name
	if who == "" {
		who = p.CharName
	}

	mark := func(kind string, mx, my, mz float32) CampfixMarkRecord {
		return CampfixMarkRecord{Kind: kind, Typed: typed, Name: name, World: int(world), X: campfix2(mx), Y: campfix2(my),
			Z: campfix2(mz), Facing: math.Round(float64(facing)*10000) / 10000, FacingByte: facingByte(float64(facing)), At: at}
	}
	from := func(m *CampfixMarkRecord, s campfixRowNPC, mx, my, mz float32) {
		m.FromX, m.FromY, m.FromZ, m.FromFacing = campfix2(s.X), campfix2(s.Y), campfix2(s.Z), facingByte(float64(s.Facing))
		m.Level = s.Level
		m.Distance = campfix2(campfixDist(s.X, s.Y, s.Z, mx, my, mz))
	}
	// the spawn a move, a turn or a removal names: the target when it is one of
	// the name within reach, else the nearest of the name within stand
	subject := func(reach, stand float32) (campfixRowNPC, bool) {
		if t := v.Target; t != nil && t.NPCID != 0 {
			if r, ok := rows.row(entity.EntityID(t.NPCID)); ok && campfixKey(r.Name) == key {
				if d := campfixDist(r.X, r.Y, r.Z, x, y, z); d <= reach {
					r.Dist = d
					return r, true
				}
			}
		}
		if r, ok := rows.nearestNamed(name, x, y, z); ok && r.Dist <= stand {
			return r, true
		}
		return campfixRowNPC{}, false
	}
	switch kind {
	case campfixAdd:
		for i := 0; i < count; i++ {
			dx, dz := campfixRing(i)
			mx, mz := x+dx, z+dz
			m := mark(campfixAdd, mx, y, mz)
			if t, ok := rows.nearestNamed(name, mx, y, mz); ok {
				m.Template = int(t.ID)
				from(&m, t, mx, y, mz)
			}
			plan.Marks = append(plan.Marks, m)
		}
	case campfixMove, campfixTurn:
		s, ok := subject(campfixSpan, campfixSpan)
		if !ok {
			return plan, fmt.Errorf("no %s spawns within %.0f of where %s stood: there is none to %s (an add makes one)",
				name, campfixSpan, who, kind)
		}
		k := kind
		if k == campfixMove && s.Dist <= campfixApart { // standing on it: a turn of it, as /mark makes one
			k = campfixTurn
		}
		m := mark(k, x, y, z)
		m.Row = int(s.ID)
		from(&m, s, x, y, z)
		plan.Marks = append(plan.Marks, m)
	case campfixRemove:
		s, ok := subject(campfixRemoveTarget, campfixRemoveStand)
		if !ok {
			return plan, fmt.Errorf("no %s spawns within %.0f of where %s stood, and its target was not one within %.0f: "+
				"there is none to remove", name, campfixRemoveStand, who, campfixRemoveTarget)
		}
		m := mark(campfixRemove, x, y, z)
		m.Row = int(s.ID)
		from(&m, s, x, y, z)
		plan.Marks = append(plan.Marks, m)
	}

	// the creatures near, and what the database alone flags
	camps, err := db.CampSpawns()
	if err != nil {
		return plan, fmt.Errorf("campspawns: %w", err)
	}
	campOf := map[int]database.CampSpawnRow{}
	plan.camps = map[int]bool{}
	for _, cs := range camps {
		campOf[cs.NPCID] = cs
		plan.camps[cs.NPCID] = true
	}
	pts := make([][3]float32, len(plan.Marks))
	for i, m := range plan.Marks {
		pts[i] = [3]float32{float32(m.X), float32(m.Y), float32(m.Z)}
	}
	for _, n := range rows.near(pts, campfixNearReach, campfixNearMost) {
		j := CampfixNearRecord{Row: int(n.ID), Name: n.Name, Level: n.Level, X: campfix2(n.X), Y: campfix2(n.Y), Z: campfix2(n.Z),
			FacingByte: facingByte(float64(n.Facing)), SameName: campfixKey(n.Name) == key, Distance: campfix2(n.Dist)}
		if cs, ok := campOf[int(n.ID)]; ok {
			j.Camp, j.Kinds = cs.Camp, cs.Kinds
		}
		plan.Near = append(plan.Near, j)
	}
	nearName := false
	for _, n := range rows.near(pts, campfixSpan, 0) {
		if campfixKey(n.Name) == key {
			nearName = true
			break
		}
	}
	inWorld := len(rows.named(name))
	median, haveMedian := campfixMedian(plan.Near)
	flag := func(i int, kind, text string) {
		plan.Flags = append(plan.Flags, CampfixFlagRecord{Mark: i + 1, Flag: kind, Text: text})
	}
	for i, m := range plan.Marks {
		if inWorld <= campfixFew {
			flag(i, "few", campfixFewText(name, inWorld, world))
		}
		if !nearName {
			flag(i, "none_near", campfixNoneNearText(name))
		}
		subjectLevel := m.Level
		if (m.Row != 0 || m.Template != 0) && haveMedian && math.Abs(float64(subjectLevel)-median) > campfixLevelOff {
			flag(i, "level", campfixLevelText(name, subjectLevel, median))
		}
		if m.Kind == campfixMove && m.Distance > campfixFarMove {
			flag(i, "far_move", fmt.Sprintf("a move of %d", int(math.Round(m.Distance))))
		}
	}

	note := fmt.Sprintf("From petition %d.", p.ID)
	if n := strings.TrimSpace(req.Note); n != "" {
		note = fmt.Sprintf("From petition %d: %s", p.ID, n)
	}
	note, _ = campfixCleanNote(note)
	class := -1
	var tclass int
	if err := db.QueryRow(`SELECT tclass FROM characters WHERE serverid = ?`, p.ServerID).Scan(&tclass); err == nil {
		class = tclass
	} else {
		class = campfixClassID(c.Class)
	}
	class = max(class, 0)
	first := plan.Marks[0]
	plan.Report = database.CampfixReport{AccountID: p.AccountID, CharName: p.CharName, Level: c.Level, Class: class,
		World: int(world), X: first.X, Z: first.Z, Note: note,
		Marks: campfixJSON(plan.Marks), Near: campfixJSON(plan.Near), Flags: campfixJSON(plan.Flags)}
	plan.who = campfixFiler{Name: p.CharName, Level: c.Level, Class: class, CharID: p.ServerID, AccountID: p.AccountID,
		Petition: p.ID, PetitionFiled: filed}
	return plan, nil
}

// File keeps the plan as a waiting report and posts its work board entry, as
// filedBy (the GM or the session making it a report: the entry's poster and
// the details' "made a camp report by"). It returns the report and the entry
// as kept.
func (plan CampfixPetitionPlan) File(db *database.SQLiteDB, filedBy database.GMEdit, now time.Time) (database.CampfixReport, database.WorkEntry, error) {
	if len(plan.Marks) == 0 {
		return database.CampfixReport{}, database.WorkEntry{}, errors.New("no mark to file")
	}
	who := plan.who
	who.FiledBy = database.WorkByName(filedBy)
	post := func(id int64) database.WorkPost {
		return campfixPost(nil, id, who, plan.Marks, plan.Report.Note, plan.Near, plan.Flags, plan.camps, now)
	}
	return db.FileCampfixReport(plan.Report, now, post, filedBy)
}

// campfixFewText, campfixNoneNearText and campfixLevelText are the flags'
// words, the sending's and a petition's alike.
func campfixFewText(name string, n int, world uint8) string {
	return fmt.Sprintf("%s has %d %s in %s: few in the world: named or rare?", name, n, plural(n, "spawn", "spawns"), worldName(world))
}

func campfixNoneNearText(name string) string {
	return fmt.Sprintf("%s had no spawn within %.0f of the marks", name, campfixSpan)
}

func campfixLevelText(name string, level int, median float64) string {
	return fmt.Sprintf("%s is level %d, %s from the median level %s of the creatures nearby", name, level,
		campfixNum(math.Abs(float64(level)-median)), campfixNum(median))
}
