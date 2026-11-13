// Developer tool: draws the Trivex mark (the same shapes as the sidebar logo) at 16, 32, 48 and 256 pixels, writes the
// PNGs and an .ico to icon/, and writes rsrc_windows_amd64.syso so the Windows linker embeds the icon in the executable.
//
//   node scripts/make_windows_icon.js
//
// No external libraries: a small supersampling rasteriser, a PNG writer (zlib) and a COFF resource writer.
const fs = require('fs'), path = require('path'), zlib = require('zlib');
const root = path.join(__dirname, '..'), iconDir = path.join(root, 'icon');
fs.mkdirSync(iconDir, { recursive: true });

// --- geometry in a 64 x 64 box -------------------------------------------------------------------------------
const BG = [0x10, 0x25, 0x3d], TEAL = [0x29, 0xc1, 0xbb], WHITE = [255, 255, 255];
function cubic(p0, p1, p2, p3, n = 24) { const out = []; for (let i = 0; i <= n; i++) { const t = i / n, u = 1 - t; out.push([u * u * u * p0[0] + 3 * u * u * t * p1[0] + 3 * u * t * t * p2[0] + t * t * t * p3[0], u * u * u * p0[1] + 3 * u * u * t * p1[1] + 3 * u * t * t * p2[1] + t * t * t * p3[1]]); } return out; }
const polylines = [
  [[10, 32], [26, 32]],
  [...cubic([26, 32], [33, 32], [33, 20], [42, 20]), [50, 20]],
  [[26, 32], [50, 32]],
  [...cubic([26, 32], [33, 32], [33, 44], [42, 44]), [50, 44]],
];
const dots = [[52, 20], [52, 32], [52, 44]];
function segDist(px, py, a, b) { const dx = b[0] - a[0], dy = b[1] - a[1], l2 = dx * dx + dy * dy; let t = l2 ? ((px - a[0]) * dx + (py - a[1]) * dy) / l2 : 0; t = Math.max(0, Math.min(1, t)); return Math.hypot(px - (a[0] + t * dx), py - (a[1] + t * dy)); }
function inRoundRect(x, y) { const r = 14; if (x < 0 || y < 0 || x > 64 || y > 64) return false; const cx = x < r ? r : x > 64 - r ? 64 - r : x, cy = y < r ? r : y > 64 - r ? 64 - r : y; return Math.hypot(x - cx, y - cy) <= r; }
function sample(x, y) {
  if (!inRoundRect(x, y)) return null;
  for (const d of dots) if (Math.hypot(x - d[0], y - d[1]) <= 4) return WHITE;
  for (const pl of polylines) for (let i = 0; i + 1 < pl.length; i++) if (segDist(x, y, pl[i], pl[i + 1]) <= 2) return TEAL;
  return BG;
}
function render(size) {
  const px = Buffer.alloc(size * size * 4), SS = 4;
  for (let j = 0; j < size; j++) for (let i = 0; i < size; i++) {
    let a = 0, r = 0, g = 0, b = 0;
    for (let sj = 0; sj < SS; sj++) for (let si = 0; si < SS; si++) {
      const c = sample((i + (si + 0.5) / SS) * 64 / size, (j + (sj + 0.5) / SS) * 64 / size);
      if (c) { a++; r += c[0]; g += c[1]; b += c[2]; }
    }
    const o = (j * size + i) * 4, n = SS * SS;
    px[o] = a ? Math.round(r / a) : 0; px[o + 1] = a ? Math.round(g / a) : 0; px[o + 2] = a ? Math.round(b / a) : 0; px[o + 3] = Math.round(255 * a / n);
  }
  return px;
}

// --- PNG ------------------------------------------------------------------------------------------------------
const crcT = (() => { const t = []; for (let n = 0; n < 256; n++) { let c = n; for (let k = 0; k < 8; k++) c = c & 1 ? 0xEDB88320 ^ (c >>> 1) : c >>> 1; t[n] = c >>> 0; } return t; })();
const crc32 = b => { let r = 0xFFFFFFFF; for (const x of b) r = crcT[(r ^ x) & 255] ^ (r >>> 8); return (r ^ 0xFFFFFFFF) >>> 0; };
function chunk(type, data) { const len = Buffer.alloc(4); len.writeUInt32BE(data.length); const td = Buffer.concat([Buffer.from(type), data]), crc = Buffer.alloc(4); crc.writeUInt32BE(crc32(td)); return Buffer.concat([len, td, crc]); }
function png(size, px) {
  const ihdr = Buffer.alloc(13); ihdr.writeUInt32BE(size, 0); ihdr.writeUInt32BE(size, 4); ihdr[8] = 8; ihdr[9] = 6;
  const raw = Buffer.alloc((size * 4 + 1) * size);
  for (let j = 0; j < size; j++) { raw[j * (size * 4 + 1)] = 0; px.copy(raw, j * (size * 4 + 1) + 1, j * size * 4, (j + 1) * size * 4); }
  return Buffer.concat([Buffer.from('89504e470d0a1a0a', 'hex'), chunk('IHDR', ihdr), chunk('IDAT', zlib.deflateSync(raw, { level: 9 })), chunk('IEND', Buffer.alloc(0))]);
}
// 32-bit DIB for small icon sizes: BITMAPINFOHEADER (height doubled), BGRA rows bottom-up, then a 1-bit AND mask (all zero).
function dib(size, px) {
  const h = Buffer.alloc(40); h.writeUInt32LE(40, 0); h.writeInt32LE(size, 4); h.writeInt32LE(size * 2, 8); h.writeUInt16LE(1, 12); h.writeUInt16LE(32, 14); h.writeUInt32LE(0, 16);
  const xor = Buffer.alloc(size * size * 4);
  for (let j = 0; j < size; j++) for (let i = 0; i < size; i++) { const s = ((size - 1 - j) * size + i) * 4, d = (j * size + i) * 4; xor[d] = px[s + 2]; xor[d + 1] = px[s + 1]; xor[d + 2] = px[s]; xor[d + 3] = px[s + 3]; }
  const maskRow = Math.ceil(size / 32) * 4;
  return Buffer.concat([h, xor, Buffer.alloc(maskRow * size)]);
}

