import { test, expect } from "@playwright/test";

test("app shell can load about:blank (placeholder for IDE preview E2E)", async ({ page }) => {
  await page.goto("about:blank");
  await expect(page.locator("body")).toBeVisible();
});
