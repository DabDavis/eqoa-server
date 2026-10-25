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
		"classmask": classes, // itempattern.classuse: 32767 = every class
		"racemask":  races,   // itempattern.raceuse: 1023 = every race
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
		Help: "A character's standing with each faction it has moved off the faction's starting one (-2000 to 2000); " +
			"with no row it has the starting standing (Factions).",
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
	"playerquest": {
		Label: "Character quests", Group: "Guilds & quests",
		Cols: map[string]colNote{
			"serverID": {Label: "Character", Ref: "characters"},
			"questID":  {Label: "Quest", Ref: "quests"},
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
	"npcitem": {
		Label: "Merchant stock", Group: "World", Sort: "npcid",
		Help: "What a merchant sells: a row an item, by npcs.id; listnumber orders the window.",
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
		Help: "Every faction and the standing a character starts with (-2000 to 2000). Filled from an eqoa.live capture's " +
			"331 (one gnome's: Klick`Anon 300, Qeynos -2000) when empty; a character's own standings are Character factions.",
		Live: "At once: every faction window and the next kill read it.",
		Cols: map[string]colNote{
			"name":     {Label: "Faction"},
			"standing": {Label: "Starting standing", Help: "-2000 to 2000"},
		},
	},
	"factionhits": {
		Label: "Faction hits", Group: "World", Title: "npc_name", Sort: "npc_name",
		Help: "What killing an NPC does to its killer's standings: each row adds delta to the standing with one faction " +
			"for a kill of any NPC of that NAME (a #spawn copy too). The killer alone, as the kill's XP. Town rows " +
			"(source town-seed) are tools/seed_faction_hits.py's guesses, not measured: no capture holds a faction hit.",
		Live:   "At once: the next kill of that NPC uses it.",
		Search: []string{"npc_name", "faction", "source"},
		Cols: map[string]colNote{
			"npc_name": {Label: "NPC name", Help: "as in NPCs, exactly (a trailing space included)"},
			"faction":  {Label: "Faction", Ref: "factions"},
			"delta":    {Label: "Change", Help: "added to the killer's standing; below 0 lowers it"},
			"source":   {Label: "Source", Help: "gm, or town-seed for the seeding tool's rows (its --undo deletes those)"},
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
			"trade":        {Label: "Tradeable", Section: "Use"},
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
		Help: "Every spell and ability, by spellid: level, range, cast and recast times (ms), power cost, icon. What a spell " +
			"DOES is its spelleffect line.",
		Live:   "Read at each cast and scribe; a player's spell book takes it at their next login.",
		Show:   []string{"spellid", "spellname", "abilitylvl", "casttime", "recast", "power", "range"},
		Search: []string{"spellname", "spelldesc"},
		Cols: map[string]colNote{
			"spellid":    {Label: "Spell", ReadOnly: true},
			"spellname":  {Label: "Name"},
			"spelldesc":  {Label: "Description", Format: "multiline"},
			"abilitylvl": {Label: "Level"},
			"casttime":   {Label: "Cast time (ms)"},
			"recast":     {Label: "Recast (ms)"},
			"power":      {Label: "Power cost"},
			"icon":       {Format: hex32},
			"iconColor":  {Label: "Icon colour", Format: hex32},
		},
	},
	"spelleffect": {
		Label: "Spell effects", Group: "Items & spells", Title: "line", Sort: "spellid",
		Help: "What each spell does, as EQOA-Data's spell lists word it (\"Inflicts 45 magic damage\", \"Increases STR by 10 " +
			"for 5 minutes\"); entity/spelltext.go reads the words. fxdict is the visual (0: the server's stand-in).",
		Live:   "At once: read at every cast.",
		Search: []string{"line"},
		Cols: map[string]colNote{
			"spellid": {Label: "Spell", Ref: "spellpattern"},
			"line":    {Label: "What it does", Format: "multiline"},
			"fxdict":  {Label: "Visual", Format: hex32},
		},
	},
	"classspells": {
		Label: "Class spells", Group: "Items & spells", Sort: "class",
		Help: "Which spells each class has: given to a new character when world.start_all_spells is on, and by grant_spell's \"all\".",
		Cols: map[string]colNote{
			"class":   {Label: "Class", Enum: "class"},
			"spellid": {Label: "Spell", Ref: "spellpattern"},
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
		Label: "Quests", Group: "Guilds & quests",
		Cols: map[string]colNote{"questLog": {Format: "multiline"}},
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
	"sqlite_sequence":  {Hidden: true},
	"sqlite_stat1":     {Hidden: true},
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
