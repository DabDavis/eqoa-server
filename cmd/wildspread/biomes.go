package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/eqoa/server/internal/database"
)

// The creatures a place's wildlife may be, by Prima's bestiary (Prima's
// Official Strategy Guide, EverQuest Online Adventures: Frontiers, December
// 2004, pp. 653-673: "almost all of the creature NPC's found in Tunaria and
// those that have been added with the release of EQOA: Frontiers ...
// presented by region", each with its levels) and EQOA-Data's map (its
// zones' biomes, a quarter of each zone apiece). jib's F3 asked for it:
// "reference prima prefronts and frontiers strategy guides for wildlife
// spawns". The guide is the operator's own copy, read as pdftotext -layout
// writes it (tools/seed_signs.py's --prima).

// primaEntry is one of the bestiary's rows: its section and levels.
type primaEntry struct {
	section string
	lo, hi  int
}

// primaSections are the bestiary's sections (its tables' headings).
var primaSections = map[string]bool{
	"Arctic Beasts": true, "Beach": true, "Desert": true, "Forest": true, "Fresh Water": true, "Global": true,
	"Grassland": true, "Jungle": true, "Mountain": true, "Ocean": true, "Savannah": true, "Swamp": true,
	"Volcanic": true, "Deep Jungle": true, "Marine": true, "Rathe Mountains": true, "Red Desert": true,
	"Wasteland": true, "Wetlands": true,
}

var (
	primaHeading = regexp.MustCompile(`^ {20,}([A-Z][A-Za-z' ]+?)(?: \(cont.d\))?\s*$`)
	primaRow     = regexp.MustCompile(`^(\S(?:.*?\S)?)\s{2,}(\d{1,2})(?:\s*[-–]\s*(\d{1,2}))?\s{2,}\S`)
)

// readPrima reads the bestiary out of the guide's text: name (as
// database.NPCClassKey keys it) -> its rows, the part ("Tunaria" or
// "Frontiers") prefixed to each section.
func readPrima(path string) (map[string][]primaEntry, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(b), "\n")
	start := -1
	for i, l := range lines {
		// the chapter opens with this sentence; the table of contents and
		// the class chapters before it have rows of the same shape
		if strings.Contains(l, "The following tables represent almost all of the creature NPC") {
			start = i
		}
	}
	out := map[string][]primaEntry{}
	if start < 0 {
		return out, nil
	}
	part, section := "", ""
	for _, l := range lines[start:] {
		if m := primaHeading.FindStringSubmatch(l); m != nil {
			t := strings.TrimSpace(m[1])
			switch {
			case t == "Tunaria" || t == "Frontiers":
				part = t
				continue
			case primaSections[t]:
				section = t
				continue
			}
		}
		m := primaRow.FindStringSubmatch(l)
		if m == nil || section == "" || strings.HasPrefix(l, "Name") {
			continue
		}
		lo, _ := strconv.Atoi(m[2])
		hi := lo
		if m[3] != "" {
			hi, _ = strconv.Atoi(m[3])
		}
		key := database.NPCClassKey(m[1])
		out[key] = append(out[key], primaEntry{section: part + "/" + section, lo: lo, hi: hi})
	}
	return out, nil
}

// biomeSections are the bestiary's sections a place of EQOA-Data's biome
// draws its wildlife from (and Tunaria's Global table, sectionsAt).
var biomeSections = map[string][]string{
	"Arctic":   {"Tunaria/Arctic Beasts"},
	"Forest":   {"Tunaria/Forest"},
	"Desert":   {"Tunaria/Desert"},
	"Plains":   {"Tunaria/Grassland", "Tunaria/Savannah"},
	"Mountain": {"Tunaria/Mountain"},
	"Swamp":    {"Tunaria/Swamp"},
	"Marine":   {"Tunaria/Beach"},
}

// worldSections are the sections of a world EQOA-Data gives no biomes for:
// Rathe and Odus, the Frontiers' lands.
var worldSections = map[int][]string{
	1: {"Frontiers/Rathe Mountains"},
	2: {"Frontiers/Deep Jungle", "Frontiers/Red Desert", "Frontiers/Savannah", "Frontiers/Wasteland", "Frontiers/Wetlands"},
}

// readZoneBiomes reads EQOA-Data's zones' biomes (Map/zones.json): the
// zone's name, as its letters alone in lower case, -> its four quarters'
// biomes, north-west, north-east, south-west, south-east.
func readZoneBiomes(dir string) (map[string][]string, error) {
	b, err := os.ReadFile(filepath.Join(dir, "Map", "zones.json"))
	if err != nil {
		return nil, err
	}
	var doc struct {
		Zones []struct {
			Name   string   `json:"name"`
			Biomes []string `json:"biomes"`
		} `json:"zones"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for _, z := range doc.Zones {
		if len(z.Biomes) == 4 {
			out[letters(z.Name)] = z.Biomes
		}
	}
	return out, nil
}

var notLetters = regexp.MustCompile(`[^a-z]`)

func letters(s string) string { return notLetters.ReplaceAllString(strings.ToLower(s), "") }

// sectionsAt are the bestiary sections wildlife at (x, z) of world w comes
// from: Tunaria's Global table and its zone's quarter's biome's, the other
// worlds' as worldSections says.
func (p *plan) sectionsAt(w int, x, z float64) []string {
	if w != 0 {
		return worldSections[w]
	}
	word := tunariaZone(x, z)
	if word >= len(p.words) {
		return nil
	}
	// the bestiary's Global table is its Tunaria chapter's: anywhere there
	global := []string{"Tunaria/Global"}
	b := p.biomes[letters(p.words[word])]
	if b == nil {
		return global
	}
	q := 0
	if x-2000*float64(int(x/2000)) >= 1000 {
		q++
	}
	if z-2000*float64(int(z/2000)) >= 1000 {
		q += 2
	}
	return append(global, biomeSections[b[q]]...)
}

// primaRange is the levels the bestiary gives a kind in one of sections,
// false when it lists the kind in none of them. A kind it
// lists more than once (the warthog: Tunaria's 26-30, the Frontiers'
// savannah's 36-55) takes the entry that holds the place's level, else the
// nearest to it.
func (p *plan) primaRange(key string, sections []string, place int) (lo, hi int, ok bool) {
	best := -1
	for _, e := range p.prima[key] {
		match := false
		for _, s := range sections {
			if e.section == s {
				match = true
			}
		}
		if !match {
			continue
		}
		off := 0
		if place < e.lo {
			off = e.lo - place
		} else if place > e.hi {
			off = place - e.hi
		}
		if best < 0 || off < best {
			lo, hi, ok, best = e.lo, e.hi, true, off
		}
	}
	return lo, hi, ok
}
