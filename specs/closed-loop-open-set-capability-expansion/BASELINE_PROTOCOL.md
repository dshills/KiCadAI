# Closed-Loop Open-Set Baseline Protocol

Status: version-1 normative contract frozen; no baseline has been recorded yet

## Purpose

The baseline must capture what the current production system can prove before a
ranked capability changes production behavior. It is an immutable experimental
record, not a hand-edited expectation file.

## Preconditions

1. The corpus, manifest, authorship record, evaluator policy, impact registry,
   acceptance profile, and work budgets are frozen and committed.
2. The worktree is clean and the freeze commit is recorded in the baseline.
3. The evidence adapter, report codec, classifier, clustering, ranking, and
   aggregate planner have generic tests and are frozen before case outcomes are
   inspected.
4. No architecture, component, model, simulation, physical, routing, writer, or
   verification change intended to improve a corpus case is present.
5. Installed KiCad, symbol roots, footprint roots, Go version, OS/architecture,
   and relevant tool versions are recorded.

## Execution

For every case in authoritative manifest order:

1. Strictly decode and normalize the requirement.
2. Verify the requirement and corpus hashes.
3. Run synthesis twice with identical immutable inventory, catalog, model
   policy, ranking policy, and budgets.
4. Require byte-identical normalized synthesis evidence.
5. If and only if synthesis is physically ready, run the normal physical
   promotion twice from separate clean roots.
6. Require complete applicable analyses, connectivity, route completion,
   writer correctness, clean installed-KiCad ERC and strict DRC, zero normalized
   round-trip differences, and identical project bytes for `pass`.
7. Adapt non-passing evidence to exactly one terminal outcome and one or more
   independent root gaps; retain downstream symptoms without ranking them.
8. Write raw case evidence and its SHA-256 before moving to the next case.

Run the complete corpus twice from separate clean output roots. Aggregate
reports must be byte-identical.

## Classification Checks

- `pass` requires all requested gates; a writable project or passing simulation
  alone is insufficient.
- `unsupported` requires a structured unavailable capability or missing
  reviewed evidence, not generic failure prose.
- `unsafe` requires a proved contradiction, explicit safety-evidence absence,
  or bounded candidate rejection by a critical safety gate.
- `exhausted` requires bounded consumption evidence and absence of a stronger
  unsupported or unsafe proof.
- Invalid, corrupt, nondeterministic, or tool-failed runs abort the baseline.

## Aggregate Report

The baseline report records:

- schema, evaluator policy, ranking policy, corpus role, corpus/manifest hash,
  freeze commit, and toolchain identity;
- inventory, catalog, model-policy, impact-registry, acceptance-profile, and
  budget hashes;
- total and per-domain outcome counts;
- per-case outcome, stop reason, stage reach, root gaps, downstream symptoms,
  consumption, and raw evidence hash;
- discovery clusters with complete ranking tuples;
- sealed but unranked held-out observations;
- deterministic rank-1 selection;
- aggregate expansion plan and plan hash; and
- report hash.

## Selection Seal

After the discovery report is complete:

1. Verify rank 1 is actionable through the existing expansion planner.
2. Write `SELECTION.json` containing the cluster key, full ranking tuple,
   affected discovery cases, required evidence, expansion-plan hash, corpus
   hash, policy hashes, baseline report hash, and freeze commit.
3. Hash and commit the selection record.
4. Only then unseal held-out results for later comparison.

If rank 1 is not actionable, baseline closeout fails with a planner capability
gap. Do not select a lower-ranked cluster.

## Required Checked-In Artifacts

```text
internal/.../testdata/closed_loop_open_set_corpus/
  manifest.json
  manifest.sha256
  discovery/*.json
  held_out/*.json
specs/closed-loop-open-set-capability-expansion/
  AUTHORSHIP.md
  BASELINE_REPORT.json
  BASELINE_REPORT.sha256
  SELECTION.json
  SELECTION.sha256
```

Raw per-case evidence may be stored in a deterministic content-addressed test
artifact directory when checking every artifact into Git would be excessive;
the report must retain hashes and the reproduction command.

## Reproduction And Drift Tests

Tests must prove:

- two clean-root baseline runs are byte-identical;
- checked-in report and selection bytes reproduce exactly;
- corpus reorder, byte mutation, role change, policy change, registry change,
  budget change, toolchain mismatch, missing evidence, and evidence corruption
  fail closed;
- report ordering is stable under diagnostic/map ordering;
- held-out observations do not affect rank or selection; and
- no capability implementation commit predates the baseline commit.

## Baseline Closeout

The baseline phase closes only after:

- focused adapter/classifier/ranking/planner tests pass;
- every corpus case has a valid terminal outcome;
- the report, selection, and checksums reproduce;
- the corpus and baseline are committed separately from capability work;
- Prism finds no unresolved high or medium correctness issue; and
- no GitHub Actions run or monitoring is started.
