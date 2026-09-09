# Corrected V21 public maintenance evaluation

Date: 2026-09-08. Result: **the frozen advancement and preservation gates pass**.
All 24 public cases completed exactly two replays in one uninterrupted run.
Four selected cases across four reporting domains advance beyond topology
exhaustion. There are **no additional complete circuit passes**: the final
outcomes are 1 pass, 6 unsupported, 1 unsafe, and 16 exhausted.

V21 remains experimental and outside the supported v1 surface. This result is
not an arbitrary-circuit, general feasibility, or fabrication-readiness claim.

## Scope and provenance

This evaluation-only milestone changes no production capability, component
catalog, model, schema, corpus, selection, historical report, threshold, or
resource limit. Corrected production source is the merged security-maintenance
commit `b961b7a73aa9d19398d2adbaeee89de008e5d390`. The run identity, runner,
assessment rules, and tests were committed before execution at
`277663c8f09e3046013c2e22dac64f6895dd8c19`.

- [Run identity](RUN.json) and [pre-execution seal](FREEZE.sha256).
- [Authoritative inherited protocol](../V21_EVALUATOR_PROTOCOL.md).
- [Published public report](../../../internal/capabilityfeedback/testdata/closed_loop_open_set_v21_maintenance_1/report.json)
  and [file checksum](../../../internal/capabilityfeedback/testdata/closed_loop_open_set_v21_maintenance_1/report.sha256).
- [Machine-readable assessment](ASSESSMENT.json), reproduced by the frozen
  `TestPublishedMaintenanceEvaluation` after publication.

The active evaluator manifest SHA-256 is
`d3c9c6124d52cb5c59bd4a175d75f1c8fa2f4c9d9e4148d70979f62de06974d1`.
The canonical report hash is
`7269d4df5c8b2129dd1f451dbb457fe5e3a51bfcd22de48c4d6f5f3c6ce6ff9e`;
the report file SHA-256 is
`949963f6f38574626d125fe8d4e2f34966437b9a3c21dfe6518ceff37985efb1`.
The assessment file SHA-256 is
`26456f6f1e68d076b5b564c3f0a04d017dd8ee12d15a4806f6ff117a9fe34574`.
An additive publication test seals these output bytes without modifying the
pre-run assessment test or requiring a passing capability result.

Execution used Go 1.26.8, darwin/arm64, and KiCad 10.0.3. The runner disabled
implicit Go environment/workspace/flag/experiment overrides, authenticated
module and source seals, required a clean source tree, and verified unchanged
HEAD and tracked source after execution. It exited successfully and published
atomically without replacement. No case was restarted; no baseline was rerun;
no held-out key or plaintext was accessed; no GitHub Actions workflow was
manually dispatched. The original V21 report and pre-run maintenance freeze
remain unchanged as historical evidence.

## Outcomes and advancement

| Public evaluation | Pass | Unsupported | Unsafe | Exhausted |
| --- | ---: | ---: | ---: | ---: |
| Frozen V20 comparison | 1 | 5 | 1 | 17 |
| Original V21, historical and superseded | 1 | 5 | 1 | 17 |
| Corrected V21 maintenance revision 1 | 1 | 6 | 1 | 16 |

The original evaluator's six-case advancement claim does not validate the
corrected evaluator. This independent maintenance run qualifies four cases:
`v10_case_004`, `v10_case_017`, `v10_case_018`, and `v10_case_021`, covering
mixed-signal data conversion, sensing/instrumentation, protection/power
integrity, and digital control. This exceeds the unchanged three-case/two-domain
threshold. All four remain `exhausted`, not passing circuits.

| Selected public case | Corrected terminal blocker | Counts as advancement? |
| --- | --- | --- |
| 001 | V21 topology-completion bound exhausted | No |
| 004 | Complete structural certificate; behavioral evidence nonpassing | Yes |
| 009 | Contradictory causal path; now unsupported | No |
| 012 | Complete-topology search exhausted, unchanged | No |
| 016 | V21 topology-completion bound exhausted | No |
| 017 | Complete structural certificate; behavioral evidence nonpassing | Yes |
| 018 | Complete structural certificate; behavioral evidence nonpassing | Yes |
| 021 | Complete structural certificate; behavioral evidence nonpassing | Yes |

Case numbers above have the public `v10_case_` prefix. Case 009 accounts for
unsupported increasing from five to six and exhausted falling from seventeen
to sixteen: corrected invariant analysis refuses a contradictory causal path.
This is not counted as progress and is not proof that every possible physical
implementation of the requirement is impossible.

The unchanged qualifying rule requires a complete structural certificate and
the exact V20 admission/evaluation path. A later model-admission refusal can
prevent numerical execution; a qualifying blocker does not imply successful
simulation. Canceled repairs, exhausted structural bounds, contradictions, and
diagnostic renames do not qualify.

Selected terminal frontier occurrences changed as follows. These are diagnostic
leaves, not case counts; a case can have more than one kind of leaf.

