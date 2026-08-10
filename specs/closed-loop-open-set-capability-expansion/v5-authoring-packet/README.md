# V5 Isolated Corpus Author Packet

This packet is the complete and only input permitted for one independent V5
corpus author. It contains exactly one file below `assignments/`; that file
fixes the author's twelve identities, roles, reporting domains, safety impacts,
and output paths.

The author must:

1. verify the supplied `AUTHOR_N_PACKET.sha256` before reading the assignment;
2. verify that `CONTRACT_BINDING.json` names the committed V5 contract freeze;
3. read `PUBLIC_REQUIREMENT_CONTRACT.md` and `CORPUS_RULES.md`;
4. create exactly the twelve JSON files named by the sole assignment file;
5. preserve the assignment byte-for-byte;
6. copy `AUTHORSHIP_TEMPLATE.md` to an output file named `AUTHORSHIP.md` and
   replace every bracketed field with truthful provenance and isolation
   attestations;
7. run no KiCadAI synthesis, search, simulation, classifier, feasibility, or
   outcome tool;
8. return `AUTHORSHIP.md` and the twelve requirement files as one quarantine
   bundle; and
9. disclose uncertainty in `AUTHORSHIP.md` instead of guessing an
   implementation or outcome.

Project names, titles, descriptions, ports, operating cases, assertions, and
electrical behavior must be independently conceived. Manifest-only `v5_case_*`
and `v5_source_*` identities must not occur inside requirement files.

Any input not named by the supplied per-author checksum invalidates the
isolation claim and requires a fresh author context and fresh requirements.
