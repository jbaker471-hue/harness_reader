// Developer tool: reads the numeric "Contact Hole Locations" tables from the Amphenol MIL-DTL-38999
// catalog (a PDF the user downloaded) and writes validated rows to assets/CONNECTOR_INSERT_GEOMETRY.csv.
//
//   node scripts/extract_amphenol_38999.js <catalog.pdf> [--write]
//
// It uses the external `pdftotext` (poppler/xpdf) command; the application itself does not.
// Numbers come from the printed tables, never from the drawings. An arrangement is written only when
//   - the table has exactly the stated number of contacts, numbered 1..N once each,
//   - every coordinate parses,
//   - for X35 arrangements, the contact numbers equal the reviewed inventory in CONNECTOR_LAYOUT_REFERENCE.csv.
// Anything else is reported and skipped. The catalog tables are in inches, front face of the pin insert,
// normal rotation; they are stored in millimetres with the view "pin-front".
const fs = require('fs'), path = require('path'), cp = require('child_process'), crypto = require('crypto');

const pdf = process.argv[2], write = process.argv.includes('--write');
if (!pdf) { console.error('usage: node scripts/extract_amphenol_38999.js <catalog.pdf> [--write]'); process.exit(2); }
const buf = fs.readFileSync(pdf);
if (!buf.subarray(0, 1024).toString('latin1').includes('%PDF-')) { console.error('not a PDF'); process.exit(1); }
const sha = crypto.createHash('sha256').update(buf).digest('hex');
const text = mode => cp.execFileSync('pdftotext', [mode, pdf, '-'], { maxBuffer: 256 << 20, encoding: 'utf8' });

const NUM = String.raw`[+-]?(?:\d+)?\.\d{3}`;
const reTriple = new RegExp(String.raw`(?<![\w.+-])(\d{1,3}|[A-Za-z]{1,2})\s+(${NUM})\s+(${NUM})(?![\d.])`, 'g');
const reHead = /INSERT ARRANGEMENT\s*#\s*(\d{1,2})-(\d{1,2})\s*\/\s*(\d{1,2})-(\d{1,2})/i;
const reSpec = /(?<!\S)(\d{1,3})\s+(22D|20|16|12)\s+(M|I{1,3})(?!\S)/;
const shellLetter = { 9: 'A', 11: 'B', 13: 'C', 15: 'D', 17: 'E', 19: 'F', 21: 'G', 23: 'H', 25: 'J' };

const inv = {};
for (const line of fs.readFileSync(path.join(__dirname, '..', 'assets', 'CONNECTOR_LAYOUT_REFERENCE.csv'), 'utf8').split(/\r?\n/).slice(1)) {
  const c = line.split(','); if (c[0] === 'D38999 Series III') inv[c[1]] = c[2].split(';');
}

// One pass over the catalog text produced by a given pdftotext mode.
function parse(mode) {
  const out = new Map();
  text(mode).split('\f').forEach((page, pi) => {
    if (!/Contact Hole Locations/i.test(page)) return;
    const lines = page.split('\n'), heads = [];
    lines.forEach((l, i) => { const m = l.match(reHead); if (m) heads.push({ i, m }); });
    heads.forEach((h, k) => {
      const seg = lines.slice(h.i, k + 1 < heads.length ? heads[k + 1].i : lines.length).join('\n');
      const id = h.m[3] + '-' + h.m[4], spec = seg.match(reSpec), pts = new Map(), dup = [], problems = [];
      for (const m of seg.matchAll(reTriple)) { if (pts.has(m[1])) dup.push(m[1]); pts.set(m[1], [m[2], m[3]]); }
      const n = spec ? +spec[1] : 0;
      if (!spec) problems.push('contact count / size line not found');
      if (dup.length) problems.push('repeated contact numbers ' + [...new Set(dup)].join(','));
      if (n && pts.size !== n) problems.push('found ' + pts.size + ' coordinate rows, expected ' + n);
      // Numbered contacts must run 1..N; lettered insert patterns (A, B, a, b ...) are checked for count and uniqueness only.
      const lettered = [...pts.keys()].some(k => /[A-Za-z]/.test(k));
      if (!lettered) for (let i = 1; n && i <= n; i++) if (!pts.has(String(i))) { problems.push('missing contact ' + i); break; }
      const key = (shellLetter[+h.m[3]] || '?') + h.m[4];
      if (!problems.length && inv[key] && JSON.stringify([...pts.keys()].sort()) !== JSON.stringify([...inv[key]].sort())) problems.push('contact numbers differ from the reviewed inventory ' + key);
      // Two contacts cannot sit on top of each other: a mis-paired number and coordinate would usually break this.
      if (!problems.length) {
        const v = [...pts.entries()].map(([k, c]) => [k, parseFloat(c[0]), parseFloat(c[1])]);
        for (let i = 0; i < v.length && !problems.length; i++) for (let j = i + 1; j < v.length; j++) if (Math.hypot(v[i][1] - v[j][1], v[i][2] - v[j][2]) < 0.05) { problems.push('contacts ' + v[i][0] + ' and ' + v[j][0] + ' overlap'); break; }
      }
      out.set(id, { id, page: pi + 1, h, n, spec, pts, lettered, problems, key });
    });
  });
  return out;
}

