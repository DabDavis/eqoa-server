package handlers

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// InteractHandler handles NPC interaction (dialogue).
type InteractHandler struct {
	sessionManager *session.Manager
	gameWorld      *entity.World

	// corpse opens a dead NPC's loot (NPCWindowHandler.OpenCorpse); it
	// reports whether the NPC was a corpse.
	corpse func(*session.Session, *entity.NPC) bool

	// binds keeps the bind points spiritmasters give (spiritmaster.go).
	binds bindStore

	// ledger keeps the stables' ledgers the coachmen sign (coachledger.go);
	// isGM is whether a session's account is a GM's (ChatHandler.isGM), who
	// is offered every coach town. Nil: the coach menus as before.
	ledger coachLedger
	isGM   func(*session.Session) bool

	// spend takes a waysender's fee from the player, kept and shown
	// (NPCWindowHandler.spendTunar); false when it cannot pay. Nil: free.
	spend func(*session.Session, *entity.Player, int) bool

	// dialogLines is npcdialog (database.NPCDialog): what a town's people and
	// the guards say (entity/dialog.go). Nil: they say nothing.
	dialogLines func() ([]database.NPCDialogLine, error)

	// lastLine is the npcdialog line each speaker said to each listener last
	// (by lineKey), so a second talk hears another.
	lastMu   sync.Mutex
	lastLine map[uint64]int

	// rehab is the rehabilitators (rehab.go): their menu, and its answers.
	// Nil: a rehabilitator is any other NPC.
	rehab *Rehab
}

// SetDialogLines gives the handler npcdialog's reader (database.NPCDialog).
func (h *InteractHandler) SetDialogLines(f func() ([]database.NPCDialogLine, error)) {
	h.dialogLines = f
}

func lineKey(p *entity.Player, npc *entity.NPC) uint64 {
	return uint64(p.ServerID)<<32 | uint64(uint32(npc.ServerID))
}

// townSay is a town's person's or a guard's line to the player, if it says
// one (entity.World.TownLine): its name and words in a box of their own, as
// a quest NPC's are (QuestEngine.tellLocked).
func (h *InteractHandler) townSay(sess *session.Session, player *entity.Player, npc *entity.NPC) bool {
	if h.dialogLines == nil {
		return false
	}
	lines, err := h.dialogLines()
	if err != nil {
		log.Printf("Interact: npcdialog: %v", err)
		return false
	}
	key := lineKey(player, npc)
	h.lastMu.Lock()
	last := h.lastLine[key]
	h.lastMu.Unlock()
	speaker, line, id, ok := h.gameWorld.TownLine(player, npc, lines, last)
	if !ok {
		return false
	}
	h.lastMu.Lock()
	if h.lastLine == nil || len(h.lastLine) > 20000 {
		h.lastLine = make(map[uint64]int)
	}
	h.lastLine[key] = id
	h.lastMu.Unlock()
	forgetDialogs(player)
	for _, part := range splitSay(speaker+": "+line, questBoxChars) {
		SendDialogueBox(sess, 0, part)
	}
	return true
}

// npcText is what an NPC says when talked to, by name (config
// world.npc_text), in place of the debug box. The GM panel replaces it while
// the server runs, so it is swapped whole, never written in place.
var npcText atomic.Pointer[map[string]string]

// SetNPCText sets what NPCs say when talked to, by name. It keeps a copy.
func SetNPCText(m map[string]string) {
	c := make(map[string]string, len(m))
	for k, v := range m {
		c[k] = v
	}
	npcText.Store(&c)
}

// NPCText is a copy of what NPCs say when talked to, by name.
func NPCText() map[string]string {
	c := map[string]string{}
	if m := npcText.Load(); m != nil {
		for k, v := range *m {
			c[k] = v
		}
	}
	return c
}

// npcTextFor is what the NPC of that name says, if anything.
func npcTextFor(name string) (string, bool) {
	m := npcText.Load()
	if m == nil {
		return "", false
	}
	text, ok := (*m)[name]
	return text, ok
}

