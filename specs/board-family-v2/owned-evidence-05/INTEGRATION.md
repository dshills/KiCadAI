# Owned-evidence 05: offline command integration

Status on 2026-09-15: **integrated and tested offline, not live-evaluated or
promoted**. This work follows prototype commit `168a6c23` on the isolated local
branch `codex/owned-evidence-05`. It is not a new AI evaluation result.

## What now works

The existing `kicadai-board-family` command has an explicit `--intent-protocol
owned-v4` route. It uses the application-owned reference compiler, a distinct
request context/schema, the existing guarded provider transport, immutable
capture, accounting, deterministic admission, generation and complete validation.
The default remains `typed-v2`. The indexed-v3 mode and its captured request
contract are preserved.

The common capture/accounting machinery now takes a closed internal protocol
choice. This avoids a second copy of the journal and audit engines. A response
or journal never chooses its own decoder: each inspector explicitly selects
one protocol and rejects the other's journal version. Unknown protocol values
cannot prepare, decode or dispatch a request.

| Boundary | Indexed-v3 | Owned-v4 |
| --- | --- | --- |
| Request revision | `indexed-request-04` | `owned-request-05` |
| Wire version | `3-indexed-quantities-experimental` | `4-owned-evidence-experimental` |
| Schema name | `board_family_indexed_requirements_v3` | `board_family_owned_requirements_v4` |
| Journal version | `indexed-evidence-journal-1` | `owned-evidence-journal-1` |
| Audit version | `indexed-journal-audit-1` | `owned-journal-audit-1` |
| Maximum encoded request | 24,000 bytes, unchanged | 65,536 bytes |

Both keep the pinned `gpt-4.1-mini-2025-04-14`, a 1,600-output-token ceiling,
one physical request, no redirects/retries/background requests, the existing
50,000-microdollar reservation and immutable ledger policy. The larger new-only
byte ceiling remains below that reservation under the ledger's existing
conservative one-input-token-per-byte arithmetic. This is an accounting bound,
not a new quote for provider pricing or permission to spend.

The model-facing source includes the full original request, clauses and literal
quantity table. It omits the redundant `NumericChoices` copy: eligible choice IDs
are already enumerated by the schema. Converted values still come only from the
application's original quantity table, never from model-authored numbers.

## Semantic change still requiring evaluation

The new extraction context does not include the capability catalog. It asks for
the user's requirements only; deterministic admission owns feasibility and
defaults. This directly separates requested state from what the available boards
can implement. The public family catalog, supported configurations and hardware
limits are unchanged and remain available through `--list-families`.

The instructions distinguish states, alternatives, temporal conditions, numeric
roles and actual constraints from incidental context. The aim is to reduce
invented features and polarity errors without replacing natural language with
a form or dropping unfamiliar requirements. This is an implementation hypothesis,
not a demonstrated accuracy gain. The deliberately wrong wired-to-wireless
counterexample in the prototype tests still remains possible.

