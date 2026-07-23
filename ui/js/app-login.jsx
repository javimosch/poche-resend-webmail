function LoginForm({ tokenInput, setTokenInput, setToken, address, setAddress, password, setPassword, loginMode, setLoginMode, loginError, setLoginError }) {
  const onLogin = (e) => {
    e.preventDefault();
    if (!address.trim() || !password.trim()) return;
    setLoginError("");
    fetch("/api/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ address: address.trim(), password: password.trim() }),
    })
      .then(async (r) => {
        const j = await r.json().catch(() => ({}));
        if (!j.ok) throw new Error(j.error?.message || j.error || "login failed");
        return j.data;
      })
      .then((data) => {
        setToken(data.token);
      })
      .catch((e) => setLoginError(String(e.message || e)));
  };

  const onToken = (e) => {
    e.preventDefault();
    if (tokenInput.trim()) setToken(tokenInput.trim());
  };

  if (loginMode === "token") {
    return (
      <div className="max-w-lg mx-auto mt-24 p-8 border border-paper-line rounded-lg bg-paper-raised">
        <h1 className="font-display text-2xl mb-2">Admin token</h1>
        <p className="text-ink-muted text-sm mb-4">
          Paste the Bearer token (WEBMAIL_TOKEN or ADMIN_TOKEN).
        </p>
        <form className="flex gap-2" onSubmit={onToken}>
          <input
            value={tokenInput}
            onChange={(e) => setTokenInput(e.target.value)}
            className="flex-1 bg-paper border border-paper-line rounded px-2 py-1.5 text-sm"
            placeholder="token"
          />
          <button type="submit" className="text-xs px-3 py-1.5 border border-paper-line rounded">
            Save
          </button>
        </form>
        <button
          className="mt-4 text-xs text-ink-muted hover:text-accent"
          onClick={() => setLoginMode("password")}
        >
          ← Back to login
        </button>
      </div>
    );
  }

  return (
    <div className="max-w-lg mx-auto mt-24 p-8 border border-paper-line rounded-lg bg-paper-raised">
      <h1 className="font-display text-2xl mb-2">Webmail login</h1>
      <p className="text-ink-muted text-sm mb-4">
        Sign in with your email address and password.
      </p>
      {loginError && (
        <div className="mb-4 text-sm text-red-400 border border-red-400/30 rounded px-3 py-2 bg-red-400/5">
          {loginError}
        </div>
      )}
      <form className="space-y-3" onSubmit={onLogin}>
        <input
          value={address}
          onChange={(e) => setAddress(e.target.value)}
          className="w-full bg-paper border border-paper-line rounded px-2 py-1.5 text-sm"
          placeholder="email address"
          autoFocus
        />
        <input
          type="password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          className="w-full bg-paper border border-paper-line rounded px-2 py-1.5 text-sm"
          placeholder="password"
        />
        <button type="submit" className="w-full text-sm px-3 py-1.5 border border-paper-line rounded hover:bg-paper-line">
          Sign in
        </button>
      </form>
      <button
        className="mt-4 text-xs text-ink-muted hover:text-accent"
        onClick={() => setLoginMode("token")}
      >
        Admin? Use a token →
      </button>
    </div>
  );
}
