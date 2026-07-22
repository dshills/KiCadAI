# Function-Level Circuit Workflow

Function-level requests describe semantic operations and ports without KiCad
pins, pads, coordinates, support-component instances, or routes. They use the
same `kicadai.circuit-graph.v1` envelope as explicit graphs, with a `synthesis`
object instead of `components` and `nets`.

## Discover The Installed Contract

Do not infer operation names or parameter shapes from examples. Query the
registry used by validation:

```sh
kicadai capability generation --json |
  jq '.data.function_level_contract'
```

Render the current operation table directly from that response:

```sh
kicadai capability generation --json |
  jq -r '.data.function_level_contract.operations[] |
    [.name,
     (.required_parameters | map(.name + ":" + .value_kind + if (.unit // "") == "" then "" else "[" + .unit + "]" end) | join(",")),
     (.optional_parameters | map(.name + ":" + .value_kind + if (.unit // "") == "" then "" else "[" + .unit + "]" end) | join(",")),
     (.endpoint_roles | map(.role + "=" + (.functions | join("|"))) | join(",")),
     .proven_readiness] | @tsv'
```

The contract includes:

- stable operation names and supported component roles;
- required and optional parameters, value kinds, and units;
- required and optional semantic endpoint roles;
- the readiness reached by checked-in evidence;
- global unit conventions, limits, and unsupported claims.

Published operations are validated from this registry. Unlisted names are
rejected unless they belong to explicitly retained internal compatibility
corpora; those legacy labels are not public claims of supported behavior or
readiness.

## Complete Public Example

The identity-neutral
[`function_low_side_status_driver.json`](../examples/circuit-graph/function_low_side_status_driver.json)
describes a logic-controlled NPN LED driver. It uses the published
`low_side_switch` operation and explicit external power and control interfaces.
No provider call is involved.

From the repository root, preflight the request without writing a project:

```sh
kicadai \
  --request examples/circuit-graph/function_low_side_status_driver.json \
  circuit preflight > /tmp/function-preflight.json

jq '{ok, ready: .data.ready_for_write, gates: .data.gates, issues}' \
  /tmp/function-preflight.json
```

Create the project offline using configured KiCad library roots:

```sh
kicadai \
  --symbols-root "$KICADAI_SYMBOLS_ROOT" \
  --footprints-root "$KICADAI_FOOTPRINTS_ROOT" \
  --request examples/circuit-graph/function_low_side_status_driver.json \
  --output /tmp/function-low-side-driver \
  --overwrite \
  circuit create > /tmp/function-create.json
```

Inspect stage evidence, outstanding external evidence, and written artifacts:

```sh
jq '{ok,
     stages: [.data.workflow.stages[] | {name, status, issues}],
     outstanding: .data.outstanding_evidence,
     artifacts}' /tmp/function-create.json

jq . /tmp/function-low-side-driver/.kicadai/transaction.json
jq . /tmp/function-low-side-driver/.kicadai/manifest.json
```

To require installed KiCad ERC, strict DRC, connectivity, route completion,
writer correctness, and zero normalized round-trip differences, use a fresh
output directory and add the external gates:

```sh
kicadai \
  --symbols-root "$KICADAI_SYMBOLS_ROOT" \
  --footprints-root "$KICADAI_FOOTPRINTS_ROOT" \
  --kicad-cli "$KICADAI_KICAD_CLI" \
  --require-erc \
  --require-drc \
  --require-kicad-roundtrip \
  --strict-diffs \
  --request examples/circuit-graph/function_low_side_status_driver.json \
  --output /tmp/function-low-side-driver-kicad \
  --overwrite \
  circuit create > /tmp/function-create-kicad.json

jq '{ok, stages: [.data.workflow.stages[] | {name, status, issues}]}' \
  /tmp/function-create-kicad.json
```

`proven_readiness` reports the checked-in fixture boundary, not a guarantee for
every selected part, board constraint, or environment. ERC/DRC does not prove
analog performance, thermal margin, sourcing, regulatory compliance, or
fabrication release. Treat any blocking diagnostic as a stop condition.
