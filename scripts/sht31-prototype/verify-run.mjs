import fs from 'node:fs';
import { pathToFileURL } from 'node:url';

// A log consistency check, not proof of physical provenance or a bench signoff.
export function verifyRun(text) {
  let boot = false, ready = false, first, previous, count = 0;
  const fail = message => { throw new Error(message); };
  for (const line of text.split(/\r?\n/)) {
    if (!line.trim()) continue;
    if (!line.startsWith('{')) {
      if (boot) fail('non-JSON output after boot (possible reset or truncated log)');
      continue; // Boot-ROM banners before the single application boot are retained.
    }
    let r;
    try { r = JSON.parse(line); } catch { fail('malformed JSON record'); }
    if (r.type === 'fault') fail(`firmware fault: ${r.reason}`);
    if (r.type === 'boot') {
      if (boot) fail('multiple boots');
      if (r.firmware !== 'sht31-prototype-1' || r.family !== 'esp32_sht31_v1' ||
          r.profile !== 'standard' || r.sda !== 21 || r.scl !== 22 ||
          r.hz !== 100000 || r.address !== 68) fail('wrong firmware/configuration');
      boot = true;
    } else if (r.type === 'ready') {
      if (!boot || ready || r.heater !== false || r.radio_initialized !== false) fail('invalid ready');
      ready = true;
    } else if (r.type === 'sample') {
      if (!ready || r.crc_ok !== true || r.status_ok !== true || r.seq !== count + 1) fail('invalid or missing sample');
      for (const key of ['raw_t', 'raw_rh'])
        if (!Number.isInteger(r[key]) || r[key] < 0 || r[key] > 65535) fail('invalid raw word');
      if (!Number.isInteger(r.ms) || r.ms < 0 || r.ms > 0xffffffff) fail('invalid time');
      if (previous !== undefined && (r.ms - previous < 1000 || r.ms - previous > 2000)) fail('sample gap, time reversal or reset');
      if (!Number.isFinite(r.celsius) || !Number.isFinite(r.rh_percent) ||
          Math.abs(r.celsius - (-45 + 175 * r.raw_t / 65535)) > 0.00011 ||
          Math.abs(r.rh_percent - 100 * r.raw_rh / 65535) > 0.00011) fail('conversion mismatch');
      if (r.celsius < 10 || r.celsius > 35 || r.rh_percent < 35 || r.rh_percent > 60)
        fail('reading outside prototype screening envelope; investigate, do not call this an accuracy result');
      first ??= r.ms; previous = r.ms; count++;
    } else fail('unknown record');
  }
  if (!boot || !ready || count < 580 || previous - first < 600000) fail('incomplete ten-minute capture');
  return { status: 'serial_log_consistent_not_physical_signoff', samples: count,
    duration_ms: previous - first, physical_authenticity_verified: false };
}
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    if (process.argv.length !== 3) throw new Error('usage: node verify-run.mjs UART_LOG');
    console.log(JSON.stringify(verifyRun(fs.readFileSync(process.argv[2], 'utf8')), null, 2));
  } catch (e) { console.error(e.message); process.exitCode = 1; }
}
