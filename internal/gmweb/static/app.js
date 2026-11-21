// The GM panel's page: the sign-in, the frame (navigation, search, who), and
// the router that hands #/<page>/<id> to a view.

import { h, mount, icon, toast, debounce, store } from './lib/dom.js';
import { api, op, setCSRF, onUnauthorized, setShard, currentShard } from './lib/api.js';
import { state, go, isTester, isStoryteller, isSubGM, isLimited } from './lib/nav.js';

const NAV = [
  ['Overview', [
    ['dashboard', 'Dashboard', 'home'],
    ['work', 'Work board', 'clipboard'],
    ['online', 'Players online', 'users'],
    ['economy', 'Economy', 'chart'],
    ['map', 'World map', 'map'],
  ]],
  ['Players', [
    ['accounts', 'Accounts', 'user'],
    ['characters', 'Characters', 'users'],
    ['petitions', 'Petitions', 'send'],
    ['transfers', 'Character transfers', 'users'],
    ['pwresets', 'Password resets', 'lock'],
    ['fallen', 'Hardcore heroes', 'skull'],
    ['emails', 'E-mail', 'send'],
  ]],
  ['World', [
    ['npcs', 'NPCs', 'skull'],
    ['merchants', 'Merchants', 'shop'],
    ['campfix', 'Camp reports', 'flag'],
    ['triggers', 'Triggers', 'bolt'],
  ]],
  ['Content', [
    ['items', 'Items', 'sword'],
    ['spells', 'Spells', 'spark'],
    ['quests', 'Quests', 'pin'],
    ['visuals', 'Spell visuals', 'play'],
    ['community', 'Community visuals', 'users'],
    ['mapmarks', 'Map suggestions', 'flag'],
    ['starting', 'New characters', 'book'],
  ]],
  ['Server', [
    ['tables', 'All tables', 'db'],
    ['audit', 'Activity log', 'history'],
    ['log', 'Server log', 'log'],
    ['sql', 'SQL', 'terminal', 'admin'],
    ['settings', 'Settings', 'cog'],
  ]],
  ['You', [
    ['ai', 'AI agents', 'bot'],
    ['security', 'Security', 'lock'],
    ['disc', 'Game disc', 'disc'],
  ]],
];

// The pages a tester has: the rest are all operations a tester is refused
// (tester.go), so they are not offered.
const TESTER_PAGES = new Set(['items', 'spells', 'visuals', 'tables', 'audit', 'ai', 'security', 'disc']);
// The pages a storyteller has (storyteller.go): the quests' words, its tables
// (NPC dialog among them), its log.
const STORYTELLER_PAGES = new Set(['quests', 'tables', 'audit', 'ai', 'security']);
// The pages a sub GM has beside a tester's (subgm.go): the World map, to move creatures.
const SUBGM_PAGES = new Set(['map']);

// mayOpen is whether a page is the signed-in account's: a GM's every one;
// below a GM those of its roles.
function mayOpen(id) {
  if (!isLimited()) return true;
  return (isTester() && TESTER_PAGES.has(id)) || (isStoryteller() && STORYTELLER_PAGES.has(id)) || (isSubGM() && SUBGM_PAGES.has(id));
}

// the label of the tables page, as a role below a GM sees it
function tablesLabel() {
  if (!isLimited()) return 'All tables';
  if (isTester() && isStoryteller()) return 'Your tables';
  return isTester() ? 'Item and spell tables' : 'Dialog and quest tables';
}

const VIEWS = {
  dashboard: () => import('./views/dashboard.js'),
  work: () => import('./views/work.js'),
  online: () => import('./views/online.js'),
  economy: () => import('./views/economy.js'),
  map: () => import('./views/map.js'),
  accounts: () => import('./views/accounts.js'),
  characters: () => import('./views/characters.js'),
  petitions: () => import('./views/petitions.js'),
  transfers: () => import('./views/transfers.js'),
  pwresets: () => import('./views/pwresets.js'),
  fallen: () => import('./views/fallen.js'),
  emails: () => import('./views/emails.js'),
  npcs: () => import('./views/npcs.js'),
  merchants: () => import('./views/merchants.js'),
  campfix: () => import('./views/campfix.js'),
  triggers: () => import('./views/triggers.js'),
  items: () => import('./views/items.js'),
  spells: () => import('./views/spells.js'),
  quests: () => import('./views/quests.js'),
  visuals: () => import('./views/visuals.js'),
  community: () => import('./views/community.js'),
  mapmarks: () => import('./views/mapmarks.js'),
  starting: () => import('./views/starting.js'),
  tables: () => import('./views/tables.js'),
  audit: () => import('./views/audit.js'),
  log: () => import('./views/log.js'),
  sql: () => import('./views/sql.js'),
  settings: () => import('./views/settings.js'),
  ai: () => import('./views/ai.js'),
  security: () => import('./views/security.js'),
  disc: () => import('./views/disc.js'),
};

