# KiCad-Backed Design Promotion Specification

## 1. Purpose

KiCadAI now has optional KiCad-backed `design create` examples under
`examples/design/kicad-backed/`. They are valuable because they exercise the
full path from intent, through block selection, schematic generation, PCB
realization, placement, routing evidence, file writing, internal validation,
and optional real `kicad-cli` ERC/DRC checks.

The current fixtures are intentionally marked `expected_fail`. They document
known generated-board gaps, but the project needs a clear promotion system so a
fixture can move from:

1. `expected_fail`: useful failure evidence exists;
2. `candidate`: internal validation and optional KiCad evidence are good enough
   for provisional generated-board confidence;
3. `pass`: the fixture is stable enough to be a required regression target.

This specification defines the promotion model, evidence gates, report format,
and fixture-level criteria needed to graduate KiCad-backed generated design
examples without making normal `go test ./...` depend on KiCad.

## 2. Background

The optional fixtures currently include:

| Fixture | Current readiness | Current role |
| --- | --- | --- |
| `led_indicator_kicad_smoke` | `expected_fail` | Smallest generated design-level smoke case. Pad/copper net-code assignment and local endpoint binding have improved, but full validation plus clean ERC/DRC promotion is still pending. |
| `connector_led_kicad_smoke` | `expected_fail` | Multi-block connector-to-LED composition case. Inter-block route candidates and endpoint-contact diagnostics exist, but all required contacts do not yet graph-connect cleanly. |
| `i2c_sensor_breakout_candidate` | `expected_fail` | Richer generated breakout candidate. VCC/GND/SDA/SCL route candidates and contact evidence exist, but routed same-net completion and KiCad-clean layout proof are still pending. |
| `opamp_headphone_buffer_kicad_candidate` | `expected_fail` | Amplifier seed at fabrication-candidate strictness. It must remain fail-closed until verified amplifier component evidence, output protection/DC blocking, analog layout proof, and ERC/DRC evidence exist. |

Existing metadata already stores `readiness`, `acceptance`, `require_erc`,
`require_drc`, `expected_stages`, `expected_artifacts`, `known_gaps`, and
`notes`. This project should preserve that metadata while adding a normalized
promotion report and deterministic gate evaluation.

## 3. Goals

- Make every optional KiCad-backed design fixture produce a machine-readable
  promotion report.
- Distinguish expected failure, candidate quality, and required pass quality
  using explicit gates rather than ad hoc test expectations.
- Keep default tests KiCad-independent.
- Make optional KiCad evidence opt-in and deterministic when
  `KICADAI_KICAD_CLI` is configured.
- Promote fixtures one at a time when their evidence is strong enough.
- Convert current blockers into AI-actionable repair guidance.
- Prevent a fixture from silently remaining `expected_fail` without a current
  blocker.
- Prevent a fixture from being promoted when it only parses but is not
  electrically meaningful.

## 4. Non-Goals

- This project does not require KiCad for `go test ./...`.
- This project does not make every generated design fabrication ready.
- This project does not implement a complete autorouter by itself.
- This project does not promote all optional fixtures at once.
- This project does not make the amplifier fixture pass fabrication-candidate
  checks before verified analog output-stage and component evidence exist.
- This project does not mutate user KiCad projects.

## 5. Readiness States

### 5.1 `expected_fail`

A fixture is `expected_fail` when it should run far enough to expose a known
blocker. The blocker must be explicit and current.

An `expected_fail` fixture passes its test only when:

- the fixture executes the expected stages up to the blocker;
- produced artifacts match the metadata expectation where applicable;
- the promotion report records at least one blocking issue;
- the blocking issue matches the fixture's known gaps or expected issue codes;
- a silent skip is not reported as useful evidence;
- a clean pass is treated as an unexpected promotion opportunity requiring
  metadata review.

### 5.2 `candidate`

A fixture is `candidate` when it is provisionally successful but not yet stable
enough to become a required pass fixture.

A `candidate` fixture must:

- complete all expected internal generation stages;
- write valid KiCad project, schematic, and PCB files;
- pass writer correctness checks;
- pass internal board validation for the requested acceptance level;
- prove required net-to-pad connectivity;
- prove route-completion or explicitly accepted unrouted status for every
  required net;
- prove route endpoint contacts are physically connected, not merely
  topologically requested;
- produce optional KiCad ERC/DRC evidence when `require_erc` or `require_drc`
  is true and `KICADAI_KICAD_CLI` is configured;
