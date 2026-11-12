package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
	"github.com/eqoa/server/internal/worlds"
)

// siege.go — the siege of Zentar's Keep's places, its sign-up and its commands (entity/siege.go has the rules; the
// owner, 2026-11-12: "lets add a pvp zone for pvp events. zentaurs keep castle, it is siege mode, team inside has king
// with high hp. entrances have a door/wood prop as a mob that doesnt attack with high hp, when dead way is opened for
// attacking team to kill king, when king dies the attackers win"). The siege is the second arena game: capture the
// flag's engine (entity/ctfgames.go), its world.siege config, its world a server world (serverworlds.go) whose
// built recipe's "arena" block of game "siege" (tools/worlds/siege.json) is its places: each team's camp, the King
// and the gates. The PvP Events herald (ctf.go) signs players up for either game; /siege [join|leave|rules] (and
// #siege, and a PS2's "/t siege ...") says how it stands, signs up and leaves. A GM's #siege start|stop|time
// <minutes>|teams <n> (and #ctf the same for capture the flag: arenaGMCommand) runs an event by hand.

// siegeJSON is a siege recipe's "arena" block past the teams' names (arenaJSON has those).
type siegeJSON struct {
	Teams []struct {
		Camp arenaSpot `json:"camp"`
	} `json:"teams"`
	King  siegeNPCJSON   `json:"king"`
	Gates []siegeNPCJSON `json:"gates"`
}

// siegeNPCJSON is the King or a gate in the recipe.
type siegeNPCJSON struct {
	Name     string    `json:"name"`
	Original string    `json:"original"`
	At       arenaSpot `json:"at"`
	Body     dictID    `json:"body"`
	Size     float32   `json:"size"`
	Level    int       `json:"level"`
}

func (n siegeNPCJSON) layout() entity.SiegeNPCLayout {
	return entity.SiegeNPCLayout{Name: n.Name, Original: n.Original, At: n.At.spot(), Body: uint32(n.Body), Size: n.Size, Level: n.Level}
}

// ReadSiege is a built world's siege (its recipe's "arena" block of game "siege"), checked.
func ReadSiege(b worlds.Built) (entity.CTFLayout, error) {
	var lay entity.CTFLayout
	if len(b.Arena) == 0 {
		return lay, fmt.Errorf("world %d %q has no arena in its recipe", b.World, b.Name)
	}
	var a arenaJSON
	var sj siegeJSON
	if err := json.Unmarshal(b.Arena, &a); err != nil {
		return lay, fmt.Errorf("world %d's arena: %w", b.World, err)
	}
	if err := json.Unmarshal(b.Arena, &sj); err != nil {
		return lay, fmt.Errorf("world %d's arena: %w", b.World, err)
	}
	if a.Game != entity.ArenaSiege {
		return lay, fmt.Errorf("world %d's arena is for %q, not the siege", b.World, a.Game)
	}
	if len(a.Teams) != 2 {
		return lay, fmt.Errorf("world %d's siege has %d teams: the defenders and the attackers", b.World, len(a.Teams))
	}
	lay.Game = entity.ArenaSiege
	lay.Name = a.Name
	if lay.Name == "" {
		lay.Name = b.Name
	}
	for i, t := range a.Teams {
		camp := sj.Teams[i].Camp
		if t.Name == "" || t.Short == "" {
			return lay, fmt.Errorf("world %d's siege: team %d needs a name and a short name", b.World, i)
		}
		if camp.X == 0 && camp.Z == 0 {
			return lay, fmt.Errorf("world %d's siege: team %d's camp is at 0, 0", b.World, i)
		}
		lay.Teams[i] = entity.CTFTeamLayout{Name: t.Name, Short: t.Short, Camp: camp.spot(), Stand: camp.spot()}
	}
	check := func(what string, n siegeNPCJSON) error {
		switch {
		case n.Name == "":
			return fmt.Errorf("world %d's siege: %s has no name", b.World, what)
		case n.At.X == 0 && n.At.Z == 0:
			return fmt.Errorf("world %d's siege: %s is at 0, 0", b.World, what)
		case n.Original == "" && n.Body == 0:
			return fmt.Errorf("world %d's siege: %s has neither an original to copy nor a body", b.World, what)
		}
		return nil
	}
	if err := check("the King", sj.King); err != nil {
		return lay, err
	}
	lay.Siege.King = sj.King.layout()
	for i, g := range sj.Gates {
		if err := check(fmt.Sprintf("gate %d", i), g); err != nil {
			return lay, err
		}
		lay.Siege.Gates = append(lay.Siege.Gates, g.layout())
	}
	if len(a.Field) > 0 && len(a.Field) < 3 {
		return lay, fmt.Errorf("world %d's siege: a field of %d points", b.World, len(a.Field))
	}
	lay.Field = a.Field
	if c := a.FieldCircle; c != nil {
		if c.R <= 0 {
			return lay, fmt.Errorf("world %d's siege: a round field of radius %.1f", b.World, c.R)
		}
		lay.Ring = entity.CTFRing{X: c.X, Z: c.Z, Radius: c.R}
	}
	in := func(s entity.CTFSpot) bool { return lay.InField(s.X, s.Z) }
	if !in(lay.Teams[0].Camp) || !in(lay.Teams[1].Camp) || !in(lay.Siege.King.At) {
		return lay, fmt.Errorf("world %d's siege: a camp or the King is outside its field", b.World)
	}
	return lay, nil
}

