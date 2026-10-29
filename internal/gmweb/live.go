package gmweb

import (
	"fmt"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/handlers"
	"github.com/eqoa/server/internal/session"
)

// What a committed row change does to the running game, where the game holds
// the data in memory: an NPC spawns, changes or leaves; NPCs of a name take a
// new class; an online character's level, purse and place change where it
// stands; a banned account's sessions end. The answer says what happened, or
// when the change will be seen, in words for the page and for an agent.

func (s *Server) applyLive(a *actor, t *Table, action string, before, after map[string]any) string {
	switch t.Name {
	case "npcs":
		return s.liveNPC(action, before, after)
	case "npcclass":
		return s.liveNPCClass(before, after)
	case "npcpaths":
		return s.livePath(before, after)
	case "defaultcharacter":
		database.InvalidateDefaultCharacters()
		return "the next character made uses it"
	case "accountinfo":
		return s.liveAccount(a, action, before, after)
	case "characters":
		return s.liveCharacter(a, action, before, after)
	case "factions", "factionhits", "factionstart", "npcfaction":
		return s.liveFactions(t.Name)
	case "npcaggro":
		return s.liveAggro()
	case "npcundead":
		return s.liveUndead()
	case "npcsocial":
		return s.liveSocial()
	case "npcmove":
		return s.liveMove()
	case "spellgates":
		return s.liveSpellGates() // live_gates.go
	case "gmtriggers":
		return s.liveTriggers() // ops_triggers.go
	case "charfactions":
		return s.liveCharFaction(before, after)
	case "cmcatalog":
		return s.liveCMCatalog()
	case "charcms":
		return s.liveCharCMs(before, after)
	case "quests", "queststeps", "questunlocks":
		return s.liveQuests()
	case "spellpattern", "spelleffect", "classspells":
		// the community spell page's list too (internal/spellfx)
		database.InvalidateSpellFX()
		return s.livePetTaps(t.Live)
	case "charquests":
		return s.liveCharQuest(before, after)
	case "mapmarks":
		return s.liveMapMarks()
	case "charinventory", "spells", "hotkeys", "charbuffs", "weaponhotbar", "quickslots", "gearsets":
		row := after
		if row == nil {
			row = before
		}
		col := "serverid"
		if id, ok := asFloat(row[col]); ok {
			if p, online := s.onlineCharacter(uint32(id)); online {
				return fmt.Sprintf("%s is in the world: they get it at their next login (kick_player to apply it now)", p.Name)
			}
		}
		return "at the character's next login"
	}
	return t.Live
}

// liveFactions reads the factions and the hits again (the change is
// committed: the database is free).
func (s *Server) liveFactions(table string) string {
	if s.d.World == nil || s.d.DB == nil {
		return "no world is running: at the next start"
	}
	if err := s.d.World.LoadFactions(s.d.DB); err != nil {
		return "not applied to the world: " + err.Error()
	}
	switch table {
	case "factions":
		return "at once: every faction window, target box and the next kill read it"
	case "factionstart":
		return "at once: every faction window, target box and look round (half a second) after it use it, for every faction a character has not moved"
	case "npcfaction":
		return "at once: " + s.d.World.FactionMembersSummary() + "; the next look round (half a second) and the next target box use it"
	}
	return "at once: the next kill of that NPC uses it"
}

// livePetTaps reads the necromancer's pets' life taps again (entity/
// pettap.go; the change is committed: the database is free), after a spell's
// row, line or class changed; the table's own words say the rest.
func (s *Server) livePetTaps(live string) string {
	if s.d.World == nil || s.d.DB == nil {
		return live
	}
	if _, err := s.d.World.LoadPetTaps(s.d.DB); err != nil {
		return live + " (the pets' life taps not read again: " + err.Error() + ")"
	}
	return live + " A necromancer's pet's life tap follows it at once."
}

