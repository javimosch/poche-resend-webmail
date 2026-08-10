const LINK_ARCHIVE = "message_tags.message_id:tag=archive";

function getWebmailToken() {
  const q = new URLSearchParams(location.search).get("token");
  if (q) {
    try {
      localStorage.setItem("webmail_token", q);
    } catch (_) {}
    return q;
  }
  try {
    return localStorage.getItem("webmail_token") || "";
  } catch (_) {
    return "";
  }
}

function clearWebmailToken() {
  try {
    localStorage.removeItem("webmail_token");
  } catch (_) {}
}

function setWebmailToken(t) {
  try {
    localStorage.setItem("webmail_token", t);
  } catch (_) {}
}

function getWebmailAccount() {
  try {
    const raw = localStorage.getItem("webmail_account");
    return raw ? JSON.parse(raw) : null;
  } catch (_) {
    return null;
  }
}

function setWebmailAccount(account) {
  try {
    if (account) {
      localStorage.setItem("webmail_account", JSON.stringify(account));
    } else {
      localStorage.removeItem("webmail_account");
    }
  } catch (_) {}
}

function clearWebmailAccount() {
  try {
    localStorage.removeItem("webmail_account");
  } catch (_) {}
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
  const [token, setToken] = React.useState(getWebmailToken);
  const [account, setAccount] = React.useState(getWebmailAccount);
  React.useEffect(() => {
    fetch("/api/config")
      .then((r) => r.json())
      .then((j) => setCfg(j.data || j))
      .catch((e) => setErr(String(e)));
  }, []);
  return {
    cfg,
    err,
    token,
    account,
    setToken: (t) => { setWebmailToken(t); setToken(t); },
    setAccount: (a) => { setWebmailAccount(a); setAccount(a); },
    clearAuth: () => { clearWebmailToken(); clearWebmailAccount(); setToken(""); setAccount(null); },
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
    .then((d) => d.addresses || [])
    .catch(() => []);
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
