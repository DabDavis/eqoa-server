-- Human East (humType 1) offers Cleric -- gRaces' classMask for it is 0x7E69,
-- bit 9 set (claude-notes/new-race-checklist.md) -- but the imported
-- defaultcharacter table has no row for it, so a Freeport cleric had no
-- template and the creation page could not offer one. The stats are the
-- race's and the class's (Human West's Cleric row); the start is Human
-- East's (its Warrior row). Safe to run twice.
INSERT INTO defaultcharacter (tclass, race, humType, level, tunar, bankTunar, unusedTP, totalTP,
    x, y, z, facing, defaultSpeed, world,
    defaultStrength, defaultStamina, defaultAgility, defaultDexterity,
    defaultWisdom, defaultIntelligence, defaultCharisma)
SELECT 9, 0, 1, w.level, w.tunar, w.bankTunar, w.unusedTP, w.totalTP,
    e.x, e.y, e.z, e.facing, e.defaultSpeed, e.world,
    w.defaultStrength, w.defaultStamina, w.defaultAgility, w.defaultDexterity,
    w.defaultWisdom, w.defaultIntelligence, w.defaultCharisma
FROM defaultcharacter w, defaultcharacter e
WHERE w.race = 0 AND w.humType = 2 AND w.tclass = 9
  AND e.race = 0 AND e.humType = 1 AND e.tclass = 0
  AND NOT EXISTS (SELECT 1 FROM defaultcharacter WHERE race = 0 AND humType = 1 AND tclass = 9);
