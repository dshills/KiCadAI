# V9 Historical-Commitment Custodian Freeze

The V9 history custodian is frozen before the real V8 source key is read. Its
only secret-bearing operation authenticates the 18 encrypted V8 held-out
records and immediately reduces them to the already committed source, neutral
semantic, and normalized semantic digests. Requirement plaintext is cleared
inside the V9-owned bridge and cannot cross its exported API.

The custodian:

- requires the exact external 32-byte `0600` V8 source key;
- authenticates the exact committed V8 corpus and its canonical 24-entry
  checksum set before opening the key;
- verifies the byte-frozen V1–V7 predecessor history;
- requires exactly 18 discovery plus 18 held-out commitment records;
- writes exactly 240 raw, 168 neutral-semantic, and 144 normalized-semantic
  commitments in canonical order;
- validates the completed history while it is still staged;
- publishes with an atomic, no-replace same-directory hard-link operation;
- reports only aggregate counts and the final artifact digest; and
- imports no synthesis, simulation, feasibility, ranking, frontier, gap,
  selection, or outcome implementation.

`V9_HISTORY_CUSTODIAN.sha256` binds the narrow custodian, its tests, the
standalone history package, the metadata-only V8 bridge, external-key reader,
V8 publisher freeze, V8 corpus checksums, predecessor history, V9 contract,
and line-ending policy. The real key remains unopened at this freeze.