// NewInteractHandler creates a new interact handler.
func NewInteractHandler(sessionMgr *session.Manager) *InteractHandler {
	return &InteractHandler{sessionManager: sessionMgr}
}

// SetGameWorld sets the game world reference.
func (h *InteractHandler) SetGameWorld(gw *entity.World) {
	h.gameWorld = gw
}

// RegisterHandlers registers interact handlers with the registry.
func (h *InteractHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeInteract, h.HandleInteract)
	r.Register(protocol.OpcodeDialogueBoxOption, h.HandleDialogueOption)
}

// HandleInteract handles NPC interaction requests (opcode 0x0004).
// Client sends: uint32 targetObjectID
// A corpse opens its loot, the hard-disk technician his menu, a travel NPC
// its routes, a greeter its words, a quest giver the player is too low for
// its city's "not yet" and a town's person or a guard a line of npcdialog;
// anything else answers nothing.
func (h *InteractHandler) HandleInteract(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 4 {
		return nil
	}

	reader := buffer.NewReader(data)
	targetObjectID, _ := reader.ReadUint32()

	log.Printf("Interact from %s: targetObjectID=%d", sess.RemoteAddr(), targetObjectID)

	if h.gameWorld == nil {
		return nil
	}

	// Look up the target entity
	targetEntity, found := h.gameWorld.FindEntityByObjectID(sess, targetObjectID)
	if !found {
		log.Printf("Interact: target objectID %d not found", targetObjectID)
		return nil
	}

	// Distance check
	player := h.gameWorld.SessionPlayer(sess)
	if player == nil {
		return nil
	}
	if !entity.InRange(player, targetEntity, entity.InteractRange) {
		// Name the target and print both positions. "too far (dist=21.3)" alone
		// cannot distinguish the two failures that look identical from the
		// client: a genuinely distant NPC, versus the server resolving the
		// WRONG entity for the objectID the player clicked (object IDs are
		// recycled per session, so a stale one still resolves — to someone
		// else). With the name in the log, one line tells them apart.
		tx, ty, tz := targetEntity.GetPosition()
		log.Printf("Interact: %s too far from %s (dist=%.1f > %.1f) player=(%.1f, %.1f, %.1f) target=(%.1f, %.1f, %.1f) objID=%d",
			player.Name, entity.EntityName(targetEntity),
			entity.DistanceBetween(player, targetEntity), entity.InteractRange,
			player.X, player.Y, player.Z, tx, ty, tz, targetObjectID)
		return nil
	}

	// A corpse: its loot (loot.go).
	if npc, ok := targetEntity.(*entity.NPC); ok && npc.Dead && h.corpse != nil && h.corpse(sess, npc) {
		return nil
	}

	// Travel NPCs. Two kinds, and the difference is the world:
	//
	//   Coachman/Coachwoman — overland travel WITHIN the current world.
	//   Gatekeeper/Harbormaster — CROSS-world travel. These are the only
	//     routes that set CoachDest.World, and the only way to reach another
	//     world short of a GM #goto.
	//
	// Both reuse the same pending-destination list and option-box paging, so a
	// warp NPC needs no new dialogue plumbing.
	// The hard-disk technician. Checked before the travel NPCs because a name
	// belongs to exactly one of them, and because his menu is not a list of
	// destinations — a numbered pick here means something else entirely.
	if npc, ok := targetEntity.(*entity.NPC); ok && npc.Name == HDDSetupNPC {
		player.PendingCoachDests = nil // this box is not a travel box
		// Say so BEFORE opening the menu, rather than letting the player pick a
		// line and be turned down. A menu whose every option is refused is worse
		// than no menu, and the player has something to do about this one.
		if hddConsoleUpdating(sess) {
			player.PendingHDDMenu = HDDMenuNone
			h.npcSay(sess, HDDUpdatingReply)
			log.Printf("HDD: %s opened %s while mid-SD-refresh; told to update again",
				player.Name, HDDSetupNPC)
			return nil
		}
		player.PendingYesNo = false
		player.PendingBind = entity.Bind{}
		player.PendingSpar = entity.SparPick{}
		player.PendingHDDMenu = HDDMenuMain
		h.sendOptionBox(sess, HDDGreeting, HDDMenuFor(HDDMenuMain))
		log.Printf("HDD: %s opened %s", player.Name, HDDSetupNPC)
		return nil
	}

	// A spiritmaster not yet bound to offers its bind first, its quest's answer
	// at the next talk (Sandstorm's Zole, recorded by questrec16).
	if npc, ok := targetEntity.(*entity.NPC); ok && IsSpiritmaster(npc.Name) {
		if b, bound := h.gameWorld.BindOf(player.ServerID); !bound || b.By != npc.Name {
			forgetDialogs(player)
			h.askBind(sess, player, npc)
			return nil
		}
	}

	// A coachman signs his ledger for whoever talks to him (coachledger.go),
	// before his quest's step, which is that signing.
	if npc, ok := targetEntity.(*entity.NPC); ok && entity.IsCoachman(npc.Name) {
		h.signLedger(sess, player, npc)
	}

	// A quest's step at this NPC, or a quest it offers (quests.go), before
	// the NPC's own business.
	if npc, ok := targetEntity.(*entity.NPC); ok && Quests().Talk(sess, player, npc) {
		return nil
	}

	// A rehabilitator's menu (rehab.go): resets, paths, roses.
	if npc, ok := targetEntity.(*entity.NPC); ok && h.rehab.Talk(sess, player, npc) {
		return nil
	}

	// A spiritmaster binds (spiritmaster.go), a waysender sends the player to
	// another town (waysender.go).
	if npc, ok := targetEntity.(*entity.NPC); ok && IsSpiritmaster(npc.Name) {
		forgetDialogs(player)
		h.askBind(sess, player, npc)
		return nil
	}
	if npc, ok := targetEntity.(*entity.NPC); ok && IsWaysender(npc.Name) && h.offerWay(sess, player, npc) {
		return nil
	}
	// a sparring dummy asks its level and element (sparring.go)
	if npc, ok := targetEntity.(*entity.NPC); ok && npc.Dummy == entity.SparringDummy {
		h.askSparLevel(sess, player, npc)
		return nil
	}

	if npc, ok := targetEntity.(*entity.NPC); ok {
		var routes []entity.CoachDest
		switch {
		case entity.IsWarpNPC(npc.Name):
			// only the worlds this client's table has (LoadableWorlds)
			routes = h.gameWorld.GetWarpDestinations(npc.Name, LoadableWorlds(sess))
		case entity.IsCoachman(npc.Name):
			// the towns his coach runs to whose ledgers the player signed
			// (coachledger.go), else why not
			var none string
			if routes, none = h.coachDests(sess, player, npc); len(routes) == 0 && none != "" {
				forgetDialogs(player)
				for _, part := range splitSay(none, questBoxChars) {
					h.sendDialogueBox(sess, part)
				}
				return nil
			}
		}
		if len(routes) > 0 {
			forgetDialogs(player)
			player.PendingCoachDests = routes
			player.CoachPage = 0
			h.sendCoachPage(sess, player)
			return nil
		}
	}

	// A sign post or a greeter: its words (world.npc_text) instead of the
	// debug box.
	if npc, ok := targetEntity.(*entity.NPC); ok {
		if text, ok := npcTextFor(npc.Name); ok {
			forgetDialogs(player)
			// a box at most CLIENT shows, as a quest NPC's words (tellLocked)
			for _, part := range splitSay(text, questBoxChars) {
				h.sendDialogueBox(sess, part)
			}
			return nil
		}
	}

	// A quest giver whose next quest for the player waits on its level alone
	// (quests_notyet.go): its city's "come back when you are stronger", in
	// place of a town's words.
	if npc, ok := targetEntity.(*entity.NPC); ok && Quests().NotYet(sess, player, npc) {
		forgetDialogs(player)
		return nil
	}

	// A town's people and the guards of Norrath (entity/dialog.go): a line of
	// npcdialog, when one fits.
	if npc, ok := targetEntity.(*entity.NPC); ok && h.townSay(sess, player, npc) {
		return nil
	}

	// Anything else has nothing to say: the interact button does nothing, as
	// on a PS2 an NPC with no words, no shop and no route answers nothing
	// (F3 20261025-171528). CLIENT waits on no answer to 0x0004
	// (VIClient::InteractWithTarget). What the box said is a GM's #npcinfo.
	log.Printf("Interact: %s has nothing to say to %s", entity.EntityName(targetEntity), player.Name)
	return nil
}

