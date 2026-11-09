package entity

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The number tags (numbertags.go): what made each figure, one byte after the
// retail 0x00DB, to a session that asked, for its player's own figures and its
// pet's; none to any other, nor on a figure another made.

// figTag is one 0x00DB as a client reads it: its body and the byte after it,
// -1 for none (the retail body alone).
type figTag struct {
	src    uint32
	amount int
	dst    uint32
	tag    int
}

func (f figTag) String() string {
	if f.tag < 0 {
		return fmt.Sprintf("%08X %+d %08X untagged", f.src, f.amount, f.dst)
	}
	return fmt.Sprintf("%08X %+d %08X tag %#02x", f.src, f.amount, f.dst, f.tag)
}

// figTags drains s's queue and returns its 0x00DBs. A body with more than one
// byte after it fails the test: a tag is one byte.
func figTags(t *testing.T, s *session.Session) []figTag {
	t.Helper()
	var out []figTag
	for _, m := range s.Queue().GetPending() {
		b := m.Data()
		if len(b) < 2 || protocol.GameOpcode(binary.LittleEndian.Uint16(b)) != protocol.OpcodeDamageIndication {
			continue
		}
		src := binary.LittleEndian.Uint32(b[2:])
		u, n := binary.Uvarint(b[6:])
		amount := int(int64(u>>1) ^ -int64(u&1))
		dst := binary.LittleEndian.Uint32(b[6+n:])
		f := figTag{src: src, amount: amount, dst: dst, tag: -1}
		switch rest := b[6+n+4:]; len(rest) {
		case 0:
		case 1:
			f.tag = int(rest[0])
		default:
			t.Fatalf("0x00DB % X: %d bytes after its body", b, len(rest))
		}
		out = append(out, f)
	}
	return out
}

// tags is each figure's tag alone.
func tags(fs []figTag) []int {
	out := make([]int, len(fs))
	for i, f := range fs {
		out[i] = f.tag
	}
	return out
}

// The wire: a session that answered 1 is sent the retail body and one byte,
// the kind, the pet's bit on a pet's; one that never answered, and any figure
// of no kind, the retail body alone, byte for byte what it was before. (The
// control is the session that never answered: the same calls, no byte.)
func TestNumberTagsOnTheWire(t *testing.T) {
	c := newCaster(t)
	c.sess.SetNumberTags(true)
	plain := c.w.SessionManager.CreateSession(c.sess.RemoteAddr(), 9, 9, 3, 4, true)
	plain.SetPlayerObjectID(c.me)
	// the retail body of a 120 hit on the creature: DB 00, the attacker, zigzag
	// -120 (EF 01), the defender
	body := unhex(t, "db00d0a12414ef01982e3101")
	if want := damageMsg(t, c.me, -120, c.npcO); !bytes.Equal(body, want) {
		t.Fatalf("the fixture's body % X, want % X", body, want)
	}
	kinds := []numberKind{numberMelee, numberSpell, numberDoT, numberShield, numberProc, numberHeal, numberHoT}
	for _, k := range kinds {
		for _, pet := range []bool{false, true} {
			kind := k
			if pet {
				kind = k.byPet()
			}
			c.w.sendDamageIndication(c.sess, c.me, -120, c.npcO, kind)
			c.w.sendDamageIndication(plain, c.me, -120, c.npcO, kind)
			got, gotPlain := c.sent(), plain.Queue().GetPending()
			want := append(append([]byte(nil), body...), byte(k)|map[bool]byte{false: 0, true: 0x80}[pet])
			if len(got) != 1 || !bytes.Equal(got[0], want) {
				t.Errorf("kind %#02x: sent % X, want % X", kind, got, want)
			}
			if len(gotPlain) != 1 || !bytes.Equal(gotPlain[0].Data(), body) {
				t.Errorf("kind %#02x to a session that never answered: % X, want the retail % X", kind, gotPlain[0].Data(), body)
			}
		}
	}
	// a heal: zigzag +75 (96 01), its byte after
	c.w.sendDamageIndication(c.sess, c.me, 75, c.me, numberHeal)
	if got := c.sent(); len(got) != 1 || !bytes.Equal(got[0], unhex(t, "db00d0a124149601d0a1241406")) {
		t.Errorf("a heal on oneself: % X", got)
	}
	// none (0), and the pet's bit alone, are no kind: the retail body
	for _, k := range []numberKind{0, numberByPet} {
		c.w.sendDamageIndication(c.sess, c.me, -120, c.npcO, k)
		if got := c.sent(); len(got) != 1 || !bytes.Equal(got[0], body) {
			t.Errorf("kind %#02x: % X, want the retail % X", k, got, body)
		}
	}
	// stopped (the answer 0): the retail body again
	c.sess.SetNumberTags(false)
	c.w.sendDamageIndication(c.sess, c.me, -120, c.npcO, numberMelee)
	if got := c.sent(); len(got) != 1 || !bytes.Equal(got[0], body) {
		t.Errorf("after the answer 0: % X", got)
	}
}

