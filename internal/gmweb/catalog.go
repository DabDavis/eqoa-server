package gmweb

import (
	"strings"

	"github.com/eqoa/server/internal/handlers"
)

// The catalog: what the tables the game reads MEAN. The page builds its forms
// from it and an AI agent reads it through describe_table, so a column is
// explained once. Sources: the C# schema the tables keep (database/sqlite.go),
// what the server's code does with each column, and the root CLAUDE.md's
// verified constants (race and class ids). A column not described here is
// shown and edited as its SQLite type.

// EnumValue is one value of an enumeration.
type EnumValue struct {
	Value int    `json:"value"`
	Label string `json:"label"`
}

// FlagBit is one bit of a bit set.
type FlagBit struct {
	Bit   int    `json:"bit"`
	Label string `json:"label"`
	Help  string `json:"help,omitempty"`
}

// The table groups, in the page's order.
var groups = []string{"Players", "World", "Items & spells", "New characters", "Guilds & quests", "Imports", "Panel", "Other"}

func groupOrder(g string) int {
	for i, x := range groups {
		if x == g {
			return i
		}
	}
	return len(groups)
}

func numbered(labels ...string) []EnumValue {
	out := make([]EnumValue, len(labels))
	for i, l := range labels {
		out[i] = EnumValue{i, l}
	}
	return out
}

// enums are the enumerations columns name; "world" is filled from the
// handlers' own table at each read.
func enums() map[string][]EnumValue {
	worlds := handlers.WorldNames()
	w := make([]EnumValue, len(worlds))
	for i, n := range worlds {
		w[i] = EnumValue{i, n}
	}
	return map[string][]EnumValue{
		// verified on the wire (root CLAUDE.md, "Race/Class IDs"); 10 is the
		// froglok our S2 adds
		"race": numbered("Human", "Elf", "Dark Elf", "Gnome", "Dwarf", "Troll", "Barbarian",
			"Halfling", "Erudite", "Ogre", "Froglok"),
		"class": numbered("Warrior", "Ranger", "Paladin", "Shadow Knight", "Monk", "Bard", "Rogue",
			"Druid", "Shaman", "Cleric", "Magician", "Necromancer", "Enchanter", "Wizard", "Alchemist"),
		"sex":   numbered("Male", "Female"),
		"world": w,
		// handlers/equip.go: charinventory.equiploc, CLIENT's equip slots
		"equiploc": append([]EnumValue{{-1, "Not worn"}}, numbered("Head", "Hands", "Left ear", "Right ear",
			"Neck", "Chest", "Right bracelet", "Left bracelet", "Bracers", "Left ring", "Right ring", "Belt",
			"Legs", "Feet", "Primary", "Secondary", "Two-handed", "Shield", "Ranged", "Held", "Held 2", "Robe")...),
		// handlers/equip.go: itempattern.equipslot, the item's wear slot
		"wearslot": numbered("None", "Head", "Robe", "Earring", "Neck", "Chest", "Forearm", "Both forearms",
			"Ring", "Belt", "Legs", "Feet", "Primary", "Shield", "Secondary", "Two-handed", "Bow", "Thrown",
			"Held", "Hands", "Fishing pole", "Bait", "Tool (1)", "Tool (2)", "Tool (3)", "Tool (4)", "Tool (5)", "Tool (6)"),
		"location":   {{0, "Inventory"}, {1, "Bank"}},
		"acctstatus": {{1, "Active"}, {0, "Inactive"}, {-1, "Banned"}},
		"bool":       {{0, "No"}, {1, "Yes"}},
		// npcmove.roam (entity/movement.go)
		"roam": {{-1, "As its kind (wildlife wanders)"}, {1, "Wanders"}, {0, "Stands until pulled"}},
		// spellarea.centre (entity/area.go)
		"areacentre": {{0, "As its line reads"}, {1, "Round its target"}, {2, "Round its caster"}, {-1, "Not an area spell"}},
		// factionstart (entity/faction.go): -1 any
		"raceany":  append([]EnumValue{{-1, "Any race"}}, numbered("Human", "Elf", "Dark Elf", "Gnome", "Dwarf", "Troll", "Barbarian", "Halfling", "Erudite", "Ogre", "Froglok")...),
		"classany": append([]EnumValue{{-1, "Any class"}}, numbered("Warrior", "Ranger", "Paladin", "Shadow Knight", "Monk", "Bard", "Rogue", "Druid", "Shaman", "Cleric", "Magician", "Necromancer", "Enchanter", "Wizard", "Alchemist")...),
		"humtype":  {{-1, "Any city"}, {0, "Not a human"}, {1, "East (Freeport)"}, {2, "West (Qeynos, Surefall Glade)"}},
		// npcfaction.world (entity/faction.go)
		"worldany": append([]EnumValue{{-1, "Any world"}}, w...),
		// craftrecipes.skill, charcraftskills.skill (handlers/craftrules.go):
		// CLIENT's six slots
		"craftskill": numbered("Weapon Crafting", "Armor Crafting", "Tailoring", "Jewel Crafting", "Carpentry", "Alchemy"),
		// craftrecipes.rarity: the book's Rarity column (C, UC, R, UR)
		"craftrarity": numbered("Common", "Uncommon", "Rare", "Ultra Rare"),
	}
}

// flagSets are the bit sets columns name.
func flagSets() map[string][]FlagBit {
	classes := []FlagBit{}
	for _, e := range enums()["class"] {
		classes = append(classes, FlagBit{1 << e.Value, e.Label, ""})
	}
	races := []FlagBit{}
	for _, e := range enums()["race"][:10] {
		races = append(races, FlagBit{1 << e.Value, e.Label, ""})
	}
	allRaces := []FlagBit{}
	for _, e := range enums()["race"] {
		allRaces = append(allRaces, FlagBit{1 << e.Value, e.Label, ""})
	}
	return map[string][]FlagBit{
		// npcs.npc_type, stored byte-reversed from the wire's m_npcFlags
		// (entity/statesync.go); the bits the server and CLIENT act on
		"npctype": {
			{0x01, "Merchant", "opens the merchant window (its stock is npcitem)"},
			{0x02, "Banker", "opens the bank"},
			{0x08, "Blacksmith", "repairs items"},
			{0x80, "Not attackable", "friendly or neutral; without it and with any other bit set the NPC is hostile"},
			{0x100, "Coach / travel", "set on coachmen and the travel NPCs"},
			{0x400, "Large (+3 reach)", "players reach it from 3 further (CLIENT's TargetInRange)"},
			{0x800, "Huge (+5 reach)", "players reach it from 5 further"},
		},
		"classmask":  classes,  // itempattern.classuse: 32767 = every class
		"racemask":   races,    // itempattern.raceuse: 1023 = every race
		"racemask11": allRaces, // cmcatalog.races: 2047 = every race, the froglok too
	}
}

type colNote struct {
	Label, Help, Enum, Flags, Ref, Format, Section string
	Secret, ReadOnly                               bool
}

type tableNote struct {
	Label, Group, Help, Title, Sort, Write, Live string
	Show, Search                                 []string
	Hidden                                       bool
	Cols                                         map[string]colNote
}

// hex32 marks the 32-bit identifiers the game reads as unsigned: ESF DictIDs
// (models, icons, effects) and colours. The tables hold them both signed and
// unsigned (the importers differ), and the game takes the low 32 bits of
// either, so the page shows them as 0xXXXXXXXX and stores what is typed.
const hex32 = "hex32"

