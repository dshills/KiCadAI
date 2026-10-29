# NetForm KiCadAI local client

This standalone Rust crate connects a NetForm consumer to KiCadAI through a
local subprocess. It does not depend on NetForm crates, change the GUI or
replace the simulation worker. NetForm's current uncommitted checkout is left
untouched. Rust 1.99 and the locked serde dependencies are required to build
this reference client; the deployed CLI protocol needs only a KiCadAI binary.

```sh
make build
KICADAI_ADAPTER_BIN="$PWD/bin/kicadai" cargo test --manifest-path integrations/netform-client/Cargo.toml --locked
cargo run --manifest-path integrations/netform-client/Cargo.toml --example catalog -- "$PWD/bin/kicadai"
```

The crate can be added as a local path dependency. Call it from a worker, never
from a GUI event handler. The caller selects an absolute executable path; the
client passes only the fixed `adapter` argument, writes one request to stdin,
closes stdin, and reads one response. It does not use a shell or network.
Credential-named environment variables are removed from the child environment.

The client bounds input to 64 KiB, output to 2 MiB, stderr to 64 KiB and the
process/stream wait to 45 seconds. Excess data, mismatched IDs/schema, unknown
response fields, contradictory envelopes and exit-status disagreements fail.
The selected executable must be trusted: this wrapper is not an OS sandbox.
On timeout it kills its direct child, not arbitrary descendant processes.

## Protocol v1

Request:

```json
{"schema":"netform.kicadai.request.v1","id":"catalog-1","method":"catalog.search","params":{"family":"resistor","limit":5}}
```

Success:

```json
{"schema":"netform.kicadai.response.v1","id":"catalog-1","ok":true,"result":{"items":[],"total":0,"truncated":false}}
```

Failure:

```json
{"schema":"netform.kicadai.response.v1","id":"catalog-1","ok":false,"error":{"code":"INVALID_PARAMS","message":"query/identity/limit exceeds bounds"}}
```

KiCadAI exits zero for success and nonzero for a protocol error. Domain
findings in a successful `schematic.inspect` result are not a transport error;
inspect `result.status` and individual checks. Unknown request/parameter
fields, unsupported schemas/methods, oversized data and trailing JSON fail.
IDs contain 1–64 ASCII letters, digits, underscore, period or hyphen.

| Method | Params | Result |
|---|---|---|
| `capabilities` | `{}` | Build identity, allowed methods, workflow inventory and byte limits |
| `catalog.search` | Optional `query`, `family`, `limit` (default 20, maximum 100) | Sorted items, total matches, truncation flag |
| `catalog.get` | Exact `id` only | Catalog identity, values/ratings, symbols, packages and verification metadata |
| `pinmap.get` | Exact `symbol` and `footprint` | Registered mapping; missing means unsupported, not approved |
| `schematic.inspect` | Local `path` | Read-only multi-sheet review, no native tool execution |
| `evidence.verify` | Local `path`, `kind`: `schematic`, `review`, `conformance`, `reproduction`, `component-test` | Integrity result, never independent attestation or a numerical rerun |
| `family.check` | `configuration` using existing board-family config v1 | Bounded electrical checks with native/hardware flags false |

The local process reads paths deliberately supplied by its caller; there is
no background scan, remote service, automatic repair, provider call or arbitrary
method-to-command forwarding. Existing schematic review bounds also apply.
Inspect data and paths may be private; no automatic upload occurs. The host
application remains responsible for trusted executable selection, user consent
to local file reads, worker scheduling and stale-revision handling.

New methods may be added compatibly to v1, but callers must not assume a
method exists: query `capabilities`. Incompatible envelope/parameter semantics
require a new schema version. Numerical and hardware evidence remain distinct
from catalog confidence labels and portable file integrity.
