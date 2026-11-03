"""Descriptions of our own for item records that have none (the user, 2026-11-03: "go ahead and add your own
description with eqoa flavor"), written the way SOE's own read: what the thing is and what it is made of, then
who made it, where it is from or what it has been through, then the magic it holds ("A magical gladius; it
holds a minor strength enchantment."). Everything here is OURS: no record says these things of these items.

describe(item) takes a dict of the record's figures (name, wear slot, look, level, attack type and the stats)
and returns one to three short sentences, at most MAX_LEN characters, the same every time for the same record
(the variants are picked by a hash of the name, not at random).
"""
import hashlib
import re

MAX_LEN = 150

# armour looks (itempattern.model in the armour slots) by what they are made of
MATERIAL = {1: "cloth", 2: "leather", 3: "chain", 4: "plate", 5: "banded", 6: "ringmail", 7: "scale", 8: "padded"}
ARMOUR = (1, 5, 7, 10, 11, 19)
# what each material's piece is made of (a phrase after the noun)
MADE_OF = {
    "cloth": ("woven from sturdy thread", "stitched from layered cloth", "of tightly woven cloth"),
    "leather": ("cut from cured hide", "stitched from tanned leather", "of thick, oiled leather"),
    "chain": ("of tightly linked rings", "forged of small iron rings", "of heavy chain links"),
    "scale": ("of overlapping metal scales", "covered in small scales of metal", "of riveted scales"),
    "plate": ("hammered from heavy steel", "of thick, polished plate", "forged of solid plate"),
    "banded": ("of riveted metal bands", "bound in bands of steel", "of overlapping metal bands"),
    "ringmail": ("of rings sewn onto leather", "of iron rings on a leather backing"),
    "padded": ("of quilted padding", "of thick quilted cloth"),
}
# a piece's own noun, from its name: (noun, plural?) — the last one in the name wins
NOUNS = {
    "tunic": ("tunic", False), "shirt": ("shirt", False), "vest": ("vest", False), "jerkin": ("jerkin", False),
    "coat": ("coat", False), "hauberk": ("hauberk", False), "breastplate": ("breastplate", False),
    "chestplate": ("chestplate", False), "chestguard": ("chestguard", False), "cuirass": ("cuirass", False),
    "mantle": ("mantle", False), "cloak": ("cloak", False), "robe": ("robe", False), "robes": ("robe", False),
    "gown": ("gown", False), "shroud": ("shroud", False), "vestment": ("vestment", False),
    "vestments": ("vestments", True), "tabard": ("tabard", False), "tabbard": ("tabard", False),
    "helm": ("helm", False), "helmet": ("helmet", False), "cap": ("cap", False), "hat": ("hat", False),
    "hood": ("hood", False), "cowl": ("cowl", False), "coif": ("coif", False), "circlet": ("circlet", False),
    "crown": ("crown", False), "headband": ("headband", False), "turban": ("turban", False),
    "headwrap": ("headwrap", False), "mask": ("mask", False), "visor": ("visor", False),
    "bracers": ("bracers", True), "bracer": ("bracer", False), "vambraces": ("vambraces", True),
    "wristguards": ("wristguards", True), "sleeves": ("sleeves", True), "armguards": ("armguards", True),
    "leggings": ("leggings", True), "greaves": ("greaves", True), "legplates": ("legplates", True),
    "pants": ("pants", True), "trousers": ("trousers", True), "pantaloons": ("pantaloons", True),
    "kilt": ("kilt", False), "skirt": ("skirt", False), "breeches": ("breeches", True),
    "boots": ("boots", True), "shoes": ("shoes", True), "sandals": ("sandals", True),
    "slippers": ("slippers", True), "treads": ("treads", True), "sabatons": ("sabatons", True),
    "gloves": ("gloves", True), "gauntlets": ("gauntlets", True), "guantlets": ("gauntlets", True),
    "gauntlet": ("gauntlets", True), "glove": ("gloves", True), "boot": ("boots", True),
    "earrings": ("earrings", True), "studs": ("studs", True),
    "mitts": ("mitts", True), "mittens": ("mittens", True), "wraps": ("wraps", True), "handwraps": ("handwraps", True),
    "ring": ("ring", False), "band": ("band", False), "loop": ("loop", False), "hoop": ("hoop", False),
    "earring": ("earring", False), "stud": ("stud", False),
    "necklace": ("necklace", False), "pendant": ("pendant", False), "amulet": ("amulet", False),
    "choker": ("choker", False), "collar": ("collar", False), "talisman": ("talisman", False),
    "medallion": ("medallion", False), "charm": ("charm", False), "scarf": ("scarf", False),
    "insignia": ("insignia", False), "icon": ("icon", False),
    "bracelet": ("bracelet", False), "bangle": ("bangle", False), "wristband": ("wristband", False),
    "armband": ("armband", False), "cuff": ("cuff", False),
    "belt": ("belt", False), "sash": ("sash", False), "girdle": ("girdle", False), "cord": ("cord", False),
    "sword": ("sword", False), "shortsword": ("short sword", False), "longsword": ("long sword", False),
    "greatsword": ("greatsword", False), "claymore": ("claymore", False), "scimitar": ("scimitar", False),
    "rapier": ("rapier", False), "katana": ("katana", False), "gladius": ("gladius", False),
    "blade": ("blade", False), "saber": ("saber", False), "sabre": ("saber", False),
    "axe": ("axe", False), "waraxe": ("war axe", False), "greataxe": ("greataxe", False),
    "handaxe": ("hand axe", False), "cutlass": ("cutlass", False), "foil": ("foil", False),
    "partizan": ("partisan", False), "partisan": ("partisan", False), "ranseur": ("ranseur", False),
    "spetum": ("spetum", False), "glaive": ("glaive", False), "zweihander": ("zweihander", False),
    "wakizashi": ("wakizashi", False), "katar": ("katar", False), "crook": ("crook", False),
    "crosier": ("crosier", False), "warhammer": ("warhammer", False), "longspear": ("long spear", False),
    "pitchfork": ("pitchfork", False), "branch": ("branch", False), "sticker": ("sticker", False),
    "crusher": ("crusher", False), "beheader": ("beheader", False), "needle": ("needle", False),
    "symbol": ("symbol", False), "censer": ("censer", False), "lantern": ("lantern", False),
    "broadsword": ("broadsword", False), "falchion": ("falchion", False), "sickle": ("sickle", False),
    "scythe": ("scythe", False), "whip": ("whip", False), "warspear": ("war spear", False),
    "beads": ("string of beads", False), "bead": ("bead", False), "pin": ("pin", False), "brooch": ("brooch", False),
    "hatchet": ("hatchet", False), "cleaver": ("cleaver", False), "chopper": ("chopper", False),
    "club": ("club", False), "cudgel": ("cudgel", False), "mace": ("mace", False), "hammer": ("hammer", False),
    "maul": ("maul", False), "flail": ("flail", False), "mattock": ("mattock", False), "morningstar": ("morning star", False),
    "stick": ("stick", False), "baton": ("baton", False), "bo": ("bo staff", False),
    "dagger": ("dagger", False), "dirk": ("dirk", False), "knife": ("knife", False), "kris": ("kris", False),
    "stiletto": ("stiletto", False), "shiv": ("shiv", False), "stinger": ("stinger", False),
    "spear": ("spear", False), "lance": ("lance", False), "pike": ("pike", False), "halberd": ("halberd", False),
    "trident": ("trident", False), "staff": ("staff", False), "quarterstaff": ("quarterstaff", False),
    "bow": ("bow", False), "longbow": ("longbow", False), "shortbow": ("shortbow", False),
    "crossbow": ("crossbow", False),
    "shield": ("shield", False), "buckler": ("buckler", False), "targe": ("targe", False),
    "wand": ("wand", False), "rod": ("rod", False), "scepter": ("scepter", False), "sceptre": ("scepter", False),
    "orb": ("orb", False), "sphere": ("sphere", False), "totem": ("totem", False), "fetish": ("fetish", False),
    "book": ("book", False), "tome": ("tome", False), "focus": ("focus", False), "foci": ("focus", False),
    "horn": ("horn", False), "stone": ("stone", False), "eye": ("eye", False), "idol": ("idol", False),
    "lute": ("lute", False), "drum": ("drum", False), "flute": ("flute", False), "mandolin": ("mandolin", False),
}
# the nouns of the drawn armour: any other noun in an armour slot (a ring, a necklace) is described as itself
ARMOUR_NOUNS = {"tunic", "shirt", "vest", "jerkin", "coat", "hauberk", "breastplate", "chestplate", "chestguard",
                "cuirass", "mantle", "cloak", "robe", "gown", "shroud", "vestment", "vestments", "tabard", "helm",
                "helmet", "cap", "hat", "hood", "cowl", "coif", "bracers", "bracer", "vambraces", "wristguards",
                "sleeves", "armguards", "leggings", "greaves", "legplates", "pants", "trousers", "pantaloons",
                "kilt", "skirt", "breeches", "boots", "shoes", "sandals", "slippers", "treads", "sabatons", "gloves",
                "gauntlets", "mitts", "mittens", "wraps", "handwraps"}
