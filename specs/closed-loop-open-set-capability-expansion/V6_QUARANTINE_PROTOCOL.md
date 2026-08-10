# Closed-Loop Open-Set V6 Quarantine Protocol

Status: validator freeze candidate; no corpus synthesis or outcome inspection

## Isolation boundary

Each of the three authors receives only the common files and their own
assignment frozen by `v6-authoring-packet/AUTHOR_N_PACKET.sha256`. Returned
bundles remain in three disjoint quarantine roots. A bundle contains exactly
its assigned requirement JSON files and `AUTHORSHIP.json`; symlinks, special
files, extra paths, missing paths, hash drift, cross-author identities, and
incomplete provenance fail closed.

The custodian may run only the outcome-neutral validator in this phase:

```text
go run ./cmd/kicadai-corpus-validate-v6 \
  -packet-root specs/closed-loop-open-set-capability-expansion/v6-authoring-packet \
  -history specs/closed-loop-open-set-capability-expansion/V6_HISTORICAL_COMMITMENTS.json \
  -bundle author_1=/isolated/author_1 \
  -bundle author_2=/isolated/author_2 \
  -bundle author_3=/isolated/author_3 \
  -output /isolated/validation_report.json
```

The validator must not run synthesis, simulation, feasibility classification,
gap extraction, ranking, selection, or KiCad promotion. Successful stdout
contains only the total validated case count and isolated-author count.

## Historical boundary

`V6_HISTORICAL_COMMITMENTS.json` is mechanically and exactly derived from the
frozen V5 historical file and the public V5 corpus manifest. It contains:

- 132 raw commitments: every published V1 through V5 requirement;
- 60 neutral-semantic commitments: every published V4 and V5 commitment; and
- 36 identifier-normalized commitments: every published V5 commitment.

The derivation test opens no retired requirement source. It proves canonical
ordering, digest uniqueness, exact set extension, and
`retired_source_opened: false`. The production V6 loader strictly decodes this
stronger format and the V6 validator rejects matches against all three sets.

## Mechanical validation

The validator enforces the frozen packet, assignments, authorship attestations,
source hashes, strict public requirement contract, enums, references, finite
bounds, acceptance gates, quotas, diversity, structural uniqueness, prohibited
implementation language, and historical non-overlap. It writes only canonical
case identities, commitments, timestamps, and aggregate counts. The report
contains no source bytes, expected outcomes, synthesis results, feasibility
results, gaps, or selected capability data.

`V6_VALIDATOR.sha256` binds the complete V6 command and wrapper plus every
byte-frozen V5 parser, normalizer, validator, secure loader, report writer, and
public requirement dependency it reuses. The wrapper leaves those V5 bytes
unchanged and adds only the V6 policy, stronger history loader, and
identifier-normalized historical rejection.
