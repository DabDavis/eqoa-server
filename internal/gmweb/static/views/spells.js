import { h, card, button, toast, mount } from '../lib/dom.js';
import { op } from '../lib/api.js';
import { describe } from '../lib/meta.js';
import { listPage, recordPage, canWrite } from './tablepage.js';
import { go } from '../lib/nav.js';
import { playInGame } from './visuals.js';

export async function render(box, [id]) {
  if (id === undefined) {
    return listPage(box, { table: 'spellpattern', base: 'spells', title: 'Spells', sub: 'Every spell and ability. What one does is its effect line, on its page.' });
  }
  return recordPage(box, {
    table: 'spellpattern', keySeg: id, base: 'spells',
    crumbs: h('a', { href: '#/spells' }, 'Spells'),
    title: (row) => row.spellname || 'Spell ' + row.spellid,
    sub: (row) => `Spell ${row.spellid} · level ${row.abilitylvl}`,
    newDefaults: { abilitylvl: 1, unk2: 0, unk3: 0, range: 30, casttime: 2, power: 5, iconColor: 0, icon: 0, scope: 0, recast: 6, eqprequire: 0, spellname: '', spelldesc: '' },
    side: async (row) => {
      const out = await op('fetch', { id: 'spell:' + row.spellid });
      const r = out.record;
      const effMeta = await describe('spelleffect');
      const writable = canWrite(effMeta);
      const line = h('textarea', { class: 'code', disabled: !writable, spellcheck: 'false' });
      line.value = r.effect?.line ?? '';
      const fx = h('input', { type: 'text', class: 'mono', value: r.effect ? '0x' + (Number(r.effect.fxdict) >>> 0).toString(16).toUpperCase().padStart(8, '0') : '0x00000000', disabled: !writable });
      const save = button(r.effect ? 'Save the effect' : 'Give it an effect', { class: 'primary', onclick: async () => {
        try {
          const values = { line: line.value, fxdict: fx.value.trim() || '0' };
          const out2 = r.effect
            ? await op('update_row', { table: 'spelleffect', key: { spellid: row.spellid }, values })
            : await op('insert_row', { table: 'spelleffect', values: { spellid: row.spellid, source: 'gm', ...values } });
          toast('Effect saved', 'ok', out2.live || 'read at its next cast');
          r.effect = r.effect || { line: line.value };
        } catch (e) { toast(e.message, 'error'); }
      } });
      // the visual: played in the game (what is typed, else what the spell
      // plays), or chosen on the disc's list
      const play = button('Play in game', { icon: 'play', title: 'From your character in the game onto its target', onclick: () => {
        const typed = fx.value.trim();
        return playInGame(/^(0x)?0*$/i.test(typed) ? { spell: row.spellid } : { visual: typed, spell: row.spellid });
      } });
      const pick = button('Choose on the disc…', { icon: 'disc', onclick: () => go('visuals/' + row.spellid) });
      const classes = new Set(r.classes || []);
      const csMeta = await describe('classspells');
      const classBox = h('div', { class: 'flagset' }, ...(csMeta.enums.class || []).map((c) => {
        const box = h('input', { type: 'checkbox', checked: classes.has(c.label), disabled: !canWrite(csMeta) });
        box.addEventListener('change', async () => {
          try {
            if (box.checked) await op('insert_row', { table: 'classspells', values: { class: c.value, spellid: row.spellid, source: 'gm' } });
            else await op('delete_row', { table: 'classspells', key: { class: c.value, spellid: row.spellid } });
            toast(`${c.label}: ${box.checked ? 'has' : 'does not have'} it`, 'ok');
          } catch (e) { box.checked = !box.checked; toast(e.message, 'error'); }
        });
        return h('label', null, box, c.label);
      }));
      return h('div', { class: 'stack' },
        card('What it does', h('div', { class: 'stack' },
          h('p', { class: 'muted small' }, 'The server reads these words at every cast: a target (Target, Self, Group), then clauses: "Decrease Hitpoints by 45", "Increase STR by 10", "Increase HP per tic by 5", "Root", "Snare", "Sow", "Invis", "Return to bind point"; a scale "; Increase 10% based on INT"; a duration "30sec", "5min". The guide has the rest.'),
          line, h('label', null, 'Visual (CastingCombat DictID; 0: the server\'s stand-in)', fx),
          h('div', { class: 'row' }, writable ? save : null, play, pick))),
        card('Classes that have it', h('div', { class: 'stack' }, h('p', { class: 'muted small' }, 'Given to a new character of the class when start_all_spells is on, and by "All class spells".'), classBox)),
        card('Known', h('p', null, `${r.knownByCharacters} character${r.knownByCharacters === 1 ? '' : 's'} know it.`)));
    },
  });
}

export { mount };