# armour nouns that take no material word ("a cloth crown" reads wrong)
PLAIN_NOUNS = {"circlet", "crown", "headband", "turban", "headwrap", "mask", "visor", "mantle", "cloak", "robe",
               "gown", "shroud", "vestment", "vestments", "tabard", "sleeves", "wraps", "handwraps", "kilt",
               "skirt", "slippers", "sandals"}
SLOT_NOUN = {1: ("helm", False), 2: ("robe", False), 3: ("earring", False), 4: ("necklace", False),
             5: ("tunic", False), 6: ("bracelet", False), 7: ("bracers", True), 8: ("ring", False),
             9: ("belt", False), 10: ("leggings", True), 11: ("boots", True), 13: ("shield", False),
             16: ("bow", False), 17: ("throwing weapon", False), 18: ("charm", False), 19: ("gloves", True),
             20: ("fishing pole", False), 21: ("bait", False)}
# itempattern.attacktype: 1 a one-handed slash, 2 two-handed, 3 one-handed blunt, 4 two-handed, 5 a one-handed
# pierce, 6 two-handed, 7 a bow, 9 a crossbow, 10 thrown (read off the records' names)
ATTACK_NOUN = {1: "blade", 2: "great blade", 3: "club", 4: "staff", 5: "dagger", 6: "spear", 7: "bow", 9: "crossbow",
               10: "throwing weapon"}
