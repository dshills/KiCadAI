# Deterministic Dense-Board Correction Specification

## 1. Purpose

Extend the generic autonomous correction loop so a generated board can recover
from real placement and routing failures without changing circuit intent or
rebuilding unrelated copper.

This milestone implements the routing-only contracts reserved by
`generic-autonomous-correction` and strengthens the existing placement
corrections with transaction-operation scope. It reuses the deterministic
placer, router, route-tree, contact graph, physical-clearance validator,
transaction writer, and KiCad-backed promotion gates.

## 2. Required Outcomes

The implementation must:

1. classify congestion, foreign-net crossings, endpoint access, branch order,
   and missing layer transitions using normalized engine diagnostics;
2. correlate every automatic routing correction with generated route
   transaction operations;
3. derive the minimal affected-net set from diagnostic and operation evidence;
4. preserve non-affected route operations byte-for-byte;
5. move only components authorized by the existing mobility contract;
6. preserve topology, required nets, rules, board geometry, fixed components,
   required regions, keepouts, and protected local routes;
7. apply deterministic branch-order and legal layer-transition corrections;
8. fail closed when operation scope, endpoint identity, layer legality, or
   preservation cannot be proven;
9. prove identity-neutral held-out and adversarial recovery;
10. produce byte-identical results and correction evidence on replay; and
11. preserve all existing promoted circuit families.

## 3. Non-Goals

The milestone does not:

- add provider-visible geometry controls;
- accept provider prose or raw external-tool text as repair evidence;
- weaken ERC, DRC, connectivity, clearance, writer, or round-trip gates;
- change net membership, component selection, footprints, pin mappings, board
  layers, widths, clearances, via rules, or required-net policy;
- move fixed components or copper protected by local-route ownership;
- implement fixture-specific coordinates, identities, allowlists, schemas,
  block families, or topology dispatch;
- claim general push-and-shove or arbitrary-size autorouting.

## 4. Normalized Operation-Correlated Diagnostics

`AutonomousCorrectionDiagnostic` is extended with:

- sorted affected net names;
- sorted route-operation slice indexes;
- stable route-operation identities derived from canonical operation content;
- correlation status and a structured unsupported reason.

Correlation uses only:

1. an issue's exact operation identity when it resolves to a route operation;
2. an exact `operations[N]` path when it resolves to a route operation; or
3. exact affected-net membership when every selected operation is a generated
   route operation for a diagnostic-named net.

Reference-only or message-only inference is insufficient for routing mutation.
Ambiguous identity, an empty affected-net set, a named net with no replaceable
route operation, or a non-route operation in the selected scope stops
correction.

Operation correlation is recomputed against the current in-memory routing
result before every retry. Stored indexes are evidence, not permission to
mutate a later operation slice.

## 5. Correction Taxonomy

The existing taxonomy remains stable. This milestone adds explicit support for:

| Failure | Authorized correction |
| --- | --- |
| foreign-net copper crossing or clearance conflict | selectively rebuild the named conflicting nets while preserving all other copper |
| route-tree branch order | rebuild only the named net tree using a deterministic alternate branch order |
| missing legal layer transition | insert a transition at an already-proven same-net layer junction, or selectively rebuild the named net with the existing legal layer policy |
| inaccessible endpoint | apply a bounded eligible-component move when placement evidence exists; otherwise selectively rebuild the named net with endpoint-access ordering |
| routing-region exhaustion | first apply a bounded eligible placement action when authorized; otherwise selectively rebuild only operation-proven affected nets |

`unsupported_geometry` remains terminal.

## 6. Routing Correction Plan

A routing action contains:

- action kind;
- normalized category;
- sorted affected nets;
- current operation identities and indexes;
- deterministic route-order variant;
- expected circuit invariant fingerprint;
- expected route-operation preservation fingerprint;
- authorization and stop reason.

Routing actions do not carry coordinates. Geometry is recomputed from the
current board, pads, obstacles, rules, and deterministic router.