// liveAggro reads the creatures' aggro rules again (entity/aggro.go; the
// change is committed: the database is free).
func (s *Server) liveAggro() string {
	if s.d.World == nil || s.d.DB == nil {
		return "no world is running: at the next start"
	}
	if _, err := s.d.World.LoadAggroRules(s.d.DB); err != nil {
		return "not applied to the world: " + err.Error()
	}
	return "at once: the next look round (half a second) and the next target box use it"
}

// liveUndead reads the undead tags again (entity/undead.go; the change is
// committed: the database is free).
func (s *Server) liveUndead() string {
	if s.d.World == nil || s.d.DB == nil {
		return "no world is running: at the next start"
	}
	if _, _, err := s.d.World.LoadUndeadTags(s.d.DB); err != nil {
		return "not applied to the world: " + err.Error()
	}
	return "at once: the next cast at a creature of that name uses it"
}

// liveSocial reads which NPCs come to the help of their own again
// (entity/socialaggro.go; the change is committed: the database is free).
func (s *Server) liveSocial() string {
	if s.d.World == nil || s.d.DB == nil {
		return "no world is running: at the next start"
	}
	if _, err := s.d.World.LoadSocialRules(s.d.DB); err != nil {
		return "not applied to the world: " + err.Error()
	}
	return "at once: the next hit uses it"
}

// liveMove reads how the creatures of a name move again (entity/movement.go;
// the change is committed: the database is free).
func (s *Server) liveMove() string {
	if s.d.World == nil || s.d.DB == nil {
		return "no world is running: at the next start"
	}
	if _, err := s.d.World.LoadMoveRules(s.d.DB); err != nil {
		return "not applied to the world: " + err.Error()
	}
	return "at once: the next step of every creature of that name uses it"
}

// liveCMCatalog reads the class masteries again (entity/cmbuy.go; the change
// is committed: the database is free).
func (s *Server) liveCMCatalog() string {
	if s.d.World == nil || s.d.DB == nil {
		return "no world is running: at the next start"
	}
	n, err := s.d.World.LoadCMCatalog(s.d.DB)
	if err != nil {
		return "not applied to the world: " + err.Error()
	}
	return fmt.Sprintf("at once: %d ranks; every list opened from now on, and the stats of the characters online", n)
}

// liveCharCMs puts a character's masteries, as charcms holds them now, on it
// where it stands.
func (s *Server) liveCharCMs(before, after map[string]any) string {
	row := after
	if row == nil {
		row = before
	}
	id, ok := asFloat(row["serverid"])
	if !ok || s.d.World == nil || s.d.DB == nil {
		return "at the character's next login"
	}
	owned, err := database.NewSQLiteCharacterStore(s.d.DB).LoadOwnedCMs(uint32(id))
	if err != nil {
		return "not applied to the world: " + err.Error()
	}
	if name, online := s.d.World.ReloadCMs(uint32(id), owned); online {
		return fmt.Sprintf("at once: %s holds %d masteries now (its points are not changed: #cmreset in the game gives them back)", name, len(owned))
	}
	return "at the character's next login"
}

// liveCharFaction applies a charfactions edit where the character stands, so
// its logout does not save the old standing over it.
func (s *Server) liveCharFaction(before, after map[string]any) string {
	if s.d.World == nil {
		return "at the character's next login"
	}
	text := func(v any) string { str, _ := v.(string); return str }
	var name string
	var online bool
	if before != nil && (after == nil || text(before["faction"]) != text(after["faction"])) {
		if id, ok := asFloat(before["serverid"]); ok {
			name, online = s.d.World.SetOnlineFaction(uint32(id), text(before["faction"]), 0, true)
		}
	}
	if after != nil {
		id, ok1 := asFloat(after["serverid"])
		v, ok2 := asFloat(after["standing"])
		if ok1 && ok2 {
			name, online = s.d.World.SetOnlineFaction(uint32(id), text(after["faction"]), int(v), false)
		}
	}
	if online {
		return fmt.Sprintf("applied where %s stands: their faction window shows it the next time it opens", name)
	}
	return "at the character's next login"
}

