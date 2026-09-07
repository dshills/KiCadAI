# Project review — September 6, 2026

Review baseline: `c31f3bc7af70c2787ddb728fb65358a207f71ded` on
`codex/generic-causal-topology-repair`. This is a correctness and security
maintenance review, not capability expansion or a new circuit evaluation.

## Confirmed defects and corrections

| Finding | Correction and regression evidence |
|---|---|
| Release builds could label the current source with a different commit, compile from the caller's directory, or accept malformed version/date overrides. | Authenticate `COMMIT` against `HEAD`, compile the repository root, validate complete metadata strings, and preserve caller-relative output paths. Isolated temporary-repository tests exercise invalid inputs and foreign working directories. |
| Release worker input accepted multiline values and arithmetic-overflow-sized integers. | Validate the whole string and clamp valid positive values to the four actual build targets before arithmetic. |
| V21 could certify contending active outputs, unproven feedback cycles, wrong external-port domains, shorted active supplies, and floating references. | Require existing generic electrical structural checks before certification; retain typed diagnostic issues. Five negative regressions reproduce the former acceptance. No unproven feedback is certified by this API, which has no typed feedback evidence input. |
| V21 state hashes described operations before their acceptance/rejection fields were finalized. | Compute each state hash after finalizing its published operation evidence; reconstruct candidate and selected hashes in regression tests. |
| V21's critical-obligation count could improve while introducing a different critical failure. | Reject newly introduced critical-obligation identities, not just increasing counts. Test both forbidden tradeoffs and genuine improvement. |
| Initial graph and retained-state limits could be bypassed by early successful selection. | Check initial graph bytes and count retained states before selection. Tests use tiny explicit bounds. |
| Repair trusted a nonempty evaluation hash without recomputing it, including on the passed-result fast path; canceled input could also return success. | Recompute the exact evaluation digest before admission and handle cancellation before any success shortcut. Add tamper and cancellation regressions. |
| Release Go 1.23.12 and `x/text` 0.28.0 produced 25 reachable vulnerability advisories. | Move source/CI/release tooling to Go 1.26.8 and `x/text` 0.39.0. Add pinned `make security-check` and a separate CI gate. This raises the current source-build minimum; it does not rewrite v1.0.0 artifacts. |
| Documentation treated the original V21 structural advancement as established validation. | Mark it historical and explicitly require reevaluation of the repaired implementation. Preserve original source, manifests, corpus, selection, and report attribution. |

The Go versions were verified against the [official release index](https://go.dev/dl/).
The Unicode issue is [GO-2026-5970](https://pkg.go.dev/vuln/GO-2026-5970).
`x/sys` is also updated to 0.44.0 for the module-only, Windows-specific
[GO-2026-5024](https://pkg.go.dev/vuln/GO-2026-5024); it was not reachable in
the supported macOS scan, and this update does not add Windows support.
Reachability reported by static analysis is not a demonstration that every
advisory is exploitable through KiCadAI's validated input surface.

## Historical evaluation integrity

Generation zero's 24-case, two-replay report remains unchanged and bound to its
original evaluator manifest
`400b3ab32100b356d3db373c3111ce04ee678785fcc8598d93e20100c532320b`.
Changed original source and seals are archived byte-for-byte under
`specs/generic-causal-topology-repair/generation-zero-source/`. An independent
test pins the historical manifest digests and authenticates every original
manifest entry. Active V21 manifests identify maintenance revision 1 and
explicitly record `not_run_after_maintenance`.

No source key or held-out case was opened. The frozen corpus, selection,
protocol, and V18–V20 reports/seals were not changed. No new corpus outcomes
are claimed. The previous six-case advancement cannot establish material
improvement for the corrected evaluator. A separately admitted, clean-checkout
public reevaluation is needed before making that claim again; it must not
overwrite generation zero.

Older V5–V17 manifests also pin the pre-maintenance `go.mod` and `go.sum`.
Those bytes and changed audit-test sources are preserved in the separate
`specs/closed-loop-open-set-capability-expansion/historical-source/` archive.
Historical audit tests authenticate the old files without modifying old seals.
Only dependency and test-harness files can resolve to that archive; production
source, protocols, manifests, and corpus files cannot. Production manifest
verification remains unchanged, and negative tests require it to reject the
maintained build as an old frozen V8/V10 environment. V8's opt-in evaluation
updater retains live verification; only read-only historical audits use snapshots.

## Review coverage and validation

Review included repository-wide static and test gates; dependency scanning;
release/CI scripts; bounded search and evidence validation; atomic file and
directory publication; and Prism reviews of release, trust-boundary, and V21
code. Focused source inspection followed the repository's Atlas search protocol.
This is not a formal proof that every possible project defect has been found.

The local validation matrix covers `make lint`, `make test-bounded`,
`make coverage-check`, `make race-short`, focused V21 race/replay and historical
seal checks, `make security-check`, `go mod verify`, the two-run external-review
matrix, educational schematic replay, and the 13-case installed-KiCad design
tier under its declared success/refusal expectations. Release verification
includes two byte-identical four-platform builds and host-binary smoke tests.
Packaging uses `ALLOW_DIRTY_RELEASE=1` for pre-commit verification only; those
artifacts must not be published as tagged release evidence.

Final measured outcomes are recorded in the review handoff; this document
does not claim a new clean-checkout corpus evaluation or a tagged release.
No GitHub workflow was manually triggered. Existing CI on the baseline PR was
green before review; the defects above demonstrate why that alone was insufficient.

## Review disposition

Prism's broad reviews included several unverifiable or incorrect findings
(nonexistent file paths, stale model/action version assumptions, and already
checked close errors). Those were checked against repository evidence and not
implemented. Only reproduced defects or supported security findings are
included in this change. The final staged-diff review returned nine findings.
Disposition:

- Four claims that pinned actions, Go, the scanner, and dependency versions do
  not exist were contradicted by official Go release/proxy responses,
  completed local builds and scans, and successful baseline CI using the same
  action commits. No downgrade was made.
- The claimed missing `corpusHash` is a package-level test helper; the cited
  repair file does not call it. Package compilation and tests pass.
- The claimed missing Make cache variables are defined in the same Makefile.
- The JSON concern does not justify changing the wire encoding: verification
  must match the producer's `json.Marshal` exactly. Maps are deterministically
  ordered by that encoder, and the toolchain/dependencies are pinned. Tamper
  and replay regressions pass.
- Caching a tiny critical-obligation map is an optional performance suggestion,
  not a demonstrated error in the bounded search. No speculative cache was added.
- The release-test stub's stale Go version was corrected to 1.26.8.

There are no unresolved valid Prism findings. The configured external review
provider was Gemini through Prism; byte-exact historical snapshots were
excluded from the final diff review and authenticated separately by tests.
