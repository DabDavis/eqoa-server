package entity

// racialpet.go — a racial ability's pet has the same hit points whatever its
// owner's class (the owner, 2026-11-13: "make racial pets equal hp for every
// class"). A summoned pet's hit points are its level's (balancelevel) times
// its owner's class's share (balanceclasslevel's pet_hp_pct, tuned for each
// class's own pets: at 60 a magician's 76%, an enchanter's 31%), so the same
// racial pet had 3945 hit points for most classes and 1223 for an enchanter.
// A pet whose summoning spell a racial rank adds (racialRank: the racial
// category, or a master class or any rank of some races only: the gnome's
// Construct, the troll's Swamp Friends, the Mermaid's Aquatic Birthrights
// and Aquatic Ascension) takes its level's alone.

// racialRank is whether a rank is a race's: of the racial category, or
// offered to some races and not every base race.
func (r *CMRank) racialRank() bool {
	const baseRaces = 1<<11 - 1 // human .. ogre (racedefs 0-10)
	return r.Category == 1 || r.Races != 0 && r.Races&baseRaces != baseRaces
}

// racialSpellLocked is whether a racial rank adds the spell. Must be called
// with the lock held.
func (w *World) racialSpellLocked(spellID int) bool {
	return w.cms != nil && w.cms.racial[spellID]
}
