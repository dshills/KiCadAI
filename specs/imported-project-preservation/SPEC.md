# Imported Project Preservation Specification

## Purpose

Make KiCadAI safe around user-authored KiCad projects.

The project can already inspect and evaluate imported projects, and it already
blocks broad imported-project mutation when unsupported preserved content is
present. The next step is to turn that defensive posture into explicit,
machine-readable preservation evidence so AI agents can:

1. review imported projects without changing files;
2. understand which content KiCadAI can model, preserve, or must avoid;
3. plan edits without applying them;
4. eventually apply a narrow safe edit set only when preservation and ownership
   gates pass.

The immediate goal is not broad imported editing. The immediate goal is a
reliable preservation boundary that prevents data loss and gives agents
actionable reasons when imported edits are blocked.

## Background

Current foundations:

- `inspect project`, `inspect schematic`, and `inspect pcb` expose unsupported
  and preservation-only counts.
- `evaluate project` and file-level evaluation work read-only.
- `transactions.PlanTransaction` detects existing projects and blocks design
  mutations when unsupported imported content is present.
- `transactions.Apply` contains imported-project apply paths for some operations,
  but planning currently blocks unsafe cases.
- Generated-project provenance and repair apply are separate and already require
  generated ownership evidence.
- Schematic round-trip compatibility exists for generated-output quality, but it
  is not full arbitrary imported schematic preservation.

Current gaps:

- unsupported/preservation-only objects are not rich enough for edit decisions;
- ownership is still coarse: generated vs imported, not object-level;
- unknown schematic and PCB raw nodes are not guaranteed through a read-modify-
  write path for arbitrary files;
- imported local libraries, library tables, project settings, and ordering-
  sensitive sections lack explicit preservation gates;
- imported edit plans do not return a comprehensive preservation report;
- no imported apply mode is allowed until preservation safety is proven.

## Goals

1. Add a first-class imported preservation report.
2. Classify project, schematic, PCB, library-table, local-library, and unknown
   node preservation risk.
3. Add object-level ownership and mutability classifications for imported
   projects.
4. Keep imported apply blocked by default unless the target and operation class
   are explicitly proven safe.
5. Add a dry-run planning workflow for imported edits that explains blocked,
   safe, and unsupported operations.
6. Preserve unknown nodes and ordering-sensitive sections where the readers
   already capture enough raw information.
7. Add a conservative first safe edit class only after preservation gates exist.
8. Expose all results through JSON suitable for AI agents.

## Non-Goals

- Broad arbitrary imported-project mutation.
- Lossless modeling of every KiCad object in this project.
- Semantic reverse engineering of user intent from arbitrary drawings, scripts,
  or comments.
- Merging two independent editing histories.
- Mutating imported hierarchical sheets before hierarchy ownership and sheet
  identity rules are explicit.
- Mutating imported local symbol or footprint library contents.
- Claiming that KiCadAI can preserve data it does not parse, store, or re-emit.

## Definitions

### Imported Project

A KiCad project directory without fresh KiCadAI generated ownership evidence for
all files that a requested operation would mutate.

### Generated Project

A project with a valid `.kicadai/manifest.json`, fresh file hashes, and, where
required, transaction provenance that proves KiCadAI owns the target files.

### Preservation-Only Content

Content the reader can detect and preserve as raw S-expression or raw metadata,
but KiCadAI does not model semantically. Preservation-only content may be safe to
round-trip but unsafe to edit around.

### Unsupported Content

Content KiCadAI cannot safely preserve, cannot classify, or cannot prove stable
through read/write. Unsupported content blocks imported mutation.

### Ownership

Object-level evidence about whether KiCadAI may modify an object:

- `generated_owned`: generated and proven by manifest/provenance.
- `imported_user`: user-authored imported content.
- `preservation_only`: raw-preserved content not safe for semantic edits.
- `unknown`: insufficient evidence.
- `new_operation`: new object created by the pending transaction.

### Mutability

Operation-level policy:

- `read_only`: may inspect/evaluate only.
- `plan_only`: may produce a dry-run plan but cannot apply.
- `safe_add`: may add a new isolated object without modifying user-authored
  content.
- `unsafe`: must block before write.

## Preservation Report

Add an imported preservation report shape that can be embedded in inspect,
evaluate, transaction plan, repair, and future intent outputs.

Suggested package:

```text
internal/preservation
```

Suggested report shape:

```go
type Report struct {
    Target           string            `json:"target"`
    Status           Status            `json:"status"`
    Scope            Scope             `json:"scope"`
    Summary          Summary           `json:"summary"`
    Files            []FileReport      `json:"files,omitempty"`
    Objects          []ObjectReport    `json:"objects,omitempty"`
    OperationReviews []OperationReview `json:"operation_reviews,omitempty"`
    Issues           []reports.Issue   `json:"issues,omitempty"`
}
```

Statuses:

- `clean`: no known preservation blockers for read-only evaluation.
- `warning`: preservation-only content exists but no write is requested.
- `blocked`: requested mutation is unsafe or unsupported.
- `unknown`: insufficient evidence to classify.

Summary fields:

- file count;
- schematic count;
- PCB count;
- local library table count;
- local library file count;
- unsupported object count;
- preservation-only object count;
- imported user object count;
- generated-owned object count;
- safe operation count;
- blocked operation count.

File report fields:

- path;
- kind;
- parser status;
- raw preservation status;
- hash before plan;
- ownership;
- mutability;
- unsupported nodes;
- preservation-only nodes;
- issues.

Object report fields:

- stable path;
- UUID when available;
- reference/net/layer when available;
- file path;
- object kind;
- ownership;
- mutability;
- preservation status;
- reason.

Operation review fields:

- operation index;
- operation ID when available;
- operation kind;
- affected refs/nets/files;
- mutability;
- blocked reason;
- required preservation evidence;
- issues.

## Preservation Classes

### Class A: Read-Only Imported Review

Allowed:

- inspect project/schematic/PCB;
- evaluate project/schematic/PCB;
- pinmap and library resolver checks that do not write;
- KiCad ERC/DRC checks that write only explicit output artifacts outside the
  imported project or into a configured artifact directory.

Required:

- no project file mutation;
- preservation report can warn but must not block read-only evaluation unless the
  reader cannot parse the target.

### Class B: Imported Edit Planning

Allowed:

- transaction validate;
- transaction plan;
- repair plan;
- future intent/edit plan dry run.

Required:

- operation reviews for every mutation;
- no writes;
- any unsupported/preservation-only content that overlaps an operation becomes a
  blocked operation review;
- ambiguous refs, duplicate UUIDs, missing schematic/PCB roots, and unverified
  pinmaps remain blocked;
- output must distinguish target-level blockers from operation-local blockers.

### Class C: First Safe Imported Add

The first imported apply class should be intentionally narrow.

Candidate operation:

- add a new schematic symbol plus optional footprint assignment and no-connect
  markers, where:
  - target root schematic parses cleanly;
  - no unsupported schematic raw nodes are present;
  - no preservation-only schematic nodes overlap the insertion area;
  - generated reference is new and non-ambiguous;
  - symbol library and pin metadata resolve;
  - operation does not modify existing user-authored symbols, wires, sheets,
    labels, properties, local libraries, or PCB content;
  - `write_project` is final;
  - atomic write and post-write readback succeed.

This class may remain plan-only in the first implementation if preservation
evidence is not strong enough.

Explicitly excluded from first safe apply:

- removing imported symbols;
- reconnecting imported symbols;
- editing imported symbol properties;
- moving imported footprints;
- routing imported PCB nets;
- refilling zones;
- changing net classes;
- modifying local libraries or library tables.

### Class D: Future Safe Update

Future imported updates may be considered only after:

- object ownership can be proven;
- dependencies are known;
- raw preservation round-trip coverage is strong;
- operation-specific conflict detection exists.

## Unknown Node Preservation

Schematic and PCB readers must classify unsupported nodes into one of:

- modeled;
- preservation-only;
- unsupported-blocking.

Rules:

- Modeled nodes may be semantically inspected and potentially edited if ownership
  permits.
- Preservation-only nodes must be emitted exactly enough for round-trip when the
  surrounding file is otherwise unchanged.
- Unsupported-blocking nodes prevent imported writes.
- If ordering matters and the writer cannot preserve relative order, the file is
  write-blocked.
- If raw data exists but cannot be re-rendered losslessly, the file is
  unsupported-blocking, not preservation-only.

## Ordering-Sensitive Sections

The preservation report must call out sections whose order or grouping matters:

- schematic `lib_symbols`;
- schematic sheets and sheet instances;
- schematic symbol instances;
- PCB layer stack;
- PCB setup;
- PCB net classes and rules;
- PCB footprints and footprint-local objects;
- local symbol/footprint library tables;
- project settings.

Before imported write support expands, each section must be one of:

- modeled and stable;
- raw-preserved and stable;
- blocked.

## Local Libraries And Library Tables

Imported local libraries are user content.

Rules:

