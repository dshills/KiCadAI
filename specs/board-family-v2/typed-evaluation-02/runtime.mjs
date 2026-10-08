// Read-only identities shared by the runner and final audit.
import fs from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
import {hash,read,authenticateFiles,authenticateExamples} from '../evaluation/acceptance-lib.mjs';
import {E,evaluationID,policy,model,admissionVersion,runtimeFile,freezeFile,cli,validateCases} from './acceptance.mjs';

export function authenticatePlan() {
  const freeze=read(freezeFile);
  assert.equal(freeze.status,'frozen-not-spending-authority');
  assert.equal(freeze.evaluation_id,evaluationID);
  authenticateFiles('.',freeze.files_sha256);
  const spec=validateCases(read(`${E}/cases-02.json`));
  assert.deepEqual(read(`${E}/budget-02.json`),policy);
  const contract=read(`${E}/LIVE_CONTRACT-02.json`);
  assert.equal(contract.admission_version,admissionVersion);
  assert.equal(contract.model,model);
  assert.equal(contract.max_output_tokens,1600);
  assert.equal(contract.destination,'https://api.openai.com/v1/responses');
  return {freeze,spec,contract};
}

export function authenticateRuntime() {
  const result=authenticatePlan(), runtime=read(runtimeFile);
  assert.equal(runtime.status,'offline-qualified-live-not-authorized');
  assert.equal(runtime.evaluation_id,evaluationID);
  assert.equal(runtime.freeze_sha256,hash(freezeFile));
  assert.match(runtime.binary,/^\.cache\/board-family-v2\/typed-runtime-02-[0-9]+\/kicadai-board-family$/);
  assert.ok(fs.lstatSync(runtime.binary).isFile());
  authenticateFiles('.',runtime.files_sha256);
  assert.equal(hash(runtime.binary),runtime.binary_sha256);
  assert.equal(runtime.files_sha256[runtime.binary],runtime.binary_sha256);
  assert.equal(runtime.kicad_cli,cli);
  assert.equal(hash(cli),runtime.kicad_cli_sha256);
  assert.equal(hash(process.execPath),runtime.node_sha256);
  assert.equal(path.resolve(runtime.node_path),path.resolve(process.execPath));
  assert.equal(runtime.qualification_reuse.source_binary_sha256,runtime.binary_sha256);
  assert.equal(runtime.live_requests,0);
  assert.ok(runtime.commands.every(c=>c.exit_code===0 && c.signal===null && c.child_terminal_observed && !c.timed_out && !c.spawn_error));
  for (const [file,sha] of Object.entries(runtime.historical_bytes_sha256)) assert.equal(hash(file),sha,`historical state changed: ${file}`);
  const examples=authenticateExamples();
  return {...result,runtime,...examples};
}
