# Closed-Loop Open-Set Capability Expansion V1 Validation Audit

Status: blind validation failed; milestone completion is not claimed

Date: 2026-08-09

## Frozen experiment

The experiment used the committed 24-case behavior-only corpus, the untouched
baseline, and the sealed rank-one selection without changing their bytes,
policies, catalogs, work budgets, or acceptance gates.

- corpus manifest SHA-256:
  `c86d9bef8c8cc97bde4389109ffa4e6140c31d61394295d02fdec37e9cd5117a`
- baseline report SHA-256:
  `4e7bdd4c1eb330af14ac328f18c3fb7256a295bb0e4a5af4b6970da2fa9d4e05`
- selected cluster:
  `exhausted:topology_repair:topology:causal_topology_repair:OPEN_TOPOLOGY_REPAIR_EXHAUSTED`
- synthesis-policy SHA-256:
  `4b067326445c90ac125ee5bf61ab7d57d96118806a83e02e7675ea2905038df4`
- frozen work ceilings: 128 candidate simulations, 2,048 corner evaluations,
  128 value trials, and 16 topology repairs per case

The implementation did not use held-out requirement bytes, identities,
outcomes, cluster membership, or diagnostics for selection or tuning before
the final blind run.

## Blind result

| Role | Baseline pass count | Blind final pass count | Required result | Verdict |
| --- | ---: | ---: | --- | --- |
| Discovery | 0 | 2 | strict increase | pass |
| Held-out | 0 | 0 | strict increase | fail |

Discovery cases `case_001` and `case_003`, the two cases attributed to the
selected rank-one cluster, synthesized deterministically twice and passed
their clean-root installed-KiCad promotion evidence. Every held-out case
remained `unsupported` or `exhausted`; no held-out physical promotion was
reached.

The final verifier failed closed with:

```text
held-out pass count did not improve: before=0 after=0
```

`FINAL_REPORT.json`, `FINAL_COMPARISON.json`, `PROMOTION_MATRIX.json`, and the
final per-case evidence directory were not written. Their absence is expected:
the artifact writer runs only after the complete improvement contract passes.

## Safety and preservation evidence

- Every corpus synthesis was executed twice and produced identical evidence.
- The newly passing discovery designs completed their required local
  installed-KiCad promotion checks before being counted as passes.
- `usb_c_led_indicator_protected` passed the optional installed-KiCad tier.
- `usb_c_i2c_sensor_3v3_protected` passed the optional installed-KiCad tier.
- Focused topology, value-domain, simulation-budget, simmodel, repair, and
  capability-feedback tests passed.
- A broad open-topology package run reached Go's default ten-minute timeout in
  the pre-existing
  `TestWindowedHeatingPowerPassesElectricalAndSafetyCorners` test after that
  test had run for 6 minutes 17 seconds. The simmodel and repair packages
  passed in the same run. This timeout is incomplete evidence, not a pass and
  not an attributed regression.

## Interpretation

V1 proves discovery uplift and preservation, but it does not prove that the
selected generic capability generalizes to an untouched requirement. The
milestone therefore remains incomplete under section 11 of `SPEC.md`.

The held-out set has now served its one allowed validation use. Its aggregate
result is known and it may not be tuned against, reused as blind evidence, or
silently relabeled to make V1 pass. Retrying V1 after inspecting or responding
to its held-out results would invalidate the experiment even if the same files
later passed.

## Required continuation

The valid continuation is a new versioned experiment under
`V2_CONTINUATION_PROTOCOL.md`. It requires a fresh independently authored and
sealed validation corpus before any additional outcome-changing production
work. V1 may remain as historical discovery evidence, but it can no longer be
the held-out proof for a completion claim.
