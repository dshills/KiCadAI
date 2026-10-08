// Bind the implementing agent's recorded visual observations to exact review
// inputs and images. Authentication does not itself perform visual inspection.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import assert from 'node:assert/strict';
import { normalizeReplay } from './replay-normalization.mjs';
const output = process.argv[2];
const assembly = process.argv[3] === 'assembly';
if (process.argv[3] && !assembly) throw new Error('unknown review mode');
if (!/^specs\/board-family-v2\/evidence\/visual-review-[0-9]+\.json$/.test(output || '') || fs.existsSync(output)) throw new Error('new visual-review evidence path required');
const hash = p => crypto.createHash('sha256').update(fs.readFileSync(p)).digest('hex');
const read = p => JSON.parse(fs.readFileSync(p, 'utf8'));
const replayPath = `specs/board-family-v2/evidence/deterministic-replay-${assembly ? '02' : '01'}.json`;
const replay = read(replayPath);
const reviews = [];
for (const family of ['sht31', 'bmp280']) {
  const dir = `.cache/board-family-v2/visual-review-${assembly && family === 'sht31' ? '08' : '07'}-${family}`;
  const receipt = read(`${dir}/receipt.json`);
  assert.equal(receipt.source, `.cache/board-family-v2/integration-${assembly && family === 'sht31' ? '10' : '06'}-${family}-standard`);
  assert.equal(receipt.recipe_sha256, hash('specs/board-family-v2/development/render-manufacturing.mjs'));
  for (const [root, hashes] of [[receipt.source, receipt.source_sha256], [dir, receipt.files_sha256]]) {
    for (const [file, sha] of Object.entries(hashes)) {
      if (file.includes('..') || path.isAbsolute(file)) throw new Error('unexpected evidence path');
      assert.equal(hash(`${root}/${file}`), sha, `review artifact changed: ${root}/${file}`);
    }
  }
  const group = replay.cases.filter(c => c.id.startsWith(family));
  // Bind even a reused older visual review to every current standard-profile
  // deliverable. This compares actual content, not just family names or shapes.
  for (const [file, expected] of Object.entries(group[0].files)) {
    const reviewed = normalizeReplay(file, fs.readFileSync(`${receipt.source}/${file}`));
    assert.equal(crypto.createHash('sha256').update(reviewed.content).digest('hex'), expected.compared_sha256, `reviewed source differs from current replay: ${family}/${file}`);
    const current = fs.readFileSync(`${group[0].right_directory}/${file}`);
    assert.equal(crypto.createHash('sha256').update(current).digest('hex'), expected.right_raw_sha256, `current replay artifact changed: ${file}`);
  }
  const nonPlacement = Object.keys(group[0].files).filter(f => f.startsWith('manufacturing/') && !f.endsWith('placement.csv'));
  assert.equal(nonPlacement.length, 15);
  for (const file of nonPlacement) for (const c of group) assert.equal(c.files[file].compared_sha256, group[0].files[file].compared_sha256, `profile geometry differs: ${file}`);
  reviews.push({ family, directory: dir, receipt_sha256: hash(`${dir}/receipt.json`), source_directory: receipt.source, source_sha256: receipt.source_sha256, reviewed_images_sha256: Object.fromEntries(Object.entries(receipt.files_sha256).filter(([f]) => f.endsWith('.png'))), reusable_profiles: group.map(c => c.id), profile_identical_nonplacement_exports: nonPlacement, renderer: receipt.renderer, rasterizer: receipt.rasterizer, warnings_logs_sha256: Object.fromEntries(Object.entries(receipt.files_sha256).filter(([f]) => f.endsWith('.stderr') && fs.statSync(`${dir}/${f}`).size)) });
}
const notes = `specs/board-family-v2/${assembly ? 'ELECTRICAL_ASSEMBLY_REVIEW' : 'MANUFACTURING_REVIEW'}.md`;
const remaining = ['publish final examples and acceptance evidence', 'newly budgeted final live language evaluation', 'final PR review and updated CI', 'separately authorized physical fabrication/bring-up'];
if (!assembly) remaining.unshift('complete consolidated SHT31 electrical/assembly review');
fs.writeFileSync(output, JSON.stringify({ status: 'software-export-visual-review-with-disclosed-limitations', reviewer: 'implementing Codex agent; not an independent engineer', created_utc: new Date().toISOString(), review_notes_path: notes, review_notes_sha256: hash(notes), replay_evidence_sha256: hash(replayPath), recipe_sha256: hash(process.argv[1]), reviews, remaining }, null, 2) + '\n', { flag: 'wx' });
console.log(JSON.stringify({ output, families_reviewed: reviews.length, reviewed_images: reviews.reduce((n, r) => n + Object.keys(r.reviewed_images_sha256).length, 0) }));