// numberKind's own rules: byPet sets bit 7 on a kind and leaves none none; a
// cast's figures make each kind (a gift's none).
func TestNumberKinds(t *testing.T) {
	if numberSpell.byPet() != 0x82 || numberHoT.byPet() != 0x87 || numberKind(0).byPet() != 0 {
		t.Errorf("byPet: %#02x %#02x %#02x", numberSpell.byPet(), numberHoT.byPet(), numberKind(0).byPet())
	}
	for _, c := range []struct {
		f                      castFigures
		hit, heal, tick, htick numberKind
	}{
		{figuresSpell, numberSpell, numberHeal, numberDoT, numberHoT},
		{figuresProc, numberProc, numberProc, numberProc, numberProc},
		{figuresPet, 0x82, 0x86, 0x83, 0x87},
		{figuresGift, 0, 0, 0, 0},
	} {
		s := CastSpell{figures: c.f}
		ae := ActiveEffect{figures: c.f}
		if s.hitKind() != c.hit || s.healKind() != c.heal || ae.tickKind(false) != c.tick || ae.tickKind(true) != c.htick {
			t.Errorf("figures %d: hit %#02x heal %#02x tick %#02x heal tick %#02x, want %#02x %#02x %#02x %#02x",
				c.f, s.hitKind(), s.healKind(), ae.tickKind(false), ae.tickKind(true), c.hit, c.heal, c.tick, c.htick)
		}
	}
}

// tagRig is newProcRig's enchanter and friend, both asking for their tags,
// each seeing the other, the enchanter short of its hit points (a heal shows),
// and a pet of the enchanter's beside the creature.
type tagRig struct {
	*procRig
	pet   *NPC
	ps    *petState
	petO  uint32 // the pet as the enchanter knows it
	meF   uint32 // the enchanter as the friend knows it
	later time.Time
}

func newTagRig(t *testing.T) *tagRig {
	r := newProcRig(t)
	r.sess.SetNumberTags(true)
	r.fsess.SetNumberTags(true)
	r.p.HP, r.p.MaxHP = 300, 536
	r.friend.HP = 500
	pet := &NPC{ServerID: 2500, Name: "Pet", Level: 20, HP: 400, MaxHP: 800, Owner: r.p.ServerID, X: 101, Y: 10, Z: 100}
	r.w.NPCs[pet.ServerID] = pet
	ps := &petState{owner: r.p, npc: pet, stance: petAggressive}
	if r.w.pets == nil {
		r.w.pets = make(map[EntityID]*petState)
	}
	r.w.pets[r.p.ServerID] = ps
	vs := GetOrCreateVisibility(r.sess)
	vs.ChannelEntity[2], vs.ChannelObjectID[2] = pet.ServerID, 0x0150BEE0
	fv := GetOrCreateVisibility(r.fsess)
	fv.ChannelEntity[1], fv.ChannelObjectID[1] = r.p.ServerID, 0x0140BBB1
	r.sent()
	r.fsess.Queue().GetPending()
	return &tagRig{procRig: r, pet: pet, ps: ps, petO: 0x0150BEE0, meF: 0x0140BBB1, later: r.at(time.Hour)}
}

