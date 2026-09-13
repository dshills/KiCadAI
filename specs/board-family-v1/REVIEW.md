# Local review checkpoint

Reviewer: the implementing Codex agent. This is a disclosed self-review, not independent engineering approval. No code/material was sent to Gemini, another provider, or an external reviewer for this goal.

Reviewed the new configuration/selection/generation/validation/ledger command and actual integrated reference. Production generation preserves one native geometry; no arbitrary part selection or routing search is added. Existing repository parsers, connectivity checks, round-trip infrastructure and OpenAI provider are reused. Historical benchmark code and evidence remain unchanged.

Corrections made during review:

- Added sensor rail bypasses, repaired explicit development copper crossings and made schematic layout readable before acceptance.
- Moved controller bypasses closer while respecting courtyard/copper clearance, and recorded the two-layer stack explicitly.
- Corrected the omitted sensor internal-pull-up current; an independent contribution assertion now prevents integer-division regression.
- Persistently halt a ledger with anomalous usage; retain completed and unknown attempts and never refill the request counter.
- Bound prompt and configuration inputs, reject unknown/trailing JSON, exclusively create outputs, remove provider credentials before native subprocesses, and prohibit endpoint changes, redirects and automatic HTTP retries.
- Corrected native metadata UUID/value/parity problems and required unchanged input digests around native validation. No allowlist or disabled gate was introduced.

Verification: current-source focused tests, repository-wide bounded test tier, `go vet`, exact contract export comparison, all ten offline acceptance cases and three byte-identical clean replays. The current publication contains copied native/BOM/report evidence whose 234 recorded file digests were checked after copying. Changes to only input-size rejection were revalidated, not passed off as the earlier source version.

Open gates: real provider/schema integration; ≥9/10 first-shot supported language cases, 4/4 unsupported and 2/2 clarification; full natural-language runtime and cost results; final results review and one PR. The language runner is syntax-checked but unexecuted, and does not include expected answers in model input. Clause preservation proves no request text disappeared from the record; it does not prove arbitrary natural-language semantics were interpreted correctly.

Hardware/readability limitations are explicit in [FAMILY.md](FAMILY.md): no manufactured board, firmware execution, independent reviewer, PDN/transient/thermal/EMC measurement, guaranteed effective capacitance, or assembly approval. This lane does not inherit an old benchmark pass or fabricate evidence for those missing checks.