- preserve artifacts needed to inspect failures.

Candidate status may still carry warnings for non-blocking quality issues, such
as incomplete fabrication metadata when acceptance is below
`fabrication-candidate`.

### 5.3 `pass`

A fixture is `pass` when it is stable enough to be considered a required
regression target for the optional KiCad-backed tier.

A `pass` fixture must satisfy every `candidate` gate and additionally:

- have no blocking internal validation issues;
- have no route-contact misses for required nets;
- have no missing required artifacts;
- have no KiCad ERC/DRC findings except explicitly allowed findings documented
  in metadata;
- be deterministic across repeated runs;
- have docs describing the fixture as a passing generated-board case.

### 5.4 `blocked`

A fixture is `blocked` when it is documented but should not run as part of the
optional tier. The report must explain why it is blocked before execution.

Valid blocked reasons include:

- unsupported topology;
- missing required library data;
- missing external toolchain;
- missing verified component evidence when metadata declares the fixture should
  not execute until the evidence exists;
- intentionally unavailable acceptance policy.

If missing verified component evidence is discovered during a runnable fixture's
planning or component selection stage, the fixture should be classified as
`expected_fail` when the metadata declares that blocker, not pre-run `blocked`.

## 6. Promotion Gates

Every promotion report should evaluate the following gates.

### 6.1 Metadata Gate

The fixture metadata must be valid and complete:

- `id` matches the request basename and metadata prefix. For fixture `foo`,
  the request file is `foo.json` and the metadata file is
  `foo.metadata.json`;
- `id` is unique across the entire optional KiCad-backed tier. Nested request
  directories are not currently allowed; if they are added later, the fixture
  ID must include enough normalized relative-path context to avoid collisions;
- `request` points to an existing request file;
- `tier` is known;
- `readiness` is one of `expected_fail`, `candidate`, `pass`, or `blocked`;
- `acceptance` is a known design workflow acceptance level;
- `require_erc` and `require_drc` are explicit booleans;
- `expected_stages` is non-empty for runnable fixtures, where runnable means
  `expected_fail`, `candidate`, or `pass` readiness;
- `known_gaps` is non-empty for `expected_fail` and `blocked` fixtures;
- `notes` explain the fixture's purpose.
- `id`, `request`, expected artifact paths, and report artifact paths are local
  relative paths only. They must reject absolute paths, `..` path components,
  path traversal after cleaning, empty path elements, and paths outside the
  fixture or output root before any filesystem access.

### 6.2 Stage Gate

The design workflow must report which stages were reached:

- `block_planning`;
- `component_selection`;
- `schematic`;
- `pcb_realization`;
- `placement`;
- `routing`;
- `project_write`;
- `writer_correctness`;
- `validation`;
- `kicad_checks`.

The gate should record:

- reached stages;
- missing expected stages;
- stage where execution stopped;
- first blocking issue per stopped stage;
- whether a stopped stage is expected for current readiness.

### 6.3 Writer Correctness Gate

Generated files must satisfy the writer-level invariants appropriate for the
fixture. This gate validates that emitted files are structurally sound and
faithfully reflect the internal model; the connectivity gate validates whether
that model matches design intent.

- KiCad project file shape is complete;
- schematic file parses through the internal model;
- PCB file parses through the internal model;
- footprints contain expected pads;
- pads and copper shapes preserve the internal model's net assignments;
- board outlines are present;
- layers and stackup are legal;
- generated files have valid S-expression syntax and required top-level nodes;
- preservation or unknown-node support, when present, does not remove or
  corrupt required generated project, schematic, PCB, net, layer, footprint,
  pad, or board-outline structures.

This gate protects against parseable but malformed KiCad files.
Internal parser checks are not a substitute for KiCad-backed validation. When
`KICADAI_KICAD_CLI` is configured, the optional KiCad gate provides external
tool evidence that generated files are accepted by KiCad.

### 6.4 Connectivity Gate

The fixture must prove electrical intent:

- each required net exists;
- each required net owns the expected pads;
- same-net pads are connected by copper or are explicitly reported unrouted;
- no required pad is assigned to the wrong net;
- no route segment bridges incompatible nets;
- route-contact diagnostics distinguish contact, miss, ambiguous contact, and
  unsupported endpoint.

The gate should consume existing board validation, route completion, and
contact graph evidence rather than inventing a second connectivity model.

