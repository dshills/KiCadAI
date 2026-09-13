# Board-family v1 — autonomous implementation

Authority: user-supplied goal, September 13, 2026. Build one reviewed low-voltage sensor/controller board family with three meaningful configurations, deterministic generation before AI, 10 configuration passes, >=9/10 unseen language selections, 4 refusals, 2 clarifications, native/electrical/readability checks and three representative deterministic replays. Runtime target: median <5 minutes, each supported case <10 minutes. One reviewed PR; no merge or fabrication. Original existing-key ceiling: 20 live requests and USD 10; subsequently extended to **35 total requests, same USD 10**, per `evaluation/AUTHORIZATION.md`. Earlier entries below describe historical checkpoints, not current permission or usage.

Base: `7076c6529e978477d73e2c22c81ef930b2f35036`. PR #12 was open at the initial checkpoint; it is now verified merged into main (`b5565763`, September 13). Branch `codex/board-family-v1` is a separate continuation; historical files and results remain unchanged.

Implementation sequence:

1. Qualify a complete existing-controller/reference sensor board and produce native output.
2. Reuse its schematic, placement and local routes through a small validated family configuration contract; add meaningful supported variants.
3. Add constrained language selection and a single persistent request/cost ledger.
4. Run the separately declared acceptance set, inspect rendered drawings, record timings and replay results.
5. Run integration regression/review and publish usable commands, examples, BOMs and one PR.

Development inputs may be edited and implementation failures repaired. Keep compact logs under `.cache/board-family-v1/`; do not turn each edit into a new frozen campaign. Generated evaluation artifacts must never be manually repaired. Reference engineering and implementing-agent review are disclosed assistance, not independent hardware certification.

## Implementation checkpoint (September 13)

Selected family: ESP32-WROOM-32E-N4 + BMP280 wired pressure controller, external regulated 3.3 V, 120x80 mm / two copper layers. Three electrical profiles use 4.7k / 2.2k / 10k bus pull-ups with distinct speed/current/capacitance envelopes. This uses one sensor type, not three unrelated PCB architectures. Radios, batteries and additional active loads are outside the qualified scope.

Development engineering (all historical evidence unchanged):

- ESP32-only native pass: 11.535 s. SHT31 integration probe had incomplete routing; not pursued for this first family.
- BMP280 integration routed completely. Sandbox KiCad crashed during macOS UI initialization; ordinary offline DRC revealed one real copper crossing.
- Added an explicit two-via VCC bridge; aligned schematic/PCB net names after verifying the electrical partition; shortened references.
- Visual review found inherited off-sheet/crowded schematic geometry. Rebuilt drawing layout from the netlist, retained installed pin definitions, fixed grid and grouped-pin handling.
- Bosch Figure 17 required separate VDD/VDDIO bypasses. Added C5/C6 and explicit local routes. Engineered reference 06 passed ERC, strict DRC, routing and schematic parity.
- Native writer/round-trip checks caught missing property UUIDs, duplicate empty metadata, and stale schematic instance values. Fixed serialization and saved a separate development copy with KiCad to establish native canonical ordering. No validation allowlist or disabled checks.
- New deterministic command reuses repository parsers, writer validators, complete connectivity, KiCad round-trip checks and existing OpenAI provider. Geometry is embedded reference data; no runtime placement/routing search.
- Complete DEVELOPMENT runs: standard 3.081 s; fast 3.444 s; low_current 3.445 s. All checks passed; the latter two also include the new approved-reference/BOM integrity gate. These are not final acceptance results.
- Focused tests pass for margins, unsupported/malformed limits, deterministic native outputs, writer/connectivity, BOM completeness, non-overwrite, preserved language clauses, refusal/clarification gates and ledger bounds/corruption/locking.

Live testing attempted only after deterministic readiness. Automatic review rejected the command before execution over concrete-payload authorization; no API call was sent and no live ledger exists yet. Remaining scope continues offline while this authority is checked.

One intended live ledger for this entire goal: `.cache/board-family-v1/live-ledger.json`. Pinned model `gpt-4.1-mini-2025-04-14`; official price checked at $0.40/M input and $1.60/M output. Each physical HTTP attempt reserves $0.05 conservatively, keeps its request count permanently, disallows other endpoints/redirects/retries, and records usage without credentials.

## Offline integration checkpoint

