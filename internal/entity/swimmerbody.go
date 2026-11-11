package entity

// The sea creatures' bodies (sharks, fish, eels, sirens, mermen, the
// Megaladon) have no attack of their own on land. Their CHAR.ESF play lists
// are all the same six slots: 0 idle, 12 swim, 13 tread water, 14 death, 34
// hit and 44, the swimming attack. CLIENT's swing on a creature
// (VISpellEventDriver's PlayAttack, VICSprite::SetAttackAction) asks an
// unarmed hand's 23 or 22 (25 or 24 for the alternate), which these bodies
// lack: SetDefaultEntries fills them from the idle, played once, so a shark's
// round showed nothing (the owner, 2026-11-11: "im not seeing megaladons
// attack animations"). VICSprite::SetAnimation turns any attack (22-53 but
// 34) into 44 only while the body plays 12, 13 or 44, a swimmer's. Byte 58
// (SetLocomotion) said 0 standing and 1 or 3 moving, which on these bodies
// is the idle either way, so the swing never reached 44. Sent as the water's
// (13 still, 12 moving), the body treads water and swims, and every swing
// plays its own attack, on a PS2 and in vanilla alike (the same code).

// swimmerBodies is every CHAR.ESF body whose play list is 0, 12, 13, 14, 34
// and 44 (29 of 568; swimmerbody_test.go re-reads them off the disc's
// CHAR.ESF where it is at hand).
var swimmerBodies = map[uint32]bool{
	0x0A7B189B: true, // octoravager
	0x0AD3F647: true, // pike, salmon, minnows, trout
	0x0B0E4AEE: true,
	0x1738A692: true, // guardian serpant
	0x1740427C: true, // enchanting siren, Quintashi's handmaiden
	0x2AEB61A0: true, // razorfang piranha, razorgill, bonefish
	0x3FEA57BE: true, // dragon eels, electric eel
	0x55EA34E5: true, // Neskar the Deep, twisted hippocampus
	0x67542784: true,
	0x69842C02: true, // Megaladon, bloodfin brood mother
	0x74BA7C3D: true, // mermaid
	0x7565344B: true, // darkswamp piranha
	0x756BEB5F: true,
	0x7F4B413F: true, // piranhas, bottlefish
	0x885344E8: true, // shocking eel, lightning eel, hungry serpant
	0x8DF8EB5E: true, // kraken
	0x8F95C044: true, // deep mare
	0x9CAEF765: true, // ferocious anglers
	0xA316EB78: true, // merman
	0xBEFB4465: true, // mako, fierce, hammerhead, bloodfin sharks
	0xBF21FEF5: true, // sirens
	0xC03536EA: true, // strangling squid, marrow maw
	0xC4DBD0A5: true, // moray eels, leviathan, filth squid
	0xD275EE8F: true, // Quintashi
	0xD913FA19: true, // manta rays
	0xDF4AE0A5: true,
	0xE089A5D1: true, // saw fish, water wolf
	0xE3F33528: true, // hammerhead sharks
	0xE42C4BB2: true,
}

// bodyLoco is the animation byte 58 carries for a body: a swimmer's standing
// or walking as the water's (the file's doc), anything else as it is.
func bodyLoco(model uint32, loco byte) byte {
	if !swimmerBodies[model] {
		return loco
	}
	switch loco {
	case LocoStand:
		return LocoTread
	case LocoWalk, LocoRun:
		return LocoSwim
	}
	return loco
}
