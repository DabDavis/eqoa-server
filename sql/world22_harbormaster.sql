-- Harbormaster Deuces — the world 22 end of the Freeport <-> world 22 route.
--
-- World 22 is a standalone AUTHORED world (8,565,802 bytes, one zone), streamed
-- off the PS2 hard disk by the SD patch's VIFile hooks and fetched on demand by
-- its downloader: slot 22 already says "hdd:world22.ESF", so the world itself
-- needed no code, no SD rebuild and no memory-card write — only the file in the
-- server's worlds/ directory.
--
-- The Freeport end needs no row, and neither does world 9. Harbormaster Thand
-- (id 515) stands at (25377.7, 54.1, 15741.5) and GetWarpDestinations pairs
-- every warp NPC with every OTHER warp NPC automatically, so this single row
-- wires up both directions at once: Thand gains world 22, Deuces offers
-- Freeport, Widows Peak, Guk and the two control worlds back. World 9 has been
-- on Thand's menu since sql/world9_harbormaster.sql added Machado — routes are
-- discovered from the NPCs that exist rather than kept in a second table that
-- could disagree with them.
--
-- POSITION. (9000.0, 59.3, 9000.0) is an OBSERVED IN-WORLD ground height, not a
-- reading off the ESF. On 2026-08-11 a GM teleport dropped a player at
-- (9000, 400, 9000) in world 22 and the client's own position reports fell to
-- (8999.7, 59.3, 9000.0) and stopped there — that landing is the measurement.
--
-- IT HAD TO BE MEASURED THAT WAY. `cmd/groundpick` reports a flat vertex at
-- (9000.00, 54.12, 8584.62), but its coordinates are ZONE-LOCAL and this
-- world's zone is placed at an offset — the same trap that removed
-- entity.worldBounds[22], where two readings taken off the file were both
-- zone-local and put the camera underground. The two agree here to within ~5
-- units, which is corroboration, not a source: an NPC whose Y comes from the
-- file either floats or sinks the moment the offset is not zero.
--
-- THE COORDINATES AND THE WORLD FILE MUST CHANGE TOGETHER: if worlds/world22.ESF
-- is ever replaced with different geometry, this row is wrong and the landing
-- has to be re-measured.
--
-- Every other column is copied from Thand so the appearance and equipment
-- fields are known-good rather than invented; only what must differ is
-- overridden. npc_type 128 (0x80) = FactionNeutral, so he is not attackable,
-- matching Machado and Krup. MODEL: Thand's own 1893243078 (0x70D898C6, human
-- male), inherited by the copy — no reason to reach for a race whose CSprite
-- might not be registered in an authored world.
--
-- Re-runnable: the DELETE makes this idempotent.

DELETE FROM npcs WHERE npc_name = 'Harbormaster Deuces';

INSERT INTO npcs (
    npc_name, zone, unk_a, unk_b, x_coord, y_coord, z_coord, facing, world,
    dt_kill, hp, hp_percent, modelid, size, unk_d, unk_f, unk_g,
    primary_weapon, secondary_weapon, shield, unk_h,
    torso, forearm, gloves, legs, feet, head,
    hair_color, hair_length, hair_style, face, robe_style, race,
    npc_level, npc_type, npc_id
)
SELECT
    'Harbormaster Deuces', zone, unk_a, unk_b, 9000.0, 59.3, 9000.0, 0, 22,
    dt_kill, hp, hp_percent, modelid, size, unk_d, unk_f, unk_g,
    primary_weapon, secondary_weapon, shield, unk_h,
    torso, forearm, gloves, legs, feet, head,
    hair_color, hair_length, hair_style, face, robe_style, race,
    npc_level, 128, npc_id
FROM npcs WHERE npc_name = 'Harbormaster Thand';
