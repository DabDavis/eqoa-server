-- Archivist Vaal — the hard-disk setup NPC, in Freeport.
--
-- Talking to him opens a dialogue that can CHECK the console's hard disk,
-- CREATE the EQOAWORLDS partition, or FORMAT the disk and create it. The work
-- happens in the running game: the SD payload's lazy init already loads
-- DEV9/ATAD/HDD/PFS and mounts pfs0: (that is how world streaming works at
-- all), and EQPREP.IRX does the partitioning. The player never leaves Freeport
-- and never sees a loading screen.
--
-- HOW THE DIALOGUE REACHES THE CONSOLE. The server sends an ordinary chat
-- message whose first character is U+E000, and the payload's hook on the
-- engine's message-display routine reads the second character as a verb and
-- never draws the line. A client WITHOUT the payload simply displays nothing
-- and nothing happens — which is the correct failure for a player whose
-- console has no hard disk support installed.
--
-- FORMATTING IS GUARDED TWICE, and neither guard is here: picking the format
-- line only ARMS it (30 seconds, one shot, cleared by any other pick or by
-- closing the box), and a second, separate menu whose safe answer is FIRST has
-- to be answered before the verb is sent. See internal/handlers/hddsetup.go.
--
-- THE NAME IS THE WIRING. internal/handlers matches this NPC by name, the same
-- way the coach and warp NPCs are matched, so the row can be re-inserted or
-- moved without the code learning a new id. TestHDDSetupNPCNameMatchesSQL fails
-- the build if the two spellings drift apart — renaming him here alone would
-- leave an NPC standing in Freeport with nothing to say.
--
-- POSITION. (25385.7, 54.1, 15741.5) is Harbormaster Thand's MEASURED ground
-- height at his own X/Z, offset 8 units east so the two do not stand inside one
-- another. Freeport's dock is not perfectly flat — Aldan Vas is at y=61.6 and
-- Ambassador Vu'Malik at y=56.2, both within a few hundred units — so this is
-- the one assumption in this file: that 8 units of eastward offset does not
-- change the ground height. If he floats or sinks, `#tele freeport` and
-- `cmd/groundpick` will say by how much, and only the y_coord here changes.
--
-- Thand is also the right donor for everything else: he is a neutral, non-
-- hostile, human-male Freeport NPC whose appearance and equipment columns are
-- known-good on this server rather than invented. npc_type 128 (0x80) =
-- FactionNeutral, so Vaal is not attackable.
--
-- Re-runnable: the DELETE makes this idempotent.

DELETE FROM npcs WHERE npc_name = 'Archivist Vaal';

INSERT INTO npcs (
    npc_name, zone, unk_a, unk_b, x_coord, y_coord, z_coord, facing, world,
    dt_kill, hp, hp_percent, modelid, size, unk_d, unk_f, unk_g,
    primary_weapon, secondary_weapon, shield, unk_h,
    torso, forearm, gloves, legs, feet, head,
    hair_color, hair_length, hair_style, face, robe_style, race,
    npc_level, npc_type, npc_id
)
SELECT
    'Archivist Vaal', zone, unk_a, unk_b, 25385.7, 54.1, 15741.5, 0, 0,
    dt_kill, hp, hp_percent, modelid, size, unk_d, unk_f, unk_g,
    primary_weapon, secondary_weapon, shield, unk_h,
    torso, forearm, gloves, legs, feet, head,
    hair_color, hair_length, hair_style, face, robe_style, race,
    npc_level, 128, npc_id
FROM npcs WHERE npc_name = 'Harbormaster Thand';
