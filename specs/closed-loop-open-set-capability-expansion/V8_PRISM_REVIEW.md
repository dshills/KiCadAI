# V8 Contract Prism Review

Scope: the staged V8 specification addendum, plan, corpus rules, baseline
protocol, machine-readable selection policy, contract checksum manifest, and
executable contract verifier. Prism used the configured external Gemini
provider. V8 corpus author packets, corpus source, synthesis, selection, and
held-out artifacts did not yet exist.

## Result

Prism reported zero high findings and zero medium findings.

## Low-finding dispositions

- Safety weights were present in the policy but not explicitly asserted by the
  verifier: `remediated`. The verifier now requires the exact four keys and
  freezes weights `0`, `1`, `3`, and `5`.
- Effect-closure properties were not completely checked:
  `remediated`. The verifier already asserted every closure value and now also
  rejects missing or additional closure keys.
- Runtime-trace terminology differed between two documents: `remediated` by
  consistently using `focused non-corpus runtime traces`.
- Repository-relative prerequisite paths used embedded separators in the Go
  verifier: `remediated` with platform-native `filepath.Join` segments.
- Nested policy maps could admit unverified additional fields: `remediated`.
  Every nested object now requires its exact frozen key set.
- The complete prohibition list was not asserted: `remediated` with an exact
  ordered-list check covering fixture/corpus/author/outcome dispatch, mutation,
  overrides, gate relaxation, disclosure, and manual GitHub Actions.

The exact reviewed contract preserves fresh-corpus isolation, behavior-only
authoring, obligation-anchored append-only lineage, bounded same-stage or
higher-stage refinement, predeclared budgeted effect closure, deterministic
selection, installed-KiCad promotion, held-out blindness, and fail-closed
terminal behavior.
