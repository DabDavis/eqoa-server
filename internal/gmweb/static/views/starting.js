import { h, mount, pageHead } from '../lib/dom.js';
import { describe } from '../lib/meta.js';
import { DataGrid } from '../lib/grid.js';
import { go } from '../lib/nav.js';
import { keyPath } from './tablepage.js';

// What a new character starts as: the templates per race, city and class,
// the class spell lists and the body models.
export async function render(box) {
  const sections = [
    ['defaultcharacter', 'Starting templates', 'One row per race, home city and class a character may be: starting stats, money and place.'],
    ['classspells', 'Class spells', 'Which spells each class has (given at creation when start_all_spells is on).'],
    ['charactermodel', 'Body models', 'The body model for each race and sex.'],
  ];
  mount(box, pageHead('New characters', 'What a character starts as. The level and "every class spell" switches are on the Settings page.'));
  for (const [table, title, sub] of sections) {
    const meta = await describe(table);
    const grid = new DataGrid({ table, meta, pageSize: 25, onOpen: (r) => go('tables/' + table + '/' + keyPath(meta, r.__key)) });
    box.append(h('h2', null, title), h('p', { class: 'muted' }, sub), grid.el, h('div', { class: 'spacer' }));
    grid.load();
  }
}
