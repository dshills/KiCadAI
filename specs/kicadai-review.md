# KiCadAI — Code & Architecture Review

*Reviewed 2026-07-15 · repo: [github.com/dshills/KiCadAI](https://github.com/dshills/KiCadAI) · reviewed at HEAD `7f39a4c`*

## What it is

KiCadAI is a Go toolkit and CLI for AI-assisted KiCad design: ~265K LOC (171K prod, 94K test), 42 internal packages, 1,294 commits — all in about five weeks, single author, clearly agent-driven development with a spec-first process (127 spec folders, mostly SPEC.md + PLAN.md pairs that map cleanly to real packages). The dependency footprint is remarkably small for the size: protobuf and mangos for KiCad IPC, and that's essentially it — all the EDA logic is first-party. MIT licensed, with KiCad's API protos properly vendored and attributed.

## The domain model

Two entry lanes — structured intent and an LLM circuit-graph lane — converge on one deterministic backend, `designworkflow.Create`, which runs an explicit staged, fail-closed pipeline: planning → component selection → schematic → electrical checks → PCB → placement → routing → writers → correctness → ERC/DRC. Fail-closed is real, not aspirational: strict `DisallowUnknownFields` at every input boundary, blocking issues instead of silent defaults, bounded inputs (max components, max document bytes), and a uniform `{ok, issues[], artifacts[]}` result envelope. The `reports` package as a zero-dependency shared kernel (in-degree 361) is the right foundation, and the dependency graph is fully acyclic — with deliberate adapter packages keeping `placement` and `routing` mutually ignorant. That's taste, not accident.

## Where it's genuinely strong

**The hard parts are real, not scaffolding.** The S-expression layer is a proper typed AST with a validating renderer — correctness by construction, not `fmt.Sprintf`. The router is a legitimate multi-layer A* with admissible heuristics, clearance-correct occupancy (radius inflation, separate via grids, via-span checks), deterministic tie-breaking, and search budgets. The nanomsg REQ/REP handling avoids a state-machine desync that only someone who's been bitten gets right.

**The LLM trust boundary is a highlight.** Provider output is capped, strict-decoded against a pinned schema, treated as untrusted until deterministic and KiCad-backed gates pass, with recorded fixtures checked in for deterministic replay. The whole `aiprovider` package is only ~940 LOC because validation correctly lives downstream.

**Correctness strategy is evidence-based.** kicad-cli round-trip gates, ERC/DRC findings classified as writer-bug vs design-bug, convergence-bounded repair loops with anti-thrash logic, and a 75% coverage floor in the Makefile. Tests are behavioral, not tautological — 0.55 test-to-prod ratio, golden tests, idempotency checks.

**The docs and self-reviews are brutally honest.** "Arbitrary electronics generation is not yet guaranteed"; "pass evidence is not a fabrication-release claim." The archived CODE_REVIEW_07_02 doc that found reverse-biased LED subcircuits and round-trip silkscreen loss is a real engineering artifact.

## Where I'd push back

**The enum drift is the highest-value refactor.** `AcceptanceLevel` is defined four times, `NetRole` four times, `ComponentRole` twice — and the *string spellings* have diverged across contexts (`"erc_clean"` vs `"erc-drc"` vs `"erc_drc"`), bridged by hand-written `case` maps where a typo mis-maps silently instead of failing to compile. A bounded-context defense exists, but the execution undercuts it. This is the finding to act on first — promote these into a shared kernel next to `reports`.

**`designworkflow` is a god package.** 22K LOC, 50 files, importing 22 of the 42 internal packages, mixing orchestration with a 48K-line interblock routing engine, 42K of autonomous correction, and schematic layout inference. It's the biggest maintainability risk. Relatedly, `cmd/kicadai/main.go` is a 5,710-line hand-rolled dispatcher with ~80 global flags that must precede subcommands.

**The router and placer are greedy one-pass.** Real A*, but net-by-net with no rip-up-and-reroute — completeness is order-sensitive. Worse, `Strategy.RipupRetryLimit` is declared but never consumed anywhere: a dead field implying capability the router doesn't have. The placer computes HPWL but never optimizes it, and dual legacy/new scoring paths coexist. The docs are upfront that this isn't a general autorouter, but the dead field isn't.

**The intent planner is the honest ceiling on the "AI" claim.** `intentplanner/map.go` is ~2,000 lines of hardcoded topology mappers (`mapClassABHeadphoneAmplifier`, `mapUSBPowerProtection`) — a deterministic template expander that scales linearly with each supported circuit, not generically. The docs concede this; it's still the gap between the name and the capability.

**Process gaps.** No CI at all for a 1,294-commit project — every gate is a local Makefile target. The `.golangci.yml` enables only govet; the project's own self-review found 82 issues a full lint run surfaces that the committed config never will. Only ~16% of `Errorf` calls use `%w` despite 86 `errors.Is/As` sites. And `data/components` isn't `go:embed`ed — the installed binary only finds its catalog from the repo root, a real deployment wart for a tool whose curated catalog is core domain data.

## Overall take

As software engineering, this is high-quality work — noticeably above what five weeks of agent-driven development usually produces, strong on process discipline (specs, coverage floors, self-reviews). The fail-closed pipeline, the typed S-expr AST, and the LLM trust boundary are the kind of decisions that pay compound interest.

The substantive risk isn't the code, it's the domain: the project's own review found real electronics bugs (reverse-biased LEDs, VCC-to-AREF shorts, round-trip data loss), and electronics correctness matures slower than Go correctness.

Working backwards from "arbitrary designs generate reliably," the priority order:

1. Unify the drifted enums into a shared kernel.
2. Decompose `designworkflow` (orchestration vs routing engine vs correction policy vs layout inference).
3. Add CI with the full lint config already known to surface issues.
4. `go:embed` the component catalog.
5. Implement rip-up or delete `RipupRetryLimit`.

After that, the generic-circuit lane is the long pole — and it's a domain-knowledge pole, not an architecture one.

---

### Appendix: quick facts

| | |
|---|---|
| Language / module | Go 1.23, module `kicadai` |
| Size | ~265K LOC total; ~171K prod / ~94K test (0.55 ratio) |
| Packages | 42 under `internal/`, acyclic dependency graph |
| History | 1,294 commits, 2026-06-08 → 2026-07-15, single author |
| Direct deps | `google.golang.org/protobuf`, `go.nanomsg.org/mangos/v3` |
| License | MIT (vendored KiCad protos retain upstream licenses) |
| Tests | 335 test files, ~3,200 test/bench/fuzz funcs, 75% coverage floor, golden + recorded-fixture replay |
| CI | None (`.github/` absent); Makefile gates only |
| Known hotspots | `designworkflow` (22K LOC, 22 imports), `cmd/kicadai/main.go` (5,710 lines), `intentplanner/map.go` (~2,000 lines hardcoded topologies) |
