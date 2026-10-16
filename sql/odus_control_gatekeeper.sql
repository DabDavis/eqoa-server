-- Gatekeeper Sulon — a CONTROL, not content.
--
-- guk loads from the memory card, parses, gets a valid streaming handle, and
-- still renders nothing — no terrain AND no NPCs, even though the server has
-- Gatekeeper Krup in channel 1 the whole time. That last part is what makes a
-- content explanation weak: entity rendering does not depend on terrain bakes,
-- so "no terrain and no entities" looks more like a scene that was never
-- brought up than like a world missing its lighting records.
--
-- This splits the question with one dialogue pick instead of a GM command
-- typed on an on-screen keyboard. Odus is a RETAIL world on the disc, reached
-- by the same 0x00B2 teleport with world=2:
--
--   Odus renders  -> the world-change path is fine, guk's CONTENT is at fault
--   Odus is empty -> the world-change path itself is broken for everything,
--                    and guk was never the problem
--
-- Position is the exact one captured from a real gatekeeper port on Sandstorm
-- (claude-notes/memcard-world-loading.md): world 02, (7338.12, 54.21, 11124.15).
-- Being a warp NPC it also gives Odus a way BACK, so the control cannot strand
-- anyone the way the earlier failed guk teleport did.
--
-- Delete this row once the question is answered.

DELETE FROM npcs WHERE npc_name = 'Gatekeeper Sulon';

INSERT INTO npcs (
    npc_name, zone, unk_a, unk_b, x_coord, y_coord, z_coord, facing, world,
    dt_kill, hp, hp_percent, modelid, size, unk_d, unk_f, unk_g,
    primary_weapon, secondary_weapon, shield, unk_h,
    torso, forearm, gloves, legs, feet, head,
    hair_color, hair_length, hair_style, face, robe_style, race,
    npc_level, npc_type, npc_id
)
SELECT
    'Gatekeeper Sulon', zone, unk_a, unk_b, 7338.12, 54.21, 11124.15, 0, 2,
    dt_kill, hp, hp_percent, modelid, size, unk_d, unk_f, unk_g,
    primary_weapon, secondary_weapon, shield, unk_h,
    torso, forearm, gloves, legs, feet, head,
    hair_color, hair_length, hair_style, face, robe_style, race,
    npc_level, 128, npc_id
FROM npcs WHERE npc_name = 'Harbormaster Thand';
