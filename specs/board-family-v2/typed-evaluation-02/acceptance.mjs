// Evaluator-only logic. Gold requirements are never part of a provider payload.
import assert from 'node:assert/strict';
import {isDeepStrictEqual as equal} from 'node:util';
import {assessDecision} from '../evaluation/acceptance-lib.mjs';

export const E = 'specs/board-family-v2/typed-evaluation-02';
export const evaluationID = 'board-family-v2-typed-final-02';
export const policy = {goal: evaluationID, max_requests: 14, max_micro_usd: 1_000_000};
export const model = 'gpt-4.1-mini-2025-04-14';
export const admissionVersion = 'typed-requirements-02';
export const batch = '.cache/board-family-v2/typed-evaluation-02-final';
export const ledgerFile = `${batch}-ledger.json`;
export const runtimeFile = `${E}/runtime-02.json`;
export const freezeFile = `${E}/freeze-02.json`;
export const approvalFile = `${E}/APPROVAL-02.json`;
export const cli = '/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli';

export function validateCases(spec) {
  assert.equal(spec.evaluation_id, evaluationID);
  assert.equal(spec.cases.length, 14);
  assert.deepEqual(spec.cases.map(c => c.id), ['useful-01','useful-02','useful-03','useful-04','useful-05','choice-01','choice-02','choice-03','refuse-01','refuse-02','refuse-03','refuse-04','refuse-05','refuse-06']);
  assert.deepEqual(spec.cases.map(c => c.expected_disposition), [...Array(5).fill('supported'), ...Array(3).fill('clarify'), ...Array(6).fill('unsupported')]);
  for (const c of spec.cases) {
    assert.ok(typeof c.prompt === 'string' && Buffer.byteLength(c.prompt) > 0 && Buffer.byteLength(c.prompt) <= 2000);
    assert.ok(typeof c.review === 'string' && c.review.trim());
    assert.ok(Array.isArray(c.requirements));
    assert.equal(new Set(c.requirements.map(r => r.id)).size, c.requirements.length);
    for (const r of c.requirements) {
      assert.match(r.id, /^[a-zA-Z0-9-]+$/);
      assert.ok(r.description && Array.isArray(r.any_of) && r.any_of.length > 0);
      for (const m of r.any_of) {
        assert.ok(['sensor','measurement','profile','feature','number'].includes(m.kind));
        assert.ok(m.value);
        assert.ok(m.kind === 'number' ? Number.isFinite(m.number) : ['required','not_required','forbidden','uncertain'].includes(m.state));
      }
    }
  }
  return spec;
}

// The frozen provider schema uses only this small closed-object/anyOf subset.
// This checks structure independently; it does not prove English semantics.
export function conforms(value, schema) {
  if (schema.anyOf) return schema.anyOf.some(s => conforms(value, s));
  if (schema.enum && !schema.enum.some(v => equal(v, value))) return false;
  switch (schema.type) {
    case 'object': {
      if (!value || typeof value !== 'object' || Array.isArray(value)) return false;
      if (schema.additionalProperties !== false || !Array.isArray(schema.required)) return false;
      const properties = schema.properties;
      if (!properties || !equal([...schema.required].sort(), Object.keys(properties).sort())) return false;
      return equal(Object.keys(value).sort(), Object.keys(properties).sort()) && Object.entries(properties).every(([key,s]) => conforms(value[key],s));
    }
    case 'array': return Array.isArray(value) && value.every(v => conforms(v, schema.items));
    case 'string': return typeof value === 'string';
    case 'integer': return Number.isSafeInteger(value);
    case 'number': return typeof value === 'number' && Number.isFinite(value);
    case 'null': return value === null;
    default: return false;
  }
}

