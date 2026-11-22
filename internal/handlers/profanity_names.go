package handlers

import "strings"

// A new character's name is held to the High profanity filter and more
// (the GM Jibrish, 2026-11-18: "No words used in that can be created for
// new characters going forward eg; we need to block swears, 'sex, cum,
// piss' and slurs from being character names"): every word High masks in
// chat (profanity.go, profanity_slurs.go), and the sexual words, slurs and
// hate names a name must not carry though the chat lets them by ("sex").
// A name is one word, so a word hides in it ("Bigcum", "Pissjug"): each
// word is refused where it is found in the name, unless the letters round
// it are one of its fair words (Cucumber holds "cum", Nightwatch "twat").
// Letters drawn out to slip by ("Fuuuck", "Seeex", "Shiit") are drawn in
// again.

// namePlace is where in a name a refused word counts.
type namePlace uint8

const (
	atStart namePlace = 1 << iota // the name begins with it
	atEnd                         // the name ends with it, or with it and an s
	inside                        // between the two

	anywhere = atStart | atEnd | inside
	edges    = atStart | atEnd
)

// nameBan is a word no new character's name may carry where it counts.
type nameBan struct {
	word string
	at   namePlace
	// fair are words holding it that a name may carry: an occurrence
	// inside one of them is not the word ("cumber" in Cumberland).
	fair []string
}

// nameBans are the words, lower case. A word whose fair words would be
// endless is refused only where it cannot be one ("ass" begins few fair
// names but ends many: Glass, Kass, Cutlass), and its compounds are named.
var nameBans = []nameBan{
	// The High filter's swears and sexual words.
	{word: "fuck", at: anywhere},
	{word: "shit", at: anywhere},
	{word: "cunt", at: anywhere},
	{word: "bitch", at: anywhere},
	{word: "whore", at: anywhere},
	{word: "wank", at: anywhere, fair: []string{"swank"}},
	{word: "dildo", at: anywhere},
	{word: "twat", at: anywhere, fair: []string{"twatch", "twater"}},
	{word: "jizz", at: anywhere},
	{word: "pussy", at: anywhere},
	{word: "penis", at: anywhere},
	{word: "vagina", at: anywhere},
	{word: "slut", at: anywhere},
	{word: "bastard", at: anywhere},
	{word: "douche", at: anywhere},
	{word: "retard", at: anywhere},
	{word: "faggot", at: anywhere},
	{word: "asshole", at: anywhere},
	{word: "arsehole", at: anywhere},
	{word: "cocksuck", at: anywhere},
	{word: "piss", at: anywhere, fair: []string{"pissarro"}},
	{word: "cum", at: anywhere, fair: []string{
		"cucumber", "document", "circum", "accumul", "incumb", "succumb",
		"acumen", "tecumseh", "cumber", "cumul", "cumin", "cumbria",
		"scum", "talcum", "locum", "slocum", "marcum", "mecum"}},
	{word: "dick", at: anywhere, fair: []string{
		"dickens", "dickson", "dickinson", "dickerson", "dickie", "dickey",
		"dickon", "dickory", "benedick", "riddick"}},
	{word: "prick", at: anywhere, fair: []string{"prickle", "prickly", "pinprick"}},
	{word: "rape", at: anywhere, fair: []string{
		"grape", "drape", "scrape", "crape", "trapez", "therape"}},
	{word: "tits", at: anywhere, fair: []string{"petits"}},
	{word: "titty", at: anywhere},
	{word: "titties", at: anywhere},
	{word: "boob", at: anywhere, fair: []string{"booboo", "bamboob"}},
	{word: "ass", at: atStart, fair: []string{
		"assassin", "assist", "assault", "asset", "assign", "assembl",
		"assert", "assess", "assum", "assur", "assort", "assail",
		"associat", "assent", "assay", "assam", "assyr"}},
	{word: "jackass", at: anywhere},
	{word: "dumbass", at: anywhere},
	{word: "dumass", at: anywhere},
	{word: "fatass", at: anywhere},
	{word: "badass", at: anywhere},
	{word: "bigass", at: anywhere},
	{word: "smartass", at: anywhere},
	{word: "kickass", at: anywhere},
	{word: "lardass", at: anywhere},
	{word: "hardass", at: anywhere},
	{word: "arse", at: atStart, fair: []string{"arsen"}},
	{word: "cock", at: edges, fair: []string{
		"cockatri", "cockatoo", "cockroach", "cocktail", "cockpit",
		"cockerel", "cockle", "cockney", "cockade", "cockburn",
		"peacock", "hancock", "babcock", "woodcock", "gamecock",
		"hitchcock", "haycock", "moorcock", "shuttlecock", "wilcock",
		"alcock", "adcock", "laycock", "pocock", "stopcock"}},
	{word: "fag", at: edges, fair: []string{"fagin", "fagan"}},
	{word: "boner", at: edges, fair: []string{
		"bonera", "bonere", "boneri", "bonero", "boneru"}},

	// Not masked in chat, never a name.
	{word: "sex", at: anywhere, fair: []string{
		"sextant", "sexton", "sextet", "sextus", "essex", "sussex", "wessex",
		"middlesex"}},
	{word: "porn", at: anywhere},
	{word: "anal", at: atStart, fair: []string{"analog", "analy", "anali", "analee"}},
	{word: "anus", at: atStart, fair: []string{"anush"}},
	{word: "semen", at: atStart, fair: []string{"semeno"}},
	{word: "sperm", at: anywhere},
	{word: "orgasm", at: anywhere},
	{word: "clit", at: anywhere, fair: []string{"clith"}},
	{word: "horny", at: anywhere, fair: []string{"thorny"}},
	{word: "milf", at: anywhere, fair: []string{"milford", "milfoil"}},
	{word: "nipple", at: anywhere},
	{word: "blowjob", at: anywhere},
	{word: "handjob", at: anywhere},
	{word: "rapist", at: anywhere, fair: []string{"therapist"}},
	{word: "molest", at: anywhere},
	{word: "incest", at: anywhere},
	{word: "pedo", at: edges, fair: []string{"torpedo"}},
	{word: "skank", at: anywhere},

	// The slurs (Medium and High mask them in chat).
	{word: "nigg", at: anywhere},
	{word: "nig", at: edges, fair: []string{"nigh", "nigel", "niger", "konig"}},
	{word: "niglet", at: anywhere},
	{word: "kike", at: anywhere},
	{word: "kyke", at: anywhere},
	{word: "spic", at: edges, fair: []string{"spice", "spicy", "spick", "aspic"}},
	{word: "beaner", at: anywhere},
	{word: "gook", at: anywhere},
	{word: "paki", at: edges, fair: []string{"pakistan"}},
	{word: "wop", at: edges},
	{word: "dago", at: edges, fair: []string{"dagon", "dagobert", "dagoth"}},
	{word: "wetback", at: anywhere},
	{word: "towelhead", at: anywhere},
	{word: "raghead", at: anywhere},
	{word: "zipperhead", at: anywhere},
	{word: "porchmonk", at: anywhere},
	{word: "jigaboo", at: anywhere},

	// Slurs and hate names the chat leaves to its sense ("chink" in armour).
	{word: "chink", at: anywhere},
	{word: "coon", at: anywhere, fair: []string{
		"raccoon", "racoon", "cocoon", "tycoon", "coonhound"}},
	{word: "tranny", at: anywhere},
	{word: "dyke", at: anywhere, fair: []string{"vandyke"}},
	{word: "negro", at: edges, fair: []string{"montenegro", "negroni"}},
	{word: "jap", at: edges, fair: []string{"japan", "jape", "japh"}},
	{word: "wigger", at: anywhere},
	{word: "redskin", at: anywhere},
	{word: "nazi", at: anywhere, fair: []string{"nazir", "nazim", "kenazi"}},
	{word: "hitler", at: anywhere},
	{word: "kkk", at: anywhere},
}