func (s *Server) liveNPC(action string, before, after map[string]any) string {
	if s.d.World == nil || s.d.NPCs == nil {
		return "no world is running: at the next start"
	}
	var msgs []string
	if before != nil {
		bid, _ := asFloat(before["id"])
		aid := bid
		if after != nil {
			aid, _ = asFloat(after["id"])
		}
		if after == nil || aid != bid {
			if name, ok := s.d.World.RemoveNPC(entity.EntityID(bid)); ok {
				msgs = append(msgs, fmt.Sprintf("%s left the world", name))
			}
		}
	}
	if after != nil {
		id, _ := asFloat(after["id"])
		rec, err := s.d.NPCs.GetNPC(int(id))
		if err != nil {
			return "not applied to the world: " + err.Error()
		}
		class, _ := s.d.NPCs.NPCClassOf(rec.NPCName)
		created, err := s.d.World.UpsertNPC(rec, class)
		switch {
		case err != nil:
			msgs = append(msgs, "not applied to the world: "+err.Error())
		case created:
			msgs = append(msgs, fmt.Sprintf("%s spawned in %s at (%.0f, %.0f, %.0f)", rec.NPCName,
				worldName(rec.World), rec.XCoord, rec.YCoord, rec.ZCoord))
		default:
			msgs = append(msgs, fmt.Sprintf("%s changed in the world: it leaves every view for a second and comes back as edited, on its spawn point", rec.NPCName))
		}
	}
	return strings.Join(msgs, "; ")
}

func (s *Server) liveNPCClass(before, after map[string]any) string {
	if s.d.World == nil {
		return "no world is running: at the next start"
	}
	var msgs []string
	if before != nil && (after == nil || fmt.Sprint(before["name"]) != fmt.Sprint(after["name"])) {
		n := s.d.World.SetNPCClass(fmt.Sprint(before["name"]), 0)
		msgs = append(msgs, fmt.Sprintf("%d NPCs named %q are warriors again", n, before["name"]))
	}
	if after != nil {
		class, _ := asFloat(after["class"])
		n := s.d.World.SetNPCClass(fmt.Sprint(after["name"]), int(class))
		msgs = append(msgs, fmt.Sprintf("%d NPCs named %q swing as a %s now", n, after["name"], className(int(class))))
	}
	return strings.Join(msgs, "; ")
}

func (s *Server) liveAccount(a *actor, action string, before, after map[string]any) string {
	row := after
	if row == nil {
		row = before
	}
	id, _ := asFloat(row["accountid"])
	banned := func(m map[string]any) bool {
		if m == nil {
			return false
		}
		st, ok := asFloat(m["acctstatus"])
		return ok && st < 0
	}
	if action == "delete" || (banned(after) && !banned(before)) {
		n := handlers.EjectAccount(s.d.Sessions, int(id))
		s.sessions.endAccount(int(id), "")
		s.revokeAccountTokens(int(id))
		if action == "delete" {
			return fmt.Sprintf("account deleted; %d game session(s) ejected", n)
		}
		return fmt.Sprintf("banned: %d game session(s) ejected; the next login is refused", n)
	}
	if banned(before) && !banned(after) {
		return "unbanned: the account may log in again"
	}
	level := func(m map[string]any) int {
		v, _ := asFloat(m["acctlevel"])
		return int(v)
	}
	if action == "update" && level(before) >= s.cfg.MinLevel && level(after) < s.cfg.MinLevel {
		// however it came (set_account_level, a revert): no longer a GM, so
		// nothing of the panel's works for it any more
		s.sessions.endAccount(int(id), "")
		n := s.revokeAccountTokens(int(id))
		return fmt.Sprintf("no longer a GM: its panel sessions ended and %d token(s) were revoked", n)
	}
	return ""
}

