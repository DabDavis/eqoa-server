// The GMs' triggers (handlers/gmtriggers.go): scripted events. Slots 1 to 8
// are the lines of the GM's TEST > Programming > TRIGGER menu in the game
// (vanilla shows each one's name); a slot past 8 is fired by #trigger or here.
// Saving is a row change, logged and revertible; firing and stopping are
// logged.

import { h, mount, card, button, toast, pageHead, empty, confirmBox } from '../lib/dom.js';
import { op } from '../lib/api.js';
import { go, state } from '../lib/nav.js';
import { describe } from '../lib/meta.js';
import { field, text, num, area, check, counted, npcInput, itemsInput } from './quests.js';

const SCOPES = {
  near: 'Players near its place',
  gm: 'The GM who fires it',
  world: 'Everyone in its world',
  all: 'Everyone in the game',
};
const DOS = {
  broadcast: 'A line in their chat',
  popup: 'A window they close',
  say: 'An NPC says',
  give: 'Give items',
  visual: 'Play a spell visual on them',
  teleport: 'Teleport them',
  spawn: 'Spawn NPCs round the place',
  despawn: "Take this run's spawns away",
  stop: "Stop another trigger's runs",
  wait: 'Wait',
};
const MENU = 8;

export async function render(box, parts) {
  const [seg] = parts;
  if (seg === undefined || seg === '') return list(box);
  // #/triggers/new, or #/triggers/new?3 for a new one in slot 3
  if (seg.startsWith('new')) return editor(box, 0, Number(seg.split('?')[1]) || 0);
  return editor(box, Number(seg), 0);
}

function place(t, worlds) {
  if (Number(t.world) < 0) return 'where the GM stands';
  const w = worlds.find((x) => x.value === Number(t.world));
  return `${Math.round(t.x)}, ${Math.round(t.z)} in ${w ? w.label : 'world ' + t.world}`;
}

function reaches(t) {
  if (t.scope === 'near') return `within ${Number(t.radius) > 0 ? t.radius : 100}`;
  return SCOPES[t.scope] ? t.scope : t.scope + '?';
}

async function fire(slot, then) {
  try {
    const out = await op('trigger_fire', { slot });
    toast(`Trigger ${slot} fired`, 'ok', out.lines.join(' · '));
  } catch (e) { toast(e.message, 'error'); }
  if (then) then();
}

async function stop(slot, then) {
  const ok = await confirmBox({ title: slot ? `Stop trigger ${slot}'s runs?` : 'Stop every run?',
    message: 'Their steps not yet run are left undone, and their spawns still alive are taken away.', confirmLabel: 'Stop', danger: true });
  if (!ok) return;
  try {
    const out = await op('trigger_stop', { slot });
    toast(out.message, 'ok');
  } catch (e) { toast(e.message, 'error'); }
  if (then) then();
}

function runsTable(runs, worlds, reload) {
  if (!runs.length) return empty('Nothing runs now.');
  return h('div', { class: 'grid-scroll' }, h('table', { class: 'grid' },
    h('thead', null, h('tr', null, ...['Trigger', 'Fired by', 'At', 'Started', 'Next step', 'Spawns alive', ''].map((x) => h('th', null, x)))),
    h('tbody', null, ...runs.map((r) => h('tr', null,
      h('td', null, h('a', { href: '#/triggers/' + r.slot }, `${r.slot} ${r.name || ''}`)),
      h('td', null, r.by || '—'),
      h('td', { class: 'small' }, place(r, worlds)),
      h('td', { class: 'small' }, new Date(r.started).toLocaleTimeString()),
      h('td', { class: 'small' }, r.step > r.steps ? 'all done' : `${r.step} of ${r.steps}${r.waitFor ? ` (in ${Math.ceil(r.waitFor)} s)` : ''}`),
      h('td', { class: 'num' }, String(r.spawns)),
      h('td', null, button('Stop', { onclick: () => stop(r.slot, reload) })))))));
}

