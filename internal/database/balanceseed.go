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
var (
	BalanceSeedLevels = []BalanceLevel{
		{Level: 1, HPPct: 300, DamagePct: 250, ConPct: 0, PetHPPct: 300, PetDamagePct: 250, ArtCooldownMS: 1500},
		{Level: 5, HPPct: 215, DamagePct: 265, ConPct: 0, PetHPPct: 215, PetDamagePct: 265, ArtCooldownMS: 1500},
		{Level: 8, HPPct: 225, DamagePct: 260, ConPct: 0, PetHPPct: 225, PetDamagePct: 260, ArtCooldownMS: 1500},
		{Level: 10, HPPct: 220, DamagePct: 225, ConPct: 25, PetHPPct: 220, PetDamagePct: 225, ArtCooldownMS: 1500},
		{Level: 15, HPPct: 230, DamagePct: 255, ConPct: 70, PetHPPct: 230, PetDamagePct: 280, ArtCooldownMS: 1500},
		{Level: 18, HPPct: 230, DamagePct: 250, ConPct: 100, PetHPPct: 230, PetDamagePct: 300, ArtCooldownMS: 1500},
		{Level: 25, HPPct: 250, DamagePct: 220, ConPct: 100, PetHPPct: 250, PetDamagePct: 285, ArtCooldownMS: 1500},
		{Level: 30, HPPct: 300, DamagePct: 200, ConPct: 100, PetHPPct: 300, PetDamagePct: 300, ArtCooldownMS: 1500},
		{Level: 35, HPPct: 300, DamagePct: 195, ConPct: 100, PetHPPct: 300, PetDamagePct: 340, ArtCooldownMS: 1500},
		{Level: 45, HPPct: 330, DamagePct: 190, ConPct: 100, PetHPPct: 410, PetDamagePct: 430, ArtCooldownMS: 1500},
		{Level: 50, HPPct: 340, DamagePct: 190, ConPct: 100, PetHPPct: 510, PetDamagePct: 475, ArtCooldownMS: 1500},
		{Level: 60, HPPct: 400, DamagePct: 160, ConPct: 100, PetHPPct: 800, PetDamagePct: 560, ArtCooldownMS: 1500},
	}
	BalanceSeedCons = []BalanceCon{
		{Diff: 0, DamagePct: 100, TakenPct: 100, Note: "even, and every blue and green below it"},
		{Diff: 1, DamagePct: 112, TakenPct: 91},
		{Diff: 2, DamagePct: 125, TakenPct: 83},
		{Diff: 3, DamagePct: 135, TakenPct: 79},
		{Diff: 4, DamagePct: 142, TakenPct: 76},
		{Diff: 5, DamagePct: 150, TakenPct: 72, Note: "and every red above it"},
	}
)
