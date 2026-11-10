# The lore the town dialog's lore lines rest on

`tools/dialog/lore_lines.json` (the `lore` set of `tools/seed_town_dialog.py`) is what the towns' people and the
guards of Norrath say of their world's own story: EverQuest Online Adventures', about five hundred years before
EverQuest's. No line of it is invented lore. Each line's `note` names the facts it rests on by the ids below
(`tools/seed_town_dialog.py --set lore --list` prints every line with who says it and its note), and each fact
names where it stands in a source, with the source's own words, so a reviewer can hold any line to it. A line
whose fact could not be found in a source was left out. `tools/seed_town_dialog_lore_test.py` holds every line to
a note whose ids are here.

The continent is Tunaria; the elves' cities are Fayspires and Tethelin, the gnomes' Klick'Anon, the dwarves'
Moradhim, the Erudites' Highbourne (on Tunaria's south-west coast, their fleet building for the Exodus to Odus,
where Arcadin is rising). Where a source shows a thing of EverQuest's own day in its earlier form, the lines use
EQOA's form; nothing of EverQuest's present is spoken of (the test keeps the list of names no line may say).

## The sources, in the order they are trusted

| A fact's source reads | What it is |
|---|---|
| WORDS.TXT n | the game disc's DATA/WORDS.TXT (EQOA: Frontiers, SLUS-20744), by line: the client's own strings |
| Frontiers manual | the EverQuest Online Adventures: Frontiers game manual (SOE, 2003) |
| Prima, factions: X | Prima's Official Strategy Guide, EverQuest Online Adventures: Frontiers (2003), the Faction chapter's Table 1: the note beside the faction named |
| Prima, walkthrough, line n | the same guide's quest walkthroughs (its text, by line of the layout-free extraction) |
| SOE lore, page | SOE's lore pages for EQOA ("EQOA Official Lore (From SOE's Website)"), as Project Return Home keeps them: https://www.projectreturnhome.com/<page> (fetched 2026-11-10) |
| EQOA-Data, set/guide | EQOA-Data (github.com/Jadiction/EQOA-Data), Quests/<set>/<guide>.json: players' quest guides |
| EQOA-Data map: X | EQOA-Data's Map/zones.json: a camp or a point players marked on the world map |
| quest data, file | this repository's quest data (tools/quests/data/<file>.json, written from EQOA-Data's guides) |
| zone grid: B lies n zones (way) of A | the client's zone grid (the zones' names are WORDS.TXT 1041-1187; which lies beside which is the client's table of them, 2,000 units a zone) |
| npcs: X | who stands in the town on this server (table npcs, by name; Return Home's records of the live game and Sandstorm's) |

The game's own text and SOE's own lore come first; Prima's guide was written with SOE's designers (its faction notes are theirs); players' guides and maps say where things stand and who asks for what; the zone grid and the server's own NPCs say who is whose neighbour.

## The gods and their pacts

- **GODS-1** Veeshan, the crystalline dragon, was the first god to come to a lifeless Norrath; she marked it with her claws and gave it her children, the dragons, wurms and drakes. That first time is the Age of Scale.
  - SOE lore, gods-veeshan: "Veeshan, the great crystalline dragon, was the first of the deities to arrive on the then lifeless world of Norrath."
  - SOE lore, gods-veeshan: "with a cataclysmic swipe of her massive claws, she marked the planet and populated it with her children, the Dragonkind. The Dragonkind include all races of dragons, wurms and drakes."
  - SOE lore, gods-veeshan: "This marked the begining of Norrathian History and is referred to as "The Age of Scale"."
- **GODS-2** Veeshan is the Wurm Queen, the Mother of All Wurms; her home is the Plane of Sky, she has no allies, and Brell Serilis is her enemy.
  - SOE lore, gods-veeshan: "Title: The Wurm Queen or The Mother of all Wurms"
  - SOE lore, gods-veeshan: "Enemies: Brell Serilis"
  - SOE lore, gods-veeshan: "Home: Plane of Sky"
- **GODS-3** Brell Serilis, the Duke of Below, came second, claimed the caves under Norrath, loosed creatures no one can name into its belly from his Plane of the Underfoot, and sealed his portal with a gate of living stone.
  - SOE lore, gods-brell-serlis: "The second diety to arrive on Norrath, Brell Serilis, is a clever and conniving God. Upon discovery of Norrath, Brell immediately laid claim to the underground caves."
  - SOE lore, gods-brell-serlis: "He released a variety of creatures into the belly of Norrath. The identity of these creatures remains unclear to this day."
  - SOE lore, gods-brell-serlis: "sealed the planar portal with a gate of living stone"
- **GODS-4** In the First Pact of the Gods Brell made the dwarves and set them deep below the crust; by a second pact he made the gnomes and put them deep underground too.
  - SOE lore, gods-brell-serlis: "during the The First Pact of the Gods, Brell created the Dwarves and deposited them deep below Norrath's crust."
  - SOE lore, gods-brell-serlis: "The result of this pact was the birth of Brell's second known creation on Norrath, the Gnomes. Following tradition, Brell placed them deep underground"
- **GODS-5** Goblins and gnolls also claim to be Brell's children; gnomes and dwarves call that blasphemy.
  - SOE lore, gods-brell-serlis: "The Goblins and the Gnolls also claim to be children of Brell, but this is dismissed quickly by the Gnomes and the Dwarves as blasphemy."
- **GODS-6** Tunare, the Mother of All, came soon after Brell, took the surface of Norrath in the First Pact and made the elves to guard it. Her home is the Plane of Growth; Karana, Erollisi Marr and Rodcet Nife are her allies, Bertoxxulous her enemy.
  - SOE lore, gods-tunare: "Tunare came to Norrath soon after Brell Serilis arrived. She was a part of the First Pact of the Gods. Tunare assumed the surface of Norrath as her territory and to guard it, she created the elves."
  - SOE lore, gods-tunare: "Allies: Karana, Erollisi Marr, Rodcet Nife"
  - SOE lore, gods-tunare: "Enemies: Bertoxxulous"
- **GODS-7** Prexus, the Ocean Lord, was the third of the First Pact; he took the seas and made the Kedge, who live deep under water and are a mystery to everyone else.
  - SOE lore, gods-prexus: "He was the third and final member of the First Pact of the Gods. He laid claim over Norrath's Seas."
  - SOE lore, gods-prexus: "The Kedge live deep underwater and are very much a mystery to other Norrathians."
- **GODS-8** Rallos Zek, the Warlord, came with the first gods but kept out of the First Pact; he left his mark anyway by making the giants.
  - SOE lore, gods-rallos-zek: "Rallos Zek came with the first group of gods. He chose not to participate in The First Pact of the Gods. He did, however, manage to leave his mark on the world by creating the powerful warlike race of the Giants."