// EntityInfo is what #npcinfo shows of an NPC or a player: its ids, look,
// level and place, and an NPC's spawn and gear. objectID is the id the
// asking session knows it by.
func EntityInfo(target entity.Entity, objectID uint32) string {
	var info string
	switch t := target.(type) {
	case *entity.NPC:
		info += fmt.Sprintf("[%s] ServerID=%d ObjID=%d\n", t.Name, t.ServerID, objectID)
		info += fmt.Sprintf("NPCID=%d Type=%d\n", t.NPCID, t.NPCType)
		info += fmt.Sprintf("HP=%d/%d\n", t.HP, t.MaxHP)
		info += fmt.Sprintf("Level=%d\n", t.Level)
		info += fmt.Sprintf("Speed=%.1f\n", t.Speed)
		info += fmt.Sprintf("Pos=(%.1f, %.1f, %.1f)\n", t.X, t.Y, t.Z)
		info += fmt.Sprintf("Facing=%.2f\n", t.Facing)
		info += fmt.Sprintf("Model=%d Size=%.2f\n", t.ModelID, t.Size)
		info += fmt.Sprintf("Spawn=(%.1f, %.1f, %.1f) F=%.2f\n", t.SpawnX, t.SpawnY, t.SpawnZ, t.SpawnFacing)
		info += fmt.Sprintf("Wpn=%d/%d Shld=%d\nTorso=%d Arm=%d Glv=%d\nLeg=%d Ft=%d Hd=%d\n",
			t.PrimaryWeapon, t.SecondaryWeapon, t.Shield,
			t.Torso, t.Forearm, t.NPCGloves,
			t.NPCLegs, t.Feet, t.Head)
	case *entity.Player:
		info += fmt.Sprintf("[%s] ServerID=%d ObjID=%d\n", t.Name, t.ServerID, objectID)
		info += fmt.Sprintf("HP=%d/%d Power=%d/%d\n", t.HP, t.MaxHP, t.Power, t.MaxPower)
		info += fmt.Sprintf("Level=%d\n", t.Level)
		info += fmt.Sprintf("STR=%d STA=%d AGI=%d DEX=%d\nWIS=%d INT=%d CHA=%d\n",
			t.Str, t.Sta, t.Agi, t.Dex, t.Wis, t.Int, t.Cha)
		info += fmt.Sprintf("AC=%d Poi=%d Dis=%d Fire=%d\nCold=%d Lit=%d Arc=%d\n",
			t.AC, t.PoisonR, t.DiseaseR, t.FireR, t.ColdR, t.LightningR, t.ArcaneR)
		info += fmt.Sprintf("Speed=%.1f\n", t.Speed)
		info += fmt.Sprintf("Pos=(%.1f, %.1f, %.1f)\n", t.X, t.Y, t.Z)
		info += fmt.Sprintf("Facing=%.2f\n", t.Facing)
		info += fmt.Sprintf("Model=%d Size=%.2f\n", t.ModelID, t.Size)
	default:
		info += fmt.Sprintf("[???] ObjID=%d\n", objectID)
	}
	return info
}

