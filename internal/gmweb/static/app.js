// The GM panel's page: the sign-in, the frame (navigation, search, who), and
// the router that hands #/<page>/<id> to a view.

import { h, mount, icon, toast, debounce, store } from './lib/dom.js';
import { api, op, setCSRF, onUnauthorized } from './lib/api.js';
import { state, go } from './lib/nav.js';

const NAV = [
  ['Overview', [
    ['dashboard', 'Dashboard', 'home'],
    ['online', 'Players online', 'users'],
    ['map', 'World map', 'map'],
  ]],
  ['Players', [
    ['accounts', 'Accounts', 'user'],
    ['characters', 'Characters', 'users'],
  ]],
  ['World', [
    ['npcs', 'NPCs', 'skull'],
    ['merchants', 'Merchants', 'shop'],
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
    ['audit', 'Audit log', 'history'],
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

const VIEWS = {
  dashboard: () => import('./views/dashboard.js'),
  online: () => import('./views/online.js'),
  map: () => import('./views/map.js'),
  accounts: () => import('./views/accounts.js'),
  characters: () => import('./views/characters.js'),
  npcs: () => import('./views/npcs.js'),
  merchants: () => import('./views/merchants.js'),
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
    h('label', null, 'GM account', user), h('label', null, 'Password', pass), codeRow, msg, submit);
  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    submit.disabled = true;
    msg.textContent = '';
    try {
      const s = await api('api/auth/login', { username: user.value, password: pass.value, code: code.value });
      start(s);
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
    h('h1', null, 'Game masters only'),
    h('p', { class: 'muted' }, 'Accounts with GM access sign in with their game password. Everything you change is logged under your name.'),
    form)));
  setTimeout(() => user.focus(), 0);
}

onUnauthorized(() => showLogin('Your session ended. Sign in again.'));

// ---- the frame

let main, nav, shell;

function start(session) {
  state.session = session;
  setCSRF(session.csrf);
  app.className = '';
  nav = h('nav', { class: 'nav', 'aria-label': 'Sections' });
  for (const [group, items] of NAV) {
    nav.appendChild(h('div', { class: 'nav-group' }, group));
    for (const [id, label, ic, need] of items) {
      if (need === 'admin' && !session.user.admin) continue;
      nav.appendChild(h('a', { href: '#/' + id, dataset: { page: id } }, icon(ic), label));
    }
  }
  main = h('main', { id: 'main', tabindex: '-1' });
  const menuBtn = h('button', { class: 'ghost icon menu-btn', type: 'button', 'aria-label': 'Menu', onclick: () => shell.classList.toggle('nav-open') }, icon('menu'));
  const lvl = session.user.admin ? h('span', { class: 'badge accent' }, 'Admin') : h('span', { class: 'badge info' }, 'GM');
  const themeBtn = h('button', { class: 'ghost icon', type: 'button', title: 'Light or dark', 'aria-label': 'Theme', onclick: () => {
    const cur = document.documentElement.dataset.theme || (matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark');
    store('theme', cur === 'light' ? 'dark' : 'light');
    applyTheme();
  } }, icon('sun'));
  const outBtn = h('button', { class: 'ghost icon', type: 'button', title: 'Sign out', 'aria-label': 'Sign out', onclick: async () => {
    try { await api('api/auth/logout', {}); } catch { /* signed out anyway */ }
    showLogin('Signed out.');
  } }, icon('out'));
  shell = h('div', { class: 'shell' },
    h('aside', { class: 'side' },
      h('div', { class: 'brand' }, h('span', { class: 'brand-mark' }, 'GM'), h('div', null, 'EQOA GM', h('small', null, 'build ' + session.server.build))),
      nav,
      h('div', { class: 'side-foot' }, 'Changes are logged and revertible.')),
    h('header', { class: 'top' }, menuBtn, globalSearch(), h('span', { class: 'spacer' }),
      h('span', { class: 'who' }, lvl, h('strong', { class: 'name' }, session.user.username)), themeBtn, outBtn),
    main);
  mount(app, shell);
  if (session.totp?.required && !session.totp.enabled) {
    location.hash = '#/security';
    toast('This server asks every GM for an authenticator: enrol one to go on.', 'warn');
  }
  route();
}

window.addEventListener('hashchange', () => route());

let current = null;
async function route() {
  if (!state.session) return;
  const parts = location.hash.replace(/^#\/?/, '').split('/').map(decodeURIComponent);
  const page = VIEWS[parts[0]] ? parts[0] : 'dashboard';
  for (const a of nav.querySelectorAll('a')) a.classList.toggle('on', a.dataset.page === page);
  shell.classList.remove('nav-open');
  if (current?.leave) current.leave();
  const box = h('div', { class: 'page' });
  mount(main, box);
  main.scrollTop = 0;
  try {
    const mod = await VIEWS[page]();
    current = (await mod.render(box, parts.slice(1), state)) || null;
  } catch (e) {
    console.error(e);
    mount(box, h('div', { class: 'notice danger' }, 'This page failed: ' + e.message));
  }
}

// ---- search across everything

function globalSearch() {
  const input = h('input', { type: 'search', placeholder: 'Search accounts, characters, NPCs, items, spells…', 'aria-label': 'Search everything', autocomplete: 'off' });
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
    start(await api('api/auth/session'));
  } catch {
    showLogin();
  }
})();
