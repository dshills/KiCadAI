# V18 Generation-One Audit

V18 is admitted.

The clean committed evaluator at
`e7d5e96c9cd6f5ee573b4cbdbe7ac0b042cbd111` evaluated all 24 authenticated
public discovery cases exactly twice. The two executions of every case were
deterministic. No held-out source key, baseline key, record, or outcome was
accessed.

## Result

- Baseline: 0 pass, 7 unsupported, 1 unsafe, 16 exhausted.
- V18 generation one: 1 pass, 7 unsupported, 1 unsafe, 15 exhausted.
- `v10_case_005` advanced from exhausted to pass.
- No other outcome changed.
- Every typed obligation satisfied by the baseline remains satisfied.

The passing case has two byte-identical synthesis replay digests and two
installed-KiCad promotions. Both promotions produced the same run and project
digests. Primitive-only synthesis, topology search, simulation, all-corner
evidence, model provenance, closed-loop evidence, routing completion,
connectivity, writer correctness, zero round-trip differences, ERC, strict
DRC, deterministic replay, and fail-closed behavior all passed.

An initial non-published run exposed that V18 delegated ineligible
requirements through V17 code while still supplying the V18-extended catalog
and model registry. That changed an unrelated search frontier and lost one
previously satisfied typed obligation. The generic correction now binds an
immutable legacy inventory and simulation environment independently from the
V18 extension. The corrected run preserved that obligation and all other
baseline evidence. The rejected report remained outside the repository and is
not publication evidence.

Authoritative artifacts:

- `internal/capabilityfeedback/testdata/closed_loop_open_set_v18_generation_one/report.json`
- `specs/closed-loop-open-set-capability-expansion/V18_GENERATION_ONE_COMPARISON.json`