# materials a name may say, for jewelry, weapons and anything not drawn as armour
NAME_MATERIAL = ("gold", "golden", "silver", "iron", "steel", "bronze", "brass", "copper", "mithril", "electrum",
                 "ivory", "bone", "obsidian", "crystal", "jade", "ruby", "onyx", "pearl", "amber", "ebony", "oak",
                 "granite", "clay", "coral", "coralium", "rathesteel", "rubicite", "magmanite", "adamantite",
                 "silk", "leather", "hide", "wooden", "wood", "stone", "glass", "emerald", "sapphire", "diamond")
MATERIAL_ADJ = {"golden": "gold", "wood": "wooden", "coralium": "coral"}
SHIELD_MADE = ("of hardened wood rimmed with iron", "faced with a sheet of steel", "of layered hide over a wooden frame",
               "banded with iron", "of thick oak", "with a boss of hammered metal")
# sentences that pick up a word of the name: the first word found (in the name's order) gives one
LORE = {
    "etheric": "It shimmers faintly with an etheric light.",
    "otherworldly": "Its make is not of this world.",
    "dun": "Its dull brown hue blends well with the wilds.",
    "imperial": "It bears the crest of a long fallen empire.",
    "war": "It bears the scars of many battles.",
    "warlord": "It was made for one who led others into battle.",
    "nature": "The spirits of nature are said to favor its wearer.",
    "forest": "It carries the scent of the deep forest.",
    "bark": "It is reinforced with bark from an ancient tree.",
    "leaf": "Leaves are worked into its pattern.",
    "dark": "It seems to swallow the light around it.",
    "darkness": "It seems to swallow the light around it.",
    "shadow": "It is hard to see in dim light.",
    "silent": "It makes no sound, even in battle.",
    "assassin": "It was made for those who work unseen.",
    "wind": "It is as light as the wind.",
    "winds": "A faint breeze seems to follow it.",
    "whistling": "It whistles softly when moved quickly.",
    "guard": "It is the kind of gear a city guard is issued.",
    "guardian": "It was made for those who stand watch.",
    "protector": "It was made for those who shield others.",
    "defender": "It was made for those who hold the line.",
    "soldier": "It is standard issue among soldiers.",
    "knight": "It is fit for a knight of the realm.",
    "templar": "It bears the marks of a holy order.",
    "champion": "It was made for a champion of the arena.",
    "gladiator": "It was made for the fighting pits.",
    "valiant": "It was made for the valiant.",
    "black": "It is black as pitch.",
    "blackened": "Its surface has been blackened by fire.",
    "white": "It is a clean, bright white.",
    "blue": "It has been dyed a deep blue.",
    "red": "It has been dyed a deep red.",
    "green": "It has been dyed a deep green.",
    "spirit": "It hums with the presence of the spirits.",
    "spiritual": "It hums with the presence of the spirits.",
    "spiritrune": "Spirit runes are etched along its edges.",
    "ward": "Warding runes are inscribed upon it.",
    "runic": "Runes run along its surface.",
    "runed": "Runes run along its surface.",
    "etched": "Fine patterns are etched into its surface.",
    "woven": "It is tightly woven.",
    "torn": "This piece of armor has seen better days, but it is still wearable.",
    "decrepit": "This piece of armor has seen better days, but it is still wearable.",
    "shabby": "This piece of armor has seen better days, but it is still wearable.",
    "patchwork": "It has been patched together from many scraps.",
    "stained": "It is stained from long use.",
    "rugged": "It is rugged and made to last.",
    "goblin": "It is of goblin make, crude but sturdy.",
    "pickclaw": "The Pickclaw goblins of Highpass wear such things.",
    "gnoll": "It bears the marks of gnoll make.",
    "orcish": "Orcish smiths hammered it out for war.",
    "orc": "Orcish smiths hammered it out for war.",
    "froglok": "Frogloks of the swamps made it to their own measure.",
    "centaur": "Centaurs of the plains prize such work.",
    "hydra": "Hydra scales are worked into it.",
    "dragon": "It is said to have been touched by dragonfire.",
    "bear": "It is lined with bear fur.",
    "wolf": "It is trimmed with wolf fur.",
    "lion": "It is trimmed with a lion's mane.",
    "dog": "It is trimmed with the fur of a wild dog.",
    "howler": "It is trimmed with the pelt of a howler.",
    "serpent": "It bears the pattern of a coiled serpent.",
    "siren": "It is said to have been taken from a siren's hoard.",
    "golem": "It is as heavy as a golem's fist.",
    "giant": "It was made to a giant's measure.",
    "fang": "Fangs are set along its edges.",
    "claw": "A claw has been worked into it.",
    "claws": "Claws have been worked into it.",
    "skull": "A small skull is set into it.",
    "blood": "Old blood still stains it.",
    "flame": "It is warm to the touch.",
    "fire": "It is warm to the touch.",
    "fiery": "It is warm to the touch.",
    "ember": "It glows like a dying ember.",
    "magma": "It was cooled from molten rock.",
    "lava": "It was cooled from molten rock.",
    "firescale": "Its scales glow as if lit from within.",
    "firepit": "It smells of smoke and ash.",
    "ice": "It is cold to the touch.",
    "icy": "It is cold to the touch.",
    "frost": "Frost forms on it even in summer.",
    "snow": "It is as white as fresh snow.",
    "icescale": "Its scales are cold to the touch.",
    "glacierplate": "It was carved from ancient glacier ice.",
    "light": "It glows softly in the dark.",
    "radiance": "It glows softly in the dark.",
    "dawn": "It shines like the first light of day.",
    "predawn": "It is the grey of the sky before dawn.",
    "holy": "It carries a holy blessing.",
    "divine": "It carries a divine blessing.",
    "blessed": "It carries a priest's blessing.",
    "aura": "It is wrapped in a faint aura.",
    "mystic": "It hums with mystic power.",
    "arcane": "It hums with arcane power.",
    "elemental": "The elements seem to bend around it.",
    "astral": "It glitters like the night sky.",
    "star": "It glitters like the night sky.",
    "cosmos": "It glitters like the night sky.",
    "charmed": "A minor charm has been laid upon it.",
    "harmony": "Bards favor it for its pleasing balance.",
    "minstrel": "It is favored by minstrels and bards.",
    "song": "It seems to hum a tune of its own.",
    "melody": "It seems to hum a tune of its own.",
    "disciple": "It is worn by disciples of the fist.",
    "sensei": "It is worn by masters of the fist.",
    "fist": "It is worn by those who fight with their hands.",
    "brawler": "It was made for brawlers.",
    "archer": "It was made for an archer.",
    "traveler": "It was made for long roads.",
    "strider": "It was made for long roads.",
    "walker": "It was made for long roads.",
    "skystrider": "It is light enough to walk on the wind.",
    "seeker": "It was made for those who seek.",
    "tainted": "It reeks of plague.",
    "toxicity": "It reeks of poison.",
    "plaguereaver": "It reeks of plague.",
    "plaguebringer": "It reeks of plague.",
    "noroxus": "It reeks of the Plane of Disease.",
    "fungal": "Fungus grows in its seams.",
    "ancient": "It is very old.",
    "rathesteel": "It is forged of steel from the Rathe Mountains.",
    "chemist": "It smells faintly of an alchemist's workshop.",
    "experimentalist": "It smells faintly of an alchemist's workshop.",
    "transmuted": "An alchemist's craft has changed its very nature.",
    "summoner": "It is made for those who call upon the elements.",
    "summoners": "It is made for those who call upon the elements.",
    "channeler": "It is made for students of the arcane.",
    "evoker": "It is made for students of the arcane.",
    "arcanist": "It is made for students of the arcane.",
    "beguiler": "It is made for those who bend minds.",
    "illusionary": "It flickers when looked at directly.",
    "diviner": "It is made for those who read the signs.",
    "wizard": "It is made for students of the arcane.",
    "mage": "It is made for students of the arcane.",
    "reaver": "It was made for slaughter.",
    "ransacker": "It was taken in a raid.",
    "slayer": "It was made for slaughter.",
    "worldbreaker": "It was made to break all that stands before it.",
    "sea": "It smells of the sea.",
    "fathom": "It was brought up from the deep.",
    "deep": "It was brought up from the deep.",
    "isle": "It was made on a distant isle.",
    "badland": "Sand still clings to it.",
    "toxxulia": "It comes from the Toxxulia Forest.",
    "primordial": "It is older than memory.",
    "earthen": "It is caked with earth.",
    "rock": "It is as hard as rock.",
    "granite": "It is as hard as granite.",
    "crystal": "It catches the light like a crystal.",
    "obsidian": "Its obsidian surface is glassy and black.",
    "rubicite": "It is made of deep red rubicite.",
    "mithril": "Its mithril is light and strong.",
    "ivory": "It is carved from ivory.",
    "ebony": "It is made of dark ebony.",
    "bone": "It is carved from bone.",
    "gods": "It is said to have been made by the gods.",
    "creator": "It is said to be the work of a great creator.",
    "golden": "It gleams like gold.",
    "dead": "It carries the chill of the grave.",
    "flesh": "It is stitched from dried flesh.",
    "tower": "It is as solid as a tower wall.",
    "kite": "It is shaped like a kite.",
    "splint": "It is reinforced with splints of metal.",
    "studded": "It is studded with metal rivets.",
    "plated": "It is plated in metal.",
    "superior": "It is of superior make.",
    "hardened": "It has been hardened for battle.",
    "tempered": "Its metal has been well tempered.",
    "forged": "It was forged by a skilled smith.",
    "encrusted": "It is encrusted with small gems.",
    "deathfist": "It bears the mark of the Deathfist orcs.",
    "elddar": "It was made beneath the great trees of the Elddar Forest.",
    "elven": "It is of elven make, light and graceful.",
    "dwarven": "It is of dwarven make, heavy and sure.",
    "kerran": "It is of kerran make.",
    "aviak": "Aviak feathers are worked into it.",
    "griffon": "Griffon feathers are worked into it.",
    "wyrm": "It is said to have been taken from a wyrm's lair.",
    "drake": "Drake scales are worked into it.",
    "chimera": "It is made from the hide of a chimera.",
    "spectre": "A spectral chill clings to it.",
    "zombie": "It reeks of the grave.",
    "werewolf": "It is lined with werewolf fur.",
    "treant": "It is carved from a treant's limb.",
    "pixie": "Pixie dust still glitters upon it.",
    "clockwork": "It ticks faintly when held to the ear.",
    "ogre": "It was made to an ogre's measure.",
    "tae": "It is of Tae Ew make.",
    "rathe": "It comes from the Rathe Mountains.",
    "karana": "It carries the blessing of Karana, the Rainkeeper.",
    "brell": "It bears the mark of Brell Serilis.",
    "solusek": "It carries the heat of Solusek Ro.",
    "zek": "It bears the mark of Rallos Zek.",
    "rallosian": "It bears the mark of Rallos Zek.",
}
# the virtues and vices of an "of ..." name: "Gloves of Insight" -> "Those who wear it are said to find insight."
VIRTUES = {"insight", "might", "clarity", "purity", "grace", "faith", "honor", "virtue", "fortitude", "valor",
           "courage", "wisdom", "intellect", "power", "energy", "focus", "vision", "strength", "vigor", "will",
           "harmony", "balance", "serenity", "patience", "resolve", "wrath", "fury", "glory", "truth"}
