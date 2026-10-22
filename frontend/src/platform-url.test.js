import { test } from "node:test";
import assert from "node:assert/strict";
import { buildPublishURL, splitPublishURL } from "./platform-url.js";

test("editing existing URLs preserves keys, query parameters and ports", () => {
  for (const raw of [
    "rtmps://a.rtmps.youtube.com/live2/aaaa-bbbb-cccc",
    "rtmp://localhost:1936/live/my-key?auth=abc",
    "rtmp://[::1]:1935/app/one/two",
  ]) {
    const parsed = splitPublishURL(raw);
    assert.equal(buildPublishURL(parsed.serverURL, parsed.streamKey), raw);
  }
});
test("server and stream key fields trim surrounding whitespace and avoid double slashes", () => {
  assert.equal(
    buildPublishURL(" rtmp://server/live/ ", " key "),
    "rtmp://server/live/key",
  );
});
test("common beginner mistakes return actionable messages", () => {
  assert.throws(
    () => buildPublishURL("https://youtube.com", "key"),
    /rtmp:\/\//,
  );
  assert.throws(
    () => buildPublishURL("rtmp://server", "key"),
    /application path/,
  );
  assert.throws(() => buildPublishURL("rtmp://server/live", ""), /stream key/);
  assert.throws(
    () => buildPublishURL("rtmp://server/live", "key with spaces"),
    /space or line break/,
  );
});
