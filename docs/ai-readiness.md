# AI Readiness Matrix

The AI readiness matrix tracks the remaining verified-design knowledge needed
before an AI agent can generate broader schematics and PCBs without constant
human review.

The matrix is intentionally machine-readable. Records live under:

```text
data/ai-readiness/matrix/*.json
data/ai-readiness/requirements/*.json
```

The Go validator lives in `internal/aireadiness`.

## Record Shape

Each matrix record describes one gap or evidence target:

- `id`: stable `domain.category.slug` identifier. Each ID segment
  (`domain`, `category`, and `slug`) must be dot-free; use underscores for
  word separation within a segment. The validator enforces that the first two
  ID segments match the explicit `domain` and `category` fields.
- `category`: `component`, `block`, `layout`, `validation`, or
  `documentation`.
- `domain`: design domain, such as `amplifier`.
- `readiness`: enum validated by `internal/aireadiness`.
- `blocker`: why the item is not ready.
- `evidence_needed`: concrete evidence required before promotion.
- `next_task`: enum validated by `internal/aireadiness`.
- `evidence`: required when a record is marked `verified`.
- `parallel_group`: optional workstream owner for parallel execution planning.
  Missing values and explicit `unassigned` values are counted as `unassigned`
  in coverage summaries.
- `depends_on`: optional sorted list of readiness record IDs that must exist and
  must be `verified` before this record may be marked `verified`.

Verified records must carry supporting evidence. Evidence that references a
checked-in artifact must include either a semantic hash or git object ID.
Evidence may also include documented source references when the evidence kind
is not a generated artifact.

`internal/aireadiness` is the source of truth for enum validation. The docs
list current values for convenience.

Current `readiness` values:

- `missing`
- `draft`
- `connectivity`
- `candidate`
- `verified`

Current `next_task` values:

- `add_component`
- `add_block`
- `verify_pinmap`
- `verify_layout`
- `capture_kicad_evidence`
- `write_docs`

Current `parallel_group` values:

- `unassigned`
- `fixture_promotion`
- `catalog_block_expansion`
- `engine_hardening`
- `intent_ai_ux`
- `documentation`

`depends_on` references must use stable record IDs, must be sorted
alphabetically by full record ID string, must not reference the current record,
and must form a directed acyclic graph across the fully loaded matrix.
Cross-group dependencies are allowed, but they mean a workstream is not fully
independent from the referenced group.

Semantic hashes are intended to be hashes over canonicalized, non-volatile
representations of generated artifacts. Until a dedicated hash command exists,
prefer git object IDs for checked-in artifacts or keep the record below
`verified`.

## Requirement Shape

Requirement files under `data/ai-readiness/requirements/*.json` describe the
minimum matrix coverage expected for a domain.

- `version`: requirements schema version.
- `domain`: domain the requirements apply to.
- `required_categories`: matrix categories that must have at least one record
  for the domain.
- `required_record_ids`: specific record IDs that must exist for the domain.

## Amplifier Coverage

The initial `amplifier` matrix covers:

- verified op-amp drive and stability choices;
- Class A/Class AB output devices;
- Class AB bias networks;
- headphone DC blocking and output protection;
- thermal and high-current layout constraints;
- feedback, decoupling, and stability layout;
- KiCad-backed amplifier promotion evidence;
- AI-facing amplifier design-limit documentation.

These records are mostly `missing` or `draft`. They are not claims that
amplifier generation is fabrication-ready. They are a queue of work needed to
make that claim defensible.

## Validation

Run:

```sh
go test ./internal/aireadiness
```

The tests validate:

- schema and enum values;
- stable ID format;
- ID/domain/category consistency;
- duplicate IDs;
- valid parallel groups;
- dependency references, ordering, duplicate entries, cycles, and verified
  dependency readiness;
- verified records requiring evidence;
- amplifier requirement coverage;
- domain coverage summaries, including `by_parallel_group`.

## Contributor Flow

When adding a component, block, or layout capability for AI generation:

1. Add or update a matrix record.
2. Keep non-verified records explicit about blockers and required evidence.
3. Add evidence only when it is backed by checked-in artifacts, semantic hashes,
   git object IDs, or documented source references.
4. Promote readiness in small steps: `missing` -> `draft` -> `connectivity` ->
   `candidate` -> `verified`.
5. Keep `requirements/*.json` updated when a domain has mandatory coverage.
6. Assign `parallel_group` when the record belongs to a known workstream, and
   use `depends_on` for real prerequisites instead of burying ordering in prose.