- **GODS-9** When Brell formed the Second Pact, Rallos Zek loosed the ogres and the orcs; all his creations are fierce and made for war.
  - SOE lore, gods-rallos-zek: "It was at this time that Rallos released his newest creations; the Ogres and the Orcs."
  - SOE lore, gods-rallos-zek: "All of Rallos Zek's creations were fierce and powerful races that embodied their creator's propensity for war."
- **GODS-10** In the Second Pact Cazic-Thule, the Faceless, made the lizard men and the trolls and gave them the swamps and the dark jungles. Fear governs them, and to his followers the power of fear is ultimate.
  - SOE lore, gods-cazic-thule: "Cazic-Thule, as part of The Second Pact of the Gods, created the Lizard Men and the Trolls. He chose the swamps and the dark jungles of Norrath for the home of his creations."
  - SOE lore, gods-cazic-thule: "Fear governs them and they use it to govern the lives of others. Needless to say, to the followers of Cazic-Thule, the power of fear is ultimate."
- **GODS-11** Fizzlethorpe Bristlebane, the King of Thieves, came fashionably late, joined Brell and Cazic-Thule in the Second Pact, and made the halflings, tucked away in an out-of-the-way corner to keep them out of quarrels.
  - SOE lore, gods-fizzlethorpe-bristlebane: "Fizzlethorpe Bristlebane strolls into the scene fashionably late."
  - SOE lore, gods-fizzlethorpe-bristlebane: "Bristlebane met with Brell and Cazic-Thule to form the Second Pact of the Gods."
  - SOE lore, gods-fizzlethorpe-bristlebane: "chosing to avoid conflict with the other races, placed his Halflings in an out-of-the-way, secluded area of Norrath"
- **GODS-12** Followers of Bristlebane should not be trusted with anything of value, but they can be counted on for a good time.
  - SOE lore, gods-fizzlethorpe-bristlebane: "Followers of Bristlebane should not be trusted with anything of significant value but they can be counted on for a good time."
- **GODS-13** Innoruuk, the Prince of Hate, found Norrath after the First Pact, raged that it was peopled without him, and turned on Tunare: he took the elves' first King and Queen into his Plane of Hate and tortured them three hundred years, and of them came the Teir'Dal, the dark elves.
  - SOE lore, gods-innoruk: "Innoruuk discovered Norrath shortly after the First Pact of the Gods. He was infuriated when he discovered that his fellow gods had populated a world and formed a pact without his knowledge."
  - SOE lore, gods-innoruk: "Innoruuk corrupted the first King and Queen of Tunare's beloved Elves. He took them into his Plane of Hate and tortured them for 300 years"
- **GODS-14** Mithaniel Marr, the Truthbringer, and his twin Erollisi were the last of the greater gods to come, to a world already at war; together they made the barbarians. His followers are valorous and honorable.
  - SOE lore, gods-mithaniel-marr: "Mithaniel and his twin sister, Erollisi Marr, were the last of the greater gods to arrive to Norrath. The planet had already been wrought with war"
  - SOE lore, gods-mithaniel-marr: "Followers of Mithaniel Marr are valorous and honorable men and women."
- **GODS-15** Erollisi Marr, the Queen of Love, and her brother set the barbarians to roam the frozen north in nomad tribes; the barbarians are the forefathers of the humans, and even of the Erudites. Her followers are warrior poets.
  - SOE lore, gods-erollisi-marr: "She and her brother created the Barbarians and deposited them to roam in nomadic tribes in the frozen north of Norrath. The Barbarians are the ancestors of the Humans and even the Erudites."
  - SOE lore, gods-erollisi-marr: "Followers of Erollisi Marr are passionate and can be described as 'Warrior Poets'"
- **GODS-16** The gods' quarrels: Innoruuk and Cazic-Thule stand with Rallos Zek; the Marr twins are enemies of Innoruuk and Cazic-Thule; Bristlebane's one ally is Brell and he has no enemies.
  - SOE lore, gods-rallos-zek: "Allies: Innoruuk, Cazic-Thule"
  - SOE lore, gods-mithaniel-marr: "Enemies: Innoruuk, Cazic-Thule, Bertoxxulous"
  - SOE lore, gods-fizzlethorpe-bristlebane: "Enemies: none"

## The peoples of Tunaria

- **BARB-1** The frozen north was shut off for centuries; only lately did the barbarians unite, leave their nomad ways and join Tunaria's other peoples as a settled one.
  - SOE lore, race-barbarian: "The frozen north has remained isolated for centuries. It wasn't until just recently that the Barbarians, uniting and leaving their nomadic heritage behind, joined the other races of Tunaria as a civilized people."
- **BARB-2** Orcs, goblins and the frost giants press the barbarians from every side and could snuff out all they have built.
  - SOE lore, race-barbarian: "Orcs, goblins, and even the frost giants assault the Barbarians from all sides, threatening to snuff out all they have worked to build."
- **BARB-3** Their city Halas stands in the mountains of Everfrost; the world calls them barbarians, and they call themselves Northmen.
  - SOE lore, race-barbarian: "Their city of Halas is located in the mountains of Everfrost. Though referred to as barbarians by the rest of the world, these proud people refer to themselves as "Northmen.""
- **BARB-4** Northmen take great pride in clan lineage; eating, drinking and battle are their pastimes. Most become warriors for their clan's honor, some hear the call to be clan shamans, and a few loners win honor as rogues.
  - SOE lore, race-barbarian: "Barbarians take great pride in clan lineage, and eating, drinking, and battle are their preferred pastimes."
  - SOE lore, race-barbarian: "some feel the mystical call to serve as clan shamans"
  - SOE lore, race-barbarian: "Barbarian rogues are usually loners who have decided to bring their clan honor through less respectable methods."
- **BARB-5** The barbarians are the hardy stock of Northmen from which all humans came.
  - WORDS.TXT 372: "Barbarians are the hardy stock of Northmen from which all humans originated.  They are the fierce, steely warriors of legend."
- **DELF-1** The dark elves poured up out of the Underfoot onto Tunaria, and by necromancy have just won their greatest victory over the elves of light: the destruction of Takish'Hiz.
  - SOE lore, race-delf: "From deep Underfoot the Elves of dark have poured forth in a seething mass upon Tunaria. Through the foul powers of necromancy the Dark Elves have just realized their greatest victory against the Elves of light, the destruction of Takish`Hiz."
- **DELF-2** Once the elves are driven from Tunaria the dark elves mean to be the greatest power on it.
  - SOE lore, race-delf: "Once the Elves are finally driven from Tunaria, the Dark Elves will stand on the brink of becoming the most powerful force on all of Tunaria."
