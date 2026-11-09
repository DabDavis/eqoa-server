package entity

import "strings"

// wereownbody.go — a were's forms without their look (the user, 2026-11-08:
// "lets add a CM 0 Cost that gives you the same Were Forms, but without the
// illusion"). A mastery rank whose effects carry NoWereIllusion (cmcatalog's
// effects, "NoWereIllusion:1": the rank "Were Forms Without Illusion", cost
// 0, for any were who has turned, tools/seed_were_own_body.py) keeps its
// owner in its own body while a were form is on it: the form lands as it
// always did, its stats, its time and its place among the illusions (a new
// form or illusion still takes it off, Remove Illusion still ends it), and
// only the body every client is sent (Player.Body, the entity channel's
// model and scale) is the player's own. Removing the rank (the window's
// removal, cost 0: nothing lost) shows the beast again at once, as buying it
// hides it at once: the channel is the body each tick.
//
// A were form is one of the five lines' (wolf, bear, rat, lion and lioness,
// gator) by its name: the form each line's third quest gives ("Ratwere") and
// the two each Feral mastery adds ("Feral Rat Form", "Mouse Form"), as the
// EQOAEmu workbook names them. Not database.IsWereAbility, which also takes
// the vampire's forms ("Assume the form of a vampire."): a vampire, a
// druid's or a shaman's beast form and every illusion spell keep their
// bodies.

// wereForms are the were forms' names, in lower case.
var wereForms = map[string]bool{
	// the lines' own forms, from their third quests
	"wolfwere": true, "bearwere": true, "ratwere": true, "lionwere": true, "lionesswere": true, "gatorwere": true,
	// the Feral masteries' two ranks
	"feral wolf form": true, "dire wolf form": true,
	"feral bear form": true, "primordial bear form": true,
	"feral rat form": true, "mouse form": true,
	"feral lion form": true, "sabretooth lion form": true,
	"feral lioness form": true, "sabretooth lioness form": true,
	"feral gator form": true, "primordial gator form": true,
}

// IsWereForm reports whether a spell (an effect) by this name is a were form.
func IsWereForm(name string) bool {
	return wereForms[strings.ToLower(strings.TrimSpace(name))]
}

// cmNoWereIllusion is the effects key of a rank that keeps its owner's body
// under a were form (parseStatList's spelling of "NoWereIllusion").
const cmNoWereIllusion = "nowereillusion"

// shownForm is whether the illusion effect e shows its body on the player:
// every illusion does, but a were form on a player who keeps its own body.
func (p *Player) shownForm(e *ActiveEffect) bool {
	return e.Form != 0 && !(p.WereOwnBody && IsWereForm(e.Name))
}
