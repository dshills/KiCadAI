# KiCad-Backed Block DRC Corpus Specification

Date: 2026-06-25

## Summary

KiCadAI already has a circuit block verification harness, internal board
validation, writer correctness checks, optional KiCad ERC/DRC stages, and a
small opt-in smoke path for selected manifests. The next gap is turning that
ad hoc KiCad evidence into a named, reportable, opt-in corpus of generated
block projects that can prove which reusable blocks are currently DRC-clean in
real KiCad.

This specification defines a KiCad-backed block DRC corpus that promotes
selected block verification manifests from "structurally checked" to
"KiCad-backed clean" without making normal `go test ./...` depend on KiCad,
external libraries, or local GUI state.

## Problem

Internal checks prove that generated block projects are parseable and
structurally meaningful, but they are not a substitute for KiCad DRC/ERC. The
current harness can run KiCad when configured, yet the evidence is not organized
as a deliberate corpus with:

- explicit fixture readiness states;
- stable artifact layout;
- clear skip/block/pass semantics;
- corpus-wide summaries;
- documented allowlist policy;
- regression tests for the opt-in path;
- roadmap evidence that says which block families have real KiCad proof.

Without this corpus, AI callers cannot distinguish:

- a block that has only manifest-level PCB evidence;
- a block that was generated and internally validated;
- a block that was actually checked by KiCad DRC/ERC on this machine;
- a block that is expected to fail until writer, placement, routing, or
  footprint gaps are closed.

## Goals

- Define a first-class "KiCad DRC corpus" for selected built-in block
  verification manifests.
- Keep default tests KiCad-independent.
- Let manifests declare corpus membership, KiCad proof expectations, expected
  readiness, and allowed findings.
- Generate deterministic corpus reports suitable for AI-facing status and
  roadmap evidence.
- Store KiCad report artifacts in stable per-case output directories when the
  corpus is run.
- Fail required corpus runs when KiCad is unavailable, when generated projects
  are stale, or when non-allowlisted DRC/ERC findings appear.
- Distinguish `pass`, `skip`, `expected_fail`, `blocked`, and `not_in_corpus`.
- Provide actionable issue metadata for failures so future repair work can map
  KiCad findings back to block, writer, placement, routing, or library causes.

## Non-Goals

- Do not require KiCad for normal unit tests or default block verification.
- Do not guarantee fabrication readiness or manufacturer acceptance.
- Do not expand routing or placement behavior as part of this spec.
- Do not invent new DRC rules outside KiCad.
- Do not mutate imported user projects.
- Do not require network access or remote package sources.
- Do not make all built-in blocks DRC-clean in the first implementation. The
  corpus must support explicit expected-fail states while quality improves.

## Current Foundations

Existing implementation to reuse:

- `internal/blocks` verification manifests under
  `internal/blocks/testdata/verification/<case-id>/manifest.json`.
- `block verify` CLI support for built-ins, suites, single cases, output
  directories, overwrite policy, and KiCad CLI flags.
- Manifest `expected.pcb.require_board_validation`.
- Manifest `expected.erc_drc.require_erc`, `require_drc`, `required`,
  `allowed_codes`, and `expected_issues`.
- Optional versus required KiCad ERC/DRC behavior.
- Stable report artifact descriptions and check-context signatures.
- Local opt-in smoke tests gated by `KICADAI_RUN_KICAD_CLI=1`.
- Writer correctness, board validation, generated project manifests, and
  repair-bundle provenance.

The new work should extend those surfaces rather than creating a separate
runner.

## Corpus Model

### Corpus Membership

Each verification manifest may declare KiCad corpus metadata:

```json
{
  "expected": {
    "kicad_corpus": {
      "include": true,
      "tier": "smoke",
      "readiness": "candidate",
      "requires_erc": false,
      "requires_drc": true,
      "expected_status": "pass",
      "allowed_codes": [],
      "expected_issues": [],
      "notes": "Generated layout should be DRC-clean once routed."
    }
  }
}
```

The exact Go structure may reuse or embed existing `expected.erc_drc` fields
where practical, but the report must expose corpus-specific membership and
readiness.

### Tiers

Supported tiers:

- `smoke`: small, fast representative cases that local developers can run
  frequently.
- `block`: one or more cases per implemented built-in block.
- `layout`: generated PCB cases that exercise placement, routing, zones, and
  board validation.
- `regression`: cases added after a bug fix to prevent a known KiCad DRC/ERC
  regression.

The initial implementation should support filtering by tier even if only
`smoke` and `block` are populated.

### Readiness

Supported readiness values:

- `candidate`: intended to pass once KiCad is available; failure is blocking
  for required corpus runs.
- `expected_fail`: intentionally tracked as not yet clean; failure is reported
  but does not fail non-strict exploratory corpus runs.
- `blocked`: known blocked by an explicit project gap such as unsupported
  routing, missing footprint evidence, or known writer limitation.
- `reference`: curated known-good case that should never drift without review.

### Status

Corpus case results must use stable statuses:

- `pass`: KiCad ran and findings matched expectations.
- `skip`: KiCad or an output directory was unavailable in optional mode.
- `expected_fail`: KiCad ran and failure matched an expected-fail corpus case.
- `blocked`: required evidence was missing or unexpected findings appeared.
- `not_in_corpus`: manifest was verified by normal harness paths but not
  selected for the KiCad corpus.

## Manifest Requirements

For a case included in the corpus:

- `expected.pcb.require_board_validation` should be true unless the case is
  schematic/ERC-only.
- `expected.erc_drc.require_drc` should be true for PCB corpus cases when the
  case is a pass candidate.
- `expected.erc_drc.require_erc` should be true only when the generated
  schematic is expected to be ERC-clean or has explicit allowlisted ERC
  findings.
