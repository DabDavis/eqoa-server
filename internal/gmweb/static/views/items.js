import { h, card, button, toast, empty, formBox } from '../lib/dom.js';
import { op } from '../lib/api.js';
import { listPage, recordPage, canWrite } from './tablepage.js';
import { itemPanel } from './disc.js';

export async function render(box, [id]) {
  if (id === undefined) {
    return listPage(box, { table: 'itempattern', base: 'items', title: 'Items', sub: 'Every item that exists. Players\' copies take an edit at their next login; merchants at once.' });
  }
  return recordPage(box, {
    table: 'itempattern', keySeg: id, base: 'items',
    crumbs: h('a', { href: '#/items' }, 'Items'),
    title: (row) => row.itemname || 'Item ' + row.patternid,
    sub: (row) => `Item ${row.patternid}`,
    newDefaults: { itemcost: 0, unk1: 0, itemicon: 0, unk2: 0, equipslot: 0, unk3: 0, trade: 1, rent: 0, unk4: 0, attacktype: 0, weapondamage: 0, unk5: 0,
      levelreq: 1, maxstack: 1, maxhp: 0, duration: 0, classuse: 32767, raceuse: 1023, procanim: 0, lore: 0, unk6: 0, craft: 0, itemname: '', itemdesc: '', model: 0, color: 0 },
    actions: (row, meta) => [canWrite(meta) ? button('Give to a character', { icon: 'send', onclick: () => give(row) }) : null].filter(Boolean),
    side: async (row) => {
      const out = await op('fetch', { id: 'item:' + row.patternid });
      const r = out.record;
      return h('div', { class: 'stack' },
        await itemPanel(row),
        card('Sold by', r.soldBy.length
          ? h('div', { class: 'table-scroll' }, h('table', { class: 'plain' }, h('tbody', null, ...r.soldBy.map((s) => h('tr', null,
            h('td', null, h('a', { href: '#/npcs/' + s.npcid }, s.npc_name || 'NPC ' + s.npcid)), h('td', { class: 'muted' }, 'world ' + s.world))))))
          : empty('No merchant sells it.')),
        card('Held', h('p', null, `${r.heldByCharacters} character item row${r.heldByCharacters === 1 ? '' : 's'}.`)));
    },
  });
}

async function give(row) {
  const v = await formBox({
    title: 'Give ' + row.itemname, submitLabel: 'Give',
    fields: [
      { name: 'character', label: 'Character (name or serverid)' },
      { name: 'count', label: 'How many', type: 'number', value: 1 },
      { name: 'location', label: 'Where', type: 'select', options: [{ value: 'inventory', label: 'Inventory' }, { value: 'bank', label: 'Bank' }], value: 'inventory' },
    ],
  });
  if (!v || !v.character.trim()) return;
  try {
    const out = await op('give_item', { character: v.character.trim(), item: row.patternid, count: v.count || 1, location: v.location });
    toast(out.message, 'ok');
  } catch (e) { toast(e.message, 'error'); }
}