VICES = {"loathing", "spite", "malice", "disbelief", "turmoil", "miscontent", "doubt", "despair", "horror",
         "hatred", "misery", "agony", "pain", "decay", "torment", "dread", "fear", "sorrow", "envy", "greed"}
STAT_WORD = (("str", "strength"), ("sta", "stamina"), ("agi", "agility"), ("dex", "dexterity"),
             ("wis", "wisdom"), ("intelligence", "intelligence"), ("cha", "charisma"))
RESIST = (("FR", "fire"), ("CR", "cold"), ("LR", "lightning"), ("AR", "arcane magic"), ("PR", "poison"),
          ("DR", "disease"))
QUALITY = ((12, ("simple", "plain", "crude", "worn")), (30, ("sturdy", "well made", "serviceable", "fine")),
           (50, ("finely made", "superior", "well crafted", "handsome")),
           (99, ("masterwork", "exquisite", "flawless", "superbly crafted")))
# the same, after "is": "This cap is of masterwork quality."
QUALITY_IS = ((12, ("simple", "plain", "crude", "well worn")), (30, ("sturdy", "well made", "serviceable", "sound")),
              (50, ("finely made", "of superior make", "well crafted", "handsome")),
              (99, ("of masterwork quality", "exquisite", "flawless", "superbly crafted")))
