# V21 maintenance revision 1: public reevaluation

This is an evaluation-only successor run, not a new implementation generation.
`RUN.json` and `FREEZE.sha256` are committed before execution. The corrected
evaluator, its existing maintenance seal, selection, corpus, historical reports,
and original protocol remain unchanged. The existing freeze's
`not_run_after_maintenance` value records its pre-run state; this directory and
the separate maintenance report record the new run without rewriting it.

The inherited [protocol](../V21_EVALUATOR_PROTOCOL.md) remains authoritative:
24 public cases in manifest order, exactly two serial replays, unchanged
depth/width/work/retained/graph limits, V20-first admission and delegation,
V18 pass and safety preservation, and installed-KiCad proof for every pass.
No held-out key or plaintext access is permitted. Public corpus authentication
may verify opaque publication checksums through the existing public loader.

Run `bash specs/generic-causal-topology-repair/maintenance-evaluation-1/run.sh
/absolute/fresh/scratch-root` from the clean freeze commit. The runner retains
physical artifacts and publishes the new report atomically without replacement.
It does not rerun V18/V20 baselines, tune production code, alter selection or
budgets, overwrite history, or retry based on results. If interrupted, first
establish whether the original process remains live; do not start a second
cohort merely because a monitoring call timed out.

## Assessment frozen before execution

- Authenticate the complete report with the existing V10 evidence validator,
  including canonical report/case hashes, two replays, and pass/promotion gates.
- Compare all 16 unselected case objects byte-for-byte with frozen V20. Preserve
  V18 passes and safety outcomes. The sealed evaluator's critical-invariant
  preservation and V20 admission paths also retain their regression tests.
- Require at least three selected cases across two reporting domains to pass
  or advance to a strictly later, more specific blocker without regressions.
- The frozen evaluator emits `OPEN_TOPOLOGY_NO_PASSING_GRAPH` with
  `passing_behavioral_evidence` only after a complete structural certificate and
  admitted candidate evaluation. Count that terminal frontier as advancement,
  as in the original protocol. A complete installed-KiCad pass also qualifies.
  A bound, contradiction, cycle, invalid repair, or diagnostic rename does not.
  Any different putative downstream advance must be justified from retained
  evidence against the same rule; it is not automatically counted by its label.
- Publish aggregate outcomes, frontier changes, qualifying cases/domains,
  regressions, and work/evidence limitations. A failed threshold or preservation
  gate is a valid evaluation result, not permission to retune and repeat.
- V21 remains experimental regardless of this run. Admission to the v1 support
  surface is a separate decision. No subsequent capability phase starts here.

After publication, the read-only `TestPublishedMaintenanceEvaluation` authenticates
and assesses the new report. It must not demand a passing capability outcome;
the assessment, not test success alone, determines advancement.
