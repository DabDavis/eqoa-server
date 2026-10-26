package database

import "strings"

// A classspells row's source says where the row came from (dump, vendor,
// eqoa-data) and, for a spell no level gives, how it is got instead:
//
//	eqoa-data cm             a class mastery, bought with mastery points
//	eqoa-data epic           a level-60 epic, quested
//	eqoa-data mca            a master class's (the lists name no class)
//	eqoa-data master <name>  a master class's own ability ("master Lich")
//
// Such a row says whose the spell is, so the community spell-visual page
// lists it with its class (internal/spellfx), but no class grant gives it:
// #template and world.start_all_spells (GrantClassSpells), grant_spell's
// "all" (the GM panel) and a necromancer pet's taps (ClassSpellLines) skip
// it, as they skip the were forms (IsWereAbility). None of those can be
// bought or earned here yet. tools/import_eqoadata_mastery_spells.py writes
// them; a GM may write one by hand (the "eqoa-data " is optional).

// GotBy is how a spell of a classspells row of that source is got when no
// level gives it: "mastery", "epic" or "master class"; "" for a spell a
// character of the class gets by level (dump, vendor, eqoa-data, anything
// else).
func GotBy(source string) string {
	words := strings.Fields(strings.ToLower(sourceKind(source)))
	if len(words) == 0 {
		return ""
	}
	switch words[0] {
	case "cm", "mastery":
		return "mastery"
	case "epic":
		return "epic"
	case "mca", "master":
		return "master class"
	}
	return ""
}

// MasterOf is the master class a "master <name>" source names, as written
// ("Lich", "Storm Caller"); "" for any other source.
func MasterOf(source string) string {
	k := sourceKind(source)
	if len(k) > 7 && strings.EqualFold(k[:7], "master ") {
		return strings.TrimSpace(k[7:])
	}
	return ""
}

// Granted reports whether a class grant gives a classspells row of that
// source: every source but a mastery's, an epic's and a master class's.
func Granted(source string) bool { return GotBy(source) == "" }

// sourceKind is a source with "eqoa-data " taken off its front.
func sourceKind(source string) string {
	s := strings.TrimSpace(source)
	if len(s) > 10 && strings.EqualFold(s[:10], "eqoa-data ") {
		s = strings.TrimSpace(s[10:])
	}
	return s
}
