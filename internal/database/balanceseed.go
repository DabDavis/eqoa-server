package database

// BalanceSeedLevels and BalanceSeedCons are the rows ensureBalance gives a
// balancelevel or balancecon table it makes: the tuning of 2026-10-29 (the
// user: "yellows (1-2 levels higher) should always be a tough fight. Reds (3+
// levels) are possible, but definitely should not be trivial"), found with the
// fight simulator (entity/balancesim.go) on eqoaclassic's own items and spells
// and held to its targets by entity/balancesim_test.go.
//
// By level: the NPCs' hit points and melee so that a player of the level, in
// the middling gear of its last ten levels and fighting as its class plays,
// beats an even NPC in about 40 seconds (15 at level 1) with about two thirds
// of its hit points left, the middle of ten classes (every tank, melee class
// and priest); a low level's con_pct small, its own hit points and floor
// already climbing a third in three levels. A summoned pet's of its owner's
// level: the spells' pets lag their owners (a magician's last of its levels is
// level 47's Ghilan), and a pet's hits roll a creature's top less a tank's
// share of the NPC's defence, its floor at 50 and over, so its damage climbs
// past the NPCs'. By con: each level up to two a fifth harder to beat, then
// gentler, as a red is where a fight turns.
//
// The combat arts' cooldown (art_cooldown_ms, by the player's level): 1.5 s
// at every level, the length a global cooldown commonly has, so no burst
// lands five arts in a second (a level 60 warrior's landed 5,000 in four
// seconds); the simulator found it changes a fight's end little (tanks and
// bards within a few points of hit points left, a monk 3-10 fewer at 45-60,
// a rogue's even fight at 60 13 fewer), and 3 s cost a rogue of 60 its even
// fights' safety (96% won with 24% left).
//
// A summoned pet's hit points (pet_hp_pct) from level 15 are seven tenths of
// the first tuning's (230 ... 800): a pet heals a 50 of them every tick now
// (npcregen.go), and with the first tuning's the pet classes of 45-60 won a
// red three up nearly always (a magician 100% of the time, a necromancer
// five up 58%); at seven tenths every pet class's wins by the con are within
// a few points of the first tuning's without regeneration (24 fights a cell,
// 15-60). BalanceSeedRevisions takes a table's rows still at the first
// tuning's to these.
//
// A creature's heal out of a fight (regen_pct): 5% of its hit points a tick
// from its third tick (the user: "Mobs regen slower"; jib's was 15%), so one
// left empty is whole in about 2 min 10 s, one left at half in 1 min 12 s:
// longer than an even fight (about 40 s) and a red's (70-90 s), so a fight
// broken off and taken up again inside that time is not lost, and still
// shorter than a player's rest (2-3% a tick: 3-5 minutes from empty), so
// no player out-rests a creature to wear it down over pulls.
var (
	BalanceSeedLevels = []BalanceLevel{
		{Level: 1, HPPct: 300, DamagePct: 250, ConPct: 0, PetHPPct: 300, PetDamagePct: 250, ArtCooldownMS: 1500, RegenPct: 5},
		{Level: 5, HPPct: 215, DamagePct: 265, ConPct: 0, PetHPPct: 215, PetDamagePct: 265, ArtCooldownMS: 1500, RegenPct: 5},
		{Level: 8, HPPct: 225, DamagePct: 260, ConPct: 0, PetHPPct: 225, PetDamagePct: 260, ArtCooldownMS: 1500, RegenPct: 5},
		{Level: 10, HPPct: 220, DamagePct: 225, ConPct: 25, PetHPPct: 220, PetDamagePct: 225, ArtCooldownMS: 1500, RegenPct: 5},
		{Level: 15, HPPct: 230, DamagePct: 255, ConPct: 70, PetHPPct: 161, PetDamagePct: 280, ArtCooldownMS: 1500, RegenPct: 5},
		{Level: 18, HPPct: 230, DamagePct: 250, ConPct: 100, PetHPPct: 161, PetDamagePct: 300, ArtCooldownMS: 1500, RegenPct: 5},
		{Level: 25, HPPct: 250, DamagePct: 220, ConPct: 100, PetHPPct: 175, PetDamagePct: 285, ArtCooldownMS: 1500, RegenPct: 5},
		{Level: 30, HPPct: 300, DamagePct: 200, ConPct: 100, PetHPPct: 210, PetDamagePct: 300, ArtCooldownMS: 1500, RegenPct: 5},
		{Level: 35, HPPct: 300, DamagePct: 195, ConPct: 100, PetHPPct: 210, PetDamagePct: 340, ArtCooldownMS: 1500, RegenPct: 5},
		{Level: 45, HPPct: 330, DamagePct: 190, ConPct: 100, PetHPPct: 287, PetDamagePct: 430, ArtCooldownMS: 1500, RegenPct: 5},
		{Level: 50, HPPct: 340, DamagePct: 190, ConPct: 100, PetHPPct: 357, PetDamagePct: 475, ArtCooldownMS: 1500, RegenPct: 5},
		{Level: 60, HPPct: 400, DamagePct: 160, ConPct: 100, PetHPPct: 560, PetDamagePct: 560, ArtCooldownMS: 1500, RegenPct: 5},
	}
	// BalanceSeedClasses: every class's row, 100 but the ranger's, the
	// bard's and the rogue's. With a melee class's 16 hit points a level and
	// +15 a hit against a tank's 24 and shield, they won a yellow two up at
	// 45-60 half the time or less (ranger 15%, rogue 26%, bard 49%) while a
	// tank won a red five up half the time. The simulator over 40 fights a
	// cell, levels 1-60 (with the arts' cooldown and the spell fixes of the
	// same day): the ranger at 70%, the bard at 80% and the rogue at 65% win
	// two up at 45-60 79%, 85% and 76% of the time with 8-52% of their hit
	// points left, an even fight with 48-79% left, and never a red five up;
	// a bard needs less, its songs and heals. Below 45 they were nearer the
	// targets already and are a little easier now (an even fight with 61-80%
	// left).
	BalanceSeedClasses = []BalanceClass{
		{Class: 0, MeleeTakenPct: 100}, {Class: 1, MeleeTakenPct: 70}, {Class: 2, MeleeTakenPct: 100},
		{Class: 3, MeleeTakenPct: 100}, {Class: 4, MeleeTakenPct: 100}, {Class: 5, MeleeTakenPct: 80},
		{Class: 6, MeleeTakenPct: 65}, {Class: 7, MeleeTakenPct: 100}, {Class: 8, MeleeTakenPct: 100},
		{Class: 9, MeleeTakenPct: 100}, {Class: 10, MeleeTakenPct: 100}, {Class: 11, MeleeTakenPct: 100},
		{Class: 12, MeleeTakenPct: 100}, {Class: 13, MeleeTakenPct: 100}, {Class: 14, MeleeTakenPct: 100},
	}
	// BalanceSeedCons' root columns (root.go): a root's resist at its landing
	// and its break at each hit, by the con on its caster. With none (the
	// game before them) a rooting caster of 45-60 (druid, wizard, alchemist,
	// ranger, in the simulator's root-and-nuke play) won nearly every fight
	// to five up, most with all its hit points. The simulator over 40 fights
	// a cell, from the design's 0/5 even ... 65/30 five up: these win an even
	// fight 98-100% of the time, one up 84-100%, two up 49-99% (the wizard's
	// the least, the ranger's the most: its AC and Balance by class's share
	// make a break cheap), three up 12-61%, five up 0-1%. Gentler at one and
	// two up than the design (5% and 15% resisted, not 10% and 20%: the
	// wizard won two up 42% of the time), harder at three to five (a break
	// of 25-35%, not 20-30%).
	// BalanceSeedRevisions are seed values changed since a table could have
	// been made with them: a row still the seed's (source seed) and still
	// at the old value takes the new one at the next open; a GM's row, or a
	// value a GM changed, is left.
	BalanceSeedRevisions = []BalanceSeedRevision{
		{Table: "balancelevel", Col: "pet_hp_pct", Key: "level",
			Old: map[int]float64{15: 230, 18: 230, 25: 250, 30: 300, 35: 300, 45: 410, 50: 510, 60: 800},
			New: map[int]float64{15: 161, 18: 161, 25: 175, 30: 210, 35: 210, 45: 287, 50: 357, 60: 560}},
	}
	BalanceSeedCons = []BalanceCon{
		{Diff: 0, DamagePct: 100, TakenPct: 100, RootResistPct: 0, RootBreakPct: 5, Note: "even, and every blue and green below it"},
		{Diff: 1, DamagePct: 112, TakenPct: 91, RootResistPct: 5, RootBreakPct: 10},
		{Diff: 2, DamagePct: 125, TakenPct: 83, RootResistPct: 15, RootBreakPct: 15},
		{Diff: 3, DamagePct: 135, TakenPct: 79, RootResistPct: 35, RootBreakPct: 25},
		{Diff: 4, DamagePct: 142, TakenPct: 76, RootResistPct: 50, RootBreakPct: 30},
		{Diff: 5, DamagePct: 150, TakenPct: 72, RootResistPct: 65, RootBreakPct: 35, Note: "and every red above it"},
	}
)
