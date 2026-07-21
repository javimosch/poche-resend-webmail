function Sidebar({ view, tagView, setView, tags, unread, total, status, onCreateTag }) {
  const [newTag, setNewTag] = React.useState("");
  const [tagBusy, setTagBusy] = React.useState(false);
  const navBtn = (active, hasUnread) =>
    "w-full text-left px-3 py-2 rounded-md text-sm mb-0.5 " +
    (active
      ? "bg-accent-soft text-accent"
      : hasUnread
        ? "text-ink font-semibold hover:bg-paper-line/50"
        : "text-ink-muted hover:bg-paper-line/50 hover:text-ink");
  const userTags = (tags || []).filter((t) => t !== "archive");
  const label = (name, n) => (n > 0 ? name + " (" + n + ")" : name);

  const submitTag = (e) => {
    e.preventDefault();
    const name = (newTag || "").trim();
    if (!name || tagBusy) return;
    setTagBusy(true);
    onCreateTag(name)
      .then(() => setNewTag(""))
      .catch((err) => console.error(err))
      .finally(() => setTagBusy(false));
  };

  return (
    <aside className="w-52 shrink-0 border-r border-paper-line bg-paper-raised/60 flex flex-col">
      <div className="px-4 py-5 border-b border-paper-line">
        <div className="font-display text-xl tracking-tight text-ink">poche</div>
        <div className="text-[11px] uppercase tracking-[0.18em] text-ink-dim mt-1">resend webmail</div>
      </div>
      <nav className="p-2 flex-1 overflow-y-auto scrollbar-thin">
        <button
          className={navBtn(view === "inbox", (unread.inbox || 0) > 0)}
          onClick={() => setView("inbox", "")}
        >
          {label("Inbox", unread.inbox || 0)}
        </button>
        <button
          className={navBtn(view === "archive", (unread.archive || 0) > 0)}
          onClick={() => setView("archive", "")}
        >
          {label("Archive", unread.archive || 0)}
        </button>
        <div className="px-3 pt-3 pb-1 text-[10px] uppercase tracking-wider text-ink-dim">Tags</div>
        {userTags.map((name) => {
          const n = unread.tags?.[name] || 0;
          return (
            <button
              key={name}
              className={navBtn(view === "tag" && tagView === name, n > 0)}
              onClick={() => setView("tag", name)}
            >
              {label("#" + name, n)}
            </button>
          );
        })}
        <form onSubmit={submitTag} className="mt-2 px-1 flex gap-1">
          <input
            value={newTag}
            onChange={(e) => setNewTag(e.target.value)}
            placeholder="new tag"
            className="flex-1 min-w-0 bg-paper border border-paper-line rounded px-1.5 py-1 text-xs text-ink placeholder:text-ink-dim focus:outline-none focus:border-accent"
          />
          <button
            type="submit"
            disabled={tagBusy || !newTag.trim()}
            className="text-[10px] px-1.5 py-1 rounded border border-paper-line text-ink-muted hover:border-accent hover:text-accent disabled:opacity-30"
          >
            +
          </button>
        </form>
      </nav>
      <div className="px-4 py-3 border-t border-paper-line text-[11px] font-mono text-ink-dim space-y-1">
        <div>{total} in view</div>
        <div className={status?.poche_ok ? "text-accent" : "text-red-400"}>
          poche {status?.poche_ok ? "ok" : "down"}
        </div>
      </div>
    </aside>
  );
}

