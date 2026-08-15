# V18 Public Generation-One Protocol

From the clean commit containing the authenticated V18 evaluator freeze, run
`kicadai-discovery-baseline-v18` against all 24 immutable V10 public discovery
cases. Execute exactly two sequential clean-root replays per case under the
locked installed-KiCad environment.

Atomically publish only a complete deterministic public report. Partial case
checkpoints may support authenticated resume but are not accepted evidence.
Compare results with the committed V17 public baseline by case identity and
typed obligation. V17 artifacts remain immutable.

Acceptance requires no public regression and at least one new complete,
physically promoted pass. Otherwise publish the protocol-defined fail-closed
retirement rather than a partial or selectively filtered result. Do not access
held-out keys or encrypted held-out records.