# a possessive that names a kind of person, not a person: "Dancer's Foil" -> "It was made for a dancer."
ROLES = {"dancer", "champion", "soldier", "warrior", "hunter", "ranger", "rogue", "thief", "priest", "cleric",
         "druid", "shaman", "wizard", "mage", "magician", "necromancer", "enchanter", "alchemist", "monk", "bard",
         "paladin", "knight", "lord", "lady", "king", "queen", "prince", "princess", "captain", "sergeant",
         "general", "guard", "guardian", "scout", "raider", "sailor", "miner", "smith", "merchant", "farmer",
         "shepherd", "apprentice", "student", "master", "elder", "chief", "chieftain", "warlord", "invoker",
         "summoner", "channeler", "healer", "herbalist", "animator", "brawler", "archer", "gladiator",
         "traveler", "traveller", "pilgrim", "acolyte", "initiate", "novice", "squire", "mercenary", "bandit",
         "pirate", "assassin", "duelist", "fighter", "sorcerer", "warlock", "witch", "seer", "mystic", "oracle",
         "sage", "scholar", "hermit", "noble", "nomad", "trapper", "fisherman", "woodsman", "lumberjack",
         "executioner", "jailer", "jester", "minstrel", "beggar", "peasant", "slave", "ransacker", "marauder",
         "defender", "protector", "templar", "crusader", "zealot", "disciple", "sensei", "evoker", "arcanist",
         "beguiler", "diviner", "conjurer", "illusionist", "berserker", "slayer", "dragonslayer", "seeker",
         "wanderer", "outcast", "exile", "rower", "chemist", "experimentalist", "caller", "visionwalker",
         "golem", "giant", "dragon", "troll", "ogre", "orc", "goblin", "gnoll", "froglok", "kobold", "centaur",
         "crafter", "tinkerer", "tailor", "jeweler", "fletcher", "bowyer", "brewer", "baker", "cook", "warder",
         "legionnaire", "legionaire", "sentry", "sentinel", "watchman", "warden", "ranger", "bishop", "abbot",
         "friar", "deacon", "wayfarer", "vagabond", "rogue", "highwayman", "hunter", "slaver", "tracker"}
