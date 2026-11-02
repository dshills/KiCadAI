// Read-only qualification receipt checks. Local hashes are not independent
// attestation; actual user approval and exact-source CI/review remain separate.
import path from 'node:path';
import assert from 'node:assert/strict';
import {hash,read,authenticateFiles} from '../evaluation/acceptance-lib.mjs';

export const qualificationVersion='indexed-production-qualification-1';
export const requiredCommands=['build-production','build-test-helper','unit-safeguards','go-short','go-race','lint','go-process-integration','native-bmp280','native-sht31','historical-authentication','export-indexed-contract'];

export function verifyQualification(m) {
  const binding=m.production_qualification;
  assert.ok(binding&&typeof binding.path==='string','production qualification is required');
  assert.equal(m.runtime_files_sha256[binding.path],binding.sha256,'qualification missing from runtime freeze');
  assert.equal(hash(binding.path),binding.sha256,'qualification changed');
  const q=read(binding.path);
  assessQualification(m,q);
  assert.equal(hash(q.compiler.path),q.compiler.sha256);
  authenticateFiles('.',q.source_sha256);
  for(const c of q.commands) {
    for(const suffix of ['process.json','stdout.log','stderr.log']) {
      const file=`${q.directory}/${c.id}.${suffix}`;
      assert.equal(hash(file),m.runtime_files_sha256[file],'qualification process evidence changed');
    }
    assert.deepEqual(read(`${q.directory}/${c.id}.process.json`),c.result);
  }
  assert.equal(q.contract_sha256,hash(m.scoring_contract_path));
  return q;
}

// Pure contract checks, separately testable without a macOS compiler on Linux.
// Filesystem authentication remains mandatory in verifyQualification above.
export function assessQualification(m,q) {
  assert.equal(q.version,qualificationVersion);assert.equal(q.status,'passed-offline-live-not-authorized');
  assert.equal(q.evaluation_id,m.evaluation_id);assert.equal(q.live_requests,0);
  assert.equal(q.binary,m.binary);assert.equal(q.binary_sha256,m.binary_sha256);
  assert.equal(q.node_sha256,m.node_sha256);assert.equal(q.kicad_cli_sha256,m.kicad_cli_sha256);
  assert.equal(q.compiler.version,'go1.26.8');
  assert.equal(q.compiler.sha256,'2ebc27dd4e38e9b86a9f41df0307785f4f7e2997e4be761a7b4af04b41a0de57');
  assert.equal(q.plan_sha256,m.evaluation_plan_sha256);
  assert.ok(Object.keys(q.source_sha256).length>0);
  for(const [file,sha] of Object.entries(q.source_sha256))assert.equal(m.runtime_files_sha256[file],sha,'source missing from runtime freeze');
  assert.deepEqual(q.commands.map(c=>c.id),requiredCommands);
  for(const c of q.commands) {
    const p=c.result;
    assert.equal(p.child_terminal_observed,true);assert.equal(p.exit_code,0);
    assert.equal(p.signal,null);assert.equal(p.timed_out,false);assert.equal(p.spawn_error,null);
    assert.equal(p.log_overflow,false);assert.equal(p.storage_error,false);
    assert.ok(Number.isFinite(p.wall_seconds)&&p.wall_seconds>=0);
    assert.ok(typeof c.executable==='string'&&c.executable&&Array.isArray(c.args));
  }
  assert.deepEqual(q.native.map(c=>c.id),['bmp280-standard','sht31-standard']);
  assert.deepEqual(q.native.map(c=>Object.keys(c.comparison.files_compared).length),[39,42]);
  assert.equal(m.runtime_files_sha256[path.relative(process.cwd(),m.binary)],m.binary_sha256);
  return q;
}
