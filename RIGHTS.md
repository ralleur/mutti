# Rights, contributions and dependencies

Mutti is open source. Its own source files are licensed GPL-2.0-or-later
(server-side additions, manager, Mac shell, packaging) and MPL-2.0 (Connect
transport and Apple bridge); the combined package is offered under the GNU GPL
version 3 as recorded in [docs/mutti/licensing.md](docs/mutti/licensing.md).
This policy does not replace those licences or revoke earlier grants.

Ralf Hauser maintains Mutti. He holds the economic rights in the original Mutti
work created by him or under his direction, the rights validly granted to him
under the [CLA](CLA.md), and the project assets he controls (name, wordmark,
symbol, app icon, release records). These rights are transferable as a whole to
successors and assigns. This does not make him the owner of Jellyfin, Jellyfin
Web, Intro Skipper, FFmpeg, other third-party components or purely
machine-generated material. Existing recipients retain their open-source
permissions. A transfer of the project preserves every applicable licence
obligation; this repository is not a promise of unrestricted proprietary use of
the combined work.

## What belongs to whom

| Material | Rights and evidence |
| --- | --- |
| Jellyfin server and web foundation, including modified upstream files | Respective authors under the GPL; history records provenance. Mutti changes do not erase upstream rights. |
| Original Mutti work under `mutti/`, `docs/mutti/`, `Jellyfin.Server/Mutti/` and `tests/Jellyfin.Server.Tests/Mutti/` | Ralf's rights, to the extent they exist; distributed under the SPDX licence of each file. File location or a commit author alone is not proof of exclusive ownership. AI-assisted work is disclosed in commit trailers and may be only partly protectable. |
| New original contributions accepted under the Mutti CLA | Contributor retains authorship; Ralf receives the additional non-exclusive, transferable and sublicensable rights in [CLA.md](CLA.md). Retain acceptance evidence. |
| Earlier contributions and imported upstream changes | Their existing terms. No retrospective CLA grant is assumed. Importing, merging or signing on somebody else's behalf does not create additional rights. |
| Dependencies, copied code, fonts, media and bundled binaries | Their own licences and notices, recorded in [mutti/THIRD-PARTY.md](mutti/THIRD-PARTY.md), [mutti/components.lock.json](mutti/components.lock.json) and [docs/mutti/licensing.md](docs/mutti/licensing.md). |
| Product identity | [TRADEMARKS.md](TRADEMARKS.md). Brand policy does not supersede previously granted copyright permissions. |

Git history, SPDX headers, licence texts, the component lock, documented
authorship and actual CLA acceptances together form the evidence. No blanket
copyright replacement, mass header rewrite or assertion that a directory is
exclusively owned is permitted. Record copied material even when its licence is
permissive. Disclose AI-assisted work and known sources; do not invent human
authorship or promise exclusive rights in output that may not be protected.

## Every new or changed dependency

1. Identify the exact upstream, version or commit, direct and transitive
   inputs, licence texts and copyright notices. Include native binaries,
   downloaded build artifacts, fonts, assets, snippets and build-only tools.
2. Record how it is used: modified or unmodified, linked or separate, shipped
   or development-only. Assess source delivery, attribution, copyleft, patent,
   trademark and redistribution obligations for the actual build.
3. Explain which rights remain with third parties and how a transfer of the
   project would preserve those obligations. An SPDX label or CLA alone is not a
   compatibility decision. New restrictions require an explicit decision.
4. Pin the input in `mutti/components.lock.json` with its hash, add the row to
   `mutti/THIRD-PARTY.md` and, where a question is open, to
   `docs/mutti/licensing.md`. Bundle the notices and matching sources the
   build needs (`mutti/packaging/`).
5. Unresolved distribution obligations block packaging and release. A
   repository-wide rights scanner as in kurtz is planned; until then the
   maintainer reviews every manifest, lockfile, fetch script and binary change
   in review.

Open-source dependencies are welcome. They are not automatically relicensable
by Ralf. Unknown or conflicting terms must be resolved, the component replaced,
or its use confined to an explicitly documented development scope.

## Contributions and enforcement

New original external contributions require [CLA v1](CLA.md) acceptance before
merge. Ralf does not need to grant rights to himself. Upstream imports retain
their authors and licences and require provenance review; they must not be
labelled as CLA-covered original contributions. Do not evade the check by
squashing away authors or changing author metadata. Automated updates do not
grant rights in the packages they introduce.

The `license/cla` status from `.github/workflows/cla.yml` records acceptances
on the `cla-signatures` branch. GitHub must require it on the default branch;
that activation is a repository setting, not a file in this checkout.

## Legal references

- [GPL-2.0 sections 2 and 3](https://www.gnu.org/licenses/old-licenses/gpl-2.0.html) and [GPL-3.0 sections 5 and 6](https://www.gnu.org/licenses/gpl-3.0.html): modified source, combined works and source delivery.
- [MPL-2.0 sections 2 and 3](https://www.mozilla.org/en-US/MPL/2.0/): grants, notices, modified source and larger works.
- [UrhG §29](https://www.gesetze-im-internet.de/urhg/__29.html) and [§31](https://www.gesetze-im-internet.de/urhg/__31.html): authorship and rights of use.
- [kurtz RIGHTS.md](https://github.com/ralleur/kurtz/blob/kurtz/RIGHTS.md): the sibling policy this one follows.