// coachDestsPerPage is the max destinations per page (client option box limit is 6,
// reserve slots for "More..." / "Back" navigation as needed).
const coachDestsPerPage = 5

// sendCoachPage sends the current page of coaching destinations to the player.
func (h *InteractHandler) sendCoachPage(sess *session.Session, player *entity.Player) {
	dests := player.PendingCoachDests
	page := player.CoachPage
	total := len(dests)

	start := page * coachDestsPerPage
	end := start + coachDestsPerPage
	if end > total {
		end = total
	}
	pageDests := dests[start:end]
	hasMore := end < total
	hasBack := page > 0

	options := make([]string, 0, len(pageDests)+2)
	for _, d := range pageDests {
		options = append(options, destLabel(d))
	}
	if hasMore {
		options = append(options, "More...")
	}
	if hasBack {
		options = append(options, "Back")
	}

	greeting := "Greetings, traveler! Where would you like to go?"
	if len(dests) > 0 && dests[0].Via != "" {
		greeting = wayAskMany // a waysender's, as Sandstorm's dockmasters ask
	}
	log.Printf("Coaching: page %d, showing %d dests (%d-%d of %d), options=%v", page, len(pageDests), start, end-1, total, options)
	h.sendOptionBox(sess, greeting, options)
}

// HandleDialogueOption handles the client's dialogue option selection (opcode 0x0035).
// Client sends: uint32 targetObjectID + byte selectedOption (255 = cancel).
func (h *InteractHandler) HandleDialogueOption(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 5 {
		return nil
	}

	reader := buffer.NewReader(data)
	targetObjectID, _ := reader.ReadUint32()
	selectedOption, _ := reader.ReadByte()
	log.Printf("DialogueOption from %s: targetObjectID=%d option=%d", sess.RemoteAddr(), targetObjectID, selectedOption)

	if h.gameWorld == nil {
		return nil
	}

	player := h.gameWorld.SessionPlayer(sess)
	if player == nil {
		return nil
	}

	// A quest's box (quests.go): the u32 is the box's id, which only the
	// quests' boxes set.
	if Quests().Answer(sess, player, targetObjectID, selectedOption) {
		return nil
	}
	// A GM trigger's popup (gmtriggers.go): its one answer changes nothing.
	if IsTriggerBox(targetObjectID) {
		return nil
	}
	// A rehabilitator's question (rehab.go); an old one does nothing.
	if h.rehab.Answer(sess, player, targetObjectID, selectedOption) {
		return nil
	}
	// A duel's invitation or "Cancel duel?" (duel.go)
	if entity.IsDuelBox(targetObjectID) {
		h.gameWorld.DuelBoxAnswer(sess, targetObjectID, selectedOption, time.Now())
		return nil
	}
	// A Call of the Hero's question (callhero.go)
	if entity.IsHeroCallBox(targetObjectID) {
		h.answerHeroCall(sess, targetObjectID, selectedOption)
		return nil
	}

	// The technician's menu, before the travel list is consulted. A pick is
	// just a number; only the menu the player has open says what it means, and
	// reading a "format the disk" pick against a list of destinations is the
	// one mistake here that cannot be undone.
	if player.PendingHDDMenu != HDDMenuNone {
		menu := player.PendingHDDMenu
		if selectedOption == 255 {
			// Same rule as below: a close is not the player's last word, the
			// PS2 sends 255 before the real pick when two boxes are open. But
			// an ARMED format must not survive a closed box, so the arm goes
			// even though the menu stays.
			clearFormatArm(player.ServerID)
			return nil
		}
		player.PendingHDDMenu = HandleHDDMenuPick(sess, player, menu,
			int(selectedOption), func(t string) { h.npcSay(sess, t) })
		if next := HDDMenuFor(player.PendingHDDMenu); next != nil {
			h.sendOptionBox(sess, HDDGreeting, next)
		}
		return nil
	}

	// A spiritmaster's question (spiritmaster.go); a close (255) keeps it
	// open, as a travel list stays below.
	if player.PendingBind.Set {
		if selectedOption != 255 {
			h.answerBind(sess, player, selectedOption)
		}
		return nil
	}
	// A sparring dummy's level or element (sparring.go)
	if player.PendingSpar.Asked != 0 {
		if selectedOption != 255 {
			h.answerSpar(sess, player, selectedOption)
		}
		return nil
	}

	if len(player.PendingCoachDests) == 0 {
		return nil
	}

	// 255 is the client closing a dialogue box, and it is NOT reliably the
	// player's last word: the PS2 sends it immediately BEFORE the real
	// selection when more than one box is open (opening the dialogue twice is
	// easy, and the Interact rate limiter makes a double-click likely).
	// Observed 2026-08-06:
	//
	//   option=255   <- clears the list
	//   option=1     <- "Odus", dropped on the floor, no error anywhere
	//
	// Discarding the destinations here therefore ate a real pick and looked
	// exactly like the travel NPC ignoring the player. Only a selection or the
	// next Interact should replace the list; a close just closes the box.
	if selectedOption == 255 {
		return nil
	}

	// A waysender's one trip, asked as Yes or No (waysender.go)
	if player.PendingYesNo {
		dest := player.PendingCoachDests[0]
		player.PendingCoachDests, player.PendingYesNo = nil, false
		if selectedOption == 0 {
			h.travel(sess, player, dest)
		}
		return nil
	}

	dests := player.PendingCoachDests
	page := player.CoachPage
	total := len(dests)

	start := page * coachDestsPerPage
	end := start + coachDestsPerPage
	if end > total {
		end = total
	}
	destsOnPage := end - start
	hasMore := end < total
	hasBack := page > 0

	idx := int(selectedOption)

	// Check for "More..." selection
	if hasMore && idx == destsOnPage {
		player.CoachPage++
		h.sendCoachPage(sess, player)
		return nil
	}

	// Check for "Back" selection
	moreOffset := 0
	if hasMore {
		moreOffset = 1
	}
	if hasBack && idx == destsOnPage+moreOffset {
		player.CoachPage--
		h.sendCoachPage(sess, player)
		return nil
	}

	// Destination selection
	destIdx := start + idx
	if destIdx >= total {
		log.Printf("DialogueOption: invalid option %d (page %d, total %d)", selectedOption, page, total)
		player.PendingCoachDests = nil
		return nil
	}

	dest := dests[destIdx]
	player.PendingCoachDests = nil
	h.travel(sess, player, dest)
	return nil
}

