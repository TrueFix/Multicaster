import { defineConfig } from "@playwright/test";
export default defineConfig({
  testDir: "./tests",
  workers: 1,
  use: { baseURL: "http://127.0.0.1:23080", headless: true },
  webServer: {
    command: "go run ./cmd/multicaster",
    cwd: "..",
    url: "http://127.0.0.1:23080/api/v1/health",
    timeout: 60000,
    env: {
      MULTICASTER_DATA_DIR: "/tmp/opencode/multicaster-ui-" + Date.now(),
      MULTICASTER_HTTP_LISTEN: "127.0.0.1:23080",
      MULTICASTER_RTMP_LISTEN: "127.0.0.1:23935",
      MULTICASTER_ADMIN_TOKEN: "ui-test-token",
      MULTICASTER_STREAM_KEY: "ui-stream-key",
    },
  },
});
