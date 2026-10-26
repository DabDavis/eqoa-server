package gmweb

import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/eqoa/server/internal/database"
)

// EQOA-Data's quest guides (github.com/Jadiction/EQOA-Data, Quests/**/*.json:
// {key, guide, contributors}, the guide in Markdown) made into DRAFT quests:
// a class's starting-city file has a "# Level N" section a quest, any other
// file is one quest. What a guide states in its fields is read (the quest NPC,
// the level, the class, the experience and the reward items in bold), the
// names resolved to this server's NPCs and items where one of the name exists,
// and the guide's own text kept in the notes: the steps are prose there, so a
// draft has one talk step to its giver until a GM writes the rest.

// eqoaDataFile is one guide as the page sends it.
type eqoaDataFile struct {
	Path string `json:"path"`
	Text string `json:"text"`
}

// eqoaDraft is one quest a guide makes.
type eqoaDraft struct {
	Name       string   `json:"name"`
	Source     string   `json:"source"`
	Giver      string   `json:"giver"`
	Level      int      `json:"level"`
	Classes    []int    `json:"classes,omitempty"`
	XP         int      `json:"xp"`
	ItemNames  []string `json:"itemNames,omitempty"`
	Journal    string   `json:"journal"`
	Notes      string   `json:"-"`
	GiverFound bool     `json:"giverFound"`
	Items      []int    `json:"items,omitempty"`
	Unresolved []string `json:"unresolved,omitempty"`
	Exists     bool     `json:"exists,omitempty"`
}

// eqoaClasses names the classes as EQOA-Data writes them, whole or short.
var eqoaClasses = map[string]int{
	"warrior": 0, "war": 0, "ranger": 1, "rng": 1, "ran": 1, "paladin": 2, "pal": 2,
	"shadowknight": 3, "shadow knight": 3, "sk": 3, "shd": 3, "monk": 4, "mnk": 4, "mon": 4,
	"bard": 5, "brd": 5, "rogue": 6, "rog": 6, "druid": 7, "drd": 7, "dru": 7, "shaman": 8, "shm": 8,
	"sha": 8, "cleric": 9, "clr": 9, "cle": 9, "magician": 10, "mag": 10, "necromancer": 11, "nec": 11,
	"enchanter": 12, "enc": 12, "wizard": 13, "wiz": 13, "alchemist": 14, "alc": 14,
}

var (
	reLevelHead = regexp.MustCompile(`(?i)^#\s*level\s+(\d+)`)
	reQuestName = regexp.MustCompile(`(?i)\*\*Quest Name:\*\*\s*([^\n]+)|Quest Name:\s*\*\*([^*\n]+)\*\*`)
	reLevel     = regexp.MustCompile(`(?i)\*\*Level:\*\*\s*(\d+)`)
	reGiver     = regexp.MustCompile(`(?i)\*\*(?:Quest NPC|Starting NPC|NPC):\*\*\s*([^\n]+)`)
	reClass     = regexp.MustCompile(`(?i)\*\*Class:\*\*\s*([^\n]+)`)
	reXP        = regexp.MustCompile(`(?i)\*\*\s*([\d,]+)\s*XP\s*\*\*`)
	reBold      = regexp.MustCompile(`\*\*([^*\n]+)\*\*`)
	reMarks     = regexp.MustCompile(`[*_` + "`" + `#>]+`)
)

