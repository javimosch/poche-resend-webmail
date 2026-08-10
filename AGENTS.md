# AGENTS.md — poche-resend-webmail

OSS self-hosted Resend webmail over [poche](https://github.com/javimosch/poche).
Go BFF + React CDN UI. MIT.

## Rules

- ≤500 LOC per Go file; ≤300 LOC per JSX when splitting
- Store = **poche** only; transport = **Resend** (inbound + send)
- Browser talks to **BFF only** — never poche token / Resend key in the client
- Auth v0 = single `WEBMAIL_TOKEN` (Bearer or `?token=`)
- Auth v1 (v0.3.0+) = per-mailbox sessions: `POST /api/login` {address, password} → session token; `ADMIN_TOKEN` for admin API; `WEBMAIL_TOKEN` still works as legacy admin
- Attachments: open in new tab (`/api/attachments/:id/open`)
- Do not deploy against inbox.intrane.fr from this repo (separate dogfood later)
- stdout JSON for CLI data; stderr for progress events
- Adopt cli-specs where useful (`guide`, structured errors)

## Loop

```
poche init → POCHE_TOKEN
poche serve 17781
export POCHE_URL=http://127.0.0.1:17781 POCHE_TOKEN=…
export WEBMAIL_TOKEN=$(openssl rand -hex 16)
export RESEND_API_KEY=re_…          # optional until sync/reply
export RESEND_WEBHOOK_SECRET=…      # optional; empty = insecure accept (dev)
./poche-resend-webmail seed -count 50   # offline UI without Resend
./poche-resend-webmail serve -port 3090
# open http://127.0.0.1:3090/?token=$WEBMAIL_TOKEN
```

## Model

- Inbox = `missing_link=message_tags.message_id:tag=archive`
- Archive / tags = `has_link=…`
- Inbound upsert keyed by `resend_id` (idempotent webhook + `sync`)

## Persistence policy (v0.2.0+)

The webmail layer keeps full email bodies in poche. **`retention_months: 0` does not mean "keep forever."** Zero is read as *unset*
and falls back to `MAILBOX_RETENTION_MONTHS` (default 3), same for
`max_messages` / `max_bytes` at 0. To actually keep mail indefinitely, set a
large value (e.g. `--retention-months 1200`). The effective policy is reported
by `GET /api/mailbox/usage` and shown under the storage bar in the sidebar.

Note also that nothing purges on its own: `cleanup` runs only when invoked
(CLI, `POST /api/cleanup`, or after `sync` when `AUTO_CLEANUP=1`). There is no
scheduler on dk1, so the policy is currently stated but not enforced.

By default, unstarred messages are purged when:
- older than `MAILBOX_RETENTION_MONTHS` (default 3), or
- the mailbox exceeds `MAILBOX_MAX_MESSAGES` (default 1000), or
- the mailbox exceeds `MAILBOX_MAX_BYTES` (default 100 MB).

Starred messages are always kept. Per-address policy can be set on each `mailboxes` doc (now exposed for `update`).

Commands / API:
- `PUT /api/messages/:id/star` — toggle star; also exposed in the UI.
- `POST /api/bulk` with `"action":"star"|"unstar"`.
- `POST /api/cleanup` or `./poche-resend-webmail cleanup` — run retention/quota purge.
- `AUTO_CLEANUP=1` runs cleanup automatically after each sync.

Default env:
```
MAILBOX_RETENTION_MONTHS=3
MAILBOX_MAX_MESSAGES=1000
MAILBOX_MAX_BYTES=104857600
```

## Multi-tenant mailbox provisioning (v0.3.0+)

Per-address mailboxes with bcrypt password auth, storage caps, and session-based login.

### Provision a mailbox

```bash
export POCHE_TOKEN=…  # admin token for poche
./poche-resend-webmail mailbox create \
  --address contact@lacure.enbauges.fr \
  --password "SecurePass123!" \
  --recovery-email "arancibiajav@gmail.com" \
  --alias "florence@lacure.enbauges.fr,tom@lacure.enbauges.fr,flo@lacure.enbauges.com" \
  --max-bytes 524288000        # 500 MB
  --max-messages 10000          # optional
  --retention-months 1200       # ~100 years; 0 would inherit the 3-month default
```

### Manage mailboxes

```bash
./poche-resend-webmail mailbox list
./poche-resend-webmail mailbox update --address x@y.fr --max-bytes 1073741824
./poche-resend-webmail mailbox update --address x@y.fr --password "newpass"
./poche-resend-webmail mailbox update --address x@y.fr --recovery-email emergency@x.fr
./poche-resend-webmail mailbox update --address x@y.fr --active false   # suspend
./poche-resend-webmail mailbox delete --address x@y.fr --force
```

### Aliases

A mailbox has one primary address + N aliases. Login and ingest routing
check both. All aliases share the same inbox, password, and quota.

```bash
./poche-resend-webmail mailbox alias add    --mailbox contact@x.fr --alias florence@x.fr
./poche-resend-webmail mailbox alias remove --alias florence@x.fr
./poche-resend-webmail mailbox alias list   --mailbox contact@x.fr
```

### Catch-all domain mailboxes

A mailbox can additionally claim an entire domain: any address at that
domain which doesn't match a real mailbox or alias first routes into it.
Useful for a single "admin@x.fr" login that reads everything sent to
`anything@x.fr` — e.g. reusing a Resend account/key that already owns the
whole domain, without provisioning one mailbox per sender address.

```bash
./poche-resend-webmail mailbox update --address admin@intrane.fr --catchall-domain intrane.fr
./poche-resend-webmail mailbox update --address admin@intrane.fr --catchall-domain none   # clear
```

At most one mailbox may claim a given domain — setting it on a second
mailbox is refused with the address that already owns it. `mailbox list`
reports `catchall_domain` on whichever mailbox has one.

**Precedence**: exact `mailboxes.address` match, then `aliases.address`,
then a domain catch-all — in that order, for all three inbound paths
(webhook, SMTP receiver, `sync`), which all funnel through the same
`findMailboxRecordForAddress`/`findOrCreateMailboxForAddress` resolvers
(session.go). This means giving one specific address under a claimed domain
its own mailbox later (or aliasing it elsewhere) makes it stop landing in
the catch-all automatically — nothing on the catch-all mailbox itself needs
to change.

The catch-all mailbox is an ordinary mailbox row: its own password, its own
`max_bytes` quota (see "Storage usage" above), its own Resend credentials.
A busy catch-all inbox will fill its quota faster than a single-address one,
so budget accordingly. Nothing in `mailboxAllowsIngest`/`mailboxUsage`
changes — quota enforcement is unaware of catch-all routing.

### Forgot password

Requires `--recovery-email` set on the mailbox. The reset link is sent
via Resend to the recovery address (not the mailbox address).

```bash
# CLI (sends via Resend, or prints token if RESEND_API_KEY is unset)
./poche-resend-webmail mailbox reset-password --address contact@x.fr

# API (called by the UI "Forgot password?" link)
POST /api/forgot-password {"address":"contact@x.fr"}
  → sends reset link to recovery_email (always returns same response)

POST /api/reset-password {"token":"...","new_password":"..."}
  → sets new password, invalidates all sessions, token is single-use
```

The reset URL is `WEBMAIL_URL/?reset_token=TOKEN` (set `WEBMAIL_URL` env).
Reset tokens expire after 1 hour.

### Ingest routing

Two paths for inbound email:

1. **Resend webhook** (`POST /webhooks/resend`): Resend receives email on
   verified domains (e.g. `intrane.fr`), posts to webhook. Routes by `to` field.

2. **Built-in SMTP receiver** (`smtp -port 25`): Minimal SMTP server that
   accepts email for any address, parses RFC 822, and routes to the matching
   mailbox. Used for domains where MX records point directly to the server
   (e.g. `lacure.enbauges.fr` → `mail.lacure.enbauges.fr` → dk1).

Both paths use the same `upsertInbound` → `findOrCreateMailboxForAddress` flow.

### Storage usage

`GET /api/mailbox/usage` (auth: session or admin) returns:
```json
{"used_bytes": 1895581, "max_bytes": 524288000, "percent": 0.36, "message_count": 11}
```

The UI sidebar shows a live storage bar (fetches every 30s).

### Address badge + filter (issue #2)

Every inbox row shows a small badge with the address the message was
actually sent to (`to_addr`) — the point once a mailbox can catch mail for
many addresses (see catch-all domains above). The toolbar has a two-step
filter: pick "Recipient" or "Sender", then pick one of the addresses that
field has actually seen, with a live count.

```
GET /api/messages/facets?field=to_addr   (or from_addr)
  → {"field":"to_addr","values":[{"value":"javi@intrane.fr","count":5},...],"truncated":false}
```

Requires a mailbox-scoped session (not an admin token — an admin has no
single mailbox to scope the facet to). There is no GROUP BY in poche's
query language reachable from this client, so `messageFieldFacets`
(messages.go) pages through the mailbox's messages and aggregates in Go,
capped at `facetScanCap` (20,000 messages) — `truncated:true` means the
mailbox has more than that and the counts are a partial view, not the
whole mailbox. The facet list always reflects the WHOLE mailbox, independent
of whatever view/tag/search is currently active in the message list — it's
a stable "addresses this mailbox has ever seen" list to filter FROM, not a
live facet-of-the-current-results.

Selecting a value adds an ordinary `to_addr=<addr>`/`from_addr=<addr>`
equality clause to the same `where=` the search box already builds
(`buildListPath`, ui/js/api.jsx) — no new query capability was needed for
the filtering itself, only for building the dropdown's option list.

### Seeding test emails

```bash
./poche-resend-webmail seed-mailbox --address contact@x.fr --count 10 --size-kb 200
```

Generates `count` emails of ~`size-kb` KB each, routed to the specified mailbox.
Emails to addresses with no matching mailbox are dropped (not stored).
Quota is checked at ingest time — if `used + new_message_size > max_bytes`, the email is dropped.

## Compose (v0.3.1+)

Send a brand-new email (not a reply). The sender must be an address that
belongs to the caller's mailbox (primary or alias) **and** pass
`MAIL_FROM_ALLOWLIST`.

```bash
POST /api/compose
  {"from":"contact@x.fr","to":"a@b.fr, c@d.fr","cc":"e@f.fr","bcc":"g@h.fr",
   "subject":"Devis","text":"Bonjour…"}
  → {"sent_id":"…","from":"…","to":[…],"stored":true}

GET /api/mailbox/addresses
  → {"addresses":["contact@x.fr","florence@x.fr"],"primary":"contact@x.fr"}

# CLI (admin: From decides which mailbox stores the sent copy)
./poche-resend-webmail send --from contact@x.fr --to a@b.fr \
  --subject "Devis" --text "Bonjour…" [--cc …] [--bcc …]
```

- `to` / `cc` / `bcc` accept a JSON array or a comma/semicolon-separated string.
- The sent copy is stored with `direction=out` (bcc is **not** stored).
- Inbox now filters `direction=in`; the new **Sent** view filters `direction=out`.
- UI: "Compose" button in the sidebar → modal with From selector (primary + aliases).
- Plain text only — no attachments, no HTML compose yet.

`RESEND_BASE_URL` overrides the Resend API host (default
`https://api.resend.com`) so sends can be pointed at a stub in tests.

## Attachments (v0.3.6+)

Outgoing mail can carry files, and the Sent copy keeps them: bytes go to
Resend **and** to a blob directory on the host (`BLOB_DIR`, default
`/var/lib/poche-resend-webmail/blobs`), with metadata in poche. Inbound
attachments still redirect to Resend's URL, which expires eventually.

Bytes are not in poche because poche holds documents in memory — 6 MB of
base64 took the store from 4 MB to 315 MB RSS — and its `_files` store has a
GET route but **no HTTP upload**. So the mail model stays in poche and only
opaque bytes live on disk.

**Back up `BLOB_DIR` with `poche.data`.** A poche-only backup restores
messages whose attachments have vanished.

Serving is deliberately hostile to the browser: `Content-Disposition:
attachment`, `nosniff`, and a sandbox CSP, so an HTML or SVG attachment cannot
execute in the mailbox origin. Access is scoped to the owning mailbox — an
attachment id alone is not a capability. Deleting a message removes its blobs
and returns the bytes to the quota.

```bash
./poche-resend-webmail send --to a@b.fr --subject Devis --text "ci-joint" \
  --attach ./devis.pdf,./photo.jpg
```

Caps (env): `ATTACHMENT_MAX_BYTES` 10 MB per file, `ATTACHMENTS_MAX_TOTAL_BYTES`
20 MB, `ATTACHMENTS_MAX_COUNT` 10, and `BLOB_MIN_FREE_BYTES` (512 MB) below
which storing is refused so a mailbox cannot fill the host. The compose endpoint wraps its body in a
`MaxBytesReader` so an oversized upload is refused before it buffers. File
names are stripped of any path — `../../etc/passwd` is sent as `passwd`.

## Inbound attachment content — fixed (2026-08-09)

Inbound attachments used to arrive as names without content, attributed to
La Cure's Resend key being send-only. That diagnosis was **wrong**: Resend's
attachment objects carry no per-attachment download link at all, verified
by dumping a real message's raw API response directly with a genuine
read-capable key (the same wrong assumption was found and fixed first in
the sibling machin-resend-inbox project's issue #1 — this is that same fix,
ported to Go). The old code read `attachment.download_url`, which never
exists; it would have stayed empty forever regardless of key permissions.

What's actually there: `.raw.download_url`, a link to the entire raw RFC822
email (also expiring). `upsertAttachments` (sync.go) now downloads that
once per message and extracts each attachment's bytes from the MIME parts
directly (`mime_extract.go`, Go's `mime/multipart` + `net/mail` — no
hand-rolled parser needed, unlike the MFL side, which had no such stdlib).
Extracted bytes are stored in **machin-esetres** (`esetres.go`; `ESETRES_URL`/
`ESETRES_BUCKET`/`ESETRES_TOKEN`, one shared bucket, not bucket-per-mailbox
— matches the naming convention and single-bucket recommendation from the
scoping below) under `<message_id>/<resend_attach_id>/<filename>`, and
`messages.go`'s `handleAttachmentOpen` now proxies from there (never a
redirect — machin-esetres doesn't know about the
Content-Disposition/nosniff/CSP headers this app sets, so the app keeps
serving the bytes itself, same as the existing local-blob path).

