# Simulation admission evidence

These files are stable public examples of the production
`kicadai.simulation-admission.v1` decision artifact. `admitted.json` records an
exact bundled model set and deterministic DC solver. The other files show each
typed fail-closed refusal category without reproducing a V19 or V20 evaluation
case.

Every artifact is strict-decoded, hash-verified, and regenerated from the same
generic resistor/load circuit in `internal/simulationadmission` tests. They are
evidence examples, not model registries or synthesis templates, and no circuit
family, coordinate, or evaluation-case identity is used by production code.