const matches = (f, r) => r.any_of.some(m => Object.entries(m).every(([key,v]) => equal(f[key],v)));
export function assessRaw(c, selection, schema) {
  const raw = selection?.raw_intent, clauses = selection?.request_clauses;
  let structure = selection?.admission_version === admissionVersion && selection?.original_request === c.prompt && !selection?.raw_decision;
  structure = structure && Array.isArray(clauses) && clauses.length > 0 && clauses.length <= 32 && clauses.every((x,i) => x && equal(Object.keys(x).sort(), ['id','text']) && x.id === i && typeof x.text === 'string') && clauses.map(x => x.text).join('') === c.prompt;
  structure = Boolean(structure && conforms(raw,schema) && raw.version === '2' && raw.clauses.length === clauses.length && new Set(raw.clauses.map(x => x.id)).size === clauses.length);
  const facts = [];
  if (structure) {
    for (const clause of raw.clauses) {
      if (!Number.isSafeInteger(clause.id) || clause.id < 0 || clause.id >= clauses.length || clause.facts.length < 1 || clause.facts.length > 24) { structure = false; break; }
      for (const [index,f] of clause.facts.entries()) {
        if (f.kind !== 'none' && (!f.quote?.trim() || !clauses[clause.id].text.includes(f.quote))) structure = false;
        facts.push({fact:f, clause_id:clause.id, fact_index:index});
      }
    }
    if (facts.length > 64) structure = false;
  }
  const requirements = c.requirements.map(r => ({id:r.id, matches: facts.filter(f => matches(f.fact,r)).map(({clause_id,fact_index}) => ({clause_id,fact_index}))}));
  const complete = requirements.every(r => r.matches.length > 0);
  return {structure_pass:structure, required_facts_present:complete, requirements,
    automatic_checks_pass:structure && complete, semantic_review:'pending-source-bound-review'};
}

export function assessSelection(spec, c, selection, schema) {
  return {raw:assessRaw(c,selection,schema), admitted:assessDecision(spec,c,selection?.decision)};
}

export function checkLedger(ledger, prefix = []) {
  assert.equal(ledger.version,2);
  for (const [key,value] of Object.entries(policy)) assert.equal(ledger[key],value);
  assert.ok(!ledger.halt_reason, 'ledger halted');
  assert.ok(Array.isArray(ledger.entries) && ledger.entries.length <= policy.max_requests);
  assert.ok(ledger.entries.length * 50_000 <= policy.max_micro_usd);
  assert.ok(ledger.entries.length >= prefix.length, 'ledger truncated');
  assert.deepEqual(ledger.entries.slice(0,prefix.length),prefix,'previous reservations changed');
  const responseIDs=new Set();
  for (const [i,e] of ledger.entries.entries()) {
    assert.equal(e.index,i+1); assert.equal(e.model,model); assert.equal(e.reserve_micro_usd,50_000);
    assert.ok(['reserved_unknown_outcome','failed_or_unknown','completed'].includes(e.status));
    if (e.status === 'completed') {
      assert.ok(typeof e.response_id === 'string' && e.response_id);
      assert.equal(responseIDs.has(e.response_id),false,'duplicate completed response identity');responseIDs.add(e.response_id);
      assert.ok(Number.isSafeInteger(e.input_tokens) && e.input_tokens > 0);
      assert.ok(Number.isSafeInteger(e.output_tokens) && e.output_tokens > 0 && e.output_tokens <= 1600);
      assert.equal(e.estimated_micro_usd,Math.ceil((e.input_tokens*4 + e.output_tokens*16)/10));
      assert.ok(e.estimated_micro_usd <= 50_000);
    }
  }
  return ledger.entries;
}

export function checkApproval(a, runtimeHash, freezeHash, resume = false) {
  assert.equal(a.status,'explicit-user-approved'); assert.equal(a.evaluation_id,evaluationID);
  assert.equal(a.max_physical_requests,policy.max_requests); assert.equal(a.max_micro_usd,policy.max_micro_usd);
  assert.equal(a.existing_key_only,true); assert.equal(a.runtime_sha256,runtimeHash); assert.equal(a.freeze_sha256,freezeHash);
  assert.ok(typeof a.user_message === 'string' && a.user_message.trim());
  assert.ok(typeof a.recorded_utc === 'string' && Number.isFinite(Date.parse(a.recorded_utc)));
  if (resume) assert.equal(a.allow_unattempted_resume,true,'continuation not included in recorded approval');
}

