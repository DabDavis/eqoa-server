// What a GM does to a character, from the players list or its own page:
// message, teleport, level and money, heal, give, kick, ban.

import { h, formBox, confirmBox, toast } from '../lib/dom.js';
import { op } from '../lib/api.js';
import { pickRow } from '../lib/form.js';

const COLOURS = ['yellow', 'white', 'green', 'cyan', 'orange', 'red', 'pink'].map((c) => ({ value: c, label: c }));

async function run(name, args, okText) {
  try {
    const out = await op(name, args);
    toast(okText ? okText(out) : out.message || 'Done', 'ok', out.message && okText ? out.message : out.live || '');
    return out;
  } catch (e) {
    toast(e.message, 'error');
    return null;
  }
}

export async function messagePlayer(ch) {
  const v = await formBox({
    title: 'Message ' + ch.name, submitLabel: 'Send',
    fields: [{ name: 'text', label: 'Line (they see it as [GM] …)' }, { name: 'colour', label: 'Colour', type: 'select', options: COLOURS, value: 'yellow' }],
  });
  if (v && v.text.trim()) return run('message_player', { character: ch.serverid, text: v.text, colour: v.colour }, () => 'Sent to ' + ch.name);
  return null;
}

let destCache = null;
async function destinations() {
  if (!destCache) destCache = (await op('teleport_destinations')).destinations;
  return destCache;
}

export async function teleport(ch) {
  let dests = [];
  try { dests = await destinations(); } catch { /* coordinates still work */ }
  const seen = new Set();
  const options = [{ value: '', label: '— coordinates, an NPC or a player below —' }];
  for (const d of dests) {
    if (seen.has(d.name)) continue;
    seen.add(d.name);
    options.push({ value: d.name, label: `${d.label} (${d.name})` });
  }
  const v = await formBox({
    title: 'Teleport ' + ch.name, submitLabel: 'Teleport',
    intro: ch.online === false ? `${ch.name} is offline: this changes where their next login starts.` : `${ch.name} is moved where they stand, as a harbormaster's warp moves them.`,
    fields: [
      { name: 'place', label: 'A travel destination', type: 'select', options },
      { name: 'npc', label: 'or an NPC (name or id) or where they stand', placeholder: 'Coachman Ronks, guk' },
      { name: 'player', label: 'or an online character', placeholder: 'their name' },
      { name: 'x', label: 'or x', type: 'number' }, { name: 'y', label: 'y (height)', type: 'number' }, { name: 'z', label: 'z', type: 'number' },
      { name: 'world', label: 'world', type: 'number', value: ch.world ?? 0 },
    ],
  });
  if (!v) return null;
  const args = { character: ch.serverid };
  if (v.place) args.place = v.place;
  else if (v.npc.trim()) args.npc = /^\d+$/.test(v.npc.trim()) ? Number(v.npc.trim()) : v.npc.trim();
  else if (v.player.trim()) args.player = v.player.trim();
  else if (v.x !== null && v.y !== null && v.z !== null) Object.assign(args, { x: v.x, y: v.y, z: v.z, world: v.world ?? 0 });
  else { toast('Say where: a destination, an NPC, a player or coordinates', 'warn'); return null; }
  return run('teleport_character', args, (o) => o.message);
}

export async function setLevelMoney(ch) {
  const v = await formBox({
    title: 'Level and money of ' + ch.name, submitLabel: 'Set',
    intro: 'Saved, and applied where they stand if they are in the world.',
    fields: [
      { name: 'level', label: 'Level (1-60)', type: 'number', value: ch.level },
      { name: 'tunar', label: 'Tunar carried', type: 'number', value: ch.tunar ?? '' },
      { name: 'bank_tunar', label: 'Tunar in the bank', type: 'number', value: ch.bankTunar ?? '' },
    ],
  });
  if (!v) return null;
  const args = { character: ch.serverid };
  if (v.level !== null && v.level !== ch.level) args.level = v.level;
  if (v.tunar !== null && v.tunar !== ch.tunar) args.tunar = v.tunar;
  if (v.bank_tunar !== null && v.bank_tunar !== ch.bankTunar) args.bank_tunar = v.bank_tunar;
  if (Object.keys(args).length === 1) return null;
  return run('set_character', args, () => 'Set ' + ch.name);
}

