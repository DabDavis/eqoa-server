// The disc image's file table: an ISO 9660 walk from the primary volume
// descriptor, so a file is found by its path rather than by a sector number
// one disc happened to have.

const SECTOR = 2048;

// readTOC returns Map(PATH -> {lba, size, offset}) for every file, paths in
// upper case with "/" ("DATA/CHAR.ESF"); read(offset, length) reads the image.
export async function readTOC(read, imageSize) {
  const pvd = new DataView(await read(16 * SECTOR, SECTOR));
  const id = String.fromCharCode(pvd.getUint8(1), pvd.getUint8(2), pvd.getUint8(3), pvd.getUint8(4), pvd.getUint8(5));
  if (pvd.getUint8(0) !== 1 || id !== 'CD001') throw new Error('not an ISO 9660 disc image');
  const label = new TextDecoder().decode(new Uint8Array(pvd.buffer, 40, 32)).trim();
  const rootLBA = pvd.getUint32(156 + 2, true);
  const rootSize = pvd.getUint32(156 + 10, true);
  const out = new Map();
  const seen = new Set();
  const walk = async (lba, size, prefix, depth) => {
    if (depth > 8 || seen.has(lba) || size > 4 << 20) return;
    seen.add(lba);
    const dv = new DataView(await read(lba * SECTOR, size));
    let p = 0;
    while (p < size) {
      const len = dv.getUint8(p);
      if (len === 0) {
        p = (Math.floor(p / SECTOR) + 1) * SECTOR; // records never cross a sector
        continue;
      }
      if (p + 33 > size) break;
      const ext = dv.getUint32(p + 2, true);
      const dsize = dv.getUint32(p + 10, true);
      const flags = dv.getUint8(p + 25);
      const nlen = dv.getUint8(p + 32);
      const raw = new Uint8Array(dv.buffer, p + 33, nlen);
      p += len;
      if (nlen === 1 && (raw[0] === 0 || raw[0] === 1)) continue; // . and ..
      let name = String.fromCharCode(...raw).toUpperCase();
      const semi = name.indexOf(';');
      if (semi >= 0) name = name.slice(0, semi);
      if (name.endsWith('.')) name = name.slice(0, -1);
      const path = prefix ? prefix + '/' + name : name;
      if (flags & 2) await walk(ext, dsize, path, depth + 1);
      else if (ext * SECTOR + dsize <= imageSize) out.set(path, { lba: ext, size: dsize, offset: ext * SECTOR });
    }
  };
  await walk(rootLBA, rootSize, '', 0);
  return { label, files: out };
}

// Which world file each world index loads (CLIENT's gWorldInfo, as the
// server's worldPaths has it). 6 and 8 up stream off a PS2's hard disk.
export const WORLD_FILES = {
  0: 'DATA/TUNARIA.ESF', 1: 'DATA2/RATHE.ESF', 2: 'DATA2/ODUS.ESF', 3: 'DATA2/LAVASTM.ESF',
  4: 'DATA2/PLANESKY.ESF', 5: 'DATA3/SECRETS.ESF', 7: 'DATA3/SECRETS.ESF',
};
