-- Gatekeeper Rilis — world 7 (`secrets2`), a CONTROL that costs no deploy.
--
-- Every teleport to world 8 has crashed in the same teardown, and the question
-- is whether that is our memory-card loader or the world-change path itself.
-- The SD-disc8 build answers it but has to survive the two-pass Delete dance to
-- reach the console. World 7 answers a closely related question using the SD
-- ALREADY on the card:
--
--   world 7 crashes too  -> a high-index DISC world crashes, so the loader is
--                           exonerated without shipping anything
--   world 7 is fine      -> the mc: path or index 8 specifically is implicated
--
-- Why 7 is the right index to borrow. Sandstorm's own table entry 7 (`secrets2`)
-- is a VERBATIM duplicate of entry 5 (`secrets`) — same path, same id 0x06F8,
-- same extents — so it is a disc world at a high index that needs no patch of
-- ours, and it sits inside the bound our SD already raised from 6 to 9.
--
-- Do NOT do this with world 6 (`underfoot`): UNDERFOOT.ESF is absent from the
-- patched ISO, so a failure there would mean nothing.
--
-- POSITION, measured rather than guessed. DATA3/SECRETS.ESF (ISO LBA 1167458,
-- 36671513 bytes) has 10 zones that tile world space on the usual 2000-unit
-- grid. Zones 1-3 are dead-flat filler (every vertex nY=1.000 at Y=-60.94);
-- zone 0 is the real terrain, X 0..5984.6, Z 0..4000, Y -67.6..239.4.
--
-- (4562.12, -12.34, 3663.84) is an actual mesh vertex of zone 0 with nY=1.000
-- and 559 other flat up-facing vertices within 60 units — picked by
-- cmd/groundpick, which ranks by local flat-ground DENSITY rather than by
-- distance to the zone centre. That distinction matters: the centre-nearest
-- flat vertex was (4107.69, -20.00, 2153.84) at nY=0.987, and "a normal
-- pointing up" alone does not distinguish the middle of a plain from an
-- isolated sliver on a cliff edge.
--
-- Every other column is copied from Harbormaster Thand so appearance and
-- equipment are known-good rather than invented; only what must differ is
-- overridden. npc_type 128 (0x80) = FactionNeutral, matching the coachmen and
-- the other two gatekeepers, so he is not attackable.
--
-- Being a warp NPC he also gives world 7 a way BACK, so this cannot strand
-- anyone — which is the whole reason to reach a test world through a gatekeeper
-- rather than a GM #goto.
--
-- Re-runnable: the DELETE makes this idempotent. Delete the row once the
-- question is answered.

DELETE FROM npcs WHERE npc_name = 'Gatekeeper Rilis';

INSERT INTO npcs (
    npc_name, zone, unk_a, unk_b, x_coord, y_coord, z_coord, facing, world,
    dt_kill, hp, hp_percent, modelid, size, unk_d, unk_f, unk_g,
    primary_weapon, secondary_weapon, shield, unk_h,
    torso, forearm, gloves, legs, feet, head,
    hair_color, hair_length, hair_style, face, robe_style, race,
    npc_level, npc_type, npc_id
)
SELECT
    'Gatekeeper Rilis', zone, unk_a, unk_b, 4562.12, -12.34, 3663.84, 0, 7,
    dt_kill, hp, hp_percent, modelid, size, unk_d, unk_f, unk_g,
    primary_weapon, secondary_weapon, shield, unk_h,
    torso, forearm, gloves, legs, feet, head,
    hair_color, hair_length, hair_style, face, robe_style, race,
    npc_level, 128, npc_id
FROM npcs WHERE npc_name = 'Harbormaster Thand';
