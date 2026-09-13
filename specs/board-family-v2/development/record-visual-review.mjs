// Bind the implementing agent's recorded visual observations to exact review
// inputs and images. Authentication does not itself perform visual inspection.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import assert from 'node:assert/strict';
const output = process.argv[2];
if (!/^specs\/board-family-v2\/evidence\/visual-review-[0-9]+\.json$/.test(output || '') || fs.existsSync(output)) throw new Error('new visual-review evidence path required');
const hash = p => crypto.createHash('sha256').update(fs.readFileSync(p)).digest('hex');
const read = p => JSON.parse(fs.readFileSync(p, 'utf8'));
const replayPath = 'specs/board-family-v2/evidence/deterministic-replay-01.json';
const replay = read(replayPath);
const reviews = [];
for (const family of ['sht31', 'bmp280']) {
  const dir = `.cache/board-family-v2/visual-review-07-${family}`;
  const receipt = read(`${dir}/receipt.json`);
  assert.equal(receipt.source, `.cache/board-family-v2/integration-06-${family}-standard`);
  assert.equal(receipt.recipe_sha256, hash('specs/board-family-v2/development/render-manufacturing.mjs'));
  for (const [root, hashes] of [[receipt.source, receipt.source_sha256], [dir, receipt.files_sha256]]) {
    for (const [file, sha] of Object.entries(hashes)) {
      if (file.includes('..') || path.isAbsolute(file)) throw new Error('unexpected evidence path');
      assert.equal(hash(`${root}/${file}`), sha, `review artifact changed: ${root}/${file}`);
    }
  }
  const group = replay.cases.filter(c => c.id.startsWith(family));
  const nonPlacement = Object.keys(group[0].files).filter(f => f.startsWith('manufacturing/') && !f.endsWith('placement.csv'));
  assert.equal(nonPlacement.length, 15);
  for (const file of nonPlacement) for (const c of group) assert.equal(c.files[file].compared_sha256, group[0].files[file].compared_sha256, `profile geometry differs: ${file}`);
  reviews.push({ family, directory: dir, receipt_sha256: hash(`${dir}/receipt.json`), source_directory: receipt.source, source_sha256: receipt.source_sha256, reviewed_images_sha256: Object.fromEntries(Object.entries(receipt.files_sha256).filter(([f]) => f.endsWith('.png'))), reusable_profiles: group.map(c => c.id), profile_identical_nonplacement_exports: nonPlacement, renderer: receipt.renderer, rasterizer: receipt.rasterizer, warnings_logs_sha256: Object.fromEntries(Object.entries(receipt.files_sha256).filter(([f]) => f.endsWith('.stderr') && fs.statSync(`${dir}/${f}`).size)) });
}
fs.writeFileSync(output, JSON.stringify({ status: 'software-export-visual-review-with-disclosed-limitations', reviewer: 'implementing Codex agent; not an independent engineer', created_utc: new Date().toISOString(), review_notes_sha256: hash('specs/board-family-v2/MANUFACTURING_REVIEW.md'), replay_evidence_sha256: hash(replayPath), recipe_sha256: hash(process.argv[1]), reviews, remaining: ['complete consolidated SHT31 electrical/assembly review', 'publish final examples and acceptance evidence', 'newly budgeted final live language evaluation', 'final PR review and updated CI', 'separately authorized physical fabrication/bring-up'] }, null, 2) + '\n', { flag: 'wx' });
console.log(JSON.stringify({ output, families_reviewed: reviews.length, reviewed_images: reviews.reduce((n, r) => n + Object.keys(r.reviewed_images_sha256).length, 0) }));