async function worldList() {
  try { return (await describe('spellgates')).enums?.world || []; } catch { return []; }
}

// ---- the list

async function list(box) {
  const worlds = await worldList();
  const body = h('div');
  const runsBox = h('div');
  mount(box,
    pageHead('Triggers', 'Scripted events a GM fires: lines in chat, windows, an NPC\'s words, items, spell visuals, teleports, ' +
      'creatures spawned for a while, with waits between. Slots 1 to 8 are the lines of the GM\'s TEST > Programming > TRIGGER ' +
      'menu in the game, top to bottom (a PS2 shows eight TRIGGERs, vanilla each one\'s name); any slot fires with #trigger <n> ' +
      'or from here. A hostile spawn attacks the GM too: Safe Mode (TEST > Avatar > Self) or #aggro off keeps them off you.', null,
    button('New trigger', { class: 'primary', icon: 'plus', onclick: () => go('triggers/new') })),
    card(null, body),
    card('Running now', runsBox, button('Stop every run', { onclick: () => stop(0, load) })));
  let busy = false;
  const load = async () => {
    if (busy) return;
    busy = true;
    let out;
    try {
      out = await op('trigger_list', {});
    } catch (e) {
      mount(body, h('p', { class: 'err' }, e.message));
      busy = false;
      return;
    }
    busy = false;
    const bySlot = new Map(out.triggers.map((t) => [t.slot, t]));
    const slots = [...new Set([...Array.from({ length: MENU }, (_, i) => i + 1), ...out.triggers.map((t) => t.slot)])].sort((a, b) => a - b);
    const rows = slots.map((slot) => {
      const t = bySlot.get(slot);
      const line = slot > MENU ? h('span', { class: 'muted small' }, ' · not in the menu') : null;
      if (!t) {
        return h('tr', { class: 'muted' }, h('td', { class: 'mono' }, String(slot), line), h('td', { colspan: 6 }, 'empty'),
          h('td', null, button('Make one', { onclick: () => go('triggers/new?' + slot) })));
      }
      return h('tr', null,
        h('td', { class: 'mono' }, String(slot), line),
        h('td', null, h('a', { href: '#/triggers/' + slot }, t.name || '(no name)')),
        h('td', null, t.enabled ? h('span', { class: 'badge ok' }, 'on') : h('span', { class: 'badge' }, 'off')),
        h('td', { class: 'small' }, reaches(t)),
        h('td', { class: 'small' }, place(t, worlds)),
        h('td', { class: 'num' }, String(t.steps)),
        h('td', null, t.problems ? h('span', { class: 'badge warn' }, `${t.problems} problem${t.problems === 1 ? '' : 's'}`) : ''),
        h('td', null, t.running ? h('span', { class: 'badge info' }, 'running') : '',
          button('Fire', { disabled: !t.enabled || t.problems > 0, title: t.problems ? 'Mend its problems first' : 'Fire it now',
            onclick: () => fire(slot, load) }),
          t.running ? button('Stop', { onclick: () => stop(slot, load) }) : null));
    });
    mount(body, h('div', { class: 'grid-scroll' }, h('table', { class: 'grid' },
      h('thead', null, h('tr', null, ...['Slot', 'Name', 'On', 'Reaches', 'Where', 'Steps', 'Problems', ''].map((x) => h('th', null, x)))),
      h('tbody', null, ...rows))));
    mount(runsBox, runsTable(out.runs, worlds, load));
  };
  await load();
  const timer = setInterval(load, 3000);
  return { leave: () => clearInterval(timer) };
}

// ---- the editor