// nuke is a cast of the enchanter's of these effects at the creature.
func (r *tagRig) cast(effects ...SpellEffect) *castState {
	return &castState{spell: CastSpell{SpellID: 9100, Name: "Test Spell", Level: 20, Scope: 1,
		Def: SpellDef{Effects: effects, Harmful: effects[0].Amount < 0, Duration: 18 * time.Second}},
		caster: r.p.ServerID, targets: []EntityID{r.npc.ServerID}}
}

// Every path that makes a figure, told as what made it (numbertags.go's
// list): the enchanter's own swing, spells, ticks, shields, procs and heals,
// its pet's, and the figures of others' making on it, untagged. Each case
// names the tags the enchanter's session gets, in order, and the friend's.
func TestNumberTagsByPath(t *testing.T) {
	defer func(f func(int) int) { procIntn = f }(procIntn)
	const none = -1
	hit := func(n int) SpellEffect { return SpellEffect{Kind: EffectHP, Amount: -n, AmountMax: -n} }
	heal := func(n int) SpellEffect { return SpellEffect{Kind: EffectHP, Amount: n, AmountMax: n} }
	tick := func(n int) SpellEffect { return SpellEffect{Kind: EffectHPTick, Amount: n, AmountMax: n} }
	for _, c := range []struct {
		name         string
		run          func(r *tagRig)
		mine, friend []int
	}{
		{"a swing", func(r *tagRig) { r.w.playerHitLocked(r.sess, r.p, r.npc, r.npcO) }, []int{1}, nil},
		{"a nuke", func(r *tagRig) { r.w.landOnNPCLocked(r.cast(hit(50)), r.p, r.sess, r.npc, r.t0) }, []int{2}, nil},
		{"a damage over time's tick", func(r *tagRig) {
			r.w.landOnNPCLocked(r.cast(tick(-20)), r.p, r.sess, r.npc, r.t0)
		}, []int{3}, nil},
		{"a life tap: the hit and the take", func(r *tagRig) {
			e := hit(40)
			e.Drain = DrainCaster
			r.w.landOnNPCLocked(r.cast(e), r.p, r.sess, r.npc, r.t0)
		}, []int{2, 6}, nil},
		{"a drain over time: the tick and the take", func(r *tagRig) {
			e := tick(-20)
			e.Drain = DrainCaster
			r.w.landOnNPCLocked(r.cast(e), r.p, r.sess, r.npc, r.t0)
		}, []int{3, 7}, nil},
		{"a damage shield's answer", func(r *tagRig) {
			r.p.Effects = []*ActiveEffect{{SpellID: 9200, Name: "Test Shield", Until: r.later, Shield: &Shield{Min: 12, Max: 12}}}
			r.w.playerShieldsLocked(r.sess, r.p, r.npc, r.t0)
		}, []int{4}, nil},
		{"a defensive proc", func(r *tagRig) {
			procIntn = dice(0, 99)
			r.p.Effects = []*ActiveEffect{{SpellID: 9201, Name: "Test Ward", Until: r.later, Shield: &Shield{ProcMin: 30, ProcMax: 30}}}
			r.w.playerShieldsLocked(r.sess, r.p, r.npc, r.t0)
		}, []int{5}, nil},
		{"an effect's proc and its life steal", func(r *tagRig) {
			procIntn = dice(0, 99)
			r.p.Effects = []*ActiveEffect{{SpellID: 9202, Name: "Test Flux", Until: r.later, Proc: 40, ProcMax: 40, Rules: EffectRules{ProcHeal: 25}}}
			r.w.playerProcsLocked(r.sess, r.p, r.npc, r.t0)
		}, []int{5, 5}, nil},
		{"a proc heal at a hit", func(r *tagRig) {
			procIntn = dice(0)
			r.p.Effects = []*ActiveEffect{{SpellID: 9203, Name: "Test Aura", Until: r.later, Rules: EffectRules{ProcHeal: 25, ProcHealDef: true}}}
			r.w.defenceProcsLocked(r.sess, r.p, r.npc, r.t0)
		}, []int{5}, nil},
		{"a weapon's proc", func(r *tagRig) {
			procIntn = dice(0, 99)
			r.p.Gear.MainProc = &WeaponProc{Item: "Test Blade", Kind: ProcHit, Damage: 33, Chance: 100}
			r.w.weaponProcLocked(r.sess, r.p, r.npc, r.t0)
		}, []int{5}, nil},
		{"a weapon's tap", func(r *tagRig) {
			procIntn = dice(0, 99)
			r.p.Gear.MainProc = &WeaponProc{Item: "Test Saw", Kind: ProcTap, Damage: 25, Chance: 100}
			r.w.weaponProcLocked(r.sess, r.p, r.npc, r.t0)
		}, []int{5, 5}, nil},
		{"a weapon's damage over time", func(r *tagRig) {
			procIntn = dice(0, 99)
			r.p.Gear.MainProc = &WeaponProc{Item: "Test Tongue", Kind: ProcDoT, Damage: 9, Chance: 100}
			r.w.weaponProcLocked(r.sess, r.p, r.npc, r.t0)
		}, []int{5}, nil},
		{"a weapon's restore, on its wielder and its group", func(r *tagRig) {
			procIntn = dice(0, 99)
			r.friend.HP = 100
			g := &Group{ID: 77, Members: []EntityID{r.p.ServerID, r.friend.ServerID}}
			if r.w.groups == nil {
				r.w.groups = make(map[uint32]*Group)
			}
			r.w.groups[g.ID] = g
			r.p.GroupID, r.friend.GroupID = g.ID, g.ID
			r.p.Gear.MainProc = &WeaponProc{Item: "Test Mace", Kind: ProcRestore, Damage: 20, Chance: 100}
			r.w.weaponProcLocked(r.sess, r.p, r.npc, r.t0)
		}, []int{5, 5}, []int{none}},
		{"a worn item's answer and heal", func(r *tagRig) {
			r.p.Gear.Shields = []WornShield{{Item: "Test Crown", Damage: 30, Heal: 15}}
			r.w.wornShieldsLocked(r.sess, r.p, r.npc)
		}, []int{5, 5}, nil},
		{"a mark's heal at a weapon's hit", func(r *tagRig) {
			r.npc.Effects = []*ActiveEffect{{SpellID: 9204, Name: "Test Mark", Until: r.later, Rules: EffectRules{MarkHeal: 65}}}
			r.w.markHealPlayerLocked(r.sess, r.p, r.npc, r.t0)
		}, []int{5}, nil},
		{"a spell proc: its heal for its bearer, its hit, its tick (a workbook's first, twice)", func(r *tagRig) {
			procIntn = dice(0, 99)
			own := heal(22)
			own.Self = true
			r.p.Effects = []*ActiveEffect{{SpellID: 9205, Name: "Test Fervor", Until: r.later,
				ProcSpells: []ProcSpell{{Chance: 100, Duration: 12 * time.Second, Effects: []SpellEffect{own, hit(44), tick(-6)}}}}}
			r.w.procSpellsLocked(r.p, r.sess, r.npc, false, r.t0)
		}, []int{5, 5, 5, 5}, nil},
		{"a heal on oneself", func(r *tagRig) {
			r.w.landOnPlayerLocked(r.cast(heal(100)), r.p, r.sess, r.p, r.t0)
		}, []int{6}, nil},
		{"a heal on a friend: the healer's, the friend's untagged", func(r *tagRig) {
			r.w.landOnPlayerLocked(r.cast(heal(100)), r.p, r.sess, r.friend, r.t0)
		}, []int{6}, []int{none}},
		{"a heal over time's tick", func(r *tagRig) {
			r.w.landOnPlayerLocked(r.cast(tick(30)), r.p, r.sess, r.p, r.t0)
		}, []int{7}, nil},
		{"a heal over time whose caster is gone: the bearer's figure, untagged", func(r *tagRig) {
			ae := &ActiveEffect{SpellID: 9206, Name: "Test Rejuv", Caster: 4242, Until: r.later, NextTick: r.t0, HPTick: 30}
			r.friend.Effects = []*ActiveEffect{ae}
			r.w.runTickLocked(r.friend, ae, r.t0)
		}, nil, []int{none}},
		{"a heal on its pet", func(r *tagRig) {
			r.w.landOnPetLocked(r.cast(heal(100)), r.p, r.sess, r.pet, r.t0)
		}, []int{6}, nil},
		{"a spell's own part for its caster", func(r *tagRig) {
			own := heal(40)
			own.Self = true
			r.w.landOnCasterLocked(r.cast(own, hit(10)), r.p, r.sess, r.t0)
		}, []int{6}, nil},
		{"a spell's price on its caster: untagged", func(r *tagRig) {
			own := hit(30)
			own.Self = true
			r.w.landOnCasterLocked(r.cast(own, hit(10)), r.p, r.sess, r.t0)
		}, []int{none}, nil},
		{"a gift landed as its own cast (a town board's): untagged", func(r *tagRig) {
			r.w.mu.Unlock()
			defer r.w.mu.Lock()
			r.w.LandOnSelf(r.sess, CastSpell{SpellID: 9207, Name: "Test Gift", Def: SpellDef{Effects: []SpellEffect{heal(50)}}}, r.t0)
		}, []int{none}, nil},
		{"a training dummy's hit and heal on it: untagged", func(r *tagRig) {
			r.npc.Dummy, r.npc.Name, r.npc.Level = TrainingDummy, "Training Dummy Lv20", 20
			r.p.AC = 600
			r.w.dummyHitLocked(r.sess, r.npc, r.p, r.t0)
		}, []int{none, none}, nil},
		{"the sun on a vampire: untagged", func(r *tagRig) { r.w.sunBurnLocked(r.sess, r.p, 50, r.t0) }, []int{none}, nil},
		{"its pet's swing", func(r *tagRig) { r.w.petHitLocked(r.ps, r.npc, r.t0) }, []int{0x81}, nil},
		{"its pet's taunt", func(r *tagRig) { r.w.petTauntLocked(r.ps, r.npc, r.t0) }, []int{0x82}, nil},
		{"its pet's damage over time's tick", func(r *tagRig) {
			ae := &ActiveEffect{SpellID: 9208, Name: "Test Swarm", Caster: r.p.ServerID, pet: r.pet.ServerID, Until: r.later, NextTick: r.t0, HPTick: -15}
			r.npc.Effects = []*ActiveEffect{ae}
			r.w.tickNPCEffectLocked(r.npc, ae, r.t0)
		}, []int{0x83}, nil},
		{"its pet's damage shield", func(r *tagRig) {
			r.pet.Effects = []*ActiveEffect{{SpellID: 9209, Name: "Test Shield", Until: r.later, Shield: &Shield{Min: 10, Max: 10}}}
			r.w.petShieldsLocked(r.pet, r.npc, r.t0)
		}, []int{0x84}, nil},
		{"its pet's defensive proc", func(r *tagRig) {
			procIntn = dice(0, 99)
			r.pet.Effects = []*ActiveEffect{{SpellID: 9210, Name: "Test Ward", Until: r.later, Shield: &Shield{ProcMin: 9, ProcMax: 9}}}
			r.w.petShieldsLocked(r.pet, r.npc, r.t0)
		}, []int{0x85}, nil},
		{"its pet's effect's proc", func(r *tagRig) {
			procIntn = dice(0, 99)
			r.pet.Effects = []*ActiveEffect{{SpellID: 9211, Name: "Test Flux", Until: r.later, Proc: 20, ProcMax: 20}}
			r.w.petProcsLocked(r.ps, r.npc, r.t0)
		}, []int{0x85}, nil},
		{"its pet's heal on itself", func(r *tagRig) { r.w.tellPetHealLocked(r.ps, 40) }, []int{0x86}, nil},
		{"its pet's buff on it: the heal now and at its ticks", func(r *tagRig) {
			cs := &castState{spell: CastSpell{SpellID: 9212, Name: "Test Pet Buff", Level: 20,
				Def: SpellDef{Effects: []SpellEffect{heal(60), tick(25)}, Duration: 18 * time.Second}, figures: figuresPet},
				caster: r.p.ServerID, targets: []EntityID{r.p.ServerID}}
			r.w.landOnPlayerLocked(cs, r.p, r.sess, r.p, r.t0)
		}, []int{0x86, 0x87}, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newTagRig(t)
			r.w.mu.Lock()
			c.run(r)
			r.w.mu.Unlock()
			mine, friend := figTags(t, r.sess), figTags(t, r.fsess)
			if fmt.Sprint(tags(mine)) != fmt.Sprint(c.mine) && !(len(mine) == 0 && len(c.mine) == 0) {
				t.Errorf("the enchanter was told %v, want tags %v", mine, c.mine)
			}
			if fmt.Sprint(tags(friend)) != fmt.Sprint(c.friend) && !(len(friend) == 0 && len(c.friend) == 0) {
				t.Errorf("the friend was told %v, want tags %v", friend, c.friend)
			}
			// every tagged figure is told from the player's own id (CLIENT's
			// "made by me"), but a pet's heal on its owner, which comes from the pet
			for _, f := range mine {
				if f.tag >= 0 && f.src != r.me {
					t.Errorf("%v: tagged but not from the player (%08X)", f, r.me)
				}
			}
		})
	}
}

