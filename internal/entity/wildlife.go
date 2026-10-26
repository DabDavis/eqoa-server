package entity

import (
	"regexp"
	"strings"
)

// wildlife.go — which creatures are wildlife, the kind that wanders round its
// spawn (roam.go) when table npcmove does not say otherwise (F3
// 20261026-042712, jib: "Not all mobs are wild life. Fire Giants are roaming
// currently when they should not be unless otherwise specified / designated
// as a roamer with a path").
//
// Wildlife is an animal, by its name's head noun: the last word, past the
// words that name an animal's place in its pack or hive ("Wolf Pack Leader",
// "Bloodfin Brood Shark Mother", "red ant warrior": wildlifeTitles), is one of
// wildlifeNouns (as written, or a plural of one), or a word ending in "fly"
// ("Scorchflies"). So "Giant Rat" is wildlife and "Fire Giant" is not, nor an
// "Orc Pawn" or a "Deathfist reaver". A name with a guard's word in it
// (wildlifeGuards: "serpent guardian", "griffon nest protector") is a camp's
// guard, not wildlife, and an undead creature (undead.go: "Skeletal Wolf")
// never is. No record of the live game says which creatures wandered: the
// names are all there is. tools/seed_npc_move.py keeps the same three lists
// (TestSeedToolKeepsTheWildlifeLists) and writes npcmove rows for the
// animals the words miss, by the bodies they wear, and for the camps
// EQOA-Data's map says roam.

// wildlifeNouns are the animals' head nouns, singular (wildlifeNoun takes
// plurals).
var wildlifeNouns = wordSet(`
rat mouse mice mole badger wolverine weasel ferret raccoon skunk squirrel rabbit hare beaver otter seal walrus
wolf warg worg jackal hyena fox coyote dog hound mastiff
bear grizzly kodiak cub greatbear blackbear
lion lioness lionness tiger panther puma cougar catamount lynx jaguar leopard sabretooth sabertooth cat kitten
boar pig hog sawtusk warthog
deer elk stag doe fawn moose caribou bison buffalo ox oxen yak cow bull horse pony mule goat ram sheep mammoth rhino rhinoceros elephant hippo camel
gorilla ape monkey baboon
bat bird hawk eagle falcon vulture crow raven owl parrot gull seagull pelican heron crane stork flamingo ostrich rooster chicken turkey duck goose geese swan kestrel buzzard condor magpie rook
snake asp viper cobra python adder rattler rattlesnake moccasin boa serpent serpant copperhead cooperhead kingsnake anaconda sidewinder mamba
lizard gecko iguana skink crocodile alligator caiman gharial turtle tortoise terrapin frog toad salamander newt
basilisk cockatrice
ant antlion beetle firebeetle scarab spider spiderling tarantula widow recluse scorpion scropion wasp hornet bee
fly firefly sparkfly emberfly drakefly saurianfly dragonfly mosquito gnat moth butterfly locust grasshopper cricket mantis centipede millipede crawler lavacrawler burrower
worm grub larva larvae maggot slug snail leech tick roach cockroach termite bug
crab lobster shrimp crayfish
fish eel shark hammerhead piranha pike trout salmon bass carp catfish minnow angler ray squid octopus octopi jellyfish jelly kraken whale dolphin barracuda bonefish stonefish sawfish
goo ooze slime
griffon griffin gryphon griffenne griffawn hippogriff drake wyvern manticore
hatchling mindwhipper bixie dreadwolf filthbat snowbat sandcrawler warboar hornfly razorback ghariel
`)

// wildlifeTitles are the words after an animal's noun that name its place
// ("Lioness Stalker", "Vulture Chick", "ant harvester"): passed over for the
// head noun. After a person's ("gnoll scout") the noun is the person's.
var wildlifeTitles = wordSet(`
mother father matriarch patriarch queen king alpha leader pack brood broodmother young elder runt pup calf kit chick
stalker worker soldier drone carpenter leafcutter harvester scout warrior hunter lurker breeder monarch swarm
`)

