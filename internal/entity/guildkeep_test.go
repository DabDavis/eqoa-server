package entity

import (
	"reflect"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// guildStoreLog records what the World asks its guild store to keep.
type guildStoreLog struct {
	saved   []database.GuildRow
	deleted []uint32
}

func (s *guildStoreLog) SaveGuild(g database.GuildRow) error {
	s.saved = append(s.saved, g)
	return nil
}
func (s *guildStoreLog) DeleteGuild(id uint32) error { s.deleted = append(s.deleted, id); return nil }

// Every guild change is kept as it is made (the user, 2026-11-06: "did
// larrybirds guild not get saved"): a guild made, joined, a rank and the
// charter changed, each written whole (its leader, its members' ranks); the
// last member out ends it, and so does a disband. The control: a refused
// change writes nothing.
func TestGuildChangesAreKept(t *testing.T) {
	w := NewWorld()
	store := &guildStoreLog{}
	w.SetGuildStore(store)
	dasher := &Player{ServerID: 1, CharServerID: 101, Name: "Dasher"}
	zaeire := &Player{ServerID: 2, CharServerID: 102, Name: "Zaeire"}
	w.Players[dasher.ServerID], w.Players[zaeire.ServerID] = dasher, zaeire
	g, err := w.CreateGuild("Bad Boys", dasher)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.JoinGuild(g, zaeire); err != nil {
		t.Fatal(err)
	}
	if _, _, err := w.SetGuildRank(dasher, 102, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := w.SetCharter(dasher, "Bad boys, bad boys"); err != nil {
		t.Fatal(err)
	}
	// refused: a member raising its leader
	if _, _, err := w.SetGuildRank(zaeire, 101, 1); err == nil {
		t.Fatal("a member ranked its leader")
	}
	if len(store.saved) != 4 {
		t.Fatalf("%d writes, want 4 (made, joined, rank, charter): %+v", len(store.saved), store.saved)
	}
	last := store.saved[3]
	want := database.GuildRow{ID: g.ID, Name: "Bad Boys", Charter: "Bad boys, bad boys", Leader: 101,
		Members: []database.GuildMemberRow{{ID: 101, Rank: GuildRankLeader}, {ID: 102, Rank: 1}}}
	if !reflect.DeepEqual(last, want) {
		t.Fatalf("kept %+v\nwant %+v", last, want)
	}
	// the leader leaves: Zaeire leads, kept; the last out ends it
	if _, ended, _, err := w.LeaveGuild(dasher); err != nil || ended {
		t.Fatalf("the leader leaving: ended %v %v", ended, err)
	}
	if got := store.saved[len(store.saved)-1]; got.Leader != 102 || len(got.Members) != 1 || got.Members[0].Rank != GuildRankLeader {
		t.Fatalf("after the leader left: %+v", got)
	}
	if _, ended, _, err := w.LeaveGuild(zaeire); err != nil || !ended || !reflect.DeepEqual(store.deleted, []uint32{g.ID}) {
		t.Fatalf("the last out: ended %v %v, deleted %v", ended, err, store.deleted)
	}
	// a disband ends a guild too
	g2, _ := w.CreateGuild("Prophets of Zion", dasher)
	if _, _, err := w.DisbandGuild(dasher); err != nil || store.deleted[len(store.deleted)-1] != g2.ID {
		t.Fatalf("disbanded: %v, deleted %v", err, store.deleted)
	}
}

// Kept guilds come back at the start: a member made after rejoins its guild
// at its rank with its charter, and a new guild takes the next id after the
// kept ones; a guild kept without a leader (its leader's character gone)
// gets its highest member as leader.
func TestKeptGuildsComeBack(t *testing.T) {
	w := NewWorld()
	n := w.LoadGuilds([]database.GuildRow{
		{ID: 7, Name: "Bad Boys", Charter: "hi", Leader: 101, Members: []database.GuildMemberRow{
			{ID: 101, Rank: GuildRankLeader, Name: "Dasher"}, {ID: 186, Rank: 2, Name: "Larrybird", Level: 60}}},
		{ID: 3, Name: "Orphans", Members: []database.GuildMemberRow{{ID: 50, Rank: 1, Name: "Lone"}, {ID: 51, Rank: 0, Name: "Other"}}},
	})
	if n != 2 {
		t.Fatalf("loaded %d", n)
	}
	larry := &Player{ServerID: 9, CharServerID: 186, Name: "Larrybird", Level: 60}
	w.mu.Lock()
	w.Players[larry.ServerID] = larry
	w.rejoinGuildLocked(larry)
	w.mu.Unlock()
	if larry.GuildID != 7 || larry.GuildName != "Bad Boys" || larry.GuildRank != 2 || larry.GuildMOTD != "hi" {
		t.Fatalf("Larrybird back: guild %d %q rank %d motd %q", larry.GuildID, larry.GuildName, larry.GuildRank, larry.GuildMOTD)
	}
	if lead := w.GuildByID(3).Members[50]; lead.Rank != GuildRankLeader {
		t.Fatalf("the leaderless guild's highest: rank %d", lead.Rank)
	}
	newbie := &Player{ServerID: 10, CharServerID: 300, Name: "Newbie"}
	w.Players[newbie.ServerID] = newbie
	if g, err := w.CreateGuild("Fresh", newbie); err != nil || g.ID != 8 {
		t.Fatalf("a new guild after the kept: %+v %v", g, err)
	}
}
