# Diagnostic recovery review

Prism reviewed the staged recovery through its authorized configured Gemini
provider (`gemini-3-flash-preview`). No synthesis or numerical code changed.

- `0fa27a371a8b2b1b` (nil dereference): not valid. `attempt.Actual` is checked
  at function entry; each diagnosis actual and each minimum/maximum is guarded
  before dereference by Go's short-circuit Boolean evaluation. Added explicit
  missing-actual and missing-bound tests.
- `981e7da3c1660382` (use floating-point tolerance): not valid for this operation.
  This is an identity check between duplicated, authenticated evidence fields,
  not a numerical convergence test. An epsilon would permit a different recorded
  result to be used as evidence. Added a near-but-unequal value rejection test.
- `32686e5b0497a6ad` (accept mixed diagnostic codes): intentionally not applied.
  A simultaneous solver failure must not be reclassified as assertion-only.
  These diagnostics have no warning severity field that would support the
  proposed distinction. Added a mixed solver/assertion evidence rejection test.

The supplemental changes are tests and this disposition only. Recovery requires
passing race tests, lint, and the unchanged historical seals before execution.
