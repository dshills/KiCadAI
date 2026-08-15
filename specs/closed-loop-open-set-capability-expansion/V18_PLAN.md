# V18 Plan

1. Record the exact V6-V17 historical-source hashes before implementation and
   fail if any is already dirty.
2. Move the reviewed low-voltage component evidence into a V18-only embedded
   catalog/provenance extension; prove legacy catalog and inventory hashes are
   unchanged.
3. Add V18-only search, composition, and input-impedance adapters implementing
   the bounded generic capability contract.
4. Add a V18 synthesis constructor that uses the extension only when the public
   requirement electrically requires it and otherwise delegates byte-for-byte
   to the V17 path.
5. Add generic unit tests, a public discovery replay test, and explicit legacy
   preservation tests. Do not encode a corpus filename, case ID, or expected
   outcome in production code.
6. Run focused tests, the complete local Go suite, deterministic replay, and
   installed-KiCad promotion. Repair implementation defects without changing
   historical evidence.
7. Freeze the exact V18 source and environment manifests, review the staged
   diff with Prism, remediate valid findings, commit, and push.
8. From the clean committed tree, execute the authorized 24-case public
   generation-one runner twice per case and atomically publish either the
   complete comparison or a fail-closed retirement. Do not access held-out
   keys.
