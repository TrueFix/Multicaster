import React, { useEffect, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import {
  Activity,
  ArrowDownToLine,
  ArrowUpRight,
  Check,
  CheckCircle2,
  ChevronRight,
  Copy,
  Eye,
  EyeOff,
  Globe2,
  Heart,
  KeyRound,
  Leaf,
  Loader2,
  LockKeyhole,
  MonitorPlay,
  MoreHorizontal,
  Pencil,
  Plus,
  Radio,
  RefreshCw,
  Settings2,
  ShieldCheck,
  Sparkles,
  Trash2,
  Twitch,
  Youtube,
  Zap,
} from "lucide-react";
import DashboardFrame from "./DashboardFrame";
import DestinationDialog from "./DestinationDialog";
import GettingStarted, { StreamingHelp } from "./GettingStarted";
import StorageSettings from "./StorageSettings";
import AuthGate from "./AuthGate";
import SystemInfo from "./SystemInfo";
import { TooltipProvider } from "@/components/ui/tooltip";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Switch } from "@/components/ui/switch";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Toaster, toast } from "sonner";
import "./styles.css";

async function api(path, options = {}) {
  const res = await fetch("/api/v1" + path, {
    ...options,
    credentials: "same-origin",
    headers: {
      "Content-Type": "application/json",
      ...options.headers,
    },
  });
  if (res.status === 401) {
    window.dispatchEvent(new Event("auth-expired"));
    throw new Error("Your session has ended. Please sign in again.");
  }
  if (!res.ok)
    throw new Error(
      (await res.text()).trim() || "Something went wrong. Please try again.",
    );
  return res.status === 204 ? null : res.json();
}
function usePage() {
  const allowed = [
    "overview",
    "platforms",
    "settings",
    "settings/updates",
    "settings/system",
    "updates",
  ];
  const [page, set] = useState(
    allowed.includes(location.hash.slice(1))
      ? location.hash.slice(1)
      : "overview",
  );
  useEffect(() => {
    const handler = () =>
      set(
        allowed.includes(location.hash.slice(1))
          ? location.hash.slice(1)
          : "overview",
      );
    window.addEventListener("hashchange", handler);
    return () => window.removeEventListener("hashchange", handler);
  }, []);
  return [
    page,
    (p) => {
      location.hash = p;
      set(p);
    },
  ];
}
function Mascot() {
  return (
    <svg viewBox="0 0 120 104" className="mascot" aria-hidden="true">
      <path
        d="M29 40Q19 4 39 12L52 32M71 32Q88 0 95 15L90 48"
        fill="var(--secondary)"
        stroke="var(--primary)"
        strokeWidth="3"
      />
      <rect
        x="22"
        y="31"
        width="78"
        height="64"
        rx="29"
        fill="var(--card)"
        stroke="var(--primary)"
        strokeWidth="3"
      />
      <path
        d="M43 59v5m34-5v5m-23 9q7 8 14 0"
        stroke="var(--primary)"
        strokeWidth="3"
        strokeLinecap="round"
        fill="none"
      />
      <ellipse cx="38" cy="72" rx="8" ry="4" fill="var(--secondary)" />
      <ellipse cx="82" cy="72" rx="8" ry="4" fill="var(--secondary)" />
      <path
        d="m8 42 2-6 2 6 6 2-6 2-2 6-2-6-6-2zm100-16 2-6 2 6 6 2-6 2-2 6-2-6-6-2z"
        fill="var(--primary)"
      />
    </svg>
  );
}
function Status({ status = "off", children }) {
  return (
    <span className={"status-chip status-" + status}>
      <span />
      {children ||
        {
          live: "Live",
          off: "Off",
          offline: "Offline",
          connecting: "Connecting",
          error: "Needs attention",
          starting: "Warming up",
        }[status] ||
        status}
    </span>
  );
}
function ToggleRow({
  icon: Icon,
  title,
  description,
  checked,
  onChange,
  disabled = false,
}) {
  return (
    <div className="toggle-row">
      <span className="icon-tile">
        <Icon size={20} />
      </span>
      <div className="flex-1">
        <Label className="text-sm font-medium">{title}</Label>
        <p className="text-muted-foreground text-xs mt-1 leading-relaxed">
          {description}
        </p>
      </div>
      <Switch
        aria-label={title}
        checked={checked}
        disabled={disabled}
        onCheckedChange={onChange}
      />
    </div>
  );
}
function CopyField({ label, value, secret = false }) {
  const [show, setShow] = useState(false);
  return (
    <div className="space-y-2">
      <Label>{label}</Label>
      <div className="copy-field">
        <code>{secret && !show ? "•".repeat(24) : value}</code>
        {secret && (
          <Button
            size="icon"
            variant="ghost"
            aria-label={show ? "Hide key" : "Show key"}
            onClick={() => setShow(!show)}
          >
            {show ? <EyeOff /> : <Eye />}
          </Button>
        )}
        <Button
          size="icon"
          variant="ghost"
          aria-label={"Copy " + label}
          onClick={async () => {
            try {
              await navigator.clipboard.writeText(value);
              toast.success("Copied!");
            } catch {
              toast.error(
                "Copy is unavailable over plain HTTP. Select the revealed value instead.",
              );
            }
          }}
        >
          <Copy />
        </Button>
      </div>
    </div>
  );
}
function Preview({ enabled, live, url, status }) {
  const video = useRef(null);
  const [playing, setPlaying] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    const hide = () => {
      if (document.hidden) setPlaying(false);
    };
    document.addEventListener("visibilitychange", hide);
    return () => document.removeEventListener("visibilitychange", hide);
  }, []);
  useEffect(() => {
    setError("");
    if (!playing || !enabled || !live || !video.current) return;
    const el = video.current;
    let hls,
      cancelled = false;
    import("hls.js")
      .then(({ default: Hls }) => {
        if (cancelled) return;
        if (Hls.isSupported()) {
          hls = new Hls({
            enableWorker: true,
            maxBufferLength: 10,
            maxMaxBufferLength: 20,
            maxBufferSize: 8 * 1024 * 1024,
            backBufferLength: 0,
          });
          hls.loadSource(url);
          hls.attachMedia(el);
          hls.on(Hls.Events.ERROR, (_, data) => {
            if (data.fatal)
              setError(
                "Preview could not load. The stream may still be warming up.",
              );
          });
        } else if (el.canPlayType("application/vnd.apple.mpegurl"))
          el.src = url;
        else setError("This browser does not support HLS.");
      })
      .catch(() =>
        setError("Could not load the player. Refresh to try again."),
      );
    return () => {
      cancelled = true;
      hls?.destroy();
      el.removeAttribute("src");
      el.load();
    };
  }, [playing, enabled, live, url]);
  return (
    <div className="preview-box">
      {playing && enabled && live ? (
        <>
          <video
            ref={video}
            controls
            playsInline
            autoPlay
            muted
            className="w-full rounded-xl"
          />
          {error && <p className="text-xs text-destructive p-3">{error}</p>}
          <Button
            variant="ghost"
            className="m-2"
            size="sm"
            onClick={() => setPlaying(false)}
          >
            Close preview · save resources
          </Button>
        </>
      ) : (
        <div className="preview-placeholder">
          <div className="preview-orbit">
            <MonitorPlay size={28} />
          </div>
          <h3>
            {!enabled
              ? "A quiet little preview"
              : !live
                ? "Waiting for your stream"
                : status === "live"
                  ? "Your stream is ready to watch"
                  : "Getting your preview ready"}
          </h3>
          <p>
            {!enabled
              ? "Turn on web streaming to watch right here."
              : !live
                ? "Go live in OBS and your preview will appear."
                : "HLS playback stays off until you open it to save resources."}
          </p>
          {enabled && live && (
            <Button
              variant="secondary"
              size="sm"
              onClick={() => setPlaying(true)}
            >
              <MonitorPlay size={15} />
              Open preview
            </Button>
          )}
        </div>
      )}
    </div>
  );
}
function PlatformIcon({ name }) {
  const n = name.toLowerCase();
  const Icon = n.includes("youtube")
    ? Youtube
    : n.includes("twitch")
      ? Twitch
      : Radio;
  return (
    <span
      className={
        "platform-icon " +
        (n.includes("youtube")
          ? "youtube"
          : n.includes("twitch")
            ? "twitch"
            : "custom")
      }
    >
      <Icon size={24} />
    </span>
  );
}
function Dashboard({ logout }) {
  const [route, navigateRoute] = usePage();
  const page =
    route.startsWith("settings") || route === "updates" ? "settings" : route;
  const settingsTab =
    route === "updates" ? "updates" : route.split("/")[1] || "general";
  const navigate = (p) =>
    navigateRoute(p === "updates" ? "settings/updates" : p);
  const [snap, setSnap] = useState(null);
  const [dests, setDests] = useState([]);
  const [settings, setSettings] = useState({});
  const [connection, setConnection] = useState(null);
  const [offline, setOffline] = useState(false);
  const [dialog, setDialog] = useState(null);
  const [busy, setBusy] = useState(false);
  const [updates, setUpdates] = useState(null);
  const [checking, setChecking] = useState(false);
  const [repo, setRepo] = useState("");
  const [manifestURL,setManifestURL]=useState('')
  const [theme, setTheme] = useState(
    localStorage.getItem("multicaster.theme") || "light",
  );
  const [deleteID, setDeleteID] = useState(null);
  const refreshEpoch = useRef(0),
    operationBusy = useRef(false),
    requestController = useRef(new AbortController());
  useEffect(() => {
    document.documentElement.classList.toggle("dark", theme === "dark");
    localStorage.setItem("multicaster.theme", theme);
  }, [theme]);
  const refresh = async () => {
    const epoch = ++refreshEpoch.current,
      options = { signal: requestController.current.signal };
    const [s, d, c] = await Promise.all([
      api("/session", options),
      api("/destinations", options),
      api("/settings", options),
    ]);
    if (
      epoch !== refreshEpoch.current ||
      requestController.current.signal.aborted
    )
      return c;
    setSnap(s);
    setDests(d || []);
    setSettings(c);
    setOffline(false);
    return c;
  };
  useEffect(() => {
    let stop = false,
      timer;
    async function poll() {
      try {
        if (!document.hidden && !operationBusy.current) await refresh();
      } catch {
        if (!stop) setOffline(true);
      } finally {
        if (!stop) timer = setTimeout(poll, 5000);
      }
    }
    poll();
    api("/connection")
      .then(setConnection)
      .catch(() => {});
    api("/updates")
      .then(setUpdates)
      .catch(() => {});
    return () => {
      stop = true;
      clearTimeout(timer);
      requestController.current.abort();
    };
  }, []);
  useEffect(() => {
    setRepo(settings.update_repository || "");
  }, [settings.update_repository]);
  useEffect(()=>{setManifestURL(settings.update_manifest_url||'')},[settings.update_manifest_url])
  const act = async (fn) => {
    if (operationBusy.current) return;
    operationBusy.current = true;
    refreshEpoch.current++;
    setBusy(true);
    try {
      await fn();
      await refresh();
    } catch (err) {
      toast.error(err.message);
    } finally {
      operationBusy.current = false;
      setBusy(false);
    }
  };
  const setting = (key, value) =>
    act(() =>
      api("/settings", {
        method: "PATCH",
        body: JSON.stringify({ [key]: value }),
      }),
    );
  const live = snap?.ingest === "live";
  const active =
    snap?.destinations?.filter((d) => d.status === "live").length || 0;
  const runtime = (id) => snap?.destinations?.find((d) => d.id === id) || {};
  const titles = {
    overview: "Your streaming dashboard",
    platforms: "Your streaming platforms",
    settings: "Settings",
    updates: "App updates",
  };
  const platformList = (
    <div className="space-y-3">
      {dests.length ? (
        dests.map((d) => (
          <div className="provider-row" key={d.id}>
            <PlatformIcon name={d.name} />
            <div className="min-w-0 flex-1">
              <p className="font-medium truncate">{d.name}</p>
              <div className="flex gap-2 items-center mt-1">
                <span className="text-xs text-muted-foreground">
                  {d.url.startsWith("rtmps:") ? "RTMPS · encrypted" : "RTMP"}
                </span>
                <Status status={runtime(d.id).status || "off"}>
                  {!d.enabled
                    ? "Paused"
                    : !live
                      ? "Waiting for OBS"
                      : !snap?.restream_enabled
                        ? "Restream is off"
                        : undefined}
                </Status>
              </div>
              {runtime(d.id).last_error && (
                <p className="text-xs text-destructive mt-2">
                  {runtime(d.id).last_error}
                </p>
              )}
            </div>
            <Button
              variant="ghost"
              size="icon"
              aria-label={"Edit " + d.name}
              disabled={busy}
              onClick={() => setDialog(d)}
            >
              <Pencil size={15} />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              aria-label={"Delete " + d.name}
              disabled={busy}
              onClick={() => setDeleteID(d.id)}
            >
              <Trash2 size={15} />
            </Button>
            <Switch
              aria-label={"Enable " + d.name}
              checked={d.enabled}
              disabled={busy || offline}
              onCheckedChange={(enabled) =>
                act(() =>
                  api("/destinations/" + d.id, {
                    method: "PATCH",
                    body: JSON.stringify({ enabled }),
                  }),
                )
              }
            />
          </div>
        ))
      ) : (
        <div className="empty-state">
          <span className="icon-tile">
            <Radio />
          </span>
          <h3>Your next audience is out there</h3>
          <p>
            Add YouTube, Twitch, or a custom RTMP platform.
            <br />
            One stream. A few more possibilities.
          </p>
          <Button variant="secondary" onClick={() => setDialog({ new: true })}>
            <Plus />
            Add your first platform
          </Button>
        </div>
      )}
    </div>
  );
  return (
    <>
      <DashboardFrame
        page={page}
        onNavigate={navigate}
        onLogout={logout}
        offline={offline}
      >
        <div className="flex flex-1 flex-col">
          <div className="dashboard-content">
            <main className="main-content">
              <div className="page-heading">
                <div>
                  <p className="eyebrow">
                    {page === "overview"
                      ? "MAKE A LITTLE NOISE"
                      : page === "platforms"
                        ? "ONE STREAM, EVERYWHERE"
                        : "YOUR LITTLE CONTROL ROOM"}
                  </p>
                  <h1>
                    {titles[page]}
                    <span className="text-primary">.</span>
                  </h1>
                  <p>
                    {page === "overview"
                      ? "Send video from OBS once. Share it with every platform you choose."
                      : page === "platforms"
                        ? "Add the services your audience watches. Turn each one on or off whenever you like."
                        : page === "settings"
                          ? "Connect your video app, adjust performance, and choose how everything looks."
                          : "See your installed version and check for a new release."}
                  </p>
                </div>
                {(page === "overview" || page === "platforms") && (
                  <Button onClick={() => setDialog({ new: true })}>
                    <Plus size={17} />
                    Add platform
                  </Button>
                )}
              </div>
              {offline && (
                <div className="notice error">
                  <Activity size={18} />
                  <div>
                    <strong>We lost touch with your server.</strong>
                    <p>
                      Reconnecting automatically. The values below may be out of
                      date.
                    </p>
                  </div>
                </div>
              )}
              {!snap && !offline ? (
                <div className="empty-state">
                  <Loader2 className="animate-spin" />
                  <p>Making everything cozy…</p>
                </div>
              ) : (
                <>
                  {page === "overview" && (
                    <>
                      <section className="welcome-banner">
                        <div>
                          <span className="tiny-label">
                            <Sparkles size={13} />
                            STREAM SIMPLY
                          </span>
                          <h2>
                            {live
                              ? "You're doing your thing."
                              : "Big dreams. Little footprint."}
                          </h2>
                          <p>
                            {live
                              ? "We’re carrying your stream. You focus on the good stuff."
                              : "Send us your stream once. We’ll take it everywhere."}
                          </p>
                          <div className="flex gap-2 mt-4">
                            <span className="feature-tag">
                              <Leaf size={12} />
                              Video stays as OBS made it
                            </span>
                            <span className="feature-tag">
                              <Heart size={12} />
                              Light on resources
                            </span>
                          </div>
                        </div>
                        <Mascot />
                      </section>
                      {(!live || !dests.length) && (
                        <GettingStarted
                          live={live}
                          hasPlatforms={!!dests.length}
                          onSettings={() => navigate("settings")}
                          onAdd={() => setDialog({ new: true })}
                        />
                      )}
                      <div className="stats-grid">
                        <div className="stat">
                          <span>
                            <Radio size={16} />
                            Video from OBS
                          </span>
                          <strong>
                            {live ? "We’re live" : "Waiting for OBS"}
                          </strong>
                          <small>
                            <Status status={live ? "live" : "offline"} />
                            {live ? "OBS is connected" : "Waiting for OBS"}
                          </small>
                        </div>
                        <div className="stat">
                          <span>
                            <ArrowUpRight size={16} />
                            Platforms live
                          </span>
                          <strong>
                            {active}
                            <em>/ {dests.length}</em>
                          </strong>
                          <small>
                            {active
                              ? "A few more places to say hello"
                              : "Ready when you are"}
                          </small>
                        </div>
                        <div className="stat">
                          <span>
                            <Activity size={16} />
                            Video data arriving
                          </span>
                          <strong>
                            {live
                              ? (snap?.bitrate_kbps / 1000).toFixed(1)
                              : "0.0"}
                            <em>Mbps</em>
                          </strong>
                          <small>One upload from your encoder</small>
                        </div>
                      </div>
                    </>
                  )}
                  {(page === "overview" || page === "platforms") && (
                    <>
                      {snap?.warning_message && (
                        <div className="notice">
                          <Radio size={19} />
                          <div>
                            <strong>
                              Let’s give that stream somewhere to go.
                            </strong>
                            <p>
                              {snap.warning_message}
                              {snap.web_stream_enabled
                                ? " Web playback has a separate status below."
                                : ""}
                            </p>
                          </div>
                        </div>
                      )}
                      <div
                        className={
                          page === "overview" ? "overview-grid" : "space-y-5"
                        }
                      >
                        <Card>
                          <CardHeader className="flex flex-row items-start justify-between">
                            <div>
                              <CardTitle>
                                Your platforms{" "}
                                <span className="count-badge">
                                  {dests.length}
                                </span>
                              </CardTitle>
                              <CardDescription>
                                Toggle a platform without missing a beat.
                              </CardDescription>
                            </div>
                            <Radio
                              size={19}
                              className="text-muted-foreground"
                            />
                          </CardHeader>
                          <CardContent>{platformList}</CardContent>
                        </Card>
                        <Card>
                          <CardHeader>
                            <CardTitle>Where your video goes</CardTitle>
                            <CardDescription>
                              You decide where your stream goes.
                            </CardDescription>
                          </CardHeader>
                          <CardContent>
                            <ToggleRow
                              icon={Radio}
                              title="Restream to platforms"
                              description="The main switch for YouTube, Twitch, and your other platforms. Turning this off stops all platform outputs, but OBS can stay connected."
                              checked={!!snap?.restream_enabled}
                              disabled={busy || offline}
                              onChange={(v) => setting("restream_enabled", v)}
                            />
                            <ToggleRow
                              icon={Globe2}
                              title="Web stream"
                              description="Create a browser-friendly stream so you can watch video here or share its HLS link. Not needed for YouTube or Twitch."
                              checked={!!snap?.web_stream_enabled}
                              disabled={busy || offline}
                              onChange={(v) => setting("web_stream_enabled", v)}
                            />
                            <ToggleRow
                              icon={Leaf}
                              title="Performance mode"
                              description="Do less web-stream packaging work by using longer video chunks. Browser playback will be a little further behind live."
                              checked={!!snap?.performance_mode}
                              disabled={busy || offline}
                              onChange={(v) => setting("performance_mode", v)}
                            />
                            <div className="soft-tip">
                              <Zap size={15} />
                              <p>
                                OBS prepares your video once. Multicaster
                                forwards it to your platforms without encoding
                                it again, keeping the extra processing work low.
                              </p>
                            </div>
                          </CardContent>
                        </Card>
                      </div>
                    </>
                  )}
                  {page === "overview" && snap?.preview_enabled && (
                    <Card className="mt-5">
                      <CardHeader className="flex flex-row justify-between">
                        <div>
                          <CardTitle>Your stream, a little closer</CardTitle>
                          <CardDescription>
                            Preview loads only when you open it.
                          </CardDescription>
                        </div>
                        <Status status={snap?.hls_status || "off"} />
                      </CardHeader>
                      <CardContent>
                        <Preview
                          enabled={snap?.web_stream_enabled}
                          live={live}
                          url={snap?.hls_playlist_url}
                          status={snap?.hls_status}
                        />
                        {snap?.hls_error && (
                          <p className="text-destructive text-sm mt-3">
                            {snap.hls_error}
                          </p>
                        )}
                        {snap?.web_stream_enabled && (
                          <a
                            className="text-primary text-xs inline-flex items-center gap-1 mt-3"
                            href={snap.hls_playlist_url}
                            target="_blank"
                            rel="noreferrer"
                          >
                            Open HLS playlist
                            <ArrowUpRight size={13} />
                          </a>
                        )}
                      </CardContent>
                    </Card>
                  )}
                  {page === "settings" && (
                    <div
                      className="settings-navigation"
                      role="tablist"
                      aria-label="Settings sections"
                    >
                      {[
                        ["general", "General"],
                        ["updates", "Updates"],
                        ["system", "System info"],
                      ].map(([id, title]) => (
                        <button
                          key={id}
                          role="tab"
                          aria-selected={settingsTab === id}
                          onClick={() =>
                            navigate(
                              id === "general" ? "settings" : "settings/" + id,
                            )
                          }
                        >
                          {title}
                        </button>
                      ))}
                    </div>
                  )}
                  {page === "settings" && settingsTab === "general" && (
                    <div className="settings-grid">
                      <Card>
                        <CardHeader>
                          <CardTitle>
                            <KeyRound size={17} className="inline mr-2" />
                            Connect OBS or your streaming app
                          </CardTitle>
                          <CardDescription>
                            OBS captures your game or camera and sends the video
                            here. In OBS, open Settings → Stream and choose
                            Custom as the service.
                          </CardDescription>
                        </CardHeader>
                        <CardContent className="space-y-6">
                          {connection ? (
                            <>
                              <CopyField
                                label="OBS server URL"
                                value={`rtmp://${location.hostname.includes(":") ? "[" + location.hostname + "]" : location.hostname}${connection.rtmp_listen === ":1935" ? "" : ":" + connection.rtmp_listen.split(":").at(-1)}/${connection.app}`}
                              />
                              <CopyField
                                label="Stream key"
                                value={connection.stream_key}
                                secret
                              />
                              <p className="field-help">
                                This is your <strong>Multicaster key</strong>.
                                Paste it into OBS. YouTube, Twitch, and other
                                platforms have different keys that belong in
                                “Add platform”.
                              </p>
                              <div className="soft-tip">
                                <ShieldCheck size={17} />
                                <p>
                                  In OBS → Settings → Output, choose an H.264
                                  video encoder and AAC audio. If you see
                                  “Keyframe Interval”, set it to 2 seconds.
                                  These are widely supported video settings;
                                  Multicaster forwards what OBS makes without
                                  encoding it again.
                                </p>
                              </div>
                            </>
                          ) : (
                            <p className="text-muted-foreground">
                              Connection details are unavailable. Reload to
                              retry.
                            </p>
                          )}
                        </CardContent>
                      </Card>
                      <Card>
                        <CardHeader>
                          <CardTitle>Make it yours</CardTitle>
                          <CardDescription>
                            Thoughtful defaults. A little room to play.
                          </CardDescription>
                        </CardHeader>
                        <CardContent>
                          <ToggleRow
                            icon={MonitorPlay}
                            title="Dashboard preview"
                            description="Show the optional video player on Overview. Off by default. It loads only when you open the preview, and requires Web stream to be enabled."
                            checked={!!snap?.preview_enabled}
                            disabled={busy}
                            onChange={(v) => setting("preview_enabled", v)}
                          />
                          <ToggleRow
                            icon={Leaf}
                            title="Performance mode"
                            description="Use 4-second web-video chunks instead of 2-second chunks. This reduces packaging work but adds playback delay. YouTube and Twitch connections stay unchanged."
                            checked={!!snap?.performance_mode}
                            disabled={busy}
                            onChange={(v) => setting("performance_mode", v)}
                          />
                          <ToggleRow
                            icon={MonitorPlay}
                            title="Dark little corner"
                            description="A darker dashboard, for late-night streams."
                            checked={theme === "dark"}
                            onChange={(v) => setTheme(v ? "dark" : "light")}
                          />
                          <div className="soft-tip">
                            <Heart size={16} />
                            <p>
                              When web streaming is off, no browser-video
                              packaging runs. Preview playback starts only when
                              you open it. The dashboard stops checking for
                              updates to stream status while this tab is hidden.
                            </p>
                          </div>
                        </CardContent>
                      </Card>
                      <details className="help-panel advanced-settings col-span-full">
                        <summary>
                          Optional: cloud storage & CDN publishing
                        </summary>
                        <p className="field-help">
                          You don’t need this to stream to YouTube or Twitch, or
                          to preview video here. Open it only if you want to
                          publish your web stream through your own S3/R2 bucket
                          and a public viewing address.
                        </p>
                        <StorageSettings request={api} runtime={snap} />
                      </details>
                    </div>
                  )}
                  {page === "settings" && settingsTab === "system" && (
                    <SystemInfo request={api} />
                  )}
                  {page === "settings" && settingsTab === "updates" && (
                    <div className="settings-grid">
                      <Card>
                        <CardHeader>
                          <CardTitle>Fresh little improvements</CardTitle>
                          <CardDescription>
                            Check releases on your terms. Nothing interrupts a
                            live stream.
                          </CardDescription>
                        </CardHeader>
                        <CardContent className="space-y-6">
                          <div className="version-box">
                            <span className="icon-tile">
                              <Sparkles />
                            </span>
                            <div>
                              <p className="font-medium">
                                Multicaster {updates?.version || "dev"}
                              </p>
                              <p className="text-xs text-muted-foreground mt-1">
                                {updates?.platform ||
                                  "Loading build information"}
                              </p>
                            </div>
                            <span className="ml-auto text-xs text-muted-foreground">
                              Installed version
                            </span>
                          </div>
                          <div className="space-y-2">
                            <Label htmlFor="manifest-url">Update manifest URL</Label>
                            <Input id="manifest-url" type="url" value={manifestURL} disabled={busy||checking} onChange={e=>setManifestURL(e.target.value)} placeholder="https://github.com/TrueFix/Multicaster/releases/latest/download/update-manifest.json"/>
                            <p className="field-help">The official Multicaster feed is configured by default. Clear this field and leave the repository empty to disable update checks. Nothing is checked or restarted automatically.</p>
                          </div>
                          <div className="space-y-2">
                            <Label htmlFor="repo">
                              GitHub release repository (optional)
                            </Label>
                            {updates?.version === "dev" && (
                              <p className="field-help">
                                “dev” means this copy was built from source,
                                rather than installed as a numbered release.
                              </p>
                            )}
                            <Input
                              id="repo"
                              placeholder="your-organization/multicaster"
                              value={repo}
                              disabled={busy||checking}
                              onChange={(e) => setRepo(e.target.value)}
                            />
                            <p className="text-xs text-muted-foreground">
                              Optional override for another GitHub release repository. Leave it empty to use the manifest URL above. This is not your YouTube or Twitch channel.
                            </p>
                          </div>
                          <div className="flex gap-2 flex-wrap">
                            <Button
                              variant="outline"
                              disabled={busy}
                              onClick={()=>act(async()=>{await api('/settings',{method:'PATCH',body:JSON.stringify({update_repository:repo,update_manifest_url:manifestURL})});setUpdates(await api('/updates'))})}
                            >
                              <Check size={16} />
                              Save update source
                            </Button>
                            <Button
                              disabled={checking || busy || (!repo&&!manifestURL)}
                              onClick={async () => {
                                setChecking(true);
                                try {
                                  await api("/settings", {
                                    method: "PATCH",
                                    body: JSON.stringify({
                                      update_repository: repo,
                                      update_manifest_url:manifestURL,
                                    }),
                                  });
                                  setUpdates(await api("/updates?check=true"));
                                  await refresh();
                                } catch (err) {
                                  toast.error(err.message);
                                } finally {
                                  setChecking(false);
                                }
                              }}
                            >
                              <RefreshCw
                                size={16}
                                className={checking ? "animate-spin" : ""}
                              />
                              {checking
                                ? "Looking for magic…"
                                : "Check for updates"}
                            </Button>
                          </div>
                          <div className="soft-tip">
                            <ArrowDownToLine size={18} />
                            <div>
                              <p>
                                {updates?.message ||
                                  "The official release feed is ready."}
                              </p>
                              {updates?.checked_at && (
                                <p className="mt-1">
                                  Last checked{" "}
                                  {new Date(
                                    updates.checked_at,
                                  ).toLocaleString()}
                                </p>
                              )}
                              {updates?.url && (
                                <a
                                  className="inline-flex items-center gap-1 text-primary mt-3"
                                  target="_blank"
                                  rel="noreferrer"
                                  href={updates.url}
                                >
                                  Open releases <ArrowUpRight size={13} />
                                </a>
                              )}
                              {updates?.download_url&&<div className="space-y-3 mt-4"><Button asChild><a href={updates.download_url}>Download for {updates.platform}<ArrowDownToLine size={16}/></a></Button><details><summary className="font-semibold">Release SHA-256 checksum</summary><code className="block break-all text-xs mt-2">{updates.sha256}</code></details></div>}
                            </div>
                          </div>
                        </CardContent>
                      </Card>
                      <Card>
                        <CardHeader>
                          <CardTitle>A tiny update ritual</CardTitle>
                          <CardDescription>
                            Keep your setup and your settings.
                          </CardDescription>
                        </CardHeader>
                        <CardContent className="space-y-5 text-sm text-muted-foreground">
                          <p>1. Finish your stream before restarting.</p>
                          <p>
                            2. Download the binary for Windows, macOS or Linux from
                            your release page. For Docker, pull or rebuild your
                            image.
                          </p>
                          <p>
                            3. Stop Multicaster, replace the binary, and start
                            it again.
                          </p>
                          <div className="soft-tip">
                            <LockKeyhole size={17} />
                            <p>
                              Your config stays in your OS configuration directory, or Docker volume. See Settings → System info for its exact location. Keep that directory and your platforms stay with you.
                            </p>
                          </div>
                        </CardContent>
                      </Card>
                    </div>
                  )}
                </>
              )}
              <StreamingHelp />
              <footer className="page-footer">
                <span>
                  <Sparkles size={12} />A little stream magic, hosted by you.
                </span>
                <button onClick={() => navigate("updates")}>
                  Multicaster {updates?.version || "dev"}
                  <ArrowUpRight size={12} />
                </button>
              </footer>
            </main>
          </div>
        </div>
      </DashboardFrame>
      {dialog && (
        <DestinationDialog
          key={dialog.id || "new"}
          destination={dialog.new ? null : dialog}
          onClose={() => setDialog(null)}
          onSaved={refresh}
          request={api}
        />
      )}
      <Dialog
        open={!!deleteID}
        onOpenChange={(open) => !open && setDeleteID(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Remove this platform?</DialogTitle>
            <DialogDescription>
              This stops its live connection immediately. Your other platforms
              keep streaming.
            </DialogDescription>
          </DialogHeader>
          <div className="flex justify-end gap-2">
            <Button variant="outline" onClick={() => setDeleteID(null)}>
              Keep it
            </Button>
            <Button
              variant="destructive"
              onClick={() =>
                act(async () => {
                  await api("/destinations/" + deleteID, { method: "DELETE" });
                  setDeleteID(null);
                  toast.success("Platform removed");
                })
              }
            >
              Remove platform
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}
function App() {
  return (
    <TooltipProvider>
      <AuthGate>{(logout) => <Dashboard logout={logout} />}</AuthGate>
      <Toaster richColors position="bottom-right" />
    </TooltipProvider>
  );
}
createRoot(document.getElementById("root")).render(<App />);
