// The quests (handlers/quests.go): every quest, an editor that saves a quest
// and its steps together, a character's quests, and drafts made out of
// EQOA-Data's guides. Everything goes through the quest_* operations, each row
// change logged and revertible on the Audit log page.

import { h, mount, card, button, toast, pageHead, empty, confirmBox, debounce, dialog } from '../lib/dom.js';
import { op } from '../lib/api.js';
import { go, state } from '../lib/nav.js';
import { describe } from '../lib/meta.js';

// Ids as the game numbers them (root CLAUDE.md, verified on the wire).
const CLASSES = ['Warrior', 'Ranger', 'Paladin', 'Shadowknight', 'Monk', 'Bard', 'Rogue', 'Druid', 'Shaman', 'Cleric',
  'Magician', 'Necromancer', 'Enchanter', 'Wizard', 'Alchemist'];
const RACES = ['Human', 'Elf', 'Dark Elf', 'Gnome', 'Dwarf', 'Troll', 'Barbarian', 'Halfling', 'Erudite', 'Ogre'];
const KINDS = {
  talk: 'Talk to an NPC',
  show: 'Show an NPC items (kept)',
  give: 'Hand an NPC items (taken)',
  kill: 'Kill creatures',
  loot: 'Loot items off creatures',
  reach: 'Reach a place',
};
const AT_NPC = new Set(['talk', 'show', 'give']);

// A quest's events and a step's (handlers/questevents.go): when, and what.
const QUEST_WHENS = { accept: 'When taken', finish: 'When finished' };
const STEP_WHENS = { start: 'When it starts', done: 'When it is done' };
const DOS = {
  popup: 'A window the player closes',
  say: 'An NPC says',
  give: 'Give items',
  spawn: 'Spawn an NPC for the step',
  visual: 'Play a spell visual on the player',
  teleport: 'Teleport the player',
};

export async function render(box, parts) {
  const [seg, arg] = parts;
  if (seg === 'characters') return characters(box, arg);
  if (seg === 'import') return importer(box);
  if (seg === undefined || seg === '') return list(box, {});
  if (seg === 'f') return list(box, Object.fromEntries(new URLSearchParams(arg || '')));
  return editor(box, seg === 'new' ? 0 : Number(seg));
}

// A quest's status, as quest_list gives it (questindex.go).
const STATUS = {
  'in the game': ['ok', 'in the game'],
  ready: ['info', 'ready'],
  draft: ['warn', 'draft'],
};
const SHORT = ['WAR', 'RNG', 'PAL', 'SK', 'MNK', 'BRD', 'RGE', 'DRU', 'SHA', 'CLR', 'MAG', 'NEC', 'ENC', 'WIZ', 'ALC'];
const CITY_NAMES = { 'klick-anon': "Klick'Anon", 'neriak-nektulos': 'Neriak',
  // the later guides' categories (tools/quests/data/<category>.json, questindex.go's questCity)
  17: 'Level 17', '23-47': 'Levels 23-47', '30-40-49': 'Levels 30, 40, 49', '53-59': 'Levels 53-59', 60: 'Level 60 epics',
  'hunt-and-gather': 'Hunt and gather', 'tank-exemplar': 'Tank exemplar', weres: 'Weres', misc: 'Misc', portal: 'Portals' };

function cityName(c) {
  if (!c) return '—';
  return CITY_NAMES[c] || c.split('-').map((w) => w[0].toUpperCase() + w.slice(1)).join(' ');
}

function statusBadge(st) {
  const [kind, label] = STATUS[st] || ['', st];
  return h('span', { class: 'badge ' + kind }, label);
}

function tabs(on) {
  const t = (id, label, path) => h('button', { type: 'button', class: id === on ? 'on' : '', onclick: () => go(path) }, label);
  return h('div', { class: 'tabs' }, t('list', 'Quests', 'quests'), t('chars', "A character's quests", 'quests/characters'),
    t('import', 'Drafts from EQOA-Data', 'quests/import'));
}

// ---- the list

