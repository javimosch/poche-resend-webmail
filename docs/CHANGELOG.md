# Changelog

## v0.4.0

### Fixed

- **Retention `0` now means keep forever** — `mailboxFloat()` treated 0 as unset and fell back to the 3-month default, silently purging mail meant to be kept indefinitely. New `mailboxFloatOrEnv` distinguishes absent (use default) from present-zero (no limit).
- **Inbound attachment content** — the `download_url` field never existed in Resend's API response; attachments were always empty. Now downloads the raw RFC822 and extracts MIME parts.
- **Starring from the UI** — the star toggle never actually worked.
- **Compose From stuck on previous account** — switching accounts left the compose sender stale.
- **Cross-tenant IDOR** — any signed-in tenant could star/archive/tag/delete another tenant's mail by sending its ids directly.
- **Bulk tag not creating tag row** — the `tag` bulk action created `message_tags` links but not the `tags` collection row, so tags applied via bulk action didn't appear in the sidebar.
- **Stale browser cache after deploys** — UI files now send `Cache-Control: no-cache, must-revalidate`.

### Added

- **Scheduled sync timer** — systemd timer runs sync every 6 hours, caching emails from Resend before their retention expires, even if the user never opens the UI.
- **Auto-refresh** — 60-second polling when the tab is visible (Visibility API), plus a manual refresh button.
- **Spam auto-tagging** — DMARC reports and scam/domain-renewal emails are auto-tagged `spam` on ingest (both webhook and sync paths).
- **DMARC auto-tagging** — DMARC reports are also tagged `dmarc` for filtering.
- **Tag counts in sidebar** — shows total message count per tag, not just unread.
- **Hide tagged messages from inbox** — any message with any user tag is excluded from the inbox; only untagged messages appear.
- **"Tag similar" modal** — tagging a single message offers to also tag all messages from the same sender, with an optional subject-contains filter.
- **Gmail-style conversation threading** — inbox collapses by thread; message pane shows full thread.
- **Multi-account switcher** — log into a second mailbox without losing the first session (Proton-style).
- **Catch-all domain mailboxes** — one mailbox can claim an entire domain; any unmatched address routes to it.
- **Inbox address badge + filter** — filter by recipient or sender.
- **Rich reply formats** — reply supports plain/markdown/HTML, same as compose.
- **Editable reply From** — catch-all mailboxes can reply as any address.
- **WYSIWYG HTML compose** — live split preview for Markdown, full-screen minimizable compose modal.

## v0.3.6

- Per-mailbox sessions, compose, attachments, multi-tenant provisioning.
