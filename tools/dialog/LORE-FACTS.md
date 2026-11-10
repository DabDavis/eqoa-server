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

## Qeynos and the west

- **QEY-1** Qeynos is the greatest human city on Tunaria, ruled by the lately crowned Antonius Bayle II.
  - Prima, factions: Citizens of Qeynos: "The greatest Human city found on Tunaria, ruled by the recently crowned, Antonius Bayle II."
- **QEY-2** Hagley is a small village just east of Qeynos, under that city's protection; Blakedown is a farming community of good-hearted humans under Qeynos's protection too.
  - Prima, factions: Citizens of Hagley: "A small village just to the east of Qeynos, and under that city's protection."
  - Prima, factions: Citizens of Blakedown: "A farming community inhabited by good-hearted humans under the protection of Qeynos."
- **QEY-3** Talamir's Regiment was an old regiment of the Qeynos Guard that Antonius Bayle II disbanded for its ruthless tactics; its men wander the land as outlaws.
  - Prima, factions: Talamirs Regiment: "An old regiment of the Qeynos Guard, disbanded by Antonious Bayle II for their ruthless tactics. They now wander the land as outlaws."
- **QEY-4** The prisoners of Qeynos Prison, south of the city, have taken the prison over and are hostile to all but bards and rogues; and a new cult of Rodcet Nife is forming in its north-east corner.
  - Prima, factions: Qeynos Prisoners: "Faction for prisoners in Qeynos Prison that have taken over the prison. KOS to all except bards and rogues."
  - Prima, factions: Followers of Rodcet Nife: "New cult forming in the North East Corner of Qeynos Prison."
  - zone grid: Qeynos Prison lies 1 zone south of Qeynos
- **QEY-5** The Servants of the Sickle are a cult of necromancers and shadowknights who live in a tomb near Blakedown.
  - Prima, factions: Servants of the Sickle: "A cult of necromancers and shadowknights living in a tomb near Blakedown."
- **QEY-6** The guilds of Qeynos: the monks' Order of the Silent Fist, the bards' Qeynos Troupe, the Anagogical Order of wizards, magicians and enchanters, the clerics' Defenders of Erollisi Marr, and the rogues' Heavy Purse.
  - Prima, factions: Order of the Silent Fist: "Qeynos monk guild."
  - Prima, factions: Qeynos Troupe: "Bard's guild of Qeynos."
  - Prima, factions: The Anagogical Order: "Wizard, magician, and enchanters' guild of Qeynos."
  - Prima, factions: Defenders of Erollisi Marr: "Clerics Guild of Qeynos."
  - Prima, factions: The Heavy Purse: "Qeynos rogues guild."
- **QEY-7** Some guards of Qeynos went out on a patrol and became lost.
  - Prima, factions: Lost guards: "Qeynos guards who became lost after a patrol."
- **QEY-8** Crethley Manor, north-east of Qeynos, is an ancient ruined mansion rumored to be haunted by a vampire and his minions.
  - Prima, factions: Crethley Manor: "This ancient and ruined mansion is rumored to be haunted by a vampire and his minions."
  - zone grid: Crethley Manor lies 1 zone north-east of Qeynos
- **QEY-9** A ragtag band of desperate gnolls raids the western regions of Tunaria; the vicious gnolls of Blackburrow hold that dungeon, deadly scarabs live deeper in it than even the gnolls go, and an old dwarven mining guild is trapped there.
  - Prima, factions: Gnoll Raiders: "A ragtag band of desperate gnolls found in the Western regions of Tunaria."
  - Prima, factions: Blackburrow Gnolls: "A tribe of vicious gnolls that inhabit the dungeon of Blackburrow."
  - Prima, factions: Deepscarabs: "The deadly scarabs found deep within the dungeon Blackburrow, deeper than even the Gnolls venture."
  - Prima, factions: Miners Guild 189: "Old Dwarven mining guild trapped in Blackburrow."
- **WYND-1** Wyndhaven is a town of shady humans on the northern edge of the Qeynos city-state; Bogman, north of it, is a ramshackle village of shady barbarians.
  - Prima, factions: Citizens of Wyndhaven: "A town of shady humans living on the northern edge of the Qeynos city-state."
  - Prima, factions: Citizens of Bogman: "A ramshackle village of shady Barbarians found to the north of Wyndhaven."
- **WYND-2** Mariel, the zone north of Wyndhaven, is the only elven outpost left on the western side of Tunaria since Al'Karad fell; Binshore is a barbarian fishing village in Whale Hill, the zone south of Wyndhaven.
  - Prima, factions: Citizens of Mariel: "The only Elven outpost found on the Western side of Tunaria after the fall of Al`Karad."
  - Prima, factions: Binshore: "Barbarian fishing village in Whale Hill."
  - zone grid: Mariel Village lies 1 zone north of Wyndhaven
  - zone grid: Whale Hill lies 1 zone south of Wyndhaven
- **SURE-1** The circle of rangers and druids of Surefall Glade has worked for many years to protect the Jaggedpine Forest; its guards are the Protectors of the Pine.
  - Prima, factions: Citizens of Surefall Glade: "This circle of rangers and druids have worked to protect the Jaggedpine Forest for many years."
  - Prima, factions: Protectors of the Pine: "Surefall Glade guard faction."
- **SURE-2** Jethro's Pike is a fishing village on the shore of a large lake just west of Surefall Glade; the Red Ear gnolls quarrel with its humans, yet live at peace with the protectors of the Jaggedpine.
  - Prima, factions: Citizens of Jethro Pike: "A fishing village built on the shores of a large lake just west of Surefall Glade."
  - Prima, factions: Red Ear Gnolls: "A tribe of gnolls who come into conflict with the humans of Jethro's Pike, yet coexist peacefully with the protectors of the Jaggedpine Forest."
