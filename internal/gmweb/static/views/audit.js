// The activity log (audit_log): every change by the panel, an agent, a GM in
// the game or a data tool, filtered by what a GM means (an NPC's name, the
// items added, a camp report's changes, a tool's run) with a count beside each
// choice, newest first; each line what thing, what changed (only the changed
// fields, before -> after), who, when, why, and its revert.

import { h, mount, pageHead, button, toast, empty, confirmBox, debounce } from '../lib/dom.js';
import { op } from '../lib/api.js';
import { ago, when } from '../lib/fmt.js';
import { showChange } from '../lib/form.js';
import { isLimited, isTester, isStoryteller } from '../lib/nav.js';
import { tables } from '../lib/meta.js';

const KIND_BADGE = { added: 'ok', changed: 'info', deleted: 'danger' };
const ACTION_BADGE = { insert: 'ok', update: 'info', delete: 'danger', live: 'accent', settings: 'warn', sql: 'warn', account: 'warn', security: 'warn' };
const PAGE = 50;

function changedKeys(e) {
  const b = e.before || {}, a = e.after || {};
  return [...new Set([...Object.keys(b), ...Object.keys(a)])].filter((k) => JSON.stringify(b[k]) !== JSON.stringify(a[k]));
}

function rowLink(e) {
  if (!e.table || !e.key) return null;
  const vals = Object.values(e.key);
  // the purpose-built pages the reader has: below a GM its roles' own
  const pages = !isLimited() ? { npcs: 'npcs', itempattern: 'items', spellpattern: 'spells', characters: 'characters', accountinfo: 'accounts' }
    : { ...(isTester() ? { itempattern: 'items', spellpattern: 'spells' } : {}), ...(isStoryteller() ? { quests: 'quests' } : {}) };
  const href = pages[e.table] && vals.length === 1 ? `#/${pages[e.table]}/${vals[0]}`
    : `#/tables/${e.table}/${encodeURIComponent(vals.length === 1 ? String(vals[0]) : JSON.stringify(e.key))}`;
  return h('a', { href, class: 'mono small', title: 'open the row' }, `${e.table} ${vals.join(', ')}`);
}

function fmtV(v) {
  if (v === undefined) return '—';
  if (v === null) return 'null';
  const s = typeof v === 'object' ? JSON.stringify(v) : String(v);
  return s === '' ? '""' : s.length > 120 ? s.slice(0, 120) + '…' : s;
}

// the changed fields, before -> after
function diffBox(e) {
  const list = e.changes || [];
  if (!list.length) return null;
  const SHOW = 6;
  const box = h('div', { class: 'audit-diff' });
  const row = (c) => [
    h('span', { class: 'k', title: c.field }, c.label ? `${c.label}` : c.field),
    h('span', { class: 'b', title: typeof c.before === 'string' ? c.before : '' }, fmtV(c.before)),
    h('span', { class: 'arrow', 'aria-hidden': 'true' }, '→'),
    h('span', { class: 'a', title: typeof c.after === 'string' ? c.after : '' }, fmtV(c.after)),
  ];
  for (const c of list.slice(0, SHOW)) box.append(...row(c));
  if (list.length > SHOW) {
    const more = button(`${list.length - SHOW} more`, { class: 'small ghost', onclick: () => {
      more.remove();
      for (const c of list.slice(SHOW)) box.append(...row(c));
    } });
    box.append(h('span', { class: 'more' }, more));
  }
  return box;
}

// why: the petitions, camp reports and board entries its words name
function whyLinks(e) {
  const out = [];
  for (const p of e.petitions || []) out.push(h('a', { href: `#/tables/petitions/${p}`, class: 'badge accent', title: 'the petition' }, `petition ${p}`));
  for (const c of e.campReports || []) out.push(h('a', { href: `#/campfix/${c}`, class: 'badge accent', title: 'the camp report' }, `camp report ${c}`));
  for (const b of e.boardEntries || []) out.push(h('a', { href: `#/work/${b}`, class: 'badge', title: 'the work board entry' }, `board ${b}`));
  return out;
}

