// UI translations. Keys are grouped by screen; values are plain strings or
// functions when a value has to be interpolated, so pluralisation and word
// order stay inside the translation rather than being assembled in the view.
//
// English is the fallback: a missing French key renders the English string
// instead of a bare key, so a partial translation degrades readably.

const I18N = {
  en: {
    lang_name: "English",
    booting: "Booting…",
    loading: "Loading…",
    config_error: (e) => "Config error: " + e,

    // login
    login_title: "Webmail login",
    login_hint: "Sign in with your email address and password.",
    login_address: "email address",
    login_password: "password",
    login_remember: "Remember me",
    login_submit: "Sign in",
    login_forgot: "Forgot password?",
    login_admin: "Admin? Use a token →",
    login_back: "← Back to password login",
    token_title: "Admin token",
    token_hint: "Paste the admin or webmail token.",
    token_placeholder: "token",
    token_submit: "Use token",
    forgot_title: "Forgot password",
    forgot_hint: "We will email a reset link to the mailbox recovery address.",
    forgot_submit: "Send reset link",
    forgot_sent: "If the address exists, a reset email was sent.",
    reset_title: "Set new password",
    reset_token: "reset token",
    reset_new: "new password",
    reset_submit: "Set new password",
    reset_done: "Password updated. You can sign in now.",

    // navigation
    compose: "Compose",
    inbox: "Inbox",
    sent: "Sent",
    archive: "Archive",
    archive_action: "Archive",
    tags: "Tags",
    new_tag: "new tag",
    rename_tag: (n) => "Rename #" + n,
    delete_tag: (n) => "Delete #" + n,
    rename_prompt: (n) => "Rename #" + n + " to:",
    delete_confirm: (n) =>
      "Delete #" + n + "? Messages keep their content, they just lose this tag.",

    // sidebar footer
    in_view: (n) => n + " in view",
    poche_ok: "poche ok",
    poche_down: "poche down",
    storage: "Storage",
    msgs: (n) => n + " msgs",
    kept_months: (p) => "kept " + p,
    kept_starred: "★ kept forever",
    months: (n) => (n === 1 ? "1 month" : n + " months"),
    retention_tooltip: (p, cap) =>
      "Messages older than " + p +
      " are removed automatically to stay within the mailbox quota. " +
      "Starred messages are never removed." + cap,
    capped_at: (n) => " Also capped at " + n + " messages.",
    copy_address: "Copy address to clipboard",
    copied: "copied ✓",
    copy_blocked: "copy blocked — select manually",
    theme_light: "Light theme",
    theme_dark: "Dark theme",
    switch_to_light: "Switch to light theme",
    switch_to_dark: "Switch to dark theme",
    sign_out: "Sign out",
    language: "Language",

    // list + toolbar
    search_placeholder: "Filter subject / from / body…",
    search: "Search",
    select_page: "Select page",
    select_all: (n) => "Select all " + n + " messages (all pages)",
    no_messages: "No messages in this view.",
    prev: "Prev",
    next: "Next",
    mark_read_n: (n) => "Mark read (" + n + ")",
    archive_n: (n) => "Archive (" + n + ")",
    unarchive_n: (n) => "Unarchive (" + n + ")",
    star_n: (n) => "Star (" + n + ")",
    unstar_n: (n) => "Unstar (" + n + ")",
    delete_n: (n) => "Delete (" + n + ")",
    mark_all_read: "Mark all read",
    tag_n: (n) => "Tag (" + n + ")…",
    to_prefix: (a) => "To " + a,

    // message pane
    select_message: "Select a message",
    from: "From",
    to: "To",
    mark_read: "Mark read",
    mark_unread: "Mark unread",
    star: "Star",
    unstar: "Unstar",
    unarchive: "Unarchive",
    delete: "Delete",
    attachment: "attachment",
    reply_placeholder: "Reply…",
    send_reply: "Send reply",

    // compose
    new_message: "New message",
    close: "Close",
    cc: "Cc",
    bcc: "Bcc",
    add_cc: "Add Cc / Bcc",
    subject: "Subject",
    to_placeholder: "someone@example.com, other@example.com",
    body_placeholder: "Write your message…",
    md_placeholder: "# Hello\n\nWrite **Markdown** — it is converted to HTML when sent.",
    html_placeholder: "<p>Write HTML — it is sanitized before sending.</p>",
    fmt_plain: "Plain",
    fmt_markdown: "Markdown",
    fmt_html: "HTML",
    preview: "Preview",
    edit: "Edit",
    send: "Send",
    sending: "Sending…",
    sent_to: (a) => "Sent to " + a,
    note_plain: "Plain text",
    note_markdown: "Markdown → HTML on send",
    note_html: "HTML is sanitized before sending",
    attach: "Attach files",
    attachments: "Attachments",
    remove: "Remove",
    attach_total: (n, size) => n + " file(s) · " + size,
    attach_too_big: (name, cap) => name + " is over the " + cap + " limit",
    not_stored: "unavailable",
    not_stored_title: "This attachment has no stored content. Inbound attachments are not retrieved yet - it needs a Resend key with read access.",
    note_attachments: "",
    err_recipient: "Add at least one recipient.",
    err_subject: "Subject is required.",
    err_body: "Message body is required.",
    preview_failed: (e) => "Preview failed: " + e,
    empty: "(empty)",
    wysiwyg_bold: "Bold",
    wysiwyg_italic: "Italic",
    wysiwyg_underline: "Underline",
    wysiwyg_h1: "Heading",
    wysiwyg_quote: "Quote",
    wysiwyg_ul: "Bulleted list",
    wysiwyg_ol: "Numbered list",
    wysiwyg_link: "Link",
    wysiwyg_link_prompt: "Link URL:",
    wysiwyg_clear: "Clear formatting",
    html_placeholder_wysiwyg: "Write your message…",
    switch_format_confirm: "Switching format clears the current draft. Continue?",
    minimize: "Minimize",
    expand: "Expand",
  },

  fr: {
    lang_name: "Français",
    booting: "Démarrage…",
    loading: "Chargement…",
    config_error: (e) => "Erreur de configuration : " + e,

    login_title: "Connexion webmail",
    login_hint: "Connectez-vous avec votre adresse e-mail et votre mot de passe.",
    login_address: "adresse e-mail",
    login_password: "mot de passe",
    login_remember: "Se souvenir de moi",
    login_submit: "Se connecter",
    login_forgot: "Mot de passe oublié ?",
    login_admin: "Admin ? Utiliser un jeton →",
    login_back: "← Retour à la connexion par mot de passe",
    token_title: "Jeton administrateur",
    token_hint: "Collez le jeton admin ou webmail.",
    token_placeholder: "jeton",
    token_submit: "Utiliser le jeton",
    forgot_title: "Mot de passe oublié",
    forgot_hint: "Nous enverrons un lien de réinitialisation à l'adresse de secours.",
    forgot_submit: "Envoyer le lien",
    forgot_sent: "Si l'adresse existe, un e-mail de réinitialisation a été envoyé.",
    reset_title: "Nouveau mot de passe",
    reset_token: "jeton de réinitialisation",
    reset_new: "nouveau mot de passe",
    reset_submit: "Définir le mot de passe",
    reset_done: "Mot de passe mis à jour. Vous pouvez vous connecter.",

    compose: "Nouveau message",
    inbox: "Boîte de réception",
    sent: "Envoyés",
    archive: "Archives",
    archive_action: "Archiver",
    tags: "Étiquettes",
    new_tag: "nouvelle étiquette",
    rename_tag: (n) => "Renommer #" + n,
    delete_tag: (n) => "Supprimer #" + n,
    rename_prompt: (n) => "Renommer #" + n + " en :",
    delete_confirm: (n) =>
      "Supprimer #" + n + " ? Les messages sont conservés, ils perdent seulement cette étiquette.",

    in_view: (n) => n + " affichés",
    poche_ok: "poche ok",
    poche_down: "poche hors service",
    storage: "Stockage",
    msgs: (n) => n + " msgs",
    kept_months: (p) => "conservés " + p,
    kept_starred: "★ conservés indéfiniment",
    months: (n) => (n === 1 ? "1 mois" : n + " mois"),
    retention_tooltip: (p, cap) =>
      "Les messages de plus de " + p +
      " sont supprimés automatiquement pour respecter le quota de la boîte. " +
      "Les messages suivis (★) ne sont jamais supprimés." + cap,
    capped_at: (n) => " Limite également fixée à " + n + " messages.",
    copy_address: "Copier l'adresse",
    copied: "copié ✓",
    copy_blocked: "copie bloquée — sélectionnez manuellement",
    theme_light: "Thème clair",
    theme_dark: "Thème sombre",
    switch_to_light: "Passer au thème clair",
    switch_to_dark: "Passer au thème sombre",
    sign_out: "Se déconnecter",
    language: "Langue",

    search_placeholder: "Filtrer objet / expéditeur / contenu…",
    search: "Rechercher",
    select_page: "Sélectionner la page",
    select_all: (n) => "Sélectionner les " + n + " messages (toutes les pages)",
    no_messages: "Aucun message dans cette vue.",
    prev: "Préc.",
    next: "Suiv.",
    mark_read_n: (n) => "Marquer lu (" + n + ")",
    archive_n: (n) => "Archiver (" + n + ")",
    unarchive_n: (n) => "Désarchiver (" + n + ")",
    star_n: (n) => "Suivre (" + n + ")",
    unstar_n: (n) => "Ne plus suivre (" + n + ")",
    delete_n: (n) => "Supprimer (" + n + ")",
    mark_all_read: "Tout marquer comme lu",
    tag_n: (n) => "Étiqueter (" + n + ")…",
    to_prefix: (a) => "À " + a,

    select_message: "Sélectionnez un message",
    from: "De",
    to: "À",
    mark_read: "Marquer lu",
    mark_unread: "Marquer non lu",
    star: "Suivre",
    unstar: "Ne plus suivre",
    unarchive: "Désarchiver",
    delete: "Supprimer",
    attachment: "pièce jointe",
    reply_placeholder: "Répondre…",
    send_reply: "Envoyer la réponse",

    new_message: "Nouveau message",
    close: "Fermer",
    cc: "Cc",
    bcc: "Cci",
    add_cc: "Ajouter Cc / Cci",
    subject: "Objet",
    to_placeholder: "quelquun@exemple.com, autre@exemple.com",
    body_placeholder: "Écrivez votre message…",
    md_placeholder: "# Bonjour\n\nÉcrivez en **Markdown** — converti en HTML à l'envoi.",
    html_placeholder: "<p>Écrivez du HTML — il est nettoyé avant l'envoi.</p>",
    fmt_plain: "Texte",
    fmt_markdown: "Markdown",
    fmt_html: "HTML",
    preview: "Aperçu",
    edit: "Éditer",
    send: "Envoyer",
    sending: "Envoi…",
    sent_to: (a) => "Envoyé à " + a,
    note_plain: "Texte brut",
    note_markdown: "Markdown → HTML à l'envoi",
    note_html: "Le HTML est nettoyé avant l'envoi",
    attach: "Joindre des fichiers",
    attachments: "Pièces jointes",
    remove: "Retirer",
    attach_total: (n, size) => n + " fichier(s) · " + size,
    attach_too_big: (name, cap) => name + " dépasse la limite de " + cap,
    not_stored: "indisponible",
    not_stored_title: "Cette pièce jointe n'a pas de contenu stocké. Les pièces jointes reçues ne sont pas encore récupérées - une clé Resend avec accès en lecture est nécessaire.",
    note_attachments: "",
    err_recipient: "Ajoutez au moins un destinataire.",
    err_subject: "L'objet est obligatoire.",
    err_body: "Le message est vide.",
    preview_failed: (e) => "Échec de l'aperçu : " + e,
    empty: "(vide)",
    wysiwyg_bold: "Gras",
    wysiwyg_italic: "Italique",
    wysiwyg_underline: "Souligné",
    wysiwyg_h1: "Titre",
    wysiwyg_quote: "Citation",
    wysiwyg_ul: "Liste à puces",
    wysiwyg_ol: "Liste numérotée",
    wysiwyg_link: "Lien",
    wysiwyg_link_prompt: "URL du lien :",
    wysiwyg_clear: "Effacer la mise en forme",
    html_placeholder_wysiwyg: "Écrivez votre message…",
    switch_format_confirm: "Changer de format efface le brouillon actuel. Continuer ?",
    minimize: "Réduire",
    expand: "Agrandir",
  },
};