const sizes = [16, 32, 48, 256], images = sizes.map(s => { const px = render(s); const p = png(s, px); fs.writeFileSync(path.join(iconDir, `trivex_${s}.png`), p); return { size: s, data: s >= 256 ? p : dib(s, px) }; });

// --- .ico (for reference and for the browser) -----------------------------------------------------------------
{
  const head = Buffer.alloc(6); head.writeUInt16LE(0, 0); head.writeUInt16LE(1, 2); head.writeUInt16LE(images.length, 4);
  let off = 6 + images.length * 16; const dir = [], blobs = [];
  for (const im of images) { const e = Buffer.alloc(16); e[0] = im.size >= 256 ? 0 : im.size; e[1] = im.size >= 256 ? 0 : im.size; e.writeUInt16LE(1, 4); e.writeUInt16LE(32, 6); e.writeUInt32LE(im.data.length, 8); e.writeUInt32LE(off, 12); off += im.data.length; dir.push(e); blobs.push(im.data); }
  fs.writeFileSync(path.join(iconDir, 'trivex.ico'), Buffer.concat([head, ...dir, ...blobs]));
}

// --- COFF resource object -------------------------------------------------------------------------------------
// Tree: type (3 = RT_ICON, 14 = RT_GROUP_ICON) -> name (icon id / group id 1) -> language (0x0409) -> data entry.
const u16 = n => { const b = Buffer.alloc(2); b.writeUInt16LE(n); return b; }, u32 = n => { const b = Buffer.alloc(4); b.writeUInt32LE(n >>> 0); return b; };
const dirHeader = n => Buffer.concat([u32(0), u32(0), u16(0), u16(0), u16(0), u16(n)]);
const entry = (id, off, sub) => Buffer.concat([u32(id), u32(sub ? (off | 0x80000000) >>> 0 : off)]);
const grp = Buffer.concat([u16(0), u16(1), u16(images.length), ...images.map((im, i) => { const e = Buffer.alloc(14); e[0] = im.size >= 256 ? 0 : im.size; e[1] = im.size >= 256 ? 0 : im.size; e.writeUInt16LE(1, 4); e.writeUInt16LE(32, 6); e.writeUInt32LE(im.data.length, 8); e.writeUInt16LE(i + 1, 12); return e; })]);
const blobs = [...images.map(im => im.data), grp], n = images.length;
// layout: root (16 + 2*8) | RT_ICON names dir (16 + n*8) | RT_GROUP names dir (16 + 8) | language dirs (n+1) * (16 + 8) | data entries (n+1) * 16 | blobs
const rootSz = 16 + 16, iconNamesSz = 16 + n * 8, grpNamesSz = 16 + 8, langSz = 16 + 8;
const oRoot = 0, oIconNames = rootSz, oGrpNames = oIconNames + iconNamesSz, oLang = oGrpNames + grpNamesSz, oData = oLang + (n + 1) * langSz;
let oBlob = oData + (n + 1) * 16; const blobOff = [];
for (const b of blobs) { blobOff.push(oBlob); oBlob += (b.length + 3) & ~3; }
const parts = [dirHeader(2), entry(3, oIconNames, true), entry(14, oGrpNames, true), dirHeader(n)];
for (let i = 0; i < n; i++) parts.push(entry(i + 1, oLang + i * langSz, true));
parts.push(dirHeader(1), entry(1, oLang + n * langSz, true));
for (let i = 0; i <= n; i++) parts.push(dirHeader(1), entry(0x0409, oData + i * 16, false));
const relocs = [];
for (let i = 0; i <= n; i++) { relocs.push(oData + i * 16); parts.push(u32(blobOff[i]), u32(blobs[i].length), u32(0), u32(0)); }
for (let i = 0; i <= n; i++) parts.push(blobs[i], Buffer.alloc(((blobs[i].length + 3) & ~3) - blobs[i].length));
const section = Buffer.concat(parts);
const fileHeaderSz = 20, secHdrSz = 40, rawPtr = fileHeaderSz + secHdrSz, relPtr = rawPtr + section.length, symPtr = relPtr + relocs.length * 10;
const fh = Buffer.concat([u16(0x8664), u16(1), u32(0), u32(symPtr), u32(1), u16(0), u16(0)]);
const sh = Buffer.concat([Buffer.from('.rsrc\0\0\0', 'binary'), u32(0), u32(0), u32(section.length), u32(rawPtr), u32(relPtr), u32(0), u16(relocs.length), u16(0), u32(0x40000040)]);
const rel = Buffer.concat(relocs.map(o => Buffer.concat([u32(o), u32(0), u16(3)])));
const sym = Buffer.concat([Buffer.from('.rsrc\0\0\0', 'binary'), u32(0), u16(1), u16(0), Buffer.from([3, 0])]);
fs.writeFileSync(path.join(root, 'rsrc_windows_amd64.syso'), Buffer.concat([fh, sh, section, rel, sym, u32(4)]));
console.log('wrote icon/trivex_{16,32,48,256}.png, icon/trivex.ico and rsrc_windows_amd64.syso (' + section.length + ' byte resource section)');
