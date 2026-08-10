// Gmail-style conversation strip: every message that shares this one's
// thread_id (received AND sent, see reply.go/sync.go for how that's kept
// consistent across a reply round-trip), so you don't have to switch
// between Inbox and Sent to see the whole back-and-forth. Only renders
// once there's actually more than one message to show.
function ThreadStrip({ thread, current, onSelect, lang }) {
  const { t } = useI18n();
  if (!thread || thread.length < 2) return null;
  return (
    <div className="border-b border-paper-line max-h-40 overflow-y-auto scrollbar-thin">
      {thread.map((m) => {
        const active = current && m.id === current.id;
        return (
          <button
            key={m.id}
            onClick={() => onSelect(m.id)}
            className={
              "w-full text-left px-6 py-1.5 flex items-baseline gap-2 text-xs border-l-2 " +
              (active
                ? "border-l-accent bg-accent-soft text-ink"
                : "border-l-transparent text-ink-muted hover:bg-paper-line/40")
            }
          >
            <span className={"shrink-0 " + (m.unread ? "font-semibold" : "")}>
              {m.direction === "out" ? t("to_prefix", m.to_addr || "") : m.from_addr}
            </span>
            <span className="truncate flex-1 min-w-0 text-ink-dim">{m.preview}</span>
            <span className="shrink-0 font-mono tabular-nums text-ink-dim">
              {formatWhen(m.created_at, lang)}
            </span>
          </button>
        );
      })}
    </div>
  );
}

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
  addresses,
  catchallDomain,
  seenAddresses,
  thread,
  onSelectMessage,
  onBack,
}) {
  const { t, lang } = useI18n();
  const [reply, setReply] = React.useState("");
  // Plain stays the default here too (see ComposeModal): it's what most
  // replies are, and it can't be mangled by a converter.
  const [replyFormat, setReplyFormat] = React.useState("text");
  const [replyHtml, setReplyHtml] = React.useState("");
  const [replyHtmlResetKey, setReplyHtmlResetKey] = React.useState(0);
  // Same precedence the server falls back to (reply.go: received_for, then
  // to_addr, then the mailbox's own address) — so what's shown here is what
  // will actually be sent unless the user picks something else.
  const defaultReplyFrom = (msg) =>
    (msg && (msg.received_for || msg.to_addr)) || account?.address || (addresses || [])[0] || "";
  const [replyFrom, setReplyFrom] = React.useState(() => defaultReplyFrom(msg));
  React.useEffect(() => {
    setReply("");
    setReplyFormat("text");
    setReplyHtml("");
    setReplyHtmlResetKey((k) => k + 1);
    setReplyFrom(defaultReplyFrom(msg));
  }, [msg?.id]);

  // Mirrors ComposeModal's switchFormat: carry the draft forward through a
  // real conversion (or ask before it would otherwise silently vanish)
  // rather than leaving stale, wrongly-interpreted content behind a format
  // switch.
  const switchReplyFormat = (next) => {
    if (next === replyFormat) return;
    if (replyFormat === "html" && next !== "html") {
      const hasContent = stripHtmlForCheck(replyHtml).length > 0;
      if (hasContent && !reply.trim()) {
        if (!window.confirm(t("switch_format_confirm"))) return;
        setReplyHtml("");
        setReplyHtmlResetKey((k) => k + 1);
      }
    }
    if (next === "html" && replyFormat !== "html" && reply.trim() && !stripHtmlForCheck(replyHtml)) {
      renderBodyPreview(token, reply, replyFormat)
        .then((d) => {
          setReplyHtml(d.html || "");
          setReplyHtmlResetKey((k) => k + 1);
        })
        .catch(() => {
          const escaped = reply
            .split("\n")
            .map((line) => "<p>" + escapeHtmlText(line) + "</p>")
            .join("");
          setReplyHtml(escaped);
          setReplyHtmlResetKey((k) => k + 1);
        });
    }
    setReplyFormat(next);
  };
  const replyBodyValue = replyFormat === "html" ? replyHtml : reply;
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
        {onBack && (
          <button
            onClick={onBack}
            className="md:hidden mb-2 text-[0.76rem] text-ink-dim hover:text-accent"
          >
            ← {t("inbox")}
          </button>
        )}
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
          <div className="mt-2 flex flex-wrap gap-2 items-center">
            <span className="text-[0.7rem] uppercase tracking-wider text-ink-dim">
              {t("attachments")}
            </span>
            {attachments.map((a) => {
              const size = a.bytes ? " (" + fmtBytes(a.bytes) + ")" : "";
              // Outbound copies keep the name and size but not the bytes, so
              // there is deliberately nothing to link to.
              if (a.stored === false || !a.download_url) {
                return (
                  <span
                    key={a.id}
                    title={t("not_stored_title")}
                    className="text-xs text-ink-dim border border-paper-line rounded px-1.5 py-0.5"
                  >
                    {(a.filename || t("attachment")) + size} · {t("not_stored")}
                  </span>
                );
              }
              return (
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
                  {(a.filename || t("attachment")) + size}
                </a>
              );
            })}
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
      <ThreadStrip thread={thread} current={msg} onSelect={onSelectMessage} lang={lang} />
      <div
        className={"flex-1 overflow-y-auto scrollbar-thin px-6 py-5 prose-mail text-ink-muted leading-relaxed" + bodyClass}
        dangerouslySetInnerHTML={{ __html: html }}
      />
      {msg.direction !== "out" && (
        <form
          className="border-t border-paper-line px-6 py-3 space-y-2"
          onSubmit={(e) => {
            e.preventDefault();
            if (!replyBodyValue.trim() || busy) return;
            onReply(replyBodyValue, replyFrom, replyFormat).then(() => {
              setReply("");
              setReplyHtml("");
              setReplyHtmlResetKey((k) => k + 1);
            });
          }}
        >
          <label className="block text-xs text-ink-dim">
            <span className="text-ink-dim/70">{t("from")} </span>
            {catchallDomain ? (
              <React.Fragment>
                <input
                  type="text"
                  list="reply-from-suggestions"
                  value={replyFrom}
                  onChange={(e) => setReplyFrom(e.target.value)}
                  placeholder={"anything@" + catchallDomain}
                  className="mt-1 w-full bg-paper border border-paper-line rounded px-2 py-1 text-sm text-ink placeholder:text-ink-dim focus:outline-none focus:border-accent"
                />
                <datalist id="reply-from-suggestions">
                  {Array.from(new Set([...(addresses || []), ...(seenAddresses || [])])).map((a) => (
                    <option key={a} value={a} />
                  ))}
                </datalist>
              </React.Fragment>
            ) : (addresses || []).length > 1 ? (
              <select
                value={replyFrom}
                onChange={(e) => setReplyFrom(e.target.value)}
                className="mt-1 bg-paper border border-paper-line rounded px-2 py-1 text-sm text-ink"
              >
                {(addresses || []).map((a) => (
                  <option key={a} value={a}>
                    {a}
                  </option>
                ))}
              </select>
            ) : (
              <span className="text-ink">{replyFrom || "—"}</span>
            )}
          </label>
          <div className="flex gap-1">
            {[
              ["text", t("fmt_plain")],
              ["markdown", t("fmt_markdown")],
              ["html", t("fmt_html")],
            ].map(([value, name]) => (
              <button
                key={value}
                type="button"
                onClick={() => switchReplyFormat(value)}
                className={
                  "text-[0.7rem] px-2 py-1 rounded border " +
                  (replyFormat === value
                    ? "border-accent text-accent bg-accent-soft"
                    : "border-paper-line text-ink-dim hover:text-ink")
                }
              >
                {name}
              </button>
            ))}
          </div>
          {replyFormat === "html" ? (
            <HtmlWysiwygEditor
              initialHtml={replyHtml}
              resetKey={replyHtmlResetKey}
              onChange={setReplyHtml}
              placeholder={t("html_placeholder_wysiwyg")}
              className="w-full bg-paper border border-paper-line rounded px-4 py-3 text-sm text-ink min-h-[16rem] max-h-[50vh] prose-mail wysiwyg-editable overflow-y-auto focus:outline-none focus:border-accent"
            />
          ) : replyFormat === "markdown" ? (
            <MarkdownSplitEditor
              value={reply}
              onChange={setReply}
              token={token}
              placeholder={t("md_placeholder")}
              fieldClass="w-full bg-paper border border-paper-line rounded px-3 py-2 text-sm text-ink placeholder:text-ink-dim focus:outline-none focus:border-accent"
            />
          ) : (
            <textarea
              value={reply}
              onChange={(e) => setReply(e.target.value)}
              rows={3}
              placeholder={t("reply_placeholder")}
              className="w-full bg-paper border border-paper-line rounded px-3 py-2 text-sm text-ink placeholder:text-ink-dim focus:outline-none focus:border-accent"
            />
          )}
          <button type="submit" disabled={busy || !replyBodyValue.trim()} className={btn}>
            {t("send_reply")}
          </button>
        </form>
      )}
    </div>
  );
}
