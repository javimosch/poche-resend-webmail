# AGENTS.md — poche-resend-webmail

OSS self-hosted Resend webmail over [poche](https://github.com/javimosch/poche).
Go BFF + React CDN UI. MIT.

## Rules

- ≤500 LOC per Go file; ≤300 LOC per JSX when splitting
- Store = **poche** only; transport = **Resend** (inbound + send)
- Browser talks to **BFF only** — never poche token / Resend key in the client
- Auth v0 = single `WEBMAIL_TOKEN` (Bearer or `?token=`)
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

The webmail layer keeps full email bodies in poche. By default, unstarred messages are purged when:
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
