# Contributing

Use a `codex/` or `claude/` task branch based on the Mutti product branch.
Explain the user-visible behaviour, upstream differences, tests and remaining
release limitations in the pull request. Keep third-party attributions. Report
security issues through SECURITY.md. Do not automatically publish packages,
migrate user databases or enable relays. The server repository tracks the
product plan and platform acceptance matrix in `docs/mutti/`.

## Rights and dependencies

- New original external contributions require acceptance of the
  [Mutti Contributor License Agreement v1](CLA.md) on the pull request; the
  `license/cla` status must pass before merge. Upstream imports and third-party
  material are not covered by the CLA and must be identified separately.
- Read [RIGHTS.md](RIGHTS.md) before adding or changing dependencies, copied
  code, fonts, media or binaries. Pin inputs in `mutti/components.lock.json`,
  record them in `mutti/THIRD-PARTY.md` and bundle the required notices.
- Keep existing licence and copyright notices. Disclose AI-assisted work in
  the commit message. Never rewrite author metadata or squash away authors.
- The product identity is covered by [TRADEMARKS.md](TRADEMARKS.md).
