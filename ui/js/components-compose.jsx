function ComposeModal({ open, onClose, onSend, addresses, defaultFrom, busy, token }) {
  const [from, setFrom] = React.useState(defaultFrom || "");
  const [to, setTo] = React.useState("");
  const [cc, setCc] = React.useState("");
  const [bcc, setBcc] = React.useState("");
  const [showCc, setShowCc] = React.useState(false);
  const [subject, setSubject] = React.useState("");
  const [text, setText] = React.useState("");
  // Plain stays the default: it is what most replies are, and it cannot be
  // mangled by a converter.
  const [format, setFormat] = React.useState("text");
  const [preview, setPreview] = React.useState(false);
  const [previewHtml, setPreviewHtml] = React.useState("");
  const [error, setError] = React.useState("");
  const [sent, setSent] = React.useState("");

  React.useEffect(() => {
    if (!open) return;
    setError("");
    setSent("");
    setFrom((prev) => prev || defaultFrom || (addresses || [])[0] || "");
  }, [open, defaultFrom, addresses]);

  if (!open) return null;

  const field =
    "w-full bg-paper border border-paper-line rounded px-3 py-2 text-sm text-ink placeholder:text-ink-dim focus:outline-none focus:border-accent";
  const btn =
    "text-xs px-2.5 py-1 rounded border border-paper-line text-ink-muted hover:border-accent hover:text-accent";

  const reset = () => {
    setTo("");
    setCc("");
    setBcc("");
    setShowCc(false);
    setSubject("");
    setText("");
    setPreview(false);
  };

  const submit = (e) => {
    e.preventDefault();
    if (busy) return;
    setError("");
    setSent("");
    if (!to.trim()) return setError("Add at least one recipient.");
    if (!subject.trim()) return setError("Subject is required.");
    if (!text.trim()) return setError("Message body is required.");
    onSend({ from, to: to.trim(), cc: cc.trim(), bcc: bcc.trim(), subject: subject.trim(), text, format })
      .then((res) => {
        setSent("Sent to " + (Array.isArray(res?.to) ? res.to.join(", ") : to.trim()));
        reset();
      })
      .catch((err) => setError(String(err.message || err)));
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center bg-black/60 px-4 py-10"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <form
        onSubmit={submit}
        className="w-full max-w-2xl bg-paper-raised border border-paper-line rounded-lg shadow-2xl flex flex-col max-h-full"
      >
        <header className="px-5 py-3 border-b border-paper-line flex items-center justify-between">
          <span className="font-display text-lg text-ink">New message</span>
          <button type="button" onClick={onClose} className={btn}>
            Close
          </button>
        </header>

        <div className="px-5 py-4 space-y-3 overflow-y-auto scrollbar-thin">
          <label className="block">
            <span className="text-[0.7rem] uppercase tracking-wider text-ink-dim">From</span>
            {(addresses || []).length > 1 ? (
              <select value={from} onChange={(e) => setFrom(e.target.value)} className={field}>
                {(addresses || []).map((a) => (
                  <option key={a} value={a}>
                    {a}
                  </option>
                ))}
              </select>
            ) : (
              <div className="text-sm text-ink py-2">{from || defaultFrom || "—"}</div>
            )}
          </label>

          <label className="block">
            <span className="text-[0.7rem] uppercase tracking-wider text-ink-dim">To</span>
            <input
              value={to}
              onChange={(e) => setTo(e.target.value)}
              placeholder="someone@example.com, other@example.com"
              className={field}
              autoFocus
            />
          </label>

          {showCc ? (
            <div className="grid grid-cols-2 gap-3">
              <label className="block">
                <span className="text-[0.7rem] uppercase tracking-wider text-ink-dim">Cc</span>
                <input value={cc} onChange={(e) => setCc(e.target.value)} className={field} />
              </label>
              <label className="block">
                <span className="text-[0.7rem] uppercase tracking-wider text-ink-dim">Bcc</span>
                <input value={bcc} onChange={(e) => setBcc(e.target.value)} className={field} />
              </label>
            </div>
          ) : (
            <button type="button" onClick={() => setShowCc(true)} className={btn}>
              Add Cc / Bcc
            </button>
          )}

          <label className="block">
            <span className="text-[0.7rem] uppercase tracking-wider text-ink-dim">Subject</span>
            <input value={subject} onChange={(e) => setSubject(e.target.value)} className={field} />
          </label>

          <div className="flex items-center justify-between gap-2">
            <div className="flex gap-1">
              {[
                ["text", "Plain"],
                ["markdown", "Markdown"],
                ["html", "HTML"],
              ].map(([value, name]) => (
                <button
                  key={value}
                  type="button"
                  onClick={() => { setFormat(value); setPreview(false); }}
                  className={
                    "text-[0.7rem] px-2 py-1 rounded border " +
                    (format === value
                      ? "border-accent text-accent bg-accent-soft"
                      : "border-paper-line text-ink-dim hover:text-ink")
                  }
                >
                  {name}
                </button>
              ))}
            </div>
            {format !== "text" && (
              <button
                type="button"
                onClick={() => {
                  const next = !preview;
                  setPreview(next);
                  if (next) {
                    setPreviewHtml("");
                    // Ask the server so the preview is the very same output that
                    // would be sent, sanitizer included.
                    renderBodyPreview(token, text, format)
                      .then((d) => setPreviewHtml(d.html || "<p><em>(empty)</em></p>"))
                      .catch((err) => setPreviewHtml("<p>Preview failed: " + String(err.message || err) + "</p>"));
                  }
                }}
                className={btn}
              >
                {preview ? "Edit" : "Preview"}
              </button>
            )}
          </div>

          {preview && format !== "text" ? (
            <div
              className="min-h-[16rem] rounded border border-paper-line bg-paper px-4 py-3 prose-mail prose-mail--html overflow-y-auto"
              dangerouslySetInnerHTML={{ __html: previewHtml || "<p>…</p>" }}
            />
          ) : (
            <textarea
              value={text}
              onChange={(e) => setText(e.target.value)}
              rows={14}
              placeholder={
                format === "markdown"
                  ? "# Bonjour\n\nWrite **Markdown** — it is converted to HTML when sent."
                  : format === "html"
                    ? "<p>Write HTML — it is sanitized before sending.</p>"
                    : "Write your message…"
              }
              className={field + " resize-y font-mono text-[0.9rem]"}
            />
          )}

          {error && <div className="text-sm text-red-400">{error}</div>}
          {sent && <div className="text-sm text-accent">{sent}</div>}
        </div>

        <footer className="px-5 py-3 border-t border-paper-line flex items-center gap-2">
          <button type="submit" disabled={busy} className={btn + " disabled:opacity-40"}>
            {busy ? "Sending…" : "Send"}
          </button>
          <span className="text-xs text-ink-dim">
            {format === "markdown"
              ? "Markdown → HTML on send"
              : format === "html"
                ? "HTML is sanitized before sending"
                : "Plain text"}
            {" · attachments not supported yet"}
          </span>
        </footer>
      </form>
    </div>
  );
}
