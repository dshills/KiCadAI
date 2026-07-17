# Frozen tolerance corpus matrix

Each category has a catalog-evidence-backed near-boundary case that passes
every selected corner and a nominal-only case that passes nominal evaluation
but is blocked by worst-case proof. Fixture identity is never used by the
evaluator; these are only frozen input/report expectations.

| Category | Passing proof | Nominal-only blocked proof | Dominant evidence |
| --- | --- | --- | --- |
| Regulator output | output accuracy and headroom remain inside assertion | output accuracy corner leaves output band | output-voltage model parameter |
| Divider | ratio remains inside band | opposing resistor endpoints leave band | upper/lower resistance |
| RC timing/filter | gain and cutoff remain inside band | R/C endpoints leave timing or gain band | resistance/capacitance |
| Transistor bias | collector/emitter bias remains inside band | beta/temperature corner leaves bias band | beta or junction temperature |
| Op-amp gain | output remains inside gain/headroom band | gain or supply corner clips/leaves band | open-loop gain/supply |
| Comparator threshold | threshold decision margin remains inside band | source/threshold corner reverses margin | source or threshold |
| Protection network | protected node/current remains inside limits | clamp/source corner exceeds protected limit | clamp/source evidence |

The embedded shared corpus manifest at
`internal/testsupport/tolerancecorpus/testdata/manifest.json` and its
generated report must record input hashes,
catalog hash, nominal and worst-case status, failure taxonomy, and dominant
corner contributors. Every `worst_case_pass` is promoted through the standard
KiCad ERC, strict DRC, connectivity, route-completion, writer-correctness,
round-trip, and replay gates.
