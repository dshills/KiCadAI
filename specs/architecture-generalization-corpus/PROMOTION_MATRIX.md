# Architecture Generalization Promotion Matrix

## Result

Five of the six frozen design requirements complete deterministic electrical
search and two identical installed-KiCad physical promotions. The remaining
protected current-output requirement fails closed before physical lowering
with stable evidence.

| Requirement | Outcome | Synthesis hash | Topology hash | Physical hash | Project hash | Promotion evidence hash |
| --- | --- | --- | --- | --- | --- | --- |
| Dual-threshold voltage-window indication | pass | `b2d967e54a1a1f010192ef5b4e04135967cf15c8f746283b724fc036ca6be0d5` | `3950f517486c8b5694ff0f783ce09293ef04507983fcdc11d52d4249b9a36d4d` | `d4938b3c557bff6a22a2523f5d946661b731bd39f03c89ab56203dd79ad9cef2` | `e25cfcdf37c9b43267bf050455988af622da3e426430c606f0d0965e802f2b4f` | `c25ce550c96b54a624e881cc19264f5c5dfdb579be9e8678fd6bf3b6d298bfe9` |
| Low-current-to-voltage conversion | pass | `7dd2009d866592336bc04db1fd7d84683487f45499fb5592cff95e8c603d8d36` | `dc5e1f0947f7774d2c23726de99930768866f339bc87f6609e9b7634faebbf81` | `620d01afff932f24dec054ea26508744130ab3c02724aff2dc5b721d69f36b70` | `012ad983ad07c04c1e4113bcefb003be024778595f1ca55e5a8e088e76d064b1` | `b96f709229f879869985f38dc7bdcca702b235b6dcf765b99db9f6fa8d7b340f` |
| Low-level full-wave transfer | pass | `69e002c8674945eaa5814cadb8c805079e1cbccb3f2e3f5ddc42491927a68cd6` | `cbcf4092eeec0d99828548be844619537288e9b98d67783c923e4f26433261b7` | `d8f1a25082f475f96981c8ac5457c5a093894a010d8b389fc8784af928c80783` | `a0496a676ae39919208573f78c76aba321589535518c652e12c3651e26c2c06f` | `4c7c77d669eba71a64135e25b752d16d9e1f90f31379c0ce34e10c5e3c3dafc9` |
| Regulated low-voltage output | pass | `3aa32e20332e09f041cfb13218b4133a07b62b7e86e996e428820436a9359413` | `92263c2b17de4f91631bebb65963537ef85cb9dd5296adb8a4cf66481df82b54` | `5cc82f8b8bf89db7656fc82a1b956c4938cc1505a07b89a1566ad6cb170b3075` | `79da92b06f33d47edf549077bb3cf6dea5f65980ec4fdf6718e814153ecb57a1` | `a556dbf5431d5ea803825d1a47d8a645d1a12de263d6eb3aa41b16a4dcace7a9` |
| Frequency-selective midband transfer | pass | `8dbed54e98886a9dea60fa292bf5a9e6f26b22f41e5935dc4d7b4e8931c09df5` | `ae22d39a8fd81cf128da13c251aadd769d1c26f077f3904d835cf5ca4bfc5a61` | `2071332f58c586a7e041b4045622858122ce40c8582908ffc314643ab608fa40` | `dc4ad65b755feeb0358d9656f148afd8961a53398ce752758b6f0790f055533d` | `84e17bcddf21027ebebf8df54bb70c77794936f78ebfb07959d28b22e8c9888e` |
| Protected programmable current output | deterministic fail closed | `596f627523ace41257a6f7ec3fcaed54914c66ad5ee6563168a228d8d23663b5` | n/a | n/a | n/a | same as synthesis result |

Every passing row includes clean connectivity and route-completion checks,
writer correctness, installed-KiCad ERC and strict DRC, zero normalized
round-trip differences, and identical second-run hashes. Electrical acceptance
also requires at least two evaluated topology hashes and an explainable,
deterministically ranked winner.

## Adversarial Rejection Evidence

| Requirement | Outcome | Stable evidence hash |
| --- | --- | --- |
| Inadequate safe operating area | fail closed | `5733d410ee2d7dee7464acb26ac69b86802f03da2eebaa591d6d954a2aa81773` |
| Inconsistent standing current / invalid bias | fail closed | `ba99fe661b306212bf6bf33c1f06e42b05359e97b3cb10469ce35a06be5b0e5f` |
| Unsafe dissipation | fail closed | `d47b8055dd0a548bd5cac1429e524e2d9d27326da4402699411971b28938b1c6` |
| Unstable dynamic envelope | fail closed | `87cd7fd197dae5c4701942d1824a5aa868a6251512ebe3021c67070373de1725` |

All four adversarial results reproduce byte-identically on a second run and
emit no executable physical project.

## Reproduction

```sh
KICADAI_VERIFY_ARCHITECTURE_GENERALIZATION=1 go test ./internal/opentopologysynthesis -run '^TestArchitectureGeneralizationAcceptance$' -count=1 -timeout 20m -v
KICADAI_OPEN_TOPOLOGY_KICAD_PROMOTION=1 go test ./internal/opentopologysynthesis -run '^TestArchitectureGeneralizationCorpusOptionalKiCadPromotion$' -count=1 -timeout 70m -v
```
