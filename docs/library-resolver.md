# KiCad Library Resolver

The library resolver indexes local KiCad symbol, footprint, and template
repositories and returns deterministic JSON reports through the `kicadai`
CLI. It is the bridge between AI design intent and real KiCad library IDs such
as `Device:R` and `Resistor_SMD:R_0805_2012Metric`.

## Setup

Configure roots with flags or environment variables:

```sh
export KICADAI_KLC_ROOT=/path/to/klc
export KICADAI_SYMBOLS_ROOT=/path/to/kicad-symbols
export KICADAI_FOOTPRINTS_ROOT=/path/to/kicad-footprints
export KICADAI_TEMPLATES_ROOT=/path/to/kicad-templates
```

Equivalent flags:

```sh
kicadai \
  --klc-root /path/to/klc \
  --symbols-root /path/to/kicad-symbols \
  --footprints-root /path/to/kicad-footprints \
  --templates-root /path/to/kicad-templates \
  library index
```

## Commands

Index summary:

```sh
kicadai library index
```

Resolve records:

```sh
kicadai library symbol Device:R
kicadai library footprint Resistor_SMD:R_0805_2012Metric
```

Inspect hardened symbol evidence:

```sh
kicadai library symbols list
kicadai library symbols show Device:R
kicadai library symbols pins Device:R
kicadai library symbols validate Device:R
```

The nested `library symbols` commands return stable JSON for AI agents and
tests. `list` emits compact symbol summaries. `show` returns the parsed symbol
record, including properties, units, pins, inherited metadata, power-symbol
classification, and rough graphics bounds. `pins` returns the deterministic pin
list with unit, common-pin, hidden-pin, position, orientation, and electrical
type evidence. `validate` runs symbol KLC checks and surfaces resolver
diagnostics.

Search:

```sh
kicadai library search-symbols resistor
kicadai library search-footprints 0805
```

Compatibility and pinmap candidate generation:

```sh
kicadai \
  library validate-assignment Device:R Resistor_SMD:R_0805_2012Metric

kicadai \
  library compatible-footprints Device:R

kicadai \
  library pinmap-candidate Device:R Resistor_SMD:R_0805_2012Metric
```

KLC checks:

```sh
kicadai library klc-symbol Device:R
kicadai library klc-footprint Resistor_SMD:R_0805_2012Metric
```

Templates:

```sh
kicadai library templates
kicadai library template Arduino_Nano
```

## Cache

Large KiCad libraries are expensive to parse repeatedly. Enable the optional
cache with either a flag or `KICADAI_LIBRARY_CACHE`:

```sh
kicadai \
  --library-cache .kicadai/library-index.json \
  library index
```

The cache stores the schema version, roots, file metadata, parsed symbol and
footprint records, diagnostics, and generation timestamp. It is invalidated
when the schema changes, roots change, file count changes, or any indexed file
size or modification time changes. Use `--refresh-library-cache` to force a
rebuild.

Cache writes are atomic: the resolver writes a temporary file, syncs it, and
renames it into place. The resolver only writes a cache when a cache path is
explicitly configured.

## Compatibility Statuses

- `compatible`: the resolver has enough evidence that the assignment is
  structurally compatible.
- `candidate`: the resolver found heuristic evidence, such as footprint
  filters or compatible pad counts, but not enough evidence to call the
  assignment compatible.
- `needs_verification`: the assignment shape is plausible, but a trusted pinmap
  is missing, so fabrication readiness should remain blocked.
- `incompatible`: the resolver found blocking mismatches.
- `unknown`: one or more records could not be resolved.

Pinmap candidates are inferred suggestions. They are useful for AI planning,
but they are not treated as human-verified fabrication evidence until promoted
to the pinmap registry.

## Integration Tests

Normal tests do not require local KiCad library repositories. Run resolver
integration tests explicitly:

```sh
KICADAI_RUN_LIBRARY_INTEGRATION=1 \
KICADAI_KLC_ROOT=/path/to/klc \
KICADAI_SYMBOLS_ROOT=/path/to/kicad-symbols \
KICADAI_FOOTPRINTS_ROOT=/path/to/kicad-footprints \
KICADAI_TEMPLATES_ROOT=/path/to/kicad-templates \
go test ./internal/libraryresolver
```

The symbol CLI golden corpus is checked in under
`cmd/kicadai/testdata/golden/library_symbols`. Update it only when the JSON
contract intentionally changes:

```sh
go test ./cmd/kicadai -run TestRunLibrarySymbolsGolden -update-library-symbol-goldens
```

Optional external KiCad symbol smoke tests are separate from the default test
suite and require both environment variables:

```sh
KICADAI_RUN_EXTERNAL_SYMBOL_TESTS=1 \
KICAD_SYMBOLS_DIR=/path/to/kicad-symbols \
go test ./cmd/kicadai -run TestExternalKiCadSymbolsSmoke
```

## Known Limitations

- KLC checks are conservative and partial.
- Native KiCad pin-stack parsing is not modeled yet, so duplicate flattened pin
  numbers are reported conservatively.
- Compatibility uses symbol pins, footprint pads, filters, names, and metadata;
  it does not prove electrical correctness.
- Hidden power pins currently require explicit connectivity policy before they
  can be treated as safe evidence.
- Template indexing records project structure but does not instantiate
  templates yet.
- Cache validation still performs discovery so added or removed files invalidate
  cached records correctly.
