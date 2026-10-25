// Spell visuals: every visual on the GM's own disc (SPELLFX.CSF, read by the
// disc worker: nothing of it is sent anywhere but its DictIDs), what each
// does, the spells that use it, and its playback: Play in game casts it from
// the GM's own character in the game onto its target, as a spell's cast
// would. #/visuals/<spellid> picks one for that spell.

import { h, mount, pageHead, button, toast, card, empty, formBox, debounce } from '../lib/dom.js';
import { op } from '../lib/api.js';
import { go } from '../lib/nav.js';
import { spellVisualsOnDisc } from './disc.js';

const hex = (n) => '0x' + (Number(n) >>> 0).toString(16).toUpperCase().padStart(8, '0');

// what an event list holds, in a few words: "action, 2 particles, sound"
function listText(l) {
  if (!l || !l.events) return '—';
  const order = ['action', 'attack', 'particles', 'projectile', 'instant', 'lightning', 'sprite', 'sound', 'shake', 'vibrate', 'fade'];
  const kinds = Object.entries(l.kinds).filter(([k]) => !/end$|^wait$|^motif$|^attractor$/.test(k));
  kinds.sort((a, b) => (order.indexOf(a[0]) + 99) % 99 - (order.indexOf(b[0]) + 99) % 99);
  const words = kinds.map(([k, n]) => (n > 1 ? `${n} ${k}` : k));
  return words.length ? words.join(', ') : `${l.events} event${l.events === 1 ? '' : 's'}`;
}

// play plays a visual (or a spell's) in the game, said in a toast
export async function playInGame(args) {
  try {
    const out = await op('play_visual', args);
    toast('Playing ' + out.visual + ' on ' + out.on, 'ok', out.spell || `a ${out.seconds.toFixed(1)} s cast, in the game`);
    return true;
  } catch (e) {
    toast(e.message, 'error');
    return false;
  }
}

