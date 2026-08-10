# V5 Corpus Authorship Record

- Author/context identity: [record opaque context identity]
- Author slot: [record the exact assignment author_slot]
- Authoring tool/model and exact version: [record exact identity]
- Authoring start/end in UTC: [record timestamps]
- Per-author packet manifest: [record exact AUTHOR_N_PACKET.sha256 filename]
- Per-author packet SHA-256: [record SHA-256 of that manifest]
- Contract binding SHA-256: [record SHA-256 of CONTRACT_BINDING.json]
- Assignment SHA-256: [record SHA-256 of the supplied assignment]
- Returned bundle root: [record quarantine bundle name]
- Requirement source SHA-256 values: [record every assigned path and the
  lowercase hexadecimal SHA-256 of that returned JSON file's full exact bytes,
  computed after authoring and before normalization or reformatting]

I attest that:

- the files frozen by the recorded per-author packet manifest were my only task
  inputs;
- `CONTRACT_BINDING.json` named the frozen V5 contract before authoring began;
- I had no repository, previous corpus, implementation, circuit example,
  conversation, baseline, selection, diagnostic, or outcome access;
- I did not see another author's assignment or requirement content;
- I independently conceived all twelve behavior-only requirements;
- I did not synthesize, simulate with KiCadAI, classify, test feasibility, or
  predict support;
- discovery and held-out membership remained fixed throughout authoring;
- no requirement prescribes an implementation or expected result;
- I did not inspect or modify a requirement after seeing an evaluation; and
- I have disclosed every uncertainty here: [record uncertainties or `none`].

Signed/attested by: [opaque author/context identity]
