package handlers

import "strings"

// The slurs the Medium profanity filter masks (the GM Jibrish's petition
// 1625: "medium doesnt block swears, but blocks hard slurs (But not the word
// 'retarded', think racial slurs only)"). High masks these too, with every
// word of profanity.go. Only racial and ethnic slurs no clean word is: a
// word with a fair meaning in the game's talk ("chink" in armour, "coon",
// "spook", "jap" for a jab) is left out, as are ordinary swears and
// "retarded" (High's alone).

// slurRoots mask any word they begin ("niggas"); no clean word begins with
// one ("snigger" holds the first inside, not at its start).
var slurRoots = []string{
	"nigg", "sandnigg", "wetback", "towelhead", "raghead", "zipperhead",
	"porchmonk", "jigaboo",
}

// slurWords are masked as whole words only.
var slurWords = map[string]bool{
	"nig": true, "nigs": true, "niglet": true, "niglets": true,
	"kike": true, "kikes": true, "kyke": true, "kykes": true,
	"spic": true, "spics": true, "beaner": true, "beaners": true,
	"gook": true, "gooks": true, "paki": true, "pakis": true,
	"wop": true, "wops": true, "dago": true, "dagos": true,
}

// slur is whether one word (letters, any case) is a slur.
func slur(word string) bool {
	w := strings.ToLower(word)
	if slurWords[w] {
		return true
	}
	for _, r := range slurRoots {
		if strings.HasPrefix(w, r) {
			return true
		}
	}
	return false
}