- **SURE-3** Murnf lies north of Surefall Glade and the Fog Marsh south of it; Wymondham, to the east, is a village of Qeynos.
  - zone grid: Murnf lies 1 zone north of Surefall Glade
  - zone grid: Fog Marsh lies 1 zone south of Surefall Glade
  - zone grid: Wymondham lies 1 zone east of Surefall Glade
  - Prima, factions: Citizens of Wymondham: "Qeynos Village in Wymondham"
- **MURNF-1** Murnf is a farming village on the edge of the Unkempt Wood; its simple folk warn people not to enter the forest.
  - Prima, factions: Citizens of Murnf: "A farming village built along the edge of the Unkempt Wood. The simple folk there warn people not to enter the forest."
- **MURNF-2** The Twisted Tower lies west of Murnf and Spirit Talkers Wood east of it.
  - WORDS.TXT 1060: "Twisted Tower"
  - zone grid: Twisted Tower lies 1 zone west of Murnf
  - zone grid: Spirit Talkers Wood lies 1 zone east of Murnf
- **MURNF-3** The Last Smith, Grim Brellhand, is a legendary dwarven smith who lives in seclusion in the Unkempt Wood.
  - Prima, factions: The Last Smith: "Also known as Grim Brellhand, the Last Smith is a legendary dwarven smith living in seclusion in the Unkempt Wood."
- **HONJ-1** Honjour, in the Fog Marsh, is a place of religious significance to the devout troll worshippers of Cazic-Thule.
  - Prima, factions: Citizens of Honjour: "A place of religious significance to devout Troll worshippers of Cazic-Thule."
- **HONJ-2** Surefall Glade lies north of the Fog Marsh, the ruins of Al'Karad east of it and Crethley Manor west of it.
  - zone grid: Surefall Glade lies 1 zone north of Fog Marsh
  - zone grid: Al Karad Ruins lies 1 zone east of Fog Marsh
  - zone grid: Crethley Manor lies 1 zone west of Fog Marsh
- **ZENT-1** Zentar's Keep is a fortress built by Zentar, the legendary knight of darkness; Zentar Reese is its evil leader.
  - Prima, factions: Citizens of Zentar's Keep: "A fortress built by the legendary knight of darkness, Zentar."
  - Prima, factions: Zentar Reese: "Zentar Reese is the evil leader of Zentar's Keep."
- **ZENT-2** Permafrost lies north of Zentar's Keep, the northern Unkempt east of it and Bogman south of it; great ice worms roam near the keep.
  - zone grid: Permafrost lies 1 zone north of Zentars Keep
  - zone grid: Unkempt North lies 1 zone east of Zentars Keep
  - zone grid: Bogman Village lies 1 zone south of Zentars Keep
  - EQOA-Data, 23-47/45: "You need 3 chunks of ice worm meat. Random spawns NW/E of ZK."
- **LIGHT-1** Castle Lightwolf, in the North Wilderlands, is a training center for warriors and is charged with guarding the north from invasion; werewolves prowl the country round it.
  - Prima, factions: Citizens of Castle Lightwolf: "Castle Lightwolf is a training center for warriors and charged with guarding the north from invasion."
  - Prima, factions: Werewolves of Lightwolf: "Faction for the werewolves in the areas surrounding Castle Lightwolf."
  - EQOA-Data, 23-47/45: "Castle Lightwolf - North Wilderlands"
- **LIGHT-2** The castle trusts those who slay the beasts of Lady Vox's castle in Permafrost; ambassadors keep rooms in its towers.
  - EQOA-Data, 23-47/45: "Obtain faction by slaying beasts in Lady Vox's Castle, Permafrost."
  - EQOA-Data, 23-47/45: "Talk to **Ambassador Kizden G'Lux** (first floor, north tower)."
- **LIGHT-3** The village of the Anu lies north of the castle's Wilderlands, the northern Unkempt west of them and the Guardian Forest east of them.
  - zone grid: Anu Village lies 1 zone north of North Wilderlands
  - zone grid: Unkempt North lies 1 zone west of North Wilderlands
  - zone grid: Guardian Forest lies 1 zone east of North Wilderlands

## Freeport and the east

- **FREE-1** Freeport is the largest human city on the eastern coast of Tunaria, and its citizens are driven by wealth.
  - Prima, factions: Citizens of Freeport: "The wealth-driven citizens of the largest human city on the Eastern coast of Tunaria."
- **FREE-2** Merchant House Nothard is the ruling merchant house of Freeport and of the Iron Coffer; Merchant House Slaerin is the city's guild of necromancers.
  - Prima, factions: Merchant House Nothard: "Ruling merchant house of Freeport and the Iron Coffer."
  - Prima, factions: Merchant House Slaerin: "Freeport necromancer Guild."
- **FREE-3** The Crusaders of Marr's Fist keep an outpost that is a bastion of honor and law among the greedy humans of Freeport.
  - Prima, factions: Crusaders of Marr's Fist: "An outpost that acts as a bastion of honor and law among the greedy humans of Freeport."
- **FREE-4** The Towers of Noctix are towers of darkness that corrupt the commonlands west of Freeport; what lies inside them is a mystery.
  - Prima, factions: Towers of Noctix: "The towers of darkness that corrupt the region of the commonlands west of Freeport. What lies inside is a mystery."
- **FREE-5** The Cracktooth are a small clan of orcs that raids the eastern commonlands near Freeport; they lurk near Bobble-by-Water too.
  - Prima, factions: Cracktooth Orcs: "A small clan of orcs that raid the Eastern Commanlands near Freeport."
