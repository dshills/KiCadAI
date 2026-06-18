# Adding Verified Component Records

Verified records must be curated, resolver-compatible, and test-covered.

Minimum checklist:

- add or reuse a family in `data/components/families.json`;
- add a catalog record under `data/components`;
- include symbol ID, footprint ID, function pins, pad functions, and confidence;
- add or reuse a built-in pinmap in `internal/pinmap`;
- include MPN, lifecycle, ratings, companions, and notes when known;
- keep placeholders honest when evidence is missing;
- add or update component selection and coverage golden tests;
- run `go test ./internal/components ./internal/pinmap`.

Do not mark an active component `verified` unless symbol, footprint, and pinmap
evidence are explicit and reviewed.
