# V4 Isolated Corpus Authoring Packet

This directory is the complete and only input permitted for the independent V4
corpus author. Do not provide repository access, prior corpus files, circuit
examples, implementation code, synthesis results, diagnostics, rankings,
expected outcomes, or conversation history.

The author must:

1. read `PUBLIC_REQUIREMENT_CONTRACT.md` and `CORPUS_RULES.md`;
2. create exactly the 24 JSON files named by `AUTHOR_MANIFEST.json`;
3. preserve `AUTHOR_MANIFEST.json` byte-for-byte, including fixed role,
   reporting-domain, safety-impact, identity, and path assignments;
4. copy `AUTHORSHIP_TEMPLATE.md` to an output file named `AUTHORSHIP.md` and
   replace every bracketed field with truthful provenance and isolation
   attestations without modifying this input packet;
5. run no KiCadAI synthesis, search, simulation, classifier, feasibility, or
   outcome tool;
6. return `AUTHOR_MANIFEST.json`, `AUTHORSHIP.md`, and the two requirement
   directories as one quarantine bundle; and
7. disclose uncertainty in `AUTHORSHIP.md` rather than guessing an
   implementation or outcome.

Project names, titles, descriptions, ports, operating cases, assertions, and
electrical behavior must be independently conceived. `v4_case_*` and
`v4_source_*` are manifest-only identities and must not occur inside requirement
files.

`PACKET.sha256` freezes every author input. Any additional input invalidates the
isolation claim and requires a fresh author/context and fresh requirements.
