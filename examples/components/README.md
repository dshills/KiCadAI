# Component Selection Examples

These requests are intended for `kicadai --request <file> component select`.

`select_concrete_resistor.json` requires a concrete verified 10 kOhm 0805
resistor and should resolve to the checked-in Yageo seed alternative.

Additional component-selection examples in this directory demonstrate broader
selection, rejection, and draft-placeholder behavior.

Local lifecycle and availability evidence is loaded with `--source-dir` when a
caller provides a compatible snapshot directory. The repository test fixtures
include curated snapshots, but live distributor lookups are not performed.

Value strings use the same engineering suffix parser as the selector, so
resistance examples can use values such as `10k`.
