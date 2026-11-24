# Reviewing tickets for Live/Vanilla

One ticket can contain several changes. Certify each claim against the original EQOA service, name one destination, and give the release operator the exact fields or code revision to review. The work board remains the place to coordinate ownership. Keep the editable JSON ledger with the project leads. Its history and approval references are part of the release record.

## Intake and routing

1. Import a bounded GM export using its actual source. Petitions have independent number spaces on Live and Heirloom. The work board is shared. A repeated import preserves existing claims. Intake copies identifiers only. Open the original ticket to review its contents.
2. Split distinct requests into named claims. For example, an item correction, a Heirloom reward multiplier and a player's inventory repair each receive a disposition. Include the filing ID and a short stable suffix. Record related petition and board IDs together in the claim.
3. Give each claim a lane. `retail` requires evidence for the original-service behavior. `heirloom-only` covers custom mechanics and sandbox experiments. `live-policy` covers deliberate local rules on Live. `operations` covers service reliability, moderation and individual player work. `unreviewed` keeps new or ambiguous requests visible.
4. For a retail claim, set the intended original-service period and record what each source establishes. Use a film timestamp, patch paragraph, guide page or archived item field. Distinguish observation, publication, capture and gameplay-effective dates. Compare patches for the same era. Resolve disagreements against the project's source ranking and record outstanding ones in `conflicts`.
5. Group all mutually dependent changes into one release. Use `holds` to preserve a parent ticket's release order, missing dependencies or a pending decision. A reverse transfer gets a separate claim, destination and approval. Link a replaced claim through `superseded_by` to retain its history.

From the server repository root, a compact SQL export with `columns` and `rows`, a GM petition list, or a board list can feed intake. Begin with the project's existing ledger, or a version 1 ledger containing an empty `claims` array.

~~~sh
python tools/vanilla_tickets.py intake review.json petitions.json --source heirloom:petition --output next-review.json
python tools/vanilla_tickets.py queue next-review.json --target live --certified
python tools/vanilla_tickets.py queue next-review.json --target live --snapshot live-copy.db --ready
~~~

The certified view includes held claims and verification-only records. The ready view opens the supplied snapshot read-only and selects approved data changes that still need a write. Use `--json` for the decision, digest, holds and destination check. Running `queue` without filters shows the other lanes and evidence gaps.

## The review record