// parseEQOAData reads one guide into its drafts (nothing resolved yet).
func parseEQOAData(p, text string) ([]eqoaDraft, error) {
	var doc struct {
		Key   string `json:"key"`
		Guide string `json:"guide"`
	}
	if err := json.Unmarshal([]byte(text), &doc); err != nil {
		return nil, fmt.Errorf("%s: %v", p, err)
	}
	if strings.TrimSpace(doc.Guide) == "" {
		return nil, nil
	}
	p = strings.TrimPrefix(path.Clean(strings.ReplaceAll(p, "\\", "/")), "/")
	if i := strings.Index(p, "Quests/"); i >= 0 {
		p = p[i+len("Quests/"):]
	}
	parts := strings.Split(p, "/")
	city := ""
	if len(parts) >= 3 {
		city = titleWords(strings.ReplaceAll(parts[len(parts)-2], "-", " "))
	}
	keyClass, isClass := eqoaClasses[strings.ToLower(strings.TrimSpace(doc.Key))]

	// the sections: "# Level N" headings, else the whole guide
	type section struct {
		level int
		text  string
	}
	var secs []section
	var cur *section
	for _, line := range strings.Split(doc.Guide, "\n") {
		if m := reLevelHead.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			lv, _ := strconv.Atoi(m[1])
			secs = append(secs, section{level: lv})
			cur = &secs[len(secs)-1]
			continue
		}
		if cur != nil {
			cur.text += line + "\n"
		}
	}
	if len(secs) == 0 {
		secs = []section{{text: doc.Guide}}
	}

	var out []eqoaDraft
	for _, sec := range secs {
		body := strings.TrimSpace(sec.text)
		if body == "" {
			continue
		}
		d := eqoaDraft{Level: sec.level, Source: "EQOA-Data " + p, Notes: body}
		if sec.level > 0 {
			d.Source += fmt.Sprintf(" level %d", sec.level)
		}
		if d.Level == 0 {
			if m := reLevel.FindStringSubmatch(body); m != nil {
				d.Level, _ = strconv.Atoi(m[1])
			}
		}
		if m := reGiver.FindStringSubmatch(body); m != nil {
			d.Giver = guideName(m[1])
		}
		switch {
		case isClass:
			d.Classes = []int{keyClass}
		default:
			if m := reClass.FindStringSubmatch(body); m != nil {
				d.Classes = parseClasses(m[1])
			}
		}
		if m := reQuestName.FindStringSubmatch(body); m != nil {
			d.Name = guideName(m[1] + m[2])
		}
		if d.Name == "" {
			switch {
			case isClass && city != "":
				d.Name = fmt.Sprintf("%s %s, level %d", city, titleWords(doc.Key), d.Level)
			case d.Level > 0:
				d.Name = fmt.Sprintf("%s, level %d", titleWords(doc.Key), d.Level)
			default:
				d.Name = titleWords(doc.Key)
			}
		}
		// the experience, and the rewards named in bold on its line
		for _, line := range strings.Split(body, "\n") {
			m := reXP.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			if xp, err := strconv.Atoi(strings.ReplaceAll(m[1], ",", "")); err == nil && xp > d.XP {
				d.XP = xp
			}
			for _, b := range reBold.FindAllStringSubmatch(line, -1) {
				if reXP.MatchString("**" + b[1] + "**") {
					continue
				}
				if n := guideName(b[1]); n != "" && !containsStr(d.ItemNames, n) {
					d.ItemNames = append(d.ItemNames, n)
				}
			}
		}
		d.Journal = firstSentence(body)
		out = append(out, d)
	}
	return out, nil
}

// guideName is a name out of Markdown: marks gone, a parenthesis or a second
// clause cut off.
func guideName(s string) string {
	s = reMarks.ReplaceAllString(s, "")
	for _, cut := range []string{" (", "(", " - ", ";"} {
		if i := strings.Index(s, cut); i > 0 {
			s = s[:i]
		}
	}
	return strings.Trim(strings.TrimSpace(s), ".,:")
}

// parseClasses reads "Drd, Nec, Wiz" (or "All": none).
func parseClasses(s string) []int {
	var out []int
	for _, f := range strings.FieldsFunc(strings.ToLower(reMarks.ReplaceAllString(s, "")), func(r rune) bool {
		return r == ',' || r == '/' || r == '&'
	}) {
		f = strings.TrimSpace(f)
		if c, ok := eqoaClasses[f]; ok && !containsInt(out, c) {
			out = append(out, c)
		}
	}
	return out
}

func containsInt(v []int, n int) bool {
	for _, x := range v {
		if x == n {
			return true
		}
	}
	return false
}

// firstSentence is a guide's first line of prose, its first sentence, cut to
// a journal line's 127 characters.
func firstSentence(body string) string {
	for _, line := range strings.Split(body, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "---") || strings.HasPrefix(t, "**") ||
			strings.HasPrefix(t, "|") || strings.HasPrefix(t, "!") {
			continue
		}
		t = strings.TrimLeft(t, "-*0123456789. ")
		t = strings.TrimSpace(reMarks.ReplaceAllString(t, ""))
		if t == "" {
			continue
		}
		for i, r := range t {
			if (r == '.' || r == '!' || r == '?') && i > 10 {
				t = t[:i+1]
				break
			}
		}
		if utf8.RuneCountInString(t) > 127 {
			t = strings.TrimSpace(string([]rune(t)[:124])) + "..."
		}
		return t
	}
	return ""
}

// titleWords capitalises each word.
func titleWords(s string) string {
	w := strings.Fields(s)
	for i, x := range w {
		if x != "" {
			w[i] = strings.ToUpper(x[:1]) + x[1:]
		}
	}
	return strings.Join(w, " ")
}

