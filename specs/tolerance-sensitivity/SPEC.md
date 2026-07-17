# Bounded tolerance and sensitivity proof

## Contract

`simulation.worst_case` requests fabrication-proof evaluation over only
resolved catalog uncertainty evidence. Evidence has a canonical plan target,
reviewed catalog source, nominal value, and finite inclusive bounds. Provider
input cannot select a solver, distribution, sample count, or corner policy.

The trusted evaluator runs nominal, every one-at-a-time endpoint, then the
complete lower/upper Cartesian corner set. It permits at most six uncertain
scalars (64 complete corners); missing, malformed, incompatible, or larger
evidence sets block the proof.

Every corner is evaluated through the same trusted simulation registry. A
corner assertion failure blocks the report and names its corner plus the
dominant one-at-a-time contributor. MNA corner plans recompute the trusted
topology hash after each catalog-backed value substitution.

## Evidence

Passive resistance and capacitance tolerances are accepted only when a
catalog record has verification sources and a finite percent tolerance. The
resolver converts the catalog percentage into lower and upper SI bounds.
Unknown catalog tolerance forms are deliberately not inferred.

## Compatibility

The default is nominal evaluation. Existing fixtures remain unchanged unless
they explicitly set `simulation.worst_case: true`; that request with no
compatible evidence is blocked rather than silently downgraded.
