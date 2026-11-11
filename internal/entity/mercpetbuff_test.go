package entity

import (
	"os"
	"slices"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// necroPetBook is a necromancer's pet spells as the book has them: three
// ranks of one line (Reinforce Bone, Death Mastery, Soul Mastery: STR STA
// DEX, the two higher a heal over time too), two forms of one AC buff
// (Strengthen Bone, 30 and 5 minutes), Largesse of Plague's proc, and what a
// merc never puts on its pet: a run speed and a blast at it.
func necroPetBook() []CastSpell {
	sp := func(id int, name string, level int, cast time.Duration, line string) CastSpell {
		s := bookSpell(id, name, 3, cast, 2*time.Second, 5, line)
		s.Level = level
		return s
	}
	// Largesse's proc as the workbook reads it (its row's effects)
	largesse := sp(700794, "Largesse of Plague", 60, 2*time.Second, "Pet, 5min")
	largesse.Def.Effects = []SpellEffect{{Kind: EffectProc, Amount: -650, Resist: StatDR}}
	return []CastSpell{
		sp(900876, "Reinforce Bone", 1, 3*time.Second, "Pet, Increase STR by 10, Increase STA by 10, Increase DEX by 10, 5min"),
		sp(888, "Strengthen Bone +", 7, 3*time.Second, "Pet, Increase AGI by 30, Increase AC by 70, 30min"),
		sp(900887, "Strengthen Bone", 7, 3*time.Second, "Pet, Increase AGI by 15, Increase AC by 35, 5min"),
		sp(900951, "Death Mastery", 43, 3*time.Second, "Pet, Increase STR by 35, Increase STA by 35, Increase DEX by 35, Increase HP per tic by 50, 10min"),
		sp(700684, "Soul Mastery", 51, 3*time.Second, "Pet, Increase STR by 45, Increase STA by 45, Increase DEX by 45, Increase HP per tic by 75, 10min"),
		largesse,
		sp(700229, "Velocity", 20, 2*time.Second, "Pet, Increase Movement Speed by 130, 5min"),
		sp(900045, "Destroy Contraption 1", 24, time.Second, "Pet, Decrease Hitpoints by 20000, Increase HP per tic by 50, 24sec"),
	}
}

// petBuffsOn is the spell ids of the effects on the pet, sorted.
func petBuffsOn(pet *NPC) []int {
	var ids []int
	for _, e := range pet.Effects {
		ids = append(ids, e.SpellID)
	}
	slices.Sort(ids)
	return ids
}

// The pet book's lines (mercpetbuff.go): one buff a line, its best.
func TestMercPetBookLines(t *testing.T) {
	ms := &mercState{kit: MercKit{Spells: necroPetBook()}, body: &Player{}}
	var got []int
	for _, s := range ms.mercPetBook().buffs {
		got = append(got, s.SpellID)
	}
	if want := []int{700794, 700684, 888}; !slices.Equal(got, want) {
		t.Errorf("pet buffs %v, want %v (Largesse, Soul Mastery, Strengthen Bone +)", got, want)
	}
}

// A necromancer merc keeps its pet buffed (the owner, 2026-11-11: "for 60
// nec largesse of plague, soul mastery, the ac buff"): its pet summoned, it
// casts Largesse of Plague, Soul Mastery and Strengthen Bone + on it, each at
// its cast time and paying its power, and none of the lower ranks, the
// shorter AC form, the run speed or the blast; Largesse worn off (5 minutes)
// as the hirer is hurt, the hirer is healed first, then Largesse cast again;
// the pet killed and summoned again, the new one is buffed.
func TestMercKeepsItsPetBuffed(t *testing.T) {
	r := newMercRig(t)
	r.w.mu.Lock()
	r.w.putNPCLocked(&NPC{ServerID: 4900, Name: "Fire Elemental", Level: 39, HP: 2340, MaxHP: 2340, ModelID: 0x8A416241, Size: 1,
		NPCType: 0x80, X: 20000, Z: 20000, World: 0})
	r.w.mu.Unlock()
	summon := bookSpell(717, "Fire Elemental", 0, 8*time.Second, 45*time.Second, 30, "Summons a Fire Elemental to aid you")
	summon.Level = 28
	heal := bookSpell(50, "Minor Healing", 1, 2*time.Second, 2*time.Second, 5, "Increase Hitpoints by 100")
	ms := r.hire(r.kit(11, 0, 0, append([]CastSpell{summon, heal}, necroPetBook()...)...), MercAssist)
	full := func() {
		r.w.mu.Lock()
		ms.npc.Power = ms.npc.MaxPower
		r.w.mu.Unlock()
	}
	at := time.Duration(0)
	for ; at <= 10*time.Second && ms.pet == nil; at += 100 * time.Millisecond {
		r.tick(at)
	}
	if ms.pet == nil {
		t.Fatal("no pet after the summoning")
	}
	pet := ms.pet.npc

	for ; at < 40*time.Second; at += 100 * time.Millisecond {
		full()
		r.tick(at)
	}
	if got, want := petBuffsOn(pet), []int{888, 700684, 700794}; !slices.Equal(got, want) {
		t.Fatalf("the pet's effects %v, want %v", got, want)
	}
	for _, id := range []int{900876, 900887, 900951, 700229, 900045} {
		if _, ok := ms.ready[id]; ok {
			t.Errorf("the merc cast %d on its pet", id)
		}
	}
	if pet.Dead || pet.HP <= 0 {
		t.Error("the pet died of its buffs")
	}
	cast := ms.ready[700794] // its lockout from its cast

	// Largesse wears off (5 minutes) as the hirer is hurt: the hirer's
	// heal first, then Largesse again
	r.w.mu.Lock()
	for _, e := range pet.Effects {
		if e.SpellID == 700794 {
			e.Until = r.at(at)
		}
	}
	r.p.HP = r.p.MaxHPNow() / 4
	r.w.mu.Unlock()
	for _, ok := ms.ready[50]; !ok && len(pet.Effects) == 3; _, ok = ms.ready[50] {
		full()
		r.tick(at)
		at += 100 * time.Millisecond
	}
	if _, ok := ms.ready[50]; !ok {
		t.Fatal("the hurt hirer was not healed")
	}
	if !ms.ready[700794].Equal(cast) {
		t.Fatal("Largesse cast again before the hurt hirer's heal")
	}
	r.w.mu.Lock()
	r.p.HP = r.p.MaxHPNow()
	r.w.mu.Unlock()
	for end := at + 10*time.Second; at < end; at += 100 * time.Millisecond {
		full()
		r.tick(at)
	}
	if !ms.ready[700794].After(cast) {
		t.Error("Largesse of Plague was not cast again once it wore off")
	}
	if got := petBuffsOn(pet); !slices.Contains(got, 700794) {
		t.Errorf("after it wore off the pet's effects %v lack Largesse", got)
	}

	// a new pet is buffed anew
	r.w.mu.Lock()
	pet.HP = 0
	r.w.killNPC(pet)
	r.w.mu.Unlock()
	for end := at + 90*time.Second; at < end && (ms.pet == nil || ms.pet.npc == pet || len(ms.pet.npc.Effects) < 3); at += 100 * time.Millisecond {
		full()
		r.tick(at)
	}
	if ms.pet == nil || ms.pet.npc == pet {
		t.Fatal("no new pet")
	}
	if got, want := petBuffsOn(ms.pet.npc), []int{888, 700684, 700794}; !slices.Equal(got, want) {
		t.Errorf("the new pet's effects %v, want %v", got, want)
	}
}

// A magician merc heals its hurt pet before it buffs it (its pet heals:
// Infuse, Fusion; the biggest first). The control: a pet at full health is
// not healed.
func TestMercHealsItsPet(t *testing.T) {
	r := newMercRig(t)
	ms := mercWithPet(t, r, MercAssist)
	pet := ms.pet.npc
	small := bookSpell(638, "Infuse", 3, 2*time.Second, 4*time.Second, 5, "Pet, Increase Hitpoints by 15")
	big := bookSpell(700785, "Fusion", 3, 2*time.Second, 6*time.Second, 5, "Pet, Increase Hitpoints by 722")
	big.Level = 60
	buff := bookSpell(700703, "Agitate", 3, 2*time.Second, time.Second, 5, "Pet, Increase STR by 80, Increase STA by 80, 5min")
	buff.Level = 55
	r.w.mu.Lock()
	ms.kit.Spells = append(ms.kit.Spells, small, big, buff)
	ms.book, ms.petBook = nil, nil
	ms.npc.Power = ms.npc.MaxPower
	r.w.mu.Unlock()
	r.tick(10100 * time.Millisecond)
	if _, ok := ms.ready[700785]; ok {
		t.Fatal("a pet at full health was healed")
	}
	if _, ok := ms.ready[700703]; !ok {
		t.Fatal("no Agitate on the pet")
	}
	r.run(10200*time.Millisecond, 14*time.Second)
	r.w.mu.Lock()
	pet.HP = pet.MaxHP / 2
	for _, e := range pet.Effects {
		e.Until = r.at(14 * time.Second) // Agitate off too: the heal goes first
	}
	ms.npc.Power = ms.npc.MaxPower
	hp := pet.HP
	r.w.mu.Unlock()
	r.tick(14100 * time.Millisecond)
	if _, ok := ms.ready[700785]; !ok {
		t.Fatal("the hurt pet was not healed with Fusion first")
	}
	if _, ok := ms.ready[638]; ok {
		t.Error("Infuse begun beside Fusion")
	}
	r.run(14200*time.Millisecond, 19*time.Second)
	if pet.HP <= hp {
		t.Errorf("the pet's hit points %d, %d before Fusion", pet.HP, hp)
	}
}

// On a real database (EQOA_REAL_DB, a copy): a level 60 necromancer merc's
// pet buffs are Largesse of Plague, Soul Mastery and one Strengthen Bone; a
// magician's include no run speed and one of each of its lines.
func TestRealMercPetBuffs(t *testing.T) {
	path := os.Getenv("EQOA_REAL_DB")
	if path == "" {
		t.Skip("EQOA_REAL_DB unset")
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, class := range []int{8, 10, 11, 12, 13} {
		book, err := SimBookFromDB(db, class, 60, false)
		if err != nil {
			t.Fatal(err)
		}
		ms := &mercState{kit: MercKit{Spells: book}, body: &Player{}}
		pb := ms.mercPetBook()
		var names, heals []string
		for _, s := range pb.buffs {
			names = append(names, s.Name)
			if keys := petBuffKeys(s); slices.Contains(keys, "speed") {
				t.Errorf("%s: a run speed among the pet buffs", ClassName(class))
			}
		}
		for _, s := range pb.heals {
			heals = append(heals, s.Name)
		}
		t.Logf("%s 60: pet buffs %q, pet heals %q", ClassName(class), names, heals)
		if class == 11 {
			slices.Sort(names)
			if want := []string{"Largesse of Plague", "Soul Mastery", "Strengthen Bone +"}; !slices.Equal(names, want) {
				t.Errorf("necromancer 60: %q, want %q", names, want)
			}
		}
	}
}
