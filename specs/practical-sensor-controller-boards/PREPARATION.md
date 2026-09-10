# Preparation log

Status at sealing: preparation complete. Zero new-corpus executions and zero
live OpenAI requests occurred before the companion freeze was committed.

## Completed preparation

- Verified clean merged main at `87c411b7a13bdeb0efc6ff36b35e9a69c4e2706d`.
- Created `codex/practical-sensor-controller-milestone`.
- User authorized reuse of the existing OpenAI API key for testing. No key value
  was written or printed; environment presence was checked only as a boolean.
- Drafted SPEC, PLAN, feasibility review, adjudication procedure, eight positive
  cases, four refusals, two clarifications and two paraphrases.
- Added an isolated evaluator; no production source or historical evidence has
  changed. It uses existing provider, compiler, architecture, electrical,
  placement/routing, writer and native-KiCad entry points.
- Added append-only request receipts, fixed provider settings, secret-safe
  capture, conservative request/spend reservations, immutable evidence writes,
  full-library lossless compression and narrow normalized replay comparison.
- Added a serial campaign supervisor with case/campaign time, sampled process-
  tree RSS, storage and no-overwrite controls. A complete campaign lock prevents
  overlapping baseline/final workers using the same evidence root.
- Added preflight acceptance checks that reject dropped mandatory proof gates,
  missing ambient coverage and omitted/enlarged enclosure bounds.

## Verification so far

Focused unit and race tests passed during preparation. Focused golangci-lint
reported zero issues. The supervisor passes Node syntax checking. These checks
are preparation evidence only; final pre-freeze commands must be rerun on the
exact committed evaluator revision.

An opt-in native plumbing control reads the pre-existing
`regulated_mcu_sensor_subsystem.json` regression requirement, not the new corpus.
It does not contact OpenAI and never changes that historical input.

1. `/tmp/kicadai-practical-native-plumbing-1`: rejected by an overly broad
   evaluator check on unrelated stock-library symbols. The evaluator now keeps
   all load diagnostics and relies on selected-reference resolution and native
   writer checks, as the existing promotion harness does.
2. `/tmp/kicadai-practical-native-plumbing-2`: both native runs passed physical
   gates; normalization found 33 differences, all native temporary directory
   suffixes and ERC/DRC command elapsed times. Electrical values and native
   project identities did not differ. Exact volatile fields are now documented
   in SPEC and protected with narrow normalization tests.
3. `/tmp/kicadai-practical-native-plumbing-3`: PASS, 40.62 s test body / 41.108 s
   package time. Both clean native/electrical runs have equal normalized
   evidence, including all hierarchical native project files. SVG schematic
   and board previews were generated. Complete retained evidence occupies
   171 MiB after losslessly compressing stock-library indexes (the earlier
   uncompressed control occupied 2.9 GiB). These are plumbing-control figures,
   not new-corpus performance results.

All three preparation control directories are retained. None is counted as a
new capability pass. Preparation corrections precede the acceptance freeze;
they do not authorize post-freeze evaluator changes or final-result retries.

## Final freeze preparation

The evidence-quality review added authenticated paired-input inventories,
per-attempt elapsed times and local rejection reasons, precise estimated-cost
labels, clean-build/current-source checks, native environment identities and
synthetic process-monitor tests. The final focused race tests passed (1.717 s),
focused lint reported zero issues, and all three Node test groups passed.
Syntax and whitespace checks passed. Only evaluator/docs changed.

Snapshot 3 records 22,730 symbols and 15,433 footprints with full compressed
library evidence, baseline catalog/model/capability identities and closed-loop
policy. `environment.json` pins those identities plus Go 1.26.8, Node 26.8.2 and
KiCad 10.0.3 binary identities. `QUALITY.md` resolves steady-state rail-window
semantics and independently audited physical constraints before any outcomes.

The next action is to commit/seal these bytes, build the exact sealed worker,
verify production equality with merged main, and run the one baseline campaign.

Production improvements, the new baseline, final/paired evaluation, acceptance
audits, capability-growth claims and PR publication are not complete.
