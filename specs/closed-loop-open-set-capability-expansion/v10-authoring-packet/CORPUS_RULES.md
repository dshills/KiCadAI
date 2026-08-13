# V10 Fresh Behavior-Only Corpus Rules

V10 inherits the complete V9 behavior-only requirement contract and adds one
mandatory preparation gate: the complete assignment metadata must pass the
production assignment-feasibility preflight before packets are written or
authors are dispatched.

The corpus contains exactly 48 fresh requirements: 24 discovery and 24 held-
out, authored by six isolated contexts with four cases per partition each.
No V1–V9 requirement text, quarantine content, synthesis result, diagnostic,
outcome, frontier, ranking, selection, or implementation information may enter
an author context.

Assignments fix path, partition, reporting domain, circuit role, safety impact,
primary static/dynamic class, required analysis, output multiplicity, and off-
nominal obligation. Before authoring they must prove exact totals, unique
partition/domain/circuit-role triples, per-author diversity, exact dimension
and safety balance, and complete high-safety domain and circuit-role coverage
in both partitions.

Requirements may specify only externally observable ports, domains, operating
cases, events, bounded assertions, environmental and board limits, safety
impact, and all 14 mandatory acceptance gates. They may not prescribe parts,
values as implementation choices, topology, circuit-family names, symbols,
footprints, packages, placement, coordinates, routes, layers, templates,
fixture identities, expected outcomes, or KiCadAI internals.

All IDs and references are local, canonical, unique, and resolving. Bounds are
finite, ordered, dimensionally valid, and physically coherent. Ground-
referenced signals have explicit bounded external returns. Every assertion is
externally testable and coherent with its operating case, events, observation,
and output.

Every author supplies at least two static-primary and two dynamic-primary
requirements, meaningful multi-output behavior in both partitions, at least
four distinct canonical analyses, and at least two bounded off-nominal/event
cases. The complete corpus covers static, transient, frequency, distortion or
noise, stability, thermal/electrothermal, startup/shutdown, protection/fault,
power/efficiency, digital timing/threshold, sensing accuracy, and interaction
behavior. Normalized semantic signatures are globally unique.

All 14 gates are mandatory: contract validity, deterministic planning,
topology/electrical validity, component/model availability,
simulation/assertion evidence, safety evidence, schematic export, PCB export,
clean ERC, strict DRC, exact connectivity, route completion, writer
correctness, and zero round-trip diffs. Omission or bypass invalidates the
requirement.

Publication accepts only complete validated bundles. Discovery is public;
held-out records are independently encrypted. Source, baseline, and final keys
are distinct external 32-byte 0600 files under
`~/.config/kicadai/closed-loop-open-set/v10/`. Keys and plaintext never enter
the repository, logs, arguments, environment, implementation context, or Prism.
