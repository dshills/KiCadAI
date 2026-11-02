# Offline experiment checks — 2026-09-14

These are local development checks, not a frozen evaluation or release approval.
The candidate remains disconnected from the production selector.

## Completed evidence

- Fourteen actual final-02 selections replay through `DecodeIntent` with their
  original decisions and exactly the same six validation errors. All 14 are
  rejected if passed directly to the experimental version-3 decoder.
- Fourteen implementing-agent-authored synthetic fact lists for those known
  prompts produce the expected local decisions/configurations. No live score is
  changed. This is not independent or unseen language evidence.
- Final focused race run passed for `internal/boardfamily` (6.460 s) and
  `cmd/kicadai-board-family` (1.712 s), with `-short -count=1`.
- Targeted `golangci-lint` completed with zero issues.
- A 15-second bounded fuzz run completed 346,659 executions, with no invalid
  configuration/source-retention invariant failure (16.308 s command duration).
- The portable final-02 authenticator passed before and after these edits:
  all 203 original publication files plus the five-file CI addendum remain
  authenticated; 14 attempts, raw 5/14, application 7/14, complete 5/14,
  two native bundles, 81 compared deliverables. The failed scores are unchanged.
- PR #14 was checked at the start: open draft, head
  `f8ac5099c2e2d21a72a2faf2bb2161ffbec8792a`, all 27 checks successful. Those
  published checks do **not** qualify these new, local experimental files.

All Go checks stripped provider credentials, disabled live-provider tests, and
used `GOPROXY=off`, `GOSUMDB=off`, and the repository's existing Go caches. No API
requests, native builds, paid reviews, firewall operations, or screen controls
were used. No evaluated output or historical source was repaired.

## Failed or superseded checks retained in this assessment

The initial test launches could not access the default external Go build/module
cache paths under the sandbox. The successful launches used the repository's
existing `.cache/go/build` and `.cache/go/mod` paths; no dependency download was
needed or enabled.

Two initial omission-test expectations incorrectly demanded clarification for
requests independently known to be unsupported (150 pF with fast, and a 200 kHz
I2C clock). The application correctly refused those requests. The tests were
adjusted to use supported numerical values when isolating the omission question,
and a separate test retains the rule that known conflicts take precedence.
No admission/safety check was weakened to make the assertions pass.

A broad `go test ./... -short` run observed one of those earlier test expectation
failures. It was explicitly interrupted while still running unaffected packages,
after the corrected focused race run passed; the command returned exit 1.
**No full-suite pass is claimed for this candidate.** Full production qualification
and exact-head CI remain necessary if it is later proposed for adoption.

## Review judgment

Implementing-agent review only. The experiment removes quote transcription and
per-clause response bookkeeping while retaining local numeric and admission
checks. It does not solve semantic classification: the explicit indoor-project /
custom-geometry counterexample remains a wrong user outcome despite valid source
references. Do not publish a structural test pass as language reliability or use
this result to justify another live batch yet. See [next engineering boundary](README.md#decision-and-next-engineering-boundary).
