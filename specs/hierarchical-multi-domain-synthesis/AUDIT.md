# Hierarchical Multi-Domain Circuit Synthesis Audit

## Outcome

The milestone is locally complete. KiCadAI now accepts the additive V4
behavioral requirement contract, derives and validates hierarchical system
evidence, closes the selected system through simulation and physical
realization, and reproducibly emits clean KiCad projects for all six frozen
multi-domain systems.

This establishes the bounded capability described by the specification. It
does not claim unrestricted arbitrary-circuit generation.

## Frozen Baseline And Corpus

- The six behavior-only V4 requirements were frozen before production support
  in `internal/architecturesearch/testdata/hierarchical_multi_domain_corpus`.
- `BASELINE_REPORT.json` records that the pre-implementation production path
  rejected V4 and could not emit hierarchy, contracts, resources, physical
  partitions, global backtracking, or end-to-end traceability.
- The independent freeze validator checks manifest membership, SHA-256,
  schema neutrality, minimum obligation/domain counts, environmental corners,
  acceptance gates, and absence of implementation hints.
- A production-source scan found no Go references to any frozen scenario ID.

## Requirement-To-Evidence Audit

| Specification outcome | Implemented evidence |
| --- | --- |
| Additive strict V4 model | Strict decode, normalization, limits, canonical hashes, independent mirror/freeze validation, and preserved V1-V3 tests |
| Derived hierarchy | Canonical system/subsystem/block nodes with stable IDs, ownership, coverage, ordering, and acyclicity validation |
| Interface contracts | Typed electrical, protocol, loading, timing, startup, noise, thermal, protection, and isolation contracts with fail-closed compatibility |
| Shared resources | Globally accounted rails, references, clocks, resets, sequencing, decoupling, connectors, grounding, protection, and thermal resources |
| Deterministic backtracking | Complete-candidate ordering, structured rejection evidence, bounded global retry, and selected-candidate trace |
| Verification closure | Block-scoped and end-to-end analyses across declared operating, tolerance, startup, model, noise, thermal, timing, and fault corners |
| Physical co-design | Derived partitions, locality/separation rules, return-path policy, crossing constraints, high-current/thermal treatment, and generic placement/routing consumption |
| Traceability | Requirement, hierarchy, contract, resource, candidate, transaction, KiCad object, simulation, and promotion links |
| Fail-closed behavior | Mutation, ambiguity, missing-proof, incompatible-contract, insufficient-resource, unsafe, and budget-exhaustion tests |

The implementation also added the generic component, topology, simulation,
placement, routing, writer, and evidence support needed by the held-out
systems. The final routing correction orders explicit strict-DRC repairs so
transition-via clearance is judged against final repaired copper rather than a
stale pre-repair route.

Post-publication CI exposed one additional contract-projection defect:
lowering retained an external power domain's nominal voltage but discarded its
declared minimum and maximum. That caused derived simulation evidence to
invent a narrower nominal ±1% source-node assertion. The generic correction
preserves declared domain limits through lowering and uses them for external
power-input assertions, while retaining the deterministic ±1% fallback for
contracts without limits.

## Exact Installed-KiCad Promotion

The authoritative parent implementation revision was:

`5f96deaf42b1c17bfacdd980793dd96dbed18082`

The audit itself is a later documentation-only commit, so the receipt hash is
not self-referential. On the audit host's current date, 2026-07-25, both
`toolchain/kicad-promotion.lock.json` and the generated toolchain receipt pin
KiCad 10.0.3 on Darwin arm64. The receipt also records promotion-lock SHA-256
`667ed018b06d4b719600853f5d68119c9d94782bb05e27b8e142cd649c565ae2`,
stock symbol and footprint table hashes, and complete installed-library
identities.

The exact command

```text
make HIERARCHICAL_PROMOTION_ROOT=/tmp/kicadai-hierarchical-promotion-5f96deaf \
  PROMOTION_CACHE_DIR=/tmp/kicadai-promotion-cache \
  PROMOTION_SCENARIO_TIMEOUT=20m hierarchical-promotion-bundle
```