// travel sends the player on a picked trip: a waysender's level and fee
// first (its words: Sandstorm's "Very well."), then the teleport.
func (h *InteractHandler) travel(sess *session.Session, player *entity.Player, dest entity.CoachDest) {
	if dest.MinLevel > 0 && player.Level < dest.MinLevel {
		h.sendDialogueBox(sess, tooWeak(dest.Name))
		return
	}
	if dest.Fee > 0 && h.spend != nil && !h.spend(sess, player, dest.Fee) {
		h.sendDialogueBox(sess, fmt.Sprintf("The trip to %s costs %d tunar.", dest.Name, dest.Fee))
		log.Printf("Waysender: %s cannot pay %d for %s", player.Name, dest.Fee, dest.Name)
		return
	}
	if dest.Via != "" {
		sess.Queue().AddWithSequence(buildClientMessage(dest.Via + ": " + wayGo))
	}

	log.Printf("Coaching: teleporting %s to %s (%.1f, %.1f, %.1f) world=%d",
		player.Name, dest.Name, dest.X, dest.Y, dest.Z, dest.World)

	// Increment teleport counter
	player.TeleportCounter++

	// Remember the world BEFORE the teleport so it survives a relog. This used
	// to be hardcoded to 0 in the send below, which was correct while every
	// route was intra-Tunaria and silently wrong the moment one was not.
	worldChanged := player.World != dest.World
	player.World = dest.World

	// Send teleport packet
	h.sendTeleport(sess, dest.World, dest.X, dest.Y, dest.Z, dest.Facing, player.TeleportCounter)

	// Hold the state channels until the client has loaded, and drop the
	// entities it can currently see. The client re-arms everything by sending
	// 0x0049 when it is done — see worldload.go.
	beginWorldLoad(sess, worldChanged)

	// Update player position in the world
	h.gameWorld.UpdatePosition(player.ServerID, dest.X, dest.Y, dest.Z, dest.Facing)
}