export async function heal(ch) {
  return run('set_character', { character: ch.serverid, heal: true }, (o) => (o.healed ? ch.name + ' healed' : o.healNote));
}

export async function giveItem(ch) {
  const item = await pickRow('itempattern', 'an item to give');
  if (item === null) return null;
  const v = await formBox({
    title: 'Give ' + ch.name + ' item #' + item, submitLabel: 'Give',
    fields: [
      { name: 'count', label: 'How many', type: 'number', value: 1 },
      { name: 'location', label: 'Where', type: 'select', options: [{ value: 'inventory', label: 'Inventory' }, { value: 'bank', label: 'Bank' }], value: 'inventory' },
    ],
  });
  if (!v) return null;
  return run('give_item', { character: ch.serverid, item, count: v.count || 1, location: v.location }, (o) => o.message);
}

export async function grantSpell(ch, all = false) {
  let spell = 'all';
  if (!all) {
    spell = await pickRow('spellpattern', 'a spell to give');
    if (spell === null) return null;
  }
  return run('grant_spell', { character: ch.serverid, spell }, (o) => o.message);
}

// signLedger signs a stable's ledger for a character (charcoaches,
// handlers/coachledger.go): a coachman whose coach runs to the town offers it
// from their next talk. The towns are this world's coachmen's (map_layers).
export async function signLedger(ch) {
  let towns = [];
  try {
    const out = await op('map_layers', { world: 0 });
    towns = [...new Set(out.coaches.filter((c) => c.named).map((c) => c.town))].sort();
  } catch (e) {
    toast(e.message, 'error');
    return null;
  }
  if (!towns.length) {
    toast('No coachman of Tunaria serves a town', 'warn');
    return null;
  }
  const v = await formBox({
    title: 'Sign a ledger for ' + ch.name, submitLabel: 'Sign',
    intro: 'A coachman whose coach runs to the town offers it to them from their next talk. Delete the row to take it back.',
    fields: [{ name: 'town', label: 'Town', type: 'select', options: towns.map((t) => ({ value: t, label: t })) }],
  });
  if (!v) return null;
  const signed = new Date().toISOString().replace(/\.\d+Z$/, 'Z');
  return run('insert_row', { table: 'charcoaches', values: { serverid: ch.serverid, town: v.town, source: 'gm', signed } },
    () => ch.name + ' signed ' + v.town + "'s ledger");
}

export async function kick(ch) {
  const v = await formBox({
    title: 'Kick ' + ch.name, submitLabel: 'Kick',
    intro: 'They are ejected ("You have been ejected by a server administrator.") and saved as at a logout. They can log in again: ban the account to stop that.',
    fields: [{ name: 'reason', label: 'A line they see first (optional)' }],
  });
  if (!v) return null;
  return run('kick_player', { character: ch.serverid, reason: v.reason }, () => ch.name + ' kicked');
}

export async function ban(account, banned) {
  if (banned) {
    const v = await formBox({
      title: 'Ban ' + account.username, submitLabel: 'Ban',
      intro: 'Their game sessions end at once and their logins are refused until the ban is lifted.',
      fields: [{ name: 'reason', label: 'Reason (kept in the audit log)' }],
    });
    if (!v) return null;
    return run('ban_account', { account: account.accountid, banned: true, reason: v.reason }, () => account.username + ' banned');
  }
  if (!(await confirmBox({ title: 'Lift the ban on ' + account.username + '?', confirmLabel: 'Lift the ban' }))) return null;
  return run('ban_account', { account: account.accountid, banned: false }, () => account.username + ' may log in again');
}

export function playerActions(ch, after) {
  const b = (label, fn, cls = 'small') => {
    const el = h('button', { type: 'button', class: cls }, label);
    el.addEventListener('click', async (e) => {
      e.stopPropagation();
      el.disabled = true;
      try { const out = await fn(ch); if (out) after?.(out); } finally { el.disabled = false; }
    });
    return el;
  };
  return h('div', { class: 'actions' },
    b('Message', messagePlayer), b('Teleport', teleport), b('Level / money', setLevelMoney), b('Heal', heal),
    b('Give item', giveItem), b('Kick', kick, 'small danger'));
}
