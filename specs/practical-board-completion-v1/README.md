# Practical-board completion: negative readiness publication

The practical AI-generated sensor/controller-board milestone is **not achieved**. The renewed final live evaluation was **not run**: six human-authored, non-reserved development probes failed before electrical synthesis or native creation. This publication closes the approved attempt with evidence and a limited correctness fix, not a success claim or an exhausted search for feasible implementations.

## Results

| Measure | Observed result |
|---|---|
| Original complete positive baseline | 0/8; unchanged, not rescored |
| Renewed live final | Not run; scores and paired improvements unavailable |
| Development | 6 candidate search rejections; 5 baseline search rejections; 1 baseline protocol incompatibility |
| New electrical / native / replay executions | 0 / 0 / 0 |
| New API requests / estimated or reserved cost | 0 / USD 0 |
| Development allocation | 1 of at most 3 batches used; 2 deliberately unused |
| Original corpus | 8 positives, 4 refusals, 2 clarifications, 2 paraphrases retained; P07/P08 not executed or tuned |

The probes are not accepted AI outputs. Their input-shape validity does not demonstrate clause fidelity: some constraint facts are unconsumed, and P04/P06 have unresolved interface-encoding limitations. P04's old-decoder rejection is not a material engine uplift. See [the readiness decision](READINESS.md), [all 60 development clause audits](clause-audits.json), and [machine-readable results](results.json). The eight gate names follow the SPEC's ordered gate paragraph; the separate no-manual-output-repair requirement is also recorded.

The source change fixes common electrical-return handling when no side-specific reference was provided; an invalid explicit side reference still fails closed. No catalog entries, preset circuit answers, case-ID production branches or new readability algorithm were added in this attempt. The new offline adapter uses real search/synthesis/native APIs, but these probes never reached its native path, so this experiment does not certify that integration.

## Verification and review

At source `75cfcc978f37ef751aab58dfc1fb2cdfc16ecef7`, `go test -short -count=1 -timeout=12m ./...` passed **152 packages**, with 14 packages having no test files. The longest package took 672.259 seconds. Scoped reference, adapter and evaluator race checks passed, as did repository vet, command/internal lint and 9 Node evidence tests. The reference regression's expected red run is retained. The historical full composition race timeout remains uncertified; scoped race checks do not replace it.

See [verification receipts](publication/verification.json), [publication QA](publication/qa.json) (14 Node tests, including 5 new report checks, and successful read-only reauthentication), and [local self-review](REVIEW.md). This is implementing-agent review, not independent engineering or Gemini review. The PR includes accumulated earlier unsubmitted work: the pre-publication snapshot has 747 changed files, including 131 command/internal files. [The scope inventory](publication/branch-review-scope.json) is bound to its recorded revision and excludes these later publication additions. Earlier phase reviews are retained, not represented as a new independent full-PR review.

## Evidence and reproduction

- [Development summary](publication/development-summary.json), [frozen batch](development-batch-1.json), and [preparation ledger](publication/preparation-summary.json).
- [Original evidence authentication](publication/history.json): all 25 frozen files and 472 original raw files matched their historical seals.
- [Eight closed offline phases](publication/history-closed-phases.json): retained raw/source-at-commit and archive hashes reauthenticated, without rerunning them or changing their conclusions.
- [Archive record](publication/archive.json) and [456-file manifest](publication/archive-manifest.json): 202,181,472 raw bytes; gzip archive 93,501,440 bytes, SHA-256 `155394882dcc4f9d6fd7f5e4d32f138bd1edac7a19f185b92772e8d2bf97b4db`. Every archive member was streamed and checked before publication.

The raw tree is local at `.cache/practical-board-completion-v1/{preparation,checks,development}`; the local archive is `.cache/practical-board-completion-v1-readiness.tar.gz`. Large raw archives are **not uploaded in this PR**. A repository clone alone therefore cannot reauthenticate unavailable local raw files; the verifier fails rather than silently skipping them. SHA-256 establishes retained-byte identity, not external attestation, electrical correctness, actual billing or manufactured hardware performance.

From the repository root, `node specs/practical-board-completion-v1/verify-publication.mjs` performs read-only consistency/hash checks. The existing approved `OPENAI_API_KEY` is required only in memory for an exact local leak scan; the verifier sends no network requests, never displays the key, and strips provider credentials from child processes. Publication tests need no key: `node --test specs/practical-board-completion-v1/publication.test.mjs`.

Do not rerun the frozen development or launch a live campaign as a publication check. The preparation/development runners are historical experiment recipes, not a new authorization. The retained pre-spawn naming error and initial audit traversal-order error are disclosed in the ledger and [audit-attempt record](publication/audit-attempt-1.json); neither is erased or counted as a successful evaluation.

The next substantive gap is faithful requirement-to-provider evidence propagation and interface semantics, followed by complete electrical/native/readability/replay proof. Further work needs an explicit implementation/experiment scope; this publication does not weaken the 6/8, two-improvement, reserved-case or negative-control gates.
