import { h, mount, card, button, toast, empty, formBox, dialog } from '../lib/dom.js';
import { op } from '../lib/api.js';
import { describe } from '../lib/meta.js';
import { DataGrid } from '../lib/grid.js';
import { go } from '../lib/nav.js';
import { listPage, recordPage, relatedGrid, canWrite } from './tablepage.js';
import { modelPanel } from './disc.js';

export async function render(box, [id, tab]) {
  if (id === undefined) {
    const meta = await describe('npcs');
    const worlds = meta.enums.world || [];
    const world = h('select', { 'aria-label': 'World' }, h('option', { value: '' }, 'every world'), ...worlds.slice(0, 8).map((w) => h('option', { value: w.value }, w.label)));
    const kind = h('select', { 'aria-label': 'Kind' },
      h('option', { value: '' }, 'every kind'), h('option', { value: 'merchant' }, 'merchants'), h('option', { value: 'friendly' }, 'not attackable'),
      h('option', { value: 'hostile' }, 'attackable'));
    const page = await listPage(box, {
      table: 'npcs', base: 'npcs', title: 'NPCs',
      sub: 'Every NPC spawn. An edit takes effect in the running world at once.',
      tools: [world, kind],
    });
    const apply = () => {
      const f = [];
      if (world.value !== '') f.push({ column: 'world', op: '=', value: Number(world.value) });
      if (kind.value === 'merchant') f.push({ column: 'npc_type', op: 'bits', value: 1 });
      if (kind.value === 'friendly') f.push({ column: 'npc_type', op: 'bits', value: 128 });
      if (kind.value === 'hostile') f.push({ column: 'npc_type', op: '<', value: 128 });
      page.grid.setFilters(f);
    };
    world.addEventListener('change', apply);
    kind.addEventListener('change', apply);
    return page;
  }
  return recordPage(box, {
    table: 'npcs', keySeg: id, base: 'npcs',
    crumbs: h('a', { href: '#/npcs' }, 'NPCs'),
    title: (row) => row.npc_name || 'NPC ' + row.id,
    sub: (row) => `NPC ${row.id} · level ${row.npc_level}`,
    newDefaults: { size: 1, npc_level: 1, world: 0, npc_type: 128, race: '0', hp_percent: 100 },
    actions: (row, meta) => [
      canWrite(meta) ? button('Move to a character', { icon: 'pin', onclick: () => moveToCharacter(row) }) : null,
      button('On the map', { icon: 'map', onclick: () => go(`map/${row.world}/${row.id}`) }),
    ].filter(Boolean),
    side: async (row, meta, refresh) => {
      const out = await op('fetch', { id: 'npc:' + row.id });
      const r = out.record;
      const live = r.live;
      const act = (label, action, cls = 'small') => button(label, { class: cls, onclick: async () => {
        try { const o = await op('npc_action', { id: row.id, action }); toast(o.message, 'ok'); refresh(); } catch (e) { toast(e.message, 'error'); }
      } });
      return h('div', { class: 'stack' },
        card('In the world', live
          ? h('div', { class: 'stack' },
            h('div', { class: 'row' },
              live.dead ? h('span', { class: 'badge danger' }, 'dead') : h('span', { class: 'badge ok' }, 'alive'),
              h('span', { class: 'badge' }, r.className || 'Warrior'),
              ...(r.typeFlags || []).map((f) => h('span', { class: 'badge accent' }, f))),
            h('dl', { class: 'kv' },
              h('dt', null, 'Health'), h('dd', null, `${live.hp} / ${live.maxHP}`),
              h('dt', null, 'Stands at'), h('dd', null, `${r.worldName}: ${live.x.toFixed(1)}, ${live.y.toFixed(1)}, ${live.z.toFixed(1)}`),
              live.fighting ? [h('dt', null, 'Fighting'), h('dd', null, live.fighting)] : null,
              h('dt', null, 'Respawns'), h('dd', null, `${live.respawnSec} s after it dies`)),
            canWrite(meta) ? h('div', { class: 'actions' }, act('Respawn', 'respawn'), act('Kill', 'kill', 'small danger'), act('Reload its row', 'refresh'),
              button('Spawn a copy…', { class: 'small', onclick: () => spawnCopy(row) })) : null)
          : h('p', { class: 'muted' }, 'Not in the running world (the world has not loaded it, or its id is taken).')),
        r.spawnsOfThisName > 1 ? card('Same name', h('p', null, `${r.spawnsOfThisName} spawns are named ${row.npc_name}. `,
          h('a', { href: '#/npcs', onclick: (e) => { e.preventDefault(); showSameName(row.npc_name); } }, 'List them'))) : null,
        await modelPanel(row));
    },
    below: async (row) => {
      if (!(row.npc_type & 1)) {
        return [h('p', { class: 'muted small' }, 'Not a merchant. Tick Merchant in its type to give it a window and a stock.')];
      }
      const stock = await relatedGrid({
        table: 'npcitem', filters: [{ column: 'npcid', op: '=', value: row.id }], base: 'tables/npcitem',
        columns: ['itemid', 'patternid', 'stackleft', 'listnumber'], emptyText: 'Sells nothing yet.',
        tools: [button('Add an item', { class: 'small primary', icon: 'plus', onclick: async () => {
          const { pickRow } = await import('../lib/form.js');
          const item = await pickRow('itempattern', 'an item to sell');
          if (item === null) return;
          try {
            const next = stock.total;
            await op('insert_row', { table: 'npcitem', values: { npcid: row.id, patternid: item, stackleft: 1, listnumber: next } });
            toast('Added to the stock', 'ok', 'Its window shows it the next time it opens');
            stock.load();
          } catch (e) { toast(e.message, 'error'); }
        } })],
      });
      return [h('h2', null, 'What it sells'), stock.el];
    },
  });
}

