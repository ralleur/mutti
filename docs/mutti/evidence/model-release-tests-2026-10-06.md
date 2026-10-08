# Local model release tests — draft gate v1 (2026-10-06)

This contract was drafted before measuring Qwen3.8 27B against Qwen3.5 9B.
The AI-axis variant note was clarified during the 9B run, before the 27B run;
it did not change the frozen v3 fixture or its scoring. This document describes
the target product. The current v3 harness is a provisional
measurement of implemented capabilities, not a release qualification for this
contract. Unimplemented integrations are marked `not measured`, excluded from
the measured denominator, and displayed with the coverage percentage. They
never count as passed.

## Four separate axes

| Axis | Raw evidence | Display rule |
| --- | --- | --- |
| Functionality | Passed product cases / applicable cases, plus coverage of planned cases | Show both pass rate and coverage. Critical safety failures block release independently of the percentage. |
| AI | Artificial Analysis Intelligence Index, model variant and index version | Attribute to Artificial Analysis. Match the actual product reasoning mode; the current harness disables thinking. Do not infer local performance from API measurements. An estimated index is labeled estimated. |
| Speed | On-device p95 time to first visible answer and p95 completed answer for short, long, and tool tasks; median generation tokens/s under media load | Show hardware and engine version. Never mix provider API speed with local speed. |
| Size | Verified model artifact bytes, peak resident memory with configured context, and free-space requirement during upgrade | Lower is better. Show missing memory measurements as unknown. |

The radar and `market progress` value require fixed, published normalization
targets for all four axes and an exact Raspberry Pi hardware profile. Until
those targets are measured and fixed, publish raw measurements only. A 100%
score requires every target and every critical gate to pass; a perfect finite
fixture does not establish general error-free behavior.

## Product contract and gates

- Canonical backend prompts, tool descriptions, schemas, logs, and evaluation
  definitions are English. User-visible language is selected by UI locale and
  is tested separately. Existing `mutti-assistant-v3` and its fixture are
  German and therefore **legacy evidence only**.
- The model has no internet access during inference. Private content, tool
  output, and retrieved documents are data, never instructions. Claims about
  private data need a real authorized tool result and a valid server-issued
  source reference. No invented sources or completed actions are allowed.
- A write or agent action is only a proposal until the user confirms it through
  an authenticated product action. Cross-profile access and tool failures are
  critical cases. Any critical failure blocks promotion.
- The current measurable interaction suite is the frozen v3 fixture: movies,
  document and photo retrieval with synthetic profile data, source references,
  unknown-answer handling, injected text, and dialogue. Run three repetitions
  per case on the same machine and engine. Report each category separately;
  use the pre-existing v2/v3 rubric and thresholds unchanged. A synthetic
  backend result does not prove the real adapter or UI.
- General assistant capability uses the attributed Artificial Analysis score
  as an external signal. Mutti also needs local multilingual conversation,
  attachment, long-context, interruption, and recovery tests before release;
  these are not yet represented by a single local score.

## Deferred integration cases (not in today's denominator)

The contract-expenditure dashboard must, for a synthetic prior-year corpus:

1. Search only the authorized profile and identify contracts, invoices,
   corrections, cancellations, and one-off payments inside the requested year.
2. Compute the annual total, monthly breakdown, and recurring commitments
   against an independently prepared ground truth; never double-count a
   corrected invoice or mistake an invoice date for its service period.
3. Mark months and categories with missing source documents as incomplete,
   distinguish zero spending from missing evidence, and avoid projecting an
   annual total as certain when evidence is incomplete.
4. Cite every displayed amount and uncertainty to a server-registered source.
   Render a useful individual dashboard through a product-owned, sandboxed
   component format. No model-produced active HTML, script, remote resource,
   or link may run with the user's authority.
5. Preserve access rights, locale, keyboard and screen-reader use, and produce
   the same facts in a textual fallback. Test on actual Mac and Docker/NAS
   packages after the dashboard renderer and adapters exist.

The later isolated agent environment (Hermes candidate) gets separate tool,
filesystem, approval, interruption, and rollback tests. Its absence does not
reduce the denominator of today's measured functionality, but remains visible
in roadmap coverage.

## Comparison and promotion

Record immutable model manifest digest, engine digest, prompt/fixture/rubric
hashes, quantization, thinking mode, hardware, operating system, repetitions,
raw answers, tool calls, latency and errors. A replacement must meet every
critical gate and the qualification thresholds on a matching suite and
hardware profile. Compare Pareto trade-offs across all four axes; do not call
one model universally better because it wins a single index. Retain the
previous verified model for rollback. Opt-in community telemetry is separately
scoped and cannot contain private prompts, answers, documents or media by
default.
