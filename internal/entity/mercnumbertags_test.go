package entity

import (
	"testing"
	"time"
)

// A merc's figures (numbertags.go's numberByMerc, petition 1613): to a client
// that answered layout 2 its hits, ticks and heals on its owner carry the
// merc's bit; to one that answered 1 (every vanilla before) its hits go as its
// pet's and its heals untagged, as before; to one that never answered, the
// retail body alone.

func TestNumberKindWireByte(t *testing.T) {
	for _, c := range []struct {
		k      numberKind
		layout uint8
		want   int // -1 no byte
	}{
		{numberMelee, 0, -1},
		{numberMelee, 1, 0x01},
		{numberMelee, 2, 0x01},
		{numberMelee.byPet(), 1, 0x81},
		{numberMelee.byPet(), 2, 0x81},
		{numberMelee.byMerc(), 1, 0x81},
		{numberMelee.byMerc(), 2, 0x41},
		{numberHoT.byMerc(), 1, -1}, // layout 1 never had a merc's heal tagged
		{numberHeal.byMerc(), 1, -1},
		{numberProc.byMerc(), 1, 0x85},
		{numberHoT.byMerc(), 2, 0x47},
		{numberByMerc, 2, -1},
		{numberByPet | numberByMerc, 2, -1},
		{numberKind(0).byMerc(), 2, -1},
		{numberMelee.byPet().byMerc(), 2, 0x41}, // the merc's bit alone
	} {
		b, ok := c.k.wireByte(c.layout)
		got := -1
		if ok {
			got = int(b)
		}
		if got != c.want {
			t.Errorf("%#02x at layout %d: %#x, want %#x", byte(c.k), c.layout, got, c.want)
		}
	}
	if k := figuresMerc.kind(true, false); k != numberHeal|numberByMerc {
		t.Errorf("a merc's heal: %#02x", byte(k))
	}
	if k := figuresMerc.kind(false, true); k != numberDoT|numberByMerc {
		t.Errorf("a merc's tick: %#02x", byte(k))
	}
	if !numberSpell.byMerc().byHelper() || !numberSpell.byPet().byHelper() || numberSpell.byHelper() {
		t.Error("byHelper: the damage meter's pet share")
	}
}

func TestMercFiguresByLayout(t *testing.T) {
	const none = -1
	for _, c := range []struct {
		name   string
		layout uint8
		want   []int // hit, spell, tick, heal on its owner, its HoT's tick on its owner, the owner's pet's swing
	}{
		{"never answered (a PS2)", 0, []int{none, none, none, none, none, none}},
		{"layout 1 (an older vanilla)", 1, []int{0x81, 0x82, 0x83, none, none, 0x81}},
		{"layout 2", 2, []int{0x41, 0x42, 0x43, 0x46, 0x47, 0x81}},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newMercRig(t)
			r.sess.SetNumberTagsLayout(c.layout)
			heal := bookSpell(700187, "Field Dress", 1, 0, 0, 0, "Target, Increase Hitpoints by 125")
			hot := bookSpell(464, "Healing", 1, 0, 0, 0, "Target, Increase HP per tic by 125, 24sec")
			ms := r.hire(r.kit(9, 0, 0, heal, hot), MercPassive)
			crab := r.creature("a crab", 30, 3)
			const crabO, mercO = 0x01312E98, 0x0150BEE0
			vs := GetOrCreateVisibility(r.sess)
			vs.ChannelEntity[1], vs.ChannelObjectID[1] = crab.ServerID, crabO
			vs.ChannelEntity[2], vs.ChannelObjectID[2] = ms.npc.ServerID, mercO
			r.sess.Queue().GetPending()
			me := r.sess.PlayerObjectID()
			now := r.at(time.Second)

			r.w.mu.Lock()
			r.w.mercDamageLocked(ms, crab, 50, numberMelee, now)
			r.w.mercDamageLocked(ms, crab, 60, numberSpell, now)
			r.w.mercDamageLocked(ms, crab, 70, numberDoT, now)
			got := figTags(t, r.sess)
			r.p.HP = r.p.MaxHPNow() / 2
			r.w.mercHelpLocked(ms, &heal, r.p.ServerID, now)
			heals := figTags(t, r.sess)
			r.p.HP = r.p.MaxHPNow() / 2
			r.w.mercHelpLocked(ms, &hot, r.p.ServerID, now)
			var ae *ActiveEffect
			for _, e := range r.p.Effects {
				if e.SpellID == hot.SpellID {
					ae = e
				}
			}
			figTags(t, r.sess) // a first tick at its landing, if any
			if ae == nil {
				r.w.mu.Unlock()
				t.Fatal("the merc's heal over time did not land on its owner")
			}
			r.p.HP = r.p.MaxHPNow() / 2
			r.w.runTickLocked(r.p, ae, now.Add(6*time.Second))
			ticks := figTags(t, r.sess)
			r.w.sendDamageIndication(r.sess, me, -9, crabO, numberMelee.byPet()) // the control: the owner's own pet
			pet := figTags(t, r.sess)
			r.w.mu.Unlock()

			if len(got) != 3 || len(heals) != 1 || len(ticks) != 1 || len(pet) != 1 {
				t.Fatalf("figures: hits %v, heal %v, tick %v, pet %v", got, heals, ticks, pet)
			}
			for i, f := range got {
				if f.src != me || f.dst != crabO || f.amount != -[]int{50, 60, 70}[i] {
					t.Errorf("hit %d: %v, want from the owner on the crab", i, f)
				}
			}
			if f := heals[0]; f.src != mercO || f.dst != me || f.amount != 125 {
				t.Errorf("the heal on its owner: %v, want the merc's 125 on it", f)
			}
			if f := ticks[0]; f.dst != me || f.amount <= 0 {
				t.Errorf("its heal over time's tick: %v", f)
			}
			all := append(append(append(tags(got), heals[0].tag), ticks[0].tag), pet[0].tag)
			for i := range all {
				if all[i] != c.want[i] {
					t.Errorf("tags %#x, want %#x", all, c.want)
					break
				}
			}
		})
	}
}
