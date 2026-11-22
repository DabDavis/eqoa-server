package handlers

import (
	"log"
	"math"
	"sync"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// lightsphere.go — teleporting spheres known by their place (work board 773
// step 40: Highbourne library's three spheres of light under the guilds'
// banners, and one back in each guild house). They have no name on the live
// game, so a waysender's route by name (waysender.go) cannot hold them: table
// lightspheres keys each by where it stands. A talk to an NPC standing within
// database.LightSphereNear of a row's place opens CLIENT's question box with
// the row's line and its one answer ("Touch the light."; the box's own Exit
// closes it); the answer sends the player as a waysender's single trip does
// (PendingYesNo, travel: no fee, no level, any class), landing in front of the
// other sphere and facing it. The rows are read at most lightSphereReread old,
// so a GM's edit counts within it, and a talk never waits on the database
// otherwise. Nothing here is kept over a restart: an unanswered box's answer
// after one is an old box's.

// lightSphereReread is how old the rows may be before a talk reads them again.
const lightSphereReread = 30 * time.Second

// lightSpheres is the rows, read at most lightSphereReread old.
type lightSpheres struct {
	mu   sync.Mutex
	read func() ([]database.LightSphere, error)
	rows []database.LightSphere
	at   time.Time
	now  func() time.Time
}

// SetLightSpheres gives the handler lightspheres' reader (database.LightSpheres).
func (h *InteractHandler) SetLightSpheres(read func() ([]database.LightSphere, error)) {
	h.spheres.mu.Lock()
	h.spheres.read, h.spheres.at = read, time.Time{}
	h.spheres.mu.Unlock()
}

// at is the row whose place npc stands at, if any.
func (s *lightSpheres) of(npc *entity.NPC) (database.LightSphere, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.read == nil {
		return database.LightSphere{}, false
	}
	now := time.Now()
	if s.now != nil {
		now = s.now()
	}
	if s.at.IsZero() || now.Sub(s.at) >= lightSphereReread {
		rows, err := s.read()
		if err != nil {
			log.Printf("LightSphere: rows: %v", err)
		} else {
			s.rows = rows
		}
		s.at = now
	}
	for _, r := range s.rows {
		dx, dz := float64(npc.SpawnX-r.X), float64(npc.SpawnZ-r.Z)
		if r.World == int(npc.World) && dx*dx+dz*dz <= database.LightSphereNear*database.LightSphereNear {
			return r, true
		}
	}
	return database.LightSphere{}, false
}

// lightSphereTalk is a talk to a sphere: its line and its answer, the trip
// waiting on it. It reports whether npc is one.
func (h *InteractHandler) lightSphereTalk(sess *session.Session, player *entity.Player, npc *entity.NPC) bool {
	r, ok := h.spheres.of(npc)
	if !ok {
		return false
	}
	if !CanLoadWorld(sess, uint8(r.ToWorld)) {
		log.Printf("LightSphere: %s's client has no world %d (sphere %d)", player.Name, r.ToWorld, r.ID)
		return false
	}
	h2 := float32(float64(r.ToFacing) / 256 * 2 * math.Pi)
	fx, fz, face := inFrontOf(r.ToX, r.ToZ, h2)
	forgetDialogs(player)
	player.PendingCoachDests = []entity.CoachDest{{Name: "the light", X: fx, Y: r.ToY, Z: fz, Facing: face,
		World: uint8(r.ToWorld)}}
	player.PendingYesNo = true
	answer := r.Answer
	if answer == "" {
		answer = "Touch the light."
	}
	h.sendOptionBox(sess, r.Line, []string{answer})
	log.Printf("LightSphere: %s at sphere %d", player.Name, r.ID)
	return true
}