- **DELF-3** The Teir'Dal hate every other race and each other; they help one another only for their own ends: "I will help you now to use you later."
  - SOE lore, race-delf: "Dark elves are a race filled with hate; they hate all other races, and even their fellow Teir`Dal. They will lend aid to one another, but only for their own selfish reasons."
- **DELF-4** The dark elves are unrivaled in cunning and wit, and driven by a thirst for personal power that is never quenched.
  - WORDS.TXT 368: "The Dark Elves are unrivaled in their cunning, wit, and malevolent drive to fulfill their unquenchable desire for personal power."
- **DWARF-1** Almost three centuries ago the dwarven King Stormhammer sent out many expeditions for new resources; one of them became the mining colony of Moradhim, in the hills of the Serpent's Spine.
  - SOE lore, race-dwarf: "Almost 3 centuries ago, the Dwarven King Stormhammer sent out many expeditions in search of new resources. One of those expeditions became the mining colony of Moradhim, nestled in the hills of the Serpent's Spine."
- **DWARF-2** The dwarves are content to harvest the mountains and care nothing for the elven war raging to the east.
  - SOE lore, race-dwarf: "From there, the dwarves stay content harvesting the fruits of the mountains, apathetic to the Elven war that rages to the East."
- **DWARF-3** Dwarves do not count their height a lack (a kneecap breaks as well as a nose); every dwarf believes in good strong ale, and taverns across the lands keep stools built to take a beating for them.
  - SOE lore, race-dwarf: "it is just as satisfying to break kneecaps as it is to break noses"
  - SOE lore, race-dwarf: "To better serve their dwarven patrons, taverns across the lands have specially constructed stools that can take a beating."
- **DWARF-4** Dwarves are stout and hardy, jovial of temper, and stubborn and unforgiving when crossed.
  - WORDS.TXT 370: "Dwarves are a stout and hardy race whose temperament is jovial, though stubborn and unforgiving when crossed."
- **ELF-1** Takish'Hiz, the ancient home of the elves, is a smoking ruin; most of its refugees fled north to the new city of Fayspires and left the old one to be looted by its conquerors, the dark elves.
  - SOE lore, race-elf: "Takish`Hiz, the ancient home of the Elves, is now a smoking ruin. Most of the Elven refugees have fled north to the new city of Fayspires, leaving their ancestral city to be looted by their conquerors, the Dark Elves."
- **ELF-2** From Fayspires the elves left on Tunaria race to build a fleet and join their brethren across the Ocean of Tears.
  - SOE lore, race-elf: "From this new city the remaining elves of Tunaria race to build their fleet and join their brethren across the Ocean of Tears."
- **ELF-3** Most races like the elves; dark elves and trolls would sooner turn them on a spit than speak to them. Some elves live high in the trees of Tethelin, safe and in sight of their woods.
  - SOE lore, race-elf: "dark elves and trolls would much rather turn them on a spit than speak to them"
  - SOE lore, race-elf: "Some elves make their homes high up in the trees of Tethlin, which provide them with not only safety but also a view of their cherished woodlands."
- **ELF-4** The elves of Tunaria hold the balance of the natural world sacred above all things.
  - WORDS.TXT 367: "The Elves of Tunaria are intelligent, wise and charismatic beings that hold the delicate balance of the natural world as sacred above all things."
- **ERUD-1** Highbourne, the city of the Highmen in the south and west, was built on earth, stone and the words of their leader Erud; under him the Erudites began a new age of discovery and learning.
  - SOE lore, race-erudite: "The city of the Highmen, Highbourne, has been built upon foundation of earth, stone, and the words of their leader, Erud. Under Erud's tutelage, the Erudites begin a new age of discovery and learning."
- **ERUD-2** The violence and greed of the land grew too much for them: the Erudites look to the Exodus of Tunaria, and from Highbourne they will build the fleet to carry the Highmen across the ocean.
  - SOE lore, race-erudite: "But the increasing violence and opportunism of the land is too much. Desperate to escape, the Erudites now look towards the Exodus of Tunaria."
  - SOE lore, race-erudite: "Erud has gathered his people together to build the city of Highbourne, from which they will build their fleet to carry the Highmen across the ocean."
- **ERUD-3** The High Men are the opposite of the Northmen: proud of mind and manners, careless of the body, sure any obstacle yields to intelligence and wisdom, and with few friends outside their own race.
  - SOE lore, race-erudite: "Being the exact opposite of the barbarian Northmen, the high man prides himself on his intellectual abilities and social graces"
  - SOE lore, race-erudite: "Any obstacle can be overcome with intelligence and wisdom."
  - SOE lore, race-erudite: "Erudites have very few friends outside their own race."
  - WORDS.TXT 374: "Erudites, otherwise known as the High Men, are unrivaled in their affinity and mastery of the arcane sciences."
- **GNOME-1** By exploratory mining a colony of gnomes found a place on the surface to build Klick'Anon and experiment in peace, and found themselves in the middle of the elves' great war; now they work against, and beside, both elven peoples to survive.
  - SOE lore, race-gnome: "Through exploratory mining a colony of Gnomes found a suitable location on the surface of Norrath where they could build their city of Klick`Anon and go about their experiments in peace. Unfortunately they also found themselves in the middle of a titanic Elven war."
  - SOE lore, race-gnome: "Now they must work against, and along side, both of the Elven races in a desperate struggle for survival."
- **GNOME-2** Gnomes hold that building contraptions ranks with saving the world; explosions and lost limbs are learning experiences, and there are no failures, only temporary setbacks.
  - SOE lore, race-gnome: "Gnomes believe that building contraptions ranks right up there with saving the world."
  - SOE lore, race-gnome: "They do not believe in failures, only temporary setbacks."
- **GNOME-3** Gnomes do not waste time on revenge; it cannot be helped if an experiment goes bad on someone who offended them.
  - SOE lore, race-gnome: "Not ones to waste time on such things as revenge, gnomes prefer to experiment. It cannot be helped if an experiment happens to go bad on someone who just offended them in some way."
- **HALF-1** Halflings live as they did hundreds of years ago, simple farmers and craftsmen who care little for the rest of the world; their worries are nasty goblins, visiting relatives and ruined pot roasts.
  - SOE lore, race-halfling: "Halflings today exist much as they did hundreds of years ago. Content to live the simple lives of farmers and craftsman, the Halflings care little for the doings of the rest of the world."
  - SOE lore, race-halfling: "Their worries continue to consist of nasty goblins, visiting relatives, and ruined pot roasts"
- **HALF-2** Rivervale is a quiet hamlet in the very middle of Tunaria; halflings love travel and adventure, and defend Rivervale and the Misty Thicket with all their might.
  - SOE lore, race-halfling: "Halflings hail from Rivervale, a quiet little hamlet smack dab in the center of Tunaria."
  - SOE lore, race-halfling: "they defend Rivervale and Misty Thicket with all of their might"
