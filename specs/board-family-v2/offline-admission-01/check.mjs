// Authenticate the published offline correction. No commands or network access.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import assert from 'node:assert/strict';
const base = 'specs/board-family-v2/offline-admission-01';
const current = process.argv[2] === '--current';
assert.ok(process.argv.length === 2 || process.argv.length === 3 && current);
const hash = f => crypto.createHash('sha256').update(fs.readFileSync(f)).digest('hex');
const read = f => JSON.parse(fs.readFileSync(f, 'utf8'));
const publication = read(`${base}/publication.json`);
assert.equal(publication.status, 'offline-correction-published-not-live-acceptance');
for (const [file, sha] of Object.entries(publication.files_sha256)) {
  assert.ok(!path.isAbsolute(file) && !file.split('/').includes('..'));
  assert.equal(hash(`${base}/${file}`), sha, `changed publication: ${file}`);
}
const r = read(`${base}/verification/receipt.json`);
assert.equal(r.status, 'offline-development-pass-not-live-acceptance');
assert.equal(r.new_api_requests, 0);
assert.equal(r.new_api_spend_usd, 0);
assert.equal(r.provider_credentials_removed, true);
assert.equal(r.seen_response_replay_passes, 14);
assert.equal(r.recorded_cases, 14);
assert.equal(r.commands.length, 12);
for (const c of r.commands) {
  assert.equal(c.exitCode, 0);
  assert.equal(c.signal, null);
  assert.equal(c.timedOut, false);
  assert.equal(c.spawnError, null);
  assert.equal(hash(`${base}/verification/${c.log}`), c.log_sha256);
}
assert.equal(hash(`${base}/verification/admission.cover`), r.coverage_sha256);
assert.equal(r.native_smoke.length, 2);
let comparisons = 0;
for (const c of r.native_smoke) {
  assert.equal(c.checks_passed, 14);
  assert.equal(c.validation.checks.length, 14);
  assert.ok(c.validation.passed && c.validation.checks.every(x => x.passed));
  comparisons += Object.keys(c.comparison.files_compared).length;
}
assert.equal(comparisons, 81);
// Historical bytes can be checked in any checkout; cache-only execution records
// and the original binary require the originating workspace (--current).
for (const [file, sha] of Object.entries(r.historical_bytes_unchanged_sha256)) {
  if (current || !file.startsWith('.cache/')) assert.equal(hash(file), sha, `changed historical record: ${file}`);
}
if (current) {
  for (const [file, sha] of Object.entries(r.source_sha256)) assert.equal(hash(file), sha, `source is not qualified revision: ${file}`);
  assert.equal(hash(r.binary_path), r.binary_sha256);
  for (const c of r.native_smoke) {
    for (const [file, sha] of Object.entries(c.comparison.all_files_sha256)) assert.equal(hash(`${c.directory}/${file}`), sha, `changed smoke artifact: ${file}`);
  }
}
console.log(JSON.stringify({status: publication.status, current_source_and_cache_checked: current,
  published_files: Object.keys(publication.files_sha256).length, seen_response_replays: 14, native_comparisons: comparisons,
  live_acceptance: 'historical failure unchanged; no new live evaluation'}));
