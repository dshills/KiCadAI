# Closed-Loop Open-Set Untouched Baseline Audit

Status: complete and selection sealed

Date: 2026-08-09

Corpus freeze commit: `8ce268fb5acb2cb64d3fd26888f2abec38f150e9`

## Execution

The version-2 corpus was executed locally through the normal
`opentopologysynthesis.Synthesize` entry point with the exact policy stored in
the frozen manifest. Each of the 24 requirements was strictly decoded and
synthesized twice. All 48 normalized synthesis runs were byte-identical in
their case pair. Completed case evidence was written and hash-validated before
the next case.

The run completed in 273.46 seconds on the frozen Darwin/arm64 reference
environment. No GitHub Actions workflow was started or monitored.

No case reached a fully passing, physically ready synthesis result. Therefore
the physical-promotion path and installed KiCad were not invoked, and the
baseline makes no ERC, DRC, connectivity, route-completion, writer,
round-trip, or project-replay pass claim. Those gates remain mandatory for any
newly passing case after capability expansion.

## Outcomes

- Discovery: 0 pass, 1 unsupported, 0 unsafe, 11 exhausted.
- Held-out: 0 pass, 2 unsupported, 0 unsafe, 10 exhausted.
- All cases have one authoritative terminal outcome and at least one
  content-addressed causal gap.
- Held-out evaluation contains no rankable clusters and did not participate in
  discovery selection.

Detailed per-domain counts and all per-case evidence appear in
`BASELINE_REPORT.json` and the content-addressed testdata directory.

## Selection Seal

Discovery rank 1 is:

`exhausted:topology_repair:topology:causal_topology_repair:OPEN_TOPOLOGY_REPAIR_EXHAUSTED`

It affects two independently authored discovery cases across analog and power,
covering seven analysis kinds. The existing capability-expansion planner
accepted it as an architecture need and produced expansion plan hash
`2efca89055a231923de3d109140cb28979a49b259803dda4978594ee1b46cec7`.

The selection was computed after discovery completed and before held-out
synthesis began. `SELECTION.json` binds the full ranking tuple, affected cases,
required evidence, policy hashes, manifest hash, baseline report hash, freeze
commit, expansion plan hash, and its own content hash.

## Reproduction

Fast artifact reproduction:

```sh
go test ./internal/capabilityfeedback -count=2
```

Full local synthesis reproduction, including installed-KiCad promotion for any
case that reaches pass:

```sh
UPDATE_CLOSED_LOOP_BASELINE=1 go test ./internal/capabilityfeedback \
  -run '^TestUpdateClosedLoopBaseline$' -count=1 -v -timeout 3h
```

The fast test strictly decodes all checked-in evidence, recomputes discovery
and held-out reports, rebuilds the rank-one expansion plan and selection, and
requires exact report bytes and checksums.