- **HUMAN-1** In the west Antonius Bayle II has been crowned King of the city-state of Qeynos, and its farming people look to prosperity and safety under him.
  - SOE lore, race-human: "In the West, Antonius Bayle II has been crowned King of the city-state of Qeynos. The farming people of Qeynos look forward to greater prosperity and safety under his reign."
- **HUMAN-2** In the east the new merchant city of Freeport rings with gold changing hands, not always honorably; corruption and deceit thrive in it.
  - SOE lore, race-human: "To the East, the new merchant city of Freeport chimes with the sound of gold changing hands, sometimes by less than honorable means. Corruption and deceit thrives in this new city of opportunity."
- **HUMAN-3** Inland, Highpass carves its living out of the Serpent Spine Mountains and commands the only pass through them; its rulers can gather immense wealth, and what they mean to do with it worries everyone.
  - SOE lore, race-human: "Farther inland the city of Highpass carves its existence out of the Serpent Spine Mountains. Commanding the only pass through the impossible mountains, the rulers of Highpass have the opportunity to gather immense resources. What those resources will be used for is a worry for all."
- **HUMAN-4** Humans are the most numerous people of Norrath (some say only rats outnumber them), open-minded, and found in every trade but the shaman's.
  - SOE lore, race-human: "Humans are the most abundant race in Norrath. Some would say only rats outnumber them"
  - SOE lore, race-human: "Humans are an open-minded lot, and they accept almost any other race."
- **HUMAN-5** In Freeport the Church of the Marr twins is only now beginning its crusade against tyranny, for the few good people the city has.
  - SOE lore, race-human: "in EQoA's timeline, the Church of the Marr Twins is just beginning their crusade to remove tyranny from the lives of what few good people can be found in Freeport"
- **HUMAN-6** Humans of the east are drawn to the paths of shadow and the forbidden dark arts; humans of the west are the most versatile in magic, faith and the sword.
  - WORDS.TXT 454: "Humans of the east tend to be drawn into the paths of the shadow, often binding themselves to the forbidden dark arts."
  - WORDS.TXT 453: "Humans of the west are the most versatile in their innate knowledge of magic, faith, and the sword."
- **TROLL-1** Ykesha, the greatest troll leader who ever lived, is dead; the frogloks have retaken the caves of Guk, and Grobb is now the strongest community of trolls on Norrath.
  - SOE lore, race-troll: "Ykesha, the greatest Troll leader to ever live, is now dead. The frogloks have retaken the caves of Guk and suddenly the city of Grobb has emerged as the most powerful community of Trolls on Norrath."
- **TROLL-2** The trolls mean to widen their lands; their new alliance with the dark elves is only the start. They would eat dark elves too if the dark elves let them.
  - SOE lore, race-troll: "Quick to capitalize on their newfound power, the Trolls seek to expand their territory and influence. Their recent alliance with the Dark Elves is just the beginning."
  - SOE lore, race-troll: "If they could get away with it, they would dine on dark elves too (dark elves do not let them get away with it)."
- **TROLL-3** Trolls delight in killing, eating what they kill, and killing again; now and then one with a little wisdom becomes a shaman.
  - SOE lore, race-troll: "Trolls bask in the glory of killing, eating their kills, and killing some more."
  - SOE lore, race-troll: "Occasionally a troll with more than a minimum level of wisdom may become a shaman."
- **OGRE-1** Ogres are large, powerful and warlike, and never hesitate to attack whoever stands against them.
  - WORDS.TXT 375: "Ogres are a large and powerful race.  Aggressive and warlike, they never hesitate to attack those who stand against them."

## Rosethorn Spire (Aros'Thalinor)

- **SPIRE-1** Rosethorn Spire, Aros'Thalinor in the elven tongue, is older than almost anything mortals built; no one knows its age, or even whether elves raised it.
  - SOE lore, chapter-1: "It was difficult to find any buildings of mortal make older than the Rosethorn Spire (or Aros`Thalinor in the Elven tongue). None knew its exact age; in fact, no one was even certain if it was constructed by the Elves."
- **SPIRE-2** The elves say they were the first people to wield wizardry, and point to the Spire as proof.
  - SOE lore, chapter-1: "It had been long argued among the sentient races of Norrath that the Elves, the children of Tunare, were the first of the races to wield the art of wizardry."
- **SPIRE-3** The elves shaped its black stone by magic and carved vaults beneath it that hold the artifacts and the learning of the Koada'Dal wizards.
  - SOE lore, chapter-1: "They used their powerful magic to guide the black stone into useful shapes."
  - SOE lore, chapter-1: "creating the many vaults that hold much of the artifacts and knowledge of the Koada`Dal wizards"
- **SPIRE-4** Digging there the elves found a strange room of the same black stone with carvings foreign to them, a sarcophagus and an arch of a stone found nowhere else in the land; the riddle held their scholars nearly a century.
  - SOE lore, chapter-1: "the elves found a strange room carved from the same black stone, but of vastly different architecture. The style of the wall carvings was completely foreign to the exploring elves and had stumped all scholars. In addition to the room, a sarcophagus and an arch were found."
  - SOE lore, chapter-1: "The mystery of the tomb held the attention of many elven scholars and historians for nearly a century."
- **SPIRE-5** The Teir'Dal raised a legion of dead to take the Spire; its masters met them with the greatest firestorm the world had seen, and the bodies choked the Lifire river.
  - SOE lore, chapter-1: "Through the necromantic arts, a legion of dead warriors was raised to assault Aros`Thalinor."
  - SOE lore, chapter-1: "From atop the Spire, the wizards rained down the greatest firestorm that the world had ever seen."
  - SOE lore, chapter-1: "or floating bodies in the Lifire river threatening to choke and flood the area"
- **SPIRE-6** The necromancers laughed last: they had poisoned the mana itself, the weave all magic draws on, and left the wizards to die. All but four of the Spire's wizards lie dead in their tombs, and the roses around it are dry and withered.
  - SOE lore, chapter-1: "to taint mana itself, the aether, the weave that all magic drew life from"
  - SOE lore, chapter-1: "All but four of the wizards of the Spire were dead in their tombs."
  - SOE lore, chapter-1: "even though the wild rosebushes that once surrounded it were now dry and withered"
- **SPIRE-7** The Spire is still a place of learning: wizards travel from afar to be taught there, Teir'Dal wizards too, though necromancers are reviled; the dying masters send visiting wizards into the vaults the necromancers keep them from.
  - SOE lore, chapter-1: "Many wizards still traveled from afar to learn from the potent keepers of the Spire. Even Teir`Dal wizards were welcomed, though necromancers and the art of necromancy were reviled."
  - SOE lore, chapter-1: "Instead the wizards relied on adventurous wizards from other lands to claim the books and artifacts from the vaults."
