package handlers

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// hotzones.go — the Hot zones (work board 1927 section 6): GMs mark a camp or a quest set, and
// Heirloom Marks earned there get its bonus (default +10%). Changing one needs no build: it is table
// hotzones (the GM panel's "Hot zones"), or a GM's #hotzone (a /hotzone from vanilla), each change a
// gm_audit row change as #spellfx writes one, read at once here (forgetZones) and at most 30 s old
// anywhere else.
//
//   - A camp zone fits a kill when the creature's spawn is in its world within its radius of (x, z);
//     a quest zone a quest whose import key starts with one of its prefixes, or whose id it lists.
//   - #hotzone lists every zone; #hotzone camp <camp key> makes a camp zone on that #tele camp's
//     anchor (camps.go's keys), radius 150, named for the camp; #hotzone here <radius> <name> one at the
//     GM's place; #hotzone quests <key prefix or ids> <name> a quest zone, its place for the teleport
//     the GM's; #hotzone move <id>, on|off <id>, bonus <id> <pct>, note <id> <text>, delete <id>.
//   - #hotzone level60 <id> on|off opens a camp zone to level 60s (column level60): a 60 not reliving
//     earns kill marks for its creatures, which it earns nowhere else; off by default, so a camp is
//     below 60s' alone until a GM says otherwise (jibrish, 2026-11-23). It lasts while the zone is on.
//   - A zone with announce 1 turned on tells every player on the shard one line.
//   - The Hot Zone Herald (any NPC of the name, in Freeport by Dabrish) lists the zones on, each its
//     name, bonus and note, then "Report anything wrong you find with /bug.", and asks which to go
//     to: a camp zone made on a camp lands beside its anchor (World.CampLanding), any other at its
//     place; a world the client cannot load is refused. Anyone may be sent; only earners get the
//     bonus.

// HotZoneHeraldName is the herald's name.
const HotZoneHeraldName = "Hot Zone Herald"

// hotZoneBoxMark marks the herald's questions.
const hotZoneBoxMark = 0x75000000

// The zones' defaults and words (ours, but the announcement's and the herald's last line, the spec's).
const (
	hotZoneRadius     = 150
	hotZoneBonus      = 10
	hotZoneAnnounce   = "A Hot zone is up: %s, +%d%% Heirloom Marks. The Hot Zone Herald in Freeport can send you there."
	heraldReport      = "Report anything wrong you find with /bug."
	heraldAsk         = "Where shall I send you?"
	heraldNone        = "No Hot zone is up just now. Come back when the GMs mark one."
	heraldTooFar      = "Come closer, and I will send you."
	heraldLost        = "I cannot find the way there just now."
	heraldClosed      = "I send no one anywhere on this server."
	hotZoneUsage      = "#hotzone [camp <key> | here <radius> <name> | quests <prefix or ids> <name> | move <id> | on <id> | off <id> | bonus <id> <pct> | level60 <id> on|off | note <id> <text> | delete <id>]"
	hotZone60Camp     = "Only a camp zone opens to level 60s."
	hotZoneDetail     = "#hotzone in the game (work board 1927)"
	hotZoneNoteChars  = 200
	hotZoneNameChars  = 48
	heraldAnswersMost = maxDialogOptions
)

// IsHotZoneHerald is whether an NPC is the herald.
func IsHotZoneHerald(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), HotZoneHeraldName)
}

// --- the herald ---

// heraldTalk is a talk to npc: the herald's list and question when npc is the herald, else false.
func (m *HeirloomMarks) heraldTalk(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	if npc == nil || npc.Dead || !IsHotZoneHerald(npc.Name) {
		return false
	}
	if m == nil || !HeirloomMarksOn() {
		marksSay(sess, HotZoneHeraldName, heraldClosed)
		return true
	}
	on := m.enabledZones()
	if len(on) == 0 {
		marksSay(sess, HotZoneHeraldName, heraldNone)
		return true
	}
	sortZones(on)
	if len(on) > heraldAnswersMost {
		on = on[:heraldAnswersMost]
	}
	answers := make([]string, len(on))
	for i, z := range on {
		line := fmt.Sprintf("%s: +%d%% Heirloom Marks.", z.Name, z.BonusPct)
		if open60Part(z) != "" {
			line += " Level 60s earn here too."
		}
		if strings.TrimSpace(z.Note) != "" {
			line += " " + strings.TrimSpace(z.Note)
		}
		marksSay(sess, HotZoneHeraldName, line)
		answers[i] = z.Name
	}
	marksSay(sess, HotZoneHeraldName, heraldReport)
	m.ask(sess, p, hotZoneBoxMark, marksBox{npc: npc.ServerID, zones: on}, heraldAsk, answers)
	return true
}

