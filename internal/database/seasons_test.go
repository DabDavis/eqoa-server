package database

import (
	"math"
	"path/filepath"
	"testing"
)

// The seasons (seasons.go) and the decoration lights' bulbs (decorlights.go).

func TestASeasonTurnsAndHoldsItsNPCsWhileOff(t *testing.T) {
	db, err := NewSQLite(filepath.Join(t.TempDir(), "eqoa.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO npcseason (name, season) VALUES ('Christmas Tree', 'Christmas'), ('Harvest Scarecrow', 'Harvest')`); err != nil {
		t.Fatal(err)
	}
	held, err := db.NPCSeasonHeld()
	if err != nil || !held["christmas tree"] || !held["harvest scarecrow"] {
		t.Fatalf("no season on: held %v (%v); want both", held, err)
	}
	if changed, err := db.SetSeason("christmas", true, "dab"); err != nil || !changed {
		t.Fatalf("SetSeason on: changed %v, %v", changed, err)
	}
	if on, _ := db.SeasonActive("CHRISTMAS"); !on {
		t.Fatalf("the season is not on, by any case")
	}
	held, _ = db.NPCSeasonHeld()
	if held["christmas tree"] || !held["harvest scarecrow"] {
		t.Fatalf("Christmas on: held %v; want the scarecrow alone", held)
	}
	if changed, _ := db.SetSeason("Christmas", true, "dab"); changed {
		t.Errorf("turned on twice: changed")
	}
	if n, _ := db.NPCSeasonCount("christmas"); n != 1 {
		t.Errorf("NPCSeasonCount: %d, want 1", n)
	}
	if changed, err := db.SetSeason("Christmas", false, "zak"); err != nil || !changed {
		t.Fatalf("SetSeason off: changed %v, %v", changed, err)
	}
	all, _ := db.Seasons()
	if len(all) != 1 || all[0].Name != "christmas" || all[0].Active || all[0].By != "zak" {
		t.Errorf("Seasons: %+v; want one row, made as 'christmas', off, by zak", all)
	}
}

func TestDecorBulbsLayTheShapes(t *testing.T) {
	tmpl := LightTemplate{Name: "x", Colors: []uint32{0xFF0000, 0x00FF00, 0x0000FF}, Pattern: LightCycle, Effect: LightTwinkle, Spacing: 1}
	// a spiral: every bulb on the cone, rising, the colours in turn
	s := LightString{ID: 1, Shape: ShapeSpiral, Params: `{"r0": 4, "y0": 1, "r1": 1, "y1": 10, "turns": 4}`}
	b, err := DecorBulbs(tmpl, s)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) < 40 || len(b) > 120 {
		t.Errorf("spiral: %d bulbs, want about its length (some 70)", len(b))
	}
	for i, u := range b {
		f := (float64(u.Y) - 1) / 9
		r := math.Hypot(float64(u.X), float64(u.Z))
		if f < -0.001 || f > 1.001 || math.Abs(r-(4-3*f)) > 0.01 {
			t.Fatalf("bulb %d at (%.2f, %.2f, %.2f) is off the cone", i, u.X, u.Y, u.Z)
		}
		if int(u.Color) != i%3 {
			t.Fatalf("bulb %d colour %d, want %d (cycle)", i, u.Color, i%3)
		}
	}
	// the same string lays the same bulbs (phases too)
	b2, _ := DecorBulbs(tmpl, s)
	for i := range b {
		if b[i] != b2[i] {
			t.Fatalf("laid twice: bulb %d differs", i)
		}
	}
	// random: never one colour twice in a row
	tmpl.Pattern = LightRandom
	b, _ = DecorBulbs(tmpl, s)
	for i := 1; i < len(b); i++ {
		if b[i].Color == b[i-1].Color {
			t.Fatalf("random: bulbs %d and %d share colour %d", i-1, i, b[i].Color)
		}
	}
	// a line with its ends, sagging between posts 4 apart
	b, err = DecorBulbs(tmpl, LightString{ID: 2, Shape: ShapeLine, Params: `{"from": [0, 2, 0], "to": [8, 2, 0], "sag": 0.5, "posts": 4}`})
	if err != nil || len(b) != 9 || b[0].Y != 2 || b[4].Y != 2 || b[8].Y != 2 || math.Abs(float64(b[2].Y)-1.5) > 1e-5 {
		t.Errorf("line: %d bulbs %+v, %v", len(b), b, err)
	}
	// a box's top: its four edges once round, corners once
	b, err = DecorBulbs(tmpl, LightString{ID: 3, Shape: ShapeBox, Params: `{"min": [0, 0, 0], "max": [4, 3, 2]}`})
	if err != nil || len(b) != 12 {
		t.Errorf("box: %d bulbs (%v), want 12 round a 4 by 2 top", len(b), err)
	}
	for _, u := range b {
		if u.Y != 3 {
			t.Fatalf("box top: a bulb at y %.1f", u.Y)
		}
	}
	// the cord breaks between rings, and between the box's edges apart
	b, _ = DecorBulbs(tmpl, LightString{ID: 5, Shape: ShapeRings, Params: `{"r0": 3, "y0": 1, "r1": 1, "y1": 5, "rings": 3}`})
	breaks := 0
	for i, u := range b {
		if u.Break {
			breaks++
			if i == 0 || u.Y == b[i-1].Y {
				t.Errorf("rings: a break at bulb %d, not a ring's first", i)
			}
		}
	}
	if breaks != 2 {
		t.Errorf("rings: %d breaks, want 2 (three rings)", breaks)
	}
	b, _ = DecorBulbs(tmpl, LightString{ID: 6, Shape: ShapeBox, Params: `{"min": [0, 0, 0], "max": [2, 2, 2], "edges": "all"}`})
	breaks = 0
	for _, u := range b {
		if u.Break {
			breaks++
		}
	}
	if breaks != 5 {
		t.Errorf("box, all edges: %d breaks, want 5 (the top ring and four uprights)", breaks)
	}
	// a path: its points in order, the star where it says
	b, err = DecorBulbs(tmpl, LightString{ID: 7, Shape: ShapePath, Params: `{"points": [[1,2,3],[4,5,6]], "top": "FFE070", "star": [0, 9, 0]}`})
	if err != nil || len(b) != 2 || b[1].X != 4 {
		t.Errorf("path: %+v, %v", b, err)
	}
	if c, at, ok := DecorTopColor(LightString{Shape: ShapePath, Params: `{"points": [[1,2,3]], "top": "FFE070", "star": [0, 9, 0]}`}); !ok || c != 0xFFE070 || at != [3]float32{0, 9, 0} {
		t.Errorf("path's star: %06X at %v, %v", c, at, ok)
	}
	// alternate: odd and even half a cycle apart
	tmpl.Effect = LightAlternate
	b, _ = DecorBulbs(tmpl, LightString{ID: 4, Shape: ShapePoints, Params: `{"points": [[0,0,0],[1,0,0],[2,0,0]]}`})
	if len(b) != 3 || b[0].Phase != 0 || b[1].Phase != 128 || b[2].Phase != 0 {
		t.Errorf("alternate: %+v", b)
	}
	// what lays nothing says why
	for _, bad := range []LightString{
		{Shape: ShapeSpiral, Params: `{"r0": 4}`},
		{Shape: "star", Params: `{}`},
		{Shape: ShapeLine, Params: `{"from": [0,0,0]}`},
		{Shape: ShapeLine, Params: `{"from": [0,0,0], "to": [1000,0,0]}`, ID: 9},
		{Shape: ShapeLine, Params: `not json`},
	} {
		if b, err := DecorBulbs(tmpl, bad); err == nil {
			t.Errorf("%s %s: %d bulbs and no problem", bad.Shape, bad.Params, len(b))
		}
	}
	if c, at, ok := DecorTopColor(LightString{Shape: ShapeSpiral, Params: `{"r0": 4, "y0": 1, "r1": 1, "y1": 10, "turns": 4, "top": "FFD040"}`}); !ok || c != 0xFFD040 || at[1] <= 10 {
		t.Errorf("the star: %06X at %v, %v", c, at, ok)
	}
}

func TestLightTemplateProblems(t *testing.T) {
	if p := LightTemplateProblems("FF2020, 20D040,#3070FF", LightCycle, LightTwinkle, 1.5, 0.8, 0.3, 1, "1E3F1E", 0.06); len(p) != 0 {
		t.Errorf("a good template: %v", p)
	}
	if p := LightTemplateProblems("red", "spiral", "blink", -1, 0, 0, 9, "green", 5); len(p) != 9 {
		t.Errorf("a bad template: %d problems %v, want 7", len(p), p)
	}
}