function sourceBadge(e) {
  if (e.source === 'agent') return h('span', { class: 'badge info', title: 'an AI agent signed in as this GM (' + e.via + ')' }, 'agent ' + (e.agent || ''));
  if (e.source === 'tool') return h('span', { class: 'badge warn', title: 'a data tool or a shell on the server (' + e.via + ')' }, e.tool && e.tool !== e.via ? e.tool : 'tool');
  if (e.source === 'game') return h('span', { class: 'badge accent', title: 'in the game' }, e.tool ? 'game ' + e.tool : 'in game');
  if (e.via && e.via !== 'web' && e.via !== 'staff') return h('span', { class: 'badge', title: 'through ' + e.via }, e.via);
  return null;
}

async function revertFlow(e, box, line) {
  try {
    const dry = await op('revert_change', { id: e.id, dry_run: true });
    const ok = await confirmBox({
      title: `Revert entry ${e.id}?`,
      message: `${dry.action} on ${dry.table} ${Object.values(dry.key || {}).join(', ')}: the row is put back as it was before ${e.username}'s change.`,
      confirmLabel: 'Revert',
    });
    if (!ok) return;
    const out = await op('revert_change', { id: e.id });
    toast('Reverted', 'ok', out.live || '');
    box.querySelector('.actions')?.remove();
    line.appendChild(h('span', { class: 'badge' }, 'reverted'));
  } catch (err) {
    if (err.status === 409 && await confirmBox({ title: 'The row changed since', message: err.message + ' Revert anyway, over the newer change?', confirmLabel: 'Revert anyway', danger: true })) {
      try { const out = await op('revert_change', { id: e.id, force: true }); toast('Reverted', 'ok', out.live || ''); } catch (e2) { toast(e2.message, 'error'); }
    } else if (err.status !== 409) toast(err.message, 'error');
  }
}

// auditLine is one entry: what thing, what changed, who, when, why; open for
// the whole row and a revert. opts.pick(filters) narrows the log to a value
// clicked (the activity log page's own).
export function auditLine(e, compact = false, opts = {}) {
  const kind = e.kind && e.kind !== 'other' ? e.kind : e.action;
  const badge = KIND_BADGE[e.kind] || ACTION_BADGE[e.action] || '';
  const keyVals = e.key ? Object.values(e.key) : [];
  const thingText = e.thing || (e.table ? `${e.table} ${keyVals.join(', ')}` : '');
  const pick = opts.pick;
  const thing = thingText
    ? (pick && e.table && keyVals.length
      ? h('button', { type: 'button', class: 'linkish thing', title: 'this row\'s history', onclick: () => pick({ table: e.table, row_id: keyVals.length === 1 ? String(keyVals[0]) : '', name: keyVals.length === 1 ? '' : e.thing || '' }, true) }, thingText)
      : h('strong', { class: 'thing' }, thingText))
    : null;
  const keys = changedKeys(e);
  const fallback = e.action === 'update' ? 'changed ' + keys.join(', ') : e.action === 'insert' ? 'added' : e.action === 'delete' ? 'deleted' : e.action;
  const what = e.detail || (thing ? '' : fallback);
  const who = pick && e.username !== 'staff'
    ? h('button', { type: 'button', class: 'linkish who', title: 'only theirs', onclick: () => pick({ who: e.username }) }, e.username)
    : h('span', { class: 'who' }, e.username);
  const line = h('div', { class: 'line' },
    h('span', { class: 'badge ' + badge }, kind),
    thing,
    compact ? null : rowLink(e),
    e.worldName ? h('span', { class: 'badge', title: 'world ' + e.world }, e.worldName) : null,
    e.isRevert ? h('span', { class: 'badge warn', title: 'this undoes another change' }, 'revert') : null,
    e.revertedBy ? h('span', { class: 'badge', title: `undone by entry ${e.revertedBy}` }, 'reverted') : null,
    compact && what ? h('span', { class: 'grow muted' }, what.length > 120 ? what.slice(0, 120) + '…' : what) : h('span', { class: 'grow' }),
    compact ? h('span', { class: 'faint small' }, e.username) : null,
    compact ? h('span', { class: 'faint small', title: when(e.at) }, ago(e.at)) : null);
  const box = h('div', { class: 'audit-entry' }, line);
  if (compact) return box;
  box.append(h('div', { class: 'audit-meta' },
    who, sourceBadge(e),
    h('span', { class: 'faint', title: when(e.at) }, `${ago(e.at)} · ${when(e.at)}`),
    h('span', { class: 'faint mono' }, '#' + e.id),
    ...whyLinks(e)));
  const diff = diffBox(e);
  if (diff) box.append(diff);
  if (what) box.append(h('div', { class: 'audit-why muted', title: what.length > 300 ? what : '' }, what.length > 300 ? what.slice(0, 300) + '…' : what));
  const canRevert = !!e.table && ['insert', 'update', 'delete'].includes(e.action) && !e.revertedBy && !e.noRevert;
  line.appendChild(h('span', { class: 'actions' },
    e.before || e.after ? button('Show', { class: 'small ghost', onclick: () => showChange({ ...e, live: '' }, `Entry ${e.id}: ${e.action} by ${e.username}`) }) : null,
    canRevert ? button('Revert', { class: 'small', icon: 'undo', onclick: () => revertFlow(e, box, line) }) : null));
  return box;
}

