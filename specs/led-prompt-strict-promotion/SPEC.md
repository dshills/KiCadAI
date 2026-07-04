# LED Prompt Strict Promotion

## Status

Draft specification for the next roadmap item after AI-lane candidate promotion.

## Purpose

KiCadAI can now turn:

```sh
kicadai --text "make a simple LED indicator board" --output ./out/ai_led --overwrite intent create
```

into a generated KiCad project with `data.ai_status.status: "candidate"`.
That is enough for a controlled AI handoff, but it is not strict promotion
readiness. The generated project still records warning or skipped evidence in
`.kicadai/design-promotion.json`, so the promotion report currently reaches
`achieved_readiness: "blocked"` with `status: "failed"` when KiCad CLI evidence
is not configured.

This project closes that gap for the simple LED prompt. The target is not
arbitrary AI board generation. The target is one narrow, deterministic prompt
that can progress from AI-lane `candidate` to strict promotion `candidate` and,
when real KiCad checks pass, `pass`.

## Current Evidence

The latest generated LED prompt output is structurally useful:

- project, schematic, and PCB files are written;
- `data.ai_status.status` is `candidate`;
- workflow artifacts, validation summary, retry state, manifest, rationale, and
  promotion report are written;
- no placement outside-board blocker remains;
- no disconnected-pad blocker remains in the default structural lane.

The strict promotion report still identifies these gaps:

- `component_selection`: connector block components have no selected
  `component_id` or `component_query`;
- `placement`: `indicator.VCC` does not resolve to a generated PCB pad for one
  inter-block endpoint;
- `routing`: fixed nets are preserved and skipped as info findings;
- `routing`: GND route branches do not have explicit net class evidence;
- `writer_correctness`: schematic-to-PCB transfer has unverified pinmap/pad-net
  evidence because no library index is provided;
- `writer_correctness`: KiCad round trip is skipped when no KiCad CLI path is
  configured;
- `validation`: KiCad ERC/DRC is skipped when no KiCad CLI path is configured;
- `kicad_checks`: promotion gate is skipped without configured KiCad CLI.

## Goals

1. Make the simple LED prompt produce promotion `candidate` without requiring
   fabrication-level proof when KiCad CLI is unavailable.
2. Make the same prompt produce promotion `pass` when real KiCad ERC/DRC and
   round-trip checks are configured and clean.
3. Remove warning-level gaps that are under KiCadAI control:
   component-selection placeholder warnings, missing endpoint-to-pad mappings,
   missing net class evidence, and unverified pinmap evidence for the generated
   LED topology.
4. Preserve fail-closed behavior for unsupported prompts, unsafe prompts,
   missing tools, and richer topologies.
5. Keep generated evidence machine-readable for AI agents.

## Non-Goals

- Do not claim broad natural-language PCB generation.
- Do not require KiCad CLI for default `go test ./...`.
- Do not relax promotion gates globally just to make the LED prompt pass.
- Do not hide warning evidence from reports.
- Do not invent external distributor/procurement evidence for catalog parts
  that are not modeled.
- Do not hand-author special-case KiCad files for the prompt outside the normal
  planner and workflow path.

## Definitions

- **AI-lane candidate**: `data.ai_status.status == "candidate"`. The generated
  output exists and has no blocking issue, but warnings or skipped evidence may
  remain.
- **Promotion candidate**: `.kicadai/design-promotion.json` reaches
  `achieved_readiness: "candidate"` and `status: "warn"` or better. Required
  candidate gates cannot be failed, blocked, skipped, or not run.
- **Promotion pass**: `.kicadai/design-promotion.json` reaches
  `achieved_readiness: "pass"` and `status: "pass"`. Required pass gates must
  pass, including KiCad-backed gates when required.
- **Default structural lane**: the no-KiCad default path used by ordinary tests.
- **KiCad-backed lane**: opt-in path using `--kicad-cli` or
  `KICADAI_KICAD_CLI`, with `--require-erc` and `--require-drc` where strict
  promotion is requested.

## Required Behavior

### 1. Component Selection Evidence

The LED prompt must not emit warning-level component-selection issues for
connector block placeholder components.

Acceptable solutions:

- select checked-in component records for generated connector, LED, and resistor
  roles; or
- mark block-local generated connectors as structurally verified synthetic
  components with explicit evidence that they do not require catalog selection
  for structural acceptance.

The result must remain explicit in workflow evidence. Agents should be able to
tell whether the part came from the catalog or from a verified synthetic block
policy.

### 2. Endpoint-To-Pad Mapping

The generated LED prompt must resolve every inter-block endpoint used by
placement and routing to an actual generated PCB pad or validated local-route
anchor.

Specifically, `indicator.VCC` must resolve cleanly. The fix should address the
block pin/port map, schematic-to-PCB transfer, or local fragment metadata rather
than suppressing the issue.

### 3. Net Class Evidence

Generated power/ground nets used by the LED prompt must have explicit net class
evidence. At minimum:

