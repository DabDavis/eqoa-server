package handlers

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/worlds"
)

// Every feature that asks a question box (0x0034) marks its boxes' ids with a high byte of its own, and an answer
// (0x0035, the id echoed) goes to the first feature in HandleDialogueOption whose mark it carries: two features under
// one mark have the first take the other's answers and drop them as old (the Mercenary Liaison and the PvP Events
// herald were both 0x5F000000 when their branches met, and every answer to the herald vanished). A feature's mark is
// a const named <feature>BoxMark; boxMarks is all of them, and declaredBoxMarks keeps it whole.

// boxMarks is every feature's mark by its const's name.
var boxMarks = map[string]uint32{
	"questBoxMark":      questBoxMark,
	"triggerBoxMark":    triggerBoxMark,
	"duelBoxMark":       entity.DuelBoxID(0), // entity/duelwire.go's, unexported there
	"rehabBoxMark":      rehabBoxMark,
	"advisorBoxMark":    advisorBoxMark,
	"dyerBoxMark":       dyerBoxMark,
	"coachBoxMark":      coachBoxMark,
	"genieBoxMark":      genieBoxMark,
	"boardBoxMark":      boardBoxMark,
	"portalBoxMark":     portalBoxMark,
	"moogieBoxMark":     moogieBoxMark,
	"dabrishBoxMark":    dabrishBoxMark,
	"archivistBoxMark":  archivistBoxMark,
	"teleporterBoxMark": teleporterBoxMark,
	"mercBoxMark":       mercBoxMark,
	"welcomeBoxMark":    welcomeBoxMark,
	"ctfBoxMark":        ctfBoxMark,
	"heroCallBoxMark":   heroCallBoxMark,
	"heroesBoxMark":     heroesBoxMark,
}

// boxMarkClashes says what is wrong with a set of marks: two of one high byte, a mark with any of the low 24 bits
// (each feature's sequence and flags live there, and every answer is told by id&0xFF000000), and a mark of high
// byte 0 (the plain boxes' id: sendOptionBox's travel lists, the technician's menu, a spiritmaster's question).
func boxMarkClashes(marks map[string]uint32) []string {
	names := make([]string, 0, len(marks))
	for n := range marks {
		names = append(names, n)
	}
	sort.Strings(names)
	var bad []string
	byHigh := map[uint32]string{}
	for _, n := range names {
		m := marks[n]
		if m&0x00FFFFFF != 0 {
			bad = append(bad, fmt.Sprintf("%s %08X has low bits", n, m))
		}
		if m&0xFF000000 == 0 {
			bad = append(bad, fmt.Sprintf("%s %08X is a plain box's id", n, m))
		}
		if other, ok := byHigh[m&0xFF000000]; ok {
			bad = append(bad, fmt.Sprintf("%s and %s are both %08X", other, n, m&0xFF000000))
			continue
		}
		byHigh[m&0xFF000000] = n
	}
	return bad
}

// declaredBoxMarks reads the non-test Go files of dir for constants named <feature>BoxMark, with the value of each
// declared as a plain number (-1 for one that is not).
func declaredBoxMarks(dir string) (map[string]int64, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}
	got := map[string]int64{}
	fset := token.NewFileSet()
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		for _, d := range f.Decls {
			g, ok := d.(*ast.GenDecl)
			if !ok || g.Tok != token.CONST {
				continue
			}
			for _, s := range g.Specs {
				vs := s.(*ast.ValueSpec)
				for i, name := range vs.Names {
					if !strings.HasSuffix(name.Name, "BoxMark") {
						continue
					}
					v := int64(-1)
					if i < len(vs.Values) {
						if lit, ok := vs.Values[i].(*ast.BasicLit); ok && lit.Kind == token.INT {
							if n, err := strconv.ParseUint(lit.Value, 0, 32); err == nil {
								v = int64(n)
							}
						}
					}
					got[name.Name] = v
				}
			}
		}
	}
	return got, nil
}