// ---- the page

// the quick filters: each sets plain filters (so the rest still narrow
// it, and a breakdown's choice changes one of them); the server counts each
// (audit_log's presets, the same filters)
const PRESETS = [
  ['npcs_deleted', 'NPCs deleted', { table: 'npcs', action: 'deleted' }],
  ['npcs_added', 'NPCs added', { table: 'npcs', action: 'added' }],
  ['npcs_changed', 'NPCs changed', { table: 'npcs', action: 'changed' }],
  ['items_added', 'Items added', { table: 'itempattern', action: 'added' }],
  ['items_changed', 'Items changed', { table: 'itempattern', action: 'changed' }],
  ['items_given', 'Items given', { table: 'charinventory', action: 'added' }],
  ['spells_changed', 'Spells changed', { group: 'spells', action: 'changed' }],
  ['loot_changed', 'Loot changed', { group: 'loot' }],
  ['camp_reports', 'Camp reports', { camp_report: '-1' }, 'gm'],
  ['petitions', 'For petitions', { petition: '-1' }, 'gm'],
  ['reverts', 'Reverts', { action: 'revert' }],
  ['reverted', 'Reverted', { action: 'reverted' }],
  ['today', 'Today', () => ({ since: todayText() })],
];
const presetFields = (p) => (typeof p[2] === 'function' ? p[2]() : p[2]);
const GROUPS = ['npcs', 'items', 'spells', 'loot', 'merchants', 'quests', 'characters', 'factions', 'triggers', 'camps', 'petitions', 'transfers', 'accounts', 'balance', 'world', 'work', 'panel', 'other', 'system'];
const KINDS = [['added', 'Added'], ['changed', 'Changed'], ['deleted', 'Deleted'], ['reverted', 'Reverted since'], ['revert', 'Is a revert'], ['other', 'Other (no row)']];
const RAW_ACTIONS = ['insert', 'update', 'delete', 'live', 'account', 'settings', 'sql', 'token', 'security', 'login', 'backup', 'snapshot', 'server'];
const SOURCES = [['panel', 'Panel (a person)'], ['agent', 'AI agent'], ['game', 'In the game'], ['tool', 'Data tool / shell']];
// the filters, as the page's address keeps them
const FIELDS = ['name', 'who', 'source', 'tool', 'table', 'group', 'action', 'row_id', 'world', 'since', 'until', 'petition', 'camp_report', 'search'];
const LABELS = { camp_report: 'Camp report', name: 'Name', who: 'Who', source: 'Source', tool: 'Tool', table: 'Table', group: 'Group', action: 'Action',
  row_id: 'Row id', world: 'World', since: 'From', until: 'To', petition: 'Petition', search: 'Text' };

