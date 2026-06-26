# Component Lifecycle And Availability Intelligence Specification

Date: 2026-06-26

## Summary

KiCadAI's component catalog already records manufacturer, MPN, lifecycle, ratings,
confidence, pinmap evidence, and workflow selection output for seed parts. The
next gap is procurement-facing evidence: whether a selected concrete part is
still acceptable for generated designs, whether lifecycle status should block or
warn, and whether local availability snapshots can be used without turning
default tests into live distributor queries.

This project adds a deterministic lifecycle and availability evidence layer for
component selection. It does not call live distributor APIs. It defines local,
reviewable source snapshots and selection gates that can later be backed by
trusted provider imports.

## Roadmap Context

This implements the next Priority 1 roadmap item after verified regulator
family expansion:

- "Add availability/lifecycle source integration when a trusted local or remote
  source is chosen."

The project keeps the current conservative posture: AI-generated projects should
not imply procurement or fabrication readiness from stale, missing, or
untrusted part evidence.

## Problem Statement

The current catalog can say a part is `active`, `mature`, or similar, but that
field is manually curated and not tied to structured source evidence. Component
selection also lacks a procurement policy layer that can answer:

- Is lifecycle evidence present, sourced, and fresh enough for the requested
  acceptance level?
- Is this part marked obsolete, not recommended for new designs, end-of-life, or
  unknown?
- Does a local availability snapshot show stock, lead time, or source confidence?
- Should missing availability evidence warn, block, or be ignored for this
  request?
- Can generated workflow, rationale, BOM, and fabrication previews explain the
  sourcing status without claiming real-time availability?

Without that layer, AI agents can choose electrically valid parts that are poor
production choices, or overstate fabrication readiness.

## Goals

- Add a local lifecycle/availability evidence schema that can be committed or
  supplied through a user-selected source directory.
- Validate lifecycle evidence deterministically with clear issue paths.
- Merge lifecycle/availability evidence with catalog records during selection.
- Add selection policy that can block, warn, or ignore lifecycle and availability
  gaps by acceptance level.
- Surface lifecycle and availability evidence in:
  - `component select`;
  - `component coverage`;
  - design workflow `component_selection` stage summaries;
  - intent rationale/component evidence;
  - fabrication/BOM readiness output where applicable.
- Keep default tests hermetic: no network, no live provider dependency, no
  hidden credentials.
- Provide fixtures for active, mature, NRND, obsolete, unknown, stale, and
  unavailable cases.

## Non-Goals

- Do not call live distributor APIs.
- Do not scrape distributor websites.
- Do not guarantee pricing, stock, or manufacturer acceptance.
- Do not build a real procurement system or supplier ranking engine.
- Do not choose alternates automatically when a selected part is unavailable.
  Alternate search may be planned later once equivalence rules exist.
- Do not make fabrication readiness depend on real-time stock unless a future
  explicitly trusted source provider is implemented.

## Terminology

- **Catalog lifecycle**: the lifecycle string already stored on component or
  package records.
- **Lifecycle evidence**: structured source-backed status, source name, source
  date, and optional notes for a manufacturer/MPN.
- **Availability evidence**: structured source-backed stock, lead time, source
  date, confidence, and optional supplier identifiers.
- **Procurement policy**: request/workflow policy that decides whether missing,
  stale, or unfavorable evidence is blocking.
- **Freshness**: deterministic age calculation against the current run date or a
  test-injected clock.

## Evidence Model

### Source File

Add support for local JSON files under a directory such as:

```text
data/component-sources/
```

The first source format should be provider-neutral:

```json
{
  "schema": "kicadai.component.source.v1",
  "source_id": "curated_seed_procurement",
  "generated_at": "2026-06-26",
  "records": [
    {
      "manufacturer": "Diodes Incorporated",
      "mpn": "AP2112K-3.3",
      "lifecycle": {
        "status": "active",
        "source": "curated",
        "source_date": "2026-06-26",
        "confidence": "curated"
      },
      "availability": {
        "status": "unknown",
        "source": "curated",
        "source_date": "2026-06-26",
        "confidence": "not_checked"
      },
      "notes": [
        "No live availability is implied by this curated record."
      ]
    }
  ]
}
```

### Lifecycle Status

Supported lifecycle statuses:

- `active`
- `mature`
- `nrnd`
- `eol`
- `obsolete`
- `unknown`

