# Local review checkpoint

Reviewer: the implementing Codex agent. This is a disclosed self-review, not independent engineering approval. No code/material was sent to Gemini, another provider, or an external reviewer for this goal.

Reviewed the new configuration/selection/generation/validation/ledger command and actual integrated reference. Production generation preserves one native geometry; no arbitrary part selection or routing search is added. Existing repository parsers, connectivity checks, round-trip infrastructure and OpenAI provider are reused. Historical benchmark code and evidence remain unchanged.

Corrections made during review:

- Added sensor rail bypasses, repaired explicit development copper crossings and made schematic layout readable before acceptance.
- Moved controller bypasses closer while respecting courtyard/copper clearance, and recorded the two-layer stack explicitly.
- Corrected the omitted sensor internal-pull-up current; an independent contribution assertion now prevents integer-division regression.
- Persistently halt a ledger with anomalous usage; retain completed and unknown attempts and never refill the request counter.
- Bound prompt and configuration inputs, reject unknown/trailing JSON, exclusively create outputs, remove provider credentials before native subprocesses, and prohibit endpoint changes, redirects and automatic HTTP retries.
- Corrected native metadata UUID/value/parity problems and required unchanged input digests around native validation. No per-finding exclusion or project-specific disabling was introduced; native default-ignored rule categories are disclosed in FAMILY.md.

Verification: current-source focused tests, repository-wide bounded test tier, `go vet`, exact contract export comparison, all ten offline acceptance cases and three byte-identical clean replays. The current publication contains copied native/BOM/report evidence whose 234 recorded file digests were checked after copying. Changes to only input-size rejection were revalidated, not passed off as the earlier source version.

## Post-live review

The original sixteen cases completed across three interrupted/continued executions, consuming seventeen physical requests. The first unknown result and the explicit recovery were distinguished. The resumed runner follows the complete hash-linked prior-run chain and executes only never-attempted cases; it cannot silently turn a retry into first-shot success.

The provider integration assumed an intent-envelope shape despite a caller-supplied root-object schema. Added `GenerateJSON` with identical outbound request bytes and shared transport/status/refusal/usage gates; the legacy `GenerateIntent` envelope gate remains strict. Tests verify both behaviors and preservation of accounting metadata on decode errors. One historical decode failure lost usage; its full reserve remains.

The clause decoder now restores only original whitespace at boundaries and retains the unmodified raw decision. Tests reject dropped negation, punctuation, trailing requirements, reordered/duplicated clauses and changed internal spacing. A separate deterministic low-current scope gate asks for clarification rather than accepting an unspecified energy goal. Recorded-response tests show `nl-05` now decodes and `ambiguous-02` now clarifies; the incorrect `nl-03` refusal is deliberately **not** silently rewritten. Revised model instructions address negation, ambiguous power and the existing maximum source capacity; they are not live-validated.

The data-validation skill informed a separate evidence audit: **382 file hashes**, all seventeen request/response-index bindings, token-based estimates, original first/latest denominators and conditional timings reconciled. Eight successful supported native projects match reviewed example geometry. The semantically rejected ambiguous board is not delivered as an accepted project. Cost is an estimate plus reserve, not a provider invoice or account-wide total.

Native report inspection found four ERC and five DRC default-ignored categories. Earlier absolute wording that no check was disabled was inaccurate; docs now name the categories and distinguish native defaults from added exclusions. No raw report was changed to hide this limitation.

Verification after corrections: focused tests and recorded-response replay pass; `go vet` passes; complete bounded repository regression passes, including the final [6.820 s cached integration run](evidence/regression/post-language-bounded.json). The earlier [74.174 s post-provider run](evidence/regression/provider-fix-bounded.json) also passed. This is disclosed self-review, not an independent review.

**Open acceptance gates:** original first selections **8/10**, first complete boards **7/10**, refusals **4/4**, clarifications **1/2**. Successful-board latency is fast but conditional on eight completions, not proof for all ten. Corrections cannot alter those original outcomes. A new holdout requires more than the three remaining request slots; the proposed 35-request ceiling and fresh prompts are unapproved. The final PR and goal completion remain pending. No additional provider upload, security change, hardware fabrication or higher dollar ceiling was used.

Hardware/readability limitations are explicit in [FAMILY.md](FAMILY.md): no manufactured board, firmware execution, independent reviewer, PDN/transient/thermal/EMC measurement, guaranteed effective capacitance, or assembly approval. This lane does not inherit an old benchmark pass or fabricate evidence for those missing checks.
