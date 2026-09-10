# V23 residual diagnostic: exact failures recovered

All **118 recorded V22 repair trials reproduced exactly** under the frozen
diagnostic at `2216e34c8e930385d479140de645af45d38d37c5`. Every reconstructed graph
and numerical evaluation hash matched. No synthesis search, physical promotion,
held-out access, retry, or V22 artifact modification occurred. V22 still has one
complete pass out of 24; these diagnostic results are not additional pass evidence.

## What failed

| Public case | Recorded trials | Authenticated residual failures |
| --- | ---: | --- |
| 004 | 0 | Original critical-failure guard; circuit-level stability observation unresolved. No numerical replay. |
| 017 | 114 | 94 voltage-bound failures, 4 current-bound failures, 13 solver nonconvergence failures, 3 ambiguous current-observation refusals. |
| 018 | 4 | Negative-feedback trial: two noise checks pass, then DC-sweep operating-point nonconvergence. Positive-feedback trial: instability refusal. Two grounded-input trials: noise exceeds its bound. |
| 021 | 0 | Original critical-failure guard; positive-feedback instability in the selected model. No numerical replay. |

### Case 018: the useful repair reaches a solver failure

Trial 1 redirects the inverting control terminal to the output. Its graph hash is
`052b2c3e6ed815e8fd21ffb57353ae5802b762c0aa8a18a5f24b4dc8336d6cc5`;
its evaluation hash is
`feec5f5e7a2759508ab36c0464990d99a98b1ce0d760ca33d69d441644132b29`.
Both noise evaluation attempts pass. Attempt 3, `b_transfer_gain`, fails in
`dc_sweep`, operating case `nominal_transfer`, corner `nominal`, with
`nonconvergent`: the bounded op-amp operating-point states do not converge.
There is no measured gain; the required interval remains 0.98–1.02.

This identifies the next failed gate rather than inferring it from a count of
passing attempts. It does **not** establish that the entire requirement would
pass with a corrected solver. In particular, the trusted selected component has
0.3 V output rail margins, and the declared excitation includes values below
that lower margin. Later gain, bandwidth, and output-swing checks remain unproved.
Do not change the model, its parameters, or the requirement bounds to bypass them.

Source inspection identifies a concrete mechanism to test independently:
`solveDCSweepAnalysis` carries saturation clamps from one sweep point to the next.
`solveBoundedOpAmpDCFromState` replaces the freshly solved unclamped linear state
with those inherited clamps, then updates all clamp states simultaneously. It
rejects repeated states without trying a fully revalidated release of an obsolete
rail clamp. A negative-feedback circuit crossing from saturation back into its
linear range can therefore be a useful independent reproducer. This is a
source-supported hypothesis, not yet an isolated proof of the corpus failure.

### Case 017: multiple remaining electrical blockers

- 94 trials first fail `a_voltage_excitation_level`: measured 2 V or 3 V,
  required 2.45–2.55 V.
- 4 first fail `b_current_excitation_level`: measured approximately 0.500 mA,
  3.162 mA, or 5.246 mA, required 0.950–1.050 mA.
- 13 first fail the voltage assertion's operating-point solver with active-device
  state nonconvergence. These use the nonlinear active-device path; they are not
  evidence that the simpler sweep mechanism above is also their cause.
- 3 reach the current assertion but fail before simulation because observation
  cannot identify a unique catalog-backed load or active current path. This is
  measurement ambiguity, not proof of a missing transistor model.

Counts are mutually exclusive first-failure categories and sum to 114. They count
trials, not distinct requirements or independent circuit families. Passed-attempt
counts include assertion/case/corner attempts, not necessarily unique assertions.

## Priority for the correction investigation

The only observed residual numerical-failure class spanning both repair-eligible
cases is **bounded operating-point active-state nonconvergence**. Investigate that
class first using independent saturation-to-linear, coupled-amplifier, comparator,
and nonlinear-load tests. Establish which failures are numerical defects versus
valid refusals before freezing a correction. The linear and nonlinear paths must
not be presumed to share one root cause merely because their codes match.

Keep the real voltage/current failures and ambiguous observations separate.
Preserve both critical guards. Do not expand the repair beam, relax numerical
tolerances, alter safety metadata, or add component/topology special cases.
Only a separately versioned, fully validated successor can establish new passes.

## Timing, execution counts, and evidence size

- One diagnostic invocation; each of the 118 recorded candidates evaluated once.
  No repeats or recovery invocation were needed.
- Case 017: 114 evaluator invocations, reproducing V22's 128 numerical simulation
  calls (including its internal retries) and 638 corner evaluations; 8.594189625 s.
- Case 018: 4 evaluator invocations, reproducing 6 numerical simulation calls and
  20 corner evaluations; 0.223193667 s.
- Cases 004/021: zero evaluator invocations each; retained critical refusals only.
- Native process: 9.10 s real, 16.85 s user, 1.57 s system.
- Native maximum resident set size: 48,054,272 bytes. Separately reported peak
  memory footprint: 33,276,528 bytes; these are different OS metrics.
- Four diagnostic JSON files: 855,234 bytes. Four per-case timing/count JSON files:
  393 bytes. Total: 855,627 bytes, excluding source/environment/process receipts.

This is far smaller work than V22's full search/evaluation. The 9.10 s versus
V22's approximately 4 h 51 m is **not** a controlled speedup comparison. No solver
or concurrency optimization was made. Retained JSON is a compact diagnostic
projection, not lossless storage of numerical waveforms.

## Evidence

`evidence/` contains the unmodified generated traces, counts, process log, native
resource report, source identity, and executable digest. `go-environment.json`
names the original eight environment values and preserves the raw stdout digest;
only its representation is normalized to avoid trailing empty lines in Git.
`EVIDENCE.sha256` authenticates the report and artifact files. The frozen
diagnostic code and plan remain under their original `DIAGNOSTIC.sha256` seal.