Status policy:

- `active` and `mature` are acceptable when source freshness and confidence pass
  policy.
- `nrnd` is allowed for draft/structural, warning for connectivity, blocking for
  fabrication-candidate unless explicitly allowed.
- `eol` and `obsolete` block connectivity and stronger acceptance by default.
- `unknown` warns for connectivity and blocks fabrication-candidate unless
  policy allows unknown lifecycle.

### Availability Status

Supported availability statuses:

- `in_stock`
- `limited`
- `backorder`
- `unavailable`
- `unknown`
- `not_checked`

Availability is advisory for draft, structural, connectivity, and ERC/DRC by
default. For fabrication-candidate it should be warning by default, with an
option to make it blocking. `unavailable` should block only when procurement
policy requires availability.

### Confidence

Supported source confidence values:

- `curated`
- `provider_snapshot`
- `manual_review`
- `not_checked`
- `unknown`

`curated`, `provider_snapshot`, and `manual_review` are trusted by default.
`not_checked` and `unknown` are visible but cannot satisfy required evidence.

### Freshness

Each source entry should include `source_date` in `YYYY-MM-DD` form. Dates are
parsed as UTC calendar dates. Freshness comparisons use an injected run date in
tests and UTC for normal runs, so behavior does not depend on the machine's
local timezone. Future-dated source records are treated as fresh but should
emit validation or review diagnostics if the date is implausibly ahead of the
run date. Policy defines max age in days:

- default lifecycle max age: 730 days;
- default availability max age: 30 days;
- fabrication-candidate recommended availability max age: 14 days.

Stale lifecycle evidence should warn for connectivity and block
fabrication-candidate. Stale availability evidence should warn unless policy
requires fresh availability.

## Catalog Integration

The source evidence layer should join records by normalized:

- manufacturer;
- MPN.

Initial normalization is intentionally conservative for manufacturers and more
tolerant for part numbers. Manufacturer names are trimmed, lowercased, and
compared after collapsing repeated internal whitespace; punctuation and suffix
aliases such as `Inc`/`Incorporated` are not removed until an explicit alias
table exists. MPNs are trimmed, uppercased, and compared after stripping
non-alphanumeric characters so common source variations such as
`AP2112K-3.3` and `AP2112K 3.3` join safely. Duplicate detection uses the same
normalized manufacturer/MPN key used for lookup.

If the catalog record lacks manufacturer or MPN, lifecycle/availability evidence
cannot attach. That should be:

- no-op for draft/structural;
- warning for connectivity if a concrete part is expected;
- blocking for fabrication-candidate manufacturer-profile flows that require
  exact part identity.

Evidence should not mutate the loaded catalog records in place. Prefer an
enriched selection result or sidecar evidence map so base catalog data remains
deterministic and auditable.

## Procurement Policy

Add a request-level or selection-level policy shape:

```json
{
  "procurement_policy": {
    "source_dir": "data/component-sources",
    "require_lifecycle": true,
    "require_availability": false,
    "allow_lifecycle": ["active", "mature"],
    "warn_lifecycle": ["nrnd", "unknown"],
    "block_lifecycle": ["eol", "obsolete"],
    "max_lifecycle_age_days": 730,
    "max_availability_age_days": 30
  }
}
```

Initial scope can put this on component selection and design workflow requests.
Intent planning can derive defaults from acceptance level later in the same
project.

Default policy:

- draft/structural: do not require evidence; warn on explicit obsolete/EOL if
  evidence is loaded.
- connectivity/ERC/DRC: require catalog lifecycle field for concrete active
  parts; warn on missing source evidence; block explicit EOL/obsolete.
- fabrication-candidate: require sourced lifecycle evidence; block stale,
  unknown, NRND, EOL, and obsolete unless policy explicitly allows them; warn on
  missing availability unless `require_availability` is true.

## Selection Behavior

Component selection should include lifecycle/availability in candidate issues
and selected evidence:

- selected candidate includes normalized lifecycle status, availability status,
  source IDs, source dates, freshness status, and policy outcome.
- procurement outcome values are initially `snapshot` for attached local
  evidence and `blocked` when required lifecycle or availability policy fails.
  Empty outcome means no source evidence was attached.