Without `ESETRES_*` configured, attachment metadata still stores exactly as
before (`stored: false`, no content) — this is additive, not a hard
dependency on machin-esetres existing.

Verified end-to-end against real production data, not just typechecked:
a scratch poche + machin-esetres instance, La Cure's real Resend account,
`sync` pulling a real message with a real attachment — the extracted
content matched Resend's own reported size (240 bytes) exactly, and
`GET /api/attachments/<id>/open` served it back with the correct
`Content-Disposition`/`nosniff`/CSP headers intact.

New schema field: `attachments.esetres_key` (string). `download_url` is
kept in the schema for backward compatibility but is now always written
empty — it never carried real data.

## Scoped: moving attachment storage to machin-esetres (2026-08-08, not yet built)

[machin-esetres](https://github.com/javimosch/machin-esetres) is a self-hosted
object store (buckets, sha256-deduped objects, bearer-token REST + a real
SigV4 S3 facade) built specifically to replace `blobstore.go`. It's deployed
on rbm21 (`http://100.123.0.125:9000`, internal-only over Tailscale) but
**not wired up here yet** — this section is the plan, written before the code,
same discipline as machin-esetres's own AGENTS.md.

### Why (recap)

`blobstore.go` is local-disk-only (`BLOB_DIR`), single host, no dedup by
design (see its own comment: random not content-addressed ids, specifically
*because* there was no refcounting — deleting one message could pull bytes
out from under another that happened to share content). machin-esetres
solves the refcounting problem properly, so that specific past limitation
goes away as a side effect of migrating, not just the disk-location problem.

### Target architecture

**Proxy, don't redirect.** `messages.go`'s `handleAttachmentOpen` currently
opens the local file and streams it with hostile-to-browser headers
(`Content-Disposition: attachment`, `nosniff`, sandbox CSP) — machin-esetres
has no concept of those and must never serve bytes straight to a browser.
poche-resend-webmail keeps terminating that policy: it does an HTTP GET to
machin-esetres and copies the response body through with its own headers
unchanged. The same reasoning applies to `PUT` on write.

Four call sites, all in `blobstore.go`'s current interface — the swap is a
drop-in replacement of that file, not a redesign of its callers:

| Current (`blobstore.go`) | Becomes | Call site |
|---|---|---|
| `putBlob(data) (id, err)` | `HTTP PUT /b/<bucket>/o/<id>` to machin-esetres, body = data | `compose.go:487` |
| `blobPath(id) (path, ok)` + `os.Open` | `HTTP GET /b/<bucket>/o/<id>`, stream `resp.Body` through | `messages.go:156` |
| `deleteBlob(id)` | `HTTP DELETE /b/<bucket>/o/<id>` | `cleanup.go:365`, `compose.go:505` |
| `freeBytes`/`blobMinFree` | drop — machin-esetres's own host disk is now its concern, not this app's | (internal to blobstore.go only) |

Existing `FileID` values (the random hex ids `putBlob` already generates and
that poche documents already reference) become the machin-esetres **object
key** unchanged — no document rewrites needed anywhere, just a different
place the bytes named by that key physically live.

### Config (new env vars)

- `ESETRES_URL` — e.g. `http://100.123.0.125:9000` (Phase 1 API, not the S3
  facade — no reason to pay the SigV4 signing cost for an internal
  service-to-service call this app fully controls both ends of).
- `ESETRES_BUCKET`, `ESETRES_TOKEN` — **recommend one shared bucket** (e.g.
  `poche-resend-webmail`) for the whole deployment, not bucket-per-mailbox.
  machin-esetres's own design assumed bucket-per-mailbox for a
  multi-tenant *object store*, but poche-resend-webmail already isolates by
  `mailbox_id` inside its own documents — a second isolation layer at the
  bucket level buys nothing here and would mean encrypting/rotating a
  per-mailbox token (reusing the existing `secrets.go` AES-GCM machinery)
  for no real benefit. One token, `readSecretFlag`'d in like every other
  credential in this app (stdin or `env:NAME`, never argv).

### Migration path for existing local blobs

1. Ship the machin-esetres client code with a **feature flag** (`ESETRES_URL`
   unset = current behavior, local disk only — zero risk to existing
   deployments, including La Cure's live one).
2. A one-time `poche-resend-webmail migrate-blobs` CLI command: walk
   `BLOB_DIR`, `PUT` each file to machin-esetres under its existing id as the
   key, verify the round-trip (download it back, compare sha256), then
   **don't delete the local copy yet**.
3. Cut reads over first (`handleAttachmentOpen` tries machin-esetres, falls
   back to local disk if not found there) — this is the reversible step.
4. Once reads have run clean for a while, cut writes over (`putBlob` calls
   removed, `compose.go`/`cleanup.go` call the HTTP client only).
5. Only then delete local `BLOB_DIR` — and only after `migrate-blobs` has
   been re-run once more to catch anything written between steps 2 and 4.

This is the same "rehearse against a real copy of prod data before touching
prod" discipline used for the credentials-encryption migration and the
0.2.3→0.3.2 poche upgrade earlier in this project's history — copy La Cure's
real `BLOB_DIR` to a scratch machin-esetres bucket first, verify counts and
byte-for-byte content, before running `migrate-blobs` against the live one.

### New failure mode to design for

Today, serving an attachment is a local disk read — it cannot fail for
network reasons. After this migration, it depends on rbm21 being reachable
over Tailscale. `handleAttachmentOpen` needs an explicit timeout and a clear
error (not a hang) when machin-esetres is unreachable, and this becomes a
new entry in whatever uptime/monitoring exists for poche-resend-webmail
(there is none dedicated today, per the README's honest-limitations list).

### Explicitly not decided yet

- Whether `migrate-blobs` is a real subcommand shipped in `poche-resend-webmail`
  or a one-off script run once and thrown away — leaning subcommand, since
  "a bad blob turned up, re-sync it" could plausibly recur.
- Whether inbound attachments (currently blocked on a read-capable Resend
  key — a separate, unrelated gap) land in machin-esetres too once that gets
  unblocked, or stay on the Resend-redirect path. No decision needed until
  the Resend key gap is actually resolved.

## Branding and layout (v0.3.5+)

The sidebar name is per **mailbox** (`brand`), falling back to `BRAND_NAME`
then `poche` — one deployment serves several clients, so it cannot be a single
global string. It rides on the login response into the stored account.

```bash
./poche-resend-webmail mailbox update --address contact@x.fr --brand "Enbauges"
```

Layout is responsive below `md` (768px): the sidebar becomes an overlay
drawer behind a ☰ button, and the list and the message take turns on the full
width with a back link. Mail bodies are forced to wrap (`pre-wrap`, capped
image/table widths) because senders write for arbitrary widths and a fixed
`<pre>` scrolls the whole page sideways on a phone.

The store-health line was removed from the sidebar; it only appears now when
poche is actually down.

## i18n (v0.3.4+)

UI strings live in `ui/js/i18n.jsx` as `{en, fr}` dictionaries; values may be
functions when interpolation is needed, so word order and plurals stay inside
the translation. English is the fallback — a missing French key renders the
English string rather than a bare key, so a partial translation degrades
readably.

- Language is per-browser (`localStorage.webmail_lang`), first visit follows
  `navigator.language`, and the EN/FR switch sits in the sidebar. It applies
  live, without a reload, and sets `<html lang>`.
- Dates go through `formatWhen(ms, lang)` so months localise too.
- Watch for one noun/verb trap: `archive` is the folder, `archive_action` is
  the button. Reusing one key gave a French pane button reading "Archives".

Adding a language = one more block in `I18N` plus its code in `LANGS`.

## Account switcher (2026-08-10)

Multiple mailbox logins now coexist client-side, Proton-style — logging
into a second mailbox used to silently overwrite `webmail_token`/
`webmail_account` in localStorage, so switching accounts meant losing the
first one's session entirely. That was a real problem once one mailbox
(e.g. the intrane.fr catch-all, see above) is a completely different login
from another (e.g. contact@lacure.enbauges.fr).

Storage moved from two single-slot keys to one list, `webmail_accounts`
(`ui/js/api.jsx`): `[{key, token, address, name, brand}, ...]` plus
`webmail_active_key` for which one is current. `key` is the mailbox address
when known, or `token:<first-12-chars>` for a raw admin-token login where
no address comes back from the server. `migrateLegacyAccount()` runs once
on load and folds any pre-existing single-slot session (or a `?token=` URL
param) into the new list — nobody already logged in is signed out by this
change.

Switching accounts (`useConfig().switchAccount`) is instant — no network
call, since the target session's token is already cached locally and
sessions last 7 days server-side (session.go). "Sign out" now means sign
out of the ONE active account, not all of them: it removes that entry and
falls back to whatever's left, or the login screen if none remain.

UI: `AccountSwitcher` (components.jsx) replaces the old static address
display in the sidebar footer — click it to open a dropdown of every
locally logged-in account (with a ✕ to sign out any one of them) plus
"+ Add account", which reopens `LoginForm` in an "Add another account"
mode (a `onCancel` prop) without touching the accounts already logged in.

Verified end-to-end in a real browser (Playwright): logged into two
mailboxes, confirmed both appear in the switcher, switched between them
with no re-authentication, reloaded the page and confirmed the active
account persisted, signed out of the active one and confirmed it fell back
to the other (not the login screen), then signed out of the last one and
confirmed only then did the login screen appear.

## Compose formats (v0.3.3+)

`POST /api/compose` takes `format`: `text` (default), `html`, or `markdown`.
Markdown is converted server-side (goldmark, GFM) so the CLI and the UI get
identical output, and **all outbound HTML is sanitized with the same policy as
inbound** before it is sent or stored. HTML mail always ships a text/plain
alternative — mail without one reads as spam to filters.

`POST /api/render {text, format}` previews exactly what compose would send:
same converter, same sanitizer. The UI preview uses it rather than rendering
Markdown in the browser, which would be a second pipeline that could disagree
with what actually goes out.

```bash
./poche-resend-webmail send --to a@b.fr --subject Devis \
  --format markdown --text "# Bonjour\n\n**devis** [lien](https://x.fr)"
```

## Tags: rename and delete (v0.3.3+)

```bash
PUT    /api/tags {"name":"devis","new_name":"quotes"}  # moves every message link
DELETE /api/tags?name=quotes                           # drops the tag + its links
```

`archive` is a system tag and is refused by both. Renaming is
attach-new-then-drop-old because poche exposes `message_tags` for
create+delete but **not update** — an `Update` there fails with
"not exposed for update", and swallowing that error makes a rename look like
it worked while silently losing labels. Deleting a tag needs `delete` exposure
on the `tags` collection (added in `ensureSchema`).

## Per-mailbox Resend credentials (v0.3.1+)

Each tenant can own a **separate Resend account** (its own verified domain,
API key and webhook secret) while sharing one deployment. A mailbox with a key
uses it; a mailbox without one falls back to the process-wide
`RESEND_API_KEY` / `RESEND_WEBHOOK_SECRET`.

```bash
# never put a key in argv — '-' reads stdin, 'env:NAME' reads the environment
printf 're_xxx' | ./poche-resend-webmail mailbox update --address contact@x.fr --resend-key -
RK=whsec_xxx ./poche-resend-webmail mailbox update --address contact@x.fr --resend-webhook-secret env:RK
./poche-resend-webmail mailbox update --address contact@x.fr --resend-key none   # clear
```

`mailbox list` reports `has_resend_key` / `has_webhook_secret` and a masked
`resend_key` — it never prints the credential.

Which key is used where:

| Path | Key |
|---|---|
| `POST /api/compose`, `POST /api/reply` | the sending mailbox's key |
| `POST /webhooks/resend` — signature verify | the recipient mailbox's secret |
| `POST /webhooks/resend` — fetch full email | the recipient mailbox's key |
| `sync` | polls **every** distinct key (env + each mailbox) |

The webhook resolves the recipient before verifying, because the recipient
decides whose signing secret applies. Nothing is written to the store until
the signature passes.

## Two poche traps that cost real debugging

1. **`p.Get` returns `{"id":…,"doc":{…}}`.** Parsing that directly instead of
   `.doc` makes every field read as empty — silently. It hit
   `updateMailboxUsage` (counters never accumulated), `mailboxUsage` (fast path
   never matched, so it recalculated from bodies and *persisted* that, erasing
   attachment bytes) and `deleteMessageWithLinks` (mailbox_id empty, so quota
   never went back down). Use `loadDoc` or unwrap explicitly.
2. **Read-after-write is not immediate.** Two `updateMailboxUsage` calls moments
   apart: the second read a stale counter and clobbered the first. Compute the
   whole delta and write once.

## Credentials at rest (v0.3.2+)

poche has **no encryption at rest** — it writes append-only WAL chunks in the
clear, so any copy of the data directory (backup, snapshot, pre-deploy tarball)
carries live tenant credentials. `resend_api_key` and `resend_webhook_secret`
are therefore encrypted with AES-256-GCM before storage.

```bash
./poche-resend-webmail secret-key                      # generate CREDENTIALS_KEY
# put it in the env file (mode 600), restart, then:
./poche-resend-webmail mailbox encrypt-secrets --apply # migrate existing rows
poche compact                                          # drop plaintext WAL history
```

- Stored form is `enc:v1:<base64(nonce||ciphertext)>`; values without the prefix
  are read as plaintext, so adoption needs no flag day.
- Without `CREDENTIALS_KEY` set, new credentials are stored **plaintext** and a
  `credential_stored_plaintext` event is logged.
- Reading an encrypted value without the key logs `credential_decrypt_err` and
  yields empty rather than silently falling back to the env credential.
- **Encrypting is not enough on its own:** the plaintext survives in older WAL
  chunks until `poche compact` rewrites them. Compact with the services stopped.
- Losing the key makes stored credentials unrecoverable — they must be re-entered.

### Admin access

Set `ADMIN_TOKEN` env to a separate token for admin API access (no mailbox scoping).
`WEBMAIL_TOKEN` still works as a legacy admin token (backwards compat).
