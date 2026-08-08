// stripHtmlForCheck is a rough "is there anything here" check — not a
// sanitizer, just cheap enough to decide whether switching format would
// silently drop a draft the user cares about.
function stripHtmlForCheck(html) {
  return String(html || "").replace(/<[^>]*>/g, "").trim();
}

function escapeHtmlText(s) {
  return String(s || "")
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
}

// HtmlWysiwygEditor is a real rich-text editor, not a raw-HTML textarea with
// a preview button: what you see while typing is what gets sent (after the
// server sanitizes it again before delivery). It is intentionally
// uncontrolled — a controlled contentEditable that re-renders from state on
// every keystroke resets the caret to the start of the field.
function HtmlWysiwygEditor({ initialHtml, resetKey, onChange, placeholder, className }) {
  const { t } = useI18n();
  const ref = React.useRef(null);

  React.useEffect(() => {
    if (ref.current) ref.current.innerHTML = initialHtml || "";
    // resetKey (not initialHtml) drives re-seeding — initialHtml only matters
    // at the moment the editor is (re)created, not on every parent render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [resetKey]);

  const exec = (cmd, value) => {
    if (ref.current) ref.current.focus();
    document.execCommand(cmd, false, value);
    if (ref.current) onChange(ref.current.innerHTML);
  };

  const toolBtn =
    "px-2 py-1 rounded border border-paper-line text-xs text-ink-muted hover:border-accent hover:text-accent leading-none";

  return (
    <div className="space-y-1.5">
      {/* onMouseDown preventDefault keeps the selection inside the editable
          area — otherwise clicking a toolbar button steals focus first and
          execCommand has nothing to act on. */}
      <div className="flex flex-wrap gap-1" onMouseDown={(e) => e.preventDefault()}>
        <button type="button" title={t("wysiwyg_bold")} className={toolBtn + " font-bold"} onClick={() => exec("bold")}>
          B
        </button>
        <button type="button" title={t("wysiwyg_italic")} className={toolBtn + " italic"} onClick={() => exec("italic")}>
          I
        </button>
        <button type="button" title={t("wysiwyg_underline")} className={toolBtn + " underline"} onClick={() => exec("underline")}>
          U
        </button>
        <button type="button" title={t("wysiwyg_h1")} className={toolBtn} onClick={() => exec("formatBlock", "H1")}>
          H1
        </button>
        <button type="button" title={t("wysiwyg_quote")} className={toolBtn} onClick={() => exec("formatBlock", "BLOCKQUOTE")}>
          “ ”
        </button>
        <button type="button" title={t("wysiwyg_ul")} className={toolBtn} onClick={() => exec("insertUnorderedList")}>
          • ―
        </button>
        <button type="button" title={t("wysiwyg_ol")} className={toolBtn} onClick={() => exec("insertOrderedList")}>
          1. ―
        </button>
        <button
          type="button"
          title={t("wysiwyg_link")}
          className={toolBtn}
          onClick={() => {
            const url = window.prompt(t("wysiwyg_link_prompt"), "https://");
            if (url) exec("createLink", url);
          }}
        >
          🔗
        </button>
        <button type="button" title={t("wysiwyg_clear")} className={toolBtn} onClick={() => exec("removeFormat")}>
          Tx
        </button>
      </div>
      <div
        ref={ref}
        contentEditable
        suppressContentEditableWarning
        onInput={(e) => onChange(e.currentTarget.innerHTML)}
        data-placeholder={placeholder}
        className={className}
      />
    </div>
  );
}

// MarkdownSplitEditor renders source and rendered output side by side (stacked
// on narrow screens) so the preview is always live instead of a manual
// toggle that goes stale the moment you type another character.
function MarkdownSplitEditor({ value, onChange, token, placeholder, fieldClass }) {
  const { t } = useI18n();
  const [html, setHtml] = React.useState("");
  const [err, setErr] = React.useState("");

  React.useEffect(() => {
    if (!value.trim()) {
      setHtml("");
      setErr("");
      return;
    }
    const id = setTimeout(() => {
      renderBodyPreview(token, value, "markdown")
        .then((d) => {
          setHtml(d.html || "");
          setErr("");
        })
        .catch((e) => setErr(String(e.message || e)));
    }, 300);
    return () => clearTimeout(id);
  }, [value, token]);

  return (
    <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
      <textarea
        value={value}
        onChange={(e) => onChange(e.target.value)}
        rows={22}
        placeholder={placeholder}
        className={fieldClass + " resize-y font-mono text-[0.9rem] min-h-[50vh]"}
      />
      <div className="min-h-[50vh] max-h-[70vh] rounded border border-paper-line bg-paper px-4 py-3 prose-mail prose-mail--html overflow-y-auto text-sm">
        {err ? (
          <p className="text-red-400 text-xs">{t("preview_failed", err)}</p>
        ) : value.trim() ? (
          <div dangerouslySetInnerHTML={{ __html: html || "<p>…</p>" }} />
        ) : (
          <p className="text-ink-dim text-xs italic">{t("empty")}</p>
        )}
      </div>
    </div>
  );
}

function ComposeModal({ open, onClose, minimized, setMinimized, onSend, addresses, defaultFrom, busy, token }) {
  const { t } = useI18n();
  const [from, setFrom] = React.useState(defaultFrom || "");
  const [to, setTo] = React.useState("");
  const [cc, setCc] = React.useState("");
  const [bcc, setBcc] = React.useState("");
  const [showCc, setShowCc] = React.useState(false);
  const [subject, setSubject] = React.useState("");
  // Plain and Markdown share one buffer — both are raw-text surfaces, so
  // nothing is lost switching between them. HTML gets its own buffer because
  // it is edited visually (WYSIWYG), not as source text.
  const [text, setText] = React.useState("");
  const [htmlBody, setHtmlBody] = React.useState("");
  const [htmlResetKey, setHtmlResetKey] = React.useState(0);
  // Plain stays the default: it is what most replies are, and it cannot be
  // mangled by a converter.
  const [format, setFormat] = React.useState("text");
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
    setHtmlBody("");
    setHtmlResetKey((k) => k + 1);
    setFiles([]);
    if (fileInput.current) fileInput.current.value = "";
  };

  // Body content lives in two shapes (raw text vs. rendered HTML) that don't
  // convert losslessly in both directions. Rather than let a format switch
  // silently show stale, wrongly-interpreted content (the "prints raw text"
  // bug — Markdown source rendered as if it were already HTML), each switch
  // either carries the draft forward through a real conversion or asks
  // before it would otherwise vanish.
  const switchFormat = (next) => {
    if (next === format) return;
    if (format === "html" && next !== "html") {
      const hasContent = stripHtmlForCheck(htmlBody).length > 0;
      if (hasContent && !text.trim()) {
        if (!window.confirm(t("switch_format_confirm"))) return;
        setHtmlBody("");
        setHtmlResetKey((k) => k + 1);
      }
    }
    if (next === "html" && format !== "html" && text.trim() && !stripHtmlForCheck(htmlBody)) {
      renderBodyPreview(token, text, format)
        .then((d) => {
          setHtmlBody(d.html || "");
          setHtmlResetKey((k) => k + 1);
        })
        .catch(() => {
          const escaped = text
            .split("\n")
            .map((line) => "<p>" + escapeHtmlText(line) + "</p>")
            .join("");
          setHtmlBody(escaped);
          setHtmlResetKey((k) => k + 1);
        });
    }
    setFormat(next);
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
  const bodyValue = format === "html" ? htmlBody : text;

  const submit = (e) => {
    e.preventDefault();
    if (busy) return;
    setError("");
    setSent("");
    if (!to.trim()) return setError(t("err_recipient"));
    if (!subject.trim()) return setError(t("err_subject"));
    if (!bodyValue.trim()) return setError(t("err_body"));
    onSend({
      from,
      to: to.trim(),
      cc: cc.trim(),
      bcc: bcc.trim(),
      subject: subject.trim(),
      text: bodyValue,
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

  const closeAndReset = () => {
    onClose();
    setMinimized(false);
  };

  if (minimized) {
    return (
      <div
        className="fixed bottom-0 right-4 z-50 w-72 bg-paper-raised border border-b-0 border-paper-line rounded-t-lg shadow-2xl cursor-pointer"
        onClick={() => setMinimized(false)}
      >
        <header className="px-4 py-2.5 flex items-center justify-between gap-2">
          <span className="text-sm text-ink truncate">{subject.trim() || t("new_message")}</span>
          <span className="flex items-center gap-1 shrink-0">
            <button
              type="button"
              title={t("expand")}
              onClick={(e) => { e.stopPropagation(); setMinimized(false); }}
              className="w-6 h-6 flex items-center justify-center rounded text-ink-muted hover:text-accent hover:bg-accent-soft leading-none"
            >
              ▢
            </button>
            <button
              type="button"
              title={t("close")}
              onClick={(e) => { e.stopPropagation(); closeAndReset(); }}
              className="w-6 h-6 flex items-center justify-center rounded text-ink-muted hover:text-accent hover:bg-accent-soft leading-none"
            >
              ✕
            </button>
          </span>
        </header>
      </div>
    );
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center bg-black/60 px-0 py-0 md:px-4 md:py-6"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <form
        onSubmit={submit}
        className="w-full h-full md:h-[92vh] md:w-[94vw] md:max-w-[1400px] bg-paper-raised border border-paper-line md:rounded-lg shadow-2xl flex flex-col max-h-full"
      >
        <header className="px-5 py-3 border-b border-paper-line flex items-center justify-between">
          <span className="font-display text-lg text-ink">{t("new_message")}</span>
          <span className="flex items-center gap-2">
            <button type="button" title={t("minimize")} onClick={() => setMinimized(true)} className={btn}>
              ‒
            </button>
            <button type="button" onClick={closeAndReset} className={btn}>
              {t("close")}
            </button>
          </span>
        </header>

        <div className="px-5 py-4 space-y-3 overflow-y-auto scrollbar-thin flex-1">
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

          <div className="flex gap-1">
            {[
              ["text", t("fmt_plain")],
              ["markdown", t("fmt_markdown")],
              ["html", t("fmt_html")],
            ].map(([value, name]) => (
              <button
                key={value}
                type="button"
                onClick={() => switchFormat(value)}
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

          {format === "html" ? (
            <HtmlWysiwygEditor
              initialHtml={htmlBody}
              resetKey={htmlResetKey}
              onChange={setHtmlBody}
              placeholder={t("html_placeholder_wysiwyg")}
              className={
                field.replace("px-3 py-2", "px-4 py-3") +
                " min-h-[50vh] max-h-[70vh] prose-mail wysiwyg-editable overflow-y-auto"
              }
            />
          ) : format === "markdown" ? (
            <MarkdownSplitEditor
              value={text}
              onChange={setText}
              token={token}
              placeholder={t("md_placeholder")}
              fieldClass={field}
            />
          ) : (
            <textarea
              value={text}
              onChange={(e) => setText(e.target.value)}
              rows={22}
              placeholder={t("body_placeholder")}
              className={field + " resize-y min-h-[50vh]"}
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
