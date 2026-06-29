# Schematic Example Readability Audit

Date: 2026-06-29

This audit captures the baseline parsed schematic structure for the scoped checked-in examples before readability fixture rewrites.

| Example | Symbols | Wires | Labels | Junctions | Diagonal wires | Symbols at origin | Symbols with pin anchors | Pin anchors |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `01_led_indicator` | 4 | 3 | 1 | 0 | 0 | 0 | 0 | 0 |
| `02_button_pullup` | 4 | 4 | 1 | 1 | 0 | 0 | 0 | 0 |
| `03_rc_filter` | 5 | 3 | 2 | 1 | 0 | 0 | 0 | 0 |
| `04_555_timer` | 9 | 24 | 2 | 4 | 0 | 0 | 0 | 0 |
| `05_sensor_node` | 0 | 0 | 3 | 0 | 0 | 0 | 0 | 0 |
| `06_class_ab_headphone_amp` | 22 | 34 | 6 | 8 | 1 | 0 | 0 | 0 |
| `09_class_a_headphone_amp` | 22 | 34 | 6 | 8 | 1 | 0 | 0 | 0 |
| `10_opamp_buffer_headphone_amp` | 22 | 34 | 6 | 8 | 1 | 0 | 0 | 0 |

## Notes

- Diagonal wire counts are derived from parsed wire point pairs only.
- Symbol pin anchors are present when the parser can recover explicit anchor coordinates from generated fixture metadata.
- Later phases add geometry and amplifier-specific readability validation on top of this baseline.
