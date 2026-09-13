# Current results — not final goal acceptance

September 13, 2026. The deterministic lane is working; live-language behavior and the final PR are pending. All numbers below are measured results for this new family, not the historical practical-board corpus.

| Criterion | Evidence-backed result |
|---|---|
| Complete supported configuration projects | **10/10** passed without output repair |
| Meaningful electrical profiles | **3**: 4.7 kΩ/100 kHz, 2.2 kΩ/400 kHz, 10 kΩ/100 kHz |
| Clean native/config/BOM replays | **3/3 byte-identical** |
| End-to-end offline wall time | Median **3.273 s**, maximum **3.564 s** |
| Native validation | All 13 original/replay runs passed electrical contract, reference/BOM, writers, connectivity, KiCad 10.0.3 ERC, strict DRC/parity, round trips and unchanged-native-input hashes |
| Live supported language selections | **Not run**; target ≥9/10 first shot |
| Live unsupported / ambiguous behavior | **Not run**; targets 4/4 and 2/2 |
| Live requests / incurred API spend | **0 / $0**; no live ledger created |
| Review | Implementing-agent code/reference/readability review; no independent hardware reviewer or bench testing |
| Final PR | Not opened; final live evidence/review still required |

[Summary JSON](evidence/offline/summary.json) records the local Apple ARM machine, source/base/binary/runner hashes, every run's generation and validation durations, case wall times and artifact digests. The median includes generation plus installed-KiCad validation and subprocess startup. It does **not** include AI inference: no live request has executed. The less-than-five-minute target is demonstrated for deterministic configuration input only, not yet for end-to-end natural language.

[The manifest](evidence/offline/manifest.json) verifies unchanged copies of the run evidence and [three example projects](../../examples/board-family-v1/README.md). These hashes establish local integrity and provenance, not third-party authentication. Original absolute execution paths in KiCad reports are preserved honestly. The remaining seven original projects and three replay projects are retained in the local source-run directory; their check records and complete artifact hashes are included here.

Repository integration used the complete **bounded** tier (`go test -short -p=1 -timeout 20m ./...`), matching `make test` and `make test-bounded`. It passed before and after review corrections; see the [initial execution receipt](evidence/regression/initial-bounded.json) and [final receipt](evidence/regression/final-bounded.json). Unchanged tests used Go's cache in later runs. Focused new-package tests and `go vet` also passed. The exhaustive release tier and a live-provider test are not implied by these results.

The implementing agent checked actual rendered schematics for all profiles and the revised PCB. [Family limits and review caveats](FAMILY.md) cover crowded ground-pin labels, incomplete assembly silkscreen, electrical assumptions, assembly/firmware obligations and absent hardware-performance proof.

To finish, obtain authorization for the exact [contract/schema](evaluation/LIVE_CONTRACT.json) and [literal requests](evaluation/language.json) to be sent to `api.openai.com`, then run the already-authored live runner under the same goal ledger and existing 20-request/$10 ceiling. Expected answers are not model input. Automatic review twice rejected the command before execution despite broader goal authorization; no alternative route was used.
