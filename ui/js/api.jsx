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

function setWebmailToken(t) {
  try {
    localStorage.setItem("webmail_token", t);
  } catch (_) {}
}

function useConfig() {
  const [cfg, setCfg] = React.useState(null);
  const [err, setErr] = React.useState("");
  const [token, setToken] = React.useState(getWebmailToken);
  React.useEffect(() => {
    fetch("/api/config")
      .then((r) => r.json())
      .then((j) => setCfg(j.data || j))
      .catch((e) => setErr(String(e)));
  }, []);
  return { cfg, err, token, setToken: (t) => { setWebmailToken(t); setToken(t); } };
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

function formatWhen(ms) {
  if (!ms) return "";
  const d = new Date(typeof ms === "number" && ms < 1e12 ? ms * 1000 : ms);
  return d.toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function decodeBodyHtml(s) {
  if (!s || typeof s !== "string") return "";
  return s
    .replace(/\\u003c/gi, "<")
    .replace(/\\u003e/gi, ">")
    .replace(/\\u0026/gi, "&");
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

function viewLabel(view, tagView) {
  if (view === "archive") return "Archive";
  if (view === "tag") return "#" + tagView;
  return "Inbox";
}

function appendViewLinks(params, view, tagView) {
  if (view === "archive") {
    params.append("has_link", LINK_ARCHIVE);
  } else if (view === "tag" && tagView) {
    params.append("has_link", "message_tags.message_id:tag=" + tagView);
  } else {
    params.append("missing_link", LINK_ARCHIVE);
  }
}

function buildListPath(view, tagView, q, limit, offset) {
  const params = new URLSearchParams();
  params.set("limit", String(limit));
  params.set("offset", String(offset));
  params.set("sort", "created_at");
  params.set("order", "desc");
  const needle = (q || "").trim().replace(/,/g, " ");
  if (needle) params.set("where", "search_text~=" + needle);
  appendViewLinks(params, view, tagView);
  return "/api/messages?" + params.toString();
}

function buildUnreadCountPath(view, tagView) {
  const params = new URLSearchParams();
  params.set("where", "unread=true");
  appendViewLinks(params, view, tagView);
  return "/api/messages/count?" + params.toString();
}

function createTag(token, name) {
  return apiFetch(token, "/api/tags", {
    method: "POST",
    body: JSON.stringify({ name }),
  });
}