- **SPIRE-8** Rosethorn Spire was once a great center of elven magic in the Elddar Forest and is now a den of necromancy and corruption.
  - Prima, factions: Necromancers of Rosethorn Spire: "A once great center of elven magic within the Elddar Forest. Now a den of Necromancy and corruption."

## Takish'Hiz and the Tomb of the Crown (Holciel Rowen'dal)

- **TOMB-1** Takish'Hiz built its tomb and catacombs to the west, within the Serpent's Spine: Holciel Rowen'dal, which is roughly "Tomb of the Crown". Lyriam Kaelean, guildmaster of the College of High Magic in Fayspires, wrote its history down.
  - SOE lore, chapter-2: "Off to the west, within the Serpent's Spine, the tomb and catacombs of Takish`Hiz were built. It was named Holciel Rowen`dal, roughly translating to Tomb of the Crown."
  - SOE lore, chapter-2: "as documented by Lyriam Kaelean,"
  - SOE lore, chapter-2: "Guildmaster of the College of High Magic, Fayspire"
- **TOMB-2** For hundreds of years the honored dead of Takish'Hiz, commoner to royalty, each caste in its own place, were carried there by family and servants in great processions along the Road of the Dead.
  - SOE lore, chapter-2: "carried there by family members and servants along the Road of the Dead in great processions. The tomb was divided among the social castes and each elf had his place."
- **TOMB-3** When the forest of Elddar fell to the burning sands, most of the elves of Takish'Hiz left Tunaria for new lands, fleeing the spiteful god who envied Tunare; many stayed, believing the forest could be restored, and the sands buried the Road of the Dead.
  - SOE lore, chapter-2: "After the forest of Elldar fell to the advance of the burning sands, most of the elves of Takish`Hiz left Tunaria for new lands, desperate to escape the wrath of the spiteful god who envied Tunare and her creations."
  - SOE lore, chapter-2: "Many remained, believing that the forest could be restored to its former glory."
  - SOE lore, chapter-2: "even though the sands buried the Road of the Dead, the elves still remembered its path"
- **TOMB-4** The elves' attempts to restore Elddar failed and their numbers dwindled; then the Teir'Dal came and made war on Takish'Hiz. With Al'Karad's defeat the city was doomed, and those who could not escape were slaughtered.
  - SOE lore, chapter-2: "It was then that the Teir`dal appeared and made war on Takish`Hiz, the home of their ancient enemies. With Al`Karad's defeat, Takish`Hiz was doomed."
  - SOE lore, chapter-2: "the tide of darkness that were the Teir`dal overcame the elves, slaughtering all who could not escape"
- **TOMB-5** The dark elves looted and defiled the city, and its treasure was, and still is, shipped to Neriak; they and greedy human nomads from the north loot the catacombs too.
  - SOE lore, chapter-2: "The dark elves began the looting and defiling of the ancient elf city. Treasure was, and still is, being shipped to Neriak."
  - SOE lore, chapter-2: "as did the greedy and opportunistic human nomads from the north"
- **TOMB-6** The dead there do not rest: the spirits of the Kings and Queens haunt the halls, and still treasure hunters come from afar.
  - SOE lore, chapter-2: "The dead were not always restful, and the spirits of the Kings and Queens still haunted the halls. And now, to this day, treasure hunters come from afar to take their chances in the catacombs"
- **TOMB-7** The way to the tomb crosses the Desert of Ro to Scorchstone Gorge, green once and hidden from travelers, withered by Solusek Ro's curse on the forest; beasts, rival treasure hunters and the wandering dead hold it now.
  - SOE lore, chapter-2: "Fearsome creatures of all sizes lurk in the Desert of Ro; one can only hope that they are few when crossing it in search of the gorge that leads to the tomb. Scorchstone Gorge was once not so scorched."
  - SOE lore, chapter-2: "Solusek Ro's curse of the forest also affected the gorge."
- **TOMB-8** With Takish'Hiz and its people gone, nothing ties the spirits to who they were; they have grown lost and bitter, and the bodies of adventurers who fall there rise to guard the tombs.
  - SOE lore, chapter-2: "Without Takish`Hiz and the elven people to link the spirits to their former selves, they have become lost and bitter."
  - SOE lore, chapter-2: "The bodies of unfortunate adventurers are risen and guard the tombs from intrusion."
- **TOMB-9** Rumors, lies and half-truths about where the Tomb of the Crown lies and what it holds go round many circles of Tunaria.
  - SOE lore, chapter-2: "Rumors, lies and half-truths abound in many circles on Tunaria regarding the whereabouts and details of the Tomb of the Crown."
- **TOMB-10** Dark elf forces under General Torvek hold what was Takish'Hiz; the last of its elves fight on for vengeance under Cyrian Truearrow, and the somber elves of Tak'Xiv made a last stand to save the Elddar Forest.
  - Prima, factions: Raiders of Takish`Hiz: "Dark Elven forces occupying the area of Takish`Hiz. Led by General Torvek."
  - Prima, factions: Remnant Forces of Takish`Hiz: "The remaining Elves of Takish`Hiz fighting for vengeance. Led by Cyrian Truearrow."
  - Prima, factions: Elves of Tak`Xiv: "The somber elves who have made a last stand to save the now destroyed Elddar Forest."
- **TOMB-11** A sacred tree, guarded by beasts, still keeps one grove of the old forest alive in the middle of the desert.
  - Prima, factions: Guardians of the Grove: "These animals guard the sacred tree which preserves a sacred grove of forest in the middle of the desert."
- **TOMB-12** The Spire fell into peril when the Teir'Dal advanced on elves already suffering under the jealous hands of Solusek Ro, whose doing the death of the Elddar forest was.
  - SOE lore, chapter-1: "The advance of the embittered Teir`Dal proved to be too strong for the elves, especially after suffering under the jealous hands of Solusek Ro."
  - SOE lore, chapter-1: "the destruction of the Elddar forest fell upon the Elven nation"

## The Isle of Dread

- **DREAD-1** The Faceless has more realms than his Plane of Fear, and the Isle of Dread is one: a reward for his most faithful after death, perhaps, or a trap to draw fools into endless terror.
  - SOE lore, chapter-3: "The Isle of Dread is one such incarnation. Perhaps a land to reward the most faithful of The Faceless' followers in their afterlife or, perhaps a simple trap to lure foolish mortals into endless terror for the glory of Cazic Thule."
- **DREAD-2** All that is known of the isle comes from the ramblings of madmen washed ashore after days lost at sea.
  - SOE lore, chapter-3: "so far the only word of it comes from the ramblings of madmen washed ashore after days lost at sea"
