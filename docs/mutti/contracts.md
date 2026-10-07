# Mutti content contracts — v1

Status: accepted design baseline, 2026-10-06. Canonical backend terminology is
English. This document specifies the independent Mutti/kurtz architecture; it
contains no Umbrel implementation. Field names below are contracts for upcoming
work, not claims that every field or endpoint already exists.

Implementation and evidence: [baseline](baseline-2026-10-06.md),
[P0 checks](evidence/p0-2026-10-06.md). Delivery order: [product plan](plan.md#16-integrierter-ausbauplan-p0p4).

## Product and authority boundary

Mutti is the user's central content store. kurtz is the normal entry point to
browse, find and work with that content. Internal service names are reserved for
administration, diagnostics and deliberate service choices. The existing normal
Jellyfin client mode remains compatible. Mutti's logical content layer does not
write directly into backend databases or create a second authoritative library.

Three interfaces stay distinct:

- Content API: authenticated device/profile operations over existing Connect.
- Administration: owner-only, fixed local management routes; no delegation from
  the content assistant, no generic URL proxy, shell or container control tool.
- Qualification: trusted, reviewed deployment evidence, never an owner toggle,
  client claim, model output or restored conversation field.

## Identity and authorization

`AccessContext`: `muttiId`, `profileId`, `deviceId`, `sessionId`, `rightsRevision`.
All values come from the authenticated server boundary, never request identity
headers supplied by kurtz. Existing Jellyfin IDs may be mapped initially.
Backend account IDs and credentials stay in server-side mappings.

Effective access is the intersection of device grant, profile policy, object
permission and requested operation. Search counts, thumbnails, excerpts,
attachments, sources and model context are covered, not just original downloads.
No index or cached capability is an authorization authority. Recheck before
execution and source opening; revocation invalidates future history reuse too.
The target cancellation budget for in-flight work is at most five seconds;
measure it on the Mac package (Day 1 is Mac only; Docker/NAS is a roadmap item). Previously delivered bytes cannot be recalled.

Pairing binds one expiring invitation, server key, proven device key and owner-
approved profile. Persist keys, not network addresses, as identity. QR codes do
not contain permanent backend credentials. Local discovery/signalling supplies
candidates only. TLS identity verification still applies. Quick Connect is not
network enrollment. Day 1 has no DERP, TURN, peer relay or plaintext fallback.

Restore must not reactivate revoked device keys. Current safe behavior remains
explicit re-pairing; identity continuity/revocation epochs need a separate design
and test before enabling transparent identity restore.

## Content reference and provenance

`ContentRef`: `muttiId`, `contentId`, `revision`. IDs are opaque, stable and never
reused for another object. Server mapping includes backend instance identity and
backend object ID; an ID reused by a replaced backend is a different object.
A source fingerprint/checksum can support reconciliation but does not merge
ownership or duplicates automatically. Revision changes when relevant content
or metadata changes; missing reliable backend revisions must be represented as
unknown, not guessed. Revision-dependent writes are denied until a safe strategy
is implemented.

`ContentItem`: reference, `kind` (movie, series, photo, video, document, attachment),
`title`, timestamps with meaning/timezone, allowed `actions`, `availability`
(ready, processing, unavailable, removed), preview reference and provenance.
A transient attachment is not an archived document. Generic file storage is an
open scope decision; do not advertise it as implemented.

`SourceRef`: conversation/run-scoped marker, ContentRef, `locator` (page, time
range or text span), extraction version, retrieval time and optional content
hash. AI sees only server-issued markers. The server, not the model, supplies
links. Source opening reauthorizes the original and reports changed/unavailable
content. Missing page accuracy is explicit. Derived summaries and dashboards
retain their source dependencies; every factual amount must have evidence.
Current Q markers are a useful starting point but lack full revision/locator
semantics. Existing response DTOs remain compatible until an additive migration.

**P1 implementation (2026-10-06):** `content-ids.json` maps random 32-hex
content IDs to (area, backend instance, object). `muttiId`, `mediaInstance` and
a per-module `instance` live in `hub.json`; re-pointing a module to another
service rotates its instance, so earlier IDs and sources stop resolving.
Revisions are hashes of Jellyfin `Etag`, Immich `updatedAt` and Paperless
`modified`; `null` when absent. Sources touched by a run carry `content`,
`retrieved` and `locator` (`scope: object` — no page/time accuracy yet).
`GET ai/sources/{conversation}/{ref}` re-authorizes and adds `status`
(current, changed, unknown) plus the current item. Answers whose touched or
cited sources are no longer authorized (or not verifiable) are replaced by a
neutral placeholder in model history. The media area keeps the Jellyfin item ID
as `media.itemId` so the native player opens it (initial mapping as allowed
above); photo/document items carry the existing profile-visible DTOs.

## Federated search

`SearchRequest`: query, content-kind/date filters, bounded page size and opaque
cursor. `SearchPage`: authorized ContentItems, next cursor, `completeness`
(complete, partial), and typed unavailable content areas. Never report an outage
as zero results. Cursor binds request, profile and rights revision; changing
rights invalidates it. Search each backend with the caller's internal account.
Do not compare unrelated relevance scores directly; start with deterministic
per-kind groups/date ordering and explicit coverage. Deduplicate only with
proven identity. Initial implementation is federation, not a new shared index.
Manual search remains usable independently of model qualification.

**P1 implementation:** `GET /mutti/hub/v1/content/search?q=&kinds=&from=&to=&size=&cursor=`
(kinds: movie, series, photo, video, document; dates are inclusive
`YYYY-MM-DD` on premiere/taken/created date). Areas `media`, `photos`,
`documents` are grouped in that order; inside a group the backend's order.
Area states: `searched`, `unavailable` (makes the page `partial`) and
`not_available` (module not granted/configured; does not reduce completeness).
Cursors are HMAC-signed with a per-process key, expire after 30 minutes and
bind query, profile and rights revision (hash of Jellyfin policy, device,
module state, grant, link and instance); mismatch is HTTP 409
`cursor_invalid`. No counts are returned. `GET content/items/{id}` re-
authorizes with the caller's backend account (`?revision=` adds
`revisionStatus`); `GET content/items/{id}/{thumbnail|preview|original|video}`
streams photos/documents and re-checks the object every four seconds, ending
the transfer when the backend revokes it. `GET content/jobs` lists the
profile's AI runs, document imports and journaled actions. Capabilities expose
a `content` module (search, open, jobs) independent of AI qualification.
Sharing in P1 is exactly what each backend shows the profile's own account; no
Mutti-level household or collection model (open decision 3 unchanged).

