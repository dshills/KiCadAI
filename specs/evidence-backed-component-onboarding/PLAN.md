# Implementation Plan

## Phase 0: Freeze scope and threat model

- Define the behavior-only request, document, extraction, candidate, promotion,
  and overlay schemas.
- Freeze the seven-category held-out corpus and SHA-256 manifest.
- Enumerate fabricated, conflicting, missing, incompatible, and
  nondeterministic evidence attacks.

## Phase 1: Document and extraction boundary

- Add bounded, content-addressed document ingestion.
- Add the untrusted `Extractor` interface.
- Add a structured AI-provider adapter that sends only supplied immutable
  documents and decodes strict extraction output.
- Reject binary provider prompts until they are converted to a bounded
  content-addressed text representation.

## Phase 2: Claim verification

- Normalize and order claims.
- Verify exact excerpts, value/unit anchoring, subject identity, locations, and
  document membership.
- Reject conflicting claims for the same subject and field.
- Require matching manufacturer publishers and document revisions.

## Phase 3: Component construction and electrical validation

- Require evidence bindings for identity, ratings, temperature, package, pin
  mapping, derating, model, and provenance.
- Validate new concrete records through the ordinary catalog validator and
  selector.
- Enforce full operating range and minimum derating.

## Phase 4: KiCad physical validation

- Resolve symbols and footprints from the supplied library snapshot.
- Verify every required function against existing symbol pins and footprint
  pads.
- Reject incomplete or mismatched pin maps.

## Phase 5: Simulation-model onboarding

- Validate model family, parameters, supported analyses, canonical model hash,
  revision, source, license, and temperature domain.
- Permit bounded analytic substitutes only with explicit assumptions.
- Produce independent model-provenance records keyed by the new component ID.

## Phase 6: Ranking, quarantine, and overlays

- Implement deterministic candidate ranking and stable replay.
- Keep candidates quarantined and prove the base catalog remains unchanged.
- Require exact-hash approval plus two normalized passing physical runs.
- Build and validate immutable supported component/model overlays.

## Phase 7: Held-out corpus and negative coverage

- Run op-amp, transistor, regulator, converter, sensor, logic, and interface
  cases through onboarding, promotion, overlay application, selection, and
  model lookup.
- Verify held-out identities are absent from production Go.
- Add all negative cases from the promotion matrix.

## Phase 8: Physical promotion

- Materialize one behavior-driven design per held-out category using the
  promoted overlay.
- Run simulation, connectivity, routing, writer, round trip, ERC, and strict
  DRC twice.
- Compare normalized run outputs and generate a content-addressed bundle.
- Repeat from two clean local roots.

## Phase 9: Integration and documentation

- Add CLI/API loading for candidate and overlay artifacts.
- Thread promoted catalog and model overlays through component selection,
  architecture search, closed-loop simulation, and project creation.
- Update AI generation, readiness, project status, and reproduction docs.

## Phase 10: Final verification and review

- Run focused tests, repository short tests, lint, and race-short locally.
- Run the complete installed-KiCad promotion bundle locally.
- Review the staged diff with Prism and resolve every high/medium finding.
- Commit the milestone without starting GitHub Actions.
