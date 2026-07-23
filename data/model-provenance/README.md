# Model provenance registry

Each record binds one reviewed catalog component/model claim to a trusted
simulation primitive.

- `provenance.source` and `provenance.revision` identify the component evidence
  used to approve that binding.
- `provenance.sha256` is the SHA-256 of the canonical trusted model definition,
  as enforced by `internal/modelprovenance`. It is not a digest of the source
  datasheet.

Components that use the same canonical primitive therefore intentionally share
the same `provenance.sha256`, even when their source documents differ. Component
limits and uncertainty parameters remain catalog-specific.
