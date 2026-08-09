# Closed-Loop Open-Set Capability Expansion V2 Baseline Protocol

Status: baseline complete; held-out evidence sealed pending implementation freeze

## 1. Preconditions

The baseline may start only after one commit contains:

- `V2_SPEC_ADDENDUM.md`, this protocol, and the inherited corpus rules;
- the 24 independently authored requirement files and authorship statement;
- the V2 manifest, requirement SHA-256 values, manifest checksum, role order,
  reporting domains, safety declarations, source IDs, and authoring provenance;
- the frozen evaluator/ranking policy, impact registry, synthesis policy,
  environment contract, and starting commit
  `8bdc31e668152b7324066bd75182d86d7320d3f8`; and
- automated strict-decode, neutrality, collision, diversity, mutation,
  reorder, role-leakage, and identity-leakage tests.

No outcome-changing production file may differ from the starting commit before
this freeze commit is created.

## 2. Execution order

1. Create two clean evaluation roots.
2. Run all 12 discovery requirements through normal production synthesis in
   manifest order in both roots.
3. Require exact JSON-byte equality for the two synthesis runs of every case.
4. Promote every discovery pass twice through the normal physical and
   installed-KiCad path.
5. Classify each discovery case exactly as `pass`, `unsupported`, `unsafe`, or
   `exhausted` from authoritative evidence.
6. Complete the held-out baseline through the same automated harness, but seal
   requirement content, case outcomes, gaps, clusters, and promotion evidence
   from the implementation agent.
7. Abort rather than record a product outcome for malformed evidence, tool
   crashes, nondeterminism, missing required installed tooling, artifact
   corruption, or policy/hash drift.

## 3. Discovery report and selection

Convert discovery failures into the seven typed scopes from `SPEC.md`, retain
the earliest authoritative cause and suppressed downstream symptoms, then
cluster and rank with the unchanged identity-neutral tuple. The rank-one
selection record must bind:

- corpus manifest and baseline hashes;
- starting and freeze commit hashes;
- evaluator, impact-registry, synthesis-policy, catalog, model-registry, and
  environment hashes;
- complete ranking tuple and tie-break evidence;
- affected discovery IDs, domains, analysis kinds, safety score, and required
  evidence; and
- the existing capability-expansion plan hash.

If rank one is not actionable through the existing planner, baseline closeout
fails. Rank two may not be substituted.

## 4. Artifacts

The freeze and baseline commits must contain:

```text
internal/capabilityfeedback/testdata/closed_loop_open_set_v2_corpus/
  manifest.json
  manifest.sha256
  AUTHORSHIP.md
  discovery/request_001.json ... request_012.json
  held_out/request_013.json ... request_024.json
internal/capabilityfeedback/testdata/closed_loop_open_set_v2_baseline/
  v2_case_001.json ... v2_case_024.json
specs/closed-loop-open-set-capability-expansion/
  V2_BASELINE_REPORT.json
  V2_BASELINE_REPORT.sha256
  V2_SELECTION.json
  V2_SELECTION.sha256
  V2_BASELINE_AUDIT.md
```

Held-out per-case evidence may be stored for reproducibility only if the test
harness prevents it from entering selection, implementation decisions, logs,
or agent-visible summaries before the production diff is sealed.

## 5. Baseline closeout

Before implementation begins, prove:

- both clean-root runs are byte-identical;
- all 24 outcomes are closed and evidence-backed;
- every baseline pass has complete promotion evidence;
- discovery alone reproduces the ranked report and selection;
- held-out evidence cannot influence discovery ranking under mutation tests;
- all artifact and source hashes reproduce; and
- the corpus freeze and baseline are committed separately from the selected
  capability implementation.
