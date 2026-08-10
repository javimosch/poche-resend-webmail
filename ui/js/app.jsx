const { useState, useEffect, useCallback, useRef } = React;

function App() {
  const { t } = useI18n();
  const { cfg, err, token, account, accounts, activeKey, addAccount, switchAccount, removeAccount } = useConfig();
  const [tokenInput, setTokenInput] = useState(token || "");
  const [addingAccount, setAddingAccount] = useState(false);
  const [address, setAddress] = useState("");
  const [password, setPassword] = useState("");
  const [loginMode, setLoginMode] = useState("password");
  const [loginError, setLoginError] = useState("");
  const [resetToken, setResetToken] = useState(() => {
    try {
      return new URLSearchParams(location.search).get("reset_token") || "";
    } catch (_) { return ""; }
  });
  const resetMode = resetToken !== "";
  if (resetMode && loginMode !== "reset") setLoginMode("reset");
  const [view, setViewState] = useState("inbox");
  const [tagView, setTagView] = useState("");
  const [tags, setTags] = useState([]);
  const [unread, setUnread] = useState({ inbox: 0, archive: 0, tags: {} });
  const [qInput, setQInput] = useState("");
  const [q, setQ] = useState("");
  const [addrField, setAddrField] = useState("");
  const [addrValue, setAddrValue] = useState("");
  const [addrFacets, setAddrFacets] = useState([]);
  const [items, setItems] = useState([]);
  const [total, setTotal] = useState(0);
  const [selected, setSelected] = useState(null);
  const [msg, setMsg] = useState(null);
  const [msgTags, setMsgTags] = useState([]);
  const [attachments, setAttachments] = useState([]);
  const [thread, setThread] = useState([]);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [status, setStatus] = useState(null);
  const [offset, setOffset] = useState(0);
  const [checked, setChecked] = useState([]);
  const [selectAllPages, setSelectAllPages] = useState(false);
  const [composeOpen, setComposeOpen] = useState(false);
  const [composeMinimized, setComposeMinimized] = useState(false);
  const [sendAddresses, setSendAddresses] = useState([]);
  const [sendCatchallDomain, setSendCatchallDomain] = useState("");
  const [sendSeenAddresses, setSendSeenAddresses] = useState([]);
  const pageSize = 50;
  const ctxRef = useRef({ view: "inbox", tagView: "", q: "" });

  const setView = (v, t) => {
    setViewState(v);
    setTagView(t || "");
  };

  const loadTags = useCallback(() => {
    if (!token) return Promise.resolve([]);
    return apiFetch(token, "/api/tags")
      .then((data) => {
        const names = tagNamesFromPage(data);
        setTags(names);
        return names;
      })
      .catch(() => []);
  }, [token]);

  const loadUnread = useCallback(
    (tagNames) => {
      if (!token) return;
      const names = (tagNames || tags).filter((t) => t !== "archive");
      const countOne = (v, t) =>
        apiFetch(token, buildUnreadCountPath(v, t)).then((d) => d.count || 0).catch(() => 0);
      Promise.all([
        countOne("inbox", ""),
        countOne("archive", ""),
        ...names.map((name) => countOne("tag", name)),
      ]).then((counts) => {
        const tagMap = {};
        names.forEach((name, i) => {
          tagMap[name] = counts[2 + i] || 0;
        });
        setUnread({ inbox: counts[0], archive: counts[1], tags: tagMap });
      });
    },
    [token, tags]
  );

  useEffect(() => {
    if (token) loadTags().then((names) => loadUnread(names));
  }, [token, loadTags]);

  useEffect(() => {
    if (!token) return;
    apiFetch(token, "/api/status")
      .then((d) => setStatus(d))
      .catch(() => {});
  }, [token, total]);

  useEffect(() => {
    if (!token) {
      setSendAddresses([]);
      setSendCatchallDomain("");
      setSendSeenAddresses([]);
      return;
    }
    fetchSendAddresses(token).then((r) => {
      setSendAddresses(r.addresses);
      setSendCatchallDomain(r.catchallDomain);
      setSendSeenAddresses(r.seenAddresses);
    });
  }, [token]);

  useEffect(() => {
    if (!token || !addrField) {
      setAddrFacets([]);
      return;
    }
    fetchAddressFacets(token, addrField).then((values) => {
      setAddrFacets(values);
      // A value picked under the previous field (or one that no longer has
      // any messages) wouldn't match anything in the new list — drop it
      // rather than silently keep filtering on a stale value.
      setAddrValue((v) => (values.some((f) => f.value === v) ? v : ""));
    });
  }, [token, addrField]);

  const addrFilter = addrField && addrValue ? { field: addrField, value: addrValue } : null;

  const loadList = useCallback(() => {
    if (!token) return;
    setLoading(true);
    ctxRef.current = { view, tagView, q };
    apiFetch(token, buildListPath(view, tagView, q, pageSize, offset, addrFilter))
      .then((data) => {
        setItems((data.items || []).map(rowFromItem));
        setTotal(data.total || 0);
      })
      .catch((e) => console.error(e))
      .finally(() => setLoading(false));
  }, [token, view, tagView, q, offset, addrField, addrValue]);

  useEffect(() => {
    // token is in these deps too: without it, switching accounts kept
    // whatever message id was selected under the PREVIOUS account and
    // re-fetched it under the new one's token — at best a 403 now that
    // messages.go checks mailbox ownership, at worst (before that fix) a
    // cross-tenant read.
    setSelected(null);
    setMsg(null);
    setMsgTags([]);
    setAttachments([]);
    setOffset(0);
    setChecked([]);
    setSelectAllPages(false);
  }, [token, view, tagView, q, addrField, addrValue]);

  useEffect(() => {
    loadList();
  }, [loadList]);

  const loadMsgMeta = (id) => {
    apiFetch(token, "/api/message-tags?message_id=" + encodeURIComponent(id))
      .then((data) => setMsgTags(tagNamesFromPage(data || {})))
      .catch(() => setMsgTags([]));
    apiFetch(token, "/api/messages/" + encodeURIComponent(id) + "/attachments")
      .then((data) => {
        setAttachments(
          (data.items || []).map((it) => {
            const doc = typeof it.doc === "string" ? JSON.parse(it.doc) : it.doc || {};
            return Object.assign({ id: it.id }, doc);
          })
        );
      })
      .catch(() => setAttachments([]));
  };

  useEffect(() => {
    if (!token || !selected) {
      setMsg(null);
      setMsgTags([]);
      setAttachments([]);
      setThread([]);
      return;
    }
    apiFetch(token, "/api/messages/" + encodeURIComponent(selected))
      .then((data) => {
        const doc = data.doc || data;
        const id = data.id || data._id || selected;
        setMsg(Object.assign({ id }, typeof doc === "string" ? JSON.parse(doc) : doc));
      })
      .catch((e) => console.error(e));
    loadMsgMeta(selected);
    fetchThread(token, selected).then(setThread);
  }, [token, selected]);

  const refreshAfter = () => {
    setChecked([]);
    setSelectAllPages(false);
    loadList();
    loadUnread();
    if (selected) {
      loadMsgMeta(selected);
      fetchThread(token, selected).then(setThread);
    }
    apiFetch(token, "/api/status")
      .then((d) => setStatus(d))
      .catch(() => {});
  };

  const onCreateTag = (name) =>
    createTag(token, name).then(() => loadTags().then((names) => loadUnread(names)));

  const afterTagChange = (gone) => {
    // Leaving the view of a tag that no longer exists would show an empty list
    // with no way back, so fall back to the inbox.
    if (gone && view === "tag" && tagView === gone) setView("inbox", "");
    return loadTags().then((names) => loadUnread(names));
  };

  const onRenameTag = (name, newName) =>
    renameTag(token, name, newName).then((res) => {
      if (view === "tag" && tagView === name) setView("tag", res?.name || newName);
      return loadTags().then((names) => loadUnread(names));
    });

  const onDeleteTag = (name) => deleteTag(token, name).then(() => afterTagChange(name));

  const onBulk = (action, tag) => {
    const ctx = ctxRef.current;
    if (!selectAllPages && !checked.length) return;
    setBusy(true);
    const body = { action, ids: checked };
    if (tag) body.tag = tag;
    if (selectAllPages) {
      body.all_pages = true;
      body.view = ctx.view;
      body.tag_view = ctx.tagView;
      body.q = ctx.q;
    }
    bulkFetch(token, body)
      .then(() => {
        if (selected && (selectAllPages || checked.includes(selected))) setSelected(null);
        refreshAfter();
      })
      .catch((e) => console.error(e))
      .finally(() => setBusy(false));
  };

  const onMarkAll = () => {
    setBusy(true);
    const ctx = ctxRef.current;
    bulkFetch(token, {
      action: "mark_read_all",
      view: ctx.view,
      tag_view: ctx.tagView,
      q: ctx.q,
    })
      .then(refreshAfter)
      .catch((e) => console.error(e))
      .finally(() => setBusy(false));
  };

  const onToggleUnread = () => {
    if (!msg) return;
    setBusy(true);
    bulkFetch(token, {
      action: msg.unread ? "mark_read" : "mark_unread",
      ids: [msg.id],
    })
      .then(() => {
        setMsg(Object.assign({}, msg, { unread: !msg.unread }));
        refreshAfter();
      })
      .catch((e) => console.error(e))
      .finally(() => setBusy(false));
  };

  const runOne = (action) => {
    if (!msg) return;
    setBusy(true);
    bulkFetch(token, { action, ids: [msg.id] })
      .then(() => {
        setSelected(null);
        refreshAfter();
      })
      .catch((e) => console.error(e))
      .finally(() => setBusy(false));
  };

  const onStar = (id) => {
    if (!token || !id) return;
    setBusy(true);
    toggleStar(token, id)
      .then((res) => {
        const starred = !!res.starred;
        if (msg && msg.id === id) setMsg(Object.assign({}, msg, { starred }));
        setItems((prev) => prev.map((it) => (it.id === id ? Object.assign({}, it, { starred }) : it)));
      })
      .catch((e) => console.error(e))
      .finally(() => setBusy(false));
  };

  const onReply = (text, from, format) => {
    if (!msg) return Promise.resolve();
    setBusy(true);
    return apiFetch(token, "/api/reply", {
      method: "POST",
      body: JSON.stringify({ id: msg.id, text, from: from || "", format: format || "" }),
    })
      .then(refreshAfter)
      .catch((e) => {
        console.error(e);
        alert(String(e.message || e));
      })
      .finally(() => setBusy(false));
  };

  const onCompose = (body) => {
    setBusy(true);
    return composeMail(token, body)
      .then((res) => {
        refreshAfter();
        return res;
      })
      .finally(() => setBusy(false));
  };

  // Clears whatever message/selection belonged to the PREVIOUS account
  // synchronously, in the same click handler that changes the active
  // account — batched into the same render as the token change, so the
  // "fetch message by selected id" effect never fires with a stale id
  // against the new account's token (which the server now correctly
  // rejects with 403, but there's no reason to even ask).
  const clearOpenMessage = () => {
    setSelected(null);
    setMsg(null);
    setMsgTags([]);
    setAttachments([]);
  };

  if (err) return <div className="p-8 text-red-400">{t("config_error", err)}</div>;
  if (!cfg) return <div className="p-8 text-ink-muted">{t("booting")}</div>;
  if (!token || addingAccount) {
    return (
      <LoginForm
        tokenInput={tokenInput}
        setTokenInput={setTokenInput}
        onAuthenticated={(tok, acct) => {
          clearOpenMessage();
          addAccount(tok, acct);
          setAddingAccount(false);
          setAddress("");
          setPassword("");
        }}
        address={address}
        setAddress={setAddress}
        password={password}
        setPassword={setPassword}
        loginMode={loginMode}
        setLoginMode={setLoginMode}
        loginError={loginError}
        setLoginError={setLoginError}
        resetToken={resetToken}
        setResetToken={setResetToken}
        setResetMode={(v) => { if (!v) { setResetToken(""); setLoginMode("password"); } }}
        onCancel={token && accounts.length > 0 ? () => { setAddingAccount(false); setLoginError(""); } : undefined}
      />
    );
  }

  return (
    <AppLayout
      view={view}
      tagView={tagView}
      setView={setView}
      tags={tags}
      unread={unread}
      total={total}
      status={status}
      qInput={qInput}
      setQInput={setQInput}
      onSearch={() => setQ(qInput.trim())}
      addrField={addrField}
      setAddrField={setAddrField}
      addrValue={addrValue}
      setAddrValue={setAddrValue}
      addrFacets={addrFacets}
      items={items}
      selected={selected}
      setSelected={setSelected}
      loading={loading}
      checked={checked}
      setChecked={setChecked}
      selectAllPages={selectAllPages}
      setSelectAllPages={setSelectAllPages}
      onCreateTag={onCreateTag}
      onRenameTag={onRenameTag}
      onDeleteTag={onDeleteTag}
      onBulk={onBulk}
      onMarkAll={onMarkAll}
      msg={msg}
      msgTags={msgTags}
      attachments={attachments}
      thread={thread}
      onSelectMessage={setSelected}
      token={token}
      archived={msgTags.includes("archive") || view === "archive"}
      busy={busy}
      onToggleUnread={onToggleUnread}
      onStar={onStar}
      onArchive={() => runOne("archive")}
      onUnarchive={() => runOne("unarchive")}
      onDelete={() => runOne("delete")}
      onReply={onReply}
      offset={offset}
      setOffset={setOffset}
      pageSize={pageSize}
      account={account}
      accounts={accounts}
      activeKey={activeKey}
      onSwitchAccount={(key) => { clearOpenMessage(); switchAccount(key); }}
      onAddAccount={() => { setAddress(""); setPassword(""); setLoginMode("password"); setAddingAccount(true); }}
      onRemoveAccount={(key) => { if (key === activeKey) clearOpenMessage(); removeAccount(key); }}
      brand={account?.brand || cfg?.brand || "poche"}
      onBack={() => setSelected(null)}
      onLogout={() => { clearOpenMessage(); removeAccount(activeKey); setPassword(""); }}
      composeOpen={composeOpen}
      setComposeOpen={setComposeOpen}
      composeMinimized={composeMinimized}
      setComposeMinimized={setComposeMinimized}
      sendAddresses={sendAddresses}
      sendCatchallDomain={sendCatchallDomain}
      sendSeenAddresses={sendSeenAddresses}
      onCompose={onCompose}
    />
  );
}

ReactDOM.createRoot(document.getElementById("root")).render(
  <LangProvider>
    <App />
  </LangProvider>
);