## Action and operation

`ActionProposal`: proposal ID, profile/device origin, task, exact target/revision,
explicit arguments, qualification binding, expiry and idempotency key.
State: pending → rejected | expired | executing → confirmed | failed |
outcome_unknown. Confirmation is an authenticated UI/API action. The model cannot
confirm, grant itself capabilities or claim completion.

At dispatch: reauthorize, verify the same qualification/configuration, check
revision and record execution intent durably before side effects. Serialize
conflicting operations. Read back results. A timeout or crash after a write is
outcome_unknown until reconciled; never blindly repeat. Exactly-once effects
across independent services are not promised without backend idempotency.
An old proposal without qualification provenance cannot acquire a new grant.
Manual user operations are outside the model gate but retain normal rights.

**P1 implementation:** `actions.json` journals each confirmation (proposal ID
as idempotency key, profile, device, task, target key/content ID, observed
revision, arguments, qualification) as `executing` before the write; a restart
turns open entries into `outcome_unknown`, and a later confirmation reports the
recorded outcome without repeating it. Operations on one target are serialized
across conversations. The target is re-authorized before the effect and read
back after it. Proposals carry `target` (ContentRef). Favorite set/unset is an
absolute per-user state, so a changed item revision is recorded but does not
block it; content-modifying actions still require a revision strategy first.
Exactly-once across a crash between write and journal update is not claimed:
that case is reported as `outcome_unknown`.