// list is every quest a filter takes (the address keeps the filter:
// #/quests/f/status=draft&city=freeport): its status and why, the counts,
// what the drafts lack, and the quests a GM puts in the game or takes out in
// one go (quest_set_enabled, each quest an audited row).
async function list(box, params) {
  const f = { ...params };
  const keep = () => {
    const q = new URLSearchParams(Object.entries(f).filter(([, v]) => v !== '' && v !== undefined)).toString();
    history.replaceState(null, '', q ? '#/quests/f/' + encodeURIComponent(q) : '#/quests');
  };
  const input = (key, attrs) => {
    const i = h('input', { value: f[key] ?? '', ...attrs });
    i.addEventListener('input', debounce(() => { f[key] = i.value.trim(); keep(); load(); }, 300));
    return i;
  };
  const select = (key, label, options) => {
    const sel = h('select', { 'aria-label': label }, ...options.map(([v, l]) => h('option', { value: v, selected: String(f[key] ?? '') === String(v) }, l)));
    sel.addEventListener('change', () => { f[key] = sel.value; keep(); load(); });
    return sel;
  };
  const search = input('search', { type: 'search', placeholder: 'Search by name, giver, note or key', 'aria-label': 'Search quests' });
  const status = select('status', 'Status', [['', 'every status'], ['in the game', 'in the game'], ['draft', 'every draft'],
    ['ready', 'drafts ready to go in'], ['blocked', 'drafts not ready']]);
  const citySel = select('city', 'City or category', [['', 'every city or category']]);
  const klass = select('class', 'Class', [['', 'every class'], ...CLASSES.map((c, i) => [String(i), c])]);
  const level = input('level', { type: 'number', min: 1, max: 60, placeholder: 'for level', 'aria-label': 'For a level', class: 'narrow' });
  const giver = input('giver', { type: 'search', placeholder: 'giver', 'aria-label': 'Giver' });
  const chips = h('div', { class: 'row' });
  const summary = h('div', { class: 'row' });
  const actions = h('div', { class: 'row' });
  const lacks = h('div');
  const body = h('div');
  mount(box,
    pageHead('Quests', "What NPCs give. A quest's giver offers it when talked to; its steps are done by talking to an NPC, " +
      'showing or handing one items, killing creatures, looting their drops and reaching places. A draft is offered to ' +
      'no one; a ready one is a draft nothing keeps out now (every name it uses is on this server, every quest it ' +
      'requires in the game or ready): put it in the game here.', null,
    button('New quest', { class: 'primary', icon: 'plus', onclick: () => go('quests/new') })),
    tabs('list'),
    card(null, h('div', { class: 'stack' },
      h('div', { class: 'row quest-tools' }, h('div', { class: 'grow' }, search), status, citySel, klass, level, giver),
      chips, summary, actions, lacks, body)));
  let last = null;
  const load = async () => {
    const args = { limit: 2000 };
    for (const k of ['search', 'status', 'city', 'giver', 'missing']) if (f[k]) args[k] = f[k];
    if (f.class !== undefined && f.class !== '') args.class = Number(f.class);
    if (f.level) args.level = Number(f.level);
    let out;
    try {
      out = await op('quest_list', args);
    } catch (e) {
      mount(body, h('p', { class: 'err' }, e.message));
      return;
    }
    last = out;
    // the cities, with their counts, kept in the choice
    const have = f.city ?? '';
    mount(citySel, h('option', { value: '' }, 'every city or category'),
      ...out.cities.map((c) => h('option', { value: c.city || 'none', selected: have === (c.city || 'none') },
        `${c.city ? cityName(c.city) : 'no city (a GM\'s)'} · ${c.quests}${c.drafts ? `, ${c.drafts} drafts` : ''}${c.ready ? `, ${c.ready} ready` : ''}`)));
    mount(chips, f.missing ? h('span', { class: 'chip' }, 'lacking ' + f.missing,
      h('button', { type: 'button', 'aria-label': 'Clear', onclick: () => { delete f.missing; keep(); load(); } }, '×')) : null);
    const c = out.counts;
    mount(summary, h('strong', null, `${out.total} quest${out.total === 1 ? '' : 's'}`),
      h('span', { class: 'muted' }, `${c['in the game']} in the game · ${c.ready} ready · ${c.draft} drafts not ready`),
      out.quests.length < out.total ? h('span', { class: 'muted small' }, `(the first ${out.quests.length} shown)`) : null);
    const filterArgs = () => { const a = { ...args }; delete a.limit; return a; };
    mount(actions,
      c.ready ? button(`Put the ${c.ready} ready in the game`, { class: 'primary', onclick: () => setEnabled({ ...filterArgs(), status: 'ready' }, true) }) : null,
      c['in the game'] ? button(`Take the ${c['in the game']} in the game out`, { onclick: () => setEnabled({ ...filterArgs(), status: 'in the game' }, false) }) : null);
    mount(lacks, out.missing.length && !f.missing
      ? h('div', { class: 'stack' }, h('span', { class: 'muted small' }, 'What the drafts shown lack, the most wanted first (placing that NPC, or making that item, frees every quest that lacked only it; click one for its quests):'),
        h('div', { class: 'row quest-lacks' }, ...out.missing.slice(0, 24).map((m) =>
          h('a', { href: '#', class: 'badge warn', title: `${m.kind}: ${m.quests} quest(s) lack it`,
            onclick: (e) => { e.preventDefault(); f.missing = m.name; f.status = ''; status.value = ''; keep(); load(); } }, `${m.name} · ${m.quests}`))))
      : null);
    if (!out.quests.length) {
      mount(body, empty(Object.keys(args).length > 1 ? 'No quest matches.' : 'No quests yet: make one, or import the city quests (tools/import_quests.py).'));
      return;
    }
    const rows = out.quests.map((q) => h('tr', { onclick: () => go('quests/' + q.questid) },
      h('td', { class: 'mono' }, String(q.questid)), h('td', null, q.name || '(no name)'), h('td', null, statusBadge(q.status)),
      h('td', null, cityName(q.city)),
      h('td', { class: 'small' }, q.classes.length ? q.classes.map((i) => SHORT[i] ?? i).join(' ') : 'any'),
      h('td', { class: 'num' }, `${q.minlevel}–${q.maxlevel || 'any'}`), h('td', null, q.giver || '—'),
      h('td', { class: 'num' }, String(q.steps)), h('td', { class: 'num' }, String(q.underway)), h('td', { class: 'num' }, String(q.finished)),
      h('td', { class: 'small muted', title: (q.why || []).join('\n') }, (q.why || []).length ? q.why[0] + (q.why.length > 1 ? ` (+${q.why.length - 1})` : '') : '')));
    mount(body, h('div', { class: 'grid-scroll' }, h('table', { class: 'grid' },
      h('thead', null, h('tr', null, ...['#', 'Name', 'Status', 'City or category', 'Classes', 'Levels', 'Giver', 'Steps', 'Under way', 'Finished', 'Why it is a draft'].map((x) => h('th', null, x)))),
      h('tbody', null, ...rows))));
  };
  // turning many on or off: a dry run, what it does, then for real
  const setEnabled = async (filter, on) => {
    let dry;
    try {
      dry = await op('quest_set_enabled', { filter, enabled: on, dryRun: true });
    } catch (e) { toast(e.message, 'error'); return; }
    if (!dry.changed.length) { toast('Nothing to change', 'ok'); return; }
    const req = dry.changed.filter((x) => x.required);
    const msg = h('div', { class: 'stack' },
      h('p', null, on
        ? `${dry.changed.length} quest(s) go in the game: their givers offer them from now on.`
        : `${dry.changed.length} quest(s) become drafts: offered to no one, a character who has one under way goes on with it.`),
      req.length ? h('p', { class: 'muted small' }, `With them, ${req.length} quest(s) they require: ${req.slice(0, 6).map((x) => x.name).join(', ')}${req.length > 6 ? '…' : ''}`) : null,
      dry.skipped.length ? h('p', { class: 'muted small' }, `${dry.skipped.length} left out (not ready).`) : null,
      h('p', { class: 'muted small' }, 'Each quest is its own row change, logged and revertible.'));
    const ok = await confirmBox({ title: on ? 'Put these quests in the game?' : 'Take these quests out of the game?', message: msg,
      confirmLabel: on ? 'Put them in' : 'Take them out' });
    if (!ok) return;
    try {
      const out = await op('quest_set_enabled', { filter, enabled: on });
      toast(`${out.changed.length} quest(s) ${on ? 'in the game' : 'out of the game'}`, 'ok', out.live);
      load();
    } catch (e) { toast(e.message, 'error'); }
  };
  await load();
}

// ---- inputs bound to an object's keys (the Triggers page's too)

export function field(label, input, hint, wide) {
  return h('div', { class: 'field' + (wide ? ' wide' : '') },
    h('label', null, label, hint ? h('span', { class: 'hint' }, ' · ' + hint) : null), input);
}

export function text(obj, key, attrs = {}) {
  const i = h('input', { type: 'text', value: obj[key] ?? '', ...attrs });
  i.addEventListener('input', () => { obj[key] = i.value; });
  return i;
}

export function num(obj, key, attrs = {}) {
  const i = h('input', { type: 'number', value: obj[key] ?? 0, ...attrs });
  i.addEventListener('input', () => { obj[key] = i.value === '' ? 0 : Number(i.value); });
  return i;
}

export function area(obj, key, rows = 3) {
  const t = h('textarea', { rows });
  t.value = obj[key] ?? '';
  t.addEventListener('input', () => { obj[key] = t.value; });
  return t;
}

export function check(obj, key, label) {
  const c = h('input', { type: 'checkbox', checked: !!Number(obj[key]) });
  c.addEventListener('change', () => { obj[key] = c.checked ? 1 : 0; });
  return h('label', { class: 'check' }, c, label);
}

