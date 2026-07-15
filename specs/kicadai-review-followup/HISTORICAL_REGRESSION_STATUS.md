# Historical Regression Status

Audit date: 2026-07-15  
Scope: findings cited by `specs/kicadai-review.md` from archived code reviews.

This ledger distinguishes a current regression test from a historical claim. A
focused unit test proves the modeled behavior; optional KiCad-backed tests add
external application evidence only when their environment variables are set.

| Historical finding | Current status | Evidence |
| --- | --- | --- |
| MCU AREF decoupling route selected an identical VCC capacitor | Addressed | `internal/blocks/realize_test.go` verifies that `mcu_aref_decoupling` starts at the component with role `aref_decoupling_capacitor`. |
| Regulator and USB-C power LEDs were reverse-biased | Addressed | `internal/blocks/regulator_test.go` and `internal/blocks/usb_c_power_test.go` assert resistor-to-anode and cathode-to-ground polarity. |
| PCB reader/writer converted oval slots to round drills | Addressed for modeled pads | `internal/kicadfiles/pcb/read_test.go` preserves an oval drill and track arc through read/write. |
| Schematic reader/writer cleared DNP or collapsed multi-unit symbols | Addressed for covered model fields | `internal/kicadfiles/schematic/read_test.go` covers DNP parsing; `internal/kicadfiles/roundtrip/schematic_ir_integration_test.go` provides optional KiCad-backed multi-unit round-trip evidence. |
| Off-grid pads were reported disconnected after routing | Addressed | `internal/routing/validation_test.go` covers endpoint connectivity and same-layer joins. |
| Crossed copper segments escaped clearance validation | Addressed | `internal/routing/validation_test.go` includes `TestValidateResultDetectsCrossedTraceClearanceViolation`. |
| Arbitrary KiCad-authored imported content could be lost on rewrite | Open limitation | Generated and preservation-reviewed fixtures have writer and normalized round-trip gates. Arbitrary imported features remain fail-closed or preservation-scoped; they are not covered by a blanket lossless rewrite guarantee. |

## Reproduction Commands

Run the deterministic regression corpus:

```sh
make test
```

Run optional KiCad-backed round-trip evidence when KiCad CLI and the required
libraries are configured:

```sh
KICADAI_RUN_KICAD_CLI=1 go test ./internal/kicadfiles/roundtrip
```

Do not change writer behavior based solely on an archived finding. First add a
minimal reproducer to the appropriate reader, writer, preservation, or
round-trip package and then promote it to the regression corpus.
