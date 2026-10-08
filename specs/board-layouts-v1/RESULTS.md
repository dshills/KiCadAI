# Bounded board-layout delivery

2026-10-08. Both existing board families now generate new board outlines and
connector placement/routing from structured specifications. Two sizes and three
independent binary connector choices yield 16 mechanical combinations per family;
the five existing electrical profiles yield 80 configurations.

Controller, sensor and sensitive support groups retain reviewed placement and
local copper. Connector placement derives from board-boundary constraints, and
the router constructs new connections around pads and existing copper. This is
a bounded mechanical extension of existing circuits. No manual artifact repair,
paid model request, firewall change, hardware order or physical test occurred.

## Verified result

- All 80 configurations passed all 14 installed-KiCad gates using KiCad 10.0.3:
  electrical admission, generated-source/BOM identity, connectivity, schematic
  validation, native parse/render, version, ERC, strict DRC/parity, both round trips,
  previews, manufacturing export verification and delivered-input immutability.
- Every DRC report has zero violations, zero unconnected items and zero parity
  differences, including warning-level findings. No exclusions were added.
- All 32 family/mechanical combinations reproduced byte-identical native PCB,
  schematic, project, BOM and layout records on two generations and passed
  connectivity checks.
- Full CLI bundles for BMP280 120x70 edge connectors and SHT31 140x80 edge
  connectors were generated twice; both exact file inventories/hashes passed,
  and their native/design records reproduced. Each bundle includes Gerbers,
  drills, placement CSV, local libraries, previews and confirmation receipt.
- All five legacy configurations passed the native confirmed-specification suite
  and retained the reviewed example native hashes. Layout omission preserves the
  historical 120x80 reference and review semantics.
- Layout race tests passed; scoped golangci-lint reported zero issues. Closed
  choices, duplicate/unknown-field rejection, changed confirmation fingerprints
  and injected routing/physical-validation failures have negative controls.

The 80-case matrix took 135.925 seconds with four workers on this host. This
includes concurrent native CLI work and is not a performance guarantee or a
comparison against a different generation strategy. The repeat-built CLI bundle
tests took 16.98 seconds for four complete builds after the matrix finished.

## Retained evidence and bundles

[evidence.json](evidence.json) binds seven implementation source hashes, all 80
case identities, native hashes, complete per-case inventory digests, validation
report hashes and manufacturing-manifest hashes. Its SHA-256 is
`69dd2bd1cd81921b5644d7490bb52eaaf28f070c8b44537a352cf59b1b62ec26`.
Reproduce the audit with `node scripts/board-layouts/audit.mjs MATRIX_ROOT NEW_REPORT`;
the destination must be new. Native raw evidence is retained under
`.cache/qualification-final-01` in the isolated `layouts-01` worktree.

The local `.cache/board-layouts-reviewed-bundles.zip` contains the first complete
CLI bundle for each family, under `bundles-final-01`'s family directories. ZIP CRC
testing passed; ZIP SHA-256:
`7f011ded831e4d3bda3d7f127b79e0952f62ef57f58cfb51242f8ac0dc9b2711`.
The BMP280 and SHT31 bundle-manifest hashes are respectively
`021dff1003b149e07f4d4522a2c34810688ad73ee2187ab6f1e90eaedeeea386`
and `cdcd26f85a1ac23d140a77c1b99e8d96132dee9b939f3ca378592053d0d466fc`.
The archives and raw matrix evidence are retained locally; the source and compact
audit are the reviewable repository deliverables. File hashes establish integrity,
not an independent engineering signature or physical performance.

Both generated PCB previews were inspected. Edge connector positions, unchanged
core groups and new routes agree with the requested layouts. Existing sparse
silkscreen and crowded reference labels remain limitations.

## Source corrections and limits

Development checks exposed KiCad coordinate spelling differences, redundant
escape tracks and single-layer anchor vias/stubs. These were corrected in the
source generator before final qualification. Pad rotation is converted from
native board-relative angle to router-local angle, and the custom sensor copper
polygon uses conservative bounds. Initial diagnostics and interrupted matrices
remain in separate cache directories and are not counted as final qualification.

Routing has finite search/time limits and cannot fall back to another geometry.
Failure retains diagnostics and withholds manufacturing exports or the successful
bundle manifest. The declared total bus capacitance and other electrical limits
remain mandatory; no capacitance extraction, power-integrity measurement or
assembled-sensor accuracy claim is made. The original SHT31 prototype package's
physical preparation applies only to its original board hash.

See [usage and limits](../../docs/board-layouts.md). This change depends on PR #14's
confirmed-specification implementation; the prototype work in PR #15 is separate.