// counted is a text input that says how many of max characters it holds.
export function counted(obj, key, max) {
  const i = text(obj, key);
  const n = h('span', { class: 'hint' });
  const show = () => {
    const len = [...(i.value || '')].length;
    n.textContent = `${len}/${max}`;
    n.style.color = len > max ? 'var(--danger, #e66)' : '';
  };
  i.addEventListener('input', show);
  show();
  const box = h('div', { class: 'stack' }, i, n);
  box.style.gap = '2px'; // CSSOM: the panel's CSP refuses style attributes
  return box;
}

// idSet is checkboxes for a comma list of ids.
function idSet(obj, key, names) {
  const have = new Set(String(obj[key] || '').split(/[^0-9]+/).filter(Boolean).map(Number));
  return h('div', { class: 'flagset' }, ...names.map((name, i) => {
    const c = h('input', { type: 'checkbox', checked: have.has(i) });
    c.addEventListener('change', () => {
      if (c.checked) have.add(i); else have.delete(i);
      obj[key] = [...have].sort((a, b) => a - b).join(', ');
    });
    return h('label', null, c, name);
  }));
}

// pick asks for an NPC, an item or a spell by name (the search operation), or
// for anything find(query) lists ({title, subtitle, key}).
function pick(kind, title, find) {
  return new Promise((resolve) => {
    let done = false;
    let dlg;
    const search = h('input', { type: 'search', placeholder: 'part of a name', autocomplete: 'off' });
    const out = h('div', { class: 'stack' });
    const run = debounce(async () => {
      const q = search.value.trim();
      if (!q) { mount(out, h('p', { class: 'muted small' }, 'Type part of a name.')); return; }
      try {
        const r = find ? { results: await find(q) } : await op('search', { query: q, kinds: [kind], limit: 25 });
        mount(out, ...(r.results || []).map((x) => {
          const b = h('button', { type: 'button', class: 'ghost' }, h('strong', null, x.title), ' ', h('span', { class: 'muted small' }, x.subtitle || ''));
          b.addEventListener('click', () => { done = true; resolve(x); dlg.close(); });
          return b;
        }), (r.results || []).length ? null : h('p', { class: 'muted small' }, 'Nothing by that name.'));
      } catch (e) {
        mount(out, h('p', { class: 'err' }, e.message));
      }
    }, 200);
    search.addEventListener('input', run);
    dlg = dialog({ title, body: h('div', { class: 'stack' }, search, out), onClose: () => { if (!done) resolve(null); } });
    run();
    setTimeout(() => search.focus(), 0);
  });
}

// npcInput is an NPC's name with a picker and a link to it on the map.
export function npcInput(obj, key, what) {
  const i = text(obj, key, { placeholder: 'its name as the game shows it' });
  const pickB = button('Pick', { onclick: async () => {
    const r = await pick('npc', 'Pick ' + what);
    if (r) { i.value = r.title; obj[key] = r.title; }
  } });
  const mapB = button('Map', { title: 'Show it on the world map', onclick: async () => {
    const name = (obj[key] || '').trim();
    if (!name) return;
    try {
      const r = await op('query_rows', { table: 'npcs', filters: [{ column: 'npc_name', op: '=', value: name }], limit: 1, columns: ['id', 'world'] });
      if (!r.rows.length) { toast(`No NPC is named "${name}"`, 'error'); return; }
      go(`map/${r.rows[0].world}/${r.rows[0].id}`);
    } catch (e) { toast(e.message, 'error'); }
  } });
  return h('div', { class: 'with' }, i, pickB, mapB);
}

// itemsInput is "patternid*count, ..." with a picker that adds one.
export function itemsInput(obj, key) {
  const i = text(obj, key, { placeholder: 'patternid*count, ...', class: 'mono' });
  const add = button('Add', { title: 'Pick an item to add', onclick: async () => {
    const r = await pick('item', 'Add an item');
    if (!r) return;
    const cur = (obj[key] || '').trim();
    obj[key] = (cur ? cur + ', ' : '') + r.key;
    i.value = obj[key];
    toast(`${r.title} added (one; write *2 after its id for two)`, 'ok');
  } });
  return h('div', { class: 'with' }, i, add);
}

// spellsInput is spell ids, comma separated, with a picker that adds one.
function spellsInput(obj, key) {
  const i = text(obj, key, { placeholder: 'spell ids, comma separated', class: 'mono' });
  const add = button('Add', { title: 'Pick a spell to add', onclick: async () => {
    const r = await pick('spell', 'Add a spell');
    if (!r) return;
    const cur = (obj[key] || '').trim();
    obj[key] = (cur ? cur + ', ' : '') + r.key;
    i.value = obj[key];
    toast(`${r.title} added`, 'ok');
  } });
  return h('div', { class: 'with' }, i, add);
}

// findFactions lists this server's factions whose names hold q (the factions
// table, through query_rows), for pick.
async function findFactions(q) {
  const r = await op('query_rows', { table: 'factions', filters: [{ column: 'name', op: 'like', value: q }], limit: 25, columns: ['name', 'standing'] });
  return (r.rows || []).map((x) => ({ title: x.name, subtitle: `starts at ${x.standing}`, key: x.name }));
}

// factionReqs edits obj[key], the standings a quest asks for ("Faction:min"
// or "Faction:min..max", comma separated: database.ParseFactionReqs), a row a
// faction: its name as the factions table has it, its lowest standing and,
// when there is one, its highest.
function factionReqs(obj, key) {
  const box = h('div', { class: 'stack' });
  const list = String(obj[key] || '').split(',').map((x) => x.trim()).filter(Boolean).map((x) => {
    const i = x.lastIndexOf(':');
    if (i < 0) return { faction: x, min: 0, max: '' };
    const [lo, hi] = x.slice(i + 1).split('..');
    return { faction: x.slice(0, i).trim(), min: Number(lo) || 0, max: hi === undefined ? '' : String(Number(hi)) };
  });
  const keep = () => {
    obj[key] = list.filter((r) => r.faction.trim())
      .map((r) => `${r.faction.trim()}:${Number(r.min) || 0}${String(r.max).trim() === '' ? '' : '..' + Number(r.max)}`).join(', ');
  };
  const row = (r, i) => {
    const name = text(r, 'faction', { placeholder: 'its name as the factions table has it' });
    name.addEventListener('input', keep);
    const pickB = button('Pick', { onclick: async () => {
      const x = await pick('faction', 'Pick a faction', findFactions);
      if (x) { r.faction = x.key; name.value = x.key; keep(); }
    } });
    const lo = num(r, 'min', { min: -2000, max: 2000 });
    lo.addEventListener('input', keep);
    const hi = h('input', { type: 'number', min: -2000, max: 2000, value: r.max, placeholder: 'none' });
    hi.addEventListener('input', () => { r.max = hi.value; keep(); });
    return h('div', { class: 'fields' }, field('Faction', h('div', { class: 'with' }, name, pickB), null, true),
      field('At least', lo), field('At most', hi, 'empty: no highest'),
      field('', button('Remove', { icon: 'trash', onclick: () => { list.splice(i, 1); keep(); draw(); } })));
  };
  const draw = () => mount(box, ...list.map(row),
    list.length ? null : h('p', { class: 'muted small' }, 'None: whoever its levels, classes and races allow.'),
    button('Ask a standing', { icon: 'plus', onclick: () => { list.push({ faction: '', min: 0, max: '' }); draw(); } }));
  draw();
  return box;
}

