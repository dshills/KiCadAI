# Offline contract repair — September 11, 2026

The approved repair improves fail-closed validation, provider guidance, and
offline replay. It is **not a new live evaluation or a complete-board success**.
The practical board milestone remains unmet. No provider requests were sent,
no API spend was added, and no historical score, evidence file, publication,
sealed binary, model setting, or frozen campaign budget was changed.

Starting revision: `49953eaa599612aaf8e60f31ab3e786f32ddc722`.
Branch: `codex/ai-requirement-offline-repair`.
Scope and diagnostic correction: [PLAN.md](PLAN.md).
Local engineering review: [REVIEW.md](REVIEW.md).
Command outcomes and tested-source hashes: [verification.json](verification.json).
Historical authentication receipt: [preservation.json](preservation.json).

## Repairs

| Area | Change | Offline evidence |
| --- | --- | --- |
| Replay | Preserve an omitted empty `issues` member; sort only complete diagnostic objects | Nil/empty issues for ready, refusal and clarification; zero-issue end-to-end replay; inventory tampering rejected; changed status, diagnostics, duplicate diagnostics and non-diagnostic array order remain significant |
| Regulator provenance | A v3+ voltage-regulation objective must declare its own generated power signal and derived supply; an external rail cannot substitute for its output | Retained I04 counterexample rejected, including reordered bindings; existing coherent derived-rail fixture preserved; source substitution, wrong signal kind and extra external-output substitution rejected |
| Compiler diagnostics | Name rejected coverage IDs, operating targets and duplicate constraints | Project/constraint/local participant-port references and objective operating targets still rejected; normalized ambient-target index identifies the actual offending objective |
| Provider contract | Explain legal identities, source/sink provenance, voltage bounds, normalized paths and observation limits | Existing contract conformance and affected package tests pass; 16 initial/correction request forms captured offline |
| Observation schema | Whole-circuit observations require both kind and ID to equal `circuit` | Valid circuit/port forms accepted; wrong circuit ID and unsupported participant observation kinds rejected |
| Clarification test handoff | Atomically publish the synthetic approval file after it is fully written | Approval/rejection workflow passes 100 repetitions; production malformed-approval rejection remains unchanged |

The I04 fixture is the original retained JSON plus one final line feed for the
repository file. A test pins the original content SHA-256
`ea3ff80eaed18fec6316a56a61a47046285576d0b56b29de48ec445a87c94b02`;
the repository file SHA-256 is
`1db54ac9e1953cb99682d0c7507705fb7c71c110fc0a34c6bede5aba2a21ebfb`.
It is a known-failure regression, not held-out evaluation evidence.

## Verification

The new failure regressions were first observed failing before implementation.
After implementation, affected package short tests and race tests passed, the
approval test passed 100 repetitions, `go vet ./...` passed, and the configured
lint pass on affected packages reported zero issues. The local request capture
measured **114,651 bytes** at maximum, below the unchanged **131,072-byte** limit.
This check covers the initial and maximum-diagnostic correction forms only; it
does not claim a bound for every possible clarification follow-up or prove
live provider acceptance of the schema.

The first repository-wide short run did not pass: it exposed the approval-file
test race and reached its five-minute package timeout in topology synthesis.
The race was repaired in the test writer; the complete topology package then
passed in 686.761 seconds with a twelve-minute local test allowance. All 164
other packages also completed successfully (150 test passes, 14 with no tests).
The final combined `go test -short -p 4 -timeout 12m ./...` also passed all 165
packages (151 test passes, 14 with no tests, with normal Go cache reuse).
This local test allowance is not a live campaign resource change. Final command
outcomes are recorded in the verification receipt.

Initial toolchain setup also failed before tests ran: the ambient `GOROOT`
pointed to a different compiler version. Explicitly selecting the already
cached Go 1.26.8 toolchain resolved that without a download. Lint's first pass
could not persist its default cache under the sandbox; a workspace-local cache
produced a clean zero-issue run.

All Go tests use provider credentials and the live-test switch unset, with
`GOPROXY=off`, `GOSUMDB=off`, `GOTOOLCHAIN=local`, `GOENV=off`, `GOWORK=off`,
an explicit matching `GOROOT`/`PATH`, and workspace-local build/module caches.
Provider-shaped unit requests use an in-memory transport with no network
delegate. Official [Structured Outputs guidance](https://developers.openai.com/api/docs/guides/structured-outputs)
informed the schema-versus-semantics distinction; the credential-safety skill
kept test processes credential-free. Existing-key access in authentication is
solely an exact-secret scan of local bytes, not authentication to a service.

## Historical evidence remains negative

The source-bound interface authenticator runs from the unmodified starting
revision at `.cache/interface-v1-source-49953`, not from the repaired compiler.
Its archive paths are hard links to the unchanged local archives. Verification
reproduces the original publication and independently compares SQLite and Node
rows: 304 raw files, 110 frozen inputs, 8 cases, 57 source-bound clauses and 18
publication files. Raw and sealed-binary archives verify after extraction.
The old sealed replay still failed; the separately reported 15-attempt helper
match and 8 rejected tampered bindings do not turn it into a sealed pass.

The practical protocol-v2 history also authenticates: 472 files, 101,237,999
bytes, 16 cases and 108 audited clauses, plus the earlier baseline/recovery
inventories. Its archive hash and sealed evaluator binary remain unchanged.
Neither history demonstrates the required six complete positive boards or the
two materially different baseline-to-pass designs.

Reproduce these read-only checks with the existing key already available in
the environment (never paste it into a command):

```sh
node specs/ai-requirement-contract-integration/offline-repair-v1/verify-preservation.mjs
```

The verifier requires the retained local evidence, archives, binaries and the
historical checkout. It prints a receipt and may extract archives into a fresh
temporary directory; it neither modifies historical evidence nor contacts a
provider. A different historical checkout can be supplied as its sole argument.

## Remaining work before another live experiment

1. Specify and test faithful measurement identity for a particular participant
   endpoint. V3 currently has only port, signal, domain and circuit observation
   kinds. A whole-circuit assertion is not proof of an individual ADC input.
2. Resolve the supported representation for a standalone regulated external
   output. Existing generated power signals require a source and a real sink;
   do not add a fictitious consumer, controller or extra function to satisfy it.
3. Prove any proposed representation with offline feasibility and failure tests,
   preserve engineering gates, then obtain separate authorization for a fresh,
   preregistered held-out live protocol. Do not relabel retained failures as a
   new baseline or rerun them as evidence of generalization.
4. Only after faithful requirements succeed, pursue schematic, electrical,
   readability, routing, KiCad round-trip and replay evidence for complete
   boards. This repair establishes none of those downstream successes.

No Gemini review, remote push, PR opening, merge, release or fabrication is part
of this offline approval. Local self-review is not independent review.
