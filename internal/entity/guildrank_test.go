package entity

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// Petition 2976 (Lazarus, 2026-11-25): "I'm unable as a master to promote above elder."
// SetGuildRank asks what CLIENT's CanPromote and CanDemote ask (0x00BEE8B8, 0x00BEE980):
// promoting needs two ranks above the member, or a master over anyone below master, so a
// master may make an elder a master beside it, as the reference server's guilds had several
// (3 of 250 in the captures). Demoting needs a rank above, so masters never demote each other,
// and an elder still cannot make an officer an elder.
func TestAMasterMakesAnElderAMaster(t *testing.T) {
	w := NewWorld()
	store := &guildStoreLog{}
	w.LoadGuilds([]database.GuildRow{
		{ID: 23, Name: "Rites of the Four Horsemen", Leader: 2546, Members: []database.GuildMemberRow{
			{ID: 2546, Rank: GuildRankLeader, Name: "Lazarus"}, {ID: 3042, Rank: 2, Name: "Tootielovr"},
			{ID: 3154, Rank: 1, Name: "Pestilence"}, {ID: 2802, Rank: 0, Name: "Gor"}}},
	})
	w.SetGuildStore(store)
	laz := &Player{ServerID: 5, CharServerID: 2546, Name: "Lazarus"}
	toot := &Player{ServerID: 6, CharServerID: 3042, Name: "Tootielovr"}
	w.mu.Lock()
	w.Players[laz.ServerID], w.Players[toot.ServerID] = laz, toot
	w.rejoinGuildLocked(laz)
	w.rejoinGuildLocked(toot)
	w.mu.Unlock()
	g := w.GuildByID(23)

	// An elder (2) cannot make an officer (1) an elder: one rank above is not enough.
	if _, _, err := w.SetGuildRank(toot, 3154, 1); err != ErrNotQualified {
		t.Fatalf("an elder promoting an officer: %v, want not qualified", err)
	}
	// The master makes the elder a master.
	if _, m, err := w.SetGuildRank(laz, 3042, 1); err != nil || m.Rank != GuildRankLeader {
		t.Fatalf("the master promoting the elder: %+v %v, want a master", m, err)
	}
	if toot.GuildRank != GuildRankLeader || g.Members[2546].Rank != GuildRankLeader {
		t.Fatalf("ranks: Tootielovr %d, Lazarus %d, want both masters", toot.GuildRank, g.Members[2546].Rank)
	}
	if n := len(store.saved); n == 0 || store.saved[n-1].Leader != 2546 {
		t.Fatalf("kept: %+v, want the lowest id's master as leader_id", store.saved)
	}
	// No rank past master; masters do not demote each other.
	if _, _, err := w.SetGuildRank(laz, 3042, 1); err != ErrNotQualified {
		t.Fatalf("promoting a master: %v", err)
	}
	if _, _, err := w.SetGuildRank(toot, 2546, -1); err != ErrNotQualified {
		t.Fatalf("a master demoting a master: %v", err)
	}
	// The new master may promote too, and set the charter.
	if _, m, err := w.SetGuildRank(toot, 3154, 1); err != nil || m.Rank != 2 {
		t.Fatalf("the second master promoting an officer: %+v %v", m, err)
	}
	if _, err := w.SetCharter(toot, "Ride"); err != nil {
		t.Fatalf("the second master's charter: %v", err)
	}
	// One master leaving leaves the other leading: no one else is raised.
	if _, ended, lead, err := w.LeaveGuild(laz); err != nil || ended || lead != nil {
		t.Fatalf("a master leaves: ended %v lead %+v %v, want the other master kept, none raised", ended, lead, err)
	}
	if g.Members[3154].Rank != 2 || g.Members[3042].Rank != GuildRankLeader {
		t.Fatalf("after: Pestilence %d, Tootielovr %d", g.Members[3154].Rank, g.Members[3042].Rank)
	}
}

// A guild kept with two masters, one fallen: the fallen one steps down at the start, whichever
// of them leaderLocked would have found.
func TestAKeptGuildWithAFallenCoMaster(t *testing.T) {
	w := NewWorld()
	w.LoadGuilds([]database.GuildRow{
		{ID: 1, Name: "Two Heads", Leader: 10, Members: []database.GuildMemberRow{
			{ID: 10, Rank: GuildRankLeader, Name: "Here"}, {ID: 11, Rank: GuildRankLeader, Name: "Gone"},
			{ID: 12, Rank: 0, Name: "Member"}}},
	})
	if n := w.FallenGuildLeads([]uint32{11}); n != 1 {
		t.Fatalf("changed %d guilds, want 1", n)
	}
	g := w.GuildByID(1)
	if g.Members[10].Rank != GuildRankLeader || g.Members[11].Rank != GuildRankMember || g.Members[12].Rank != 0 {
		t.Fatalf("ranks: Here %d, Gone %d, Member %d", g.Members[10].Rank, g.Members[11].Rank, g.Members[12].Rank)
	}
}