const LANGS = ["en", "fr"];

function detectLang() {
  try {
    const stored = localStorage.getItem("webmail_lang");
    if (LANGS.includes(stored)) return stored;
  } catch (_) {}
  // First visit: follow the browser rather than defaulting to English, since
  // the people reading these mailboxes are not necessarily English speakers.
  const nav = (navigator.languages && navigator.languages[0]) || navigator.language || "en";
  return String(nav).toLowerCase().startsWith("fr") ? "fr" : "en";
}

function setLang(lang) {
  const l = LANGS.includes(lang) ? lang : "en";
  try {
    localStorage.setItem("webmail_lang", l);
  } catch (_) {}
  document.documentElement.setAttribute("lang", l);
  return l;
}

// translate(lang) returns t(key, ...args); English fills any gap in French.
function translate(lang) {
  const dict = I18N[lang] || I18N.en;
  return function t(key, ...args) {
    const v = dict[key] !== undefined ? dict[key] : I18N.en[key];
    if (v === undefined) return key;
    return typeof v === "function" ? v(...args) : v;
  };
}

const LangContext = React.createContext({ lang: "en", t: translate("en"), setLang: () => {} });

function useI18n() {
  return React.useContext(LangContext);
}

function LangProvider({ children }) {
  const [lang, setLangState] = React.useState(detectLang);
  React.useEffect(() => {
    setLang(lang);
  }, [lang]);
  const value = React.useMemo(
    () => ({ lang, t: translate(lang), setLang: setLangState }),
    [lang]
  );
  return <LangContext.Provider value={value}>{children}</LangContext.Provider>;
}

function LangToggle() {
  const { lang, setLang: choose, t } = useI18n();
  return (
    <div className="flex gap-1" title={t("language")}>
      {LANGS.map((l) => (
        <button
          key={l}
          onClick={() => choose(l)}
          className={
            "flex-1 text-[0.7rem] px-2 py-1 rounded border " +
            (lang === l
              ? "border-accent text-accent bg-accent-soft"
              : "border-paper-line text-ink-dim hover:text-ink")
          }
        >
          {l.toUpperCase()}
        </button>
      ))}
    </div>
  );
}