- **DREAD-3** A statue of Cazic-Thule stands on the Isle of Dread, a foothold on Tunaria from the Plane of Fear, and he hosts trials there for adventurers; only the most worthy are meant to pass.
  - SOE lore, side-story-3: "They were drawn to the statue of Cazic Thule on the Isle of Dread, a foothold on Tunaria from the Plane of Fear."
  - SOE lore, side-story-3: "My trials were to be impossible to all but the most worthy and powerful adventures."
- **DREAD-4** Graumord is a nightmarish village of lost souls, known only in the terrifying dreams of all peoples.
  - Prima, factions: Citizens of Graumord: "A nightmarish village of lost souls known only in the terrifying dreams of all humanoids."
- **DREAD-5** The Isle of Dread lies in the Dread Sea off Tunaria's north-east, beyond the sea east of the Fayspires' country.
  - WORDS.TXT 1175: "Isle of Dread"
  - WORDS.TXT 1152: "W Dread Sea"
  - zone grid: Isle of Dread lies 2 zones north-east of NE Mountain Boundary

## The plague

- **PLAGUE-1** A deadly plague has lately broken out across Norrath and keeps spreading; for all the work of adventurers and the kindly Healers of Springview, no cure has been found.
  - SOE lore, side-story-1: "The deadly plague that recently erupted onto the scene in Norrath continues to grow and spread. Despite the best efforts of countless adventurers and the benevolent Healers of Springview, a cure has not been found."
- **PLAGUE-2** Creatures the plague killed are rising from their shallow graves as undead, and no one yet knows the plague's source.
  - SOE lore, side-story-1: "creatures who fell prey to the disease and perished are rising from their thin graves to menace the land as undead"
  - SOE lore, side-story-1: "What dangers lie ahead in uncovering the source of this plague"
- **PLAGUE-3** Bertoxxulous is the Plague King, Lord of Decay; a hidden temple of his stands even in Klick'Anon, though his followers there are few.
  - SOE lore, side-story-13: "This secret cavern holds the gnomish temple of Bertoxxulous, the Lord of Decay. Here, the evil and destructive influences of the Plague King are met with gnomish ingenuity and creativity."
  - SOE lore, side-story-13: "Fortunately the followers of Bertoxxulous are few"

## The were-folk of the Unkempt

- **WERE-1** The were-folk began with pacts the ancient elven druid Wegadas made with the Lujien, the wolfmen, and the Gihjna, the bearmen, of the Unkempt, before Takish'Hiz fell, when humans were still little more than barbaric tribes.
  - SOE lore, chapter-4: "Lycanthropes are the result of spiritual rituals and pacts made by the Lujien (wolfmen) and Gihjna (bearmen) with the ancient elven druid Wegadas."
  - SOE lore, chapter-4: "Wegadas made his pact with the Lujien and Gihjna before the fall of Takish'Hiz when the humans were still little more than barbaric tribes."
- **WERE-2** Wegadas gave two unborn children to be raised by them; in the Valley of the Dawn their mortal spirits were taken, and they became Levandius the Wolf Father and Dorandas the Bear Father.
  - SOE lore, chapter-4: "In the Valley of the Dawn, the children's mortal spirits were taken from them."
  - SOE lore, chapter-4: "They would become Levandius the Wolf Father, and Dorandas the Bear Father."
- **WERE-3** Levandius has lived in the Unkempt Forest for centuries, fighting all who trespass; his blood mixed with those he fought and the curse spread. His werewolf warriors are the Night Reapers, under the silver crescent on a black circle, the sign of Drinal the Silver Reaper.
  - SOE lore, chapter-4: "For centuries Levandius had lived within the Unkempt Forest and fought fiercely against those who trespassed into the sacred woods"
  - SOE lore, chapter-4: "became warriors known as the Night Reapers and adopted the standard of the silver crescent on a black circle, the sacred symbol of Drinal the Silver Reaper"
- **WERE-4** The barbarians of the north still fear the Unkempt woods and believe their forefathers' tales of its wolfmen; the people of Qeynos and its villages tell the same tales, and many pass them off as legend and superstition.
  - SOE lore, chapter-4: "The barbarians of the north still fear the Unkempt woods, and believe their forefathers' tales of the wolfmen of the Unkempt. The humans of Qeynos and its outlying villages still tell similar folktales passed down from their less civilized ages, but many pass them off as legend and primitive superstitions."
- **WERE-5** Wegadas is dying, and his heirs are at war for the leadership of the Unkempt; the Night Reapers watch the wood's borders closer than ever and destroy werewolves they judge unworthy of the gift.
  - SOE lore, chapter-4: "In the Unkempt woods, Wegadas is passing away and his heirs are at war for leadership of the Unkempt."
  - SOE lore, chapter-4: "the Night Reapers are watching the borders of the Unkempt Woods closer than ever"
  - SOE lore, chapter-4: "they seek and destroy independent werewolves deemed unworthy of their willing or unwilling gift"
- **WERE-6** For three generations the lords of Castle Lightwolf have kept their lycanthropy secret from all but their most trusted advisors and soldiers, and governed their pack in secret; Levandius leaves this pack be and uses Count Lightwolf to help guard the northern edge of the Unkempt.
  - SOE lore, chapter-4: "For three generations, the lords of Castle Lightwolf have struggled to keep their lycanthropy a secret from all but their most trusted advisors and soldiers, while secretly governing their packs."
  - SOE lore, chapter-4: "Levandius uses Count Lightwolf and his pack to help defend the outskirts of the northern Unkempt Woods"
- **WERE-7** Wolfweres have no human form and no mastery of themselves; when the moon of Drinal is full they all take the wolfman's shape and hunt, bloodthirsty, from dusk until dawn. Lone wolves and wolfweres are behind most werewolf attacks outside the Unkempt.
  - SOE lore, chapter-4: "When the moon of Drinal is full, the wolfweres all shift into their Lujien form and go on a bloodthirsty hunt from dusk until dawn."
  - SOE lore, chapter-4: "These lone wolves and wolfweres are responsible for most werewolf attacks and the resulting legends amongst the civilized races outside of the Unkempt Woods."
- **WERE-8** Dorandas's werebears are the Wards of Dawn, under a golden half circle on black for the rising and setting sun; fierce when forced to fight, they are spirit guides and healers, and few werebears have ever lived outside the Unkempt.
  - SOE lore, chapter-4: "became known as the Wards of Dawn and adopted the standard of a golden half circle on black representing the rising and setting sun"
  - SOE lore, chapter-4: "the werebears of the Wards of Dawn served the human Unkempt tribes primarily as spiritual guides and healers; thus, few werebears have ever existed beyond the boundaries of the Unkempt Woods"