// classRewards edits obj[key], a step's rewards by class (database.
// ClassReward: a JSON object by class id, each {xp, tunar, items, spells},
// items and spells written as the step's own are), a row a class. Text that
// is not such an object is shown as it is, to be mended by hand.
function classRewards(obj, key) {
  const box = h('div', { class: 'stack' });
  let rows;
  try {
    const m = obj[key] ? JSON.parse(obj[key]) : {};
    if (!m || typeof m !== 'object' || Array.isArray(m)) throw new Error('not an object of class ids');
    rows = Object.entries(m).map(([c, v]) => ({ class: c, xp: v.xp || 0, tunar: v.tunar || 0, items: v.items || '', spells: v.spells || '' }));
  } catch (e) {
    mount(box, h('p', { class: 'err small' }, 'These rewards by class are not a JSON object (' + e.message + '): mend them here.'), area(obj, key, 3));
    return box;
  }
  const keep = () => {
    const out = {};
    for (const r of rows) {
      const v = {};
      if (Number(r.xp)) v.xp = Number(r.xp);
      if (Number(r.tunar)) v.tunar = Number(r.tunar);
      if (String(r.items).trim()) v.items = String(r.items).trim();
      if (String(r.spells).trim()) v.spells = String(r.spells).trim();
      out[String(r.class)] = v;
    }
    obj[key] = rows.length ? JSON.stringify(out) : '';
  };
  box.addEventListener('input', keep);
  box.addEventListener('change', keep);
  const row = (r, i) => {
    const cls = h('select', null, ...CLASSES.map((c, k) => h('option', { value: String(k), selected: String(r.class) === String(k) }, c)));
    cls.addEventListener('change', () => { r.class = cls.value; keep(); });
    return h('div', { class: 'fields' }, field('Class', cls), field('Experience', num(r, 'xp', { min: 0 })), field('Tunar', num(r, 'tunar', { min: 0 })),
      field('Items', itemsInput(r, 'items'), null, true), field('Spells taught', spellsInput(r, 'spells'), 'its own class\'s', true),
      field('', button('Remove', { icon: 'trash', onclick: () => { rows.splice(i, 1); keep(); draw(); } })));
  };
  const draw = () => mount(box, ...rows.map(row),
    rows.length ? null : h('p', { class: 'muted small' }, 'None: every class gets the rewards above alone.'),
    button('Add a class', { icon: 'plus', onclick: () => {
      const free = CLASSES.findIndex((_, k) => !rows.some((r) => String(r.class) === String(k)));
      rows.push({ class: String(Math.max(free, 0)), xp: 0, tunar: 0, items: '', spells: '' });
      keep();
      draw();
    } }));
  draw();
  return box;
}

// choiceEditor edits obj[key], a talk, show or give step's choice
// (database.StepChoices: {ask, options: [{answer, says, stay, xp, tunar,
// items, spells}]}): its question and a row an option. Text that is not such
// a choice is shown as it is, to be mended by hand.
function choiceEditor(obj, key) {
  const box = h('div', { class: 'stack' });
  let ch;
  try {
    ch = obj[key] ? JSON.parse(obj[key]) : { options: [] };
    if (!ch || typeof ch !== 'object' || !Array.isArray(ch.options || [])) throw new Error('no options list');
    ch.options = ch.options || [];
  } catch (e) {
    mount(box, h('p', { class: 'err small' }, 'This choice is not {ask, options} (' + e.message + '): mend it here.'), area(obj, key, 3));
    return box;
  }
  const keep = () => {
    const opts = ch.options.map((o) => {
      const v = { answer: String(o.answer || '').trim() };
      if (String(o.says || '').trim()) v.says = o.says;
      if (o.stay) v.stay = true;
      if (Number(o.xp)) v.xp = Number(o.xp);
      if (Number(o.tunar)) v.tunar = Number(o.tunar);
      if (String(o.items || '').trim()) v.items = String(o.items).trim();
      if (String(o.spells || '').trim()) v.spells = String(o.spells).trim();
      return v;
    });
    obj[key] = opts.length ? JSON.stringify(String(ch.ask || '').trim() ? { ask: ch.ask.trim(), options: opts } : { options: opts }) : '';
  };
  box.addEventListener('input', keep);
  box.addEventListener('change', keep);
  const row = (o, i) => {
    const stay = h('input', { type: 'checkbox', checked: !!o.stay });
    stay.addEventListener('change', () => { o.stay = stay.checked; keep(); });
    return h('div', { class: 'fields' }, field(`Answer ${i + 1}`, counted(o, 'answer', 63), null, true),
      field('NPC says when picked', area(o, 'says', 2), 'a line a box', true),
      field('', h('label', { class: 'check' }, stay, 'Stays: the step is not done (a wrong answer)')),
      field('Experience', num(o, 'xp', { min: 0 })), field('Tunar', num(o, 'tunar', { min: 0 })),
      field('Items', itemsInput(o, 'items'), null, true), field('Spells taught', spellsInput(o, 'spells'), null, true),
      field('', button('Remove', { icon: 'trash', onclick: () => { ch.options.splice(i, 1); keep(); draw(); } })));
  };
  const ask = text(ch, 'ask', { placeholder: 'Which do you choose?' });
  const draw = () => mount(box, ch.options.length ? field('The question', ask, 'asked when the step is done', true) : null,
    ...ch.options.map(row),
    ch.options.length ? null : h('p', { class: 'muted small' }, 'None: the step is done when talked to.'),
    ch.options.length < 6 ? button('Add an option', { icon: 'plus', onclick: () => { ch.options.push({ answer: '' }); keep(); draw(); } }) : null);
  draw();
  return box;
}

// routeKey is a place as an unlock's is matched to a route's label
// (handlers/questtrips.go): case, a leading "the" and all but letters and
// digits aside.
function routeKey(s) {
  return String(s || '').trim().toLowerCase().replace(/^the /, '').replace(/[^\p{L}\p{N}]/gu, '');
}