- Declared ten numerical acceptance cases and sixteen language requests before any live request. Numeric language expectations were made explicit before live execution; no prompts were changed in response to model results because there are no model results yet. The implementing agent authored the set; this is not an independently authored benchmark.
- Initial offline acceptance: 10/10, three byte-identical replays, median 3.331 s. Preserved in `.cache/board-family-v1/acceptance-configurations-01`.
- Reference review moved C1/C2 close to the controller. Development placement 01 overlapped the module's extended courtyard; 02 cleared that but failed enable-track clearance; 03 passed all native checks. Development 04 additionally recorded the nominal two-layer stackup. Canonical native copy 03 became the embedded PCB; schematic connectivity and BOM did not change. Acceptance outputs were never edited.
- Second offline acceptance: 10/10, three replays, median 3.301 s. Cross-checking calculated values then found the internal 70 kΩ pull-up term was lost to integer constant division. Replaced it with floating-point division and added a separate current-contribution regression assertion. Earlier calculated current allocations are historical, not the final accepted values.
- Corrected-current offline acceptance (`acceptance-configurations-03`): **10/10**, **3/3** identical replays; median **3.254 s**, maximum **3.684 s**. All electrical/native/writer/ERC/strict DRC/parity/connectivity/round-trip checks passed.
- Final input-hardening review rejected oversized configuration JSON explicitly instead of allowing a bounded reader to conceal trailing content. Final source-matched offline run `acceptance-configurations-04` again passed **10/10** and **3/3** replays, median **3.273 s**, maximum **3.564 s**. Its hash-verified evidence and three unchanged example projects are the current publication. The prior publication copy is preserved under `.cache/board-family-v1/publication-checkpoint-03` with its original run. Final bounded regression passed in 7.455 s (unchanged packages cached).
- Safeguard review: accounting anomalies now persist a halt before returning an error; prompt-file reading is bounded; non-supported output directories are exclusively created; native children receive no provider credentials. Focused tests and vet pass.
- Repository-wide bounded regression (`go test -short -p=1 -timeout 20m ./...`) passed in 996.634 s initially. Post-change runs passed in 13.683 s and 7.738 s using the Go test cache for unchanged packages. This matches `make test` / `make test-bounded`; it is not the separate exhaustive release tier.
- Readability reviewer: implementing Codex agent. Actual three profile schematics inspected, along with the revised copper preview. Grouped ESP32 ground-pin labels are crowded; assembly silkscreen labeling is incomplete. These limitations are disclosed in FAMILY.md.
- Public Murata C2 reference sheet initially failed web extraction but was successfully downloaded from the manufacturer's original URL and read locally. Its publication date is March 7, 2016; current ordering/approval status is not inferred from this old sheet.

## Historical payload-permission checkpoint

Automatic review rejected the live smoke command before process creation twice, including after auditing the goal's explicit OpenAI-key/budget approval. The rejection requires permission for this concrete board-family capability contract and language request text to leave the machine. No executable/client/indirect-execution workaround was used. The exact non-secret material is in `evaluation/LIVE_CONTRACT.json` and `evaluation/language.json`; expected answers are **not** sent to the model. The live runner is authored and syntax-checked but has not been executed.

**API requests executed: 0; API spend incurred by this goal: $0; live ledger: not created.** Live accuracy/refusal/clarification results, final acceptance review and the one final PR remain pending. Do not mark the goal complete or publish a negative result as success.

## Approved live evaluation and corrective checkpoint

The user explicitly approved the concrete original contract and sixteen literal requests. No alternative provider or firewall change was used.

