# Closed-Loop Open-Set Capability Expansion V11 Plan

1. Commit the authenticated V10 terminal retirement and freeze V11 corpus-reuse
   and memory-bounded evaluation rules.
2. Implement the generic streaming replay store and prove byte-equivalence,
   no-replace behavior, bounded decoding, one-live-replay sequencing, resume,
   and fail-closed behavior with synthetic tests.
3. Review the complete freeze with Prism, remediate valid findings, commit, and
   push. Do not manually trigger GitHub Actions.
4. From a clean committed environment, evaluate all 24 discovery cases exactly
   twice and publish either the complete public baseline or terminal retirement.
5. If the baseline passes, cluster typed gaps, rank generic capabilities, and
   select only the highest-impact capability permitted by the frozen protocol.
6. Implement the selected generic capability, run local and installed-KiCad
   regressions, and publish the complete public successor evidence.
7. Only after public acceptance, run the authorized isolated blind held-out
   evaluation, publish encrypted and aggregate artifacts, and verify uplift and
   preservation without disclosing held-out content.