- **FREE-6** Temby is a small smuggling village just north of Freeport; Hodstock is a village of xenophobic humans north of Freeport on the River Saren, where the retired scholar and explorer Feria Hurkert lives.
  - Prima, factions: Smugglers of Temby: "A small smuggling village situated just north of Freeport."
  - Prima, factions: Citizens of Hodstock: "A village of xenophobic humans found north of Freeport, along the River Saren."
  - Prima, factions: Feria Hurkert: "A retired scholar and explorer now living in the village of Hodstock."
- **FREE-7** The guilds of Freeport: the Academy of Arcane Science for wizards, enchanters and magicians, the bards' Silken Gauntlet, and the Spiteful Shield of shadowknights, clerics and rogues; its guards are the Freeport Militia.
  - Prima, factions: Academy of Arcane Science: "Freeport wizard, enchanter, and magician guild."
  - Prima, factions: Silken Gauntlet: "Freeport bard guild."
  - Prima, factions: The Spiteful Shield: "Freeport shadowknight, cleric, and rogue guild."
  - Prima, factions: The Freeport Militia: "Freeport guard faction."
- **FREE-8** The northern Desert of Ro begins in the zone south of Freeport; humans who wander into the deserts of Ro lose their minds completely.
  - zone grid: Northern Ro lies 1 zone south of Freeport
  - Prima, factions: Madman: "A group of humans who have wandered into the deserts of Ro, only to lose their minds completely."
- **BOB-1** The halflings of Bobble-by-Water live near Nektulos Forest: Neriak's country is the zone north of theirs, the Green Rift lies west and Hodstock south.
  - Prima, factions: Citizens of Bobble-by-water: "Halflings living near Nektulos Forest."
  - zone grid: Neriak Nektulos lies 1 zone north of Bobble-by-Water
  - zone grid: The Green Rift lies 1 zone west of Bobble-by-Water
  - zone grid: Hodstock and Temby lies 1 zone south of Bobble-by-Water
- **TEA-1** People live and work at Muniel's Tea Garden, south along the coast from Freeport; skeleton pirates and a pirate captain hold an island off it.
  - Prima, factions: Muniel's Tea Garden: "Faction for the people who live and work at Muniel's Tea Garden."
  - Prima, walkthrough, line 21040: "There are several skeleton pirates and a pirate captain on the island."
- **TEA-2** The Deathfist citadel is the zone west of Muniel's Tea Garden, the desert of north-western Ro lies north of it and the ruins of Al Farak south of it.
  - zone grid: Deathfist Citadel lies 1 zone west of Muniels Tea Garden
  - zone grid: Northwestern Ro lies 1 zone north of Muniels Tea Garden
  - zone grid: Al Farak Ruins lies 1 zone south of Muniels Tea Garden
- **TEA-3** Sand giants walk the Desert of Ro, and a desert dragon, Skahyir.
  - Prima, factions: Sand Giants of Ro: "Faction used by sand giants."
  - Prima, factions: Desert Dragon Skahyir: "Faction of the Desert Dragon Skahyir."
- **OASIS-1** The Nasehir are aggressive desert nomads who live by robbing and killing whoever passes; the Taksehir Nation, the more light-minded of the desert's bandits, is at war with them. The gypsies of Oasis count the Nasehir their enemies.
  - Prima, factions: Nasehir Cutthroats: "An aggressive group of desert nomads that sustain themselves by robbing and killing whomever passes their way."
  - Prima, factions: The Taksehir Nation: "They are at war with Nahsehir."
  - EQOA-Data, 23-47/37: "Improve gypsy faction via Siloth or Nasehir kills if quest unavailable."
- **OASIS-2** Tak'Xiv lies west of Oasis, the Great Waste east of it, the Eternal Desert north of it and the Sea of Lions south; the ruins of Takish'Hiz lie to the south-west.
  - zone grid: Tak Xiv lies 1 zone west of Oasis
  - zone grid: Great Waste lies 1 zone east of Oasis
  - zone grid: Eternal Desert lies 1 zone north of Oasis
  - zone grid: Sea of Lions lies 1 zone south of Oasis
  - zone grid: Takish Hiz lies 1 zone south-west of Oasis
- **HAZ-1** Hazinak is a dark elf outpost and port, the staging area for shipping the artifacts of Takish'Hiz to Neriak.
  - Prima, factions: Citizens of Hazinak: "A Dark Elf outpost and port used as a staging area for the shipment of artifacts from Takish`Hiz to Neriak."
- **HAZ-2** Guk lies south of Hazinak and the hives of the Slith'Tar west of it; the Slith'Tar are an evil, half-thinking race, hostile to all.
  - zone grid: Guk lies 1 zone south of Hazinak
  - zone grid: Slithtar Hive lies 1 zone west of Hazinak
  - Prima, factions: SlithTar: "Evil race of semi-sentient creatures, KOS to all."
  - EQOA-Data, 23-47/33: "Coach to Hazinak, head west. Many hives, one is larger."

## Highpass and the middle lands

- **PASS-1** The rugged humans of Highpass have carved a lucrative living out of the Serpent's Spine Mountains; its garrison, the Highpass Guard, is tough and well paid, and tolerates the evil races.
  - Prima, factions: Citizens of Highpass: "The rugged humans that have carved out a lucrative existence in the Serpent's Spine Mountains."
  - Prima, factions: The Highpass Guard: "The tough, and well-paid, human garrison that guards the city of Highpass."
  - Prima, factions: Defenders of Highpass: "Tolerant of evil races, neutral to good races"
- **PASS-2** Bastable is a human village east of Highpass, on the great Trade Road; the bards of Highpass are the Royal Players.
  - Prima, factions: Citizens of Bastable: "A human village found east of Highpass, along the great Trade Road."
  - Prima, factions: Royal Players: "Faction for bards of Highpass."