// No two features share a mark, none has low bits or is a plain box's, and boxMarks holds every <feature>BoxMark the
// handlers and the entities declare, at the value each is declared (so a new feature's mark is checked with the
// rest). The controls: the herald back under the liaison's 0x5F000000 is a clash named as such, a mark with low bits
// and one of high byte 0 are told, and the scan finds a mark declared in a file of its own.
func TestBoxMarksAreEachFeaturesOwn(t *testing.T) {
	if bad := boxMarkClashes(boxMarks); len(bad) != 0 {
		t.Errorf("the box marks: %q", bad)
	}
	if ctfBoxMark != 0x62000000 {
		t.Errorf("the herald's mark is %08X, 0x62000000 by the CTF deploy", uint32(ctfBoxMark))
	}
	declared := map[string]int64{}
	for _, dir := range []string{".", filepath.Join("..", "entity")} {
		got, err := declaredBoxMarks(dir)
		if err != nil {
			t.Fatal(err)
		}
		for n, v := range got {
			declared[n] = v
		}
	}
	for n, v := range declared {
		m, ok := boxMarks[n]
		switch {
		case !ok:
			t.Errorf("%s is declared but not in boxMarks: add it there", n)
		case v >= 0 && uint32(v) != m:
			t.Errorf("%s is declared %08X, boxMarks has %08X", n, v, m)
		}
	}
	for n := range boxMarks {
		if _, ok := declared[n]; !ok {
			t.Errorf("boxMarks has %s, which nothing declares", n)
		}
	}

	// the controls
	shared := map[string]uint32{}
	for n, m := range boxMarks {
		shared[n] = m
	}
	shared["ctfBoxMark"] = mercBoxMark
	if bad := boxMarkClashes(shared); len(bad) != 1 || bad[0] != "ctfBoxMark and mercBoxMark are both 5F000000" {
		t.Errorf("the herald under the liaison's mark: %q", bad)
	}
	if bad := boxMarkClashes(map[string]uint32{"lowBoxMark": 0x63000001, "plainBoxMark": 0x00000000}); len(bad) != 2 ||
		!strings.Contains(bad[0], "has low bits") || !strings.Contains(bad[1], "plain box's id") {
		t.Errorf("a mark with low bits and a plain one: %q", bad)
	}
	dir := t.TempDir()
	src := "package handlers\n\nconst (\n\tfooBoxMark = 0x63000000\n\tfooBoxMarkOther = 2\n)\n\nconst barBoxMark = fooBoxMark\n"
	if err := os.WriteFile(filepath.Join(dir, "foo.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := declaredBoxMarks(dir); err != nil || len(got) != 2 || got["fooBoxMark"] != 0x63000000 || got["barBoxMark"] != -1 {
		t.Errorf("a file of its own: %v %v", got, err)
	}
}

// A player asked by the Mercenary Liaison and by the PvP Events herald at once answers each, through
// HandleDialogueOption, and each answer is its own feature's: the herald's signs up and leaves the liaison's
// question standing, then the liaison's goes on to its next box and signs nothing. The control: the herald's box id
// under the liaison's high byte (which the liaison never asked) signs nothing up, so it is the mark that routes the
// answer.
func TestHeraldAndLiaisonAnswersAreTheirOwn(t *testing.T) {
	r, _, _ := newMercFixture(t)
	SetServerWorlds([]worlds.Built{{Recipe: worlds.Recipe{World: 11, Name: "the Test Canyon"}}})
	t.Cleanup(func() { SetServerWorlds(nil) })
	lay := entity.CTFLayout{Name: "the Test Canyon", Teams: [2]entity.CTFTeamLayout{
		{Name: "the Ember Band", Short: "Ember", Flag: "Ember Banner", Body: 1, Stand: entity.CTFSpot{X: 100, Z: 100}, Camp: entity.CTFSpot{X: 100, Z: 150}},
		{Name: "the Dusk Band", Short: "Dusk", Flag: "Dusk Banner", Body: 2, Stand: entity.CTFSpot{X: 900, Z: 100}, Camp: entity.CTFSpot{X: 900, Z: 150}}}}
	r.gw.SetCTF(true, entity.DefaultCTFSettings(), lay)
	herald := ctfHerald(t, r)
	r.anna.SetServerWorlds([]uint8{11})
	lia := r.liaison()
	r.drain()

	r.talk(r.anna, lia)
	liaBox := boxID(r.msgs(r.anna))
	r.talk(r.anna, herald) // asked twice: the second box stands, its sequence past the liaison's
	r.msgs(r.anna)
	r.talk(r.anna, herald)
	heraldBox := boxID(r.msgs(r.anna))
	if !IsMercBox(liaBox) || heraldBox&0xFF000000 != ctfBoxMark {
		t.Fatalf("the boxes: %08X %08X", liaBox, heraldBox)
	}

	// the control first: the herald's id under the liaison's mark, which is not the liaison's own box
	control := mercBoxMark | heraldBox&0x00FFFFFF
	if control == liaBox {
		t.Fatalf("the control %08X is the liaison's own box", control)
	}
	r.pick(r.anna, control, 0)
	if got := r.msgs(r.anna); len(got) != 0 || r.gw.CTFQueued(r.anna) {
		t.Fatalf("an answer under the liaison's mark signed up: %q", got)
	}
	r.pick(r.anna, heraldBox, 0) // "Sign me up"
	if got := r.msgs(r.anna); len(got) != 1 || !strings.Contains(got[0], "You are signed up") || !r.gw.CTFQueued(r.anna) {
		t.Fatalf("the herald's answer: %q", got)
	}
	r.pick(r.anna, liaBox, 1) // "A tank"
	got := r.msgs(r.anna)
	if len(got) != 1 || !strings.HasSuffix(got[0], "|Warrior|Paladin|Shadow Knight") {
		t.Fatalf("the liaison's answer after the herald's: %q", got)
	}
	if !r.gw.CTFQueued(r.anna) {
		t.Error("the liaison's answer took the name off the herald's list")
	}
}