### 6.5 Route Completion Gate

For required routed nets, the report must distinguish:

- candidate route found;
- route attempted;
- route emitted;
- route physically contacts both endpoints;
- route completes the same-net connected component;
- route remains partial or unrouted.

The gate must not treat route emission alone as completion.

### 6.6 Physical Rule Gate

The fixture must expose available physical-rule evidence:

- clearance;
- minimum track width;
- minimum via size;
- annular ring;
- copper sliver;
- solder mask web;
- silkscreen clearance;
- board edge clearance;
- zone connectivity where zones exist;
- fabrication metadata where fabrication-candidate acceptance is requested.

The gate should classify missing evidence separately from failing evidence.

### 6.7 Optional KiCad Gate

When `KICADAI_KICAD_CLI` is not configured, optional KiCad-backed tests may
skip real KiCad execution, but the skip must be reported as missing external
evidence, not as a pass.

When `KICADAI_KICAD_CLI` is configured:

- the implementation targets KiCad 7 or newer unified CLI behavior;
- the runner should record the `kicad-cli --version` output when available and
  fail external evidence clearly for unsupported older versions;
- the path must be resolved without shell interpretation;
- the runner must execute it through safe process APIs such as Go's `os/exec`
  with argument arrays, not shell-constructed commands;
- `KICADAI_KICAD_CLI` must be an absolute path to an executable file. If the
  implementation offers PATH discovery when the variable is unset, it must use
  a restricted, documented search path and report the resolved absolute path;
- missing, non-executable, or directory paths must be reported as skipped or
  failed external evidence, not retried through a shell;
- ERC must run when `require_erc` is true;
- DRC must run when `require_drc` is true;
- raw report artifacts must be preserved when `--keep-artifacts` or the test
  harness artifact path is active;
- allowed findings must be matched by stable issue identifiers or explicit
  message fragments;
- unexpected findings block `candidate` and `pass`;
- missing report artifacts block `pass`.

### 6.8 Determinism Gate

Promotion should require deterministic evidence:

- stable fixture IDs;
- stable references where expected;
- stable net names;
- stable JSON report ordering;
- stable issue ordering;
- golden expectation compatibility for promoted fixtures. These expectations
  may be checked into Go tests as stable field/value assertions rather than as
  generated JSON report files. Repeated local real-KiCad runs are useful extra
  evidence, but the normal automated gate should compare deterministic report
  data rather than rerun expensive generation loops by default.

## 7. Promotion Report

Each optional fixture should produce a JSON report. The report may be emitted as
a test artifact and, when run through the CLI, under the design output's
`.kicadai/` directory.

The report destination must not depend on `project_write` succeeding. The CLI
or test harness should create the output artifact directory before running the
workflow when promotion reporting is requested. Early failures, such as the
amplifier fixture failing during component selection, must still be able to
write a report.

Recommended path:

```text
.kicadai/design-promotion.json
```

### 7.1 Report Shape

The report should include:

```json
{
  "id": "connector_led_kicad_smoke",
  "request": "connector_led_kicad_smoke.json",
  "tier": "block-composition",
  "declared_readiness": "expected_fail",
  "achieved_readiness": "expected_fail",
  "acceptance": "erc-drc",
  "status": "expected_fail",
  "matches_expectation": true,
  "summary": "Inter-block LED_EN route emits contact-miss evidence.",
  "gates": [
    {
      "id": "route_completion",
      "status": "failed",
      "required_for": ["candidate", "pass"],
      "issue_codes": ["route_contact_miss"],
      "artifacts": [".kicadai/transaction.json"]
    }
  ],
  "stages": {
    "expected": ["block_planning", "component_selection", "schematic", "pcb_realization", "placement", "routing", "project_write", "writer_correctness", "validation", "kicad_checks"],
    "reached": ["block_planning", "component_selection", "schematic", "pcb_realization", "placement", "routing", "project_write", "writer_correctness", "validation"],
    "stopped_at": "validation"
  },
  "issues": [
    {
      "code": "route_contact_miss",
      "severity": "error",
      "stage": "validation",
      "message": "LED_EN route does not graph-connect all required contacts.",
      "repair": "Retry inter-block routing with pad-anchor endpoints for the connector and LED block."
    }
  ],
  "artifacts": [
    ".kicadai/transaction.json",
    ".kicadai/manifest.json"
  ]
}
```

### 7.2 Gate Status Values

