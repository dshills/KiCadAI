# KiCadAI Docs

This directory holds the detailed reference material that used to live in the project README. Start with the root [README](../README.md) for setup and the shortest happy path, then use these pages for subsystem details.

## User Workflows

- [Featured Public Demo](../examples/public-demo/protected-programmable-current-output/README.md): behavior-only protected current-output synthesis, native KiCad artifacts, compact evidence, refusal replay, and video storyboard.
- [Project Status](project-status.md): current capabilities, proven workflows, evidence levels, and explicit limitations.
- [v1 Support Contract](../SUPPORT.md): release platforms, KiCad compatibility,
  stable interfaces, and explicit unsupported boundaries.
- [Changelog](../CHANGELOG.md) and [Security](../SECURITY.md): release changes,
  vulnerability reporting, and electrical-safety boundaries.
- [Educational Schematic Examples](../examples/educational/README.md): five
  generated teaching circuits with conventional topology-aware layout and
  reproducible KiCad projects.
- [Simulation Admission Evidence](../examples/simulation-admission/README.md):
  authenticated admitted and typed fail-closed model/solver decisions.
- [KiCadAI Agent Skill](kicadai-agent-skill.md): prescriptive command and validation contract for AI agents using KiCadAI.
- [CLI Reference](cli-reference.md): command overview, KiCad IPC setup, and direct generation commands.
- [AI Generation](ai-generation.md): behavioral, bounded, and generic provider
  setup, reproducible KiCad-backed lanes, evidence, and failure behavior.
- [Capability-Aware Generation Gate](capability-gating.md): supported,
  experimental, and unsupported decisions; evidence links; opt-in behavior;
  monotonic reassessment; deterministic evidence-driven expansion proposals;
  and hash-bound promotion rules.
- [Intent Planning And AI Workflow](intent-planning.md): uncertainty-aware
  behavioral compilation, structured intent, rationale reports, semantic
  synthesis, and current AI workflow limits.
- [Circuit Blocks](circuit-blocks.md): reusable block workflows and block-library commands.
- [Placement And Routing](layout-routing.md): placement quality, routing policy, route diagnostics, and retry-related evidence.
- [Validation And Analysis](validation-and-analysis.md): inspection, evaluation,
  writer correctness, transactions, round-trip validation, ERC/DRC checks, and
  independently verifiable clean-checkout promotion bundles.
- [Simulation Admission](simulation-admission.md): deterministic analysis
  planning, trusted model sources, solver availability, provenance, and typed
  refusal behavior.
- [Fabrication Export And Readiness](fabrication.md): readiness gates, BOM/CPL evidence, physical-rule fabrication profiles, provenance, and export commands.

## Libraries And Internals

- [Detailed Capability Record](capability-record.md): the preserved chronological implementation and evidence inventory formerly shown at the top of the root README.
- [Libraries And Components](libraries-and-components.md): component intelligence, pinmaps, and library resolver details.
- [Development Reference](development.md): examples, Go packages, testing, protobuf maintenance, limitations, troubleshooting, and design direction.
- [Performance and Test Tiers](performance.md): reproducible benchmarks,
  profiling findings, test-cost tiers, and deterministic concurrency controls.
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
