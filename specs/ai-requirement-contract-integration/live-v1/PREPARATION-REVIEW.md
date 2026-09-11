# Offline preparation and local review

September 11, 2026. Scope: the one separately approved eight-case interface
campaign. This record predates the freeze and any live request. Review was local
to the implementing task; no independent external or Gemini review is claimed.

## What was checked

- The exact corpus contains four ready, two refusal and two clarification cases,
  with 57 withheld acceptance clauses. The interpretation and feasibility limits
  are documented in `FEASIBILITY.md`. No practical-board prompt or result was
  reused as a live input, and no new board pass is claimed.
- Installed objective/participant capabilities and trusted analyses were loaded
  afresh. Required vocabulary checks pass for all eight cases; concrete circuit
  synthesis and physical feasibility are not established by vocabulary checks.
- A fake transport captured 16 actual production request bodies: initial and
  maximum bounded-diagnostic forms for every case. The largest initial body is
  92,325 bytes; the largest diagnostic body is 104,986 bytes. Both fit the frozen
  131,072-byte cap. Bound-answer bodies depend on returned clarification and are
  checked before dispatch, without truncating source facts or raising this cap.
- The successful preflight contains zero live requests and no account-access
  probe. Capabilities, schema, instructions, corpus and each captured body have
  recorded SHA-256 hashes. The live evidence root did not exist.
- Current official model and Structured Outputs documentation was checked before
  pricing was recorded. The cache-write multiplier is included in the conservative
  input rate. Actual account/model access is deliberately unprobed and can still
  fail on the first scheduled case; that failure must stop this campaign.
- Code review checked explicit live gating, a clean source/binary/freeze identity,
  exact endpoint/model/request settings, exclusive raw-root creation, pre-dispatch
  journal reservations, integer cost reconciliation, one correction per leg,
  whole-campaign provider-error stops, fixed-answer admission and hash binding,
  resource sampling, bounded raw capture, and separation of raw evidence from
  post-run audits. Production code and all historical evaluation files remain
  unchanged relative to the approved proposal's parent commit.

## Tests and preparation findings

All offline tests remove OpenAI and other provider keys and the live-test switch.
Go checks use Go 1.26.8, the local module/build cache, race detection and bounded
package parallelism. The evaluator and command tests passed three consecutive
runs after the replay finding below was addressed. Scoped `golangci-lint` reports
zero issues. Three independent Node authentication-helper tests pass.
The final affected-package command also passed:
`go test -short -race -p=1 -count=1 -timeout=5m ./internal/airequirementeval ./cmd/ai-requirement-eval ./internal/aiprovider ./internal/behavioralintent`.
The exact existing credential and credential-shaped-content scan was negative
across all 36 new files, with zero provider calls.

The Go tests cover wire-setting mutations, unknown request fields and oversize
requests; conservative reservation/reconciliation and cap failures; forbidden
endpoints, missing guards, capture overflow and credential-shaped output; no
transport retry; exactly one compiler correction; clarification admission deny
and approve paths; bound follow-up generation using synthetic responses; unsealed
build rejection; exact-secret scanning without false matches on ordinary words;
nonregular evidence rejection; offline recompilation; and inventory tampering.
Replay comparison tests retain duplicate diagnostics and reject changed messages,
status or capability identity. None of these synthetic tests is a live pass.

1. The first fake-transport preflight reached local RSS sampling and failed with
   `fork/exec /bin/ps: operation not permitted` inside the sandbox. It sent no API
   request and created no live root. Its partial snapshot was preserved at
   `/tmp/kicadai-interface-preflight-sandbox-1`. The same offline preparation was
   then run with the required process-inspection permission and keys removed;
   the successful retained snapshot is `snapshot/`. This was not a live retry.
2. An offline replay test exposed nondeterministic ordering of otherwise identical
   compiler coverage errors, caused by existing Go map traversal. The isolated
   replay comparison now sorts full diagnostic objects only, preserving all
   fields and duplicates. Production compilation, retained output and actual
   correction ordering were not changed. The comparison policy is explicit in
   `SPEC.md`; it does not claim byte-deterministic diagnostic ordering.
3. The terminal authenticator's inventory/path/SSE helpers have negative tests,
   and the sealed-binary replay path has a synthetic inventory integration test.
   Authentication of a real new terminal campaign has not happened because no
   campaign has run. A future authentication failure must be reported honestly,
   not repaired inside the raw evidence tree or converted into a pass.

## Remaining pre-live checks

Commit this reviewed source, create the hash manifest, commit that freeze alone,
and build the new fixed-path binary with the manifest hash injected. Verify clean
build/source identity, fresh installed capabilities, root absence and RSS access
using its offline `verify` command before any `run --live` invocation.

The Mac was locked when LuLu inspection was attempted; no UI state or rule was
changed. The user has been asked to unlock it. The old evaluator's restricted
rule is not authority to modify a rule or reuse its executable path. Confirm
access for `/tmp/kicadai-ai-requirement-eval-v1`; if a new rule is needed, request
separate explicit approval for only that executable to api.openai.com:443.
Do not start the one-shot campaign until this prerequisite is resolved.

No live spend, endpoint probe, new practical-board run, merge, release, stable
support expansion or additional provider review is part of this preparation.

Official sources inspected: [model documentation](https://developers.openai.com/api/docs/models/gpt-5.6-sol)
and [Structured Outputs](https://developers.openai.com/api/docs/guides/structured-outputs).