- **WERE-9** The Werebear Witchking was a tribesman of the Unkempt, infected while tending a wounded bear; he fled, gathered other masterless were-creatures in a remote part of the wood, and means to overthrow Levandius and Dorandas once Wegadas is gone.
  - SOE lore, chapter-4: "He was infected with lycanthropy while tending to a wounded bear."
  - SOE lore, chapter-4: "The Witchking has become intent on overthrowing Levandius and Dorandas and assuming control of the Unkempt once Wegadas has passed."
- **WERE-10** Among the barbarian tribes of the north the bear's curse is second only to the wolf's; the great Anu warrior Ungar Glacierfist was rumored to be a werebear, and tales of heroes with a bear's strength and wisdom are common in the north.
  - SOE lore, chapter-4: "The great Anu warrior, Ungar Glacierfist was rumored to be a werebear. Myths of barbarian figures possessing the strength and wisdom of a bear are not uncommon amongst barbarian folk tales."
- **WERE-11** Bearweres are hard to tell from bears, but at dusk and dawn they take the bearman's shape and can be seen standing or walking upright; they have been sighted as far as the Commonlands.
  - SOE lore, chapter-4: "Bearweres aren't easily recognizable from normal bears aside from the fact that they tend to assume their Gihjna form at dusk and dawn, and can be seen standing or walking upright."
  - SOE lore, chapter-4: "sightings of bearweres occur as far away as the Commonlands"
- **WERE-12** Corindas, another son of Wegadas, went south to the Plains of Karana and worked with the druids of Surefall Glade and the villages round the still young city of Qeynos; he lived among the Kejrahn, the lionmen, fought the gnolls beside them, and was healed and made a werelion by the spirit Sateb Mahlni. His werelions are the Sun Manes.
  - SOE lore, chapter-4: "He worked there with the local human druids in Surefall Glade and the villages surrounding the still young city of Qeynos."
  - SOE lore, chapter-4: "He also fought against the Kejrahn's mortal enemies, the gnolls."
  - SOE lore, chapter-4: "Corindas and his loyal werelions became known as The Sun Manes."
- **WERE-13** The Kejrahn lionmen are dwindling, and many have fled Tunaria for the lands of their cousins the Kerran, on Odus; the Sun Manes fight the poachers, the plowmen and the spreading human cities that press on the plains.
  - SOE lore, chapter-4: "Many of the Kejrahn have already fled their ancestral Tunarian homeland for the lands of their cousins, The Kerran, on Odus."
  - SOE lore, chapter-4: "struggle to be fought against the encroaching poachers and agriculturists, and expanding human cities"
- **WERE-14** Werelions hunt down and kill any werewolf, bearwere or ratwere they meet on the Plains of Karana; the wererats of Qeynos seldom go out on the plains and keep to the human roads.
  - SOE lore, chapter-4: "hunt down and kill any werewolves, wolfweres, bearweres, or ratweres encountered in the Plains of Karana"
  - SOE lore, chapter-4: "the wererats residing in Qeynos rarely venture into the plains and tend to stick to the human roads"
- **WERE-15** Wegadas's son Yindrius was stolen as a baby by the Ratonga, masters of stealth living under Qeynos, and made the first wererat by their god Caertex; the Ratonga then vanished into the Underfoot, and his spy network, the Topi di Ombra, forgot its old masters.
  - SOE lore, chapter-4: "However, the abductors were the Ratonga, masters of stealth and deception. A Ratonga spy living beneath Qeynos"
  - SOE lore, chapter-4: "Unfortunately the Ratonga vanished into the Underfoot from which they came not long after Yindrius established his network of spies dubbed the Topi di Ombra."
- **WERE-16** The Topi di Ombra are a powerful family of wererats under Yindrius, chiefly in Qeynos, Neriak, Freeport and outlying villages, with a few among the dwarves and gnomes of Moradhim and Klick'Anon; they live by smuggling, theft, racketeering, extortion and murder, and a new one is made only by leave of a city's gang boss.
  - SOE lore, chapter-4: "Their members are located primarily in Qeynos, Neriak, Freeport, and several outlying villages. The Topi Di Ombra also have a minor presence amongst the dwarves and gnomes in Moradhim and Klik'Anon."
  - SOE lore, chapter-4: "The Topi di Ombra survives and prospers through smuggling, stealing, racketeering, extortion, assassination, and other such illicit activities."
  - SOE lore, chapter-4: "New Topi di Ombra can only be "made" with the permission of a gang boss"
- **WERE-17** An orphan wererat the Topi di Ombra find is offered a place in the family and killed if he refuses. Rumor tells of a family of dwarven and gnomish wererats in the north-eastern mountains between Moradhim and Klick'Anon, and of Teir'Dal wererats out of the Desert of Ro.
  - SOE lore, chapter-4: "When orphan wererats are found by the Topi di Ombra, they are offered a chance to join the family. If they refuse, they are executed."
  - SOE lore, chapter-4: "There are rumors of one such orphan family of dwarven and gnomish wererats operating out of the northeastern mountains of Tunaria between Moradhim and Klik'Anon."
  - SOE lore, chapter-4: "a family of teir'dal wererats operating out of the Desert of Ro"
- **WERE-18** Rumor tells of a pride of Erudite werelions on Odus whose matriarch came from Highbourne.
  - SOE lore, chapter-4: "There are rumors of an Erudite pride of strays on Odus, their matriarch having originally been from Highbourne"
- **WERE-19** Beasts take the curse too: feral weres keep an animal's mind and are behind most of the tales of blood-mad were-creatures.
  - SOE lore, chapter-4: "Feral lycanthropes retain their animal intelligence and instincts, thus are responsible for much of the fear and horror stories about lycanthropes with an uncontrollable thirst for blood and violence."
- **WERE-20** The Werebear Witchking and his cultists camp near Zentar's Keep, where werewolves prowl too; ancient treants of the Unkempt Wood try to drive every invader out of it.
  - EQOA-Data map: Werebear Witchking
  - EQOA-Data map: Witchking cultists
  - Prima, factions: The Werebear Witchking: "The Werebear Witchking and his cultists."
  - Prima, factions: Treants of the Unkempt Wood: "These ancient treants seek to drive out all invaders from the Unkempt Wood."
- **WERE-21** The Lujien, the Night Reapers and the Unkempt worship Drinal the Silver Reaper, spirit of night, destruction and death; Dorandas was gifted by Ehayae, Matron of the Dawn, spirit of birth, rebirth and creation.
  - SOE lore, chapter-4: "gifted by the spirit of Drinal the spirit deity of night, destruction, and death"
  - SOE lore, chapter-4: "gifted by the spirit of Ehayae, Matron of the Dawn and spirit deity of birth, rebirth, and creation"
  - SOE lore, chapter-4: "The Night Reapers, like the Unkempt and the Lujien are fervent worshipers of Drinal the Silver Reaper."

