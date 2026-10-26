package database

import "regexp"

// wereAbility matches what marks a were form or a were's ability in its name
// or its description: "Ratwere", "Lionwere", "Werewolf Form" ("Assume the
// form of the werewolf."), "Wererat Melee Ability", the feral and dire forms
// ("CM: Illusion", "Assume the form of a feral wolf."), and their copies that
// keep the body ("This spell grants the power of the beast without changing
// your form.").
var wereAbility = regexp.MustCompile(`(?i)\bwere(rat|wolf|bear|lion|gator)|(rat|lion|wolf|bear|gator)were\b|power of the beast|^assume the form of|^cm: illusion\.?$`)

// IsWereAbility reports whether a spell (its name and description) is a were
// form or a were's ability. Those come from a were quest, which any class may
// do ("Class: ALL" in EQOA-Data's guides), and the forms past the first are
// bought as masteries; they sit in classspells only because characters of
// those classes on Sandstorm had them (F3 20261025-220040: #template gave a
// necromancer the lion forms). No class grant gives one.
func IsWereAbility(name, desc string) bool {
	return wereAbility.MatchString(name) || wereAbility.MatchString(desc)
}