produced and verified:

`sha256-3fa6c3faf704f4ac948ea6e92b66ccae280af4219bdfe602e640fafc60526e0d`

The receipt status is `pass`, its manifest has 436 files, and every scenario
passed two independent runs with identical normalized project hashes:

| Scenario | Deterministic project SHA-256 |
| --- | --- |
| Current-limited switched load | `5c9cffde14571335dd0b9db72006cae198e6af0193a3cb8093d41f41f9b262d2` |
| Isolated mixed-voltage gateway | `ce73824bc1c53b50cd3bc9bfe56c22f53adf54530393915980e543fd050ef5f9` |
| Precision acquisition and alarm | `8d22fca5f1ec59a9e146b55cabe47363a1422c5a801ac1612a4660fa233fcf63` |
| Protected Class-AB amplifier | `a9735f5841e8c2a933a79c8e14ac5772e023f2656731633d22f33a72d5c201bd` |
| Regulated sensor/MCU/communications | `fe8e1d2a27bb33be706a7b14ee15e3fa5679b51a0036bec8e9f254a8a908d035` |
| Split-supply precision monitor | `fd939214e214ba0af3398ec65f8edef621ad3b20fc98e612546a8390e81ddeb4` |

Each run required clean ERC, strict DRC, connectivity, route completion,
writer correctness, zero normalized KiCad schematic/PCB round-trip
differences, complete simulation evidence, and a passing promotion report.

## Preserved Regression Evidence

The exact command

```text
make PROMOTION_ROOT=/tmp/kicadai-promotion-5f96deaf \
  PROMOTION_CACHE_DIR=/tmp/kicadai-promotion-cache \
  PROMOTION_SCENARIO_TIMEOUT=20m promotion-bundle
```

produced and verified the 286-file baseline regression bundle:

`sha256-4fd046e7808e3893aa403db8c761583dba9a9cce37d1209e039afe93fb29f2fe`

This is a distinct baseline matrix rather than a relabeling of the six
hierarchical scenarios above. It passed all six scenarios twice: the compact
LED, AP2112K sensor, stock-library RC filter, multi-unit active filter,
protected split-supply Class-AB amplifier, and function-level status driver.

Additional local gates passed:

- `go test -short -timeout 20m ./...`;
- focused `internal/designworkflow`, `internal/closedloopsynthesis`,
  `internal/compositionlowering`, `internal/simmodel`, and CLI regressions;
- unit coverage for domain-limit projection and derived source assertions,
  plus three consecutive isolated-gateway adversarial workflow passes;
- `go vet ./...`;
- `golangci-lint run ./cmd/... ./internal/...` with zero issues;
- `gofmt` and `git diff --check`;
- the existing 12/12 held-out benchmark and installed regression fixtures
  exercised during implementation promotion.

## Review

Prism reviewed the complete implementation and the final routing-order
correction through the configured external Gemini provider. The complete
implementation review had no high or medium findings. The routing correction
review reached zero high findings; two medium notes were adjudicated as
non-actionable because:

- the pre-existing `finalizeEmittedRoutePhysicalClearanceWhenRequired` path
  was already a no-op when strict DRC was not requested; and
- pre-repair clearance findings are intentionally superseded by the same
  validator running once against final copper, after which retained blocking
  issues update routing status.

Focused tests and the exact installed-KiCad isolated-gateway promotion confirm
that adjudication.

The final staged review also covers the CI-discovered domain-limit projection
correction and its regression tests.

## Closeout Boundary

Local implementation, review, regression, and clean-checkout evidence are
complete. Publication remains a separate final gate: this audit commit must be
pushed, the ordinary GitHub Actions suite and installed-KiCad promotion
workflow must pass for the exact pushed revision, and the repository must
remain clean. Those external results are reported at closeout rather than
embedded into a commit that would change the revision being verified.