// Both text modes must read the same table. A mode that fails validation is ignored; if two modes both
// validate they must agree on every coordinate, otherwise the arrangement is skipped.
const modes = [parse('-table'), parse('-layout')], report = [], rows = [];
const ids = new Set(modes.flatMap(m => [...m.keys()]));
for (const id of ids) {
  const ok = modes.map(m => m.get(id)).filter(r => r && !r.problems.length);
  const any = modes.map(m => m.get(id)).find(Boolean);
  let why = ok.length ? '' : [...new Set(modes.map(m => m.get(id)).filter(Boolean).flatMap(r => r.problems))].join('; ');
  if (ok.length === 2) {
    const [a, b] = ok;
    if (a.pts.size !== b.pts.size || [...a.pts].some(([k, v]) => !b.pts.has(k) || b.pts.get(k)[0] !== v[0] || b.pts.get(k)[1] !== v[1])) why = 'the two text readings of the table disagree';
  }
  const tag = 'PDF page ' + any.page;
  report.push(id.padEnd(6) + ' ' + tag.padEnd(12) + ' contacts=' + any.n + ' ' + (why ? 'SKIPPED: ' + why : 'ok (' + ok.length + ' reading' + (ok.length > 1 ? 's agree' : '') + (inv[any.key] ? '; matches reviewed inventory ' + any.key : '; no reviewed inventory to compare') + ')'));
  if (why) continue;
  const r = ok[0], size = r.spec[2], rating = r.spec[3];
  for (const label of r.lettered ? [...r.pts.keys()] : Array.from({ length: r.n }, (_, i) => String(i + 1))) {
    const [x, y] = r.pts.get(label).map(parseFloat), yy = (-y * 25.4).toFixed(3);
    // Table Y points up; the application draws Y down. Inches to millimetres.
    rows.push([id, 'MIL-STD-1560', r.h.m[3], label, (x * 25.4).toFixed(3), yy === '-0.000' ? '0.000' : yy, size, 'pin-front', 'normal rotation as shown',
      'Amphenol Aerospace MIL-DTL-38999 catalog PDF (SHA-256 ' + sha.slice(0, 16) + '...) ' + tag, String(r.page), 'Insert arrangement ' + r.h.m[1] + '-' + r.h.m[2] + ' / ' + id + ', Contact Hole Locations table, service rating ' + rating,
      'EXTRACTED FROM MANUFACTURER NUMERIC TABLE — UNREVIEWED']);
  }
}
console.log(report.join('\n'));
const q = s => /[",\n]/.test(s) ? '"' + s.replace(/"/g, '""') + '"' : s;
if (write) {
  const f = path.join(__dirname, '..', 'assets', 'CONNECTOR_INSERT_GEOMETRY.csv');
  const head = 'arrangement_id,standard,shell_size,cavity,x_mm,y_mm,contact_size,view,keying,source,source_page,source_figure,status';
  fs.writeFileSync(f, head + '\r\n' + rows.map(r => r.map(q).join(',')).join('\r\n') + '\r\n');
  console.log(`wrote ${rows.length} rows to ${f}`);
} else console.log(`${rows.length} rows ready (use --write)`);
