# Approved offline repair pass

Approval: September 11, 2026, user reply "approved" to an offline repair and
regression-test pass with no API calls and preservation of historical evidence.
Starting revision: `49953eaa599612aaf8e60f31ab3e786f32ddc722`.
Branch: `codex/ai-requirement-offline-repair`.

## Scope and invariants

Repair only evidenced reusable contract/validation defects; add synthetic and
recorded-counterexample regressions. No live provider calls, model/configuration
changes, new campaign, Gemini review, merge, release, fabrication or stable
support expansion. Do not rewrite the frozen corpus, raw evidence, publications,
archives or sealed binaries. Existing failures remain historical failures.

A pinned checkout at `.cache/interface-v1-source-49953` preserves the old source
for its source-bound authenticator. The new revision intentionally changes
production files that were inputs to that old run; it must not be passed off as
the evaluated revision. Tests run with all provider keys and live switches unset.

## Work items

1. Reproduce the empty-issues replay failure and preserve omitted fields while
   keeping all typed values, duplicate diagnostics and non-diagnostic order.
2. Reject a voltage-regulation output whose supply domain is declared external
   or is sourced by a different objective; preserve coherent existing derived
   rail contracts. Do not invent physical topology or automatically fix output.
3. Make the provider contract explicit about legal source-coverage identities,
   unique constraint names, operating targets and measurable observation IDs.
   Preserve existing rejection of unknown project/constraint/local participant
   IDs and objective names used as operating targets.
4. Constrain whole-circuit observation identity in the emitted schema. Do not
   introduce new participant-observation semantics or claim whole-circuit
   measurements prove a particular participant endpoint. Missing faithful v3
   representation must remain an explicit gap, not a guessed substitute.
5. Run focused adversarial tests, affected downstream tests, race checks, local
   review and preservation checks. Record results and residual limitations
   separately; no new live success rate or complete-board pass is inferred.

## Diagnostic clarification

Compiler paths refer to the normalized requirement: operating conditions and
constraints are sorted before validation. In C02, the unresolved target at
`conditions[0]` is the ambient-temperature target `voltage_regulation` (an
objective), not the declared `input_supply` domain at index zero in the original
proposal. New diagnostics should name the offending identity to avoid reading
normalized indexes against unsorted input. The historical rejected outcome and
its raw evidence remain unchanged.

## Documentation basis

The official [Structured Outputs guide](https://developers.openai.com/api/docs/guides/structured-outputs)
states that structured output can still contain mistakes. Schema shape checks
therefore supplement, and do not replace, deterministic engineering validation.
The credential-safety skill keeps verification processes credential-free.
