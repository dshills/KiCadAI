# V3 Isolated Corpus Authoring Packet

This directory is the complete input permitted for the independent V3 corpus
author. Do not provide the author repository access, prior corpus files,
synthesis results, implementation code, examples, or conversation history.

The author must:

1. read `PUBLIC_REQUIREMENT_CONTRACT.md` and `CORPUS_RULES.md`;
2. create exactly the 24 requirement files named by `AUTHOR_MANIFEST.json`;
3. preserve the manifest bytes, including fixed role/domain/safety assignments;
4. replace bracketed fields in `AUTHORSHIP.md` with truthful provenance and
   isolation attestations;
5. run no KiCadAI synthesis, search, simulation, classifier, or outcome tool;
6. return the manifest, authorship record, and requirement directories as one
   quarantine bundle; and
7. disclose any uncertainty in the authorship record without guessing an
   implementation.

Project names, titles, descriptions, operating-case IDs, assertion IDs, and
electrical behavior must be independently conceived. `v3_case_*` and
`v3_source_*` are manifest-only identities and must not appear inside a
requirement file.

The packet checksum is recorded in `PACKET.sha256`. Any input outside this
packet invalidates the isolation claim and requires a new author/context.
