# Connection representation: offline prototype

Status: **expressiveness improvement tested; language reliability unproven**.
No provider, CLI, journal, or live-evaluation entry point is enabled for this
prototype. The existing model, production defaults, owned-v4 protocol, failed
live batch, CAD generation and engineering qualification remain unchanged.

## What the failure exposed

Owned-v4 gives the model a `wireless_operation` feature but no corresponding
supported wired-connection fact. A required `other` fact faithfully describing
a wired connection receives an unsupported refusal. This is independently
reproduced in the new historical regression test. It is a representation gap;
it does not prove why the live model chose the wrong wireless fact.

The separate `5-connection-evidence-offline` schema provides `connection` facts
with `wired` and `wireless` values and all four requirement states. Radio has one
canonical representation in this schema; the legacy feature form is rejected.
Wired means only the reviewed non-radio connection mode. It does not promise an
arbitrary connector, Ethernet, extra GPIO loading, delivered firmware, or any
other unstated capability. Those requirements must remain separate facts.

The pure decoder validates every connection reference before lowering to an
ephemeral admission program. Required wired is supported by either existing
family, required wireless and forbidden wired are unsupported, exclusions do
not become prohibitions, and uncertainty produces a connection-mode question.
A wired fact cannot cancel radio or consume a numeric occurrence. Unknown
connector requirements, conflicts and omitted quantities remain blocking.
Raw caller bytes remain untouched. Historical v4 bytes are rejected by v5, and
v5 bytes are rejected by the original v4 decoder.

## Verification

Using the existing cached Go 1.26.8 toolchain with downloads disabled and all
provider keys removed:

- Complete `internal/boardfamily` short tests passed: 1,115 passing named test
  events, including subtests, with no failures or skips.
- Connection-focused race tests passed: 37 passing named test events, including
  subtests. These include 15 ideal-fact wording combinations, 10 state/constraint
  cases and all five existing family/profile configurations.
- Bounded fuzzing passed: 168,827 executions, no panic or caller-byte mutation.
  The fuzz phase was requested for 3 seconds and completed in approximately 4
  seconds; the whole command including build/startup took 19.231 seconds.
- All five successor decisions equal their unchanged baseline decisions when
  given correct synthetic connection facts. This permits reuse of existing
  engineering qualification; it is not a new native-output or live-model result.

Actual process records and stdout/stderr are retained in
`.cache/connection-06-checks-01`. The first attempt with the host's different Go
installation failed at dependency setup before tests; it is not counted as a
passing run. No dependencies were downloaded and no API requests were made.

## Remaining release blocker

`TestConnectionStillRequiresSemanticEvaluation` deliberately supplies the
**wrong synthetic wireless interpretation of a wired request**. That input still
passes the schema and causes a false refusal. The test preserves this known
limitation instead of claiming that the representation change solves model
understanding. References establish source location, not semantic entailment.
This follows the distinction in the [OpenAI Structured Outputs guidance](https://developers.openai.com/api/docs/guides/structured-outputs)
between schema compliance and correct content.

There is no measured live accuracy or latency improvement, no independent
holdout result, no replacement of the 14-case frozen evaluation, and no new
spending authority. Broader semantic validation and a reviewed, separately
versioned integration are still required before proposing another live batch.
The full two-family natural-language-to-board goal remains unachieved.
