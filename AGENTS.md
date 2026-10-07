# Mutti development

- Preserve upstream API compatibility, file-specific licenses and contributor history.
- Read the product plan and status in the Mutti server repository before changing scope.
- Day 1 must never silently use DERP, TURN, peer relays or plaintext remote fallback.
- Do not treat Jellyfin Quick Connect as cryptographic network enrollment.
- Changes belong on `codex/` or `claude/` task branches. Keep the upstream diff small.
- Never use real libraries or existing Jellyfin databases for destructive tests.
- The first iteration ships Mac-only (owner decision of 6 October 2026, see docs/mutti/plan.md). The Docker package stays buildable and CI-checked as a developer path, not as a release gate. Local development is not remote readiness.
- No central relay, ever: a Tailscale wiki article is the documented fallback for networks without a direct path.
