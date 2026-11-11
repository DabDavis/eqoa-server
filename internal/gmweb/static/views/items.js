import { h, card, button, toast, empty, formBox, mount } from '../lib/dom.js';
import { op } from '../lib/api.js';
import { listPage, recordPage, canWrite } from './tablepage.js';
import { itemPanel } from './disc.js';
import { isTester } from '../lib/nav.js';
import { itemDress, SHOW_BODY } from '../lib/look.js';
import { debounce } from '../lib/dom.js';

// how the item's look shows on a body (lib/look.js itemDress), kept up with
// the form's wear slot, model and colour
function dressing(meta, form) {
  const tints = meta.palettes?.tints || [];
  return {
    dress: (v) => { const d = itemDress(v, tints); return d && { ...d, body: SHOW_BODY }; },
    watch: (cb) => {
      if (!form) return;
      const read = () => {
        const v = { ...form.row };
        for (const n of ['equipslot', 'model', 'color']) { const el = form.inputs.get(n); if (el) v[n] = el.value; }
        return v;
      };
      const go = debounce(() => cb(read()), 200);
      for (const n of ['equipslot', 'model', 'color']) form.inputs.get(n)?.addEventListener('input', go);
    },
  };
}

// the columns item_reading reads (ops_items.go readingCols)
const READING = ['itemname', 'itemdesc', 'attacktype', 'unk5', 'equipslot', 'procanim', 'trade', 'rent', 'lore', 'craft', 'unk3', 'weapondamage'];

// readingCard is "In a fight": the record as the server reads it (op
// item_reading), asked again with the form's values as a GM types, so an edit
// of the description shows the proc it makes before it is saved.
function readingCard(row, form) {
  const body = h('div', { class: 'stack small' }, h('p', { class: 'faint' }, 'Reading…'));
  const values = () => {
    const v = {};
    for (const n of READING) { const el = form?.inputs.get(n); v[n] = el ? el.value : row[n]; }
    return v;
  };
  const line = (k, ...v) => h('div', null, h('strong', null, k + ': '), ...v);
  const load = async () => {
    try {
      const r = await op('item_reading', { item: row.patternid, values: values() });
      const at = r.attackType || {};
      mount(body,
        line('Attack type', at.name || '', at.word ? h('span', { class: 'muted' }, ` · the item window says ATT: ${at.word}`) : null),
        at.reads ? h('div', { class: 'muted' }, at.reads) : null,
        r.range !== undefined ? line('Range', String(r.range)) : null,
        line('Worn in', r.wornIn || ''),
        line('Item type', r.itemType?.name || '', r.itemType?.help ? h('span', { class: 'muted' }, ' · ' + r.itemType.help) : null),
        line('Its info shows', r.tags?.length ? r.tags.join(' ') : h('span', { class: 'faint' }, 'no tags')),
        r.proc
          ? h('div', { class: 'notice info' }, h('strong', null, 'Weapon proc: '), r.proc.text,
            r.proc.note ? h('div', { class: 'muted' }, r.proc.note) : null)
          : h('div', null, h('strong', null, 'Weapon proc: '), h('span', { class: 'faint' }, 'none'),
            r.procUnread ? h('div', { class: 'notice warn' }, r.procUnread) : null),
        r.worn ? h('div', { class: 'notice info' }, h('strong', null, 'When its wearer is hit: '), r.worn) : null,
        r.use
          ? h('div', null, h('strong', null, 'When used: '), r.use.name ? r.use.name + ': ' : '', r.use.line, ' ',
            h('a', { href: '#/tables/itemeffect/' + row.patternid }, 'its effect row'))
          : null,
        h('p', { class: 'faint' }, 'No column holds a proc: the server reads a weapon\'s from its Description ("Procs a 100 point DD Spell.", ' +
          '"Procs 400-600 fire, a 10% chance each hit."), an epic\'s by its name. This card reads the form as you type.'));
    } catch (e) { mount(body, h('p', { class: 'err' }, e.message)); }
  };
  const again = debounce(load, 300);
  for (const n of READING) { const el = form?.inputs.get(n); el?.addEventListener('input', again); el?.addEventListener('change', again); }
  load();
  return card('In a fight (as the server reads it)', body);
}

export async function render(box, [id]) {
  if (id === undefined) {
    return listPage(box, { table: 'itempattern', base: 'items', title: 'Items', sub: 'Every item that exists. Players\' copies take an edit at their next login; merchants at once.' });
  }
  return recordPage(box, {
    table: 'itempattern', keySeg: id, base: 'items',
    crumbs: h('a', { href: '#/items' }, 'Items'),
    title: (row) => row.itemname || 'Item ' + row.patternid,
    sub: (row) => `Item ${row.patternid}`,
    newDefaults: { itemcost: 0, unk1: 0, itemicon: 0, unk2: 0, equipslot: -1, unk3: 0, trade: 1, rent: 1, unk4: 0, attacktype: 0, weapondamage: 0, unk5: 0,
      levelreq: 1, maxstack: 1, maxhp: 0, duration: 0, classuse: 32767, raceuse: 1023, procanim: 0, lore: 0, unk6: 0, craft: 0, itemname: '', itemdesc: '', model: 0, color: 0 },
    actions: (row, meta) => [canWrite(meta) && !isTester() ? button('Give to a character', { icon: 'send', onclick: () => give(row) }) : null].filter(Boolean),
    side: async (row, meta, refresh, form) => {
      if (isTester()) return h('div', { class: 'stack' }, readingCard(row, form), await itemPanel(row, dressing(meta, form))); // who sells or holds it is not a tester's
      const out = await op('fetch', { id: 'item:' + row.patternid });
      const r = out.record;
      return h('div', { class: 'stack' },
        readingCard(row, form),
        await itemPanel(row, dressing(meta, form)),
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