// A pet's heal on its owner comes from the pet (CLIENT draws it over the
// player) and is the pet's heal; the same figure to a session that never
// answered is the retail body.
func TestNumberTagsPetHealOnItsOwner(t *testing.T) {
	r := newTagRig(t)
	r.w.mu.Lock()
	r.w.petLandLocked(&petCast{ps: r.ps, target: r.p.ServerID, spell: &CastSpell{SpellID: 9213, Name: "Test Mend",
		Def: SpellDef{Effects: []SpellEffect{{Kind: EffectHP, Amount: 80, AmountMax: 80}}}}, a: &templateAbility{TemplateAbility: database.TemplateAbility{Kind: database.AbilityHeal}}}, r.t0)
	r.w.mu.Unlock()
	got := figTags(t, r.sess)
	if len(got) != 1 || got[0].tag != 0x86 || got[0].src != r.petO || got[0].dst != r.me || got[0].amount != 80 {
		t.Fatalf("the owner was told %v, want the pet's 80 on it, tag 0x86", got)
	}
}

// A duel's figures: the attacker's own tagged (its swing, its spell, its
// tick), its opponent's of them untagged, as a creature's on it are.
func TestNumberTagsInADuel(t *testing.T) {
	d := newDuelists(t)
	duel := d.start(t)
	d.sa.SetNumberTags(true)
	d.sb.SetNumberTags(true)
	d.w.mu.Lock()
	d.w.duelHurtLocked(duel, d.a, d.a.ServerID, d.b, 100, d.at(2*time.Second), numberMelee)
	cs := &castState{spell: CastSpell{SpellID: 9300, Name: "Test Bolt", Level: 30, Scope: 1, duel: true,
		Def: SpellDef{Effects: []SpellEffect{{Kind: EffectHP, Amount: -60, AmountMax: -60}, {Kind: EffectHPTick, Amount: -10, AmountMax: -10}},
			Harmful: true, Duration: 12 * time.Second}}, caster: d.a.ServerID, targets: []EntityID{d.b.ServerID}}
	d.w.duelLandLocked(cs, d.a, d.sa, d.b, d.at(3*time.Second)) // its hit, then its first tick at once
	d.w.mu.Unlock()
	if got := tags(figTags(t, d.sa)); fmt.Sprint(got) != "[1 2 3]" {
		t.Errorf("the attacker was told tags %v, want its swing, spell and tick [1 2 3]", got)
	}
	if got := tags(figTags(t, d.sb)); fmt.Sprint(got) != "[-1 -1 -1]" {
		t.Errorf("its opponent was told tags %v, want none of them tagged", got)
	}
}