- Inspect and evaluate may read local library tables.
- Planning may report required library references and missing libraries.
- Mutation must not rewrite local `.kicad_sym`, `.pretty`, `.kicad_mod`, or
  library table files unless a later spec defines explicit local-library
  ownership.
- If a requested operation requires adding a local library entry, it is blocked
  until library-table preservation and ownership are implemented.

## Transaction Planning Integration

`transactions.PlanTransaction` should include imported preservation evidence for
existing project targets.

Required behavior:

- Existing project detection remains.
- Planning builds a preservation report before mutation classification.
- Touching design content when target preservation status is `blocked` creates
  `PRESERVATION_CONFLICT`.
- Existing operation-specific blockers remain:
  - unsafe remove;
  - ambiguous ref;
  - missing ref;
  - unverified pinmap;
  - write_project ordering.
- Plan output includes operation review details for each blocked mutation.

## Apply Integration

Imported apply remains blocked by default until plan evidence explicitly allows a
safe operation class.

Apply requirements before any imported write:

1. Recompute target preservation report.
2. Verify file hashes have not changed since plan where plan carries hashes.
3. Verify operation reviews still pass.
4. Acquire project lock.
5. Write through existing atomic file writer paths.
6. Re-read changed files.
7. Verify preservation report did not worsen unexpectedly.
8. Return artifacts and issues with operation IDs.

If any step fails, do not leave a partial imported edit behind. Where multiple
files would be mutated, the implementation must either support transaction-like
rollback or block the operation before write.

## CLI Surface

Minimum CLI additions:

```sh
kicadai inspect project ./user-project
kicadai evaluate project ./user-project
kicadai transaction plan ./user-project ./edit.json
```

Existing commands should include preservation evidence where relevant. A
dedicated command can be added if useful:

```sh
kicadai preservation report ./user-project
```

The dedicated command is optional if inspect/evaluate/plan surfaces are clear.

## JSON And Agent Guidance

Agents must be able to answer:

- Is this project imported or generated?
- Can I inspect/evaluate it read-only?
- Can I plan edits?
- Can I apply edits?
- Which object blocks the edit?
- Is the blocker unsupported content, preservation-only content, ownership, or
  operation policy?
- What is the next safe action?

Every preservation blocker must include:

- severity;
- stable code;
- path;
- message;
- suggestion;
- affected operation ID when applicable.

## Tests

### Unit Tests

- Preservation report status computation.
- File report aggregation.
- Unsupported vs preservation-only classification.
- Object ownership classification.
- Operation review classification.
- Existing transaction plan blockers still fire.
- Imported apply refuses stale or missing preservation evidence.

### Fixture Tests

Add small imported fixtures:

- clean minimal imported project;
- schematic with unsupported raw node;
- PCB with preservation-only raw node;
- local library table fixture;
- duplicate reference fixture;
- safe-add candidate fixture;
- unsafe remove fixture.

### CLI Tests

- `inspect project` includes preservation summary.
- `evaluate project` includes preservation warning for imported read-only target.
- `transaction plan` reports operation reviews.
- unsupported content blocks mutation with `PRESERVATION_CONFLICT`.
- imported apply remains blocked unless a safe apply class is explicitly enabled.

### Optional KiCad Tests

Where `KICADAI_KICAD_CLI` is configured:

- read-only evaluation does not dirty imported project files;
- preservation fixtures round-trip without unexpected loss for covered
  preservation-only nodes.

## Documentation

Update:

- `README.md` imported project status;
- `docs/validation-and-analysis.md` preservation report shape;
- `docs/kicadai-agent-skill.md` imported-project stop conditions;
- `docs/development.md` preservation fixture guidance;
- `specs/ROADMAP.md` current foundation and remaining work.

## Risks

Risk: accidentally mutating user content.

Mitigation: imported apply remains blocked unless preservation gates pass and
the operation class is explicitly safe.

Risk: false confidence from raw preservation.

Mitigation: preservation-only means "do not edit semantically"; unsupported
means "do not write."

Risk: object ownership is inferred from weak evidence.

Mitigation: default to `imported_user` or `unknown`, not `generated_owned`.

Risk: atomicity across multiple files is incomplete.

Mitigation: block multi-file imported writes until rollback or staged commit is
implemented.

## Acceptance Criteria

This project is complete when:

- imported project inspection/evaluation emits preservation status;
- transaction planning emits per-operation preservation reviews;
- existing imported mutation blockers are preserved or made more precise;
- unsupported and preservation-only content are distinguishable in JSON;
- imported apply remains fail-closed unless a narrow safe class is proven;
- test fixtures cover clean, blocked, preservation-only, and unsafe cases;
- docs explain how agents should treat imported projects.
