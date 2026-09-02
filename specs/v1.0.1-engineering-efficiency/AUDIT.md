# v1.0.1 Engineering-Efficiency Audit

## Scope preservation

The change is confined to test orchestration, proof authentication, workflow
action pins, contract tests, and documentation. The production Go packages,
schemas, circuit catalogs, routing, writers, and v1 support matrix are
unchanged. Existing promotion scenarios still execute run 1 before run 2 and
publish in their frozen order.

## Baseline

The v1.0.0 pull-request `Offline quality gates` job completed in 46 minutes 37
seconds. Its raw coverage was 76.4 percent and generated-excluded coverage was
80.4 percent. Parsed package timing identified:

| Package | Instrumented time |
|---|---:|
| `internal/opentopologysynthesis` | 2,048.088 s |
| `internal/compositionlowering` | 208.708 s |
| `internal/schematicir` | 75.240 s |
| `internal/capabilityfeedback` | 53.253 s |
| `internal/designworkflow` | 38.805 s |
| `internal/circuitgraph` | 26.189 s |

Local top-level profiling split the dominant package by measured proof cost;
the six largest tests ranged from 27.86 to 72.59 seconds on the reference Apple
M4 Pro host.

## Cold local proof

The first complete portable ten-shard run authenticated and selected exactly
6,296 top-level tests across all 144 packages. All shards passed. The slowest
shard was 137 seconds; the three bounded batches had a 363-second test critical
path, followed by approximately 79 seconds of complete inventory regeneration,
authentication, and merge work. This is approximately 7 minutes 22 seconds,
an 84 percent wall-time reduction from the 46-minute-37-second GitHub baseline.

The ten macOS shard resource reports totaled 3,545.34 user CPU seconds and
102.39 system CPU seconds. Sandboxed macOS did not expose reliable peak RSS;
Linux CI's `/usr/bin/time -v` reports that value in the same profile schema.
The merged profile retained the 80.4 percent generated-excluded result and the
75 percent release floor.

## Cache behavior

An exact authenticated open-topology shard replay completed in 1.125 seconds
after a 77-second cold execution. Editing the merge or reporting protocol
changed the source fingerprint and correctly rejected all prior artifacts.
Cache entries are therefore useful only for exact proofs and cannot mask a
changed repository, toolchain, environment, selection, or shard plan.

## Workflow hardening

- Four ordinary-package and six open-topology coverage jobs have 15-minute
  individual limits.
- The required merge job verifies exact inventories and every artifact digest
  before enforcing the unchanged coverage floor.
- `checkout`, `setup-go`, `cache`, `upload-artifact`, `download-artifact`, and
  `golangci-lint-action` are pinned to verified Node.js 24-compatible commits.
- Promotion and external-review scenario definitions and two-run ordering are
  unchanged.

## Required closeout evidence

The staged source must still pass formatting, lint, broad bounded and race
gates, historical seals/replay, release reproducibility, Prism, and the full
13-case installed-KiCad tier. The natural pull-request run supplies the Linux
peak-RSS and sub-15-minute GitHub measurement; Actions must not be manually
triggered.

## Next milestone

Once these preservation gates pass, the next capability milestone should add
generic analysis/model/solver availability selected from the highest-impact
V19 failure cluster. No V19 topology-specific shortcut or v1 support expansion
is authorized by this audit.