// sendDialogueBox sends a simple dialogue box (no options) to the client.
// C# reference: Entity.Interaction.SendDialogue with dialogueType = DialogueBox (0x0046)
// Wire format: uint16 opcode + uint32 counter(0) + int32 charCount + UTF-16LE string
func (h *InteractHandler) sendDialogueBox(sess *session.Session, text string) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeDialogueBox)
	writer := buffer.NewWriter(msg.Span())

	writer.WriteUint16(uint16(protocol.OpcodeDialogueBox))
	writer.WriteUint32(0) // counter = 0 for simple dialogue
	writer.WriteStringUnicode(text)

	msg.SetSize(writer.Position())
	sess.Queue().AddWithSequence(msg)
}

// sendOptionBox sends a dialogue with selectable options to the client.
// Wire format (from eqoa.live pcap): uint16 opcode + uint32 dialogueCounter + UTF-16LE text + byte optionCount + [UTF-16LE options...]
// The uint32 is a dialogue counter/ID (pcap shows values like 4), NOT the option count.
func (h *InteractHandler) sendOptionBox(sess *session.Session, text string, options []string) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeQuestPopupMulti)
	writer := buffer.NewWriter(msg.Span())

	writer.WriteUint16(uint16(protocol.OpcodeQuestPopupMulti))
	writer.WriteUint32(0) // dialogue counter (not option count)
	writer.WriteStringUnicode(text)
	writer.WriteByte(byte(len(options))) // text options count
	for _, opt := range options {
		writer.WriteStringUnicode(opt)
	}

	msg.SetSize(writer.Position())
	sess.Queue().AddWithSequence(msg)
}

