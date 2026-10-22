import { useState } from "react";
import {
  ArrowUpRight,
  Check,
  Eye,
  EyeOff,
  HelpCircle,
  Loader2,
  Plus,
  Radio,
  ShieldCheck,
  Twitch,
  Youtube,
} from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { buildPublishURL, splitPublishURL } from "./platform-url";

const providers = {
  youtube: {
    title: "YouTube",
    icon: Youtube,
    server: "rtmps://a.rtmps.youtube.com/live2",
    help: "Open YouTube Studio → Create → Go live → Stream settings. Copy “Stream URL” and “Stream key” from there.",
    link: "https://studio.youtube.com",
    linkTitle: "Open YouTube Studio",
  },
  twitch: {
    title: "Twitch",
    icon: Twitch,
    server: "rtmp://live.twitch.tv/app",
    help: "Open Twitch Creator Dashboard → Settings → Stream. Copy your Primary Stream key. We’ve filled in Twitch’s server address for you.",
    link: "https://dashboard.twitch.tv/settings/stream",
    linkTitle: "Open Twitch stream settings",
  },
  custom: {
    title: "Custom RTMP",
    icon: Radio,
    server: "",
    help: "Open your streaming service’s live or encoder settings. Look for “Server URL” (sometimes called “RTMP URL”) and “Stream key”. Ask that service for these values if you can’t find them.",
  },
};
function providerFor(raw = "") {
  try {
    const host = new URL(raw).hostname;
    return host.endsWith(".youtube.com")
      ? "youtube"
      : host.endsWith(".twitch.tv")
        ? "twitch"
        : "custom";
  } catch {
    return "custom";
  }
}
export default function DestinationDialog({
  destination,
  onClose,
  onSaved,
  request,
}) {
  const initial = splitPublishURL(destination?.url);
  const [provider, setProvider] = useState(providerFor(destination?.url));
  const [name, setName] = useState(destination?.name || "");
  const [server, setServer] = useState(initial.serverURL);
  const [key, setKey] = useState(initial.streamKey);
  const [enabled, setEnabled] = useState(destination?.enabled ?? true);
  const [showKey, setShowKey] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [fullURL, setFullURL] = useState("");
  const info = providers[provider];
  const select = (id) => {
    setProvider(id);
    setName(providers[id].title);
    setServer(providers[id].server);
    setKey("");
    setError("");
  };
  return (
    <Dialog open onOpenChange={(open) => !open && !busy && onClose()}>
      <DialogContent className="platform-dialog">
        <DialogHeader>
          <span className="dialog-eyebrow">
            <Radio size={17} />
            YOUR NEXT AUDIENCE
          </span>
          <DialogTitle>
            {destination ? "Edit your platform" : "Add a streaming platform"}
          </DialogTitle>
          <DialogDescription>
            Tell us where to send your video. These details come from YouTube,
            Twitch, or the service you want to stream to.
          </DialogDescription>
        </DialogHeader>
        <form
          className="platform-form"
          onSubmit={async (e) => {
            e.preventDefault();
            setError("");
            let url;
            try {
              url = buildPublishURL(server, key);
            } catch (err) {
              setError(err.message);
              return;
            }
            setBusy(true);
            try {
              await request(
                destination
                  ? "/destinations/" + destination.id
                  : "/destinations",
                {
                  method: destination ? "PATCH" : "POST",
                  body: JSON.stringify({
                    name: name.trim(),
                    url,
                    enabled,
                    type: "rtmp_push",
                  }),
                },
              );
              onClose();
              toast.success(
                destination
                  ? "Platform updated"
                  : "Platform saved. You can go live whenever you’re ready.",
              );
              try {
                await onSaved();
              } catch {
                toast.error(
                  "Platform saved, but status could not refresh. Reload the dashboard to retry.",
                );
              }
            } catch (err) {
              setError(err.message);
            } finally {
              setBusy(false);
            }
          }}
        >
          <fieldset disabled={busy}>
            <legend>1. Choose your platform</legend>
            <div className="provider-choices">
              {Object.entries(providers).map(([id, p]) => (
                <button
                  type="button"
                  aria-pressed={id === provider}
                  key={id}
                  onClick={() => select(id)}
                >
                  <p.icon size={22} />
                  <span>{p.title}</span>
                  {id === provider && <Check size={17} />}
                </button>
              ))}
            </div>
          </fieldset>
          <div className="provider-instructions">
            <HelpCircle size={21} />
            <div>
              <strong>Where do I find these details?</strong>
              <p>{info.help}</p>
              {info.link && (
                <a href={info.link} target="_blank" rel="noreferrer">
                  {info.linkTitle}
                  <ArrowUpRight size={16} />
                </a>
              )}
            </div>
          </div>
          <div className="space-y-2">
            <Label htmlFor="platform-name">2. Give it a name</Label>
            <Input
              id="platform-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="For example, My YouTube channel"
              required
              maxLength={80}
              disabled={busy}
              aria-describedby="platform-name-help"
            />
            <p id="platform-name-help" className="field-help">
              This is just a label for your dashboard. It won’t change your
              channel name.
            </p>
          </div>
          <div className="space-y-2">
            <Label htmlFor="platform-server">3. RTMP server URL</Label>
            <Input
              id="platform-server"
              value={server}
              onChange={(e) => setServer(e.target.value)}
              placeholder="rtmps://a.rtmps.youtube.com/live2"
              required
              disabled={busy}
              autoComplete="off"
              spellCheck={false}
              aria-describedby="platform-server-help"
            />
            <p id="platform-server-help" className="field-help">
              The platform’s video address, starting with <code>rtmp://</code>{" "}
              or <code>rtmps://</code>. Don’t put your stream key here.
            </p>
          </div>
          <div className="space-y-2">
            <Label htmlFor="platform-key">4. Stream key</Label>
            <div className="key-input">
              <Input
                id="platform-key"
                type={showKey ? "text" : "password"}
                value={key}
                onChange={(e) => setKey(e.target.value)}
                placeholder="Paste the key from your platform"
                required
                disabled={busy}
                autoComplete="off"
                spellCheck={false}
                aria-describedby="platform-key-help"
              />
              <Button
                variant="ghost"
                type="button"
                size="icon"
                aria-label={showKey ? "Hide platform key" : "Show platform key"}
                onClick={() => setShowKey(!showKey)}
              >
                {showKey ? <EyeOff /> : <Eye />}
              </Button>
            </div>
            <p id="platform-key-help" className="field-help">
              Use{" "}
              <strong>
                {provider === "custom" ? "this platform’s" : info.title + "’s"}{" "}
                key
              </strong>
              , not your Multicaster key from OBS. Each platform has its own
              private key.
            </p>
          </div>
          <details className="full-url-import">
            <summary>Already have a full publish URL?</summary>
            <p className="field-help">
              If your service gives you a single address containing the key,
              paste it here. We’ll separate it into the fields above.
            </p>
            <Label htmlFor="platform-full-url">Full publish URL</Label>
            <Input
              id="platform-full-url"
              type="password"
              value={fullURL}
              onChange={(e) => setFullURL(e.target.value)}
              placeholder="rtmp://server/live/your-key"
              autoComplete="off"
            />
            <Button
              variant="outline"
              type="button"
              onClick={() => {
                const parsed = splitPublishURL(fullURL.trim());
                if (!parsed.streamKey) {
                  setError(
                    "That URL does not include a stream key. It should look like rtmp://server/live/your-key.",
                  );
                  return;
                }
                setServer(parsed.serverURL);
                setKey(parsed.streamKey);
                setProvider(providerFor(fullURL));
                setError("");
                toast.success("Server URL and key filled in above.");
              }}
            >
              Fill in URL and key
            </Button>
          </details>
          <div className="platform-enable">
            <div>
              <Label htmlFor="platform-enabled">Enable this platform</Label>
              <p className="field-help">
                If OBS is already streaming and “Restream to platforms” is on,
                we’ll start sending video here. You can turn it off anytime.
              </p>
            </div>
            <Switch
              id="platform-enabled"
              checked={enabled}
              disabled={busy}
              onCheckedChange={setEnabled}
            />
          </div>
          {error && (
            <p role="alert" className="form-error">
              {error}
            </p>
          )}
          <p className="key-privacy">
            <ShieldCheck size={18} />
            Your key is saved on your Multicaster server and hidden in the
            dashboard.
          </p>
          <div className="dialog-actions">
            <Button
              type="button"
              variant="outline"
              disabled={busy}
              onClick={onClose}
            >
              Cancel
            </Button>
            <Button disabled={busy}>
              {busy ? <Loader2 className="animate-spin" /> : <Plus />}
              {destination ? "Save changes" : "Add platform"}
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}
