// Portable published-evidence authentication; --current also binds local source/cache.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import assert from 'node:assert/strict';
const base = 'specs/board-family-v2/typed-intent-02';
const current = process.argv[2] === '--current';
assert.ok(process.argv.length === 2 || process.argv.length === 3 && current);
const hash = f => crypto.createHash('sha256').update(fs.readFileSync(f)).digest('hex');
const read = f => JSON.parse(fs.readFileSync(f, 'utf8'));
const publication = read(`${base}/publication.json`);
assert.equal(publication.status, 'offline-typed-intent-published-not-live-acceptance');
for (const [file, sha] of Object.entries(publication.files_sha256)) {
  assert.ok(!path.isAbsolute(file) && !file.split('/').includes('..'));
  assert.equal(hash(`${base}/${file}`), sha, `changed publication: ${file}`);
}
const r = read(`${base}/verification/receipt.json`);
assert.equal(r.status, 'offline-typed-intent-pass-not-live-acceptance');
assert.equal(r.new_api_requests, 0); assert.equal(r.new_api_spend_usd, 0);
assert.equal(r.provider_credentials_removed, true);
assert.equal(r.synthetic_wording_passes, 25); assert.equal(r.synthetic_frozen_prompt_passes, 14);
assert.equal(r.legacy_seen_response_passes, 14); assert.equal(r.commands.length, 13);
for (const c of r.commands) {
  assert.equal(c.exitCode, 0); assert.equal(c.signal, null);
  assert.equal(c.timedOut, false); assert.equal(c.spawnError, null);
  assert.equal(hash(`${base}/verification/${c.log}`), c.log_sha256);
}
assert.equal(hash(`${base}/verification/intent.cover`), r.coverage_sha256);
assert.equal(hash(`${base}/verification/contract.json`), r.successor_contract_sha256);
assert.equal(r.native_smoke.length, 2);
let comparisons = 0;
for (const c of r.native_smoke) {
  assert.equal(c.checks_passed, 14); assert.equal(c.validation.checks.length, 14);
  assert.ok(c.validation.passed && c.validation.checks.every(x => x.passed));
  comparisons += Object.keys(c.comparison.files_compared).length;
}
assert.equal(comparisons, 81);
assert.equal(hash(`${base}/usability-01/results.json`), '4a378b90dd9db912f005aa952c9867ab9ad26bb36c98891084b4d6045e7df5d2');
for (const [file, sha] of Object.entries(r.historical_bytes_unchanged_sha256)) {
  if (current || !file.startsWith('.cache/')) assert.equal(hash(file), sha, `changed historical record: ${file}`);
}
if (current) {
  for (const [file, sha] of Object.entries(r.source_sha256)) assert.equal(hash(file), sha, `unqualified source: ${file}`);
  assert.equal(hash(r.binary_path), r.binary_sha256);
  for (const c of r.native_smoke) {
    for (const [file, sha] of Object.entries(c.comparison.all_files_sha256)) assert.equal(hash(`${c.directory}/${file}`), sha);
  }
}
console.log(JSON.stringify({status: publication.status, current_source_and_cache_checked: current,
  published_files: Object.keys(publication.files_sha256).length, synthetic_wording_passes: 25,
  synthetic_frozen_prompt_passes: 14, legacy_seen_response_passes: 14, native_comparisons: comparisons,
  live_acceptance: 'not run; historical failure unchanged'}));