## Qualification contract and P0 enforcement

Task IDs now defined: `content.assist`, `media.search`, `media.read`,
`media.favorite`, `documents.read`, `photos.search`. Unknown tasks are denied.
`content.assist` covers conversation generation including history and attachments;
it does not implicitly grant tools. Each tool needs its own task grant. Source
resolution/manual content browsing do not invoke a model and keep their rights
checks without requiring a model grant.

Exact binding: model manifest digest (includes the model/quantization artifact),
engine artifact digest, hardware profile, OS build, actual prompt/tool-contract
hash, adapter artifact digest, context size, temperature and thinking mode.
A release record also needs a unique ID, task, passed status, evidence digest,
suite digest and expiry. Any absent, changed, expired, failed or unmeasured field
means deny. Critical security failures block the entire affected promotion,
regardless of aggregate score. RAM recommendations or catalog labels never
substitute for a passed task.

Implemented P0 boundaries:

- New messages and retries return HTTP 409 `qualification_required` before
  inference. Queued runs recheck before starting.
- Tools are filtered before advertising and checked again at dispatch.
- Confirmation rechecks the action grant; proposals carry its ID and binding
  hash. Missing/changed grants cannot execute, but rejection remains possible.
- Capabilities expose `qualification_required` instead of ready/chat/favorite.
- Production grants come only from the signed records file shipped in the app.
  No API, environment flag, hub.json field or model-selection action can add
  grants. Without a matching record every deployment is blocked, including
  conversational inference, because history and attachments may contain
  private content.
- Synthetic test grants exist only in `_test.go`. The isolated casting harness
  may exercise fake backends; this never qualifies a product deployment.

P2 (implemented, nothing promoted yet): the hub measures its own deployment —
engine directory digest (bundled engine incl. MLX kernels, links must stay
inside it, re-measured after every engine start), sandbox network lock, exact
Mac model/chip/cores/memory, macOS build and its own executable digest. Model
files are verified on disk: manifest digest equals the catalog digest and
every blob equals its content address, re-hashed when a file changes. An
external engine, an unverified model or an engine started since the last
measurement leaves the binding incomplete, so nothing matches. A run uses the
managed engine's address only and fails if thinking support cannot be
determined (the qualified setting is thinking off). Records are signed with
Ed25519 (trusted key IDs compiled in, keys outside the repository), can be
revoked and expire. The binding also contains the answer language: the model
contract is English (`mutti-assistant-v6`), German and English are answer-
language options, each qualified separately. Qualification validates exact
evidence matching for measured tasks; it is not a general proof of model
correctness. Protocol and measurements:
[qualification-v6-protocol](evidence/qualification-v6-protocol.md).

The German `mutti-assistant-v3` and its frozen casting suites remain unchanged
legacy measurement inputs. The current German product prompt is
`mutti-assistant-v5` (tool contract and bounded harness corrections, see
[casting-v4-rubric](evidence/casting-v4-rubric.md) and
[casting-v5-rubric](evidence/casting-v5-rubric.md)); each version changes the
qualification fingerprint, so no earlier evidence applies to it. English production prompts/tools and localized output
require a new version and fresh qualification, never relabel old measurements.
Use [release-test draft](evidence/model-release-tests-2026-10-06.md) for four-axis
measurement and the deferred expenditure dashboard. No current model is promoted.

## Compatibility and operational contracts

Existing `/mutti/hub/v1` routes remain. Qualification adds a stable error/state and
proposal provenance field.

User-facing texts are German or English per request: an explicit `language`
(query or message field) wins, then the first supported `Accept-Language`
entry. The hub defaults to English; the service manager (setup, update and
maintenance messages) keeps German when a request states no language, so
older tools see no change. Stable codes, not texts, are the contract for
clients; the setup page picks its language from the browser or app. Clients must display the server message and tolerate
unknown capabilities; native runtime UX for this state remains an explicit test.
No separate coupling, service ports or backend sessions are required in kurtz.

