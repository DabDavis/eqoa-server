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
  if (seg === undefined || seg === '') return list(box);
  return editor(box, seg === 'new' ? 0 : Number(seg));
}

function tabs(on) {
  const t = (id, label, path) => h('button', { type: 'button', class: id === on ? 'on' : '', onclick: () => go(path) }, label);
  return h('div', { class: 'tabs' }, t('list', 'Quests', 'quests'), t('chars', "A character's quests", 'quests/characters'),
    t('import', 'Drafts from EQOA-Data', 'quests/import'));
}

// ---- the list

async function list(box) {
  const search = h('input', { type: 'search', placeholder: 'Search by name, giver or note', 'aria-label': 'Search quests' });
  const body = h('div');
  mount(box,
    pageHead('Quests', "What NPCs give. A quest's giver offers it when talked to; its steps are done by talking to an NPC, " +
      'showing or handing one items, killing creatures and reaching places; the journal shows the step to do. ' +
      'A draft is offered to no one, but a GM can try it.', null,
    button('New quest', { class: 'primary', icon: 'plus', onclick: () => go('quests/new') })),
    tabs('list'), card(null, h('div', { class: 'stack' }, search, body)));
  const load = async () => {
    try {
      const out = await op('quest_list', { search: search.value.trim() });
      if (!out.quests.length) {
        mount(body, empty(search.value ? 'No quest matches.' : 'No quests yet: make one, or make drafts out of EQOA-Data.'));
        return;
      }
      const rows = out.quests.map((q) => h('tr', { onclick: () => go('quests/' + q.questid) },
        h('td', { class: 'mono' }, String(q.questid)), h('td', null, q.name || '(no name)'),
        h('td', null, Number(q.enabled) ? h('span', { class: 'badge' }, 'in the game') : h('span', { class: 'muted' }, 'draft')),
        h('td', null, q.giver || '—'), h('td', null, `${q.minlevel}–${q.maxlevel || 'any'}`), h('td', null, String(q.steps)),
        h('td', null, String(q.underway)), h('td', null, String(q.finished)), h('td', { class: 'muted small' }, q.source || '')));
      mount(body, h('div', { class: 'grid-scroll' }, h('table', { class: 'grid' },
        h('thead', null, h('tr', null, ...['#', 'Name', '', 'Giver', 'Levels', 'Steps', 'Under way', 'Finished', 'Source'].map((c) => h('th', null, c)))),
        h('tbody', null, ...rows))));
    } catch (e) {
      mount(body, h('p', { class: 'err' }, e.message));
    }
  };
  search.addEventListener('input', debounce(load, 250));
  await load();
}

// ---- inputs bound to an object's keys

function field(label, input, hint, wide) {
  return h('div', { class: 'field' + (wide ? ' wide' : '') },
    h('label', null, label, hint ? h('span', { class: 'hint' }, ' · ' + hint) : null), input);
}

function text(obj, key, attrs = {}) {
  const i = h('input', { type: 'text', value: obj[key] ?? '', ...attrs });
  i.addEventListener('input', () => { obj[key] = i.value; });
  return i;
}

function num(obj, key, attrs = {}) {
  const i = h('input', { type: 'number', value: obj[key] ?? 0, ...attrs });
  i.addEventListener('input', () => { obj[key] = i.value === '' ? 0 : Number(i.value); });
  return i;
}

function area(obj, key, rows = 3) {
  const t = h('textarea', { rows });
  t.value = obj[key] ?? '';
  t.addEventListener('input', () => { obj[key] = t.value; });
  return t;
}

function check(obj, key, label) {
  const c = h('input', { type: 'checkbox', checked: !!Number(obj[key]) });
  c.addEventListener('change', () => { obj[key] = c.checked ? 1 : 0; });
  return h('label', { class: 'check' }, c, label);
}

// counted is a text input that says how many of max characters it holds.
function counted(obj, key, max) {
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

// pick asks for an NPC or an item by name (the search operation).
function pick(kind, title) {
  return new Promise((resolve) => {
    let done = false;
    let dlg;
    const search = h('input', { type: 'search', placeholder: 'part of a name', autocomplete: 'off' });
    const out = h('div', { class: 'stack' });
    const run = debounce(async () => {
      const q = search.value.trim();
      if (!q) { mount(out, h('p', { class: 'muted small' }, 'Type part of a name.')); return; }
      try {
        const r = await op('search', { query: q, kinds: [kind], limit: 25 });
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
function npcInput(obj, key, what) {
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
function itemsInput(obj, key) {
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
      offer: '', accept: '', decline: '', declined: '', started: '', source: 'gm', notes: '', events: '' },
    steps: [], problems: [], characters: [],
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
        chance: 0, events: '' });
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
      field('Reward: spells taught', text(s, 'spells', { placeholder: 'spell ids, comma separated', class: 'mono' }), null, true),
      field('Events', eventsEditor(s, 'events', STEP_WHENS, worlds), 'run in order after the step\'s own words', true));
    const move = (d) => { const j = i + d; if (j < 0 || j >= steps.length) return; [steps[i], steps[j]] = [steps[j], steps[i]]; drawSteps(); };
    return card(`Step ${i + 1} · ${KINDS[s.kind] || s.kind}`, h('div', { class: 'fields' }, ...fields),
      button('↑', { title: 'Earlier', onclick: () => move(-1) }), button('↓', { title: 'Later', onclick: () => move(1) }),
      button('Remove', { icon: 'trash', onclick: () => { steps.splice(i, 1); drawSteps(); } }));
  };
  drawSteps();

  const save = button('Save', { class: 'primary', icon: 'save', onclick: async () => {
    try {
      const out = await op('quest_save', { quest: { ...q, questid: id || undefined }, steps });
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
          field('Quests finished first', text(q, 'requires', { placeholder: 'quest ids, comma separated', class: 'mono' }), null, true))),
        card('What the giver says', h('div', { class: 'fields' },
          field('Offer', area(q, 'offer', 6), 'a paragraph a question: its lines a box each, its last line the question (no name before it)', true),
          field('Answers taking it', area(q, 'accept', 2), 'a line a question; empty: taken as soon as the giver is talked to', true),
          field('Answer refusing it', counted(q, 'decline', 63), 'offered with each answer; empty: none', true),
          field('Reply to a refusal', area(q, 'declined', 2), 'said as written, no name put before it', true),
          field('Once taken', area(q, 'started', 3), 'a line a box', true),
          h('p', { class: 'muted small field wide' }, "An NPC's lines are shown with its name before them, as Sandstorm's are, unless they begin with it."))),
        card('Events', h('div', { class: 'fields' },
          field('', eventsEditor(q, 'events', QUEST_WHENS, worlds), "run in order after the giver's words when it is taken, and after the last step's when it is finished", true))),
        card('Steps', stepsBox)),
      h('div', { class: 'stack' },
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
    card('The guides', h('div', { class: 'stack' },
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
