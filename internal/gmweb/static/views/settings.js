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
  // the sun on vampires (entity/vampire.go)
  const sunDamage = h('input', { type: 'number', min: '0', max: '100000', value: live.vampire_sun_damage, disabled: !admin });
  const sunInterval = h('input', { type: 'number', min: '1', max: '3600', value: live.vampire_sun_interval, disabled: !admin });
  const sunShade = h('input', { type: 'checkbox', checked: live.vampire_sun_shade, disabled: !admin });
  const sunGMs = h('input', { type: 'checkbox', checked: live.vampire_sun_gms, disabled: !admin });

  const save = button('Save settings', { class: 'primary', icon: 'save', disabled: !admin, onclick: async () => {
    const npc_text = {};
    for (const r of rows.querySelectorAll('[data-kind="npctext"]')) {
      const [n, t] = r.querySelectorAll('input');
      if (n.value.trim()) npc_text[n.value.trim()] = t.value;
    }
    try {
      const out = await op('update_settings', { npc_text, start_level: Number(level.value), start_all_spells: allSpells.checked, allow_auto_register: autoReg.checked,
        vampire_sun_damage: Number(sunDamage.value), vampire_sun_interval: Number(sunInterval.value), vampire_sun_shade: sunShade.checked, vampire_sun_gms: sunGMs.checked });
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

  // a copy of the database for a GM's own machine (the real-data tests),
  // every credential and person's data taken out: download_snapshot, the
  // same operation an agent calls
  const snapshotOut = h('div', { class: 'stack' });
  const snapshotButton = button('Make one and download it', { class: 'small primary', icon: 'db', onclick: async () => {
    snapshotButton.disabled = true;
    try {
      const out = await op('download_snapshot');
      const a = h('a', { href: out.url, download: out.file });
      document.body.appendChild(a);
      a.click();
      a.remove();
      mount(snapshotOut,
        h('p', { class: 'small' }, `${out.file}: ${bytes(out.bytes)} gzipped, ${bytes(out.databaseBytes)} as a database, ${out.tables} tables (sanitiser ${out.sanitiser}).`),
        h('p', { class: 'mono small' }, 'sha256 ' + out.sha256),
        h('ul', { class: 'small muted' }, ...(out.removed || []).map((r) => h('li', null, h('span', { class: 'mono' }, r.table), ': ', r.did))));
      toast('Snapshot downloading', 'ok', 'Its link works once; the file is the server\'s content: keep it out of the repository.');
    } catch (e) { toast(e.message, 'error'); }
    snapshotButton.disabled = false;
  } });
  const snapshot = h('div', { class: 'stack' },
    h('p', { class: 'muted small' }, 'A copy of the live database with every credential and person\'s data taken out (password hashes, tokens, authenticators, OAuth clients, the audit log, the work board, e-mail and IP addresses; players\' login names become account-<id>) and the game whole: for the real-data tests on your own machine (EQOA_REAL_DB). Its link works once, for ten minutes. Never commit, attach or share the file.'),
    snapshotButton, snapshotOut);

  mount(box,
    pageHead('Settings', admin ? 'These apply at once and are written to ' + (view.configFile || 'config.yaml') + ' (backed up first), so a restart keeps them.' : 'An admin changes these.'),
    h('div', { class: 'cols' },
      card('New characters', h('div', { class: 'stack' },
        h('label', null, 'Starting level (1-60)', level),
        h('label', { class: 'check' }, allSpells, 'Start with every spell of the class'),
        h('p', { class: 'muted small' }, 'A creation message never picks the level; these are for testing.'))),
      card('Accounts', h('div', { class: 'stack' },
        h('label', { class: 'check' }, autoReg, 'An unknown name registers itself at login'),
        h('p', { class: 'muted small' }, 'Off: only accounts that exist (made here, on the account page, or seeded in config) can log in.'))),
      card('Vampires in the sun', h('div', { class: 'stack' },
        h('label', null, 'Damage a tick (0: none)', sunDamage),
        h('label', null, 'Seconds between ticks', sunInterval),
        h('label', { class: 'check' }, sunShade, 'Shade shelters a vampire: anything over it (a roof, rock, a big tree\'s crown)'),
        h('label', { class: 'check' }, sunGMs, 'A GM\'s vampire burns too'),
        h('p', { class: 'muted small' }, 'A vampire (the Vampires table, #vampire in the game) takes this while its client shows the sun up, 6:00 to 18:00; it can die of it.')))),
    card('What NPCs say', h('div', { class: 'stack' },
      h('p', { class: 'muted small' }, 'An NPC of this exact name answers a talk with these words instead of the debug box.'),
      rows, admin ? button('Add a line', { icon: 'plus', class: 'small', onclick: () => addRow() }) : null)),
    admin ? h('div', { class: 'row' }, save) : null,
    admin ? h('div', { class: 'cols' },
      card('Database backups', backups, button('Back up now', { class: 'small primary', onclick: async () => {
        try { const out = await op('create_backup', { label: 'panel' }); toast('Backed up', 'ok', `${out.path} (${bytes(out.bytes)})`); loadBackups(); } catch (e) { toast(e.message, 'error'); }
      } })),
      card('Sanitised snapshot', snapshot)) : null,
    admin && state.session.server.restart ? card('Restart', h('div', { class: 'stack' },
      h('p', null, 'Saves every player as at a logout and starts the server again from its binary on disk. Everyone in the game is disconnected for about ten seconds.'),
      button('Restart the server', { class: 'danger', onclick: async () => {
        if (!(await confirmBox({ title: 'Restart the server?', danger: true, confirmLabel: 'Restart', typeToConfirm: 'restart', message: 'Every player is disconnected and saved; the panel comes back after it.' }))) return;
        try { await op('restart_server', { confirm: 'restart' }); toast('Restarting…', 'warn'); setTimeout(() => location.reload(), 8000); } catch (e) { toast(e.message, 'error'); }
      } }))) : null,
    card('The rest of the configuration (read-only)', h('pre', { class: 'json' }, JSON.stringify(view.config, null, 2))));
  loadBackups();
}
