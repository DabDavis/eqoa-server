// Fallen heroes: this shard's fallen Hardcore characters (fallen_list), the
// newest first, with what their fall kept; Unfall (unfall_character) brings one
// back for a death that was a mistake, with a reason for the audit log.

import { h, mount, pageHead, button, toast, card, empty, formBox } from '../lib/dom.js';
import { op } from '../lib/api.js';

const COLS = [['name', 'Character'], ['level', 'Level'], ['class', 'Class'], ['race', 'Race'], ['cms', 'CMs'],
  ['fell', 'Fell (UTC)'], ['place', 'Where'], ['killer', 'Killer'], ['cause', 'Cause'], ['played', 'Played'], ['lived', 'Lived']];

const LIVE = [['rank', '#'], ['name', 'Character'], ['level', 'Level'], ['class', 'Class'], ['race', 'Race'], ['cms', 'CMs'],
  ['xp', 'XP'], ['alive', 'Alive'], ['played', 'Played']];

async function loadLiving(liveBox) {
  let out;
  try {
    out = await op('hardcore_living', { limit: 100 });
  } catch (e) {
    mount(liveBox, h('p', { class: 'err' }, e.message));
    return;
  }
  const rows = out.living || [];
  if (!rows.length) {
    mount(liveBox, empty('No living Hardcore character on this shard.'));
    return;
  }
  mount(liveBox, h('div', { class: 'table-scroll' }, h('table', { class: 'grid' },
    h('thead', null, h('tr', null, ...LIVE.map(([, l]) => h('th', { scope: 'col' }, l)))),
    h('tbody', null, ...rows.map((r) => h('tr', null, ...LIVE.map(([k]) => k === 'name'
      ? h('td', null, h('a', { href: '#/characters/' + r.serverid, text: r.name }))
      : h('td', { class: typeof r[k] === 'number' ? 'num' : '' }, String(r[k] ?? '')))))))));
}

export async function render(box) {
  const listBox = h('div', { class: 'stack' });
  const liveBox = h('div', { class: 'stack' });
  async function load() {
    let out;
    try {
      out = await op('fallen_list', { limit: 500 });
    } catch (e) {
      mount(listBox, h('p', { class: 'err' }, e.message));
      return;
    }
    const rows = out.fallen || [];
    if (!rows.length) {
      mount(listBox, empty('No Hardcore character has fallen on this shard.'));
      return;
    }
    const unfall = async (r) => {
      const got = await formBox({
        title: `Unfall ${r.name}?`,
        intro: 'Only for a death that was a mistake (a bug, a lag spike). The character enters whole where it fell, still Hardcore.',
        fields: [{ name: 'reason', label: 'Reason (kept in the audit log)', required: true }],
        submitLabel: 'Unfall',
      });
      if (!got) return;
      try {
        await op('unfall_character', { character: r.serverid, reason: got.reason });
        toast(`${r.name} is back`, 'success');
        load();
      } catch (e) { toast(e.message, 'error'); }
    };
    mount(listBox, h('div', { class: 'table-scroll' }, h('table', { class: 'grid' },
      h('thead', null, h('tr', null, ...COLS.map(([, l]) => h('th', { scope: 'col' }, l)), h('th', null, ''))),
      h('tbody', null, ...rows.map((r) => h('tr', null,
        ...COLS.map(([k]) => k === 'name'
          ? h('td', null, h('a', { href: '#/characters/' + r.serverid, text: r.name }))
          : h('td', { class: typeof r[k] === 'number' ? 'num' : '' }, String(r[k] ?? ''))),
        h('td', null, button('Unfall', { onclick: () => unfall(r) }))))))));
  }
  mount(box, pageHead('Hardcore heroes', 'The living ranked, and the characters whose first death was their last'),
    card('Living Hardcore leaderboard', liveBox), card('Memorial of the Fallen', listBox));
  loadLiving(liveBox);
  load();
}
