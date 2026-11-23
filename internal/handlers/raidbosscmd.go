package handlers

import (
	"strings"

	"github.com/eqoa/server/internal/session"
)

// raidbosscmd.go — #raidboss, a GM's look at the raid bosses up now (a GM's
// petition 1947; entity/heroes.go's lines). The owner, 2026-11-22: "improve the
// /raidmobs (bosses?) command to only show actual raid bosses like uzra and
// poxathixius, lord nagafen etc, bertoxulous throne etc. Right now it shows
// tons of random raid mobs, not bosses".
//
//   - #raidboss lists the bosses of the GMs' list alone (table raidbosslist, the
//     panel's Raid boss list: database/raidbosslist.go), read at each use, so a
//     GM's row is in the next one.
//   - #raidboss all lists what it did until then: every creature the raid
//     loot's rule takes for a raid boss (a class template of a raid's hit
//     points, a boss loot profile or table): The Hunt's wildlife, a raid's
//     trash and its nameds as well as its bosses.
//
// The raid loot's need before greed and the Testing Heroes' credit keep that
// rule: the list is only what #raidboss shows.

// raidBossListStore is where #raidboss reads its bosses (database.SQLiteDB).
type raidBossListStore interface {
	RaidBossList() ([]string, error)
}

// SetRaidBossListStore gives #raidboss its list. Without it #raidboss lists
// as #raidboss all does.
func (h *ChatHandler) SetRaidBossListStore(s raidBossListStore) { h.raidBossList = s }

// raidBossCommand answers #raidboss [all].
func (h *ChatHandler) raidBossCommand(sess *session.Session, args []string) {
	all := len(args) > 0 && strings.EqualFold(args[0], "all")
	if len(args) > 0 && !all {
		h.gmReply(sess, "#raidboss: the raid bosses up now; #raidboss all: every creature the raid loot takes for one")
		return
	}
	if all || h.raidBossList == nil {
		lines := h.gameWorld.RaidBossesUp()
		if len(lines) == 0 {
			h.gmReply(sess, "No raid boss is up.")
		}
		for _, l := range lines {
			h.gmReply(sess, l)
		}
		return
	}
	names, err := h.raidBossList.RaidBossList()
	if err != nil {
		h.gmReply(sess, "#raidboss: the raid boss list could not be read: "+err.Error())
		return
	}
	if len(names) == 0 {
		h.gmReply(sess, "No raid boss is listed (the GM panel's Raid boss list); #raidboss all lists every creature the raid loot takes for one.")
		return
	}
	listed := make(map[string]bool, len(names))
	for _, n := range names {
		listed[n] = true
	}
	lines := h.gameWorld.ListedRaidBossesUp(listed)
	if len(lines) == 0 {
		h.gmReply(sess, "No raid boss is up.")
	}
	for _, l := range lines {
		h.gmReply(sess, l)
	}
}