- rejected candidates include lifecycle/procurement reason codes when they fail.
- sorting should not prefer a sourcing status over electrical correctness.
  Source status gates candidates after required electrical/package/confidence
  constraints and before final tie-break.
- deterministic tie-break remains component ID.

New issue codes should be stable and specific, for example:

- `COMPONENT_LIFECYCLE_BLOCKED`
- `COMPONENT_LIFECYCLE_STALE`
- `COMPONENT_AVAILABILITY_BLOCKED`
- `COMPONENT_AVAILABILITY_STALE`
- `COMPONENT_SOURCE_MISSING`
- `COMPONENT_SOURCE_INVALID`

## CLI Behavior

### Component Commands

Extend component commands with source options:

```sh
kicadai --json --source-dir data/component-sources component select --request request.json
kicadai --json --source-dir data/component-sources component coverage
kicadai --json --source-dir data/component-sources component validate
```

The exact flag name can reuse existing CLI patterns if another name is more
consistent. It must be explicit; loading source evidence should not silently use
network or developer-local files.

`component validate` should validate both catalog and source evidence when a
source dir is provided.

`component coverage` should report:

- total concrete records with manufacturer/MPN;
- records with lifecycle source evidence;
- records with availability source evidence;
- stale lifecycle count;
- stale availability count;
- blocked lifecycle count;
- unknown/not_checked availability count.

### Design Workflow

`design create` should accept procurement policy and source directory options
through request JSON and CLI flags where consistent with the existing component
policy flow.

The `component_selection` stage should include summary fields:

- `lifecycle_evidence_count`;
- `availability_evidence_count`;
- `procurement_warnings`;
- `procurement_blockers`;
- selected component evidence per role.

### Rationale And Fabrication Output

Rationale output should turn selected procurement evidence into reader-facing
evidence records.

BOM/fabrication preview should include lifecycle/availability evidence when
available, but must label it as source snapshot evidence, not real-time
availability.

## Validation Rules

Source validation must reject:

- missing or invalid schema;
- missing `source_id`;
- duplicate normalized `(manufacturer, mpn)` records inside a source file or
  across files in the same source directory unless an explicit precedence rule
  is implemented. The initial implementation should reject cross-file
  duplicates instead of silently shadowing by sorted file order;
- invalid lifecycle status;
- invalid availability status;
- invalid confidence;
- malformed source dates;
- lifecycle/availability objects missing source dates when they claim checked
  evidence;
- untrimmed manufacturer/MPN/source strings.

Source loading must be deterministic:

- sorted file traversal;
- stable duplicate detection;
- stable issue ordering;
- no network.

## Testing Requirements

Add tests for:

- valid source fixture loading;
- invalid schema and invalid status values;
- duplicate source records;
- source join by normalized manufacturer/MPN;
- selected AP2112K/source evidence surfaced in component selection;
- obsolete/EOL candidates blocking;
- NRND/unknown behavior by acceptance level;
- stale lifecycle behavior using a test clock;
- stale availability behavior;
- coverage report counts;
- CLI JSON golden output for select/coverage/validate with a source dir;
- design workflow component-selection evidence;
- fabrication/BOM source snapshot propagation if touched.

## Documentation Requirements

Update:

- `README.md`;
- `docs/component-intelligence.md`;
- `docs/fabrication.md` if BOM/fab output changes;
- `docs/kicadai-agent-skill.md`;
- `specs/ROADMAP.md`.

Documentation must explicitly state that local source evidence is not live
availability and does not guarantee procurement.

## Risks

- Overstating availability can mislead users. Mitigation: require explicit
  source files and label evidence as snapshot/local.
- Lifecycle names vary by vendor. Mitigation: normalize into a small internal
  status enum and keep raw source notes as optional metadata.
- Blocking fabrication-candidate too aggressively may slow iteration.
  Mitigation: policy can allow NRND/unknown only with explicit request intent.
- Live API integration later could introduce secrets/network instability.
  Mitigation: define importer boundaries later; keep default runtime local.

## Acceptance Criteria

- Source evidence can be loaded, validated, joined, and reported
  deterministically.
- Component selection blocks explicit obsolete/EOL concrete parts for
  connectivity and stronger acceptance.
- Fabrication-candidate selection can require fresh sourced lifecycle evidence.
- Design workflow and CLI outputs explain lifecycle/availability outcomes.
- Default tests remain hermetic and pass without network access.