// resolveDraft finds a draft's giver and items among this server's.
func (s *Server) resolveDraft(d *eqoaDraft) {
	if d.Giver != "" {
		var n int
		s.d.DB.QueryRow(`SELECT COUNT(*) FROM npcs WHERE LOWER(TRIM(npc_name)) = LOWER(TRIM(?))`, d.Giver).Scan(&n)
		d.GiverFound = n > 0
		if !d.GiverFound {
			d.Unresolved = append(d.Unresolved, "NPC "+d.Giver)
		}
	}
	for _, name := range d.ItemNames {
		var id int64
		found := false
		for _, try := range []string{name, name + " Scroll", "Scroll of " + name, "Scroll: " + name} {
			if err := s.d.DB.QueryRow(`SELECT patternid FROM itempattern WHERE LOWER(itemname) = LOWER(?) ORDER BY patternid LIMIT 1`, try).Scan(&id); err == nil {
				found = true
				break
			}
		}
		if found {
			d.Items = append(d.Items, int(id))
		} else {
			d.Unresolved = append(d.Unresolved, "item "+name)
		}
	}
	var n int
	s.d.DB.QueryRow(`SELECT COUNT(*) FROM quests WHERE source = ?`, d.Source).Scan(&n)
	// the content import (tools/import_quests.py) makes a city guide's
	// sections whole, keyed "1-20/<city>/<class>/L<level>" (a second quest
	// of a section "...L<level>b"): a section it made is not made again
	if key := contentKey(d.Source); n == 0 && key != "" {
		s.d.DB.QueryRow(`SELECT COUNT(*) FROM quests WHERE source LIKE ? OR source LIKE ?`,
			"EQOA-Data "+key+" (%", "EQOA-Data "+key+"_ (%").Scan(&n)
	}
	d.Exists = n > 0
}

// contentKey is the content import's key for a draft's source ("EQOA-Data
// 1-20/freeport/warrior.json level 3" is "1-20/freeport/warrior/L3"), or "".
func contentKey(source string) string {
	m := reContentSource.FindStringSubmatch(source)
	if m == nil {
		return ""
	}
	return m[1] + "/L" + m[2]
}

var reContentSource = regexp.MustCompile(`^EQOA-Data (?:Quests/)?(1-20/[^/]+/[^/.]+)\.json level (\d+)$`)

func (s *Server) questImport(a *actor, files []eqoaDataFile, dry bool) (any, error) {
	if len(files) == 0 {
		return nil, badRequest("no files")
	}
	var drafts []eqoaDraft
	var errs []string
	for _, f := range files {
		if strings.HasSuffix(f.Path, "_meta.json") {
			continue
		}
		ds, err := parseEQOAData(f.Path, f.Text)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		drafts = append(drafts, ds...)
	}
	for i := range drafts {
		s.resolveDraft(&drafts[i])
	}
	made, skipped := 0, 0
	if !dry {
		qt, err := s.table("quests")
		if err != nil {
			return nil, err
		}
		st, err := s.table("queststeps")
		if err != nil {
			return nil, err
		}
		for i := range drafts {
			d := &drafts[i]
			if d.Exists {
				skipped++
				continue
			}
			notes := d.Notes
			if len(d.Unresolved) > 0 {
				notes = "Not found on this server: " + strings.Join(d.Unresolved, "; ") + "\n\n" + notes
			}
			ch, err := s.insertRow(a, qt, map[string]any{
				"name": d.Name, "enabled": int64(0), "giver": d.Giver, "minlevel": int64(max(d.Level, 1)), "maxlevel": int64(60),
				"classes": database.FormatIntList(d.Classes), "source": d.Source, "notes": notes,
			}, writeOpts{})
			if err != nil {
				return nil, fmt.Errorf("%s: %w", d.Name, err)
			}
			id, _ := asFloat(ch.Key["questid"])
			items := make([]database.ItemCount, len(d.Items))
			for j, it := range d.Items {
				items[j] = database.ItemCount{PatternID: it, Count: 1}
			}
			if _, err := s.insertRow(a, st, map[string]any{
				"questid": int64(id), "step": int64(1), "kind": database.StepTalk, "target": d.Giver,
				"journal": d.Journal, "xp": int64(d.XP), "rewarditems": database.FormatItemCounts(items),
			}, writeOpts{}); err != nil {
				return nil, fmt.Errorf("%s: %w", d.Name, err)
			}
			made++
		}
	}
	resolved := 0
	for _, d := range drafts {
		if len(d.Unresolved) == 0 {
			resolved++
		}
	}
	out := map[string]any{"drafts": drafts, "total": len(drafts), "fullyResolved": resolved, "dryRun": dry,
		"made": made, "skipped": skipped, "errors": errs}
	if !dry {
		out["live"] = "drafts are offered to no one until a GM writes their steps and turns them on"
	}
	return out, nil
}
