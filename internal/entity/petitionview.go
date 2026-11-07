package entity

import (
	"math"
	"sort"
	"time"

	"github.com/eqoa/server/internal/session"
)

// PetitionView is the server's own view of a player at the moment it files a
// petition (handlers/petitionview.go keeps it with the petition, for the GMs
// alone): where it stands and how, what it targets and how that stands, #stats'
// lines on both, and what its client is shown around it (its entity channels).
type PetitionView struct {
	Character PetitionCharacter `json:"character"`
	Stats     []string          `json:"stats"`
	Target    *PetitionEntity   `json:"target,omitempty"`
	Shown     []PetitionEntity  `json:"shown,omitempty"`
}

// PetitionCharacter is the filing character as the server holds it.
type PetitionCharacter struct {
	Name      string  `json:"name"`
	ServerID  uint32  `json:"serverid"`
	Level     int     `json:"level"`
	Class     string  `json:"class"`
	Race      string  `json:"race"`
	World     uint8   `json:"world"`
	X         float32 `json:"x"`
	Y         float32 `json:"y"`
	Z         float32 `json:"z"`
	Facing    float32 `json:"facing"`
	HP        int     `json:"hp"`
	MaxHP     int     `json:"maxHP"`
	Power     int     `json:"power"`
	MaxPower  int     `json:"maxPower"`
	Dead      bool    `json:"dead,omitempty"`
	Fighting  string  `json:"fighting,omitempty"`
	Group     uint32  `json:"group,omitempty"`
	Guild     string  `json:"guild,omitempty"`
	Tunar     int     `json:"tunar"`
	BankTunar int     `json:"bankTunar"`
	Effects   int     `json:"effects"`
}

// PetitionEntity is an entity the player targets or is shown: an NPC (with
// its database id, its spawn and whom it fights) or a player.
type PetitionEntity struct {
	Kind     string      `json:"kind"`
	Name     string      `json:"name"`
	ObjectID uint32      `json:"objectID,omitempty"`
	NPCID    int         `json:"npcid,omitempty"`
	ServerID uint32      `json:"serverid,omitempty"`
	Level    int         `json:"level"`
	HP       int         `json:"hp"`
	MaxHP    int         `json:"maxHP"`
	X        float32     `json:"x"`
	Y        float32     `json:"y"`
	Z        float32     `json:"z"`
	Distance float32     `json:"distance"`
	Dead     bool        `json:"dead,omitempty"`
	Fighting string      `json:"fighting,omitempty"`
	Spawn    *[3]float32 `json:"spawn,omitempty"`
	Stats    []string    `json:"stats,omitempty"`
}

// PetitionView is the session's player as the server holds it now, false when
// it has none in the world.
func (w *World) PetitionView(sess *session.Session, now time.Time) (PetitionView, bool) {
	w.mu.Lock() // npcStatsLocked keeps what it works out (as StatsReport)
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return PetitionView{}, false
	}
	v := PetitionView{
		Character: PetitionCharacter{
			Name: p.Name, ServerID: p.CharServerID, Level: p.Level,
			Class: nameIn(classNames, p.Class), Race: RaceName(p.Race),
			World: p.World, X: p.X, Y: p.Y, Z: p.Z, Facing: p.Facing,
			HP: p.HP, MaxHP: p.MaxHPNow(), Power: p.Power, MaxPower: p.MaxPowerNow(),
			Dead: p.HP <= 0 || !p.DiedAt.IsZero(), Fighting: w.petitionNameLocked(p.AttackTarget),
			Group: p.GroupID, Guild: p.GuildName, Tunar: p.Tunar, BankTunar: p.BankTunar, Effects: len(p.Effects),
		},
		Stats: w.playerStatsLocked(p, now),
	}
	if id := p.TargetObjectID; id != 0 && id != 0xFFFFFFFF {
		if ent, ok := w.findEntityByObjectIDLocked(sess, id); ok {
			if t, ok := w.petitionEntityLocked(ent, p); ok {
				t.ObjectID = id
				switch e := ent.(type) {
				case *NPC:
					t.Stats = w.npcStatsLocked(e, p, now)
				case *Player:
					t.Stats = w.playerStatsLocked(e, now)
				}
				v.Target = &t
			}
		}
	}
	if vis, ok := sess.VisibilityState().(*VisibilityState); ok && vis != nil {
		for i, id := range vis.ChannelEntity {
			if id == 0 || id == p.ServerID {
				continue
			}
			var ent Entity
			if n, ok := w.NPCs[id]; ok {
				ent = n
			} else if o, ok := w.Players[id]; ok {
				ent = o
			} else {
				continue
			}
			if e, ok := w.petitionEntityLocked(ent, p); ok {
				e.ObjectID = vis.ChannelObjectID[i]
				v.Shown = append(v.Shown, e)
			}
		}
		sort.Slice(v.Shown, func(a, b int) bool { return v.Shown[a].Distance < v.Shown[b].Distance })
	}
	return v, true
}

// petitionEntityLocked is an NPC or a player as a petition's view lists it,
// its distance from p.
func (w *World) petitionEntityLocked(ent Entity, p *Player) (PetitionEntity, bool) {
	dist := func(x, y, z float32) float32 {
		dx, dy, dz := float64(x-p.X), float64(y-p.Y), float64(z-p.Z)
		return float32(math.Round(math.Sqrt(dx*dx+dy*dy+dz*dz)*10) / 10)
	}
	switch e := ent.(type) {
	case *NPC:
		spawn := [3]float32{e.SpawnX, e.SpawnY, e.SpawnZ}
		return PetitionEntity{Kind: "npc", Name: e.Name, NPCID: e.NPCID, Level: e.Level, HP: e.HP, MaxHP: e.MaxHP,
			X: e.X, Y: e.Y, Z: e.Z, Distance: dist(e.X, e.Y, e.Z), Dead: e.Dead,
			Fighting: w.topThreatNameLocked(e), Spawn: &spawn}, true
	case *Player:
		return PetitionEntity{Kind: "player", Name: e.Name, ServerID: e.CharServerID, Level: e.Level, HP: e.HP,
			MaxHP: e.MaxHPNow(), X: e.X, Y: e.Y, Z: e.Z, Distance: dist(e.X, e.Y, e.Z),
			Fighting: w.petitionNameLocked(e.AttackTarget)}, true
	}
	return PetitionEntity{}, false
}

// petitionNameLocked is the name of the entity of id, "" for none or one gone.
func (w *World) petitionNameLocked(id EntityID) string {
	if id == 0 {
		return ""
	}
	if n, ok := w.NPCs[id]; ok {
		return n.Name
	}
	if p, ok := w.Players[id]; ok {
		return p.Name
	}
	return ""
}

// topThreatNameLocked is whom an NPC fights: the name at the top of its hate
// list, "" for none.
func (w *World) topThreatNameLocked(n *NPC) string {
	var top EntityID
	var most int64 = math.MinInt64
	for id, hate := range n.Threats {
		if hate > most || (hate == most && id < top) {
			top, most = id, hate
		}
	}
	return w.petitionNameLocked(top)
}
