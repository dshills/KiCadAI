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
- writer correctness for generated KiCad projects;
- optional KiCad ERC/DRC evidence through `kicad-cli`.

Default tests do not require KiCad. ERC/DRC stages are skipped unless a manifest
or CLI flag requires them.

## Evidence Levels

- `definition_only`: block definition exists, but generated output is not
  asserted.
- `schematic_verified`: generated schematic semantics are asserted.
- `transfer_verified`: schematic-to-PCB transfer expectations are asserted.
- `pcb_verified`: local PCB placement/pad/route/zone expectations are asserted.
- `erc_drc_verified`: KiCad ERC/DRC evidence is required.
- `reference_verified`: strongest level, reserved for reference-backed evidence.

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
