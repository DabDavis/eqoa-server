package handlers

import (
	"strings"
	"testing"
)

// Every word the High filter masks in chat, and every slur, is refused as a
// character's name: alone, capitalised as the creation sends it, and at the
// start of a longer name.
func TestHighFilterWordsAreRefusedNames(t *testing.T) {
	var words []string
	words = append(words, profaneRoots...)
	words = append(words, slurRoots...)
	for w := range profaneWords {
		words = append(words, w)
	}
	for w := range slurWords {
		words = append(words, w)
	}
	for _, w := range words {
		for _, n := range []string{w, strings.ToUpper(w[:1]) + w[1:], w + "lord"} {
			if !refusedName(n) {
				t.Errorf("%s was taken", n)
			}
		}
	}
}

// The words a name hides ("Bigcum", "Pissjug"), the plain ones the chat
// lets by ("Sex"), the letters drawn out ("Fuuuck", "Shiit") and a word's
// plural at the end ("Bigcocks") are refused.
func TestNamesWithAWordAreRefused(t *testing.T) {
	h := &CharacterHandler{}
	for _, n := range []string{
		"Sex", "Sexy", "Sexking", "Xxsexxx", "Cum", "Bigcum", "Cumlord",
		"Piss", "Pissjug", "Pissface", "Fishtits", "Crushboner", "Bigcocks",
		"Cockmaster", "Bigdick", "Asshat", "Asses", "Jackass", "Arsewipe",
		"Fuuuck", "Fuuck", "Shiit", "Seeex", "Cuuum", "Piiiss", "Twatwaffle",
		"Bootyrape", "Rapist", "Kikeslayer", "Nigman", "Wetbacks", "Spicboy",
		"Chink", "Coon", "Hitler", "Nazifan", "Porn", "Analbeads", "Milf",
		"Skank", "Pedobear",
	} {
		if h.isValidName(n) {
			t.Errorf("%s was taken", n)
		}
	}
}

// The control: fair names that hold such a word inside a fair one pass
// (each word's fair words, and the words refused only at a name's start).
func TestFairNamesPassTheNameFilter(t *testing.T) {
	h := &CharacterHandler{}
	for _, n := range []string{
		"Cassandra", "Glass", "Cutlass", "Assassin", "Passinstones",
		"Cucumber", "Cumberland", "Talcum", "Essex", "Sexton", "Hancock",
		"Peacocks", "Cockatrice", "Dickens", "Riddick", "Titania", "Petits",
		"Grapes", "Trapper", "Therapist", "Nightwatch", "Saltwater", "Nigel",
		"Knight", "Spicer", "Aspic", "Pakistan", "Dagon", "Santiago",
		"Swanky", "Prickly", "Bamboo", "Raccoon", "Tycoon", "Montenegro",
		"Stonegrove", "Nazira", "Thorny", "Torpedo", "Japan", "Japheth",
		"Fagin", "Bonereaver", "Hatebone", "Analyst", "Canal", "Alanus",
		"Uranus", "Semenov", "Horsemen", "Milford", "Arsenal", "Farseer",
		"Moonseeker", "Leeroy", "Funyun", "Jibrish",
	} {
		if !h.isValidName(n) {
			t.Errorf("%s was refused", n)
		}
	}
}

// Every name the creation's random button can give passes the filter
// (a suffix is "anus": Alanus, Valanus).
func TestRandomNamesPassTheNameFilter(t *testing.T) {
	h := &CharacterHandler{}
	for _, p := range namePrefixes {
		for _, s := range nameSuffixes {
			if n := p + s; !h.isValidName(n) {
				t.Errorf("%s was refused", n)
			}
		}
	}
}
