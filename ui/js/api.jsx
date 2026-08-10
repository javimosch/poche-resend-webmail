const LINK_ARCHIVE = "message_tags.message_id:tag=archive";

// ─── multi-account session store ───────────────────────────────────────
// Each logged-in mailbox keeps its own session token, all stored together,
// so switching accounts (like Proton's account switcher) never needs to
// re-authenticate as long as that session hasn't expired — and logging
// into a second mailbox no longer overwrites the first one's credentials.
// `key` is the mailbox address when known, or a synthetic "token:<hash>"
// for a raw admin-token login where no address was returned.

function accountKey(account, token) {
  if (account && account.address) return account.address;
  return "token:" + token.slice(0, 12);
}

function loadAccounts() {
  try {
    const raw = localStorage.getItem("webmail_accounts");
    const list = raw ? JSON.parse(raw) : [];
    return Array.isArray(list) ? list : [];
  } catch (_) {
    return [];
  }
}

function saveAccounts(list) {
  try {
    if (list.length) {
      localStorage.setItem("webmail_accounts", JSON.stringify(list));
    } else {
      localStorage.removeItem("webmail_accounts");
    }
  } catch (_) {}
}

function getActiveAccountKey() {
  try {
    return localStorage.getItem("webmail_active_key") || "";
  } catch (_) {
    return "";
  }
}

function setActiveAccountKey(key) {
  try {
    if (key) localStorage.setItem("webmail_active_key", key);
    else localStorage.removeItem("webmail_active_key");
  } catch (_) {}
}

// One-time migration from the old single-slot webmail_token/webmail_account
// keys (and a ?token= URL param) into the new multi-account list, so
// existing logged-in users aren't signed out by this change.
function migrateLegacyAccount() {
  const q = new URLSearchParams(location.search).get("token");
  let legacyToken = q;
  let legacyAccount = null;
  try {
    if (!legacyToken) legacyToken = localStorage.getItem("webmail_token") || "";
    const raw = localStorage.getItem("webmail_account");
    legacyAccount = raw ? JSON.parse(raw) : null;
  } catch (_) {}
  if (!legacyToken) return;
  const key = accountKey(legacyAccount, legacyToken);
  const list = loadAccounts().filter((a) => a.key !== key);
  list.push({
    key,
    token: legacyToken,
    address: legacyAccount?.address || "",
    name: legacyAccount?.name || "",
    brand: legacyAccount?.brand || "",
  });
  saveAccounts(list);
  setActiveAccountKey(key);
  try {
    localStorage.removeItem("webmail_token");
    localStorage.removeItem("webmail_account");
  } catch (_) {}
}

function upsertAccount(token, account) {
  const key = accountKey(account, token);
  const list = loadAccounts().filter((a) => a.key !== key);
  list.push({
    key,
    token,
    address: account?.address || "",
    name: account?.name || "",
    brand: account?.brand || "",
  });
  saveAccounts(list);
  setActiveAccountKey(key);
  return { list, key };
}

function removeAccount(key) {
  const list = loadAccounts().filter((a) => a.key !== key);
  saveAccounts(list);
  return list;
}

function getRememberedCreds() {
  try {
    const raw = localStorage.getItem("webmail_remember");
    if (!raw) return null;
    const parsed = JSON.parse(raw);
    if (parsed && typeof parsed.address === "string" && typeof parsed.password === "string") {
      return parsed;
    }
    return null;
  } catch (_) {
    return null;
  }
}

function setRememberedCreds(creds) {
  try {
    if (creds && creds.address) {
      localStorage.setItem("webmail_remember", JSON.stringify(creds));
    } else {
      localStorage.removeItem("webmail_remember");
    }
  } catch (_) {}
}

function clearRememberedCreds() {
  try {
    localStorage.removeItem("webmail_remember");
  } catch (_) {}
}

// ─── theme ──────────────────────────────────────────────────────────────
// Dark stays the default; the choice is per-browser and applied to <html> so
// the CSS variables in app.css swap the whole palette.

function getTheme() {
  try {
    return localStorage.getItem("webmail_theme") === "light" ? "light" : "dark";
  } catch (_) {
    return "dark";
  }
}

