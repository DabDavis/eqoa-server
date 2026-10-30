import { h, mount, card, button, toast, empty, confirmBox } from '../lib/dom.js';
import { op } from '../lib/api.js';
import { listPage, recordPage, relatedGrid } from './tablepage.js';
import { playerActions, grantSpell, giveItem, teleport, setLevelMoney, signLedger } from './actions.js';

export async function render(box, [id]) {
  if (id === undefined) {
    return listPage(box, {
      table: 'characters', base: 'characters', title: 'Characters', allowNew: false,
      sub: 'Player characters. Open one for its items, spells and live state.',
    });
  }
  return recordPage(box, {
    table: 'characters', keySeg: id, base: 'characters',
    crumbs: h('a', { href: '#/characters' }, 'Characters'),
    title: (row) => row.charName,
    sub: (row) => `Character ${row.serverid} · account ${row.accountid}`,
    actions: (row) => [
      button('Delete character', { icon: 'trash', class: 'danger', onclick: async () => {
        try {
          const dry = await op('delete_row', { table: 'characters', key: { serverid: row.serverid }, dry_run: true });
          const also = Object.entries(dry.alsoDeleted || {}).map(([t, n]) => `${n} ${t}`).join(', ');
          if (!(await confirmBox({ title: 'Delete ' + row.charName + '?', danger: true, confirmLabel: 'Delete', typeToConfirm: row.charName,
            message: `The character and everything that is its (${also || 'nothing else'}) are deleted. Each row stays in the audit log and can be reverted.` }))) return;
          await op('delete_row', { table: 'characters', key: { serverid: row.serverid } });
          toast(row.charName + ' deleted', 'ok');
          location.hash = '#/characters';
        } catch (e) { toast(e.message, 'error'); }
      } }),
    ],
    side: async (row, meta, refresh) => {
      const out = await op('fetch', { id: 'character:' + row.serverid });
      const r = out.record;
      const on = r.online;
      const ch = { serverid: row.serverid, name: row.charName, level: row.level, tunar: row.tunar, bankTunar: row.bankTunar, world: row.world, online: !!on };
      return h('div', { class: 'stack' },
        card(on ? 'In the world now' : 'Offline', on
          ? h('div', { class: 'stack' },
            h('dl', { class: 'kv' },
              h('dt', null, 'Where'), h('dd', null, `${r.worldName}: ${on.x.toFixed(1)}, ${on.y.toFixed(1)}, ${on.z.toFixed(1)}`),
              h('dt', null, 'Health'), h('dd', null, on.dead ? 'dead' : `${on.hp} / ${on.maxHP} HP, ${on.power} / ${on.maxPower} power`),
              h('dt', null, 'Level'), h('dd', null, String(on.level)),
              h('dt', null, 'Tunar'), h('dd', null, on.tunar.toLocaleString()),
              on.fighting ? [h('dt', null, 'Fighting'), h('dd', null, on.fighting)] : null),
            playerActions(ch, refresh))
          : h('div', { class: 'stack' },
            h('p', { class: 'muted' }, `Last saved in ${r.worldName} at ${Number(row.x).toFixed(0)}, ${Number(row.y).toFixed(0)}, ${Number(row.z).toFixed(0)}. Changes reach them at their next login.`),
            h('div', { class: 'actions' },
              button('Teleport', { class: 'small', onclick: async () => { if (await teleport(ch)) location.reload(); } }),
              button('Level / money', { class: 'small', onclick: async () => { if (await setLevelMoney(ch)) location.reload(); } }),
              button('Give item', { class: 'small', onclick: async () => { if (await giveItem(ch)) refresh(); } })))),
        card('Account', r.account ? h('div', null, h('a', { href: '#/accounts/' + r.account.accountid }, r.account.username),
          r.account.acctstatus < 0 ? h('span', { class: 'badge danger' }, 'banned') : null) : empty('No account')));
    },
    below: async (row) => {
      const items = await relatedGrid({
        table: 'charinventory', filters: [{ column: 'serverid', op: '=', value: row.serverid }], base: 'tables/charinventory',
        columns: ['itemid', 'patternid', 'stackleft', 'remainHP', 'equiploc', 'location', 'listnumber'], emptyText: 'No items.',
        tools: [button('Give item', { class: 'small primary', icon: 'plus', onclick: async () => { if (await giveItem({ serverid: row.serverid, name: row.charName })) items.load(); } })],
      });
      const spells = await relatedGrid({
        table: 'spells', filters: [{ column: 'serverid', op: '=', value: row.serverid }], base: 'tables/spells',
        columns: ['id', 'spellid', 'addedorder', 'onHotBar', 'whereonBar'], emptyText: 'No spells.',
        tools: [
          button('Give spell', { class: 'small primary', icon: 'plus', onclick: async () => { if (await grantSpell({ serverid: row.serverid, name: row.charName })) spells.load(); } }),
          button('All class spells', { class: 'small', onclick: async () => { if (await grantSpell({ serverid: row.serverid, name: row.charName }, true)) spells.load(); } }),
        ],
      });
      const effects = await relatedGrid({
        table: 'charbuffs', filters: [{ column: 'serverid', op: '=', value: row.serverid }], base: 'tables/charbuffs',
        columns: ['seq', 'spellid', 'name', 'left_ms'], emptyText: 'No effects kept.',
      });
      const ledgers = await relatedGrid({
        table: 'charcoaches', filters: [{ column: 'serverid', op: '=', value: row.serverid }], base: 'tables/charcoaches',
        columns: ['town', 'source', 'signed'], emptyText: 'No stable ledger signed: coachmen offer them no town.',
        tools: [button('Sign a ledger', { class: 'small primary', icon: 'plus', onclick: async () => { if (await signLedger({ serverid: row.serverid, name: row.charName })) ledgers.load(); } })],
      });
      return [h('h2', null, 'Items'), items.el, h('h2', null, 'Spell book'), spells.el, h('h2', null, 'Effects kept over a logout'), effects.el,
        h('h2', null, 'Coach ledgers signed'), ledgers.el];
    },
  });
}

export { mount };
