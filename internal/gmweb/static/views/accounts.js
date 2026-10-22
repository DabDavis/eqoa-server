import { h, mount, card, button, toast, formBox, empty } from '../lib/dom.js';
import { op } from '../lib/api.js';
import { isAdmin, state, go } from '../lib/nav.js';
import { ago, when } from '../lib/fmt.js';
import { listPage, recordPage } from './tablepage.js';
import { ban } from './actions.js';

export async function render(box, [id]) {
  if (id === undefined) {
    return listPage(box, {
      table: 'accountinfo', base: 'accounts', title: 'Accounts', allowNew: false,
      sub: 'Game accounts. A level of 1 or more is a GM; a status below 0 is a ban.',
      tools: [button('New account', { icon: 'plus', class: 'small primary', onclick: () => createAccount() })],
    });
  }
  return recordPage(box, {
    table: 'accountinfo', keySeg: id, base: 'accounts',
    crumbs: h('a', { href: '#/accounts' }, 'Accounts'),
    title: (row) => row.username,
    sub: (row) => `Account ${row.accountid} · created ${when(row.acctcreation) || 'unknown'} · last login ${ago(row.lastlogin) || 'never'}`,
    actions: (row, meta, refresh) => {
      const me = state.session.user;
      const self = row.accountid === me.accountid;
      const above = row.acctlevel >= me.level && !self;
      const banned = row.acctstatus < 0;
      return [
        !above && !self ? button(banned ? 'Lift ban' : 'Ban', { class: banned ? '' : 'danger', onclick: async () => { if (await ban(row, !banned)) location.reload(); } }) : null,
        !above ? button('Set password', { icon: 'lock', onclick: () => setPassword(row) }) : null,
        isAdmin() && !self && !above ? button('GM level', { onclick: () => setLevel(row) }) : null,
        !above && !self ? button('Kick sessions', { onclick: async () => {
          try { const out = await op('kick_player', { account: row.accountid, reason: '' }); toast(`${out.sessions} session(s) ejected`, 'ok'); } catch (e) { toast(e.message, 'error'); }
        } }) : null,
      ].filter(Boolean);
    },
    side: async (row) => {
      const out = await op('fetch', { id: 'account:' + row.accountid });
      const r = out.record;
      return h('div', { class: 'stack' },
        card('Status', h('div', { class: 'stack' },
          h('div', { class: 'row' },
            r.banned ? h('span', { class: 'badge danger' }, 'Banned') : h('span', { class: 'badge ok' }, 'Active'),
            r.isGM ? h('span', { class: 'badge accent' }, `GM level ${row.acctlevel}`) : h('span', { class: 'badge' }, 'Player'),
            r.authenticator ? h('span', { class: 'badge info' }, 'Authenticator') : null),
          h('dl', { class: 'kv' }, h('dt', null, 'Last address'), h('dd', null, row.ipaddress || '—'), h('dt', null, 'Email'), h('dd', null, row.email || '—')))),
        card(`Characters (${r.characters.length})`, r.characters.length
          ? h('div', { class: 'table-scroll' }, h('table', { class: 'plain' }, h('tbody', null, ...r.characters.map((c) => h('tr', null,
            h('td', null, h('a', { href: '#/characters/' + c.serverid }, c.charName), c.online ? h('span', { class: 'badge ok' }, 'online') : null),
            h('td', { class: 'muted' }, `${c.level} ${c.raceName} ${c.className}`))))))
          : empty('No characters.')));
    },
  });
}

async function createAccount() {
  const v = await formBox({
    title: 'New account', submitLabel: 'Make it',
    fields: [
      { name: 'username', label: 'Name (3-16 letters, digits, _)' },
      { name: 'password', label: 'Password (4+ characters)', type: 'password' },
      ...(isAdmin() ? [{ name: 'level', label: 'acctlevel (0 player, 1 GM, 2 admin)', type: 'number', value: 0 }] : []),
    ],
  });
  if (!v) return;
  try {
    const out = await op('create_account', { username: v.username, password: v.password, level: v.level || 0 });
    toast('Account ' + out.username + ' made', 'ok');
    go('accounts/' + out.accountid);
  } catch (e) { toast(e.message, 'error'); }
}

async function setPassword(row) {
  const v = await formBox({
    title: 'New password for ' + row.username, submitLabel: 'Set it',
    intro: 'It is hashed as the game hashes it. Their panel sessions end.',
    fields: [{ name: 'password', label: 'Password', type: 'password' }, { name: 'again', label: 'Again', type: 'password' }],
  });
  if (!v) return;
  if (v.password !== v.again) { toast('The two do not match', 'warn'); return; }
  try {
    await op('set_account_password', { account: row.accountid, password: v.password });
    toast('Password set', 'ok');
  } catch (e) { toast(e.message, 'error'); }
}

async function setLevel(row) {
  const lv = state.session.levels;
  const v = await formBox({
    title: 'GM level of ' + row.username, submitLabel: 'Set',
    intro: `0 is a player, ${lv.gm} a GM, ${lv.admin} an admin. You may give up to your own level (${state.session.user.level}).`,
    fields: [{ name: 'level', label: 'acctlevel', type: 'number', value: row.acctlevel }],
  });
  if (!v || v.level === null) return;
  try {
    await op('set_account_level', { account: row.accountid, level: v.level });
    toast('Level set', 'ok');
    location.reload();
  } catch (e) { toast(e.message, 'error'); }
}