// unlocksEditor edits list, the trips a quest unlocks ({npc, dest}), a row a
// trip: the travel NPC and its route's place, offered from routes (quest_get's:
// every waysender's and portal's trips); a place no route of that NPC is
// opens nothing until one is there.
function unlocksEditor(list, routes) {
  const box = h('div', { class: 'stack' });
  const row = (u, i) => {
    const dl = h('datalist', { id: `unlock-routes-${i}` });
    const note = h('span', { class: 'hint' });
    const dest = text(u, 'dest', { placeholder: "the route's place", list: dl.id });
    const refresh = () => {
      const mine = routes.filter((r) => r.npc.trim().toLowerCase() === String(u.npc || '').trim().toLowerCase());
      mount(dl, ...mine.map((r) => h('option', { value: r.to })));
      const found = mine.some((r) => routeKey(r.to) === routeKey(u.dest));
      note.textContent = !u.npc ? '' : !mine.length ? 'no route of that NPC' : found ? '' : `no such trip: ${mine.map((r) => r.to).join(', ')}`;
      note.style.color = note.textContent ? 'var(--danger, #e66)' : '';
    };
    const npc = npcInput(u, 'npc', 'the travel NPC');
    npc.addEventListener('input', refresh);
    dest.addEventListener('input', refresh);
    dest.addEventListener('focus', refresh); // after a pick, which sets the NPC without an input event
    refresh();
    return h('div', { class: 'fields' }, field('Travel NPC', npc, 'a waysender or a portal', true),
      field('Trip to', h('div', { class: 'stack' }, dest, dl, note), null, true),
      field('', button('Remove', { icon: 'trash', onclick: () => { list.splice(i, 1); draw(); } })));
  };
  const draw = () => mount(box, ...list.map(row),
    list.length ? null : h('p', { class: 'muted small' }, 'None.'),
    button('Unlock a trip', { icon: 'plus', onclick: () => { list.push({ npc: '', dest: '' }); draw(); } }));
  draw();
  return box;
}

// eventsEditor edits obj[key], a JSON list of events (handlers/questevents.go),
// a row an event: when, what, and what that needs. Text that is not such a
// list is shown as it is, to be mended by hand.
function eventsEditor(obj, key, whens, worlds) {
  const box = h('div', { class: 'stack' });
  let list;
  try {
    list = obj[key] ? JSON.parse(obj[key]) : [];
    if (!Array.isArray(list)) throw new Error('not a list');
  } catch (e) {
    mount(box, h('p', { class: 'err small' }, "These events are not a JSON list (" + e.message + "): mend them here."), area(obj, key, 4));
    return box;
  }
  const keep = () => { obj[key] = list.length ? JSON.stringify(list) : ''; };
  box.addEventListener('input', keep);
  box.addEventListener('change', keep);
  const flag = (e, k, label) => {
    const c = h('input', { type: 'checkbox', checked: !!e[k] });
    c.addEventListener('change', () => { if (c.checked) e[k] = true; else delete e[k]; keep(); });
    return h('label', { class: 'check' }, c, label);
  };
  const row = (e, i) => {
    const when = h('select', null, ...Object.entries(whens).map(([k, label]) => h('option', { value: k, selected: e.on === k }, label)));
    when.addEventListener('change', () => { e.on = when.value; });
    const what = h('select', null, ...Object.entries(DOS).map(([k, label]) => h('option', { value: k, selected: e.do === k }, label)));
    what.addEventListener('change', () => { e.do = what.value; keep(); draw(); });
    const fields = [field('When', when), field('What', what)];
    if (e.do === 'popup') {
      fields.push(field('Text', area(e, 'text', 2), '%p: the player\'s name; its last 127 characters ask', true),
        field('Its answer', text(e, 'answer', { placeholder: 'OK' })));
    } else if (e.do === 'say') {
      fields.push(field('Who', npcInput(e, 'npc', 'who says it'), 'empty: no name before the words', true),
        field('Words', area(e, 'text', 2), 'a line a box', true));
    } else if (e.do === 'give') {
      fields.push(field('Items', itemsInput(e, 'items'), null, true));
    } else if (e.do === 'spawn') {
      fields.push(field('Whom', npcInput(e, 'npc', 'the NPC to copy'), 'a copy before the player, gone when the step is done', true),
        field('', flag(e, 'hostile', 'Hostile: it may be attacked, and attacks the player')));
    } else if (e.do === 'visual') {
      fields.push(field('Visual', text(e, 'fx', { placeholder: '0x1DAEB5B6', class: 'mono' }), "a 0x00B1 DictID (the Spell visuals page lists them)"),
        field('Casting part', num(e, 'seconds', { min: 0, max: 30, step: 0.1 }), 'seconds'));
    } else if (e.do === 'teleport') {
      const w = h('select', null, ...(worlds.length ? worlds : [{ value: 0, label: 'World 0' }]).map((x) => h('option', { value: x.value, selected: Number(e.world || 0) === x.value }, x.label)));
      w.addEventListener('change', () => { e.world = Number(w.value); });
      fields.push(field('World', w), field('x', num(e, 'x', { step: 'any' })), field('y', num(e, 'y', { step: 'any' })), field('z', num(e, 'z', { step: 'any' })));
    }
    const move = (d) => { const j = i + d; if (j < 0 || j >= list.length) return; [list[i], list[j]] = [list[j], list[i]]; keep(); draw(); };
    return h('div', { class: 'stack' }, h('div', { class: 'fields' }, ...fields),
      h('div', { class: 'row' }, button('↑', { title: 'Earlier', onclick: () => move(-1) }), button('↓', { title: 'Later', onclick: () => move(1) }),
        button('Remove event', { icon: 'trash', onclick: () => { list.splice(i, 1); keep(); draw(); } })));
  };
  const draw = () => mount(box, ...list.map(row), list.length ? null : h('p', { class: 'muted small' }, 'No events.'),
    button('Add an event', { icon: 'plus', onclick: () => { list.push({ on: Object.keys(whens)[0], do: 'popup', text: '' }); keep(); draw(); } }));
  draw();
  return box;
}

// ---- the editor