// wildlifeGuards are a guard's words: a creature named one keeps its post.
var wildlifeGuards = wordSet(`
guard guardian sentry sentinel warden warder watcher watchman watchmen keeper protector defender gatekeeper bodyguard lookout
`)

func wordSet(s string) map[string]bool {
	out := make(map[string]bool)
	for _, w := range strings.Fields(s) {
		out[w] = true
	}
	return out
}

var reNameWord = regexp.MustCompile(`[a-z]+`)

// nameWords is a name's words, lower case: letters alone ("Qy'tha" is two).
func nameWords(name string) []string {
	return reNameWord.FindAllString(strings.ToLower(strings.ReplaceAll(name, "$", "")), -1)
}

// singulars is a word and what it is with a plural's ending taken off.
func singulars(w string) []string {
	out := []string{w}
	if strings.HasSuffix(w, "ies") && len(w) > 4 {
		out = append(out, w[:len(w)-3]+"y")
	}
	if strings.HasSuffix(w, "ves") && len(w) > 4 {
		out = append(out, w[:len(w)-3]+"f")
	}
	if strings.HasSuffix(w, "es") && len(w) > 3 {
		out = append(out, w[:len(w)-2])
	}
	if strings.HasSuffix(w, "s") && len(w) > 3 {
		out = append(out, w[:len(w)-1])
	}
	return out
}

// inSet reports a word of the set, as written or as a plural of one.
func inSet(w string, set map[string]bool) bool {
	for _, s := range singulars(w) {
		if set[s] {
			return true
		}
	}
	return false
}

// wildlifeNoun reports an animal's noun: one of wildlifeNouns, or a word
// ending in "fly" ("drakeflies", "hornfly").
func wildlifeNoun(w string) bool {
	for _, s := range singulars(w) {
		if wildlifeNouns[s] || (len(s) >= 6 && strings.HasSuffix(s, "fly")) {
			return true
		}
	}
	return false
}

// headNoun is a name's head noun: its last word, past wildlifeTitles' words.
func headNoun(words []string) string {
	for len(words) > 1 && inSet(words[len(words)-1], wildlifeTitles) {
		words = words[:len(words)-1]
	}
	if len(words) == 0 {
		return ""
	}
	return words[len(words)-1]
}

// WildlifeWord is the word that makes a name wildlife's (its head noun),
// "" when the name is not an animal's (the file's comment). The name alone:
// an undead creature of such a name is not wildlife (wildlifeLocked).
func WildlifeWord(name string) string {
	words := nameWords(name)
	if len(words) == 0 {
		return ""
	}
	for _, w := range words {
		if inSet(w, wildlifeGuards) {
			return ""
		}
	}
	if h := headNoun(words); wildlifeNoun(h) {
		return h
	}
	return ""
}

// wildKind is a name's verdict: its head noun when an animal's, and whether
// its creatures are wildlife (not undead).
type wildKind struct {
	word string
	wild bool
}

// wildlifeLocked reports a creature that is wildlife by its name (and not
// undead, by its npcundead row or its name), and the word that made it so,
// worked out once a name. Must be called with the World's lock held (a read
// lock will do).
func (w *World) wildlifeLocked(npc *NPC) (word string, ok bool) {
	w.wildMu.Lock()
	defer w.wildMu.Unlock()
	k, found := w.wildCache[npc.Name]
	if !found {
		k.word = WildlifeWord(npc.Name)
		k.wild = k.word != "" && !w.undeadLocked(npc)
		if w.wildCache == nil {
			w.wildCache = make(map[string]wildKind)
		}
		w.wildCache[npc.Name] = k
	}
	return k.word, k.wild
}

// forgetWildlifeLocked drops the verdicts, for the undead tags changed. Must
// be called with the World's lock held.
func (w *World) forgetWildlifeLocked() {
	w.wildMu.Lock()
	w.wildCache = nil
	w.wildMu.Unlock()
}
