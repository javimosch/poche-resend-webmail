# Changelog

All notable changes to this project. Dates are ISO; versions follow semver
loosely — the store schema and API are still moving.

## v0.3.6 — 2026-08-06

First release with a real client on it: a mailbox on its own domain, its own
Resend account, sending and receiving live mail.

### Added

- **Compose.** New mail, not just replies: `POST /api/compose`, a UI modal, and
  `send` on the CLI. To/Cc/Bcc accept an array or a comma-separated string.
- **Sent view.** Inbox now filters `direction=in`, Sent filters `direction=out`,
  so replies stop landing back in the inbox.
- **Editor formats.** Plain, Markdown and HTML. Markdown is converted
  server-side (goldmark, GFM) so the CLI and the UI produce identical output,
  and `POST /api/render` previews exactly what would be sent.
- **Attachments (outgoing).** Attach files in compose or with
  `send --attach a.pdf,b.jpg`. Bytes go to Resend and to `BLOB_DIR`, so the
  Sent copy keeps them. Per-file, total and count caps; storing is refused
  below `BLOB_MIN_FREE_BYTES`; deleting a message reclaims disk and quota.
- **Multi-tenant transport.** A mailbox can carry its own `resend_api_key` and
  `resend_webhook_secret`, so one deployment serves several client domains.
  Send, webhook verification, content fetch and `sync` all resolve per mailbox.
- **Credentials encrypted at rest** (AES-256-GCM under `CREDENTIALS_KEY`),
  with `mailbox encrypt-secrets` to migrate existing rows. poche stores
  documents in the clear, so any copy of the data directory used to carry live
  client keys.
- **Tag rename and delete**, from the sidebar or the API; `archive` stays
  protected as a system tag.
- **Per-tenant identity.** `brand` (sidebar name), `webmail_url` (used in
  password-reset links) and `reset_from`, each falling back to a deployment
  default.
- **English/French UI** with a language switch, browser-language detection on
  first visit, and locale-aware dates.
- **Light theme** alongside dark, larger base type, and a mobile layout: the
  sidebar becomes a drawer and the list and message take turns below 768px.
- **Retention is visible.** The sidebar states the policy in force
  ("kept 3 months · ★ kept forever") rather than deleting mail silently.
- **Click-to-copy** for the logged-in address.
- **Observability.** Webhook deliveries (with the Host they arrived on),
  destructive bulk actions, failed content fetches and attachment stores are
  all logged.

### Security

- **Real Svix webhook verification** — base64 HMAC over
  `<id>.<timestamp>.<body>` keyed by the decoded `whsec_` secret, with a
  5-minute replay window. The previous check HMAC'd the raw body and could
  never match a genuine signature; setting a real secret would have rejected
  every inbound email.
- **Inbound HTML is sanitized before storage** (bluemonday): script, style,
  iframe and form elements dropped with their contents, event handlers and
  `javascript:` URLs stripped, inline CSS limited to a safe property list.
  `sanitize` migrates messages stored earlier.
- The client also un-escaped `<` back into `<` after `JSON.parse` had
  decoded genuine escapes, rebuilding tags out of inert text and undoing the
  sanitizing. Removed.
- **Attachment access is scoped to the owning mailbox.** An attachment id was
  previously enough for any signed-in tenant to fetch any file.
- Attachments always download (`Content-Disposition`, `nosniff`, sandbox CSP),
  so an HTML or SVG attachment cannot execute in the mailbox origin.
- Secrets — Resend keys, webhook secrets, mailbox passwords — are accepted from
  stdin or the environment, never argv, and masked in `mailbox list`.

### Fixed

- `p.Get` returns `{"id":…,"doc":{…}}`, and three call sites parsed the
  envelope instead of the document, so fields read as empty: mailbox usage
  counters never accumulated, `mailboxUsage` never hit its fast path and
  persisted a body-only recalculation over the real total, and deleting a
  message skipped the usage update entirely.
- Two mailbox-usage writes moments apart lost the first: poche's
  read-after-write is not immediate. Compose now computes one delta.
- `mailbox update` did not ensure the schema, so a store predating a field
  dropped it silently while reporting success.
- Password-reset links used a single global `WEBMAIL_URL` and a shared sender,
  so a client's reset mail carried the wrong hostname and was sent from a
  domain verified in a different Resend account — it would have failed outright.
- `startServer` sliced the token to 8 characters and panicked on shorter ones.
- `smoke.sh` failed intermittently: under `pipefail`, `cmd | grep -q` dies on
  SIGPIPE when grep exits early.
- The storage bar rendered `undefined B` for admin tokens.
- The `sanitize` migration was not idempotent for a body carrying literal
  backslash escapes.

### Known gaps

- **Inbound attachments have no content.** Resend's webhook payload carries no
  attachment bytes or URL, and fetching the full message needs a read-capable
  API key; a send-only key falls back to metadata. Names arrive, contents do not.
- `sync` needs the same read access, so webhook delivery is currently the only
  ingest path.
- `retention_months: 0` means "inherit the default", not "keep forever".
- Retention is stated but never enforced automatically — `cleanup` only runs
  when invoked.
- poche holds documents in memory, so attachment bytes live on the host
  filesystem; back up `BLOB_DIR` alongside `poche.data`.
