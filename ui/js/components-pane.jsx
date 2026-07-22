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
}) {
  const [reply, setReply] = React.useState("");
  React.useEffect(() => {
    setReply("");
  }, [msg?.id]);
  if (!msg) {
    return (
      <div className="h-full flex items-center justify-center text-ink-dim text-sm">
        Select a message
      </div>
    );
  }
  const html = decodeBodyHtml(msg.body_html) || "<pre>" + (msg.body_text || "") + "</pre>";
  const btn =
    "text-xs px-2.5 py-1 rounded border border-paper-line text-ink-muted hover:border-accent hover:text-accent";
  const chips = (msgTags || []).filter((t) => t !== "archive");
  return (
    <div className="h-full flex flex-col">
      <header className="px-6 py-5 border-b border-paper-line">
        <h1 className="font-display text-2xl text-balance text-ink">{msg.subject}</h1>
        <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-sm text-ink-muted">
          <span>
            <span className="text-ink-dim">From </span>
            {msg.from_addr}
          </span>
          <span>
            <span className="text-ink-dim">To </span>
            {msg.to_addr}
          </span>
          <span className="font-mono text-xs tabular-nums">{formatWhen(msg.created_at)}</span>
        </div>
        {chips.length > 0 && (
          <div className="mt-2 flex flex-wrap gap-1">
            {chips.map((t) => (
              <span
                key={t}
                className="text-[10px] uppercase tracking-wide px-1.5 py-0.5 rounded border border-paper-line text-ink-dim"
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
                {a.filename || "attachment"}
              </a>
            ))}
          </div>
        )}
        <div className="mt-3 flex flex-wrap gap-1.5">
          <button onClick={onToggleUnread} className={btn}>
            Mark {msg.unread ? "read" : "unread"}
          </button>
          <button onClick={() => onStar(msg.id, !msg.starred)} className={btn}>
            {msg.starred ? "Unstar" : "Star"}
          </button>
          {archived ? (
            <button onClick={onUnarchive} className={btn}>
              Unarchive
            </button>
          ) : (
            <button onClick={onArchive} className={btn}>
              Archive
            </button>
          )}
          <button onClick={onDelete} className={btn}>
            Delete
          </button>
        </div>
      </header>
      <div
        className="flex-1 overflow-y-auto scrollbar-thin px-6 py-5 prose-mail text-ink-muted leading-relaxed"
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
          <textarea
            value={reply}
            onChange={(e) => setReply(e.target.value)}
            rows={3}
            placeholder="Reply…"
            className="w-full bg-paper border border-paper-line rounded px-3 py-2 text-sm text-ink placeholder:text-ink-dim focus:outline-none focus:border-accent"
          />
          <button type="submit" disabled={busy || !reply.trim()} className={btn}>
            Send reply
          </button>
        </form>
      )}
    </div>
  );
}
