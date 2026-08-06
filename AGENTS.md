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