- **PASS-3** Runnyeye Citadel, a place of festering evil on the edge of the Misty Thicket, is held by the goblins of Clan Runnyeye; it lies in the zone north of Highpass Hold, and the Bandit Hills west.
  - Prima, factions: Clan Runnyeye: "The goblins that inhabit Runnyeye Citadel, a place of festering evil found on the edge of Misty Thicket."
  - zone grid: Runnyeye lies 1 zone north of Highpass Hold
  - zone grid: Bandit Hills lies 1 zone west of Highpass Hold
- **VALE-1** Rivervale is the warm and comfortable homestead of the halflings; its guards are the Guardians of the Vale, its druids the Stormreapers, its clerics keep the Temple of Bristlebane, and its rogues are the Deep Pockets.
  - Prima, factions: Citizens of Rivervale: "The warm and comfortable homestead of the Halflings."
  - Prima, factions: The Guardians of the Vale: "Rivervale guard faction."
  - Prima, factions: The Stormreapers: "Rivervale druid guild."
  - Prima, factions: Temple of Bristlebane: "Rivervale cleric guild."
  - Prima, factions: The Deep Pockets: "Rivervale rogue guild."
- **VALE-2** Merry-by-Water is one of the few halfling hamlets of Tunaria, on the shore of the northern Serpent River.
  - Prima, factions: Citizens of Merry-by-water: "One of the few Halfling hamlets of Tunaria, built along the shore of the Northern Serpent River."
- **VALE-3** The Thinhairs are halflings who do not kill necromancers and shadowknights outright, which is remarkable.
  - Prima, factions: The Thinhairs: "outright kill necromancers and shadowknights. Amazing."
- **VALE-4** The Misty Thicket lies west of Rivervale and the northern Kithicor east of it; Loreth Kith is a village of druids and rangers sworn to protect Kithicor Forest.
  - zone grid: Misty Thicket lies 1 zone west of Rivervale
  - zone grid: North Kithicor lies 1 zone east of Rivervale
  - Prima, factions: Citizens of Loreth Kith: "A village of druids and rangers sworn to protect Kithicor Forest from those that would do it harm."
- **DARV-1** Darvar Manor stands in the land called Jared's Blight; Jared is one of Tunaria's most powerful enchanters, and has a tower.
  - WORDS.TXT 1091: "Jared's Blight"
  - Prima, factions: Jared's Tower: "The tower of Jared, one of Tunaria's most powerful enchanters."
- **DARV-2** The Bandit Hills lie east of the manor's land, the ruins of Al'Karad west of it and Blackburrow north; the Black Swan Inn is the first building on the road south of the manor.
  - zone grid: Bandit Hills lies 1 zone east of Jareds Blight
  - zone grid: Al Karad Ruins lies 1 zone west of Jareds Blight
  - zone grid: Blackburrow lies 1 zone north of Jareds Blight
  - EQOA-Data, 23-47/30: "From DM, follow road south, Inn is first building."
- **DARV-3** A dark elf ambassador keeps a front room at Darvar Manor; west of the manor a place is called the Crazed Experiments, and a barrow of cultists, that of Thenon the Wise, lies along the river between the manor and Forkwatch.
  - EQOA-Data, 23-47/45: "**Ambassador Do'Nerua** in Darvar Manor, by coachman, front room."
  - EQOA-Data, 23-47/43: "west area called Crazed Experiments"
  - EQOA-Data, 23-47/43: "Barrow is along the river between Forkwatch and Darvar Manor."
- **FORK-1** Forkwatch is a castle with its guards and citizens and the country round it; gypsies camp north of it.
  - Prima, factions: Citizens of Forkwatch: "Guards and citizens of Forkwatch castle and its surrounding areas."
  - EQOA-Data, 23-47/37: "at gypsy camp north of Forkwatch"
- **FORK-2** The Salt Mine lies east of Forkwatch, held by the Blackscourge river pirates; the South Crossroads, where Dark Solace stands, lie south of it.
  - zone grid: Salt Mine lies 1 zone east of Forkwatch
  - Prima, factions: Blackscourge Pirates: "River pirates in salt mine, KOS to all."
  - zone grid: South Crossroads lies 1 zone south of Forkwatch
  - EQOA-Data, 23-47/35: "**Dark Solace** (South Crossroads)"
- **SOLACE-1** Dark Solace is a safe sanctuary offered to every evil humanoid persecuted by the self-righteous of Tunaria; mindless drones, the Disciples of Plague, fill its front part.
  - Prima, factions: Citizens of Dark Solace: "A safe sanctuary offered to all evil humanoids persecuted by the self-righteous of Tunaria."
  - Prima, factions: Disciples of Plague: "Faction of mindless drones in the front part of Dark Solace."
- **SOLACE-2** Urglunt's Wall lies west of Dark Solace's crossroads, the valley of the centaurs east of them and Widow's Peak south.
  - zone grid: Urglunts Wall lies 1 zone west of South Crossroads
  - zone grid: Centaur Valley lies 1 zone east of South Crossroads
  - zone grid: Widows Peak lies 1 zone south of South Crossroads
- **BLACK-1** Blackwater is a human town bordering a swamp on the Lifire River, in the Brog Fens, where the crazed troll shaman Mog and his followers live.
  - Prima, factions: Citizens of Blackwater: "A human town bordering a swamp along the Lifire River."
  - EQOA-Data, 23-47/37: "**Ralis** in Brog Fens (BW, Black Water)"
  - Prima, factions: Followers of Mog: "Faction for the crazed Troll shaman, Mog, and his followers living in the Brog Fens."