# hand-written lines for the items no wear slot describes (quest pieces, ingredients): OURS
MISC = {
    "ransacker's club": "A heavy club carried by orc ransackers.",
    "orc ransacker's cap": "A battered cap worn by orc ransackers.",
    "orc ransacker's belt": "A crude belt worn by orc ransackers.",
    "ransacker's boots": "Heavy boots worn by orc ransackers.",
    "charm of the isle spirit": "A charm that holds the blessing of an isle spirit.",
    "daran's sight": "It is said to let its bearer see as Daran once saw.",
    "elddar tome": "An old tome written in the tongue of the Elddar.",
    "elddar seeds": "Seeds gathered beneath the great trees of the Elddar Forest.",
    "blue raven feather": "A glossy blue feather from a raven.",
    "olive oil seeds": "Seeds that can be pressed for olive oil.",
    "fish liver oil": "A small vial of foul smelling fish liver oil.",
    "albino rat tail": "The pale tail of an albino rat.",
    "green mamba snake venom": "A vial of venom milked from a green mamba.",
    "blacktoed wolf fang": "A fang from a blacktoed wolf.",
    "shimmering silk strand": "A single strand of silk that shimmers in the light.",
    "windy's recipe": "A recipe written in Windy's careful hand.",
    "pixie tears": "A tiny vial of pixie tears.",
    "crelwin's robe": "A robe once worn by Crelwin.",
    "clay mixing vial": "A clay vial for mixing reagents.",
    "silver glowing bar": "A bar of silver that glows faintly.",
    "moradhim ore": "A lump of raw ore, heavy for its size.",
    "sea of chains": "A tangle of fine chains.",
    "ivory": "A piece of polished ivory.",
}
GENERIC = {
    "armour": ("It offers fair protection in battle.", "It will turn a careless blow.", "It fits snugly.",
               "It is light enough to fight in.", "It is comfortable to wear."),
    "jewel": ("It glints in the light.", "It is pleasing to the eye.", "It is cool to the touch.",
              "Its maker took great care with it."),
    "weapon": ("It is well balanced.", "It holds a keen edge.", "It feels good in the hand.",
               "It has seen its share of fights."),
    "blunt": ("It is well balanced.", "It lands with a satisfying weight.", "It feels good in the hand.",
              "It has seen its share of fights."),
    "held": ("It hums faintly when held.", "It is warm in the hand.", "Its maker's craft is plain to see."),
    "shield": ("It has turned many blows.", "Its face is scarred from use.", "It is well balanced on the arm."),
    "bow": ("Its string is taut and true.", "It draws smoothly.", "It is well balanced."),
}
JEWEL_SLOTS = (3, 4, 6, 8, 9)
WEAPON_SLOTS = (12, 14, 15, 17)
HELD_SLOTS = (18,)
WIELDED = (12, 13, 14, 15, 16, 17, 18, 20)
# a plural piece's sentences: "It is" -> "They are" (the verbs these sentences use)
THEY = [(r"\bIt is\b", "They are"), (r"\bIt has\b", "They have"), (r"\bIt was\b", "They were"),
        (r"\bIt will\b", "They will"), (r"\bIt (\w+?)(e?s)\b", None), (r"\bIts\b", "Their"),
        (r"\b(on|into|to|upon|around|with|wear|follow|stains|carry|in) it\b", r"\1 them"), (r"\bits\b", "their")]


