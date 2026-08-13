# V8 Blind Baseline Prism Disposition

Prism reported that two pairs of V8 baseline binding fields have equal
SHA-256 values. This is intentional and reproducible, not a copied
commitment:

- `gap_registry_sha256` and `gap_policy_sha256` both bind
  `V4_GAP_TRANSITION_POLICY.json`. The transition policy is the executable
  registry of admitted gap transitions; V8 does not define a second registry
  artifact.
- `environment_policy_sha256` and `resource_ceilings_sha256` both bind the
  normalized `V4_SYNTHESIS_POLICY.json`. That policy contains the complete
  bounded-search resource ceilings, so no separate ceilings artifact exists.

This follows the V8 baseline protocol's requirement to freeze the evaluator,
gap registry, synthesis policy, environment, and resource ceilings. The fields
identify semantic roles in the authenticated binding; they do not assert that
each role comes from a distinct file.

`TestClosedLoopV8HeldOutBaselineSealIsFrozen` checks all four fields against
their literal frozen commitments. The public verifier separately authenticates
the manifest self-hash, canonical checksums, exact encrypted file set, and
ciphertext commitment. Editing the already authenticated manifest merely to
make these values different would invalidate the blind publication and is not
permitted by the no-replace V8 protocol.
