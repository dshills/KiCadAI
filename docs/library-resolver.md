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
go run ./cmd/kicadai --json \
  --klc-root /path/to/klc \
  --symbols-root /path/to/kicad-symbols \
  --footprints-root /path/to/kicad-footprints \
  --templates-root /path/to/kicad-templates \
  library index
```

## Commands

Index summary:

```sh
go run ./cmd/kicadai --json library index
```

Resolve records:

```sh
go run ./cmd/kicadai --json library symbol Device:R
go run ./cmd/kicadai --json library footprint Resistor_SMD:R_0805_2012Metric
```

Search:

```sh
go run ./cmd/kicadai --json library search-symbols resistor
go run ./cmd/kicadai --json library search-footprints 0805
```

Compatibility and pinmap candidate generation:

```sh
go run ./cmd/kicadai --json \
  library validate-assignment Device:R Resistor_SMD:R_0805_2012Metric

go run ./cmd/kicadai --json \
  library compatible-footprints Device:R

go run ./cmd/kicadai --json \
  library pinmap-candidate Device:R Resistor_SMD:R_0805_2012Metric
```

KLC checks:

```sh
go run ./cmd/kicadai --json library klc-symbol Device:R
go run ./cmd/kicadai --json library klc-footprint Resistor_SMD:R_0805_2012Metric
```

Templates:

```sh
go run ./cmd/kicadai --json library templates
go run ./cmd/kicadai --json library template Arduino_Nano
```

## Cache

Large KiCad libraries are expensive to parse repeatedly. Enable the optional
cache with either a flag or `KICADAI_LIBRARY_CACHE`:

```sh
go run ./cmd/kicadai --json \
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

## Known Limitations

- KLC checks are conservative and partial.
- Native KiCad pin-stack parsing is not modeled yet, so duplicate flattened pin
  numbers are reported conservatively.
- Compatibility uses symbol pins, footprint pads, filters, names, and metadata;
  it does not prove electrical correctness.
- Template indexing records project structure but does not instantiate
  templates yet.
- Cache validation still performs discovery so added or removed files invalidate
  cached records correctly.
