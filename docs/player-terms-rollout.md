# EQOA Classic Player Terms

This setting activates EQOA Classic’s own player agreement, privacy notice and account acceptance form. Only the operators of that service should enable it. The retail disc license continues to appear during startup.

## Publish on the account authority

Route public HTTPS requests for the exact paths `/terms` and `/privacy` to the primary account HTTP server. Forward GET, HEAD and POST, including cookies and the original Host and scheme. Disable form-body logging and caching on these routes. Keep the normal account recovery and email verification routes available. For example, the following location fits the main HTTPS server in [the hosting guide](hosting.md).

```nginx
location ~ ^/(terms|privacy)$ {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_cache off;
    client_max_body_size 8k;
}
```

Put a **Read and accept Player Terms** link to `/terms` and a **Privacy Notice** link to `/privacy` on the public homepage/footer, browser play sign-in and any separately hosted account pages. Point public **Create account** links to `/terms?mode=create`. The built-in account website provides the links automatically.

## What players see before choosing a shard

The [public site patch](player-terms-public-site.patch) updates the homepage, PS2 instructions and server guide as served on November 24. From a staging copy of the website root, use `git apply --check /path/to/player-terms-public-site.patch` before applying it. If a page is generated or has changed since then, carry the same edits into its maintained source. Publish those pages and the updated browser play page together with the working account routes.

Keep the two names in the server picker. Use this explanation in the guide, PS2 setup instructions and browser play page, where players decide which character to level:

> **Vanilla Test - Wipes** (Live) characters and their progress will be wiped for testing. **Sandbox Test - No Wipes** (Heirloom) has no planned character wipes. Bugs or rollbacks can still erase progress on either server.

On the public PS2 page, replace the first-login account creation instruction with “Create an account or accept the current Player Terms at eqoaclassic.com/terms on a phone or computer. Then sign in to the game.” Put the terms and privacy links beside sign-in on the browser play page and in the public footer. For the homepage, describe the price and duration as “EQOA Classic will remain free for as long as we operate it.” Keep account help and private operator contact available from outside the game. The Station patch directs users to the account website. Clients that show a generic login error can use the same published link.

## Enable both shards together

1. Build the same server revision for the account authority and every game shard. Take the normal restricted recovery backup and retain the previous server binary. Keep public gameplay in maintenance while the two processes are switched.
2. Set `auth.require_player_terms: true` in both shard configurations. Keep the secondary’s existing `auth.shared_db` pointed at the authority. All acceptances are written on the primary. The secondary reads a current receipt there at sign-in, character selection and session resume. A read error denies entry.
3. Start the primary with its account HTTP service, verify the two public HTTPS routes and then start the secondary. Confirm the current Station patch is served, and reevaluate or end sessions on both processes. Portal registration and acceptance use the same account store. An existing game account has no receipt until its owner explicitly agrees.
4. Before reopening, use operator-controlled accounts. On **each** shard verify that no receipt, a declined or unchecked first agreement, and a v1.3-only receipt all refuse play, including reconnect and carried sessions. From signed-out HTTPS, read both documents, create a new eligible account, verify its recovery email path and enter both shards. Have the returning account explicitly accept v1.4 once, then enter both shards. Check that an under-13 answer ends signup before credentials. On native, browser and retail-compatible clients check the actual original SOE disc screen and the separate project terms link. Record both build IDs, the version shown and each shard’s refusal and admission results before opening play.
5. Keep maintenance in place if a path admits an unaccepted account or the public form cannot record acceptance. A binary rollback predating the gate needs the same access restriction. Leave successful receipts intact for retry.

## Records and updates

Player Terms v1.4 and Privacy Notice v1.1 appear together. On both account forms, verify that the unchecked box follows the character-wipe explanation and the notice about permanent loss, service availability, lawful GM discretion, the damages exclusion and the US $250 fallback for those ordinary game losses. Check the exclusion and exceptions links. Record the exact release, publication date and a copy of the rendered acceptance screen with the terms for that release. Earlier receipts remain tied to their original versions.

The primary SQLite database stores one UTC timestamp per account and terms version in `account_terms_acceptances`. Repeated clicks keep the original timestamp. Sanitized public snapshots clear this table. An account deletion removes its receipts. Preserve only the specific records subject to a documented legal hold before deleting the account.

For a material change, add a new versioned agreement file, update the active template and `PlayerTermsVersion` and deploy the revision to every shard together. Keep the old published text in version history. Earlier receipts continue to describe the version the player actually accepted.