async function editor(box, id) {
  let data = {
    quest: { name: '', enabled: 0, giver: '', minlevel: 1, maxlevel: 60, classes: '', races: '', requires: '', repeatable: 0,
      offer: '', accept: '', decline: '', declined: '', started: '', source: 'gm', notes: '', events: '', factionreq: '' },
    steps: [], unlocks: [], routes: [], problems: [], characters: [],
  };
  if (id) {
    try {
      data = await op('quest_get', { quest: id });
    } catch (e) {
      mount(box, pageHead('Quest ' + id, null, h('a', { href: '#/quests' }, 'Quests')), h('p', { class: 'err' }, e.message));
      return;
    }
  }
  const q = { ...data.quest };
  const steps = data.steps.map((s) => ({ ...s }));
  const unlocks = (data.unlocks || []).map((u) => ({ ...u }));
  let worlds = [];
  try { worlds = (await describe('queststeps')).enums?.world || []; } catch { /* no catalog: numbers */ }

  const problems = h('div');
  const showProblems = (ps) => mount(problems, ps && ps.length
    ? h('ul', { class: 'steps' }, ...ps.map((p) => h('li', { class: 'err' }, p)))
    : h('p', { class: 'muted' }, id ? 'Nothing found wrong.' : 'Saved quests are checked here.'));
  showProblems(data.problems);

  const stepsBox = h('div', { class: 'stack' });
  const drawSteps = () => mount(stepsBox, ...steps.map((s, i) => stepCard(s, i)),
    steps.length ? null : empty('No steps yet: a quest needs one to be taken.'),
    button('Add a step', { icon: 'plus', onclick: () => {
      steps.push({ kind: 'talk', target: q.giver || '', npcid: 0, items: '', tunar: 0, count: 1, world: 0, x: 0, y: 0, z: 0, radius: 15,
        journal: '', answer: '', says: '', waiting: '', notyet: '', reminder: '', xp: 0, rewardtunar: 0, rewarditems: '', factions: '', spells: '',
        chance: 0, events: '', byclass: '', choices: '' });
      drawSteps();
    } }));

  const stepCard = (s, i) => {
    const kind = h('select', null, ...Object.entries(KINDS).map(([k, label]) => h('option', { value: k, selected: s.kind === k }, label)));
    kind.addEventListener('change', () => { s.kind = kind.value; drawSteps(); });
    const fields = [field('Kind', kind)];
    if (AT_NPC.has(s.kind)) {
      fields.push(field('NPC', npcInput(s, 'target', "the step's NPC"), 'a merchant, banker or blacksmith never talks', true));
    } else if (s.kind === 'kill') {
      fields.push(field('Creature', npcInput(s, 'target', 'the creature'), 'every one of the name counts', true),
        field('How many', num(s, 'count', { min: 1 })));
    } else if (s.kind === 'loot') {
      fields.push(field('Creature', npcInput(s, 'target', 'the creature'), 'its kills drop the item', true),
        field('The item that drops', itemsInput(s, 'items'), "onto the player's own view of the corpse", true),
        field('How many to take', num(s, 'count', { min: 1 }), 'taken off corpses during the step'),
        field('Drop chance', num(s, 'chance', { min: 0, max: 1, step: 0.05 }), '0 to 1 a kill; 0: every kill'));
    }
    if (s.kind !== 'reach') fields.push(field('Only spawn', num(s, 'npcid', { min: 0 }), '0: any of the name'));
    if (s.kind === 'show' || s.kind === 'give') {
      fields.push(field(s.kind === 'give' ? 'Items handed in' : 'Items to hold', itemsInput(s, 'items'), s.kind === 'give' ? 'taken' : 'kept', true),
        field(s.kind === 'give' ? 'Tunar handed in' : 'Tunar to hold', num(s, 'tunar', { min: 0 })));
    }
    if (s.kind === 'reach') {
      const w = h('select', null, ...(worlds.length ? worlds : [{ value: 0, label: 'World 0' }]).map((x) => h('option', { value: x.value, selected: Number(s.world) === x.value }, x.label)));
      w.addEventListener('change', () => { s.world = Number(w.value); });
      const here = button('Where my character stands', { onclick: async () => {
        try {
          const out = await op('online_players', {});
          const me = out.players.find((p) => p.accountid === state.session?.user?.accountid);
          if (!me) { toast('You have no character in the world', 'error'); return; }
          Object.assign(s, { world: me.world, x: me.x, y: me.y, z: me.z });
          drawSteps();
          toast(`${me.name} stands at ${me.x}, ${me.z} in ${me.worldName}`, 'ok');
        } catch (e) { toast(e.message, 'error'); }
      } });
      fields.push(field('World', w), field('x', num(s, 'x', { step: 'any' })), field('y', num(s, 'y', { step: 'any' })),
        field('z', num(s, 'z', { step: 'any' })), field('Within', num(s, 'radius', { min: 1, step: 'any' })), field('', here));
    }
    fields.push(field('Journal line', counted(s, 'journal', 127), 'shown while this is the step to do', true));
    if (AT_NPC.has(s.kind)) {
      fields.push(field("The player's answer", area(s, 'answer', 2), 'the first line does it, any more change nothing ("Farewell"); empty: done on talking', true),
        field('NPC says before', area(s, 'waiting', 2), 'a line a box; with an answer, the last line asks', true));
      if (s.kind !== 'talk') {
        fields.push(field('Answer while the items lack', area(s, 'notyet', 2), 'offered instead of the answer until they are held', true),
          field('NPC says while they lack', area(s, 'reminder', 2), null, true));
      }
    }
    fields.push(field(AT_NPC.has(s.kind) ? 'NPC says when done' : 'Said when done', area(s, 'says', 2), 'a line a box', true),
      field('Reward: experience', num(s, 'xp', { min: 0 })), field('Reward: tunar', num(s, 'rewardtunar', { min: 0 })),
      field('Reward: items', itemsInput(s, 'rewarditems'), null, true),
      field('Reward: faction changes', text(s, 'factions', { placeholder: 'Faction Name:+10, ...' }), null, true),
      field('Reward: spells taught', spellsInput(s, 'spells'), null, true),
      field('Rewards by class', classRewards(s, 'byclass'), 'besides those above, for a player of that class', true),
      AT_NPC.has(s.kind) ? field('A choice', choiceEditor(s, 'choices'), "asked when it is done: the pick's rewards besides those above", true) : null,
      field('Events', eventsEditor(s, 'events', STEP_WHENS, worlds), 'run in order after the step\'s own words', true));
    const move = (d) => { const j = i + d; if (j < 0 || j >= steps.length) return; [steps[i], steps[j]] = [steps[j], steps[i]]; drawSteps(); };
    return card(`Step ${i + 1} · ${KINDS[s.kind] || s.kind}`, h('div', { class: 'fields' }, ...fields),
      button('↑', { title: 'Earlier', onclick: () => move(-1) }), button('↓', { title: 'Later', onclick: () => move(1) }),
      button('Remove', { icon: 'trash', onclick: () => { steps.splice(i, 1); drawSteps(); } }));
  };
  drawSteps();

  const save = button('Save', { class: 'primary', icon: 'save', onclick: async () => {
    try {
      const out = await op('quest_save', { quest: { ...q, questid: id || undefined }, steps, unlocks });
      const newID = Number(out.quest.questid);
      toast(out.changes ? `Saved (${out.changes} row change${out.changes === 1 ? '' : 's'})` : 'Nothing to save', 'ok', out.live);
      showProblems(out.problems);
      if (!id) go('quests/' + newID);
    } catch (e) { toast(e.message, 'error'); }
  } });
  const tryIt = id ? button('Try it', { title: 'Give it to your own character in the world', onclick: async () => {
    try {
      const out = await op('quest_try', { quest: id });
      toast(out.message, 'ok');
    } catch (e) { toast(e.message, 'error'); }
  } }) : null;
  const del = id ? button('Delete', { icon: 'trash', class: 'danger', onclick: async () => {
    const dry = await op('quest_delete', { quest: id, dryRun: true });
    const ok = await confirmBox({ title: 'Delete this quest?', danger: true, confirmLabel: 'Delete',
      message: `Its ${dry.steps} step(s) go too, and ${dry.characters} character(s) lose their progress in it. Every row is logged and can be reverted.` });
    if (!ok) return;
    try {
      await op('quest_delete', { quest: id });
      toast('Deleted', 'ok');
      go('quests');
    } catch (e) { toast(e.message, 'error'); }
  } }) : null;

  // where it stands among the rest (quest_get's status, why and chain) and
  // putting it in the game or taking it out (quest_set_enabled)
  const links = (list) => list?.length
    ? h('ul', { class: 'steps' }, ...list.map((x) => h('li', null, h('a', { href: '#/quests/' + x.questid }, x.name || 'quest ' + x.questid), ' ', statusBadge(x.status))))
    : h('p', { class: 'muted small' }, 'none');
  const flip = async (on) => {
    try {
      const dry = await op('quest_set_enabled', { quests: [id], enabled: on, dryRun: true });
      if (!dry.changed.length) {
        toast(dry.skipped.length ? 'Not ready: ' + dry.skipped[0].why : 'Nothing to change', dry.skipped.length ? 'error' : 'ok');
        return;
      }
      const more = dry.changed.filter((x) => x.required);
      if (more.length && !(await confirmBox({ title: 'Put it in the game with the quests it requires?',
        message: `It requires ${more.map((x) => x.name).join(', ')}, a draft as well: they go in with it.`, confirmLabel: 'Put them in' }))) return;
      const out = await op('quest_set_enabled', { quests: [id], enabled: on });
      toast(on ? 'In the game' : 'A draft again', 'ok', out.live);
      editor(box, id);
    } catch (e) { toast(e.message, 'error'); }
  };
  const stands = id ? h('div', { class: 'stack' },
    h('div', { class: 'row' }, statusBadge(data.status), data.city ? h('span', { class: 'muted small' }, cityName(data.city) + (data.key ? ' · ' + data.key : '')) : null),
    data.why?.length ? h('ul', { class: 'steps' }, ...data.why.map((w) => h('li', { class: 'err' }, w))) : null,
    h('div', null, h('strong', { class: 'small' }, 'Before it'), links(data.before)),
    h('div', null, h('strong', { class: 'small' }, 'After it'), links(data.after)),
    data.status === 'ready' ? button('Put it in the game', { class: 'primary', onclick: () => flip(true) }) : null,
    data.status === 'in the game' ? button('Take it out of the game', { onclick: () => flip(false) }) : null) : null;

  const who = data.characters?.length
    ? h('ul', { class: 'steps' }, ...data.characters.map((c) => h('li', null,
      h('a', { href: '#/quests/characters/' + c.serverid }, c.charName || 'character ' + c.serverid), ' · ',
      Number(c.step) > 0 ? `at step ${c.step}` : `finished ${c.done} time(s)`)))
    : h('p', { class: 'muted' }, 'No one has it.');

  mount(box,
    pageHead(id ? (q.name || 'Quest ' + id) : 'New quest', id ? `Quest ${id}${q.source ? ' · ' + q.source : ''}` : 'Saved as a draft until you tick In the game.',
      h('a', { href: '#/quests' }, 'Quests'), tryIt, del, save),
    tabs('list'),
    h('div', { class: 'cols-2' },
      h('div', { class: 'stack' },
        card('The quest', h('div', { class: 'fields' },
          field('Name', text(q, 'name'), 'for GMs; players see journal lines', true),
          field('', check(q, 'enabled', 'In the game (offered by its giver)')),
          field('', check(q, 'repeatable', 'May be taken again once finished')),
          field('Giver', npcInput(q, 'giver', 'its giver'), 'offers it when talked to', true),
          field('From level', num(q, 'minlevel', { min: 0 })), field('To level', num(q, 'maxlevel', { min: 0 }), '0: any'),
          field('Classes', idSet(q, 'classes', CLASSES), 'none ticked: any', true),
          field('Races', idSet(q, 'races', RACES), 'none ticked: any', true),
          field('Quests finished first', text(q, 'requires', { placeholder: 'quest ids, comma separated', class: 'mono' }), null, true),
          field('Standings asked', factionReqs(q, 'factionreq'), 'offered, marked and done at its giver only while each is met (-2000 to 2000)', true))),
        card('What the giver says', h('div', { class: 'fields' },
          field('Offer', area(q, 'offer', 6), 'a paragraph a question: its lines a box each, its last line the question (no name before it)', true),
          field('Answers taking it', area(q, 'accept', 2), 'a line a question; empty: taken as soon as the giver is talked to', true),
          field('Answer refusing it', counted(q, 'decline', 63), 'offered with each answer; empty: none', true),
          field('Reply to a refusal', area(q, 'declined', 2), 'said as written, no name put before it', true),
          field('Once taken', area(q, 'started', 3), 'a line a box', true),
          h('p', { class: 'muted small field wide' }, "An NPC's lines are shown with its name before them, as Sandstorm's are, unless they begin with it."))),
        card('Events', h('div', { class: 'fields' },
          field('', eventsEditor(q, 'events', QUEST_WHENS, worlds), "run in order after the giver's words when it is taken, and after the last step's when it is finished", true))),
        card('Trips it unlocks', h('div', { class: 'stack' },
          h('p', { class: 'muted small' }, "A travel NPC offers each trip only to a player who has finished this quest (or another that unlocks it), while one of them is in the game."),
          unlocksEditor(unlocks, data.routes || []))),
        card('Steps', stepsBox)),
      h('div', { class: 'stack' },
        stands ? card('Where it stands', stands) : null,
        card('Problems', problems),
        card('Who has it', who),
        card('Notes', h('div', { class: 'fields' }, field('', area(q, 'notes', 10), null, true))))));
}

