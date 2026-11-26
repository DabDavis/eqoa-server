package entity

import (
	"fmt"
	"testing"
	"time"

	"github.com/eqoa/server/internal/session"
)

// tickBenchRig is newMercRig for a benchmark (its offbeat switch put back after).
func tickBenchRig(b *testing.B) *mercRig {
	offbeat := mercOffbeat
	b.Cleanup(func() { mercOffbeat = offbeat })
	return newMercRig(&testing.T{})
}

// Benchmarks of the two systems the tick caches are for (chancache.go,
// linecache.go), at a shard's size (pprof of Heirloom, 2026-11-26: ~68
// players): the same scenes run on a build before them for the figures.

// benchMercTeams is owners players each with three mercs (a shaman, a cleric
// and a necromancer with its pet), their buffs up after a minute.
func benchMercTeams(b *testing.B, owners int) (*mercRig, time.Duration) {
	r := tickBenchRig(b)
	brawn := bookSpell(700901, "Tribal Brawn", 1, 3*time.Second, 0, 20, "Target, Increase STR by 20, 30min")
	brawn.Level = mercOwnerLevel
	breeze := bookSpell(700902, "Breeze", 2, 3*time.Second, 0, 20, "Group, Increase AGI by 10, Radius 30, 30min")
	breeze.Level = mercOwnerLevel - 2
	mend := bookSpell(700903, "Mend", 1, 2*time.Second, 2*time.Second, 10, "Increase Hitpoints by 300")
	dward := bookSpell(700904, "Disease Ward", 1, 2*time.Second, 0, 10, "Target, Increase DR by 15, 30min")
	dward.Level = 20
	pward := bookSpell(700905, "Poison Ward", 1, 2*time.Second, 0, 10, "Target, Increase DR by 20, Increase PR by 20, 30min")
	pward.Level = 26
	aegis := bookSpell(700906, "Aegis", 0, 2*time.Second, 0, 10, "Self, Increase AC by 40, Increase STA by 10, 30min")
	aegis.Level = 28
	summon := bookSpell(717, "Bone Servant", 0, 4*time.Second, 45*time.Second, 30, "Summons a Fire Elemental to aid you")
	summon.Level = 28
	r.w.mu.Lock()
	r.w.putNPCLocked(&NPC{ServerID: 4900, Name: "Fire Elemental", Level: 28, HP: 2000, MaxHP: 2000, ModelID: 0x8A416241, Size: 1,
		NPCType: 0x80, X: 20000, Z: 20000, World: 0})
	r.w.mu.Unlock()
	for o := 0; o < owners; o++ {
		sess, p := r.sess, r.p
		if o > 0 {
			sess, p = r.player(fmt.Sprintf("Owner%02d", o), o%15, 0)
			r.w.mu.Lock()
			r.w.SpatialIndex.Remove(p.ServerID, p.X, p.Z)
			p.X, p.Z = 1000+float32(o%10)*200, 1000+float32(o/10)*200 // each team its own camp
			r.w.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
			r.w.mu.Unlock()
		}
		for slot, k := range []MercKit{r.kit(8, 3, 1, brawn, breeze, mend), r.kit(9, 0, 0, mend, dward, pward, aegis),
			r.kit(11, 0, 0, append([]CastSpell{summon}, necroPetBook()...)...)} {
			k.Name = fmt.Sprintf("M%d_%d", o, slot)
			if _, why, _ := r.w.HireMercAt(sess, slot+1, k, MercAssist, r.at(0)); why != "" {
				b.Fatalf("owner %d slot %d: %s", o, slot+1, why)
			}
			r.w.mu.Lock()
			_, up := r.w.invites[p.ServerID][p.ServerID]
			r.w.mu.Unlock()
			if up {
				r.w.GroupAccept(sess, sess.PlayerObjectID())
			}
		}
	}
	d := time.Duration(0)
	for ; d < 60*time.Second; d += 100 * time.Millisecond {
		r.tick(d)
	}
	return r, d
}

// BenchmarkMercSystem is mercSystem's turn over 24 teams (72 mercs).
func BenchmarkMercSystem(b *testing.B) {
	r, d := benchMercTeams(b, 24)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d += 100 * time.Millisecond
		r.w.mu.Lock()
		r.w.TickCount++
		r.w.mercSystem(r.at(d))
		r.w.mu.Unlock()
	}
}

// benchChannelCrowd is players players in a town, each seeing creatures
// creatures and the others, through the whole tick's view.
func benchChannelCrowd(b *testing.B, players, creatures int) *mercRig {
	r := tickBenchRig(b)
	for i := 1; i < players; i++ {
		s, p := r.player(fmt.Sprintf("Crowd%02d", i), i%15, i%10)
		r.w.mu.Lock()
		r.w.SpatialIndex.Remove(p.ServerID, p.X, p.Z)
		p.X, p.Z = 1000+float32(i%10)*3, 1000+float32(i/10)*3
		r.w.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
		r.w.mu.Unlock()
		if i%3 != 1 { // two in three vanilla clients, every look
			conn := s.Connection()
			conn.SetEntitySlots(60)
			s.SetLiveLook(true)
			s.SetArmorLook(true)
			s.SetPieceAuras(true)
			s.SetBodyAura(true)
			s.SetCapeAura(true)
			s.SetHandAura(true)
			s.SetSecondAura(true)
			s.SetHatAura(true)
		}
	}
	for i := 0; i < creatures; i++ {
		n := r.creature(fmt.Sprintf("a townsfolk %d", i), 10+i%40, float32(i%15)*2)
		r.w.mu.Lock()
		r.w.SpatialIndex.Remove(n.ServerID, n.X, n.Z)
		n.Z += float32(i/15) * 2
		r.w.SpatialIndex.Insert(n.ServerID, n.X, n.Z)
		r.w.mu.Unlock()
	}
	r.w.mu.Lock()
	r.w.TickCount = VisibilityTickInterval - 1
	r.w.mu.Unlock()
	r.w.Tick(r.at(100 * time.Millisecond)) // the view's pass
	return r
}

// BenchmarkEntityChannelSync is entityChannelSyncSystem over 68 players in a
// town with 150 creatures.
func BenchmarkEntityChannelSync(b *testing.B) {
	r := benchChannelCrowd(b, 68, 150)
	r.w.mu.Lock()
	used := 0
	r.mgr.ForEach(func(s *session.Session) bool {
		for _, id := range GetOrCreateVisibility(s).ChannelEntity {
			if id != 0 {
				used++
			}
		}
		return true
	})
	r.w.mu.Unlock()
	b.ReportMetric(float64(used), "channels")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.w.mu.Lock()
		r.w.entityChannelSyncSystem()
		r.w.mu.Unlock()
	}
}
