# Review for signing: AI qualification, package 7a036fc459 (07.10.2026)

Status: **ready for owner review; nothing is signed.** Signing grants the
local AI to this exact deployment only and is the owner's decision.

## What would be granted

Twelve records (German and English × content.assist, media.search,
media.read, media.favorite, documents.read, photos.search) for exactly:

| Binding | Value |
| --- | --- |
| Model | `qwen3.8:27b-mlx`, manifest `5642e97495e1…` |
| Engine | bundled Ollama 0.32.13 incl. MLX, directory `9dc018e018b0…`, network lock verified |
| Hardware / OS | `Mac16,9` (Apple M4 Max, 128 GB), macOS 27.0 (26A428) |
| Hub | `812a518260fd…` (reproducible from commit `7a036fc459`) |
| Contract | `mutti-assistant-v6`, per language (DE `dc6dc5fbb750…`, EN `157fc5bdc959…`) |
| Settings | context 8192, temperature 0, thinking off |
| Validity | 180 days (until 2027-04-05); any change of the above denies again |

## Evidence

- Gates real-de-v6 and real-en-v4 (fresh, frozen before their run): 84/84
  each, 0 critical, unknown-answer cases 6/6, playback unaffected under AI
  load. Details: [measurements](qualification-v6-2026-10-06.md),
  [protocol](qualification-v6-protocol.md).
- History today: real-de-v4, real-en-v2, real-en-v3 were seen and used for
  fixes (reasoning-tag leaks, compound-word and photo word/stem search,
  multilingual search terms); real-de-v5 passed on an earlier package.
- v6 holdout once: 129/132 (one scorer flaw, fixed). Dev regression 60/60.

## Limits to weigh

- Synthetic test data on one Mac; no real user data was used.
- The gate suites were written by the development agent (as all earlier
  suites); they were frozen before their run and not used for tuning.
- Test-data quirks: the English skateboard and German bike video are one
  deduplicated asset; Alpha's photos include untitled photos of earlier
  native tests.
- Known product behaviour not gated: the model sometimes narrates its tool
  use ("Let me search …") in the final text.
- A macOS update, a new hub build or another Mac requires a new measurement.

## Signing (owner)

From the repository root, with the existing qualification key:

```
S=build/qualification-session-20261007-0901
(cd mutti/hub && go run ./cmd/mutti-release sign \
  --key "$HOME/Library/Application Support/Mutti/release-keys/<qualification key file>" \
  --id mutti-qualification-2026-10 \
  --candidates ../../$S/qualification-real-de-v6/result/candidates.json,../../$S/qualification-real-en-v4/result/candidates.json \
  --out ../packaging/qualification/records.json)
(cd mutti/hub && go run ./cmd/mutti-release verify --file ../packaging/qualification/records.json)
```

`verify` must list the twelve records. The combining step lives in
`mutti-release` (not shipped): the hub built from the current branch is
still bit-identical to the measured one (`812a518260fd…`, checked). A dry
run with a throwaway, untrusted key produced the twelve records and was
rejected by the hub as expected.

After signing: rebuild the package with `records.json` (hub digest
unchanged), run `module-package-smoke.py --model qwen3.8:27b-mlx
--model-source adopt --adopt-from … --expect-qualified`, then the native AI
check in kurtz.
