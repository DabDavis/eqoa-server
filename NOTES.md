# petpass-20261120e NOTES

Tickets: 2525 2526 2528 2529 2530 (Heirloom)

## 2528 (Guardian Forest goblins "35-60")
camps.go's line was stale: gen_camp_teles.py on today's Heirloom extract (~/.cache/petpass-20261120c/heirloom-extract.db)
gives 59-60 (LEVELS_WITHIN 120 of the anchor). Only that line taken: the whole regenerated file also drops the four
towns outside Tunaria (the extract lacks their NPCs), so a full rerun needs the box's DB. Also stale by the same rerun:
nasehir2 13-23 -> 13-20, bloodfoots 15-30 -> 28-30, hillgiants 45-48 -> 44-48, kappawarriors 57-59 -> 58-59 (not changed).

## 2525/2526 (Ellemental, magician paths)
No fault: Ellemental (L36) held the missile path (Lava Stone L20, Ice Shard L30) from the archivist before the L30 quest,
so the quest's Ice Shard was already in her book. archivistPathLines[10] pairs by kind (marks = pet buffs: Frozen Mark 20,
Burning Mark 30; missiles: Lava Stone 20, Ice Shard 30), so "the path of Ice Shard" swaps Frozen Mark out, Lava Stone in.
She then took the Burning Mark path: book now Frozen Mark, Burning Mark, Ice Shard (quest's). Nothing to change.

## 2529 (survive: deaths do not restart the 30 minutes)
By design (questsurvive.go, the t196 walkthrough: a questor who died at 29 minutes and was rezzed had the popup). No change.

## 2530 (the second Kithgrin silent)
Waldorf (step 3, pinned to the second Kithgrin, row 163614 at 25603,3096) talked to the THIRD (row 163615, his ticket's
target, 33 off), who had nothing to say: three NPCs named Kithgrin. Code: a talk to another spawn of a name a step is
pinned to says "Not I. " + the step's journal (quests.go otherSpawnLineLocked). Test questotherspawn_test.go.