var catalog = map[string]tableNote{
	"accountinfo": {
		Label: "Accounts", Group: "Players", Title: "username", Sort: "accountid",
		Help: "Game accounts. acctlevel 1+ is a GM (in-game # commands and this panel); acctstatus < 0 is banned. " +
			"Passwords are bcrypt hashes and are set with the account tools, never written here.",
		Live:   "A ban (acctstatus below 0) ejects the account's sessions at once. Levels and passwords have their own tools.",
		Show:   []string{"accountid", "username", "acctlevel", "acctstatus", "lastlogin", "ipaddress", "acctcreation"},
		Search: []string{"username", "email", "ipaddress"},
		Cols: map[string]colNote{
			"accountid":    {Label: "Account", ReadOnly: true},
			"username":     {Label: "Name"},
			"pwhash":       {Label: "Password", Secret: true},
			"acctstatus":   {Label: "Status", Enum: "acctstatus", Help: "> 0 active, 0 inactive, < 0 banned"},
			"acctlevel":    {Label: "Level", ReadOnly: true, Help: "0 player, 1+ GM; changed with the account tools (set_account_level)"},
			"acctcreation": {Label: "Created", Format: "stamp"},
			"lastlogin":    {Label: "Last login", Format: "stamp"},
			"ipaddress":    {Label: "Address"},
		},
	},
	"characters": {
		Label: "Characters", Group: "Players", Title: "charName", Sort: "serverid",
		Help: "Player characters. serverid is the character's id everywhere (inventory, spells, buffs). " +
			"A character in the world holds level, purse and place in memory: change those through this table or the " +
			"character tools and they are applied live too.",
		Live: "Offline: everything at the next login. Online: level, tunar, bankTunar and x/y/z/world are applied in the " +
			"world at once (a move is a teleport); the rest at the next login.",
		Show:   []string{"serverid", "charName", "accountid", "level", "tclass", "race", "world", "x", "y", "z", "tunar"},
		Search: []string{"charName"},
		Cols: map[string]colNote{
			"serverid":     {Label: "Character", ReadOnly: true},
			"charName":     {Label: "Name", Section: "Identity"},
			"accountid":    {Label: "Account", Ref: "accountinfo", Section: "Identity"},
			"modelid":      {Label: "Model", Format: hex32, Section: "Look", Help: "CHAR.ESF DictID of the body (race and sex decide it)"},
			"tclass":       {Label: "Class", Enum: "class", Section: "Identity"},
			"race":         {Label: "Race", Enum: "race", Section: "Identity"},
			"humType":      {Label: "Home city", Section: "Identity", Help: "the race's starting city: humans 1 or 2, others 0"},
			"level":        {Label: "Level", Section: "Progress"},
			"sex":          {Label: "Sex", Enum: "sex", Section: "Look"},
			"haircolor":    {Section: "Look"},
			"hairlength":   {Section: "Look"},
			"hairstyle":    {Section: "Look"},
			"faceoption":   {Section: "Look"},
			"totalXP":      {Label: "Total XP", Section: "Progress"},
			"debt":         {Label: "XP debt", Section: "Progress"},
			"tunar":        {Label: "Tunar", Section: "Money"},
			"bankTunar":    {Label: "Bank tunar", Section: "Money"},
			"unusedTP":     {Label: "Unspent TP", Section: "Progress"},
			"totalTP":      {Label: "Total TP", Section: "Progress"},
			"world":        {Label: "World", Enum: "world", Section: "Place"},
			"x":            {Section: "Place"},
			"y":            {Section: "Place", Help: "height; at or under -1000 is a fall"},
			"z":            {Section: "Place"},
			"facing":       {Section: "Place", Format: "radians"},
			"Speed":        {Section: "Place"},
			"currentHP":    {Label: "HP", Section: "Stats"},
			"currentPower": {Label: "Power", Section: "Stats"},
			"tpstrength":   {Label: "TP in STR", Section: "Stats"},
			"tpstamina":    {Label: "TP in STA", Section: "Stats"},
			"tpagility":    {Label: "TP in AGI", Section: "Stats"},
			"tpdexterity":  {Label: "TP in DEX", Section: "Stats"},
			"tpwisdom":     {Label: "TP in WIS", Section: "Stats"},
			"tpintel":      {Label: "TP in INT", Section: "Stats"},
			"tpcharisma":   {Label: "TP in CHA", Section: "Stats"},
			"breath":       {Section: "Stats", Help: "0-255, 255 full"},
		},
	},
	"charinventory": {
		Label: "Character items", Group: "Players", Sort: "serverid",
		Help: "Every item a character has: location 0 the inventory, 1 the bank; equiploc the slot it is worn in (-1 not worn).",
		Live: "A character in the world gets its items at its next login (eject it to apply now).",
		Show: []string{"itemid", "serverid", "patternid", "stackleft", "remainHP", "equiploc", "location", "listnumber"},
		Cols: map[string]colNote{
			"itemid":       {Label: "Item row", ReadOnly: true},
			"serverid":     {Label: "Character", Ref: "characters"},
			"patternid":    {Label: "Item", Ref: "itempattern"},
			"stackleft":    {Label: "Stack"},
			"remainHP":     {Label: "Condition", Help: "the item's hit points (repairs restore them)"},
			"remaincharge": {Label: "Charges"},
			"equiploc":     {Label: "Worn in", Enum: "equiploc"},
			"location":     {Label: "Where", Enum: "location"},
			"listnumber":   {Label: "Order", Help: "its place in the list"},
		},
	},
	"spells": {
		Label: "Character spells", Group: "Players", Sort: "serverid",
		Help: "Each character's spell book: one row a spell. addedorder is its place in the book (below 252).",
		Live: "A character in the world gets its book at its next login.",
		Show: []string{"id", "serverid", "spellid", "addedorder", "onHotBar", "whereonBar"},
		Cols: map[string]colNote{
			"serverid":   {Label: "Character", Ref: "characters"},
			"spellid":    {Label: "Spell", Ref: "spellpattern"},
			"addedorder": {Label: "Book order"},
			"onHotBar":   {Label: "On the bar", Enum: "bool"},
			"whereonBar": {Label: "Bar slot", Help: "-1 for none"},
		},
	},
	"charfactions": {
		Label: "Character factions", Group: "Players", Sort: "serverid",
		Help: "A character's standing with each faction it has moved off its start (-2000 to 2000); with no row it has " +
			"its start: its race, class and city's (Faction starts), else the faction's own (Factions).",
		Live: "At once where the character stands, else at its next login.",
		Cols: map[string]colNote{
			"serverid": {Label: "Character", Ref: "characters"},
			"faction":  {Label: "Faction", Ref: "factions"},
			"standing": {Label: "Standing"},
		},
	},
	"charbuffs": {
		Label: "Character effects", Group: "Players",
		Help: "Spell effects kept over a logout, what is left of each (entity/buffsave.go).",
		Live: "Read when the character next enters the world.",
		Cols: map[string]colNote{
			"serverid": {Label: "Character", Ref: "characters"},
			"spellid":  {Label: "Spell", Ref: "spellpattern"},
			"icon":     {Format: hex32},
			"caster":   {Ref: "characters"},
			"left_ms":  {Label: "Left (ms)"},
			"mods":     {Format: "json"},
		},
	},
	"hotkeys": {
		Label: "Hotkeys", Group: "Players",
		Help: "Characters' quick-menu hotkeys (the L2 d-pad).",
		Cols: map[string]colNote{"serverid": {Label: "Character", Ref: "characters"}},
	},
	"weaponhotbar": {
		Label: "Weapon hotbar (unused)", Group: "Players",
		Help: "Return Home's schema, never read: the weapon sets are gearsets.",
		Cols: map[string]colNote{"serverid": {Label: "Character", Ref: "characters"}},
	},
	"quickslots": {
		Label: "Item quick slots", Group: "Players",
		Help: "The item prepared on each of the HUD's four item quick slots (0-1 potions, 2-3 gems; on a PS2 slot 3 is a weapon's). Read at the character's next login.",
		Cols: map[string]colNote{
			"serverid": {Label: "Character", Ref: "characters"},
			"pos":      {Label: "Slot"},
			"itemid":   {Label: "Item", Ref: "charinventory"},
		},
	},
	"gearsets": {
		Label: "Weapon sets", Group: "Players",
		Help: "The HUD's four weapon sets (SOE's live client): the main and off hand as worn when the set was prepared, each by charinventory item and its item pattern; -1 is none, an empty set has no main hand.",
		Cols: map[string]colNote{
			"serverid":    {Label: "Character", Ref: "characters"},
			"slot":        {Label: "Set (0-3)"},
			"name":        {Label: "Name"},
			"mainitem":    {Label: "Main hand", Ref: "charinventory"},
			"mainpattern": {Label: "Main hand item", Ref: "itempattern"},
			"offitem":     {Label: "Off hand", Ref: "charinventory"},
			"offpattern":  {Label: "Off hand item", Ref: "itempattern"},
		},
	},
	"charquests": {
		Label: "Character quests", Group: "Guilds & quests", Sort: "serverid",
		Help: "How far each character has come in each quest: step is the step to do (0 when finished), count the " +
			"creatures killed so far on a kill step, done how many times it was finished, slot the journal's order. " +
			"The Quests page's Characters tab gives, sets, finishes and takes back quests with the journal kept in step.",
		Live: "At once: a character in the world has the journal redrawn as the row says.",
		Cols: map[string]colNote{
			"serverid": {Label: "Character", Ref: "characters"},
			"questid":  {Label: "Quest", Ref: "quests"},
			"step":     {Label: "Step to do", Help: "0: finished"},
			"count":    {Label: "Killed so far"},
			"done":     {Label: "Times finished"},
			"slot":     {Label: "Journal order"},
			"updated":  {Label: "Updated", Format: "stamp", ReadOnly: true},
		},
	},
	"npcs": {
		Label: "NPCs", Group: "World", Title: "npc_name", Sort: "id",
		Help: "Every NPC spawn: one row one NPC in the world, its id its entity id. npc_type's bits make it a merchant, " +
			"banker, blacksmith or non-attackable (see the flags). modelid is the CHAR.ESF DictID of its body; copy one from " +
			"an NPC that looks right. Gear columns are the item models (primary_weapon, shield) and armour pieces it wears.",
		Live: "At once: an edited NPC disappears for a second and comes back as edited, alive on its spawn point; a new row " +
			"spawns; a deleted row leaves the world.",
		Show:   []string{"id", "npc_name", "npc_level", "world", "x_coord", "y_coord", "z_coord", "npc_type", "modelid"},
		Search: []string{"npc_name"},
		Cols: map[string]colNote{
			"id":               {Label: "NPC", ReadOnly: true},
			"npc_name":         {Label: "Name", Section: "Identity", Help: "names drive some behaviour: \"Training Dummy …\", coachmen, travel NPCs, Archivist Vaal (the login spawn)"},
			"npc_level":        {Label: "Level", Section: "Identity"},
			"npc_type":         {Label: "Type", Flags: "npctype", Section: "Identity"},
			"npc_id":           {Label: "NPC id", Section: "Identity", Help: "Sandstorm's id for the NPC (imports), not the row"},
			"race":             {Label: "Race", Section: "Look", Help: "text: '0'..'10' as the race ids; scales the body"},
			"modelid":          {Label: "Model", Format: hex32, Section: "Look", Help: "CHAR.ESF DictID of the body"},
			"size":             {Label: "Size", Section: "Look", Help: "1.0 normal, sent as is (a gnome body is already gnome-sized)"},
			"world":            {Label: "World", Enum: "world", Section: "Place"},
			"zone":             {Label: "Zone", Section: "Place"},
			"x_coord":          {Label: "X", Section: "Place"},
			"y_coord":          {Label: "Y (height)", Section: "Place"},
			"z_coord":          {Label: "Z", Section: "Place"},
			"facing":           {Label: "Facing", Format: "angle256", Section: "Place", Help: "0-255 is a full turn"},
			"primary_weapon":   {Label: "Primary", Format: hex32, Section: "Gear", Help: "ITEM.ESF model DictID, 0 none"},
			"secondary_weapon": {Label: "Secondary", Format: hex32, Section: "Gear"},
			"shield":           {Label: "Shield", Format: hex32, Section: "Gear"},
			"torso":            {Section: "Gear", Help: "armour set index"},
			"forearm":          {Section: "Gear"},
			"gloves":           {Section: "Gear"},
			"legs":             {Section: "Gear"},
			"feet":             {Section: "Gear"},
			"head":             {Section: "Gear"},
			"robe_style":       {Label: "Robe", Section: "Gear", Help: "text: '0'-'3' a robe, empty or '255' none"},
			"hair_color":       {Section: "Look"},
			"hair_length":      {Section: "Look"},
			"hair_style":       {Section: "Look"},
			"face":             {Section: "Look"},
			"hp":               {Section: "Other", Help: "not read: hit points come from the level"},
			"hp_percent":       {Section: "Other"},
			"dt_kill":          {Section: "Other"},
		},
	},
	"npcpaths": {
		Label: "NPC paths", Group: "World", Sort: "npcid",
		Help: "The paths GMs give NPCs (the World map's Path editor draws them): one row an NPC, by its npcs id; points " +
			"is its waypoints in order, a JSON list of {x, y, z} (and pause, seconds there); mode loop, pingpong (back and " +
			"forth) or once (then it stands at the last point); pause, seconds at each point; speed walk or run. A pathed " +
			"NPC does not wander; a fight takes it off its path and it takes it up again at the nearest leg.",
		Live: "At once: the NPC walks its new path from its nearest leg; a deleted path sends it back to its spawn point.",
		Show: []string{"npcid", "world", "mode", "pause", "speed"},
		Cols: map[string]colNote{
			"npcid":  {Label: "NPC", Ref: "npcs"},
			"world":  {Label: "World", Enum: "world", Help: "the NPC's own world (a path in another is not walked)"},
			"mode":   {Label: "Mode", Help: "loop, pingpong (back and forth) or once"},
			"pause":  {Label: "Pause (s)", Help: "seconds at each point, 0-600"},
			"speed":  {Label: "Speed", Help: "walk (5 a second) or run (8.8)"},
			"points": {Label: "Waypoints", Format: "multiline", Help: `[{"x": 25000, "y": 50, "z": 15000}, {"x": 25040, "y": 51, "z": 15000, "pause": 5}]`},
		},
	},
	"npcitem": {
		Label: "Merchant stock", Group: "World", Sort: "npcid",
		Help: "What a merchant sells: a row an item, by npcs.id; listnumber orders the window. A creature's rows are " +
			"the loot its corpse holds every death (its loot tables, Loot tables and Creature loot, roll more beside them).",
		Live: "At once: the window reads its stock each time it opens.",
		Show: []string{"itemid", "npcid", "patternid", "stackleft", "listnumber"},
		Cols: map[string]colNote{
			"itemid":     {Label: "Row", ReadOnly: true},
			"npcid":      {Label: "Merchant", Ref: "npcs"},
			"patternid":  {Label: "Item", Ref: "itempattern"},
			"stackleft":  {Label: "Stack"},
			"listnumber": {Label: "Order"},
		},
	},
	"loottables": {
		Label: "Loot tables", Group: "World", Title: "tablename", Sort: "tablename",
		Help: "The items a loot table may give, a row an item (handlers/loot.go): the table's name (an EQOA-Data biome's, " +
			"arctic desert forest marine mountain plains swamp, or rathe mountains, red desert, wetlands, deep jungle, " +
			"wastelands, generic, or a named creature's own), the item, its weight among the table's (2 is twice as " +
			"likely as 1; 0 never) and the creature levels it drops for. Creature loot binds creatures to tables. Rows " +
			"of source camp-seed are tools/seed_camp_loot.py's, EQOA-Data's items by the table their source names, " +
			"each for creatures within 5 levels of its own; the weights are ours.",
		Live:   "At once: the next corpse of a creature bound to it rolls with it.",
		Search: []string{"tablename", "source"},
		Show:   []string{"id", "tablename", "patternid", "weight", "minlevel", "maxlevel", "source"},
		Cols: map[string]colNote{
			"id":        {Label: "Row", ReadOnly: true},
			"tablename": {Label: "Table"},
			"patternid": {Label: "Item", Ref: "itempattern"},
			"weight":    {Label: "Weight", Help: "its share of the table's rolls: 2 twice as likely as 1, 0 never"},
			"minlevel":  {Label: "From level", Help: "the lowest creature level it drops for"},
			"maxlevel":  {Label: "To level", Help: "the highest creature level it drops for"},
			"source":    {Label: "Source", Help: "gm, or camp-seed for tools/seed_camp_loot.py's rows"},
		},
	},
	"npcloot": {
		Label: "Creature loot", Group: "World", Title: "npc_name", Sort: "npc_name",
		Help: "Which loot tables a creature's corpse rolls at its death, by NPC NAME as NPC classes key it (lower case, " +
			"no trailing space or $), beside its own Merchant stock rows: each roll lands at its chance (percent) and " +
			"takes one of the table's items for the creature's level, by weight; rolls is how many a kill makes. A " +
			"GM's #spawn copy rolls as its original; a pet's corpse holds nothing. Rows of source camp-seed are " +
			"tools/seed_camp_loot.py's: the XP camps' creatures (#tele camps) on their zone's biome table, from " +
			"EQOA-Data's map, and the generic table; the chances are ours, not the live game's.",
		Live:   "At once: the next corpse of that name rolls with it.",
		Search: []string{"npc_name", "tablename", "note", "source"},
		Cols: map[string]colNote{
			"npc_name":  {Label: "NPC name (key)", Help: "lower case, as in NPCs without a trailing space or $"},
			"tablename": {Label: "Table", Help: "a Loot tables name"},
			"chance":    {Label: "Chance %", Help: "a roll's chance to give an item, 0-100"},
			"rolls":     {Label: "Rolls", Help: "how many rolls a kill makes"},
			"note":      {Label: "Note", Help: "why"},
			"source":    {Label: "Source", Help: "gm, or camp-seed for tools/seed_camp_loot.py's rows (its --undo deletes those no GM changed)"},
		},
	},
	"npcclass": {
		Label: "NPC classes", Group: "World", Title: "name", Sort: "name",
		Help: "Each NPC NAME's class (lower case, no trailing $), which picks its melee swing; a name not here is a warrior.",
		Live: "At once: every NPC of that name swings as the new class.",
		Cols: map[string]colNote{
			"name":  {Label: "NPC name (key)"},
			"class": {Label: "Class", Enum: "class"},
		},
	},
	"factions": {
		Label: "Factions", Group: "World", Title: "name", Sort: "name",
		Help: "Every faction and the standing a character starts with (-2000 to 2000) where its race, class and city have " +
			"none of their own (Faction starts). Filled from an eqoa.live capture's 331 (one gnome's: Klick`Anon 300, " +
			"Qeynos -2000) when empty; a character's own standings are Character factions.",
		Live: "At once: every faction window, target box and the next kill read it.",
		Cols: map[string]colNote{
			"name":     {Label: "Faction"},
			"standing": {Label: "Starting standing", Help: "-2000 to 2000"},
		},
	},
	"factionstart": {
		Label: "Faction starts", Group: "World", Title: "faction", Sort: "faction",
		Help: "The standing the characters of a race, class and home city start with in a faction where it is not the " +
			"faction's own (Factions): -1 in a column is any, and the row naming most of the three wins (then the one " +
			"naming the race, then the class). A froglok starts as a human of the east. A character takes its start for " +
			"every faction it has not moved (Character factions). Rows of source seed are tools/seed_faction_starts.py's, " +
			"Sandstorm's starts measured per race, class and city; its --undo takes them out.",
		Live:   "At once: every faction window, target box and look round (half a second) after it uses it.",
		Search: []string{"faction", "note", "source"},
		Cols: map[string]colNote{
			"race":     {Label: "Race", Enum: "raceany"},
			"class":    {Label: "Class", Enum: "classany"},
			"humtype":  {Label: "Home city", Enum: "humtype", Help: "a human's city: 1 east, 2 west; 0 the other races"},
			"faction":  {Label: "Faction", Ref: "factions"},
			"standing": {Label: "Starting standing", Help: "-2000 to 2000"},
			"note":     {Label: "Note"},
			"source":   {Label: "Source", Help: "gm, or seed for tools/seed_faction_starts.py's rows (its --undo takes those out)"},
		},
	},
	"npcfaction": {
		Label: "NPC factions", Group: "World", Title: "name", Sort: "name",
		Help: "Which faction an NPC is of, by NPC NAME as NPC classes key it (lower case, no trailing space or $). Its " +
			"target box shows a player the player's standing with it (below -700 Aggressive, Dubious, Cautious from -350, " +
			"Neutral from -150, Friendly from 150, Warm, Allied from 700), and it attacks on sight a player whose standing " +
			"is below -350 (the red faces), at any level, and no one else, whatever its body (entity/aggro.go); one with the NPC bit (0x80) " +
			"never attacks, and a Creature aggro row for the name decides its aggro as before. world -1: every NPC of the " +
			"name; a world with radius 0: those in it; a radius: those that spawned within it of (x, z) in that world, " +
			"the smallest place first (a city's namesakes told from the wild's). An empty faction: of none. Rows of " +
			"source seed are tools/seed_npc_factions.py's: a town's people under its Citizens faction (its guards its " +
			"guard faction), the houses by name, the creatures among a town's people; its --undo takes them out. " +
			"world.faction_aggro: false leaves factions out of aggro and faces.",
		Live:   "At once: the next look round (half a second) and the next target box use it.",
		Search: []string{"name", "faction", "note", "source"},
		Cols: map[string]colNote{
			"name":    {Label: "NPC name (key)", Help: "lower case, as in NPCs without a trailing space or $"},
			"world":   {Label: "World", Enum: "worldany"},
			"x":       {Label: "Place x", Help: "with radius: the middle of the place"},
			"z":       {Label: "Place z"},
			"radius":  {Label: "Radius", Help: "0: anywhere in the world; else only the NPCs that spawned this near (x, z)"},
			"faction": {Label: "Faction", Ref: "factions", Help: "empty: of none"},
			"note":    {Label: "Note"},
			"source":  {Label: "Source", Help: "gm, or seed for tools/seed_npc_factions.py's rows (its --undo takes those out)"},
		},
	},
	"factionhits": {
		Label: "Faction hits", Group: "World", Title: "npc_name", Sort: "npc_name",
		Help: "What killing an NPC does to its killer's standings: each row adds delta to the standing with one faction " +
			"for a kill of any NPC of that NAME (a #spawn copy too). The killer alone, as the kill's XP. Town rows " +
			"(source town-seed) are tools/seed_faction_hits.py's guesses, not measured: no capture holds a faction hit; " +
			"quest-seed rows are tools/seed_quest_hits.py's, the kills EQOA-Data's guides say earn a quest's faction gate " +
			"(+5 a kill, ours).",
		Live:   "At once: the next kill of that NPC uses it.",
		Search: []string{"npc_name", "faction", "source"},
		Cols: map[string]colNote{
			"npc_name": {Label: "NPC name", Help: "as in NPCs, exactly (a trailing space included)"},
			"faction":  {Label: "Faction", Ref: "factions"},
			"delta":    {Label: "Change", Help: "added to the killer's standing; below 0 lowers it"},
			"source":   {Label: "Source", Help: "gm; town-seed (tools/seed_faction_hits.py) or quest-seed (tools/seed_quest_hits.py), each tool's --undo deletes its own"},
		},
	},
	"npcaggro": {
		Label: "Creature aggro", Group: "World", Title: "npc_name", Sort: "npc_name",
		Help: "Which creatures attack a player on sight, by NPC name, where the rule is wrong for them (entity/aggro.go): " +
			"a creature (a body no player race has, not an NPC-bit NPC) standing outside the towns attacks a player " +
			"below three times its level who comes within 30, and shows that player the red face; townsfolk, guards " +
			"and bandits with players' bodies do not. aggressive 1 makes every NPC of the name attack on sight, 0 never; " +
			"radius how near (0: 30). An NPC with the NPC bit (0x80) never does: no one may attack it.",
		Live:   "At once: the next look round (half a second) and the next target box use it.",
		Search: []string{"npc_name", "note"},
		Cols: map[string]colNote{
			"npc_name":   {Label: "NPC name", Help: "as in NPCs, exactly (a trailing space included)"},
			"aggressive": {Label: "Attacks on sight", Help: "1 yes, 0 never"},
			"radius":     {Label: "Radius", Help: "how near a player must come; 0 the default, 30"},
			"note":       {Label: "Note"},
			"source":     {Label: "Source", Help: "gm, or seed for tools/seed_npc_aggro.py's rows (its --undo deletes those)"},
		},
	},
	"npcundead": {
		Label: "Undead creatures", Group: "World", Title: "name", Sort: "name",
		Help: "Which creatures are undead, by NPC NAME (lower case, no trailing $): the spells for undead alone (a " +
			"\"Target undead\" line, as the ... Death nukes have, and the necromancer's Charm, Command, Control and Coerce " +
			"Undead) are refused at anything else with \"This spell only works on the undead.\", and every other charm " +
			"fails on undead. undead 1 makes every NPC of the name undead, 0 none, whatever its name says; a name with no " +
			"row is undead when its name says so (skeleton, zombie, ghoul, mummy, wraith, spectre, ghost...). Rows of " +
			"source seed are tools/seed_npc_undead.py's, from the creatures' names and bodies: no record of the live game " +
			"says which creatures were undead.",
		Live:   "At once: the next cast at a creature of that name uses it.",
		Search: []string{"name", "note"},
		Cols: map[string]colNote{
			"name":   {Label: "NPC name (key)", Help: "lower case, as in NPCs without a trailing space or $"},
			"undead": {Label: "Undead", Help: "1 yes, 0 no"},
			"note":   {Label: "Note", Help: "why"},
			"source": {Label: "Source", Help: "gm, or seed for tools/seed_npc_undead.py's rows (its --undo deletes those)"},
		},
	},
	"npcdialog": {
		Label: "NPC dialog", Group: "World", Title: "line", Sort: "town",
		Help: "What a town's people and the guards of Norrath say when talked to and nothing else answers (no quest, " +
			"shop, route, bind or sign post's words; entity/dialog.go). A speaker is an NPC of a town's faction (NPC " +
			"factions: a town's Citizens, its guards', Neriak's houses) or one named a guard, on a body of a players' race, " +
			"never wildlife, undead, a pet or a dummy. Each row is one line and who says it to whom; an empty field fits " +
			"anyone: town (as its faction names it: Freeport, Klick`Anon), role (by the speaker's name: guard, trader, " +
			"priest, scholar, worker, noble, citizen), name (one NPC), mood (empty: neutral, friendly or warm; else a comma " +
			"list of neutral, friendly, warm, wary, hostile: the listener's standing with the speaker's faction, as the " +
			"target box's face shows it), race (the listener's), weight. A line of its town counts three times, of its role " +
			"twice, of its name six times. In a line %p is the listener's name, %r its race, %c its class, %t the town, %n " +
			"the speaker. tools/seed_town_dialog.py's rows are source ours (its --undo deletes those no GM changed).",
		Live:   "At once: the next talk.",
		Search: []string{"line", "town", "role", "name", "mood", "source"},
		Cols: map[string]colNote{
			"town":   {Label: "Town", Help: "the speaker's town, as its faction names it; empty any"},
			"role":   {Label: "Role", Help: "guard, trader, priest, scholar, worker, noble or citizen; empty any"},
			"name":   {Label: "NPC", Help: "one NPC's name; empty any"},
			"mood":   {Label: "Mood", Help: "empty: neutral, friendly or warm; else e.g. wary or hostile,friendly"},
			"race":   {Label: "Listener's race", Enum: "race", Help: "-1 any"},
			"line":   {Label: "Line", Help: "%p name, %r race, %c class, %t town, %n speaker"},
			"weight": {Label: "Weight", Help: "how often against the others that fit"},
			"source": {Label: "Source", Help: "gm, or ours for tools/seed_town_dialog.py's rows"},
		},
	},
	"npcsocial": {
		Label: "Social NPCs", Group: "World", Title: "npc_name", Sort: "npc_name",
		Help: "Which NPCs come to the help of their own, by NPC name (entity/socialaggro.go): when a player (or a player's " +
			"pet) hits an NPC or casts something hostile on it, every NPC of the same faction whose row says social 1 and " +
			"stands within its radius of it (default 30, a spell's usual range) joins the fight against that player. " +
			"faction is the faction (or any group name) it belongs to and helps; an attacked NPC is of its own row's " +
			"faction and of every faction its kill lowers (Faction hits), so only the NPCs that help need a row. A " +
			"creature noticing a player on its own calls no one; one already fighting, going home, a pet or with the NPC " +
			"bit (0x80) does not come. The town guards are tools/seed_npc_social.py's rows, each town's guards under its " +
			"guard faction (or \"Guards of <town>\"): guards help guards, not townsfolk.",
		Live:   "At once: the next hit uses it.",
		Search: []string{"npc_name", "faction", "note", "source"},
		Cols: map[string]colNote{
			"npc_name": {Label: "NPC name", Help: "as in NPCs, exactly (a trailing space included)"},
			"social":   {Label: "Helps", Help: "1 comes to the help of its faction, 0 never"},
			"faction":  {Label: "Faction", Help: "the faction or group it belongs to and helps; empty: the faction its kill lowers most (Faction hits)"},
			"radius":   {Label: "Radius", Help: "how near the NPC attacked it must stand; 0 the default, 30"},
			"note":     {Label: "Note"},
			"source":   {Label: "Source", Help: "gm, or seed for tools/seed_npc_social.py's rows (its --undo deletes those)"},
		},
	},
	"npcmove": {
		Label: "Creature movement", Group: "World", Title: "name", Sort: "name",
		Help: "How the creatures of a name move, by NPC NAME as NPC classes key it (lower case, no trailing space or $; " +
			"entity/movement.go, wildlife.go, water.go). Wanders: as its kind (-1, the default, and what a name with no row " +
			"does): wildlife wanders round its spawn, anything else stands on its spawn until pulled and goes back there " +
			"after; 1 wanders, wildlife or not (a camp's roamers, a patrol; townsfolk bodies too); 0 stands until pulled. " +
			"Wildlife is an animal by its name's last word (Giant Rat, Wolf Pack Leader, red ant warrior: not Fire Giant, Orc " +
			"Pawn, serpent guardian, nor an undead Skeletal Wolf); an NPC's page says whether it wanders and why. An NPC " +
			"players talk to (the NPC bit) never wanders: a path walks it. radius its own wander radius (0: the server's, " +
			"75); water what it keeps to: land (it never wanders into the water), both (in and out of it: crocodiles, " +
			"caiman, crabs), water (it keeps to the water, wandering and fighting), empty by where it spawned. A fight takes " +
			"a creature where its enemy is, but one of the water stops at the water's edge; no damage crosses the water's " +
			"edge (a harmful spell is refused, a melee hit lost). A creature that stands off the ground (a keep's floor, a " +
			"bridge, a dock) does not wander whatever its row says. Rows of source seed are tools/seed_npc_move.py's (camp " +
			"guards standing, the water's creatures, animals the words miss by the bodies they wear, the camps EQOA-Data's " +
			"map says roam); the water is the game's own, baked off the disc beside the ground files " +
			"(worlds/ground/<world>.env).",
		Live:   "At once: the next step of every creature of that name uses it.",
		Search: []string{"name", "water", "note", "source"},
		Cols: map[string]colNote{
			"name":   {Label: "NPC name (key)", Help: "lower case, as in NPCs without a trailing space or $"},
			"roam":   {Label: "Wanders", Enum: "roam", Help: "-1 as its kind (wildlife wanders, the rest stands), 1 wanders, 0 stands until pulled"},
			"radius": {Label: "Radius", Help: "how far from its spawn it wanders; 0 the server's (75)"},
			"water":  {Label: "Water", Help: "land, both, water, or empty: by where it spawned (in the water: water)"},
			"note":   {Label: "Note", Help: "why"},
			"source": {Label: "Source", Help: "gm, or seed for tools/seed_npc_move.py's rows (its --undo deletes those)"},
		},
	},
	"cmcatalog": {
		Label: "Class masteries", Group: "Items & spells", Title: "name", Sort: "id",
		Help: "Every class mastery a character may buy in the Class Mastery window, a row a rank (entity/cmbuy.go). " +
			"category is the window's: 0 General, 1 Racial, 2 Archetype, 3 Class, 4 Master Class (one master class at " +
			"a time). id is the window's skill id; parent the rank owned first: the rank before it, but a master " +
			"class's abilities (its ranks after the first) the master class's first rank, so they are bought in any " +
			"order once it is. A rank is " +
			"listed to the classes and races its bit sets name, and buyable (\"known\") once the character's level, " +
			"the points it has spent, its base stats (minstats, \"STR:100,STA:150\"), the abilities it needs " +
			"(require_spells), its parent, the other masteries' ranks it needs (require_ranks) and the quests it needs " +
			"finished (require_quests: the stat-boost quests open the stat-boost lines, tools/gate_stat_masteries.py) are " +
			"there; the cost is then taken from its unspent points. effects is " +
			"what it gives, in EQOA-Data's names: STR..CHA, STRMax..CHAMax (the stat caps: a character's cap is the " +
			"level's, 400 from 46 and 350 below, plus what its masteries raise), HP, Pow, HoT, PoT, AC, Off Mod and " +
			"Def Mod (shown), HP Factor (a level's HP each), FR CR LR AR PR DR, MovRate (20 a run, 100 Spirit of the " +
			"Wolf). add_spells are given at the purchase, remove_spells taken out (gone at the next login); a name " +
			"with no id shows and gives nothing. A removal gives nothing back (#cmreset does). Rows of source " +
			"\"eqoa-data cms\" are tools/seed_masteries.py's, from EQOA-Data's lists (its --undo deletes those).",
		Live:   "At once: every list opened from now on, and the stats of the characters online who own the rank.",
		Search: []string{"name", "mastery", "effects", "adds", "description"},
		Show:   []string{"id", "name", "category", "cost", "level", "spent", "parent", "effects", "adds", "enabled"},
		Cols: map[string]colNote{
			"id":             {Label: "Id", Help: "the window's skill id; never change one a character owns (charcms)"},
			"category":       {Label: "Category", Help: "0 General, 1 Racial, 2 Archetype, 3 Class, 4 Master Class"},
			"classes":        {Label: "Classes", Flags: "classmask"},
			"races":          {Label: "Races", Flags: "racemask11"},
			"mastery":        {Label: "Mastery", Help: "the mastery the rank is of (Brawn); a master class's ranks share one"},
			"rank":           {Label: "Rank"},
			"name":           {Label: "Name", Help: "as the window lists it, at most 63 characters"},
			"parent":         {Label: "Rank before", Ref: "cmcatalog", Help: "owned first: the rank before it, or for a master class's ability the master class (its first rank); 0 for a first rank"},
			"cost":           {Label: "Cost", Help: "mastery points"},
			"level":          {Label: "Level"},
			"spent":          {Label: "Points spent", Help: "the mastery points the character must have spent already"},
			"minstats":       {Label: "Base stats needed", Help: "STR:100,STA:150 (base and training, and what masteries add)"},
			"requires":       {Label: "Abilities needed", Help: "Name@level; ... as shown"},
			"require_ranks":  {Label: "Masteries needed", Help: "other masteries' ranks owned first, their ids (6634, ...); the rank before it in its own line is Rank before; an id the catalog lacks is not checked, and a rank another one needs is not removed before it"},
			"require_spells": {Label: "Abilities needed (spells)", Help: "their spellpattern ids, in the same order: the book must hold them; 0 is not checked"},
			"require_quests": {Label: "Quests needed", Help: "quests finished first, their ids (quests.questid: 101195, ...); a quest not in the game (a draft, or none) bars no one, and a quest put in or taken out counts at once"},
			"effects":        {Label: "Gives", Help: "STR:1,STRMax:15 in EQOA-Data's names (the table's help lists them)"},
			"adds":           {Label: "Adds abilities", Help: "Name@level; ... as shown"},
			"add_spells":     {Label: "Adds (spells)", Help: "their spellpattern ids, put in the book at the purchase"},
			"removes":        {Label: "Replaces abilities", Help: "Name@level; ... as shown"},
			"remove_spells":  {Label: "Replaces (spells)", Help: "their spellpattern ids, taken out of the book at the purchase (given back at a removal)"},
			"description":    {Label: "Text", Help: "the inspect window's, at most 255 characters with the cost the server adds"},
			"enabled":        {Label: "Offered", Enum: "bool"},
			"source":         {Label: "Source", Help: "gm, or eqoa-data cms for tools/seed_masteries.py's rows"},
		},
	},
	"charcms": {
		Label: "Character masteries", Group: "Players", Sort: "serverid",
		Help: "The class masteries each character bought (Class masteries) and what each cost it; its points are " +
			"Character mastery points. A row deleted here gives no points back: in the game #cmreset does both.",
		Live: "At once where the character stands (its stats and caps), else at its next login.",
		Cols: map[string]colNote{
			"serverid": {Label: "Character", Ref: "characters"},
			"cmid":     {Label: "Mastery", Ref: "cmcatalog"},
			"cost":     {Label: "Paid", Help: "what a removal gives back"},
			"bought":   {Label: "Bought", Help: "Unix time"},
		},
	},
	"charmastery": {
		Label: "Character mastery points", Group: "Players", Sort: "serverid",
		Help: "Each character's mastery points: unspent, spent (at most 1500 together), the share of experience " +
			"going to them (0-100) and the experience toward the next point.",
		Live: "At the character's next login; a character in the world saves its own over it at its logout (in the " +
			"game #grantcm gives points at once).",
		Cols: map[string]colNote{
			"serverid": {Label: "Character", Ref: "characters"},
			"points":   {Label: "Unspent"},
			"spent":    {Label: "Spent"},
			"percent":  {Label: "Share of experience"},
			"xp":       {Label: "Toward the next point"},
		},
	},
	"itemeffect": {
		Label: "Item uses", Group: "Items & spells", Title: "patternid", Sort: "patternid",
		Help: "What using an item does (entity/potion.go): the alchemists' potions, drunk or thrown. line is in a spell " +
			"line's words, its first word on whom: Self (drunk) or Target (thrown at the creature targeted, within 20). " +
			"name is the effect's name on the effects list (empty: the item's), icon its icon there (an ability's icon, " +
			"spellpattern.icon). breaks: attack (ended by its bearer's attack, harmful cast or throw), damage (by a hit), " +
			"both as attack,damage. wear: the percent chance each six-second tick that it wears off. area: a thrown " +
			"one's splash round its target. A thrown potion worn as a thrown weapon (wear 17, attack type 10) is each " +
			"auto-attack's throw. Spells give potions by their words (\"Summon Watery Draught\", \"x3\"), the item by name.",
		Live:   "At once: read at every use. Which items are thrown in an auto-attack is read at the server's start.",
		Search: []string{"line", "name"},
		Cols: map[string]colNote{
			"patternid": {Label: "Item", Ref: "itempattern"},
			"line":      {Label: "What it does", Help: "spelleffect's words: Self, Decrease Hitpoints by 20, Increase Power per tic by 25, 18sec"},
			"name":      {Label: "Effect name", Help: "on the effects list; empty: the item's name"},
			"icon":      {Label: "Effect icon", Format: hex32, Help: "an ability's front icon (spellpattern.icon)"},
			"breaks":    {Label: "Ends on", Help: "attack, damage, or attack,damage"},
			"wear":      {Label: "Wears off", Help: "percent a six-second tick"},
			"area":      {Label: "Splash", Help: "a thrown one's: how far round its target creatures take it too"},
			"source":    {Label: "Source", Help: "gm, or potions for tools/seed_potions.py's rows (its --undo deletes those)"},
		},
	},
	"itempattern": {
		Label: "Items", Group: "Items & spells", Title: "itemname", Sort: "patternid",
		Help: "Every item that exists, by patternid. model is the ITEM.ESF DictID worn or held, itemicon the ITEMICON.ESF " +
			"icon; equipslot where it is worn; classuse and raceuse who may (bit sets: 32767 every class, 1023 every race); " +
			"weapondamage is the DMG the offence roll uses; stats and resists are what wearing it adds.",
		Live:   "Merchants' windows show it at once; a player's own items take it at their next login.",
		Show:   []string{"patternid", "itemname", "equipslot", "levelreq", "itemcost", "maxstack", "weapondamage", "AC"},
		Search: []string{"itemname", "itemdesc"},
		Cols: map[string]colNote{
			"patternid":    {Label: "Item", ReadOnly: true},
			"itemname":     {Label: "Name", Section: "Identity"},
			"itemdesc":     {Label: "Description", Format: "multiline", Section: "Identity"},
			"itemcost":     {Label: "Value (tunar)", Section: "Identity"},
			"itemicon":     {Label: "Icon", Format: hex32, Section: "Look"},
			"model":        {Label: "Model", Format: hex32, Section: "Look"},
			"color":        {Label: "Colour", Format: hex32, Section: "Look"},
			"equipslot":    {Label: "Worn in", Enum: "wearslot", Section: "Use"},
			"levelreq":     {Label: "Level required", Section: "Use"},
			"classuse":     {Label: "Classes", Flags: "classmask", Section: "Use"},
			"raceuse":      {Label: "Races", Flags: "racemask", Section: "Use"},
			"maxstack":     {Label: "Stacks to", Section: "Use"},
			"maxhp":        {Label: "Condition (max)", Section: "Use"},
			"duration":     {Section: "Use"},
			"trade":        {Label: "No trade", Help: "1: it can never be traded (the client's m_noTrade: its info says NO TRADE)", Section: "Use"},
			"rent":         {Section: "Use"},
			"lore":         {Label: "Lore", Section: "Use"},
			"craft":        {Section: "Use"},
			"attacktype":   {Label: "Attack type", Section: "Combat"},
			"weapondamage": {Label: "Weapon DMG", Section: "Combat"},
			"procanim":     {Label: "Proc animation", Section: "Combat"},
			"weaponproc":   {Label: "Proc", Section: "Combat"},
			"str":          {Label: "STR", Section: "Stats"},
			"sta":          {Label: "STA", Section: "Stats"},
			"agi":          {Label: "AGI", Section: "Stats"},
			"dex":          {Label: "DEX", Section: "Stats"},
			"wis":          {Label: "WIS", Section: "Stats"},
			"intelligence": {Label: "INT", Section: "Stats"},
			"cha":          {Label: "CHA", Section: "Stats"},
			"HPMAX":        {Label: "Max HP", Section: "Stats"},
			"POWMAX":       {Label: "Max power", Section: "Stats"},
			"PoT":          {Label: "Power over time", Section: "Stats"},
			"HoT":          {Label: "Health over time", Section: "Stats"},
			"AC":           {Label: "AC", Section: "Stats"},
			"PR":           {Label: "Poison resist", Section: "Resists"},
			"DR":           {Label: "Disease resist", Section: "Resists"},
			"FR":           {Label: "Fire resist", Section: "Resists"},
			"CR":           {Label: "Cold resist", Section: "Resists"},
			"LR":           {Label: "Lightning resist", Section: "Resists"},
			"AR":           {Label: "Arcane resist", Section: "Resists"},
			"fish":         {Label: "Fishing", Section: "Stats"},
		},
	},
	"spellpattern": {
		Label: "Spells", Group: "Items & spells", Title: "spellname", Sort: "spellid",
		Help: "Every spell and ability, by spellid: level, range, cast and recast times (seconds: the server casts with CastTime × 1 s), power cost, icon. What a spell " +
			"DOES is its spelleffect line.",
		Live:   "Read at each cast and scribe; a player's spell book takes it at their next login.",
		Show:   []string{"spellid", "spellname", "abilitylvl", "casttime", "recast", "power", "range"},
		Search: []string{"spellname", "spelldesc"},
		Cols: map[string]colNote{
			"spellid":    {Label: "Spell", ReadOnly: true},
			"spellname":  {Label: "Name"},
			"spelldesc":  {Label: "Description", Format: "multiline"},
			"abilitylvl": {Label: "Level"},
			"casttime":   {Label: "Cast time (s)"},
			"recast":     {Label: "Recast (s)"},
			"power":      {Label: "Power cost"},
			"icon":       {Format: hex32},
			"iconColor":  {Label: "Icon colour", Format: hex32},
		},
	},
	"spelleffect": {
		Label: "Spell effects", Group: "Items & spells", Title: "line", Sort: "spellid",
		Help: "What each spell does, as EQOA-Data's spell lists word it (\"Inflicts 45 magic damage\", \"Increases STR by 10 " +
			"for 5 minutes\"); entity/spelltext.go reads the words. fxdict is the visual (0: the server's stand-in of its " +
			"kind): play_visual plays one in the game, Spell visuals lists the disc's, and #spellfx sets one in the game.",
		Live:   "At once: read at every cast.",
		Search: []string{"line"},
		Cols: map[string]colNote{
			"spellid": {Label: "Spell", Ref: "spellpattern"},
			"line":    {Label: "What it does", Format: "multiline"},
			"fxdict":  {Label: "Visual", Format: hex32},
		},
	},
	"spellarea": {
		Label: "Spell areas", Group: "Items & spells", Title: "spellid", Sort: "spellid",
		Help: "A spell's area, where its line's reading is not what you want (entity/area.go). A spell's line makes it " +
			"an area spell: \"Target, Decrease Hitpoints by 210 aoe Decrease Hitpoints by 90\" (the target takes 210, every " +
			"creature round it 90), a bare \"AOE\" (the shouts: all of it on all), a group heal's \"Radius 15\". A harmful " +
			"one lands on creatures only (never a player or a pet, nor across the water's edge; a spell for undead alone, " +
			"undead alone), a group's on the caster's group and their pets. centre: as its line reads, round its target, " +
			"round its caster, or not an area spell at all; radius how far (0: its line's, else 15 round a target and 10 " +
			"round a harmful spell's caster); maxtargets the most it lands on, its own target among them (0: 8; the cast's " +
			"visual names 7 others at most, all the client holds).",
		Live:   "At once: read at every cast.",
		Search: []string{"note"},
		Cols: map[string]colNote{
			"spellid":    {Label: "Spell", Ref: "spellpattern"},
			"centre":     {Label: "Centre", Enum: "areacentre", Help: "as its line reads, round its target, round its caster, or not an area spell"},
			"radius":     {Label: "Radius", Help: "how far round its centre; 0: its line's, else its kind's (15, or 10 round a harmful spell's caster)"},
			"maxtargets": {Label: "Most targets", Help: "the most it lands on, its own target among them; 0: 8"},
			"note":       {Label: "Note", Help: "why"},
		},
	},
	"fishcatch": {
		Label: "Fishing catches", Group: "Items & spells", Title: "itemid", Sort: "id",
		Help: "What a cast brings up (handlers/fishing.go): a row an item, picked by weight among the rows that apply " +
			"where the line went in (ten ahead of the angler) and when. world: -1 every world; the box minx..maxx, " +
			"minz..maxz (all four, or none: anywhere in the world); minskill the angler's fishing skill it needs " +
			"(characters.fishing, 0 to 100, plus the worn items' Fish stat; a catch may teach a point); months a bit a " +
			"month, January 1, February 2, March 4 ... December 2048 (0 the whole year); count how many of it come up at " +
			"once (one of an item that does not stack). With no row that applies a cast says nothing bites and keeps " +
			"the bait. A fishing session starts when a player uses a fishing pole (wear slot Fishing pole) from a HUD " +
			"quick slot, uses a gear set holding one, or says /fish; casting needs bait (wear slot Bait) worn. Rows of " +
			"source seed are tools/seed_fishing.py's: the catches of April 2003 (Allakhazam), the quest fish where " +
			"EQOA-Data's guides fish for them; the weights, skills and seasons are ours.",
		Live:   "At once: read at every cast.",
		Search: []string{"note", "source"},
		Cols: map[string]colNote{
			"itemid":   {Label: "Item", Ref: "itempattern"},
			"count":    {Label: "How many", Help: "at once; one of an item that does not stack"},
			"weight":   {Label: "Weight", Help: "its share among the rows that apply; 0 never"},
			"world":    {Label: "World", Help: "-1 every world, else the world's number (0 Tunaria, 2 Odus...)"},
			"minx":     {Label: "West edge (x)", Section: "Box"},
			"minz":     {Label: "South edge (z)", Section: "Box"},
			"maxx":     {Label: "East edge (x)", Section: "Box"},
			"maxz":     {Label: "North edge (z)", Section: "Box"},
			"minskill": {Label: "Skill", Help: "the fishing skill it needs, 0 to 100"},
			"months":   {Label: "Months", Help: "a bit a month: January 1 ... December 2048; 0 the whole year"},
			"note":     {Label: "Note", Help: "why"},
			"source":   {Label: "Source", Help: "gm, or seed for tools/seed_fishing.py's rows (its --undo deletes those)"},
		},
	},
	"craftrecipes": {
		Label: "Tradeskill recipes", Group: "Items & spells", Title: "name", Sort: "id",
		Help: "Every recipe (handlers/craft.go): id is the number the client echoes; skill the craft; comp the Recipe " +
			"Book's material tab, an index into the skill's list (Weapon and Armor, 11: Crude Iron, Iron, Bronze, Brass, " +
			"Steel, Combine Steel, Adamantite, Mithril, Brellium, Seralite, Special; Tailoring, 19: Fur, Cotton, Wool, " +
			"Tattered Hide, Hemp, Hide, Ramie, Suede, Canvas, Leather, Linen, Analine, Silk, Nubuck, Elven Twill, Vale " +
			"Leather, Mithril Silk, Stone Hide, Special; Jewel, 11: Iron, Bronze, Steel, Copper, Silver, Electrum, Gold, " +
			"Platinum, Adamantite, Mithril, Special; Carpentry, 11: Birch ... Ironwood, Special; Alchemy, 13: Salt Water ... " +
			"Liquified Mana, Special); level the trade level 1-10 the skill must reach, and the tier the held tool must " +
			"have (its level / 5 + 1: a level 1 tool tier 1, level 46 tier 10); rarity the book's column; name the book's " +
			"(63 characters at most); product and yield what a success makes; ingredients a JSON list " +
			"[{\"item\": itempattern id, \"count\": n, \"keep\": false}], keep true when a failure keeps it (a gem, an " +
			"alloy), false when a failure loses it (the metal, the mold: such an item is lost in an Experiment that makes " +
			"nothing too); difficulty the skill points the roll opposes (a skill's points are 10 a level and 1 a rank: " +
			"0.60 at the difficulty, 0.05 a point either side, plus the craft's two stats over 120 at 1/2000 a point, " +
			"5% to 95%); xp what a success gives at the recipe's own level (half one level below the skill's, none two " +
			"below, ten times for a discovery); discover 1 when an Experiment of exactly its ingredients finds it (a " +
			"recipe in the book is made by one either way); scroll the item whose use from the inventory puts it in the " +
			"book (0 none). Carpentry and Alchemy have no Recipe Book on the disc: an Experiment alone reaches them, and " +
			"only for a character a GM gave the skill (#craftskill). The numbers are ours, not SOE's.",
		Live:   "At once: the next book, inspect, create or experiment reads it.",
		Show:   []string{"id", "name", "skill", "comp", "level", "rarity", "product", "yield", "difficulty", "xp", "discover", "scroll", "source"},
		Search: []string{"name", "description", "ingredients", "source"},
		Cols: map[string]colNote{
			"id":          {Label: "Recipe", Help: "the number the client sends back"},
			"skill":       {Label: "Skill", Enum: "craftskill"},
			"comp":        {Label: "Book tab", Help: "the index in the skill's component list (the Help lists them); outside it the book never shows the recipe"},
			"level":       {Label: "Trade level", Help: "1-10: the skill's level it needs, and the held tool's tier"},
			"rarity":      {Label: "Rarity", Enum: "craftrarity"},
			"name":        {Label: "Name", Help: "the book's, 63 characters at most"},
			"product":     {Label: "Makes", Ref: "itempattern"},
			"yield":       {Label: "How many", Help: "of the product a success makes; a stacking one within its stack"},
			"ingredients": {Label: "Ingredients", Format: "json", Help: "[{\"item\": id, \"count\": n, \"keep\": false}]: keep true survives a failure"},
			"difficulty":  {Label: "Difficulty", Help: "the skill points the roll opposes (10 a level, 1 a rank)"},
			"xp":          {Label: "XP", Help: "a success at the recipe's own level; half one level below, none two; a discovery ten times"},
			"discover":    {Label: "An Experiment finds it", Enum: "bool"},
			"scroll":      {Label: "Taught by", Ref: "itempattern", Help: "the scroll item that puts it in the book when used; 0 none"},
			"description": {Label: "Description", Help: "shown on Inspect when the product has none of its own (255 characters at most)"},
			"source":      {Label: "Source", Help: "gm, or the tool that wrote it"},
		},
	},
	"charcraftskills": {
		Label: "Character tradeskills", Group: "Players", Sort: "charid",
		Help: "A character's tradeskill: level 1-10, rank 0-9 and the XP into the rank (100 x the level makes a rank, ten " +
			"ranks the next level; level 10 rank 9 is the top). With no row: Weapon, Armor, Tailoring and Jewel Crafting " +
			"at level 1 rank 0, Carpentry and Alchemy not live (sent as Sandstorm sends them, 99 and 99, and refusing " +
			"every craft) until a row gives them.",
		Live: "At once: a character in the world is sent its six skills (its Recipe Book's title); else at its next login.",
		Cols: map[string]colNote{
			"charid": {Label: "Character", Ref: "characters"},
			"skill":  {Label: "Skill", Enum: "craftskill"},
			"level":  {Label: "Level", Help: "1-10"},
			"rank":   {Label: "Rank", Help: "0-9"},
			"xp":     {Label: "XP into the rank", Help: "100 x the level makes the next rank"},
		},
	},
	"charrecipes": {
		Label: "Character recipe books", Group: "Players", Sort: "charid",
		Help: "The recipes in a character's Recipe Book (learned by an Experiment's discovery, a scroll or a GM's " +
			"#craftlearn); ord is the book's order, which the client's Arrange changes.",
		Live: "At once: the next time the character opens a Recipe Book.",
		Cols: map[string]colNote{
			"charid": {Label: "Character", Ref: "characters"},
			"recipe": {Label: "Recipe", Ref: "craftrecipes"},
			"ord":    {Label: "Order", Help: "the book's order, lowest first"},
		},
	},
	"gmtriggers": {
		Label: "Triggers", Group: "World", Title: "name", Sort: "slot",
		Help: "The GMs' scripted events (handlers/gmtriggers.go; edit them on the Triggers page): a row a trigger. " +
			"Slots 1 to 8 are the lines of the GM's TEST > Programming > TRIGGER menu, top to bottom (a PS2 shows eight " +
			"TRIGGERs, vanilla each one's name); a slot past 8 is fired by #trigger or the panel alone. A firing is a " +
			"run: its steps in order at its place (world -1: where the GM who fires it stands; else world, x, y, z), " +
			"each step that reaches players reaching those its scope takes there as the step runs (near: within " +
			"radius, 0 being 100; gm: the GM alone; world: its world; all: everyone). steps is a JSON list: " +
			triggerStepsHelp + ". A trigger with a problem (trigger_get lists them) cannot fire; enabled 0 keeps it " +
			"from firing.",
		Live:   "At once: the next firing runs it, and vanilla's TEST menu shows its name.",
		Show:   []string{"slot", "name", "enabled", "scope", "radius", "world"},
		Search: []string{"name", "notes", "steps"},
		Cols: map[string]colNote{
			"slot":    {Label: "Slot", Help: "1 to 8: the TEST menu's TRIGGER line, top to bottom; past 8: #trigger and the panel alone"},
			"name":    {Label: "Name", Help: "vanilla's TEST menu shows it (32 bytes)"},
			"enabled": {Label: "On", Enum: "bool"},
			"scope":   {Label: "Reaches", Help: "near, gm, world or all"},
			"radius":  {Label: "Radius", Help: "near: how far from its place; 0: 100"},
			"world":   {Label: "World", Help: "-1: where the GM who fires it stands; else its own place's world (0 Tunaria, 2 Odus...)"},
			"x":       {Label: "X", Section: "Its own place"},
			"y":       {Label: "Y (height)", Section: "Its own place"},
			"z":       {Label: "Z", Section: "Its own place"},
			"steps":   {Label: "Steps", Help: "a JSON list of steps; the Triggers page edits them"},
			"notes":   {Label: "Notes", Help: "for GMs"},
		},
	},
	"spellgates": {
		Label: "Spell gates", Group: "Items & spells", Title: "spell", Sort: "spell",
		Help: "Where the spells that teleport send, and whom (entity/gates.go): the wizard's gates, portals and evacuations " +
			"and the druid's circles, a row a spell by its name in lower case. kind: self sends the caster; group the caster " +
			"and the members of its group within radius of it as it lands (0: 20, the AE radius Allakhazam's 2004 records " +
			"give); evac that group to the caster's bind point (where Return Home takes it), world/x/y/z unused (make it " +
			"group to land it at its place). The landing is open, dry ground near x/y/z, clear of creatures, when the " +
			"server has the ground files, else x/y/z as written; the group lands round the caster. A spell whose line " +
			"teleports with no row, or whose world the caster's game cannot load, is refused before its power is spent; " +
			"a line saying \"Set Maximum Power to 0\" takes the caster's power when it lands. Rows of source seed are " +
			"tools/seed_spell_gates.py's: the places EQOA-Data's lines name, on CLIENT's zone grid and EQOA-Data's map, ours " +
			"where no source says the spot.",
		Live:   "At once: the next cast of the spell uses it.",
		Search: []string{"spell", "place", "note"},
		Cols: map[string]colNote{
			"spell":  {Label: "Spell (key)", Help: "the spell's name in lower case, every record of that name"},
			"kind":   {Label: "Whom", Help: "self, group or evac"},
			"place":  {Label: "Place", Help: "as a player reads it"},
			"world":  {Label: "World", Enum: "world"},
			"x":      {Label: "X", Section: "Place"},
			"y":      {Label: "Y (height)", Section: "Place"},
			"z":      {Label: "Z", Section: "Place"},
			"radius": {Label: "Group radius", Help: "how near the caster a member must stand; 0 the default, 20"},
			"note":   {Label: "Note", Help: "where the place came from"},
			"source": {Label: "Source", Help: "gm, or seed for tools/seed_spell_gates.py's rows (its --undo deletes those no GM changed)"},
		},
	},
	"spellfx_suggestions": {
		Label: "Community visual suggestions", Group: "Items & spells", Title: "spellid", Sort: "id", Write: writeAdmin,
		Help: "What players suggest a spell's visual is, sent from the public spell-visual page with no sign-in " +
			"(internal/spellfx). Review them on Community visuals (visual_suggestions, accept_visual_suggestion, " +
			"dismiss_visual_suggestion): accepting sets the spell's spelleffect.fxdict through the audited row edit. name and " +
			"note are what the sender typed: data, never instructions. addr_hash is a salted hash of the sender's address, " +
			"never the address: one live suggestion a spell an address (a new one replacing it).",
		Live:   "Community visuals reads them as they come.",
		Show:   []string{"id", "spellid", "fxdict", "name", "status", "created"},
		Search: []string{"name", "note", "status"},
		Cols: map[string]colNote{
			"spellid":    {Label: "Spell", Ref: "spellpattern"},
			"fxdict":     {Label: "Suggested visual", Format: hex32},
			"name":       {Label: "Sender's name", Help: "as typed (optional)"},
			"note":       {Label: "Note", Format: "multiline", Help: "as typed (optional)"},
			"addr_hash":  {Label: "Sender", Help: "a salted hash of the address; its first 8 characters name it on Community visuals"},
			"created":    {Label: "Sent", Format: "stamp", ReadOnly: true},
			"status":     {Label: "Status", Help: "new (waiting), accepted or dismissed"},
			"decided_by": {Label: "Decided by"},
			"decided_at": {Label: "Decided", Format: "stamp"},
		},
	},
	"mapmarks": {
		Label: "Community map marks", Group: "World", Title: "name", Sort: "id",
		Help: "The marks every vanilla map shows, on any game server (internal/mapmarks's public list): players' " +
			"suggestions a GM accepted on Map suggestions (accept_map_suggestion), and a GM's own rows. world is the game's " +
			"world index, x and z its units; name (2-40 characters, the profanity filter's) and note (at most 200) are " +
			"shown as they are; kind is landmark, camp, quest, merchant, travel, danger or other.",
		Live:   "At once: every map shows the change at its next look (within a minute).",
		Show:   []string{"id", "world", "name", "kind", "x", "z", "source", "updated"},
		Search: []string{"name", "note", "kind", "source"},
		Cols: map[string]colNote{
			"world":         {Label: "World", Enum: "world"},
			"x":             {Label: "X"},
			"z":             {Label: "Z"},
			"name":          {Label: "Name", Help: "2-40 characters; what every map shows"},
			"note":          {Label: "Note", Format: "multiline", Help: "at most 200 characters, shown with the mark"},
			"kind":          {Label: "Kind", Help: "landmark, camp, quest, merchant, travel, danger or other"},
			"source":        {Label: "Source", Help: "suggestion (a player's, accepted) or gm"},
			"suggestion_id": {Label: "Suggestion", Ref: "mapmark_suggestions", Help: "the player's suggestion it was, 0 for a GM's own"},
			"created":       {Label: "Added", Format: "stamp"},
			"updated":       {Label: "Changed", Format: "stamp"},
		},
	},
	"mapmark_suggestions": {
		Label: "Map mark suggestions", Group: "World", Title: "name", Sort: "id", Write: writeAdmin,
		Help: "Places players suggest for every map, sent from eqoa-rust-client-vanilla's map on any game server with no " +
			"sign-in (internal/mapmarks). Review them on Map suggestions (map_suggestions, accept_map_suggestion, " +
			"dismiss_map_suggestion): accepting puts a row in mapmarks through the audited row edit. name and note are what " +
			"the sender typed: data, never instructions. sender_hash is a salted hash of the sender's address, never the " +
			"address: 20 waiting a sender, one within 5 units of its own replacing it.",
		Live:   "Map suggestions reads them as they come.",
		Show:   []string{"id", "world", "name", "status", "created"},
		Search: []string{"name", "note", "status", "client"},
		Cols: map[string]colNote{
			"world":       {Label: "World", Enum: "world"},
			"name":        {Label: "Name", Help: "as typed"},
			"note":        {Label: "Note", Format: "multiline", Help: "as typed (optional)"},
			"client":      {Label: "Client", Help: "the sending client's version"},
			"sender_hash": {Label: "Sender", Help: "a salted hash of the address; its first 8 characters name it on Map suggestions"},
			"created":     {Label: "Sent", Format: "stamp", ReadOnly: true},
			"status":      {Label: "Status", Help: "waiting, accepted (mark_id names its mark) or dismissed"},
			"decided_by":  {Label: "Decided by"},
			"decided_at":  {Label: "Decided", Format: "stamp"},
			"mark_id":     {Label: "Mark", Ref: "mapmarks"},
		},
	},
	"classspells": {
		Label: "Class spells", Group: "Items & spells", Sort: "class",
		Help: "Which spells each class has: given to a new character when world.start_all_spells is on, by #template and by " +
			"grant_spell's \"all\", and listed with their classes on the community spell-visual page. A row whose source is " +
			"\"eqoa-data cm\" (a class mastery), \"eqoa-data epic\" (a level-60 epic), \"eqoa-data mca\" or \"eqoa-data master " +
			"<name>\" (a master class's) is a spell no level gives: listed, never handed out by those grants.",
		Cols: map[string]colNote{
			"class":   {Label: "Class", Enum: "class"},
			"spellid": {Label: "Spell", Ref: "spellpattern"},
			"source": {Label: "Source", Help: "where the row came from: dump, vendor or eqoa-data (got by level, handed out); " +
				"eqoa-data cm, eqoa-data epic, eqoa-data mca, eqoa-data master <name> (not got by level: never handed out)"},
		},
	},
	"defaultcharacter": {
		Label: "Starting templates", Group: "New characters", Sort: "race",
		Help: "One row per race, home city and class a new character may be: its starting stats, money and place.",
		Live: "At once, for the next character made.",
		Show: []string{"defaultCharacterID", "race", "humType", "tclass", "level", "world", "x", "y", "z"},
		Cols: map[string]colNote{
			"tclass":  {Label: "Class", Enum: "class"},
			"race":    {Label: "Race", Enum: "race"},
			"humType": {Label: "Home city"},
			"world":   {Label: "World", Enum: "world"},
		},
	},
	"defaultcharinventory": {
		Label: "Starting items (unused)", Group: "New characters",
		Help: "Kept from the C# server's schema; this server does not read it.",
		Cols: map[string]colNote{"patternid": {Label: "Item", Ref: "itempattern"}},
	},
	"startitems": {
		Label: "Starting items", Group: "New characters", Title: "note", Sort: "race",
		Help: "What a new character carries, by race, class and (a human's) city: Sandstorm's, a starting weapon " +
			"(an Apprentice Sword, Club, Dagger or Staff) and twenty of the race's rations, none worn " +
			"(tools/seed_start_items.py, from a new character of every combo on Sandstorm). A froglok carries an " +
			"eastern human's. count is the stack.",
		Live: "At once: every character made from now on.",
		Cols: map[string]colNote{
			"race":      {Label: "Race", Enum: "race"},
			"class":     {Label: "Class", Enum: "class"},
			"humtype":   {Label: "Human's city", Help: "1 the east (Freeport), 2 the west (Qeynos); 0 for every other race"},
			"patternid": {Label: "Item", Ref: "itempattern"},
			"count":     {Label: "How many"},
			"source":    {Label: "Source", Help: "gm, or sandstorm for tools/seed_start_items.py's rows"},
		},
	},
	"defaultspells": {
		Label: "Starting spells (unused)", Group: "New characters",
		Help: "Kept from the C# server's schema; this server does not read it (classspells is what it gives).",
		Cols: map[string]colNote{"spellid": {Label: "Spell", Ref: "spellpattern"}},
	},
	"charactermodel": {
		Label: "Body models", Group: "New characters",
		Help: "The body model (CHAR.ESF DictID) for each race and sex.",
		Cols: map[string]colNote{
			"modelid": {Label: "Model", Format: hex32},
			"race":    {Label: "Race", Enum: "race"},
			"sex":     {Label: "Sex", Enum: "sex"},
		},
	},
	"guilds": {
		Label: "Guilds", Group: "Guilds & quests", Title: "name",
		Cols: map[string]colNote{"leader_id": {Label: "Leader", Ref: "characters"}},
	},
	"guild_members": {
		Label: "Guild members", Group: "Guilds & quests",
		Cols: map[string]colNote{
			"guild_id":  {Label: "Guild", Ref: "guilds"},
			"player_id": {Label: "Character", Ref: "characters"},
		},
	},
	"quests": {
		Label: "Quests", Group: "Guilds & quests", Title: "name", Sort: "questid",
		Help: "Each quest: who offers it (giver, an NPC's name), to whom (levels, classes, races, quests finished first), " +
			"what the giver says (offer, a line a box) and the answers that take it or refuse it. enabled 0 is a draft only " +
			"this panel sees. Its steps are queststeps; the Quests page edits both together.",
		Live:   "At once: the next talk, kill or step uses it.",
		Show:   []string{"questid", "name", "enabled", "giver", "minlevel", "maxlevel", "source"},
		Search: []string{"name", "giver", "notes"},
		Cols: map[string]colNote{
			"questid":    {Label: "Quest"},
			"name":       {Label: "Name"},
			"enabled":    {Label: "In the game", Help: "1: players get it; 0: a draft"},
			"giver":      {Label: "Giver", Help: "the NPC who offers it, by its name as the game shows it"},
			"minlevel":   {Label: "From level"},
			"maxlevel":   {Label: "To level"},
			"classes":    {Label: "Classes", Help: "class ids, comma separated; empty: any"},
			"races":      {Label: "Races", Help: "race ids, comma separated; empty: any"},
			"requires":   {Label: "Quests first", Help: "quest ids finished first, comma separated"},
			"repeatable": {Label: "Repeatable"},
			"offer":      {Label: "Giver offers", Format: "multiline", Help: "a paragraph a question: a line a box, its last line asks"},
			"accept":     {Label: "Answers taking it", Format: "multiline", Help: "a line a question; empty: taken when talked to"},
			"decline":    {Label: "Answer refusing it", Help: "empty: none (Close refuses)"},
			"declined":   {Label: "Giver replies to a refusal", Format: "multiline", Help: "said as written"},
			"started":    {Label: "Giver says once taken", Format: "multiline"},
			"source":     {Label: "Source"},
			"notes":      {Label: "Notes", Format: "multiline"},
			"events":     {Label: "Events", Format: "multiline", Help: "a JSON list: on accept or finish, do popup, say, give, spawn, visual or teleport (the Quests page edits them)"},
			"factionreq": {Label: "Standings asked", Help: "Faction Name:min or Faction Name:min..max, comma separated (the factions table's names): offered, and done at its giver, only while each is met; empty: none"},
		},
	},
	"questunlocks": {
		Label: "Quest unlocks", Group: "Guilds & quests", Sort: "questid",
		Help: "The trips quests unlock: a travel NPC (npc, a waysender or a portal, by its name as the game shows it) " +
			"offers its trip to dest (the route's label in the server's waysender.go) only to a player who has finished " +
			"one of the quests in the game that unlock it. The Quests page edits a quest's with it.",
		Live:   "At once: the next talk to that NPC uses it.",
		Show:   []string{"questid", "npc", "dest"},
		Search: []string{"npc", "dest"},
		Cols: map[string]colNote{
			"questid": {Label: "Quest", Ref: "quests"},
			"npc":     {Label: "Travel NPC", Help: "its name as the game shows it"},
			"dest":    {Label: "Trip to", Help: "the route's label: case and a leading \"the\" aside"},
		},
	},
	"queststeps": {
		Label: "Quest steps", Group: "Guilds & quests", Sort: "questid",
		Help: "A quest's steps in order (step 1, 2, ...). kind talk: talk to the NPC named target (answering it when answer " +
			"is set); show: talk to it holding items, which are kept; give: hand that NPC items (patternid*count, comma " +
			"separated) and tunar, which are taken; kill: kill count creatures " +
			"named target; loot: kill creatures named target until count of the item in items are taken off their corpses " +
			"during the step, each kill dropping one on the corpse with chance for a player on the step; reach: come within radius of x, y, z in world. journal is " +
			"the journal's line while it is the step to do; says is the NPC's words when it is done; the rewards are given " +
			"then; events are what happens when it becomes the step to do (start) and when it is done (done).",
		Live:   "At once: the next talk, kill or step uses it.",
		Show:   []string{"questid", "step", "kind", "target", "journal"},
		Search: []string{"target", "journal", "says"},
		Cols: map[string]colNote{
			"questid":     {Label: "Quest", Ref: "quests"},
			"step":        {Label: "Step"},
			"kind":        {Label: "Kind", Help: "talk, show, give, kill, loot or reach"},
			"target":      {Label: "NPC or creature", Help: "its name as the game shows it"},
			"npcid":       {Label: "Only spawn", Ref: "npcs", Help: "0: any of the name"},
			"items":       {Label: "Items held or handed in", Help: "show: held and kept; give: taken. patternid*count, comma separated"},
			"tunar":       {Label: "Tunar held or handed in"},
			"count":       {Label: "How many", Help: "a kill step's creatures, a loot step's items"},
			"chance":      {Label: "Drop chance", Help: "a loot step's: 0 to 1 a kill; 0 every kill"},
			"world":       {Label: "World", Enum: "world"},
			"radius":      {Label: "Within"},
			"journal":     {Label: "Journal line", Help: "at most 127 characters"},
			"answer":      {Label: "Player's answer", Format: "multiline", Help: "the first line does it, any others change nothing; empty: done on talking"},
			"notyet":      {Label: "Answer while items lack", Format: "multiline", Help: "show/give: offered instead of the answer until the items are held"},
			"reminder":    {Label: "NPC says while items lack", Format: "multiline"},
			"says":        {Label: "NPC says when done", Format: "multiline"},
			"waiting":     {Label: "NPC says before", Format: "multiline"},
			"xp":          {Label: "Reward: experience"},
			"rewardtunar": {Label: "Reward: tunar"},
			"rewarditems": {Label: "Reward: items", Help: "patternid*count, comma separated"},
			"factions":    {Label: "Reward: factions", Help: "Faction:+10, comma separated"},
			"spells":      {Label: "Reward: spells", Help: "spell ids, comma separated"},
			"events":      {Label: "Events", Format: "multiline", Help: "a JSON list: on start or done, do popup, say, give, spawn, visual or teleport (the Quests page edits them)"},
			"byclass":     {Label: "Reward: by class", Format: "multiline", Help: `a JSON object by class id, each {"xp", "tunar", "items": "patternid*count, ...", "spells": "spell ids"}, given besides the above to a player of that class`},
			"choices":     {Label: "Choice", Format: "multiline", Help: `talk/show/give: {"ask", "options": [{"answer", "says", "stay", "xp", "tunar", "items", "spells"}]}, asked when the step is done; the pick's rewards given besides the step's, one with stay does not do it`},
		},
	},
	"npc_models_observed": {Label: "Observed NPC models", Group: "Imports", Write: writeAdmin,
		Cols: map[string]colNote{"modelid": {Format: hex32}, "world": {Enum: "world"}}},
	"npcs_ff_import": {Label: "FrontiersForge NPC import", Group: "Imports", Write: writeAdmin,
		Cols: map[string]colNote{"modelid": {Format: hex32}, "world": {Enum: "world"}}},
	"prh_import_ids":        {Label: "Return Home import: ids", Group: "Imports", Write: writeAdmin},
	"prh_import_keyed":      {Label: "Return Home import: keyed", Group: "Imports", Write: writeAdmin},
	"prh_import_meta":       {Label: "Return Home import: meta", Group: "Imports", Write: writeAdmin},
	"prh_import_npcs":       {Label: "Return Home import: NPCs", Group: "Imports", Write: writeAdmin},
	"sandstorm_import_meta": {Label: "Sandstorm import: meta", Group: "Imports", Write: writeAdmin},
	"sandstorm_import_rows": {Label: "Sandstorm import: rows", Group: "Imports", Write: writeAdmin},
	"gm_audit": {
		Label: "GM audit log", Group: "Panel", Write: writeNone, Sort: "id",
		Help: "Every change made through the panel or by an agent, with the row before and after (audit_log reads it).",
		Cols: map[string]colNote{"before": {Format: "json"}, "after": {Format: "json"}, "rowkey": {Format: "json"}},
	},
	"gm_tokens": {
		Label: "API tokens", Group: "Panel", Write: writeNone,
		Help: "The GMs' API and OAuth tokens (hashes only); managed on the Tokens page.",
		Cols: map[string]colNote{"hash": {Secret: true}, "accountid": {Ref: "accountinfo"}},
	},
	"gm_oauth_clients": {Label: "OAuth clients", Group: "Panel", Write: writeAdmin},
	"gm_totp":          {Hidden: true},
	// the salt the community suggestions' senders are hashed with: with it an
	// IPv4 sender's hash can be walked back to the address, so no page shows it
	"spellfx_meta": {Hidden: true},
	// the map marks' senders' salt, hidden for the same reason
	"mapmark_meta":    {Hidden: true},
	"sqlite_sequence": {Hidden: true},
	"sqlite_stat1":    {Hidden: true},
}

