# Pickfit v1.0.1 investigation

Date: 2026-11-04. Scope: reproduce and classify the three reported CLI observations; no release, package installation, provider circuit generation or hardware validation.

## Source and limits

The [third-party record](https://github.com/ncchen99/pickfit/blob/20b452d6a7b7c9d57bf74aaa119108d3e0ab3848/data/software/kicadai.json) is an unverified summary, not an authenticated test transcript. Its frozen source SHA-256 is `05b8cb59dda1a8fd48a4f5fd46ae038c76f15edb2138a065611daac33a4116ae`. Run ID: `verify-kicadai-20261101-000504-d83f`; recorded environment: Linux/x86_64, Modal gVisor, image `pickfit-sbx:2649442dfa6d`, install method `binary`, tool version `1.0.1`.

Its installation command downloads the official `kicadai_v1.0.1_linux_amd64` and `SHA256SUMS`, copies the executable into `./bin` and `~/.local/bin`, and invokes `./bin/kicadai version`. The summary therefore records a release-binary test, not a source build. It separately scans current-source dependency metadata for security; that does not identify the executable used for the functional test.

The referenced `runs/verify/verify-kicadai-20261101-000504-d83f/report.json` returns GitHub HTTP 404, and the complete repository tree at the pinned revision contains no run-ID artifacts. `log_path` and `evidence_url` are null. Listed screenshots were not available in that tree. Consequently their contents, exact shell state, native-tool errors and any unlisted commands cannot be independently checked.

Actual reproduction host: macOS/Darwin arm64, not Linux. Docker's client is installed but its configured OrbStack daemon is unavailable; no runtime was started, installed or downloaded. The exact Linux AMD64 release asset was downloaded and checksum-verified, and its Go metadata identifies Linux/AMD64, Go 1.26.8, CGO disabled. It was **not executed**. The native Mac ARM64 asset from the same official release was independently checksum-verified and executed. This verifies portable command behavior locally, not Linux runtime qualification.

Release identity: tag `v1.0.1` resolves to `b961b7a73aa9d19398d2adbaeee89de008e5d390`; manifest Go version `go1.26.8`. Linux binary SHA-256: `df262f84d793adc86399d2556cc52f1e4feff4053c15bd3adb18268c36b1ae85`, matching the report's abbreviated `df262f84...`. Mac ARM64 binary SHA-256: `e794f99a7170107aaf00168a01349c56592c3c864fbe4003d6849024203a9f1d`. Matching a reported prefix does not authenticate the report's actual downloaded bytes. Both binaries and manifest were compared with the official release checksums; checksums are not an independent signature.

Current-source baseline: `be5dc1b19e041d86987efa66e9e08eed9743316e`, built with Go 1.26.10. Its `version` command identifies a development pseudoversion, not the published v1.0.1 artifact. Existing native KiCad CLI: `/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli`, version 10.0.3. No KiCad or system package was installed. Generated projects and output checks used fresh temporary directories; copied check fixtures were used for follow-up native checks.

## Findings

| Claim | Verdict | Evidence and root cause |
| --- | --- | --- |
| Release binary lacks documented `schematic` | **Confirmed for v1.0.1; not current source. Documentation/version mismatch.** | The report quotes `bin/kicadai --output ./out/divider schematic example divider` in a pitfall, but omits that attempt's full transcript/exit code. Local verified release: exit 1, `unknown command "schematic"`, no output files. Baseline current source: exit 0 and a complete divider bundle. The v1.0.1 tag's README/help/dispatch contain `schematic-ir`, not `schematic`; commit `66cb10d6` introduced the latter after the release. The report read main-branch documentation alongside an older binary. |
| `generate-led-demo` ignores output | **Confirmed defect in handling invalid trailing arguments, in release and pre-fix current source.** | The summary records `bin/kicadai generate-led-demo --output ./out/led_demo --overwrite`, exit 0, writing `led_indicator/led_indicator.kicad_sch`; both local baselines reproduce it exactly. Go `flag.FlagSet.Parse` stops at the first positional command. Trailing options become `opts.commandArgs`, and the two direct LED aliases formerly ignored them. `opts.output` remains empty, causing fallback to `./led_indicator`; trailing `--overwrite` is also unparsed. Correct leading options honor the requested output. The fix rejects any trailing argument before generation or filesystem mutation rather than changing global parsing semantics. |
| PCB and ERC/DRC could not be fully evaluated without KiCad | **Inconclusive for the remote Linux environment; output absence is explained by invocation, not a PCB-writer failure.** | The recorded command omits `--with-pcb`, so two files are the documented default at both revisions. Locally, `--output ./out/pcb_demo --with-pcb generate-led-demo` writes all three native files without calling KiCad. The report's `validate board ./led_indicator` fails on a missing PCB locally even with KiCad installed. Native ERC/DRC need an installed CLI; the report says it was absent but supplies no underlying tool log. Local 10.0.3 checks execute successfully as tools and return actual findings (below). This does not reproduce the remote Linux environment or prove a clean PCB. |

The summary also explicitly records `./bin/kicadai version`, `bin/kicadai inspect project ./led_indicator`, `bin/kicadai block list`, and `bin/kicadai validate board ./led_indicator`. It mentions `writer check` and absence of KiCad in prose, but gives no exact writer/native-check invocation. Those omitted commands/flags must not be reconstructed by guessing.

## Reproduction results

Each command ran in its own temporary working directory unless a setup step intentionally reused a generated project. The retained JSON evidence contains exact argv, cwd, setup, stdout/stderr, exit code and file inventory.

```sh
# Both v1.0.1 and current pre-fix source: exit 0, wrong default directory.
kicadai generate-led-demo --output ./out/led_demo --overwrite
# Creates ./led_indicator/{led_indicator.kicad_pro,led_indicator.kicad_sch}.

# Both versions: intended output, no PCB by default.
mkdir -p ./out
kicadai --output ./out/led_demo generate-led-demo
# Creates ./out/led_demo/{led_demo.kicad_pro,led_demo.kicad_sch}.

# Both versions: optional PCB file is written, independently of KiCad availability.
kicadai --output ./out/pcb_demo --with-pcb generate-led-demo
# Also creates ./out/pcb_demo/pcb_demo.kicad_pcb.
```

An initial leading-flag probe omitted `mkdir -p ./out` and failed with `stat out: no such file or directory`. This was a reproduction setup error, not evidence that leading flags are ignored: `WriteProjectDirectory` stages atomically inside the existing parent. The corrected setup passes. An initial ERC probe used the reserved/nonexistent `erc_pass` fixture and returned a missing-file diagnostic; the real follow-ups below supersede that invalid-target probe. Both attempts remain visible in the retained evidence rather than being erased.

With KiCad 10.0.3, both the official native release and pre-fix source produced the same native results:

- Generated legacy LED schematic: ERC exit 1, 8 findings (4 errors, 4 warnings). Generated minimal PCB: DRC exit 0, 4 warnings, **check status `fail`**, not a clean pass. A warning-only exit 0 must not be mistaken for zero findings.
- Copied `erc_fail` fixture: ERC exit 1, 4 error findings. Copied historical `drc_pass` fixture: 4 warnings, likewise not a clean pass.
- The temporary divider schematic produced by current source: ERC exit 0, check status `pass`, zero findings, including when checked by v1.0.1. This demonstrates v1.0.1's native ERC integration, not v1.0.1 `schematic` generation.
- An explicitly nonexistent `--kicad-cli` produces a nonzero exit and `KICAD_CLI_FAILED`; dependency absence is not silently treated as success.

The legacy demo's ERC/DRC limitations are already documented in [check fixtures](../examples/checks/README.md): its schematic is the negative ERC fixture, and the historically named `drc_pass` fixture exercises warning-report ingestion. They are file-writer/parser demos, not fabrication or clean-check acceptance examples. Repairing that legacy design would require a separate design change, not merely a CLI flag fix; this investigation does not weaken native checks or silently repair generated artifacts.

## Changes and tests

- Both `generate-led-demo` and `generate-project` reject trailing flags/positionals before writing. The fixed binary returns exit 1 with a global-flag-placement explanation and an empty output inventory for the report's invocation.
- Ten regression cases cover both aliases, mixed leading/trailing flags, `--output=value`, unexpected positionals, and the especially dangerous leading `--overwrite` with trailing `--output` case. Existing sentinels in both requested and default directories remain unchanged; no default project appears when one did not exist. Existing correctly ordered generation/overwrite/optional-PCB tests pass unchanged.
- README now separates maintained-source commands from published v1.0.1 commands and includes a compatible LED quick start, parent-directory setup, flag ordering and PCB/check limitations. CLI help/reference clarify global flag placement.
- Passed focused regression/help/generation tests; `go test -short ./cmd/kicadai ./internal/kicadfiles/design ./internal/kicadfiles/checks ./internal/buildinfo`; `make lint` (gofmt, vet, full-scope golangci-lint: 0 issues); current CLI build; real temporary release/current CLI reproductions.
- The actual opt-in native test was run with both `KICADAI_RUN_KICAD_CLI=1` **and** `KICADAI_KICAD_FIXTURE_TARGET` pointing at the temporary divider, using KiCad 10.0.3. `TestOptionalKiCadCLIReportFixtureGeneration` passed without skipping. A preliminary run lacking the target skipped that test and is not counted as native evidence.

No paid circuit API requests, project artifact repair, package installation, published release change, push, or external report modification was performed. Prism availability/results and exact final validation commands are recorded with the evidence in `specs/pickfit-investigation-2026-11-04/`.

Prism with Anthropic `claude-sonnet-5-5` was attempted twice; both exited 4 with a provider request-header timeout and produced no review report. Per the user's fallback instruction, an internal Codex self-review checked both dispatch aliases, parser boundary, error envelopes, pre-write guard placement, existing valid invocations, destructive overwrite risk, tests and version-qualified docs. It identified the missing pre-existing-default-directory regression above, now added. No remaining actionable issue was found in the scoped patch. This is not an independent external or human review. See `internal-review.json` and `final-validation.json`; earlier `validation.json` preserves the pre-self-review test snapshot rather than claiming its test-file hash is final.

## Linux follow-up (not executed here)

Use the checksum-verified Linux v1.0.1 binary and an already installed compatible KiCad CLI. Substitute absolute binary paths; this does not install anything:

```sh
KICADAI=/absolute/path/to/kicadai_v1.0.1_linux_amd64
KICAD_CLI=/absolute/path/to/kicad-cli
work=$(mktemp -d)
cd "$work"
"$KICADAI" version
"$KICAD_CLI" version
mkdir -p out
"$KICADAI" --output "$work/out/led_demo" generate-led-demo
"$KICADAI" --output "$work/out/pcb_demo" --with-pcb generate-led-demo
"$KICADAI" inspect project "$work/out/pcb_demo"
"$KICADAI" --kicad-cli "$KICAD_CLI" check erc "$work/out/led_demo/led_demo.kicad_sch"
"$KICADAI" --kicad-cli "$KICAD_CLI" check drc "$work/out/pcb_demo/pcb_demo.kicad_pcb"
```

Run check commands independently and retain their JSON even when they exit nonzero. Inspect status/findings, not only process exit status. The legacy LED is expected to have findings; this sequence evaluates tooling and output placement, not manufacturing readiness. Testing the trailing-argument rejection requires a source build containing this fix, not the unchanged v1.0.1 download.