// ---- a character's quests

async function characters(box, who) {
  const input = h('input', { type: 'search', placeholder: 'a character, by name or serverid', value: who ? decodeURIComponent(who) : '' });
  const body = h('div', { class: 'stack' });
  const show = () => { const v = input.value.trim(); if (v) go('quests/characters/' + encodeURIComponent(v)); };
  input.addEventListener('keydown', (e) => { if (e.key === 'Enter') show(); });
  mount(box,
    pageHead("A character's quests", 'Under way in journal order, then finished. A change here keeps the journal of a character in the world in step, and is logged and revertible. No reward is given.'),
    tabs('chars'),
    card(null, h('div', { class: 'stack' }, h('div', { class: 'row' }, h('div', { class: 'grow' }, input), button('Show', { class: 'primary', onclick: show })), body)));
  if (!who) { mount(body, empty('Name a character.')); return; }
  const load = async () => {
    let out;
    try {
      out = await op('character_quests', { character: decodeURIComponent(who) });
    } catch (e) { mount(body, h('p', { class: 'err' }, e.message)); return; }
    const set = async (quest, action, step) => {
      try {
        const r = await op('character_quest_set', { character: out.serverid, quest, action, step });
        toast(r.message, 'ok');
        load();
      } catch (e) { toast(e.message, 'error'); }
    };
    const rows = out.quests.map((x) => {
      const act = h('select', null,
        h('option', { value: '' }, 'Change…'),
        ...Array.from({ length: x.steps }, (_, k) => h('option', { value: 'step:' + (k + 1) }, `Set to step ${k + 1}`)),
        h('option', { value: 'complete' }, 'Finish it (no reward)'),
        x.step > 0 ? h('option', { value: 'giveup' }, 'Give it up') : null,
        h('option', { value: 'reset' }, 'Forget it (may be taken again)'));
      act.addEventListener('change', () => {
        const v = act.value;
        if (!v) return;
        if (v.startsWith('step:')) set(x.questid, 'step', Number(v.slice(5)));
        else set(x.questid, v, 0);
      });
      return h('tr', null,
        h('td', null, h('a', { href: '#/quests/' + x.questid }, x.name)),
        h('td', null, x.step > 0 ? `${x.step} of ${x.steps}${x.kind ? ' (' + x.kind + ')' : ''}` : 'finished'),
        h('td', null, x.journal || '—'),
        h('td', null, x.need ? `${x.count} of ${x.need}` : ''),
        h('td', null, String(x.done)),
        h('td', null, act));
    });
    const quests = (await op('quest_list', {})).quests;
    const giveSel = h('select', null, ...quests.map((qq) => h('option', { value: qq.questid }, `${qq.questid} · ${qq.name}${Number(qq.enabled) ? '' : ' (draft)'}`)));
    const stepIn = h('input', { type: 'number', min: 0, value: 0, title: 'the step to start at (0: its first)' });
    stepIn.style.width = '6em';
    mount(body,
      h('p', null, h('strong', null, out.character), out.online ? h('span', { class: 'badge' }, ' in the world') : h('span', { class: 'muted' }, ' (not in the world: seen at the next login)')),
      out.quests.length ? h('div', { class: 'grid-scroll' }, h('table', { class: 'grid' },
        h('thead', null, h('tr', null, ...['Quest', 'Step', 'Journal line', 'Killed', 'Finished', ''].map((c) => h('th', null, c)))),
        h('tbody', null, ...rows))) : empty('No quests.'),
      h('div', { class: 'row' }, 'Give', giveSel, 'at step', stepIn,
        button('Give', { class: 'primary', onclick: () => set(Number(giveSel.value), 'give', Number(stepIn.value)) })));
  };
  await load();
}