function localMidnight(dateText) {
  const [y, m, d] = (dateText || '').split('-').map(Number);
  if (!y || !m || !d) return '';
  return new Date(y, m - 1, d).toISOString();
}
function todayText() {
  const t = new Date();
  return `${t.getFullYear()}-${String(t.getMonth() + 1).padStart(2, '0')}-${String(t.getDate()).padStart(2, '0')}`;
}

export async function render(box, parts = []) {
  const limited = isLimited();
  const f = Object.fromEntries(FIELDS.map((k) => [k, '']));
  let offset = 0;
  try {
    const q = new URLSearchParams(parts[0] || '');
    for (const k of FIELDS) f[k] = q.get(k) || '';
    offset = Math.max(0, Number(q.get('offset')) || 0);
  } catch { /* an odd address: no filters */ }

  // the inputs
  const input = (k, attrs) => {
    const el = h('input', { ...attrs, value: f[k], 'aria-label': LABELS[k] });
    el.addEventListener('input', () => { f[k] = el.value.trim(); reload(); });
    return el;
  };
  const select = (k, options, anyLabel) => {
    const el = h('select', { 'aria-label': LABELS[k] }, h('option', { value: '' }, anyLabel), ...options.map(([v, l]) => h('option', { value: v, selected: f[k] === v }, l)));
    el.addEventListener('change', () => { f[k] = el.value; reload(); });
    return el;
  };
  const whoList = h('datalist', { id: 'audit-who' });
  const toolList = h('datalist', { id: 'audit-tools' });
  const tableList = h('datalist', { id: 'audit-tables' });
  tables().then((ts) => mount(tableList, ...ts.map((t) => h('option', { value: t.name }, t.label)))).catch(() => {});
  const els = {
    name: input('name', { type: 'search', placeholder: 'name: an NPC, item, spell, character…', class: 'grow-input' }),
    who: input('who', { type: 'search', placeholder: 'who: GM, agent, tool, game', list: 'audit-who' }),
    action: select('action', [...KINDS, ...RAW_ACTIONS.map((a) => [a, 'raw: ' + a])], 'any change'),
    group: select('group', GROUPS.map((g) => [g, g]), 'any kind of thing'),
    source: select('source', SOURCES, 'any source'),
    tool: input('tool', { type: 'search', placeholder: 'tool, script, /command, agent', list: 'audit-tools' }),
    table: input('table', { type: 'search', placeholder: 'table (or a,b)', list: 'audit-tables' }),
    row_id: input('row_id', { type: 'number', placeholder: 'row id', inputmode: 'numeric', class: 'num' }),
    world: select('world', [], 'any world'),
    since: input('since', { type: 'date', title: 'from this day' }),
    until: input('until', { type: 'date', title: 'to this day (whole)' }),
    petition: input('petition', { type: 'number', placeholder: 'petition # (-1 any)', inputmode: 'numeric', class: 'num' }),
    camp_report: input('camp_report', { type: 'number', placeholder: 'camp report # (-1 any)', inputmode: 'numeric', class: 'num' }),
    search: input('search', { type: 'search', placeholder: 'text in the words or rows' }),
  };
  const forGM = (el) => (limited ? null : el);
  const field = (label, el) => (el ? h('label', { class: 'audit-field' }, h('span', null, label), el) : null);

  const presetBar = h('div', { class: 'audit-presets' });
  const chips = h('div', { class: 'audit-chips' });
  const breakdown = h('div', { class: 'audit-breakdown' });
  const breakdownBox = h('details', { class: 'audit-counts', open: window.innerWidth > 700 },
    h('summary', null, 'Counts: by change, kind, source, table, who and tool'), breakdown);
  const note = h('div', { class: 'audit-note' });
  const list = h('div', { class: 'card audit-list' });
  const count = h('span', { class: 'muted small' });
  const older = button('Older', { class: 'small', onclick: () => { offset += PAGE; load(); } });
  const newer = button('Newer', { class: 'small', onclick: () => { offset = Math.max(0, offset - PAGE); load(); } });
  const pager = () => h('div', { class: 'grid-foot' }, count, h('span', { class: 'grow' }), newer, older);
  const more = h('details', { class: 'audit-more', open: ['source', 'tool', 'row_id', 'world', 'since', 'until', 'petition', 'camp_report', 'search'].some((k) => f[k]) },
    h('summary', null, 'More filters'),
    h('div', { class: 'audit-grid' },
      forGM(field('Source', els.source)), forGM(field('Tool / agent', els.tool)), field('Table', els.table), field('Row id', els.row_id),
      field('World', els.world), field('From', els.since), field('To', els.until),
      forGM(field('Petition', els.petition)), forGM(field('Camp report', els.camp_report)), forGM(field('Text', els.search))));

  const whose = isTester() && isStoryteller() ? 'your tables' : isTester() ? 'the item and spell tables' : 'NPC dialog, the quests and the NPCs';
  mount(box,
    limited
      ? pageHead('Activity log', `Every change to ${whose}: your own whole, and of anyone else's what changed and when.`)
      : pageHead('Activity log', 'Every change through the panel, an AI agent, a GM in the game or a data tool: what thing, what changed, who, when and why. A row change can be reverted.'),
    whoList, toolList, tableList,
    h('div', { class: 'card audit-filters' },
      presetBar,
      h('div', { class: 'grid-tools' }, els.name, forGM(els.who), els.action, els.group),
      more, chips, breakdownBox),
    note, list);

  const reload = debounce(() => { offset = 0; load(); }, 250);
  let seq = 0;

  function setFilters(patch, replace = false) {
    if (replace) for (const k of FIELDS) f[k] = '';
    Object.assign(f, patch);
    for (const k of FIELDS) {
      const el = els[k];
      if (el) el.value = f[k];
    }
    if (['source', 'tool', 'row_id', 'world', 'since', 'until', 'petition', 'camp_report', 'search'].some((k) => f[k])) more.open = true;
    offset = 0;
    load();
    window.scrollTo?.({ top: 0 });
  }

  function args() {
    const a = { limit: PAGE, offset, counts: true, today: localMidnight(todayText()) };
    for (const k of ['name', 'who', 'source', 'tool', 'table', 'group', 'action', 'search']) if (f[k]) a[k] = f[k];
    if (f.row_id !== '' && Number.isFinite(Number(f.row_id))) a.row_id = Number(f.row_id);
    if (f.world !== '') a.world = Number(f.world);
    if (f.petition !== '' && Number.isFinite(Number(f.petition))) a.petition = Number(f.petition);
    if (f.camp_report !== '' && Number.isFinite(Number(f.camp_report))) a.camp_report = Number(f.camp_report);
    if (f.since) a.since = localMidnight(f.since);
    if (f.until) {
      const end = localMidnight(f.until);
      if (end) a.until = new Date(Date.parse(end) + 86400000 - 1000).toISOString().replace(/\.\d+Z$/, 'Z');
    }
    if (a.since) a.since = a.since.replace(/\.\d+Z$/, 'Z');
    if (a.today) a.today = a.today.replace(/\.\d+Z$/, 'Z');
    return a;
  }

  function keepAddress() {
    const q = new URLSearchParams();
    for (const k of FIELDS) if (f[k]) q.set(k, f[k]);
    if (offset) q.set('offset', String(offset));
    const s = q.toString();
    try { history.replaceState(null, '', '#/audit' + (s ? '/' + encodeURIComponent(s) : '')); } catch { /* not ours to keep */ }
  }

  function drawPresets(counts) {
    const n = counts?.presets || {};
    mount(presetBar, ...PRESETS.filter((p) => !(p[3] && limited)).map((p) => {
      const [id, label] = p;
      const want = presetFields(p);
      const on = Object.entries(want).every(([k, v]) => f[k] === v);
      return h('button', { type: 'button', class: 'chip-btn' + (on ? ' on' : ''), 'aria-pressed': on ? 'true' : 'false',
        title: on ? 'take it off' : 'only these (the other filters cleared)',
        onclick: () => (on ? setFilters(Object.fromEntries(Object.keys(want).map((k) => [k, '']))) : setFilters(want, true)) },
      label, n[id] !== undefined ? h('span', { class: 'n' }, n[id].toLocaleString()) : null);
    }));
  }

  function drawChips() {
    const active = FIELDS.filter((k) => f[k]);
    mount(chips, ...active.map((k) => {
      let v = f[k];
      if ((k === 'petition' || k === 'camp_report') && v === '-1') v = 'any';
      if (k === 'world') v = els.world.selectedOptions[0]?.textContent?.replace(/ \(\d[\d,]*\)$/, '') || v;
      return h('span', { class: 'chip' }, `${LABELS[k]}: ${v}`, h('button', { type: 'button', 'aria-label': `remove ${LABELS[k]}`, onclick: () => setFilters({ [k]: '' }) }, '×'));
    }), active.length > 1 ? button('Clear all', { class: 'small ghost', onclick: () => setFilters({}, true) }) : null);
  }

  function drawBreakdown(c) {
    if (!c) { mount(breakdown); return; }
    const pickRow = (title, all, key, fmt = (r) => r.value) => { const rows = all.filter((r) => r.value !== ''); return rows.length ? h('div', { class: 'audit-facet' }, h('span', { class: 'faint small' }, title),
      ...rows.map((r) => h('button', { type: 'button', class: 'chip-btn small' + (String(f[key]) === String(r.value) ? ' on' : ''),
        onclick: () => setFilters({ [key]: String(f[key]) === String(r.value) ? '' : String(r.value) }) }, fmt(r), h('span', { class: 'n' }, r.n.toLocaleString())))) : null; };
    const kindRows = ['added', 'changed', 'deleted', 'other'].filter((k) => c.kind?.[k]).map((k) => ({ value: k, n: c.kind[k] }));
    const groupRows = Object.entries(c.group || {}).sort((a, b) => b[1] - a[1]).slice(0, 10).map(([value, n]) => ({ value, n }));
    const srcRows = Object.entries(c.source || {}).sort((a, b) => b[1] - a[1]).map(([value, n]) => ({ value, n }));
    mount(breakdown,
      pickRow('Change', kindRows, 'action'),
      pickRow('Kind', groupRows, 'group'),
      limited ? null : pickRow('Source', srcRows, 'source'),
      pickRow('Table', (c.table || []).slice(0, 8), 'table'),
      limited ? null : pickRow('Who', (c.who || []).slice(0, 8), 'who'),
      limited ? null : pickRow('Tool', (c.tool || []).slice(0, 8), 'tool'));
    // the lists the inputs offer, from what the log holds
    mount(whoList, ...(c.who || []).map((r) => h('option', { value: r.value })));
    mount(toolList, ...(c.tool || []).map((r) => h('option', { value: r.value })));
    const keep = f.world;
    mount(els.world, h('option', { value: '' }, 'any world'),
      ...(c.world || []).map((r) => h('option', { value: r.value, selected: keep === r.value }, `${r.label || 'world ' + r.value} (${r.n.toLocaleString()})`)));
    if (keep && ![...els.world.options].some((o) => o.value === keep)) els.world.append(h('option', { value: keep, selected: true }, 'world ' + keep));
  }

  async function load() {
    const my = ++seq;
    keepAddress();
    drawChips();
    try {
      const out = await op('audit_log', args());
      if (my !== seq) return;
      count.textContent = out.total ? `${(offset + 1).toLocaleString()}–${Math.min(offset + PAGE, out.total).toLocaleString()} of ${out.total.toLocaleString()}` : 'No entries';
      older.disabled = offset + PAGE >= out.total;
      newer.disabled = offset === 0;
      mount(note, out.indexing ? h('div', { class: 'notice warn' }, `Indexing the log for its filters: ${out.indexing.indexed.toLocaleString()} of ${out.indexing.lines.toLocaleString()} lines so far (once, after the server's start). The filters see those lines alone until it is done.`) : null);
      drawPresets(out.counts);
      drawBreakdown(out.counts);
      drawChips();
      const pick = (patch, replace) => setFilters(patch, replace);
      mount(list, out.entries.length ? h('div', null, ...out.entries.map((e) => auditLine(e, false, { pick }))) : empty('No entries match.'), pager());
    } catch (e) {
      if (my === seq) toast(e.message, 'error');
    }
  }
  await load();
}
