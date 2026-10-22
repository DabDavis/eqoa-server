import { h, mount, pageHead, button, toast, card, confirmBox } from '../lib/dom.js';
import { op } from '../lib/api.js';
import { isAdmin, state } from '../lib/nav.js';
import { bytes, when } from '../lib/fmt.js';

export async function render(box) {
  const admin = isAdmin();
  const view = await op('get_settings');
  const live = view.live;

  // what NPCs say
  const rows = h('div', { class: 'stack' });
  const addRow = (name = '', text = '') => {
    const n = h('input', { type: 'text', value: name, placeholder: 'NPC name, exactly', disabled: !admin });
    const t = h('input', { type: 'text', value: text, placeholder: 'what it says when talked to', disabled: !admin });
    const del = button('', { icon: 'x', class: 'ghost icon', title: 'Remove', disabled: !admin, onclick: () => row.remove() });
    const row = h('div', { class: 'row' }, h('div', { class: 'grow' }, n), h('div', { class: 'grow' }, t), del);
    row.dataset.kind = 'npctext';
    rows.appendChild(row);
  };
  for (const [k, v] of Object.entries(live.npc_text || {}).sort()) addRow(k, v);

  const level = h('input', { type: 'number', min: '1', max: '60', value: live.start_level, disabled: !admin });
  const allSpells = h('input', { type: 'checkbox', checked: live.start_all_spells, disabled: !admin });
  const autoReg = h('input', { type: 'checkbox', checked: live.allow_auto_register, disabled: !admin });

  const save = button('Save settings', { class: 'primary', icon: 'save', disabled: !admin, onclick: async () => {
    const npc_text = {};
    for (const r of rows.querySelectorAll('[data-kind="npctext"]')) {
      const [n, t] = r.querySelectorAll('input');
      if (n.value.trim()) npc_text[n.value.trim()] = t.value;
    }
    try {
      const out = await op('update_settings', { npc_text, start_level: Number(level.value), start_all_spells: allSpells.checked, allow_auto_register: autoReg.checked });
      toast('Settings saved and applied', 'ok', out.configBackup ? 'config.yaml backed up to ' + out.configBackup : '');
    } catch (e) { toast(e.message, 'error'); }
  } });

  const backups = h('div');
  const loadBackups = async () => {
    if (!admin) return;
    try {
      const out = await op('list_backups');
      mount(backups, h('p', { class: 'muted small' }, 'Into ' + out.dir),
        out.backups.length ? h('div', { class: 'table-scroll' }, h('table', { class: 'plain' }, h('tbody', null, ...out.backups.slice(0, 15).map((b) => h('tr', null,
          h('td', { class: 'mono small' }, b.path), h('td', { class: 'num nowrap' }, bytes(b.bytes)), h('td', { class: 'muted nowrap' }, when(b.at)))))))
          : h('p', { class: 'faint' }, 'None yet.'));
    } catch (e) { mount(backups, h('p', { class: 'err' }, e.message)); }
  };

  mount(box,
    pageHead('Settings', admin ? 'These apply at once and are written to ' + (view.configFile || 'config.yaml') + ' (backed up first), so a restart keeps them.' : 'An admin changes these.'),
    h('div', { class: 'cols' },
      card('New characters', h('div', { class: 'stack' },
        h('label', null, 'Starting level (1-60)', level),
        h('label', { class: 'check' }, allSpells, 'Start with every spell of the class'),
        h('p', { class: 'muted small' }, 'A creation message never picks the level; these are for testing.'))),
      card('Accounts', h('div', { class: 'stack' },
        h('label', { class: 'check' }, autoReg, 'An unknown name registers itself at login'),
        h('p', { class: 'muted small' }, 'Off: only accounts that exist (made here, on the account page, or seeded in config) can log in.')))),
    card('What NPCs say', h('div', { class: 'stack' },
      h('p', { class: 'muted small' }, 'An NPC of this exact name answers a talk with these words instead of the debug box.'),
      rows, admin ? button('Add a line', { icon: 'plus', class: 'small', onclick: () => addRow() }) : null)),
    admin ? h('div', { class: 'row' }, save) : null,
    admin ? card('Database backups', backups, button('Back up now', { class: 'small primary', onclick: async () => {
      try { const out = await op('create_backup', { label: 'panel' }); toast('Backed up', 'ok', `${out.path} (${bytes(out.bytes)})`); loadBackups(); } catch (e) { toast(e.message, 'error'); }
    } })) : null,
    admin && state.session.server.restart ? card('Restart', h('div', { class: 'stack' },
      h('p', null, 'Saves every player as at a logout and starts the server again from its binary on disk. Everyone in the game is disconnected for about ten seconds.'),
      button('Restart the server', { class: 'danger', onclick: async () => {
        if (!(await confirmBox({ title: 'Restart the server?', danger: true, confirmLabel: 'Restart', typeToConfirm: 'restart', message: 'Every player is disconnected and saved; the panel comes back after it.' }))) return;
        try { await op('restart_server', { confirm: 'restart' }); toast('Restarting…', 'warn'); setTimeout(() => location.reload(), 8000); } catch (e) { toast(e.message, 'error'); }
      } }))) : null,
    card('The rest of the configuration (read-only)', h('pre', { class: 'json' }, JSON.stringify(view.config, null, 2))));
  loadBackups();
}