// SiegeSettings are world.siege's numbers, each unset one the siege's default.
func SiegeSettings(c config.SiegeConfig) entity.CTFSettings {
	s := arenaSettings(entity.DefaultSiegeSettings(), c.CTFConfig)
	if c.KingHP > 0 {
		s.KingHP = c.KingHP
	}
	if c.GateHP > 0 {
		s.GateHP = c.GateHP
	}
	return s
}

// SetupSiege turns world.siege on at the start: its world must be one of the server's own built worlds with a
// siege (LoadServerWorlds first). It says what it did, and reports whether the siege is on.
func SetupSiege(gw *entity.World, c config.SiegeConfig, worldsDir string) bool {
	set := SiegeSettings(c)
	if !c.Enabled {
		gw.SetArena(entity.GameSiege, false, set, entity.CTFLayout{Game: entity.ArenaSiege})
		return false
	}
	b, ok := ServerWorld(set.World)
	if !ok {
		log.Printf("WARNING: world.siege: world %d is none of the server's own built worlds (world.server_worlds, cmd/worldbuild); the siege is closed", set.World)
		gw.SetArena(entity.GameSiege, false, set, entity.CTFLayout{Game: entity.ArenaSiege})
		return false
	}
	_ = worldsDir
	lay, err := ReadSiege(b)
	if err != nil {
		log.Printf("WARNING: world.siege: %v; the siege is closed", err)
		gw.SetArena(entity.GameSiege, false, set, entity.CTFLayout{Game: entity.ArenaSiege})
		return false
	}
	gw.SetArena(entity.GameSiege, true, set, lay)
	siegeRulesLines = siegeRules(set, lay)
	gates := make([]string, 0, len(lay.Siege.Gates))
	for _, g := range lay.Siege.Gates {
		gates = append(gates, g.Name)
	}
	log.Printf("Siege: %s in world %d: %s against %s, teams of up to %d (start at %d a side), %v; %s (%d HP, a copy of %q) behind %v (%d HP each)",
		lay.Name, set.World, lay.Teams[0].Short, lay.Teams[1].Short, set.TeamSize, set.MinPerTeam, set.TimeLimit,
		lay.Siege.King.Name, set.KingHP, lay.Siege.King.Original, gates, set.GateHP)
	return true
}

// siegeRulesLines are the siege's rules as told, made at the start (SetupSiege).
var siegeRulesLines []string

