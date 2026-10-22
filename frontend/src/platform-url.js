// The existing relay stores one publish URL. Keep server/key fields separate in
// the form, then compose that same format so older configurations still work.
export function splitPublishURL(raw = "") {
  try {
    const u = new URL(raw);
    const parts = u.pathname.split("/").filter(Boolean);
    if (parts.length < 2) return { serverURL: raw, streamKey: "" };
    const app = parts.shift();
    return {
      serverURL: `${u.protocol}//${u.host}/${app}`,
      streamKey: parts.join("/") + u.search,
    };
  } catch {
    return { serverURL: raw, streamKey: "" };
  }
}
export function buildPublishURL(server, key) {
  const address = server.trim(),
    streamKey = key.trim();
  let u;
  try {
    u = new URL(address);
  } catch {
    throw new Error(
      "Enter a complete server URL, starting with rtmp:// or rtmps://.",
    );
  }
  if (!["rtmp:", "rtmps:"].includes(u.protocol) || !u.hostname)
    throw new Error(
      "The server URL must start with rtmp:// or rtmps://. Copy it from your platform’s streaming settings.",
    );
  if (u.username || u.password || u.search || u.hash)
    throw new Error(
      "Use the server address without a key or query string here. Paste a complete publish URL under “Already have a full publish URL?” instead.",
    );
  if (!u.pathname.split("/").filter(Boolean).length)
    throw new Error(
      "Include the application path in the server URL, such as /live or /app.",
    );
  if (!streamKey)
    throw new Error(
      "Paste the stream key from your platform’s streaming settings.",
    );
  if (/\s/.test(streamKey))
    throw new Error(
      "The stream key contains a space or line break. Copy the key again without extra text.",
    );
  return address.replace(/\/+$/, "") + "/" + streamKey.replace(/^\/+/, "");
}