- Run `acceptance-language-01`: one request; wrong legacy-envelope decoder rejected the root decision object. Response usage was lost and the $0.05 reserve remains. Historical first-shot failure retained.
- Fixed the provider decoder offline, with tests proving identical outbound request bytes and preserving the old envelope API. Rebuilt the binary and byte-compared the exported contract to the approved file.
- Run `acceptance-language-02`: five requests. `nl-01` explicit recovery passed; `nl-02` and `nl-04` passed; `nl-03` incorrectly refused a negative/excluded requirement; `nl-05` selected the correct profile but failed whitespace-boundary checking. Stopped on the execution failure. Six total requests.
- Fixed only boundary whitespace restoration offline; retained raw decisions and added rejection tests. Continued only eleven never-attempted cases, following complete prior-run hashes.
- Run `acceptance-language-03`: five supported requests passed, four unsupported requests correctly refused, one ambiguous request clarified, and the other incorrectly generated a low-current board. **17 total requests**, **8/10 first selections**, **7/10 first complete boards**, **4/4 refusals**, **1/2 clarifications**. No prompts or expected values were changed.
- All eight accepted supported completions passed native checks and match reviewed examples byte-for-byte; the ninth generated board belongs to the rejected ambiguous request. Conditional accepted latency: median 6.105 s, max 7.315 s; median generation 0.017 s / validation 3.256 s.
- Added an explicit low-current scope gate and strengthened instructions for negation/ambiguous power and the existing 10 A source-capability ceiling. Offline recorded-response replay fixes whitespace/clarification behavior but intentionally leaves the old false refusal unchanged. The revised outgoing contract is **proposed, not live-tested or approved**.
- Preserved all raw runs and ledger. Copied compact original evidence plus representative language selection/validation records, verified 382 hashes, reconciled seventeen indices, and independently recomputed counts and costs. Known estimated usage $0.011711 + unknown reserve $0.05 = $0.061711; no provider invoice inspected.
- Corrected the overly broad no-disabled-checks documentation after inspecting KiCad's default-ignored categories. No raw native report was altered. No project-specific rule weakening was introduced.
- Complete bounded regression after shared provider changes passed in 74.174 s; final post-correction integration passed in 6.820 s with unchanged packages cached. Focused tests, recorded-response regression and vet pass.

Remaining barrier: the original first-attempt targets cannot be repaired by retries, and only three requests remain. Proposed up to two development checks plus one new sixteen-case holdout would require **35 total requests**, keeping **$10 total** and the same ledger/key. Proposed files are `evaluation/LIVE_CONTRACT_REVISED_PROPOSED.json` and `evaluation/language-holdout-proposed.json`. No revised request has executed, the ceiling remains 20, and the goal/final PR are not complete.

## Authorized continuation and final local review

- User approved exactly the proposed 35-request ceiling, unchanged $10 limit, revised contract and fresh prompts. Approved file hashes remain unchanged.
- `recovery-checks-01`, indices 18–19: both explicitly recorded development checks passed. Neither is new first-shot evidence.
- `acceptance-holdout-01`, indices 20–31, source `b5731a56`: all ten fresh supported requests generated correct complete native boards; first unsupported passed; second had a correct overall refusal but inconsistent clause tags caused a local command failure.
- `acceptance-holdout-02`, indices 32–34, source `2bf37fdb`: third unsupported passed; requested 80×60 mm resize wrongly generated the unchanged 120×80 mm board; first ambiguous request correctly asked a question but omitted a sentence in clause transcription and failed locally. The wrong resize is rejected evidence despite passing native checks.
- Added deterministic fixed-size/layer guards and whole-input preservation for null-configuration non-design decisions. No native generator, electrical model, embedded reference or validator changed; no generated output was repaired.
- `acceptance-holdout-03`, index 35, source `0e791b14`: last unseen ambiguous request clarified correctly. Strict cumulative live result remains **10/10 supported, 2/4 unsupported, 1/2 ambiguous (13/16)** across three versions.
- `decision-replay-01`: all sixteen retained real provider decisions passed current decoding offline, with zero calls and zero native generation. This is targeted correction evidence, not replacement live scoring.
- Final bounded full-repository regression passed in **33.535 s**, provider keys removed; current source retained. Native/BOM hashes for all twelve newly generated boards (eleven accepted supported including development, one rejected resize) match the reviewed profiles. Final publication keeps acceptance flags separate from native validity.
- Total **35/35 physical calls**; 34 known responses: 37,443 input / 6,883 output tokens, estimated **$0.026006** plus the original unknown **$0.05** reserve = **$0.076006**. No new provider, security change, fabrication, invoice claim or further request is authorized.
- Unaffected implementation, evidence and self-review are finalized for the single scoped draft PR. Live rejection/clarification acceptance remains open; the goal is not complete. Do not erase scores, reset the ledger or silently broaden the evidence standard.
- Opened [draft PR #13](https://github.com/dshills/KiCadAI/pull/13) against `main`; no merge. Initial CI static check found a missing gofmt blank line in `ledger.go`. Formatting corrected without behavior or budget changes; local full-branch formatting check passes. Remote integration status is separate from the recorded local acceptance results.
