# V23 public evaluation: preservation passes; improvement criterion fails

The original frozen evaluation completed on September 10, 2026, at approximately
16:03 UTC. All 24 public cases completed exactly two fresh serial replays.
Evaluated source is `16013c7d3f4e44ae8cfbda18be655c853ef8f111`.
The original launcher's completion marker confirms that its source checks and
frozen post-run assessment passed. No case, numerical evaluation, or physical
promotion was restarted. V22 artifacts remain unchanged.

| Outcome | V22 | V23 |
| --- | ---: | ---: |
| Complete pass | 1 | 1 |
| Unsupported | 6 | 6 |
| Unsafe | 1 | 1 |
| Exhausted | 16 | 16 |

**Zero additional complete passes; preservation passed; no regressions.**
The frozen advancement criterion required at least one additional complete
simulation-and-installed-KiCad pass and was not met. Passing the publication
tests means the negative result is authenticated, not that capability improved.

## Complete retained evidence

All 24 cases / 48 replays pass the separate retained-evidence audit. All case
objects, full synthesis replay hashes, and gate records match V22, including
all twenty unselected cases. Historical pass and unsafe outcomes are preserved.
The existing full-pass case 005 again clears both installed-KiCad promotions;
each promotion has two clean native projects. All four original project byte
hashes independently match
`749e28df8589469077f4242a65c804d02a74f0068a6b883aa39002d8d2cc7af6`.

The deterministic report hash is
`fa30e2ec94444c3a4813934a5cf80cd105648228374c3b6c4b561a6c8d352e2c`.
The exact retained publication is 154 files / 521,456 bytes including its
checksum manifest, whose SHA-256 is
`4d3c8dc8c0f07f068631f0c3427e095226104a5614cb2ee1006d9f3c56117182`.
It binds report, assessment, invocation summary, executable/source/environment
provenance, process measurements, and all compact replay/promotion sidecars.
The manifest digest is pinned independently in the publication test.

Canonical synthesis volume totals 121,026,790,256 bytes, with zero synthesis
spool bytes. The compact publication is not a lossless copy of every numerical
report. Native projects remain in the original scratch directory outside Git.
See [resource measurements](PERFORMANCE_OBSERVATIONS.md),
[local validation](LOCAL_VALIDATION.md), and [review](REVIEW.md).

All four selected cases (004, 017, 018, 021) have now completed both replays;
each retains the case outcome `exhausted`. In both replays of cases 004 and
021, V23 repair returns `unsupported` / `critical_failure`, with zero binding
work, trials, candidate simulations, and corner evaluations. This is zero
**V23 repair work**, not zero computation in the shared predecessor synthesis.
Neither guard has been weakened or bypassed.

## Repair-eligible targets

Case 017 remains `exhausted` with `evaluation_budget_exhausted`, 114 repair
trials, and no physical promotion. Its binding work and complete consumption
record equal V22. Every trial's number, graph hash, numerical evaluation hash,
status, and passing-attempt count also match V22. These are retained-evidence
comparisons, not new numerical diagnostic replays.

Case 018 remains `exhausted` with `repair_frontier_exhausted`, four repair
trials, six candidate simulation calls, and twenty corner evaluations. Binding
work and consumption equal V22; no physical promotion occurred. All four trial
graphs, statuses, and passing-attempt counts are unchanged. Only trial 1's
numerical evaluation hash differs:

- V22: `feec5f5e7a2759508ab36c0464990d99a98b1ce0d760ca33d69d441644132b29`
- V23: `a1bd9fdf81838b43aba6836e7715c5a514605d2805dfefc72f57b571cd6d6fe6`

Trial 1 still fails after two passing attempts. The other three numerical
evaluation hashes remain identical to V22. The new wrapper's solver-policy and
execution commitments authenticate the selected implementation; they do not
establish a complete electrical pass.

### Diagnostic limit

The compact repair sidecar retains trial identities, status, passing-attempt
counts, work accounting, and evaluation/execution hashes. It does not retain
the failed trial's complete numerical report or first-failure projection.
Consequently, a changed evaluation hash alone cannot establish that the old
nonconvergence became a measured gain, range, or other assertion failure.
No such claim is made here, and no additional numerical replay has been run to
fill that gap. The previously published V22 residual traces remain historical
evidence; they must not be relabeled as V23 trial-1 diagnostics.

## Goal boundary

None of the four selected cases has produced an additional complete simulation
and installed-KiCad pass. Independent regression fixtures and a genuine generic
solver correction do not substitute for that success criterion. No new v1
capability is admitted, and no further correction or capability iteration begins
under this frozen V23 protocol.

The frozen V23 evaluation is complete and its negative result is retained.
The improvement goal is not achieved. Publishing and reviewing this completed
experiment does not authorize a new capability iteration, a relaxed acceptance
gate, a merge, or any change to the v1 support boundary.