- GND must not emit `MISSING_NET_CLASS`;
- the LED signal net must retain appropriate signal-class defaults;
- net class evidence must be serialized into workflow summaries and PCB output
  where the writer supports it.

The implementation may add deterministic default classes for simple generated
designs, but it must not overwrite user-provided net class policy.

### 4. Route Completion And Fixed-Net Findings

The LED prompt should not count expected fixed-net preservation as a promotion
warning when the preserved copper is connected and electrically meaningful.

Required behavior:

- fixed-net skipped evidence remains available as info/debug evidence;
- promotion route-completion gates do not downgrade readiness for non-blocking
  preserved routes;
- any actual route graph split, contact miss, disconnected pad, or no-legal-path
  finding remains blocking or warning as appropriate.

### 5. Writer Correctness And Pinmap Evidence

The LED prompt must provide enough resolver-backed or verified-template-backed
symbol/footprint pinmap evidence that writer correctness does not warn:

- generated schematic symbols map to PCB footprints deterministically;
- pad nets are transferred or hydrated for LED, resistor, and connector pads;
- the generated transaction includes pad-net hints where available;
- warning `schematic-to-PCB transfer has no pad net hints` no longer appears
  for the LED prompt.

If the library resolver is not configured, verified fallback templates may be
used, but the evidence must be explicit and narrow to the built-in LED prompt
topology.

### 6. KiCad-Backed Promotion Path

Default tests must still skip real KiCad checks. When KiCad CLI is configured:

```sh
kicadai --kicad-cli /path/to/kicad-cli \
  --require-erc --require-drc \
  --text "make a simple LED indicator board" \
  --output ./out/ai_led \
  --overwrite \
  intent create
```

the workflow must:

- run KiCad ERC/DRC through existing check infrastructure;
- persist KiCad artifacts under `.kicadai/`;
- expose `kicad_checks` stage evidence;
- classify KiCad findings through promotion gates;
- reach promotion `pass` only if the KiCad-backed checks are clean or explicitly
  allowed by existing policy.

If KiCad CLI is missing, unavailable, or fails to run, the system must report
`tool_error` or a skipped/blocked promotion gate honestly.

### 7. Reports And Artifacts

The LED prompt output must include:

- root `.kicad_pro`, `.kicad_sch`, and `.kicad_pcb`;
- `.kicadai/workflow-result.json`;
- `.kicadai/validation-summary.json`;
- `.kicadai/retry-state.json`;
- `.kicadai/design-rationale.json`;
- `.kicadai/design-promotion.json`;
- `.kicadai/manifest.json`;
- `.kicadai/transaction.json`.

The AI-lane status and promotion status must remain separate. Documentation and
tests should not conflate them.

## Acceptance Criteria

Default structural lane:

- `go test ./...` passes without KiCad CLI.
- The LED prompt exits successfully.
- `data.ai_status.status == "candidate"` or better.
- `.kicadai/design-promotion.json` reaches `achieved_readiness: "candidate"`
  if KiCad-specific gates are optional and legitimately skipped by policy.
- No promotion issues remain for component selection placeholders,
  `indicator.VCC` unresolved endpoint, missing GND net class, route graph split,
  disconnected pad, or pinmap-unverified writer correctness.
- KiCad-specific missing-tool evidence is still visible and not misreported as
  clean proof.

KiCad-backed lane:

- Optional tests skip when `KICADAI_KICAD_CLI` is unset.
- When `KICADAI_KICAD_CLI` is set, the LED prompt runs KiCad ERC/DRC.
- `kicad_checks` is not skipped.
- Promotion reaches `pass` if checks are clean, or reports precise KiCad-backed
  blockers if KiCad finds real issues.

## Test Strategy

- Unit tests for component selection evidence classification.
- Unit tests for endpoint-to-pad resolution of `indicator.VCC`.
- Unit tests for default net class assignment on generated GND/VCC/signal nets.
- Workflow golden test for the LED prompt promotion report.
- Optional KiCad smoke test gated by `KICADAI_KICAD_CLI`.
- Regression tests proving unsupported or unsafe prompts still fail closed.
- Full `go test ./...` before completion.

## Risks

- Over-relaxing promotion gates could make weaker designs look ready.
- Hard-coding LED-specific behavior too deeply could block future topology
  generalization.
- KiCad ERC may require schematic symbol electrical pin metadata that the
  current writer does not fully model.
- DRC may expose board-outline, clearance, netclass, or footprint details not
  covered by current structural checks.
- Optional KiCad tests can become flaky if they depend on user-local KiCad
  versions; tests must report version/path evidence.

## Open Questions

- Should default structural promotion `candidate` allow skipped KiCad gates, or
  should strict promotion `candidate` require configured KiCad checks and reserve
  no-KiCad output for AI-lane `candidate` only?
- Should verified synthetic connector parts be acceptable for strict promotion
  candidate, or must every connector select a catalog-backed component record?
- Should `pass` require KiCad round-trip in addition to ERC/DRC for this prompt?
- Should KiCad-backed failures be tracked as an `expected_fail` fixture until
  the writer is fully ERC clean?