// answerHerald is HandleDialogueOption's question for the herald's box: the trip. It reports
// whether the box was his.
func (m *HeirloomMarks) answerHerald(sess *session.Session, p *entity.Player, box uint32, option byte) bool {
	if box&0xFF000000 != hotZoneBoxMark {
		return false
	}
	if m == nil || option == 255 {
		return true
	}
	b, ok := m.answered(p, box)
	if !ok || int(option) >= len(b.zones) {
		return true
	}
	if !HeirloomMarksOn() {
		marksSay(sess, HotZoneHeraldName, heraldClosed)
		return true
	}
	if w, x, _, z, here := m.gw.NPCSpot(b.npc); !here || w != p.World || (x-p.X)*(x-p.X)+(z-p.Z)*(z-p.Z) > teleReach*teleReach {
		marksSay(sess, HotZoneHeraldName, heraldTooFar)
		return true
	}
	zone := b.zones[option]
	dest, ok := m.zoneLanding(zone)
	if !ok {
		marksSay(sess, HotZoneHeraldName, heraldLost)
		return true
	}
	if _, err := ValidateTeleport(dest.X, dest.Y, dest.Z, dest.World); err != nil || !CanLoadWorld(sess, dest.World) {
		log.Printf("Marks: %s's trip to Hot zone %d refused: %v (world %d)", p.Name, zone.ID, err, dest.World)
		marksSay(sess, HotZoneHeraldName, heraldLost)
		return true
	}
	if m.travel == nil {
		return true
	}
	log.Printf("Marks: the Hot Zone Herald sends %s to Hot zone %d %q", p.Name, zone.ID, zone.Name)
	m.travel(sess, p, dest)
	return true
}

// zoneLanding is where a trip to a zone lands: beside its camp's anchor (World.CampLanding) for a
// zone made on a camp, else its place.
func (m *HeirloomMarks) zoneLanding(z database.HotZone) (entity.CoachDest, bool) {
	dest := entity.CoachDest{Name: z.Name, World: uint8(z.World), X: z.X, Y: z.Y, Z: z.Z, Via: HotZoneHeraldName}
	if d, ok := campDests[z.Camp]; ok && z.Camp != "" && m.gw != nil {
		if x, y, zz, ok := m.gw.CampLanding(d.NPC, d.World, d.X, d.Z, campAnchorReach); ok {
			dest.X, dest.Y, dest.Z, dest.World = x, y, zz, d.World
			dest.Facing = heading(x, zz, d.X, d.Z)
			return dest, true
		}
	}
	if z.X == 0 && z.Z == 0 {
		return dest, false
	}
	return dest, true
}

// --- #hotzone ---