// sendTeleport sends a teleport packet to the client.
// C# reference: ServerTeleportPlayer.cs (opcode 0x07F6)
// Wire format: uint16 opcode + int32 world + float32 x,y,z,facing + 8 bytes padding + uint32 counter
func (h *InteractHandler) sendTeleport(sess *session.Session, world byte, x, y, z, facing float32, counter uint32) {
	sendTeleportWarp(sess, world, x, y, z, facing, counter)
}

// sendTeleportWarp is the travel NPCs' teleport, as a plain function so the
// account pages can perform the same warp.
//
// THE COUNTER IS THE DIFFERENCE between this and sendTeleportGM, which is
// otherwise byte-for-byte identical and always writes 0. The travel NPCs
// increment a per-player counter for each warp; that is the path proven to work
// live, so anything that wants a warp to behave like a Harbormaster's has to
// send it the same way rather than approximately.
func sendTeleportWarp(sess *session.Session, world byte, x, y, z, facing float32, counter uint32) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeTeleport)
	writer := buffer.NewWriter(msg.Span())

	writer.WriteUint16(uint16(protocol.OpcodeTeleport))
	writer.WriteByte(world) // world is enum World : byte in C#
	writer.WriteFloat32(x)
	writer.WriteFloat32(y)
	writer.WriteFloat32(z)
	writer.WriteFloat32(facing)
	// 8 bytes padding
	writer.WriteUint32(0)
	writer.WriteUint32(0)
	// Teleport counter
	writer.WriteUint32(counter)

	msg.SetSize(writer.Position())
	sess.Queue().AddWithSequence(msg)
}

// npcSay puts one line in the player's chat window, attributed to the
// technician. It is the same ClientMessage the chat command replies with, so
// the NPC and #hddsetup produce identical text for identical outcomes — the
// point of routing both through hddSetupCommand in the first place.
func (h *InteractHandler) npcSay(sess *session.Session, text string) {
	sess.Queue().AddWithSequence(buildClientMessage(HDDSetupNPC + ": " + text))
}
