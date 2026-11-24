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

Put a **Read and accept Player Terms** link to `/terms` and a **Privacy Notice** link to `/privacy` on the public homepage/footer, browser play sign-in and any separately hosted account pages. Point public **Create account** links to `/terms`. The built-in account website provides the links automatically.

## Enable both shards together

1. Build the same server revision for the account authority and every game shard. Take the normal restricted recovery backup and retain the previous server binary. Keep public gameplay in maintenance while the two processes are switched.
2. Set `auth.require_player_terms: true` in both shard configurations. Keep the secondary’s existing `auth.shared_db` pointed at the authority. All acceptances are written on the primary. The secondary reads a current receipt there at sign-in, character selection and session resume. A read error denies entry.
3. Start the primary with its account HTTP service, verify the two public HTTPS routes and then start the secondary. Confirm the current Station patch is served, and reevaluate or end sessions on both processes. Portal registration and acceptance use the same account store. An existing game account has no receipt until its owner explicitly agrees.
4. Before reopening, use a controlled account on both shards. Check rejection on first login, after a decline, through a reconnect and for a carried session. Open both documents without signing in. Complete the agreement for an existing account on a phone or computer, then sign in on both shards. Create a new eligible account through the public link and verify its recovery email path. Check the under-13 stop on a separate clean browser. Use the native client, browser game and a retail-compatible client to verify the visible prompts and the original disc license. Record the tested builds and shard results privately.
5. Keep maintenance in place if a path admits an unaccepted account or the public form cannot record acceptance. A binary rollback predating the gate needs the same access restriction. Leave successful receipts intact for retry.

## Records and updates

Player Terms v1.3 requires an explicit choice to accept the ordinary risks of free, as-available play. On both signup and returning-account forms, check that the unchecked box follows the notice about bugs, permanent loss of progress, uptime and access, GM discretion, the money-damages exclusion and the US $250 fallback. Check the links to the full exclusion and exceptions. Verify that an account with only a v1.2 receipt is refused on both shards, then accepts v1.3 explicitly and can sign in to both. Keep earlier text and receipts associated with their original versions.

The primary SQLite database stores one UTC timestamp per account and terms version in `account_terms_acceptances`. Repeated clicks keep the original timestamp. Sanitized public snapshots clear this table. An account deletion removes its receipts. Preserve only the specific records subject to a documented legal hold before deleting the account.

For a material change, add a new versioned agreement file, update the active template and `PlayerTermsVersion` and deploy the revision to every shard together. Keep the old published text in version history. Earlier receipts continue to describe the version the player actually accepted.