Gate statuses should be:

- `pass`: required evidence passed;
- `warn`: evidence exists but is not strong enough for higher readiness;
- `failed`: evidence failed or required evidence is missing;
- `skipped`: gate was not applicable or external tool was not configured. If
  the gate's `required_for` list includes the target readiness, `skipped`
  blocks promotion to that readiness;
- `not_run`: execution stopped before the gate.

Promotion gates should map existing `StageStatusBlocked` or blocking issue
evidence to `failed`. Fixture-level `blocked` remains a readiness state and
appears only under `declared_readiness` or `achieved_readiness`.

### 7.3 Report Status Values

The top-level `status` field summarizes the run result, not the fixture's
readiness declaration. Consumers should use `matches_expectation` to decide
whether the run satisfied fixture metadata, because `status` includes both raw
execution outcomes and expected-failure classifications. Permitted values:

- `pass`: all gates required for the achieved readiness passed;
- `warn`: the fixture produced usable evidence with non-blocking warnings;
- `failed`: one or more required gates failed or required evidence is missing;
- `expected_fail`: required gates failed in a way that matches an
  `expected_fail` readiness declaration;
- `unexpected_pass`: the fixture was declared `expected_fail` but achieved
  `candidate` or `pass`;
- `blocked`: the fixture is intentionally documented but not executed;
- `skipped`: execution did not run because an optional external prerequisite
  was unavailable;
- `error`: the runner hit an infrastructure failure, such as an unexpected I/O
  error, malformed artifact write, panic recovery, or invalid internal state;
- `not_run`: the runner did not attempt the fixture.

### 7.4 Achieved Readiness

The report should compute `achieved_readiness` from gates:

- `pass` only when every gate whose `required_for` contains `pass` has
  `status: "pass"`, required artifacts are present, and optional KiCad evidence
  is present when required;
- `candidate` when every gate whose `required_for` contains `candidate` has
  `status: "pass"` or an explicitly allowlisted `status: "warn"`, required
  candidate artifacts are present, and any metadata required KiCad ERC/DRC
  evidence is present and clean. A skipped KiCad gate prevents `candidate` when
  `require_erc` or `require_drc` is true. Warning gates never permit `pass`;
- `expected_fail` when blockers match known expected blockers;
- `blocked` when execution cannot produce meaningful evidence or when failed
  gates do not match known expected blockers. For a runnable fixture that
  reaches this achieved readiness after execution, the top-level `status`
  should be `failed`, not `blocked`.

An `expected_fail` achieved readiness requires both metadata `known_gaps` and
specific report issue codes that justify the failure. A fixture declared
`expected_fail` but achieving `candidate` or `pass` should set
`matches_expectation: false` and top-level `status: "unexpected_pass"` so the
runner can require metadata review before promotion.

The runner should compare `achieved_readiness` with `declared_readiness` and
set `matches_expectation` accordingly. A fixture whose
`declared_readiness: "expected_fail"` and `achieved_readiness:
"expected_fail"` should use top-level `status: "expected_fail"`, not
`failed`.

## 8. Fixture-Specific Promotion Criteria

### 8.1 `led_indicator_kicad_smoke`

Can promote to `candidate` when:

- project, schematic, and PCB write cleanly;
- LED and resistor footprints contain resolved pads;
- VCC and GND nets are assigned correctly;
- local route endpoint binding proves copper touches same-net pad anchors;
- internal validation has no blocking issues;
- KiCad ERC/DRC passes or produces only explicitly allowed findings when
  `KICADAI_KICAD_CLI` is configured.

Can promote to `pass` after repeated deterministic optional KiCad-backed runs
produce clean artifacts.

### 8.2 `connector_led_kicad_smoke`

Can promote to `candidate` when:

- connector-to-LED intent creates required nets;
- connector and LED block pads are assigned to the same intended nets;
- inter-block route candidates are attempted;
- each required inter-block route physically contacts both endpoint pad
  anchors;
- contact graph completion shows the same-net component is connected;
- KiCad DRC is clean when configured.

It must remain `expected_fail` while route-contact misses or incomplete same-net
graphs exist.

### 8.3 `i2c_sensor_breakout_candidate`

Can promote to `candidate` when:

- VCC, GND, SDA, and SCL are represented as required nets;
- sensor, pullup, and connector pads resolve to concrete footprints;
- promoted inter-block route candidates are attempted for all required nets;
- same-net contact graph evidence is complete or intentionally accepted as
  unrouted with an explicit lower acceptance level;
