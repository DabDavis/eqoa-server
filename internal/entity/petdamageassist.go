package entity

// A mob can hit a player without targeting them or putting them on its threat
// list. Keep the actual participants for the pet AI without changing mob hate.
type petDamagePair struct {
	player *Player
	mob    *NPC
}

func (w *World) notePetDamageLocked(player *Player, mob *NPC, damage int) {
	if damage <= 0 || mob.HP <= 0 || (len(w.pets) == 0 && len(w.sidePets) == 0) {
		return
	}
	if w.petDamage == nil {
		w.petDamage = make(map[petDamagePair]uint64)
	}
	w.petDamage[petDamagePair{player, mob}] = w.TickCount
}

// The ordinary target picker checks the pet's stance, existing fight, range,
// Back Off and quest walls before it considers an exchange of damage.
func (w *World) petAssistsDamageLocked(ps *petState, player *Player, mob *NPC) bool {
	if ps.stance != petAggressive {
		return false
	}
	_, hit := w.petDamage[petDamagePair{player, mob}]
	return hit && SamePlace(ps.owner, player) && w.npcSharesLocked(mob, ps.owner) && w.npcsShareLocked(ps.npc, mob)
}