// refusedName is whether a new character's name carries a word of
// nameBans where it counts: as written, with every run of three or more of
// one letter drawn in to one ("Fuuuck", "Piiiss"), and with every doubled
// vowel drawn in ("Shiit", "Seex") for the words with none (drawing in a
// doubled consonant too would make Trapper "traper", holding "rape").
func refusedName(name string) bool {
	n := strings.ToLower(name)
	if bannedIn(n, false) {
		return true
	}
	if s := squeezeRuns(n, 3, false); s != n && bannedIn(s, false) {
		return true
	}
	if s := squeezeRuns(n, 2, true); s != n && bannedIn(s, true) {
		return true
	}
	return false
}

// bannedIn is whether n carries a word of nameBans where it counts;
// squeezed is n with its doubled vowels drawn in, so the words with one
// are left out and the fair words are drawn in too.
func bannedIn(n string, squeezed bool) bool {
	for _, b := range nameBans {
		if squeezed && squeezeRuns(b.word, 2, true) != b.word {
			continue
		}
		for i := 0; ; {
			j := strings.Index(n[i:], b.word)
			if j < 0 {
				break
			}
			at := i + j
			if b.at&placeOf(n, at, len(b.word)) != 0 && !fairAt(n, at, len(b.word), b.fair, squeezed) {
				return true
			}
			i = at + 1
		}
	}
	return false
}

// placeOf is where in n the word at at, w letters long, sits: at its
// start, its end (a plural's s after it counting as the end), or inside.
func placeOf(n string, at, w int) namePlace {
	var p namePlace
	if at == 0 {
		p |= atStart
	}
	if end := at + w; end == len(n) || (end == len(n)-1 && n[end] == 's') {
		p |= atEnd
	}
	if p == 0 {
		p = inside
	}
	return p
}

// fairAt is whether the word at at, w letters long, lies within one of
// its fair words in n.
func fairAt(n string, at, w int, fair []string, squeezed bool) bool {
	for _, f := range fair {
		if squeezed {
			f = squeezeRuns(f, 2, true)
		}
		for i := 0; ; {
			j := strings.Index(n[i:], f)
			if j < 0 {
				break
			}
			start := i + j
			if start <= at && at+w <= start+len(f) {
				return true
			}
			i = start + 1
		}
	}
	return false
}

// squeezeRuns is s with every run of least or more of one letter (of a
// vowel only, when vowels) drawn in to that letter once.
func squeezeRuns(s string, least int, vowels bool) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		j := i
		for j < len(s) && s[j] == s[i] {
			j++
		}
		if j-i >= least && (!vowels || strings.IndexByte("aeiouy", s[i]) >= 0) {
			b.WriteByte(s[i])
		} else {
			b.WriteString(s[i:j])
		}
		i = j
	}
	return b.String()
}
