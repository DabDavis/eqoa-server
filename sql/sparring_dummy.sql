-- A Sparring Dummy beside each Training Dummy Lv50 (F3 20261025-174503:
-- a dummy whose level you set, that quads you and strikes you with an element
-- you pick; internal/entity/sparring.go, internal/handlers/sparring.go).
--
-- Its name is what makes it one (DummyKindOf: "Sparring Dummy" ...); its level
-- is the player's pick, so the row's is only where it starts. Every other
-- column is the Training Dummy Lv50's, whose body and size are known good,
-- standing 4 units east of it: on Freeport's plaza and on Muniel's Tea
-- Garden's ladder of dummies (8 from its Lv45, the nearest).
--
-- Re-runnable: the DELETE makes this idempotent. Restart the server after, or
-- add the row through the GM panel, which applies it at once.

DELETE FROM npcs WHERE npc_name = 'Sparring Dummy';

INSERT INTO npcs (
    npc_name, zone, unk_a, unk_b, x_coord, y_coord, z_coord, facing, world,
    dt_kill, hp, hp_percent, modelid, size, unk_d, unk_f, unk_g,
    primary_weapon, secondary_weapon, shield, unk_h,
    torso, forearm, gloves, legs, feet, head,
    hair_color, hair_length, hair_style, face, robe_style, race,
    npc_level, npc_type, npc_id
)
SELECT
    'Sparring Dummy', zone, unk_a, unk_b, x_coord + 4.0, y_coord, z_coord, facing, world,
    dt_kill, hp, hp_percent, modelid, size, unk_d, unk_f, unk_g,
    primary_weapon, secondary_weapon, shield, unk_h,
    torso, forearm, gloves, legs, feet, head,
    hair_color, hair_length, hair_style, face, robe_style, race,
    10, npc_type, npc_id
FROM npcs WHERE npc_name = 'Training Dummy Lv50';
