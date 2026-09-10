# V23 bounded repair and physical handoff

Base: `aa3722fff1a599ab416c614cd9aa14a77d35fc01`.
The separately frozen solver and admission/certificate layers are now connected
to an opt-in repair/synthesis path. Historical production and evidence remain
unchanged. The frozen public successor evaluation has not run.

## Preserved search and acceptance

`RepairElectricalV23` uses the existing V22 control-edit enumerator, structural
proof, deterministic ranking/deduplication, critical-failure guards, and
previously-passing-corner preservation. Default limits remain depth 4, beam 8,
4096 binding work, 128 numerical invocations, and 4096 corners, further limited
by the caller's existing simulation policy. There is no new graph operator,
candidate template, component model, value tuning, or larger search budget.

Each candidate uses the explicit admitted V23 evaluator. The compact trial
ledger retains the old graph/evaluation identities and adds the versioned
execution hash and charged simulation/corner counts. The selected result retains
the full execution envelope and V23 certificate; rejected trial waveforms are
not duplicated. Admission refusals still charge an invocation.

Selection verification requires the exact solver policy, bounded ledger,
contiguous trial numbers, valid identities, matching aggregate accounting, and
a final trial matching the selected graph, execution, and certified control
path. It then verifies the complete electrical/solver/corner certificate.

`ContinueElectricalV23` authenticates the exact complete V21 source and retains
the original result when it is ineligible or repair fails. Historical passes,
critical failures, cancellation, incomplete topology, and tampered provenance
cannot become new repair work. The source's mutable containers are copied before
an accepted result is appended; retained historical waveforms remain read-only.

Physical lowering is reached only after `VerifyElectricalRepairSelectionV23`
passes. The enclosing V23 synthesis envelope binds its solver-policy identity,
predecessor, repair, and final synthesis. Lowering still does not constitute
installed-KiCad promotion; the unchanged native promotion gate remains mandatory.

## Independent regression evidence

The hand-authored saturation test now also requires a 6 V midpoint output under
12 V supply. This excludes an open-loop rail transition that merely shares the
same endpoint slope. V22 exhausts; V23 certifies a single feedback edit in two
trials, six charged simulation calls, and 41 corner evaluations. Permuted graph
inputs replay identically. This is not a corpus requirement or source graph.

The independent dual-monitor regression uses only historical numerical paths.
V23 preserves its exact V22 trial ledger, work counts, selected graph, and full
numerical result while adding explicit execution provenance. Negative tests
cover forged policy/execution identities, missing or reordered ledger evidence,
negative or inconsistent counts, invalid depth, missing certificate paths,
missing solver evidence, critical guards, cancellation, initial evidence, and
ineligible/tampered synthesis predecessors.

Installed-KiCad 10.0.3 validation ran successfully for both independent circuits,
two clean project roots each, in 23.747 s total package time. Promotion requires
ERC, strict DRC, routing, connectivity, writer correctness, zero round-trip
differences, and project replay equality.

| Independent circuit | Project SHA-256 | Promotion SHA-256 |
| --- | --- | --- |
| Saturation-recovery monitor | `156dfc12bdaeadba01cc462ae5e74e97d69947441c7cfb49d2df1dd372b8a7aa` | `2e18fa148a07a42a84861f18218e4d2526ecf7e0bea4123ba734442d1241ef52` |
| Dual monitor | `207ae84c8d11a306e0c8b81355e0e81201bf843f919181500aafcc06f72a881a` | `1a09bef38870545a846705720fe32fc80b7b5c51a8e92c34b4ffef3dffdb3fc7` |

Artifacts remain outside Git at `/tmp/kicadai-v23-independent-promotion-1`;
the test transcript is `/tmp/kicadai-v23-independent-promotion-1.log`.
No timing comparison or corpus capability increase is inferred from these tests.
The full 24-public-case/two-replay evaluation and remaining milestone gates
must still complete under a separate committed source/protocol freeze.
