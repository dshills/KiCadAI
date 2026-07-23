# Clean-Checkout KiCad Promotion Specification

Date: 2026-07-23
Status: Accepted for implementation

## 1. Objective

Provide one deterministic, manifest-driven command that starts from a clean
checkout, resolves a pinned KiCad toolchain and matching stock libraries, runs
the supported promotion corpus twice, and emits a verifiable content-addressed
evidence bundle.

The workflow closes the gap between repository tests that prove individual
fixtures and a release artifact that another machine can independently
reproduce and audit.

## 2. Scope

The promotion workflow MUST:

- consume `testdata/external-review-mitigation/matrix.json` as its scenario
  inventory;
- derive commands only from a versioned lane registry and scenario fields;
- discover an already installed locked toolchain or bootstrap an explicitly
  locked distribution;
- reject a toolchain whose version, executable, symbol root, or footprint root
  does not satisfy the lock;
- run every positive scenario twice in isolated output roots;
- require real KiCad ERC, strict DRC, connectivity, route completion, writer
  correctness, and zero normalized round-trip differences;
- compare normalized project files and evidence between runs;
- emit a deterministic top-level manifest, per-file SHA-256 inventory, and
  checksum file;
- record the repository revision, tool and library identities, normalized
  environment, argument-vector commands, request hashes, artifact hashes,
  validation status, and deterministic comparison;
- place the result below a directory named by the top-level manifest digest;
- verify a completed bundle without executing KiCad;
- publish the verified bundle from the configured GitHub Actions promotion
  lane;
- leave ordinary offline CI usable without KiCad; and
- preserve the external-review, USB-C, power/interface, function-level,
  amplifier, ESP32, writer, and round-trip regression evidence.

## 3. Non-Goals

This work does not:

- expand supported circuit families;
- weaken or reinterpret any design promotion gate;
- establish sourcing, thermal, analog-performance, EMC, safety, regulatory, or
  fabrication readiness;
- make network access part of ordinary offline tests;
- accept skipped KiCad checks as passing evidence; or
- create fixture-specific production branches, coordinates, allowlists,
  schemas, block families, or net-name exceptions.

## 4. Versioned Inputs

### 4.1 Toolchain lock

`toolchain/kicad-promotion.lock.json` is the toolchain source of truth. It MUST
contain:

- schema and lock version;
- exact KiCad semantic version;
- supported host platform and architecture records;
- discovery candidates expressed as trusted absolute roots plus executable and
  shared-data paths relative to those roots, never relative to the process
  working directory;
- a bootstrap distribution reference pinned by immutable SHA-256 or OCI
  digest;
- symbol and footprint relative paths;
- accepted `kicad-cli --version` pattern; and
- the lock file's own normalized identity in promotion evidence.

Mutable tags, unchecked downloads, PATH-only version assumptions, and silently
falling back to another KiCad release are forbidden.

Promotion bundles are platform-specific. The manifest records the selected
lock platform and architecture, and content addresses are comparable only when
that toolchain identity matches. Cross-platform semantic equivalence is outside
the v1 byte-reproducibility claim.

### 4.2 Promotion matrix

The existing external-review matrix remains the scenario inventory. Each
positive scenario MUST have:

- a unique ID;
- a supported lane;
- a repository-relative request path;
- declared expected pass status;
- the common creation evidence artifacts;
- internal routing, connectivity, route-completion, writer, round-trip, and
  deterministic-repeat gates; and
- required KiCad ERC, strict DRC, writer, and round-trip gates.

Lane-to-command translation lives in one small, versioned lane registry. The
registry may map `intent`, `circuit-explicit`, `circuit-function`, and `design`
to stable CLI verbs. It MUST NOT inspect scenario IDs, filenames, component
names, circuit families, nets, coordinates, or board dimensions.

## 5. Toolchain Resolution

Resolution order is deterministic:

1. explicit command arguments;
2. lock-defined environment variables;
3. lock-defined host discovery candidates;
4. locked bootstrap, only when explicitly enabled.

For a discovered toolchain, the resolver MUST:

- resolve absolute canonical paths;
- execute `kicad-cli --version` with a bounded timeout;
- require the locked version;
- require readable stock symbol and footprint roots;
- content-hash every regular file in a deterministic library identity
  inventory on every promotion; metadata-only caches must never substitute for
  content verification;
- return structured provenance; and
- avoid modifying the host.

Bootstrap MUST download or pull only the immutable locked distribution, verify
its digest before use, install/extract into a caller-owned cache, then rerun
the same discovery validation. Cache population uses a process lock and an
atomic rename from a unique sibling temporary directory on the same filesystem;
an existing complete verified entry wins. Network access is never implicit in
verification or offline tests.

