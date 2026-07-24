function LoginForm({ tokenInput, setTokenInput, setToken, setAccount, address, setAddress, password, setPassword, loginMode, setLoginMode, loginError, setLoginError, resetToken, setResetToken, setResetMode }) {
  const [forgotSent, setForgotSent] = React.useState(false);
  const [remember, setRemember] = React.useState(() => !!getRememberedCreds());

  React.useEffect(() => {
    const saved = getRememberedCreds();
    if (saved) {
      setAddress(saved.address);
      setPassword(saved.password);
    }
  }, [setAddress, setPassword]);

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
        if (data.address) {
          setAccount({ address: data.address, name: data.name || "" });
        }
        if (remember) {
          setRememberedCreds({ address: address.trim(), password: password.trim() });
        } else {
          clearRememberedCreds();
        }
      })
      .catch((e) => setLoginError(String(e.message || e)));
  };

  const onToken = (e) => {
    e.preventDefault();
    if (tokenInput.trim()) setToken(tokenInput.trim());
  };

  const onForgot = (e) => {
    e.preventDefault();
    if (!address.trim()) return;
    setLoginError("");
    setForgotSent(false);
    fetch("/api/forgot-password", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ address: address.trim() }),
    })
      .then(async (r) => {
        const j = await r.json().catch(() => ({}));
        if (!j.ok) throw new Error(j.error?.message || j.error || "request failed");
        setForgotSent(true);
      })
      .catch((e) => setLoginError(String(e.message || e)));
  };

  const onResetPassword = (e) => {
    e.preventDefault();
    if (!resetToken.trim() || !password.trim()) return;
    setLoginError("");
    fetch("/api/reset-password", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ token: resetToken.trim(), new_password: password.trim() }),
    })
      .then(async (r) => {
        const j = await r.json().catch(() => ({}));
        if (!j.ok) throw new Error(j.error?.message || j.error || "reset failed");
        setResetMode(false);
        setResetToken("");
        setLoginMode("password");
        setLoginError("");
        alert("Password reset. Please log in with your new password.");
      })
      .catch((e) => setLoginError(String(e.message || e)));
  };

  // ─── reset password screen ──────────────────────────────────────────
  if (loginMode === "reset") {
    return (
      <div className="max-w-lg mx-auto mt-24 p-8 border border-paper-line rounded-lg bg-paper-raised">
        <h1 className="font-display text-2xl mb-2">Set new password</h1>
        <p className="text-ink-muted text-sm mb-4">
          Enter the reset token and your new password.
        </p>
        {loginError && (
          <div className="mb-4 text-sm text-red-400 border border-red-400/30 rounded px-3 py-2 bg-red-400/5">
            {loginError}
          </div>
        )}
        <form className="space-y-3" onSubmit={onResetPassword}>
          <input
            value={resetToken}
            onChange={(e) => setResetToken(e.target.value)}
            className="w-full bg-paper border border-paper-line rounded px-2 py-1.5 text-sm font-mono"
            placeholder="reset token"
            autoFocus
          />
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            className="w-full bg-paper border border-paper-line rounded px-2 py-1.5 text-sm"
            placeholder="new password"
          />
          <button type="submit" className="w-full text-sm px-3 py-1.5 border border-paper-line rounded hover:bg-paper-line">
            Reset password
          </button>
        </form>
        <button
          className="mt-4 text-xs text-ink-muted hover:text-accent"
          onClick={() => { setLoginMode("password"); setLoginError(""); }}
        >
          ← Back to login
        </button>
      </div>
    );
  }

  // ─── admin token screen ─────────────────────────────────────────────
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

  // ─── default: password login ────────────────────────────────────────
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
      {forgotSent && (
        <div className="mb-4 text-sm text-accent border border-accent/30 rounded px-3 py-2 bg-accent/5">
          If the address exists, a reset link was sent to the recovery email.
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
        <label className="flex items-center gap-2 text-xs text-ink-dim cursor-pointer">
          <input
            type="checkbox"
            checked={remember}
            onChange={(e) => setRemember(e.target.checked)}
            className="accent-accent"
          />
          Remember me
        </label>
        <button type="submit" className="w-full text-sm px-3 py-1.5 border border-paper-line rounded hover:bg-paper-line">
          Sign in
        </button>
      </form>
      <div className="mt-4 flex justify-between text-xs">
        <button
          className="text-ink-muted hover:text-accent"
          onClick={onForgot}
        >
          Forgot password?
        </button>
        <button
          className="text-ink-muted hover:text-accent"
          onClick={() => setLoginMode("token")}
        >
          Admin? Use a token →
        </button>
      </div>
    </div>
  );
}