- **BLACK-2** The valley of the centaurs lies north of Blackwater's fens, Fort Alliance south of them, Widow's Peak west and the Serpent Hills east; aviaks, Bluefeather's Brood, live on Widow's Peak, griffons nest in the Serpent Hills, and the humans of Waterspout eke out a living there.
  - zone grid: Centaur Valley lies 1 zone north of Brog Fens
  - zone grid: Fort Alliance lies 1 zone south of Brog Fens
  - zone grid: Widows Peak lies 1 zone west of Brog Fens
  - zone grid: Serpent Hills lies 1 zone east of Brog Fens
  - Prima, factions: Bluefeather's Brood: "Aviaks from Widow's Peak and Kelinar."
  - Prima, factions: Mountain Feather: "A flock of griffons that nest within the Serpent Hills."
  - Prima, factions: Villagers of Waterspout: "A small village of humans trying to eke out a living in the dangerous Serpent Hills."

## Halas, Moradhim and the north

- **HAL-1** Halas is a fledgling city far to the north, built and sustained by the united barbarian tribes of Everfrost; its guards are the Wolves of the North, its rogues the Eyes of the Tribunal, its shamans the Seers of the Tribunal.
  - Prima, factions: Citizens of Halas: "The fledgling city far to the north built and sustained by the unified Barbarian tribes of the Everfrost region."
  - Prima, factions: The Wolves of the North: "Halas guard faction."
  - Prima, factions: Eyes of the Tribunal: "Halas rogue guild."
  - Prima, factions: Seers of the Tribunal: "Halas shaman guild."
- **HAL-2** The barbarians of the Anu tribe keep apart: their trust is won only by killing their most hated enemy, the frost giants, and they do not trust magic users or priests.
  - Prima, factions: Barbarians of the Anu Tribe: "Isolationist Barbarians whose trust can only be gained by killing their most hated enemy, the frost giants. They do not trust magic users or priests."
- **HAL-3** Clan Snowfist, brutal orcs of the frozen wastes, hold the zone east of Halas; the Frosteye orcs lie south of Halas and west of Moradhim; the arctic goblins are the Freezeblood, whose village is the zone south of Halas.
  - Prima, factions: Clan Snowfist: "A clan of brutal orcs who live in the frozen wastes of the North."
  - Prima, factions: Frosteye Cohort: "Frosteye orc faction located south of Halas and west of Moradhim."
  - Prima, factions: The Freezeblood Goblins: "Arctic goblins of the North."
  - zone grid: Snowfist lies 1 zone east of Halas
  - zone grid: Freezeblood Village lies 1 zone south of Halas
- **HAL-4** Diren's Hold is a barbarian village built securely on a mountain south of Halas; Brontar's bandits wander the northern tundras of Everfrost.
  - Prima, factions: Citizens of Diren Hold: "A Barbarian village securely built upon a mountain south of Halas."
  - Prima, factions: Brontar's Bandits: "A group of bandits that wander the northern tundras of Everfrost."
- **MOR-1** The guards of Moradhim are the Stoneguard, its clerics the Church of Below, its paladins the Doomseekers, and its rogues Miners Guild 231.
  - Prima, factions: The Stoneguard: "Moradhim guard faction"
  - Prima, factions: The Church of Below: "Moradhim cleric guild."
  - Prima, factions: The Doomseekers of Moradhim: "Moradhim paladin guild."
  - Prima, factions: Miners Guild 231: "Moradhim rogue guild."
- **MOR-2** Chiasta is a quiet village in a small canyon east of Moradhim; Baga is a small troll village south of Moradhim.
  - Prima, factions: Chiasta Citizens: "A quiet village nestled in a small canyon east of Moradhim."
  - Prima, factions: Citizens of Baga: "A small Troll village found south of Moradhim."
- **MOR-3** The eyrie of the Goldfeather aviaks lies north of Moradhim, the Shon-To monastery east of it and Diren's village west; Jareth Roanir of Goldfeather is called the Blue Magician.
  - zone grid: Goldfeather Eyrie lies 1 zone north of Moradhim
  - zone grid: Shon-To Monastery lies 1 zone east of Moradhim
  - zone grid: Diren Village lies 1 zone west of Moradhim
  - Prima, factions: Goldfeather Aviaks: "Aviaks of Goldfeather Eyrie"
  - Prima, factions: Jareth Roanir: "The Blue Magician, Goldfeather."
- **GERN-1** Gerntar is an active mining operation worked by the dwarves of Clan Gerntar; Head Foreman Zeke needs help with the minotaurs there. The Rustel minotaurs of the mines are peaceful but for their evil leader Khelmarak.
  - Prima, factions: Citizens of Gerntar: "An active mining operation worked by the dwarves of Clan Gerntar."
  - EQOA-Data, 23-47/47: "**Head Foreman Zeke** in Gerntars Mine, help with minotaurs."
  - Prima, factions: Khelmarak: "the evil leader of the otherwise peaceful Rustel Minotaurs in Gentar's Mines."
- **GERN-2** Kelinar lies north of Gerntar's mines, the gate of Oggok south of them and the Elephant Graveyard east.
  - zone grid: Kelinar lies 1 zone north of Gerntar Mines
  - zone grid: Oggok Gate lies 1 zone south of Gerntar Mines
  - zone grid: Elephant Graveyard lies 1 zone east of Gerntar Mines

## The elves' and the gnomes' country

- **FAY-1** Fayspires is the greatest elven city since the fall of Takish'Hiz; its guards are the Paladins of Tunare, and the College of High Magic is its guild of wizards, enchanters and magicians.
  - Prima, factions: Citizens of Fayspires: "The elven citizenry of Fayspires, the greatest elven city since the fall of Takish`Hiz."
  - Prima, factions: The Paladins of Tunare: "Fayspires guard faction."
  - Prima, factions: College of High Magic: "Fayspires Wizard, Enchanter, and Magician guild."
