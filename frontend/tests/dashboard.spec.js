import { test, expect } from "@playwright/test";

test.use({ viewport: { width: 1440, height: 1000 } });

async function signIn(page) {
  const status = await (await page.request.get("/api/v1/auth/status")).json();
  if (status.setup_required) {
    await page.goto("/app/#setup=ui-test-token");
    await page
      .getByLabel("New password", { exact: true })
      .fill("ui-test-password-2026");
    await page.getByLabel("Confirm password").fill("ui-test-password-2026");
    await page
      .getByRole("button", { name: "Create password & open dashboard" })
      .click();
  } else {
    await page.goto("/app/");
    await page
      .getByLabel("Password", { exact: true })
      .fill("ui-test-password-2026");
    await page.getByRole("button", { name: "Open my dashboard" }).click();
  }
  await expect(
    page.getByRole("heading", { name: "Your streaming dashboard." }),
  ).toBeVisible();
}

test("login, platform management, live settings and release configuration", async ({
  page,
  request,
}) => {
  const errors = [];
  page.on("pageerror", (error) => errors.push(error.message));
  expect((await request.get("/api/v1/connection")).status()).toBe(401);
  await signIn(page);
  await expect(
    page.getByRole("heading", { name: "Your streaming dashboard." }),
  ).toBeVisible();
  await page.evaluate(() => document.fonts.ready);
  const frame = await page.locator(".dashboard-frame").boundingBox();
  expect(frame.x).toBeGreaterThan(100);
  expect(frame.x + frame.width).toBeLessThan(1340);
  expect(
    await page
      .locator("body")
      .evaluate((el) => parseFloat(getComputedStyle(el).fontSize)),
  ).toBeGreaterThanOrEqual(18);
  expect(
    await page
      .locator(".page-heading>div>p:last-child")
      .evaluate((el) => parseFloat(getComputedStyle(el).fontSize)),
  ).toBeGreaterThanOrEqual(17);
  await expect(
    page.getByRole("heading", { name: "First stream? Start here." }),
  ).toBeVisible();
  await page.screenshot({
    path: test.info().outputPath("overview.png"),
    fullPage: true,
  });
  await page.getByRole("button", { name: "Add platform", exact: true }).click();
  await page.getByLabel("2. Give it a name").fill("YouTube test");
  await page.getByLabel("3. RTMP server URL").fill("rtmp://127.0.0.1:1/live");
  await page
    .getByLabel("4. Stream key", { exact: true })
    .fill("test-private-key");
  await page.screenshot({
    path: test.info().outputPath("add-platform.png"),
    fullPage: true,
  });
  await page
    .getByRole("button", { name: "Add platform", exact: true })
    .last()
    .click();
  await expect(page.getByText("YouTube test", { exact: true })).toBeVisible();
  const toggle = page.getByRole("switch", { name: "Enable YouTube test" });
  await expect(toggle).toBeChecked();
  await toggle.click();
  await expect(toggle).not.toBeChecked();
  await page.getByRole("button", { name: "Edit YouTube test" }).click();
  await expect(page.getByLabel("3. RTMP server URL")).toHaveValue(
    "rtmp://127.0.0.1:1/live",
  );
  await expect(page.getByLabel("4. Stream key", { exact: true })).toHaveValue(
    "test-private-key",
  );
  await page.getByLabel("2. Give it a name").fill("Twitch test");
  await page.getByRole("button", { name: "Save changes" }).click();
  await expect(page.getByText("Twitch test", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Settings", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Connect OBS or your streaming app" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Show key" }).click();
  await expect(page.getByText("ui-stream-key", { exact: true })).toBeVisible();
  await expect(page.getByLabel("S3 API endpoint")).toBeHidden();
  await page
    .getByText("Optional: cloud storage & CDN publishing", { exact: true })
    .click();
  await page.getByLabel("S3 API endpoint").fill("http://127.0.0.1:1");
  await page
    .getByLabel("Public bucket / CDN base URL")
    .fill("https://cdn.example.com");
  await page.getByLabel("Bucket", { exact: true }).fill("streams");
  await page.getByLabel("Access key ID", { exact: true }).fill("ui-access-key");
  await page
    .getByLabel("Secret access key", { exact: true })
    .fill("ui-secret-key");
  await page
    .getByRole("switch", { name: "Publish HLS to object storage" })
    .click();
  await page.getByRole("button", { name: "Save storage settings" }).click();
  await expect(
    page.getByLabel("Secret access key", { exact: true }),
  ).toHaveValue("");
  const stored = await request.get("/api/v1/storage", {
    headers: { Authorization: "Bearer ui-test-token" },
  });
  const storage = await stored.json();
  expect(storage.has_secret).toBe(true);
  expect(storage.config.secret_key).toBeUndefined();
  await page.getByRole("switch", { name: "Dark little corner" }).click();
  await expect(page.locator("html")).toHaveClass(/dark/);
  const performanceSwitch = page.getByRole("switch", {
    name: "Performance mode",
  });
  const beforeBackground = await performanceSwitch.evaluate(
    (el) => getComputedStyle(el).backgroundColor,
  );
  await performanceSwitch.click();
  await expect(
    page.getByRole("switch", { name: "Performance mode" }),
  ).toBeChecked();
  await expect
    .poll(() =>
      performanceSwitch.evaluate((el) => getComputedStyle(el).backgroundColor),
    )
    .not.toBe(beforeBackground);
  await page.getByRole("tab", { name: "Updates", exact: true }).click();
  await page
    .getByLabel("GitHub release repository")
    .fill("example/multicaster");
  await page.getByRole("button", { name: "Save update source" }).click();
  await expect(page.getByLabel("GitHub release repository")).toHaveValue(
    "example/multicaster",
  );
  await page.getByRole("button", { name: "My platforms", exact: true }).click();
  await page.getByRole("button", { name: "Delete Twitch test" }).click();
  await page
    .getByRole("button", { name: "Remove platform", exact: true })
    .click();
  await expect(page.getByText("Twitch test", { exact: true })).toHaveCount(0);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/app/#overview");
  await expect(
    page.getByRole("heading", { name: "Your streaming dashboard." }),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: test.info().outputPath("mobile.png"),
    fullPage: true,
  });
  await page.getByRole("button", { name: "Open navigation" }).click();
  await page
    .getByRole("navigation", { name: "Mobile navigation" })
    .getByRole("button", { name: /Settings/ })
    .click();
  await expect(
    page.getByRole("heading", { name: "Connect OBS or your streaming app" }),
  ).toBeVisible();
  expect(errors).toEqual([]);
});

test("beginner platform presets, URL validation and full-URL import on mobile", async ({
  page,
  request,
}) => {
  await signIn(page);
  await page.getByRole("button", { name: "Add platform", exact: true }).click();
  await page.getByRole("button", { name: "YouTube", exact: true }).click();
  await expect(page.getByLabel("3. RTMP server URL")).toHaveValue(
    "rtmps://a.rtmps.youtube.com/live2",
  );
  await expect(
    page.getByLabel("4. Stream key", { exact: true }),
  ).toHaveAttribute("type", "password");
  await page
    .getByLabel("4. Stream key", { exact: true })
    .fill("new-channel-key");
  await page
    .getByLabel("3. RTMP server URL")
    .fill("https://not-a-video-server.example");
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Add platform", exact: true })
    .click();
  await expect(page.getByRole("alert")).toContainText(
    "must start with rtmp:// or rtmps://",
  );
  await page
    .getByText("Already have a full publish URL?", { exact: true })
    .click();
  await page
    .getByLabel("Full publish URL", { exact: true })
    .fill("rtmp://relay.example.com/live/channel-key?token=one");
  await page.getByRole("button", { name: "Fill in URL and key" }).click();
  await expect(page.getByLabel("3. RTMP server URL")).toHaveValue(
    "rtmp://relay.example.com/live",
  );
  await expect(page.getByLabel("4. Stream key", { exact: true })).toHaveValue(
    "channel-key?token=one",
  );
  await page.setViewportSize({ width: 390, height: 844 });
  const modal = await page.getByRole("dialog").boundingBox();
  expect(modal.width).toBeLessThanOrEqual(358);
  expect(modal.x).toBeGreaterThanOrEqual(15);
  expect(modal.y).toBeGreaterThanOrEqual(0);
  expect(modal.height).toBeLessThanOrEqual(844 - 40);
  await page.getByRole("dialog").evaluate((el) => {
    el.scrollTop = 0;
  });
  await page.screenshot({
    path: test.info().outputPath("mobile-add-platform.png"),
    fullPage: false,
  });
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Add platform", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  const headers = { Authorization: "Bearer ui-test-token" };
  const response = await request.get("/api/v1/destinations", { headers });
  const dests = await response.json();
  const added = dests.find(
    (d) => d.url === "rtmp://relay.example.com/live/channel-key?token=one",
  );
  expect(added).toBeTruthy();
  await request.delete("/api/v1/destinations/" + added.id, { headers });
});

test("password reset opens setup; updates/system stay in Settings; preview is opt-in", async ({
  page,
  request,
}) => {
  const headers = { Authorization: "Bearer ui-test-token" };
  const resetResponse = await request.post("/api/v1/auth/reset", { headers });
  expect(resetResponse.ok()).toBe(true);
  const reset = await resetResponse.json();
  await page.goto("/app/");
  await expect(
    page.getByRole("heading", { name: "Create your dashboard password" }),
  ).toBeVisible();
  await page.goto("/app/#setup=" + reset.setup_token);
  await expect(page.getByLabel("Setup code")).toHaveValue(reset.setup_token);
  await page
    .getByLabel("New password", { exact: true })
    .fill("ui-test-password-2026");
  await page.getByLabel("Confirm password").fill("ui-test-password-2026");
  await page
    .getByRole("button", { name: "Create password & open dashboard" })
    .click();
  await expect(
    page.getByRole("heading", { name: "Your streaming dashboard." }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Your stream, a little closer" }),
  ).toHaveCount(0);
  let mediaRequests = 0;
  page.on("request", (r) => {
    if (r.url().includes("/hls/") || r.url().includes("/assets/hls-"))
      mediaRequests++;
  });
  await page.getByRole("button", { name: "Settings", exact: true }).click();
  await page.getByRole("tab", { name: "System info" }).click();
  await expect(
    page.getByRole("heading", { name: "Your Multicaster server" }),
  ).toBeVisible();
  await expect(
    page.getByText("Configuration file", { exact: true }),
  ).toBeVisible();
  await page.getByRole("tab", { name: "Updates" }).click();
  await page.getByLabel("GitHub release repository").fill("");
  await expect(page.getByLabel('Update manifest URL')).toHaveValue('https://github.com/TrueFix/Multicaster/releases/latest/download/update-manifest.json')
  await page.getByLabel('Update manifest URL').fill('')
  await page.getByRole("button", { name: "Save update source" }).click();
  await expect(
    page.getByRole("button", { name: "Check for updates" }),
  ).toBeDisabled();
  await page.getByRole("tab", { name: "General" }).click();
  await page.getByRole("switch", { name: "Dashboard preview" }).click();
  await expect(
    page.getByRole("switch", { name: "Dashboard preview" }),
  ).toBeChecked();
  await page.getByRole("button", { name: "Overview", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Your stream, a little closer" }),
  ).toBeVisible();
  await expect(page.locator("video")).toHaveCount(0);
  expect(mediaRequests).toBe(0);
  await page.getByRole("button", { name: "Settings", exact: true }).click();
  await page.getByRole("switch", { name: "Dashboard preview" }).click();
  await page.getByRole("button", { name: "Overview", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Your stream, a little closer" }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "Lock dashboard" }).click();
  await expect(page.getByLabel("Password", { exact: true })).toBeVisible();
});