def _pick(name, salt, options):
    h = int(hashlib.sha1(f"{salt}:{name}".encode()).hexdigest(), 16)
    return options[h % len(options)]


def _words(name):
    return re.findall(r"[A-Za-z']+", name)


def _a(phrase):
    return ("an " if phrase[:1].lower() in "aeiou" else "a ") + phrase


# two-word names of one piece: "Breast Plate" is a breastplate
JOINED = ((r"\bbreast ?plates?\b", "breastplate"), (r"\bchest ?plates?\b", "chestplate"),
          (r"\bchest ?guard\b", "chestguard"), (r"\bleg ?plates\b", "legplates"), (r"\bhead ?band\b", "headband"),
          (r"\bwrist ?guards\b", "wristguards"), (r"\bshort ?sword\b", "shortsword"), (r"\blong ?sword\b", "longsword"),
          (r"\bgreat ?sword\b", "greatsword"), (r"\bwar ?hammer\b", "warhammer"), (r"\bhand ?axe\b", "handaxe"),
          (r"\blong ?bow\b", "longbow"), (r"\bshort ?bow\b", "shortbow"))
# the heavy armour's own nouns, where a name gives none (the database's plate pieces are named so)
HEAVY_NOUN = {5: ("breastplate", False), 10: ("greaves", True), 19: ("gauntlets", True)}


def _noun(item):
    found = None
    name = item["name"]
    for pat, rep in JOINED:
        name = re.sub(pat, rep, name, flags=re.I)
    for w in _words(name.replace("'s ", " ")):
        k = w.lower().strip("'")
        if k in NOUNS:
            found = NOUNS[k]
    if found:
        return found
    slot = item["slot"]
    if slot in HEAVY_NOUN and MATERIAL.get(item.get("model") or 0) in ("plate", "banded"):
        return HEAVY_NOUN[slot]
    if slot in (12, 14, 15):
        return ATTACK_NOUN.get(item.get("attacktype") or 0, "weapon"), False
    return SLOT_NOUN.get(slot, ("trinket", False))


def _name_material(item):
    for w in _words(item["name"]):
        k = w.lower()
        if k in NAME_MATERIAL:
            return MATERIAL_ADJ.get(k, k)
    return None


def _quality(name, level, table=QUALITY):
    for top, words in table:
        if level <= top:
            return _pick(name, "quality", words)
    return table[-1][1][0]


def _first(item, noun, plural, magic=False):
    """The first sentence: what it is and what it is made of."""
    name, slot, level = item["name"], item["slot"], item.get("level") or 0
    q = "magical" if magic and _pick(name, "magical", (0, 0, 0, 1)) else _quality(name, level)
    qi = _quality(name, level, QUALITY_IS)
    if slot in ARMOUR and noun in PLAIN_NOUNS:
        mat = _name_material(item)
        stuff = f"{mat} {noun}" if mat and mat not in noun else noun
        return f"A pair of {q} {stuff}." if plural else _a(f"{q} {stuff}.").capitalize()
    if slot in ARMOUR and noun in ARMOUR_NOUNS:
        mat = MATERIAL.get(item.get("model") or 0)
        if not mat:
            return f"{'These' if plural else 'This'} {noun} {'are' if plural else 'is'} {qi}."
        made = _pick(name, "made", MADE_OF[mat])
        if mat in noun or (mat == "plate" and "plate" in noun):
            phrase = noun
        else:
            phrase = f"{mat} {noun}"
        form = _pick(name, "form", (0, 1, 2))
        if form == 0:
            return ("A pair of " + f"{q} {phrase}." if plural else _a(f"{q} {phrase}.")).capitalize()
        if form == 1:
            return f"{'These' if plural else 'This'} {phrase} {'are' if plural else 'is'} {qi}."
        return ("A pair of " + f"{noun} {made}." if plural else _a(f"{noun} {made}.")).capitalize()
    mat = _name_material(item)
    if slot == 2:
        mat = mat or _pick(name, "robe", ("fine cloth", "heavy cloth", "soft silk", "dark cloth"))
        return _a(f"{q} {noun} of {mat}.").capitalize()
    stuff = f"{mat} {noun}" if mat and mat not in noun else noun
    if slot == 13 and not mat and _pick(name, "shieldform", (0, 1)):
        return _a(f"{noun} {_pick(name, 'shieldmade', SHIELD_MADE)}.").capitalize()
    if plural:
        return f"A pair of {q} {stuff}."
    return _a(f"{q} {stuff}.").capitalize()


