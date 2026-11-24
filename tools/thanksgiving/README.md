# The King's Last Thanksgiving

An original Heirloom/Sandbox event. A bird who fed Blackwater after the lost years wants thirty-five marks, ten suits of armor and his holiday back. His last demand costs the raid more than the first two.

## The muster at Blackwater

The Turkey King holds court in the open ground at Blackwater. Archivist Quill has a ledger that jumps from “One turkey. No crown.” to a requisition for eight royal chairs. Quartermaster Giblet remembers what happened afterward. A wet bird climbed onto a flour barrel and ordered the granary opened. By supper, people were calling him King. Neither witness can account for the fourteen missing years.

The King remembers Captain Scilond's militia hunting the Grinch. Now Santa has planted a tree in Freeport, too. The King wants his empire back before the next holiday.

| Stop | What the player does |
| --- | --- |
| Turkey King, Blackwater | Accept the muster. Bring him **35 Heirloom Marks** from the same balance shown by `/marks`. Players with the marks already can pay immediately. |
| Captain Tallyspur, Keliner's hill giant camp | Drive six hill giants away from the grain wagons and report back for **7 marks**. |
| Scout Brinebeak, Klik'Anon coast | Clear six deckhands, pirate scouts or pirate stonemasons from the gravy boats. Report back for **7 marks**. They really are boats full of gravy. |
| Turkey King, Blackwater | Hand over 35 marks in one payment. The King uses them to empower his army and sends the player to his armorer. |
| Sergeant Rivetbeak, Tomb City of Envar | Raid the city's ordinary defenders for **10 Envar Field Armor**. Each eligible corpse rolls once at **20%**. The armor stacks to 99 and expires when the season ends. |
| Turkey King, Blackwater | Deliver all ten bundles. The King arms his birds, announces that Santa and the Grinch must die, and asks the player to meet him outside Freeport's west gate. |

The two optional patrols accept any mix of up to five paid reports per character, including level 60. A character starting with zero can earn the tribute in thirty patrol kills. Other eligible sources of Heirloom Marks work as usual. Quill, Giblet and the three field officers each have their own dialogue.

## The west gate

Walking **out** through the gate starts one shared encounter. Further quest-ready players crossing the gate join it. The King faces the gate and shouts across the shard.

> **Turkey King** — Fourteen years I miss dinner, and you two steal my holiday? Listen up, Freeport! This Black Friday the loot Santa and the Grinch drop is about to be HALF OFF!

Santa and the Grinch appear in front of him and strike first. The King fights back and takes each to one hit point. The combat stops. Both remain immune through the exchange.

> **Santa** — I'm sorry. People woke up missing fourteen years. All I wanted was for them to have something to look forward to for the first time since.
>
> **Turkey King** — You holly jolly traitor. ... Keep your beard. But next year you wait until I've cleared the plates.
>
> **The Grinch** — Thanksgiving isn't even a real holiday. It's a long lunch with worse company.

The King strikes the Grinch dead. Santa retreats. The King spots tinsel on Freeport's west wall and pine needles on the players' boots.

> **Turkey King** — That's it. I'm crashing out. I'm going to wipe the floor with every last one of you!

## The raid and the reward

| Mechanic | Rule |
| --- | --- |
| Turkey King | Level 64 with **2,000,000 HP** when he goes hostile. He uses Quintessence of Veeshan's warrior melee and Critical Hit. His area spell is Turkey time. |
| Turkey time | All players in a **35-unit radius** lose **200 STR and 200 DEX** for **10 minutes**. He recasts every **60 seconds**. Each recast refreshes the duration. |
| Leftovers | At **90%, 80%, 70%, 60%, 50%, 40%, 30%, 20% and 10%**, five miniature aviaks join the fight. Each has **1,000 HP**, lands four hits per round at half the King's normal melee damage, and deals **2,000 unresistable damage** to its killer on death. |
| Victory | The corpse contains **20 individual parcels of Stuffing**. A character can carry one. Santa reappears and asks the players to give him time to recover, then meet him by the Freeport tree. |
| Turkey Smasher | Trade one Stuffing to the existing tree Santa for an oversized **790-damage one-handed blunt club**, all classes and races, Lore and No Trade. The item reads exactly **“This procs.. something, but you're not sure what”**. |

The King uses a large aviak skylord body, and the Smasher uses the oversized Mountain Giant's Club model in one hand. The mystery proc has a fixed 1% chance per landed weapon hit. Its destination stays out of dialogue, item inspection and player-facing announcements.

## Heirloom operator handoff

Apply the accompanying source patch to the server repository at `f462af7000aba37c6476c0215811f24c1a5fee1b`. Build `./cmd/gameserver` through the normal deployment process. Run the installer against a Heirloom database copy first. Its default mode rolls back. The explicit apply mode stages the content with Thanksgiving switched off.

```sh
git apply --check thanksgiving.patch
git apply thanksgiving.patch
go build ./cmd/gameserver
python3 tools/thanksgiving/install.py /path/to/heirloom-copy.db
python3 tools/thanksgiving/install.py /path/to/heirloom.db --apply
```

Load the staged data using the patched Heirloom server. Fire GM trigger **341, Thanksgiving on**, to open the court and quests. Trigger **340** belongs to the players' west-gate crossing. Fire **342, Thanksgiving off**, at the chosen end of the event. Closing the season ends active encounters, removes the field armor from inventories and banks, and blocks late pickups from existing corpses. Characters retain their Stuffing and their earned clubs. Only the tree and its Santa are borrowed from Christmas when that season is closed.

### Quartermaster Giblet's Heirloom Marks stock

With the Thanksgiving content already staged and the server built with marks shop support, preview and install the two items with `install_mark_rewards.py`. This works with the season open or closed. It adds two item patterns, two uses and two stock entries in one audited transaction. `--undo` is available before the items enter circulation. Closing Thanksgiving stops sales. Purchased charges remain usable.

```sh
python3 tools/thanksgiving/install_mark_rewards.py --db /path/to/heirloom-copy.db
python3 tools/thanksgiving/install_mark_rewards.py --db /path/to/heirloom.db --apply
```

The handoff includes the event-specific server tests, selected content definitions and test results. From the patched checkout with the ZIP extracted in the parent directory, reproduce the checks with the following commands.

```sh
python3 ../qa/make-fixture.py --repo . --output ../stage-fixture.sql
export EQOA_EVENT_SQL="$(cd .. && pwd)/stage-fixture.sql"
export EQOA_EVENT_PLACES="$PWD/tools/thanksgiving/places.json"
export EQOA_EVENT_GROUND="/path/to/frontiers-collision-bake"
go test ./internal/handlers ./internal/entity ./internal/ground -run 'Test(Thanksgiving|HolidayFootprints)' -count=1
```

Collision checks use locally baked Frontiers terrain, environment and placed walls. The original game assets stay on their owner's machine. A connected-client pilot on the patched shard is the remaining visual acceptance check.