async function moveToCharacter(row) {
  let players = [];
  try { players = (await op('online_players')).players; } catch (e) { toast(e.message, 'error'); return; }
  if (!players.length) { toast('Nobody is in the world to take a place from', 'warn'); return; }
  const v = await formBox({
    title: 'Move ' + row.npc_name, submitLabel: 'Move it there',
    intro: 'Its spawn point becomes where the character stands now (walk your GM character to the spot first). It reappears there in a second.',
    fields: [
      { name: 'who', label: 'Where this character stands', type: 'select', options: players.map((p) => ({ value: p.serverid, label: `${p.name} (${p.worldName} ${Math.round(p.x)}, ${Math.round(p.z)})` })) },
      { name: 'face', label: 'Face the way they face', type: 'checkbox', value: true },
    ],
  });
  if (!v) return;
  const p = players.find((x) => String(x.serverid) === String(v.who));
  const values = { x_coord: p.x, y_coord: p.y, z_coord: p.z, world: p.world };
  if (v.face) values.facing = Math.round(((((p.facing / (2 * Math.PI)) * 256) % 256) + 256) % 256);
  try {
    const out = await op('update_row', { table: 'npcs', key: { id: row.id }, values });
    toast('Moved', 'ok', out.live || '');
    location.reload();
  } catch (e) { toast(e.message, 'error'); }
}

async function spawnCopy(row) {
  let players = [];
  try { players = (await op('online_players')).players; } catch { /* coordinates only */ }
  const v = await formBox({
    title: 'A temporary copy of ' + row.npc_name, submitLabel: 'Spawn',
    intro: 'It has no row: it goes when killed (after its corpse time) or at a restart. Copy the row instead to keep it.',
    fields: [{
      name: 'beside', label: 'In front of', type: 'select',
      options: [{ value: '', label: 'its own spawn point' }, ...players.map((p) => ({ value: p.name, label: p.name }))],
    }],
  });
  if (!v) return;
  const args = v.beside ? { template: row.npc_name, beside: v.beside } : { template: row.npc_name, x: row.x_coord, y: row.y_coord, z: row.z_coord, world: row.world };
  try { const out = await op('spawn_npc', args); toast(out.message, 'ok'); } catch (e) { toast(e.message, 'error'); }
}

async function showSameName(name) {
  const meta = await describe('npcs');
  const grid = new DataGrid({
    table: 'npcs', meta, filters: [{ column: 'npc_name', op: '=', value: name }], columns: ['id', 'npc_level', 'world', 'x_coord', 'y_coord', 'z_coord'],
    onOpen: (r) => { d.close(); go('npcs/' + r.id); },
  });
  const d = dialog({ title: 'Spawns named ' + name, body: grid.el, wide: true });
  grid.load();
}

export { mount, empty };
