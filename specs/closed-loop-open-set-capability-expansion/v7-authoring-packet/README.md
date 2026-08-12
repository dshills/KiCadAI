# V7 Isolated Corpus Author Packet

This packet is the complete and only input permitted for one independent V7
corpus author. It contains exactly one file below `assignments/`; that file
fixes twelve identities, roles, reporting domains, safety impacts, and paths.

The author must:

1. verify `AUTHOR_N_PACKET.sha256` before reading the assignment;
2. verify `CONTRACT_BINDING.json` names the committed V7 contract freeze;
3. read `PUBLIC_REQUIREMENT_CONTRACT.md` and `CORPUS_RULES.md` completely;
4. create exactly the twelve JSON files named by the sole assignment;
5. preserve every packet input byte-for-byte;
6. instantiate `AUTHORSHIP_TEMPLATE.json` as `AUTHORSHIP.json`, replacing every
   bracketed value truthfully and recording exactly twelve ordered source hashes;
7. run no KiCadAI synthesis, search, simulation, classifier, feasibility, or
   outcome tool;
8. return only `AUTHORSHIP.json` and the twelve requirement files in the
   supplied empty quarantine; and
9. disclose uncertainty in `AUTHORSHIP.json` instead of guessing.

Project names, descriptions, ports, cases, assertions, and electrical behavior
must be independently conceived. Manifest-only `v7_case_*` and `v7_source_*`
identities must not occur inside requirement files.

Any input not named by the supplied per-author checksum invalidates isolation
and requires a fresh author context and fresh requirements.
