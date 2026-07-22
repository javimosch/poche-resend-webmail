function AppLayout({
  view,
  tagView,
  setView,
  tags,
  unread,
  total,
  status,
  qInput,
  setQInput,
  onSearch,
  items,
  selected,
  setSelected,
  loading,
  checked,
  setChecked,
  selectAllPages,
  setSelectAllPages,
  onCreateTag,
  onBulk,
  onMarkAll,
  msg,
  msgTags,
  attachments,
  token,
  archived,
  busy,
  onToggleUnread,
  onStar,
  onArchive,
  onUnarchive,
  onDelete,
  onReply,
  offset,
  setOffset,
  pageSize,
}) {
  const pages = Math.max(1, Math.ceil(total / pageSize));
  const page = Math.floor(offset / pageSize) + 1;

  return (
    <div className="h-full flex">
      <Sidebar
        view={view}
        tagView={tagView}
        setView={setView}
        tags={tags}
        unread={unread}
        total={total}
        status={status}
        onCreateTag={onCreateTag}
      />
      <section className="w-[400px] shrink-0 border-r border-paper-line flex flex-col bg-paper/40">
        <div className="px-4 py-3 border-b border-paper-line flex items-center justify-between">
          <span className="text-sm text-ink-muted">{viewLabel(view, tagView)}</span>
          <div className="flex gap-1 items-center">
            <button
              disabled={offset <= 0}
              onClick={() => setOffset(Math.max(0, offset - pageSize))}
              className="text-xs px-2 py-1 border border-paper-line rounded disabled:opacity-30"
            >
              Prev
            </button>
            <span className="text-[11px] font-mono text-ink-dim px-1 tabular-nums">
              {page}/{pages}
            </span>
            <button
              disabled={offset + pageSize >= total}
              onClick={() => setOffset(offset + pageSize)}
              className="text-xs px-2 py-1 border border-paper-line rounded disabled:opacity-30"
            >
              Next
            </button>
          </div>
        </div>
        <Toolbar
          q={qInput}
          setQ={setQInput}
          onSearch={onSearch}
          selCount={selectAllPages ? total : checked.length}
          onBulk={onBulk}
          onMarkAll={onMarkAll}
          view={view}
          tags={tags}
          busy={busy}
        />
        <div className="flex-1 min-h-0">
          <MessageList
            items={items}
            selected={selected}
            onSelect={setSelected}
            onStar={onStar}
            loading={loading}
            checked={checked}
            setChecked={setChecked}
            total={total}
            selectAllPages={selectAllPages}
            setSelectAllPages={setSelectAllPages}
          />
        </div>
      </section>
      <main className="flex-1 min-w-0 bg-paper">
        <MessagePane
          msg={msg}
          msgTags={msgTags}
          attachments={attachments}
          token={token}
          archived={archived}
          busy={busy}
          onToggleUnread={onToggleUnread}
          onStar={onStar}
          onArchive={onArchive}
          onUnarchive={onUnarchive}
          onDelete={onDelete}
          onReply={onReply}
        />
      </main>
    </div>
  );
}