// The same figures to a session that never answered: the retail body, every
// one (the control of the table above).
func TestNumberTagsNotAskedForNoneTagged(t *testing.T) {
	r := newTagRig(t)
	r.sess.SetNumberTags(false)
	r.w.mu.Lock()
	r.w.playerHitLocked(r.sess, r.p, r.npc, r.npcO)
	r.w.landOnNPCLocked(r.cast(SpellEffect{Kind: EffectHP, Amount: -50, AmountMax: -50}), r.p, r.sess, r.npc, r.t0)
	r.w.landOnPlayerLocked(r.cast(SpellEffect{Kind: EffectHP, Amount: 50, AmountMax: 50}), r.p, r.sess, r.p, r.t0)
	r.w.petHitLocked(r.ps, r.npc, r.t0)
	r.w.tellPetHealLocked(r.ps, 40)
	r.w.mu.Unlock()
	got := figTags(t, r.sess)
	if len(got) != 5 {
		t.Fatalf("told %v, want five figures", got)
	}
	for _, f := range got {
		if f.tag != -1 {
			t.Errorf("%v: tagged to a session that never asked", f)
		}
	}
}

// What lasts keeps whose its figures are over a logout (buffsave.go): a gift's
// heal over time comes back untagged, a proc's as a proc's.
func TestNumberTagsFiguresKeptOverALogout(t *testing.T) {
	r := newTagRig(t)
	r.p.Effects = []*ActiveEffect{
		{SpellID: 9400, Name: "Test Gift", Caster: r.p.ServerID, Until: r.later, NextTick: r.at(time.Second), HPTick: 30, figures: figuresGift},
		{SpellID: 9401, Name: "Test Fervor", Caster: r.p.ServerID, Until: r.later, NextTick: r.at(time.Second), HPTick: 20, figures: figuresProc},
		{SpellID: 9402, Name: "Test Regrowth", Caster: r.p.ServerID, Until: r.later, NextTick: r.at(time.Second), HPTick: 10},
	}
	rows := savedEffects(r.p, r.t0)
	r.p.Effects = nil
	if n := r.w.RestoreEffects(r.sess, rows, r.t0); n != 3 {
		t.Fatalf("restored %d of %d", n, len(rows))
	}
	want := map[int]castFigures{9400: figuresGift, 9401: figuresProc, 9402: figuresSpell}
	for _, ae := range r.p.Effects {
		if ae.figures != want[ae.SpellID] {
			t.Errorf("%q came back with figures %d, want %d", ae.Name, ae.figures, want[ae.SpellID])
		}
	}
}

// Buffer's spells land as the player's own casts on itself (buffer.go), but
// they are his: Avatar's heal is told untagged, though the player asked for
// its tags (the control: its own heal on itself, tagged 6, in the table).
func TestNumberTagsNPCBufferIsAGift(t *testing.T) {
	c := newBufferRig(t)
	c.sess.SetNumberTags(true)
	c.p.HP = 100
	if _, ok := c.w.StartBuffer(c.sess, c.npc.ServerID, bufferFive(), c.at(0)); !ok {
		t.Fatal("StartBuffer refused")
	}
	c.sent()
	c.bufferTick(time.Second)
	got := figTags(t, c.sess)
	if len(got) != 1 || got[0].amount != 300 || got[0].tag != -1 {
		t.Fatalf("Avatar's heal was told %v, want its 300 untagged", got)
	}
}
