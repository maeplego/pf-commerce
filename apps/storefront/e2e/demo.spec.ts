import { expect, test } from "@playwright/test";

test("demo page shows simultaneous-buy panes", async ({ page }) => {
  await page.goto("/demo");
  await expect(page.getByRole("heading", { name: "在庫 1 の同時購入" })).toBeVisible();
  await expect(page.getByRole("button", { name: "MUG-1 を 1点買う" })).toHaveCount(2);
});
