function ComposeModal({ open, onClose, onSend, addresses, defaultFrom, busy, token }) {
  const { t } = useI18n();
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
  const [files, setFiles] = React.useState([]);
  const fileInput = React.useRef(null);
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
    setFiles([]);
    if (fileInput.current) fileInput.current.value = "";
  };

  const addFiles = (list) => {
    const chosen = Array.from(list || []);
    if (!chosen.length) return;
    setError("");
    Promise.all(
      chosen.map(
        (f) =>
          new Promise((resolve, reject) => {
            const r = new FileReader();
            r.onload = () =>
              resolve({
                filename: f.name,
                content_type: f.type || "application/octet-stream",
                bytes: f.size,
                // strip the "data:...;base64," prefix the reader adds
                content: String(r.result).split(",", 2)[1] || "",
              });
            r.onerror = () => reject(new Error(f.name));
            r.readAsDataURL(f);
          })
      )
    )
      .then((read) => setFiles((prev) => prev.concat(read)))
      .catch((err) => setError(String(err.message || err)));
  };

  const totalBytes = files.reduce((n, f) => n + (f.bytes || 0), 0);

  const submit = (e) => {
    e.preventDefault();
    if (busy) return;
    setError("");
    setSent("");
    if (!to.trim()) return setError(t("err_recipient"));
    if (!subject.trim()) return setError(t("err_subject"));
    if (!text.trim()) return setError(t("err_body"));
    onSend({
      from,
      to: to.trim(),
      cc: cc.trim(),
      bcc: bcc.trim(),
      subject: subject.trim(),
      text,
      format,
      attachments: files.map((f) => ({
        filename: f.filename,
        content_type: f.content_type,
        content: f.content,
      })),
    })
      .then((res) => {
        setSent(t("sent_to", Array.isArray(res?.to) ? res.to.join(", ") : to.trim()));
        reset();
      })
      .catch((err) => setError(String(err.message || err)));
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center bg-black/60 px-0 py-0 md:px-4 md:py-10"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <form
        onSubmit={submit}
        className="w-full h-full md:h-auto md:max-w-2xl bg-paper-raised border border-paper-line md:rounded-lg shadow-2xl flex flex-col max-h-full"
      >
        <header className="px-5 py-3 border-b border-paper-line flex items-center justify-between">
          <span className="font-display text-lg text-ink">{t("new_message")}</span>
          <button type="button" onClick={onClose} className={btn}>
            {t("close")}
          </button>
        </header>

        <div className="px-5 py-4 space-y-3 overflow-y-auto scrollbar-thin">
          <label className="block">
            <span className="text-[0.7rem] uppercase tracking-wider text-ink-dim">{t("from")}</span>
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
            <span className="text-[0.7rem] uppercase tracking-wider text-ink-dim">{t("to")}</span>
            <input
              value={to}
              onChange={(e) => setTo(e.target.value)}
              placeholder={t("to_placeholder")}
              className={field}
              autoFocus
            />
          </label>

          {showCc ? (
            <div className="grid grid-cols-2 gap-3">
              <label className="block">
                <span className="text-[0.7rem] uppercase tracking-wider text-ink-dim">{t("cc")}</span>
                <input value={cc} onChange={(e) => setCc(e.target.value)} className={field} />
              </label>
              <label className="block">
                <span className="text-[0.7rem] uppercase tracking-wider text-ink-dim">{t("bcc")}</span>
                <input value={bcc} onChange={(e) => setBcc(e.target.value)} className={field} />
              </label>
            </div>
          ) : (
            <button type="button" onClick={() => setShowCc(true)} className={btn}>
              {t("add_cc")}
            </button>
          )}

          <label className="block">
            <span className="text-[0.7rem] uppercase tracking-wider text-ink-dim">{t("subject")}</span>
            <input value={subject} onChange={(e) => setSubject(e.target.value)} className={field} />
          </label>

          <div className="flex items-center justify-between gap-2">
            <div className="flex gap-1">
              {[
                ["text", t("fmt_plain")],
                ["markdown", t("fmt_markdown")],
                ["html", t("fmt_html")],
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
                      .then((d) => setPreviewHtml(d.html || "<p><em>" + t("empty") + "</em></p>"))
                      .catch((err) => setPreviewHtml("<p>" + t("preview_failed", String(err.message || err)) + "</p>"));
                  }
                }}
                className={btn}
              >
                {preview ? t("edit") : t("preview")}
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
                  ? t("md_placeholder")
                  : format === "html"
                    ? t("html_placeholder")
                    : t("body_placeholder")
              }
              className={field + " resize-y font-mono text-[0.9rem]"}
            />
          )}

          <div className="space-y-2">
            <div className="flex items-center gap-2 flex-wrap">
              <button type="button" onClick={() => fileInput.current && fileInput.current.click()} className={btn}>
                📎 {t("attach")}
              </button>
              {files.length > 0 && (
                <span className="text-[0.7rem] text-ink-dim">
                  {t("attach_total", files.length, fmtBytes(totalBytes))}
                </span>
              )}
              <input
                ref={fileInput}
                type="file"
                multiple
                className="hidden"
                onChange={(e) => {
                  addFiles(e.target.files);
                  e.target.value = "";
                }}
              />
            </div>
            {files.length > 0 && (
              <ul className="space-y-1">
                {files.map((f, i) => (
                  <li
                    key={f.filename + i}
                    className="flex items-center gap-2 text-[0.76rem] text-ink-muted border border-paper-line rounded px-2 py-1"
                  >
                    <span className="truncate flex-1" title={f.filename}>{f.filename}</span>
                    <span className="text-ink-dim tabular-nums shrink-0">{fmtBytes(f.bytes)}</span>
                    <button
                      type="button"
                      title={t("remove")}
                      onClick={() => setFiles(files.filter((_, j) => j !== i))}
                      className="text-ink-dim hover:text-accent shrink-0"
                    >
                      ✕
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>

          {error && <div className="text-sm text-red-400">{error}</div>}
          {sent && <div className="text-sm text-accent">{sent}</div>}
        </div>

        <footer className="px-5 py-3 border-t border-paper-line flex items-center gap-2">
          <button type="submit" disabled={busy} className={btn + " disabled:opacity-40"}>
            {busy ? t("sending") : t("send")}
          </button>
          <span className="text-xs text-ink-dim">
            {format === "markdown"
              ? t("note_markdown")
              : format === "html"
                ? t("note_html")
                : t("note_plain")}
            {t("note_attachments")}
          </span>
        </footer>
      </form>
    </div>
  );
}
