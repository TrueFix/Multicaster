import { useEffect, useState } from "react";
import {
  Cpu,
  Folder,
  Loader2,
  MemoryStick,
  RefreshCw,
  Server,
} from "lucide-react";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
const memory = (n) => ((n || 0) / 1024 / 1024).toFixed(1) + " MiB";
const uptime = (n) => {
  n = Math.floor(n || 0);
  return `${Math.floor(n / 3600)}h ${Math.floor((n % 3600) / 60)}m ${n % 60}s`;
};
export default function SystemInfo({ request }) {
  const [data, setData] = useState(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const load = async () => {
    setBusy(true);
    try {
      setData(await request("/system"));
      setError("");
    } catch (err) {
      setError(err.message);
    } finally {
      setBusy(false);
    }
  };
  useEffect(() => {
    load();
  }, []);
  return (
    <Card>
      <CardHeader>
        <div className="flex justify-between items-start gap-4">
          <div>
            <CardTitle>Your Multicaster server</CardTitle>
            <CardDescription>
              System details are loaded only when you open this section or
              refresh them.
            </CardDescription>
          </div>
          <Button variant="outline" onClick={load} disabled={busy}>
            <RefreshCw className={busy ? "animate-spin" : ""} />
            Refresh
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        {error && (
          <p role="alert" className="form-error">
            {error}
          </p>
        )}
        {!data && busy ? (
          <Loader2 className="animate-spin" />
        ) : (
          data && (
            <>
              <div className="system-grid">
                {[
                  ["Operating system", data.os + " / " + data.arch],
                  ["Version", data.version],
                  ["Uptime", uptime(data.uptime_seconds)],
                  ["Logical CPUs", data.logical_cpus],
                  ["Go CPU parallelism", data.go_max_procs],
                  ["Live Go heap", memory(data.heap_alloc_bytes)],
                  [
                    "Go runtime reserved memory",
                    memory(data.go_reserved_bytes),
                  ],
                  ["Background goroutines", data.goroutines],
                ].map(([label, value]) => (
                  <div className="system-tile" key={label}>
                    <span>{label}</span>
                    <strong>{value}</strong>
                  </div>
                ))}
              </div>
              <div className="soft-tip">
                <MemoryStick size={18} />
                <p>
                  Memory values describe this app’s Go runtime, not your
                  computer’s total RAM or total process memory. “CPU
                  parallelism” is a concurrency limit, not CPU usage. No
                  background system monitor runs.
                </p>
              </div>
              <dl className="system-paths">
                <dt>Configuration file</dt>
                <dd>
                  <code>{data.config_path}</code>
                </dd>
                <dt>Data directory</dt>
                <dd>
                  <code>{data.data_path}</code>
                </dd>
                <dt>Runtime</dt>
                <dd>
                  {data.go_version} · Process {data.pid}
                </dd>
              </dl>
            </>
          )
        )}
      </CardContent>
    </Card>
  );
}