- `allowed_codes` must be narrow and documented. Broad message substring
  allowlists are discouraged.
- `expected_issues` may be used for expected-fail cases but must include a
  note explaining why the failure is accepted.
- Expected-fail and blocked cases must preserve AI-facing notes that explain
  the missing implementation gap.

## CLI Requirements

Extend block verification with corpus-oriented filtering and reporting.
Acceptable CLI shapes include either new flags on `block verify` or a new
subcommand, as long as behavior is documented and covered by tests.

Required behavior:

- Run only corpus cases:

```sh
kicadai --json --builtins --kicad-corpus block verify
```

- Filter by tier:

```sh
kicadai --json --builtins --kicad-corpus --kicad-corpus-tier smoke block verify
```

- Require KiCad evidence:

```sh
kicadai --json --builtins \
  --kicad-corpus \
  --output ./out/block-kicad-corpus \
  --overwrite \
  --kicad-cli /path/to/kicad-cli \
  --require-drc \
  block verify
```

The final CLI design should preserve existing `block verify` behavior for users
who do not pass corpus flags.

## Output Requirements

When an output directory is provided, each corpus case should produce a stable
layout:

```text
<output>/
  corpus-summary.json
  <case-id>/
    project/
      <generated KiCad project files>
    reports/
      erc.json
      drc.json
      writer.json
      board-validation.json
      corpus-result.json
```

Reports may omit files for skipped stages, but `corpus-result.json` and
`corpus-summary.json` should explain why the stage was skipped.

The summary must include:

- total selected cases;
- counts by status;
- counts by tier;
- counts by block ID;
- KiCad CLI path and version when available;
- units and command options used;
- generated artifact root;
- stale project/provenance status;
- unexpected finding summaries;
- allowlisted finding summaries;
- expected-fail summaries.

## Report Model

Each corpus case result should expose:

- case ID;
- block ID;
- tier;
- readiness;
- selected evidence level;
- generated project path when present;
- KiCad CLI context;
- writer correctness summary;
- internal board validation summary;
- ERC result summary;
- DRC result summary;
- status;
- blocking issues;
- allowlisted issues;
- expected issues;
- repair category hints where available;
- notes from the manifest.

The report model should remain JSON-stable for AI callers.

## KiCad Policy

Default behavior:

- If corpus mode is not requested, existing block verification behavior stays
  unchanged.
- If corpus mode is requested without `--require-erc` or `--require-drc` and
  KiCad is unavailable, selected cases may report `skip`.
- If corpus mode is requested with required KiCad evidence, missing KiCad,
  missing output directory, or failed command execution is `blocked`.
- If `KICADAI_RUN_KICAD_CLI=1` is set in tests, opt-in integration tests may
  run real KiCad. Otherwise they must skip.

KiCad command behavior:

- Use existing KiCad CLI discovery and report parsing paths.
- Prefer JSON report output.
- Use millimeter units for consistency.
- Preserve DRC `--refill-zones` behavior according to current ERC/DRC adapter
  policy.
- Do not save mutated generated boards unless an explicit existing adapter
  policy already permits it.

## Initial Corpus Candidates

The initial corpus should start with a small, honest set:

- `led_indicator_default`: smoke candidate.
- `connector_breakout_4pin`: smoke candidate.
- `voltage_regulator_3v3`: block candidate if current routing and zones are
  sufficient.
- `esd_protection_5v`: expected-fail or candidate depending on current board
  route/footprint evidence.
- `reverse_polarity_schottky`: expected-fail or candidate depending on current
  power-path routing evidence.
- `crystal_oscillator_default` and `canned_oscillator_default`: expected-fail
  until KiCad-backed timing fixture layouts are clean, unless local evidence
  proves otherwise.

Implementation should not mark a case `pass` by assertion. A case becomes
`pass` only when the opt-in KiCad run actually succeeds or when a checked-in
fake-runner unit test is explicitly testing report logic rather than real DRC
cleanliness.

## Testing Requirements

Default test coverage:

- manifest parsing and validation for corpus metadata;
- corpus case selection and tier filtering;
- optional KiCad unavailable produces `skip`;
- required KiCad unavailable produces `blocked`;
- fake KiCad runner pass produces `pass`;
- fake KiCad runner unexpected finding produces `blocked`;
- fake KiCad runner expected-fail case produces `expected_fail`;
- summary counts are deterministic;
- output paths and artifact descriptors are stable;
- existing non-corpus block verification behavior is unchanged.

Opt-in local coverage:

```sh
KICADAI_RUN_KICAD_CLI=1 \
KICADAI_KICAD_CLI=/path/to/kicad-cli \
go test ./internal/blocks/...
```

The opt-in test should skip with a clear message if KiCad is not configured.

## Documentation Requirements

Update:

- `docs/circuit-block-verification.md` with corpus metadata, CLI examples, and
  status meanings.
- `docs/circuit-block-readiness.md` if corpus readiness is surfaced there.
- `README.md` with the current user-facing capability.
- `specs/ROADMAP.md` after implementation to reflect corpus coverage and
  remaining gaps.

## Acceptance Criteria

- A checked-in spec and plan exist for the KiCad-backed block DRC corpus.
- Manifest metadata can identify corpus membership, tier, readiness, and
  expected status.
- `block verify` can run selected corpus cases without changing existing
  default behavior.
- Corpus summaries expose deterministic case, tier, block, and status counts.
- Required KiCad evidence fails closed when unavailable.
- Optional KiCad evidence skips cleanly when unavailable.
- Fake-runner tests cover pass, blocked, skipped, and expected-fail outcomes.
- At least two built-in manifests are included in the initial smoke corpus.
- Documentation explains how to run the corpus locally with `kicadai`.
