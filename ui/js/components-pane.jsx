function MessagePane({
  msg,
  msgTags,
  attachments,
  token,
  onToggleUnread,
  onStar,
  onArchive,
  onUnarchive,
  onDelete,
  onReply,
  archived,
  busy,
  account,
}) {
  const { t, lang } = useI18n();
  const [reply, setReply] = React.useState("");
  React.useEffect(() => {
    setReply("");
  }, [msg?.id]);
  if (!msg) {
    return (
      <div className="h-full flex items-center justify-center text-ink-dim text-sm">
        {t("select_message")}
      </div>
    );
  }
  const rawHtml = decodeBodyHtml(msg.body_html);
  const html = rawHtml || "<pre>" + (msg.body_text || "") + "</pre>";
  // Senders style HTML mail for a white page. On the dark theme that means
  // dark text on a dark background, so real HTML mail gets its own light
  // surface; plain text keeps following the UI theme.
  const bodyClass = rawHtml ? " prose-mail--html" : "";
  const btn =
    "text-xs px-2.5 py-1 rounded border border-paper-line text-ink-muted hover:border-accent hover:text-accent";
  const chips = (msgTags || []).filter((t) => t !== "archive");
  return (
    <div className="h-full flex flex-col">
      <header className="px-6 py-5 border-b border-paper-line">
        <h1 className="font-display text-2xl text-balance text-ink">{msg.subject}</h1>
        <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-sm text-ink-muted">
          <span>
            <span className="text-ink-dim">{t("from")} </span>
            {msg.from_addr}
          </span>
          <span>
            <span className="text-ink-dim">{t("to")} </span>
            {msg.to_addr}
          </span>
          <span className="font-mono text-xs tabular-nums">{formatWhen(msg.created_at, lang)}</span>
        </div>
        {chips.length > 0 && (
          <div className="mt-2 flex flex-wrap gap-1">
            {chips.map((t) => (
              <span
                key={t}
                className="text-[0.7rem] uppercase tracking-wide px-1.5 py-0.5 rounded border border-paper-line text-ink-dim"
              >
                #{t}
              </span>
            ))}
          </div>
        )}
        {(attachments || []).length > 0 && (
          <div className="mt-2 flex flex-wrap gap-2">
            {attachments.map((a) => (
              <a
                key={a.id}
                href={
                  "/api/attachments/" +
                  encodeURIComponent(a.id) +
                  "/open?token=" +
                  encodeURIComponent(token || "")
                }
                target="_blank"
                rel="noopener noreferrer"
                className="text-xs text-accent underline"
              >
                {a.filename || t("attachment")}
              </a>
            ))}
          </div>
        )}
        <div className="mt-3 flex flex-wrap gap-1.5">
          <button onClick={onToggleUnread} className={btn}>
            {msg.unread ? t("mark_read") : t("mark_unread")}
          </button>
          <button onClick={() => onStar(msg.id, !msg.starred)} className={btn}>
            {msg.starred ? t("unstar") : t("star")}
          </button>
          {archived ? (
            <button onClick={onUnarchive} className={btn}>
              {t("unarchive")}
            </button>
          ) : (
            <button onClick={onArchive} className={btn}>
              {t("archive_action")}
            </button>
          )}
          <button onClick={onDelete} className={btn}>
            {t("delete")}
          </button>
        </div>
      </header>
      <div
        className={"flex-1 overflow-y-auto scrollbar-thin px-6 py-5 prose-mail text-ink-muted leading-relaxed" + bodyClass}
        dangerouslySetInnerHTML={{ __html: html }}
      />
      {msg.direction !== "out" && (
        <form
          className="border-t border-paper-line px-6 py-3 space-y-2"
          onSubmit={(e) => {
            e.preventDefault();
            if (!reply.trim() || busy) return;
            onReply(reply.trim()).then(() => setReply(""));
          }}
        >
          <div className="text-xs text-ink-dim">
            <span className="text-ink-dim/70">{t("from")} </span>
            <span className="text-ink">{account?.address || msg.received_for || msg.to_addr || "—"}</span>
          </div>
          <textarea
            value={reply}
            onChange={(e) => setReply(e.target.value)}
            rows={3}
            placeholder={t("reply_placeholder")}
            className="w-full bg-paper border border-paper-line rounded px-3 py-2 text-sm text-ink placeholder:text-ink-dim focus:outline-none focus:border-accent"
          />
          <button type="submit" disabled={busy || !reply.trim()} className={btn}>
            {t("send_reply")}
          </button>
        </form>
      )}
    </div>
  );
}