// hotZoneCommand is a GM's #hotzone (the file's doc); raw is what followed the word, as typed.
func (m *HeirloomMarks) hotZoneCommand(sess *session.Session, p *entity.Player, raw string) {
	reply := func(s string) { SendClientMessage(sess, s) }
	if m == nil || m.db == nil {
		reply(marksOffLine)
		return
	}
	f := strings.Fields(raw)
	if len(f) == 0 {
		zones, err := m.db.HotZones()
		if err != nil {
			reply("Hot zones: " + err.Error())
			return
		}
		if len(zones) == 0 {
			reply("No Hot zones. " + hotZoneUsage)
			return
		}
		for _, z := range zones {
			reply(hotZoneLine(z))
		}
		return
	}
	who := gmEditOf(sess)
	sub := strings.ToLower(f[0])
	idArg := func() (database.HotZone, bool) {
		if len(f) < 2 {
			reply(hotZoneUsage)
			return database.HotZone{}, false
		}
		id, err := strconv.Atoi(f[1])
		if err != nil {
			reply(hotZoneUsage)
			return database.HotZone{}, false
		}
		zones, err := m.db.HotZones()
		if err != nil {
			reply(err.Error())
			return database.HotZone{}, false
		}
		for _, z := range zones {
			if z.ID == id {
				return z, true
			}
		}
		reply(fmt.Sprintf("No Hot zone %d.", id))
		return database.HotZone{}, false
	}
	save := func(z database.HotZone, said string) {
		id, err := m.db.SaveHotZone(z, who, hotZoneDetail)
		if err != nil {
			reply("Not saved: " + err.Error())
			return
		}
		z.ID = id
		m.forgetZones()
		log.Printf("Marks: %s's #hotzone %s: zone %d %q", p.Name, sub, id, z.Name)
		reply(said + " " + hotZoneLine(z))
		m.hotZones() // read now: a zone of announce 1 turned on is announced (marks.go)
	}
	here := func(z *database.HotZone) {
		z.World, z.X, z.Y, z.Z = int(p.World), p.X, p.Y, p.Z
	}
	switch sub {
	case "camp":
		if len(f) < 2 {
			reply(hotZoneUsage)
			return
		}
		key := strings.ToLower(f[1])
		d, ok := campDests[key]
		if !ok {
			reply(fmt.Sprintf("No camp %q: #tele camps lists them.", key))
			return
		}
		z := database.HotZone{Name: d.Label, Enabled: true, Kind: database.HotZoneCamp, World: int(d.World), X: d.X, Y: d.Y, Z: d.Z,
			Radius: hotZoneRadius, BonusPct: hotZoneBonus, CreatedBy: p.Name, Camp: key}
		if x, y, zz, _, w, ok := m.gw.SpawnOfNPCNear(d.NPC, d.World, d.X, d.Z, campAnchorReach); ok {
			z.X, z.Y, z.Z, z.World = x, y, zz, int(w)
		}
		save(z, "Made:")
	case "here":
		if len(f) < 3 {
			reply(hotZoneUsage)
			return
		}
		r, err := strconv.ParseFloat(f[1], 32)
		if err != nil || r <= 0 || r > 5000 {
			reply("A radius of 1 to 5000.")
			return
		}
		z := database.HotZone{Name: zoneName(strings.Join(f[2:], " ")), Enabled: true, Kind: database.HotZoneCamp, Radius: float32(r),
			BonusPct: hotZoneBonus, CreatedBy: p.Name}
		here(&z)
		save(z, "Made:")
	case "quests":
		if len(f) < 3 {
			reply(hotZoneUsage)
			return
		}
		z := database.HotZone{Name: zoneName(strings.Join(f[2:], " ")), Enabled: true, Kind: database.HotZoneQuests, Quests: f[1],
			Radius: hotZoneRadius, BonusPct: hotZoneBonus, CreatedBy: p.Name}
		here(&z)
		save(z, "Made:")
	case "move":
		if z, ok := idArg(); ok {
			here(&z)
			z.Camp = "" // its place is the GM's now
			save(z, "Moved:")
		}
	case "on", "off":
		if z, ok := idArg(); ok {
			z.Enabled = sub == "on"
			save(z, "Turned "+sub+":")
		}
	case "bonus":
		if z, ok := idArg(); ok {
			if len(f) < 3 {
				reply(hotZoneUsage)
				return
			}
			pct, err := strconv.Atoi(strings.TrimSuffix(f[2], "%"))
			if err != nil || pct < 0 || pct > 500 {
				reply("A bonus of 0 to 500 (%).")
				return
			}
			z.BonusPct = pct
			save(z, "Bonus set:")
		}
	case "level60":
		if z, ok := idArg(); ok {
			word := ""
			if len(f) >= 3 {
				word = strings.ToLower(f[2])
			}
			if word != "on" && word != "off" {
				reply(hotZoneUsage)
				return
			}
			if word == "on" && z.Kind != database.HotZoneCamp {
				reply(hotZone60Camp)
				return
			}
			z.Level60 = word == "on"
			said := "Level 60s earn nothing here:"
			if z.Level60 {
				said = "Level 60s earn here:"
			}
			save(z, said)
		}
	case "note":
		if z, ok := idArg(); ok {
			// the text as typed, after the id
			_, rest, _ := strings.Cut(strings.TrimSpace(raw), " ")
			_, text, _ := strings.Cut(strings.TrimSpace(rest), " ")
			z.Note = cleanZoneText(text, hotZoneNoteChars)
			save(z, "Note set:")
		}
	case "delete":
		if z, ok := idArg(); ok {
			if err := m.db.DeleteHotZone(z.ID, who, hotZoneDetail); err != nil {
				reply("Not deleted: " + err.Error())
				return
			}
			m.forgetZones()
			m.hotZones()
			log.Printf("Marks: %s's #hotzone delete: zone %d %q", p.Name, z.ID, z.Name)
			reply(fmt.Sprintf("Deleted Hot zone %d, %s.", z.ID, z.Name))
		}
	default:
		reply(hotZoneUsage)
	}
}

// hotZoneLine is a zone in #hotzone's list: id, on/off, name, bonus, place.
func hotZoneLine(z database.HotZone) string {
	state := "off"
	if z.Enabled {
		state = "on"
	}
	place := fmt.Sprintf("world %d (%.0f, %.0f, %.0f) r %.0f", z.World, z.X, z.Y, z.Z, z.Radius)
	if z.Kind == database.HotZoneQuests {
		place = "quests " + z.Quests
	}
	return fmt.Sprintf("%d %s: %s, +%d%%%s, %s", z.ID, state, z.Name, z.BonusPct, open60Part(z), place)
}

// zoneName is a zone's name as kept.
func zoneName(s string) string { return cleanZoneText(s, hotZoneNameChars) }

// cleanZoneText is a GM's words as players are shown them: no '|' (a run kills a PS2's TextBox), no
// control character, trimmed and cut.
func cleanZoneText(s string, most int) string {
	s = strings.Map(func(r rune) rune {
		if r == '|' || r < 0x20 {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > most {
		s = string(r[:most])
	}
	return s
}

// announce tells every player on the shard a zone is up.
func (m *HeirloomMarks) announce(z database.HotZone) {
	if m.gw == nil || m.gw.SessionManager == nil {
		return
	}
	line := fmt.Sprintf(hotZoneAnnounce, z.Name, z.BonusPct)
	n := 0
	m.gw.SessionManager.ForEach(func(s *session.Session) bool {
		if s.CharacterInWorld() {
			SendClientMessage(s, line)
			n++
		}
		return true
	})
	log.Printf("Marks: Hot zone %d %q announced to %d players", z.ID, z.Name, n)
}
