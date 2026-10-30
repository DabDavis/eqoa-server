// con.go — the consider colours: what a player sees a creature as, by their
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
// targetLevel as: the C# server's GenerateConColor, which every 0x0010 reply
// in the captures agrees with (handlers' concolour_test.go). Red three or
// more above, yellow one or two, white even, dark blue one or two below;
// further below light blue, then green: more than three below up to level
// 15, more than a quarter of the observer's level above it.
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
	case observerLevel <= 15:
		if diff > 3 {
			return ConGreen
		}
		return ConLightBlue
	case diff > observerLevel/4:
		return ConGreen
	default:
		return ConLightBlue
	}
}

// greenTo reports npc green to p: one it leaves alone on sight (aggro.go).
func greenTo(npc *NPC, p *Player) bool {
	return ConColor(p.Level, npc.Level) == ConGreen
}
