// concolor.go — the consider colours: what a player sees a creature as, by their
// levels (0x0010's con byte, handlers/target.go), and the colour aggro.go
// keeps creatures off a player at on sight (green).

package entity

// The consider colours, as 0x0010 carries them.
const (
	ConRed byte = iota
	ConYellow
	ConWhite
	ConDarkBlue
	ConLightBlue
	ConGreen
)

// ConColor is the colour a player of observerLevel sees a target of
// targetLevel as. Red three or more above, yellow one or two, white even,
// dark blue one or two below; further below light blue while the kill still
// pays experience, green once it pays none (expLimit, killexp.go). Live
// petition 5690 (2026-11-28, a player of the live game): "The con system was
// in sync with the xp system in that if it conned green it gave no xp. Every
// other con gave xp." The C# server's GenerateConColor (green past a quarter
// of the level, or three under up to 15) drew green one level early where a
// quarter rounds down (a level 20 creature at 27: paid, yet green).
func ConColor(observerLevel, targetLevel int) byte {
	diff := observerLevel - targetLevel // positive: the observer is higher
	switch {
	case diff <= -3:
		return ConRed
	case diff <= -1:
		return ConYellow
	case diff == 0:
		return ConWhite
	case diff <= 2:
		return ConDarkBlue
	case targetLevel < expLimit(observerLevel):
		return ConGreen
	default:
		return ConLightBlue
	}
}

// greenTo reports npc green to p: one it leaves alone on sight (aggro.go).
func greenTo(npc *NPC, p *Player) bool {
	return ConColor(p.Level, npc.Level) == ConGreen
}
