# Final owned-v4 evaluation gate

The two-family workflow now has a separately versioned final collector,
read-only evidence verifier and source-bound semantic review entry point.
This is preparation for a real evaluation, not a new live success result.

The published owned-v4 checkpoint `2a2b7c17` passed all seven workflows,
including [full repository CI](https://github.com/dshills/KiCadAI/actions/runs/34975233288).
The final addition introduces an eighth, offline-only safeguard workflow.
Every release-gates record must name successful runs for the exact final
source commit; earlier green CI cannot authorize a changed evaluator.

## What is reused

The production and test executables, all 14 request-specific contracts,
the unchanged regression corpus, and the five qualified native board bundles
come from the retained [collection](COLLECTION.md). The
[raw scoring and synthetic review](SCORING.md) stay frozen and unchanged.
No Go compilation, KiCad regeneration, output repair or historical rescoring
was required to implement this final wrapper.

The new recorded safeguard run passed 88 checks in 6.535 seconds, using the
existing test binary and an in-memory provider transport. This comprises
36 approval/release-gate checks, 12 real subprocess/failure checks, the 37
unchanged pure scoring checks, and 3 final-entry checks. These are software
tests, not live accuracy or latency. The retained native rehearsal still
authenticates 14 synthetic outcomes, five native bundles and 201 compared files.

The new safeguard receipt binds all 24 final evaluator and shared dependencies,
the test executable, actual terminal process state, and captured stdout/stderr.
The final runtime additionally binds the complete application source snapshot,
reused qualification evidence, node/KiCad executables and production executable.
Source hashes bind actual files; the final manifest identifies the committed
source HEAD, not a dirty checkout base.

## Live authority is separate

The candidate policy is exactly 14 first-attempt requests and a maximum
1,000,000 micro-USD ($1) ledger cap, with the unchanged
`gpt-4.1-mini-2025-04-14` model and Responses endpoint.
**That policy is not approved spending.** Previous batch04 authority is exhausted.

Before the extraction child can receive the existing key, the collector needs:

1. The exact-source runtime, contracts and reused evidence to authenticate.
2. A separate release-gates record binding the manifest and source commit,
   completed code review, and all eight successful CI workflows.
3. A fresh explicit user approval binding that manifest, release-gates hash,
   fixed batch directory, model, endpoint and exact request/cost caps.

The approval must follow the release gates and expire within seven days.
It cannot permit retries, recovery or a different key. A gate/approval record
is a local accountability record, not a cryptographic attestation by GitHub,
the user or the model provider; the agent must record actual observed review,
CI and user approval rather than manufacture them.

Each manifest fixes one canonical absolute batch directory. There is no output
override or resume option. Exclusive directory creation prevents a second
invocation under the same batch identity, including after partial failure.
A test-only manifest cannot enter live mode. Keys are stripped from test,
contract-export and audit processes; only the approved extraction child
receives the existing OpenAI key. The application strips it before native tools.

Completed invalid extraction and provider refusal remain measured failures.
Transport/metadata faults, crashes, deadlines, log overflow, failed generation
or failed native validation stop before the next case. No failed attempt is
retried, refunded, repaired, replaced or removed from the denominator.

## Commands and records

All preparation and checking commands below are offline. Use the isolated
development worktree and preserve existing evidence directories.

```sh
node specs/board-family-v2/owned-evidence-05/final-runtime.mjs \
  --test REHEARSAL_DIRECTORY NEW_CHECKS_DIRECTORY

# After committing the verified source:
node specs/board-family-v2/owned-evidence-05/final-runtime.mjs \
  --freeze REHEARSAL_DIRECTORY REVIEW_DIRECTORY CHECKS_DIRECTORY NEW_RUNTIME_DIRECTORY

node specs/board-family-v2/owned-evidence-05/final-runtime.mjs \
  --check NEW_RUNTIME_DIRECTORY/manifest.json
```

Freezing creates neither an approval nor a release-gates receipt. It does not
connect to OpenAI or alter firewall rules. The standalone collector deliberately
has no implicit mode; its live entry requires all three separately recorded
inputs and must not be invoked without the fresh approval.

After an authorized collection, `final-authenticate.mjs` replays journals and
checks original prompts, actual request/response bytes, ledger prefixes,
terminal states, outcomes, file inventories and native comparisons. The copied
approval and release gates are authenticated at the recorded start time, so
their later expiration does not invalidate honest historical verification.

`final-scoring.mjs` then prepares an unapproved review template from those
authenticated live inputs. Every raw fact retains its original JSON and source
resolution. Explicit judgments cover meaning, source scope, completeness,
all gold requirements, and truthful/targeted application decisions.

Only the final file-backed route can report
`all-14-first-attempt-acceptance-met`, and only when all 14 raw and application
outcomes pass, all required bundles exist, and the original timing criteria
hold. The pure scorer cannot grant acceptance from a forged live flag.
This separation follows [OpenAI's evaluation guidance](https://developers.openai.com/api/docs/guides/evaluation-best-practices)
on the limits of automated metrics and the need for task-specific judgment.

## What remains unproven

There is no new owned-v4 live result yet. The most recent live evaluation is
still **9/14 complete, 11/14 application, 9/14 raw, and 3/5 required bundles**.
The corpus is known regression data, not an independent holdout or a
statistical reliability estimate. Implementing-agent review is not independent
review. Fabrication and physical bench performance remain separately
authorized, unproven milestones.