// siegeRules are the siege's rules as the herald and "/siege rules" tell them, the numbers the server's (set).
func siegeRules(set entity.CTFSettings, lay entity.CTFLayout) []string {
	def, att := lay.Teams[entity.SiegeDefenders], lay.Teams[entity.SiegeAttackers]
	king := lay.Siege.King.Name
	gates := make([]string, 0, len(lay.Siege.Gates))
	for _, g := range lay.Siege.Gates {
		gates = append(gates, "the "+g.Name)
	}
	mins := int(set.TimeLimit.Round(time.Minute) / time.Minute)
	return []string{
		"The siege, the rules:",
		fmt.Sprintf("%s hold %s; %s come at it from outside its gates. The teams are split as even as they come.", upFirst(def.Name), lay.Name, att.Name),
		fmt.Sprintf("%s stands in the courtyard with %d hit points. Only %s may strike him; he fights back at them. His defenders may heal him.", king, set.KingHP, att.Name),
		fmt.Sprintf("The way in is shut by %s, %d hit points each. A gate never fights back, and only %s may strike it; once broken, the way stays open.", strings.Join(gates, " and "), set.GateHP, att.Name),
		fmt.Sprintf("%s win when %s falls; %s win if he still stands after %d minutes.", upFirst(att.Name), king, def.Name, mins),
		fmt.Sprintf("The fallen rise together at their camp every %d seconds. A death in the siege costs nothing.", int(set.WaveEvery.Round(time.Second)/time.Second)),
		fmt.Sprintf("Teams of up to %d; a siege begins when %d a side have signed up. /siege shows it, /siege leave takes you out.", set.TeamSize, set.MinPerTeam),
	}
}

// siegeCommandHelp is /siege's help.
const siegeCommandHelp = "/siege: how the siege stands. /siege join signs you up (or talk to the PvP Events herald beside Dabrish), /siege leave takes you off the list or out of a siege, /siege rules says how it is fought."

// siegeCommand is /siege [join|leave|rules]: the siege's state, a sign-up, a leave, the rules.
func siegeCommand(gw *entity.World, sess *session.Session, rest string) {
	if gw == nil {
		return
	}
	now := time.Now()
	switch strings.ToLower(strings.TrimSpace(rest)) {
	case "":
		ctfTell(sess, gw.ArenaStatus(entity.GameSiege, sess, now)...)
	case "join":
		if on, world := gw.ArenaOn(entity.GameSiege); on && !ctfReaches(sess, world) {
			ctfTell(sess, ctfHeraldNoArena)
			return
		}
		ctfTell(sess, gw.ArenaJoin(entity.GameSiege, sess, now)...)
	case "leave":
		ctfTell(sess, gw.ArenaLeave(entity.GameSiege, sess, now))
	case "rules":
		ctfTell(sess, siegeRulesLines...)
	default:
		ctfTell(sess, siegeCommandHelp)
	}
}

// arenaGMHelp is a GM's #ctf/#siege help.
const arenaGMHelp = "%s start (now, with whoever is signed up: one a side at least), %s stop (ends it, no victor), %s time <minutes> (this match's and the next), %s teams <n> (the most a team holds)"

// arenaGMCommand is a GM's #ctf or #siege start|stop|time <minutes>|teams <n> (word the command's), which runs the
// game by hand; false for anything else (the players' own words: join, leave, rules, or nothing).
func arenaGMCommand(gw *entity.World, g entity.ArenaGame, word string, sess *session.Session, rest string) bool {
	if gw == nil {
		return false
	}
	fields := strings.Fields(strings.ToLower(rest))
	if len(fields) == 0 {
		return false
	}
	now := time.Now()
	num := func() (int, bool) {
		if len(fields) < 2 {
			return 0, false
		}
		n, err := strconv.Atoi(fields[1])
		return n, err == nil && n > 0
	}
	var line string
	switch fields[0] {
	case "start":
		line = gw.ArenaStartNow(g, now)
	case "stop":
		line = gw.ArenaStop(g, now)
	case "time":
		n, ok := num()
		if !ok {
			line = fmt.Sprintf("%s time <minutes>: a number of minutes, 1 or more", word)
			break
		}
		line = gw.ArenaSetTime(g, time.Duration(n)*time.Minute, now)
	case "teams":
		n, ok := num()
		if !ok {
			line = fmt.Sprintf("%s teams <n>: the most a team holds, 1 or more", word)
			break
		}
		line = gw.ArenaSetTeamSize(g, n)
	case "help":
		line = fmt.Sprintf(arenaGMHelp, word, word, word, word)
	default:
		return false
	}
	if p := gw.SessionPlayer(sess); p != nil {
		log.Printf("GM: %s %s %s: %s", p.Name, word, rest, line)
	}
	SendColoredChat(sess, "[GM] "+line, 255, 215, 90)
	return true
}
