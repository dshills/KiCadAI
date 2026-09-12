# Implementation and evidence review

Assessment: share with caveats for this offline implementation phase only.
This is implementation-agent self-review, not independent human, subagent,
Gemini, or PR review. Final command outcomes are authoritative in
[verification.json](verification.json); no incomplete check is a pass.

## Question and methodology

Do exact participant-pin observations and real standalone regulated outputs
survive compilation, architecture selection, concrete pin/net lowering, and
trusted simulation without substituting another endpoint or inventing a
consumer? Review uses source inspection, failing-then-passing regressions,
three synthetic integration fixtures, whole-repository short tests, bounded
request captures, and source-bound historical authentication. It does not
measure how often an AI model will generate a faithful complete design.

The unit of evidence is a specific contract/test or named integration fixture.
There is no new evaluation population, exclusion policy, performance percentage,
or causal uplift estimate. The 100 mA fixture is not independent of the 150 mA
fixture. Measurements are model-derived engineering predictions, not lab data.

## Findings addressed

1. Exact pin identity was not publicly expressible. Qualified scalar IDs now
   resolve through projection and hierarchy; missing/local-only/sibling IDs do
   not silently stand in for the requested pin. Integration proves the selected
   ADC function's physical symbol pin and pad are on the measured net.
2. A standalone output required an internal signal with a sink. Explicit
   `port:<id>` provenance now permits a real public output and one producer,
   without adding an unrequested consumer. External-source substitution, wrong
   kind/domain, absent/duplicate producers, self-cycles and cross-rail cycles
   are rejected. The retained I04 external-source counterexample is unchanged.
3. Public output direction wrongly made internal power consumers appear to be
   producers. Direction is now interpreted by the new generated-port role
   contract, consistently in binding, search and behavioral dataflow. Consumer
   contracts retain the current-demand limit and clear the source-capacity
   requirement; unknown roles fail closed. Legacy public-port behavior is
   retained outside the new source form.
4. Bundled participant bindings can use a representative first lane in existing
   lowering, and multi-reference participant grounding can default to a reference.
   The new observation form rejects both unsupported cases. The new generated
   output source also rejects multiple references. This avoids claiming that a
   matching name proves the intended physical reference route.
5. Executing the 150 mA controller example exposed a real thermal rejection.
   The test requires that rejection. A separate 100 mA design variation executes
   and passes its own unchanged measurement bounds; no production temperature
   limit, formula, model, or historical case was weakened to obtain it.

## Calculation and preservation checks

- The three fixture rows trace to named subtests in `focused-tests.log`.
  Two produce complete requested simulation measurements; one is an expected
  thermal rejection. Neither category is a complete board-promotion result.
- 131,072 − 126,382 = 4,690 bytes of sampled wire-form headroom. All 16 captured
  forms use a local transport stub; arbitrary future follow-up state is unproven.
- Historical interface/practical case and clause counts are checked against
  retained authenticators and original sources, not combined into a new success
  rate. Historical evidence and prior repair artifacts are preserved in place.
- Source and test file hashes bind the retained command receipts to this phase.
  Integrity checks alone do not prove that a test was executed; the logs and
  reproduction command provide the separate execution trail.

No chart is needed: the small table in README preserves named design variants,
units, and the negative case. Displayed values are rounded; machine-readable
results retain the observed floating-point values.

## Remaining caveats and handoff boundaries

- Multi-reference and explicit bus-lane measurement contracts remain unsupported.
- The new integration fixtures cover scalar ADC conditioning and a regulator;
  they do not qualify every participant capability or every behavioral metric.
- Full schematic readability, placement, routing, native KiCad ERC/DRC, generated
  project replay, and fabrication readiness are not demonstrated for these new
  fixtures. Existing repository regressions are not substitutes for that work.
- No new live provider schema probe or campaign occurred. Model reliability,
  full-board success, and paired improvement are not measured by these tests.
- The historical sealed replay failure and historical failed goal remain failed.
- External review, PR publication, and subsequent live evaluation are outside
  this approval. Credential-safety and evidence-validation skills kept tests
  key-free and claims narrower than the overall milestone.
