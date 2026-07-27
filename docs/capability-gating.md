# Capability-Aware Generation Gate

KiCadAI evaluates every normalized creation request before project files are
written. The assessment is deterministic, evidence-linked, and specific to the
requested domains, architecture, components, models, physical realization, and
verification contract.

## Classifications

| Classification | Meaning | Generation behavior |
| --- | --- | --- |
| `supported` | Every required capability claim is linked to verified, reproducible evidence. | Proceeds automatically and may become fabrication-ready only if all later gates also pass. |
| `experimental` | At least one required claim is inferred, but none is missing or failed. | Refused by default. `--experimental` permits generation, but the workflow and promotion report cannot label the result fabrication-ready or promotion `pass`. |
| `unsupported` | Required evidence is missing or failed, or a structured capability gap exists. | Refused before filesystem mutation with stable gap codes, reasons, and suggested actions. |

Verified evidence must have a stable source and SHA-256 digest. Inferred
confidence is kept distinct from verified evidence in the report. For example,
a catalog hash can verify which component and package were selected while an
inferred suitability rule remains explicitly marked as advisory evidence and a
visible risk for downstream verification. Inferred evidence linked to a
required capability always makes the request experimental; inferred evidence
cannot be left unlinked unless it carries the advisory marker.

## Experimental opt-in

The opt-in is a global CLI flag:

```sh
kicadai --experimental \
  --request ./request.json \
  --output ./out/experimental \
  design create
```

The flag authorizes generation only. It does not waive validation, convert
inferred evidence to verified evidence, or authorize a fabrication-ready claim.

## Evidence lifecycle

The assessment uses schema `kicadai.capability-assessment.v1` and policy
`capability-evidence-policy-v1`. It records:

- normalized capability requirements and their evidence IDs;
- evidence status (`verified`, `inferred`, `missing`, or `failed`);
- actionable gaps and remaining risks;
- stage checkpoints and the assessment hash;
- explicit experimental opt-in and fabrication-ready eligibility.

Architecture selection, component resolution, simulation, routing, writer
correctness, validation, and KiCad checks append deterministic checkpoints.
Classification is monotonic: later stages may preserve or reduce confidence,
but cannot upgrade it. A failed required stage downgrades the assessment and
removes fabrication-ready eligibility.

The final assessment is embedded in:

- `.kicadai/workflow-result.json`;
- `.kicadai/design-promotion.json`;
- `.kicadai/manifest.json`.

Promotion `pass` requires a supported assessment. Experimental evidence can
reach at most `candidate`; unsupported evidence blocks promotion.

## Promoting a new capability

A capability becomes supported by adding generic metadata and reproducible
evidence, not a fixture-name exception:

1. represent the domain, architecture, component/model, physical, and
   verification requirements in the existing typed registries;
2. provide stable catalog, provider, model, or block-verification sources with
   content hashes;
3. pass deterministic held-out and adversarial tests;
4. pass the complete local workflow and representative installed-KiCad ERC,
   strict DRC, connectivity, route completion, writer, and zero-diff
   round-trip gates;
5. preserve the evidence links in workflow, promotion, and manifest artifacts.

The current promoted and compositionally supported corpora classify as
`supported`. Unknown providers, components, pins, models, physical
capabilities, or required verification remain fail-closed.