Future service manifests define pinned artifacts, dependencies, minimal networks/
mounts, resource budgets, health, backup/restore and migration contracts. Missing
mounts are unavailable, never empty. Backups include originals, consistent DB
state, ACLs and mappings in a common recovery manifest; model blobs may be
redownloadable. An old binary alone is not a database rollback.

### Updates and rollback (P3, Mac)

The service manager records the data version (`data-version.json`: schema,
Jellyfin version, build, commit time of the server sources, product version,
component-list digest, signed, channel) after a verified start. Builds of the
same Jellyfin release are ordered by the commit time of their sources; a
package without a version file never starts unguarded once a component list
or recorded data exists.

- The build lists every file and link under `Contents/Resources` with its
  SHA-256 (`components.json`, written last before code signing; links must
  stay inside the package). A release signs exactly these bytes with Ed25519
  (`components.sig`, `mutti-release sign-components`, key outside the
  repository, trusted key IDs compiled into `mutti-migrate`). On every start
  the manager checks the whole set (about 0.4 s for the 804 MB package): a
  changed, missing or additional file, a foreign or unknown-key signature, or
  a `release` channel build without a valid signature blocks the start before
  anything is touched. A package replaced while Mutti runs is not relaunched
  (server, modules, Connect) until Mutti is opened again. Finder metadata
  (`.DS_Store`, `._*`) is ignored; links must resolve inside the package.
  The channel is declared by the package itself: the signature requirement
  catches unsigned release builds, not a deliberately modified package; that
  protection is the macOS code signature of a released app. Development builds without a list or signature run and
  are recorded as unsigned. Two builds of the same commit with different
  files count as different builds (snapshot before the second one starts).

- A start with a different build first takes an offline snapshot of the
  stopped instance (config, data, Connect state, setup state) and the module
  data (without downloaded models, temporary files and transcodes), hashed and
  made visible atomically; APFS clones keep it cheap. No space, no update:
  the previous state stays untouched. Interrupted snapshots are discarded.
- The update is `pending` until the new version is set up and its module and
  device state still parse; then `verified` and the new data version is
  recorded. A crash loop or 20 minutes without verification is `failed`; the
  same build retries, an older build is `blocked`.
- Data written by a newer Jellyfin or by a build from newer sources of the
  same release is never started by an older package (`blocked`, downgrade
  lock). A data folder that is a link elsewhere blocks the update (it could
  not be snapshotted). Only the native owner app can roll back, only
  while blocked and only with exactly the snapshot's version.
- Rollback is resumable: it records the access in force, persists
  `rolling_back` with the folder for the replaced data, moves the whole data
  stage aside (`before-rollback-*`, never deleted), restores the snapshot
  (only listed files and links, active path validated), points the active
  instance back to the snapshot's one and then applies withdrawals. An
  interrupted rollback blocks the start until it is completed. Withdrawals
  made after the snapshot stay in force: a device survives only with the same
  profile and session, a module grant or enabled module only if still in
  force, a profile link only if unchanged; an unreadable replaced state counts
  as withdrawn. Devices paired after the snapshot must pair again.
  Limitation: Jellyfin's own users and sessions return to the snapshot, so a
  user disabled inside Jellyfin after the snapshot is active again while a
  device paired before stays paired.
- Snapshots kept: the newest two, the one the update record refers to and
  the newest of every Jellyfin release.
- The native Mac app shows the pre-update backup and the blocked state and
  offers the rollback. Not yet implemented: restore of a snapshot onto an
  empty second target, and the release key itself (no key is trusted yet, so
  every current package is an unsigned development build).

## Open decisions

1. Managed Linux-service runtime on Mac, support and licensing obligations.
2. Generic file storage versus typed archives plus transient attachments.
3. Household sharing, collections and cross-backend ACL reconciliation.
4. Identity continuity and revocation across loss, cloning and restore.
5. Signalling operator and real WAN reachability envelope.
6. Measured hardware classes, workload budgets and trusted qualification loading.

These do not block specifying contracts or denying unqualified actions. They do
block claiming the corresponding deployment or product feature as finished.