The minimal affected-net set is the union of diagnostic-named nets and nets of
exactly correlated route operations. The set may expand only when a structured
foreign-net conflict explicitly names the other net. Incidental nearby nets do
not enter the mutation scope.

## 7. Selective Route Replacement

For an authorized routing action:

1. snapshot the current operation slice;
2. partition generated route operations by affected-net membership;
3. expose every non-affected route as fixed, net-aware existing copper;
4. remove affected generated routes from the retry request;
5. route only affected nets under the unchanged resolved design rules;
6. re-run endpoint contact, connectivity, physical-clearance, via, and route
   completion checks;
7. splice replacement operations at a deterministic position;
8. verify non-affected operation canonical bytes and ordering are unchanged;
9. reject the candidate on any invariant, preservation, or validation failure.

The selected retry may not write intermediate project files. Only the selected
best attempt reaches the writer.

## 8. Deterministic Branch Ordering

Branch-order correction operates on structured route-tree branches for one
affected net. Candidate orders are a closed deterministic sequence:

1. current access-ranked order;
2. previously failed branches first;
3. previously failed branches last;
4. canonical endpoint-key order.

Candidates are ranked by:

1. fewer blocking validation findings;
2. more proven required endpoints;
3. fewer incomplete contact-graph components;
4. fewer failed branches;
5. fewer vias and shorter total route length;
6. canonical order key.

The same inputs must choose the same candidate. Random seeds, map iteration,
fixture identity, and provider text cannot affect the choice.

## 9. Legal Layer Transitions

A transition may be inserted only when:

- two same-net copper paths meet at the same canonical point on different
  allowed copper layers;
- the resolved net policy permits both layers and an additional via;
- the existing via dimensions and clearances are legal;
- the point is not inside a prohibited keepout or illegal via-in-pad region;
- no plated same-net transition already proves the connection.

Otherwise the action performs a selective affected-net rebuild using the
existing layer-aware router. The action never changes layer policy or design
rules. If neither insertion nor selective rebuild produces validated
connectivity and clearance, correction stops.

## 10. Placement Preservation

Placement actions continue through `BuildPlacementRetryAdjustment`.
Eligibility remains limited to the existing mobility classes and hard
constraints. A routing-only plan must not be translated into a placement move.

After any placement change, routing may replace only diagnostic-affected nets
unless the moved component invalidates endpoint geometry for additional nets.
Those additional nets must be derived mechanically from the moved component's
connected pads and recorded in the correction plan before routing.

## 11. Determinism And Invariants

The protected circuit fingerprint covers the existing generic-circuit
invariants. A route-preservation fingerprint additionally covers canonical
bytes for every non-affected route operation in slice order.

Two identical runs must produce identical:

- diagnostics and operation correlation;
- plans, retry keys, and stop reasons;
- selected placements and route operations;
- writer output and `.kicadai` evidence.

Maps are never iteration-order inputs. Coordinates use the existing canonical
fixed-point quantization.

## 12. Held-Out And Adversarial Evidence

Fixtures must be identity-neutral and production code must pass equivalent
renamed inputs.

Required cases:

- two foreign nets whose initial generated routes cross;
- a multi-endpoint tree whose baseline branch order blocks a later branch;
- a legal two-layer junction missing a transition;
- an endpoint-access failure with one movable and one fixed component;
- an all-fixed or illegal-layer case that stops without mutation;
- a decoy unrelated net proving byte-preserving selective replacement.

At least one held-out case must combine three or more nets and must not share
the training fixture's component references, net names, or coordinates.

## 13. Acceptance Gates

Completion requires current evidence for every item below:

- focused unit and integration tests for taxonomy, correlation, authorization,
  selective replacement, preservation, branch order, transitions, and stops;
- renamed-input and repeated-run equivalence;
- byte-identical non-affected operation proof;
- local `go test ./...`;
- local configured KiCad ERC and strict DRC;
- required-net connectivity and route completion;
- writer correctness and zero round-trip differences;
- existing MCU, USB-C, sensor, amplifier, clock/programming, and
  power-integrity promotion suites remain passing.

No GitHub Actions run is required for local acceptance.
