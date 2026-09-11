## v0.3.6 release — compose, threading, multi-account

- **Conversation threading** — Gmail-style thread collapsing in the inbox, full thread view in the message pane
- **Multi-account switcher** — log into a second mailbox without losing the first session (Proton-style)
- **Catch-all domain mailboxes** — one mailbox claims an entire domain; any unmatched address routes to it
- **Rich compose** — WYSIWYG HTML editor, live Markdown split preview, full-screen minimizable modal
- **Attachments** — send attachments (streamed to Resend), sent copies keep attachment bytes
- **Inbox filtering** — address badge + filter by recipient or sender
- **Reply improvements** — plain/markdown/HTML formats, editable From for catch-all mailboxes
- **Security fix** — cross-tenant IDOR patched; signed-in tenants can no longer manipulate another tenant's mail
- **Attachment fix** — inbound attachment content now extracted from raw RFC822 (the `download_url` field never existed)