export async function render(box, [spellSeg]) {
  const forSpell = spellSeg !== undefined && spellSeg !== '' ? Number(decodeURIComponent(spellSeg)) : null;
  mount(box, pageHead('Spell visuals', 'Loading…'));
  let spell = null;
  if (forSpell !== null) {
    try { spell = await op('spell_visual', { spell: forSpell }); } catch (e) { toast(e.message, 'error'); }
  }
  let visuals;
  try {
    visuals = await spellVisualsOnDisc();
  } catch (e) {
    mount(box, pageHead('Spell visuals', 'The visuals are read off your own game disc.'),
      card('No disc yet', h('div', { class: 'stack' },
        h('p', null, 'Load your disc on the Game disc page first (once: SPELLFX.CSF is kept in this browser after that). ' + e.message),
        button('Game disc', { icon: 'disc', class: 'primary', onclick: () => go('disc') }))));
    return;
  }
  let uses = { used: [], standIns: [] };
  const loadUses = async () => {
    try { uses = await op('visual_uses'); } catch (e) { toast(e.message, 'error'); }
  };
  await loadUses();

  // the typed playback: any DictID, with a cast time
  const fx = h('input', { type: 'text', class: 'mono', placeholder: '0x1DAEB5B6', spellcheck: 'false', autocomplete: 'off', 'aria-label': 'Visual DictID' });
  const secs = h('input', { type: 'text', inputmode: 'decimal', value: '2', size: '4', 'aria-label': 'Cast time in seconds' });
  const playTyped = button('Play in game', { icon: 'play', class: 'primary', onclick: () => {
    const v = fx.value.trim();
    if (!v) { toast('Type a visual\'s DictID, as 0x1DAEB5B6', 'warn'); return; }
    return playInGame({ visual: v, seconds: Number(secs.value) || 0 });
  } });
  fx.addEventListener('keydown', (e) => { if (e.key === 'Enter') playTyped.click(); });

  const search = h('input', { type: 'search', placeholder: 'Search by DictID, a spell using it, or what it does (projectile, sound…)', 'aria-label': 'Search visuals' });
  const listBox = h('div', { class: 'grid-scroll' });
  const countLine = h('span', { class: 'faint small' });

  const usedBy = (id) => {
    const key = hex(id);
    const u = uses.used.find((x) => x.visual === key);
    const s = uses.standIns.find((x) => x.visual === key);
    return { spells: u ? u.spells : [], standIn: s };
  };

  const useFor = async (v, sp) => {
    let target = sp;
    if (!target) {
      const got = await formBox({
        title: 'Give ' + hex(v.id) + ' to a spell',
        intro: 'The spell plays this visual from its next cast. The change is in the audit log and can be reverted there.',
        fields: [{ name: 'spell', label: 'Spell (its id, or its exact name)', placeholder: '333 or Ward Death' }],
        submitLabel: 'Give it',
      });
      if (!got || !got.spell.trim()) return;
      target = /^\d+$/.test(got.spell.trim()) ? Number(got.spell.trim()) : got.spell.trim();
    }
    try {
      const out = await op('set_spell_visual', { spell: target, visual: hex(v.id) });
      toast(out.visual.name + ' now plays ' + hex(v.id), 'ok', out.change?.action === 'unchanged' ? 'it had it already' : 'from its next cast');
      if (spell && out.visual.spellid === spell.spellid) spell = out.visual;
      await loadUses();
      draw();
    } catch (e) { toast(e.message, 'error'); }
  };

  const draw = () => {
    const q = search.value.trim().toLowerCase();
    const rows = visuals.filter((v) => {
      if (!q) return true;
      const u = usedBy(v.id);
      const text = [hex(v.id), ...v.lists.map(listText), ...u.spells.map((s) => s.name), u.standIn ? u.standIn.kind + ' stand-in' : ''].join(' ').toLowerCase();
      return text.includes(q) || hex(v.id).toLowerCase().includes(q.replace(/^0x/, ''));
    });
    countLine.textContent = `${rows.length} of ${visuals.length} visuals`;
    if (!rows.length) { mount(listBox, empty('No visual matches.')); return; }
    const body = h('tbody');
    for (const v of rows) {
      const u = usedBy(v.id);
      const current = spell && hex(v.id) === spell.plays;
      const used = h('td', { class: 'small' },
        u.standIn ? h('span', { class: 'badge info', title: 'played by spells with no visual of their own' }, `${u.standIn.kind} stand-in · ${u.standIn.spells}`) : null,
        ...u.spells.slice(0, 4).map((s) => h('a', { href: '#/spells/' + s.spellid, class: 'chip' }, s.name)),
        u.spells.length > 4 ? h('span', { class: 'faint' }, ` +${u.spells.length - 4}`) : null);
      body.appendChild(h('tr', { class: current ? 'sel' : '' },
        h('td', { class: 'mono' }, hex(v.id)),
        h('td', { class: 'nowrap' },
          button('Play', { icon: 'play', class: 'small', title: 'Play it in the game, from your character onto its target', onclick: () => playInGame(spell ? { visual: hex(v.id), spell: spell.spellid } : { visual: hex(v.id), seconds: Number(secs.value) || 0 }) }),
          ' ',
          spell
            ? button(current ? 'Its visual' : 'Use', { class: 'small' + (current ? '' : ' primary'), disabled: current, title: 'Give it to ' + spell.name, onclick: () => useFor(v, spell.spellid) })
            : button('Use for…', { class: 'small', title: 'Give it to a spell', onclick: () => useFor(v, null) })),
        used,
        h('td', { title: 'the caster\'s events' }, listText(v.lists[0])),
        h('td', { title: 'the target\'s events' }, listText(v.lists[1])),
        h('td', { title: 'the area\'s events' }, listText(v.lists[2]))));
    }
    mount(listBox, h('table', { class: 'grid' },
      h('thead', null, h('tr', null, ...['Visual', '', 'Used by', 'Caster', 'Target', 'Area'].map((t) => h('th', { scope: 'col' }, t)))),
      body));
  };
  search.addEventListener('input', debounce(draw, 150));

  const forCard = spell ? card('Choosing for ' + spell.name, h('div', { class: 'stack' },
    h('p', null, spell.summary + '. ', h('a', { href: '#/spells/' + spell.spellid }, 'Back to the spell')),
    h('div', { class: 'row' },
      button('Play its visual', { icon: 'play', onclick: () => playInGame({ spell: spell.spellid }) }),
      spell.own ? button('Back to its stand-in', { class: 'ghost', onclick: () => useFor({ id: 0 }, spell.spellid) }) : null),
    h('p', { class: 'muted small' }, 'Play a visual on the list to see it in the game, then Use to give it to ' + spell.name + '.'))) : null;

  mount(box,
    pageHead('Spell visuals', 'Every visual on your disc: what a spell shows when cast (0x00B1\'s DictID). Play one in the game to see it, then give it to a spell.',
      spell ? h('a', { href: '#/spells/' + spell.spellid }, spell.name) : null),
    forCard,
    card('Play in game', h('div', { class: 'stack' },
      h('p', { class: 'muted small' }, 'Log in to the game with this account\'s character and target something (nothing targeted: yourself). A visual plays from you onto your target, seen by everyone near; nothing is saved and nothing lands. The cast time is how long its casting part runs.'),
      h('div', { class: 'row' }, fx, h('span', { class: 'row' }, 'cast', secs, 's'), playTyped))),
    card('On your disc', h('div', { class: 'stack' }, h('div', { class: 'grid-tools' }, search, h('span', { class: 'grow' }), countLine), listBox)));
  draw();
}