const app = document.getElementById('app');

function applyTheme() {
  const t = store('theme');
  if (t === 'light' || t === 'dark') document.documentElement.dataset.theme = t;
  else delete document.documentElement.dataset.theme;
}
applyTheme();

// ---- sign-in

function showLogin(message) {
  state.session = null;
  setCSRF('');
  app.className = '';
  const user = h('input', { name: 'username', autocomplete: 'username', required: true, autofocus: true });
  const pass = h('input', { name: 'password', type: 'password', autocomplete: 'current-password', required: true });
  const code = h('input', { name: 'code', inputmode: 'numeric', autocomplete: 'one-time-code', maxlength: '7', pattern: '[0-9 ]*' });
  const codeRow = h('label', { class: 'hidden' }, 'Authenticator code', code);
  const msg = h('p', { class: 'err', role: 'alert' }, message || '');
  const submit = h('button', { type: 'submit', class: 'primary' }, 'Sign in');
  const form = h('form', { class: 'stack' },
    h('label', null, 'Account', user), h('label', null, 'Password', pass), codeRow, msg, submit);
  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    submit.disabled = true;
    msg.textContent = '';
    try {
      base = await api('api/auth/login', { username: user.value, password: pass.value, code: code.value });
      await enterShard(store('shard') || '');
    } catch (err) {
      if (err.data?.needCode) {
        codeRow.classList.remove('hidden');
        code.focus();
        msg.textContent = codeRow.dataset.asked ? err.message : '';
        codeRow.dataset.asked = '1';
      } else {
        msg.textContent = err.message;
      }
    } finally {
      submit.disabled = false;
    }
  });
  mount(app, h('div', { class: 'login' }, h('div', { class: 'login-card' },
    h('div', { class: 'brand' }, h('span', { class: 'brand-mark' }, 'GM'), h('div', null, 'EQOA GM panel', h('small', null, 'Sign in with your game account'))),
    h('h1', null, 'Game masters, testers and storytellers'),
    h('p', { class: 'muted' }, 'Accounts given the panel sign in with their game password. Everything you change is logged under your name.'),
    form)));
  setTimeout(() => user.focus(), 0);
}

onUnauthorized(() => showLogin('Your session ended. Sign in again.'));

// ---- the frame

let main, nav, shell;

// base is the session as this panel gives it; on another shard (the header's
// switch, forward.go) the page is drawn from that shard's view of the same GM.
let base = null;

// enterShard draws the page for a shard: '' (or this panel's own) is here;
// another's name sends every operation there, the page drawn as that shard
// sees the GM (its level, its admin, what it leaves out).
async function enterShard(id) {
  setCSRF(base.csrf);
  const target = (base.shards || []).find((x) => x.id === id && !x.here);
  if (!target) {
    setShard('');
    store('shard', '');
    start(base);
    return;
  }
  setShard(target.id);
  try {
    const st = await op('server_status');
    const you = st.you || {};
    store('shard', target.id);
    start({
      ...base,
      user: { ...base.user, level: you.level, role: you.role, admin: !!you.admin, adminNeedsTOTP: false },
      server: { ...base.server, shard: st.shard || target.label, build: st.build || base.server.build,
        noTransfers: !!st.noTransfers, accountsShared: !!st.accountsShared },
    });
  } catch (e) {
    setShard('');
    store('shard', '');
    toast(target.label + ': ' + e.message, 'error');
    start(base);
  }
}

// shardSwitch is the header's switch between the shards, when there are more
// than one.
function shardSwitch() {
  const shards = base?.shards;
  if (!shards || shards.length < 2) return null;
  const cur = currentShard();
  return h('div', { class: 'shard-switch', role: 'group', 'aria-label': 'Shard' },
    ...shards.map((x) => {
      const on = x.here ? !cur : cur === x.id;
      return h('button', {
        type: 'button', class: (on ? 'on' : '') + (x.here ? '' : ' away'), 'aria-pressed': String(on),
        title: x.here ? 'Work on ' + x.label + ' (this panel\'s own shard)' : 'Work on ' + x.label + ': every change goes to its server, as you',
        disabled: x.ready === false ? true : null,
        onclick: () => { if (!on) enterShard(x.here ? '' : x.id); },
      }, x.label);
    }));
}

