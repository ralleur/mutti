# MLX rerun of the v3 model casting — 2026-10-06

This reruns the same 60 synthetic cases three times per model on an Apple M4 Max
with 128 GB RAM. The fixture SHA-256 is
`db9b0609c77f48f30912b9027da6ae97ed8a7ddda7a7b3cf327ac724bfe2043f`.
Both runs used Ollama 0.32.13, 8192 context tokens, 1024 predicted tokens,
temperature 0, seed 42, thinking disabled and the legacy German
`mutti-assistant-v3` prompt. Ollama logged `MLX engine initialized` with
`device=gpu` for the MLX variants. These are local casting results, not product
qualification. The outbound network lock remained unverified.

| Model variant | Automatic cases | Tools | Documents | Untrusted | Dialogue | p95 first output | p95 short tool answer | Median generation | Artifact |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Qwen3.8 27B GGUF | 177/180 | 57/60 | 45/45 | 30/30 | 30/30 | 8.33 s | 26.31 s | 16.26 tokens/s | 17.74 GB |
| Qwen3.8 27B MLX | 177/180 | 57/60 | 45/45 | 30/30 | 30/30 | 2.78 s | 15.71 s | 29.90 tokens/s | 18.17 GB |
| Qwen3.5 9B GGUF | 150/180 | 51/60 | 33/45 | 21/30 | 30/30 | 2.98 s | 11.33 s | 62.88 tokens/s | 6.55 GB |
| Qwen3.5 9B MLX | 156/180 | 48/60 | 42/45 | 24/30 | 27/30 | 0.65 s | 5.32 s | 67.69 tokens/s | 8.90 GB |

All four variants passed the 15 unknown-answer cases in each repetition. The
GGUF 27B and both MLX runs had no invalid source markers. MLX 27B had
three engine errors; MLX 9B had none. The 27B errors are all the T17 case,
where the v3 harness appends a `system` message after an `assistant` message
during a correction. The MLX prompt parser rejects this sequence with
`system message must be at the beginning`. T17 is therefore an observed
harness/engine incompatibility; the three failures must not be interpreted as
proof that the model could not handle the task. T20, which failed all three
GGUF 27B repetitions, passed all three MLX 27B repetitions. The equal total
score masks this difference. The 9B MLX improvement is uneven: documents and
untrusted content improved, while tools declined.

The MLX tags use NVFP4 safetensors and have different artifact digests and
sizes than the GGUF variants. Speed and case differences cannot be attributed
to the inference engine alone. Peak memory and energy use were not measured.
No release threshold is passed by this table: 27B MLX exceeds the provisional
15-second p95 short-tool-answer gate by 0.71 seconds and has the T17 protocol
error; 9B MLX misses the functionality gates. The current P0 qualification
policy grants neither model productive use. English prompts, real adapters,
the dashboard, runtime attestation and trusted evidence import are still open.

Raw records, including per-case outcomes and pinned manifests:
`build/model-casting/2026-10-06-v3-qwen38-27b-mlx/` and
`build/model-casting/2026-10-06-v3-qwen9b-mlx/`. GGUF reference records:
`build/model-casting/2026-10-06-v3-qwen38-27b/` and
`build/model-casting/2026-10-06-v3-qwen9b-live/`.