// here sets obj's world, x, y, z to where the GM's character stands.
function hereButton(obj, redraw) {
  return button('Where my character stands', { onclick: async () => {
    try {
      const out = await op('online_players', {});
      const me = out.players.find((p) => p.accountid === state.session?.user?.accountid);
      if (!me) { toast('You have no character in the world', 'error'); return; }
      Object.assign(obj, { world: me.world, x: me.x, y: me.y, z: me.z });
      redraw();
      toast(`${me.name} stands at ${me.x}, ${me.z} in ${me.worldName}`, 'ok');
    } catch (e) { toast(e.message, 'error'); }
  } });
}

function worldSelect(obj, key, worlds) {
  const w = h('select', null, ...(worlds.length ? worlds : [{ value: 0, label: 'World 0' }]).map((x) =>
    h('option', { value: x.value, selected: Number(obj[key] || 0) === x.value }, x.label)));
  w.addEventListener('change', () => { obj[key] = Number(w.value); });
  return w;
}

// stepsEditor edits obj.steps, a JSON list of steps, a card a step. Text that
// is not such a list is shown as it is, to be mended by hand.
function stepsEditor(obj, worlds) {
  const box = h('div', { class: 'stack' });
  let list;
  try {
    list = obj.steps ? JSON.parse(obj.steps) : [];
    if (!Array.isArray(list)) throw new Error('not a list');
  } catch (e) {
    mount(box, h('p', { class: 'err small' }, "These steps are not a JSON list (" + e.message + "): mend them here."), area(obj, 'steps', 8));
    return box;
  }
  const keep = () => { obj.steps = list.length ? JSON.stringify(list) : ''; };
  box.addEventListener('input', keep);
  box.addEventListener('change', keep);
  const flag = (e, k, label) => {
    const c = h('input', { type: 'checkbox', checked: !!e[k] });
    c.addEventListener('change', () => { if (c.checked) e[k] = true; else delete e[k]; keep(); });
    return h('label', { class: 'check' }, c, label);
  };
  // a spawn's place of its own: world, x, y, z set, else the trigger's
  const ownPlace = (e) => {
    const c = h('input', { type: 'checkbox', checked: e.world !== undefined });
    c.addEventListener('change', () => {
      if (c.checked) Object.assign(e, { world: 0, x: 0, y: 0, z: 0 });
      else { delete e.world; delete e.x; delete e.y; delete e.z; }
      keep(); draw();
    });
    return h('label', { class: 'check' }, c, "A place of their own (else the trigger's)");
  };
  const row = (e, i) => {
    const what = h('select', null, ...Object.entries(DOS).map(([k, label]) => h('option', { value: k, selected: e.do === k }, label)));
    what.addEventListener('change', () => { e.do = what.value; keep(); draw(); });
    const fields = [field('What', what, null, true)];
    const placeFields = () => [field('World', worldSelect(e, 'world', worlds)), field('x', num(e, 'x', { step: 'any' })),
      field('y', num(e, 'y', { step: 'any' })), field('z', num(e, 'z', { step: 'any' })), field('', hereButton(e, draw))];
    if (e.do === 'broadcast') {
      fields.push(field('Text', area(e, 'text', 2), "%p: each player's name; the GMs' yellow", true));
    } else if (e.do === 'popup') {
      fields.push(field('Text', area(e, 'text', 2), "%p: each player's name; its last 127 characters ask", true),
        field('Its answer', text(e, 'answer', { placeholder: 'OK' })));
    } else if (e.do === 'say') {
      fields.push(field('Who', npcInput(e, 'npc', 'who says it'), 'any name; empty: no name before the words', true),
        field('Words', area(e, 'text', 2), 'a line a box', true));
    } else if (e.do === 'give') {
      fields.push(field('Items', itemsInput(e, 'items'), 'to each player it reaches', true));
    } else if (e.do === 'visual') {
      fields.push(field('Visual', text(e, 'fx', { placeholder: '0x1DAEB5B6', class: 'mono' }), 'a 0x00B1 DictID (the Spell visuals page lists them)'),
        field('Casting part', num(e, 'seconds', { min: 0, max: 30, step: 0.1 }), 'seconds'));
    } else if (e.do === 'teleport') {
      if (e.world === undefined) e.world = 0;
      fields.push(...placeFields());
    } else if (e.do === 'spawn') {
      fields.push(field('Whom', npcInput(e, 'npc', 'the NPC to copy'), 'copies of it, as it is named in the game', true),
        field('How many', num(e, 'count', { min: 1, max: 50 }), '1 to 50'),
        field('Spread', num(e, 'spread', { min: 0, max: 200, step: 'any' }), 'how far round the place; 0: 5'),
        field('Stay', num(e, 'minutes', { min: 0, max: 240, step: 'any' }), 'minutes alive at most; 0: 30'),
        field('', flag(e, 'hostile', 'Hostile: they may be attacked, and attack any player on sight (in a town too)')),
        field('', ownPlace(e)));
      if (e.world !== undefined) fields.push(...placeFields());
    } else if (e.do === 'despawn') {
      fields.push(field('Whom', npcInput(e, 'npc', 'whose copies'), "empty: every spawn of this run", true));
    } else if (e.do === 'stop') {
      fields.push(field('Trigger', num(e, 'slot', { min: 0 }), "its slot; 0: every run but this one"));
    } else if (e.do === 'wait') {
      fields.push(field('Seconds', num(e, 'seconds', { min: 0.5, max: 3600, step: 'any' }), 'the steps after it run then (at most 3600)'));
    }
    const move = (d) => { const j = i + d; if (j < 0 || j >= list.length) return; [list[i], list[j]] = [list[j], list[i]]; keep(); draw(); };
    return card(`Step ${i + 1} · ${DOS[e.do] || e.do}`, h('div', { class: 'fields' }, ...fields),
      button('↑', { title: 'Earlier', onclick: () => move(-1) }), button('↓', { title: 'Later', onclick: () => move(1) }),
      button('Remove', { icon: 'trash', onclick: () => { list.splice(i, 1); keep(); draw(); } }));
  };
  const draw = () => mount(box, ...list.map(row), list.length ? null : empty('No steps yet: a trigger needs one to do anything.'),
    button('Add a step', { icon: 'plus', onclick: () => { list.push({ do: 'broadcast', text: '' }); keep(); draw(); } }));
  draw();
  return box;
}

