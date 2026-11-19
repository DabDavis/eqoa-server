package entity

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// The owner, 2026-11-18 (petition 2384): a Hardcore guild master's fall hands the lead to
// the highest member still playing (an officer first), never to another fallen member; the
// fallen stays on the roster as a member, and the change is kept. An officer's fall changes no
// ranks. A guild kept with a fallen master hands it on at the start; one of fallen members alone
// keeps its master. A master leaving later passes the lead past a fallen member too.
func TestAFallenHardcoreMasterHandsOnTheLead(t *testing.T) {
	w := NewWorld()
	store := &guildStoreLog{}
	w.LoadGuilds([]database.GuildRow{
		{ID: 1, Name: "VENOM", Leader: 10, Members: []database.GuildMemberRow{
			{ID: 10, Rank: GuildRankLeader, Name: "Anzu"}, {ID: 20, Rank: 2, Name: "Arthurius"},
			{ID: 21, Rank: 2, Name: "Festerfarts"}, {ID: 30, Rank: 1, Name: "Sporefarts"}, {ID: 40, Rank: 0, Name: "Newbie"}}},
	})
	w.SetGuildStore(store)
	anzu := &Player{ServerID: 5, CharServerID: 10, Name: "Anzu"}
	arth := &Player{ServerID: 6, CharServerID: 20, Name: "Arthurius"}
	w.mu.Lock()
	w.Players[anzu.ServerID], w.Players[arth.ServerID] = anzu, arth
	w.rejoinGuildLocked(anzu)
	w.rejoinGuildLocked(arth)
	w.mu.Unlock()

	if _, ok := w.GuildFall(21); ok { // an officer falls first: no ranks change
		t.Fatal("an officer's fall changed the lead")
	}
	c, ok := w.GuildFall(10)
	if !ok || c.NewLead.ID != 20 || c.Fallen.ID != 10 || c.Fallen.Rank != GuildRankMember {
		t.Fatalf("the master's fall: %+v %v, want Arthurius (20) to lead and Anzu a member", c, ok)
	}
	g := w.GuildByID(1)
	if g.Members[20].Rank != GuildRankLeader || g.Members[10].Rank != GuildRankMember || g.Members[21].Rank != 2 {
		t.Fatalf("ranks: Arthurius %d, Anzu %d, Festerfarts %d", g.Members[20].Rank, g.Members[10].Rank, g.Members[21].Rank)
	}
	if arth.GuildRank != GuildRankLeader || anzu.GuildRank != GuildRankMember {
		t.Fatalf("players' ranks: Arthurius %d, Anzu %d", arth.GuildRank, anzu.GuildRank)
	}
	if len(store.saved) != 1 || store.saved[0].Leader != 20 {
		t.Fatalf("kept: %+v, want one save led by 20", store.saved)
	}

	// Arthurius leaves: the lead passes over fallen Festerfarts (an officer) to Sporefarts.
	if _, _, lead, err := w.LeaveGuild(arth); err != nil || lead == nil || lead.ID != 30 {
		t.Fatalf("the master leaves: lead %+v %v, want Sporefarts (30), not the fallen officer", lead, err)
	}
}

func TestAGuildKeptWithAFallenMaster(t *testing.T) {
	w := NewWorld()
	w.LoadGuilds([]database.GuildRow{
		{ID: 1, Name: "Led By The Dead", Leader: 10, Members: []database.GuildMemberRow{
			{ID: 10, Rank: GuildRankLeader, Name: "Gone"}, {ID: 11, Rank: 1, Name: "Alive"}}},
		{ID: 2, Name: "All Fallen", Leader: 12, Members: []database.GuildMemberRow{
			{ID: 12, Rank: GuildRankLeader, Name: "GoneToo"}, {ID: 13, Rank: 0, Name: "AlsoGone"}}},
		{ID: 3, Name: "Fine", Leader: 14, Members: []database.GuildMemberRow{{ID: 14, Rank: GuildRankLeader, Name: "Here"}}},
	})
	if n := w.FallenGuildLeads([]uint32{10, 12, 13}); n != 1 {
		t.Fatalf("handed on in %d guilds, want 1", n)
	}
	if g := w.GuildByID(1); g.Members[11].Rank != GuildRankLeader || g.Members[10].Rank != GuildRankMember {
		t.Fatalf("Led By The Dead: Alive %d, Gone %d", g.Members[11].Rank, g.Members[10].Rank)
	}
	if g := w.GuildByID(2); g.Members[12].Rank != GuildRankLeader {
		t.Fatalf("All Fallen kept its master: %d", g.Members[12].Rank)
	}
	if g := w.GuildByID(3); g.Members[14].Rank != GuildRankLeader {
		t.Fatalf("Fine: %d", g.Members[14].Rank)
	}
}
