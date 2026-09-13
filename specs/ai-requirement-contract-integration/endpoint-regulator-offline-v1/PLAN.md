# Approved offline endpoint and regulator phase

Approval: September 11, 2026, user reply "approved" to implementing faithful
participant-pin measurements and standalone regulator outputs, with regression
tests, preservation of historical evidence, and no API calls.
Starting revision: `7f2752c0462370909273a161c00195c50240a2cb`.
Branch: `codex/endpoint-regulator-contracts`.

## Contract and implementation plan

1. Expose exact participant-port observations using the existing downstream
   `participant_port` semantic binding identity, `participant_id.port_id`.
   Validate both identifiers and existence; never silently substitute a public
   connector, sibling pin, domain or whole circuit. Trace the selected pin
   through behavioral projection, lowering, reference resolution and simulation.
   Reject bundled bus/differential interfaces until an explicit lane identity
   is supported; never measure whichever lane happens to sort first.
   Both new forms are limited to one reference domain until explicit participant
   and generated-output reference routing is supported. Do not infer a physical
   ground connection from a matching domain name.
2. Represent a generated supply at a real external power output with explicit
   domain provenance `port:<output_id>` in behavioral v3+ requirements. Keep
   existing signal-sourced domains and v1/v2 semantics unchanged. Require a
   same-domain power source port and exactly one actual selected producer;
   require the producer's explicit `output` role and distinguish real internal
   power/sense consumers from that producer;
   preserve cycle, source, load, voltage, current and downstream checks.
   Do not create a dummy consumer or fictitious controller to complete a signal.
3. Add failing-then-passing offline tests: pin identity/sibling isolation,
   unresolved or malformed pins, scoped constraints, concrete simulated target
   and reference, generated output/domain connectivity, absent/duplicate/wrong
   producers and cycles. Include end-to-end offline integration where supported,
   and report any later engineering failure instead of weakening a gate.
4. Update the provider schema and contract guidance for these explicit forms,
   with strict local schema tests and bounded request captures only. No provider
   model/configuration change, schema-probing request or live evaluation.
5. Run affected uncached tests, race checks, vet/lint and repository regressions;
   retain a source-bound verification report and reauthenticate old evidence
   from its pinned source. Local Go tests use the cached 1.26.8 toolchain,
   workspace-local caches, offline module settings, GOMAXPROCS=4 and provider
   keys/live switches unset. Start with the existing 12-minute package timeout;
   record timeouts as limitations rather than repeatedly increasing it.

## Preservation and claim boundary

No historical corpus, raw run, seal, publication, archive, evaluator binary,
acceptance denominator, retry budget or result is edited. New examples are
offline implementation regressions, not held-out model performance evidence.
No new provider calls, Gemini review, push, PR, merge, release or fabrication.
The six-complete-board/two-new-design milestone stays unmet until its complete
electrical, physical, KiCad and reproducibility evidence exists.

The official Structured Outputs guide requires closed object shapes and all
fields to be required (nullable where needed), and permits nested unions. Keep
schema shape separate from cross-reference and engineering validation:
https://developers.openai.com/api/docs/guides/structured-outputs
