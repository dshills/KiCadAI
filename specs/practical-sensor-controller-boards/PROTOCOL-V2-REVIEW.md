# Protocol-v2 publication review

Assessment: **ready to share as a negative baseline, with the limitations below**.
The practical-board growth milestone is not achieved. Production implementation
and final/paired campaigns require a new scope decision.

Reviewer: Codex, September 11, 2026. This is single-agent self-review, not Gemini
review, a second independent reviewer, external attestation or hardware testing.
No external review service or new provider request was used for publication.
The data-quality and report-validation workflows guided completeness, source
bindings, denominator definitions, missing-stage treatment and limitations.

## Reviewed scope and evidence

The complete terminal baseline has 16 cases in frozen order: eight primary
boards, four refusals, two clarifications and two non-independent paraphrases.
All 108 acceptance clauses have separate dispositions and source references.
No compiler-ready requirement, circuit, native project or replay exists for
these cases. Seven downstream gates for each board/paraphrase are not run;
provider-limit failures also leave requirement interpretation unaccepted.

The [results](PROTOCOL-V2-RESULTS.md), [aggregate JSON](protocol-v2-results.json),
16 per-case audits and [scope decision](SCOPE-DECISION.md) were reconciled against
the terminal records and retained responses. P07/P08 results are disclosed but
were not used to select implementation changes. Final/paired comparison metrics
are explicitly unavailable. No production correction was implemented.

## Findings and dispositions

1. **Plausible prose is not a valid contract.** Non-reserved positive proposals
   fail semantic/version/coverage validation. N01/N04 contain appropriate
   limitation prose but invalid refusal records; C02's question-content clause
   passes while its structured workflow fails. N02 is the only full behavioral
   success, after correction. Disposition: grade full workflows conservatively;
   do not infer electrical failures or downstream capability from compilation.
2. **Transport completion differs from client acceptance.** P07/W01/W02 retain
   complete streams exceeding the unchanged 2 MiB client limit. C01's correction
   lacks a terminal response. Disposition: preserve failures and original usage
   reservations; expose supplemental raw usage without accepting rejected JSON.
3. **Opaque redaction changes retained length.** Exact/key-shaped redaction can
   match encrypted metadata. The recorder labels pre-redaction length as
   `retained_bytes`. Disposition: disclose actual byte length and every redacted
   JSON location; check stream sequence and final text. All observed redactions
   are confined to opaque encrypted-content fields. Frozen records are untouched.
4. **The library index exceeds a JavaScript single-string limit.** The initial
   publication-only whole-index scan could not convert the decompressed buffer
   to one string. Disposition: bounded overlapping chunk scan, with clean-buffer
   and chunk-boundary regressions; successful exact-credential and key-pattern
   scan of all retained files and the decompressed index. No evaluator change.
5. **Audit tooling must reject incomplete publication.** The checkpoint audit
   checker originally reported a partial corpus without failing. Disposition:
   terminal publication now requires exactly all 16 audit filenames; empty,
   missing and extra-file tests fail closed before reading raw evidence. A draft
   N02 evidence pointer to an absent issue list was corrected to its retained
   compilation status. All final pointers resolve. No raw evidence was edited.
6. **Integrity is local, not independent provenance.** The outer inventory seals
   472 files / 101,237,999 bytes, including case inventories and journal files.
   It authenticates retained bytes, not external timestamps or original redacted
   ciphertext. Disposition: retain this limit and disclosed self-review status.
7. **Remote raw-evidence availability remains limited.** The 102,119,424-byte
   archive is local in ignored `.cache`, not in Git or remotely backed up. Every
   extracted payload matched the pinned tree. Disposition: PR publishes audits,
   hashes and metadata, but full raw revalidation requires access to that archive,
   the preserved prior cohorts, the sealed evaluator and the local scan credential.
   Do not characterize the PR alone as independently reproducible raw evidence.
8. **Resource and cost observations have bounded meaning.** RSS is sampled and
   review intervals overlap. USD 6.646416 is cumulative estimated/reserved cost,
   not actual billing; seven receipts lack reconciled usage. Disposition: retain
   unknowns and separate build/preflight/review costs. No further spend is inferred
   from the remaining budget.
9. **No credible three-fix complete-uplift plan is established.** Schema/prompt
   improvements can change fresh proposals without qualifying the same retained
   baseline inputs. Downstream gaps remain unmeasured. Disposition: stop before
   implementation and request the separate integration scope; do not weaken the
   compiler, repair case inputs or run another campaign to improve the headline.

## Verification

Publication checks are read-only, make no provider calls and are distinct from
the generation budget. The final repository-path checks on September 11 passed:

- 13 publication tests: seven stream integrity tests, three chunked secret-scan
  tests and three exact-corpus completeness tests.
- All 16 audits, 108 clauses and 565 evidence bindings resolve; file hashes,
  JSON pointers, review intervals and full-pass refusal preconditions verify.
- Full terminal raw-tree authentication: 472 files / 101,237,999 bytes; all 25
  frozen files and the sealed binary match. All three prior cohort inventories
  reverify without changes. The pinned outer inventory matches exactly.
- Request accounting: 29 new requests, three carried once, contiguous receipts
  001–032; unchanged request shape, model, attempts, token and resource caps.
- Published case fields, metric denominators, missing comparison values and all
  16 table rows reconcile with audits and authentication metadata. Local Markdown
  links resolve. The table uses original case order, explicit units and neutral
  status text; it is a lookup table, not a statistical or trend visualization.
- Archive byte count/SHA-256 reverify; prior extraction comparison matched all
  472 payloads with no missing or additional files.
- JavaScript syntax and Git whitespace checks pass. The branch is confined to
  the isolated experimental evaluator/command and milestone documentation;
  production behavior and historical V18–V23 evidence are unchanged.

Earlier checks remain separately dated evidence, not fresh board successes:
[offline review](OFFLINE_REVIEW.md) records 162 bounded package results, zero
lint issues and the existing native reference's two matching clean replays;
[v2 verification](protocol-v2-verification.json) records focused race tests and
five protocol/helper tests for the approved timeout-only revision. The full Go
suite and native reference were not repeated for publication-only additions.

## Revalidation

```sh
node --test specs/practical-sensor-controller-boards/publication-v2/*.test.mjs
node specs/practical-sensor-controller-boards/publication-v2/verify-publication.mjs
node specs/practical-sensor-controller-boards/publication-v2/verify-publication.mjs --verify-local-archive
node specs/practical-sensor-controller-boards/publication-v2/verify-audits.mjs
node specs/practical-sensor-controller-boards/publication-v2/authenticate.mjs
```

The first two commands need only the checked-out publication files; the third
also needs the recorded local archive. The last two need the original raw root;
full authentication additionally requires the retained binary, prior raw cohorts
and previously authorized key for the local exact-secret scan. The key is never
printed or transmitted by these checks. Authentication refuses a live campaign.
Metadata consistency is not a replacement for raw authentication or semantic
review. None of these commands reruns generation or changes retained evidence.

## Handoff decision

Open the reviewed evidence PR without merging or releasing. Request approval of
the separate AI-facing integration milestone before production edits or another
live protocol. No stable-support expansion, fabrication approval or full-board
capability claim is supported by this baseline.