function applyTheme(theme) {
  const t = theme === "light" ? "light" : "dark";
  document.documentElement.setAttribute("data-theme", t);
  try {
    localStorage.setItem("webmail_theme", t);
  } catch (_) {}
  return t;
}

function useTheme() {
  const [theme, setTheme] = React.useState(getTheme);
  React.useEffect(() => {
    applyTheme(theme);
  }, [theme]);
  return [theme, () => setTheme((t) => (t === "light" ? "dark" : "light"))];
}

function useConfig() {
  const [cfg, setCfg] = React.useState(null);
  const [err, setErr] = React.useState("");
  const [accounts, setAccounts] = React.useState(() => {
    migrateLegacyAccount();
    return loadAccounts();
  });
  // Reads AFTER the accounts-state initializer above has already run
  // migrateLegacyAccount() — a fresh ?token= login or a pre-existing
  // single-slot session is migrated into the list before this executes.
  const [activeKey, setActiveKey] = React.useState(getActiveAccountKey);

  React.useEffect(() => {
    fetch("/api/config")
      .then((r) => r.json())
      .then((j) => setCfg(j.data || j))
      .catch((e) => setErr(String(e)));
  }, []);

  const active = accounts.find((a) => a.key === activeKey) || null;

  return {
    cfg,
    err,
    token: active?.token || "",
    account: active ? { address: active.address, name: active.name, brand: active.brand } : null,
    accounts,
    activeKey,
    // Logs in a NEW account (or refreshes an existing one's token) and
    // switches to it, without disturbing any other already-open session.
    addAccount: (token, account) => {
      const { list, key } = upsertAccount(token, account);
      setAccounts(list);
      setActiveKey(key);
    },
    // Switches the active session instantly from an already-open one —
    // no network round trip, since its token is still cached locally.
    switchAccount: (key) => {
      setActiveAccountKey(key);
      setActiveKey(key);
    },
    // Signs out of ONE account. If it was the active one, falls back to
    // whatever's left (or the login screen if this was the last account) —
    // signing into a second mailbox should never cost you the first one.
    removeAccount: (key) => {
      const list = removeAccount(key);
      setAccounts(list);
      if (key === activeKey) {
        const next = list[0]?.key || "";
        setActiveAccountKey(next);
        setActiveKey(next);
      }
    },
  };
}

function apiFetch(token, path, opts = {}) {
  const headers = Object.assign(
    { "Content-Type": "application/json" },
    opts.headers || {}
  );
  if (token) headers.Authorization = "Bearer " + token;
  return fetch(path, Object.assign({}, opts, { headers })).then(async (r) => {
    const j = await r.json().catch(() => ({}));
    if (!j.ok) throw new Error(j.error?.message || j.error || "api error " + r.status);
    return j.data;
  });
}