- **FAY-2** The small village of Feydwell was almost destroyed by dark elf raiders out of Neriak; its survivors are its last defenders.
  - Prima, factions: Last Defenders of Feydwell: "Survivors of the small village of Feydwell, which was almost destroyed by Dark Elf raiders out of Neriak."
- **FAY-3** Melton is a druidic community charged with protecting the Glinaldal Forest; Proudpine is a small outpost of rangers and druids.
  - Prima, factions: Citizens of Melton: "A druidic community charged with the protection of the Glinaldal Forest."
  - Prima, factions: Proudpine Outpost: "A small outpost of rangers and druids."
- **FAY-4** Kara Village lies east of Fayspires and Tethelin, and the path east of Kara Village leads to Klick'Anon; Castle Felstar lies to the south-east.
  - zone grid: Kara Village lies 1 zone east of Fayspires Tethelin
  - Prima, walkthrough, line 27446: "Travel the path east of Kara Village to reach Klick'Anon."
  - zone grid: Castle Felstar lies 1 zone south-east of Fayspires Tethelin
- **TETH-1** Tethelin's druids are the Keepers of the Glade, its rogues the Scouts of Tunare, its bards the Songweavers of Tunare; its guards and rangers are the Emerald Guardians.
  - Prima, factions: Keepers of the Glade: "Tethelin druid guild."
  - Prima, factions: Scouts of Tunare: "Tethelin rogue guild."
  - Prima, factions: Songweavers of Tunare: "Tethelin bard guild."
  - Prima, factions: The Emerald Guardians: "Tethelin guard and ranger faction."
- **TETH-2** Dryads, satyrs and fauns are the spirits of the forest.
  - Prima, factions: The Spirits of the Forest: "Faction for dryad, satyr, and faun NPCs."
- **KLICK-1** Klick'Anon is the only gnomish city on Tunaria, and lies along the edge of the Lavastorm Mountains.
  - Prima, factions: Citizens of Klick`Anon: "The only Gnomish city found on Tunaria. It lies along the edge of the Lavastorm Mountains."
- **KLICK-2** Clockwork automatons were made to serve the citizens of Klick'Anon; rogue clockworks felled their masters there and settled to the east, in Clockwork Town.
  - Prima, factions: Klick`Anon Clockworks: "The clockwork automatons created to serve the citizens of Klick`Anon."
  - Prima, factions: Clockwork Town: "These rogue clockwork creations felled their masters in Klick`Anon to settle to the East."
  - zone grid: Rogue Clockworks lies 1 zone east of KlickAnon
- **KLICK-3** The guilds of Klick'Anon: the Mechanamagical College of wizards, enchanters and magicians, the Necrological Society of necromancers, the clerics' Temple of Brell, and the rogues' Junk Mongers; its guards are the Klick'Anon Watch.
  - Prima, factions: Mechanamagical College: "Klick`Anon wizard, enchanter, and magician guild."
  - Prima, factions: Necrological Society: "Klick`Anon necromancer guild."
  - Prima, factions: Temple of Brell: "Klick`Anon cleric faction"
  - Prima, factions: The Junk Mongers: "Klick`Anon rogue guild."
  - Prima, factions: Klick`Anon Watch: "Klick`Anon guard faction."
- **KLICK-4** Gnomes of the Institute of Astral Cognizance keep observatories across Tunaria; the gnomes of Explorers Guild 135 work in the Lavastorm Mountains.
  - Prima, factions: Institute of Astral Cognizance: "Faction for all Gnomes at the various observatories on Tunaria."
  - Prima, factions: Explorers Guild 135: "Faction of Gnomes in Lavastorm Mountains."
- **KLICK-5** Plankwalker's Pirates are gnomish pirates whose fascination with piracy outweighs their skill at it.
  - Prima, factions: Plankwalker's Pirates: "A group of gnomish pirates whose fascination with piracy outweighs their actually skill."
- **KLICK-6** The Coldpaw gnolls have a village just outside Klick'Anon; the Taereg, a band of rogues and burglars, camp west of the city.
  - Prima, factions: Coldpaw: "Faction for a gnoll village just outside of Klick`Anon."
  - Prima, factions: Taereg of Tunaria: "A band of rogues and burglars camped to the West of Klick`Anon."

## Neriak and its outposts

- **NER-1** Neriak is the subterranean city of the dark elves, with its laypeople and its nobility.
  - Prima, factions: Citizens of Neriak: "The laypeople and nobility of Neriak, the subterranean city of the Dark Elves."
- **NER-2** House Do'Vexis is the first House of Neriak, closest to the royal family and the most influential; House U'Dedne is the second, and rumor persists that it will soon fall; House J'Narus is the third, and many believe it will soon be much more.
  - Prima, factions: House Do`Vexis: "1st house of Neriak, closest to the royal family and the most influential of all Neriak's Houses."
  - Prima, factions: House U`Dedne: "The 2nd House of Neriak. Rumors persist that this house will soon fall."
  - Prima, factions: House J`Narus: "Currently the 3rd House of Neriak, though many believe they will soon become much more influential."
- **NER-3** House Gal'Saris, a noble house of Neriak, has won the task of excavating the Combine ruins in the Green Rift.
  - Prima, factions: House Gal`Saris: "A noble house of Neriak. They have won the responsibility of excavating the Combine ruins found in the Greenrift."
- **NER-4** The Royal Guard are elite dark elf guards born and bred for one purpose: to protect the Royal Family and the first House of Neriak from outsiders. The city's own guards are the Indigo Brotherhood.
  - Prima, factions: The Royal Guard: "The elite Dark Elf guards born and bred for one purpose: to protect the Royal Family and 1st House of Neriak from outsiders."
  - Prima, factions: The Indigo Brotherhood: "Neriak guard faction."
