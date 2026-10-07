# Mutti development

- Preserve upstream API compatibility, file-specific licenses and contributor history.
- Read the product plan and status in the Mutti server repository before changing scope.
- Day 1 must never silently use DERP, TURN, peer relays or plaintext remote fallback.
- Do not treat Jellyfin Quick Connect as cryptographic network enrollment.
- Changes belong on `codex/` or `claude/` task branches. Keep the upstream diff small.
- Never use real libraries or existing Jellyfin databases for destructive tests.
- The first iteration ships Mac-only (owner decision of 6 October 2026, see docs/mutti/plan.md). The Docker package stays buildable and CI-checked as a developer path, not as a release gate. Local development is not remote readiness.
- No central relay, ever: a Tailscale wiki article is the documented fallback for networks without a direct path.

## Rights, contributions and new dependencies

- Before new or imported dependencies, copied code, fonts, media or binaries read `RIGHTS.md` and `docs/mutti/licensing.md`.
- Keep Mutti's own work, the Jellyfin foundation and third-party code separately evidenced. A CLA transfers no third-party rights; earlier contributions are not retroactively signed. Keep existing licence and copyright notices.
- Every new or changed dependency, including transitive lockfile and fetch-script changes, needs a pinned hash in `mutti/components.lock.json` and a row in `mutti/THIRD-PARTY.md`; do not update hashes merely to make a check pass. Document unresolved distribution obligations as release blockers, never as cleared.
- New external original contributions need the actual CLA acceptance on the pull request; never invent signatures or rights confirmations for others. Disclose AI-assisted work in commit messages.
