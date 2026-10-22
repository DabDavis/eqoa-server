import { listPage } from './tablepage.js';

// Merchants are NPCs with the merchant bit: their own page shows the stock.
export async function render(box) {
  return listPage(box, {
    table: 'npcs', base: 'npcs', title: 'Merchants', allowNew: false,
    sub: 'NPCs that open a merchant window (npc_type bit 0x01). Open one to change what it sells.',
    filters: [{ column: 'npc_type', op: 'bits', value: 1 }],
    columns: ['id', 'npc_name', 'npc_level', 'world', 'x_coord', 'z_coord', 'npc_type'],
  });
}