// applyCatalog lays the catalog over a table read from the schema.
func applyCatalog(t *Table) {
	n, ok := catalog[t.Name]
	if !ok {
		t.Label = strings.ReplaceAll(t.Name, "_", " ")
		t.Group = "Other"
		t.Write = writeAdmin // a table the panel knows nothing about: an admin's
		if strings.HasPrefix(t.Name, "sqlite_") {
			t.Hidden = true
		}
		return
	}
	t.Label, t.Group, t.Help, t.Title, t.Sort, t.Live = n.Label, n.Group, n.Help, n.Title, n.Sort, n.Live
	t.Show, t.Search, t.Hidden = n.Show, n.Search, n.Hidden
	t.Write = n.Write
	if t.Write == "" {
		t.Write = writeGM
	}
	if t.Label == "" {
		t.Label = t.Name
	}
	if t.Group == "" {
		t.Group = "Other"
	}
	for i := range t.Columns {
		c := &t.Columns[i]
		cn, ok := n.Cols[c.Name]
		if !ok {
			continue
		}
		c.Label, c.Help, c.Enum, c.Flags, c.Ref, c.Format, c.Section = cn.Label, cn.Help, cn.Enum, cn.Flags, cn.Ref, cn.Format, cn.Section
		c.Secret, c.ReadOnly = cn.Secret, cn.ReadOnly
	}
	// the default search: every text column
	if len(t.Search) == 0 {
		for _, c := range t.Columns {
			if c.Affinity == "TEXT" && !c.Secret {
				t.Search = append(t.Search, c.Name)
			}
		}
	}
}