Official GPT-4.1 guidance informed the use of clear extraction rules and an
explicit scope boundary. It also calls for empirical evaluation; documentation
does not establish that these instructions work on this board task.
[GPT-4.1 prompting guidance](https://developers.openai.com/api/docs/guides/latest-model?model=gpt-4.1)

## Exact contract export and audit

Owned schemas are request-specific, so export requires exactly one original
prompt or prompt file. A generic static blueprint is not presented as the exact
contract. Export is credential-free and cannot create a ledger or generate a
board. For example, with a locally built binary:

```sh
kicadai-board-family --intent-protocol owned-v4 \
  --prompt 'Please use BMP280 with standard profile.' \
  --export-live-contract owned-contract.json
```

The exported document contains the exact source table, context and schema for
that request, version identifiers and bounds. The journal retains the actual
HTTP bytes, including the unchanged provider wrapper, for exact runtime-bound
replay. Inspection is offline:

```sh
kicadai-board-family --inspect-owned-journal PATH_TO_NEW_OWNED_JOURNAL
```

These examples authorize no provider requests. The generation route additionally
requires a separate budget policy, ledger and private journal outside the output
tree. Policy files, old approvals, journal state and outcome labels are not
spending authority. No new live runtime was frozen or allowed through a firewall.

## Verification completed

Tests ran with the cached Go 1.26.8 toolchain and `GOPROXY=off`, `GOSUMDB=off`.
All real provider credentials and the live-test switch were removed from test
processes. In-memory transports use conspicuously fake test credentials; native
tools and child audit processes receive no provider credentials.

- All 14 known, hand-authored fixtures traverse the owned command flow and
  authenticate their eight-file journals. Full decisions/configurations match
  the existing synthetic admissions. Only the five supported cases generate.
- Those five generations produce **111 files total** (21 per BMP280 profile,
  24 per SHT31 profile), all byte-identical to their reviewed examples. Their
  ordinary short-test validation hook is a stub and is not counted as native
  validation.
- Separately, two explicitly enabled offline smoke tests use **real KiCad
  10.0.3**, one standard board per family. Both pass all **14** existing
  electrical, BOM, connectivity, parsing, ERC, strict DRC/parity, round-trip,
  preview, manufacturing-export and input-immutability gates. The three native
  file hashes per board match the reviewed examples. These check the new command
  integration; they do not replace the unchanged family qualification or prove
  fabricated-board behavior. The observed test duration was 7.98 seconds total.
- All 14 archived indexed-v3 requests still reproduce **exactly their original
  HTTP request bytes** and raw extraction JSON through the unchanged indexed
  contract. This is compatibility evidence only, not a replay scored as new AI
  success. Archive files are read, never rewritten.
- Twenty actual encoded request fixtures include the 14 known prompts and
  maximum-size/escaping/Unicode cases. Known prompts range from **6,847 to 8,531
  bytes**; the largest stress case is **63,229 bytes**. The simultaneous
  32-clause/128-quantity case is 39,199 bytes. These are full encoded request
  sizes measured against in-memory transports, not live token usage or latency.
- Eight tampering scenarios plus an unchanged control verify owned-journal
  rejection of altered decisions/outcomes, coherently rehashed requests,
  duplicate JSON keys, public files, extra/missing files and symlinks.
- Nine provider outcome scenarios retain the distinction between completed,
  invalid extraction, provider refusal, incomplete, wrong-model, transport,
  rate-limit and close failures. Existing journal reuse cannot dispatch again.
- Command tests cover nine invalid flag combinations, six contract-export
  conditions and seven extraction/generation/validation failure modes. Real
  child-process entrypoint tests observe terminal success for a truthful refusal
  and terminal failure for invalid extraction, with only selection output in
  both cases. Separate key-free child audits pass; the wrong inspector rejects.
- The affected short tests, race tests and lint pass. Lint reports **0 issues**.
  The short regression set includes `boardfamily`, the board-family command,
  `aiprovider`, `runtimebudget` and `fabrication`; race covers the first two.
  This is not a claim of a fresh full-project CI run on the unpublished branch.

Representative test commands, after configuring the pinned offline toolchain
and removing credentials:

```sh
go test -short -p=1 -count=1 ./internal/boardfamily ./cmd/kicadai-board-family \
  ./internal/aiprovider ./internal/runtimebudget ./internal/fabrication
go test -race -short -p=1 -count=1 ./internal/boardfamily ./cmd/kicadai-board-family
golangci-lint run ./internal/boardfamily/... ./cmd/kicadai-board-family/...
KICADAI_OFFLINE_NATIVE_CLI=/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli \
  go test ./cmd/kicadai-board-family -run '^TestOwnedCandidateCommandNative$' \
  -count=1 -timeout=3m -v
```

## Remaining gates and authority

The implementing agent reviewed protocol routing, request-size accounting,
capture durability, replay boundaries, credential removal and legacy isolation.
This is self-review, not an independent Gemini review. No new external review,
live evaluation, PR update or primary-branch change occurred in this step.

The next step is to finish the versioned collection/freezing and review work for
this exact route, reusing unchanged native qualification. It must preserve exact
request contracts and terminal process evidence, expose all failed cases, and
not silently adapt old evaluation records. A new live evaluation needs fresh,
specific approval after that preparation; the previous 14-request authority is
exhausted. No retry, probe or recovery is approved.

The authoritative live result is still **9/14 complete**, **11/14 application**,
**9/14 raw**, with **3/5 useful native bundles**. The overall reliability goal is
not achieved. Physical fabrication and bench bring-up remain separate.
