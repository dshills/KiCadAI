// Copy completed local evidence only. Existing publication/evidence is immutable.
import fs from 'node:fs';
import crypto from 'node:crypto';
import assert from 'node:assert/strict';
const base = 'specs/board-family-v2/typed-intent-02', out = process.argv[2];
assert.ok(process.argv.length === 3 && /^\.cache\/board-family-v2\/typed-intent-02-run-[0-9]+$/.test(out || ''));
const hash = f => crypto.createHash('sha256').update(fs.readFileSync(f)).digest('hex');
const r = JSON.parse(fs.readFileSync(`${out}/receipt.json`, 'utf8'));
assert.equal(r.status, 'offline-typed-intent-pass-not-live-acceptance');
assert.ok(!fs.existsSync(`${base}/publication.json`) && !fs.existsSync(`${base}/verification`));
for (const [file, sha] of Object.entries(r.source_sha256)) assert.equal(hash(file), sha);
fs.mkdirSync(`${base}/verification`);
for (const file of ['receipt.json', 'contract.json', 'intent.cover', ...r.commands.map(c => c.log)]) {
  assert.ok(!file.includes('/') && !file.includes('..'));
  fs.copyFileSync(`${out}/${file}`, `${base}/verification/${file}`, fs.constants.COPYFILE_EXCL);
}
const files = ['README.md', 'REVIEW.md', 'verify.mjs', 'check.mjs', 'publish.mjs',
  ...fs.readdirSync(`${base}/verification`).sort().map(f => `verification/${f}`),
  ...fs.readdirSync(`${base}/usability-01`).sort().map(f => `usability-01/${f}`)];
fs.writeFileSync(`${base}/publication.json`, JSON.stringify({status: 'offline-typed-intent-published-not-live-acceptance',
  source_run: out, published_utc: new Date().toISOString(), files_sha256: Object.fromEntries(files.map(f => [f, hash(`${base}/${f}`)]))}, null, 2) + '\n', {flag: 'wx'});
console.log(`published ${files.length} source-bound evidence files; no new live result`);