def _lore(item, noun):
    name = item["name"]
    m = re.match(r"^([A-Za-z`']+)'s\s", name)
    if m:
        who = m.group(1)
        k = who.lower()
        if k in ROLES and k not in LORE:
            return f"It was made for {_a(k)}."
        if who[:1].isupper() and k not in LORE and k not in NOUNS and k not in ROLES:
            return _pick(name, "owner", (f"Once the property of {who}.", f"It is said to have belonged to {who}.",
                                         f"It bears the mark of {who}."))
    for w in _words(name):
        k = re.sub(r"'s$", "", w.lower()).strip("'")
        if k in LORE:
            return LORE[k]
    tail = re.search(r"\bof (?:the )?([A-Za-z]+)\s*$", name)
    if tail:
        k = tail.group(1).lower()
        if k in VIRTUES:
            return f"Those who {'carry' if item['slot'] in WIELDED else 'wear'} it are said to find {k}."
        if k in VICES:
            return f"It is steeped in {k}."
    return None


def _enchant(name, mag, what):
    m = mag + " " if mag else ""
    form = _pick(name, "enchant", (0, 0, 1, 2))
    if form == 1:
        return f"{_a(m + what + ' enchantment').capitalize()} has been laid upon it."
    if form == 2:
        return f"It carries {_a(m + 'enchantment')} of {what}."
    return f"It holds {_a(m + what + ' enchantment')}."


def _magnitude(v):
    return "minor" if v <= 5 else "lesser" if v <= 12 else "" if v <= 25 else "greater" if v <= 45 else "potent"


def _magic(item):
    stats = [(item.get(c) or 0, w) for c, w in STAT_WORD]
    v, w = max(stats)
    hp, power = item.get("HPMAX") or 0, item.get("POWMAX") or 0
    if v > 0 and v * 4 >= max(hp, power):
        return _enchant(item["name"], _magnitude(v), w)
    if hp > 0 or power > 0:
        return _enchant(item["name"], _magnitude(max(hp, power) / 4), "health" if hp >= power else "power")
    res = max(((item.get(c) or 0, w) for c, w in RESIST))
    who = "wielder" if item["slot"] in WIELDED else "wearer"
    if res[0] > 0:
        return f"It wards its {who} against {res[1]}."
    if (item.get("HoT") or 0) > 0:
        return f"It speeds the healing of its {who}."
    if (item.get("PoT") or 0) > 0:
        return f"It quickens the return of its {who}'s power."
    return None


def _kind(item, noun):
    slot = item["slot"]
    if slot in ARMOUR or slot == 2:
        return "armour"
    if slot in JEWEL_SLOTS:
        return "jewel"
    if slot == 13:
        return "shield"
    if slot == 16:
        return "bow"
    if slot in HELD_SLOTS:
        return "held"
    if noun in ("club", "mace", "hammer", "maul", "flail", "cudgel", "stick", "baton", "staff", "quarterstaff",
                "bo staff", "great club", "mattock", "morning star"):
        return "blunt"
    return "weapon"


def _they(text):
    """A sentence about one thing turned to a pair's ("It holds" -> "They hold")."""
    for pat, rep in THEY:
        if rep is None:
            def verb(m):
                v = m.group(1) + m.group(2)
                stem = v[:-3] + "y" if v.endswith("ies") else v[:-2] if re.search(r"(sh|ch|ss|x|z)es$", v) else v[:-1]
                return "They " + stem
            text = re.sub(pat, verb, text)
        else:
            text = re.sub(pat, rep, text)
    return text


def describe(item):
    """One to three short sentences for the record (a dict: name, slot, model, level, attacktype, the stats)."""
    misc = MISC.get(" ".join(item["name"].lower().split()))
    if misc:
        return misc
    noun, plural = _noun(item)
    magic = _magic(item)
    first = _first(item, noun, plural, bool(magic))
    lore = _lore(item, noun)
    if lore and "armor" in lore and not (item["slot"] in ARMOUR or item["slot"] == 2):
        lore = "It has seen better days."
    parts = [first]
    if lore:
        parts.append(lore)
    if magic:
        parts.append(magic)
    if len(parts) == 1:
        parts.append(_pick(item["name"], "generic", GENERIC[_kind(item, noun)]))
    if plural:
        parts = [parts[0]] + [_they(p) for p in parts[1:]]
    text = " ".join(parts)
    while len(text) > MAX_LEN and len(parts) > 1:
        parts.pop(1 if len(parts) > 2 else -1)
        text = " ".join(parts)
    return re.sub(r"\s+", " ", text).strip()
