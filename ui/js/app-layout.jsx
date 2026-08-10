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
  addrField,
  setAddrField,
  addrValue,
  setAddrValue,
  addrFacets,
  items,
  selected,
  setSelected,
  loading,
  checked,
  setChecked,
  selectAllPages,
  setSelectAllPages,
  onCreateTag,
  onRenameTag,
  onDeleteTag,
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
  account,
  accounts,
  activeKey,
  onSwitchAccount,
  onAddAccount,
  onRemoveAccount,
  onLogout,
  composeOpen,
  brand,
  onBack,
  setComposeOpen,
  composeMinimized,
  setComposeMinimized,
  sendAddresses,
  sendCatchallDomain,
  sendSeenAddresses,
  onCompose,
}) {
  const { t } = useI18n();
  // Below md the three panes do not fit side by side: the sidebar becomes a
  // drawer, and the list and the message take turns on the full width.
  const [navOpen, setNavOpen] = React.useState(false);
  const showPane = !!msg;
  const pages = Math.max(1, Math.ceil(total / pageSize));
  const page = Math.floor(offset / pageSize) + 1;

  return (
    <div className="h-full flex flex-col md:flex-row">
      <header className="md:hidden flex items-center gap-3 px-3 py-2 border-b border-paper-line bg-paper-raised/60 shrink-0">
        <button
          onClick={() => setNavOpen(true)}
          className="text-ink-muted hover:text-accent px-1 text-lg leading-none"
          aria-label={t("tags")}
        >
          ☰
        </button>
        <span className="text-sm text-ink truncate">{viewLabel(view, tagView, t)}</span>
        <span className="ml-auto text-[0.7rem] text-ink-dim tabular-nums">{total}</span>
      </header>

      {navOpen && (
        <div
          className="md:hidden fixed inset-0 z-30 bg-black/50"
          onClick={() => setNavOpen(false)}
        />
      )}

      <div
        className={
          "z-40 shrink-0 transition-transform md:transition-none " +
          "max-md:fixed max-md:inset-y-0 max-md:left-0 max-md:w-64 " +
          (navOpen ? "max-md:translate-x-0" : "max-md:-translate-x-full")
        }
      >
      <Sidebar
        view={view}
        tagView={tagView}
        setView={(v, tag) => { setView(v, tag); setNavOpen(false); }}
        tags={tags}
        unread={unread}
        total={total}
        status={status}
        onCreateTag={onCreateTag}
        onRenameTag={onRenameTag}
        onDeleteTag={onDeleteTag}
        onLogout={onLogout}
        token={token}
        account={account}
        accounts={accounts}
        activeKey={activeKey}
        onSwitchAccount={onSwitchAccount}
        onAddAccount={() => { onAddAccount(); setNavOpen(false); }}
        onRemoveAccount={onRemoveAccount}
        onComposeClick={() => { setComposeOpen(true); setComposeMinimized(false); setNavOpen(false); }}
        brand={brand}
        onCloseNav={() => setNavOpen(false)}
      />
      </div>
      <section className={"w-full md:w-[400px] shrink-0 border-r border-paper-line flex-col bg-paper/40 min-h-0 " + (showPane ? "hidden md:flex" : "flex")}>
        <div className="px-4 py-3 border-b border-paper-line flex items-center justify-between">
          <span className="text-sm text-ink-muted">{viewLabel(view, tagView, t)}</span>
          <div className="flex gap-1 items-center">
            <button
              disabled={offset <= 0}
              onClick={() => setOffset(Math.max(0, offset - pageSize))}
              className="text-xs px-2 py-1 border border-paper-line rounded disabled:opacity-30"
            >
              {t("prev")}
            </button>
            <span className="text-[0.76rem] font-mono text-ink-dim px-1 tabular-nums">
              {page}/{pages}
            </span>
            <button
              disabled={offset + pageSize >= total}
              onClick={() => setOffset(offset + pageSize)}
              className="text-xs px-2 py-1 border border-paper-line rounded disabled:opacity-30"
            >
              {t("next")}
            </button>
          </div>
        </div>
        <Toolbar
          q={qInput}
          setQ={setQInput}
          onSearch={onSearch}
          addrField={addrField}
          setAddrField={setAddrField}
          addrValue={addrValue}
          setAddrValue={setAddrValue}
          addrFacets={addrFacets}
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
      <main className={"flex-1 min-w-0 bg-paper " + (showPane ? "block" : "hidden md:block")}>
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
          account={account}
          addresses={sendAddresses}
          catchallDomain={sendCatchallDomain}
          seenAddresses={sendSeenAddresses}
          onBack={onBack}
        />
      </main>
      <ComposeModal
        open={composeOpen}
        onClose={() => setComposeOpen(false)}
        minimized={composeMinimized}
        setMinimized={setComposeMinimized}
        onSend={onCompose}
        addresses={sendAddresses}
        catchallDomain={sendCatchallDomain}
        seenAddresses={sendSeenAddresses}
        defaultFrom={account?.address || (sendAddresses || [])[0] || ""}
        token={token}
        busy={busy}
      />
    </div>
  );
}
