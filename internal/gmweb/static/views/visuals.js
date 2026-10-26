// Spell visuals: every visual on the GM's own disc (SPELLFX.CSF, read by the
// disc worker: nothing of it is sent anywhere but its DictIDs), what each
// does, the spells that use it, and its playback: the preview (gm.preview_url,
// eqoa-rust-client-vanilla's web/fxview.html, framed) plays it here, a human
// casting it as the game plays a cast's 0x00B1, off the same disc; Play in
// game casts it from the GM's own character in the game onto its target.
// #/visuals/<spellid> picks one for that spell.

import { h, mount, pageHead, button, toast, card, empty, formBox, debounce } from '../lib/dom.js';
import { op } from '../lib/api.js';
import { go, state } from '../lib/nav.js';
import { spellVisualsOnDisc, previewFiles } from './disc.js';

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

// The preview's messages, to the pane on the page now (one listener for all
// visits of the page: each visit's pane replaces the last).
let pane = null;
addEventListener('message', (e) => {
  if (pane && e.source === pane.frame.contentWindow && e.origin === pane.origin) pane.onMessage(e.data || {});
});

// The preview's training dummy: the body every training dummy wears (an npcs
// row's modelid) at the Training Dummy rows' size.
const DUMMY = { model: '0x66262F1C', size: 0.25 };

// previewPane is the framed preview: a default human casting a visual as a
// cast's 0x00B1 plays it, at a training dummy (a harmful spell's), a second
// human, or itself (anything else's). Null with no gm.preview_url.
function previewPane() {
  const url = state.session?.preview;
  if (!url) return null;
  let origin;
  try { origin = new URL(url).origin; } catch { return null; }
  // the preview takes this panel's origin from its own list, or from here
  // when it is on this machine (a test's)
  const src = url + (url.includes('?') ? '&' : '?') + 'panel=' + encodeURIComponent(location.origin);
  const frame = h('iframe', { src, class: 'fx-preview', sandbox: 'allow-scripts allow-same-origin', title: 'Spell visual preview', referrerpolicy: 'no-referrer' });
  const status = h('div', { class: 'faint small' }, 'Loading the preview…');
  const aim = h('select', { 'aria-label': 'What the cast lands on' },
    h('option', { value: 'auto' }, 'By the spell (a harmful one at the dummy, else the caster)'),
    h('option', { value: 'dummy' }, 'A training dummy'),
    h('option', { value: 'human' }, 'A second human'),
    h('option', { value: 'caster' }, 'The caster'));
  let started = false;
  let wanted = null; // {effect, castMs, spell}
  const aimOf = (spell) => (aim.value !== 'auto' ? aim.value : spell ? (spell.harmful ? 'dummy' : 'caster') : 'dummy');
  const post = (m) => frame.contentWindow && frame.contentWindow.postMessage(m, origin);
  const playMsg = () => ({ effect: wanted.effect, castMs: wanted.castMs, aim: aimOf(wanted.spell) });
  const p = {
    frame, origin,
    async onMessage(m) {
      if (m.source !== 'eqoa-fxview') return;
      if (m.type === 'ready') {
        let got;
        try { got = await previewFiles(m.files || []); } catch (err) { status.textContent = err.message; return; }
        if (got.missing.some((n) => n !== 'ITEM.ESF')) {
          status.textContent = 'Load your disc on the Game disc page (once more): the preview needs ' + got.missing.join(', ') + ', kept from then on.';
          return;
        }
        post({ type: 'fxview-files', files: got.files, play: wanted ? playMsg() : { aim: aimOf(null) }, dummy: DUMMY });
      } else if (m.type === 'started') {
        started = true;
      } else if (m.type === 'status' || m.type === 'error') {
        status.textContent = m.text || '';
      }
    },
    // plays a visual (hex DictID) cast for castMs, for spell when it is a
    // spell's (what it lands on follows the spell unless chosen above)
    play(effect, castMs, spell) {
      wanted = { effect, castMs: Math.max(0, Math.round(castMs)), spell: spell || null };
      if (started) post({ type: 'fxview-play', ...playMsg() });
    },
  };
  aim.addEventListener('change', () => { if (wanted && started) post({ type: 'fxview-play', ...playMsg() }); });
  p.card = card('Preview', h('div', { class: 'stack' },
    frame,
    h('div', { class: 'row' }, aim,
      button('Again', { icon: 'refresh', class: 'small', title: 'Cast it again now', onclick: () => started && post({ type: 'fxview-again' }) })),
    status,
    h('p', { class: 'muted small fx-help' }, 'A human casts the visual as the game plays a cast (0x00B1), off your own disc, again and again: Preview on a visual or a spell plays it here. Drag to turn round, wheel to zoom; nothing in the game changes.')));
  pane = p;
  return p;
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

  const preview = previewPane();
  const previewVisual = (v) => preview && preview.play(hex(v.id), (Number(secs.value) || 0) * 1000, null);

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
          preview ? button('Preview', { class: 'small', title: 'Play it in the preview', onclick: () => (spell ? preview.play(hex(v.id), spell.castSeconds * 1000, spell) : previewVisual(v)) }) : null,
          preview ? ' ' : null,
          button(preview ? 'In game' : 'Play', { icon: 'play', class: 'small', title: 'Play it in the game, from your character onto its target', onclick: () => playInGame(spell ? { visual: hex(v.id), spell: spell.spellid } : { visual: hex(v.id), seconds: Number(secs.value) || 0 }) }),
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
      preview ? button('Preview its visual', { onclick: () => preview.play(spell.plays, spell.castSeconds * 1000, spell) }) : null,
      button('Play its visual in game', { icon: 'play', onclick: () => playInGame({ spell: spell.spellid }) }),
      spell.own ? button('Back to its stand-in', { class: 'ghost', onclick: () => useFor({ id: 0 }, spell.spellid) }) : null),
    h('p', { class: 'muted small' }, 'Play a visual on the list to see it in the game, then Use to give it to ' + spell.name + '.'))) : null;

  const cards = [
    forCard,
    card('Play in game', h('div', { class: 'stack' },
      h('p', { class: 'muted small' }, 'Log in to the game with this account\'s character and target something (nothing targeted: yourself). A visual plays from you onto your target, seen by everyone near; nothing is saved and nothing lands. The cast time is how long its casting part runs.'),
      h('div', { class: 'row' }, fx, h('span', { class: 'row' }, 'cast', secs, 's'), playTyped))),
    card('On your disc', h('div', { class: 'stack' }, h('div', { class: 'grid-tools' }, search, h('span', { class: 'grow' }), countLine), listBox)),
  ];
  mount(box,
    pageHead('Spell visuals', 'Every visual on your disc: what a spell shows when cast (0x00B1\'s DictID). Play one in the game to see it, then give it to a spell.',
      spell ? h('a', { href: '#/spells/' + spell.spellid }, spell.name) : null),
    // with a preview, the list beside it and the preview held in view while the list scrolls, so a
    // Preview pressed far down the list plays where it can be seen (the user, F3 pass 17)
    preview ? h('div', { class: 'fx-layout' }, h('div', { class: 'fx-main' }, ...cards), h('div', { class: 'fx-side' }, preview.card)) : null,
    ...(preview ? [] : cards));
  draw();
  // a spell's own visual plays in the preview at once
  if (preview && spell) preview.play(spell.plays, spell.castSeconds * 1000, spell);
}
