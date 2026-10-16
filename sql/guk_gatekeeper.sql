-- Gatekeeper Krup — the guk end of the Freeport <-> guk warp route.
--
-- Gatekeepers and Harbormasters are EQOA's CROSS-WORLD travel NPCs; coachmen
-- only move you around inside one world. guk is world 8, loaded off the memory
-- card by our SD patch, so it needs a warp NPC rather than a coach stop.
--
-- The Freeport end needs no row: Harbormaster Thand (id 515) already stands at
-- (25377.7, 54.1, 15741.5), and GetWarpDestinations pairs every warp NPC with
-- every other one automatically, so adding this row wires up BOTH directions.
--
-- Position. REPOINTED 2026-08-08: guk.esf is no longer the authored 2x2 world,
-- it is RATHE zone 2 lifted out whole by the zone editor's new single-zone
-- export. So the old coordinates (1200, 0, 1000) name nothing — that was the
-- authored world's little island, and this world's terrain spans
-- X 0..4000, Y -10.0..270.0, Z 0..4000.
--
-- Why the swap: the authored world rendered EMPTY on hardware after the loader
-- was fixed, and its rooms are the suspect — 9 rooms with 2-unit-tall bboxes
-- against retail's 20..77 and 176..264, which `guk-empty-world-rooms.md`
-- predicted would fail CullByTop next. A retail zone is the control that
-- settles it: 520 rooms, 304 with sprites, 0 unresolvable, 9 with portals, and
-- bbox heights of 38..135. If THIS renders, the authoring is the bug.
--
-- The spot comes from `cmd/groundpick`, which ranks by local flat-ground
-- DENSITY rather than distance to the zone centre — a normal pointing up does
-- not tell a plain from an isolated sliver on a cliff edge:
--
--   PICK: (3661.53, 24.12, 3184.62)  nY=1.000  490 flat neighbours within 60u
--
-- 2026-08-08, AGAIN: guk.esf is back to the AUTHORED world for the heap
-- control, so the coordinates went back with it. Leaving RATHE zone 2's
-- (3661.53, 24.12, 3184.62) in place put the player ~2000 units outside an
-- authored world that spans X 750..1700, Z 750..1250 -- every room then culls
-- CORRECTLY and the screen is empty for a reason that has nothing to do with
-- the world. THE COORDINATES AND THE WORLD FILE MUST BE CHANGED TOGETHER.
--
-- Zone 2 replaced zone 16 because zone 16's 5.8 MB forced a 5,898,240-byte
-- malloc inside the world-change window and it FAILED on hardware: the
-- loader died ~89k cycles in, before any card I/O. Zone 2 is 2,297,998 B
-- (buffer 2,424,832) and keeps every property that makes it a control --
-- 512 rooms, 272 with sprites, 0 unresolvable, bbox heights 30..141.
--
-- It is 2041 units off the zone centre, and that is the ranking working rather
-- than failing: the middle of this zone is not the flattest part of it.
--
-- Every other column is copied from Thand so the appearance/equipment fields
-- are known-good rather than invented; only what must differ is overridden.
-- npc_type 128 (0x80) matches the coachmen = FactionNeutral, so he is not
-- attackable. Thand's own 0 also resolves to neutral, so he is left alone.
--
-- MODEL: 0x141BFE0C = 337378828, the froglok CSprite in CHAR.ESF — guk is a
-- froglok city, so its gatekeeper is one. This is the DB/WIRE form, which is
-- BYTE-REVERSED from the ESF DictID tables in CLAUDE.md; it needs no swap here
-- because it is the same value handlers.getModelID puts on the wire for a
-- froglok player (cross-check: that table's human male is 0x70D898C6, exactly
-- Thand's modelid below).
--
-- The froglok CSprite is only in the dictionary because S2-race10's pregame
-- SetSky hook parses it out of CHAR.ESF. That hook runs at pregame, so whether
-- the registration survives a world change into guk is UNVERIFIED — if Krup
-- renders as nothing, that is the thing to check first, and it would apply
-- equally to a froglok player walking in. Reverting to Thand's human model is
-- a one-word edit.
--
-- race is set to '10' for honesty rather than effect: raceScaleFromString ->
-- RaceScale(10) is out of the 10-entry scale table and falls back to 1.0,
-- which is the same scale human race '0' would have given.
--
-- Re-runnable: the DELETE makes this idempotent.

DELETE FROM npcs WHERE npc_name = 'Gatekeeper Krup';

INSERT INTO npcs (
    npc_name, zone, unk_a, unk_b, x_coord, y_coord, z_coord, facing, world,
    dt_kill, hp, hp_percent, modelid, size, unk_d, unk_f, unk_g,
    primary_weapon, secondary_weapon, shield, unk_h,
    torso, forearm, gloves, legs, feet, head,
    hair_color, hair_length, hair_style, face, robe_style, race,
    npc_level, npc_type, npc_id
)
SELECT
    'Gatekeeper Krup', zone, unk_a, unk_b, 1125.0, -62.19, 1143.69, 0, 8,
    dt_kill, hp, hp_percent, 337378828, size, unk_d, unk_f, unk_g,
    primary_weapon, secondary_weapon, shield, unk_h,
    torso, forearm, gloves, legs, feet, head,
    hair_color, hair_length, hair_style, face, robe_style, '10',
    npc_level, 128, npc_id
FROM npcs WHERE npc_name = 'Harbormaster Thand';
