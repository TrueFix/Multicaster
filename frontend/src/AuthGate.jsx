import { useEffect, useState } from "react";
import { KeyRound, Loader2, LockKeyhole, Sparkles } from "lucide-react";
import { toast } from "sonner";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

async function authRequest(path, body) {
  const res = await fetch("/api/v1/auth/" + path, {
    method: body ? "POST" : "GET",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: body ? JSON.stringify(body) : undefined,
    cache: "no-store",
  });
  if (!res.ok)
    throw new Error((await res.text()).trim() || "Could not reach your server");
  return res.status === 204 ? null : res.json();
}
export default function AuthGate({ children }) {
  const [status, setStatus] = useState(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [code, setCode] = useState(() => {
    if (location.hash.startsWith("#setup=")) {
      const code =
        new URLSearchParams(location.hash.slice(1)).get("setup") || "";
      history.replaceState(null, "", location.pathname + location.search);
      return code;
    }
    return "";
  });
  const reload = async () => {
    try {
      setStatus(await authRequest("status"));
      setError("");
    } catch (err) {
      setError(err.message);
    }
  };
  useEffect(() => {
    sessionStorage.removeItem("multicaster.token");
    reload();
    const setupLink = () => {
      if (location.hash.startsWith("#setup=")) {
        setCode(new URLSearchParams(location.hash.slice(1)).get("setup") || "");
        history.replaceState(null, "", location.pathname + location.search);
        reload();
      }
    };
    window.addEventListener("auth-expired", reload);
    window.addEventListener("hashchange", setupLink);
    return () => {
      window.removeEventListener("auth-expired", reload);
      window.removeEventListener("hashchange", setupLink);
    };
  }, []);
  const logout = async () => {
    try {
      await authRequest("logout", {});
      setPassword("");
      setStatus({ authenticated: false, setup_required: false });
    } catch (err) {
      toast.error("Could not lock the dashboard: " + err.message);
    }
  };
  if (status?.authenticated) return children(logout);
  const setup = status?.setup_required;
  return (
    <div className="login-wrap">
      <Card className="login-card">
        <CardContent className="space-y-6 p-8">
          <div className="text-center">
            <span className="auth-symbol">
              <Sparkles size={30} />
            </span>
            <h1 className="text-2xl font-semibold mt-3">
              {setup
                ? "Create your dashboard password"
                : "Welcome back, streamer"}
            </h1>
            <p className="text-muted-foreground text-sm mt-2">
              {setup
                ? "Keep your streaming controls private. Your platforms and stream keys stay with you."
                : "Sign in to manage your platforms and settings."}
            </p>
          </div>
          {!status ? (
            <div className="space-y-4 text-center">
              {error ? (
                <>
                  <p className="form-error" role="alert">
                    {error}
                  </p>
                  <Button onClick={reload}>Try connecting again</Button>
                </>
              ) : (
                <Loader2 className="animate-spin mx-auto" />
              )}
            </div>
          ) : (
            <form
              className="space-y-5"
              onSubmit={async (e) => {
                e.preventDefault();
                setError("");
                if (setup && password !== confirm) {
                  setError(
                    "The passwords don’t match. Please enter the same password twice.",
                  );
                  return;
                }
                setBusy(true);
                try {
                  await authRequest(
                    setup ? "setup" : "login",
                    setup ? { password, setup_token: code } : { password },
                  );
                  setPassword("");
                  setConfirm("");
                  setCode("");
                  await reload();
                } catch (err) {
                  setError(err.message);
                } finally {
                  setBusy(false);
                }
              }}
            >
              {setup && (
                <div className="space-y-2">
                  <Label htmlFor="setup-code">Setup code</Label>
                  <Input
                    id="setup-code"
                    type="password"
                    value={code}
                    onChange={(e) => setCode(e.target.value)}
                    autoComplete="off"
                    required
                  />
                  <p className="field-help">
                    The link printed by Multicaster fills this in automatically.
                    If it’s empty, open that link or copy its setup code from
                    the terminal. You can generate a new link with{" "}
                    <code>multicaster --reset-password</code>.
                  </p>
                </div>
              )}
              <div className="space-y-2">
                <Label htmlFor="password">
                  {setup ? "New password" : "Password"}
                </Label>
                <Input
                  id="password"
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  autoComplete={setup ? "new-password" : "current-password"}
                  minLength={setup ? 8 : undefined}
                  maxLength={72}
                  required
                />
                {setup && (
                  <p className="field-help">
                    Use at least 8 characters. This password protects your
                    dashboard, not your OBS connection.
                  </p>
                )}
              </div>
              {setup && (
                <div className="space-y-2">
                  <Label htmlFor="confirm-password">Confirm password</Label>
                  <Input
                    id="confirm-password"
                    type="password"
                    value={confirm}
                    onChange={(e) => setConfirm(e.target.value)}
                    autoComplete="new-password"
                    minLength={8}
                    maxLength={72}
                    required
                  />
                </div>
              )}
              {error && (
                <p className="form-error" role="alert">
                  {error}
                </p>
              )}
              <Button className="w-full" disabled={busy}>
                {busy ? <Loader2 className="animate-spin" /> : <LockKeyhole />}
                {setup
                  ? "Create password & open dashboard"
                  : "Open my dashboard"}
              </Button>
              {!setup && (
                <details className="help-panel">
                  <summary>Forgot your password?</summary>
                  <p className="field-help">
                    On the computer running Multicaster, run{" "}
                    <code>multicaster --reset-password</code>. It opens a
                    create-password page. If the server is already running, your
                    live streams keep going.
                  </p>
                </details>
              )}
            </form>
          )}
          <p className="text-xs text-center text-muted-foreground">
            Self-hosted. No cloud account required.
          </p>
        </CardContent>
      </Card>
    </div>
  );
}
