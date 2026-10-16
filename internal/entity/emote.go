package entity

import "github.com/eqoa/server/internal/session"

// Emote actions, CLIENT's PlayEmote (0x00BCFC38 [V]): the opcode it sends
// and the VICSprite action it plays on itself.
const (
	ActionWave  = 56 // 0x001E
	ActionBow   = 57 // 0x0042
	ActionPoint = 60 // 0x0043
	ActionCheer = 61 // 0x0044
)

// ShowEmote shows the sender's emote to every other player who can see it:
// 0x0775 (u32 id, u8 action), which CLIENT's dispatcher plays on that entity
// when it is a character (case 0x0775 [V]; on the client's own entity only
// action 34 plays, so the sender gets nothing — it played the emote itself).
// The id is the one each viewer knows the sender by, its visibility channel's.
// It returns how many saw it.
func (w *World) ShowEmote(sess *session.Session, action byte) int {
	w.mu.RLock()
	defer w.mu.RUnlock()

	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return 0
	}
	shown := 0
	w.SessionManager.ForEach(func(viewer *session.Session) bool {
		if viewer == sess || !viewer.CharacterInWorld() {
			return true
		}
		if id := w.objectIDForEntityLocked(viewer, p.ServerID); id != 0 {
			w.sendEntityAction(viewer, id, action)
			shown++
		}
		return true
	})
	return shown
}