// editor is a trigger's page: slot's (an empty slot opens as a new one), or
// a new one in want (0: the first empty slot).
async function editor(box, slot, want) {
  const worlds = await worldList();
  let data = null;
  if (slot) {
    try {
      data = await op('trigger_get', { slot });
    } catch (e) {
      if (!String(e.message).includes('no trigger')) {
        mount(box, pageHead('Trigger ' + slot, null, h('a', { href: '#/triggers' }, 'Triggers')), h('p', { class: 'err' }, e.message));
        return;
      }
      want = slot;
    }
  }
  const fresh = !data;
  if (fresh) {
    if (!want) {
      try {
        const have = new Set((await op('trigger_list', {})).triggers.map((t) => t.slot));
        want = 1;
        while (have.has(want)) want++;
      } catch { want = 1; }
    }
    data = { trigger: { slot: want, name: '', enabled: 1, scope: 'near', radius: 0, world: -1, x: 0, y: 0, z: 0, steps: '', notes: '' },
      problems: [], runs: [] };
  }
  const t = { ...data.trigger };

  const problems = h('div');
  const showProblems = (ps) => mount(problems, ps && ps.length
    ? h('ul', { class: 'steps' }, ...ps.map((p) => h('li', { class: 'err' }, p)))
    : h('p', { class: 'muted' }, fresh ? 'Saved triggers are checked here.' : 'Nothing found wrong.'));
  showProblems(data.problems);

  const runsBox = h('div');
  const showRuns = (runs) => mount(runsBox, runsTable(runs || [], worlds, () => go('triggers/' + t.slot)));
  showRuns(data.runs);

  const where = h('div', { class: 'fields' });
  const drawWhere = () => {
    const own = Number(t.world) >= 0;
    const mode = h('select', null, h('option', { value: 'gm', selected: !own }, 'Where the GM stands when firing it'),
      h('option', { value: 'own', selected: own }, 'A place of its own'));
    mode.addEventListener('change', () => { t.world = mode.value === 'own' ? 0 : -1; drawWhere(); });
    const fields = [field('Its place', mode, null, true)];
    if (own) {
      fields.push(field('World', worldSelect(t, 'world', worlds)), field('x', num(t, 'x', { step: 'any' })),
        field('y', num(t, 'y', { step: 'any' })), field('z', num(t, 'z', { step: 'any' })), field('', hereButton(t, drawWhere)));
    }
    const scope = h('select', null, ...Object.entries(SCOPES).map(([k, label]) => h('option', { value: k, selected: t.scope === k }, label)));
    scope.addEventListener('change', () => { t.scope = scope.value; drawWhere(); });
    fields.push(field('Its steps reach', scope, 'looked at as each step runs', true));
    if (t.scope === 'near') fields.push(field('Within', num(t, 'radius', { min: 0, step: 'any' }), 'of its place; 0: 100'));
    mount(where, ...fields);
  };
  drawWhere();

  const save = button('Save', { class: 'primary', icon: 'save', onclick: async () => {
    try {
      const out = await op('trigger_save', { trigger: { ...t, slot: Number(t.slot) } });
      toast(out.change ? 'Saved' : 'Nothing to save', 'ok', out.change?.live || '');
      showProblems(out.problems);
      if (fresh) go('triggers/' + t.slot);
    } catch (e) { toast(e.message, 'error'); }
  } });
  const fireB = button('Fire', { icon: 'play', title: 'Save first: it fires as saved', onclick: async () => {
    await fire(Number(t.slot));
    try { showRuns((await op('trigger_get', { slot: Number(t.slot) })).runs); } catch { /* not saved */ }
  } });
  const del = fresh ? null : button('Delete', { icon: 'trash', onclick: async () => {
    const ok = await confirmBox({ title: `Delete trigger ${t.slot}?`, message: 'Its row goes (logged and revertible); its runs go on until stopped.',
      confirmLabel: 'Delete', danger: true });
    if (!ok) return;
    try {
      await op('trigger_delete', { slot: Number(t.slot) });
      toast(`Trigger ${t.slot} deleted`, 'ok');
      go('triggers');
    } catch (e) { toast(e.message, 'error'); }
  } });

  const slotNote = () => Number(t.slot) >= 1 && Number(t.slot) <= MENU ? `TEST > Programming > TRIGGER, line ${t.slot}` : 'fired by #trigger or here';
  mount(box,
    pageHead(fresh ? 'New trigger' : (t.name || 'Trigger ' + t.slot), fresh ? 'A trigger fires only once saved.' : `Slot ${t.slot} · ${slotNote()}`,
      h('a', { href: '#/triggers' }, 'Triggers'), del, fireB, save),
    h('div', { class: 'cols-2' },
      h('div', { class: 'stack' },
        card('The trigger', h('div', { class: 'fields' },
          field('Slot', num(t, 'slot', { min: 1, disabled: !fresh }), '1 to 8: the TEST menu\'s TRIGGER line, top to bottom'),
          field('Name', counted(t, 'name', 32), "vanilla's TEST menu shows it: about a dozen characters fit its line", true),
          field('', check(t, 'enabled', 'On (may be fired)')))),
        card('Where, and whom', where),
        card('Steps', stepsEditor(t, worlds))),
      h('div', { class: 'stack' },
        card('Problems', problems),
        card('Running now', runsBox),
        card('Notes', h('div', { class: 'fields' }, field('', area(t, 'notes', 8), null, true))))));
}
