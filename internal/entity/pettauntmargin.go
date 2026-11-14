package entity

// pettauntmargin.go — a pet's taunt takes its creature and holds it a while (the owner,
// 2026-11-14: "pet taunts may not be working strongly enough").
//
// Until then a pet's taunt (pet.go petTauntLocked, pettemplate.go petTemplateTauntLocked, both
// through petTopHateLocked) raised the pet's threat to one above the top of the creature's list:
// the creature turned to the pet, and the next point of anyone's hate, its owner's nuke or heal
// a moment later, took it back before the pet's next swing. A player's taunt (taunt.go) sets its
// caster to the top plus the taunt's own hate, a margin of hundreds; a pet's taunt has no hate
// figure of its own (its hit's damage is its hate), so it is given one: petTauntMargin, the
// larger of petTauntMarginPct of the top threat and petTauntMarginPerLevel a level of the pet's.
// A player tank's taunt still takes the creature back (it sets the tank to the top, the pet's
// threat included, plus its hate). An enchanter's pet's taunt is still cut to 70% of the threat
// it adds (pettaunthate.go, the GM Jibwiz's petition 1928), the margin with it.
const (
	petTauntMarginPct      = 10
	petTauntMarginPerLevel = 20
)

// petTauntMargin is how far above the top threat top a pet's taunt puts it (the file's doc).
func petTauntMargin(pet *NPC, top int64) int64 {
	m := int64(max(pet.Level, 1)) * petTauntMarginPerLevel
	if p := top * petTauntMarginPct / 100; p > m {
		m = p
	}
	return m
}