- **NER-5** The Houseless Rebellion is a small group of dark elves who seek to rebel against the ancient established society of the Teir'Dal.
  - Prima, factions: The Houseless Rebelion: "A small group of Dark Elf individuals who seek to rebel against the ancient established society of the Teir`Dal."
- **NER-6** The guilds of Neriak: the Lodge of the Dead of shadowknights and necromancers, the Spurned of wizards, magicians and enchanters, the Church of Innoruuk, and the rogues' Ebon Mask.
  - Prima, factions: Lodge of the Dead: "Neriak shadowknight and necromancer guild."
  - Prima, factions: The Spurned: "Neriak wizard, magician, and enchanter guild."
  - Prima, factions: The Church of Innoruuk: "Neriak cleric guild."
  - Prima, factions: The Ebon Mask: "Neriak rogue guild."
- **NER-7** The noble House of Dshinn works from a secret dark elf outpost; rats infest the Double Daggers Inn of Neriak.
  - Prima, factions: Citizens of Dshinn: "A secret Dark Elf outpost used as a base of operations by the noble House of Dshinn."
  - Prima, factions: Double Dagger Rats: "Rats that infest the Double Daggers Inn of Neriak."
- **SERIAK-1** Fort Seriak is a dark elf fort on the shores of Lake Nerius, the westernmost outpost of Neriak, north-west of Collonridge Cemetery.
  - Prima, factions: Fort Seriak: "Dark Elf fort on the shores of Lake Nerrius in Castle Felstar."
  - Prima, walkthrough, line 19145: "To the west is Fort Seriak, the western most outpost of Neriak. Fort Seriak is northwest of Collonridge Cemetery"
- **SERIAK-2** The human necromancer Verghest Felstar and his undead legions hold Castle Felstar, by Lake Nerius.
  - Prima, factions: Verghest Felstar: "Verghest Felstar (Human necro) and his undead legions are in this faction."

## Highbourne and Odus

- **HIGH-1** Highbourne is the center of Erudite society and home to Erud, one of the greatest minds on Tunaria; the Library of Highbourne, led by Erud, rules it and is its guild of wizards, magicians and enchanters. The city has a Senate.
  - Prima, factions: Citizens of Highbourne: "The center of Erudite society, and home to one of the greatest minds on Tunaria, Erud."
  - Prima, factions: Library of Highbourne: "Ruling faction of Highbourne, lead by Erud. Wizards, magicians, and enchanters Guild."
  - Prima, factions: The Highbourne Senate: "Exclusive to the Senate of Highbourne"
- **HIGH-2** Highbourne's church of Prexus is the Church of the Holy Pearl; its clerics are the Hand of Quellious, and its guards are the Peacekeepers.
  - Prima, factions: Church of the Holy Pearl: "Highbourne church of Prexus."
  - Prima, factions: The Hand of Quellious: "Highbourne cleric guild."
  - Prima, factions: The Peacekeepers: "Highbourne guard faction."
- **HIGH-3** Evil Erudites keep a hidden chapel of Cazic-Thule, the Hidden; undead rats outside Highbourne serve it.
  - Prima, factions: The Hidden: "Hidden Chapel of Cazic Thule guard faction. (For evil Erudites)"
  - Prima, factions: Servants of the Hidden: "Used specifically for the undead rats outside of Highbourne."
- **HIGH-4** Arcadin is a gleaming city of marble still under construction on Odus, the new home of the Erudites; at Jextalan a small camp of Erudites sells black market goods against the wishes of Arcadin's people.
  - Prima, factions: Denizens of Arcadin: "A gleaming city of marble still under construction on the continent of Odus. The new home of the Erudites."
  - Prima, factions: People of Jextalan: "A small encampment of Erudites who attempt to sell black market merchandise against the wishes of the people of Arcadin."
- **HIGH-5** The Erudite followers of Cazic-Thule built Morthalis, a temporary town, to secure their foothold on Odus, and are building a haven of their own there; many of its slaves are Kerrans of the Merka tribe, and undead raised from the grave guard the building.
  - Prima, factions: Citizens of Morthalis: "A temporary town built by the Erudite followers of Cazic-Thule in order to secure their foothold on Odus."
  - Prima, factions: The Merka Tribe: "Many of this tribe make up the slaves"
  - Prima, factions: Undead of Paineel: "The undead servants risen from the grave to protect the construction"
- **HIGH-6** Odus has its terrors: Sylhilthis, Queen of the evil and deceitful Naga, a being of awesome power; Toxxulia, the poison dragon; and evil beings made by some unknown god of nightmare, which haunt its mountains.
  - Prima, factions: Sylhilthis: "The Queen of the evil and deceitful Naga of Odus. A being of awesome power."
  - Prima, factions: Toxxulia's Brood: "The servants of the poison dragon of Odus, Toxxulia."
  - Prima, factions: Coalescence of Evil: "A faction of evil beings created by some unknown god of nightmare. They haunt the mountains of Odus."
- **HIGH-7** Qeynos Prison is the zone north of Highbourne and Stoneclaw the zone east of it, where phase jackals and shadow pumas are found; the road to Highbourne runs south across a river's mouth, with dangerous creatures along it.
  - zone grid: Qeynos Prison lies 1 zone north of Highbourne
  - zone grid: Stoneclaw lies 1 zone east of Highbourne
  - Prima, factions: Light of Stoneclaw: "Faction unique to Phase Jackals in Stoneclaw."
  - Prima, factions: Shadows of Stoneclaw: "Faction unique to the shadow pumas of Stoneclaw."
  - Prima, walkthrough, line 52347: "Travel south and cross the river's mouth until you reach Highbourne."

