# Adversarial Function Corpus Closure History

The corpus was frozen before implementation at commit `93063343`, with 3 of
18 circuits passing the complete promotion profile and 15 blocked. Corrections
were applied in descending root-cause frequency and remained generic; no frozen
fixture, coordinate, allowlist, or component-specific block family was changed.

| Closure | Baseline count | Final count | Generic correction |
|---|---:|---:|---|
| Missing level-translation catalog capability | 3 | 0 | Added reviewed bidirectional and direction-controlled translation semantics. |
| Missing negative-power schematic lane | 3 | 0 | Derived signed power-lane placement from normalized net roles. |
| Missing bounded transient proof | 3 | 0 | Derived deterministic pulse grids and trusted diode/BJT transient analyses. |
| Dense placement envelope failure | 2 | 0 | Reserved physical envelopes generically during board-size derivation. |
| Missing multi-unit op-amp semantics | 2 | 0 | Added catalog-backed named-unit inference, shared power ownership, and bounded output saturation. |
| Dense route-completion failure | 1 | 0 | Selected four-layer routing under pressure and scheduled compact narrow-pitch escapes before peers. |
| Missing adjustable-regulator synthesis | 1 | 0 | Added catalog-driven companion formulas, deterministic E96 selection, and generic model-parameter overlay. |

The final run passes 17 of 18 circuits (94.44%). The remaining
`bipolar_lmv321_inverting_amplifier` case is intentionally blocked: its ±5 V
rails impose 10 V across an LMV321 whose reviewed supply range is 2.7–5.5 V.
This is a correct fail-closed electrical incompatibility, not an implementation
gap to bypass.

Every reported pass completed strict decode, deterministic synthesis, verified
resolution, applicable simulation, placement, routing and connectivity, writer
correctness, KiCad ERC, strict KiCad DRC, zero round-trip differences, and
byte-identical project replay. The original eight-case corpus and both protected
USB-C regression fixtures also remained green during closeout.
