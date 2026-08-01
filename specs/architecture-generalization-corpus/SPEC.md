# Out-of-Distribution Architecture Generalization Specification

## Objective

Measure and expand behavior-driven circuit architecture synthesis against a
second independently authored corpus that is frozen before production search
changes. The corpus covers six new analog and power behaviors plus four
adversarial safety envelopes.

Production inference must remain independent of case identity. No requirement
file may name a component, primitive family, implementation structure,
internal connection, value, geometry, route, provider, simulation model, or
repair action.

## Frozen Design Corpus

The six design requirements are:

1. line/load regulated low-voltage power output;
2. dual-threshold voltage-window indication;
3. low-current-to-voltage conversion with bandwidth and noise limits;
4. low-level full-wave transfer with bounded error;
5. frequency-selective passband transfer with rejected lower and upper bands;
6. control-programmed constant-current load drive with protection evidence.

Each requirement declares only external domains, ports, operating conditions,
events, measurable behavioral limits, physical bounds, and the complete
acceptance profile.

## Adversarial Corpus

Four additional behavior-only requirements demand an unstable dynamic
envelope, unsafe dissipation, inadequate safe-operating-area margin, and an
inconsistent standing-current envelope. They are acceptance cases only when
the system rejects them deterministically with stable actionable evidence and
does not emit an executable project.

## Generic Expansion Rule

After the manifest and untouched baseline are frozen, implementation changes
may add only reusable graph obligations, primitive relationships, analytic
value derivations, trusted measurement contracts, diagnosis-to-repair
operators, and topology-derived physical rules. Case IDs, filenames, exact
fixture constants, coordinates, allowlists, named block families, and hidden
selection keys are prohibited from production code.

## Acceptance

- At least five of six design cases pass deterministic architecture search,
  trusted simulation, readable physical lowering, connectivity, writer
  correctness, installed-KiCad ERC and strict DRC, zero normalized round-trip
  differences, and identical two-run replay.
- Every passing design evaluates multiple distinct topology hashes and records
  equation/value provenance plus a deterministic ranked winner.
- All four adversarial cases fail closed with stable diagnostic and evidence
  hashes and produce no executable physical promotion.
- The frozen Class A, Class AB, notch, first open-topology held-out corpus, and
  all previously promoted fixtures remain green under their authoritative
  local commands.

## Evidence

The checked-in evidence set consists of the corpus manifest and checksum,
untouched baseline report, gap audit, promotion matrix, installed-KiCad
two-run artifacts, preservation results, and completion audit.

The completed evidence is recorded in `PROMOTION_MATRIX.md`,
`PRESERVATION_REPORT.md`, and `COMPLETION_AUDIT.md`.
