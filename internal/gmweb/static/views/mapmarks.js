// Map suggestions: the places players suggest for every map, sent from
// eqoa-rust-client-vanilla's map on any game server with no sign-in
// (internal/mapmarks), each with what is already on the maps near it. Accept
// puts it on every map (accept_map_suggestion: an audited insert into mapmarks,
// revertible in the audit log), its name, note, kind and place the GM's to change
// first; Dismiss sets it aside. What a player typed is shown as text. A sender is
// the first 8 characters of a salted hash of its address; a spammer's waiting
// rows go with Delete sender. Below, the marks on the maps now (map_marks), each
// opening its row to edit or delete.

import { h, mount, pageHead, button, toast, card, empty, confirmBox } from '../lib/dom.js';
import { op } from '../lib/api.js';
import { describe } from '../lib/meta.js';

export async function render(box) {
  mount(box, pageHead('Map suggestions', 'Loading…'));
  let worlds = [];
  try {
    worlds = ((await describe('mapmarks')).enums.world || []).slice(0, 10);
  } catch { /* the names stay numbers */ }
  const worldName = (w) => (worlds.find((x) => x.value === w) || {}).label || `World ${w}`;

  const status = h('select', { 'aria-label': 'Which suggestions' },
    h('option', { value: 'waiting' }, 'Waiting'),
    h('option', { value: 'accepted' }, 'Accepted'),
    h('option', { value: 'dismissed' }, 'Dismissed'),
    h('option', { value: 'all' }, 'All'));
  const world = h('select', { 'aria-label': 'World' },
    h('option', { value: '' }, 'Every world'), ...worlds.map((w) => h('option', { value: String(w.value) }, `${w.label} (${w.value})`)));
  const countLine = h('span', { class: 'faint small' });
  const listBox = h('div', { class: 'stack' });
  const marksBox = h('div', { class: 'stack' });

  async function decide(name, args, done) {
    try {
      const out = await op(name, args);
      toast(done(out), 'ok', out.change && out.change.live ? out.change.live : '');
      load();
    } catch (e) {
      toast(e.message, 'error');
    }
  }

  function suggestionCard(sg, kinds) {
    const waiting = sg.status === 'waiting';
    const name = h('input', { type: 'text', value: sg.name, maxlength: 40, 'aria-label': 'Name' });
    const note = h('input', { type: 'text', value: sg.note, maxlength: 200, 'aria-label': 'Note', placeholder: 'no note' });
    const kind = h('select', { 'aria-label': 'Kind' }, ...kinds.map((k) => h('option', { value: k }, k)));
    const x = h('input', { type: 'number', step: '0.1', value: sg.x, 'aria-label': 'X' });
    const z = h('input', { type: 'number', step: '0.1', value: sg.z, 'aria-label': 'Z' });
    const near = sg.near.length
      ? h('div', { class: 'notice warn small' }, 'Already on the maps near it: ',
        ...sg.near.flatMap((m, i) => [i ? ', ' : null, h('a', { href: '#/tables/mapmarks/' + m.id }, m.name), ` (${m.kind}, ${m.distance} off)`]))
      : null;
    const decided = !waiting
      ? h('div', { class: 'faint small' }, `${sg.status} by ${sg.decided_by || '?'} ${sg.decided_at || ''}`,
        sg.mark_id ? h('span', null, ' · ', h('a', { href: '#/tables/mapmarks/' + sg.mark_id }, 'its mark')) : null)
      : null;
    return card(sg.name, h('div', { class: 'stack' },
      h('div', { class: 'row small' },
        h('span', null, `${sg.worldName} (${sg.world})`), h('span', { class: 'faint' }, '·'),
        h('span', { class: 'mono' }, `${sg.x}, ${sg.z}`), h('span', { class: 'faint' }, '·'),
        h('a', { href: `#/map/${sg.world}/@${sg.x},${sg.z}` }, 'On the World map'),
        h('span', { class: 'grow' }),
        h('span', { class: 'faint' }, `sent ${sg.created}`, sg.client ? ` · ${sg.client}` : '', ' · sender '),
        h('span', { class: 'mono' }, sg.sender)),
      sg.note ? h('div', { class: 'small' }, '“', sg.note, '”') : h('div', { class: 'faint small' }, 'no note'),
      near, decided,
      waiting ? h('div', { class: 'row' },
        h('label', { class: 'stack small' }, 'Name', name),
        h('label', { class: 'stack small grow' }, 'Note', note),
        h('label', { class: 'stack small' }, 'Kind', kind),
        h('label', { class: 'stack small' }, 'X', x),
        h('label', { class: 'stack small' }, 'Z', z)) : null,
      waiting ? h('div', { class: 'row' },
        button('Accept', {
          class: 'small primary', title: 'Put it on every map',
          onclick: () => decide('accept_map_suggestion', {
            id: sg.id, name: name.value, note: note.value, kind: kind.value, x: Number(x.value), z: Number(z.value),
          }, (out) => out.message),
        }),
        button('Dismiss', { class: 'small ghost', title: 'Set it aside', onclick: () => decide('dismiss_map_suggestion', { id: sg.id }, () => 'Dismissed') }),
        h('span', { class: 'grow' }),
        button('Delete sender ' + sg.sender, {
          class: 'small ghost', title: "Delete every waiting suggestion this sender made",
          onclick: async () => {
            if (!(await confirmBox({ title: 'Delete sender ' + sg.sender + '?', message: "Every waiting suggestion this sender made is deleted (decided ones stay). This cannot be undone.", confirmLabel: 'Delete', danger: true }))) return;
            decide('clear_map_suggestions', { sender: sg.sender }, (out) => `Deleted ${out.deleted} suggestion${out.deleted === 1 ? '' : 's'} of ${sg.sender}`);
          },
        })) : null));
  }

  function marksTable(marks) {
    if (!marks.length) return empty('No marks on the maps yet.');
    return h('div', { class: 'table-scroll' }, h('table', { class: 'grid' },
      h('thead', null, h('tr', null, ...['Name', 'Kind', 'World', 'Place', 'Note', 'Source'].map((t) => h('th', { scope: 'col' }, t)))),
      h('tbody', null, ...marks.map((m) => h('tr', null,
        h('td', null, h('a', { href: '#/tables/mapmarks/' + m.id }, m.name)),
        h('td', null, m.kind),
        h('td', null, worldName(m.world)),
        h('td', { class: 'mono' }, h('a', { href: `#/map/${m.world}/@${m.x},${m.z}` }, `${m.x}, ${m.z}`)),
        h('td', { class: 'small' }, m.note),
        h('td', { class: 'faint small' }, m.source))))));
  }

  async function load() {
    const args = { status: status.value };
    if (world.value !== '') args.world = Number(world.value);
    mount(listBox, h('p', { class: 'faint' }, 'Loading…'));
    try {
      const out = await op('map_suggestions', args);
      countLine.textContent = `${out.matching} suggestion${out.matching === 1 ? '' : 's'}${out.matching > out.suggestions.length ? `, the ${out.suggestions.length} newest shown` : ''} · ${out.waiting} waiting in all`;
      mount(listBox, out.suggestions.length ? out.suggestions.map((sg) => suggestionCard(sg, out.kinds))
        : empty(status.value === 'waiting' ? 'No suggestions waiting. Players send them from the map in the game.' : 'None.'));
    } catch (e) {
      mount(listBox, h('p', { class: 'err' }, e.message));
    }
    try {
      const out = await op('map_marks', world.value !== '' ? { world: Number(world.value) } : {});
      mount(marksBox, marksTable(out.marks));
    } catch (e) {
      mount(marksBox, h('p', { class: 'err' }, e.message));
    }
  }
  for (const el of [status, world]) el.addEventListener('change', load);

  mount(box,
    pageHead('Map suggestions', 'Places players suggest for every map, to accept or set aside.'),
    card('Show', h('div', { class: 'stack' },
      h('div', { class: 'row' }, status, world),
      countLine,
      h('p', { class: 'muted small' }, 'Players suggest a place from the map in the game (vanilla, on any server; no sign-in, 20 waiting a sender). Accept puts it on every map within a minute, with the name, note, kind and place as edited here (revertible in the Audit log); Dismiss sets it aside.'))),
    listBox,
    card('On the maps now', marksBox));
  load();
}
