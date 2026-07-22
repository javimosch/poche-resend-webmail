function LoginForm({ tokenInput, setTokenInput, setToken }) {
  return (
    <div className="max-w-lg mx-auto mt-24 p-8 border border-paper-line rounded-lg bg-paper-raised">
      <h1 className="font-display text-2xl mb-2">WEBMAIL_TOKEN</h1>
      <p className="text-ink-muted text-sm mb-4">
        Open with <code className="text-accent">?token=…</code> or paste the Bearer token.
      </p>
      <form
        className="flex gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          if (tokenInput.trim()) setToken(tokenInput.trim());
        }}
      >
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
    </div>
  );
}
