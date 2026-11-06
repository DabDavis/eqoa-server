package database

import (
	"reflect"
	"testing"
)

// Guilds kept and read back: each guild's row and its members with their
// characters' names, classes and levels; a save replaces the members whole
// and leaves out a member whose character does not exist; a stale row under
// a name taken again goes; a delete takes the guild out; a guild none of
// whose characters exist is not read back. The control: a guild never saved
// is not there.
func TestGuildsKept(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/guilds.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewSQLiteCharacterStore(db)
	ids := map[string]uint32{}
	for _, c := range []struct {
		name         string
		class, level int
	}{{"Dasher", 0, 60}, {"Zaeire", 6, 60}, {"Larrybird", 3, 60}, {"Pozme", 9, 30}} {
		rec := &CharacterRecord{CharName: c.name, AccountID: 1, TClass: c.class, Level: c.level}
		if err := store.Create(rec); err != nil {
			t.Fatal(err)
		}
		ids[c.name] = rec.ServerID
	}
	bad := GuildRow{ID: 1, Name: "Bad Boys", Charter: "hi", Leader: ids["Dasher"], Members: []GuildMemberRow{
		{ID: ids["Dasher"], Rank: 3}, {ID: ids["Zaeire"], Rank: 2}, {ID: ids["Larrybird"], Rank: 2}, {ID: 99999, Rank: 0}}}
	if err := db.SaveGuild(bad); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveGuild(GuildRow{ID: 2, Name: "Prophets of Zion", Leader: ids["Pozme"], Members: []GuildMemberRow{{ID: ids["Pozme"], Rank: 3}}}); err != nil {
		t.Fatal(err)
	}
	got, err := db.LoadGuilds()
	if err != nil || len(got) != 2 {
		t.Fatalf("loaded %+v %v", got, err)
	}
	want := []GuildMemberRow{{ID: ids["Dasher"], Rank: 3, Name: "Dasher", Level: 60}, {ID: ids["Zaeire"], Rank: 2, Name: "Zaeire", Class: 6, Level: 60},
		{ID: ids["Larrybird"], Rank: 2, Name: "Larrybird", Class: 3, Level: 60}}
	if got[0].Name != "Bad Boys" || got[0].Charter != "hi" || got[0].Leader != ids["Dasher"] || !reflect.DeepEqual(got[0].Members, want) {
		t.Fatalf("Bad Boys read back: %+v\nwant members %+v", got[0], want)
	}
	// a rank changed and Zaeire out: the members replaced whole
	bad.Members = []GuildMemberRow{{ID: ids["Dasher"], Rank: 3}, {ID: ids["Larrybird"], Rank: 1}}
	if err := db.SaveGuild(bad); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.LoadGuilds(); len(got[0].Members) != 2 || got[0].Members[1].Rank != 1 {
		t.Fatalf("after the change: %+v", got[0].Members)
	}
	// the name taken again under a new id (the world's old guild gone): the stale row goes
	if err := db.SaveGuild(GuildRow{ID: 5, Name: "bad boys", Leader: ids["Zaeire"], Members: []GuildMemberRow{{ID: ids["Zaeire"], Rank: 3}}}); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.LoadGuilds(); len(got) != 2 || got[1].ID != 5 || got[0].ID != 2 {
		t.Fatalf("after the name was taken again: %+v", got)
	}
	if err := db.DeleteGuild(5); err != nil {
		t.Fatal(err)
	}
	// Pozme's character deleted: its guild is not read back
	if err := store.Delete(ids["Pozme"]); err != nil {
		t.Fatal(err)
	}
	if got, err := db.LoadGuilds(); err != nil || len(got) != 0 {
		t.Fatalf("at the end: %+v %v", got, err)
	}
}
