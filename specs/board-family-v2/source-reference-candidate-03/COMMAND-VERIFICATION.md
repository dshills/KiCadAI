# Explicit experimental command and terminal-process verification

Date: 2026-09-14. Local successor to [journal verification](JOURNAL-VERIFICATION.md).
No live API request, budget approval, final evaluation, publication or production
default change is represented by this checkpoint.

## Implemented command boundary

The same `kicadai-board-family` command now supports explicit
`--intent-protocol indexed-v3`, requiring a prompt, `--live-budget`, `--ledger`
and `--evidence-journal`. The default remains `typed-v2`. A policy file is not
spending approval, and the old exhausted ledgers are not new allowance.

The journal and output directory must be disjoint, including after resolving
existing symlink ancestors. This prevents journal preparation from creating the
generator's output directory and prevents output cleanup from owning evidence.
Unknown protocols and incompatible flags fail before extraction. Experimental
configuration-only generation is disallowed; normal `--config` still works
through the existing default path.

`--intent-protocol indexed-v3 --export-live-contract FILE` is offline and exports
the exact context and maximum-inventory schema blueprint. It explicitly explains
that actual requests tighten their clause/quantity bounds and remove the numeric
branch for quantity-free requests. This export is not a claim that a maximum
blueprint was the exact schema of every executed request.

The new command mode invokes the existing guarded journal path. There is no
production fixture-file switch, alternate provider URL, retry, unmetered path,
or network bypass. The command clears provider credentials before native work.

## Observed checks

- All command package short tests passed, **2.496 s**, exit 0: fourteen existing
  injected pipeline cases now use the real protocol/journal flags, plus contract
  export, nine incompatible flag cases, eight path separation cases, ten actual
  child-process outcomes, and existing typed/default guards.
- Child-process tests execute the real `main()` in the Go test binary, replacing
  only `http.DefaultTransport` with an in-memory response. This is not the
  separately built executable talking to a real provider. Parent observations
  come from `cmd.Run()` and `ProcessState`, not a state file or PID guess.
- The ten child cases cover clarification and unsupported decisions (exit 0),
  invalid extraction, malformed structured output, refusal, transport error,
  missing model, generation conflict and validation failure (exit 1), and an
  abrupt exit after request publication (exit 74).
- In every attempted case, exactly one reservation remained. Failed extraction
  retained wire evidence and its distinct outcome. Abrupt exit retained request
  evidence and an unsettled reservation, with no completed-selection checkpoint.
  A generation conflict preserved the deliberately created user-owned file and
  the independent journal. No model or exit failure was counted as successful
  board generation.
- Real native subprocesses passed BMP280 standard (**6.119 s wall**) and SHT31
  standard (**6.307 s wall**), terminal exit 0 and all 14 validation/export gates.
  Total test command **12.692 s**, exit 0. No manual repair or new qualification
  publication; provider responses were synthetic. A final assertion-only repeat
  also checked every gate individually plus nonempty BOM/previews/manifest:
  BMP280 **4.019 s wall**, SHT31 **3.898 s wall**, command **8.285 s**, exit 0.
- Race checks passed: command **8.878 s**, boardfamily **15.539 s**, aiprovider
  **2.957 s**, exit 0. The final assertion-only native test additions are checked
  separately, not retroactively included in those race timings.
- Full repository lint: **0 issues**, exit 0, using the workspace lint cache.
  Frozen final-02 publication authentication: passed, unchanged **5/14 complete**
  acceptance result and all original artifacts. No current-source CI pass is
  inferred from the earlier published head.
- The prior journal checkpoint's existing `make test-fast` session **72228**
  completed with exit 0; the open-topology package took **263.091 s**. That tier
  excludes six pre-existing heavy tests and predates these command changes. The
  older unfiltered short run's ten-minute timeout remains a failure, not erased.

All verification commands removed real provider keys and live-test enablement,
disabled module downloads, and reused local Go caches. Subprocesses received no
real key; the child installed its own dummy key and in-memory transport. The
API-key safety check reused the user's recorded credential decision without
reading or persisting its value. No screen control or firewall change occurred.

## Separately built executable

An actual command binary was built successfully into the newly allocated local
directory `.cache/board-family-v2/indexed-command-03.goeox3/`. It exported both
contracts with provider credentials removed; both commands exited 0 and their
contents were checked. This is an offline build/contract check, not a frozen or
live-approved evaluator runtime.

| Artifact in that directory | SHA-256 |
| --- | --- |
| `kicadai-board-family` | `5eeedd17a6e145006edd78ba06c1b2fa300957bfe05d0e648422cfc9742fdd17` |
| `default-contract.json` | `f4f039460befc30d3de8d47d5c0c5f27ba6c8509476da31d68a1d8146a1df55a` |
| `indexed-contract.json` | `61a66a05ae60b3564b977218a3adcf50cf46a02cdaf213639f242e74ed74332a` |

Default admission identity was `typed-requirements-02`; experimental identity
was `3-indexed-quantities-experimental`. Later edits require fresh runtime/source
qualification. No historical binary was overwritten.

## Collector review and next work

The subsequent [collector checkpoint](COLLECTOR-VERIFICATION.md) implements and
tests these continuation rules with the real Go journal auditor. The following
records the command checkpoint's earlier review and requirements.

Read-only inspection of the frozen typed-final-02 runner confirmed that it
requires `execution.exit_code === 0` before evaluating the selection. Thus a
complete, accounted provider response with a locally invalid extraction stops
that old runner. Its repeated stops must not be fixed by modifying frozen code
or by simply ignoring all nonzero exits.

The new collector must combine freshly observed terminal process state with
verified journal/ledger/runtime joins. The intended distinctions are:

| Evidence | Collection handling to implement and test |
| --- | --- |
| Complete, accounted response; `invalid_extraction` or provider refusal; expected error exit; no board | Preserve a measured failed case, then consider the next *unattempted* case under the same approved frozen budget. Never retry or count it as correct. |
| Valid decision; exit 0; appropriate output and reviewed native bundle if supported | Preserve the outcome for source-bound semantic scoring, including wrong but structurally valid decisions. |
| Missing/disputed provider metadata, incomplete transport, unknown/signal/timeout exit, accounting or journal error | Stop without retry; retain evidence and all remaining planned cases in the denominator. |
| Supported decision with generation/validation failure | Preserve the selection and partial output; stop rather than treating the selector's completed request as a completed board. |

These are requirements for the next collector implementation, **not implemented
continuation authority**. This checkpoint has no new collection loop. A new
runtime/corpus freeze, complete review, fresh exact-source verification and
explicit request/dollar approval remain necessary before any live evaluation.
The candidate's known semantic counterexample also remains unresolved; the goal
is still reliable natural-language generation, not merely durable failures.
