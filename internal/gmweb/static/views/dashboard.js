import { h, mount, card, pageHead, button, toast, empty } from '../lib/dom.js';
import { op } from '../lib/api.js';
import { go, state } from '../lib/nav.js';
import { duration, ago, bytes } from '../lib/fmt.js';
import { auditLine } from './audit.js';

export async function render(box) {
  const kpis = h('div', { class: 'kpis' });
  const online = h('div');
  const recent = h('div');
  const info = h('div');
  const text = h('input', { type: 'text', placeholder: 'A line every player sees, as from the server', maxlength: '400' });
  const colour = h('select', { 'aria-label': 'Colour' }, ...['yellow', 'white', 'green', 'cyan', 'orange', 'red', 'pink'].map((c) => h('option', { value: c }, c)));
  const send = button('Send to everyone', { class: 'primary', icon: 'send', onclick: async () => {
    if (!text.value.trim()) return;
    try {
      const out = await op('broadcast', { text: text.value.trim(), colour: colour.value });
      toast(`Sent to ${out.sentTo} player${out.sentTo === 1 ? '' : 's'}`, 'ok');
      text.value = '';
    } catch (e) { toast(e.message, 'error'); }
  } });
  text.addEventListener('keydown', (e) => { if (e.key === 'Enter') send.click(); });

  const needTOTP = state.session?.user?.adminNeedsTOTP
    ? h('p', { class: 'notice warn' }, 'You are an admin, and an admin\'s tools (SQL, settings, backups, restart, account levels) need a sign-in with an authenticator code. ',
      h('a', { href: '#/security' }, 'Set one up'), ', then sign in again.')
    : null;
  mount(box,
    pageHead('Dashboard', 'The server now.', null, button('Refresh', { icon: 'refresh', onclick: () => load() })),
    needTOTP,
    kpis,
    h('div', { class: 'cols-2' },
      h('div', { class: 'stack' },
        card('Broadcast', h('div', { class: 'row' }, h('div', { class: 'grow' }, text), colour, send)),
        card('Players online', online, button('All', { class: 'small ghost', onclick: () => go('online') })),
        card('Recent GM activity', recent, button('Audit log', { class: 'small ghost', onclick: () => go('audit') }))),
      card('Server', info)));

  let timer;
  async function load() {
    try {
      const [st, on, log] = await Promise.all([
        op('server_status'), op('online_players'), op('audit_log', { limit: 8 }),
      ]);
      const c = st.counts || {};
      const kpi = (label, value, sub, cls = '') => h('div', { class: 'kpi ' + cls }, h('div', { class: 'label' }, label),
        h('div', { class: 'value' }, value ?? '—'), sub ? h('div', { class: 'sub' }, sub) : null);
      mount(kpis,
        kpi('Players online', st.playersOnline ?? 0, `${st.sessions ?? 0} sessions`, 'live'),
        kpi('NPCs in the world', (st.npcsInWorld ?? 0).toLocaleString(), `${(c.npcRows ?? 0).toLocaleString()} rows, ${st.gmSpawns || 0} GM spawns`),
        kpi('Accounts', (c.accounts ?? 0).toLocaleString()),
        kpi('Characters', (c.characters ?? 0).toLocaleString()),
        kpi('Items', (c.items ?? 0).toLocaleString(), `${(c.merchantStock ?? 0).toLocaleString()} on sale`),
        kpi('Spells', (c.spells ?? 0).toLocaleString()));
      mount(online, on.players.length
        ? h('div', { class: 'table-scroll' }, h('table', { class: 'plain' }, h('thead', null, h('tr', null, h('th', null, 'Character'), h('th', null, 'Level'), h('th', null, 'Where'), h('th', null, 'Account'))),
          h('tbody', null, ...on.players.slice(0, 12).map((p) => {
            const tr = h('tr', null,
              h('td', null, h('a', { href: '#/characters/' + p.serverid }, p.name), p.dead ? h('span', { class: 'badge danger' }, 'dead') : null),
              h('td', null, `${p.level} ${p.race} ${p.class}`),
              h('td', null, `${p.worldName} (${Math.round(p.x)}, ${Math.round(p.z)})`),
              h('td', null, h('a', { href: '#/accounts/' + p.accountid }, p.account)));
            return tr;
          }))))
        : empty('Nobody is in the world.'));
      mount(recent, log.entries.length ? h('div', null, ...log.entries.map((e) => auditLine(e, true))) : empty('No changes yet.'));
      const s = st.settings || {};
      mount(info, h('dl', { class: 'kv' },
        h('dt', null, 'Build'), h('dd', null, h('code', null, st.build)),
        h('dt', null, 'Up'), h('dd', null, duration(st.uptimeSeconds), h('span', { class: 'faint' }, ' since ' + new Date(st.started).toLocaleString())),
        h('dt', null, 'Database'), h('dd', null, bytes(st.databaseBytes)),
        h('dt', null, 'New characters'), h('dd', null, `level ${s.start_level}${s.start_all_spells ? ', every class spell' : ''}`),
        h('dt', null, 'Self-registration'), h('dd', null, s.allow_auto_register ? 'on: an unknown name registers at login' : 'off'),
        h('dt', null, 'NPC lines'), h('dd', null, `${Object.keys(s.npc_text || {}).length} NPCs say their own words`),
        h('dt', null, 'You'), h('dd', null, `${st.you.username}, level ${st.you.level} (${st.you.scope})`)));
    } catch (e) {
      toast(e.message, 'error');
    }
  }
  await load();
  timer = setInterval(load, 15000);
  return { leave: () => clearInterval(timer) };
}

export { ago };
