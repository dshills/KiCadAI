# V2 Baseline Audit

Status: discovery baseline frozen; held-out baseline not yet executed

## Frozen inputs

- Production starting commit: `8bdc31e668152b7324066bd75182d86d7320d3f8`
- V2 corpus freeze commit: `cea6040301230d16372aa1c390acb36903a0e711`
- Discovery baseline harness commit: `3ebe4116`
- Discovery baseline report hash:
  `33f660a711236f657e8fda52994326dcb5b54b0a657a7dd71919a9df77f96f23`
- Expansion-plan hash:
  `de5fbd23c7f4ca1312ea970463c0180d18e15704339a4f3bb729ba43a19a1780`

The 12 discovery requirements were synthesized twice in manifest order with
the frozen policy. Every pair produced byte-identical normalized synthesis
results. No held-out synthesis was run before this report and rank-one
selection were generated.

## Discovery outcomes

| Outcome | Count |
| --- | ---: |
| pass | 0 |
| unsupported | 6 |
| unsafe | 1 |
| exhausted | 5 |

No baseline case reached physical promotion because no synthesis case passed.
The unsafe case remains preserved as authoritative infeasibility evidence.

## Rank-one selection

The unchanged ranking policy selected:

- stage: `topology_search`
- scope: `topology`
- capability: `complete_topology`
- code: `OPEN_TOPOLOGY_SEARCH_EXHAUSTED`
- affected discovery cases: 5
- domains: analog, digital, and mixed-signal
- analysis kinds: 8
- safety score: 8
- reuse score: 5

Required evidence is a reviewed reusable topology-construction capability with
complete-graph evidence. The selected expansion plan requires the normal
simulation, connectivity, routing, writer, round-trip, KiCad ERC/DRC,
deterministic replay, and workflow promotion gates.

## Blind boundary

Held-out requirements and results did not participate in discovery evaluation,
outcome counts, clustering, ranking, or plan construction. The selection commit
must be created before held-out synthesis begins. Held-out baseline execution
will suppress per-case logs and store evidence in a cryptographically sealed
artifact whose key remains outside the repository until the generic production
diff is fixed.
