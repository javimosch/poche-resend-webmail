function Sidebar({ view, tagView, setView, tags, unread, total, status, onCreateTag, onLogout, token, account, onComposeClick }) {
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
          onClick={onComposeClick}
          className="w-full mb-2 px-3 py-2 rounded-md text-sm bg-accent-soft text-accent border border-accent/40 hover:bg-accent/20"
        >
          Compose
        </button>
        <button
          className={navBtn(view === "inbox", (unread.inbox || 0) > 0)}
          onClick={() => setView("inbox", "")}
        >
          {label("Inbox", unread.inbox || 0)}
        </button>
        <button
          className={navBtn(view === "sent", false)}
          onClick={() => setView("sent", "")}
        >
          Sent
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
      <div className="px-4 py-3 border-t border-paper-line text-[11px] font-mono text-ink-dim space-y-2">
        {account?.address && (
          <div className="space-y-0.5">
            <div className="text-ink-muted truncate" title={account.address}>
              {account.address}
            </div>
            {account.name && account.name !== account.address && (
              <div className="truncate" title={account.name}>{account.name}</div>
            )}
          </div>
        )}
        <div>{total} in view</div>
        <div className={status?.poche_ok ? "text-accent" : "text-red-400"}>
          poche {status?.poche_ok ? "ok" : "down"}
        </div>
        <StorageBar token={token} />
        {onLogout && (
          <button
            onClick={onLogout}
            className="mt-1 text-ink-dim hover:text-accent text-[11px] underline"
          >
            Sign out
          </button>
        )}
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
        <button className={btn} disabled={busy || n === 0} onClick={() => onBulk("star")}>
          Star ({n})
        </button>
        <button className={btn} disabled={busy || n === 0} onClick={() => onBulk("unstar")}>
          Unstar ({n})
        </button>
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
  onStar,
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
            <button
              className={
                "text-base leading-none shrink-0 " +
                (m.starred ? "text-accent" : "text-ink-dim hover:text-accent")
              }
              title={m.starred ? "Unstar" : "Star"}
              onClick={(e) => {
                e.stopPropagation();
                onStar(m.id, !m.starred);
              }}
            >
              {m.starred ? "★" : "☆"}
            </button>
            <button className="flex-1 min-w-0 text-left" onClick={() => onSelect(m.id)}>
              <div className="flex items-baseline justify-between gap-2">
                <span className={"text-sm truncate " + (m.unread ? "font-semibold text-ink" : "text-ink-muted")}>
                  {m.direction === "out" ? "To " + (m.to_addr || "") : m.from_addr}
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

function StorageBar({ token }) {
  const [usage, setUsage] = React.useState(null);
  const [loading, setLoading] = React.useState(true);

  React.useEffect(() => {
    if (!token) return;
    let active = true;
    const fetchUsage = () => {
      fetch("/api/mailbox/usage", { headers: { Authorization: "Bearer " + token } })
        .then((r) => r.json())
        .then((j) => { if (active && j.ok) { setUsage(j.data); setLoading(false); } })
        .catch(() => {});
    };
    fetchUsage();
    const interval = setInterval(fetchUsage, 30000);
    return () => { active = false; clearInterval(interval); };
  }, [token]);

  if (loading || !usage) return null;

  const fmt = (b) => {
    if (b >= 1073741824) return (b / 1073741824).toFixed(1) + " GB";
    if (b >= 1048576) return (b / 1048576).toFixed(1) + " MB";
    if (b >= 1024) return (b / 1024).toFixed(0) + " KB";
    return b + " B";
  };

  const pct = Math.min(100, Math.round(usage.percent * 10) / 10);
  const barWidth = Math.max(2, usage.percent);
  const barColor = pct > 90 ? "bg-red-400" : pct > 70 ? "bg-yellow-400" : "bg-accent";

  return (
    <div className="space-y-1">
      <div className="flex justify-between text-[10px] text-ink-dim">
        <span>Storage</span>
        <span className="tabular-nums">{fmt(usage.used_bytes)} / {fmt(usage.max_bytes)}</span>
      </div>
      <div className="h-1.5 bg-paper-line rounded-full overflow-hidden">
        <div
          className={"h-full rounded-full transition-all " + barColor}
          style={{ width: barWidth + "%" }}
        />
      </div>
      <div className="text-[10px] text-ink-dim tabular-nums">
        {pct}% · {usage.message_count} msgs
      </div>
    </div>
  );
}

