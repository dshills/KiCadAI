# Offline review after the interrupted baseline

Date: 2026-09-10. Reviewer: Codex self-review, not a Gemini or independent
external review. Reviewed source: `ee7c0f42`; evaluator bytes remain those in
the committed acceptance freeze. This review does not authorize a recovery
baseline, production changes, a final run, release or merge.

## Findings and dispositions

1. **Connection failure is not a design failure.** The stored transport error
   occurs during TCP connection establishment. No response or provider proposal
   exists, so credentials, model access, compilation and all electrical/native
   gates remain untested. The baseline report and per-case audits preserve this
   distinction. Disposition: correctly reported; live evaluation is incomplete.
2. **The stopped campaign must not be restarted implicitly.** The production
   client makes one POST for this non-background call. The evaluator returns on
   provider errors; the supervisor stops subsequent cases on the observed
   timeout. The actual journal still contains one reservation. Disposition:
   verified for this failure path; recovery requires the pending scope decision.
3. **Offline checks must not consume provider budget.** OpenAI, Gemini/Google,
   and Anthropic credential environment variables were removed from the test
   and lint child processes under the API-key safety workflow. No live corpus
   phase or external review service was invoked. Disposition: offline-only.
4. **Temporary-only retention was a handoff risk.** A project-local archive now
   preserves the entire original 25-file tree, and every extracted payload was
   checked against the original byte count and SHA-256. The archive identity is
   in `baseline-archive.json`; original files remain untouched. Disposition:
   local-retention risk reduced. This is still not a remote evidence publication
   or a guarantee against deliberate cache cleanup.
5. **No production capability regression can be inferred from added docs.**
   A source-tree comparison against merged V23 found no change outside the
   isolated evaluator, command and milestone documentation. Historical evidence
   and support boundaries remain unchanged. Disposition: source scope verified;
   the regression results below are separate evidence, not new board passes.
6. **Limits of this review.** One-second RSS samples are not instantaneous peak
   memory measurements; the report says so. Missing usage retains an estimated
   reservation, not a claimed invoice. Local hashes are not independent signing
   or blind evaluation. The network-interrupted baseline cannot exercise the
   evaluator's successful live-to-board path. Disposition: keep these caveats and
   do not promote offline checks into milestone success.

## Verification

All commands use the Go/cache/environment settings in frozen SPEC, with
provider credentials removed for offline child processes. Test output and
completion status are recorded separately from live-generation metrics.

- Repository-wide `golangci-lint run ./cmd/... ./internal/...`: PASS, 0 issues.
- Repository-wide frozen bounded test command: PASS, exit 0. The complete
  package-result output is in `offline-bounded-tests.log`; its package set
  exactly matches `go list ./...`. This is the frozen short/bounded lane, not
  an exhaustive test claim. The topology-synthesis package completed in
  674.389 s, within its 15-minute test timeout.
- Native reference control on this reviewed source: PASS, two clean runs with
  equal normalized evidence; test body 45.13 s, package time 45.664 s. Retained
  output: `/tmp/kicadai-practical-native-plumbing-4`. This independent offline
  control ran while the long-running bounded synthesis test was still active;
  these timings are not isolated performance measurements.
  Both normalized-evidence files hash to
  `868e6d2425d21eb4122d0fb3bda6d078004e4f0288a7ef30e69163d82a98b4d8`.
  Test output is in `offline-native-reference.log`; retained control disk usage
  measured 175,144 KiB. No new-corpus case or live attempt ran during this check.

The reference control uses an existing regulated MCU/sensor requirement, never
a new-corpus input, and does not count as a capability uplift. Frozen acceptance
files and the interrupted raw evidence must remain byte-identical throughout
these checks. The milestone and its reviewed PR remain incomplete while the
live baseline recovery decision is pending.
