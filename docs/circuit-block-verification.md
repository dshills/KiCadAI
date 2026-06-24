# Circuit Block Verification

The circuit block verification harness proves reusable blocks against checked-in
manifest expectations. It is the quality gate between "a block can generate
operations" and "a block has evidence an AI workflow can cite."

## What It Checks

Each manifest can verify:

- request validity for a built-in block;
- expected symbols, footprints, refs or ref prefixes, ports, nets, and pin
  memberships;
- PCB placements, footprint IDs, pad-net assignments, required routes, and
  required zones;
- PCB realization metadata, including required block-local route IDs and timing
  fixture satisfaction where a block exposes that evidence;
- internal board validation for generated block projects when explicitly
  requested;
- writer correctness for generated KiCad projects;
- optional KiCad ERC/DRC evidence through `kicad-cli`.

Default tests do not require KiCad. Board validation writes a generated project
when requested, then runs deterministic in-process validation logic by default.
ERC/DRC stages are visible as skipped unless a manifest or CLI flag requires
them, and required KiCad evidence fails verification when `kicad-cli` or an
output directory is unavailable.

## Evidence Levels

- `definition_only`: block definition exists, but generated output is not
  asserted.
- `schematic_verified`: generated schematic semantics are asserted.
- `transfer_verified`: schematic-to-PCB transfer expectations are asserted.
- `pcb_verified`: local PCB placement/pad/route/zone expectations are asserted.
- `erc_drc_verified`: KiCad ERC/DRC evidence is required.
- `reference_verified`: strongest level, reserved for reference-backed evidence.

## PCB Evidence Fields

The `expected.pcb` section supports deterministic PCB evidence without requiring
KiCad:

- `require_realization` (boolean): run `RealizeBlockPCB` and fail on blocking
  realization issues. Use this when realization evidence is needed without
  writing a generated project; it is redundant when `require_board_validation`
  is true because board validation takes precedence and always performs
  realization.
- `require_board_validation` (boolean): write a generated project and run
  internal board validation. This catches invalid pad-net assignments, route
  completion gaps, zone issues, and related structural problems without needing
  KiCad CLI. Board validation implicitly enables the realization stage.
- `allow_unrouted` (boolean): only applies when `require_board_validation` is
  true; allow intentionally incomplete local routing during board validation.
  It has no effect without board validation.
- `required_local_routes` (list of strings): assert realized block-local route
  IDs exist.
- `timing_fixtures` (list of objects): each object must contain an `id` string
  to identify the realized fixture and may optionally require `satisfied`
  (boolean), `required_findings` (list of strings), or `forbidden_findings`
  (list of strings). If `satisfied` is omitted, only fixture presence and
  finding lists are checked. Finding strings match internal timing finding IDs
  exactly. `satisfied` checks the fixture's overall realized pass/fail state;
  `required_findings` and `forbidden_findings` are additional ID presence or
  absence assertions and do not change that overall state.

Blocking realization issues are fatal generator or metadata problems that make
the requested PCB fragment unsafe to trust, such as missing realized components,
unresolved route endpoints, or invalid conditional realization metadata. Local
route and timing fixture IDs come from the block's PCB realization metadata and
can be discovered with `kicadai --json --builtins block show <block_id>` or by
inspecting a successful `kicadai --json --builtins block realize-pcb <block_id>`
result.

Common timing finding IDs include:

- `timing.decoupling.present`
- `timing.decoupling.proximity`
- `timing.clock_routes.length`
- `timing.ground_return.present`
- `timing.reset_programming.route_length`
- `timing.programming.ground_reference`

These IDs identify emitted timing check results. Use `forbidden_findings` for
findings that must not appear in a passing fixture, and `required_findings` only
when a manifest intentionally expects a diagnostic to be present.

Protection blocks currently require realization evidence for modeled
entry-anchor and power-path routes. The standard ESD manifest requires route
IDs `esd_signal_entry_to_tvs` and `esd_tvs_to_ground`; the standard
diode-based reverse-polarity manifest requires `raw_input_to_diode` and
`diode_to_protected_output`. Manifests declare these IDs in
`expected.pcb.required_local_routes`; they are route evidence requirements, not
`required_findings` diagnostics. The verification engine performs literal
string matching between
`expected.pcb.required_local_routes[]` and realized
`realization.local_routes[].id` values; matching is case-sensitive. Anchor
presence is verified through required routes whose endpoint `anchor_id` values
reference `realization.entry_anchors[].id`. These checks prove block-local
route evidence, not surge/thermal behavior or final KiCad-backed board DRC.
The standard manifests assert route IDs that encode the intended signal,
ground, raw-input, and protected-output paths.

