# Scope decision: repair the AI-facing contract before claiming board growth

Date: September 11, 2026. Decision: **stop before production implementation and
request a separately approved scope**. This is the baseline stopping rule in
the frozen SPEC and PLAN, not a successful capability milestone.

## The non-reserved evidence does not support two complete uplifts yet

P01–P06 all failed requirement compilation after their initial proposal and one
permitted correction. Their final compiler issue counts are respectively 134,
103, 128, 156, 136 and 140. These are diagnostic counts, not independent bugs.
Every one of these cases has zero accepted compiler-ready requirements, zero
completed downstream replays, and no generated circuit or native board.

The source-bound case audits retain all conditions and expose the exact first
failure. The feasibility review established that the briefs are electrically
coherent; it did not establish that the current engine can synthesize them.
Passing older supported reference fixtures is also not proof of these new
compositions.

The scope assessment uses P01–P06 plus the non-reserved behavioral cases and
W01. P07/P08 are public-frozen reserved tests: their outcomes are disclosed in
the results, but their individual details do not select any change below.
Paraphrases remain outside the eight-board denominator.

## Three recurring integration areas were considered, not implemented

### 1. Version-aligned, semantically constrained requirement emission

The provider schema and capability context pin `kicadai.open-set-requirement.v3`,
while the reflected schema exposes fields whose populated forms require newer
contract versions. P01–P06 each retain a version-related compiler failure.
Their proposals also contain incompatible domain kinds/sources, multiple
endpoint selectors in one binding, invalid units/relations and missing usable
signal endpoints. The JSON shape can be accepted by the provider while the
engineering contract remains invalid.

Source: `internal/behavioralintent/schema.go:18` reflects the shared proposal
type and pins v3; `internal/behavioralintent/provider_context.go:102` advertises
v3; `internal/architecturesearch/validate.go` enforces semantic and version
rules. The retained per-case `provider-schema.json`, proposals and compilations
are the execution evidence, rather than an inference from source alone.

A future change needs independent, non-corpus regressions for version/field
compatibility, domain/source vocabulary, exclusive endpoint bindings, numeric
bounds and canonical units. It must retain strict compiler rejection of
unsupported or ambiguous values. It must not silently reinterpret arbitrary
invalid proposals or weaken engineering checks.

### 2. Source coverage and clarification/refusal contract consistency

The provider emits plausible but unsupported coverage terms and reference kinds
such as `captured`, `incorporated`, `objective` and `domain`. The compiler expects
its own typed dispositions and references. Many retained material requirements
therefore have no valid source-coverage link.

N01 and N04 explain the expected safety/capability limitations but use prose in
fields requiring normalized capability identifiers. N03 asks to change the
protection/guarantee choice rather than producing the predetermined unsupported
high-energy refusal. C02 asks the required load questions without an executable
requirement, but its uncertainty ownership/resolution and coverage are invalid.
These distinctions are preserved; correct prose is not equated with a working
structured workflow.

Source: `internal/behavioralintent/compile.go:206` validates gap identities,
`:300` validates coverage references and `:425` matches reference types to
dispositions. `internal/behavioralintent/model.go:33` declares the actual
disposition vocabulary. The existing bounded correction sends at most eight
diagnostics (`internal/aiprovider/model.go:15`); that limit was not increased.

A future change needs independent regressions for complete source accounting,
exclusive ready/clarification/unsupported outcomes, exact ownership paths,
cryptographically bound clarification answers, and predetermined refusal
reasons. No supplied fact may be replaced by a guessed circuit solution.

### 3. Bounded provider-response acceptance and evidence accounting

W01's complete retained stream exceeded the production client's 2 MiB response
limit and was rejected before an accepted proposal or usage record was exposed.
C01's correction stream ended without a terminal response. Neither output is
repaired or promoted into an accepted requirement. The journal retains the
frozen conservative reservations; supplemental usage visible in a complete
rejected stream is reported separately, not used to rewrite history.

A future investigation should use independent synthetic streams to distinguish
transport completion, output JSON, parser limits, token usage and opaque
encrypted metadata. Any change to a frozen response/evaluation limit needs a
new recorded protocol decision. No such production or protocol change is made
by this report.

## Why these are not an approved three-fix board-growth plan

Naming three broad integration areas does not make them three bounded fixes
with demonstrated end-to-end effects. The baseline has not reached component
qualification, electrical synthesis, schematic readability, placement, routing,
native KiCad checks or deterministic replay for any primary case. The remaining
downstream gaps and resource costs are unmeasured.

More importantly, improving the schema or prompt changes future provider
proposals. It does not by itself make the **same retained baseline proposal**
pass the required paired control. Many retained proposals contain ambiguous
bindings, inconsistent source references and invalid contract fields. Manually
editing them, supplying a topology, or inventing a permissive normalization would
not establish the required two independent complete uplifts.

There is therefore no evidence-backed commitment to two full fresh-plus-paired
uplifts under the current maximum-three-change plan. The correct action is to
publish the baseline and ask for a new scope, not to spend the final campaign
budget on an unqualified candidate.

## Recommended next scope, requiring user approval

Establish a reliable AI-facing requirement contract as a separate integration
milestone before renewing the practical-board growth evaluation. Begin with
offline, independently authored synthetic inputs covering valid generation,
refusal, clarification, semantic vocabulary and provider-stream boundaries.
Demonstrate valid, faithful contracts and fail-closed handling without changing
historical or frozen evidence. Then propose a separately frozen live evaluation
and resource plan, with an explicit distinction between interface reliability
and complete-board capability growth.

The existing API key is not used for additional calls by this scope decision.
The unspent original budget is not authority for another recovery, additional
correction cycle, model change or relaxed acceptance. Further live work requires
an agreed scope/protocol; no additional dollar or request allocation is inferred.

The current branch changes only the experimental evaluator and its evidence/
documentation. Merged V23 production behavior and the stable support boundary
remain unchanged. There is no merge, release, fabrication authorization or
manufactured-hardware validation. Final/paired growth metrics remain unavailable
because production implementation and those campaigns have not run.
