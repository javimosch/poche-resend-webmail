# poche-resend-webmail

**OSS self-hosted Resend webmail** over [poche](https://github.com/javimosch/poche). MIT.

Go BFF + React UI. Resend is the **transport** (inbound + send); poche is the **store** (search, tags, archive via `has_link` / `missing_link`).

Evolves [poche-webmail-demo](https://github.com/javimosch/poche-webmail-demo) and the agent surface of [machin-resend-inbox](https://github.com/javimosch/machin-resend-inbox).

## Quick start

```bash
# terminal A — poche
cd ~/ai/poche && ./build.sh
export POCHE_DB=/tmp/poche-resend-webmail.data POCHE_NO_NUDGE=1
./poche init   # → admin_token
export POCHE_TOKEN=<admin_token>
./poche serve 17781

# terminal B — webmail
cd ~/ai/poche-resend-webmail
export POCHE_URL=http://127.0.0.1:17781
export POCHE_TOKEN=<same>
export WEBMAIL_TOKEN=$(openssl rand -hex 16)
# optional until you sync/reply:
# export RESEND_API_KEY=re_…
# export RESEND_WEBHOOK_SECRET=…
./build.sh
./poche-resend-webmail seed -count 80
./poche-resend-webmail serve -port 3090
# open http://127.0.0.1:3090/?token=$WEBMAIL_TOKEN
```

## Commands

| | |
|---|---|
| `serve [-port 3090]` | BFF + UI |
| `seed [-count N]` | schema + mock mail (offline CI) |
| `sync` | pull Resend receiving → poche |
| `cleanup` | purge unstarred messages by retention/quota |
| `list` / `read` / `reply` | agent CLI |
| `guide` | JSON mental model |

## HTTP

Auth on `/api/*` (except `/api/health`, `/api/config`): `Authorization: Bearer` or `?token=`.

| Route | |
|---|---|
| `GET /api/messages` | list (+ `has_link` / `missing_link` / `where`) |
| `GET /api/messages/:id` | one message |
| `GET /api/messages/:id/attachments` | attachment rows |
| `PUT /api/messages/:id/star` | toggle `starred` flag |
| `GET /api/attachments/:id/open` | redirect → new tab |
| `POST /api/reply` | `{id,text,[from]}` threaded send |
| `POST /api/sync` | backfill |
| `POST /api/cleanup` | purge unstarred by retention/quota |
| `POST /webhooks/resend` | `email.received` (no webmail token; signature if secret set) |
| `POST /api/bulk` · tags · message-tags | `star`, `unstar`, archive, tag, delete |

## Persistence policy

Each mailbox has `retention_months`, `max_messages`, and `max_bytes` (set on the `mailboxes` collection). Missing/zero values fall back to `MAILBOX_RETENTION_MONTHS` (3), `MAILBOX_MAX_MESSAGES` (1000), and `MAILBOX_MAX_BYTES` (100 MB). Run `./poche-resend-webmail cleanup` or `POST /api/cleanup` to delete unstarred messages oldest-first until the policy is satisfied. Starred messages are never deleted.

Set `AUTO_CLEANUP=1` to run cleanup automatically after `./poche-resend-webmail sync`.

## Architecture

```
Browser ──Bearer──► Go BFF :3090 ──► poche :17781
                         │
                         ├── Resend receiving (sync / webhook)
                         └── Resend send (reply)
```

Poche admin token and Resend API key **never** leave the server.

## License

MIT

## Live dogfood

- **https://inbox2.intrane.fr** (dk1) — same `INBOX_TOKEN` as `inbox.intrane.fr`
- Units: `poche-resend-store` (:17783) + `poche-resend-webmail` (:8805)
- Does **not** replace `inbox.intrane.fr` (machin-resend-inbox stays primary for now)