## The land and its ways

- **LAND-1** Every creature of Tunaria is bound to the land and is reborn after it dies. Spiritmasters have learned to link the spirit to the land: one binds you to where you stand, and there you are reborn. Most cities have several.
  - Frontiers manual: "creatures of Tunaria are tightly bound to the land. When a creature dies, it is reborn after a short time."
  - Frontiers manual: "Spiritmasters have learned to link the spirit to the land. They can bind your character to the land that you are standing on, making it the place where you are reborn if you die."
- **LAND-2** Coachmen run Tunaria's coaches between their stables and other stables that are affiliated with them, and take you only to a stable whose ledger you have signed. Most cities have a coachman.
  - Frontiers manual: "These are the folks that handle the coach services of Tunaria. Most cities have a coachman. The coachmen provide transportation between their stables and other affiliated stables."
  - Frontiers manual: "In order to get transportation to a stable you will need to sign the coachman's ledger for that stable."
- **LAND-3** Money in Tunaria is coins called Tunar.
  - Frontiers manual: "Money in Tunaria comes in the form of coins called Tunar."
- **LAND-4** Tunaria is full of rival factions: dark elves fight humans, humans fight ogres, and nobody really understands what dwarves want.
  - Frontiers manual: "There are many competing factions in Tunaria. Dark elves fight humans, humans fight ogres, and nobody really understands what dwarves want."
- **LAND-5** A banker keeps your goods and Tunar in his vault, and one account answers at any banker in any city.
  - Frontiers manual: "Your character has one bank account, and can access that bank account from any banker in any city."
- **LAND-6** Gypsies travel all over Tunaria selling valuable wares to those who seek them out; an underground society of bandits works all over Tunaria too.
  - Prima, factions: Gypsies of Tunaria: "A small society of gypsies that travel all over Tunaria and offer to sell valuable wares to those who seek them."
  - Prima, factions: Bandits of Tunaria: "An underground society of bandits who operate all over Tunaria."
- **LAND-7** The age the world is in is called the Age of Adventure.
  - SOE lore, chapter-2: "To help you better understand EQOA's setting in the Age of Adventure"
- **LAND-8** A cult of mad humans worships dragons, whatever the plain danger of it.
  - Prima, factions: Cult of the Dragon: "A cult of insane humans dedicated to the worship of Dragons, regardless of the obvious dangers involved."
- **LAND-9** Lady Vox is the terrible and legendary ice dragon of the north, said to lead legions of goblins and giants.
  - Prima, factions: Lady Vox: "The terrible and legendary ice dragon of the North. Said to lead legions of goblins and giants."
- **LAND-10** Siliskor is a loathsome black dragon that terrorizes the swamps of southern Tunaria.
  - Prima, factions: Siliskor: "The loathsome black dragon that terrorizes the swamps of Southern Tunaria."
- **LAND-11** The monks of the Endless Tower guard a sacred stairway said to lead to another plane, where genies rule over all.
  - Prima, factions: Monks of the Endless Tower: "The monks that protect a sacred stairway said to lead to an alternate plane where genies rule over all."
- **LAND-12** Radavex Planeswalker is a famous wizard known for his long travels to the outer planes.
  - Prima, factions: Radavex Planeswalker: "A famous wizard known for his extensive travels to the outer planes."
- **LAND-13** Kelinar is a society of Tunaria's most powerful wizards.
  - Prima, factions: Citizens of Kelinar: "A society of Tunaria's most powerful practitioners of wizardry."
- **LAND-14** Fire giants and a tribe of kobolds live in Solusek's Eye, and goblins hold the Lavastorm Mountains.
  - Prima, factions: Fire Giants: "Giants found in Solusek's Eye."
  - Prima, factions: Fire Pit Kobolds: "Kobold tribe within Solusek's Eye."
  - Prima, factions: Solusek Goblins: "The main goblin faction for Lavastorm Mountains."
- **LAND-15** At Fort Alliance several of the light races have banded together and live in harmony.
  - Prima, factions: Fort Alliance: "Outpost in Fort Alliance. Several light races have banded together and live harmoniously here."
- **LAND-16** The thralls of Xorbb, the prophet of the Evil Eyes, are called the Eye.
  - Prima, factions: The Eye: "The thralls and servants of Xorbb, the prophet of the Evil Eyes."
- **LAND-17** Deep in a cave full of snakes, some think, an unknown snake-god is worshipped.
  - Prima, factions: Serpent's Den: "It is theorized that this cave is dedicated to some unknown snake-god, due to the large number of snakes that infest it."
- **LAND-18** Cyclops guards watch Urglunt's Gate, one of the few passes into the Rathe Mountains.
  - Prima, factions: Gate Guards of Urglunt: "The Cyclops guards that watch over Urglunt's Gate, one of the few passes into the Rathe Mountains."
- **LAND-19** Vampire hunters camp by the evil Jaeger Mansion, a dark house on the edge of the Unkempt Wood, hoping to end all vampirism in those parts.
  - Prima, factions: Vampire Hunters: "The group of adventurers who camp near the evil Jaeger Mansion. They hope to bring all vampirism in the region to an end."
  - Prima, factions: Jaeger Mansion: "A dark and forboding mansion built on the edge of the Unkempt Wood."
- **LAND-20** Ferran's Hope, once a thriving human village in the Serpent Hills, is a charred husk.
  - Prima, factions: Citizens of Ferran's Hope: "The charred husk of a human village that once flourished in the Serpent Hills."
- **LAND-21** Centaurs live on the southern Plains of Karana.
  - Prima, factions: Centaurs of the Plains: "A society of Centaurs that live on Southern Plains of Karana."
- **LAND-22** The Stone Watchers are a wholly neutral people.
  - Prima, factions: Stone Watchers: "The Stone Watchers are a completely neutral people."
- **LAND-23** The Deathfist are the largest horde of orcs known in the world, and live by raiding and pillaging the caravans that cross the Desert of Ro; ogres and trolls have signed on with them.
  - Prima, factions: Deathfist Hordes: "The largest horde of orcs known to exist in the world. They live by the raiding and pillaging of caravans traveling through the Desert of Ro."
  - Prima, factions: Deathfist Mercenaries: "Ogres and Trolls that have signed on with the Horde."
- **LAND-24** The black orcs of the Hatebone are found through most of eastern Tunaria.
  - Prima, factions: Hatebone Orcs: "Black orcs of the Hatebone can be found throughout most of Eastern Tunaria."
- **LAND-25** Old ruins of the Combine lie in the Green Rift.
  - Prima, factions: House Gal`Saris: "excavating the Combine ruins found in the Greenrift"
  - WORDS.TXT 1155: "The Green Rift"
