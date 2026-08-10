# Closed-Loop Open-Set V5 Corpus Rules

## Neutral behavior only

Every requirement describes externally observable electrical behavior using
only the public requirement schema: semantic domains, ports, operating cases,
events, analyses, metrics, bounds, board limits, and the inherited acceptance
profile.

A requirement must not name or imply a topology, component, part, package,
model, footprint, symbol, net, layer, coordinate, route, placement, repair,
provider, algorithm, outcome, diagnostic, gap, capability, package rank,
expected implementation, fixture, example, or earlier requirement.

## Independent authorship

The supplied assignment contains exactly one discovery and one held-out case
in each of the six reporting domains. The author must not see another author's
assignment or requirement content.

The packet, assignment, acceptance requirements, and these rules are the only
inputs. The author must not access repository implementation, V1-V4 corpus
bytes or semantic summaries, any baseline or final result, selected capability,
development conversation, or expected classification. The authorship record
must contain exact inputs, tool/model identity if used, time, file hashes, and
a no-synthesis/no-outcome-inspection declaration.

Held-out files enter author-specific quarantine immediately. The implementation
context receives no held-out plaintext, filename-to-behavior summary, or
diagnostic before the one-time final boundary.

## Mechanical and public sanity validation

Before freeze, validation is limited to strict JSON/schema decoding, canonical
enums, reference integrity, finite ordered bounds, complete acceptance fields,
source hashes, exact and normalized uniqueness, quotas, and the diversity rules
below. Historical non-overlap compares new semantic hashes only with frozen
commitments; retired source or keys must not be opened.

Public electrical sanity review may check units, physical environmental bounds,
reference integrity, energy consistency, and simultaneous-corner coherence. It
must not run synthesis, query production feasibility, predict classification,
or recommend implementation changes.

## Required balance

Across the three returned bundles, the corpus must contain:

- exactly 18 discovery and 18 held-out cases;
- exactly three cases per role in each reporting domain: analog, power,
  digital/control, MCU/interface, sensor, and mixed-signal;
- exactly one case per author, role, and reporting domain;
- 36 unique neutral semantic IDs and stable manifest order;
- all inherited 14 acceptance booleans explicitly present and true; and
- at least two operating cases, four behavioral assertions, and two analysis
  kinds per requirement.

## Diversity within each role

The aggregate must include:

- single-positive, bipolar, and multiple-supply declarations;
- voltage-, current-, and power-observed behavior;
- at least five requirements with multiple observed source ports;
- at least five requirements with distinct excitations converging on one
  observed source port;
- DC, AC/noise/stability, transient/startup, and thermal/electrothermal work;
- load, tolerance/model, temperature, and supply variation;
- input, load, and power steps, startup, rail-loss, and bounded short-circuit
  events; and
- safety-critical assertions in at least four reporting domains.

Within this author's contribution, at least four analysis kinds and three event
kinds must appear across both roles. Discovery and held-out cases in the same
reporting domain must not be normalized paraphrases or share identical port,
assertion, and analysis signatures.

## Delivery

Do not rename paths or embed manifest identities in content. After authoring is
complete, calculate SHA-256 over the full exact byte stream of each of the
twelve returned JSON files—before normalization, reformatting, or any custodian
change—and record every assigned path and lowercase hexadecimal digest in
`AUTHORSHIP.md`. A substring, line range, semantic digest, normalized form, or
input-packet hash is not a requirement source hash. Do not disclose a held-out
behavior summary outside the quarantine bundle. Any requirement or authorship
change after the corpus freeze creates a new experiment version and requires a
fresh baseline. Invalid or uncertain evidence fails closed.