function start(session) {
  state.session = session;
  setCSRF(session.csrf);
  app.className = '';
  nav = h('nav', { class: 'nav', 'aria-label': 'Sections' });
  for (const [group, items] of NAV) {
    const shown = items.filter(([id, , , need]) => (need !== 'admin' || session.user.admin) && mayOpen(id)
      && !(id === 'transfers' && session.server.noTransfers));
    if (!shown.length) continue;
    nav.appendChild(h('div', { class: 'nav-group' }, group));
    for (const [id, label, ic] of shown) {
      nav.appendChild(h('a', { href: '#/' + id, dataset: { page: id } }, icon(ic), id === 'tables' ? tablesLabel() : label));
    }
  }
  main = h('main', { id: 'main', tabindex: '-1' });
  const menuBtn = h('button', { class: 'ghost icon menu-btn', type: 'button', 'aria-label': 'Menu', onclick: () => shell.classList.toggle('nav-open') }, icon('menu'));
  const lvl = isLimited() ? h('span', { class: 'badge ok' }, [isSubGM() ? 'Sub GM' : isTester() ? 'Tester' : null, isStoryteller() ? 'Storyteller' : null].filter(Boolean).join(' · ') || 'No role')
    : session.user.admin ? h('span', { class: 'badge accent' }, 'Admin') : h('span', { class: 'badge info' }, 'GM');
  const themeBtn = h('button', { class: 'ghost icon', type: 'button', title: 'Light or dark', 'aria-label': 'Theme', onclick: () => {
    const cur = document.documentElement.dataset.theme || (matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark');
    store('theme', cur === 'light' ? 'dark' : 'light');
    applyTheme();
  } }, icon('sun'));
  const outBtn = h('button', { class: 'ghost icon', type: 'button', title: 'Sign out', 'aria-label': 'Sign out', onclick: async () => {
    try { await api('api/auth/logout', {}); } catch { /* signed out anyway */ }
    showLogin('Signed out.');
  } }, icon('out'));
  const away = currentShard();
  shell = h('div', { class: 'shell' + (away ? ' shard-away' : '') },
    h('aside', { class: 'side' },
      h('div', { class: 'brand' }, h('span', { class: 'brand-mark' }, 'GM'), h('div', null, 'EQOA GM', h('small', null, (session.server.shard ? session.server.shard + ' · ' : '') + 'build ' + session.server.build))),
      nav,
      h('div', { class: 'side-foot' }, 'Changes are logged and revertible.')),
    // search is a GM's and a tester's (the items and spells): a storyteller's
    // gate refuses it
    h('header', { class: 'top' }, menuBtn, !isLimited() || isTester() ? globalSearch() : null, h('span', { class: 'spacer' }),
      shardSwitch(),
      h('span', { class: 'who' }, lvl, h('strong', { class: 'name' }, session.user.username)), themeBtn, outBtn),
    main);
  mount(app, shell);
  if (session.server.shard) document.title = 'EQOA GM: ' + session.server.shard;
  if (away) document.title += ' (from ' + (base.shards?.find((x) => x.here)?.label || 'this panel') + ')';
  waitingBadge();
  if (session.totp?.required && !session.totp.enabled) {
    location.hash = '#/security';
    toast('This server asks every GM, tester and storyteller for an authenticator: enrol one to go on.', 'warn');
  }
  route();
}

// The petitions, the character transfers and the camp reports waiting on a
// GM, beside their pages in the navigation: asked at the start and every
// minute while signed in.
let badgeTimer = null;
async function waitingBadge() {
  clearTimeout(badgeTimer);
  if (!state.session) return;
  for (const [page, name, args] of [['petitions', 'petition_list', { status: 'waiting', limit: 1 }],
    ['transfers', 'transfer_list', { status: 'waiting', limit: 1 }],
    ['campfix', 'campfix_list', { counts: true }],
    ['pwresets', 'pwreset_list', { counts: true }]]) {
    const link = nav?.querySelector(`a[data-page="${page}"]`);
    if (!link) continue;
    try {
      const out = await op(name, args);
      const n = out.counts?.waiting || 0;
      link.querySelector('.badge')?.remove();
      if (n > 0) link.appendChild(h('span', { class: 'badge warn', title: `${n} waiting on a GM` }, String(n)));
    } catch { /* signed out, or a read the session may not make: no badge */ }
  }
  badgeTimer = setTimeout(waitingBadge, 60000);
}

window.addEventListener('hashchange', () => route());

let current = null;
async function route() {
  if (!state.session) return;
  const parts = location.hash.replace(/^#\/?/, '').split('/').map(decodeURIComponent);
  const home = !isLimited() ? 'dashboard' : isTester() ? 'items' : isStoryteller() ? 'quests' : 'security';
  const page = VIEWS[parts[0]] && mayOpen(parts[0]) ? parts[0] : home;
  for (const a of nav.querySelectorAll('a')) a.classList.toggle('on', a.dataset.page === page);
  shell.classList.remove('nav-open');
  if (current?.leave) current.leave();
  const box = h('div', { class: 'page' });
  mount(main, box);
  main.scrollTop = 0;
  try {
    const mod = await VIEWS[page]();
    current = (await mod.render(box, page === parts[0] ? parts.slice(1) : [], state)) || null;
  } catch (e) {
    console.error(e);
    mount(box, h('div', { class: 'notice danger' }, 'This page failed: ' + e.message));
  }
}

// ---- search across everything

function globalSearch() {
  const input = h('input', { type: 'search', placeholder: isTester() ? 'Search items and spells…' : 'Search accounts, characters, NPCs, items, spells…', 'aria-label': 'Search everything', autocomplete: 'off' });
  const pop = h('div', { class: 'search-pop hidden', role: 'listbox' });
  let results = [], sel = -1;
  const KIND = { account: 'accounts', character: 'characters', npc: 'npcs', item: 'items', spell: 'spells' };
  const draw = () => {
    if (!results.length) {
      mount(pop, h('div', { class: 'empty' }, 'Nothing found'));
      return;
    }
    const kids = [];
    let last = '';
    results.forEach((r, i) => {
      if (r.kind !== last) { kids.push(h('div', { class: 'kind' }, r.kind + 's')); last = r.kind; }
      const b = h('button', { type: 'button', class: i === sel ? 'sel' : '' }, h('div', null, r.title), h('div', { class: 'sub' }, r.subtitle || ''));
      b.addEventListener('mousedown', (e) => { e.preventDefault(); open(r); });
      kids.push(b);
    });
    mount(pop, ...kids);
  };
  const open = (r) => {
    pop.classList.add('hidden');
    input.value = '';
    go(KIND[r.kind] + '/' + r.key);
  };
  const run = debounce(async () => {
    const q = input.value.trim();
    if (q.length < 2) { pop.classList.add('hidden'); return; }
    try {
      const out = await op('search', { query: q, limit: 6 });
      results = out.results;
      sel = results.length ? 0 : -1;
      draw();
      pop.classList.remove('hidden');
    } catch (e) {
      mount(pop, h('div', { class: 'empty err' }, e.message));
      pop.classList.remove('hidden');
    }
  }, 220);
  input.addEventListener('input', run);
  input.addEventListener('keydown', (e) => {
    if (pop.classList.contains('hidden')) return;
    if (e.key === 'ArrowDown') { sel = Math.min(results.length - 1, sel + 1); draw(); e.preventDefault(); }
    else if (e.key === 'ArrowUp') { sel = Math.max(0, sel - 1); draw(); e.preventDefault(); }
    else if (e.key === 'Enter' && results[sel]) { open(results[sel]); e.preventDefault(); }
    else if (e.key === 'Escape') pop.classList.add('hidden');
  });
  input.addEventListener('blur', () => setTimeout(() => pop.classList.add('hidden'), 150));
  document.addEventListener('keydown', (e) => {
    if (e.key === '/' && !['INPUT', 'TEXTAREA', 'SELECT'].includes(document.activeElement?.tagName)) { e.preventDefault(); input.focus(); }
  });
  return h('div', { class: 'search' }, icon('search'), input, pop);
}

// ---- boot

(async () => {
  try {
    base = await api('api/auth/session');
  } catch {
    showLogin();
    return;
  }
  await enterShard(store('shard') || '');
})();