// ---- drafts out of EQOA-Data

async function importer(box) {
  const note = h('div', { class: 'notice info' }, "The city quests (1-20) come in whole from tools/import_quests.py, which the server's operator runs: " +
    'this makes one-step drafts of any OTHER guide, and skips a section the import made.');
  const folder = h('input', { type: 'file', multiple: true });
  folder.setAttribute('webkitdirectory', '');
  const some = h('input', { type: 'file', multiple: true, accept: '.json,application/json' });
  const body = h('div', { class: 'stack' });
  mount(box,
    pageHead('Drafts from EQOA-Data', "EQOA-Data's quest guides (github.com/Jadiction/EQOA-Data, its Quests folder) made into drafts: " +
      'a quest a "# Level N" section of a class\'s guide, else a quest a file, with its giver, level, class, experience and reward ' +
      "items where this server has them by name, one step to talk to its giver, and the guide's words in its notes. Drafts are " +
      'offered to no one: write each one\'s steps from its notes and tick In the game. A guide imported before is skipped.'),
    tabs('import'),
    card('The guides', h('div', { class: 'stack' }, note,
      h('label', null, 'The Quests folder: ', folder),
      h('label', null, 'or some of its files: ', some),
      body)));
  let files = [];
  // batches under the panel's 4 MB a request
  const batches = () => {
    const out = [];
    let cur = [], size = 0;
    for (const f of files) {
      if (cur.length && (size + f.text.length > 2_500_000 || cur.length >= 60)) { out.push(cur); cur = []; size = 0; }
      cur.push(f);
      size += f.text.length;
    }
    if (cur.length) out.push(cur);
    return out;
  };
  const run = async (dryRun) => {
    const all = { drafts: [], made: 0, skipped: 0, errors: [] };
    for (const b of batches()) {
      const r = await op('quest_import_eqoadata', { files: b, dryRun });
      all.drafts.push(...(r.drafts || []));
      all.made += r.made || 0;
      all.skipped += r.skipped || 0;
      all.errors.push(...(r.errors || []));
    }
    return all;
  };
  const take = async (list) => {
    files = [];
    for (const f of list) {
      if (!f.name.endsWith('.json') || f.name === '_meta.json') continue;
      files.push({ path: f.webkitRelativePath || f.name, text: await f.text() });
    }
    if (!files.length) { mount(body, empty('No guides there (.json files).')); return; }
    mount(body, h('p', { class: 'muted' }, `Reading ${files.length} guides…`));
    let plan;
    try { plan = await run(true); } catch (e) { mount(body, h('p', { class: 'err' }, e.message)); return; }
    const fresh = plan.drafts.filter((d) => !d.exists);
    const rows = plan.drafts.map((d) => h('tr', null,
      h('td', null, d.name), h('td', null, d.giver ? (d.giverFound ? d.giver : h('span', { class: 'err' }, d.giver + ' ✗')) : '—'),
      h('td', null, String(d.level || '')), h('td', null, d.xp ? d.xp.toLocaleString() : ''),
      h('td', null, (d.itemNames || []).join(', ')), h('td', { class: 'err small' }, (d.unresolved || []).join('; ')),
      h('td', { class: 'muted small' }, d.exists ? 'made before' : '')));
    mount(body,
      h('p', null, `${plan.drafts.length} quests in ${files.length} guides: ${fresh.length} new, ${plan.drafts.length - fresh.length} made before.`),
      plan.errors.length ? h('p', { class: 'err' }, plan.errors.join('; ')) : null,
      fresh.length ? button(`Make ${fresh.length} drafts`, { class: 'primary', onclick: async () => {
        try {
          const r = await run(false);
          toast(`${r.made} drafts made, ${r.skipped} skipped`, 'ok', 'offered to no one until a GM turns them on');
          go('quests');
        } catch (e) { toast(e.message, 'error'); }
      } }) : null,
      h('div', { class: 'grid-scroll' }, h('table', { class: 'grid' },
        h('thead', null, h('tr', null, ...['Name', 'Giver', 'Level', 'Experience', 'Rewards named', 'Not on this server', ''].map((c) => h('th', null, c)))),
        h('tbody', null, ...rows))));
  };
  folder.addEventListener('change', () => take(folder.files));
  some.addEventListener('change', () => take(some.files));
}
