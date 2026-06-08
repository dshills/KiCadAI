# KiCad API Proto Vendor Snapshot

This directory contains a pinned subset of the KiCad source tree used for Go protobuf generation.

Vendored content:

- `api/proto`
- `LICENSE*`
- `AUTHORS.txt`

The exact upstream source is recorded in `VERSION`. The proto files are vendored so generated Go bindings can be reproduced without relying on an arbitrary local KiCad checkout.

Refresh with:

```sh
make refresh-kicad-proto
```

To use a different KiCad commit or tag:

```sh
KICAD_REF=<commit-or-tag> make refresh-kicad-proto
```
