# V9 Prepublication Retirement Audit

V9 retired fail-closed during aggregate corpus validation, before corpus
publication, baseline evaluation, source-key creation, or any synthesis.

The six frozen author assignments cannot satisfy the frozen validator. In the
discovery partition, no `amplification_conditioning` assignment is
`safety_relevant` or `safety_critical`. In the held-out partition, no
`source_bias` assignment is high-safety. The validator requires every circuit
role in each partition to have at least one high-safety case and emitted
`V9_CIRCUIT_ROLE_BALANCE` at the first missing role.

This is an assignment/validator inconsistency, not an authored behavior or
synthesis result. Altering assignment metadata, weakening the validator, or
relabeling authored cases after the author-packet and validator freezes would
violate V9's immutable evaluation boundary. V9 is therefore permanently
retired. Its corpus will not be published or evaluated.

The digest-only V9 historical commitment remains valid and reusable by a new
experiment version. Author quarantines are retained pending an explicit cleanup
boundary; no quarantine content is committed or disclosed.
