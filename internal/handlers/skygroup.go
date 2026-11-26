package handlers

// The Plane of Sky's chains: each player does its own steps (the owner,
// 2026-11-26, Heirloom petition 3146: "Talker only"). From 2026-11-23 (Live
// petition 5510) a Sky chain step one player did was done too for each
// groupmate by it on that step, and a chain one took was taken by them; a talk
// to Sterie Glitterlight moved a whole group on, which a player reported as
// "updating multiple characters' quests at the same time". That rule is gone:
// a talk, a give, a show, a reach and a taken quest are the doer's alone. What
// a group shares is what every quest's group shares: a kill credited to the
// killer counts on the kill step of each member of its group (a raid's too) in
// the creature's world within entity.KillShareRange (200) of it, alive or lying
// dead, and of a member who died in that fight wherever it is
// (entity/questhooks.go). A passage still carries the group by it, and a
// carried member's own passage step is done with its own key (skypass.go), as
// before 5510.

// skyChainQuests are the chains: rain (101301-101303), the sun and day
// (101304-101305), night (101306), snow (101307), desolation (101308).
var skyChainQuests = map[int]bool{101301: true, 101302: true, 101303: true, 101304: true, 101305: true,
	101306: true, 101307: true, 101308: true}
