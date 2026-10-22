import { h, mount, pageHead, button, toast, debounce } from '../lib/dom.js';
import { op } from '../lib/api.js';
import { bytes } from '../lib/fmt.js';

export async function render(box) {
  const filter = h('input', { type: 'search', placeholder: 'only lines containing… (GM, Auth, error, a name)' });
  const lines = h('select', null, ...[100, 200, 500, 1000, 2000].map((n) => h('option', { value: n, selected: n === 200 }, n + ' lines')));
  const auto = h('input', { type: 'checkbox' });
  const pre = h('pre', { class: 'log' });
  const info = h('span', { class: 'muted small' });
  mount(box, pageHead('Server log', 'The end of the game server\'s own log.'),
    h('div', { class: 'card' },
      h('div', { class: 'grid-tools' }, filter, lines, h('label', { class: 'check' }, auto, 'Follow'), h('span', { class: 'grow' }), info,
        button('', { icon: 'refresh', class: 'small ghost', title: 'Reload', onclick: () => load() })),
      pre));
  async function load() {
    try {
      const out = await op('server_log', { lines: Number(lines.value), filter: filter.value.trim() });
      const atEnd = pre.scrollTop + pre.clientHeight >= pre.scrollHeight - 30;
      pre.textContent = out.lines.join('\n');
      info.textContent = `${out.file} · ${bytes(out.bytes)}`;
      if (atEnd || !pre.dataset.loaded) pre.scrollTop = pre.scrollHeight;
      pre.dataset.loaded = '1';
    } catch (e) { toast(e.message, 'error'); }
  }
  filter.addEventListener('input', debounce(load, 300));
  lines.addEventListener('change', load);
  await load();
  const timer = setInterval(() => { if (auto.checked) load(); }, 3000);
  return { leave: () => clearInterval(timer) };
}
