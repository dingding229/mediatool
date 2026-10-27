import { test, expect, type Page } from "@playwright/test";
import { mkdirSync } from "node:fs";
import { resolve } from "node:path";
import { u7Fixture } from "./u7-fixture";

const output = resolve(
  import.meta.dirname,
  "../../scratchpad/i-phase/i2/screens",
);
mkdirSync(output, { recursive: true });

type Target = {
  name: string;
  path: string;
  setup?: (page: Page) => Promise<void>;
  open: (page: Page) => Promise<void>;
  title: RegExp | string;
};
const targets: Target[] = [
  {
    name: "media-library",
    path: "/library",
    title: /作品详情/,
    open: async (page) => {
      await page.locator(".catalog-poster").first().click();
    },
  },
  {
    name: "user-detail",
    path: "/playback?tab=users",
    title: /用户详情/,
    open: async (page) => {
      await page
        .getByRole("button", { name: "用户详情", exact: true })
        .first()
        .click();
    },
  },
  {
    name: "log-detail",
    path: "/ops?tab=logs",
    title: "完整原始日志",
    open: async (page) => {
      await page
        .getByRole("button", { name: "查看完整链接", exact: true })
        .click();
    },
  },
  {
    name: "site-account",
    path: "/sites",
    title: "站点数据与趋势",
    open: async (page) => {
      await page.locator(".f5-site-name").first().click();
    },
  },
  {
    name: "organize-history",
    path: "/organize?tab=history",
    title: "整理步骤详情",
    setup: async (page) => {
      await page.route("**/api/v1/library/activity?*", (r) =>
        r.fulfill({
          json: {
            data: {
              items: [
                {
                  id: "history-owned",
                  kind: "file",
                  state: "succeeded",
                  source: "/115/电影/海边的故事.mkv",
                  destination: "/媒体/电影/海边的故事.mkv",
                  updatedAt: "2026-10-25T00:00:00Z",
                  createdAt: "2026-10-25T00:00:00Z",
                  media: { title: "海边的故事", year: "2026" },
                  details: {},
                },
              ],
              total: 1,
            },
          },
        }),
      );
    },
    open: async (page) => {
      await page
        .getByRole("button", { name: "查看步骤详情", exact: true })
        .first()
        .click({ timeout: 15000 });
    },
  },
];

for (const width of [1440, 390])
  for (const theme of ["light", "dark"])
    for (const target of targets) {
      test(`I2 ${target.name} ${width} ${theme}`, async ({ page }) => {
        await page.setViewportSize({ width, height: 1000 });
        await page.addInitScript(
          (mode) => localStorage.setItem("media-tool:theme-mode:v1", mode),
          theme,
        );
        await u7Fixture(page, target.path.split("?")[0]);
        await target.setup?.(page);
        await page.goto(target.path);
        await target.open(page);
        const dialog = page.getByRole("dialog").last();
        await expect(dialog).toBeVisible({ timeout: 15000 });
        await expect(
          page
            .getByRole("button", { name: "关闭此对话框", exact: true })
            .last(),
        ).toHaveCount(1);
        await expect(dialog).toBeVisible();
        await expect(
          page
            .getByRole("button", { name: "关闭此对话框", exact: true })
            .last(),
        ).toHaveAttribute("aria-label", "关闭此对话框");
        expect(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= innerWidth,
          ),
        ).toBe(true);
        await page.screenshot({
          path: resolve(output, `${target.name}-${width}-${theme}.png`),
          fullPage: true,
          animations: "disabled",
        });
        await dialog
          .getByRole("button", { name: "关闭此对话框", exact: true })
          .click();
        await expect(dialog).toBeHidden();
      });
    }