## 6. Execution Contract

For each scenario, the orchestrator MUST:

1. hash the request bytes;
2. create isolated `run-1` and `run-2` roots;
3. invoke the repository-built `kicadai` binary by argument vector;
4. pass resolved symbol, footprint, and KiCad CLI paths explicitly;
5. require ERC, DRC, KiCad round trip, and strict zero-difference validation;
6. capture stdout, stderr, exit status, command, and normalized environment,
   while enforcing a bounded timeout;
7. require successful structured CLI output;
8. require all matrix artifacts;
9. inspect design-promotion and validation evidence for every required gate;
10. hash normalized project and evidence files; and
11. require equal normalized inventories between runs.

Execution order is matrix order. Environment keys and file inventories are
lexically ordered. The command environment is constructed from a strict list:
`KICAD_CONFIG_HOME`, `KICAD10_SYMBOL_DIR`, `KICAD10_FOOTPRINT_DIR`,
`KICADAI_KICAD_CLI`, `KICADAI_SYMBOLS_ROOT`, and
`KICADAI_FOOTPRINTS_ROOT`. `PATH`, `HOME`, and the inherited process
environment are neither captured nor copied; executable paths are explicit.
Recorded values are normalized to lock, checkout, or output tokens. Secrets,
provider keys, CI tokens, shell state, and unrelated environment variables are
never copied to evidence. No wall-clock timestamp, temporary root, checkout
path, username, hostname, process ID, duration, or random identifier may affect
the normalized comparison or content address.

## 7. Evidence Bundle

The bundle schema is `kicadai.clean-checkout-promotion.v1` and contains:

- `manifest.json`: canonical top-level evidence;
- `manifest.sha256`: SHA-256 and the literal filename `manifest.json`;
- `toolchain.json`: resolved immutable tool and library provenance;
- `commands.json`: normalized command and allowlisted environment records;
- `scenarios/<id>/request.json` or its hash-bound source copy;
- `scenarios/<id>/run-1/**` and `run-2/**` promotion artifacts;
- `scenarios/<id>/comparison.json`; and
- `verification.json`: an optional, non-authoritative verifier receipt generated
  on demand.

The top-level manifest records the lane-registry schema and SHA-256 alongside
the matrix and toolchain-lock identities. It inventories every included
immutable file except its own checksum
and an ephemeral verifier receipt. A consumer MUST rerun verification over the
hashed bundle and MUST NOT trust `verification.json` by itself. CI publication
is authorized by the verifier process exit status over the completed bundle,
not by a cached receipt. The final directory name is
`sha256-<manifest-sha256>`. Verification MUST reject:

- malformed schemas;
- unsafe or duplicate paths;
- every symbolic link in the generated evidence bundle, regardless of target;
- unrecognized hidden or system files;
- missing or extra inventoried files;
- size or hash mismatches;
- a manifest checksum mismatch;
- a content-address directory mismatch;
- non-pass scenario status;
- missing required gates;
- skipped KiCad gates; and
- non-equal deterministic comparisons.

The checksum is an integrity signature, not an identity signature. Public-key
signing may be layered on later without changing the evidence semantics.

## 8. CI Policy

Ordinary `push` and `pull_request` CI continues to run offline quality and
promotion tests without requiring KiCad.

A separate configured promotion lane MUST:

- use the immutable locked toolchain;
- run the single clean-checkout command;
- verify the resulting bundle;
- upload the entire content-addressed directory with
  `actions/upload-artifact`;
- fail if KiCad or libraries are unavailable, mismatched, or any gate skips;
  and
- use an artifact name containing the Git commit.

The lane may be scheduled, manually dispatched, or run on a specifically
labeled runner. Once the repository environment is configured for promotion,
the lane is required and must not silently degrade to offline behavior.

## 9. Acceptance

This goal is complete when:

- focused unit and integration tests cover lock parsing, discovery,
  normalization, duplicate/unsafe paths, deterministic comparison, bundle
  construction, tamper detection, and missing/skipped gates;
- the existing promotion and regression suites pass;
- an installed KiCad run produces a verified two-run bundle;
- a fresh checkout has one documented reproduction command with no hand-set
  library paths;
- CI preserves offline jobs and defines the required installed promotion lane;
- the installed lane publishes a verified content-addressed artifact;
- staged changes for every phase receive Prism review with no unresolved
  material findings; and
- all phase commits are pushed and GitHub Actions succeeds.