- internal board validation passes for `erc-drc`;
- optional KiCad ERC/DRC evidence is clean when configured.

It should not promote to `pass` until it is stable across repeated runs and the
documentation describes it as a default-quality generated breakout example.

### 8.4 `opamp_headphone_buffer_kicad_candidate`

This fixture should stay `expected_fail` until the amplifier-specific gaps are
closed:

- verified op-amp and output connector component evidence;
- output DC-blocking and load-protection realization;
- feedback-loop placement constraints;
- supply decoupling placement constraints;
- high-current output track width evidence;
- headphone load safety evidence;
- analog layout proof;
- KiCad ERC/DRC-clean artifacts.

It should not be promoted based only on schematic generation or parseable PCB
output.

## 9. AI-Actionable Repair Guidance

Promotion blockers should be converted into repair suggestions. Suggested
categories:

- `missing_component_evidence`: choose a verified component or relax
  acceptance;
- `missing_pinmap`: add or verify symbol-to-footprint pin mapping;
- `route_contact_miss`: rerun routing with concrete pad anchors;
- `route_incomplete`: adjust placement or route strategy;
- `wrong_net_assignment`: inspect footprint pad mapping and net transfer;
- `missing_outline`: create or repair board outline;
- `drc_clearance`: increase spacing or adjust placement;
- `missing_kicad_cli`: configure `KICADAI_KICAD_CLI` for optional proof;
- `missing_artifact`: keep artifacts or fix artifact path handling.

Repair text should be deterministic, short, and suitable for CLI JSON output.

## 10. CLI And Test Behavior

### 10.1 Default Tests

Default tests must remain KiCad-independent. They may validate metadata, report
schema, synthetic evidence, and internal promotion gates.

### 10.2 Optional KiCad Tests

Optional tests should continue using the existing `KICADAI_KICAD_CLI` model.
They should:

- skip cleanly when the variable is absent;
- never report skipped KiCad execution as a clean pass;
- preserve artifacts when the optional test harness requests them;
- fail when a `pass` fixture cannot produce required KiCad reports.

### 10.3 CLI Output

When the CLI runs a KiCad-backed request with artifact output enabled, it should
include promotion evidence in the normal JSON response and write the promotion
report artifact.

The documentation should continue assuming users invoke the compiled `kicadai`
binary, not `go run`.

## 11. Documentation Requirements

Update documentation to explain:

- what the optional KiCad-backed tier is;
- what `expected_fail`, `candidate`, `pass`, and `blocked` mean;
- how to run the optional tier;
- how to interpret promotion reports;
- why a fixture can be useful while still being `expected_fail`;
- what evidence is needed to promote each current fixture.

Likely docs:

- `examples/design/kicad-backed/README.md`;
- `docs/intent-planning.md`;
- `docs/validation-and-analysis.md`;
- `docs/kicadai-agent-skill.md`;
- `specs/ROADMAP.md`.

## 12. Acceptance Criteria

This project is complete when:

- every optional KiCad-backed fixture produces or can produce a normalized
  promotion report;
- metadata validation catches invalid readiness declarations;
- internal gates classify writer correctness, connectivity, route completion,
  physical-rule, and artifact evidence deterministically;
- optional KiCad evidence is represented as present, skipped, allowed-finding,
  or blocking;
- at least one fixture has an explicit promotion decision based on the new
  gates;
- no `expected_fail` fixture can pass silently without blockers;
- docs explain how to promote fixtures and how to read blockers;
- `go test ./...` remains KiCad-independent.

## 13. Policy Decisions

- CI may skip optional KiCad-backed fixtures when KiCad is unavailable, but a
  skipped KiCad gate must never promote a fixture to `candidate` or `pass` when
  metadata requires ERC/DRC evidence.
- Promotion reports should be written under the run output's `.kicadai/`
  directory. The optional test harness may additionally preserve that output in
  a temporary or caller-provided artifact directory, but checked-in generated
  reports are not required.
- `pass` should require deterministic report output and stable golden tests.
  Repeated real KiCad runs are useful local evidence, but the default pass gate
  should rely on deterministic internal report generation plus optional
  KiCad-backed artifact checks when configured.
- KiCad warnings block `pass` unless allowlisted. KiCad warnings may allow
  `candidate` only when explicitly allowlisted in metadata and documented in
  the promotion report.
