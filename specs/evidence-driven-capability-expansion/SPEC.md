# Evidence-Driven Capability Expansion Specification

Status: implemented

## Purpose

Convert a fail-closed capability assessment into a deterministic, reviewable
proposal for adding reusable generation capability. Expansion must preserve the
existing confidence gate: source collection and candidate construction never
change the classification of the original request, and only an explicit
reviewed promotion may update a supported registry used by a fresh run.

## Trust boundaries

1. The unsupported `capabilitygate.Assessment` is immutable source evidence.
2. Expansion planning is pure and derives only from normalized gaps,
   requirements, and evidence.
3. Source ingestion is local, bounded, content-addressed, and strict. CLI
   source files must resolve inside the candidate manifest directory, including
   after symlink resolution. A candidate may contain at most 512 sources,
   8 MiB per source, and 64 MiB in aggregate. A URL, filename, model name, or
   AI assertion is not proof.
4. Candidate providers live in a quarantined registry and are always
   experimental.
5. Generated promotion cases and submitted gate results are distinct.
6. A supported registry changes only through `Promote` with an approval bound
   to the exact promotion-bundle hash.
7. A fresh architecture search may load the supported registry. The original
   unsupported assessment is never upgraded in place.

## Stable artifacts

The implementation defines:

- `kicadai.capability-expansion-plan.v1`
- `kicadai.capability-candidate-registry.v1`
- `kicadai.capability-promotion-bundle.v1`
- `kicadai.supported-capability-registry.v1`
- `kicadai.capability-promotion-approval.v1`

Every artifact is normalized, strictly validated, deterministically serialized,
and sealed with SHA-256.

The CLI also strictly decodes local orchestration manifests:

- `kicadai.capability-candidate-build-request.v1`
- `kicadai.capability-bundle-build-request.v1`

## Expansion planning

Every capability gap maps to one reusable need category:

- `architecture`
- `component`
- `model`
- `physical_rule`
- `routing`
- `verification`

The mapping uses typed requirement kind, stage, and diagnostic code—not project
identity, corpus identity, descriptions, or fixture paths. Each need lists the
source evidence kinds, implementation artifact, promotion gates, risks, and
action required for promotion.

## Quarantined candidates

A candidate package contains:

- the exact expansion plan and assessment hash;
- local source records with publisher, immutable locator, byte count, SHA-256,
  license where applicable, and explicit need claims;
- bounded canonical JSON for each proposed catalog, model, physical, routing,
  verification, or provider artifact, with a verified SHA-256;
- generic declarative architecture providers using the existing typed provider
  and fragment-realization contracts;
- assumptions and residual risks;
- deterministic representative and adversarial promotion cases.

Candidate validation rejects missing bytes, digest mismatches, duplicate or
conflicting identities, unknown source kinds, claims unrelated to the plan,
missing required source kinds, invalid provider realizations, provider
capability mismatches, unbound provider evidence, and artifact payload/hash
mismatches. The artifact and provider schemas expose reusable typed
capabilities rather than project identities and do not contain layout
coordinate or route-exception fields.

## Promotion evidence

Case generation emits one representative case and adversarial missing,
conflicting, irrelevant, and fabricated-evidence cases for every need.
Promotion additionally requires deterministic replay, simulation, workflow,
installed-KiCad ERC and strict DRC, connectivity, route completion, writer
correctness, and zero normalized round-trip differences.

The bundle remains `experimental` until every required result is present and
passing. Passing results make the bundle `review_ready`, not supported.

## Explicit promotion

Promotion requires:

- a `review_ready` bundle;
- approval with reviewer, decision, review reference, review SHA-256, and exact
  bundle hash;
- explicit mutation authorization from the caller;
- a non-conflicting supported registry.

Promotion copies only validated generic artifacts, any validated declarative
architecture provider, and their source, test, result, and review hashes. It
never copies source bytes or candidate-only state. The resulting registry is
content-addressed; architecture entries can instantiate typed providers for a
fresh request, while component, model, physical, routing, and verification
entries remain reviewed registry artifacts for their respective consumers.

## Acceptance

1. Three unsupported requests from different electrical domains produce
   accurate byte-identical plans.
2. Two reusable declarative capabilities move from unsupported to experimental
   candidate, review-ready bundle, and explicitly reviewed supported registry.
3. Fresh held-out architecture requests select those promoted providers.
4. Missing, conflicting, irrelevant, and fabricated evidence fails closed.
5. Candidate and incomplete bundles cannot be labeled supported or
   fabrication-ready.
6. Identical assessments, sources, candidates, cases, results, approvals, and
   registries produce byte-identical artifacts.
7. No fixture names, coordinates, allowlists, component exceptions, or
   circuit-family branches are added to production decisions.
8. `requirement create` may load a promoted registry explicitly and otherwise
   preserves the built-in registry behavior.
9. The complete local short suite and representative installed-KiCad lanes
   pass.
10. Documentation, Prism review, commit, and push close the milestone.