function bulkFetch(token, body) {
  return apiFetch(token, "/api/bulk", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

function toggleStar(token, id) {
  return apiFetch(token, "/api/messages/" + encodeURIComponent(id) + "/star", {
    method: "PUT",
  });
}

function fmtBytes(b) {
  if (!b && b !== 0) return "";
  if (b >= 1073741824) return (b / 1073741824).toFixed(1) + " GB";
  if (b >= 1048576) return (b / 1048576).toFixed(1) + " MB";
  if (b >= 1024) return (b / 1024).toFixed(0) + " KB";
  return b + " B";
}

function formatWhen(ms, locale) {
  if (!ms) return "";
  const d = new Date(typeof ms === "number" && ms < 1e12 ? ms * 1000 : ms);
  return d.toLocaleString(locale || undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

// Body HTML is sanitized server-side before storage. Do NOT un-escape
// sequences here: JSON.parse already decodes genuine \u escapes, so any
// remaining "<" is literal text the sender wrote — turning it back into
// a tag would rebuild markup that sanitizing had just neutralized.
function decodeBodyHtml(s) {
  return typeof s === "string" ? s : "";
}

function rowFromItem(it) {
  const doc = typeof it.doc === "string" ? JSON.parse(it.doc) : it.doc || {};
  return Object.assign({ id: it.id }, doc);
}

function tagNamesFromPage(data) {
  return (data?.items || [])
    .map((it) => {
      const doc = typeof it.doc === "string" ? JSON.parse(it.doc) : it.doc || {};
      return doc.name || doc.tag;
    })
    .filter(Boolean);
}

function viewLabel(view, tagView, t) {
  const tr = t || ((k) => k);
  if (view === "archive") return tr("archive");
  if (view === "sent") return tr("sent");
  if (view === "tag") return "#" + tagView;
  return tr("inbox");
}

function appendViewLinks(params, view, tagView) {
  if (view === "archive") {
    params.append("has_link", LINK_ARCHIVE);
  } else if (view === "tag" && tagView) {
    params.append("has_link", "message_tags.message_id:tag=" + tagView);
  } else if (view !== "sent") {
    params.append("missing_link", LINK_ARCHIVE);
  }
}

// Inbox shows received mail only; Sent shows what this mailbox sent out.
function viewWhere(view) {
  if (view === "sent") return "direction=out";
  if (view === "inbox") return "direction=in";
  return "";
}

function buildListPath(view, tagView, q, limit, offset, addrFilter) {
  const params = new URLSearchParams();
  params.set("limit", String(limit));
  params.set("offset", String(offset));
  params.set("sort", "created_at");
  params.set("order", "desc");
  const needle = (q || "").trim().replace(/,/g, " ");
  const where = [viewWhere(view), needle ? "search_text~=" + needle : ""];
  if (addrFilter && addrFilter.field && addrFilter.value) {
    where.push(addrFilter.field + "=" + addrFilter.value.replace(/,/g, ""));
  }
  const whereClauses = where.filter(Boolean);
  if (whereClauses.length) params.set("where", whereClauses.join(","));
  appendViewLinks(params, view, tagView);
  return "/api/messages?" + params.toString();
}

function buildFacetPath(field) {
  return "/api/messages/facets?field=" + encodeURIComponent(field);
}

function fetchAddressFacets(token, field) {
  return apiFetch(token, buildFacetPath(field))
    .then((d) => d.values || [])
    .catch(() => []);
}

function buildUnreadCountPath(view, tagView) {
  const params = new URLSearchParams();
  params.set("where", ["unread=true", viewWhere(view)].filter(Boolean).join(","));
  appendViewLinks(params, view, tagView);
  return "/api/messages/count?" + params.toString();
}

function composeMail(token, body) {
  return apiFetch(token, "/api/compose", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

function fetchSendAddresses(token) {
  return apiFetch(token, "/api/mailbox/addresses")
    .then((d) => ({
      addresses: d.addresses || [],
      catchallDomain: d.catchall_domain || "",
      seenAddresses: d.seen_addresses || [],
    }))
    .catch(() => ({ addresses: [], catchallDomain: "", seenAddresses: [] }));
}

function renderBodyPreview(token, text, format) {
  return apiFetch(token, "/api/render", {
    method: "POST",
    body: JSON.stringify({ text, format }),
  });
}

function renameTag(token, name, newName) {
  return apiFetch(token, "/api/tags", {
    method: "PUT",
    body: JSON.stringify({ name, new_name: newName }),
  });
}

function deleteTag(token, name) {
  return apiFetch(token, "/api/tags?name=" + encodeURIComponent(name), { method: "DELETE" });
}

// The async clipboard API needs a secure context AND permission, and it
// rejects rather than throwing when denied — so the textarea fallback has to
// run on rejection too, not only when the API is missing.
function legacyCopy(text) {
  try {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.setAttribute("readonly", "");
    ta.style.position = "fixed";
    ta.style.top = "-1000px";
    document.body.appendChild(ta);
    ta.select();
    const ok = document.execCommand("copy");
    document.body.removeChild(ta);
    return ok;
  } catch (_) {
    return false;
  }
}

function copyToClipboard(text) {
  if (navigator.clipboard && window.isSecureContext) {
    return navigator.clipboard.writeText(text).catch(() =>
      legacyCopy(text) ? Promise.resolve() : Promise.reject(new Error("copy blocked"))
    );
  }
  return legacyCopy(text) ? Promise.resolve() : Promise.reject(new Error("copy blocked"));
}

function createTag(token, name) {
  return apiFetch(token, "/api/tags", {
    method: "POST",
    body: JSON.stringify({ name }),
  });
}
