## v0.4.0 release — retention fix, auto-tagging, tag management

- **Critical retention fix** — `retention_months=0` now means keep forever (was silently falling back to 3-month default, purging mail meant to be kept indefinitely)
- **Scheduled sync** — systemd timer caches emails from Resend every 6 hours, before their retention expires, even if the user never opens the UI
- **Auto-refresh** — 60-second polling when the tab is visible, plus a manual refresh button
- **Spam auto-tagging** — DMARC reports and scam/domain-renewal emails auto-tagged `spam` on ingest
- **DMARC auto-tagging** — DMARC reports also tagged `dmarc` for filtering
- **Tag counts in sidebar** — total message count per tag, not just unread
- **Hide tagged from inbox** — any message with any user tag is excluded from the inbox; only untagged messages appear
- **"Tag similar" modal** — tagging a message offers to also tag all messages from the same sender, with optional subject filter
- **Cache headers** — UI files now send `no-cache` to prevent stale JS after deploys
