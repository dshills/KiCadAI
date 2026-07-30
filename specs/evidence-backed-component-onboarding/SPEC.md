# Evidence-Backed Automatic Component and Model Onboarding

## Status

Implementation milestone.

## Problem

KiCadAI can compose and verify a broad set of reviewed circuit architectures,
but selectable components and trusted simulation bindings still come from a
finite checked-in catalog. Adding each unfamiliar manufacturer part directly
to production code or a hand-maintained allowlist does not scale toward
arbitrary-circuit generation.

This milestone introduces a generic trust boundary between untrusted document
extraction and the supported component/model registries. An AI, OCR pipeline,
vendor feed, or deterministic parser may propose facts. A proposal cannot be
selected merely because its extraction succeeded.

## Goals

Given a behavior-only component requirement and content-addressed manufacturer
documentation, the system shall:

1. discover one or more concrete candidates without an input part number;
2. retain exact source excerpts, locations, revisions, publishers, locators,
   licenses, and SHA-256 identities for extracted claims;
3. validate component identity, ratings, temperature range, derating, symbol
   pins, package pads, and pin mapping;
4. bind a registered simulation primitive with reviewed provenance, or a
   bounded analytic substitute with explicit assumptions;
5. rank safe candidates deterministically and retain safe alternatives;
6. quarantine every candidate until independent two-run promotion evidence and
   an exact-hash approval are present;
7. produce an immutable catalog/model overlay that ordinary selection and
   closed-loop simulation can consume; and
8. fail closed on missing, conflicting, irrelevant, fabricated, incompatible,
   out-of-range, or nondeterministic evidence.

## Non-Goals

- Treating model output as evidence.
- Guessing omitted ratings, pins, package mappings, thermal limits, or license.
- Downloading arbitrary remote content during design generation.
- Automatically granting fabrication approval.
- Adding held-out identities, package-specific branches, or part-family
  allowlists to production code.
- Supporting an unregistered physical or simulation primitive merely because a
  datasheet names it.

## Trust Model

Extraction is untrusted. `componentonboarding.Extractor` implementations may use
AI or deterministic tooling, but their output has no selection authority.

Every accepted claim must:

- reference one ingested immutable document;
- preserve an exact excerpt found in that document;
- anchor the claimed value and unit in that excerpt;
- identify a document location;
- agree with all other claims for the same subject and field; and
- be consumed by a required evidence category.

Manufacturer identity claims must come from a datasheet, package drawing, or
model document whose publisher matches the proposed manufacturer.

## Request Contract

`kicadai.component-onboarding-request.v1` contains:

- a canonical requirement ID;
- one already registered component family;
- required electrical functions;
- required ratings and units;
- a complete operating-temperature interval;
- required registered analyses;
- optional allowed package types; and
- a minimum rating-derating ratio.

The requirement is behavior-first. It contains no candidate manufacturer, MPN,
symbol, footprint, or simulation-model ID.

## Document Contract

Each document supplies:

- canonical document ID and kind;
- publisher;
- stable publisher HTTPS or KiCadAI content-identity locator; untrusted local
  file locators are rejected and locator metadata is never dereferenced;
- revision;
- model license where applicable;
- expected SHA-256; and
- bounded immutable content.

Binary documentation must first be converted to a content-addressed text
representation for AI extraction. The original document identity remains part
of the evidence chain.

## Candidate Contract

`kicadai.component-onboarding-candidate.v1` is always `quarantined`. It records:

- normalized document metadata;
- source-anchored claims;
- proposed `components.ComponentRecord` values;
- evidence bindings for identity, ratings, temperature, package, pin mapping,
  derating, model, and provenance;
- trusted-model provenance or bounded analytic assumptions;
- deterministic score/rank information;
- selected and alternate candidate identities; and
- a content hash.

Candidate building must not mutate the base component catalog or default model
registry.

## Deterministic Validation

### Catalog and ratings

The proposed record must be concrete, new to the base catalog, and a member of
the requested registered family. The ordinary component catalog validator and
selector must accept it at the requested functions, ratings, temperature, and
package bounds.

Every required rating must have source evidence and meet the declared minimum
derating ratio. Temperature evidence must cover the complete requested
interval.

### KiCad libraries

Every proposed symbol and footprint must exist in the supplied KiCad library
snapshot. Every required function must map to:

- an existing symbol pin;
- an existing footprint pad; and
- a matching source claim for the proposed pin-to-pad mapping.

No name-only footprint or symbol match is sufficient.

### Models

The proposed model ID must exist in the trusted simulation registry and support
every required analysis. Catalog parameters must pass the family-specific
model validator.

Manufacturer models require a licensed model document. A bounded analytic
substitute requires explicit applicability assumptions and may run only inside
the claimed ratings and temperature interval. Model provenance is keyed by the
new catalog ID and canonical model-definition hash.

### Ranking

Eligible candidates are ranked by:

1. descending minimum required-rating margin;
2. descending source-evidence coverage;
3. ascending component ID; and
4. ascending package variant ID.

Reordered documents, claims, and proposals must produce the same candidate
hash, ranking, and selection.

## Promotion

Promotion requires two passing, normalized, hash-identical runs of:

- simulation;
- connectivity;
- route completion;
- writer correctness;
- schematic/PCB round trip;
- ERC; and
- strict DRC.

It also requires an independent approval bound to the exact candidate hash.
Only then may the system create
`kicadai.component-catalog-overlay.v1` with `supported` status.

Applying an overlay revalidates its content hash and merges component and model
records without mutating the embedded base registries. Unknown, duplicated, or
conflicting records fail closed.

## Held-Out Acceptance Corpus

The frozen corpus contains one previously unseen candidate in each category:

- op-amp;
- transistor;
- regulator;
- converter;
- sensor;
- logic device; and
- interface component.

Corpus membership and its SHA-256 sidecar are frozen before promotion.
Production Go files may not contain corpus candidate IDs or MPNs.

Every positive row must prove:

- source and claim validation;
- catalog and model validation;
- exact KiCad symbol/footprint/pin-map compatibility;
- deterministic ranking and replay;
- quarantine before approval;
- supported overlay selection after approval;
- trusted model lookup after approval; and
- the complete two-run installed-KiCad promotion lane.

Negative rows cover fabricated excerpts, unanchored values, conflicting claims,
missing evidence categories, digest mismatch, publisher mismatch, missing
license, insufficient ratings/temperature/derating, nonexistent symbol pins or
footprint pads, unregistered/incompatible models, failed physical gates,
nondeterministic gate evidence, approval mismatch, and overlay tampering.

## Completion Criteria

The milestone is complete only when:

- the production path is identity-neutral and contains no held-out IDs;
- the seven held-out categories pass from document extraction through supported
  overlay use;
- all negative cases fail closed with stable errors;
- local unit, repository, lint, and race gates pass;
- all seven cases pass the installed-KiCad lane twice with clean ERC, strict
  DRC, complete connectivity/routing, writer correctness, and zero round-trip
  differences;
- two clean roots produce the same promotion bundle; and
- Prism reports no unresolved high- or medium-severity findings.
