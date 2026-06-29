# Component And Block Catalog Expansion Plan

## Phase C1: AI Readiness Matrix

- Add a directory-based machine-readable readiness matrix under
  `data/ai-readiness/`.
- Add a Go package in `internal/aireadiness` that loads and validates the
  matrix.
- Cover stable IDs, allowed categories/domains/readiness values, and required
  fields.

## Phase C2: Amplifier Gap Coverage

- Populate amplifier-oriented records for Class A/Class AB/headphone amplifier
  generation blockers.
- Add tests requiring the core amplifier gap categories.

## Phase C3: Documentation

- Document how contributors should use the matrix when expanding components and
  circuit blocks.
- Link the matrix from development docs.
