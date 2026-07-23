# KiCadAI Docs

This directory holds the detailed reference material that used to live in the project README. Start with the root [README](../README.md) for setup and the shortest happy path, then use these pages for subsystem details.

## User Workflows

- [Project Status](project-status.md): current capabilities, proven workflows, evidence levels, and explicit limitations.
- [KiCadAI Agent Skill](kicadai-agent-skill.md): prescriptive command and validation contract for AI agents using KiCadAI.
- [CLI Reference](cli-reference.md): command overview, KiCad IPC setup, and direct generation commands.
- [AI Generation](ai-generation.md): behavioral, bounded, and generic provider
  setup, reproducible KiCad-backed lanes, evidence, and failure behavior.
- [Intent Planning And AI Workflow](intent-planning.md): uncertainty-aware
  behavioral compilation, structured intent, rationale reports, semantic
  synthesis, and current AI workflow limits.
- [Circuit Blocks](circuit-blocks.md): reusable block workflows and block-library commands.
- [Placement And Routing](layout-routing.md): placement quality, routing policy, route diagnostics, and retry-related evidence.
- [Validation And Analysis](validation-and-analysis.md): inspection, evaluation,
  writer correctness, transactions, round-trip validation, ERC/DRC checks, and
  independently verifiable clean-checkout promotion bundles.
- [Fabrication Export And Readiness](fabrication.md): readiness gates, BOM/CPL evidence, physical-rule fabrication profiles, provenance, and export commands.

## Libraries And Internals

- [Libraries And Components](libraries-and-components.md): component intelligence, pinmaps, and library resolver details.
- [Development Reference](development.md): examples, Go packages, testing, protobuf maintenance, limitations, troubleshooting, and design direction.
- [KiCad Direct File Writers](kicad-file-writers.md): lower-level writer behavior.
- [Component Intelligence](component-intelligence.md): focused component catalog reference.
- [AI Readiness Matrix](ai-readiness.md): machine-readable AI-agent guidance for component, block, layout, validation, and documentation gaps. This complements the human narrative in circuit block readiness docs.
- [Circuit Block Library](circuit-block-library.md): focused block-library reference.
- [Circuit Block Readiness](circuit-block-readiness.md): readiness review and gaps.
- [Circuit Block Verification](circuit-block-verification.md): verification corpus and workflow evidence.
- [Library Resolver](library-resolver.md): focused symbol/footprint resolver reference.
- [Validation Repair Loop](validation-repair.md): deterministic repair planning and apply behavior.

Completed feature specs and audits remain under `../specs/`. Historical review
snapshots are under `../specs/archive/`; they describe the repository at their
recorded date and are not current capability references.