function Toolbar({ q, setQ, onSearch, selCount, onBulk, onMarkAll, view, tags, busy }) {
  const n = selCount;
  const btn =
    "text-xs px-2.5 py-1 rounded border border-paper-line text-ink-muted hover:border-accent hover:text-accent disabled:opacity-30";
  const userTags = (tags || []).filter((t) => t !== "archive");
  return (
    <div className="px-3 py-2 border-b border-paper-line space-y-2">
      <form
        className="flex gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          onSearch();
        }}
      >
        <input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="Filter subject / from / body…"
          className="flex-1 bg-paper border border-paper-line rounded px-2 py-1.5 text-sm text-ink placeholder:text-ink-dim focus:outline-none focus:border-accent"
        />
        <button type="submit" className={btn} disabled={busy}>
          Search
        </button>
      </form>
      <div className="flex flex-wrap gap-1.5 items-center">
        <button className={btn} disabled={busy || n === 0} onClick={() => onBulk("mark_read")}>
          Mark read ({n})
        </button>
        {view === "archive" ? (
          <button className={btn} disabled={busy || n === 0} onClick={() => onBulk("unarchive")}>
            Unarchive ({n})
          </button>
        ) : (
          <button className={btn} disabled={busy || n === 0} onClick={() => onBulk("archive")}>
            Archive ({n})
          </button>
        )}
        <button className={btn} disabled={busy || n === 0} onClick={() => onBulk("delete")}>
          Delete ({n})
        </button>
        <button className={btn} disabled={busy} onClick={onMarkAll}>
          Mark all read
        </button>
        {n > 0 && userTags.length > 0 && (
          <select
            className={btn + " bg-paper"}
            defaultValue=""
            disabled={busy}
            onChange={(e) => {
              const t = e.target.value;
              e.target.value = "";
              if (t) onBulk("tag", t);
            }}
          >
            <option value="">Tag ({n})…</option>
            {userTags.map((t) => (
              <option key={t} value={t}>
                #{t}
              </option>
            ))}
          </select>
        )}
      </div>
    </div>
  );
}

function MessageList({
  items,
  selected,
  onSelect,
  loading,
  checked,
  setChecked,
  total,
  selectAllPages,
  setSelectAllPages,
}) {
  const allIds = items.map((m) => m.id);
  const pageOn = allIds.length > 0 && allIds.every((id) => checked.includes(id));
  const multiPage = total > allIds.length && allIds.length > 0;

  if (loading) return <div className="p-6 text-ink-muted text-sm">Loading…</div>;
  if (!items.length) {
    return <div className="p-8 text-ink-muted text-sm">No messages in this view.</div>;
  }
  return (
    <div className="overflow-y-auto scrollbar-thin h-full">
      <div className="px-3 py-2 border-b border-paper-line space-y-1.5 text-xs text-ink-dim">
        <div className="flex items-center gap-2">
          <input
            type="checkbox"
            checked={pageOn || selectAllPages}
            onChange={(e) => {
              setSelectAllPages(false);
              setChecked(e.target.checked ? allIds.slice() : []);
            }}
          />
          <span>Select page</span>
        </div>
        {pageOn && multiPage && (
          <label className="flex items-center gap-2 text-accent cursor-pointer">
            <input
              type="checkbox"
              checked={selectAllPages}
              onChange={(e) => setSelectAllPages(e.target.checked)}
            />
            <span>Select all {total} messages (all pages)</span>
          </label>
        )}
      </div>
      {items.map((m) => {
        const active = selected === m.id;
        const on = selectAllPages || checked.includes(m.id);
        return (
          <div
            key={m.id}
            className={
              "mail-row flex gap-2 px-3 py-2.5 border-b border-paper-line border-l-2 border-l-transparent " +
              (active ? "active" : "")
            }
          >
            <input
              type="checkbox"
              className="mt-1 shrink-0"
              checked={on}
              onChange={(e) => {
                e.stopPropagation();
                setSelectAllPages(false);
                setChecked(
                  e.target.checked ? checked.concat([m.id]) : checked.filter((x) => x !== m.id)
                );
              }}
            />
            <button className="flex-1 min-w-0 text-left" onClick={() => onSelect(m.id)}>
              <div className="flex items-baseline justify-between gap-2">
                <span className={"text-sm truncate " + (m.unread ? "font-semibold text-ink" : "text-ink-muted")}>
                  {m.from_addr}
                </span>
                <span className="text-[11px] font-mono text-ink-dim shrink-0 tabular-nums">
                  {formatWhen(m.created_at)}
                </span>
              </div>
              <div className={"text-sm truncate mt-0.5 " + (m.unread ? "text-ink" : "text-ink-muted")}>
                {m.subject}
              </div>
              <div className="text-xs text-ink-dim truncate mt-0.5">{m.preview}</div>
            </button>
          </div>
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