export function verifyResume(state, spec, entries, alive) {
  assert.equal(state.evaluation_id,evaluationID); assert.equal(state.status,'stopped');
  assert.ok(Number.isSafeInteger(state.owner_pid) && state.owner_pid > 0);
  assert.equal(alive(state.owner_pid),false,'prior runner is still live');
  assert.ok(state.records.length > 0 && state.records.length < spec.cases.length);
  assert.deepEqual(state.records.map(r => r.id),spec.cases.slice(0,state.records.length).map(c => c.id));
  assert.ok(state.records.every(r => r.state === 'finished' && r.child_terminal_observed === true),'unobserved terminal child; do not restart');
  assert.deepEqual(entries,state.ledger_entries,'ledger changed since terminal child observation');
  return spec.cases.slice(state.records.length);
}

export function metrics(spec,records) {
  const supported = spec.cases.filter(c => c.expected_disposition === 'supported');
  const times = supported.map(c => records.find(r => r.id === c.id)?.wall_seconds);
  const allKnown = times.every(t => Number.isFinite(t) && t >= 0);
  const sorted = allKnown ? [...times].sort((a,b) => a-b) : [];
  return {planned_cases:spec.cases.length, attempted_cases:records.length, recorded_outcomes:records.filter(r => r.state === 'finished').length,
    raw_automatic_passes:records.filter(r => r.raw?.automatic_checks_pass).length,
    application_automatic_passes:records.filter(r => !r.execution_or_evidence_failure && r.admitted?.automatic_checks_pass && r.output_checks_pass).length,
    median_all_useful_seconds:allKnown ? sorted[2] : null, maximum_all_useful_seconds:allKnown ? sorted[4] : null,
    timing_target_met:allKnown && sorted[2] < 60 && sorted[4] < 120, acceptance:'not-established-by-automatic-scores'};
}

export function checkSemanticReview(spec,state,review,bindings,selections) {
  assert.equal(review.status,'source-bound-semantic-review'); assert.equal(review.evaluation_id,evaluationID);
  for (const [key,value] of Object.entries(bindings)) if (key !== 'prompt_hashes') assert.equal(review[key],value);
  assert.ok(['implementing-agent','independent-human','independent-agent'].includes(review.reviewer?.kind));
  assert.ok(typeof review.reviewer?.name === 'string' && review.reviewer.name.trim());
  assert.deepEqual(review.cases.map(c => c.id),spec.cases.map(c => c.id));
  return spec.cases.map((c,i) => {
    const r=review.cases[i], record=state.records.find(x => x.id===c.id), evidence=selections[c.id];
    assert.equal(r.prompt_sha256,bindings.prompt_hashes[c.id]);
    assert.equal(r.selection_sha256,evidence?.sha256 ?? null);
    for (const key of ['raw_complete','raw_correct','decision_correct']) assert.equal(typeof r[key],'boolean');
    assert.ok(typeof r.notes==='string' && r.notes.trim());
    assert.deepEqual(r.requirements.map(x=>x.id),c.requirements.map(x=>x.id));
    for (const [j,rr] of r.requirements.entries()) {
      assert.equal(typeof rr.passed,'boolean'); assert.ok(rr.note?.trim()); assert.ok(Array.isArray(rr.evidence));
      if (rr.passed) {
        assert.ok(rr.evidence.length>0);
        assert.ok(rr.evidence.some(p => {
          const f=evidence?.selection?.raw_intent?.clauses?.find(x=>x.id===p.clause_id)?.facts?.[p.fact_index];
          return f && matches(f,c.requirements[j]);
        }), 'review evidence does not support the expected requirement');
      }
    }
    const rawPass=Boolean(record?.raw?.automatic_checks_pass && r.raw_complete && r.raw_correct && r.requirements.every(x=>x.passed));
    const applicationPass=Boolean(!record?.execution_or_evidence_failure && record?.admitted?.automatic_checks_pass && record?.output_checks_pass && r.decision_correct);
    return {id:c.id,raw_pass:rawPass,application_pass:applicationPass,complete_pass:rawPass && applicationPass};
  });
}