The Firewood Staff is a real example. Work 2042 specifies a coordinated Live release. Its item record provides evidence for two fields. The archive capture establishes that the values were documented by March 28, 2004. The exact patch that introduced them is unknown. [Archived Allakhazam item 1269](https://web.archive.org/web/20040328194853/http://eqoa.allakhazam.com:80/item.html?eqoaitem=1269)

~~~json
{
  "id": "work:2042/firewood-staff",
  "target": "live",
  "lane": "retail",
  "delivery": "data",
  "claim": "Restore the Firewood Staff to 2600 maximum item HP and Dark Elf eligibility.",
  "period": "Neriak level 10 wizard reward documented by March 28, 2004.",
  "sources": [{
    "kind": "era-database",
    "as_of": "2004-03-28",
    "lineage": "Allakhazam item 1269",
    "locator": "https://web.archive.org/web/20040328194853/http://eqoa.allakhazam.com:80/item.html?eqoaitem=1269",
    "supports": "The Races and Max Item HP fields give Dark Elf and 2600."
  }],
  "conflicts": [],
  "holds": ["Work 2042 requires a single coordinated release."],
  "checks": [],
  "changes": [{
    "table": "itempattern",
    "key": {"patternid": 5628},
    "match": {"itemname": "Firewood Staff", "levelreq": 10},
    "before": {"maxhp": 2000, "raceuse": 1023},
    "after": {"maxhp": 2600, "raceuse": 4}
  }]
}
~~~

Accepted direct source kinds are `soe`, `original-recording`, `original-disc`, `official-guide`, `era-database` and `era-guide`. When relying on `era-testimony`, record at least two independently sourced accounts with distinct locators. Assign copied material its original lineage. Petitions, current shard values, emulator data, Sandstorm, Return Home and post-service recollections can supply leads. A reviewer must open the cited material and check the identity, field, value and period. The software checks the record's completeness and decision binding.

Use `queue --json` to read the claim digest. After verification, add `review` with `by`, `date` in YYYY-MM-DD, `ref` to the review, and `digest`. For release, record a separate `approval` with the same four fields and a reference to the owner's explicit destination-specific decision. Editing a claim, its destination, sources, fields or holds invalidates its previous digest. The operator trusts a ledger controlled by the project leads. A digest detects edits. Authorship and permission come from the referenced decision and access to that ledger.

For data updates, supply a real destination primary key and the exact changed fields. Add a `match` guard using the item or spell name, the quest name and giver, or the NPC name, world and position. Inserts and deletes take a full destination row. Encoded effect definitions and quest scripts need review of every changed value and embedded reference. Put target-local dependencies in `checks` as `table`, `key`, `expect`. For typed references, identify the parent in a check or earlier change. Give references the destination's IDs. Complete the dependency and key mapping before removing a bundle hold.

For a retail code fix, use `delivery` of `code` and record `patch.commit` as the full Git commit, `patch.scope` as the behavior and target, and `patch.tests` as the test receipt. This routes the revision to the owner for code review and the ordinary build pipeline. Keep shared-binary Heirloom features behind the shard's configuration and test both configurations. Operational fixes use their own lane.

## Filter Heirloom findings for Live review

The read-only `shortlist` command works from a recorded comparison of both shards. Run this command from the server repository to filter the November 23 audit.

~~~sh
python tools/vanilla_tickets.py shortlist ../../content-research/reports/junior-dev-audit-20261123/live-transfer-review.json
~~~

Add `--csv` to print the candidate table. Each row gives the proposed Live correction, its evidence, the exact field boundaries and the UTC time of the comparison. The audit ledger also contains a disposition for each excluded finding. A source case can have several dispositions when it mixes an original feature with custom tuning.

For another audit, give each finding a stable source ID. An audit-only ID uses `audit:YYYYMMDD:C01/subject`, or `G1` for a path group. These IDs are accepted by the shortlist command. The release workflow still requires a work or shard-qualified petition ID.

For a proposed transfer, use the ordinary retail claim, evidence, `checks`, `changes` and review digest. Put the paired field readings in `transfer.rows`, the checked values for the Live dependencies in `transfer.live_dependencies`, and record both builds and the read time. Quest steps also carry both parents' names and givers. Encoded numeric event actors need an explicit `npc_pairs` mapping with the two local IDs, names and worlds. The Sarek case in the audit ledger shows an example. Renew the source review after changing a claim or its readings.

The `transfer_scope` section of `tools/shard_overlays.json` identifies which features and fields have shared scope. Any unknown or shard-specific feature, protected named dependency, unresolved conflict, hold, unmatched parent, ambiguous actor ID, destination drift or already-satisfied claim drops out. Add new shared fields only with a recorded scope decision. Moogie's actors, quest and trigger targets are protected by identity even if someone labels their claim as shared. Live's separate Sarek respawn and Erudite item race mask have their own scope.

The shortlist uses the recorded read time. A future release requires fresh destination verification and its own approval.

## Release and verification

The November 15 and 22 comparisons remain available for discovery. Their bulk plans have no release authority. The reviewed path can express an authoritative target value even when both shards currently agree on the wrong value. Destination policy in [shard_overlays.json](../tools/shard_overlays.json) applies to every ledger. The initial rules cover the vendor incident and the named quest reward race masks. Add confirmed exceptions there as the project identifies them.

On a destination snapshot, preview a certified data claim. Adding `--plan` writes a new receipt for a claim whose holds are clear. The operator's release command uses that receipt and the trusted ledger. The normal project approval, backup, maintenance and readback procedure applies to production.

~~~sh
python tools/live_sync_20261122.py --live live-copy.db --tickets review.json --ticket CLAIM
python tools/live_sync_20261122.py --live live-copy.db --tickets review.json --ticket CLAIM --plan receipt.json
python tools/live_sync_20261122.py --live DESTINATION.db --tickets review.json --plan receipt.json --apply
~~~

The writer checks the claim, competing tickets, approval, shard policy, identities and current values inside the release workflow. It commits the ticket as one transaction, checks each changed row, and preserves unrelated target fields. Unknown keys, drift, side effects or active progress on a structural quest change hold the packet. A successful retry finds the requested state already present.

Look up the release in `vanilla_ticket_release` by target, claim and digest. Its run ID joins to `live_sync_1122_log` for exact before/after images and to `gm_audit` through the log's audit ID. Verify the affected fields on the actual destination and verify any cached gameplay behavior after the appropriate reload. A rollback uses `--undo --run RUN_ID` on the named shard. A ticket whose applied rows have since changed is held intact. To reconsider a reverted revision, record the new decision and its reason in a claim `revision` field, then review and approve the new digest.

## Adoption

Point the owner's two-way sync job at the reviewed entry point after testing on real destination copies. Preserve its bulk comparison as a report. Apply the same ticket and evidence requirement to one-off GM data tools and the shared binary's release review. Direct GM edits, one-off scripts and deployments have separate entry points. Record exceptions and their resulting audit or build IDs on the owning work item.