| Terminal capability | V20 | Corrected V21 |
| --- | ---: | ---: |
| `causal_topology_repair` | 47 | 0 |
| `catalog_value_domain` | 5 | 0 |
| `complete_topology` | 9 | 9 |
| `bounded_topology_completion` | 0 | 14 |
| `causal_path_consistency` | 0 | 8 |
| `passing_behavioral_evidence` | 0 | 25 |

The complete aggregate frontier, including unchanged model, solver, and
electrical-assertion blockers, is in [ASSESSMENT.json](ASSESSMENT.json).
The selected gap has materially advanced, but is not fully resolved. The
twenty-five later behavioral-evidence leaves do not identify one proven root
cause; further public diagnosis is needed before selecting a generic repair.
No subsequent capability phase begins in this milestone.

## Preservation and physical evidence

The frozen assessment authenticates all 24 report records and their canonical
hashes, confirms 48 matching replays, and compares all 16 unselected raw case
objects byte-for-byte with V20. V18's public pass and unsafe outcome are
preserved; no new unsafe outcome or frozen preservation regression occurred.
V20 admission preservation is also covered by the local admission/executor
regressions listed below.

Public case 005 remains the sole pass. Both installed-KiCad promotion records
have distinct clean-root digests, matching run and project digests, and all
14 required gates true: primitive-only construction, topology search,
simulation, all corners, model provenance, closed-loop evidence, routing,
connectivity, writer correctness, zero round-trip differences, ERC, strict
DRC, deterministic replay, and fail-closed behavior.

The current within-run project digest is
`749e28df8589469077f4242a65c804d02a74f0068a6b883aa39002d8d2cc7af6`.
It differs from the historical V18 project digest; preserved behavior and
current deterministic replay are not historical project-byte identity.

Direct inspection of retained physical artifacts found four zero-violation ERC
reports, eight zero-violation DRC/refill reports with zero unconnected items and
schematic parity findings, and sixteen empty writer normalized-diff files.
Each numerical replay performs two physical promotion runs; this does not mean
the case was numerically evaluated four times.

KiCad configuration limitation: zero reported violations is measured under the
frozen check configuration, not with every possible check enabled. ERC reports
list `single_global_label`, `four_way_junction`, `simulation_model_issue`, and
`footprint_filter` as disabled. DRC reports list `missing_courtyard`,
`track_not_centered_on_via`, `tuning_profile_track_geometries`,
`footprint_filters_mismatch`, and `footprint_type_mismatch` as disabled. This run
introduced no project rule-severity overrides or check-setting changes.

## Work and retention limitations

The sealed V21 planner enforces depth 3, width 8, generated work 48, retained
states 64, conservative graph size 1,048,576 bytes, and one topology worker.
The transport is serial with two replays per case and a six-hour run limit.
Aggregate inherited-plus-successor counters are not interchangeable with the
separate V21 planner budget.

The existing transport removes numerical synthesis spools after completed case
checkpoints. The published report retains case/frontier digests rather than a
separately reviewable raw work trace for every attempted graph. Bound claims
therefore rest on the authenticated implementation, frozen policy, and
regressions, not a reconstructed all-attempt work audit.

Two read-only resource observations found temporary first-replay synthesis
spools of 10,720,798,586 bytes for case 016 and 9,070,942,131 bytes for case 020.
The latter is unselected and delegated through V20, so large evidence volume
is not unique to V21 repair. These are spot measurements, not a complete
performance profile or proof of a runtime bottleneck. No output handling or
budget was changed in response.

## Local validation and review

The following checks passed on the clean freeze commit, without restarting the
public evaluation:

- Complete uncached bounded suite: `make test-bounded GO_TEST_FLAGS=-count=1`.
- Formatting, vet, and lint: `make lint`, zero issues.
- `make race-short` plus focused V21/topology race regressions.
- `make review-matrix`, two runs of its five-package review matrix.
- Five educational schematic layout/library checks, two rounds using the
  installed KiCad symbol and footprint libraries.
- The 13-case optional installed-KiCad design-example tier. Every declared
  success and refusal expectation passed; this is not thirteen passing designs.
- V20/V21 specification and simulation-admission suites, historical/current
  evaluator and corpus seals, and focused V20 preservation regressions.
- Six-scenario clean-checkout promotion bundle: twelve passing command/promotion
  results, six equal replay inventories, zero differences, and 286 independently
  authenticated files in bundle
  `sha256-414ba1137410bf295156d794e8853b88ee8717769af48940147a751f9aceefe0`.
- macOS/Linux AMD64/ARM64 verification builds were byte-identical across two
  builds, and host clean-install/first-run release smoke passed. These are
  verification builds, not newly published release assets.
- The new audit-test package cross-compiled for Linux/AMD64; this is not a claim
  of Linux runtime execution.

After publication, the complete frozen maintenance test package passed uncached
and reproduced the assessment above. This read-only verification did not rerun
the corpus. The pre-run freeze received three authorized Prism/Gemini reviews;
its valid stderr-capture finding was fixed before execution.

The complete result diff receives a final Prism review, followed by coverage
against the final unchanged staged source. Exact final review dispositions,
coverage results, and commit identity are recorded in the PR verification
record; this avoids editing source-bound proof inputs after coverage.
