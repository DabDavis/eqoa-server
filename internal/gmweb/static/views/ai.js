import { h, mount, pageHead, button, toast, card, copyButton, confirmBox, formBox, empty } from '../lib/dom.js';
import { api } from '../lib/api.js';
import { isAdmin } from '../lib/nav.js';
import { ago, when } from '../lib/fmt.js';

// Working with an AI agent: what to paste where for ChatGPT, Claude or any
// MCP client, and the GM's own tokens.
export async function render(box) {
  const base = location.origin + location.pathname.replace(/\/$/, '');
  const mcp = base + '/mcp';
  const openapi = base + '/api/openapi.json';
  const tokens = h('div');

  const urlRow = (label, url) => h('div', { class: 'stack' }, h('strong', null, label), h('div', { class: 'secret-box' }, h('code', null, url), copyButton(url)));
  const codeBlock = (text) => h('div', { class: 'secret-box' }, h('pre', { class: 'json grow' }, text), copyButton(text));

  mount(box,
    pageHead('AI agents', 'An AI agent works here as you: it signs in with your GM account (or a token you make), uses the same tools as these pages, and everything it changes is in the audit log under your name, revertible.'),
    h('div', { class: 'cols' },
      card('ChatGPT', h('div', { class: 'stack' },
        h('ol', { class: 'steps' },
          h('li', null, 'In ChatGPT: Settings → Apps & Connectors → Advanced settings: turn on Developer mode (on a Business, Enterprise or Edu workspace an admin may have to allow it).'),
          h('li', null, 'Settings → Apps & Connectors → Create: name it "EQOA GM", paste the MCP address below, authentication OAuth (leave the client ID and secret empty).'),
          h('li', null, 'ChatGPT opens this server\'s sign-in: sign in with your GM account and choose what it may do.'),
          h('li', null, 'In a chat: + → More → EQOA GM, then ask: "who is online?", "make Guard Dahvel level 35"...')),
        h('p', { class: 'muted small' }, 'Some ChatGPT plans give a connector read-only (search and fetch) use; a custom GPT (below) can change things on those.'),
        urlRow('MCP address', mcp))),
      card('Claude', h('div', { class: 'stack' },
        h('ol', { class: 'steps' },
          h('li', null, 'In Claude: Settings → Connectors → Add custom connector.'),
          h('li', null, 'Paste the MCP address below, then Connect: sign in with your GM account here.'),
          h('li', null, 'Claude Code: claude mcp add --transport http eqoa-gm ' + mcp)),
        urlRow('MCP address', mcp)))),
    card('Codex, or any agent in a terminal', h('div', { class: 'stack' },
      h('p', null, 'Codex (and Claude Code, and most coding agents) take this server as a remote MCP server. Either sign in through the browser, or give it a token.'),
      h('strong', null, 'Sign in with your GM account'),
      codeBlock(`codex mcp add eqoa-gm --url ${mcp}\ncodex mcp login eqoa-gm`),
      h('p', { class: 'muted small' }, 'The login opens this server\'s sign-in in your browser (on the same machine: the answer comes back to a port Codex opens on 127.0.0.1).'),
      h('strong', null, 'Or with a token (a terminal with no browser, or over SSH)'),
      codeBlock(`read -rp 'GM account: ' U; read -rsp 'Password: ' P; echo; read -rp 'Authenticator code: ' C\nexport EQOA_GM_TOKEN=$(curl -s ${base}/api/auth/token -H 'Content-Type: application/json' \\\n  -d "{\\"username\\":\\"$U\\",\\"password\\":\\"$P\\",\\"code\\":\\"$C\\",\\"name\\":\\"codex\\"}" | sed -nE 's/.*"token": *"([^"]+)".*/\\1/p'); unset P C\necho \${EQOA_GM_TOKEN:+signed in}`),
      h('p', { class: 'muted small' }, 'That signs in once (the password is not echoed or kept in the history; the code is the six digits your authenticator app shows) and holds a 30-day token in the variable, for this shell; "signed in" says it worked. Then in ~/.codex/config.toml:'),
      codeBlock(`[mcp_servers.eqoa-gm]\nurl = "${mcp}"\nbearer_token_env_var = "EQOA_GM_TOKEN"`),
      h('p', { class: 'muted small' }, 'The token is listed below and can be revoked there.'))),
    card('A custom GPT, or any program', h('div', { class: 'stack' },
      h('ol', { class: 'steps' },
        h('li', null, 'Make a token below ("write" lets it change things).'),
        h('li', null, 'In the GPT editor: Configure → Create new action → Import from URL: the OpenAPI address below (the short list if it refuses the long one).'),
        h('li', null, 'Authentication: API Key, Auth Type Bearer, and paste the token.'),
        h('li', null, 'In the GPT\'s instructions, tell it to call guide first.')),
      urlRow('OpenAPI', openapi), urlRow('OpenAPI, short list', openapi + '?core=1'),
      h('p', { class: 'muted small' }, 'Any MCP client that takes a header can also use a token: Authorization: Bearer <token>, at the MCP address.'))),
    card('Your tokens and connected agents', tokens, button('New token', { icon: 'plus', class: 'small primary', onclick: () => newToken() })),
    card('What agents are told', h('p', null, 'An agent is told to read the guide first (', h('a', { href: 'api/guide', target: '_blank', rel: 'noopener' }, 'read it'),
      '): how the data fits together, its conventions, what takes effect when, and to ask before deleting, banning, kicking or changing many rows.')));

  async function load() {
    try {
      const out = await api('api/tokens' + (isAdmin() ? '?all=1' : ''));
      const list = out.tokens;
      mount(tokens, list.length ? h('div', { class: 'table-scroll' }, h('table', { class: 'plain' },
        h('thead', null, h('tr', null, ...['Name', 'Kind', 'Scope', isAdmin() ? 'GM' : null, 'Made', 'Last used', 'Expires', ''].filter(Boolean).map((t) => h('th', null, t)))),
        h('tbody', null, ...list.map((t) => h('tr', null,
          h('td', null, t.name, h('div', { class: 'faint small mono' }, t.prefix + '…')),
          h('td', null, t.kind === 'oauth' ? 'connected agent' : 'API token'),
          h('td', null, h('span', { class: 'badge ' + (t.scope === 'admin' ? 'warn' : t.scope === 'write' ? 'info' : '') }, t.scope)),
          isAdmin() ? h('td', null, t.username) : null,
          h('td', { title: when(t.created) }, ago(t.created)),
          h('td', { title: t.lastIP || '' }, t.lastUsed ? ago(t.lastUsed) : 'never'),
          h('td', null, t.revoked ? h('span', { class: 'badge danger' }, 'revoked') : t.expires ? when(t.expires) : 'never'),
          h('td', null, t.revoked ? null : button('Revoke', { class: 'small danger', onclick: async () => {
            if (!(await confirmBox({ title: 'Revoke ' + t.name + '?', message: 'Whatever uses it stops working at once.', confirmLabel: 'Revoke', danger: true }))) return;
            try { await api('api/tokens', { action: 'revoke', id: t.id }); toast('Revoked', 'ok'); load(); } catch (e) { toast(e.message, 'error'); }
          } })))))))
        : empty('No tokens or connected agents yet.'));
    } catch (e) { mount(tokens, h('p', { class: 'err' }, e.message)); }
  }

  async function newToken() {
    const v = await formBox({
      title: 'New API token', submitLabel: 'Make it',
      intro: 'It acts as you, within the scope you choose, until it expires or you revoke it. You see it once.',
      fields: [
        { name: 'name', label: 'What it is for', placeholder: 'ChatGPT custom GPT' },
        { name: 'scope', label: 'It may', type: 'select', value: 'write', options: [
          { value: 'read', label: 'look only' }, { value: 'write', label: 'look and change game data and players' },
          ...(isAdmin() ? [{ value: 'admin', label: 'all that, and an admin\'s tools' }] : [])] },
        { name: 'days', label: 'Expires after', type: 'select', value: '90', options: [
          { value: '7', label: '7 days' }, { value: '30', label: '30 days' }, { value: '90', label: '90 days' }, { value: '365', label: 'a year' }] },
      ],
    });
    if (!v) return;
    try {
      const out = await api('api/tokens', { action: 'create', name: v.name, scope: v.scope, days: Number(v.days) });
      const { dialog } = await import('../lib/dom.js');
      dialog({
        title: 'Your new token',
        body: h('div', { class: 'stack' },
          h('p', { class: 'notice warn' }, 'Copy it now: it is not shown again. Anyone with it works as you.'),
          h('div', { class: 'secret-box' }, h('code', null, out.token), copyButton(out.token)),
          h('p', { class: 'muted small' }, 'Send it as the header Authorization: Bearer <token>.')),
        actions: [{ label: 'Done', kind: 'primary' }],
      });
      load();
    } catch (e) { toast(e.message, 'error'); }
  }
  await load();
}
