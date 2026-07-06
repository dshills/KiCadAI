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

The `amplifier` matrix covers:

- family-level block contracts for input buffer, gain stage, bias network,
  Class AB output pair, output protection, supply decoupling, and load
  connectors;
- verified op-amp drive and stability choices;
- Class A/Class AB output devices;
- Class AB bias networks;
- headphone DC blocking and output protection;
- thermal and high-current layout constraints;
- feedback, decoupling, and stability layout;
- simulation-backed Class AB headphone promotion evidence;
- KiCad-backed amplifier promotion evidence;
- AI-facing amplifier design-limit documentation.

Several amplifier records have now reached `connectivity` for the narrow
low-voltage headphone slice: verified LMV321 op-amp selection for the gain
stage, seeded MMBT3904/MMBT3906 output devices, diode-string Class AB
output-stage realization, single-supply DC-blocking diagnostics, an explicit
`headphone_output_protection` block, and AI-facing design-limit notes. The
protection block covers AC output coupling through a DC-blocking capacitor,
required bleed/reference policy, optional series output resistance, and
connector return/reference diagnostics for headphone-only loads. The optional
protected KiCad-backed fixture is still `expected_fail`: it passes schematic
electrical validation, PCB realization, placement, endpoint binding, project
write, and writer-correctness evidence. It currently stops on structural
validation evidence for generated schematic label/connectivity issues and
unrouted or partially routed PCB nets before real KiCad ERC/DRC checks.
The new simulation evidence layer can emit a SPICE-like Class AB headphone
artifact, normalize runner measurements, write `.kicadai/amplifier-simulation*`
files, and feed a `simulation` promotion gate when configured. Missing
simulator configuration remains a clean skip/not-supported result unless a
fixture explicitly requires simulation evidence.
These are not claims that amplifier generation is fabrication-ready. The
remaining queue is validation/connectivity closeout for the protected amplifier
fixture, routing policy promotion, SOA and thermal evidence, active output fault
protection, speaker/bridge/power-amplifier load safety, analog stability/layout
proof, and KiCad ERC/DRC promotion evidence.

The circuit-block inventory now exposes the full verified-amplifier family as
roadmap entries. Entries that do not have a safe implementation are marked
unsupported with explicit gaps instead of being hidden; this lets AI agents
explain why broad speaker or power-amplifier requests are blocked.

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
