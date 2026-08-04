# Untouched-engine diagnosis

The frozen baseline was measured at commit `75d0f39b06e7bb3f91e61ae3e90f58ddf1568e7e` with the production synthesis policy and deterministic replay. All three behavior-only requirements reach simulation, none passes, and none reaches physical realization. Search produces only two complete graphs for each requirement, confirming that the existing regulated-voltage relationship seed is narrow rather than an open-ended protected-output solution.

## Low-noise adjustable output

Both retained candidates receive ready value plans, but their fixed feedback relationships cannot reproduce the two requested command/output pairs. The baseline records both over- and under-voltage at the high setting, over-voltage at the low setting, and excessive line movement. The missing capabilities are command-derived reference scaling, feedback ratios derived across operating cases, dropout checks at each requested setting, and coordinated noise/stability compensation.

## Protected high-power output

The existing candidate reaches simulation but fails line regulation, quiescent current, and phase margin, with many invalid line-sweep evaluations. It does not derive a high-current protected pass stage from load, short-circuit, thermal, and SOA requirements. The missing capabilities are staged drive, enabled/default-off behavior, current sensing and limiting, compensation under wide capacitive load, and electrothermal/SOA sizing at rated load and sustained short.

## Bidirectional virtual rail

The existing positive-only candidates cannot regulate through signed load current. Every evaluated family exceeds the line-regulation limit, and most signed load sweeps do not converge. The missing capabilities are a symmetric source/sink output stage, bidirectional current limiting, stable feedback under capacitive and signed loading, and thermal/SOA checks for both current directions.

## Required remediation boundary

Implementation must expand generic relationship generation and derivation from requirement semantics. It must not recognize these fixture names, embed their values, or introduce a predefined regulator block selector. Failure diagnostics must remain evidence-driven and deterministic, and physical promotion remains inaccessible until all required simulation corners pass.
