#!/usr/bin/env python3
"""Box Canyon's creatures a player cannot walk to (petition 2139, Zerkx, Heirloom, 2026-11-15: "box canyon instance
there are a bunch of mobs on the hills that cant be get too"; he stood at world 14 (20736.0, 90.2, 20263.3), on a hill
over the canyon, the griffons, drakes and spiders he could see on the mesa tops at 139-151 over him).

Box Canyon (tools/worlds/dungeons.json key "boxcanyon", box 20150-21850 x/z, entrance (20480, 6.5, 21200)) is world 0's
Box Canyons cut out whole, its 295 creatures copied where they stood (tools/dungeons_more_20261112.py). In world 0 its
canyons are entered from the desert round them; in world 14 the desert is shut off (cmd/worldedges' rocks), and only
the canyon the entrance stands in is a player's. cmd/playerreach walked world 14's ground files from the entrance as
entity/movecheck.go judges a player's walk (a floor is any upward face, the terrain's or anything placed, with 1.8 of
room over it; a climb no steeper than 1:1 plus a step of 1.3, mcRiseSlope/mcRiseStep: no climb of the live players'
58,422 reports was steeper; any fall; nothing through a face), on a grid of 2:

    go run ./cmd/playerreach -ground worlds/ground -world 14 -box 20150,20150,21850,21850 -seed 20480,6.5,21200 \
        -within 1200 -apart 12 -keepoff 20480,21200,120 -npcs NPCS.json   (NPCS: {id, x, y, z, gone} a creature)

163,048 floors are reached (the canyon's north half, its floor at ~6 and its slopes up to ~130); every place a player
has stood in it (the log's MOVECHECK lines, Zerkx's hill at 90) is among them. 98 of its 296 rows (the warden's
twin among them) stand within 3 of one. The cut-off is not the climb's rule: at 2:1 (63 degrees) the same 198 are
cut off; cliffs part them. The other 198 are of two kinds:

  - 63 stand OVER the reached canyon (a player comes down from where they stand, never up to it): mesa tops and
    hills at 79-151. The 43 of them within 200 of a reached floor (the petition's) are MOVED, camps (rows within 20
    of one another) as they stood, to the nearest open reached floor (every floor within 6 reached and within 3 of
    its height, the creature's own floor, ground.Floor, the same; 12 from every creature that stays, 35 from another
    moved camp, a social creature's 30; never within 120 of the entrance): moves of 66-316, median 136.
  - 135 stand in canyons WALLED OFF from the entrance's (the south and east boxes, the rim's ledges, Skahyir's pit at
    -67), and 20 on mesa tops 280-600 off. A place "near its camp" for them is the far tip of another canyon, 300-870
    away; put there they would stand 130 deep at two tips of the reached canyon. They are REMOVED from the templates
    (their npcs row, its npctemplate "#n" and npcloot "#n" rows: kept whole in the log), but the boss and a named:
    Skahyir (the boss) is MOVED to the nearest open floor to his pit (21042, 8.75, 21120), and Overlord Frank with
    the three of his camp beside him (a rogue, a wizard, a bandit), as they stood, 56 from him (21046, 7.08, 21064).

Box Canyon is 145 creatures after (and the warden's twin at the entrance): 97 that stood where a player reaches, 48
moved. A way into the walled-off canyons
(a passage in the world, or a GM portal like the Plane of Sky's) would want the 150 back: --undo puts them back
exactly (and the moves too).

The other dungeons of world 14, surveyed the same way from their entrances (not changed here): Guk 11 of 306 (the ghoul
urds' hall at y -11/-12 under (24830-24880, 28780-28850): Rubbip, Howling Death, two Mummified Guardians, seven
ghoul urds; its floor is not joined to the sunken halls the rest stand in, or joined by a way narrower than the
grid's 2: to be looked at), Blackburrow 1 of 153 (a gnoll forager at -42 in a dip 3 under the tunnel's floor);
Runnyeye, the Tomb, Felstar, Collonridge, Vox, Toxxulia, the Warrens, the Salt Mines and Crethley none.

Rows are found by id, name, world 14 and where they stood (to 0.01); on a shard whose rows differ (another id, the
same name and place) by name and place; a row a GM has moved since, or that is gone, is left alone and said. Report
only by default; --apply writes the moves and removals in one transaction with a gm_audit line, the log
boxcanyon_reach_1115_log (every row before and after); a second --apply does nothing; --undo puts back exactly what
it wrote (a row changed since is left and said) and drops the log.

WHEN IT TAKES EFFECT: the dungeons' templates are npcs rows, read at the server's start or by the GM panel's
reload_npcs (it keeps them out of the world as templates: applyNPCSettingsLocked, dungeonTemplateLocked; a removed
row's template leaves). An instance already open keeps its own copies; the next one opened is made from the rows as
they are now.

    python3 -E -s tools/boxcanyon_reach_20261115.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "boxcanyon_reach_1115_log"
WHO, VIA = "dab", "tool"
DETAIL = ("petition 2139 (Zerkx): Box Canyon's creatures a player cannot walk to, moved to the reached canyon or "
          "removed from the templates: tools/boxcanyon_reach_20261115.py")
WORLD = 14
TOL = 0.01

# (row, name, (x, y, z) as surveyed, (x, y, z) to, why): cmd/playerreach's places, camps kept as they stood
MOVES = [
    (167134, 'torrid drake', (21044.87, 125.283, 20403.374), (20886, 17.01, 20292),
     'on a hill over the canyon at y 125 (the nearest floor a player reaches 137 across at y 91): moved 194 to the floor below'),
    (167135, 'crag spider', (21065.748, 145.686, 20579.895), (21178, 7.97, 20658),
     'on a hill over the canyon at y 146 (the nearest floor a player reaches 108 across at y 84): moved 137 to the floor below'),
    (167136, 'pox griffon', (21154.834, 121.722, 20458.535), (21238, 10.63, 20492),
     'on a hill over the canyon at y 122 (the nearest floor a player reaches 48 across at y 83): moved 90 to the floor below'),
    (167140, 'pox griffon', (21227.793, 125.471, 20300.027), (21288, 10.53, 20422),
     'on a hill over the canyon at y 126 (the nearest floor a player reaches 84 across at y 97): moved 136 to the floor below'),
    (167143, 'crag spider', (21750.233, 137.867, 20445.135), (21602, 7.47, 20626),
     'on a hill over the canyon at y 138 (the nearest floor a player reaches 176 across at y 108): moved 234 to the floor below'),
    (167147, 'crag spider', (21629.64, 129.047, 20496.35), (21550, 8.45, 20572),
     'on a hill over the canyon at y 129 (the nearest floor a player reaches 53 across at y 102): moved 110 to the floor below'),
    (167148, 'crag spider', (21519.812, 104.738, 20401.252), (21450, 7.6, 20470),
     'on a hill over the canyon at y 105 (the nearest floor a player reaches 39 across at y 101): moved 98 to the floor below'),
    (167150, 'crag spider', (21382.905, 115.292, 20272.273), (21324, 7.59, 20386),
     'on a hill over the canyon at y 115 (the nearest floor a player reaches 79 across at y 109): moved 128 to the floor below'),
    (167172, 'Taksehir rogue', (21766.2, 6.46875, 21576.4), (21056, 12.68, 21076),
     "Overlord Frank's camp, in a canyon walled off from the entrance's: moved with him, as it stood"),
    (167173, 'Taksehir wizard', (21764.7, 6.46875, 21560.8), (21054, 10.7, 21060),
     "Overlord Frank's camp, in a canyon walled off from the entrance's: moved with him, as it stood"),
    (167176, 'Overlord Frank', (21756.5, 6.46875, 21563.9), (21046, 7.08, 21064),
     "Overlord Frank's camp, in a canyon walled off from the entrance's: moved with him, as it stood"),
    (167180, 'Taksehir bandit', (21767.9, 6.46875, 21546.2), (21058, 11.13, 21046),
     "Overlord Frank's camp, in a canyon walled off from the entrance's: moved with him, as it stood"),
    (167266, 'Skahyir', (21586.0, -67.0, 21513.9), (21042, 8.75, 21120),
     "the boss, in a pit at y -67 of a canyon walled off from the entrance's: moved to the nearest open floor"),
    (167271, 'pox griffon', (20295.457, 107.696, 21220.627), (20432, 6.45, 21090),
     'on a hill over the canyon at y 108 (the nearest floor a player reaches 94 across at y 40): moved 189 to the floor below'),
    (167274, 'torrid drake', (21340.249, 145.631, 20865.773), (21256, 11.66, 20764),
     'on a hill over the canyon at y 146 (the nearest floor a player reaches 73 across at y 109): moved 132 to the floor below'),
    (167280, 'crag spider', (20850.089, 147.786, 20495.429), (20766, 7.07, 20356),
     'on a hill over the canyon at y 148 (the nearest floor a player reaches 98 across at y 116): moved 163 to the floor below'),
    (167281, 'inferno drake', (20849.662, 147.965, 20482.354), (20766, 6.87, 20344),
     'on a hill over the canyon at y 148 (the nearest floor a player reaches 86 across at y 116): moved 162 to the floor below'),
    (167282, 'inferno drake', (20844.489, 150.825, 20513.764), (20760, 12.75, 20374),
     'on a hill over the canyon at y 151 (the nearest floor a player reaches 112 across at y 114): moved 163 to the floor below'),
    (167283, 'plague griffon', (20864.432, 142.905, 20504.049), (20780, 15.59, 20366),
     'on a hill over the canyon at y 143 (the nearest floor a player reaches 112 across at y 116): moved 162 to the floor below'),
    (167288, 'calescent ant', (21183.303, 135.868, 21002.836), (21060, 11.11, 21004),
     'on a hill over the canyon at y 136 (the nearest floor a player reaches 69 across at y 106): moved 123 to the floor below'),
    (167289, 'torrid drake', (21171.627, 133.985, 20991.578), (21048, 7.22, 20992),
     'on a hill over the canyon at y 134 (the nearest floor a player reaches 56 across at y 106): moved 124 to the floor below'),
    (167291, 'inferno drake', (20591.265, 108.82, 20209.413), (20742, 24.69, 20200),
     'on a hill over the canyon at y 109 (the nearest floor a player reaches 84 across at y 76): moved 151 to the floor below'),
    (167292, 'crag spider', (20698.755, 102.402, 21401.4), (20656, 8.97, 21134),
     'on a hill over the canyon at y 102 (the nearest floor a player reaches 155 across at y 109): moved 271 to the floor below'),
    (167293, 'plague griffon', (20693.808, 103.837, 21394.118), (20652, 8.97, 21126),
     'on a hill over the canyon at y 104 (the nearest floor a player reaches 146 across at y 109): moved 271 to the floor below'),
    (167300, 'plague griffon', (21000.0, 79.95665740966797, 21275.0), (20984, 7.56, 21166),
     'on a hill over the canyon at y 79 (the nearest floor a player reaches 79 across at y 56): moved 110 to the floor below'),
    (167301, 'plague griffon', (20515.206, 144.302, 20596.519), (20516, 7.52, 20468),
     'on a hill over the canyon at y 144 (the nearest floor a player reaches 68 across at y 102): moved 128 to the floor below'),
    (167302, 'plague griffon', (20500.203, 141.179, 20592.548), (20500, 6.77, 20464),
     'on a hill over the canyon at y 141 (the nearest floor a player reaches 63 across at y 102): moved 128 to the floor below'),
    (167303, 'plague griffon', (20514.967, 145.743, 20608.463), (20516, 10.38, 20480),
     'on a hill over the canyon at y 146 (the nearest floor a player reaches 77 across at y 117): moved 128 to the floor below'),
    (167304, 'crag spider', (20506.514, 145.303, 20617.447), (20508, 11.36, 20490),
     'on a hill over the canyon at y 145 (the nearest floor a player reaches 65 across at y 117): moved 128 to the floor below'),
    (167311, 'inferno drake', (20221.002, 107.594, 20554.199), (20284, 9.97, 20574),
     'on a hill over the canyon at y 107 (the nearest floor a player reaches 41 across at y 50): moved 66 to the floor below'),
    (167312, 'inferno drake', (20196.553, 107.062, 20545.685), (20298, 8.83, 20540),
     'on a hill over the canyon at y 107 (the nearest floor a player reaches 66 across at y 54): moved 102 to the floor below'),
    (167316, 'plague griffon', (20821.397, 96.437, 21295.191), (20902, 16.45, 21174),
     'on a hill over the canyon at y 96 (the nearest floor a player reaches 102 across at y 106): moved 146 to the floor below'),
    (167317, 'crag spider', (20807.395, 97.321, 21273.864), (20888, 7.29, 21152),
     'on a hill over the canyon at y 98 (the nearest floor a player reaches 87 across at y 107): moved 146 to the floor below'),
    (167318, 'calescent ant', (20813.197, 97.332, 21283.322), (20894, 10.41, 21162),
     'on a hill over the canyon at y 98 (the nearest floor a player reaches 93 across at y 106): moved 146 to the floor below'),
    (167319, 'pox griffon', (21471.233, 136.704, 20791.468), (21420, 11.59, 20686),
     'on a hill over the canyon at y 137 (the nearest floor a player reaches 68 across at y 97): moved 117 to the floor below'),
    (167320, 'singetail scropion', (20368.617, 88.129, 21383.493), (20594, 6.6, 21162),
     'on a hill over the canyon at y 88 (the nearest floor a player reaches 114 across at y 109): moved 316 to the floor below'),
    (167321, 'singetail scropion', (20318.34, 96.049, 21073.122), (20396, 10.3, 21056),
     'on a hill over the canyon at y 96 (the nearest floor a player reaches 58 across at y 38): moved 80 to the floor below'),
    (167322, 'pox griffon', (20327.032, 100.4, 21074.76), (20406, 7.16, 21058),
     'on a hill over the canyon at y 101 (the nearest floor a player reaches 49 across at y 38): moved 81 to the floor below'),
    (167323, 'singetail scropion', (20319.876, 96.647, 21091.66), (20398, 9.42, 21076),
     'on a hill over the canyon at y 97 (the nearest floor a player reaches 57 across at y 39): moved 80 to the floor below'),
    (167324, 'pox griffon', (20328.238, 100.993, 21066.578), (20406, 6.97, 21050),
     'on a hill over the canyon at y 101 (the nearest floor a player reaches 48 across at y 38): moved 80 to the floor below'),
    (167332, 'pox griffon', (20716.519, 146.615, 20978.954), (20598, 11, 21046),
     'on a hill over the canyon at y 147 (the nearest floor a player reaches 68 across at y 114): moved 136 to the floor below'),
    (167333, 'torrid drake', (20709.901, 145.433, 20987.843), (20590, 7.07, 21054),
     'on a hill over the canyon at y 145 (the nearest floor a player reaches 58 across at y 114): moved 137 to the floor below'),
    (167334, 'inferno drake', (20676.37, 143.594, 20565.961), (20596, 6.79, 20454),
     'on a hill over the canyon at y 144 (the nearest floor a player reaches 55 across at y 112): moved 138 to the floor below'),
    (167335, 'plague griffon', (20658.144, 144.291, 20574.914), (20564, 9.58, 20472),
     'on a hill over the canyon at y 144 (the nearest floor a player reaches 62 across at y 106): moved 140 to the floor below'),
    (167336, 'plague griffon', (20694.585, 143.014, 20557.212), (20652, 11.82, 20460),
     'on a hill over the canyon at y 143 (the nearest floor a player reaches 49 across at y 115): moved 106 to the floor below'),
    (167337, 'inferno drake', (20697.458, 145.604, 20583.107), (20616, 13.88, 20472),
     'on a hill over the canyon at y 146 (the nearest floor a player reaches 75 across at y 115): moved 138 to the floor below'),
    (167338, 'plague griffon', (20681.21, 145.803, 20584.381), (20600, 12.17, 20472),
     'on a hill over the canyon at y 146 (the nearest floor a player reaches 74 across at y 112): moved 139 to the floor below'),
    (167339, 'pox griffon', (20693.927, 139.624, 20546.6), (20652, 7.72, 20448),
     'on a hill over the canyon at y 139 (the nearest floor a player reaches 39 across at y 115): moved 107 to the floor below'),
]
# (row, name, (x, y, z) as surveyed, why)
REMOVES = [
    (167137, 'defiled mummy', (21091.0, 39.0391, 20227.8), "in a canyon walled off from the entrance's (y 39; the nearest floor a player reaches 136 across at y 88)"),
    (167138, 'singetail scropion', (21041.17, 40.466, 20192.673), "in a canyon walled off from the entrance's (y 40; the nearest floor a player reaches 99 across at y 88)"),
    (167139, 'crag spider', (21224.309, 37.99, 20159.208), "in a canyon walled off from the entrance's (y 38; the nearest floor a player reaches 205 across at y 103)"),
    (167141, 'defiled mummy', (21093.4, 38.9297, 20224.1), "in a canyon walled off from the entrance's (y 39; the nearest floor a player reaches 139 across at y 88)"),
    (167142, 'torrid drake', (21673.223, 37.555, 20316.229), "in a canyon walled off from the entrance's (y 38; the nearest floor a player reaches 209 across at y 104)"),
    (167144, 'pox griffon', (21774.097, 76.592, 20168.894), "in a canyon walled off from the entrance's (y 77; the nearest floor a player reaches 383 across at y 101)"),
    (167145, 'defiled mummy', (21547.488, 89.657, 20150.95), "in a canyon walled off from the entrance's (y 89; the nearest floor a player reaches 256 across at y 106)"),
    (167146, 'defiled mummy', (21543.611, 90.581, 20156.1), "in a canyon walled off from the entrance's (y 91; the nearest floor a player reaches 250 across at y 106)"),
    (167149, 'crag spider', (21525.591, 79.538, 20201.175), "in a canyon walled off from the entrance's (y 79; the nearest floor a player reaches 202 across at y 106)"),
    (167161, 'pox griffon', (21825.0, 80.56241607666016, 20775.0), "in a canyon walled off from the entrance's (y 80; the nearest floor a player reaches 215 across at y 9)"),
    (167162, 'singetail scropion', (21629.8, 6.46875, 21679.1), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 759 across at y 104)"),
    (167163, 'torrid drake', (21625.6, 6.46875, 21682.2), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 758 across at y 104)"),
    (167164, 'crag spider', (21622.3, 6.46875, 21687.8), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 760 across at y 104)"),
    (167165, 'pox griffon', (21618.4, 6.46875, 21691.5), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 760 across at y 104)"),
    (167166, 'defiled mummy', (21616.6, 6.57812, 21693.3), "in a canyon walled off from the entrance's (y 7; the nearest floor a player reaches 760 across at y 104)"),
    (167167, 'singetail scropion', (21632.3, 6.46875, 21681.4), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 762 across at y 104)"),
    (167168, 'torrid drake', (21628.4, 6.46875, 21684.7), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 762 across at y 104)"),
    (167169, 'crag spider', (21625.4, 6.46875, 21691.6), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 764 across at y 104)"),
    (167170, 'pox griffon', (21621.8, 6.89844, 21696.1), "in a canyon walled off from the entrance's (y 7; the nearest floor a player reaches 765 across at y 104)"),
    (167171, 'defiled mummy', (21619.6, 7.00781, 21697.0), "in a canyon walled off from the entrance's (y 7; the nearest floor a player reaches 764 across at y 104)"),
    (167174, 'Taksehir bandit', (21745.7, 6.46875, 21595.5), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 794 across at y 104)"),
    (167175, 'Taksehir warrior', (21731.3, 6.38281, 21582.1), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 774 across at y 104)"),
    (167177, 'Taksehir bandit', (21744.8, 6.46875, 21531.1), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 758 across at y 104)"),
    (167178, 'Taksehir rogue', (21726.3, 6.46875, 21538.0), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 745 across at y 104)"),
    (167179, 'Taksehir bandit', (21731.1, 6.46875, 21544.7), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 753 across at y 104)"),
    (167181, 'Taksehir warrior', (21655.5, 5.49219, 21553.8), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 696 across at y 104)"),
    (167182, 'Taksehir bandit', (21576.9, 6.46875, 21545.2), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 629 across at y 104)"),
    (167183, 'Taksehir cleric', (21723.7, 6.46094, 21554.1), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 752 across at y 104)"),
    (167184, 'Taksehir bandit', (21738.5, 6.46875, 21468.3), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 722 across at y 104)"),
    (167185, 'Taksehir rogue', (21668.2, 6.46875, 21662.7), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 775 across at y 104)"),
    (167186, 'singetail scropion', (21642.2, 6.53125, 21689.0), "in a canyon walled off from the entrance's (y 7; the nearest floor a player reaches 774 across at y 104)"),
    (167187, 'torrid drake', (21638.9, 6.53906, 21692.3), "in a canyon walled off from the entrance's (y 7; the nearest floor a player reaches 774 across at y 104)"),
    (167188, 'crag spider', (21636.0, 7.08594, 21696.7), "in a canyon walled off from the entrance's (y 7; the nearest floor a player reaches 776 across at y 104)"),
    (167189, 'pox griffon', (21633.6, 7.69531, 21701.6), "in a canyon walled off from the entrance's (y 8; the nearest floor a player reaches 777 across at y 104)"),
    (167190, 'defiled mummy', (21631.1, 8.48438, 21707.8), "in a canyon walled off from the entrance's (y 9; the nearest floor a player reaches 780 across at y 104)"),
    (167191, 'singetail scropion', (21215.2, 55.875, 21839.3), "in a canyon walled off from the entrance's (y 56; the nearest floor a player reaches 670 across at y 83)"),
    (167192, 'torrid drake', (21222.6, 57.0, 21840.3), "in a canyon walled off from the entrance's (y 57; the nearest floor a player reaches 673 across at y 83)"),
    (167193, 'pox griffon', (21229.9, 56.3203, 21840.5), "in a canyon walled off from the entrance's (y 56; the nearest floor a player reaches 675 across at y 83)"),
    (167194, 'crag spider', (21237.1, 57.5, 21838.5), "in a canyon walled off from the entrance's (y 58; the nearest floor a player reaches 676 across at y 83)"),
    (167195, 'defiled mummy', (21241.3, 58.2109, 21835.8), "in a canyon walled off from the entrance's (y 58; the nearest floor a player reaches 674 across at y 83)"),
    (167196, 'defiled mummy', (21245.6, 57.0156, 21844.1), "in a canyon walled off from the entrance's (y 57; the nearest floor a player reaches 684 across at y 83)"),
    (167197, 'crag spider', (21241.6, 56.8047, 21847.1), "in a canyon walled off from the entrance's (y 57; the nearest floor a player reaches 685 across at y 83)"),
    (167198, 'crag spider', (20273.496, 41.584, 21798.271), "in a canyon walled off from the entrance's (y 42; the nearest floor a player reaches 522 across at y 109)"),
    (167199, 'singetail scropion', (20191.2, 17.2578, 20966.2), "in a canyon walled off from the entrance's (y 17; the nearest floor a player reaches 159 across at y 23)"),
    (167200, 'torrid drake', (20189.0, 15.6406, 20974.8), "in a canyon walled off from the entrance's (y 16; the nearest floor a player reaches 164 across at y 23)"),
    (167201, 'pox griffon', (20188.9, 14.5469, 20981.6), "in a canyon walled off from the entrance's (y 14; the nearest floor a player reaches 167 across at y 25)"),
    (167202, 'crag spider', (20188.8, 13.9922, 20989.1), "in a canyon walled off from the entrance's (y 14; the nearest floor a player reaches 169 across at y 26)"),
    (167203, 'defiled mummy', (20189.0, 14.0625, 20993.1), "in a canyon walled off from the entrance's (y 14; the nearest floor a player reaches 170 across at y 26)"),
    (167204, 'singetail scropion', (20401.898, 66.058, 20204.195), "in a canyon walled off from the entrance's (y 66; the nearest floor a player reaches 162 across at y 108)"),
    (167205, 'defiled mummy', (20182.6, 14.2266, 20997.6), "in a canyon walled off from the entrance's (y 14; the nearest floor a player reaches 178 across at y 26)"),
    (167206, 'defiled mummy', (20238.1, 81.0312, 20150.0), "in a canyon walled off from the entrance's (y 81; the nearest floor a player reaches 251 across at y 112)"),
    (167207, 'cipher guardian', (21617.7, -19.0, 20904.0), "in a canyon walled off from the entrance's (y -19; the nearest floor a player reaches 190 across at y 89)"),
    (167212, 'wretched mummy', (21548.0, 6.4375, 21013.2), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 295 across at y 72)"),
    (167213, 'sand giant', (21526.8, 6.91406, 20987.2), "in a canyon walled off from the entrance's (y 7; the nearest floor a player reaches 269 across at y 85)"),
    (167214, 'inferno drake', (21527.0, 6.64062, 20995.9), "in a canyon walled off from the entrance's (y 7; the nearest floor a player reaches 277 across at y 85)"),
    (167215, 'plague griffon', (21531.9, 6.46875, 21011.2), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 293 across at y 85)"),
    (167216, 'malign wisp', (21538.0, 6.46875, 21016.5), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 298 across at y 71)"),
    (167217, 'wretched mummy', (21543.6, 6.46875, 21021.4), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 303 across at y 72)"),
    (167218, 'inferno drake', (21685.4, -56.0, 21401.2), "in a canyon walled off from the entrance's (y -56; the nearest floor a player reaches 645 across at y 104)"),
    (167219, 'inferno drake', (21704.82, 5.486, 21042.296), "in a canyon walled off from the entrance's (y 5; the nearest floor a player reaches 347 across at y 89)"),
    (167220, 'calescent ant', (21551.8, 6.46875, 21018.5), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 300 across at y 72)"),
    (167221, 'calescent ant', (21548.3, 6.46875, 21022.8), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 305 across at y 72)"),
    (167222, 'calescent ant', (21544.8, 6.46875, 21026.7), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 309 across at y 72)"),
    (167223, 'sand giant', (21514.3, 6.99219, 21005.7), "in a canyon walled off from the entrance's (y 7; the nearest floor a player reaches 285 across at y 102)"),
    (167224, 'inferno drake', (21230.436, 6.469, 21202.27), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 148 across at y 104)"),
    (167225, 'plague griffon', (21231.416, 6.52, 21183.213), "in a canyon walled off from the entrance's (y 7; the nearest floor a player reaches 142 across at y 104)"),
    (167226, 'malign wisp', (21243.293, 6.469, 21189.861), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 156 across at y 104)"),
    (167227, 'calescent ant', (21523.0, 6.46875, 21027.9), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 308 across at y 102)"),
    (167228, 'inferno drake', (21243.328, 6.469, 21204.787), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 161 across at y 104)"),
    (167229, 'sand giant', (21257.566, 6.479, 21198.754), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 172 across at y 104)"),
    (167230, 'plague griffon', (21217.986, 6.469, 21190.455), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 132 across at y 104)"),
    (167231, 'malign wisp', (21244.977, 6.73, 21180.127), "in a canyon walled off from the entrance's (y 7; the nearest floor a player reaches 154 across at y 104)"),
    (167232, 'calescent ant', (21220.246, 6.469, 21201.979), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 139 across at y 104)"),
    (167233, 'inferno drake', (21254.3, 6.46875, 21220.1), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 177 across at y 104)"),
    (167234, 'sand giant', (21246.1, 6.46875, 21217.8), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 169 across at y 104)"),
    (167235, 'plague griffon', (21239.3, 6.46875, 21216.9), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 163 across at y 104)"),
    (167236, 'malign wisp', (21226.1, 6.46875, 21217.8), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 152 across at y 104)"),
    (167237, 'calescent ant', (21218.3, 6.46875, 21218.0), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 145 across at y 104)"),
    (167238, 'wretched mummy', (20891.1, 3.63281, 21452.2), "in a canyon walled off from the entrance's (y 4; the nearest floor a player reaches 256 across at y 66)"),
    (167239, 'wretched mummy', (21227.904, 6.469, 21192.422), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 142 across at y 104)"),
    (167240, 'calescent ant', (20928.4, 7.24219, 21460.6), "in a canyon walled off from the entrance's (y 7; the nearest floor a player reaches 265 across at y 44)"),
    (167241, 'wretched mummy', (21218.8, 6.46875, 21213.4), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 143 across at y 104)"),
    (167242, 'wretched mummy', (21425.8, 9.53125, 21117.3), "in a canyon walled off from the entrance's (y 9; the nearest floor a player reaches 324 across at y 100)"),
    (167243, 'malign wisp', (20896.8, 4.625, 21455.0), "in a canyon walled off from the entrance's (y 5; the nearest floor a player reaches 259 across at y 63)"),
    (167244, 'plague griffon', (20909.3, 6.32812, 21461.1), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 265 across at y 53)"),
    (167245, 'inferno drake', (20915.7, 6.45312, 21463.1), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 267 across at y 49)"),
    (167246, 'sand giant', (20921.2, 6.57812, 21465.5), "in a canyon walled off from the entrance's (y 7; the nearest floor a player reaches 270 across at y 45)"),
    (167247, 'calescent ant', (20924.0, 6.50781, 21474.2), "in a canyon walled off from the entrance's (y 7; the nearest floor a player reaches 278 across at y 44)"),
    (167248, 'sand giant', (20900.4, 6.46875, 21502.4), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 306 across at y 60)"),
    (167249, 'inferno drake', (20899.1, 6.46875, 21496.6), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 301 across at y 60)"),
    (167250, 'plague griffon', (20895.4, 6.46875, 21497.6), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 302 across at y 63)"),
    (167251, 'malign wisp', (20490.1, 5.25, 21513.4), "in a canyon walled off from the entrance's (y 5; the nearest floor a player reaches 201 across at y 103)"),
    (167252, 'wretched mummy', (20515.5, 6.13281, 21543.7), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 230 across at y 103)"),
    (167253, 'wretched mummy', (20512.8, 6.25, 21545.7), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 232 across at y 103)"),
    (167254, 'wretched mummy', (20509.0, 6.40625, 21548.2), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 235 across at y 103)"),
    (167255, 'wretched mummy', (20505.3, 6.46875, 21550.6), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 238 across at y 103)"),
    (167256, 'inferno drake', (20505.4, 6.46875, 21540.3), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 227 across at y 103)"),
    (167257, 'sand giant', (20501.7, 6.42969, 21537.6), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 225 across at y 103)"),
    (167258, 'plague griffon', (20496.4, 6.36719, 21531.7), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 220 across at y 103)"),
    (167259, 'plague griffon', (20492.9, 6.42188, 21535.4), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 223 across at y 103)"),
    (167260, 'plague griffon', (20489.6, 6.46875, 21538.9), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 227 across at y 103)"),
    (167261, 'malign wisp', (20485.3, 6.28906, 21520.9), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 209 across at y 103)"),
    (167262, 'inferno drake', (20500.7, 6.46875, 21545.4), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 233 across at y 103)"),
    (167263, 'sand giant', (20497.9, 6.46875, 21543.1), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 231 across at y 103)"),
    (167264, 'wretched mummy', (20497.7, 6.46875, 21555.4), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 243 across at y 103)"),
    (167265, 'calescent ant', (20845.357, 6.807, 21625.62), "in a canyon walled off from the entrance's (y 7; the nearest floor a player reaches 417 across at y 109)"),
    (167267, 'inferno drake', (21647.4, -14.0, 21262.4), "in a canyon walled off from the entrance's (y -14; the nearest floor a player reaches 550 across at y 89)"),
    (167268, 'inferno drake', (21641.089, 6.469, 21154.202), "in a canyon walled off from the entrance's (y 6; the nearest floor a player reaches 441 across at y 89)"),
    (167269, 'inferno drake', (21255.647, 139.695, 21530.091), 'on a mesa top far over the canyon (y 140; the nearest floor a player reaches 404 across at y 83)'),
    (167270, 'inferno drake', (20199.845, 81.255, 21425.95), "in a canyon walled off from the entrance's (y 81; the nearest floor a player reaches 278 across at y 103)"),
    (167272, 'plague griffon', (21072.035, 113.613, 21655.274), 'on a mesa top far over the canyon (y 113; the nearest floor a player reaches 461 across at y 83)'),
    (167273, 'crag spider', (21430.153, 135.235, 21354.558), 'on a mesa top far over the canyon (y 135; the nearest floor a player reaches 397 across at y 104)'),
    (167275, 'pox griffon', (21406.322, 88.71, 21706.861), 'on a mesa top far over the canyon (y 89; the nearest floor a player reaches 636 across at y 83)'),
    (167276, 'pox griffon', (21399.675, 90.14, 21690.265), 'on a mesa top far over the canyon (y 90; the nearest floor a player reaches 618 across at y 83)'),
    (167277, 'torrid drake', (21394.356, 85.498, 21707.97), 'on a mesa top far over the canyon (y 85; the nearest floor a player reaches 630 across at y 83)'),
    (167278, 'inferno drake', (20649.842, 33.431, 21703.148), "in a canyon walled off from the entrance's (y 32; the nearest floor a player reaches 396 across at y 109)"),
    (167279, 'plague griffon', (20656.184, 38.13, 21696.103), "in a canyon walled off from the entrance's (y 38; the nearest floor a player reaches 391 across at y 109)"),
    (167284, 'calescent ant', (21849.312, 50.328, 21265.442), "in a canyon walled off from the entrance's (y 50; the nearest floor a player reaches 608 across at y 89)"),
    (167285, 'singetail scropion', (21845.103, 50.21, 21251.274), "in a canyon walled off from the entrance's (y 50; the nearest floor a player reaches 594 across at y 89)"),
    (167286, 'pox griffon', (21842.118, 51.127, 21292.41), "in a canyon walled off from the entrance's (y 51; the nearest floor a player reaches 630 across at y 89)"),
    (167287, 'inferno drake', (21839.814, 51.098, 21243.129), "in a canyon walled off from the entrance's (y 51; the nearest floor a player reaches 584 across at y 89)"),
    (167290, 'plague griffon', (20293.318, 108.597, 21594.515), 'on a mesa top far over the canyon (y 109; the nearest floor a player reaches 328 across at y 109)'),
    (167294, 'calescent ant', (20699.294, 97.878, 21410.971), "in a canyon walled off from the entrance's (y 99; the nearest floor a player reaches 162 across at y 109)"),
    (167295, 'torrid drake', (20973.132, 132.413, 21774.032), 'on a mesa top far over the canyon (y 132; the nearest floor a player reaches 578 across at y 42)'),
    (167296, 'inferno drake', (20985.308, 130.474, 21766.926), 'on a mesa top far over the canyon (y 130; the nearest floor a player reaches 571 across at y 46)'),
    (167297, 'calescent ant', (20972.555, 137.337, 21752.764), 'on a mesa top far over the canyon (y 137; the nearest floor a player reaches 557 across at y 42)'),
    (167298, 'torrid drake', (20982.059, 127.499, 21777.683), 'on a mesa top far over the canyon (y 127; the nearest floor a player reaches 582 across at y 44)'),
    (167299, 'torrid drake', (20957.03, 134.543, 21784.751), 'on a mesa top far over the canyon (y 134; the nearest floor a player reaches 589 across at y 40)'),
    (167305, 'pox griffon', (21768.854, 53.482, 21830.215), "in a canyon walled off from the entrance's (y 53; the nearest floor a player reaches 964 across at y 104)"),
    (167306, 'torrid drake', (21770.335, 56.419, 21813.682), "in a canyon walled off from the entrance's (y 56; the nearest floor a player reaches 953 across at y 104)"),
    (167307, 'pox griffon', (21122.472, 10.214, 21392.598), "in a canyon walled off from the entrance's (y 10; the nearest floor a player reaches 218 across at y 83)"),
    (167308, 'inferno drake', (21114.182, 8.169, 21396.114), "in a canyon walled off from the entrance's (y 8; the nearest floor a player reaches 218 across at y 83)"),
    (167309, 'inferno drake', (20449.857, 119.271, 21756.662), 'on a mesa top far over the canyon (y 119; the nearest floor a player reaches 447 across at y 108)'),
    (167310, 'plague griffon', (20458.748, 122.165, 21752.138), 'on a mesa top far over the canyon (y 122; the nearest floor a player reaches 442 across at y 103)'),
    (167325, 'inferno drake', (21423.985, 146.495, 21491.204), 'on a mesa top far over the canyon (y 146; the nearest floor a player reaches 480 across at y 104)'),
    (167326, 'inferno drake', (21430.345, 145.765, 21505.551), 'on a mesa top far over the canyon (y 146; the nearest floor a player reaches 495 across at y 104)'),
    (167327, 'pox griffon', (21437.634, 143.72, 21483.906), 'on a mesa top far over the canyon (y 144; the nearest floor a player reaches 485 across at y 104)'),
    (167328, 'torrid drake', (21281.5, 117.265, 21367.485), 'on a mesa top far over the canyon (y 117; the nearest floor a player reaches 293 across at y 104)'),
    (167329, 'crag spider', (21293.849, 115.79, 21364.858), 'on a mesa top far over the canyon (y 115; the nearest floor a player reaches 299 across at y 104)'),
    (167330, 'inferno drake', (21275.54, 114.193, 21360.261), 'on a mesa top far over the canyon (y 114; the nearest floor a player reaches 284 across at y 104)'),
    (167331, 'inferno drake', (20794.776, 34.673, 21776.022), "in a canyon walled off from the entrance's (y 34; the nearest floor a player reaches 514 across at y 109)"),
    (167340, 'singetail scropion', (21800.0, 111.26713562011719, 20925.0), "in a canyon walled off from the entrance's (y 111; the nearest floor a player reaches 296 across at y 95)"),
    (167341, 'crag spider', (21216.228, 71.512, 21678.076), "in a canyon walled off from the entrance's (y 72; the nearest floor a player reaches 518 across at y 83)"),
    (167342, 'inferno drake', (21224.92, 71.947, 21676.343), "in a canyon walled off from the entrance's (y 72; the nearest floor a player reaches 519 across at y 83)"),
    (167343, 'calescent ant', (21203.322, 74.763, 21677.582), "in a canyon walled off from the entrance's (y 74; the nearest floor a player reaches 512 across at y 83)"),
    (167344, 'pox griffon', (21803.028, 103.696, 20625.907), "in a canyon walled off from the entrance's (y 103; the nearest floor a player reaches 149 across at y 7)"),
    (167345, 'pox griffon', (21798.2, 108.615, 20617.902), "in a canyon walled off from the entrance's (y 109; the nearest floor a player reaches 144 across at y 7)"),
    (167346, 'pox griffon', (21802.266, 108.167, 20608.658), "in a canyon walled off from the entrance's (y 109; the nearest floor a player reaches 149 across at y 7)"),
    (167347, 'calescent ant', (21811.589, 102.487, 20607.052), "in a canyon walled off from the entrance's (y 102; the nearest floor a player reaches 158 across at y 7)"),
    (167348, 'inferno drake', (21803.225, 100.978, 20645.856), "in a canyon walled off from the entrance's (y 100; the nearest floor a player reaches 149 across at y 9)"),
]


def rows(db, sql, args=()):
    cur = db.execute(sql, args)
    cols = [d[0] for d in cur.description]
    return [dict(zip(cols, r)) for r in cur.fetchall()]


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def npc(db, rid):
    r = rows(db, "SELECT * FROM npcs WHERE id = ?", (rid,))
    return r[0] if r else None


def at(r, xyz):
    return all(r[c] is not None and abs(r[c] - v) <= TOL for c, v in zip(("x_coord", "y_coord", "z_coord"), xyz))


def where(r):
    return f"({r['x_coord']:.2f}, {r['y_coord']:.2f}, {r['z_coord']:.2f})"


def go_json(m):
    return json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def find(db, rid, name, xyz):
    """(row, how): the row by id, or by name and place in world 14; None and why not."""
    r = npc(db, rid)
    if r is not None and r["npc_name"] == name and r["world"] == WORLD:
        return r, ""
    alt = [a for a in rows(db, "SELECT * FROM npcs WHERE world = ? AND npc_name = ? AND abs(x_coord - ?) <= ? AND "
                               "abs(z_coord - ?) <= ?", (WORLD, name, xyz[0], TOL, xyz[2], TOL))]
    if len(alt) == 1:
        return alt[0], f" (row {alt[0]['id']} here)"
    if r is None:
        return None, "no such row"
    return None, f"the row is {r['npc_name']!r} in world {r['world']}, not as surveyed"


def plan(db):
    """(moves, removes, done, left)."""
    moves, removes, done, left = [], [], [], []
    for rid, name, frm, to, why in MOVES:
        r, how = find(db, rid, name, frm)
        if r is None:
            r, how2 = find(db, rid, name, to)
            if r is not None and at(r, to):
                done.append(f"{rid} {name}{how2}: at {where(r)} already")
            else:
                left.append(f"{rid} {name}: {how}")
        elif at(r, to):
            done.append(f"{rid} {name}{how}: at {where(r)} already")
        elif at(r, frm):
            moves.append((r, name, to, why))
        else:
            left.append(f"{rid} {name}{how}: at {where(r)}, moved since the survey (it stood at {frm}): left for a GM")
    for rid, name, frm, why in REMOVES:
        r, how = find(db, rid, name, frm)
        if r is None:
            (done if how == "no such row" else left).append(f"{rid} {name}: {how}")
        elif at(r, frm):
            removes.append((r, name, why))
        else:
            left.append(f"{rid} {name}{how}: at {where(r)}, moved since the survey (it stood at {frm}): left for a GM")
    return moves, removes, done, left


def keyed(db, rid):
    """The rows keyed "#n" by a row: its npctemplate and npcloot (table, key columns, row)."""
    out = []
    if table(db, "npctemplate"):
        out += [("npctemplate", ("name",), r) for r in rows(db, "SELECT * FROM npctemplate WHERE name = ?", (f"#{rid}",))]
    if table(db, "npcloot"):
        out += [("npcloot", ("npc_name", "tablename"), r)
                for r in rows(db, "SELECT * FROM npcloot WHERE npc_name = ?", (f"#{rid}",))]
    return out


def log(db, tbl, keys, before, after, why):
    db.execute(f"INSERT INTO {LOG} (tbl, keycols, before, after, why) VALUES (?, ?, ?, ?, ?)",
               (tbl, json.dumps(list(keys)), json.dumps(before, sort_keys=True) if before is not None else None,
                json.dumps(after, sort_keys=True) if after is not None else None, why))


def audit(db, action, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() \
        if table(db, "accountinfo") else None
    now = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', ?, 'npcs', ?, ?, ?, ?)",
               (now, acct[0] if acct else 0, WHO, VIA, action, go_json({"tool": "boxcanyon_reach_20261115", "log": LOG}),
                go_json(before), go_json(after), detail))


def insert(db, tbl, r):
    cols = list(r)
    db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [r[c] for c in cols])


def current(db, tbl, keys, r):
    got = rows(db, f"SELECT * FROM {tbl} WHERE {' AND '.join(f'{k} = ?' for k in keys)}", [r[k] for k in keys])
    return got[0] if got else None


def do_apply(db, moves, removes):
    db.execute(f"""CREATE TABLE IF NOT EXISTS {LOG} (
        seq     INTEGER PRIMARY KEY AUTOINCREMENT,
        tbl     TEXT NOT NULL,
        keycols TEXT NOT NULL,          -- the key columns (JSON list)
        before  TEXT,                   -- the row before (JSON; NULL: none)
        after   TEXT,                   -- the row as written (JSON; NULL: deleted)
        why     TEXT NOT NULL DEFAULT ''
    )""")
    for before, name, to, why in moves:
        cur = db.execute("UPDATE npcs SET x_coord = ?, y_coord = ?, z_coord = ? WHERE id = ? AND abs(x_coord - ?) <= ? "
                         "AND abs(y_coord - ?) <= ? AND abs(z_coord - ?) <= ?",
                         (to[0], to[1], to[2], before["id"], before["x_coord"], TOL, before["y_coord"], TOL,
                          before["z_coord"], TOL))
        if cur.rowcount != 1:
            raise RuntimeError(f"{before['id']} changed under the transaction")
        log(db, "npcs", ("id",), before, npc(db, before["id"]), why)
    for before, name, why in removes:
        for tbl, keys, r in keyed(db, before["id"]):
            db.execute(f"DELETE FROM {tbl} WHERE {' AND '.join(f'{k} = ?' for k in keys)}", [r[k] for k in keys])
            log(db, tbl, keys, r, None, why)
        db.execute("DELETE FROM npcs WHERE id = ?", (before["id"],))
        log(db, "npcs", ("id",), before, None, why)
    audit(db, "update", {"moved": 0, "removed": 0},
          {"moved": [m[0]["id"] for m in moves], "removed": [r[0]["id"] for r in removes]},
          f"{DETAIL}: {len(moves)} moved, {len(removes)} removed")


def do_undo(db):
    if not table(db, LOG):
        print("nothing to undo")
        return 0
    back = kept = 0
    for seq, tbl, keycols, before, after in db.execute(
            f"SELECT seq, tbl, keycols, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        keys = json.loads(keycols)
        was = json.loads(before) if before else None
        wrote = json.loads(after) if after else None
        ref = wrote if wrote is not None else was
        now = current(db, tbl, keys, ref)
        if wrote is not None and (now is None or json.dumps(now, sort_keys=True) != json.dumps(wrote, sort_keys=True)):
            kept += 1
            print(f"  left: {tbl} {[ref[k] for k in keys]} changed since it was written")
            continue
        if wrote is None and now is not None:
            kept += 1
            print(f"  left: {tbl} {[ref[k] for k in keys]} is there again (a row made since)")
            continue
        if wrote is not None:
            db.execute(f"DELETE FROM {tbl} WHERE {' AND '.join(f'{k} = ?' for k in keys)}", [ref[k] for k in keys])
        insert(db, tbl, was)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if kept == 0:
        db.execute(f"DROP TABLE {LOG}")
    audit(db, "update", {}, {"put_back": back, "left": kept}, "undo: " + DETAIL)
    print(f"put back {back} rows, {kept} left (changed since)")
    return 0 if kept == 0 else 1


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    try:
        for t in ("npcs", "gm_audit"):
            if not table(db, t):
                print(f"refused, nothing done: no {t} table here")
                return 1
        db.execute("BEGIN IMMEDIATE")
        try:
            if a.undo:
                rc = do_undo(db)
                db.execute("COMMIT")
                return rc
            moves, removes, done, left = plan(db)
            print(f"Box Canyon (world {WORLD}): {len(moves)} to move, {len(removes)} to remove, {len(done)} as this tool "
                  f"has them already, {len(left)} left alone")
            for r, name, to, why in moves:
                print(f"  move   {r['id']} {name}: {where(r)} -> ({to[0]}, {to[1]}, {to[2]}): {why}")
            for r, name, why in removes:
                print(f"  remove {r['id']} {name} at {where(r)}: {why}")
            for d in done:
                print("  ok:    " + d)
            for x in left:
                print("  left:  " + x)
            if not moves and not removes:
                print("nothing to do" + (" (applied already)" if table(db, LOG) else ""))
                db.execute("ROLLBACK")
                return 0
            if not a.apply:
                print(f"report only: {len(moves)} would be moved, {len(removes)} removed (--apply)")
                db.execute("ROLLBACK")
                return 0
            do_apply(db, moves, removes)
            db.execute("COMMIT")
            print(f"applied: {len(moves)} moved, {len(removes)} removed, in gm_audit and {LOG}; the GM panel's "
                  "reload_npcs (or a restart) for the next instance opened")
            return 0
        except BaseException:
            if db.in_transaction:
                db.execute("ROLLBACK")
            raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