## The trolls' and the ogres' country

- **GROBB-1** Grobb is the largest community of trolls on Tunaria; its guards are Da Bashers, its shamans the Dark Ones, its shadowknights those of Nightkeep.
  - Prima, factions: Citizens of Grobb: "The largest community of Trolls on Tunaria."
  - Prima, factions: Da Bashers: "Grobb guard faction."
  - Prima, factions: The Dark Ones: "Grobb shaman guild."
  - Prima, factions: Shadowknights of Nightkeep: "Grobb shadowknight guild."
- **GROBB-2** Guk is the zone north of Grobb; the undead frogloks there are possessed by the angry spirits of the trolls who once lived in Guk, and a primitive society of fungusmen lives in it.
  - zone grid: Guk lies 1 zone north of Grobb
  - Prima, factions: Undead Frogloks: "The undead frogloks possessed by the angry Troll spirits that once inhabited Guk."
  - Prima, factions: Fungusmen of Guk: "The primitive fungusman society living within the dungeon of Guk."
- **GROBB-3** The troll clans struggle for dominance: the Razgut, camped at Fort Alliance, are losing; the Brokenskull are a clan of their own; Grimrik's outcasts keep to the Spider Mine and hate everyone.
  - Prima, factions: Razgut: "Camp of Trolls in Fort Alliance, a clan losing the struggle for dominance in the Troll society."
  - Prima, factions: Brokenskull Rock: "Faction for the Brokenskull Troll clan."
  - Prima, factions: Grimrik's Outcasts: "This faction is for the Trolls in the mines at Spider Mine. They hate all PC races"
- **GROBB-4** The Bloodfoot are orc and troll bandits with a large camp on Mount Hatespike.
  - Prima, factions: Bloodfoot Bandits: "Orc and Troll bandits with a large encampment in Mount Hatespike."
- **OGG-1** Oggok is the ogres' city, built on the ruins of the once great Rallosian Empire; the Craknek, its strongest warriors, protect its citizens; its shamans are the Chosen of Gunthak, and the Temple of Greenblood is its guild of shadowknights and necromancers.
  - Prima, factions: Citizens of Oggok: "The Ogre city of Tunaria built upon the ruins of the once great Rallosian Empire."
  - Prima, factions: Craknek Warriors: "The strongest warriors of Oggok charged with the protection of the Citizens of Oggok."
  - Prima, factions: Chosen of Gunthak: "Oggok shaman guild."
  - Prima, factions: Temple of Greenblood: "Shadowknight and necromancer guild of Oggok."
- **OGG-2** Sarek is the Great Betrayer of the ogre race, and has earthen minions.
  - Prima, factions: Sarek the Betrayer: "The Great Betrayer of the Ogre race and his earthen minions."
- **OGG-3** The Blacktoe ogres keep the shrine at Oggok and trust no one, except trolls a little; the Mog Thugs are ogre bandits in the Rathe Mountains.
  - Prima, factions: Blacktoe Ogres: "This is a faction made for the Ogres at the shrine in Ogguk. They do not trust anyone"
  - Prima, factions: Mog Thugs: "Faction for Ogre bandits in Rathe Mountains."
- **OGG-4** The Rathe Mountains are the only home of the mysterious and secretive sphinxes; the Rainfeather tribe of aviaks lives there, and hobgoblins.
  - Prima, factions: Sphynxs of Rathe: "The only home of the mysterious and secretive race known as the Sphinx."
  - Prima, factions: The Rainfeather Tribe: "A tribe of Aviaks living in the Rathe Mountains."
  - Prima, factions: Hobgoblins of Rathe Mtns.: "Hobgoblins of Rathe Mountains."
- **KERP-1** Kerplunk is the last ogre outpost outside the Rathe Mountains, deep in the jungles of southern Tunaria.
  - Prima, factions: Citizens of Kerplunk: "The last Ogre outpost existing outside of the Rathe Mountains, deep within the jungles of Southern Tunaria."
- **KERP-2** The temple-land of Cazic-Thule is the zone south of Kerplunk, the gate of Oggok west of it, Lake Noregard east and the Elephant Graveyard north; the Tae Ew are a tribe of lizardmen.
  - zone grid: Cazic-Thule lies 1 zone south of Kerplunk Outpost
  - zone grid: Oggok Gate lies 1 zone west of Kerplunk Outpost
  - zone grid: Lake Noregard lies 1 zone east of Kerplunk Outpost
  - zone grid: Elephant Graveyard lies 1 zone north of Kerplunk Outpost
  - Prima, factions: Tae Ew Lizards: "The lizardman tribe know as the Tae Ew."

## More of the land

- **LAND-26** The Seisen are a militaristic society of warmongering monks who worship Rallos Zek.
  - Prima, factions: Seisen Monks: "A militaristic society of warmongering monks who worship Rallos Zek."
- **LAND-27** The Actour's Guild is a mysterious guild of actors whose motives are unknown.
  - Prima, factions: Actour's Guild: "A mysterious guild of actors."
- **LAND-28** Saerk is a magician who keeps a tower with his minions and his staff.
  - Prima, factions: Saerk: "Faction for the magician Saerk, his minions, and his staff at his tower."
- **LAND-29** The creatures of the Plane of Sky are majestic and powerful; the dreadful creatures Cazic-Thule made are found only in his Plane of Fear.
  - Prima, factions: Denizens of the Plane of Sky: "The majestic and powerful creatures that inhabit the Plane of Sky."
  - Prima, factions: Denizens of Dread: "The dreadful creatures created by the Lord of Fear, Cazic-Thule. They're found only in the Plane of Fear."
