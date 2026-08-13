# V9 Generation-Zero Baseline Evidence Envelope

Status: corpus-independent implementation prepared and tested. No real V9 case
has been synthesized and no held-out key or record has been opened.

The V9 baseline validator accepts exactly 24 public discovery records in
canonical manifest order. Every record is bound to one requirement hash, one
frozen evaluator manifest, one frozen environment, and exactly two identical
full-run replay hashes. Environment or evaluator drift between cases fails the
entire baseline.

Every case has exactly one terminal classification: `pass`, `unsupported`,
`unsafe`, or `exhausted`. Unknown, skipped, partial, conflicting, or timeout
classifications are not representable by the accepted schema.

## Pass evidence

A pass requires all fourteen result gates:

1. primitive-only construction;
2. topology search;
3. simulation;
4. all-corner evaluation;
5. model provenance;
6. closed-loop evidence;
7. complete routing;
8. connectivity;
9. writer correctness;
10. zero round-trip diff;
11. clean ERC;
12. strict DRC;
13. deterministic replay; and
14. fail-closed behavior.

It also requires exactly two installed-KiCad promotions from distinct clean
roots. Both promotions must report replay identity and must have identical run
and project hashes. Passing cases have no active frontier.

## Nonpass evidence

Every nonpass still requires byte-identical deterministic replay and fail-closed
behavior. It must have no promotion claim, at least one failed result gate, and
a nonempty generation-zero root frontier. Every root has exactly one typed leaf
in `topology`, `component`, `model`, `simulation`, `physical_design`, or
`verification`, with canonical evidence, diagnostics, obligation identity, and
path ordering. A satisfied obligation cannot simultaneously retain a gap.

## Aggregate evidence

The validator publishes all four outcome counts, including zero counts, and a
canonical hash for every case and the complete report. Report verification
rebuilds case hashes, outcome counts, common environment/evaluator bindings,
and the aggregate hash rather than trusting serialized summaries.

This envelope validates evidence produced by the later frozen evaluator. It
does not synthesize circuits, classify raw tool output, load corpus source,
publish artifacts, or access keys.
