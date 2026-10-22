import { useEffect, useState } from "react";
import { CloudUpload, ExternalLink, Loader2, Save } from "lucide-react";
import { toast } from "sonner";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";

const fieldDescriptions = {
  endpoint:
    "The upload address provided by AWS, Cloudflare R2, or your storage service. It is different from the public viewing address.",
  public_url:
    "The public address viewers use to download video from your bucket or CDN. A CDN is a service that delivers those files to viewers.",
  bucket:
    "The name of the storage container you created with your provider. The bucket must already exist.",
  region:
    "Your storage location. Use auto for Cloudflare R2, or your AWS region, such as us-east-1.",
  prefix:
    "An optional folder name inside your bucket, such as live. It keeps stream files separate from your other files.",
  access_key:
    "The access key ID from your storage provider. This is not a YouTube, Twitch, or Multicaster stream key.",
  secret_key:
    "The matching private storage password. Leave this blank to keep a secret you already saved.",
};

export default function StorageSettings({ request, runtime }) {
  const [config, setConfig] = useState(null);
  const [hasSecret, setHasSecret] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    request("/storage")
      .then((data) => {
        setConfig({
          ...data.config,
          region: data.config.region || "auto",
          prefix: data.config.prefix || "live",
          secret_key: "",
        });
        setHasSecret(data.has_secret);
      })
      .catch((err) => setError(err.message));
  }, []);
  const field = (key, value) => setConfig((c) => ({ ...c, [key]: value }));
  return (
    <Card className="col-span-full">
      <CardHeader>
        <CardTitle>
          <CloudUpload size={18} className="inline mr-2" />A little further · S3
          & R2
        </CardTitle>
        <CardDescription>
          Optional HLS publishing to your own bucket and CDN. Uploads run only
          while web streaming and live input are on.
        </CardDescription>
      </CardHeader>
      <CardContent>
        {error ? (
          <p className="text-destructive text-sm">{error}</p>
        ) : !config ? (
          <Loader2 className="animate-spin" />
        ) : (
          <form
            className="space-y-5"
            onSubmit={async (e) => {
              e.preventDefault();
              setBusy(true);
              try {
                await request("/storage", {
                  method: "PUT",
                  body: JSON.stringify(config),
                });
                setHasSecret(hasSecret || !!config.secret_key);
                field("secret_key", "");
                toast.success("Object storage settings saved");
              } catch (err) {
                toast.error(err.message);
              } finally {
                setBusy(false);
              }
            }}
          >
            <div className="flex gap-3 items-center">
              <span className="icon-tile">
                <CloudUpload size={20} />
              </span>
              <div className="flex-1">
                <Label>Publish HLS to object storage</Label>
                <p className="text-xs text-muted-foreground mt-1">
                  Disabling stops uploads immediately after saving. Local
                  playback stays available.
                </p>
              </div>
              <Switch
                aria-label="Publish HLS to object storage"
                checked={config.enabled}
                onCheckedChange={(v) => field("enabled", v)}
              />
            </div>
            <div className="grid sm:grid-cols-2 gap-4">
              {[
                [
                  "endpoint",
                  "S3 API endpoint",
                  "https://ACCOUNT.r2.cloudflarestorage.com",
                ],
                [
                  "public_url",
                  "Public bucket / CDN base URL",
                  "https://live.example.com",
                ],
                ["bucket", "Bucket", "my-streams"],
                ["region", "Region", "auto for R2; us-east-1 for AWS"],
                ["prefix", "Object prefix", "live"],
                [
                  "access_key",
                  "Access key ID",
                  "Your S3-compatible access key",
                ],
                [
                  "secret_key",
                  "Secret access key",
                  hasSecret
                    ? "Saved · leave blank to keep"
                    : "Your S3-compatible secret key",
                ],
              ].map(([key, title, placeholder]) => (
                <div className="space-y-2" key={key}>
                  <Label htmlFor={"storage-" + key}>{title}</Label>
                  <Input
                    id={"storage-" + key}
                    type={key.includes("key") ? "password" : "text"}
                    autoComplete="off"
                    value={config[key] || ""}
                    onChange={(e) => field(key, e.target.value)}
                    placeholder={placeholder}
                    aria-describedby={"storage-help-" + key}
                    required={
                      config.enabled &&
                      key !== "prefix" &&
                      (key !== "secret_key" || !hasSecret)
                    }
                  />
                  <p id={"storage-help-" + key} className="field-help">
                    {fieldDescriptions[key]}
                  </p>
                </div>
              ))}
            </div>
            <div className="soft-tip">
              <CloudUpload size={17} />
              <div>
                <p>
                  Video files upload before the list that tells players how to
                  watch them. An upload failure won’t stop your platform
                  streams. Each streaming session gets a new viewing URL.
                </p>
                <details className="mt-3">
                  <summary className="font-semibold">
                    What needs setting up at my storage provider?
                  </summary>
                  <p className="mt-2">
                    Make the bucket readable through your public viewing
                    address. If your player is on a different website, allow
                    that website in the bucket’s CORS settings (the rules for
                    other websites accessing its files). Set a lifecycle rule to
                    remove old stream files automatically; Multicaster does not
                    delete uploaded files.
                  </p>
                </details>
                {runtime?.cdn_status === "error" && (
                  <p className="text-destructive mt-2">{runtime.cdn_error}</p>
                )}
                {runtime?.cdn_status === "live" && runtime.cdn_url && (
                  <a
                    className="text-primary inline-flex gap-1 items-center mt-2"
                    href={runtime.cdn_url}
                    target="_blank"
                    rel="noreferrer"
                  >
                    Open CDN playlist <ExternalLink size={12} />
                  </a>
                )}
                {config.enabled && runtime?.cdn_status !== "live" && (
                  <p className="mt-2">
                    {runtime?.cdn_status === "off"
                      ? "Waiting for live input and web streaming."
                      : "Publishing status: " +
                        (runtime?.cdn_status || "waiting")}
                  </p>
                )}
              </div>
            </div>
            <Button disabled={busy}>
              {busy ? <Loader2 className="animate-spin" /> : <Save size={16} />}
              Save storage settings
            </Button>
          </form>
        )}
      </CardContent>
    </Card>
  );
}
