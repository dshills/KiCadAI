# Requirement fidelity candidate — offline verified, live unproven

2026-09-25. Base: 2147afa59213beb80016fcb4a81b47774ec26466.
Working branch: codex/coverage-fidelity-15. No publication or merge performed.

## Scope

The closed V11 run remains 11/14 complete passes, 5/5 native bundles. Its
original prompts, gold, provider bytes, source checkout and executable remain
unchanged. This candidate does not claim to revise or pass that run.

Opt in with `--intent-protocol requirement-fidelity-v12`. Default typed-v2
and V11 remain unchanged. The new protocol has a distinct wire version,
request revision, schema name, ledger accounting identity and journal/audit
identity. The schema structure and source-table/compiler logic are reused.

Changes address the three recorded failures:

- Accepted named choices are explained as requested, not merely optional.
- Explicit no-omission/no-substitution restrictions are distinguished from
  task framing; clear restrictions are distinguished from genuine uncertainty.
- Candidate refusal messages no longer prescribe an external USB-power or
  UART substitute. Both the main message and clause reasons use the revised
  catalog explanation. Extracted facts, disposition and configuration are not
  repaired or rewritten. Legacy messages remain unchanged.

This is prompt guidance plus scoped refusal wording, not a semantic proof.
The known omitted-constraint vulnerability remains. No regex based on the
evaluation prompts, gold injection, response retries or output repair was added.

## Verification

Using cached Go 1.26.8 with GOPROXY/GOSUMDB off and an empty environment apart
from HOME/PATH and explicit Go paths (API credentials absent):

- `go test ./internal/boardfamily ./cmd/kicadai-board-family`: passed both
  packages, including new synthetic corpus, command gates, exact provider
  payload checks, accounting/ledger isolation, version isolation and journal
  tamper checks. Synthetic provider responses are not live accuracy evidence.
- `go vet ./internal/boardfamily ./cmd/kicadai-board-family`: passed.
- `TestFidelityCommandNative` with real KiCad 10.0.3: all five configurations
  passed all 14 gates, with reviewed native hashes unchanged. 21.83 seconds
  for the test; extraction was synthetic, so this is not live latency.
- An offline-only build replayed all 14 actual historical V11 journals and
  exported all 14 old V11 contracts; every JSON value matched the frozen
  originals. Script: `.cache/fidelity-15-compatibility.mjs`.
- `git diff --check`: passed.

The two initial setup attempts failed before tests because the default Go
cache was inaccessible and the default module cache lacked pinned modules.
The successful run used the project's existing cached dependencies. No
dependency updates or network downloads were performed.

## Remaining work

Local review found and removed a post-render string replacement in the initial
candidate. A closed admission-explanation policy now selects only application
catalog reasons before rendering. An adversarial regression verifies that
untrusted detail identical to the old USB reason remains unchanged. The first
regression fixture failed preflight because it omitted quantity classifications;
the corrected fixture isolates this preservation property without quantities.
This test is not evidence that the adversarial detail is semantically correct.

No live V12 request has run. No firewall rule was added, existing rule changed,
PR updated, source committed or branch pushed in this offline step. The local
check binary was intentionally built with buildvcs=false and is not a release
executable; exact source provenance is required before a live release run.

The user granted standing approval for runs within existing request/spending
limits. This does not increase limits or grant firewall changes/publication.
Do not reuse V11's consumed 14-slot ledger or overwrite any frozen evidence.
Next release preparation must bind a new candidate identity, retain the same
14-case denominator and acceptance thresholds, and account for authorized
spending explicitly. Do not claim prompt fixes have solved model behavior
until a separately recorded live run and semantic review establish it.

The API-key safety skill kept this implementation and its tests credential-free.
