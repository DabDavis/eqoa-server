import { h, mount, pageHead, button, empty, toast } from '../lib/dom.js';
import { op } from '../lib/api.js';
import { playerActions } from './actions.js';

export async function render(box) {
  const list = h('div', { class: 'grid-wrap' });
  const count = h('span', { class: 'badge ok' }, '…');
  const auto = h('input', { type: 'checkbox', checked: true });
  mount(box, pageHead('Players online', 'Every character in the world, refreshed every few seconds.', null,
    count, h('label', { class: 'check' }, auto, 'Live'), button('Refresh', { icon: 'refresh', onclick: () => load() })), list);

  async function load() {
    let out;
    try { out = await op('online_players'); } catch (e) { toast(e.message, 'error'); return; }
    const ps = out.players;
    count.textContent = `${ps.length} online`;
    if (!ps.length) { mount(list, empty('Nobody is in the world right now.')); return; }
    mount(list, h('div', { class: 'grid-scroll' }, h('table', { class: 'grid' },
      h('thead', null, h('tr', null, ...['Character', 'Account', 'Level', 'World', 'Place', 'Health', 'Tunar', 'Doing', ''].map((t) => h('th', null, t)))),
      h('tbody', null, ...ps.map((p) => {
        const tr = h('tr', null,
          h('td', null, h('strong', null, p.name), p.guild ? h('span', { class: 'faint' }, ' <' + p.guild + '>') : null),
          h('td', null, p.account),
          h('td', { class: 'num' }, `${p.level} ${p.race} ${p.class}`),
          h('td', null, p.worldName),
          h('td', { class: 'mono' }, `${p.x}, ${p.y}, ${p.z}`),
          h('td', null, p.dead ? h('span', { class: 'badge danger' }, 'dead') : `${p.hp} / ${p.maxHP}`),
          h('td', { class: 'num' }, p.tunar.toLocaleString()),
          h('td', null, p.fighting ? 'fighting ' + p.fighting : ''),
          h('td', null, playerActions({ serverid: p.serverid, name: p.name, level: p.level, tunar: p.tunar, world: p.world, online: true }, () => load())));
        tr.addEventListener('click', () => { location.hash = '#/characters/' + p.serverid; });
        return tr;
      })))));
  }
  await load();
  const timer = setInterval(() => { if (auto.checked) load(); }, 5000);
  return { leave: () => clearInterval(timer) };
}