Example protection route requirement:

```json
{
  "expected": {
    "pcb": {
      "required_local_routes": [
        "esd_signal_entry_to_tvs",
        "esd_tvs_to_ground"
      ]
    }
  }
}
```

Example:

```json
{
  "expected": {
    "pcb": {
      "require_board_validation": true,
      "allow_unrouted": false,
      "required_local_routes": ["xtal1_load", "xtal2_load"],
      "timing_fixtures": [
        {
          "id": "crystal_loop",
          "satisfied": true,
          "forbidden_findings": ["timing.clock_routes.length"]
        }
      ]
    }
  }
}
```

## ERC/DRC Manifest Fields

The `expected.erc_drc` section controls KiCad-backed evidence:

- `required` (boolean): require both ERC and DRC unless narrower flags are set.
- `require_erc` (boolean): require ERC evidence.
- `require_drc` (boolean): require DRC evidence.
- `allowed_codes` (list of strings): allow known KiCad finding codes.
- `expected_issues` (list of strings): require specific finding codes, IDs,
  rules, or message substrings to appear. Prefer stable codes, IDs, and rules
  over message substrings because KiCad messages can vary by version.

If `expected.evidence_level` is `erc_drc_verified` or `reference_verified`, ERC
and DRC are required even without `expected.erc_drc.required`; an explicit
`expected.erc_drc.required: false` does not override the evidence level. If the
evidence level does not require KiCad checks, `required`, `require_erc`, and
`require_drc` decide whether missing KiCad evidence blocks. Optional ERC/DRC
expectations can be expressed with `allowed_codes` or `expected_issues` while
leaving the required flags false; they produce a visible skipped stage when
KiCad CLI or an output directory is unavailable.

Example:

```json
{
  "expected": {
    "erc_drc": {
      "require_erc": true,
      "require_drc": true,
      "allowed_codes": ["unconnected_items"]
    }
  }
}
```

Timing blocks currently assert local routes and timing fixtures. Protection
blocks assert realization evidence for modeled placement, entry-anchor, and
power-path route metadata. Entry anchors are evidence points until higher-level
board composition maps them to physical connector pads or board-edge features.

## CLI Usage

Run all built-in manifests:

```sh
kicadai --json --builtins block verify
```

Run one manifest:

```sh
kicadai --json \
  --case ./internal/blocks/testdata/verification/led_indicator_default/manifest.json \
  block verify
```

Run a suite and retain generated projects:

```sh
kicadai --json \
  --suite ./internal/blocks/testdata/verification \
  --output ./out/block-verification \
  --overwrite \
  block verify
```

Require KiCad evidence:

```sh
kicadai --json \
  --builtins \
  --output ./out/block-verification \
  --overwrite \
  --kicad-cli <path-to-kicad-cli> \
  --require-erc \
  --require-drc \
  block verify
```

Optional ERC/DRC expectations are reported as a skipped `erc_drc` stage when no
output directory or KiCad CLI is available. Required ERC/DRC expectations block
with an explicit reason. When checks run, report artifacts are included in the
result.

## Adding A Built-In Manifest

Add a new directory under:

```text
internal/blocks/testdata/verification/<case-id>/manifest.json
```

Keep the case ID lowercase with underscores. Start with
`schematic_verified`, then raise the evidence level only after adding the
corresponding PCB, writer, or KiCad-backed expectations.

Use `ref_prefix` when refs are deterministic but contain generated tokens. Use
explicit `ref` only when the block always emits that exact reference.

Custom manifests can live outside the repository and be passed directly with
`--case` or grouped under any directory passed with `--suite`.

## Golden Reports

CLI report snapshots are stored in:

```text
cmd/kicadai/testdata/golden/block_verification/
```

Refresh them intentionally:

```sh
go test ./cmd/kicadai -run TestRunBlockVerificationGoldens -update-block-verification-goldens
```

The golden snapshots intentionally normalize away verbose operation payloads.
Lower-level block tests cover operation details; report goldens lock the stable
agent-facing contract.

## Design Workflow Integration

`design create` now includes block verification evidence in the
`block_planning` stage summary. Missing evidence is a warning. Fabrication
candidate requests are halted only when a circuit block claims
fabrication-level readiness without `erc_drc_verified` or
`reference_verified` evidence.
