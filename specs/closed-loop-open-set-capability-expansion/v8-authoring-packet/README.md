# V8 Isolated Corpus Author Packet

This packet is the complete and only input permitted for one independent V8
corpus author. It contains exactly one file below `assignments/`; that file
fixes six identities, corpus roles, reporting domains, circuit roles, safety
impacts, and paths.

The author must:

1. verify `AUTHOR_N_PACKET.sha256` completely before reading the assignment;
2. verify `CONTRACT_BINDING.json` names the committed V8 contract freeze;
3. read `PUBLIC_REQUIREMENT_CONTRACT.md` and `CORPUS_RULES.md` completely;
4. create exactly the six JSON files named by the sole assignment;
5. preserve every packet input byte-for-byte;
6. instantiate `AUTHORSHIP_TEMPLATE.json` as `AUTHORSHIP.json`, replacing every
   bracketed value truthfully and recording exactly six ordered source hashes;
7. run no KiCadAI synthesis, search, simulation, classifier, feasibility,
   ranking, or outcome tool;
8. return only `AUTHORSHIP.json` and the six requirement files in the supplied
   empty quarantine; and
9. disclose uncertainty in `AUTHORSHIP.json` instead of guessing.

Project names, descriptions, ports, cases, assertions, and electrical behavior
must be independently conceived. Manifest-only `v8_case_*` and `v8_source_*`
identities must not occur inside requirement files. Authors do not create
obligation-anchor or causal-path hashes; the publisher derives them later.

Any input not named by the supplied per-author checksum invalidates isolation
and requires a fresh author context and entirely fresh requirements.
