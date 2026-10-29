// Developer tool, not part of the offline application: fetches the public reference PDFs that
// CONNECTOR_SPEC_REFERENCE.csv points to, into a local folder OUTSIDE the repository, and records
// what happened for each one in manifest.json.
//
//   node scripts/fetch_reference_docs.js <output-dir>
//
// If TRIVEX_URL and TRIVEX_TOKEN are set (headless app: TRIVEX_NO_WINDOW=1), each file is also run
// through the app's own PDF parser via /api/reference-validate.
//
// A download is kept only when the HTTP status is 200 and the file starts with %PDF- and is not an
// HTML page. Consent banners, login pages and access-denied pages are never bypassed: they are
// recorded as MANUAL_DOWNLOAD_REQUIRED so the file can be downloaded in a browser and imported.
// Publisher licence terms apply to anything stored.
const fs = require('fs'), path = require('path'), crypto = require('crypto');

const DOCS = [
  { id: 'amphenol-38999-catalog', publisher: 'Amphenol Aerospace', title: 'MIL-DTL-38999 catalog (insert arrangements, cavity coordinate tables)', url: 'https://www.amphenol-aerospace.com/resources/literature/download/38999-catalog' },
  { id: 'glenair-1560-standard', publisher: 'Glenair', title: 'MIL-STD-1560 standard power and signal contact arrangements', url: 'https://www.glenair.com/mil-dtl-38999-connector-series-iii/pdf/mil-std-1560-standard-power-and-signal-contact-arrangements.pdf' },
  { id: 'glenair-supernine-1560', publisher: 'Glenair', title: 'SuperNine MIL-STD-1560 and other standard contact arrangements', url: 'https://cdn.glenair.com/supernine/reference/pdf/mil-std-1560-and-other-standard-contact-arrangements.pdf' },
  { id: 'glenair-1560-high-speed', publisher: 'Glenair', title: 'MIL-STD-1560 high-speed contact arrangements', url: 'https://www.glenair.com/mil-dtl-38999-connector-series-iii/pdf/mil-std-1560-high-speed-contact-arrangements.pdf' },
  { id: 'glenair-1560-combo', publisher: 'Glenair', title: 'MIL-STD-1560 mixed (combo) contact arrangements, pin front view', url: 'https://www.glenair.com/mil-dtl-38999-connector-series-iii/pdf/mil-std-1560-combo-contact-arrangements-pin-front-view.pdf' },
  { id: 'glenair-1560-high-density', publisher: 'Glenair', title: 'MIL-STD-1560 high-density contact arrangements', url: 'https://www.glenair.com/mil-dtl-38999-connector-series-iii/pdf/mil-std-1560-high-density-contact-arrangements.pdf' },
  { id: 'glenair-pin-contact-guide', publisher: 'Glenair', title: 'Series III and IV pin contact selection guide', url: 'https://www.glenair.com/mil-dtl-38999-connector-series-iii/pdf/series-iii-and-iv-pin-contact-selection-guide-for-environmental-connectors.pdf' },
  { id: 'glenair-socket-contact-guide', publisher: 'Glenair', title: 'Series III and IV socket contact selection guide', url: 'https://www.glenair.com/mil-dtl-38999-connector-series-iii/pdf/series-iii-and-iv-socket-contact-selection-guide.pdf' },
  { id: 'glenair-crimp-install-tools', publisher: 'Glenair', title: 'Contact crimp and installation tools', url: 'https://www.glenair.com/mil-dtl-38999-connector-series-iii/pdf/contact-crimp-and-installation-tools.pdf' },
  { id: 'dla-mil-std-1560-record', publisher: 'DLA ASSIST', title: 'Official MIL-STD-1560 document record', url: 'https://quicksearch.dla.mil/qsDocDetails.aspx?ident_number=36980', manualOnly: true },
];

async function validateWithApp(buf) {
  const base = process.env.TRIVEX_URL, token = process.env.TRIVEX_TOKEN;
  if (!base || !token) return null;
  const r = await fetch(new URL('api/reference-validate', base), { method: 'POST', headers: { 'X-Trivex-Token': token, 'Content-Type': 'application/pdf' }, body: buf });
  return r.ok ? r.json() : { ok: false, status: 'REJECTED', reason: 'validator returned HTTP ' + r.status };
}

(async () => {
  const out = process.argv[2];
  if (!out) { console.error('usage: node scripts/fetch_reference_docs.js <output-dir outside the repository>'); process.exit(2); }
  if (path.resolve(out).toLowerCase().startsWith(path.resolve(__dirname, '..').toLowerCase() + path.sep)) { console.error('output directory must be outside the repository'); process.exit(2); }
  fs.mkdirSync(out, { recursive: true });
  const manifest = [];
  for (const d of DOCS) {
    const rec = { id: d.id, publisher: d.publisher, title: d.title, url: d.url, retrieved: new Date().toISOString().slice(0, 10), status: '', detail: '' };
    if (d.manualOnly) { rec.status = 'MANUAL_DOWNLOAD_REQUIRED'; rec.detail = 'Official record page; retrieve it in a browser.'; manifest.push(rec); continue; }
    try {
      const r = await fetch(d.url, { redirect: 'follow', headers: { 'User-Agent': 'TrivexReferenceFetch/1.0' }, signal: AbortSignal.timeout(180000) });
      const buf = Buffer.from(await r.arrayBuffer());
      const head = buf.subarray(0, 1024).toString('latin1');
      rec.http = r.status; rec.contentType = r.headers.get('content-type') || ''; rec.bytes = buf.length;
      if (r.status !== 200) { rec.status = 'MANUAL_DOWNLOAD_REQUIRED'; rec.detail = 'HTTP ' + r.status + ' (access control or error page); not bypassed.'; }
      else if (/^\s*(<!doctype html|<html|<head)/i.test(head)) { rec.status = 'MANUAL_DOWNLOAD_REQUIRED'; rec.detail = 'HTML page returned instead of a PDF (banner, login or access-denied page); not bypassed.'; }
      else if (!head.includes('%PDF-')) { rec.status = 'REJECTED'; rec.detail = 'No %PDF- signature.'; }
      else {
        const v = await validateWithApp(buf);
        if (v && !v.ok) { rec.status = v.status === 'MANUAL_DOWNLOAD_REQUIRED' ? 'MANUAL_DOWNLOAD_REQUIRED' : 'REJECTED'; rec.detail = v.reason; }
        else {
          const file = d.id + '.pdf';
          fs.writeFileSync(path.join(out, file), buf);
          rec.status = 'DOWNLOADED_PDF_VERIFIED'; rec.file = file; rec.sha256 = crypto.createHash('sha256').update(buf).digest('hex');
          rec.pages = v ? v.pages : undefined;
          rec.detail = v ? 'Signature and PDF parser checks passed.' : 'Signature check passed; parser check skipped (set TRIVEX_URL and TRIVEX_TOKEN).';
        }
      }
    } catch (e) { rec.status = 'MANUAL_DOWNLOAD_REQUIRED'; rec.detail = 'Download failed: ' + (e.cause?.code || e.message); }
    manifest.push(rec);
    console.log(rec.status.padEnd(26), d.id, rec.detail);
  }
  fs.writeFileSync(path.join(out, 'manifest.json'), JSON.stringify(manifest, null, 2));
  console.log('manifest written to', path.join(out, 'manifest.json'));
})();
