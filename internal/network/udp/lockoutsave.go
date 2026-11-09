package udp

import (
	"log"
	"time"

	"github.com/eqoa/server/internal/session"
)

// saveLockouts keeps the player's ability lockouts still running over the
// logout, each by when it ends (entity/lockoutsave.go): the time logged out
// counts, so the next login is refused what is still locked out and nothing
// more. As saveEffects, an old session's Player is not saved (the login that
// took it over has it).
func (l *Listener) saveLockouts(sess *session.Session) {
	player := l.gameWorld.SessionPlayer(sess)
	if player == nil || !l.gameWorld.IsLivePlayer(player) {
		return
	}
	rows, ok := l.gameWorld.SavedLockouts(player.ServerID, time.Now())
	if !ok {
		return
	}
	if err := l.charStore.SaveLockouts(player.CharServerID, rows); err != nil {
		log.Printf("saveLockouts: '%s' %d lockouts: %v", player.Name, len(rows), err)
	} else if len(rows) > 0 {
		log.Printf("saveLockouts: '%s' keeps %d abilities locked out", player.Name, len(rows))
	}
}
