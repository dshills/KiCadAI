# Pre-execution runner review

Reviewed by the implementing agent. This is a source/guard review, **not a live
semantic review, external Gemini review or independent engineering certification**.

The successor has its own evaluation identity, policy, batch directory, ledger,
freeze and runtime record. It imports unchanged native/evidence comparison and
durable-child helpers, but does not mutate or dispatch the old final-01 runner.
Only original prompt text reaches the selector; gold fact/configuration checks
stay in the evaluator process. The selector itself is unchanged from the
source-bound 8913e2b6 qualification.

The pre-freeze guard suite passes 14 tests. They exercise exact case selection,
all required fact omissions, wrong values/polarity, source/schema integrity,
separate raw/application scoring, approval budget and runtime bindings, ledger
mutation/exhaustion/duplicate response identities, observed-terminal-only resume,
all-attempt timing, source-bound meaning review and durable failed child identity.
The runner's missing-authority smoke tests execute from empty temporary working
directories with a fake credential; they cannot reach the actual batch even if
real approval is later recorded. This isolation was corrected during review.

Another review correction ensures a native/comparison exception clears any prior
optimistic output flag; both metric aggregation and semantic-review admission
independently reject an execution/evidence failure. The scorer never accepts raw
fact presence as a semantic pass. Correct final decisions cannot erase a wrong
raw extraction, and correct facts cannot excuse a wrong board or irrelevant
question. Usage sums disclose failed/unknown outcomes instead of calling missing
usage zero-cost execution.

A read-only compiler closure check confirmed **1,191 selected source/embed/test
and toolchain identity files**: 46 covered by the unchanged typed successor
qualification and 1,145 matching the original runtime closure byte-for-byte.
Preparation repeats this check and records each file's provenance. The standard
library retains the original pinned toolchain identity convention; this does not
claim a new independent toolchain certification. New evaluator code receives its
own frozen hashes and guard tests.

## Limits requiring actual acceptance

The unit fixtures are deliberately synthetic. They validate scorer mechanics,
not how the live model will interpret the new prompts. Some fixture quotes cover
whole source clauses/prompts to stress structure; they are not prompt examples
or a claim of good extraction behavior. All new raw outputs still need actual
requirement-by-requirement review with source fact references. Quotes and fact
tags do not prove semantic correctness.

The full new paid runner has not been exercised with a provider. Preflight and
offline guards cannot prove network access, model availability, firewall behavior
or that all 14 cases will complete. Unchanged transport/ledger code was exercised
with in-memory streamed responses in the prior selector qualification. This
successor does not add complete raw HTTP-stream archiving; available metadata,
selection files, child logs, ledger snapshots and native outputs are retained.

Preparation/freeze are not authorization. Do not write an approval from this
review, run a live probe, reuse old request slots, modify a qualified executable
to reuse a firewall allowance, or rerun failed cases. Required firewall authority
must name the new runtime and api.openai.com:443 separately. Once a real batch
starts, any source/protocol alteration requires a new explicitly recorded version;
keep every previous artifact and failure.

The exact-head production CI and separate new runner safeguard workflow must be
checked after publication. PR #14 stays draft pending genuine complete acceptance.
