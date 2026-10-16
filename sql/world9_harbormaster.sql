-- Harbormaster Machado — the world 9 end of the Freeport <-> world 9 route.
--
-- World 9 is Tunaria zone 127 (WIDOWS PEAK, the grid cell at X=12000 Z=22000 in
-- `claude-notes/tunaria-map.md`) lifted into a standalone one-zone world by
-- `cmd/zonelift` and streamed off the PS2 HARD DISK by the SD patch's
-- VIFile hooks. Unlike guk (world 8, memory card) nothing here is authored: the
-- Zone record is byte-identical to retail's, so the terrain, rooms, actors and
-- baked lighting are all Tunaria's own.
--
-- The Freeport end needs no row. Harbormaster Thand (id 515) already stands at
-- (25377.7, 54.1, 15741.5), and GetWarpDestinations pairs every warp NPC with
-- every other one automatically, so this single row wires up BOTH directions —
-- Thand gains world 9, and Machado offers Freeport back. That is the same
-- property `sql/guk_gatekeeper.sql` relies on.
--
-- POSITION. (13000, 66.5, 23000) is the MEASURED ground height at the centre of
-- the zone, not an estimate: the auto-goto drops the player at y=400 and the
-- server logged them landing at exactly (13000.0, 66.5, 23000.0). Nowhere else
-- in this zone has a height we have measured, so Machado stands on the one
-- point we can place him on without guessing — an NPC whose Y is wrong either
-- floats or sinks into the terrain, and the guk row's history is a string of
-- coordinates that stopped naming anything when the world file changed under
-- them. THE COORDINATES AND THE WORLD FILE MUST CHANGE TOGETHER: if world 9
-- ever stops being the zone-127 lift, this row is wrong.
--
-- The zone spans X 12000..14000, Z 22000..24000 (cmd/zonelift's bbox, which is
-- also what entity.worldBounds[9] asserts), so the centre is comfortably
-- inside it in every direction.
--
-- Every other column is copied from Thand so the appearance and equipment
-- fields are known-good rather than invented; only what must differ is
-- overridden. npc_type 128 (0x80) = FactionNeutral, so he is not attackable —
-- matching Gatekeeper Krup rather than Thand's own 0, which also resolves to
-- neutral but says so less clearly.
--
-- MODEL: Thand's own 1893243078 (0x70D898C6, human male) is inherited by the
-- copy. Widows Peak is an ordinary Tunaria zone, so there is no reason to reach
-- for a race whose CSprite might not be registered — the froglok model Krup
-- uses is only in the dictionary because of S2-race10's pregame SetSky hook.
--
-- Re-runnable: the DELETE makes this idempotent.

DELETE FROM npcs WHERE npc_name = 'Harbormaster Machado';

INSERT INTO npcs (
    npc_name, zone, unk_a, unk_b, x_coord, y_coord, z_coord, facing, world,
    dt_kill, hp, hp_percent, modelid, size, unk_d, unk_f, unk_g,
    primary_weapon, secondary_weapon, shield, unk_h,
    torso, forearm, gloves, legs, feet, head,
    hair_color, hair_length, hair_style, face, robe_style, race,
    npc_level, npc_type, npc_id
)
SELECT
    'Harbormaster Machado', zone, unk_a, unk_b, 13000.0, 66.5, 23000.0, 0, 9,
    dt_kill, hp, hp_percent, modelid, size, unk_d, unk_f, unk_g,
    primary_weapon, secondary_weapon, shield, unk_h,
    torso, forearm, gloves, legs, feet, head,
    hair_color, hair_length, hair_style, face, robe_style, race,
    npc_level, 128, npc_id
FROM npcs WHERE npc_name = 'Harbormaster Thand';