func (s *Server) liveCharacter(a *actor, action string, before, after map[string]any) string {
	row := after
	if row == nil {
		row = before
	}
	id, _ := asFloat(row["serverid"])
	p, online := s.onlineCharacter(uint32(id))
	if !online {
		if action == "delete" {
			return ""
		}
		return "the character is offline: everything at its next login"
	}
	sess := handlers.SessionOfCharacter(s.d.World, s.d.Sessions, uint32(id))
	if action == "delete" {
		if sess != nil {
			handlers.Eject(sess)
		}
		return fmt.Sprintf("%s was in the world and was ejected", p.Name)
	}
	if action != "update" {
		return ""
	}
	var done, later []string
	changed := func(col string) bool { return !sameValue(before[col], after[col]) }
	if changed("level") {
		lvl, _ := asFloat(after["level"])
		if s.d.World.SetPlayerLevel(uint32(id), int(lvl)) && sess != nil {
			handlers.SendLevel(sess, int(lvl))
		}
		done = append(done, fmt.Sprintf("level %d", int(lvl)))
	}
	if changed("tunar") || changed("bankTunar") {
		t, _ := asFloat(after["tunar"])
		b, _ := asFloat(after["bankTunar"])
		if s.d.World.SetPlayerTunar(uint32(id), int(t), int(b)) && sess != nil {
			handlers.SendPlayerTunar(sess, int64(t))
		}
		done = append(done, fmt.Sprintf("%d tunar, %d in the bank", int(t), int(b)))
	}
	if changed("x") || changed("y") || changed("z") || changed("world") {
		x, _ := asFloat(after["x"])
		y, _ := asFloat(after["y"])
		z, _ := asFloat(after["z"])
		w, _ := asFloat(after["world"])
		if msg, err := s.teleportLive(p, float32(x), float32(y), float32(z), byte(w)); err != nil {
			later = append(later, "not moved: "+err.Error()+" (they will be saved where they stand at logout)")
		} else {
			done = append(done, msg)
		}
	}
	for _, c := range []string{"charName", "tclass", "race", "sex", "modelid", "haircolor", "hairlength", "hairstyle",
		"faceoption", "tpstrength", "tpstamina", "tpagility", "tpdexterity", "tpwisdom", "tpintel", "tpcharisma",
		"totalXP", "unusedTP", "totalTP", "humType", "accountid"} {
		if changed(c) {
			later = append(later, c)
		}
	}
	msg := p.Name + " is in the world"
	if len(done) > 0 {
		msg += ": applied now: " + strings.Join(done, ", ")
	}
	if len(later) > 0 {
		msg += "; at their next login: " + strings.Join(later, ", ")
	}
	return msg
}

// onlineCharacter is the world's player for a character, if it is in the
// world.
func (s *Server) onlineCharacter(serverID uint32) (entity.PlayerState, bool) {
	if s.d.World == nil {
		return entity.PlayerState{}, false
	}
	return s.d.World.PlayerByCharacter(serverID)
}

// teleportLive moves a character who is in the world, as the account pages'
// teleport does (a harbormaster's warp), once the place passes the game's
// checks.
func (s *Server) teleportLive(p entity.PlayerState, x, y, z float32, world byte) (string, error) {
	w, err := handlers.ValidateTeleport(x, y, z, world)
	if err != nil {
		return "", err
	}
	if !handlers.TeleportOnlineCharacter(s.d.World, s.d.Sessions, p.ID, x, y, z, w) {
		return "", fmt.Errorf("%s has no session to send it to", p.Name)
	}
	return fmt.Sprintf("teleported to (%.0f, %.0f, %.0f) in %s", x, y, z, worldName(int(w))), nil
}

// sessionFor is a character's in-world session, or nil.
func (s *Server) sessionFor(serverID uint32) *session.Session {
	return handlers.SessionOfCharacter(s.d.World, s.d.Sessions, serverID)
}

func worldName(w int) string {
	names := handlers.WorldNames()
	if w >= 0 && w < len(names) {
		return names[w]
	}
	return fmt.Sprintf("world %d", w)
}

func className(c int) string {
	for _, e := range enums()["class"] {
		if e.Value == c {
			return e.Label
		}
	}
	return fmt.Sprintf("class %d", c)
}

func raceName(r int) string {
	for _, e := range enums()["race"] {
		if e.Value == r {
			return e.Label
		}
	}
	return fmt.Sprintf("race %d", r)
}
