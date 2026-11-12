// Command disperseplan plans the spread of stacked camps and the wander and
// patrols of static creatures (the owner, 2026-11-12: "there are many zones
// with mobs not dispersed or patroling, lets fix that disperse and patrol"),
// for tools/disperse_patrol_20261112.py to apply. It reads a COPY of the
// server's database (opening it migrates its schema, as the server does) and
// the baked ground, and writes the survey and the plan; it never writes the
// database's rows.
package main

import (
	"flag"
	"log"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/ground"
)

func main() {
	dbPath := flag.String("db", "", "a COPY of the server's database")
	groundDir := flag.String("ground", "", "the baked ground (<worlds>/ground)")
	models := flag.String("models", "", "models.txt (cmd/modelbake), for the bodies' widths")
	survey := flag.String("survey", "", "write the survey (TSV) here")
	out := flag.String("out", "", "write the plan (JSON) here")
	flag.StringVar(&gmFile, "gm", "", "the GMs' camps (TSV: world x z petition)")
	flag.Parse()
	if *dbPath == "" || *groundDir == "" {
		log.Fatal("-db and -ground are needed")
	}
	w := loadWorld(*dbPath, *groundDir, *models)
	rows := w.DisperseSurvey()
	if *survey != "" {
		writeSurvey(*survey, rows)
	}
	if *out != "" {
		db, err := database.NewSQLite(*dbPath)
		if err != nil {
			log.Fatal(err)
		}
		p := plan(w, db, rows)
		writePlan(*out, p)
	}
}

func loadWorld(dbPath, groundDir, models string) *entity.World {
	db, err := database.NewSQLite(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	store := database.NewNPCStore(db)
	w := entity.NewWorld()
	if m, err := store.PlayerModels(); err == nil {
		w.SetPlayerModels(m)
	} else {
		log.Fatal(err)
	}
	if m, err := store.ModelSexes(); err == nil {
		w.SetModelSexes(m)
	}
	if m, err := store.ModelRaces(); err == nil {
		w.SetModelRaces(m)
	}
	w.LoadNPCs(store)
	g, err := ground.Open(groundDir)
	if err != nil {
		log.Fatal(err)
	}
	w.SetGround(g)
	if e, err := ground.OpenEnv(groundDir); err == nil && e.Worlds() > 0 {
		w.SetEnv(e)
	} else {
		log.Fatalf("env: %v", err)
	}
	if models != "" {
		m, err := entity.LoadModelRadii(models)
		if err != nil {
			log.Fatal(err)
		}
		entity.SetModelRadii(m)
	}
	if recs, _, err := database.NewNPCPathStore(db).All(); err == nil {
		w.LoadNPCPaths(recs)
	} else {
		log.Fatal(err)
	}
	if _, err := w.LoadCampSpawns(db); err != nil {
		log.Fatal(err)
	}
	if _, _, err := w.LoadUndeadTags(db); err != nil {
		log.Fatal(err)
	}
	if _, err := w.LoadSocialRules(db); err != nil {
		log.Fatal(err)
	}
	if _, err := w.LoadMoveRules(db); err != nil {
		log.Fatal(err)
	}
	if _, err := w.LoadRowWander(db); err != nil {
		log.Fatal(err)
	}
	if _, _, err := w.LoadSpawnOnly(db); err != nil {
		log.Fatal(err)
	}
	if err := w.LoadFactions(db); err != nil {
		log.Fatal(err)
	}
	return w
}
