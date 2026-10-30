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
  { id: 'glenair-26482-contact-arrangements', publisher: 'Glenair', title: 'MIL-DTL-26482 contact face layouts (MIL-STD-1669)', url: 'https://cdn.glenair.com/mil-dtl-26482-series-2-type-bayonet-lock-connectors/pdf/mil-std-1669-contact-arrangements.pdf' },
  { id: 'glenair-26482-tables-alternate', publisher: 'Glenair', title: 'MIL-DTL-26482 contact count and alternate rotation tables', url: 'https://www.glenair.com/mil-dtl-26482-series-ii-type-bayonet-lock-connectors/pdf/mil-std-1669-contact-arrangements-table-and-alternate-positions.pdf' },
  { id: 'glenair-26482-catalog', publisher: 'Glenair', title: 'MIL-DTL-26482 complete catalog', url: 'https://www.glenair.com/catalogs/mil-dtl-26482-series-2-type-bayonet-lock-connectors.pdf' },
  { id: 'glenair-5015-1651', publisher: 'Glenair', title: 'MIL-DTL-5015 insert arrangements per MIL-STD-1651', url: 'https://www.glenair.com/hermetic-connectors/mil-dtl-5015/pdf/insert-arrangements-per-mil-std-1651.pdf' },
  { id: 'glenair-83723-1554', publisher: 'Glenair', title: 'MIL-DTL-83723 Series III contact face layouts (MIL-STD-1554)', url: 'https://www.glenair.com/hermetic-connectors/mil-dtl-83723/pdf/insert-arrangements-per-mil-std-1554.pdf' },
  { id: 'glenair-83723-keyway', publisher: 'Glenair', title: 'MIL-DTL-83723 keying and contact populations', url: 'https://www.glenair.com/hermetic-connectors/mil-dtl-83723/pdf/insert-arrangements-and-keyway-positions.pdf' },
  { id: 'glenair-28840-arrangements', publisher: 'Glenair', title: 'MIL-DTL-28840 standard insert arrangements', url: 'https://www.glenair.com/navy-approved-connectors-and-accessories/mil-dtl-28840-shipboard-electrical-connectors/pdf/standard-insert-arrangements.pdf' },
  { id: 'glenair-micro-d-arrangements', publisher: 'Glenair', title: 'MIL-DTL-83513 micro-D contact arrangements', url: 'https://cdn.glenair.com/micro-d/pdf/a/micro-d-contact-arrangements.pdf' },
  { id: 'glenair-hermetic-micro-d', publisher: 'Glenair', title: 'Hermetic micro-D design notes and contact arrangements', url: 'https://www.glenair.com/hermetic-connectors/mil-dtl-83513/pdf/design-notes-and-contact-arrangements.pdf' },
  { id: 'glenair-arinc-600', publisher: 'Glenair', title: 'ARINC 600 filter connector insert arrangements', url: 'https://www.glenair.com/capability/rectangular-connectors/pdf/arinc-600-filter-connectors.pdf' },
  { id: 'digikey-amphenol-5015-matrix', publisher: 'Amphenol Aerospace (via DigiKey)', title: 'MIL-DTL-5015 Matrix catalog', url: 'https://mm.digikey.com/Volume0/opasdata/d220001/medias/docus/1154/MIL-DTL-5015.pdf' },
  { id: 'mouser-amphenol-high-power-5015', publisher: 'Amphenol Aerospace (via Mouser)', title: 'High-power 5015 catalog', url: 'https://www.mouser.com/pdfDocs/amphenol-high-power-5015-data.pdf' },
  { id: 'heilind-amphenol-class-l', publisher: 'Amphenol Aerospace (via Heilind)', title: 'MIL-DTL-22992 Class L catalog', url: 'https://development.heilind.com/cms/wp-content/uploads/sites/3/2022/06/amphenol-aerospace-22992-class-l-catalog.pdf' },
  { id: 'mouser-amphenol-fiber-optic', publisher: 'Amphenol Aerospace (via Mouser)', title: 'Fiber-optic catalog', url: 'https://www.mouser.com/datasheet/2/18/1/fiber_catalog-1128958.pdf' },
  { id: 'glenair-high-speed-rf-hybrid', publisher: 'Glenair', title: '38999 high-speed / RF hybrid insert arrangements', url: 'https://www.glenair.com/space-mechanisms/blind-mate/pdf/data-rf/high-speed-data-and-rf-insert-arrangements.pdf' },
  { id: 'glenair-series-iv-hybrid', publisher: 'Glenair', title: '38999 Series IV hybrid arrangements', url: 'https://www.glenair.com/supernine-series-iv/pdf/234-217.pdf' },
  { id: 'dla-mil-std-1669-record', publisher: 'DLA ASSIST', title: 'MIL-STD-1669 (MIL-DTL-26482) document record', url: 'https://quicksearch.dla.mil/qsDocDetails.aspx?ident_number=37064', manualOnly: true },
  { id: 'dla-mil-std-1651-record', publisher: 'DLA ASSIST', title: 'MIL-STD-1651 (5015, 22992, 83723 Series II) document record', url: 'https://quicksearch.dla.mil/qsDocDetails.aspx?ident_number=37045', manualOnly: true },
  { id: 'dla-mil-std-1554-record', publisher: 'DLA ASSIST', title: 'MIL-STD-1554 (83723 Series III, 26500) document record', url: 'https://quicksearch.dla.mil/qsDocDetails.aspx?ident_number=36974', manualOnly: true },
  { id: 'dla-mil-std-1698-record', publisher: 'DLA ASSIST', title: 'MIL-STD-1698 (28840) document record', url: 'https://quicksearch.dla.mil/qsDocDetails.aspx?ident_number=37104', manualOnly: true },
  { id: 'dla-mil-std-3067-record', publisher: 'DLA ASSIST', title: 'MIL-STD-3067 (32689) document record', url: 'https://quicksearch.dla.mil/qsDocDetails.aspx?ident_number=284646', manualOnly: true },
  { id: 'dla-mil-std-1674-record', publisher: 'DLA ASSIST', title: 'MIL-STD-1674 (MIL-C-85028 rectangular) document record', url: 'https://quicksearch.dla.mil/qsDocDetails.aspx?ident_number=37068', manualOnly: true },
  { id: 'dla-mil-hdbk-536-record', publisher: 'DLA ASSIST', title: 'MIL-HDBK-536 (38999 contacts and termini) document record', url: 'https://quicksearch.dla.mil/qsDocDetails.aspx?ident_number=284910', manualOnly: true },
  { id: 'dla-mil-dtl-24308-record', publisher: 'DLA ASSIST', title: 'MIL-DTL-24308 D-sub document record', url: 'https://quicksearch.dla.mil/qsDocDetails.aspx?ident_number=17161', manualOnly: true },
  { id: 'dla-mil-dtl-83513-record', publisher: 'DLA ASSIST', title: 'MIL-DTL-83513 micro-D document record', url: 'https://quicksearch.dla.mil/qsDocDetails.aspx?ident_number=33934', manualOnly: true },
  { id: 'dla-mil-dtl-28748-record', publisher: 'DLA ASSIST', title: 'MIL-DTL-28748 rack and panel document record', url: 'https://quicksearch.dla.mil/qsDocDetails.aspx?ident_number=19223', manualOnly: true },
  { id: 'dla-mil-dtl-83527-record', publisher: 'DLA ASSIST', title: 'MIL-DTL-83527 multiple insert rack-to-panel document record', url: 'https://quicksearch.dla.mil/qsDocDetails.aspx?ident_number=67471', manualOnly: true },
  { id: 'dla-mil-dtl-55302-59-record', publisher: 'DLA ASSIST', title: 'MIL-DTL-55302/59 document record', url: 'https://quicksearch.dla.mil/qsDocDetails.aspx?ident_number=29472', manualOnly: true },
  { id: 'dla-mil-dtl-55302-61-record', publisher: 'DLA ASSIST', title: 'MIL-DTL-55302/61 document record', url: 'https://quicksearch.dla.mil/qsDocDetails.aspx?ident_number=29474', manualOnly: true },
  { id: 'dla-mil-dtl-55302-128-record', publisher: 'DLA ASSIST', title: 'MIL-DTL-55302/128 document record', url: 'https://quicksearch.dla.mil/qsDocDetails.aspx?ident_number=29533', manualOnly: true },
  { id: 'dla-mil-dtl-55302-157-record', publisher: 'DLA ASSIST', title: 'MIL-DTL-55302/157 document record', url: 'https://quicksearch.dla.mil/qsDocDetails.aspx?ident_number=29562', manualOnly: true },
  { id: 'dla-mil-dtl-55302-159-record', publisher: 'DLA ASSIST', title: 'MIL-DTL-55302/159 document record', url: 'https://quicksearch.dla.mil/qsDocDetails.aspx?ident_number=115576', manualOnly: true },
  { id: 'dla-mil-dtl-55302-176-record', publisher: 'DLA ASSIST', title: 'MIL-DTL-55302/176 document record', url: 'https://quicksearch.dla.mil/qsDocDetails.aspx?ident_number=29581', manualOnly: true },
  { id: 'dla-mil-dtl-55302-190-record', publisher: 'DLA ASSIST', title: 'MIL-DTL-55302/190 document record', url: 'https://quicksearch.dla.mil/qsDocDetails.aspx?ident_number=67273', manualOnly: true },
  { id: 'dla-mil-std-348-record', publisher: 'DLA ASSIST', title: 'MIL-STD-348 RF connector interfaces document record', url: 'https://quicksearch.dla.mil/qsDocDetails.aspx?ident_number=35726', manualOnly: true },
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
      const r = await fetch(d.url, { redirect: 'follow', headers: { 'User-Agent': 'TrivexReferenceFetch/1.0' }, signal: AbortSignal.timeout(60000) });
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
